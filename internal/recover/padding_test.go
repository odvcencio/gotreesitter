package recover

import "testing"

func TestPaddingBeforeLookahead(t *testing.T) {
	for _, tc := range []struct {
		position, start, end, lookahead uint32
		want                            bool
	}{
		{20, 21, 21, 21, true},
		{20, 20, 20, 21, false}, // no byte progress
		{20, 21, 23, 21, false}, // consumes the real token
		{20, 21, 21, 22, false}, // does not reach the lookahead
		{21, 21, 21, 21, false}, // already consumed
	} {
		if got := PaddingBeforeLookahead(tc.position, tc.start, tc.end, tc.lookahead); got != tc.want {
			t.Fatalf("PaddingBeforeLookahead(%+v) = %t", tc, got)
		}
	}
}

func TestPreferConflictShift(t *testing.T) {
	if !PreferConflictShift(true, true, 1, 1, false, true) {
		t.Fatal("equally ranked paused siblings must retain C's original shift first")
	}
	for _, tc := range []struct {
		aPaused, bPaused   bool
		aGroup, bGroup     uint16
		aReduced, bReduced bool
	}{
		{false, true, 1, 1, false, true},
		{true, false, 1, 1, false, true},
		{true, true, 0, 0, false, true},
		{true, true, 1, 2, false, true},
		{true, true, 1, 1, true, false},
		{true, true, 1, 1, false, false},
	} {
		if PreferConflictShift(tc.aPaused, tc.bPaused, tc.aGroup, tc.bGroup, tc.aReduced, tc.bReduced) {
			t.Fatalf("changed order without a paused shift/reduce sibling proof: %+v", tc)
		}
	}
	if HasOriginalConflictShift(true, true, false) || HasOriginalConflictShift(false, false, false) || HasOriginalConflictShift(true, false, true) || !HasOriginalConflictShift(true, false, false) {
		t.Fatal("repetition/extra shifts and reduction-only conflicts have no original shift")
	}
}

func TestHasErrorModeExternalToken(t *testing.T) {
	for _, tc := range []struct {
		symbols []bool
		want    bool
	}{
		{nil, false}, {[]bool{false, false}, false}, {[]bool{true, false}, true}, {[]bool{false, true}, true},
	} {
		if got := HasErrorModeExternalToken(tc.symbols); got != tc.want {
			t.Fatalf("error-mode external proof for %v = %t, want %t", tc.symbols, got, tc.want)
		}
	}
}
