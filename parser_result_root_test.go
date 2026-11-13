package gotreesitter

import (
	"sync"
	"testing"
	"unsafe"
)

func TestIncrementalResultRootIgnoresOldErrorSymbol(t *testing.T) {
	parser := &Parser{rootSymbol: 3, hasRootSymbol: true}
	old := &Tree{root: &Node{symbol: errorSymbol}}
	build := newResultRootBuild(parser, nil, nil, old, nil, nil)
	if !build.hasExpectedRoot || build.expectedRootSymbol != 3 {
		t.Fatalf("expected root symbol = %d, want 3", build.expectedRootSymbol)
	}
}

func TestAttachResultRootExtraSplitOmitsZeroWidthChildren(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "source_file", "comment"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	child := newLeafNodeInArena(arena, 1, true, 1, 3, Point{Column: 1}, Point{Column: 3})
	root := newParentNodeInArena(arena, 1, true, []*Node{child}, nil, 0)
	zeroWidth := newLeafNodeInArena(arena, 2, true, 0, 0, Point{}, Point{})
	zeroWidth.setExtra(true)
	positiveWidth := newLeafNodeInArena(arena, 2, true, 3, 4, Point{Column: 3}, Point{Column: 4})
	positiveWidth.setExtra(true)

	split := classifyResultRootExtras([]*Node{zeroWidth, positiveWidth}, lang)
	attachResultRootExtraSplit(root, split, arena)

	children := resultChildSliceForMutation(root)
	if len(children) != 2 || children[0] != child || children[1] != positiveWidth {
		t.Fatalf("attached root children=%+v", children)
	}
	if root.startByte != 0 || root.endByte != 4 {
		t.Fatalf("attached root span=%d..%d, want 0..4", root.startByte, root.endByte)
	}
}

func TestAttachResultRootExtraSplitOmitsZeroWidthFinalRefsWithoutDrain(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "source_file", "declaration", "comment"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "declaration", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
		},
	}
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true

	child := newCompactFullLeafInArena(
		arena,
		2,
		true,
		1,
		3,
		Point{Column: 1},
		Point{Column: 3},
	)
	child.parseState = 12
	parent := newPendingParentInArena(
		arena,
		1,
		true,
		0,
		[]stackEntry{newStackEntryCompactFullLeaf(child.parseState, child)},
		1,
		3,
		Point{Column: 1},
		Point{Column: 3},
		false,
	)
	parent.parseState = 13
	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(
		arena,
		&entry,
		pendingParentMaterializeForFinalTree,
	)

	zeroWidth := newLeafNodeInArena(arena, 3, true, 0, 0, Point{}, Point{})
	zeroWidth.setExtra(true)
	positiveWidth := newLeafNodeInArena(
		arena,
		3,
		true,
		3,
		4,
		Point{Column: 3},
		Point{Column: 4},
	)
	positiveWidth.setExtra(true)

	split := classifyResultRootExtras([]*Node{zeroWidth, positiveWidth}, lang)
	attachResultRootExtraSplit(root, split, arena)

	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("materialized parent count = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("materialized child count = %d, want 0", got)
	}
	if !nodeHasFinalChildRefs(root) {
		t.Fatal("root lost final-child refs")
	}
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got := root.Child(1); got != positiveWidth {
		t.Fatalf("root child 1 = %p, want %p", got, positiveWidth)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("materialized child count after edge access = %d, want 0", got)
	}
	if got, want := root.startByte, uint32(0); got != want {
		t.Fatalf("root start = %d, want %d", got, want)
	}
	if got, want := root.endByte, uint32(4); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
}

func newRootFrameReplayLanguage(name, rootName, childName string, repeat bool) *Language {
	const (
		childSym = Symbol(3)
	)
	state2 := make([]uint16, 4)
	state2[0] = 1
	if repeat {
		state2[childSym] = 2
	}
	return &Language{
		Name:            name,
		TokenCount:      2,
		SymbolCount:     4,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		SymbolNames:     []string{"EOF", "_token", rootName, childName},
		SymbolMetadata:  []SymbolMetadata{{Name: "EOF"}, {Name: "_token"}, {Name: rootName, Visible: true, Named: true}, {Name: childName, Visible: true, Named: true}},
		ParseActions:    []ParseActionEntry{{}, {Actions: []ParseAction{{Type: ParseActionAccept}}}},
		ParseTable:      [][]uint16{{}, {0, 0, 0, 2}, state2},
	}
}

func newRootFrameReplayParser(lang *Language) *Parser {
	parser := NewParser(lang)
	parser.rootSymbol = 2
	parser.hasRootSymbol = true
	return parser
}

func newRootFrameReplayGapLanguage() *Language {
	const (
		newlineSym = Symbol(2)
		rootSym    = Symbol(3)
		childSym   = Symbol(4)
		repeatSym  = Symbol(5)
	)
	rows := make([][]uint16, 7)
	for i := range rows {
		rows[i] = make([]uint16, 7)
	}
	rows[1][rootSym] = 2
	rows[1][childSym] = 3
	rows[1][repeatSym] = 4
	rows[2][0] = 1
	rows[3][newlineSym] = 2
	rows[4][0] = 5
	rows[4][childSym] = 6
	rows[5][0] = 4
	rows[6][0] = 4

	lexModes := make([]LexMode, 7)
	for i := range lexModes {
		lexModes[i] = LexMode{LexState: 0}
	}
	lexModes[3] = LexMode{LexState: 1}

	return &Language{
		Name:            "gap_root",
		TokenCount:      3,
		SymbolCount:     7,
		StateCount:      7,
		LargeStateCount: 7,
		InitialState:    1,
		SymbolNames:     []string{"EOF", "_token", "_newline", "source_file", "declaration", "source_file_repeat1", "comment"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "_token"},
			{Name: "_newline"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "declaration", Visible: true, Named: true},
			{Name: "source_file_repeat1"},
			{Name: "comment", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 5}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: repeatSym, ChildCount: 1}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: repeatSym, ChildCount: 2}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: rootSym, ChildCount: 1}}},
		},
		ParseTable: rows,
		LexModes:   lexModes,
		LexStates: []LexState{
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '\n', Hi: '\n', NextState: 2}}},
			{AcceptToken: newlineSym, Default: -1, EOF: -1},
			{AcceptToken: newlineSym, Default: -1, EOF: -1},
		},
		ZeroWidthTokens: []bool{false, false, true},
	}
}

func newRootFrameReplayReduceLanguage() *Language {
	const (
		rootSym      = Symbol(2)
		childSym     = Symbol(3)
		childListSym = Symbol(4)
	)
	rows := make([][]uint16, 6)
	for i := range rows {
		rows[i] = make([]uint16, 5)
	}
	rows[1][rootSym] = 2
	rows[1][childSym] = 3
	rows[1][childListSym] = 4
	rows[2][0] = 1
	rows[3][0] = 2
	rows[4][0] = 3
	rows[4][childSym] = 5
	rows[5][0] = 4

	return &Language{
		Name:            "reduce_root",
		TokenCount:      2,
		SymbolCount:     5,
		StateCount:      6,
		LargeStateCount: 6,
		InitialState:    1,
		SymbolNames:     []string{"EOF", "_token", "source_file", "declaration", "source_file_repeat1"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "_token"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "declaration", Visible: true, Named: true},
			{Name: "source_file_repeat1"},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: childListSym, ChildCount: 1}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: rootSym, ChildCount: 1}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: childListSym, ChildCount: 2}}},
		},
		ParseTable: rows,
	}
}

func TestSyntheticRootReplayStackCanonicalPushPop(t *testing.T) {
	builder := resultRootBuild{}
	empty := syntheticRootReplayFrame{}
	if _, ok := builder.syntheticRootReplayTopState(empty); ok {
		t.Fatal("empty replay stack unexpectedly has a top state")
	}
	if _, ok := builder.syntheticRootReplayPop(empty, 0); ok {
		t.Fatal("empty replay stack unexpectedly supports pop")
	}

	base := builder.syntheticRootReplayPush(empty, 1)
	if base.top == 0 {
		t.Fatal("initial push returned an empty replay frame")
	}
	if got, ok := builder.syntheticRootReplayTopState(base); !ok || got != 1 {
		t.Fatalf("base top = %d, %v; want 1, true", got, ok)
	}
	if duplicate := builder.syntheticRootReplayPush(empty, 1); duplicate.top != base.top {
		t.Fatalf("canonical initial push top = %d, want %d", duplicate.top, base.top)
	}

	left := builder.syntheticRootReplayPush(base, 2)
	right := builder.syntheticRootReplayPush(base, 3)
	if left.top == right.top {
		t.Fatal("distinct sibling pushes aliased the same replay frame")
	}
	if got, _ := builder.syntheticRootReplayTopState(base); got != 1 {
		t.Fatalf("sibling pushes mutated base top to %d, want 1", got)
	}
	if duplicate := builder.syntheticRootReplayPush(base, 2); duplicate.top != left.top {
		t.Fatalf("canonical shared-prefix push top = %d, want %d", duplicate.top, left.top)
	}

	popped, ok := builder.syntheticRootReplayPop(left, 1)
	if !ok || popped.top != base.top {
		t.Fatalf("pop(left, 1) = {%d, %v}, want base top %d", popped.top, ok, base.top)
	}
	if popped, ok := builder.syntheticRootReplayPop(left, 0); !ok || popped.top != left.top {
		t.Fatalf("pop(left, 0) = {%d, %v}, want left top %d", popped.top, ok, left.top)
	}
	if _, ok := builder.syntheticRootReplayPop(left, 2); ok {
		t.Fatal("pop through the initial state unexpectedly succeeded")
	}
	if _, ok := builder.syntheticRootReplayPop(left, 3); ok {
		t.Fatal("pop beyond replay stack depth unexpectedly succeeded")
	}
}

func TestSyntheticRootReplayFrameSetScratch(t *testing.T) {
	var scratch syntheticRootReplayFrameSetScratch
	frames := make([]syntheticRootReplayFrame, 0, syntheticRootReplayMaxFrontier)
	for top := uint32(1); top <= syntheticRootReplayMaxFrontier; top++ {
		frames = scratch.append(frames, syntheticRootReplayFrame{top: top})
	}
	if got, want := len(frames), syntheticRootReplayMaxFrontier; got != want {
		t.Fatalf("frame set length = %d, want %d", got, want)
	}
	for top := uint32(1); top <= syntheticRootReplayMaxFrontier; top++ {
		frames = scratch.append(frames, syntheticRootReplayFrame{top: top})
	}
	if got, want := len(frames), syntheticRootReplayMaxFrontier; got != want {
		t.Fatalf("frame set length after duplicates = %d, want %d", got, want)
	}

	scratch.reset()
	frames = frames[:0]
	frames = scratch.append(frames, syntheticRootReplayFrame{top: syntheticRootReplayMaxFrontier + 1})
	if got, want := len(frames), 1; got != want {
		t.Fatalf("reset frame set length = %d, want %d", got, want)
	}
}

func TestSyntheticRootReplayGapCursorSetScratch(t *testing.T) {
	var scratch syntheticRootReplayGapCursorSetScratch
	cursors := make([]syntheticRootReplayGapCursor, 0, syntheticRootReplayMaxFrontier)
	for top := uint32(1); top <= syntheticRootReplayMaxFrontier; top++ {
		point := Point{Row: top / 8, Column: top % 8}
		cursors = scratch.append(cursors, syntheticRootReplayFrame{top: top}, top*3, point)
	}
	if got, want := len(cursors), syntheticRootReplayMaxFrontier; got != want {
		t.Fatalf("gap cursor set length = %d, want %d", got, want)
	}
	for _, cursor := range cursors {
		cursors = scratch.append(cursors, cursor.frame, cursor.byte, cursor.point)
	}
	if got, want := len(cursors), syntheticRootReplayMaxFrontier; got != want {
		t.Fatalf("gap cursor set length after duplicates = %d, want %d", got, want)
	}

	scratch.reset()
	cursors = cursors[:0]
	frame := syntheticRootReplayFrame{top: 1}
	cursors = scratch.append(cursors, frame, 10, Point{Row: 1, Column: 2})
	cursors = scratch.append(cursors, frame, 10, Point{Row: 1, Column: 3})
	if got, want := len(cursors), 2; got != want {
		t.Fatalf("distinct point cursor set length = %d, want %d", got, want)
	}
}

func TestSyntheticRootReplayGapLexMemoUsesPackedRecords(t *testing.T) {
	if got, want := unsafe.Sizeof(syntheticRootReplayGapToken{}), uintptr(16); got != want {
		t.Fatalf("gap token size = %d, want %d", got, want)
	}

	lang := newRootFrameReplayGapLanguage()
	parser := newRootFrameReplayParser(lang)
	source := []byte("\n\n")
	builder := newResultRootBuild(parser, source, nil, nil, nil, nil)
	initial := builder.syntheticRootReplayPush(syntheticRootReplayFrame{}, lang.InitialState)
	frame := builder.syntheticRootReplayPush(initial, 3)

	first, ok := builder.syntheticRootReplayLexGapToken(frame, 0, Point{}, 1)
	if !ok || first.symbol != 2 {
		t.Fatalf("first gap token = %+v, %v; want symbol 2, true", first, ok)
	}
	if got, want := len(builder.replayGapLexMemo), 1; got != want {
		t.Fatalf("first memo length = %d, want %d", got, want)
	}
	second, ok := builder.syntheticRootReplayLexGapToken(frame, 0, Point{}, 1)
	if !ok || second != first {
		t.Fatalf("cached gap token = %+v, %v; want %+v, true", second, ok, first)
	}
	if got, want := len(builder.replayGapLexMemo), 1; got != want {
		t.Fatalf("cached memo length = %d, want %d", got, want)
	}

	_, _ = builder.syntheticRootReplayLexGapToken(frame, 0, Point{}, 2)
	if got, want := len(builder.replayGapLexMemo), 2; got != want {
		t.Fatalf("end-bound memo length = %d, want %d", got, want)
	}
}

func TestSyntheticRootReplayAdvanceMemoUsesPackedStream(t *testing.T) {
	if got, want := unsafe.Sizeof(syntheticRootReplayAdvanceSpan(0)), uintptr(4); got != want {
		t.Fatalf("advance span size = %d, want %d", got, want)
	}
	packed := newSyntheticRootReplayAdvanceSpan(
		syntheticRootReplayAdvanceMaxPages-1,
		syntheticRootReplayAdvancePageFrames-syntheticRootReplayMaxFrontier,
		syntheticRootReplayMaxFrontier,
	)
	if got, want := packed.page(), syntheticRootReplayAdvanceMaxPages-1; got != want {
		t.Fatalf("packed advance page = %d, want %d", got, want)
	}
	if got, want := packed.offset(), syntheticRootReplayAdvancePageFrames-syntheticRootReplayMaxFrontier; got != want {
		t.Fatalf("packed advance offset = %d, want %d", got, want)
	}
	if got, want := packed.count(), syntheticRootReplayMaxFrontier; got != want {
		t.Fatalf("packed advance count = %d, want %d", got, want)
	}

	lang := newRootFrameReplayGapLanguage()
	parser := newRootFrameReplayParser(lang)
	builder := newResultRootBuild(parser, []byte("\n"), nil, nil, nil, nil)
	initial := builder.syntheticRootReplayPush(syntheticRootReplayFrame{}, lang.InitialState)
	frame := builder.syntheticRootReplayPush(initial, 3)
	var frameScratch [2][syntheticRootReplayMaxFrontier]syntheticRootReplayFrame
	var setScratch [2]syntheticRootReplayFrameSetScratch

	first := builder.syntheticRootReplayAdvanceToken(
		[]syntheticRootReplayFrame{frame},
		2,
		frameScratch[0][:0],
		frameScratch[1][:0],
		&setScratch[0],
		&setScratch[1],
	)
	if len(first) == 0 {
		t.Fatal("first packed transition returned no frames")
	}
	wantTops := make([]uint32, len(first))
	for i := range first {
		wantTops[i] = first[i].top
	}
	if got, want := len(builder.replayAdvanceMemo), 1; got != want {
		t.Fatalf("advance memo length = %d, want %d", got, want)
	}
	if got, want := int(builder.replayAdvanceFrames), len(first); got != want {
		t.Fatalf("advance stream frame count = %d, want %d", got, want)
	}
	pageCount := len(builder.replayAdvancePages)

	second := builder.syntheticRootReplayAdvanceToken(
		[]syntheticRootReplayFrame{frame},
		2,
		frameScratch[0][:0],
		frameScratch[1][:0],
		&setScratch[0],
		&setScratch[1],
	)
	if got, want := len(second), len(wantTops); got != want {
		t.Fatalf("cached transition length = %d, want %d", got, want)
	}
	for i, want := range wantTops {
		if got := second[i].top; got != want {
			t.Fatalf("cached transition top %d = %d, want %d", i, got, want)
		}
	}
	if got, want := int(builder.replayAdvanceFrames), len(first); got != want {
		t.Fatalf("cached advance stream frame count = %d, want %d", got, want)
	}
	if got := len(builder.replayAdvancePages); got != pageCount {
		t.Fatalf("cached advance page count = %d, want %d", got, pageCount)
	}
}

func TestSyntheticRootReplayCloseMemoPreservesCanonicalClosure(t *testing.T) {
	if got, want := unsafe.Sizeof(syntheticRootReplayCloseSpan(0)), uintptr(4); got != want {
		t.Fatalf("close span size = %d, want %d", got, want)
	}
	packed := newSyntheticRootReplayCloseSpan(
		syntheticRootReplayCloseMaxPages-1,
		syntheticRootReplayClosePageFrames-syntheticRootReplayMaxFrontier,
		syntheticRootReplayMaxFrontier,
	)
	if got, want := packed.page(), syntheticRootReplayCloseMaxPages-1; got != want {
		t.Fatalf("packed close page = %d, want %d", got, want)
	}
	if got, want := packed.offset(), syntheticRootReplayClosePageFrames-syntheticRootReplayMaxFrontier; got != want {
		t.Fatalf("packed close offset = %d, want %d", got, want)
	}
	if got, want := packed.count(), syntheticRootReplayMaxFrontier; got != want {
		t.Fatalf("packed close count = %d, want %d", got, want)
	}

	lang := newRootFrameReplayReduceLanguage()
	parser := newRootFrameReplayParser(lang)
	builder := newResultRootBuild(parser, nil, nil, nil, nil, nil)
	initial := builder.syntheticRootReplayPush(syntheticRootReplayFrame{}, lang.InitialState)
	shifted := builder.syntheticRootReplayPush(initial, 3)

	first := builder.syntheticRootReplayCloseLookahead([]syntheticRootReplayFrame{shifted}, 0)
	if got, want := len(first), 3; got != want {
		t.Fatalf("first EOF closure length = %d, want %d", got, want)
	}
	if cap(first) != len(first) {
		t.Fatalf("cached EOF closure capacity = %d, want length %d", cap(first), len(first))
	}
	wantTops := make([]uint32, len(first))
	for i := range first {
		wantTops[i] = first[i].top
	}
	if got, want := len(builder.replayStack.closeMemo), 1; got != want {
		t.Fatalf("close memo length = %d, want %d", got, want)
	}
	if got, want := int(builder.replayStack.closeFrames), len(first); got != want {
		t.Fatalf("close stream frame count = %d, want %d", got, want)
	}
	pageCount := len(builder.replayStack.closePages)
	nodesAfterFirst := len(builder.replayStack.nodes)

	appended := append(first, initial)
	if got, want := len(appended), len(first)+1; got != want {
		t.Fatalf("appended closure length = %d, want %d", got, want)
	}
	second := builder.syntheticRootReplayCloseLookahead([]syntheticRootReplayFrame{shifted}, 0)
	if got, want := len(second), len(wantTops); got != want {
		t.Fatalf("memoized EOF closure length = %d, want %d", got, want)
	}
	if got := len(builder.replayStack.nodes); got != nodesAfterFirst {
		t.Fatalf("memoized EOF closure added stack nodes: got %d, want %d", got, nodesAfterFirst)
	}
	if got, want := int(builder.replayStack.closeFrames), len(first); got != want {
		t.Fatalf("memoized close stream frame count = %d, want %d", got, want)
	}
	if got := len(builder.replayStack.closePages); got != pageCount {
		t.Fatalf("memoized close page count = %d, want %d", got, pageCount)
	}
	for i, want := range wantTops {
		if got := second[i].top; got != want {
			t.Fatalf("memoized EOF closure top %d = %d, want %d", i, got, want)
		}
	}
}

func TestExpectedRootCanFrameLongRepeatWithCanonicalReplayStacks(t *testing.T) {
	const childCount = 4096
	lang := newRootFrameReplayReduceLanguage()
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	children := make([]*Node, childCount)
	for i := range children {
		children[i] = newLeafNodeInArena(arena, 3, true, uint32(i), uint32(i+1), Point{Column: uint32(i)}, Point{Column: uint32(i + 1)})
	}
	builder := newResultRootBuild(parser, nil, arena, nil, nil, nil)
	if !builder.expectedRootCanFrameRecoveredFragments(children) {
		t.Fatal("long repeated root did not reach an accepting replay frame")
	}
	if got, wantMax := len(builder.replayStack.nodes), 8; got > wantMax {
		t.Fatalf("canonical replay stack nodes = %d, want <= %d for repeated state paths", got, wantMax)
	}
}

func BenchmarkExpectedRootCanFrameLongRepeat(b *testing.B) {
	const childCount = 4096
	lang := newRootFrameReplayReduceLanguage()
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	children := make([]*Node, childCount)
	for i := range children {
		children[i] = newLeafNodeInArena(arena, 3, true, uint32(i), uint32(i+1), Point{Column: uint32(i)}, Point{Column: uint32(i + 1)})
	}
	b.ReportAllocs()
	b.SetBytes(childCount)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder := newResultRootBuild(parser, nil, arena, nil, nil, nil)
		if !builder.expectedRootCanFrameRecoveredFragments(children) {
			b.Fatal("long repeated root did not reach an accepting replay frame")
		}
	}
}

func TestFinalizeReturnedTreeRootSpanExtendsAcceptedCleanTailOnlyOnRoot(t *testing.T) {
	lang := &Language{
		Name:        "root_tail",
		SymbolNames: []string{"EOF", "source_file", "statement"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "statement", Visible: true, Named: true},
		},
	}
	source := []byte("abc\n")
	child := NewLeafNode(2, true, 0, 3, Point{}, Point{Column: 3})
	root := newParentNode(nil, 1, true, []*Node{child}, nil, 0)
	tree := NewTree(root, source, lang)
	t.Cleanup(tree.Release)
	tree.setParseRuntime(ParseRuntime{
		StopReason:      ParseStopAccepted,
		SourceLen:       uint32(len(source)),
		ExpectedEOFByte: uint32(len(source)),
	})

	finalizeReturnedTreeRootSpan(tree, source)

	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	if got, want := root.EndPoint(), (Point{Row: 1, Column: 0}); got != want {
		t.Fatalf("root end point = %+v, want %+v", got, want)
	}
	if got, want := child.EndByte(), uint32(3); got != want {
		t.Fatalf("child end = %d, want %d", got, want)
	}
	if got := tree.ParseRuntime().RootEndByte; got != uint32(len(source)) {
		t.Fatalf("runtime root end = %d, want %d", got, len(source))
	}
	if tree.ParseRuntime().Truncated {
		t.Fatal("runtime should not report truncation after clean accepted tail extension")
	}
}

func TestFinalizeReturnedTreeRootSpanPreservesIncludedRangeEOF(t *testing.T) {
	lang := &Language{
		Name:        "root_tail_included",
		SymbolNames: []string{"EOF", "source_file", "statement"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "statement", Visible: true, Named: true},
		},
	}
	source := []byte("abc\noutside")
	child := NewLeafNode(2, true, 0, 3, Point{}, Point{Column: 3})
	root := newParentNode(nil, 1, true, []*Node{child}, nil, 0)
	tree := NewTree(root, source, lang)
	t.Cleanup(tree.Release)
	tree.setIncludedRanges([]Range{{StartByte: 0, EndByte: 4}})
	tree.setParseRuntime(ParseRuntime{
		StopReason:      ParseStopAccepted,
		SourceLen:       uint32(len(source)),
		ExpectedEOFByte: 4,
	})

	finalizeReturnedTreeRootSpan(tree, source)

	if got, want := root.EndByte(), uint32(4); got != want {
		t.Fatalf("root end = %d, want included EOF %d", got, want)
	}
	if got, want := child.EndByte(), uint32(3); got != want {
		t.Fatalf("child end = %d, want %d", got, want)
	}
}

func TestFinalizeReturnedTreeRootSpanDoesNotExtendNonCleanTail(t *testing.T) {
	lang := &Language{
		Name:        "root_tail_dirty",
		SymbolNames: []string{"EOF", "source_file", "statement"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "statement", Visible: true, Named: true},
		},
	}
	source := []byte("abcx")
	child := NewLeafNode(2, true, 0, 3, Point{}, Point{Column: 3})
	root := newParentNode(nil, 1, true, []*Node{child}, nil, 0)
	tree := NewTree(root, source, lang)
	t.Cleanup(tree.Release)
	tree.setParseRuntime(ParseRuntime{
		StopReason:      ParseStopAccepted,
		SourceLen:       uint32(len(source)),
		ExpectedEOFByte: uint32(len(source)),
	})

	finalizeReturnedTreeRootSpan(tree, source)

	if got, want := root.EndByte(), uint32(3); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	if !tree.ParseRuntime().Truncated {
		t.Fatal("runtime should still report truncation for non-clean tail")
	}
	// wave2b SILENT-TRUNCATION contract: a truncated non-clean tail must carry a
	// consumer-visible error signal (root.HasError()), not just the internal
	// Truncated flag. See markTruncatedTreeHasError.
	if !root.HasError() {
		t.Fatal("truncated non-clean tail must report root.HasError() (silent-truncation contract)")
	}
}

// TestFinalizeReturnedTreeRootSpanFlagsSilentTruncation pins the wave2b fix: a
// non-early-stop parse (ParseStopNoStacksAlive) whose live stack reduced to a
// clean start symbol before the frontier died mid-file returns a prefix-only
// root with hasError=false. finalizeReturnedTreeRootSpan must mark it HasError so
// the returned tree is never a silent clean-looking prefix (crystal string.cr,
// typescript checker.ts / dom.generated.d.ts class), while leaving it honestly
// truncated (span unchanged, Truncated=true).
func TestFinalizeReturnedTreeRootSpanFlagsSilentTruncation(t *testing.T) {
	lang := &Language{
		Name:        "root_silent_trunc",
		SymbolNames: []string{"EOF", "source_file", "statement"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "statement", Visible: true, Named: true},
		},
	}
	// A cleanly-reduced source_file prefix [0,49) over a 100-byte input: the
	// dropped tail [49,100) is real (non-trivia) source.
	source := make([]byte, 100)
	for i := range source {
		source[i] = 'a'
	}
	child := NewLeafNode(2, true, 0, 49, Point{}, Point{Column: 49})
	root := newParentNode(nil, 1, true, []*Node{child}, nil, 0)
	tree := NewTree(root, source, lang)
	t.Cleanup(tree.Release)
	tree.setParseRuntime(ParseRuntime{
		StopReason:      ParseStopNoStacksAlive,
		SourceLen:       uint32(len(source)),
		ExpectedEOFByte: uint32(len(source)),
	})
	if root.HasError() {
		t.Fatal("precondition: synthetic clean prefix must start without an error")
	}

	finalizeReturnedTreeRootSpan(tree, source)

	if root.HasError() != true {
		t.Fatal("silent truncation must be flagged HasError after finalize")
	}
	if !tree.ParseRuntime().Truncated {
		t.Fatal("tree must remain honestly truncated (Truncated=true)")
	}
	if got, want := root.EndByte(), uint32(49); got != want {
		t.Fatalf("root span must be left truncated at %d, got %d (do not silently extend)", want, got)
	}
}

func TestFinalizeForestRootReextendsAcceptedCleanTailAfterCompatibility(t *testing.T) {
	lang := &Language{
		Name:        "hcl",
		SymbolNames: []string{"EOF", "config_file", "comment", "_whitespace", "body"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "config_file", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "_whitespace", Visible: false, Named: false},
			{Name: "body", Visible: true, Named: true},
		},
	}
	parser := NewParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("body\n")

	body := newLeafNodeInArena(arena, 4, true, 0, 4, Point{}, Point{Column: 4})
	ws := newLeafNodeInArena(arena, 3, false, 4, 5, Point{Column: 4}, Point{Row: 1})
	root := newParentNodeInArena(arena, 1, true, []*Node{body, ws}, nil, 0)

	parser.finalizeForestRoot(root, source, false)

	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want accepted EOF %d", got, want)
	}
	if got, want := root.EndPoint(), (Point{Row: 1, Column: 0}); got != want {
		t.Fatalf("root end point = %+v, want %+v", got, want)
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got, want := root.Child(0).Type(lang), "body"; got != want {
		t.Fatalf("root child type = %q, want %q", got, want)
	}
	if got, want := root.Child(0).EndByte(), uint32(4); got != want {
		t.Fatalf("root child end = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesFlattensInvisibleRootChildren(t *testing.T) {
	lang := &Language{
		Name: "test",
		SymbolNames: []string{
			"",
			"source_file",
			"function_declaration",
			"source_file_repeat1",
		},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: false, Named: false},
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("a\nb\nc\n")

	fn0 := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	fn1 := newLeafNodeInArena(arena, 2, true, 2, 3, Point{Row: 1}, Point{Row: 1, Column: 1})
	fn2 := newLeafNodeInArena(arena, 2, true, 4, 5, Point{Row: 2}, Point{Row: 2, Column: 1})

	repeat1a := newParentNodeInArena(arena, 3, false, []*Node{fn1}, nil, 0)
	repeat1a.endByte = 4
	repeat1a.endPoint = Point{Row: 1, Column: 2}
	repeat1b := newParentNodeInArena(arena, 3, false, []*Node{fn2}, nil, 0)
	repeat1b.endByte = 6
	repeat1b.endPoint = Point{Row: 2, Column: 2}

	root := newParentNodeInArena(arena, 1, true, []*Node{fn0, repeat1a, repeat1b}, nil, 0)
	tree := parser.buildResultFromNodes([]*Node{root}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	gotRoot := tree.RootNode()
	if gotRoot == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := gotRoot.ChildCount(), 3; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	for i := 0; i < gotRoot.ChildCount(); i++ {
		child := gotRoot.Child(i)
		if child == nil {
			t.Fatalf("root child %d is nil", i)
		}
		if got, want := child.Type(lang), "function_declaration"; got != want {
			t.Fatalf("root child %d type = %q, want %q", i, got, want)
		}
		if !child.IsNamed() {
			t.Fatalf("root child %d should be named after flattening", i)
		}
	}
	if got, want := gotRoot.Child(1).EndByte(), uint32(3); got != want {
		t.Fatalf("second child end = %d, want %d", got, want)
	}
	if got, want := gotRoot.Child(2).EndByte(), uint32(5); got != want {
		t.Fatalf("third child end = %d, want %d", got, want)
	}
	if got, want := gotRoot.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesRecursivelyFlattensInvisibleRootChildren(t *testing.T) {
	lang := &Language{
		Name: "test",
		SymbolNames: []string{
			"",
			"source_file",
			"function_declaration",
			"source_file_repeat1",
			"_named_wrapper",
		},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "function_declaration", Visible: true, Named: true},
			{Name: "source_file_repeat1", Visible: false, Named: false, GeneratedRepeatAux: true},
			{Name: "_named_wrapper", Visible: false, Named: true},
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("a\nb\nc\n")

	fn0 := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	fn1 := newLeafNodeInArena(arena, 2, true, 2, 3, Point{Row: 1}, Point{Row: 1, Column: 1})
	fn2 := newLeafNodeInArena(arena, 2, true, 4, 5, Point{Row: 2}, Point{Row: 2, Column: 1})

	innerRepeat := newParentNodeInArena(arena, 3, false, []*Node{fn1}, nil, 0)
	namedWrapper := newParentNodeInArena(arena, 4, true, []*Node{innerRepeat}, nil, 0)
	outerRepeat := newParentNodeInArena(arena, 3, false, []*Node{namedWrapper, fn2}, nil, 0)

	root := newParentNodeInArena(arena, 1, true, []*Node{fn0, outerRepeat}, nil, 0)
	tree := parser.buildResultFromNodes([]*Node{root}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	gotRoot := tree.RootNode()
	if gotRoot == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := gotRoot.ChildCount(), 3; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	for i := 0; i < gotRoot.ChildCount(); i++ {
		child := gotRoot.Child(i)
		if child == nil {
			t.Fatalf("root child %d is nil", i)
		}
		if got, want := child.Type(lang), "function_declaration"; got != want {
			t.Fatalf("root child %d type = %q, want %q", i, got, want)
		}
		if got := child.Parent(); got != gotRoot {
			t.Fatalf("root child %d parent = %p, want root %p", i, got, gotRoot)
		}
		if got, want := child.childIndex, int32(i); got != want {
			t.Fatalf("root child %d childIndex = %d, want %d", i, got, want)
		}
	}
}

func TestBuildResultFromNodesPreservesVisibleRootContainerNestedInHiddenRepeats(t *testing.T) {
	lang := &Language{
		Name: "test",
		SymbolNames: []string{
			"",
			"source_file",
			"source_file_repeat1",
			"_section_wrapper",
			"section",
			"entry",
		},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "source_file_repeat1", Visible: false, Named: false, GeneratedRepeatAux: true},
			{Name: "_section_wrapper", Visible: false, Named: true},
			{Name: "section", Visible: true, Named: true},
			{Name: "entry", Visible: true, Named: true},
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("a\nb\n")

	entry0 := newLeafNodeInArena(arena, 5, true, 0, 1, Point{}, Point{Column: 1})
	entry1 := newLeafNodeInArena(arena, 5, true, 2, 3, Point{Row: 1}, Point{Row: 1, Column: 1})
	section := newParentNodeInArena(arena, 4, true, []*Node{entry0, entry1}, nil, 0)
	namedHiddenWrapper := newParentNodeInArena(arena, 3, true, []*Node{section}, nil, 0)
	innerRepeat := newParentNodeInArena(arena, 2, false, []*Node{namedHiddenWrapper}, nil, 0)
	outerRepeat := newParentNodeInArena(arena, 2, false, []*Node{innerRepeat}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{outerRepeat}, nil, 0)

	tree := parser.buildResultFromNodes([]*Node{root}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	gotRoot := tree.RootNode()
	if gotRoot == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := gotRoot.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	gotSection := gotRoot.Child(0)
	if gotSection == nil {
		t.Fatal("root child is nil")
	}
	if got, want := gotSection.Type(lang), "section"; got != want {
		t.Fatalf("root child type = %q, want %q", got, want)
	}
	if got := gotSection.Parent(); got != gotRoot {
		t.Fatalf("section parent = %p, want root %p", got, gotRoot)
	}
	if got, want := gotSection.childIndex, int32(0); got != want {
		t.Fatalf("section childIndex = %d, want %d", got, want)
	}
	if got, want := gotSection.ChildCount(), 2; got != want {
		t.Fatalf("section child count = %d, want %d", got, want)
	}
	for i := 0; i < gotSection.ChildCount(); i++ {
		child := gotSection.Child(i)
		if child == nil {
			t.Fatalf("section child %d is nil", i)
		}
		if got, want := child.Type(lang), "entry"; got != want {
			t.Fatalf("section child %d type = %q, want %q", i, got, want)
		}
		if got := child.Parent(); got != gotSection {
			t.Fatalf("section child %d parent = %p, want section %p", i, got, gotSection)
		}
		if got, want := child.childIndex, int32(i); got != want {
			t.Fatalf("section child %d childIndex = %d, want %d", i, got, want)
		}
	}
}

func TestBuildResultFromNodesKeepsWrappedSingleChildSpan(t *testing.T) {
	lang := &Language{
		Name:        "expected_root_wrapper",
		SymbolNames: []string{"", "item", "root"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    2,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("x\n")

	item := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	tree := parser.buildResultFromNodes([]*Node{item}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Symbol(), Symbol(2); got != want {
		t.Fatalf("root symbol = %d, want %d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	child := root.Child(0)
	if child == nil {
		t.Fatal("root child is nil")
	}
	if got, want := child.EndByte(), uint32(1); got != want {
		t.Fatalf("wrapped child end = %d, want %d", got, want)
	}
	if got, want := child.Text(tree.Source()), "x"; got != want {
		t.Fatalf("wrapped child text = %q, want %q", got, want)
	}
}

func TestBuildResultFromNodesWidensSingleRootToExtraChildSpan(t *testing.T) {
	lang := &Language{
		Name:        "extra_root_span",
		SymbolNames: []string{"", "source_file", "comment"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("// one \\\n   two")

	comment := newLeafNodeInArena(arena, 2, true, 0, uint32(len(source)), Point{}, Point{Row: 1, Column: 6})
	comment.setExtra(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{comment}, nil, 0)
	root.startByte = 0
	root.endByte = 0
	root.startPoint = Point{}
	root.endPoint = Point{}

	tree := parser.buildResultFromNodes([]*Node{root}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	gotRoot := tree.RootNode()
	if gotRoot == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := gotRoot.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	if got, want := gotRoot.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	child := gotRoot.Child(0)
	if child == nil {
		t.Fatal("root child is nil")
	}
	if !child.IsExtra() {
		t.Fatal("root child should remain extra")
	}
	if got, want := child.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("comment end = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesKeepsSQLSourceFileRootWhenChildrenHaveErrors(t *testing.T) {
	lang := newRootFrameReplayLanguage("sql", "source_file", "select_statement", true)
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("SELECT a,\n")

	stmt := newLeafNodeInArena(arena, 3, true, 0, 8, Point{}, Point{Column: 8})
	errNode := newLeafNodeInArena(arena, errorSymbol, true, 8, 9, Point{Column: 8}, Point{Column: 9})
	errNode.setHasError(true)
	errNode.setExtra(true)

	tree := parser.buildResultFromNodes([]*Node{stmt, errNode}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatal("expected source_file root to retain HasError=true")
	}
}

func TestBuildResultFromNodesKeepsGoSourceFileRootWhenChildrenHaveErrors(t *testing.T) {
	lang := newRootFrameReplayLanguage("go", "source_file", "package_clause", true)
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("package main\nfunc broken")

	pkg := newLeafNodeInArena(arena, 3, true, 0, 12, Point{}, Point{Column: 12})
	errNode := newLeafNodeInArena(arena, errorSymbol, true, 13, uint32(len(source)), Point{Row: 1}, Point{Row: 1, Column: 11})
	errNode.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{pkg, errNode}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatal("expected source_file root to retain HasError=true")
	}
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got, want := root.Child(0).Type(lang), "package_clause"; got != want {
		t.Fatalf("root child 0 type = %q, want %q", got, want)
	}
	if got, want := root.Child(1).Type(lang), "ERROR"; got != want {
		t.Fatalf("root child 1 type = %q, want %q", got, want)
	}
}

func TestBuildResultFromNodesKeepsSwiftSourceFileRootWhenChildrenHaveErrors(t *testing.T) {
	lang := newRootFrameReplayLanguage("swift", "source_file", "class_declaration", true)
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("class A {}\nextra\n")

	classDecl := newLeafNodeInArena(arena, 3, true, 0, 10, Point{}, Point{Column: 10})
	errNode := newLeafNodeInArena(arena, errorSymbol, true, 11, 16, Point{Row: 1}, Point{Row: 1, Column: 5})
	errNode.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{classDecl, errNode}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatal("expected source_file root to retain HasError=true")
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesKeepsGomodSourceFileRootWhenChildrenHaveErrors(t *testing.T) {
	lang := &Language{
		Name: "gomod",
		SymbolNames: []string{
			"EOF",
			"source_file",
			"module_directive",
			"ERROR",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "module_directive", Visible: true, Named: true},
			{Name: "ERROR", Visible: true, Named: true},
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("module example.com/m\nunknown value\n")

	module := newLeafNodeInArena(arena, 2, true, 0, 21, Point{}, Point{Row: 1})
	errNode := newLeafNodeInArena(arena, errorSymbol, true, 21, uint32(len(source)), Point{Row: 1}, Point{Row: 2})
	errNode.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{module, errNode}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatal("expected source_file root to retain HasError=true")
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesFramesSingleErrorRootChildrenThroughSourceGapToken(t *testing.T) {
	lang := newRootFrameReplayGapLanguage()
	parser := NewParser(lang)
	parser.rootSymbol = 3
	parser.hasRootSymbol = true
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("decl\ndecl\n")

	first := newLeafNodeInArena(arena, 4, true, 0, 4, Point{}, Point{Column: 4})
	second := newLeafNodeInArena(arena, 4, true, 5, 9, Point{Row: 1}, Point{Row: 1, Column: 4})
	errRoot := newParentNodeInArena(arena, errorSymbol, true, []*Node{first, second}, nil, 0)
	errRoot.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{errRoot}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatal("expected framed root to retain HasError=true")
	}
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesFramesSingleErrorRootChildrenThroughSkippedExtraSourceGap(t *testing.T) {
	lang := newRootFrameReplayGapLanguage()
	parser := NewParser(lang)
	parser.rootSymbol = 3
	parser.hasRootSymbol = true
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("decl\n# comment\ndecl\n")

	first := newLeafNodeInArena(arena, 4, true, 0, 4, Point{}, Point{Column: 4})
	comment := newLeafNodeInArena(arena, 6, true, 5, 14, Point{Row: 1}, Point{Row: 1, Column: 9})
	comment.setExtra(true)
	second := newLeafNodeInArena(arena, 4, true, 15, 19, Point{Row: 2}, Point{Row: 2, Column: 4})
	errRoot := newParentNodeInArena(arena, errorSymbol, true, []*Node{first, comment, second}, nil, 0)
	errRoot.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{errRoot}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatal("expected framed root to retain HasError=true")
	}
	if got, want := root.ChildCount(), 3; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got := root.Child(1); got == nil || !got.IsExtra() {
		t.Fatalf("root child 1 extra = %v, want true", got != nil && got.IsExtra())
	}
}

func TestBuildResultFromNodesFramesSingleErrorRootChildrenThroughEOFReductions(t *testing.T) {
	lang := newRootFrameReplayReduceLanguage()
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("decl\ndecl\n")

	first := newLeafNodeInArena(arena, 3, true, 0, 4, Point{}, Point{Column: 4})
	second := newLeafNodeInArena(arena, 3, true, 5, 9, Point{Row: 1}, Point{Row: 1, Column: 4})
	errRoot := newParentNodeInArena(arena, errorSymbol, true, []*Node{first, second}, nil, 0)
	errRoot.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{errRoot}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatal("expected framed root to retain HasError=true")
	}
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesFramesSingleErrorRootChildrenWithExpectedRoot(t *testing.T) {
	lang := newRootFrameReplayLanguage("generic", "source_file", "declaration", true)
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("#\ndecl\ndecl\n")

	extra := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	extra.setExtra(true)
	first := newLeafNodeInArena(arena, 3, true, 2, 6, Point{Row: 1}, Point{Row: 1, Column: 4})
	second := newLeafNodeInArena(arena, 3, true, 7, 11, Point{Row: 2}, Point{Row: 2, Column: 4})
	errRoot := newParentNodeInArena(arena, errorSymbol, true, []*Node{extra, first, second}, nil, 0)
	errRoot.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{errRoot}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatal("expected framed root to retain HasError=true")
	}
	if got, want := root.ChildCount(), 3; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got := root.Child(0); got == nil || !got.IsExtra() {
		t.Fatalf("root child 0 extra = %v, want true", got != nil && got.IsExtra())
	}
}

func TestBuildResultFromNodesKeepsExpectedRootWhenReplayFramesRecoveredFragment(t *testing.T) {
	lang := newRootFrameReplayLanguage("generic", "source_file", "declaration", true)
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("decl\n???")

	decl := newLeafNodeInArena(arena, 3, true, 0, 4, Point{}, Point{Column: 4})
	errNode := newLeafNodeInArena(arena, errorSymbol, true, 5, 8, Point{Row: 1}, Point{Row: 1, Column: 3})
	errNode.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{decl, errNode}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatal("expected source_file root to retain HasError=true")
	}
}

func TestBuildResultFromNodesFallsBackToErrorRootWithoutReplayTable(t *testing.T) {
	lang := &Language{
		Name:           "go",
		SymbolNames:    []string{"EOF", "source_file", "declaration"},
		SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "source_file", Visible: true, Named: true}, {Name: "declaration", Visible: true, Named: true}},
	}
	parser := &Parser{language: lang, rootSymbol: 1, hasRootSymbol: true}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("decl\n???")

	decl := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	errNode := newLeafNodeInArena(arena, errorSymbol, true, 5, 8, Point{Row: 1}, Point{Row: 1, Column: 3})
	errNode.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{decl, errNode}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got := root.Symbol(); got != errorSymbol {
		t.Fatalf("root symbol = %d, want ERROR", got)
	}
}

// TestBuildResultFromNodesPublishesMarkedCRecoverEOFRootUnwrapped covers the
// doxygen witness from pine's diagnosis: tree-sitter C's recover_eof wraps
// the whole remaining stack in one childless ERROR root spanning the entire
// source and publishes it as-is (ts_parser__accept), never nested under the
// grammar's expected root. The Go port must match once cRecoverEOFAccept has
// marked the root with nodeFlagCompactRecoverEOF.
//
// The language is named "corn" (a real CRecoverEOFBareRootReceipted entry),
// not because it uses corn's actual grammar tables — it is still the
// synthetic minimal replay language — but because tryPublishCRecoverEOFRoot
// gates on the receipted grammar set (review round-2 finding B-A), and this
// test's job is to exercise the marker mechanism, not the receipt table
// itself (see TestCRecoverEOFBareRootReceiptedGatesPublish for that).
func TestBuildResultFromNodesPublishesMarkedCRecoverEOFRootUnwrapped(t *testing.T) {
	lang := newRootFrameReplayLanguage("corn", "document", "value", false)
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	source := []byte(`/** Adds all words in \a s to document \a doc with weight \a wfd */`)

	errNode := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
	cSetNodeSpan(errNode, 0, uint32(len(source)), Point{}, Point{Column: uint32(len(source))})
	errNode.setHasError(true)
	errNode.setFlag(nodeFlagCompactRecoverEOF, true)

	tree := parser.buildResultFromNodes([]*Node{errNode}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got := root.Symbol(); got != errorSymbol {
		t.Fatalf("root symbol = %d, want ERROR", got)
	}
	if got := root.ChildCount(); got != 0 {
		t.Fatalf("root child count = %d, want 0", got)
	}
	if !root.HasError() {
		t.Fatal("root HasError() = false, want true")
	}
	if root.StartByte() != 0 || root.EndByte() != uint32(len(source)) {
		t.Fatalf("root span = %d..%d, want 0..%d", root.StartByte(), root.EndByte(), len(source))
	}
}

// TestBuildResultFromNodesWrapsUnmarkedZeroChildErrorRoot guards the "non-C
// recovery path keeps its current wrapper behavior" requirement: an
// otherwise identical zero-child ERROR root that never went through
// cRecoverEOFAccept (no nodeFlagCompactRecoverEOF marker) must still be
// wrapped under the grammar's expected root, unchanged from today.
func TestBuildResultFromNodesWrapsUnmarkedZeroChildErrorRoot(t *testing.T) {
	lang := newRootFrameReplayLanguage("recover_eof_probe_unmarked", "document", "value", false)
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	source := []byte(`/** Adds all words in \a s to document \a doc with weight \a wfd */`)

	errNode := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
	cSetNodeSpan(errNode, 0, uint32(len(source)), Point{}, Point{Column: uint32(len(source))})
	errNode.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{errNode}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got := root.Symbol(); got != parser.rootSymbol {
		t.Fatalf("root symbol = %d, want expected root %d", got, parser.rootSymbol)
	}
	if got := root.ChildCount(); got != 1 {
		t.Fatalf("root child count = %d, want 1 (wrapped ERROR child)", got)
	}
	if got := root.Child(0).Symbol(); got != errorSymbol {
		t.Fatalf("root child symbol = %d, want ERROR", got)
	}
}

// TestCRecoverEOFBareRootReceiptedGatesPublish is review round-2 finding
// B-A's unit-level receipt gate check: an otherwise identical marked,
// childless, whole-source ERROR root is published bare only when
// CRecoverEOFBareRootReceipted names the language, and stays wrapped when
// it does not — for example c_sharp, whose recover_eof route diverges from
// C's own recovery (cgo_harness/parity_recover_eof_publish_sweep_test.go,
// TestParityRecoverEOFRouteDivergences).
func TestCRecoverEOFBareRootReceiptedGatesPublish(t *testing.T) {
	cases := []struct {
		name         string
		languageName string
		wantBare     bool
	}{
		{name: "receipted grammar publishes bare", languageName: "corn", wantBare: true},
		{name: "c_sharp stays wrapped", languageName: "c_sharp", wantBare: false},
		{name: "unreceipted grammar stays wrapped", languageName: "not_a_receipted_grammar", wantBare: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CRecoverEOFBareRootReceipted(tc.languageName); got != tc.wantBare {
				t.Fatalf("CRecoverEOFBareRootReceipted(%q) = %v, want %v (sanity check before parsing)", tc.languageName, got, tc.wantBare)
			}

			lang := newRootFrameReplayLanguage(tc.languageName, "document", "value", false)
			parser := newRootFrameReplayParser(lang)
			arena := acquireNodeArena(arenaClassFull)
			source := []byte("broken")

			errNode := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
			cSetNodeSpan(errNode, 0, uint32(len(source)), Point{}, Point{Column: uint32(len(source))})
			errNode.setHasError(true)
			errNode.setFlag(nodeFlagCompactRecoverEOF, true)

			tree := parser.buildResultFromNodes([]*Node{errNode}, source, arena, nil, nil, nil)
			t.Cleanup(tree.Release)

			root := tree.RootNode()
			if root == nil {
				t.Fatal("buildResultFromNodes returned nil root")
			}
			gotBare := root.Symbol() == errorSymbol && root.ChildCount() == 0
			if gotBare != tc.wantBare {
				t.Fatalf("published bare = %v, want %v (root symbol=%d children=%d)", gotBare, tc.wantBare, root.Symbol(), root.ChildCount())
			}
		})
	}
}

func TestBuildResultFromNodesRejectsSingleValueRootWithMultipleFragments(t *testing.T) {
	lang := newRootFrameReplayLanguage("single_value", "document", "value", false)
	parser := newRootFrameReplayParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("a b ?")

	first := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	second := newLeafNodeInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	errNode := newLeafNodeInArena(arena, errorSymbol, true, 4, 5, Point{Column: 4}, Point{Column: 5})
	errNode.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{first, second, errNode}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if got := root.Symbol(); got != errorSymbol {
		t.Fatalf("root symbol = %d, want ERROR", got)
	}
}

func TestAttachResultRootExtraSplitReportsUnfoldedErrorExtras(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "source_file", "comment"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	root := newParentNodeInArena(arena, 1, true, nil, nil, 0)
	leading := newParentNodeInArena(arena, errorSymbol, true, []*Node{newLeafNodeInArena(arena, 2, true, 0, 2, Point{}, Point{Column: 2})}, nil, 0)
	leading.setExtra(true)
	leading.setHasError(true)
	comment := newLeafNodeInArena(arena, 2, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	comment.setExtra(true)

	split := classifyResultRootExtras([]*Node{leading, comment}, lang)
	attachResultRootExtraSplit(root, split, arena)

	if !root.HasError() {
		t.Fatal("root HasError=false, want true when an ERROR extra is left beside it")
	}
	if got := len(resultChildSliceForMutation(root)); got != 0 {
		t.Fatalf("root children=%d, want 0: extras are not folded while one carries an error", got)
	}
	if root.startByte != 0 || root.endByte != 4 {
		t.Fatalf("root span=%d..%d, want 0..4", root.startByte, root.endByte)
	}

	clean := newParentNodeInArena(arena, 1, true, nil, nil, 0)
	attachResultRootExtraSplit(clean, classifyResultRootExtras([]*Node{comment}, lang), arena)
	if clean.HasError() {
		t.Fatal("root HasError=true for a clean extra")
	}
}

// The root's error flag must survive flattenInvisibleRootChildren, which
// rebuilds the children of a root with a hidden child and recomputes the
// flag from those children alone; the ERROR extra beside the root is not one.
func TestBuildResultFromNodesKeepsErrorExtraFlagThroughHiddenRootChild(t *testing.T) {
	lang := &Language{
		Name:        "test",
		SymbolNames: []string{"EOF", "source_file", "_wrapper", "word"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "_wrapper", Named: true},
			{Name: "word", Visible: true, Named: true},
		},
	}
	// Without a known root symbol the real root is not split into fragments.
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("ab")

	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{wrapper}, nil, 0)
	errorExtra := newParentNodeInArena(arena, errorSymbol, true, []*Node{newLeafNodeInArena(arena, 3, true, 1, 2, Point{Column: 1}, Point{Column: 2})}, nil, 0)
	errorExtra.setExtra(true)
	errorExtra.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{root, errorExtra}, source, arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromNodes returned nil tree/root")
	}
	defer tree.Release()
	got := tree.RootNode()
	if got.Type(lang) != "source_file" {
		t.Fatalf("root type = %q, want source_file (the real root)", got.Type(lang))
	}
	if got.EndByte() != 2 {
		t.Fatalf("root ends at %d, want 2 (it spans the ERROR extra)", got.EndByte())
	}
	if !got.HasError() {
		t.Fatalf("root HasError=false over an ERROR extra: %s", got.SExpr(lang))
	}
}

func TestFinalizeForestRootKeepsUnfoldedErrorExtra(t *testing.T) {
	lang := &Language{
		Name:        "test",
		SymbolNames: []string{"EOF", "source_file", "_wrapper", "word"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "_wrapper", Named: true},
			{Name: "word", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{wrapper}, nil, 0)
	errorExtra := newLeafNodeInArena(arena, errorSymbol, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	errorExtra.setExtra(true)
	errorExtra.setHasError(true)
	attachResultRootExtraSplit(root, classifyResultRootExtras([]*Node{errorExtra}, lang), arena)
	if !root.HasError() {
		t.Fatal("ERROR extra did not mark root before forest finalization")
	}

	parser.finalizeForestRoot(root, []byte("ab"), true)
	if !root.HasError() {
		t.Fatal("forest finalization cleared the unfolded ERROR extra flag")
	}
	parser.finalizeForestRoot(root, []byte("ab"), true)
	if !root.HasError() {
		t.Fatal("repeated forest finalization cleared the unfolded ERROR extra flag")
	}
	if root.EndByte() != 2 {
		t.Fatalf("root ends at %d, want 2", root.EndByte())
	}
	if resultChildCount(root) != 1 || resultChildAt(root, 0) != leaf {
		t.Fatal("forest finalization did not flatten the hidden wrapper to its visible child")
	}
}

func TestFinalizeForestRootDoesNotPreserveUnrelatedErrorFlag(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "EOF"}, {Name: "source_file", Visible: true, Named: true},
		{Name: "_wrapper", Named: true}, {Name: "word", Visible: true, Named: true},
	}}
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{wrapper}, nil, 0)
	root.setHasError(true)
	parser.finalizeForestRoot(root, []byte("a"), false)
	if root.HasError() {
		t.Fatal("forest finalization preserved an unrelated error flag")
	}
}

func TestForestCleanRescueDeclinesOmittedErrorExtra(t *testing.T) {
	lang := &Language{
		Name:        "cooklang",
		SymbolNames: []string{"EOF", "recipe", "_wrapper", "step", "metadata", "comment"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"}, {Name: "recipe", Visible: true, Named: true},
			{Name: "_wrapper", Named: true}, {Name: "step", Visible: true, Named: true},
			{Name: "metadata", Visible: true, Named: true}, {Name: "comment", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{wrapper}, nil, 0)
	extra := newLeafNodeInArena(arena, errorSymbol, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	extra.setExtra(true)
	extra.setHasError(true)
	split := classifyResultRootExtras([]*Node{extra}, lang)
	attachResultRootExtraSplit(root, split, arena)
	if !split.hasErrorExtras() || !root.HasError() {
		t.Fatal("fixture lacks omitted error extra")
	}
	if parser.finalizeExperimentalForestRoot(root, []byte("ab"), true, split.hasErrorExtras()) {
		// Model the clean-rescue caller: Cooklang's second compatibility pass
		// rebuilds the visible children and can erase the omitted-error flag.
		tree := newTreeWithArenas(root, []byte("ab"), lang, arena, nil)
		parser.normalizeReturnedTreeForParse(tree, []byte("ab"))
		if !root.HasError() {
			t.Fatal("clean rescue normalization erased the omitted error extra")
		}
		t.Fatal("clean rescue accepted a root with an omitted error extra")
	}
	if parser.forestDeclineReason != forestDeclineRootHasError {
		t.Fatalf("decline reason = %q, want %q", parser.forestDeclineReason, forestDeclineRootHasError)
	}
	if !parser.finalizeExperimentalForestRoot(root, []byte("ab"), false, split.hasErrorExtras()) || !root.HasError() {
		t.Fatal("ordinary experimental result did not retain its omitted error")
	}
}

func makeOmittedCobolFixture() (*Tree, *Parser, []byte) {
	lang := &Language{
		Name:                  "cobol",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"EOF", ".", "start", "program_definition", "identification_division", "procedure_division", "END_EVALUATE", "period", "paragraph_header"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "start", Visible: true, Named: true},
			{Name: "program_definition", Visible: true, Named: true},
			{Name: "identification_division", Visible: true, Named: true},
			{Name: "procedure_division", Visible: true, Named: true},
			{Name: "END_EVALUATE", Visible: true, Named: true},
			{Name: "period", Visible: true, Named: true},
			{Name: "paragraph_header", Visible: true, Named: true},
		},
	}
	source := []byte("       identification division.\n" +
		"       program-id. a.\n" +
		"       procedure division.\n" +
		"       evaluate 1\n" +
		"       when 1\n" +
		"         go to aa\n" +
		"       when 2\n" +
		"         go to aa\n" +
		"       when other\n" +
		"         go to aa\n" +
		"       end-evaluate.\n" +
		"       aa.\n")

	arena := newNodeArena(arenaClassFull)
	id := newLeafNodeInArena(arena, 4, true, 7, 53, advancePointByBytes(Point{}, source[:7]), advancePointByBytes(Point{}, source[:53]))
	endEvaluate := newLeafNodeInArena(arena, 6, true, 206, 218, advancePointByBytes(Point{}, source[:206]), advancePointByBytes(Point{}, source[:218]))
	period := newLeafNodeInArena(arena, 7, true, 218, 219, advancePointByBytes(Point{}, source[:218]), advancePointByBytes(Point{}, source[:219]))
	proc := newParentNodeInArena(arena, 5, true, []*Node{endEvaluate, period}, nil, 0)
	proc.startByte = 61
	proc.startPoint = advancePointByBytes(Point{}, source[:61])
	proc.endByte = 230
	proc.endPoint = advancePointByBytes(Point{}, source[:230])
	def := newParentNodeInArena(arena, 3, true, []*Node{id, proc}, nil, 0)
	def.startByte = 7
	def.startPoint = advancePointByBytes(Point{}, source[:7])
	def.endByte = 230
	def.endPoint = advancePointByBytes(Point{}, source[:230])
	errDot := newLeafNodeInArena(arena, 1, false, 229, 230, advancePointByBytes(Point{}, source[:229]), advancePointByBytes(Point{}, source[:230]))
	errDot.setHasError(true)
	err := newParentNodeInArena(arena, errorSymbol, true, []*Node{errDot}, nil, 0)
	err.setExtra(true)
	err.setHasError(true)
	err.startByte = 227
	err.startPoint = advancePointByBytes(Point{}, source[:227])
	err.endByte = 230
	err.endPoint = advancePointByBytes(Point{}, source[:230])
	root := newParentNodeInArena(arena, 2, true, []*Node{def, err}, nil, 0)
	root.startByte = 7
	root.startPoint = advancePointByBytes(Point{}, source[:7])
	root.endByte = uint32(len(source))
	root.endPoint = advancePointByBytes(Point{}, source)

	tree := newTreeWithArenas(root, source, lang, arena, nil)
	tree.resultCompatibilityApplied = true
	return tree, &Parser{language: lang}, source
}

func TestCobolSecondNormalizationPreservesOmittedError(t *testing.T) {
	for _, omitted := range []bool{false, true} {
		tree, p, source := makeOmittedCobolFixture()
		tree.omittedRootError = omitted
		tree.resultErrorSummary = resultErrorSummaryPresent
		p.normalizeReturnedTreeForParse(tree, source)
		if tree.root.HasError() != omitted {
			t.Errorf("omitted=%v: root.HasError=%v", omitted, tree.root.HasError())
		}
		if omitted && tree.resultErrorSummary != resultErrorSummaryPresent {
			t.Error("omitted error summary lost")
		}
		tree.Release()
	}
}

func TestRestoreOmittedErrorPreservesHiddenMissingSummary(t *testing.T) {
	for _, tc := range []struct {
		name         string
		omitted      bool
		before, want resultErrorSummary
		wantError    bool
	}{
		{"clean", false, resultErrorSummaryClean, resultErrorSummaryClean, false},
		{"ordinary error", true, resultErrorSummaryClean, resultErrorSummaryPresent, true},
		{"hidden missing", true, resultErrorSummaryHiddenMissing, resultErrorSummaryHiddenMissing, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := &Node{}
			tree := &Tree{root: root, omittedRootError: tc.omitted, resultErrorSummary: tc.before}
			tree.restoreOmittedRootError()
			if root.HasError() != tc.wantError || tree.resultErrorSummary != tc.want {
				t.Fatalf("error=%v summary=%v, want %v/%v", root.HasError(), tree.resultErrorSummary, tc.wantError, tc.want)
			}
		})
	}
}

func TestOrdinaryOmittedErrorSurvivesCooklangNormalization(t *testing.T) {
	lang := &Language{Name: "cooklang", SymbolNames: []string{"EOF", "recipe", "_wrapper", "step", "metadata", "comment"}, SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "recipe", Visible: true, Named: true}, {Name: "_wrapper", Named: true}, {Name: "step", Visible: true, Named: true}, {Name: "metadata", Visible: true, Named: true}, {Name: "comment", Visible: true, Named: true}}}
	p := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("ab")
	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{wrapper}, nil, 0)
	extra := newParentNodeInArena(arena, errorSymbol, true, []*Node{newLeafNodeInArena(arena, 3, true, 1, 2, Point{Column: 1}, Point{Column: 2})}, nil, 0)
	extra.setExtra(true)
	extra.setHasError(true)
	tree := p.buildResultFromNodes([]*Node{root, extra}, source, arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("nil result")
	}
	defer tree.Release()
	p.reconcileAcceptedResultErrors(tree, tree.root, arena)
	if tree.resultErrorSummary != resultErrorSummaryPresent || !tree.RootNode().HasError() {
		t.Fatalf("omitted ERROR lost during Cooklang normalization or reconciliation: %s", tree.RootNode().SExpr(lang))
	}
}
func TestOrdinaryCleanExtraStaysCleanAfterCooklangNormalization(t *testing.T) {
	lang := &Language{Name: "cooklang", SymbolNames: []string{"EOF", "recipe", "_wrapper", "step", "metadata", "comment"}, SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "recipe", Visible: true, Named: true}, {Name: "_wrapper", Named: true}, {Name: "step", Visible: true, Named: true}, {Name: "metadata", Visible: true, Named: true}, {Name: "comment", Visible: true, Named: true}}}
	p := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{wrapper}, nil, 0)
	extra := newLeafNodeInArena(arena, 5, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	extra.setExtra(true)
	tree := p.buildResultFromNodes([]*Node{root, extra}, []byte("ab"), arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("nil result")
	}
	defer tree.Release()
	tree.root.setHasError(true) // stale flag, with no error-bearing extras
	p.reconcileAcceptedResultErrors(tree, tree.root, arena)
	if tree.resultErrorSummary != resultErrorSummaryClean || tree.RootNode().HasError() {
		t.Fatal("clean extra or stale flag made root erroneous")
	}
}
func TestOrdinaryOmittedErrorSurvivesDeferredCompatibility(t *testing.T) {
	lang := &Language{Name: "typescript", SymbolNames: []string{"EOF", "program", "_wrapper", "word"}, SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "program", Visible: true, Named: true}, {Name: "_wrapper", Named: true}, {Name: "word", Visible: true, Named: true}}}
	p := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{wrapper}, nil, 0)
	extra := newParentNodeInArena(arena, errorSymbol, true, []*Node{newLeafNodeInArena(arena, 3, true, 1, 2, Point{Column: 1}, Point{Column: 2})}, nil, 0)
	extra.setExtra(true)
	extra.setHasError(true)
	tree := p.buildResultFromNodes([]*Node{root, extra}, []byte("ab"), arena, nil, nil, nil)
	if tree == nil {
		t.Fatal("nil tree")
	}
	defer tree.Release()
	if !tree.hasDeferredResultCompatibility() {
		t.Fatal("compatibility ran eagerly")
	}
	if !tree.root.HasError() {
		t.Fatal("unpublished root is clean")
	}
	if !tree.RootNode().HasError() {
		t.Fatal("deferred normalization lost omitted error")
	}
	if tree.resultErrorSummary != resultErrorSummaryPresent {
		t.Fatal("error summary lost")
	}
}
func omittedDeferredRetryFixture(t *testing.T, withMissing bool) *Tree {
	t.Helper()
	lang := &Language{Name: "typescript", SymbolNames: []string{"EOF", "program", "word"}, SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "program", Visible: true, Named: true}, {Name: "word", Visible: true, Named: true}}}
	p := &Parser{language: lang}
	a := acquireNodeArena(arenaClassFull)
	a1 := newLeafNodeInArena(a, 2, true, 0, 1, Point{}, Point{Column: 1})
	a2 := newLeafNodeInArena(a, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	if withMissing {
		a2.endByte = a2.startByte
		a2.endPoint = a2.startPoint
		a2.setMissing(true)
	}
	root := newParentNodeInArena(a, 1, true, []*Node{a1, a2}, nil, 0)
	extra := newParentNodeInArena(a, errorSymbol, true, []*Node{newLeafNodeInArena(a, 2, true, 2, 3, Point{Column: 2}, Point{Column: 3})}, nil, 0)
	extra.setExtra(true)
	extra.setHasError(true)
	tree := p.buildResultFromNodes([]*Node{root, extra}, []byte("abc"), a, nil, nil, nil)
	if tree == nil {
		t.Fatal("nil tree")
	}
	tree.setParseRuntime(ParseRuntime{StopReason: ParseStopAccepted, IncrementalOldTreeReuseRoute: true, MaxStacksSeen: 8})
	return tree
}
func TestOmittedErrorPreAccessRetryAndCopy(t *testing.T) {
	for _, missing := range []bool{false, true} {
		tree := omittedDeferredRetryFixture(t, missing)
		if !tree.hasDeferredResultCompatibility() || !tree.omittedRootError || !tree.root.HasError() || tree.resultErrorSummary != resultErrorSummaryPresent {
			t.Fatal("missing pre-access provenance or summary")
		}
		(&Parser{language: tree.language}).reconcileAcceptedResultErrors(tree, tree.root, tree.arena)
		if !tree.root.HasError() || tree.resultErrorSummary != resultErrorSummaryPresent {
			t.Fatal("accepted-result reconciliation lost omitted error before retry")
		}
		if incrementalAcceptedErrorIsLocal(tree, 3) || !shouldRetryIncrementalParseAsFull(tree, 3, 8) {
			t.Fatal("omitted error incorrectly declared local before access")
		}
		copyTree := tree.Copy()
		if copyTree == nil || !copyTree.omittedRootError || !copyTree.RootNode().HasError() {
			t.Fatal("copy lost omitted error")
		}
		if incrementalAcceptedErrorIsLocal(tree, 3) || incrementalAcceptedErrorIsLocal(copyTree, 3) {
			t.Fatal("omitted error incorrectly declared local after normalization or copy")
		}
		copyTree.Edit(InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}})
		if !copyTree.omittedRootError {
			t.Fatal("edit cleared unproven omitted error")
		}
		copyTree.Release()
		if copyTree.omittedRootError {
			t.Fatal("release did not reset provenance")
		}
		tree.Release()
	}
}
func TestOmittedErrorConcurrentDeferredReaders(t *testing.T) {
	tree := omittedDeferredRetryFixture(t, false)
	defer tree.Release()
	var wg sync.WaitGroup
	errs := make(chan bool, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); n := tree.RootNode(); errs <- n != nil && n.HasError() }()
	}
	wg.Wait()
	close(errs)
	for ok := range errs {
		if !ok {
			t.Fatal("reader lost omitted error")
		}
	}
	if tree.resultErrorSummary != resultErrorSummaryPresent {
		t.Fatal("deferred summary lost")
	}
}
func TestAcceptedErrorLocalityRequiresErrorBearingChild(t *testing.T) {
	a := acquireNodeArena(arenaClassFull)
	defer a.Release()
	x := newLeafNodeInArena(a, 1, true, 0, 1, Point{}, Point{Column: 1})
	y := newLeafNodeInArena(a, 1, true, 1, 1, Point{Column: 1}, Point{Column: 1})
	root := newParentNodeInArena(a, 2, true, []*Node{x, y}, nil, 0)
	root.setHasError(true)
	tree := &Tree{root: root}
	tree.setParseRuntime(ParseRuntime{IncrementalOldTreeReuseRoute: true})
	defer func() {
		if tree.parseRuntime != nil {
			releaseParseRuntime(tree.parseRuntime)
		}
	}()
	if incrementalAcceptedErrorIsLocal(tree, 10) {
		t.Fatal("clean children cannot prove error locality")
	}
	y.setMissing(true)
	if !incrementalAcceptedErrorIsLocal(tree, 10) {
		t.Fatal("zero-width MISSING child should count as local")
	}
	tree.omittedRootError = true
	if incrementalAcceptedErrorIsLocal(tree, 10) {
		t.Fatal("visible missing child must not hide omitted error")
	}
}
func TestForestResultPreservesOmittedErrorProvenance(t *testing.T) {
	lang := &Language{Name: "cooklang", SymbolNames: []string{"EOF", "recipe", "step", "metadata", "comment"}, SymbolMetadata: []SymbolMetadata{{Name: "EOF"}, {Name: "recipe", Visible: true, Named: true}, {Name: "step", Visible: true, Named: true}, {Name: "metadata", Visible: true, Named: true}, {Name: "comment", Visible: true, Named: true}}}
	p := &Parser{language: lang}
	for _, omitted := range []bool{false, true} {
		a := acquireNodeArena(arenaClassFull)
		leaf := newLeafNodeInArena(a, 2, true, 0, 1, Point{}, Point{Column: 1})
		root := newParentNodeInArena(a, 1, true, []*Node{leaf}, nil, 0)
		tree := newForestResultTree(root, []byte("a"), lang, a, omitted)
		p.normalizeReturnedTreeForParse(tree, []byte("a"))
		p.reconcileAcceptedResultErrors(tree, tree.root, a)
		if tree.omittedRootError != omitted || tree.root.HasError() != omitted {
			t.Errorf("omitted=%v: provenance=%v HasError=%v", omitted, tree.omittedRootError, tree.root.HasError())
		}
		copyTree := tree.Copy()
		if copyTree.omittedRootError != omitted || copyTree.RootNode().HasError() != omitted {
			t.Errorf("omitted=%v: copy lost provenance", omitted)
		}
		copyTree.Edit(InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}})
		if copyTree.omittedRootError != omitted {
			t.Errorf("omitted=%v: edit lost provenance", omitted)
		}
		copyTree.Release()
		tree.Release()
	}
}
