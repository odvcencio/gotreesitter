package gotreesitter

import "testing"

// The unrelated keyword-action state keeps hasKeywordState non-nil. Without
// it the old prefilter disables itself and hides the no-action-state defect.
func reservedRuntimePrefilterLanguage() *Language {
	lang := keywordAdoptionLanguage()
	lang.Name = "reserved_prefilter_witness"
	lang.LanguageVersion = 15
	lang.SymbolCount, lang.TokenCount = 4, 4
	lang.SymbolNames = []string{"end", "identifier", "if", "other_reserved"}
	lang.SymbolMetadata = []SymbolMetadata{{}, {Visible: true, Named: true}, {Visible: true}, {Visible: true}}
	lang.StateCount, lang.LargeStateCount = 3, 3
	lang.LexModes = []LexMode{{LexState: 0}, {LexState: 0, ReservedWordSetID: 1}, {LexState: 0}}
	lang.ReservedWords = []Symbol{0, 0, 2, 0}
	lang.MaxReservedWordSetSize = 2
	lang.ParseTable = [][]uint16{{0, 1, 1, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}
	lang.ParseActions = []ParseActionEntry{{}, {Actions: []ParseAction{{Type: ParseActionShift, State: 1}}}}
	return lang
}

func TestReservedRuntimePrefilterNextKeepsReservedNoActionKeyword(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   StateID
		glr     []StateID
		source  string
		symbol  Symbol
		keyword bool
	}{
		{name: "reserved_without_word_or_keyword_actions", state: 1, source: "if", symbol: 2, keyword: true},
		{name: "unexpected_unreserved_keyword_stays_word", state: 2, source: "if", symbol: 1},
		{name: "ordinary_unreserved_word", state: 1, source: "other", symbol: 1},
		{name: "keyword_prefix_is_not_a_keyword", state: 1, source: "ifx", symbol: 1},
		{name: "reserved_primary_with_unreserved_glr_member", state: 1, glr: []StateID{1, 2}, source: "if", symbol: 2, keyword: true},
		{name: "unreserved_primary_with_keyword_action_member", state: 2, glr: []StateID{2, 0}, source: "if", symbol: 2, keyword: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := reservedRuntimePrefilterLanguage()
			parser := NewParser(lang)
			if len(parser.hasKeywordState) != 3 || !parser.hasKeywordState[0] || !parser.hasKeywordState[1] || parser.hasKeywordState[2] {
				t.Fatalf("keyword-action and reserved-state prefilter=%v; want [true true false]", parser.hasKeywordState)
			}
			if parser.lookupActionIndex(1, 1) != 0 || parser.lookupActionIndex(1, 2) != 0 {
				t.Fatal("reserved witness unexpectedly has a word or keyword action")
			}
			source := []byte(tc.source)
			tokens := acquireDFATokenSource(NewLexer(lang.LexStates, source), lang, parser.lookupActionIndex, parser.hasKeywordState, nil, nil)
			defer tokens.Close()
			tokens.SetParserState(tc.state)
			tokens.SetGLRStates(tc.glr)
			got := tokens.Next()
			if got.Symbol != tc.symbol || got.isKeyword() != tc.keyword || got.StartByte != 0 || got.EndByte != uint32(len(source)) {
				t.Fatalf("Next(%q,state=%d,glr=%v)=(symbol=%d keyword=%v span=%d..%d), want symbol=%d keyword=%v complete span", tc.source, tc.state, tc.glr, got.Symbol, got.isKeyword(), got.StartByte, got.EndByte, tc.symbol, tc.keyword)
			}
		})
	}
}

func TestReservedRuntimePrefilterRejectsMalformedOrUnlexableSets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Language)
	}{
		{name: "zero_stride", mutate: func(l *Language) { l.MaxReservedWordSetSize = 0 }},
		{name: "absent_pool", mutate: func(l *Language) { l.ReservedWords = nil }},
		{name: "zero_set_id", mutate: func(l *Language) { l.LexModes[1].ReservedWordSetID = 0 }},
		{name: "out_of_range_set_id", mutate: func(l *Language) { l.LexModes[1].ReservedWordSetID = 65535 }},
		{name: "truncated_row", mutate: func(l *Language) { l.ReservedWords = l.ReservedWords[:3] }},
		{name: "empty_set", mutate: func(l *Language) { l.ReservedWords[2] = 0 }},
		{name: "word_capture_is_not_keyword", mutate: func(l *Language) { l.ReservedWords[2] = 1 }},
		{name: "symbol_absent_from_keyword_dfa", mutate: func(l *Language) { l.ReservedWords[2] = 3 }},
		{name: "out_of_range_symbol", mutate: func(l *Language) { l.ReservedWords[2] = 65535 }},
		{name: "zero_terminates_before_keyword", mutate: func(l *Language) { l.ReservedWords[2], l.ReservedWords[3] = 0, 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := reservedRuntimePrefilterLanguage()
			tc.mutate(lang)
			states := buildKeywordStates(lang)
			if len(states) != 3 || !states[0] || states[1] || states[2] {
				t.Fatalf("malformed/unlexable reserved set invented keyword support: %v", states)
			}
		})
	}
}
