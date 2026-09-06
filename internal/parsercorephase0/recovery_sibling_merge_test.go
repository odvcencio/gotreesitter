package parsercorephase0

import (
	"errors"
	"reflect"
	"testing"
)

func TestRecoverySiblingMergePreservesIncumbentAndPublishes(t *testing.T) {
	for _, changed := range []bool{false, true} {
		name := "unchanged_tie"
		if changed {
			name = "distinct_payloads"
		}
		t.Run(name, func(t *testing.T) {
			compact, sibling, incoming, payload := recoverySiblingMergeFixture(t, changed)
			var merged Head
			err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				var err error
				merged, err = compact.MergeEquivalentRecoverySiblingHeadsOwned(owner, 3, 1, 0, false, sibling, incoming, recoverySiblingPayloadHasError)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			canonical, ok := compact.CanonicalBoundary(3, 1, false, 0)
			if !ok || canonical != merged {
				t.Fatalf("canonical=%+v/%t, merged=%+v", canonical, ok, merged)
			}
			paths, err := compact.Derivations(merged)
			if err != nil {
				t.Fatal(err)
			}
			if changed {
				if merged == sibling || merged == incoming || len(paths) != 2 {
					t.Fatalf("changed merge=%+v paths=%+v", merged, paths)
				}
			} else if merged != sibling || len(paths) != 1 || len(paths[0].Payloads) != 1 || paths[0].Payloads[0] != payload {
				t.Fatalf("tie lost incumbent: merged=%+v paths=%+v", merged, paths)
			}
			lineage, err := compact.nodeLineage(merged.Node)
			if err != nil || lineage.storedErrorCost != 10 || lineage.rank != CleanPathRankUnknown {
				t.Fatalf("merged lineage=%+v err=%v", lineage, err)
			}
		})
	}
}

func TestRecoverySiblingMergeRollbackRestoresPublicationAndLineage(t *testing.T) {
	for _, changed := range []bool{false, true} {
		compact, sibling, incoming, _ := recoverySiblingMergeFixture(t, changed)
		beforeLineages := append([]nodeLineageRecord(nil), compact.nodeLineages...)
		beforeNodes, beforeLinks, beforeWork := len(compact.nodes), len(compact.links), compact.Work()
		abort := errors.New("abort sibling merge")
		err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			if _, err := compact.MergeEquivalentRecoverySiblingHeadsOwned(owner, 3, 1, 0, false, sibling, incoming, recoverySiblingPayloadHasError); err != nil {
				return err
			}
			return abort
		})
		canonical, ok := compact.CanonicalBoundary(3, 1, false, 0)
		if !errors.Is(err, abort) || !ok || canonical != incoming || len(compact.nodes) != beforeNodes || len(compact.links) != beforeLinks || compact.Work() != beforeWork || !reflect.DeepEqual(compact.nodeLineages, beforeLineages) {
			t.Fatalf("rollback changed=%t err=%v canonical=%+v/%t", changed, err, canonical, ok)
		}
	}
}

func TestRecoverySiblingMergeRejectsUnownedBoundaryAndUnequalCosts(t *testing.T) {
	for _, reason := range []string{"boundary", "cost", "checkpoint", "callback"} {
		t.Run(reason, func(t *testing.T) {
			compact, sibling, incoming, _ := recoverySiblingMergeFixture(t, false)
			callback := PayloadErrorPresenceFunc(recoverySiblingPayloadHasError)
			checkpoint := CheckpointID(0)
			switch reason {
			case "boundary":
				other, err := compact.appendNode(nodeRecord{state: 3, byteOffset: 1})
				if err != nil {
					t.Fatal(err)
				}
				if err := compact.writeBoundary(compact.boundaryKey(3, 1), other); err != nil {
					t.Fatal(err)
				}
			case "cost":
				compact.nodeLineages[incoming.Node-1].storedErrorCost++
			case "checkpoint":
				checkpoint = 1
			case "callback":
				callback = nil
			}
			beforeLineages := append([]nodeLineageRecord(nil), compact.nodeLineages...)
			beforeNodes, beforeLinks, beforeWork := len(compact.nodes), len(compact.links), compact.Work()
			beforeCanonical, beforeOK := compact.CanonicalBoundary(3, 1, false, 0)
			err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				_, err := compact.MergeEquivalentRecoverySiblingHeadsOwned(owner, 3, 1, checkpoint, false, sibling, incoming, callback)
				return err
			})
			canonical, ok := compact.CanonicalBoundary(3, 1, false, 0)
			if err == nil || ok != beforeOK || canonical != beforeCanonical || len(compact.nodes) != beforeNodes || len(compact.links) != beforeLinks || compact.Work() != beforeWork || !reflect.DeepEqual(compact.nodeLineages, beforeLineages) {
				t.Fatalf("rejection changed state: err=%v canonical=%+v/%t", err, canonical, ok)
			}
		})
	}
}

func TestRecoverySiblingMergeKeepsExistingAPICanonicalRestriction(t *testing.T) {
	compact, sibling, incoming, _ := recoverySiblingMergeFixture(t, true)
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, err := compact.MergeEquivalentRecoveryHeadsOwned(owner, 3, 1, 0, false, sibling, incoming, recoverySiblingPayloadHasError)
		return err
	})
	if err == nil || err.Error() != "parser-core phase zero: physical head merge lost its incumbent boundary" {
		t.Fatalf("existing API error=%v", err)
	}
}

func recoverySiblingPayloadHasError(SubtreeID) (bool, error) { return true, nil }

func recoverySiblingMergeFixture(t *testing.T, distinctSymbol bool) (*Core, Head, Head, SubtreeID) {
	t.Helper()
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	heads := make([]Head, 2)
	var firstPayload SubtreeID
	for i := range heads {
		symbol := Symbol(10)
		if i == 1 && distinctSymbol {
			symbol++
		}
		payload, err := compact.appendSubtree(subtreeRecord{symbol: symbol, startByte: uint32(i), endByte: 1, terminal: true}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstPayload = payload
		}
		link := compact.appendGraphLink(linkRecord{prev: seed.Node, payload: payload})
		node, err := compact.appendNode(nodeRecord{state: 3, byteOffset: 1, firstLink: uint32(link), linkCount: 1, pathCount: 1})
		if err != nil {
			t.Fatal(err)
		}
		heads[i] = Head{Node: node}
		compact.nodeLineages[node-1].storedErrorCost = 10
		if err := compact.recordNodeLineage(heads[i], CleanPathRankSelected, uint16(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if err := compact.writeBoundary(compact.boundaryKey(3, 1), heads[1].Node); err != nil {
		t.Fatal(err)
	}
	return compact, heads[0], heads[1], firstPayload
}
