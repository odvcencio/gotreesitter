package gotreesitter

import "testing"

func TestCVersionStatusAddsPausedSkippedTreeCost(t *testing.T) {
	parser := &Parser{}

	paused := glrStack{cPaused: true}
	stackCost := parser.cStackErrorCost(&paused)
	if stackCost != cErrCostPerRecovery {
		t.Fatalf("paused stack cost = %d, want %d", stackCost, cErrCostPerRecovery)
	}
	status := parser.cVersionStatus(&paused)
	wantStatusCost := stackCost + cErrCostPerSkippedTree
	if status.cost != wantStatusCost {
		t.Fatalf("paused version status cost = %d, want %d", status.cost, wantStatusCost)
	}
	if !status.isInError {
		t.Fatal("paused version status isInError = false, want true")
	}

	openError := glrStack{cRec: &cRecoverState{}}
	stackCost = parser.cStackErrorCost(&openError)
	if stackCost != cErrCostPerRecovery {
		t.Fatalf("open ERROR_STATE stack cost = %d, want %d", stackCost, cErrCostPerRecovery)
	}
	status = parser.cVersionStatus(&openError)
	if status.cost != stackCost {
		t.Fatalf("open ERROR_STATE version status cost = %d, want stack cost %d", status.cost, stackCost)
	}
	if !status.isInError {
		t.Fatal("open ERROR_STATE version status isInError = false, want true")
	}
}

func TestCRecoveryCostCompetitionDisabledInNoTreeModes(t *testing.T) {
	parser := &Parser{errorCostCompetition: true}
	if !parser.errorCostCompetitionEnabled() {
		t.Fatal("errorCostCompetitionEnabled = false, want true for ordinary tree mode")
	}

	parser.noTreeBenchmarkOnly = true
	if parser.errorCostCompetitionEnabled() {
		t.Fatal("errorCostCompetitionEnabled = true, want false for no-tree benchmark mode")
	}

	parser.noTreeBenchmarkOnly = false
	parser.noTreeCheckpointBenchmarkOnly = true
	if parser.errorCostCompetitionEnabled() {
		t.Fatal("errorCostCompetitionEnabled = true, want false for no-tree checkpoint mode")
	}
}

func TestCRecoverDispatchInErrorAdvancesZeroWidthSkippedToken(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	openErr := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
	cSetNodeSpan(openErr, 10, 10, Point{Column: 10}, Point{Column: 10})
	openErr.setHasError(true)
	openErr.parseState = cErrorState
	stack := newGLRStack(1)
	stack.pushEntry(newStackEntryNode(cErrorState, openErr), nil, nil)
	stack.byteOffset = 10
	stack.cRec = &cRecoverState{group: group, openErr: openErr}
	stacks := []glrStack{stack}
	nodeCount := 0

	outcome, forked := parser.cRecoverDispatchInError(
		&stacks,
		0,
		[]byte("012345678901234567890"),
		Token{Symbol: 2, StartByte: 18, EndByte: 18, StartPoint: Point{Column: 18}, EndPoint: Point{Column: 18}},
		&nodeCount,
		arena,
		nil,
		nil,
		nil,
	)

	if outcome != cRecConsumed || forked {
		t.Fatalf("outcome/forked = %v/%t, want %v/false", outcome, forked, cRecConsumed)
	}
	if !stacks[0].shifted {
		t.Fatal("zero-width skipped token did not mark stack shifted")
	}
	if got, want := stacks[0].byteOffset, uint32(18); got != want {
		t.Fatalf("stack byteOffset = %d, want %d", got, want)
	}
	if got, want := openErr.EndByte(), uint32(18); got != want {
		t.Fatalf("open error end = %d, want %d", got, want)
	}
	if nodeCount == 0 {
		t.Fatal("zero-width skipped token did not update recovery node count")
	}
}

func TestCRecoverDispatchInErrorAbsorbsZeroWidthSkippedTokenAtCurrentOffset(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	openErr := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
	cSetNodeSpan(openErr, 18, 18, Point{Column: 18}, Point{Column: 18})
	openErr.setHasError(true)
	openErr.parseState = cErrorState
	stack := newGLRStack(1)
	stack.pushEntry(newStackEntryNode(cErrorState, openErr), nil, nil)
	stack.byteOffset = 18
	stack.cRec = &cRecoverState{group: group, openErr: openErr}
	stacks := []glrStack{stack}
	nodeCount := 0

	outcome, forked := parser.cRecoverDispatchInError(
		&stacks,
		0,
		[]byte("012345678901234567890"),
		Token{Symbol: 2, StartByte: 18, EndByte: 18, StartPoint: Point{Column: 18}, EndPoint: Point{Column: 18}},
		&nodeCount,
		arena,
		nil,
		nil,
		nil,
	)

	if outcome != cRecConsumed || forked {
		t.Fatalf("outcome/forked = %v/%t, want %v/false", outcome, forked, cRecConsumed)
	}
	if !stacks[0].shifted {
		t.Fatal("zero-width skipped token did not mark stack shifted")
	}
	if got, want := stacks[0].byteOffset, uint32(18); got != want {
		t.Fatalf("stack byteOffset = %d, want %d", got, want)
	}
	if got, want := openErr.ChildCount(), 1; got != want {
		t.Fatalf("open error ChildCount = %d, want %d", got, want)
	}
	if got, want := openErr.Child(0).StartByte(), uint32(18); got != want {
		t.Fatalf("zero-width child StartByte = %d, want %d", got, want)
	}
	if nodeCount == 0 {
		t.Fatal("zero-width skipped token did not update recovery node count")
	}
}

func TestCRecoverSkipTailBetterVersionUsesErrorStatus(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	absorbing := newGLRStack(1)
	absorbing.pushEntry(stackEntry{state: cErrorState}, nil, nil)
	absorbing.byteOffset = 10
	absorbing.cRec = &cRecoverState{group: group}

	cleanFork := newGLRStack(2)
	errNode := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
	cleanFork.pushEntry(newStackEntryNode(3, errNode), nil, nil)
	cleanFork.byteOffset = absorbing.byteOffset

	stacks := []glrStack{absorbing, cleanFork}
	skipCost := parser.cStackErrorCost(&stacks[0]) + cErrCostPerSkippedTree
	if got := parser.cStackErrorCost(&stacks[1]); got >= skipCost {
		t.Fatalf("clean fork cost = %d, want less than skip candidate cost %d", got, skipCost)
	}

	if parser.cBetterVersionExists(stacks, 0, false, skipCost) {
		t.Fatal("clean fork killed non-error skip candidate; want it to only prefer a non-mergeable clean candidate")
	}
	if !parser.cBetterVersionExists(stacks, 0, true, skipCost) {
		t.Fatal("clean fork did not kill in-error skip candidate")
	}
}

func TestCApplyMergedErrorGroupBaselineUsesMaxHeadNodeCount(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	short := cRecoveryBaselineStack(arena, 2, 1)
	long := cRecoveryBaselineStack(arena, 2, 3)
	versions := []glrStack{short, long}
	for vi := range versions {
		versions[vi].pushEntry(stackEntry{state: cErrorState}, nil, nil)
	}

	baseline := parser.cApplyMergedErrorGroupBaseline(versions)
	if baseline != 3 {
		t.Fatalf("merged baseline = %d, want max cumulative node count 3", baseline)
	}
	for i := range versions {
		if got := versions[i].cNodeBaseline; got != baseline {
			t.Fatalf("version %d baseline = %d, want shared %d", i, got, baseline)
		}
	}
	if got := parser.cNodeCountSinceError(&versions[0]); got != 0 {
		t.Fatalf("short version node count since error = %d, want clamp to 0", got)
	}
	if got := versions[0].cNodeBaseline; got != 1 {
		t.Fatalf("short version clamped baseline = %d, want current cumulative count 1", got)
	}
	if got := parser.cNodeCountSinceError(&versions[1]); got != 0 {
		t.Fatalf("long version node count since error = %d, want 0", got)
	}
	if got := versions[1].cNodeBaseline; got != baseline {
		t.Fatalf("long version baseline after node count = %d, want shared %d", got, baseline)
	}
}

func TestCRecordSummaryUsesCompactEntryPosition(t *testing.T) {
	parser := &Parser{}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newCompactFullLeafInArena(arena, 1, true, 13, 21, Point{Row: 2, Column: 3}, Point{Row: 2, Column: 11})
	entry := newStackEntryCompactFullLeaf(3, leaf)
	summary := parser.cRecordSummary([]stackEntry{entry, {state: 2}})
	if len(summary) == 0 {
		t.Fatal("summary is empty")
	}
	if got := summary[0].posBytes; got != 21 {
		t.Fatalf("compact summary posBytes = %d, want 21", got)
	}
	if got := summary[0].posRow; got != 2 {
		t.Fatalf("compact summary posRow = %d, want 2", got)
	}
}

func TestCStackPosRowUsesCompactEntryPoint(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newCompactFullLeafInArena(arena, 1, true, 13, 21, Point{Row: 4, Column: 3}, Point{Row: 4, Column: 11})
	entry := newStackEntryCompactFullLeaf(3, leaf)

	sliceStack := glrStack{entries: []stackEntry{{state: 1}, entry}}
	if got := cStackPosRow(&sliceStack); got != 4 {
		t.Fatalf("slice-backed compact stack row = %d, want 4", got)
	}

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	head := scratch.allocNode(entry, base, 2)
	gssStack := glrStack{gss: gssStack{head: head}}
	if got := cStackPosRow(&gssStack); got != 4 {
		t.Fatalf("GSS-backed compact stack row = %d, want 4", got)
	}
}

// TestCAppendVisibleSpliceRecoveryPreservesExtraErrorCarrier pins C parity
// for ordinary (non-recovery) reduce/splice paths around an extra ERROR
// carrier: cAppendVisibleSplice never dissolves an ERROR node (only invisible
// hidden-symbol subtrees flatten), and an ordinary reduce keeps a popped extra
// ERROR carrier as a direct child — the C oracle's `program` for php's
// `static function a() {}` carries `(ERROR ...)` as a direct child, and an
// earlier (reverted) mechanism that dissolved it here dropped both the
// HasError bit and the error cost. The wave-1 investigation additionally
// found that the production-signature rewrapping this test used to pin
// (cAppendRecoveryVisibleSplice and its cRecoveryVisibleSplice* helpers) is
// itself NOT what C does — C's ts_parser__reduce/recover_to_state never
// reconstruct a synthesized production node over an ERROR carrier's
// children — so that cluster was deleted as production-dead, C-incorrect
// code rather than kept "for coverage".
func TestCAppendVisibleSpliceRecoveryPreservesExtraErrorCarrier(t *testing.T) {
	lang := &Language{
		TokenCount:  1,
		StateCount:  3,
		SymbolCount: 5,
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "brace", Visible: true, Named: true},
			{Name: "match_arm_a", Visible: true, Named: true},
			{Name: "match_arm_b", Visible: true, Named: true},
			{Name: "match_block", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, errorCostCompetition: true}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 1, true, 3, 4, Point{Column: 3}, Point{Column: 4})
	armA := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	armB := newLeafNodeInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	armB.setHasError(true)
	carrier := newParentNodeInArena(arena, errorSymbol, true, []*Node{armA, armB}, nil, 0)
	carrier.setExtra(true)
	carrier.setHasError(true)

	preserved := parser.cAppendVisibleSplice(nil, carrier)
	if len(preserved) != 1 || preserved[0] != carrier {
		t.Fatalf("ordinary visible splice = %#v, want ERROR carrier preserved", preserved)
	}

	entries := []stackEntry{
		newStackEntryNode(1, left),
		newStackEntryNode(1, carrier),
		newStackEntryNode(1, right),
	}
	children, _, _ := parser.buildReduceChildren(entries, 0, len(entries), 2, 4, 0, arena)
	if len(children) != 3 {
		t.Fatalf("reduced child count = %d, want 3", len(children))
	}
	if children[0] != left || children[1] != carrier || children[2] != right {
		t.Fatalf("reduced children = %#v, want left/carrier/right", children)
	}
	if !children[1].hasError() {
		t.Fatal("reduced ERROR carrier hasError was cleared")
	}
}

// TestCRecoverStrategy1ElectionDedupesDuplicateEntryAcrossMembers pins the C
// semantics of the merged-version summary: ts_stack_record_summary dedupes
// (depth, state) pairs across the merged paths at RECORD time, and
// ts_parser__recover reads position/cost from the ONE merged version (m0 in
// this port), not each member's own byteOffset.
//
// Caveat (both guards fire independently here): this fixture's duplicate
// entry's posBytes(5) equals m0's own position(5), so the m0-position-equality
// guard (`entry.posBytes == pos`) ALSO produces no-fork on its own, regardless
// of the seen-key dedup — stack1's own byteOffset(10) is irrelevant because
// position is read from m0, not the member being scanned. This test therefore
// pins the m0-position basis; it does not, by itself, isolate the
// record-time dedup from that guard (both independently veto the fork here).
func TestCRecoverStrategy1ElectionDedupesDuplicateEntryAcrossMembers(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	entry := cStackSummaryEntry{depth: 1, state: 2, posBytes: 5}
	stacks := []glrStack{
		cRecoveryElectionStack(arena, 4, 3, 5, group, 0, []cStackSummaryEntry{entry}),
		cRecoveryElectionStack(arena, 2, 3, 10, group, 1, []cStackSummaryEntry{entry}),
	}
	nodeCount := 0
	didRecover, forked := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, nil, nil)
	if didRecover || forked {
		t.Fatalf("strategy 1 election = didRecover %v forked %v, want false/false (entry position equals the merged version position; duplicate entries dedupe at first encounter)", didRecover, forked)
	}
	if len(stacks) != 2 {
		t.Fatalf("stack count = %d, want no fork appended", len(stacks))
	}
}

func TestCRecoverStrategy1ElectionIgnoresBetterVersionForNoActionEntry(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	stacks := []glrStack{
		cRecoveryElectionStack(arena, 2, 3, 2500, group, 0, []cStackSummaryEntry{
			{depth: 0, state: 4, posBytes: 0},
			{depth: 1, state: 2, posBytes: 2490},
		}),
		cRecoveryElectionPlainStack(arena, 4, 4, 2500),
	}
	nodeCount := 0
	didRecover, forked := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, nil, nil)
	if !didRecover || !forked {
		t.Fatalf("strategy 1 election = didRecover %v forked %v, want true/true", didRecover, forked)
	}
	if len(stacks) != 3 {
		t.Fatalf("stack count = %d, want recovered fork appended", len(stacks))
	}
	if got := stacks[2].top().state; got != StateID(2) {
		t.Fatalf("recovered fork top state = %d, want 2", got)
	}
}

// TestCRecoverStrategy1ElectionUsesMergedVersionCostBasis pins C's cost basis
// for the strategy-1 scan: parser.c ts_parser__recover computes position and
// current_error_cost ONCE for the single merged error-state version (this
// engine's first group member). A candidate entry whose hypothetical recovery
// is clearly worse than an existing version aborts the whole scan (C `break`),
// even when a later member's engine-local byte offset would have produced a
// cheaper-looking cost.
func TestCRecoverStrategy1ElectionUsesMergedVersionCostBasis(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	entry := cStackSummaryEntry{depth: 1, state: 2, posBytes: 0}
	stacks := []glrStack{
		cRecoveryElectionStack(arena, 4, 3, 2500, group, 0, nil),
		cRecoveryElectionStack(arena, 2, 3, 10, group, 1, []cStackSummaryEntry{entry}),
		cRecoveryElectionPlainStack(arena, 4, 4, 2500),
	}
	nodeCount := 0
	didRecover, forked := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, nil, nil)
	if didRecover || forked {
		t.Fatalf("strategy 1 election = didRecover %v forked %v, want false/false (merged-version cost makes the entry clearly worse than the clean sibling, aborting the scan)", didRecover, forked)
	}
	if len(stacks) != 3 {
		t.Fatalf("stack count = %d, want no fork appended", len(stacks))
	}
}

func cRecoveryElectionTestParser() *Parser {
	lang := &Language{
		TokenCount:  2,
		StateCount:  5,
		SymbolCount: 3,
		ParseTable: [][]uint16{
			nil,
			nil,
			{0, 1},
			nil,
			nil,
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "tok", Visible: true, Named: true},
			{Name: "node", Visible: true, Named: true},
		},
	}
	return &Parser{language: lang, denseLimit: len(lang.ParseTable)}
}

func cRecoveryElectionStack(arena *nodeArena, base, top StateID, endByte uint32, group *cRecGroup, groupOrder int, summary []cStackSummaryEntry) glrStack {
	s := cRecoveryElectionPlainStack(arena, base, top, endByte)
	s.cRec = &cRecoverState{
		group:      group,
		groupOrder: groupOrder,
		summary:    summary,
	}
	s.cNodeBaseline = 1
	return s
}

func cRecoveryElectionPlainStack(arena *nodeArena, base, top StateID, endByte uint32) glrStack {
	s := newGLRStack(base)
	n := newLeafNodeInArena(arena, 1, true, 0, endByte, Point{}, Point{Column: endByte})
	s.pushEntry(newStackEntryNode(top, n), nil, nil)
	return s
}

func cRecoveryBaselineStack(arena *nodeArena, start StateID, visibleNodes int) glrStack {
	s := newGLRStack(start)
	for i := 0; i < visibleNodes; i++ {
		startByte := uint32(i)
		endByte := startByte + 1
		n := newLeafNodeInArena(arena, 1, true, startByte, endByte, Point{Column: startByte}, Point{Column: endByte})
		s.pushEntry(newStackEntryNode(start+StateID(i)+1, n), nil, nil)
	}
	return s
}

func TestCDoAllPotentialReductionsCollapsesSamePopTargetSlicesByCParentSelection(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNode(3, right), leftNode, 3)
	altRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	altRight.dynamicPrecedence = 9
	rightNode.extraLinks = append(rightNode.extraLinks, gssMainLink{
		prev:  leftNode,
		entry: newStackEntryNode(3, altRight),
	})
	start := glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}

	nodeCount := 0
	versions, canShift := parser.cDoAllPotentialReductions(nil, start, 0, true, Token{}, &nodeCount, arena, nil, &scratch, nil)
	if canShift {
		t.Fatal("canShift = true, want false")
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0", len(parser.pendingForkStacks))
	}
	if len(versions) != 1 {
		t.Fatalf("version count = %d, want one selected version", len(versions))
	}
	if versions[0].dead {
		t.Fatal("selected reduction version is dead")
	}
	// The forked GSS reducer now applies the selected primary fork directly;
	// C recovery's invariant is that no internal pending fork survives this
	// potential-reduction probe.
	top := versions[0].top()
	if !stackEntryHasNode(top) {
		t.Fatal("selected reduction version top has no node")
	}
	if got, want := stackEntryNode(top).symbol, Symbol(4); got != want {
		t.Fatalf("selected reduction symbol = %d, want %d", got, want)
	}
	parent := stackEntryNode(top)
	if len(parent.children) != 2 || parent.children[1] != altRight {
		t.Fatal("selected parent children did not preserve the C-selected same-pop child array")
	}
	if got := versions[0].gss.head.linkCount(); got != 1 {
		t.Fatalf("reduced top link count = %d, want one selected same-pop alternative", got)
	}
}

func TestCDoAllPotentialReductionsCollapsesSamePopWithTrailingExtra(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	rightLow := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightHigh := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightHigh.dynamicPrecedence = 9
	extraLow := newLeafNodeInArena(arena, 5, false, 2, 3, Point{Column: 2}, Point{Column: 3})
	extraLow.setExtra(true)
	extraHigh := newLeafNodeInArena(arena, 6, false, 2, 3, Point{Column: 2}, Point{Column: 3})
	extraHigh.setExtra(true)

	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightLowNode := scratch.allocNode(newStackEntryNode(3, rightLow), leftNode, 3)
	rightHighNode := scratch.allocNode(newStackEntryNode(3, rightHigh), leftNode, 3)
	head := scratch.allocNode(newStackEntryNode(3, extraLow), rightLowNode, 4)
	head.extraLinks = append(head.extraLinks, gssMainLink{
		prev:  rightHighNode,
		entry: newStackEntryNode(3, extraHigh),
	})
	start := glrStack{gss: gssStack{head: head}, byteOffset: 3}

	nodeCount := 0
	versions, canShift := parser.cDoAllPotentialReductions(nil, start, 0, true, Token{}, &nodeCount, arena, nil, &scratch, nil)
	if canShift {
		t.Fatal("canShift = true, want false")
	}
	if len(versions) != 1 {
		t.Fatalf("version count = %d, want one selected version", len(versions))
	}
	if got := versions[0].gss.head.linkCount(); got != 1 {
		t.Fatalf("top link count = %d, want one selected extra replay path", got)
	}
	if stackEntryNode(versions[0].top()) != extraHigh {
		t.Fatal("selected trailing extra did not come from the C-selected same-pop child path")
	}
	parent := stackEntryNode(versions[0].gss.head.prev.entry)
	if parent == nil || parent.symbol != 4 {
		t.Fatalf("replayed extra predecessor = %+v, want reduced parent", parent)
	}
	if len(parent.children) != 2 || parent.children[1] != rightHigh {
		t.Fatal("trailing-extra same-pop collapse did not preserve selected parent children")
	}
}

func TestCAppendActionReductionVersionsCollapsesSamePopBeforeOlderMerge(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	olderPop := scratch.allocNode(stackEntry{state: 8}, nil, 1)
	popTo := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	original := glrStack{gss: gssStack{head: scratch.allocNode(stackEntry{state: 3}, popTo, 2)}, byteOffset: 2}

	olderChild := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	lowChild := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	highChild := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	highChild.dynamicPrecedence = 7
	olderParent := newParentNodeInArena(arena, 4, true, []*Node{olderChild}, nil, 0)
	lowParent := newParentNodeInArena(arena, 4, true, []*Node{lowChild}, nil, 0)
	highParent := newParentNodeInArena(arena, 4, true, []*Node{highChild}, nil, 0)
	highParent.dynamicPrecedence = highChild.dynamicPrecedence

	versions := []glrStack{
		{gss: gssStack{head: scratch.allocNode(newStackEntryNode(9, olderParent), olderPop, 2)}, byteOffset: 2},
		original,
	}
	candidates := []glrStack{
		{gss: gssStack{head: scratch.allocNode(newStackEntryNode(9, lowParent), popTo, 2)}, byteOffset: 2},
		{gss: gssStack{head: scratch.allocNode(newStackEntryNode(9, highParent), popTo, 2)}, byteOffset: 2},
	}

	versions, reductionVersion := parser.cAppendActionReductionVersions(versions, candidates, 1, arena)
	if reductionVersion != -1 {
		t.Fatalf("reduction version = %d, want merge into older existing version", reductionVersion)
	}
	if len(versions) != 2 {
		t.Fatalf("version count = %d, want older plus original", len(versions))
	}
	if got := versions[0].gss.head.linkCount(); got != 2 {
		t.Fatalf("older merged head link count = %d, want one selected reduction link added", got)
	}
	_, mergedEntry := versions[0].gss.head.link(1)
	if stackEntryNode(mergedEntry) != highParent {
		t.Fatal("older merge received uncollapsed lower-precedence same-pop parent")
	}
}

func newCRecoverySyntheticReduceParser() *Parser {
	lang := &Language{
		TokenCount:  3,
		StateCount:  10,
		SymbolCount: 7,
		ParseTable: [][]uint16{
			nil,
			{0, 0, 0, 0, 2}, // goto parent(4) -> state 9.
			nil,
			{0, 1, 1},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 9}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "a", Visible: true, Named: true},
			{Name: "b", Visible: true, Named: true},
			{Name: "unused", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
			{Name: "extra_low", Visible: false, Named: false},
			{Name: "extra_high", Visible: false, Named: false},
		},
	}
	return &Parser{language: lang, denseLimit: len(lang.ParseTable)}
}

// TestCDoAllPotentialReductionsKeepsShiftableOriginalWithReductionFork pins
// C's version bookkeeping (parser.c ts_parser__do_all_potential_reductions):
// a version whose state has a non-extra shift action for SOME token is KEPT
// as its own path, and its reduction results become separate versions —
// `if (has_shift_action) can_shift = true; else if (reduction_version ...)
// renumber`. Replacing the shiftable original with its reduction drops the
// merged version's original-shape path and the summary entries C's
// strategy-1 scan elects from it (authzed stray backtick: the (depth 1,
// pre-reduction state) entry closes binary_expression on the newline).
func TestCDoAllPotentialReductionsKeepsShiftableOriginalWithReductionFork(t *testing.T) {
	lang := &Language{
		TokenCount:  2,
		StateCount:  10,
		SymbolCount: 5,
		ParseTable: [][]uint16{
			nil,
			{0, 0, 0, 0, 2}, // goto parent(4) -> state 9.
			nil,
			{0, 1}, // token 1 has both reduce(parent) and shift.
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
				{Type: ParseActionShift, State: 8},
			}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 9}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "tok", Visible: true, Named: true},
			{Name: "leaf", Visible: true, Named: true},
			{Name: "unused", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, denseLimit: len(lang.ParseTable)}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	start := newGLRStack(1)
	start.pushEntry(newStackEntryNode(3, leaf), nil, nil)

	nodeCount := 0
	versions, canShift := parser.cDoAllPotentialReductions(nil, start, 0, true, Token{}, &nodeCount, arena, nil, nil, nil)
	if !canShift {
		t.Fatal("canShift = false, want true from the shiftable original state")
	}
	if len(versions) != 2 {
		t.Fatalf("version count = %d, want shiftable original retained alongside its reduction fork", len(versions))
	}
	if got := versions[0].top().state; got != StateID(3) {
		t.Fatalf("original top state = %d, want pre-reduction state 3 kept", got)
	}
	if got := versions[1].top().state; got != StateID(9) {
		t.Fatalf("reduction fork top state = %d, want reduced goto state 9", got)
	}
	if top := versions[1].top(); !stackEntryHasNode(top) || stackEntryNode(top).symbol != 4 {
		t.Fatalf("reduction fork top entry = %+v, want reduced parent symbol", top)
	}
}
func TestCDoAllPotentialReductionsDistinguishesEOFFromAnyLookahead(t *testing.T) {
	lang := &Language{
		TokenCount:  2,
		StateCount:  4,
		SymbolCount: 3,
		ParseTable: [][]uint16{
			nil,
			{1: 1}, // state 1 can shift a real token, but has no EOF action.
			{0: 2}, // state 2 can accept EOF.
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "tok", Visible: true, Named: true},
			{Name: "node", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, denseLimit: len(lang.ParseTable)}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	nodeCount := 0
	versions, canShift := parser.cDoAllPotentialReductions(nil, newGLRStack(1), 0, true, Token{}, &nodeCount, arena, nil, nil, nil)
	if !canShift || len(versions) != 1 {
		t.Fatalf("any-lookahead reductions: canShift=%v versions=%d, want true/1", canShift, len(versions))
	}

	versions, canShift = parser.cDoAllPotentialReductions(nil, newGLRStack(1), 0, false, Token{}, &nodeCount, arena, nil, nil, nil)
	if canShift || len(versions) != 0 {
		t.Fatalf("exact EOF reductions on non-EOF state: canShift=%v versions=%d, want false/0", canShift, len(versions))
	}

	versions, canShift = parser.cDoAllPotentialReductions(nil, newGLRStack(2), 0, false, Token{}, &nodeCount, arena, nil, nil, nil)
	if !canShift || len(versions) != 1 {
		t.Fatalf("exact EOF accept reductions: canShift=%v versions=%d, want true/1", canShift, len(versions))
	}
}

func TestCResultSelectionPrefersLaterEqualErrorCostTree(t *testing.T) {
	lang := &Language{
		TokenCount:  2,
		StateCount:  2,
		SymbolCount: 3,
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "translation_unit", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, errorCostCompetition: true}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leftMissing := newLeafNodeInArena(arena, 1, true, 0, 0, Point{}, Point{})
	leftMissing.setMissing(true)
	leftMissing.setHasError(true)
	rightMissing := newLeafNodeInArena(arena, 1, true, 0, 0, Point{}, Point{})
	rightMissing.setMissing(true)
	rightMissing.setHasError(true)

	left := glrStack{accepted: true}
	left.pushEntry(newStackEntryNode(1, leftMissing), nil, nil)
	right := glrStack{accepted: true}
	right.pushEntry(newStackEntryNode(1, rightMissing), nil, nil)

	if lc, rc := parser.cStackResultErrorCost(&left), parser.cStackResultErrorCost(&right); lc == 0 || lc != rc {
		t.Fatalf("test setup costs = %d/%d, want equal nonzero", lc, rc)
	}
	if got := stackCompareForResultSelection(parser, arena, &right, &left, false); got <= 0 {
		t.Fatalf("later equal-error candidate compare = %d, want preferred", got)
	}
}

func makeCCondenseMissingGroupTestStack(topState StateID, nodes int, missingTop bool) glrStack {
	if nodes < 1 {
		nodes = 1
	}
	s := newGLRStack(1)
	for i := 0; i < nodes; i++ {
		start := uint32(i)
		end := start + 1
		n := NewLeafNode(1, true, start, end, Point{Column: start}, Point{Column: end})
		if missingTop && i == nodes-1 {
			n.startByte = start
			n.endByte = start
			n.startPoint = Point{Column: start}
			n.endPoint = Point{Column: start}
			n.setMissing(true)
			n.setHasError(true)
		}
		state := StateID(2 + i)
		if i == nodes-1 {
			state = topState
		}
		s.push(state, n, nil, nil)
	}
	if missingTop {
		s.byteOffset = uint32(nodes)
	}
	return s
}

func TestCRecoveryRelevantStackIncludesMissingGroup(t *testing.T) {
	if cRecoveryRelevantStack([]glrStack{{cRecoverMissingGroup: &cRecGroup{}}}) != true {
		t.Fatal("cRecoveryRelevantStack = false for missing recovery group")
	}
}

func TestCCondenseAndResumeComparesMissingGroupStacks(t *testing.T) {
	lang := &Language{
		TokenCount:  2,
		StateCount:  8,
		SymbolCount: 2,
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "item", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, errorCostCompetition: true}
	clean := makeCCondenseMissingGroupTestStack(7, 4, false)
	missing := makeCCondenseMissingGroupTestStack(7, 4, true)
	missing.cRecoverMissingGroup = &cRecGroup{}
	if clean.top().state != missing.top().state || clean.byteOffset != missing.byteOffset {
		t.Fatalf("test setup headers differ: clean=%d@%d missing=%d@%d", clean.top().state, clean.byteOffset, missing.top().state, missing.byteOffset)
	}
	if cleanCost, missingCost := parser.cStackErrorCost(&clean), parser.cStackErrorCost(&missing); cleanCost >= missingCost {
		t.Fatalf("test setup costs clean=%d missing=%d, want clean lower", cleanCost, missingCost)
	}

	var nodeCount int
	condensed, resumed, _ := parser.cCondenseAndResume([]glrStack{missing, clean}, nil, Token{Symbol: 1}, &nodeCount, nil, nil, nil, nil)
	if resumed {
		t.Fatal("cCondenseAndResume resumed without paused/error-state versions")
	}
	if len(condensed) != 1 {
		t.Fatalf("condensed len = %d, want 1", len(condensed))
	}
	if condensed[0].cRecoverMissingGroup != nil {
		t.Fatal("condense kept missing-group stack; want lower-cost clean stack")
	}
}
