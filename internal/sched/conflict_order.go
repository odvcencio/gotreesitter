package sched

// ConflictBranchOrder gives the last action the source version's priority.
// Earlier actions keep their allocated priorities, so no alternative is
// discarded. Callers use this only for a certified grammar artifact.
func ConflictBranchOrder(ordinal, actionCount int, source, allocated uint64) uint64 {
	if ordinal == actionCount-1 {
		return source
	}
	return allocated
}
