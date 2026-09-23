package gotreesitter

import "testing"

func TestCompactTreeIncrementalReuseProofReusesScratch(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	children := make([]*Node, 128)
	for index := range children {
		child := newLeafNodeInArena(arena, 1, true, uint32(index), uint32(index+1), Point{}, Point{})
		child.setCompactMaterialized(true)
		child.setCompactParseStateProof(true)
		children[index] = child
	}
	root := newParentNodeInArena(arena, 2, true, children, nil, 0)
	var scratch []*Node
	assertEmpty := func() {
		t.Helper()
		if len(scratch) != 0 {
			t.Fatalf("scratch length = %d, want 0", len(scratch))
		}
		for index, node := range scratch[:cap(scratch)] {
			if node != nil {
				t.Fatalf("scratch retains a node at %d", index)
			}
		}
	}
	if !compactTreeIncrementalReuseProven(root, &scratch) {
		t.Fatal("proved tree was rejected")
	}
	assertEmpty()
	if allocations := testing.AllocsPerRun(100, func() {
		if !compactTreeIncrementalReuseProven(root, &scratch) {
			t.Fatal("proved tree was rejected")
		}
	}); allocations != 0 {
		t.Fatalf("reuse proof allocations = %v, want 0", allocations)
	}
	assertEmpty()
	children[0].setCompactParseStateProof(false)
	if compactTreeIncrementalReuseProven(root, &scratch) {
		t.Fatal("unproved child was accepted")
	}
	assertEmpty()
	if compactTreeIncrementalReuseProven(nil, &scratch) || compactTreeIncrementalReuseProven(root, nil) {
		t.Fatal("missing proof inputs were accepted")
	}
}

// TestCompactTreeIncrementalReuseProofExemptsExtraLeaves pins the issue #454
// downstream finding: replayCompactDerivation never authenticates an extra
// (comment) leaf's parseState, because it is injected around the grammar's
// normal shift/goto skeleton and so almost never has a live shift-table
// entry at its own position (parsestate_replay.go, replayShiftTarget).
// Before the isExtra() exemption in compactTreeIncrementalReuseProven, one
// such unproven comment leaf disabled incremental reuse for the WHOLE tree,
// even though every non-extra sibling was fully proven. A tree made entirely
// of proven nodes plus one unproven extra leaf must still pass.
func TestCompactTreeIncrementalReuseProofExemptsExtraLeaves(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	children := make([]*Node, 8)
	for index := range children {
		child := newLeafNodeInArena(arena, 1, true, uint32(index), uint32(index+1), Point{}, Point{})
		child.setCompactMaterialized(true)
		child.setCompactParseStateProof(true)
		children[index] = child
	}
	// One comment leaf: compact-materialized, but its replay parseState proof
	// was never established (the ordinary abstain case for an extra).
	comment := newLeafNodeInArena(arena, 3, true, 8, 9, Point{}, Point{})
	comment.setExtra(true)
	comment.setCompactMaterialized(true)
	children = append(children, comment)
	root := newParentNodeInArena(arena, 2, true, children, nil, 0)

	var scratch []*Node
	if !compactTreeIncrementalReuseProven(root, &scratch) {
		t.Fatal("a fully proven tree with one unproven extra leaf was rejected")
	}
	if len(scratch) != 0 {
		t.Fatalf("scratch length = %d, want 0", len(scratch))
	}

	// A non-extra unproven leaf must still bar the tree: the exemption is
	// specific to extras, not a blanket relaxation.
	children[0].setCompactParseStateProof(false)
	root = newParentNodeInArena(arena, 2, true, children, nil, 0)
	if compactTreeIncrementalReuseProven(root, &scratch) {
		t.Fatal("an unproven ordinary leaf was accepted")
	}
}
