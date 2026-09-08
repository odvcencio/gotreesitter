package gotreesitter

import "testing"

func TestLexerExplicitEOFTransitions(t *testing.T) {
	states := []LexState{
		{Default: -1, EOF: 1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 2}, {Lo: 0, Hi: 0, NextState: 3}}},
		{Default: -1, EOF: -1, AcceptToken: 1},
		{Default: -1, EOF: -1, AcceptToken: 2},
		{Default: -1, EOF: -1, AcceptToken: 3},
	}
	t.Run("physical", func(t *testing.T) {
		lex := NewLexer(states, nil)
		token := lex.Next(0)
		if token.Symbol != 1 || token.StartByte != 0 || token.EndByte != 0 {
			t.Fatalf("EOF token=%+v", token)
		}
		if end := lex.Next(2); end.Symbol != 0 {
			t.Fatalf("terminal token=%+v", end)
		}
	})
	t.Run("literal_nul", func(t *testing.T) {
		lex := NewLexer(states, []byte{0})
		token := lex.Next(0)
		if token.Symbol != 3 || token.StartByte != 0 || token.EndByte != 1 {
			t.Fatalf("NUL token=%+v", token)
		}
		if end := lex.Next(0); end.Symbol != 1 || end.StartByte != 1 || end.EndByte != 1 {
			t.Fatalf("EOF token=%+v", end)
		}
	})
	t.Run("included_eof_excludes_nul", func(t *testing.T) {
		lex := NewLexer(states, []byte{'a', 0})
		lex.setIncludedRanges([]Range{{StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}}})
		if token := lex.Next(0); token.Symbol != 2 || token.EndByte != 1 {
			t.Fatalf("prefix=%+v", token)
		}
		if end := lex.Next(0); end.Symbol != 1 || end.StartByte != 1 || end.EndByte != 1 || end.EndPoint != (Point{Column: 1}) {
			t.Fatalf("included EOF=%+v", end)
		}
	})
	t.Run("zero_width_mask", func(t *testing.T) {
		lex := NewLexer(states, nil)
		lex.zeroWidthTokens = []bool{false, false}
		if token := lex.Next(0); token.Symbol != 0 {
			t.Fatalf("masked token=%+v", token)
		}
	})
}

func TestLexerExplicitEOFCycleTerminates(t *testing.T) {
	lex := NewLexer([]LexState{{Default: -1, EOF: 0}}, nil)
	if token := lex.Next(0); token.Symbol != 0 || token.EndByte != 0 {
		t.Fatalf("cycle token=%+v", token)
	}
}

func TestDFATokenSourceExplicitIncludedEOFProgress(t *testing.T) {
	for _, actionable := range []bool{false, true} {
		name := "unusable"
		if actionable {
			name = "repeated_action"
		}
		t.Run(name, func(t *testing.T) {
			states := []LexState{
				{Default: -1, EOF: 1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 2}}},
				{Default: -1, EOF: -1, AcceptToken: 1},
				{Default: -1, EOF: -1, AcceptToken: 2},
			}
			lang := &Language{LexStates: states, LexModes: []LexMode{{LexState: 0}}, SymbolNames: []string{"end", "zero", "a"}}
			lex := NewLexer(states, []byte("a!"))
			lex.setIncludedRanges([]Range{{StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}}})
			if token := lex.Next(0); token.Symbol != 2 {
				t.Fatalf("prefix=%+v", token)
			}
			d := &dfaTokenSource{lexer: lex, language: lang, lookupActionIndex: func(_ StateID, sym Symbol) uint16 {
				if actionable && sym == 1 {
					return 1
				}
				return 0
			}}
			if actionable {
				for i := 0; i < maxConsecutiveZeroWidthTokens; i++ {
					if token := d.Next(); token.Symbol != 1 {
						t.Fatalf("token %d=%+v", i, token)
					}
				}
			}
			token := d.Next()
			if token.Symbol != 0 || token.StartByte != 1 || token.EndByte != 1 {
				t.Fatalf("bounded EOF=%+v", token)
			}
			if lex.pos != 1 {
				t.Fatalf("consumed excluded suffix: pos=%d", lex.pos)
			}
		})
	}
}
