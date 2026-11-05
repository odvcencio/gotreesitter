package gotreesitter

import (
	"sync"
	"testing"
	"unsafe"
)

func TestParserScratchMemoryBudgetExhaustedByEntrySlabGrowth(t *testing.T) {
	var scratch parserScratch
	scratch.entries.ensureInitialCap(defaultStackEntrySlabCap)
	scratch.setBudget(1)

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
	scratch.setBudget(1)

	for depth := 2; depth <= defaultGSSNodeSlabCap; depth++ {
		_ = scratch.gss.allocNode(stackEntry{state: 1}, nil, uint32(depth))
	}
	if scratch.budgetExhausted() {
		t.Fatal("budget exhausted before gss-slab overflow")
	}

	_ = scratch.gss.allocNode(stackEntry{state: 1}, nil, defaultGSSNodeSlabCap+1)
	if !scratch.budgetExhausted() {
		t.Fatal("budget not exhausted after gss-slab overflow")
	}
}

func TestParserScratchMemoryBudgetExhaustedByMergeScratchGrowth(t *testing.T) {
	var scratch parserScratch
	_ = ensureMergeSlotCap(&scratch.merge, 1)
	scratch.setBudget(1)

	_ = ensureMergeSlotCap(&scratch.merge, 2)
	if !scratch.budgetExhausted() {
		t.Fatal("budget not exhausted after merge-slot growth")
	}
}

func TestParserScratchBudgetUsesPerParseGrowth(t *testing.T) {
	scratch := &parserScratch{}
	scratch.entries.allocatedBytes = 8 << 20
	scratch.gss.allocatedBytes = 96 << 20

	scratch.setBudget(64 << 20)
	if scratch.budgetExhausted() {
		t.Fatal("budget exhausted by retained scratch baseline")
	}

	scratch.gss.allocatedBytes += 63 << 20
	if scratch.budgetExhausted() {
		t.Fatal("budget exhausted before per-parse growth reached budget")
	}

	scratch.entries.allocatedBytes += 1 << 20
	if !scratch.budgetExhausted() {
		t.Fatal("budget not exhausted after per-parse growth reached budget")
	}
}

func TestParserScratchBudgetCountsRecoveryStackEntryGrowth(t *testing.T) {
	var scratch parserScratch
	entries := make([]stackEntry, 8)
	for i := range entries {
		entries[i] = stackEntry{state: StateID(i + 1)}
	}
	stack := glrStack{entries: entries}
	stack.ensureGSS(&scratch.gss)
	scratch.setBudget(1)

	_ = cStackEntriesTopFirst(&stack, &scratch.gss)

	if !scratch.budgetExhausted() {
		t.Fatal("budget not exhausted by recovery stack-entry growth")
	}
}

func TestMergeAliveLimitHonorsScratchBudget(t *testing.T) {
	var scratch glrMergeScratch
	perStack := int64(unsafe.Sizeof(glrStack{}) + unsafe.Sizeof(glrMergeSlot{}))
	scratch.budgetBytes = perStack * 3

	if got, want := mergeAliveLimitForScratch(&scratch, 100), 3; got != want {
		t.Fatalf("mergeAliveLimitForScratch = %d, want %d", got, want)
	}
}

func TestMergeAliveLimitAppliesEmergencyCap(t *testing.T) {
	if got, want := mergeAliveLimitForScratch(nil, maxMergeAliveStacks+100), maxMergeAliveStacks; got != want {
		t.Fatalf("mergeAliveLimitForScratch = %d, want emergency cap %d", got, want)
	}
}

func TestParserScratchResetRecomputesAllocatedBytes(t *testing.T) {
	var scratch parserScratch
	scratch.entries.ensureInitialCap(defaultStackEntrySlabCap)
	_ = scratch.entries.allocWithCap(defaultStackEntrySlabCap, defaultStackEntrySlabCap)
	_ = scratch.entries.alloc(1)
	for depth := 1; depth <= defaultGSSNodeSlabCap+1; depth++ {
		_ = scratch.gss.allocNode(stackEntry{state: 1}, nil, uint32(depth))
	}
	_ = ensureMergeResultCap(&scratch.merge, 2)
	_ = ensureMergeSlotCap(&scratch.merge, 2)
	scratch.merge.beginEquivEpoch()

	if scratch.allocatedBytes() <= 0 {
		t.Fatal("allocatedBytes should be positive before reset")
	}

	scratch.entries.reset()
	scratch.gss.reset()
	scratch.merge.reset()

	want := scratch.entries.allocatedBytes + scratch.gss.allocatedBytes + scratch.merge.allocatedBytes()
	if got := scratch.allocatedBytes(); got != want {
		t.Fatalf("allocatedBytes after reset = %d, want %d", got, want)
	}
}

func TestCaptureParseScratchStatsReportsGSSShape(t *testing.T) {
	var scratch parserScratch
	first := scratch.gss.allocNode(stackEntry{state: 1}, nil, 1)
	scratch.setBudget(64 << 20)
	second := scratch.gss.allocNode(stackEntry{state: 2}, first, 2)
	if second == nil {
		t.Fatal("second GSS node is nil")
	}

	var runtime ParseRuntime
	if !captureParseScratchStats(&runtime, &scratch, nil, nil) {
		t.Fatal("captureParseScratchStats returned false")
	}
	if got, want := runtime.ScratchBaselineBytes, scratch.budgetBaselineBytes; got != want {
		t.Fatalf("ScratchBaselineBytes = %d, want %d", got, want)
	}
	if got, want := runtime.GSSBaselineBytes, scratch.gssBaselineBytes; got != want {
		t.Fatalf("GSSBaselineBytes = %d, want %d", got, want)
	}
	if got, want := runtime.GSSSlabCount, len(scratch.gss.slabs); got != want {
		t.Fatalf("GSSSlabCount = %d, want %d", got, want)
	}
	if got, want := runtime.GSSNodesUsed, 2; got != want {
		t.Fatalf("GSSNodesUsed = %d, want %d", got, want)
	}
	if got, want := runtime.GSSNodesCapacity, scratch.gss.capacityNodes(); got != want {
		t.Fatalf("GSSNodesCapacity = %d, want %d", got, want)
	}
	if runtime.GSSNodesCapacity < runtime.GSSNodesUsed {
		t.Fatalf("GSSNodesCapacity = %d, used = %d", runtime.GSSNodesCapacity, runtime.GSSNodesUsed)
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

func TestGoCompatFramesReleasedAboveRetentionCap(t *testing.T) {
	var scratch parserScratch
	scratch.goCompatFrames = make([]goCompatSubtreeFrame, 0, maxRetainedGoCompatFrames+1)

	releaseParserScratch(&scratch, false)

	if scratch.goCompatFrames != nil {
		t.Fatalf("goCompatFrames capacity = %d, want released", cap(scratch.goCompatFrames))
	}
}

func TestHeavySmallScratchReusesRetainedCapacity(t *testing.T) {
	s := &parserScratch{}
	s.sourceBytes = 4096
	s.gss.slabs = []gssNodeSlab{{data: make([]gssNode, maxRetainedGSSNodes), reachMarks: make([]uint32, maxRetainedGSSNodes)}}
	s.gss.recomputeAllocatedBytes()
	lang := &Language{grammarBlobSHA256Valid: true, grammarBlobSHA256: [32]byte{1}}
	p := &Parser{language: lang}
	s.heavyGrammar, _ = scratchKeyForLanguage(lang)
	fresh := &Parser{language: lang}
	if !parserHeavySmallScratchFits(s, fresh, 4096) {
		t.Fatal("heavy small scratch was not classified for reuse")
	}
	if parserHeavySmallScratchFits(s, fresh, 100) {
		t.Fatal("strongly shrunken input inherited heavy scratch")
	}
	other := &Parser{language: &Language{grammarBlobSHA256Valid: true, grammarBlobSHA256: [32]byte{2}}}
	if parserHeavySmallScratchFits(s, other, 4096) {
		t.Fatal("different language inherited heavy scratch")
	}
	releaseParserScratchForParser(s, false, p)
	reused := acquireParserScratchForSource(4096, fresh)
	defer releaseParserScratchForParser(reused, false, fresh)
	if reused != s {
		t.Skip("sync.Pool did not return the retained scratch")
	}
	if reused.gss.capacityNodes() != maxRetainedGSSNodes {
		t.Fatalf("heavy scratch lost its reusable GSS: %d nodes", reused.gss.capacityNodes())
	}
}

func TestUnknownHeavyScratchMatchesGrammarGeometry(t *testing.T) {
	lang := &Language{StateCount: 10, SymbolCount: 5, TokenCount: 3}
	p := &Parser{language: lang}
	s := &parserScratch{sourceBytes: 4096}
	s.gss.slabs = []gssNodeSlab{{data: make([]gssNode, maxRetainedGSSNodes), reachMarks: make([]uint32, maxRetainedGSSNodes)}}
	s.gss.recomputeAllocatedBytes()
	s.heavyGrammar, _ = scratchKeyForLanguage(lang)
	fresh := &Parser{language: &Language{StateCount: 10, SymbolCount: 5, TokenCount: 3}}
	if !parserHeavySmallScratchFits(s, fresh, 4096) {
		t.Fatal("matching custom grammar cannot reuse heavy scratch")
	}
	if parserHeavySmallScratchFits(s, &Parser{language: &Language{StateCount: 11, SymbolCount: 5, TokenCount: 3}}, 4096) {
		t.Fatal("different grammar geometry inherited scratch")
	}
	if parserHeavySmallScratchFits(s, fresh, 100) {
		t.Fatal("strongly shrunken input inherited heavy scratch")
	}
	releaseParserScratchForParser(s, false, p)
	if got := acquireParserScratchForSource(4096, fresh); got != s {
		releaseParserScratchForParser(got, false, fresh)
		t.Skip("sync.Pool did not return the retained scratch")
	} else {
		releaseParserScratchForParser(got, false, fresh)
	}
}

func TestFreshCustomParsersReuseHeavyScratchWithoutRegrowth(t *testing.T) {
	lang := &Language{StateCount: 13, SymbolCount: 7, TokenCount: 4}
	seed := &parserScratch{sourceBytes: 4096}
	seed.gss.slabs = []gssNodeSlab{{data: make([]gssNode, maxRetainedGSSNodes), reachMarks: make([]uint32, maxRetainedGSSNodes)}}
	seed.gss.recomputeAllocatedBytes()
	releaseParserScratchForParser(seed, false, &Parser{language: lang})
	allocs := testing.AllocsPerRun(10, func() {
		parser := &Parser{language: lang}
		s := acquireParserScratchForSource(4096, parser)
		if s.gss.capacityNodes() != maxRetainedGSSNodes {
			t.Skip("sync.Pool did not retain the seeded heavy scratch")
		}
		releaseParserScratchForParser(s, false, parser)
	})
	if allocs > 1 {
		t.Fatalf("fresh custom parser scratch acquisition allocated %.2f times per run", allocs)
	}
}

func TestLargeScratchReusesRetainedCapacity(t *testing.T) {
	s := &parserScratch{largePool: true}
	s.gss.slabs = []gssNodeSlab{{data: make([]gssNode, maxRetainedGSSNodes)}}
	s.gss.recomputeAllocatedBytes()
	releaseParserScratch(s, false)
	reused := acquireParserScratchForSource(320<<10, &Parser{})
	defer releaseParserScratch(reused, false)
	if reused != s {
		t.Skip("sync.Pool did not return the retained scratch")
	}
	if reused.gss.capacityNodes() != maxRetainedGSSNodes {
		t.Fatalf("large scratch lost its reusable GSS: %d nodes", reused.gss.capacityNodes())
	}
}

func TestSmallParseReclaimsIdleLargeTransientScratch(t *testing.T) {
	s := &parserScratch{}
	s.gss.slabs = []gssNodeSlab{{data: make([]gssNode, 128)}}
	s.gss.recomputeAllocatedBytes()
	s.transientParents.allocatedBytes = 33 << 20
	s.transientChildren.allocatedBytes = 1 << 20
	reclaimLargeScratchForSmallParse(s)
	if s.transientParents.allocatedBytes != 0 || s.transientChildren.allocatedBytes != 0 {
		t.Fatal("large transient scratch was retained")
	}
	if s.gss.capacityNodes() != 128 {
		t.Fatal("reclamation dropped the reusable GSS")
	}
}

func TestParserScratchSizeClassesConcurrent(t *testing.T) {
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			parser := &Parser{language: &Language{}}
			for i := 0; i < 100; i++ {
				large := (worker+i)%2 == 0
				size := 4096
				if large {
					size = 320 << 10
				}
				s := acquireParserScratchForSource(size, parser)
				if s.largePool != large {
					errs <- "scratch returned from wrong size class"
					releaseParserScratchForParser(s, false, parser)
					return
				}
				releaseParserScratchForParser(s, false, parser)
			}
		}(worker)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
