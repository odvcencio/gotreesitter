package gotreesitter

import "testing"

func buildDeepHiddenChain(depth, leaves int) *Node {
	leafParent := &Node{
		symbol:   1,
		children: make([]*Node, leaves),
	}
	for i := 0; i < leaves; i++ {
		leafParent.children[i] = &Node{
			symbol:  2,
			isNamed: true,
		}
	}
	root := leafParent
	for i := 0; i < depth; i++ {
		root = &Node{
			symbol:   1,
			children: []*Node{root},
		}
	}
	return root
}

func TestFlattenHiddenChildrenHandlesDeepInvisibleChains(t *testing.T) {
	symbolMeta := []SymbolMetadata{
		{Name: "EOF", Visible: false},
		{Name: "_hidden", Visible: false},
		{Name: "leaf", Visible: true, Named: true},
	}
	root := buildDeepHiddenChain(600, 512)

	if got, want := countFlattenedHiddenChildren(root, symbolMeta), 512; got != want {
		t.Fatalf("countFlattenedHiddenChildren() = %d, want %d", got, want)
	}

	dst := make([]*Node, 512)
	out := appendFlattenedHiddenChildren(dst, 0, root, symbolMeta)
	if got, want := out, 512; got != want {
		t.Fatalf("appendFlattenedHiddenChildren() out = %d, want %d", got, want)
	}
	for i := 0; i < out; i++ {
		if dst[i] == nil {
			t.Fatalf("flattened child %d is nil", i)
		}
		if got, want := dst[i].symbol, Symbol(2); got != want {
			t.Fatalf("flattened child %d symbol = %d, want %d", i, got, want)
		}
	}
}
