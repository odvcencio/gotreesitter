package forestindex

import "testing"

func TestOrderedEndsRequiresPresentMonotoneChildren(t *testing.T) {
	for _, tc := range []struct {
		ends    []uint32
		missing int
		want    bool
	}{
		{nil, -1, true},
		{[]uint32{1, 2, 2, 9}, -1, true},
		{[]uint32{1, 9, 2}, -1, false},
		{[]uint32{1, 2, 2}, 1, false},
	} {
		if got := OrderedEnds(len(tc.ends), func(i int) (uint32, bool) { return tc.ends[i], i != tc.missing }); got != tc.want {
			t.Fatalf("ends=%v missing=%d: ordered=%t, want %t", tc.ends, tc.missing, got, tc.want)
		}
	}
}

func TestUpperBoundMatchesLinearBoundarySearch(t *testing.T) {
	for _, ends := range [][]uint32{nil, {0}, {1, 2, 2, 3, 9}, {5, 5, 5}} {
		for end := uint32(0); end <= 10; end++ {
			want := 0
			for want < len(ends) && ends[want] <= end {
				want++
			}
			if got := UpperBound(len(ends), end, func(i int) uint32 { return ends[i] }); got != want {
				t.Fatalf("ends=%v end=%d: upper=%d, want %d", ends, end, got, want)
			}
		}
	}
}
