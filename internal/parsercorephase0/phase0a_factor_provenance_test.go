//go:build gts_parsercorephase0 && gts_parsercorephase0a

package parsercorephase0

import (
	"testing"
	"unsafe"
)

func phase0AObservedPrivate(t *testing.T, core *Core, payload SubtreeID, prev NodeID, state StateID, end uint32) Head {
	return phase0AObservedTerminalPrivate(t, core, payload, prev, state, end, false, 0, ForkOrder{})
}

func phase0AObservedTerminalPrivate(t *testing.T, core *Core, payload SubtreeID, prev NodeID, state StateID, end uint32, extra bool, score int64, order ForkOrder) Head {
	t.Helper()
	var out Head
	err := core.ApplyAtomic(func() error {
		key := core.boundaryKey(state, end)
		phase0AObserveTerminalShift(core, payload, prev, key.state, key.byteOffset, key.shifted, extra, score, order)
		var err error
		out, err = core.appendPrivate(state, end, linkInput{prev: prev, payload: payload, scoreDelta: score, order: order})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func phase0AObservedCondense(t *testing.T, core *Core, payload SubtreeID, prev NodeID, state StateID, end uint32) condenseOutcome {
	t.Helper()
	var out condenseOutcome
	err := core.ApplyAtomic(func() error {
		key := core.boundaryKey(state, end)
		phase0AObserveTerminalShift(core, payload, prev, key.state, key.byteOffset, key.shifted, false, 0, ForkOrder{})
		var err error
		err = core.condenseWithOutcomeAtomic(&out, key, linkInput{prev: prev, payload: payload})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func phase0AObservedReductionCondense(t *testing.T, core *Core, payload SubtreeID, prev NodeID, state StateID, end uint32, score int64) condenseOutcome {
	t.Helper()
	var out condenseOutcome
	err := core.ApplyAtomic(func() error {
		key := core.boundaryKey(state, end)
		phase0ABeginReductionConstruction(core, 1)
		phase0AObserveReductionOccurrence(core, linkInput{prev: prev, payload: payload, scoreDelta: score}, key)
		var err error
		err = core.condenseWithOutcomeAtomic(&out, key, linkInput{prev: prev, payload: payload, scoreDelta: score})
		if err == nil {
			phase0AFinishReductionConstruction(core)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPhase0ADirectAlternateDuplicateAndReplacementTransitions(t *testing.T) {
	t.Run("direct-alternate-duplicate", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		run := phase0ABeginShiftProvenanceRun(t, core, phase0AShiftProvenanceLimits())
		root, _ := core.Seed(1, 0)
		firstPayload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		secondPayload, _ := core.appendSubtree(subtreeRecord{symbol: 21, endByte: 1, terminal: true}, nil, nil, nil)
		first := phase0AObservedCondense(t, core, firstPayload, root.Node, 2, 1)
		second := phase0AObservedCondense(t, core, secondPayload, root.Node, 2, 1)
		duplicate := phase0AObservedCondense(t, core, secondPayload, root.Node, 2, 1)
		if first.change != condenseNew || second.change != condenseUpdated || duplicate.change != condenseUnchanged || duplicate.head != second.head {
			t.Fatalf("outcomes first=%+v second=%+v duplicate=%+v", first, second, duplicate)
		}
		proof, err := Phase0AObserverProof(core, run)
		if err != nil {
			t.Fatal(err)
		}
		counts := map[Phase0ATransitionKind]int{}
		for _, transition := range proof.Transitions {
			counts[transition.Kind]++
		}
		if counts[Phase0ATransitionDirectPublication] != 1 || counts[Phase0ATransitionAlternateAppend] != 1 || counts[Phase0ATransitionCanonicalRedirect] != 1 || counts[Phase0ATransitionDuplicateDrop] != 1 || len(proof.Bindings) != 2 || len(proof.Expressions) != 2 {
			t.Fatalf("transition counts=%v expressions=%+v bindings=%+v transitions=%+v", counts, proof.Expressions, proof.Bindings, proof.Transitions)
		}
	})

	t.Run("precedence-drop-replacement-copy", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		run := phase0ABeginShiftProvenanceRun(t, core, phase0AReductionLimits())
		root, _ := core.Seed(1, 0)
		childA, _ := core.appendSubtree(subtreeRecord{symbol: 40, endByte: 1, terminal: true}, nil, nil, nil)
		childB, _ := core.appendSubtree(subtreeRecord{symbol: 41, endByte: 1, terminal: true}, nil, nil, nil)
		childC, _ := core.appendSubtree(subtreeRecord{symbol: 42, endByte: 1, terminal: true}, nil, nil, nil)
		incumbent, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1}, []SubtreeID{childA}, nil, nil)
		lower, _ := core.appendSubtree(subtreeRecord{symbol: 20, productionID: 2, endByte: 1}, []SubtreeID{childB}, nil, nil)
		higher, _ := core.appendSubtree(subtreeRecord{symbol: 20, productionID: 3, endByte: 1}, []SubtreeID{childC}, nil, nil)
		distinct, _ := core.appendSubtree(subtreeRecord{symbol: 21, endByte: 1}, []SubtreeID{childA}, nil, nil)
		first := phase0AObservedReductionCondense(t, core, incumbent, root.Node, 2, 1, 10)
		alternate := phase0AObservedReductionCondense(t, core, distinct, root.Node, 2, 1, 0)
		dropped := phase0AObservedReductionCondense(t, core, lower, root.Node, 2, 1, 9)
		replaced := phase0AObservedReductionCondense(t, core, higher, root.Node, 2, 1, 11)
		if first.change != condenseNew || alternate.change != condenseUpdated || dropped.change != condenseUnchanged || replaced.change != condenseUpdated {
			t.Fatalf("outcomes first=%+v alternate=%+v dropped=%+v replaced=%+v", first, alternate, dropped, replaced)
		}
		proof, err := Phase0AObserverProof(core, run)
		if err != nil {
			t.Fatal(err)
		}
		counts := map[Phase0ATransitionKind]int{}
		for _, transition := range proof.Transitions {
			counts[transition.Kind]++
		}
		if counts[Phase0ATransitionPrecedenceDrop] != 1 || counts[Phase0ATransitionPrecedenceReplacement] != 1 || counts[Phase0ATransitionPhysicalCopy] != 1 || counts[Phase0ATransitionCanonicalRedirect] < 2 {
			t.Fatalf("replacement transition counts=%v transitions=%+v", counts, proof.Transitions)
		}
	})
}

func TestPhase0ACompoundCapsAndLeakedMergeScopeFailClosed(t *testing.T) {
	t.Run("direct-undo-byte-cap-is-atomic", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		run := phase0ABeginShiftProvenanceRun(t, core, phase0AShiftProvenanceLimits())
		root, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		if err := core.ApplyAtomic(func() error {
			key := core.boundaryKey(2, 1)
			phase0AObserveTerminalShift(core, payload, root.Node, key.state, key.byteOffset, key.shifted, false, 0, ForkOrder{})
			phase0AObservers.Lock()
			observer := phase0AObservers.byCore[core]
			beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
			required := phase0AExpressionBytes + phase0ABindingBytes + phase0ATransitionBytes + 2*phase0ACandidateUndoBytes
			phase0AObservers.limits.MaxBytes = beforeBytes + required - 1
			phase0AObservers.Unlock()
			_, parseErr := core.appendPrivate(2, 1, linkInput{prev: root.Node, payload: payload})
			phase0AObservers.Lock()
			if phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes || len(observer.factor.expressions) != 0 || len(observer.factor.bindings) != 0 || len(observer.factor.transitions) != 0 || len(observer.factor.candidateUndos) != 0 || observer.factor.candidates[0].Claimed || observer.factor.candidates[0].Resolved {
				t.Errorf("direct undo cap partially retained observer=%+v records=%d/%d bytes=%d/%d", observer.factor, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
			}
			phase0AObservers.Unlock()
			return parseErr
		}); err != nil {
			t.Fatalf("observer cap changed parser commit: %v", err)
		}
		proof, proofErr := Phase0AObserverProof(core, run)
		if !isPhase0AError(proofErr, Phase0AErrorByteCap, "") || len(proof.Candidates) != 1 || proof.Candidates[0].Claimed || proof.Candidates[0].Resolved {
			t.Fatalf("direct undo cap proof=%+v err=%v", proof, proofErr)
		}
	})

	t.Run("drop-undo-byte-cap-is-atomic", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		run := phase0ABeginShiftProvenanceRun(t, core, phase0AShiftProvenanceLimits())
		root, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		first := phase0AObservedCondense(t, core, payload, root.Node, 2, 1)
		if err := core.ApplyAtomic(func() error {
			key := core.boundaryKey(2, 1)
			in := linkInput{prev: root.Node, payload: payload}
			phase0AObserveTerminalShift(core, payload, root.Node, key.state, key.byteOffset, key.shifted, false, 0, ForkOrder{})
			phase0AObservers.Lock()
			observer := phase0AObservers.byCore[core]
			beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
			beforeTransitions := len(observer.factor.transitions)
			phase0AObservers.limits.MaxBytes = beforeBytes + phase0ATransitionBytes + 2*phase0ACandidateUndoBytes - 1
			phase0AObservers.Unlock()
			phase0AObserveCandidateDrop(core, key, in, first.head.Node, 0, Phase0ATransitionDuplicateDrop)
			phase0AObservers.Lock()
			candidate := observer.factor.candidates[len(observer.factor.candidates)-1]
			if phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes || len(observer.factor.transitions) != beforeTransitions || len(observer.factor.candidateUndos) != 0 || candidate.Claimed || candidate.Resolved {
				t.Errorf("drop undo cap partially retained observer=%+v records=%d/%d bytes=%d/%d", observer.factor, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
			}
			phase0AObservers.Unlock()
			return nil
		}); err != nil {
			t.Fatalf("observer cap changed parser commit: %v", err)
		}
		proof, proofErr := Phase0AObserverProof(core, run)
		if !isPhase0AError(proofErr, Phase0AErrorByteCap, "") || len(proof.Candidates) != 2 || proof.Candidates[1].Claimed || proof.Candidates[1].Resolved {
			t.Fatalf("drop undo cap proof=%+v err=%v", proof, proofErr)
		}
	})

	t.Run("candidate-cap-before-mutations", func(t *testing.T) {
		core, _, boundaries := phase0ATwoShiftFixture(t, false, Limits{})
		limits := phase0AShiftProvenanceLimits()
		limits.MaxCandidates = 1
		run := phase0ABeginShiftProvenanceRun(t, core, limits)
		heads, parseErr := core.ShiftOrdinaryClassifiedCohort(boundaries, Token{Symbol: 9, EndByte: 1})
		if parseErr != nil || len(heads) != 2 {
			t.Fatalf("observer cap changed parse heads=%v err=%v", heads, parseErr)
		}
		proof, proofErr := Phase0AObserverProof(core, run)
		if !isPhase0AError(proofErr, Phase0AErrorRecordCap, "") || len(proof.Mutations) != 0 || len(proof.Candidates) != 0 || len(proof.Expressions) != 0 || len(proof.Bindings) != 0 || len(proof.Transitions) != 0 {
			t.Fatalf("candidate cap partially published proof=%+v err=%v", proof, proofErr)
		}
	})

	t.Run("direct-expression-cap-is-compound", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		limits := phase0AShiftProvenanceLimits()
		limits.MaxExpressions = 1
		run := phase0ABeginShiftProvenanceRun(t, core, limits)
		root, _ := core.Seed(1, 0)
		first, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		second, _ := core.appendSubtree(subtreeRecord{symbol: 21, endByte: 1, terminal: true}, nil, nil, nil)
		phase0AObservedCondense(t, core, first, root.Node, 2, 1)
		phase0AObservedCondense(t, core, second, root.Node, 2, 1)
		proof, proofErr := Phase0AObserverProof(core, run)
		if !isPhase0AError(proofErr, Phase0AErrorRecordCap, "") || len(proof.Expressions) != 1 || len(proof.Bindings) != 1 || len(proof.Transitions) != 1 {
			t.Fatalf("direct compound cap proof=%+v err=%v", proof, proofErr)
		}
	})

	t.Run("commit-leaked-merge-plan", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		run := phase0ABeginShiftProvenanceRun(t, core, phase0AShiftProvenanceLimits())
		root, _ := core.Seed(1, 0)
		leftPayload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		rightPayload, _ := core.appendSubtree(subtreeRecord{symbol: 21, endByte: 1, terminal: true}, nil, nil, nil)
		left := phase0AObservedPrivate(t, core, leftPayload, root.Node, 2, 1)
		right := phase0AObservedPrivate(t, core, rightPayload, root.Node, 2, 1)
		if err := core.ApplyAtomic(func() error {
			phase0ABeginPredecessorMerge(core, left.Node, right.Node)
			return nil
		}); err != nil {
			t.Fatalf("observer leak changed parser commit: %v", err)
		}
		proof, proofErr := Phase0AObserverProof(core, run)
		if !isPhase0AError(proofErr, Phase0AErrorTransactionProof, "") || proof.Failure == nil {
			t.Fatalf("leaked merge scope proof=%+v err=%v", proof, proofErr)
		}
	})

	t.Run("merge-plan-byte-cap-before-append", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		run := phase0ABeginShiftProvenanceRun(t, core, phase0AShiftProvenanceLimits())
		root, _ := core.Seed(1, 0)
		leftPayload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		rightPayload, _ := core.appendSubtree(subtreeRecord{symbol: 21, endByte: 1, terminal: true}, nil, nil, nil)
		left := phase0AObservedPrivate(t, core, leftPayload, root.Node, 2, 1)
		right := phase0AObservedPrivate(t, core, rightPayload, root.Node, 2, 1)
		if err := core.ApplyAtomic(func() error {
			phase0AObservers.Lock()
			beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
			planBytes := uint64(unsafe.Sizeof(phase0AMergePlan{})) + 2*uint64(unsafe.Sizeof(phase0AMergeEntry{})) + uint64(unsafe.Sizeof(LinkID(0)))
			phase0AObservers.limits.MaxBytes = beforeBytes + planBytes - 1
			phase0AObservers.Unlock()
			phase0ABeginPredecessorMerge(core, left.Node, right.Node)
			phase0AObservers.Lock()
			if len(phase0AObservers.byCore[core].factor.mergePlans) != 0 || phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
				t.Errorf("merge cap partially retained plan=%+v records=%d/%d bytes=%d/%d", phase0AObservers.byCore[core].factor.mergePlans, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
			}
			phase0AObservers.Unlock()
			return nil
		}); err != nil {
			t.Fatalf("observer cap changed parser commit: %v", err)
		}
		proof, proofErr := Phase0AObserverProof(core, run)
		if !isPhase0AError(proofErr, Phase0AErrorByteCap, "") || proof.Failure == nil {
			t.Fatalf("merge cap proof=%+v err=%v", proof, proofErr)
		}
	})
}

func TestPhase0AFactorChoiceBindsExactLowerRoutes(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 16, MaxPopPaths: 16})
	run := phase0ABeginShiftProvenanceRun(t, core, Phase0AObserverLimits{
		MaxCores: 2, MaxRecords: 4096, MaxBytes: 1 << 20, MaxFrames: 32, MaxMutations: 512,
		MaxOccurrences: 128, MaxOccurrenceBytes: 128 << 10,
		MaxCandidates: 128, MaxExpressions: 128, MaxBindings: 256, MaxTransitions: 512, MaxSelectors: 32, MaxSelectorRoutes: 128,
	})
	rootA, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rootB, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftPayload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 10, terminal: true}, nil, nil, nil)
	rightPayload, _ := core.appendSubtree(subtreeRecord{symbol: 21, endByte: 10, terminal: true}, nil, nil, nil)
	topPayload, _ := core.appendSubtree(subtreeRecord{symbol: 30, startByte: 10, endByte: 11, terminal: true}, nil, nil, nil)
	left := phase0AObservedPrivate(t, core, leftPayload, rootA.Node, 7, 10)
	right := phase0AObservedPrivate(t, core, rightPayload, rootB.Node, 7, 10)
	old := phase0AObservedCondense(t, core, topPayload, left.Node, 8, 11)
	merged := phase0AObservedCondense(t, core, topPayload, right.Node, 8, 11)
	if old.change != condenseNew || merged.change != condenseUpdated || old.head == merged.head {
		t.Fatalf("factor outcomes old=%+v merged=%+v", old, merged)
	}
	proof, err := Phase0AObserverProof(core, run)
	if err != nil {
		t.Fatalf("proof=%+v err=%v", proof, err)
	}
	if len(proof.Selectors) != 1 || len(proof.SelectorRoutes) != 2 {
		t.Fatalf("selector proof selectors=%+v routes=%+v", proof.Selectors, proof.SelectorRoutes)
	}
	selector := proof.Selectors[0]
	if selector.MergedPredecessor == 0 || selector.RouteCount != 2 {
		t.Fatalf("selector=%+v", selector)
	}
	branches := map[Phase0ASelectorBranch]bool{}
	for _, route := range proof.SelectorRoutes {
		if route.Selector != selector.ID || route.SelectedLowerLink == 0 || route.SourceLowerLink == 0 {
			t.Fatalf("route=%+v selector=%+v", route, selector)
		}
		branches[route.Branch] = true
	}
	if !branches[Phase0ASelectorLeft] || !branches[Phase0ASelectorRight] {
		t.Fatalf("selector did not retain both branch origins: %+v", proof.SelectorRoutes)
	}
	var factor Phase0AExpressionRecord
	for _, expression := range proof.Expressions {
		if expression.Kind == Phase0AExpressionFactorChoice {
			factor = expression
		}
	}
	if factor.ID == 0 || factor.Selector != selector.ID || factor.Left == 0 || factor.Right == 0 || factor.Left == factor.Right {
		t.Fatalf("factor expression=%+v selectors=%+v", factor, proof.Selectors)
	}
	mergedNode, _ := core.node(merged.head.Node)
	mergedLinks, _ := phase0AStableLinkIDs(core, merged.head.Node)
	if mergedNode.linkCount != 1 || len(mergedLinks) != 1 {
		t.Fatalf("merged outer node=%+v links=%v", mergedNode, mergedLinks)
	}
	bound := false
	for _, binding := range proof.Bindings {
		if !binding.RolledBack && binding.Link == mergedLinks[0] && binding.Expression == factor.ID {
			bound = true
		}
	}
	if !bound {
		t.Fatalf("factor expression not bound to merged outer link: factor=%+v bindings=%+v", factor, proof.Bindings)
	}
	phase0AObservers.Lock()
	observer := phase0AObservers.byCore[core]
	for _, route := range proof.SelectorRoutes {
		resolved, resolveErr := phase0AResolveAcceptedExpressionLocked(observer, factor.ID, route.SelectedLowerLink)
		if resolveErr != nil {
			phase0AObservers.Unlock()
			t.Fatalf("resolve factor route %+v: %v", route, resolveErr)
		}
		want := factor.Left
		if route.Branch == Phase0ASelectorRight {
			want = factor.Right
		}
		if resolved.ID != want || resolved.Kind != Phase0AExpressionDirect {
			phase0AObservers.Unlock()
			t.Fatalf("factor route %+v resolved=%+v want=%d", route, resolved, want)
		}
	}
	phase0AObservers.Unlock()
}

func TestPhase0AFactorChoiceBindsRecursiveLowerRoutes(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 16, MaxPopPaths: 16})
	run := phase0ABeginShiftProvenanceRun(t, core, Phase0AObserverLimits{
		MaxCores: 2, MaxRecords: 4096, MaxBytes: 1 << 20, MaxFrames: 32, MaxMutations: 512,
		MaxOccurrences: 128, MaxOccurrenceBytes: 128 << 10,
		MaxCandidates: 128, MaxExpressions: 128, MaxBindings: 256, MaxTransitions: 512, MaxSelectors: 32, MaxSelectorRoutes: 128,
	})
	rootA, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rootB, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftPayload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 10, terminal: true}, nil, nil, nil)
	rightPayload, _ := core.appendSubtree(subtreeRecord{symbol: 21, endByte: 10, terminal: true}, nil, nil, nil)
	wrapper, _ := core.appendSubtree(subtreeRecord{symbol: 30, startByte: 10, endByte: 11, terminal: true}, nil, nil, nil)
	topPayload, _ := core.appendSubtree(subtreeRecord{symbol: 40, startByte: 11, endByte: 12, terminal: true}, nil, nil, nil)
	leftLower := phase0AObservedPrivate(t, core, leftPayload, rootA.Node, 7, 10)
	rightLower := phase0AObservedPrivate(t, core, rightPayload, rootB.Node, 7, 10)
	leftUpper := phase0AObservedPrivate(t, core, wrapper, leftLower.Node, 8, 11)
	rightUpper := phase0AObservedPrivate(t, core, wrapper, rightLower.Node, 8, 11)
	old := phase0AObservedCondense(t, core, topPayload, leftUpper.Node, 9, 12)
	merged := phase0AObservedCondense(t, core, topPayload, rightUpper.Node, 9, 12)
	if old.change != condenseNew || merged.change != condenseUpdated || old.head == merged.head {
		t.Fatalf("recursive factor outcomes old=%+v merged=%+v", old, merged)
	}
	proof, err := Phase0AObserverProof(core, run)
	if err != nil {
		t.Fatalf("recursive proof=%+v err=%v", proof, err)
	}
	factorExpressions := 0
	for _, expression := range proof.Expressions {
		if expression.Kind == Phase0AExpressionFactorChoice {
			factorExpressions++
		}
	}
	if len(proof.Selectors) != 2 || len(proof.SelectorRoutes) != 3 ||
		proof.Selectors[0].RouteCount != 2 || proof.Selectors[1].RouteCount != 1 ||
		factorExpressions != 2 {
		t.Fatalf(
			"recursive proof selectors=%+v routes=%+v factor expressions=%d expressions=%+v",
			proof.Selectors, proof.SelectorRoutes, factorExpressions, proof.Expressions,
		)
	}
}

func TestPhase0APhysicalLinkReuseKeepsRollbackTombstone(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	run := phase0ABeginShiftProvenanceRun(t, core, phase0AShiftProvenanceLimits())
	root, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
	var rolled Head
	_ = core.ApplyAtomic(func() error {
		rolled = phase0AObservedPrivate(t, core, payload, root.Node, 2, 1)
		return errPhase0ATestRollback
	})
	reused := phase0AObservedPrivate(t, core, payload, root.Node, 2, 1)
	rolledIDs, _ := phase0AStableLinkIDs(core, reused.Node)
	if len(rolledIDs) != 1 || rolled.Node != reused.Node {
		t.Fatalf("fixture did not reuse physical ordinals rolled=%+v reused=%+v links=%v", rolled, reused, rolledIDs)
	}
	proof, err := Phase0AObserverProof(core, run)
	if err != nil {
		t.Fatal(err)
	}
	var live, tombstone int
	for _, binding := range proof.Bindings {
		if binding.Link != rolledIDs[0] {
			continue
		}
		if binding.RolledBack {
			tombstone++
		} else {
			live++
		}
	}
	if live != 1 || tombstone != 1 {
		t.Fatalf("physical reuse bindings=%+v", proof.Bindings)
	}
}

func TestPhase0AAncestorCandidateUndoAcrossNestedTransactions(t *testing.T) {
	t.Run("child rollback restores ancestor candidate for retry", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		run := phase0ABeginShiftProvenanceRun(t, core, phase0AShiftProvenanceLimits())
		root, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		var retried Head
		err := core.ApplyAtomic(func() error {
			key := core.boundaryKey(2, 1)
			phase0AObserveTerminalShift(core, payload, root.Node, key.state, key.byteOffset, key.shifted, false, 0, ForkOrder{})
			innerErr := core.ApplyAtomic(func() error {
				if _, err := core.appendPrivate(2, 1, linkInput{prev: root.Node, payload: payload}); err != nil {
					return err
				}
				return errPhase0ATestRollback
			})
			if innerErr == nil {
				return &phase0ATestError{"nested rollback was not observed"}
			}
			var err error
			retried, err = core.appendPrivate(2, 1, linkInput{prev: root.Node, payload: payload})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		proof, err := Phase0AObserverProof(core, run)
		if err != nil {
			t.Fatal(err)
		}
		links, _ := phase0AStableLinkIDs(core, retried.Node)
		if len(links) != 1 || len(proof.Candidates) != 1 || proof.Candidates[0].RolledBack || !proof.Candidates[0].Claimed || !proof.Candidates[0].Resolved {
			t.Fatalf("retry proof candidate=%+v links=%v", proof.Candidates, links)
		}
		live, tombstone := 0, 0
		for _, binding := range proof.Bindings {
			if binding.Link != links[0] {
				continue
			}
			if binding.RolledBack {
				tombstone++
			} else {
				live++
			}
		}
		if live != 1 || tombstone != 1 {
			t.Fatalf("retry bindings=%+v", proof.Bindings)
		}
		if err := core.EndRun(run); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("child commit remains undoable by parent rollback", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		run := phase0ABeginShiftProvenanceRun(t, core, phase0AShiftProvenanceLimits())
		root, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		var retried Head
		err := core.ApplyAtomic(func() error {
			key := core.boundaryKey(2, 1)
			phase0AObserveTerminalShift(core, payload, root.Node, key.state, key.byteOffset, key.shifted, false, 0, ForkOrder{})
			middleErr := core.ApplyAtomic(func() error {
				if err := core.ApplyAtomic(func() error {
					_, err := core.appendPrivate(2, 1, linkInput{prev: root.Node, payload: payload})
					return err
				}); err != nil {
					return err
				}
				return errPhase0ATestRollback
			})
			if middleErr == nil {
				return &phase0ATestError{"middle rollback was not observed"}
			}
			var err error
			retried, err = core.appendPrivate(2, 1, linkInput{prev: root.Node, payload: payload})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		proof, err := Phase0AObserverProof(core, run)
		if err != nil {
			t.Fatal(err)
		}
		links, _ := phase0AStableLinkIDs(core, retried.Node)
		if len(proof.Candidates) != 1 || proof.Candidates[0].RolledBack || !proof.Candidates[0].Claimed || !proof.Candidates[0].Resolved {
			t.Fatalf("parent rollback candidate=%+v", proof.Candidates)
		}
		live, tombstone := 0, 0
		for _, binding := range proof.Bindings {
			if len(links) == 0 || binding.Link != links[0] {
				continue
			}
			if binding.RolledBack {
				tombstone++
			} else {
				live++
			}
		}
		if live != 1 || tombstone != 1 {
			t.Fatalf("parent rollback bindings=%+v", proof.Bindings)
		}
		if err := core.EndRun(run); err != nil {
			t.Fatal(err)
		}
	})
}

var errPhase0ATestRollback = &phase0ATestError{"rollback"}

type phase0ATestError struct{ text string }

func (e *phase0ATestError) Error() string { return e.text }
