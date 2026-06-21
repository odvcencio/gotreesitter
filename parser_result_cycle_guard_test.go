package gotreesitter

import "testing"

func TestStripResultTreeSelfCycles(t *testing.T) {
	// direct self-reference: x is its own child
	x := &Node{}
	x.children = []*Node{x}
	stripResultTreeSelfCycles(x)
	for _, c := range x.children {
		if c == x {
			t.Fatal("self-reference not removed")
		}
	}

	// ancestor back-edge: a -> b -> a
	a := &Node{}
	b := &Node{}
	a.children = []*Node{b}
	b.children = []*Node{a}
	stripResultTreeSelfCycles(a)
	for _, c := range b.children {
		if c == a {
			t.Fatal("ancestor back-edge not removed")
		}
	}

	// clean tree must be left untouched
	leaf := &Node{}
	mid := &Node{children: []*Node{leaf}}
	root := &Node{children: []*Node{mid}}
	stripResultTreeSelfCycles(root)
	if len(root.children) != 1 || root.children[0] != mid || len(mid.children) != 1 || mid.children[0] != leaf {
		t.Fatal("clean tree was mutated")
	}
}
