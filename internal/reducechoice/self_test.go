package reducechoice

import "testing"

func TestLongestSelfGuards(t *testing.T) {
	one := Reduction{Symbol: 231, Children: 1, Plain: true}
	two := one
	two.Children = 2
	for _, tc := range []struct {
		name  string
		rows  []Reduction
		index int
		ok    bool
	}{
		{"wider", []Reduction{one, two}, 1, true},
		{"reverse", []Reduction{two, one}, 0, true},
		{"equal", []Reduction{one, one}, 0, false},
		{"single", []Reduction{one}, 0, false},
		{"shift", []Reduction{one, {Symbol: 231, Children: 2}}, 0, false},
		{"other-symbol", []Reduction{one, {Symbol: 232, Children: 2, Plain: true}}, 0, false},
		{"precedence", []Reduction{one, {Symbol: 231, Children: 2, Dynamic: 1, Plain: true}}, 0, false},
		{"field-production", []Reduction{one, {Symbol: 231, Children: 2, Production: 1, Plain: true}}, 0, false},
		{"empty-production", []Reduction{one, {Symbol: 231, Plain: true}}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, ok := LongestSelf(tc.rows, func(a Reduction) Reduction { return a })
			if ok != tc.ok || (ok && index != tc.index) {
				t.Fatalf("choice=%d/%t, want %d/%t", index, ok, tc.index, tc.ok)
			}
		})
	}
}
