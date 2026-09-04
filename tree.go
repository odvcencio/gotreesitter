package gotreesitter

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
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
	children          []*Node
	fieldMetadata     *nodeFieldMetadata
	parent            *Node
	ownerArena        *nodeArena
	startPoint        Point
	endPoint          Point
	startByte         uint32
	endByte           uint32
	parseState        StateID // parser state after this node was pushed
	preGotoState      StateID // parser state before goto (state exposed after popping children)
	equivVersion      uint32
	dynamicPrecedence int32
	childIndex        int32
	rawShape          rawShapeRef
	symbol            Symbol
	productionID      uint16
	flags             nodeFlags
	errorRankCache    uint8 // current parse arena only; 0 = unknown, rank + 1
	subtreeHeight     uint8 // forest dedup tie-break cache (0 = uncomputed); see nodeCachedHeight
}

// nodeFlags is uint16, not uint8: Node has exactly one byte of trailing
// padding under the 104-byte layout budget (TestNodeLayoutSizeBudget), and
// the original 8 flag bits below were already fully packed. Widening by one
// byte spends that padding on the fragileLeft/fragileRight bits without
// changing Node's size. nodeFlags is never serialized/blob-encoded (grep for
// "nodeFlags" before adding more bits, or before touching any blob-encode
// path) -- fragility in particular is runtime-only and must never be
// persisted, since it is recomputed fresh every parse.
type nodeFlags uint16

const (
	nodeFlagNamed nodeFlags = 1 << iota
	nodeFlagExtra
	nodeFlagMissing
	nodeFlagHasError
	nodeFlagDirty
	// nodeFlagFieldIDCacheComputed + nodeFlagFieldIDCacheHasFieldIDs memoize
	// hiddenTreeHasFieldIDs: a materialized subtree's field-ID presence is an
	// immutable property once built, but it was recomputed by a full subtree
	// walk at every reduce-time call site (O(n^2)-ish on deeply nested grammars
	// like scss). Fresh arena nodes start with flags=0 (cache uncomputed); the
	// bit is set lazily on first query. Only ever read/written during reduce, on
	// already-built immutable child subtrees.
	nodeFlagFieldIDCacheComputed
	nodeFlagFieldIDCacheHasFieldIDs
	nodeFlagExternalScannerToken
	// nodeFlagLexerSkippedPrefixAtSourceStart records authenticated DFA skip
	// provenance on the retained leaf that starts after source byte zero.
	nodeFlagLexerSkippedPrefixAtSourceStart
	// nodeFlagFragileLeft / nodeFlagFragileRight mirror C tree-sitter's
	// Subtree.fragile_left / fragile_right (subtree.h): a subtree is fragile
	// on a given edge when the reduce that produced it happened under an
	// ambiguous parse decision (LR-table conflict, GSS multi-pop, or
	// concurrent GLR stack versions -- see markReduceFragility,
	// parser_reduce.go), or when that edge's boundary child is itself
	// fragile on the same side (propagation). Query via isFragileLeft /
	// isFragileRight / isFragile, which also fold in isMissing()/IsError()
	// (error/missing nodes are always fragile, regardless of these bits).
	// Fragile subtrees must never be spliced into a fresh parse by
	// incremental non-leaf reuse (reuseNonLeafTargetStateOnStack,
	// incremental.go) -- see ts_parser__reuse_node's is_fragile rejection in
	// C. These two bits are pending-parent-domain-safe: pendingParent reuses
	// bits 5/6 (pendingParentFlagFieldEntries/DirectFieldEntry) as
	// pending-only scratch that publicPendingParentNodeFlags masks away
	// before a pendingParent's flags are copied onto a materialized Node;
	// fragileLeft/fragileRight live above that reused range and must be
	// added to publicPendingParentNodeFlags (pending_parent.go) so they
	// survive materialization.
	nodeFlagFragileLeft
	nodeFlagFragileRight
	// nodeFlagCompactRecoverEOF marks the one compact recover_eof root whose
	// raw C span must survive public result finalization. It is runtime-only
	// provenance and fits the existing uint16 flag budget.
	nodeFlagCompactRecoverEOF
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
func (n *Node) isExternalScannerToken() bool {
	return n != nil && n.hasFlag(nodeFlagExternalScannerToken)
}
func (n *Node) setExternalScannerToken(v bool) { n.setFlag(nodeFlagExternalScannerToken, v) }

func (n *Node) setLexerSkippedPrefixAtSourceStart(v bool) {
	n.setFlag(nodeFlagLexerSkippedPrefixAtSourceStart, v)
}

func (n *Node) hasLeadingLexerSkippedPrefixAtSourceStart() bool {
	if n == nil {
		return false
	}
	if n.hasFlag(nodeFlagLexerSkippedPrefixAtSourceStart) {
		return true
	}
	for i := 0; i < nodeChildCountNoMaterialize(n); i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(n, i)
		if !ok || stackEntryNodeIsExtra(entry) {
			continue
		}
		return stackEntryHasLeadingLexerSkippedPrefixAtSourceStart(entry, n.ownerArena)
	}
	return false
}

func stackEntryHasLeadingLexerSkippedPrefixAtSourceStart(entry stackEntry, arena *nodeArena) bool {
	if n := stackEntryNode(entry); n != nil {
		return n.hasLeadingLexerSkippedPrefixAtSourceStart()
	}
	if n := stackEntryNoTreeNode(entry); n != nil {
		return n.hasFlag(nodeFlagLexerSkippedPrefixAtSourceStart)
	}
	if n := stackEntryCompactFullLeaf(entry); n != nil {
		return n.hasFlag(nodeFlagLexerSkippedPrefixAtSourceStart)
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		for i := 0; i < parent.childEntryCount(); i++ {
			child := parent.childEntry(arena, i)
			if !stackEntryNodeIsExtra(child) {
				return stackEntryHasLeadingLexerSkippedPrefixAtSourceStart(child, arena)
			}
		}
	}
	return false
}
func (n *Node) dirty() bool {
	return n != nil && n.hasFlag(nodeFlagDirty)
}

// setFragileLeft/setFragileRight set the raw fragility bits. Reduce-time
// callers should use markReduceFragility (parser_reduce.go) instead of
// calling these directly, so left/right propagation from boundary children
// stays consistent everywhere a parent node is built.
func (n *Node) setFragileLeft(v bool) {
	if n == nil {
		return
	}
	n.setFlag(nodeFlagFragileLeft, v)
}

func (n *Node) setFragileRight(v bool) {
	if n == nil {
		return
	}
	n.setFlag(nodeFlagFragileRight, v)
}

// isFragileLeft/isFragileRight/isFragile report whether n's subtree is
// unsafe to splice into a fresh parse via incremental non-leaf reuse, on the
// given edge. Besides the raw fragileLeft/fragileRight bits (set by
// markReduceFragility for reduces that ran under ambiguity, and propagated
// from boundary children), any node that is itself missing or an ERROR node
// is unconditionally fragile on both edges -- mirroring C tree-sitter, which
// never reuses an is_missing/is_error subtree regardless of its fragile
// bits. This is computed from isMissing()/IsError() rather than set at
// construction time so every existing and future construction site (there
// are dozens across the per-grammar parser_result_*.go shaping helpers)
// is covered automatically, with no risk of a missed setFragileLeft/Right
// call silently under-marking a repair/recovery node.
func (n *Node) isFragileLeft() bool {
	if n == nil {
		return false
	}
	return n.hasFlag(nodeFlagFragileLeft) || n.isMissing() || n.symbol == errorSymbol
}

func (n *Node) isFragileRight() bool {
	if n == nil {
		return false
	}
	return n.hasFlag(nodeFlagFragileRight) || n.isMissing() || n.symbol == errorSymbol
}

// isFragile is the coarse (edge-agnostic) query used by the interior
// non-leaf incremental-reuse gate: reject the whole candidate if either edge
// is fragile. See reuseNonLeafTargetStateOnStack (incremental.go).
func (n *Node) isFragile() bool {
	return n.isFragileLeft() || n.isFragileRight()
}

func (n *Node) setDirty(v bool) {
	if n == nil {
		return
	}
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

// nodeBumpEquivVersionMetadata invalidates caches keyed by the node's complete
// parser identity without invalidating GSS recovery-prefix aggregates. Call it
// directly only for mutations that cannot change cNodeErrorCost or
// cNodeVisibleSubtreeCount: parser states, production/raw-shape metadata, and
// dynamic precedence. On published nodes, symbol, child, missing/extra, and
// ERROR-span mutations must use nodeBumpEquivVersion instead; fresh nodes use
// nodeBumpEquivVersionBeforePublication.
func nodeBumpEquivVersionMetadata(n *Node) {
	if n == nil {
		return
	}
	n.equivVersion++
	if n.equivVersion == 0 {
		n.equivVersion = 1
	}
	// Error rank is a pure function of the node's current subtree. The same
	// mutation choke point that invalidated the former version-keyed arena map
	// now invalidates the inline cache directly.
	n.errorRankCache = 0
}

// nodeBumpEquivVersionBeforePublication invalidates node-local caches after a
// recovery-relevant mutation to a fresh node that is provably not reachable
// from any GSS stack or returned tree yet. Because no cached GSS prefix can
// include an unpublished node, changing its error-cost or visible-count
// contribution must not invalidate every live prefix process-wide.
//
// Published nodes must use nodeBumpEquivVersion instead.
func nodeBumpEquivVersionBeforePublication(n *Node) {
	nodeBumpEquivVersionMetadata(n)
}

func nodeBumpEquivVersion(n *Node) {
	if n == nil {
		return
	}
	nodeBumpEquivVersionMetadata(n)
	// Recovery-relevant in-place mutations can change the node's error cost /
	// visible count, which invalidates every cached GSS prefix aggregate that
	// may sum over it (see gssPrefixAggGen, parser_recover_c.go). Metadata-only
	// mutations use nodeBumpEquivVersionMetadata above.
	gssPrefixAggGen.Add(1)
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
	fieldIDs := n.fieldIDs()
	if i < 0 || i >= len(fieldIDs) {
		return 0
	}
	return fieldIDs[i]
}

// ParseStopReason reports why parseInternal terminated.
type ParseStopReason string

const (
	ParseStopNone               ParseStopReason = "none"
	ParseStopAccepted           ParseStopReason = "accepted"
	ParseStopNoStacksAlive      ParseStopReason = "no_stacks_alive"
	ParseStopTokenSourceEOF     ParseStopReason = "token_source_eof"
	ParseStopTimeout            ParseStopReason = "timeout"
	ParseStopCancelled          ParseStopReason = "cancelled"
	ParseStopIterationLimit     ParseStopReason = "iteration_limit"
	ParseStopStackDepthLimit    ParseStopReason = "stack_depth_limit"
	ParseStopNodeLimit          ParseStopReason = "node_limit"
	ParseStopMemoryBudget       ParseStopReason = "memory_budget"
	ParseStopInvariantViolation ParseStopReason = "invariant_violation"
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
	MergeHeaderEqTotal                    uint64
	MergeDeepTrue                         uint64
	MergeDeepFalse                        uint64
	MergeHeaderDeepDivergent              uint64
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

// IncrementalRetryCause identifies why an incremental parse ran an additional
// bounded attempt.
type IncrementalRetryCause uint8

const (
	IncrementalRetryCauseNone IncrementalRetryCause = iota
	IncrementalRetryCauseAcceptedErrorBaseMerge
)

// RecoveryNodeMemoTier identifies the largest bounded recovery memo used by a
// parse. Entries and Bytes expand the compact telemetry value.
type RecoveryNodeMemoTier uint8

const (
	RecoveryNodeMemoTierNone RecoveryNodeMemoTier = iota
	RecoveryNodeMemoTierInitial
	RecoveryNodeMemoTierStandard
	RecoveryNodeMemoTierTemporary
)

// Entries reports the number of entries in this memo tier.
func (tier RecoveryNodeMemoTier) Entries() uint32 {
	switch tier {
	case RecoveryNodeMemoTierInitial:
		return cNodeMemoCacheInitialSize
	case RecoveryNodeMemoTierStandard:
		return cNodeMemoCacheSize
	case RecoveryNodeMemoTierTemporary:
		return cNodeMemoRecoveryCacheSize
	default:
		return 0
	}
}

// Bytes reports the allocated byte size of this memo tier.
func (tier RecoveryNodeMemoTier) Bytes() uint32 {
	return uint32(cNodeMemoCacheBytesForEntries(int(tier.Entries())))
}

// RecoveryNodeMemoRuntime reports bounded recovery-memo use for one tree.
type RecoveryNodeMemoRuntime struct {
	PeakTier   RecoveryNodeMemoTier
	Collisions uint32
}

// RecoveryRuntimeAttemptStats reports diagnostics for one parser attempt.
//
// These facts stay separate from RecoveryRuntimeStats. The latter reports the
// selected tree. A losing attempt can still consume time and memory.
type RecoveryRuntimeAttemptStats struct {
	Ordinal uint32
	Rung    string
	Cause   string

	StopReason          ParseStopReason
	Truncated           bool
	TokenSourceEOFEarly bool
	AttemptHasError     bool
	AttemptFullSpan     bool

	WallNanos            uint64
	HeapAllocDeltaBytes  int64
	TotalAllocDeltaBytes uint64
	MallocsDelta         uint64

	RecoveryEntryCount           uint64
	Strategy1ElectionCount       uint64
	RecoveryCostCompetitionCount uint64
	RecoveryCostWalkCount        uint64
	RecoveryCostWalkNanos        uint64

	MaterializationNanos                uint64
	ResultSelectionNanos                uint64
	TransientParentMaterializationNanos uint64
	ResultTreeBuildNanos                uint64
	TransientChildMaterializationNanos  uint64
	CondenseNanos                       uint64

	ArenaBytesPeak        uint64
	ScratchBytesPeak      uint64
	EntryScratchBytesPeak uint64
	GSSBytesPeak          uint64
	GSSNodesPeak          uint64
	NodesAllocated        uint64
	MaxStacksSeen         uint64
	PeakStackDepth        uint64
	LiveVersions          uint64
	PeakLiveVersions      uint64

	CandidateSelected          bool
	CandidateReplacedIncumbent bool
}

// RecoveryRuntimeStats reports opt-in recovery facts for the most recent
// completed parse attempt on a parser.
//
// The default value reports no facts. Enable the telemetry before parsing.
// Existing tree runtime fields and RecoveryNodeMemoRuntime provide the other
// B16 facts, including materialization, allocation, stop, and memo metrics.
type RecoveryRuntimeStats struct {
	Enabled   bool
	Completed bool

	RecoveryEntryCount           uint64
	Strategy1ElectionCount       uint64
	RecoveryCostCompetitionCount uint64
	RecoveryCostWalkCount        uint64
	RecoveryCostWalkNanos        uint64
	ErrorNodeCount               uint64
	ErrorSpanBytes               uint32
	RetryPassCount               uint64
	RetryReason                  string
	RetryAttemptCount            uint64
	RetrySelectedAttempt         string
	RetrySelectedAttemptHasError bool
	RetrySelectedAttemptFullSpan bool
	ErrorModeTokenCount          uint64
	ScannerResyncCount           uint64
	LiveVersionCount             uint64
	PeakLiveVersionCount         uint64
}

// RecoveryRuntimeAttempts contains attempt-local facts for one parse
// operation. Use Parser.DebugRecoveryRuntimeAttempts to read this receipt.
type RecoveryRuntimeAttempts []RecoveryRuntimeAttemptStats

// ParseRuntime captures parser-loop diagnostics for a completed tree.
type ParseRuntime struct {
	StopReason     ParseStopReason
	ForestFastPath bool
	// IncrementalAcceptedErrorRetryAttempts records the bounded second
	// incremental pass used when an accepted, full-span ERROR tree was produced
	// under the wider incremental merge policy. The retry keeps the edited old
	// tree and reruns with the ordinary full-parse merge cap; it is not a fresh
	// parse fallback.
	IncrementalAcceptedErrorRetryAttempts uint8
	// IncrementalAcceptedErrorRetryAdopted is true only when that retry produced
	// a strictly better tree and replaced the first incremental result.
	IncrementalAcceptedErrorRetryAdopted bool
	// IncrementalAcceptedErrorRetryMergePerKey is the exact merge-per-key cap
	// used by the bounded retry.
	IncrementalAcceptedErrorRetryMergePerKey int
	// IncrementalAcceptedErrorRetryCause is a stable diagnostic reason for the
	// retry so corpus and benchmark matrices can distinguish this path.
	IncrementalAcceptedErrorRetryCause IncrementalRetryCause
	// IncrementalOldTreeReuseRoute reports whether this result was produced by
	// an actual old-tree reuse parse rather than an internal fresh fallback.
	IncrementalOldTreeReuseRoute bool
	// CRecoveryEnteredErrorState is true when the faithful C error-recovery
	// port (parser_recover_c.go) actually ran ts_parser__handle_error at
	// least once while producing this specific tree — i.e. some no-action
	// point was hit for the current lookahead. This is NOT proof the input is
	// malformed: LALR table limitations routinely drive well-formed,
	// compiling input into a momentary no-action point that C-recovery
	// resolves losslessly (ordinary GLR disambiguation). It is only a cheap
	// pre-filter for Parse()'s post-parse swallowed-error safety net
	// (resolveCRecoverySwallowedError) — see CRecoveryDroppedErrorForClean
	// for the actual, precise suspicion signal. It is captured per finalized
	// tree (not read from a raw Parser field) so a discarded retry attempt
	// can never leave a stale value on the tree that is actually returned.
	CRecoveryEnteredErrorState bool
	// CRecoveryDroppedErrorForClean is true when the stack SELECTED as this
	// tree's parse result (buildResultFromGLR, parser_result.go) carried an
	// unvalidated C-recovery marker (see glrStack.cRecoveryUnvalidatedMarker)
	// with no unflagged sibling reaching the same final position — i.e. the
	// selected lineage itself created a real ERROR node via cRecoverToState
	// (for a single-stack dead end with a small recovered span) and was
	// never re-validated by another cost competition. This is the precise
	// signature of the swallowed-error defect class (see
	// resolveCRecoverySwallowedError): unlike an ordinary clean recovery (no
	// unvalidated marker, or a corroborating clean sibling, at the same
	// position), this means the specific result being returned lost real
	// ERROR content somewhere along its own lineage. Deliberately scoped to
	// the selected result only — NOT set for drops/forks on discarded
	// lineages elsewhere in the parse, nor for large/multi-stack recoveries;
	// both were tried and found to fire on ordinary GLR disambiguation for a
	// measurable fraction of valid, compiling source in a real repo-file
	// walk (the discarded-lineage version: thousands of times per large Go
	// file) and were the cause of a prior, over-broad version of this
	// signal's false-fire rate.
	CRecoveryDroppedErrorForClean bool
	// CRecoverySwallowedErrorFallbackAttempted is true when
	// resolveCRecoverySwallowedError actually re-parsed the source with the
	// C-recovery gate disabled to double-check a suspicious clean result (see
	// CRecoveryDroppedErrorForClean). Diagnostic only — lets callers measure
	// the fallback's false-fire rate (extra latency from a re-parse that is
	// usually discarded) against their own corpora, e.g. by walking a tree of
	// known-valid source files and counting how often this is true.
	CRecoverySwallowedErrorFallbackAttempted bool
	// CompactExternalScannerCheckpointTransferProven reports whether compact
	// materialization transferred every required terminal scanner checkpoint
	// into node sidecars. A false value disables later subtree reuse.
	CompactExternalScannerCheckpointTransferProven bool
	// CRecoverReductionCandidateCeilingHits and CRecoverMissingTokenCeilingHits
	// count how many times this parse's cDoAllPotentialReductions /
	// cHandleError missing-token search hit the
	// cRecoverMaxReductionCandidateAttempts / cRecoverMaxMissingTokenTrials
	// Go-side backstop ceilings (parser_recover_c.go), a diagnostic signal for
	// the spore.2026-08-02.walnut-e.memory-exhaustion fix. Both stay zero on
	// every currently-passing parse; neither ceiling halts the parse itself
	// (both fail gracefully into an already-supported "search found nothing"
	// path), so this counter is the only way to observe that either engaged.
	CRecoverReductionCandidateCeilingHits uint64
	CRecoverMissingTokenCeilingHits       uint64
	// CRecoverReductionCandidateAttemptsPeak and
	// CRecoverMissingTokenTrialAttemptsPeak record the single largest
	// candidateAttempts / missingTokenTrialAttempts value any ONE
	// cDoAllPotentialReductions call / cHandleError missing-token search
	// reached during this parse (not cumulative across calls). Diagnostic
	// only: lets a corpus walk report how close real input gets to
	// cRecoverMaxReductionCandidateAttempts / cRecoverMaxMissingTokenTrials.
	CRecoverReductionCandidateAttemptsPeak        uint64
	CRecoverMissingTokenTrialAttemptsPeak         uint64
	SourceLen                                     uint32
	ExpectedEOFByte                               uint32
	RootEndByte                                   uint32
	Truncated                                     bool
	TokenSourceEOFEarly                           bool
	TokensConsumed                                uint64
	LastTokenEndByte                              uint32
	LastTokenSymbol                               Symbol
	LastTokenWasEOF                               bool
	StopDiagnosticCaptured                        bool
	StopDiagnosticCRecoveryEnabled                bool
	StopDiagnosticCRecoveryGateReason             string
	StopDiagnosticRecoverActionAvailable          bool
	StopDiagnosticLastStackState                  StateID
	StopDiagnosticLastStackByte                   uint32
	StopDiagnosticLastStackDepth                  int
	StopDiagnosticTokenSymbol                     Symbol
	StopDiagnosticTokenStartByte                  uint32
	StopDiagnosticTokenEndByte                    uint32
	StopDiagnosticTokenNoLookahead                bool
	StopDiagnosticRootType                        string
	StopDiagnosticRootStartByte                   uint32
	StopDiagnosticRootEndByte                     uint32
	StopDiagnosticRootHasError                    bool
	StopDiagnosticFirstErrorFound                 bool
	StopDiagnosticFirstErrorStartByte             uint32
	StopDiagnosticFirstErrorEndByte               uint32
	StopDiagnosticFrontierStacks                  string
	StopDiagnosticFrontierActions                 string
	StopDiagnosticSameHeaderGroups                string
	StopDiagnosticCondenseGating                  string
	StopDiagnosticActionCaptured                  bool
	StopDiagnosticActionPhase                     string
	StopDiagnosticActionStackState                StateID
	StopDiagnosticActionStackByte                 uint32
	StopDiagnosticActionStackDepth                int
	StopDiagnosticActionTokenSymbol               Symbol
	StopDiagnosticActionTokenStartByte            uint32
	StopDiagnosticActionTokenEndByte              uint32
	StopDiagnosticActionTokenNoLookahead          bool
	StopDiagnosticActionType                      ParseActionType
	StopDiagnosticActionState                     StateID
	StopDiagnosticActionSymbol                    Symbol
	StopDiagnosticActionChildCount                uint8
	StopDiagnosticActionProductionID              uint16
	StopDiagnosticActionDynamicPrecedence         int16
	StopDiagnosticActionCount                     int
	StopDiagnosticActionResultState               StateID
	StopDiagnosticActionInReduceChain             bool
	StopDiagnosticActionReduceChainStep           int
	StopDiagnosticActionRepeatedSignatureCount    int
	StopDiagnosticActionReduceChainCycle          bool
	StopDiagnosticActionForceAdvanceAfterReduce   bool
	StopDiagnosticActionPostDispatchDefaultReduce bool
	StopDiagnosticActionAnyReduced                bool
	StopDiagnosticActionConsumedToken             bool
	StopDiagnosticActionDispatchShiftActions      int
	StopDiagnosticActionDispatchReduceActions     int
	StopDiagnosticActionDispatchAcceptActions     int
	StopDiagnosticActionDispatchRecoverActions    int
	StopDiagnosticActionDispatchOtherActions      int
	StopDiagnosticLastReduceCaptured              bool
	StopDiagnosticLastReducePhase                 string
	StopDiagnosticLastReduceStackState            StateID
	StopDiagnosticLastReduceStackByte             uint32
	StopDiagnosticLastReduceStackDepth            int
	StopDiagnosticLastReduceSymbol                Symbol
	StopDiagnosticLastReduceChildCount            uint8
	StopDiagnosticLastReduceProductionID          uint16
	StopDiagnosticLastReduceDynamicPrecedence     int16
	StopDiagnosticLastReduceResultState           StateID
	StopDiagnosticLastReduceInChain               bool
	StopDiagnosticLastReduceChainStep             int
	StopDiagnosticLastReduceRepeatedSigCount      int
	StopDiagnosticLastReduceChainCycle            bool
	IterationLimit                                int
	StackDepthLimit                               int
	NodeLimit                                     int
	MemoryBudgetBytes                             int64
	MemoryBudgetStopSource                        string // First budget guard: arena, scratch, runtime_heap, runtime_sys, or hard_ceiling.
	RuntimeHeapGrowthBytes                        uint64
	RuntimeSysGrowthBytes                         uint64
	Iterations                                    int
	NodesAllocated                                int
	ArenaBytesAllocated                           int64
	ArenaBaselineBytes                            int64
	ScratchBytesAllocated                         int64
	ScratchBaselineBytes                          int64
	EntryScratchBytesAllocated                    int64
	EntryScratchPeak                              uint64
	GSSBytesAllocated                             int64
	GSSBaselineBytes                              int64
	GSSSlabCount                                  int
	GSSNodesUsed                                  int
	GSSNodesCapacity                              int
	GSSDemotions                                  uint64
	GSSNodesDemoted                               uint64
	TransientScratchCheckpoints                   uint64
	PeakStackDepth                                int
	MaxStacksSeen                                 int
	SingleStackIterations                         int
	MultiStackIterations                          int
	SingleStackTokens                             uint64
	MultiStackTokens                              uint64
	SingleStackGSSNodes                           uint64
	MultiStackGSSNodes                            uint64
	GSSNodesAllocated                             uint64
	GSSNodesRetained                              uint64
	GSSNodesDroppedSameToken                      uint64
	ParentNodesAllocated                          uint64
	ParentNodesRetained                           uint64
	ParentNodesDroppedSameToken                   uint64
	LeafNodesAllocated                            uint64
	LeafNodesRetained                             uint64
	LeafNodesDroppedSameToken                     uint64
	ChildSlicesAllocated                          uint64
	ChildSlicesRetained                           uint64
	ChildSlicesDroppedSameToken                   uint64
	ChildPointersAllocated                        uint64
	ChildPointersRetained                         uint64
	ChildPointersDroppedSameToken                 uint64
	ReduceChildFastGSS                            ReduceChildPathRuntime
	ReduceChildAllVisible                         ReduceChildPathRuntime
	ReduceChildScratchGeneral                     ReduceChildPathRuntime
	ReduceChildScratchNoAlias                     ReduceChildPathRuntime
	TransientChildSlicesAllocated                 uint64
	TransientChildPointersAllocated               uint64
	TransientChildSlicesMaterialized              uint64
	TransientChildPointersMaterialized            uint64
	TransientParentNodesAllocated                 uint64
	TransientParentNodesMaterialized              uint64
	FinalNodes                                    uint64
	FinalParentNodes                              uint64
	FinalLeafNodes                                uint64
	FinalFieldedParentNodes                       uint64
	FinalUnfieldedParentNodes                     uint64
	FinalVisibleParentNodes                       uint64
	FinalHiddenParentNodes                        uint64
	FinalCheckpointLeafNodes                      uint64
	FinalChildSlices                              uint64
	FinalChildPointers                            uint64
	FinalFieldIDElements                          uint64
	FinalFieldSourceElements                      uint64
	FinalChildRefParents                          uint64
	FinalChildRefs                                uint64
	FinalChildRefMaterializedParents              uint64
	FinalChildRefMaterializedChildren             uint64
	FinalChildRefSingleChildAccesses              uint64
	FinalChildRefSingleChildMaterializedChildren  uint64
	MergeStacksIn                                 uint64
	MergeStacksOut                                uint64
	MergeSlotsUsed                                uint64
	GlobalCullStacksIn                            uint64
	GlobalCullStacksOut                           uint64
	StackEquivCalls                               uint64
	StackEquivTrue                                uint64
	StackEquivDepthMismatch                       uint64
	StackEquivHashMismatch                        uint64
	StackEquivStateMismatch                       uint64
	StackEquivPayloadMismatch                     uint64
	StackEquivEntryCompares                       uint64
	StackEquivStateMismatchDepthSum               uint64
	StackEquivStateMismatchMaxDepth               uint32
	StackEquivStateMismatchDepthBuckets           [stackEquivMismatchDepthBucketCount]uint64
	StackEquivPayloadMismatchDepthSum             uint64
	StackEquivPayloadMismatchMaxDepth             uint32
	StackEquivPayloadMismatchDepthBuckets         [stackEquivMismatchDepthBucketCount]uint64
	StackEquivPayloadHeaderSigDiff                uint64
	StackEquivPayloadHeaderSigSame                uint64
	StackEquivPayloadShallowSigDiff               uint64
	StackEquivPayloadShallowSigSame               uint64
	StackEquivPairKeyed                           uint64
	StackEquivPairUnkeyed                         uint64
	StackEquivPairRepeats                         uint64
	StackEquivPairRepeatTrue                      uint64
	StackEquivPairRepeatFalse                     uint64
	StackEquivPairRepeatMismatch                  uint64
	StackEquivPairStores                          uint64
	MergeHeaderEqTotal                            uint64
	MergeDeepTrue                                 uint64
	MergeDeepFalse                                uint64
	MergeHeaderDeepDivergent                      uint64
	EquivCacheLookups                             uint64
	EquivCacheHits                                uint64
	EquivCacheStores                              uint64
	EquivCacheMisses                              uint64
	EquivCacheTrueHits                            uint64
	EquivCacheFalseHits                           uint64
	EquivCacheEpochMisses                         uint64
	EquivCacheKeyMisses                           uint64
	EquivCacheVersionMisses                       uint64
	EquivSkipError                                uint64
	EquivSkipLeaf                                 uint64
	EquivSkipFieldMismatch                        uint64
	EquivExactCalls                               uint64
	EquivExactTrue                                uint64
	EquivExactPointerTrue                         uint64
	EquivExactNilMismatch                         uint64
	EquivExactHeaderMismatch                      uint64
	EquivExactChildMismatch                       uint64
	EquivExactTerminalCalls                       uint64
	EquivExactTerminalTrue                        uint64
	EquivExactTerminalFalse                       uint64
	EquivFrontierCalls                            uint64
	EquivFrontierTrue                             uint64
	EquivExactChildCompares                       uint64
	EquivFrontierChildScans                       uint64
	EquivFrontierCandidateCompares                uint64
	EquivStateStats                               []ParseEquivStateRuntime
	ParseWallNanos                                int64
	ParserLoopNanos                               int64
	TokenNextNanos                                int64
	ActionDispatchNanos                           int64
	ActionLookupNanos                             int64
	GLRMergeNanos                                 int64
	GLRCullNanos                                  int64
	ReduceTiming                                  *ParseReduceTiming
	ActionTiming                                  *ParseActionTiming

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
	// RecoveryProbeInitialAttempts counts initial-only nested recovery probes.
	// Only scoped clean full-source recovery normalizers use these probes.
	RecoveryProbeInitialAttempts uint64
	// RecoveryProbeInitialAccepted counts probes that met the caller's exact
	// clean and full-span acceptance rule.
	RecoveryProbeInitialAccepted uint64
	// RecoveryProbeLegacyFallbacks counts probes that ran the legacy retry path.
	RecoveryProbeLegacyFallbacks uint64
	// RecoveryProbeInitialRetryPasses is zero when the initial-only contract
	// holds. It records violations for diagnostics.
	RecoveryProbeInitialRetryPasses uint64
	// RecoveryProbeLegacyRetryPasses counts retry-ladder passes after a probe
	// declined its initial result.
	RecoveryProbeLegacyRetryPasses uint64
	// SwiftLegacyRecoverySubparseAttempts counts legacy Swift recovery parses.
	// It excludes the initial-only probe and its measured fallback route.
	SwiftLegacyRecoverySubparseAttempts uint64
	// SwiftLegacyRecoveryRetryPasses counts retry-ladder passes in those Swift
	// legacy recovery parses.
	SwiftLegacyRecoveryRetryPasses uint64
	// The parser sets NativeRecoveredStructureAuthoritative when an exact
	// grammar profile certifies the recovered tree before compatibility.
	NativeRecoveredStructureAuthoritative bool
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
	NodeStructBytesAllocated            int64
	NodeFieldMetadataBytesAllocated     int64
	NoTreeNodeBytesAllocated            int64
	CompactFullLeafBytesAllocated       int64
	PendingParentBytesAllocated         int64
	PendingChildEntryBytesAllocated     int64
	RawShapeBytesAllocated              int64
	RawShapeChildBytesAllocated         int64
	RawShapeHashCacheBytesAllocated     int64
	FinalChildSidecarBytesAllocated     int64
	MissingNodeDependencyBytesAllocated int64
	CompactCheckpointLeafBytesAllocated int64
	MissingNodeDependencyCount          uint64
	PendingChildEntriesAllocated        uint64
	PendingChildEntryCapacity           uint64
	PendingChildEntryWaste              uint64
	ChildSliceBytesAllocated            int64
	FieldIDBytesAllocated               int64
	FieldSourceBytesAllocated           int64
	MergeScratchBytesAllocated          int64

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
	s := fmt.Sprintf(
		"truncated=%v stopReason=%s forestFastPath=%v tokenEOFEarly=%v tokens=%d lastTokenEnd=%d expectedEOF=%d lastTokenSymbol=%d lastTokenEOF=%v iterations=%d/%d nodes=%d/%d arena=%d/%d scratch=%d(entry=%d gss=%d)/%d peakDepth=%d/%d maxStacks=%d",
		rt.Truncated, stopReason, rt.ForestFastPath, rt.TokenSourceEOFEarly, rt.TokensConsumed,
		rt.LastTokenEndByte, rt.ExpectedEOFByte, rt.LastTokenSymbol, rt.LastTokenWasEOF,
		rt.Iterations, rt.IterationLimit, rt.NodesAllocated, rt.NodeLimit,
		rt.ArenaBytesAllocated, rt.MemoryBudgetBytes,
		rt.ScratchBytesAllocated, rt.EntryScratchBytesAllocated, rt.GSSBytesAllocated, rt.MemoryBudgetBytes,
		rt.PeakStackDepth, rt.StackDepthLimit, rt.MaxStacksSeen,
	)
	if rt.MemoryBudgetStopSource != "" {
		s += fmt.Sprintf(
			" memoryBudgetStop={source=%s arenaBaseline=%d heapGrowth=%d sysGrowth=%d}",
			rt.MemoryBudgetStopSource,
			rt.ArenaBaselineBytes,
			rt.RuntimeHeapGrowthBytes,
			rt.RuntimeSysGrowthBytes,
		)
	}
	if rt.StopDiagnosticCaptured {
		s += fmt.Sprintf(
			" stopDiag={cRecovery=%v recoverAction=%v stackState=%d stackByte=%d stackDepth=%d token=%d[%d:%d] noLookahead=%v root=%q[%d:%d] rootErr=%v firstError=%v[%d:%d]",
			rt.StopDiagnosticCRecoveryEnabled,
			rt.StopDiagnosticRecoverActionAvailable,
			rt.StopDiagnosticLastStackState,
			rt.StopDiagnosticLastStackByte,
			rt.StopDiagnosticLastStackDepth,
			rt.StopDiagnosticTokenSymbol,
			rt.StopDiagnosticTokenStartByte,
			rt.StopDiagnosticTokenEndByte,
			rt.StopDiagnosticTokenNoLookahead,
			rt.StopDiagnosticRootType,
			rt.StopDiagnosticRootStartByte,
			rt.StopDiagnosticRootEndByte,
			rt.StopDiagnosticRootHasError,
			rt.StopDiagnosticFirstErrorFound,
			rt.StopDiagnosticFirstErrorStartByte,
			rt.StopDiagnosticFirstErrorEndByte,
		)
		if rt.StopDiagnosticCRecoveryGateReason != "" {
			s += " cRecoveryGateReason=" + rt.StopDiagnosticCRecoveryGateReason
		}
		if rt.StopDiagnosticFrontierStacks != "" {
			s += " " + rt.StopDiagnosticFrontierStacks
		}
		if rt.StopDiagnosticFrontierActions != "" {
			s += " " + rt.StopDiagnosticFrontierActions
		}
		if rt.StopDiagnosticSameHeaderGroups != "" {
			s += " " + rt.StopDiagnosticSameHeaderGroups
		}
		if rt.StopDiagnosticCondenseGating != "" {
			s += " " + rt.StopDiagnosticCondenseGating
		}
		s += "}"
	}
	if rt.StopDiagnosticActionCaptured {
		s += fmt.Sprintf(
			" stopAction={phase=%q stack=%d[%d]@%d token=%d[%d:%d] noLookahead=%v type=%d state=%d symbol=%d childCount=%d productionID=%d dynamicPrecedence=%d actionCount=%d resultState=%d reduceChain=%v chainStep=%d repeatedSigCount=%d chainCycle=%v dispatchShift=%d dispatchReduce=%d dispatchAccept=%d dispatchRecover=%d dispatchOther=%d forceAdvanceAfterReduce=%v postDispatchDefaultReduce=%v anyReduced=%v consumedToken=%v}",
			rt.StopDiagnosticActionPhase,
			rt.StopDiagnosticActionStackState,
			rt.StopDiagnosticActionStackDepth,
			rt.StopDiagnosticActionStackByte,
			rt.StopDiagnosticActionTokenSymbol,
			rt.StopDiagnosticActionTokenStartByte,
			rt.StopDiagnosticActionTokenEndByte,
			rt.StopDiagnosticActionTokenNoLookahead,
			rt.StopDiagnosticActionType,
			rt.StopDiagnosticActionState,
			rt.StopDiagnosticActionSymbol,
			rt.StopDiagnosticActionChildCount,
			rt.StopDiagnosticActionProductionID,
			rt.StopDiagnosticActionDynamicPrecedence,
			rt.StopDiagnosticActionCount,
			rt.StopDiagnosticActionResultState,
			rt.StopDiagnosticActionInReduceChain,
			rt.StopDiagnosticActionReduceChainStep,
			rt.StopDiagnosticActionRepeatedSignatureCount,
			rt.StopDiagnosticActionReduceChainCycle,
			rt.StopDiagnosticActionDispatchShiftActions,
			rt.StopDiagnosticActionDispatchReduceActions,
			rt.StopDiagnosticActionDispatchAcceptActions,
			rt.StopDiagnosticActionDispatchRecoverActions,
			rt.StopDiagnosticActionDispatchOtherActions,
			rt.StopDiagnosticActionForceAdvanceAfterReduce,
			rt.StopDiagnosticActionPostDispatchDefaultReduce,
			rt.StopDiagnosticActionAnyReduced,
			rt.StopDiagnosticActionConsumedToken,
		)
	}
	if rt.StopDiagnosticLastReduceCaptured {
		s += fmt.Sprintf(
			" stopLastReduce={phase=%q stack=%d[%d]@%d symbol=%d childCount=%d productionID=%d dynamicPrecedence=%d resultState=%d reduceChain=%v chainStep=%d repeatedSigCount=%d chainCycle=%v}",
			rt.StopDiagnosticLastReducePhase,
			rt.StopDiagnosticLastReduceStackState,
			rt.StopDiagnosticLastReduceStackDepth,
			rt.StopDiagnosticLastReduceStackByte,
			rt.StopDiagnosticLastReduceSymbol,
			rt.StopDiagnosticLastReduceChildCount,
			rt.StopDiagnosticLastReduceProductionID,
			rt.StopDiagnosticLastReduceDynamicPrecedence,
			rt.StopDiagnosticLastReduceResultState,
			rt.StopDiagnosticLastReduceInChain,
			rt.StopDiagnosticLastReduceChainStep,
			rt.StopDiagnosticLastReduceRepeatedSigCount,
			rt.StopDiagnosticLastReduceChainCycle,
		)
	}
	return s
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

// HasErrorOrMissing reports whether this node or a descendant contains an
// ERROR or MISSING node. Use it for strict parse-health checks.
func (n *Node) HasErrorOrMissing() bool {
	if n == nil {
		return false
	}
	if n.hasError() || n.IsError() || n.IsMissing() {
		return true
	}

	var local [64]*Node
	stack := local[:0]
	stack = append(stack, n)
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		for _, child := range current.children {
			if child == nil {
				continue
			}
			if child.hasError() || child.IsError() || child.IsMissing() {
				return true
			}
			stack = append(stack, child)
		}
	}
	return false
}

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
		return unescapePunctuationSymbolName(lang.SymbolNames[n.symbol])
	}
	return ""
}

// unescapePunctuationSymbolName maps blob symbol names back to C's display
// names. The blob generator escapes exactly one character in anonymous
// string-token display names — `?` becomes `\?` (grammargen's
// escapeAnonymousName) — where C names the token by its literal text
// (`?`, `defined?`, `.?`, `??=`). This function is the exact inverse:
// every `\?` becomes `?`, and nothing else changes. All other backslashes
// are literal name content that C also reports verbatim: string tokens
// whose text contains a backslash (pkl/swift/jq `\(`, cue `\#(`, circom
// `\=`, tmux/cmake `\;`, twig `\"`, org `\]`, tlaplus `\/`, textproto
// `\?` which the generator escapes to `\\?`) and regex-source token names
// (fortran's `\.not\.` dot-keyword operators). Verified against the C
// oracle over every backslash-containing symbol name in the embedded
// blobs (2026-06-10).
func unescapePunctuationSymbolName(name string) string {
	if !strings.Contains(name, `\?`) {
		return name
	}
	return strings.ReplaceAll(name, `\?`, `?`)
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

func stackEntryContainsByteRange(entry stackEntry, startByte, endByte uint32) bool {
	return startByte >= stackEntryNodeStartByte(entry) && endByte <= stackEntryNodeEndByte(entry)
}

// descendantForByteRange walks to the smallest descendant covering [startByte,
// endByte], matching upstream tree-sitter's ts_node__descendant_for_byte_range
// (lib/src/node.c): descend into the first child whose end reaches range_end
// and whose start is at or before range_start; stop scanning once a child
// starts past range_start. Non-empty children must extend past range_start,
// while zero-width children at range_start remain eligible. It returns the
// last relevant node visited, i.e. the node itself for a valid unsatisfiable
// or out-of-bounds range. A nil receiver or reversed range returns nil.
func (n *Node) descendantForByteRange(startByte, endByte uint32, namedOnly bool) *Node {
	if n == nil || endByte < startByte {
		return nil
	}
	node := n
	lastRelevant := n
	for {
		descended := false
		childCount := nodeChildCountNoMaterialize(node)
		for i := 0; i < childCount; i++ {
			entry, ok := nodeChildEntryAtNoMaterialize(node, i)
			if !ok {
				continue
			}
			childStart := stackEntryNodeStartByte(entry)
			childEnd := stackEntryNodeEndByte(entry)
			if childEnd < endByte {
				continue
			}
			if childStart == childEnd {
				if childEnd < startByte {
					continue
				}
			} else if childEnd <= startByte {
				continue
			}
			if childStart > startByte {
				break
			}
			child := nodeChildAtForReason(node, i, materializeForParentAPI)
			if child == nil {
				continue
			}
			node = child
			if !namedOnly || child.isNamed() {
				lastRelevant = child
			}
			descended = true
			break
		}
		if !descended {
			break
		}
	}
	return lastRelevant
}

// descendantForByteRangeContained is the pre-C-parity walk: it returns the
// smallest descendant that *fully contains* [startByte, endByte], or nil when
// the range is not contained. It is retained for internal callers (incremental
// reuse fast paths) that depend on the nil-on-miss / boundary-inclusive
// behavior; the public Descendant*ForByteRange API uses the C-faithful walk.
func (n *Node) descendantForByteRangeContained(startByte, endByte uint32, namedOnly bool) *Node {
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
		if d := child.descendantForByteRangeContained(startByte, endByte, namedOnly); d != nil {
			deepest = d
		}
	}
	return deepest
}

// descendantForPointRange is the point-coordinate C-faithful walk; see
// descendantForByteRange for the boundary/out-of-range rule it mirrors.
func (n *Node) descendantForPointRange(startPoint, endPoint Point, namedOnly bool) *Node {
	if n == nil || pointLessThan(endPoint, startPoint) {
		return nil
	}
	node := n
	lastRelevant := n
	for {
		descended := false
		childCount := nodeChildCountNoMaterialize(node)
		for i := 0; i < childCount; i++ {
			entry, ok := nodeChildEntryAtNoMaterialize(node, i)
			if !ok {
				continue
			}
			childStart := stackEntryNodeStartPoint(entry)
			childEnd := stackEntryNodeEndPoint(entry)
			if pointLessThan(childEnd, endPoint) {
				continue
			}
			if childStart == childEnd {
				if pointLessThan(childEnd, startPoint) {
					continue
				}
			} else if pointLessOrEqual(childEnd, startPoint) {
				continue
			}
			if pointGreaterThan(childStart, startPoint) {
				break
			}
			child := nodeChildAtForReason(node, i, materializeForParentAPI)
			if child == nil {
				continue
			}
			node = child
			if !namedOnly || child.isNamed() {
				lastRelevant = child
			}
			descended = true
			break
		}
		if !descended {
			break
		}
	}
	return lastRelevant
}

// DescendantForByteRange returns the smallest descendant selected by upstream
// tree-sitter's byte-range walk. For a valid unsatisfiable or out-of-bounds
// range it returns the receiver; a nil receiver or reversed range returns nil.
func (n *Node) DescendantForByteRange(startByte, endByte uint32) *Node {
	return n.descendantForByteRange(startByte, endByte, false)
}

// NodeAtByte returns the smallest descendant that contains byteOffset. If the
// offset is exactly at this node's end byte, it performs a zero-width lookup at
// that boundary. Returns nil when the offset is outside this node.
func (n *Node) NodeAtByte(byteOffset uint32) *Node {
	if n == nil || byteOffset < n.startByte || byteOffset > n.endByte {
		return nil
	}
	endByte := byteOffset
	if byteOffset < n.endByte {
		endByte = byteOffset + 1
	}
	return n.descendantForByteRangeContained(byteOffset, endByte, false)
}

// NamedDescendantForByteRange returns the smallest named descendant selected
// by upstream tree-sitter's byte-range walk. For a valid unsatisfiable or
// out-of-bounds range it returns the receiver; a nil receiver or reversed
// range returns nil.
func (n *Node) NamedDescendantForByteRange(startByte, endByte uint32) *Node {
	return n.descendantForByteRange(startByte, endByte, true)
}

// NamedNodeAtByte returns the smallest named descendant that contains
// byteOffset. It follows the same boundary behavior as NodeAtByte.
func (n *Node) NamedNodeAtByte(byteOffset uint32) *Node {
	if n == nil || byteOffset < n.startByte || byteOffset > n.endByte {
		return nil
	}
	endByte := byteOffset
	if byteOffset < n.endByte {
		endByte = byteOffset + 1
	}
	return n.descendantForByteRangeContained(byteOffset, endByte, true)
}

// DescendantForPointRange returns the smallest descendant selected by upstream
// tree-sitter's point-range walk. For a valid unsatisfiable or out-of-bounds
// range it returns the receiver; a nil receiver or reversed range returns nil.
func (n *Node) DescendantForPointRange(startPoint, endPoint Point) *Node {
	return n.descendantForPointRange(startPoint, endPoint, false)
}

// NamedDescendantForPointRange returns the smallest named descendant selected
// by upstream tree-sitter's point-range walk. For a valid unsatisfiable or
// out-of-bounds range it returns the receiver; a nil receiver or reversed
// range returns nil.
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

		hasError := false
		for i, c := range children {
			setNodeParentLink(c, n, i)
			if c.hasError() {
				hasError = true
			}
		}
		n.setHasError(hasError)
	}
}

// refreshRewrittenParentPreservingProducedSpan refreshes an existing parent
// after an in-place child rewrite. A produced span can widen but cannot shrink.
func refreshRewrittenParentPreservingProducedSpan(n *Node, children []*Node) {
	if n == nil {
		return
	}
	oldStartByte := n.startByte
	oldEndByte := n.endByte
	oldStartPoint := n.startPoint
	oldEndPoint := n.endPoint
	preserveProducedSpan := n.productionID != 0 &&
		oldStartByte <= oldEndByte &&
		!pointLessThan(oldEndPoint, oldStartPoint)

	populateParentNode(n, children)
	if !preserveProducedSpan {
		return
	}
	if n.startByte >= oldStartByte {
		n.startByte = oldStartByte
		n.startPoint = oldStartPoint
	}
	if n.endByte <= oldEndByte {
		n.endByte = oldEndByte
		n.endPoint = oldEndPoint
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
	wireParentLinksWithScratchUntil(root, scratch, nil, nil)
}

func wireParentLinksWithScratchUntil(
	root *Node,
	scratch *[]*Node,
	p *Parser,
	errorSummary *resultErrorSummary,
) bool {
	if root == nil {
		if errorSummary != nil {
			*errorSummary = resultErrorSummaryUnknown
		}
		return true
	}
	setNodeRootLink(root)
	if errorSummary != nil {
		*errorSummary = resultErrorSummaryClean
		if root.IsError() || root.hasError() {
			*errorSummary = resultErrorSummaryPresent
		}
	}

	var stack []*Node
	if scratch != nil {
		stack = (*scratch)[:0]
	} else {
		var local [64]*Node
		stack = local[:0]
	}
	stack = append(stack, root)
	if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
		if scratch != nil {
			*scratch = stack[:0]
		}
		if errorSummary != nil && *errorSummary != resultErrorSummaryPresent {
			*errorSummary = resultErrorSummaryUnknown
		}
		return false
	}
	for len(stack) > 0 {
		if reason := p.materializationParseStopReason(); parseStopReasonIsTerminal(reason) {
			if scratch != nil {
				*scratch = stack[:0]
			}
			if errorSummary != nil && *errorSummary != resultErrorSummaryPresent {
				*errorSummary = resultErrorSummaryUnknown
			}
			return false
		}
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if errorSummary != nil && (n.IsError() || n.hasError()) {
			*errorSummary = resultErrorSummaryPresent
		}
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
	if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
		if errorSummary != nil && *errorSummary != resultErrorSummaryPresent {
			*errorSummary = resultErrorSummaryUnknown
		}
		return false
	}
	return true
}

type finalTreeMaterializationStats struct {
	nodes                uint64
	parentNodes          uint64
	leafNodes            uint64
	nodeFieldMetadata    uint64
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
	if n.fieldMetadata != nil {
		stats.nodeFieldMetadata++
	}
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
	fieldIDs := n.fieldIDs()
	fieldSources := n.fieldSources()
	if len(fieldIDs) > 0 {
		stats.fieldIDElements += uint64(len(fieldIDs))
	}
	if len(fieldSources) > 0 {
		stats.fieldSourceElements += uint64(len(fieldSources))
	}
	if hasParentFieldMetadata(fieldIDs, fieldSources) {
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
	if t.resultCompatibilityFinalizer == nil {
		t.resultCompatibilityFinalizer = &treeResultCompatibilityFinalizer{}
	}
}

// treeResultCompatibilityFinalizer is installed while a tree is still parser-
// owned, before it can be observed by callers. Keeping the synchronization
// object behind a pointer avoids copying a used sync.Once when pooled Tree
// values are reset or when Tree.Copy constructs a finalized clone.
type treeResultCompatibilityFinalizer struct {
	once sync.Once
}

func (t *Tree) hasDeferredResultCompatibility() bool {
	return t != nil && t.resultCompatibilityFinalizer != nil
}

func (t *Tree) ensureResultCompatibility() {
	if t == nil {
		return
	}
	finalizer := t.resultCompatibilityFinalizer
	if finalizer == nil {
		return
	}
	finalizer.once.Do(func() {
		if t.root == nil || t.language == nil {
			return
		}
		if !parsePhaseTimingEnabled() {
			parser := &Parser{language: t.language}
			result := normalizeResultCompatibility(t.root, t.source, parser, nil)
			t.resultErrorSummary = result.errorSummary
			t.resultCompatibilityApplied = !parseStopReasonIsActive(result.stopReason)
			// Diagnostic-only, mirrors the timing-enabled branch below: without
			// this, every deferred-compatibility language (typescript/tsx by
			// default, see shouldDeferResultCompatibility) would
			// look UNCOVERED to the dispatcher-arm census
			// (GTS_DISPATCHER_CENSUS, parser_result_compat.go) even though its
			// normalizer just ran on the line above. parser.normalizationStats is
			// only ever non-empty when that census flag is set, so this is a
			// no-op field copy in every ordinary parse.
			parser.copyNormalizationStats(&t.parseRuntime)
			return
		}
		timing := &parseMaterializationTiming{}
		parser := &Parser{
			language:              t.language,
			materializationTiming: timing,
		}
		start := materializationTimingStart(timing)
		result := normalizeResultCompatibility(t.root, t.source, parser, nil)
		t.resultErrorSummary = result.errorSummary
		t.resultCompatibilityApplied = !parseStopReasonIsActive(result.stopReason)
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
	n.setFieldMetadata(fieldIDs, defaultFieldSourcesInArena(arena, fieldIDs))
	n.productionID = productionID
	n.dynamicPrecedence = nodeSliceDynamicPrecedence(children)
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
	n.rawShape = 0
	n.ownerArena = arena
	arena.leafNodesConstructed++
	workCountRecordLeafConstruction()
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
	resolvedFieldSources := fieldSources
	if resolvedFieldSources == nil {
		resolvedFieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	}
	n.setFieldMetadata(fieldIDs, resolvedFieldSources)
	n.productionID = productionID
	n.dynamicPrecedence = nodeSliceDynamicPrecedence(children)
	n.childIndex = -1
	arena.parentNodesConstructed++
	workCountRecordParentConstruction()
	if arena.breakdownEnabled {
		arena.recordParentNodeConstructedBreakdown(len(children), fieldIDs, resolvedFieldSources, fieldSources != nil, false, false)
	}
	populateParentNode(n, children)
	nodeInitEquivVersion(n)
	if arena.externalScannerCheckpointRecords > 0 {
		recordExternalScannerCheckpointForParent(n, children)
	}
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
	resolvedFieldSources := fieldSources
	if resolvedFieldSources == nil {
		resolvedFieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	}
	n.setFieldMetadata(fieldIDs, resolvedFieldSources)
	n.productionID = productionID
	n.dynamicPrecedence = nodeSliceDynamicPrecedence(children)
	n.childIndex = -1
	arena.parentNodesConstructed++
	workCountRecordParentConstruction()
	if arena.breakdownEnabled {
		arena.recordParentNodeConstructedBreakdown(len(children), fieldIDs, resolvedFieldSources, fieldSources != nil, true, trackChildErrors)
	}
	populateParentNodeNoLinks(n, children, trackChildErrors)
	nodeInitEquivVersion(n)
	if arena.externalScannerCheckpointRecords > 0 {
		recordExternalScannerCheckpointForParent(n, children)
	}
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
	workCountRecordParentConstruction()
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
	workCountRecordParentConstruction()
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

type resultErrorSummary uint8

const (
	resultErrorSummaryUnknown resultErrorSummary = iota
	resultErrorSummaryClean
	resultErrorSummaryPresent
)

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
	includedRanges                     []Range
	externalScannerCheckpointsDeferred bool
	forestFastPath                     bool
	incrementalReuseDisabled           bool
	// compactMaterialized marks a tree built by the phase-zero compact route
	// (parsercore_phase0_driver.go). Such a tree carries table-REPLAYED per-node
	// parser states (not the production parser's live-recorded ones): most are
	// exact, extras/unrecoverable nodes deliberately abstain to the 0
	// "unknown -> recompute" sentinel, and a bounded internal collapse class
	// carries the principled outer state rather than production's forest-selected
	// inner one. incrementalReuseDisabled remains set unless materialization
	// proves every required replay state and scanner quiescence for this exact
	// tree. Barred compact trees may still take an independently re-authenticated
	// single-leaf edit; all subtree reuse remains fail-closed.
	compactMaterialized bool
	// Finalization state avoids repeated retry scans and compatibility passes.
	// The error summary is not a persistent invariant of a caller-edited tree.
	resultErrorSummary           resultErrorSummary
	resultCompatibilityApplied   bool
	resultCompatibilityFinalizer *treeResultCompatibilityFinalizer
	released                     bool
	// Recovery-memo telemetry occupies the Tree's existing tail padding.
	recoveryNodeMemoPeakTier   RecoveryNodeMemoTier
	recoveryNodeMemoCollisions uint32
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
	resetTreeForReuse(tree, root, source, lang, arena, borrowed)
	if !tree.externalScannerCheckpointsDeferred {
		rebuildExternalScannerCheckpoints(root, lang)
	}
	return tree
}

// resetTreeForReuse is the deterministic scrub boundary for pooled Tree
// values. It is kept separate from sync.Pool acquisition so lifecycle tests can
// force the reset path without assuming that sync.Pool returns a given pointer.
func resetTreeForReuse(tree *Tree, root *Node, source []byte, lang *Language, arena *nodeArena, borrowed []*nodeArena) {
	if tree == nil {
		return
	}
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
	t.includedRanges = nil
	t.resultErrorSummary = resultErrorSummaryUnknown
	t.resultCompatibilityApplied = false
	t.resultCompatibilityFinalizer = nil
	t.recoveryNodeMemoPeakTier = RecoveryNodeMemoTierNone
	t.recoveryNodeMemoCollisions = 0
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
	if t == nil {
		return nil
	}
	t.ensureResultCompatibility()
	if t.root == nil {
		return nil
	}
	if offsetBytes == 0 && offsetExtent == (Point{}) {
		return t.root
	}
	return cloneTreeNodesWithOffset(t.root, offsetBytes, offsetExtent)
}

// Source returns the original source text.
func (t *Tree) Source() []byte { return t.source }

// UsedForestFastPath reports whether the node data behind this tree was
// produced by the GSS-forest GLR fast path (Parser.Parse trying
// tryForestFastPath before the production loop, or a direct
// ParseForestExperimental call) rather than the production parser. Unlike
// LanguageWantsForest (which only reports whether a language is eligible for
// the fast path), this reports what actually produced the data for a FRESH
// parse. It is provenance of the data, not necessarily of this exact call:
// reuseTreeWithNewSource (incremental_leaf_fastpath.go) copies the field from
// the old tree onto a new *Tree sharing the old root when reusing a tree
// incrementally, so an incrementally-reused tree reports whichever route
// produced the shared data, not whether this particular reuse call itself
// dispatched to forest (it did not run parser dispatch at all). Regression
// gates that always call Parser.Parse fresh (never ParseIncremental) are
// unaffected by this distinction; callers mixing in incremental reuse should
// not read this as "this call routed through forest."
func (t *Tree) UsedForestFastPath() bool {
	return t != nil && t.forestFastPath
}

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
	if t == nil {
		return nil
	}
	t.ensureResultCompatibility()
	if t.utf16Map == nil || t.root == nil || endCodeUnit < startCodeUnit {
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
	if t == nil {
		_, err := io.WriteString(w, "digraph gotreesitter {\n}\n")
		return err
	}
	t.ensureResultCompatibility()
	if t.root == nil {
		_, err := io.WriteString(w, "digraph gotreesitter {\n}\n")
		return err
	}

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
		source:                     t.source,
		sourceEncoding:             t.sourceEncoding,
		sourceUTF16:                t.sourceUTF16,
		utf16Map:                   t.utf16Map,
		language:                   t.language,
		parseRuntime:               t.parseRuntime,
		resultErrorSummary:         t.resultErrorSummary,
		resultCompatibilityApplied: t.resultCompatibilityApplied,
		// Reuse-provenance flags must survive Copy: cloneNodeHeaderInto keeps the
		// per-node stamped/replayed states, so a copy that dropped these would
		// become reuse-eligible on the standard DFA path and splice replayed or
		// abstained states into a live parse (Phase-3 Lane 3 review). Carry all
		// three: forestFastPath, incrementalReuseDisabled, and compactMaterialized.
		forestFastPath:           t.forestFastPath,
		incrementalReuseDisabled: t.incrementalReuseDisabled,
		compactMaterialized:      t.compactMaterialized,
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
	arena.inheritExternalScannerCheckpointIdentity(t.arena)
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

		if oldNode.ownerArena != nil && oldNode.ownerArena.finalChildRefs && cloneFinalChildRefsIntoArena(oldNode, newNode, arena, nil) {
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
	dst.errorRankCache = 0
	if offset != nil {
		dst.startByte = addUint32Delta(src.startByte, int64(offset.byteDelta))
		dst.endByte = addUint32Delta(src.endByte, int64(offset.byteDelta))
		dst.startPoint = offset.offsetPoint(src.startPoint)
		dst.endPoint = offset.offsetPoint(src.endPoint)
	}
	dst.children = nil
	dst.clearFieldMetadata()
	dst.parent = nil
	dst.childIndex = -1
	dst.ownerArena = arena
	if !copyMissingNodeDependency(dst, src, offset) {
		if _, present := missingNodeDependencyEntryForNode(src); present {
			dst.setDirty(true)
		}
	}
	if arena != nil && src.ownerArena != nil {
		arena.inheritExternalScannerCheckpointIdentity(src.ownerArena)
	}
	if src.ownerArena == nil || src.ownerArena.externalScannerCheckpointRecords == 0 {
		return
	}
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
	if dst == nil || src == nil {
		return
	}
	if src.fieldMetadata == nil {
		dst.clearFieldMetadata()
		return
	}
	fieldIDs := cloneFieldIDsIntoArena(arena, src.fieldMetadata.ids)
	fieldSources := cloneFieldSourcesIntoArena(arena, src.fieldMetadata.sources)
	dst.setFieldMetadata(fieldIDs, fieldSources)
}

func cloneFieldIDsIntoArena(arena *nodeArena, src []FieldID) []FieldID {
	if src == nil {
		return nil
	}
	if len(src) == 0 {
		return []FieldID{}
	}
	dst := arena.allocFieldIDSlice(len(src))
	copy(dst, src)
	return dst
}

func cloneFieldSourcesIntoArena(arena *nodeArena, src []uint8) []uint8 {
	if src == nil {
		return nil
	}
	if len(src) == 0 {
		return []uint8{}
	}
	dst := arena.allocFieldSourceSlice(len(src))
	copy(dst, src)
	return dst
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
	if dstArena != nil && srcArena != nil {
		dstArena.inheritExternalScannerCheckpointIdentity(srcArena)
	}
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
	dst := newPendingParentShellInArena(dstArena, src.symbol, src.isNamed(), src.productionID, childCount, src.startByte, src.endByte, src.startPoint, src.endPoint, src.hasError())
	dst.noTreeNode = src.noTreeNode
	dst.startPoint = src.startPoint
	dst.endPoint = src.endPoint
	applyCloneOffsetToPendingParent(dst, offset)
	dst.setHasFieldEntries(src.hasFieldEntries())
	dst.setHasDirectFieldEntries(src.hasDirectFieldEntries())
	for i := 0; i < childCount; i++ {
		dst.setChildEntry(dstArena, i, cloneStackEntryIntoArena(srcArena, dstArena, src.childEntry(srcArena, i), offset, metrics))
		if src.hasFieldEntries() || src.hasDirectFieldEntries() {
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
	t.ensureResultCompatibility()
	return t.rawParseStopReason()
}

// NodeAtByte returns the smallest root descendant that contains byteOffset.
func (t *Tree) NodeAtByte(byteOffset uint32) *Node {
	if t == nil {
		return nil
	}
	return t.RootNode().NodeAtByte(byteOffset)
}

// NamedNodeAtByte returns the smallest named root descendant that contains
// byteOffset.
func (t *Tree) NamedNodeAtByte(byteOffset uint32) *Node {
	if t == nil {
		return nil
	}
	return t.RootNode().NamedNodeAtByte(byteOffset)
}

// ParseStoppedEarly reports whether parsing hit an early-stop condition.
func (t *Tree) ParseStoppedEarly() bool {
	if t == nil {
		return false
	}
	t.ensureResultCompatibility()
	return t.rawParseStoppedEarly()
}

// rawParseStopReason reports the parser-captured stop reason without running
// deferred result compatibility. Parser-owned retry, selection, and
// normalization code must use this accessor until the tree is returned; public
// observers use ParseStopReason and join the synchronized finalization boundary.
func (t *Tree) rawParseStopReason() ParseStopReason {
	if t == nil || t.parseRuntime.StopReason == "" {
		return ParseStopNone
	}
	return t.parseRuntime.StopReason
}

// rawParseStoppedEarly is the parser-owned counterpart to ParseStoppedEarly.
// It intentionally does not initiate deferred result compatibility.
func (t *Tree) rawParseStoppedEarly() bool {
	switch t.rawParseStopReason() {
	case ParseStopIterationLimit, ParseStopStackDepthLimit, ParseStopNodeLimit, ParseStopMemoryBudget, ParseStopTokenSourceEOF, ParseStopTimeout, ParseStopCancelled, ParseStopInvariantViolation:
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
	t.ensureResultCompatibility()
	out := *t.rawParseRuntime()
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

// RecoveryNodeMemoRuntime returns bounded memo telemetry for this tree. The
// collision count saturates at the uint32 maximum.
func (t *Tree) RecoveryNodeMemoRuntime() RecoveryNodeMemoRuntime {
	if t == nil {
		return RecoveryNodeMemoRuntime{}
	}
	t.ensureResultCompatibility()
	return RecoveryNodeMemoRuntime{
		PeakTier:   t.recoveryNodeMemoPeakTier,
		Collisions: t.recoveryNodeMemoCollisions,
	}
}

// rawParseRuntime returns the parser-captured runtime record without running
// deferred result compatibility and without the public accessor's live arena
// counter overlay. Parser-owned decision helpers may use it only while the tree
// is alive and must not mutate the result.
func (t *Tree) rawParseRuntime() *ParseRuntime {
	if t == nil {
		return nil
	}
	return &t.parseRuntime
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

func (t *Tree) setRecoveryNodeMemoRuntime(rt RecoveryNodeMemoRuntime) {
	if t == nil {
		return
	}
	t.recoveryNodeMemoPeakTier = rt.PeakTier
	t.recoveryNodeMemoCollisions = rt.Collisions
}

func (t *Tree) setIncludedRanges(ranges []Range) {
	if t == nil {
		return
	}
	if len(ranges) == 0 {
		t.includedRanges = nil
		return
	}
	t.includedRanges = append(t.includedRanges[:0], ranges...)
}

func (t *Tree) setParseStopReason(reason ParseStopReason) {
	if t == nil || reason == "" {
		return
	}
	rt := *t.rawParseRuntime()
	rt.StopReason = reason
	t.setParseRuntime(rt)
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

func subtractPointDelta(a, b Point) Point {
	if a.Row > b.Row {
		return Point{Row: a.Row - b.Row, Column: a.Column}
	}
	column := uint32(0)
	if a.Column >= b.Column {
		column = a.Column - b.Column
	}
	return Point{Column: column}
}

func (t *Tree) editIncludedRanges(edit InputEdit) {
	if t == nil {
		return
	}
	for i := range t.includedRanges {
		range_ := &t.includedRanges[i]
		if range_.EndByte >= edit.OldEndByte {
			if range_.EndByte != ^uint32(0) {
				range_.EndByte = edit.NewEndByte + (range_.EndByte - edit.OldEndByte)
				range_.EndPoint = addPointDelta(edit.NewEndPoint,
					subtractPointDelta(range_.EndPoint, edit.OldEndPoint))
				if range_.EndByte < edit.NewEndByte {
					range_.EndByte = ^uint32(0)
					range_.EndPoint = Point{Row: ^uint32(0), Column: ^uint32(0)}
				}
			}
		} else if range_.EndByte > edit.StartByte {
			range_.EndByte = edit.StartByte
			range_.EndPoint = edit.StartPoint
		}
		if range_.StartByte >= edit.OldEndByte {
			range_.StartByte = edit.NewEndByte + (range_.StartByte - edit.OldEndByte)
			range_.StartPoint = addPointDelta(edit.NewEndPoint,
				subtractPointDelta(range_.StartPoint, edit.OldEndPoint))
			if range_.StartByte < edit.NewEndByte {
				range_.StartByte = ^uint32(0)
				range_.StartPoint = Point{Row: ^uint32(0), Column: ^uint32(0)}
			}
		} else if range_.StartByte > edit.StartByte {
			range_.StartByte = edit.StartByte
			range_.StartPoint = edit.StartPoint
		}
	}
}

func inputEditIsSingleByteReplacement(edit InputEdit) bool {
	return edit.NewEndByte == edit.OldEndByte &&
		edit.OldEndByte > edit.StartByte &&
		edit.OldEndByte-edit.StartByte == 1 &&
		edit.NewEndPoint == edit.OldEndPoint
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
	t.editIncludedRanges(edit)
	if t.root != nil {
		if inputEditIsSingleByteReplacement(edit) {
			editNodeSingleByteReplacement(t.root, edit, &t.lastEditedLeaf)
			return
		}
		byteDelta := int64(edit.NewEndByte) - int64(edit.OldEndByte)
		rowDelta := int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row)
		hasTailShift := byteDelta != 0 || edit.NewEndPoint != edit.OldEndPoint
		var shiftScratch []*Node
		editNodeWithDelta(t.root, edit, byteDelta, rowDelta, hasTailShift, &shiftScratch, &t.lastEditedLeaf)
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
	hasTailShift := byteDelta != 0 || edit.NewEndPoint != edit.OldEndPoint
	var shiftScratch []*Node
	editNodeWithDelta(n, edit, byteDelta, rowDelta, hasTailShift, &shiftScratch, nil)
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

// editNodeSingleByteReplacement marks the affected path without recomputing unchanged spans.
func editNodeSingleByteReplacement(n *Node, edit InputEdit, leafHint **Node) {
	if editMissingNodeDependency(n, edit, 0, 0) {
		if leafHint != nil {
			*leafHint = n
		}
		return
	}
	if missingNodeDependencyNoopAtEnd(n, edit) {
		return
	}
	if nodeEndsBeforeEditDependency(n, edit.StartByte) || n.startByte >= edit.OldEndByte {
		return
	}
	if nodeHasFinalChildRefs(n) {
		var shiftScratch []*Node
		editNodeWithDelta(n, edit, 0, 0, false, &shiftScratch, leafHint)
		return
	}

	n.setDirty(true)
	if perfCountersEnabled {
		perfRecordNodeEditMarked()
	}

	descended := false
	for _, child := range n.children {
		if nodeEndsBeforeEditDependency(child, edit.StartByte) {
			continue
		}
		if child.startByte >= edit.OldEndByte {
			break
		}
		descended = true
		editNodeSingleByteReplacement(child, edit, leafHint)
	}
	if leafHint != nil && !descended && len(n.children) == 0 {
		*leafHint = n
	}
}

func editNodeWithDelta(n *Node, edit InputEdit, byteDelta, rowDelta int64, hasTailShift bool, shiftScratch *[]*Node, leafHint **Node) {
	if editMissingNodeDependency(n, edit, byteDelta, rowDelta) {
		if leafHint != nil {
			*leafHint = n
		}
		return
	}
	if missingNodeDependencyNoopAtEnd(n, edit) {
		return
	}
	// If the node ends before the edit starts, it's completely unaffected.
	if nodeEndsBeforeEditDependency(n, edit.StartByte) {
		return
	}

	// If the node starts after the old edit end, shift its offsets.
	if n.startByte >= edit.OldEndByte {
		if !hasTailShift {
			return
		}
		dependency, hasMissingDependency := missingNodeDependencyForNode(n)
		n.startByte = addUint32Delta(n.startByte, byteDelta)
		n.endByte = addUint32Delta(n.endByte, byteDelta)
		n.startPoint = shiftPointAfterEdit(n.startPoint, edit, rowDelta)
		n.endPoint = shiftPointAfterEdit(n.endPoint, edit, rowDelta)
		if hasMissingDependency {
			dependency.stackByte = addUint32Delta(dependency.stackByte, byteDelta)
			dependency.stackPoint = shiftPointAfterEdit(dependency.stackPoint, edit, rowDelta)
			if !n.ownerArena.setMissingNodeDependency(n, dependency) {
				n.setDirty(true)
			}
		}
		shiftNodeChildrenAfterEdit(n, edit, byteDelta, rowDelta, shiftScratch)
		return
	}

	// The node overlaps the edit — mark it dirty and adjust its end.
	n.setDirty(true)
	if perfCountersEnabled {
		perfRecordNodeEditMarked()
	}
	if n.startByte > edit.StartByte {
		n.startByte = edit.NewEndByte
		n.startPoint = edit.NewEndPoint
	}
	if n.endByte <= edit.OldEndByte {
		// Node is fully within the edited region.
		n.endByte = edit.NewEndByte
		n.endPoint = edit.NewEndPoint
	} else {
		// Node extends past the edit — adjust end.
		n.endByte = addUint32Delta(n.endByte, byteDelta)
		n.endPoint = shiftPointAfterEdit(n.endPoint, edit, rowDelta)
	}

	// Recurse only into children that can be affected.
	descended := false
	childCount := nodeChildCountNoMaterialize(n)
	if !nodeHasFinalChildRefs(n) {
		for _, c := range n.children {
			if nodeEndsBeforeEditDependency(c, edit.StartByte) {
				continue
			}
			if c.startByte >= edit.OldEndByte {
				if !hasTailShift {
					break
				}
				shiftSubtreeNodeAfterEdit(c, edit, byteDelta, rowDelta, shiftScratch)
				continue
			}
			descended = true
			editNodeWithDelta(c, edit, byteDelta, rowDelta, hasTailShift, shiftScratch, leafHint)
		}
	} else {
		for i := 0; i < childCount; i++ {
			entry, ok := nodeChildEntryAtNoMaterialize(n, i)
			if ok && perfCountersEnabled {
				perfRecordNodeEditCompactRef()
			}
			if !ok || stackEntryEndsBeforeEditDependency(n.ownerArena, entry, edit.StartByte) {
				continue
			}
			if stackEntryNodeStartByte(entry) >= edit.OldEndByte {
				if !hasTailShift {
					break
				}
				shiftStackEntrySubtreeAfterEdit(n.ownerArena, entry, edit, byteDelta, rowDelta)
				continue
			}
			descended = true
			editStackEntryWithDelta(n.ownerArena, entry, edit, byteDelta, rowDelta, hasTailShift, shiftScratch, leafHint)
		}
	}
	if leafHint != nil && !descended && childCount == 0 {
		*leafHint = n
	}
}

func editStackEntryWithDelta(arena *nodeArena, entry stackEntry, edit InputEdit, byteDelta, rowDelta int64, hasTailShift bool, shiftScratch *[]*Node, leafHint **Node) {
	if node := stackEntryNode(entry); node != nil {
		editNodeWithDelta(node, edit, byteDelta, rowDelta, hasTailShift, shiftScratch, leafHint)
		return
	}
	if !stackEntryHasNode(entry) || stackEntryEndsBeforeEditDependency(arena, entry, edit.StartByte) {
		return
	}
	if stackEntryNodeStartByte(entry) >= edit.OldEndByte {
		if hasTailShift {
			shiftStackEntrySubtreeAfterEdit(arena, entry, edit, byteDelta, rowDelta)
		}
		return
	}

	setStackEntryDirty(entry, true)
	if perfCountersEnabled {
		perfRecordNodeEditMarked()
	}
	if stackEntryNodeStartByte(entry) > edit.StartByte {
		setStackEntryStart(entry, edit.NewEndByte, edit.NewEndPoint)
	}
	if stackEntryNodeEndByte(entry) <= edit.OldEndByte {
		setStackEntryEnd(entry, edit.NewEndByte, edit.NewEndPoint)
	} else {
		setStackEntryEnd(entry,
			addUint32Delta(stackEntryNodeEndByte(entry), byteDelta),
			shiftPointAfterEdit(stackEntryNodeEndPoint(entry), edit, rowDelta))
	}

	parent := stackEntryPendingParent(entry)
	if parent == nil {
		return
	}
	childCount := parent.childEntryCount()
	for i := 0; i < childCount; i++ {
		child := parent.childEntry(arena, i)
		if !stackEntryHasNode(child) || stackEntryEndsBeforeEditDependency(arena, child, edit.StartByte) {
			continue
		}
		if stackEntryNodeStartByte(child) >= edit.OldEndByte {
			if !hasTailShift {
				break
			}
			shiftStackEntrySubtreeAfterEdit(arena, child, edit, byteDelta, rowDelta)
			continue
		}
		if perfCountersEnabled {
			perfRecordNodeEditCompactRef()
		}
		editStackEntryWithDelta(arena, child, edit, byteDelta, rowDelta, hasTailShift, shiftScratch, leafHint)
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

func setStackEntryStart(entry stackEntry, startByte uint32, startPoint Point) {
	if node := stackEntryNode(entry); node != nil {
		node.startByte = startByte
		node.startPoint = startPoint
		return
	}
	if node := stackEntryNoTreeNode(entry); node != nil {
		node.startByte = startByte
		return
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		leaf.startByte = startByte
		leaf.startPoint = startPoint
		return
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		parent.startByte = startByte
		parent.startPoint = startPoint
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

func shiftNodeChildrenAfterEdit(parent *Node, edit InputEdit, byteDelta, rowDelta int64, shiftScratch *[]*Node) {
	childCount := nodeChildCountNoMaterialize(parent)
	if childCount == 0 {
		return
	}
	if !nodeHasFinalChildRefs(parent) {
		shiftSubtreeAfterEdit(parent.children, edit, byteDelta, rowDelta, shiftScratch)
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
		shiftStackEntrySubtreeAfterEdit(parent.ownerArena, entry, edit, byteDelta, rowDelta)
	}
}

func shiftSubtreeAfterEdit(roots []*Node, edit InputEdit, byteDelta, rowDelta int64, shiftScratch *[]*Node) {
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
		dependency, hasMissingDependency := missingNodeDependencyForNode(n)

		n.startByte = addUint32Delta(n.startByte, byteDelta)
		n.endByte = addUint32Delta(n.endByte, byteDelta)

		n.startPoint = shiftPointAfterEdit(n.startPoint, edit, rowDelta)
		n.endPoint = shiftPointAfterEdit(n.endPoint, edit, rowDelta)
		if hasMissingDependency {
			dependency.stackByte = addUint32Delta(dependency.stackByte, byteDelta)
			dependency.stackPoint = shiftPointAfterEdit(dependency.stackPoint, edit, rowDelta)
			if !n.ownerArena.setMissingNodeDependency(n, dependency) {
				n.setDirty(true)
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
				shiftStackEntrySubtreeAfterEdit(n.ownerArena, entry, edit, byteDelta, rowDelta)
			}
		}
	}
	if shiftScratch != nil {
		*shiftScratch = stack[:0]
	}
}

func shiftSubtreeNodeAfterEdit(root *Node, edit InputEdit, byteDelta, rowDelta int64, shiftScratch *[]*Node) {
	if root == nil {
		return
	}
	var roots [1]*Node
	roots[0] = root
	shiftSubtreeAfterEdit(roots[:], edit, byteDelta, rowDelta, shiftScratch)
}

func shiftStackEntrySubtreeAfterEdit(arena *nodeArena, entry stackEntry, edit InputEdit, byteDelta, rowDelta int64) {
	if node := stackEntryNode(entry); node != nil {
		shiftSubtreeNodeAfterEdit(node, edit, byteDelta, rowDelta, nil)
		return
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		shiftCompactFullLeafAfterEdit(leaf, edit, byteDelta, rowDelta)
		return
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		shiftPendingParentAfterEdit(arena, parent, edit, byteDelta, rowDelta)
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

func shiftCompactFullLeafAfterEdit(n *compactFullLeaf, edit InputEdit, byteDelta, rowDelta int64) {
	if n == nil {
		return
	}
	shiftNoTreeNodeAfterEdit(&n.noTreeNode, byteDelta)
	n.startPoint = shiftPointAfterEdit(n.startPoint, edit, rowDelta)
	n.endPoint = shiftPointAfterEdit(n.endPoint, edit, rowDelta)
}

func shiftPendingParentAfterEdit(arena *nodeArena, n *pendingParent, edit InputEdit, byteDelta, rowDelta int64) {
	if n == nil {
		return
	}
	shiftNoTreeNodeAfterEdit(&n.noTreeNode, byteDelta)
	n.startPoint = shiftPointAfterEdit(n.startPoint, edit, rowDelta)
	n.endPoint = shiftPointAfterEdit(n.endPoint, edit, rowDelta)
	childCount := n.childEntryCount()
	for i := 0; i < childCount; i++ {
		child := n.childEntry(arena, i)
		if !stackEntryHasNode(child) {
			continue
		}
		if perfCountersEnabled {
			perfRecordNodeEditCompactRef()
		}
		shiftStackEntrySubtreeAfterEdit(arena, child, edit, byteDelta, rowDelta)
	}
}

func shiftPointAfterEdit(p Point, edit InputEdit, rowDelta int64) Point {
	if p.Row < edit.OldEndPoint.Row {
		return p
	}
	if p.Row > edit.OldEndPoint.Row {
		p.Row = addUint32Delta(p.Row, rowDelta)
		return p
	}
	p.Row = addUint32Delta(p.Row, rowDelta)
	columnDelta := uint32(0)
	if p.Column >= edit.OldEndPoint.Column {
		columnDelta = p.Column - edit.OldEndPoint.Column
	}
	p.Column = addUint32Delta(edit.NewEndPoint.Column, int64(columnDelta))
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
