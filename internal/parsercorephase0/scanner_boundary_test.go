package parsercorephase0

import "testing"

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
