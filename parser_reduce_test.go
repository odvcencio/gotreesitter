package gotreesitter

import "testing"

func buildTwoWindowFullGSSReduceCase(t *testing.T, scratch *gssScratch, arena *nodeArena) (*Parser, glrStack, ParseAction) {
	t.Helper()

	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNode(3, right), leftNode, 3)
	altRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightNode.extraLinks = append(rightNode.extraLinks, gssMainLink{
		prev:  leftNode,
		entry: newStackEntryNode(3, altRight),
	})

	parser := &Parser{language: &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}}
	stack := glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2, DynamicPrecedence: 4}
	return parser, stack, act
}

func setSyntheticEOFAction(t *testing.T, parser *Parser, state StateID, actions []ParseAction) {
	t.Helper()
	if parser == nil || parser.language == nil {
		t.Fatal("parser/language must be initialized")
	}
	if parser.language.SymbolCount == 0 {
		parser.language.SymbolCount = 4
	}
	if parser.language.TokenCount == 0 {
		parser.language.TokenCount = 3
	}
	if parser.language.StateCount <= uint32(state) {
		parser.language.StateCount = uint32(state) + 1
	}
	for len(parser.language.ParseTable) <= int(state) {
		parser.language.ParseTable = append(parser.language.ParseTable, make([]uint16, parser.language.SymbolCount))
	}
	for i := range parser.language.ParseTable {
		if len(parser.language.ParseTable[i]) < int(parser.language.SymbolCount) {
			row := make([]uint16, parser.language.SymbolCount)
			copy(row, parser.language.ParseTable[i])
			parser.language.ParseTable[i] = row
		}
	}
	if len(parser.language.ParseActions) == 0 {
		parser.language.ParseActions = append(parser.language.ParseActions, ParseActionEntry{})
	}
	parser.language.ParseActions = append(parser.language.ParseActions, ParseActionEntry{Actions: actions})
	parser.language.ParseTable[state][0] = uint16(len(parser.language.ParseActions) - 1)
	parser.denseLimit = len(parser.language.ParseTable)
}

func setSyntheticPostReducePackingSafeEOF(t *testing.T, parser *Parser, states ...StateID) {
	t.Helper()
	for _, state := range states {
		setSyntheticEOFAction(t, parser, state, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1}})
	}
}

func TestFastVisibleReduceFromGSSDeclinesMultiLinkSpan(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})
	right := NewLeafNode(2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNode(3, right), leftNode, 3)
	altRight := NewLeafNode(2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightNode.extraLinks = append(rightNode.extraLinks, gssMainLink{
		prev:  leftNode,
		entry: newStackEntryNode(3, altRight),
	})

	parser := &Parser{language: &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}}
	stack := &glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}
	var anyReduced bool
	nodeCount := 0
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	if parser.tryFastVisibleReduceActionFromGSS(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, false, false) {
		t.Fatal("tryFastVisibleReduceActionFromGSS = true, want false for multi-link span")
	}
	if anyReduced {
		t.Fatal("anyReduced = true, want false")
	}
	if nodeCount != 0 {
		t.Fatalf("nodeCount = %d, want 0", nodeCount)
	}
	if stack.gss.head != rightNode {
		t.Fatal("stack mutated by declined fast reduce")
	}
}

func TestFaithfulForkReduceFromGSSLinkedWindowsCoalescesPostReduceHead(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })
	if perfCountersEnabled {
		ResetPerfCounters()
	}

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	parser, stack, act := buildTwoWindowFullGSSReduceCase(t, &scratch, arena)
	setSyntheticPostReducePackingSafeEOF(t, parser, 1)
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if !anyReduced {
		t.Fatal("anyReduced = false, want true")
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want 1", nodeCount)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0", len(parser.pendingForkStacks))
	}
	if stack.gss.head == nil {
		t.Fatal("post-reduce GSS head is nil")
	}
	if got := stack.gss.head.linkCount(); got != 1 {
		t.Fatalf("post-reduce head linkCount = %d, want 1", got)
	}
	if perfCountersEnabled {
		perf := PerfCountersSnapshot()
		if perf.ReduceForkCalls != 1 || perf.ReduceForkWindows != 2 || perf.ReduceForkMaxWindows != 2 {
			t.Fatalf("reduce fork counters = calls:%d windows:%d max:%d, want 1/2/2", perf.ReduceForkCalls, perf.ReduceForkWindows, perf.ReduceForkMaxWindows)
		}
		if perf.PostReduceMergeAttempts != 0 || perf.PostReduceMergePrimarySuccesses != 0 || perf.PendingForkStackAppends != 0 {
			t.Fatalf("post-reduce merge counters = attempts:%d primary:%d appends:%d, want 0/0/0", perf.PostReduceMergeAttempts, perf.PostReduceMergePrimarySuccesses, perf.PendingForkStackAppends)
		}
	}
}

func TestFaithfulForkReduceImmediateAcceptPacksFinalizerVisibleFork(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })
	if perfCountersEnabled {
		ResetPerfCounters()
	}

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	parser, stack, act := buildTwoWindowFullGSSReduceCase(t, &scratch, arena)
	setSyntheticEOFAction(t, parser, 1, []ParseAction{{Type: ParseActionAccept}})
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if !anyReduced {
		t.Fatal("anyReduced = false, want true")
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want 1", nodeCount)
	}
	if got := stack.gss.head.linkCount(); got != 1 {
		t.Fatalf("primary post-reduce head linkCount = %d, want 1", got)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0", len(parser.pendingForkStacks))
	}
	if perfCountersEnabled {
		perf := PerfCountersSnapshot()
		if perf.ReduceForkCalls != 1 || perf.ReduceForkWindows != 2 || perf.ReduceForkMaxWindows != 2 {
			t.Fatalf("reduce fork counters = calls:%d windows:%d max:%d, want 1/2/2", perf.ReduceForkCalls, perf.ReduceForkWindows, perf.ReduceForkMaxWindows)
		}
		if perf.PostReduceMergeFinalizationRiskSkips != 0 || perf.PostReduceMergeAttempts != 0 || perf.PostReduceMergePrimarySuccesses != 0 || perf.PendingForkStackAppends != 0 {
			t.Fatalf("accept merge counters = skips:%d attempts:%d primary:%d appends:%d, want 0/0/0/0", perf.PostReduceMergeFinalizationRiskSkips, perf.PostReduceMergeAttempts, perf.PostReduceMergePrimarySuccesses, perf.PendingForkStackAppends)
		}
	}
}

func TestFaithfulForkReduceFromPackedGSSHeadEnumeratesReducedParents(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	parser, stack, act := buildTwoWindowFullGSSReduceCase(t, &scratch, arena)
	setSyntheticPostReducePackingSafeEOF(t, parser, 1)
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	forks := reduceWindowsFromGSS(&stack, 1, maxStacksPerMergeKey)
	if len(forks) != 1 {
		t.Fatalf("reduced-parent windows = %d, want 1", len(forks))
	}
	for i, fork := range forks {
		if len(fork.window) != 1 {
			t.Fatalf("fork %d window length = %d, want 1", i, len(fork.window))
		}
		parent := stackEntryNode(fork.window[0])
		if parent == nil || parent.symbol != act.Symbol {
			t.Fatalf("fork %d parent symbol = %v, want %d", i, parent, act.Symbol)
		}
	}
}

func TestFaithfulGSSMergeRecursesPredecessorLinksAndReduceSelectsConstructedParent(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	leftLow := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftHigh := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
	leftHigh.dynamicPrecedence = 9
	leftLowNode := scratch.allocNode(newStackEntryNode(2, leftLow), base, 2)
	leftHighNode := scratch.allocNode(newStackEntryNode(2, leftHigh), base, 2)
	rightLow := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightHigh := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	lowHead := scratch.allocNode(newStackEntryNode(3, rightLow), leftLowNode, 3)
	highHead := scratch.allocNode(newStackEntryNode(3, rightHigh), leftHighNode, 3)

	low := glrStack{gss: gssStack{head: lowHead}, byteOffset: 2}
	high := glrStack{gss: gssStack{head: highHead}, byteOffset: 2}
	if !gssMainCanMerge(&low, &high) {
		t.Fatal("clean same-state synthetic stacks should be mergeable")
	}
	if !gssMainMerge(&low, &high) {
		t.Fatal("gssMainMerge returned false")
	}
	if got := low.gss.head.linkCount(); got != 1 {
		t.Fatalf("top link count = %d, want 1 after equivalent-top recursive merge", got)
	}
	if got := low.gss.head.prev.linkCount(); got != 2 {
		t.Fatalf("predecessor link count = %d, want 2 linked child alternatives", got)
	}
	forks := reduceWindowsFromGSS(&low, 2, maxMainLinkCount)
	if len(forks) != 2 {
		t.Fatalf("pre-reduce linked windows = %d, want 2", len(forks))
	}

	parser := &Parser{language: &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left_low", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
			{Name: "left_high", Visible: true, Named: true},
		},
	}}
	setSyntheticPostReducePackingSafeEOF(t, parser, 1)
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &low, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if low.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if !anyReduced {
		t.Fatal("anyReduced = false, want true")
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want exactly one selected parent", nodeCount)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0; selection should not need final expansion", len(parser.pendingForkStacks))
	}
	parent := stackEntryNode(low.top())
	if parent == nil || parent.symbol != act.Symbol {
		t.Fatalf("top parent = %v, want reduced parent symbol %d", parent, act.Symbol)
	}
	if len(parent.children) != 2 || parent.children[0] != leftHigh {
		t.Fatal("constructed-parent selection did not keep the higher dynamic-precedence child path")
	}
	if got := parent.dynamicPrecedence; got != leftHigh.dynamicPrecedence {
		t.Fatalf("parent dynamic precedence = %d, want %d", got, leftHigh.dynamicPrecedence)
	}
}

func TestSelectReduceForkChildrenCoalescesRawOnlySameGroupForksByRecursiveOrder(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	flatRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	nestedRightChild := newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	nestedRight := newParentNodeInArena(arena, 2, true, []*Node{nestedRightChild}, nil, 0)
	nestedRight.startByte = 1
	nestedRight.endByte = 2
	nestedRight.startPoint = Point{Column: 1}
	nestedRight.endPoint = Point{Column: 2}

	flat := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, flatRight),
		},
	}
	nested := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, nestedRight),
		},
	}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
		{Name: "inner", Visible: true, Named: true},
	}}}
	selected := parser.selectReduceForkChildren(arena, ParseAction{Symbol: 3}, []reduceFork{nested, flat})
	if len(selected) != 1 {
		t.Fatalf("selected windows = %d, want one C-selected raw-only alternative", len(selected))
	}
	if stackEntryNode(selected[0].window[1]) != flatRight {
		t.Fatal("raw-only same-group selection did not keep the recursive-order winner")
	}
}

func TestSelectReduceForkChildrenCoalescesHigherDynamicPrecedence(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	leftLow := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftHigh := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftHigh.dynamicPrecedence = 9
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	low := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, leftLow),
			newStackEntryNode(9, right),
		},
	}
	high := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, leftHigh),
			newStackEntryNode(9, right),
		},
	}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}}
	selected := parser.selectReduceForkChildren(arena, ParseAction{Symbol: 3, ChildCount: 2}, []reduceFork{low, high})
	if len(selected) != 1 {
		t.Fatalf("selected windows = %d, want 1 dynamic-precedence winner", len(selected))
	}
	if stackEntryNode(selected[0].window[0]) != leftHigh {
		t.Fatal("selected low dynamic-precedence fork, want high dynamic-precedence fork")
	}
}

func TestSelectReduceForkChildrenCoalescesLowerErrorCost(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	cleanRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	missingRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	missingRight.setMissing(true)
	missingRight.setHasError(true)
	clean := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, cleanRight),
		},
	}
	missing := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, missingRight),
		},
	}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}}
	selected := parser.selectReduceForkChildren(arena, ParseAction{Symbol: 3, ChildCount: 2}, []reduceFork{missing, clean})
	if len(selected) != 1 {
		t.Fatalf("selected windows = %d, want 1 lower-error-cost winner", len(selected))
	}
	if stackEntryNode(selected[0].window[1]) != cleanRight {
		t.Fatal("selected missing/error fork, want clean lower-error-cost fork")
	}
}

func TestSelectReduceForkChildrenCoalescesNonAdjacentRawOnlySameGroupForks(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	otherPopTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	flatRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	otherRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	nestedRightChild := newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	nestedRight := newParentNodeInArena(arena, 2, true, []*Node{nestedRightChild}, nil, 0)
	nestedRight.startByte = 1
	nestedRight.endByte = 2
	nestedRight.startPoint = Point{Column: 1}
	nestedRight.endPoint = Point{Column: 2}

	nested := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, nestedRight),
		},
	}
	distinct := reduceFork{
		popTo:    otherPopTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, otherRight),
		},
	}
	flat := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, flatRight),
		},
	}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
		{Name: "inner", Visible: true, Named: true},
	}}}
	selected := parser.selectReduceForkChildren(arena, ParseAction{Symbol: 3}, []reduceFork{nested, distinct, flat})
	if len(selected) != 2 {
		t.Fatalf("selected windows = %d, want distinct group plus one C-selected same-pop fork", len(selected))
	}
	if stackEntryNode(selected[0].window[1]) != flatRight {
		t.Fatal("selected first group did not keep the recursive-order winner")
	}
	if selected[1].popTo != otherPopTo {
		t.Fatal("selected second group did not preserve distinct group order")
	}
}

func TestReduceForkSelectionScansPastCrossProductRawCapForSameGroupWinner(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	var bestLeft, bestRight *Node
	var firstLeftHub *gssNode
	var head *gssNode
	for rightIdx := 0; rightIdx < maxMainLinkCount; rightIdx++ {
		left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
		leftHub := scratch.allocNode(newStackEntryNode(8, left), popTo, 2)
		for leftIdx := 1; leftIdx < maxMainLinkCount; leftIdx++ {
			altLeft := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
			if rightIdx == 1 && leftIdx == maxMainLinkCount-1 {
				altLeft.dynamicPrecedence = 99
				bestLeft = altLeft
			}
			leftHub.extraLinks = append(leftHub.extraLinks, gssMainLink{
				prev:  popTo,
				entry: newStackEntryNode(8, altLeft),
			})
		}
		right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
		if rightIdx == 1 {
			bestRight = right
		}
		if rightIdx == 0 {
			firstLeftHub = leftHub
			head = scratch.allocNode(newStackEntryNode(9, right), leftHub, 3)
			continue
		}
		head.extraLinks = append(head.extraLinks, gssMainLink{
			prev:  leftHub,
			entry: newStackEntryNode(9, right),
		})
	}
	if bestLeft == nil || bestRight == nil {
		t.Fatal("setup did not create late best child path")
	}
	if got := head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("head link count = %d, want production cap %d", got, maxMainLinkCount)
	}
	for i, count := 0, head.linkCount(); i < count; i++ {
		prev, _ := head.link(i)
		if got := prev.linkCount(); got != maxMainLinkCount {
			t.Fatalf("left predecessor %d link count = %d, want production cap %d", i, got, maxMainLinkCount)
		}
	}
	stack := glrStack{gss: gssStack{head: head}, byteOffset: 1}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}

	cappedRaw := reduceWindowsFromGSS(&stack, int(act.ChildCount), maxMainLinkCount)
	if len(cappedRaw) != maxMainLinkCount {
		t.Fatalf("capped raw windows = %d, want %d", len(cappedRaw), maxMainLinkCount)
	}
	for i, fork := range cappedRaw {
		if fork.popTo != popTo {
			t.Fatalf("capped raw fork %d popTo = %p, want shared popTo %p", i, fork.popTo, popTo)
		}
		if stackEntryNode(fork.window[1]) == bestRight || stackEntryNode(fork.window[0]) == bestLeft {
			t.Fatalf("proof setup invalid: capped raw fork %d unexpectedly included late best path", i)
		}
	}
	if firstLeftHub == nil || cappedRaw[0].popTo != popTo || cappedRaw[len(cappedRaw)-1].popTo != popTo {
		t.Fatal("proof setup invalid: capped raw windows are not the first same-pop cross-product group")
	}
	cappedSelected := parser.selectReduceForkChildren(arena, act, cappedRaw)
	if len(cappedSelected) != 1 {
		t.Fatalf("capped selected windows = %d, want one C-selected same-group window", len(cappedSelected))
	}
	for i, fork := range cappedSelected {
		if stackEntryNode(fork.window[0]) == bestLeft {
			t.Fatalf("proof setup invalid: capped raw fork %d unexpectedly selected the late best left child", i)
		}
	}
	if got := firstLeftHub.linkCount(); got != len(cappedRaw) {
		t.Fatalf("proof setup invalid: capped raw windows = %d, want first predecessor product width %d", len(cappedRaw), got)
	}

	selected := parser.selectedReduceWindowsFromGSS(arena, act, &stack, int(act.ChildCount), maxMainLinkCount)
	if len(selected) != 1 {
		t.Fatalf("integrated selected windows = %d, want 1 same-group winner", len(selected))
	}
	if stackEntryNode(selected[0].window[0]) != bestLeft || stackEntryNode(selected[0].window[1]) != bestRight {
		t.Fatal("integrated selection did not scan the cross product past the raw cap to keep the late same-group winner")
	}

	var anyReduced bool
	nodeCount := 0
	parser.applyReduceActionForked(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if !anyReduced {
		t.Fatal("anyReduced = false, want true")
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want 1 selected reduction", nodeCount)
	}
	parent := stackEntryNode(stack.top())
	if parent == nil || parent.symbol != act.Symbol {
		t.Fatalf("top parent = %v, want symbol %d", parent, act.Symbol)
	}
	if len(parent.children) != 2 || parent.children[0] != bestLeft || parent.children[1] != bestRight {
		t.Fatal("forked reduce did not construct the parent from the late cross-product same-group winner")
	}
	if got := parent.dynamicPrecedence; got != bestLeft.dynamicPrecedence {
		t.Fatalf("parent dynamic precedence = %d, want %d", got, bestLeft.dynamicPrecedence)
	}
}

func TestReduceForkSelectionCapsDistinctCrossProductGroups(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	var head *gssNode
	totalWindows := 0
	for rightIdx := 0; rightIdx < 2; rightIdx++ {
		right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
		leftPopTo := scratch.allocNode(stackEntry{state: StateID(20 + rightIdx*maxMainLinkCount)}, nil, 1)
		left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
		leftHub := scratch.allocNode(newStackEntryNode(8, left), leftPopTo, 2)
		for leftIdx := 1; leftIdx < maxMainLinkCount; leftIdx++ {
			distinctPopTo := scratch.allocNode(stackEntry{state: StateID(20 + rightIdx*maxMainLinkCount + leftIdx)}, nil, 1)
			altLeft := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
			leftHub.extraLinks = append(leftHub.extraLinks, gssMainLink{
				prev:  distinctPopTo,
				entry: newStackEntryNode(8, altLeft),
			})
		}
		totalWindows += leftHub.linkCount()
		if rightIdx == 0 {
			head = scratch.allocNode(newStackEntryNode(9, right), leftHub, 3)
			continue
		}
		head.extraLinks = append(head.extraLinks, gssMainLink{
			prev:  leftHub,
			entry: newStackEntryNode(9, right),
		})
	}
	if got := head.linkCount(); got >= maxMainLinkCount {
		t.Fatalf("head link count = %d, want below production cap %d", got, maxMainLinkCount)
	}
	for i, count := 0, head.linkCount(); i < count; i++ {
		prev, _ := head.link(i)
		if got := prev.linkCount(); got != maxMainLinkCount {
			t.Fatalf("left predecessor %d link count = %d, want production cap %d", i, got, maxMainLinkCount)
		}
	}
	if totalWindows <= maxMainLinkCount {
		t.Fatalf("setup total windows = %d, want more than distinct group cap %d", totalWindows, maxMainLinkCount)
	}

	stack := glrStack{gss: gssStack{head: head}, byteOffset: 1}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}

	selected := parser.selectedReduceWindowsFromGSS(arena, act, &stack, int(act.ChildCount), maxMainLinkCount)
	if len(selected) != maxMainLinkCount {
		t.Fatalf("selected distinct groups = %d, want cap %d", len(selected), maxMainLinkCount)
	}
	seen := make(map[*gssNode]struct{}, len(selected))
	for i, fork := range selected {
		if _, ok := seen[fork.popTo]; ok {
			t.Fatalf("selected fork %d reused popTo %p, want distinct groups", i, fork.popTo)
		}
		seen[fork.popTo] = struct{}{}
	}
}

func TestSelectedReduceWindowsFromGSSCapsTraversalWork(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const childCount = 8
	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	prev := popTo
	for depth := 0; depth < childCount; depth++ {
		leaf := newLeafNodeInArena(arena, Symbol(1+depth), true, uint32(depth), uint32(depth+1), Point{Column: uint32(depth)}, Point{Column: uint32(depth + 1)})
		layer := scratch.allocNode(newStackEntryNode(StateID(20+depth), leaf), prev, depth+2)
		for alt := 1; alt < maxMainLinkCount; alt++ {
			altLeaf := newLeafNodeInArena(arena, Symbol(1+depth), true, uint32(depth), uint32(depth+1), Point{Column: uint32(depth)}, Point{Column: uint32(depth + 1)})
			layer.extraLinks = append(layer.extraLinks, gssMainLink{
				prev:  prev,
				entry: newStackEntryNode(StateID(20+depth), altLeaf),
			})
		}
		prev = layer
	}
	stack := glrStack{gss: gssStack{head: prev}, byteOffset: childCount}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "n1", Visible: true, Named: true},
		{Name: "n2", Visible: true, Named: true},
		{Name: "n3", Visible: true, Named: true},
		{Name: "n4", Visible: true, Named: true},
		{Name: "n5", Visible: true, Named: true},
		{Name: "n6", Visible: true, Named: true},
		{Name: "n7", Visible: true, Named: true},
		{Name: "n8", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}}
	act := ParseAction{Type: ParseActionReduce, Symbol: 9, ChildCount: childCount}
	budget := selectedReduceGSSWorkBudget(childCount, maxMainLinkCount)

	selected, work, capped := parser.selectedReduceWindowsFromGSSWithBudget(arena, act, &stack, int(act.ChildCount), maxMainLinkCount, budget)
	if !capped {
		t.Fatal("selected reduce traversal did not report hitting the work budget")
	}
	if work != budget {
		t.Fatalf("selected reduce traversal work = %d, want budget %d", work, budget)
	}
	if len(selected) == 0 {
		t.Fatal("selected reduce traversal returned no windows before hitting the work budget")
	}
	if len(selected) > maxMainLinkCount {
		t.Fatalf("selected windows = %d, want at most group cap %d", len(selected), maxMainLinkCount)
	}
}

func TestFaithfulForkReduceTargetMismatchFallsBackToPendingFork(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })
	if perfCountersEnabled {
		ResetPerfCounters()
	}

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNode(3, right), leftNode, 3)

	altBase := scratch.allocNode(stackEntry{state: 9}, nil, 1)
	altLeft := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	altLeftNode := scratch.allocNode(newStackEntryNode(8, altLeft), altBase, 2)
	altRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightNode.extraLinks = append(rightNode.extraLinks, gssMainLink{
		prev:  altLeftNode,
		entry: newStackEntryNode(7, altRight),
	})

	parser := &Parser{language: &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}}
	setSyntheticPostReducePackingSafeEOF(t, parser, 9)
	stack := glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if len(parser.pendingForkStacks) != 1 {
		t.Fatalf("pending forks = %d, want 1", len(parser.pendingForkStacks))
	}
	if stack.top().state == parser.pendingForkStacks[0].top().state {
		t.Fatalf("primary and pending top states both = %d, want mismatch fallback", stack.top().state)
	}
	if got := stack.gss.head.linkCount(); got != 1 {
		t.Fatalf("primary post-reduce head linkCount = %d, want 1", got)
	}
	if perfCountersEnabled {
		perf := PerfCountersSnapshot()
		if perf.ReduceForkCalls != 1 || perf.ReduceForkWindows != 2 || perf.ReduceForkMaxWindows != 2 {
			t.Fatalf("reduce fork counters = calls:%d windows:%d max:%d, want 1/2/2", perf.ReduceForkCalls, perf.ReduceForkWindows, perf.ReduceForkMaxWindows)
		}
		if perf.PostReduceMergeAttempts != 1 || perf.PostReduceMergePrimarySuccesses != 0 || perf.PendingForkStackAppends != 1 || perf.PendingForkStacksMaxLen != 1 {
			t.Fatalf("target-mismatch counters = attempts:%d primary:%d appends:%d max_pending:%d, want 1/0/1/1", perf.PostReduceMergeAttempts, perf.PostReduceMergePrimarySuccesses, perf.PendingForkStackAppends, perf.PendingForkStacksMaxLen)
		}
	}
}

func TestPostReduceForkMergeFinalizationRiskPredicate(t *testing.T) {
	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	stack := &glrStack{gss: gssStack{head: scratch.allocNode(stackEntry{state: 2}, base, 2)}, byteOffset: 1}
	eof := Token{Symbol: 0}

	t.Run("nil parser", func(t *testing.T) {
		var parser *Parser
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("dead stack", func(t *testing.T) {
		parser := &Parser{language: &Language{}}
		dead := *stack
		dead.dead = true
		if !parser.postReduceForkMergeHasFinalizationRisk(&dead, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("real current-lookahead single childful reduce is risky", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1}})
		parser.language.ParseTable[2][1] = parser.language.ParseTable[2][0]
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, Token{Symbol: 1}) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("non EOF shift lookahead", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionShift, State: 3}})
		parser.language.ParseTable[2][1] = parser.language.ParseTable[2][0]
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, Token{Symbol: 1}) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("missing EOF action", func(t *testing.T) {
		parser := &Parser{language: &Language{
			StateCount:     3,
			SymbolCount:    4,
			TokenCount:     3,
			ParseActions:   []ParseActionEntry{{}},
			ParseTable:     [][]uint16{{0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
			SymbolMetadata: []SymbolMetadata{{Name: "eof"}},
		}}
		parser.denseLimit = len(parser.language.ParseTable)
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("out of range EOF action", func(t *testing.T) {
		parser := &Parser{language: &Language{
			StateCount:     3,
			SymbolCount:    4,
			TokenCount:     3,
			ParseActions:   []ParseActionEntry{{}},
			ParseTable:     [][]uint16{{0, 0, 0, 0}, {0, 0, 0, 0}, {5, 0, 0, 0}},
			SymbolMetadata: []SymbolMetadata{{Name: "eof"}},
		}}
		parser.denseLimit = len(parser.language.ParseTable)
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("single childful EOF reduce", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1}})
		if parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = true, want false")
		}
	})
	t.Run("EOF accept", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionAccept}})
		if parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = true, want false")
		}
	})
	t.Run("no-lookahead is risky", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1}})
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, Token{Symbol: 0, NoLookahead: true}) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("zero child EOF reduce", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 0}})
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("mixed EOF actions", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{
			{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
			{Type: ParseActionAccept},
		})
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("single EOF shift", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionShift, State: 3}})
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
}

func TestFaithfulForkReduceEOFNoActionKeepsPendingFork(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	parser, stack, act := buildTwoWindowFullGSSReduceCase(t, &scratch, arena)
	parser.language.StateCount = 4
	parser.language.SymbolCount = 4
	parser.language.TokenCount = 3
	parser.language.ParseActions = []ParseActionEntry{{}}
	parser.language.ParseTable = [][]uint16{
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
	}
	parser.denseLimit = len(parser.language.ParseTable)
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{Symbol: 0}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if got := stack.gss.head.linkCount(); got != 1 {
		t.Fatalf("primary post-reduce head linkCount = %d, want 1", got)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0 after constructed-parent same-pop selection", len(parser.pendingForkStacks))
	}
}

func TestFaithfulForkReduceEOFZeroChildReduceKeepsPendingFork(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	parser, stack, act := buildTwoWindowFullGSSReduceCase(t, &scratch, arena)
	setSyntheticEOFAction(t, parser, 1, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 0}})
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{Symbol: 0}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if got := stack.gss.head.linkCount(); got != 1 {
		t.Fatalf("primary post-reduce head linkCount = %d, want 1", got)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0 after constructed-parent same-pop selection", len(parser.pendingForkStacks))
	}
}

func TestTryMergePostReduceForkDeclinesAcceptedStacks(t *testing.T) {
	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := glrStack{
		gss:        gssStack{head: scratch.allocNode(stackEntry{state: 2}, base, 2)},
		byteOffset: 3,
		accepted:   true,
	}
	right := glrStack{
		gss:        gssStack{head: scratch.allocNode(stackEntry{state: 2}, base, 2)},
		byteOffset: 3,
		accepted:   true,
	}

	if tryMergePostReduceFork(nil, &left, &right) {
		t.Fatal("tryMergePostReduceFork = true, want false for accepted stacks")
	}
}

func TestTryMergePostReduceForkRejectsDistinctCRecoveryCosts(t *testing.T) {
	var scratch gssScratch
	build := func(sym Symbol, paused bool) glrStack {
		node := NewLeafNode(sym, true, 0, 5, Point{}, Point{Column: 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &scratch),
			byteOffset: stackByteOffset(entries),
			cPaused:    paused,
		}
	}

	clean := build(11, false)
	paused := build(12, true)
	if cleanCost, pausedCost := cStackErrorCostForMerge(nil, &clean), cStackErrorCostForMerge(nil, &paused); cleanCost == pausedCost {
		t.Fatalf("test setup costs equal: clean=%d paused=%d", cleanCost, pausedCost)
	}
	parser := &Parser{errorCostCompetition: true}

	if tryMergePostReduceFork(parser, &clean, &paused) {
		t.Fatal("tryMergePostReduceFork = true, want false for distinct C recovery costs")
	}
	if got := clean.gss.head.linkCount(); got != 1 {
		t.Fatalf("clean link count after rejected merge = %d, want 1", got)
	}

	noTreeParser := &Parser{errorCostCompetition: true, noTreeBenchmarkOnly: true}
	clean = build(11, false)
	paused = build(12, true)
	if !tryMergePostReduceFork(noTreeParser, &clean, &paused) {
		t.Fatal("tryMergePostReduceFork = false, want true when C cost competition is disabled")
	}
	if got := clean.gss.head.linkCount(); got != 2 {
		t.Fatalf("clean link count after no-tree merge = %d, want 2", got)
	}
}

func TestNoTreeReduceFromGSSRejectsMultiLinkSpan(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newNoTreeLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newNoTreeLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNoTreeNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNoTreeNode(3, right), leftNode, 3)
	altRight := newNoTreeLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightNode.extraLinks = append(rightNode.extraLinks, gssMainLink{
		prev:  leftNode,
		entry: newStackEntryNoTreeNode(3, altRight),
	})

	parser := &Parser{language: &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}}
	stack := &glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}
	var anyReduced bool
	nodeCount := 0

	parser.applyNoTreeReduceActionFromGSS(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false)
	if !stack.dead {
		t.Fatal("stack.dead = false, want true for no-tree multi-link GSS reduce")
	}
	if anyReduced {
		t.Fatal("anyReduced = true, want false")
	}
	if nodeCount != 0 {
		t.Fatalf("nodeCount = %d, want 0", nodeCount)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0", len(parser.pendingForkStacks))
	}
}

func TestRejectUndrainedPendingForkStacksClearsAndKills(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	parser := &Parser{pendingForkStacks: []glrStack{newGLRStack(9)}}
	stack := newGLRStack(1)

	if !parser.rejectUndrainedPendingForkStacks(&stack) {
		t.Fatal("rejectUndrainedPendingForkStacks = false, want true")
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0", len(parser.pendingForkStacks))
	}
	if !stack.dead {
		t.Fatal("stack.dead = false, want true")
	}
}

func TestBuildReduceChainHintsUsesLanguageMetadata(t *testing.T) {
	t.Setenv("GOT_GLR_REDUCE_CHAIN_HINTS", "1")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	lang := &Language{
		Name:        "python",
		StateCount:  10,
		SymbolCount: 10,
		SymbolNames: []string{
			"", "", "", "", "", "", "", "", "", "",
		},
		ReduceChainHints: []ReduceChainHint{{
			StartState:     StateID(3),
			Lookahead:      Symbol(2),
			TerminalStates: []StateID{StateID(4), StateID(5)},
			TerminalAction: ReduceChainTerminalSingleShift,
			MaxSteps:       7,
		}},
	}

	got := buildReduceChainHints(lang)
	if len(got) != 1 {
		t.Fatalf("hint count = %d, want 1", len(got))
	}
	hint := got[0]
	if hint.startState != StateID(3) || hint.lookahead != Symbol(2) || hint.maxSteps != 7 {
		t.Fatalf("hint = %+v, want state=3 lookahead=2 maxSteps=7", hint)
	}
	if hint.terminalAction != classifiedParseActionSingleShift {
		t.Fatalf("terminal action = %d, want single shift", hint.terminalAction)
	}
	if len(hint.terminalStates) != 2 || hint.terminalStates[0] != StateID(4) || hint.terminalStates[1] != StateID(5) {
		t.Fatalf("terminal states = %v, want [4 5]", hint.terminalStates)
	}

	lang.ReduceChainHints[0].TerminalStates[0] = StateID(9)
	if hint.terminalStates[0] != StateID(4) {
		t.Fatalf("internal hint terminal states alias language metadata: got %v", hint.terminalStates)
	}
}

func TestReduceChainHintForUsesStateIndex(t *testing.T) {
	p := &Parser{
		reduceChainHints: []reduceChainHint{
			{startState: StateID(8), lookahead: Symbol(3), maxSteps: 4},
			{startState: StateID(10), lookahead: Symbol(4), maxSteps: 5},
			{startState: StateID(10), lookahead: Symbol(5), maxSteps: 6},
		},
	}
	p.reduceChainHintByState = buildReduceChainHintIndex(p.reduceChainHints)

	hint, ok := p.reduceChainHintFor(StateID(8), Symbol(3))
	if !ok || hint.maxSteps != 4 {
		t.Fatalf("hint for state=8 lookahead=3 = %+v, %v; want maxSteps=4, true", hint, ok)
	}
	hint, ok = p.reduceChainHintFor(StateID(10), Symbol(5))
	if !ok || hint.maxSteps != 6 {
		t.Fatalf("hint for duplicate state=10 lookahead=5 = %+v, %v; want maxSteps=6, true", hint, ok)
	}
	if _, ok := p.reduceChainHintFor(StateID(9), Symbol(3)); ok {
		t.Fatal("unexpected hint for state without entry")
	}
	if _, ok := p.reduceChainHintFor(StateID(10), Symbol(6)); ok {
		t.Fatal("unexpected hint for duplicate state with unmatched lookahead")
	}
}

func TestCanCollapseNamedLeafWrapperSingleAnonymousToken(t *testing.T) {
	p := &Parser{
		language: &Language{
			SymbolMetadata: []SymbolMetadata{
				{Name: "EOF"},
				{Name: "optional_chain", Visible: true, Named: true},
				{Name: "?.", Visible: true, Named: false},
				{Name: "identifier", Visible: true, Named: true},
				{Name: "_hidden", Visible: false, Named: false},
			},
		},
	}

	if !p.canCollapseNamedLeafWrapper(1, 2) {
		t.Fatal("expected visible named wrapper over visible anonymous token to collapse")
	}
	if p.canCollapseNamedLeafWrapper(1, 3) {
		t.Fatal("did not expect visible named wrapper over visible named child to collapse")
	}
	if p.canCollapseNamedLeafWrapper(1, 4) {
		t.Fatal("did not expect visible named wrapper over invisible child to collapse")
	}
}

// A named rule wrapping a DIFFERENT-named visible anonymous token (e.g.
// optional_chain over "?.") must NOT collapse to a childless leaf: C tree-sitter
// keeps the anonymous token as a visible child (childCount==1). The unary
// self-reduction collapse must therefore decline (return nil) so the normal
// reduce keeps the child.
func TestCollapsibleUnarySelfReductionKeepsDifferentNamedAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "optional_chain", Visible: true, Named: true},
			{Name: "?.", Visible: true, Named: false},
		},
	}
	p := &Parser{language: lang}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil); got != nil {
		t.Fatalf("expected different-named visible anon child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionKeepsDifferentNamedAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "optional_chain", Visible: true, Named: true},
			{Name: "?.", Visible: true, Named: false},
		},
	}
	p := &Parser{language: lang}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("expected different-named visible anon child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleUnarySelfReductionKeepsStarlarkContinueTokenChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "continue_statement", Visible: true, Named: true},
			{Name: "continue", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 2209, 2217, Point{Row: 73, Column: 16}, Point{Row: 73, Column: 24})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil); got != nil {
		t.Fatalf("expected Starlark continue token child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionKeepsStarlarkContinueTokenChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "continue_statement", Visible: true, Named: true},
			{Name: "continue", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 2209, 2217, Point{Row: 73, Column: 16}, Point{Row: 73, Column: 24})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("expected raw Starlark continue token child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleUnarySelfReductionKeepsSingleTokenWrapperDifferentAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "wildcard_pattern", Visible: true, Named: true},
			{Name: "_", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 4, 5, Point{Column: 4}, Point{Column: 5})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil); got != nil {
		t.Fatalf("expected different-named wildcard token child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionKeepsSingleTokenWrapperDifferentAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "wildcard_pattern", Visible: true, Named: true},
			{Name: "_", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 4, 5, Point{Column: 4}, Point{Column: 5})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("expected raw different-named wildcard token child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleUnarySelfReductionCollapsesSameNamedInlinedTokenChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "nil", Visible: true, Named: true},
			{Name: "nil", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 4, 7, Point{Column: 4}, Point{Column: 7})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil)
	if got == nil {
		t.Fatal("expected same-named inlined token child to collapse")
	}
	if got.symbol != 1 {
		t.Fatalf("collapsed symbol = %d, want 1", got.symbol)
	}
	if !got.IsNamed() {
		t.Fatal("collapsed node should be named")
	}
	if got.ChildCount() != 0 {
		t.Fatalf("collapsed ChildCount = %d, want 0", got.ChildCount())
	}
	if got.StartByte() != 4 || got.EndByte() != 7 {
		t.Fatalf("collapsed range = [%d,%d], want [4,7]", got.StartByte(), got.EndByte())
	}
}

func TestCollapsibleRawUnarySelfReductionCollapsesSameNamedInlinedTokenChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "nil", Visible: true, Named: true},
			{Name: "nil", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 4, 7, Point{Column: 4}, Point{Column: 7})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1)
	if got == nil {
		t.Fatal("expected raw same-named inlined token child to collapse")
	}
	if got.symbol != 1 {
		t.Fatalf("collapsed symbol = %d, want 1", got.symbol)
	}
	if !got.IsNamed() {
		t.Fatal("collapsed node should be named")
	}
	if got.ChildCount() != 0 {
		t.Fatalf("collapsed ChildCount = %d, want 0", got.ChildCount())
	}
	if got.StartByte() != 4 || got.EndByte() != 7 {
		t.Fatalf("collapsed range = [%d,%d], want [4,7]", got.StartByte(), got.EndByte())
	}
}

func TestCollapsibleUnarySelfReductionKeepsSharedSingleTokenWrapperAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "self_expression", Visible: true, Named: true},
			{Name: "self", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language:                   lang,
		sharedAnonymousTokenSymbol: []bool{false, false, true},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 0, 4, Point{}, Point{Column: 4})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil); got != nil {
		t.Fatalf("expected shared anonymous token child to be kept, got collapsed node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionKeepsSharedSingleTokenWrapperAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "self_expression", Visible: true, Named: true},
			{Name: "self", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language:                   lang,
		sharedAnonymousTokenSymbol: []bool{false, false, true},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 0, 4, Point{}, Point{Column: 4})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("expected raw shared anonymous token child to be kept, got collapsed node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleUnarySelfReductionKeepsSharedSameSymbolAnonymousChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "shared", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language:                   lang,
		sharedAnonymousTokenSymbol: []bool{false, true},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 1, false, 12, 13, Point{Column: 12}, Point{Column: 13})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil); got != nil {
		t.Fatalf("expected shared same-symbol anonymous child to be kept, got collapsed node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionKeepsSharedSameSymbolAnonymousChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "shared", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language:                   lang,
		sharedAnonymousTokenSymbol: []bool{false, true},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 1, false, 12, 13, Point{Column: 12}, Point{Column: 13})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("expected raw shared same-symbol anonymous child to be kept, got collapsed node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionRejectsInvisibleChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "optional_chain", Visible: true, Named: true},
			{Name: "_hidden", Visible: false, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("raw unary collapse returned %v for invisible child", got)
	}
}

func TestCollapsibleRawUnarySelfReductionCollapsesGeneratedHiddenChoicePassthrough(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "_statement", Visible: false, Named: true},
			{Name: "_simple_statements", Visible: false, Named: true},
			{Name: "import_statement", Visible: true, Named: true},
		},
		HiddenChoicePassthroughSymbols: []bool{false, true, false, false},
	}
	p := &Parser{language: lang}
	arena := newNodeArena(arenaClassFull)
	importStmt := newLeafNodeInArena(arena, 3, true, 1, 7, Point{Column: 1}, Point{Column: 7})
	child := newParentNodeInArena(arena, 2, true, []*Node{importStmt}, nil, 0)
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1)
	if got != child {
		t.Fatalf("raw unary collapse = %p, want hidden choice child %p", got, child)
	}
}

func TestNonTerminalAliasMapPreservesInvisibleUnaryWrappers(t *testing.T) {
	lang := &Language{
		Name:        "synthetic",
		SymbolNames: []string{"EOF", "literal", "_aliased_wrapper", "alias_target", "_plain_wrapper"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "literal", Visible: true, Named: true},
			{Name: "_aliased_wrapper", Visible: false, Named: false},
			{Name: "alias_target", Visible: true, Named: true},
			{Name: "_plain_wrapper", Visible: false, Named: false},
		},
		HiddenChoicePassthroughSymbols: []bool{false, false, true, false, true},
		NonTerminalAliasMap: [][]Symbol{
			nil,
			nil,
			{2, 3},
		},
	}

	marked := buildAliasPreservedWrapperSymbols(lang)
	if !symbolMarked(marked, 2) {
		t.Fatalf("alias map did not preserve wrapper symbol 2")
	}
	if symbolMarked(marked, 4) {
		t.Fatalf("alias map unexpectedly preserved plain wrapper symbol 4")
	}

	p := NewParser(lang)
	if p.canCollapseInvisibleUnaryWrapperSymbol(2) {
		t.Fatalf("aliased wrapper should not collapse as an invisible unary wrapper")
	}
	if p.canCollapseHiddenChoicePassthroughSymbol(2) {
		t.Fatalf("aliased wrapper should not collapse as hidden choice passthrough")
	}
	if !p.canCollapseInvisibleUnaryWrapperSymbol(4) {
		t.Fatalf("plain invisible wrapper should remain collapsible")
	}
	if !p.canCollapseHiddenChoicePassthroughSymbol(4) {
		t.Fatalf("plain hidden choice passthrough should remain collapsible")
	}
}

func TestReduceProductionHasEffectiveFieldsIgnoresConflictedZeroFields(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expr", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
		},
		FieldNames: []string{"", "left", "right"},
		FieldMapSlices: [][2]uint16{
			{0, 2},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
			{FieldID: 2, ChildIndex: 0, Inherited: true},
		},
		ParseActions: []ParseActionEntry{
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 1, ChildCount: 1, ProductionID: 0}}},
		},
	}
	p := NewParser(lang)
	arena := newNodeArena(arenaClassFull)

	if p.reduceProductionHasFields(0) {
		t.Fatal("reduceProductionHasFields = true, want false for conflicted zero field IDs")
	}
	if p.reduceProductionHasEffectiveFields(1, 0, arena) {
		t.Fatal("reduceProductionHasEffectiveFields = true, want false for conflicted zero field IDs")
	}
	fieldIDs, _ := p.buildFieldIDs(1, 0, arena)
	if got := len(fieldIDs); got != 1 {
		t.Fatalf("buildFieldIDs len = %d, want 1", got)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("buildFieldIDs[0] = %d, want 0", got)
	}
}

func TestTryPushPendingNoFieldParentAllowsEffectiveNoFieldProduction(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expr", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
		},
		FieldNames: []string{"", "left", "right"},
		FieldMapSlices: [][2]uint16{
			{0, 2},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
			{FieldID: 2, ChildIndex: 0, Inherited: true},
		},
		ParseActions: []ParseActionEntry{
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 1, ChildCount: 1, ProductionID: 0}}},
		},
	}
	p := NewParser(lang)
	p.pendingFullParents = true
	arena := newNodeArena(arenaClassFull)
	leaf := newCompactFullLeafInArena(arena, 2, true, 1, 3, Point{Column: 1}, Point{Column: 3})
	entry := newStackEntryCompactFullLeaf(4, leaf)
	stack := &glrStack{entries: []stackEntry{entry}}
	act := ParseAction{Symbol: 1, ChildCount: 1, ProductionID: 0}
	anyReduced := false
	nodeCount := 0

	if !p.tryPushPendingNoFieldParent(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, nil, []stackEntry{entry}, 0, 1, 1, 0, 0) {
		t.Fatal("tryPushPendingNoFieldParent = false, want true for effective no-field production")
	}
	if !anyReduced {
		t.Fatal("anyReduced = false, want true")
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want 1", nodeCount)
	}
	if got := arena.pendingParentRejectedFields; got != 0 {
		t.Fatalf("pendingParentRejectedFields = %d, want 0", got)
	}
	if got := arena.pendingParentCreated; got != 1 {
		t.Fatalf("pendingParentCreated = %d, want 1", got)
	}
	if got := len(stack.entries); got != 1 {
		t.Fatalf("stack entries = %d, want 1", got)
	}
	parent := stackEntryPendingParent(stack.entries[0])
	if parent == nil {
		t.Fatal("stack entry is not a pending parent")
	}
	if got := parent.childEntryCount(); got != 1 {
		t.Fatalf("pending parent child count = %d, want 1", got)
	}
}

func TestTryPushPendingNoFieldParentCountsOrdinaryHiddenNodeRefs(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expr", Visible: true, Named: true},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
	}
	p := NewParser(lang)
	p.pendingFullParents = true
	arena := newNodeArena(arenaClassFull)
	first := newLeafNodeInArena(arena, 3, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	second := newLeafNodeInArena(arena, 3, true, 3, 4, Point{Column: 3}, Point{Column: 4})
	hidden := newParentNodeInArena(arena, 2, false, []*Node{first, second}, nil, 0)
	entry := newStackEntryNode(4, hidden)
	stack := &glrStack{entries: []stackEntry{entry}}
	act := ParseAction{Symbol: 1, ChildCount: 1, ProductionID: 0}
	anyReduced := false
	nodeCount := 0

	if !p.tryPushPendingNoFieldParent(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, nil, []stackEntry{entry}, 0, 1, 1, 0, 0) {
		t.Fatal("tryPushPendingNoFieldParent = false, want true")
	}
	if got := arena.pendingParentsFlattened; got != 0 {
		t.Fatalf("pendingParentsFlattened = %d, want 0 for ordinary hidden node", got)
	}
	if got := arena.pendingChildRefsFlattened; got != 2 {
		t.Fatalf("pendingChildRefsFlattened = %d, want 2", got)
	}
	parent := stackEntryPendingParent(stack.entries[0])
	if parent == nil {
		t.Fatal("stack entry is not a pending parent")
	}
	if got := parent.childEntryCount(); got != 2 {
		t.Fatalf("pending parent child count = %d, want 2", got)
	}
}

func TestTryPushPendingNoFieldParentKeepsHiddenCompactLeafCompact(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expr", Visible: true, Named: true},
			{Name: "_hidden", Visible: false, Named: false},
		},
	}
	p := NewParser(lang)
	p.pendingFullParents = true
	arena := newNodeArena(arenaClassFull)
	leaf := newCompactFullLeafInArena(arena, 2, false, 5, 8, Point{Column: 5}, Point{Column: 8})
	entry := newStackEntryCompactFullLeaf(4, leaf)
	stack := &glrStack{entries: []stackEntry{entry}}
	act := ParseAction{Symbol: 1, ChildCount: 1, ProductionID: 0}
	anyReduced := false
	nodeCount := 0

	if !p.tryPushPendingNoFieldParent(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, nil, []stackEntry{entry}, 0, 1, 1, 0, 0) {
		t.Fatal("tryPushPendingNoFieldParent = false, want true for hidden compact leaf")
	}
	if got := arena.compactFullLeafMaterialized; got != 0 {
		t.Fatalf("compactFullLeafMaterialized = %d, want 0", got)
	}
	parent := stackEntryPendingParent(stack.entries[0])
	if parent == nil {
		t.Fatal("stack entry is not a pending parent")
	}
	if got := parent.childEntryCount(); got != 0 {
		t.Fatalf("pending parent child count = %d, want 0", got)
	}
	if parent.startByte != 5 || parent.endByte != 8 {
		t.Fatalf("pending parent span = [%d,%d], want [5,8]", parent.startByte, parent.endByte)
	}
}

func TestCollapsibleRawUnarySelfReductionEntryCollapsesPendingParentSameSymbol(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expr", Visible: true, Named: true},
		},
	}
	p := &Parser{language: lang}
	arena := newNodeArena(arenaClassFull)
	parent := newPendingParentInArena(arena, 1, true, 3, nil, 1, 3, Point{Column: 1}, Point{Column: 3}, false)
	entry := newStackEntryPendingParent(4, parent)
	act := ParseAction{Symbol: 1, ChildCount: 1, ProductionID: 9}

	got, ok := p.collapsibleRawUnarySelfReductionEntry(act, Token{}, arena, []stackEntry{entry}, 0, 1)
	if !ok {
		t.Fatal("expected pending parent raw unary reduction to collapse")
	}
	if stackEntryPendingParent(got) != parent {
		t.Fatal("collapsed entry did not preserve pending parent payload")
	}
	setCollapsedUnaryEntryMetadata(&got, act, false, 2, 5)
	if parent.productionID != 9 || parent.preGotoState != 2 || parent.parseState != 5 || got.state != 5 {
		t.Fatalf("pending parent metadata = prod %d pre %d state %d entry %d", parent.productionID, parent.preGotoState, parent.parseState, got.state)
	}
}

func TestCollapsibleRawUnarySelfReductionEntryCollapsesPendingParentInvisibleWrapper(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "_wrapper", Visible: false, Named: false},
			{Name: "expr", Visible: true, Named: true},
		},
	}
	p := &Parser{language: lang}
	arena := newNodeArena(arenaClassFull)
	parent := newPendingParentInArena(arena, 2, true, 3, nil, 1, 3, Point{Column: 1}, Point{Column: 3}, false)
	entry := newStackEntryPendingParent(4, parent)
	act := ParseAction{Symbol: 1, ChildCount: 1, ProductionID: 9}

	got, ok := p.collapsibleRawUnarySelfReductionEntry(act, Token{}, arena, []stackEntry{entry}, 0, 1)
	if !ok {
		t.Fatal("expected invisible wrapper over pending parent to collapse")
	}
	if stackEntryPendingParent(got) != parent {
		t.Fatal("collapsed wrapper did not preserve pending parent payload")
	}
}
