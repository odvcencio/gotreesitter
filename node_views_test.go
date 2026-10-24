package gotreesitter

import "testing"

func TestNodeViewArenaBoundsColdReserveAndScrubsReuse(t *testing.T) {
	nodeViewArenaPool.drain()
	defer nodeViewArenaPool.drain()
	arena := acquireNodeViewArena()
	if arenaAllocatedVolume(arena) > 128*1024 || len(arena.childSlabs) != 0 || len(arena.fieldSlabs) != 0 {
		t.Fatal("sparse result acquired eager full-tree reserves")
	}
	arena.allocNodeSlice(1)
	arena.allocFieldIDSlice(1)
	arena.allocFieldSourceSlice(1)
	tracked := arena.allocatedBytes
	arena.recomputeAllocatedBytes()
	if arena.allocatedBytes != tracked {
		t.Fatalf("first lazy slice growth was not charged: %d vs %d", tracked, arena.allocatedBytes)
	}
	for i := 0; i < 4096; i++ {
		node := arena.allocNodeFast()
		node.symbol = 42
		node.ownerArena = arena
	}
	arena.Release()
	reused := acquireNodeViewArena()
	defer reused.Release()
	if reused.used != 0 {
		t.Fatal("view arena retained live node count")
	}
	for i := 0; i < 4096; i++ {
		node := reused.allocNodeFast()
		if node.symbol != 0 || node.ownerArena != nil {
			t.Fatal("view arena retained a prior result")
		}
	}
}

func TestNodeViewArenaStopsReserveAtKnownOccurrenceCount(t *testing.T) {
	nodeViewArenaPool.drain()
	defer nodeViewArenaPool.drain()
	arena := acquireNodeViewArena()
	defer arena.Release()
	const count = 17003
	arena.viewNodeLimit = count
	for i := 0; i < count; i++ {
		arena.allocNodeFast()
	}
	capacity := len(arena.nodes)
	for _, slab := range arena.nodeSlabs {
		capacity += len(slab.data)
	}
	if capacity != count {
		t.Fatalf("full traversal reserved %d headers for %d occurrences", capacity, count)
	}
}
