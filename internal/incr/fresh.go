package incr

// RequiresFreshResult bounds the memory used to authenticate an unproven
// incremental frontier. At this size the published result is always a fresh
// parse, rather than a comparison with a second live tree.
func RequiresFreshResult(sourceBytes int) bool {
	return sourceBytes >= 512*1024
}

// RequiresFreshWorkLimits keeps edited parses on the fresh work schedule.
// A reused subtree skips iterations, nodes, and stack checkpoints, so it
// cannot prove the same partial tree or stop as a fresh bounded parse.
func RequiresFreshWorkLimits(nodeLimit, iterationLimit, stackDepthLimit int) bool {
	return nodeLimit > 0 || iterationLimit > 0 || stackDepthLimit > 0
}
