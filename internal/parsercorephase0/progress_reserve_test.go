package parsercorephase0

import "testing"

func TestProgressGrowthPreservesPublishedIdentityAndWork(t *testing.T) {
	c := reserveTestCore(t, reserveTestLimits())
	head, err := c.Seed(12, 7)
	if err != nil {
		t.Fatal(err)
	}
	before := c.nodes[0]
	stats, err := c.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	storage := c.StorageBytes()
	if !c.GrowRecordArenasForProgress(1000, 10, 1<<20) {
		t.Fatal("observed density did not grow")
	}
	if c.nodes[0] != before || len(c.nodes) != 1 || c.StorageBytes() != storage {
		t.Fatal("growth changed a published record")
	}
	after, err := c.Stats(head)
	if err != nil || after != stats {
		t.Fatalf("growth changed head work: %+v %+v %v", stats, after, err)
	}
	state, offset, err := c.Boundary(head)
	if err != nil || state != 12 || offset != 7 {
		t.Fatalf("growth invalidated identity: %d %d %v", state, offset, err)
	}
}

func TestProgressGrowthHonorsTransientBudgetAndRecordLimits(t *testing.T) {
	c := reserveTestCore(t, reserveTestLimits())
	if _, err := c.Seed(0, 0); err != nil {
		t.Fatal(err)
	}
	before, capacity := c.FootprintBytes(), cap(c.nodes)
	if c.GrowRecordArenasForProgress(1000, 1, before+1) || c.FootprintBytes() != before || cap(c.nodes) != capacity {
		t.Fatal("insufficient budget allocated replacement arrays")
	}
	c.limits.MaxNodes = 8
	if !c.GrowRecordArenasForProgress(1000, 1, 1<<20) || cap(c.nodes) != 8 || cap(c.nodeLineages) != 8 {
		t.Fatal("density growth exceeded or failed to reach record limit")
	}
	for _, progress := range []uint32{0, 1000, 1001} {
		if c.GrowRecordArenasForProgress(1000, progress, 1<<20) {
			t.Fatalf("invalid progress %d grew", progress)
		}
	}
}
