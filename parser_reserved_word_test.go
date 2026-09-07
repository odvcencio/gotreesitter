package gotreesitter

import "testing"

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

func buildCaseKeywordLanguage(name string) *Language {
	return &Language{
		Name:                name,
		SymbolCount:         4,
		TokenCount:          3,
		StateCount:          1,
		LargeStateCount:     1,
		KeywordCaptureToken: 1, // IDENT
		SymbolNames: []string{
			"end",
			"identifier",
			"FROM",
			"stmt",
		},
		KeywordLexStates: []LexState{
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'F', Hi: 'F', NextState: 1},
			}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'R', Hi: 'R', NextState: 2},
			}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'O', Hi: 'O', NextState: 3},
			}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'M', Hi: 'M', NextState: 4},
			}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
	}
}

func promoteCaseKeyword(lang *Language, source []byte) Token {
	d := &dfaTokenSource{
		lexer:    &Lexer{source: source},
		language: lang,
	}
	tok := Token{
		Symbol:    lang.KeywordCaptureToken,
		StartByte: 0,
		EndByte:   uint32(len(source)),
	}
	d.promoteKeyword(&tok)
	return tok
}

func TestSQLKeywordPromotionRetriesUppercase(t *testing.T) {
	lang := buildCaseKeywordLanguage("sql")

	got := promoteCaseKeyword(lang, []byte("from"))
	if got.Symbol != 2 {
		t.Fatalf("lowercase SQL keyword: got symbol %d, want 2 (FROM)", got.Symbol)
	}
}

func TestNonSQLKeywordPromotionRemainsCaseSensitive(t *testing.T) {
	lang := buildCaseKeywordLanguage("case_keyword_test")

	got := promoteCaseKeyword(lang, []byte("from"))
	if got.Symbol != 1 {
		t.Fatalf("lowercase non-SQL keyword: got symbol %d, want 1 (identifier)", got.Symbol)
	}

	got = promoteCaseKeyword(lang, []byte("FROM"))
	if got.Symbol != 2 {
		t.Fatalf("uppercase non-SQL keyword: got symbol %d, want 2 (FROM)", got.Symbol)
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
		got := tok
		d.promoteKeyword(&got)
		return got
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
	got := tok
	d.promoteKeyword(&got)
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
	got := tok
	d.promoteKeyword(&got)
	if got.Symbol != 2 {
		t.Fatalf("setID=0: got symbol %d, want 2 (KW_IF — promoted)", got.Symbol)
	}
}

func TestActiveLiteralKeywordSymbolUsesTokenNameIndex(t *testing.T) {
	lang := &Language{
		Name:        "literal_keyword_test",
		SymbolCount: 6,
		TokenCount:  5,
		SymbolNames: []string{
			"end",
			"identifier",
			"static",
			"static",
			"requires",
			"static",
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	lookup := func(state StateID, sym Symbol) uint16 {
		switch {
		case state == 1 && sym == 3:
			return 1
		case state == 2 && sym == 4:
			return 1
		default:
			return 0
		}
	}
	d := &dfaTokenSource{
		language:          lang,
		state:             1,
		glrStates:         []StateID{1, 1, 2},
		lookupActionIndex: lookup,
	}

	// Warm the per-language symbol index; activeLiteralKeywordSymbol should not
	// allocate while consulting it for repeated token lookups.
	lang.TokenSymbolsByName("static")

	staticTok := Token{Text: "static"}
	if got, ok := d.activeLiteralKeywordSymbol(staticTok); !ok || got != 3 {
		t.Fatalf("active static literal = (%d, %v), want (3, true)", got, ok)
	}
	requiresTok := Token{Text: "requires"}
	if got, ok := d.activeLiteralKeywordSymbol(requiresTok); !ok || got != 4 {
		t.Fatalf("active requires literal = (%d, %v), want (4, true)", got, ok)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := d.activeLiteralKeywordSymbol(staticTok); !ok {
			t.Fatal("active static literal not found")
		}
	})
	if allocs != 0 {
		t.Fatalf("active literal lookup allocations = %v, want 0", allocs)
	}
}
