package gotreesitter

import (
	"testing"
	"unsafe" //nolint:depguard
)

func TestEnsureNodeCapacityPanicsAfterAllocationStarted(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	_ = arena.allocNode()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when ensureNodeCapacity is called after allocations started")
		}
	}()
	arena.ensureNodeCapacity(len(arena.nodes) + 1)
}

func TestEnsureNodeCapacityPreallocationBeforeUse(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	before := len(arena.nodes)
	arena.ensureNodeCapacity(before + 128)
	if len(arena.nodes) <= before {
		t.Fatalf("ensureNodeCapacity did not grow nodes: before=%d after=%d", before, len(arena.nodes))
	}
}

func TestAllocNodeUsesOverflowSlabsWhenPrimaryExhausted(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	primaryCap := len(arena.nodes)
	if primaryCap <= 0 {
		t.Fatal("expected positive primary node capacity")
	}

	target := primaryCap + primaryCap/2
	for i := 0; i < target; i++ {
		n := arena.allocNode()
		if n == nil {
			t.Fatalf("allocNode returned nil at index %d", i)
		}
	}

	if arena.used != target {
		t.Fatalf("arena.used = %d, want %d", arena.used, target)
	}
	if len(arena.nodeSlabs) == 0 {
		t.Fatal("expected overflow node slabs to be allocated")
	}
}

func TestArenaResetRetainsOverflowWithinBudget(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	primaryCap := len(arena.nodes)
	if primaryCap <= 0 {
		t.Fatal("expected positive primary node capacity")
	}

	// Force multiple overflow slabs.
	target := primaryCap * 8
	for i := 0; i < target; i++ {
		_ = arena.allocNode()
	}
	if len(arena.nodeSlabs) < 2 {
		t.Fatalf("expected multiple overflow slabs before reset, got %d", len(arena.nodeSlabs))
	}

	arena.reset()
	if arena.used != 0 {
		t.Fatalf("arena.used after reset = %d, want 0", arena.used)
	}

	retained := 0
	for i, slab := range arena.nodeSlabs {
		if slab.used != 0 {
			t.Fatalf("slab %d used after reset = %d, want 0", i, slab.used)
		}
		retained += len(slab.data)
	}
	limit := maxRetainedOverflowNodeCapacityForClass(arena.class)
	if retained > limit {
		t.Fatalf("retained overflow capacity = %d, limit = %d", retained, limit)
	}
}

func TestArenaResetRetainsChildSlabsWithinBudget(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	base := defaultChildSliceCap(arena.class)
	if base <= 0 {
		t.Fatal("expected positive child slab capacity")
	}

	for i := 0; i < 32; i++ {
		s := arena.allocNodeSlice(base)
		if len(s) != base {
			t.Fatalf("allocNodeSlice len = %d, want %d", len(s), base)
		}
	}
	if len(arena.childSlabs) < 2 {
		t.Fatalf("expected multiple child slabs before reset, got %d", len(arena.childSlabs))
	}

	arena.reset()

	retained := 0
	for i, slab := range arena.childSlabs {
		if slab.used != 0 {
			t.Fatalf("child slab %d used after reset = %d, want 0", i, slab.used)
		}
		retained += len(slab.data)
	}
	limit := maxRetainedChildSliceCapacityForClass(arena.class)
	if retained > limit {
		t.Fatalf("retained child slab capacity = %d, limit = %d", retained, limit)
	}
}

func TestArenaResetRetainsFieldSlabsWithinBudget(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	base := defaultFieldSliceCap(arena.class)
	if base <= 0 {
		t.Fatal("expected positive field slab capacity")
	}

	for i := 0; i < 32; i++ {
		s := arena.allocFieldIDSlice(base)
		if len(s) != base {
			t.Fatalf("allocFieldIDSlice len = %d, want %d", len(s), base)
		}
	}
	if len(arena.fieldSlabs) < 2 {
		t.Fatalf("expected multiple field slabs before reset, got %d", len(arena.fieldSlabs))
	}

	arena.reset()

	retained := 0
	for i, slab := range arena.fieldSlabs {
		if slab.used != 0 {
			t.Fatalf("field slab %d used after reset = %d, want 0", i, slab.used)
		}
		retained += len(slab.data)
	}
	limit := maxRetainedFieldSliceCapacityForClass(arena.class)
	if retained > limit {
		t.Fatalf("retained field slab capacity = %d, limit = %d", retained, limit)
	}
}

// TestArenaNodeSlabFullClearOnReset verifies that reset() zeros the full backing
// array of each node slab, not just [:used]. This is required so that Go's GC
// can collect Node structs: Node contains pointer fields (children, parent, etc.)
// and stale pointers in the unused tail of the backing array prevent GC collection.
func TestArenaNodeSlabFullClearOnReset(t *testing.T) {
	arena := newNodeArena(arenaClassFull)

	// Fill primary array and spill into at least one overflow slab.
	primaryCap := len(arena.nodes)
	if primaryCap <= 0 {
		t.Fatal("expected positive primary node capacity")
	}
	target := primaryCap + 64
	for i := 0; i < target; i++ {
		n := arena.allocNode()
		if n == nil {
			t.Fatalf("allocNode returned nil at i=%d", i)
		}
		// Write a non-zero pointer into the node to make stale data detectable.
		n.parent = n
	}
	if len(arena.nodeSlabs) == 0 {
		t.Fatal("expected at least one overflow slab after allocating past primary capacity")
	}

	// Capture a raw pointer to the first element of the first overflow slab.
	// We will check after reset() that the slot is fully zeroed.
	firstSlab := &arena.nodeSlabs[0]
	if firstSlab.used == 0 {
		t.Fatal("expected overflow slab to have used > 0")
	}
	firstSlabDataPtr := unsafe.Pointer(&firstSlab.data[0])

	arena.reset()

	// After reset(), the slab's used counter must be 0.
	if firstSlab.used != 0 {
		t.Fatalf("slab.used after reset = %d, want 0", firstSlab.used)
	}
	// After reset(), the first element of the slab must be zero.
	// If only [:used] was cleared, a Node that was at index 0 before reset
	// would have its parent pointer still set, keeping the Node alive for GC.
	// Check that the parent pointer field is zeroed. Before the fix,
	// clear(slab.data[:used]) left stale pointers in the unused tail
	// after the first reset when primaryCap < len(slab.data).
	got := (*Node)(firstSlabDataPtr)
	if got.parent != nil {
		t.Fatalf("slab.data[0].parent after reset is %p, want nil; full clear not applied", got.parent)
	}
	if got.ownerArena != nil {
		t.Fatalf("slab.data[0].ownerArena after reset is %p, want nil; full clear not applied", got.ownerArena)
	}
}
