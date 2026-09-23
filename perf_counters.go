package gotreesitter

type PerfCounters struct {
	MergeCalls           uint64
	MergeDeadPruned      uint64
	MergePerKeyOverflow  uint64
	MergeReplacements    uint64
	StackEquivalentCalls uint64
	StackEquivalentTrue  uint64
	StackEqHashMissSkips uint64
	StackCompareCalls    uint64
	ConflictRR           uint64
	ConflictRS           uint64
	ConflictOther        uint64
	ForkCount            uint64
	FirstConflictToken   uint64
	MaxConcurrentStacks  uint64
	// GSSCanReachVisits counts gssNode visits inside gssNodeCanReach's DFS
	// (glr.go), across every call in the parse. A fixed nesting depth that
	// forks and merges on every token should keep this roughly linear in
	// token count once depth-based pruning is in effect; before the prune it
	// grows with the stack depth on every call, i.e. superlinearly.
	GSSCanReachVisits uint64
	// ShapePrefixWalkSteps counts the GSS nodes gssMaterializingShapePrefix
	// (glr.go) had to hash because no cached prefix covered them, across the
	// parse. A fixed nesting depth that forks and merges on every token
	// should keep this roughly linear in token count; when every successful
	// merge invalidated the whole cache it grew with the spine depth on every
	// head hash, i.e. superlinearly (issue #454).
	ShapePrefixWalkSteps uint64
	// ShapePrefixEpochBumps counts full shape-prefix cache invalidations
	// (glrMergeScratch.bumpShapePrefixEpoch) across the parse.
	ShapePrefixEpochBumps uint64
	LexBytes              uint64
	LexTokens             uint64
	// ProbeLexBytes and ProbeLexTokens count the compact EOF scanner
	// quiescence probe's own lexing (proveCompactEOFScannerQuiescence,
	// parsercore_phase0_eof_scanner_quiescence.go), kept apart from
	// LexBytes/LexTokens, which name the parse's own token stream only.
	ProbeLexBytes                        uint64
	ProbeLexTokens                       uint64
	ReuseNodesVisited                    uint64
	ReuseNodesPushed                     uint64
	ReuseNodesPopped                     uint64
	ReuseCandidatesChecked               uint64
	ReuseSuccesses                       uint64
	ReuseLeafSuccesses                   uint64
	ReuseNonLeafChecks                   uint64
	ReuseNonLeafSuccesses                uint64
	ReuseNonLeafBytes                    uint64
	ReuseNonLeafNoGoto                   uint64
	ReuseNonLeafNoGotoTerm               uint64
	ReuseNonLeafNoGotoNt                 uint64
	ReuseNonLeafStateMiss                uint64
	ReuseNonLeafStateZero                uint64
	MergeHashZero                        uint64
	GlobalCapCulls                       uint64
	GlobalCapCullDropped                 uint64
	ReduceChainSteps                     uint64
	ReduceChainMaxLen                    uint64
	ReduceChainBreakMulti                uint64
	ReduceChainBreakShift                uint64
	ReduceChainBreakAccept               uint64
	ReduceChainHintCandidates            uint64
	ReduceChainHintTaken                 uint64
	ReduceChainHintSteps                 uint64
	ReduceChainHintTerminalOK            uint64
	ReduceChainHintTerminalMismatch      uint64
	ReduceChainHintLimit                 uint64
	ReduceChainHintDead                  uint64
	ReduceChainHintUnexpected            uint64
	ParentChildPointers                  uint64
	ReduceChildrenFastGSS                uint64
	ReduceChildrenAllVis                 uint64
	ReduceChildrenScratch                uint64
	ReduceScratchNoAlias                 uint64
	ReduceScratchGeneral                 uint64
	ReduceChildEmptyBuilds               uint64
	ReduceChildAllVisibleBuilds          uint64
	ReduceChildScratchNoAliasBuilds      uint64
	ReduceChildScratchGeneralBuilds      uint64
	ReduceForkCalls                      uint64
	ReduceForkWindows                    uint64
	ReduceForkMaxWindows                 uint64
	PostReduceMergeAttempts              uint64
	PostReduceMergePrimarySuccesses      uint64
	PostReduceMergePendingSuccesses      uint64
	PostReduceMergeDisabledSkips         uint64
	PostReduceMergeFinalizationRiskSkips uint64
	PendingForkStackAppends              uint64
	PendingForkStacksMaxLen              uint64
	ForestReduceCalls                    uint64
	ForestReduceZero                     uint64
	ForestReduceLinearNoExtras           uint64
	ForestReduceDFS                      uint64
	ForestReduceDFSLinks                 uint64
	ForestReduceDFSMultiLinkSteps        uint64
	ForestReduceDFSExtraLinks            uint64
	ForestReduceDFSVisits                uint64
	ForestReduceDFSPathEntries           uint64
	ForestReduceGotoHits                 uint64
	ForestReduceGotoMisses               uint64
	ForestReduceMaxPathLen               uint64
	ForestReduceMaxChildCount            uint64
	ForestCoalesceCalls                  uint64
	ForestCoalesceNewNodes               uint64
	ForestCoalesceLinkAppends            uint64
	ForestCoalesceDedupHits              uint64
	ForestCoalesceDedupReplacements      uint64
	ForestCoalescePreCapDrops            uint64
	ForestCoalesceCapDrops               uint64
	ForestCoalesceCapReplacements        uint64
	ExtraNodes                           uint64
	ErrorNodes                           uint64
	SyntheticReplayGapBridgeCalls        uint64
	SyntheticReplayGapSteps              uint64
	SyntheticReplayGapCursorAttempts     uint64
	SyntheticReplayGapCursorDedupHits    uint64
	SyntheticReplayGapCursorPeak         uint64
	SyntheticReplayGapLexAttempts        uint64
	SyntheticReplayGapLexCacheHits       uint64
	SyntheticReplayGapLexCacheMisses     uint64
	SyntheticReplayGapLexCacheStores     uint64
	SyntheticReplayGapLexCacheCapSkips   uint64
	SyntheticReplayAdvanceAttempts       uint64
	SyntheticReplayAdvanceCacheHits      uint64
	SyntheticReplayAdvanceCacheMisses    uint64
	SyntheticReplayAdvanceCacheStores    uint64
	SyntheticReplayAdvanceCacheCapSkips  uint64
	SyntheticReplayAdvanceZeroOutputs    uint64
	SyntheticReplayAdvanceSingleOutputs  uint64
	SyntheticReplayAdvanceMultiOutputs   uint64
	SyntheticReplayAdvanceOutputPeak     uint64
	SyntheticReplayAdvanceOutputFrames   uint64
	SyntheticReplayAdvanceUniqueOutputs  uint64
	SyntheticReplayAdvanceUniqueFrames   uint64
	SyntheticReplayAdvanceUniqueSingle   uint64
	SyntheticReplayAdvanceUniqueMulti    uint64
	SyntheticReplayAdvanceUniquePeak     uint64
	SyntheticReplayStackPushCalls        uint64
	SyntheticReplayStackInternHits       uint64
	SyntheticReplayCloseMemoHits         uint64
	SyntheticReplayCloseMemoMisses       uint64
	SyntheticReplayCloseMemoStores       uint64
	SyntheticReplayCloseMemoCapSkips     uint64
	SyntheticReplayCloseZeroOutputs      uint64
	SyntheticReplayCloseSingleOutputs    uint64
	SyntheticReplayCloseMultiOutputs     uint64
	SyntheticReplayCloseOutputPeak       uint64
	SyntheticReplayCloseOutputFrames     uint64
	SyntheticReplayCloseUniqueOutputs    uint64
	SyntheticReplayCloseUniqueFrames     uint64
	SyntheticReplayCloseUniqueSingle     uint64
	SyntheticReplayCloseUniqueMulti      uint64
	SyntheticReplayCloseUniquePeak       uint64
	MergeStacksInHist                    [maxGLRStacks + 2]uint64
	MergeAliveHist                       [maxGLRStacks + 2]uint64
	MergeOutHist                         [maxGLRStacks + 2]uint64
	ForkActionsHist                      [8]uint64
	CloneTreeCalls                       uint64
	CloneTreePublicNodes                 uint64
	CloneTreeFinalRefs                   uint64
	CloneTreeCompactCopies               uint64
	CloneTreeChildRefs                   uint64
	CloneOffsetCalls                     uint64
	CloneOffsetPublicNodes               uint64
	CloneOffsetCopies                    uint64
	CloneOffsetShifted                   uint64
	NodeEditCalls                        uint64
	NodeEditNoopCalls                    uint64
	NodeEditCompactRefs                  uint64
	NodeEditShifted                      uint64
	NodeEditMarked                       uint64
	DenseMutationCalls                   uint64
	DenseMutationDrains                  uint64
	MutationChildRefCOW                  uint64
}
