package ccore

import "testing"

func TestShortcutScannerOwnedAncestorPrefix(t *testing.T) {
	tables := Tables{TokenCount: 3, ExternalSymbols: []Symbol{1, 2}, LexStates: []LexDFAState{{AcceptEnd: true, Default: -1, EOF: -1}},
		LexModes:          []LexMode{{}, {ExternalLexState: 1}, {ExternalLexState: 2}, {ExternalLexState: 3}},
		ExternalLexStates: [][]bool{nil, {true, false}, {true, true}, {true, false}}}
	p := Parser{t: &tables}
	if !tables.externalOnlyEndLexer() {
		t.Fatal("scanner owns both terminals and whitespace")
	}
	if !p.shortcutExternalPrefixDiffers(1, 2) {
		t.Fatal("reopening under a smaller eligible set must reparse")
	}
	for _, states := range [][2]uint16{{1, 1}, {1, 3}, {0, 2}, {stateNone, 1}} {
		if p.shortcutExternalPrefixDiffers(states[0], states[1]) {
			t.Fatalf("unwarranted refusal for states %v", states)
		}
	}
	tables.LexStates[0].AcceptToken = 1
	if tables.externalOnlyEndLexer() {
		t.Fatal("mixed lexer must retain its existing shortcut behavior")
	}
}
