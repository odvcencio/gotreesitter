package gotreesitter

import "testing"

func TestRepairPythonRootNodeCollapsesFlatClassFunctionFragments(t *testing.T) {
	lang := &Language{
		Name:        "python",
		FieldNames:  []string{"", "name", "parameters", "body", "superclasses"},
		SymbolNames: []string{"EOF", "module", "class", "identifier", "argument_list", ":", "_indent", "class_definition", "block", "def", "parameters", "function_definition", "assignment"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "module", Visible: true, Named: true},
			{Name: "class", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "argument_list", Visible: true, Named: true},
			{Name: ":", Visible: true, Named: false},
			{Name: "_indent", Visible: true, Named: false},
			{Name: "class_definition", Visible: true, Named: true},
			{Name: "block", Visible: true, Named: true},
			{Name: "def", Visible: true, Named: false},
			{Name: "parameters", Visible: true, Named: true},
			{Name: "function_definition", Visible: true, Named: true},
			{Name: "assignment", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	classKw := newLeafNodeInArena(arena, 2, false, 0, 5, Point{}, Point{Column: 5})
	className := newLeafNodeInArena(arena, 3, true, 6, 16, Point{Column: 6}, Point{Column: 16})
	argList := newLeafNodeInArena(arena, 4, true, 16, 21, Point{Column: 16}, Point{Column: 21})
	classColon := newLeafNodeInArena(arena, 5, false, 21, 22, Point{Column: 21}, Point{Column: 22})
	classIndent := newLeafNodeInArena(arena, 6, false, 22, 22, Point{Column: 22}, Point{Column: 22})
	defKw := newLeafNodeInArena(arena, 9, false, 27, 30, Point{Row: 1, Column: 4}, Point{Row: 1, Column: 7})
	fnName := newLeafNodeInArena(arena, 3, true, 31, 40, Point{Row: 1, Column: 8}, Point{Row: 1, Column: 17})
	params := newLeafNodeInArena(arena, 10, true, 40, 46, Point{Row: 1, Column: 17}, Point{Row: 1, Column: 23})
	fnColon := newLeafNodeInArena(arena, 5, false, 46, 47, Point{Row: 1, Column: 23}, Point{Row: 1, Column: 24})
	fnIndent := newLeafNodeInArena(arena, 6, false, 47, 47, Point{Row: 1, Column: 24}, Point{Row: 1, Column: 24})
	assign := newLeafNodeInArena(arena, 12, true, 56, 67, Point{Row: 2, Column: 8}, Point{Row: 2, Column: 19})
	root := newParentNodeInArena(arena, 1, true, []*Node{classKw, className, argList, classColon, classIndent, defKw, fnName, params, fnColon, fnIndent, assign}, nil, 0)

	repaired := repairPythonRootNode(root, arena, lang)

	if got, want := len(repaired.children), 1; got != want {
		t.Fatalf("len(repaired.children) = %d, want %d", got, want)
	}
	classDef := repaired.children[0]
	if got, want := classDef.Type(lang), "class_definition"; got != want {
		t.Fatalf("classDef.Type = %q, want %q", got, want)
	}
	if got, want := classDef.FieldNameForChild(1, lang), "name"; got != want {
		t.Fatalf("classDef.FieldNameForChild(1) = %q, want %q", got, want)
	}
	if got, want := classDef.FieldNameForChild(2, lang), "superclasses"; got != want {
		t.Fatalf("classDef.FieldNameForChild(2) = %q, want %q", got, want)
	}
	if got, want := classDef.FieldNameForChild(4, lang), "body"; got != want {
		t.Fatalf("classDef.FieldNameForChild(4) = %q, want %q", got, want)
	}
	classBlock := classDef.children[4]
	if got, want := len(classBlock.children), 1; got != want {
		t.Fatalf("len(classBlock.children) = %d, want %d", got, want)
	}
	fn := classBlock.children[0]
	if got, want := fn.Type(lang), "function_definition"; got != want {
		t.Fatalf("fn.Type = %q, want %q", got, want)
	}
	if got, want := fn.FieldNameForChild(1, lang), "name"; got != want {
		t.Fatalf("fn.FieldNameForChild(1) = %q, want %q", got, want)
	}
	if got, want := fn.FieldNameForChild(2, lang), "parameters"; got != want {
		t.Fatalf("fn.FieldNameForChild(2) = %q, want %q", got, want)
	}
	if got, want := fn.FieldNameForChild(4, lang), "body"; got != want {
		t.Fatalf("fn.FieldNameForChild(4) = %q, want %q", got, want)
	}
}

func TestRepairPythonBlockFlattensSimpleStatements(t *testing.T) {
	lang := &Language{
		Name:        "python",
		SymbolNames: []string{"EOF", "block", "_simple_statements", "expression_statement", "assignment", "_simple_statements_repeat1", ";", "call"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "block", Visible: true, Named: true},
			{Name: "_simple_statements", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "assignment", Visible: true, Named: true},
			{Name: "_simple_statements_repeat1", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
			{Name: "call", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	assign := newLeafNodeInArena(arena, 4, true, 0, 5, Point{}, Point{Column: 5})
	call := newLeafNodeInArena(arena, 7, true, 6, 12, Point{Column: 6}, Point{Column: 12})
	exprAssign := newParentNodeInArena(arena, 3, true, []*Node{assign}, nil, 0)
	exprCall := newParentNodeInArena(arena, 3, true, []*Node{call}, nil, 0)
	semi := newLeafNodeInArena(arena, 6, false, 5, 6, Point{Column: 5}, Point{Column: 6})
	repeat := newParentNodeInArena(arena, 5, true, []*Node{semi, exprCall}, nil, 0)
	simple := newParentNodeInArena(arena, 2, true, []*Node{exprAssign, repeat}, nil, 0)
	block := newParentNodeInArena(arena, 1, true, []*Node{simple}, nil, 0)

	repaired, changed := repairPythonBlock(block, arena, lang, false)

	if !changed {
		t.Fatalf("repairPythonBlock changed = false, want true")
	}
	if got, want := len(repaired.children), 3; got != want {
		t.Fatalf("len(repaired.children) = %d, want %d", got, want)
	}
	if got, want := repaired.children[0].Type(lang), "assignment"; got != want {
		t.Fatalf("child[0].Type = %q, want %q", got, want)
	}
	if got, want := repaired.children[1].Type(lang), ";"; got != want {
		t.Fatalf("child[1].Type = %q, want %q", got, want)
	}
	if got, want := repaired.children[2].Type(lang), "call"; got != want {
		t.Fatalf("child[2].Type = %q, want %q", got, want)
	}
}

func TestNormalizePythonStringContinuationEscapesAddsMissingChildren(t *testing.T) {
	lang := &Language{
		Name:        "python",
		SymbolNames: []string{"EOF", "string_content", "escape_sequence"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "string_content", Visible: true, Named: true},
			{Name: "escape_sequence", Visible: true, Named: true},
		},
	}

	source := []byte("\\n\\\nfoo")
	arena := newNodeArena(arenaClassFull)
	first := newLeafNodeInArena(arena, 2, true, 0, 2, Point{}, Point{Column: 2})
	content := newParentNodeInArena(arena, 1, true, []*Node{first}, nil, 0)
	content.startByte = 0
	content.startPoint = Point{}
	content.endByte = uint32(len(source))
	content.endPoint = Point{Row: 1, Column: 3}

	normalizePythonStringContinuationEscapes(content, source, lang)

	if got, want := len(content.children), 2; got != want {
		t.Fatalf("len(content.children) = %d, want %d", got, want)
	}
	if got, want := content.children[1].startByte, uint32(2); got != want {
		t.Fatalf("content.children[1].startByte = %d, want %d", got, want)
	}
	if got, want := content.children[1].endByte, uint32(4); got != want {
		t.Fatalf("content.children[1].endByte = %d, want %d", got, want)
	}
}
