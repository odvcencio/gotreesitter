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
	a.ensureFullParseNodeCapacity(target)
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
	a.ensureFullParseNodeCapacity(target)
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
	a.ensureFullParseNodeCapacity(len(a.nodes) + 1)
}
