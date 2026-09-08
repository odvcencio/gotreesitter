package gotreesitter

import "testing"

func TestArenaSupertypeBudgetChargesAllocationAndGrowth(t *testing.T) {
	arena := &nodeArena{nodes: make([]Node, 2), nodeSlabs: []nodeSlab{{data: make([]Node, 3)}}}
	arena.recomputeAllocatedBytes()
	base := arena.allocatedBytes
	arena.setBudget(1)
	if arena.nodeSupertypeMask(&arena.nodes[0]) != 0 || arena.allocatedBytes != base {
		t.Fatal("reading an absent mask allocated storage")
	}
	for _, node := range []*Node{&arena.nodes[0], &arena.nodeSlabs[0].data[0]} {
		arena.setNodeSupertypeMask(node, 1)
		if !arena.budgetExhausted() {
			t.Fatal("supertype allocation did not exhaust the growth budget")
		}
		charged := arena.allocatedBytes
		arena.recomputeAllocatedBytes()
		if arena.allocatedBytes != charged {
			t.Fatalf("allocation accounting changed on recompute: got=%d want=%d", arena.allocatedBytes, charged)
		}
		arena.setBudget(1)
	}
	for mask := uint32(2); mask <= 255; mask++ {
		arena.internSupertypeSet(mask)
		want := base + int64(cap(arena.nodeSupertypes)+cap(arena.nodeSlabs[0].supertypes)) + 4*int64(cap(arena.supertypeSets))
		if arena.allocatedBytes != want {
			t.Fatalf("mask %d allocation=%d, want %d", mask, arena.allocatedBytes, want)
		}
	}
	arena.setBudget(1)
	arena.internSupertypeSet(1)
	if arena.internSupertypeSet(256) != 0 || arena.budgetExhausted() {
		t.Fatal("duplicate or rejected mask changed allocation accounting")
	}
}

func TestArenaSupertypeBudgetResetAndReuse(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	primary := arena.allocNode()
	for arena.used < len(arena.nodes) {
		arena.allocNode()
	}
	overflow := arena.allocNode()
	arena.setNodeSupertypeMask(primary, 1)
	arena.setNodeSupertypeMask(overflow, 2)
	retained := arena.nodeSupertypeBytesAllocated()
	arena.reset()
	if got := arena.nodeSupertypeBytesAllocated(); got != retained {
		t.Fatalf("retained supertype bytes=%d, want %d", got, retained)
	}
	charged := arena.allocatedBytes
	arena.recomputeAllocatedBytes()
	if arena.allocatedBytes != charged {
		t.Fatal("reset did not retain exact allocation accounting")
	}
	if arena.nodeSupertypeMask(primary) != 0 || arena.nodeSupertypeMask(overflow) != 0 {
		t.Fatal("reset retained a previous mask")
	}
	arena.setBudget(1)
	arena.setNodeSupertypeMask(arena.allocNode(), 1)
	if arena.budgetExhausted() || arena.allocatedBytes != charged {
		t.Fatal("reuse charged retained supertype capacity again")
	}
}

func TestArenaSupertypeBudgetDropsTrimmedPrimaryStorage(t *testing.T) {
	arena := &nodeArena{class: arenaClassIncremental}
	arena.nodes = make([]Node, maxRetainedNodeCapacityForClass(arena.class)+1)
	arena.setNodeSupertypeMask(&arena.nodes[0], 1)
	arena.trimPrimaryNodeCapacity()
	if arena.nodes != nil || arena.nodeSupertypes != nil {
		t.Fatal("primary trimming retained parallel supertype storage")
	}
	arena.recomputeAllocatedBytes()
	if want := 4 * int64(cap(arena.supertypeSets)); arena.allocatedBytes != want {
		t.Fatalf("trimmed allocation=%d, want %d", arena.allocatedBytes, want)
	}
}
