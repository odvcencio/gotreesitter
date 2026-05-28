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
		ReuseRejectDirty:                    t.reuseRejectDirty,
		ReuseRejectAncestorDirtyBeforeEdit:  t.reuseRejectAncestorDirtyBeforeEdit,
		ReuseRejectHasError:                 t.reuseRejectHasError,
		ReuseRejectInvalidSpan:              t.reuseRejectInvalidSpan,
		ReuseRejectOutOfBounds:              t.reuseRejectOutOfBounds,
		ReuseRejectRootNonLeafChanged:       t.reuseRejectRootNonLeafChanged,
		ReuseRejectLargeNonLeaf:             t.reuseRejectLargeNonLeaf,
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
		ScratchBytesAllocated:               t.scratchBytesAllocated,
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
