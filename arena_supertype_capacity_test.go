package gotreesitter

import "testing"

func TestArenaSupertypeMasksPreserveDistinctSets(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	language := &Language{SymbolNames: make([]string, 9), SymbolMetadata: make([]SymbolMetadata, 9)}
	for i := range language.SymbolNames {
		language.SymbolNames[i] = string(rune('a' + i))
		language.SymbolMetadata[i].Supertype = true
	}
	nodes := make([]*Node, 300)
	for i := range nodes {
		nodes[i] = arena.allocNode()
		nodes[i].ownerArena = arena
		arena.setNodeSupertypeMask(nodes[i], uint32(i+1))
	}
	for i, node := range nodes {
		if got, want := arena.nodeSupertypeMask(node), uint32(i+1); got != want {
			t.Fatalf("mask %d: got=%#x want=%#x", i+1, got, want)
		}
		for bit := 0; bit < 9; bit++ {
			if got, want := node.hasSupertype(language, Symbol(bit)), uint32(i+1)&(1<<bit) != 0; got != want {
				t.Fatalf("mask %d supertype %d: got=%t want=%t", i+1, bit, got, want)
			}
		}
	}
}
