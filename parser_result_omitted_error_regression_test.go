package gotreesitter

import (
	"sync"
	"testing"
)

func TestOrdinaryOmittedErrorSurvivesCooklangNormalization(t *testing.T) {
	lang := &Language{Name: "cooklang", SymbolNames: []string{"EOF", "recipe", "_wrapper", "step", "metadata", "comment"}, SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "recipe", Visible: true, Named: true}, {Name: "_wrapper", Named: true}, {Name: "step", Visible: true, Named: true}, {Name: "metadata", Visible: true, Named: true}, {Name: "comment", Visible: true, Named: true}}}
	p := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("ab")
	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{wrapper}, nil, 0)
	extra := newParentNodeInArena(arena, errorSymbol, true, []*Node{newLeafNodeInArena(arena, 3, true, 1, 2, Point{Column: 1}, Point{Column: 2})}, nil, 0)
	extra.setExtra(true)
	extra.setHasError(true)
	tree := p.buildResultFromNodes([]*Node{root, extra}, source, arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("nil result")
	}
	defer tree.Release()
	p.reconcileAcceptedResultErrors(tree, tree.root, arena)
	if tree.resultErrorSummary != resultErrorSummaryPresent || !tree.RootNode().HasError() {
		t.Fatalf("omitted ERROR lost during Cooklang normalization or reconciliation: %s", tree.RootNode().SExpr(lang))
	}
}
func TestOrdinaryCleanExtraStaysCleanAfterCooklangNormalization(t *testing.T) {
	lang := &Language{Name: "cooklang", SymbolNames: []string{"EOF", "recipe", "_wrapper", "step", "metadata", "comment"}, SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "recipe", Visible: true, Named: true}, {Name: "_wrapper", Named: true}, {Name: "step", Visible: true, Named: true}, {Name: "metadata", Visible: true, Named: true}, {Name: "comment", Visible: true, Named: true}}}
	p := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{wrapper}, nil, 0)
	extra := newLeafNodeInArena(arena, 5, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	extra.setExtra(true)
	tree := p.buildResultFromNodes([]*Node{root, extra}, []byte("ab"), arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("nil result")
	}
	defer tree.Release()
	tree.root.setHasError(true) // stale flag, with no error-bearing extras
	p.reconcileAcceptedResultErrors(tree, tree.root, arena)
	if tree.resultErrorSummary != resultErrorSummaryClean || tree.RootNode().HasError() {
		t.Fatal("clean extra or stale flag made root erroneous")
	}
}
func TestOrdinaryOmittedErrorSurvivesDeferredCompatibility(t *testing.T) {
	lang := &Language{Name: "typescript", SymbolNames: []string{"EOF", "program", "_wrapper", "word"}, SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "program", Visible: true, Named: true}, {Name: "_wrapper", Named: true}, {Name: "word", Visible: true, Named: true}}}
	p := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{wrapper}, nil, 0)
	extra := newParentNodeInArena(arena, errorSymbol, true, []*Node{newLeafNodeInArena(arena, 3, true, 1, 2, Point{Column: 1}, Point{Column: 2})}, nil, 0)
	extra.setExtra(true)
	extra.setHasError(true)
	tree := p.buildResultFromNodes([]*Node{root, extra}, []byte("ab"), arena, nil, nil, nil)
	if tree == nil {
		t.Fatal("nil tree")
	}
	defer tree.Release()
	if !tree.hasDeferredResultCompatibility() {
		t.Fatal("compatibility ran eagerly")
	}
	if !tree.root.HasError() {
		t.Fatal("unpublished root is clean")
	}
	if !tree.RootNode().HasError() {
		t.Fatal("deferred normalization lost omitted error")
	}
	if tree.resultErrorSummary != resultErrorSummaryPresent {
		t.Fatal("error summary lost")
	}
}
func omittedDeferredRetryFixture(t *testing.T, withMissing bool) *Tree {
	t.Helper()
	lang := &Language{Name: "typescript", SymbolNames: []string{"EOF", "program", "word"}, SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "program", Visible: true, Named: true}, {Name: "word", Visible: true, Named: true}}}
	p := &Parser{language: lang}
	a := acquireNodeArena(arenaClassFull)
	a1 := newLeafNodeInArena(a, 2, true, 0, 1, Point{}, Point{Column: 1})
	a2 := newLeafNodeInArena(a, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	if withMissing {
		a2.endByte = a2.startByte
		a2.endPoint = a2.startPoint
		a2.setMissing(true)
	}
	root := newParentNodeInArena(a, 1, true, []*Node{a1, a2}, nil, 0)
	extra := newParentNodeInArena(a, errorSymbol, true, []*Node{newLeafNodeInArena(a, 2, true, 2, 3, Point{Column: 2}, Point{Column: 3})}, nil, 0)
	extra.setExtra(true)
	extra.setHasError(true)
	tree := p.buildResultFromNodes([]*Node{root, extra}, []byte("abc"), a, nil, nil, nil)
	if tree == nil {
		t.Fatal("nil tree")
	}
	tree.setParseRuntime(ParseRuntime{StopReason: ParseStopAccepted, IncrementalOldTreeReuseRoute: true, MaxStacksSeen: 8})
	return tree
}
func TestOmittedErrorPreAccessRetryAndCopy(t *testing.T) {
	for _, missing := range []bool{false, true} {
		tree := omittedDeferredRetryFixture(t, missing)
		if !tree.hasDeferredResultCompatibility() || !tree.omittedRootError || !tree.root.HasError() || tree.resultErrorSummary != resultErrorSummaryPresent {
			t.Fatal("missing pre-access provenance or summary")
		}
		(&Parser{language: tree.language}).reconcileAcceptedResultErrors(tree, tree.root, tree.arena)
		if !tree.root.HasError() || tree.resultErrorSummary != resultErrorSummaryPresent {
			t.Fatal("accepted-result reconciliation lost omitted error before retry")
		}
		if incrementalAcceptedErrorIsLocal(tree, 3) || !shouldRetryIncrementalParseAsFull(tree, 3, 8) {
			t.Fatal("omitted error incorrectly declared local before access")
		}
		copyTree := tree.Copy()
		if copyTree == nil || !copyTree.omittedRootError || !copyTree.RootNode().HasError() {
			t.Fatal("copy lost omitted error")
		}
		if incrementalAcceptedErrorIsLocal(tree, 3) || incrementalAcceptedErrorIsLocal(copyTree, 3) {
			t.Fatal("omitted error incorrectly declared local after normalization or copy")
		}
		copyTree.Edit(InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}})
		if !copyTree.omittedRootError {
			t.Fatal("edit cleared unproven omitted error")
		}
		copyTree.Release()
		if copyTree.omittedRootError {
			t.Fatal("release did not reset provenance")
		}
		tree.Release()
	}
}
func TestOmittedErrorConcurrentDeferredReaders(t *testing.T) {
	tree := omittedDeferredRetryFixture(t, false)
	defer tree.Release()
	var wg sync.WaitGroup
	errs := make(chan bool, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); n := tree.RootNode(); errs <- n != nil && n.HasError() }()
	}
	wg.Wait()
	close(errs)
	for ok := range errs {
		if !ok {
			t.Fatal("reader lost omitted error")
		}
	}
	if tree.resultErrorSummary != resultErrorSummaryPresent {
		t.Fatal("deferred summary lost")
	}
}
func TestAcceptedErrorLocalityRequiresErrorBearingChild(t *testing.T) {
	a := acquireNodeArena(arenaClassFull)
	defer a.Release()
	x := newLeafNodeInArena(a, 1, true, 0, 1, Point{}, Point{Column: 1})
	y := newLeafNodeInArena(a, 1, true, 1, 1, Point{Column: 1}, Point{Column: 1})
	root := newParentNodeInArena(a, 2, true, []*Node{x, y}, nil, 0)
	root.setHasError(true)
	tree := &Tree{root: root}
	tree.setParseRuntime(ParseRuntime{IncrementalOldTreeReuseRoute: true})
	defer func() {
		if tree.parseRuntime != nil {
			releaseParseRuntime(tree.parseRuntime)
		}
	}()
	if incrementalAcceptedErrorIsLocal(tree, 10) {
		t.Fatal("clean children cannot prove error locality")
	}
	y.setMissing(true)
	if !incrementalAcceptedErrorIsLocal(tree, 10) {
		t.Fatal("zero-width MISSING child should count as local")
	}
	tree.omittedRootError = true
	if incrementalAcceptedErrorIsLocal(tree, 10) {
		t.Fatal("visible missing child must not hide omitted error")
	}
}
func TestForestResultPreservesOmittedErrorProvenance(t *testing.T) {
	lang := &Language{Name: "cooklang", SymbolNames: []string{"EOF", "recipe", "step", "metadata", "comment"}, SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "recipe", Visible: true, Named: true}, {Name: "step", Visible: true, Named: true}, {Name: "metadata", Visible: true, Named: true}, {Name: "comment", Visible: true, Named: true}}}
	p := &Parser{language: lang}
	for _, omitted := range []bool{false, true} {
		a := acquireNodeArena(arenaClassFull)
		leaf := newLeafNodeInArena(a, 2, true, 0, 1, Point{}, Point{Column: 1})
		root := newParentNodeInArena(a, 1, true, []*Node{leaf}, nil, 0)
		tree := newForestResultTree(root, []byte("a"), lang, a, omitted)
		p.normalizeReturnedTreeForParse(tree, []byte("a"))
		p.reconcileAcceptedResultErrors(tree, tree.root, a)
		if tree.omittedRootError != omitted || tree.root.HasError() != omitted {
			t.Errorf("omitted=%v: provenance=%v HasError=%v", omitted, tree.omittedRootError, tree.root.HasError())
		}
		copyTree := tree.Copy()
		if copyTree.omittedRootError != omitted || copyTree.RootNode().HasError() != omitted {
			t.Errorf("omitted=%v: copy lost provenance", omitted)
		}
		copyTree.Edit(InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}})
		if copyTree.omittedRootError != omitted {
			t.Errorf("omitted=%v: edit lost provenance", omitted)
		}
		copyTree.Release()
		tree.Release()
	}
}
