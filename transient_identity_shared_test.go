package gotreesitter

import "testing"

// This covers the membership map's identity semantics for shared, non-transient
// arena nodes, including a borrowed subtree from a different arena.
func TestTransientParentMaterializeSharedArenaDAG(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	borrowed := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	defer borrowed.Release()
	leaf := newLeafNodeInArena(borrowed, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	shared := newParentNodeInArena(borrowed, Symbol(2), true, []*Node{leaf}, nil, 0)
	left := newParentNodeInArena(arena, Symbol(3), true, []*Node{shared}, nil, 0)
	right := newParentNodeInArena(arena, Symbol(4), true, []*Node{shared}, nil, 0)
	nodes := []*Node{left, right}
	var scratch transientParentScratch
	if reason := scratch.materializeNodeSliceUntil(nodes, arena, nil, nil); reason != ParseStopNone {
		t.Fatalf("materialize: %v", reason)
	}
	if nodes[0] != left || nodes[1] != right || left.Child(0) != shared || right.Child(0) != shared || shared.Child(0) != leaf {
		t.Fatal("shared borrowed DAG identity changed")
	}
	if scratch.seen.Len() != 0 {
		t.Fatal("completed materialization retained seen nodes")
	}
}

func TestTransientParentMaterializeCancelledClearsPriorMarks(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	leaf := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	node := newParentNodeInArena(arena, Symbol(2), true, []*Node{leaf}, nil, 0)
	var scratch transientParentScratch
	// First traversal allocates/populates the map and its defer must clear it.
	if reason := scratch.materializeNodeSliceUntil([]*Node{node}, arena, nil, nil); reason != ParseStopNone {
		t.Fatalf("initial: %v", reason)
	}
	if scratch.seen.Len() != 0 {
		t.Fatal("initial traversal retained seen nodes")
	}
	// Seed an entry to make cleanup observable even though cancellation is polled
	// before traversal begins. Both identity-map and membership-set store this key.
	if scratch.seen.Capacity() == 0 {
		t.Fatal("initial traversal did not initialize seen map")
	}
	scratch.seen.AddKnownAbsent(node)
	var cancelled uint32 = 1
	parser := &Parser{cancellationFlag: &cancelled}
	if reason := scratch.materializeNodeSliceUntil([]*Node{node}, arena, nil, parser); reason != ParseStopCancelled {
		t.Fatalf("stop=%v", reason)
	}
	if scratch.seen.Len() != 0 {
		t.Fatal("cancelled traversal retained seen nodes")
	}
}
