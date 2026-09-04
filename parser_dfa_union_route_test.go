package gotreesitter

import "testing"

func TestNextGLRUnionDFATokenScoresOnlyStatesProducingCandidate(t *testing.T) {
	lang := &Language{
		Name:        "glr-union-route-test",
		SymbolNames: []string{"EOF", "keyword", "identifier"},
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 2}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'b', Hi: 'b', NextState: 3}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 5}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'b', Hi: 'b', NextState: 6}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{},
			{LexState: 1},
			{LexState: 4},
			{LexState: 4},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	lookup := func(state StateID, sym Symbol) uint16 {
		switch sym {
		case 1:
			if state == 1 || state == 2 || state == 3 {
				return 1
			}
		case 2:
			if state == 2 || state == 3 {
				return 1
			}
		}
		return 0
	}

	ts := acquireDFATokenSource(NewLexer(lang.LexStates, []byte("ab")), lang, lookup, nil, nil, nil)
	defer ts.Close()
	ts.SetParserState(1)
	ts.SetGLRStates([]StateID{1, 2, 3})

	tok, ok := ts.nextGLRUnionDFAToken()
	if !ok {
		t.Fatal("nextGLRUnionDFAToken returned false")
	}
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if tok.StartByte != 0 || tok.EndByte != 2 {
		t.Fatalf("token span = %d..%d, want 0..2", tok.StartByte, tok.EndByte)
	}
}

func TestGLRUnionMergesEquivalentCandidatesWithMaximumLookaheadFrontier(t *testing.T) {
	lowFrontier := Token{Symbol: 1, StartByte: 0, EndByte: 1, lexerLookaheadEndByte: 2}
	highFrontier := lowFrontier
	highFrontier.lexerLookaheadEndByte = 3
	if tokensSameLex(lowFrontier, highFrontier) {
		t.Fatal("strict token equality ignored distinct lookahead frontiers")
	}
	if !tokensSameLexForGLRCandidate(lowFrontier, highFrontier) {
		t.Fatal("GLR candidate equality rejected the same token surface")
	}

	lang := &Language{
		Name:        "glr-union-frontier-test",
		SymbolNames: []string{"end", "a"},
		SymbolCount: 2,
		TokenCount:  2,
		StateCount:  3,
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 1}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 3}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'b', Hi: 'b', NextState: 4}}},
			{Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{},
			{LexState: 0},
			{LexState: 2},
		},
	}
	lookup := func(state StateID, sym Symbol) uint16 {
		if (state == 1 || state == 2) && sym == 1 {
			return 1
		}
		return 0
	}
	newSource := func() *dfaTokenSource {
		return &dfaTokenSource{
			lexer:             NewLexer(lang.LexStates, []byte("ab")),
			language:          lang,
			state:             1,
			glrStates:         []StateID{1, 2},
			lookupActionIndex: lookup,
		}
	}

	peek := newSource()
	frontier, ok := peek.PeekTokenFrontier(nil, nil)
	if !ok {
		t.Fatal("PeekTokenFrontier returned false")
	}
	if got, want := len(frontier.Candidates), 1; got != want {
		t.Fatalf("candidate count = %d, want %d: %#v", got, want, frontier.Candidates)
	}
	if got, want := frontier.Candidates[0].RouteMask, uint16(0b11); got != want {
		t.Fatalf("route mask = %#x, want %#x", got, want)
	}
	if got, want := tokenLookaheadEndByte(frontier.Candidates[0].Tok), uint32(3); got != want {
		t.Fatalf("peek frontier = %d, want maximum candidate frontier %d", got, want)
	}

	union := newSource()
	tok, ok := union.nextGLRUnionDFAToken()
	if !ok {
		t.Fatal("nextGLRUnionDFAToken returned false")
	}
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d, want %d", got, want)
	}
	if got, want := tokenLookaheadEndByte(tok), uint32(3); got != want {
		t.Fatalf("union frontier = %d, want maximum candidate frontier %d", got, want)
	}
}
