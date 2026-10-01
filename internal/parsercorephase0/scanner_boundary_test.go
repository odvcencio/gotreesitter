package parsercorephase0

import (
	"errors"
	"testing"
)

func TestEmptyScannerBoundaryRollsBackAndResets(t *testing.T) {
	c, head, reused := reusedFixture(t)
	reused.ScannerExact = true
	abort := errors.New("rollback witness")
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, payload, err := c.PushReusedSubtreeOwned(owner, head, reused)
		if err != nil {
			return err
		}
		pair, ok := c.externalPayloadScannerProvenance(payload)
		if !ok || pair.start != 0 || pair.end != 0 || !c.subtrees[payload-1].externalProvenanceState.reused() {
			t.Fatalf("borrowed empty pair = %+v, %t", pair, ok)
		}
		return abort
	})
	if !errors.Is(err, abort) || c.SubtreeCount() != 0 {
		t.Fatalf("rollback: subtrees=%d err=%v", c.SubtreeCount(), err)
	}
	reused.ScannerExact = false
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, payload, err := c.PushReusedSubtreeOwned(owner, head, reused)
		if err != nil {
			return err
		}
		if _, ok := c.externalPayloadScannerProvenance(payload); ok {
			t.Fatal("rolled-back empty pair authenticated an opaque subtree")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	c.EnableTerminalScannerCheckpointProvenance()
	id, err := c.appendAuthenticatedTerminal(subtreeRecord{symbol: 1, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.externalPayloadScannerProvenance(id); ok {
		t.Fatal("reset retained an empty scanner receipt")
	}
}

func TestEmptyScannerBoundaryRequiresAuthenticatedPair(t *testing.T) {
	c, _, _ := reusedFixture(t)
	c.EnableTerminalScannerCheckpointProvenance()
	unproven, err := c.appendAuthenticatedTerminal(subtreeRecord{symbol: 1, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.externalPayloadScannerProvenance(unproven); ok {
		t.Fatal("absent scanner proof became an empty pair")
	}
	if err := c.SetPhaseExternalTokenScannerCheckpoints(0, 0); err != nil {
		t.Fatal(err)
	}
	leaf, err := c.appendAuthenticatedTerminal(subtreeRecord{symbol: 2, terminal: true, external: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := c.appendSubtreeRecord(subtreeRecord{symbol: 100}, []SubtreeID{leaf}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []SubtreeID{leaf, parent} {
		pair, ok := c.externalPayloadScannerProvenance(id)
		if !ok || pair.payload != id || pair.start != 0 || pair.end != 0 {
			t.Fatalf("empty pair %d = %+v, %t", id, pair, ok)
		}
		if _, exact, err := c.subtreeExternalProvenance(id); err != nil || !exact {
			t.Fatalf("external proof %d exact=%t: %v", id, exact, err)
		}
	}
	if len(c.externalProvenance) != 0 {
		t.Fatalf("empty pairs allocated %d sidecar rows", len(c.externalProvenance))
	}
	// Cache refreshes must preserve the independent empty-pair receipt.
	c.subtrees[parent-1].externalProvenanceState = subtreeScannerEmptyPair
	if _, exact, err := c.subtreeExternalProvenance(parent); err != nil || !exact {
		t.Fatalf("refreshed external proof exact=%t: %v", exact, err)
	}
	if _, ok := c.externalPayloadScannerProvenance(parent); !ok {
		t.Fatal("cache refresh lost the empty pair")
	}
}

func TestEmptyScannerBoundaryMixesWithNonemptyStates(t *testing.T) {
	c, _, _ := reusedFixture(t)
	c.EnableTerminalScannerCheckpointProvenance()
	inside, err := c.InternCheckpoint([]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	var leaves []SubtreeID
	for _, endpoints := range [][2]CheckpointID{{0, 0}, {0, inside}, {inside, 0}, {0, 0}} {
		if err := c.SetPhaseExternalTokenScannerCheckpoints(endpoints[0], endpoints[1]); err != nil {
			t.Fatal(err)
		}
		id, err := c.appendAuthenticatedTerminal(subtreeRecord{symbol: 1, terminal: true}, 0)
		if err != nil {
			t.Fatal(err)
		}
		leaves = append(leaves, id)
	}
	parent, err := c.appendSubtreeRecord(subtreeRecord{symbol: 100}, leaves, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	pair, ok := c.externalPayloadScannerProvenance(parent)
	if !ok || pair.start != 0 || pair.end != 0 || len(c.externalProvenance) != 2 {
		t.Fatalf("mixed boundary = %+v, %t; rows=%d", pair, ok, len(c.externalProvenance))
	}
	// The incomplete child prevents even an empty parent receipt.
	unproven, err := c.appendSubtreeRecord(subtreeRecord{symbol: 2, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := c.appendSubtreeRecord(subtreeRecord{symbol: 101}, []SubtreeID{leaves[0], unproven}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.externalPayloadScannerProvenance(unknown); ok {
		t.Fatal("incomplete child supplied a parent scanner receipt")
	}
}

func TestReductionScannerBoundaryIncludesHiddenZeroWidthTransition(t *testing.T) {
	c, _, _ := reusedFixture(t)
	c.EnableTerminalScannerCheckpointProvenance()
	inside, err := c.InternCheckpoint([]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	outside, err := c.InternCheckpoint([]byte{2})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetPhaseExternalTokenScannerCheckpoints(inside, inside); err != nil {
		t.Fatal(err)
	}
	body, err := c.appendAuthenticatedTerminal(subtreeRecord{symbol: 1, startByte: 0, endByte: 10, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetPhaseExternalTokenScannerCheckpoints(inside, outside); err != nil {
		t.Fatal(err)
	}
	hidden, err := c.appendAuthenticatedTerminal(subtreeRecord{symbol: 2, startByte: 10, endByte: 10, terminal: true, external: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := c.appendSubtreeRecord(subtreeRecord{symbol: 100, startByte: 0, endByte: 10}, []SubtreeID{body, hidden}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	view, err := c.MaterializationView(parent)
	if err != nil || !view.ExternalScannerCheckpointExact || view.ExternalScannerCheckpointStart != inside || view.ExternalScannerCheckpointEnd != outside {
		t.Fatalf("boundary = %+v, %v", view, err)
	}
	// A later projection can hide the zero-width leaf; the parent receipt must
	// continue to carry its end transition instead of the body's end state.
	outer, err := c.appendSubtreeRecord(subtreeRecord{symbol: 101, startByte: 0, endByte: 10}, []SubtreeID{parent}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	pair, ok := c.externalPayloadScannerProvenance(outer)
	if !ok || pair.end != outside {
		t.Fatalf("outer pair = %+v, %t", pair, ok)
	}
}
