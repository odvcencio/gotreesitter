package gotreesitter

import "testing"

func TestContextualCloseAngleProbeReadHistory(t *testing.T) {
	for _, mode := range []string{"accepted", "failed", "discarded"} {
		t.Run(mode, func(t *testing.T) {
			language := &Language{
				Name: "probe", SymbolCount: 3, TokenCount: 3, StateCount: 2, LargeStateCount: 2,
				SymbolNames: []string{"end", ">", ">>"},
				LexModes:    []LexMode{{LexState: 0}, {LexState: 0}},
				LexStates: []LexState{
					{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 1}}},
					{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 2}}},
					{AcceptToken: 2, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'b', Hi: 'b', NextState: 3}}},
					{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'b', Hi: 'b', NextState: 3}}},
				},
				ParseTable:   [][]uint16{{0, 0, 0}, {0, 1, 2}},
				ParseActions: []ParseActionEntry{{}, {Actions: []ParseAction{{Type: ParseActionShift, State: 1}}}, {Actions: []ParseAction{{Type: ParseActionShift, State: 1}}}},
			}
			if mode == "failed" {
				language.LexStates[2].AcceptToken = 0
			}
			if mode == "discarded" {
				language.ParseTable[1][2] = 0
			}
			source := []byte("x>>bbbbμ")
			token := Token{Symbol: 1, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}}
			var span uint32
			probe := &Lexer{}
			deferred := deferContextualCloseAngleAction(language, source, 1, token, nil, probe, &span)
			if deferred != (mode == "accepted") {
				t.Fatalf("deferred=%v for %s probe", deferred, mode)
			}
			if span != uint32(len(source)-1) {
				t.Fatalf("read span=%d, want %d including the failed UTF-8 branch", span, len(source)-1)
			}
			if probe.tokenInvariantReadSpanMax != nil {
				t.Fatal("probe retained the attempt observer")
			}
			parser := NewParser(language)
			var outerSpan uint32
			parser.mergeScratch = &glrMergeScratch{lexicalReadSpan: &outerSpan}
			parser.shouldDeferContextualCloseAngleAction(source, 1, token)
			if outerSpan != span || parser.relexProbeLexer.tokenInvariantReadSpanMax != nil {
				t.Fatal("legacy wrapper did not capture and detach its attempt observer")
			}
			parser.mergeScratch.reset()
			if parser.mergeScratch.lexicalReadSpan != nil {
				t.Fatal("scratch reset retained the observer")
			}
			parser.mergeScratch = nil
			parser.shouldDeferContextualCloseAngleAction(source, 1, token)
			if outerSpan != span || parser.relexProbeLexer.tokenInvariantReadSpanMax != nil {
				t.Fatal("direct call touched an expired observer")
			}
		})
	}
}

func TestContextualCloseAngleHistoryRestoresOuterScratch(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	parser.SetAdmissionCandidateRoute(false)
	var outerSpan uint32 = 77
	outer := &glrMergeScratch{lexicalReadSpan: &outerSpan}
	parser.mergeScratch = outer
	for _, source := range []string{"1", "+", "12345"} {
		tree, err := parser.Parse([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		if tree != nil {
			tree.Release()
		}
		if parser.mergeScratch != outer || outer.lexicalReadSpan != &outerSpan || outerSpan != 77 {
			t.Fatal("nested attempt replaced the outer observer")
		}
		if parser.relexProbeLexer.tokenInvariantReadSpanMax != nil {
			t.Fatal("parse retained a probe observer")
		}
	}
}
