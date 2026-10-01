package incr

// RequiresFreshResult bounds the memory used to authenticate an unproven
// incremental frontier. At this size the published result is always a fresh
// parse, rather than a comparison with a second live tree.
func RequiresFreshResult(sourceBytes int) bool {
	return sourceBytes >= 512*1024
}
