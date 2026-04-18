package gotreesitter

import "testing"

func TestNormalizeJavaScriptTopLevelObjectLiteralsRewritesObjectLiteral(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "statement_block", "{", "}", "labeled_statement", "statement_identifier", ":", "expression_statement", "arrow_function", "object", "pair", "property_identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "statement_block", Visible: true, Named: true},
			{Name: "{", Visible: true, Named: false},
			{Name: "}", Visible: true, Named: false},
			{Name: "labeled_statement", Visible: true, Named: true},
			{Name: "statement_identifier", Visible: true, Named: true},
			{Name: ":", Visible: true, Named: false},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "arrow_function", Visible: true, Named: true},
			{Name: "object", Visible: true, Named: true},
			{Name: "pair", Visible: true, Named: true},
			{Name: "property_identifier", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	open := newLeafNodeInArena(arena, 3, false, 0, 1, Point{}, Point{Column: 1})
	key := newLeafNodeInArena(arena, 6, true, 2, 8, Point{Column: 2}, Point{Column: 8})
	colon := newLeafNodeInArena(arena, 7, false, 8, 9, Point{Column: 8}, Point{Column: 9})
	value := newLeafNodeInArena(arena, 9, true, 10, 16, Point{Column: 10}, Point{Column: 16})
	valueStmt := newParentNodeInArena(arena, 8, true, []*Node{value}, nil, 0)
	label := newParentNodeInArena(arena, 5, true, []*Node{key, colon, valueStmt}, nil, 0)
	close := newLeafNodeInArena(arena, 4, false, 17, 18, Point{Column: 17}, Point{Column: 18})
	block := newParentNodeInArena(arena, 2, true, []*Node{open, label, close}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{block}, nil, 0)

	normalizeJavaScriptTopLevelObjectLiterals(root, lang)

	if got, want := root.children[0].Type(lang), "expression_statement"; got != want {
		t.Fatalf("root.children[0].Type = %q, want %q", got, want)
	}
	object := root.children[0].children[0]
	if got, want := object.Type(lang), "object"; got != want {
		t.Fatalf("object.Type = %q, want %q", got, want)
	}
	pair := object.children[1]
	if got, want := pair.Type(lang), "pair"; got != want {
		t.Fatalf("pair.Type = %q, want %q", got, want)
	}
	if got, want := pair.children[0].Type(lang), "property_identifier"; got != want {
		t.Fatalf("pair.children[0].Type = %q, want %q", got, want)
	}
	if got, want := pair.children[2].Type(lang), "arrow_function"; got != want {
		t.Fatalf("pair.children[2].Type = %q, want %q", got, want)
	}
}

func TestNormalizeJavaScriptTopLevelExpressionStatementBoundsSnapToChildren(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "expression_statement", "identifier", ";"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	expr := newLeafNodeInArena(arena, 3, true, 10, 20, Point{Row: 1, Column: 2}, Point{Row: 1, Column: 12})
	semi := newLeafNodeInArena(arena, 4, false, 20, 21, Point{Row: 1, Column: 12}, Point{Row: 1, Column: 13})
	stmt := newParentNodeInArena(arena, 2, true, []*Node{expr, semi}, nil, 0)
	stmt.startByte = 0
	stmt.startPoint = Point{}
	stmt.endByte = 22
	stmt.endPoint = Point{Row: 2}
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)

	normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)

	if got, want := stmt.StartByte(), uint32(10); got != want {
		t.Fatalf("stmt.StartByte = %d, want %d", got, want)
	}
	if got, want := stmt.EndByte(), uint32(21); got != want {
		t.Fatalf("stmt.EndByte = %d, want %d", got, want)
	}
	if got, want := stmt.StartPoint(), (Point{Row: 1, Column: 2}); got != want {
		t.Fatalf("stmt.StartPoint = %#v, want %#v", got, want)
	}
	if got, want := stmt.EndPoint(), (Point{Row: 1, Column: 13}); got != want {
		t.Fatalf("stmt.EndPoint = %#v, want %#v", got, want)
	}
}

func TestNormalizeTypeScriptRecoveredNamespaceRootRewrapsNamespaceBody(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "ERROR", "program", "comment", "namespace", "identifier", "{", "enum_declaration", "statement_block", "internal_module", "expression_statement"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "ERROR", Visible: true, Named: true},
			{Name: "program", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "namespace", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "{", Visible: true, Named: false},
			{Name: "enum_declaration", Visible: true, Named: true},
			{Name: "statement_block", Visible: true, Named: true},
			{Name: "internal_module", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	source := []byte("// c\nnamespace ts {\n  enum X\n\n")
	comment := newLeafNodeInArena(arena, 3, true, 0, 4, Point{}, Point{Column: 4})
	namespaceTok := newLeafNodeInArena(arena, 4, false, 5, 14, Point{Row: 1}, Point{Row: 1, Column: 9})
	name := newLeafNodeInArena(arena, 5, true, 15, 17, Point{Row: 1, Column: 10}, Point{Row: 1, Column: 12})
	openBrace := newLeafNodeInArena(arena, 6, false, 18, 19, Point{Row: 1, Column: 13}, Point{Row: 1, Column: 14})
	enumDecl := newLeafNodeInArena(arena, 7, true, 22, 28, Point{Row: 2, Column: 2}, Point{Row: 2, Column: 8})
	wsErr := newLeafNodeInArena(arena, 1, true, 28, 30, Point{Row: 2, Column: 8}, Point{Row: 4})
	wsErr.hasError = true
	root := newParentNodeInArena(arena, 1, true, []*Node{comment, namespaceTok, name, openBrace, enumDecl, wsErr}, nil, 0)
	root.hasError = true

	normalizeTypeScriptRecoveredNamespaceRoot(root, source, lang)

	if got, want := root.Type(lang), "program"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root.ChildCount = %d, want %d", got, want)
	}
	expr := root.Child(1)
	if got, want := expr.Type(lang), "expression_statement"; got != want {
		t.Fatalf("expr.Type = %q, want %q", got, want)
	}
	mod := expr.Child(0)
	if got, want := mod.Type(lang), "internal_module"; got != want {
		t.Fatalf("module.Type = %q, want %q", got, want)
	}
	block := mod.Child(1)
	if got, want := block.Type(lang), "statement_block"; got != want {
		t.Fatalf("block.Type = %q, want %q", got, want)
	}
	if got, want := block.ChildCount(), 1; got != want {
		t.Fatalf("block.ChildCount = %d, want %d", got, want)
	}
	if got, want := block.Child(0).Type(lang), "enum_declaration"; got != want {
		t.Fatalf("block.Child(0).Type = %q, want %q", got, want)
	}
	if root.HasError() {
		t.Fatal("root.HasError = true, want false")
	}
}
