//go:build !perf

package gotreesitter

const perfCountersEnabled = false

func ResetPerfCounters()                 {}
func PerfCountersSnapshot() PerfCounters { return PerfCounters{} }

func perfRecordMergeCall(int)                        {}
func perfRecordMergeAlive(int, int)                  {}
func perfRecordMergeOut(int)                         {}
func perfRecordMergeHashZero()                       {}
func perfRecordGlobalCapCull(int, int)               {}
func perfRecordMergePerKeyOverflow()                 {}
func perfRecordMergeReplacement()                    {}
func perfRecordStackEquivalentCall()                 {}
func perfRecordStackEquivalentTrue()                 {}
func perfRecordStackEquivalentHashMissSkip()         {}
func perfRecordStackCompare()                        {}
func perfRecordConflictRR()                          {}
func perfRecordConflictRS()                          {}
func perfRecordConflictOther()                       {}
func perfRecordFork(int, uint64)                     {}
func perfRecordMaxConcurrentStacks(int)              {}
func perfRecordLexed(int, int)                       {}
func perfRecordReuseVisited()                        {}
func perfRecordReusePushed(int)                      {}
func perfRecordReusePopped()                         {}
func perfRecordReuseCandidates(int)                  {}
func perfRecordReuseSuccess()                        {}
func perfRecordReuseLeafSuccess()                    {}
func perfRecordReuseNonLeafCheck()                   {}
func perfRecordReuseNonLeafSuccess(uint32)           {}
func perfRecordReuseNonLeafNoGoto()                  {}
func perfRecordReuseNonLeafNoGotoTerminal()          {}
func perfRecordReuseNonLeafNoGotoNonTerminal()       {}
func perfRecordReuseNonLeafStateMiss()               {}
func perfRecordReuseNonLeafStateZero()               {}
func perfRecordReduceChainStep(int)                  {}
func perfRecordReduceChainBreakMulti()               {}
func perfRecordReduceChainBreakShift()               {}
func perfRecordReduceChainBreakAccept()              {}
func perfRecordReduceChainHintCandidate()            {}
func perfRecordReduceChainHintTaken()                {}
func perfRecordReduceChainHintSteps(int)             {}
func perfRecordReduceChainHintTerminalOK()           {}
func perfRecordReduceChainHintTerminalMismatch()     {}
func perfRecordReduceChainHintLimit()                {}
func perfRecordReduceChainHintDead()                 {}
func perfRecordReduceChainHintUnexpected()           {}
func perfRecordParentChildren(int)                   {}
func perfRecordReduceChildrenFastGSS(int)            {}
func perfRecordReduceChildrenAllVisible(int)         {}
func perfRecordReduceChildrenScratch(int)            {}
func perfRecordReduceScratchNoAlias(int)             {}
func perfRecordReduceScratchGeneral(int)             {}
func perfRecordReduceForkCall(int)                   {}
func perfRecordPostReduceMergeAttempt()              {}
func perfRecordPostReduceMergePrimarySuccess()       {}
func perfRecordPostReduceMergePendingSuccess()       {}
func perfRecordPostReduceMergeDisabledSkip()         {}
func perfRecordPostReduceMergeFinalizationRiskSkip() {}
func perfRecordPendingForkStackAppend(int)           {}
func perfRecordForestReduceCall(int)                 {}
func perfRecordForestReduceZero()                    {}
func perfRecordForestReduceLinearNoExtras(int)       {}
func perfRecordForestReduceDFS()                     {}
func perfRecordForestReduceDFSStep(int, bool)        {}
func perfRecordForestReduceDFSVisit(int)             {}
func perfRecordForestReduceGotoHit()                 {}
func perfRecordForestReduceGotoMiss()                {}
func perfRecordForestCoalesceCall()                  {}
func perfRecordForestCoalesceNewNode()               {}
func perfRecordForestCoalesceLinkAppend()            {}
func perfRecordForestCoalesceDedupHit(bool)          {}
func perfRecordForestCoalescePreCapDrop()            {}
func perfRecordForestCoalesceCap(bool)               {}
func perfRecordExtraNode()                           {}
func perfRecordErrorNode()                           {}
func perfRecordCloneTreeCall()                       {}
func perfRecordCloneTreePublicNode()                 {}
func perfRecordCloneTreeFinalRefs(int)               {}
func perfRecordCloneTreeCompactCopy()                {}
func perfRecordCloneTreeChildRefs(int)               {}
func perfRecordCloneOffsetCall()                     {}
func perfRecordCloneOffsetPublicNode()               {}
func perfRecordCloneOffsetCompactCopy()              {}
func perfRecordCloneOffsetShifted()                  {}
func perfRecordNodeEditCall()                        {}
func perfRecordNodeEditNoopCall()                    {}
func perfRecordNodeEditCompactRef()                  {}
func perfRecordNodeEditShifted()                     {}
func perfRecordNodeEditMarked()                      {}
func perfRecordDenseMutationChildrenCall()           {}
func perfRecordDenseMutationChildrenDrain()          {}
func perfRecordMutationChildRefCopyOnWrite(int)      {}
