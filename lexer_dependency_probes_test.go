package gotreesitter

import "testing"

func TestPreferredTokenRetainsDiscardedProbeFrontier(t *testing.T) {
	for _, raw := range []bool{false, true} {
		lang := &Language{
			SymbolNames: []string{"end", "a", "b"},
			LexModes:    []LexMode{{LexState: 0, AfterWhitespaceLexState: 2}},
			LexStates: []LexState{
				{Default: -1, EOF: -1, Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 0, Skip: true},
					{Lo: 'a', Hi: 'a', NextState: 1},
				}},
				{Default: -1, EOF: -1, AcceptToken: 1},
				{Default: -1, EOF: -1, Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 2, Skip: true},
					{Lo: 'a', Hi: 'a', NextState: 2, Skip: true},
					{Lo: 'b', Hi: 'b', NextState: 3},
				}},
				{Default: -1, EOF: -1, AcceptToken: 2, Transitions: []LexTransition{{Lo: 'b', Hi: 'b', NextState: 3}}},
			},
		}
		d := &dfaTokenSource{language: lang, lexer: NewLexer(lang.LexStates, []byte(" a bbbb!"))}
		var token Token
		if raw {
			token, _, _, _ = d.scanRawPreferredTokenForState(0)
		} else {
			token, _, _, _ = d.scanPreferredTokenForState(0)
		}
		if token.Symbol != 1 || token.StartByte != 1 || token.EndByte != 2 {
			t.Fatalf("raw=%t selected token changed: %+v", raw, token)
		}
		if token.lexerLookaheadEndByte != 8 {
			t.Fatalf("raw=%t frontier=%d, want discarded probe frontier8", raw, token.lexerLookaheadEndByte)
		}
	}
}
