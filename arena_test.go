package gotreesitter

import (
	"testing"
	"unsafe"
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

func TestEnsureExactNodeCapacityDoesNotOverRound(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	target := len(arena.nodes) + 123
	arena.ensureExactNodeCapacity(target)
	if got := len(arena.nodes); got != target {
		t.Fatalf("ensureExactNodeCapacity nodes = %d, want %d", got, target)
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

func TestAllocRawShapeDoesNotGrowPastRefSlabLimit(t *testing.T) {
	maxSlabs := 1 << (32 - rawShapeRefIndexBits)
	arena := &nodeArena{
		class:              arenaClassFull,
		rawShapeSlabs:      make([]rawShapeSlab, maxSlabs),
		rawShapeSlabCursor: maxSlabs - 1,
		allocatedBytes:     1234,
	}

	ref, shape := arena.allocRawShape()
	if ref != 0 || shape != nil {
		t.Fatalf("allocRawShape at slab limit = (%d, %p), want zero/nil", ref, shape)
	}
	if got := len(arena.rawShapeSlabs); got != maxSlabs {
		t.Fatalf("rawShapeSlabs length after exhausted alloc = %d, want %d", got, maxSlabs)
	}
	if got := arena.rawShapeSlabCursor; got != maxSlabs-1 {
		t.Fatalf("rawShapeSlabCursor after exhausted alloc = %d, want %d", got, maxSlabs-1)
	}
	if got := arena.allocatedBytes; got != 1234 {
		t.Fatalf("allocatedBytes after exhausted alloc = %d, want 1234", got)
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

func TestArenaResetTrimsFinalChildSidecarsWithinBudget(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	limit := maxRetainedFinalChildSidecarCapacityForClass(arena.class)
	if limit <= 0 {
		t.Fatal("expected positive final child sidecar retention limit")
	}

	for i := 0; i < limit+128; i++ {
		parent := arena.allocNode()
		childRange, _ := arena.allocPendingChildEntries(1)
		arena.attachFinalChildRefs(parent, childRange)
	}
	if cap(arena.finalChildSidecars) <= limit {
		t.Fatalf("final child sidecar cap = %d, want > retention limit %d before reset", cap(arena.finalChildSidecars), limit)
	}

	arena.reset()

	if got := len(arena.finalChildSidecars); got != 0 {
		t.Fatalf("len(finalChildSidecars) after reset = %d, want 0", got)
	}
	if got := cap(arena.finalChildSidecars); got > limit {
		t.Fatalf("cap(finalChildSidecars) after reset = %d, limit = %d", got, limit)
	}
	if got, wantMax := arena.finalChildSidecarBytesAllocated(), finalChildSidecarBytesForCap(limit); got > wantMax {
		t.Fatalf("final child sidecar bytes after reset = %d, want <= %d", got, wantMax)
	}
}

// TestChildSlabStalePointersAfterReset checks whether child slabs (which hold
// []*Node) can retain stale pointers in the region [used:cap] after reset().
// allocNodeSlice calls clear(out) on each allocation, zeroing [start:used].
// The region [used:cap] within a slab is never written, so it stays nil from
// the original make(). This test verifies that assumption holds: after two
// parse cycles, child slab positions beyond the last used index are nil.
func TestChildSlabStalePointersAfterReset(t *testing.T) {
	arena := newNodeArena(arenaClassFull)

	// Cycle 1: allocate several child slices, then reset.
	dummy := arena.allocNode()
	s1 := arena.allocNodeSlice(8)
	for i := range s1 {
		s1[i] = dummy
	}
	s2 := arena.allocNodeSlice(8)
	for i := range s2 {
		s2[i] = dummy
	}
	if len(arena.childSlabs) == 0 {
		t.Fatal("expected child slabs after allocation")
	}
	usedAfterCycle1 := arena.childSlabs[0].used

	arena.reset()

	// Cycle 2: allocate a smaller child slice from the same slab.
	_ = arena.allocNodeSlice(4)
	usedAfterCycle2 := arena.childSlabs[0].used

	// Positions [usedAfterCycle2 : usedAfterCycle1] were written in cycle 1
	// and cleared by reset(). Verify they are nil now.
	slab := arena.childSlabs[0]
	for i := usedAfterCycle2; i < usedAfterCycle1; i++ {
		if slab.data[i] != nil {
			t.Fatalf("child slab data[%d] = %p after reset, expected nil (stale pointer not cleared)", i, slab.data[i])
		}
	}
	// Positions [usedAfterCycle1 : cap] were never written (make zeroes them).
	for i := usedAfterCycle1; i < len(slab.data); i++ {
		if slab.data[i] != nil {
			t.Fatalf("child slab data[%d] = %p, expected nil (was never written)", i, slab.data[i])
		}
	}
}

// TestNodeRetentionCapRespectsByteLimit checks that the maximum node storage
// an arena may retain after reset() does not exceed maxRetainedFullNodeBytes,
// while still retaining the default full-parse slab for warm reuse.
// Regression: an earlier PR revision interpreted a byte limit as a node count.
// The fix stores ceilings in bytes and converts to node counts via sizeof(Node).
func TestNodeRetentionCapRespectsByteLimit(t *testing.T) {
	nodeSize := int(unsafe.Sizeof(Node{}))
	maxNodes := maxRetainedNodeCapacityForClass(arenaClassFull)
	actualBytes := maxNodes * nodeSize
	if actualBytes > maxRetainedFullNodeBytes {
		t.Fatalf("maxRetainedNodeCapacityForClass(full) = %d nodes = %d bytes; "+
			"exceeds intended ceiling %d bytes (%d KB)",
			maxNodes, actualBytes, maxRetainedFullNodeBytes, maxRetainedFullNodeBytes/1024)
	}
	if maxNodes < nodeCapacityForClass(arenaClassFull) {
		t.Fatalf("maxRetainedNodeCapacityForClass(full) = %d nodes; "+
			"below default full-parse slab capacity %d nodes",
			maxNodes, nodeCapacityForClass(arenaClassFull))
	}

	maxOverflowNodes := maxRetainedOverflowNodeCapacityForClass(arenaClassFull)
	actualOverflowBytes := maxOverflowNodes * nodeSize
	if actualOverflowBytes > maxRetainedFullOverflowNodeBytes {
		t.Fatalf("maxRetainedOverflowNodeCapacityForClass(full) = %d nodes = %d bytes; "+
			"exceeds intended overflow ceiling %d bytes (%d MB)",
			maxOverflowNodes, actualOverflowBytes, maxRetainedFullOverflowNodeBytes, maxRetainedFullOverflowNodeBytes/(1024*1024))
	}
	if maxOverflowNodes < nodeCapacityForClass(arenaClassFull) {
		t.Fatalf("maxRetainedOverflowNodeCapacityForClass(full) = %d nodes; "+
			"below default full-parse slab capacity %d nodes",
			maxOverflowNodes, nodeCapacityForClass(arenaClassFull))
	}
}

// TestEvictionGuardPreventsOversizedArenaReuse checks that a full-parse arena
// whose allocatedBytes exceed maxRetainedFullArenaBytes at Release() time is
// dropped instead of returned to the pool.
// Regression: the guard was evaluated inside pool.release() AFTER reset() had
// already called recomputeAllocatedBytes(), overwriting the peak value with the
// much smaller post-trim value. The guard never fired.
func TestEvictionGuardPreventsOversizedArenaReuse(t *testing.T) {
	fullArenaPool.drain()

	a := fullArenaPool.acquire()
	// Simulate an arena that grew very large during a parse.
	a.allocatedBytes = maxRetainedFullArenaBytes + 1
	a.Release()

	fullArenaPool.mu.Lock()
	poolSize := len(fullArenaPool.free)
	fullArenaPool.mu.Unlock()

	if poolSize != 0 {
		t.Fatalf("oversized arena returned to pool (size=%d); eviction guard did not fire", poolSize)
	}
}

func TestArenaByteBreakdownMatchesAllocatedBytes(t *testing.T) {
	EnableArenaBreakdown(true)
	defer EnableArenaBreakdown(false)

	arena := newNodeArena(arenaClassFull)

	_ = arena.allocNode()
	_ = arena.allocNodeSlice(1)
	_ = arena.allocNodeSlice(2)
	_ = arena.allocNodeSlice(3)
	_ = arena.allocNodeSlice(4)
	_ = arena.allocFieldIDSlice(5)
	_ = arena.allocFieldSourceSlice(6)
	_ = arena.allocNoTreeNode()
	left := newLeafNodeInArena(arena, Symbol(1), true, 0, 0, Point{}, Point{})
	right := newLeafNodeInArena(arena, Symbol(2), true, 0, 0, Point{}, Point{})
	children := arena.allocNodeSlice(2)
	children[0] = left
	children[1] = right
	_ = newParentNodeInArenaNoLinksWithFieldSources(arena, Symbol(3), true, children, nil, nil, 0, false)
	childRange, childEntries := arena.allocPendingChildEntries(1)
	childEntries[0] = newPendingChildEntry(newStackEntryNode(left.parseState, left))
	_ = newParentNodeInArenaWithFinalChildRefs(arena, Symbol(4), true, childRange, 0, false)

	want := arena.nodeStructBytesAllocated() +
		arena.noTreeNodeBytesAllocated() +
		arena.compactFullLeafBytesAllocated() +
		arena.pendingParentBytesAllocated() +
		arena.pendingChildEntryBytesAllocated() +
		arena.finalChildSidecarBytesAllocated() +
		arena.compactCheckpointLeafBytesAllocated() +
		arena.childSliceBytesAllocated() +
		arena.fieldIDBytesAllocated() +
		arena.fieldSourceBytesAllocated() +
		arena.externalScannerCheckpointBytesAllocated()
	if got := arena.allocatedBytes; got != want {
		t.Fatalf("allocatedBytes = %d, breakdown sum = %d", got, want)
	}
	breakdown := arena.collectArenaBreakdown()
	if got := breakdown.FinalChildSidecarBytesAllocated; got == 0 {
		t.Fatal("FinalChildSidecarBytesAllocated = 0, want > 0")
	}
	if got, want := breakdown.NodeLiveCount, uint64(arena.used); got != want {
		t.Fatalf("NodeLiveCount = %d, want %d", got, want)
	}
	if got, want := breakdown.NodeCapacityCount, uint64(len(arena.nodes)); got != want {
		t.Fatalf("NodeCapacityCount = %d, want %d", got, want)
	}
	if got, want := breakdown.NodeCapacityWaste, breakdown.NodeCapacityCount-breakdown.NodeLiveCount; got != want {
		t.Fatalf("NodeCapacityWaste = %d, want %d", got, want)
	}
	if got, want := breakdown.PrimaryNodeCapacity, uint64(len(arena.nodes)); got != want {
		t.Fatalf("PrimaryNodeCapacity = %d, want %d", got, want)
	}
	if got, want := breakdown.PrimaryNodeUsed, uint64(arena.used); got != want {
		t.Fatalf("PrimaryNodeUsed = %d, want %d", got, want)
	}
	if breakdown.OverflowNodeCapacity != 0 || breakdown.OverflowNodeUsed != 0 || breakdown.OverflowNodeSlabs != 0 {
		t.Fatalf("overflow node stats = cap=%d used=%d slabs=%d, want all zero",
			breakdown.OverflowNodeCapacity, breakdown.OverflowNodeUsed, breakdown.OverflowNodeSlabs)
	}
	if breakdown.LargestNodeSlabUsedFraction <= 0 || breakdown.LargestNodeSlabUsedFraction > 1 {
		t.Fatalf("LargestNodeSlabUsedFraction = %g, want within (0,1]", breakdown.LargestNodeSlabUsedFraction)
	}
	if got, want := breakdown.ChildSlicesConstructed, uint64(1); got != want {
		t.Fatalf("childSlicesConstructed = %d, want %d", got, want)
	}
	if got, want := breakdown.ChildPointersConstructed, uint64(12); got != want {
		t.Fatalf("childPointersConstructed = %d, want %d", got, want)
	}
	if breakdown.ChildSlicesLen1 != 0 || breakdown.ChildSlicesLen2 != 1 || breakdown.ChildSlicesLen3 != 0 || breakdown.ChildSlicesLen4Plus != 0 {
		t.Fatalf("child slice histogram = (%d,%d,%d,%d), want (0,1,0,0)",
			breakdown.ChildSlicesLen1, breakdown.ChildSlicesLen2, breakdown.ChildSlicesLen3, breakdown.ChildSlicesLen4Plus)
	}
	if got, want := breakdown.FieldIDElementsConstructed, uint64(5); got != want {
		t.Fatalf("fieldIDElementsConstructed = %d, want %d", got, want)
	}
	if got, want := breakdown.FieldSourceElementsConstructed, uint64(6); got != want {
		t.Fatalf("fieldSourceElementsConstructed = %d, want %d", got, want)
	}
}

func TestPendingChildEntryBreakdownReportsUsedCapacityAndWaste(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	_, first := arena.allocPendingChildEntries(3)
	_, second := arena.allocPendingChildEntries(5)
	if len(first) != 3 || len(second) != 5 {
		t.Fatalf("pending child entries len = (%d,%d), want (3,5)", len(first), len(second))
	}

	breakdown := arena.collectArenaBreakdown()
	if got, want := breakdown.PendingChildEntriesAllocated, uint64(8); got != want {
		t.Fatalf("PendingChildEntriesAllocated = %d, want %d", got, want)
	}
	if breakdown.PendingChildEntryCapacity < breakdown.PendingChildEntriesAllocated {
		t.Fatalf("PendingChildEntryCapacity = %d, allocated = %d", breakdown.PendingChildEntryCapacity, breakdown.PendingChildEntriesAllocated)
	}
	if got, want := breakdown.PendingChildEntryWaste, breakdown.PendingChildEntryCapacity-breakdown.PendingChildEntriesAllocated; got != want {
		t.Fatalf("PendingChildEntryWaste = %d, want %d", got, want)
	}
}

func TestPendingChildEntrySlabsUseFixedFullGrowth(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	slabCap := defaultPendingChildEntrySlabCap(arena.class)
	for i := 0; i < slabCap+1; i++ {
		_, _ = arena.allocPendingChildEntries(1)
	}

	if got := len(arena.pendingChildEntrySlabs); got != 2 {
		t.Fatalf("pending child entry slabs = %d, want 2", got)
	}
	if got := len(arena.pendingChildEntrySlabs[1].data); got != slabCap {
		t.Fatalf("second pending child entry slab cap = %d, want %d", got, slabCap)
	}
}

func TestPendingChildEntrySlabsKeepGeometricIncrementalGrowth(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	slabCap := defaultPendingChildEntrySlabCap(arena.class)
	for i := 0; i < slabCap+1; i++ {
		_, _ = arena.allocPendingChildEntries(1)
	}

	if got := len(arena.pendingChildEntrySlabs); got != 2 {
		t.Fatalf("pending child entry slabs = %d, want 2", got)
	}
	if got, want := len(arena.pendingChildEntrySlabs[1].data), slabCap*2; got != want {
		t.Fatalf("second incremental pending child entry slab cap = %d, want %d", got, want)
	}
}

// TestArenaNodeSlabClearsWrittenSlotsOnReset verifies that reset() zeros every
// node slot written during the parse. Node contains pointer fields (children,
// parent, ownerArena), and stale pointers in retained arena slabs prevent GC
// collection.
func TestArenaNodeSlabClearsWrittenSlotsOnReset(t *testing.T) {
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
		n.flags = nodeFlagNamed | nodeFlagExtra | nodeFlagMissing | nodeFlagHasError
		n.errorRankCache = 3
	}
	if len(arena.nodeSlabs) == 0 {
		t.Fatal("expected at least one overflow slab after allocating past primary capacity")
	}
	primaryPtr := unsafe.Pointer(&arena.nodes[0])
	primaryUsedBeforeReset := len(arena.nodes)

	// Capture a raw pointer to the first element of the first overflow slab.
	// We will check after reset() that the written slot is zeroed.
	firstSlab := &arena.nodeSlabs[0]
	if firstSlab.used == 0 {
		t.Fatal("expected overflow slab to have used > 0")
	}
	firstSlabDataPtr := unsafe.Pointer(&firstSlab.data[0])
	slabUsedBeforeReset := firstSlab.used

	arena.reset()

	// After reset(), the slab's used counter must be 0.
	if firstSlab.used != 0 {
		t.Fatalf("slab.used after reset = %d, want 0", firstSlab.used)
	}
	for i := 0; i < primaryUsedBeforeReset; i++ {
		got := (*Node)(unsafe.Add(primaryPtr, uintptr(i)*unsafe.Sizeof(Node{})))
		if got.parent != nil {
			t.Fatalf("primary node[%d].parent after reset is %p, want nil", i, got.parent)
		}
		if got.ownerArena != nil {
			t.Fatalf("primary node[%d].ownerArena after reset is %p, want nil", i, got.ownerArena)
		}
		if got.flags != 0 {
			t.Fatalf("primary node[%d].flags after reset = %d, want 0", i, got.flags)
		}
		if got.errorRankCache != 0 {
			t.Fatalf("primary node[%d].errorRankCache after reset = %d, want 0", i, got.errorRankCache)
		}
	}
	for i := 0; i < slabUsedBeforeReset; i++ {
		got := (*Node)(unsafe.Add(firstSlabDataPtr, uintptr(i)*unsafe.Sizeof(Node{})))
		if got.parent != nil {
			t.Fatalf("slab.data[%d].parent after reset is %p, want nil", i, got.parent)
		}
		if got.ownerArena != nil {
			t.Fatalf("slab.data[%d].ownerArena after reset is %p, want nil", i, got.ownerArena)
		}
		if got.flags != 0 {
			t.Fatalf("slab.data[%d].flags after reset = %d, want 0", i, got.flags)
		}
		if got.errorRankCache != 0 {
			t.Fatalf("slab.data[%d].errorRankCache after reset = %d, want 0", i, got.errorRankCache)
		}
	}
}

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
