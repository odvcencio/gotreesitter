package gotreesitter

import "testing"

func TestPromoteKeywordMatchesCStateAdmission(t *testing.T) {
	for _, tc := range []struct {
		name                                             string
		keywordAction, identifierAction, reserved, grant bool
		want                                             Symbol
	}{
		{"keyword_action", true, false, false, false, 2},
		{"identifier_action", false, true, false, false, 1},
		{"neither_action", false, false, false, false, 1},
		{"reserved_grant", false, false, true, true, 2},
		{"reserved_no_grant", false, true, true, false, 1},
		{"reserved_keyword_action", true, true, true, false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := keywordAdoptionLanguage()
			if tc.reserved {
				lang.LexModes[0].ReservedWordSetID = 1
				lang.MaxReservedWordSetSize = 2
				lang.ReservedWords = []Symbol{0, 0, 1, 0}
				if tc.grant {
					lang.ReservedWords[2] = 2
				}
			}
			d := &dfaTokenSource{language: lang, lexer: NewLexer(lang.LexStates, []byte("if")), hasKeywordState: []bool{!tc.grant}, lookupActionIndex: func(_ StateID, sym Symbol) uint16 {
				if sym == 2 && tc.keywordAction || sym == 1 && tc.identifierAction {
					return 1
				}
				return 0
			}}
			got, _ := d.promoteKeyword(Token{Symbol: 1, EndByte: 2})
			if got.Symbol != tc.want {
				t.Fatalf("symbol=%d want=%d", got.Symbol, tc.want)
			}
		})
	}
}
