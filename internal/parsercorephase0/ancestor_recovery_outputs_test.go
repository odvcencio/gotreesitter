package parsercorephase0

import (
	"errors"
	"reflect"
	"testing"
)

func TestPluralAncestorRecoverySuppressesExcessLiveIterators(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{MaxLinksPerBoundary: 128, MaxPopPaths: 128})
	var links []linkRecord
	var payloads []SubtreeID
	for i := 0; i < 70; i++ {
		seed, err := c.Seed(7, uint32(i))
		if err != nil {
			t.Fatal(err)
		}
		payload := appendAncestorRecoveryPayload(t, c, 1, uint32(i), 100, false)
		payloads = append(payloads, payload)
		links = append(links, linkRecord{prev: seed.Node, payload: payload})
	}
	id, err := c.appendAdjacencyNode(20, 100, links)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := c.StackSummaryCandidates(Head{Node: id}, 1)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates=%v err=%v", candidates, err)
	}
	var outputs []Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		outputs, err = c.RecoverToAncestorStateOutputsWithCostOwned(owner, candidates[0], func(NodeID, SubtreeID) (uint32, error) { return 100, nil })
		return err
	})
	if err != nil || len(outputs) != 64 {
		t.Fatalf("outputs=%d err=%v", len(outputs), err)
	}
	for i, output := range outputs {
		paths, err := c.Derivations(output)
		if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 1 {
			t.Fatalf("paths=%v err=%v", paths, err)
		}
		view, err := c.MaterializationView(paths[0].Payloads[0])
		if err != nil || !reflect.DeepEqual(view.Children, []SubtreeID{payloads[i]}) {
			t.Fatalf("output %d children=%v err=%v", i, view.Children, err)
		}
	}
}

func TestPluralAncestorRecoveryMarkerCopiesSourceCheckpoint(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	old := mustInternCheckpoint(t, c, []byte("old"))
	current := mustInternCheckpoint(t, c, []byte("current"))
	if err := c.SetPhaseCheckpoint(old); err != nil {
		t.Fatal(err)
	}
	seed, err := c.Seed(7, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetPhaseCheckpoint(current); err != nil {
		t.Fatal(err)
	}
	// A prior empty ERROR changes the live scanner checkpoint. Recovery
	// removes it and reaches the historical target at the old checkpoint.
	prior, err := c.appendSubtree(subtreeRecord{symbol: ErrorRegionSymbol, extra: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	currentHead, err := c.appendPrivate(7, 0, linkInput{prev: seed.Node, payload: prior})
	if err != nil {
		t.Fatal(err)
	}
	var marker Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		marker, err = c.AppendRecoveryDiscontinuityOwned(owner, currentHead, RecoveryDiscontinuityContext{Checkpoint: current})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := c.StackSummaryCandidates(marker, 1)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates=%v err=%v", candidates, err)
	}
	var outputs []Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		outputs, err = c.RecoverToAncestorStateOutputsWithCostOwned(owner, candidates[0], func(NodeID, SubtreeID) (uint32, error) { t.Fatal("empty recovery priced a payload"); return 0, nil })
		return err
	})
	if err != nil || len(outputs) != 1 || outputs[0] == seed {
		t.Fatalf("outputs=%v seed=%v err=%v", outputs, seed, err)
	}
	if got, ok := c.nodeScannerCheckpoint(outputs[0].Node); !ok || got != current {
		t.Fatalf("checkpoint=%d want=%d valid=%t", got, current, ok)
	}
	if got, ok := c.nodeScannerCheckpoint(seed.Node); !ok || got != old {
		t.Fatalf("target checkpoint changed: %d valid=%t", got, ok)
	}
}

func TestPluralAncestorRecoveryPopCapPoisonsSwallowedError(t *testing.T) {
	c, candidate, _ := pluralRecoveryFixture(t)
	c.limits.MaxPopPaths = 1
	before := captureSchedulerTransactionState(c)
	var innerErr error
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, innerErr = c.RecoverToAncestorStateOutputsWithCostOwned(owner, candidate, func(NodeID, SubtreeID) (uint32, error) { t.Fatal("pop cap reached publication"); return 0, nil })
		return nil
	})
	if innerErr == nil || err == nil {
		t.Fatalf("swallowed pop cap: inner=%v outer=%v", innerErr, err)
	}
	if after := captureSchedulerTransactionState(c); !reflect.DeepEqual(before, after) {
		t.Fatal("pop cap changed storage")
	}
	assertTransactionJournalClean(t, c)
}

func pluralRecoveryFixture(t *testing.T) (*Core, StackSummaryCandidate, []SubtreeID) {
	t.Helper()
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	if err := c.SetPhaseCheckpoint(mustInternCheckpoint(t, c, []byte("recovery-checkpoint"))); err != nil {
		t.Fatal(err)
	}
	left, err := c.Seed(7, 0)
	if err != nil {
		t.Fatal(err)
	}
	right, err := c.Seed(7, 1)
	if err != nil {
		t.Fatal(err)
	}
	payloads := []SubtreeID{
		appendAncestorRecoveryPayload(t, c, 1, 0, 2, false),
		appendAncestorRecoveryPayload(t, c, 2, 1, 2, false),
	}
	key := c.shiftedBoundaryKey(20, 2)
	_, err = c.condense(key, linkInput{prev: left.Node, payload: payloads[0], scoreDelta: 3, order: ForkOrder{Present: true, Value: 9}})
	if err != nil {
		t.Fatal(err)
	}
	head, err := c.condense(key, linkInput{prev: right.Node, payload: payloads[1], scoreDelta: 5, order: ForkOrder{Present: true, Value: 11}})
	if err != nil {
		t.Fatal(err)
	}
	alternate := appendAncestorRecoveryPayload(t, c, 4, 0, 2, false)
	head, err = c.condense(key, linkInput{prev: left.Node, payload: alternate})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := c.StackSummaryCandidates(head, 1)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("summary=%v err=%v", candidates, err)
	}
	return c, candidates[0], payloads
}

func TestPluralAncestorRecoveryPreservesBothPaths(t *testing.T) {
	for _, open := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "open"}[open], func(t *testing.T) {
			c, candidate, payloads := pluralRecoveryFixture(t)
			var region []SubtreeID
			if open {
				region = []SubtreeID{appendAncestorRecoveryPayload(t, c, 3, 2, 3, false)}
			}
			var outputs []Head
			cost := func(NodeID, SubtreeID) (uint32, error) { return 100, nil }
			err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				var err error
				if open {
					outputs, err = c.RecoverToAncestorStateOutputsWithOpenRegionAndCostOwned(owner, candidate, 2, 3, region, cost)
				} else {
					outputs, err = c.RecoverToAncestorStateOutputsWithCostOwned(owner, candidate, cost)
				}
				return err
			})
			if err != nil || len(outputs) != 2 || outputs[0] == outputs[1] {
				t.Fatalf("outputs=%v err=%v", outputs, err)
			}
			for i, head := range outputs {
				if checkpoint, ok := c.nodeScannerCheckpoint(head.Node); !ok || checkpoint != c.checkpoint {
					t.Fatalf("output checkpoint=%d want=%d valid=%t", checkpoint, c.checkpoint, ok)
				}
				paths, err := c.Derivations(head)
				if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 1 {
					t.Fatalf("paths=%v err=%v", paths, err)
				}
				view, err := c.MaterializationView(paths[0].Payloads[0])
				if err != nil {
					t.Fatal(err)
				}
				want := append([]SubtreeID{payloads[i]}, region...)
				if !reflect.DeepEqual(flattenRecoveryRepeats(t, c, view.Children), want) {
					t.Fatalf("output %d children=%v want=%v", i, view.Children, want)
				}
				if paths[0].Score != []int64{3, 5}[i] {
					t.Fatalf("output %d score=%d", i, paths[0].Score)
				}
				if !paths[0].HasBranchOrder || paths[0].BranchOrder != []uint64{9, 11}[i] {
					t.Fatalf("output %d branch order=%+v", i, paths[0])
				}
				stored, err := c.RecoveryStoredErrorCost(head)
				if err != nil || stored != 100 {
					t.Fatalf("cost=%d err=%v", stored, err)
				}
			}
		})
	}
}

func TestPluralAncestorRecoveryOrdersUnequalExtraPaths(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	left, err := c.Seed(7, 0)
	if err != nil {
		t.Fatal(err)
	}
	right, err := c.Seed(7, 1)
	if err != nil {
		t.Fatal(err)
	}
	child := appendAncestorRecoveryPayload(t, c, 1, 0, 1, false)
	extra := appendAncestorRecoveryPayload(t, c, 2, 1, 2, true)
	other := appendAncestorRecoveryPayload(t, c, 3, 1, 2, false)
	left = appendAncestorRecoveryHead(t, c, left, 20, child)
	key := c.shiftedBoundaryKey(30, 2)
	_, err = c.condense(key, linkInput{prev: left.Node, payload: extra})
	if err != nil {
		t.Fatal(err)
	}
	head, err := c.condense(key, linkInput{prev: right.Node, payload: other})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := c.StackSummaryCandidates(head, 1)
	if err != nil {
		t.Fatal(err)
	}
	candidate := ancestorRecoveryCandidateForState(t, candidates, 7)
	var outputs []Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		outputs, err = c.RecoverToAncestorStateOutputsWithCostOwned(owner, candidate, func(NodeID, SubtreeID) (uint32, error) { return 100, nil })
		return err
	})
	if err != nil || len(outputs) != 2 {
		t.Fatalf("outputs=%v err=%v", outputs, err)
	}
	for i, want := range []SubtreeID{other, child} {
		paths, err := c.Derivations(outputs[i])
		if err != nil || len(paths) != 1 {
			t.Fatalf("paths=%v err=%v", paths, err)
		}
		view, err := c.MaterializationView(paths[0].Payloads[0])
		if err != nil || !reflect.DeepEqual(view.Children, []SubtreeID{want}) {
			t.Fatalf("output %d ERROR=%+v err=%v", i, view, err)
		}
		if i == 1 && (len(paths[0].Payloads) != 2 || paths[0].Payloads[1] != extra) {
			t.Fatalf("trailing extra lost: %+v", paths)
		}
	}
}

func TestPluralAncestorRecoveryFlattensPriorError(t *testing.T) {
	for _, open := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "open"}[open], func(t *testing.T) {
			c, candidate, region, _, want := priorErrorAncestorFixture(t)
			var outputs []Head
			err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				var err error
				cost := func(NodeID, SubtreeID) (uint32, error) { return 800, nil }
				if open {
					outputs, err = c.RecoverToAncestorStateOutputsWithOpenRegionAndCostOwned(owner, candidate, 2, 3, []SubtreeID{region}, cost)
				} else {
					outputs, err = c.RecoverToAncestorStateOutputsWithCostOwned(owner, candidate, cost)
				}
				return err
			})
			if err != nil || len(outputs) != 1 {
				t.Fatalf("outputs=%v err=%v", outputs, err)
			}
			paths, err := c.Derivations(outputs[0])
			if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 1 {
				t.Fatalf("paths=%v err=%v", paths, err)
			}
			view, err := c.MaterializationView(paths[0].Payloads[0])
			if !open {
				want = want[:2]
			}
			if err != nil || !reflect.DeepEqual(flattenRecoveryRepeats(t, c, view.Children), want) {
				t.Fatalf("ERROR=%+v want=%v err=%v", view, want, err)
			}
		})
	}
}

func TestPluralAncestorRecoverySelectsFirstPriorErrorHistory(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	seed, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var target Head
	var firstChild SubtreeID
	for _, symbol := range []Symbol{1, 2} {
		child := appendAncestorRecoveryPayload(t, c, symbol, 0, 1, false)
		if firstChild == 0 {
			firstChild = child
		}
		prior, err := c.appendSubtree(subtreeRecord{symbol: ErrorRegionSymbol, startByte: 0, endByte: 1, extra: true}, []SubtreeID{child}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		target, err = c.condense(c.shiftedBoundaryKey(7, 1), linkInput{prev: seed.Node, payload: prior})
		if err != nil {
			t.Fatal(err)
		}
	}
	child := appendAncestorRecoveryPayload(t, c, 3, 1, 2, false)
	head := appendAncestorRecoveryHead(t, c, target, 20, child)
	candidates, err := c.StackSummaryCandidates(head, 1)
	if err != nil {
		t.Fatal(err)
	}
	candidate := ancestorRecoveryCandidateForState(t, candidates, 7)
	var outputs []Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		outputs, err = c.RecoverToAncestorStateOutputsWithCostOwned(owner, candidate, func(NodeID, SubtreeID) (uint32, error) { return 100, nil })
		return err
	})
	if err != nil || len(outputs) != 1 {
		t.Fatalf("outputs=%v err=%v", outputs, err)
	}
	paths, err := c.Derivations(outputs[0])
	if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 1 {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
	view, err := c.MaterializationView(paths[0].Payloads[0])
	if err != nil || !reflect.DeepEqual(view.Children, []SubtreeID{firstChild, child}) {
		t.Fatalf("first prior ERROR=%+v err=%v", view, err)
	}
}

func TestPluralAncestorRecoveryRepushesExtrasWithoutErrorChildren(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	seed, err := c.Seed(7, 0)
	if err != nil {
		t.Fatal(err)
	}
	var marker Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		marker, err = c.AppendRecoveryDiscontinuityOwned(owner, seed, RecoveryDiscontinuityContext{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	extra := appendAncestorRecoveryPayload(t, c, 1, 0, 1, true)
	head := appendAncestorRecoveryHead(t, c, marker, 0, extra)
	candidates, err := c.StackSummaryCandidates(head, 1)
	if err != nil {
		t.Fatal(err)
	}
	candidate := ancestorRecoveryCandidateForState(t, candidates, 7)
	var outputs []Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		outputs, err = c.RecoverToAncestorStateOutputsWithCostOwned(owner, candidate, func(NodeID, SubtreeID) (uint32, error) { return 0, nil })
		return err
	})
	if err != nil || len(outputs) != 1 {
		t.Fatalf("outputs=%v err=%v", outputs, err)
	}
	paths, err := c.Derivations(outputs[0])
	if err != nil || len(paths) != 1 || !reflect.DeepEqual(paths[0].Payloads, []SubtreeID{extra}) {
		t.Fatalf("trailing extras=%v err=%v", paths, err)
	}
}

func TestPluralAncestorRecoveryRollsBackEveryOutput(t *testing.T) {
	for _, panicFault := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panicFault], func(t *testing.T) {
			c, candidate, _ := pluralRecoveryFixture(t)
			before := captureSchedulerTransactionState(c)
			failure := errors.New("second recovery output fault")
			calls := 0
			var outputs []Head
			var caught any
			var err error
			func() {
				defer func() { caught = recover() }()
				err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
					var err error
					outputs, err = c.RecoverToAncestorStateOutputsWithCostOwned(owner, candidate, func(NodeID, SubtreeID) (uint32, error) {
						calls++
						if calls == 1 {
							return 100, nil
						}
						if panicFault {
							panic(failure)
						}
						return 0, failure
					})
					return err
				})
			}()
			if calls != 2 || len(outputs) != 0 || (panicFault && caught != failure) || (!panicFault && !errors.Is(err, failure)) {
				t.Fatalf("calls=%d outputs=%v panic=%v err=%v", calls, outputs, caught, err)
			}
			if after := captureSchedulerTransactionState(c); !reflect.DeepEqual(before, after) {
				t.Fatal("recovery rollback changed core storage")
			}
			assertTransactionJournalClean(t, c)
		})
	}
}
