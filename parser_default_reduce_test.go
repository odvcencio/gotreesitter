package gotreesitter

import "testing"

func TestParseEagerDefaultReduceDefaultOff(t *testing.T) {
	t.Setenv("GOT_EAGER_DEFAULT_REDUCE", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	if parseEagerDefaultReduceEnabled() {
		t.Fatal("parseEagerDefaultReduceEnabled() = true with empty env, want false")
	}
}

func TestParseEagerDefaultReduceExplicitOptIn(t *testing.T) {
	t.Setenv("GOT_EAGER_DEFAULT_REDUCE", "1")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	if !parseEagerDefaultReduceEnabled() {
		t.Fatal("parseEagerDefaultReduceEnabled() = false with env=1, want true")
	}
}

func TestSettleDeterministicReduceChainForReuseUsesCertifiedRepetitionChoice(t *testing.T) {
	lang := &Language{
		Name:         "ownership_settle",
		InitialState: 1,
		StateCount:   5,
		SymbolCount:  4,
		TokenCount:   2,
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "item", Visible: true, Named: true},
			{Name: "statement", Visible: true, Named: true},
			{Name: "program_repeat1"},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 2, ProductionID: 1},
				{Type: ParseActionShift, State: 3, Repetition: true},
			}},
		},
		ParseTable: make([][]uint16, 5),
	}
	for state := range lang.ParseTable {
		lang.ParseTable[state] = make([]uint16, lang.SymbolCount)
	}
	lang.ParseTable[3][1] = 1
	lang.ParseTable[1][3] = 4

	parser := NewParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	var tmpEntries []stackEntry
	stack := newGLRStackWithScratch(1, &entryScratch)
	left := NewLeafNode(2, true, 0, 1, Point{}, Point{Column: 1})
	right := NewLeafNode(2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	stack.entries = append(stack.entries,
		newStackEntryNode(2, left),
		newStackEntryNode(3, right),
	)
	stack.byteOffset = 2
	nodeCount := 2
	trackChildErrors := false
	forked := false

	reduced, reached := parser.settleDeterministicReduceChainForReuse(
		nil,
		&stack,
		Token{Symbol: 1, StartByte: 2, EndByte: 3},
		4,
		1,
		&reuseCursor{},
		&nodeCount,
		arena,
		&entryScratch,
		&gssScratch,
		&tmpEntries,
		false,
		&trackChildErrors,
		&forked,
	)
	if !reduced || !reached {
		t.Fatalf("settle result = reduced %v reached %v state %d, want true/true at state 4", reduced, reached, stack.top().state)
	}
	if forked {
		t.Fatal("certified repetition choice unexpectedly forked")
	}
	if got := stack.top().state; got != 4 {
		t.Fatalf("settled state = %d, want recorded ownership frontier 4", got)
	}
	parent := stackEntryNode(stack.top())
	if parent == nil || parent.Symbol() != 3 {
		t.Fatalf("settled parent = %#v, want program_repeat1", parent)
	}
	if !parent.isFragile() {
		t.Fatal("conflict-selected ownership reduction did not preserve fragility metadata")
	}
}

func TestExternalNoActionDefaultReduceDrainsForksBetweenRounds(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	lang := &Language{
		Name:            "test",
		InitialState:    1,
		StateCount:      10,
		SymbolCount:     5,
		TokenCount:      3,
		ExternalSymbols: []Symbol{2},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "left", Visible: true, Named: true},
			{Name: "external", Visible: true, Named: true},
			{Name: "first_parent", Visible: true, Named: true},
			{Name: "second_parent", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 2, ProductionID: 1}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 1, ProductionID: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 5}}},
		},
		ParseTable: make([][]uint16, 10),
	}
	for i := range lang.ParseTable {
		lang.ParseTable[i] = make([]uint16, lang.SymbolCount)
	}
	lang.ParseTable[3][0] = 1
	lang.ParseTable[1][0] = 2
	lang.ParseTable[9][0] = 2
	lang.ParseTable[1][4] = 4
	lang.ParseTable[9][4] = 5

	parser := NewParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	var gssScratch gssScratch
	var entryScratch glrEntryScratch
	var tmpEntries []stackEntry
	base := gssScratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := gssScratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightNode := gssScratch.allocNode(newStackEntryNode(3, right), leftNode, 3)
	altBase := gssScratch.allocNode(stackEntry{state: 9}, nil, 1)
	altLeft := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	altLeftNode := gssScratch.allocNode(newStackEntryNode(8, altLeft), altBase, 2)
	altRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightNode.appendExtraLink(gssMainLink{
		prev:  altLeftNode,
		entry: newStackEntryNode(7, altRight),
	})
	stacks := []glrStack{{gss: gssStack{head: rightNode}, byteOffset: 2}}
	nodeCount := 0
	trackChildErrors := false
	tok := Token{Symbol: 2, StartByte: 2, EndByte: 3}
	seen := make(map[externalDefaultReduceSeenKey]int)

	if !parser.applyExternalNoActionDefaultReduceStep(nil, tok, stacks, &nodeCount, arena, &entryScratch, &gssScratch, &tmpEntries, false, &trackChildErrors, seen) {
		t.Fatal("first default-reduce step = false, want true")
	}
	if got := len(parser.pendingForkStacks); got != 1 {
		t.Fatalf("pending forks after first step = %d, want 1", got)
	}
	stacks = append(stacks, parser.pendingForkStacks...)
	parser.pendingForkStacks = parser.pendingForkStacks[:0]
	if !parser.canApplyExternalNoActionDefaultReduce(tok, stacks) {
		t.Fatal("drained fork was not eligible for the next staged default-reduce round")
	}

	if !parser.applyExternalNoActionDefaultReduceStep(nil, tok, stacks, &nodeCount, arena, &entryScratch, &gssScratch, &tmpEntries, false, &trackChildErrors, seen) {
		t.Fatal("second default-reduce step = false, want true")
	}
	if len(parser.pendingForkStacks) != 0 {
		stacks = append(stacks, parser.pendingForkStacks...)
		parser.pendingForkStacks = parser.pendingForkStacks[:0]
	}
	if got := len(stacks); got != 2 {
		t.Fatalf("stack count after draining = %d, want 2", got)
	}
	if stacks[0].top().state != 4 || stacks[1].top().state != 5 {
		t.Fatalf("top states after second round = %d/%d, want 4/5", stacks[0].top().state, stacks[1].top().state)
	}
	if !parser.externalNoActionDefaultReducesStable(tok, stacks) {
		t.Fatal("external default reductions not stable after drained fork completed its round")
	}
}

type failingExternalRelexTokenSource struct {
	tokens              []Token
	nextCalls           int
	relexCalls          int
	parserStates        []StateID
	parserStateAtRelex  StateID
	hadParserStateRelex bool
}

func (s *failingExternalRelexTokenSource) Next() Token {
	if s.nextCalls >= len(s.tokens) {
		return Token{Symbol: 0}
	}
	tok := s.tokens[s.nextCalls]
	s.nextCalls++
	return tok
}

func (s *failingExternalRelexTokenSource) CanRelexFromTokenStart(tok Token) bool {
	return tok.Symbol == 1
}

func (s *failingExternalRelexTokenSource) RelexFromTokenStart(tok Token) (Token, bool) {
	s.relexCalls++
	if len(s.parserStates) > 0 {
		s.parserStateAtRelex = s.parserStates[len(s.parserStates)-1]
		s.hadParserStateRelex = true
	}
	return Token{}, false
}

func (s *failingExternalRelexTokenSource) SetParserState(state StateID) {
	s.parserStates = append(s.parserStates, state)
}

func (s *failingExternalRelexTokenSource) SetGLRStates([]StateID) {}

func TestExternalNoActionDefaultReduceRelexFailureNoActionErrorConsumesCurrentToken(t *testing.T) {
	lang := &Language{
		Name:            "test",
		InitialState:    1,
		StateCount:      5,
		SymbolCount:     4,
		TokenCount:      3,
		ExternalSymbols: []Symbol{1},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "external", Visible: true, Named: true},
			{Name: "word", Visible: true, Named: true},
			{Name: "document", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: make([][]uint16, 5),
	}
	for i := range lang.ParseTable {
		lang.ParseTable[i] = make([]uint16, lang.SymbolCount)
	}
	lang.ParseTable[1][2] = 1
	lang.ParseTable[1][3] = 3
	lang.ParseTable[2][0] = 2
	lang.ParseTable[3][0] = 4
	lang.ParseTable[4][0] = 4

	parser := NewParser(lang)
	ts := &failingExternalRelexTokenSource{tokens: []Token{
		{Symbol: 2, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
		{Symbol: 1, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}},
		{Symbol: 0, StartByte: 2, EndByte: 2, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 2}},
	}}

	tree := parser.parseInternal([]byte("wx"), ts, nil, nil, arenaClassFull, nil, 0, 0, 0, false)
	if got, want := tree.ParseStopReason(), ParseStopAccepted; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	if ts.relexCalls != 1 {
		t.Fatalf("relex calls = %d, want 1", ts.relexCalls)
	}
	if !ts.hadParserStateRelex || ts.parserStateAtRelex != 3 {
		t.Fatalf("parser state at failed relex = %d/%t from %v, want 3/true", ts.parserStateAtRelex, ts.hadParserStateRelex, ts.parserStates)
	}
	runtime := tree.ParseRuntime()
	if runtime.TokensConsumed != 3 || !runtime.LastTokenWasEOF {
		t.Fatalf("runtime tokens = %d last EOF = %t, want 3/true", runtime.TokensConsumed, runtime.LastTokenWasEOF)
	}
	if got := countErrorNodesWithSpan(tree.RootNode(), 1, 2); got != 1 {
		t.Fatalf("error nodes for failed external token = %d, want 1", got)
	}
}

func TestExternalNoActionDefaultReduceRelexFailureExtraShiftConsumesCurrentToken(t *testing.T) {
	lang := &Language{
		Name:            "test",
		InitialState:    1,
		StateCount:      5,
		SymbolCount:     4,
		TokenCount:      3,
		ExternalSymbols: []Symbol{1},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "external", Visible: true, Named: true},
			{Name: "word", Visible: true, Named: true},
			{Name: "document", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, Extra: true}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: make([][]uint16, 5),
	}
	for i := range lang.ParseTable {
		lang.ParseTable[i] = make([]uint16, lang.SymbolCount)
	}
	lang.ParseTable[1][2] = 1
	lang.ParseTable[1][3] = 3
	lang.ParseTable[2][0] = 2
	lang.ParseTable[3][0] = 4
	lang.ParseTable[3][1] = 3

	parser := NewParser(lang)
	ts := &failingExternalRelexTokenSource{tokens: []Token{
		{Symbol: 2, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
		{Symbol: 1, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}},
		{Symbol: 0, StartByte: 2, EndByte: 2, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 2}},
	}}

	tree := parser.parseInternal([]byte("wx"), ts, nil, nil, arenaClassFull, nil, 0, 0, 0, false)
	if got, want := tree.ParseStopReason(), ParseStopAccepted; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	if ts.relexCalls != 1 {
		t.Fatalf("relex calls = %d, want 1", ts.relexCalls)
	}
	if !ts.hadParserStateRelex || ts.parserStateAtRelex != 3 {
		t.Fatalf("parser state at failed relex = %d/%t from %v, want 3/true", ts.parserStateAtRelex, ts.hadParserStateRelex, ts.parserStates)
	}
	runtime := tree.ParseRuntime()
	if runtime.TokensConsumed != 3 || !runtime.LastTokenWasEOF {
		t.Fatalf("runtime tokens = %d last EOF = %t, want 3/true", runtime.TokensConsumed, runtime.LastTokenWasEOF)
	}
	if got := countNodesWithSymbolSpan(tree.RootNode(), 1, 1, 2); got != 1 {
		t.Fatalf("external nodes for failed relex token = %d, want 1", got)
	}
}

func countErrorNodesWithSpan(n *Node, start, end uint32) int {
	return countNodesWithSymbolSpan(n, errorSymbol, start, end)
}

func countNodesWithSymbolSpan(n *Node, sym Symbol, start, end uint32) int {
	if n == nil {
		return 0
	}
	count := 0
	if n.Symbol() == sym && n.StartByte() == start && n.EndByte() == end {
		count++
	}
	for i := 0; i < n.ChildCount(); i++ {
		count += countNodesWithSymbolSpan(n.Child(i), sym, start, end)
	}
	return count
}

// Reuse requires one stack, even for artifacts without the new continuation
// publication grant. A real packed-pop fork must stop either settling loop.
func TestUncertifiedReuseSettlingStopsAtPackedFork(t *testing.T) {
	for _, mode := range []string{"eager", "deterministic"} {
		t.Run(mode, func(t *testing.T) {
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			var gs gssScratch
			var es glrEntryScratch
			var tmp []stackEntry
			leaf := func(sym Symbol, start, end uint32) *Node {
				return newLeafNodeInArena(arena, sym, true, start, end, Point{Column: start}, Point{Column: end})
			}
			base := gs.allocNode(stackEntry{state: 1}, nil, 1)
			left := gs.allocNode(newStackEntryNode(2, leaf(10, 0, 1)), base, 2)
			right := gs.allocNode(newStackEntryNode(2, leaf(11, 0, 1)), base, 2)
			head := gs.allocNode(newStackEntryNode(3, leaf(12, 1, 2)), left, 3)
			head.appendExtraLink(gssMainLink{prev: right, entry: newStackEntryNode(3, leaf(13, 1, 2))})
			lang := &Language{StateCount: 5, SymbolCount: 40, TokenCount: 2, SymbolMetadata: make([]SymbolMetadata, 40), ParseTable: make([][]uint16, 5)}
			for i := range lang.SymbolMetadata {
				lang.SymbolMetadata[i] = SymbolMetadata{Visible: true, Named: true}
			}
			for i := range lang.ParseTable {
				lang.ParseTable[i] = make([]uint16, 40)
			}
			lang.ParseActions = []ParseActionEntry{{}, {Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 32, ChildCount: 1}}}, {Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 33, ChildCount: 1}}}}
			lang.ParseTable[3][1] = 1
			lang.ParseTable[2][1] = 2
			lang.ParseTable[2][32] = 2
			lang.ParseTable[2][33] = 4
			parser := NewParser(lang)
			parser.mergeScratch = &glrMergeScratch{language: lang}
			stack := glrStack{gss: gssStack{head: head}, byteOffset: 2}
			tok := Token{Symbol: 1, StartByte: 2, EndByte: 2}
			nodes, track, forked := 0, false, false
			var reduced bool
			if mode == "eager" {
				reduced = parser.settleEagerDefaultReduceChainForReuse(nil, &stack, tok, &nodes, arena, &es, &gs, &tmp, false, &track, &forked)
			} else {
				var reached bool
				reduced, reached = parser.settleDeterministicReduceChainForReuse(nil, &stack, tok, 4, 1, &reuseCursor{}, &nodes, arena, &es, &gs, &tmp, false, &track, &forked)
				if reached {
					t.Fatal("reuse target reached before publishing its competing continuation")
				}
			}
			if !reduced || !forked || len(parser.pendingForkStacks) != 1 || len(parser.pendingFrontierForkStacks) != 0 {
				t.Fatalf("packed reduction not reported: reduced=%t forked=%t pending=%d/%d", reduced, forked, len(parser.pendingForkStacks), len(parser.pendingFrontierForkStacks))
			}
			if got := stackEntryNodeSymbol(stack.top()); got != 32 {
				t.Fatalf("primary advanced past first reduction: symbol=%d", got)
			}
			if got := stackEntryNodeSymbol(parser.pendingForkStacks[0].top()); got != 32 {
				t.Fatalf("queued fork symbol=%d, want32", got)
			}
		})
	}
}
