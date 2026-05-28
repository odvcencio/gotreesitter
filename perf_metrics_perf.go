//go:build perf

package gotreesitter

import "sync/atomic"

const (
	perfCountersEnabled = true
	perfMergeHistBins   = maxGLRStacks + 2
	perfForkHistBins    = 8 // 2..8, 9+
)

type perfCountersData struct {
	mergeCalls                      atomic.Uint64
	mergeDeadPruned                 atomic.Uint64
	mergePerKeyOverflow             atomic.Uint64
	mergeReplacements               atomic.Uint64
	stackEquivalentCalls            atomic.Uint64
	stackEquivalentTrue             atomic.Uint64
	stackEqHashMissSkips            atomic.Uint64
	stackCompareCalls               atomic.Uint64
	conflictRR                      atomic.Uint64
	conflictRS                      atomic.Uint64
	conflictOther                   atomic.Uint64
	forkCount                       atomic.Uint64
	firstConflictToken              atomic.Uint64
	maxConcurrentStacks             atomic.Uint64
	lexBytes                        atomic.Uint64
	lexTokens                       atomic.Uint64
	reuseNodesVisited               atomic.Uint64
	reuseNodesPushed                atomic.Uint64
	reuseNodesPopped                atomic.Uint64
	reuseCandidatesChecked          atomic.Uint64
	reuseSuccesses                  atomic.Uint64
	reuseLeafSuccesses              atomic.Uint64
	reuseNonLeafChecks              atomic.Uint64
	reuseNonLeafSuccesses           atomic.Uint64
	reuseNonLeafBytes               atomic.Uint64
	reuseNonLeafNoGoto              atomic.Uint64
	reuseNonLeafNoGotoTerm          atomic.Uint64
	reuseNonLeafNoGotoNt            atomic.Uint64
	reuseNonLeafStateMiss           atomic.Uint64
	reuseNonLeafStateZero           atomic.Uint64
	mergeHashZero                   atomic.Uint64
	globalCapCulls                  atomic.Uint64
	globalCapCullDropped            atomic.Uint64
	reduceChainSteps                atomic.Uint64
	reduceChainMaxLen               atomic.Uint64
	reduceChainBreakMulti           atomic.Uint64
	reduceChainBreakShift           atomic.Uint64
	reduceChainBreakAccept          atomic.Uint64
	reduceChainHintCandidates       atomic.Uint64
	reduceChainHintTaken            atomic.Uint64
	reduceChainHintSteps            atomic.Uint64
	reduceChainHintTerminalOK       atomic.Uint64
	reduceChainHintTerminalMismatch atomic.Uint64
	reduceChainHintLimit            atomic.Uint64
	reduceChainHintDead             atomic.Uint64
	reduceChainHintUnexpected       atomic.Uint64
	parentChildPointers             atomic.Uint64
	reduceChildrenFastGSS           atomic.Uint64
	reduceChildrenAllVis            atomic.Uint64
	reduceChildrenNoAlias           atomic.Uint64
	reduceChildrenScratch           atomic.Uint64
	reduceScratchNoAlias            atomic.Uint64
	reduceScratchGeneral            atomic.Uint64
	extraNodes                      atomic.Uint64
	errorNodes                      atomic.Uint64
	mergeStacksInHist               [perfMergeHistBins]atomic.Uint64
	mergeAliveHist                  [perfMergeHistBins]atomic.Uint64
	mergeOutHist                    [perfMergeHistBins]atomic.Uint64
	forkActionsHist                 [perfForkHistBins]atomic.Uint64
	cloneTreeCalls                  atomic.Uint64
	cloneTreePublicNodes            atomic.Uint64
	cloneTreeFinalRefs              atomic.Uint64
	cloneTreeCompactCopies          atomic.Uint64
	cloneTreeChildRefs              atomic.Uint64
	cloneOffsetCalls                atomic.Uint64
	cloneOffsetPublicNodes          atomic.Uint64
	cloneOffsetCopies               atomic.Uint64
	cloneOffsetShifted              atomic.Uint64
	nodeEditCalls                   atomic.Uint64
	nodeEditNoopCalls               atomic.Uint64
	nodeEditCompactRefs             atomic.Uint64
	nodeEditShifted                 atomic.Uint64
	nodeEditMarked                  atomic.Uint64
	denseMutationCalls              atomic.Uint64
	denseMutationDrains             atomic.Uint64
	mutationChildRefCOW             atomic.Uint64
}

var perfCounters perfCountersData

func ResetPerfCounters() {
	perfCounters.mergeCalls.Store(0)
	perfCounters.mergeDeadPruned.Store(0)
	perfCounters.mergePerKeyOverflow.Store(0)
	perfCounters.mergeReplacements.Store(0)
	perfCounters.stackEquivalentCalls.Store(0)
	perfCounters.stackEquivalentTrue.Store(0)
	perfCounters.stackEqHashMissSkips.Store(0)
	perfCounters.stackCompareCalls.Store(0)
	perfCounters.conflictRR.Store(0)
	perfCounters.conflictRS.Store(0)
	perfCounters.conflictOther.Store(0)
	perfCounters.forkCount.Store(0)
	perfCounters.firstConflictToken.Store(0)
	perfCounters.maxConcurrentStacks.Store(0)
	perfCounters.lexBytes.Store(0)
	perfCounters.lexTokens.Store(0)
	perfCounters.reuseNodesVisited.Store(0)
	perfCounters.reuseNodesPushed.Store(0)
	perfCounters.reuseNodesPopped.Store(0)
	perfCounters.reuseCandidatesChecked.Store(0)
	perfCounters.reuseSuccesses.Store(0)
	perfCounters.reuseLeafSuccesses.Store(0)
	perfCounters.reuseNonLeafChecks.Store(0)
	perfCounters.reuseNonLeafSuccesses.Store(0)
	perfCounters.reuseNonLeafBytes.Store(0)
	perfCounters.reuseNonLeafNoGoto.Store(0)
	perfCounters.reuseNonLeafNoGotoTerm.Store(0)
	perfCounters.reuseNonLeafNoGotoNt.Store(0)
	perfCounters.reuseNonLeafStateMiss.Store(0)
	perfCounters.reuseNonLeafStateZero.Store(0)
	perfCounters.reduceChainHintCandidates.Store(0)
	perfCounters.reduceChainHintTaken.Store(0)
	perfCounters.reduceChainHintSteps.Store(0)
	perfCounters.reduceChainHintTerminalOK.Store(0)
	perfCounters.reduceChainHintTerminalMismatch.Store(0)
	perfCounters.reduceChainHintLimit.Store(0)
	perfCounters.reduceChainHintDead.Store(0)
	perfCounters.reduceChainHintUnexpected.Store(0)
	perfCounters.parentChildPointers.Store(0)
	perfCounters.reduceChildrenFastGSS.Store(0)
	perfCounters.reduceChildrenAllVis.Store(0)
	perfCounters.reduceChildrenNoAlias.Store(0)
	perfCounters.reduceChildrenScratch.Store(0)
	perfCounters.reduceScratchNoAlias.Store(0)
	perfCounters.reduceScratchGeneral.Store(0)
	perfCounters.extraNodes.Store(0)
	perfCounters.errorNodes.Store(0)
	for i := range perfCounters.mergeStacksInHist {
		perfCounters.mergeStacksInHist[i].Store(0)
	}
	for i := range perfCounters.mergeAliveHist {
		perfCounters.mergeAliveHist[i].Store(0)
	}
	perfCounters.mergeHashZero.Store(0)
	perfCounters.globalCapCulls.Store(0)
	perfCounters.globalCapCullDropped.Store(0)
	perfCounters.reduceChainSteps.Store(0)
	perfCounters.reduceChainMaxLen.Store(0)
	perfCounters.reduceChainBreakMulti.Store(0)
	perfCounters.reduceChainBreakShift.Store(0)
	perfCounters.reduceChainBreakAccept.Store(0)
	for i := range perfCounters.mergeOutHist {
		perfCounters.mergeOutHist[i].Store(0)
	}
	for i := range perfCounters.forkActionsHist {
		perfCounters.forkActionsHist[i].Store(0)
	}
	perfCounters.cloneTreeCalls.Store(0)
	perfCounters.cloneTreePublicNodes.Store(0)
	perfCounters.cloneTreeFinalRefs.Store(0)
	perfCounters.cloneTreeCompactCopies.Store(0)
	perfCounters.cloneTreeChildRefs.Store(0)
	perfCounters.cloneOffsetCalls.Store(0)
	perfCounters.cloneOffsetPublicNodes.Store(0)
	perfCounters.cloneOffsetCopies.Store(0)
	perfCounters.cloneOffsetShifted.Store(0)
	perfCounters.nodeEditCalls.Store(0)
	perfCounters.nodeEditNoopCalls.Store(0)
	perfCounters.nodeEditCompactRefs.Store(0)
	perfCounters.nodeEditShifted.Store(0)
	perfCounters.nodeEditMarked.Store(0)
	perfCounters.denseMutationCalls.Store(0)
	perfCounters.denseMutationDrains.Store(0)
	perfCounters.mutationChildRefCOW.Store(0)
}

func PerfCountersSnapshot() PerfCounters {
	var out PerfCounters
	out.MergeCalls = perfCounters.mergeCalls.Load()
	out.MergeDeadPruned = perfCounters.mergeDeadPruned.Load()
	out.MergePerKeyOverflow = perfCounters.mergePerKeyOverflow.Load()
	out.MergeReplacements = perfCounters.mergeReplacements.Load()
	out.StackEquivalentCalls = perfCounters.stackEquivalentCalls.Load()
	out.StackEquivalentTrue = perfCounters.stackEquivalentTrue.Load()
	out.StackEqHashMissSkips = perfCounters.stackEqHashMissSkips.Load()
	out.StackCompareCalls = perfCounters.stackCompareCalls.Load()
	out.ConflictRR = perfCounters.conflictRR.Load()
	out.ConflictRS = perfCounters.conflictRS.Load()
	out.ConflictOther = perfCounters.conflictOther.Load()
	out.ForkCount = perfCounters.forkCount.Load()
	out.FirstConflictToken = perfCounters.firstConflictToken.Load()
	out.MaxConcurrentStacks = perfCounters.maxConcurrentStacks.Load()
	out.LexBytes = perfCounters.lexBytes.Load()
	out.LexTokens = perfCounters.lexTokens.Load()
	out.ReuseNodesVisited = perfCounters.reuseNodesVisited.Load()
	out.ReuseNodesPushed = perfCounters.reuseNodesPushed.Load()
	out.ReuseNodesPopped = perfCounters.reuseNodesPopped.Load()
	out.ReuseCandidatesChecked = perfCounters.reuseCandidatesChecked.Load()
	out.ReuseSuccesses = perfCounters.reuseSuccesses.Load()
	out.ReuseLeafSuccesses = perfCounters.reuseLeafSuccesses.Load()
	out.ReuseNonLeafChecks = perfCounters.reuseNonLeafChecks.Load()
	out.ReuseNonLeafSuccesses = perfCounters.reuseNonLeafSuccesses.Load()
	out.ReuseNonLeafBytes = perfCounters.reuseNonLeafBytes.Load()
	out.ReuseNonLeafNoGoto = perfCounters.reuseNonLeafNoGoto.Load()
	out.ReuseNonLeafNoGotoTerm = perfCounters.reuseNonLeafNoGotoTerm.Load()
	out.ReuseNonLeafNoGotoNt = perfCounters.reuseNonLeafNoGotoNt.Load()
	out.ReuseNonLeafStateMiss = perfCounters.reuseNonLeafStateMiss.Load()
	out.ReuseNonLeafStateZero = perfCounters.reuseNonLeafStateZero.Load()
	for i := range out.MergeStacksInHist {
		out.MergeStacksInHist[i] = perfCounters.mergeStacksInHist[i].Load()
	}
	for i := range out.MergeAliveHist {
		out.MergeAliveHist[i] = perfCounters.mergeAliveHist[i].Load()
	}
	out.MergeHashZero = perfCounters.mergeHashZero.Load()
	out.GlobalCapCulls = perfCounters.globalCapCulls.Load()
	out.GlobalCapCullDropped = perfCounters.globalCapCullDropped.Load()
	out.ReduceChainSteps = perfCounters.reduceChainSteps.Load()
	out.ReduceChainMaxLen = perfCounters.reduceChainMaxLen.Load()
	out.ReduceChainBreakMulti = perfCounters.reduceChainBreakMulti.Load()
	out.ReduceChainBreakShift = perfCounters.reduceChainBreakShift.Load()
	out.ReduceChainBreakAccept = perfCounters.reduceChainBreakAccept.Load()
	out.ReduceChainHintCandidates = perfCounters.reduceChainHintCandidates.Load()
	out.ReduceChainHintTaken = perfCounters.reduceChainHintTaken.Load()
	out.ReduceChainHintSteps = perfCounters.reduceChainHintSteps.Load()
	out.ReduceChainHintTerminalOK = perfCounters.reduceChainHintTerminalOK.Load()
	out.ReduceChainHintTerminalMismatch = perfCounters.reduceChainHintTerminalMismatch.Load()
	out.ReduceChainHintLimit = perfCounters.reduceChainHintLimit.Load()
	out.ReduceChainHintDead = perfCounters.reduceChainHintDead.Load()
	out.ReduceChainHintUnexpected = perfCounters.reduceChainHintUnexpected.Load()
	out.ParentChildPointers = perfCounters.parentChildPointers.Load()
	out.ReduceChildrenFastGSS = perfCounters.reduceChildrenFastGSS.Load()
	out.ReduceChildrenAllVis = perfCounters.reduceChildrenAllVis.Load()
	out.ReduceChildrenNoAlias = perfCounters.reduceChildrenNoAlias.Load()
	out.ReduceChildrenScratch = perfCounters.reduceChildrenScratch.Load()
	out.ReduceScratchNoAlias = perfCounters.reduceScratchNoAlias.Load()
	out.ReduceScratchGeneral = perfCounters.reduceScratchGeneral.Load()
	out.ExtraNodes = perfCounters.extraNodes.Load()
	out.ErrorNodes = perfCounters.errorNodes.Load()
	for i := range out.MergeOutHist {
		out.MergeOutHist[i] = perfCounters.mergeOutHist[i].Load()
	}
	for i := range out.ForkActionsHist {
		out.ForkActionsHist[i] = perfCounters.forkActionsHist[i].Load()
	}
	out.CloneTreeCalls = perfCounters.cloneTreeCalls.Load()
	out.CloneTreePublicNodes = perfCounters.cloneTreePublicNodes.Load()
	out.CloneTreeFinalRefs = perfCounters.cloneTreeFinalRefs.Load()
	out.CloneTreeCompactCopies = perfCounters.cloneTreeCompactCopies.Load()
	out.CloneTreeChildRefs = perfCounters.cloneTreeChildRefs.Load()
	out.CloneOffsetCalls = perfCounters.cloneOffsetCalls.Load()
	out.CloneOffsetPublicNodes = perfCounters.cloneOffsetPublicNodes.Load()
	out.CloneOffsetCopies = perfCounters.cloneOffsetCopies.Load()
	out.CloneOffsetShifted = perfCounters.cloneOffsetShifted.Load()
	out.NodeEditCalls = perfCounters.nodeEditCalls.Load()
	out.NodeEditNoopCalls = perfCounters.nodeEditNoopCalls.Load()
	out.NodeEditCompactRefs = perfCounters.nodeEditCompactRefs.Load()
	out.NodeEditShifted = perfCounters.nodeEditShifted.Load()
	out.NodeEditMarked = perfCounters.nodeEditMarked.Load()
	out.DenseMutationCalls = perfCounters.denseMutationCalls.Load()
	out.DenseMutationDrains = perfCounters.denseMutationDrains.Load()
	out.MutationChildRefCOW = perfCounters.mutationChildRefCOW.Load()
	return out
}

func perfRecordMergeCall(stacksIn int) {
	perfCounters.mergeCalls.Add(1)
	perfCounters.mergeStacksInHist[perfMergeHistBin(stacksIn)].Add(1)
}

func perfRecordMergeAlive(alive, dead int) {
	if dead > 0 {
		perfCounters.mergeDeadPruned.Add(uint64(dead))
	}
	perfCounters.mergeAliveHist[perfMergeHistBin(alive)].Add(1)
}

func perfRecordMergeOut(stacksOut int) {
	perfCounters.mergeOutHist[perfMergeHistBin(stacksOut)].Add(1)
}

func perfRecordMergeHashZero() {
	perfCounters.mergeHashZero.Add(1)
}

func perfRecordGlobalCapCull(before, cap int) {
	perfCounters.globalCapCulls.Add(1)
	if before > cap {
		perfCounters.globalCapCullDropped.Add(uint64(before - cap))
	}
}

func perfRecordMergePerKeyOverflow() {
	perfCounters.mergePerKeyOverflow.Add(1)
}

func perfRecordMergeReplacement() {
	perfCounters.mergeReplacements.Add(1)
}

func perfRecordStackEquivalentCall() {
	perfCounters.stackEquivalentCalls.Add(1)
}

func perfRecordStackEquivalentTrue() {
	perfCounters.stackEquivalentTrue.Add(1)
}

func perfRecordStackEquivalentHashMissSkip() {
	perfCounters.stackEqHashMissSkips.Add(1)
}

func perfRecordStackCompare() {
	perfCounters.stackCompareCalls.Add(1)
}

func perfRecordConflictRR() {
	perfCounters.conflictRR.Add(1)
}

func perfRecordConflictRS() {
	perfCounters.conflictRS.Add(1)
}

func perfRecordConflictOther() {
	perfCounters.conflictOther.Add(1)
}

func perfRecordFork(actionCount int, tokenPos uint64) {
	perfCounters.forkCount.Add(1)
	perfCounters.forkActionsHist[perfForkHistBin(actionCount)].Add(1)
	if tokenPos == 0 {
		return
	}
	perfCounters.firstConflictToken.CompareAndSwap(0, tokenPos)
}

func perfRecordMaxConcurrentStacks(n int) {
	if n <= 0 {
		return
	}
	target := uint64(n)
	for {
		prev := perfCounters.maxConcurrentStacks.Load()
		if target <= prev {
			return
		}
		if perfCounters.maxConcurrentStacks.CompareAndSwap(prev, target) {
			return
		}
	}
}

func perfRecordLexed(bytes, tokens int) {
	if bytes > 0 {
		perfCounters.lexBytes.Add(uint64(bytes))
	}
	if tokens > 0 {
		perfCounters.lexTokens.Add(uint64(tokens))
	}
}

func perfRecordReuseVisited() {
	perfCounters.reuseNodesVisited.Add(1)
}

func perfRecordReusePushed(n int) {
	if n > 0 {
		perfCounters.reuseNodesPushed.Add(uint64(n))
	}
}

func perfRecordReusePopped() {
	perfCounters.reuseNodesPopped.Add(1)
}

func perfRecordReuseCandidates(n int) {
	if n > 0 {
		perfCounters.reuseCandidatesChecked.Add(uint64(n))
	}
}

func perfRecordReuseSuccess() {
	perfCounters.reuseSuccesses.Add(1)
}

func perfRecordReuseLeafSuccess() {
	perfCounters.reuseLeafSuccesses.Add(1)
}

func perfRecordReuseNonLeafCheck() {
	perfCounters.reuseNonLeafChecks.Add(1)
}

func perfRecordReuseNonLeafSuccess(bytes uint32) {
	perfCounters.reuseNonLeafSuccesses.Add(1)
	if bytes > 0 {
		perfCounters.reuseNonLeafBytes.Add(uint64(bytes))
	}
}

func perfRecordReuseNonLeafNoGoto() {
	perfCounters.reuseNonLeafNoGoto.Add(1)
}

func perfRecordReuseNonLeafNoGotoTerminal() {
	perfCounters.reuseNonLeafNoGotoTerm.Add(1)
}

func perfRecordReuseNonLeafNoGotoNonTerminal() {
	perfCounters.reuseNonLeafNoGotoNt.Add(1)
}

func perfRecordReuseNonLeafStateMiss() {
	perfCounters.reuseNonLeafStateMiss.Add(1)
}

func perfRecordReuseNonLeafStateZero() {
	perfCounters.reuseNonLeafStateZero.Add(1)
}

func perfRecordReduceChainStep(chainLen int) {
	perfCounters.reduceChainSteps.Add(1)
	if chainLen <= 0 {
		return
	}
	target := uint64(chainLen)
	for {
		prev := perfCounters.reduceChainMaxLen.Load()
		if target <= prev {
			return
		}
		if perfCounters.reduceChainMaxLen.CompareAndSwap(prev, target) {
			return
		}
	}
}

func perfRecordReduceChainBreakMulti() {
	perfCounters.reduceChainBreakMulti.Add(1)
}

func perfRecordReduceChainBreakShift() {
	perfCounters.reduceChainBreakShift.Add(1)
}

func perfRecordReduceChainBreakAccept() {
	perfCounters.reduceChainBreakAccept.Add(1)
}

func perfRecordReduceChainHintCandidate() {
	perfCounters.reduceChainHintCandidates.Add(1)
}

func perfRecordReduceChainHintTaken() {
	perfCounters.reduceChainHintTaken.Add(1)
}

func perfRecordReduceChainHintSteps(n int) {
	if n > 0 {
		perfCounters.reduceChainHintSteps.Add(uint64(n))
	}
}

func perfRecordReduceChainHintTerminalOK() {
	perfCounters.reduceChainHintTerminalOK.Add(1)
}

func perfRecordReduceChainHintTerminalMismatch() {
	perfCounters.reduceChainHintTerminalMismatch.Add(1)
}

func perfRecordReduceChainHintLimit() {
	perfCounters.reduceChainHintLimit.Add(1)
}

func perfRecordReduceChainHintDead() {
	perfCounters.reduceChainHintDead.Add(1)
}

func perfRecordReduceChainHintUnexpected() {
	perfCounters.reduceChainHintUnexpected.Add(1)
}

func perfRecordParentChildren(count int) {
	if count > 0 {
		perfCounters.parentChildPointers.Add(uint64(count))
	}
}

func perfRecordReduceChildrenFastGSS(count int) {
	if count > 0 {
		perfCounters.reduceChildrenFastGSS.Add(uint64(count))
	}
}

func perfRecordReduceChildrenAllVisible(count int) {
	if count > 0 {
		perfCounters.reduceChildrenAllVis.Add(uint64(count))
	}
}

func perfRecordReduceChildrenNoAlias(count int) {
	if count > 0 {
		perfCounters.reduceChildrenNoAlias.Add(uint64(count))
	}
}

func perfRecordReduceChildrenScratch(count int) {
	if count > 0 {
		perfCounters.reduceChildrenScratch.Add(uint64(count))
	}
}

func perfRecordReduceScratchNoAlias(count int) {
	if count > 0 {
		perfCounters.reduceScratchNoAlias.Add(uint64(count))
	}
}

func perfRecordReduceScratchGeneral(count int) {
	if count > 0 {
		perfCounters.reduceScratchGeneral.Add(uint64(count))
	}
}

func perfRecordExtraNode() {
	perfCounters.extraNodes.Add(1)
}

func perfRecordErrorNode() {
	perfCounters.errorNodes.Add(1)
}

func perfRecordCloneTreeCall() {
	perfCounters.cloneTreeCalls.Add(1)
}

func perfRecordCloneTreePublicNode() {
	perfCounters.cloneTreePublicNodes.Add(1)
}

func perfRecordCloneTreeFinalRefs(n int) {
	if n > 0 {
		perfCounters.cloneTreeFinalRefs.Add(uint64(n))
	}
}

func perfRecordCloneTreeCompactCopy() {
	perfCounters.cloneTreeCompactCopies.Add(1)
}

func perfRecordCloneTreeChildRefs(n int) {
	if n > 0 {
		perfCounters.cloneTreeChildRefs.Add(uint64(n))
	}
}

func perfRecordCloneOffsetCall() {
	perfCounters.cloneOffsetCalls.Add(1)
}

func perfRecordCloneOffsetPublicNode() {
	perfCounters.cloneOffsetPublicNodes.Add(1)
}

func perfRecordCloneOffsetCompactCopy() {
	perfCounters.cloneOffsetCopies.Add(1)
}

func perfRecordCloneOffsetShifted() {
	perfCounters.cloneOffsetShifted.Add(1)
}

func perfRecordNodeEditCall() {
	perfCounters.nodeEditCalls.Add(1)
}

func perfRecordNodeEditNoopCall() {
	perfCounters.nodeEditNoopCalls.Add(1)
}

func perfRecordNodeEditCompactRef() {
	perfCounters.nodeEditCompactRefs.Add(1)
}

func perfRecordNodeEditShifted() {
	perfCounters.nodeEditShifted.Add(1)
}

func perfRecordNodeEditMarked() {
	perfCounters.nodeEditMarked.Add(1)
}

func perfRecordDenseMutationChildrenCall() {
	perfCounters.denseMutationCalls.Add(1)
}

func perfRecordDenseMutationChildrenDrain() {
	perfCounters.denseMutationDrains.Add(1)
}

func perfRecordMutationChildRefCopyOnWrite(n int) {
	if n > 0 {
		perfCounters.mutationChildRefCOW.Add(uint64(n))
	}
}

func perfMergeHistBin(n int) int {
	if n < 0 {
		return 0
	}
	if n >= perfMergeHistBins {
		return perfMergeHistBins - 1
	}
	return n
}

func perfForkHistBin(actions int) int {
	if actions <= 2 {
		return 0
	}
	if actions >= 9 {
		return perfForkHistBins - 1
	}
	return actions - 2
}
