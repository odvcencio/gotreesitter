package parsercorephase0

import "math"

// alternativeSetPrepareSpill preserves every recorded member when a new arena
// segment cannot be named by a one-based uint32 reference. Marking overflow
// prevents an incomplete recorded set from serving as a containment witness.
func alternativeSetPrepareSpill(set *AlternativeSet, start int) (uint32, bool) {
	if uint64(start) >= uint64(math.MaxUint32) {
		set.flags |= alternativeSetFlagOverflowed
		return 0, false
	}
	return uint32(start) + 1, true
}
