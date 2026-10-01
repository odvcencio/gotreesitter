package gotreesitter

import "testing"

type rejectingResultExternalScanner struct {
	checkpointByteExternalScanner
	calls *int
}

func (s rejectingResultExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	*s.calls++
	*payload.(*byte) = 9
	lexer.Advance(false)
	lexer.SetResultSymbol(1)
	if valid[0] {
		// A scanner may set a tentative result before rejecting the token.
		return false
	}
	if valid[1] {
		lexer.SetResultSymbol(2)
		return true
	}
	return false
}

func TestExternalErrorModeDoesNotMaskRejectedScannerResult(t *testing.T) {
	for _, tc := range []struct {
		name      string
		state     StateID
		cRecovery bool
		wantToken bool
		wantCalls int
		wantState byte
	}{
		{"C ERROR row rejects once", 0, true, false, 1, 0},
		{"ordinary row retains masked retry", 1, true, true, 2, 9},
		{"legacy ERROR row retains masked retry", 0, false, true, 2, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			lang := &Language{
				SymbolNames:     []string{"end", "tentative", "alternative"},
				LexModes:        []LexMode{{}, {}},
				ExternalSymbols: []Symbol{1, 2},
				ExternalScanner: rejectingResultExternalScanner{calls: &calls},
			}
			d := acquireDFATokenSourceWithCRecovery(NewLexer(nil, []byte("#")), lang, nil, nil, nil, nil, tc.cRecovery)
			defer d.Close()
			d.state = tc.state
			el := &d.externalLexer
			el.reset(d.lexer.source, 0, 0, 0)
			if got := d.runExternalScannerWithRetry(el, []bool{true, true}); got != tc.wantToken {
				t.Fatalf("accepted=%t, want %t", got, tc.wantToken)
			}
			if calls != tc.wantCalls || *d.externalPayload.(*byte) != tc.wantState {
				t.Fatalf("calls=%d scanner=%d, want %d/%d", calls, *d.externalPayload.(*byte), tc.wantCalls, tc.wantState)
			}
			if d.lexer.pos != 0 || d.externalLookaheadEndByte < 1 {
				t.Fatalf("internal cursor=%d read frontier=%d", d.lexer.pos, d.externalLookaheadEndByte)
			}
			if tc.wantToken && el.resultSymbol != 2 {
				t.Fatalf("accepted symbol=%d, want 2", el.resultSymbol)
			}
		})
	}
}

func TestNextTokenRejectedErrorScannerRestoresInternalFallback(t *testing.T) {
	calls := 0
	lang := &Language{
		SymbolNames: []string{"end", "tentative", "alternative", "internal"},
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '#', Hi: '#', NextState: 2}}},
			{Default: -1, EOF: -1},
			{Default: -1, EOF: -1, AcceptToken: 3},
		},
		LexModes:          []LexMode{{LexState: 0, ExternalLexState: 1}, {LexState: 1}},
		ExternalSymbols:   []Symbol{1, 2},
		ExternalLexStates: [][]bool{nil, {true, true}},
		ExternalScanner:   rejectingResultExternalScanner{calls: &calls},
	}
	d := acquireDFATokenSourceWithCRecovery(NewLexer(lang.LexStates, []byte("#")), lang,
		func(StateID, Symbol) uint16 { return 1 }, nil, nil, nil, true)
	defer d.Close()
	d.state = 1
	tok := d.Next()
	if tok.Symbol != 3 || tok.EndByte != 1 || calls != 1 {
		t.Fatalf("token=%+v scanner calls=%d, want internal fallback after one rejection", tok, calls)
	}
	if d.state != 1 || d.lexer.pos != 1 || *d.externalPayload.(*byte) != 0 {
		t.Fatalf("parser=%d cursor=%d scanner=%d, want restored 1/1/0", d.state, d.lexer.pos, *d.externalPayload.(*byte))
	}
	if tok.lexFlags&tokenFlagErrorModeRetried != 0 || tok.lexerLookaheadEndByte < 1 {
		t.Fatalf("fallback leaked retry marker or lost read frontier: %+v", tok)
	}
}
