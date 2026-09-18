package gotreesitter

import "testing"

func TestPythonKeywordRepairPreservesErrorWrapper(t *testing.T) {
	lang := &Language{Name: "python", SymbolNames: []string{"end", "identifier", "pass"}, SymbolMetadata: []SymbolMetadata{{}, {Named: true, Visible: true}, {Visible: true}}}
	for _, repairedChild := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged_child", true: "repaired_child"}[repairedChild], func(t *testing.T) {
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			source := []byte("name")
			symbol := Symbol(1)
			if repairedChild {
				source = []byte("pass")
				symbol = errorSymbol
			}
			child := newLeafNodeInArena(arena, symbol, true, 0, 4, Point{}, Point{Column: 4})
			if repairedChild {
				child.setHasError(true)
			}
			root := newParentNodeInArena(arena, errorSymbol, true, []*Node{child}, nil, 0)
			root.setHasError(true)
			got := repairPythonKeywordErrorNode(root, source, arena, lang)
			if !got.IsError() || !got.HasError() || got.ChildCount() != 1 || got.StartByte() != 0 || got.EndByte() != 4 {
				t.Fatal("keyword repair discarded the enclosing error")
			}
			if repairedChild && got.Child(0).Type(lang) != "pass" {
				t.Fatal("fixture did not exercise the repaired-child path")
			}
			if root.Child(0) != child {
				t.Fatal("keyword repair mutated its input")
			}
		})
	}
}
