package gotreesitter

import "testing"

func TestFullParseNodeCapacityReusesFragmentedStorage(t *testing.T) {
	a := newNodeArena(arenaClassFull)
	target := len(a.nodes) + 17
	for i := 0; i < target; i++ {
		a.allocNode().startByte = uint32(i + 1)
	}
	a.reset()
	primary := &a.nodes[0]
	overflow := &a.nodeSlabs[0].data[0]
	bytes := a.allocatedBytes
	a.ensureFullParseNodeCapacity(target, false)
	if &a.nodes[0] != primary || len(a.nodeSlabs) != 1 || &a.nodeSlabs[0].data[0] != overflow {
		t.Fatal("reservation replaced retained node storage")
	}
	for i := 0; i < target; i++ {
		if n := a.allocNode(); n.startByte != 0 {
			t.Fatalf("node %d retained data from the previous parse", i)
		}
	}
	if a.allocatedBytes != bytes {
		t.Fatalf("reservation or allocation grew storage: %d -> %d", bytes, a.allocatedBytes)
	}
}

func TestFullParseNodeCapacityGrowsInsufficientStorage(t *testing.T) {
	a := newNodeArena(arenaClassFull)
	a.nodeSlabs = []nodeSlab{{data: make([]Node, 17)}}
	a.recomputeAllocatedBytes()
	target := len(a.nodes) + 18
	a.ensureFullParseNodeCapacity(target, false)
	if len(a.nodes) != target || len(a.nodeSlabs) != 0 {
		t.Fatalf("insufficient reservation: primary=%d slabs=%d", len(a.nodes), len(a.nodeSlabs))
	}
	bytes := a.allocatedBytes
	for i := 0; i < target; i++ {
		a.allocNode()
	}
	if a.allocatedBytes != bytes {
		t.Fatal("allocation exceeded reserved storage")
	}
}

func TestFullParseNodeCapacityRejectsUsedFragmentedStorage(t *testing.T) {
	a := newNodeArena(arenaClassFull)
	a.nodeSlabs = []nodeSlab{{data: make([]Node, 17)}}
	a.allocNode()
	defer func() {
		if recover() == nil {
			t.Fatal("reservation accepted an arena with live nodes")
		}
	}()
	a.ensureFullParseNodeCapacity(len(a.nodes)+1, false)
}

func TestFullParseTransientReservationRetainsReusableStorage(t *testing.T) {
	a := newNodeArena(arenaClassFull)
	limit := maxRetainedNodeCapacityForClass(a.class)
	target := limit + 17
	a.ensureFullParseNodeCapacity(target, true)
	if len(a.nodes) != limit || len(a.nodeSlabs) != 0 {
		t.Fatalf("primary=%d overflow=%d", len(a.nodes), len(a.nodeSlabs))
	}
	for i := 0; i < target; i++ {
		a.allocNode().startByte = uint32(i + 1)
	}
	a.reset()
	primary, overflow := &a.nodes[0], &a.nodeSlabs[0].data[0]
	bytes := a.allocatedBytes
	a.ensureFullParseNodeCapacity(target, true)
	for i := 0; i < target; i++ {
		if a.allocNode().startByte != 0 {
			t.Fatalf("node %d retained prior data", i)
		}
	}
	if &a.nodes[0] != primary || &a.nodeSlabs[0].data[0] != overflow || a.allocatedBytes != bytes {
		t.Fatal("warm allocation replaced reusable storage")
	}
}

func TestFullParseTransientReservationPreservesFixedBudget(t *testing.T) {
	parser := &Parser{reduceScratch: &reduceBuildScratch{transientParents: &transientParentScratch{}}}
	parser.SetMemoryBudgetBytes(32 << 20)
	source := make([]byte, 1<<20)
	target := parseFullArenaNodeCapacityForSource(source, parser.language, parser.fullArenaHintCapacity())
	if target <= maxRetainedNodeCapacityForClass(arenaClassFull) {
		t.Fatal("fixture does not exceed retained primary capacity")
	}
	arena := newNodeArena(arenaClassFull)
	scratch := acquireParserScratch()
	defer releaseParserScratch(scratch, true)
	parser.ensureFullParseInitialCapacity(source, arena, scratch)
	if len(arena.nodes) != target || len(arena.nodeSlabs) != 0 {
		t.Fatalf("fixed-budget reservation primary=%d overflow=%d, want primary=%d", len(arena.nodes), len(arena.nodeSlabs), target)
	}
}
