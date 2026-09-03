//go:build gts_parsercorephase0

package parsercorephase0

import (
	"testing"
	"unsafe"
)

// Keep this value aligned with the first free link flag after linkFlagHasOrder.
const s5RecoveryDiscontinuityFlag uint32 = 1 << 1

func appendS5SyntheticNode(t *testing.T, core *Core, state StateID, byteOffset uint32, link linkRecord, pathCount uint64) (NodeID, LinkID) {
	t.Helper()
	linkID := core.appendGraphLink(link)
	node, err := core.appendNodeRecord(nodeRecord{
		state: state, byteOffset: byteOffset,
		firstLink: uint32(linkID), linkCount: 1, pathCount: pathCount,
	}, core.checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	return node, linkID
}

func newS5DiscontinuityFixture(t *testing.T) (*Core, Head, Head, linkRecord) {
	t.Helper()
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 16, MaxPopPaths: 16})
	seed, err := core.Seed(17, 0)
	if err != nil {
		t.Fatal(err)
	}
	link := linkRecord{prev: seed.Node, flags: s5RecoveryDiscontinuityFlag}
	node, linkID := appendS5SyntheticNode(t, core, 0, 0, link, 1)
	return core, seed, Head{Node: node}, core.links[linkID-1]
}

func TestS5RecoveryDiscontinuityLinkValidation(t *testing.T) {
	if got := unsafe.Sizeof(linkRecord{}); got != 32 {
		t.Fatalf("linkRecord size=%d, want 32", got)
	}
	if got := unsafe.Sizeof(subtreeRecord{}); got != 44 {
		t.Fatalf("subtreeRecord size=%d, want 44", got)
	}

	tests := []struct {
		name      string
		makeLink  func(seed NodeID, payload SubtreeID) linkRecord
		wantError bool
	}{
		{
			name: "sanctioned null edge",
			makeLink: func(seed NodeID, _ SubtreeID) linkRecord {
				return linkRecord{prev: seed, flags: s5RecoveryDiscontinuityFlag}
			},
		},
		{
			name: "ordinary zero payload",
			makeLink: func(seed NodeID, _ SubtreeID) linkRecord {
				return linkRecord{prev: seed}
			},
			wantError: true,
		},
		{
			name: "flagged nonzero payload",
			makeLink: func(seed NodeID, payload SubtreeID) linkRecord {
				return linkRecord{prev: seed, payload: payload, flags: s5RecoveryDiscontinuityFlag}
			},
			wantError: true,
		},
		{
			name: "flagged score",
			makeLink: func(seed NodeID, _ SubtreeID) linkRecord {
				return linkRecord{prev: seed, scoreDelta: 1, flags: s5RecoveryDiscontinuityFlag}
			},
			wantError: true,
		},
		{
			name: "flagged branch order",
			makeLink: func(seed NodeID, _ SubtreeID) linkRecord {
				return linkRecord{prev: seed, order: 1, flags: s5RecoveryDiscontinuityFlag | linkFlagHasOrder}
			},
			wantError: true,
		},
		{
			name: "unknown flag",
			makeLink: func(seed NodeID, _ SubtreeID) linkRecord {
				return linkRecord{prev: seed, flags: s5RecoveryDiscontinuityFlag | 1<<2}
			},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
			seed, err := core.Seed(17, 0)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := core.appendSubtree(subtreeRecord{
				symbol: 91, startByte: 0, endByte: 1, terminal: true,
			}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			link := test.makeLink(seed.Node, payload)
			linkID := core.appendGraphLink(link)
			next := NodeID(len(core.nodes) + 1)
			err = core.validatePublishedNodeDAG(nodeRecord{
				state: 0, byteOffset: 0, firstLink: uint32(linkID), linkCount: 1, pathCount: 1,
			}, next)
			if test.wantError && err == nil {
				t.Fatalf("link=%+v was accepted", link)
			}
			if !test.wantError && err != nil {
				t.Fatalf("link=%+v was rejected: %v", link, err)
			}
		})
	}
}

func TestS5DiscontinuityReaderSemantics(t *testing.T) {
	t.Run("precedence maximum omits null payload", func(t *testing.T) {
		core, seed, _, link := newS5DiscontinuityFixture(t)
		got, err := core.linkPrecedenceMaximum(link)
		if err != nil {
			t.Fatalf("linkPrecedenceMaximum: %v", err)
		}
		if got.value != 0 {
			t.Fatalf("precedence maximum=%d, want predecessor maximum 0 from seed %d", got.value, seed.Node)
		}
	})

	t.Run("derivations omit null payload", func(t *testing.T) {
		core, seed, head, _ := newS5DiscontinuityFixture(t)
		derivations, err := core.Derivations(head)
		if err != nil {
			t.Fatalf("Derivations: %v", err)
		}
		if len(derivations) != 1 {
			t.Fatalf("derivations=%d, want one path through seed %d", len(derivations), seed.Node)
		}
		path := derivations[0]
		if len(path.Payloads) != 0 || path.Score != 0 || path.HasBranchOrder {
			t.Fatalf("null-edge derivation=%+v, want no payload, score, or order", path)
		}
	})

	t.Run("pop counts the slot without a child", func(t *testing.T) {
		core, seed, head, _ := newS5DiscontinuityFixture(t)
		paths, err := core.popPaths(head.Node, 1)
		if err != nil {
			t.Fatalf("popPaths: %v", err)
		}
		if len(paths) != 1 {
			t.Fatalf("pop paths=%d, want one path through seed %d", len(paths), seed.Node)
		}
		path := paths[0]
		if path.prev != seed.Node || len(path.children) != 0 || len(path.trailing) != 0 || path.score != 0 || path.order.Present {
			t.Fatalf("null-edge pop path=%+v, want one structural slot and no payload", path)
		}
	})

	t.Run("summary depth counts the slot", func(t *testing.T) {
		core, seed, head, _ := newS5DiscontinuityFixture(t)
		candidates, err := core.StackSummaryCandidates(head, 1)
		if err != nil {
			t.Fatalf("StackSummaryCandidates: %v", err)
		}
		if len(candidates) != 1 {
			t.Fatalf("summary candidates=%d, want one entry for seed %d", len(candidates), seed.Node)
		}
		candidate := candidates[0]
		if candidate.Depth() != 1 || candidate.State() != 17 || candidate.ByteOffset() != 0 {
			t.Fatalf("summary candidate depth/state/byte=%d/%d/%d, want 1/17/0", candidate.Depth(), candidate.State(), candidate.ByteOffset())
		}
	})

	t.Run("edge identity includes the discontinuity flag", func(t *testing.T) {
		core, seed, _, link := newS5DiscontinuityFixture(t)
		ordinary := linkRecord{prev: seed.Node}
		if core.linkEdgesEqual(link, ordinary) {
			t.Fatalf("flagged link=%+v compared equal to ordinary link=%+v", link, ordinary)
		}
	})
}

func newS5SixHeadCandidates(t *testing.T) (*Core, []CondenseCandidate, map[SubtreeID]bool, SubtreeID) {
	t.Helper()
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 32, MaxPopPaths: 32})
	candidates := make([]CondenseCandidate, 0, 7)
	wantPayloads := make(map[SubtreeID]bool, 6)
	for index := 0; index < 6; index++ {
		seed, err := core.Seed(StateID(index+1), 0)
		if err != nil {
			t.Fatal(err)
		}
		payload := appendShallowPayload(t, core, shallowPayloadSpec{
			symbol: Symbol(index + 100), startByte: 0, endByte: 1,
		})
		predecessor, err := core.appendAdjacencyNode(400, 0, []linkRecord{{prev: seed.Node, payload: payload}})
		if err != nil {
			t.Fatal(err)
		}
		head, _ := appendS5SyntheticNode(t, core, 0, 0, linkRecord{
			prev: predecessor, flags: s5RecoveryDiscontinuityFlag,
		}, 1)
		candidates = append(candidates, CondenseCandidate{
			Head: Head{Node: head}, MergeIdentity: 41,
		})
		wantPayloads[payload] = true
	}

	missingSeed, err := core.Seed(99, 0)
	if err != nil {
		t.Fatal(err)
	}
	missingPayload, err := core.appendSubtree(subtreeRecord{
		symbol: 900, startByte: 0, endByte: 1, missing: true, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	missingHead, err := core.appendAdjacencyNode(500, 1, []linkRecord{{prev: missingSeed.Node, payload: missingPayload}})
	if err != nil {
		t.Fatal(err)
	}
	candidates = append(candidates, CondenseCandidate{
		Head: Head{Node: missingHead}, MergeIdentity: 42,
	})
	return core, candidates, wantPayloads, missingPayload
}

func TestS5SixHeadPhysicalMergeRetainsPaths(t *testing.T) {
	core, candidates, wantPayloads, missingPayload := newS5SixHeadCandidates(t)
	before := core.Work()
	err := core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return core.RunSchedulerOwned(owner, func() error {
			context := RecoveryDiscontinuityContext{ErrorState: 0, ByteOffset: 0, Checkpoint: 0}
			absorber := candidates[0].Head
			for index := 1; index < 6; index++ {
				merged, err := core.MergeRecoveryDiscontinuityHeadsOwned(owner, context, absorber, candidates[index].Head)
				if err != nil {
					return err
				}
				absorber = merged
			}
			missing := candidates[6].Head
			absorberStats, err := core.Stats(absorber)
			if err != nil {
				return err
			}
			missingStats, err := core.Stats(missing)
			if err != nil {
				return err
			}
			if absorberStats.CurrentExactPaths != 6 || missingStats.CurrentExactPaths != 1 {
				return &s5AssertionError{message: "recovery merge lost one lineage"}
			}
			derivations, err := core.Derivations(absorber)
			if err != nil {
				return err
			}
			if len(derivations) != 6 {
				return &s5AssertionError{message: "absorber derivation count is not six"}
			}
			for _, derivation := range derivations {
				if len(derivation.Payloads) != 1 || !wantPayloads[derivation.Payloads[0]] {
					return &s5AssertionError{message: "absorber lost a predecessor payload or exposed subtree zero"}
				}
				delete(wantPayloads, derivation.Payloads[0])
			}
			if len(wantPayloads) != 0 {
				return &s5AssertionError{message: "absorber lost one of six predecessor paths"}
			}
			missingDerivations, err := core.Derivations(missing)
			if err != nil {
				return err
			}
			if len(missingDerivations) != 1 || len(missingDerivations[0].Payloads) != 1 || missingDerivations[0].Payloads[0] != missingPayload {
				return &s5AssertionError{message: "missing lineage did not remain separate"}
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("six-head physical merge: %v", err)
	}
	after := core.Work()
	if after.PhysicalHeadMergeAttempts-before.PhysicalHeadMergeAttempts != 5 ||
		after.PhysicalHeadMergeSuccesses-before.PhysicalHeadMergeSuccesses != 5 ||
		after.PhysicalHeadMergeInputLinks-before.PhysicalHeadMergeInputLinks != 5 {
		t.Fatalf("physical merge telemetry=%+v before=%+v, want five attempts, successes, and input links", after, before)
	}
}

type s5AssertionError struct{ message string }

func (e *s5AssertionError) Error() string { return e.message }
