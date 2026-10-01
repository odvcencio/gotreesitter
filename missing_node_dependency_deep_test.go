package gotreesitter

import "testing"

func TestStackEntryEditDependencyDeepErrorAncestors(t *testing.T) {
	for _, lazy := range []bool{false, true} {
		name := "materialized"
		if lazy {
			name = "final-child-refs"
		}
		t.Run(name, func(t *testing.T) {
			tree, leaf, _ := newMissingDependencyTree(t)
			defer tree.Release()
			arena := tree.arena
			arena.finalChildRefs = lazy
			root := leaf
			for depth := 0; depth < 48; depth++ {
				if lazy {
					parent := newPendingParentInArena(arena, 2, true, 4,
						[]stackEntry{newStackEntryNode(root.parseState, root)},
						3, 3, Point{Column: 3}, Point{Column: 3}, true)
					entry := newStackEntryPendingParent(parent.parseState, parent)
					root = materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
				} else {
					root = newParentNodeInArena(arena, 2, true, []*Node{root}, nil, 0)
				}
				if root == nil || !root.hasError() {
					t.Fatalf("depth %d lost the missing descendant's error", depth)
				}
			}
			tree.root = root
			entry := newStackEntryNode(root.parseState, root)
			for _, boundary := range []struct {
				start uint32
				want  bool
			}{{3, false}, {6, false}, {7, true}} {
				if got := stackEntryEndsBeforeEditDependency(arena, entry, boundary.start); got != boundary.want {
					t.Fatalf("edit start %d: ends before dependency=%t, want %t", boundary.start, got, boundary.want)
				}
			}
			if lazy && (len(root.children) != 0 || arena.finalChildRefsMaterializedParents != 0) {
				t.Fatal("dependency check materialized the final child references")
			}
		})
	}
}
