package parsercorephase0

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

func recoveryDiscontinuityTestSet(core *Core, first, second uint16) AlternativeSet {
	var set AlternativeSet
	if !core.alternativeSetInsert(&set, packAlternativeSetMember(first, 0)) ||
		!core.alternativeSetInsert(&set, packAlternativeSetMember(second, 0)) {
		panic("recovery discontinuity test set insertion failed")
	}
	return set
}

func recoveryDiscontinuityTestRefs(t *testing.T, core *Core, first, second uint64) DropCohortRefSet {
	t.Helper()
	var refs DropCohortRefSet
	for _, sequence := range []uint64{first, second} {
		if !core.AddDropCohortRef(&refs, DropCohortRef{Owner: 7, Epoch: 3, Sequence: sequence, Branch: 0}) {
			t.Fatalf("failed to add test drop-cohort reference %d", sequence)
		}
	}
	return refs
}

func recoveryDiscontinuityTestLineage(
	t *testing.T,
	core *Core,
	owner uint32,
	head Head,
	set AlternativeSet,
	refs DropCohortRefSet,
) {
	t.Helper()
	if err := core.ApplySchedulerAtomic(func(token SchedulerTransactionToken) error {
		if err := core.RecordHeadLineageOwned(token, head, CleanPathRankSelected, 11, set, false, true, refs); err != nil {
			return err
		}
		return core.RecordHeadOwnerOwned(token, head, owner)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryDiscontinuityOwnedCopiesAndUnionsLineage(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxNodes: 128, MaxLinks: 128, MaxSubtrees: 128, MaxDerivations: 16, MaxPopPaths: 16})
	leftSeed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rightSeed, err := core.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftSet := recoveryDiscontinuityTestSet(core, 1, 2)
	rightSet := recoveryDiscontinuityTestSet(core, 3, 4)
	leftRefs := recoveryDiscontinuityTestRefs(t, core, 1, 2)
	rightRefs := recoveryDiscontinuityTestRefs(t, core, 3, 4)
	recoveryDiscontinuityTestLineage(t, core, 101, leftSeed, leftSet, leftRefs)
	recoveryDiscontinuityTestLineage(t, core, 102, rightSeed, rightSet, rightRefs)
	context := RecoveryDiscontinuityContext{ByteOffset: 0}
	var left, right Head
	err = core.ApplySchedulerAtomic(func(token SchedulerTransactionToken) error {
		var innerErr error
		left, innerErr = core.AppendRecoveryDiscontinuityOwned(token, leftSeed, context)
		if innerErr != nil {
			return innerErr
		}
		right, innerErr = core.AppendRecoveryDiscontinuityOwned(token, rightSeed, context)
		return innerErr
	})
	if err != nil {
		t.Fatal(err)
	}
	leftLineage, err := core.nodeLineage(left.Node)
	if err != nil {
		t.Fatal(err)
	}
	if leftLineage.owner != 0 || leftLineage.rank != CleanPathRankSelected || leftLineage.lineage != 11 {
		t.Fatalf("copied marker lineage=%+v, want owner zero and selected lineage 11", leftLineage)
	}
	if got, ok := core.AlternativeSetMembers(leftLineage.set); !ok || !slices.Equal(got, []uint32{packAlternativeSetMember(1, 0), packAlternativeSetMember(2, 0)}) {
		t.Fatalf("copied marker alternative set=%v/%t", got, ok)
	}
	if got := recoveryDiscontinuityTestRefs(t, core, 1, 2); leftLineage.dropCohortRefs != got {
		t.Fatalf("copied marker references=%+v, want %+v", leftLineage.dropCohortRefs, got)
	}

	var merged Head
	err = core.ApplySchedulerAtomic(func(token SchedulerTransactionToken) error {
		var mergeErr error
		merged, mergeErr = core.MergeRecoveryDiscontinuityHeadsOwned(token, context, left, right)
		return mergeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	mergedNode, err := core.node(merged.Node)
	if err != nil {
		t.Fatal(err)
	}
	if mergedNode.state != 0 || mergedNode.byteOffset != 0 || mergedNode.linkCount != 2 {
		t.Fatalf("merged marker node=%+v, want ERROR_STATE zero-width two links", mergedNode)
	}
	mergedLineage, err := core.nodeLineage(merged.Node)
	if err != nil {
		t.Fatal(err)
	}
	if mergedLineage.owner != 0 || mergedLineage.rank != CleanPathRankSelected || mergedLineage.lineage != 11 {
		t.Fatalf("merged marker lineage=%+v, want scheduler owner zero and lineage 11", mergedLineage)
	}
	gotSet, ok := core.AlternativeSetMembers(mergedLineage.set)
	if !ok || !slices.Equal(gotSet, []uint32{
		packAlternativeSetMember(1, 0), packAlternativeSetMember(2, 0),
		packAlternativeSetMember(3, 0), packAlternativeSetMember(4, 0),
	}) {
		t.Fatalf("merged marker alternative set=%v/%t", gotSet, ok)
	}
	wantRefs := recoveryDiscontinuityTestRefs(t, core, 1, 2)
	if !core.UnionDropCohortRefs(&wantRefs, rightRefs) || mergedLineage.dropCohortRefs.Len() != wantRefs.Len() {
		t.Fatalf("merged marker references=%+v, want union %+v", mergedLineage.dropCohortRefs, wantRefs)
	}
	if stats, statsErr := core.Stats(merged); statsErr != nil || stats.CurrentExactPaths != 2 {
		t.Fatalf("merged marker stats=%+v err=%v, want two paths", stats, statsErr)
	}
	derivations, err := core.Derivations(merged)
	if err != nil {
		t.Fatal(err)
	}
	if len(derivations) != 2 {
		t.Fatalf("merged marker derivations=%+v, want two paths", derivations)
	}
	for _, derivation := range derivations {
		if len(derivation.Payloads) != 0 || derivation.Score != 0 || derivation.HasBranchOrder {
			t.Fatalf("merged marker derivation=%+v, want no child, score, or order", derivation)
		}
	}
	work := core.Work()
	if work.PhysicalHeadMergeAttempts != 1 || work.PhysicalHeadMergeInputLinks != 1 || work.PhysicalHeadMergeSuccesses != 1 {
		t.Fatalf("marker merge telemetry=%+v, want one attempt, input link, and success", work)
	}
}

func TestRecoveryDiscontinuityRejectsUnauthenticatedContextAndOrdinaryNullableLink(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxNodes: 32, MaxLinks: 32, MaxSubtrees: 32, MaxDerivations: 8, MaxPopPaths: 8})
	seed, err := core.Seed(3, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, context := range []RecoveryDiscontinuityContext{
		{ErrorState: 1, ByteOffset: 0},
		{ByteOffset: 1},
		{ByteOffset: 0, Checkpoint: 1},
	} {
		before := len(core.nodes)
		err := core.ApplySchedulerAtomic(func(token SchedulerTransactionToken) error {
			_, appendErr := core.AppendRecoveryDiscontinuityOwned(token, seed, context)
			return appendErr
		})
		if err == nil {
			t.Fatalf("context=%+v was accepted", context)
		}
		if len(core.nodes) != before {
			t.Fatalf("context=%+v changed node arena after rejection", context)
		}
	}
	if _, err := core.appendAdjacencyNode(0, 0, []linkRecord{{prev: seed.Node}}); err == nil {
		t.Fatal("ordinary zero-payload link was accepted")
	}
}

func TestRecoveryGraphAggregatePreservesMarkerPrefixAndDeclinesUnequalCosts(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxNodes: 256, MaxLinks: 256, MaxSubtrees: 256, MaxChildren: 256, MaxDerivations: 16, MaxPopPaths: 16})
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	cleanPayload, err := core.appendSubtree(subtreeRecord{symbol: 1, startByte: 0, endByte: 0, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cleanPred, err := core.appendAdjacencyNode(2, 0, []linkRecord{{prev: seed.Node, payload: cleanPayload}})
	if err != nil {
		t.Fatal(err)
	}
	missingPayload, err := core.MissingLeaf(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	missingPred, err := core.appendAdjacencyNode(3, 0, []linkRecord{{prev: seed.Node, payload: missingPayload}})
	if err != nil {
		t.Fatal(err)
	}
	context := RecoveryDiscontinuityContext{ByteOffset: 0}
	var cleanHead, missingHead Head
	err = core.ApplySchedulerAtomic(func(token SchedulerTransactionToken) error {
		var innerErr error
		cleanHead, innerErr = core.AppendRecoveryDiscontinuityOwned(token, Head{Node: cleanPred}, context)
		if innerErr != nil {
			return innerErr
		}
		missingHead, innerErr = core.AppendRecoveryDiscontinuityOwned(token, Head{Node: missingPred}, context)
		return innerErr
	})
	if err != nil {
		t.Fatal(err)
	}
	var aggregateHead Head
	err = core.ApplySchedulerAtomic(func(token SchedulerTransactionToken) error {
		var mergeErr error
		aggregateHead, mergeErr = core.MergeRecoveryDiscontinuityHeadsOwned(token, context, cleanHead, missingHead)
		return mergeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	symbols := make([]SelectedSymbolPolicy, 3)
	symbols[1].Visible = true
	symbols[2].Visible = true
	aggregate, supported, err := core.RecoveryGraphAggregateForHead(aggregateHead, symbols, core)
	if err != nil {
		t.Fatal(err)
	}
	if supported {
		t.Fatal("unequal recovery path costs were reported as supported")
	}
	if aggregate.MaximumVisibleCount != 1 || aggregate.MinimumErrorCost != 0 || aggregate.MaximumErrorCost != RecoveryCostPerMissingTree+RecoveryCostPerRecovery || aggregate.StoredPrecedenceMaximum != 0 || aggregate.PathCount != 2 {
		t.Fatalf("aggregate=%+v, want visible 1, costs 0/%d, precedence 0, paths 2", aggregate, RecoveryCostPerMissingTree+RecoveryCostPerRecovery)
	}
}

func TestRecoveryGraphAggregateIgnoresUnreachableMalformedNode(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxNodes: 256, MaxLinks: 256, MaxSubtrees: 256, MaxDerivations: 8, MaxPopPaths: 8})
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	unreachablePayload, err := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	unreachableLink := core.appendGraphLink(linkRecord{prev: seed.Node, payload: unreachablePayload})
	if _, err := core.appendNode(nodeRecord{state: 7, byteOffset: 0, firstLink: uint32(unreachableLink), linkCount: 1, pathCount: 1}); err != nil {
		t.Fatal(err)
	}
	visiblePayload, err := core.appendSubtree(subtreeRecord{symbol: 1, startByte: 0, endByte: 0, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	visibleHead, err := core.appendAdjacencyNode(2, 0, []linkRecord{{prev: seed.Node, payload: visiblePayload}})
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt only the unrelated node after the aggregate's reachable head is built.
	core.links[unreachableLink-1].payload = 0
	symbols := []SelectedSymbolPolicy{{}, {Visible: true}}
	aggregate, supported, err := core.RecoveryGraphAggregateForHead(Head{Node: visibleHead}, symbols, core)
	if err != nil || !supported || aggregate.PathCount != 1 || aggregate.MaximumVisibleCount != 1 {
		t.Fatalf("reachable aggregate=%+v supported=%t err=%v", aggregate, supported, err)
	}
}

func TestRecoveryGraphAggregateRequiresExactCostSource(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, supported, err := core.RecoveryGraphAggregateForHead(seed, nil, nil); err == nil || supported {
		t.Fatalf("nil source result=%t err=%v, want fail closed", supported, err)
	}
	errorPayload, err := core.appendSubtree(subtreeRecord{symbol: ErrorRegionSymbol, startByte: 0, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	errorHead, err := core.appendAdjacencyNode(2, 1, []linkRecord{{prev: seed.Node, payload: errorPayload}})
	if err != nil {
		t.Fatal(err)
	}
	if _, supported, err := core.RecoveryGraphAggregateForHead(Head{Node: errorHead}, nil, core); err == nil || supported {
		t.Fatalf("row-free Core source result=%t err=%v, want ERROR row-span failure", supported, err)
	}
}

func TestEOFAdmissionRejectsRecoveryDiscontinuityWithoutRecoveryVisitor(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxNodes: 32, MaxLinks: 32, MaxDerivations: 4, MaxPopPaths: 4})
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var head Head
	if err := core.ApplySchedulerAtomic(func(token SchedulerTransactionToken) error {
		var appendErr error
		head, appendErr = core.AppendRecoveryDiscontinuityOwned(token, seed, RecoveryDiscontinuityContext{ByteOffset: 0})
		return appendErr
	}); err != nil {
		t.Fatal(err)
	}
	_, err = core.VisitEOFAdmissionExactPath(head, core.AuthenticationGeneration(), nil, func(uint32, SubtreeID) error { return nil })
	if err == nil || !errors.Is(err, ErrEOFAdmissionMalformed) {
		t.Fatalf("marker EOF admission error=%v, want malformed fail-closed error", err)
	}
}

func TestRecoveryDiscontinuityMergeRollbackRestoresLineageSpill(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxNodes: 128, MaxLinks: 128, MaxSubtrees: 128, MaxDerivations: 16, MaxPopPaths: 16})
	leftSeed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rightSeed, err := core.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	recoveryDiscontinuityTestLineage(t, core, 201, leftSeed, recoveryDiscontinuityTestSet(core, 1, 2), recoveryDiscontinuityTestRefs(t, core, 1, 2))
	recoveryDiscontinuityTestLineage(t, core, 202, rightSeed, recoveryDiscontinuityTestSet(core, 3, 4), recoveryDiscontinuityTestRefs(t, core, 3, 4))
	context := RecoveryDiscontinuityContext{ByteOffset: 0}
	var left, right Head
	if err := core.ApplySchedulerAtomic(func(token SchedulerTransactionToken) error {
		var innerErr error
		left, innerErr = core.AppendRecoveryDiscontinuityOwned(token, leftSeed, context)
		if innerErr != nil {
			return innerErr
		}
		right, innerErr = core.AppendRecoveryDiscontinuityOwned(token, rightSeed, context)
		return innerErr
	}); err != nil {
		t.Fatal(err)
	}
	beforeNodes := len(core.nodes)
	beforeLinks := len(core.links)
	beforeSpill := slices.Clone(core.alternativeSpillArena)
	beforeLineages := slices.Clone(core.nodeLineages)
	beforeWork := core.Work()
	sentinel := errors.New("recovery discontinuity rollback")
	err = core.ApplySchedulerAtomic(func(token SchedulerTransactionToken) error {
		if _, mergeErr := core.MergeRecoveryDiscontinuityHeadsOwned(token, context, left, right); mergeErr != nil {
			return mergeErr
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback error=%v, want %v", err, sentinel)
	}
	if len(core.nodes) != beforeNodes || len(core.links) != beforeLinks || !slices.Equal(core.alternativeSpillArena, beforeSpill) || !reflect.DeepEqual(core.nodeLineages, beforeLineages) || core.Work() != beforeWork {
		t.Fatalf("rollback changed nodes=%d/%d links=%d/%d spill=%v/%v lineages equal=%t work=%+v/%+v", len(core.nodes), beforeNodes, len(core.links), beforeLinks, core.alternativeSpillArena, beforeSpill, reflect.DeepEqual(core.nodeLineages, beforeLineages), core.Work(), beforeWork)
	}
}
