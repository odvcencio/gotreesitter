package gotreesitter

import "testing"

func TestDFARelexSnapshotRestoresFailedAttemptEnd(t *testing.T) {
	d := &dfaTokenSource{lexer: NewLexer([]LexState{{Default: -1, EOF: -1}}, []byte("x"))}
	want := includedLexerCursor{pos: 17, row: 2, col: 3, rangeIdx: 4}
	d.lexer.failTokenEnd = want
	snapshots := []dfaRelexSnapshot{
		d.snapshotRelexState(),
		d.snapshotRelexStateWithScratch(&dfaRelexSnapshotScratch{}),
	}
	for _, snapshot := range snapshots {
		if snapshot.failTokenEnd != want {
			t.Fatalf("snapshot lost failure cursor: %+v", snapshot.failTokenEnd)
		}
		d.lexer.failTokenEnd = includedLexerCursor{pos: 31}
		if snapshot.equal(d.snapshotRelexState()) {
			t.Fatal("snapshot equality ignored the failure cursor")
		}
		snapshot.restore(d)
		if d.lexer.failTokenEnd != want {
			t.Fatalf("rollback lost failure cursor: %+v", d.lexer.failTokenEnd)
		}
	}
}

func TestLexerFailedErrorAttemptEOFAndMultiline(t *testing.T) {
	states := []LexState{
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '\'', Hi: '\'', NextState: 1}}},
		{Default: -1, EOF: -1, Transitions: []LexTransition{
			{Lo: '}', Hi: '}', NextState: 1},
			{Lo: '\n', Hi: '\n', NextState: 1},
		}},
	}
	for _, source := range []string{"'}", "'\n}"} {
		t.Run(source, func(t *testing.T) {
			lex := NewLexer(states, []byte(source))
			lex.hasErrorRunLexState, lex.errorModeRetry = true, true
			wantPoint := Point{Column: 2}
			if len(source) == 3 {
				wantPoint = Point{Row: 1, Column: 1}
			}
			token := lex.NextWithErrorRuns(0)
			if token.Symbol != errorSymbol || token.Text != source || token.StartByte != 0 || token.EndByte != uint32(len(source)) || token.EndPoint != wantPoint {
				t.Fatalf("failed EOF attempt=%+v", token)
			}
			eof := lex.NextWithErrorRuns(0)
			if eof.Symbol != 0 || eof.StartByte != uint32(len(source)) || eof.EndByte != eof.StartByte || eof.StartPoint != wantPoint {
				t.Fatalf("EOF=%+v", eof)
			}
		})
	}
}

func TestLexerErrorRunPreservesFailedAttemptBytes(t *testing.T) {
	states := []LexState{
		{Default: -1, EOF: -1, Transitions: []LexTransition{
			{Lo: '\'', Hi: '\'', NextState: 1},
			{Lo: '}', Hi: '}', NextState: 3},
			{Lo: 'a', Hi: 'a', NextState: 3},
		}},
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '}', Hi: '}', NextState: 2}}},
		{Default: -1, EOF: -1},
		{AcceptToken: 1, Default: -1, EOF: -1},
	}
	for _, included := range []bool{false, true} {
		name := "contiguous"
		source, end := "'}a", uint32(2)
		if included {
			name, source, end = "included_ranges", "'__}a", 4
		}
		t.Run(name, func(t *testing.T) {
			lex := NewLexer(states, []byte(source))
			lex.hasErrorRunLexState = true
			lex.errorModeRetry = true
			if included {
				lex.setIncludedRanges([]Range{
					{StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
					{StartByte: 3, EndByte: 5, StartPoint: Point{Column: 3}, EndPoint: Point{Column: 5}},
				})
			}
			token := lex.NextWithErrorRuns(0)
			if token.Symbol != errorSymbol || token.Text != "'}" || token.StartByte != 0 || token.EndByte != end || token.EndPoint != (Point{Column: end}) {
				t.Fatalf("failed attempt lost consumed bytes: %+v", token)
			}
			next := lex.NextWithErrorRuns(0)
			if next.Symbol != 1 || next.Text != "a" || next.StartByte != end || next.EndByte != end+1 {
				t.Fatalf("next token=%+v", next)
			}
		})
	}
}
