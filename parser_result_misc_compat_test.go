package gotreesitter

import "testing"

func TestNormalizeSvelteTrailingExtraTriviaDropsTrailingToken(t *testing.T) {
	lang := &Language{
		Name:        "svelte",
		SymbolNames: []string{"EOF", "document", "script_element", "style_element", "_tag_value_token1"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "document", Visible: true, Named: true},
			{Name: "script_element", Visible: true, Named: true},
			{Name: "style_element", Visible: true, Named: true},
			{Name: "_tag_value_token1", Visible: false, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	script := newLeafNodeInArena(arena, 2, true, 0, 8, Point{}, Point{Column: 8})
	style := newLeafNodeInArena(arena, 3, true, 9, 17, Point{Row: 1}, Point{Row: 1, Column: 8})
	trailing := newLeafNodeInArena(arena, 4, false, 17, 18, Point{Row: 1, Column: 8}, Point{Row: 2})
	trailing.isExtra = true
	root := newParentNodeInArena(arena, 1, true, []*Node{script, style, trailing}, []FieldID{0, 0, 0}, 0)
	root.fieldSources = []uint8{fieldSourceNone, fieldSourceNone, fieldSourceNone}
	root.endByte = 18
	root.endPoint = Point{Row: 2}

	normalizeSvelteTrailingExtraTrivia(root, []byte("<script>\n<style>\n\n"), lang)

	if got, want := len(root.children), 2; got != want {
		t.Fatalf("len(root.children) = %d, want %d", got, want)
	}
	if got, want := len(root.fieldIDs), 2; got != want {
		t.Fatalf("len(root.fieldIDs) = %d, want %d", got, want)
	}
	if got, want := len(root.fieldSources), 2; got != want {
		t.Fatalf("len(root.fieldSources) = %d, want %d", got, want)
	}
	if got := root.children[1].Type(lang); got != "style_element" {
		t.Fatalf("child[1] = %q, want style_element", got)
	}
}

func TestNormalizeLuaChunkLocalDeclarationFields(t *testing.T) {
	lang := &Language{
		Name:        "lua",
		FieldNames:  []string{"", "local_declaration"},
		SymbolNames: []string{"EOF", "chunk", "function_declaration", "function_call", "variable_declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "chunk", Visible: true, Named: true},
			{Name: "function_declaration", Visible: true, Named: true},
			{Name: "function_call", Visible: true, Named: true},
			{Name: "variable_declaration", Visible: true, Named: true},
		},
	}

	source := []byte("local function foo() end\nprint(foo)\nlocal x = 1\n")
	arena := newNodeArena(arenaClassFull)
	localFn := newLeafNodeInArena(arena, 2, true, 0, 24, Point{}, Point{Row: 0, Column: 24})
	call := newLeafNodeInArena(arena, 3, true, 25, 35, Point{Row: 1}, Point{Row: 1, Column: 10})
	localVar := newLeafNodeInArena(arena, 4, true, 36, 47, Point{Row: 2}, Point{Row: 2, Column: 11})
	root := newParentNodeInArena(arena, 1, true, []*Node{localFn, call, localVar}, nil, 0)

	normalizeLuaChunkLocalDeclarationFields(root, source, lang)

	if got, want := root.FieldNameForChild(0, lang), "local_declaration"; got != want {
		t.Fatalf("root.FieldNameForChild(0) = %q, want %q", got, want)
	}
	if got := root.FieldNameForChild(1, lang); got != "" {
		t.Fatalf("root.FieldNameForChild(1) = %q, want empty", got)
	}
	if got, want := root.FieldNameForChild(2, lang), "local_declaration"; got != want {
		t.Fatalf("root.FieldNameForChild(2) = %q, want %q", got, want)
	}
}

func TestNormalizeHCLConfigFileRootDropsTopLevelWhitespace(t *testing.T) {
	lang := &Language{
		Name:        "hcl",
		SymbolNames: []string{"EOF", "config_file", "comment", "_whitespace", "body"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "config_file", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "_whitespace", Visible: false, Named: false},
			{Name: "body", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	ws1 := newLeafNodeInArena(arena, 3, false, 4, 5, Point{Column: 4}, Point{Row: 1})
	body := newLeafNodeInArena(arena, 4, true, 5, 9, Point{Row: 1}, Point{Row: 1, Column: 4})
	ws2 := newLeafNodeInArena(arena, 3, false, 9, 10, Point{Row: 1, Column: 4}, Point{Row: 2})
	root := newParentNodeInArena(arena, 1, true, []*Node{comment, ws1, body, ws2}, nil, 0)

	normalizeHCLConfigFileRoot(root, lang)

	if got, want := len(root.children), 2; got != want {
		t.Fatalf("len(root.children) = %d, want %d", got, want)
	}
	if got, want := root.children[0].Type(lang), "comment"; got != want {
		t.Fatalf("root.children[0].Type = %q, want %q", got, want)
	}
	if got, want := root.children[1].Type(lang), "body"; got != want {
		t.Fatalf("root.children[1].Type = %q, want %q", got, want)
	}
}

func TestNormalizeHCLConfigFileRootSnapsBodyToStructuralChildren(t *testing.T) {
	lang := &Language{
		Name:        "hcl",
		SymbolNames: []string{"EOF", "config_file", "body", "block", "comment"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "config_file", Visible: true, Named: true},
			{Name: "body", Visible: true, Named: true},
			{Name: "block", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	block := newLeafNodeInArena(arena, 3, true, 10, 20, Point{Row: 1}, Point{Row: 1, Column: 10})
	comment := newLeafNodeInArena(arena, 4, true, 22, 30, Point{Row: 2}, Point{Row: 2, Column: 8})
	body := newParentNodeInArena(arena, 2, true, []*Node{block, comment}, nil, 0)
	body.startByte = 0
	body.startPoint = Point{}
	body.endByte = 40
	body.endPoint = Point{Row: 4}
	root := newParentNodeInArena(arena, 1, true, []*Node{body}, nil, 0)

	normalizeHCLConfigFileRoot(root, lang)

	if got, want := body.startByte, uint32(10); got != want {
		t.Fatalf("body.startByte = %d, want %d", got, want)
	}
	if got, want := body.endByte, uint32(30); got != want {
		t.Fatalf("body.endByte = %d, want %d", got, want)
	}
	if got, want := body.startPoint, (Point{Row: 1}); got != want {
		t.Fatalf("body.startPoint = %#v, want %#v", got, want)
	}
	if got, want := body.endPoint, (Point{Row: 2, Column: 8}); got != want {
		t.Fatalf("body.endPoint = %#v, want %#v", got, want)
	}
}

func TestNormalizeSQLRecoveredSelectRootWrapsFlatSelectClause(t *testing.T) {
	lang := &Language{
		Name: "sql",
		SymbolNames: []string{
			"EOF", "source_file", "SELECT", "_aliasable_expression", "_expression", "type_cast",
			"select_clause_body_repeat1", ",", "comment", "select_statement", "select_clause",
			"select_clause_body", "NULL", "NULL",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "SELECT", Visible: true, Named: false},
			{Name: "_aliasable_expression", Visible: true, Named: true},
			{Name: "_expression", Visible: true, Named: true},
			{Name: "type_cast", Visible: true, Named: true},
			{Name: "select_clause_body_repeat1", Visible: true, Named: true},
			{Name: ",", Visible: true, Named: false},
			{Name: "comment", Visible: true, Named: true},
			{Name: "select_statement", Visible: true, Named: true},
			{Name: "select_clause", Visible: true, Named: true},
			{Name: "select_clause_body", Visible: true, Named: true},
			{Name: "NULL", Visible: true, Named: true},
			{Name: "NULL", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	selectTok := newLeafNodeInArena(arena, 2, false, 0, 6, Point{}, Point{Column: 6})
	firstExprLeaf := newLeafNodeInArena(arena, 5, true, 7, 8, Point{Column: 7}, Point{Column: 8})
	firstExpr := newParentNodeInArena(arena, 4, true, []*Node{firstExprLeaf}, nil, 0)
	firstAlias := newParentNodeInArena(arena, 3, true, []*Node{firstExpr}, nil, 0)
	comma1 := newLeafNodeInArena(arena, 7, false, 8, 9, Point{Column: 8}, Point{Column: 9})
	comment1 := newLeafNodeInArena(arena, 8, true, 10, 20, Point{Column: 10}, Point{Row: 1})
	secondExprLeaf := newLeafNodeInArena(arena, 5, true, 21, 22, Point{Row: 1}, Point{Row: 1, Column: 1})
	secondExpr := newParentNodeInArena(arena, 4, true, []*Node{secondExprLeaf}, nil, 0)
	secondAlias := newParentNodeInArena(arena, 3, true, []*Node{secondExpr}, nil, 0)
	repeat := newParentNodeInArena(arena, 6, true, []*Node{comma1, comment1, secondAlias}, nil, 0)
	comma2 := newLeafNodeInArena(arena, 7, false, 22, 23, Point{Row: 1, Column: 1}, Point{Row: 1, Column: 2})
	comment2 := newLeafNodeInArena(arena, 8, true, 24, 30, Point{Row: 1, Column: 3}, Point{Row: 1, Column: 9})
	root := newParentNodeInArena(arena, 1, true, []*Node{selectTok, firstAlias, repeat, comma2, comment2}, nil, 0)

	normalizeSQLRecoveredSelectRoot(root, lang)

	if got, want := len(root.children), 1; got != want {
		t.Fatalf("len(root.children) = %d, want %d", got, want)
	}
	if got, want := root.children[0].Type(lang), "select_statement"; got != want {
		t.Fatalf("root.children[0].Type = %q, want %q", got, want)
	}
	body := root.children[0].children[0].children[1]
	if got, want := body.Type(lang), "select_clause_body"; got != want {
		t.Fatalf("body.Type = %q, want %q", got, want)
	}
	if got, want := body.children[0].Type(lang), "type_cast"; got != want {
		t.Fatalf("body.children[0].Type = %q, want %q", got, want)
	}
	if got, want := body.children[len(body.children)-1].Type(lang), "NULL"; got != want {
		t.Fatalf("body.children[last].Type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatalf("root.HasError = false, want true")
	}
}

func TestNormalizeErlangSourceFileFormsSetsFormsOnlyAndSnapsBounds(t *testing.T) {
	lang := &Language{
		Name:        "erlang",
		FieldNames:  []string{"", "forms_only"},
		SymbolNames: []string{"EOF", "source_file", "comment", "fun_decl", "function_clause", "."},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "fun_decl", Visible: true, Named: true},
			{Name: "function_clause", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 2, true, 0, 10, Point{}, Point{Column: 10})
	comment.isExtra = true
	clause := newLeafNodeInArena(arena, 4, true, 12, 20, Point{Row: 1}, Point{Row: 1, Column: 8})
	innerComment := newLeafNodeInArena(arena, 2, true, 21, 30, Point{Row: 2}, Point{Row: 2, Column: 9})
	innerComment.isExtra = true
	dot := newLeafNodeInArena(arena, 5, false, 31, 32, Point{Row: 3}, Point{Row: 3, Column: 1})
	funDecl := newParentNodeInArena(arena, 3, true, []*Node{clause, innerComment, dot}, nil, 0)
	funDecl.startByte = 0
	funDecl.startPoint = Point{}
	root := newParentNodeInArena(arena, 1, true, []*Node{comment, funDecl}, nil, 0)

	normalizeErlangSourceFileForms(root, lang)

	if got, want := root.FieldNameForChild(1, lang), "forms_only"; got != want {
		t.Fatalf("root.FieldNameForChild(1) = %q, want %q", got, want)
	}
	if got, want := root.FieldNameForChild(0, lang), ""; got != want {
		t.Fatalf("root.FieldNameForChild(0) = %q, want empty", got)
	}
	if got, want := funDecl.startByte, clause.startByte; got != want {
		t.Fatalf("funDecl.startByte = %d, want %d", got, want)
	}
	if got, want := funDecl.endByte, dot.endByte; got != want {
		t.Fatalf("funDecl.endByte = %d, want %d", got, want)
	}
}

func TestNormalizeErlangSourceFileFormsSkipsExprsMode(t *testing.T) {
	lang := &Language{
		Name:        "erlang",
		FieldNames:  []string{"", "forms_only"},
		SymbolNames: []string{"EOF", "source_file", "comment", "atom"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "atom", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 2, true, 0, 2, Point{}, Point{Column: 2})
	comment.isExtra = true
	expr := newLeafNodeInArena(arena, 3, true, 3, 7, Point{Column: 3}, Point{Column: 7})
	root := newParentNodeInArena(arena, 1, true, []*Node{comment, expr}, nil, 0)

	normalizeErlangSourceFileForms(root, lang)

	if got := root.FieldNameForChild(1, lang); got != "" {
		t.Fatalf("root.FieldNameForChild(1) = %q, want empty", got)
	}
}
