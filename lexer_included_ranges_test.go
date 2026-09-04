package gotreesitter

import "testing"

var includedRangeLexerTokenSink Token

func TestLexerIncludedRangesNormalizeAtExactStartUsesCustomPoint(t *testing.T) {
	lex := NewLexer(nil, []byte("xabc"))
	lex.setIncludedRanges([]Range{{
		StartByte:  1,
		EndByte:    4,
		StartPoint: Point{Row: 7, Column: 9},
		EndPoint:   Point{Row: 7, Column: 12},
	}})
	cursor := includedLexerCursor{pos: 1, row: 0, col: 1, rangeIdx: 0}
	lex.normalizeIncludedCursor(&cursor)
	if cursor.pos != 1 || cursor.row != 7 || cursor.col != 9 {
		t.Fatalf("cursor at included start = %+v, want byte 1 at row 7 column 9", cursor)
	}
}

func TestLexerIncludedRangesLogicalPointAtPositionUsesRangeCoordinates(t *testing.T) {
	lex := NewLexer(nil, []byte("xabc_yz"))
	lex.setIncludedRanges([]Range{
		{StartByte: 1, EndByte: 3, StartPoint: Point{Row: 7, Column: 9}, EndPoint: Point{Row: 7, Column: 11}},
		{StartByte: 5, EndByte: 7, StartPoint: Point{Row: 10, Column: 4}, EndPoint: Point{Row: 10, Column: 6}},
	})
	tests := []struct {
		pos  int
		want Point
	}{
		{pos: 1, want: Point{Row: 7, Column: 9}},
		{pos: 2, want: Point{Row: 7, Column: 10}},
		{pos: 3, want: Point{Row: 7, Column: 11}},
		{pos: 4, want: Point{Row: 7, Column: 11}},
		{pos: 5, want: Point{Row: 10, Column: 4}},
		{pos: 6, want: Point{Row: 10, Column: 5}},
	}
	for _, test := range tests {
		got, ok := lex.includedPointAtPosition(test.pos)
		if !ok || got != test.want {
			t.Fatalf("logical point at byte %d = %+v exact=%t, want %+v", test.pos, got, ok, test.want)
		}
	}
}

func TestLexerIncludedRangesRespectTokenBoundaries(t *testing.T) {
	lex := NewLexer(buildIdentNumberWSDFA(), []byte("xxalpha"))
	lex.setIncludedRanges([]Range{{
		StartByte:  2,
		EndByte:    5,
		StartPoint: Point{Column: 2},
		EndPoint:   Point{Column: 5},
	}})

	tok := lex.Next(0)
	if tok.Symbol != 1 || tok.Text != "alp" {
		t.Fatalf("token = (%d, %q), want identifier alp", tok.Symbol, tok.Text)
	}
	if tok.StartByte != 2 || tok.EndByte != 5 {
		t.Fatalf("token bytes = [%d,%d), want [2,5)", tok.StartByte, tok.EndByte)
	}

	eof := lex.Next(0)
	if eof.Symbol != 0 || eof.StartByte != 5 || eof.EndByte != 5 {
		t.Fatalf("EOF = %+v, want byte 5", eof)
	}
}

func TestLexerIncludedRangesPreserveInspectedLookaheadEnd(t *testing.T) {
	states := []LexState{
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '=', Hi: '=', NextState: 1}}},
		{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'i', Hi: 'i', NextState: 2}}},
		{Default: -1, EOF: -1},
	}
	lex := NewLexer(states, []byte("\n=i"))
	lex.setIncludedRanges([]Range{{
		StartByte: 1, EndByte: 3,
		StartPoint: Point{Row: 1}, EndPoint: Point{Row: 1, Column: 2},
	}})
	token := lex.Next(0)
	if token.StartByte != 1 || token.EndByte != 2 || tokenLookaheadEndByte(token) != 4 {
		t.Fatalf("token bytes=%d..%d lookahead_end=%d, want 1..2 and 4", token.StartByte, token.EndByte, tokenLookaheadEndByte(token))
	}
}

func TestLexerIncludedRangesAddsInvalidUTF8Lookahead(t *testing.T) {
	states := []LexState{
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 1}}},
		{AcceptToken: 1, Default: -1, EOF: -1},
	}
	lex := NewLexer(states, []byte{'a', 0xff})
	lex.setIncludedRanges([]Range{{StartByte: 0, EndByte: 2}})
	token := lex.Next(0)
	if token.Symbol != 1 || token.StartByte != 0 || token.EndByte != 1 || tokenLookaheadEndByte(token) != 6 {
		t.Fatalf("token=%+v lookahead_end=%d, want invalid UTF-8 frontier 6", token, tokenLookaheadEndByte(token))
	}
}

func TestLexerIncludedRangesEndTokenBeforeNextRange(t *testing.T) {
	lex := NewLexer(buildIdentNumberWSDFA(), []byte("ab+12"))
	lex.setIncludedRanges([]Range{
		{StartByte: 0, EndByte: 2, EndPoint: Point{Column: 2}},
		{StartByte: 3, EndByte: 5, StartPoint: Point{Column: 3}, EndPoint: Point{Column: 5}},
	})

	first := lex.Next(0)
	if first.Text != "ab" || first.StartByte != 0 || first.EndByte != 2 {
		t.Fatalf("first token = %+v, want ab at [0,2)", first)
	}
	second := lex.Next(0)
	if second.Symbol != 2 || second.Text != "12" || second.StartByte != 3 || second.EndByte != 5 {
		t.Fatalf("second token = %+v, want number 12 at [3,5)", second)
	}
}

func TestLexerIncludedRangesBuildLogicalCrossRangeText(t *testing.T) {
	lex := NewLexer(buildIdentNumberWSDFA(), []byte("ab__cd"))
	lex.setIncludedRanges([]Range{
		{StartByte: 0, EndByte: 2, EndPoint: Point{Column: 2}},
		{StartByte: 4, EndByte: 6, StartPoint: Point{Column: 4}, EndPoint: Point{Column: 6}},
	})

	tok := lex.Next(0)
	if tok.Symbol != 1 || tok.Text != "abcd" {
		t.Fatalf("token = (%d, %q), want identifier abcd", tok.Symbol, tok.Text)
	}
	if tok.StartByte != 0 || tok.EndByte != 6 {
		t.Fatalf("token bytes = [%d,%d), want [0,6)", tok.StartByte, tok.EndByte)
	}
}

func TestLexerIncludedRangesUseUTF8PointsAcrossRanges(t *testing.T) {
	states := []LexState{
		{Default: 1, EOF: -1},
		{AcceptToken: 1, Default: 1, EOF: -1},
	}
	lex := NewLexer(states, []byte("éx\n__yz"))
	lex.setIncludedRanges([]Range{
		{
			StartByte:  2,
			EndByte:    4,
			StartPoint: Point{Column: 2},
			EndPoint:   Point{Row: 1},
		},
		{
			StartByte:  6,
			EndByte:    8,
			StartPoint: Point{Row: 1, Column: 2},
			EndPoint:   Point{Row: 1, Column: 4},
		},
	})

	tok := lex.Next(0)
	if tok.Text != "x\nyz" {
		t.Fatalf("token text = %q, want %q", tok.Text, "x\nyz")
	}
	if tok.StartPoint != (Point{Column: 2}) {
		t.Fatalf("start point = %+v, want row 0 column 2", tok.StartPoint)
	}
	if tok.EndPoint != (Point{Row: 1, Column: 4}) {
		t.Fatalf("end point = %+v, want row 1 column 4", tok.EndPoint)
	}
}

func TestLexerIncludedRangesClampPastSourceToEOF(t *testing.T) {
	lex := NewLexer(buildIdentNumberWSDFA(), []byte("a\nβ"))
	lex.setIncludedRanges([]Range{{
		StartByte:  99,
		EndByte:    100,
		StartPoint: Point{Row: 99, Column: 99},
		EndPoint:   Point{Row: 99, Column: 100},
	}})

	tok := lex.Next(0)
	if tok.Symbol != 0 || tok.StartByte != 4 || tok.EndByte != 4 {
		t.Fatalf("EOF = %+v, want byte 4", tok)
	}
	wantPoint := Point{Row: 1, Column: 2}
	if tok.StartPoint != wantPoint || tok.EndPoint != wantPoint {
		t.Fatalf("EOF points = %+v-%+v, want %+v", tok.StartPoint, tok.EndPoint, wantPoint)
	}
}

func TestLexerIncludedRangesIgnorePastSourceRangeAfterValidRange(t *testing.T) {
	lex := NewLexer(buildIdentNumberWSDFA(), []byte("a___"))
	lex.setIncludedRanges([]Range{
		{StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
		{StartByte: 99, EndByte: 100, StartPoint: Point{Row: 99}, EndPoint: Point{Row: 100}},
	})

	tok := lex.Next(0)
	if tok.Text != "a" || tok.StartByte != 0 || tok.EndByte != 1 {
		t.Fatalf("token = %+v, want a at [0,1)", tok)
	}
	eof := lex.Next(0)
	if eof.StartByte != 1 || eof.EndByte != 1 || eof.StartPoint != (Point{Column: 1}) {
		t.Fatalf("EOF = %+v, want the last valid range end", eof)
	}
}

func TestLexerIncludedRangesBuildLogicalErrorRun(t *testing.T) {
	lex := NewLexer(buildIdentNumberWSDFA(), []byte("@__#a"))
	lex.hasErrorRunLexState = true
	lex.errorRunLexState = 0
	lex.setIncludedRanges([]Range{
		{StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
		{StartByte: 3, EndByte: 5, StartPoint: Point{Column: 3}, EndPoint: Point{Column: 5}},
	})

	errTok := lex.NextWithErrorRuns(0)
	if errTok.Symbol != errorSymbol || errTok.Text != "@#" {
		t.Fatalf("error token = (%d, %q), want logical error text @#", errTok.Symbol, errTok.Text)
	}
	if errTok.StartByte != 0 || errTok.EndByte != 4 {
		t.Fatalf("error token bytes = [%d,%d), want [0,4)", errTok.StartByte, errTok.EndByte)
	}
	next := lex.NextWithErrorRuns(0)
	if next.Symbol != 1 || next.Text != "a" || next.StartByte != 4 || next.EndByte != 5 {
		t.Fatalf("next token = %+v, want a at [4,5)", next)
	}
}

func TestLexerNoIncludedRangesKeepsNextAllocationFree(t *testing.T) {
	lex := NewLexer(buildIdentNumberWSDFA(), []byte("alpha"))
	allocs := testing.AllocsPerRun(1000, func() {
		lex.pos = 0
		lex.row = 0
		lex.col = 0
		includedRangeLexerTokenSink = lex.Next(0)
	})
	if allocs != 0 {
		t.Fatalf("ordinary Next allocations = %.1f, want 0", allocs)
	}
}

func newIncludedRangeTestDFASource(source []byte) *dfaTokenSource {
	states := buildIdentNumberWSDFA()
	lang := &Language{
		LexStates: states,
		LexModes:  []LexMode{{LexState: 0}},
	}
	return newDFATokenSourceDirect(NewLexer(states, source), lang, nil, nil, nil, nil)
}

func TestDFATokenSourceIncludedRangesResetAndRelexRestoreCursor(t *testing.T) {
	ranges := []Range{{
		StartByte:  2,
		EndByte:    5,
		StartPoint: Point{Column: 2},
		EndPoint:   Point{Column: 5},
	}}
	d := newIncludedRangeTestDFASource([]byte("xxabc"))
	if !d.setIncludedRanges(ranges) {
		t.Fatal("DFA source rejected included ranges")
	}
	first := d.Next()
	if first.Text != "abc" {
		t.Fatalf("first token = %+v, want abc", first)
	}

	d.Reset([]byte("xxdef"))
	reset := d.Next()
	if reset.Text != "def" || reset.StartByte != 2 || reset.EndByte != 5 {
		t.Fatalf("reset token = %+v, want def at [2,5)", reset)
	}

	d.lexer.pos = 2
	d.lexer.row = 0
	d.lexer.col = 2
	d.lexer.includedRangeIdx = 0
	before := d.snapshotRelexState()
	if _, ok := d.RelexFromTokenStart(Token{
		StartByte:  1,
		EndByte:    2,
		StartPoint: Point{Column: 1},
		EndPoint:   Point{Column: 2},
	}); ok {
		t.Fatal("relex accepted a token outside the selected range")
	}
	if d.lexer.pos != before.lexerPos || d.lexer.includedRangeIdx != before.lexerRangeIdx {
		t.Fatalf("relex rollback cursor = byte %d range %d, want byte %d range %d",
			d.lexer.pos, d.lexer.includedRangeIdx, before.lexerPos, before.lexerRangeIdx)
	}
}

func TestDFATokenSourceIncludedRangesRestoreGLRScans(t *testing.T) {
	states := buildIdentNumberWSDFA()
	states = append(states, LexState{
		Default: -1,
		EOF:     -1,
		Transitions: []LexTransition{
			{Lo: 'a', Hi: 'z', NextState: 1},
		},
	})
	lang := &Language{
		LexStates: states,
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 4},
		},
		ParseActions:   []ParseActionEntry{{}, {}},
		SymbolMetadata: []SymbolMetadata{{}, {Visible: true}},
	}
	d := newDFATokenSourceDirect(NewLexer(states, []byte("xxabc")), lang, func(StateID, Symbol) uint16 {
		return 1
	}, nil, nil, nil)
	if !d.setIncludedRanges([]Range{{
		StartByte:  2,
		EndByte:    5,
		StartPoint: Point{Column: 2},
		EndPoint:   Point{Column: 5},
	}}) {
		t.Fatal("DFA source rejected included ranges")
	}

	startPos := d.lexer.pos
	startRangeIdx := d.lexer.includedRangeIdx
	tok, _, _, _ := d.scanDFATokenForState(0, 0)
	if tok.Text != "abc" {
		t.Fatalf("speculative token = %+v, want abc", tok)
	}
	if d.lexer.pos != startPos || d.lexer.includedRangeIdx != startRangeIdx {
		t.Fatalf("speculative scan cursor = byte %d range %d, want byte %d range %d",
			d.lexer.pos, d.lexer.includedRangeIdx, startPos, startRangeIdx)
	}

	d.state = 0
	d.glrStates = []StateID{0, 1}
	winner, ok := d.nextGLRUnionDFAToken()
	if !ok || winner.Text != "abc" {
		t.Fatalf("GLR winner = (%+v, %t), want abc", winner, ok)
	}
	if d.lexer.pos != 5 || d.lexer.includedRangeIdx != 1 {
		t.Fatalf("GLR winner cursor = byte %d range %d, want byte 5 range 1",
			d.lexer.pos, d.lexer.includedRangeIdx)
	}

	d.SeekTokenFrontier(0, Point{})
	if d.lexer.pos != 2 || d.lexer.includedRangeIdx != 0 {
		t.Fatalf("frontier cursor = byte %d range %d, want byte 2 range 0",
			d.lexer.pos, d.lexer.includedRangeIdx)
	}
}
