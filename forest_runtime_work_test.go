package gotreesitter

import "testing"

func TestForestAcceptedRuntimeCountsAllocatedSlots(t *testing.T) {
	arena := &nodeArena{nodes: make([]Node, 3), nodeSlabs: []nodeSlab{{data: make([]Node, 2)}}}
	root := arena.allocNode()
	root.endByte = 5
	// Three discarded alternatives fill the primary slab and use overflow.
	for i := 0; i < 3; i++ {
		arena.allocNode()
	}
	got := forestAcceptedRuntimeWithWork(root, []byte("[1,2]"), arena, 6)
	if got.NodesAllocated != 4 || got.TokensConsumed != 6 || !got.ForestFastPath || got.StopReason != ParseStopAccepted {
		t.Fatalf("runtime=%+v", got)
	}
}

func TestForestJSONRuntimeIncludesWork(t *testing.T) {
	parser := NewParser(loadBlobForDecode(t, "json"))
	tree, ok := parser.ParseForestExperimental([]byte("[1,2]"))
	if !ok || tree == nil {
		t.Fatal("forest declined JSON")
	}
	defer tree.Release()
	got := tree.ParseRuntime()
	if got.TokensConsumed != 6 {
		t.Fatalf("tokens=%d want=6 (including EOF)", got.TokensConsumed)
	}
	if got.NodesAllocated <= 0 {
		t.Fatal("forest returned no allocation count")
	}
}
