package gotreesitter

import "testing"

type recordingParserStateTokenSource struct {
	parserStates []StateID
	glrStates    [][]StateID
}

type zeroWidthRelexExternalScanner struct{}

func (zeroWidthRelexExternalScanner) Create() any                    { return nil }
func (zeroWidthRelexExternalScanner) Destroy(any)                    {}
func (zeroWidthRelexExternalScanner) Serialize(any, []byte) int      { return 0 }
func (zeroWidthRelexExternalScanner) Deserialize(any, []byte)        {}
func (zeroWidthRelexExternalScanner) SupportsIncrementalReuse() bool { return true }
func (zeroWidthRelexExternalScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	lexer.SetResultSymbol(3)
	lexer.MarkEnd()
	return true
}

func (s *recordingParserStateTokenSource) Next() Token { return Token{} }

func (s *recordingParserStateTokenSource) SetParserState(state StateID) {
	s.parserStates = append(s.parserStates, state)
}

func (s *recordingParserStateTokenSource) SetGLRStates(states []StateID) {
	s.glrStates = append(s.glrStates, append([]StateID(nil), states...))
}

func TestUpdateCurrentRelexParserStateTokenSourceExcludesShiftedStacks(t *testing.T) {
	p := &Parser{}
	ts := &recordingParserStateTokenSource{}
	scratch := &parserScratch{}
	stacks := []glrStack{
		{entries: []stackEntry{{state: 10}}, shifted: true},
		{entries: []stackEntry{{state: 20}}},
		{entries: []stackEntry{{state: 30}}, shifted: true},
		{entries: []stackEntry{{state: 40}}},
	}

	if ok := p.updateCurrentRelexParserStateTokenSource(ts, stacks, scratch); !ok {
		t.Fatal("updateCurrentRelexParserStateTokenSource returned false, want true")
	}
	if got, want := len(ts.parserStates), 1; got != want {
		t.Fatalf("SetParserState calls = %d, want %d", got, want)
	}
	if got, want := ts.parserStates[0], StateID(20); got != want {
		t.Fatalf("parser state = %d, want first live unshifted state %d", got, want)
	}
	if got, want := len(ts.glrStates), 1; got != want {
		t.Fatalf("SetGLRStates calls = %d, want %d", got, want)
	}
	wantStates := []StateID{20, 40}
	if len(ts.glrStates[0]) != len(wantStates) {
		t.Fatalf("GLR states = %v, want %v", ts.glrStates[0], wantStates)
	}
	for i, want := range wantStates {
		if ts.glrStates[0][i] != want {
			t.Fatalf("GLR states = %v, want %v", ts.glrStates[0], wantStates)
		}
	}
}

func TestSameSurfaceRelexTokenRequiresSameSpanAndSurface(t *testing.T) {
	p := &Parser{language: &Language{
		SymbolNames: []string{"end", "<", "<", ">"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "<"},
			{Name: "<"},
			{Name: ">"},
		},
	}}
	original := Token{Symbol: 1, StartByte: 10, EndByte: 11}

	if !p.sameSurfaceRelexToken(original, Token{Symbol: 2, StartByte: 10, EndByte: 11}) {
		t.Fatal("same-surface duplicate token was not accepted")
	}
	if p.sameSurfaceRelexToken(original, Token{Symbol: 2, StartByte: 10, EndByte: 12}) {
		t.Fatal("same-surface token with different span was accepted")
	}
	if p.sameSurfaceRelexToken(original, Token{Symbol: 3, StartByte: 10, EndByte: 11}) {
		t.Fatal("different-surface token was accepted")
	}
}

func TestDFARelexSnapshotRestoresExternalLookaheadFrontier(t *testing.T) {
	d := &dfaTokenSource{
		lexer: NewLexer([]LexState{{Default: -1, EOF: -1}}, []byte("x")),
	}
	d.externalLookaheadEndByte = 17
	snapshot := d.snapshotRelexState()
	if got, want := snapshot.externalLookaheadEndByte, uint32(17); got != want {
		t.Fatalf("snapshot frontier = %d, want %d", got, want)
	}
	scratchSnapshot := d.snapshotRelexStateWithScratch(&dfaRelexSnapshotScratch{})
	if got, want := scratchSnapshot.externalLookaheadEndByte, uint32(17); got != want {
		t.Fatalf("scratch snapshot frontier = %d, want %d", got, want)
	}

	d.externalLookaheadEndByte = 31
	changed := d.snapshotRelexState()
	if !snapshot.equal(&changed) {
		t.Fatal("transient external frontiers changed semantic snapshot equality")
	}
	snapshot.restore(d)
	if got, want := d.externalLookaheadEndByte, uint32(17); got != want {
		t.Fatalf("restored frontier = %d, want %d", got, want)
	}
}

// TestDFARelexSnapshotEqualIgnoresStaleZeroWidthCache proves equal() treats
// two dead extZero* caches as equivalent, even when their raw contents
// differ, and still detects a real difference once both caches are live.
//
// extZeroPos/extZeroState/extZeroTried (dfaRelexSnapshot) cache which
// external symbols Next already tried as a zero-width result at one exact
// (byte position, parser state) pair. Every real reader gates on the live
// lexer sitting at that same position before trusting the cache at all
// (probeZeroWidthExternalTokenForLexState and Next's own zero-width retry
// loop both check extZeroPos == lexerPos first), so a snapshot whose
// extZeroPos no longer equals its own lexerPos carries a stale,
// already-ignored mask left over from an earlier position, not live data.
// Comparing two such stale masks byte-for-byte used to treat two heads that
// reached the identical live state by different paths as different, purely
// because one of them tried and discarded a zero-width external symbol at
// some earlier, now-irrelevant position the other never visited: an
// owned-dispatch head that took a zero-width shift carries this stale mask
// forever afterward, since only a fresh zero-width attempt at the CURRENT
// position ever clears or rewrites it.
func TestDFARelexSnapshotEqualIgnoresStaleZeroWidthCache(t *testing.T) {
	a := dfaRelexSnapshot{lexerPos: 5, extZeroPos: 2, extZeroState: 9, extZeroTried: []bool{true, false}}
	b := dfaRelexSnapshot{lexerPos: 5, extZeroPos: 3, extZeroState: 1, extZeroTried: []bool{false}}
	if !a.equal(&b) {
		t.Fatal("equal() treated two stale extZero* caches as a real difference")
	}

	// Once extZeroPos matches lexerPos the cache is live, and a genuine
	// difference in it must still be detected.
	activeA := dfaRelexSnapshot{lexerPos: 5, extZeroPos: 5, extZeroState: 1, extZeroTried: []bool{true}}
	activeB := dfaRelexSnapshot{lexerPos: 5, extZeroPos: 5, extZeroState: 2, extZeroTried: []bool{true}}
	if activeA.equal(&activeB) {
		t.Fatal("equal() ignored a genuine difference in a live extZero* cache")
	}
}

func TestNoLiveStackCanAcceptLookaheadRequiresEveryEligibleVersionToReject(t *testing.T) {
	lang := &Language{
		StateCount:  4,
		SymbolCount: 3,
		ParseTable: [][]uint16{
			make([]uint16, 3),
			make([]uint16, 3),
			make([]uint16, 3),
			make([]uint16, 3),
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
		},
	}
	lang.ParseTable[2][1] = 1
	p := NewParser(lang)
	stacks := []glrStack{
		{entries: []stackEntry{{state: 1}}},
		{entries: []stackEntry{{state: 2}}},
	}
	tok := Token{Symbol: 1, StartByte: 10, EndByte: 11}

	if p.noLiveStackCanAcceptLookahead(stacks, tok) {
		t.Fatal("mixed frontier accepted replacement even though state 2 can consume the real lookahead")
	}
	stacks[1].shifted = true
	if !p.noLiveStackCanAcceptLookahead(stacks, tok) {
		t.Fatal("shifted versions should not block replacement for the remaining rejecting frontier")
	}
}

func TestTryRelexSingleParserStateAcceptsOnlyZeroWidthExternalShift(t *testing.T) {
	lang := zeroWidthRelexLanguage()
	p := NewParser(lang)
	stacks := []glrStack{
		{entries: []stackEntry{{state: 1}}},
		{entries: []stackEntry{{state: 2}}},
	}
	original := Token{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}}
	externalValidByState := make([][]uint16, 4)
	externalValidByState[1] = []uint16{0}
	ts := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte("x")), lang, p.lookupActionIndex, nil, externalValidByState, nil)
	ts.lexer.pos = 1
	ts.lexer.col = 1
	ts.state = 2
	ts.glrStates = []StateID{1, 2}
	if ts.usesExternalCheckpoints {
		t.Fatal("stateless scanner unexpectedly enabled checkpoints")
	}
	if !ts.CanRelexFromTokenStart(original) {
		t.Fatal("generic relex rejected a stateless external scanner")
	}

	got, ok := p.tryRelexSingleParserState(original, 1, ts, stacks, &parserScratch{}, nil)
	if !ok {
		t.Fatal("zero-width external shift was rejected")
	}
	if got.Symbol != 3 || got.StartByte != 0 || got.EndByte != 0 {
		t.Fatalf("relexed token = %+v, want zero-width external symbol 3 at byte 0", got)
	}
	if ts.state != 1 {
		t.Fatalf("parser state = %d, want isolated state 1", ts.state)
	}
	if len(ts.glrStates) != 0 {
		t.Fatalf("GLR states = %v, want cleared frontier", ts.glrStates)
	}
	if ts.lexer.pos != 0 {
		t.Fatalf("lexer position = %d, want committed zero-width position 0", ts.lexer.pos)
	}
}

func TestPeekZeroWidthExternalShiftForStateWorksThroughSetIncludedRanges(t *testing.T) {
	lang := &Language{
		Name:            "javascript",
		StateCount:      4,
		SymbolCount:     4,
		TokenCount:      4,
		SymbolNames:     []string{"end", "original", "candidate", "_automatic_semicolon"},
		ExternalSymbols: []Symbol{3},
		ExternalScanner: zeroWidthRelexExternalScanner{},
		LexStates: []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: 'x', Hi: 'x', NextState: 1},
				},
			},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{{LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}},
		ParseTable: [][]uint16{
			make([]uint16, 4),
			make([]uint16, 4),
			make([]uint16, 4),
			make([]uint16, 4),
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
		},
	}
	lang.ParseTable[1][3] = 1
	p := NewParser(lang)
	p.SetIncludedRanges([]Range{{StartByte: 0, EndByte: 2, EndPoint: Point{Column: 2}}})
	externalValidByState := make([][]uint16, 4)
	externalValidByState[1] = []uint16{0}
	dts := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte("xx")), lang, p.lookupActionIndex, nil, externalValidByState, nil)
	dts.lexer.pos = 2
	dts.lexer.col = 2
	dts.state = 2
	dts.glrStates = []StateID{1, 2}
	ts := p.wrapIncludedRanges(dts)
	original := Token{
		Symbol:     1,
		StartByte:  1,
		EndByte:    2,
		StartPoint: Point{Column: 1},
		EndPoint:   Point{Column: 2},
	}

	got, action, ok := p.peekZeroWidthExternalShiftForState(original, 1, ts)
	if !ok {
		t.Fatal("zero-width external shift was rejected through the included-range wrapper")
	}
	if got.Symbol != 3 || got.StartByte != 1 || got.EndByte != 1 {
		t.Fatalf("relexed token = %+v, want zero-width external symbol 3 at byte 1", got)
	}
	if action.Type != ParseActionShift || action.State != 3 {
		t.Fatalf("action = %+v, want shift to state 3", action)
	}
	if dts.state != 2 || len(dts.glrStates) != 2 || dts.glrStates[0] != 1 || dts.glrStates[1] != 2 {
		t.Fatalf("parser/GLR states = %d/%v, want restored 2/[1 2]", dts.state, dts.glrStates)
	}
	if dts.lexer.pos != 2 || dts.lexer.col != 2 {
		t.Fatalf("lexer = pos %d col %d, want restored 2/2", dts.lexer.pos, dts.lexer.col)
	}
	allocs := testing.AllocsPerRun(100, func() {
		p.peekZeroWidthExternalShiftForState(original, 1, ts)
	})
	// Scanner snapshot/relex setup currently accounts for two allocations. The
	// saved GLR frontier must remain a slice-header restore, not add save and
	// restore copies to this per-version probe.
	if allocs > 2 {
		t.Fatalf("included-range zero-width probe allocations = %.1f, want at most 2", allocs)
	}
}

func TestTryRelexSingleParserStateRejectedCandidateRestoresDFATransaction(t *testing.T) {
	lang := &Language{
		Name:            "javascript",
		StateCount:      4,
		SymbolCount:     4,
		TokenCount:      4,
		SymbolNames:     []string{"end", "original", "candidate", "external"},
		ExternalSymbols: []Symbol{3},
		ExternalScanner: byteStateExternalScanner{},
		LexStates: []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: 'x', Hi: 'x', NextState: 1},
				},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: 'y', Hi: 'y', NextState: 2},
				},
			},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{{LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}},
		ParseTable: [][]uint16{
			make([]uint16, 4),
			make([]uint16, 4),
			make([]uint16, 4),
			make([]uint16, 4),
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
		},
	}
	lang.ParseTable[1][2] = 1
	p := NewParser(lang)
	stacks := []glrStack{
		{entries: []stackEntry{{state: 1}}},
		{entries: []stackEntry{{state: 2}}},
	}
	original := Token{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}}
	ts := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte("xy")), lang, p.lookupActionIndex, nil, nil, nil)
	ts.usesExternalCheckpoints = true
	ts.lastExternalTokenStartByte = 0
	ts.lastExternalTokenEndByte = 1
	ts.lastExternalTokenValid = true
	ts.externalTokenStart = append(ts.externalTokenStart[:0], 2)
	ts.externalTokenEnd = append(ts.externalTokenEnd[:0], 9)
	*ts.externalPayload.(*byte) = 9
	ts.lexer.pos = 1
	ts.lexer.row = 4
	ts.lexer.col = 7
	ts.state = 2
	ts.glrStates = []StateID{1, 2}
	ts.extZeroPos = 11
	ts.extZeroState = 2
	ts.extZeroTried = append(ts.extZeroTried[:0], true)
	ts.zeroWidthPos = 13
	ts.zeroWidthCount = 3

	if _, ok := p.tryRelexSingleParserState(original, 1, ts, stacks, &parserScratch{}, nil); ok {
		t.Fatal("non-zero-width replacement was accepted")
	}
	if ts.lexer.pos != 1 || ts.lexer.row != 4 || ts.lexer.col != 7 {
		t.Fatalf("lexer = pos %d row %d col %d, want restored 1/4/7", ts.lexer.pos, ts.lexer.row, ts.lexer.col)
	}
	if got := *ts.externalPayload.(*byte); got != 9 {
		t.Fatalf("external scanner state = %d, want restored 9", got)
	}
	if !ts.lastExternalTokenValid || ts.lastExternalTokenStartByte != 0 || ts.lastExternalTokenEndByte != 1 {
		t.Fatalf("last external token = valid %t start %d end %d, want true/0/1", ts.lastExternalTokenValid, ts.lastExternalTokenStartByte, ts.lastExternalTokenEndByte)
	}
	if got := ts.externalTokenStart; len(got) != 1 || got[0] != 2 {
		t.Fatalf("external token start checkpoint = %v, want [2]", got)
	}
	if got := ts.externalTokenEnd; len(got) != 1 || got[0] != 9 {
		t.Fatalf("external token end checkpoint = %v, want [9]", got)
	}
	if ts.extZeroPos != 11 || ts.extZeroState != 2 || len(ts.extZeroTried) != 1 || !ts.extZeroTried[0] {
		t.Fatalf("external zero-width guard = pos %d state %d tried %v, want 11/2/[true]", ts.extZeroPos, ts.extZeroState, ts.extZeroTried)
	}
	if ts.zeroWidthPos != 13 || ts.zeroWidthCount != 3 {
		t.Fatalf("zero-width guard = pos %d count %d, want 13/3", ts.zeroWidthPos, ts.zeroWidthCount)
	}
	if ts.state != 1 || len(ts.glrStates) != 2 || ts.glrStates[0] != 1 || ts.glrStates[1] != 2 {
		t.Fatalf("parser/GLR states = %d/%v, want current frontier 1/[1 2]", ts.state, ts.glrStates)
	}
}

// refusedRelexLanguage lexes `x` as symbol 2, which state 1 shifts as an
// extra, so both DFA re-lex helpers find symbol 2 for a lookahead lexed as 1.
func refusedRelexLanguage() *Language {
	lang := &Language{
		Name:        "refused_relex",
		StateCount:  4,
		SymbolCount: 3,
		TokenCount:  3,
		SymbolNames: []string{"end", "original", "candidate"},
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'x', Hi: 'x', NextState: 1}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes:   []LexMode{{LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}},
		ParseTable: [][]uint16{make([]uint16, 3), make([]uint16, 3), make([]uint16, 3), make([]uint16, 3)},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1, Extra: true}}},
		},
	}
	lang.ParseTable[1][2] = 1
	return lang
}

// zeroWidthRelexLanguage gives state 1 a single shift on the zero-width
// external symbol 3 that the scanner emits before `x`.
func zeroWidthRelexLanguage() *Language {
	lang := &Language{
		Name:            "javascript",
		StateCount:      4,
		SymbolCount:     4,
		TokenCount:      4,
		SymbolNames:     []string{"end", "original", "candidate", "_automatic_semicolon"},
		ExternalSymbols: []Symbol{3},
		ExternalScanner: zeroWidthRelexExternalScanner{},
		LexStates: []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: 'x', Hi: 'x', NextState: 1},
				},
			},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{{LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}},
		ParseTable: [][]uint16{
			make([]uint16, 4),
			make([]uint16, 4),
			make([]uint16, 4),
			make([]uint16, 4),
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
		},
	}
	lang.ParseTable[1][3] = 1
	return lang
}

// A reading the stack has already dispatched is refused, and the refusal must
// leave the lexer where it was, as a candidate the helper rejects itself
// does; otherwise the next token starts at the refused candidate's end.
func TestDFARelexRefusedReadingRestoresLexer(t *testing.T) {
	lang := refusedRelexLanguage()
	p := NewParser(lang)
	original := Token{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}}
	reading := Token{Symbol: 2, StartByte: 0, EndByte: 1}
	helpers := []struct {
		name  string
		relex func(Token, StateID, TokenSource, *stackRelexReadings) (Token, bool)
	}{
		{"current state", p.tryRelexCurrentStateDFA},
		{"broad", p.tryRelexBroadDFA},
	}
	for _, h := range helpers {
		for _, seen := range []bool{false, true} {
			ts := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte("xx")), lang, p.lookupActionIndex, nil, nil, nil)
			ts.lexer.pos, ts.lexer.row, ts.lexer.col = 2, 3, 4
			var readings stackRelexReadings
			if seen {
				readings.admit(1, reading)
			}
			got, ok := h.relex(original, 1, ts, &readings)
			if seen {
				if ok {
					t.Fatalf("%s: an already dispatched reading was returned: %+v", h.name, got)
				}
				if ts.lexer.pos != 2 || ts.lexer.row != 3 || ts.lexer.col != 4 {
					t.Fatalf("%s: lexer = %d/%d/%d after a refused reading, want 2/3/4", h.name, ts.lexer.pos, ts.lexer.row, ts.lexer.col)
				}
				continue
			}
			if !ok || got.Symbol != 2 || ts.lexer.pos != 1 {
				t.Fatalf("%s: ok=%v got %+v with lexer at %d, want symbol 2 with lexer at 1", h.name, ok, got, ts.lexer.pos)
			}
			if readings.n != 1 {
				t.Fatalf("%s: %d readings recorded, want 1", h.name, readings.n)
			}
		}
	}
}

// A zero-width shift the stack has already dispatched is refused, and the
// refusal must roll the scanner transaction back like any rejected probe.
func TestTryRelexSingleParserStateRefusedReadingRestoresDFATransaction(t *testing.T) {
	lang := zeroWidthRelexLanguage()
	p := NewParser(lang)
	stacks := []glrStack{
		{entries: []stackEntry{{state: 1}}},
		{entries: []stackEntry{{state: 2}}},
	}
	original := Token{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}}
	externalValidByState := make([][]uint16, 4)
	externalValidByState[1] = []uint16{0}
	ts := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte("x")), lang, p.lookupActionIndex, nil, externalValidByState, nil)
	ts.lexer.pos = 1
	ts.lexer.col = 1
	ts.state = 2
	ts.glrStates = []StateID{1, 2}

	var readings stackRelexReadings
	readings.admit(1, Token{Symbol: 3})
	if got, ok := p.tryRelexSingleParserState(original, 1, ts, stacks, &parserScratch{}, &readings); ok {
		t.Fatalf("an already dispatched reading was returned: %+v", got)
	}
	if ts.lexer.pos != 1 || ts.lexer.col != 1 {
		t.Fatalf("lexer = pos %d col %d, want restored 1/1", ts.lexer.pos, ts.lexer.col)
	}
}

// ownLexModeLanguage gives the byte "x" two readings with the same name "x",
// like C++'s binary and template-closing `>`: lex mode 0 reads it as the
// visible x_a (symbol 2), lex mode 1 as the hidden x_b (symbol 3). After `a`,
// state 0 forks into Q (state 1, lex mode 1, accepts both readings) and P
// (state 2, lex mode 0, accepts only x_a). The shared election picks the
// visible x_a, so Q reaches its own x_b only through the per-stack re-read.
// Each tail state reduces its two children and accepts at EOF; tests remove
// the EOF action of the branches that must die.
func ownLexModeLanguage() *Language {
	return &Language{
		Name: "own_lex_mode", SymbolCount: 7, TokenCount: 4, StateCount: 9, ProductionIDCount: 1,
		SymbolNames: []string{"end", "a", "x", "x", "rule_qa", "rule_qb", "rule_p"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"}, {Name: "a", Visible: true}, {Name: "x", Visible: true}, {Name: "x"},
			{Name: "rule_qa", Visible: true, Named: true}, {Name: "rule_qb", Visible: true, Named: true}, {Name: "rule_p", Visible: true, Named: true},
		},
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 2}, {Lo: 'x', Hi: 'x', NextState: 3}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'x', Hi: 'x', NextState: 4}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 2, Default: -1, EOF: -1},
			{AcceptToken: 3, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{{LexState: 0}, {LexState: 1}, {LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}, {Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 5}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 2}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 5, ChildCount: 2}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 6, ChildCount: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 6}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 7}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 8}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: [][]uint16{
			{0, 1, 0, 0, 8, 9, 10},
			{0, 0, 2, 3, 0, 0, 0}, // Q
			{0, 0, 4, 0, 0, 0, 0}, // P
			{5, 0, 0, 0, 0, 0, 0},
			{6, 0, 0, 0, 0, 0, 0},
			{7, 0, 0, 0, 0, 0, 0},
			{11, 0, 0, 0, 0, 0, 0},
			{11, 0, 0, 0, 0, 0, 0},
			{11, 0, 0, 0, 0, 0, 0},
		},
	}
}

func TestOwnLexModeTokenGuards(t *testing.T) {
	shared := Token{Symbol: 2, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}}
	for _, tc := range []struct {
		name   string
		source string
		edit   func(*Language, *Token)
		want   Symbol // 0: rejected, the shared token comes back
	}{
		{"accepted", "ax", func(*Language, *Token) {}, 3},
		{"external scanner token", "ax", func(_ *Language, tok *Token) { tok.ExternalScannerToken = true }, 0},
		{"shared token is keyword capture", "ax", func(lang *Language, _ *Token) { lang.KeywordCaptureToken = 2 }, 0},
		{"own reading is keyword capture", "ax", func(lang *Language, _ *Token) { lang.KeywordCaptureToken = 3 }, 0},
		{"reserved word", "ax", func(lang *Language, _ *Token) {
			lang.ReservedWords, lang.MaxReservedWordSetSize, lang.LexModes[1].ReservedWordSetID = []Symbol{0, 2}, 1, 1
		}, 0},
		{"different span", "axx", func(_ *Language, tok *Token) { tok.EndByte, tok.EndPoint.Column = 3, 3 }, 0},
		{"no action for own reading", "ax", func(lang *Language, _ *Token) { lang.ParseTable[1][3] = 0 }, 0},
		{"own reading under another name", "ax", func(lang *Language, _ *Token) {
			lang.SymbolNames[3], lang.SymbolMetadata[3].Name = "y", "y"
		}, 0},
		{"election scanned no rival at this span", "ax", func(_ *Language, tok *Token) { tok.StartByte, tok.StartPoint.Column = 0, 0 }, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := ownLexModeLanguage()
			tok := shared
			tc.edit(lang, &tok)
			p := NewParser(lang)
			source := []byte(tc.source)
			dts := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, p.lookupActionIndex, nil, nil, nil)
			defer dts.Close()
			// P's lex mode won the union election; it also scanned Q's own x_b.
			dts.glrUnionScanScratch = append(dts.glrUnionScanScratch[:0],
				glrUnionDFAScan{state: 2, tok: tok},
				glrUnionDFAScan{state: 1, tok: Token{Symbol: 3, StartByte: 1, EndByte: 2}})
			got, ok := p.ownLexModeToken(source, 1, tok, tok.StartByte, nil, dts)
			if tc.want == 0 {
				if ok || got != tok {
					t.Fatalf("got (%+v, %v), want the shared token back unchanged", got, ok)
				}
				return
			}
			if !ok || got.Symbol != tc.want || got.StartByte != tok.StartByte || got.EndByte != tok.EndByte {
				t.Fatalf("got (%+v, %v), want symbol %d over [%d,%d)", got, ok, tc.want, tok.StartByte, tok.EndByte)
			}
		})
	}
}

// TestOwnLexModeTokenSkipsAReusableSharedToken: P's lex mode elected x_a, and
// Q reads no token of its own where ts_parser__can_reuse_first_leaf lets C
// reuse x_a in Q's state.
func TestOwnLexModeTokenSkipsAReusableSharedToken(t *testing.T) {
	shared := Token{Symbol: 2, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}}
	for _, tc := range []struct {
		name string
		edit func(*Language)
		want Symbol // 0: rejected, the shared token comes back
	}{
		{"lexed in another lex mode", func(*Language) {}, 3},
		{"lexed in the stack's lex mode", func(lang *Language) { lang.LexModes[2] = lang.LexModes[1] }, 0},
		{"reusable entry", func(lang *Language) { lang.ParseActions[lang.ParseTable[1][2]].Reusable = true }, 0},
		{"reusable entry, external lex state", func(lang *Language) {
			lang.ParseActions[lang.ParseTable[1][2]].Reusable = true
			lang.LexModes[1].ExternalLexState = 1
		}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := ownLexModeLanguage()
			lang.ParseTable[1][2] = 2 // Q has an action for x_a too
			tc.edit(lang)
			p := NewParser(lang)
			source := []byte("ax")
			dts := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, p.lookupActionIndex, nil, nil, nil)
			defer dts.Close()
			dts.glrUnionScanScratch = append(dts.glrUnionScanScratch[:0],
				glrUnionDFAScan{state: 2, tok: shared},
				glrUnionDFAScan{state: 1, tok: Token{Symbol: 3, StartByte: 1, EndByte: 2}})
			got, ok := p.ownLexModeToken(source, 1, shared, shared.StartByte, nil, dts)
			if tc.want == 0 {
				if ok || got != shared {
					t.Fatalf("got (%+v, %v), want the shared token back unchanged", got, ok)
				}
				return
			}
			if !ok || got.Symbol != tc.want {
				t.Fatalf("got (%+v, %v), want symbol %d", got, ok, tc.want)
			}
		})
	}
}

// TestOwnLexModeTokenReadsFromStackPosition re-reads "x" in "a x" from the
// stack's position 1, before the space, as C lexes that version. Q's lex mode
// reads x_b only if it skips the space itself.
func TestOwnLexModeTokenReadsFromStackPosition(t *testing.T) {
	shared := Token{Symbol: 2, StartByte: 2, EndByte: 3, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 3}}
	skipSpace := func(lang *Language) {
		lang.LexStates[1].Transitions = append(lang.LexStates[1].Transitions, LexTransition{Lo: ' ', Hi: ' ', NextState: 1, Skip: true})
	}
	skipSpaceToken := func(lang *Language) {
		lang.LexStates = append(lang.LexStates, LexState{Skip: true, Default: -1, EOF: -1})
		lang.LexStates[1].Transitions = append(lang.LexStates[1].Transitions, LexTransition{Lo: ' ', Hi: ' ', NextState: len(lang.LexStates) - 1})
	}
	immediateXB := func(lang *Language) { lang.ImmediateTokens = []bool{false, false, false, true} }
	for _, tc := range []struct {
		name string
		from uint32
		edit []func(*Language)
		want Symbol // 0: rejected, the shared token comes back
	}{
		{"mode skips the space", 1, []func(*Language){skipSpace}, 3},
		{"mode does not skip the space", 1, nil, 0},
		{"stack past the token start", 3, []func(*Language){skipSpace}, 0},
		{"mode has its own after-whitespace start", 1, []func(*Language){skipSpace, func(lang *Language) { lang.LexModes[1].AfterWhitespaceLexState = 1 }}, 0},
		{"skip token, then a reading", 1, []func(*Language){skipSpaceToken}, 3},
		{"skip token, then an immediate reading", 1, []func(*Language){skipSpaceToken, immediateXB}, 0},
		{"skipped space, then an immediate reading", 1, []func(*Language){skipSpace, immediateXB}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := ownLexModeLanguage()
			for _, edit := range tc.edit {
				edit(lang)
			}
			p := NewParser(lang)
			source := []byte("a x")
			dts := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, p.lookupActionIndex, nil, nil, nil)
			defer dts.Close()
			// P's lex mode won the union election; it also scanned Q's own x_b.
			dts.glrUnionScanScratch = append(dts.glrUnionScanScratch[:0],
				glrUnionDFAScan{state: 2, tok: shared},
				glrUnionDFAScan{state: 1, tok: Token{Symbol: 3, StartByte: 2, EndByte: 3}})
			got, ok := p.ownLexModeToken(source, 1, shared, tc.from, nil, dts)
			if tc.want == 0 {
				if ok || got != shared {
					t.Fatalf("got (%+v, %v), want the shared token back unchanged", got, ok)
				}
				return
			}
			if !ok || got.Symbol != tc.want || got.StartByte != shared.StartByte || got.EndByte != shared.EndByte || got.StartPoint != shared.StartPoint {
				t.Fatalf("got (%+v, %v), want symbol %d over [%d,%d) at %v", got, ok, tc.want, shared.StartByte, shared.EndByte, shared.StartPoint)
			}
		})
	}
}

// TestOwnLexModeTokenAtDispatch parses "ax" through the Q/P fork. Q dispatches
// first: it must take its own x_b, and P must still get the shared x_a.
func TestOwnLexModeTokenAtDispatch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dead     []StateID // tail states whose EOF action is removed
		wantRoot string
	}{
		{"stack reads its own token", []StateID{3, 5}, "rule_qb"},
		{"next stack gets the shared token", []StateID{3, 4}, "rule_p"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := ownLexModeLanguage()
			for _, st := range tc.dead {
				lang.ParseTable[st][0] = 0
			}
			// Reserve x_b in P so the no-action re-lex cannot turn a leaked
			// x_b back into x_a: P survives only if it sees the shared token.
			lang.ReservedWords, lang.MaxReservedWordSetSize, lang.LexModes[2].ReservedWordSetID = []Symbol{0, 3}, 1, 1
			p := NewParser(lang)
			source := []byte("ax")
			ts := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, p.lookupActionIndex, nil, nil, nil)
			tree := p.parseInternal(source, ts, nil, nil, arenaClassFull, nil, 0, 0, 0, false)
			if tree == nil {
				t.Fatal("parse returned nil tree")
			}
			defer tree.Release()
			root := tree.RootNode()
			if got := root.Type(lang); got != tc.wantRoot || root.HasError() {
				t.Fatalf("root = %s (error %v), want %s without error", got, root.HasError(), tc.wantRoot)
			}
		})
	}
}

func keptTokenLanguage() *Language {
	return &Language{
		Name: "kept_token", SymbolCount: 7, TokenCount: 4, StateCount: 9, ProductionIDCount: 1,
		SymbolNames: []string{"end", "a", "x", "x", "n", "rule_xa", "rule_xb"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"}, {Name: "a", Visible: true}, {Name: "x", Visible: true}, {Name: "x"},
			{Name: "n", Visible: true, Named: true}, {Name: "rule_xa", Visible: true, Named: true}, {Name: "rule_xb", Visible: true, Named: true},
		},
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 2}, {Lo: 'x', Hi: 'x', NextState: 3}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'x', Hi: 'x', NextState: 4}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 2, Default: -1, EOF: -1},
			{AcceptToken: 3, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{{LexState: 0}, {LexState: 1}, {LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}, {Type: ParseActionShift, State: 2}}}, // 1: `a` forks into Q and P
			{Actions: []ParseAction{{Type: ParseActionShift, State: 8}}},                                     // 2: P on the shared reading
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 1}}},                    // 3: Q reduces on its own reading
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},                                     // 4: T on the shared reading
			{Actions: []ParseAction{{Type: ParseActionShift, State: 5}}},                                     // 5: T on Q's reading
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 5, ChildCount: 2}}},                    // 6
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 6, ChildCount: 2}}},                    // 7
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},                                     // 8: goto n -> T
			{Actions: []ParseAction{{Type: ParseActionShift, State: 6}}},                                     // 9: goto rule_xa
			{Actions: []ParseAction{{Type: ParseActionShift, State: 7}}},                                     // 10: goto rule_xb
			{Actions: []ParseAction{{Type: ParseActionAccept}}},                                              // 11
		},
		ParseTable: [][]uint16{
			{0, 1, 0, 0, 8, 9, 10},
			{0, 0, 0, 3, 0, 0, 0}, // Q (lex mode 1)
			{0, 0, 2, 0, 0, 0, 0}, // P
			{0, 0, 4, 5, 0, 0, 0}, // T
			{6, 0, 0, 0, 0, 0, 0},
			{7, 0, 0, 0, 0, 0, 0},
			{11, 0, 0, 0, 0, 0, 0},
			{11, 0, 0, 0, 0, 0, 0},
			{0, 0, 0, 0, 0, 0, 0},
		},
	}
}

func TestOwnTokenIsKeptThroughAReduction(t *testing.T) {
	lang := keptTokenLanguage()
	p := NewParser(lang)
	source := []byte("ax")
	ts := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, p.lookupActionIndex, nil, nil, nil)
	tree := p.parseInternal(source, ts, nil, nil, arenaClassFull, nil, 0, 0, 0, false)
	if tree == nil {
		t.Fatal("nil tree")
	}
	defer tree.Release()
	if got := tree.RootNode().Type(lang); got != "rule_xb" {
		t.Fatalf("root = %s, want rule_xb: Q reduced on its own reading and must shift it too", got)
	}
}

// TestRelexTokenForStackLexStateNoAction covers the no-action re-lex that
// shares its scan with ownLexModeToken: P (lex mode 0) has no action for a
// shared x_b, so it takes its own x_a when it has an action for it and keeps
// the shared token when it does not. A failed DFA scan still falls through to
// the zero-width external rescue.
func TestRelexTokenForStackLexStateNoAction(t *testing.T) {
	shared := Token{Symbol: 3, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}}
	for _, hasAction := range []bool{true, false} {
		lang := ownLexModeLanguage()
		if !hasAction {
			lang.ParseTable[2][2] = 0
		}
		p := NewParser(lang)
		source := []byte("ax")
		dts := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, p.lookupActionIndex, nil, nil, nil)
		got, state, ok := p.relexTokenForStackLexState(source, 2, shared, nil, dts, nil, nil, nil, nil, nil, nil)
		dts.Close()
		if hasAction && (!ok || state != 2 || got.Symbol != 2 || got.StartByte != 1 || got.EndByte != 2) {
			t.Fatalf("with action: got (%+v, %d, %v), want x_a in state 2", got, state, ok)
		}
		if !hasAction && (ok || got != shared) {
			t.Fatalf("without action: got (%+v, %v), want the shared token back", got, ok)
		}
	}

	f := newZeroWidthRelexWitnessFixture(t, perlNonassocWitnessLanguage())
	got, state, ok := f.p.relexTokenForStackLexState(f.source, f.starved.top().state, f.tok, nil, f.dts, f.starved,
		&f.nodeCount, f.arena, f.scratch, &f.trackChildErrors, &f.rescueBudget)
	if !ok || state != 3 || got != f.tok {
		t.Fatalf("rescue: got (%+v, %d, %v), want the shared token in rescued state 3", got, state, ok)
	}
}
