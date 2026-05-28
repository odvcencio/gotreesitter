package gotreesitter

import "testing"

func TestNormalizeCTranslationUnitRootRetagsRecoveredTopLevelChildren(t *testing.T) {
	lang := &Language{
		Name:        "c",
		SymbolNames: []string{"EOF", "ERROR", "translation_unit", "preproc_ifdef", "declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "ERROR", Visible: true, Named: true},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "preproc_ifdef", Visible: true, Named: true},
			{Name: "declaration", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	ifdef := newLeafNodeInArena(arena, 3, true, 0, 7, Point{}, Point{Column: 7})
	decl := newLeafNodeInArena(arena, 4, true, 8, 18, Point{Row: 1}, Point{Row: 1, Column: 10})
	root := newParentNodeInArena(arena, 1, true, []*Node{ifdef, decl}, nil, 0)
	root.setHasError(true)

	normalizeCTranslationUnitRoot(root, lang)

	if got, want := root.Type(lang), "translation_unit"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatalf("root.HasError = false, want true")
	}
}

func TestNormalizeCCollapsedKeywordChildrenRestoresNull(t *testing.T) {
	lang := &Language{
		Name:        "c",
		SymbolNames: []string{"EOF", "translation_unit", "null", "NULL"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "null", Visible: true, Named: true},
			{Name: "NULL", Visible: true, Named: false},
		},
	}
	arena := newNodeArena(arenaClassFull)
	source := []byte("NULL")
	nullNode := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	root := newParentNodeInArena(arena, 1, true, []*Node{nullNode}, nil, 0)

	normalizeCCompatibility(root, source, lang)

	if got, want := nullNode.ChildCount(), 1; got != want {
		t.Fatalf("null child count = %d, want %d", got, want)
	}
	child := nullNode.Child(0)
	if child == nil {
		t.Fatal("null child = nil")
	}
	if got, want := child.Type(lang), "NULL"; got != want {
		t.Fatalf("null child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored NULL child should be anonymous")
	}
}

func TestNormalizeCCollapsedKeywordChildrenUsesFinalRefsSelectively(t *testing.T) {
	lang := &Language{
		Name:        "c",
		SymbolNames: []string{"EOF", "translation_unit", "null", "NULL", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "null", Visible: true, Named: true},
			{Name: "NULL", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
	}
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	source := []byte("NULL name")
	nullNode := newCompactFullLeafInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	nullNode.parseState = 11
	identifier := newCompactFullLeafInArena(arena, 4, true, 5, 9, Point{Column: 5}, Point{Column: 9})
	identifier.parseState = 12
	parent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{
		newStackEntryCompactFullLeaf(nullNode.parseState, nullNode),
		newStackEntryCompactFullLeaf(identifier.parseState, identifier),
	}, 0, 9, Point{}, Point{Column: 9}, false)
	parent.parseState = 13
	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)

	normalizeCCollapsedKeywordChildren(root, source, lang)

	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("single final child materializations = %d, want 1", got)
	}
	if !nodeHasFinalChildRefs(root) {
		t.Fatal("root lost final-child refs")
	}
	restored := root.Child(0)
	if restored == nil {
		t.Fatal("root child 0 = nil")
	}
	if got, want := restored.ChildCount(), 1; got != want {
		t.Fatalf("null child count = %d, want %d", got, want)
	}
	child := restored.Child(0)
	if child == nil {
		t.Fatal("null child = nil")
	}
	if got, want := child.Type(lang), "NULL"; got != want {
		t.Fatalf("null child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored NULL child should be anonymous")
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents after access = %d, want 0", got)
	}
}

func TestNormalizeCCollapsedKeywordChildrenRestoresStorageClassSpecifier(t *testing.T) {
	lang := &Language{
		Name:        "c",
		SymbolNames: []string{"EOF", "translation_unit", "storage_class_specifier", "extern", "inline"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "storage_class_specifier", Visible: true, Named: true},
			{Name: "extern", Visible: true, Named: false},
			{Name: "inline", Visible: true, Named: false},
		},
	}
	arena := newNodeArena(arenaClassFull)
	source := []byte("extern")
	storage := newLeafNodeInArena(arena, 2, true, 0, 6, Point{}, Point{Column: 6})
	root := newParentNodeInArena(arena, 1, true, []*Node{storage}, nil, 0)

	normalizeCCompatibility(root, source, lang)

	if got, want := storage.ChildCount(), 1; got != want {
		t.Fatalf("storage class child count = %d, want %d", got, want)
	}
	child := storage.Child(0)
	if child == nil {
		t.Fatal("storage class child = nil")
	}
	if got, want := child.Type(lang), "extern"; got != want {
		t.Fatalf("storage class child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored extern child should be anonymous")
	}
}

func TestNormalizeCCollapsedKeywordChildrenRestoresTypeQualifier(t *testing.T) {
	lang := &Language{
		Name:        "c",
		SymbolNames: []string{"EOF", "translation_unit", "type_qualifier", "const"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "translation_unit", Visible: true, Named: true},
			{Name: "type_qualifier", Visible: true, Named: true},
			{Name: "const", Visible: true, Named: false},
		},
	}
	arena := newNodeArena(arenaClassFull)
	source := []byte("const")
	qualifier := newLeafNodeInArena(arena, 2, true, 0, 5, Point{}, Point{Column: 5})
	root := newParentNodeInArena(arena, 1, true, []*Node{qualifier}, nil, 0)

	normalizeCCompatibility(root, source, lang)

	if got, want := qualifier.ChildCount(), 1; got != want {
		t.Fatalf("type qualifier child count = %d, want %d", got, want)
	}
	child := qualifier.Child(0)
	if child == nil {
		t.Fatal("type qualifier child = nil")
	}
	if got, want := child.Type(lang), "const"; got != want {
		t.Fatalf("type qualifier child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored const child should be anonymous")
	}
}

func TestNormalizeGoSourceFileRootRetagsRecoveredTopLevelChildren(t *testing.T) {
	lang := &Language{
		Name:        "go",
		SymbolNames: []string{"EOF", "ERROR", "source_file", "package_clause", "function_declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "ERROR", Visible: true, Named: true},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "package_clause", Visible: true, Named: true},
			{Name: "function_declaration", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	pkg := newLeafNodeInArena(arena, 3, true, 0, 12, Point{}, Point{Column: 12})
	fn := newLeafNodeInArena(arena, 4, true, 13, 30, Point{Row: 1}, Point{Row: 1, Column: 17})
	root := newParentNodeInArena(arena, 1, true, []*Node{pkg, fn}, nil, 0)
	root.setHasError(true)

	normalizeGoSourceFileRoot(root, nil, &Parser{language: lang})

	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	if root.HasError() {
		t.Fatalf("root.HasError = true, want false")
	}
}

func TestNormalizeGoStatementListTrailingExtrasStopsBeforeComment(t *testing.T) {
	source := []byte("stmt\n// trailing comment\n")
	arena := newNodeArena(arenaClassFull)
	stmt := newLeafNodeInArena(arena, 3, true, 0, 4, Point{}, Point{Column: 4})
	list := newParentNodeInArena(arena, 2, true, []*Node{stmt}, nil, 0)
	list.endByte = uint32(len(source))
	list.endPoint = advancePointByBytes(Point{}, source)

	normalizeGoStatementListTrailingExtras(list, source, goCompatibilitySymbols{statementList: 2})

	if got, want := list.EndByte(), uint32(5); got != want {
		t.Fatalf("statement_list.EndByte = %d, want %d", got, want)
	}
	if got, want := list.EndPoint(), (Point{Row: 1, Column: 0}); got != want {
		t.Fatalf("statement_list.EndPoint = %+v, want %+v", got, want)
	}
}

func TestNormalizeResultCompatibilityDispatchesUppercaseCobol(t *testing.T) {
	lang := &Language{
		Name:        "COBOL",
		SymbolNames: []string{"EOF", "start", "program_definition", "identification_division"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
		},
	}

	source := []byte("       identification division.\n")
	arena := newNodeArena(arenaClassFull)
	div := newLeafNodeInArena(arena, 3, true, 0, uint32(len(source)-1), Point{}, Point{Column: uint32(len(source) - 1)})
	def := newParentNodeInArena(arena, 2, true, []*Node{div}, nil, 0)
	def.startByte = 0
	def.endByte = uint32(len(source) - 1)
	root := newParentNodeInArena(arena, 1, true, []*Node{def}, nil, 0)
	root.startByte = 0
	root.endByte = uint32(len(source))

	normalizeResultCompatibility(root, source, &Parser{language: lang})

	if got, want := root.StartByte(), uint32(7); got != want {
		t.Fatalf("root.StartByte = %d, want %d", got, want)
	}
	if got, want := root.Child(0).StartByte(), uint32(7); got != want {
		t.Fatalf("program_definition.StartByte = %d, want %d", got, want)
	}
}
