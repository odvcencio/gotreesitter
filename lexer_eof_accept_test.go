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

// nulStateWithSingles returns a lex state with a NUL transition to state 1 and
// extra single-character transitions to state 2, so the total number of
// single-character entries is count.
func nulStateWithSingles(count int) LexState {
	transitions := []LexTransition{{Lo: 0, Hi: 0, NextState: 1}}
	for i := 1; i < count; i++ {
		c := rune('a' + i)
		transitions = append(transitions, LexTransition{Lo: c, Hi: c, NextState: 2})
	}
	return LexState{Default: -1, EOF: -1, Transitions: transitions}
}

func TestLexStateNulAtEOFFollowsMapThreshold(t *testing.T) {
	// The C generator renders single-character targets as an unguarded
	// ADVANCE_MAP from eight entries up. Below that, a lone NUL condition has
	// an end-of-input guard and is not taken at EOF.
	for count, want := range map[int]bool{1: false, 7: false, 8: true, 12: true} {
		st := nulStateWithSingles(count)
		next, ok := lexStateNulAtEOF(&st)
		if ok != want || (ok && next != 1) {
			t.Fatalf("%d single-character entries: next=%d ok=%v, want ok=%v", count, next, ok, want)
		}
	}

	// A NUL target that also covers a range cannot be a map entry.
	st := nulStateWithSingles(9)
	st.Transitions = append(st.Transitions, LexTransition{Lo: 'x', Hi: 'z', NextState: 1})
	if next, ok := lexStateNulAtEOF(&st); ok {
		t.Fatalf("NUL target shared with a range was taken: next=%d", next)
	}

	// No NUL transition, no move.
	st = LexState{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 1}}}
	if _, ok := lexStateNulAtEOF(&st); ok {
		t.Fatal("state without a NUL transition moved at EOF")
	}
}

func TestLexerNextTakesNulTransitionAtEOF(t *testing.T) {
	for _, tc := range []struct {
		name    string
		singles int
		want    Symbol
	}{
		{"map_entry", 8, 5},
		{"guarded_condition", 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			states := []LexState{
				nulStateWithSingles(tc.singles),
				{AcceptToken: 5, Default: -1, EOF: -1},
				{Default: -1, EOF: -1},
			}
			lex := NewLexer(states, []byte("a"))
			lex.pos, lex.col = 1, 1
			tok := lex.Next(0)
			if tok.Symbol != tc.want || tok.StartByte != 1 || tok.EndByte != 1 {
				t.Fatalf("token=%+v, want zero-width symbol %d at byte 1", tok, tc.want)
			}
		})
	}
}

func TestLexerNextAcceptsZeroWidthTokenBeforeEnd(t *testing.T) {
	// C runs the lex function at end of input, so a state that reaches a
	// zero-width token through its end-of-input edge wins over the end token.
	states := []LexState{
		{Default: -1, EOF: 1},
		{AcceptToken: 7, Default: -1, EOF: -1},
	}
	lex := NewLexer(states, []byte("a"))
	lex.pos, lex.col = 1, 1
	tok := lex.Next(0)
	if tok.Symbol != 7 || tok.StartByte != 1 || tok.EndByte != 1 {
		t.Fatalf("token=%+v, want zero-width symbol 7 at byte 1", tok)
	}

	// A state that only accepts the end token still yields the end token.
	states = []LexState{{AcceptEOF: true, Default: -1, EOF: -1}}
	lex = NewLexer(states, []byte("a"))
	lex.pos, lex.col = 1, 1
	if tok := lex.Next(0); tok.Symbol != 0 || tok.StartByte != 1 || tok.EndByte != 1 {
		t.Fatalf("token=%+v, want the end token at byte 1", tok)
	}
}

func TestLexerNextEndEdgeReplacesZeroWidthStartAccept(t *testing.T) {
	// A start state that accepts a token and also has an end-of-input edge: the
	// generated lex function accepts again at the edge target, so the end token
	// wins and the zero-width token is not produced.
	states := []LexState{
		{AcceptToken: 5, Default: -1, EOF: 1},
		{Default: -1, EOF: -1},
	}
	lex := NewLexer(states, []byte("a"))
	lex.pos, lex.col = 1, 1
	if tok := lex.Next(0); tok.Symbol != 0 || tok.StartByte != 1 || tok.EndByte != 1 {
		t.Fatalf("token=%+v, want the end token at byte 1", tok)
	}

	// Without the edge the start accept stands.
	states = []LexState{{AcceptToken: 5, Default: -1, EOF: -1}}
	lex = NewLexer(states, []byte("a"))
	lex.pos, lex.col = 1, 1
	if tok := lex.Next(0); tok.Symbol != 5 || tok.StartByte != 1 || tok.EndByte != 1 {
		t.Fatalf("token=%+v, want zero-width symbol 5 at byte 1", tok)
	}
}
