package gotreesitter

import "testing"

// perlNonassocZeroWidthExternalScanner is a minimal external scanner
// standing in for perl upstream 8917c6e9's `_NONASSOC` precedence marker: it
// unconditionally emits a zero-width token for its one external symbol and
// mutates its persistent payload every time it runs, so a test can prove the
// rescue restores that payload on every path (relexZeroWidthExternalTokenForStackLexState,
// parser_recover_c.go).
type perlNonassocZeroWidthExternalScanner struct{}

func (perlNonassocZeroWidthExternalScanner) Create() any { return new(int) }
func (perlNonassocZeroWidthExternalScanner) Destroy(any) {}

func (perlNonassocZeroWidthExternalScanner) Serialize(payload any, buf []byte) int {
	n, ok := payload.(*int)
	if !ok || len(buf) == 0 {
		return 0
	}
	buf[0] = byte(*n)
	return 1
}

func (perlNonassocZeroWidthExternalScanner) Deserialize(payload any, buf []byte) {
	n, ok := payload.(*int)
	if !ok {
		return
	}
	if len(buf) == 0 {
		*n = 0
		return
	}
	*n = int(buf[0])
}

func (perlNonassocZeroWidthExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	if n, ok := payload.(*int); ok {
		// Mutate persistent memory the way a real precedence-marker scanner
		// might (tracking a nesting depth, for example). The test asserts
		// this mutation never survives the rescue.
		*n++
	}
	lexer.SetResultSymbol(2) // _nonassoc
	lexer.MarkEnd()
	return true
}

// perlNonassocWitnessLanguage builds the smallest grammar that reproduces
// the perl `_NONASSOC` fork described on relexTokenForStackLexState: a
// starved stack (state 1) has no action for the shared DFA token `number`
// (symbol 1) until it first shifts the zero-width external `_nonassoc`
// (symbol 2), landing in state 3, which does. A sibling stack (state 2) is
// not starved: it already has a direct action for `number`. Neither state's
// DFA lex mode can produce anything (LexStates has no transitions), so the
// DFA-only probe in relexTokenForStackLexState always fails first, exactly
// as it does for real perl witness bytes where the DFA reading of `number`
// exists but is the wrong reading for the starved stack.
func perlNonassocWitnessLanguage() *Language {
	lang := &Language{
		Name:            "perl_nonassoc_witness",
		StateCount:      4,
		SymbolCount:     3,
		TokenCount:      3,
		SymbolNames:     []string{"end", "number", "_nonassoc"},
		ExternalSymbols: []Symbol{2},
		ExternalScanner: perlNonassocZeroWidthExternalScanner{},
		LexStates: []LexState{
			{Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0, ExternalLexState: 0}, // state 1: starved, wants _nonassoc first
			{LexState: 0},                      // state 2: sibling, already accepts number
			{LexState: 0},                      // state 3: post-shift state, now accepts number
		},
		ExternalLexStates: [][]bool{
			{true}, // row 0: _nonassoc valid
		},
		ParseTable: [][]uint16{
			make([]uint16, 3),
			make([]uint16, 3),
			make([]uint16, 3),
			make([]uint16, 3),
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}}, // shift _nonassoc -> state 3
			{Actions: []ParseAction{{Type: ParseActionShift, State: 9}}}, // shift number -> state 9 (accepting)
		},
	}
	lang.ParseTable[1][2] = 1 // state 1, symbol _nonassoc -> action 1 (shift to 3)
	lang.ParseTable[2][1] = 2 // state 2, symbol number -> action 2 (shift to 9), sibling already survives
	lang.ParseTable[3][1] = 2 // state 3, symbol number -> action 2 (shift to 9): the starved stack now survives
	return lang
}

// TestRelexZeroWidthExternalTokenRescuesStarvedStack is the perl `_NONASSOC`
// witness named on relexTokenForStackLexState's doc comment: a two-stack GLR
// frontier where one stack is starved for the shared DFA token until it
// shifts a zero-width external token first. It asserts the starved stack
// shifts the zero-width external and survives (its new state has a real
// action for the token that killed it before), and that the shared external
// scanner payload comes back unchanged.
func TestRelexZeroWidthExternalTokenRescuesStarvedStack(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	p := NewParser(lang)

	source := []byte("foo(1, 2;\n")
	// The shared lookahead both forked stacks contend for: the DFA `number`
	// token at byte 4, matching the perl witness's `(1` fork point.
	tok := Token{Symbol: 1, StartByte: 4, EndByte: 5, StartPoint: Point{Column: 4}, EndPoint: Point{Column: 5}}

	dts := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, p.lookupActionIndex, nil, nil, nil)
	defer dts.Close()

	// Baseline: capture the scanner payload before any probe runs, so the
	// test can prove the rescue restores it exactly, including on success.
	baseline := dts.captureExternalScannerStateInto(&dts.externalSnapshot)
	baselineCopy := append([]byte(nil), baseline...)

	stacks := []glrStack{
		{entries: []stackEntry{{state: 1}}}, // starved: no action for `number` yet
		{entries: []stackEntry{{state: 2}}}, // sibling: already accepts `number`
	}
	starved := &stacks[0]

	var nodeCount int
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	scratch := &parserScratch{}
	var trackChildErrors bool
	var lexicalReadSpan uint32

	if starved.top().state != 1 {
		t.Fatalf("precondition: starved stack state = %d, want 1", starved.top().state)
	}
	if p.stateHasActionForSymbol(1, tok.Symbol) {
		t.Fatal("precondition: starved stack must have no action for the shared token before the rescue")
	}

	gotTok, newState, ok := p.relexTokenForStackLexState(
		source, 1, tok, &lexicalReadSpan,
		dts, starved, &nodeCount, arena, scratch, &trackChildErrors,
	)
	if !ok {
		t.Fatal("zero-width external rescue declined the perl _NONASSOC witness")
	}

	// The shared token itself must come back unchanged: the rescue advances
	// the stack's own state, not the frontier's shared lookahead.
	if gotTok != tok {
		t.Fatalf("rescue returned tok=%+v, want the unmodified shared token %+v", gotTok, tok)
	}

	// Survival: the starved stack's new state must now have a real action
	// for the token that killed it before.
	if newState != 3 {
		t.Fatalf("new state = %d, want 3 (post-_nonassoc-shift)", newState)
	}
	if !p.stateHasActionForSymbol(newState, tok.Symbol) {
		t.Fatal("starved stack did not survive: its new state still has no action for the shared token")
	}
	if starved.top().state != 3 {
		t.Fatalf("stack top state = %d, want 3", starved.top().state)
	}

	// The zero-width external leaf must have actually shifted onto the
	// stack, at the shared token's own start byte, moving no bytes.
	if len(starved.entries) != 2 {
		t.Fatalf("stack entries = %d, want 2 (initial + shifted _nonassoc leaf)", len(starved.entries))
	}

	// The sibling stack must be untouched: the rescue is scoped to the one
	// starved stack.
	if stacks[1].top().state != 2 || len(stacks[1].entries) != 1 {
		t.Fatalf("sibling stack mutated: %+v", stacks[1])
	}

	// The shared scanner payload must come back exactly as it was, even
	// though the probe's scan succeeded: the mutation belongs to this one
	// stack, not to every live stack's future external tokens.
	after := dts.captureExternalScannerStateInto(&dts.externalSnapshot)
	if string(after) != string(baselineCopy) {
		t.Fatalf("shared external scanner payload changed: before=%v after=%v", baselineCopy, after)
	}
}

// TestRelexZeroWidthExternalTokenRequiresExactStartByte proves the rescue
// declines a probed token that is not zero-width at the shared token's own
// start byte, keeping the frontier in lockstep (requirement 3 on
// relexTokenForStackLexState's doc comment).
func TestRelexZeroWidthExternalTokenRequiresExactStartByte(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	// Move the shared token's start byte away from where the scanner marks
	// its zero-width end (byte 0 from lexer reset position tok.StartByte):
	// the scanner still reports success, but the resulting token's span
	// still starts at tok.StartByte by construction, so instead this test
	// removes the starved state's shift action for `_nonassoc` to prove the
	// action-verification guard, which is the other half of requirement 3/4.
	lang.ParseTable[1][2] = 0 // no action for _nonassoc in state 1 anymore

	p := NewParser(lang)
	source := []byte("foo(1, 2;\n")
	tok := Token{Symbol: 1, StartByte: 4, EndByte: 5, StartPoint: Point{Column: 4}, EndPoint: Point{Column: 5}}
	dts := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, p.lookupActionIndex, nil, nil, nil)
	defer dts.Close()

	stacks := []glrStack{{entries: []stackEntry{{state: 1}}}}
	starved := &stacks[0]
	var nodeCount int
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	scratch := &parserScratch{}
	var trackChildErrors bool
	var lexicalReadSpan uint32

	_, _, ok := p.relexTokenForStackLexState(
		source, 1, tok, &lexicalReadSpan,
		dts, starved, &nodeCount, arena, scratch, &trackChildErrors,
	)
	if ok {
		t.Fatal("rescue applied a zero-width external shift with no action for it in the starved state")
	}
	if len(starved.entries) != 1 {
		t.Fatalf("stack mutated despite the missing action: entries=%d", len(starved.entries))
	}
}
