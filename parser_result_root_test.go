package gotreesitter

import "testing"

func TestBuildResultFromNodesFlattensInvisibleRootChildren(t *testing.T) {
	lang := &Language{
		Name: "test",
		SymbolNames: []string{
			"",
			"source_file",
			"function_declaration",
			"source_file_repeat1",
		},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: false, Named: false},
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("a\nb\nc\n")

	fn0 := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	fn1 := newLeafNodeInArena(arena, 2, true, 2, 3, Point{Row: 1}, Point{Row: 1, Column: 1})
	fn2 := newLeafNodeInArena(arena, 2, true, 4, 5, Point{Row: 2}, Point{Row: 2, Column: 1})

	repeat1a := newParentNodeInArena(arena, 3, false, []*Node{fn1}, nil, 0)
	repeat1a.endByte = 4
	repeat1a.endPoint = Point{Row: 1, Column: 2}
	repeat1b := newParentNodeInArena(arena, 3, false, []*Node{fn2}, nil, 0)
	repeat1b.endByte = 6
	repeat1b.endPoint = Point{Row: 2, Column: 2}

	root := newParentNodeInArena(arena, 1, true, []*Node{fn0, repeat1a, repeat1b}, nil, 0)
	tree := parser.buildResultFromNodes([]*Node{root}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	gotRoot := tree.RootNode()
	if gotRoot == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := gotRoot.ChildCount(), 3; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	for i := 0; i < gotRoot.ChildCount(); i++ {
		child := gotRoot.Child(i)
		if child == nil {
			t.Fatalf("root child %d is nil", i)
		}
		if got, want := child.Type(lang), "function_declaration"; got != want {
			t.Fatalf("root child %d type = %q, want %q", i, got, want)
		}
		if !child.IsNamed() {
			t.Fatalf("root child %d should be named after flattening", i)
		}
	}
	if got, want := gotRoot.Child(1).EndByte(), uint32(3); got != want {
		t.Fatalf("second child end = %d, want %d", got, want)
	}
	if got, want := gotRoot.Child(2).EndByte(), uint32(5); got != want {
		t.Fatalf("third child end = %d, want %d", got, want)
	}
	if got, want := gotRoot.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesKeepsWrappedSingleChildSpan(t *testing.T) {
	lang := &Language{
		Name:        "expected_root_wrapper",
		SymbolNames: []string{"", "item", "root"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    2,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("x\n")

	item := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	tree := parser.buildResultFromNodes([]*Node{item}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Symbol(), Symbol(2); got != want {
		t.Fatalf("root symbol = %d, want %d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	child := root.Child(0)
	if child == nil {
		t.Fatal("root child is nil")
	}
	if got, want := child.EndByte(), uint32(1); got != want {
		t.Fatalf("wrapped child end = %d, want %d", got, want)
	}
	if got, want := child.Text(tree.Source()), "x"; got != want {
		t.Fatalf("wrapped child text = %q, want %q", got, want)
	}
}
