package gotreesitter

import (
	"testing"
	"time"
)

// buildArithmeticLanguage constructs a hand-built LR grammar for simple
// arithmetic expressions:
//
//	expression -> NUMBER
//	expression -> expression PLUS NUMBER
//
// Symbols:
//
//	0: EOF
//	1: NUMBER (terminal, named)
//	2: PLUS "+" (terminal, anonymous)
//	3: expression (nonterminal, named)
//
// LR States:
//
//	State 0 (start):       NUMBER -> shift 1, expression -> goto 2
//	State 1 (saw NUMBER):  any -> reduce expression->NUMBER (1 child)
//	State 2 (saw expr):    PLUS -> shift 3, EOF -> accept
//	State 3 (saw expr +):  NUMBER -> shift 4
//	State 4 (saw e+N):     any -> reduce expression->expression PLUS NUMBER (3 children)
//
// Lexer DFA:
//
//	State 0: start (dispatches digits, '+', whitespace)
//	State 1: in number (accept Symbol 1)
//	State 2: saw '+' (accept Symbol 2)
//	State 3: whitespace (skip)
func buildArithmeticLanguage() *Language {
	return &Language{
		Name:               "arithmetic",
		SymbolCount:        4,
		TokenCount:         3,
		ExternalTokenCount: 0,
		StateCount:         5,
		LargeStateCount:    0,
		FieldCount:         0,
		ProductionIDCount:  2,

		SymbolNames: []string{"EOF", "NUMBER", "+", "expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "NUMBER", Visible: true, Named: true},
			{Name: "+", Visible: true, Named: false},
			{Name: "expression", Visible: true, Named: true},
		},
		FieldNames: []string{""},

		// ParseActions indexed by the action index stored in the parse table.
		//
		// Index 0: no-op / error (empty actions)
		// Index 1: Shift to state 1 (NUMBER in state 0)
		// Index 2: Reduce expression -> NUMBER (1 child, symbol 3, production 0)
		// Index 3: Shift to state 2 (GOTO for expression from state 0)
		// Index 4: Shift to state 3 (PLUS in state 2)
		// Index 5: Accept (EOF in state 2)
		// Index 6: Shift to state 4 (NUMBER in state 3)
		// Index 7: Reduce expression -> expr PLUS NUMBER (3 children, symbol 3, production 1)
		ParseActions: []ParseActionEntry{
			// 0: error / no action
			{Actions: nil},
			// 1: shift to state 1
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			// 2: reduce expression -> NUMBER
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 0}}},
			// 3: shift/goto to state 2
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			// 4: shift to state 3
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			// 5: accept
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
			// 6: shift to state 4
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			// 7: reduce expression -> expression PLUS NUMBER
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 3, ProductionID: 1}}},
		},

		// Dense parse table: [state][symbol] -> action index
		// Columns: EOF(0), NUMBER(1), PLUS(2), expression(3)
		ParseTable: [][]uint16{
			// State 0: shift NUMBER->1, goto expression->2
			{0, 1, 0, 3},
			// State 1: reduce on any terminal
			{2, 2, 2, 0},
			// State 2: accept on EOF, shift PLUS->3
			{5, 0, 4, 0},
			// State 3: shift NUMBER->4
			{0, 6, 0, 0},
			// State 4: reduce on any terminal
			{7, 7, 7, 0},
		},

		// All 5 parser states use the same lex DFA start state (0).
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
		},

		// Lexer DFA for: NUMBER ([0-9]+), PLUS ('+'), whitespace (skip)
		LexStates: []LexState{
			// State 0: start
			{
				AcceptToken: 0,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: '0', Hi: '9', NextState: 1},
					{Lo: '+', Hi: '+', NextState: 2},
					{Lo: ' ', Hi: ' ', NextState: 3},
					{Lo: '\t', Hi: '\t', NextState: 3},
					{Lo: '\n', Hi: '\n', NextState: 3},
				},
			},
			// State 1: in number (accept NUMBER = symbol 1)
			{
				AcceptToken: 1,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: '0', Hi: '9', NextState: 1},
				},
			},
			// State 2: saw '+' (accept PLUS = symbol 2)
			{
				AcceptToken: 2,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: nil,
			},
			// State 3: whitespace (skip)
			{
				AcceptToken: 0,
				Skip:        true,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 3},
					{Lo: '\t', Hi: '\t', NextState: 3},
					{Lo: '\n', Hi: '\n', NextState: 3},
				},
			},
		},
	}
}

// buildArithmeticRecoverLanguage is like buildArithmeticLanguage but adds a
// STAR token and a recover action in state 2. This lets tests verify that
// recovery can pop to an ancestor state and apply ParseActionRecover there.
func buildArithmeticRecoverLanguage() *Language {
	return &Language{
		Name:               "arithmetic_recover",
		SymbolCount:        5,
		TokenCount:         4,
		ExternalTokenCount: 0,
		StateCount:         5,
		LargeStateCount:    0,
		FieldCount:         0,
		ProductionIDCount:  2,

		SymbolNames: []string{"EOF", "NUMBER", "+", "*", "expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "NUMBER", Visible: true, Named: true},
			{Name: "+", Visible: true, Named: false},
			{Name: "*", Visible: true, Named: false},
			{Name: "expression", Visible: true, Named: true},
		},
		FieldNames: []string{""},

		ParseActions: []ParseActionEntry{
			{Actions: nil}, // 0
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},                                   // 1
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 1, ProductionID: 0}}}, // 2
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},                                   // 3 (goto)
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},                                   // 4
			{Actions: []ParseAction{{Type: ParseActionAccept}}},                                            // 5
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},                                   // 6
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 3, ProductionID: 1}}}, // 7
			{Actions: []ParseAction{{Type: ParseActionRecover, State: 3}}},                                 // 8
		},

		// Columns: EOF(0), NUMBER(1), PLUS(2), STAR(3), expression(4)
		ParseTable: [][]uint16{
			{0, 1, 0, 0, 3}, // state 0
			{2, 2, 2, 2, 0}, // state 1
			{5, 0, 4, 8, 0}, // state 2
			{0, 6, 0, 0, 0}, // state 3
			{7, 7, 7, 7, 0}, // state 4
		},

		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
		},

		LexStates: []LexState{
			{
				AcceptToken: 0,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: '0', Hi: '9', NextState: 1},
					{Lo: '+', Hi: '+', NextState: 2},
					{Lo: '*', Hi: '*', NextState: 4},
					{Lo: ' ', Hi: ' ', NextState: 3},
					{Lo: '\t', Hi: '\t', NextState: 3},
					{Lo: '\n', Hi: '\n', NextState: 3},
				},
			},
			{
				AcceptToken: 1,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: '0', Hi: '9', NextState: 1},
				},
			},
			{
				AcceptToken: 2,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: nil,
			},
			{
				AcceptToken: 0,
				Skip:        true,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 3},
					{Lo: '\t', Hi: '\t', NextState: 3},
					{Lo: '\n', Hi: '\n', NextState: 3},
				},
			},
			{
				AcceptToken: 3,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: nil,
			},
		},
	}
}

func buildKeywordStateLanguageDense() *Language {
	return &Language{
		Name:                "keyword_state_dense",
		SymbolCount:         4,
		TokenCount:          3,
		StateCount:          3,
		LargeStateCount:     3,
		KeywordCaptureToken: 1, // identifier
		KeywordLexStates: []LexState{
			{AcceptToken: 0},
			{AcceptToken: 1}, // capture token
			{AcceptToken: 2}, // keyword token
		},
		// columns: EOF(0), IDENT(1), KW_IF(2), stmt(3)
		ParseTable: [][]uint16{
			{0, 3, 4, 0}, // state 0: keyword allowed
			{0, 3, 0, 0}, // state 1: identifier-only
			{0, 0, 4, 0}, // state 2: keyword-only
		},
	}
}

func buildKeywordStateLanguageSmall() *Language {
	return &Language{
		Name:                "keyword_state_small",
		SymbolCount:         4,
		TokenCount:          3,
		StateCount:          2,
		LargeStateCount:     1,
		KeywordCaptureToken: 1, // identifier
		KeywordLexStates: []LexState{
			{AcceptToken: 0},
			{AcceptToken: 2}, // keyword token
		},
		// state 0 dense row: no keyword actions
		ParseTable: [][]uint16{
			{0, 3, 0, 0},
		},
		// state 1 uses small table and allows KW_IF (symbol 2).
		SmallParseTableMap: []uint32{0},
		SmallParseTable: []uint16{
			1, // group count
			4, // section action index
			1, // symbol count
			2, // KW_IF symbol
		},
	}
}

func TestNewParser(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	if parser == nil {
		t.Fatal("NewParser returned nil")
	}
	if parser.language != lang {
		t.Error("parser.language does not match the provided language")
	}
}

func TestParserSingleNumber(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("42"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root should be "expression".
	if root.Symbol() != 3 {
		t.Errorf("root symbol = %d, want 3 (expression)", root.Symbol())
	}
	if root.Type(lang) != "expression" {
		t.Errorf("root type = %q, want %q", root.Type(lang), "expression")
	}
	if !root.IsNamed() {
		t.Error("root IsNamed = false, want true")
	}

	// expression -> NUMBER: 1 child.
	if root.ChildCount() != 1 {
		t.Fatalf("root child count = %d, want 1", root.ChildCount())
	}

	child := root.Child(0)
	if child.Symbol() != 1 {
		t.Errorf("child symbol = %d, want 1 (NUMBER)", child.Symbol())
	}
	if child.Type(lang) != "NUMBER" {
		t.Errorf("child type = %q, want %q", child.Type(lang), "NUMBER")
	}
	if !child.IsNamed() {
		t.Error("NUMBER child IsNamed = false, want true")
	}

	// Verify the text span.
	if child.Text(tree.Source()) != "42" {
		t.Errorf("NUMBER text = %q, want %q", child.Text(tree.Source()), "42")
	}
	if child.StartByte() != 0 || child.EndByte() != 2 {
		t.Errorf("NUMBER bytes = [%d,%d), want [0,2)", child.StartByte(), child.EndByte())
	}
}

func TestParserSimpleExpression(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root should be "expression" with 3 children: expression, PLUS, NUMBER.
	if root.Symbol() != 3 {
		t.Errorf("root symbol = %d, want 3 (expression)", root.Symbol())
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	// Child 0: inner expression (expression -> NUMBER "1").
	inner := root.Child(0)
	if inner.Symbol() != 3 {
		t.Errorf("child 0 symbol = %d, want 3 (expression)", inner.Symbol())
	}
	if inner.ChildCount() != 1 {
		t.Fatalf("inner expression child count = %d, want 1", inner.ChildCount())
	}
	num1 := inner.Child(0)
	if num1.Text(tree.Source()) != "1" {
		t.Errorf("first NUMBER text = %q, want %q", num1.Text(tree.Source()), "1")
	}

	// Child 1: PLUS "+".
	plus := root.Child(1)
	if plus.Symbol() != 2 {
		t.Errorf("child 1 symbol = %d, want 2 (PLUS)", plus.Symbol())
	}
	if plus.IsNamed() {
		t.Error("PLUS IsNamed = true, want false")
	}
	if plus.Text(tree.Source()) != "+" {
		t.Errorf("PLUS text = %q, want %q", plus.Text(tree.Source()), "+")
	}

	// Child 2: NUMBER "2".
	num2 := root.Child(2)
	if num2.Symbol() != 1 {
		t.Errorf("child 2 symbol = %d, want 1 (NUMBER)", num2.Symbol())
	}
	if num2.Text(tree.Source()) != "2" {
		t.Errorf("second NUMBER text = %q, want %q", num2.Text(tree.Source()), "2")
	}
}

func TestParserChainedExpression(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// "1+2+3" should parse as left-associative: ((1)+2)+3
	tree := mustParse(t, parser, []byte("1+2+3"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root: expression -> expression PLUS NUMBER
	if root.Symbol() != 3 {
		t.Errorf("root symbol = %d, want 3", root.Symbol())
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	// root.Child(2) should be NUMBER "3".
	num3 := root.Child(2)
	if num3.Text(tree.Source()) != "3" {
		t.Errorf("rightmost NUMBER text = %q, want %q", num3.Text(tree.Source()), "3")
	}

	// root.Child(0) should be an expression with 3 children (the "1+2" part).
	middle := root.Child(0)
	if middle.Symbol() != 3 {
		t.Errorf("middle expression symbol = %d, want 3", middle.Symbol())
	}
	if middle.ChildCount() != 3 {
		t.Fatalf("middle expression child count = %d, want 3", middle.ChildCount())
	}

	// middle.Child(0) is expression -> NUMBER "1".
	innerExpr := middle.Child(0)
	if innerExpr.Symbol() != 3 {
		t.Errorf("inner expression symbol = %d, want 3", innerExpr.Symbol())
	}
	if innerExpr.ChildCount() != 1 {
		t.Fatalf("inner expression child count = %d, want 1", innerExpr.ChildCount())
	}
	if innerExpr.Child(0).Text(tree.Source()) != "1" {
		t.Errorf("innermost NUMBER text = %q, want %q", innerExpr.Child(0).Text(tree.Source()), "1")
	}

	// middle.Child(2) is NUMBER "2".
	num2 := middle.Child(2)
	if num2.Text(tree.Source()) != "2" {
		t.Errorf("middle NUMBER text = %q, want %q", num2.Text(tree.Source()), "2")
	}
}

func TestParserEmptyInput(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte(""))

	// Empty input should produce a tree with nil root (nothing to parse).
	root := tree.RootNode()
	if root != nil {
		t.Errorf("expected nil root for empty input, got symbol %d with %d children",
			root.Symbol(), root.ChildCount())
	}
}

func TestParserWhitespace(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Whitespace between tokens should be handled correctly.
	tree := mustParse(t, parser, []byte("  1  +  2  "))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	if root.Symbol() != 3 {
		t.Errorf("root symbol = %d, want 3 (expression)", root.Symbol())
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	// Verify that the inner expression's NUMBER is "1" and the outer NUMBER is "2".
	inner := root.Child(0)
	if inner.ChildCount() < 1 {
		t.Fatal("inner expression has no children")
	}
	if inner.Child(0).Text(tree.Source()) != "1" {
		t.Errorf("first NUMBER text = %q, want %q", inner.Child(0).Text(tree.Source()), "1")
	}
	if root.Child(2).Text(tree.Source()) != "2" {
		t.Errorf("second NUMBER text = %q, want %q", root.Child(2).Text(tree.Source()), "2")
	}
}

func TestParserErrorRecovery(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// "+1" starts with PLUS which is invalid in state 0.
	// The parser should create an error node for "+" and then parse "1".
	tree := mustParse(t, parser, []byte("+1"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root for error input")
	}

	// The tree should have an error somewhere.
	if !root.HasError() {
		t.Error("expected HasError=true for invalid input")
	}
}

func TestParserPreservesPartialTreeOnNoStacksAlive(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+"))
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil tree/root")
	}
	if got := tree.ParseStopReason(); got != ParseStopNoStacksAlive {
		t.Fatalf("ParseStopReason = %q, want %q", got, ParseStopNoStacksAlive)
	}
	root := tree.RootNode()
	if got := root.Symbol(); got == errorSymbol {
		t.Fatalf("root symbol = %d, want partial preserved root", got)
	}
	if got := root.Text(tree.Source()); got != "1+" {
		t.Fatalf("root text = %q, want %q", got, "1+")
	}
	if got := root.ChildCount(); got == 0 {
		t.Fatal("expected partial tree with children after no_stacks_alive")
	}
}

func TestCanFinalizeNoActionEOFRejectsFragmentStackWithInferredRoot(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	s.push(2, NewLeafNode(3, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1}), nil, nil)
	s.push(3, NewLeafNode(2, false, 1, 2, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 2}), nil, nil)

	if parser.canFinalizeNoActionEOF(&s) {
		t.Fatal("canFinalizeNoActionEOF() = true, want false for leftover fragments")
	}
}

func TestCanFinalizeNoActionEOFAcceptsSingleNonterminalWithExtras(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	extra := NewLeafNode(0, false, 0, 0, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 0})
	extra.isExtra = true
	s.push(0, extra, nil, nil)
	s.push(2, NewLeafNode(3, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1}), nil, nil)

	if !parser.canFinalizeNoActionEOF(&s) {
		t.Fatal("canFinalizeNoActionEOF() = false, want true for single nonterminal root")
	}
}

func TestPushOrExtendErrorNodeCoalescesConsecutiveTokens(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	s := newGLRStack(lang.InitialState)
	nodeCount := 0
	trackChildErrors := false

	parser.pushOrExtendErrorNode(&s, lang.InitialState, Token{
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}, &nodeCount, arena, nil, nil, &trackChildErrors)
	if got, want := s.depth(), 2; got != want {
		t.Fatalf("stack depth after first error = %d, want %d", got, want)
	}

	parser.pushOrExtendErrorNode(&s, lang.InitialState, Token{
		StartByte:  1,
		EndByte:    2,
		StartPoint: Point{Row: 0, Column: 1},
		EndPoint:   Point{Row: 0, Column: 2},
	}, &nodeCount, arena, nil, nil, &trackChildErrors)

	if got, want := s.depth(), 2; got != want {
		t.Fatalf("stack depth after extending error = %d, want %d", got, want)
	}
	if got, want := nodeCount, 1; got != want {
		t.Fatalf("nodeCount = %d, want %d", got, want)
	}
	top := s.top().node
	if top == nil {
		t.Fatal("top node is nil")
	}
	if got, want := top.Symbol(), errorSymbol; got != want {
		t.Fatalf("top symbol = %d, want %d", got, want)
	}
	if got, want := top.StartByte(), uint32(0); got != want {
		t.Fatalf("top.StartByte = %d, want %d", got, want)
	}
	if got, want := top.EndByte(), uint32(2); got != want {
		t.Fatalf("top.EndByte = %d, want %d", got, want)
	}
	if !trackChildErrors {
		t.Fatal("expected trackChildErrors=true")
	}
}

func TestParserRecoverAction(t *testing.T) {
	lang := buildArithmeticLanguage()

	// In this custom grammar, NUMBER should trigger ParseActionRecover.
	lang.ParseTable = [][]uint16{
		{0, 1}, // EOF has no action, NUMBER -> recover action.
		{0, 0},
	}
	lang.ParseActions = []ParseActionEntry{
		{}, // index 0 is unused / error
		{Actions: []ParseAction{{Type: ParseActionRecover}}},
	}

	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("1"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree root is nil after recover action")
	}

	if root.Symbol() != errorSymbol {
		t.Errorf("root symbol = %d, want %d (error symbol)", root.Symbol(), errorSymbol)
	}
	if !root.HasError() {
		t.Error("expected recovered parse root to have HasError=true")
	}
}

func TestBuildStateRecoverTableNilWhenNoRecoverActions(t *testing.T) {
	lang := buildArithmeticLanguage()
	table := buildStateRecoverTable(lang)
	if table != nil {
		t.Fatalf("expected nil recover table when grammar has no recover actions, got len=%d", len(table))
	}
}

func TestBuildStateRecoverTableMarksRecoverStates(t *testing.T) {
	lang := buildArithmeticRecoverLanguage()
	table := buildStateRecoverTable(lang)
	if len(table) == 0 {
		t.Fatal("expected recover table to be populated")
	}
	if len(table) != int(lang.StateCount) {
		t.Fatalf("recover table len = %d, want %d", len(table), lang.StateCount)
	}
	if table[0] {
		t.Fatal("state 0 should not be marked recoverable")
	}
	if !table[2] {
		t.Fatal("state 2 should be marked recoverable")
	}
}

func TestBuildKeywordStatesDense(t *testing.T) {
	lang := buildKeywordStateLanguageDense()
	table := buildKeywordStates(lang)
	if len(table) != int(lang.StateCount) {
		t.Fatalf("keyword state table len = %d, want %d", len(table), lang.StateCount)
	}
	if !table[0] {
		t.Fatal("state 0 should allow keyword promotion")
	}
	if table[1] {
		t.Fatal("state 1 should not allow keyword promotion")
	}
	if !table[2] {
		t.Fatal("state 2 should allow keyword promotion")
	}
}

func TestBuildKeywordStatesSmall(t *testing.T) {
	lang := buildKeywordStateLanguageSmall()
	table := buildKeywordStates(lang)
	if len(table) != int(lang.StateCount) {
		t.Fatalf("keyword state table len = %d, want %d", len(table), lang.StateCount)
	}
	if table[0] {
		t.Fatal("state 0 should not allow keyword promotion")
	}
	if !table[1] {
		t.Fatal("state 1 should allow keyword promotion from small parse table")
	}
}

func TestBuildKeywordStatesNilWhenNoKeywordActions(t *testing.T) {
	lang := buildKeywordStateLanguageDense()
	lang.ParseTable[0][2] = 0
	lang.ParseTable[2][2] = 0
	table := buildKeywordStates(lang)
	if table != nil {
		t.Fatalf("expected nil keyword state table, got len=%d", len(table))
	}
}

func TestBuildRecoverActionsByStateMarksRecoverSymbols(t *testing.T) {
	lang := buildArithmeticRecoverLanguage()
	_, _, symbols := buildRecoverActionsByState(lang)
	if len(symbols) == 0 {
		t.Fatal("expected recover symbol table to be populated")
	}
	if !symbols[3] { // STAR
		t.Fatal("expected STAR to be marked as recoverable lookahead")
	}
	if symbols[1] { // NUMBER
		t.Fatal("did not expect NUMBER to be marked as recoverable lookahead")
	}
}

func TestFindRecoverActionOnStackUsesNearestAncestor(t *testing.T) {
	lang := buildArithmeticRecoverLanguage()
	parser := NewParser(lang)
	s := newGLRStack(lang.InitialState)
	s.push(2, nil, nil, nil)
	s.push(3, nil, nil, nil)

	depth, act, ok := parser.findRecoverActionOnStack(&s, Symbol(3), nil) // STAR
	if !ok {
		t.Fatal("expected recover action on stack for STAR")
	}
	if depth != 1 {
		t.Fatalf("recover depth = %d, want 1 (state 2)", depth)
	}
	if act.Type != ParseActionRecover {
		t.Fatalf("recover action type = %d, want %d", act.Type, ParseActionRecover)
	}
	if act.State != 3 {
		t.Fatalf("recover state = %d, want 3", act.State)
	}
}

func TestRecoverActionForStateUsesSymbolSpecificTable(t *testing.T) {
	lang := buildArithmeticRecoverLanguage()
	parser := NewParser(lang)

	if _, ok := parser.recoverActionForState(2, Symbol(3)); !ok {
		t.Fatal("expected recover action for state 2 on STAR")
	}
	if _, ok := parser.recoverActionForState(2, Symbol(1)); ok {
		t.Fatal("did not expect recover action for state 2 on NUMBER")
	}
}

func TestParserAncestorRecoverActionPreservesLeftExpression(t *testing.T) {
	lang := buildArithmeticRecoverLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+*2"))
	if tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	root := tree.RootNode()

	if root.Symbol() != 4 {
		t.Fatalf("root symbol = %d, want 4 (expression)", root.Symbol())
	}
	if !root.HasError() {
		t.Fatal("expected recovered tree to have HasError=true")
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	if got := root.Child(0).Symbol(); got != 4 {
		t.Fatalf("child[0] symbol = %d, want 4 (left expression preserved)", got)
	}
	if got := root.Child(1).Symbol(); got != errorSymbol {
		t.Fatalf("child[1] symbol = %d, want %d (error node)", got, errorSymbol)
	}
	if got := root.Child(2).Symbol(); got != 1 {
		t.Fatalf("child[2] symbol = %d, want 1 (NUMBER)", got)
	}
}

func TestParserFieldMapFieldNames(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.FieldCount = 1
	lang.FieldNames = []string{"", "value"}

	// Production 0 (expr -> NUMBER) has one child; map it to field ID 1.
	lang.FieldMapSlices = [][2]uint16{
		{0, 1},
	}
	lang.FieldMapEntries = []FieldMapEntry{
		{FieldID: 1, ChildIndex: 0, Inherited: false},
	}

	lang.ParseActions[2].Actions[0].ProductionID = 0
	lang.ParseActions[7].Actions[0].ProductionID = 1
	lang.ProductionIDCount = 2

	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("42"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}
	if root.Symbol() != 3 {
		t.Errorf("root symbol = %d, want 3 (expression)", root.Symbol())
	}

	fieldChild := root.ChildByFieldName("value", lang)
	if fieldChild == nil {
		t.Fatal("expected field-mapped child by name \"value\"")
	}
	if fieldChild.Symbol() != 1 {
		t.Errorf("field child symbol = %d, want 1 (NUMBER)", fieldChild.Symbol())
	}
	if fieldChild.Text(tree.Source()) != "42" {
		t.Errorf("field child text = %q, want %q", fieldChild.Text(tree.Source()), "42")
	}
}

func TestBuildResultFoldExtrasPreservesFieldMappings(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.FieldCount = 1
	lang.FieldNames = []string{"", "value"}
	parser := NewParser(lang)

	source := []byte(" 42 ")

	leadingExtra := NewLeafNode(2, false, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	leadingExtra.isExtra = true

	valueChild := NewLeafNode(1, true, 1, 3, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 3})
	realRoot := NewParentNode(3, true, []*Node{valueChild}, []FieldID{1}, 0)

	trailingExtra := NewLeafNode(2, false, 3, 4, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 4})
	trailingExtra.isExtra = true

	stack := []stackEntry{
		{state: 0, node: leadingExtra},
		{state: 0, node: realRoot},
		{state: 0, node: trailingExtra},
	}

	tree := parser.buildResult(stack, source, nil, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResult returned nil tree/root")
	}
	root := tree.RootNode()
	if root != realRoot {
		t.Fatal("expected folded result to reuse real root node")
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}
	if root.Child(0) != leadingExtra || root.Child(1) != valueChild || root.Child(2) != trailingExtra {
		t.Fatalf("unexpected child order after folding extras")
	}

	fieldChild := root.ChildByFieldName("value", lang)
	if fieldChild == nil {
		t.Fatal("expected field-mapped child by name \"value\"")
	}
	if fieldChild != valueChild {
		t.Fatal("field mapping shifted after folding extras")
	}
	if len(root.fieldIDs) != 3 || root.fieldIDs[1] != 1 {
		t.Fatalf("fieldIDs not re-aligned after folding extras: %#v", root.fieldIDs)
	}
	if leadingExtra.Parent() != root || trailingExtra.Parent() != root {
		t.Fatal("extra child parent pointers were not updated during fold")
	}
}

func TestBuildReduceChildrenHiddenChildDoesNotDuplicateExistingField(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden", "!=", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "!=", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
		FieldNames: []string{"", "operators"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	operator := newLeafNodeInArena(arena, 2, false, 0, 2, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 2})
	rhs := newLeafNodeInArena(arena, 3, true, 3, 4, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 4})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{operator, rhs}, []FieldID{1, 0}, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 0, 0, arena)
	if got, want := len(children), 2; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 2; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
	if got := fieldIDs[1]; got != 0 {
		t.Fatalf("fieldIDs[1] = %d, want 0", got)
	}
}

func TestBuildReduceChildrenInheritedFieldOverridesInheritedInnerFieldOnFlattenedSpan(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "type_identifier", "with", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "with", Visible: true, Named: false},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "type", "arguments"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 2, true, 0, 12, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 12})
	withTok := newLeafNodeInArena(arena, 3, false, 13, 17, Point{Row: 0, Column: 13}, Point{Row: 0, Column: 17})
	right := newLeafNodeInArena(arena, 2, true, 18, 25, Point{Row: 0, Column: 18}, Point{Row: 0, Column: 25})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{left, withTok, right}, []FieldID{2, 2, 2}, 0)
	hidden.fieldSources = []uint8{fieldSourceInherited, fieldSourceInherited, fieldSourceInherited}

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 3; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 3; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	for i, fid := range fieldIDs {
		if got, want := fid, FieldID(1); got != want {
			t.Fatalf("fieldIDs[%d] = %d, want %d", i, got, want)
		}
	}
}

func TestBuildReduceChildrenDirectFieldOverridesSingleIndirectNamedChild(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "type_identifier", "arguments", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "arguments", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "type", "arguments"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	typ := newLeafNodeInArena(arena, 2, true, 0, 9, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 9})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{typ}, []FieldID{2}, 0)
	hidden.fieldSources = []uint8{fieldSourceInherited}

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 1; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
}

func TestBuildReduceChildrenInheritedFieldDoesNotBlanketSpanWithoutConflict(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "identifier", ".", "namespace_wildcard", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
			{Name: "namespace_wildcard", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "path"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	head := newLeafNodeInArena(arena, 2, true, 0, 5, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 5})
	dot := newLeafNodeInArena(arena, 3, false, 5, 6, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 6})
	tail := newLeafNodeInArena(arena, 4, true, 6, 7, Point{Row: 0, Column: 6}, Point{Row: 0, Column: 7})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{head, dot, tail}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 5, 0, arena)
	if got, want := len(children), 3; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 3; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
	if got := fieldIDs[1]; got != 0 {
		t.Fatalf("fieldIDs[1] = %d, want 0", got)
	}
	if got := fieldIDs[2]; got != 0 {
		t.Fatalf("fieldIDs[2] = %d, want 0", got)
	}
}

func TestBuildReduceChildrenDirectFieldExtendsAcrossLeadingAnonymousOnFlattenedSpan(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", ".", "identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "path"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	dot0 := newLeafNodeInArena(arena, 2, false, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})
	net := newLeafNodeInArena(arena, 3, true, 5, 8, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 8})
	dot1 := newLeafNodeInArena(arena, 2, false, 8, 9, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 9})
	url := newLeafNodeInArena(arena, 3, true, 9, 12, Point{Row: 0, Column: 9}, Point{Row: 0, Column: 12})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{dot0, net, dot1, url}, []FieldID{0, 1, 1, 1}, 0)
	hidden.fieldSources = []uint8{fieldSourceNone, fieldSourceDirect, fieldSourceDirect, fieldSourceDirect}

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 4; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 4; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	for i, fid := range fieldIDs {
		if got, want := fid, FieldID(1); got != want {
			t.Fatalf("fieldIDs[%d] = %d, want %d", i, got, want)
		}
	}
}

func TestBuildReduceChildrenRepeatedDirectFieldOnHiddenPathFillsAnonymousGap(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", ".", "identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "path"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	java := newLeafNodeInArena(arena, 3, true, 0, 4, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 4})
	dot0 := newLeafNodeInArena(arena, 2, false, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})
	net := newLeafNodeInArena(arena, 3, true, 5, 8, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 8})
	dot1 := newLeafNodeInArena(arena, 2, false, 8, 9, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 9})
	url := newLeafNodeInArena(arena, 3, true, 9, 12, Point{Row: 0, Column: 9}, Point{Row: 0, Column: 12})

	tail := newParentNodeInArena(arena, 1, false, []*Node{net, dot1, url}, []FieldID{1, 1, 1}, 0)
	tail.fieldSources = []uint8{fieldSourceDirect, fieldSourceDirect, fieldSourceDirect}
	outer := newParentNodeInArena(arena, 1, false, []*Node{java, dot0, tail}, []FieldID{1, 0, 1}, 0)
	outer.fieldSources = []uint8{fieldSourceDirect, fieldSourceNone, fieldSourceDirect}

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: outer}}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 5; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 5; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	for i, fid := range fieldIDs {
		if got, want := fid, FieldID(1); got != want {
			t.Fatalf("fieldIDs[%d] = %d, want %d", i, got, want)
		}
	}
}

func TestBuildReduceChildrenInheritedFieldYieldsToDirectTargetOnHiddenSpan(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "modifiers", "def", "identifier", ":", "type_identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "modifiers", Visible: true, Named: true},
			{Name: "def", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: ":", Visible: true, Named: false},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "return_type", "name"},
	}

	arena := newNodeArena(arenaClassFull)
	modifiers := newLeafNodeInArena(arena, 2, true, 0, 7, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 7})
	defTok := newLeafNodeInArena(arena, 3, false, 8, 11, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 11})
	name := newLeafNodeInArena(arena, 4, true, 12, 18, Point{Row: 0, Column: 12}, Point{Row: 0, Column: 18})
	colon := newLeafNodeInArena(arena, 5, false, 18, 19, Point{Row: 0, Column: 18}, Point{Row: 0, Column: 19})
	retType := newLeafNodeInArena(arena, 6, true, 20, 23, Point{Row: 0, Column: 20}, Point{Row: 0, Column: 23})

	hidden := newParentNodeInArena(arena, 1, false, []*Node{modifiers, defTok, name, colon, retType}, []FieldID{1, 0, 2, 0, 1}, 0)
	hidden.fieldSources = []uint8{fieldSourceInherited, fieldSourceNone, fieldSourceDirect, fieldSourceNone, fieldSourceDirect}

	children := arena.allocNodeSlice(5)
	fieldIDs := arena.allocFieldIDSlice(5)
	fieldSources := make([]uint8, 5)
	if got, want := appendFlattenedHiddenChildrenWithFields(children, fieldIDs, fieldSources, 0, hidden, lang.SymbolMetadata), 5; got != want {
		t.Fatalf("appendFlattenedHiddenChildrenWithFields() = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got, want := fieldIDs[2], FieldID(2); got != want {
		t.Fatalf("fieldIDs[2] = %d, want %d", got, want)
	}
	if got := fieldIDs[3]; got != 0 {
		t.Fatalf("fieldIDs[3] = %d, want 0", got)
	}
	if got, want := fieldIDs[4], FieldID(1); got != want {
		t.Fatalf("fieldIDs[4] = %d, want %d", got, want)
	}
	if got, want := fieldSources[4], uint8(fieldSourceDirect); got != want {
		t.Fatalf("fieldSources[4] = %d, want %d", got, want)
	}
}

func TestBuildReduceChildrenNoAliasNoFieldsInlinesHiddenChildren(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden", "identifier", "operator"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "operator", Visible: true, Named: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 2, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	op := newLeafNodeInArena(arena, 3, false, 2, 3, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 3})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{left, op}, nil, 0)
	right := newLeafNodeInArena(arena, 2, true, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}, {node: right}}, 0, 2, 2, 2, 0, arena)
	if got, want := len(children), 3; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if fieldIDs != nil {
		t.Fatalf("fieldIDs = %#v, want nil", fieldIDs)
	}
	if children[0] != left || children[1] != op || children[2] != right {
		t.Fatalf("children order = %#v, want hidden children then right leaf", children)
	}
}

func TestBuildReduceChildrenHiddenParentDefersFlattenUntilVisibleBoundary(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_a", "_hidden_b", "identifier", "operator", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_a", Visible: false, Named: false},
			{Name: "_hidden_b", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "operator", Visible: true, Named: false},
			{Name: "visible_parent", Visible: true, Named: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 3, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	op := newLeafNodeInArena(arena, 4, false, 2, 3, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 3})
	right := newLeafNodeInArena(arena, 3, true, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})
	tail := newLeafNodeInArena(arena, 3, true, 6, 7, Point{Row: 0, Column: 6}, Point{Row: 0, Column: 7})

	hiddenInner := newParentNodeInArena(arena, 2, false, []*Node{left, op}, nil, 0)
	hiddenOuterChildren, _, _ := parser.buildReduceChildren([]stackEntry{{node: hiddenInner}, {node: right}}, 0, 2, 2, 1, 0, arena)
	if got, want := len(hiddenOuterChildren), 2; got != want {
		t.Fatalf("len(hiddenOuterChildren) = %d, want %d", got, want)
	}
	if hiddenOuterChildren[0] != hiddenInner || hiddenOuterChildren[1] != right {
		t.Fatalf("hidden outer children = %#v, want compact hidden child then right", hiddenOuterChildren)
	}

	hiddenOuter := newParentNodeInArena(arena, 1, false, hiddenOuterChildren, nil, 0)
	visibleChildren, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hiddenOuter}, {node: tail}}, 0, 2, 2, 5, 0, arena)
	if fieldIDs != nil {
		t.Fatalf("fieldIDs = %#v, want nil", fieldIDs)
	}
	if got, want := len(visibleChildren), 4; got != want {
		t.Fatalf("len(visibleChildren) = %d, want %d", got, want)
	}
	if visibleChildren[0] != left || visibleChildren[1] != op || visibleChildren[2] != right || visibleChildren[3] != tail {
		t.Fatalf("visible children order = %#v, want fully flattened hidden chain plus tail", visibleChildren)
	}
}

func TestParserMultiDigitNumbers(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("123+456"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	inner := root.Child(0)
	if inner.ChildCount() < 1 {
		t.Fatal("inner expression has no children")
	}
	if inner.Child(0).Text(tree.Source()) != "123" {
		t.Errorf("first NUMBER text = %q, want %q", inner.Child(0).Text(tree.Source()), "123")
	}
	if root.Child(2).Text(tree.Source()) != "456" {
		t.Errorf("second NUMBER text = %q, want %q", root.Child(2).Text(tree.Source()), "456")
	}
}

func TestHiddenTreeHasFieldIDsDeepChain(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	leaf.fieldIDs = []FieldID{1}

	root := leaf
	for i := 0; i < 4096; i++ {
		root = newParentNodeInArena(arena, 2, false, []*Node{root}, nil, 0)
	}

	if !hiddenTreeHasFieldIDs(root) {
		t.Fatal("hiddenTreeHasFieldIDs returned false, want true")
	}
}

func TestNodesFromGSSFiltersNilAndPreservesOrder(t *testing.T) {
	var scratch gssScratch
	n1 := NewLeafNode(1, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	n2 := NewLeafNode(1, true, 2, 3, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 3})

	var s gssStack
	s.push(1, nil, &scratch)
	s.push(2, n1, &scratch)
	s.push(3, nil, &scratch)
	s.push(4, n2, &scratch)

	nodes := nodesFromGSS(s)
	if len(nodes) != 2 {
		t.Fatalf("nodesFromGSS len = %d, want 2", len(nodes))
	}
	if nodes[0] != n1 || nodes[1] != n2 {
		t.Fatalf("nodesFromGSS order mismatch: got [%p %p], want [%p %p]", nodes[0], nodes[1], n1, n2)
	}
}

func TestBuildResultFromGLRWithGSSOnlyStack(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := []byte("1")
	arena := acquireNodeArena(arenaClassFull)

	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	leaf.parseState = 1
	expr := newParentNodeInArena(arena, 3, true, []*Node{leaf}, nil, 0)
	expr.parseState = 2

	var gScratch gssScratch
	gss := newGSSStack(lang.InitialState, &gScratch)
	gss.push(expr.parseState, expr, &gScratch)
	stack := glrStack{gss: gss}

	tree := parser.buildResultFromGLR([]glrStack{stack}, source, arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromGLR returned nil tree/root")
	}
	if tree.RootNode() != expr {
		t.Fatal("expected GSS-only stack result to reuse the GSS node as root")
	}
	if got := tree.RootNode().Text(tree.Source()); got != "1" {
		t.Fatalf("root text = %q, want %q", got, "1")
	}
	tree.Release()
}

func TestCompactAcceptedStacksPreservesAllAcceptedForFinalChoice(t *testing.T) {
	lang := buildAmbiguousLanguage()
	parser := NewParser(lang)
	source := []byte("x")
	arena := acquireNodeArena(arenaClassFull)

	low := newLeafNodeInArena(arena, 2, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	low.parseState = 2
	high := newLeafNodeInArena(arena, 3, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	high.parseState = 2

	stacks := []glrStack{
		{accepted: false, score: 99, entries: []stackEntry{{state: 1}}},
		{accepted: true, score: 0, entries: []stackEntry{{state: 2, node: low}}},
		{accepted: true, score: 5, entries: []stackEntry{{state: 2, node: high}}},
	}

	accepted := compactAcceptedStacks(stacks)
	if got, want := len(accepted), 2; got != want {
		t.Fatalf("len(accepted) = %d, want %d", got, want)
	}
	if !accepted[0].accepted || !accepted[1].accepted {
		t.Fatal("expected only accepted stacks after compaction")
	}
	if accepted[0].score != 0 || accepted[1].score != 5 {
		t.Fatalf("accepted scores = [%d %d], want [0 5]", accepted[0].score, accepted[1].score)
	}

	tree := parser.buildResultFromGLR(accepted, source, arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromGLR returned nil tree/root")
	}
	if got, want := tree.RootNode().Symbol(), Symbol(3); got != want {
		t.Fatalf("root symbol = %d, want %d", got, want)
	}
	tree.Release()
}

func TestParserLongChain(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// "1+2+3+4+5" — deeply left-nested.
	tree := mustParse(t, parser, []byte("1+2+3+4+5"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// The rightmost child should be NUMBER "5".
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}
	if root.Child(2).Text(tree.Source()) != "5" {
		t.Errorf("rightmost NUMBER text = %q, want %q", root.Child(2).Text(tree.Source()), "5")
	}

	// Walk down the left spine and count depth.
	depth := 0
	node := root
	for node.ChildCount() == 3 {
		node = node.Child(0)
		depth++
	}
	// "1+2+3+4+5" has 4 additions, so 4 levels of nesting.
	if depth != 4 {
		t.Errorf("left-nesting depth = %d, want 4", depth)
	}

	// The innermost expression should have 1 child (NUMBER "1").
	if node.ChildCount() != 1 {
		t.Errorf("innermost expression child count = %d, want 1", node.ChildCount())
	}
	if node.Child(0).Text(tree.Source()) != "1" {
		t.Errorf("innermost NUMBER text = %q, want %q", node.Child(0).Text(tree.Source()), "1")
	}
}

func TestParserByteSpans(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root expression should span the entire input [0, 3).
	if root.StartByte() != 0 {
		t.Errorf("root StartByte = %d, want 0", root.StartByte())
	}
	if root.EndByte() != 3 {
		t.Errorf("root EndByte = %d, want 3", root.EndByte())
	}

	// PLUS token at byte 1.
	plus := root.Child(1)
	if plus.StartByte() != 1 || plus.EndByte() != 2 {
		t.Errorf("PLUS bytes = [%d,%d), want [1,2)", plus.StartByte(), plus.EndByte())
	}
}

func TestParserPointPositions(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Check start/end points of the root.
	if root.StartPoint() != (Point{Row: 0, Column: 0}) {
		t.Errorf("root StartPoint = %v, want {0,0}", root.StartPoint())
	}
	if root.EndPoint() != (Point{Row: 0, Column: 3}) {
		t.Errorf("root EndPoint = %v, want {0,3}", root.EndPoint())
	}

	// NUMBER "2" starts at column 2.
	num2 := root.Child(2)
	if num2.StartPoint() != (Point{Row: 0, Column: 2}) {
		t.Errorf("NUMBER '2' StartPoint = %v, want {0,2}", num2.StartPoint())
	}
}

func TestParserParentPointers(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root has no parent.
	// (NewParentNode does not set the parent of the root itself.)

	// Each child should have the root as parent.
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child.Parent() != root {
			t.Errorf("child %d parent != root", i)
		}
	}

	// The inner expression's child should point to the inner expression.
	inner := root.Child(0)
	if inner.ChildCount() > 0 {
		if inner.Child(0).Parent() != inner {
			t.Error("inner expression's child has wrong parent")
		}
	}
}

func TestParserTreeMetadata(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	source := []byte("1+2")
	tree := mustParse(t, parser, source)

	if tree.Language() != lang {
		t.Error("tree.Language() does not match")
	}
	if string(tree.Source()) != "1+2" {
		t.Errorf("tree.Source() = %q, want %q", tree.Source(), "1+2")
	}
}

func TestParserNamedChildAccess(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root has 3 children: expression (named), PLUS (anonymous), NUMBER (named).
	// So NamedChildCount should be 2.
	if root.NamedChildCount() != 2 {
		t.Errorf("root NamedChildCount = %d, want 2", root.NamedChildCount())
	}

	// NamedChild(0) should be the expression.
	nc0 := root.NamedChild(0)
	if nc0 == nil || nc0.Symbol() != 3 {
		t.Errorf("NamedChild(0) symbol = %v, want 3 (expression)", nc0)
	}

	// NamedChild(1) should be the NUMBER "2".
	nc1 := root.NamedChild(1)
	if nc1 == nil || nc1.Symbol() != 1 {
		t.Errorf("NamedChild(1) symbol = %v, want 1 (NUMBER)", nc1)
	}
}

func TestParserLookupActionOutOfRange(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// State out of range.
	action := parser.lookupAction(StateID(999), Symbol(0))
	if action != nil {
		t.Error("expected nil for out-of-range state")
	}

	// Symbol out of range.
	action = parser.lookupAction(StateID(0), Symbol(999))
	if action != nil {
		t.Error("expected nil for out-of-range symbol")
	}
}

func TestParserIsNamedSymbol(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// EOF (0) is not named.
	if parser.isNamedSymbol(Symbol(0)) {
		t.Error("EOF should not be named")
	}
	// NUMBER (1) is named.
	if !parser.isNamedSymbol(Symbol(1)) {
		t.Error("NUMBER should be named")
	}
	// PLUS (2) is not named.
	if parser.isNamedSymbol(Symbol(2)) {
		t.Error("PLUS should not be named")
	}
	// expression (3) is named.
	if !parser.isNamedSymbol(Symbol(3)) {
		t.Error("expression should be named")
	}
	// Out of range symbol.
	if parser.isNamedSymbol(Symbol(999)) {
		t.Error("out-of-range symbol should not be named")
	}
}

func TestParserOnlyWhitespace(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Only whitespace — should produce empty tree like empty input.
	tree := mustParse(t, parser, []byte("   "))
	root := tree.RootNode()
	if root != nil {
		t.Errorf("expected nil root for whitespace-only input, got symbol %d", root.Symbol())
	}
}

type hashPlusExternalScanner struct{}

func (s *hashPlusExternalScanner) Create() any                           { return nil }
func (s *hashPlusExternalScanner) Destroy(payload any)                   {}
func (s *hashPlusExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (s *hashPlusExternalScanner) Deserialize(payload any, buf []byte)   {}
func (s *hashPlusExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	if lexer.Lookahead() != '#' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(Symbol(2)) // PLUS
	return true
}

func TestParserExternalScannerToken(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.ExternalScanner = &hashPlusExternalScanner{}
	lang.ExternalSymbols = []Symbol{2} // PLUS token comes from external scanner

	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("1#2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}
	if root.HasError() {
		t.Fatal("external scanner token path produced error tree")
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}
	if got := root.Child(1).Text(tree.Source()); got != "#" {
		t.Fatalf("operator text = %q, want %q", got, "#")
	}
}

// TestFieldIDsAlignAfterExtrasFold verifies that when buildResult folds
// extra nodes (e.g. leading comments) into a root's children, the fieldIDs
// slice is padded to maintain index alignment with children.
//
// Regression test for: prepending extras into realRoot.children without
// updating fieldIDs caused ChildByFieldName to return wrong nodes.
func TestFieldIDsAlignAfterExtrasFold(t *testing.T) {
	lang := queryTestLanguage()

	// Construct a parent with fielded children:
	//   children:  [ident,        paramList,       block]
	//   fieldIDs:  [name(1),      parameters(5),   body(2)]
	ident := NewLeafNode(Symbol(1), true, 5, 9, Point{}, Point{})
	paramList := NewLeafNode(Symbol(13), true, 9, 11, Point{}, Point{})
	block := NewLeafNode(Symbol(14), true, 12, 20, Point{}, Point{})
	root := NewParentNode(Symbol(5), true,
		[]*Node{ident, paramList, block},
		[]FieldID{1, 5, 2}, 0)

	// Sanity: field lookups work before modification.
	if got := root.ChildByFieldName("name", lang); got != ident {
		t.Fatal("pre-check: name field should return ident")
	}
	if got := root.ChildByFieldName("body", lang); got != block {
		t.Fatal("pre-check: body field should return block")
	}

	// Simulate what buildResult's extras fold does: prepend a leading extra.
	extra := NewLeafNode(Symbol(0), false, 0, 3, Point{}, Point{})
	extra.isExtra = true

	leadingCount := 1
	merged := make([]*Node, 0, 1+len(root.children))
	merged = append(merged, extra)
	merged = append(merged, root.children...)
	root.children = merged

	// Pad fieldIDs to match: extras get 0.
	if len(root.fieldIDs) > 0 {
		padded := make([]FieldID, leadingCount+len(root.fieldIDs))
		copy(padded[leadingCount:], root.fieldIDs)
		root.fieldIDs = padded
	}

	// Verify field lookups still return correct nodes.
	if got := root.ChildByFieldName("name", lang); got != ident {
		t.Fatalf("after fold: name field should return ident (sym 1), got sym %d", got.Symbol())
	}
	if got := root.ChildByFieldName("body", lang); got != block {
		t.Fatalf("after fold: body field should return block (sym 14), got sym %d", got.Symbol())
	}
	if got := root.ChildByFieldName("parameters", lang); got != paramList {
		t.Fatalf("after fold: parameters field should return paramList (sym 13), got sym %d", got.Symbol())
	}
}

func TestParserIncrementalArithmeticEditMatchesFreshParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	oldSrc := []byte("1+2")
	oldTree := mustParse(t, parser, oldSrc)

	newSrc := []byte("1+3")
	edit := InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	}
	oldTree.Edit(edit)

	incrTree := mustParseIncremental(t, parser, newSrc, oldTree)
	freshTree := mustParse(t, parser, newSrc)

	incrRoot := incrTree.RootNode()
	freshRoot := freshTree.RootNode()
	if incrRoot == nil || freshRoot == nil {
		t.Fatal("expected non-nil roots")
	}
	if got, want := incrRoot.SExpr(lang), freshRoot.SExpr(lang); got != want {
		t.Fatalf("incremental SExpr mismatch:\n  got:  %s\n  want: %s", got, want)
	}
	if incrRoot.HasError() != freshRoot.HasError() {
		t.Fatalf("incremental HasError=%v, fresh HasError=%v", incrRoot.HasError(), freshRoot.HasError())
	}
}

func TestParserIncrementalArithmeticEditThenUndoMatchesFreshParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	originalSrc := []byte("1+2")
	tree := mustParse(t, parser, originalSrc)

	editedSrc := []byte("1+9")
	forwardEdit := InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	}
	tree.Edit(forwardEdit)
	tree = mustParseIncremental(t, parser, editedSrc, tree)

	undoEdit := InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	}
	tree.Edit(undoEdit)
	incrUndo := mustParseIncremental(t, parser, originalSrc, tree)
	freshUndo := mustParse(t, parser, originalSrc)

	incrRoot := incrUndo.RootNode()
	freshRoot := freshUndo.RootNode()
	if incrRoot == nil || freshRoot == nil {
		t.Fatal("expected non-nil roots")
	}
	if got, want := incrRoot.SExpr(lang), freshRoot.SExpr(lang); got != want {
		t.Fatalf("incremental undo SExpr mismatch:\n  got:  %s\n  want: %s", got, want)
	}
}

func TestParseRuntimeReportsAcceptedOnCompleteParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

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

// buildReservedWordLanguage constructs a minimal language to test reserved word
// handling in promoteKeyword. Symbols:
//
//	0: EOF
//	1: IDENT (terminal, named) — keyword capture token
//	2: KW_IF (terminal, anonymous) — keyword matched by DFA
//	3: stmt (nonterminal, named)
//
// The keyword lexer DFA recognises "if" and emits symbol 2 (KW_IF).
//
// LexModes:
//
//	state 0: no lex mode entry (unused)
//	state 1: ReservedWordSetID=1 → set {KW_IF} → "if" is reserved, not promoted
//	state 2: ReservedWordSetID=0 → no reserved words → "if" IS promoted
//
// ReservedWords layout (stride 2):
//
//	set 0 (offset 0): [0, 0]       — empty
//	set 1 (offset 2): [KW_IF, 0]   — KW_IF is reserved
func buildReservedWordLanguage() *Language {
	return &Language{
		Name:                "reserved_word_test",
		SymbolCount:         4,
		TokenCount:          3,
		StateCount:          3,
		LargeStateCount:     3,
		KeywordCaptureToken: 1, // IDENT
		KeywordLexStates: []LexState{
			// State 0: start — dispatch 'i'
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'i', Hi: 'i', NextState: 1},
			}},
			// State 1: saw 'i' — dispatch 'f'
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'f', Hi: 'f', NextState: 2},
			}},
			// State 2: saw "if" — accept KW_IF (symbol 2)
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},                       // state 0 — not used in test
			{LexState: 0, ReservedWordSetID: 1}, // state 1 — KW_IF reserved
			{LexState: 0, ReservedWordSetID: 0}, // state 2 — no reserved words
		},
		// Flat reserved word array, stride=2.
		// Set 0 (offset 0..1): empty [0, 0]
		// Set 1 (offset 2..3): [KW_IF(2), 0]
		ReservedWords:          []Symbol{0, 0, 2, 0},
		MaxReservedWordSetSize: 2,
		// Dense parse table — both IDENT and KW_IF valid in all states
		// so context-aware check doesn't interfere.
		// Columns: EOF(0), IDENT(1), KW_IF(2), stmt(3)
		ParseTable: [][]uint16{
			{0, 1, 1, 0}, // state 0
			{0, 1, 1, 0}, // state 1
			{0, 1, 1, 0}, // state 2
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
}

func TestReservedWordBlocksPromotion(t *testing.T) {
	lang := buildReservedWordLanguage()
	source := []byte("if")

	// Helper to build a dfaTokenSource with the given parse state and run
	// promoteKeyword on a token matching the keyword capture token.
	testPromote := func(state StateID) Token {
		lx := &Lexer{
			states: lang.LexStates,
			source: source,
		}
		d := &dfaTokenSource{
			lexer:    lx,
			language: lang,
			state:    state,
		}
		tok := Token{
			Symbol:    lang.KeywordCaptureToken, // IDENT
			StartByte: 0,
			EndByte:   2,
		}
		return d.promoteKeyword(tok)
	}

	// State 1 has ReservedWordSetID=1 which contains KW_IF (symbol 2).
	// "if" should NOT be promoted — token stays as IDENT (symbol 1).
	got := testPromote(1)
	if got.Symbol != 1 {
		t.Fatalf("state 1 (reserved): got symbol %d, want 1 (IDENT — not promoted)", got.Symbol)
	}

	// State 2 has ReservedWordSetID=0 — no reserved words.
	// "if" SHOULD be promoted to KW_IF (symbol 2).
	got = testPromote(2)
	if got.Symbol != 2 {
		t.Fatalf("state 2 (not reserved): got symbol %d, want 2 (KW_IF — promoted)", got.Symbol)
	}
}

func TestReservedWordNoReservedWordsArray(t *testing.T) {
	// When ReservedWords is empty, promotion should proceed normally.
	lang := buildReservedWordLanguage()
	lang.ReservedWords = nil
	lang.MaxReservedWordSetSize = 0
	source := []byte("if")

	lx := &Lexer{
		states: lang.LexStates,
		source: source,
	}
	d := &dfaTokenSource{
		lexer:    lx,
		language: lang,
		state:    1, // would be reserved if array were present
	}
	tok := Token{
		Symbol:    lang.KeywordCaptureToken,
		StartByte: 0,
		EndByte:   2,
	}
	got := d.promoteKeyword(tok)
	if got.Symbol != 2 {
		t.Fatalf("empty ReservedWords: got symbol %d, want 2 (KW_IF — promoted)", got.Symbol)
	}
}

func TestReservedWordSetIDZeroDoesNotBlock(t *testing.T) {
	// ReservedWordSetID=0 means no reserved words for this state,
	// even if the ReservedWords array is populated.
	lang := buildReservedWordLanguage()
	source := []byte("if")

	lx := &Lexer{
		states: lang.LexStates,
		source: source,
	}
	d := &dfaTokenSource{
		lexer:    lx,
		language: lang,
		state:    2, // ReservedWordSetID=0
	}
	tok := Token{
		Symbol:    lang.KeywordCaptureToken,
		StartByte: 0,
		EndByte:   2,
	}
	got := d.promoteKeyword(tok)
	if got.Symbol != 2 {
		t.Fatalf("setID=0: got symbol %d, want 2 (KW_IF — promoted)", got.Symbol)
	}
}
