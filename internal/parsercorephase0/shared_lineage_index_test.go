package parsercorephase0

import (
	"errors"
	"testing"
)

func TestSharedLineageIndexCollisionRollbackAcrossTableEnd(t *testing.T) {
	c := newSharedLineageCore(t)
	head, _ := c.Seed(1, 0)
	var records []nodeLineageRecord
	for cost := uint32(1); len(records) < 4; cost++ {
		record := nodeLineageRecord{storedErrorCost: cost}
		if sharedLineageHash(record)&15 == 15 {
			records = append(records, record)
		}
	}
	for _, record := range records[:2] {
		if err := c.storeNodeLineage(head.Node, record); err != nil {
			t.Fatal(err)
		}
	}
	abort := errors.New("rollback")
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		for _, record := range records[2:] {
			if err := c.storeNodeLineage(head.Node, record); err != nil {
				return err
			}
		}
		return abort
	})
	if !errors.Is(err, abort) || c.sharedLineageEntries() != 2 {
		t.Fatalf("rollback: %v, entries=%d", err, c.sharedLineageEntries())
	}
	for i, record := range records {
		reference := c.lookupSharedLineage(record)
		if (reference != 0) != (i < 2) {
			t.Fatalf("record %d resolves to %d after rollback", i, reference)
		}
	}
	if c.sharedLineageSlots()[15] == 0 || c.sharedLineageSlots()[0] == 0 {
		t.Fatal("fixture did not wrap its probe chain")
	}
}

func TestSharedLineageIndexGrowthAndResetPreserveExactHistory(t *testing.T) {
	c := newSharedLineageCore(t)
	head, _ := c.Seed(1, 0)
	for cost := uint32(1); cost <= 10000; cost++ {
		if err := c.storeNodeLineage(head.Node, nodeLineageRecord{storedErrorCost: cost}); err != nil {
			t.Fatal(err)
		}
	}
	for cost := uint32(1); cost <= 10000; cost++ {
		ref := c.lookupSharedLineage(nodeLineageRecord{storedErrorCost: cost})
		if ref != cost {
			t.Fatalf("cost %d resolves to %d", cost, ref)
		}
	}
	if bytes := uint64(cap(c.sharedLineageSlots())) * coreUint32Bytes; bytes >= 10000*80/4 {
		t.Fatalf("index retains %d bytes", bytes)
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = c.storeNodeLineage(head.Node, nodeLineageRecord{storedErrorCost: 42}) }); allocs != 0 {
		t.Fatalf("warm history allocated %g", allocs)
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	if c.sharedLineageEntries() != 0 || c.lookupSharedLineage(nodeLineageRecord{storedErrorCost: 42}) != 0 {
		t.Fatal("reset retained a history reference")
	}
}
