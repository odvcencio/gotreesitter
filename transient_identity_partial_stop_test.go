package gotreesitter

import "testing"

// Exhaust the scratch budget only after real postorder insertions. The outer
// materializer must clear the set on its early-return path.
func TestTransientParentPartialBudgetStopClearsIdentitySet(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	leaf := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	nodes := make([]*Node, 130)
	for i := range nodes {
		nodes[i] = newParentNodeInArena(arena, Symbol(2), true, []*Node{leaf}, nil, 0)
	}
	var scratch parserScratch
	scratch.setBudget(1024)
	parser := &Parser{budgetScratch: &scratch}
	reason := scratch.transientParents.materializeNodeSliceUntil(nodes, arena, nil, parser)
	if reason != ParseStopMemoryBudget {
		t.Fatalf("stop=%v, want memory budget", reason)
	}
	if scratch.transientParents.seen.Capacity() == 0 {
		t.Fatal("budget stopped before any real membership allocation")
	}
	if got := scratch.transientParents.seen.Len(); got != 0 {
		t.Fatalf("partial traversal retained %d identities", got)
	}
	for i, node := range nodes {
		if node == nil || node.Child(0) != leaf {
			t.Fatalf("partial traversal corrupted node %d", i)
		}
	}
}
