//go:build !gts_no_parsercorephase0

package gotreesitter

import "testing"

func TestCompactIncludedRangeProbes(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", ">", ">>", "shared"},
		LexModes:    []LexMode{{LexState: 0}},
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 1}}},
			{Default: -1, EOF: -1, AcceptToken: 1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 2}}},
			{Default: -1, EOF: -1, AcceptToken: 2},
		},
	}
	source := []byte(">>x")
	ts := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, nil, nil, nil, nil)
	defer ts.Close()
	ranges := []Range{{EndByte: 1, EndPoint: Point{Column: 1}}}
	scheduler := &diagnosticParserCoreGenericScheduler{tokenSource: ts, options: DiagnosticParserCorePrefixOptions{includedRanges: ranges, DisablePerHeaderSpanUnlockedRelex: true}}
	shared := Token{Symbol: 3, EndByte: 1, EndPoint: Point{Column: 1}}
	relexed, ok := scheduler.relexTokenForState(0, shared)
	if !ok || relexed.Symbol != 1 || relexed.EndByte != 1 || relexed.EndPoint != shared.EndPoint {
		t.Fatalf("state probe crossed excluded suffix: %+v %v", relexed, ok)
	}
	recovery, ok := scheduler.s3ErrorModeRelex(0)
	if !ok || recovery.Symbol != 1 || recovery.EndByte != 1 {
		t.Fatalf("recovery probe crossed excluded suffix: %+v %v", recovery, ok)
	}
	if ts.lexer.pos != 0 || len(ts.lexer.includedRanges) != 0 {
		t.Fatal("probe changed the shared lexer")
	}
	scheduler.options.includedRanges = nil
	scheduler.options.DisablePerHeaderSpanUnlockedRelex = false
	relexed, ok = scheduler.relexTokenForState(0, shared)
	if !ok || relexed.Symbol != 2 || relexed.EndByte != 2 {
		t.Fatalf("reset retained excluded suffix: %+v %v", relexed, ok)
	}
}
