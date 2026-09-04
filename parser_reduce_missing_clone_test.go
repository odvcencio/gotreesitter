package gotreesitter

import "testing"

func TestParserReduceClonesCopyMissingNodeDependency(t *testing.T) {
	tests := []struct {
		name  string
		clone func(*nodeArena, *Node) *Node
	}{
		{
			name: "alias",
			clone: func(arena *nodeArena, node *Node) *Node {
				return aliasedNodeInArena(arena, nil, node, 2)
			},
		},
		{
			name:  "generic",
			clone: cloneNodeInArena,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sourceArena, source, want := newParserReduceMissingCloneFixture(t)
			defer sourceArena.Release()
			destinationArena := acquireNodeArena(arenaClassIncremental)
			defer destinationArena.Release()

			clone := test.clone(destinationArena, source)
			if clone == nil {
				t.Fatal("clone = nil")
			}
			if clone.ownerArena != destinationArena {
				t.Fatal("clone owner arena does not match destination")
			}
			if clone.dirty() {
				t.Fatal("valid missing dependency made the clone dirty")
			}
			if got, ok := missingNodeDependencyForNode(clone); !ok || got != want {
				t.Fatalf("cloned dependency=%+v exact=%t, want %+v", got, ok, want)
			}
		})
	}
}

func TestParserReduceClonesFailClosedForStaleMissingNodeDependency(t *testing.T) {
	tests := []struct {
		name  string
		clone func(*nodeArena, *Node) *Node
	}{
		{
			name: "alias",
			clone: func(arena *nodeArena, node *Node) *Node {
				return aliasedNodeInArena(arena, nil, node, 2)
			},
		},
		{
			name:  "generic",
			clone: cloneNodeInArena,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sourceArena, source, _ := newParserReduceMissingCloneFixture(t)
			defer sourceArena.Release()
			destinationArena := acquireNodeArena(arenaClassIncremental)
			defer destinationArena.Release()

			source.startByte++
			source.endByte++
			clone := test.clone(destinationArena, source)
			if clone == nil {
				t.Fatal("clone = nil")
			}
			if !clone.dirty() {
				t.Fatal("stale missing dependency did not dirty the clone")
			}
			if _, present := missingNodeDependencyEntryForNode(clone); present {
				t.Fatal("stale missing dependency was copied to the clone")
			}
		})
	}
}

func newParserReduceMissingCloneFixture(t *testing.T) (*nodeArena, *Node, missingNodeDependency) {
	t.Helper()
	arena := acquireNodeArena(arenaClassIncremental)
	dependency := missingNodeDependency{
		stackByte: 0, stackPoint: Point{},
		paddingBytes: 3, paddingExtent: Point{Column: 3},
		lookaheadBytes: 3,
	}
	node := newLeafNodeInArena(arena, 1, true, 3, 3, Point{Column: 3}, Point{Column: 3})
	node.setMissing(true)
	node.setHasError(true)
	if !arena.setMissingNodeDependency(node, dependency) {
		arena.Release()
		t.Fatal("set missing dependency")
	}
	return arena, node, dependency
}
