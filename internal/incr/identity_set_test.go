package incr

import "testing"

func TestIdentitySetCollisionsGrowthAndClearing(t *testing.T) {
	const n = 10000
	values := make([]int, n)
	var set IdentitySet[int]
	for i := range values {
		if set.Contains(&values[i]) {
			t.Fatalf("unexpected membership %d", i)
		}
		if set.GrowthBytes() != 0 && !set.Grow() {
			t.Fatal("growth declined")
		}
		set.AddKnownAbsent(&values[i])
	}
	if set.Len() != n {
		t.Fatalf("count %d", set.Len())
	}
	for i := range values {
		if !set.Contains(&values[i]) {
			t.Fatalf("missing %d", i)
		}
	}
	if set.Contains(nil) {
		t.Fatal("nil member")
	}
	before := set.Bytes()
	set.Clear()
	if set.Len() != 0 || set.Bytes() != before {
		t.Fatal("clear changed retained capacity")
	}
	for i, p := range set.slots {
		if p != nil {
			t.Fatalf("slot %d retained pointer", i)
		}
	}
	for i := range values {
		if set.Contains(&values[i]) {
			t.Fatal("clear retained membership")
		}
	}
	set.Drop()
	if set.Bytes() != 0 || set.Capacity() != 0 {
		t.Fatal("drop retained storage")
	}
}
