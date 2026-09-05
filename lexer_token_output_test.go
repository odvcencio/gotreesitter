package gotreesitter

import "testing"

func poisonedLexerOutputToken() Token {
	return Token{
		Symbol: 91, Text: "previous token", StartByte: 92, EndByte: 93,
		StartPoint: Point{Row: 94, Column: 95}, EndPoint: Point{Row: 96, Column: 97},
		Missing: true, lexerLookaheadEndByte: 98,
		missingStackByte: 99, missingStackPoint: Point{Row: 100, Column: 101},
		missingDependencyExact: true, NoLookahead: true,
		ExternalScannerToken: true, ExternalScannerStartByte: 102,
		lexerSkippedPrefix: true, lexerSkippedPrefixStart: 103,
		lexerErrorModeLexed: true, lexerInternalDFALexed: true, isKeyword: true,
	}
}

func TestLexerScanIntoReplacesPoisonedOutput(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		state      uint32
		skipPrefix bool
		want       Token
		accepted   bool
		position   int
		failed     bool
		failure    int
		readSpan   uint32
	}{
		{
			name: "accepted", source: "ab!", accepted: true, position: 2, readSpan: 3,
			want: Token{Symbol: 1, Text: "ab", EndByte: 2, EndPoint: Point{Column: 2},
				lexerInternalDFALexed: true, lexerLookaheadEndByte: 3},
		},
		{
			name: "skip", source: " ab!", accepted: true, position: 1, readSpan: 2,
			want: Token{EndByte: 1, EndPoint: Point{Column: 1}, lexerLookaheadEndByte: 2},
		},
		{
			name: "failed", source: "!", failed: true, readSpan: 1,
			want: Token{lexerLookaheadEndByte: 1},
		},
		{name: "invalid_state", source: "ab!", state: 4},
		{name: "negative_state", source: "ab!", state: ^uint32(0)},
		{
			name: "unicode_frontier", source: "abé", accepted: true, position: 2, readSpan: 4,
			want: Token{Symbol: 1, Text: "ab", EndByte: 2, EndPoint: Point{Column: 2},
				lexerInternalDFALexed: true, lexerLookaheadEndByte: 3},
		},
		{
			name: "invalid_unicode_frontier", source: "ab\xff", accepted: true, position: 2, readSpan: 7,
			want: Token{Symbol: 1, Text: "ab", EndByte: 2, EndPoint: Point{Column: 2},
				lexerInternalDFALexed: true, lexerLookaheadEndByte: 7},
		},
		{
			name: "accepted_after_skip", source: " ab!", skipPrefix: true, accepted: true, position: 3, readSpan: 4,
			want: Token{Symbol: 1, Text: "ab", StartByte: 1, EndByte: 3,
				StartPoint: Point{Column: 1}, EndPoint: Point{Column: 3},
				lexerSkippedPrefix: true, lexerInternalDFALexed: true, lexerLookaheadEndByte: 4},
		},
		{
			name: "failed_after_skip", source: " !", skipPrefix: true, failed: true, failure: 1, readSpan: 2,
			want: Token{lexerLookaheadEndByte: 2},
		},
	}
	for _, route := range []struct {
		name     string
		included bool
	}{{name: "contiguous"}, {name: "included", included: true}} {
		for _, test := range tests {
			t.Run(route.name+"/"+test.name, func(t *testing.T) {
				states := buildIdentNumberWSDFA()
				if test.skipPrefix {
					states[0].Transitions[2].NextState = 0
					states[0].Transitions[2].Skip = true
				}
				lex := NewLexer(states, []byte(test.source))
				if route.included {
					lex.setIncludedRanges([]Range{{
						EndByte: uint32(len(test.source)), EndPoint: Point{Column: uint32(len(test.source))},
					}})
				}
				lex.failTokenStartPos, lex.failTokenStartRow, lex.failTokenStartCol, lex.failTokenStartRangeIdx = 77, 79, 83, 89
				var span uint32
				lex.tokenInvariantReadSpanMax = &span
				output := poisonedLexerOutputToken()
				accepted := lex.scanInto(test.state, 0, 0, 0, &output)
				if accepted != test.accepted || output != test.want {
					t.Fatalf("accepted=%t token=%+v, want accepted=%t token=%+v", accepted, output, test.accepted, test.want)
				}
				if lex.pos != test.position || lex.row != 0 || lex.col != uint32(test.position) || lex.includedRangeIdx != 0 {
					t.Fatalf("cursor=(%d,%d,%d,%d), want (%d,0,%d,0)", lex.pos, lex.row, lex.col, lex.includedRangeIdx, test.position, test.position)
				}
				failurePos, failureRow, failureCol, failureRange := 77, uint32(79), uint32(83), 89
				if test.failed {
					failurePos, failureRow, failureCol, failureRange = test.failure, 0, uint32(test.failure), 0
				}
				if lex.failTokenStartPos != failurePos || lex.failTokenStartRow != failureRow || lex.failTokenStartCol != failureCol || lex.failTokenStartRangeIdx != failureRange {
					t.Fatalf("failure cursor=(%d,%d,%d,%d), want (%d,%d,%d,%d)", lex.failTokenStartPos, lex.failTokenStartRow, lex.failTokenStartCol, lex.failTokenStartRangeIdx, failurePos, failureRow, failureCol, failureRange)
				}
				if span != test.readSpan {
					t.Fatalf("read span=%d, want %d", span, test.readSpan)
				}
			})
		}
	}
}

func TestLexerScanIntoReusesOutputAcrossRangeGap(t *testing.T) {
	lex := NewLexer(buildIdentNumberWSDFA(), []byte("aXXb!"))
	lex.setIncludedRanges([]Range{
		{StartByte: 0, EndByte: 1, StartPoint: Point{Row: 4, Column: 2}, EndPoint: Point{Row: 4, Column: 3}},
		{StartByte: 3, EndByte: 5, StartPoint: Point{Row: 7, Column: 9}, EndPoint: Point{Row: 7, Column: 11}},
	})
	var span uint32
	lex.tokenInvariantReadSpanMax = &span
	output := poisonedLexerOutputToken()
	want := Token{
		Symbol: 1, Text: "ab", EndByte: 4,
		StartPoint: Point{Row: 4, Column: 2}, EndPoint: Point{Row: 7, Column: 10},
		lexerInternalDFALexed: true, lexerLookaheadEndByte: 5,
	}
	if !lex.scanInto(0, 0, 4, 2, &output) || output != want {
		t.Fatalf("cross-range token=%+v, want %+v", output, want)
	}
	previous := output
	if lex.pos != 4 || lex.row != 7 || lex.col != 10 || lex.includedRangeIdx != 1 {
		t.Fatalf("cross-range cursor=(%d,%d,%d,%d)", lex.pos, lex.row, lex.col, lex.includedRangeIdx)
	}
	if lex.scanInto(0, 4, 7, 10, &output) || output != (Token{lexerLookaheadEndByte: 5}) {
		t.Fatalf("failed scan retained token fields: %+v", output)
	}
	if lex.failTokenStartPos != 4 || lex.failTokenStartRow != 7 || lex.failTokenStartCol != 10 || lex.failTokenStartRangeIdx != 1 {
		t.Fatalf("failure cursor=(%d,%d,%d,%d)", lex.failTokenStartPos, lex.failTokenStartRow, lex.failTokenStartCol, lex.failTokenStartRangeIdx)
	}
	if lex.scanInto(^uint32(0), 4, 7, 10, &output) || output != (Token{}) {
		t.Fatalf("invalid-state scan retained token fields: %+v", output)
	}
	if lex.pos != 4 || lex.row != 7 || lex.col != 10 || lex.includedRangeIdx != 1 || span != 5 {
		t.Fatalf("failed scans changed cursor or lost coverage: cursor=(%d,%d,%d,%d), span=%d", lex.pos, lex.row, lex.col, lex.includedRangeIdx, span)
	}
	lex.pos, lex.row, lex.col, lex.includedRangeIdx = 0, 4, 2, 0
	if !lex.scanInto(0, 0, 4, 2, &output) || output != want || previous != want {
		t.Fatalf("reused output=%+v previous=%+v, want %+v", output, previous, want)
	}
}

func TestLexerCanLexAtPreservesFailedReadSpan(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.LexStates[1].AcceptToken = 0
	lex := NewLexer(lang.LexStates, []byte("=i"))
	lex.pos, lex.row, lex.col = 1, 7, 11
	var span uint32
	lex.tokenInvariantReadSpanMax = &span
	frontier := uint32(1)
	if lex.canLexAt(0, 0, 0, 0, &frontier) {
		t.Fatal("failed fixture accepted")
	}
	if lex.pos != 1 || lex.row != 7 || lex.col != 11 || span != 3 || frontier != 3 {
		t.Fatalf("probe cursor=(%d,%d,%d), span=%d frontier=%d", lex.pos, lex.row, lex.col, span, frontier)
	}
}
