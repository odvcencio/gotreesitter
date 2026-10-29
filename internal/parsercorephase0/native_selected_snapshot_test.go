package parsercorephase0

import "testing"

func TestNativeSelectedSnapshotOwnsReplay(t *testing.T) {
	c, root, replay, scratch := nativeSelectedFixture(t)
	snapshot, err := c.BuildNativeSelectedSnapshot([]SubtreeID{root}, []byte("x"), replay, scratch, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Release()
	if cap(c.selectedBuild.precedence) != 0 {
		t.Fatal("single-root publication allocated an unused precedence plane")
	}
	if scratch.reservePublicOutput {
		t.Fatal("public reserve policy escaped its build scope")
	}
	id := snapshot.Store().Root()
	before, symbol, ok := snapshot.Record(id)
	if !ok || !before.ReplayPreKnown() || !before.ReplayStateKnown() || symbol != before.Symbol {
		t.Fatalf("missing replay: %+v", before)
	}
	if cap(scratch.replay) != 0 {
		t.Fatal("producer still owns replay backing")
	}
	second, err := c.BuildNativeSelectedSnapshot([]SubtreeID{root}, []byte("x"), replay, scratch, nil)
	if err != nil {
		t.Fatal(err)
	}
	second.Release()
	if err = c.Reset(); err != nil {
		t.Fatal(err)
	}
	after, _, ok := snapshot.Record(id)
	if !ok || before != after {
		t.Fatal("producer reuse changed replay proof")
	}
	if snapshot.RetainedBytes() <= snapshot.Store().RetainedBytes() {
		t.Fatal("replay bytes not accounted")
	}
	snapshot.Release()
	if _, _, ok = snapshot.Record(id); ok {
		t.Fatal("released snapshot is readable")
	}
	if snapshot.RetainedBytes() != 0 {
		t.Fatal("release retained backing")
	}
}

func TestNativeSelectedSnapshotCopiedHandleCannotRecycleLiveReplay(t *testing.T) {
	c, root, replay, scratch := nativeSelectedFixture(t)
	first, err := c.BuildNativeSelectedSnapshot([]SubtreeID{root}, []byte("x"), replay, scratch, nil)
	if err != nil {
		t.Fatal(err)
	}
	copied := first
	first.Release()
	replay.RootPre = 7
	second, err := c.BuildNativeSelectedSnapshot([]SubtreeID{root}, []byte("x"), replay, scratch, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
	before, _, _ := second.Record(second.Store().Root())
	copied.Release()
	if copied.RetainedBytes() != 0 {
		t.Fatal("released copy retains replay")
	}
	replay.RootPre = 17
	third, err := c.BuildNativeSelectedSnapshot([]SubtreeID{root}, []byte("x"), replay, scratch, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Release()
	after, _, _ := second.Record(second.Store().Root())
	if before != after {
		t.Fatal("copied handle recycled a live snapshot's replay")
	}
}
