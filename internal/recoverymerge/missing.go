package recoverymerge

// MayContainMissing checks retained history without relying on public child
// flags or error-cost memo reads. An incomplete walk cannot prove a flag stale.
func MayContainMissing[T any](root T, maxDepth int, inspect func(T) (bool, int), childAt func(T, int) (T, bool)) bool {
	if maxDepth < 0 {
		// An unverified history cannot prove an existing error flag stale.
		return true
	}
	missing, count := inspect(root)
	if missing {
		return true
	}
	for i := 0; i < count; i++ {
		child, ok := childAt(root, i)
		if !ok || MayContainMissing(child, maxDepth-1, inspect, childAt) {
			return true
		}
	}
	return false
}
