package incr

// FrontierArenaCapacity keeps cold reservations small. Once a parse records
// its dirty frontier, the bounded observed size replaces the source estimate.
func FrontierArenaCapacity(sourceBytes, hint, base, limit int) int {
	if hint > 0 {
		return max(base, min(hint, limit))
	}
	return max(base, min(sourceBytes/8, 4*1024))
}

// RetainFrontierArena selects an existing larger pool when the certified
// frontier exceeds the smaller pool's unchanged retention ceiling.
func RetainFrontierArena(certified bool, capacity, incrementalLimit int) bool {
	return certified && capacity > incrementalLimit
}

// RetainDependencyScratchForDemand discards a previous input's oversized
// allocation after an active producer used less than half of it. The next
// parse grows from the normal minimum; no allocation is needed to shrink.
// Call only with an active parse's observed length, never an idle reset's zero.
func RetainDependencyScratchForDemand(entries []uint32, minimum int) []uint32 {
	if cap(entries) > minimum && len(entries) < cap(entries)-len(entries) {
		return nil
	}
	return entries
}

// ResetDependencyScratch retains leaf provenance first: it remains useful even
// after forward dependency tracking abstains. Both buffers share one limit in
// uint32 entries, and every retained entry loses its previous authorization.
func ResetDependencyScratch(ends, leafWords []uint32, limit int) ([]uint32, []uint32) {
	if cap(leafWords) > limit {
		leafWords = nil
	}
	if cap(ends) > limit-cap(leafWords) {
		ends = nil
	}
	clear(ends[:cap(ends)])
	clear(leafWords[:cap(leafWords)])
	return ends[:0], leafWords[:0]
}
