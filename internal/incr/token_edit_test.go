package incr

import "testing"

type runClasses struct{}

func (runClasses) ExternalScannerLengthNeutralASCIIClass(b byte) uint8 {
	if b >= 'a' && b <= 'z' {
		return 1
	}
	if b >= '0' && b <= '9' {
		return 2
	}
	return 0
}

func TestLengthNeutralScannerEdit(t *testing.T) {
	for _, tc := range []struct {
		name, old, next       string
		start, oldEnd, newEnd uint32
		want                  bool
	}{
		{"insert", "abc!", "abxxx c!", 2, 2, 6, false},
		{"insert_run", "abc!", "abxxc!", 2, 2, 4, true},
		{"delete_run", "abxxc!", "abc!", 2, 4, 2, true},
		{"replace_run", "abxc!", "abyyc!", 2, 3, 4, true},
		{"erase_run", "!abc!", "!!", 1, 4, 1, false},
		{"new_run", "!!", "!abc!", 1, 1, 4, false},
		{"mixed_classes", "abc!", "abx1c!", 2, 2, 4, false},
		{"newline", "abc!", "ab\nc!", 2, 2, 3, false},
		{"utf8", "abc!", "abéc!", 2, 2, 4, false},
		{"same_width", "abc!", "axc!", 1, 2, 2, false},
		{"bad_delta", "abc!", "abxxc!", 2, 2, 3, false},
		{"bad_span", "abc!", "abc!", 3, 5, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := LengthNeutralScannerEdit(runClasses{}, []byte(tc.old), []byte(tc.next), TokenEdit{Start: tc.start, OldEnd: tc.oldEnd, NewEnd: tc.newEnd})
			if got != tc.want {
				t.Fatalf("certificate=%t, want %t", got, tc.want)
			}
		})
	}
	if LengthNeutralScannerEdit(nil, []byte("abc"), []byte("abbc"), TokenEdit{Start: 2, OldEnd: 2, NewEnd: 3}) {
		t.Fatal("unknown scanner admitted")
	}
}

func TestTokenEditProjection(t *testing.T) {
	e := TokenEdit{Start: 5, OldEnd: 8, NewEnd: 6, Row: 2}
	for _, tc := range []struct {
		old, next uint32
		ok        bool
	}{{4, 4, true}, {5, 5, true}, {6, 0, false}, {7, 0, false}, {8, 6, true}, {12, 10, true}} {
		next, ok := e.Byte(tc.old)
		if next != tc.next || ok != tc.ok {
			t.Fatalf("byte %d: %d/%t", tc.old, next, ok)
		}
	}
	if row, col, ok := e.Point(12, 2, 12); !ok || row != 2 || col != 10 {
		t.Fatal("same-row projection")
	}
	if row, col, ok := e.Point(12, 3, 1); !ok || row != 3 || col != 1 {
		t.Fatal("later-row projection")
	}
	if _, _, ok := e.Point(12, 2, 1); ok {
		t.Fatal("column underflow admitted")
	}
	if _, ok := (TokenEdit{Start: 1, OldEnd: 1, NewEnd: 2}).Byte(^uint32(0)); ok {
		t.Fatal("byte overflow admitted")
	}
}

func TestTokenReadBoundInverseEdits(t *testing.T) {
	key := new(int)
	var anchor TokenReadBound
	bound := uint32(80)
	for i := 0; i < 1000; i++ {
		oldWidth, newWidth := uint32(20), uint32(21)
		want := uint32(81)
		if i%2 != 0 {
			oldWidth, newWidth, want = 21, 20, 80
		}
		var ok bool
		anchor, bound, ok = anchor.Edit(key, oldWidth, newWidth, bound)
		if !ok || bound != want {
			t.Fatalf("edit %d: bound=%d, want %d", i, bound, want)
		}
	}
	_, bound, ok := anchor.Edit(new(int), 30, 31, 100)
	if !ok || bound != 101 {
		t.Fatal("new token did not reanchor")
	}
	_, _, ok = (TokenReadBound{}).Edit(key, 1, 2, ^uint32(0))
	if ok {
		t.Fatal("read bound overflow admitted")
	}
}
