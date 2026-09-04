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
	if snapshot.equal(changed) {
		t.Fatal("snapshots with different external frontiers compare equal")
	}
	snapshot.restore(d)
	if got, want := d.externalLookaheadEndByte, uint32(17); got != want {
		t.Fatalf("restored frontier = %d, want %d", got, want)
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

	got, ok := p.tryRelexSingleParserState(original, 1, ts, stacks, &parserScratch{})
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

	if _, ok := p.tryRelexSingleParserState(original, 1, ts, stacks, &parserScratch{}); ok {
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
