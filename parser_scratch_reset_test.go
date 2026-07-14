package gotreesitter

import (
	"math"
	"testing"
)

func TestGLRMergeScratchResetInvalidatesEquivCacheByEpoch(t *testing.T) {
	var scratch glrMergeScratch
	childErrors := false
	scratch.childErrors = &childErrors
	scratch.beginEquivEpoch()

	a := &Node{symbol: 1, startByte: 1, endByte: 2, equivVersion: 1}
	b := &Node{symbol: 1, startByte: 1, endByte: 2, equivVersion: 1}
	storeNodeEquivCache(&scratch, a, b, 0, true)
	if got, ok := lookupNodeEquivCache(&scratch, a, b, 0); !ok || !got {
		t.Fatalf("lookupNodeEquivCache before reset = %v, %v; want true, true", got, ok)
	}

	before := scratch.equivEpoch
	scratch.reset()
	if scratch.childErrors != nil {
		t.Fatal("reset retained fresh-parse child-error proof pointer")
	}
	scratch.beginEquivEpoch()
	if scratch.equivEpoch == before {
		t.Fatalf("equiv epoch did not advance after reset: %d", before)
	}
	if _, ok := lookupNodeEquivCache(&scratch, a, b, 0); ok {
		t.Fatal("stale equivalence cache entry remained visible after reset")
	}
}

func TestGLRMergeScratchResetPreservesCleanZeroEpochAndRejectsReusedAddress(t *testing.T) {
	var nodes gssScratch
	old := nodes.allocNode(stackEntry{state: 1}, nil, 1)
	var pooled parserScratch
	scratch := &pooled.merge
	scratch.ensureMergeHotCaches()
	scratch.beginCleanZeroEpoch()
	storeCleanZeroFrontCache(scratch, old, true)
	scratch.cleanZeroCache = map[*gssNode]gssCleanZeroErrorCacheEntry{
		old: {resultEpoch: scratch.cleanZeroEpoch, clean: true},
	}
	scratch.cleanZeroStack = append(scratch.cleanZeroStack, old)
	scratch.cleanZeroVisited = append(scratch.cleanZeroVisited, old)

	epochBefore := scratch.cleanZeroEpoch
	scratch.reset()
	if scratch.cleanZeroEpoch != epochBefore {
		t.Fatalf("clean-zero epoch after reset = %d, want preserved %d", scratch.cleanZeroEpoch, epochBefore)
	}
	if got, ok := lookupCleanZeroFrontCache(scratch, old); !ok || !got {
		t.Fatalf("clean-zero front after reset = %v, %v; want retained true, true", got, ok)
	}
	if len(scratch.cleanZeroCache) != 0 {
		t.Fatalf("clean-zero map retained %d GSS pointers", len(scratch.cleanZeroCache))
	}
	if len(scratch.cleanZeroStack) != 0 || len(scratch.cleanZeroVisited) != 0 {
		t.Fatalf("clean-zero traversal scratch not reset: stack=%d visited=%d", len(scratch.cleanZeroStack), len(scratch.cleanZeroVisited))
	}
	if cap(scratch.cleanZeroStack) > 0 && scratch.cleanZeroStack[:cap(scratch.cleanZeroStack)][0] != nil {
		t.Fatal("clean-zero stack backing retained a GSS pointer")
	}
	if cap(scratch.cleanZeroVisited) > 0 && scratch.cleanZeroVisited[:cap(scratch.cleanZeroVisited)][0] != nil {
		t.Fatal("clean-zero visited backing retained a GSS pointer")
	}

	nodes.recycleForParse()
	reused := nodes.allocNode(stackEntry{state: 2}, nil, 1)
	if reused != old {
		t.Fatalf("recycled GSS address = %p, want %p", reused, old)
	}
	parser := NewParser(buildArithmeticLanguage())
	parser.configureParseScratch(&pooled, nil, nil, nil, arenaClassFull, true)
	if scratch.cleanZeroEpoch != epochBefore+1 {
		t.Fatalf("clean-zero epoch after acquisition = %d, want %d", scratch.cleanZeroEpoch, epochBefore+1)
	}
	if _, ok := lookupCleanZeroFrontCache(scratch, reused); ok {
		t.Fatal("clean-zero front hit after a GSS address was reused in a new parse epoch")
	}
}

func TestGLRMergeScratchCleanZeroEpochWrapClearsFront(t *testing.T) {
	var nodes gssScratch
	node := nodes.allocNode(stackEntry{state: 1}, nil, 1)
	var scratch glrMergeScratch
	scratch.ensureMergeHotCaches()
	scratch.cleanZeroEpoch = ^uint32(0)
	storeCleanZeroFrontCache(&scratch, node, true)
	scratch.cleanZeroCache = map[*gssNode]gssCleanZeroErrorCacheEntry{
		node: {resultEpoch: scratch.cleanZeroEpoch, clean: true},
	}

	scratch.beginCleanZeroEpoch()
	if scratch.cleanZeroEpoch != 1 {
		t.Fatalf("clean-zero epoch after wrap = %d, want 1", scratch.cleanZeroEpoch)
	}
	if len(scratch.cleanZeroCache) != 0 {
		t.Fatalf("clean-zero map retained %d entries across wrap", len(scratch.cleanZeroCache))
	}
	if _, ok := lookupCleanZeroFrontCache(&scratch, node); ok {
		t.Fatal("clean-zero front entry survived epoch wrap")
	}
	for i, entry := range scratch.cleanZeroFront {
		if entry != (glrCleanZeroFrontCacheEntry{}) {
			t.Fatalf("clean-zero front slot %d after wrap = %#v, want zero", i, entry)
		}
	}
}

func TestGLREntryScratchResetClearsReservedWrittenRange(t *testing.T) {
	var scratch glrEntryScratch
	entries := scratch.allocWithCap(1, 8)
	node := &Node{symbol: 1}
	entries = entries[:cap(entries)]
	entries[len(entries)-1] = newStackEntryNode(7, node)
	if got := scratch.peakEntriesUsed(); got != 8 {
		t.Fatalf("peak entries before reset = %d, want 8", got)
	}

	scratch.reset()
	if got := scratch.peakEntriesUsed(); got != 0 {
		t.Fatalf("peak entries after reset = %d, want 0", got)
	}
	if len(scratch.slabs) == 0 {
		t.Fatal("expected retained entry slab")
	}
	for i, entry := range scratch.slabs[0].data[:8] {
		if stackEntryNode(entry) != nil || entry.state != 0 {
			t.Fatalf("entry slab slot %d after reset = %#v, want zero", i, entry)
		}
	}
}

func TestInitialParseStackReservationMatchesCurrentFullParseSize(t *testing.T) {
	var scratch parserScratch
	scratch.entries.ensureInitialCap(maxFullParseEntryScratchEntries)
	parser := &Parser{language: &Language{InitialState: 1}}

	tinySourceLen := len("node\n")
	if got := len(scratch.entries.slabs[0].data); got != maxFullParseEntryScratchEntries {
		t.Fatalf("retained full-parse physical slab = %d, want %d", got, maxFullParseEntryScratchEntries)
	}
	stacks, _ := parser.newInitialParseStacks(&scratch, nil, nil, tinySourceLen)
	wantTiny := parseFullEntryScratchReservation(tinySourceLen)
	wantTinyScaled := tinySourceLen * fullParseEntryScratchEntriesPerSourceByte
	if wantTiny != wantTinyScaled {
		t.Fatalf("tiny full-parse logical reservation = %d, want source-scaled %d", wantTiny, wantTinyScaled)
	}
	if wantTiny >= defaultStackEntrySlabCap {
		t.Fatalf("tiny full-parse logical reservation = %d, want less than physical slab %d", wantTiny, defaultStackEntrySlabCap)
	}
	if got := cap(stacks[0].entries); got != wantTiny {
		t.Fatalf("tiny full-parse reservation = %d, want %d", got, wantTiny)
	}
	if got := scratch.entries.peakEntriesUsed(); got != wantTiny {
		t.Fatalf("tiny full-parse reserved range = %d, want %d", got, wantTiny)
	}

	scratch.entries.reset()
	largeSourceLen := 2 * 1024 * 1024
	stacks, _ = parser.newInitialParseStacks(&scratch, nil, nil, largeSourceLen)
	wantLarge := parseFullEntryScratchReservation(largeSourceLen)
	if got := cap(stacks[0].entries); got != wantLarge {
		t.Fatalf("large full-parse reservation = %d, want %d", got, wantLarge)
	}

	scratch.entries.reset()
	reuse := &reuseCursor{}
	stacks, _ = parser.newInitialParseStacks(&scratch, reuse, nil, largeSourceLen)
	if got := cap(stacks[0].entries); got != defaultStackEntrySlabCap {
		t.Fatalf("incremental reservation = %d, want %d", got, defaultStackEntrySlabCap)
	}
}

func TestInitialParseStackTinyReservationGrowsAndResetsSafely(t *testing.T) {
	var scratch parserScratch
	parser := &Parser{language: &Language{InitialState: 1}}
	sourceLen := len("node\n")
	scratch.entries.ensureInitialCap(parseFullEntryScratchCapacity(sourceLen))
	if got := len(scratch.entries.slabs[0].data); got != defaultStackEntrySlabCap {
		t.Fatalf("fresh tiny full-parse physical slab = %d, want %d", got, defaultStackEntrySlabCap)
	}

	stacks, _ := parser.newInitialParseStacks(&scratch, nil, nil, sourceLen)
	stack := &stacks[0]
	reserved := cap(stack.entries)
	for len(stack.entries) <= reserved {
		stack.pushEntry(stackEntry{state: StateID(len(stack.entries) + 1)}, &scratch.entries, nil)
	}
	if got := len(stack.entries); got != reserved+1 {
		t.Fatalf("grown stack length = %d, want %d", got, reserved+1)
	}
	if got := cap(stack.entries); got < reserved+1 {
		t.Fatalf("grown stack capacity = %d, want at least %d", got, reserved+1)
	}
	for i, entry := range stack.entries {
		if want := StateID(i + 1); entry.state != want {
			t.Fatalf("grown stack entry %d state = %d, want %d", i, entry.state, want)
		}
	}

	scratch.entries.reset()
	for slabIndex := range scratch.entries.slabs {
		for entryIndex, entry := range scratch.entries.slabs[slabIndex].data {
			if entry != (stackEntry{}) {
				t.Fatalf("entry scratch slab %d slot %d after reset = %#v, want zero", slabIndex, entryIndex, entry)
			}
		}
	}
}

func TestFullEntryScratchReservationNeverExceedsPhysicalCapacity(t *testing.T) {
	threshold := maxFullParseEntryScratchEntries / fullParseEntryScratchEntriesPerSourceByte
	for _, tc := range []struct {
		sourceLen    int
		wantReserved int
		wantPhysical int
	}{
		{sourceLen: -1, wantReserved: 8, wantPhysical: defaultStackEntrySlabCap},
		{sourceLen: 0, wantReserved: 8, wantPhysical: defaultStackEntrySlabCap},
		{sourceLen: 1, wantReserved: fullParseEntryScratchEntriesPerSourceByte, wantPhysical: defaultStackEntrySlabCap},
		{sourceLen: len("node\n"), wantReserved: len("node\n") * fullParseEntryScratchEntriesPerSourceByte, wantPhysical: defaultStackEntrySlabCap},
		{sourceLen: threshold - 1, wantReserved: (threshold - 1) * fullParseEntryScratchEntriesPerSourceByte, wantPhysical: (threshold - 1) * fullParseEntryScratchEntriesPerSourceByte},
		{sourceLen: threshold, wantReserved: threshold * fullParseEntryScratchEntriesPerSourceByte, wantPhysical: threshold * fullParseEntryScratchEntriesPerSourceByte},
		{sourceLen: threshold + 1, wantReserved: maxFullParseEntryScratchEntries, wantPhysical: maxFullParseEntryScratchEntries},
		{sourceLen: 2 * 1024 * 1024, wantReserved: maxFullParseEntryScratchEntries, wantPhysical: maxFullParseEntryScratchEntries},
		{sourceLen: math.MaxInt, wantReserved: maxFullParseEntryScratchEntries, wantPhysical: maxFullParseEntryScratchEntries},
	} {
		reserved := parseFullEntryScratchReservation(tc.sourceLen)
		physical := parseFullEntryScratchCapacity(tc.sourceLen)
		if reserved != tc.wantReserved {
			t.Fatalf("source length %d reservation = %d, want %d", tc.sourceLen, reserved, tc.wantReserved)
		}
		if physical != tc.wantPhysical {
			t.Fatalf("source length %d physical capacity = %d, want %d", tc.sourceLen, physical, tc.wantPhysical)
		}
		if reserved > physical {
			t.Fatalf("source length %d reservation = %d, exceeds physical capacity %d", tc.sourceLen, reserved, physical)
		}
	}
}

func BenchmarkInitialParseStackReservationAfterLarge(b *testing.B) {
	var scratch parserScratch
	scratch.entries.ensureInitialCap(64 * 1024)
	parser := &Parser{language: &Language{InitialState: 1}}
	tinySourceLen := len("{}")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stacks, _ := parser.newInitialParseStacks(&scratch, nil, nil, tinySourceLen)
		if len(stacks) != 1 {
			b.Fatalf("initial stack count = %d, want 1", len(stacks))
		}
		scratch.entries.reset()
	}
}

func TestGSSScratchResetClearsWrittenRange(t *testing.T) {
	var scratch gssScratch
	node := &Node{symbol: 1}
	stack := newGSSStack(1, &scratch)
	stack.push(2, node, &scratch)

	scratch.reset()
	if len(scratch.slabs) == 0 {
		t.Fatal("expected retained GSS slab")
	}
	for i, n := range scratch.slabs[0].data[:2] {
		if stackEntryNode(n.entry) != nil || n.prev != nil || n.depth != 0 || n.hash != 0 {
			t.Fatalf("GSS slab slot %d after reset = %#v, want zero", i, n)
		}
	}
}
