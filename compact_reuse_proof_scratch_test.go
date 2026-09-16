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
