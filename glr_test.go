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

func TestGLRStackClone(t *testing.T) {
	s := newGLRStack(StateID(1))
	s.entries = append(s.entries, stackEntry{state: 2, node: nil})
	s.score = 5

	clone := s.clone()
	clone.entries = append(clone.entries, stackEntry{state: 3, node: nil})
	clone.score = 10

	if len(s.entries) != 2 {
		t.Errorf("original entries modified: len=%d, want 2", len(s.entries))
	}
	if s.score != 5 {
		t.Errorf("original score modified: %d, want 5", s.score)
	}
	if len(clone.entries) != 3 {
		t.Errorf("clone entries wrong: len=%d, want 3", len(clone.entries))
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

	tree := parser.Parse([]byte("x"))
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
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
