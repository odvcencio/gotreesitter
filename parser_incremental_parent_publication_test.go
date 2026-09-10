package gotreesitter

import "testing"

// An abandoned recovery branch must not own a child in the selected result.
func TestIncrementalResultPublishesSelectedParentLinks(t *testing.T) {
	for _, borrowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh_child", true: "borrowed_child"}[borrowed], func(t *testing.T) {
			lang := &Language{Name: "parent_fixture", TokenCount: 1, SymbolNames: []string{"leaf", "root"}, SymbolMetadata: []SymbolMetadata{{Visible: true}, {Visible: true, Named: true}}}
			p := NewParser(lang)
			oldArena := acquireNodeArena(arenaClassFull)
			oldRoot := newParentNodeInArena(oldArena, 1, true, nil, nil, 0)
			old := newTreeWithArenas(oldRoot, []byte("x"), lang, oldArena, nil)
			arena := acquireNodeArena(arenaClassFull)
			childArena := arena
			if borrowed {
				childArena = oldArena
			}
			child := newLeafNodeInArena(childArena, 0, false, 0, 1, Point{}, Point{Column: 1})
			selected := newParentNodeInArena(arena, 1, true, []*Node{child}, nil, 0)
			discarded := newParentNodeInArena(arena, errorSymbol, true, []*Node{child}, nil, 0)
			if child.Parent() != discarded {
				t.Fatal("fixture did not replace the speculative parent")
			}
			reuse := &parseReuseState{}
			if borrowed {
				reuse.markReused(child, arena)
			}
			b := newResultRootBuild(p, []byte("x"), arena, old, reuse, nil)
			next := b.finishTree(selected, b.shouldWireParentLinks, false)
			old.Release()
			defer next.Release()
			if child.Parent() != selected || selected.Parent() != nil || selected.Child(0) != child {
				t.Fatal("selected result retained a discarded parent")
			}
		})
	}
}
