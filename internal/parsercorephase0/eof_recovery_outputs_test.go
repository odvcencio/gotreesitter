package parsercorephase0

import (
	"errors"
	"reflect"
	"testing"
)

func TestRecoverEOFOutputsPreserveAllPathsAndOrder(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{MaxPopPaths: 128, MaxLinksPerBoundary: 128})
	checkpoint := mustInternCheckpoint(t, c, []byte("source"))
	if err := c.SetPhaseCheckpoint(checkpoint); err != nil {
		t.Fatal(err)
	}
	seed, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var links []linkRecord
	var want []SubtreeID
	for i := 0; i < 2; i++ {
		leaf := appendAncestorRecoveryPayload(t, c, Symbol(i+1), 0, 1, i == 1)
		want = append(want, leaf)
		links = append(links, linkRecord{prev: seed.Node, payload: leaf, scoreDelta: int64(i + 3), order: uint64(i + 9), flags: linkFlagHasOrder})
	}
	id, err := c.appendAdjacencyNode(0, 1, links)
	if err != nil {
		t.Fatal(err)
	}
	otherCheckpoint := mustInternCheckpoint(t, c, []byte("unrelated"))
	if err := c.SetPhaseCheckpoint(otherCheckpoint); err != nil {
		t.Fatal(err)
	}
	var outputs []Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		outputs, err = c.RecoverEOFAcceptOutputsWithOpenRegionAndCostOwned(owner, Head{Node: id}, 1, 1, nil, func(NodeID, SubtreeID) (uint32, error) { return 700, nil })
		return err
	})
	if err != nil || len(outputs) != 2 {
		t.Fatalf("outputs=%v err=%v", outputs, err)
	}
	for i, head := range outputs {
		paths, err := c.Derivations(head)
		if err != nil || len(paths) != 1 || len(paths[0].Payloads) != 1 {
			t.Fatalf("paths=%v err=%v", paths, err)
		}
		root := paths[0].Payloads[0]
		view, err := c.MaterializationView(root)
		if err != nil || !reflect.DeepEqual(view.Children, []SubtreeID{want[i]}) || view.Extra || !c.IsRecoverEOFAcceptRoot(root) || paths[0].Score != int64(i+3) {
			t.Fatalf("output %d view=%+v paths=%+v err=%v", i, view, paths, err)
		}
		if got, _ := c.nodeScannerCheckpoint(head.Node); got != checkpoint {
			t.Fatal("EOF output lost source checkpoint")
		}
		if !paths[0].HasBranchOrder || paths[0].BranchOrder != uint64(i+9) {
			t.Fatalf("lost branch order: %+v", paths[0])
		}
		if got, err := c.RecoveryStoredErrorCost(head); err != nil || got != 700 {
			t.Fatalf("cost=%d err=%v", got, err)
		}
	}
	before := len(c.subtrees)
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		calls := 0
		_, err := c.RecoverEOFAcceptOutputsWithOpenRegionAndCostOwned(owner, Head{Node: id}, 1, 1, nil, func(NodeID, SubtreeID) (uint32, error) {
			calls++
			if calls == 2 {
				return 0, errors.New("second output failed")
			}
			return 1, nil
		})
		return err
	})
	if err == nil || len(c.subtrees) != before {
		t.Fatal("plural EOF failure retained partial outputs")
	}
}

func TestRecoverEOFOutputsSuppressExcessLiveIterators(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{MaxPopPaths: 128, MaxLinksPerBoundary: 128})
	seed, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var links []linkRecord
	for i := 0; i < 70; i++ {
		leaf := appendAncestorRecoveryPayload(t, c, Symbol(i+1), 0, 1, false)
		links = append(links, linkRecord{prev: seed.Node, payload: leaf})
	}
	id, err := c.appendAdjacencyNode(0, 1, links)
	if err != nil {
		t.Fatal(err)
	}
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		outputs, err := c.RecoverEOFAcceptOutputsWithOpenRegionAndCostOwned(owner, Head{Node: id}, 1, 1, nil, func(NodeID, SubtreeID) (uint32, error) { return 1, nil })
		if err != nil {
			return err
		}
		if len(outputs) != 64 {
			t.Fatalf("outputs=%d, want 64", len(outputs))
		}
		for i, head := range outputs {
			paths, err := c.Derivations(head)
			if err != nil {
				return err
			}
			root, err := c.Subtree(paths[0].Payloads[0])
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(root.Children, []SubtreeID{links[i].payload}) {
				t.Fatalf("suppressed path published at %d: %v", i, root.Children)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecoverEOFOutputsGroupPathsByTerminalSeed(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	left, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	right, err := c.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	a := appendAncestorRecoveryPayload(t, c, 1, 0, 2, false)
	b := appendAncestorRecoveryPayload(t, c, 2, 0, 2, false)
	c1 := appendAncestorRecoveryPayload(t, c, 3, 0, 1, false)
	extra := appendAncestorRecoveryPayload(t, c, 4, 1, 2, true)
	middle := appendAncestorRecoveryHead(t, c, left, 8, c1)
	id, err := c.appendAdjacencyNode(0, 2, []linkRecord{{prev: left.Node, payload: a}, {prev: right.Node, payload: b}, {prev: middle.Node, payload: extra}})
	if err != nil {
		t.Fatal(err)
	}
	var outputs []Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		outputs, err = c.RecoverEOFAcceptOutputsWithOpenRegionAndCostOwned(owner, Head{Node: id}, 2, 2, nil, func(NodeID, SubtreeID) (uint32, error) { return 1, nil })
		return err
	})
	if err != nil || len(outputs) != 3 {
		t.Fatalf("outputs=%v err=%v", outputs, err)
	}
	for i, want := range [][]SubtreeID{{a}, {c1, extra}, {b}} {
		paths, err := c.Derivations(outputs[i])
		if err != nil {
			t.Fatal(err)
		}
		root, err := c.Subtree(paths[0].Payloads[0])
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(root.Children, want) {
			t.Fatalf("slice %d children=%v want=%v", i, root.Children, want)
		}
	}
}

func TestRecoverEOFOutputsRollbackPoisonAndPanic(t *testing.T) {
	for _, mode := range []string{"cost", "panic", "pop-cap"} {
		t.Run(mode, func(t *testing.T) {
			c, candidate, _ := pluralRecoveryFixture(t)
			if mode == "pop-cap" {
				c.limits.MaxPopPaths = 1
			}
			before := captureSchedulerTransactionState(c)
			var gotErr error
			panicked := false
			func() {
				defer func() {
					if v := recover(); v != nil {
						panicked = true
					}
				}()
				gotErr = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
					calls := 0
					_, _ = c.RecoverEOFAcceptOutputsWithOpenRegionAndCostOwned(owner, Head{Node: candidate.source}, 2, 2, nil, func(NodeID, SubtreeID) (uint32, error) {
						calls++
						if calls == 2 {
							if mode == "panic" {
								panic("second EOF output")
							}
							return 0, errors.New("second EOF output")
						}
						return 1, nil
					})
					return nil
				})
			}()
			if mode == "panic" && !panicked || mode != "panic" && gotErr == nil {
				t.Fatalf("mode=%s panic=%v err=%v", mode, panicked, gotErr)
			}
			if !reflect.DeepEqual(before, captureSchedulerTransactionState(c)) {
				t.Fatal("failed EOF publication retained storage")
			}
			assertTransactionJournalClean(t, c)
		})
	}
}
