package gotreesitter

import "testing"

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

func TestBuildSingleTokenWrapperSymbols(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "single_wrapper", Visible: true, Named: true},
			{Name: "multi_wrapper", Visible: true, Named: true},
			{Name: "statement", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 1, ChildCount: 1, ProductionID: 10}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 2, ChildCount: 1, ProductionID: 11}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 2, ChildCount: 1, ProductionID: 12}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 2, ProductionID: 13}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}

	got := buildSingleTokenWrapperSymbols(lang)
	if !got[1] {
		t.Fatal("expected single_wrapper to be marked as a single-token wrapper")
	}
	if got[2] {
		t.Fatal("did not expect multi_wrapper to be marked as a single-token wrapper")
	}
	if got[3] {
		t.Fatal("did not expect statement to be marked as a single-token wrapper")
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

func TestCollapsibleUnarySelfReductionAliasesSingleAnonymousLeaf(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "optional_chain", Visible: true, Named: true},
			{Name: "?.", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language:                 lang,
		singleTokenWrapperSymbol: []bool{false, true, false},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil)
	if got == nil {
		t.Fatal("expected unary single-token reduction to collapse to named leaf")
	}
	if got.Symbol() != 1 {
		t.Fatalf("collapsed symbol = %d, want %d", got.Symbol(), 1)
	}
	if !got.IsNamed() {
		t.Fatal("collapsed node should be named")
	}
	if got.ChildCount() != 0 {
		t.Fatalf("collapsed child count = %d, want 0", got.ChildCount())
	}
	if got.StartByte() != 1 || got.EndByte() != 3 {
		t.Fatalf("collapsed span = [%d,%d), want [1,3)", got.StartByte(), got.EndByte())
	}
}

func TestCollapsibleRawUnarySelfReductionAliasesSingleAnonymousLeaf(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "optional_chain", Visible: true, Named: true},
			{Name: "?.", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language:                 lang,
		singleTokenWrapperSymbol: []bool{false, true, false},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1)
	if got == nil {
		t.Fatal("expected raw unary single-token reduction to collapse to named leaf")
	}
	if got.Symbol() != 1 {
		t.Fatalf("collapsed symbol = %d, want %d", got.Symbol(), 1)
	}
	if !got.IsNamed() {
		t.Fatal("collapsed node should be named")
	}
	if got.ChildCount() != 0 {
		t.Fatalf("collapsed child count = %d, want 0", got.ChildCount())
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
		language:                 lang,
		singleTokenWrapperSymbol: []bool{false, true, false},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("raw unary collapse returned %v for invisible child", got)
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
