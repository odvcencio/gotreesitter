package gotreesitter

import "testing"

func TestLexerAcceptEOFOnlyAtEnd(t *testing.T) {
	states := []LexState{{AcceptEOF: true, Default: -1, EOF: -1}}
	lex := NewLexer(states, []byte("x"))
	if token, ok := lex.scanContiguous(0, 0, 0, 0); ok {
		t.Fatalf("accepted end before input ended: %+v", token)
	}
	if token, ok := lex.scanContiguous(0, 1, 0, 1); !ok || token.Symbol != 0 || token.EndByte != 1 {
		t.Fatalf("missing end acceptance: token=%+v ok=%v", token, ok)
	}
	lex.setIncludedRanges([]Range{{StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}}})
	if token, ok := lex.scanIncluded(0, 0, 0, 0); ok {
		t.Fatalf("accepted included end before input ended: %+v", token)
	}
	if token, ok := lex.scanIncluded(0, 1, 0, 1); !ok || token.Symbol != 0 || token.EndByte != 1 {
		t.Fatalf("missing included end acceptance: token=%+v ok=%v", token, ok)
	}
}

func TestLexerEOFAcceptReplacesEqualWidthToken(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  LexState
		want Symbol
	}{
		{"end", LexState{AcceptEOF: true, Default: -1, EOF: -1}, 0},
		{"token", LexState{AcceptToken: 2, Default: -1, EOF: -1}, 2},
		{"nonaccepting", LexState{Default: -1, EOF: -1}, 1},
		{"lower_priority", LexState{AcceptEOF: true, AcceptPriority: 1, Default: -1, EOF: -1}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			states := []LexState{
				{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 1}}},
				{AcceptToken: 1, Default: -1, EOF: 2},
				tc.end,
			}
			for _, included := range []bool{false, true} {
				source := []byte("a")
				if included {
					source = []byte("a!")
				}
				lex := NewLexer(states, source)
				var token Token
				var ok bool
				if included {
					lex.setIncludedRanges([]Range{{StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}}})
					token, ok = lex.scanIncluded(0, 0, 0, 0)
				} else {
					token, ok = lex.scanContiguous(0, 0, 0, 0)
				}
				if !ok || token.Symbol != tc.want || token.StartByte != 0 || token.EndByte != 1 {
					t.Fatalf("included=%v: token=%+v ok=%v, want symbol %d", included, token, ok, tc.want)
				}
			}
		})
	}
}

func TestKeywordEOFAcceptReplacesEqualWidthToken(t *testing.T) {
	for _, end := range []LexState{
		{AcceptEOF: true, Default: -1, EOF: -1},
		{AcceptToken: 2, Default: -1, EOF: -1},
	} {
		lang := &Language{KeywordLexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 1}}},
			{AcceptToken: 1, Default: -1, EOF: 2},
			end,
		}}
		source := &dfaTokenSource{language: lang}
		token, ok := source.lexKeywordSource([]byte("a"))
		if ok != !end.AcceptEOF || token.Symbol != end.AcceptToken {
			t.Fatalf("end=%+v: token=%+v ok=%v", end, token, ok)
		}
	}
}
