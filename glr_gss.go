package gotreesitter

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// maxPooledGSSHashWalkCap bounds what gssNodeHashPendingPool will retain, so a
// single very deep chain cannot park a huge backing array in the pool for the
// rest of the process.
const maxPooledGSSHashWalkCap = 1 << 16

// gssNodeHashPendingPool recycles the walk buffer gssNodeHash uses when an
// unhashed chain is longer than its 32-entry inline array.
//
// CLEARING CONTRACT: the buffer holds *gssNode pointers. A pooled slice stays
// reachable from the pool for at least one GC cycle, so returning it with live
// pointers still in its backing array would keep those nodes, and everything
// they reach, uncollectable. The contents are cleared before it goes back.
//
// The buffer used to grow on the heap from scratch on every such call. A chain
// only needs hashing until it reaches an already-hashed node, so this is cheap
// on ordinary parses; on an error path that rebuilds deep GSS chains it is not.
// Profiling a PHP transient-error edit attributed 124 MB of allocation to this
// one walk. Pooling keeps the buffer alive across calls.
var gssNodeHashPendingPool = sync.Pool{
	New: func() any {
		s := make([]*gssNode, 0, 256)
		return &s
	},
}

// resetGSSNodeHashPendingForPool removes node references before pool release.
// It reports false when the buffer should be dropped instead.
func resetGSSNodeHashPendingForPool(pending *[]*gssNode) bool {
	if pending == nil || cap(*pending) > maxPooledGSSHashWalkCap {
		return false
	}
	clear(*pending)
	*pending = (*pending)[:0]
	return true
}

const (
	defaultGSSNodeSlabCap              = 4 * 1024
	fullParseGSSNodeSlabCap            = 32 * 1024
	maxRetainedGSSNodes                = 256 * 1024
	maxRetainedGSSStackEntries         = 4 * 1024
	cRecoverSummaryChunkInitialEntries = 256
	cRecoverSummaryChunkMaxEntries     = 4 * 1024
	maxRetainedGSSSummaryBytes         = 1 << 20
	maxRetainedGSSSummaryChunks        = 16
)

const (
	gssAggCostValid uint8 = 1 << iota
	gssAggVisValid
)

type gssNode struct {
	entry stackEntry
	prev  *gssNode
	hash  uint64

	// Prefix aggregates for the C-recovery cost competition: the cumulative
	// error cost / visible subtree count of the link-0 prev chain root..this
	// node inclusive — the port of C StackNode.error_cost / node_count
	// maintained at push (stack.c stack_node_new), read O(1) by
	// ts_stack_error_cost (stack.c:493). Valid only while the matching gen
	// field equals gssPrefixAggGen (parser_recover_c.go); allocNode resets the
	// generation to 0 (never valid — the counter starts at 1). aggCost is filled
	// by both the parser-side and merge-side walks (identical math); aggVis only
	// by the parser side. aggValid records which values are current.
	aggGen uint64

	// extraLinks points at the backing array for links 1..k after a gated
	// lossless condense merge. Keeping only a pointer and compact length/capacity
	// on the overwhelmingly single-link node avoids paying a slice header on
	// every GSS record. The inline (prev, entry) pair remains link 0.
	extraLinks     *gssMainLink
	aggCost        uint32
	aggVis         int32
	depth          uint32
	extraLinkCount uint8
	extraLinkCap   uint8
	aggValid       uint8
	// cleanZeroState fills the final layout byte. Allocation and recycling
	// reset it. aggGen validates it against recovery-relevant node mutations.
	// These fields do not increase the 64-bit or 32-bit node size.
	cleanZeroState uint8
}

type gssMainLink struct {
	prev  *gssNode
	entry stackEntry
}

const maxMainLinkCount = maxStacksPerMergeKey

// extraLinkCount/extraLinkCap are uint8; the compacted layout is only valid
// while every possible link count fits. This conversion fails to compile if
// maxMainLinkCount ever grows past what uint8 can carry.
const _ = uint8(maxMainLinkCount - 1)

func (n *gssNode) linkCount() int {
	if n == nil {
		return 0
	}
	return 1 + int(n.extraLinkCount)
}

func (n *gssNode) link(i int) (prev *gssNode, entry stackEntry) {
	if i == 0 {
		return n.prev, n.entry
	}
	l := n.extraLink(i - 1)
	return l.prev, l.entry
}

func (n *gssNode) extraLink(i int) gssMainLink {
	if n == nil || i < 0 || i >= int(n.extraLinkCount) || n.extraLinks == nil {
		panic("gssNode.extraLink: index out of range")
	}
	return unsafe.Slice(n.extraLinks, int(n.extraLinkCount))[i]
}

func (n *gssNode) setExtraLink(i int, link gssMainLink) {
	if n == nil || i < 0 || i >= int(n.extraLinkCount) || n.extraLinks == nil {
		panic("gssNode.setExtraLink: index out of range")
	}
	unsafe.Slice(n.extraLinks, int(n.extraLinkCount))[i] = link
	// An extra-link rewrite can change the all-links cleanliness result.
	// Clear the local result before another merge check reads it.
	n.cleanZeroState = gssCleanZeroUnknown
}

func (n *gssNode) appendExtraLink(link gssMainLink) {
	n.appendExtraLinkWithOwner(link, nil)
}

// appendExtraLinkWithOwner bills current packed-link capacity to the scratch
// that owns n. Standalone callers pass nil because they do not own a parser
// scratch budget.
func (n *gssNode) appendExtraLinkWithOwner(link gssMainLink, owner *gssScratch) {
	if n == nil || int(n.extraLinkCount) >= maxMainLinkCount-1 {
		panic("gssNode.appendExtraLink: link limit exceeded")
	}
	n.cleanZeroState = gssCleanZeroUnknown
	count := int(n.extraLinkCount)
	capacity := int(n.extraLinkCap)
	if count < capacity && n.extraLinks != nil {
		unsafe.Slice(n.extraLinks, capacity)[count] = link
		n.extraLinkCount++
		workCountRecordGraphLinkAddition()
		if workCountInstrumentationEnabled {
			workCountTopologyRecordLinkInsert(n, link.prev, count+1, false) // work-count-assembly: topology alternate-link-reuse seam
		}
		workCountRecordAlternatePredecessorLinkAppended() // work-count-assembly: alternate predecessor append-reuse seam
		workCountRecordGSSMutation()                      // work-count-assembly: GSS mutation append-reuse seam
		return
	}
	newCapacity := 1
	if capacity > 0 {
		newCapacity = capacity * 2
	}
	if max := maxMainLinkCount - 1; newCapacity > max {
		newCapacity = max
	}
	links := make([]gssMainLink, newCapacity)
	if count > 0 {
		copy(links, unsafe.Slice(n.extraLinks, count))
	}
	links[count] = link
	n.extraLinks = &links[0]
	n.extraLinkCount++
	n.extraLinkCap = uint8(newCapacity)
	if owner != nil {
		delta := gssMainLinkBytesForCap(newCapacity) - gssMainLinkBytesForCap(capacity)
		owner.allocatedBytes += delta
		owner.packedLinkBytes += delta
	}
	workCountRecordGraphLinkAddition()
	if workCountInstrumentationEnabled {
		workCountTopologyRecordLinkInsert(n, link.prev, count+1, false) // work-count-assembly: topology alternate-link-grow seam
	}
	workCountRecordAlternatePredecessorLinkAppended() // work-count-assembly: alternate predecessor append-grow seam
	workCountRecordGSSMutation()                      // work-count-assembly: GSS mutation append-grow seam
}

// gssStack is a shared-prefix stack foundation for future GLR work.
// Cloning is O(1): clones share the same head pointer until diverging pushes.
type gssStack struct {
	head *gssNode
}

type gssSummaryChunk struct {
	data      []cStackSummaryEntry
	used      int
	dedicated bool
}

type gssSummaryBudgetOwner interface {
	summaryBudgetState() (budgetBytes, baselineBytes, allocatedBytes int64)
}

type forestGSSSummaryBudgetOwner struct {
	arena   *nodeArena
	scratch *gssScratch
}

func (o *forestGSSSummaryBudgetOwner) summaryBudgetState() (int64, int64, int64) {
	if o == nil || o.arena == nil {
		return 0, 0, 0
	}
	allocated := o.arena.allocatedBytes
	if o.scratch != nil {
		allocated += o.scratch.allocatedBytes
	}
	return o.arena.budgetBytes, o.arena.budgetBaselineBytes, allocated
}

type gssScratch struct {
	slabs               []gssNodeSlab
	slabCursor          int
	initialCap          int
	skipClear           bool
	usedTotal           int
	peakUsed            int
	allocatedBytes      int64
	packedLinkBytes     int64
	singleStackMode     bool
	singleStackAllocs   uint64
	multiStackAllocs    uint64
	demotions           uint64
	nodesDemoted        uint64
	audit               *runtimeAudit
	frontier            conflictReduceFrontierScratch
	recoveryElection    cRecoverElectionScratch
	stackEntries        []stackEntry
	summaryChunks       []gssSummaryChunk
	summaryChunkCursor  int
	summaryNextChunkCap int
	summaryBudgetOwner  gssSummaryBudgetOwner
	// everForked latches true the first time this parse observes more than
	// one live GLR stack (see the singleStackMode writers in parser.go, and
	// the explicit early latch at the "len(actions) > 1" conflict-fork
	// decision point), OR enters the faithful C-recovery port (latched at
	// cHandleError's entry in parser_recover_c.go, before it can build or
	// compare a single node — recovery's version-spawning and its raw-shape
	// reads, e.g. cSelectReplacementParentEntry, are not visible to the
	// ordinary singleStackMode bookkeeping since recovery mutates *stacks
	// directly). It is intentionally sticky for the rest of the parse: once
	// true, every later reduction captures its raw shape normally, even if
	// the live stack count later collapses back to one.
	//
	// IMPORTANT: this latch is NOT retroactive. It does not, and cannot, add
	// shapes to nodes that were already built (and left shapeless) before it
	// flipped true. Behavior preservation for those earlier, still-shapeless
	// nodes does not come from this field — it comes from a structural
	// property of GSS/C-recovery sharing (every subsequent stack shares such
	// a node by pointer, so it is only ever compared against itself). See
	// captureRawShape's doc comment (raw_shape.go) for the full argument, and
	// spore.2026-07-12.hazel.rawshape-elision-rca for the originating RCA.
	everForked bool
}

func gssSummaryBytesForCap(n int) (int64, bool) {
	if n < 0 {
		return 0, false
	}
	size := uint64(unsafe.Sizeof(cStackSummaryEntry{}))
	if size == 0 || uint64(n) > uint64(^uint64(0)>>1)/size {
		return 0, false
	}
	return int64(uint64(n) * size), true
}

func (s *gssScratch) summaryBudgetAllows(additional int64) bool {
	if s == nil || additional < 0 {
		return false
	}
	owner := s.summaryBudgetOwner
	if owner == nil {
		return true
	}
	budget, baseline, allocated := owner.summaryBudgetState()
	if budget <= 0 {
		return true
	}
	used := allocated - baseline
	if used < 0 {
		used = 0
	}
	remaining := budget - used
	return remaining > additional
}

func (s *gssScratch) summaryBudgetStopReason() ParseStopReason {
	if s == nil || s.summaryBudgetOwner == nil {
		return ParseStopNone
	}
	budget, baseline, allocated := s.summaryBudgetOwner.summaryBudgetState()
	if budget <= 0 || allocated < baseline {
		return ParseStopNone
	}
	if allocated-baseline >= budget {
		return ParseStopMemoryBudget
	}
	return ParseStopNone
}

// allocRecoverySummary reserves one contiguous, immutable summary region.
// A parse never reuses a region until its owning parser scratch is released.
func (s *gssScratch) allocRecoverySummary(count int) ([]cStackSummaryEntry, ParseStopReason) {
	if count == 0 {
		return nil, ParseStopNone
	}
	if s == nil || count < 0 {
		return nil, ParseStopMemoryBudget
	}
	if reason := s.summaryBudgetStopReason(); reason != ParseStopNone {
		return nil, reason
	}
	if s.summaryNextChunkCap <= 0 {
		s.summaryNextChunkCap = cRecoverSummaryChunkInitialEntries
	}
	for scanned := 0; scanned < len(s.summaryChunks); scanned++ {
		chunkIndex := (s.summaryChunkCursor + scanned) % len(s.summaryChunks)
		chunk := &s.summaryChunks[chunkIndex]
		if !chunk.dedicated && len(chunk.data)-chunk.used >= count {
			start := chunk.used
			chunk.used += count
			s.summaryChunkCursor = chunkIndex
			return chunk.data[start : start+count : start+count], ParseStopNone
		}
	}

	capacity := count
	dedicated := count > cRecoverSummaryChunkMaxEntries
	if !dedicated {
		capacity = s.summaryNextChunkCap
		for capacity < count && capacity < cRecoverSummaryChunkMaxEntries {
			capacity *= 2
		}
		if capacity < count {
			capacity = count
			dedicated = true
		}
	}
	bytes, ok := gssSummaryBytesForCap(capacity)
	if !ok || !s.summaryBudgetAllows(bytes) {
		return nil, ParseStopMemoryBudget
	}
	maxInt64 := int64(^uint64(0) >> 1)
	if s.allocatedBytes > maxInt64-bytes {
		return nil, ParseStopMemoryBudget
	}
	chunk := gssSummaryChunk{data: make([]cStackSummaryEntry, capacity), used: count, dedicated: dedicated}
	s.summaryChunks = append(s.summaryChunks, chunk)
	s.summaryChunkCursor = len(s.summaryChunks) - 1
	if !dedicated {
		if capacity >= cRecoverSummaryChunkMaxEntries {
			s.summaryNextChunkCap = cRecoverSummaryChunkMaxEntries
		} else {
			s.summaryNextChunkCap = capacity * 2
		}
	}
	s.allocatedBytes += bytes
	return chunk.data[:count:count], ParseStopNone
}

// resetRecoverySummary drops dedicated and excess chunks after the parse.
// Callers must clear all live stacks and pending buffers before gssScratch.reset.
func (s *gssScratch) resetRecoverySummary() {
	if s == nil {
		return
	}
	write := 0
	retainedBytes := 0
	for read := range s.summaryChunks {
		chunk := s.summaryChunks[read]
		clear(chunk.data)
		chunk.used = 0
		bytes, ok := gssSummaryBytesForCap(cap(chunk.data))
		keep := write < maxRetainedGSSSummaryChunks && !chunk.dedicated && ok && bytes <= int64(maxRetainedGSSSummaryBytes-retainedBytes)
		if keep {
			retainedBytes += int(bytes)
			s.summaryChunks[write] = chunk
			write++
			continue
		}
		chunk.data = nil
		s.summaryChunks[read] = chunk
	}
	for i := write; i < len(s.summaryChunks); i++ {
		s.summaryChunks[i] = gssSummaryChunk{}
	}
	s.summaryChunks = s.summaryChunks[:write]
	if write == 0 {
		s.summaryChunks = nil
	} else if cap(s.summaryChunks) > maxRetainedGSSSummaryChunks {
		retained := make([]gssSummaryChunk, write)
		copy(retained, s.summaryChunks)
		s.summaryChunks = retained
	}
	s.summaryChunkCursor = 0
	if write == 0 {
		s.summaryNextChunkCap = cRecoverSummaryChunkInitialEntries
	} else {
		capacity := cap(s.summaryChunks[write-1].data)
		if capacity >= cRecoverSummaryChunkMaxEntries {
			s.summaryNextChunkCap = cRecoverSummaryChunkMaxEntries
		} else {
			s.summaryNextChunkCap = capacity * 2
		}
	}
	s.summaryBudgetOwner = nil
}

// rawShapeElisionDisabledForDiagnostics forces every reduction to capture its
// raw shape, even on the pure single-stack happy path, matching parser
// behavior before the single-stack capture elision optimization. It exists so
// differential tests (and offline profiling harnesses) can A/B the
// optimization directly against otherwise-identical parses. Stored as an
// atomic so concurrent parsers observe a consistent value; it remains a
// diagnostics-only hook and normal parser paths leave it disabled.
var rawShapeElisionDisabledForDiagnostics atomic.Bool

// SetRawShapeElisionDisabledForDiagnostics toggles rawShapeElisionDisabledForDiagnostics.
// See its doc comment: this is a diagnostics-only hook, not part of the
// parser's production behavior contract.
func SetRawShapeElisionDisabledForDiagnostics(disabled bool) {
	rawShapeElisionDisabledForDiagnostics.Store(disabled)
}

// mayElideRawShape reports whether it is safe to skip captureRawShape for the
// reduction currently in flight. This is true only on the pure single-stack
// happy path: exactly one GLR stack is live right now, and this parse has
// never forked before. Once a parse forks even once, this permanently
// returns false for the remainder of the parse (see everForked).
func (s *gssScratch) mayElideRawShape() bool {
	return s != nil && s.singleStackMode && !s.everForked && !rawShapeElisionDisabledForDiagnostics.Load()
}

// setSingleStackMode updates singleStackMode and latches everForked the
// moment the parse is observed leaving single-stack mode. everForked is
// sticky for the rest of the parse (cleared only by reset()), even if the
// stack count later collapses back to one.
func (s *gssScratch) setSingleStackMode(v bool) {
	if s == nil {
		return
	}
	s.singleStackMode = v
	if !v {
		s.everForked = true
	}
}

type gssNodeSlab struct {
	data       []gssNode
	reachMarks []uint32
	used       int
}

// ensureReachMarks provisions one external generation mark per retained GSS
// slot. Keep the mark outside gssNode so the node remains 64 bytes.
func (s *gssScratch) ensureReachMarks() {
	if s == nil {
		return
	}
	changed := false
	for i := range s.slabs {
		slab := &s.slabs[i]
		if len(slab.data) == 0 {
			continue
		}
		if cap(slab.reachMarks) < len(slab.data) {
			slab.reachMarks = make([]uint32, len(slab.data))
			changed = true
		} else {
			changed = changed || len(slab.reachMarks) != len(slab.data)
			slab.reachMarks = slab.reachMarks[:len(slab.data)]
		}
	}
	if changed {
		s.recomputeAllocatedBytes()
	}
}

func gssNodeIndexInSlab(slab *gssNodeSlab, nodePtr, nodeSize uintptr) (int, bool) {
	if slab == nil || len(slab.data) == 0 || nodeSize == 0 {
		return 0, false
	}
	base := uintptr(unsafe.Pointer(&slab.data[0]))
	if nodePtr < base {
		return 0, false
	}
	delta := nodePtr - base
	if delta%nodeSize != 0 {
		return 0, false
	}
	index := delta / nodeSize
	if index >= uintptr(len(slab.data)) {
		return 0, false
	}
	return int(index), true
}

func (s *gssScratch) ensureReachMarksForSlab(index int) bool {
	if s == nil || index < 0 || index >= len(s.slabs) {
		return false
	}
	slab := &s.slabs[index]
	if len(slab.data) == 0 {
		return false
	}
	if cap(slab.reachMarks) < len(slab.data) {
		slab.reachMarks = make([]uint32, len(slab.data))
	} else if len(slab.reachMarks) != len(slab.data) {
		slab.reachMarks = slab.reachMarks[:len(slab.data)]
	} else {
		return true
	}
	s.recomputeAllocatedBytes()
	return true
}

func (s *gssScratch) reachMarkForSlab(index int, nodePtr, nodeSize uintptr) (*uint32, bool) {
	if s == nil || index < 0 || index >= len(s.slabs) {
		return nil, false
	}
	slab := &s.slabs[index]
	nodeIndex, ok := gssNodeIndexInSlab(slab, nodePtr, nodeSize)
	if !ok || !s.ensureReachMarksForSlab(index) {
		return nil, false
	}
	return &slab.reachMarks[nodeIndex], true
}

// reachMarkFor returns the external mark for n when n belongs to this GSS
// scratch. A slab that grows after preflight acquisition provisions its marks
// on first use, so owned nodes do not fall back to a pointer map.
func (s *gssScratch) reachMarkFor(n *gssNode, hint *int) (*uint32, bool) {
	if s == nil || n == nil {
		return nil, false
	}
	nodePtr := uintptr(unsafe.Pointer(n))
	nodeSize := unsafe.Sizeof(gssNode{})
	if hint != nil && *hint >= 0 && *hint < len(s.slabs) {
		if mark, ok := s.reachMarkForSlab(*hint, nodePtr, nodeSize); ok {
			return mark, true
		}
	}
	for i := range s.slabs {
		if hint != nil && i == *hint {
			continue
		}
		if mark, ok := s.reachMarkForSlab(i, nodePtr, nodeSize); ok {
			if hint != nil {
				*hint = i
			}
			return mark, true
		}
	}
	return nil, false
}

func (s *gssScratch) clearReachMarks() {
	if s == nil {
		return
	}
	for i := range s.slabs {
		clear(s.slabs[i].reachMarks)
	}
}

const (
	// 64-bit FNV-1a constants.
	gssHashSeed        uint64 = 1469598103934665603
	gssHashPrime       uint64 = 1099511628211
	gssNilNodeSentinel uint64 = 0xff51afd7ed558ccd
)

func gssEntryHash(prev uint64, entry stackEntry) uint64 {
	h := prev ^ uint64(entry.state)
	h *= gssHashPrime

	if entry.node == nil {
		h ^= gssNilNodeSentinel
		h *= gssHashPrime
		return h
	}

	var symbol Symbol
	var startByte, endByte uint32
	var parseState StateID
	var preGotoState StateID
	var productionID uint16
	var childCount int
	var flags nodeFlags
	var dynamicPrecedence int32

	switch entry.kind {
	case stackEntryKindNode:
		n := (*Node)(entry.node)
		symbol = n.symbol
		startByte = n.startByte
		endByte = n.endByte
		parseState = n.parseState
		preGotoState = n.preGotoState
		productionID = n.productionID
		childCount = nodeChildCountNoMaterialize(n)
		flags = n.flags
		dynamicPrecedence = n.dynamicPrecedence
	case stackEntryKindNoTreeNode:
		n := (*noTreeNode)(entry.node)
		symbol = n.symbol
		startByte = n.startByte
		endByte = n.endByte
		parseState = n.parseState
		preGotoState = n.preGotoState
		productionID = n.productionID
		flags = n.flags
		dynamicPrecedence = n.dynamicPrecedence
	case stackEntryKindCompactFullLeaf:
		n := (*compactFullLeaf)(entry.node)
		symbol = n.symbol
		startByte = n.startByte
		endByte = n.endByte
		parseState = n.parseState
		preGotoState = n.preGotoState
		productionID = n.productionID
		flags = n.flags
		dynamicPrecedence = n.dynamicPrecedence
	case stackEntryKindPendingParent:
		n := (*pendingParent)(entry.node)
		symbol = n.symbol
		startByte = n.startByte
		endByte = n.endByte
		parseState = n.parseState
		preGotoState = n.preGotoState
		productionID = n.productionID
		childCount = n.childEntryCount()
		flags = n.flags
		dynamicPrecedence = n.dynamicPrecedence
	default:
		h ^= gssNilNodeSentinel
		h *= gssHashPrime
		return h
	}

	h ^= uint64(symbol)
	h *= gssHashPrime
	h ^= (uint64(startByte) << 32) | uint64(endByte)
	h *= gssHashPrime
	h ^= uint64(parseState)
	h *= gssHashPrime
	h ^= uint64(preGotoState)
	h *= gssHashPrime
	h ^= uint64(productionID)
	h *= gssHashPrime
	h ^= uint64(childCount)
	h *= gssHashPrime
	h ^= uint64(uint32(dynamicPrecedence))
	h *= gssHashPrime
	h ^= gssEntryFlagHash(flags)
	h *= gssHashPrime
	if entry.kind == stackEntryKindNode {
		h = gssNodeShallowMergeHash(h, (*Node)(entry.node))
	}
	return h
}

func gssNodeShallowMergeHash(h uint64, n *Node) uint64 {
	if n == nil {
		h ^= gssNilNodeSentinel
		h *= gssHashPrime
		return h
	}
	fieldIDs := n.fieldIDs()
	h ^= uint64(len(fieldIDs))
	h *= gssHashPrime
	for _, fieldID := range fieldIDs {
		h ^= uint64(fieldID)
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

func gssEntryFlagHash(flags nodeFlags) uint64 {
	var h uint64
	if flags&nodeFlagExtra != 0 {
		h |= 1
	}
	if flags&nodeFlagNamed != 0 {
		h |= 1 << 1
	}
	if flags&nodeFlagHasError != 0 {
		h |= 1 << 2
	}
	if flags&nodeFlagMissing != 0 {
		h |= 1 << 3
	}
	return h
}

func gssNodeHash(n *gssNode) uint64 {
	if n == nil {
		return gssHashSeed
	}
	if n.hash != 0 {
		return n.hash
	}

	const inlineCap = 32
	count := 0
	for ancestor := n; ancestor != nil && ancestor.hash == 0 && count <= inlineCap; ancestor = ancestor.prev {
		count++
	}
	if count <= inlineCap {
		return gssNodeHashInline(n, count)
	}

	// The chain outgrew the inline array. Use pooled storage directly.
	// This keeps the inline array on the stack for ordinary walks.
	pooled := gssNodeHashPendingPool.Get().(*[]*gssNode)
	pending := (*pooled)[:0]
	for cur := n; cur != nil && cur.hash == 0; cur = cur.prev {
		pending = append(pending, cur)
	}
	*pooled = pending
	hash := gssNodeHashPending(pending)
	// Released explicitly rather than with defer: this is a hot path and the
	// function has a single exit once the walk buffer exists.
	if resetGSSNodeHashPendingForPool(pooled) {
		gssNodeHashPendingPool.Put(pooled)
	}
	return hash
}

func gssNodeHashInline(n *gssNode, count int) uint64 {
	var local [32]*gssNode
	pending := local[:count]
	cur := n
	for i := range pending {
		pending[i] = cur
		cur = cur.prev
	}
	return gssNodeHashPending(pending)
}

func gssNodeHashPending(pending []*gssNode) uint64 {
	prevHash := uint64(gssHashSeed)
	if prev := pending[len(pending)-1].prev; prev != nil {
		prevHash = prev.hash
	}
	for i := len(pending) - 1; i >= 0; i-- {
		h := gssEntryHash(prevHash, pending[i].entry)
		if h == 0 {
			h = 1
		}
		pending[i].hash = h
		prevHash = h
	}
	return pending[0].hash
}

func newGSSStack(initial StateID, scratch *gssScratch) gssStack {
	return buildGSSStack([]stackEntry{{state: initial}}, scratch)
}

func buildGSSStack(entries []stackEntry, scratch *gssScratch) gssStack {
	var s gssStack
	for i := range entries {
		s.pushEntry(entries[i], scratch)
	}
	return s
}

func (s gssStack) clone() gssStack {
	return s
}

func (s gssStack) len() int {
	if s.head == nil {
		return 0
	}
	return int(s.head.depth)
}

func (s gssStack) top() stackEntry {
	if s.head == nil {
		return stackEntry{}
	}
	return s.head.entry
}

func (s gssStack) byteOffset() uint32 {
	for n := s.head; n != nil; n = n.prev {
		if stackEntryHasNode(n.entry) {
			return stackEntryNodeEndByte(n.entry)
		}
	}
	return 0
}

func (s *gssStack) push(state StateID, node *Node, scratch *gssScratch) {
	s.pushEntry(newStackEntryNode(state, node), scratch)
}

func (s *gssStack) pushEntry(entry stackEntry, scratch *gssScratch) {
	depth := uint32(1)
	if s.head != nil {
		if s.head.depth == ^uint32(0) {
			panic("gssStack.pushEntry: stack depth overflow")
		}
		depth = s.head.depth + 1
	}
	n := scratch.allocNode(entry, s.head, depth)
	if workCountInstrumentationEnabled {
		workCountTopologyRecordNodeAllocation(n)
		if s.head != nil {
			workCountTopologyRecordLinkInsert(n, s.head, 0, true) // work-count-assembly: topology primary-link seam
		}
	}
	if s.head != nil {
		workCountRecordGraphLinkAddition()
	}
	s.head = n
}

func (s *gssStack) truncate(depth int) bool {
	if depth <= 0 {
		s.head = nil
		return true
	}
	if s.head == nil {
		return depth == 0
	}
	if uint64(depth) > uint64(^uint32(0)) {
		return false
	}
	depth32 := uint32(depth)
	if depth32 > s.head.depth {
		return false
	}
	keep := s.head
	for keep != nil && keep.depth > depth32 {
		keep = keep.prev
	}
	if keep == nil || keep.depth != depth32 {
		return false
	}
	s.head = keep
	return true
}

func (s gssStack) materialize(dst []stackEntry) []stackEntry {
	n := s.len()
	if n == 0 {
		return dst[:0]
	}
	if cap(dst) < n {
		dst = make([]stackEntry, n)
	} else {
		dst = dst[:n]
	}
	i := n - 1
	for node := s.head; node != nil && i >= 0; node = node.prev {
		dst[i] = node.entry
		i--
	}
	if i >= 0 {
		// Invariant violation: depth metadata does not match linked-list length.
		// This indicates internal GSS corruption and is not recoverable here.
		panic("gssStack.materialize: corrupt depth metadata")
	}
	return dst
}

func (s *gssScratch) allocNode(entry stackEntry, prev *gssNode, depth uint32) *gssNode {
	hash := uint64(0)
	if s == nil || !s.singleStackMode {
		prevHash := gssHashSeed
		if prev != nil {
			prevHash = gssNodeHash(prev)
		}
		hash = gssEntryHash(prevHash, entry)
		if hash == 0 {
			hash = 1
		}
	}

	if s == nil {
		return &gssNode{entry: entry, prev: prev, depth: depth, hash: hash}
	}

	// Fast path: current slab has space. Kept small for inlining.
	if cur := s.slabCursor; cur >= 0 && cur < len(s.slabs) {
		slab := &s.slabs[cur]
		if slab.used < len(slab.data) {
			idx := slab.used
			slab.used++
			s.usedTotal++
			if s.usedTotal > s.peakUsed {
				s.peakUsed = s.usedTotal
			}
			if s.singleStackMode {
				s.singleStackAllocs++
			} else {
				s.multiStackAllocs++
			}
			n := &slab.data[idx]
			n.entry = entry
			n.prev = prev
			n.depth = depth
			n.hash = hash
			n.extraLinks = nil
			n.extraLinkCount = 0
			n.extraLinkCap = 0
			n.aggGen = 0
			n.aggValid = 0
			n.cleanZeroState = 0
			return n
		}
	}
	return s.allocNodeSlow(entry, prev, depth, hash)
}

func (s *gssScratch) allocNodeSlow(entry stackEntry, prev *gssNode, depth uint32, hash uint64) *gssNode {
	if len(s.slabs) == 0 {
		capacity := defaultGSSNodeSlabCap
		if s.initialCap > capacity {
			capacity = s.initialCap
		}
		s.slabs = append(s.slabs, gssNodeSlab{data: make([]gssNode, capacity)})
		s.allocatedBytes += gssNodeBytesForCap(capacity)
		s.slabCursor = 0
	}
	if s.slabCursor < 0 || s.slabCursor >= len(s.slabs) {
		s.slabCursor = 0
	}
	for i := s.slabCursor; ; i++ {
		if i >= len(s.slabs) {
			lastCap := defaultGSSNodeSlabCap
			if len(s.slabs) > 0 {
				lastCap = len(s.slabs[len(s.slabs)-1].data)
			}
			capacity := boundedNextSlabCap(lastCap, defaultGSSNodeSlabCap, int(unsafe.Sizeof(gssNode{})))
			s.slabs = append(s.slabs, gssNodeSlab{data: make([]gssNode, capacity)})
			s.allocatedBytes += gssNodeBytesForCap(capacity)
		}
		slab := &s.slabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		slab.used++
		s.usedTotal++
		if s.usedTotal > s.peakUsed {
			s.peakUsed = s.usedTotal
		}
		s.slabCursor = i
		if s.singleStackMode {
			s.singleStackAllocs++
		} else {
			s.multiStackAllocs++
		}
		n := &slab.data[idx]
		n.entry = entry
		n.prev = prev
		n.depth = depth
		n.hash = hash
		n.extraLinks = nil
		n.extraLinkCount = 0
		n.extraLinkCap = 0
		n.aggGen = 0
		n.aggValid = 0
		n.cleanZeroState = 0
		if s.audit != nil {
			s.audit.recordGSSAlloc(n)
		}
		return n
	}
}

// recycleForParse makes every GSS slab slot available to the current parse.
// The caller must first prove that no live parse stack points into the slabs
// and invalidate every pointer-keyed merge/recovery cache. Keeping that proof
// outside this allocator makes accidental address reuse impossible from lower
// level allocation call sites.
func (s *gssScratch) recycleForParse() {
	if s == nil {
		return
	}
	packedLinkBytes := s.packedLinkBytes
	s.packedLinkBytes = 0
	if s.usedTotal == 0 {
		if packedLinkBytes != 0 {
			s.recomputeAllocatedBytes()
		}
		return
	}
	for i := range s.slabs {
		used := s.slabs[i].used
		if used > len(s.slabs[i].data) {
			used = len(s.slabs[i].data)
		}
		clear(s.slabs[i].data[:used])
		if used <= len(s.slabs[i].reachMarks) {
			clear(s.slabs[i].reachMarks[:used])
		}
		s.slabs[i].used = 0
	}
	s.slabCursor = 0
	s.usedTotal = 0
	if packedLinkBytes < 0 || packedLinkBytes > s.allocatedBytes {
		// Repair an inconsistent counter without allowing signed underflow.
		s.recomputeAllocatedBytes()
		return
	}
	s.allocatedBytes -= packedLinkBytes
}

func (s *gssScratch) reset() {
	s.recoveryElection.resetForRelease()
	if cap(s.stackEntries) > maxRetainedGSSStackEntries {
		s.stackEntries = nil
	} else if cap(s.stackEntries) > 0 {
		clear(s.stackEntries[:cap(s.stackEntries)])
		s.stackEntries = s.stackEntries[:0]
	}
	if len(s.slabs) == 0 {
		s.packedLinkBytes = 0
		s.singleStackMode = false
		s.everForked = false
		s.singleStackAllocs = 0
		s.multiStackAllocs = 0
		s.demotions = 0
		s.nodesDemoted = 0
		s.skipClear = false
		s.resetRecoverySummary()
		s.peakUsed = 0
		s.recomputeAllocatedBytes()
		s.audit = nil
		return
	}
	total := 0
	for i := range s.slabs {
		total += len(s.slabs[i].data)
	}
	if total > maxRetainedGSSNodes {
		keepFrom := len(s.slabs) - 1
		retained := len(s.slabs[keepFrom].data)
		if retained > maxRetainedGSSNodes {
			// Even the single most recent slab alone outgrew the retention
			// budget: a pathological wide GLR fork count on one parse (a
			// denser grammar table forking more can drive one parse's GSS
			// far past ordinary steady state). Drop every slab rather than
			// pool an oversized backing array, which would otherwise sit in
			// the process-wide sync.Pool and get billed, unshrunk, to every
			// unrelated future parse that reuses this scratch object
			// regardless of language or file size. The next parse that
			// needs GSS nodes simply reallocates.
			s.slabs = nil
		} else {
			for keepFrom > 0 {
				next := retained + len(s.slabs[keepFrom-1].data)
				if next > maxRetainedGSSNodes {
					break
				}
				keepFrom--
				retained = next
			}
			if keepFrom > 0 {
				oldLen := len(s.slabs)
				copy(s.slabs, s.slabs[keepFrom:])
				newLen := oldLen - keepFrom
				for i := newLen; i < oldLen; i++ {
					s.slabs[i] = gssNodeSlab{}
				}
				s.slabs = s.slabs[:newLen]
			}
		}
	}
	for i := range s.slabs {
		used := s.slabs[i].used
		if used > len(s.slabs[i].data) {
			used = len(s.slabs[i].data)
		}
		clear(s.slabs[i].data[:used])
		if used <= len(s.slabs[i].reachMarks) {
			clear(s.slabs[i].reachMarks[:used])
		}
		s.slabs[i].used = 0
	}
	s.packedLinkBytes = 0
	s.slabCursor = 0
	s.skipClear = false
	s.usedTotal = 0
	s.peakUsed = 0
	s.singleStackMode = false
	s.everForked = false
	s.singleStackAllocs = 0
	s.multiStackAllocs = 0
	s.demotions = 0
	s.nodesDemoted = 0
	s.resetRecoverySummary()
	s.audit = nil
	s.recomputeAllocatedBytes()
}

func (s *glrStack) toGSS(scratch *gssScratch) gssStack {
	if s.gss.head != nil {
		return s.gss.clone()
	}
	return buildGSSStack(s.entries, scratch)
}

func gssSpanIsLinear(head *gssNode, childCount int) bool {
	if childCount == 0 {
		return true
	}
	nonExtraFound := 0
	for n := head; n != nil; n = n.prev {
		if n.extraLinkCount > 0 {
			return false
		}
		if stackEntryHasNode(n.entry) && !stackEntryNodeIsExtra(n.entry) {
			nonExtraFound++
			if nonExtraFound == childCount {
				return true
			}
		}
	}
	return true
}

func gssNodeBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(gssNode{}))
}

func gssMainLinkBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(gssMainLink{}))
}

func (s *gssScratch) capacityNodes() int {
	if s == nil {
		return 0
	}
	var total int
	for i := range s.slabs {
		total += len(s.slabs[i].data)
	}
	return total
}

func (s *gssScratch) recomputeAllocatedBytes() {
	if s == nil {
		return
	}
	var total int64
	for i := range s.slabs {
		total += gssNodeBytesForCap(len(s.slabs[i].data))
		total += gssReachMarkBytesForCap(len(s.slabs[i].reachMarks))
	}
	total += s.packedLinkBytes
	for i := range s.summaryChunks {
		bytes, ok := gssSummaryBytesForCap(cap(s.summaryChunks[i].data))
		if ok {
			total += bytes
		}
	}
	total += s.frontier.allocatedBytes()
	total += s.recoveryElection.allocatedBytes()
	total += stackEntryBytesForCap(cap(s.stackEntries))
	s.allocatedBytes = total
}

func gssReachMarkBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(uint32(0)))
}
