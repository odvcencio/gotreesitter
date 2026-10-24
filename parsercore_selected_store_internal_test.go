//go:build gts_parsercorephase0

package gotreesitter

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCoreSelectedStoreCanonicalAdmissions(t *testing.T) {
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		t.Run(row.id, func(t *testing.T) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
			runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
			if err != nil {
				t.Fatal(err)
			}
			scheduler, tokenSource, err := runner.executeSchedulerOpen(fixture.Source, runner.compact, false)
			if tokenSource != nil {
				defer tokenSource.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(scheduler.acceptedPayloads) == 0 {
				t.Fatal("accepted payloads were not carried from acceptance")
			}
			store, err := runner.compact.BuildAuthenticatedSelectedStore(scheduler.acceptedPayloads, fixture.Source, func() error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			defer store.Release()
			if got := store.NodeCount(); got != row.selectedNodes {
				t.Fatalf("selected store nodes=%d want=%d", got, row.selectedNodes)
			}
			if bytesPerNode := store.RetainedBytes() / store.NodeCount(); bytesPerNode > 80 {
				t.Fatalf("selected store retained bytes/node=%d want<=80", bytesPerNode)
			}
			tree, materializeErr := runner.materialize(fixture.Source, runner.compact, scheduler.acceptedHead)
			if materializeErr != nil {
				t.Fatal(materializeErr)
			}
			defer tree.Release()
			requireDiagnosticParserCoreSelectedStoreMatchesTree(t, store, tree.root, runner.lang)
			if got := diagnosticParserCoreSelectedStoreDeepDigest(t, store, runner.lang, fixture.Source); got != row.deepTreeSHA256 {
				t.Fatalf("selected store digest=%s want=%s", got, row.deepTreeSHA256)
			}
			if work := runner.compact.Work(); work != row.work || work.Overflow {
				t.Fatalf("selected store work=%+v want=%+v", work, row.work)
			}
			t.Logf("SELECTED_STORE fixture=%s nodes=%d retained_bytes=%d", row.id, store.NodeCount(), store.RetainedBytes())
		})
	}
}

func TestDiagnosticParserCoreSelectedStoreCompletenessFailureReleasesBacking(t *testing.T) {
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	store, err := runner.parseSelectedStore(fixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	if adopted, err := requireParserCoreSelectedStoreCompleteness(store, len(fixture.Source)+1); err == nil || adopted != nil {
		t.Fatalf("incomplete selected store adopted=%v err=%v", adopted, err)
	}
	if store.NodeCount() != 0 || store.RetainedBytes() != 0 {
		t.Fatalf("failed completeness retained nodes=%d bytes=%d", store.NodeCount(), store.RetainedBytes())
	}
	fresh, err := runner.parseSelectedStore(fixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Release()
}

func requireDiagnosticParserCoreSelectedStoreMatchesTree(t testing.TB, store *core.SelectedStore, root *Node, lang *Language) {
	t.Helper()
	type pair struct {
		id    core.SelectedNodeID
		node  *Node
		field core.FieldID
	}
	stack := []pair{{id: store.Root(), node: root}}
	for visited := 0; len(stack) != 0; visited++ {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		record, ok := store.Record(current.id)
		if !ok || current.node == nil {
			t.Fatalf("selected store/tree missing node at visit %d", visited)
		}
		treeField := FieldID(0)
		if current.node.parent != nil && current.node.childIndex >= 0 {
			treeField = nodeFieldIDAt(current.node.parent, int(current.node.childIndex))
		}
		terminal := current.node.ChildCount() == 0
		if Symbol(record.Symbol) != current.node.symbol || record.StartByte != current.node.startByte || record.EndByte != current.node.endByte ||
			record.Named() != current.node.IsNamed() || record.Extra() != current.node.IsExtra() || record.External() != current.node.hasFlag(nodeFlagExternalScannerToken) ||
			record.Terminal() != terminal || record.Missing() != current.node.IsMissing() || record.HasError() != current.node.HasError() ||
			int(record.ChildCount) != current.node.ChildCount() || FieldID(current.field) != treeField ||
			record.ProductionID != current.node.productionID || record.DynamicPrecedence != current.node.dynamicPrecedence || record.SupertypeMask != current.node.supertypeMask() {
			t.Fatalf("selected store metadata mismatch visit=%d store=%+v tree={sym:%d span:%d..%d named:%t extra:%t external:%t terminal:%t missing:%t has_error:%t children:%d field:%d production:%d precedence:%d type:%s}", visited, record, current.node.symbol, current.node.startByte, current.node.endByte, current.node.IsNamed(), current.node.IsExtra(), current.node.hasFlag(nodeFlagExternalScannerToken), terminal, current.node.IsMissing(), current.node.HasError(), current.node.ChildCount(), treeField, current.node.productionID, current.node.dynamicPrecedence, current.node.Type(lang))
		}
		for index := int(record.ChildCount) - 1; index >= 0; index-- {
			childID, _ := store.Child(record, uint32(index))
			childRecord, _ := store.Record(childID)
			stack = append(stack, pair{id: childID, node: current.node.Child(index), field: childRecord.Field})
		}
	}
}

type selectedStoreDigest struct {
	h     hash.Hash
	open  uint64
	nodes uint64
}

func diagnosticParserCoreSelectedStoreDeepDigest(t testing.TB, store *core.SelectedStore, lang *Language, source []byte) string {
	t.Helper()
	if store == nil || lang == nil {
		t.Fatal("selected store digest requires store and language")
	}
	d := selectedStoreDigest{h: sha256.New(), open: 1}
	_, _ = d.h.Write([]byte("gts-deep-tree-v1\x00"))
	points, err := newDiagnosticParserCorePointIndex(source, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	type item struct {
		id    core.SelectedNodeID
		field core.FieldID
	}
	stack := []item{{id: store.Root()}}
	for len(stack) != 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		record, ok := store.Record(current.id)
		if !ok {
			t.Fatalf("selected store omitted node %d", current.id)
		}
		typeName := ""
		if int(record.Symbol) < len(lang.SymbolNames) {
			typeName = unescapePunctuationSymbolName(lang.SymbolNames[record.Symbol])
		}
		fieldName := ""
		if current.field != 0 && int(current.field) < len(lang.FieldNames) {
			fieldName = lang.FieldNames[current.field]
		}
		start, end := points.point(record.StartByte), points.point(record.EndByte)
		d.addString(typeName)
		d.addString(fieldName)
		for _, value := range [...]uint32{record.StartByte, record.EndByte, start.Row, start.Column, end.Row, end.Column} {
			d.add32(value)
		}
		var flags byte
		if record.Named() {
			flags |= 1 << 0
		}
		if record.Extra() {
			flags |= 1 << 1
		}
		_, _ = d.h.Write([]byte{flags})
		d.add32(uint32(record.ChildCount))
		if d.open == 0 {
			t.Fatal("selected store digest closed before traversal ended")
		}
		d.open--
		d.open += uint64(record.ChildCount)
		d.nodes++
		for index := int(record.ChildCount) - 1; index >= 0; index-- {
			childID, ok := store.Child(record, uint32(index))
			if !ok {
				t.Fatalf("selected store child window drifted at node %d child %d", current.id, index)
			}
			child, ok := store.Record(childID)
			if !ok || child.Parent != current.id {
				t.Fatalf("selected store parent linkage drifted at child %d", childID)
			}
			stack = append(stack, item{id: childID, field: child.Field})
		}
	}
	if d.open != 0 || d.nodes == 0 {
		t.Fatalf("selected store digest incomplete: open=%d nodes=%d", d.open, d.nodes)
	}
	return fmt.Sprintf("%x", d.h.Sum(nil))
}

func (d *selectedStoreDigest) addString(value string) {
	d.add32(uint32(len(value)))
	_, _ = d.h.Write([]byte(value))
}

func (d *selectedStoreDigest) add32(value uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], value)
	_, _ = d.h.Write(buf[:])
}

var yamlSelectedProjectionLanguageForTest *Language

func SetYAMLSelectedProjectionLanguageForTest(lang *Language) {
	yamlSelectedProjectionLanguageForTest = lang
}
func TestOwnedYAMLSelectedProjectionAdmission(t *testing.T) {
	t.Cleanup(core.SetExternalLexerOwnershipProbeForTest(true))
	lang := yamlSelectedProjectionLanguageForTest
	if lang == nil {
		t.Fatal("YAML loader absent")
	}
	sample, err := os.ReadFile("internal/benchfixtures/testdata/real/yaml")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(sample)
	if len(sample) != 1998 || hex.EncodeToString(sum[:]) != "5aa2c70bccfe73254f13eca40476c3f88f522c6a47aea499f9d672dbc5aae8c4" {
		t.Fatal("sample identity drift")
	}
	document := append([]byte("---\n"), sample...)
	document = append(document, '\n')
	for _, fixture := range []struct {
		name   string
		source []byte
	}{{"r4", sample}, {"r4_leading_padding", append([]byte("  "), sample...)}, {"r4_leading_newline", append([]byte("\n  "), sample...)}, {"eof_8k", bytes.Repeat(document, 4)}} {
		t.Run(fixture.name, func(t *testing.T) {
			parser := NewParser(lang)
			runner, err := newAdmissionCandidateRunner(parser)
			if err != nil {
				t.Fatal(err)
			}
			scheduler, tokens, err := runner.executeSchedulerOpen(fixture.source, runner.compact, true)
			if tokens != nil {
				defer tokens.Close()
			}
			if err != nil {
				t.Fatalf("owned acceptance: %v", err)
			}
			for _, id := range scheduler.acceptedPayloads {
				view, e := runner.compact.MaterializationView(id)
				if e != nil {
					t.Fatal(e)
				}
				t.Logf("YAML_RAW_ROOT fixture=%s id=%d symbol=%d production=%d extra=%v children=%d", fixture.name, id, view.Symbol, view.ProductionID, view.Extra, len(view.Children))
			}
			work := runner.compact.Work()
			store, projectionErr := runner.compact.BuildAuthenticatedSelectedStore(scheduler.acceptedPayloads, fixture.source, func() error { return nil })
			if projectionErr != nil {
				t.Fatalf("YAML_SELECTED_PROJECTION fixture=%s bytes=%d declined=%q work=%+v", fixture.name, len(fixture.source), projectionErr, work)
			}
			defer store.Release()
			tree, err := runner.materializeSelection(fixture.source, runner.compact, scheduler)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			requireDiagnosticParserCoreSelectedStoreMatchesTree(t, store, tree.root, lang)
			if runner.compact.Work() != work {
				t.Fatal("projection changed committed work")
			}
			t.Logf("YAML_SELECTED_PROJECTION fixture=%s bytes=%d nodes=%d retained=%d admitted=true", fixture.name, len(fixture.source), store.NodeCount(), store.RetainedBytes())
		})
	}
}

func BenchmarkOwnedYAMLSelectedProjectionPublication(b *testing.B) {
	b.Cleanup(core.SetExternalLexerOwnershipProbeForTest(true))
	lang := yamlSelectedProjectionLanguageForTest
	if lang == nil {
		b.Fatal("YAML loader absent")
	}
	source, err := os.ReadFile("internal/benchfixtures/testdata/real/yaml")
	if err != nil {
		b.Fatal(err)
	}
	if len(source) != 1998 {
		b.Fatal("YAML sample size drift")
	}
	runner, err := newAdmissionCandidateRunner(NewParser(lang))
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(source)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selected, tokens, err := runner.executeSchedulerOpen(source, runner.compact, true)
		if err != nil {
			if tokens != nil {
				tokens.Close()
			}
			b.Fatal(err)
		}
		store, err := runner.compact.BuildAuthenticatedSelectedStore(selected.acceptedPayloads, source, func() error { return nil })
		if err != nil {
			if tokens != nil {
				tokens.Close()
			}
			b.Fatal(err)
		}
		if store.NodeCount() != 231 {
			b.Fatalf("selected YAML node count=%d", store.NodeCount())
		}
		store.Release()
		if tokens != nil {
			tokens.Close()
		}
	}
}

func TestNativeProjectionUnusedSourceMapIsReleased(t *testing.T) {
	t.Cleanup(core.SetExternalLexerOwnershipProbeForTest(false))
	source, err := os.ReadFile("internal/benchfixtures/testdata/real/yaml")
	if err != nil {
		t.Fatal(err)
	}
	runner, err := newAdmissionCandidateRunner(NewParser(yamlSelectedProjectionLanguageForTest))
	if err != nil {
		t.Fatal(err)
	}
	if runner.scratch.nativeProjection == nil {
		t.Fatal("native publication fixture unavailable")
	}
	// Publication must release every public-node pointer after use.
	scheduler, tokens, err := runner.executeSchedulerOpen(source, runner.compact, true)
	if tokens != nil {
		defer tokens.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.reuseDependencies.reads != nil || compactIncrementalReuseProvenForLanguage(runner.lang) {
		t.Fatal("fixture now consumes source-map certificates")
	}
	tree, err := runner.materializeSelection(source, runner.compact, scheduler)
	if err != nil || tree == nil {
		t.Fatalf("native publication: %v", err)
	}
	defer tree.Release()
	if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) || tree.ParseStopReason() != ParseStopAccepted {
		t.Fatal("incomplete native tree")
	}
	if !tree.incrementalReuseDisabled {
		t.Fatal("unproven scanner advertised reuse")
	}
	if runner.scratch.nativeProjection.records.Len() != 0 {
		t.Fatal("published source records retained")
	}
}

var quiescentNativeGoLanguageForTest func() *Language

func SetQuiescentNativeGoLanguageForTest(language func() *Language) {
	quiescentNativeGoLanguageForTest = language
}

func TestQuiescentNativeGoPublicationMatchesReference(t *testing.T) {
	check := func(t *testing.T, source []byte, wide bool) {
		t.Helper()
		makeRunner := func(native bool) *parserCoreFreshFullRunner {
			r, err := newAdmissionCandidateRunner(NewParser(quiescentNativeGoLanguageForTest()))
			if err != nil {
				t.Fatal(err)
			}
			if !native {
				r.scratch.nativeProjection = nil
			}
			return r
		}
		reference := makeRunner(false)
		before, err := reference.parse(source)
		if err != nil {
			t.Fatal(err)
		}
		defer before.Release()
		candidate := makeRunner(true)
		if candidate.scratch.nativeProjection == nil {
			t.Fatal("stateless Go publisher unavailable")
		}
		marker := []uint32{0xbadc0de}
		candidate.scratch.nativeProjection.lines = marker[:0]
		after, err := candidate.parse(source)
		if err != nil {
			t.Fatal(err)
		}
		defer after.Release()
		if marker[0] != 0 {
			t.Fatal("native publication did not execute")
		}
		a := requireDiagnosticParserCoreCanonicalTreeDigest(t, before, candidate.lang)
		b := requireDiagnosticParserCoreCanonicalTreeDigest(t, after, candidate.lang)
		if a != b {
			t.Fatalf("publication digest changed: %s / %s", a, b)
		}
		type pair struct{ a, b *Node }
		stack := []pair{{before.RootNode(), after.RootNode()}}
		maxChildren := 0
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			mask := ^(nodeFlagFieldIDCacheComputed | nodeFlagFieldIDCacheHasFieldIDs)
			if p.a.parseState != p.b.parseState || p.a.preGotoState != p.b.preGotoState || p.a.productionID != p.b.productionID || p.a.dynamicPrecedence != p.b.dynamicPrecedence || p.a.flags&mask != p.b.flags&mask || p.a.supertypeMask() != p.b.supertypeMask() || p.a.dependsOnColumn() != p.b.dependsOnColumn() {
				t.Fatalf("publication metadata changed at %s %d..%d", p.a.Type(candidate.lang), p.a.StartByte(), p.a.EndByte())
			}
			x, xok := compactReuseDependencyForNode(p.a)
			y, yok := compactReuseDependencyForNode(p.b)
			if x != y || xok != yok {
				t.Fatalf("Go compact receipt changed at %s %d..%d: %d/%t vs %d/%t", p.a.Type(candidate.lang), p.a.StartByte(), p.a.EndByte(), x, xok, y, yok)
			}
			xw, yw := legacyReuseWord(p.a, false), legacyReuseWord(p.b, false)
			if (xw == nil) != (yw == nil) || xw != nil && *xw != *yw {
				t.Fatalf("Go legacy receipt changed at %s %d..%d", p.a.Type(candidate.lang), p.a.StartByte(), p.a.EndByte())
			}
			if p.a.ChildCount() > maxChildren {
				maxChildren = p.a.ChildCount()
			}
			for i := 0; i < p.a.ChildCount(); i++ {
				stack = append(stack, pair{p.a.Child(i), p.b.Child(i)})
			}
		}
		if wide && maxChildren <= 65535 {
			t.Fatalf("wide witness failed to exceed uint16: %d", maxChildren)
		}
		t.Logf("NATIVE_GO bytes=%d full_digest=%s max_children=%d metadata_equal=true", len(source), a, maxChildren)
	}
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		t.Run(row.id, func(t *testing.T) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
			check(t, fixture.Source, false)
		})
	}
	t.Run("wide_parent", func(t *testing.T) {
		source := append([]byte("package p\nvar X = []int{"), bytes.Repeat([]byte("1,"), 32769)...)
		source = append(source, []byte("}\n")...)
		check(t, source, true)
	})
}

func TestQuiescentNativeGoRecoveryKeepsReferencePublisher(t *testing.T) {
	source := []byte("package main\n\nimport \"fmt\"\n\nfunc f0(a int, b int) int {\n\t := a + b\n\tfmt.Println(\"f0\", x)\n\treturn x\n}\n")
	candidate, err := newAdmissionCandidateRunner(NewParser(quiescentNativeGoLanguageForTest()))
	if err != nil {
		t.Fatal(err)
	}
	reference, err := newAdmissionCandidateRunner(NewParser(quiescentNativeGoLanguageForTest()))
	if err != nil {
		t.Fatal(err)
	}
	reference.scratch.nativeProjection = nil
	marker := []uint32{0xbadc0de}
	candidate.scratch.nativeProjection.lines = marker[:0]
	before, beforeErr := reference.parse(source)
	if before != nil {
		defer before.Release()
	}
	after, afterErr := candidate.parse(source)
	if after != nil {
		defer after.Release()
	}
	if (beforeErr == nil) != (afterErr == nil) {
		t.Fatalf("recovery disposition changed: %v / %v", beforeErr, afterErr)
	}
	if marker[0] != 0xbadc0de {
		t.Fatal("recovered derivation entered clean native publication")
	}
	if beforeErr == nil && requireDiagnosticParserCoreCanonicalTreeDigest(t, before, candidate.lang) != requireDiagnosticParserCoreCanonicalTreeDigest(t, after, candidate.lang) {
		t.Fatal("recovered tree changed")
	}
}

var quiescentNativeFaustLanguageForTest func() *Language

func SetQuiescentNativeFaustLanguageForTest(language func() *Language) {
	quiescentNativeFaustLanguageForTest = language
}

func TestQuiescentNativeHiddenDirectFieldPunctuation(t *testing.T) {
	source, err := os.ReadFile("internal/benchfixtures/testdata/real/faust")
	if err != nil {
		t.Fatal(err)
	}
	for _, edited := range []bool{false, true} {
		input := source
		if edited {
			input = append(append(append([]byte(nil), source[:1287]...), 'x'), source[1287:]...)
		}
		reference, err := newAdmissionCandidateRunner(NewParser(quiescentNativeFaustLanguageForTest()))
		if err != nil {
			t.Fatal(err)
		}
		reference.scratch.nativeProjection = nil
		before, err := reference.parse(input)
		if err != nil {
			t.Fatal(err)
		}
		defer before.Release()
		candidate, err := newAdmissionCandidateRunner(NewParser(quiescentNativeFaustLanguageForTest()))
		if err != nil {
			t.Fatal(err)
		}
		if candidate.scratch.nativeProjection == nil {
			t.Fatal("quiescent publisher unavailable")
		}
		marker := []uint32{0xbadc0de}
		candidate.scratch.nativeProjection.lines = marker[:0]
		after, err := candidate.parse(input)
		if err != nil {
			t.Fatal(err)
		}
		defer after.Release()
		if marker[0] != 0 {
			t.Fatal("native publication did not execute")
		}
		a := requireDiagnosticParserCoreCanonicalTreeDigest(t, before, candidate.lang)
		b := requireDiagnosticParserCoreCanonicalTreeDigest(t, after, candidate.lang)
		if a != b {
			t.Fatalf("hidden field projection changed: %s / %s", a, b)
		}
		stack := []*Node{after.RootNode()}
		punctuation := 0
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for i := 0; i < n.ChildCount(); i++ {
				child := n.Child(i)
				stack = append(stack, child)
				if (child.StartByte() == 740 || child.StartByte() == 1069) && child.EndByte() == child.StartByte()+1 {
					if n.FieldNameForChild(i, candidate.lang) != "left" {
						t.Fatal("parenthesized hidden slot lost left field")
					}
					punctuation++
				}
			}
		}
		if punctuation != 2 {
			t.Fatalf("punctuation witness drifted: %d", punctuation)
		}
		t.Logf("NATIVE_FAUST edited=%t full_digest=%s punctuation_fields=%d", edited, a, punctuation)
	}
}

var quiescentNativeINILanguageForTest func() *Language

func SetQuiescentNativeINILanguageForTest(language func() *Language) {
	quiescentNativeINILanguageForTest = language
}

func TestQuiescentNativeHiddenExtras(t *testing.T) {
	source, err := os.ReadFile("internal/benchfixtures/testdata/real/ini")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{[]byte("[s]\na = 1\n\nb = 2\n"), source} {
		language := quiescentNativeINILanguageForTest()
		reference, err := newAdmissionCandidateRunner(NewParser(language))
		if err != nil {
			t.Fatal(err)
		}
		reference.scratch.nativeProjection = nil
		before, err := reference.parse(input)
		if err != nil {
			t.Fatal(err)
		}
		defer before.Release()
		candidate, err := newAdmissionCandidateRunner(NewParser(language))
		if err != nil {
			t.Fatal(err)
		}
		if candidate.scratch.nativeProjection == nil {
			t.Fatal("quiescent publisher unavailable")
		}
		marker := []uint32{0xbadc0de}
		candidate.scratch.nativeProjection.lines = marker[:0]
		after, err := candidate.parse(input)
		if err != nil {
			t.Fatal(err)
		}
		defer after.Release()
		if marker[0] != 0 {
			t.Fatal("native publication did not execute")
		}
		a := requireDiagnosticParserCoreCanonicalTreeDigest(t, before, language)
		b := requireDiagnosticParserCoreCanonicalTreeDigest(t, after, language)
		if a != b {
			t.Fatalf("hidden extra projection changed: %s / %s", a, b)
		}
		stack := []*Node{after.RootNode()}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for i := 0; i < n.ChildCount(); i++ {
				child := n.Child(i)
				if !language.SymbolMetadata[child.symbol].Visible {
					t.Fatalf("invisible extra escaped at %d..%d", child.StartByte(), child.EndByte())
				}
				stack = append(stack, child)
			}
		}
		t.Logf("NATIVE_HIDDEN_EXTRA bytes=%d full_digest=%s", len(input), a)
	}
}

var quiescentNativeAdaLanguageForTest func() *Language

func SetQuiescentNativeAdaLanguageForTest(language func() *Language) {
	quiescentNativeAdaLanguageForTest = language
}
func TestQuiescentNativeUnsupportedFieldsKeepReferencePublisher(t *testing.T) {
	source, err := os.ReadFile("internal/benchfixtures/testdata/real/ada")
	if err != nil {
		t.Fatal(err)
	}
	language := quiescentNativeAdaLanguageForTest()
	candidate, err := newAdmissionCandidateRunner(NewParser(language))
	if err != nil {
		t.Fatal(err)
	}
	marker := []uint32{0xbadc0de}
	candidate.scratch.nativeProjection.lines = marker[:0]
	after, err := candidate.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Release()
	if marker[0] != 0xbadc0de {
		t.Fatal("unsupported fields entered native publication")
	}
	reference, err := newAdmissionCandidateRunner(NewParser(language))
	if err != nil {
		t.Fatal(err)
	}
	reference.scratch.nativeProjection = nil
	before, err := reference.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer before.Release()
	if before.ParseRuntime().NodesAllocated != after.ParseRuntime().NodesAllocated {
		t.Fatal("unsupported profile changed node work")
	}
	if requireDiagnosticParserCoreCanonicalTreeDigest(t, before, language) != requireDiagnosticParserCoreCanonicalTreeDigest(t, after, language) {
		t.Fatal("reference publication changed")
	}
}

var quiescentNativeReuseLanguageForTest func(string) *Language

func SetQuiescentNativeReuseLanguageForTest(factory func(string) *Language) {
	quiescentNativeReuseLanguageForTest = factory
}
func TestQuiescentNativeReuseMetadataMatchesReference(t *testing.T) {
	for _, name := range []string{"foam", "hyprlang", "pkl", "smithy"} {
		t.Run(name, func(t *testing.T) {
			language := quiescentNativeReuseLanguageForTest(name)
			source, err := os.ReadFile("internal/benchfixtures/testdata/real/" + name)
			if err != nil {
				t.Fatal(err)
			}
			reference, err := newAdmissionCandidateRunner(NewParser(language))
			if err != nil {
				t.Fatal(err)
			}
			reference.scratch.nativeProjection = nil
			before, err := reference.parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer before.Release()
			candidate, err := newAdmissionCandidateRunner(NewParser(language))
			if err != nil {
				t.Fatal(err)
			}
			if candidate.scratch.nativeProjection == nil {
				t.Fatal("quiescent publisher unavailable")
			}
			marker := []uint32{0xbadc0de}
			candidate.scratch.nativeProjection.lines = marker[:0]
			after, err := candidate.parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer after.Release()
			if marker[0] != 0 {
				t.Fatal("native publication did not execute")
			}
			a := requireDiagnosticParserCoreCanonicalTreeDigest(t, before, language)
			b := requireDiagnosticParserCoreCanonicalTreeDigest(t, after, language)
			if a != b {
				t.Fatalf("published tree changed: %s / %s", a, b)
			}
			type pair struct{ a, b *Node }
			stack := []pair{{before.RootNode(), after.RootNode()}}
			nodes := 0
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				nodes++
				mask := ^(nodeFlagFieldIDCacheComputed | nodeFlagFieldIDCacheHasFieldIDs)
				if p.a.parseState != p.b.parseState || p.a.preGotoState != p.b.preGotoState || p.a.productionID != p.b.productionID || p.a.dynamicPrecedence != p.b.dynamicPrecedence || p.a.flags&mask != p.b.flags&mask || p.a.supertypeMask() != p.b.supertypeMask() {
					t.Fatalf("replay metadata changed at %s %d..%d: state %d/%d pre %d/%d production %d/%d", p.a.Type(language), p.a.StartByte(), p.a.EndByte(), p.a.parseState, p.b.parseState, p.a.preGotoState, p.b.preGotoState, p.a.productionID, p.b.productionID)
				}
				x, xok := compactReuseDependencyForNode(p.a)
				y, yok := compactReuseDependencyForNode(p.b)
				if x != y || xok != yok {
					t.Fatalf("compact dependency changed at %s %d..%d: %d/%t vs %d/%t", p.a.Type(language), p.a.StartByte(), p.a.EndByte(), x, xok, y, yok)
				}
				xw, yw := legacyReuseWord(p.a, false), legacyReuseWord(p.b, false)
				if (xw == nil) != (yw == nil) || xw != nil && *xw != *yw {
					t.Fatalf("legacy dependency changed at %s %d..%d", p.a.Type(language), p.a.StartByte(), p.a.EndByte())
				}
				for i := 0; i < p.a.ChildCount(); i++ {
					stack = append(stack, pair{p.a.Child(i), p.b.Child(i)})
				}
			}
			if before.incrementalReuseDisabled != after.incrementalReuseDisabled {
				t.Fatal("tree reuse admission changed")
			}
			t.Logf("NATIVE_REUSE language=%s nodes=%d full_digest=%s metadata_and_receipts_equal=true", name, nodes, a)
		})
	}
}

func TestNativeProjectionAliasReceiptRejectsChangedPublication(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	workspace := nativeProjectionWorkspace{}
	workspace.records.Append(nativeStreamRecord{})
	workspace.records.Append(nativeStreamRecord{node: leaf, metadata: nativeStreamMetadata{source: 7, field: 3}})
	if !workspace.matchesPublishedNode(arena, leaf, 7) {
		t.Fatal("original publication did not match")
	}
	if workspace.matchesPublishedNode(arena, leaf, 8) {
		t.Fatal("different raw source retained publication proof")
	}
	leaf.symbol = 4
	if workspace.matchesPublishedNode(arena, leaf, 7) {
		t.Fatal("changed alias retained publication proof")
	}
	leaf.symbol = 3
	clone := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	if workspace.matchesPublishedNode(arena, clone, 7) {
		t.Fatal("unpublished clone retained publication proof")
	}
	workspace.records.Append(nativeStreamRecord{node: leaf, metadata: nativeStreamMetadata{source: 7, field: 3}})
	if workspace.matchesPublishedNode(arena, clone, 7) {
		t.Fatal("different node at arena index retained publication proof")
	}
	workspace.reset()
	if workspace.matchesPublishedNode(arena, leaf, 7) {
		t.Fatal("reset retained publication proof")
	}
}
