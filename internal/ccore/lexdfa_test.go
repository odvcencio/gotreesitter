package ccore

import "testing"

// Unit tests for the lex automaton walk on small hand-made tables. Each pins
// one rule of lexdfa.go that the goldens (JSON only) cannot reach.

func scanOne(states []LexDFAState, src string) (Token, bool, [2]Length) {
	var tok Token
	var fail [2]Length
	ok := scanDFA(states, BuildLexAscii(states), []byte(src), 0, 0, 0, 0, &tok, &fail)
	return tok, ok, fail
}

// C's SKIP does not reset the accepted token; ts_lexer_finish moves the token
// start to the token end (C/lexer.c:455-457), so the token is empty.
func TestScanSkipKeepsAccept(t *testing.T) {
	states := []LexDFAState{
		{Default: -1, EOF: -1, Trans: []LexTrans{{Lo: 'a', Hi: 'a', Next: 1}}},
		{AcceptToken: 5, Default: -1, EOF: -1, Trans: []LexTrans{{Lo: ' ', Hi: ' ', Next: 2, Skip: true}}},
		{Default: -1, EOF: -1, Trans: []LexTrans{{Lo: 'b', Hi: 'b', Next: 3}}},
		{AcceptToken: 6, Default: -1, EOF: -1},
	}
	tok, ok, _ := scanOne(states, "a x")
	if !ok || tok.Symbol != 5 || tok.StartByte != 1 || tok.EndByte != 1 || tok.StartCol != 1 || tok.EndCol != 1 {
		t.Errorf("accept before a skip: ok=%v token=%+v, want symbol 5 over bytes 1..1", ok, tok)
	}
	tok, ok, _ = scanOne(states, "a b")
	if !ok || tok.Symbol != 6 || tok.StartByte != 2 || tok.EndByte != 3 {
		t.Errorf("accept after a skip: ok=%v token=%+v, want symbol 6 over bytes 2..3", ok, tok)
	}
}

// C/lexer.c:438-441 ts_lexer_start: U+FEFF at byte 0 is skipped as padding.
func TestScanByteOrderMark(t *testing.T) {
	states := []LexDFAState{
		{Default: -1, EOF: -1, Trans: []LexTrans{{Lo: 'a', Hi: 'a', Next: 1}}},
		{AcceptToken: 5, Default: -1, EOF: -1},
	}
	tok, ok, _ := scanOne(states, "\xef\xbb\xbfa")
	if !ok || tok.Symbol != 5 || tok.StartByte != 3 || tok.EndByte != 4 || tok.StartCol != 3 || tok.EndCol != 4 {
		t.Errorf("after a byte order mark: ok=%v token=%+v, want symbol 5 over bytes 3..4, columns 3..4", ok, tok)
	}
	tok, ok, _ = scanOne(states, "a")
	if !ok || tok.StartByte != 0 || tok.EndByte != 1 {
		t.Errorf("without a mark: ok=%v token=%+v", ok, tok)
	}
	// Not at byte 0: no skip.
	var t2 Token
	var fail [2]Length
	if scanDFA(states, nil, []byte("x\xef\xbb\xbfa"), 0, 1, 0, 1, &t2, &fail) {
		t.Errorf("a mark inside the text was skipped: %+v", t2)
	}
}

func nulState(run int, skipFirst bool) []LexDFAState {
	var tr []LexTrans
	if skipFirst {
		tr = append(tr, LexTrans{Lo: ' ', Hi: ' ', Next: 0, Skip: true})
	}
	tr = append(tr, LexTrans{Lo: 0, Hi: 0, Next: 1})
	for c := rune('a'); len(tr) < run; c++ {
		tr = append(tr, LexTrans{Lo: c, Hi: c, Next: 0})
	}
	return []LexDFAState{
		{Default: -1, EOF: -1, Trans: tr},
		{AcceptToken: 9, Default: -1, EOF: -1},
	}
}

// ccNulNext: a step on character 0 at the end of input only for a [0,0] range
// in a leading run of 8 or more single-character, non-SKIP transitions.
func TestScanNulAtEnd(t *testing.T) {
	cases := []struct {
		name   string
		states []LexDFAState
		want   bool
	}{
		{"map of 8", nulState(8, false), true},
		{"map of 7", nulState(7, false), false},
		{"run broken by a skip", nulState(9, true), false},
		{"range [0,5]", []LexDFAState{{Default: -1, EOF: -1, Trans: []LexTrans{{Lo: 0, Hi: 5, Next: 1}}}, {AcceptToken: 9, Default: -1, EOF: -1}}, false},
	}
	for _, c := range cases {
		tok, ok, _ := scanOne(c.states, "")
		if ok != c.want || (ok && tok.Symbol != 9) {
			t.Errorf("%s: ok=%v token=%+v, want ok=%v", c.name, ok, tok, c.want)
		}
	}
	// An end transition comes first.
	st := nulState(8, false)
	st[0].EOF = 1
	if tok, ok, _ := scanOne(st, ""); !ok || tok.Symbol != 9 {
		t.Errorf("end transition: ok=%v token=%+v", ok, tok)
	}
}

// ccInvalidNext: the step on an ill-formed byte (lookahead -1).
func TestScanInvalidByte(t *testing.T) {
	accept := LexDFAState{AcceptToken: 4, Default: -1, EOF: -1}
	cases := []struct {
		name string
		tr   []LexTrans
		want bool
	}{
		{"starts at 1", []LexTrans{{Lo: 1, Hi: 0x10FFFF, Next: 1}}, true},
		// Known difference: a range from 0 is "(!eof && lookahead < x)" (false for
		// -1 in C only when written with a lower bound) or a bare "if (!eof)"
		// (true for -1); the blob cannot tell, and the scan finds no state with
		// the bare form, so the rule refuses it.
		{"starts at 0 (known difference)", []LexTrans{{Lo: 0, Hi: 0x10FFFF, Next: 1}}, false},
		// C takes the first condition in source order when two targets overlap
		// with equal coverage.
		{"overlap, first in source order", []LexTrans{{Lo: 1, Hi: 'a' - 1, Next: 1}, {Lo: 'a' + 1, Hi: 0x10FFFF, Next: 1}, {Lo: 1, Hi: 'b' - 1, Next: 2}, {Lo: 'b' + 1, Hi: 0x10FFFF, Next: 2}}, true},
		{"lone [0,0] ignored", []LexTrans{{Lo: 0, Hi: 0, Next: 2}, {Lo: 1, Hi: 0x10FFFF, Next: 1}}, true},
		{"holes of 300 code points", []LexTrans{{Lo: 1, Hi: 100, Next: 1}, {Lo: 401, Hi: 0x10FFFF, Next: 1}}, false},
		{"lower bound", []LexTrans{{Lo: 5, Hi: 0x10FFFF, Next: 1}}, false},
		{"two targets", []LexTrans{{Lo: 1, Hi: 9, Next: 2}, {Lo: 10, Hi: 0x10FFFF, Next: 1}}, false},
	}
	for _, c := range cases {
		states := []LexDFAState{{Default: -1, EOF: -1, Trans: c.tr}, accept, {AcceptToken: 8, Default: -1, EOF: -1}}
		tok, ok, _ := scanOne(states, "\xff")
		if ok != c.want {
			t.Errorf("%s: ok=%v token=%+v, want ok=%v", c.name, ok, tok, c.want)
		}
		if ok && (tok.Symbol != 4 || tok.EndByte != 1) {
			t.Errorf("%s: token %+v, want symbol 4 ending at byte 1", c.name, tok)
		}
	}
}

// A failed scan from a start state outside the table still reports where it began.
func TestScanBadStartState(t *testing.T) {
	var tok Token
	var fail [2]Length
	fail[0] = NewLength(99, 9, 9)
	if scanDFA(nil, nil, []byte("abc"), 3, 1, 0, 1, &tok, &fail) {
		t.Fatal("scan from a missing state succeeded")
	}
	if fail[0].bytes != 1 || fail[1].bytes != 1 {
		t.Errorf("fail = %v, want both at byte 1", fail)
	}
}
