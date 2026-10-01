package incr

// ReductionFragile rejects a reduction whose published span or selected path
// cannot supply an independent subtree reuse certificate.
func ReductionFragile(actualEnd, reducedEnd uint32, versions, links, actions, popVisits int) bool {
	return actualEnd != reducedEnd || versions > 1 || links > 1 || actions > 1 || popVisits > 1
}
