//go:build perf

package gotreesitter

import "sync/atomic"

const (
	perfCountersEnabled = true
	perfMergeHistBins   = maxGLRStacks + 2
	perfForkHistBins    = 8 // 2..8, 9+
)

type perfCountersData struct {
	mergeCalls           atomic.Uint64
	mergeDeadPruned      atomic.Uint64
	mergePerKeyOverflow  atomic.Uint64
	mergeReplacements    atomic.Uint64
	stackEquivalentCalls atomic.Uint64
	stackEquivalentTrue  atomic.Uint64
	stackEqHashMissSkips atomic.Uint64
	stackCompareCalls    atomic.Uint64
	conflictRR           atomic.Uint64
	conflictRS           atomic.Uint64
	conflictOther        atomic.Uint64
	forkCount            atomic.Uint64
	firstConflictToken   atomic.Uint64
	maxConcurrentStacks  atomic.Uint64
	lexBytes             atomic.Uint64
	lexTokens            atomic.Uint64
	// probeLexBytes and probeLexTokens count bytes and tokens the compact EOF
	// scanner quiescence probe lexes (proveCompactEOFScannerQuiescence,
	// parsercore_phase0_eof_scanner_quiescence.go). The probe re-runs the
	// external scanner outside the parse, once per head state, and must not
	// bill that lexing to lexBytes/lexTokens: those two name the parse's own
	// token stream.
	probeLexBytes                        atomic.Uint64
	probeLexTokens                       atomic.Uint64
	reuseNodesVisited                    atomic.Uint64
	reuseNodesPushed                     atomic.Uint64
	reuseNodesPopped                     atomic.Uint64
	reuseCandidatesChecked               atomic.Uint64
	reuseSuccesses                       atomic.Uint64
	reuseLeafSuccesses                   atomic.Uint64
	reuseNonLeafChecks                   atomic.Uint64
	reuseNonLeafSuccesses                atomic.Uint64
	reuseNonLeafBytes                    atomic.Uint64
	reuseNonLeafNoGoto                   atomic.Uint64
	reuseNonLeafNoGotoTerm               atomic.Uint64
	reuseNonLeafNoGotoNt                 atomic.Uint64
	reuseNonLeafStateMiss                atomic.Uint64
	reuseNonLeafStateZero                atomic.Uint64
	mergeHashZero                        atomic.Uint64
	globalCapCulls                       atomic.Uint64
	globalCapCullDropped                 atomic.Uint64
	reduceChainSteps                     atomic.Uint64
	reduceChainMaxLen                    atomic.Uint64
	reduceChainBreakMulti                atomic.Uint64
	reduceChainBreakShift                atomic.Uint64
	reduceChainBreakAccept               atomic.Uint64
	reduceChainHintCandidates            atomic.Uint64
	reduceChainHintTaken                 atomic.Uint64
	reduceChainHintSteps                 atomic.Uint64
	reduceChainHintTerminalOK            atomic.Uint64
	reduceChainHintTerminalMismatch      atomic.Uint64
	reduceChainHintLimit                 atomic.Uint64
	reduceChainHintDead                  atomic.Uint64
	reduceChainHintUnexpected            atomic.Uint64
	parentChildPointers                  atomic.Uint64
	reduceChildrenFastGSS                atomic.Uint64
	reduceChildrenAllVis                 atomic.Uint64
	reduceChildrenScratch                atomic.Uint64
	reduceScratchNoAlias                 atomic.Uint64
	reduceScratchGeneral                 atomic.Uint64
	reduceChildEmptyBuilds               atomic.Uint64
	reduceChildAllVisibleBuilds          atomic.Uint64
	reduceChildScratchNoAliasBuilds      atomic.Uint64
	reduceChildScratchGeneralBuilds      atomic.Uint64
	reduceForkCalls                      atomic.Uint64
	reduceForkWindows                    atomic.Uint64
	reduceForkMaxWindows                 atomic.Uint64
	postReduceMergeAttempts              atomic.Uint64
	postReduceMergePrimarySuccesses      atomic.Uint64
	postReduceMergePendingSuccesses      atomic.Uint64
	postReduceMergeDisabledSkips         atomic.Uint64
	postReduceMergeFinalizationRiskSkips atomic.Uint64
	pendingForkStackAppends              atomic.Uint64
	pendingForkStacksMaxLen              atomic.Uint64
	forestReduceCalls                    atomic.Uint64
	forestReduceZero                     atomic.Uint64
	forestReduceLinearNoExtras           atomic.Uint64
	forestReduceDFS                      atomic.Uint64
	forestReduceDFSLinks                 atomic.Uint64
	forestReduceDFSMultiLinkSteps        atomic.Uint64
	forestReduceDFSExtraLinks            atomic.Uint64
	forestReduceDFSVisits                atomic.Uint64
	forestReduceDFSPathEntries           atomic.Uint64
	forestReduceGotoHits                 atomic.Uint64
	forestReduceGotoMisses               atomic.Uint64
	forestReduceMaxPathLen               atomic.Uint64
	forestReduceMaxChildCount            atomic.Uint64
	forestCoalesceCalls                  atomic.Uint64
	forestCoalesceNewNodes               atomic.Uint64
	forestCoalesceLinkAppends            atomic.Uint64
	forestCoalesceDedupHits              atomic.Uint64
	forestCoalesceDedupReplacements      atomic.Uint64
	forestCoalescePreCapDrops            atomic.Uint64
	forestCoalesceCapDrops               atomic.Uint64
	forestCoalesceCapReplacements        atomic.Uint64
	extraNodes                           atomic.Uint64
	errorNodes                           atomic.Uint64
	syntheticReplayGapBridgeCalls        atomic.Uint64
	syntheticReplayGapSteps              atomic.Uint64
	syntheticReplayGapCursorAttempts     atomic.Uint64
	syntheticReplayGapCursorDedupHits    atomic.Uint64
	syntheticReplayGapCursorPeak         atomic.Uint64
	syntheticReplayGapLexAttempts        atomic.Uint64
	syntheticReplayGapLexCacheHits       atomic.Uint64
	syntheticReplayGapLexCacheMisses     atomic.Uint64
	syntheticReplayGapLexCacheStores     atomic.Uint64
	syntheticReplayGapLexCacheCapSkips   atomic.Uint64
	syntheticReplayAdvanceAttempts       atomic.Uint64
	syntheticReplayAdvanceCacheHits      atomic.Uint64
	syntheticReplayAdvanceCacheMisses    atomic.Uint64
	syntheticReplayAdvanceCacheStores    atomic.Uint64
	syntheticReplayAdvanceCacheCapSkips  atomic.Uint64
	syntheticReplayAdvanceZeroOutputs    atomic.Uint64
	syntheticReplayAdvanceSingleOutputs  atomic.Uint64
	syntheticReplayAdvanceMultiOutputs   atomic.Uint64
	syntheticReplayAdvanceOutputPeak     atomic.Uint64
	syntheticReplayAdvanceOutputFrames   atomic.Uint64
	syntheticReplayAdvanceUniqueOutputs  atomic.Uint64
	syntheticReplayAdvanceUniqueFrames   atomic.Uint64
	syntheticReplayAdvanceUniqueSingle   atomic.Uint64
	syntheticReplayAdvanceUniqueMulti    atomic.Uint64
	syntheticReplayAdvanceUniquePeak     atomic.Uint64
	syntheticReplayStackPushCalls        atomic.Uint64
	syntheticReplayStackInternHits       atomic.Uint64
	syntheticReplayCloseMemoHits         atomic.Uint64
	syntheticReplayCloseMemoMisses       atomic.Uint64
	syntheticReplayCloseMemoStores       atomic.Uint64
	syntheticReplayCloseMemoCapSkips     atomic.Uint64
	syntheticReplayCloseZeroOutputs      atomic.Uint64
	syntheticReplayCloseSingleOutputs    atomic.Uint64
	syntheticReplayCloseMultiOutputs     atomic.Uint64
	syntheticReplayCloseOutputPeak       atomic.Uint64
	syntheticReplayCloseOutputFrames     atomic.Uint64
	syntheticReplayCloseUniqueOutputs    atomic.Uint64
	syntheticReplayCloseUniqueFrames     atomic.Uint64
	syntheticReplayCloseUniqueSingle     atomic.Uint64
	syntheticReplayCloseUniqueMulti      atomic.Uint64
	syntheticReplayCloseUniquePeak       atomic.Uint64
	mergeStacksInHist                    [perfMergeHistBins]atomic.Uint64
	mergeAliveHist                       [perfMergeHistBins]atomic.Uint64
	mergeOutHist                         [perfMergeHistBins]atomic.Uint64
	forkActionsHist                      [perfForkHistBins]atomic.Uint64
	cloneTreeCalls                       atomic.Uint64
	cloneTreePublicNodes                 atomic.Uint64
	cloneTreeFinalRefs                   atomic.Uint64
	cloneTreeCompactCopies               atomic.Uint64
	cloneTreeChildRefs                   atomic.Uint64
	cloneOffsetCalls                     atomic.Uint64
	cloneOffsetPublicNodes               atomic.Uint64
	cloneOffsetCopies                    atomic.Uint64
	cloneOffsetShifted                   atomic.Uint64
	nodeEditCalls                        atomic.Uint64
	nodeEditNoopCalls                    atomic.Uint64
	nodeEditCompactRefs                  atomic.Uint64
	nodeEditShifted                      atomic.Uint64
	nodeEditMarked                       atomic.Uint64
	denseMutationCalls                   atomic.Uint64
	denseMutationDrains                  atomic.Uint64
	mutationChildRefCOW                  atomic.Uint64
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
	perfCounters.probeLexBytes.Store(0)
	perfCounters.probeLexTokens.Store(0)
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
	perfCounters.reduceChildrenScratch.Store(0)
	perfCounters.reduceScratchNoAlias.Store(0)
	perfCounters.reduceScratchGeneral.Store(0)
	perfCounters.reduceChildEmptyBuilds.Store(0)
	perfCounters.reduceChildAllVisibleBuilds.Store(0)
	perfCounters.reduceChildScratchNoAliasBuilds.Store(0)
	perfCounters.reduceChildScratchGeneralBuilds.Store(0)
	perfCounters.reduceForkCalls.Store(0)
	perfCounters.reduceForkWindows.Store(0)
	perfCounters.reduceForkMaxWindows.Store(0)
	perfCounters.postReduceMergeAttempts.Store(0)
	perfCounters.postReduceMergePrimarySuccesses.Store(0)
	perfCounters.postReduceMergePendingSuccesses.Store(0)
	perfCounters.postReduceMergeDisabledSkips.Store(0)
	perfCounters.postReduceMergeFinalizationRiskSkips.Store(0)
	perfCounters.pendingForkStackAppends.Store(0)
	perfCounters.pendingForkStacksMaxLen.Store(0)
	perfCounters.forestReduceCalls.Store(0)
	perfCounters.forestReduceZero.Store(0)
	perfCounters.forestReduceLinearNoExtras.Store(0)
	perfCounters.forestReduceDFS.Store(0)
	perfCounters.forestReduceDFSLinks.Store(0)
	perfCounters.forestReduceDFSMultiLinkSteps.Store(0)
	perfCounters.forestReduceDFSExtraLinks.Store(0)
	perfCounters.forestReduceDFSVisits.Store(0)
	perfCounters.forestReduceDFSPathEntries.Store(0)
	perfCounters.forestReduceGotoHits.Store(0)
	perfCounters.forestReduceGotoMisses.Store(0)
	perfCounters.forestReduceMaxPathLen.Store(0)
	perfCounters.forestReduceMaxChildCount.Store(0)
	perfCounters.forestCoalesceCalls.Store(0)
	perfCounters.forestCoalesceNewNodes.Store(0)
	perfCounters.forestCoalesceLinkAppends.Store(0)
	perfCounters.forestCoalesceDedupHits.Store(0)
	perfCounters.forestCoalesceDedupReplacements.Store(0)
	perfCounters.forestCoalescePreCapDrops.Store(0)
	perfCounters.forestCoalesceCapDrops.Store(0)
	perfCounters.forestCoalesceCapReplacements.Store(0)
	perfCounters.extraNodes.Store(0)
	perfCounters.errorNodes.Store(0)
	perfCounters.syntheticReplayGapBridgeCalls.Store(0)
	perfCounters.syntheticReplayGapSteps.Store(0)
	perfCounters.syntheticReplayGapCursorAttempts.Store(0)
	perfCounters.syntheticReplayGapCursorDedupHits.Store(0)
	perfCounters.syntheticReplayGapCursorPeak.Store(0)
	perfCounters.syntheticReplayGapLexAttempts.Store(0)
	perfCounters.syntheticReplayGapLexCacheHits.Store(0)
	perfCounters.syntheticReplayGapLexCacheMisses.Store(0)
	perfCounters.syntheticReplayGapLexCacheStores.Store(0)
	perfCounters.syntheticReplayGapLexCacheCapSkips.Store(0)
	perfCounters.syntheticReplayAdvanceAttempts.Store(0)
	perfCounters.syntheticReplayAdvanceCacheHits.Store(0)
	perfCounters.syntheticReplayAdvanceCacheMisses.Store(0)
	perfCounters.syntheticReplayAdvanceCacheStores.Store(0)
	perfCounters.syntheticReplayAdvanceCacheCapSkips.Store(0)
	perfCounters.syntheticReplayAdvanceZeroOutputs.Store(0)
	perfCounters.syntheticReplayAdvanceSingleOutputs.Store(0)
	perfCounters.syntheticReplayAdvanceMultiOutputs.Store(0)
	perfCounters.syntheticReplayAdvanceOutputPeak.Store(0)
	perfCounters.syntheticReplayAdvanceOutputFrames.Store(0)
	perfCounters.syntheticReplayAdvanceUniqueOutputs.Store(0)
	perfCounters.syntheticReplayAdvanceUniqueFrames.Store(0)
	perfCounters.syntheticReplayAdvanceUniqueSingle.Store(0)
	perfCounters.syntheticReplayAdvanceUniqueMulti.Store(0)
	perfCounters.syntheticReplayAdvanceUniquePeak.Store(0)
	perfCounters.syntheticReplayStackPushCalls.Store(0)
	perfCounters.syntheticReplayStackInternHits.Store(0)
	perfCounters.syntheticReplayCloseMemoHits.Store(0)
	perfCounters.syntheticReplayCloseMemoMisses.Store(0)
	perfCounters.syntheticReplayCloseMemoStores.Store(0)
	perfCounters.syntheticReplayCloseMemoCapSkips.Store(0)
	perfCounters.syntheticReplayCloseZeroOutputs.Store(0)
	perfCounters.syntheticReplayCloseSingleOutputs.Store(0)
	perfCounters.syntheticReplayCloseMultiOutputs.Store(0)
	perfCounters.syntheticReplayCloseOutputPeak.Store(0)
	perfCounters.syntheticReplayCloseOutputFrames.Store(0)
	perfCounters.syntheticReplayCloseUniqueOutputs.Store(0)
	perfCounters.syntheticReplayCloseUniqueFrames.Store(0)
	perfCounters.syntheticReplayCloseUniqueSingle.Store(0)
	perfCounters.syntheticReplayCloseUniqueMulti.Store(0)
	perfCounters.syntheticReplayCloseUniquePeak.Store(0)
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
	out.ProbeLexBytes = perfCounters.probeLexBytes.Load()
	out.ProbeLexTokens = perfCounters.probeLexTokens.Load()
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
	out.ReduceChildrenScratch = perfCounters.reduceChildrenScratch.Load()
	out.ReduceScratchNoAlias = perfCounters.reduceScratchNoAlias.Load()
	out.ReduceScratchGeneral = perfCounters.reduceScratchGeneral.Load()
	out.ReduceChildEmptyBuilds = perfCounters.reduceChildEmptyBuilds.Load()
	out.ReduceChildAllVisibleBuilds = perfCounters.reduceChildAllVisibleBuilds.Load()
	out.ReduceChildScratchNoAliasBuilds = perfCounters.reduceChildScratchNoAliasBuilds.Load()
	out.ReduceChildScratchGeneralBuilds = perfCounters.reduceChildScratchGeneralBuilds.Load()
	out.ReduceForkCalls = perfCounters.reduceForkCalls.Load()
	out.ReduceForkWindows = perfCounters.reduceForkWindows.Load()
	out.ReduceForkMaxWindows = perfCounters.reduceForkMaxWindows.Load()
	out.PostReduceMergeAttempts = perfCounters.postReduceMergeAttempts.Load()
	out.PostReduceMergePrimarySuccesses = perfCounters.postReduceMergePrimarySuccesses.Load()
	out.PostReduceMergePendingSuccesses = perfCounters.postReduceMergePendingSuccesses.Load()
	out.PostReduceMergeDisabledSkips = perfCounters.postReduceMergeDisabledSkips.Load()
	out.PostReduceMergeFinalizationRiskSkips = perfCounters.postReduceMergeFinalizationRiskSkips.Load()
	out.PendingForkStackAppends = perfCounters.pendingForkStackAppends.Load()
	out.PendingForkStacksMaxLen = perfCounters.pendingForkStacksMaxLen.Load()
	out.ForestReduceCalls = perfCounters.forestReduceCalls.Load()
	out.ForestReduceZero = perfCounters.forestReduceZero.Load()
	out.ForestReduceLinearNoExtras = perfCounters.forestReduceLinearNoExtras.Load()
	out.ForestReduceDFS = perfCounters.forestReduceDFS.Load()
	out.ForestReduceDFSLinks = perfCounters.forestReduceDFSLinks.Load()
	out.ForestReduceDFSMultiLinkSteps = perfCounters.forestReduceDFSMultiLinkSteps.Load()
	out.ForestReduceDFSExtraLinks = perfCounters.forestReduceDFSExtraLinks.Load()
	out.ForestReduceDFSVisits = perfCounters.forestReduceDFSVisits.Load()
	out.ForestReduceDFSPathEntries = perfCounters.forestReduceDFSPathEntries.Load()
	out.ForestReduceGotoHits = perfCounters.forestReduceGotoHits.Load()
	out.ForestReduceGotoMisses = perfCounters.forestReduceGotoMisses.Load()
	out.ForestReduceMaxPathLen = perfCounters.forestReduceMaxPathLen.Load()
	out.ForestReduceMaxChildCount = perfCounters.forestReduceMaxChildCount.Load()
	out.ForestCoalesceCalls = perfCounters.forestCoalesceCalls.Load()
	out.ForestCoalesceNewNodes = perfCounters.forestCoalesceNewNodes.Load()
	out.ForestCoalesceLinkAppends = perfCounters.forestCoalesceLinkAppends.Load()
	out.ForestCoalesceDedupHits = perfCounters.forestCoalesceDedupHits.Load()
	out.ForestCoalesceDedupReplacements = perfCounters.forestCoalesceDedupReplacements.Load()
	out.ForestCoalescePreCapDrops = perfCounters.forestCoalescePreCapDrops.Load()
	out.ForestCoalesceCapDrops = perfCounters.forestCoalesceCapDrops.Load()
	out.ForestCoalesceCapReplacements = perfCounters.forestCoalesceCapReplacements.Load()
	out.ExtraNodes = perfCounters.extraNodes.Load()
	out.ErrorNodes = perfCounters.errorNodes.Load()
	out.SyntheticReplayGapBridgeCalls = perfCounters.syntheticReplayGapBridgeCalls.Load()
	out.SyntheticReplayGapSteps = perfCounters.syntheticReplayGapSteps.Load()
	out.SyntheticReplayGapCursorAttempts = perfCounters.syntheticReplayGapCursorAttempts.Load()
	out.SyntheticReplayGapCursorDedupHits = perfCounters.syntheticReplayGapCursorDedupHits.Load()
	out.SyntheticReplayGapCursorPeak = perfCounters.syntheticReplayGapCursorPeak.Load()
	out.SyntheticReplayGapLexAttempts = perfCounters.syntheticReplayGapLexAttempts.Load()
	out.SyntheticReplayGapLexCacheHits = perfCounters.syntheticReplayGapLexCacheHits.Load()
	out.SyntheticReplayGapLexCacheMisses = perfCounters.syntheticReplayGapLexCacheMisses.Load()
	out.SyntheticReplayGapLexCacheStores = perfCounters.syntheticReplayGapLexCacheStores.Load()
	out.SyntheticReplayGapLexCacheCapSkips = perfCounters.syntheticReplayGapLexCacheCapSkips.Load()
	out.SyntheticReplayAdvanceAttempts = perfCounters.syntheticReplayAdvanceAttempts.Load()
	out.SyntheticReplayAdvanceCacheHits = perfCounters.syntheticReplayAdvanceCacheHits.Load()
	out.SyntheticReplayAdvanceCacheMisses = perfCounters.syntheticReplayAdvanceCacheMisses.Load()
	out.SyntheticReplayAdvanceCacheStores = perfCounters.syntheticReplayAdvanceCacheStores.Load()
	out.SyntheticReplayAdvanceCacheCapSkips = perfCounters.syntheticReplayAdvanceCacheCapSkips.Load()
	out.SyntheticReplayAdvanceZeroOutputs = perfCounters.syntheticReplayAdvanceZeroOutputs.Load()
	out.SyntheticReplayAdvanceSingleOutputs = perfCounters.syntheticReplayAdvanceSingleOutputs.Load()
	out.SyntheticReplayAdvanceMultiOutputs = perfCounters.syntheticReplayAdvanceMultiOutputs.Load()
	out.SyntheticReplayAdvanceOutputPeak = perfCounters.syntheticReplayAdvanceOutputPeak.Load()
	out.SyntheticReplayAdvanceOutputFrames = perfCounters.syntheticReplayAdvanceOutputFrames.Load()
	out.SyntheticReplayAdvanceUniqueOutputs = perfCounters.syntheticReplayAdvanceUniqueOutputs.Load()
	out.SyntheticReplayAdvanceUniqueFrames = perfCounters.syntheticReplayAdvanceUniqueFrames.Load()
	out.SyntheticReplayAdvanceUniqueSingle = perfCounters.syntheticReplayAdvanceUniqueSingle.Load()
	out.SyntheticReplayAdvanceUniqueMulti = perfCounters.syntheticReplayAdvanceUniqueMulti.Load()
	out.SyntheticReplayAdvanceUniquePeak = perfCounters.syntheticReplayAdvanceUniquePeak.Load()
	out.SyntheticReplayStackPushCalls = perfCounters.syntheticReplayStackPushCalls.Load()
	out.SyntheticReplayStackInternHits = perfCounters.syntheticReplayStackInternHits.Load()
	out.SyntheticReplayCloseMemoHits = perfCounters.syntheticReplayCloseMemoHits.Load()
	out.SyntheticReplayCloseMemoMisses = perfCounters.syntheticReplayCloseMemoMisses.Load()
	out.SyntheticReplayCloseMemoStores = perfCounters.syntheticReplayCloseMemoStores.Load()
	out.SyntheticReplayCloseMemoCapSkips = perfCounters.syntheticReplayCloseMemoCapSkips.Load()
	out.SyntheticReplayCloseZeroOutputs = perfCounters.syntheticReplayCloseZeroOutputs.Load()
	out.SyntheticReplayCloseSingleOutputs = perfCounters.syntheticReplayCloseSingleOutputs.Load()
	out.SyntheticReplayCloseMultiOutputs = perfCounters.syntheticReplayCloseMultiOutputs.Load()
	out.SyntheticReplayCloseOutputPeak = perfCounters.syntheticReplayCloseOutputPeak.Load()
	out.SyntheticReplayCloseOutputFrames = perfCounters.syntheticReplayCloseOutputFrames.Load()
	out.SyntheticReplayCloseUniqueOutputs = perfCounters.syntheticReplayCloseUniqueOutputs.Load()
	out.SyntheticReplayCloseUniqueFrames = perfCounters.syntheticReplayCloseUniqueFrames.Load()
	out.SyntheticReplayCloseUniqueSingle = perfCounters.syntheticReplayCloseUniqueSingle.Load()
	out.SyntheticReplayCloseUniqueMulti = perfCounters.syntheticReplayCloseUniqueMulti.Load()
	out.SyntheticReplayCloseUniquePeak = perfCounters.syntheticReplayCloseUniquePeak.Load()
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

// perfRecordProbeLexed records bytes and tokens the compact EOF scanner
// quiescence probe lexes, kept apart from perfRecordLexed's parse-only
// counters (finding F8).
func perfRecordProbeLexed(bytes, tokens int) {
	if bytes > 0 {
		perfCounters.probeLexBytes.Add(uint64(bytes))
	}
	if tokens > 0 {
		perfCounters.probeLexTokens.Add(uint64(tokens))
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

func perfRecordReduceChildBuild(path reduceChildPath) {
	switch path {
	case reduceChildPathNone:
		perfCounters.reduceChildEmptyBuilds.Add(1)
	case reduceChildPathAllVisible:
		perfCounters.reduceChildAllVisibleBuilds.Add(1)
	case reduceChildPathScratchNoAlias:
		perfCounters.reduceChildScratchNoAliasBuilds.Add(1)
	case reduceChildPathScratchGeneral:
		perfCounters.reduceChildScratchGeneralBuilds.Add(1)
	}
}

func perfRecordReduceForkCall(windowCount int) {
	perfCounters.reduceForkCalls.Add(1)
	if windowCount > 0 {
		perfCounters.reduceForkWindows.Add(uint64(windowCount))
		perfMaxUint64(&perfCounters.reduceForkMaxWindows, uint64(windowCount))
	}
}

func perfRecordPostReduceMergeAttempt() {
	perfCounters.postReduceMergeAttempts.Add(1)
}

func perfRecordPostReduceMergePrimarySuccess() {
	perfCounters.postReduceMergePrimarySuccesses.Add(1)
}

func perfRecordPostReduceMergePendingSuccess() {
	perfCounters.postReduceMergePendingSuccesses.Add(1)
}

func perfRecordPostReduceMergeDisabledSkip() {
	perfCounters.postReduceMergeDisabledSkips.Add(1)
}

func perfRecordPostReduceMergeFinalizationRiskSkip() {
	perfCounters.postReduceMergeFinalizationRiskSkips.Add(1)
}

func perfRecordPendingForkStackAppend(length int) {
	perfCounters.pendingForkStackAppends.Add(1)
	if length > 0 {
		perfMaxUint64(&perfCounters.pendingForkStacksMaxLen, uint64(length))
	}
}

func perfRecordForestReduceCall(childCount int) {
	perfCounters.forestReduceCalls.Add(1)
	perfMaxUint64(&perfCounters.forestReduceMaxChildCount, uint64(childCount))
}

func perfRecordForestReduceZero() {
	perfCounters.forestReduceZero.Add(1)
}

func perfRecordForestReduceLinearNoExtras(childCount int) {
	perfCounters.forestReduceLinearNoExtras.Add(1)
	perfMaxUint64(&perfCounters.forestReduceMaxPathLen, uint64(childCount))
}

func perfRecordForestReduceDFS() {
	perfCounters.forestReduceDFS.Add(1)
}

func perfRecordForestReduceDFSStep(linkCount int, extra bool) {
	perfCounters.forestReduceDFSLinks.Add(1)
	if linkCount > 1 {
		perfCounters.forestReduceDFSMultiLinkSteps.Add(1)
	}
	if extra {
		perfCounters.forestReduceDFSExtraLinks.Add(1)
	}
}

func perfRecordForestReduceDFSVisit(pathLen int) {
	perfCounters.forestReduceDFSVisits.Add(1)
	if pathLen > 0 {
		perfCounters.forestReduceDFSPathEntries.Add(uint64(pathLen))
		perfMaxUint64(&perfCounters.forestReduceMaxPathLen, uint64(pathLen))
	}
}

func perfRecordForestReduceGotoHit() {
	perfCounters.forestReduceGotoHits.Add(1)
}

func perfRecordForestReduceGotoMiss() {
	perfCounters.forestReduceGotoMisses.Add(1)
}

func perfRecordForestCoalesceCall() {
	perfCounters.forestCoalesceCalls.Add(1)
}

func perfRecordForestCoalesceNewNode() {
	perfCounters.forestCoalesceNewNodes.Add(1)
}

func perfRecordForestCoalesceLinkAppend() {
	perfCounters.forestCoalesceLinkAppends.Add(1)
}

func perfRecordForestCoalesceDedupHit(replaced bool) {
	perfCounters.forestCoalesceDedupHits.Add(1)
	if replaced {
		perfCounters.forestCoalesceDedupReplacements.Add(1)
	}
}

func perfRecordForestCoalescePreCapDrop() {
	perfCounters.forestCoalescePreCapDrops.Add(1)
}

func perfRecordForestCoalesceCap(replaced bool) {
	if replaced {
		perfCounters.forestCoalesceCapReplacements.Add(1)
	} else {
		perfCounters.forestCoalesceCapDrops.Add(1)
	}
}

func perfRecordExtraNode() {
	perfCounters.extraNodes.Add(1)
}

func perfRecordErrorNode() {
	perfCounters.errorNodes.Add(1)
}

func perfRecordSyntheticReplayGapBridge() {
	perfCounters.syntheticReplayGapBridgeCalls.Add(1)
}

func perfRecordSyntheticReplayGapStep() {
	perfCounters.syntheticReplayGapSteps.Add(1)
}

func perfRecordSyntheticReplayGapCursorAttempt() {
	perfCounters.syntheticReplayGapCursorAttempts.Add(1)
}

func perfRecordSyntheticReplayGapCursorDedup() {
	perfCounters.syntheticReplayGapCursorDedupHits.Add(1)
}

func perfRecordSyntheticReplayGapCursorPeak(n int) {
	if n > 0 {
		perfMaxUint64(&perfCounters.syntheticReplayGapCursorPeak, uint64(n))
	}
}

func perfRecordSyntheticReplayGapLexAttempt() {
	perfCounters.syntheticReplayGapLexAttempts.Add(1)
}

func perfRecordSyntheticReplayGapLexCacheHit() {
	perfCounters.syntheticReplayGapLexCacheHits.Add(1)
}

func perfRecordSyntheticReplayGapLexCacheMiss() {
	perfCounters.syntheticReplayGapLexCacheMisses.Add(1)
}

func perfRecordSyntheticReplayGapLexCacheStore() {
	perfCounters.syntheticReplayGapLexCacheStores.Add(1)
}

func perfRecordSyntheticReplayGapLexCacheCapSkip() {
	perfCounters.syntheticReplayGapLexCacheCapSkips.Add(1)
}

func perfRecordSyntheticReplayAdvanceAttempt() {
	perfCounters.syntheticReplayAdvanceAttempts.Add(1)
}

func perfRecordSyntheticReplayAdvanceCacheHit() {
	perfCounters.syntheticReplayAdvanceCacheHits.Add(1)
}

func perfRecordSyntheticReplayAdvanceCacheMiss() {
	perfCounters.syntheticReplayAdvanceCacheMisses.Add(1)
}

func perfRecordSyntheticReplayAdvanceCacheStore() {
	perfCounters.syntheticReplayAdvanceCacheStores.Add(1)
}

func perfRecordSyntheticReplayAdvanceCacheCapSkip() {
	perfCounters.syntheticReplayAdvanceCacheCapSkips.Add(1)
}

func perfRecordSyntheticReplayAdvanceOutput(n int, unique bool) {
	switch n {
	case 0:
		perfCounters.syntheticReplayAdvanceZeroOutputs.Add(1)
	case 1:
		perfCounters.syntheticReplayAdvanceSingleOutputs.Add(1)
	default:
		perfCounters.syntheticReplayAdvanceMultiOutputs.Add(1)
	}
	if n > 0 {
		perfCounters.syntheticReplayAdvanceOutputFrames.Add(uint64(n))
		perfMaxUint64(&perfCounters.syntheticReplayAdvanceOutputPeak, uint64(n))
	}
	if !unique {
		return
	}
	perfCounters.syntheticReplayAdvanceUniqueOutputs.Add(1)
	perfCounters.syntheticReplayAdvanceUniqueFrames.Add(uint64(n))
	if n == 1 {
		perfCounters.syntheticReplayAdvanceUniqueSingle.Add(1)
	} else if n > 1 {
		perfCounters.syntheticReplayAdvanceUniqueMulti.Add(1)
	}
	if n > 0 {
		perfMaxUint64(&perfCounters.syntheticReplayAdvanceUniquePeak, uint64(n))
	}
}

func perfRecordSyntheticReplayStackPush() {
	perfCounters.syntheticReplayStackPushCalls.Add(1)
}

func perfRecordSyntheticReplayStackInternHit() {
	perfCounters.syntheticReplayStackInternHits.Add(1)
}

func perfRecordSyntheticReplayCloseMemoHit() {
	perfCounters.syntheticReplayCloseMemoHits.Add(1)
}

func perfRecordSyntheticReplayCloseMemoMiss() {
	perfCounters.syntheticReplayCloseMemoMisses.Add(1)
}

func perfRecordSyntheticReplayCloseMemoStore() {
	perfCounters.syntheticReplayCloseMemoStores.Add(1)
}

func perfRecordSyntheticReplayCloseMemoCapSkip() {
	perfCounters.syntheticReplayCloseMemoCapSkips.Add(1)
}

func perfRecordSyntheticReplayCloseOutput(n int, unique bool) {
	switch n {
	case 0:
		perfCounters.syntheticReplayCloseZeroOutputs.Add(1)
	case 1:
		perfCounters.syntheticReplayCloseSingleOutputs.Add(1)
	default:
		perfCounters.syntheticReplayCloseMultiOutputs.Add(1)
	}
	if n > 0 {
		perfCounters.syntheticReplayCloseOutputFrames.Add(uint64(n))
		perfMaxUint64(&perfCounters.syntheticReplayCloseOutputPeak, uint64(n))
	}
	if !unique {
		return
	}
	perfCounters.syntheticReplayCloseUniqueOutputs.Add(1)
	perfCounters.syntheticReplayCloseUniqueFrames.Add(uint64(n))
	if n == 1 {
		perfCounters.syntheticReplayCloseUniqueSingle.Add(1)
	} else if n > 1 {
		perfCounters.syntheticReplayCloseUniqueMulti.Add(1)
	}
	if n > 0 {
		perfMaxUint64(&perfCounters.syntheticReplayCloseUniquePeak, uint64(n))
	}
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

func perfMaxUint64(slot *atomic.Uint64, target uint64) {
	for {
		prev := slot.Load()
		if target <= prev {
			return
		}
		if slot.CompareAndSwap(prev, target) {
			return
		}
	}
}
