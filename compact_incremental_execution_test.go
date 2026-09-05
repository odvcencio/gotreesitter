//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompactIncrementalExecutionLifetime(t *testing.T) {
	for _, separator := range []string{"", "// between declarations\n"} {
		t.Run(separator, func(t *testing.T) {
			source := []byte("package p\nfunc a() { _ = 1 }\n" + separator + "func b() { _ = 2 }\n")
			parser := newAdmissionCandidateGoParser(t)
			parser.SetAdmissionCandidateRoute(true)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if old != nil {
					old.Release()
				}
			}()
			if !old.compactMaterialized {
				t.Fatal("initial tree is not compact")
			}
			for _, replacement := range []string{"1+3", "1+3+4"} {
				offset := bytes.IndexByte(source, '1')
				end := bytes.Index(source[offset:], []byte(" }")) + offset
				edited, edit := compactExecutionEdit(source, offset, end, replacement)
				oldB := compactExecutionNode(old.RootNode(), parser.language, "function_declaration", uint32(bytes.Index(source, []byte("func b"))))
				if oldB == nil {
					t.Fatal("initial tree has no function b")
				}
				old.Edit(edit)
				routed, fallback := AdmissionCandidateCounters()
				runner := parser.admissionCandidateRunner.(*parserCoreFreshFullRunner)
				legacyRuns := runner.legacyParseRuns
				next, profile, err := parser.ParseIncrementalProfiled(edited, old)
				if err != nil {
					t.Fatal(err)
				}
				if next == old {
					t.Fatal("edited parse returned the old tree")
				}
				old.Release()
				old = next
				if r, f := AdmissionCandidateCounters(); r != routed || f != fallback {
					t.Fatal("incremental parse changed full admission counters")
				}
				newB := compactExecutionNode(next.RootNode(), parser.language, "function_declaration", uint32(bytes.Index(edited, []byte("func b"))))
				if newB != oldB {
					t.Fatalf("function b lost its node identity: decline=%q", next.rawParseRuntime().CompactIncrementalFallbackReason)
				}
				if !next.compactMaterialized {
					t.Fatalf("incremental tree is not compact: replacement=%q decline=%q", replacement, next.rawParseRuntime().CompactIncrementalFallbackReason)
				}
				if runner.legacyParseRuns != legacyRuns {
					t.Fatal("compact incremental parse invoked the legacy parser")
				}
				runtime := next.ParseRuntime()
				if !runtime.CompactIncrementalReuseRoute || runtime.CompactIncrementalReusedSubtrees == 0 || runtime.CompactIncrementalReusedBytes == 0 {
					t.Fatalf("missing compact execution: %+v", runtime)
				}
				if runtime.CompactIncrementalReusedSubtrees != profile.ReusedSubtrees || runtime.CompactIncrementalReusedBytes != profile.ReusedBytes {
					t.Fatalf("compact reuse counters disagree with the profile: runtime=%+v profile=%+v", runtime, profile)
				}
				if profile.ReuseUnsupported || !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
					t.Fatalf("missing reuse: %+v", profile)
				}
				if !runtime.IncrementalOldTreeReuseRoute || profile.StopReason != ParseStopAccepted ||
					profile.LastTokenEndByte != uint32(len(edited)) || profile.ExpectedEOFByte != uint32(len(edited)) ||
					profile.ReuseCursorNanos <= 0 || profile.ReparseNanos <= 0 ||
					profile.NewNodesAllocated != uint64(runtime.NodesAllocated) {
					t.Fatalf("compact profile omitted execution metadata: %+v", profile)
				}
				if next.RootNode().HasError() || newB.Text(edited) != "func b() { _ = 2 }" {
					t.Fatal("released source tree damaged the result")
				}
				freshParser := NewParser(parser.language)
				freshParser.SetAdmissionCandidateRoute(true)
				fresh, err := freshParser.Parse(edited)
				if err != nil {
					t.Fatal(err)
				}
				freshRuntime := fresh.ParseRuntime()
				freshCompact := fresh.compactMaterialized
				fresh.Release()
				if !freshCompact {
					t.Fatal("fresh comparison tree is not compact")
				}
				if runtime.TokensConsumed == 0 || runtime.TokensConsumed >= freshRuntime.TokensConsumed {
					t.Fatalf("compact reuse did not skip tokens: incremental=%d fresh=%d", runtime.TokensConsumed, freshRuntime.TokensConsumed)
				}
				if runtime.CompactReductions == 0 || runtime.CompactReductions >= freshRuntime.CompactReductions {
					t.Fatalf("compact reuse did not skip reductions: incremental=%d fresh=%d", runtime.CompactReductions, freshRuntime.CompactReductions)
				}
				if runtime.NodesAllocated <= 0 || runtime.NodesAllocated >= freshRuntime.NodesAllocated {
					t.Fatalf("compact reuse did not avoid node allocation: incremental=%d fresh=%d", runtime.NodesAllocated, freshRuntime.NodesAllocated)
				}
				copyTree := next.Copy()
				rootRange, bRange := next.RootNode().Range(), newB.Range()
				copySource, copyEdit := compactExecutionEdit(edited, 0, 0, "\n")
				copyTree.Edit(copyEdit)
				if next.RootNode().Range() != rootRange || newB.Range() != bRange || len(next.Edits()) != 0 {
					t.Fatal("editing a copy changed the source tree")
				}
				if copyTree.RootNode() == next.RootNode() || copyTree.RootNode().EndByte() != uint32(len(copySource)) {
					t.Fatal("copy did not isolate the tree")
				}
				copyTree.Release()
				source = edited
			}
		})
	}
}

func TestCompactIncrementalExecutionLanguageMismatch(t *testing.T) {
	parser := newAdmissionCandidateGoParser(t)
	parser.SetAdmissionCandidateRoute(true)
	source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	otherLanguage := *parser.language
	other := NewParser(&otherLanguage)
	other.SetAdmissionCandidateRoute(true)
	edited, edit := compactExecutionEdit(source, bytes.IndexByte(source, '1'), bytes.IndexByte(source, '1')+1, "1+3")
	old.Edit(edit)
	next, profile, err := other.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	if profile.ReusedSubtrees != 0 {
		t.Fatalf("mismatched language reused nodes: %+v", profile)
	}
	if next.ParseRuntime().CompactIncrementalReuseRoute {
		t.Fatal("mismatched language entered compact reuse")
	}
	if next.RootNode().HasError() {
		t.Fatal("mismatched language fallback produced an error")
	}
}

func TestCompactIncrementalExecutionSameWidth(t *testing.T) {
	for _, replacement := range []string{"x", "2"} {
		t.Run(replacement, func(t *testing.T) {
			parser := newAdmissionCandidateGoParser(t)
			parser.SetAdmissionCandidateRoute(true)
			source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if !old.compactMaterialized {
				old.Release()
				t.Fatal("initial tree is not compact")
			}
			oldRoot := old.RootNode()
			bStart := uint32(bytes.Index(source, []byte("func b")))
			oldB := compactExecutionNode(oldRoot, parser.language, "function_declaration", bStart)
			if oldB == nil {
				old.Release()
				t.Fatal("initial tree has no function b")
			}
			offset := bytes.IndexByte(source, '1')
			edited, edit := compactExecutionEdit(source, offset, offset+1, replacement)
			old.Edit(edit)
			runner := parser.admissionCandidateRunner.(*parserCoreFreshFullRunner)
			legacyRuns := runner.legacyParseRuns
			routed, fallback := AdmissionCandidateCounters()
			parser.fullParseRetryPassesTaken = 3
			next, profile, err := parser.ParseIncrementalProfiled(edited, old)
			old.Release()
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			if parser.fullParseRetryPassesTaken != 0 {
				t.Fatal("incremental parse retained the previous retry count")
			}
			if r, f := AdmissionCandidateCounters(); r != routed || f != fallback {
				t.Fatal("incremental parse changed full admission counters")
			}
			if got := compactExecutionNode(next.RootNode(), parser.language, "function_declaration", bStart); got != oldB {
				t.Fatal("same-width edit lost the borrowed function b")
			}
			if next.RootNode().HasError() {
				t.Fatal("same-width edit produced an error")
			}
			if replacement == "2" {
				if profile.TokenInvariantDependencyChecks != 1 || profile.ReparseNanos != 0 ||
					profile.NewNodesAllocated != 0 || next.RootNode() != oldRoot || profile.ReusedSubtrees == 0 {
					t.Fatalf("numeric edit lost authenticated whole-tree reuse: %+v", profile)
				}
				return
			}
			if profile.ReparseNanos <= 0 || profile.NewNodesAllocated == 0 || next.RootNode() == oldRoot {
				t.Fatalf("token-kind change bypassed compact reparsing: %+v", profile)
			}
			if changed := compactExecutionNode(next.RootNode(), parser.language, "identifier", uint32(offset)); changed == nil || changed.Text(edited) != "x" {
				t.Fatal("same-width kind change did not produce an identifier")
			}
			if runner.legacyParseRuns != legacyRuns {
				t.Errorf("same-width kind change invoked the legacy parser: calls=%d", runner.legacyParseRuns-legacyRuns)
			}
			if !next.compactMaterialized || !next.ParseRuntime().CompactIncrementalReuseRoute {
				t.Errorf("same-width kind change skipped compact execution: compact=%t route=%t decline=%q", next.compactMaterialized, next.ParseRuntime().CompactIncrementalReuseRoute, next.ParseRuntime().CompactIncrementalFallbackReason)
			}
		})
	}
}

func TestCompactIncrementalExecutionSameWidthCopyLifetime(t *testing.T) {
	parser := newAdmissionCandidateGoParser(t)
	parser.SetAdmissionCandidateRoute(true)
	source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
	original, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Release()
	if !original.compactMaterialized {
		t.Fatal("initial tree is not compact")
	}
	snapshot := original.Copy()
	defer snapshot.Release()
	current := original.Copy()
	defer func() { current.Release() }()
	bStart := uint32(bytes.Index(source, []byte("func b")))
	borrowedB := compactExecutionNode(current.RootNode(), parser.language, "function_declaration", bStart)
	if borrowedB == nil || borrowedB == compactExecutionNode(original.RootNode(), parser.language, "function_declaration", bStart) {
		t.Fatal("copy did not isolate function b")
	}
	offset := bytes.IndexByte(source, '1')
	for _, replacement := range []string{"x", "3", "y"} {
		edited, edit := compactExecutionEdit(source, offset, offset+1, replacement)
		current.Edit(edit)
		compactExecutionEqual(t, original.RootNode(), snapshot.RootNode(), parser.language)
		if len(original.Edits()) != 0 {
			t.Fatal("editing a copy changed the original edit list")
		}
		runner := parser.admissionCandidateRunner.(*parserCoreFreshFullRunner)
		before := runner.legacyParseRuns
		routed, fallback := AdmissionCandidateCounters()
		next, _, err := parser.ParseIncrementalProfiled(edited, current)
		if err != nil {
			t.Fatal(err)
		}
		if next == current {
			t.Fatal("kind change returned the old tree")
		}
		current.Release()
		current = next
		if r, f := AdmissionCandidateCounters(); r != routed || f != fallback {
			t.Fatal("incremental parse changed full admission counters")
		}
		if got := compactExecutionNode(next.RootNode(), parser.language, "function_declaration", bStart); got != borrowedB {
			t.Fatalf("edit to %q lost the borrowed sibling", replacement)
		}
		if next.RootNode().HasError() || borrowedB.Text(edited) != "func b() { _ = 2 }" {
			t.Fatal("released tree damaged its successor")
		}
		compactExecutionEqual(t, original.RootNode(), snapshot.RootNode(), parser.language)
		if runner.legacyParseRuns != before {
			t.Errorf("edit to %q invoked legacy parsing %d times", replacement, runner.legacyParseRuns-before)
		}
		if !next.compactMaterialized || !next.ParseRuntime().CompactIncrementalReuseRoute {
			t.Errorf("edit to %q skipped compact execution: %q", replacement, next.ParseRuntime().CompactIncrementalFallbackReason)
		}
		source = edited
	}
}

func TestCompactIncrementalExecutionSameWidthUnsupported(t *testing.T) {
	for _, test := range []struct {
		name, replacement string
		mismatch          bool
	}{
		{"recovery", "}", false},
		{"language_mismatch", "x", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			parser := newAdmissionCandidateGoParser(t)
			parser.SetAdmissionCandidateRoute(true)
			source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			offset := bytes.IndexByte(source, '1')
			oldLeaf := compactExecutionNode(old.RootNode(), parser.language, "int_literal", uint32(offset))
			if oldLeaf == nil {
				t.Fatal("initial tree has no edited leaf")
			}
			edited, edit := compactExecutionEdit(source, offset, offset+1, test.replacement)
			old.Edit(edit)
			if test.mismatch {
				language := *parser.language
				parser = NewParser(&language)
				parser.SetAdmissionCandidateRoute(true)
			}
			next, profile, err := parser.ParseIncrementalProfiled(edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			if next.ParseRuntime().CompactIncrementalReuseRoute {
				t.Fatal("unsupported edit published compact reuse")
			}
			if compactExecutionContains(next.RootNode(), oldLeaf) {
				t.Fatal("unsupported edit reused the changed leaf")
			}
			if test.mismatch && profile.ReusedSubtrees != 0 {
				t.Fatal("language mismatch reused old nodes")
			}
			if !test.mismatch && !next.RootNode().HasError() {
				t.Fatal("malformed edit produced no recovery error")
			}
			freshParser := NewParser(parser.language)
			freshParser.SetAdmissionCandidateRoute(false)
			fresh, err := freshParser.Parse(edited)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			compactExecutionEqual(t, next.RootNode(), fresh.RootNode(), parser.language)
		})
	}
}

// This test checks recovery fallback against production. It does not certify C parity.
func TestCompactIncrementalExecutionMissingBraceRejectsWrongSplice(t *testing.T) {
	parser := newAdmissionCandidateGoParser(t)
	parser.SetAdmissionCandidateRoute(true)
	source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	oldB := compactExecutionNode(old.RootNode(), parser.language, "function_declaration", uint32(bytes.Index(source, []byte("func b"))))
	if oldB == nil {
		t.Fatal("initial tree has no function b")
	}
	offset := bytes.IndexByte(source, '}')
	edited, edit := compactExecutionEdit(source, offset, offset+1, "")
	old.Edit(edit)
	next, _, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	if compactExecutionContains(next.RootNode(), oldB) {
		t.Fatal("recovery reused function b in the wrong context")
	}
	if next.ParseRuntime().CompactIncrementalReuseRoute {
		t.Fatal("recovery incorrectly accepted compact incremental reuse")
	}
	if !next.RootNode().HasError() {
		t.Fatal("the missing brace produced no recovery error")
	}
	freshParser := NewParser(parser.language)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	compactExecutionEqual(t, next.RootNode(), fresh.RootNode(), parser.language)
}

func TestCompactIncrementalExecutionCancellation(t *testing.T) {
	parser := newAdmissionCandidateGoParser(t)
	parser.SetAdmissionCandidateRoute(true)
	source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	offset := bytes.IndexByte(source, '1')
	edited, edit := compactExecutionEdit(source, offset, offset+1, "1+3")
	old.Edit(edit)
	cancelled := uint32(1)
	parser.SetCancellationFlag(&cancelled)
	next, _, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	if next.ParseStopReason() != ParseStopCancelled || !next.ParseStoppedEarly() {
		t.Fatalf("cancelled incremental parse continued: %s", next.ParseRuntime().Summary())
	}
	if next.ParseRuntime().CompactIncrementalReuseRoute {
		t.Fatal("cancelled parse published compact incremental success")
	}
	parser.SetCancellationFlag(nil)
	retry, _, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer retry.Release()
	if retry.RootNode().HasError() || !retry.ParseRuntime().CompactIncrementalReuseRoute {
		t.Fatal("cancelled attempt damaged the next compact parse")
	}
}

func TestCompactIncrementalExecutionMemoryBudgetReleasesRetention(t *testing.T) {
	requireCandidateRouteBudgetMB(t, 128)
	parser := newAdmissionCandidateGoParser(t)
	parser.SetAdmissionCandidateRoute(true)
	source := []byte("package p\nfunc a() {\n" + strings.Repeat("_ = 1\n", 5000) + "}\nfunc b() { _ = 2 }\n")
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	if !old.compactMaterialized || old.RootNode().HasError() {
		t.Fatal("memory test requires a clean compact source tree")
	}
	oldB := compactExecutionNode(old.RootNode(), parser.language, "function_declaration", uint32(bytes.Index(source, []byte("func b"))))
	if oldB == nil {
		t.Fatal("initial tree has no function b")
	}
	offset := bytes.IndexByte(source, '1')
	edited, edit := compactExecutionEdit(source, offset, offset+1, "1+3")
	old.Edit(edit)
	snapshot := old.Copy()
	defer snapshot.Release()
	before := admissionCandidateCompactFootprintBytes(parser)
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "1")
	ResetParseEnvConfigCacheForTests()
	DrainArenaPools()
	stopped, _, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer stopped.Release()
	runtime := stopped.ParseRuntime()
	if runtime.CompactIncrementalReuseRoute || !strings.Contains(runtime.CompactIncrementalFallbackReason, "memory_budget") {
		t.Fatalf("low budget did not decline compact reuse: route=%t reason=%q", runtime.CompactIncrementalReuseRoute, runtime.CompactIncrementalFallbackReason)
	}
	if got := admissionCandidateCompactStorageBytes(parser); got != 0 {
		t.Fatalf("declined runner retains %d live bytes", got)
	}
	after := admissionCandidateCompactFootprintBytes(parser)
	if after >= before || after > 64<<20 {
		t.Fatalf("declined runner did not release retained capacity: before=%d after=%d", before, after)
	}
	runner, ok := parser.admissionCandidateRunner.(*parserCoreFreshFullRunner)
	if !ok || runner == nil {
		t.Fatal("the cached compact runner disappeared")
	}
	if runner.options.compactIncrementalReuse != nil || runner.scheduler.options.compactIncrementalReuse != nil || runner.scratch.incrementalReuse != nil {
		t.Fatal("declined runner retains borrowed tree references")
	}
	compactExecutionEqual(t, old.RootNode(), snapshot.RootNode(), parser.language)
	if oldB.Text(edited) != "func b() { _ = 2 }" {
		t.Fatal("budget decline damaged the old sibling")
	}
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "128")
	ResetParseEnvConfigCacheForTests()
	retry, _, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer retry.Release()
	if !retry.ParseRuntime().CompactIncrementalReuseRoute || retry.RootNode().HasError() {
		t.Fatalf("budget reset did not restore compact reuse: %q", retry.ParseRuntime().CompactIncrementalFallbackReason)
	}
	if got := compactExecutionNode(retry.RootNode(), parser.language, "function_declaration", uint32(bytes.Index(edited, []byte("func b")))); got != oldB {
		t.Fatal("budget retry lost the old sibling identity")
	}
}

func compactExecutionContains(node, target *Node) bool {
	if node == target {
		return true
	}
	for i := 0; i < node.ChildCount(); i++ {
		if compactExecutionContains(node.Child(i), target) {
			return true
		}
	}
	return false
}

func compactExecutionEqual(t *testing.T, actual, expected *Node, language *Language) {
	t.Helper()
	if actual.Symbol() != expected.Symbol() || actual.Range() != expected.Range() ||
		actual.IsNamed() != expected.IsNamed() || actual.IsExtra() != expected.IsExtra() ||
		actual.IsMissing() != expected.IsMissing() || actual.IsError() != expected.IsError() ||
		actual.HasError() != expected.HasError() || actual.ChildCount() != expected.ChildCount() {
		t.Fatalf("incremental node differs from fresh production: kind=%s range=%+v expected=%s range=%+v", actual.Type(language), actual.Range(), expected.Type(language), expected.Range())
	}
	for i := 0; i < actual.ChildCount(); i++ {
		if actual.FieldNameForChild(i, language) != expected.FieldNameForChild(i, language) {
			t.Fatal("incremental field differs from fresh production")
		}
		compactExecutionEqual(t, actual.Child(i), expected.Child(i), language)
	}
}

func compactExecutionEdit(source []byte, start, end int, replacement string) ([]byte, InputEdit) {
	edited := append([]byte(nil), source[:start]...)
	edited = append(edited, replacement...)
	edited = append(edited, source[end:]...)
	return edited, InputEdit{StartByte: uint32(start), OldEndByte: uint32(end), NewEndByte: uint32(start + len(replacement)), StartPoint: admissionTestPointAtByte(source, start), OldEndPoint: admissionTestPointAtByte(source, end), NewEndPoint: admissionTestPointAtByte(edited, start+len(replacement))}
}

func compactExecutionNode(node *Node, language *Language, kind string, start uint32) *Node {
	if node == nil {
		return nil
	}
	if node.Type(language) == kind && node.StartByte() == start {
		return node
	}
	for i := 0; i < node.ChildCount(); i++ {
		if found := compactExecutionNode(node.Child(i), language, kind, start); found != nil {
			return found
		}
	}
	return nil
}
