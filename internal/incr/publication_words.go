package incr

import "math"

// PublicationWord addresses a proven arena slot without searching its pointer
// across slabs. The caller verifies node ownership and slot order first, then
// accounts the returned growth in its arena. Allocation preserves the legacy
// word arrays' capacity and budget semantics.
func PublicationWord(words *[]uint32, index, slots int, reserve func(int64) bool) (*uint32, int64) {
	if index < 0 || index >= slots {
		return nil, 0
	}
	var growth int64
	if index >= len(*words) {
		if cap(*words) >= slots {
			*words = (*words)[:slots]
		} else {
			if uint64(slots-cap(*words)) > math.MaxInt64/4 || reserve == nil {
				return nil, 0
			}
			growth = int64(slots-cap(*words)) * 4
			if !reserve(growth) {
				return nil, 0
			}
			next := make([]uint32, slots)
			copy(next, *words)
			*words = next
		}
	}
	return &(*words)[index], growth
}
