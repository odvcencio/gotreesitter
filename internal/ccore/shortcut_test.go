package ccore

import "testing"

func TestShortcutEditReason(t *testing.T) {
	edit := editRange{start: Length{bytes: 1, col: 1}, oldEnd: Length{bytes: 2, col: 2}, newEnd: Length{bytes: 2, col: 2}}
	cases := []struct {
		name, old, src string
		edit           editRange
		want           string
	}{
		{"digit", "[1]", "[2]", edit, ""},
		{"old newline", "[\n]", "[2]", edit, "newline"},
		{"new newline", "[1]", "[\n]", edit, "newline"},
		{"outside edit", "[1]", "{2]", edit, "edit-mismatch"},
		{"out of bounds", "[", "[", edit, "edit-mismatch"},
		{"length change", "[1]", "[22]", editRange{start: edit.start, oldEnd: edit.oldEnd, newEnd: Length{bytes: 3, col: 3}}, "not-same-length"},
		{"bad columns", "[1]", "[2]", editRange{start: edit.start, oldEnd: Length{bytes: 2}, newEnd: edit.newEnd}, "not-same-length"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortcutEditReason([]byte(tc.old), []byte(tc.src), tc.edit); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
