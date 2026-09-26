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
// eighth of the source so far. reusedBytes is tracked unconditionally by the
// caller (parseInternal's block-splice loop), independent of whether a
// profiling *incrementalParseTiming record is present: this stop guards
// correctness and cost on EVERY old-tree reuse parse, not only calls made
// through ParseIncrementalProfiled. Gating it on timing != nil (as an earlier
// version did) let plain ParseIncremental skip the budget stop entirely,
// because it never allocates a timing record -- profiling was choosing the
// route instead of only observing it (issue #454 §6).
func incrementalReuseHostile(reusedBytes uint64, sourceLen int) bool {
	return reusedBytes*8 < uint64(sourceLen)
}

// Stop large edits when reuse has not paid for the nodes already built.
// A fresh parse is the certified fallback for this stop reason.
func incrementalReusePoorYield(oldTree *Tree, nodesBuilt int, reusedBytes uint64, sourceLen, maxStacksSeen int, progressByte, editedTopEnd uint32) bool {
	if oldTree == nil || sourceLen < 64<<10 || reusedBytes*2 >= uint64(sourceLen) || len(oldTree.edits) != 1 {
		return false
	}
	// A middle edit can reuse its suffix only after the parser reaches it.
	if oldTree.edits[0].StartByte >= uint32(sourceLen/8) {
		return false
	}
	// A live GLR fork shows that rebuilding has entered the costly path.
	if maxStacksSeen < 2 {
		return false
	}
	root := rawRootOrNil(oldTree)
	if root == nil {
		return false
	}
	oldNodes := oldTree.rawParseRuntime().NodesAllocated
	minBuilt := max(4096, oldNodes/10)
	if root.ChildCount() > 4 {
		if oldTree.Language() == nil || oldTree.Language().Name != "dart" || editedTopEnd == 0 || progressByte == 0 {
			return false
		}
		// Estimate reuse through the edited top-level child at the rate
		// observed so far. Later siblings can all remain reusable.
		progress := min(uint64(progressByte), uint64(sourceLen))
		topEnd := min(uint64(editedTopEnd), uint64(sourceLen))
		projected := reusedBytes + uint64(sourceLen) - progress
		if progress < topEnd {
			projected = reusedBytes + reusedBytes*(topEnd-progress)/progress + uint64(sourceLen) - topEnd
		}
		if projected*20 >= uint64(sourceLen)*11 {
			return false
		}
		minBuilt = max(4096, oldNodes/20)
	}
	return oldNodes > 0 && nodesBuilt > minBuilt
}

func incrementalReuseEditedTopLevelEnd(oldTree *Tree) uint32 {
	if oldTree == nil || len(oldTree.edits) != 1 || oldTree.Language() == nil || oldTree.Language().Name != "dart" {
		return 0
	}
	root := rawRootOrNil(oldTree)
	if root == nil || root.ChildCount() <= 4 {
		return 0
	}
	start := oldTree.edits[0].StartByte
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child != nil && child.StartByte() <= start && start < child.EndByte() {
			return child.EndByte()
		}
	}
	return 0
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
