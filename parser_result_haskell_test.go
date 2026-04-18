package gotreesitter

import "testing"

func TestNormalizeHaskellZeroWidthTokensDropsEmptySeparators(t *testing.T) {
	lang := &Language{
		Name:        "haskell",
		SymbolNames: []string{"EOF", "haskell", "pragma", "_token1", "haddock", "header"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "haskell", Visible: true, Named: true},
			{Name: "pragma", Visible: true, Named: true},
			{Name: "_token1", Visible: false, Named: false},
			{Name: "haddock", Visible: true, Named: true},
			{Name: "header", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	pragma := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	sep1 := newLeafNodeInArena(arena, 3, false, 4, 4, Point{Column: 4}, Point{Column: 4})
	haddock := newLeafNodeInArena(arena, 4, true, 5, 12, Point{Row: 1}, Point{Row: 1, Column: 7})
	sep2 := newLeafNodeInArena(arena, 3, false, 12, 12, Point{Row: 1, Column: 7}, Point{Row: 1, Column: 7})
	header := newLeafNodeInArena(arena, 5, true, 12, 20, Point{Row: 1, Column: 7}, Point{Row: 2, Column: 8})
	root := newParentNodeInArena(arena, 1, true, []*Node{pragma, sep1, haddock, sep2, header}, nil, 0)

	normalizeHaskellZeroWidthTokens(root, lang)

	if got, want := len(root.children), 3; got != want {
		t.Fatalf("len(root.children) = %d, want %d", got, want)
	}
	if got := root.children[0].Type(lang); got != "pragma" {
		t.Fatalf("child[0] = %q, want pragma", got)
	}
	if got := root.children[1].Type(lang); got != "haddock" {
		t.Fatalf("child[1] = %q, want haddock", got)
	}
	if got := root.children[2].Type(lang); got != "header" {
		t.Fatalf("child[2] = %q, want header", got)
	}
}

func TestNormalizeHaskellRootImportFieldSetsImportsField(t *testing.T) {
	lang := &Language{
		Name:        "haskell",
		SymbolNames: []string{"EOF", "haskell", "pragma", "haddock", "header", "imports", "declarations"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "haskell", Visible: true, Named: true},
			{Name: "pragma", Visible: true, Named: true},
			{Name: "haddock", Visible: true, Named: true},
			{Name: "header", Visible: true, Named: true},
			{Name: "imports", Visible: true, Named: true},
			{Name: "declarations", Visible: true, Named: true},
		},
		FieldNames: []string{"", "imports", "declarations"},
	}

	arena := newNodeArena(arenaClassFull)
	pragma := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	haddock := newLeafNodeInArena(arena, 3, true, 5, 12, Point{Row: 1}, Point{Row: 1, Column: 7})
	header := newLeafNodeInArena(arena, 4, true, 12, 20, Point{Row: 1, Column: 7}, Point{Row: 2, Column: 8})
	imports := newLeafNodeInArena(arena, 5, true, 21, 30, Point{Row: 3}, Point{Row: 3, Column: 9})
	declarations := newLeafNodeInArena(arena, 6, true, 31, 40, Point{Row: 4}, Point{Row: 4, Column: 9})
	root := newParentNodeInArena(arena, 1, true, []*Node{pragma, haddock, header, imports, declarations}, nil, 0)

	normalizeHaskellRootImportField(root, lang)

	if got, want := len(root.fieldIDs), len(root.children); got != want {
		t.Fatalf("len(root.fieldIDs) = %d, want %d", got, want)
	}
	if got, want := root.fieldIDs[3], FieldID(1); got != want {
		t.Fatalf("fieldIDs[3] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(root.fieldSources, 3), uint8(fieldSourceInherited); got != want {
		t.Fatalf("fieldSources[3] = %d, want %d", got, want)
	}
	if got, want := root.fieldIDs[4], FieldID(2); got != want {
		t.Fatalf("fieldIDs[4] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(root.fieldSources, 4), uint8(fieldSourceInherited); got != want {
		t.Fatalf("fieldSources[4] = %d, want %d", got, want)
	}
}

func TestNormalizeHaskellDeclarationsSpanExtendsToTrailingTrivia(t *testing.T) {
	lang := &Language{
		Name:        "haskell",
		SymbolNames: []string{"EOF", "haskell", "declarations"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "haskell", Visible: true, Named: true},
			{Name: "declarations", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	decls := newLeafNodeInArena(arena, 2, true, 10, 14, Point{Row: 1}, Point{Row: 1, Column: 4})
	root := newParentNodeInArena(arena, 1, true, []*Node{decls}, nil, 0)
	root.endByte = 15
	root.endPoint = Point{Row: 2}

	normalizeHaskellDeclarationsSpan(root, []byte("0123456789body\n"), lang)

	if got, want := decls.endByte, uint32(15); got != want {
		t.Fatalf("decls.endByte = %d, want %d", got, want)
	}
	if got, want := decls.endPoint, root.endPoint; got != want {
		t.Fatalf("decls.endPoint = %#v, want %#v", got, want)
	}
}
