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
			calls := 0
			value, ok := SingleTokenGap(tc.start, tc.end, func() (string, uint32, uint32, bool) {
				calls++
				return "token", tc.left, tc.right, tc.eligible
			}, func(token string) bool { return token == "token" && tc.padding })
			if ok != tc.want || (ok && value != "token") || (!ok && value != "") {
				t.Fatalf("token=%q exact=%t, want exact=%t", value, ok, tc.want)
			}
			if tc.start >= tc.end && calls != 0 {
				t.Fatal("empty gap invoked lexer")
			}
		})
	}
	t.Run("missing padding proof", func(t *testing.T) {
		_, ok := SingleTokenGap(0, 2, func() (string, uint32, uint32, bool) {
			return "token", 1, 2, true
		}, nil)
		if ok {
			t.Fatal("accepted a leading gap without a padding proof")
		}
	})
}
