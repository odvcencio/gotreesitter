package parsercorephase0

import (
	"fmt"
	"reflect"
	"testing"
)

func checkpointReserveFixture(t *testing.T) *Core {
	t.Helper()
	c := reserveTestCore(t, reserveTestLimits())
	for n := 0; n < 100; n++ {
		// Deliberate hash collisions exercise complete interner chains across growth.
		if _, err := c.checkpoints.internHash([]byte(fmt.Sprintf("state-%03d", n)), uint64(n%7)); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func TestCheckpointReservePreservesIDsBytesReceiptsAndCollisions(t *testing.T) {
	c := checkpointReserveFixture(t)
	beforeRecords := append([]checkpointRecord(nil), c.checkpoints.records...)
	beforeBytes := append([]byte(nil), c.checkpoints.bytes...)
	beforeStats, work := c.CheckpointInternerStats(), c.Work()
	digests := make([][32]byte, len(beforeRecords))
	for n := range digests {
		_, digests[n], _ = c.checkpoints.receipt(CheckpointID(n + 1))
	}
	if !c.growCheckpointBuffersFromObservedDensity(10000, 625, 1<<20) {
		t.Fatal("observed capacity not reserved")
	}
	if !reflect.DeepEqual(c.checkpoints.records, beforeRecords) || !reflect.DeepEqual(c.checkpoints.bytes, beforeBytes) || c.CheckpointInternerStats() != beforeStats || c.Work() != work {
		t.Fatal("reserve changed live state or work")
	}
	for n, expected := range digests {
		id, err := c.checkpoints.internHash([]byte(fmt.Sprintf("state-%03d", n)), uint64(n%7))
		_, digest, exact := c.checkpoints.receipt(id)
		if err != nil || id != CheckpointID(n+1) || !exact || digest != expected {
			t.Fatalf("identity or receipt changed at %d: id=%d err=%v", n, id, err)
		}
	}
	if c.CheckpointInternerStats() != beforeStats {
		t.Fatal("reserve changed collision or identity counts")
	}
	if n := testing.AllocsPerRun(10, func() { c.growCheckpointBuffersFromObservedDensity(10000, 625, 1<<20) }); n != 0 {
		t.Fatalf("warm reserve allocs=%g", n)
	}
}

func TestCheckpointReserveBudgetAndCapacityAccounting(t *testing.T) {
	c := checkpointReserveFixture(t)
	before := c.FootprintBytes()
	records, bytes := cap(c.checkpoints.records), cap(c.checkpoints.bytes)
	if c.growCheckpointBuffersFromObservedDensity(10000, 625, 1) || c.FootprintBytes() != before {
		t.Fatal("spent budget changed capacity")
	}
	if !c.growCheckpointBuffersFromObservedDensity(10000, 625, 1<<20) {
		t.Fatal("reserve refused sufficient budget")
	}
	want := uint64(cap(c.checkpoints.records)-records)*coreCheckpointRecordBytes + uint64(cap(c.checkpoints.bytes)-bytes)
	if got := c.FootprintBytes() - before; got != want || got == 0 {
		t.Fatalf("capacity charge=%d want=%d", got, want)
	}
	retained := c.checkpoints.footprintBytes()
	c.checkpoints.reset()
	if c.checkpoints.footprintBytes() > retained || cap(c.checkpoints.records) == 0 || cap(c.checkpoints.bytes) == 0 {
		t.Fatal("reset lost retained capacity accounting")
	}
	c.checkpoints.dropOversized()
	if c.checkpoints.footprintBytes() != 0 {
		t.Fatal("trim retained checkpoint storage")
	}
}

func TestCheckpointReserveRejectsInvalidProgressAndHonorsLimits(t *testing.T) {
	c := checkpointReserveFixture(t)
	before := c.FootprintBytes()
	for _, v := range []struct {
		s int
		p uint32
	}{{0, 1}, {-1, 1}, {1, 1}, {10000, 0}, {10000, 624}, {10000, 10000}, {10000, 10001}} {
		if c.growCheckpointBuffersFromObservedDensity(v.s, v.p, 0) || c.FootprintBytes() != before {
			t.Fatalf("invalid progress allocated: %+v", v)
		}
	}
	c.checkpoints.maxIDs = 250
	c.checkpoints.maxBytes = 2000
	if !c.growCheckpointBuffersFromObservedDensity(10000, 625, 1<<20) {
		t.Fatal("limited reserve refused")
	}
	if cap(c.checkpoints.records) > 250 || cap(c.checkpoints.bytes) > 2000 {
		t.Fatal("reserve exceeded interner limits")
	}
	if (*Core)(nil).growCheckpointBuffersFromObservedDensity(10000, 625, 0) {
		t.Fatal("nil core reserved")
	}
}
