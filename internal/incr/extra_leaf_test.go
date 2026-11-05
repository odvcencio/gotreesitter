package incr

import "testing"

func TestExtraLeafNeedsEveryCondition(t *testing.T) {
	for mask := 0; mask < 16; mask++ {
		extraLeaf, sameToken, sameState, loneShift := mask&1 != 0, mask&2 != 0, mask&4 != 0, mask&8 != 0
		want := mask == 15
		if got := ExtraLeaf(extraLeaf, sameToken, sameState, loneShift); got != want {
			t.Errorf("ExtraLeaf(%t, %t, %t, %t) = %t, want %t", extraLeaf, sameToken, sameState, loneShift, got, want)
		}
	}
}
