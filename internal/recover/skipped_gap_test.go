package recover

import "testing"

func TestSingleTokenGap(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		start, end, left, right uint32
		eligible, padding, want bool
	}{
		{"one terminal", 3, 4, 3, 4, true, false, true},
		{"leading padding", 1, 4, 3, 4, true, true, true},
		{"unproven prefix", 1, 4, 3, 4, true, false, false},
		{"crosses lookahead", 1, 4, 3, 5, true, true, false},
		{"unfinished gap", 1, 4, 1, 2, true, false, false},
		{"before gap", 1, 4, 0, 4, true, false, false},
		{"zero width", 1, 4, 4, 4, true, true, false},
		{"ineligible token", 1, 4, 3, 4, false, true, false},
		{"empty gap", 4, 4, 4, 4, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := SingleTokenGap(tc.start, tc.end, "token", tc.left, tc.right, tc.eligible, tc.padding)
			if ok != tc.want || (ok && value != "token") || (!ok && value != "") {
				t.Fatalf("token=%q exact=%t, want exact=%t", value, ok, tc.want)
			}
		})
	}
	t.Run("missing padding proof", func(t *testing.T) {
		_, ok := SingleTokenGap(0, 2, "token", 1, 2, true, false)
		if ok {
			t.Fatal("accepted a leading gap without a padding proof")
		}
	})
}
