package gotreesitter

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseRuntimeReportsAcceptedOnCompleteParse(t *testing.T) {
	EnableArenaBreakdown(true)
	defer EnableArenaBreakdown(false)

	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	// Pin to production: this test asserts production-engine runtime internals
	// (iteration limits, arena accounting) the compact candidate route does not
	// populate.
	parser.SetAdmissionCandidateRoute(false)

	tree := mustParse(t, parser, []byte("1+2"))
	rt := tree.ParseRuntime()

	if rt.StopReason != ParseStopAccepted {
		t.Fatalf("StopReason = %q, want %q", rt.StopReason, ParseStopAccepted)
	}
	if tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = true, want false")
	}
	if rt.TokenSourceEOFEarly {
		t.Fatal("TokenSourceEOFEarly = true, want false")
	}
	if rt.Truncated {
		t.Fatal("Truncated = true, want false")
	}
	if rt.IterationLimit <= 0 {
		t.Fatalf("IterationLimit = %d, want > 0", rt.IterationLimit)
	}
	if rt.StackDepthLimit <= 0 {
		t.Fatalf("StackDepthLimit = %d, want > 0", rt.StackDepthLimit)
	}
	if rt.NodeLimit <= 0 {
		t.Fatalf("NodeLimit = %d, want > 0", rt.NodeLimit)
	}
	if rt.Iterations <= 0 {
		t.Fatalf("Iterations = %d, want > 0", rt.Iterations)
	}
	if rt.LeafNodesConstructed == 0 {
		t.Fatal("LeafNodesConstructed = 0, want > 0")
	}
	if rt.ParentNodesConstructed == 0 {
		t.Fatal("ParentNodesConstructed = 0, want > 0")
	}
	if rt.FinalNodes == 0 {
		t.Fatal("FinalNodes = 0, want > 0")
	}
	if got, want := rt.FinalParentNodes+rt.FinalLeafNodes, rt.FinalNodes; got != want {
		t.Fatalf("final parent+leaf nodes = %d, want %d", got, want)
	}
	if got, want := rt.FinalFieldedParentNodes+rt.FinalUnfieldedParentNodes, rt.FinalParentNodes; got != want {
		t.Fatalf("final fielded+unfielded parents = %d, want %d", got, want)
	}
	if got, want := rt.FinalVisibleParentNodes+rt.FinalHiddenParentNodes, rt.FinalParentNodes; got != want {
		t.Fatalf("final visible+hidden parents = %d, want %d", got, want)
	}
	if got := rt.FinalHiddenParentNodes; got != 0 {
		t.Fatalf("FinalHiddenParentNodes = %d, want 0", got)
	}
	if got := rt.FinalCheckpointLeafNodes; got != 0 {
		t.Fatalf("FinalCheckpointLeafNodes = %d, want 0", got)
	}
	if rt.FinalChildPointers == 0 {
		t.Fatal("FinalChildPointers = 0, want > 0")
	}
	if rt.NoTreeReduceNodesConstructed != 0 {
		t.Fatalf("NoTreeReduceNodesConstructed = %d, want 0", rt.NoTreeReduceNodesConstructed)
	}
	if rt.NoTreeLeafNodesConstructed != 0 {
		t.Fatalf("NoTreeLeafNodesConstructed = %d, want 0", rt.NoTreeLeafNodesConstructed)
	}
	breakdown := assertParseRuntimeArenaBreakdown(t, tree, rt)
	if got := breakdown.NoTreePlaceholderNodesConstructed; got != 0 {
		t.Fatalf("NoTreePlaceholderNodesConstructed = %d, want 0", got)
	}
	if got, want := breakdown.FieldedParentNodesConstructed+breakdown.UnfieldedParentNodesConstructed, rt.ParentNodesConstructed; got != want {
		t.Fatalf("parent field attribution = %d, want %d", got, want)
	}
	if got, want := breakdown.ParentConstructedChildLen0+breakdown.ParentConstructedChildLen1+breakdown.ParentConstructedChildLen2+breakdown.ParentConstructedChildLen3+breakdown.ParentConstructedChildLen4Plus, rt.ParentNodesConstructed; got != want {
		t.Fatalf("parent child-count attribution = %d, want %d", got, want)
	}
	if got, want := breakdown.ParentConstructedNoLinks+breakdown.ParentConstructedWithLinks, rt.ParentNodesConstructed; got != want {
		t.Fatalf("parent link attribution = %d, want %d", got, want)
	}
}

func TestAcceptedPrefixWithRealTailDoesNotReturnCleanParse(t *testing.T) {
	lang := buildPrefixAcceptLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("ab"))
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	rt := tree.ParseRuntime()

	if got, want := rt.StopReason, ParseStopAccepted; got != want {
		t.Fatalf("StopReason = %q, want %q; runtime=%s root=%s", got, want, rt.Summary(), root.SExpr(lang))
	}
	if rt.Truncated {
		t.Fatalf("Truncated = true, want false; runtime=%s root=%s", rt.Summary(), root.SExpr(lang))
	}
	if root.HasError() {
		t.Fatalf("root has error, want clean full parse; runtime=%s root=%s", rt.Summary(), root.SExpr(lang))
	}
	if got, want := root.EndByte(), uint32(2); got != want {
		t.Fatalf("root end = %d, want %d; runtime=%s root=%s", got, want, rt.Summary(), root.SExpr(lang))
	}
	if got, want := root.Text(tree.Source()), "ab"; got != want {
		t.Fatalf("root text = %q, want %q; root=%s", got, want, root.SExpr(lang))
	}
}

func TestAcceptedPrefixOnlyBeforeRealTailDemotesToError(t *testing.T) {
	lang := buildPrefixAcceptLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("abb"))
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	rt := tree.ParseRuntime()

	if rt.StopReason == ParseStopAccepted && !rt.Truncated && !root.HasError() {
		t.Fatalf("clean accepted prefix returned before real tail; runtime=%s root=%s", rt.Summary(), root.SExpr(lang))
	}
	if root.EndByte() >= uint32(len(tree.Source())) && !root.HasError() {
		t.Fatalf("real tail was reported as cleanly consumed; runtime=%s root=%s", rt.Summary(), root.SExpr(lang))
	}
}

func TestAcceptedPrefixWithPaddingTailRemainsClean(t *testing.T) {
	lang := buildPrefixAcceptLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("a \n\t"))
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	rt := tree.ParseRuntime()

	if got, want := rt.StopReason, ParseStopAccepted; got != want {
		t.Fatalf("StopReason = %q, want %q; runtime=%s root=%s", got, want, rt.Summary(), root.SExpr(lang))
	}
	if rt.Truncated {
		t.Fatalf("Truncated = true, want false for parser-padding tail; runtime=%s root=%s", rt.Summary(), root.SExpr(lang))
	}
	if root.HasError() {
		t.Fatalf("root has error, want clean padding-tail parse; runtime=%s root=%s", rt.Summary(), root.SExpr(lang))
	}
	if got, want := root.EndByte(), uint32(len(tree.Source())); got != want {
		t.Fatalf("root end = %d, want %d for padding-tail parse; runtime=%s root=%s", got, want, rt.Summary(), root.SExpr(lang))
	}
}

func TestAcceptedPrefixWithIncludedRangePaddingTailRemainsClean(t *testing.T) {
	lang := buildPrefixAcceptLanguage()
	parser := NewParser(lang)
	source := []byte("aXXX \t")
	parser.SetIncludedRanges([]Range{
		{StartByte: 0, EndByte: 1},
		{StartByte: 4, EndByte: uint32(len(source))},
	})

	tree := mustParse(t, parser, source)
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	rt := tree.ParseRuntime()

	if got, want := rt.StopReason, ParseStopAccepted; got != want {
		t.Fatalf("StopReason = %q, want %q; runtime=%s root=%s", got, want, rt.Summary(), root.SExpr(lang))
	}
	if rt.Truncated {
		t.Fatalf("Truncated = true, want false for included padding tail; runtime=%s root=%s", rt.Summary(), root.SExpr(lang))
	}
	if root.HasError() {
		t.Fatalf("root has error, want clean included padding-tail parse; runtime=%s root=%s", rt.Summary(), root.SExpr(lang))
	}
	if !retryTreeCoversExpectedEOF(tree) {
		t.Fatalf("retryTreeCoversExpectedEOF = false for included padding tail; runtime=%s root=%s", rt.Summary(), root.SExpr(lang))
	}
}

func buildConflictReduceFrontierLanguage(postReduceBAction uint16, frontierActions []ParseAction) *Language {
	return &Language{
		Name:              "conflict_reduce_frontier",
		SymbolCount:       4,
		TokenCount:        3,
		StateCount:        4,
		InitialState:      0,
		ProductionIDCount: 4,
		SymbolNames:       []string{"end", "a", "b", "source"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false},
			{Name: "a", Visible: true},
			{Name: "b", Visible: true},
			{Name: "source", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
			{Actions: frontierActions},
		},
		ParseTable: [][]uint16{
			{0, 1, 0, 3},
			{2, 0, 6, 0},
			{0, 0, postReduceBAction, 0},
			{5, 0, 0, 0},
		},
	}
}

func newConflictReduceFrontierStackForTest(parser *Parser, arena *nodeArena, entries *glrEntryScratch, gss *gssScratch) glrStack {
	a := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	a.parseState = 1
	stack := newGLRStack(0)
	parser.pushStackNode(&stack, 1, a, entries, gss)
	return stack
}

func applyConflictFrontierReduceForTest(parser *Parser, stack *glrStack, reduce ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entries *glrEntryScratch, gss *gssScratch, tmp *[]stackEntry, trackChildErrors *bool) conflictReduceFrontierSeed {
	beforeState, beforeByte, beforeDepth := stackTraceState(stack)
	parser.applyAction(nil, stack, reduce, tok, anyReduced, nodeCount, arena, entries, gss, tmp, false, trackChildErrors)
	afterState, afterByte, afterDepth := stackTraceState(stack)
	return conflictReduceFrontierSeed{
		action:      reduce,
		beforeState: beforeState,
		beforeByte:  beforeByte,
		beforeDepth: beforeDepth,
		afterState:  afterState,
		afterByte:   afterByte,
		afterDepth:  afterDepth,
	}
}

func TestConflictReduceFrontierScratchRetainsExactFlags(t *testing.T) {
	if conflictReduceFrontierTableSize&(conflictReduceFrontierTableSize-1) != 0 {
		t.Fatalf("table size %d is not a power of two", conflictReduceFrontierTableSize)
	}
	if conflictReduceFrontierTableSize < maxConflictReduceFrontierActions+1 {
		t.Fatalf("table size %d cannot hold the %d-action frontier plus its seed", conflictReduceFrontierTableSize, maxConflictReduceFrontierActions)
	}
	var scratch conflictReduceFrontierScratch
	scratch.begin()
	if got, want := len(scratch.entries), conflictReduceFrontierTableSize; got != want {
		t.Fatalf("entries = %d, want %d", got, want)
	}

	keys := make([]frontierReduceKey, maxConflictReduceFrontierActions+1)
	for i := range keys {
		keys[i] = frontierReduceKey{
			state:             StateID(i),
			byteOffset:        uint32(i * 7),
			depth:             i + 1,
			symbol:            Symbol(i * 3),
			childCount:        uint8(i),
			productionID:      uint16(i * 5),
			dynamicPrecedence: int16(i - 128),
			actionState:       StateID(i * 11),
		}
		scratch.add(keys[i], conflictReduceFrontierSeen)
		if i%2 == 0 {
			scratch.add(keys[i], conflictReduceFrontierForked)
		}
	}
	for i, key := range keys {
		if !scratch.has(key, conflictReduceFrontierSeen) {
			t.Fatalf("key %d lost seen flag", i)
		}
		if got, want := scratch.has(key, conflictReduceFrontierForked), i%2 == 0; got != want {
			t.Fatalf("key %d forked flag = %t, want %t", i, got, want)
		}
	}

	oldGeneration := scratch.generation
	scratch.begin()
	if scratch.generation == oldGeneration {
		t.Fatal("begin did not advance generation")
	}
	for i, key := range keys {
		if scratch.has(key, conflictReduceFrontierSeen) || scratch.has(key, conflictReduceFrontierForked) {
			t.Fatalf("key %d survived generation reset", i)
		}
	}

	allocs := testing.AllocsPerRun(100, func() {
		scratch.begin()
		scratch.add(keys[0], conflictReduceFrontierSeen)
		if !scratch.has(keys[0], conflictReduceFrontierSeen) {
			panic("frontier scratch lost warm entry")
		}
	})
	if allocs != 0 {
		t.Fatalf("warm frontier scratch allocations = %g, want 0", allocs)
	}
}

func TestConflictReduceFrontierCompletesSingleShift(t *testing.T) {
	lang := buildConflictReduceFrontierLanguage(4, []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionShift, State: 3},
	})
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	var entries glrEntryScratch
	var gss gssScratch
	var tmp []stackEntry
	nodeCount := 0
	anyReduced := false
	trackChildErrors := false

	stack := newConflictReduceFrontierStackForTest(parser, arena, &entries, &gss)
	tok := Token{Symbol: 2, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}}
	reduce := lang.ParseActions[6].Actions[0]

	seed := applyConflictFrontierReduceForTest(parser, &stack, reduce, tok, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, &trackChildErrors)
	if !anyReduced {
		t.Fatal("conflict reduce did not mark anyReduced")
	}
	if stack.dead || stack.shifted || stack.accepted {
		t.Fatalf("after reduce: dead=%t shifted=%t accepted=%t", stack.dead, stack.shifted, stack.accepted)
	}
	if got, want := stack.top().state, StateID(2); got != want {
		t.Fatalf("after reduce state = %d, want %d", got, want)
	}

	nextBranchOrder := uint64(99)
	parser.completeConflictReduceFrontier(nil, &stack, tok, seed, 1, 0, func() uint64 {
		order := nextBranchOrder
		nextBranchOrder++
		return order
	}, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, false, &trackChildErrors)
	if stack.dead {
		t.Fatal("frontier completion killed stack")
	}
	if !stack.shifted {
		t.Fatal("frontier completion did not shift same-lookahead token")
	}
	if got, want := stack.top().state, StateID(3); got != want {
		t.Fatalf("after frontier completion state = %d, want %d", got, want)
	}
	if got, want := stack.byteOffset, uint32(2); got != want {
		t.Fatalf("after frontier completion byte offset = %d, want %d", got, want)
	}
}

func TestConflictReduceFrontierDegenerateSameReduceShiftCompletes(t *testing.T) {
	lang := buildConflictReduceFrontierLanguage(6, []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionShift, State: 3},
	})
	parser := NewParser(lang)
	parser.stopActionDiag = &parseStopActionDiagnostic{
		lastReduceCaptured:          true,
		lastReduceSymbol:            99,
		lastReduceChildCount:        7,
		lastReduceProductionID:      11,
		lastReduceDynamicPrecedence: 13,
		lookaheadSymbol:             99,
		lookaheadStartByte:          99,
		lookaheadEndByte:            100,
		lookaheadNoLookahead:        true,
	}
	arena := newNodeArena(arenaClassFull)
	var entries glrEntryScratch
	var gss gssScratch
	var tmp []stackEntry
	nodeCount := 0
	anyReduced := false
	trackChildErrors := false

	stack := newConflictReduceFrontierStackForTest(parser, arena, &entries, &gss)
	tok := Token{Symbol: 2, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}}
	reduce := lang.ParseActions[6].Actions[0]
	seed := applyConflictFrontierReduceForTest(parser, &stack, reduce, tok, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, &trackChildErrors)

	nextBranchOrder := uint64(99)
	parser.completeConflictReduceFrontier(nil, &stack, tok, seed, 1, 0, func() uint64 {
		order := nextBranchOrder
		nextBranchOrder++
		return order
	}, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, false, &trackChildErrors)
	if !stack.dead {
		t.Fatal("degenerate same-reduce cycle left original branch live")
	}
	if got, want := len(parser.pendingFrontierForkStacks), 1; got != want {
		t.Fatalf("pending frontier forks = %d, want %d", got, want)
	}
	fork := parser.pendingFrontierForkStacks[0]
	if !fork.shifted {
		t.Fatal("terminal frontier fork did not shift")
	}
	if got, want := fork.top().state, StateID(3); got != want {
		t.Fatalf("terminal frontier fork state = %d, want %d", got, want)
	}
	if got, want := fork.branchOrder, uint64(99); got != want {
		t.Fatalf("terminal frontier fork branchOrder = %d, want %d", got, want)
	}
	if got, want := nextBranchOrder, uint64(100); got != want {
		t.Fatalf("next branch order = %d, want %d", got, want)
	}
}

func TestConflictReduceFrontierSkipsRepetitionShiftAfterCFold(t *testing.T) {
	lang := buildConflictReduceFrontierLanguage(6, []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionShift, State: 3, Repetition: true},
	})
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	var entries glrEntryScratch
	var gss gssScratch
	var tmp []stackEntry
	nodeCount := 0
	anyReduced := false
	trackChildErrors := false

	stack := newConflictReduceFrontierStackForTest(parser, arena, &entries, &gss)
	tok := Token{Symbol: 2, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}}
	reduce := lang.ParseActions[6].Actions[0]
	seed := applyConflictFrontierReduceForTest(parser, &stack, reduce, tok, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, &trackChildErrors)
	seed.skipRepetitionShift = true

	parser.completeConflictReduceFrontier(nil, &stack, tok, seed, 1, 0, nil, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, false, &trackChildErrors)
	if stack.dead || stack.shifted {
		t.Fatalf("C repetition fold branch dead=%t shifted=%t, want live reduce branch", stack.dead, stack.shifted)
	}
	if got := len(parser.pendingFrontierForkStacks); got != 0 {
		t.Fatalf("pending frontier forks = %d, want 0", got)
	}
}

func TestConflictReduceFrontierSameHeaderFirstReduceStaysLive(t *testing.T) {
	lang := &Language{
		Name:              "conflict_reduce_frontier_same_header_first",
		SymbolCount:       4,
		TokenCount:        3,
		StateCount:        4,
		InitialState:      0,
		ProductionIDCount: 3,
		SymbolNames:       []string{"end", "a", "b", "wrapper"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false},
			{Name: "a", Visible: true},
			{Name: "b", Visible: true},
			{Name: "wrapper", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{},
			{},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 1},
				{Type: ParseActionShift, State: 3},
			}},
		},
		ParseTable: [][]uint16{
			{0, 1, 0, 0},
			{0, 0, 0, 2},
			{0, 0, 6, 0},
			{0, 0, 0, 0},
		},
	}
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	var entries glrEntryScratch
	var gss gssScratch
	var tmp []stackEntry
	nodeCount := 0
	anyReduced := false
	trackChildErrors := false

	stack := newGLRStack(0)
	a := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	a.parseState = 1
	parser.pushStackNode(&stack, 1, a, &entries, &gss)
	wrapper := newParentNodeInArena(arena, 3, true, []*Node{a}, nil, 0)
	wrapper.parseState = 2
	wrapper.endByte = 1
	wrapper.endPoint = Point{Column: 1}
	parser.pushStackNode(&stack, 2, wrapper, &entries, &gss)

	tok := Token{Symbol: 2, StartByte: 2, EndByte: 3, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 3}}
	seed := conflictReduceFrontierSeed{
		action:      ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 0},
		beforeState: 1,
		beforeByte:  1,
		beforeDepth: 2,
		afterState:  2,
		afterByte:   1,
		afterDepth:  2,
	}

	parser.completeConflictReduceFrontier([]byte("xxb"), &stack, tok, seed, 1, 0, nil, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, false, &trackChildErrors)
	if stack.dead {
		t.Fatal("same-header first reduce killed original branch")
	}
	if stack.shifted {
		t.Fatal("same-header first reduce shifted original branch")
	}
	if got, want := stack.top().state, StateID(2); got != want {
		t.Fatalf("same-header reduce state = %d, want %d", got, want)
	}
	if got, want := stack.byteOffset, uint32(1); got != want {
		t.Fatalf("same-header reduce byte = %d, want %d", got, want)
	}
	if got := len(parser.pendingFrontierForkStacks); got != 0 {
		t.Fatalf("pending frontier forks = %d, want 0 after guarded terminal gap", got)
	}
}

func TestConflictReduceFrontierDepthChangingSameReduceDoesNotFakeDuplicate(t *testing.T) {
	lang := &Language{
		Name:              "conflict_reduce_frontier_depth",
		SymbolCount:       6,
		TokenCount:        3,
		StateCount:        7,
		InitialState:      0,
		ProductionIDCount: 2,
		SymbolNames:       []string{"end", "a", "b", "unused", "pair", "source"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false},
			{Name: "a", Visible: true},
			{Name: "b", Visible: true},
			{Name: "unused", Visible: true, Named: true},
			{Name: "pair", Visible: true, Named: true},
			{Name: "source", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 6}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 4, ChildCount: 2, ProductionID: 1},
				{Type: ParseActionShift, State: 5},
			}},
		},
		ParseTable: [][]uint16{
			{0, 1, 0, 0, 4, 0},
			{0, 2, 0, 0, 5, 0},
			{0, 3, 0, 0, 0, 0},
			{0, 0, 6, 0, 0, 0},
			{0, 0, 0, 0, 0, 0},
			{0, 0, 0, 0, 0, 0},
			{0, 0, 6, 0, 0, 0},
		},
	}
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	var entries glrEntryScratch
	var gss gssScratch
	var tmp []stackEntry
	nodeCount := 0
	anyReduced := false
	trackChildErrors := false

	stack := newGLRStack(0)
	for i, state := range []StateID{1, 2, 6} {
		start := uint32(i)
		end := start + 1
		a := newLeafNodeInArena(arena, 1, true, start, end, Point{Column: start}, Point{Column: end})
		a.parseState = state
		parser.pushStackNode(&stack, state, a, &entries, &gss)
	}
	tok := Token{Symbol: 2, StartByte: 3, EndByte: 4, StartPoint: Point{Column: 3}, EndPoint: Point{Column: 4}}
	reduce := lang.ParseActions[6].Actions[0]
	seed := applyConflictFrontierReduceForTest(parser, &stack, reduce, tok, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, &trackChildErrors)
	if got, want := stack.depth(), 3; got != want {
		t.Fatalf("after seed reduce depth = %d, want %d", got, want)
	}
	if got, want := stack.top().state, StateID(3); got != want {
		t.Fatalf("after seed reduce state = %d, want %d", got, want)
	}

	parser.completeConflictReduceFrontier(nil, &stack, tok, seed, 1, 0, nil, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, false, &trackChildErrors)
	if stack.dead {
		t.Fatal("depth-changing frontier reduce killed stack")
	}
	if stack.shifted {
		t.Fatal("depth-changing frontier shifted original instead of preserving reduce branch")
	}
	if got, want := stack.depth(), 2; got != want {
		t.Fatalf("after frontier reduce depth = %d, want %d", got, want)
	}
	if got, want := stack.top().state, StateID(4); got != want {
		t.Fatalf("after frontier reduce state = %d, want %d", got, want)
	}
	if got, want := len(parser.pendingFrontierForkStacks), 1; got != want {
		t.Fatalf("pending frontier forks = %d, want %d", got, want)
	}
	fork := parser.pendingFrontierForkStacks[0]
	if !fork.shifted {
		t.Fatal("terminal frontier fork did not shift")
	}
	if got, want := fork.top().state, StateID(5); got != want {
		t.Fatalf("terminal frontier fork state = %d, want %d", got, want)
	}
}

func TestConflictReduceFrontierRuntimeReduceTerminalPreservesReduceBranch(t *testing.T) {
	lang := &Language{
		Name:              "conflict_reduce_frontier_runtime",
		SymbolCount:       6,
		TokenCount:        3,
		StateCount:        9,
		InitialState:      0,
		ProductionIDCount: 3,
		SymbolNames:       []string{"end", "a", "b", "unused", "pair", "source"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false},
			{Name: "a", Visible: true},
			{Name: "b", Visible: true},
			{Name: "unused", Visible: true, Named: true},
			{Name: "pair", Visible: true, Named: true},
			{Name: "source", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 6}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 4, ChildCount: 2, ProductionID: 1},
				{Type: ParseActionShift, State: 5},
			}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 7}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 5, ChildCount: 2, ProductionID: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 8}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: [][]uint16{
			{0, 1, 0, 0, 4, 9},
			{0, 2, 0, 0, 5, 0},
			{0, 3, 0, 0, 0, 0},
			{0, 0, 6, 0, 0, 0},
			{0, 0, 7, 0, 0, 0},
			{0, 0, 0, 0, 0, 0},
			{0, 0, 6, 0, 0, 0},
			{8, 0, 0, 0, 0, 0},
			{10, 0, 0, 0, 0, 0},
		},
	}
	parser := NewParser(lang)
	source := []byte("aaab")
	tree, err := parser.ParseWithTokenSource(source, &slowArithmeticTokenSource{
		tokens: []Token{
			{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
			{Symbol: 1, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}},
			{Symbol: 1, StartByte: 2, EndByte: 3, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 3}},
			{Symbol: 2, StartByte: 3, EndByte: 4, StartPoint: Point{Column: 3}, EndPoint: Point{Column: 4}},
			{Symbol: 0, StartByte: 4, EndByte: 4, StartPoint: Point{Column: 4}, EndPoint: Point{Column: 4}},
		},
	})
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	defer tree.Release()
	rt := tree.ParseRuntime()
	if got, want := rt.StopReason, ParseStopAccepted; got != want {
		t.Fatalf("StopReason = %q, want %q; runtime=%s", got, want, rt.Summary())
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	sexpr := root.SExpr(lang)
	if !strings.Contains(sexpr, "pair") {
		t.Fatalf("root = %s, want reduce branch pair; runtime=%s", sexpr, rt.Summary())
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d; runtime=%s tree=%s", got, want, rt.Summary(), sexpr)
	}
}

func TestConflictReduceFrontierDoesNotCollapseOtherReduceShift(t *testing.T) {
	lang := buildConflictReduceFrontierLanguage(6, []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 0},
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 1, DynamicPrecedence: 1},
		{Type: ParseActionShift, State: 3},
	})
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	var entries glrEntryScratch
	var gss gssScratch
	var tmp []stackEntry
	nodeCount := 0
	anyReduced := false
	trackChildErrors := false

	stack := newConflictReduceFrontierStackForTest(parser, arena, &entries, &gss)
	tok := Token{Symbol: 2, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}}
	reduce := lang.ParseActions[6].Actions[0]
	seed := applyConflictFrontierReduceForTest(parser, &stack, reduce, tok, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, &trackChildErrors)

	parser.completeConflictReduceFrontier(nil, &stack, tok, seed, 1, 0, nil, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, false, &trackChildErrors)
	if stack.dead {
		t.Fatal("frontier completion killed stack")
	}
	if stack.shifted {
		t.Fatal("non-degenerate same-reduce+other-reduce+shift frontier was collapsed")
	}
	if got, want := stack.top().state, StateID(2); got != want {
		t.Fatalf("after non-degenerate frontier state = %d, want %d", got, want)
	}
}

func TestConflictReduceFrontierRecoverMarksTokenConsumed(t *testing.T) {
	lang := buildConflictReduceFrontierLanguage(6, []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionRecover, State: 3},
	})
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	var entries glrEntryScratch
	var gss gssScratch
	var tmp []stackEntry
	nodeCount := 0
	anyReduced := false
	trackChildErrors := false

	stack := newConflictReduceFrontierStackForTest(parser, arena, &entries, &gss)
	tok := Token{Symbol: 2, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}}
	reduce := lang.ParseActions[6].Actions[0]
	seed := applyConflictFrontierReduceForTest(parser, &stack, reduce, tok, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, &trackChildErrors)

	parser.completeConflictReduceFrontier(nil, &stack, tok, seed, 1, 0, nil, &anyReduced, &nodeCount, arena, &entries, &gss, &tmp, false, &trackChildErrors)
	if !stack.dead {
		t.Fatal("frontier recover same-reduce cycle left original branch live")
	}
	if got, want := len(parser.pendingFrontierForkStacks), 1; got != want {
		t.Fatalf("pending frontier forks = %d, want %d", got, want)
	}
	fork := parser.pendingFrontierForkStacks[0]
	if !fork.shifted {
		t.Fatal("frontier recover fork left token reusable")
	}
	if got, want := fork.byteOffset, uint32(2); got != want {
		t.Fatalf("after frontier recover byte offset = %d, want %d", got, want)
	}
}

func TestConflictReduceFrontierShiftDoesNotAdvanceSiblingLookahead(t *testing.T) {
	lang := &Language{
		Name:              "conflict_reduce_frontier_sibling_lookahead",
		SymbolCount:       7,
		TokenCount:        3,
		StateCount:        7,
		InitialState:      0,
		ProductionIDCount: 4,
		SymbolNames:       []string{"end", "a", "b", "low_prefix", "high_prefix", "junk", "source"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false},
			{Name: "a", Visible: true},
			{Name: "b", Visible: true},
			{Name: "low_prefix", Visible: true, Named: true},
			{Name: "high_prefix", Visible: true, Named: true},
			{Name: "junk", Visible: true, Named: true},
			{Name: "source", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 6}}},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 0},
				{Type: ParseActionReduce, Symbol: 4, ChildCount: 1, ProductionID: 1, DynamicPrecedence: 5},
			}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			{Actions: []ParseAction{
				{Type: ParseActionShift, State: 5},
				{Type: ParseActionReduce, Symbol: 5, ChildCount: 1, ProductionID: 2},
			}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 6, ChildCount: 2, ProductionID: 3}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: [][]uint16{
			{0, 1, 0, 3, 4, 0, 5},
			{0, 0, 6, 0, 0, 0, 0},
			{0, 0, 7, 0, 0, 0, 0},
			{0, 0, 8, 0, 0, 0, 0},
			{9, 0, 0, 0, 0, 0, 0},
			{9, 0, 0, 0, 0, 0, 0},
			{10, 0, 0, 0, 0, 0, 0},
		},
	}
	parser := NewParser(lang)
	source := []byte("ab")
	tree, err := parser.ParseWithTokenSource(source, &slowArithmeticTokenSource{
		tokens: []Token{
			{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
			{Symbol: 2, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}},
			{Symbol: 0, StartByte: 2, EndByte: 2, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 2}},
		},
	})
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	defer tree.Release()
	rt := tree.ParseRuntime()
	if got, want := rt.StopReason, ParseStopAccepted; got != want {
		t.Fatalf("StopReason = %q, want %q; runtime=%s", got, want, rt.Summary())
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	sexpr := root.SExpr(lang)
	if !strings.Contains(sexpr, "high_prefix") {
		t.Fatalf("root = %s, want high-precedence sibling that reused the b lookahead; runtime=%s", sexpr, rt.Summary())
	}
	if strings.Contains(sexpr, "low_prefix") {
		t.Fatalf("root = %s, got low branch selected after frontier shift; runtime=%s", sexpr, rt.Summary())
	}
}

func TestConflictReduceFrontierAllShiftedAdvancesLookahead(t *testing.T) {
	lang := &Language{
		Name:              "conflict_reduce_frontier_all_shifted",
		SymbolCount:       6,
		TokenCount:        3,
		StateCount:        8,
		InitialState:      0,
		ProductionIDCount: 4,
		SymbolNames:       []string{"end", "a", "b", "low_prefix", "high_prefix", "source"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false},
			{Name: "a", Visible: true},
			{Name: "b", Visible: true},
			{Name: "low_prefix", Visible: true, Named: true},
			{Name: "high_prefix", Visible: true, Named: true},
			{Name: "source", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 5}}},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 0},
				{Type: ParseActionReduce, Symbol: 4, ChildCount: 1, ProductionID: 1, DynamicPrecedence: 5},
			}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 5, ChildCount: 2, ProductionID: 2}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 5, ChildCount: 2, ProductionID: 3, DynamicPrecedence: 5}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 7}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 4, ChildCount: 1, ProductionID: 1, DynamicPrecedence: 5},
				{Type: ParseActionShift, State: 5},
			}},
		},
		ParseTable: [][]uint16{
			{0, 1, 0, 2, 3, 9},
			{0, 0, 6, 0, 0, 0},
			{0, 0, 4, 0, 0, 0},
			{0, 0, 11, 0, 0, 0},
			{7, 0, 0, 0, 0, 0},
			{8, 0, 0, 0, 0, 0},
			{0, 0, 0, 0, 0, 0},
			{10, 0, 0, 0, 0, 0},
		},
	}
	parser := NewParser(lang)
	source := []byte("ab")
	tree, err := parser.ParseWithTokenSource(source, &slowArithmeticTokenSource{
		tokens: []Token{
			{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
			{Symbol: 2, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}},
			{Symbol: 0, StartByte: 2, EndByte: 2, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 2}},
		},
	})
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	defer tree.Release()
	rt := tree.ParseRuntime()
	if got, want := rt.StopReason, ParseStopAccepted; got != want {
		t.Fatalf("StopReason = %q, want %q; runtime=%s", got, want, rt.Summary())
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d; runtime=%s tree=%s", got, want, rt.Summary(), root.SExpr(lang))
	}
}

func TestParseRuntimeStopDiagnosticSummary(t *testing.T) {
	rt := ParseRuntime{
		StopReason:                           ParseStopNoStacksAlive,
		StopDiagnosticCaptured:               true,
		StopDiagnosticCRecoveryEnabled:       true,
		StopDiagnosticCRecoveryGateReason:    "runtime_supported",
		StopDiagnosticRecoverActionAvailable: true,
		StopDiagnosticLastStackState:         7,
		StopDiagnosticLastStackByte:          11,
		StopDiagnosticLastStackDepth:         3,
		StopDiagnosticTokenSymbol:            5,
		StopDiagnosticTokenStartByte:         11,
		StopDiagnosticTokenEndByte:           12,
		StopDiagnosticRootType:               "source",
		StopDiagnosticRootStartByte:          0,
		StopDiagnosticRootEndByte:            11,
		StopDiagnosticRootHasError:           true,
		StopDiagnosticFirstErrorFound:        true,
		StopDiagnosticFirstErrorStartByte:    4,
		StopDiagnosticFirstErrorEndByte:      8,
		StopDiagnosticFrontierStacks:         "stacks={total=2 live=2 limit=16 rows=[0:7@11/d3 sh=false dead=false acc=false pause=false rec=false score=0 br=0 snap=true]}",
		StopDiagnosticFrontierActions:        "post_dispatch_unshifted_actionable=1 post_dispatch_snapshot_actionable=1 post_dispatch_appended_actionable=0",
		StopDiagnosticSameHeaderGroups:       "sameHeader={groups=1 max=2 pairs=1 deepReject=1 costDiff=0 externalScannerDiffKnown=false gssEligible=0 gssAttemptable=0 gssWouldMerge=0 pairBudgetHit=false}",
		StopDiagnosticCondenseGating:         "condense={errorCostCompetitionEnabled=true anyReduced=true hasPaused=true hasCRec=false condenseSkippedDueToAnyReduced=true condenseRan=false condenseResumed=false}",
	}
	summary := rt.Summary()
	for _, want := range []string{
		"stopDiag={",
		"cRecovery=true",
		"cRecoveryGateReason=runtime_supported",
		"recoverAction=true",
		"stackState=7",
		"token=5[11:12]",
		"root=\"source\"[0:11]",
		"firstError=true[4:8]",
		"stacks={total=2 live=2",
		"post_dispatch_snapshot_actionable=1",
		"sameHeader={groups=1 max=2 pairs=1 deepReject=1",
		"condenseSkippedDueToAnyReduced=true",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("Summary() missing %q: %s", want, summary)
		}
	}
}

func TestParseRuntimeStopDiagnosticCRecoveryGateReasonSummary(t *testing.T) {
	rt := ParseRuntime{
		StopReason:                        ParseStopNoStacksAlive,
		StopDiagnosticCaptured:            true,
		StopDiagnosticCRecoveryEnabled:    false,
		StopDiagnosticCRecoveryGateReason: "external_scanner_requires_precise_externallexstates",
	}
	summary := rt.Summary()
	for _, want := range []string{
		"cRecovery=false",
		"cRecoveryGateReason=external_scanner_requires_precise_externallexstates",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("Summary() missing %q: %s", want, summary)
		}
	}
	if strings.Contains(summary, "precise ExternalLexStates") {
		t.Fatalf("Summary() included spaced human reason in machine field: %s", summary)
	}
}

func TestParseRuntimeStopDiagnosticRecordsCRecoveryGateReason(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")
	lang := buildPrefixAcceptLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("abb"))
	defer tree.Release()
	rt := tree.ParseRuntime()

	if got, want := rt.StopReason, ParseStopNoStacksAlive; got != want {
		t.Fatalf("StopReason = %q, want %q; runtime=%s", got, want, rt.Summary())
	}
	if !rt.StopDiagnosticCaptured {
		t.Fatalf("StopDiagnosticCaptured = false; runtime=%s", rt.Summary())
	}
	if rt.StopDiagnosticCRecoveryEnabled {
		t.Fatalf("StopDiagnosticCRecoveryEnabled = true, want false; runtime=%s", rt.Summary())
	}
	if got, want := rt.StopDiagnosticCRecoveryGateReason, "initial_state_is_not_1"; got != want {
		t.Fatalf("StopDiagnosticCRecoveryGateReason = %q, want %q; runtime=%s", got, want, rt.Summary())
	}
	if !strings.Contains(rt.Summary(), "cRecoveryGateReason=initial_state_is_not_1") {
		t.Fatalf("Summary() missing cRecoveryGateReason: %s", rt.Summary())
	}
}

func TestCRecoveryGateReasonSlugs(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "all")
	lang := cRecoveryGateLanguage()
	lang.ExternalScanner = cRecoveryGateScanner{}
	lang.ExternalSymbols = []Symbol{1}
	lang.ExternalTokenCount = 1
	if got, want := cRecoveryGateReason(lang), "external_scanner_requires_precise_externallexstates"; got != want {
		t.Fatalf("cRecoveryGateReason(runtime unsupported) = %q, want %q", got, want)
	}

	t.Setenv("GOT_C_RECOVERY", "0")
	if got, want := cRecoveryGateReason(lang), "disabled_by_got_c_recovery_0"; got != want {
		t.Fatalf("cRecoveryGateReason(env disabled unsupported) = %q, want %q", got, want)
	}

	lang = cRecoveryGateLanguage()
	if got, want := cRecoveryGateReason(lang), "disabled_by_got_c_recovery_0"; got != want {
		t.Fatalf("cRecoveryGateReason(env disabled) = %q, want %q", got, want)
	}

	t.Setenv("GOT_C_RECOVERY", "")
	lang = cRecoveryGateLanguage()
	lang.CRecoveryCostCompetitionEnabledByDefault = false
	if got, want := cRecoveryGateReason(lang), "not_enabled_by_default"; got != want {
		t.Fatalf("cRecoveryGateReason(default disabled) = %q, want %q", got, want)
	}

	lang.CRecoveryCostCompetitionCapable = false
	if got, want := cRecoveryGateReason(lang), "not_c_recovery_capable"; got != want {
		t.Fatalf("cRecoveryGateReason(not capable) = %q, want %q", got, want)
	}
}

func TestParseRuntimeReportsNoTreeNodeVolume(t *testing.T) {
	EnableArenaBreakdown(true)
	defer EnableArenaBreakdown(false)

	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree, err := parser.ParseNoTreeBenchmarkOnly([]byte("1+2"))
	if err != nil {
		t.Fatalf("ParseNoTreeBenchmarkOnly() error = %v", err)
	}
	defer tree.Release()
	rt := tree.ParseRuntime()

	if rt.LeafNodesConstructed == 0 {
		t.Fatal("LeafNodesConstructed = 0, want > 0")
	}
	if rt.ParentNodesConstructed != 0 {
		t.Fatalf("ParentNodesConstructed = %d, want 0", rt.ParentNodesConstructed)
	}
	if rt.NoTreeReduceNodesConstructed == 0 {
		t.Fatal("NoTreeReduceNodesConstructed = 0, want > 0")
	}
	if rt.NoTreeLeafNodesConstructed != 0 {
		t.Fatalf("NoTreeLeafNodesConstructed = %d, want 0", rt.NoTreeLeafNodesConstructed)
	}
	if got, want := rt.FinalNodes, uint64(1); got != want {
		t.Fatalf("FinalNodes = %d, want %d", got, want)
	}
	if got := rt.FinalParentNodes; got != 0 {
		t.Fatalf("FinalParentNodes = %d, want 0", got)
	}
	if got, want := rt.FinalLeafNodes, uint64(1); got != want {
		t.Fatalf("FinalLeafNodes = %d, want %d", got, want)
	}
	if got := rt.FinalChildPointers; got != 0 {
		t.Fatalf("FinalChildPointers = %d, want 0", got)
	}
	breakdown := assertParseRuntimeArenaBreakdown(t, tree, rt)
	if got := breakdown.NoTreePlaceholderNodesConstructed; got != 1 {
		t.Fatalf("NoTreePlaceholderNodesConstructed = %d, want 1", got)
	}
	if got := breakdown.NoTreeLeafNodesConstructed; got != 0 {
		t.Fatalf("NoTreeLeafNodesConstructed breakdown = %d, want 0", got)
	}
	if got := breakdown.FieldedParentNodesConstructed + breakdown.UnfieldedParentNodesConstructed; got != 0 {
		t.Fatalf("parent field attribution = %d, want 0", got)
	}
	if got := breakdown.ParentConstructedChildLen0 + breakdown.ParentConstructedChildLen1 + breakdown.ParentConstructedChildLen2 + breakdown.ParentConstructedChildLen3 + breakdown.ParentConstructedChildLen4Plus; got != 0 {
		t.Fatalf("parent child-count attribution = %d, want 0", got)
	}
	if got := breakdown.ParentConstructedNoLinks + breakdown.ParentConstructedWithLinks; got != 0 {
		t.Fatalf("parent link attribution = %d, want 0", got)
	}
	if breakdown.NoTreeNodeBytesAllocated == 0 {
		t.Fatal("NoTreeNodeBytesAllocated = 0, want > 0")
	}
}

func TestParseRuntimeReportsNoTreeCheckpointLeavesRemainNodes(t *testing.T) {
	EnableArenaBreakdown(true)
	defer EnableArenaBreakdown(false)

	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree, err := parser.ParseNoTreeWithExternalCheckpointsBenchmarkOnly([]byte("1+2"))
	if err != nil {
		t.Fatalf("ParseNoTreeWithExternalCheckpointsBenchmarkOnly() error = %v", err)
	}
	defer tree.Release()
	rt := tree.ParseRuntime()

	if rt.LeafNodesConstructed == 0 {
		t.Fatal("LeafNodesConstructed = 0, want > 0")
	}
	if rt.NoTreeLeafNodesConstructed != 0 {
		t.Fatalf("NoTreeLeafNodesConstructed = %d, want 0", rt.NoTreeLeafNodesConstructed)
	}
	if rt.NoTreeReduceNodesConstructed == 0 {
		t.Fatal("NoTreeReduceNodesConstructed = 0, want > 0")
	}
}

type checkpointedByteStateExternalScanner struct{ byteStateExternalScanner }

func (checkpointedByteStateExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

func TestParseNoTreeBenchmarkDropsRetainedExternalCheckpointCapacity(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.ExternalScanner = checkpointedByteStateExternalScanner{}

	fullParser := NewParser(lang)
	// Pin to production: this test asserts a production-engine runtime internal
	// (external scanner checkpoint allocation) the compact route does not report.
	fullParser.SetAdmissionCandidateRoute(false)
	fullTree, err := fullParser.Parse([]byte("1+2"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	fullRuntime := fullTree.ParseRuntime()
	fullTree.Release()
	if fullRuntime.ExternalScannerCheckpointBytesAllocated == 0 {
		t.Fatalf("full checkpoint bytes = 0, want > 0; runtime=%s", fullRuntime.Summary())
	}

	noTreeParser := NewParser(lang)
	noTree, err := noTreeParser.ParseNoTreeBenchmarkOnly([]byte("1+2"))
	if err != nil {
		t.Fatalf("ParseNoTreeBenchmarkOnly() error = %v", err)
	}
	defer noTree.Release()
	noTreeRuntime := noTree.ParseRuntime()
	if noTreeRuntime.ExternalScannerCheckpointRecords != 0 {
		t.Fatalf("no-tree checkpoint records = %d, want 0; runtime=%s", noTreeRuntime.ExternalScannerCheckpointRecords, noTreeRuntime.Summary())
	}
	if noTreeRuntime.ExternalScannerCheckpointBytesAllocated != 0 {
		t.Fatalf("no-tree checkpoint bytes = %d, want 0; runtime=%s", noTreeRuntime.ExternalScannerCheckpointBytesAllocated, noTreeRuntime.Summary())
	}
}

func TestFinalTreeMaterializationStatsClassifiesHiddenParentsAndCheckpointLeaves(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "leaf", Visible: true, Named: true},
			{Name: "_hidden", Visible: false, Named: false},
		},
	}
	leaf := newLeafNodeInArena(arena, 0, true, 0, 1, Point{}, Point{Column: 1})
	arena.recordExternalScannerLeafCheckpoint(leaf, []byte{1}, []byte{2})
	root := newParentNodeInArena(arena, 1, false, []*Node{leaf}, nil, 0)

	stats := collectFinalTreeMaterializationStats(root, lang)
	if got, want := stats.nodes, uint64(2); got != want {
		t.Fatalf("nodes = %d, want %d", got, want)
	}
	if got, want := stats.hiddenParentNodes, uint64(1); got != want {
		t.Fatalf("hiddenParentNodes = %d, want %d", got, want)
	}
	if got := stats.visibleParentNodes; got != 0 {
		t.Fatalf("visibleParentNodes = %d, want 0", got)
	}
	if got, want := stats.checkpointLeafNodes, uint64(1); got != want {
		t.Fatalf("checkpointLeafNodes = %d, want %d", got, want)
	}
}

func assertParseRuntimeArenaBreakdown(t *testing.T, tree *Tree, rt ParseRuntime) ArenaBreakdown {
	t.Helper()
	arenaBreakdown, ok := tree.ArenaBreakdown()
	if !ok {
		t.Fatal("ArenaBreakdown = nil, want populated")
	}
	breakdown := arenaBreakdown.NodeStructBytesAllocated +
		arenaBreakdown.NodeFieldMetadataBytesAllocated +
		arenaBreakdown.NoTreeNodeBytesAllocated +
		arenaBreakdown.CompactFullLeafBytesAllocated +
		arenaBreakdown.PendingParentBytesAllocated +
		arenaBreakdown.PendingChildEntryBytesAllocated +
		arenaBreakdown.RawShapeBytesAllocated +
		arenaBreakdown.RawShapeChildBytesAllocated +
		arenaBreakdown.RawShapeHashCacheBytesAllocated +
		arenaBreakdown.FinalChildSidecarBytesAllocated +
		arenaBreakdown.MissingNodeDependencyBytesAllocated +
		arenaBreakdown.CompactReuseDependencyBytesAllocated +
		arenaBreakdown.CompactCheckpointLeafBytesAllocated +
		arenaBreakdown.ChildSliceBytesAllocated +
		arenaBreakdown.FieldIDBytesAllocated +
		arenaBreakdown.FieldSourceBytesAllocated +
		rt.ExternalScannerCheckpointBytesAllocated
	if rt.ArenaBytesAllocated != breakdown {
		t.Fatalf("arena bytes = %d, breakdown sum = %d", rt.ArenaBytesAllocated, breakdown)
	}
	if arenaBreakdown.NodeStructBytesAllocated == 0 {
		t.Fatal("ArenaBreakdown.NodeStructBytesAllocated = 0, want > 0")
	}
	if arenaBreakdown.NodeLiveCount == 0 {
		t.Fatal("ArenaBreakdown.NodeLiveCount = 0, want > 0")
	}
	if arenaBreakdown.NodeCapacityCount < arenaBreakdown.NodeLiveCount {
		t.Fatalf("NodeCapacityCount = %d, NodeLiveCount = %d", arenaBreakdown.NodeCapacityCount, arenaBreakdown.NodeLiveCount)
	}
	if got, want := arenaBreakdown.NodeCapacityWaste, arenaBreakdown.NodeCapacityCount-arenaBreakdown.NodeLiveCount; got != want {
		t.Fatalf("NodeCapacityWaste = %d, want %d", got, want)
	}
	knownNodes := rt.LeafNodesConstructed +
		rt.ParentNodesConstructed +
		arenaBreakdown.NoTreePlaceholderNodesConstructed +
		arenaBreakdown.OtherNodesConstructed
	if arenaBreakdown.ArenaNodesConstructed != knownNodes {
		t.Fatalf("ArenaNodesConstructed = %d, known node sum = %d", arenaBreakdown.ArenaNodesConstructed, knownNodes)
	}
	return arenaBreakdown
}

type eofAtZeroTokenSource struct{}

func (eofAtZeroTokenSource) Next() Token {
	return Token{
		Symbol:    0,
		StartByte: 0,
		EndByte:   0,
	}
}

type slowArithmeticTokenSource struct {
	delay  time.Duration
	tokens []Token
	idx    int
}

func (s *slowArithmeticTokenSource) Next() Token {
	time.Sleep(s.delay)
	if s.idx >= len(s.tokens) {
		return Token{Symbol: 0}
	}
	tok := s.tokens[s.idx]
	s.idx++
	return tok
}

func TestEOFDrainKeepsLiveReducedBranchAfterEarlyAccept(t *testing.T) {
	lang := &Language{
		Name:         "eof_drain_regression",
		SymbolCount:  4,
		TokenCount:   2,
		StateCount:   5,
		InitialState: 1,
		SymbolNames:  []string{"end", "x", "low_root", "high_root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false},
			{Name: "x", Visible: true},
			{Name: "low_root", Visible: true, Named: true},
			{Name: "high_root", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 2, ChildCount: 1}}},
			{Actions: []ParseAction{
				{Type: ParseActionAccept},
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, DynamicPrecedence: 1},
			}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: [][]uint16{
			make([]uint16, 4),
			{0, 1, 2, 4},
			{3, 0, 0, 0},
			{2, 0, 0, 0},
			{4, 0, 0, 0},
		},
	}
	parser := NewParser(lang)
	parser.hasRootSymbol = false
	source := []byte("x")
	tree, err := parser.ParseWithTokenSource(source, &slowArithmeticTokenSource{
		tokens: []Token{
			{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
			{Symbol: 0, StartByte: 1, EndByte: 1, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 1}},
		},
	})
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	if got, want := tree.RootNode().Type(lang), "high_root"; got != want {
		t.Fatalf("root type = %q, want %q; tree=%s", got, want, tree.RootNode().SExpr(lang))
	}
}

func TestParseRuntimeReportsTokenSourceEOFEarly(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	src := []byte("1+2")

	tree, err := parser.ParseWithTokenSource(src, eofAtZeroTokenSource{})
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	rt := tree.ParseRuntime()

	if rt.StopReason != ParseStopTokenSourceEOF {
		t.Fatalf("StopReason = %q, want %q", rt.StopReason, ParseStopTokenSourceEOF)
	}
	if !rt.TokenSourceEOFEarly {
		t.Fatal("TokenSourceEOFEarly = false, want true")
	}
	if rt.LastTokenEndByte != 0 {
		t.Fatalf("LastTokenEndByte = %d, want 0", rt.LastTokenEndByte)
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
}

func TestParserCancellationFlagStopsParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	var cancelled uint32 = 1
	parser.SetCancellationFlag(&cancelled)
	if got := parser.CancellationFlag(); got != &cancelled {
		t.Fatalf("CancellationFlag() = %p, want %p", got, &cancelled)
	}

	tree := mustParse(t, parser, []byte("1+2"))
	if got, want := tree.ParseStopReason(), ParseStopCancelled; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
}

func TestParseStrictReturnsStoppedEarlyError(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	var cancelled uint32 = 1
	parser.SetCancellationFlag(&cancelled)

	tree, err := parser.ParseStrict([]byte("1+2"))
	if tree == nil {
		t.Fatal("ParseStrict returned nil tree, want partial tree")
	}
	if !errors.Is(err, ErrParseStoppedEarly) {
		t.Fatalf("ParseStrict error = %v, want ErrParseStoppedEarly", err)
	}
	var stopped *ParseStoppedEarlyError
	if !errors.As(err, &stopped) {
		t.Fatalf("ParseStrict error type = %T, want *ParseStoppedEarlyError", err)
	}
	if stopped.Reason != ParseStopCancelled {
		t.Fatalf("stopped reason = %q, want %q", stopped.Reason, ParseStopCancelled)
	}
	if stopped.Runtime.StopReason != ParseStopCancelled {
		t.Fatalf("runtime stop reason = %q, want %q", stopped.Runtime.StopReason, ParseStopCancelled)
	}
}

func TestParserTimeoutMicrosStopsParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	parser.SetTimeoutMicros(200)
	if got := parser.TimeoutMicros(); got != 200 {
		t.Fatalf("TimeoutMicros() = %d, want 200", got)
	}

	ts := &slowArithmeticTokenSource{
		delay: 2 * time.Millisecond,
		tokens: []Token{
			{Symbol: 1, StartByte: 0, EndByte: 1},
			{Symbol: 0, StartByte: 1, EndByte: 1},
		},
	}
	tree, err := parser.ParseWithTokenSource([]byte("1"), ts)
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	if got, want := tree.ParseStopReason(), ParseStopTimeout; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
}

func TestParseWithTokenSourceStrictReturnsTimeoutError(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	parser.SetTimeoutMicros(200)

	ts := &slowArithmeticTokenSource{
		delay: 2 * time.Millisecond,
		tokens: []Token{
			{Symbol: 1, StartByte: 0, EndByte: 1},
			{Symbol: 0, StartByte: 1, EndByte: 1},
		},
	}
	tree, err := parser.ParseWithTokenSourceStrict([]byte("1"), ts)
	if tree == nil {
		t.Fatal("ParseWithTokenSourceStrict returned nil tree, want partial tree")
	}
	if !errors.Is(err, ErrParseStoppedEarly) {
		t.Fatalf("ParseWithTokenSourceStrict error = %v, want ErrParseStoppedEarly", err)
	}
	var stopped *ParseStoppedEarlyError
	if !errors.As(err, &stopped) {
		t.Fatalf("ParseWithTokenSourceStrict error type = %T, want *ParseStoppedEarlyError", err)
	}
	if stopped.Reason != ParseStopTimeout {
		t.Fatalf("stopped reason = %q, want %q", stopped.Reason, ParseStopTimeout)
	}
}

func TestParserLoggerReceivesEvents(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	var parseEvents int
	var lexEvents int
	parser.SetLogger(func(kind ParserLogType, msg string) {
		if msg == "" {
			t.Fatal("logger message should not be empty")
		}
		switch kind {
		case ParserLogParse:
			parseEvents++
		case ParserLogLex:
			lexEvents++
		}
	})

	if _, err := parser.Parse([]byte("1+2")); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parseEvents == 0 {
		t.Fatal("expected at least one parse log event")
	}
	if lexEvents == 0 {
		t.Fatal("expected at least one lex log event")
	}

	// Nil logger disables logging.
	parser.SetLogger(nil)
	parseEvents = 0
	lexEvents = 0
	if _, err := parser.Parse([]byte("1+2")); err != nil {
		t.Fatalf("Parse() with nil logger error = %v", err)
	}
	if parseEvents != 0 || lexEvents != 0 {
		t.Fatalf("expected no events with nil logger, got parse=%d lex=%d", parseEvents, lexEvents)
	}
}
