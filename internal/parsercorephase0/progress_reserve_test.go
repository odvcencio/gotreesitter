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

func TestProgressGrowthPreservesUsedVisibleCountCache(t *testing.T) {
	c := reserveTestCore(t, reserveTestLimits())
	head, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	id, err := c.appendSubtree(subtreeRecord{symbol: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	symbols := []SelectedSymbolPolicy{{}, {Visible: true, Named: true}}
	before, err := c.CachedVisibleSubtreeCount(symbols, id)
	if err != nil {
		t.Fatal(err)
	}
	row := c.recoveryVisibleCounts[id-1]
	stats, err := c.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if !c.GrowRecordArenasForProgress(1000, 10, 1<<20) {
		t.Fatal("observed density did not grow")
	}
	if cap(c.recoveryVisibleCounts) < cap(c.subtrees) || c.recoveryVisibleCounts[id-1] != row {
		t.Fatal("growth lost the published count or left the used cache unreserved")
	}
	after, err := c.CachedVisibleSubtreeCount(symbols, id)
	if err != nil || after != before {
		t.Fatalf("cached count changed: %d -> %d, %v", before, after, err)
	}
	got, err := c.Stats(head)
	if err != nil || got != stats {
		t.Fatalf("growth changed work: %+v -> %+v, %v", stats, got, err)
	}
}

func TestProgressGrowthChargesUsedVisibleCountCacheBeforeAllocation(t *testing.T) {
	c := reserveTestCore(t, reserveTestLimits())
	id, err := c.appendSubtree(subtreeRecord{symbol: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CachedVisibleSubtreeCount([]SelectedSymbolPolicy{{}, {Visible: true}}, id); err != nil {
		t.Fatal(err)
	}
	before := c.FootprintBytes()
	// The projected subtree arena fits, but its already-used count cache does not.
	budget := before + 107*coreSubtreeRecordBytes + 16
	subtreeCap, cacheCap := cap(c.subtrees), cap(c.recoveryVisibleCounts)
	if c.GrowRecordArenasForProgress(1000, 10, budget) || c.FootprintBytes() != before ||
		cap(c.subtrees) != subtreeCap || cap(c.recoveryVisibleCounts) != cacheCap {
		t.Fatal("insufficient transient budget allocated projected sidecars")
	}
}
