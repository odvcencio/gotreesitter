package gotreesitter

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
