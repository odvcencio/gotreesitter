package gotreesitter

import "testing"

func TestStackRelexPreservesKeywordAdmission(t *testing.T) {
	for _, reserved := range []bool{false, true} {
		name := "contextual"
		if reserved {
			name = "reserved"
		}
		t.Run(name, func(t *testing.T) {
			lang := keywordAdoptionLanguage()
			lang.TokenCount = 3
			lang.StateCount = 1
			lang.LargeStateCount = 1
			lang.ParseTable = [][]uint16{{0, 1, 0}}
			lang.ParseActions = []ParseActionEntry{{}, {Actions: []ParseAction{{Type: ParseActionShift}}}}
			if reserved {
				lang.LexModes[0].ReservedWordSetID = 1
				lang.MaxReservedWordSetSize = 2
				lang.ReservedWords = []Symbol{0, 0, 2, 0}
			}
			parser := NewParser(lang)
			original := Token{Symbol: 2, EndByte: 2, EndPoint: Point{Column: 2}}
			got, replaced := parser.relexTokenForStackLexState([]byte("if"), 0, original, nil)
			if reserved {
				if replaced || got.Symbol != original.Symbol {
					t.Fatal("stack re-lex demoted a reserved keyword")
				}
			} else if !replaced || got.Symbol != 1 || !got.isKeyword() {
				t.Fatalf("contextual re-lex lost its capture symbol or keyword metadata: %+v", got)
			}
		})
	}
}

func TestPromoteKeywordMatchesCStateAdmission(t *testing.T) {
	for _, tc := range []struct {
		name                                             string
		keywordAction, identifierAction, reserved, grant bool
		want                                             Symbol
	}{
		{"keyword_action", true, false, false, false, 2},
		{"identifier_action", false, true, false, false, 1},
		{"capture_without_cached_action", false, true, false, false, 1},
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
			d := &dfaTokenSource{language: lang, lexer: NewLexer(lang.LexStates, []byte("if")), hasKeywordState: []bool{!tc.grant && tc.name != "capture_without_cached_action"}, lookupActionIndex: func(_ StateID, sym Symbol) uint16 {
				if sym == 2 && tc.keywordAction || sym == 1 && tc.identifierAction {
					return 1
				}
				return 0
			}}
			got := Token{Symbol: 1, EndByte: 2}
			d.promoteKeyword(&got)
			if got.Symbol != tc.want {
				t.Fatalf("symbol=%d want=%d", got.Symbol, tc.want)
			}
			if !got.isKeyword() {
				t.Fatal("successful keyword recognition lost its metadata")
			}
		})
	}
}
