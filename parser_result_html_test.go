package gotreesitter

import (
	"bytes"
	"testing"
)

func TestNormalizeHTMLRecoveredNestedCustomTagsWrapsStartTagPrefix(t *testing.T) {
	lang := &Language{
		Name:        "html",
		SymbolNames: []string{"EOF", "ERROR", "document", "start_tag", "element", "end_tag", "</", "tag_name", ">"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "ERROR", Visible: true, Named: true},
			{Name: "document", Visible: true, Named: true},
			{Name: "start_tag", Visible: true, Named: true},
			{Name: "element", Visible: true, Named: true},
			{Name: "end_tag", Visible: true, Named: true},
			{Name: "</", Visible: true, Named: false},
			{Name: "tag_name", Visible: true, Named: true},
			{Name: ">", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	start0 := newLeafNodeInArena(arena, 3, true, 0, 5, Point{}, Point{Column: 5})
	start1 := newLeafNodeInArena(arena, 3, true, 6, 11, Point{Row: 1}, Point{Row: 1, Column: 5})
	wrapped := newParentNodeInArena(arena, 1, true, []*Node{start1}, nil, 0)
	deepStart := newLeafNodeInArena(arena, 3, true, 11, 16, Point{Row: 2}, Point{Row: 2, Column: 5})
	leafElem := newParentNodeInArena(arena, 4, true, []*Node{deepStart}, nil, 0)
	leafElem.endByte = 20
	leafElem.endPoint = Point{Row: 3}
	closeTok := newLeafNodeInArena(arena, 6, false, 21, 23, Point{Row: 4}, Point{Row: 4, Column: 2})
	tagName := newLeafNodeInArena(arena, 7, true, 23, 26, Point{Row: 4, Column: 2}, Point{Row: 4, Column: 5})
	closeAngle := newLeafNodeInArena(arena, 8, false, 26, 27, Point{Row: 4, Column: 5}, Point{Row: 4, Column: 6})
	root := newParentNodeInArena(arena, 1, true, []*Node{start0, wrapped, leafElem, closeTok, tagName, closeAngle}, nil, 0)
	root.endByte = 28
	root.endPoint = Point{Row: 5}
	root.hasError = true

	normalizeHTMLRecoveredNestedCustomTags(root, lang)

	if got, want := root.Type(lang), "document"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	if got, want := len(root.children), 1; got != want {
		t.Fatalf("len(root.children) = %d, want %d", got, want)
	}
	outer := root.children[0]
	if got, want := outer.Type(lang), "element"; got != want {
		t.Fatalf("outer.Type = %q, want %q", got, want)
	}
	if got, want := len(outer.children), 3; got != want {
		t.Fatalf("len(outer.children) = %d, want %d", got, want)
	}
	if got, want := outer.children[2].Type(lang), "end_tag"; got != want {
		t.Fatalf("outer.children[2].Type = %q, want %q", got, want)
	}
	inner := outer.children[1]
	if got, want := inner.Type(lang), "element"; got != want {
		t.Fatalf("inner.Type = %q, want %q", got, want)
	}
	if got, want := inner.endByte, uint32(21); got != want {
		t.Fatalf("inner.endByte = %d, want %d", got, want)
	}
}

func TestNormalizeHTMLRecoveredNestedCustomTagRangesExtendsInnerChain(t *testing.T) {
	lang := &Language{
		Name:        "html",
		SymbolNames: []string{"EOF", "document", "element", "start_tag", "end_tag", "</", "tag_name", ">"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "document", Visible: true, Named: true},
			{Name: "element", Visible: true, Named: true},
			{Name: "start_tag", Visible: true, Named: true},
			{Name: "end_tag", Visible: true, Named: true},
			{Name: "</", Visible: true, Named: false},
			{Name: "tag_name", Visible: true, Named: true},
			{Name: ">", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	start0 := newLeafNodeInArena(arena, 3, true, 0, 5, Point{}, Point{Column: 5})
	start1 := newLeafNodeInArena(arena, 3, true, 6, 11, Point{Row: 1}, Point{Row: 1, Column: 5})
	start2 := newLeafNodeInArena(arena, 3, true, 11, 16, Point{Row: 1, Column: 5}, Point{Row: 1, Column: 10})
	leaf := newParentNodeInArena(arena, 2, true, []*Node{start2}, nil, 0)
	leaf.endByte = 20
	leaf.endPoint = Point{Row: 3}
	inner := newParentNodeInArena(arena, 2, true, []*Node{start1, leaf}, nil, 0)
	inner.endByte = 20
	inner.endPoint = Point{Row: 3}
	closeTok := newLeafNodeInArena(arena, 5, false, 21, 23, Point{Row: 4}, Point{Row: 4, Column: 2})
	tagName := newLeafNodeInArena(arena, 6, true, 23, 26, Point{Row: 4, Column: 2}, Point{Row: 4, Column: 5})
	closeAngle := newLeafNodeInArena(arena, 7, false, 26, 27, Point{Row: 4, Column: 5}, Point{Row: 4, Column: 6})
	endTag := newParentNodeInArena(arena, 4, true, []*Node{closeTok, tagName, closeAngle}, nil, 0)
	outer := newParentNodeInArena(arena, 2, true, []*Node{start0, inner, endTag}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{outer}, nil, 0)

	source := bytes.Repeat([]byte{'x'}, 27)
	source[20] = '\n'
	normalizeHTMLRecoveredNestedCustomTagRanges(root, source, lang)

	if got, want := inner.endByte, uint32(21); got != want {
		t.Fatalf("inner.endByte = %d, want %d", got, want)
	}
	if got, want := leaf.endByte, uint32(21); got != want {
		t.Fatalf("leaf.endByte = %d, want %d", got, want)
	}
}

func TestNormalizeHTMLRecoveredNestedCustomTagsExtendsContinuationRange(t *testing.T) {
	lang := &Language{
		Name:        "html",
		SymbolNames: []string{"EOF", "ERROR", "document", "start_tag", "element", "end_tag", "</", "tag_name", ">"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "ERROR", Visible: true, Named: true},
			{Name: "document", Visible: true, Named: true},
			{Name: "start_tag", Visible: true, Named: true},
			{Name: "element", Visible: true, Named: true},
			{Name: "end_tag", Visible: true, Named: true},
			{Name: "</", Visible: true, Named: false},
			{Name: "tag_name", Visible: true, Named: true},
			{Name: ">", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	start0 := newLeafNodeInArena(arena, 3, true, 0, 5, Point{}, Point{Column: 5})
	start1 := newLeafNodeInArena(arena, 3, true, 6, 11, Point{Row: 1}, Point{Row: 1, Column: 5})
	start2 := newLeafNodeInArena(arena, 3, true, 11, 16, Point{Row: 1, Column: 5}, Point{Row: 1, Column: 10})
	leaf := newParentNodeInArena(arena, 4, true, []*Node{start2}, nil, 0)
	leaf.endByte = 20
	leaf.endPoint = Point{Row: 3}
	continuation := newParentNodeInArena(arena, 4, true, []*Node{start1, leaf}, nil, 0)
	continuation.endByte = 20
	continuation.endPoint = Point{Row: 3}
	closeTok := newLeafNodeInArena(arena, 6, false, 21, 23, Point{Row: 4}, Point{Row: 4, Column: 2})
	tagName := newLeafNodeInArena(arena, 7, true, 23, 26, Point{Row: 4, Column: 2}, Point{Row: 4, Column: 5})
	closeAngle := newLeafNodeInArena(arena, 8, false, 26, 27, Point{Row: 4, Column: 5}, Point{Row: 4, Column: 6})
	root := newParentNodeInArena(arena, 1, true, []*Node{start0, continuation, closeTok, tagName, closeAngle}, nil, 0)
	root.hasError = true

	normalizeHTMLRecoveredNestedCustomTags(root, lang)

	if got, want := root.Type(lang), "document"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	inner := root.children[0].children[1]
	if got, want := inner.endByte, uint32(21); got != want {
		t.Fatalf("inner.endByte = %d, want %d", got, want)
	}
	if got, want := inner.children[1].endByte, uint32(21); got != want {
		t.Fatalf("inner.children[1].endByte = %d, want %d", got, want)
	}
}
