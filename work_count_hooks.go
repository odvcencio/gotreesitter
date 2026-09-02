//go:build !gts_workcount

package gotreesitter

type workCountAttemptToken uint32
type diagnosticTopologyStackToken struct{}

const workCountInstrumentationEnabled = false
const diagnosticTopologyStackOverhead = uintptr(0)

func workCountAttemptTraceActive() bool { return false }

// These hooks are compile-time instrumentation seams. The production build
// supplies empty, inlineable bodies; the gts_workcount build replaces them
// with parse-local saturating counters.
func workCountSetNextParseAttempt(string, string)                                       {}
func workCountBeginParseAttempt(int, int, int) workCountAttemptToken                    { return 0 }
func workCountResolveParseAttempt(workCountAttemptToken, int, bool, int, int, int, int) {}
func workCountBeginFinalizeParseAttempt(workCountAttemptToken)                          {}
func workCountEndFinalizeParseAttempt(workCountAttemptToken, ParseStopReason, *Tree)    {}
func workCountRecordLexerFrontDoor()                                                    {}
func workCountRecordFrontierLexerElection()                                             {}
func workCountRecordRawMainLexerInvocation()                                            {}
func workCountRecordTableLookup()                                                       {}
func workCountRecordResolvedActionCell(int)                                             {}
func workCountRecordAlternatePredecessorLinkAppended()                                  {}
func workCountAddActionEntries(int)                                                     {}
func workCountRecordShift()                                                             {}
func workCountRecordReduce()                                                            {}
func workCountRecordAccept()                                                            {}
func workCountRecordExplicitRecover()                                                   {}
func workCountObserveReductionPop(*glrStack, int)                                       {}
func workCountTopologyRecordNoActionPendingPop()                                        {}
func workCountAddPopPaths(uint64, uint64)                                               {}
func workCountObservePopWindow([]stackEntry)                                            {}
func workCountRecordVersionCreation()                                                   {}
func workCountRecordMergeAttempt()                                                      {}
func workCountRecordMergeSuccess()                                                      {}
func workCountRecordGraphLinkAddition()                                                 {}
func workCountRecordLeafConstruction()                                                  {}
func workCountRecordParentConstruction()                                                {}
func workCountRecordPendingParentConstruction()                                         {}
func workCountRecordNoTreeParentConstruction()                                          {}
func workCountSetConvergenceIteration(int)                                              {}
func workCountConvergenceActive() bool                                                  { return false }
func workCountSetConvergenceLookahead(Token)                                            {}
func workCountRefreshConvergenceLookahead(Token)                                        {}

func semanticPhaseTraceRecordActionCell(*Parser, *glrStack, StateID, Token, []ParseAction) {}
func semanticPhaseTraceActive() bool                                                       { return false }
func semanticPhaseTraceRecordActionExecution(*Parser, *glrStack, Token, ParseAction, int, string, bool) {
}

func workCountConvergenceShapeOf(*glrStack) workCountConvergenceShape {
	return workCountConvergenceShape{}
}
func workCountRecordConvergence(*Parser, string, string, string, string, *glrStack, *glrStack, int, int, workCountConvergenceShape, workCountConvergenceShape, bool, bool) {
}
func workCountConvergencePackedResultExpansion(*glrStack, int) (int, bool, bool) {
	return 0, false, false
}
func workCountRecordPairCandidate(*Parser, string, string, *glrStack, *glrStack) {}
func workCountRecordGSSReject(*Parser, string, string, string, *glrStack, *glrStack) {
}
func workCountRecordGSSScoreShiftReject(*Parser, string, *glrStack, *glrStack)  {}
func workCountRecordGSSCleanReject(*Parser, string, *glrStack, *glrStack, bool) {}
func workCountGSSMutationEpoch() uint64                                         { return 0 }
func workCountRecordGSSMutation()                                               {}
func workCountMergeGSSObserved(_ *Parser, scratch *glrMergeScratch, _ string, _ string, target, candidate *glrStack) bool {
	return gssMainMergeWithScratch(scratch, target, candidate)
}
func workCountRecordPostReduceCandidate(*Parser, string, string, *glrStack, *glrStack, string) {
}
func workCountRecordPendingQueued(*Parser, *glrStack, *glrStack, int, int, string)  {}
func workCountRecordAcceptedHead(*Parser, *glrStack, string)                        {}
func workCountRecordReduceWindow(*Parser, *glrStack, int, int, int, bool)           {}
func workCountRecordPendingTransition(*Parser, *glrStack, int, int, string, string) {}
func workCountRecordFinalPendingDiscards(*Parser, []glrStack, []glrStack)           {}
func workCountRecordBoundaryCull(*Parser, int, int)                                 {}
func workCountRecordFinalExpand(*Parser, *glrStack, int, bool, bool)                {}
func workCountRecordFinalExpansions(*Parser, []glrStack)                            {}
func workCountRecordFinalSelect(*Parser, []glrStack, *glrStack)                     {}
func workCountTopologySetParseContext(*Parser, *nodeArena, *bool)                   {}
func workCountTopologyRecordAction(*glrStack, Token, ParseAction, int)              {}
func workCountTopologyRecordActionResult(*glrStack)                                 {}
func workCountTopologyBeginReductionVersion(*glrStack, *glrStack, Token, ParseAction, int) {
}
func workCountTopologyEndReductionVersion(*glrStack, *glrStack)                {}
func workCountTopologyFinishReductionAction()                                  {}
func workCountTopologyRecordInitialVersion(*glrStack)                          {}
func workCountTopologyPrepareVersionCopy(*glrStack, *glrStack)                 {}
func workCountTopologyRecordVersionCopy(*glrStack, *glrStack)                  {}
func workCountTopologyCommitVersion(*glrStack)                                 {}
func workCountTopologyPreparePromotion(*glrStack)                              {}
func workCountTopologyCommitPromotion(*glrStack)                               {}
func workCountTopologyRecordDemotion(*glrStack, []stackEntry)                  {}
func workCountTopologyRecordNodeAllocation(*gssNode)                           {}
func workCountTopologyRecordLinkInsert(*gssNode, *gssNode, int, bool)          {}
func workCountTopologyRecordEntryPush(*glrStack)                               {}
func workCountTopologyRecordPopPath(*glrStack, []stackEntry, *gssNode, uint64) {}
func workCountTopologyRecordDirectPop(*glrStack, int)                          {}
func workCountTopologyRecordPackedReduceGroup(*Parser, *nodeArena, *glrStack, ParseAction, []reduceFork, uint64) {
}
func workCountTopologyRecordPackedReductionMergeAttempts(*Parser, *glrStack)      {}
func workCountTopologyRecordChildElection(*glrStack, reduceFork, reduceFork, int) {}
func workCountTopologyRecordMerge(*glrStack, *glrStack, bool)                     {}
func workCountTopologyRecordMergeBeforeMutation(*glrStack, *glrStack)             {}
func workCountTopologyCommitMerge(*glrStack)                                      {}
func workCountTopologyClearVersion(*glrStack)                                     {}
func workCountTopologyRequireMergeSuccess(bool)                                   {}
func workCountTopologyRetireVersion(*glrStack)                                    {}
func workCountTopologyRetireVersionIfActive(*glrStack)                            {}
func workCountTopologyRetireVersionsIfActive([]glrStack)                          {}
func workCountTopologyRetireUnpublishedVersions([]glrStack, []glrStack)           {}
func workCountTopologyRenumberVersion(*glrStack, *glrStack)                       {}
func workCountTopologyRetireMissingVersions([]glrStack, []glrStack)               {}
func workCountTopologyReconcileVersionSelection([]glrStack, []glrStack)           {}
func workCountTopologySyncVersionOrder([]glrStack)                                {}
func workCountTryPostReduceMergeObserved(p *Parser, _ string, _ string, target, candidate *glrStack) bool {
	return tryMergePostReduceFork(p, target, candidate)
}
