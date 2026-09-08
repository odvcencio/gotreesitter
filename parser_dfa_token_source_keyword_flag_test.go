package gotreesitter

import "testing"

// keywordAdoptionLanguage builds a minimal language with a word-shaped main
// lexer (any run of lowercase letters) and a keyword lexer that recognizes
// only the literal "if". Symbols:
//
//	0: EOF
//	1: identifier (terminal, named) — keyword capture token
//	2: if (terminal, anonymous) — keyword matched by the keyword DFA
//
// lookupActionIndex is left nil so promoteKeyword's context-aware gating
// (parser_dfa_token_source.go ~4588-4622) is skipped and every full-length
// keyword match adopts unconditionally, matching this fixture's single,
// context-free parser state.
func keywordAdoptionLanguage() *Language {
	return &Language{
		Name:                "keyword_adoption_test",
		SymbolCount:         3,
		TokenCount:          2,
		KeywordCaptureToken: 1, // identifier
		SymbolNames:         []string{"end", "identifier", "if"},
		LexStates: []LexState{
			// state 0: start of a word — dispatch any lowercase letter
			{Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'a', Hi: 'z', NextState: 1},
			}},
			// state 1: accept identifier, loop on further lowercase letters
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'a', Hi: 'z', NextState: 1},
			}},
		},
		LexModes: []LexMode{{LexState: 0}},
		KeywordLexStates: []LexState{
			// kw state 0: start — dispatch 'i'
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'i', Hi: 'i', NextState: 1},
			}},
			// kw state 1: saw 'i' — dispatch 'f'
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'f', Hi: 'f', NextState: 2},
			}},
			// kw state 2: saw "if" — accept the keyword (symbol 2)
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
	}
}

// TestPromoteKeywordSetsIsKeywordOnAdoption drives the token source's real
// Next() entry point over a source that is exactly the keyword-lexer's
// literal ("if"). The main lexer produces the keyword-capture (identifier)
// token; the keyword re-lex fully consumes the same span, so the token is
// adopted: its symbol becomes the "if" keyword and IsKeyword records the
// adoption, mirroring C's Subtree.is_keyword (subtree.h:131).
func TestPromoteKeywordSetsIsKeywordOnAdoption(t *testing.T) {
	lang := keywordAdoptionLanguage()
	source := []byte("if")
	ts := acquireDFATokenSource(NewLexer(lang.LexStates, source), lang, nil, nil, nil, nil)
	defer ts.Close()

	tok := ts.Next()
	if tok.Symbol != 2 {
		t.Fatalf("token symbol = %d, want 2 (if keyword — adopted)", tok.Symbol)
	}
	if !tok.isKeyword() {
		t.Fatal("adopted keyword token: IsKeyword = false, want true")
	}
}

// TestPromoteKeywordLeavesIsKeywordFalseForNonKeywordWord drives Next() over
// a word-shaped span the keyword lexer cannot fully match: "ifx" shares the
// keyword's "if" prefix, but the keyword DFA only accepts the exact literal
// "if" (parser_dfa_token_source.go's lexKeywordSource requires the keyword to
// consume the entire captured span). The token stays an identifier and is
// never adopted, so IsKeyword must be false.
func TestPromoteKeywordLeavesIsKeywordFalseForNonKeywordWord(t *testing.T) {
	lang := keywordAdoptionLanguage()
	source := []byte("ifx")
	ts := acquireDFATokenSource(NewLexer(lang.LexStates, source), lang, nil, nil, nil, nil)
	defer ts.Close()

	tok := ts.Next()
	if tok.Symbol != 1 {
		t.Fatalf("token symbol = %d, want 1 (identifier — not adopted)", tok.Symbol)
	}
	if tok.isKeyword() {
		t.Fatal("non-keyword word token: IsKeyword = true, want false")
	}
}

// TestPromoteKeywordIsKeywordFalseWithoutKeywordCapture drives Next() over
// the keyword's own literal spelling ("if") in a language that has no
// keyword-capture token at all. promoteKeyword must return unmodified
// (parser_dfa_token_source.go:4500-4502), so no token from this language is
// ever adopted and IsKeyword stays false.
func TestPromoteKeywordIsKeywordFalseWithoutKeywordCapture(t *testing.T) {
	lang := keywordAdoptionLanguage()
	lang.KeywordCaptureToken = 0
	lang.KeywordLexStates = nil
	source := []byte("if")
	ts := acquireDFATokenSource(NewLexer(lang.LexStates, source), lang, nil, nil, nil, nil)
	defer ts.Close()

	tok := ts.Next()
	if tok.Symbol != 1 {
		t.Fatalf("token symbol = %d, want 1 (identifier — no keyword capture)", tok.Symbol)
	}
	if tok.isKeyword() {
		t.Fatal("language without keyword capture: IsKeyword = true, want false")
	}
}

// unionSameLexKeywordLanguage builds a two-lex-mode language where one GLR
// stack state reaches the "if" keyword through the identifier-capture path
// (promoted, IsKeyword = true) and another reaches the literal "if"
// spelling directly in the main DFA (IsKeyword = false). Both lex the same
// span and symbol for source "if", so they are the same tokenization and
// the live-GLR union election must treat them as such, regardless of which
// lex path produced the token. Symbols:
//
//	0: end
//	1: identifier (terminal, named) — keyword capture token
//	2: if (terminal, anonymous) — keyword, reachable via promotion or direct
//
// Parser states used by the test: state 1 selects the identifier-capture
// lex mode (DFA state 0); state 2 selects the direct keyword-literal lex
// mode (DFA state 2).
func unionSameLexKeywordLanguage() *Language {
	return &Language{
		Name:                "keyword_union_samelex_test",
		SymbolCount:         3,
		TokenCount:          2,
		KeywordCaptureToken: 1, // identifier
		SymbolNames:         []string{"end", "identifier", "if"},
		LexStates: []LexState{
			// state 0: identifier path start — dispatch any lowercase letter
			{Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'a', Hi: 'z', NextState: 1},
			}},
			// state 1: accept identifier, loop on further lowercase letters
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'a', Hi: 'z', NextState: 1},
			}},
			// state 2: direct keyword path start — dispatch 'i'
			{Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'i', Hi: 'i', NextState: 3},
			}},
			// state 3: saw 'i' — dispatch 'f'
			{Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'f', Hi: 'f', NextState: 4},
			}},
			// state 4: saw "if" directly — accept the keyword (symbol 2)
			// without going through the identifier-capture promotion path.
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{},            // parser state 0: unused placeholder
			{LexState: 0}, // parser state 1: identifier-capture path
			{LexState: 2}, // parser state 2: direct keyword-literal path
		},
		KeywordLexStates: []LexState{
			// kw state 0: start — dispatch 'i'
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'i', Hi: 'i', NextState: 1},
			}},
			// kw state 1: saw 'i' — dispatch 'f'
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'f', Hi: 'f', NextState: 2},
			}},
			// kw state 2: saw "if" — accept the keyword (symbol 2)
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
	}
}

// TestPeekTokenFrontierMergesSameLexDespiteIsKeywordMismatch pins the F1
// finding: PeekTokenFrontier must not let IsKeyword split one tokenization
// across two candidates in the live GLR union election. One state lexes
// "if" through the identifier-capture path and adopts the keyword
// (IsKeyword = true); another lexes the same span directly as the keyword
// literal (IsKeyword = false). Same symbol, same span — the union election
// must merge them into a single candidate whose route mask covers both
// states, not split the vote across two candidates that each look
// unsupported by the other state.
func TestPeekTokenFrontierMergesSameLexDespiteIsKeywordMismatch(t *testing.T) {
	lang := unionSameLexKeywordLanguage()
	lookup := func(state StateID, sym Symbol) uint16 {
		if sym == 2 && (state == 1 || state == 2) {
			return 1
		}
		return 0
	}

	source := []byte("if")
	ts := acquireDFATokenSource(NewLexer(lang.LexStates, source), lang, lookup, nil, nil, nil)
	defer ts.Close()
	ts.SetParserState(1)

	states := []StateID{1, 2}
	frontier, ok := ts.PeekTokenFrontier(states, nil)
	if !ok {
		t.Fatal("PeekTokenFrontier returned false")
	}
	if len(frontier.Candidates) != 1 {
		t.Fatalf("candidates = %d, want 1 (identifier-promoted and direct-literal \"if\" must merge into one candidate)", len(frontier.Candidates))
	}
	cand := frontier.Candidates[0]
	if cand.Tok.Symbol != 2 {
		t.Fatalf("candidate symbol = %d, want 2 (if keyword)", cand.Tok.Symbol)
	}
	if cand.Tok.StartByte != 0 || cand.Tok.EndByte != 2 {
		t.Fatalf("candidate span = %d..%d, want 0..2", cand.Tok.StartByte, cand.Tok.EndByte)
	}
	if want := uint16(1<<0 | 1<<1); cand.RouteMask != want {
		t.Fatalf("candidate route mask = %#x, want %#x (both states must vote for the merged tokenization)", cand.RouteMask, want)
	}
}

// unionSameLexKeywordLanguageWithDecoy extends unionSameLexKeywordLanguage
// with a second, unrelated pair of GLR states (3 and 4) that both lex "if"
// as a same-span, differently-symbolled decoy token ("other", symbol 3)
// through two independently-dedup-safe lex modes (DFA states 5 and 8). The
// decoy pair always scores 2 (each votes for the other; neither promotion
// nor isKeyword ever enters their comparison), which is exactly high enough
// to beat the keyword pair's score if — and only if — nextGLRUnionDFAToken's
// keyword-adoption pair (states 1 and 2) loses ITS mutual vote to the
// isKeyword-mismatch bug fixed at parser_dfa_token_source.go's tokensSameLex
// call in that function's own score loop. With the bug: keyword-pair score
// stays at 1 (self only), the decoy's constant 2 wins outright, and
// Next() elects the wrong token (symbol 3, "other") for source "if". Fixed:
// the keyword pair also scores 2, ties the decoy under identical span/
// visibility/specificity, and the first-processed state (1) keeps the
// correctly-elected keyword (symbol 2, isKeyword = true).
//
// This isolates the SPECIFIC production call site the F1 mutation-testing
// gap flagged (nextGLRUnionDFAToken's own tokensSameLex call): unlike
// TestPeekTokenFrontierMergesSameLexDespiteIsKeywordMismatch, reverting only
// that one call site to a raw != comparison flips this test's elected
// token, because here the decoy pair's constant score of 2 only wins when
// the keyword pair's score is wrongly held down to 1.
//
// Symbols:
//
//	0: end
//	1: identifier (terminal, named) — keyword capture token
//	2: if (terminal, anonymous) — keyword, reachable via promotion or direct
//	3: other (terminal, anonymous) — same-span decoy, never keyword-related
//
// Parser states: 1 (identifier-capture path, DFA state 0), 2 (direct
// keyword-literal path, DFA state 2), 3 (decoy path A, DFA state 5), 4
// (decoy path B, DFA state 8).
func unionSameLexKeywordLanguageWithDecoy() *Language {
	lang := unionSameLexKeywordLanguage()
	lang.Name = "keyword_union_samelex_decoy_test"
	lang.SymbolCount = 4
	// TokenCount stays at the base fixture's 2 (not 4): buildSymbolMaps only
	// registers a symbol's name in the anonymous-token shape index when its
	// index is below TokenCount. Registering "if" there would let
	// promoteActiveLiteralForCurrentState's UNRELATED literal-promotion path
	// (triggered by matching raw source text against any live-actionable
	// anonymous token name, not by symbol identity) silently rewrite the
	// decoy's own Symbol 3 token back to Symbol 2, because both tokens
	// share the literal source text "if" and Symbol 2 has live actions in
	// states 1 and 2. Leaving TokenCount at 2 keeps "if" (and "other") out
	// of that registry, exactly as the base two-state fixture already
	// relies on, so this decoy exercises only the score-based tie-break
	// nextGLRUnionDFAToken itself performs.
	lang.SymbolNames = []string{"end", "identifier", "if", "other"}
	lang.LexStates = append(lang.LexStates,
		// state 5: decoy path A start — dispatch 'i'
		LexState{Default: -1, EOF: -1, Transitions: []LexTransition{
			{Lo: 'i', Hi: 'i', NextState: 6},
		}},
		// state 6: saw 'i' — dispatch 'f'
		LexState{Default: -1, EOF: -1, Transitions: []LexTransition{
			{Lo: 'f', Hi: 'f', NextState: 7},
		}},
		// state 7: saw "if" — accept the decoy (symbol 3), independent of
		// both the identifier-capture and direct-keyword paths.
		LexState{AcceptToken: 3, Default: -1, EOF: -1},
		// state 8: decoy path B start — an independent DFA chain that
		// accepts the identical decoy token as path A, so the two decoy
		// parser states are never dedup-merged (different LexState indexes)
		// but always agree on their own tokenization.
		LexState{Default: -1, EOF: -1, Transitions: []LexTransition{
			{Lo: 'i', Hi: 'i', NextState: 9},
		}},
		LexState{Default: -1, EOF: -1, Transitions: []LexTransition{
			{Lo: 'f', Hi: 'f', NextState: 10},
		}},
		LexState{AcceptToken: 3, Default: -1, EOF: -1},
	)
	lang.LexModes = append(lang.LexModes,
		LexMode{LexState: 5}, // parser state 3: decoy path A
		LexMode{LexState: 8}, // parser state 4: decoy path B
	)
	return lang
}

// TestNextGLRUnionDFATokenElectsMergedKeywordOverCompetingDecoy closes the
// F1 mutation-testing gap: it fails when tokensSameLex's call inside
// nextGLRUnionDFAToken's own score loop (parser_dfa_token_source.go) is
// reverted to a raw != comparison, even though
// TestPeekTokenFrontierMergesSameLexDespiteIsKeywordMismatch (which pins the
// other two call sites) keeps passing under that exact mutation.
//
// See unionSameLexKeywordLanguageWithDecoy's doc comment for the scoring
// argument. Verified by mutation: reverting nextGLRUnionDFAToken's
// tokensSameLex call to `stateTok != candTok` makes this test fail (elects
// symbol 3, "other") and leaves
// TestPeekTokenFrontierMergesSameLexDespiteIsKeywordMismatch passing;
// restoring the call makes both pass.
func TestNextGLRUnionDFATokenElectsMergedKeywordOverCompetingDecoy(t *testing.T) {
	lang := unionSameLexKeywordLanguageWithDecoy()
	lookup := func(state StateID, sym Symbol) uint16 {
		switch {
		case sym == 2 && (state == 1 || state == 2):
			return 1
		case sym == 3 && (state == 3 || state == 4):
			return 1
		}
		return 0
	}

	source := []byte("if")
	ts := acquireDFATokenSource(NewLexer(lang.LexStates, source), lang, lookup, nil, nil, nil)
	defer ts.Close()
	ts.SetParserState(1)
	ts.SetGLRStates([]StateID{1, 2, 3, 4})

	tok := ts.Next()
	if tok.Symbol != 2 {
		t.Fatalf("elected symbol = %d, want 2 (if keyword — the merged keyword pair must outscore the decoy pair)", tok.Symbol)
	}
	if !tok.isKeyword() {
		t.Fatal("elected keyword token: isKeyword = false, want true (elected via the identifier-capture path's own promotion)")
	}
	if tok.StartByte != 0 || tok.EndByte != 2 {
		t.Fatalf("elected span = %d..%d, want 0..2", tok.StartByte, tok.EndByte)
	}
}
