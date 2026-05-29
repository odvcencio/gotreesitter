package gotreesitter

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Range is a span of source text.
type Range struct {
	StartByte  uint32
	EndByte    uint32
	StartPoint Point
	EndPoint   Point
}

// Node is a syntax tree node.
type Node struct {
	// Layout is performance-sensitive. Keep TestNodeLayoutSizeBudget updated
	// when changing field order or adding fields.
	children     []*Node
	fieldIDs     []FieldID // parallel to children, 0 = no field
	fieldSources []uint8   // parallel to children, 0 = none, 1 = direct, 2 = inherited
	parent       *Node
	ownerArena   *nodeArena
	startPoint   Point
	endPoint     Point
	startByte    uint32
	endByte      uint32
	parseState   StateID // parser state after this node was pushed
	preGotoState StateID // parser state before goto (state exposed after popping children)
	equivVersion uint32
	childIndex   int32
	symbol       Symbol
	productionID uint16
	flags        nodeFlags
	dirtyFlag    bool
}

type nodeFlags uint8

const (
	nodeFlagNamed nodeFlags = 1 << iota
	nodeFlagExtra
	nodeFlagMissing
	nodeFlagHasError
	nodeFlagDirty
)

func (n *Node) hasFlag(flag nodeFlags) bool {
	return n.flags&flag != 0
}

func (n *Node) setFlag(flag nodeFlags, enabled bool) {
	if enabled {
		n.flags |= flag
		return
	}
	n.flags &^= flag
}

func (n *Node) isNamed() bool      { return n.hasFlag(nodeFlagNamed) }
func (n *Node) setNamed(v bool)    { n.setFlag(nodeFlagNamed, v) }
func (n *Node) isExtra() bool      { return n.hasFlag(nodeFlagExtra) }
func (n *Node) setExtra(v bool)    { n.setFlag(nodeFlagExtra, v) }
func (n *Node) isMissing() bool    { return n.hasFlag(nodeFlagMissing) }
func (n *Node) setMissing(v bool)  { n.setFlag(nodeFlagMissing, v) }
func (n *Node) hasError() bool     { return n.hasFlag(nodeFlagHasError) }
func (n *Node) setHasError(v bool) { n.setFlag(nodeFlagHasError, v) }
func (n *Node) dirty() bool {
	return n != nil && (n.dirtyFlag || n.hasFlag(nodeFlagDirty))
}

func (n *Node) setDirty(v bool) {
	if n == nil {
		return
	}
	n.dirtyFlag = v
	n.setFlag(nodeFlagDirty, v)
}

// Version 0 is valid for fresh immutable arena nodes; initialized public nodes
// use 1 so later mutations can still invalidate equivalence-cache entries.
func nodeInitEquivVersion(n *Node) {
	if n == nil {
		return
	}
	n.equivVersion = 1
}

func nodeBumpEquivVersion(n *Node) {
	if n == nil {
		return
	}
	n.equivVersion++
	if n.equivVersion == 0 {
		n.equivVersion = 1
	}
}

func defaultFieldSourcesInArena(arena *nodeArena, fieldIDs []FieldID) []uint8 {
	if len(fieldIDs) == 0 {
		return nil
	}
	var out []uint8
	if arena != nil {
		out = arena.allocFieldSourceSlice(len(fieldIDs))
	} else {
		out = make([]uint8, len(fieldIDs))
	}
	for i, fid := range fieldIDs {
		if fid != 0 {
			out[i] = fieldSourceDirect
		}
	}
	return out
}

type finalChildSidecar struct {
	childRange       pendingChildRange
	parent           *Node
	parentChildIndex int32
}

const finalChildSidecarIndexBase int32 = -2

func finalChildSidecarID(childIndex int32) (int, bool) {
	if childIndex > finalChildSidecarIndexBase {
		return 0, false
	}
	return int(-childIndex - 2), true
}

func (a *nodeArena) attachFinalChildRefs(parent *Node, childRange pendingChildRange) {
	if a == nil || parent == nil || childRange.count() == 0 {
		return
	}
	if len(a.finalChildSidecars) >= int(^uint32(0)>>1) {
		return
	}
	oldCap := cap(a.finalChildSidecars)
	id := len(a.finalChildSidecars)
	a.finalChildSidecars = append(a.finalChildSidecars, finalChildSidecar{
		childRange:       childRange,
		parentChildIndex: -1,
	})
	if newCap := cap(a.finalChildSidecars); newCap != oldCap {
		a.allocatedBytes += finalChildSidecarBytesForCap(newCap) - finalChildSidecarBytesForCap(oldCap)
	}
	parent.childIndex = -int32(id) - 2
	a.finalChildRefParents++
	a.finalChildRefsCreated += uint64(childRange.count())
}

func (a *nodeArena) finalChildSidecarForNode(n *Node) (*finalChildSidecar, bool) {
	if a == nil || n == nil {
		return nil, false
	}
	id, ok := finalChildSidecarID(n.childIndex)
	if !ok || id < 0 || id >= len(a.finalChildSidecars) {
		return nil, false
	}
	return &a.finalChildSidecars[id], true
}

func (a *nodeArena) finalChildRange(parent *Node) (pendingChildRange, bool) {
	if a == nil || parent == nil {
		return 0, false
	}
	sidecar, ok := a.finalChildSidecarForNode(parent)
	if !ok {
		return 0, false
	}
	childRange := sidecar.childRange
	return childRange, childRange.count() > 0
}

func (a *nodeArena) clearFinalChildRefs(parent *Node) {
	if a == nil || parent == nil {
		return
	}
	sidecar, ok := a.finalChildSidecarForNode(parent)
	if !ok {
		return
	}
	restoredIndex := sidecar.parentChildIndex
	*sidecar = finalChildSidecar{}
	parent.childIndex = restoredIndex
	if restoredIndex < 0 {
		parent.childIndex = -1
	}
}

func setNodeRootLink(n *Node) {
	if n == nil {
		return
	}
	n.parent = nil
	if sidecar, ok := n.ownerArena.finalChildSidecarForNode(n); ok {
		sidecar.parent = nil
		sidecar.parentChildIndex = -1
		return
	}
	n.childIndex = -1
}

func setNodeParentLink(child, parent *Node, index int) {
	if child == nil {
		return
	}
	child.parent = parent
	if sidecar, ok := child.ownerArena.finalChildSidecarForNode(child); ok {
		sidecar.parent = parent
		sidecar.parentChildIndex = int32(index)
		return
	}
	child.childIndex = int32(index)
}

func nodeParentLink(n *Node) (*Node, int, bool) {
	if n == nil {
		return nil, -1, false
	}
	if sidecar, ok := n.ownerArena.finalChildSidecarForNode(n); ok {
		if sidecar.parent != nil {
			return sidecar.parent, int(sidecar.parentChildIndex), sidecar.parentChildIndex >= 0
		}
		if n.parent != nil {
			return n.parent, int(sidecar.parentChildIndex), sidecar.parentChildIndex >= 0
		}
		return nil, -1, false
	}
	if n.parent == nil {
		return nil, -1, false
	}
	return n.parent, int(n.childIndex), n.childIndex >= 0
}

func nodeMaterializedChildAtNoMaterialize(n *Node, i int) *Node {
	entry, ok := nodeChildEntryAtNoMaterialize(n, i)
	if !ok {
		return nil
	}
	return stackEntryNode(entry)
}

// wireParentPathToNodeNoMaterialize wires the parent links along the single path
// from root to target (and leaves untouched subtrees unwired). It is used only by
// the single-threaded incremental-edit path (nodeEditRoot) to keep edit-time
// finalChildRefs materialization lazy. It must NOT be used from the concurrent
// public read paths (Parent/NextSibling/PrevSibling) — those write parent links
// under parentLinkMu via ensureParentLinks instead (issue #93 race fix).
func wireParentPathToNodeNoMaterialize(root, target *Node) bool {
	if root == nil || target == nil {
		return false
	}
	if root == target {
		setNodeRootLink(root)
		return true
	}

	type pathFrame struct {
		node       *Node
		childIndex int
		next       int
	}

	path := []pathFrame{{node: root, childIndex: -1}}
	for len(path) > 0 {
		top := &path[len(path)-1]
		if top.next >= nodeChildCountNoMaterialize(top.node) {
			path = path[:len(path)-1]
			continue
		}
		childIndex := top.next
		top.next++
		child := nodeMaterializedChildAtNoMaterialize(top.node, childIndex)
		if child == nil {
			continue
		}
		if child == target {
			setNodeRootLink(root)
			for i := 1; i < len(path); i++ {
				setNodeParentLink(path[i].node, path[i-1].node, path[i].childIndex)
			}
			setNodeParentLink(child, top.node, childIndex)
			return true
		}
		path = append(path, pathFrame{
			node:       child,
			childIndex: childIndex,
		})
	}
	return false
}

func nodeDeferredParentRoot(n *Node) (*Node, bool) {
	if n == nil || n.ownerArena == nil {
		return nil, false
	}
	arena := n.ownerArena
	arena.parentLinkMu.Lock()
	deferredRoot := arena.deferredParentRoot
	parentLinksDeferred := arena.parentLinksDeferred.Load()
	arena.parentLinkMu.Unlock()
	if !parentLinksDeferred || deferredRoot == nil {
		return nil, false
	}
	return deferredRoot, true
}

// wireDeferredParentPathToNode performs single-path parent wiring for the
// incremental-edit path only (single-threaded). Concurrent read traversal goes
// through ensureParentLinks (wire-all-once under parentLinkMu).
func wireDeferredParentPathToNode(n *Node) (*Node, bool) {
	deferredRoot, ok := nodeDeferredParentRoot(n)
	if !ok {
		return nil, false
	}
	if deferredRoot == n {
		setNodeRootLink(deferredRoot)
		return deferredRoot, true
	}
	if wireParentPathToNodeNoMaterialize(deferredRoot, n) {
		return deferredRoot, true
	}
	return deferredRoot, false
}

func nodeEditRoot(n *Node) *Node {
	if n == nil {
		return nil
	}
	root := n
	for {
		parent, _, _ := nodeParentLink(root)
		if parent == nil {
			break
		}
		root = parent
	}
	arena := n.ownerArena
	if arena == nil {
		return root
	}

	deferredRoot, hasDeferredRoot := nodeDeferredParentRoot(n)
	if !hasDeferredRoot || root == deferredRoot {
		return root
	}
	if _, ok := wireDeferredParentPathToNode(n); ok {
		return deferredRoot
	}

	n.ensureParentLinks()
	root = n
	for {
		parent, _, _ := nodeParentLink(root)
		if parent == nil {
			return root
		}
		root = parent
	}
}

func nodeChildCountNoMaterialize(n *Node) int {
	if n == nil {
		return 0
	}
	if n.childIndex > finalChildSidecarIndexBase || n.ownerArena == nil {
		return len(n.children)
	}
	id := int(-n.childIndex - 2)
	if id < 0 || id >= len(n.ownerArena.finalChildSidecars) {
		return len(n.children)
	}
	count := n.ownerArena.finalChildSidecars[id].childRange.count()
	if count > 0 {
		return count
	}
	return len(n.children)
}

func nodeHasFinalChildRefs(n *Node) bool {
	if n == nil || n.ownerArena == nil {
		return false
	}
	_, ok := n.ownerArena.finalChildRange(n)
	return ok
}

func nodeChildEntryAtNoMaterialize(n *Node, i int) (stackEntry, bool) {
	if n == nil || i < 0 {
		return stackEntry{}, false
	}
	if n.ownerArena != nil {
		if childRange, ok := n.ownerArena.finalChildRange(n); ok {
			if i >= childRange.count() {
				return stackEntry{}, false
			}
			refs := childRange.refs(n.ownerArena)
			if i >= len(refs) {
				return stackEntry{}, false
			}
			entry := refs[i].stackEntry()
			if entry.node == nil {
				return entry, entry.kind != stackEntryKindNode
			}
			return entry, true
		}
	}
	if i >= len(n.children) || n.children[i] == nil {
		return stackEntry{}, false
	}
	child := n.children[i]
	return newStackEntryNode(child.parseState, child), true
}

func nodeMaterializeFinalChildRefs(n *Node, reason materializeReason) {
	if n == nil || n.ownerArena == nil {
		return
	}
	arena := n.ownerArena
	childRange, ok := arena.finalChildRange(n)
	if !ok {
		return
	}
	refs := childRange.refs(arena)
	count := childRange.count()
	children := arena.allocNodeSliceNoClear(count)
	for i := 0; i < count; i++ {
		entry := refs[i].stackEntry()
		child, updated := materializeStackEntryPayloadEntryWithParser(
			nil,
			arena,
			entry,
			compactFullLeafMaterializeReason(reason),
			pendingParentMaterializeReason(reason),
		)
		children[i] = child
		refs[i] = newPendingChildEntry(updated)
		if child != nil {
			setNodeParentLink(child, n, i)
		}
	}
	n.children = children
	arena.clearFinalChildRefs(n)
	arena.finalChildRefsMaterializedParents++
	arena.finalChildRefsMaterializedChildren += uint64(count)
}

func nodeMaterializeFinalChildRefAt(n *Node, i int, reason materializeReason) *Node {
	if n == nil {
		return nil
	}
	if n.ownerArena == nil {
		if i < 0 || i >= len(n.children) {
			return nil
		}
		return n.children[i]
	}
	arena := n.ownerArena
	childRange, ok := arena.finalChildRange(n)
	if !ok {
		if i < 0 || i >= len(n.children) {
			return nil
		}
		return n.children[i]
	}
	if i < 0 || i >= childRange.count() {
		return nil
	}
	refs := childRange.refs(arena)
	if i >= len(refs) {
		return nil
	}
	entry := refs[i].stackEntry()
	arena.finalChildRefsSingleChildAccesses++
	wasMaterialized := entry.kind == stackEntryKindNode
	child, updated := materializeStackEntryPayloadEntryWithParser(
		nil,
		arena,
		entry,
		compactFullLeafMaterializeReason(reason),
		pendingParentMaterializeReason(reason),
	)
	refs[i] = newPendingChildEntry(updated)
	if child == nil {
		return nil
	}
	setNodeParentLink(child, n, i)
	if !wasMaterialized {
		arena.finalChildRefsSingleChildMaterializedChildren++
	}
	return child
}

func nodeChildCount(n *Node) int {
	return nodeChildCountNoMaterialize(n)
}

func nodeChildAtForReason(n *Node, i int, reason materializeReason) *Node {
	if n == nil || i < 0 || i >= nodeChildCountNoMaterialize(n) {
		return nil
	}
	return nodeMaterializeFinalChildRefAt(n, i, reason)
}

func nodeChildrenForReason(n *Node, reason materializeReason) []*Node {
	if n == nil {
		return nil
	}
	nodeMaterializeFinalChildRefs(n, reason)
	return n.children
}

func nodeFieldIDAt(n *Node, i int) FieldID {
	if n == nil || i < 0 || i >= len(n.fieldIDs) {
		return 0
	}
	return n.fieldIDs[i]
}

// ParseStopReason reports why parseInternal terminated.
type ParseStopReason string

const (
	ParseStopNone            ParseStopReason = "none"
	ParseStopAccepted        ParseStopReason = "accepted"
	ParseStopNoStacksAlive   ParseStopReason = "no_stacks_alive"
	ParseStopTokenSourceEOF  ParseStopReason = "token_source_eof"
	ParseStopTimeout         ParseStopReason = "timeout"
	ParseStopCancelled       ParseStopReason = "cancelled"
	ParseStopIterationLimit  ParseStopReason = "iteration_limit"
	ParseStopStackDepthLimit ParseStopReason = "stack_depth_limit"
	ParseStopNodeLimit       ParseStopReason = "node_limit"
	ParseStopMemoryBudget    ParseStopReason = "memory_budget"
)

type PendingParentRejectStats struct {
	Unknown    uint64
	Empty      uint64
	ChildLimit uint64
	Alias      uint64
	RawSpan    uint64
	Fields     uint64
	Child      uint64
	Span       uint64
	Fill       uint64
}

type PendingParentFieldRejectStats struct {
	Unknown               uint64
	ParentHidden          uint64
	NoIDs                 uint64
	Inherited             uint64
	HiddenChild           uint64
	HiddenChildPlain      uint64
	HiddenChildPlainEmpty uint64
	HiddenChildPlainOne   uint64
	HiddenChildPlainMany  uint64
	HiddenChildWithFields uint64
	Child                 uint64
	AllVisibleDirect      uint64
}

type PendingParentFieldRejectPayloadStats struct {
	Unknown              uint64
	Visible              uint64
	VisibleFinalLike     uint64
	VisibleNestedPayload uint64
	VisibleCompactLeaf   uint64
	VisibleFieldedDesc   uint64
	HiddenEmpty          uint64
	HiddenOne            uint64
	HiddenMany           uint64
	HiddenWithFields     uint64
}

type ParseEquivStateRuntime struct {
	State                                 StateID
	StackEquivCalls                       uint64
	StackEquivTrue                        uint64
	StackEquivDepthMismatch               uint64
	StackEquivHashMismatch                uint64
	StackEquivStateMismatch               uint64
	StackEquivPayloadMismatch             uint64
	StackEquivEntryCompares               uint64
	StackEquivStateMismatchDepthSum       uint64
	StackEquivStateMismatchMaxDepth       uint32
	StackEquivStateMismatchDepthBuckets   [stackEquivMismatchDepthBucketCount]uint64
	StackEquivPayloadMismatchDepthSum     uint64
	StackEquivPayloadMismatchMaxDepth     uint32
	StackEquivPayloadMismatchDepthBuckets [stackEquivMismatchDepthBucketCount]uint64
	StackEquivPayloadHeaderSigDiff        uint64
	StackEquivPayloadHeaderSigSame        uint64
	StackEquivPayloadShallowSigDiff       uint64
	StackEquivPayloadShallowSigSame       uint64
	StackEquivPairKeyed                   uint64
	StackEquivPairUnkeyed                 uint64
	StackEquivPairRepeats                 uint64
	StackEquivPairRepeatTrue              uint64
	StackEquivPairRepeatFalse             uint64
	StackEquivPairRepeatMismatch          uint64
	StackEquivPairStores                  uint64
	EquivCacheLookups                     uint64
	EquivCacheHits                        uint64
	EquivCacheStores                      uint64
	EquivCacheMisses                      uint64
	EquivCacheTrueHits                    uint64
	EquivCacheFalseHits                   uint64
	EquivCacheEpochMisses                 uint64
	EquivCacheKeyMisses                   uint64
	EquivCacheVersionMisses               uint64
	EquivSkipError                        uint64
	EquivSkipLeaf                         uint64
	EquivSkipFieldMismatch                uint64
	EquivExactCalls                       uint64
	EquivExactTrue                        uint64
	EquivExactPointerTrue                 uint64
	EquivExactNilMismatch                 uint64
	EquivExactHeaderMismatch              uint64
	EquivExactChildMismatch               uint64
	EquivExactTerminalCalls               uint64
	EquivExactTerminalTrue                uint64
	EquivExactTerminalFalse               uint64
	EquivFrontierCalls                    uint64
	EquivFrontierTrue                     uint64
	EquivExactChildCompares               uint64
	EquivFrontierChildScans               uint64
	EquivFrontierCandidateCompares        uint64
}

// ParseRuntime captures parser-loop diagnostics for a completed tree.
type ParseRuntime struct {
	StopReason                                   ParseStopReason
	SourceLen                                    uint32
	ExpectedEOFByte                              uint32
	RootEndByte                                  uint32
	Truncated                                    bool
	TokenSourceEOFEarly                          bool
	TokensConsumed                               uint64
	LastTokenEndByte                             uint32
	LastTokenSymbol                              Symbol
	LastTokenWasEOF                              bool
	IterationLimit                               int
	StackDepthLimit                              int
	NodeLimit                                    int
	MemoryBudgetBytes                            int64
	Iterations                                   int
	NodesAllocated                               int
	ArenaBytesAllocated                          int64
	ScratchBytesAllocated                        int64
	EntryScratchBytesAllocated                   int64
	GSSBytesAllocated                            int64
	PeakStackDepth                               int
	MaxStacksSeen                                int
	SingleStackIterations                        int
	MultiStackIterations                         int
	SingleStackTokens                            uint64
	MultiStackTokens                             uint64
	SingleStackGSSNodes                          uint64
	MultiStackGSSNodes                           uint64
	GSSNodesAllocated                            uint64
	GSSNodesRetained                             uint64
	GSSNodesDroppedSameToken                     uint64
	ParentNodesAllocated                         uint64
	ParentNodesRetained                          uint64
	ParentNodesDroppedSameToken                  uint64
	LeafNodesAllocated                           uint64
	LeafNodesRetained                            uint64
	LeafNodesDroppedSameToken                    uint64
	ChildSlicesAllocated                         uint64
	ChildSlicesRetained                          uint64
	ChildSlicesDroppedSameToken                  uint64
	ChildPointersAllocated                       uint64
	ChildPointersRetained                        uint64
	ChildPointersDroppedSameToken                uint64
	ReduceChildFastGSS                           ReduceChildPathRuntime
	ReduceChildAllVisible                        ReduceChildPathRuntime
	ReduceChildNoAlias                           ReduceChildPathRuntime
	ReduceChildScratchGeneral                    ReduceChildPathRuntime
	ReduceChildScratchNoAlias                    ReduceChildPathRuntime
	TransientChildSlicesAllocated                uint64
	TransientChildPointersAllocated              uint64
	TransientChildSlicesMaterialized             uint64
	TransientChildPointersMaterialized           uint64
	TransientParentNodesAllocated                uint64
	TransientParentNodesMaterialized             uint64
	FinalNodes                                   uint64
	FinalParentNodes                             uint64
	FinalLeafNodes                               uint64
	FinalFieldedParentNodes                      uint64
	FinalUnfieldedParentNodes                    uint64
	FinalVisibleParentNodes                      uint64
	FinalHiddenParentNodes                       uint64
	FinalCheckpointLeafNodes                     uint64
	FinalChildSlices                             uint64
	FinalChildPointers                           uint64
	FinalFieldIDElements                         uint64
	FinalFieldSourceElements                     uint64
	FinalChildRefParents                         uint64
	FinalChildRefs                               uint64
	FinalChildRefMaterializedParents             uint64
	FinalChildRefMaterializedChildren            uint64
	FinalChildRefSingleChildAccesses             uint64
	FinalChildRefSingleChildMaterializedChildren uint64
	MergeStacksIn                                uint64
	MergeStacksOut                               uint64
	MergeSlotsUsed                               uint64
	GlobalCullStacksIn                           uint64
	GlobalCullStacksOut                          uint64
	StackEquivCalls                              uint64
	StackEquivTrue                               uint64
	StackEquivDepthMismatch                      uint64
	StackEquivHashMismatch                       uint64
	StackEquivStateMismatch                      uint64
	StackEquivPayloadMismatch                    uint64
	StackEquivEntryCompares                      uint64
	StackEquivStateMismatchDepthSum              uint64
	StackEquivStateMismatchMaxDepth              uint32
	StackEquivStateMismatchDepthBuckets          [stackEquivMismatchDepthBucketCount]uint64
	StackEquivPayloadMismatchDepthSum            uint64
	StackEquivPayloadMismatchMaxDepth            uint32
	StackEquivPayloadMismatchDepthBuckets        [stackEquivMismatchDepthBucketCount]uint64
	StackEquivPayloadHeaderSigDiff               uint64
	StackEquivPayloadHeaderSigSame               uint64
	StackEquivPayloadShallowSigDiff              uint64
	StackEquivPayloadShallowSigSame              uint64
	StackEquivPairKeyed                          uint64
	StackEquivPairUnkeyed                        uint64
	StackEquivPairRepeats                        uint64
	StackEquivPairRepeatTrue                     uint64
	StackEquivPairRepeatFalse                    uint64
	StackEquivPairRepeatMismatch                 uint64
	StackEquivPairStores                         uint64
	MergeHeaderEqTotal                           uint64
	MergeDeepTrue                                uint64
	MergeDeepFalse                               uint64
	MergeHeaderDeepDivergent                     uint64
	EquivCacheLookups                            uint64
	EquivCacheHits                               uint64
	EquivCacheStores                             uint64
	EquivCacheMisses                             uint64
	EquivCacheTrueHits                           uint64
	EquivCacheFalseHits                          uint64
	EquivCacheEpochMisses                        uint64
	EquivCacheKeyMisses                          uint64
	EquivCacheVersionMisses                      uint64
	EquivSkipError                               uint64
	EquivSkipLeaf                                uint64
	EquivSkipFieldMismatch                       uint64
	EquivExactCalls                              uint64
	EquivExactTrue                               uint64
	EquivExactPointerTrue                        uint64
	EquivExactNilMismatch                        uint64
	EquivExactHeaderMismatch                     uint64
	EquivExactChildMismatch                      uint64
	EquivExactTerminalCalls                      uint64
	EquivExactTerminalTrue                       uint64
	EquivExactTerminalFalse                      uint64
	EquivFrontierCalls                           uint64
	EquivFrontierTrue                            uint64
	EquivExactChildCompares                      uint64
	EquivFrontierChildScans                      uint64
	EquivFrontierCandidateCompares               uint64
	EquivStateStats                              []ParseEquivStateRuntime
	ParseWallNanos                               int64
	ParserLoopNanos                              int64
	TokenNextNanos                               int64
	ActionDispatchNanos                          int64
	ActionLookupNanos                            int64
	GLRMergeNanos                                int64
	GLRCullNanos                                 int64
	ReduceTiming                                 *ParseReduceTiming
	ActionTiming                                 *ParseActionTiming

	ExternalScannerCheckpointRecords                 uint64
	ExternalScannerCheckpointSlotsAllocated          uint64
	ExternalScannerCheckpointBytesAllocated          int64
	ExternalScannerSnapshotBytesAllocated            uint64
	ExternalScannerCheckpointLeafNodes               uint64
	CompactFullLeafCreated                           uint64
	CompactFullLeafMaterialized                      uint64
	CompactFullLeafMaterializedForParentReduce       uint64
	CompactFullLeafMaterializedForParentReject       PendingParentRejectStats
	CompactFullLeafMaterializedForFinalTree          uint64
	CompactFullLeafMaterializedForNormalization      uint64
	CompactFullLeafMaterializedForRecovery           uint64
	CompactFullLeafMaterializedForQuery              uint64
	CompactFullLeafMaterializedForCursor             uint64
	CompactFullLeafMaterializedForParentAPI          uint64
	CompactFullLeafMaterializedForEdit               uint64
	CompactFullLeafMaterializedForCheckpointRebuild  uint64
	CompactFullLeafDropped                           uint64
	CompactFullLeafMaterializedForFieldRejectPayload PendingParentFieldRejectPayloadStats
	PendingParentCreated                             uint64
	PendingParentMaterialized                        uint64
	PendingParentMaterializedForParentReduce         uint64
	PendingParentMaterializedForParentReject         PendingParentRejectStats
	PendingParentMaterializedForFieldReject          PendingParentFieldRejectStats
	PendingParentMaterializedForFieldRejectPayload   PendingParentFieldRejectPayloadStats
	PendingParentMaterializedForFinalTree            uint64
	PendingParentMaterializedForNormalization        uint64
	PendingParentMaterializedForRecovery             uint64
	PendingParentMaterializedForQuery                uint64
	PendingParentMaterializedForCursor               uint64
	PendingParentMaterializedForParentAPI            uint64
	PendingParentMaterializedForEdit                 uint64
	PendingParentMaterializedForCheckpointRebuild    uint64
	PendingParentDropped                             uint64
	PendingParentsFlattened                          uint64
	PendingChildRefsFlattened                        uint64
	PendingChildEntriesAllocated                     uint64
	PendingChildEntryCapacity                        uint64
	PendingChildEntryWaste                           uint64
	PendingParentCandidates                          uint64
	PendingParentRejectedEmpty                       uint64
	PendingParentRejectedChildLimit                  uint64
	PendingParentRejectedAlias                       uint64
	PendingParentRejectedRawSpan                     uint64
	PendingParentRejectedFields                      uint64
	PendingParentRejectedFieldsParentHidden          uint64
	PendingParentRejectedFieldsNoIDs                 uint64
	PendingParentRejectedFieldsInherited             uint64
	PendingParentRejectedFieldsHiddenChild           uint64
	PendingParentRejectedFieldsHiddenChildPlain      uint64
	PendingParentRejectedFieldsHiddenChildPlainEmpty uint64
	PendingParentRejectedFieldsHiddenChildPlainOne   uint64
	PendingParentRejectedFieldsHiddenChildPlainMany  uint64
	PendingParentRejectedFieldsHiddenChildWithFields uint64
	PendingParentRejectedFieldsChild                 uint64
	PendingParentRejectedFieldsAllVisibleDirect      uint64
	PendingParentRejectedChild                       uint64
	PendingParentRejectedSpan                        uint64
	PendingParentRejectedFill                        uint64
	PreMaterializationFieldRejectCandidates          uint64
	PreMaterializationFieldRejectSameKeyCandidates   uint64
	PreMaterializationFieldRejectOverflowCandidates  uint64

	CheckpointLeafFullNodesAvoided      uint64
	LeafNodesConstructed                uint64
	ParentNodesConstructed              uint64
	NoTreeReduceNodesConstructed        uint64
	NoTreeLeafNodesConstructed          uint64
	ResultSelectionNanos                int64
	TransientParentMaterializationNanos int64
	ResultTreeBuildNanos                int64
	TransientChildMaterializationNanos  int64
	ResultPythonKeywordRepairNanos      int64
	ResultPythonRootRepairNanos         int64
	ResultFinalizeRootNanos             int64
	ResultExtendTrailingNanos           int64
	ResultNormalizeRootStartNanos       int64
	ResultCompatibilityNanos            int64
	ResultParentLinkNanos               int64
	NormalizationPassesChecked          uint64
	NormalizationPassesRun              uint64
	NormalizationNodesVisited           uint64
	NormalizationNodesRewritten         uint64
	NormalizationNanos                  int64
	NormalizationPasses                 *[]NormalizationPassRuntime
}

type NormalizationPassRuntime struct {
	Name           string
	Checked        uint64
	Run            uint64
	NodesVisited   uint64
	NodesRewritten uint64
	Nanos          int64
}

type ParseReduceTiming struct {
	RangeNanos         int64
	PendingParentNanos int64
	ChildBuildNanos    int64
	ParentBuildNanos   int64
	SpanNanos          int64
	StackPushNanos     int64
	NoTreeBuildNanos   int64
}

type ParseActionTiming struct {
	ExtraShiftNanos      int64
	NoActionNanos        int64
	NoActionRelexNanos   int64
	NoActionMissingNanos int64
	NoActionRecoverNanos int64
	NoActionErrorNanos   int64
	ConflictChoiceNanos  int64
	ConflictForkNanos    int64
	SingleShiftNanos     int64
	SingleReduceNanos    int64
	SingleAcceptNanos    int64
	SingleRecoverNanos   int64
	SingleOtherNanos     int64
}

type ReduceChildPathRuntime struct {
	SlicesAllocated   uint64
	SlicesRetained    uint64
	SlicesDropped     uint64
	PointersAllocated uint64
	PointersRetained  uint64
	PointersDropped   uint64
}

type reduceChildPath uint8

const (
	reduceChildPathNone reduceChildPath = iota
	reduceChildPathFastGSS
	reduceChildPathAllVisible
	reduceChildPathNoAlias
	reduceChildPathScratchGeneral
	reduceChildPathScratchNoAlias
	reduceChildPathCount
)

func (p reduceChildPath) valid() bool {
	return p > reduceChildPathNone && p < reduceChildPathCount
}

// ArenaBreakdown captures optional arena/materialization attribution. It is
// populated only when EnableArenaBreakdown(true) is set before parsing.
type ArenaBreakdown struct {
	NodeStructBytesAllocated        int64
	NoTreeNodeBytesAllocated        int64
	CompactFullLeafBytesAllocated   int64
	PendingParentBytesAllocated     int64
	PendingChildEntryBytesAllocated int64
	FinalChildSidecarBytesAllocated int64
	PendingChildEntriesAllocated    uint64
	PendingChildEntryCapacity       uint64
	PendingChildEntryWaste          uint64
	ChildSliceBytesAllocated        int64
	FieldIDBytesAllocated           int64
	FieldSourceBytesAllocated       int64
	MergeScratchBytesAllocated      int64

	ArenaNodesConstructed uint64
	// NodeLiveCount is arena allocation-slot usage, not root-reachable tree
	// liveness. It includes parser alternatives and recovery nodes allocated
	// during the parse.
	NodeLiveCount                     uint64
	NodeCapacityCount                 uint64
	NodeCapacityWaste                 uint64
	PrimaryNodeCapacity               uint64
	PrimaryNodeUsed                   uint64
	OverflowNodeCapacity              uint64
	OverflowNodeUsed                  uint64
	OverflowNodeSlabs                 uint64
	LargestNodeSlabUsedFraction       float64
	LeafNodesConstructed              uint64
	ParentNodesConstructed            uint64
	FieldedParentNodesConstructed     uint64
	UnfieldedParentNodesConstructed   uint64
	ParentConstructedChildLen0        uint64
	ParentConstructedChildLen1        uint64
	ParentConstructedChildLen2        uint64
	ParentConstructedChildLen3        uint64
	ParentConstructedChildLen4Plus    uint64
	ParentConstructedNoLinks          uint64
	ParentConstructedWithLinks        uint64
	ParentConstructedTrackErrors      uint64
	ParentConstructedFieldSources     uint64
	ParentReductionVisible            uint64
	ParentReductionInvisible          uint64
	ParentReductionVisibleFielded     uint64
	ParentReductionVisibleUnfielded   uint64
	ParentReductionInvisibleFielded   uint64
	ParentReductionInvisibleUnfielded uint64
	ParentReductionVisibleChildPtrs   uint64
	ParentReductionInvisibleChildPtrs uint64
	ParentReductionVisibleLen0        uint64
	ParentReductionVisibleLen1        uint64
	ParentReductionVisibleLen2        uint64
	ParentReductionVisibleLen3        uint64
	ParentReductionVisibleLen4Plus    uint64
	ParentReductionInvisibleLen0      uint64
	ParentReductionInvisibleLen1      uint64
	ParentReductionInvisibleLen2      uint64
	ParentReductionInvisibleLen3      uint64
	ParentReductionInvisibleLen4Plus  uint64
	ReduceChildSlicesFastGSS          uint64
	ReduceChildPointersFastGSS        uint64
	ReduceChildSlicesAllVisible       uint64
	ReduceChildPointersAllVisible     uint64
	ReduceChildSlicesNoAlias          uint64
	ReduceChildPointersNoAlias        uint64
	ReduceChildSlicesScratchGeneral   uint64
	ReduceChildPointersScratchGeneral uint64
	ReduceChildSlicesScratchNoAlias   uint64
	ReduceChildPointersScratchNoAlias uint64
	CollapseRawUnaryAttempts          uint64
	CollapseRawUnarySuccesses         uint64
	CollapseRawUnaryMissShape         uint64
	CollapseRawUnaryMissGrammar       uint64
	CollapseRawUnaryMissChild         uint64
	CollapseRawUnaryMissRule          uint64
	CollapseUnaryAttempts             uint64
	CollapseUnarySuccesses            uint64
	CollapseUnaryMissShape            uint64
	CollapseUnaryMissGrammar          uint64
	CollapseUnaryMissFielded          uint64
	CollapseUnaryMissChild            uint64
	CollapseUnaryMissRule             uint64
	CollapseRuleSameSymbol            uint64
	CollapseRuleInvisibleWrapper      uint64
	CollapseRuleNamedLeafAlias        uint64
	NoTreeReduceNodesConstructed      uint64
	NoTreeLeafNodesConstructed        uint64
	NoTreePlaceholderNodesConstructed uint64
	OtherNodesConstructed             uint64
	ExtraNodesConstructed             uint64
	ErrorSymbolNodesConstructed       uint64
	HasErrorNodesConstructed          uint64
	ChildSlicesConstructed            uint64
	ChildPointersConstructed          uint64
	ChildSlicesLen1                   uint64
	ChildSlicesLen2                   uint64
	ChildSlicesLen3                   uint64
	ChildSlicesLen4Plus               uint64
	ParentChildPointersConstructed    uint64
	ParentChildrenLen0                uint64
	ParentChildrenLen1                uint64
	ParentChildrenLen2                uint64
	ParentChildrenLen3                uint64
	ParentChildrenLen4Plus            uint64
	FieldIDElementsConstructed        uint64
	FieldSourceElementsConstructed    uint64
}

// Summary returns a stable one-line diagnostic string for parse-runtime stats.
func (rt ParseRuntime) Summary() string {
	stopReason := rt.StopReason
	if stopReason == "" {
		stopReason = ParseStopNone
	}
	return fmt.Sprintf(
		"truncated=%v stopReason=%s tokenEOFEarly=%v tokens=%d lastTokenEnd=%d expectedEOF=%d lastTokenSymbol=%d lastTokenEOF=%v iterations=%d/%d nodes=%d/%d arena=%d/%d scratch=%d(entry=%d gss=%d)/%d peakDepth=%d/%d maxStacks=%d",
		rt.Truncated, stopReason, rt.TokenSourceEOFEarly, rt.TokensConsumed,
		rt.LastTokenEndByte, rt.ExpectedEOFByte, rt.LastTokenSymbol, rt.LastTokenWasEOF,
		rt.Iterations, rt.IterationLimit, rt.NodesAllocated, rt.NodeLimit,
		rt.ArenaBytesAllocated, rt.MemoryBudgetBytes,
		rt.ScratchBytesAllocated, rt.EntryScratchBytesAllocated, rt.GSSBytesAllocated, rt.MemoryBudgetBytes,
		rt.PeakStackDepth, rt.StackDepthLimit, rt.MaxStacksSeen,
	)
}

// Symbol returns the node's grammar symbol.
func (n *Node) Symbol() Symbol { return n.symbol }

// ParseState returns the parser state associated with this node.
func (n *Node) ParseState() StateID { return n.parseState }

// PreGotoState returns the parser state that was on top of the stack before
// this node was pushed (i.e., the state exposed after popping children during
// reduce). For non-leaf nodes: lookupGoto(PreGotoState, Symbol) == ParseState.
func (n *Node) PreGotoState() StateID { return n.preGotoState }

// IsNamed reports whether this is a named node (as opposed to anonymous syntax like punctuation).
func (n *Node) IsNamed() bool { return n.isNamed() }

// IsExtra reports whether this node was marked as extra syntax
// (e.g. whitespace/comments outside the core parse structure).
func (n *Node) IsExtra() bool { return n.isExtra() }

// IsMissing reports whether this node was inserted by error recovery.
func (n *Node) IsMissing() bool { return n.isMissing() }

// IsError reports whether this node is an explicit error node.
func (n *Node) IsError() bool { return n.symbol == errorSymbol }

// HasError reports whether this node or any descendant contains a parse error.
func (n *Node) HasError() bool { return n.hasError() }

// HasChanges reports whether this node was marked dirty by Tree.Edit.
func (n *Node) HasChanges() bool { return n.dirty() }

// StartByte returns the byte offset where this node begins.
func (n *Node) StartByte() uint32 { return n.startByte }

// EndByte returns the byte offset where this node ends (exclusive).
func (n *Node) EndByte() uint32 { return n.endByte }

// StartPoint returns the row/column position where this node begins.
func (n *Node) StartPoint() Point { return n.startPoint }

// EndPoint returns the row/column position where this node ends.
func (n *Node) EndPoint() Point { return n.endPoint }

// Range returns the full span of this node as a Range.
func (n *Node) Range() Range {
	return Range{
		StartByte:  n.startByte,
		EndByte:    n.endByte,
		StartPoint: n.startPoint,
		EndPoint:   n.endPoint,
	}
}

// Parent returns this node's parent, or nil if it is the root.
func (n *Node) Parent() *Node {
	if n == nil {
		return nil
	}
	n.ensureParentLinks()
	parent, _, _ := nodeParentLink(n)
	return parent
}

// ChildCount returns the number of children (both named and anonymous).
func (n *Node) ChildCount() int { return nodeChildCount(n) }

// Child returns the i-th child, or nil if i is out of range.
func (n *Node) Child(i int) *Node {
	return nodeChildAtForReason(n, i, materializeForParentAPI)
}

// NextSibling returns the next sibling node, or nil when this is the last child
// or has no parent.
func (n *Node) NextSibling() *Node {
	if n == nil {
		return nil
	}
	n.ensureParentLinks()
	parent, index, ok := nodeParentLink(n)
	if parent == nil {
		return nil
	}
	childCount := nodeChildCountNoMaterialize(parent)
	if ok && index >= 0 && index < childCount && nodeChildAtForReason(parent, index, materializeForParentAPI) == n {
		if index+1 < childCount {
			return nodeChildAtForReason(parent, index+1, materializeForParentAPI)
		}
		return nil
	}
	for i := 0; i < childCount; i++ {
		if nodeMaterializedChildAtNoMaterialize(parent, i) != n {
			continue
		}
		if i+1 < childCount {
			return nodeChildAtForReason(parent, i+1, materializeForParentAPI)
		}
		return nil
	}
	return nil
}

// PrevSibling returns the previous sibling node, or nil when this is the first
// child or has no parent.
func (n *Node) PrevSibling() *Node {
	if n == nil {
		return nil
	}
	n.ensureParentLinks()
	parent, index, ok := nodeParentLink(n)
	if parent == nil {
		return nil
	}
	childCount := nodeChildCountNoMaterialize(parent)
	if ok && index >= 0 && index < childCount && nodeChildAtForReason(parent, index, materializeForParentAPI) == n {
		if index > 0 {
			return nodeChildAtForReason(parent, index-1, materializeForParentAPI)
		}
		return nil
	}
	for i := 0; i < childCount; i++ {
		if nodeMaterializedChildAtNoMaterialize(parent, i) != n {
			continue
		}
		if i > 0 {
			return nodeChildAtForReason(parent, i-1, materializeForParentAPI)
		}
		return nil
	}
	return nil
}

// NamedChildCount returns the number of named children.
func (n *Node) NamedChildCount() int {
	count := 0
	childCount := nodeChildCountNoMaterialize(n)
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(n, i)
		if ok && stackEntryNodeIsNamed(entry) {
			count++
		}
	}
	return count
}

// NamedChild returns the i-th named child (skipping anonymous children),
// or nil if i is out of range.
func (n *Node) NamedChild(i int) *Node {
	if i < 0 {
		return nil
	}
	count := 0
	childCount := nodeChildCountNoMaterialize(n)
	for childIndex := 0; childIndex < childCount; childIndex++ {
		entry, ok := nodeChildEntryAtNoMaterialize(n, childIndex)
		if !ok || !stackEntryNodeIsNamed(entry) {
			continue
		}
		if count == i {
			return nodeChildAtForReason(n, childIndex, materializeForParentAPI)
		}
		count++
	}
	return nil
}

// ChildByFieldName returns the first child assigned to the given field name,
// or nil if no child has that field. The Language is needed to resolve field
// names to IDs. Uses Language.FieldByName for O(1) lookup.
func (n *Node) ChildByFieldName(name string, lang *Language) *Node {
	fid, ok := lang.FieldByName(name)
	if !ok || fid == 0 {
		return nil
	}

	childCount := nodeChildCountNoMaterialize(n)
	for i := 0; i < childCount; i++ {
		if nodeFieldIDAt(n, i) == fid {
			return nodeChildAtForReason(n, i, materializeForParentAPI)
		}
	}
	return nil
}

// FieldNameForChild returns the field name assigned to the i-th child,
// or an empty string when no field is assigned.
func (n *Node) FieldNameForChild(i int, lang *Language) string {
	if n == nil || lang == nil || i < 0 || i >= nodeChildCountNoMaterialize(n) {
		return ""
	}
	fid := nodeFieldIDAt(n, i)
	if fid == 0 || int(fid) >= len(lang.FieldNames) {
		return ""
	}
	return lang.FieldNames[fid]
}

// Children returns a slice of all children.
func (n *Node) Children() []*Node { return nodeChildrenForReason(n, materializeForParentAPI) }

// SExpr returns a tree-sitter-style S-expression for this node.
// It includes only named nodes for stable debug snapshots.
func (n *Node) SExpr(lang *Language) string {
	if n == nil || lang == nil {
		return ""
	}
	if !n.IsNamed() {
		return ""
	}
	var b strings.Builder
	// S-expressions are typically ~5x the source byte count for named nodes.
	// Pre-growing the builder avoids intermediate reallocations.
	b.Grow((int(n.endByte-n.startByte) * 5) + 32)
	sexprWrite(n, lang, &b)
	return b.String()
}

// sexprWrite writes the S-expression for n into b, returning true if anything
// was written. Using a shared builder avoids the per-node string allocation and
// the intermediate []string slice that the previous implementation required.
func sexprWrite(n *Node, lang *Language, b *strings.Builder) {
	if n == nil || !n.IsNamed() {
		return
	}
	name := n.Type(lang)
	b.WriteByte('(')
	b.WriteString(name)

	// Walk children, writing only named ones. Because a named child always
	// produces at least "(type)", we can write a space before each one eagerly.
	childCount := nodeChildCountNoMaterialize(n)
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(n, i)
		if !ok || !stackEntryNodeIsNamed(entry) {
			continue
		}
		child := nodeChildAtForReason(n, i, materializeForParentAPI)
		if child == nil {
			continue
		}
		b.WriteByte(' ')
		sexprWrite(child, lang, b)
	}

	b.WriteByte(')')
}

// Text returns the source text covered by this node.
// Returns an empty string for nil nodes or invalid byte ranges.
func (n *Node) Text(source []byte) string {
	if n == nil {
		return ""
	}
	start := int(n.startByte)
	end := int(n.endByte)
	if end < start || start > len(source) || end > len(source) {
		return ""
	}
	return string(source[start:end])
}

// Type returns the node's type name from the language.
func (n *Node) Type(lang *Language) string {
	if n != nil && n.symbol == errorSymbol {
		return "ERROR"
	}
	if int(n.symbol) < len(lang.SymbolNames) {
		name := lang.SymbolNames[n.symbol]
		name = unescapePunctuationSymbolName(name)
		return name
	}
	return ""
}

func unescapePunctuationSymbolName(name string) string {
	if !strings.Contains(name, "\\") {
		return name
	}
	var b strings.Builder
	b.Grow(len(name))
	changed := false
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if r != '\\' {
			b.WriteRune(r)
			i += size
			continue
		}
		if i+size >= len(name) {
			b.WriteRune(r)
			i += size
			continue
		}
		next, nextSize := utf8.DecodeRuneInString(name[i+size:])
		if next == '\\' || unicode.IsLetter(next) || unicode.IsDigit(next) {
			b.WriteRune(r)
			i += size
			continue
		}
		changed = true
		b.WriteRune(next)
		i += size + nextSize
	}
	if !changed {
		return name
	}
	return b.String()
}

func pointLessThan(a, b Point) bool {
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.Column < b.Column
}

func pointLessOrEqual(a, b Point) bool {
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.Column <= b.Column
}

func (n *Node) containsByteRange(startByte, endByte uint32) bool {
	return startByte >= n.startByte && endByte <= n.endByte
}

func (n *Node) containsPointRange(startPoint, endPoint Point) bool {
	return pointLessOrEqual(n.startPoint, startPoint) && pointLessOrEqual(endPoint, n.endPoint)
}

func stackEntryContainsByteRange(entry stackEntry, startByte, endByte uint32) bool {
	return startByte >= stackEntryNodeStartByte(entry) && endByte <= stackEntryNodeEndByte(entry)
}

func stackEntryContainsPointRange(entry stackEntry, startPoint, endPoint Point) bool {
	return pointLessOrEqual(stackEntryNodeStartPoint(entry), startPoint) && pointLessOrEqual(endPoint, stackEntryNodeEndPoint(entry))
}

func (n *Node) descendantForByteRange(startByte, endByte uint32, namedOnly bool) *Node {
	if n == nil || endByte < startByte || !n.containsByteRange(startByte, endByte) {
		return nil
	}

	var deepest *Node
	if !namedOnly || n.isNamed() {
		deepest = n
	}
	childCount := nodeChildCountNoMaterialize(n)
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(n, i)
		if !ok || !stackEntryContainsByteRange(entry, startByte, endByte) {
			continue
		}
		child := nodeChildAtForReason(n, i, materializeForParentAPI)
		if child == nil {
			continue
		}
		if !child.containsByteRange(startByte, endByte) {
			continue
		}
		if d := child.descendantForByteRange(startByte, endByte, namedOnly); d != nil {
			deepest = d
		}
	}
	return deepest
}

func (n *Node) descendantForPointRange(startPoint, endPoint Point, namedOnly bool) *Node {
	if n == nil || pointLessThan(endPoint, startPoint) || !n.containsPointRange(startPoint, endPoint) {
		return nil
	}

	var deepest *Node
	if !namedOnly || n.isNamed() {
		deepest = n
	}
	childCount := nodeChildCountNoMaterialize(n)
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(n, i)
		if !ok || !stackEntryContainsPointRange(entry, startPoint, endPoint) {
			continue
		}
		child := nodeChildAtForReason(n, i, materializeForParentAPI)
		if child == nil {
			continue
		}
		if !child.containsPointRange(startPoint, endPoint) {
			continue
		}
		if d := child.descendantForPointRange(startPoint, endPoint, namedOnly); d != nil {
			deepest = d
		}
	}
	return deepest
}

// DescendantForByteRange returns the smallest descendant that fully contains
// the given byte range, or nil when no such descendant exists.
func (n *Node) DescendantForByteRange(startByte, endByte uint32) *Node {
	return n.descendantForByteRange(startByte, endByte, false)
}

// NamedDescendantForByteRange returns the smallest named descendant that fully
// contains the given byte range, or nil when no such descendant exists.
func (n *Node) NamedDescendantForByteRange(startByte, endByte uint32) *Node {
	return n.descendantForByteRange(startByte, endByte, true)
}

// DescendantForPointRange returns the smallest descendant that fully contains
// the given point range, or nil when no such descendant exists.
func (n *Node) DescendantForPointRange(startPoint, endPoint Point) *Node {
	return n.descendantForPointRange(startPoint, endPoint, false)
}

// NamedDescendantForPointRange returns the smallest named descendant that
// fully contains the given point range, or nil when no such descendant exists.
func (n *Node) NamedDescendantForPointRange(startPoint, endPoint Point) *Node {
	return n.descendantForPointRange(startPoint, endPoint, true)
}

// NewLeafNode creates a terminal/leaf node.
func NewLeafNode(sym Symbol, named bool, startByte, endByte uint32, startPoint, endPoint Point) *Node {
	n := &Node{
		symbol:     sym,
		startByte:  startByte,
		endByte:    endByte,
		startPoint: startPoint,
		endPoint:   endPoint,
		childIndex: -1,
	}
	n.setNamed(named)
	nodeInitEquivVersion(n)
	return n
}

func populateParentNode(n *Node, children []*Node) {
	switch len(children) {
	case 0:
		return
	case 1:
		c0 := children[0]
		n.startByte = c0.startByte
		n.endByte = c0.endByte
		n.startPoint = c0.startPoint
		n.endPoint = c0.endPoint
		setNodeParentLink(c0, n, 0)
		n.setHasError(c0.hasError())
		return
	case 2:
		c0 := children[0]
		c1 := children[1]
		n.startByte = c0.startByte
		n.endByte = c1.endByte
		n.startPoint = c0.startPoint
		n.endPoint = c1.endPoint
		setNodeParentLink(c0, n, 0)
		setNodeParentLink(c1, n, 1)
		n.setHasError(c0.hasError() || c1.hasError())
		return
	default:
		first := children[0]
		last := children[len(children)-1]
		n.startByte = first.startByte
		n.endByte = last.endByte
		n.startPoint = first.startPoint
		n.endPoint = last.endPoint

		for i, c := range children {
			setNodeParentLink(c, n, i)
			if c.hasError() {
				n.setHasError(true)
				break
			}
		}
	}
}

// populateParentNodeNoLinks computes parent span/error metadata from children
// without wiring child.parent/childIndex links. Used on deferred-link paths.
func populateParentNodeNoLinks(n *Node, children []*Node, trackChildErrors bool) {
	switch len(children) {
	case 0:
		return
	case 1:
		c0 := children[0]
		n.startByte = c0.startByte
		n.endByte = c0.endByte
		n.startPoint = c0.startPoint
		n.endPoint = c0.endPoint
		if trackChildErrors {
			n.setHasError(c0.hasError())
		}
		return
	case 2:
		c0 := children[0]
		c1 := children[1]
		n.startByte = c0.startByte
		n.endByte = c1.endByte
		n.startPoint = c0.startPoint
		n.endPoint = c1.endPoint
		if trackChildErrors {
			n.setHasError(c0.hasError() || c1.hasError())
		}
		return
	default:
		first := children[0]
		last := children[len(children)-1]
		n.startByte = first.startByte
		n.endByte = last.endByte
		n.startPoint = first.startPoint
		n.endPoint = last.endPoint
		if trackChildErrors {
			for i := range children {
				if children[i].hasError() {
					n.setHasError(true)
					break
				}
			}
		}
	}
}

func wireParentLinksWithScratch(root *Node, scratch *[]*Node) {
	if root == nil {
		return
	}
	setNodeRootLink(root)

	var stack []*Node
	if scratch != nil {
		stack = (*scratch)[:0]
	} else {
		var local [64]*Node
		stack = local[:0]
	}
	stack = append(stack, root)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		childCount := nodeChildCountNoMaterialize(n)
		for i := 0; i < childCount; i++ {
			c := nodeChildAtForReason(n, i, materializeForParentAPI)
			if c == nil {
				continue
			}
			setNodeParentLink(c, n, i)
			stack = append(stack, c)
		}
	}
	if scratch != nil {
		*scratch = stack[:0]
	}
}

type finalTreeMaterializationStats struct {
	nodes                uint64
	parentNodes          uint64
	leafNodes            uint64
	fieldedParentNodes   uint64
	unfieldedParentNodes uint64
	visibleParentNodes   uint64
	hiddenParentNodes    uint64
	checkpointLeafNodes  uint64
	childSlices          uint64
	childPointers        uint64
	fieldIDElements      uint64
	fieldSourceElements  uint64
}

type finalTreeStatsItem struct {
	node  *Node
	entry stackEntry
	arena *nodeArena
}

func collectFinalTreeMaterializationStats(root *Node, lang *Language) finalTreeMaterializationStats {
	var stats finalTreeMaterializationStats
	if root == nil {
		return stats
	}
	var local [64]finalTreeStatsItem
	stack := local[:0]
	stack = append(stack, finalTreeStatsItem{node: root, arena: root.ownerArena})
	for len(stack) > 0 {
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if item.node != nil {
			collectFinalNodeStats(item.node, lang, &stats, &stack)
			continue
		}
		collectFinalEntryStats(item.entry, item.arena, lang, &stats, &stack)
	}
	return stats
}

func collectFinalNodeStats(n *Node, lang *Language, stats *finalTreeMaterializationStats, stack *[]finalTreeStatsItem) {
	if n == nil || stats == nil || stack == nil {
		return
	}
	stats.nodes++
	if childRange, ok := n.ownerArena.finalChildRange(n); ok {
		childCount := childRange.count()
		if childCount == 0 {
			stats.leafNodes++
			if _, ok := externalScannerCheckpointRefForNode(n); ok {
				stats.checkpointLeafNodes++
			}
			return
		}
		stats.parentNodes++
		if nodeVisibleInLanguage(n, lang) {
			stats.visibleParentNodes++
		} else {
			stats.hiddenParentNodes++
		}
		stats.childSlices++
		stats.childPointers += uint64(childCount)
		stats.unfieldedParentNodes++
		refs := childRange.refs(n.ownerArena)
		for i := childCount - 1; i >= 0; i-- {
			*stack = append(*stack, finalTreeStatsItem{entry: refs[i].stackEntry(), arena: n.ownerArena})
		}
		return
	}
	childCount := len(n.children)
	if childCount == 0 {
		stats.leafNodes++
		if _, ok := externalScannerCheckpointRefForNode(n); ok {
			stats.checkpointLeafNodes++
		}
		return
	}
	stats.parentNodes++
	if nodeVisibleInLanguage(n, lang) {
		stats.visibleParentNodes++
	} else {
		stats.hiddenParentNodes++
	}
	stats.childSlices++
	stats.childPointers += uint64(childCount)
	if len(n.fieldIDs) > 0 {
		stats.fieldIDElements += uint64(len(n.fieldIDs))
	}
	if len(n.fieldSources) > 0 {
		stats.fieldSourceElements += uint64(len(n.fieldSources))
	}
	if hasParentFieldMetadata(n.fieldIDs, n.fieldSources) {
		stats.fieldedParentNodes++
	} else {
		stats.unfieldedParentNodes++
	}
	for i := childCount - 1; i >= 0; i-- {
		child := n.children[i]
		var arena *nodeArena
		if child != nil {
			arena = child.ownerArena
		}
		*stack = append(*stack, finalTreeStatsItem{node: child, arena: arena})
	}
}

func collectFinalEntryStats(entry stackEntry, arena *nodeArena, lang *Language, stats *finalTreeMaterializationStats, stack *[]finalTreeStatsItem) {
	if stats == nil || stack == nil || !stackEntryHasNode(entry) {
		return
	}
	if node := stackEntryNode(entry); node != nil {
		collectFinalNodeStats(node, lang, stats, stack)
		return
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		stats.nodes++
		stats.leafNodes++
		if leaf.hasCheckpoint {
			stats.checkpointLeafNodes++
		}
		return
	}
	parent := stackEntryPendingParent(entry)
	if parent == nil {
		return
	}
	stats.nodes++
	childCount := parent.childEntryCount()
	if childCount == 0 {
		stats.leafNodes++
		return
	}
	stats.parentNodes++
	var symbolMeta []SymbolMetadata
	if lang != nil {
		symbolMeta = lang.SymbolMetadata
	}
	if symbolVisibleForPending(parent.symbol, symbolMeta) {
		stats.visibleParentNodes++
	} else {
		stats.hiddenParentNodes++
	}
	stats.childSlices++
	stats.childPointers += uint64(childCount)
	if parent.hasFieldEntries() || parent.hasDirectFieldEntries() {
		stats.fieldedParentNodes++
		stats.fieldIDElements += uint64(childCount)
		stats.fieldSourceElements += uint64(childCount)
	} else {
		stats.unfieldedParentNodes++
	}
	refs := parent.childRefs(arena)
	limit := childCount
	if limit > len(refs) {
		limit = len(refs)
	}
	for i := limit - 1; i >= 0; i-- {
		*stack = append(*stack, finalTreeStatsItem{entry: refs[i].stackEntry(), arena: arena})
	}
}

func nodeVisibleInLanguage(n *Node, lang *Language) bool {
	if n == nil || lang == nil {
		return true
	}
	idx := int(n.symbol)
	if idx < 0 || idx >= len(lang.SymbolMetadata) {
		return true
	}
	return lang.SymbolMetadata[idx].Visible
}

func (a *nodeArena) deferParentLinks(root *Node) {
	if a == nil || root == nil {
		return
	}
	a.parentLinkMu.Lock()
	a.deferredParentRoot = root
	a.parentLinksDeferred.Store(true)
	setNodeRootLink(root)
	a.parentLinkMu.Unlock()
}

// ensureParentLinks wires the arena's deferred parent links exactly once, under
// parentLinkMu, and is safe for concurrent callers. The atomic parentLinksDeferred
// flag pairs a release store (after wiring) with the lock-free Load fast path:
// once a caller observes the flag false, every parent-link field written during
// wiring is visible to it without holding the lock, so subsequent reads via
// nodeParentLink are race-free. This replaces the previous lazy per-path wiring
// (wireDeferredParentPathToNode), which wrote parent pointers outside the lock and
// raced with concurrent Parent()/sibling reads on a freshly parsed tree (the
// java/python/typescript/tsx deferred path).
func (a *nodeArena) ensureParentLinks() {
	if a == nil {
		return
	}
	if !a.parentLinksDeferred.Load() {
		return
	}
	a.parentLinkMu.Lock()
	root := a.deferredParentRoot
	if a.parentLinksDeferred.Load() && root != nil {
		wireParentLinksWithScratch(root, nil)
		a.deferredParentRoot = nil
		a.parentLinksDeferred.Store(false)
	}
	a.parentLinkMu.Unlock()
}

func (n *Node) ensureParentLinks() {
	if n == nil || n.ownerArena == nil {
		return
	}
	n.ownerArena.ensureParentLinks()
}

func (t *Tree) ensureParentLinks() {
	if t == nil || t.root == nil {
		return
	}
	t.root.ensureParentLinks()
}

func (t *Tree) ensureExternalScannerCheckpoints() {
	if t == nil || !t.externalScannerCheckpointsDeferred {
		return
	}
	t.externalScannerCheckpointsDeferred = false
	rebuildExternalScannerCheckpoints(t.root, t.language)
}

func (t *Tree) deferResultCompatibility() {
	if t == nil || t.root == nil || t.language == nil {
		return
	}
	t.resultCompatibilityPending = true
}

func (t *Tree) ensureResultCompatibility() {
	if t == nil || !t.resultCompatibilityPending {
		return
	}
	t.resultCompatibilityOnce.Do(func() {
		defer func() {
			t.resultCompatibilityPending = false
		}()
		if t.root == nil || t.language == nil {
			return
		}
		if !parsePhaseTimingEnabled() {
			normalizeResultCompatibility(t.root, t.source, &Parser{language: t.language})
			return
		}
		timing := &parseMaterializationTiming{}
		parser := &Parser{
			language:              t.language,
			materializationTiming: timing,
		}
		start := materializationTimingStart(timing)
		normalizeResultCompatibility(t.root, t.source, parser)
		timing.addResultCompatibility(start)
		t.parseRuntime.ResultCompatibilityNanos += timing.resultCompatibilityNanos
		parser.copyNormalizationStats(&t.parseRuntime)
	})
}

func newParentNode(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID, productionID uint16) *Node {
	var n *Node
	if arena == nil {
		n = &Node{}
	} else {
		n = arena.allocNode()
		n.ownerArena = arena
	}
	n.symbol = sym
	n.setNamed(named)
	n.children = children
	n.fieldIDs = fieldIDs
	n.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	n.productionID = productionID
	n.childIndex = -1
	populateParentNode(n, children)
	nodeInitEquivVersion(n)
	return n
}

// NewParentNode creates a non-terminal node with children.
// It sets parent pointers on all children and computes byte/point spans
// from the first and last children. If any child has an error, the parent
// is marked as having an error too.
func NewParentNode(sym Symbol, named bool, children []*Node, fieldIDs []FieldID, productionID uint16) *Node {
	return newParentNode(nil, sym, named, children, fieldIDs, productionID)
}

func newLeafNodeInArena(arena *nodeArena, sym Symbol, named bool, startByte, endByte uint32, startPoint, endPoint Point) *Node {
	if arena == nil {
		n := &Node{
			symbol:     sym,
			startByte:  startByte,
			endByte:    endByte,
			startPoint: startPoint,
			endPoint:   endPoint,
			childIndex: -1,
		}
		n.setNamed(named)
		return n
	}
	n := arena.allocNodeFast()
	n.symbol = sym
	n.setNamed(named)
	n.startByte = startByte
	n.endByte = endByte
	n.startPoint = startPoint
	n.endPoint = endPoint
	n.childIndex = -1
	n.ownerArena = arena
	arena.leafNodesConstructed++
	if arena.audit != nil {
		arena.audit.recordNodeAlloc(n, runtimeAuditNodeKindLeaf)
	}
	if internLeavesObserveEnabled {
		observeLeafIntern(arena, n)
	}
	return n
}

// observeLeafIntern records whether the just-allocated leaf node would
// have hit an interning table. This is Phase 2 of the node-interning
// initiative — measurement only, no behavior change. Called from the
// hot path only when GOT_PARSE_INTERN_LEAVES_OBSERVE=1 is set, so the
// default build pays nothing.
func observeLeafIntern(arena *nodeArena, n *Node) {
	if arena.internLeaves == nil {
		arena.internLeaves = newInternTable()
	}
	key := buildKey(n.symbol, n.productionID, n.flags, n.startByte, n.endByte, nil)
	if hit := arena.internLeaves.lookup(key, nil); hit == nil {
		arena.internLeaves.store(key, n)
	}
}

func newParentNodeInArena(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID, productionID uint16) *Node {
	return newParentNodeInArenaWithFieldSources(arena, sym, named, children, fieldIDs, nil, productionID)
}

func newParentNodeInArenaWithFieldSources(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID, fieldSources []uint8, productionID uint16) *Node {
	if arena == nil {
		return newParentNode(nil, sym, named, children, fieldIDs, productionID)
	}
	if perfCountersEnabled {
		perfRecordParentChildren(len(children))
	}
	n := arena.allocNodeFast()
	n.ownerArena = arena
	n.symbol = sym
	n.setNamed(named)
	n.children = children
	n.fieldIDs = fieldIDs
	if fieldSources != nil {
		n.fieldSources = fieldSources
	} else {
		n.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	}
	n.productionID = productionID
	n.childIndex = -1
	arena.parentNodesConstructed++
	if arena.breakdownEnabled {
		arena.recordParentNodeConstructedBreakdown(len(children), fieldIDs, n.fieldSources, fieldSources != nil, false, false)
	}
	populateParentNode(n, children)
	nodeInitEquivVersion(n)
	if arena.audit != nil {
		arena.audit.recordNodeAlloc(n, runtimeAuditNodeKindParent)
	}
	return n
}

func newParentNodeInArenaNoLinksWithFieldSources(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID, fieldSources []uint8, productionID uint16, trackChildErrors bool) *Node {
	if arena == nil {
		return newParentNode(nil, sym, named, children, fieldIDs, productionID)
	}
	if perfCountersEnabled {
		perfRecordParentChildren(len(children))
	}
	n := arena.allocNodeFast()
	n.ownerArena = arena
	n.symbol = sym
	n.setNamed(named)
	n.children = children
	n.fieldIDs = fieldIDs
	if fieldSources != nil {
		n.fieldSources = fieldSources
	} else {
		n.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	}
	n.productionID = productionID
	n.childIndex = -1
	arena.parentNodesConstructed++
	if arena.breakdownEnabled {
		arena.recordParentNodeConstructedBreakdown(len(children), fieldIDs, n.fieldSources, fieldSources != nil, true, trackChildErrors)
	}
	populateParentNodeNoLinks(n, children, trackChildErrors)
	nodeInitEquivVersion(n)
	if arena.audit != nil {
		arena.audit.recordNodeAlloc(n, runtimeAuditNodeKindParent)
	}
	return n
}

func newParentNodeInArenaWithFinalChildRefs(arena *nodeArena, sym Symbol, named bool, childRange pendingChildRange, productionID uint16, trackChildErrors bool) *Node {
	if arena == nil {
		return newParentNode(nil, sym, named, nil, nil, productionID)
	}
	childCount := childRange.count()
	if perfCountersEnabled {
		perfRecordParentChildren(childCount)
	}
	n := arena.allocNodeFast()
	n.ownerArena = arena
	n.symbol = sym
	n.setNamed(named)
	n.productionID = productionID
	n.childIndex = -1
	arena.parentNodesConstructed++
	if arena.breakdownEnabled {
		arena.recordParentNodeConstructedBreakdown(childCount, nil, nil, false, true, trackChildErrors)
	}
	arena.attachFinalChildRefs(n, childRange)
	nodeInitEquivVersion(n)
	if arena.audit != nil {
		arena.audit.recordNodeAlloc(n, runtimeAuditNodeKindParent)
	}
	return n
}

func (a *nodeArena) recordParentNodeConstructed(childCount int, fieldIDs []FieldID, fieldSources []uint8, fieldSourcesProvided bool, noLinks bool, trackChildErrors bool) {
	if a == nil {
		return
	}
	a.parentNodesConstructed++
	if !a.breakdownEnabled {
		return
	}
	a.recordParentNodeConstructedBreakdown(childCount, fieldIDs, fieldSources, fieldSourcesProvided, noLinks, trackChildErrors)
}

func (a *nodeArena) recordParentNodeConstructedBreakdown(childCount int, fieldIDs []FieldID, fieldSources []uint8, fieldSourcesProvided bool, noLinks bool, trackChildErrors bool) {
	switch childCount {
	case 0:
		a.parentConstructedChildLen0++
	case 1:
		a.parentConstructedChildLen1++
	case 2:
		a.parentConstructedChildLen2++
	case 3:
		a.parentConstructedChildLen3++
	default:
		a.parentConstructedChildLen4Plus++
	}
	if noLinks {
		a.parentConstructedNoLinks++
	} else {
		a.parentConstructedWithLinks++
	}
	if trackChildErrors {
		a.parentConstructedTrackErrors++
	}
	if fieldSourcesProvided {
		a.parentConstructedFieldSources++
	}
	if hasParentFieldMetadata(fieldIDs, fieldSources) {
		a.fieldedParentNodesConstructed++
		return
	}
	a.unfieldedParentNodesConstructed++
}

func (a *nodeArena) recordReductionParentConstructed(visible bool, childCount int, fieldIDs []FieldID, fieldSources []uint8) {
	if a == nil || !a.breakdownEnabled {
		return
	}
	fielded := hasParentFieldMetadata(fieldIDs, fieldSources)
	if visible {
		a.parentReductionVisible++
		a.parentReductionVisibleChildPointers += uint64(childCount)
		if fielded {
			a.parentReductionVisibleFielded++
		} else {
			a.parentReductionVisibleUnfielded++
		}
		switch childCount {
		case 0:
			a.parentReductionVisibleChildLen0++
		case 1:
			a.parentReductionVisibleChildLen1++
		case 2:
			a.parentReductionVisibleChildLen2++
		case 3:
			a.parentReductionVisibleChildLen3++
		default:
			a.parentReductionVisibleChildLen4Plus++
		}
		return
	}
	a.parentReductionInvisible++
	a.parentReductionInvisibleChildPtrs += uint64(childCount)
	if fielded {
		a.parentReductionInvisibleFielded++
	} else {
		a.parentReductionInvisibleUnfielded++
	}
	switch childCount {
	case 0:
		a.parentReductionInvisibleChildLen0++
	case 1:
		a.parentReductionInvisibleChildLen1++
	case 2:
		a.parentReductionInvisibleChildLen2++
	case 3:
		a.parentReductionInvisibleChildLen3++
	default:
		a.parentReductionInvisibleChildLen4P++
	}
}

func (a *nodeArena) recordReduceChildSliceFastGSS(n int) {
	if a == nil || !a.breakdownEnabled || n <= 0 {
		return
	}
	a.reduceChildSlicesFastGSS++
	a.reduceChildPointersFastGSS += uint64(n)
}

func (a *nodeArena) recordReduceChildSliceAllVisible(n int) {
	if a == nil || !a.breakdownEnabled || n <= 0 {
		return
	}
	a.reduceChildSlicesAllVisible++
	a.reduceChildPointersAllVisible += uint64(n)
}

func (a *nodeArena) recordReduceChildSliceNoAlias(n int) {
	if a == nil || !a.breakdownEnabled || n <= 0 {
		return
	}
	a.reduceChildSlicesNoAlias++
	a.reduceChildPointersNoAlias += uint64(n)
}

func (a *nodeArena) recordReduceChildSliceScratchGeneral(n int) {
	if a == nil || !a.breakdownEnabled || n <= 0 {
		return
	}
	a.reduceChildSlicesScratchGeneral++
	a.reduceChildPointersScratchGeneral += uint64(n)
}

func (a *nodeArena) recordReduceChildSliceScratchNoAlias(n int) {
	if a == nil || !a.breakdownEnabled || n <= 0 {
		return
	}
	a.reduceChildSlicesScratchNoAlias++
	a.reduceChildPointersScratchNoAlias += uint64(n)
}

func hasParentFieldMetadata(fieldIDs []FieldID, fieldSources []uint8) bool {
	for _, fid := range fieldIDs {
		if fid != 0 {
			return true
		}
	}
	for _, source := range fieldSources {
		if source != 0 {
			return true
		}
	}
	return false
}

// Tree holds a complete syntax tree along with its source text and language.
// Tree is safe for concurrent reads after construction. Edit and Release are
// not safe for concurrent use.
type Tree struct {
	root                               *Node
	source                             []byte
	sourceEncoding                     InputEncoding
	sourceUTF16                        []uint16
	utf16Map                           *utf16SourceMap
	language                           *Language
	edits                              []InputEdit  // pending edits applied to this tree
	lastEditedLeaf                     *Node        // deepest leaf overlapped by the most recent edit, when tracked
	arena                              *nodeArena   // primary arena that owns newly-built nodes
	borrowedArena                      []*nodeArena // arenas borrowed via subtree reuse
	parseRuntime                       ParseRuntime
	arenaBreakdown                     *ArenaBreakdown
	externalScannerCheckpointsDeferred bool
	resultCompatibilityPending         bool
	resultCompatibilityOnce            sync.Once
	released                           bool
}

const maxRetainedTreeEditCap = 8

var treePool = sync.Pool{
	New: func() any {
		return &Tree{}
	},
}

// NewTree creates a new Tree.
func NewTree(root *Node, source []byte, lang *Language) *Tree {
	return &Tree{
		root:           root,
		source:         source,
		sourceEncoding: InputEncodingUTF8,
		language:       lang,
	}
}

func newTreeWithArenas(root *Node, source []byte, lang *Language, arena *nodeArena, borrowed []*nodeArena) *Tree {
	return newTreeWithUniqueArenas(root, source, lang, arena, uniqueArenas(borrowed, arena))
}

func newTreeWithUniqueArenas(root *Node, source []byte, lang *Language, arena *nodeArena, borrowed []*nodeArena) *Tree {
	tree := treePool.Get().(*Tree)
	edits := reusableTreeEditScratch(tree.edits)
	deferExternalCheckpoints := root != nil && languageUsesExternalScannerCheckpoints(lang)
	*tree = Tree{
		root:                               root,
		source:                             source,
		sourceEncoding:                     InputEncodingUTF8,
		language:                           lang,
		edits:                              edits,
		arena:                              arena,
		borrowedArena:                      borrowed,
		externalScannerCheckpointsDeferred: deferExternalCheckpoints,
	}
	if !deferExternalCheckpoints {
		rebuildExternalScannerCheckpoints(root, lang)
	}
	return tree
}

func uniqueArenas(arenas []*nodeArena, exclude *nodeArena) []*nodeArena {
	if len(arenas) == 0 {
		return nil
	}
	out := make([]*nodeArena, 0, len(arenas))
	for _, a := range arenas {
		if a == nil {
			continue
		}
		if a == exclude {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing == a {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func reusableTreeEditScratch(edits []InputEdit) []InputEdit {
	if cap(edits) == 0 || cap(edits) > maxRetainedTreeEditCap {
		return nil
	}
	return edits[:0]
}

// Release decrements arena references held by this tree.
// After Release, the tree should be treated as invalid and not reused.
func (t *Tree) Release() {
	if t == nil || t.released {
		return
	}
	t.released = true
	edits := reusableTreeEditScratch(t.edits)
	t.lastEditedLeaf = nil
	for _, a := range t.borrowedArena {
		a.Release()
	}
	if len(t.borrowedArena) > 0 {
		clear(t.borrowedArena)
		t.borrowedArena = t.borrowedArena[:0]
	}
	if t.arena != nil {
		t.arena.Release()
		t.arena = nil
	}
	t.root = nil
	t.source = nil
	t.sourceEncoding = InputEncodingUTF8
	t.sourceUTF16 = nil
	t.utf16Map = nil
	t.language = nil
	t.edits = edits
	t.parseRuntime = ParseRuntime{}
	t.arenaBreakdown = nil
	treePool.Put(t)
}

// RootNode returns the tree's root node.
func (t *Tree) RootNode() *Node {
	t.ensureResultCompatibility()
	return t.root
}

// RootNodeWithOffset returns a copy of the root node with all spans shifted by
// the provided byte and point offsets.
//
// This mirrors tree-sitter C's root-node-with-offset behavior for callers that
// need to embed a parsed tree at a larger document offset.
func (t *Tree) RootNodeWithOffset(offsetBytes uint32, offsetExtent Point) *Node {
	if t == nil || t.root == nil {
		return nil
	}
	t.ensureResultCompatibility()
	if offsetBytes == 0 && offsetExtent == (Point{}) {
		return t.root
	}
	return cloneTreeNodesWithOffset(t.root, offsetBytes, offsetExtent)
}

// Source returns the original source text.
func (t *Tree) Source() []byte { return t.source }

// SourceEncoding returns the encoding used by the caller that produced this tree.
//
// For UTF-16 parses, Source still returns the parser's canonical UTF-8 copy.
// Use SourceUTF16 and UTF16RangeForNode when caller-facing UTF-16 coordinates
// are needed.
func (t *Tree) SourceEncoding() InputEncoding {
	if t == nil {
		return InputEncodingUTF8
	}
	return t.sourceEncoding
}

// SourceUTF16 returns the original UTF-16 source for trees produced by ParseUTF16.
// It returns nil for ordinary UTF-8 parses.
func (t *Tree) SourceUTF16() []uint16 {
	if t == nil || t.sourceEncoding != InputEncodingUTF16 {
		return nil
	}
	return t.sourceUTF16
}

// UTF16OffsetForByte converts a parser UTF-8 byte offset to a UTF-16 code-unit
// offset for trees produced by ParseUTF16.
func (t *Tree) UTF16OffsetForByte(offset uint32) (uint32, bool) {
	if t == nil || t.utf16Map == nil {
		return 0, false
	}
	return t.utf16Map.byteToUTF16Unit(offset)
}

// UTF8ByteForUTF16Offset converts a UTF-16 code-unit offset to the parser's
// canonical UTF-8 byte offset for trees produced by ParseUTF16.
func (t *Tree) UTF8ByteForUTF16Offset(offset uint32) (uint32, bool) {
	if t == nil || t.utf16Map == nil {
		return 0, false
	}
	return t.utf16Map.utf16UnitToByte(offset)
}

// UTF16PointForByte converts a parser UTF-8 byte offset to a UTF-16 point.
func (t *Tree) UTF16PointForByte(offset uint32) (Point, bool) {
	if t == nil || t.utf16Map == nil {
		return Point{}, false
	}
	return t.utf16Map.pointForByte(offset)
}

// UTF16RangeForNode returns a node range in UTF-16 code-unit coordinates.
func (t *Tree) UTF16RangeForNode(n *Node) (UTF16Range, bool) {
	if t == nil || t.utf16Map == nil {
		return UTF16Range{}, false
	}
	return t.utf16Map.rangeForNode(n)
}

// UTF16RangeForByteRange converts a canonical UTF-8 byte range into UTF-16
// code-unit coordinates.
func (t *Tree) UTF16RangeForByteRange(startByte, endByte uint32) (UTF16Range, bool) {
	if t == nil || t.utf16Map == nil {
		return UTF16Range{}, false
	}
	return t.utf16Map.rangeForByteRange(startByte, endByte)
}

// UTF16RangeForRange converts a canonical UTF-8 Range into UTF-16 code-unit
// coordinates.
func (t *Tree) UTF16RangeForRange(r Range) (UTF16Range, bool) {
	return t.UTF16RangeForByteRange(r.StartByte, r.EndByte)
}

func (t *Tree) descendantForUTF16Range(startCodeUnit, endCodeUnit uint32, namedOnly bool) *Node {
	if t == nil || t.utf16Map == nil || t.root == nil || endCodeUnit < startCodeUnit {
		return nil
	}
	startByte, ok := t.utf16Map.utf16UnitToByte(startCodeUnit)
	if !ok {
		return nil
	}
	endByte, ok := t.utf16Map.utf16UnitToByte(endCodeUnit)
	if !ok {
		return nil
	}
	return t.root.descendantForByteRange(startByte, endByte, namedOnly)
}

// DescendantForUTF16Range returns the smallest descendant that fully contains
// the given UTF-16 code-unit range, or nil when no such descendant exists.
func (t *Tree) DescendantForUTF16Range(startCodeUnit, endCodeUnit uint32) *Node {
	return t.descendantForUTF16Range(startCodeUnit, endCodeUnit, false)
}

// NamedDescendantForUTF16Range returns the smallest named descendant that fully
// contains the given UTF-16 code-unit range, or nil when no such descendant
// exists.
func (t *Tree) NamedDescendantForUTF16Range(startCodeUnit, endCodeUnit uint32) *Node {
	return t.descendantForUTF16Range(startCodeUnit, endCodeUnit, true)
}

// UTF16SourceForNode returns the original UTF-16 code units covered by n.
func (t *Tree) UTF16SourceForNode(n *Node) ([]uint16, bool) {
	rng, ok := t.UTF16RangeForNode(n)
	if !ok || rng.EndCodeUnit < rng.StartCodeUnit {
		return nil, false
	}
	source := t.SourceUTF16()
	start := int(rng.StartCodeUnit)
	end := int(rng.EndCodeUnit)
	if start > len(source) || end > len(source) {
		return nil, false
	}
	return source[start:end], true
}

// Language returns the language used to parse this tree.
func (t *Tree) Language() *Language { return t.language }

// WriteDOT writes a DOT graph representation of this tree to w.
func (t *Tree) WriteDOT(w io.Writer, lang *Language) error {
	if w == nil {
		return fmt.Errorf("tree: nil writer")
	}
	if t == nil || t.root == nil {
		_, err := io.WriteString(w, "digraph gotreesitter {\n}\n")
		return err
	}
	t.ensureResultCompatibility()

	type dotItem struct {
		node *Node
		id   int
	}

	if _, err := io.WriteString(w, "digraph gotreesitter {\n"); err != nil {
		return err
	}

	nextID := 1
	stack := []dotItem{{node: t.root, id: 0}}
	for len(stack) > 0 {
		last := len(stack) - 1
		item := stack[last]
		stack = stack[:last]
		n := item.node
		if n == nil {
			continue
		}

		label := fmt.Sprintf("%s [%d,%d)", n.Type(lang), n.StartByte(), n.EndByte())
		if _, err := fmt.Fprintf(w, "  n%d [label=%q];\n", item.id, label); err != nil {
			return err
		}

		childCount := nodeChildCountNoMaterialize(n)
		for i := 0; i < childCount; i++ {
			child := nodeChildAtForReason(n, i, materializeForParentAPI)
			if child == nil {
				continue
			}
			childID := nextID
			nextID++
			if _, err := fmt.Fprintf(w, "  n%d -> n%d;\n", item.id, childID); err != nil {
				return err
			}
			stack = append(stack, dotItem{node: child, id: childID})
		}
	}

	_, err := io.WriteString(w, "}\n")
	return err
}

// DOT returns a DOT graph representation of this tree.
func (t *Tree) DOT(lang *Language) string {
	var b strings.Builder
	_ = t.WriteDOT(&b, lang)
	return b.String()
}

// Copy returns an independent copy of this tree.
//
// The copied tree has distinct node objects, so subsequent Tree.Edit calls on
// either tree do not mutate the other's spans/dirty bits. Source bytes and
// language pointer are shared (read-only).
func (t *Tree) Copy() *Tree {
	if t == nil {
		return nil
	}
	t.ensureResultCompatibility()

	out := &Tree{
		source:         t.source,
		sourceEncoding: t.sourceEncoding,
		sourceUTF16:    t.sourceUTF16,
		utf16Map:       t.utf16Map,
		language:       t.language,
		parseRuntime:   t.parseRuntime,
	}
	if len(t.edits) > 0 {
		out.edits = make([]InputEdit, len(t.edits))
		copy(out.edits, t.edits)
	}
	if t.root == nil {
		return out
	}

	class := arenaClassIncremental
	if t.arena != nil {
		class = t.arena.class
	}
	arena := acquireNodeArena(class)
	out.root = cloneTreeNodesIntoArena(t.root, arena)
	out.arena = arena
	return out
}

func cloneTreeNodesIntoArena(root *Node, arena *nodeArena) *Node {
	if root == nil {
		return nil
	}
	if perfCountersEnabled {
		perfRecordCloneTreeCall()
	}
	if arena == nil {
		return cloneTreeNodesWithOffset(root, 0, Point{})
	}

	type clonePair struct {
		old *Node
		new *Node
	}

	cloneNode := func(src *Node) *Node {
		dst := arena.allocNodeFast()
		cloneNodeHeaderInto(dst, src, arena, nil)
		if perfCountersEnabled {
			perfRecordCloneTreePublicNode()
		}
		return dst
	}

	newRoot := cloneNode(root)
	stack := []clonePair{{old: root, new: newRoot}}
	for len(stack) > 0 {
		last := len(stack) - 1
		pair := stack[last]
		stack = stack[:last]

		oldNode := pair.old
		newNode := pair.new

		cloneNodeFieldMetadataInto(newNode, oldNode, arena)

		if cloneFinalChildRefsIntoArena(oldNode, newNode, arena, nil) {
			continue
		}
		if n := len(oldNode.children); n > 0 {
			children := arena.allocNodeSlice(n)
			newNode.children = children
			for i := 0; i < n; i++ {
				oldChild := oldNode.children[i]
				if oldChild == nil {
					continue
				}
				newChild := cloneNode(oldChild)
				newChild.parent = newNode
				newChild.childIndex = int32(i)
				children[i] = newChild
				stack = append(stack, clonePair{old: oldChild, new: newChild})
			}
		}
	}

	return newRoot
}

type cloneOffset struct {
	byteDelta uint32
	point     Point
	baseRow   uint32
}

func cloneTreeNodesWithOffset(root *Node, offsetBytes uint32, offsetExtent Point) *Node {
	if root == nil {
		return nil
	}
	if perfCountersEnabled {
		perfRecordCloneOffsetCall()
	}
	arena := newNodeArena(arenaClassIncremental)
	arena.finalChildRefs = true
	offset := &cloneOffset{
		byteDelta: offsetBytes,
		point:     offsetExtent,
		baseRow:   root.startPoint.Row,
	}

	type clonePair struct {
		old *Node
		new *Node
	}

	cloneNode := func(src *Node) *Node {
		dst := arena.allocNodeFast()
		cloneNodeHeaderInto(dst, src, arena, offset)
		if perfCountersEnabled {
			perfRecordCloneOffsetPublicNode()
		}
		return dst
	}

	newRoot := cloneNode(root)
	stack := []clonePair{{old: root, new: newRoot}}
	for len(stack) > 0 {
		last := len(stack) - 1
		pair := stack[last]
		stack = stack[:last]

		oldNode := pair.old
		newNode := pair.new

		cloneNodeFieldMetadataInto(newNode, oldNode, arena)

		if cloneFinalChildRefsIntoArena(oldNode, newNode, arena, offset) {
			continue
		}
		if n := len(oldNode.children); n > 0 {
			children := arena.allocNodeSlice(n)
			newNode.children = children
			for i := 0; i < n; i++ {
				oldChild := oldNode.children[i]
				if oldChild == nil {
					continue
				}
				newChild := cloneNode(oldChild)
				newChild.parent = newNode
				newChild.childIndex = int32(i)
				children[i] = newChild
				stack = append(stack, clonePair{old: oldChild, new: newChild})
			}
		}
	}

	return newRoot
}

func cloneNodeHeaderInto(dst, src *Node, arena *nodeArena, offset *cloneOffset) {
	*dst = *src
	if offset != nil {
		dst.startByte = addUint32Delta(src.startByte, int64(offset.byteDelta))
		dst.endByte = addUint32Delta(src.endByte, int64(offset.byteDelta))
		dst.startPoint = offset.offsetPoint(src.startPoint)
		dst.endPoint = offset.offsetPoint(src.endPoint)
	}
	dst.children = nil
	dst.fieldIDs = nil
	dst.fieldSources = nil
	dst.parent = nil
	dst.childIndex = -1
	dst.ownerArena = arena
	if cp, ok := externalScannerCheckpointRefForNode(src); ok {
		if cloned, ok := cloneExternalScannerCheckpointRef(src.ownerArena, arena, cp); ok && arena.setExternalScannerCheckpoint(dst, cloned) {
			arena.externalScannerCheckpointRecords++
		}
	}
}

func (o *cloneOffset) offsetPoint(p Point) Point {
	if o == nil {
		return p
	}
	originalRow := p.Row
	p.Row = addUint32Delta(p.Row, int64(o.point.Row))
	// When adding a multi-line prefix, only nodes on the original first row
	// of this tree receive the column offset. Rows after that keep columns.
	if o.point.Row == 0 || originalRow == o.baseRow {
		p.Column = addUint32Delta(p.Column, int64(o.point.Column))
	}
	return p
}

func cloneNodeFieldMetadataInto(dst, src *Node, arena *nodeArena) {
	if n := len(src.fieldIDs); n > 0 {
		fieldIDs := arena.allocFieldIDSlice(n)
		copy(fieldIDs, src.fieldIDs)
		dst.fieldIDs = fieldIDs
	}
	if n := len(src.fieldSources); n > 0 {
		fieldSources := arena.allocFieldSourceSlice(n)
		copy(fieldSources, src.fieldSources)
		dst.fieldSources = fieldSources
	}
}

type cloneMetricScope uint8

const (
	cloneMetricScopeNone cloneMetricScope = iota
	cloneMetricScopeTree
	cloneMetricScopeOffset
)

func cloneMetricScopeForOffset(offset *cloneOffset) cloneMetricScope {
	if offset != nil {
		return cloneMetricScopeOffset
	}
	return cloneMetricScopeTree
}

func cloneFinalChildRefsIntoArena(src, dst *Node, arena *nodeArena, offset *cloneOffset) bool {
	return cloneFinalChildRefsIntoArenaWithMetrics(src, dst, arena, offset, cloneMetricScopeForOffset(offset))
}

func cloneFinalChildRefsIntoArenaForMutation(src, dst *Node, arena *nodeArena) bool {
	return cloneFinalChildRefsIntoArenaWithMetrics(src, dst, arena, nil, cloneMetricScopeNone)
}

func cloneFinalChildRefsIntoArenaWithMetrics(src, dst *Node, arena *nodeArena, offset *cloneOffset, metrics cloneMetricScope) bool {
	if src == nil || dst == nil || arena == nil || src.ownerArena == nil {
		return false
	}
	childRange, ok := src.ownerArena.finalChildRange(src)
	if !ok {
		return false
	}
	count := childRange.count()
	if count == 0 {
		return false
	}
	srcRefs := childRange.refs(src.ownerArena)
	if len(srcRefs) < count {
		return false
	}
	if metrics == cloneMetricScopeTree && perfCountersEnabled {
		perfRecordCloneTreeFinalRefs(count)
		perfRecordCloneTreeChildRefs(count)
	}
	dstRange, dstRefs := arena.allocPendingChildEntries(count)
	for i := 0; i < count; i++ {
		entry := srcRefs[i].stackEntry()
		dstRefs[i] = newPendingChildEntry(cloneStackEntryIntoArena(src.ownerArena, arena, entry, offset, metrics))
	}
	arena.finalChildRefs = arena.finalChildRefs || src.ownerArena.finalChildRefs
	parentLink := dst.parent
	parentChildIndex := dst.childIndex
	arena.attachFinalChildRefs(dst, dstRange)
	if sidecar, ok := arena.finalChildSidecarForNode(dst); ok {
		sidecar.parent = parentLink
		sidecar.parentChildIndex = parentChildIndex
	}
	return true
}

func cloneStackEntryIntoArena(srcArena, dstArena *nodeArena, entry stackEntry, offset *cloneOffset, metrics cloneMetricScope) stackEntry {
	if node := stackEntryNode(entry); node != nil {
		cloned := cloneTreeNodesIntoArenaWithOffset(node, dstArena, offset)
		return newStackEntryNode(cloned.parseState, cloned)
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		cloned := dstArena.allocCompactFullLeaf()
		*cloned = *leaf
		if perfCountersEnabled {
			if metrics == cloneMetricScopeOffset {
				perfRecordCloneOffsetCompactCopy()
			} else if metrics == cloneMetricScopeTree {
				perfRecordCloneTreeCompactCopy()
			}
		}
		applyCloneOffsetToCompactFullLeaf(cloned, offset)
		if cloned.hasCheckpoint {
			cloned.checkpoint, _ = cloneExternalScannerCheckpointRef(srcArena, dstArena, leaf.checkpoint)
		}
		dstArena.compactFullLeafCreated++
		return newStackEntryCompactFullLeaf(cloned.parseState, cloned)
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		cloned := clonePendingParentIntoArena(srcArena, dstArena, parent, offset, metrics)
		if perfCountersEnabled {
			if metrics == cloneMetricScopeOffset {
				perfRecordCloneOffsetCompactCopy()
			} else if metrics == cloneMetricScopeTree {
				perfRecordCloneTreeCompactCopy()
			}
		}
		return newStackEntryPendingParent(cloned.parseState, cloned)
	}
	if noTree := stackEntryNoTreeNode(entry); noTree != nil {
		cloned := dstArena.allocNoTreeNode()
		*cloned = *noTree
		if perfCountersEnabled {
			if metrics == cloneMetricScopeOffset {
				perfRecordCloneOffsetCompactCopy()
			} else if metrics == cloneMetricScopeTree {
				perfRecordCloneTreeCompactCopy()
			}
		}
		applyCloneOffsetToNoTreeNode(cloned, offset)
		return newStackEntryNoTreeNode(cloned.parseState, cloned)
	}
	return stackEntry{}
}

func cloneTreeNodesIntoArenaWithOffset(root *Node, arena *nodeArena, offset *cloneOffset) *Node {
	if root == nil {
		return nil
	}
	if offset == nil {
		return cloneTreeNodesIntoArena(root, arena)
	}

	type clonePair struct {
		old *Node
		new *Node
	}

	cloneNode := func(src *Node) *Node {
		dst := arena.allocNodeFast()
		cloneNodeHeaderInto(dst, src, arena, offset)
		if perfCountersEnabled {
			perfRecordCloneOffsetPublicNode()
		}
		return dst
	}

	newRoot := cloneNode(root)
	stack := []clonePair{{old: root, new: newRoot}}
	for len(stack) > 0 {
		last := len(stack) - 1
		pair := stack[last]
		stack = stack[:last]

		oldNode := pair.old
		newNode := pair.new

		cloneNodeFieldMetadataInto(newNode, oldNode, arena)
		if cloneFinalChildRefsIntoArena(oldNode, newNode, arena, offset) {
			continue
		}
		if n := len(oldNode.children); n > 0 {
			children := arena.allocNodeSlice(n)
			newNode.children = children
			for i := 0; i < n; i++ {
				oldChild := oldNode.children[i]
				if oldChild == nil {
					continue
				}
				newChild := cloneNode(oldChild)
				newChild.parent = newNode
				newChild.childIndex = int32(i)
				children[i] = newChild
				stack = append(stack, clonePair{old: oldChild, new: newChild})
			}
		}
	}
	return newRoot
}

func clonePendingParentIntoArena(srcArena, dstArena *nodeArena, src *pendingParent, offset *cloneOffset, metrics cloneMetricScope) *pendingParent {
	childCount := src.childEntryCount()
	entrySlots := childCount
	if src.hasFieldEntries() {
		entrySlots = childCount * 2
	}
	dst := newPendingParentShellWithEntrySlotsInArena(dstArena, src.symbol, src.isNamed(), src.productionID, childCount, entrySlots, src.startByte, src.endByte, src.startPoint, src.endPoint, src.hasError())
	dst.noTreeNode = src.noTreeNode
	dst.startPoint = src.startPoint
	dst.endPoint = src.endPoint
	applyCloneOffsetToPendingParent(dst, offset)
	dst.setHasFieldEntries(src.hasFieldEntries())
	dst.setHasDirectFieldEntries(src.hasDirectFieldEntries())
	for i := 0; i < childCount; i++ {
		dst.setChildEntry(dstArena, i, cloneStackEntryIntoArena(srcArena, dstArena, src.childEntry(srcArena, i), offset, metrics))
		if src.hasFieldEntries() {
			fid, source := src.childFieldEntry(srcArena, i)
			dst.setChildFieldEntry(dstArena, i, fid, source)
		}
	}
	return dst
}

func applyCloneOffsetToNoTreeNode(n *noTreeNode, offset *cloneOffset) {
	if n == nil || offset == nil {
		return
	}
	n.startByte = addUint32Delta(n.startByte, int64(offset.byteDelta))
	n.endByte = addUint32Delta(n.endByte, int64(offset.byteDelta))
	if perfCountersEnabled {
		perfRecordCloneOffsetShifted()
	}
}

func applyCloneOffsetToCompactFullLeaf(n *compactFullLeaf, offset *cloneOffset) {
	if n == nil || offset == nil {
		return
	}
	applyCloneOffsetToNoTreeNode(&n.noTreeNode, offset)
	n.startPoint = offset.offsetPoint(n.startPoint)
	n.endPoint = offset.offsetPoint(n.endPoint)
}

func applyCloneOffsetToPendingParent(n *pendingParent, offset *cloneOffset) {
	if n == nil || offset == nil {
		return
	}
	applyCloneOffsetToNoTreeNode(&n.noTreeNode, offset)
	n.startPoint = offset.offsetPoint(n.startPoint)
	n.endPoint = offset.offsetPoint(n.endPoint)
}

func cloneExternalScannerCheckpointRef(srcArena, dstArena *nodeArena, cp externalScannerCheckpointRef) (externalScannerCheckpointRef, bool) {
	if srcArena == nil || dstArena == nil || (cp.start.len == 0 && cp.end.len == 0) {
		return externalScannerCheckpointRef{}, false
	}
	start := srcArena.externalScannerSnapshotBytes(cp.start)
	end := srcArena.externalScannerSnapshotBytes(cp.end)
	if len(start) == 0 && len(end) == 0 {
		return externalScannerCheckpointRef{}, false
	}
	startRef := dstArena.copyExternalScannerSnapshotRef(start)
	endRef := startRef
	if !equalBytesForCheckpoint(start, end) {
		endRef = dstArena.copyExternalScannerSnapshotRef(end)
	}
	return externalScannerCheckpointRef{start: startRef, end: endRef}, true
}

func equalBytesForCheckpoint(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ParseStopReason reports why parsing terminated.
func (t *Tree) ParseStopReason() ParseStopReason {
	if t == nil {
		return ParseStopNone
	}
	if t.parseRuntime.StopReason == "" {
		return ParseStopNone
	}
	return t.parseRuntime.StopReason
}

// ParseStoppedEarly reports whether parsing hit an early-stop condition.
func (t *Tree) ParseStoppedEarly() bool {
	switch t.ParseStopReason() {
	case ParseStopIterationLimit, ParseStopStackDepthLimit, ParseStopNodeLimit, ParseStopMemoryBudget, ParseStopTokenSourceEOF, ParseStopTimeout, ParseStopCancelled:
		return true
	default:
		return false
	}
}

// ParseRuntime returns parser-loop diagnostics captured when this tree was built.
func (t *Tree) ParseRuntime() ParseRuntime {
	if t == nil {
		return ParseRuntime{StopReason: ParseStopNone}
	}
	out := t.parseRuntime
	if arena := t.arena; arena != nil {
		out.FinalChildRefParents = arena.finalChildRefParents
		out.FinalChildRefs = arena.finalChildRefsCreated
		out.FinalChildRefMaterializedParents = arena.finalChildRefsMaterializedParents
		out.FinalChildRefMaterializedChildren = arena.finalChildRefsMaterializedChildren
		out.FinalChildRefSingleChildAccesses = arena.finalChildRefsSingleChildAccesses
		out.FinalChildRefSingleChildMaterializedChildren = arena.finalChildRefsSingleChildMaterializedChildren
	}
	if out.StopReason == "" {
		out.StopReason = ParseStopNone
	}
	return out
}

// ArenaBreakdown returns optional arena/materialization attribution captured
// when EnableArenaBreakdown(true) was set before parsing.
func (t *Tree) ArenaBreakdown() (ArenaBreakdown, bool) {
	if t == nil || t.arenaBreakdown == nil {
		return ArenaBreakdown{}, false
	}
	return *t.arenaBreakdown, true
}

func (t *Tree) setParseRuntime(rt ParseRuntime) {
	if t == nil {
		return
	}
	if rt.StopReason == "" {
		rt.StopReason = ParseStopNone
	}
	t.parseRuntime = rt
}

func (t *Tree) setArenaBreakdown(breakdown *ArenaBreakdown) {
	if t == nil {
		return
	}
	t.arenaBreakdown = breakdown
}

// InputEdit describes a single edit to the source text. It tells the parser
// what byte range was replaced and what the new range looks like, so the
// incremental parser can skip unchanged subtrees.
type InputEdit struct {
	StartByte   uint32
	OldEndByte  uint32
	NewEndByte  uint32
	StartPoint  Point
	OldEndPoint Point
	NewEndPoint Point
}

// InputEditForUTF16 converts a UTF-16 code-unit edit into the parser's internal
// UTF-8 byte-coordinate edit. The tree must have been produced by ParseUTF16.
func (t *Tree) InputEditForUTF16(edit UTF16Edit, newSource []uint16) (InputEdit, bool) {
	if t == nil || t.utf16Map == nil {
		return InputEdit{}, false
	}
	if edit.OldEndCodeUnit < edit.StartCodeUnit || edit.NewEndCodeUnit < edit.StartCodeUnit {
		return InputEdit{}, false
	}
	if !utf16Boundary(newSource, edit.StartCodeUnit) || !utf16Boundary(newSource, edit.NewEndCodeUnit) {
		return InputEdit{}, false
	}

	startByte, ok := t.utf16Map.utf16UnitToByte(edit.StartCodeUnit)
	if !ok {
		return InputEdit{}, false
	}
	oldEndByte, ok := t.utf16Map.utf16UnitToByte(edit.OldEndCodeUnit)
	if !ok {
		return InputEdit{}, false
	}
	replacementByteLen, replacementPoint := measureUTF16AsUTF8(newSource[edit.StartCodeUnit:edit.NewEndCodeUnit])
	if replacementByteLen > ^uint32(0)-startByte {
		return InputEdit{}, false
	}
	newEndByte := startByte + replacementByteLen
	startPoint, ok := t.utf16Map.pointForUTF8Byte(startByte)
	if !ok {
		return InputEdit{}, false
	}
	oldEndPoint, ok := t.utf16Map.pointForUTF8Byte(oldEndByte)
	if !ok {
		return InputEdit{}, false
	}
	newEndPoint := addPointDelta(startPoint, replacementPoint)
	return InputEdit{
		StartByte:   startByte,
		OldEndByte:  oldEndByte,
		NewEndByte:  newEndByte,
		StartPoint:  startPoint,
		OldEndPoint: oldEndPoint,
		NewEndPoint: newEndPoint,
	}, true
}

// EditUTF16 records a UTF-16 code-unit edit on a UTF-16 tree.
//
// newSource is the full source after the edit; it is used to derive the
// internal UTF-8 endpoint for NewEndCodeUnit.
func (t *Tree) EditUTF16(edit UTF16Edit, newSource []uint16) bool {
	inputEdit, ok := t.InputEditForUTF16(edit, newSource)
	if !ok {
		return false
	}
	t.Edit(inputEdit)
	return true
}

// Edit adjusts this node's byte/point span for a source edit.
//
// If the node belongs to a larger tree, the edit is applied from the
// containing root so sibling and ancestor spans remain consistent.
// Unlike Tree.Edit, this method does not record edit history on a Tree.
func (n *Node) Edit(edit InputEdit) {
	if n == nil {
		return
	}
	if perfCountersEnabled {
		perfRecordNodeEditCall()
	}
	if inputEditIsNoop(edit) {
		if perfCountersEnabled {
			perfRecordNodeEditNoopCall()
		}
		return
	}
	if root := nodeEditRoot(n); root != nil {
		editNode(root, edit)
	}
}

func inputEditIsNoop(edit InputEdit) bool {
	return edit.StartByte == edit.OldEndByte &&
		edit.OldEndByte == edit.NewEndByte &&
		edit.StartPoint == edit.OldEndPoint &&
		edit.OldEndPoint == edit.NewEndPoint
}

// Edit records an edit on this tree. Call this before ParseIncremental to
// inform the parser which regions changed. The edit adjusts byte offsets
// and marks overlapping nodes as dirty so the incremental parser knows
// what to re-parse.
func (t *Tree) Edit(edit InputEdit) {
	t.ensureResultCompatibility()
	if perfCountersEnabled {
		perfRecordNodeEditCall()
		if inputEditIsNoop(edit) {
			perfRecordNodeEditNoopCall()
		}
	}
	t.edits = append(t.edits, edit)
	t.lastEditedLeaf = nil
	if t.root != nil {
		byteDelta := int64(edit.NewEndByte) - int64(edit.OldEndByte)
		rowDelta := int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row)
		colDelta := int64(edit.NewEndPoint.Column) - int64(edit.OldEndPoint.Column)
		hasTailShift := byteDelta != 0 || edit.NewEndPoint != edit.OldEndPoint
		var shiftScratch []*Node
		editNodeWithDelta(t.root, edit, byteDelta, rowDelta, colDelta, hasTailShift, &shiftScratch, &t.lastEditedLeaf)
	}
}

// Edits returns the pending edits recorded on this tree.
func (t *Tree) Edits() []InputEdit { return t.edits }

// ChangedRanges converts this tree's recorded edits into changed source ranges.
// Overlapping ranges are coalesced.
func (t *Tree) ChangedRanges() []Range {
	if t == nil || len(t.edits) == 0 {
		return nil
	}
	ranges := make([]Range, 0, len(t.edits))
	for _, e := range t.edits {
		ranges = append(ranges, Range{
			StartByte:  e.StartByte,
			EndByte:    e.NewEndByte,
			StartPoint: e.StartPoint,
			EndPoint:   e.NewEndPoint,
		})
	}
	return coalesceRanges(ranges)
}

func rangesOverlapOrTouch(a, b Range) bool {
	return !(a.EndByte < b.StartByte || b.EndByte < a.StartByte)
}

func coalesceRanges(in []Range) []Range {
	if len(in) <= 1 {
		return in
	}
	sort.Slice(in, func(i, j int) bool {
		if in[i].StartByte != in[j].StartByte {
			return in[i].StartByte < in[j].StartByte
		}
		return in[i].EndByte < in[j].EndByte
	})
	out := make([]Range, 0, len(in))
	current := in[0]
	for i := 1; i < len(in); i++ {
		r := in[i]
		if rangesOverlapOrTouch(current, r) {
			if r.StartByte < current.StartByte {
				current.StartByte = r.StartByte
				current.StartPoint = r.StartPoint
			}
			if r.EndByte > current.EndByte {
				current.EndByte = r.EndByte
				current.EndPoint = r.EndPoint
			}
			continue
		}
		out = append(out, current)
		current = r
	}
	out = append(out, current)
	return out
}

// editNode recursively adjusts a node's byte/point spans for an edit and
// marks nodes that overlap the edited region as dirty.
func editNode(n *Node, edit InputEdit) {
	byteDelta := int64(edit.NewEndByte) - int64(edit.OldEndByte)
	rowDelta := int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row)
	colDelta := int64(edit.NewEndPoint.Column) - int64(edit.OldEndPoint.Column)
	hasTailShift := byteDelta != 0 || edit.NewEndPoint != edit.OldEndPoint
	var shiftScratch []*Node
	editNodeWithDelta(n, edit, byteDelta, rowDelta, colDelta, hasTailShift, &shiftScratch, nil)
}

func addUint32Delta(value uint32, delta int64) uint32 {
	next := int64(value) + delta
	if next < 0 {
		return 0
	}
	if next > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(next)
}

func editNodeWithDelta(n *Node, edit InputEdit, byteDelta, rowDelta, colDelta int64, hasTailShift bool, shiftScratch *[]*Node, leafHint **Node) {
	// If the node ends before the edit starts, it's completely unaffected.
	if n.endByte <= edit.StartByte {
		return
	}

	// If the node starts after the old edit end, shift its offsets.
	if n.startByte >= edit.OldEndByte {
		if !hasTailShift {
			return
		}
		n.startByte = addUint32Delta(n.startByte, byteDelta)
		n.endByte = addUint32Delta(n.endByte, byteDelta)
		// Shift points approximately (row stays, col shifts if same row).
		if n.startPoint.Row == edit.OldEndPoint.Row {
			n.startPoint.Row = addUint32Delta(n.startPoint.Row, rowDelta)
			if rowDelta == 0 {
				n.startPoint.Column = addUint32Delta(n.startPoint.Column, colDelta)
			}
		}
		if n.endPoint.Row == edit.OldEndPoint.Row {
			n.endPoint.Row = addUint32Delta(n.endPoint.Row, rowDelta)
			if rowDelta == 0 {
				n.endPoint.Column = addUint32Delta(n.endPoint.Column, colDelta)
			}
		}
		shiftNodeChildrenAfterEdit(n, edit, byteDelta, rowDelta, colDelta, shiftScratch)
		return
	}

	// The node overlaps the edit — mark it dirty and adjust its end.
	n.setDirty(true)
	if perfCountersEnabled {
		perfRecordNodeEditMarked()
	}
	if n.endByte <= edit.OldEndByte {
		// Node is fully within the edited region.
		n.endByte = edit.NewEndByte
		n.endPoint = edit.NewEndPoint
	} else {
		// Node extends past the edit — adjust end.
		n.endByte = addUint32Delta(n.endByte, byteDelta)
	}

	// Recurse only into children that can be affected.
	descended := false
	childCount := nodeChildCountNoMaterialize(n)
	if !nodeHasFinalChildRefs(n) {
		for _, c := range n.children {
			if c.endByte <= edit.StartByte {
				continue
			}
			if c.startByte >= edit.OldEndByte {
				if !hasTailShift {
					break
				}
				shiftSubtreeNodeAfterEdit(c, edit, byteDelta, rowDelta, colDelta, shiftScratch)
				continue
			}
			descended = true
			editNodeWithDelta(c, edit, byteDelta, rowDelta, colDelta, hasTailShift, shiftScratch, leafHint)
		}
	} else {
		for i := 0; i < childCount; i++ {
			entry, ok := nodeChildEntryAtNoMaterialize(n, i)
			if ok && perfCountersEnabled {
				perfRecordNodeEditCompactRef()
			}
			if !ok || stackEntryNodeEndByte(entry) <= edit.StartByte {
				continue
			}
			if stackEntryNodeStartByte(entry) >= edit.OldEndByte {
				if !hasTailShift {
					break
				}
				shiftStackEntrySubtreeAfterEdit(n.ownerArena, entry, edit, byteDelta, rowDelta, colDelta)
				continue
			}
			descended = true
			editStackEntryWithDelta(n.ownerArena, entry, edit, byteDelta, rowDelta, colDelta, hasTailShift, shiftScratch, leafHint)
		}
	}
	if leafHint != nil && !descended && childCount == 0 {
		*leafHint = n
	}
}

func editStackEntryWithDelta(arena *nodeArena, entry stackEntry, edit InputEdit, byteDelta, rowDelta, colDelta int64, hasTailShift bool, shiftScratch *[]*Node, leafHint **Node) {
	if node := stackEntryNode(entry); node != nil {
		editNodeWithDelta(node, edit, byteDelta, rowDelta, colDelta, hasTailShift, shiftScratch, leafHint)
		return
	}
	if !stackEntryHasNode(entry) || stackEntryNodeEndByte(entry) <= edit.StartByte {
		return
	}
	if stackEntryNodeStartByte(entry) >= edit.OldEndByte {
		if hasTailShift {
			shiftStackEntrySubtreeAfterEdit(arena, entry, edit, byteDelta, rowDelta, colDelta)
		}
		return
	}

	setStackEntryDirty(entry, true)
	if perfCountersEnabled {
		perfRecordNodeEditMarked()
	}
	if stackEntryNodeEndByte(entry) <= edit.OldEndByte {
		setStackEntryEnd(entry, edit.NewEndByte, edit.NewEndPoint)
	} else {
		setStackEntryEndByte(entry, addUint32Delta(stackEntryNodeEndByte(entry), byteDelta))
	}

	parent := stackEntryPendingParent(entry)
	if parent == nil {
		return
	}
	childCount := parent.childEntryCount()
	for i := 0; i < childCount; i++ {
		child := parent.childEntry(arena, i)
		if !stackEntryHasNode(child) || stackEntryNodeEndByte(child) <= edit.StartByte {
			continue
		}
		if stackEntryNodeStartByte(child) >= edit.OldEndByte {
			if !hasTailShift {
				break
			}
			shiftStackEntrySubtreeAfterEdit(arena, child, edit, byteDelta, rowDelta, colDelta)
			continue
		}
		if perfCountersEnabled {
			perfRecordNodeEditCompactRef()
		}
		editStackEntryWithDelta(arena, child, edit, byteDelta, rowDelta, colDelta, hasTailShift, shiftScratch, leafHint)
	}
}

func setStackEntryDirty(entry stackEntry, dirty bool) {
	if node := stackEntryNode(entry); node != nil {
		node.setDirty(dirty)
		return
	}
	if node := stackEntryNoTreeNode(entry); node != nil {
		node.setDirty(dirty)
		return
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		leaf.setDirty(dirty)
		return
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		parent.setDirty(dirty)
	}
}

func setStackEntryEnd(entry stackEntry, endByte uint32, endPoint Point) {
	if node := stackEntryNode(entry); node != nil {
		node.endByte = endByte
		node.endPoint = endPoint
		return
	}
	if node := stackEntryNoTreeNode(entry); node != nil {
		node.endByte = endByte
		return
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		leaf.endByte = endByte
		leaf.endPoint = endPoint
		return
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		parent.endByte = endByte
		parent.endPoint = endPoint
	}
}

func setStackEntryEndByte(entry stackEntry, endByte uint32) {
	if node := stackEntryNode(entry); node != nil {
		node.endByte = endByte
		return
	}
	if node := stackEntryNoTreeNode(entry); node != nil {
		node.endByte = endByte
		return
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		leaf.endByte = endByte
		return
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		parent.endByte = endByte
	}
}

func shiftNodeChildrenAfterEdit(parent *Node, edit InputEdit, byteDelta, rowDelta, colDelta int64, shiftScratch *[]*Node) {
	childCount := nodeChildCountNoMaterialize(parent)
	if childCount == 0 {
		return
	}
	if !nodeHasFinalChildRefs(parent) {
		shiftSubtreeAfterEdit(parent.children, edit, byteDelta, rowDelta, colDelta, shiftScratch)
		return
	}
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(parent, i)
		if ok && perfCountersEnabled {
			perfRecordNodeEditCompactRef()
		}
		if !ok {
			continue
		}
		shiftStackEntrySubtreeAfterEdit(parent.ownerArena, entry, edit, byteDelta, rowDelta, colDelta)
	}
}

func shiftSubtreeAfterEdit(roots []*Node, edit InputEdit, byteDelta, rowDelta, colDelta int64, shiftScratch *[]*Node) {
	if len(roots) == 0 {
		return
	}

	var stack [](*Node)
	if shiftScratch != nil {
		stack = (*shiftScratch)[:0]
	}
	stack = append(stack, roots...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		n.startByte = addUint32Delta(n.startByte, byteDelta)
		n.endByte = addUint32Delta(n.endByte, byteDelta)

		if n.startPoint.Row == edit.OldEndPoint.Row {
			n.startPoint.Row = addUint32Delta(n.startPoint.Row, rowDelta)
			if rowDelta == 0 {
				n.startPoint.Column = addUint32Delta(n.startPoint.Column, colDelta)
			}
		}
		if n.endPoint.Row == edit.OldEndPoint.Row {
			n.endPoint.Row = addUint32Delta(n.endPoint.Row, rowDelta)
			if rowDelta == 0 {
				n.endPoint.Column = addUint32Delta(n.endPoint.Column, colDelta)
			}
		}

		if !nodeHasFinalChildRefs(n) {
			stack = append(stack, n.children...)
		} else {
			childCount := nodeChildCountNoMaterialize(n)
			for i := 0; i < childCount; i++ {
				entry, ok := nodeChildEntryAtNoMaterialize(n, i)
				if ok && perfCountersEnabled {
					perfRecordNodeEditCompactRef()
				}
				if !ok {
					continue
				}
				shiftStackEntrySubtreeAfterEdit(n.ownerArena, entry, edit, byteDelta, rowDelta, colDelta)
			}
		}
	}
	if shiftScratch != nil {
		*shiftScratch = stack[:0]
	}
}

func shiftSubtreeNodeAfterEdit(root *Node, edit InputEdit, byteDelta, rowDelta, colDelta int64, shiftScratch *[]*Node) {
	if root == nil {
		return
	}
	var roots [1]*Node
	roots[0] = root
	shiftSubtreeAfterEdit(roots[:], edit, byteDelta, rowDelta, colDelta, shiftScratch)
}

func shiftStackEntrySubtreeAfterEdit(arena *nodeArena, entry stackEntry, edit InputEdit, byteDelta, rowDelta, colDelta int64) {
	if node := stackEntryNode(entry); node != nil {
		shiftSubtreeNodeAfterEdit(node, edit, byteDelta, rowDelta, colDelta, nil)
		return
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		shiftCompactFullLeafAfterEdit(leaf, edit, byteDelta, rowDelta, colDelta)
		return
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		shiftPendingParentAfterEdit(arena, parent, edit, byteDelta, rowDelta, colDelta)
		return
	}
	if noTree := stackEntryNoTreeNode(entry); noTree != nil {
		shiftNoTreeNodeAfterEdit(noTree, byteDelta)
	}
}

func shiftNoTreeNodeAfterEdit(n *noTreeNode, byteDelta int64) {
	if n == nil {
		return
	}
	n.startByte = addUint32Delta(n.startByte, byteDelta)
	n.endByte = addUint32Delta(n.endByte, byteDelta)
	if perfCountersEnabled {
		perfRecordNodeEditShifted()
	}
}

func shiftCompactFullLeafAfterEdit(n *compactFullLeaf, edit InputEdit, byteDelta, rowDelta, colDelta int64) {
	if n == nil {
		return
	}
	shiftNoTreeNodeAfterEdit(&n.noTreeNode, byteDelta)
	n.startPoint = shiftPointAfterEdit(n.startPoint, edit, rowDelta, colDelta)
	n.endPoint = shiftPointAfterEdit(n.endPoint, edit, rowDelta, colDelta)
}

func shiftPendingParentAfterEdit(arena *nodeArena, n *pendingParent, edit InputEdit, byteDelta, rowDelta, colDelta int64) {
	if n == nil {
		return
	}
	shiftNoTreeNodeAfterEdit(&n.noTreeNode, byteDelta)
	n.startPoint = shiftPointAfterEdit(n.startPoint, edit, rowDelta, colDelta)
	n.endPoint = shiftPointAfterEdit(n.endPoint, edit, rowDelta, colDelta)
	childCount := n.childEntryCount()
	for i := 0; i < childCount; i++ {
		child := n.childEntry(arena, i)
		if !stackEntryHasNode(child) {
			continue
		}
		if perfCountersEnabled {
			perfRecordNodeEditCompactRef()
		}
		shiftStackEntrySubtreeAfterEdit(arena, child, edit, byteDelta, rowDelta, colDelta)
	}
}

func shiftPointAfterEdit(p Point, edit InputEdit, rowDelta, colDelta int64) Point {
	if p.Row != edit.OldEndPoint.Row {
		return p
	}
	p.Row = addUint32Delta(p.Row, rowDelta)
	if rowDelta == 0 {
		p.Column = addUint32Delta(p.Column, colDelta)
	}
	return p
}

// DiffChangedRanges compares two syntax trees and returns the minimal
// ranges where syntactic structure differs. The old tree should have been
// edited (via Tree.Edit) to match the new tree's source positions before
// reparsing.
//
// This is equivalent to C tree-sitter's ts_tree_get_changed_ranges().
func DiffChangedRanges(oldTree, newTree *Tree) []Range {
	if oldTree == nil || newTree == nil {
		return nil
	}
	oldRoot := oldTree.RootNode()
	newRoot := newTree.RootNode()
	if oldRoot == nil || newRoot == nil {
		return nil
	}

	var ranges []Range
	diffNodes(oldRoot, newRoot, &ranges)
	return coalesceRanges(ranges)
}

// diffNodes recursively compares old and new tree nodes, appending changed
// ranges when structural differences are found.
func diffNodes(oldNode, newNode *Node, ranges *[]Range) {
	// If both nodes are structurally identical, nothing changed.
	if nodesStructurallyEqual(oldNode, newNode) {
		return
	}

	// If they differ at the symbol level or child count, the entire range is changed.
	if oldNode.Symbol() != newNode.Symbol() ||
		oldNode.ChildCount() != newNode.ChildCount() {
		addChangedRange(oldNode, newNode, ranges)
		return
	}

	// Leaf nodes (no children) that are not structurally equal: they differ in
	// byte range or one of them has been marked dirty. Report the range.
	if oldNode.ChildCount() == 0 {
		addChangedRange(oldNode, newNode, ranges)
		return
	}

	// Same symbol and child count — recurse into children.
	for i := 0; i < oldNode.ChildCount(); i++ {
		oldChild := oldNode.Child(i)
		newChild := newNode.Child(i)
		diffNodes(oldChild, newChild, ranges)
	}
}

// nodesStructurallyEqual reports whether two nodes are structurally identical
// and can be skipped during diff. Two nodes are equal if they have the same
// symbol, the same byte range, the same child count, and neither has been
// marked as changed by Tree.Edit.
func nodesStructurallyEqual(a, b *Node) bool {
	if a.Symbol() != b.Symbol() {
		return false
	}
	if a.StartByte() != b.StartByte() || a.EndByte() != b.EndByte() {
		return false
	}
	if a.ChildCount() != b.ChildCount() {
		return false
	}
	// Fast path: if neither node has changes, they're equal.
	if !a.HasChanges() && !b.HasChanges() {
		return true
	}
	return false
}

// addChangedRange records a changed range covering both the old and new node spans.
func addChangedRange(oldNode, newNode *Node, ranges *[]Range) {
	startByte := min(oldNode.StartByte(), newNode.StartByte())
	endByte := max(oldNode.EndByte(), newNode.EndByte())
	startPoint := oldNode.StartPoint()
	endPoint := newNode.EndPoint()
	if newNode.StartByte() < oldNode.StartByte() {
		startPoint = newNode.StartPoint()
	}
	if oldNode.EndByte() > newNode.EndByte() {
		endPoint = oldNode.EndPoint()
	}
	*ranges = append(*ranges, Range{
		StartByte:  startByte,
		EndByte:    endByte,
		StartPoint: startPoint,
		EndPoint:   endPoint,
	})
}
