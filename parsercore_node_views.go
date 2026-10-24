//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/odvcencio/gotreesitter/internal/incr"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

const compactViewPageSize = 128

type compactViewPage [compactViewPageSize]atomic.Pointer[Node]

// Reuse dependencies belong only to eligible nodes near the root. Preserve
// their exact occurrence IDs without reserving one word for every terminal.
type nativeViewDependency struct {
	id      core.SelectedNodeID
	encoded uint32
}

// nativeNodeViews owns no parser, scheduler, or callback into a producer.
// Published scalar node headers never change during reads. The cache lock
// protects the arena allocator; atomic slots publish fully initialized views.
type nativeNodeViews struct {
	mu               sync.Mutex
	snapshot         core.NativeSelectedSnapshot
	arena            *nodeArena
	points           diagnosticParserCorePointIndex
	pages            []atomic.Pointer[compactViewPage]
	checkpoints      []externalScannerCheckpointRef
	lookahead        []uint32
	dependencies     []nativeViewDependency
	scannerProven    bool
	reuseProven      bool
	accounted        bool
	pageBytes        uint64
	borrowedSnapshot bool
	transientBytes   uint64
}

func (v *nativeNodeViews) get(id core.SelectedNodeID) *Node {
	if id == 0 || uint64(id) > v.snapshot.Store().NodeCount() {
		return nil
	}
	index := int(id) - 1
	if page := v.pages[index/compactViewPageSize].Load(); page != nil {
		if n := page[index%compactViewPageSize].Load(); n != nil {
			return n
		}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.getLocked(id)
}
func (v *nativeNodeViews) getLocked(id core.SelectedNodeID) *Node {
	if id == 0 {
		return nil
	}
	index := int(id) - 1
	page := v.pages[index/compactViewPageSize].Load()
	if page == nil {
		page = new(compactViewPage)
		v.pageBytes += uint64(unsafe.Sizeof(*page))
		if v.accounted {
			v.arena.allocatedBytes += int64(unsafe.Sizeof(*page))
		}
		v.pages[index/compactViewPageSize].Store(page)
	}
	if n := page[index%compactViewPageSize].Load(); n != nil {
		return n
	}
	r, _, ok := v.snapshot.Record(id)
	if !ok {
		return nil
	}
	n := v.arena.allocNodeFast()
	*n = Node{ownerArena: v.arena, rawShape: rawShapeRef(id), symbol: Symbol(r.Symbol), productionID: r.ProductionID,
		startByte: r.StartByte, endByte: r.EndByte, startPoint: v.points.point(r.StartByte), endPoint: v.points.point(r.EndByte),
		dynamicPrecedence: r.DynamicPrecedence, childIndex: compactViewChildIndex}
	n.setNamed(r.Named())
	n.setExtra(r.Extra())
	n.setMissing(r.Missing())
	n.setHasError(r.HasError())
	n.setExternalScannerToken(r.External())
	n.setCompactMaterialized(true)
	n.setCompactParseStateProof(r.ReplayStateKnown())
	n.setCompactPreGotoStateProof(r.ReplayPreKnown())
	n.setFragileLeft(r.Fragile())
	n.setFragileRight(r.Fragile())
	if r.ReplayStateKnown() {
		n.parseState = StateID(r.ReplayState)
	}
	if r.ReplayPreKnown() {
		n.preGotoState = StateID(r.ReplayPre)
	}
	if !r.Terminal() {
		nodeInitEquivVersion(n)
	}
	var fields []FieldID
	for i := uint32(0); i < r.ChildCount; i++ {
		child, _ := v.snapshot.Store().ChildID(id, i)
		field, _ := v.snapshot.Store().NodeField(child)
		if field != 0 {
			if fields == nil {
				fields = v.arena.allocFieldIDSlice(int(r.ChildCount))
			}
			fields[i] = FieldID(field)
		}
	}
	if fields != nil {
		n.setFieldMetadata(fields, defaultFieldSourcesInArena(v.arena, fields))
	}
	page[index%compactViewPageSize].Store(n)
	return n
}
func (v *nativeNodeViews) childCount(n *Node) int {
	count, _ := v.snapshot.Store().NodeChildCount(core.SelectedNodeID(n.rawShape))
	return int(count)
}
func (v *nativeNodeViews) child(n *Node, index int) *Node {
	if index < 0 || uint64(index) >= 1<<16 {
		return nil
	}
	id, ok := v.snapshot.Store().ChildID(core.SelectedNodeID(n.rawShape), uint32(index))
	if !ok {
		return nil
	}
	return v.get(id)
}
func (v *nativeNodeViews) children(n *Node) []*Node {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.childrenLocked(n)
}
func (v *nativeNodeViews) childrenLocked(n *Node) []*Node {
	if n.children != nil {
		return n.children
	}
	id := core.SelectedNodeID(n.rawShape)
	count, _ := v.snapshot.Store().NodeChildCount(id)
	if count == 0 {
		return nil
	}
	children := v.arena.allocNodeSlice(int(count))
	for i := range children {
		child, _ := v.snapshot.Store().ChildID(id, uint32(i))
		children[i] = v.getLocked(child)
	}
	n.children = children
	return children
}
func (v *nativeNodeViews) parent(n *Node) (*Node, int, bool) {
	r, ok := v.snapshot.Store().Record(core.SelectedNodeID(n.rawShape))
	if !ok || r.Parent == 0 {
		return nil, -1, false
	}
	return v.get(r.Parent), int(r.ChildIndex), true
}
func (v *nativeNodeViews) supertype(n *Node) uint32 {
	r, _ := v.snapshot.Store().Record(core.SelectedNodeID(n.rawShape))
	return r.SupertypeMask
}

// Mutating a tree requires exclusive access, as with ordinary arena trees.
// Restore all parser metadata before releasing the compact backing. Existing
// pointers remain valid; only their representation changes.
func (v *nativeNodeViews) materialize() {
	v.mu.Lock()
	if v.snapshot.Store() == nil {
		v.mu.Unlock()
		return
	}
	store := v.snapshot.Store()
	reserveNodeViewMaterialization(v.arena, int(store.NodeCount()))
	v.reserveMaterializationPages()
	for id := core.SelectedNodeID(1); uint64(id) <= store.NodeCount(); id++ {
		n := v.getLocked(id)
		v.childrenLocked(n)
	}
	// Install sidecars before clearing occurrence IDs. They use the final arena
	// addresses, not producer allocation order or raw subtree identities.
	dependency := 0
	for id := core.SelectedNodeID(1); uint64(id) <= store.NodeCount(); id++ {
		n := v.getLocked(id)
		r, _ := store.Record(id)
		n.parent = v.getLocked(r.Parent)
		n.addSupertypeMask(v.arena, r.SupertypeMask)
		if len(v.checkpoints) != 0 && externalScannerCheckpointRefComplete(v.checkpoints[id-1]) {
			v.arena.setExternalScannerCheckpoint(n, v.checkpoints[id-1])
		}
		if len(v.lookahead) != 0 && v.lookahead[id-1] != 0 {
			if word := legacyReuseWord(n, true); word != nil {
				*word = v.lookahead[id-1]
			}
		}
		if dependency < len(v.dependencies) && v.dependencies[dependency].id == id {
			setCompactReuseDependency(n, v.dependencies[dependency].encoded-1)
			dependency++
		}
	}
	for index := range v.pages {
		if page := v.pages[index].Load(); page != nil {
			for i := range page {
				if n := page[i].Load(); n != nil {
					r, _ := store.Record(core.SelectedNodeID(n.rawShape))
					n.childIndex = int32(r.ChildIndex)
					if r.Parent == 0 {
						n.childIndex = -1
					}
					n.rawShape = 0
				}
			}
		}
	}
	v.arena.views = nil
	// Return to the detached cache only after dropping the allocation lock.
	v.mu.Unlock()
	v.release()
}

// An edit commits to the complete tree. Allocate the remaining cache pages in
// one block; sparse reads still allocate only the pages they actually visit.
// Each installed page pointer keeps its block alive, including across reuse.
func (v *nativeNodeViews) reserveMaterializationPages() {
	missing := 0
	for i := range v.pages {
		if v.pages[i].Load() == nil {
			missing++
		}
	}
	if missing == 0 {
		return
	}
	pages := make([]compactViewPage, missing)
	bytes := uint64(missing) * uint64(unsafe.Sizeof(compactViewPage{}))
	v.pageBytes += bytes
	if v.accounted {
		v.arena.allocatedBytes += int64(bytes)
	}
	next := 0
	for i := range v.pages {
		if v.pages[i].Load() == nil {
			v.pages[i].Store(&pages[next])
			next++
		}
	}
}

func (v *nativeNodeViews) release() {
	if v.accounted {
		v.arena.allocatedBytes -= int64(v.retainedBytes())
		v.accounted = false
	}
	borrowed := v.borrowedSnapshot
	if !borrowed {
		v.snapshot.Release()
	}
	v.snapshot = core.NativeSelectedSnapshot{}
	v.arena = nil
	if borrowed {
		return
	}
	for index := 0; index < cap(v.pages); index++ {
		if p := v.pages[:cap(v.pages)][index].Load(); p != nil {
			for i := range p {
				p[i].Store(nil)
			}
		}
	}
	v.points = diagnosticParserCorePointIndex{lineStarts: v.points.lineStarts[:0]}
	clear(v.checkpoints)
	clear(v.lookahead)
	clear(v.dependencies)
	v.checkpoints = v.checkpoints[:0]
	v.lookahead = v.lookahead[:0]
	v.dependencies = v.dependencies[:0]
	v.pages = v.pages[:0]
	v.scannerProven, v.reuseProven = false, false
	v.transientBytes = 0
	recycleNodeViewBacking(v)
}
func (v *nativeNodeViews) retainedBytes() uint64 {
	return uint64(unsafe.Sizeof(*v)) + v.pageBytes + v.transientBytes + v.snapshot.RetainedBytes() + uint64(cap(v.points.lineStarts))*4 +
		uint64(cap(v.pages))*uint64(unsafe.Sizeof(atomic.Pointer[compactViewPage]{})) +
		uint64(cap(v.checkpoints))*uint64(unsafe.Sizeof(externalScannerCheckpointRef{})) +
		uint64(cap(v.lookahead))*4 + uint64(cap(v.dependencies))*uint64(unsafe.Sizeof(nativeViewDependency{}))
}

// Capture cold facts while their authenticated producer still exists. No raw
// subtree ID is used to authenticate an edit after this function returns.
func (v *nativeNodeViews) capture(s *diagnosticParserCoreGenericScheduler, r *parserCoreFreshFullRunner, poll func() error) error {
	store := v.snapshot.Store()
	count := int(store.NodeCount())
	scanner := languageUsesExternalScannerCheckpoints(r.lang)
	if scanner {
		_, required, valid := externalScannerCheckpointIdentityStatus(r.lang)
		if required {
			v.scannerProven = v.scannerProven && valid && v.arena.setExternalScannerCheckpointIdentityForLanguage(r.lang)
		}
		if cap(v.checkpoints) < count {
			v.checkpoints = make([]externalScannerCheckpointRef, count)
		} else {
			v.checkpoints = v.checkpoints[:count]
		}
	}
	publish := s.reuseDependencies.reads != nil && !s.s3RegionOpened && !s.recoveryIsolation
	var parents, leaves incr.LookaheadCursor
	if publish {
		s.reuseDependencies.reads.Seal()
		parents, leaves = s.reuseDependencies.reads.Cursor(true), s.reuseDependencies.reads.Cursor(false)
		if cap(v.lookahead) < count {
			v.lookahead = make([]uint32, count)
		} else {
			v.lookahead = v.lookahead[:count]
		}
	}
	v.reuseProven = r.replayParseStates && compactIncrementalReuseProvenForLanguage(r.lang)
	dependencies := v.reuseProven && !s.reuseDependencies.disabled
	var checkpoints core.CheckpointCopyScratch
	defer func() { checkpoints.Reset(); v.transientBytes = 0 }()
	for id := core.SelectedNodeID(1); int(id) <= count; id++ {
		if id&255 == 0 {
			if err := poll(); err != nil {
				return err
			}
		}
		node, original, ok := v.snapshot.Record(id)
		if !ok {
			return fmt.Errorf("compact node view has no replay record")
		}
		if node.Terminal() {
			v.arena.leafNodesConstructed++
			workCountRecordLeafConstruction()
		} else {
			v.arena.parentNodesConstructed++
			workCountRecordParentConstruction()
		}
		if scanner {
			if node.Terminal() {
				raw, err := s.compact.MaterializationView(node.Source)
				if err != nil {
					return err
				}
				if raw.ExternalScannerCheckpointExact && raw.ExternalScannerCheckpointStart != 0 && raw.ExternalScannerCheckpointEnd != 0 {
					start, end, ok := checkpoints.CopyPair(s.compact, raw.ExternalScannerCheckpointStart, raw.ExternalScannerCheckpointEnd)
					v.transientBytes = checkpoints.RetainedBytes()
					if ok {
						v.checkpoints[id-1] = v.arena.recordExternalScannerCompactCheckpoint(start, end)
					}
				}
				if !externalScannerCheckpointRefComplete(v.checkpoints[id-1]) {
					v.scannerProven = false
				} else {
					v.arena.externalScannerCheckpointLeafNodes++
				}
			} else {
				var cp externalScannerCheckpointRef
				for i := uint32(0); i < node.ChildCount; i++ {
					child, _ := store.ChildID(id, i)
					if c := v.checkpoints[child-1]; externalScannerCheckpointRefComplete(c) {
						if cp.start.len == 0 {
							cp.start = c.start
						}
						cp.end = c.end
					}
				}
				v.checkpoints[id-1] = cp
			}
			if externalScannerCheckpointRefComplete(v.checkpoints[id-1]) {
				v.arena.externalScannerCheckpointRecords++
			}
		}
		stateProven := node.ReplayStateKnown() && (node.ChildCount == 0 || node.ReplayPreKnown())
		if id != store.Root() && !node.Extra() && !node.HasError() && !node.Missing() && !node.Fragile() && !stateProven {
			v.reuseProven = false
		}
		geometry, compatible, err := s.compact.NativeProjectionReceiptGeometry(node.Source, node.Symbol, node.ChildCount == 0, original == node.Symbol)
		if err != nil || !compatible || geometry.StartByte != node.StartByte || geometry.EndByte != node.EndByte || !stateProven {
			continue
		}
		if publish {
			var count uint32
			var ok bool
			if node.ChildCount == 0 {
				count, ok = leaves.Lookahead(node.EndByte)
			} else {
				count, ok = parents.Lookahead(node.EndByte)
			}
			if encoded := incr.Encode(count); ok && encoded != 0 && encoded <= legacyReuseCountMask {
				v.lookahead[id-1] = encoded
				if node.ChildCount == 0 {
					v.lookahead[id-1] |= incr.LeafReceipt(s.reuseDependencies.leafWords, uint32(node.Source))
				}
			}
		}
		if !dependencies || node.ChildCount == 0 || node.EndByte <= node.StartByte || node.Extra() || node.HasError() || node.Fragile() || uint32(node.Symbol) < r.lang.TokenCount || !r.parser.isVisibleSymbol(Symbol(node.Symbol)) {
			continue
		}
		rec, _ := store.Record(id)
		parent, _ := store.Record(rec.Parent)
		if rec.Parent != store.Root() && parent.Parent != store.Root() {
			continue
		}
		if uint64(node.Source) < uint64(len(s.reuseDependencies.ends)) && s.reuseDependencies.ends[node.Source] >= node.EndByte {
			v.dependencies = append(v.dependencies, nativeViewDependency{id: id, encoded: s.reuseDependencies.ends[node.Source] - node.EndByte + 1})
		}
	}
	v.reuseProven = v.reuseProven && v.scannerProven
	return poll()
}

func checkedNativeNodeViews(source []byte, scratch *nativeProjectionWorkspace, s *diagnosticParserCoreGenericScheduler, r *parserCoreFreshFullRunner) (tree *Tree, err error) {
	defer r.scratch.resetTreeBuffers()
	arena := acquireNodeViewArena()
	v, _ := takeNodeViewBacking().(*nativeNodeViews)
	if v == nil {
		v = &nativeNodeViews{}
	}
	v.arena, v.scannerProven = arena, true
	defer func() {
		if err != nil {
			v.release()
			arena.views = nil
			arena.Release()
		}
	}()
	poll := func() error {
		if reason := r.parser.activeParseStopReason(); resultMaterializationShouldStop(reason) {
			return diagnosticParserCoreStopControlTripped(reason)
		}
		additional := arenaAllocatedVolume(arena) + s.compact.SelectedBuildAuxiliaryBytes()
		if !v.accounted {
			additional += v.retainedBytes()
		}
		if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes(additional); resultMaterializationShouldStop(reason) {
			return diagnosticParserCoreStopControlTripped(reason)
		}
		return nil
	}
	if err = poll(); err != nil {
		return nil, err
	}
	v.points, err = newDiagnosticParserCorePointIndexInto(source, poll, v.points.lineStarts)
	if err != nil {
		return nil, err
	}
	replay := core.NativeProjectionReplay{CheckpointsRequired: languageUsesExternalScannerCheckpoints(r.lang), Scratch: &scratch.postorder,
		RootPre: core.StateID(r.parser.replayRootPreGotoState()), NativeTransition: r.tables,
		Tiling: &core.NativeProjectionTiling{RootSymbol: core.Symbol(r.parser.rootSymbol), HasRootSymbol: r.parser.hasRootSymbol, GapIsTolerated: diagnosticParserCoreGapIsTolerated}}
	if replay.CheckpointsRequired {
		replay.VisitTerminal = func(_ core.SubtreeID, raw *core.MaterializationSubtreeView) error {
			if !raw.ExternalScannerCheckpointExact || raw.ExternalScannerCheckpointStart == 0 || raw.ExternalScannerCheckpointEnd == 0 {
				v.scannerProven = false
			}
			return nil
		}
	}
	v.snapshot, err = s.compact.BuildNativeSelectedSnapshot(s.acceptedPayloads, source, replay, &scratch.output, poll)
	if err != nil {
		return nil, err
	}
	store := v.snapshot.Store()
	rootRecord, complete := store.Record(store.Root())
	// Match native public framing: leading trivia can precede the grammar
	// root, and its C coordinates must survive publication. A non-trivia gap
	// or incomplete EOF still rejects the result.
	if !complete || rootRecord.EndByte != uint32(len(source)) || rootRecord.StartByte > firstNonTriviaByteStart(source) {
		return nil, fmt.Errorf("compact public root has an unexplained source gap: %d..%d source=%d", rootRecord.StartByte, rootRecord.EndByte, len(source))
	}
	count := int(store.NodeCount())
	// Charge all optional receipt planes and the cache directory before allocation.
	arena.viewNodeLimit = count
	reserve := uint64(count)*(uint64(unsafe.Sizeof(externalScannerCheckpointRef{}))+4+uint64(unsafe.Sizeof(nativeViewDependency{}))) + uint64((count+compactViewPageSize-1)/compactViewPageSize)*8
	if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes(arenaAllocatedVolume(arena) + v.retainedBytes() + reserve); resultMaterializationShouldStop(reason) {
		return nil, diagnosticParserCoreStopControlTripped(reason)
	}
	pageCount := (count + compactViewPageSize - 1) / compactViewPageSize
	if cap(v.pages) < pageCount {
		pages := make([]atomic.Pointer[compactViewPage], pageCount)
		for i := 0; i < cap(v.pages); i++ {
			pages[i].Store(v.pages[:cap(v.pages)][i].Load())
		}
		v.pages = pages
	} else {
		v.pages = v.pages[:pageCount]
	}
	if err = v.capture(s, r, poll); err != nil {
		return nil, err
	}
	arena.views = v
	arena.allocatedBytes += int64(v.retainedBytes())
	v.accounted = true
	root := v.get(store.Root())
	if err = poll(); err != nil {
		return nil, err
	}
	tree = newTreeWithArenas(root, source, r.lang, arena, nil)
	tree.resultCompatibilityApplied = true
	tree.compactMaterialized = true
	tree.incrementalReuseDisabled = !v.reuseProven
	tree.incrementalReuseUnsupportedClause = compactIncrementalReuseClauseScanner
	if !r.replayParseStates {
		tree.incrementalReuseUnsupportedClause = compactIncrementalReuseClauseReplay
	}
	tree.setParseRuntime(ParseRuntime{StopReason: ParseStopAccepted, NodesAllocated: count, SourceLen: uint32(len(source)), ExpectedEOFByte: uint32(len(source)), RootEndByte: root.endByte, LastTokenEndByte: uint32(len(source)), LastTokenWasEOF: true,
		ExternalScannerCheckpointRecords: arena.externalScannerCheckpointRecords, ExternalScannerCheckpointSlotsAllocated: arena.externalScannerCheckpointSlotsAllocated(), ExternalScannerCheckpointBytesAllocated: arena.externalScannerCheckpointBytesAllocated(), ExternalScannerSnapshotBytesAllocated: arena.externalScannerSnapshotPayloadBytes, ExternalScannerCheckpointLeafNodes: arena.externalScannerCheckpointLeafNodes, CompactExternalScannerCheckpointTransferProven: v.scannerProven})
	if r.scratch.freshAttemptWork != nil {
		r.scratch.freshAttemptWork.allocatedNodes += uint64(count)
	}
	return tree, nil
}

func (v *nativeNodeViews) namedChildCount(n *Node) int {
	store := v.snapshot.Store()
	id := core.SelectedNodeID(n.rawShape)
	count, _ := store.NodeChildCount(id)
	named := 0
	for index := uint32(0); index < uint32(count); index++ {
		child, _ := store.ChildID(id, index)
		yes, _ := store.NodeNamed(child)
		if yes {
			named++
		}
	}
	return named
}
func (v *nativeNodeViews) namedChild(n *Node, index int) *Node {
	if index < 0 {
		return nil
	}
	store := v.snapshot.Store()
	id := core.SelectedNodeID(n.rawShape)
	count, _ := store.NodeChildCount(id)
	for i := uint32(0); i < uint32(count); i++ {
		child, _ := store.ChildID(id, i)
		yes, _ := store.NodeNamed(child)
		if yes {
			if index == 0 {
				return v.get(child)
			}
			index--
		}
	}
	return nil
}

// Match against indexed syntax and allocate public views only for captures.
// Query predicates, directives, ordering, and budgets use the shared matcher.
func (v *nativeNodeViews) executeQuery(q *Query, n *Node, lang *Language, source []byte, dst []QueryMatch) ([]QueryMatch, bool) {
	if !selectedStoreQuerySupports(q) {
		return dst, false
	}
	reader := &nativeViewQueryReader{selectedStoreQueryReader: selectedStoreQueryReader{store: v.snapshot.Store()}, views: v}
	matches, _ := executeQueryWithReader(q, core.SelectedNodeID(n.rawShape), lang, source, reader, nil, nil)
	for _, match := range matches {
		dst = append(dst, QueryMatch{PatternIndex: match.PatternIndex, Captures: match.Captures})
	}
	return dst, true
}

// The matcher walks occurrence IDs but writes final public captures directly.
// Converting an intermediate capture graph duplicates every result allocation.
type nativeViewQueryReader struct {
	selectedStoreQueryReader
	views *nativeNodeViews
}

func (r nativeViewQueryReader) NewCapture(name string, id core.SelectedNodeID) QueryCapture {
	return QueryCapture{Name: name, Node: r.views.get(id)}
}
func (nativeViewQueryReader) CaptureName(c QueryCapture) string { return c.Name }
func (nativeViewQueryReader) CaptureNode(c QueryCapture) core.SelectedNodeID {
	if c.Node == nil {
		return 0
	}
	return core.SelectedNodeID(c.Node.rawShape)
}
func (nativeViewQueryReader) CaptureTextOverride(c QueryCapture) string { return c.TextOverride }
func (nativeViewQueryReader) SetCaptureTextOverride(c *QueryCapture, text string) {
	c.TextOverride = text
}
func (nativeViewQueryReader) CaptureRangeOverride(c QueryCapture) (uint32, uint32, Point, Point, bool) {
	return publicQueryReader{}.CaptureRangeOverride(c)
}
func (nativeViewQueryReader) SetCaptureRangeOverride(c *QueryCapture, start, end uint32, startPoint, endPoint Point) {
	publicQueryReader{}.SetCaptureRangeOverride(c, start, end, startPoint, endPoint)
}

// Compare the full D8 witness without constructing a second public node graph.
// Every property checked by incrementalTreesStructurallyEqual remains checked,
// including source points and field names. A separate local point cache keeps
// this read independent of concurrent view creation.
func (v *nativeNodeViews) equalTree(left, right *Node, lang *Language) bool {
	if left == nil || right == nil {
		return left == right
	}
	other, _ := nodeViews(left).(*nativeNodeViews)
	rightPoints := diagnosticParserCorePointIndex{lineStarts: v.points.lineStarts}
	var leftPoints diagnosticParserCorePointIndex
	if other != nil {
		leftPoints.lineStarts = other.points.lineStarts
	}
	type pair struct {
		node        *Node
		left, right core.SelectedNodeID
	}
	initial := pair{node: left, right: core.SelectedNodeID(right.rawShape)}
	if other != nil {
		initial.left = core.SelectedNodeID(left.rawShape)
	}
	stack := []pair{initial}
	fieldName := func(id core.FieldID) string {
		if lang == nil || id == 0 || int(id) >= len(lang.FieldNames) {
			return ""
		}
		return lang.FieldNames[id]
	}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		b, ok := v.snapshot.Store().Record(current.right)
		if !ok {
			return false
		}
		start, end := rightPoints.point(b.StartByte), rightPoints.point(b.EndByte)
		if other != nil {
			a, ok := other.snapshot.Store().Record(current.left)
			if !ok || a.Symbol != b.Symbol || a.StartByte != b.StartByte || a.EndByte != b.EndByte ||
				leftPoints.point(a.StartByte) != start || leftPoints.point(a.EndByte) != end ||
				a.Named() != b.Named() || a.Extra() != b.Extra() || a.Missing() != b.Missing() || a.HasError() != b.HasError() || a.ChildCount != b.ChildCount {
				return false
			}
		} else {
			a := current.node
			if a == nil || a.Symbol() != Symbol(b.Symbol) || a.StartByte() != b.StartByte || a.EndByte() != b.EndByte || a.StartPoint() != start || a.EndPoint() != end ||
				a.IsNamed() != b.Named() || a.IsMissing() != b.Missing() || a.IsExtra() != b.Extra() || a.HasError() != b.HasError() || a.ChildCount() != int(b.ChildCount) {
				return false
			}
		}
		for i := int(b.ChildCount) - 1; i >= 0; i-- {
			child, _ := v.snapshot.Store().ChildID(current.right, uint32(i))
			field, _ := v.snapshot.Store().NodeField(child)
			next := pair{right: child}
			if other != nil {
				next.left, _ = other.snapshot.Store().ChildID(current.left, uint32(i))
				leftField, _ := other.snapshot.Store().NodeField(next.left)
				if fieldName(field) != fieldName(leftField) {
					return false
				}
			} else {
				if current.node.FieldNameForChild(i, lang) != fieldName(field) {
					return false
				}
				next.node = current.node.Child(i)
			}
			stack = append(stack, next)
		}
	}
	return true
}

// Copy is a read operation. Construct the destination from immutable records
// without converting or modifying the source views, including on concurrent
// Copy/Child/query calls. Only this synchronous adapter borrows the snapshot.
func (v *nativeNodeViews) copyTree(tree *Tree) *Tree {
	arena := acquireNodeArena(arenaClassFull)
	arena.inheritExternalScannerCheckpointIdentity(v.arena)
	arena.columnDependentSpans = append(arena.columnDependentSpans[:0], v.arena.columnDependentSpans...)
	arena.columnDependentSpansSorted = v.arena.columnDependentSpansSorted
	arena.dependsOnColumnRecords = v.arena.dependsOnColumnRecords
	copyViews := &nativeNodeViews{arena: arena, snapshot: v.snapshot, borrowedSnapshot: true,
		points: diagnosticParserCorePointIndex{lineStarts: v.points.lineStarts},
		pages:  make([]atomic.Pointer[compactViewPage], len(v.pages)), lookahead: v.lookahead, dependencies: v.dependencies}
	if len(v.checkpoints) != 0 {
		copyViews.checkpoints = make([]externalScannerCheckpointRef, len(v.checkpoints))
		for i, cp := range v.checkpoints {
			if externalScannerCheckpointRefComplete(cp) {
				copyViews.checkpoints[i] = externalScannerCheckpointRef{
					start: copyExternalScannerSnapshotRefBetweenArenas(v.arena, arena, cp.start),
					end:   copyExternalScannerSnapshotRefBetweenArenas(v.arena, arena, cp.end)}
			}
		}
		arena.externalScannerCheckpointRecords = v.arena.externalScannerCheckpointRecords
		arena.externalScannerCheckpointLeafNodes = v.arena.externalScannerCheckpointLeafNodes
	}
	arena.views = copyViews
	root := copyViews.get(v.snapshot.Store().Root())
	copyViews.materialize()
	out := *tree
	out.root, out.arena, out.borrowedArena = root, arena, nil
	out.parseRuntime = acquireParseRuntime()
	*out.parseRuntime = *tree.rawParseRuntime()
	out.arenaBreakdown = nil
	out.edits = append([]InputEdit(nil), tree.edits...)
	out.includedRanges = append([]Range(nil), tree.includedRanges...)
	out.lastEditedLeaf = nil
	out.resultCompatibilityFinalizer = nil
	out.extraHandles = 0
	out.released = false
	out.dependsOnColumnPropagated = false
	out.ensureDependsOnColumnPropagated()
	arena.legacyIncrementalReuseCertified = tree.hasCertifiedIncrementalReuse()
	arena.legacyNoPolicyPruning = tree.hasCertifiedUnprunedPolicy()
	arena.ownership.Publish(false)
	return &out
}
