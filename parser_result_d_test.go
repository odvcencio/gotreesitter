package gotreesitter

import "testing"

func TestNormalizeDModuleDefinitionBoundsSnapToStructuralChildren(t *testing.T) {
	lang := &Language{
		Name:        "d",
		SymbolNames: []string{"EOF", "module_def", "module_declaration", "import_declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "module_def", Visible: true, Named: true},
			{Name: "module_declaration", Visible: true, Named: true},
			{Name: "import_declaration", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	moduleDecl := newLeafNodeInArena(arena, 2, true, 133, 178, Point{Row: 4}, Point{Row: 4, Column: 45})
	importDecl := newLeafNodeInArena(arena, 3, true, 179, 9486, Point{Row: 5}, Point{Row: 319, Column: 1})
	moduleDef := newParentNodeInArena(arena, 1, true, []*Node{moduleDecl, importDecl}, nil, 0)
	moduleDef.startByte = 0
	moduleDef.startPoint = Point{}
	moduleDef.endByte = 9487
	moduleDef.endPoint = Point{Row: 319, Column: 2}

	normalizeDModuleDefinitionBounds(moduleDef, lang)

	if got, want := moduleDef.startByte, uint32(133); got != want {
		t.Fatalf("moduleDef.startByte = %d, want %d", got, want)
	}
	if got, want := moduleDef.startPoint, moduleDecl.startPoint; got != want {
		t.Fatalf("moduleDef.startPoint = %#v, want %#v", got, want)
	}
	if got, want := moduleDef.endByte, uint32(9486); got != want {
		t.Fatalf("moduleDef.endByte = %d, want %d", got, want)
	}
	if got, want := moduleDef.endPoint, importDecl.endPoint; got != want {
		t.Fatalf("moduleDef.endPoint = %#v, want %#v", got, want)
	}
}

func TestNormalizeDSourceFileLeadingTriviaSnapsToFirstChild(t *testing.T) {
	lang := &Language{
		Name:        "d",
		SymbolNames: []string{"EOF", "source_file", "variable_declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "variable_declaration", Visible: true, Named: true},
		},
	}

	source := []byte("\nint i = 1;\n")
	arena := newNodeArena(arenaClassFull)
	decl := newLeafNodeInArena(arena, 2, true, 1, 11, Point{Row: 1}, Point{Row: 1, Column: 10})
	root := newParentNodeInArena(arena, 1, true, []*Node{decl}, nil, 0)
	root.startByte = 0
	root.startPoint = Point{}

	normalizeDSourceFileLeadingTrivia(root, source, lang)

	if got, want := root.startByte, uint32(1); got != want {
		t.Fatalf("root.startByte = %d, want %d", got, want)
	}
	if got, want := root.startPoint, decl.startPoint; got != want {
		t.Fatalf("root.startPoint = %#v, want %#v", got, want)
	}
}

func TestNormalizeDVariableStorageClassWrappersWrapsStaticLeaf(t *testing.T) {
	lang := &Language{
		Name:        "d",
		SymbolNames: []string{"EOF", "variable_declaration", "storage_class", "static", "type", "declarator", ";"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "variable_declaration", Visible: true, Named: true},
			{Name: "storage_class", Visible: true, Named: true},
			{Name: "static", Visible: true, Named: false},
			{Name: "type", Visible: true, Named: true},
			{Name: "declarator", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	staticLeaf := newLeafNodeInArena(arena, 3, false, 0, 6, Point{}, Point{Column: 6})
	typ := newLeafNodeInArena(arena, 4, true, 7, 17, Point{Column: 7}, Point{Column: 17})
	decl := newLeafNodeInArena(arena, 5, true, 18, 26, Point{Column: 18}, Point{Column: 26})
	semi := newLeafNodeInArena(arena, 6, false, 26, 27, Point{Column: 26}, Point{Column: 27})
	varDecl := newParentNodeInArena(arena, 1, true, []*Node{staticLeaf, typ, decl, semi}, nil, 0)

	normalizeDVariableStorageClassWrappers(varDecl, lang)

	if got, want := varDecl.children[0].Type(lang), "storage_class"; got != want {
		t.Fatalf("varDecl.children[0].Type = %q, want %q", got, want)
	}
	if got, want := len(varDecl.children[0].children), 1; got != want {
		t.Fatalf("storage_class child count = %d, want %d", got, want)
	}
	if got, want := varDecl.children[0].children[0].Type(lang), "static"; got != want {
		t.Fatalf("wrapped child type = %q, want %q", got, want)
	}
}

func TestNormalizeDCallExpressionTemplateTypesWrapsLeadingTemplateInstance(t *testing.T) {
	lang := &Language{
		Name:        "d",
		SymbolNames: []string{"EOF", "call_expression", "type", "template_instance", "named_arguments"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "type", Visible: true, Named: true},
			{Name: "template_instance", Visible: true, Named: true},
			{Name: "named_arguments", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	templateInstance := newLeafNodeInArena(arena, 3, true, 0, 15, Point{}, Point{Column: 15})
	args := newLeafNodeInArena(arena, 4, true, 15, 17, Point{Column: 15}, Point{Column: 17})
	call := newParentNodeInArena(arena, 1, true, []*Node{templateInstance, args}, nil, 0)

	normalizeDCallExpressionTemplateTypes(call, lang)

	if got, want := call.children[0].Type(lang), "type"; got != want {
		t.Fatalf("call.children[0].Type = %q, want %q", got, want)
	}
	if got, want := len(call.children[0].children), 1; got != want {
		t.Fatalf("type child count = %d, want %d", got, want)
	}
	if got, want := call.children[0].children[0].Type(lang), "template_instance"; got != want {
		t.Fatalf("wrapped child type = %q, want %q", got, want)
	}
}

func TestNormalizeDVariableTypeQualifiersMergesSharedIntoType(t *testing.T) {
	lang := &Language{
		Name:        "d",
		SymbolNames: []string{"EOF", "variable_declaration", "storage_class", "type_ctor", "shared", "type", "identifier", "declarator", ";"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "variable_declaration", Visible: true, Named: true},
			{Name: "storage_class", Visible: true, Named: true},
			{Name: "type_ctor", Visible: true, Named: true},
			{Name: "shared", Visible: true, Named: false},
			{Name: "type", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "declarator", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	sharedLeaf := newLeafNodeInArena(arena, 4, false, 7, 13, Point{Column: 7}, Point{Column: 13})
	typeCtor := newParentNodeInArena(arena, 3, true, []*Node{sharedLeaf}, nil, 0)
	storageClass := newParentNodeInArena(arena, 2, true, []*Node{typeCtor}, nil, 0)
	ident := newLeafNodeInArena(arena, 6, true, 14, 31, Point{Column: 14}, Point{Column: 31})
	typ := newParentNodeInArena(arena, 5, true, []*Node{ident}, nil, 0)
	decl := newLeafNodeInArena(arena, 7, true, 32, 40, Point{Column: 32}, Point{Column: 40})
	semi := newLeafNodeInArena(arena, 8, false, 40, 41, Point{Column: 40}, Point{Column: 41})
	varDecl := newParentNodeInArena(arena, 1, true, []*Node{storageClass, typ, decl, semi}, nil, 0)

	normalizeDVariableTypeQualifiers(varDecl, lang)

	if got, want := len(varDecl.children), 3; got != want {
		t.Fatalf("variable child count = %d, want %d", got, want)
	}
	if got, want := varDecl.children[0].Type(lang), "type"; got != want {
		t.Fatalf("varDecl.children[0].Type = %q, want %q", got, want)
	}
	if got, want := len(varDecl.children[0].children), 2; got != want {
		t.Fatalf("type child count = %d, want %d", got, want)
	}
	if got, want := varDecl.children[0].children[0].Type(lang), "type_ctor"; got != want {
		t.Fatalf("type child[0] = %q, want %q", got, want)
	}
}

func TestNormalizeDCallExpressionPropertyTypesWrapsQualifiedTarget(t *testing.T) {
	lang := &Language{
		Name:        "d",
		SymbolNames: []string{"EOF", "call_expression", "type", "property_expression", "identifier", ".", "named_arguments"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "type", Visible: true, Named: true},
			{Name: "property_expression", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
			{Name: "named_arguments", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	a := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
	dot1 := newLeafNodeInArena(arena, 5, false, 1, 2, Point{Column: 1}, Point{Column: 2})
	b := newLeafNodeInArena(arena, 4, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	left := newParentNodeInArena(arena, 3, true, []*Node{a, dot1, b}, nil, 0)
	dot2 := newLeafNodeInArena(arena, 5, false, 3, 4, Point{Column: 3}, Point{Column: 4})
	c := newLeafNodeInArena(arena, 4, true, 4, 5, Point{Column: 4}, Point{Column: 5})
	prop := newParentNodeInArena(arena, 3, true, []*Node{left, dot2, c}, nil, 0)
	args := newLeafNodeInArena(arena, 6, true, 5, 7, Point{Column: 5}, Point{Column: 7})
	call := newParentNodeInArena(arena, 1, true, []*Node{prop, args}, nil, 0)

	normalizeDCallExpressionPropertyTypes(call, lang)

	if got, want := call.children[0].Type(lang), "type"; got != want {
		t.Fatalf("call.children[0].Type = %q, want %q", got, want)
	}
	if got, want := len(call.children[0].children), 5; got != want {
		t.Fatalf("type child count = %d, want %d", got, want)
	}
}

func TestNormalizeDCallExpressionSimpleTypeCalleesUnwrapsSingleIdentifier(t *testing.T) {
	lang := &Language{
		Name:        "d",
		SymbolNames: []string{"EOF", "call_expression", "type", "identifier", "named_arguments"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "type", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "named_arguments", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	ident := newLeafNodeInArena(arena, 3, true, 0, 10, Point{}, Point{Column: 10})
	typ := newParentNodeInArena(arena, 2, true, []*Node{ident}, nil, 0)
	args := newLeafNodeInArena(arena, 4, true, 10, 12, Point{Column: 10}, Point{Column: 12})
	call := newParentNodeInArena(arena, 1, true, []*Node{typ, args}, nil, 0)

	normalizeDCallExpressionSimpleTypeCallees(call, lang)

	if got, want := call.children[0].Type(lang), "identifier"; got != want {
		t.Fatalf("call.children[0].Type = %q, want %q", got, want)
	}
}
