//go:build gts_parsercorephase0

package parsercorephase0

import (
	"errors"
	"reflect"
	"testing"
)

// newStage2PhysicalHeadPair creates two immutable graph heads at one boundary.
// Each head owns a different predecessor path, so a merge must retain both.
func newStage2PhysicalHeadPair(t *testing.T) (*Core, Head, Head, uint32, uint32) {
	t.Helper()
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	leftRoot, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rightRoot, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftPayload, err := compact.appendSubtree(subtreeRecord{
		symbol: 10, startByte: 0, endByte: 1, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rightPayload, err := compact.appendSubtree(subtreeRecord{
		symbol: 11, startByte: 0, endByte: 1, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	leftLink := compact.appendGraphLink(linkRecord{prev: leftRoot.Node, payload: leftPayload})
	leftNode, err := compact.appendNode(nodeRecord{
		state: 3, byteOffset: 1, firstLink: uint32(leftLink), linkCount: 1, pathCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	rightLink := compact.appendGraphLink(linkRecord{prev: rightRoot.Node, payload: rightPayload})
	rightNode, err := compact.appendNode(nodeRecord{
		state: 3, byteOffset: 1, firstLink: uint32(rightLink), linkCount: 1, pathCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return compact, Head{Node: leftNode}, Head{Node: rightNode}, uint32(leftPayload), uint32(rightPayload)
}

func TestStage2PhysicalHeadMergeUnionsEquivalentDistinctNodes(t *testing.T) {
	compact, left, right, leftPayload, rightPayload := newStage2PhysicalHeadPair(t)
	candidates := []CondenseCandidate{
		{Head: left, Shifted: false, Checkpoint: 0},
		{Head: right, Shifted: false, Checkpoint: 0},
	}
	before := compact.Work()
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.ReindexCondenseCandidatesOwned(owner, candidates)
	})
	if err != nil {
		t.Fatalf("ReindexCondenseCandidatesOwned: %v", err)
	}
	canonical, ok := compact.CanonicalBoundary(3, 1, false, 0)
	if !ok {
		t.Fatal("physical-head merge did not publish a canonical boundary")
	}
	if canonical == left || canonical == right {
		t.Fatalf("physical-head merge reused one input node: canonical=%+v left=%+v right=%+v", canonical, left, right)
	}
	stats, err := compact.Stats(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CurrentExactPaths != 2 {
		t.Fatalf("merged physical head paths=%d, want 2", stats.CurrentExactPaths)
	}
	derivations, err := compact.Derivations(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if len(derivations) != 2 {
		t.Fatalf("merged physical head derivations=%+v, want two", derivations)
	}
	seenLeft, seenRight := false, false
	for _, derivation := range derivations {
		if len(derivation.Payloads) != 1 {
			t.Fatalf("merged derivation=%+v, want one payload", derivation)
		}
		seenLeft = seenLeft || uint32(derivation.Payloads[0]) == leftPayload
		seenRight = seenRight || uint32(derivation.Payloads[0]) == rightPayload
	}
	if !seenLeft || !seenRight {
		t.Fatalf("merged derivations lost one predecessor payload: %+v", derivations)
	}
	after := compact.Work()
	if after.PhysicalHeadMergeAttempts-before.PhysicalHeadMergeAttempts != 1 ||
		after.PhysicalHeadMergeSuccesses-before.PhysicalHeadMergeSuccesses != 1 ||
		after.PhysicalHeadMergeInputLinks-before.PhysicalHeadMergeInputLinks != 1 {
		t.Fatalf("physical-head merge telemetry=%+v before=%+v", after, before)
	}
}

func TestStage2PhysicalHeadMergeAtAndPastSixVersionBoundaryRetainsEveryPath(t *testing.T) {
	for _, test := range []struct {
		name      string
		headCount int
	}{
		{name: "at_boundary", headCount: 6},
		{name: "past_boundary", headCount: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 16, MaxPopPaths: 16})
			candidates := make([]CondenseCandidate, 0, test.headCount)
			wantPayloads := make(map[SubtreeID]bool, test.headCount)
			for index := 0; index < test.headCount; index++ {
				root, err := compact.Seed(StateID(index+1), 0)
				if err != nil {
					t.Fatal(err)
				}
				payload, err := compact.appendSubtree(subtreeRecord{
					symbol: Symbol(index + 10), startByte: 0, endByte: 1, terminal: true,
				}, nil, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				link := compact.appendGraphLink(linkRecord{prev: root.Node, payload: payload})
				node, err := compact.appendNode(nodeRecord{
					state: 7, byteOffset: 1, firstLink: uint32(link), linkCount: 1, pathCount: 1,
				})
				if err != nil {
					t.Fatal(err)
				}
				head := Head{Node: node}
				candidates = append(candidates, CondenseCandidate{Head: head})
				wantPayloads[payload] = true
			}

			before := compact.Work()
			err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				return compact.ReindexCondenseCandidatesOwned(owner, candidates)
			})
			if err != nil {
				t.Fatalf("ReindexCondenseCandidatesOwned: %v", err)
			}
			canonical, ok := compact.CanonicalBoundary(7, 1, false, 0)
			if !ok {
				t.Fatal("condense did not publish a canonical boundary")
			}
			stats, err := compact.Stats(canonical)
			if err != nil {
				t.Fatal(err)
			}
			if stats.CurrentExactPaths != uint64(test.headCount) {
				t.Fatalf("physical head paths=%d, want %d", stats.CurrentExactPaths, test.headCount)
			}
			derivations, err := compact.Derivations(canonical)
			if err != nil {
				t.Fatal(err)
			}
			if len(derivations) != test.headCount {
				t.Fatalf("derivations=%d, want %d", len(derivations), test.headCount)
			}
			for _, derivation := range derivations {
				if len(derivation.Payloads) != 1 || !wantPayloads[derivation.Payloads[0]] {
					t.Fatalf("derivation=%+v lost a path", derivation)
				}
				delete(wantPayloads, derivation.Payloads[0])
			}
			if len(wantPayloads) != 0 {
				t.Fatalf("merge lost payloads=%v", wantPayloads)
			}
			after := compact.Work()
			if after.PhysicalHeadMergeAttempts-before.PhysicalHeadMergeAttempts != uint64(test.headCount-1) ||
				after.PhysicalHeadMergeSuccesses-before.PhysicalHeadMergeSuccesses != uint64(test.headCount-1) ||
				after.PhysicalHeadMergeInputLinks-before.PhysicalHeadMergeInputLinks != uint64(test.headCount-1) {
				t.Fatalf("merge telemetry=%+v before=%+v", after, before)
			}
		})
	}
}

func TestStage2PhysicalHeadMergeKeepsScannerCheckpointIdentity(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	leftCheckpoint := mustInternCheckpoint(t, compact, []byte("scanner-left"))
	rightCheckpoint := mustInternCheckpoint(t, compact, []byte("scanner-right"))
	if err := compact.SetPhaseCheckpoint(leftCheckpoint); err != nil {
		t.Fatal(err)
	}
	leftRoot, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftPayload, err := compact.appendSubtree(subtreeRecord{
		symbol: 10, startByte: 0, endByte: 1, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	leftNode, err := compact.appendAdjacencyNodeAt(7, 1, leftCheckpoint, []linkRecord{{prev: leftRoot.Node, payload: leftPayload}})
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.SetPhaseCheckpoint(rightCheckpoint); err != nil {
		t.Fatal(err)
	}
	rightRoot, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	rightPayload, err := compact.appendSubtree(subtreeRecord{
		symbol: 11, startByte: 0, endByte: 1, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rightNode, err := compact.appendAdjacencyNodeAt(7, 1, rightCheckpoint, []linkRecord{{prev: rightRoot.Node, payload: rightPayload}})
	if err != nil {
		t.Fatal(err)
	}
	candidates := []CondenseCandidate{
		{Head: Head{Node: leftNode}, Checkpoint: leftCheckpoint},
		{Head: Head{Node: rightNode}, Checkpoint: rightCheckpoint},
	}
	before := compact.Work()
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.ReindexCondenseCandidatesOwned(owner, candidates)
	})
	if err != nil {
		t.Fatalf("ReindexCondenseCandidatesOwned: %v", err)
	}
	leftCanonical, leftOK := compact.CanonicalBoundary(7, 1, false, leftCheckpoint)
	rightCanonical, rightOK := compact.CanonicalBoundary(7, 1, false, rightCheckpoint)
	if !leftOK || !rightOK || leftCanonical.Node != leftNode || rightCanonical.Node != rightNode {
		t.Fatalf("scanner checkpoint merge changed identities: left=%+v/%t right=%+v/%t", leftCanonical, leftOK, rightCanonical, rightOK)
	}
	if leftCanonical == rightCanonical {
		t.Fatalf("distinct scanner checkpoints shared one physical head: left=%+v right=%+v", leftCanonical, rightCanonical)
	}
	after := compact.Work()
	if after.PhysicalHeadMergeAttempts != before.PhysicalHeadMergeAttempts ||
		after.PhysicalHeadMergeSuccesses != before.PhysicalHeadMergeSuccesses ||
		after.PhysicalHeadMergeInputLinks != before.PhysicalHeadMergeInputLinks {
		t.Fatalf("checkpoint guard entered physical merge path: before=%+v after=%+v", before, after)
	}
}

func TestStage2PhysicalHeadMergeSeparatesLookaheadAndNodeCheckpoints(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	nodeCheckpoint := mustInternCheckpoint(t, compact, []byte("last-consumed"))
	lookaheadCheckpoint := mustInternCheckpoint(t, compact, []byte("current-lookahead"))
	if err := compact.SetPhaseCheckpoint(nodeCheckpoint); err != nil {
		t.Fatal(err)
	}
	leftRoot, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rightRoot, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftPayload, err := compact.appendSubtree(subtreeRecord{symbol: 10, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rightPayload, err := compact.appendSubtree(subtreeRecord{symbol: 11, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	leftNode, err := compact.appendAdjacencyNodeAt(7, 1, nodeCheckpoint, []linkRecord{{prev: leftRoot.Node, payload: leftPayload}})
	if err != nil {
		t.Fatal(err)
	}
	rightNode, err := compact.appendAdjacencyNodeAt(7, 1, nodeCheckpoint, []linkRecord{{prev: rightRoot.Node, payload: rightPayload}})
	if err != nil {
		t.Fatal(err)
	}
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.ReindexCondenseCandidatesOwned(owner, []CondenseCandidate{
			{Head: Head{Node: leftNode}, Checkpoint: lookaheadCheckpoint},
			{Head: Head{Node: rightNode}, Checkpoint: lookaheadCheckpoint},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	merged, ok := compact.CanonicalBoundary(7, 1, false, lookaheadCheckpoint)
	if !ok {
		t.Fatal("lookahead boundary is unavailable")
	}
	gotCheckpoint, exact := compact.nodeScannerCheckpoint(merged.Node)
	if !exact || gotCheckpoint != nodeCheckpoint {
		t.Fatalf("merged node checkpoint=%d/%t, want %d", gotCheckpoint, exact, nodeCheckpoint)
	}
}

func TestStage2PhysicalHeadMergeFailsClosedOnDifferentNodeCheckpoints(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	leftCheckpoint := mustInternCheckpoint(t, compact, []byte("left-node"))
	rightCheckpoint := mustInternCheckpoint(t, compact, []byte("right-node"))
	lookaheadCheckpoint := mustInternCheckpoint(t, compact, []byte("shared-lookahead"))
	leftRoot, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rightRoot, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftPayload, err := compact.appendSubtree(subtreeRecord{symbol: 10, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rightPayload, err := compact.appendSubtree(subtreeRecord{symbol: 11, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	leftNode, err := compact.appendAdjacencyNodeAt(7, 1, leftCheckpoint, []linkRecord{{prev: leftRoot.Node, payload: leftPayload}})
	if err != nil {
		t.Fatal(err)
	}
	rightNode, err := compact.appendAdjacencyNodeAt(7, 1, rightCheckpoint, []linkRecord{{prev: rightRoot.Node, payload: rightPayload}})
	if err != nil {
		t.Fatal(err)
	}
	before := compact.Work()
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.ReindexCondenseCandidatesOwned(owner, []CondenseCandidate{
			{Head: Head{Node: leftNode}, Checkpoint: lookaheadCheckpoint},
			{Head: Head{Node: rightNode}, Checkpoint: lookaheadCheckpoint},
		})
	})
	if err == nil || err.Error() != "parser-core phase zero: physical heads have different scanner provenance" {
		t.Fatalf("different node checkpoint error=%v", err)
	}
	if compact.Work() != before {
		t.Fatalf("different node checkpoints entered merge telemetry: before=%+v after=%+v", before, compact.Work())
	}
}

func TestStage2PhysicalHeadMergeNormalizesLiveScope(t *testing.T) {
	compact, left, right, _, _ := newStage2PhysicalHeadPair(t)
	candidates := []CondenseCandidate{{Head: left}, {Head: right}}
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.RunSchedulerOwned(owner, func() error {
			return compact.runLiveCondenseCandidates(candidates, func() error {
				if len(compact.condenseCandidates) != 1 {
					t.Fatalf("live physical heads=%d, want 1", len(compact.condenseCandidates))
				}
				stats, err := compact.Stats(compact.condenseCandidates[0].Head)
				if err != nil {
					return err
				}
				if stats.CurrentExactPaths != 2 {
					t.Fatalf("live physical-head paths=%d, want 2", stats.CurrentExactPaths)
				}
				return nil
			})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if compact.condenseScopeActive || len(compact.condenseCandidates) != 0 {
		t.Fatalf("live physical-head scope leaked: active=%t candidates=%+v", compact.condenseScopeActive, compact.condenseCandidates)
	}
}

func TestStage2PhysicalHeadMergeRollsBackCapacityFailure(t *testing.T) {
	compact, left, right, _, _ := newStage2PhysicalHeadPair(t)
	compact.limits.MaxLinksPerBoundary = 1
	beforeNodes, beforeLinks := len(compact.nodes), len(compact.links)
	beforeBoundaries := compact.BoundaryIndexStats()
	beforeWork := compact.Work()
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.ReindexCondenseCandidatesOwned(owner, []CondenseCandidate{{Head: left}, {Head: right}})
	})
	var capacityErr *LiveLinkCapacityError
	if !errors.As(err, &capacityErr) {
		t.Fatalf("physical-head merge error=%v, want LiveLinkCapacityError", err)
	}
	if len(compact.nodes) != beforeNodes || len(compact.links) != beforeLinks || compact.BoundaryIndexStats() != beforeBoundaries {
		t.Fatalf("failed physical-head merge changed storage: nodes=%d/%d links=%d/%d boundaries=%+v/%+v",
			len(compact.nodes), beforeNodes, len(compact.links), beforeLinks, compact.BoundaryIndexStats(), beforeBoundaries)
	}
	if after := compact.Work(); after != beforeWork {
		t.Fatalf("failed physical-head merge changed telemetry: before=%+v after=%+v", beforeWork, after)
	}
}

func TestStage2PhysicalHeadMergeRejectsRecoveryCost(t *testing.T) {
	compact, left, right, _, _ := newStage2PhysicalHeadPair(t)
	compact.nodeLineages[left.Node-1].storedErrorCost = 610
	compact.nodeLineages[right.Node-1].storedErrorCost = 610
	beforeNodes, beforeLinks := len(compact.nodes), len(compact.links)
	beforeBoundaries := compact.BoundaryIndexStats()
	beforeWork := compact.Work()
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.ReindexCondenseCandidatesOwned(owner, []CondenseCandidate{
			{Head: left, ErrorCost: 610}, {Head: right, ErrorCost: 610},
		})
	})
	if err == nil || err.Error() != "parser-core phase zero: physical head merge requires zero error cost" {
		t.Fatalf("recovery-cost merge error=%v", err)
	}
	if len(compact.nodes) != beforeNodes || len(compact.links) != beforeLinks ||
		compact.BoundaryIndexStats() != beforeBoundaries || compact.Work() != beforeWork {
		t.Fatal("recovery-cost rejection changed compact state")
	}
}

func TestRecursiveMergeUsesTargetCostForMixedLinkSplits(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxNodes: 32, MaxLinks: 32, MaxSubtrees: 16})
	leftBase, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rightBase, err := compact.appendNode(nodeRecord{state: 1, byteOffset: 0, pathCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	compact.nodeLineages[rightBase-1].storedErrorCost = 5
	payload, err := compact.appendSubtree(subtreeRecord{symbol: 9, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	leftLink := compact.appendGraphLink(linkRecord{prev: leftBase.Node, payload: payload})
	leftNode, err := compact.appendNode(nodeRecord{
		state: 3, byteOffset: 1, firstLink: uint32(leftLink), linkCount: 1, pathCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	rightLink := compact.appendGraphLink(linkRecord{prev: rightBase, payload: payload})
	rightNode, err := compact.appendNode(nodeRecord{
		state: 3, byteOffset: 1, firstLink: uint32(rightLink), linkCount: 1, pathCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The two top heads have the same cumulative cost. Their lower
	// predecessors have different cost splits, so recursive insertion must
	// retain both links and publish the authenticated target cost.
	compact.nodeLineages[leftNode-1].storedErrorCost = 10
	compact.nodeLineages[rightNode-1].storedErrorCost = 10
	folded := precedenceMaximumWitness{}
	merged, changed, err := compact.mergePredecessorsBounded(leftNode, rightNode, 0, &folded)
	if err != nil {
		t.Fatalf("mergePredecessorsBounded: %v", err)
	}
	if !changed {
		t.Fatal("mixed link split did not publish a merged predecessor")
	}
	cost, err := compact.RecoveryStoredErrorCost(Head{Node: merged})
	if err != nil {
		t.Fatal(err)
	}
	if cost != 10 {
		t.Fatalf("merged target cost=%d, want authenticated cost 10", cost)
	}
	mergedNode, err := compact.node(merged)
	if err != nil {
		t.Fatal(err)
	}
	if mergedNode.linkCount != 2 {
		t.Fatalf("merged link count=%d, want both mixed-split links", mergedNode.linkCount)
	}
}

func TestStage2PhysicalHeadMergePreservesCandidateOwnership(t *testing.T) {
	compact, left, right, _, _ := newStage2PhysicalHeadPair(t)
	candidates := []CondenseCandidate{{Head: left}, {Head: right}}
	want := append([]CondenseCandidate(nil), candidates...)
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.RunSchedulerOwned(owner, func() error {
			return compact.runLiveCondenseCandidates(candidates, func() error { return nil })
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("caller candidates changed: got=%+v want=%+v", candidates, want)
	}
}

func TestStage2PhysicalHeadMergeRequiresSchedulerTransaction(t *testing.T) {
	compact, left, right, _, _ := newStage2PhysicalHeadPair(t)
	beforeNodes, beforeLinks := len(compact.nodes), len(compact.links)
	beforeBoundaries := compact.BoundaryIndexStats()
	beforeWork := compact.Work()
	err := compact.enterLiveCondenseCandidates([]CondenseCandidate{{Head: left}, {Head: right}})
	if err == nil || err.Error() != "parser-core phase zero: live condense candidate scope requires a scheduler transaction" {
		t.Fatalf("unowned live merge error=%v", err)
	}
	if len(compact.nodes) != beforeNodes || len(compact.links) != beforeLinks ||
		compact.BoundaryIndexStats() != beforeBoundaries || compact.Work() != beforeWork ||
		compact.condenseScopeActive || len(compact.condenseCandidates) != 0 {
		t.Fatal("unowned live merge changed compact state")
	}
}

func TestStage2PhysicalHeadMergeSkipsUnrelatedLiveBoundary(t *testing.T) {
	compact, left, right, _, _ := newStage2PhysicalHeadPair(t)
	unrelatedRoot, err := compact.Seed(9, 0)
	if err != nil {
		t.Fatal(err)
	}
	unrelatedPayload, err := compact.appendSubtree(subtreeRecord{
		symbol: 19, startByte: 0, endByte: 1, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	unrelatedNode, err := compact.appendAdjacencyNode(3, 1, []linkRecord{{prev: unrelatedRoot.Node, payload: unrelatedPayload}})
	if err != nil {
		t.Fatal(err)
	}
	key := compact.boundaryKey(3, 1)
	probe, _ := compact.boundaries.probe(boundaryIdentityFromKey(key))
	if err := compact.publishBoundary(probe, unrelatedNode); err != nil {
		t.Fatal(err)
	}
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.RunSchedulerOwned(owner, func() error {
			return compact.runLiveCondenseCandidates(
				[]CondenseCandidate{{Head: left}, {Head: right}},
				func() error {
					if len(compact.condenseCandidates) != 2 {
						t.Fatalf("unrelated boundary merged live candidates: %+v", compact.condenseCandidates)
					}
					return nil
				},
			)
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, ok := compact.CanonicalBoundary(3, 1, false, 0)
	if !ok || canonical.Node != unrelatedNode {
		t.Fatalf("unrelated canonical changed: got=%+v/%t want node=%d", canonical, ok, unrelatedNode)
	}
}

func TestStage2PhysicalHeadMergePreservesLineageMetadata(t *testing.T) {
	compact, left, right, _, _ := newStage2PhysicalHeadPair(t)
	leftSet := NewAlternativeSetMember(11, 1)
	rightSet := NewAlternativeSetMember(12, 2)
	leftRefs := g18RefSetFrom(t, compact, g18Ref(1, 1, 1, 0))
	rightRefs := g18RefSetFrom(t, compact, g18Ref(2, 1, 1, 0))
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if err := compact.RecordHeadOwnerOwned(owner, left, 101); err != nil {
			return err
		}
		if err := compact.RecordHeadOwnerOwned(owner, right, 202); err != nil {
			return err
		}
		if err := compact.RecordHeadLineageOwned(owner, left, CleanPathRankSelected, 11, leftSet, false, true, leftRefs); err != nil {
			return err
		}
		if err := compact.RecordHeadLineageOwned(owner, right, CleanPathRankUnselected, 12, rightSet, false, true, rightRefs); err != nil {
			return err
		}
		return compact.ReindexCondenseCandidatesOwned(owner, []CondenseCandidate{{Head: left}, {Head: right}})
	})
	if err != nil {
		t.Fatal(err)
	}
	merged, ok := compact.CanonicalBoundary(3, 1, false, 0)
	if !ok {
		t.Fatal("merged lineage boundary is unavailable")
	}
	record, err := compact.nodeLineage(merged.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.owner != 0 {
		t.Fatalf("new merged node owner=%d, want canonicalization ownership", record.owner)
	}
	if !record.converged || record.rank != CleanPathRankUnknown || record.lineage != 0 || !record.blended {
		t.Fatalf("merged scalar lineage=%+v", *record)
	}
	members, ok := compact.AlternativeSetMembers(record.set)
	if !ok || len(members) != 2 {
		t.Fatalf("merged alternative set=%v/%t", members, ok)
	}
	refs := g18Members(t, compact, record.dropCohortRefs)
	if len(refs) != 2 || refs[0] != g18Ref(1, 1, 1, 0) || refs[1] != g18Ref(2, 1, 1, 0) {
		t.Fatalf("merged drop-cohort refs=%v", refs)
	}
}

func TestStage2PhysicalHeadMergePersistsCandidateDropCohortReferences(t *testing.T) {
	compact, left, right, _, _ := newStage2PhysicalHeadPair(t)
	leftRef := g18Ref(1, 1, 1, 0)
	rightRef := g18Ref(2, 1, 1, 0)
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.ReindexCondenseCandidatesOwned(owner, []CondenseCandidate{
			{Head: left, DropCohortRefs: g18RefSetFrom(t, compact, leftRef)},
			{Head: right, DropCohortRefs: g18RefSetFrom(t, compact, rightRef)},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	merged, ok := compact.CanonicalBoundary(3, 1, false, 0)
	if !ok {
		t.Fatal("candidate reference boundary is unavailable")
	}
	record, err := compact.nodeLineage(merged.Node)
	if err != nil {
		t.Fatal(err)
	}
	refs := g18Members(t, compact, record.dropCohortRefs)
	if len(refs) != 2 || refs[0] != leftRef || refs[1] != rightRef {
		t.Fatalf("candidate references=%v", refs)
	}
}

func TestStage2PhysicalHeadMergeAppliesHigherPrecedenceReplacement(t *testing.T) {
	compact, root := newDiagnosticShallowFoldCore(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	incumbentPayload := appendShallowPayload(t, compact, shallowPayloadSpec{
		symbol: 20, productionID: 1, startByte: 12, endByte: 17, childSymbols: []Symbol{30},
	})
	incomingPayload := appendShallowPayload(t, compact, shallowPayloadSpec{
		symbol: 20, productionID: 2, startByte: 12, endByte: 17, childSymbols: []Symbol{31},
	})
	leftID, err := compact.appendAdjacencyNode(3, 17, []linkRecord{{prev: root.Node, payload: incumbentPayload, scoreDelta: 2}})
	if err != nil {
		t.Fatal(err)
	}
	rightID, err := compact.appendAdjacencyNode(3, 17, []linkRecord{{prev: root.Node, payload: incomingPayload, scoreDelta: 3}})
	if err != nil {
		t.Fatal(err)
	}
	before := compact.Work()
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.ReindexCondenseCandidatesOwned(owner, []CondenseCandidate{
			{Head: Head{Node: leftID}}, {Head: Head{Node: rightID}},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	merged, ok := compact.CanonicalBoundary(3, 17, false, 0)
	if !ok {
		t.Fatal("precedence boundary is unavailable")
	}
	links, err := compact.nodeLinks(*mustNode(t, compact, merged.Node))
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].payload != incomingPayload || links[0].scoreDelta != 3 {
		t.Fatalf("precedence replacement links=%+v", links)
	}
	after := compact.Work()
	if after.PredecessorLinkUnionPrecedenceReplaced-before.PredecessorLinkUnionPrecedenceReplaced != 1 {
		t.Fatalf("precedence replacement telemetry before=%+v after=%+v", before, after)
	}
}

func TestStage2PhysicalHeadMergePreservesRecursivePrecedenceMaximum(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	leftRoot, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rightRoot, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftPayload := appendShallowPayload(t, compact, shallowPayloadSpec{symbol: 20, endByte: 1, childSymbols: []Symbol{22}})
	rightPayload := appendShallowPayload(t, compact, shallowPayloadSpec{symbol: 21, endByte: 1, childSymbols: []Symbol{23}})
	leftPrev, err := compact.appendAdjacencyNode(3, 1, []linkRecord{{prev: leftRoot.Node, payload: leftPayload, scoreDelta: 2}})
	if err != nil {
		t.Fatal(err)
	}
	rightPrev, err := compact.appendAdjacencyNode(3, 1, []linkRecord{{prev: rightRoot.Node, payload: rightPayload, scoreDelta: 7}})
	if err != nil {
		t.Fatal(err)
	}
	topPayload := appendShallowPayload(t, compact, shallowPayloadSpec{symbol: 30, startByte: 1, endByte: 2})
	leftTop, err := compact.appendAdjacencyNode(4, 2, []linkRecord{{prev: leftPrev, payload: topPayload}})
	if err != nil {
		t.Fatal(err)
	}
	rightTop, err := compact.appendAdjacencyNode(4, 2, []linkRecord{{prev: rightPrev, payload: topPayload}})
	if err != nil {
		t.Fatal(err)
	}
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return compact.ReindexCondenseCandidatesOwned(owner, []CondenseCandidate{
			{Head: Head{Node: leftTop}}, {Head: Head{Node: rightTop}},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	merged, ok := compact.CanonicalBoundary(4, 2, false, 0)
	if !ok {
		t.Fatal("recursive precedence boundary is unavailable")
	}
	maximum, err := compact.nodePrecedenceMaximum(merged.Node)
	if err != nil {
		t.Fatal(err)
	}
	if maximum.value != 7 {
		t.Fatalf("recursive precedence maximum=%d, want 7", maximum.value)
	}
	if compact.Work().PredecessorLinkUnionRecursiveChanged == 0 {
		t.Fatalf("recursive precedence path did not execute: %+v", compact.Work())
	}
}

func TestStage2PhysicalHeadMergePreservesRecursiveDuplicateLineage(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	root, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload := appendShallowPayload(t, compact, shallowPayloadSpec{symbol: 20, endByte: 1})
	leftID, err := compact.appendAdjacencyNode(3, 1, []linkRecord{{prev: root.Node, payload: payload}})
	if err != nil {
		t.Fatal(err)
	}
	rightID, err := compact.appendAdjacencyNode(3, 1, []linkRecord{{prev: root.Node, payload: payload}})
	if err != nil {
		t.Fatal(err)
	}
	left := Head{Node: leftID}
	right := Head{Node: rightID}
	leftSet := NewAlternativeSetMember(21, 1)
	rightSet := NewAlternativeSetMember(22, 2)
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if err := compact.RecordHeadLineageOwned(owner, left, CleanPathRankSelected, 21, leftSet, false, true); err != nil {
			return err
		}
		if err := compact.RecordHeadLineageOwned(owner, right, CleanPathRankUnselected, 22, rightSet, false, true); err != nil {
			return err
		}
		merged, changed, err := compact.mergePredecessorsBounded(
			leftID, rightID, 0, &precedenceMaximumWitness{},
		)
		if err != nil {
			return err
		}
		if changed || merged != leftID {
			t.Fatalf("duplicate recursive topology merged=%d changed=%t", merged, changed)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := compact.nodeLineage(leftID)
	if err != nil {
		t.Fatal(err)
	}
	members, ok := compact.AlternativeSetMembers(record.set)
	if !ok || len(members) != 2 || record.rank != CleanPathRankUnknown ||
		record.lineage != 0 || !record.converged || !record.blended {
		t.Fatalf("recursive duplicate lineage=%+v members=%v/%t", *record, members, ok)
	}
}

func mustNode(t *testing.T, compact *Core, id NodeID) *nodeRecord {
	t.Helper()
	node, err := compact.node(id)
	if err != nil {
		t.Fatal(err)
	}
	return node
}
