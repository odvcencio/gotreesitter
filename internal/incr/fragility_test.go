package incr

import "testing"

func TestReductionFragile(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		actualEnd, reducedEnd          uint32
		versions, links, actions, pops int
		want                           bool
	}{
		{"independent", 10, 10, 1, 1, 1, 1, false},
		{"shorter public span", 9, 10, 1, 1, 1, 1, true},
		{"longer public span", 11, 10, 1, 1, 1, 1, true},
		{"versions", 10, 10, 2, 1, 1, 1, true},
		{"links", 10, 10, 1, 2, 1, 1, true},
		{"actions", 10, 10, 1, 1, 2, 1, true},
		{"pop paths", 10, 10, 1, 1, 1, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReductionFragile(tc.actualEnd, tc.reducedEnd, tc.versions, tc.links, tc.actions, tc.pops); got != tc.want {
				t.Fatalf("fragile=%t, want %t", got, tc.want)
			}
		})
	}
}
