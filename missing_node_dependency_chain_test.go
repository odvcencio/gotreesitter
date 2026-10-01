package gotreesitter

import "testing"

// Recovery in Haskell and Elsa can leave many nested error-bearing parents
// before a later edit. Each dependency check must descend each node once.
func TestTreeEditSkipsDeepMissingDependencyBeforeReplacement(t *testing.T) {
	tree, missing, want := newMissingDependencyTree(t)
	defer tree.Release()
	chain := missing
	for range 64 {
		chain = newParentNodeInArena(tree.arena, 2, true, []*Node{chain}, nil, 0)
	}
	if stackEntryEndsBeforeEditDependency(tree.arena, newStackEntryNode(chain.parseState, chain), 5) {
		t.Fatal("missing lookahead was skipped before its dependency end")
	}
	later := newLeafNodeInArena(tree.arena, 3, true, 8, 9, Point{Column: 8}, Point{Column: 9})
	tree.root = newParentNodeInArena(tree.arena, 4, true, []*Node{chain, later}, nil, 0)
	tree.source = []byte("abcXYZ--z")
	tree.Edit(InputEdit{
		StartByte: 8, OldEndByte: 9, NewEndByte: 9,
		StartPoint: Point{Column: 8}, OldEndPoint: Point{Column: 9}, NewEndPoint: Point{Column: 9},
	})
	if missing.dirty() || chain.dirty() || !later.dirty() || !tree.root.dirty() {
		t.Fatalf("replacement dirtiness: missing=%t chain=%t later=%t root=%t", missing.dirty(), chain.dirty(), later.dirty(), tree.root.dirty())
	}
	if got, ok := missingNodeDependencyForNode(missing); !ok || got != want {
		t.Fatalf("unaffected missing dependency=%+v exact=%t, want %+v", got, ok, want)
	}
}

func BenchmarkMissingNodeDependencyErrorChain(b *testing.B) {
	for _, depth := range []int{8, 16} {
		b.Run(map[int]string{8: "depth8", 16: "depth16"}[depth], func(b *testing.B) {
			arena := acquireNodeArena(arenaClassIncremental)
			defer arena.Release()
			node := newLeafNodeInArena(arena, 1, true, 3, 3, Point{Column: 3}, Point{Column: 3})
			node.setMissing(true)
			node.setHasError(true)
			if !arena.setMissingNodeDependency(node, missingNodeDependency{paddingBytes: 3, paddingExtent: Point{Column: 3}, lookaheadBytes: 3}) {
				b.Fatal("set missing dependency")
			}
			for range depth {
				node = newParentNodeInArena(arena, 2, true, []*Node{node}, nil, 0)
			}
			entry := newStackEntryNode(node.parseState, node)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if !stackEntryEndsBeforeEditDependency(arena, entry, 8) {
					b.Fatal("chain should precede the edit")
				}
			}
		})
	}
}
