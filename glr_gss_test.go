package gotreesitter

import (
	"fmt"
	"testing"
	"unsafe"
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

	entries := []stackEntry{
		{state: 1},
		newStackEntryNode(2, node),
		newStackEntryNoTreeNode(3, noTree),
		newStackEntryCompactFullLeaf(4, compactLeaf),
		newStackEntryPendingParent(5, pending),
	}
	for _, entry := range entries {
		got := gssEntryHash(gssHashSeed, entry)
		want := gssEntryHashViaAccessors(gssHashSeed, entry)
		if got != want {
			t.Fatalf("gssEntryHash(%+v) = %d, want %d", entry, got, want)
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

func TestGSSMainPreflightReachPromotesLargeWalkAndCleans(t *testing.T) {
	nodes := make([]*gssNode, gssPreflightReachLinearLimit+2)
	for i := range nodes {
		nodes[i] = &gssNode{
			entry: stackEntry{state: StateID(i + 1)},
			depth: uint32(i + 1),
		}
		if i > 0 {
			nodes[i].prev = nodes[i-1]
		}
	}

	p := newGSSMainPreflight(make(map[gssMergePair]bool))
	if !p.canReach(nodes[len(nodes)-1], nodes[0]) {
		t.Fatal("large preflight walk did not reach the chain root")
	}
	if p.reachSeen == nil || len(p.reachSeen) != 0 {
		t.Fatalf("large preflight map after walk = %v, want allocated and empty", p.reachSeen)
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
