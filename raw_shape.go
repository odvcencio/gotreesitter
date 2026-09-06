package gotreesitter

import "unsafe"

type rawShapeRef uint32

const (
	rawShapeRefIndexBits = 20
	rawShapeRefArenaBase = rawShapeRef(1 << rawShapeRefIndexBits)
	// The child range stores its count in 16 bits. Reserve the saturated value
	// as unknown so a larger raw child array can never collide with it.
	rawShapeMaxExactChildCount = int(^uint16(0)) - 1
)

// rawShapeZeroChildRef authenticates C Subtree.child_count == 0 without an
// arena allocation. Arena-backed refs start at rawShapeRefArenaBase, so this
// value cannot alias a stored raw shape. Zero remains unknown or elided.
const rawShapeZeroChildRef rawShapeRef = 1

func invalidateRawShapeAfterChildMutation(node *Node) {
	if node != nil {
		node.rawShape = 0
	}
}

func rawShapeRefIsArenaBacked(ref rawShapeRef) bool {
	return ref >= rawShapeRefArenaBase
}

type rawShape struct {
	// The child count is encoded in the range's low 16 bits, so do not store it
	// again. Large GLR parses retain millions of these headers.
	childRange   rawShapeChildRange
	symbol       Symbol
	productionID uint16
}

// rawShapeHashCacheEntry keeps the original 64-bit shape fingerprint outside
// the per-shape header. A direct-mapped cache bounds its memory cost while
// preserving the old hash width and collision behavior.
type rawShapeHashCacheEntry struct {
	ref  rawShapeRef
	hash uint64
}

const (
	rawShapeHashCacheBits = 15
	rawShapeHashCacheSize = 1 << rawShapeHashCacheBits
)

func rawShapeHashCacheBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(rawShapeHashCacheEntry{}))
}

type rawShapeChild struct {
	// packedEntry.state stores the per-edge rawShapeRef snapshot. Raw-shape
	// consumers inspect payload identity/kind but never LR state; packing the
	// two uint32 values into the existing stackEntry slot keeps this record at
	// 16 bytes instead of 24. entry() restores a meaningful current state for
	// callers so the packed representation cannot escape accidentally.
	packedEntry stackEntry
}

func newRawShapeChild(entry stackEntry) rawShapeChild {
	ref := stackEntryRawShapeRef(entry)
	entry.state = StateID(ref)
	return rawShapeChild{packedEntry: entry}
}

func (c rawShapeChild) entry() stackEntry {
	entry := c.packedEntry
	entry.state = stackEntryNodeParseState(entry)
	return entry
}

func (c rawShapeChild) shapeRef() rawShapeRef {
	return rawShapeRef(c.packedEntry.state)
}

func (c *rawShapeChild) retargetNodePreservingShapeRef(node *Node) {
	if c == nil || node == nil || stackEntryNode(c.packedEntry) == nil {
		return
	}
	ref := c.packedEntry.state
	setStackEntryNode(&c.packedEntry, node)
	c.packedEntry.state = ref
}

type rawShapeSlab struct {
	data []rawShape
	used int
}

type rawShapeChildSlab struct {
	data []rawShapeChild
	used int
}

type rawShapeChildRange uint64

// reclaimRawShapeStorage releases parser-only reduction-shape sidecars after
// the returned result has been selected and fully materialized. Raw shapes are
// not part of the public tree: queries, cursors, edits, and incremental reuse
// read the materialized node structure instead.
//
// Clear every arena-owned payload's ref before resetting and trimming the
// slabs. Returned trees can lazily materialize compact leaves and pending
// parents after parse finalization, so clearing only the currently materialized
// Node values would let a stale arena-relative ref escape through that later
// materialization. Clearing all payload forms also prevents a reused subtree
// from carrying a ref into a different parse arena, where the same numeric ref
// could name an unrelated shape.
func (a *nodeArena) reclaimRawShapeStorage() {
	if a == nil {
		return
	}
	clearNodeRawShapeRefs := func(nodes []Node, used int) {
		if used > len(nodes) {
			used = len(nodes)
		}
		for i := 0; i < used; i++ {
			nodes[i].rawShape = 0
		}
	}
	clearNoTreeRawShapeRefs := func(nodes []noTreeNode, used int) {
		if used > len(nodes) {
			used = len(nodes)
		}
		for i := 0; i < used; i++ {
			nodes[i].rawShape = 0
		}
	}

	primaryUsed := a.used
	if primaryUsed > len(a.nodes) {
		primaryUsed = len(a.nodes)
	}
	clearNodeRawShapeRefs(a.nodes, primaryUsed)
	for i := range a.nodeSlabs {
		clearNodeRawShapeRefs(a.nodeSlabs[i].data, a.nodeSlabs[i].used)
	}
	for i := range a.noTreeNodeSlabs {
		clearNoTreeRawShapeRefs(a.noTreeNodeSlabs[i].data, a.noTreeNodeSlabs[i].used)
	}
	for i := range a.compactFullLeafSlabs {
		slab := &a.compactFullLeafSlabs[i]
		used := slab.used
		if used > len(slab.data) {
			used = len(slab.data)
		}
		for j := 0; j < used; j++ {
			slab.data[j].rawShape = 0
		}
	}
	for i := range a.pendingParentSlabs {
		slab := &a.pendingParentSlabs[i]
		used := slab.used
		if used > len(slab.data) {
			used = len(slab.data)
		}
		for j := 0; j < used; j++ {
			slab.data[j].rawShape = 0
		}
	}
	for i := range a.compactCheckpointLeafSlabs {
		slab := &a.compactCheckpointLeafSlabs[i]
		used := slab.used
		if used > len(slab.data) {
			used = len(slab.data)
		}
		for j := 0; j < used; j++ {
			slab.data[j].rawShape = 0
		}
	}

	// Reuse the arena's existing bounded reset policy: it drops pathological
	// overflow while retaining a small warm prefix for the next parse. Nil'ing
	// every slab here would force the 2 MiB full-parse base slabs to be allocated
	// again on each pooled parse, turning reclamation into an allocation
	// regression on ordinary files.
	a.resetRawShapeSlabs()
	a.resetRawShapeChildSlabs()
	a.resetRawShapeHashCache()
	a.recomputeAllocatedBytes()
}

func rawShapeBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(rawShape{}))
}

func rawShapeChildBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(rawShapeChild{}))
}

func defaultRawShapeSlabCap(class arenaClass) int {
	slabBytes := incrementalArenaSlab
	if class == arenaClassFull {
		slabBytes = fullParseArenaSlab
	}
	size := int(unsafe.Sizeof(rawShape{}))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func defaultRawShapeChildSlabCap(class arenaClass) int {
	slabBytes := incrementalArenaSlab
	if class == arenaClassFull {
		slabBytes = fullParseArenaSlab
	}
	size := int(unsafe.Sizeof(rawShapeChild{}))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func makeRawShapeChildRange(slab, start, count int) rawShapeChildRange {
	return rawShapeChildRange((uint64(slab+1) << 48) | (uint64(start) << 16) | uint64(count))
}

func (r rawShapeChildRange) slabIndex() int {
	return int((uint64(r)>>48)&0xffff) - 1
}

func (r rawShapeChildRange) start() int {
	return int((uint64(r) >> 16) & 0xffffffff)
}

func (r rawShapeChildRange) count() int {
	return int(uint16(uint64(r)))
}

func (s *rawShape) childCount() int {
	if s == nil {
		return 0
	}
	return s.childRange.count()
}

func (a *nodeArena) rawShapeForRef(ref rawShapeRef) (*rawShape, bool) {
	if a == nil || !rawShapeRefIsArenaBacked(ref) {
		return nil, false
	}
	slabIdx := int(uint32(ref)>>rawShapeRefIndexBits) - 1
	entryIdx := int(uint32(ref) & ((uint32(1) << rawShapeRefIndexBits) - 1))
	if slabIdx < 0 || slabIdx >= len(a.rawShapeSlabs) {
		return nil, false
	}
	slab := &a.rawShapeSlabs[slabIdx]
	if entryIdx < 0 || entryIdx >= slab.used || entryIdx >= len(slab.data) {
		return nil, false
	}
	return &slab.data[entryIdx], true
}

func rawShapeHashCacheIndex(ref rawShapeRef) int {
	// Use the high product bits so slab identity contributes to the index.
	return int((uint32(ref) * 2654435761) >> (32 - rawShapeHashCacheBits))
}

func (a *nodeArena) ensureRawShapeHashCache() {
	if a == nil || len(a.rawShapeHashCache) != 0 {
		return
	}
	a.rawShapeHashCache = make([]rawShapeHashCacheEntry, rawShapeHashCacheSize)
	a.allocatedBytes += rawShapeHashCacheBytesForCap(cap(a.rawShapeHashCache))
}

func (a *nodeArena) storeRawShapeHash(ref rawShapeRef, hash uint64) {
	if a == nil || ref == 0 {
		return
	}
	a.ensureRawShapeHashCache()
	a.rawShapeHashCache[rawShapeHashCacheIndex(ref)] = rawShapeHashCacheEntry{ref: ref, hash: hash}
}

func (a *nodeArena) rawShapeHash(ref rawShapeRef) (uint64, bool) {
	if a == nil || ref == 0 {
		return 0, false
	}
	if len(a.rawShapeHashCache) != 0 {
		cached := a.rawShapeHashCache[rawShapeHashCacheIndex(ref)]
		if cached.ref == ref {
			return cached.hash, true
		}
	}
	shape, ok := a.rawShapeForRef(ref)
	if !ok {
		return 0, false
	}
	children := a.rawShapeChildren(shape)
	hash := rawShapeComputeContentHash(a, ref, shape.symbol, shape.productionID, uint16(shape.childCount()), children)
	a.storeRawShapeHash(ref, hash)
	return hash, true
}

func (a *nodeArena) rawShapeChildren(shape *rawShape) []rawShapeChild {
	if a == nil || shape == nil || shape.childCount() == 0 || shape.childRange == 0 {
		return nil
	}
	slabIdx := shape.childRange.slabIndex()
	start := shape.childRange.start()
	count := shape.childCount()
	if slabIdx < 0 || slabIdx >= len(a.rawShapeChildSlabs) {
		return nil
	}
	slab := &a.rawShapeChildSlabs[slabIdx]
	if start < 0 || count < 0 || start+count > slab.used || start+count > len(slab.data) {
		return nil
	}
	return slab.data[start : start+count]
}

func (a *nodeArena) rawShapeChildrenForNode(node *Node) []rawShapeChild {
	if a == nil || node == nil || node.rawShape == 0 {
		return nil
	}
	shape, ok := a.rawShapeForRef(node.rawShape)
	if !ok {
		return nil
	}
	return a.rawShapeChildren(shape)
}

// captureRawShape captures the reduction-shape sidecar documented on rawShape
// (raw_shape.go). gssScratch gates a single-stack happy-path optimization:
// every reader of the resulting ref is nil-guarded with an explicit fallback
// (memory-safe unconditionally — see rawShapeForStackEntry and callers), so
// skipping the capture entirely while gssScratch.mayElideRawShape() is true
// is always safe.
//
// Behavior preservation is a separate, more careful argument. everForked
// (see gssScratch.everForked) latches permanently once a parse has forked or
// entered C-recovery — but that latch is NOT retroactive: it only stops
// FUTURE captures from being elided. A node built earlier, while elision was
// still active, stays shapeless forever; everForked cannot and does not go
// back and backfill it.
//
// What actually makes that safe is structural, not the latch: any node built
// while mayElideRawShape() was true predates this parse's first fork/
// recovery event, and every subsequent stack (GLR fork clones, C-recovery
// version-spawns) shares that node by pointer — clone()/cloneWithScratch()
// and recovery's *stacks mutation never deep-copy prior GSS structure. So a
// shapeless node is only ever compared against ITSELF (compareRawStackWalkEntriesRec
// recursing to the identical object on both sides, trivially symmetric),
// never against a different object at the same tree position. The
// comparison's one-sided-shape fallback (parser_reduce.go
// compareRawStackWalkEntriesRec, the aHasShape != bHasShape branch, falling back
// to materialized childCount/symbol) is NOT proven sign-preserving in
// isolation — TestRawShapeElisionAbstractComparisonFallbackCanFlipSign
// constructs an artificial asymmetric pair where it does flip sign — but a
// real parse under this gate can never feed it that asymmetric pair, for the
// pointer-sharing reason above. That structural argument, not "the fallback
// happens to agree", is the actual safety basis, and it is what
// TestRawShapeElisionDifferentialPrefixThenForkSharedRootSymbol and
// TestRawShapeElisionDifferentialRecoveryFromSingleStackPrefix (both
// raw_shape_elision_test.go / raw_shape_elision_recovery_test.go) exist to
// exercise empirically: same-root-symbol fork alternatives and a genuine
// C-recovery episode, both with a shared single-stack prefix that has a
// pre/post-flatten childCount mismatch, both pass tree-identical gate-on vs
// gate-off.
//
// Pass nil for gssScratch from call sites that are already proven fork-only/
// reachable only outside the pure single-stack path (e.g.
// reduceForkTemporaryParent) or that are out of scope for this optimization
// (e.g. the noTreeBenchmarkOnly lane, the forest engine, C-recovery's own
// shape helpers): nil never elides, matching the pre-optimization behavior.
// See spore.2026-07-12.hazel.rawshape-elision-rca.
func (p *Parser) captureRawShape(gssScratch *gssScratch, arena *nodeArena, symbol Symbol, productionID uint16, entries []stackEntry, start, end int) rawShapeRef {
	if arena == nil || start < 0 || end < start || end > len(entries) {
		return 0
	}
	count := 0
	for i := start; i < end; i++ {
		if stackEntryHasNode(entries[i]) {
			count++
		}
	}
	if count == 0 && p.compactPackedGSSVersionOrderEnabled() {
		return rawShapeZeroChildRef
	}
	if gssScratch.mayElideRawShape() || count > rawShapeMaxExactChildCount {
		return 0
	}
	if count == 0 {
		return 0
	}
	ref, shape := arena.allocRawShape()
	if shape == nil {
		return 0
	}
	shape.symbol = symbol
	shape.productionID = productionID
	childRange := arena.allocRawShapeChildren(count)
	children := arena.rawShapeChildren(&rawShape{childRange: childRange})
	out := 0
	for i := start; i < end && out < count; i++ {
		entry := entries[i]
		if !stackEntryHasNode(entry) {
			continue
		}
		children[out] = newRawShapeChild(entry)
		out++
	}
	shape.childRange = childRange
	// Cache the same 64-bit digest used by the previous inline field. The
	// bounded cache may evict it later, so rawShapeHash can recompute it from
	// the lossless sidecar without changing collision behavior.
	arena.storeRawShapeHash(ref, rawShapeComputeContentHash(arena, ref, symbol, productionID, uint16(count), children[:out]))
	return ref
}

// rawShapeComputeContentHash builds the bottom-up structural fingerprint
// documented on the raw-shape hash cache. It folds in the same fields the exact
// raw-shape comparators inspect (symbol, productionID, childCount, and per
// child: whether it has a node, its symbol, its span, and — recursively —
// its own already-computed hash when it has a captured shape,
// otherwise its own child count as a coarse stand-in for a leaf's shape).
// Reusing the package's existing 64-bit FNV-1a combiner (gssHashSeed/
// gssHashPrime/gssNilNodeSentinel, glr_gss.go) keeps this consistent with the
// GSS merge-key hashing that already accepts the same negligible-collision
// tradeoff for equivalence decisions.
func rawShapeComputeContentHash(arena *nodeArena, parentRef rawShapeRef, symbol Symbol, productionID uint16, childCount uint16, children []rawShapeChild) uint64 {
	h := gssHashSeed
	h ^= uint64(symbol)
	h *= gssHashPrime
	h ^= uint64(productionID)
	h *= gssHashPrime
	h ^= uint64(childCount)
	h *= gssHashPrime
	for i := range children {
		entry := children[i].entry()
		if !stackEntryHasNode(entry) {
			h ^= gssNilNodeSentinel
			h *= gssHashPrime
			continue
		}
		h ^= uint64(stackEntryNodeSymbol(entry))
		h *= gssHashPrime
		h ^= (uint64(stackEntryNodeStartByte(entry)) << 32) | uint64(stackEntryNodeEndByte(entry))
		h *= gssHashPrime
		if ref := children[i].shapeRef(); ref != 0 && ref < parentRef && arena != nil {
			if childHash, ok := arena.rawShapeHash(ref); ok {
				h ^= childHash
				h *= gssHashPrime
				continue
			}
		}
		h ^= uint64(stackEntryNodeChildCount(entry))
		h *= gssHashPrime
	}
	return h
}

func stackEntryRawShapeRef(entry stackEntry) rawShapeRef {
	if n := stackEntryNode(entry); n != nil {
		return n.rawShape
	}
	if n := stackEntryNoTreeNode(entry); n != nil {
		return n.rawShape
	}
	if n := stackEntryCompactFullLeaf(entry); n != nil {
		return n.rawShape
	}
	if n := stackEntryPendingParent(entry); n != nil {
		return n.rawShape
	}
	return 0
}

func setStackEntryRawShapeRef(entry *stackEntry, ref rawShapeRef) {
	if entry == nil {
		return
	}
	if n := stackEntryNode(*entry); n != nil {
		n.rawShape = ref
		nodeBumpEquivVersionMetadata(n)
		return
	}
	if n := stackEntryNoTreeNode(*entry); n != nil {
		n.rawShape = ref
		return
	}
	if n := stackEntryCompactFullLeaf(*entry); n != nil {
		n.rawShape = ref
		return
	}
	if n := stackEntryPendingParent(*entry); n != nil {
		n.rawShape = ref
	}
}

func compareAcceptedStackRawShapePreference(p *Parser, arena *nodeArena, a, b *glrStack) int {
	if !a.accepted || !b.accepted || arena == nil {
		return 0
	}
	aCount := stackMaterializingResultEntryCount(a)
	if aCount == 0 || aCount != stackMaterializingResultEntryCount(b) {
		return 0
	}
	const maxBufferedRawShapeEntries = 8
	if aCount > maxBufferedRawShapeEntries {
		return 0
	}
	var aBuf, bBuf [maxBufferedRawShapeEntries]stackEntry
	aEntries, aOK := stackMaterializingResultEntries(a, aBuf[:0], aCount)
	bEntries, bOK := stackMaterializingResultEntries(b, bBuf[:0], aCount)
	if !aOK || !bOK {
		return 0
	}
	if !rawStackEntriesContainShape(arena, aEntries) && !rawStackEntriesContainShape(arena, bEntries) {
		return 0
	}
	for i := 0; i < aCount; i++ {
		cmp := p.compareRawStackEntries(arena, aEntries[i], bEntries[i])
		if cmp != 0 {
			if cmp < 0 {
				return 1
			}
			return -1
		}
	}
	return 0
}

func rawStackEntriesContainShape(arena *nodeArena, entries []stackEntry) bool {
	for i := range entries {
		if rawStackEntryContainsShape(arena, entries[i], 0) {
			return true
		}
	}
	return false
}

func rawStackEntryContainsShape(arena *nodeArena, entry stackEntry, depth int) bool {
	return rawStackWalkEntryContainsShape(arena, rawStackWalkEntry{entry: entry}, depth)
}

func rawStackWalkEntryContainsShape(arena *nodeArena, item rawStackWalkEntry, depth int) bool {
	if depth > maxTreeWalkDepth {
		return false
	}
	childCount := stackEntryNodeChildCount(item.entry)
	if shape, _, ok := rawShapeForStackWalkEntry(arena, item); ok {
		if shape.childCount() > 0 {
			return true
		}
		childCount = shape.childCount()
	} else if rawStackWalkEntryRef(item) == rawShapeZeroChildRef {
		childCount = 0
	}
	for i := 0; i < childCount; i++ {
		child, ok := rawStackWalkChildAt(arena, item, i)
		if !ok {
			continue
		}
		if rawStackWalkEntryContainsShape(arena, child, depth+1) {
			return true
		}
	}
	return false
}
