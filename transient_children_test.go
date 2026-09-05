package gotreesitter

import (
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

type cancelAtEOFArithmeticTokenSource struct {
	cancel *uint32
	idx    int
}

func (s *cancelAtEOFArithmeticTokenSource) Next() Token {
	tokens := []Token{
		{Symbol: 1, StartByte: 0, EndByte: 1, StartPoint: Point{}, EndPoint: Point{Column: 1}},
		{Symbol: 2, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}},
		{Symbol: 1, StartByte: 2, EndByte: 3, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 3}},
	}
	if s.idx < len(tokens) {
		tok := tokens[s.idx]
		s.idx++
		return tok
	}
	if s.cancel != nil {
		atomic.StoreUint32(s.cancel, 1)
	}
	return Token{Symbol: 0, StartByte: 3, EndByte: 3, StartPoint: Point{Column: 3}, EndPoint: Point{Column: 3}}
}

func TestTransientChildScratchMaterializesReachableNode(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch transientChildScratch
	first := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	second := newLeafNodeInArena(arena, Symbol(2), true, 1, 2, Point{Column: 1}, Point{Column: 2})
	parent := newParentNodeInArenaNoLinksWithFieldSources(arena, Symbol(3), true, []*Node{}, nil, nil, 0, true)

	children := scratch.alloc(2)
	children[0] = first
	children[1] = second
	parent.children = children

	if !scratch.owns(parent.children) {
		t.Fatal("expected parent children to use transient storage before materialization")
	}

	var stack []*Node
	scratch.materializeNodeUntil(parent, arena, &stack, nil)

	if scratch.owns(parent.children) {
		t.Fatal("expected parent children to be copied into arena storage")
	}
	if len(parent.children) != 2 || parent.children[0] != first || parent.children[1] != second {
		t.Fatalf("materialized children = %#v, want [%p %p]", parent.children, first, second)
	}

	scratch.reset()
	if len(parent.children) != 2 || parent.children[0] != first || parent.children[1] != second {
		t.Fatal("materialized children were invalidated by transient reset")
	}
}

func TestTransientParentScratchMaterializesReachableParent(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var childScratch transientChildScratch
	var parentScratch transientParentScratch
	first := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	second := newLeafNodeInArena(arena, Symbol(2), true, 1, 2, Point{Column: 1}, Point{Column: 2})
	children := childScratch.alloc(2)
	children[0] = first
	children[1] = second
	parent := parentScratch.allocParent(arena, Symbol(3), true, children, 11, true)
	parent.parseState = 7
	parent.preGotoState = 5
	parent.dynamicPrecedence = 13
	parent.rawShape = 17

	if !parentScratch.owns(parent) {
		t.Fatal("expected parent to use transient parent storage before materialization")
	}
	if !childScratch.owns(parent.children) {
		t.Fatal("expected parent children to use transient child storage before materialization")
	}

	entries := []stackEntry{newStackEntryNode(parent.parseState, parent)}
	parentScratch.materializeEntriesUntil(entries, arena, &childScratch, nil)

	got := stackEntryNode(entries[0])
	if got == nil {
		t.Fatal("materialized entry node = nil")
	}
	if parentScratch.owns(got) {
		t.Fatal("entry still points at transient parent after materialization")
	}
	if childScratch.owns(got.children) {
		t.Fatal("materialized parent still owns transient children")
	}
	if len(got.children) != 2 || got.children[0] != first || got.children[1] != second {
		t.Fatalf("materialized children = %#v, want [%p %p]", got.children, first, second)
	}
	if got.parseState != 7 || got.preGotoState != 5 || got.productionID != 11 {
		t.Fatalf("materialized states = (%d,%d,%d), want (7,5,11)", got.parseState, got.preGotoState, got.productionID)
	}
	if got.dynamicPrecedence != 13 || got.rawShape != 17 {
		t.Fatalf("materialized parse metadata = (%d,%d), want (13,17)", got.dynamicPrecedence, got.rawShape)
	}
	if got.StartByte() != 0 || got.EndByte() != 2 {
		t.Fatalf("materialized span = [%d,%d], want [0,2]", got.StartByte(), got.EndByte())
	}
	if got := parentScratch.nodesMaterialized; got != 1 {
		t.Fatalf("nodesMaterialized = %d, want 1", got)
	}

	parentScratch.reset()
	childScratch.reset()
	if len(got.children) != 2 || got.children[0] != first || got.children[1] != second {
		t.Fatal("materialized parent was invalidated by scratch reset")
	}
}

func TestTransientChildScratchMaterializesFieldedArenaParent(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch transientChildScratch
	first := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	second := newLeafNodeInArena(arena, Symbol(2), true, 1, 2, Point{Column: 1}, Point{Column: 2})
	children := scratch.alloc(2)
	children[0] = first
	children[1] = second
	fieldIDs := arena.allocFieldIDSlice(2)
	fieldSources := arena.allocFieldSourceSlice(2)
	fieldIDs[0] = 7
	fieldSources[0] = fieldSourceDirect
	parent := newParentNodeInArenaNoLinksWithFieldSources(arena, Symbol(3), true, children, fieldIDs, fieldSources, 0, true)

	var stack []*Node
	scratch.materializeNodeUntil(parent, arena, &stack, nil)

	if scratch.owns(parent.children) {
		t.Fatal("fielded parent still owns transient children")
	}
	if len(parent.children) != 2 || parent.children[0] != first || parent.children[1] != second {
		t.Fatalf("materialized children = %#v, want [%p %p]", parent.children, first, second)
	}
	if len(parent.fieldIDs()) != 2 || parent.fieldIDs()[0] != 7 {
		t.Fatalf("field IDs = %#v, want first field 7", parent.fieldIDs())
	}
	if len(parent.fieldSources()) != 2 || parent.fieldSources()[0] != fieldSourceDirect {
		t.Fatalf("field sources = %#v, want first direct", parent.fieldSources())
	}

	scratch.reset()
	if len(parent.children) != 2 || parent.children[0] != first || parent.children[1] != second {
		t.Fatal("fielded parent children were invalidated by transient reset")
	}
}

func TestTransientParentScratchMaterializesRecoveredNodeSlice(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var childScratch transientChildScratch
	var parentScratch transientParentScratch
	first := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	second := newLeafNodeInArena(arena, Symbol(2), true, 1, 2, Point{Column: 1}, Point{Column: 2})
	children := childScratch.alloc(2)
	children[0] = first
	children[1] = second
	parent := parentScratch.allocParent(arena, Symbol(3), true, children, 17, true)
	nodes := []*Node{parent}

	materializeTransientParentNodes(nodes, arena, &parentScratch, &childScratch, nil)

	got := nodes[0]
	if got == nil {
		t.Fatal("materialized node = nil")
	}
	if parentScratch.owns(got) {
		t.Fatal("node slice still points at transient parent")
	}
	if childScratch.owns(got.children) {
		t.Fatal("materialized recovered parent still owns transient children")
	}
	if len(got.children) != 2 || got.children[0] != first || got.children[1] != second {
		t.Fatalf("materialized children = %#v, want [%p %p]", got.children, first, second)
	}
	if got.productionID != 17 {
		t.Fatalf("productionID = %d, want 17", got.productionID)
	}

	parentScratch.reset()
	childScratch.reset()
	if len(got.children) != 2 || got.children[0] != first || got.children[1] != second {
		t.Fatal("recovered materialized parent was invalidated by scratch reset")
	}
}

func TestTransientChildScratchMaterializeNodeUntilStopsWhenCancelled(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch transientChildScratch
	var cancelled uint32 = 1
	parser := &Parser{cancellationFlag: &cancelled}
	leaf := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	parent := newParentNodeInArenaNoLinksWithFieldSources(arena, Symbol(2), true, nil, nil, nil, 0, true)
	children := scratch.alloc(1)
	children[0] = leaf
	parent.children = children

	var stack []*Node
	if got, want := scratch.materializeNodeUntil(parent, arena, &stack, parser), ParseStopCancelled; got != want {
		t.Fatalf("materializeNodeUntil stop reason = %q, want %q", got, want)
	}
	if !scratch.owns(parent.children) {
		t.Fatal("parent children were materialized despite cancellation")
	}
	if len(stack) != 0 {
		t.Fatalf("scratch stack length = %d, want 0 after abort cleanup", len(stack))
	}
}

func TestTransientChildFinalizationAbortReturnsErrorTree(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_LANGS", "all")
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	var cancelled uint32
	parser.SetCancellationFlag(&cancelled)

	tree, err := parser.ParseWithTokenSource([]byte("1+2"), &cancelAtEOFArithmeticTokenSource{cancel: &cancelled})
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	defer tree.Release()
	if got, want := tree.ParseStopReason(), ParseStopCancelled; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("RootNode() = nil")
	}
	if got := root.Type(lang); got != "ERROR" {
		t.Fatalf("root type = %q, want ERROR after transient child finalization abort", got)
	}
	if got := root.ChildCount(); got != 0 {
		t.Fatalf("root child count = %d, want 0 for parse error tree", got)
	}
}

func TestTransientChildReturnedTreeMaterializationUsesRawRoot(t *testing.T) {
	// The TypeScript candidate path adds the child after deferred compatibility.
	// This marker proves that transient materialization keeps the raw root.
	lang, root, stmt := newDeferredCompatDynamicImportFixture()
	arena := root.ownerArena
	tree := newTreeWithArenas(root, []byte("import"), lang, arena, nil)
	tree.deferResultCompatibility()

	parser := &Parser{}
	scratch := &parserScratch{}
	if got := parser.materializeTransientChildrenForReturnedTree(tree, arena, scratch); got != ParseStopNone {
		t.Fatalf("materializeTransientChildrenForReturnedTree stop reason = %q, want %q", got, ParseStopNone)
	}
	if !tree.hasDeferredResultCompatibility() {
		t.Fatal("deferred compatibility finalizer missing after transient child materialization")
	}
	if got := resultChildCount(stmt); got != 0 {
		t.Fatalf("import child count after transient child materialization = %d, want deferred", got)
	}
	_ = tree.RootNode()
	if got, want := resultChildCount(stmt), 1; got != want {
		t.Fatalf("import child count after RootNode = %d, want %d", got, want)
	}
}

func TestTransientParentScratchMaterializeEntriesUntilStopsWhenCancelled(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var childScratch transientChildScratch
	var parentScratch transientParentScratch
	var cancelled uint32 = 1
	parser := &Parser{cancellationFlag: &cancelled}
	leaf := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	children := childScratch.alloc(1)
	children[0] = leaf
	parent := parentScratch.allocParent(arena, Symbol(2), true, children, 13, true)
	entries := []stackEntry{newStackEntryNode(parent.parseState, parent)}

	if got, want := parentScratch.materializeEntriesUntil(entries, arena, &childScratch, parser), ParseStopCancelled; got != want {
		t.Fatalf("materializeEntriesUntil stop reason = %q, want %q", got, want)
	}
	if stackEntryNode(entries[0]) != parent {
		t.Fatal("entry node changed despite cancellation before traversal")
	}
	if !parentScratch.owns(parent) {
		t.Fatal("parent was materialized despite cancellation")
	}
	if len(parentScratch.seen) != 0 {
		t.Fatal("materialization scratch was not cleared after abort")
	}
}

func TestBuildResultFromGLRTerminalStopReturnsErrorTreeOwningArena(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)

	var childScratch transientChildScratch
	var parentScratch transientParentScratch
	var cancelled uint32 = 1
	parser := &Parser{language: buildArithmeticLanguage(), cancellationFlag: &cancelled}
	leaf := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	children := childScratch.alloc(1)
	children[0] = leaf
	parent := parentScratch.allocParent(arena, Symbol(2), true, children, 13, true)
	stack := glrStack{entries: []stackEntry{newStackEntryNode(parent.parseState, parent)}}

	tree := parser.buildResultFromGLR([]glrStack{stack}, []byte("1"), arena, nil, nil, nil, &parentScratch, &childScratch, false, nil)
	if tree == nil {
		t.Fatal("buildResultFromGLR returned nil")
	}
	if got := arena.refs.Load(); got != 1 {
		t.Fatalf("parse arena refs after terminal stop = %d, want 1", got)
	}
	if got, want := tree.ParseStopReason(), ParseStopCancelled; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("RootNode() = nil")
	}
	if got := root.Type(parser.language); got != "ERROR" {
		t.Fatalf("root type = %q, want ERROR", got)
	}
	if got := root.ChildCount(); got != 0 {
		t.Fatalf("root child count = %d, want 0", got)
	}
	tree.Release()
	if got := arena.refs.Load(); got != 0 {
		t.Fatalf("parse arena refs after tree release = %d, want 0", got)
	}
}

func TestBuildResultFromGLRNoStacksReturnsErrorTreeOwningArena(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	parser := &Parser{language: buildArithmeticLanguage()}

	tree := parser.buildResultFromGLR(nil, []byte("1"), arena, nil, nil, nil, nil, nil, false, nil)
	if tree == nil {
		t.Fatal("buildResultFromGLR returned nil")
	}
	if got := arena.refs.Load(); got != 1 {
		t.Fatalf("parse arena refs after no-stack error tree = %d, want 1", got)
	}
	root := rawRootOrNil(tree)
	if root == nil {
		t.Fatal("raw root = nil")
	}
	if got := root.Type(parser.language); got != "ERROR" {
		t.Fatalf("root type = %q, want ERROR", got)
	}
	tree.Release()
	if got := arena.refs.Load(); got != 0 {
		t.Fatalf("parse arena refs after tree release = %d, want 0", got)
	}
}

func TestTransientParentScratchMaterializeNodeSliceUntilStopsOnExpiredTimeout(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var childScratch transientChildScratch
	var parentScratch transientParentScratch
	parser := &Parser{}
	parser.SetTimeoutMicros(100)
	operationBudget := parser.beginParseOperationBudget()
	defer parser.endParseOperationBudget(operationBudget)
	time.Sleep(2 * time.Millisecond)

	leaf := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	children := childScratch.alloc(1)
	children[0] = leaf
	parent := parentScratch.allocParent(arena, Symbol(2), true, children, 17, true)
	nodes := []*Node{parent}

	if got, want := parentScratch.materializeNodeSliceUntil(nodes, arena, &childScratch, parser), ParseStopTimeout; got != want {
		t.Fatalf("materializeNodeSliceUntil stop reason = %q, want %q", got, want)
	}
	if nodes[0] != parent {
		t.Fatal("node slice changed despite timeout before traversal")
	}
	if !parentScratch.owns(parent) {
		t.Fatal("parent was materialized despite timeout")
	}
	if len(parentScratch.seen) != 0 {
		t.Fatal("materialization scratch was not cleared after timeout")
	}
}

func TestTransientParentScratchMaterializesSharedTransientParentOnce(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var childScratch transientChildScratch
	var parentScratch transientParentScratch
	leaf := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	sharedChildren := childScratch.alloc(1)
	sharedChildren[0] = leaf
	shared := parentScratch.allocParent(arena, Symbol(2), true, sharedChildren, 19, true)
	leftChildren := childScratch.alloc(1)
	leftChildren[0] = shared
	rightChildren := childScratch.alloc(1)
	rightChildren[0] = shared
	left := parentScratch.allocParent(arena, Symbol(3), true, leftChildren, 23, true)
	right := parentScratch.allocParent(arena, Symbol(4), true, rightChildren, 29, true)
	rootChildren := childScratch.alloc(2)
	rootChildren[0] = left
	rootChildren[1] = right
	root := parentScratch.allocParent(arena, Symbol(5), true, rootChildren, 31, true)

	entries := []stackEntry{newStackEntryNode(root.parseState, root)}
	parentScratch.materializeEntriesUntil(entries, arena, &childScratch, nil)

	gotRoot := stackEntryNode(entries[0])
	if gotRoot == nil || parentScratch.owns(gotRoot) {
		t.Fatal("root was not materialized out of transient storage")
	}
	gotLeft := gotRoot.children[0]
	gotRight := gotRoot.children[1]
	if gotLeft == nil || gotRight == nil {
		t.Fatal("materialized root children are nil")
	}
	gotSharedLeft := gotLeft.children[0]
	gotSharedRight := gotRight.children[0]
	if gotSharedLeft == nil || gotSharedRight == nil {
		t.Fatal("materialized shared children are nil")
	}
	if gotSharedLeft != gotSharedRight {
		t.Fatal("shared transient parent was materialized more than once")
	}
	if parentScratch.owns(gotSharedLeft) {
		t.Fatal("shared parent still points at transient storage")
	}
	if got := parentScratch.nodesMaterialized; got != 4 {
		t.Fatalf("nodesMaterialized = %d, want 4", got)
	}
}

func TestTransientChildScratchOverflowSlabGrowthBounded(t *testing.T) {
	elemSize := int(unsafe.Sizeof((*Node)(nil)))
	ceiling := maxOverflowSlabGrowthBytes / elemSize
	if ceiling <= 0 {
		t.Fatalf("invalid transient-child slab ceiling %d", ceiling)
	}

	initial := ceiling * 3 / 4
	scratch := transientChildScratch{
		slabs:          []childSliceSlab{{data: make([]*Node, initial)}},
		allocatedBytes: childSliceBytesForCap(initial),
	}
	wantPointers := initial + ceiling*3
	for allocated := 0; allocated < wantPointers; {
		n := min(1024, wantPointers-allocated)
		_ = scratch.alloc(n)
		allocated += n
	}

	capacity := 0
	used := 0
	for i := range scratch.slabs {
		slabCap := len(scratch.slabs[i].data)
		capacity += slabCap
		used += scratch.slabs[i].used
		if i > 0 && slabCap > ceiling {
			t.Fatalf("transient-child overflow slab %d capacity=%d exceeds ceiling=%d", i, slabCap, ceiling)
		}
	}
	if waste := capacity - used; waste > ceiling {
		t.Fatalf("transient-child capacity waste=%d pointers exceeds one ceiling slab=%d", waste, ceiling)
	}
	if got, want := scratch.allocatedBytes, childSliceBytesForCap(capacity); got != want {
		t.Fatalf("transient-child allocatedBytes=%d, want %d", got, want)
	}
}

func TestTransientParentScratchOverflowSlabGrowthBounded(t *testing.T) {
	elemSize := int(unsafe.Sizeof(Node{}))
	ceiling := maxOverflowSlabGrowthBytes / elemSize
	if ceiling <= 0 {
		t.Fatalf("invalid transient-parent slab ceiling %d", ceiling)
	}

	initial := ceiling * 3 / 4
	scratch := transientParentScratch{
		slabs:          []transientParentSlab{{data: make([]Node, initial)}},
		allocatedBytes: nodeStructBytesForCap(initial),
	}
	wantNodes := initial + ceiling*3
	for i := 0; i < wantNodes; i++ {
		_ = scratch.allocParent(nil, 1, true, nil, 0, false)
	}

	capacity := 0
	used := 0
	for i := range scratch.slabs {
		slabCap := len(scratch.slabs[i].data)
		capacity += slabCap
		used += scratch.slabs[i].used
		if i > 0 && slabCap > ceiling {
			t.Fatalf("transient-parent overflow slab %d capacity=%d exceeds ceiling=%d", i, slabCap, ceiling)
		}
	}
	if waste := capacity - used; waste > ceiling {
		t.Fatalf("transient-parent capacity waste=%d nodes exceeds one ceiling slab=%d", waste, ceiling)
	}
	if got, want := scratch.allocatedBytes, nodeStructBytesForCap(capacity); got != want {
		t.Fatalf("transient-parent allocatedBytes=%d, want %d", got, want)
	}
}

func TestTransientScratchCheckpointMaterializesLiveStackAndReusesSlabs(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch parserScratch
	scratch.transientCheckpointBytes = 1
	scratch.reduce.transientParents = &scratch.transientParents
	scratch.reduce.transientChildren = &scratch.transientChildren
	parser := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)}

	leaf := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	children := scratch.transientChildren.alloc(1)
	children[0] = leaf
	parent := scratch.transientParents.allocParent(arena, Symbol(2), true, children, 0, true)
	parent.dynamicPrecedence = 13
	parent.rawShape = 17
	if slot := parser.cNodeMemoSlot(parent); slot != nil {
		*slot = cNodeMemoCacheEntry{
			node:    uintptr(unsafe.Pointer(parent)),
			hasCost: true,
			epoch:   parser.cNodeMemoEpoch,
		}
	}
	epochBefore := parser.cNodeMemoEpoch
	memoIdx := cNodeMemoCacheIndex(uintptr(unsafe.Pointer(parent)), len(parser.cNodeMemoCache)>>1)
	stack := glrStack{entries: []stackEntry{newStackEntryNode(7, parent)}, cacheEntries: true, byteOffset: 1}
	parentCapacityBytes := scratch.transientParents.allocatedBytes
	childCapacityBytes := scratch.transientChildren.allocatedBytes
	firstParentAddress := &scratch.transientParents.slabs[0].data[0]

	if reason := parser.checkpointTransientScratch([]glrStack{stack}, &scratch, arena); reason != ParseStopNone {
		t.Fatalf("checkpointTransientScratch() = %q, want no stop", reason)
	}
	materialized := stackEntryNode(stack.entries[0])
	if materialized == nil || scratch.transientParents.owns(materialized) {
		t.Fatalf("live stack parent was not materialized: %p", materialized)
	}
	if materialized.dynamicPrecedence != 13 || materialized.rawShape != 17 {
		t.Fatalf("checkpoint parse metadata = precedence %d rawShape %d, want 13 and 17", materialized.dynamicPrecedence, materialized.rawShape)
	}
	if scratch.transientParents.usedBytes() != 0 || scratch.transientChildren.usedBytes() != 0 {
		t.Fatalf("transient slabs still used after checkpoint: parents=%d children=%d", scratch.transientParents.usedBytes(), scratch.transientChildren.usedBytes())
	}
	if scratch.transientParents.allocatedBytes != parentCapacityBytes || scratch.transientChildren.allocatedBytes != childCapacityBytes {
		t.Fatal("checkpoint dropped reusable transient slab capacity")
	}
	if scratch.transientParents.nodesAllocated != 1 || scratch.transientParents.nodesMaterialized != 1 {
		t.Fatalf("parent counters not preserved: allocated=%d materialized=%d", scratch.transientParents.nodesAllocated, scratch.transientParents.nodesMaterialized)
	}
	if scratch.transientCheckpoints != 1 {
		t.Fatalf("transient checkpoints = %d, want 1", scratch.transientCheckpoints)
	}
	if parser.cNodeMemoEpoch != epochBefore+1 {
		t.Fatalf("recovery memo epoch = %d, want %d", parser.cNodeMemoEpoch, epochBefore+1)
	}
	if stale := parser.cNodeMemoCache[memoIdx]; stale.node != uintptr(unsafe.Pointer(parent)) || stale.epoch != epochBefore || !stale.hasCost {
		t.Fatalf("checkpoint physically cleared recovery memo instead of invalidating by epoch: %#v", stale)
	}
	if slot := parser.cNodeMemoSlot(parent); slot.hasCost {
		t.Fatal("recovery node memo retained recycled transient pointer in the new epoch")
	}

	reused := scratch.transientParents.allocParent(arena, Symbol(3), true, nil, 0, false)
	if reused != firstParentAddress {
		t.Fatalf("transient parent slab was not reused: got %p want %p", reused, firstParentAddress)
	}
}

func TestTransientScratchCheckpointDefersWithPendingParents(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch parserScratch
	scratch.transientCheckpointBytes = 1
	scratch.reduce.transientParents = &scratch.transientParents
	scratch.reduce.transientChildren = &scratch.transientChildren
	parser := &Parser{pendingFullParents: true}

	parent := scratch.transientParents.allocParent(arena, Symbol(2), true, nil, 0, false)
	stack := glrStack{entries: []stackEntry{newStackEntryNode(7, parent)}, cacheEntries: true, byteOffset: 1}
	usedBefore := scratch.transientParents.usedBytes()

	if reason := parser.checkpointTransientScratch([]glrStack{stack}, &scratch, arena); reason != ParseStopNone {
		t.Fatalf("checkpointTransientScratch() = %q, want no stop", reason)
	}
	if got := stackEntryNode(stack.entries[0]); got != parent {
		t.Fatalf("pending-parent mode unexpectedly materialized stack node: got %p, want %p", got, parent)
	}
	if got := scratch.transientParents.usedBytes(); got != usedBefore {
		t.Fatalf("pending-parent mode recycled transient storage: used=%d, want %d", got, usedBefore)
	}
	if scratch.transientCheckpoints != 0 {
		t.Fatalf("pending-parent mode checkpoints = %d, want 0", scratch.transientCheckpoints)
	}
}

func TestTransientScratchCheckpointRetargetsNestedRawShapeNodesBeforeSlabReuse(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch parserScratch
	scratch.transientCheckpointBytes = 1
	scratch.reduce.transientParents = &scratch.transientParents
	scratch.reduce.transientChildren = &scratch.transientChildren
	parser := &Parser{}

	leaf := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	rawGrandchild := scratch.transientParents.allocParent(arena, Symbol(2), true, nil, 0, false)
	rawGrandchild.rawShape = parser.captureRawShape(nil, arena, Symbol(2), 0, []stackEntry{
		newStackEntryNode(1, leaf),
	}, 0, 1)
	rawChild := scratch.transientParents.allocParent(arena, Symbol(3), true, nil, 0, false)
	rawChild.rawShape = parser.captureRawShape(nil, arena, Symbol(3), 0, []stackEntry{
		newStackEntryNode(2, rawGrandchild),
	}, 0, 1)
	semanticChildren := scratch.transientChildren.alloc(1)
	semanticChildren[0] = leaf
	liveParent := scratch.transientParents.allocParent(arena, Symbol(4), true, semanticChildren, 0, false)
	liveParent.rawShape = parser.captureRawShape(nil, arena, Symbol(4), 0, []stackEntry{
		newStackEntryNode(3, rawChild),
	}, 0, 1)
	wantHashes := make(map[rawShapeRef]uint64)
	for _, ref := range []rawShapeRef{rawGrandchild.rawShape, rawChild.rawShape, liveParent.rawShape} {
		hash, ok := arena.rawShapeHash(ref)
		if !ok {
			t.Fatalf("raw shape %d has no hash before materialization", ref)
		}
		wantHashes[ref] = hash
	}
	wantRootChildShape := rawChild.rawShape
	wantNestedChildShape := rawGrandchild.rawShape
	stack := glrStack{entries: []stackEntry{newStackEntryNode(4, liveParent)}, cacheEntries: true, byteOffset: 1}

	if reason := parser.checkpointTransientScratch([]glrStack{stack}, &scratch, arena); reason != ParseStopNone {
		t.Fatalf("checkpointTransientScratch() = %q, want no stop", reason)
	}
	materializedRoot := stackEntryNode(stack.entries[0])
	if materializedRoot == nil || scratch.transientParents.owns(materializedRoot) {
		t.Fatalf("live stack parent was not materialized: %p", materializedRoot)
	}
	rootShape, ok := arena.rawShapeForRef(materializedRoot.rawShape)
	if !ok {
		t.Fatalf("materialized root raw shape %d was not retained", materializedRoot.rawShape)
	}
	rootRawChildren := arena.rawShapeChildren(rootShape)
	if len(rootRawChildren) != 1 {
		t.Fatalf("materialized root raw child count = %d, want 1", len(rootRawChildren))
	}
	if got := rootRawChildren[0].shapeRef(); got != wantRootChildShape {
		t.Fatalf("materialized root child shape ref = %d, want captured ref %d", got, wantRootChildShape)
	}
	materializedRawChild := stackEntryNode(rootRawChildren[0].entry())
	if materializedRawChild == nil || scratch.transientParents.owns(materializedRawChild) {
		t.Fatalf("root raw-shape child still points into transient storage: %p", materializedRawChild)
	}
	childShape, ok := arena.rawShapeForRef(rootRawChildren[0].shapeRef())
	if !ok {
		t.Fatalf("materialized raw child shape %d was not retained", rootRawChildren[0].shapeRef())
	}
	childRawChildren := arena.rawShapeChildren(childShape)
	if len(childRawChildren) != 1 {
		t.Fatalf("materialized nested raw child count = %d, want 1", len(childRawChildren))
	}
	if got := childRawChildren[0].shapeRef(); got != wantNestedChildShape {
		t.Fatalf("materialized nested child shape ref = %d, want captured ref %d", got, wantNestedChildShape)
	}
	materializedRawGrandchild := stackEntryNode(childRawChildren[0].entry())
	if materializedRawGrandchild == nil || scratch.transientParents.owns(materializedRawGrandchild) {
		t.Fatalf("nested raw-shape child still points into transient storage: %p", materializedRawGrandchild)
	}

	// Reuse every source slot immediately. The retained sidecar graph must keep
	// pointing at its arena clones rather than observing these new reductions.
	reusedGrandchild := scratch.transientParents.allocParent(arena, Symbol(90), true, nil, 0, false)
	reusedChild := scratch.transientParents.allocParent(arena, Symbol(91), true, nil, 0, false)
	reusedRoot := scratch.transientParents.allocParent(arena, Symbol(92), true, nil, 0, false)
	if reusedGrandchild != rawGrandchild || reusedChild != rawChild || reusedRoot != liveParent {
		t.Fatalf("transient parent addresses were not reused in allocation order: got (%p,%p,%p) want (%p,%p,%p)",
			reusedGrandchild, reusedChild, reusedRoot, rawGrandchild, rawChild, liveParent)
	}
	if got := stackEntryNode(rootRawChildren[0].entry()); got != materializedRawChild {
		t.Fatalf("root raw-shape child pointer changed after slab reuse: got %p, want %p", got, materializedRawChild)
	} else if got.symbol != Symbol(3) {
		t.Fatalf("root raw-shape child symbol after slab reuse = %d, want 3", got.symbol)
	}
	if got := stackEntryNode(childRawChildren[0].entry()); got != materializedRawGrandchild {
		t.Fatalf("nested raw-shape child pointer changed after slab reuse: got %p, want %p", got, materializedRawGrandchild)
	} else if got.symbol != Symbol(2) {
		t.Fatalf("nested raw-shape child symbol after slab reuse = %d, want 2", got.symbol)
	}
	// Recompute each hash after materialization and reuse of the source storage.
	for ref, want := range wantHashes {
		clear(arena.rawShapeHashCache)
		got, ok := arena.rawShapeHash(ref)
		if !ok || got != want {
			t.Fatalf("raw shape %d hash after materialization = %#x, %v; want %#x, true", ref, got, ok, want)
		}
	}

}
