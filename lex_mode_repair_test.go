package gotreesitter

import "testing"

func TestRepairNoLookaheadLexModesIgnoresExternalAndExtraTokens(t *testing.T) {
	lang := &Language{
		TokenCount:      5,
		StateCount:      3,
		ExternalSymbols: []Symbol{2},
		LexModes: []LexMode{
			{},
			{LexState: 7},
			{LexState: 9},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce}}},
			{Actions: []ParseAction{{Type: ParseActionShift}}},
			{Actions: []ParseAction{{Type: ParseActionShift, Extra: true}}},
			{Actions: []ParseAction{{Type: ParseActionShift}}},
		},
		ParseTable: [][]uint16{
			make([]uint16, 5),
			{1, 0, 2, 3, 0},
			{1, 0, 2, 0, 4},
		},
	}

	RepairNoLookaheadLexModes(lang)

	if got := lang.LexModes[1].LexState; got != noLookaheadLexState {
		t.Fatalf("LexModes[1].LexState = %d, want %d", got, noLookaheadLexState)
	}
	if got := lang.LexModes[2].LexState; got != 9 {
		t.Fatalf("LexModes[2].LexState = %d, want 9", got)
	}
}
