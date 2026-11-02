package gotreesitter

import "testing"

// A reduction-repushed named extra has a stored goto distinct from its shift.
// The fallback must not bypass the scanner checkpoint at the token start.
func TestReviewExtraLeafFallbackRequiresScannerCheckpoint(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.ParseActions = append(lang.ParseActions, ParseActionEntry{Actions: []ParseAction{{Type: ParseActionShift, Extra: true, State: 4}}})
	lang.ParseTable[2][1] = uint16(len(lang.ParseActions) - 1)
	scanner := newC26lCheckpointScanner()
	lang.ExternalScanner = scanner
	arena := newNodeArena(arenaClassFull)
	defer arena.Release()
	if !arena.setExternalScannerCheckpointIdentityForLanguage(lang) {
		t.Fatal("identity")
	}
	leaf := newLeafNodeInArena(arena, 1, true, 0, 3, Point{}, Point{Column: 3})
	leaf.setExtra(true)
	leaf.preGotoState, leaf.parseState = 2, 9
	if !arena.recordExternalScannerLeafCheckpoint(leaf, []byte{1, 2, 3}, []byte{4, 5, 6}) {
		t.Fatal("checkpoint")
	}
	tok := Token{Symbol: 1, StartByte: 0, EndByte: 3}
	p := NewParser(lang)
	if _, ok := p.reuseTargetState(2, leaf, tok); ok {
		t.Fatal("old path accepted the repushed leaf")
	}
	if state, ok := p.reuseExtraLeafTargetState(2, leaf, tok); !ok || state != 4 {
		t.Fatalf("fallback = %d, %t", state, ok)
	}
	ts := &dfaTokenSource{language: lang, externalPayload: scanner.Create(), lastExternalTokenValid: true, lastExternalTokenStartByte: 0, externalTokenStart: []byte{1, 2, 3}}
	if _, ok := canReuseNodeWithExternalScannerCheckpointAtLookahead(ts, 2, leaf, tok.StartByte); !ok {
		t.Fatal("matching start checkpoint rejected")
	}
	ts.externalTokenStart = []byte{9, 9, 9}
	if _, ok := canReuseNodeWithExternalScannerCheckpointAtLookahead(ts, 2, leaf, tok.StartByte); ok {
		t.Fatal("mismatched start checkpoint admitted")
	}
}
