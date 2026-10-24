//go:build gts_parsercorephase0

package gotreesitter

import (
	"bytes"
	"os"
	"reflect"
	"sync"
	"testing"
)

func TestNativeNodeViewsPublicAPI(t *testing.T) {
	yaml, err := os.ReadFile("internal/benchfixtures/testdata/real/yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go", "yaml"} {
		t.Run(name, func(t *testing.T) {
			var r *parserCoreFreshFullRunner
			var source []byte
			var err error
			if name == "go" {
				r, err = newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
				source = loadDiagnosticParserCoreCanonicalFixture(t, "query_compile").Source
			} else {
				r, err = newAdmissionCandidateRunner(NewParser(yamlSelectedProjectionLanguageForTest))
				source = yaml
			}
			if err != nil {
				t.Fatal(err)
			}
			r.replayParseStates = true
			r.scratch.nativeProjection = &nativeProjectionWorkspace{}
			reference, err := r.parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer reference.Release()
			r.scratch.nativeProjection = &nativeProjectionWorkspace{compactViews: true}
			tree, err := r.parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			if tree.arena.views == nil {
				t.Fatal("compact public route was not used")
			}
			if tree.arena.used != 1 {
				t.Fatalf("parse constructed %d public views, want one", tree.arena.used)
			}
			if err = r.compact.Reset(); err != nil {
				t.Fatal(err)
			}
			// A producing parser reset must leave replay, geometry, and checkpoints intact.
			compareNodeViewTree(t, reference.RootNode(), tree.RootNode(), r.lang)
			if a, b := reference.RootNode().SExpr(r.lang), tree.RootNode().SExpr(r.lang); a != b {
				t.Fatal("S-expression differs")
			}
			queryText := "(_) @all"
			q, err := NewQuery(queryText, r.lang)
			if err != nil {
				t.Fatal(err)
			}
			if a, b := normalizePublicQueryMatches(q.Execute(reference), source), normalizePublicQueryMatches(q.Execute(tree), source); !reflect.DeepEqual(a, b) {
				t.Fatal("query captures differ")
			}
			if tree.incrementalReuseDisabled != reference.incrementalReuseDisabled {
				t.Fatalf("reuse eligibility changed: %v -> %v", reference.incrementalReuseDisabled, tree.incrementalReuseDisabled)
			}
			materializeNodeViews(tree.root)
			if tree.arena.views != nil {
				t.Fatal("materialization retained backing")
			}
			compareNodeViewTree(t, reference.root, tree.root, r.lang)
			compareNodeViewReceipts(t, reference.root, tree.root)
			clone := tree.Copy()
			defer clone.Release()
			if !bytes.Equal(clone.Source(), source) {
				t.Fatal("copy lost source")
			}
		})
	}
}
func compareNodeViewTree(t *testing.T, want, got *Node, lang *Language) {
	t.Helper()
	if want == nil || got == nil {
		if want != got {
			t.Fatal("nil node mismatch")
		}
		return
	}
	if want.Symbol() != got.Symbol() || want.Range() != got.Range() || want.IsNamed() != got.IsNamed() || want.IsExtra() != got.IsExtra() || want.IsMissing() != got.IsMissing() || want.HasError() != got.HasError() || want.ParseState() != got.ParseState() || want.PreGotoState() != got.PreGotoState() || want.productionID != got.productionID || want.dynamicPrecedence != got.dynamicPrecedence || want.supertypeMask() != got.supertypeMask() || want.ChildCount() != got.ChildCount() || want.NamedChildCount() != got.NamedChildCount() {
		t.Fatalf("node %s %v metadata differs: want=%+v got=%+v", want.Type(lang), want.Range(), want, got)
	}
	for i := 0; i < want.ChildCount(); i++ {
		if want.FieldNameForChild(i, lang) != got.FieldNameForChild(i, lang) {
			t.Fatal("field mismatch")
		}
		child := got.Child(i)
		if child != got.Child(i) || child.Parent() != got {
			t.Fatal("view identity or parent mismatch")
		}
		if i > 0 && child.PrevSibling() != got.Child(i-1) {
			t.Fatal("previous sibling mismatch")
		}
		if i+1 < got.ChildCount() && child.NextSibling() != got.Child(i+1) {
			t.Fatal("next sibling mismatch")
		}
		compareNodeViewTree(t, want.Child(i), child, lang)
	}
}
func compareNodeViewReceipts(t *testing.T, a, b *Node) {
	t.Helper()
	if b.childIndex != a.childIndex {
		t.Fatalf("materialized parent index differs at %d..%d", b.StartByte(), b.EndByte())
	}
	ac, aok := externalScannerCheckpointForNode(a)
	bc, bok := externalScannerCheckpointForNode(b)
	if aok != bok || aok && (!bytes.Equal(ac.start, bc.start) || !bytes.Equal(ac.end, bc.end)) {
		t.Fatalf("checkpoint differs at %d..%d", a.StartByte(), a.EndByte())
	}
	aw, bw := legacyReuseWord(a, false), legacyReuseWord(b, false)
	var av, bv uint32
	if aw != nil {
		av = *aw
	}
	if bw != nil {
		bv = *bw
	}
	if av != bv {
		t.Fatalf("lookahead differs at %d..%d: %x vs %x", a.StartByte(), a.EndByte(), av, bv)
	}
	ad, ak := compactReuseDependencyForNode(a)
	bd, bk := compactReuseDependencyForNode(b)
	if ad != bd || ak != bk {
		t.Fatalf("compact dependency differs at %d..%d: %d,%v vs %d,%v", a.StartByte(), a.EndByte(), ad, ak, bd, bk)
	}
	for i := 0; i < a.ChildCount(); i++ {
		compareNodeViewReceipts(t, a.Child(i), b.Child(i))
	}
}
func TestNativeNodeViewsConcurrentReads(t *testing.T) {
	r, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	r.scratch.nativeProjection = &nativeProjectionWorkspace{compactViews: true}
	source := loadDiagnosticParserCoreCanonicalFixture(t, "query_compile").Source
	tree, err := r.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 10; n++ {
				root := tree.RootNode()
				root.SExpr(r.lang)
				root.Children()
				copy := tree.Copy()
				copy.Release()
				root.NamedChildCount()
				if child := root.Child(0); child != nil {
					child.Parent()
					child.NextSibling()
				}
			}
		}()
	}
	wg.Wait()
}

func TestNativeNodeViewsPublicParseEdit(t *testing.T) {
	t.Setenv("GTS_ADMISSION_CANDIDATE", "1")
	for _, name := range []string{"go", "yaml"} {
		t.Run(name, func(t *testing.T) {
			if name == "yaml" && !compactNodeViewsEnabled {
				t.Skip("experimental publisher requires gts_compact_node_views")
			}
			lang := yamlSelectedProjectionLanguageForTest
			source, err := os.ReadFile("internal/benchfixtures/testdata/real/yaml")
			if name == "go" {
				lang, err = authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
				source = loadDiagnosticParserCoreCanonicalFixture(t, "query_compile").Source
			}
			if err != nil {
				t.Fatal(err)
			}
			identity, _ := lang.parserCoreTableIdentity()
			t.Logf("table_identity=%x", identity)
			p := NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			tree, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			if tree.arena == nil || tree.arena.views == nil {
				t.Fatalf("public parse bypassed views: %s", tree.ParseRuntime().Summary())
			}
			count := tree.arena.used
			if count != 1 {
				t.Fatalf("public Parse eagerly made %d views", count)
			}
			if allocations := testing.AllocsPerRun(50, func() {
				same, e := p.ParseIncremental(source, tree)
				if e != nil {
					panic(e)
				}
				same.Release()
			}); allocations != 0 {
				t.Fatalf("no-edit allocated %g", allocations)
			}
			if tree.arena.used != count {
				t.Fatal("no-edit materialized views")
			}
			clone := tree.Copy()
			defer clone.Release()
			assertReleaseRootTreeEqual(t, tree.root, clone.root, lang)
			for _, editKind := range []string{"replace", "insert", "delete"} {
				t.Run(editKind, func(t *testing.T) {
					old, e := p.Parse(source)
					if e != nil {
						t.Fatal(e)
					}
					defer old.Release()
					index := len(source) - 1
					removed := 1
					replacement := []byte{}
					switch editKind {
					case "insert":
						index = len(source)
						removed = 0
						replacement = []byte("\n")
					case "replace":
						index = bytes.IndexAny(source, "0123456789")
						if index < 0 {
							t.Fatal("fixture has no replacement digit")
						}
						replacement = []byte{'7'}
					}
					point := func(offset int) Point {
						prefix := source[:offset]
						row := bytes.Count(prefix, []byte{'\n'})
						last := bytes.LastIndexByte(prefix, '\n')
						return Point{Row: uint32(row), Column: uint32(offset - last - 1)}
					}
					start, end := point(index), point(index+removed)
					next := append(append(append([]byte{}, source[:index]...), replacement...), source[index+removed:]...)
					newEnd := start
					for _, b := range replacement {
						if b == '\n' {
							newEnd.Row++
							newEnd.Column = 0
						} else {
							newEnd.Column++
						}
					}
					old.Edit(InputEdit{StartByte: uint32(index), OldEndByte: uint32(index + removed), NewEndByte: uint32(index + len(replacement)), StartPoint: start, OldEndPoint: end, NewEndPoint: newEnd})
					updated, e := p.ParseIncremental(next, old)
					if e != nil {
						t.Fatal(e)
					}
					defer updated.Release()
					fresh, e := p.Parse(next)
					if e != nil {
						t.Fatal(e)
					}
					defer fresh.Release()
					assertReleaseRootTreeEqual(t, updated.root, fresh.root, lang)
				})
			}
		})
	}
}

func TestNativeNodeViewsSameWidthReparseWithoutEdit(t *testing.T) {
	lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
	if err != nil {
		t.Fatal(err)
	}
	source := loadDiagnosticParserCoreCanonicalFixture(t, "query_compile").Source
	next := bytes.Clone(source)
	index := bytes.IndexAny(source, "0123456789")
	if index < 0 {
		t.Fatal("fixture has no numeric token")
	}
	next[index] = '7'
	if next[index] == source[index] {
		next[index] = '8'
	}
	for _, profiled := range []bool{false, true} {
		p := NewParser(lang)
		p.SetAdmissionCandidateRoute(true)
		old, err := p.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if old.arena.views == nil {
			t.Fatal("old tree bypassed compact views")
		}
		var updated *Tree
		if profiled {
			updated, _, err = p.ParseIncrementalProfiled(next, old)
		} else {
			updated, err = p.ParseIncremental(next, old)
		}
		if err != nil {
			old.Release()
			t.Fatal(err)
		}
		fresh, err := p.Parse(next)
		if err != nil {
			updated.Release()
			old.Release()
			t.Fatal(err)
		}
		if !incrementalTreesStructurallyEqual(updated, fresh, lang) {
			t.Fatal("same-width reparse differs from fresh output")
		}
		if old.arena.views != nil {
			t.Fatal("incremental publication left the old tree in its read-only representation")
		}
		fresh.Release()
		updated.Release()
		old.Release()
	}
}

func TestNativeNodeViewsFreshVerificationReadsRecords(t *testing.T) {
	r, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	r.replayParseStates = true
	source := loadDiagnosticParserCoreCanonicalFixture(t, "query_compile").Source
	r.scratch.nativeProjection = &nativeProjectionWorkspace{}
	reference, err := r.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer reference.Release()
	r.scratch.nativeProjection = &nativeProjectionWorkspace{compactViews: true}
	tree, err := r.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if !incrementalTreesStructurallyEqual(reference, tree, r.lang) || !incrementalTreesStructurallyEqual(tree, reference, r.lang) {
		t.Fatal("equal indexed witness rejected")
	}
	original := *reference.root
	mutations := []func(*Node){
		func(n *Node) { n.symbol++ }, func(n *Node) { n.startByte++ }, func(n *Node) { n.endByte-- },
		func(n *Node) { n.startPoint.Column++ }, func(n *Node) { n.endPoint.Row++ },
		func(n *Node) { n.setNamed(!n.IsNamed()) }, func(n *Node) { n.setExtra(!n.IsExtra()) },
		func(n *Node) { n.setMissing(!n.IsMissing()) }, func(n *Node) { n.setHasError(!n.HasError()) },
		func(n *Node) { n.children = n.children[:len(n.children)-1] },
	}
	for i, mutate := range mutations {
		mutate(reference.root)
		if incrementalTreesStructurallyEqual(reference, tree, r.lang) {
			t.Fatalf("mutation %d escaped full verification", i)
		}
		*reference.root = original
	}
	var withFields *Node
	stack := []*Node{reference.root}
	for len(stack) > 0 && withFields == nil {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, id := range n.fieldIDs() {
			if id != 0 {
				withFields = n
				break
			}
		}
		stack = append(stack, n.children...)
	}
	if withFields == nil {
		t.Fatal("fixture has no fields")
	}
	for i, id := range withFields.fieldIDs() {
		if id != 0 {
			withFields.fieldIDs()[i] = 0
			if incrementalTreesStructurallyEqual(reference, tree, r.lang) {
				t.Fatal("field mutation escaped verification")
			}
			withFields.fieldIDs()[i] = id
			break
		}
	}
	if tree.arena.used != 1 {
		t.Fatalf("verification constructed %d views", tree.arena.used)
	}
	second, err := r.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
	if !incrementalTreesStructurallyEqual(tree, second, r.lang) || tree.arena.used != 1 || second.arena.used != 1 {
		t.Fatal("indexed/indexed comparison materialized nodes")
	}
}

func TestNativeNodeViewsSparseQueryKeepsOutputCompact(t *testing.T) {
	r, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	r.scratch.nativeProjection = &nativeProjectionWorkspace{compactViews: true}
	source := loadDiagnosticParserCoreCanonicalFixture(t, "query_compile").Source
	tree, err := r.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	q, err := NewQuery("(source_file) @root", r.lang)
	if err != nil {
		t.Fatal(err)
	}
	matches := q.Execute(tree)
	if len(matches) != 1 || len(matches[0].Captures) != 1 || matches[0].Captures[0].Node != tree.root {
		t.Fatal("sparse query capture mismatch")
	}
	if tree.arena.used != 1 {
		t.Fatalf("sparse query constructed %d views", tree.arena.used)
	}
	clone := tree.Copy()
	if tree.arena.used != 1 || tree.arena.views == nil {
		t.Fatal("Copy changed source representation")
	}
	if !incrementalTreesStructurallyEqual(clone, tree, r.lang) {
		t.Fatal("copy differs")
	}
	clone.Release()
	if tree.RootNode().EndByte() != uint32(len(source)) {
		t.Fatal("copy release invalidated source")
	}
}

func TestNativeNodeViewsExtraRootFraming(t *testing.T) {
	for _, name := range []string{"go", "yaml"} {
		t.Run(name, func(t *testing.T) {
			lang := yamlSelectedProjectionLanguageForTest
			source := []byte("# leading\na: 1\n# trailing\n")
			if name == "go" {
				var err error
				lang, err = authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
				if err != nil {
					t.Fatal(err)
				}
				source = []byte("// leading\npackage p\nfunc f(){}\n// trailing\n")
			}
			r, err := newAdmissionCandidateRunner(NewParser(lang))
			if err != nil {
				t.Fatal(err)
			}
			r.scratch.nativeProjection = &nativeProjectionWorkspace{}
			reference, err := r.parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer reference.Release()
			r.scratch.nativeProjection = &nativeProjectionWorkspace{compactViews: true}
			tree, err := r.parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			compareNodeViewTree(t, reference.root, tree.root, lang)
			materializeNodeViews(tree.root)
			compareNodeViewReceipts(t, reference.root, tree.root)
		})
	}
}

func TestNativeNodeViewsLeadingTrivia(t *testing.T) {
	for _, source := range [][]byte{[]byte("  a: 1\n"), []byte(" \n\na: 1\n")} {
		r, err := newAdmissionCandidateRunner(NewParser(yamlSelectedProjectionLanguageForTest))
		if err != nil {
			t.Fatal(err)
		}
		r.scratch.nativeProjection = &nativeProjectionWorkspace{}
		reference, err := r.parse(source)
		if err != nil {
			t.Fatal(err)
		}
		r.scratch.nativeProjection = &nativeProjectionWorkspace{compactViews: true}
		tree, err := r.parse(source)
		if err != nil {
			reference.Release()
			t.Fatal(err)
		}
		compareNodeViewTree(t, reference.root, tree.root, r.lang)
		materializeNodeViews(tree.root)
		compareNodeViewReceipts(t, reference.root, tree.root)
		reference.Release()
		tree.Release()
	}
}

func TestNativeNodeViewsRecycledMetadataDetachesResult(t *testing.T) {
	DrainArenaPools()
	defer DrainArenaPools()
	r, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	r.scratch.nativeProjection = &nativeProjectionWorkspace{compactViews: true}
	source := loadDiagnosticParserCoreCanonicalFixture(t, "query_compile").Source
	tree, err := r.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	want := tree.RootNode().SExpr(r.lang)
	tree.Release()
	views, ok := takeNodeViewBacking().(*nativeNodeViews)
	if !ok {
		t.Fatal("released metadata did not recycle")
	}
	if views.arena != nil || views.snapshot.Store() != nil || len(views.points.lineStarts) != 0 || len(views.checkpoints)+len(views.lookahead)+len(views.dependencies) != 0 {
		t.Fatal("recycled metadata retains a result")
	}
	for i := 0; i < cap(views.pages); i++ {
		if page := views.pages[:cap(views.pages)][i].Load(); page != nil {
			for j := range page {
				if page[j].Load() != nil {
					t.Fatal("recycled page retains a public node")
				}
			}
		}
	}
	recycleNodeViewBacking(views)
	// A differently sized parse must overwrite every reused receipt and page.
	small, err := r.parse([]byte("package p\nfunc f() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	small.Release()
	again, err := r.parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Release()
	if got := again.RootNode().SExpr(r.lang); got != want {
		t.Fatal("recycled metadata changed the next result")
	}
}
