package gotreesitter

import "testing"

func TestReductionRetainsHiddenMissingTokenCost(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	p := testRawShapeParser()
	missing := newLeafNodeInArena(arena, 2, false, 0, 0, Point{}, Point{})
	missing.setMissing(true)
	missing.setHasError(true)
	// The hidden terminal is absent from the public tree, as in C. Its
	// missing-token cost still belongs to the reduced visible parent.
	parent := newParentNodeInArena(arena, 1, true, nil, nil, 0)
	parent.rawShape = p.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, missing)}, 0, 1)
	setReduceNodeDynamicPrecedence(parent, nil, 0, 0, ParseAction{})
	if !parent.HasError() || parent.ChildCount() != 0 {
		t.Fatalf("reduced parent hasError=%t children=%d", parent.HasError(), parent.ChildCount())
	}
	if got := p.cNodeErrorCost(parent); got != 610 {
		t.Fatalf("hidden missing-token cost = %d, want C's 610", got)
	}
	if got := cNodeErrorCostLang(p.language, parent); got != 610 {
		t.Fatalf("language-only hidden missing-token cost = %d, want 610", got)
	}
	if got := cNodeErrorCostLangWithScratch(&glrMergeScratch{}, p.language, parent); got != 610 {
		t.Fatalf("merge-scratch hidden missing-token cost = %d, want 610", got)
	}
	if !reconcileStaleHasErrorFlags(parent, 0) || !parent.HasError() {
		t.Fatal("public cleanup cleared a hidden missing-token error")
	}
}

func TestReductionRetainsMultipleHiddenMissingTokens(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	p := testRawShapeParser()
	entries := make([]stackEntry, 2)
	for i := range entries {
		missing := newLeafNodeInArena(arena, 2, false, 0, 0, Point{}, Point{})
		missing.setMissing(true)
		missing.setHasError(true)
		entries[i] = newStackEntryNode(0, missing)
	}
	hidden := newParentNodeInArena(arena, 2, false, nil, nil, 0)
	hidden.rawShape = p.captureRawShape(nil, arena, 2, 0, entries, 0, len(entries))
	setReduceNodeDynamicPrecedence(hidden, nil, 0, 0, ParseAction{})
	parent := newParentNodeInArena(arena, 1, true, nil, nil, 0)
	parent.rawShape = p.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, hidden)}, 0, 1)
	setReduceNodeDynamicPrecedence(parent, nil, 0, 0, ParseAction{})
	if parent.ChildCount() != 0 || !parent.HasError() || p.cNodeErrorCost(parent) != 1220 {
		t.Fatalf("hidden reduction chain children=%d error=%t cost=%d", parent.ChildCount(), parent.HasError(), p.cNodeErrorCost(parent))
	}
}
