package gotreesitter

import "testing"

func TestNativeUnaryWrapperFlatteningUsesCertifiedSymbolChain(t *testing.T) {
	language := &Language{
		StateCount:  10,
		SymbolNames: []string{"EOF", "parent", "wrapper", "leaf", "other"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
		},
		NativeUnaryWrapperFlattening: []UnaryWrapperFlatteningRule{
			{PublicParent: 1, Wrapper: 2, Leaf: 3, WrapperPreGotoState: 5},
		},
	}
	parser := NewParser(language)
	if len(parser.unaryWrapperFlatteningSet) != 1 {
		t.Fatalf("compiled rules = %d, want 1", len(parser.unaryWrapperFlatteningSet))
	}

	arena := newNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 3, true, 4, 8, Point{Column: 4}, Point{Column: 8})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	wrapper.preGotoState = 5
	children := parser.flattenNativeUnaryWrapperChildren(1, []*Node{wrapper})
	if len(children) != 1 || children[0] != leaf {
		t.Fatalf("flattened children = %v, want the named leaf", children)
	}
}

func TestNativeUnaryWrapperFlatteningFailsClosed(t *testing.T) {
	language := &Language{
		StateCount:  10,
		SymbolNames: []string{"EOF", "parent", "wrapper", "leaf", "other"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
		},
		NativeUnaryWrapperFlattening: []UnaryWrapperFlatteningRule{
			{PublicParent: 1, Wrapper: 2, Leaf: 3, WrapperPreGotoState: 5},
		},
	}
	parser := NewParser(language)
	arena := newNodeArena(arenaClassFull)
	newLeaf := func(symbol Symbol, start, end uint32) *Node {
		return newLeafNodeInArena(
			arena,
			symbol,
			true,
			start,
			end,
			Point{Column: start},
			Point{Column: end},
		)
	}
	tests := []struct {
		name    string
		parent  Symbol
		wrapper *Node
	}{
		{
			name:    "wrong parent",
			parent:  4,
			wrapper: newParentNodeInArena(arena, 2, true, []*Node{newLeaf(3, 0, 1)}, nil, 0),
		},
		{
			name:   "multiple leaves",
			parent: 1,
			wrapper: newParentNodeInArena(
				arena,
				2,
				true,
				[]*Node{newLeaf(3, 0, 1), newLeaf(3, 1, 2)},
				nil,
				0,
			),
		},
		{
			name:    "wrong leaf",
			parent:  1,
			wrapper: newParentNodeInArena(arena, 2, true, []*Node{newLeaf(4, 0, 1)}, nil, 0),
		},
		{
			name:    "span mismatch",
			parent:  1,
			wrapper: newParentNodeInArena(arena, 2, true, []*Node{newLeaf(3, 1, 2)}, nil, 0),
		},
	}
	tests[3].wrapper.startByte = 0
	for index := range tests {
		tests[index].wrapper.preGotoState = 5
	}
	tests = append(tests, struct {
		name    string
		parent  Symbol
		wrapper *Node
	}{
		name:    "wrong parser state",
		parent:  1,
		wrapper: newParentNodeInArena(arena, 2, true, []*Node{newLeaf(3, 0, 1)}, nil, 0),
	})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			children := []*Node{test.wrapper}
			got := parser.flattenNativeUnaryWrapperChildren(test.parent, children)
			if len(got) != 1 || got[0] != test.wrapper {
				t.Fatalf("unsafe chain flattened to %v", got)
			}
		})
	}
}

func TestNativeUnaryWrapperFlatteningRejectsUncertifiedMetadata(t *testing.T) {
	language := &Language{
		StateCount:  10,
		SymbolNames: []string{"EOF", "parent", "wrapper", "leaf"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: false, Named: true},
			{Visible: true, Named: true},
		},
		NativeUnaryWrapperFlattening: []UnaryWrapperFlatteningRule{
			{PublicParent: 1, Wrapper: 2, Leaf: 3, WrapperPreGotoState: 5},
		},
	}
	if got := compileUnaryWrapperFlatteningPolicy(language); got != nil {
		t.Fatalf("invalid metadata compiled rules: %v", got)
	}
}
