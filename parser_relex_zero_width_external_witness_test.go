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

// zeroWidthRelexWitnessFixture bundles the pieces every rescue test needs so
// each test body only states what it is proving.
type zeroWidthRelexWitnessFixture struct {
	p       *Parser
	dts     *dfaTokenSource
	source  []byte
	tok     Token
	stacks  []glrStack
	starved *glrStack
	arena   *nodeArena

	nodeCount        int
	scratch          *parserScratch
	trackChildErrors bool
	rescueBudget     int
}

// newZeroWidthRelexWitnessFixture wires lang up against the shared perl
// witness bytes and lookahead ("foo(1, 2;\n", the DFA `number` token at byte
// 4), with a two-stack frontier: stack 0 starved in state 1, stack 1 already
// surviving in state 2. rescueBudget defaults to a generous cap; tests that
// exercise the budget itself override it after construction.
func newZeroWidthRelexWitnessFixture(t *testing.T, lang *Language) *zeroWidthRelexWitnessFixture {
	t.Helper()
	p := NewParser(lang)
	source := []byte("foo(1, 2;\n")
	tok := Token{Symbol: 1, StartByte: 4, EndByte: 5, StartPoint: Point{Column: 4}, EndPoint: Point{Column: 5}}
	dts := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, p.lookupActionIndex, nil, nil, nil)
	t.Cleanup(dts.Close)

	// byteOffset matches every live stack's real position in an actual GLR
	// frontier: all stacks stay in lockstep at the shared token's start byte
	// until one of them shifts. guardRealShiftGap (parser_recover_c.go)
	// checks this field, so a fixture stack sitting at its zero value would
	// look like a stale gap back to byte 0 instead of the starved stack this
	// test means to model.
	stacks := []glrStack{
		{entries: []stackEntry{{state: 1}}, byteOffset: tok.StartByte}, // starved: no action for `number` yet
		{entries: []stackEntry{{state: 2}}, byteOffset: tok.StartByte}, // sibling: already accepts `number`
	}
	arena := acquireNodeArena(arenaClassFull)
	t.Cleanup(arena.Release)

	return &zeroWidthRelexWitnessFixture{
		p:            p,
		dts:          dts,
		source:       source,
		tok:          tok,
		stacks:       stacks,
		starved:      &stacks[0],
		arena:        arena,
		scratch:      &parserScratch{},
		rescueBudget: maxConsecutiveZeroWidthTokens,
	}
}

func (f *zeroWidthRelexWitnessFixture) rescue(t *testing.T) (Token, StateID, bool) {
	t.Helper()
	return f.p.relexZeroWidthExternalTokenForStackLexState(
		f.source, f.dts, f.starved, f.starved.top().state, f.tok,
		&f.nodeCount, f.arena, f.scratch, &f.trackChildErrors, &f.rescueBudget,
	)
}

// TestRelexZeroWidthExternalTokenRescuesStarvedStack is the perl `_NONASSOC`
// witness named on relexTokenForStackLexState's doc comment: a two-stack GLR
// frontier where one stack is starved for the shared DFA token until it
// shifts a zero-width external token first. It asserts the starved stack
// shifts the zero-width external and survives (its new state has a real
// action for the token that killed it before), that the shared external
// scanner payload comes back unchanged, and that the stack is left able to
// retry the shared token (shifted reset to false, not stuck "done").
func TestRelexZeroWidthExternalTokenRescuesStarvedStack(t *testing.T) {
	f := newZeroWidthRelexWitnessFixture(t, perlNonassocWitnessLanguage())

	baseline := f.dts.captureExternalScannerStateInto(&f.dts.externalSnapshot)
	baselineCopy := append([]byte(nil), baseline...)

	if f.starved.top().state != 1 {
		t.Fatalf("precondition: starved stack state = %d, want 1", f.starved.top().state)
	}
	if f.p.stateHasActionForSymbol(1, f.tok.Symbol) {
		t.Fatal("precondition: starved stack must have no action for the shared token before the rescue")
	}
	f.starved.shifted = true // any pre-existing value must not leak through untouched

	gotTok, newState, ok := f.rescue(t)
	if !ok {
		t.Fatal("zero-width external rescue declined the perl _NONASSOC witness")
	}

	// The shared token itself must come back unchanged: the rescue advances
	// the stack's own state, not the frontier's shared lookahead.
	if gotTok != f.tok {
		t.Fatalf("rescue returned tok=%+v, want the unmodified shared token %+v", gotTok, f.tok)
	}

	// Survival: the starved stack's new state must now have a real action
	// for the token that killed it before.
	if newState != 3 {
		t.Fatalf("new state = %d, want 3 (post-_nonassoc-shift)", newState)
	}
	if !f.p.stateHasActionForSymbol(newState, f.tok.Symbol) {
		t.Fatal("starved stack did not survive: its new state still has no action for the shared token")
	}
	if f.starved.top().state != 3 {
		t.Fatalf("stack top state = %d, want 3", f.starved.top().state)
	}

	// The zero-width external leaf must have actually shifted onto the
	// stack, at the shared token's own start byte, moving no bytes.
	if len(f.starved.entries) != 2 {
		t.Fatalf("stack entries = %d, want 2 (initial + shifted _nonassoc leaf)", len(f.starved.entries))
	}

	// B2: the stack has not consumed the shared token yet -- the caller is
	// about to retry it -- so shifted must read false, exactly as it would
	// with no rescue at all.
	if f.starved.shifted {
		t.Fatal("rescue left shifted=true on a stack that has not consumed the shared token")
	}

	// The sibling stack must be untouched: the rescue is scoped to the one
	// starved stack.
	if f.stacks[1].top().state != 2 || len(f.stacks[1].entries) != 1 {
		t.Fatalf("sibling stack mutated: %+v", f.stacks[1])
	}

	// The shared scanner payload must come back exactly as it was, even
	// though the probe's scan succeeded: the mutation belongs to this one
	// stack, not to every live stack's future external tokens.
	after := f.dts.captureExternalScannerStateInto(&f.dts.externalSnapshot)
	if string(after) != string(baselineCopy) {
		t.Fatalf("shared external scanner payload changed: before=%v after=%v", baselineCopy, after)
	}

	// The rescue budget must have been spent by exactly one successful
	// rescue.
	if f.rescueBudget != maxConsecutiveZeroWidthTokens-1 {
		t.Fatalf("rescueBudget = %d, want %d after one rescue", f.rescueBudget, maxConsecutiveZeroWidthTokens-1)
	}
}

// TestRelexZeroWidthExternalTokenRequiresNoActionInStarvedState proves the
// rescue declines when the starved state has no action at all for the
// probed symbol.
func TestRelexZeroWidthExternalTokenRequiresNoActionInStarvedState(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	lang.ParseTable[1][2] = 0 // no action for _nonassoc in state 1 anymore
	f := newZeroWidthRelexWitnessFixture(t, lang)

	_, _, ok := f.rescue(t)
	if ok {
		t.Fatal("rescue applied a zero-width external shift with no action for it in the starved state")
	}
	if len(f.starved.entries) != 1 {
		t.Fatalf("stack mutated despite the missing action: entries=%d", len(f.starved.entries))
	}
}

// perlNonassocAdvancingExternalScanner is perlNonassocZeroWidthExternalScanner
// except it advances one byte of real content before marking the end, so its
// token is one byte wide instead of zero-width.
type perlNonassocAdvancingExternalScanner struct{}

func (perlNonassocAdvancingExternalScanner) Create() any               { return new(int) }
func (perlNonassocAdvancingExternalScanner) Destroy(any)               {}
func (perlNonassocAdvancingExternalScanner) Serialize(any, []byte) int { return 0 }
func (perlNonassocAdvancingExternalScanner) Deserialize(any, []byte)   {}
func (perlNonassocAdvancingExternalScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	lexer.Advance(false) // consumes real content: the token will not be zero-width
	lexer.SetResultSymbol(2)
	lexer.MarkEnd()
	return true
}

// TestRelexZeroWidthExternalTokenRequiresZeroWidthResult proves the rescue
// declines a probed token that consumes a real byte instead of landing
// zero-width at the shared token's start: requirement 3 on
// relexTokenForStackLexState's doc comment exists so a rescued shift never
// moves the byte frontier out from under the sibling stacks still waiting on
// the shared lookahead.
func TestRelexZeroWidthExternalTokenRequiresZeroWidthResult(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	lang.ExternalScanner = perlNonassocAdvancingExternalScanner{}
	f := newZeroWidthRelexWitnessFixture(t, lang)

	_, _, ok := f.rescue(t)
	if ok {
		t.Fatal("rescue accepted a non-zero-width probed token")
	}
	if len(f.starved.entries) != 1 {
		t.Fatalf("stack mutated despite the non-zero-width result: entries=%d", len(f.starved.entries))
	}
}

// perlNonassocSkippingExternalScanner is perlNonassocZeroWidthExternalScanner
// except it skips one byte (Advance(true)) before marking the end, so its
// zero-width token starts one byte past the shared token's own start byte.
type perlNonassocSkippingExternalScanner struct{}

func (perlNonassocSkippingExternalScanner) Create() any               { return new(int) }
func (perlNonassocSkippingExternalScanner) Destroy(any)               {}
func (perlNonassocSkippingExternalScanner) Serialize(any, []byte) int { return 0 }
func (perlNonassocSkippingExternalScanner) Deserialize(any, []byte)   {}
func (perlNonassocSkippingExternalScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	lexer.Advance(true) // skips one byte: the mark below lands past tok.StartByte
	lexer.SetResultSymbol(2)
	lexer.MarkEnd()
	return true
}

// TestRelexZeroWidthExternalTokenRequiresStartByteMatch proves the rescue
// declines a probed token that is zero-width, but at a different byte than
// the shared token's own start byte: the other half of requirement 3, since
// a rescue at the wrong byte would desynchronize this stack's position from
// every sibling still waiting on the shared lookahead at its original byte.
func TestRelexZeroWidthExternalTokenRequiresStartByteMatch(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	lang.ExternalScanner = perlNonassocSkippingExternalScanner{}
	f := newZeroWidthRelexWitnessFixture(t, lang)

	_, _, ok := f.rescue(t)
	if ok {
		t.Fatal("rescue accepted a probed token starting at the wrong byte")
	}
	if len(f.starved.entries) != 1 {
		t.Fatalf("stack mutated despite the start-byte mismatch: entries=%d", len(f.starved.entries))
	}
}

// TestRelexZeroWidthExternalTokenCallsGuardRealShiftGap is the M3 witness:
// the rescue must call guardRealShiftGap before shifting, the same
// byte-continuity check every other shift call site in the dispatch loop
// makes. A stack whose own byteOffset has fallen behind the shared token's
// start byte by an unexplained, non-padding gap must not shift at all; the
// guard kills such a stack (matching every other shift site), so this test
// also confirms that side effect fires.
func TestRelexZeroWidthExternalTokenCallsGuardRealShiftGap(t *testing.T) {
	f := newZeroWidthRelexWitnessFixture(t, perlNonassocWitnessLanguage())
	// Open an unexplained real-byte gap between the stack's own position and
	// the shared token's start byte: source[0:4] is "foo(", not padding.
	f.starved.byteOffset = 0

	_, _, ok := f.rescue(t)
	if ok {
		t.Fatal("rescue shifted across a real, unexplained byte gap")
	}
	if len(f.starved.entries) != 1 {
		t.Fatalf("stack mutated despite the byte-continuity gap: entries=%d", len(f.starved.entries))
	}
	if !f.starved.dead {
		t.Fatal("guardRealShiftGap's failure must kill the stack, matching every other shift call site")
	}
}

// TestRelexZeroWidthExternalTokenRequiresSingleShiftAction proves the rescue
// declines when the starved state's action for the probed symbol exists but
// is not exactly one shift: a lone reduce, and a shift/reduce conflict cell,
// are both out of this rescue's scope by construction
// (singleShiftActionForSymbol).
func TestRelexZeroWidthExternalTokenRequiresSingleShiftAction(t *testing.T) {
	t.Run("lone reduce", func(t *testing.T) {
		lang := perlNonassocWitnessLanguage()
		lang.ParseActions[1].Actions = []ParseAction{{Type: ParseActionReduce, Symbol: 1, ChildCount: 0}}
		f := newZeroWidthRelexWitnessFixture(t, lang)

		_, _, ok := f.rescue(t)
		if ok {
			t.Fatal("rescue applied a shift for a reduce-only action cell")
		}
		if len(f.starved.entries) != 1 {
			t.Fatalf("stack mutated despite the reduce-only action: entries=%d", len(f.starved.entries))
		}
	})

	t.Run("shift-reduce conflict", func(t *testing.T) {
		lang := perlNonassocWitnessLanguage()
		lang.ParseActions[1].Actions = []ParseAction{
			{Type: ParseActionShift, State: 3},
			{Type: ParseActionReduce, Symbol: 1, ChildCount: 0},
		}
		f := newZeroWidthRelexWitnessFixture(t, lang)

		_, _, ok := f.rescue(t)
		if ok {
			t.Fatal("rescue applied a shift from a conflicted action cell")
		}
		if len(f.starved.entries) != 1 {
			t.Fatalf("stack mutated despite the action conflict: entries=%d", len(f.starved.entries))
		}
	})
}

// TestRelexZeroWidthExternalTokenRequiresForwardProgress is the B1/B3
// termination-proof witness: state 1 shifts `_nonassoc` into state 3, but
// state 3 (unlike the primary witness) still has no action for the shared
// token `number`. Without checking this before committing, the rescue would
// shift onto state 3, immediately starve again, and (if state 3 also had a
// zero-width rescue back toward state 1) loop forever. The rescue must
// decline before ever mutating the stack.
func TestRelexZeroWidthExternalTokenRequiresForwardProgress(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	lang.ParseTable[3][1] = 0 // state 3 no longer has an action for `number`
	f := newZeroWidthRelexWitnessFixture(t, lang)

	if !f.p.stateHasActionForSymbol(1, Symbol(2)) {
		t.Fatal("precondition: state 1 must still have a shift action for _nonassoc")
	}
	if f.p.stateHasActionForSymbol(3, f.tok.Symbol) {
		t.Fatal("precondition: state 3 must have no action for the shared token")
	}

	_, _, ok := f.rescue(t)
	if ok {
		t.Fatal("rescue committed a shift into a state with no forward progress on the shared token")
	}
	if len(f.starved.entries) != 1 {
		t.Fatalf("stack mutated despite the missing forward-progress proof: entries=%d", len(f.starved.entries))
	}
}

// TestRelexZeroWidthExternalTokenRespectsRescueBudget is the explicit
// per-stack rescue bound (B1 defense-in-depth, behind the forward-progress
// proof above): once the budget for this stack's dispatch of this shared
// token is spent, the rescue declines even though every other condition
// still holds.
func TestRelexZeroWidthExternalTokenRespectsRescueBudget(t *testing.T) {
	f := newZeroWidthRelexWitnessFixture(t, perlNonassocWitnessLanguage())
	f.rescueBudget = 0

	_, _, ok := f.rescue(t)
	if ok {
		t.Fatal("rescue fired with an exhausted budget")
	}
	if len(f.starved.entries) != 1 {
		t.Fatalf("stack mutated despite the exhausted budget: entries=%d", len(f.starved.entries))
	}
	if f.rescueBudget != 0 {
		t.Fatalf("rescueBudget = %d, want unchanged 0 on decline", f.rescueBudget)
	}
}

// TestRelexZeroWidthExternalTokenRequiresMatchingLanguage proves the rescue
// declines when the token source's language does not match the parser's:
// dts.ExternalLexStates rows would otherwise describe a different grammar's
// lex states entirely.
func TestRelexZeroWidthExternalTokenRequiresMatchingLanguage(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	f := newZeroWidthRelexWitnessFixture(t, lang)
	other := perlNonassocWitnessLanguage()
	other.Name = "perl_nonassoc_witness_other"
	f.dts.language = other

	_, _, ok := f.rescue(t)
	if ok {
		t.Fatal("rescue fired against a token source scanning a different language")
	}
	if len(f.starved.entries) != 1 {
		t.Fatalf("stack mutated despite the language mismatch: entries=%d", len(f.starved.entries))
	}
}

// positionRecordingTwoSymbolExternalScanner has two external symbols (index
// 0 and 1 in ExternalLexStates order). It declines its first candidate every
// time (the shape that asks runExternalScannerWithRetry for a masked retry
// excluding that symbol), then succeeds on the second candidate -- but only
// when asked to scan from wantPos. This is the M2 witness: with two
// candidates, runExternalScannerWithRetry's masked retry loop does not run
// out of symbols after excluding the first, so it actually attempts a second
// scan, from d.lexer.pos, which is not this probe's start byte (the probe
// never moves d.lexer). *sawWrongPos records whether any attempt ever ran
// from a position other than wantPos.
type positionRecordingTwoSymbolExternalScanner struct {
	wantPos     int
	sawWrongPos *bool
}

func (s positionRecordingTwoSymbolExternalScanner) Create() any               { return new(int) }
func (s positionRecordingTwoSymbolExternalScanner) Destroy(any)               {}
func (s positionRecordingTwoSymbolExternalScanner) Serialize(any, []byte) int { return 0 }
func (s positionRecordingTwoSymbolExternalScanner) Deserialize(any, []byte)   {}
func (s positionRecordingTwoSymbolExternalScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	if lexer.pos != s.wantPos {
		*s.sawWrongPos = true
	}
	if len(valid) > 0 && valid[0] {
		// First candidate: report a result but decline it, asking (in
		// tree-sitter terms) for a masked retry excluding this symbol.
		lexer.SetResultSymbol(2)
		lexer.MarkEnd()
		return false
	}
	if len(valid) > 1 && valid[1] {
		// Second candidate, still valid after masking out the first:
		// succeed, from whatever position this attempt actually ran at.
		lexer.SetResultSymbol(3)
		lexer.MarkEnd()
		return true
	}
	return false
}

// TestRelexZeroWidthExternalTokenDoesNotRetry is the M2 witness: a scanner
// whose first candidate reports (then declines) a result asks, in
// tree-sitter terms, for a masked retry. runExternalScannerWithRetry would
// grant that by rescanning from d.lexer.pos, which the probe never sets to
// its own start byte (the probe never touches d.lexer at all). The rescue
// calls the scanner directly instead, once, and must decline outright
// rather than ever attempting that mispositioned retry.
func TestRelexZeroWidthExternalTokenDoesNotRetry(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	lang.SymbolNames = append(lang.SymbolNames, "_nonassoc_alt")
	lang.ExternalSymbols = []Symbol{2, 3}
	lang.ExternalLexStates = [][]bool{{true, true}}
	sawWrongPos := false
	source := []byte("foo(1, 2;\n")
	lang.ExternalScanner = positionRecordingTwoSymbolExternalScanner{wantPos: 4, sawWrongPos: &sawWrongPos}
	f := newZeroWidthRelexWitnessFixture(t, lang)

	// Give d.lexer a position far from the probe's start byte (4), so a
	// retry that used it would run from a clearly different position.
	f.dts.lexer.pos = len(source)

	_, _, ok := f.rescue(t)
	if ok {
		t.Fatal("rescue accepted a result that only a mispositioned retry could have produced")
	}
	if sawWrongPos {
		t.Fatal("a scan attempt ran from a position other than the probe's own start byte")
	}
	if f.dts.lexer.pos != len(source) {
		t.Fatalf("probe moved d.lexer.pos to %d, want it untouched at %d", f.dts.lexer.pos, len(source))
	}
}

// probeObserverPayload lets a test see the scanner payload value Scan
// actually read, even though the rescue restores the live payload
// afterward: counter is the persistent, serialized state; lastSeenAtScan is
// a plain bookkeeping field Serialize/Deserialize never touch, so it
// survives the restore and stays inspectable after the call returns.
type probeObserverPayload struct {
	counter        int
	lastSeenAtScan int
}

// observingCheckpointedExternalScanner is a stateful, checkpoint-capable
// scanner (UsesExternalScannerCheckpoints) standing in for a real one like
// scala's (the exposure this task's revision was asked to check): it proves
// the probe reads from the pre-scan payload (externalTokenStart), not
// whatever the live payload holds at dispatch time.
type observingCheckpointedExternalScanner struct{}

func (observingCheckpointedExternalScanner) Create() any { return &probeObserverPayload{} }
func (observingCheckpointedExternalScanner) Destroy(any) {}

func (observingCheckpointedExternalScanner) Serialize(payload any, buf []byte) int {
	p, ok := payload.(*probeObserverPayload)
	if !ok || len(buf) == 0 {
		return 0
	}
	buf[0] = byte(p.counter)
	return 1
}

func (observingCheckpointedExternalScanner) Deserialize(payload any, buf []byte) {
	p, ok := payload.(*probeObserverPayload)
	if !ok {
		return
	}
	if len(buf) == 0 {
		p.counter = 0
		return
	}
	p.counter = int(buf[0])
}

func (observingCheckpointedExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

func (observingCheckpointedExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	p := payload.(*probeObserverPayload)
	p.lastSeenAtScan = p.counter
	p.counter++
	lexer.SetResultSymbol(2)
	lexer.MarkEnd()
	return true
}

// TestRelexZeroWidthExternalTokenProbesPreScanPayload is the M1 witness: for
// a checkpoint-capable scanner, the probe must read from the payload as of
// the START of the shared token (externalTokenStart), not whatever the live
// payload holds when the rescue happens to run (dispatch time), which for a
// stateful scanner can be the state AFTER the shared token's own scan.
func TestRelexZeroWidthExternalTokenProbesPreScanPayload(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	lang.ExternalScanner = observingCheckpointedExternalScanner{}
	f := newZeroWidthRelexWitnessFixture(t, lang)

	if !f.dts.usesExternalCheckpoints {
		t.Fatal("precondition: token source did not detect checkpoint support")
	}

	// Simulate Next() having captured counter=5 as the state before the
	// shared token's own scan, then having left the LIVE payload at
	// counter=99 (as if the shared token's own external scan advanced it) --
	// the wrong state to probe from.
	f.dts.externalTokenStart = append(f.dts.externalTokenStart[:0], 5)
	f.dts.language.ExternalScanner.Deserialize(f.dts.externalPayload, []byte{99})

	_, _, ok := f.rescue(t)
	if !ok {
		t.Fatal("rescue declined against a checkpoint-capable scanner")
	}

	observed := f.dts.externalPayload.(*probeObserverPayload)
	if observed.lastSeenAtScan != 5 {
		t.Fatalf("scanner saw counter=%d at scan time, want 5 (externalTokenStart, not dispatch-time payload)", observed.lastSeenAtScan)
	}
	if observed.counter != 99 {
		t.Fatalf("live payload counter = %d after the rescue, want restored to dispatch-time 99", observed.counter)
	}
}

// TestRelexZeroWidthExternalTokenAttachesCheckpointForCheckpointedScanner is
// the M4 witness: the rescued leaf must carry a checkpoint scoped to its own
// span for a checkpoint-capable scanner, so cStackEntryExternalScannerStatesEqual
// can prove its end state instead of refusing every merge this stack takes
// part in afterward (fail-closed on an always-missing checkpoint).
func TestRelexZeroWidthExternalTokenAttachesCheckpointForCheckpointedScanner(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	lang.ExternalScanner = observingCheckpointedExternalScanner{}
	f := newZeroWidthRelexWitnessFixture(t, lang)

	_, _, ok := f.rescue(t)
	if !ok {
		t.Fatal("rescue declined against a checkpoint-capable scanner")
	}
	if len(f.starved.entries) != 2 {
		t.Fatalf("stack entries = %d, want 2 after the rescue", len(f.starved.entries))
	}

	rescuedEntry := f.starved.entries[len(f.starved.entries)-1]
	mergeScratch := &glrMergeScratch{language: lang, arena: f.arena}
	if !stackEntryIsExternalScannerLeaf(rescuedEntry, 0) {
		t.Fatal("rescued entry does not carry the external-scanner-token flag")
	}
	state, ok := cStackEntryExternalScannerEndState(mergeScratch, rescuedEntry, true)
	if !ok || len(state) == 0 {
		t.Fatalf("rescued leaf has no provable external-scanner end state: ok=%v state=%v (checkpoint was not attached)", ok, state)
	}
}

// TestRelexZeroWidthExternalTokenSkipsCheckpointWithoutSupport proves the
// perl-shaped case stays exactly as before: a scanner without checkpoint
// support never gets a checkpoint attached (matching how every other
// external-scanner leaf in a non-checkpoint-capable language already
// behaves), so the rescue does not fabricate one it cannot prove.
func TestRelexZeroWidthExternalTokenSkipsCheckpointWithoutSupport(t *testing.T) {
	f := newZeroWidthRelexWitnessFixture(t, perlNonassocWitnessLanguage())

	_, _, ok := f.rescue(t)
	if !ok {
		t.Fatal("rescue declined the perl-shaped (non-checkpointed) witness")
	}
	rescuedEntry := f.starved.entries[len(f.starved.entries)-1]
	mergeScratch := &glrMergeScratch{language: f.p.language, arena: f.arena}
	if _, ok := cStackEntryExternalScannerEndState(mergeScratch, rescuedEntry, true); ok {
		t.Fatal("rescued leaf claims a provable end state without checkpoint support")
	}
}
