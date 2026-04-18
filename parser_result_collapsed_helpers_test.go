package gotreesitter

import "testing"

func TestNormalizeCollapsedNamedLeafChildrenRestoresCollapsedImplicitTypeVar(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "root", "implicit_type", "var"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "root", Visible: true, Named: true},
			{Name: "implicit_type", Visible: true, Named: true},
			{Name: "var", Visible: true, Named: false},
		},
	}
	arena := newNodeArena(arenaClassFull)
	implicitType := newLeafNodeInArena(arena, 2, true, 4, 7, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 7})
	root := newParentNodeInArena(arena, 1, true, []*Node{implicitType}, nil, 0)

	normalizeCollapsedNamedLeafChildren(root, lang, "implicit_type", "var")

	if got, want := implicitType.ChildCount(), 1; got != want {
		t.Fatalf("implicitType.ChildCount() = %d, want %d", got, want)
	}
	child := implicitType.Child(0)
	if child == nil {
		t.Fatal("implicitType.Child(0) = nil")
	}
	if got, want := child.Type(lang), "var"; got != want {
		t.Fatalf("child.Type() = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored var child should remain anonymous")
	}
	if got, want := child.StartByte(), uint32(4); got != want {
		t.Fatalf("child.StartByte() = %d, want %d", got, want)
	}
	if got, want := child.EndByte(), uint32(7); got != want {
		t.Fatalf("child.EndByte() = %d, want %d", got, want)
	}
}
