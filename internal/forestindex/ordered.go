// Package forestindex narrows boundary searches during forest selection.
package forestindex

// OrderedEnds proves that every child is present and its end byte does not
// precede the previous child's end. Equal boundaries are allowed.
func OrderedEnds(count int, boundary func(int) (uint32, bool)) bool {
	var previous uint32
	for i := 0; i < count; i++ {
		end, present := boundary(i)
		if !present || (i > 0 && end < previous) {
			return false
		}
		previous = end
	}
	return true
}

// UpperBound returns the first child ending after end. Call only after
// OrderedEnds succeeds for the same immutable children. This excludes children
// after the candidate boundary while preserving reverse-order duplicate scans.
func UpperBound(count int, end uint32, boundary func(int) uint32) int {
	lo, hi := 0, count
	for lo < hi {
		mid := lo + (hi-lo)/2
		if boundary(mid) <= end {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}
