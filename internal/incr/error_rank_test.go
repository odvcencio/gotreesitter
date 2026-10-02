package incr

import "testing"

func TestSubtreeErrorRankPreservesErrorsAndShortCircuit(t *testing.T) {
	type node struct {
		rank     int
		children []*node
	}
	late := &node{}
	root := &node{children: []*node{nil, {rank: 1}, {children: []*node{{rank: 2}, late}}, late}}
	visits := 0
	inspect := func(n *node) (int, []*node) {
		if n == late {
			t.Fatal("rank 2 did not stop the walk")
		}
		visits++
		return n.rank, n.children
	}
	if rank := SubtreeErrorRank(root, inspect); rank != 2 || visits != 4 {
		t.Fatalf("rank=%d visits=%d, want 2/4", rank, visits)
	}
	root.children = []*node{nil, {rank: 1}}
	if rank := SubtreeErrorRank(root, inspect); rank != 1 {
		t.Fatalf("rank=%d, want 1", rank)
	}
	root.children = nil
	if rank := SubtreeErrorRank(root, inspect); rank != 0 {
		t.Fatalf("rank=%d, want 0", rank)
	}
	if rank := SubtreeErrorRank((*node)(nil), inspect); rank != 0 {
		t.Fatalf("nil rank=%d, want 0", rank)
	}
}
