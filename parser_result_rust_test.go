package gotreesitter

import "testing"

func TestNormalizeRustTokenBindingPatterns(t *testing.T) {
	lang := &Language{
		Name: "rust",
		SymbolNames: []string{
			"",
			"source_file",
			"token_tree_pattern",
			"(",
			"identifier",
			"=>",
			"metavariable",
			":",
			")",
			"token_binding_pattern",
			"fragment_specifier",
		},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Named: true},
			{Named: true},
			{Named: false},
			{Named: true},
			{Named: false},
			{Named: true},
			{Named: false},
			{Named: false},
			{Named: true},
			{Named: true},
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("(x => $e:expr)")

	tokenTree := newParentNodeInArena(arena, 2, true, []*Node{
		newLeafNodeInArena(arena, 3, false, 0, 1, Point{}, Point{Column: 1}),
		newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2}),
		newLeafNodeInArena(arena, 5, false, 3, 5, Point{Column: 3}, Point{Column: 5}),
		newLeafNodeInArena(arena, 6, true, 6, 8, Point{Column: 6}, Point{Column: 8}),
		newLeafNodeInArena(arena, 7, false, 8, 9, Point{Column: 8}, Point{Column: 9}),
		newLeafNodeInArena(arena, 4, true, 9, 13, Point{Column: 9}, Point{Column: 13}),
		newLeafNodeInArena(arena, 8, false, 13, 14, Point{Column: 13}, Point{Column: 14}),
	}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{tokenTree}, nil, 0)

	normalizeRustTokenBindingPatterns(root, source, lang)

	pattern := root.Child(0)
	if pattern == nil || pattern.Type(lang) != "token_tree_pattern" {
		t.Fatalf("expected token_tree_pattern child, got %#v", pattern)
	}
	if got, want := pattern.ChildCount(), 5; got != want {
		t.Fatalf("token_tree_pattern child count = %d, want %d", got, want)
	}
	binding := pattern.Child(3)
	if binding == nil || binding.Type(lang) != "token_binding_pattern" {
		t.Fatalf("expected token_binding_pattern, got %s", pattern.SExpr(lang))
	}
	if got, want := binding.ChildCount(), 2; got != want {
		t.Fatalf("token_binding_pattern child count = %d, want %d", got, want)
	}
	if child := binding.Child(1); child == nil || child.Type(lang) != "fragment_specifier" {
		t.Fatalf("expected fragment_specifier child, got %s", binding.SExpr(lang))
	}
}

func TestNormalizeRustRecoveredFunctionItems(t *testing.T) {
	lang := &Language{
		Name: "rust",
		SymbolNames: []string{
			"",
			"source_file",
			"identifier",
			"(",
			"_pattern",
			":",
			"impl",
			"_type",
			"function_item",
			"parameters",
			"parameter",
			"abstract_type",
			"type_parameters",
			"lifetime_parameter",
			"lifetime",
			"generic_type",
			"type_identifier",
			"type_arguments",
			"block",
		},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Named: true},
			{Named: true},
			{Named: false},
			{Named: false},
			{Named: false},
			{Named: false},
			{Named: false},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
		},
	}
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("fn foo(bar: impl for<'a> Baz<Quux<'a>>) {}\n")

	root := newParentNodeInArena(arena, errorSymbol, true, []*Node{
		newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0),
		newLeafNodeInArena(arena, 2, true, 3, 6, advancePointByBytes(Point{}, source[:3]), advancePointByBytes(Point{}, source[:6])),
		newLeafNodeInArena(arena, 3, false, 6, 7, advancePointByBytes(Point{}, source[:6]), advancePointByBytes(Point{}, source[:7])),
		newParentNodeInArena(arena, 4, false, []*Node{
			newLeafNodeInArena(arena, 2, true, 7, 10, advancePointByBytes(Point{}, source[:7]), advancePointByBytes(Point{}, source[:10])),
		}, nil, 0),
		newLeafNodeInArena(arena, 5, false, 10, 11, advancePointByBytes(Point{}, source[:10]), advancePointByBytes(Point{}, source[:11])),
		newLeafNodeInArena(arena, 6, false, 12, 16, advancePointByBytes(Point{}, source[:12]), advancePointByBytes(Point{}, source[:16])),
		newParentNodeInArena(arena, 7, false, nil, nil, 0),
		newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0),
	}, nil, 0)
	root.children[0].startByte = 0
	root.children[0].endByte = 2
	root.children[0].startPoint = Point{}
	root.children[0].endPoint = advancePointByBytes(Point{}, source[:2])
	root.children[6].startByte = 17
	root.children[6].endByte = 33
	root.children[6].startPoint = advancePointByBytes(Point{}, source[:17])
	root.children[6].endPoint = advancePointByBytes(Point{}, source[:33])
	root.children[7].startByte = 34
	root.children[7].endByte = 35
	root.children[7].startPoint = advancePointByBytes(Point{}, source[:34])
	root.children[7].endPoint = advancePointByBytes(Point{}, source[:35])
	root.startByte = 0
	root.endByte = 35
	root.startPoint = Point{}
	root.endPoint = advancePointByBytes(Point{}, source[:35])
	populateParentNode(root, root.children)

	normalizeResultCompatibility(root, source, parser)

	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if root.HasError() {
		t.Fatalf("expected recovered Rust root without error, got %s", root.SExpr(lang))
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	want := "(source_file (function_item (identifier) (parameters (parameter (identifier) (abstract_type (type_parameters (lifetime_parameter (lifetime (identifier)))) (generic_type (type_identifier) (type_arguments (generic_type (type_identifier) (type_arguments (lifetime (identifier))))))))) (block)))"
	if got := root.SExpr(lang); got != want {
		t.Fatalf("recovered Rust SExpr mismatch\nGOT:  %s\nWANT: %s", got, want)
	}
}
