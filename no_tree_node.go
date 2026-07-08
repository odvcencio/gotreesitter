package gotreesitter

import "unsafe"

type noTreeNode struct {
	startByte         uint32
	endByte           uint32
	rawShape          rawShapeRef
	parseState        StateID
	preGotoState      StateID
	dynamicPrecedence int32
	symbol            Symbol
	productionID      uint16
	flags             nodeFlags
}

type noTreeNodeSlab struct {
	data []noTreeNode
	used int
}

type compactCheckpointLeaf struct {
	noTreeNode
	checkpoint externalScannerCheckpointRef
}

type compactCheckpointLeafSlab struct {
	data []compactCheckpointLeaf
	used int
}

type compactFullLeaf struct {
	noTreeNode
	startPoint    Point
	endPoint      Point
	checkpoint    externalScannerCheckpointRef
	hasCheckpoint bool
}

type compactFullLeafSlab struct {
	data []compactFullLeaf
	used int
}

type compactFullLeafMaterializeReason = materializeReason

const (
	compactFullLeafMaterializeForParentReduce  compactFullLeafMaterializeReason = materializeForParentReduce
	compactFullLeafMaterializeForFinalTree     compactFullLeafMaterializeReason = materializeForFinalTree
	compactFullLeafMaterializeForNormalization compactFullLeafMaterializeReason = materializeForNormalization
)

const (
	stackEntryKindNode uint32 = iota
	stackEntryKindNoTreeNode
	stackEntryKindCompactFullLeaf
	stackEntryKindPendingParent
)

func newStackEntryNode(state StateID, node *Node) stackEntry {
	return stackEntry{
		node:  unsafe.Pointer(node),
		state: state,
		kind:  stackEntryKindNode,
	}
}

func newStackEntryNoTreeNode(state StateID, node *noTreeNode) stackEntry {
	return stackEntry{
		node:  unsafe.Pointer(node),
		state: state,
		kind:  stackEntryKindNoTreeNode,
	}
}

func newStackEntryCompactCheckpointLeaf(state StateID, leaf *compactCheckpointLeaf) stackEntry {
	return stackEntry{
		node:  unsafe.Pointer(leaf),
		state: state,
		kind:  stackEntryKindNoTreeNode,
	}
}

func newStackEntryCompactFullLeaf(state StateID, leaf *compactFullLeaf) stackEntry {
	return stackEntry{
		node:  unsafe.Pointer(leaf),
		state: state,
		kind:  stackEntryKindCompactFullLeaf,
	}
}

func newStackEntryPendingParent(state StateID, parent *pendingParent) stackEntry {
	return stackEntry{
		node:  unsafe.Pointer(parent),
		state: state,
		kind:  stackEntryKindPendingParent,
	}
}

func stackEntryNode(e stackEntry) *Node {
	if e.kind != stackEntryKindNode || e.node == nil {
		return nil
	}
	return (*Node)(e.node)
}

func stackEntryNoTreeNode(e stackEntry) *noTreeNode {
	if e.kind != stackEntryKindNoTreeNode || e.node == nil {
		return nil
	}
	return (*noTreeNode)(e.node)
}

func stackEntryCompactFullLeaf(e stackEntry) *compactFullLeaf {
	if e.kind != stackEntryKindCompactFullLeaf || e.node == nil {
		return nil
	}
	return (*compactFullLeaf)(e.node)
}

func stackEntryPendingParent(e stackEntry) *pendingParent {
	if e.kind != stackEntryKindPendingParent || e.node == nil {
		return nil
	}
	return (*pendingParent)(e.node)
}

func setStackEntryNode(entry *stackEntry, node *Node) {
	if entry == nil {
		return
	}
	entry.node = unsafe.Pointer(node)
	entry.kind = stackEntryKindNode
	if node != nil {
		entry.state = node.parseState
	}
}

func (n *noTreeNode) hasFlag(flag nodeFlags) bool {
	return n != nil && n.flags&flag != 0
}

func (n *noTreeNode) setFlag(flag nodeFlags, enabled bool) {
	if n == nil {
		return
	}
	if enabled {
		n.flags |= flag
		return
	}
	n.flags &^= flag
}

func (n *noTreeNode) isNamed() bool                  { return n.hasFlag(nodeFlagNamed) }
func (n *noTreeNode) setNamed(v bool)                { n.setFlag(nodeFlagNamed, v) }
func (n *noTreeNode) isExtra() bool                  { return n.hasFlag(nodeFlagExtra) }
func (n *noTreeNode) setExtra(v bool)                { n.setFlag(nodeFlagExtra, v) }
func (n *noTreeNode) isMissing() bool                { return n.hasFlag(nodeFlagMissing) }
func (n *noTreeNode) setMissing(v bool)              { n.setFlag(nodeFlagMissing, v) }
func (n *noTreeNode) hasError() bool                 { return n.hasFlag(nodeFlagHasError) }
func (n *noTreeNode) setHasError(v bool)             { n.setFlag(nodeFlagHasError, v) }
func (n *noTreeNode) setExternalScannerToken(v bool) { n.setFlag(nodeFlagExternalScannerToken, v) }
func (n *noTreeNode) dirty() bool                    { return n.hasFlag(nodeFlagDirty) }
func (n *noTreeNode) setDirty(v bool)                { n.setFlag(nodeFlagDirty, v) }

func noTreeNodeBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(noTreeNode{}))
}

func compactCheckpointLeafBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(compactCheckpointLeaf{}))
}

func compactFullLeafBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(compactFullLeaf{}))
}

func defaultNoTreeNodeSlabCap(class arenaClass) int {
	slabBytes := incrementalArenaSlab
	if class == arenaClassFull {
		slabBytes = fullParseArenaSlab
	}
	size := int(unsafe.Sizeof(noTreeNode{}))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func defaultCompactCheckpointLeafSlabCap(class arenaClass) int {
	slabBytes := incrementalArenaSlab
	if class == arenaClassFull {
		slabBytes = fullParseArenaSlab
	}
	size := int(unsafe.Sizeof(compactCheckpointLeaf{}))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func defaultCompactFullLeafSlabCap(class arenaClass) int {
	slabBytes := incrementalArenaSlab
	if class == arenaClassFull {
		slabBytes = fullParseArenaSlab
	}
	size := int(unsafe.Sizeof(compactFullLeaf{}))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func stackEntryNodeSymbol(e stackEntry) Symbol {
	if n := stackEntryNode(e); n != nil {
		return n.symbol
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.symbol
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.symbol
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.symbol
	}
	return 0
}

func stackEntryNodeStartByte(e stackEntry) uint32 {
	if n := stackEntryNode(e); n != nil {
		return n.startByte
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.startByte
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.startByte
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.startByte
	}
	return 0
}

func stackEntryNodeEndByte(e stackEntry) uint32 {
	if n := stackEntryNode(e); n != nil {
		return n.endByte
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.endByte
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.endByte
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.endByte
	}
	return 0
}

func stackEntryNodeStartPoint(e stackEntry) Point {
	if n := stackEntryNode(e); n != nil {
		return n.startPoint
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.startPoint
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.startPoint
	}
	// Compact no-tree payloads are byte-span only. No-tree benchmark results
	// suppress the parse tree, so carrying row/column points here is wasted
	// write/read traffic on large diagnostic parses.
	return Point{}
}

func stackEntryNodeEndPoint(e stackEntry) Point {
	if n := stackEntryNode(e); n != nil {
		return n.endPoint
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.endPoint
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.endPoint
	}
	// See stackEntryNodeStartPoint.
	return Point{}
}

func stackEntryNodeParseState(e stackEntry) StateID {
	if n := stackEntryNode(e); n != nil {
		return n.parseState
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.parseState
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.parseState
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.parseState
	}
	return 0
}

func stackEntryNodePreGotoState(e stackEntry) StateID {
	if n := stackEntryNode(e); n != nil {
		return n.preGotoState
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.preGotoState
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.preGotoState
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.preGotoState
	}
	return 0
}

func stackEntryNodeProductionID(e stackEntry) uint16 {
	if n := stackEntryNode(e); n != nil {
		return n.productionID
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.productionID
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.productionID
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.productionID
	}
	return 0
}

func stackEntryHasNode(e stackEntry) bool {
	return e.node != nil
}

func retargetStackEntryPayload(e stackEntry, state StateID) (stackEntry, bool) {
	if n := stackEntryNode(e); n != nil {
		n.parseState = state
		nodeBumpEquivVersion(n)
		e.state = state
		return e, true
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		n.parseState = state
		e.state = state
		return e, true
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		n.parseState = state
		e.state = state
		return e, true
	}
	if n := stackEntryPendingParent(e); n != nil {
		n.parseState = state
		e.state = state
		return e, true
	}
	return e, false
}

func stackEntryNodeIsExtra(e stackEntry) bool {
	if n := stackEntryNode(e); n != nil {
		return n.isExtra()
	}
	n := stackEntryNoTreeNode(e)
	if n != nil {
		return n.isExtra()
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.isExtra()
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.isExtra()
	}
	return false
}

func stackEntryNodeIsNamed(e stackEntry) bool {
	if n := stackEntryNode(e); n != nil {
		return n.isNamed()
	}
	n := stackEntryNoTreeNode(e)
	if n != nil {
		return n.isNamed()
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.isNamed()
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.isNamed()
	}
	return false
}

func stackEntryNodeIsMissing(e stackEntry) bool {
	if n := stackEntryNode(e); n != nil {
		return n.isMissing()
	}
	n := stackEntryNoTreeNode(e)
	if n != nil {
		return n.isMissing()
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.isMissing()
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.isMissing()
	}
	return false
}

func stackEntryNodeHasError(e stackEntry) bool {
	if n := stackEntryNode(e); n != nil {
		return n.hasError()
	}
	n := stackEntryNoTreeNode(e)
	if n != nil {
		return n.hasError()
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.hasError()
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.hasError()
	}
	return false
}

func stackEntryNodeDirty(e stackEntry) bool {
	if n := stackEntryNode(e); n != nil {
		return n.dirty()
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.dirty()
	}
	if n := stackEntryCompactFullLeaf(e); n != nil {
		return n.dirty()
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.dirty()
	}
	return false
}

func stackEntryNodeChildCount(e stackEntry) int {
	if n := stackEntryNode(e); n != nil {
		return nodeChildCountNoMaterialize(n)
	}
	if n := stackEntryPendingParent(e); n != nil {
		return n.childEntryCount()
	}
	return 0
}

func stackEntryNodeFieldIDCount(e stackEntry) int {
	if n := stackEntryNode(e); n != nil {
		return len(n.fieldIDs)
	}
	return 0
}

func noTreeNodeInitialFlags(named bool) nodeFlags {
	if named {
		return nodeFlagNamed
	}
	return 0
}

func newNoTreeLeafNodeInArena(arena *nodeArena, sym Symbol, named bool, startByte, endByte uint32, startPoint, endPoint Point) *noTreeNode {
	var n *noTreeNode
	if arena == nil {
		n = &noTreeNode{}
	} else {
		n = arena.allocNoTreeNode()
		arena.noTreeLeafNodesConstructed++
	}
	n.symbol = sym
	n.startByte = startByte
	n.endByte = endByte
	n.parseState = 0
	n.preGotoState = 0
	n.productionID = 0
	n.rawShape = 0
	n.dynamicPrecedence = 0
	n.flags = noTreeNodeInitialFlags(named)
	return n
}

func newCompactCheckpointLeafInArena(arena *nodeArena, sym Symbol, named bool, startByte, endByte uint32, checkpoint externalScannerCheckpointRef) *compactCheckpointLeaf {
	var n *compactCheckpointLeaf
	if arena == nil {
		n = &compactCheckpointLeaf{}
	} else {
		n = arena.allocCompactCheckpointLeaf()
	}
	n.symbol = sym
	n.startByte = startByte
	n.endByte = endByte
	n.parseState = 0
	n.preGotoState = 0
	n.productionID = 0
	n.rawShape = 0
	n.dynamicPrecedence = 0
	n.flags = noTreeNodeInitialFlags(named)
	n.checkpoint = checkpoint
	return n
}

func newCompactFullLeafInArena(arena *nodeArena, sym Symbol, named bool, startByte, endByte uint32, startPoint, endPoint Point) *compactFullLeaf {
	var n *compactFullLeaf
	if arena == nil {
		n = &compactFullLeaf{}
	} else {
		n = arena.allocCompactFullLeaf()
		arena.compactFullLeafCreated++
	}
	n.symbol = sym
	n.startByte = startByte
	n.endByte = endByte
	n.parseState = 0
	n.preGotoState = 0
	n.productionID = 0
	n.rawShape = 0
	n.dynamicPrecedence = 0
	n.flags = noTreeNodeInitialFlags(named)
	n.startPoint = startPoint
	n.endPoint = endPoint
	n.checkpoint = externalScannerCheckpointRef{}
	n.hasCheckpoint = false
	return n
}

func materializeStackEntryCompactFullLeaf(arena *nodeArena, entry *stackEntry, reason compactFullLeafMaterializeReason) *Node {
	if entry == nil {
		return nil
	}
	node, updated := materializeStackEntryCompactFullLeafEntry(arena, *entry, reason)
	*entry = updated
	return node
}

func materializeStackEntryCompactFullLeafEntry(arena *nodeArena, entry stackEntry, reason compactFullLeafMaterializeReason) (*Node, stackEntry) {
	leaf := stackEntryCompactFullLeaf(entry)
	if leaf == nil {
		return stackEntryNode(entry), entry
	}
	node := newLeafNodeInArena(arena, leaf.symbol, leaf.isNamed(), leaf.startByte, leaf.endByte, leaf.startPoint, leaf.endPoint)
	node.flags = leaf.flags
	node.parseState = leaf.parseState
	node.preGotoState = leaf.preGotoState
	node.productionID = leaf.productionID
	node.rawShape = leaf.rawShape
	node.dynamicPrecedence = leaf.dynamicPrecedence
	if leaf.hasCheckpoint && arena != nil {
		if arena.setExternalScannerCheckpoint(node, leaf.checkpoint) {
			arena.externalScannerCheckpointLeafNodes++
			if arena.checkpointLeafFullNodesAvoided > 0 {
				arena.checkpointLeafFullNodesAvoided--
			}
		}
	}
	setStackEntryNode(&entry, node)
	arena.recordCompactFullLeafMaterialized(reason)
	return node, entry
}
