package gotreesitter

import "unsafe"

type pendingParent struct {
	noTreeNode
	startPoint Point
	endPoint   Point
	childRange pendingChildRange
}

type pendingParentSlab struct {
	data []pendingParent
	used int
}

type pendingChildEntrySlab struct {
	data []pendingChildEntry
	used int
}

// pendingChildEntry stores arena-owned payload pointers separately from small
// stack-entry or field metadata. The payload parseState is the source of truth
// when the stack entry is reconstructed.
type pendingChildEntry struct {
	node unsafe.Pointer
	meta uintptr
}

type pendingChildRange uint64

const (
	pendingChildRangeCountBits                      = 20
	pendingChildRangeOffsetBits                     = 24
	pendingChildRangeCountMask                      = (uint64(1) << pendingChildRangeCountBits) - 1
	pendingChildRangeOffsetMask                     = (uint64(1) << pendingChildRangeOffsetBits) - 1
	pendingChildEntryKindBits                       = 2
	pendingChildEntryFieldIDBits                    = 16
	pendingChildEntryFieldSourceBits                = 2
	pendingChildEntryFieldIDShift                   = pendingChildEntryKindBits
	pendingChildEntryFieldSourceShift               = pendingChildEntryFieldIDShift + pendingChildEntryFieldIDBits
	pendingChildEntryKindMask             uintptr   = (1 << pendingChildEntryKindBits) - 1
	pendingChildEntryFieldIDValueMask     uintptr   = (1 << pendingChildEntryFieldIDBits) - 1
	pendingChildEntryFieldSourceValueMask uintptr   = (1 << pendingChildEntryFieldSourceBits) - 1
	pendingChildEntryFieldIDMask                    = pendingChildEntryFieldIDValueMask << pendingChildEntryFieldIDShift
	pendingChildEntryFieldSourceMask                = pendingChildEntryFieldSourceValueMask << pendingChildEntryFieldSourceShift
	pendingChildEntryFieldMetadataMask              = pendingChildEntryFieldIDMask | pendingChildEntryFieldSourceMask
	pendingParentFlagFieldEntries         nodeFlags = 1 << 5
	pendingParentFlagDirectFieldEntry     nodeFlags = 1 << 6

	// publicPendingParentNodeFlags is the mask applied when a pendingParent's
	// flags are copied onto the Node produced at materialization (see
	// materializeStackEntryPendingParentEntryWithParser, below): it strips
	// the pending-only scratch bits (pendingParentFlagFieldEntries /
	// pendingParentFlagDirectFieldEntry, which alias
	// nodeFlagFieldIDCacheComputed / nodeFlagFieldIDCacheHasFieldIDs -- see
	// tree.go) that only have meaning before materialization.
	// nodeFlagFragileLeft/Right MUST stay in this mask: they are set on the
	// pendingParent by markReduceFragility (parser_reduce.go) at the same
	// "parent.parseState = targetState" reduce set-point as every other
	// reduce lane, and silently dropping them here would let a fragile
	// subtree survive materialization looking safe to incremental non-leaf
	// reuse (reuseNonLeafTargetStateOnStack, incremental.go).
	publicPendingParentNodeFlags nodeFlags = nodeFlagNamed | nodeFlagExtra | nodeFlagMissing | nodeFlagHasError | nodeFlagDirty | nodeFlagFragileLeft | nodeFlagFragileRight
)

type pendingParentMaterializeReason = materializeReason

const (
	pendingParentMaterializeForParentReduce      pendingParentMaterializeReason = materializeForParentReduce
	pendingParentMaterializeForFinalTree         pendingParentMaterializeReason = materializeForFinalTree
	pendingParentMaterializeForNormalization     pendingParentMaterializeReason = materializeForNormalization
	pendingParentMaterializeForRecovery          pendingParentMaterializeReason = materializeForRecovery
	pendingParentMaterializeForQuery             pendingParentMaterializeReason = materializeForQuery
	pendingParentMaterializeForCursor            pendingParentMaterializeReason = materializeForCursor
	pendingParentMaterializeForParentAPI         pendingParentMaterializeReason = materializeForParentAPI
	pendingParentMaterializeForEdit              pendingParentMaterializeReason = materializeForEdit
	pendingParentMaterializeForCheckpointRebuild pendingParentMaterializeReason = materializeForCheckpointRebuild
)

func pendingParentBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(pendingParent{}))
}

func pendingChildEntryBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(pendingChildEntry{}))
}

func defaultPendingParentSlabCap(class arenaClass) int {
	slabBytes := incrementalArenaSlab
	if class == arenaClassFull {
		slabBytes = fullParseArenaSlab
	}
	size := int(unsafe.Sizeof(pendingParent{}))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func defaultPendingChildEntrySlabCap(class arenaClass) int {
	slabBytes := incrementalArenaSlab
	if class == arenaClassFull {
		slabBytes = fullParseArenaSlab / 2
	}
	size := int(unsafe.Sizeof(pendingChildEntry{}))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func newPendingParentInArena(arena *nodeArena, sym Symbol, named bool, productionID uint16, children []stackEntry, startByte, endByte uint32, startPoint, endPoint Point, hasError bool) *pendingParent {
	p := newPendingParentShellInArena(arena, sym, named, productionID, len(children), startByte, endByte, startPoint, endPoint, hasError)
	for i, child := range children {
		p.setChildEntry(arena, i, child)
	}
	return p
}

func newPendingParentShellInArena(arena *nodeArena, sym Symbol, named bool, productionID uint16, childCount int, startByte, endByte uint32, startPoint, endPoint Point, hasError bool) *pendingParent {
	var p *pendingParent
	var childRange pendingChildRange
	if arena == nil {
		if childCount > 0 {
			panic("pending parent child refs require arena-backed payloads")
		}
		p = &pendingParent{}
	} else {
		p = arena.allocPendingParent()
		childRange, _ = arena.allocPendingChildEntries(childCount)
		arena.pendingParentCreated++
		workCountRecordPendingParentConstruction()
	}
	p.setChildEntries(childRange)
	p.symbol = sym
	p.dependsOnColumn = false
	p.startByte = startByte
	p.endByte = endByte
	p.parseState = 0
	p.preGotoState = 0
	p.productionID = productionID
	p.rawShape = 0
	p.dynamicPrecedence = 0
	p.flags = noTreeNodeInitialFlags(named)
	p.setHasError(hasError)
	p.startPoint = startPoint
	p.endPoint = endPoint
	return p
}

func (p *pendingParent) setChildEntries(childRange pendingChildRange) {
	if p == nil {
		return
	}
	p.childRange = childRange
}

func (p *pendingParent) childRefs(arena *nodeArena) []pendingChildEntry {
	if p == nil || arena == nil || p.childRange.count() == 0 {
		return nil
	}
	return p.childRange.refs(arena)
}

func (p *pendingParent) childEntryCount() int {
	if p == nil {
		return 0
	}
	return p.childRange.count()
}

func (p *pendingParent) childEntry(arena *nodeArena, i int) stackEntry {
	if p == nil || i < 0 || i >= p.childEntryCount() {
		return stackEntry{}
	}
	refs := p.childRefs(arena)
	if i >= len(refs) {
		return stackEntry{}
	}
	return refs[i].stackEntry()
}

func (p *pendingParent) setChildEntry(arena *nodeArena, i int, entry stackEntry) {
	if p == nil || i < 0 || i >= p.childEntryCount() {
		return
	}
	refs := p.childRefs(arena)
	if i >= len(refs) {
		return
	}
	next := newPendingChildEntry(entry)
	next.meta |= refs[i].meta & pendingChildEntryFieldMetadataMask
	refs[i] = next
}

func (p *pendingParent) hasFieldEntries() bool {
	return p != nil && p.hasFlag(pendingParentFlagFieldEntries)
}

func (p *pendingParent) setHasFieldEntries(v bool) {
	p.setFlag(pendingParentFlagFieldEntries, v)
}

// hasDirectFieldEntries preserves the old hidden-flattening classification:
// these direct fields were previously safe to reconstruct from visible
// children. Their values are now packed and are never recomputed when the
// pending parent materializes.
func (p *pendingParent) hasDirectFieldEntries() bool {
	return p != nil && p.hasFlag(pendingParentFlagDirectFieldEntry)
}

func (p *pendingParent) setHasDirectFieldEntries(v bool) {
	p.setFlag(pendingParentFlagDirectFieldEntry, v)
}

func (p *pendingParent) setChildFieldEntry(arena *nodeArena, i int, fid FieldID, source uint8) {
	if p == nil || i < 0 || i >= p.childEntryCount() {
		return
	}
	if uintptr(source)&^pendingChildEntryFieldSourceValueMask != 0 {
		panic("pending child field source exceeds tag mask")
	}
	refs := p.childRefs(arena)
	if i >= len(refs) {
		return
	}
	refs[i].meta = (refs[i].meta &^ pendingChildEntryFieldMetadataMask) |
		uintptr(fid)<<pendingChildEntryFieldIDShift |
		uintptr(source)<<pendingChildEntryFieldSourceShift
	if (fid != 0 || source != fieldSourceNone) && !p.hasFieldEntries() && !p.hasDirectFieldEntries() {
		p.setHasFieldEntries(true)
	}
}

func (p *pendingParent) childFieldEntry(arena *nodeArena, i int) (FieldID, uint8) {
	if p == nil || i < 0 || i >= p.childEntryCount() {
		return 0, fieldSourceNone
	}
	refs := p.childRefs(arena)
	if i >= len(refs) {
		return 0, fieldSourceNone
	}
	return refs[i].fieldID(), refs[i].fieldSource()
}

func newPendingChildRange(slabIndex, offset, count int) pendingChildRange {
	if count <= 0 {
		return 0
	}
	if slabIndex < 0 || offset < 0 || count < 0 ||
		uint64(slabIndex) >= 1<<(64-pendingChildRangeOffsetBits-pendingChildRangeCountBits) ||
		uint64(offset) > pendingChildRangeOffsetMask ||
		uint64(count) > pendingChildRangeCountMask {
		panic("pending child range exceeds packed bounds")
	}
	return pendingChildRange(uint64(slabIndex)<<(pendingChildRangeOffsetBits+pendingChildRangeCountBits) |
		uint64(offset)<<pendingChildRangeCountBits |
		uint64(count))
}

func (r pendingChildRange) count() int {
	return int(uint64(r) & pendingChildRangeCountMask)
}

func (r pendingChildRange) slabIndex() int {
	return int(uint64(r) >> (pendingChildRangeOffsetBits + pendingChildRangeCountBits))
}

func (r pendingChildRange) offset() int {
	return int((uint64(r) >> pendingChildRangeCountBits) & pendingChildRangeOffsetMask)
}

func (r pendingChildRange) refs(arena *nodeArena) []pendingChildEntry {
	count := r.count()
	if arena == nil || count == 0 {
		return nil
	}
	slabIndex := r.slabIndex()
	if slabIndex < 0 || slabIndex >= len(arena.pendingChildEntrySlabs) {
		return nil
	}
	offset := r.offset()
	slab := arena.pendingChildEntrySlabs[slabIndex].data
	if offset < 0 || offset+count > len(slab) {
		return nil
	}
	return slab[offset : offset+count]
}

func newPendingChildEntry(entry stackEntry) pendingChildEntry {
	if entry.node == nil {
		return pendingChildEntry{}
	}
	kind := uintptr(entry.kind)
	if kind&^pendingChildEntryKindMask != 0 {
		panic("pending child entry kind exceeds tag mask")
	}
	return pendingChildEntry{node: entry.node, meta: kind}
}

func (e pendingChildEntry) fieldID() FieldID {
	return FieldID((e.meta & pendingChildEntryFieldIDMask) >> pendingChildEntryFieldIDShift)
}

func (e pendingChildEntry) fieldSource() uint8 {
	return uint8((e.meta & pendingChildEntryFieldSourceMask) >> pendingChildEntryFieldSourceShift)
}

func (entry pendingChildEntry) stackEntry() stackEntry {
	if entry.node == nil {
		return stackEntry{}
	}
	stack := stackEntry{
		node: entry.node,
		kind: uint32(entry.meta & pendingChildEntryKindMask),
	}
	stack.state = stackEntryNodeParseState(stack)
	return stack
}

func materializeStackEntryPendingParent(arena *nodeArena, entry *stackEntry, reason pendingParentMaterializeReason) *Node {
	return materializeStackEntryPendingParentWithParser(nil, arena, entry, reason)
}

func materializeStackEntryPendingParentWithParser(p *Parser, arena *nodeArena, entry *stackEntry, reason pendingParentMaterializeReason) *Node {
	if entry == nil {
		return nil
	}
	node, updated := materializeStackEntryPendingParentEntryWithParser(p, arena, *entry, reason)
	*entry = updated
	return node
}

func materializeStackEntryPendingParentEntryWithParser(p *Parser, arena *nodeArena, entry stackEntry, reason pendingParentMaterializeReason) (*Node, stackEntry) {
	parent := stackEntryPendingParent(entry)
	if parent == nil {
		return materializeStackEntryCompactFullLeafEntry(arena, entry, compactFullLeafMaterializeReason(reason))
	}
	childCount := parent.childEntryCount()
	if arena != nil &&
		arena.finalChildRefs &&
		childCount > 0 &&
		!parent.hasFieldEntries() &&
		!parent.hasDirectFieldEntries() &&
		(reason == materializeForFinalTree || reason == materializeForParentAPI || reason == materializeForQuery || reason == materializeForCursor) {
		node := newParentNodeInArenaWithFinalChildRefs(arena, parent.symbol, parent.isNamed(), parent.childRange, parent.productionID, parent.hasError())
		node.flags = parent.flags & publicPendingParentNodeFlags
		node.startByte = parent.startByte
		node.endByte = parent.endByte
		node.startPoint = parent.startPoint
		node.endPoint = parent.endPoint
		node.parseState = parent.parseState
		node.preGotoState = parent.preGotoState
		node.rawShape = parent.rawShape
		node.dynamicPrecedence = parent.dynamicPrecedence
		// Materialization can run after the column-dependency fold closed
		// this arena, and the fold wrote its answer onto the pending
		// parent. Carry it so the node the fold never saw still answers.
		node.setDependsOnColumn(arena, parent.dependsOnColumn)
		widenNodeSpanToPendingChildren(node, parent, arena)
		setStackEntryNode(&entry, node)
		arena.recordPendingParentMaterialized(reason)
		return node, entry
	}
	children := arena.allocNodeSliceNoClear(childCount)
	var fieldIDs []FieldID
	var fieldSources []uint8
	hasFieldEntries := parent.hasFieldEntries() || parent.hasDirectFieldEntries()
	if hasFieldEntries {
		fieldIDs = arena.allocFieldIDSlice(childCount)
		fieldSources = arena.allocFieldSourceSlice(childCount)
	}
	for i := 0; i < childCount; i++ {
		child := parent.childEntry(arena, i)
		var updatedChild stackEntry
		children[i], updatedChild = materializeStackEntryPayloadEntryWithParser(p, arena, child, compactFullLeafMaterializeReason(reason), reason)
		child = updatedChild
		parent.setChildEntry(arena, i, child)
		if hasFieldEntries {
			fid, source := parent.childFieldEntry(arena, i)
			fieldIDs[i] = fid
			fieldSources[i] = source
		}
	}
	if fieldIDs != nil && p != nil {
		p.suppressReducedChildFields(children, fieldIDs, fieldSources)
	}
	node := newParentNodeInArenaNoLinksWithFieldSources(arena, parent.symbol, parent.isNamed(), children, fieldIDs, fieldSources, parent.productionID, parent.hasError())
	node.flags = parent.flags & publicPendingParentNodeFlags
	node.startByte = parent.startByte
	node.endByte = parent.endByte
	node.startPoint = parent.startPoint
	node.endPoint = parent.endPoint
	node.parseState = parent.parseState
	node.preGotoState = parent.preGotoState
	node.rawShape = parent.rawShape
	node.dynamicPrecedence = parent.dynamicPrecedence
	// See the final-child-refs lane above.
	node.setDependsOnColumn(arena, parent.dependsOnColumn)
	widenNodeSpanToMaterializedChildren(node, children)
	rebuildExternalScannerCheckpointForMaterializedParent(node, reason)
	setStackEntryNode(&entry, node)
	arena.recordPendingParentMaterialized(reason)
	return node, entry
}

func widenNodeSpanToPendingChildren(node *Node, parent *pendingParent, arena *nodeArena) {
	if node == nil || parent == nil {
		return
	}
	for i := 0; i < parent.childEntryCount(); i++ {
		child := parent.childEntry(arena, i)
		widenNodeSpanToStackEntry(node, child)
	}
}

func widenNodeSpanToMaterializedChildren(node *Node, children []*Node) {
	if node == nil {
		return
	}
	for _, child := range children {
		if child == nil {
			continue
		}
		widenNodeSpanToChildSpan(node, child.startByte, child.endByte, child.startPoint, child.endPoint)
	}
}

func widenNodeSpanToRetainedChildren(node *Node) {
	if node == nil {
		return
	}
	childCount := nodeChildCountNoMaterialize(node)
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(node, i)
		if !ok {
			continue
		}
		widenNodeSpanToStackEntry(node, entry)
	}
}

func widenNodeSpanToStackEntry(node *Node, entry stackEntry) {
	if node == nil || !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) {
		return
	}
	widenNodeSpanToChildSpan(node, stackEntryNodeStartByte(entry), stackEntryNodeEndByte(entry), stackEntryNodeStartPoint(entry), stackEntryNodeEndPoint(entry))
}

func widenNodeSpanToChildSpan(node *Node, startByte, endByte uint32, startPoint, endPoint Point) {
	if node == nil {
		return
	}
	if startByte < node.startByte || (node.startByte == node.endByte && node.startByte == 0 && endByte > startByte) {
		node.startByte = startByte
		node.startPoint = startPoint
	}
	if endByte > node.endByte {
		node.endByte = endByte
		node.endPoint = endPoint
	}
}

func materializeStackEntryPayload(arena *nodeArena, entry *stackEntry, leafReason compactFullLeafMaterializeReason, parentReason pendingParentMaterializeReason) *Node {
	return materializeStackEntryPayloadWithParser(nil, arena, entry, leafReason, parentReason)
}

func materializeStackEntryPayloadWithParser(p *Parser, arena *nodeArena, entry *stackEntry, leafReason compactFullLeafMaterializeReason, parentReason pendingParentMaterializeReason) *Node {
	if entry == nil {
		return nil
	}
	node, updated := materializeStackEntryPayloadEntryWithParser(p, arena, *entry, leafReason, parentReason)
	*entry = updated
	return node
}

func materializeStackEntryPayloadEntryWithParser(p *Parser, arena *nodeArena, entry stackEntry, leafReason compactFullLeafMaterializeReason, parentReason pendingParentMaterializeReason) (*Node, stackEntry) {
	restoreShape := false
	prevPayloadShape := pendingParentFieldRejectPayloadUnknown
	if p != nil && arena != nil && arena.breakdownEnabled && arena.pendingParentActiveRejectReason == pendingParentRejectFields {
		restoreShape = true
		prevPayloadShape = arena.pendingParentActiveFieldPayloadShape
		arena.pendingParentActiveFieldPayloadShape = p.pendingParentFieldRejectPayloadShape(entry, arena)
	}
	if restoreShape {
		defer func() {
			arena.pendingParentActiveFieldPayloadShape = prevPayloadShape
		}()
	}
	if arena != nil && arena.pendingParentActiveRejectReason != pendingParentRejectUnknown {
		arena.recordParentRejectPayloadMaterialized(entry, arena.pendingParentActiveRejectReason)
	}
	if stackEntryPendingParent(entry) != nil {
		return materializeStackEntryPendingParentEntryWithParser(p, arena, entry, parentReason)
	}
	return materializeStackEntryCompactFullLeafEntry(arena, entry, leafReason)
}
