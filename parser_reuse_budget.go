package gotreesitter

// incrementalReuseNodeBudget bounds the nodes an old-tree reuse parse may
// build before it must show reuse. Four times the larger of the old tree's
// built nodes and the fresh-parse arena estimate covers ordinary edits with
// wide headroom. The floor keeps small trees clear of normal recovery work.
// Issue #454: a single-byte C delete built 3.2 million nodes for a 68
// thousand node tree before the memory budget stopped it; a fresh parse of
// the edited file needs 68 thousand.
func incrementalReuseNodeBudget(oldTree *Tree, sourceLen int) int {
	const floor = 256 * 1024
	oldNodes := 0
	if oldTree != nil {
		oldNodes = oldTree.rawParseRuntime().NodesAllocated
	}
	base := max(oldNodes, parseFullArenaInitialNodeCapacity(sourceLen))
	if base > (1<<31-1)/4 {
		return 1<<31 - 1
	}
	return max(floor, 4*base)
}

// incrementalReuseHostile reports that the parse has reused less than one
// eighth of the source so far. The incremental timing record is present on
// every old-tree reuse parse; it is the same counter the profile reports.
func incrementalReuseHostile(timing *incrementalParseTiming, sourceLen int) bool {
	if timing == nil {
		return false
	}
	return timing.reusedBytes*8 < uint64(sourceLen)
}

// incrementalReuseBudgetArmed reports whether an old-tree reuse parse may
// stop on the reuse budget. The stop is safe only when the plain full-parse
// rescue can run afterwards (shouldRetryIncrementalMemoryBudgetAsPlainFull),
// which declines sources above fullParseRetryMaxSourceBytes. A larger source
// keeps the unbudgeted reuse parse, which completes as it did before the
// budget existed, instead of publishing a truncated tree.
func incrementalReuseBudgetArmed(sourceLen int) bool {
	return sourceLen > 0 && sourceLen <= fullParseRetryMaxSourceBytes
}
