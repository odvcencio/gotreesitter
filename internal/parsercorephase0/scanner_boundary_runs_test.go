package parsercorephase0

import (
	"errors"
	"testing"
)

func TestScannerBoundaryRunsRetainPresenceAndRollbackExtensions(t *testing.T) {
	c, _, _ := reusedFixture(t)
	c.EnableTerminalScannerCheckpointProvenance()
	appendPair := func(start, end CheckpointID, proven bool) SubtreeID {
		t.Helper()
		id, err := c.appendSubtreeRecord(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if proven {
			c.recordScannerBoundary(id, start, end)
		}
		return id
	}
	first := appendPair(1, 2, true)
	for i := 0; i < 1000; i++ {
		appendPair(1, 2, true)
	}
	if len(c.externalProvenance) != 1 {
		t.Fatalf("identical pairs use %d rows", len(c.externalProvenance))
	}
	before := c.lastExternalProvenanceRun()
	abort := errors.New("rollback")
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		appendPair(1, 2, true)
		appendPair(2, 3, true)
		return abort
	})
	if !errors.Is(err, abort) || c.lastExternalProvenanceRun() != before {
		t.Fatal("rollback changed the preceding scanner run")
	}
	for id := first; id <= before.payload; id++ {
		pair, ok := c.externalPayloadScannerProvenance(id)
		if !ok || pair.payload != id || pair.start != 1 || pair.end != 2 {
			t.Fatalf("payload %d: %+v, %t", id, pair, ok)
		}
	}
	gap := appendPair(0, 0, false)
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		appendPair(1, 2, true)
		return abort
	})
	if !errors.Is(err, abort) || c.lastExternalProvenanceRun() != before {
		t.Fatal("rollback extended the preceding run across a gap")
	}
	last := appendPair(1, 2, true)
	if len(c.externalProvenance) != 2 {
		t.Fatal("a scanner run crossed an unauthenticated gap")
	}
	if _, ok := c.externalPayloadScannerProvenance(gap); ok {
		t.Fatal("a run authenticated the gap")
	}
	if pair, ok := c.externalPayloadScannerProvenance(last); !ok || pair.start != 1 || pair.end != 2 {
		t.Fatal("a gap hid the following authenticated boundary")
	}
}
