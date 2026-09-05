//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"reflect"
	"testing"
)

func TestCompactOwnedRecoverySpliceKeepsFieldsAndErrors(t *testing.T) {
	for _, withExtras := range []bool{false, true} {
		t.Run(map[bool]string{false: "root-only", true: "extras"}[withExtras], func(t *testing.T) {
			arena := acquireNodeArena(arenaClassFull)
			lang := &Language{SymbolCount: 4, SymbolMetadata: []SymbolMetadata{{}, {Visible: true, Named: true}, {Visible: true}, {Visible: true, Named: true}}}
			parser := NewParser(lang)
			parser.rootSymbol, parser.hasRootSymbol = 3, true
			child := newLeafNodeInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
			root := newParentNodeInArenaWithFieldSources(arena, 3, true, []*Node{child}, []FieldID{7}, []uint8{fieldSourceDirect}, 9)
			nodes := []*Node{root}
			var leading, trailing *Node
			if withExtras {
				leading = newLeafNodeInArena(arena, 2, false, 0, 1, Point{}, Point{Column: 1})
				leading.setExtra(true)
				bad := newLeafNodeInArena(arena, 2, false, 2, 3, Point{Column: 2}, Point{Column: 3})
				trailing = newParentNodeInArena(arena, errorSymbol, true, []*Node{bad}, nil, 0)
				trailing.setExtra(true)
				trailing.setHasError(true)
				nodes = []*Node{leading, root, trailing}
			}
			var scratch []*Node
			tree, err := buildCompactOwnedRecoveryAcceptedTree(parser, nodes, []byte("#x+"), arena, &scratch)
			if err != nil {
				arena.Release()
				t.Fatal(err)
			}
			defer tree.Release()
			if tree.RootNode() != root || !tree.resultCompatibilityApplied || compactRecoverEOFTreeMarked(tree) {
				t.Fatal("native acceptance changed the grammar root or requested compatibility")
			}
			wantChildren, wantFields := []*Node{child}, []FieldID{7}
			if withExtras {
				wantChildren, wantFields = []*Node{leading, child, trailing}, []FieldID{0, 7, 0}
				if root.StartByte() != 0 || root.EndByte() != 3 || !root.HasError() || child.HasError() {
					t.Fatalf("wrong root range or error scope: root=%d..%d error=%t child=%t", root.StartByte(), root.EndByte(), root.HasError(), child.HasError())
				}
			}
			if root.ChildCount() != len(wantChildren) || !reflect.DeepEqual(root.fieldIDs(), wantFields) {
				t.Fatalf("children=%d fields=%v", root.ChildCount(), root.fieldIDs())
			}
			for index, child := range wantChildren {
				if root.Child(index) != child || child.Parent() != root {
					t.Fatalf("child %d lost order or parent", index)
				}
			}
			if root.fieldSources()[len(wantFields)/2] != fieldSourceDirect {
				t.Fatal("splice changed field provenance")
			}
		})
	}
}

func TestCompactOwnedRecoveryZeroWidthExtrasKeepStackOrder(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	lang := &Language{SymbolCount: 4, SymbolMetadata: []SymbolMetadata{{}, {Visible: true}, {Visible: true}, {Visible: true, Named: true}}}
	parser := NewParser(lang)
	parser.rootSymbol, parser.hasRootSymbol = 3, true
	root := newParentNodeInArena(arena, 3, true, nil, nil, 0)
	extra := newLeafNodeInArena(arena, errorSymbol, true, 0, 0, Point{}, Point{})
	extra.setExtra(true)
	extra.setHasError(true)
	var scratch []*Node
	tree, err := buildCompactOwnedRecoveryAcceptedTree(parser, []*Node{root, extra}, nil, arena, &scratch)
	if err != nil {
		arena.Release()
		t.Fatal(err)
	}
	defer tree.Release()
	if root.ChildCount() != 1 || root.Child(0) != extra || !root.HasError() || extra.Parent() != root {
		t.Fatal("zero-width ERROR vanished from the acceptance splice")
	}
}

func TestCompactOwnedRecoverySpliceRejectsUnprovedRoots(t *testing.T) {
	for _, name := range []string{"wrong-root", "two-roots", "overlap", "foreign-arena", "hidden-composite"} {
		t.Run(name, func(t *testing.T) {
			arena, other := acquireNodeArena(arenaClassFull), acquireNodeArena(arenaClassFull)
			defer arena.Release()
			defer other.Release()
			lang := &Language{SymbolCount: 4, SymbolMetadata: []SymbolMetadata{{}, {Visible: true}, {}, {Visible: true}}}
			parser := NewParser(lang)
			parser.rootSymbol, parser.hasRootSymbol = 3, true
			child := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
			root := newParentNodeInArena(arena, 3, true, []*Node{child}, nil, 0)
			nodes := []*Node{root}
			switch name {
			case "wrong-root":
				root.symbol = 1
			case "two-roots":
				nodes = append(nodes, newLeafNodeInArena(arena, 3, true, 1, 2, Point{Column: 1}, Point{Column: 2}))
			case "overlap":
				nodes = append(nodes, child)
			case "foreign-arena":
				nodes[0] = newLeafNodeInArena(other, 3, true, 0, 1, Point{}, Point{Column: 1})
			case "hidden-composite":
				leaf := newLeafNodeInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
				extra := newParentNodeInArena(arena, 2, false, []*Node{leaf}, nil, 0)
				extra.setExtra(true)
				nodes = append(nodes, extra)
			}
			var scratch []*Node
			if tree, err := buildCompactOwnedRecoveryAcceptedTree(parser, nodes, []byte("ab"), arena, &scratch); err == nil || tree != nil {
				t.Fatal("native acceptance admitted an unproved root")
			}
		})
	}
}
