package lexrun

import "testing"

func TestASCIIPreservesBoundaries(t *testing.T) {
	var row [128]int32
	for i := range row {
		row[i] = NoMatch
	}
	for b := byte('a'); b <= 'z'; b++ {
		row[b] = 7
	}
	row[0] = 7
	row['s'] = SkipBit | 7 // SKIP is never a plain self-loop.
	for _, q := range []struct {
		s           string
		start, want int
	}{
		{"abcdef!", 0, 6}, {"\x00abc", 0, 4}, {"abc\ndef", 0, 3}, {"abcé", 0, 3}, {"abc\xff", 0, 3},
		{"absdef", 0, 2}, {"abcdef", 3, 6}, {"", 0, 0}, {"abc", 3, 3}, {"abc", -1, -1}, {"abc", 4, 4},
	} {
		if got := ASCII([]byte(q.s), q.start, &row, 7); got != q.want {
			t.Fatalf("%q %d: %d !=%d", q.s, q.start, got, q.want)
		}
	}
	if ASCII([]byte("abc"), 0, nil, 7) != 0 || ASCII([]byte("abc"), 0, &row, -1) != 0 {
		t.Fatal("invalid context consumed")
	}
	var missing [128]int32
	for i := range missing {
		missing[i] = NoMatch
	}
	if got := ASCII([]byte("abc"), 0, &missing, NoMatch); got != 0 {
		t.Fatalf("reserved no-match state consumed %d bytes", got)
	}
	// The last representable non-sentinel value remains eligible; this is an
	// optimization fallback, not a narrower parser/lexer state-ID contract.
	high := NoMatch - 1
	missing['a'] = high
	if got := ASCII([]byte("aa!"), 0, &missing, high); got != 2 {
		t.Fatalf("full-width non-sentinel state consumed %d", got)
	}

}
