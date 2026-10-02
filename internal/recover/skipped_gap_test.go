package recover

import "testing"

func TestSingleTokenGap(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		start, end, left, right uint32
		eligible, want          bool
	}{
		{"one terminal", 3, 4, 3, 4, true, true},
		{"leading padding", 1, 4, 3, 4, true, true},
		{"crosses lookahead", 1, 4, 3, 5, true, false},
		{"unfinished gap", 1, 4, 1, 2, true, false},
		{"before gap", 1, 4, 0, 4, true, false},
		{"zero width", 1, 4, 4, 4, true, false},
		{"ineligible token", 1, 4, 3, 4, false, false},
		{"empty gap", 4, 4, 4, 4, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			value, ok := SingleTokenGap(tc.start, tc.end, func() (string, uint32, uint32, bool) {
				calls++
				return "token", tc.left, tc.right, tc.eligible
			})
			if ok != tc.want || (ok && value != "token") || (!ok && value != "") {
				t.Fatalf("token=%q exact=%t, want exact=%t", value, ok, tc.want)
			}
			if tc.start >= tc.end && calls != 0 {
				t.Fatal("empty gap invoked lexer")
			}
		})
	}
}
