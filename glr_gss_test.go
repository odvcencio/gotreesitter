package gotreesitter

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"
	"unsafe"
)

var (
	benchmarkPendingStackBuffer      []glrStack
	benchmarkResetPendingStackBuffer = func(stacks []glrStack) []glrStack {
		return resetPendingStackBuffer(stacks, true)
	}
)

func gssNodeWithExtraLinks(node gssNode, links ...gssMainLink) *gssNode {
	n := &node
	for _, link := range links {
		n.appendExtraLink(link)
	}
	return n
}

func TestGSSNodeLayoutSizeBudget(t *testing.T) {
	want := uintptr(64)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 52
	}
	if got := unsafe.Sizeof(gssNode{}); got != want {
		t.Fatalf("gssNode size = %d bytes, want %d", got, want)
	}
}

func TestGSSPackedLinkCapacityIsTrackedAndReleasedOnRecycle(t *testing.T) {
	var owner gssScratch
	base := owner.allocNode(stackEntry{state: 1}, nil, 1)
	head := owner.allocNode(newStackEntryNode(2, &Node{symbol: 1}), base, 2)
	merge := glrMergeScratch{gssOwner: &owner}
	baseline := owner.allocatedBytes

	for i := 0; i < maxMainLinkCount-1; i++ {
		prev := owner.allocNode(stackEntry{state: StateID(10 + i)}, nil, 1)
		entry := newStackEntryNode(2, &Node{symbol: Symbol(10 + i)})
		if !gssMainAddLinkSeenMutate(&merge, head, prev, entry, make(map[gssMergePair]bool)) {
			t.Fatalf("append packed link %d failed", i)
		}
		wantCap := 1
		switch {
		case i >= 4:
			wantCap = maxMainLinkCount - 1
		case i >= 2:
			wantCap = 4
		case i >= 1:
			wantCap = 2
		}
		if got := int(head.extraLinkCap); got != wantCap {
			t.Fatalf("packed link capacity after append %d = %d, want %d", i, got, wantCap)
		}
		wantBytes := baseline + gssMainLinkBytesForCap(wantCap)
		if got := owner.allocatedBytes; got != wantBytes {
			t.Fatalf("tracked bytes after append %d = %d, want %d", i, got, wantBytes)
		}
		if got := owner.packedLinkBytes; got != gssMainLinkBytesForCap(wantCap) {
			t.Fatalf("packed link bytes after append %d = %d, want %d", i, got, gssMainLinkBytesForCap(wantCap))
		}
	}
	owner.ensureReachMarks()
	retainedBaseline := baseline
	for i := range owner.slabs {
		retainedBaseline += gssReachMarkBytesForCap(len(owner.slabs[i].reachMarks))
	}
	if got, want := owner.allocatedBytes, retainedBaseline+gssMainLinkBytesForCap(maxMainLinkCount-1); got != want {
		t.Fatalf("tracked bytes after reach-mark recompute = %d, want %d", got, want)
	}

	owner.recycleForParse()
	if got := owner.allocatedBytes; got != retainedBaseline {
		t.Fatalf("tracked bytes after recycle = %d, want retained baseline %d", got, retainedBaseline)
	}
	if owner.packedLinkBytes != 0 {
		t.Fatalf("packed link bytes after recycle = %d, want 0", owner.packedLinkBytes)
	}
}

func TestGSSPackedLinkCertifiedPolicyBillsSevenSlotCapacity(t *testing.T) {
	var owner gssScratch
	prevs := make([]*gssNode, maxCMainLinkCount)
	for i := range prevs {
		prevs[i] = owner.allocNode(stackEntry{state: StateID(10 + i)}, nil, 1)
	}
	head := owner.allocNode(newStackEntryNode(2, &Node{symbol: 1}), prevs[0], 2)
	baseline := owner.allocatedBytes
	merge := glrMergeScratch{
		language: &Language{CompactPackedGSSVersionOrderCertified: true},
		gssOwner: &owner,
	}

	for i := 1; i < maxCMainLinkCount; i++ {
		entry := newStackEntryNode(2, &Node{symbol: Symbol(10 + i)})
		if !gssMainAddLinkSeenMutate(&merge, head, prevs[i], entry, make(map[gssMergePair]bool)) {
			t.Fatalf("append certified packed link %d failed", i)
		}
	}

	wantBytes := gssMainLinkBytesForCap(maxCMainLinkCount - 1)
	if got := int(head.extraLinkCap); got != maxCMainLinkCount-1 {
		t.Fatalf("certified packed link capacity = %d, want %d", got, maxCMainLinkCount-1)
	}
	if got := owner.packedLinkBytes; got != wantBytes {
		t.Fatalf("certified packed link bytes = %d, want %d", got, wantBytes)
	}
	if got := owner.allocatedBytes; got != baseline+wantBytes {
		t.Fatalf("certified allocated bytes = %d, want %d", got, baseline+wantBytes)
	}
}

func TestGSSPackedLinkRecycleRepairsCounterUnderflow(t *testing.T) {
	var owner gssScratch
	base := owner.allocNode(stackEntry{state: 1}, nil, 1)
	head := owner.allocNode(newStackEntryNode(2, &Node{symbol: 1}), base, 2)
	prev := owner.allocNode(stackEntry{state: 3}, nil, 1)
	merge := glrMergeScratch{gssOwner: &owner}
	entry := newStackEntryNode(4, &Node{symbol: 2})
	if !gssMainAddLinkSeenMutate(&merge, head, prev, entry, make(map[gssMergePair]bool)) {
		t.Fatal("packed link append failed")
	}
	retainedBytes := owner.allocatedBytes - owner.packedLinkBytes
	owner.allocatedBytes = owner.packedLinkBytes - 1

	owner.recycleForParse()

	if got := owner.allocatedBytes; got != retainedBytes {
		t.Fatalf("tracked bytes after underflow repair = %d, want %d", got, retainedBytes)
	}
	if owner.packedLinkBytes != 0 {
		t.Fatalf("packed link bytes after underflow repair = %d, want 0", owner.packedLinkBytes)
	}
}

func TestGSSPackedLinkCapacityIsReleasedOnReset(t *testing.T) {
	var owner gssScratch
	base := owner.allocNode(stackEntry{state: 1}, nil, 1)
	head := owner.allocNode(newStackEntryNode(2, &Node{symbol: 1}), base, 2)
	prev := owner.allocNode(stackEntry{state: 3}, nil, 1)
	merge := glrMergeScratch{gssOwner: &owner}
	entry := newStackEntryNode(4, &Node{symbol: 2})
	if !gssMainAddLinkSeenMutate(&merge, head, prev, entry, make(map[gssMergePair]bool)) {
		t.Fatal("packed link append failed")
	}
	retainedBytes := owner.allocatedBytes - owner.packedLinkBytes

	owner.reset()

	if got := owner.allocatedBytes; got != retainedBytes {
		t.Fatalf("tracked bytes after reset = %d, want %d", got, retainedBytes)
	}
	if owner.packedLinkBytes != 0 {
		t.Fatalf("packed link bytes after reset = %d, want 0", owner.packedLinkBytes)
	}
	if head.extraLinks != nil || head.extraLinkCount != 0 || head.extraLinkCap != 0 {
		t.Fatalf("reset node retained packed links: %+v", *head)
	}
}

func TestGSSPackedLinkCounterIsClearedByEmptyReset(t *testing.T) {
	owner := gssScratch{
		allocatedBytes:  gssMainLinkBytesForCap(1),
		packedLinkBytes: gssMainLinkBytesForCap(1),
	}

	owner.reset()

	if owner.allocatedBytes != 0 || owner.packedLinkBytes != 0 {
		t.Fatalf("empty reset retained packed-link accounting: allocated=%d packed=%d", owner.allocatedBytes, owner.packedLinkBytes)
	}
}

func TestGSSPackedLinkCapacityCountsTowardParserScratchBudget(t *testing.T) {
	var scratch parserScratch
	base := scratch.gss.allocNode(stackEntry{state: 1}, nil, 1)
	head := scratch.gss.allocNode(newStackEntryNode(2, &Node{symbol: 1}), base, 2)
	prev := scratch.gss.allocNode(stackEntry{state: 3}, nil, 1)
	scratch.merge.gssOwner = &scratch.gss
	scratch.setBudget(gssMainLinkBytesForCap(1))

	entry := newStackEntryNode(4, &Node{symbol: 2})
	if !gssMainAddLinkSeenMutate(&scratch.merge, head, prev, entry, make(map[gssMergePair]bool)) {
		t.Fatal("packed link append failed")
	}
	if !scratch.budgetExhausted() {
		t.Fatal("parser scratch budget did not observe packed link capacity")
	}
	var stats ParseRuntime
	if !captureParseScratchStats(&stats, &scratch, nil, nil) {
		t.Fatal("captureParseScratchStats returned false")
	}
	if got, want := stats.GSSBytesAllocated, scratch.gssBaselineBytes+gssMainLinkBytesForCap(1); got != want {
		t.Fatalf("runtime GSS bytes = %d, want %d", got, want)
	}
}

func TestCRecoverSummaryArenaCrossesChunkBoundary(t *testing.T) {
	parser := &Parser{}
	var scratch gssScratch
	entries := recoverySummaryTestEntries(1)
	for i := 0; i < cRecoverSummaryChunkInitialEntries; i++ {
		if _, reason := parser.cRecordSummaryWithScratch(entries, &scratch, nil); reason != ParseStopNone {
			t.Fatalf("summary %d stopped: %s", i, reason)
		}
	}
	if len(scratch.summaryChunks) != 1 || scratch.summaryChunks[0].used != cRecoverSummaryChunkInitialEntries {
		t.Fatalf("first summary chunk = %#v, want full first chunk", scratch.summaryChunks)
	}
	if _, reason := parser.cRecordSummaryWithScratch(entries, &scratch, nil); reason != ParseStopNone {
		t.Fatalf("boundary summary stopped: %s", reason)
	}
	if len(scratch.summaryChunks) != 2 || scratch.summaryChunks[1].used != 1 {
		t.Fatalf("summary arena did not cross chunk boundary: %#v", scratch.summaryChunks)
	}
	firstSlot := &scratch.summaryChunks[0].data[0]
	scratch.reset()
	reused, reason := parser.cRecordSummaryWithScratch(entries, &scratch, nil)
	if reason != ParseStopNone {
		t.Fatalf("retained summary stopped: %s", reason)
	}
	if len(reused) != 1 || &reused[0] != firstSlot {
		t.Fatal("retained summary arena did not restart at its first reusable chunk")
	}
}

func TestCRecoverSummaryArenaReusesMixedRetainedChunks(t *testing.T) {
	var scratch gssScratch
	for _, count := range []int{cRecoverSummaryChunkInitialEntries, 2 * cRecoverSummaryChunkInitialEntries, 4 * cRecoverSummaryChunkInitialEntries} {
		if _, reason := scratch.allocRecoverySummary(count); reason != ParseStopNone {
			t.Fatalf("initial chunk allocation %d stopped: %s", count, reason)
		}
	}
	if len(scratch.summaryChunks) != 3 {
		t.Fatalf("initial chunk count = %d, want 3", len(scratch.summaryChunks))
	}
	first := &scratch.summaryChunks[0].data[0]
	third := &scratch.summaryChunks[2].data[0]
	scratch.reset()

	large, reason := scratch.allocRecoverySummary(4 * cRecoverSummaryChunkInitialEntries)
	if reason != ParseStopNone {
		t.Fatalf("retained large allocation stopped: %s", reason)
	}
	if &large[0] != third {
		t.Fatal("large allocation did not reuse the retained 1024-entry chunk")
	}
	small, reason := scratch.allocRecoverySummary(1)
	if reason != ParseStopNone {
		t.Fatalf("retained small allocation stopped: %s", reason)
	}
	if &small[0] != first {
		t.Fatal("small allocation did not wrap to the retained 256-entry chunk")
	}
	if len(scratch.summaryChunks) != 3 || scratch.summaryChunks[0].used != 1 || scratch.summaryChunks[2].used != 4*cRecoverSummaryChunkInitialEntries {
		t.Fatalf("mixed retained chunks changed unexpectedly: %#v", scratch.summaryChunks)
	}
}

func TestCRecoverSummaryArenaResetCapsRetainedChunks(t *testing.T) {
	backing := make([]gssSummaryChunk, maxRetainedGSSSummaryChunks+4)
	for i := range backing {
		backing[i].data = make([]cStackSummaryEntry, 1)
		backing[i].used = 1
	}
	var scratch gssScratch
	scratch.summaryChunks = backing
	scratch.recomputeAllocatedBytes()
	scratch.reset()
	if len(scratch.summaryChunks) != maxRetainedGSSSummaryChunks || cap(scratch.summaryChunks) != maxRetainedGSSSummaryChunks {
		t.Fatalf("retained summary descriptors = len %d cap %d, want %d", len(scratch.summaryChunks), cap(scratch.summaryChunks), maxRetainedGSSSummaryChunks)
	}
	for i := 0; i < maxRetainedGSSSummaryChunks; i++ {
		if scratch.summaryChunks[i].data == nil || scratch.summaryChunks[i].used != 0 {
			t.Fatalf("retained chunk %d = %#v, want cleared storage", i, scratch.summaryChunks[i])
		}
	}
	for i := maxRetainedGSSSummaryChunks; i < len(backing); i++ {
		if backing[i].data != nil {
			t.Fatalf("dropped chunk %d retained storage", i)
		}
	}
}

func TestCRecoverSummaryArenaAllocatesOversizedSummary(t *testing.T) {
	parser := &Parser{}
	var scratch gssScratch
	entries := recoverySummaryTestEntries(cRecoverSummaryChunkMaxEntries + 1)
	summary, reason := parser.cRecordSummaryWithScratch(entries, &scratch, nil)
	if reason != ParseStopNone {
		t.Fatalf("oversized summary stopped: %s", reason)
	}
	if len(scratch.summaryChunks) != 1 || cap(scratch.summaryChunks[0].data) != len(entries) {
		t.Fatalf("oversized summary chunk = %#v, want exact capacity %d", scratch.summaryChunks, len(entries))
	}
	if cap(summary) != len(summary) {
		t.Fatalf("summary capacity = %d, want fenced capacity %d", cap(summary), len(summary))
	}
}

func TestCRecoverSummaryArenaStopsAtMemoryBudget(t *testing.T) {
	parser := &Parser{}
	scratch := &parserScratch{}
	scratch.setBudget(int64(unsafe.Sizeof(cStackSummaryEntry{}))*cRecoverSummaryChunkInitialEntries - 1)
	entries := recoverySummaryTestEntries(1)
	summary, reason := parser.cRecordSummaryWithScratch(entries, &scratch.gss, nil)
	if reason != ParseStopMemoryBudget {
		t.Fatalf("summary stop reason = %s, want %s", reason, ParseStopMemoryBudget)
	}
	if summary != nil || len(scratch.gss.summaryChunks) != 0 {
		t.Fatalf("budget stop allocated or truncated summary: summary=%v chunks=%d", summary, len(scratch.gss.summaryChunks))
	}
	scratch.clearBudget()
}

func TestGSSStackPushCloneAndTruncate(t *testing.T) {
	var scratch gssScratch
	base := newGSSStack(1, &scratch)
	if base.len() != 1 {
		t.Fatalf("base len = %d, want 1", base.len())
	}

	clone := base.clone()
	base.push(2, nil, &scratch)
	base.push(3, nil, &scratch)

	if base.len() != 3 {
		t.Fatalf("base len after pushes = %d, want 3", base.len())
	}
	if clone.len() != 1 {
		t.Fatalf("clone len changed = %d, want 1", clone.len())
	}
	if base.top().state != 3 {
		t.Fatalf("base top state = %d, want 3", base.top().state)
	}

	ok := base.truncate(2)
	if !ok {
		t.Fatal("truncate(2) = false, want true")
	}
	if got := base.top().state; got != 2 {
		t.Fatalf("top after truncate = %d, want 2", got)
	}
}

func TestGSSStackMaterializeAndByteOffset(t *testing.T) {
	var scratch gssScratch
	n1 := &Node{endByte: 5}
	n2 := &Node{endByte: 9}

	var s gssStack
	s.push(1, nil, &scratch)
	s.push(2, n1, &scratch)
	s.push(3, nil, &scratch)
	s.push(4, n2, &scratch)

	got := s.materialize(nil)
	if len(got) != 4 {
		t.Fatalf("materialized len = %d, want 4", len(got))
	}
	if got[0].state != 1 || got[1].state != 2 || got[2].state != 3 || got[3].state != 4 {
		t.Fatalf("unexpected materialized states: %+v", got)
	}

	if off := s.byteOffset(); off != 9 {
		t.Fatalf("byteOffset = %d, want 9", off)
	}

	s.truncate(3)
	if off := s.byteOffset(); off != 5 {
		t.Fatalf("byteOffset after truncate = %d, want 5", off)
	}
}

func TestGLRStackToGSS(t *testing.T) {
	var gScratch gssScratch
	var entryScratch glrEntryScratch
	s := newGLRStackWithScratch(1, &entryScratch)
	s.push(2, nil, &entryScratch, &gScratch)
	s.push(3, nil, &entryScratch, &gScratch)

	gs := s.toGSS(&gScratch)
	mat := gs.materialize(nil)
	want := s.ensureEntries(&entryScratch)
	if len(mat) != len(want) {
		t.Fatalf("materialized len = %d, want %d", len(mat), len(want))
	}
	for i := range mat {
		if mat[i].state != want[i].state {
			t.Fatalf("state[%d] = %d, want %d", i, mat[i].state, want[i].state)
		}
	}
}

func TestConflictForkBaseSharesOriginalGSSHead(t *testing.T) {
	var scratch gssScratch
	original := glrStack{
		entries:      []stackEntry{{state: 1}, {state: 2}, {state: 3}},
		cacheEntries: true,
	}

	base := original.conflictForkBase(&scratch)
	first := base.cloneWithScratch(&scratch)
	second := base.cloneWithScratch(&scratch)

	if original.gss.head == nil {
		t.Fatal("original GSS head is nil")
	}
	if base.gss.head != original.gss.head || first.gss.head != original.gss.head || second.gss.head != original.gss.head {
		t.Fatalf("fork heads differ: original=%p base=%p first=%p second=%p", original.gss.head, base.gss.head, first.gss.head, second.gss.head)
	}
	if original.entries == nil {
		t.Fatal("original dense entries were discarded during promotion")
	}
	if first.entries != nil || second.entries != nil {
		t.Fatalf("fork clones retained dense entries: first=%d second=%d", len(first.entries), len(second.entries))
	}
}

func TestConflictForkBaseReconvergenceRetainsDivergentLinks(t *testing.T) {
	var scratch gssScratch
	original := glrStack{
		entries:      []stackEntry{{state: 1}, {state: 2}},
		cacheEntries: true,
	}
	base := original.conflictForkBase(&scratch)
	fork := base.cloneWithScratch(&scratch)
	shared := original.gss.head

	original.push(7, &Node{symbol: 11, startByte: 0, endByte: 1, flags: nodeFlagNamed}, nil, &scratch)
	fork.push(7, &Node{symbol: 12, startByte: 0, endByte: 1, flags: nodeFlagNamed}, nil, &scratch)
	if original.gss.head.prev != shared || fork.gss.head.prev != shared {
		t.Fatalf("divergent heads lost shared predecessor: original=%p fork=%p shared=%p", original.gss.head.prev, fork.gss.head.prev, shared)
	}

	if !gssMainMerge(&original, &fork) {
		t.Fatal("reconverged stacks did not merge")
	}
	if got := original.gss.head.linkCount(); got != 2 {
		t.Fatalf("reconverged link count = %d, want 2", got)
	}
	seen := make(map[Symbol]bool, 2)
	for i := 0; i < original.gss.head.linkCount(); i++ {
		prev, entry := original.gss.head.link(i)
		if prev != shared {
			t.Fatalf("link %d predecessor = %p, want shared head %p", i, prev, shared)
		}
		seen[stackEntryNodeSymbol(entry)] = true
	}
	for _, symbol := range []Symbol{11, 12} {
		if !seen[symbol] {
			t.Fatalf("reconverged links lost symbol %d", symbol)
		}
	}
}

func TestConflictForkBaseKeepsDenseAndGSSMainPathSynchronized(t *testing.T) {
	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	original := newGLRStackWithScratch(1, &entryScratch)
	original.push(2, &Node{symbol: 10, endByte: 2}, &entryScratch, &gssScratch)
	original.push(3, &Node{symbol: 11, endByte: 4}, &entryScratch, &gssScratch)
	_ = original.conflictForkBase(&gssScratch)

	assertDenseGSSMainPathEqual(t, &original)
	original.push(4, &Node{symbol: 12, endByte: 6}, &entryScratch, &gssScratch)
	assertDenseGSSMainPathEqual(t, &original)
	if !original.truncateBeforePush(2) {
		t.Fatal("truncateBeforePush(2) = false")
	}
	original.push(5, &Node{symbol: 13, endByte: 7}, &entryScratch, &gssScratch)
	assertDenseGSSMainPathEqual(t, &original)
	if !original.truncate(2) {
		t.Fatal("truncate(2) = false")
	}
	assertDenseGSSMainPathEqual(t, &original)
}

func TestConflictForkBaseAllocationIsDepthBounded(t *testing.T) {
	const (
		depth     = 64
		forkCount = 32
	)
	entries := make([]stackEntry, depth)
	for i := range entries {
		entries[i].state = StateID(i + 1)
	}
	original := glrStack{entries: entries, cacheEntries: true}
	var scratch gssScratch
	usedBefore := scratch.usedTotal

	base := original.conflictForkBase(&scratch)
	if got := scratch.usedTotal - usedBefore; got != depth {
		t.Fatalf("promotion GSS nodes = %d, want depth %d", got, depth)
	}
	usedAfterPromotion := scratch.usedTotal
	bytesAfterPromotion := scratch.allocatedBytes
	for i := 0; i < forkCount; i++ {
		fork := base.cloneWithScratch(&scratch)
		if fork.gss.head != original.gss.head {
			t.Fatalf("fork %d head = %p, want %p", i, fork.gss.head, original.gss.head)
		}
	}
	if scratch.usedTotal != usedAfterPromotion {
		t.Fatalf("clone GSS nodes changed from %d to %d", usedAfterPromotion, scratch.usedTotal)
	}
	if scratch.allocatedBytes != bytesAfterPromotion {
		t.Fatalf("clone GSS bytes changed from %d to %d", bytesAfterPromotion, scratch.allocatedBytes)
	}
}

func assertDenseGSSMainPathEqual(t *testing.T, stack *glrStack) {
	t.Helper()
	if stack.gss.head == nil || stack.entries == nil {
		t.Fatalf("stack lacks synchronized forms: head=%p entries=%d", stack.gss.head, len(stack.entries))
	}
	materialized := stack.gss.materialize(nil)
	if len(materialized) != len(stack.entries) {
		t.Fatalf("stack depth differs: dense=%d GSS=%d", len(stack.entries), len(materialized))
	}
	for i := range materialized {
		if materialized[i] != stack.entries[i] {
			t.Fatalf("stack entry %d differs: dense=%+v GSS=%+v", i, stack.entries[i], materialized[i])
		}
	}
	if got, want := stack.byteOffset, stackByteOffset(stack.entries); got != want {
		t.Fatalf("byte offset = %d, want %d", got, want)
	}
}

func TestGSSStackMaterializePanicsOnCorruptDepth(t *testing.T) {
	head := &gssNode{entry: stackEntry{state: 2}, depth: 3}
	head.prev = &gssNode{entry: stackEntry{state: 1}, depth: 1}
	s := gssStack{head: head}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on corrupt GSS depth metadata")
		}
	}()
	_ = s.materialize(nil)
}

func TestGSSNodeHashComputedLazilyForSingleStackNodes(t *testing.T) {
	var scratch gssScratch
	scratch.singleStackMode = true

	n1 := &Node{symbol: 1, startByte: 0, endByte: 1, parseState: 5}
	n2 := &Node{symbol: 2, startByte: 1, endByte: 3, parseState: 6}

	var s gssStack
	s.push(1, nil, &scratch)
	s.push(2, n1, &scratch)
	s.push(3, n2, &scratch)

	if got := s.head.hash; got != 0 {
		t.Fatalf("head hash before demand = %d, want 0", got)
	}

	got := gssNodeHash(s.head)
	if got == 0 {
		t.Fatal("expected lazy hash to compute non-zero value")
	}
	if s.head.hash != got {
		t.Fatalf("cached head hash = %d, want %d", s.head.hash, got)
	}

	entries := s.materialize(nil)
	want := gssHashSeed
	for i := range entries {
		want = gssEntryHash(want, entries[i])
	}
	if got != want {
		t.Fatalf("lazy hash = %d, want %d", got, want)
	}
}

func TestGSSNodeHashMatchesAcrossInlineBoundary(t *testing.T) {
	for _, length := range []int{1, 31, 32, 33, 64} {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			nodes := make([]gssNode, length)
			want := uint64(gssHashSeed)
			for i := range nodes {
				nodes[i].entry.state = StateID(i + 1)
				nodes[i].depth = uint32(i + 1)
				if i > 0 {
					nodes[i].prev = &nodes[i-1]
				}
				want = gssEntryHash(want, nodes[i].entry)
				if want == 0 {
					want = 1
				}
			}
			if got := gssNodeHash(&nodes[len(nodes)-1]); got != want {
				t.Fatalf("hash = %d, want %d", got, want)
			}
			for i := range nodes {
				if nodes[i].hash == 0 {
					t.Fatalf("node %d hash remains zero", i)
				}
			}
		})
	}
}

func TestGSSNodeHashInlineWalkDoesNotAllocate(t *testing.T) {
	var nodes [32]gssNode
	for i := range nodes {
		nodes[i].entry.state = StateID(i + 1)
		nodes[i].depth = uint32(i + 1)
		if i > 0 {
			nodes[i].prev = &nodes[i-1]
		}
	}

	allocs := testing.AllocsPerRun(100, func() {
		for i := range nodes {
			nodes[i].hash = 0
		}
		if got := gssNodeHash(&nodes[len(nodes)-1]); got == 0 {
			t.Fatal("inline hash walk returned zero")
		}
	})
	if allocs != 0 {
		t.Fatalf("inline hash walk allocated %.0f objects, want 0", allocs)
	}
}

// TestResetGSSNodeHashPendingForPoolClearsExactBacking proves the slice
// clearing contract without depending on sync.Pool to return a specific item.
func TestResetGSSNodeHashPendingForPoolClearsExactBacking(t *testing.T) {
	const used = 64
	backing := make([]*gssNode, 96)
	for i := 0; i < used; i++ {
		backing[i] = &gssNode{depth: uint32(i + 1)}
	}
	pending := backing[:used]

	if !resetGSSNodeHashPendingForPool(&pending) {
		t.Fatal("ordinary hash walk buffer was rejected from the pool")
	}
	if len(pending) != 0 {
		t.Fatalf("released hash walk length = %d, want 0", len(pending))
	}
	for i, node := range pending[:cap(pending)] {
		if node != nil {
			t.Fatalf("released hash walk backing slot %d retained a node pointer", i)
		}
	}
}

func TestGSSEntryHashMatchesAccessorSemantics(t *testing.T) {
	node := &Node{
		children:      []*Node{{symbol: 20, startByte: 1, endByte: 2, preGotoState: 8, fieldMetadata: &nodeFieldMetadata{ids: []FieldID{3}}, flags: nodeFlagNamed}},
		fieldMetadata: &nodeFieldMetadata{ids: []FieldID{2}},
		symbol:        10,
		startByte:     1,
		endByte:       3,
		parseState:    4,
		preGotoState:  14,
		productionID:  5,
		flags:         nodeFlagNamed | nodeFlagHasError,
	}
	noTree := &noTreeNode{
		symbol:       11,
		startByte:    2,
		endByte:      5,
		parseState:   6,
		preGotoState: 16,
		productionID: 7,
		flags:        nodeFlagExtra,
	}
	compactLeaf := &compactFullLeaf{
		noTreeNode: noTreeNode{
			symbol:       12,
			startByte:    8,
			endByte:      13,
			parseState:   9,
			preGotoState: 19,
			productionID: 10,
			flags:        nodeFlagNamed | nodeFlagMissing,
		},
	}
	pending := &pendingParent{
		noTreeNode: noTreeNode{
			symbol:       13,
			startByte:    21,
			endByte:      34,
			parseState:   11,
			preGotoState: 21,
			productionID: 12,
			flags:        nodeFlagNamed | nodeFlagExtra,
		},
		childRange: newPendingChildRange(0, 0, 3),
	}

	builders := []struct {
		kind  string
		build func(dynamicPrecedence int32) stackEntry
	}{
		{"Node", func(p int32) stackEntry {
			n := *node
			n.dynamicPrecedence = p
			return newStackEntryNode(2, &n)
		}},
		{"noTreeNode", func(p int32) stackEntry {
			n := *noTree
			n.dynamicPrecedence = p
			return newStackEntryNoTreeNode(3, &n)
		}},
		{"compactFullLeaf", func(p int32) stackEntry {
			n := *compactLeaf
			n.dynamicPrecedence = p
			return newStackEntryCompactFullLeaf(4, &n)
		}},
		{"pendingParent", func(p int32) stackEntry {
			n := *pending
			n.dynamicPrecedence = p
			return newStackEntryPendingParent(5, &n)
		}},
	}

	cases := []struct {
		name       string
		precedence int32
	}{
		{"zero", 0},
		{"positive-9", 9},
		{"negative-9", -9},
	}

	nilEntry := stackEntry{state: 1}
	got := gssEntryHash(gssHashSeed, nilEntry)
	want := gssEntryHashViaAccessors(gssHashSeed, nilEntry)
	if got != want {
		t.Fatalf("nil entry: direct hash = %d, want accessor hash = %d", got, want)
	}

	for _, b := range builders {
		zeroEntry := b.build(0)
		zeroHash := gssEntryHash(gssHashSeed, zeroEntry)
		for _, tc := range cases {
			entry := b.build(tc.precedence)
			got := gssEntryHash(gssHashSeed, entry)
			want := gssEntryHashViaAccessors(gssHashSeed, entry)
			if got != want {
				t.Fatalf("%s %s: direct hash = %d, want accessor hash = %d", b.kind, tc.name, got, want)
			}
			if tc.precedence != 0 && got == zeroHash {
				t.Fatalf("%s %s: hash = %d equals %s zero-precedence hash = %d", b.kind, tc.name, got, b.kind, zeroHash)
			}
		}
	}
}

func TestGSSEntryHashIncludesDynamicPrecedence(t *testing.T) {
	low := &Node{symbol: 10, startByte: 1, endByte: 3, parseState: 4, flags: nodeFlagNamed}
	high := &Node{symbol: 10, startByte: 1, endByte: 3, parseState: 4, flags: nodeFlagNamed, dynamicPrecedence: 7}

	lowHash := gssEntryHash(gssHashSeed, newStackEntryNode(2, low))
	highHash := gssEntryHash(gssHashSeed, newStackEntryNode(2, high))
	if lowHash == highHash {
		t.Fatal("gssEntryHash ignored node dynamic precedence")
	}

	lowChild := &Node{symbol: 20, startByte: 1, endByte: 2, flags: nodeFlagNamed}
	highChild := &Node{symbol: 20, startByte: 1, endByte: 2, flags: nodeFlagNamed, dynamicPrecedence: 5}
	lowParent := &Node{symbol: 11, startByte: 1, endByte: 3, parseState: 4, flags: nodeFlagNamed, children: []*Node{lowChild}}
	highParent := &Node{symbol: 11, startByte: 1, endByte: 3, parseState: 4, flags: nodeFlagNamed, children: []*Node{highChild}}

	lowParentHash := gssEntryHash(gssHashSeed, newStackEntryNode(2, lowParent))
	highParentHash := gssEntryHash(gssHashSeed, newStackEntryNode(2, highParent))
	if lowParentHash == highParentHash {
		t.Fatal("gssEntryHash ignored shallow child dynamic precedence")
	}
}

func TestGSSMainAddLinkAtCapRejectsUnsafeEquivalentReplacement(t *testing.T) {
	head := &gssNode{
		entry: newStackEntryNode(10, &Node{symbol: 20, startByte: 1, endByte: 2, flags: nodeFlagNamed, dynamicPrecedence: 1}),
		prev:  &gssNode{entry: stackEntry{state: 100}, depth: 1},
		depth: 2,
	}
	for i := 1; i < maxMainLinkCount; i++ {
		head.appendExtraLink(gssMainLink{
			prev:  &gssNode{entry: stackEntry{state: StateID(100 + i)}, depth: 1},
			entry: newStackEntryNode(10, &Node{symbol: 20, startByte: 1, endByte: 2, flags: nodeFlagNamed, dynamicPrecedence: int32(i + 1)}),
		})
	}
	if got := head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("head link count = %d, want cap %d", got, maxMainLinkCount)
	}
	if got := int(head.extraLinkCap); got != maxMainLinkCount-1 {
		t.Fatalf("ordinary extra-link capacity = %d, want %d", got, maxMainLinkCount-1)
	}

	latePrev := &gssNode{entry: stackEntry{state: 200}, depth: 1}
	lateEntry := newStackEntryNode(10, &Node{symbol: 20, startByte: 1, endByte: 2, flags: nodeFlagNamed, dynamicPrecedence: 99})
	if gssMainAddLink(head, latePrev, lateEntry) {
		t.Fatal("unsafe capped equivalent replacement was reported as incorporated")
	}

	if got := head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("head link count after late add = %d, want cap %d", got, maxMainLinkCount)
	}
	var foundLate, foundLowest bool
	for i := 0; i < head.linkCount(); i++ {
		prev, entry := head.link(i)
		if prev == latePrev && stackEntryDynamicPrecedence(entry) == 99 {
			foundLate = true
		}
		if stackEntryDynamicPrecedence(entry) == 1 {
			foundLowest = true
		}
	}
	if foundLate {
		t.Fatal("late non-mergeable equivalent link was retained at cap")
	}
	if !foundLowest {
		t.Fatal("original lowest dynamic-precedence branch was lost")
	}
}

func TestGSSMainAddLinkAtCapReplacesEquivalentSamePredecessorWithHigherDynamic(t *testing.T) {
	sharedPrev := &gssNode{entry: stackEntry{state: 100}, depth: 1}
	head := &gssNode{
		entry: newStackEntryNode(10, &Node{symbol: 20, startByte: 1, endByte: 2, flags: nodeFlagNamed, dynamicPrecedence: 1}),
		prev:  sharedPrev,
		depth: 2,
	}
	for i := 1; i < maxMainLinkCount; i++ {
		head.appendExtraLink(gssMainLink{
			prev:  &gssNode{entry: stackEntry{state: StateID(100 + i)}, depth: 1},
			entry: newStackEntryNode(10, &Node{symbol: Symbol(20 + i), startByte: 1, endByte: 2, flags: nodeFlagNamed, dynamicPrecedence: int32(i + 1)}),
		})
	}
	if got := head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("head link count = %d, want cap %d", got, maxMainLinkCount)
	}

	lateEntry := newStackEntryNode(10, &Node{symbol: 20, startByte: 1, endByte: 2, flags: nodeFlagNamed, dynamicPrecedence: 99})
	if !gssMainAddLink(head, sharedPrev, lateEntry) {
		t.Fatal("same-predecessor capped equivalent replacement was rejected")
	}
	if got := head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("head link count after late add = %d, want cap %d", got, maxMainLinkCount)
	}
	prev, entry := head.link(0)
	if prev != sharedPrev {
		t.Fatal("replacement changed predecessor identity")
	}
	if got := stackEntryDynamicPrecedence(entry); got != 99 {
		t.Fatalf("dynamic precedence = %d, want 99", got)
	}
}

func TestGSSMainCLinkPolicyUsesEightSlotsThenDropsDistinctLink(t *testing.T) {
	entry := func(sym Symbol, dynamic int32) stackEntry {
		return newStackEntryNode(10, &Node{
			symbol:            sym,
			startByte:         1,
			endByte:           2,
			flags:             nodeFlagNamed,
			dynamicPrecedence: dynamic,
		})
	}
	prev := func(state StateID) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, depth: 1}
	}

	head := &gssNode{entry: entry(20, 1), prev: prev(100), depth: 2}
	scratch := glrMergeScratch{language: &Language{CompactPackedGSSVersionOrderCertified: true}}
	for i := 1; i < maxCMainLinkCount; i++ {
		if !gssMainAddLinkSeenMutate(
			&scratch,
			head,
			prev(StateID(100+i)),
			entry(Symbol(20+i), int32(i+1)),
			make(map[gssMergePair]bool),
		) {
			t.Fatalf("add link %d was rejected", i)
		}
	}
	if got := head.linkCount(); got != maxCMainLinkCount {
		t.Fatalf("head link count = %d, want C cap %d", got, maxCMainLinkCount)
	}
	if got := int(head.extraLinkCap); got != maxCMainLinkCount-1 {
		t.Fatalf("C extra-link capacity = %d, want %d", got, maxCMainLinkCount-1)
	}
	if head.linkCount() <= maxMainLinkCount {
		t.Fatalf("C link policy did not exceed ordinary cap %d", maxMainLinkCount)
	}

	before := snapshotGSSMainLinks(head)
	latePrev := prev(999)
	lateEntry := entry(20, 99)
	preflight := acquirePreflightForScratch(&scratch)
	if !preflight.canAddLink(head, latePrev, lateEntry) {
		t.Fatal("C preflight rejected a distinct link that C drops at capacity")
	}
	if got := preflight.linkCount(head); got != maxCMainLinkCount {
		t.Fatalf("preflight link count = %d, want unchanged C cap %d", got, maxCMainLinkCount)
	}
	if !gssMainAddLinkSeenMutate(&scratch, head, latePrev, lateEntry, make(map[gssMergePair]bool)) {
		t.Fatal("C mutate phase rejected a distinct link that C drops at capacity")
	}
	assertGSSMainLinksEqual(t, head, before)
}

func TestGSSMainCLinkPolicyStillUpdatesSamePredecessorAtCap(t *testing.T) {
	entry := func(sym Symbol, dynamic int32) stackEntry {
		return newStackEntryNode(10, &Node{
			symbol:            sym,
			startByte:         1,
			endByte:           2,
			flags:             nodeFlagNamed,
			dynamicPrecedence: dynamic,
		})
	}
	sharedPrev := &gssNode{entry: stackEntry{state: 100}, depth: 1}
	head := &gssNode{entry: entry(20, 1), prev: sharedPrev, depth: 2}
	for i := 1; i < maxCMainLinkCount; i++ {
		head.appendExtraLinkWithLimit(gssMainLink{
			prev:  &gssNode{entry: stackEntry{state: StateID(100 + i)}, depth: 1},
			entry: entry(Symbol(20+i), int32(i+1)),
		}, maxCMainLinkCount)
	}

	scratch := glrMergeScratch{language: &Language{CompactPackedGSSVersionOrderCertified: true}}
	if !gssMainAddLinkSeenMutate(&scratch, head, sharedPrev, entry(20, 99), make(map[gssMergePair]bool)) {
		t.Fatal("same-predecessor update was rejected at the C link cap")
	}
	if got := head.linkCount(); got != maxCMainLinkCount {
		t.Fatalf("head link count = %d, want %d", got, maxCMainLinkCount)
	}
	gotPrev, gotEntry := head.link(0)
	if gotPrev != sharedPrev || stackEntryDynamicPrecedence(gotEntry) != 99 {
		t.Fatal("same-predecessor update did not retain C's higher dynamic precedence")
	}
}

func TestGSSMainCLinkPolicyPreflightMatchesMultiLinkMutationAtLimit(t *testing.T) {
	entry := func(sym Symbol) stackEntry {
		return newStackEntryNode(10, &Node{
			symbol:    sym,
			startByte: 1,
			endByte:   2,
			flags:     nodeFlagNamed,
		})
	}
	prev := func(state StateID) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, depth: 1}
	}

	destination := &gssNode{entry: entry(20), prev: prev(100), depth: 2}
	for i := 1; i < maxMainLinkCount; i++ {
		destination.appendExtraLink(gssMainLink{
			prev:  prev(StateID(100 + i)),
			entry: entry(Symbol(20 + i)),
		})
	}
	sourcePrevs := []*gssNode{prev(200), prev(201), prev(202)}
	source := &gssNode{entry: entry(50), prev: sourcePrevs[0], depth: 2}
	for i := 1; i < len(sourcePrevs); i++ {
		source.appendExtraLink(gssMainLink{
			prev:  sourcePrevs[i],
			entry: entry(Symbol(50 + i)),
		})
	}

	scratch := glrMergeScratch{language: &Language{CompactPackedGSSVersionOrderCertified: true}}
	preflight := acquirePreflightForScratch(&scratch)
	if !preflight.canMergeNodes(destination, source) {
		t.Fatal("C preflight rejected a merge that can fill and then drop links")
	}
	if got := preflight.linkCount(destination); got != maxCMainLinkCount {
		t.Fatalf("preflight destination links = %d, want %d", got, maxCMainLinkCount)
	}
	if got := len(preflight.virtualLink[destination]); got != maxCMainLinkCount-maxMainLinkCount {
		t.Fatalf("preflight staged links = %d, want %d", got, maxCMainLinkCount-maxMainLinkCount)
	}
	if !gssMainMergeNodesSeenMutate(&scratch, destination, source, make(map[gssMergePair]bool)) {
		t.Fatal("C mutation rejected a merge that its preflight accepted")
	}
	if got := destination.linkCount(); got != maxCMainLinkCount {
		t.Fatalf("mutated destination links = %d, want %d", got, maxCMainLinkCount)
	}
	for i, sourcePrev := range sourcePrevs {
		found := false
		for linkIndex := 0; linkIndex < destination.linkCount(); linkIndex++ {
			gotPrev, _ := destination.link(linkIndex)
			if gotPrev == sourcePrev {
				found = true
				break
			}
		}
		if want := i < maxCMainLinkCount-maxMainLinkCount; found != want {
			t.Fatalf("source link %d retained = %v, want %v", i, found, want)
		}
	}
}

func TestGSSMainAddLinkMergesNestedPackedPredecessorLinks(t *testing.T) {
	baseA := &gssNode{entry: stackEntry{state: 100}, depth: 1}
	baseB := &gssNode{entry: stackEntry{state: 101}, depth: 1}
	baseC := &gssNode{entry: stackEntry{state: 102}, depth: 1}

	left := func() stackEntry {
		return newStackEntryNode(2, &Node{symbol: 30, startByte: 0, endByte: 1, flags: nodeFlagNamed})
	}
	right := func() stackEntry {
		return newStackEntryNode(3, &Node{symbol: 31, startByte: 1, endByte: 2, flags: nodeFlagNamed})
	}

	packedPred := gssNodeWithExtraLinks(gssNode{
		entry: left(),
		prev:  baseA,
		depth: 2,
	}, gssMainLink{prev: baseB, entry: left()})
	head := &gssNode{
		entry: right(),
		prev:  packedPred,
		depth: 3,
	}
	incomingPred := &gssNode{
		entry: left(),
		prev:  baseC,
		depth: 2,
	}

	if !gssMainAddLink(head, incomingPred, right()) {
		t.Fatal("nested packed predecessor link was not incorporated")
	}

	if got := head.linkCount(); got != 1 {
		t.Fatalf("head link count = %d, want 1 merged top link", got)
	}
	if got := packedPred.linkCount(); got != 3 {
		t.Fatalf("packed predecessor link count = %d, want 3", got)
	}

	stack := glrStack{gss: gssStack{head: head}}
	forks := reduceWindowsFromGSS(&stack, 2, maxMainLinkCount)
	if len(forks) != 3 {
		t.Fatalf("reduce windows = %d, want 3", len(forks))
	}
	seenTopStates := make(map[StateID]bool)
	for _, fork := range forks {
		if len(fork.window) != 2 {
			t.Fatalf("window length = %d, want 2", len(fork.window))
		}
		if stackEntryNodeSymbol(fork.window[0]) != 30 || stackEntryNodeSymbol(fork.window[1]) != 31 {
			t.Fatalf("unexpected window symbols: %d, %d", stackEntryNodeSymbol(fork.window[0]), stackEntryNodeSymbol(fork.window[1]))
		}
		seenTopStates[fork.topState] = true
	}
	for _, state := range []StateID{100, 101, 102} {
		if !seenTopStates[state] {
			t.Fatalf("reduce windows did not include branch with top state %d", state)
		}
	}
}

func TestGSSMainMergeFailureLeavesIncumbentPredecessorUnchanged(t *testing.T) {
	branchEntry := func(sym Symbol) stackEntry {
		return newStackEntryNode(2, &Node{symbol: sym, startByte: 0, endByte: 3, flags: nodeFlagNamed})
	}
	topEntry := func(sym Symbol) stackEntry {
		return newStackEntryNode(9, &Node{symbol: sym, startByte: 3, endByte: 4, flags: nodeFlagNamed})
	}
	base := func(state StateID) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, depth: 1}
	}

	wBase := base(1000)
	w := &gssNode{
		entry: branchEntry(100),
		prev:  wBase,
		depth: 2,
	}
	for i := 1; i < maxMainLinkCount-1; i++ {
		w.appendExtraLink(gssMainLink{
			prev:  base(StateID(1000 + i)),
			entry: branchEntry(Symbol(100 + i)),
		})
	}
	w.appendExtraLink(gssMainLink{
		prev:  base(1999),
		entry: branchEntry(199),
	})
	if got := w.linkCount(); got != maxMainLinkCount {
		t.Fatalf("incumbent predecessor link count = %d, want %d", got, maxMainLinkCount)
	}

	x := &gssNode{
		entry: branchEntry(100),
		prev:  wBase,
		depth: 2,
	}
	y := &gssNode{
		entry: branchEntry(200),
		prev:  base(3000),
		depth: 2,
	}
	incumbentHead := gssNodeWithExtraLinks(gssNode{
		entry: topEntry(1),
		prev:  w,
		depth: 3,
	}, gssMainLink{prev: x, entry: topEntry(2)})
	incomingHead := gssNodeWithExtraLinks(gssNode{
		entry: topEntry(1),
		prev:  y,
		depth: 3,
	}, gssMainLink{prev: x, entry: topEntry(1)})
	beforePred := snapshotGSSMainLinks(w)
	beforeX := snapshotGSSMainLinks(x)
	beforeHead := snapshotGSSMainLinks(incumbentHead)

	incumbent := glrStack{gss: gssStack{head: incumbentHead}, byteOffset: 2}
	candidate := glrStack{gss: gssStack{head: incomingHead}, byteOffset: 2}
	if gssMainMerge(&incumbent, &candidate) {
		t.Fatal("gssMainMerge reported success after omitting a virtual source link")
	}
	assertGSSMainLinksEqual(t, w, beforePred)
	assertGSSMainLinksEqual(t, x, beforeX)
	assertGSSMainLinksEqual(t, incumbentHead, beforeHead)
}

func TestGSSMainCanMergeNodesEnumeratesVirtualSourceLinks(t *testing.T) {
	entry := func(sym Symbol) stackEntry {
		return newStackEntryNode(2, &Node{symbol: sym, startByte: 0, endByte: 1, flags: nodeFlagNamed})
	}
	base := func(state StateID) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, depth: 1}
	}

	sharedPrev := base(100)
	source := &gssNode{
		entry: entry(10),
		prev:  sharedPrev,
		depth: 2,
	}
	dest := &gssNode{
		entry: entry(10),
		prev:  sharedPrev,
		depth: 2,
	}
	for i := 1; i < maxMainLinkCount; i++ {
		dest.appendExtraLink(gssMainLink{
			prev:  base(StateID(200 + i)),
			entry: entry(Symbol(20 + i)),
		})
	}
	if got := dest.linkCount(); got != maxMainLinkCount {
		t.Fatalf("destination link count = %d, want %d", got, maxMainLinkCount)
	}

	p := newGSSMainPreflight(make(map[gssMergePair]bool))
	virtualPrev := base(300)
	virtualEntry := entry(99)
	if !p.canAddLink(source, virtualPrev, virtualEntry) {
		t.Fatal("failed to stage virtual source link")
	}
	if got := source.linkCount(); got != 1 {
		t.Fatalf("source real link count = %d, want 1", got)
	}
	if got := p.linkCount(source); got != 2 {
		t.Fatalf("source preflight link count = %d, want 2 including virtual source link", got)
	}

	if p.canMergeNodes(dest, source) {
		t.Fatal("canMergeNodes skipped the virtual-only source link and accepted an over-cap merge")
	}
}

func TestGSSMainPreflightCachedReachSeesVirtualLinkAfterFalse(t *testing.T) {
	base := func(state StateID, prev *gssNode) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, prev: prev, depth: 1}
	}

	tail := base(3, nil)
	mid := base(2, nil)
	head := base(1, mid)
	p := newGSSMainPreflight(make(map[gssMergePair]bool))

	if p.canReach(head, tail) {
		t.Fatal("head reached isolated tail before virtual link")
	}
	p.addVirtualLink(mid, tail, stackEntry{state: 9})
	if !p.canReach(head, tail) {
		t.Fatal("cached preflight reachability missed newly added virtual link")
	}
}

func TestGSSMainPreflightCachedReachInvalidatesFalseBeforeCycleCheck(t *testing.T) {
	base := func(state StateID, prev *gssNode) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, prev: prev, depth: 1}
	}

	mid := base(2, nil)
	head := base(1, mid)
	tail := base(3, nil)
	p := newGSSMainPreflight(make(map[gssMergePair]bool))

	if p.canReach(tail, head) {
		t.Fatal("tail reached head before virtual link")
	}
	p.addVirtualLink(tail, head, stackEntry{state: 9})
	if p.canAddLink(head, tail, stackEntry{state: 10}) {
		t.Fatal("stale false reachability allowed a virtual cycle")
	}
	if got := p.linkCount(head); got != 1 {
		t.Fatalf("head preflight link count = %d, want 1 after rejected cycle link", got)
	}
}

func TestGSSMainPreflightCleanZeroCacheInvalidatesAfterVirtualErrorLink(t *testing.T) {
	base := func(state StateID, prev *gssNode, depth int) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, prev: prev, depth: uint32(depth)}
	}

	tail := base(3, nil, 0)
	mid := base(2, tail, 1)
	head := base(1, mid, 2)
	errorPrev := base(4, nil, 0)
	errorNode := &Node{symbol: errorSymbol, startByte: 0, endByte: 0, flags: nodeFlagNamed | nodeFlagHasError}
	errorEntry := newStackEntryNode(5, errorNode)
	p := newGSSMainPreflight(make(map[gssMergePair]bool))

	if !p.cleanZeroErrorAllLinks(head) {
		t.Fatal("clean preflight graph was rejected before virtual error link")
	}
	p.addVirtualLink(mid, errorPrev, errorEntry)
	if p.cleanZeroErrorAllLinks(head) {
		t.Fatal("cached clean-zero result survived virtual error link")
	}
}

func TestGSSMainMergeRejectsVirtualCycleWithoutPartialMutation(t *testing.T) {
	entry := func(sym Symbol, start, end uint32) stackEntry {
		return newStackEntryNode(2, &Node{symbol: sym, startByte: start, endByte: end, flags: nodeFlagNamed})
	}
	topEntry := func(sym Symbol) stackEntry {
		return newStackEntryNode(9, &Node{symbol: sym, startByte: 3, endByte: 4, flags: nodeFlagNamed})
	}
	base := func(state StateID) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, depth: 1}
	}

	a := &gssNode{
		entry: entry(10, 0, 1),
		prev:  base(100),
		depth: 2,
	}
	x := &gssNode{
		entry: entry(11, 0, 3),
		prev:  base(101),
		depth: 2,
	}
	y := &gssNode{
		entry: entry(12, 0, 3),
		prev:  a,
		depth: 2,
	}

	incumbentHead := gssNodeWithExtraLinks(gssNode{
		entry: topEntry(20),
		prev:  a,
		depth: 3,
	}, gssMainLink{prev: x, entry: topEntry(21)})
	for i := incumbentHead.linkCount(); i < maxMainLinkCount; i++ {
		incumbentHead.appendExtraLink(gssMainLink{
			prev:  base(StateID(200 + i)),
			entry: topEntry(Symbol(30 + i)),
		})
	}
	if got := incumbentHead.linkCount(); got != maxMainLinkCount {
		t.Fatalf("incumbent head link count = %d, want %d", got, maxMainLinkCount)
	}

	candidate := gssNodeWithExtraLinks(gssNode{
		entry: topEntry(21),
		prev:  y,
		depth: 3,
	}, gssMainLink{prev: x, entry: topEntry(20)})

	beforeHead := snapshotGSSMainLinks(incumbentHead)
	beforeX := snapshotGSSMainLinks(x)

	incumbentStack := glrStack{gss: gssStack{head: incumbentHead}, byteOffset: 4}
	candidateStack := glrStack{gss: gssStack{head: candidate}, byteOffset: 4}
	if gssMainMerge(&incumbentStack, &candidateStack) {
		t.Fatal("gssMainMerge reported success for virtual cycle")
	}
	assertGSSMainLinksEqual(t, incumbentHead, beforeHead)
	assertGSSMainLinksEqual(t, x, beforeX)
}

func TestGSSMainEquivalentReplacementFailureLeavesWorstPredecessorUnchanged(t *testing.T) {
	equivEntry := func(dynamic int32) stackEntry {
		return newStackEntryNode(9, &Node{symbol: 60, startByte: 1, endByte: 2, flags: nodeFlagNamed, dynamicPrecedence: dynamic})
	}
	branchEntry := func(sym Symbol) stackEntry {
		return newStackEntryNode(2, &Node{symbol: sym, startByte: 0, endByte: 1, flags: nodeFlagNamed})
	}
	base := func(state StateID) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, depth: 1}
	}

	worstPrev := &gssNode{
		entry: branchEntry(300),
		prev:  base(3000),
		depth: 2,
	}
	for i := 1; i < maxMainLinkCount-1; i++ {
		worstPrev.appendExtraLink(gssMainLink{
			prev:  base(StateID(3000 + i)),
			entry: branchEntry(Symbol(300 + i)),
		})
	}
	if got := worstPrev.linkCount(); got != maxMainLinkCount-1 {
		t.Fatalf("worst predecessor link count = %d, want %d", got, maxMainLinkCount-1)
	}

	head := &gssNode{entry: equivEntry(1), prev: worstPrev, depth: 3}
	for i := 1; i < maxMainLinkCount; i++ {
		head.appendExtraLink(gssMainLink{
			prev:  base(StateID(4000 + i)),
			entry: equivEntry(int32(10 + i)),
		})
	}

	incomingPrev := gssNodeWithExtraLinks(gssNode{
		entry: branchEntry(400),
		prev:  base(5000),
		depth: 2,
	}, gssMainLink{prev: base(5001), entry: branchEntry(401)})
	beforeWorst := snapshotGSSMainLinks(worstPrev)
	beforeHead := snapshotGSSMainLinks(head)

	if gssMainReplaceWorstEquivalentLinkIfBetter(head, incomingPrev, equivEntry(99)) {
		t.Fatal("capped equivalent replacement reported success for overfull nested predecessor merge")
	}
	assertGSSMainLinksEqual(t, worstPrev, beforeWorst)
	assertGSSMainLinksEqual(t, head, beforeHead)
}

func snapshotGSSMainLinks(n *gssNode) []gssMainLink {
	links := make([]gssMainLink, n.linkCount())
	for i := range links {
		prev, entry := n.link(i)
		links[i] = gssMainLink{prev: prev, entry: entry}
	}
	return links
}

func assertGSSMainLinksEqual(t *testing.T, n *gssNode, want []gssMainLink) {
	t.Helper()
	if got := n.linkCount(); got != len(want) {
		t.Fatalf("link count = %d, want %d", got, len(want))
	}
	for i, wantLink := range want {
		gotPrev, gotEntry := n.link(i)
		if gotPrev != wantLink.prev || gotEntry != wantLink.entry {
			t.Fatalf("link %d changed: got (%p,%+v), want (%p,%+v)", i, gotPrev, gotEntry, wantLink.prev, wantLink.entry)
		}
	}
}

func gssEntryHashViaAccessors(prev uint64, entry stackEntry) uint64 {
	h := prev ^ uint64(entry.state)
	h *= gssHashPrime

	if !stackEntryHasNode(entry) {
		h ^= gssNilNodeSentinel
		h *= gssHashPrime
		return h
	}

	h ^= uint64(stackEntryNodeSymbol(entry))
	h *= gssHashPrime
	h ^= (uint64(stackEntryNodeStartByte(entry)) << 32) | uint64(stackEntryNodeEndByte(entry))
	h *= gssHashPrime
	h ^= uint64(stackEntryNodeParseState(entry))
	h *= gssHashPrime
	h ^= uint64(stackEntryNodePreGotoState(entry))
	h *= gssHashPrime
	h ^= uint64(stackEntryNodeProductionID(entry))
	h *= gssHashPrime
	h ^= uint64(stackEntryNodeChildCount(entry))
	h *= gssHashPrime
	h ^= uint64(uint32(stackEntryDynamicPrecedence(entry)))
	h *= gssHashPrime

	var flags uint64
	if stackEntryNodeIsExtra(entry) {
		flags |= 1
	}
	if stackEntryNodeIsNamed(entry) {
		flags |= 1 << 1
	}
	if stackEntryNodeHasError(entry) {
		flags |= 1 << 2
	}
	if stackEntryNodeIsMissing(entry) {
		flags |= 1 << 3
	}
	h ^= flags
	h *= gssHashPrime
	if n := stackEntryNode(entry); n != nil {
		h = gssNodeShallowMergeHashViaAccessors(h, n)
	}
	return h
}

func gssNodeShallowMergeHashViaAccessors(h uint64, n *Node) uint64 {
	if n == nil {
		h ^= gssNilNodeSentinel
		h *= gssHashPrime
		return h
	}
	h ^= uint64(len(n.fieldIDs()))
	h *= gssHashPrime
	for i := range n.fieldIDs() {
		h ^= uint64(n.fieldIDs()[i])
		h *= gssHashPrime
	}
	for i := range n.children {
		child := n.children[i]
		if child == nil {
			h ^= gssNilNodeSentinel
			h *= gssHashPrime
			continue
		}
		h ^= uint64(child.symbol)
		h *= gssHashPrime
		h ^= (uint64(child.startByte) << 32) | uint64(child.endByte)
		h *= gssHashPrime
		h ^= uint64(child.preGotoState)
		h *= gssHashPrime
		h ^= uint64(nodeChildCountNoMaterialize(child))
		h *= gssHashPrime
		h ^= uint64(len(child.fieldIDs()))
		h *= gssHashPrime
		h ^= uint64(uint32(child.dynamicPrecedence))
		h *= gssHashPrime
		h ^= gssEntryFlagHash(child.flags & nodeStackEquivNoMissingFlagMask)
		h *= gssHashPrime
	}
	return h
}

func TestGSSStacksEqualWithLazyHashes(t *testing.T) {
	var scratch gssScratch
	scratch.singleStackMode = true

	left := &Node{symbol: 1, startByte: 0, endByte: 1}
	right := &Node{symbol: 2, startByte: 1, endByte: 2}

	build := func() gssStack {
		var s gssStack
		s.push(1, nil, &scratch)
		s.push(2, left, &scratch)
		s.push(3, right, &scratch)
		return s
	}

	a := build()
	b := build()
	if a.head.hash != 0 || b.head.hash != 0 {
		t.Fatal("expected stacks to start with lazy hashes")
	}
	if !gssStacksEqual(a, b) {
		t.Fatal("expected equal GSS stacks with lazy hashes")
	}
	if a.head.hash == 0 || b.head.hash == 0 {
		t.Fatal("expected equality check to populate lazy hashes")
	}
}

func TestGLRStackDemoteLinearGSSPreservesStackAndCanRefork(t *testing.T) {
	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	stack := newGLRStackWithScratch(1, &entryScratch)
	first := &Node{symbol: 11, startByte: 0, endByte: 3}
	second := &Node{symbol: 12, startByte: 3, endByte: 7}
	stack.push(2, first, &entryScratch, &gssScratch)
	stack.push(3, second, &entryScratch, &gssScratch)
	stack.score = 9
	stack.recoverabilityKnown = true
	stack.mayRecover = true
	stack.ensureGSS(&gssScratch)
	stack.entries = nil

	if !stack.demoteLinearGSS(&entryScratch) {
		t.Fatal("demoteLinearGSS() = false, want true for a linear GSS")
	}
	if stack.gss.head != nil {
		t.Fatalf("GSS head after demotion = %p, want nil", stack.gss.head)
	}
	if got, want := len(stack.entries), 3; got != want {
		t.Fatalf("entry depth after demotion = %d, want %d", got, want)
	}
	if got, want := stack.entries[0].state, StateID(1); got != want {
		t.Fatalf("root state after demotion = %d, want %d", got, want)
	}
	if got := stackEntryNode(stack.entries[1]); got != first {
		t.Fatalf("first node after demotion = %p, want %p", got, first)
	}
	if got := stackEntryNode(stack.entries[2]); got != second {
		t.Fatalf("second node after demotion = %p, want %p", got, second)
	}
	if got, want := stack.byteOffset, uint32(7); got != want {
		t.Fatalf("byte offset after demotion = %d, want %d", got, want)
	}
	if stack.score != 9 || !stack.recoverabilityKnown || !stack.mayRecover {
		t.Fatalf("stack metadata changed after demotion: %+v", stack)
	}

	usedBeforeRefork := gssScratch.usedTotal
	clone := stack.cloneWithScratch(&gssScratch)
	if stack.gss.head == nil || clone.gss.head != stack.gss.head {
		t.Fatalf("refork did not restore shared GSS head: stack=%p clone=%p", stack.gss.head, clone.gss.head)
	}
	if got, want := gssScratch.usedTotal-usedBeforeRefork, len(stack.entries); got != want {
		t.Fatalf("refork GSS allocations = %d, want stack depth %d", got, want)
	}
}

func TestGLRStackDemoteLinearGSSRejectsPackedLinks(t *testing.T) {
	var scratch gssScratch
	stack := glrStack{gss: buildGSSStack([]stackEntry{{state: 1}, {state: 2}, {state: 3}}, &scratch)}
	packed := stack.gss.head.prev
	packed.appendExtraLink(gssMainLink{entry: stackEntry{state: 4}})
	originalHead := stack.gss.head

	if stack.demoteLinearGSS(nil) {
		t.Fatal("demoteLinearGSS() = true, want false for a deep packed link")
	}
	if stack.gss.head != originalHead || stack.entries != nil {
		t.Fatalf("packed stack mutated on rejected demotion: head=%p entries=%d", stack.gss.head, len(stack.entries))
	}
}

func TestGSSScratchResetClearsTouchedSlots(t *testing.T) {
	var scratch gssScratch
	node := &Node{endByte: 1}
	var stack gssStack
	stack.push(1, node, &scratch)
	stack.push(2, node, &scratch)
	if len(scratch.slabs) == 0 || scratch.slabs[0].used != 2 {
		t.Fatalf("expected two used GSS slots, slabs=%d used=%d", len(scratch.slabs), scratch.slabs[0].used)
	}

	scratch.reset()

	slab := scratch.slabs[0]
	if slab.used != 0 {
		t.Fatalf("slab.used after reset = %d, want 0", slab.used)
	}
	for i := 0; i < 2; i++ {
		if stackEntryNode(slab.data[i].entry) != nil {
			t.Fatalf("slab.data[%d].entry node after reset = %p, want nil", i, stackEntryNode(slab.data[i].entry))
		}
		if slab.data[i].prev != nil {
			t.Fatalf("slab.data[%d].prev after reset = %p, want nil", i, slab.data[i].prev)
		}
	}
}

func TestGSSScratchResetClearsRetainedStackEntries(t *testing.T) {
	var scratch gssScratch
	scratch.stackEntries = make([]stackEntry, 0, 8)
	scratch.stackEntries = append(scratch.stackEntries, newStackEntryNode(1, &Node{}))
	backing := scratch.stackEntries[:cap(scratch.stackEntries)]
	backing[len(backing)-1] = newStackEntryNode(2, &Node{})

	scratch.reset()

	if len(scratch.stackEntries) != 0 || cap(scratch.stackEntries) != cap(backing) {
		t.Fatalf("retained stack entries len/cap = %d/%d, want 0/%d", len(scratch.stackEntries), cap(scratch.stackEntries), cap(backing))
	}
	if stackEntryHasNode(backing[0]) {
		t.Fatal("retained stack entry still holds a node after reset")
	}
	if stackEntryHasNode(backing[len(backing)-1]) {
		t.Fatal("retained stack entry beyond slice length still holds a node after reset")
	}
	if got, want := scratch.allocatedBytes, stackEntryBytesForCap(cap(backing)); got != want {
		t.Fatalf("allocated bytes = %d, want %d", got, want)
	}
}

func TestGSSScratchResetAccountsForNodesAndStackEntries(t *testing.T) {
	var scratch gssScratch
	_ = scratch.allocNode(stackEntry{state: 1}, nil, 1)
	scratch.stackEntries = make([]stackEntry, 0, 8)
	scratch.recomputeAllocatedBytes()
	nodeCapacity := scratch.capacityNodes()

	scratch.reset()

	want := gssNodeBytesForCap(nodeCapacity) + stackEntryBytesForCap(8)
	if scratch.allocatedBytes != want {
		t.Fatalf("allocated bytes = %d, want %d", scratch.allocatedBytes, want)
	}
}

func TestGSSScratchResetDropsOversizedStackEntries(t *testing.T) {
	var scratch gssScratch
	scratch.stackEntries = make([]stackEntry, 0, maxRetainedGSSStackEntries+1)

	scratch.reset()

	if scratch.stackEntries != nil {
		t.Fatalf("stack entries capacity = %d, want released", cap(scratch.stackEntries))
	}
	if scratch.allocatedBytes != 0 {
		t.Fatalf("allocated bytes = %d, want 0", scratch.allocatedBytes)
	}
}

func TestGSSScratchOverflowSlabGrowthBounded(t *testing.T) {
	elemSize := int(unsafe.Sizeof(gssNode{}))
	ceiling := maxOverflowSlabGrowthBytes / elemSize
	if ceiling <= 0 {
		t.Fatalf("invalid GSS slab ceiling %d", ceiling)
	}

	var scratch gssScratch
	// Start close enough to the ceiling that the old lastCap*2 policy would
	// immediately overshoot it, while keeping the test allocation modest.
	scratch.initialCap = ceiling * 3 / 4
	total := scratch.initialCap + ceiling*3
	var prev *gssNode
	for depth := 1; depth <= total; depth++ {
		prev = scratch.allocNode(stackEntry{state: 1}, prev, uint32(depth))
	}

	if len(scratch.slabs) < 3 {
		t.Fatalf("expected multiple GSS overflow slabs, got %d", len(scratch.slabs))
	}
	capacity := 0
	for i := range scratch.slabs {
		slabCap := len(scratch.slabs[i].data)
		capacity += slabCap
		if i > 0 && slabCap > ceiling {
			t.Fatalf("GSS overflow slab %d capacity=%d exceeds ceiling=%d", i, slabCap, ceiling)
		}
	}
	if waste := capacity - scratch.usedTotal; waste > ceiling {
		t.Fatalf("GSS capacity waste=%d nodes exceeds one ceiling slab=%d", waste, ceiling)
	}
	if got, want := scratch.allocatedBytes, gssNodeBytesForCap(capacity); got != want {
		t.Fatalf("GSS allocatedBytes=%d, want capacity bytes=%d", got, want)
	}
}

func TestGSSScratchRecycleForParseReusesClearedSlots(t *testing.T) {
	var scratch gssScratch
	payload := &Node{symbol: 7, endByte: 3}
	first := scratch.allocNode(newStackEntryNode(2, payload), nil, 1)
	first.appendExtraLink(gssMainLink{entry: stackEntry{state: 9}})
	first.aggGen = 12
	first.aggValid = gssAggCostValid | gssAggVisValid
	first.cleanZeroState = gssCleanZeroDirty

	scratch.recycleForParse()

	if scratch.usedTotal != 0 {
		t.Fatalf("used nodes after recycle = %d, want 0", scratch.usedTotal)
	}
	if first.prev != nil || first.entry.node != nil || first.extraLinks != nil || first.extraLinkCount != 0 ||
		first.extraLinkCap != 0 || first.aggGen != 0 || first.aggValid != 0 || first.cleanZeroState != gssCleanZeroUnknown {
		t.Fatalf("recycled slot retained state: %+v", *first)
	}
	second := scratch.allocNode(stackEntry{state: 4}, nil, 1)
	if second != first {
		t.Fatalf("recycled pointer = %p, want %p", second, first)
	}
	if scratch.peakUsed != 1 || scratch.usedTotal != 1 {
		t.Fatalf("allocation high-water peak=%d used=%d", scratch.peakUsed, scratch.usedTotal)
	}
}

func TestResetPendingStackBufferRetainsAndClearsSmallBuffer(t *testing.T) {
	backing := make([]glrStack, 2)
	backing[0].gss.head = &gssNode{}
	backing[0].entries = []stackEntry{{state: 1}}
	backing[1].cRec = &cRecoverState{}

	got := resetPendingStackBuffer(backing[:1], false)

	if len(got) != 0 || cap(got) != cap(backing) {
		t.Fatalf("reset len/cap = %d/%d, want 0/%d", len(got), cap(got), cap(backing))
	}
	for i := range backing {
		if backing[i].gss.head != nil || backing[i].entries != nil || backing[i].cRec != nil {
			t.Fatalf("backing slot %d retained stack references", i)
		}
	}
}

func TestResetPendingStackBufferDropsOversizedBufferWithoutScan(t *testing.T) {
	backing := make([]glrStack, 1, maxRetainedPendingStackCap+1)
	head := &gssNode{}
	backing[0].gss.head = head

	got := resetPendingStackBuffer(backing, true)

	if got != nil {
		t.Fatalf("oversized reset retained cap %d, want nil", cap(got))
	}
	if backing[0].gss.head != head {
		t.Fatal("oversized reset scanned the buffer before dropping it")
	}
}

func BenchmarkResetPendingStackBuffer(b *testing.B) {
	const count = maxRetainedPendingStackCap + 1
	size := int64(count) * int64(unsafe.Sizeof(glrStack{}))

	b.Run("drop_oversized", func(b *testing.B) {
		backing := make([]glrStack, 0, count)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkPendingStackBuffer = benchmarkResetPendingStackBuffer(backing)
			if benchmarkPendingStackBuffer != nil {
				b.Fatal("oversized buffer was retained")
			}
		}
	})
	b.Run("clear_oversized", func(b *testing.B) {
		backing := make([]glrStack, 0, count)
		b.SetBytes(size)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			clear(backing[:cap(backing)])
			backing = backing[:0]
		}
		benchmarkPendingStackBuffer = backing
	})
}

func pendingStackLifecycleBenchmarkCount(b *testing.B) int {
	b.Helper()
	const defaultCount = maxRetainedPendingStackCap + 1
	raw := os.Getenv("GOT_BENCH_PENDING_STACK_COUNT")
	if raw == "" {
		return defaultCount
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count <= maxRetainedPendingStackCap {
		b.Fatalf("GOT_BENCH_PENDING_STACK_COUNT = %q, want an integer above %d", raw, maxRetainedPendingStackCap)
	}
	return count
}

func reportBenchmarkGCs(b *testing.B, before uint32) {
	b.Helper()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if b.N > 0 {
		b.ReportMetric(float64(after.NumGC-before)/float64(b.N), "gc/op")
	}
}

func BenchmarkPendingStackProductionLifecycle(b *testing.B) {
	count := pendingStackLifecycleBenchmarkCount(b)
	bytesPerCycle := int64(count) * int64(unsafe.Sizeof(glrStack{}))

	b.Run("grow_drain_production_demotion_append", func(b *testing.B) {
		parser := &Parser{}
		var scratch parserScratch
		stacks := make([]glrStack, 1)
		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		b.ReportAllocs()
		b.SetBytes(bytesPerCycle)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for len(parser.pendingForkStacks) < count {
				parser.pendingForkStacks = append(parser.pendingForkStacks, glrStack{})
			}
			parser.pendingForkStacks = parser.pendingForkStacks[:0]
			stacks[0] = glrStack{gss: buildGSSStack([]stackEntry{{state: 1}}, &scratch.gss)}
			if !parser.tryDemoteSingleLinearGSS(stacks, &scratch) {
				b.Fatal("production demotion failed")
			}
			parser.pendingForkStacks = append(parser.pendingForkStacks, glrStack{})
			parser.pendingForkStacks = parser.pendingForkStacks[:0]
		}
		b.StopTimer()
		reportBenchmarkGCs(b, before.NumGC)
		benchmarkPendingStackBuffer = parser.pendingForkStacks
	})

	b.Run("grow_drain_pool_release_reacquire_append", func(b *testing.B) {
		pool := NewParserPool(buildArithmeticLanguage())
		parser := pool.checkout()
		pool.release(parser)
		parser = nil
		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		b.ReportAllocs()
		b.SetBytes(bytesPerCycle)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser = pool.checkout()
			for len(parser.pendingForkStacks) < count {
				parser.pendingForkStacks = append(parser.pendingForkStacks, glrStack{})
			}
			parser.pendingForkStacks = parser.pendingForkStacks[:0]
			pool.release(parser)
			parser = pool.checkout()
			parser.pendingForkStacks = append(parser.pendingForkStacks, glrStack{})
			parser.pendingForkStacks = parser.pendingForkStacks[:0]
			pool.release(parser)
		}
		b.StopTimer()
		reportBenchmarkGCs(b, before.NumGC)
		parser = pool.checkout()
		benchmarkPendingStackBuffer = parser.pendingForkStacks
		pool.release(parser)
	})
}

func TestParserDemotionBoundsOversizedPendingStackBuffers(t *testing.T) {
	var scratch parserScratch
	stacks := []glrStack{{
		gss: buildGSSStack([]stackEntry{{state: 1}}, &scratch.gss),
	}}
	parser := &Parser{}
	forkHead := &gssNode{}
	forkBacking := make([]glrStack, 1, maxRetainedPendingStackCap+1)
	forkBacking[0].gss.head = forkHead
	parser.pendingForkStacks = forkBacking[:0]
	frontierHead := &gssNode{}
	frontierBacking := make([]glrStack, 1, maxRetainedPendingStackCap+1)
	frontierBacking[0].gss.head = frontierHead
	parser.pendingFrontierForkStacks = frontierBacking[:0]

	if !parser.tryDemoteSingleLinearGSS(stacks, &scratch) {
		t.Fatal("tryDemoteSingleLinearGSS() = false")
	}
	cold := parser.forestDeclineMemo
	if cold == nil {
		t.Fatal("oversized production demotion did not allocate the cold sidecar")
	}
	if forkBacking[0].gss.head != forkHead || frontierBacking[0].gss.head != frontierHead {
		t.Fatal("production demotion scanned an oversized backing array")
	}
	if got := cap(parser.pendingForkStacks); got != maxRetainedPendingStackCap {
		t.Fatalf("pending fork capacity = %d, want %d", got, maxRetainedPendingStackCap)
	}
	if got := cap(parser.pendingFrontierForkStacks); got != maxRetainedPendingStackCap {
		t.Fatalf("pending frontier capacity = %d, want %d", got, maxRetainedPendingStackCap)
	}
	if got := cap(cold.pendingForkStackReserve); got != maxRetainedPendingStackCap {
		t.Fatalf("cold pending fork reserve capacity = %d, want %d", got, maxRetainedPendingStackCap)
	}
	if got := cap(cold.pendingFrontierForkStackReserve); got != maxRetainedPendingStackCap {
		t.Fatalf("cold pending frontier reserve capacity = %d, want %d", got, maxRetainedPendingStackCap)
	}
	forkReserve := &parser.pendingForkStacks[:cap(parser.pendingForkStacks)][0]
	frontierReserve := &parser.pendingFrontierForkStacks[:cap(parser.pendingFrontierForkStacks)][0]
	if got := &cold.pendingForkStackReserve[:cap(cold.pendingForkStackReserve)][0]; got != forkReserve {
		t.Fatalf("cold pending fork reserve = %p, want %p", got, forkReserve)
	}
	if got := &cold.pendingFrontierForkStackReserve[:cap(cold.pendingFrontierForkStackReserve)][0]; got != frontierReserve {
		t.Fatalf("cold pending frontier reserve = %p, want %p", got, frontierReserve)
	}
	for len(parser.pendingForkStacks) < maxRetainedPendingStackCap {
		parser.pendingForkStacks = append(parser.pendingForkStacks, glrStack{})
	}
	for len(parser.pendingFrontierForkStacks) < maxRetainedPendingStackCap {
		parser.pendingFrontierForkStacks = append(parser.pendingFrontierForkStacks, glrStack{})
	}
	parser.pendingForkStacks[0].gss.head = &gssNode{}
	parser.pendingFrontierForkStacks[0].gss.head = &gssNode{}
	parser.pendingForkStacks = parser.pendingForkStacks[:0]
	parser.pendingFrontierForkStacks = parser.pendingFrontierForkStacks[:0]
	stacks[0] = glrStack{gss: buildGSSStack([]stackEntry{{state: 2}}, &scratch.gss)}

	if !parser.tryDemoteSingleLinearGSS(stacks, &scratch) {
		t.Fatal("second tryDemoteSingleLinearGSS() = false")
	}
	if got := &parser.pendingForkStacks[:cap(parser.pendingForkStacks)][0]; got != forkReserve {
		t.Fatalf("pending fork reserve changed from %p to %p", forkReserve, got)
	}
	if got := &parser.pendingFrontierForkStacks[:cap(parser.pendingFrontierForkStacks)][0]; got != frontierReserve {
		t.Fatalf("pending frontier reserve changed from %p to %p", frontierReserve, got)
	}
	if parser.pendingForkStacks[:cap(parser.pendingForkStacks)][0].gss.head != nil ||
		parser.pendingFrontierForkStacks[:cap(parser.pendingFrontierForkStacks)][0].gss.head != nil {
		t.Fatal("bounded reserves retained stack references")
	}
}

func TestParserDemotionKeepsPendingReserveSidecarLazy(t *testing.T) {
	var scratch parserScratch
	stacks := []glrStack{{
		gss: buildGSSStack([]stackEntry{{state: 1}}, &scratch.gss),
	}}
	parser := &Parser{
		pendingForkStacks:         make([]glrStack, 0, maxRetainedPendingStackCap),
		pendingFrontierForkStacks: make([]glrStack, 0, maxRetainedPendingStackCap),
	}

	if !parser.tryDemoteSingleLinearGSS(stacks, &scratch) {
		t.Fatal("tryDemoteSingleLinearGSS() = false")
	}
	if parser.forestDeclineMemo != nil {
		t.Fatal("bounded production demotion allocated the cold sidecar")
	}
}

func TestParserRecycleDemotedGSSInvalidatesPointerHolders(t *testing.T) {
	var scratch parserScratch
	payload := &Node{symbol: 11, startByte: 1, endByte: 4}
	stack := glrStack{
		gss:          buildGSSStack([]stackEntry{{state: 1}, newStackEntryNode(2, payload)}, &scratch.gss),
		byteOffset:   4,
		cRec:         &cRecoverState{summary: []cStackSummaryEntry{{depth: 2, state: 2, posBytes: 4}}},
		cEntryAggGen: gssPrefixAggGen.Load(),
	}
	oldHead := stack.gss.head
	stale := stack.cloneWithScratch(&scratch.gss)

	scratch.merge.beginEquivEpoch()
	scratch.merge.result = append(scratch.merge.result, stack, stale)
	stacks := scratch.merge.result[:1]
	scratch.merge.cPrefixPath = append(scratch.merge.cPrefixPath, oldHead)
	scratch.merge.cleanZeroCache = map[*gssNode]gssCleanZeroErrorCacheEntry{
		oldHead: {resultEpoch: scratch.merge.cleanZeroEpoch, clean: false},
	}
	storeCleanZeroNodeState(oldHead, gssPrefixAggGen.Load(), false)
	scratch.merge.cleanZeroFrames = append(scratch.merge.cleanZeroFrames, gssCleanZeroFrame{node: oldHead})
	scratch.merge.spineVisit = append(scratch.merge.spineVisit, spinePairKey{a: oldHead, b: oldHead.prev})
	scratch.merge.mergeSeen = map[gssMergePair]bool{{a: oldHead, b: oldHead.prev}: true}
	scratch.merge.preflight = newGSSMainPreflight(nil)
	scratch.merge.preflight.addVirtualLink(oldHead, oldHead.prev, oldHead.entry)
	equivEpochBefore := scratch.merge.equivEpoch
	gssPointerEpochBefore := scratch.merge.gssPointerEpoch

	parser := &Parser{crecoveryEnteredErrorState: true}
	pending := []glrStack{stale}
	parser.pendingForkStacks = pending[:0]
	frontierPending := []glrStack{stale}
	parser.pendingFrontierForkStacks = frontierPending[:0]
	parser.cPrefixPath = append(parser.cPrefixPath, oldHead)
	prefixGenBefore := gssPrefixAggGen.Load()

	if !parser.tryDemoteSingleLinearGSS(stacks, &scratch) {
		t.Fatal("tryDemoteSingleLinearGSS() = false")
	}
	if stacks[0].gss.head != nil || len(stacks[0].entries) != 2 || stackEntryNode(stacks[0].entries[1]) != payload ||
		stacks[0].cRec == nil || len(stacks[0].cRec.summary) != 1 || stacks[0].cEntryAggGen != 0 {
		t.Fatalf("live stack changed during recycle: %+v", stacks[0])
	}
	if scratch.gss.usedTotal != 0 {
		t.Fatalf("GSS used nodes after recycle = %d, want 0", scratch.gss.usedTotal)
	}
	if got := gssPrefixAggGen.Load(); got == prefixGenBefore {
		t.Fatalf("GSS prefix aggregate generation did not advance: %d", got)
	}
	if scratch.merge.equivEpoch != equivEpochBefore {
		t.Fatalf("node/spine equivalence epoch = %d, want preserved %d", scratch.merge.equivEpoch, equivEpochBefore)
	}
	if scratch.merge.gssPointerEpoch == gssPointerEpochBefore {
		t.Fatalf("GSS pointer epoch did not advance: %d", scratch.merge.gssPointerEpoch)
	}
	if len(scratch.merge.result) != 0 || len(scratch.merge.cPrefixPath) != 0 || len(scratch.merge.mergeSeen) != 0 {
		t.Fatalf("merge pointer holders not reset: result=%d prefix=%d seen=%d", len(scratch.merge.result), len(scratch.merge.cPrefixPath), len(scratch.merge.mergeSeen))
	}
	if len(scratch.merge.cleanZeroCache) != 0 {
		t.Fatalf("clean-zero cache len after invalidation = %d, want 0", len(scratch.merge.cleanZeroCache))
	}
	if scratch.merge.preflight == nil || len(scratch.merge.preflight.virtualLink) != 0 || len(scratch.merge.preflight.reachCache) != 0 {
		t.Fatal("preflight pointer holders not reset")
	}
	if parser.cPrefixPath != nil && len(parser.cPrefixPath) != 0 {
		t.Fatalf("parser prefix path len=%d, want 0", len(parser.cPrefixPath))
	}
	if got := parser.pendingForkStacks[:cap(parser.pendingForkStacks)][0].gss.head; got != nil {
		t.Fatalf("pending fork backing retained GSS head %p", got)
	}
	if got := parser.pendingFrontierForkStacks[:cap(parser.pendingFrontierForkStacks)][0].gss.head; got != nil {
		t.Fatalf("frontier pending backing retained GSS head %p", got)
	}
	if oldHead.entry.node != nil || oldHead.prev != nil {
		t.Fatalf("old slab node not cleared: %+v", *oldHead)
	}

	clone := stacks[0].cloneWithScratch(&scratch.gss)
	if stacks[0].gss.head == nil || clone.gss.head != stacks[0].gss.head {
		t.Fatalf("refork after recycle failed: stack=%p clone=%p", stacks[0].gss.head, clone.gss.head)
	}
	if got := stacks[0].gss.materialize(nil); len(got) != 2 || stackEntryNode(got[1]) != payload {
		t.Fatalf("reforked stack entries = %+v", got)
	}
	if !gssNodeCleanZeroErrorAllLinksWithScratch(&scratch.merge, stacks[0].gss.head) {
		t.Fatal("stale clean-zero entry survived recycled-address lookup")
	}
	if clean, ok := lookupCleanZeroNodeState(stacks[0].gss.head, gssPrefixAggGen.Load()); !ok || !clean {
		t.Fatalf("refreshed clean-zero state = %v, %v; want true, true", clean, ok)
	}
}

func TestGLRMergeScratchResetClearsPreflightReachCache(t *testing.T) {
	from := &gssNode{}
	target := &gssNode{}
	scratch := glrMergeScratch{
		preflight: newGSSMainPreflight(nil),
	}
	preflight := scratch.preflight
	preflight.reachCache = make([]gssReachCacheEntry, maxGSSPreflightReachCacheEntries)
	preflight.reachCache[0] = gssReachCacheEntry{
		from:       uintptr(unsafe.Pointer(from)),
		target:     uintptr(unsafe.Pointer(target)),
		generation: preflight.reachCacheGeneration,
		reachable:  true,
	}

	scratch.reset()
	if scratch.preflight != nil {
		t.Fatal("preflight survived merge-scratch reset")
	}
	if len(preflight.reachCache) != 0 {
		t.Fatalf("reset preflight reach-cache length = %d, want 0", len(preflight.reachCache))
	}
	if got := preflight.reachCache[:cap(preflight.reachCache)][0]; got != (gssReachCacheEntry{}) {
		t.Fatalf("reset preflight retained reach-cache entry: %+v", got)
	}
}

func TestGSSReuseRetainsOnlyFingerprintedSpineCache(t *testing.T) {
	var nodes gssScratch
	a := nodes.allocNode(stackEntry{state: 1}, nil, 1)
	b := nodes.allocNode(stackEntry{state: 1}, nil, 1)
	var merge glrMergeScratch
	merge.beginEquivEpoch()
	merge.ensureMergeHotCaches()
	storeSpineEquivCache(&merge, a, b, true)
	storeGSSStackEquivCache(&merge, a, b, true)
	storeShapePrefixCache(&merge, a, glrMaterializingShapeHash{hash: 9, count: 1})
	storeCleanZeroFrontCache(&merge, a, true)

	merge.invalidateGSSPointersForReuse()

	if got, ok := lookupSpineEquivCache(&merge, a, b); !ok || !got {
		t.Fatalf("fingerprinted spine cache after invalidation = %v, %v; want true, true", got, ok)
	}
	if _, ok := lookupGSSStackEquivCache(&merge, a, b); ok {
		t.Fatal("address-only stack cache survived GSS pointer epoch")
	}
	if _, ok := lookupShapePrefixCache(&merge, a); ok {
		t.Fatal("address-only shape-prefix cache survived GSS reuse invalidation")
	}
	if _, ok := lookupCleanZeroFrontCache(&merge, a); ok {
		t.Fatal("address-only clean-zero cache survived GSS reuse invalidation")
	}

	nodes.recycleForParse()
	a2 := nodes.allocNode(stackEntry{state: 2}, nil, 1)
	b2 := nodes.allocNode(stackEntry{state: 3}, nil, 1)
	if a2 != a || b2 != b {
		t.Fatalf("expected recycled addresses a=%p/%p b=%p/%p", a2, a, b2, b)
	}
	if _, ok := lookupSpineEquivCache(&merge, a2, b2); ok {
		t.Fatal("fingerprinted spine cache hit after recycled addresses changed content")
	}
}
