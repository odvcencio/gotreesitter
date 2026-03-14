package gotreesitter

import "testing"

func TestMergeStacksRemovesDead(t *testing.T) {
	s1 := newGLRStack(StateID(1))
	s2 := newGLRStack(StateID(2))
	s2.dead = true
	s3 := newGLRStack(StateID(3))

	result := mergeStacks([]glrStack{s1, s2, s3})
	if len(result) != 2 {
		t.Fatalf("expected 2 alive stacks, got %d", len(result))
	}
	if result[0].top().state != 1 || result[1].top().state != 3 {
		t.Errorf("unexpected states: %d, %d", result[0].top().state, result[1].top().state)
	}
}

func TestMergeStacksSameTopState(t *testing.T) {
	s1 := newGLRStack(StateID(5))
	s1.score = 10
	s2 := newGLRStack(StateID(5))
	s2.score = 20

	result := mergeStacks([]glrStack{s1, s2})
	if len(result) != 1 {
		t.Fatalf("expected 1 merged stack, got %d", len(result))
	}
	if result[0].score != 20 {
		t.Errorf("expected higher-scoring stack (score 20), got %d", result[0].score)
	}
}

func TestMergeStacksSameStateDifferentByteOffset(t *testing.T) {
	s1 := newGLRStack(StateID(5))
	s1.push(5, NewLeafNode(1, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	s2 := newGLRStack(StateID(5))
	s2.push(5, NewLeafNode(1, true, 0, 7, Point{}, Point{Column: 7}), nil, nil)

	result := mergeStacks([]glrStack{s1, s2})
	if len(result) != 2 {
		t.Fatalf("expected 2 stacks (distinct byte offsets), got %d", len(result))
	}
}

func TestMergeStacksSameStateDifferentEntries(t *testing.T) {
	s1 := newGLRStack(StateID(5))
	s1.push(5, NewLeafNode(1, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	s2 := newGLRStack(StateID(5))
	s2.push(5, NewLeafNode(2, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	result := mergeStacks([]glrStack{s1, s2})
	if len(result) != 2 {
		t.Fatalf("expected 2 stacks (distinct parse paths), got %d", len(result))
	}
}

func TestMergeStacksSmallPathKeepsBestDuplicateAndDistinctKeys(t *testing.T) {
	s1 := newGLRStack(StateID(5))
	s1.score = 10
	s1.push(5, NewLeafNode(1, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	s2 := newGLRStack(StateID(5))
	s2.score = 20
	s2.push(5, NewLeafNode(1, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	s3 := newGLRStack(StateID(5))
	s3.score = 15
	s3.push(5, NewLeafNode(1, true, 0, 7, Point{}, Point{Column: 7}), nil, nil)

	s4 := newGLRStack(StateID(6))
	s4.score = 5

	result := mergeStacks([]glrStack{s1, s2, s3, s4})
	if len(result) != 3 {
		t.Fatalf("expected 3 stacks after small-path merge, got %d", len(result))
	}

	foundBestDuplicate := false
	foundOffsetSeven := false
	foundStateSix := false
	for i := range result {
		top := result[i].top()
		switch {
		case top.state == 5 && result[i].byteOffset == 3:
			if result[i].score != 20 {
				t.Fatalf("best duplicate score = %d, want 20", result[i].score)
			}
			foundBestDuplicate = true
		case top.state == 5 && result[i].byteOffset == 7:
			foundOffsetSeven = true
		case top.state == 6:
			foundStateSix = true
		}
	}
	if !foundBestDuplicate || !foundOffsetSeven || !foundStateSix {
		t.Fatalf("missing expected survivors: duplicate=%v offset7=%v state6=%v", foundBestDuplicate, foundOffsetSeven, foundStateSix)
	}
}

func TestMergeStacksSmallPathKeepsDistinctDeepStructures(t *testing.T) {
	makeTop := func(grandchild Symbol) *Node {
		left := NewLeafNode(grandchild, true, 0, 2, Point{}, Point{Column: 2})
		right := NewLeafNode(13, true, 2, 5, Point{Column: 2}, Point{Column: 5})
		mid := NewParentNode(11, true, []*Node{left, right}, nil, 0)
		return NewParentNode(10, true, []*Node{mid}, nil, 0)
	}

	s1 := newGLRStack(StateID(5))
	s1.push(5, makeTop(12), nil, nil)

	s2 := newGLRStack(StateID(5))
	s2.push(5, makeTop(14), nil, nil)

	result := mergeStacks([]glrStack{s1, s2})
	if len(result) != 2 {
		t.Fatalf("expected 2 stacks for distinct deep structures, got %d", len(result))
	}
}

func TestGLRStackClone(t *testing.T) {
	s := newGLRStack(StateID(1))
	s.push(2, nil, nil, nil)
	s.score = 5

	clone := s.clone()
	clone.push(3, nil, nil, nil)
	clone.score = 10

	if s.depth() != 2 {
		t.Errorf("original entries modified: len=%d, want 2", s.depth())
	}
	if s.score != 5 {
		t.Errorf("original score modified: %d, want 5", s.score)
	}
	if clone.depth() != 3 {
		t.Errorf("clone entries wrong: len=%d, want 3", clone.depth())
	}
}

// buildAmbiguousLanguage creates a grammar where an input can be parsed
// two ways, triggering GLR fork. The grammar:
//
//	S -> A | B
//	A -> x     (production 0, DynamicPrecedence = 0)
//	B -> x     (production 1, DynamicPrecedence = 5)
//
// Both A and B match the same input "x", but B has higher precedence.
// The parser should fork, try both, and pick B.
//
// Symbols: 0=EOF, 1=x (terminal), 2=A (nonterminal), 3=B (nonterminal), 4=S (nonterminal)
//
// States:
//
//	0: x -> shift 1, S -> goto 3, A -> goto 2, B -> goto 2
//	1: any -> reduce A->x AND reduce B->x (multi-action = GLR fork!)
//	2: EOF -> accept
//	3: EOF -> accept (same as state 2 for S)
func buildAmbiguousLanguage() *Language {
	return &Language{
		Name:               "ambiguous",
		SymbolCount:        5,
		TokenCount:         2,
		ExternalTokenCount: 0,
		StateCount:         4,
		LargeStateCount:    0,
		FieldCount:         0,
		ProductionIDCount:  2,

		SymbolNames: []string{"EOF", "x", "A", "B", "S"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "x", Visible: true, Named: true},
			{Name: "A", Visible: true, Named: true},
			{Name: "B", Visible: true, Named: true},
			{Name: "S", Visible: true, Named: true},
		},
		FieldNames: []string{""},

		ParseActions: []ParseActionEntry{
			// 0: error / no action
			{Actions: nil},
			// 1: shift to state 1
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			// 2: TWO actions — GLR fork!
			//    reduce A -> x (1 child, symbol 2, prec 0)
			//    reduce B -> x (1 child, symbol 3, prec 5)
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 2, ChildCount: 1, ProductionID: 0, DynamicPrecedence: 0},
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 1, DynamicPrecedence: 5},
			}},
			// 3: goto state 2 (for A)
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			// 4: goto state 2 (for B)
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			// 5: accept
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},

		ParseTable: [][]uint16{
			// State 0: x->shift(1), A->goto(3), B->goto(4), S->... (unused)
			{0, 1, 3, 4, 0},
			// State 1: any -> action 2 (multi-action: reduce A or reduce B)
			{2, 2, 0, 0, 0},
			// State 2: EOF -> accept
			{5, 0, 0, 0, 0},
			// State 3: (unused, but needed for state count)
			{0, 0, 0, 0, 0},
		},

		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
		},

		LexStates: []LexState{
			// State 0: start
			{
				AcceptToken: 0,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: 'x', Hi: 'x', NextState: 1},
					{Lo: ' ', Hi: ' ', NextState: 2},
				},
			},
			// State 1: accept x (symbol 1)
			{
				AcceptToken: 1,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
			},
			// State 2: whitespace (skip)
			{
				AcceptToken: 0,
				Skip:        true,
				Default:     -1,
				EOF:         -1,
			},
		},
	}
}

func TestGLRForkPicksHigherPrecedence(t *testing.T) {
	lang := buildAmbiguousLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("x"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// The root should be B (symbol 3, prec 5) not A (symbol 2, prec 0)
	// because B has higher dynamic precedence.
	if root.Symbol() != 3 {
		t.Errorf("GLR should pick B (symbol 3, prec 5) but got symbol %d (%s)",
			root.Symbol(), root.Type(lang))
	}
}

func buildForkLanguage(precedences []int16, childCounts []uint8) *Language {
	if len(precedences) == 0 {
		panic("buildForkLanguage requires at least one reduce action")
	}
	if len(precedences) != len(childCounts) {
		panic("buildForkLanguage precedence and childCount lengths must match")
	}

	symbolCount := 2 + len(precedences)
	symbolNames := make([]string, symbolCount)
	symbolMeta := make([]SymbolMetadata, symbolCount)
	symbolNames[0] = "EOF"
	symbolMeta[0] = SymbolMetadata{Name: "EOF"}
	symbolNames[1] = "x"
	symbolMeta[1] = SymbolMetadata{Name: "x", Visible: true, Named: true}
	for i := range precedences {
		name := string(rune('A' + i))
		symbolNames[2+i] = name
		symbolMeta[2+i] = SymbolMetadata{Name: name, Visible: true, Named: true}
	}

	multi := make([]ParseAction, 0, len(precedences))
	for i, prec := range precedences {
		multi = append(multi, ParseAction{
			Type:              ParseActionReduce,
			Symbol:            Symbol(2 + i),
			ChildCount:        childCounts[i],
			ProductionID:      uint16(i),
			DynamicPrecedence: prec,
		})
	}

	// Parse action table:
	//   0: error/no-action
	//   1: shift x -> state 1
	//   2: multi-reduce fork entry
	//   3..(3+n-1): goto actions for non-terminals -> state 2
	//   acceptIdx: accept action
	parseActions := make([]ParseActionEntry, 0, 4+len(precedences))
	parseActions = append(parseActions, ParseActionEntry{Actions: nil})                                               // 0
	parseActions = append(parseActions, ParseActionEntry{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}}) // 1
	parseActions = append(parseActions, ParseActionEntry{Actions: multi})                                             // 2
	for range precedences {
		parseActions = append(parseActions, ParseActionEntry{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}})
	}
	acceptIdx := len(parseActions)
	parseActions = append(parseActions, ParseActionEntry{Actions: []ParseAction{{Type: ParseActionAccept}}})

	rowWidth := symbolCount
	state0 := make([]uint16, rowWidth)
	state0[1] = 1 // x -> shift
	for i := range precedences {
		state0[2+i] = uint16(3 + i) // goto for each non-terminal
	}
	state1 := make([]uint16, rowWidth)
	state1[0] = 2 // EOF -> multi reduce
	state1[1] = 2 // x   -> multi reduce
	state2 := make([]uint16, rowWidth)
	state2[0] = uint16(acceptIdx) // EOF -> accept
	state3 := make([]uint16, rowWidth)

	return &Language{
		Name:               "fork_language",
		SymbolCount:        uint32(symbolCount),
		TokenCount:         2,
		ExternalTokenCount: 0,
		StateCount:         4,
		LargeStateCount:    0,
		FieldCount:         0,
		ProductionIDCount:  uint32(len(precedences)),
		SymbolNames:        symbolNames,
		SymbolMetadata:     symbolMeta,
		FieldNames:         []string{""},
		ParseActions:       parseActions,
		ParseTable:         [][]uint16{state0, state1, state2, state3},
		LexModes:           []LexMode{{LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}},
		LexStates: []LexState{
			{
				AcceptToken: 0,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: 'x', Hi: 'x', NextState: 1},
					{Lo: ' ', Hi: ' ', NextState: 2},
				},
			},
			{
				AcceptToken: 1,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
			},
			{
				AcceptToken: 0,
				Skip:        true,
				Default:     -1,
				EOF:         -1,
			},
		},
	}
}

func TestGLRForkEqualPrecedenceTieKeepsFirstAction(t *testing.T) {
	lang := buildForkLanguage([]int16{0, 0}, []uint8{1, 1})
	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("x"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}
	if root.Symbol() != 2 {
		t.Fatalf("equal-precedence tie should keep first reduce symbol 2, got %d (%s)", root.Symbol(), root.Type(lang))
	}
}

func TestGLRForkHandlesThreeAlternatives(t *testing.T) {
	lang := buildForkLanguage([]int16{0, 5, 3}, []uint8{1, 1, 1})
	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("x"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}
	if root.Symbol() != 3 {
		t.Fatalf("three-way fork should pick highest precedence symbol 3, got %d (%s)", root.Symbol(), root.Type(lang))
	}
}

func TestGLRForkPrunesErroringAlternative(t *testing.T) {
	lang := buildForkLanguage([]int16{10, 1}, []uint8{2, 1})
	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("x"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}
	// First branch requires two non-extra children and should die.
	if root.Symbol() != 3 {
		t.Fatalf("error-pruned fork should keep surviving symbol 3, got %d (%s)", root.Symbol(), root.Type(lang))
	}
}

// TestMergeKeyGroupsEquivalentStacks proves stackEquivalent(a,b)==true
// implies mergeKeyForStack(a)==mergeKeyForStack(b). With coarse merge keys,
// this ensures equivalent stacks are always deduped in the same bucket.
func TestMergeKeyGroupsEquivalentStacks(t *testing.T) {
	scratch := &gssScratch{}

	// Helper to build a GSS-backed stack with known entries.
	buildStack := func(entries []stackEntry) glrStack {
		var s glrStack
		s.gss = buildGSSStack(entries, scratch)
		s.byteOffset = stackByteOffset(entries)
		return s
	}

	// Case 1: identical entries → equivalent, same hash.
	node1a := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, isNamed: true}
	node1b := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, isNamed: true}
	a := buildStack([]stackEntry{{state: 1}, {state: 2, node: node1a}})
	b := buildStack([]stackEntry{{state: 1}, {state: 2, node: node1b}})

	if !stackEquivalent(a, b) {
		t.Fatal("case 1: expected equivalent stacks")
	}
	ka := mergeKeyForStack(a)
	kb := mergeKeyForStack(b)
	if ka != kb {
		t.Fatalf("case 1: equivalent stacks have different merge keys: %+v vs %+v", ka, kb)
	}

	// Case 2: different symbol → not equivalent.
	node2a := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1}
	node2b := &Node{symbol: 11, startByte: 0, endByte: 5, parseState: 1}
	c := buildStack([]stackEntry{{state: 1}, {state: 2, node: node2a}})
	d := buildStack([]stackEntry{{state: 1}, {state: 2, node: node2b}})
	if stackEquivalent(c, d) {
		t.Fatal("case 2: expected non-equivalent stacks")
	}
	kc := mergeKeyForStack(c)
	kd := mergeKeyForStack(d)
	if kc == kd {
		t.Log("case 2: non-equivalent stacks share coarse merge key (expected collision)")
	}

	// Case 3: isMissing differs → not equivalent (hash includes isMissing).
	node3a := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1}
	node3b := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, isMissing: true}
	e := buildStack([]stackEntry{{state: 1}, {state: 2, node: node3a}})
	f := buildStack([]stackEntry{{state: 1}, {state: 2, node: node3b}})
	if stackEquivalent(e, f) {
		t.Fatal("case 3: isMissing differs, stacks should not be equivalent")
	}

	// Case 4: nil nodes on both sides → equivalent, same hash.
	g := buildStack([]stackEntry{{state: 1}, {state: 2}})
	h := buildStack([]stackEntry{{state: 1}, {state: 2}})
	if !stackEquivalent(g, h) {
		t.Fatal("case 4: expected equivalent nil-node stacks")
	}
	kg := mergeKeyForStack(g)
	kh := mergeKeyForStack(h)
	if kg != kh {
		t.Fatalf("case 4: equivalent nil-node stacks have different merge keys: %+v vs %+v", kg, kh)
	}

	// Case 5: with children — same children → equivalent.
	child1 := &Node{symbol: 20, startByte: 0, endByte: 3}
	child2 := &Node{symbol: 20, startByte: 0, endByte: 3}
	node5a := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, children: []*Node{child1}}
	node5b := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, children: []*Node{child2}}
	i := buildStack([]stackEntry{{state: 1}, {state: 2, node: node5a}})
	j := buildStack([]stackEntry{{state: 1}, {state: 2, node: node5b}})
	if !stackEquivalent(i, j) {
		t.Fatal("case 5: expected equivalent stacks with same children")
	}
	ki := mergeKeyForStack(i)
	kj := mergeKeyForStack(j)
	if ki != kj {
		t.Fatalf("case 5: equivalent stacks with children have different merge keys: %+v vs %+v", ki, kj)
	}

	// Case 6: children differ in symbol → not equivalent (children check).
	// Coarse key may collide; stackEquivalent must reject the mismatch.
	child3 := &Node{symbol: 20, startByte: 0, endByte: 3}
	child4 := &Node{symbol: 21, startByte: 0, endByte: 3}
	node6a := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, children: []*Node{child3}}
	node6b := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, children: []*Node{child4}}
	k := buildStack([]stackEntry{{state: 1}, {state: 2, node: node6a}})
	l := buildStack([]stackEntry{{state: 1}, {state: 2, node: node6b}})
	if stackEquivalent(k, l) {
		t.Fatal("case 6: expected non-equivalent stacks with different children")
	}
	// These may share the same coarse merge key; that's fine because
	// stackEquivalent still rejects them.
}

func TestMergeStacksAllDeadReturnsEmpty(t *testing.T) {
	s1 := newGLRStack(StateID(1))
	s2 := newGLRStack(StateID(2))
	s1.dead = true
	s2.dead = true
	merged := mergeStacks([]glrStack{s1, s2})
	if len(merged) != 0 {
		t.Fatalf("expected all-dead merge to return empty, got %d", len(merged))
	}
}
