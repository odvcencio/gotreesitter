package gotreesitter

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	// incrementalArenaSlab is sized for steady-state edits where only a small
	// frontier of nodes is rebuilt.
	incrementalArenaSlab = 16 * 1024
	// fullParseArenaSlab matches the current full-parse node footprint with
	// headroom, while remaining small enough to keep a warm pool.
	fullParseArenaSlab = 2 * 1024 * 1024
	minArenaNodeCap    = 64

	// Default capacities for slice backing storage used by reduce actions.
	// Full parses allocate many more parent-child edges than incremental edits.
	incrementalChildSliceCap = 2 * 1024
	fullChildSliceCap        = 64 * 1024
	incrementalFieldSliceCap = 2 * 1024
	fullFieldSliceCap        = 64 * 1024

	maxRetainedArenaFactor          = 4
	maxRetainedFullSliceArenaFactor = 8

	// Node retention ceilings expressed in bytes. maxRetainedNodeCapacityForClass
	// converts these to node counts at runtime using sizeof(Node). This ensures
	// the actual retained memory matches the named byte limit regardless of how
	// Node's size changes over time.
	// Full-parse retention keeps enough warm node storage for the standard full
	// parse benchmark while still capping pathological large-file retention well
	// below the old multi-hundred-MB node-count ceiling.
	maxRetainedIncrementalNodeBytes  = 256 * 1024       // 256 KB per incremental arena
	maxRetainedFullNodeBytes         = 64 * 1024 * 1024 // 64 MB primary node slab
	maxRetainedFullOverflowNodeBytes = 64 * 1024 * 1024 // 64 MB overflow node slabs

	// Slice retention ceilings in element counts (not bytes).
	maxRetainedIncrementalSliceCap = 32 * 1024  // 32 K elements
	maxRetainedFullSliceCap        = 512 * 1024 // 512 K elements

	// Pool eviction ceiling: arenas that grew beyond this byte budget are not
	// returned to the pool. Without this guard a single large parse can leave
	// a 100+ MB arena in the pool indefinitely, consumed by every subsequent
	// parse on that goroutine.
	maxRetainedFullArenaBytes = 128 * 1024 * 1024

	// maxOverflowSlabGrowthBytes bounds geometric overflow-slab growth. Overflow
	// slabs double for cheap amortization on small/medium parses, but unbounded
	// doubling makes the final backing slab of a very large parse as large as
	// everything allocated before it combined: up to ~half of that last slab is
	// never used. Because the per-parse memory budget is charged against slab
	// *capacity* (allocatedBytes), not live usage, that speculative doubled tail
	// could trip the budget hundreds of MB before the live tree actually reached
	// it — the deterministic asm.js-class arena exhaustion (box2d.js /
	// lua_binarytrees.js: enormous flat trees whose last doubled node slab is
	// added in one budget-blowing step). Past this ceiling slabs grow linearly,
	// so peak capacity — and the budget charge — tracks live usage within one
	// ceiling-sized slab. This mirrors the fixed-size slab strategy already used
	// for pendingParent / rawShape / full-parse pendingChildEntry slabs.
	maxOverflowSlabGrowthBytes = 8 * 1024 * 1024
)

type arenaClass uint8

const (
	arenaClassIncremental arenaClass = iota
	arenaClassFull
)

// nodeArena is a slab-backed allocator for Node structs.
// It uses ref counting so trees that borrow reused subtrees can keep arena
// memory alive safely until all dependent trees are released.
type nodeArena struct {
	class            arenaClass
	nodes            []Node
	used             int
	refs             atomic.Int32
	breakdownEnabled bool
	// budgetBytes is a soft per-parse cap for arena backing-storage growth.
	// A value of 0 disables budget checks.
	budgetBytes         int64
	budgetBaselineBytes int64
	allocatedBytes      int64
	parentLinkMu        sync.Mutex
	deferredParentRoot  *Node
	parentLinksDeferred atomic.Bool
	finalChildRefs      bool
	// skipChildClear allows reset() to skip child-slab pointer clearing when
	// a parse did not borrow any external nodes (full parse without reuse).
	skipChildClear bool
	audit          *runtimeAudit
	// internLeaves observes potential leaf-interning hit rates during the
	// parse loop, parseState-BLIND (hooked from newLeafNodeInArena before
	// per-fork state is set). Allocated lazily, reset between parses.
	// Phase 2 measurement; Phase 3 added the state-aware counterpart.
	internLeaves *internTable
	// internLeavesFull is the parseState-AWARE measurement. Hooked at
	// the shift call sites AFTER parseState/preGotoState are set, so the
	// hit rate against this table represents truly dedup-safe duplicates.
	internLeavesFull *internTable
	// internShiftLeafObserved counts leaves allocated by the shift path.
	// Those leaves get parseState set per-fork, so they can't be
	// canonically substituted; the counter is needed to compute the
	// "safe to dedup" leaf population (total leaves minus shift leaves).
	internShiftLeafObserved uint64

	nodeSlabs                       []nodeSlab
	nodeSlabCursor                  int
	noTreeNodeSlabs                 []noTreeNodeSlab
	noTreeNodeSlabCursor            int
	compactFullLeafSlabs            []compactFullLeafSlab
	compactFullLeafSlabCursor       int
	pendingParentSlabs              []pendingParentSlab
	pendingParentSlabCursor         int
	pendingChildEntrySlabs          []pendingChildEntrySlab
	pendingChildEntrySlabCursor     int
	rawShapeSlabs                   []rawShapeSlab
	rawShapeSlabCursor              int
	rawShapeChildSlabs              []rawShapeChildSlab
	rawShapeChildSlabCursor         int
	rawShapeHashCache               []rawShapeHashCacheEntry
	compactCheckpointLeafSlabs      []compactCheckpointLeafSlab
	compactCheckpointLeafSlabCursor int
	finalChildSidecars              []finalChildSidecar
	missingNodeDependencies         []missingNodeDependencyEntry
	nodeFieldMetadataSlabs          []nodeFieldMetadataSlab
	nodeFieldMetadataSlabCursor     int

	childSlabs                         []childSliceSlab
	fieldSlabs                         []fieldSliceSlab
	fieldSourceSlabs                   []fieldSourceSliceSlab
	externalScannerNodeCheckpoints     externalScannerCheckpointSet
	externalScannerNodeCheckpointSlabs []externalScannerCheckpointSlab
	externalScannerCheckpointIdentity  externalScannerCheckpointIdentityState
	hiddenFieldRepeatScratch           hiddenFieldRepeatScratch
	childSlabCursor                    int
	fieldSlabCursor                    int
	fieldSourceSlabCursor              int

	externalScannerCheckpointRecords                 uint64
	externalScannerSnapshotPayloadBytes              uint64
	externalScannerLastSnapshotRef                   externalScannerSnapshotRef
	externalScannerCheckpointLeafNodes               uint64
	compactFullLeafCreated                           uint64
	compactFullLeafMaterialized                      uint64
	compactFullLeafMaterializedForParentReduce       uint64
	compactFullLeafMaterializedForFinalTree          uint64
	compactFullLeafMaterializedForNormalization      uint64
	compactFullLeafMaterializedForRecovery           uint64
	compactFullLeafMaterializedForQuery              uint64
	compactFullLeafMaterializedForCursor             uint64
	compactFullLeafMaterializedForParentAPI          uint64
	compactFullLeafMaterializedForEdit               uint64
	compactFullLeafMaterializedForCheckpointRebuild  uint64
	compactFullLeafMaterializedForParentReject       PendingParentRejectStats
	compactFullLeafMaterializedForFieldRejectPayload PendingParentFieldRejectPayloadStats
	compactFullLeafDropped                           uint64
	pendingParentCreated                             uint64
	pendingParentMaterialized                        uint64
	pendingParentMaterializedForParentReduce         uint64
	pendingParentMaterializedForFinalTree            uint64
	pendingParentMaterializedForNormalization        uint64
	pendingParentMaterializedForRecovery             uint64
	pendingParentMaterializedForQuery                uint64
	pendingParentMaterializedForCursor               uint64
	pendingParentMaterializedForParentAPI            uint64
	pendingParentMaterializedForEdit                 uint64
	pendingParentMaterializedForCheckpointRebuild    uint64
	pendingParentMaterializedForParentReject         PendingParentRejectStats
	pendingParentMaterializedForFieldReject          PendingParentFieldRejectStats
	pendingParentMaterializedForFieldRejectPayload   PendingParentFieldRejectPayloadStats
	pendingParentDropped                             uint64
	pendingParentsFlattened                          uint64
	pendingChildRefsFlattened                        uint64
	pendingChildEntriesAllocated                     uint64
	pendingParentCandidates                          uint64
	pendingParentRejectedEmpty                       uint64
	pendingParentRejectedChildLimit                  uint64
	pendingParentRejectedAlias                       uint64
	pendingParentRejectedRawSpan                     uint64
	pendingParentRejectedFields                      uint64
	pendingParentRejectedFieldsParentHidden          uint64
	pendingParentRejectedFieldsNoIDs                 uint64
	pendingParentRejectedFieldsInherited             uint64
	pendingParentRejectedFieldsHiddenChild           uint64
	pendingParentRejectedFieldsChild                 uint64
	pendingParentRejectedFieldsAllVisibleDirect      uint64
	pendingParentRejectedChild                       uint64
	pendingParentRejectedSpan                        uint64
	pendingParentRejectedFill                        uint64
	finalChildRefParents                             uint64
	finalChildRefsCreated                            uint64
	finalChildRefsMaterializedParents                uint64
	finalChildRefsMaterializedChildren               uint64
	finalChildRefsSingleChildAccesses                uint64
	finalChildRefsSingleChildMaterializedChildren    uint64

	pendingParentLastRejectReason        pendingParentRejectReason
	pendingParentLastFieldRejectShape    pendingParentFieldRejectShape
	pendingParentActiveFieldPayloadShape pendingParentFieldRejectPayloadShape
	pendingParentActiveRejectReason      pendingParentRejectReason
	pendingParentActiveFieldRejectShape  pendingParentFieldRejectShape
	checkpointLeafFullNodesAvoided       uint64
	leafNodesConstructed                 uint64
	parentNodesConstructed               uint64
	fieldedParentNodesConstructed        uint64
	unfieldedParentNodesConstructed      uint64
	parentConstructedChildLen0           uint64
	parentConstructedChildLen1           uint64
	parentConstructedChildLen2           uint64
	parentConstructedChildLen3           uint64
	parentConstructedChildLen4Plus       uint64
	parentConstructedNoLinks             uint64
	parentConstructedWithLinks           uint64
	parentConstructedTrackErrors         uint64
	parentConstructedFieldSources        uint64
	parentReductionVisible               uint64
	parentReductionInvisible             uint64
	parentReductionVisibleFielded        uint64
	parentReductionVisibleUnfielded      uint64
	parentReductionInvisibleFielded      uint64
	parentReductionInvisibleUnfielded    uint64
	parentReductionVisibleChildPointers  uint64
	parentReductionInvisibleChildPtrs    uint64
	parentReductionVisibleChildLen0      uint64
	parentReductionVisibleChildLen1      uint64
	parentReductionVisibleChildLen2      uint64
	parentReductionVisibleChildLen3      uint64
	parentReductionVisibleChildLen4Plus  uint64
	parentReductionInvisibleChildLen0    uint64
	parentReductionInvisibleChildLen1    uint64
	parentReductionInvisibleChildLen2    uint64
	parentReductionInvisibleChildLen3    uint64
	parentReductionInvisibleChildLen4P   uint64
	reduceChildSlicesFastGSS             uint64
	reduceChildPointersFastGSS           uint64
	reduceChildSlicesAllVisible          uint64
	reduceChildPointersAllVisible        uint64
	reduceChildSlicesScratchGeneral      uint64
	reduceChildPointersScratchGeneral    uint64
	reduceChildSlicesScratchNoAlias      uint64
	reduceChildPointersScratchNoAlias    uint64
	collapseRawUnaryAttempts             uint64
	collapseRawUnarySuccesses            uint64
	collapseRawUnaryMissShape            uint64
	collapseRawUnaryMissGrammar          uint64
	collapseRawUnaryMissChild            uint64
	collapseRawUnaryMissRule             uint64
	collapseUnaryAttempts                uint64
	collapseUnarySuccesses               uint64
	collapseUnaryMissShape               uint64
	collapseUnaryMissGrammar             uint64
	collapseUnaryMissFielded             uint64
	collapseUnaryMissChild               uint64
	collapseUnaryMissRule                uint64
	collapseRuleSameSymbol               uint64
	collapseRuleInvisibleWrapper         uint64
	collapseRuleNamedLeafAlias           uint64
	noTreeReduceNodesConstructed         uint64
	noTreeLeafNodesConstructed           uint64
	noTreePlaceholderNodesConstructed    uint64

	pendingParentRejectedFieldsHiddenChildPlain      uint64
	pendingParentRejectedFieldsHiddenChildPlainEmpty uint64
	pendingParentRejectedFieldsHiddenChildPlainOne   uint64
	pendingParentRejectedFieldsHiddenChildPlainMany  uint64
	pendingParentRejectedFieldsHiddenChildWithFields uint64

	// pendingParentErrorRankMemo is the pending-parent counterpart to Node's
	// inline error-rank cache. pendingParent has no spare cache byte and, once
	// its child entries are filled at construction, is never mutated, so
	// pointer identity is sufficient. The map is per-arena because nodeArena
	// is already threaded through every result-rank caller. It is dropped in
	// reset() because pending-parent slabs reuse their pointers across parses.
	pendingParentErrorRankMemo map[*pendingParent]int8

	// forestResultLinkCompareScratch backs the two single-entry glrStack
	// wrappers built by forestResultLinkCompareWithRawShape to reuse the
	// stack-compare machinery for a bare gssLink pair. It is per-arena
	// (nodeArena is already threaded through every forest result-link
	// comparison call, same rationale as pendingParentErrorRankMemo above)
	// so repeated compares during forest coalescing/promotion don't each
	// heap-allocate a fresh one-element []stackEntry. Values are overwritten
	// before every read and the comparison never recurses back into
	// forestResultLinkCompare*, so no cross-call aliasing is possible; no
	// reset() clearing is needed for correctness, only to avoid pinning a
	// stale subtree pointer while the arena sits in the pool.
	forestResultLinkCompareScratch [2]stackEntry
}

type nodeSlab struct {
	data []Node
	used int
}

type childSliceSlab struct {
	data []*Node
	used int
}

type fieldSliceSlab struct {
	data []FieldID
	used int
}

type fieldSourceSliceSlab struct {
	data []uint8
	used int
}

type externalScannerCheckpointSlab struct {
	checkpoints externalScannerCheckpointSet
}

var (
	arenaBreakdownEnabled atomic.Bool

	incrementalArenaPool = nodeArenaPool{
		class:   arenaClassIncremental,
		maxSize: 8,
	}
	fullArenaPool = nodeArenaPool{
		class:   arenaClassFull,
		maxSize: 4,
	}
)

// EnableArenaBreakdown toggles detailed arena accounting for subsequently
// acquired arenas. It is intended for diagnostics and benchmark attribution;
// normal parser paths leave it disabled to avoid perturbing hot allocation
// paths.
func EnableArenaBreakdown(enabled bool) {
	arenaBreakdownEnabled.Store(enabled)
}

type nodeArenaPool struct {
	mu      sync.Mutex
	class   arenaClass
	maxSize int
	free    []*nodeArena
}

// ArenaProfile captures node arena allocation statistics.
// Enable with SetArenaProfileEnabled(true) and retrieve with GetArenaProfile().
type ArenaProfile struct {
	IncrementalAcquire uint64
	IncrementalNew     uint64
	FullAcquire        uint64
	FullNew            uint64
}

var (
	arenaProfileEnabled bool
	arenaProfileData    ArenaProfile
)

// EnableArenaProfile toggles arena pool counters.
// This debug hook is not concurrency-safe and is intended for single-threaded
// benchmark/profiling runs.
func EnableArenaProfile(enabled bool) {
	arenaProfileEnabled = enabled
}

// ResetArenaProfile resets arena pool counters.
// This debug hook is not concurrency-safe and is intended for single-threaded
// benchmark/profiling runs.
func ResetArenaProfile() {
	arenaProfileData = ArenaProfile{}
}

// ArenaProfileSnapshot returns current arena pool counters.
// This debug hook is not concurrency-safe and is intended for single-threaded
// benchmark/profiling runs.
func ArenaProfileSnapshot() ArenaProfile {
	return arenaProfileData
}

func (p *nodeArenaPool) acquire() *nodeArena {
	p.mu.Lock()
	n := len(p.free)
	if n == 0 {
		p.mu.Unlock()
		a := newNodeArena(p.class)
		if arenaProfileEnabled {
			switch p.class {
			case arenaClassIncremental:
				arenaProfileData.IncrementalAcquire++
				arenaProfileData.IncrementalNew++
			default:
				arenaProfileData.FullAcquire++
				arenaProfileData.FullNew++
			}
		}
		return a
	}
	a := p.free[n-1]
	p.free = p.free[:n-1]
	p.mu.Unlock()
	if arenaProfileEnabled {
		switch p.class {
		case arenaClassIncremental:
			arenaProfileData.IncrementalAcquire++
		default:
			arenaProfileData.FullAcquire++
		}
	}
	return a
}

func (p *nodeArenaPool) release(a *nodeArena) {
	if a == nil {
		return
	}
	p.mu.Lock()
	if len(p.free) < p.maxSize {
		p.free = append(p.free, a)
	}
	p.mu.Unlock()
}

// discard clears a stale checkout reference when an arena is rejected instead
// of returned to the pool. Ordinary checkout and successful repooling stay on
// their original hot path. Match the exact arena so concurrent checkouts in
// other unused backing slots retain their own lifecycle bookkeeping.
func (p *nodeArenaPool) discard(a *nodeArena) {
	if a == nil {
		return
	}
	p.mu.Lock()
	all := p.free[:cap(p.free)]
	for i := len(p.free); i < len(all); i++ {
		if all[i] == a {
			all[i] = nil
		}
	}
	p.mu.Unlock()
}

// discardRejectedFullArena keeps oversized-arena eviction bookkeeping out of
// the ordinary nodeArena.Release instruction path.
//
//go:noinline
func discardRejectedFullArena(a *nodeArena) {
	fullArenaPool.discard(a)
}

func (p *nodeArenaPool) drain() {
	p.mu.Lock()
	clear(p.free[:cap(p.free)]) // nil all pointers so GC can collect the arenas
	p.free = p.free[:0]
	p.mu.Unlock()
}

// DrainArenaPools releases all cached arenas from both incremental and full-parse
// pools. Arenas held in the pool are strong Go references and are not collected
// by the GC until explicitly drained or the process exits.
//
// Call this after a large batch scan (e.g. after WalkAndParse returns) to allow
// the GC to reclaim the arena memory. The next parse will allocate a fresh arena.
func DrainArenaPools() {
	incrementalArenaPool.drain()
	fullArenaPool.drain()
}

func nodeCapacityForBytes(slabBytes int) int {
	nodeSize := int(unsafe.Sizeof(Node{}))
	if nodeSize <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / nodeSize
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func newNodeArena(class arenaClass) *nodeArena {
	childCap := fullChildSliceCap
	fieldCap := fullFieldSliceCap
	fieldSourceCap := fullFieldSliceCap
	if class == arenaClassIncremental {
		childCap = incrementalChildSliceCap
		fieldCap = incrementalFieldSliceCap
		fieldSourceCap = incrementalFieldSliceCap
	}
	a := &nodeArena{
		class:            class,
		nodes:            make([]Node, nodeCapacityForClass(class)),
		breakdownEnabled: arenaBreakdownEnabled.Load(),
		childSlabs:       []childSliceSlab{{data: make([]*Node, childCap)}},
		fieldSlabs:       []fieldSliceSlab{{data: make([]FieldID, fieldCap)}},
		fieldSourceSlabs: []fieldSourceSliceSlab{{data: make([]uint8, fieldSourceCap)}},
	}
	a.recomputeAllocatedBytes()
	return a
}

func acquireNodeArena(class arenaClass) *nodeArena {
	var a *nodeArena
	switch class {
	case arenaClassIncremental:
		a = incrementalArenaPool.acquire()
	default:
		a = fullArenaPool.acquire()
	}
	a.refs.Store(1)
	a.breakdownEnabled = arenaBreakdownEnabled.Load()
	a.clearBudget()
	a.audit = nil
	return a
}

func (a *nodeArena) Retain() {
	if a == nil {
		return
	}
	a.refs.Add(1)
}

func (a *nodeArena) Release() {
	if a == nil {
		return
	}
	if a.refs.Add(-1) != 0 {
		return
	}
	// Eviction guard must fire BEFORE reset(). reset() calls recomputeAllocatedBytes()
	// which overwrites allocatedBytes with the post-trim retained value (~1-15 MB).
	// Checking after reset() means an arena that grew to hundreds of MB during a
	// large parse would report a small retained size and slip back into the pool.
	if a.class == arenaClassFull && a.allocatedBytes > maxRetainedFullArenaBytes {
		discardRejectedFullArena(a)
		return // drop; GC collects the backing arrays without reset overhead
	}
	a.reset()
	switch a.class {
	case arenaClassIncremental:
		incrementalArenaPool.release(a)
	default:
		fullArenaPool.release(a)
	}
}

func (a *nodeArena) reset() {
	a.resetPrimaryNodes()
	a.resetParentLinks()
	a.finalChildRefs = false
	a.externalScannerNodeCheckpoints.reset()
	a.resetNodeSlabs()
	a.resetNoTreeNodeSlabs()
	a.resetCompactFullLeafSlabs()
	a.resetPendingParentSlabs()
	a.resetPendingChildEntrySlabs()
	a.resetRawShapeSlabs()
	a.resetRawShapeChildSlabs()
	a.resetRawShapeHashCache()
	a.resetFinalChildSidecars()
	a.resetMissingNodeDependencies()
	a.resetCompactCheckpointLeafSlabs()
	a.resetNodeFieldMetadataSlabs()
	a.resetChildSlabs()
	a.resetCounters()
	a.resetFieldSlabs()
	a.resetFieldSourceSlabs()
	a.externalScannerCheckpointIdentity.clear()
	a.childSlabCursor = 0
	a.fieldSlabCursor = 0
	a.fieldSourceSlabCursor = 0
	a.trimPrimaryNodeCapacity()
	a.ensureDefaultSliceSlabs()
	a.clearBudget()
	// Reset the intern observation tables between parses. Both are
	// allocated lazily on first observation hit when observation is on.
	a.internLeaves.reset()
	a.internLeavesFull.reset()
	a.internShiftLeafObserved = 0
	// pendingParent slab memory is reused across parses on this arena, so a
	// stale entry could otherwise alias a structurally different parent at the
	// same address. Drop the lazily allocated map rather than clear it in place.
	a.pendingParentErrorRankMemo = nil
	// Drop any subtree pointer left in the compare scratch so a pooled arena
	// sitting idle between parses doesn't pin the previous parse's tree.
	a.forestResultLinkCompareScratch = [2]stackEntry{}
	a.hiddenFieldRepeatScratch.reset()
}

func (a *nodeArena) resetPrimaryNodes() {
	primaryUsed := a.used
	if primaryUsed > len(a.nodes) {
		primaryUsed = len(a.nodes)
	}
	clear(a.nodes[:primaryUsed])
	a.used = 0
}

func (a *nodeArena) resetParentLinks() {
	a.parentLinkMu.Lock()
	a.deferredParentRoot = nil
	a.parentLinksDeferred.Store(false)
	a.parentLinkMu.Unlock()
}

func (a *nodeArena) resetNodeSlabs() {
	if len(a.nodeSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedOverflowNodeCapacityForClass(a.class)
		for i := 0; i < len(a.nodeSlabs); i++ {
			capacity := len(a.nodeSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		for i := keep; i < len(a.nodeSlabs); i++ {
			a.nodeSlabs[i] = nodeSlab{}
		}
		a.nodeSlabs = a.nodeSlabs[:keep]
		if len(a.externalScannerNodeCheckpointSlabs) > keep {
			for i := keep; i < len(a.externalScannerNodeCheckpointSlabs); i++ {
				a.externalScannerNodeCheckpointSlabs[i] = externalScannerCheckpointSlab{}
			}
			a.externalScannerNodeCheckpointSlabs = a.externalScannerNodeCheckpointSlabs[:keep]
		}
	}
	for i := range a.nodeSlabs {
		slab := &a.nodeSlabs[i]
		used := slab.used
		if used > len(slab.data) {
			used = len(slab.data)
		}
		clear(slab.data[:used])
		slab.used = 0
		if i < len(a.externalScannerNodeCheckpointSlabs) {
			a.externalScannerNodeCheckpointSlabs[i].checkpoints.reset()
		}
	}
	a.nodeSlabCursor = 0
}

func (a *nodeArena) resetNoTreeNodeSlabs() {
	if len(a.noTreeNodeSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedNoTreeNodeCapacityForClass(a.class)
		for i := 0; i < len(a.noTreeNodeSlabs); i++ {
			capacity := len(a.noTreeNodeSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		for i := keep; i < len(a.noTreeNodeSlabs); i++ {
			a.noTreeNodeSlabs[i] = noTreeNodeSlab{}
		}
		a.noTreeNodeSlabs = a.noTreeNodeSlabs[:keep]
	}
	for i := range a.noTreeNodeSlabs {
		a.noTreeNodeSlabs[i].used = 0
	}
	a.noTreeNodeSlabCursor = 0
}

func (a *nodeArena) resetCompactFullLeafSlabs() {
	if len(a.compactFullLeafSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedCompactFullLeafCapacityForClass(a.class)
		for i := 0; i < len(a.compactFullLeafSlabs); i++ {
			capacity := len(a.compactFullLeafSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		for i := keep; i < len(a.compactFullLeafSlabs); i++ {
			a.compactFullLeafSlabs[i] = compactFullLeafSlab{}
		}
		a.compactFullLeafSlabs = a.compactFullLeafSlabs[:keep]
	}
	for i := range a.compactFullLeafSlabs {
		a.compactFullLeafSlabs[i].used = 0
	}
	a.compactFullLeafSlabCursor = 0
}

func (a *nodeArena) resetPendingParentSlabs() {
	if len(a.pendingParentSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedPendingParentCapacityForClass(a.class)
		for i := 0; i < len(a.pendingParentSlabs); i++ {
			capacity := len(a.pendingParentSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		for i := keep; i < len(a.pendingParentSlabs); i++ {
			a.pendingParentSlabs[i] = pendingParentSlab{}
		}
		a.pendingParentSlabs = a.pendingParentSlabs[:keep]
	}
	for i := range a.pendingParentSlabs {
		slab := &a.pendingParentSlabs[i]
		used := slab.used
		if used > len(slab.data) {
			used = len(slab.data)
		}
		clear(slab.data[:used])
		slab.used = 0
	}
	a.pendingParentSlabCursor = 0
}

func (a *nodeArena) resetPendingChildEntrySlabs() {
	if len(a.pendingChildEntrySlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedPendingChildEntryCapacityForClass(a.class)
		for i := 0; i < len(a.pendingChildEntrySlabs); i++ {
			capacity := len(a.pendingChildEntrySlabs[i].data)
			if capacity <= 0 {
				break
			}
			if retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		for i := keep; i < len(a.pendingChildEntrySlabs); i++ {
			a.pendingChildEntrySlabs[i] = pendingChildEntrySlab{}
		}
		a.pendingChildEntrySlabs = a.pendingChildEntrySlabs[:keep]
	}
	for i := range a.pendingChildEntrySlabs {
		slab := &a.pendingChildEntrySlabs[i]
		if slab.used > 0 {
			clear(slab.data[:slab.used])
			slab.used = 0
		}
	}
	a.pendingChildEntrySlabCursor = 0
}

func (a *nodeArena) resetRawShapeSlabs() {
	if len(a.rawShapeSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedRawShapeCapacityForClass(a.class)
		for i := 0; i < len(a.rawShapeSlabs); i++ {
			capacity := len(a.rawShapeSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		for i := keep; i < len(a.rawShapeSlabs); i++ {
			a.rawShapeSlabs[i] = rawShapeSlab{}
		}
		a.rawShapeSlabs = a.rawShapeSlabs[:keep]
	}
	for i := range a.rawShapeSlabs {
		slab := &a.rawShapeSlabs[i]
		used := slab.used
		if used > len(slab.data) {
			used = len(slab.data)
		}
		clear(slab.data[:used])
		slab.used = 0
	}
	a.rawShapeSlabCursor = 0
}

func (a *nodeArena) resetRawShapeChildSlabs() {
	if len(a.rawShapeChildSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedRawShapeChildCapacityForClass(a.class)
		for i := 0; i < len(a.rawShapeChildSlabs); i++ {
			capacity := len(a.rawShapeChildSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		for i := keep; i < len(a.rawShapeChildSlabs); i++ {
			a.rawShapeChildSlabs[i] = rawShapeChildSlab{}
		}
		a.rawShapeChildSlabs = a.rawShapeChildSlabs[:keep]
	}
	for i := range a.rawShapeChildSlabs {
		slab := &a.rawShapeChildSlabs[i]
		used := slab.used
		if used > len(slab.data) {
			used = len(slab.data)
		}
		clear(slab.data[:used])
		slab.used = 0
	}
	a.rawShapeChildSlabCursor = 0
}

func (a *nodeArena) resetRawShapeHashCache() {
	if a == nil || len(a.rawShapeHashCache) == 0 {
		return
	}
	clear(a.rawShapeHashCache)
}

func (a *nodeArena) resetFinalChildSidecars() {
	if cap(a.finalChildSidecars) > 0 {
		if len(a.finalChildSidecars) > 0 {
			clear(a.finalChildSidecars)
		}
		if limit := maxRetainedFinalChildSidecarCapacityForClass(a.class); cap(a.finalChildSidecars) > limit {
			a.finalChildSidecars = make([]finalChildSidecar, 0, limit)
		} else {
			a.finalChildSidecars = a.finalChildSidecars[:0]
		}
	}
}

func (a *nodeArena) resetCompactCheckpointLeafSlabs() {
	if len(a.compactCheckpointLeafSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedCompactCheckpointLeafCapacityForClass(a.class)
		for i := 0; i < len(a.compactCheckpointLeafSlabs); i++ {
			capacity := len(a.compactCheckpointLeafSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		for i := keep; i < len(a.compactCheckpointLeafSlabs); i++ {
			a.compactCheckpointLeafSlabs[i] = compactCheckpointLeafSlab{}
		}
		a.compactCheckpointLeafSlabs = a.compactCheckpointLeafSlabs[:keep]
	}
	for i := range a.compactCheckpointLeafSlabs {
		a.compactCheckpointLeafSlabs[i].used = 0
	}
	a.compactCheckpointLeafSlabCursor = 0
}

func (a *nodeArena) resetChildSlabs() {
	if len(a.childSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedChildSliceCapacityForClass(a.class)
		for i := 0; i < len(a.childSlabs); i++ {
			capacity := len(a.childSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if keep > 0 && retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		if keep == 0 {
			keep = 1
		}
		for i := keep; i < len(a.childSlabs); i++ {
			a.childSlabs[i] = childSliceSlab{}
		}
		a.childSlabs = a.childSlabs[:keep]
	}
	for i := range a.childSlabs {
		slab := &a.childSlabs[i]
		if !a.skipChildClear {
			clear(slab.data[:slab.used])
		}
		slab.used = 0
	}
	a.skipChildClear = false
}

func (a *nodeArena) resetCounters() {
	a.audit = nil
	a.externalScannerCheckpointRecords = 0
	a.externalScannerSnapshotPayloadBytes = 0
	a.externalScannerLastSnapshotRef = externalScannerSnapshotRef{}
	a.externalScannerCheckpointLeafNodes = 0
	a.compactFullLeafCreated = 0
	a.compactFullLeafMaterialized = 0
	a.compactFullLeafMaterializedForParentReduce = 0
	a.compactFullLeafMaterializedForFinalTree = 0
	a.compactFullLeafMaterializedForNormalization = 0
	a.compactFullLeafMaterializedForRecovery = 0
	a.compactFullLeafMaterializedForQuery = 0
	a.compactFullLeafMaterializedForCursor = 0
	a.compactFullLeafMaterializedForParentAPI = 0
	a.compactFullLeafMaterializedForEdit = 0
	a.compactFullLeafMaterializedForCheckpointRebuild = 0
	a.compactFullLeafMaterializedForParentReject = PendingParentRejectStats{}
	a.compactFullLeafMaterializedForFieldRejectPayload = PendingParentFieldRejectPayloadStats{}
	a.compactFullLeafDropped = 0
	a.pendingParentCreated = 0
	a.pendingParentMaterialized = 0
	a.pendingParentMaterializedForParentReduce = 0
	a.pendingParentMaterializedForFinalTree = 0
	a.pendingParentMaterializedForNormalization = 0
	a.pendingParentMaterializedForRecovery = 0
	a.pendingParentMaterializedForQuery = 0
	a.pendingParentMaterializedForCursor = 0
	a.pendingParentMaterializedForParentAPI = 0
	a.pendingParentMaterializedForEdit = 0
	a.pendingParentMaterializedForCheckpointRebuild = 0
	a.pendingParentMaterializedForParentReject = PendingParentRejectStats{}
	a.pendingParentMaterializedForFieldReject = PendingParentFieldRejectStats{}
	a.pendingParentMaterializedForFieldRejectPayload = PendingParentFieldRejectPayloadStats{}
	a.pendingParentDropped = 0
	a.pendingParentsFlattened = 0
	a.pendingChildRefsFlattened = 0
	a.pendingChildEntriesAllocated = 0
	a.pendingParentCandidates = 0
	a.pendingParentRejectedEmpty = 0
	a.pendingParentRejectedChildLimit = 0
	a.pendingParentRejectedAlias = 0
	a.pendingParentRejectedRawSpan = 0
	a.pendingParentRejectedFields = 0
	a.pendingParentRejectedFieldsParentHidden = 0
	a.pendingParentRejectedFieldsNoIDs = 0
	a.pendingParentRejectedFieldsInherited = 0
	a.pendingParentRejectedFieldsHiddenChild = 0
	a.pendingParentRejectedFieldsHiddenChildPlain = 0
	a.pendingParentRejectedFieldsHiddenChildPlainEmpty = 0
	a.pendingParentRejectedFieldsHiddenChildPlainOne = 0
	a.pendingParentRejectedFieldsHiddenChildPlainMany = 0
	a.pendingParentRejectedFieldsHiddenChildWithFields = 0
	a.pendingParentRejectedFieldsChild = 0
	a.pendingParentRejectedFieldsAllVisibleDirect = 0
	a.pendingParentRejectedChild = 0
	a.pendingParentRejectedSpan = 0
	a.pendingParentRejectedFill = 0
	a.finalChildRefParents = 0
	a.finalChildRefsCreated = 0
	a.finalChildRefsMaterializedParents = 0
	a.finalChildRefsMaterializedChildren = 0
	a.finalChildRefsSingleChildAccesses = 0
	a.finalChildRefsSingleChildMaterializedChildren = 0
	a.pendingParentLastRejectReason = pendingParentRejectUnknown
	a.pendingParentLastFieldRejectShape = pendingParentFieldRejectUnknown
	a.pendingParentActiveFieldPayloadShape = pendingParentFieldRejectPayloadUnknown
	a.pendingParentActiveRejectReason = pendingParentRejectUnknown
	a.pendingParentActiveFieldRejectShape = pendingParentFieldRejectUnknown
	a.checkpointLeafFullNodesAvoided = 0
	a.leafNodesConstructed = 0
	a.parentNodesConstructed = 0
	a.fieldedParentNodesConstructed = 0
	a.unfieldedParentNodesConstructed = 0
	a.parentConstructedChildLen0 = 0
	a.parentConstructedChildLen1 = 0
	a.parentConstructedChildLen2 = 0
	a.parentConstructedChildLen3 = 0
	a.parentConstructedChildLen4Plus = 0
	a.parentConstructedNoLinks = 0
	a.parentConstructedWithLinks = 0
	a.parentConstructedTrackErrors = 0
	a.parentConstructedFieldSources = 0
	a.parentReductionVisible = 0
	a.parentReductionInvisible = 0
	a.parentReductionVisibleFielded = 0
	a.parentReductionVisibleUnfielded = 0
	a.parentReductionInvisibleFielded = 0
	a.parentReductionInvisibleUnfielded = 0
	a.parentReductionVisibleChildPointers = 0
	a.parentReductionInvisibleChildPtrs = 0
	a.parentReductionVisibleChildLen0 = 0
	a.parentReductionVisibleChildLen1 = 0
	a.parentReductionVisibleChildLen2 = 0
	a.parentReductionVisibleChildLen3 = 0
	a.parentReductionVisibleChildLen4Plus = 0
	a.parentReductionInvisibleChildLen0 = 0
	a.parentReductionInvisibleChildLen1 = 0
	a.parentReductionInvisibleChildLen2 = 0
	a.parentReductionInvisibleChildLen3 = 0
	a.parentReductionInvisibleChildLen4P = 0
	a.reduceChildSlicesFastGSS = 0
	a.reduceChildPointersFastGSS = 0
	a.reduceChildSlicesAllVisible = 0
	a.reduceChildPointersAllVisible = 0
	a.reduceChildSlicesScratchGeneral = 0
	a.reduceChildPointersScratchGeneral = 0
	a.reduceChildSlicesScratchNoAlias = 0
	a.reduceChildPointersScratchNoAlias = 0
	a.collapseRawUnaryAttempts = 0
	a.collapseRawUnarySuccesses = 0
	a.collapseRawUnaryMissShape = 0
	a.collapseRawUnaryMissGrammar = 0
	a.collapseRawUnaryMissChild = 0
	a.collapseRawUnaryMissRule = 0
	a.collapseUnaryAttempts = 0
	a.collapseUnarySuccesses = 0
	a.collapseUnaryMissShape = 0
	a.collapseUnaryMissGrammar = 0
	a.collapseUnaryMissFielded = 0
	a.collapseUnaryMissChild = 0
	a.collapseUnaryMissRule = 0
	a.collapseRuleSameSymbol = 0
	a.collapseRuleInvisibleWrapper = 0
	a.collapseRuleNamedLeafAlias = 0
	a.noTreeReduceNodesConstructed = 0
	a.noTreeLeafNodesConstructed = 0
	a.noTreePlaceholderNodesConstructed = 0
}

func (a *nodeArena) resetFieldSlabs() {
	if len(a.fieldSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedFieldSliceCapacityForClass(a.class)
		for i := 0; i < len(a.fieldSlabs); i++ {
			capacity := len(a.fieldSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if keep > 0 && retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		if keep == 0 {
			keep = 1
		}
		for i := keep; i < len(a.fieldSlabs); i++ {
			a.fieldSlabs[i] = fieldSliceSlab{}
		}
		a.fieldSlabs = a.fieldSlabs[:keep]
	}
	for i := range a.fieldSlabs {
		a.fieldSlabs[i].used = 0
	}
}

func (a *nodeArena) resetFieldSourceSlabs() {
	if len(a.fieldSourceSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedFieldSourceSliceCapacityForClass(a.class)
		for i := 0; i < len(a.fieldSourceSlabs); i++ {
			capacity := len(a.fieldSourceSlabs[i].data)
			if capacity <= 0 {
				break
			}
			if keep > 0 && retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		if keep == 0 {
			keep = 1
		}
		for i := keep; i < len(a.fieldSourceSlabs); i++ {
			a.fieldSourceSlabs[i] = fieldSourceSliceSlab{}
		}
		a.fieldSourceSlabs = a.fieldSourceSlabs[:keep]
	}
	for i := range a.fieldSourceSlabs {
		a.fieldSourceSlabs[i].used = 0
	}
}

func (a *nodeArena) trimPrimaryNodeCapacity() {
	if limit := maxRetainedNodeCapacityForClass(a.class); len(a.nodes) > limit {
		// Drop the oversized primary array entirely. Setting to nil lets the GC
		// collect it and the next parse will allocate a fresh slab of default
		// size via allocNodeSlow -> ensureNodeCapacity.
		a.nodes = nil
		a.externalScannerNodeCheckpoints = externalScannerCheckpointSet{}
	}
}

func (a *nodeArena) ensureDefaultSliceSlabs() {
	if len(a.childSlabs) == 0 {
		a.childSlabs = []childSliceSlab{{data: make([]*Node, defaultChildSliceCap(a.class))}}
	}
	if len(a.fieldSlabs) == 0 {
		a.fieldSlabs = []fieldSliceSlab{{data: make([]FieldID, defaultFieldSliceCap(a.class))}}
	}
	if len(a.fieldSourceSlabs) == 0 {
		a.fieldSourceSlabs = []fieldSourceSliceSlab{{data: make([]uint8, defaultFieldSliceCap(a.class))}}
	}
}

func (a *nodeArena) allocNode() *Node {
	if a == nil {
		return &Node{}
	}
	return a.allocNodeFast()
}

func (a *nodeArena) allocNodeFast() *Node {
	if a.used < len(a.nodes) {
		n := &a.nodes[a.used]
		a.used++
		// Node is already zeroed: fresh slabs by make(), reused slabs by reset().
		return n
	}
	return a.allocNodeSlow()
}

func (a *nodeArena) allocNodeSlow() *Node {
	if len(a.nodeSlabs) == 0 {
		capacity := max(nodeCapacityForClass(a.class), minArenaNodeCap)
		a.nodeSlabs = append(a.nodeSlabs, nodeSlab{data: make([]Node, capacity)})
		a.allocatedBytes += nodeBytesForCap(capacity)
		a.nodeSlabCursor = 0
	}
	if a.nodeSlabCursor < 0 || a.nodeSlabCursor >= len(a.nodeSlabs) {
		a.nodeSlabCursor = 0
	}
	for i := a.nodeSlabCursor; ; i++ {
		if i >= len(a.nodeSlabs) {
			lastCap := len(a.nodeSlabs[len(a.nodeSlabs)-1].data)
			capacity := boundedNextSlabCap(lastCap, minArenaNodeCap, int(unsafe.Sizeof(Node{})))
			a.nodeSlabs = append(a.nodeSlabs, nodeSlab{data: make([]Node, capacity)})
			a.allocatedBytes += nodeBytesForCap(capacity)
		}

		slab := &a.nodeSlabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		slab.used++
		a.nodeSlabCursor = i
		a.used++
		n := &slab.data[idx]
		// Node is already zeroed: fresh slabs by make(), reused slabs by reset().
		return n
	}
}

func (a *nodeArena) allocNoTreeNode() *noTreeNode {
	if a == nil {
		return &noTreeNode{}
	}
	if len(a.noTreeNodeSlabs) == 0 {
		capacity := max(defaultNoTreeNodeSlabCap(a.class), minArenaNodeCap)
		a.noTreeNodeSlabs = append(a.noTreeNodeSlabs, noTreeNodeSlab{data: make([]noTreeNode, capacity)})
		a.allocatedBytes += noTreeNodeBytesForCap(capacity)
		a.noTreeNodeSlabCursor = 0
	}
	if a.noTreeNodeSlabCursor < 0 || a.noTreeNodeSlabCursor >= len(a.noTreeNodeSlabs) {
		a.noTreeNodeSlabCursor = 0
	}
	for i := a.noTreeNodeSlabCursor; ; i++ {
		if i >= len(a.noTreeNodeSlabs) {
			lastCap := len(a.noTreeNodeSlabs[len(a.noTreeNodeSlabs)-1].data)
			capacity := boundedNextSlabCap(lastCap, minArenaNodeCap, int(unsafe.Sizeof(noTreeNode{})))
			a.noTreeNodeSlabs = append(a.noTreeNodeSlabs, noTreeNodeSlab{data: make([]noTreeNode, capacity)})
			a.allocatedBytes += noTreeNodeBytesForCap(capacity)
		}

		slab := &a.noTreeNodeSlabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		slab.used++
		a.noTreeNodeSlabCursor = i
		n := &slab.data[idx]
		// Callers must initialize every field. noTreeNode has no pointer fields,
		// so avoiding per-slot clearing cuts large no-tree allocation CPU.
		return n
	}
}

func (a *nodeArena) allocCompactFullLeaf() *compactFullLeaf {
	if a == nil {
		return &compactFullLeaf{}
	}
	if len(a.compactFullLeafSlabs) == 0 {
		capacity := max(defaultCompactFullLeafSlabCap(a.class), minArenaNodeCap)
		a.compactFullLeafSlabs = append(a.compactFullLeafSlabs, compactFullLeafSlab{data: make([]compactFullLeaf, capacity)})
		a.allocatedBytes += compactFullLeafBytesForCap(capacity)
		a.compactFullLeafSlabCursor = 0
	}
	if a.compactFullLeafSlabCursor < 0 || a.compactFullLeafSlabCursor >= len(a.compactFullLeafSlabs) {
		a.compactFullLeafSlabCursor = 0
	}
	for i := a.compactFullLeafSlabCursor; ; i++ {
		if i >= len(a.compactFullLeafSlabs) {
			lastCap := len(a.compactFullLeafSlabs[len(a.compactFullLeafSlabs)-1].data)
			capacity := boundedNextSlabCap(lastCap, minArenaNodeCap, int(unsafe.Sizeof(compactFullLeaf{})))
			a.compactFullLeafSlabs = append(a.compactFullLeafSlabs, compactFullLeafSlab{data: make([]compactFullLeaf, capacity)})
			a.allocatedBytes += compactFullLeafBytesForCap(capacity)
		}

		slab := &a.compactFullLeafSlabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		slab.used++
		a.compactFullLeafSlabCursor = i
		return &slab.data[idx]
	}
}

func (a *nodeArena) allocPendingParent() *pendingParent {
	if a == nil {
		return &pendingParent{}
	}
	if len(a.pendingParentSlabs) == 0 {
		capacity := max(defaultPendingParentSlabCap(a.class), minArenaNodeCap)
		a.pendingParentSlabs = append(a.pendingParentSlabs, pendingParentSlab{data: make([]pendingParent, capacity)})
		a.allocatedBytes += pendingParentBytesForCap(capacity)
		a.pendingParentSlabCursor = 0
	}
	if a.pendingParentSlabCursor < 0 || a.pendingParentSlabCursor >= len(a.pendingParentSlabs) {
		a.pendingParentSlabCursor = 0
	}
	for i := a.pendingParentSlabCursor; ; i++ {
		if i >= len(a.pendingParentSlabs) {
			capacity := max(defaultPendingParentSlabCap(a.class), minArenaNodeCap)
			a.pendingParentSlabs = append(a.pendingParentSlabs, pendingParentSlab{data: make([]pendingParent, capacity)})
			a.allocatedBytes += pendingParentBytesForCap(capacity)
		}

		slab := &a.pendingParentSlabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		slab.used++
		a.pendingParentSlabCursor = i
		return &slab.data[idx]
	}
}

func (a *nodeArena) allocPendingChildEntries(n int) (pendingChildRange, []pendingChildEntry) {
	if n <= 0 {
		return 0, nil
	}
	if a == nil {
		panic("pending child entry ranges require an arena")
	}
	a.pendingChildEntriesAllocated += uint64(n)
	if len(a.pendingChildEntrySlabs) == 0 {
		capacity := max(defaultPendingChildEntrySlabCap(a.class), n)
		a.pendingChildEntrySlabs = append(a.pendingChildEntrySlabs, pendingChildEntrySlab{data: make([]pendingChildEntry, capacity)})
		a.allocatedBytes += pendingChildEntryBytesForCap(capacity)
		a.pendingChildEntrySlabCursor = 0
	}
	if a.pendingChildEntrySlabCursor < 0 || a.pendingChildEntrySlabCursor >= len(a.pendingChildEntrySlabs) {
		a.pendingChildEntrySlabCursor = 0
	}
	for i := a.pendingChildEntrySlabCursor; ; i++ {
		if i >= len(a.pendingChildEntrySlabs) {
			lastCap := len(a.pendingChildEntrySlabs[len(a.pendingChildEntrySlabs)-1].data)
			capacity := nextPendingChildEntrySlabCap(a.class, lastCap, n)
			a.pendingChildEntrySlabs = append(a.pendingChildEntrySlabs, pendingChildEntrySlab{data: make([]pendingChildEntry, capacity)})
			a.allocatedBytes += pendingChildEntryBytesForCap(capacity)
		}

		slab := &a.pendingChildEntrySlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.pendingChildEntrySlabCursor = i
		return newPendingChildRange(i, start, n), slab.data[start:slab.used]
	}
}

func (a *nodeArena) allocRawShape() (rawShapeRef, *rawShape) {
	if a == nil {
		return 0, nil
	}
	if len(a.rawShapeSlabs) == 0 {
		capacity := max(defaultRawShapeSlabCap(a.class), minArenaNodeCap)
		a.rawShapeSlabs = append(a.rawShapeSlabs, rawShapeSlab{data: make([]rawShape, capacity)})
		a.allocatedBytes += rawShapeBytesForCap(capacity)
		a.rawShapeSlabCursor = 0
	}
	if a.rawShapeSlabCursor < 0 || a.rawShapeSlabCursor >= len(a.rawShapeSlabs) {
		a.rawShapeSlabCursor = 0
	}
	for i := a.rawShapeSlabCursor; ; i++ {
		if i+1 >= 1<<(32-rawShapeRefIndexBits) {
			return 0, nil
		}
		if i >= len(a.rawShapeSlabs) {
			capacity := max(defaultRawShapeSlabCap(a.class), minArenaNodeCap)
			a.rawShapeSlabs = append(a.rawShapeSlabs, rawShapeSlab{data: make([]rawShape, capacity)})
			a.allocatedBytes += rawShapeBytesForCap(capacity)
		}
		slab := &a.rawShapeSlabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		if idx >= 1<<rawShapeRefIndexBits {
			return 0, nil
		}
		slab.used++
		a.rawShapeSlabCursor = i
		ref := rawShapeRef((uint32(i+1) << rawShapeRefIndexBits) | uint32(idx))
		return ref, &slab.data[idx]
	}
}

func (a *nodeArena) allocRawShapeChildren(n int) rawShapeChildRange {
	if n <= 0 {
		return 0
	}
	if a == nil {
		panic("raw shape child ranges require an arena")
	}
	if len(a.rawShapeChildSlabs) == 0 {
		capacity := max(defaultRawShapeChildSlabCap(a.class), n)
		a.rawShapeChildSlabs = append(a.rawShapeChildSlabs, rawShapeChildSlab{data: make([]rawShapeChild, capacity)})
		a.allocatedBytes += rawShapeChildBytesForCap(capacity)
		a.rawShapeChildSlabCursor = 0
	}
	if a.rawShapeChildSlabCursor < 0 || a.rawShapeChildSlabCursor >= len(a.rawShapeChildSlabs) {
		a.rawShapeChildSlabCursor = 0
	}
	for i := a.rawShapeChildSlabCursor; ; i++ {
		if i >= len(a.rawShapeChildSlabs) {
			lastCap := len(a.rawShapeChildSlabs[len(a.rawShapeChildSlabs)-1].data)
			capacity := boundedNextSlabCap(lastCap, n, int(unsafe.Sizeof(rawShapeChild{})))
			a.rawShapeChildSlabs = append(a.rawShapeChildSlabs, rawShapeChildSlab{data: make([]rawShapeChild, capacity)})
			a.allocatedBytes += rawShapeChildBytesForCap(capacity)
		}
		slab := &a.rawShapeChildSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.rawShapeChildSlabCursor = i
		return makeRawShapeChildRange(i, start, n)
	}
}

func nextPendingChildEntrySlabCap(class arenaClass, lastCap, min int) int {
	if class != arenaClassFull {
		// Bound the incremental-class geometric tail with the same 8 MB ceiling
		// as the node/leaf overflow slabs (boundedNextSlabCap): double until the
		// ceiling, then grow linearly, while still satisfying an oversized single
		// request (min = slotCount for a wide reduce) via exact-fit. The
		// arenaClassFull path already uses a fixed default-sized overflow slab
		// (never doubles) — the strategy that fixed the box2d/asm.js exhaustion.
		return boundedNextSlabCap(lastCap, min, int(unsafe.Sizeof(pendingChildEntry{})))
	}
	return max(defaultPendingChildEntrySlabCap(class), min)
}

func (a *nodeArena) allocCompactCheckpointLeaf() *compactCheckpointLeaf {
	if a == nil {
		return &compactCheckpointLeaf{}
	}
	if len(a.compactCheckpointLeafSlabs) == 0 {
		capacity := max(defaultCompactCheckpointLeafSlabCap(a.class), minArenaNodeCap)
		a.compactCheckpointLeafSlabs = append(a.compactCheckpointLeafSlabs, compactCheckpointLeafSlab{data: make([]compactCheckpointLeaf, capacity)})
		a.allocatedBytes += compactCheckpointLeafBytesForCap(capacity)
		a.compactCheckpointLeafSlabCursor = 0
	}
	if a.compactCheckpointLeafSlabCursor < 0 || a.compactCheckpointLeafSlabCursor >= len(a.compactCheckpointLeafSlabs) {
		a.compactCheckpointLeafSlabCursor = 0
	}
	for i := a.compactCheckpointLeafSlabCursor; ; i++ {
		if i >= len(a.compactCheckpointLeafSlabs) {
			lastCap := len(a.compactCheckpointLeafSlabs[len(a.compactCheckpointLeafSlabs)-1].data)
			capacity := boundedNextSlabCap(lastCap, minArenaNodeCap, int(unsafe.Sizeof(compactCheckpointLeaf{})))
			a.compactCheckpointLeafSlabs = append(a.compactCheckpointLeafSlabs, compactCheckpointLeafSlab{data: make([]compactCheckpointLeaf, capacity)})
			a.allocatedBytes += compactCheckpointLeafBytesForCap(capacity)
		}

		slab := &a.compactCheckpointLeafSlabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		slab.used++
		a.compactCheckpointLeafSlabCursor = i
		return &slab.data[idx]
	}
}

func (a *nodeArena) ensureNodeCapacity(min int) {
	if a == nil || min <= len(a.nodes) {
		return
	}
	if a.used > 0 {
		// Pre-sizing is only valid before the arena starts serving allocations.
		// Calling this after allocation begins is an internal usage bug.
		panic("ensureNodeCapacity called after arena allocations started")
	}
	newCap := max(len(a.nodes), minArenaNodeCap)
	for newCap < min {
		newCap *= 2
	}
	a.nodes = make([]Node, newCap)
	a.used = 0
	a.nodeSlabs = nil
	a.nodeSlabCursor = 0
	a.externalScannerNodeCheckpoints = externalScannerCheckpointSet{}
	a.externalScannerNodeCheckpointSlabs = nil
	a.recomputeAllocatedBytes()
}

func (a *nodeArena) ensureExactNodeCapacity(min int) {
	if a == nil || min <= len(a.nodes) {
		return
	}
	if a.used > 0 {
		panic("ensureExactNodeCapacity called after arena allocations started")
	}
	newCap := max(min, minArenaNodeCap)
	a.nodes = make([]Node, newCap)
	a.used = 0
	a.nodeSlabs = nil
	a.nodeSlabCursor = 0
	a.externalScannerNodeCheckpoints = externalScannerCheckpointSet{}
	a.externalScannerNodeCheckpointSlabs = nil
	a.recomputeAllocatedBytes()
}

func (a *nodeArena) ensureExternalScannerCheckpointCapacity(min int) {
	if a == nil || min <= 0 {
		return
	}
	a.allocatedBytes += a.externalScannerNodeCheckpoints.ensureCapacity(min)
}

func (a *nodeArena) dropExternalScannerCheckpointStorage() {
	if a == nil {
		return
	}
	before := a.externalScannerCheckpointBytesAllocated()
	a.externalScannerNodeCheckpoints = externalScannerCheckpointSet{}
	a.externalScannerNodeCheckpointSlabs = nil
	a.externalScannerLastSnapshotRef = externalScannerSnapshotRef{}
	if before <= 0 {
		return
	}
	if a.allocatedBytes >= before {
		a.allocatedBytes -= before
		return
	}
	a.recomputeAllocatedBytes()
}

func (a *nodeArena) allocNodeSlice(n int) []*Node {
	return a.allocNodeSliceInternal(n, true)
}

func (a *nodeArena) allocNodeSliceNoClear(n int) []*Node {
	return a.allocNodeSliceInternal(n, false)
}

func (a *nodeArena) allocNodeSliceInternal(n int, clearOut bool) []*Node {
	if n <= 0 {
		return nil
	}
	if a == nil {
		return make([]*Node, n)
	}

	if len(a.childSlabs) == 0 {
		a.childSlabs = append(a.childSlabs, childSliceSlab{data: make([]*Node, defaultChildSliceCap(a.class))})
		a.childSlabCursor = 0
	}
	if a.childSlabCursor < 0 || a.childSlabCursor >= len(a.childSlabs) {
		a.childSlabCursor = 0
	}

	for i := a.childSlabCursor; ; i++ {
		if i >= len(a.childSlabs) {
			capacity := max(defaultChildSliceCap(a.class), n)
			a.childSlabs = append(a.childSlabs, childSliceSlab{data: make([]*Node, capacity)})
			a.allocatedBytes += childSliceBytesForCap(capacity)
		}

		slab := &a.childSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.childSlabCursor = i
		// Cap the slice at its length: callers may `append` to a node's
		// children, and the slab is shared across many parents. Without
		// the 3-index expression the spare capacity reaches into the next
		// parent's children — an in-place append then silently overwrites
		// downstream nodes (e.g. JS while_statement.Child(0) becoming the
		// closing `}` of its own body).
		out := slab.data[start:slab.used:slab.used]
		// Full-parse arena reset can skip bulk child-slab clearing to avoid
		// large memclr work on release. Zero the slice on allocation so reused
		// child slabs never leak stale child pointers into later parses.
		if clearOut {
			clear(out)
		}
		return out
	}
}

func (a *nodeArena) allocFieldIDSlice(n int) []FieldID {
	if n <= 0 {
		return nil
	}
	if a == nil {
		return make([]FieldID, n)
	}

	if len(a.fieldSlabs) == 0 {
		a.fieldSlabs = append(a.fieldSlabs, fieldSliceSlab{data: make([]FieldID, defaultFieldSliceCap(a.class))})
		a.fieldSlabCursor = 0
	}
	if a.fieldSlabCursor < 0 || a.fieldSlabCursor >= len(a.fieldSlabs) {
		a.fieldSlabCursor = 0
	}

	for i := a.fieldSlabCursor; ; i++ {
		if i >= len(a.fieldSlabs) {
			capacity := max(defaultFieldSliceCap(a.class), n)
			a.fieldSlabs = append(a.fieldSlabs, fieldSliceSlab{data: make([]FieldID, capacity)})
			a.allocatedBytes += fieldSliceBytesForCap(capacity)
		}

		slab := &a.fieldSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.fieldSlabCursor = i
		// Cap at len: callers (e.g. parser_result_scala_compilation.go) reslice
		// then append on a parent's field-ID slice. Without the 3-index
		// expression the append can silently overwrite the next parent's field
		// IDs. Same defect class as allocNodeSlice.
		out := slab.data[start:slab.used:slab.used]
		clear(out)
		return out
	}
}

func (a *nodeArena) allocFieldSourceSlice(n int) []uint8 {
	if n <= 0 {
		return nil
	}
	if a == nil {
		return make([]uint8, n)
	}

	if len(a.fieldSourceSlabs) == 0 {
		a.fieldSourceSlabs = append(a.fieldSourceSlabs, fieldSourceSliceSlab{data: make([]uint8, defaultFieldSliceCap(a.class))})
		a.allocatedBytes += fieldSourceSliceBytesForCap(defaultFieldSliceCap(a.class))
		a.fieldSourceSlabCursor = 0
	}
	if a.fieldSourceSlabCursor < 0 || a.fieldSourceSlabCursor >= len(a.fieldSourceSlabs) {
		a.fieldSourceSlabCursor = 0
	}

	for i := a.fieldSourceSlabCursor; ; i++ {
		if i >= len(a.fieldSourceSlabs) {
			capacity := max(defaultFieldSliceCap(a.class), n)
			a.fieldSourceSlabs = append(a.fieldSourceSlabs, fieldSourceSliceSlab{data: make([]uint8, capacity)})
			a.allocatedBytes += fieldSourceSliceBytesForCap(capacity)
		}

		slab := &a.fieldSourceSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.fieldSourceSlabCursor = i
		// Cap at len for the same reason as allocFieldIDSlice — see comment above.
		out := slab.data[start:slab.used:slab.used]
		clear(out)
		return out
	}
}

func defaultChildSliceCap(class arenaClass) int {
	if class == arenaClassIncremental {
		return incrementalChildSliceCap
	}
	return fullChildSliceCap
}

func defaultFieldSliceCap(class arenaClass) int {
	if class == arenaClassIncremental {
		return incrementalFieldSliceCap
	}
	return fullFieldSliceCap
}

func nodeCapacityForClass(class arenaClass) int {
	if class == arenaClassIncremental {
		return nodeCapacityForBytes(incrementalArenaSlab)
	}
	return nodeCapacityForBytes(fullParseArenaSlab)
}

// boundedNextSlabCap returns the capacity for the next geometric overflow slab
// of an element type sized elemSize bytes. Slabs double (cheap amortization for
// small/medium parses) until doubling would exceed maxOverflowSlabGrowthBytes,
// after which they grow linearly at that ceiling. minReq guarantees room for a
// single variable-length request (e.g. a wide rawShapeChild range) that is
// itself larger than the ceiling. Bounding the geometric tail keeps peak slab
// capacity — and therefore the per-parse memory-budget charge levied against
// allocatedBytes — tracking live usage within one ceiling-sized slab instead of
// overshooting by up to ~50% of the largest slab. See maxOverflowSlabGrowthBytes.
func boundedNextSlabCap(lastCap, minReq, elemSize int) int {
	next := lastCap * 2
	if elemSize > 0 {
		ceiling := maxOverflowSlabGrowthBytes / elemSize
		if ceiling < minArenaNodeCap {
			ceiling = minArenaNodeCap
		}
		if next > ceiling {
			next = ceiling
		}
	}
	if next < minReq {
		next = minReq
	}
	if next < minArenaNodeCap {
		next = minArenaNodeCap
	}
	return next
}

func nodeBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(Node{}))
}

func childSliceBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof((*Node)(nil)))
}

func fieldSliceBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(FieldID(0)))
}

func fieldSourceSliceBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n)
}

func externalScannerCheckpointBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(externalScannerCheckpointRef{}))
}

func externalScannerCheckpointIndexBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(uint32(0)))
}

func (a *nodeArena) nodeStructBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	total := nodeBytesForCap(len(a.nodes))
	for i := range a.nodeSlabs {
		total += nodeBytesForCap(len(a.nodeSlabs[i].data))
	}
	return total
}

func (a *nodeArena) childSliceBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.childSlabs {
		total += childSliceBytesForCap(len(a.childSlabs[i].data))
	}
	return total
}

func (a *nodeArena) fieldIDBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.fieldSlabs {
		total += fieldSliceBytesForCap(len(a.fieldSlabs[i].data))
	}
	return total
}

func (a *nodeArena) fieldSourceBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.fieldSourceSlabs {
		total += fieldSourceSliceBytesForCap(len(a.fieldSourceSlabs[i].data))
	}
	return total
}

func (a *nodeArena) noTreeNodeBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.noTreeNodeSlabs {
		total += noTreeNodeBytesForCap(len(a.noTreeNodeSlabs[i].data))
	}
	return total
}

func (a *nodeArena) compactFullLeafBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.compactFullLeafSlabs {
		total += compactFullLeafBytesForCap(len(a.compactFullLeafSlabs[i].data))
	}
	return total
}

func (a *nodeArena) pendingParentBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.pendingParentSlabs {
		total += pendingParentBytesForCap(len(a.pendingParentSlabs[i].data))
	}
	return total
}

func (a *nodeArena) pendingChildEntryBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.pendingChildEntrySlabs {
		total += pendingChildEntryBytesForCap(len(a.pendingChildEntrySlabs[i].data))
	}
	return total
}

func (a *nodeArena) rawShapeBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.rawShapeSlabs {
		total += rawShapeBytesForCap(len(a.rawShapeSlabs[i].data))
	}
	return total
}

func (a *nodeArena) rawShapeChildBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.rawShapeChildSlabs {
		total += rawShapeChildBytesForCap(len(a.rawShapeChildSlabs[i].data))
	}
	return total
}

func (a *nodeArena) rawShapeHashCacheBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	return rawShapeHashCacheBytesForCap(cap(a.rawShapeHashCache))
}

func (a *nodeArena) pendingChildEntryCapacity() uint64 {
	if a == nil {
		return 0
	}
	var total uint64
	for i := range a.pendingChildEntrySlabs {
		total += uint64(len(a.pendingChildEntrySlabs[i].data))
	}
	return total
}

func (a *nodeArena) pendingChildEntryWaste() uint64 {
	capacity := a.pendingChildEntryCapacity()
	if capacity < a.pendingChildEntriesAllocated {
		return 0
	}
	return capacity - a.pendingChildEntriesAllocated
}

func finalChildSidecarBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(finalChildSidecar{}))
}

func (a *nodeArena) finalChildSidecarBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	return finalChildSidecarBytesForCap(cap(a.finalChildSidecars))
}

func (a *nodeArena) compactCheckpointLeafBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.compactCheckpointLeafSlabs {
		total += compactCheckpointLeafBytesForCap(len(a.compactCheckpointLeafSlabs[i].data))
	}
	return total
}

func (a *nodeArena) recomputeAllocatedBytes() {
	if a == nil {
		return
	}
	total := a.nodeStructBytesAllocated() +
		a.nodeFieldMetadataBytesAllocated() +
		a.noTreeNodeBytesAllocated() +
		a.compactFullLeafBytesAllocated() +
		a.pendingParentBytesAllocated() +
		a.pendingChildEntryBytesAllocated() +
		a.rawShapeBytesAllocated() +
		a.rawShapeChildBytesAllocated() +
		a.rawShapeHashCacheBytesAllocated() +
		a.finalChildSidecarBytesAllocated() +
		missingNodeDependencyEntryBytesForCap(cap(a.missingNodeDependencies)) +
		a.compactCheckpointLeafBytesAllocated() +
		a.childSliceBytesAllocated() +
		a.fieldIDBytesAllocated() +
		a.fieldSourceBytesAllocated()
	total += a.externalScannerNodeCheckpoints.bytesAllocated()
	for i := range a.externalScannerNodeCheckpointSlabs {
		total += a.externalScannerNodeCheckpointSlabs[i].checkpoints.bytesAllocated()
	}
	total += a.externalScannerCheckpointIdentity.bytesAllocated()
	a.allocatedBytes = total
}

func (a *nodeArena) recordCompactFullLeafMaterialized(reason materializeReason) {
	if a == nil {
		return
	}
	a.compactFullLeafMaterialized++
	switch reason {
	case materializeForParentReduce:
		a.compactFullLeafMaterializedForParentReduce++
	case materializeForFinalTree:
		a.compactFullLeafMaterializedForFinalTree++
	case materializeForNormalization:
		a.compactFullLeafMaterializedForNormalization++
	case materializeForRecovery:
		a.compactFullLeafMaterializedForRecovery++
	case materializeForQuery:
		a.compactFullLeafMaterializedForQuery++
	case materializeForCursor:
		a.compactFullLeafMaterializedForCursor++
	case materializeForParentAPI:
		a.compactFullLeafMaterializedForParentAPI++
	case materializeForEdit:
		a.compactFullLeafMaterializedForEdit++
	case materializeForCheckpointRebuild:
		a.compactFullLeafMaterializedForCheckpointRebuild++
	}
}

func (a *nodeArena) recordPendingParentMaterialized(reason materializeReason) {
	if a == nil {
		return
	}
	a.pendingParentMaterialized++
	switch reason {
	case materializeForParentReduce:
		a.pendingParentMaterializedForParentReduce++
	case materializeForFinalTree:
		a.pendingParentMaterializedForFinalTree++
	case materializeForNormalization:
		a.pendingParentMaterializedForNormalization++
	case materializeForRecovery:
		a.pendingParentMaterializedForRecovery++
	case materializeForQuery:
		a.pendingParentMaterializedForQuery++
	case materializeForCursor:
		a.pendingParentMaterializedForCursor++
	case materializeForParentAPI:
		a.pendingParentMaterializedForParentAPI++
	case materializeForEdit:
		a.pendingParentMaterializedForEdit++
	case materializeForCheckpointRebuild:
		a.pendingParentMaterializedForCheckpointRebuild++
	}
}

type pendingParentRejectReason uint8

const (
	pendingParentRejectUnknown pendingParentRejectReason = iota
	pendingParentRejectEmpty
	pendingParentRejectChildLimit
	pendingParentRejectAlias
	pendingParentRejectRawSpan
	pendingParentRejectFields
	pendingParentRejectChild
	pendingParentRejectSpan
	pendingParentRejectFill
)

type pendingParentFieldRejectShape uint8

const (
	pendingParentFieldRejectUnknown pendingParentFieldRejectShape = iota
	pendingParentFieldRejectParentHidden
	pendingParentFieldRejectNoIDs
	pendingParentFieldRejectInherited
	pendingParentFieldRejectHiddenChild
	pendingParentFieldRejectHiddenChildPlain
	pendingParentFieldRejectHiddenChildPlainEmpty
	pendingParentFieldRejectHiddenChildPlainOne
	pendingParentFieldRejectHiddenChildPlainMany
	pendingParentFieldRejectHiddenChildWithFields
	pendingParentFieldRejectChild
	pendingParentFieldRejectAllVisibleDirect
)

type pendingParentFieldRejectPayloadShape uint8

const (
	pendingParentFieldRejectPayloadUnknown pendingParentFieldRejectPayloadShape = iota
	pendingParentFieldRejectPayloadVisible
	pendingParentFieldRejectPayloadVisibleFinalLike
	pendingParentFieldRejectPayloadVisibleNestedPayload
	pendingParentFieldRejectPayloadVisibleCompactLeaf
	pendingParentFieldRejectPayloadVisibleFieldedDescendant
	pendingParentFieldRejectPayloadHiddenEmpty
	pendingParentFieldRejectPayloadHiddenOne
	pendingParentFieldRejectPayloadHiddenMany
	pendingParentFieldRejectPayloadHiddenWithFields
)

func (s *PendingParentRejectStats) increment(reason pendingParentRejectReason) {
	if s == nil {
		return
	}
	switch reason {
	case pendingParentRejectEmpty:
		s.Empty++
	case pendingParentRejectChildLimit:
		s.ChildLimit++
	case pendingParentRejectAlias:
		s.Alias++
	case pendingParentRejectRawSpan:
		s.RawSpan++
	case pendingParentRejectFields:
		s.Fields++
	case pendingParentRejectChild:
		s.Child++
	case pendingParentRejectSpan:
		s.Span++
	case pendingParentRejectFill:
		s.Fill++
	default:
		s.Unknown++
	}
}

func (s *PendingParentFieldRejectStats) increment(shape pendingParentFieldRejectShape) {
	if s == nil {
		return
	}
	switch shape {
	case pendingParentFieldRejectParentHidden:
		s.ParentHidden++
	case pendingParentFieldRejectNoIDs:
		s.NoIDs++
	case pendingParentFieldRejectInherited:
		s.Inherited++
	case pendingParentFieldRejectHiddenChild:
		s.HiddenChild++
	case pendingParentFieldRejectHiddenChildPlain:
		s.HiddenChild++
		s.HiddenChildPlain++
	case pendingParentFieldRejectHiddenChildPlainEmpty:
		s.HiddenChild++
		s.HiddenChildPlain++
		s.HiddenChildPlainEmpty++
	case pendingParentFieldRejectHiddenChildPlainOne:
		s.HiddenChild++
		s.HiddenChildPlain++
		s.HiddenChildPlainOne++
	case pendingParentFieldRejectHiddenChildPlainMany:
		s.HiddenChild++
		s.HiddenChildPlain++
		s.HiddenChildPlainMany++
	case pendingParentFieldRejectHiddenChildWithFields:
		s.HiddenChild++
		s.HiddenChildWithFields++
	case pendingParentFieldRejectChild:
		s.Child++
	case pendingParentFieldRejectAllVisibleDirect:
		s.AllVisibleDirect++
	default:
		s.Unknown++
	}
}

func (s *PendingParentFieldRejectPayloadStats) increment(shape pendingParentFieldRejectPayloadShape) {
	if s == nil {
		return
	}
	switch shape {
	case pendingParentFieldRejectPayloadVisible:
		s.Visible++
	case pendingParentFieldRejectPayloadVisibleFinalLike:
		s.Visible++
		s.VisibleFinalLike++
	case pendingParentFieldRejectPayloadVisibleNestedPayload:
		s.Visible++
		s.VisibleNestedPayload++
	case pendingParentFieldRejectPayloadVisibleCompactLeaf:
		s.Visible++
		s.VisibleCompactLeaf++
	case pendingParentFieldRejectPayloadVisibleFieldedDescendant:
		s.Visible++
		s.VisibleFieldedDesc++
	case pendingParentFieldRejectPayloadHiddenEmpty:
		s.HiddenEmpty++
	case pendingParentFieldRejectPayloadHiddenOne:
		s.HiddenOne++
	case pendingParentFieldRejectPayloadHiddenMany:
		s.HiddenMany++
	case pendingParentFieldRejectPayloadHiddenWithFields:
		s.HiddenWithFields++
	default:
		s.Unknown++
	}
}

func (a *nodeArena) recordPendingParentRejected(reason pendingParentRejectReason) {
	if a == nil {
		return
	}
	a.pendingParentLastRejectReason = reason
	switch reason {
	case pendingParentRejectEmpty:
		a.pendingParentRejectedEmpty++
	case pendingParentRejectChildLimit:
		a.pendingParentRejectedChildLimit++
	case pendingParentRejectAlias:
		a.pendingParentRejectedAlias++
	case pendingParentRejectRawSpan:
		a.pendingParentRejectedRawSpan++
	case pendingParentRejectFields:
		a.pendingParentRejectedFields++
	case pendingParentRejectChild:
		a.pendingParentRejectedChild++
	case pendingParentRejectSpan:
		a.pendingParentRejectedSpan++
	case pendingParentRejectFill:
		a.pendingParentRejectedFill++
	}
}

func (a *nodeArena) recordPendingParentFieldRejected(shape pendingParentFieldRejectShape) {
	if a == nil {
		return
	}
	a.pendingParentLastFieldRejectShape = shape
	switch shape {
	case pendingParentFieldRejectParentHidden:
		a.pendingParentRejectedFieldsParentHidden++
	case pendingParentFieldRejectNoIDs:
		a.pendingParentRejectedFieldsNoIDs++
	case pendingParentFieldRejectInherited:
		a.pendingParentRejectedFieldsInherited++
	case pendingParentFieldRejectHiddenChild:
		a.pendingParentRejectedFieldsHiddenChild++
	case pendingParentFieldRejectHiddenChildPlain:
		a.pendingParentRejectedFieldsHiddenChild++
		a.pendingParentRejectedFieldsHiddenChildPlain++
	case pendingParentFieldRejectHiddenChildPlainEmpty:
		a.pendingParentRejectedFieldsHiddenChild++
		a.pendingParentRejectedFieldsHiddenChildPlain++
		a.pendingParentRejectedFieldsHiddenChildPlainEmpty++
	case pendingParentFieldRejectHiddenChildPlainOne:
		a.pendingParentRejectedFieldsHiddenChild++
		a.pendingParentRejectedFieldsHiddenChildPlain++
		a.pendingParentRejectedFieldsHiddenChildPlainOne++
	case pendingParentFieldRejectHiddenChildPlainMany:
		a.pendingParentRejectedFieldsHiddenChild++
		a.pendingParentRejectedFieldsHiddenChildPlain++
		a.pendingParentRejectedFieldsHiddenChildPlainMany++
	case pendingParentFieldRejectHiddenChildWithFields:
		a.pendingParentRejectedFieldsHiddenChild++
		a.pendingParentRejectedFieldsHiddenChildWithFields++
	case pendingParentFieldRejectChild:
		a.pendingParentRejectedFieldsChild++
	case pendingParentFieldRejectAllVisibleDirect:
		a.pendingParentRejectedFieldsAllVisibleDirect++
	}
}

func (a *nodeArena) recordParentRejectPayloadMaterialized(entry stackEntry, reason pendingParentRejectReason) {
	if a == nil {
		return
	}
	if stackEntryCompactFullLeaf(entry) != nil {
		a.compactFullLeafMaterializedForParentReject.increment(reason)
		if reason == pendingParentRejectFields && a.breakdownEnabled {
			a.compactFullLeafMaterializedForFieldRejectPayload.increment(a.pendingParentActiveFieldPayloadShape)
		}
		return
	}
	if stackEntryPendingParent(entry) != nil {
		a.pendingParentMaterializedForParentReject.increment(reason)
		if reason == pendingParentRejectFields && a.breakdownEnabled {
			a.pendingParentMaterializedForFieldReject.increment(a.pendingParentActiveFieldRejectShape)
			a.pendingParentMaterializedForFieldRejectPayload.increment(a.pendingParentActiveFieldPayloadShape)
		}
	}
}

func (a *nodeArena) finalizeCompactFullLeafDropped() {
	if a == nil {
		return
	}
	if a.compactFullLeafCreated >= a.compactFullLeafMaterialized {
		a.compactFullLeafDropped = a.compactFullLeafCreated - a.compactFullLeafMaterialized
		return
	}
	a.compactFullLeafDropped = 0
}

func (a *nodeArena) finalizePendingParentDropped() {
	if a == nil {
		return
	}
	if a.pendingParentCreated >= a.pendingParentMaterialized {
		a.pendingParentDropped = a.pendingParentCreated - a.pendingParentMaterialized
		return
	}
	a.pendingParentDropped = 0
}

func (a *nodeArena) externalScannerCheckpointSlotsAllocated() uint64 {
	if a == nil {
		return 0
	}
	total := a.externalScannerNodeCheckpoints.slotsAllocated()
	for i := range a.externalScannerNodeCheckpointSlabs {
		total += a.externalScannerNodeCheckpointSlabs[i].checkpoints.slotsAllocated()
	}
	return total
}

func (a *nodeArena) externalScannerCheckpointBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	total := a.externalScannerNodeCheckpoints.bytesAllocated()
	for i := range a.externalScannerNodeCheckpointSlabs {
		total += a.externalScannerNodeCheckpointSlabs[i].checkpoints.bytesAllocated()
	}
	return total
}

func (a *nodeArena) collectArenaBreakdown() *ArenaBreakdown {
	if a == nil {
		return nil
	}
	nodeUsage := a.nodeUsageStats()
	fieldSourceElements := a.fieldSourceElementsUsed()
	if payload := a.externalScannerSnapshotPayloadBytes; fieldSourceElements >= payload {
		fieldSourceElements -= payload
	} else {
		fieldSourceElements = 0
	}
	breakdown := &ArenaBreakdown{
		NodeStructBytesAllocated:            a.nodeStructBytesAllocated(),
		NodeFieldMetadataBytesAllocated:     a.nodeFieldMetadataBytesAllocated(),
		NoTreeNodeBytesAllocated:            a.noTreeNodeBytesAllocated(),
		CompactFullLeafBytesAllocated:       a.compactFullLeafBytesAllocated(),
		PendingParentBytesAllocated:         a.pendingParentBytesAllocated(),
		PendingChildEntryBytesAllocated:     a.pendingChildEntryBytesAllocated(),
		RawShapeBytesAllocated:              a.rawShapeBytesAllocated(),
		RawShapeChildBytesAllocated:         a.rawShapeChildBytesAllocated(),
		RawShapeHashCacheBytesAllocated:     a.rawShapeHashCacheBytesAllocated(),
		FinalChildSidecarBytesAllocated:     a.finalChildSidecarBytesAllocated(),
		MissingNodeDependencyBytesAllocated: missingNodeDependencyEntryBytesForCap(cap(a.missingNodeDependencies)),
		CompactCheckpointLeafBytesAllocated: a.compactCheckpointLeafBytesAllocated(),
		MissingNodeDependencyCount:          uint64(len(a.missingNodeDependencies)),
		PendingChildEntriesAllocated:        a.pendingChildEntriesAllocated,
		PendingChildEntryCapacity:           a.pendingChildEntryCapacity(),
		PendingChildEntryWaste:              a.pendingChildEntryWaste(),
		ChildSliceBytesAllocated:            a.childSliceBytesAllocated(),
		FieldIDBytesAllocated:               a.fieldIDBytesAllocated(),
		FieldSourceBytesAllocated:           a.fieldSourceBytesAllocated(),
		ArenaNodesConstructed:               nodeUsage.live,
		NodeLiveCount:                       nodeUsage.live,
		NodeCapacityCount:                   nodeUsage.capacity,
		NodeCapacityWaste:                   nodeUsage.waste,
		PrimaryNodeCapacity:                 nodeUsage.primaryCapacity,
		PrimaryNodeUsed:                     nodeUsage.primaryUsed,
		OverflowNodeCapacity:                nodeUsage.overflowCapacity,
		OverflowNodeUsed:                    nodeUsage.overflowUsed,
		OverflowNodeSlabs:                   nodeUsage.overflowSlabs,
		LargestNodeSlabUsedFraction:         nodeUsage.largestSlabUsedFraction,
		LeafNodesConstructed:                a.leafNodesConstructed,
		ParentNodesConstructed:              a.parentNodesConstructed,
		FieldedParentNodesConstructed:       a.fieldedParentNodesConstructed,
		UnfieldedParentNodesConstructed:     a.unfieldedParentNodesConstructed,
		ParentConstructedChildLen0:          a.parentConstructedChildLen0,
		ParentConstructedChildLen1:          a.parentConstructedChildLen1,
		ParentConstructedChildLen2:          a.parentConstructedChildLen2,
		ParentConstructedChildLen3:          a.parentConstructedChildLen3,
		ParentConstructedChildLen4Plus:      a.parentConstructedChildLen4Plus,
		ParentConstructedNoLinks:            a.parentConstructedNoLinks,
		ParentConstructedWithLinks:          a.parentConstructedWithLinks,
		ParentConstructedTrackErrors:        a.parentConstructedTrackErrors,
		ParentConstructedFieldSources:       a.parentConstructedFieldSources,
		ParentReductionVisible:              a.parentReductionVisible,
		ParentReductionInvisible:            a.parentReductionInvisible,
		ParentReductionVisibleFielded:       a.parentReductionVisibleFielded,
		ParentReductionVisibleUnfielded:     a.parentReductionVisibleUnfielded,
		ParentReductionInvisibleFielded:     a.parentReductionInvisibleFielded,
		ParentReductionInvisibleUnfielded:   a.parentReductionInvisibleUnfielded,
		ParentReductionVisibleChildPtrs:     a.parentReductionVisibleChildPointers,
		ParentReductionInvisibleChildPtrs:   a.parentReductionInvisibleChildPtrs,
		ParentReductionVisibleLen0:          a.parentReductionVisibleChildLen0,
		ParentReductionVisibleLen1:          a.parentReductionVisibleChildLen1,
		ParentReductionVisibleLen2:          a.parentReductionVisibleChildLen2,
		ParentReductionVisibleLen3:          a.parentReductionVisibleChildLen3,
		ParentReductionVisibleLen4Plus:      a.parentReductionVisibleChildLen4Plus,
		ParentReductionInvisibleLen0:        a.parentReductionInvisibleChildLen0,
		ParentReductionInvisibleLen1:        a.parentReductionInvisibleChildLen1,
		ParentReductionInvisibleLen2:        a.parentReductionInvisibleChildLen2,
		ParentReductionInvisibleLen3:        a.parentReductionInvisibleChildLen3,
		ParentReductionInvisibleLen4Plus:    a.parentReductionInvisibleChildLen4P,
		ReduceChildSlicesFastGSS:            a.reduceChildSlicesFastGSS,
		ReduceChildPointersFastGSS:          a.reduceChildPointersFastGSS,
		ReduceChildSlicesAllVisible:         a.reduceChildSlicesAllVisible,
		ReduceChildPointersAllVisible:       a.reduceChildPointersAllVisible,
		ReduceChildSlicesScratchGeneral:     a.reduceChildSlicesScratchGeneral,
		ReduceChildPointersScratchGeneral:   a.reduceChildPointersScratchGeneral,
		ReduceChildSlicesScratchNoAlias:     a.reduceChildSlicesScratchNoAlias,
		ReduceChildPointersScratchNoAlias:   a.reduceChildPointersScratchNoAlias,
		CollapseRawUnaryAttempts:            a.collapseRawUnaryAttempts,
		CollapseRawUnarySuccesses:           a.collapseRawUnarySuccesses,
		CollapseRawUnaryMissShape:           a.collapseRawUnaryMissShape,
		CollapseRawUnaryMissGrammar:         a.collapseRawUnaryMissGrammar,
		CollapseRawUnaryMissChild:           a.collapseRawUnaryMissChild,
		CollapseRawUnaryMissRule:            a.collapseRawUnaryMissRule,
		CollapseUnaryAttempts:               a.collapseUnaryAttempts,
		CollapseUnarySuccesses:              a.collapseUnarySuccesses,
		CollapseUnaryMissShape:              a.collapseUnaryMissShape,
		CollapseUnaryMissGrammar:            a.collapseUnaryMissGrammar,
		CollapseUnaryMissFielded:            a.collapseUnaryMissFielded,
		CollapseUnaryMissChild:              a.collapseUnaryMissChild,
		CollapseUnaryMissRule:               a.collapseUnaryMissRule,
		CollapseRuleSameSymbol:              a.collapseRuleSameSymbol,
		CollapseRuleInvisibleWrapper:        a.collapseRuleInvisibleWrapper,
		CollapseRuleNamedLeafAlias:          a.collapseRuleNamedLeafAlias,
		NoTreeReduceNodesConstructed:        a.noTreeReduceNodesConstructed,
		NoTreeLeafNodesConstructed:          a.noTreeLeafNodesConstructed,
		NoTreePlaceholderNodesConstructed:   a.noTreePlaceholderNodesConstructed,
		ChildPointersConstructed:            a.childPointersUsed(),
		FieldIDElementsConstructed:          a.fieldIDElementsUsed(),
		FieldSourceElementsConstructed:      fieldSourceElements,
	}
	a.scanConstructedNodes(breakdown)
	knownNodes := a.leafNodesConstructed + a.parentNodesConstructed +
		a.noTreePlaceholderNodesConstructed
	if breakdown.ArenaNodesConstructed >= knownNodes {
		breakdown.OtherNodesConstructed = breakdown.ArenaNodesConstructed - knownNodes
	}
	parentsWithChildren := breakdown.ParentChildrenLen1 + breakdown.ParentChildrenLen2 +
		breakdown.ParentChildrenLen3 + breakdown.ParentChildrenLen4Plus
	if breakdown.ParentNodesConstructed >= parentsWithChildren {
		breakdown.ParentChildrenLen0 = breakdown.ParentNodesConstructed - parentsWithChildren
	}
	return breakdown
}

type nodeArenaUsageStats struct {
	live                    uint64
	capacity                uint64
	waste                   uint64
	primaryUsed             uint64
	primaryCapacity         uint64
	overflowUsed            uint64
	overflowCapacity        uint64
	overflowSlabs           uint64
	largestSlabUsedFraction float64
}

func (a *nodeArena) nodeUsageStats() nodeArenaUsageStats {
	var stats nodeArenaUsageStats
	if a == nil {
		return stats
	}
	primaryUsed := a.used
	if primaryUsed > len(a.nodes) {
		primaryUsed = len(a.nodes)
	}
	stats.primaryUsed = uint64(primaryUsed)
	stats.primaryCapacity = uint64(len(a.nodes))
	stats.live = stats.primaryUsed
	stats.capacity = stats.primaryCapacity
	largestSlabCapacity := len(a.nodes)
	stats.largestSlabUsedFraction = nodeSlabUsedFraction(primaryUsed, largestSlabCapacity)
	for i := range a.nodeSlabs {
		slab := &a.nodeSlabs[i]
		capacity := len(slab.data)
		used := slab.used
		if used > capacity {
			used = capacity
		}
		stats.overflowUsed += uint64(used)
		stats.overflowCapacity += uint64(capacity)
		stats.overflowSlabs++
		if capacity > largestSlabCapacity {
			largestSlabCapacity = capacity
			stats.largestSlabUsedFraction = nodeSlabUsedFraction(used, capacity)
		}
	}
	stats.live += stats.overflowUsed
	stats.capacity += stats.overflowCapacity
	if stats.capacity >= stats.live {
		stats.waste = stats.capacity - stats.live
	}
	return stats
}

func nodeSlabUsedFraction(used, capacity int) float64 {
	if capacity <= 0 {
		return 0
	}
	return float64(used) / float64(capacity)
}

func (a *nodeArena) childPointersUsed() uint64 {
	if a == nil {
		return 0
	}
	var total uint64
	for i := range a.childSlabs {
		used := a.childSlabs[i].used
		if used > len(a.childSlabs[i].data) {
			used = len(a.childSlabs[i].data)
		}
		total += uint64(used)
	}
	return total
}

func (a *nodeArena) fieldIDElementsUsed() uint64 {
	if a == nil {
		return 0
	}
	var total uint64
	for i := range a.fieldSlabs {
		used := a.fieldSlabs[i].used
		if used > len(a.fieldSlabs[i].data) {
			used = len(a.fieldSlabs[i].data)
		}
		total += uint64(used)
	}
	return total
}

func (a *nodeArena) fieldSourceElementsUsed() uint64 {
	if a == nil {
		return 0
	}
	var total uint64
	for i := range a.fieldSourceSlabs {
		used := a.fieldSourceSlabs[i].used
		if used > len(a.fieldSourceSlabs[i].data) {
			used = len(a.fieldSourceSlabs[i].data)
		}
		total += uint64(used)
	}
	return total
}

func (a *nodeArena) scanConstructedNodes(breakdown *ArenaBreakdown) {
	if a == nil || breakdown == nil {
		return
	}
	primaryUsed := a.used
	if primaryUsed > len(a.nodes) {
		primaryUsed = len(a.nodes)
	}
	a.scanNodeRange(a.nodes[:primaryUsed], breakdown)
	for i := range a.nodeSlabs {
		slab := &a.nodeSlabs[i]
		used := slab.used
		if used > len(slab.data) {
			used = len(slab.data)
		}
		a.scanNodeRange(slab.data[:used], breakdown)
	}
}

func (a *nodeArena) scanNodeRange(nodes []Node, breakdown *ArenaBreakdown) {
	for i := range nodes {
		n := &nodes[i]
		if n.symbol == errorSymbol {
			breakdown.ErrorSymbolNodesConstructed++
		}
		if n.isExtra() {
			breakdown.ExtraNodesConstructed++
		}
		if n.hasError() {
			breakdown.HasErrorNodesConstructed++
		}
		childCount := len(n.children)
		if childCount <= 0 {
			continue
		}
		breakdown.ChildSlicesConstructed++
		breakdown.ParentChildPointersConstructed += uint64(childCount)
		switch childCount {
		case 1:
			breakdown.ChildSlicesLen1++
			breakdown.ParentChildrenLen1++
		case 2:
			breakdown.ChildSlicesLen2++
			breakdown.ParentChildrenLen2++
		case 3:
			breakdown.ChildSlicesLen3++
			breakdown.ParentChildrenLen3++
		default:
			breakdown.ChildSlicesLen4Plus++
			breakdown.ParentChildrenLen4Plus++
		}
	}
}

func (a *nodeArena) allocExternalScannerSnapshotRef(src []byte) externalScannerSnapshotRef {
	n := len(src)
	if a == nil || n == 0 {
		return externalScannerSnapshotRef{}
	}

	if len(a.fieldSourceSlabs) == 0 {
		a.fieldSourceSlabs = append(a.fieldSourceSlabs, fieldSourceSliceSlab{data: make([]uint8, defaultFieldSliceCap(a.class))})
		a.allocatedBytes += fieldSourceSliceBytesForCap(defaultFieldSliceCap(a.class))
		a.fieldSourceSlabCursor = 0
	}
	if a.fieldSourceSlabCursor < 0 || a.fieldSourceSlabCursor >= len(a.fieldSourceSlabs) {
		a.fieldSourceSlabCursor = 0
	}

	for i := a.fieldSourceSlabCursor; ; i++ {
		if i >= len(a.fieldSourceSlabs) {
			capacity := max(defaultFieldSliceCap(a.class), n)
			a.fieldSourceSlabs = append(a.fieldSourceSlabs, fieldSourceSliceSlab{data: make([]uint8, capacity)})
			a.allocatedBytes += fieldSourceSliceBytesForCap(capacity)
		}

		slab := &a.fieldSourceSlabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		a.fieldSourceSlabCursor = i
		copy(slab.data[start:slab.used], src)
		a.externalScannerSnapshotPayloadBytes += uint64(n)
		return externalScannerSnapshotRef{
			slab: uint16(i),
			off:  uint32(start),
			len:  uint16(n),
		}
	}
}

func (a *nodeArena) externalScannerSnapshotBytes(ref externalScannerSnapshotRef) []byte {
	if !a.externalScannerSnapshotRefValid(ref) {
		return nil
	}
	slab := a.fieldSourceSlabs[ref.slab].data
	start := int(ref.off)
	end := start + int(ref.len)
	return slab[start:end]
}

func (a *nodeArena) externalScannerSnapshotRefValid(ref externalScannerSnapshotRef) bool {
	if a == nil || ref.len == 0 || int(ref.slab) >= len(a.fieldSourceSlabs) {
		return false
	}
	slab := &a.fieldSourceSlabs[ref.slab]
	used := slab.used
	if used < 0 || used > len(slab.data) {
		return false
	}
	start := uint64(ref.off)
	end := start + uint64(ref.len)
	return start <= uint64(used) && end <= uint64(used)
}

func (a *nodeArena) clearBudget() {
	if a == nil {
		return
	}
	a.budgetBytes = 0
	a.budgetBaselineBytes = 0
	a.recomputeAllocatedBytes()
}

func (a *nodeArena) setBudget(bytes int64) {
	if a == nil {
		return
	}
	a.budgetBytes = bytes
	a.budgetBaselineBytes = a.allocatedBytes
}

func (a *nodeArena) budgetExhausted() bool {
	if a == nil || a.budgetBytes <= 0 {
		return false
	}
	used := a.allocatedBytes - a.budgetBaselineBytes
	if used < 0 {
		used = 0
	}
	return used >= a.budgetBytes
}

func maxRetainedNodeCapacityForClass(class arenaClass) int {
	nodeSize := int(unsafe.Sizeof(Node{}))
	if nodeSize <= 0 {
		nodeSize = 1
	}
	// Full-parse arenas: hard ceiling in bytes. The factor-based path from the
	// previous node-count cap could retain hundreds of MB, so use the byte ceiling
	// directly while keeping enough warm capacity for normal full-parse reuse.
	if class == arenaClassFull {
		return maxRetainedFullNodeBytes / nodeSize
	}
	// Incremental arenas: retain at least the byte floor, but also allow the
	// factor-based path if it is larger (handles workloads that repeatedly
	// parse files that need more than the floor).
	floor := maxRetainedIncrementalNodeBytes / nodeSize
	return max(nodeCapacityForClass(class)*maxRetainedArenaFactor, floor)
}

func maxRetainedOverflowNodeCapacityForClass(class arenaClass) int {
	if class == arenaClassFull {
		return max(nodeCapacityForBytes(maxRetainedFullOverflowNodeBytes), nodeCapacityForClass(class))
	}
	return max(maxRetainedNodeCapacityForClass(class)/2, nodeCapacityForClass(class))
}

func maxRetainedNoTreeNodeCapacityForClass(class arenaClass) int {
	nodeSize := int(unsafe.Sizeof(noTreeNode{}))
	if nodeSize <= 0 {
		nodeSize = 1
	}
	if class == arenaClassFull {
		return max(maxRetainedFullNodeBytes/nodeSize, defaultNoTreeNodeSlabCap(class))
	}
	floor := maxRetainedIncrementalNodeBytes / nodeSize
	return max(defaultNoTreeNodeSlabCap(class)*maxRetainedArenaFactor, floor)
}

func maxRetainedCompactFullLeafCapacityForClass(class arenaClass) int {
	nodeSize := int(unsafe.Sizeof(compactFullLeaf{}))
	if nodeSize <= 0 {
		nodeSize = 1
	}
	if class == arenaClassFull {
		return max(maxRetainedFullNodeBytes/nodeSize, defaultCompactFullLeafSlabCap(class))
	}
	floor := maxRetainedIncrementalNodeBytes / nodeSize
	return max(defaultCompactFullLeafSlabCap(class)*maxRetainedArenaFactor, floor)
}

func maxRetainedPendingParentCapacityForClass(class arenaClass) int {
	nodeSize := int(unsafe.Sizeof(pendingParent{}))
	if nodeSize <= 0 {
		nodeSize = 1
	}
	if class == arenaClassFull {
		return max(maxRetainedFullNodeBytes/nodeSize, defaultPendingParentSlabCap(class))
	}
	floor := maxRetainedIncrementalNodeBytes / nodeSize
	return max(defaultPendingParentSlabCap(class)*maxRetainedArenaFactor, floor)
}

func maxRetainedPendingChildEntryCapacityForClass(class arenaClass) int {
	nodeSize := int(unsafe.Sizeof(pendingChildEntry{}))
	if nodeSize <= 0 {
		nodeSize = 1
	}
	if class == arenaClassFull {
		return max(maxRetainedFullSliceCap, defaultPendingChildEntrySlabCap(class))
	}
	return max(defaultPendingChildEntrySlabCap(class)*maxRetainedArenaFactor, maxRetainedIncrementalSliceCap)
}

func maxRetainedRawShapeCapacityForClass(class arenaClass) int {
	nodeSize := int(unsafe.Sizeof(rawShape{}))
	if nodeSize <= 0 {
		nodeSize = 1
	}
	if class == arenaClassFull {
		return max(maxRetainedFullSliceCap, defaultRawShapeSlabCap(class))
	}
	floor := maxRetainedIncrementalSliceCap
	return max(defaultRawShapeSlabCap(class)*maxRetainedArenaFactor, floor)
}

func maxRetainedRawShapeChildCapacityForClass(class arenaClass) int {
	nodeSize := int(unsafe.Sizeof(rawShapeChild{}))
	if nodeSize <= 0 {
		nodeSize = 1
	}
	if class == arenaClassFull {
		return max(maxRetainedFullSliceCap, defaultRawShapeChildSlabCap(class))
	}
	floor := maxRetainedIncrementalSliceCap
	return max(defaultRawShapeChildSlabCap(class)*maxRetainedArenaFactor, floor)
}

func maxRetainedFinalChildSidecarCapacityForClass(class arenaClass) int {
	if class == arenaClassFull {
		return maxRetainedFullSliceCap
	}
	return maxRetainedIncrementalSliceCap
}

func maxRetainedCompactCheckpointLeafCapacityForClass(class arenaClass) int {
	nodeSize := int(unsafe.Sizeof(compactCheckpointLeaf{}))
	if nodeSize <= 0 {
		nodeSize = 1
	}
	if class == arenaClassFull {
		return max(maxRetainedFullNodeBytes/nodeSize, defaultCompactCheckpointLeafSlabCap(class))
	}
	floor := maxRetainedIncrementalNodeBytes / nodeSize
	return max(defaultCompactCheckpointLeafSlabCap(class)*maxRetainedArenaFactor, floor)
}

func maxRetainedChildSliceCapacityForClass(class arenaClass) int {
	factor := maxRetainedArenaFactor
	floor := maxRetainedIncrementalSliceCap
	if class == arenaClassFull {
		factor = maxRetainedFullSliceArenaFactor
		floor = maxRetainedFullSliceCap
	}
	return max(defaultChildSliceCap(class)*factor, floor)
}

func maxRetainedFieldSliceCapacityForClass(class arenaClass) int {
	factor := maxRetainedArenaFactor
	floor := maxRetainedIncrementalSliceCap
	if class == arenaClassFull {
		factor = maxRetainedFullSliceArenaFactor
		floor = maxRetainedFullSliceCap
	}
	return max(defaultFieldSliceCap(class)*factor, floor)
}

func maxRetainedFieldSourceSliceCapacityForClass(class arenaClass) int {
	factor := maxRetainedArenaFactor
	floor := maxRetainedIncrementalSliceCap
	if class == arenaClassFull {
		factor = maxRetainedFullSliceArenaFactor
		floor = maxRetainedFullSliceCap
	}
	return max(defaultFieldSliceCap(class)*factor, floor)
}
