package incr

// SharedCandidate selects a subtree that every live version can consume.
// Adapters authenticate native subtree attributes, lexer modes, scanner
// checkpoints, and a common destination before any version is changed.
func SharedCandidate[N, H any](candidates []N, heads []H, candidate func(N) bool, head func(*H, N) bool) (N, bool) {
	for _, node := range candidates {
		if !candidate(node) {
			continue
		}
		accepted := true
		for i := range heads {
			if !head(&heads[i], node) {
				accepted = false
				break
			}
		}
		if accepted {
			return node, true
		}
	}
	var zero N
	return zero, false
}
