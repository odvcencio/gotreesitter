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
	root.hasError = true

	normalizeCTranslationUnitRoot(root, lang)

	if got, want := root.Type(lang), "translation_unit"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatalf("root.HasError = false, want true")
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
	root.hasError = true

	normalizeGoSourceFileRoot(root, nil, &Parser{language: lang})

	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	if root.HasError() {
		t.Fatalf("root.HasError = true, want false")
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
