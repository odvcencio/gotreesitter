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

// ccInvalidNext with many targets. A target qualifies if its ranges start at
// code point 1 and cover all but 256 code points; among those the one whose
// first range comes first wins. Half-size targets around a qualifying one must
// not hide it, and the two-pass rule must agree with the one-pass-per-target
// rule on every state.
func TestInvalidNextManyTargets(t *testing.T) {
	const full = 0x110000 - 256
	half := func(next int32) LexTrans { return LexTrans{Lo: 1, Hi: full / 2, Next: next} }
	// One qualifying target first, then sixteen targets of half the size.
	trans := []LexTrans{{Lo: 1, Hi: full, Next: 1}}
	for next := int32(2); next <= 17; next++ {
		trans = append(trans, half(next))
	}
	st := LexDFAState{Default: -1, EOF: -1, Trans: trans}
	if next, _ := ccInvalidNext(&st); next != 1 {
		t.Fatalf("sixteen half-size targets hid the qualifying one: got %d, want 1", next)
	}
	// A second qualifying target at the end must not win over the first.
	st.Trans = append(st.Trans, LexTrans{Lo: 1, Hi: full, Next: 18})
	if next, _ := ccInvalidNext(&st); next != 1 {
		t.Fatalf("a later qualifying target won: got %d, want 1", next)
	}
	// Two targets qualify. C takes the first condition that holds for -1, and
	// such a condition starts at code point 1: target 2's does, at index 1;
	// target 3's first range starts at 9 and does not hold for -1.
	st.Trans = []LexTrans{{Lo: 9, Hi: full, Next: 3, Skip: true}, {Lo: 1, Hi: full, Next: 2}, {Lo: 1, Hi: 8, Next: 3}}
	if next, skip := ccInvalidNext(&st); next != 2 || skip {
		t.Fatalf("got %d skip=%v, want 2 without skip", next, skip)
	}
	// The SKIP flag is that of the range that starts at 1.
	st.Trans = []LexTrans{{Lo: 9, Hi: full, Next: 3}, {Lo: 1, Hi: 8, Next: 3, Skip: true}}
	if next, skip := ccInvalidNext(&st); next != 3 || !skip {
		t.Fatalf("got %d skip=%v, want 3 with the skip flag of its range from 1", next, skip)
	}
	// A range from 0 that is not a lone [0,0] disqualifies its target, also
	// when the blob has merged [0,0] with [1,8] (http, a known difference).
	st.Trans = []LexTrans{{Lo: 0, Hi: 8, Next: 4}, {Lo: 9, Hi: full + 8, Next: 4}}
	if next, _ := ccInvalidNext(&st); next != -1 {
		t.Fatalf("a target with a range from 0 was taken: %d", next)
	}
	st.Trans = []LexTrans{{Lo: 1, Hi: full, Next: 4}, {Lo: 0, Hi: 5, Next: 4}}
	if next, _ := ccInvalidNext(&st); next != -1 {
		t.Fatalf("a target with ranges from 1 and from 0 was taken: %d", next)
	}
	if next, _ := ccInvalidNextAll(&st); next != -1 {
		t.Fatalf("per-target rule: a target with ranges from 1 and from 0 was taken: %d", next)
	}
	// Nine targets with a range starting at 1: the rule goes on to
	// ccInvalidNextAll and still finds the only qualifying one, the last.
	st.Trans = nil
	for next := int32(1); next <= 8; next++ {
		st.Trans = append(st.Trans, LexTrans{Lo: 1, Hi: 100, Next: next})
	}
	st.Trans = append(st.Trans, LexTrans{Lo: 1, Hi: full, Next: 9})
	if next, _ := ccInvalidNext(&st); next != 9 {
		t.Fatalf("more targets than slots: got %d, want 9", next)
	}
}

// The coverage a target needs is every code point but 256, in both forms of
// the rule. The number is written out here, so that a changed constant fails.
func TestInvalidNextThreshold(t *testing.T) {
	const need = 0x110000 - 256
	for _, c := range []struct {
		hi   rune
		want int32
	}{{need, 1}, {need - 1, -1}} {
		st := LexDFAState{Default: -1, EOF: -1, Trans: []LexTrans{{Lo: 1, Hi: c.hi, Next: 1}}}
		if next, _ := ccInvalidNext(&st); next != c.want {
			t.Errorf("two-pass rule, range [1,%d]: got %d, want %d", c.hi, next, c.want)
		}
		if next, _ := ccInvalidNextAll(&st); next != c.want {
			t.Errorf("per-target rule, range [1,%d]: got %d, want %d", c.hi, next, c.want)
		}
	}
}

// ccNulNext rebuilds the generator's ADVANCE_MAP from the blob's ranges. The
// first two lists are pinned lex states as the blob holds them.
func TestNulNextFollowsTheGeneratorsMap(t *testing.T) {
	one := func(c rune, next int32) LexTrans { return LexTrans{Lo: c, Hi: c, Next: next} }
	cases := []struct {
		name  string
		trans []LexTrans
		want  int32
	}{
		// http, lex state 1: "(!eof && lookahead == 0)", four single characters,
		// then a transition to 896 whose first three ranges are single
		// characters and whose fourth is wide. C takes no step at the end of
		// input; a rule that counted the three accepted an empty token for ever.
		{"http state 1: guarded test, no map", []LexTrans{
			one(0, 863), one(10, 897), one(13, 899), one(92, 891), one(123, 888),
			one(32, 896), one(160, 896), one(5760, 896), {Lo: 8192, Hi: 8202, Next: 896},
			one(8239, 896), {Lo: 48, Hi: 57, Next: 807}}, -1},
		// bitbake, lex state 239: a map of 8 ranges, one of them two code
		// points wide after the blob merged 0x0b and 0x0c. C takes the step.
		{"bitbake state 239: map with a merged pair", []LexTrans{
			one(0, 203), one(10, 197), one(13, 7), one(35, 243), one(36, 242), one(92, 240),
			one(9, 239), {Lo: 11, Hi: 12, Next: 239}, {Lo: 1, Hi: 31, Next: 244}, {Lo: 33, Hi: 0x10FFFF, Next: 244}}, 203},
		{"the same map before the merge", []LexTrans{
			one(0, 203), one(10, 197), one(13, 7), one(35, 243), one(36, 242), one(92, 240),
			one(9, 239), one(11, 239), one(12, 239), {Lo: 1, Hi: 31, Next: 244}}, 203},
		{"seven ranges are not a map", []LexTrans{
			one(0, 1), one(10, 2), one(13, 3), one(35, 4), one(36, 5), one(92, 6), one(9, 7), {Lo: 1, Hi: 31, Next: 8}}, -1},
		{"a range above 0xFFFF ends the run", []LexTrans{
			one(0, 1), one(10, 2), one(13, 3), one(35, 4), one(36, 5), one(92, 6), one(9, 7), one(0x10000, 8), one(40, 9)}, -1},
		{"a range three wide ends the run", []LexTrans{
			one(0, 1), one(10, 2), one(13, 3), one(35, 4), one(36, 5), one(92, 6), one(9, 7), {Lo: 60, Hi: 62, Next: 8}, one(40, 9)}, -1},
		{"a SKIP that ends the run takes nothing out of it", []LexTrans{
			one(0, 1), one(10, 2), one(13, 3), one(35, 4), one(36, 5), one(92, 6), one(9, 7), one(11, 7),
			{Lo: 32, Hi: 32, Next: 7, Skip: true}}, 1},
		{"the range from 0 may be two wide", []LexTrans{
			{Lo: 0, Hi: 1, Next: 9}, one(10, 2), one(13, 3), one(35, 4), one(36, 5), one(92, 6), one(9, 7), one(11, 8)}, 9},
		{"a map without character 0", []LexTrans{
			one(5, 1), one(10, 2), one(13, 3), one(35, 4), one(36, 5), one(92, 6), one(9, 7), one(11, 8)}, -1},
		// angular, lex state 14, with character 0 in place of '"' in its map:
		// the map has 13 entries and ends with six to 323, and a second
		// transition to 323 follows whose first range lies below them. Two
		// transitions: the map stays and C takes the step. A rule that rolled
		// back every range with the target left 7 and took none.
		{"angular state 14: a second transition with the same target", []LexTrans{
			one(0, 1), one('(', 234), one('*', 273), one('/', 325), one('>', 164), one('[', 279), one('\\', 324),
			one('\'', 323), one(')', 323), one('.', 323), one('<', 323), one('=', 323), one(']', 323),
			{Lo: '\t', Hi: '\r', Next: 323}, one(' ', 323), {Lo: 1, Hi: 0x10FFFF, Next: 327}}, 1},
		// The shape the blob cannot tell apart: read as one transition
		// {14, 16-18} -> 8, so the map has seven entries and no step is taken.
		// If C wrote 14 -> 8 in the map and the range in an "if", C takes the
		// step (a known difference; the exact repair is at import).
		{"ambiguous: a later range above the run with its target", []LexTrans{
			one(0, 1), one(2, 2), one(4, 3), one(6, 4), one(8, 5), one(10, 6), one(12, 7), one(14, 8),
			{Lo: 16, Hi: 18, Next: 8}}, -1},
		{"no transitions", nil, -1},
	}
	for _, c := range cases {
		st := LexDFAState{Default: -1, EOF: -1, Trans: c.trans}
		if got := ccNulNext(&st); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// The two forms of the rule agree on generated states: few and many targets,
// ranges that start at 0, at 1 and above, lone [0,0] ranges, and targets split
// over several ranges.
func TestInvalidNextFormsAgree(t *testing.T) {
	const full = 0x110000 - 256
	seed := uint64(0x9e3779b97f4a7c15)
	rnd := func(n int) int {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return int(seed % uint64(n))
	}
	qualifying, none := 0, 0
	for round := 0; round < 20000; round++ {
		var st LexDFAState
		targets := 1 + rnd(12)
		for i, n := 0, 1+rnd(24); i < n; i++ {
			tr := LexTrans{Next: int32(1 + rnd(targets)), Skip: rnd(4) == 0}
			switch rnd(6) {
			case 0:
				tr.Lo, tr.Hi = 0, 0
			case 1:
				tr.Lo, tr.Hi = 1, rune(full+rnd(300))
			case 2:
				tr.Lo, tr.Hi = 1, rune(1+rnd(full))
			case 3:
				tr.Lo = rune(2 + rnd(full/2))
				tr.Hi = tr.Lo + rune(rnd(full/2+300))
			case 4:
				tr.Lo, tr.Hi = 0, rune(rnd(full))
			default:
				tr.Lo = rune(1 + rnd(200))
				tr.Hi = tr.Lo + rune(rnd(64))
			}
			if tr.Hi > 0x10FFFF {
				tr.Hi = 0x10FFFF
			}
			st.Trans = append(st.Trans, tr)
		}
		next, skip := ccInvalidNext(&st)
		allNext, allSkip := ccInvalidNextAll(&st)
		if next != allNext || skip != allSkip {
			t.Fatalf("round %d: two-pass rule gives %d skip=%v, per-target rule gives %d skip=%v for %+v", round, next, skip, allNext, allSkip, st.Trans)
		}
		if next >= 0 {
			qualifying++
		} else {
			none++
		}
	}
	if qualifying < 1000 || none < 1000 {
		t.Fatalf("the generated states are one-sided: %d with a step, %d without", qualifying, none)
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
