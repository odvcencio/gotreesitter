package gotreesitter

import "testing"

// zeroWidthExternalForkLanguage builds the minimal shape of the Perl bareword
// call conflict: two live GLR stacks, one that can only shift a real DFA token
// and one that can only shift a ZERO-WIDTH external token.
//
//   - state 1: has an action for the DFA token ")" (symbol 1) and none for the
//     external "_ZW" (symbol 2).
//   - state 2: has an action for "_ZW" and none for ")".
//
// This is state 873 / state 4185 in perl's blob, reduced to its essentials.
func zeroWidthExternalForkLanguage() *Language {
	return &Language{
		SymbolNames: []string{"end", ")", "_ZW"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ")", Visible: true, Named: false},
			{Name: "_ZW", Visible: false, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		ExternalSymbols: []Symbol{2},
		// LexStates 1 and 2 are distinct rows with identical behavior: both
		// lex ")" into the accepting row 3. The two live stacks must sit in
		// DIFFERENT lex modes, otherwise nextGLRUnionDFAToken short-circuits
		// on its all-same check and no arbitration happens at all.
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ')', Hi: ')', NextState: 3}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ')', Hi: ')', NextState: 3}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 2},
		},
		// row[state][symbol] -> ParseActions index; 0 means no action.
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 0},
			{0, 0, 1},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
}

// TestPreferGLRUnionDFAKeepsZeroWidthExternalTokenNeededByAFork pins the fork
// rescue that restores Perl's function_call_expression derivation. When the
// external scanner produces a ZERO-WIDTH token that one live GLR stack can act
// on and the competing DFA token cannot serve, the shared-frontier arbitration
// must keep the external token. A zero-width token consumes no input, so the
// stacks that cannot use it skip it and re-lex the same byte; dropping it
// instead kills the only fork that can continue through it.
func TestPreferGLRUnionDFAKeepsZeroWidthExternalTokenNeededByAFork(t *testing.T) {
	lang := zeroWidthExternalForkLanguage()
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte(")")),
		language:          lang,
		state:             1,
		glrStates:         []StateID{1, 2},
		lookupActionIndex: parser.lookupActionIndex,
	}

	extTok := Token{Symbol: 2, StartByte: 0, EndByte: 0, ExternalScannerToken: true}
	if _, _, _, _, preferDFA := d.preferGLRUnionDFAOverExternalToken(extTok, 0, 0, 0, 0, 0, 0); preferDFA {
		t.Fatal("arbitration chose the DFA token over a zero-width external token that only one fork can act on; that fork loses its only lookahead and dies")
	}
}

// TestPreferGLRUnionDFAStillPrefersDFAWhenExternalTokenHasWidth keeps the
// rescue narrow. A non-zero-width external token DOES consume input, so
// keeping it is not free for the stacks that cannot use it, and the existing
// support/specificity ladder still decides.
func TestPreferGLRUnionDFAStillPrefersDFAWhenExternalTokenHasWidth(t *testing.T) {
	lang := zeroWidthExternalForkLanguage()
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte(")")),
		language:          lang,
		state:             1,
		glrStates:         []StateID{1, 2},
		lookupActionIndex: parser.lookupActionIndex,
	}

	extTok := Token{Symbol: 2, StartByte: 0, EndByte: 1, ExternalScannerToken: true}
	dfaTok, _, _, _, preferDFA := d.preferGLRUnionDFAOverExternalToken(extTok, 1, 0, 1, 0, 0, 0)
	if !preferDFA {
		t.Fatal("arbitration kept a width-carrying external token; the support/specificity ladder should still pick the DFA token here")
	}
	if dfaTok.Symbol != 1 {
		t.Fatalf("dfa token symbol = %d, want 1 (%q)", dfaTok.Symbol, lang.SymbolNames[1])
	}
}

// TestAnotherLiveParseStackRemainsIgnoresDeadSiblings pins the predicate the
// no-action recovery ladder now uses instead of the raw slice length. Versions
// this same dispatch pass already killed must not count: while they did, a
// frontier that had ever forked lost its recovery entirely and the parse ended
// with the rest of the file unparsed.
func TestAnotherLiveParseStackRemainsIgnoresDeadSiblings(t *testing.T) {
	stacks := make([]glrStack, 3)
	stacks[0].dead = true
	stacks[1].dead = true

	if anotherLiveParseStackRemains(stacks, 2) {
		t.Fatal("stacks[2] is the last live version, but the predicate reported a live sibling")
	}
	if !anotherLiveParseStackRemains(stacks, 0) {
		t.Fatal("stacks[2] is live, so killing stacks[0] leaves a carrier")
	}

	stacks[1].dead = false
	if !anotherLiveParseStackRemains(stacks, 2) {
		t.Fatal("stacks[1] is live again, so stacks[2] is no longer the last live version")
	}

	single := make([]glrStack, 1)
	if anotherLiveParseStackRemains(single, 0) {
		t.Fatal("a one-version frontier has no sibling")
	}
}
