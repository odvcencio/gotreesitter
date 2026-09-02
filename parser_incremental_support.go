package gotreesitter

func (s *parseReuseState) markReused(node *Node, primary *nodeArena) {
	if s == nil {
		return
	}
	s.reusedAny = true
	if node == nil {
		return
	}
	s.arenaRefs = appendUniqueArenaRef(s.arenaRefs, node.ownerArena, primary)
}

func (s *parseReuseState) retainBorrowed(primary *nodeArena) []*nodeArena {
	if s == nil || !s.reusedAny || len(s.arenaRefs) == 0 {
		return nil
	}
	uniq := uniqueArenas(s.arenaRefs, primary)
	if len(uniq) == 0 {
		return nil
	}
	for _, a := range uniq {
		a.Retain()
	}
	return uniq
}

func (t *incrementalParseTiming) toProfile() IncrementalParseProfile {
	if t == nil {
		return IncrementalParseProfile{}
	}
	reparse := t.totalNanos - t.reuseNanos
	if reparse < 0 {
		reparse = 0
	}
	return IncrementalParseProfile{
		ReuseCursorNanos:                    t.reuseNanos,
		ReparseNanos:                        reparse,
		ReusedSubtrees:                      t.reusedSubtrees,
		ReusedBytes:                         t.reusedBytes,
		NewNodesAllocated:                   t.newNodes,
		ReuseUnsupported:                    t.reuseUnsupported,
		ReuseUnsupportedReason:              t.reuseUnsupportedReason,
		AcceptedErrorRetryAttempts:          t.acceptedErrorRetryAttempts,
		AcceptedErrorRetryAdopted:           t.acceptedErrorRetryAdopted,
		AcceptedErrorRetryMergePerKey:       t.acceptedErrorRetryMergePerKey,
		AcceptedErrorRetryCause:             t.acceptedErrorRetryCause,
		OldTreeReuseRoute:                   t.oldTreeReuseRoute,
		ReuseRejectDirty:                    t.reuseRejectDirty,
		ReuseRejectAncestorDirtyBeforeEdit:  t.reuseRejectAncestorDirtyBeforeEdit,
		ReuseRejectHasError:                 t.reuseRejectHasError,
		ReuseRejectInvalidSpan:              t.reuseRejectInvalidSpan,
		ReuseRejectOutOfBounds:              t.reuseRejectOutOfBounds,
		ReuseRejectRootNonLeafChanged:       t.reuseRejectRootNonLeafChanged,
		ReuseObservedPreGotoStateMismatch:   t.reuseObservedPreGotoStateMismatch,
		ReuseRejectLargeNonLeaf:             t.reuseRejectLargeNonLeaf,
		ReuseRejectStaleNonLeafBoundary:     t.reuseRejectStaleNonLeafBoundary,
		ReuseRejectFragileNonLeaf:           t.reuseRejectFragileNonLeaf,
		BlockSpliceSteps:                    t.blockSpliceSteps,
		ReuseRejectScannerUnquiescent:       t.reuseRejectScannerUnquiescent,
		ReuseRejectFrontierProofUnavailable: t.reuseRejectFrontierProofUnavailable,
		RecoverSearches:                     t.recoverSearches,
		RecoverStateChecks:                  t.recoverStateChecks,
		RecoverStateSkips:                   t.recoverStateSkips,
		RecoverSymbolSkips:                  t.recoverSymbolSkips,
		RecoverLookups:                      t.recoverLookups,
		RecoverHits:                         t.recoverHits,
		MaxStacksSeen:                       t.maxStacksSeen,
		EntryScratchPeak:                    t.entryScratchPeak,
		StopReason:                          t.stopReason,
		TokensConsumed:                      t.tokensConsumed,
		LastTokenEndByte:                    t.lastTokenEndByte,
		ExpectedEOFByte:                     t.expectedEOFByte,
		ArenaBytesAllocated:                 t.arenaBytesAllocated,
		ArenaBaselineBytes:                  t.arenaBaselineBytes,
		ScratchBytesAllocated:               t.scratchBytesAllocated,
		ScratchBaselineBytes:                t.scratchBaselineBytes,
		EntryScratchBytesAllocated:          int64(t.entryScratchBytesAllocated),
		GSSBytesAllocated:                   int64(t.gssBytesAllocated),
		SingleStackIterations:               t.singleStackIterations,
		MultiStackIterations:                t.multiStackIterations,
		SingleStackTokens:                   t.singleStackTokens,
		MultiStackTokens:                    t.multiStackTokens,
		SingleStackGSSNodes:                 t.singleStackGSSNodes,
		MultiStackGSSNodes:                  t.multiStackGSSNodes,
		GSSNodesAllocated:                   t.gssNodesAllocated,
		GSSNodesRetained:                    t.gssNodesRetained,
		GSSNodesDroppedSameToken:            t.gssNodesDroppedSameToken,
		ParentNodesAllocated:                t.parentNodesAllocated,
		ParentNodesRetained:                 t.parentNodesRetained,
		ParentNodesDroppedSameToken:         t.parentNodesDroppedSameToken,
		LeafNodesAllocated:                  t.leafNodesAllocated,
		LeafNodesRetained:                   t.leafNodesRetained,
		LeafNodesDroppedSameToken:           t.leafNodesDroppedSameToken,
		MergeStacksIn:                       t.mergeStacksIn,
		MergeStacksOut:                      t.mergeStacksOut,
		MergeSlotsUsed:                      t.mergeSlotsUsed,
		GlobalCullStacksIn:                  t.globalCullStacksIn,
		GlobalCullStacksOut:                 t.globalCullStacksOut,
		ParserLoopNanos:                     t.parserLoopNanos,
		TokenNextNanos:                      t.tokenNextNanos,
		ActionDispatchNanos:                 t.actionDispatchNanos,
		ActionLookupNanos:                   t.actionLookupNanos,
		GLRMergeNanos:                       t.glrMergeNanos,
		GLRCullNanos:                        t.glrCullNanos,
		ResultSelectionNanos:                t.resultSelectionNanos,
		TransientParentMaterializationNanos: t.transientParentMaterializationNanos,
		ResultTreeBuildNanos:                t.resultTreeBuildNanos,
		TransientChildMaterializationNanos:  t.transientChildMaterializationNanos,
		ResultPythonKeywordRepairNanos:      t.resultPythonKeywordRepairNanos,
		ResultPythonRootRepairNanos:         t.resultPythonRootRepairNanos,
		ResultFinalizeRootNanos:             t.resultFinalizeRootNanos,
		ResultExtendTrailingNanos:           t.resultExtendTrailingNanos,
		ResultNormalizeRootStartNanos:       t.resultNormalizeRootStartNanos,
		ResultCompatibilityNanos:            t.resultCompatibilityNanos,
		ResultParentLinkNanos:               t.resultParentLinkNanos,
		ReduceRangeNanos:                    t.reduceRangeNanos,
		ReducePendingParentNanos:            t.reducePendingParentNanos,
		ReduceChildBuildNanos:               t.reduceChildBuildNanos,
		ReduceParentBuildNanos:              t.reduceParentBuildNanos,
		ReduceSpanNanos:                     t.reduceSpanNanos,
		ReduceStackPushNanos:                t.reduceStackPushNanos,
		ReduceNoTreeBuildNanos:              t.reduceNoTreeBuildNanos,
		ActionExtraShiftNanos:               t.actionExtraShiftNanos,
		ActionNoActionNanos:                 t.actionNoActionNanos,
		ActionNoActionRelexNanos:            t.actionNoActionRelexNanos,
		ActionNoActionMissingNanos:          t.actionNoActionMissingNanos,
		ActionNoActionRecoverNanos:          t.actionNoActionRecoverNanos,
		ActionNoActionErrorNanos:            t.actionNoActionErrorNanos,
		ActionConflictChoiceNanos:           t.actionConflictChoiceNanos,
		ActionConflictForkNanos:             t.actionConflictForkNanos,
		ActionSingleShiftNanos:              t.actionSingleShiftNanos,
		ActionSingleReduceNanos:             t.actionSingleReduceNanos,
		ActionSingleAcceptNanos:             t.actionSingleAcceptNanos,
		ActionSingleRecoverNanos:            t.actionSingleRecoverNanos,
		ActionSingleOtherNanos:              t.actionSingleOtherNanos,
		NormalizationNanos:                  t.normalizationNanos,
	}
}

// addAttempt aggregates operation work from another incremental attempt. The
// selected Tree's ParseRuntime remains per-attempt; IncrementalParseProfile is
// intentionally the total cost paid by the public operation.
func (t *incrementalParseTiming) addAttempt(other *incrementalParseTiming) {
	if t == nil || other == nil {
		return
	}
	t.totalNanos += other.totalNanos
	t.reuseNanos += other.reuseNanos
	t.reusedSubtrees += other.reusedSubtrees
	t.reusedBytes += other.reusedBytes
	t.newNodes += other.newNodes
	t.reuseUnsupported = t.reuseUnsupported || other.reuseUnsupported
	if t.reuseUnsupportedReason == "" {
		t.reuseUnsupportedReason = other.reuseUnsupportedReason
	}
	t.oldTreeReuseRoute = t.oldTreeReuseRoute || other.oldTreeReuseRoute
	t.reuseRejectDirty += other.reuseRejectDirty
	t.reuseRejectAncestorDirtyBeforeEdit += other.reuseRejectAncestorDirtyBeforeEdit
	t.reuseRejectHasError += other.reuseRejectHasError
	t.reuseRejectInvalidSpan += other.reuseRejectInvalidSpan
	t.reuseRejectOutOfBounds += other.reuseRejectOutOfBounds
	t.reuseRejectRootNonLeafChanged += other.reuseRejectRootNonLeafChanged
	t.reuseObservedPreGotoStateMismatch += other.reuseObservedPreGotoStateMismatch
	t.reuseRejectLargeNonLeaf += other.reuseRejectLargeNonLeaf
	t.reuseRejectStaleNonLeafBoundary += other.reuseRejectStaleNonLeafBoundary
	t.reuseRejectFragileNonLeaf += other.reuseRejectFragileNonLeaf
	t.blockSpliceSteps += other.blockSpliceSteps
	t.reuseRejectScannerUnquiescent += other.reuseRejectScannerUnquiescent
	t.reuseRejectFrontierProofUnavailable += other.reuseRejectFrontierProofUnavailable
	t.recoverSearches += other.recoverSearches
	t.recoverStateChecks += other.recoverStateChecks
	t.recoverStateSkips += other.recoverStateSkips
	t.recoverSymbolSkips += other.recoverSymbolSkips
	t.recoverLookups += other.recoverLookups
	t.recoverHits += other.recoverHits
	if other.maxStacksSeen > t.maxStacksSeen {
		t.maxStacksSeen = other.maxStacksSeen
	}
	if other.entryScratchPeak > t.entryScratchPeak {
		t.entryScratchPeak = other.entryScratchPeak
	}
	t.tokensConsumed += other.tokensConsumed
	t.arenaBytesAllocated += other.arenaBytesAllocated
	t.arenaBaselineBytes += other.arenaBaselineBytes
	t.scratchBytesAllocated += other.scratchBytesAllocated
	t.scratchBaselineBytes += other.scratchBaselineBytes
	t.entryScratchBytesAllocated += other.entryScratchBytesAllocated
	t.gssBytesAllocated += other.gssBytesAllocated
	t.singleStackIterations += other.singleStackIterations
	t.multiStackIterations += other.multiStackIterations
	t.singleStackTokens += other.singleStackTokens
	t.multiStackTokens += other.multiStackTokens
	t.singleStackGSSNodes += other.singleStackGSSNodes
	t.multiStackGSSNodes += other.multiStackGSSNodes
	t.gssNodesAllocated += other.gssNodesAllocated
	t.gssNodesRetained += other.gssNodesRetained
	t.gssNodesDroppedSameToken += other.gssNodesDroppedSameToken
	t.parentNodesAllocated += other.parentNodesAllocated
	t.parentNodesRetained += other.parentNodesRetained
	t.parentNodesDroppedSameToken += other.parentNodesDroppedSameToken
	t.leafNodesAllocated += other.leafNodesAllocated
	t.leafNodesRetained += other.leafNodesRetained
	t.leafNodesDroppedSameToken += other.leafNodesDroppedSameToken
	t.mergeStacksIn += other.mergeStacksIn
	t.mergeStacksOut += other.mergeStacksOut
	t.mergeSlotsUsed += other.mergeSlotsUsed
	t.globalCullStacksIn += other.globalCullStacksIn
	t.globalCullStacksOut += other.globalCullStacksOut
	t.parserLoopNanos += other.parserLoopNanos
	t.tokenNextNanos += other.tokenNextNanos
	t.actionDispatchNanos += other.actionDispatchNanos
	t.actionLookupNanos += other.actionLookupNanos
	t.glrMergeNanos += other.glrMergeNanos
	t.glrCullNanos += other.glrCullNanos
	t.resultSelectionNanos += other.resultSelectionNanos
	t.transientParentMaterializationNanos += other.transientParentMaterializationNanos
	t.resultTreeBuildNanos += other.resultTreeBuildNanos
	t.transientChildMaterializationNanos += other.transientChildMaterializationNanos
	t.resultPythonKeywordRepairNanos += other.resultPythonKeywordRepairNanos
	t.resultPythonRootRepairNanos += other.resultPythonRootRepairNanos
	t.resultFinalizeRootNanos += other.resultFinalizeRootNanos
	t.resultExtendTrailingNanos += other.resultExtendTrailingNanos
	t.resultNormalizeRootStartNanos += other.resultNormalizeRootStartNanos
	t.resultCompatibilityNanos += other.resultCompatibilityNanos
	t.resultParentLinkNanos += other.resultParentLinkNanos
	t.reduceRangeNanos += other.reduceRangeNanos
	t.reducePendingParentNanos += other.reducePendingParentNanos
	t.reduceChildBuildNanos += other.reduceChildBuildNanos
	t.reduceParentBuildNanos += other.reduceParentBuildNanos
	t.reduceSpanNanos += other.reduceSpanNanos
	t.reduceStackPushNanos += other.reduceStackPushNanos
	t.reduceNoTreeBuildNanos += other.reduceNoTreeBuildNanos
	t.actionExtraShiftNanos += other.actionExtraShiftNanos
	t.actionNoActionNanos += other.actionNoActionNanos
	t.actionNoActionRelexNanos += other.actionNoActionRelexNanos
	t.actionNoActionMissingNanos += other.actionNoActionMissingNanos
	t.actionNoActionRecoverNanos += other.actionNoActionRecoverNanos
	t.actionNoActionErrorNanos += other.actionNoActionErrorNanos
	t.actionConflictChoiceNanos += other.actionConflictChoiceNanos
	t.actionConflictForkNanos += other.actionConflictForkNanos
	t.actionSingleShiftNanos += other.actionSingleShiftNanos
	t.actionSingleReduceNanos += other.actionSingleReduceNanos
	t.actionSingleAcceptNanos += other.actionSingleAcceptNanos
	t.actionSingleRecoverNanos += other.actionSingleRecoverNanos
	t.actionSingleOtherNanos += other.actionSingleOtherNanos
	t.normalizationNanos += other.normalizationNanos
}

func (t *incrementalParseTiming) selectResult(tree *Tree) {
	if t == nil || tree == nil {
		return
	}
	rt := *tree.rawParseRuntime()
	t.stopReason = rt.StopReason
	t.lastTokenEndByte = rt.LastTokenEndByte
	t.expectedEOFByte = rt.ExpectedEOFByte
}

func appendUniqueArenaRef(refs []*nodeArena, arenaRef, exclude *nodeArena) []*nodeArena {
	if arenaRef == nil || arenaRef == exclude {
		return refs
	}
	for i := range refs {
		if refs[i] == arenaRef {
			return refs
		}
	}
	return append(refs, arenaRef)
}
