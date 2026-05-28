package gotreesitter

import "testing"

func TestNormalizeJavaScriptTopLevelExpressionStatementBoundsAlsoSnapsTypeScript(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "program", "expression_statement", "internal_module"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "internal_module", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	module := newLeafNodeInArena(arena, 3, true, 73, 376384, Point{Row: 3}, Point{Row: 490, Column: 1})
	stmt := newParentNodeInArena(arena, 2, true, []*Node{module}, nil, 0)
	stmt.startByte = 0
	stmt.startPoint = Point{}
	stmt.endByte = 376385
	stmt.endPoint = Point{Row: 490, Column: 2}
	root := newParentNodeInArena(arena, 1, true, []*Node{
		newLeafNodeInArena(arena, 0, false, 0, 0, Point{}, Point{}),
		newLeafNodeInArena(arena, 0, false, 0, 0, Point{}, Point{}),
		stmt,
	}, nil, 0)
	root.children[0].symbol = 0
	root.children[1].symbol = 0

	normalizeTypeScriptTreeCompatibility(root, nil, lang)

	if got, want := stmt.StartByte(), uint32(73); got != want {
		t.Fatalf("stmt.StartByte = %d, want %d", got, want)
	}
	if got, want := stmt.EndByte(), uint32(376384); got != want {
		t.Fatalf("stmt.EndByte = %d, want %d", got, want)
	}
}

func TestNormalizeJavaScriptTopLevelExpressionStatementBoundsSnapsFinalRefs(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "program", "expression_statement", "internal_module"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "internal_module", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	eofLeft := newCompactFullLeafInArena(arena, 0, false, 0, 0, Point{}, Point{})
	eofRight := newCompactFullLeafInArena(arena, 0, false, 0, 0, Point{}, Point{})
	module := newCompactFullLeafInArena(arena, 3, true, 73, 376384, Point{Row: 3}, Point{Row: 490, Column: 1})
	stmt := newPendingParentInArena(arena, 2, true, 0, []stackEntry{
		newStackEntryCompactFullLeaf(module.parseState, module),
	}, 0, 376385, Point{}, Point{Row: 490, Column: 2}, false)
	rootParent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{
		newStackEntryCompactFullLeaf(eofLeft.parseState, eofLeft),
		newStackEntryCompactFullLeaf(eofRight.parseState, eofRight),
		newStackEntryPendingParent(stmt.parseState, stmt),
	}, 0, 376385, Point{}, Point{Row: 490, Column: 2}, false)
	rootEntry := newStackEntryPendingParent(rootParent.parseState, rootParent)
	root := materializeStackEntryPendingParent(arena, &rootEntry, pendingParentMaterializeForFinalTree)
	if root == nil || !nodeHasFinalChildRefs(root) {
		t.Fatalf("root did not retain final child refs")
	}

	normalizeTypeScriptTreeCompatibility(root, nil, lang)

	view := resultMutableChildrenForMutation(root)
	entry, ok := view.Entry(2)
	if !ok {
		t.Fatalf("missing final-ref expression_statement child")
	}
	if got, want := stackEntryNodeStartByte(entry), uint32(73); got != want {
		t.Fatalf("stmt start byte = %d, want %d", got, want)
	}
	if got, want := stackEntryNodeEndByte(entry), uint32(376384); got != want {
		t.Fatalf("stmt end byte = %d, want %d", got, want)
	}
	if got, want := stackEntryNodeStartPoint(entry), (Point{Row: 3}); got != want {
		t.Fatalf("stmt start point = %+v, want %+v", got, want)
	}
	if got, want := stackEntryNodeEndPoint(entry), (Point{Row: 490, Column: 1}); got != want {
		t.Fatalf("stmt end point = %+v, want %+v", got, want)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child refs materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("single final child refs materialized children = %d, want 0", got)
	}
}

func TestNormalizeJavaScriptProgramStart(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "function_declaration", "return_statement"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "function_declaration", Visible: true, Named: true},
			{Name: "return_statement", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	fn := newLeafNodeInArena(arena, 2, true, 2, 17, Point{Column: 2}, Point{Column: 17})
	ret := newLeafNodeInArena(arena, 3, true, 20, 29, Point{Row: 1, Column: 2}, Point{Row: 1, Column: 11})
	root := newParentNodeInArena(arena, 1, true, []*Node{fn, ret}, nil, 0)
	root.startByte = 0
	root.startPoint = Point{}
	root.endByte = 29
	root.endPoint = Point{Row: 1, Column: 11}

	normalizeJavaScriptProgramStart(root, lang)

	if got, want := root.StartByte(), uint32(2); got != want {
		t.Fatalf("program.StartByte = %d, want %d", got, want)
	}
	if got, want := root.StartPoint(), (Point{Column: 2}); got != want {
		t.Fatalf("program.StartPoint = %+v, want %+v", got, want)
	}
}

func TestNormalizeJavaScriptProgramEndExtendsTerminatorTail(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "call_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "call_expression", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	call := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	root := newParentNodeInArena(arena, 1, true, []*Node{call}, nil, 0)
	source := []byte("call;\n")

	normalizeJavaScriptProgramEnd(root, source, lang)

	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("program.EndByte = %d, want %d", got, want)
	}
	if got, want := root.EndPoint(), (Point{Row: 1, Column: 0}); got != want {
		t.Fatalf("program.EndPoint = %+v, want %+v", got, want)
	}
}

func TestNormalizeJavaScriptProgramEndExtendsErrorRootTerminatorTail(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "call_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "call_expression", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	root := newLeafNodeInArena(arena, errorSymbol, true, 0, 4, Point{}, Point{Column: 4})
	source := []byte("call;\n")

	normalizeJavaScriptProgramEnd(root, source, lang)

	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("ERROR root EndByte = %d, want %d", got, want)
	}
}

func TestNormalizeJavaScriptTrailingContinueCommentSiblings(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "statement_block", "{", "}", "if_statement", "continue_statement", "continue", "statement_identifier", "comment", "lexical_declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "statement_block", Visible: true, Named: true},
			{Name: "{", Visible: true, Named: false},
			{Name: "}", Visible: true, Named: false},
			{Name: "if_statement", Visible: true, Named: true},
			{Name: "continue_statement", Visible: true, Named: true},
			{Name: "continue", Visible: true, Named: false},
			{Name: "statement_identifier", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "lexical_declaration", Visible: true, Named: true},
		},
	}

	const src = "{ if (x) continue columnLoop // eslint-disable-line no-labels\n const textNode = text }\n"

	arena := newNodeArena(arenaClassFull)
	open := newLeafNodeInArena(arena, 2, false, 0, 1, Point{}, Point{Column: 1})
	ifTok := newLeafNodeInArena(arena, 0, false, 2, 4, Point{Column: 2}, Point{Column: 4})
	cond := newLeafNodeInArena(arena, 0, false, 5, 8, Point{Column: 5}, Point{Column: 8})
	continueTok := newLeafNodeInArena(arena, 6, false, 9, 17, Point{Column: 9}, Point{Column: 17})
	label := newLeafNodeInArena(arena, 7, true, 18, 28, Point{Column: 18}, Point{Column: 28})
	comment := newLeafNodeInArena(arena, 8, true, 29, 61, Point{Column: 29}, Point{Column: 61})
	cont := newParentNodeInArena(arena, 5, true, []*Node{continueTok, label, comment}, nil, 0)
	ifStmt := newParentNodeInArena(arena, 4, true, []*Node{ifTok, cond, cont}, nil, 0)
	ifStmt.startByte = 2
	ifStmt.startPoint = Point{Column: 2}
	ifStmt.endByte = 61
	ifStmt.endPoint = Point{Column: 61}
	lex := newLeafNodeInArena(arena, 9, true, 63, 85, Point{Row: 1, Column: 1}, Point{Row: 1, Column: 23})
	close := newLeafNodeInArena(arena, 3, false, 86, 87, Point{Row: 1, Column: 24}, Point{Row: 1, Column: 25})
	block := newParentNodeInArena(arena, 1, true, []*Node{open, ifStmt, lex, close}, nil, 0)

	normalizeJavaScriptTrailingContinueComments(block, []byte(src), lang)

	if got, want := len(block.children), 5; got != want {
		t.Fatalf("len(block.children) = %d, want %d", got, want)
	}
	if got, want := block.children[2].Type(lang), "comment"; got != want {
		t.Fatalf("block.children[2].Type = %q, want %q", got, want)
	}
	if got, want := cont.ChildCount(), 2; got != want {
		t.Fatalf("continue_statement child count = %d, want %d", got, want)
	}
	if got, want := cont.EndByte(), uint32(28); got != want {
		t.Fatalf("continue_statement endByte = %d, want %d", got, want)
	}
	if got, want := ifStmt.EndByte(), uint32(28); got != want {
		t.Fatalf("if_statement endByte = %d, want %d", got, want)
	}
}

func TestNormalizeJavaScriptTrailingContinueCommentSiblingsDirectContinue(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "statement_block", "{", "}", "continue_statement", "continue", "statement_identifier", "comment", "lexical_declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "statement_block", Visible: true, Named: true},
			{Name: "{", Visible: true, Named: false},
			{Name: "}", Visible: true, Named: false},
			{Name: "continue_statement", Visible: true, Named: true},
			{Name: "continue", Visible: true, Named: false},
			{Name: "statement_identifier", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "lexical_declaration", Visible: true, Named: true},
		},
	}

	const src = "{ continue columnLoop // eslint-disable-line no-labels\n const textNode = text }\n"

	arena := newNodeArena(arenaClassFull)
	open := newLeafNodeInArena(arena, 2, false, 0, 1, Point{}, Point{Column: 1})
	continueTok := newLeafNodeInArena(arena, 5, false, 2, 10, Point{Column: 2}, Point{Column: 10})
	label := newLeafNodeInArena(arena, 6, true, 11, 21, Point{Column: 11}, Point{Column: 21})
	comment := newLeafNodeInArena(arena, 7, true, 22, 54, Point{Column: 22}, Point{Column: 54})
	cont := newParentNodeInArena(arena, 4, true, []*Node{continueTok, label, comment}, nil, 0)
	lex := newLeafNodeInArena(arena, 8, true, 56, 78, Point{Row: 1, Column: 1}, Point{Row: 1, Column: 23})
	close := newLeafNodeInArena(arena, 3, false, 79, 80, Point{Row: 1, Column: 24}, Point{Row: 1, Column: 25})
	block := newParentNodeInArena(arena, 1, true, []*Node{open, cont, lex, close}, nil, 0)

	normalizeJavaScriptTrailingContinueComments(block, []byte(src), lang)

	if got, want := len(block.children), 5; got != want {
		t.Fatalf("len(block.children) = %d, want %d", got, want)
	}
	if got, want := block.children[2].Type(lang), "comment"; got != want {
		t.Fatalf("block.children[2].Type = %q, want %q", got, want)
	}
	if got, want := cont.ChildCount(), 2; got != want {
		t.Fatalf("continue_statement child count = %d, want %d", got, want)
	}
	if got, want := cont.EndByte(), uint32(21); got != want {
		t.Fatalf("continue_statement endByte = %d, want %d", got, want)
	}
}

func TestNormalizeJavaScriptTypeScriptOptionalChainKeepsTokenChild(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "expression_statement", "call_expression", "identifier", "optional_chain", "?.", "arguments"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "optional_chain", Visible: true, Named: true},
			{Name: "?.", Visible: true, Named: false},
			{Name: "arguments", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	ident := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
	chainTok := newLeafNodeInArena(arena, 6, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	chain := newParentNodeInArena(arena, 5, true, []*Node{chainTok}, nil, 0)
	chain.startByte = 1
	chain.startPoint = Point{Column: 1}
	chain.endByte = 3
	chain.endPoint = Point{Column: 3}
	args := newLeafNodeInArena(arena, 7, true, 3, 6, Point{Column: 3}, Point{Column: 6})
	call := newParentNodeInArena(arena, 3, true, []*Node{ident, chain, args}, nil, 0)
	stmt := newParentNodeInArena(arena, 2, true, []*Node{call}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)

	normalizeJavaScriptTypeScriptOptionalChainLeaves(root, []byte("?."), lang)

	if got, want := chain.ChildCount(), 1; got != want {
		t.Fatalf("optional_chain child count = %d, want %d", got, want)
	}
	if got, want := chain.StartByte(), uint32(1); got != want {
		t.Fatalf("optional_chain start = %d, want %d", got, want)
	}
	if got, want := chain.EndByte(), uint32(3); got != want {
		t.Fatalf("optional_chain end = %d, want %d", got, want)
	}

	collapsed := newLeafNodeInArena(arena, 5, true, 0, 2, Point{}, Point{Column: 2})
	root = newParentNodeInArena(arena, 1, true, []*Node{collapsed}, nil, 0)
	normalizeJavaScriptTypeScriptOptionalChainLeaves(root, []byte("?."), lang)
	if got, want := collapsed.ChildCount(), 1; got != want {
		t.Fatalf("collapsed optional_chain child count = %d, want %d", got, want)
	}
	if child := collapsed.Child(0); child == nil || child.Type(lang) != "?." {
		if child == nil {
			t.Fatal("collapsed optional_chain child is nil")
		}
		t.Fatalf("collapsed optional_chain child type = %q, want ?.", child.Type(lang))
	}
}
