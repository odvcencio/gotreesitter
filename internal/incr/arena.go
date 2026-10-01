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
