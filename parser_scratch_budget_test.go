package gotreesitter

import (
	"testing"
	"unsafe"
)

func TestParserScratchMemoryBudgetExhaustedByEntrySlabGrowth(t *testing.T) {
	var scratch parserScratch
	scratch.entries.ensureInitialCap(defaultStackEntrySlabCap)
	initial := scratch.allocatedBytes()
	scratch.setBudget(initial + 1)

	_ = scratch.entries.allocWithCap(defaultStackEntrySlabCap, defaultStackEntrySlabCap)
	if scratch.budgetExhausted() {
		t.Fatal("budget exhausted before entry-slab overflow")
	}

	_ = scratch.entries.alloc(1)
	if !scratch.budgetExhausted() {
		t.Fatal("budget not exhausted after entry-slab overflow")
	}
}

func TestParserScratchMemoryBudgetExhaustedByGSSSlabGrowth(t *testing.T) {
	var scratch parserScratch
	_ = scratch.gss.allocNode(stackEntry{state: 1}, nil, 1)
	initial := scratch.allocatedBytes()
	scratch.setBudget(initial + 1)

	for depth := 2; depth <= defaultGSSNodeSlabCap; depth++ {
		_ = scratch.gss.allocNode(stackEntry{state: 1}, nil, depth)
	}
	if scratch.budgetExhausted() {
		t.Fatal("budget exhausted before gss-slab overflow")
	}

	_ = scratch.gss.allocNode(stackEntry{state: 1}, nil, defaultGSSNodeSlabCap+1)
	if !scratch.budgetExhausted() {
		t.Fatal("budget not exhausted after gss-slab overflow")
	}
}

func TestParserScratchResetRecomputesAllocatedBytes(t *testing.T) {
	var scratch parserScratch
	scratch.entries.ensureInitialCap(defaultStackEntrySlabCap)
	_ = scratch.entries.allocWithCap(defaultStackEntrySlabCap, defaultStackEntrySlabCap)
	_ = scratch.entries.alloc(1)
	for depth := 1; depth <= defaultGSSNodeSlabCap+1; depth++ {
		_ = scratch.gss.allocNode(stackEntry{state: 1}, nil, depth)
	}

	if scratch.allocatedBytes() <= 0 {
		t.Fatal("allocatedBytes should be positive before reset")
	}

	scratch.entries.reset()
	scratch.gss.reset()

	want := scratch.entries.allocatedBytes + scratch.gss.allocatedBytes
	if got := scratch.allocatedBytes(); got != want {
		t.Fatalf("allocatedBytes after reset = %d, want %d", got, want)
	}
}

// TestNodeLinksCapClearedOnRelease verifies that releaseParserScratch clears
// the full capacity of nodeLinks, not just [:len].
//
// wireParentLinksWithScratch grows nodeLinks via append and returns the slice
// as stack[:0] — len=0 but cap>0 with live *Node pointers in the backing array.
// Without clear(nodeLinks[:cap]), those pointers survive across parses as GC roots.
func TestNodeLinksCapClearedOnRelease(t *testing.T) {
	// Build a scratch with a nodeLinks slice that has len=0 but cap>0,
	// simulating the state left by wireParentLinksWithScratch.
	var scratch parserScratch

	// Allocate a dummy node to use as a non-nil pointer.
	dummyNode := &Node{}

	// Simulate what wireParentLinksWithScratch does: append nodes, then reslice to [:0].
	scratch.nodeLinks = append(scratch.nodeLinks, dummyNode, dummyNode, dummyNode)
	scratch.nodeLinks = scratch.nodeLinks[:0]

	if len(scratch.nodeLinks) != 0 {
		t.Fatal("expected len=0 after reslice")
	}
	if cap(scratch.nodeLinks) < 3 {
		t.Fatal("expected cap>=3 after append")
	}

	// The stale pointer lives in the backing array at index 0.
	backingPtr := (*unsafe.Pointer)(unsafe.Pointer(&scratch.nodeLinks[:cap(scratch.nodeLinks)][0]))
	if *backingPtr == nil {
		t.Fatal("expected non-nil pointer in backing array before release")
	}

	releaseParserScratch(&scratch, false)

	// After release, the backing array must be fully zeroed.
	// (scratch.nodeLinks may be nil or have len=0; check the raw memory we saved.)
	if *backingPtr != nil {
		t.Fatal("stale *Node pointer remains in nodeLinks backing array after releaseParserScratch; " +
			"clear(nodeLinks[:cap]) not applied")
	}
}
