package parsercorephase0

import (
	"math"
	"testing"
)

func TestAlternativeSetSpillReferenceCheckedBeforeMutation(t *testing.T) {
	for _, start := range []int{-1, 0, 1, math.MaxInt} {
		set := NewAlternativeSetMember(65535, 65534)
		before := set
		ref, ok := alternativeSetPrepareSpill(&set, start)
		wantOK := start >= 0 && uint64(start) < uint64(math.MaxUint32)
		if ok != wantOK {
			t.Fatalf("start=%d ref=%d ok=%v", start, ref, ok)
		}
		if ok {
			if ref != uint32(start)+1 || set != before {
				t.Fatal("valid offset changed exact set")
			}
		} else {
			before.flags |= alternativeSetFlagOverflowed
			if ref != 0 || set != before {
				t.Fatal("overflow lost members or flags")
			}
		}
	}
}
func TestAlternativeSetSpillReferenceFullUint32Range(t *testing.T) {
	upper := uint64(math.MaxUint32)
	if uint64(math.MaxInt) < upper {
		t.Skip("offset boundary exceeds int width")
	}
	for _, start := range []int{int(upper - 1), int(upper), int(upper + 1)} {
		set := NewAlternativeSetMember(65535, 65534)
		before := set
		ref, ok := alternativeSetPrepareSpill(&set, start)
		if uint64(start) < upper {
			if !ok || ref != math.MaxUint32 || set != before {
				t.Fatal("last representable offset rejected or narrowed")
			}
		} else {
			if ok || ref != 0 || !set.Overflowed() || set.inline != before.inline || set.count != before.count {
				t.Fatal("unrepresentable offset mutated membership")
			}
		}
	}
}
