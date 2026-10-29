package parsercorephase0

import (
	"reflect"
	"strconv"
	"testing"
)

func observedArenaCapacity(c *Core) uint64 {
	return c.nodes.FootprintBytes() + uint64(cap(c.nodeLineages))*coreNodeLineageRecordBytes + c.links.FootprintBytes() + c.subtrees.FootprintBytes() + uint64(cap(c.children))*coreChildRecordBytes
}
func TestObservedRecordDensityPreservesRecordsAndWork(t *testing.T) {
	c := projectionFixture(t)
	nodes := observedNodeRecords(c)
	lineages := append([]nodeLineageHot(nil), c.nodeLineages...)
	links := c.links.Values()
	subtrees := observedSubtreeRecords(c)
	children := append([]SubtreeID(nil), c.children...)
	storage, work := c.StorageBytes(), c.Work()
	if !c.GrowRecordArenasFromObservedDensity(10000, 625, 1<<20) {
		t.Fatal("observed projection did not grow")
	}
	if !reflect.DeepEqual(observedNodeRecords(c), nodes) || !reflect.DeepEqual(c.nodeLineages, lineages) || !reflect.DeepEqual(c.links.Values(), links) || !reflect.DeepEqual(observedSubtreeRecords(c), subtrees) || !reflect.DeepEqual(c.children, children) {
		t.Fatal("projection changed records")
	}
	if c.StorageBytes() != storage || c.Work() != work {
		t.Fatal("projection changed logical storage or work")
	}
	// A hundred observed nodes at 1/16 source progress projects to 1700,
	// whereas the source-based reserve projects to 7500 before its ceiling.
	if c.nodes.Capacity() != 1700 || c.links.Cap() != 1530 || c.subtrees.Capacity() != 1360 || cap(c.children) != 1275 {
		t.Fatalf("density mixture lost: %d/%d/%d/%d", c.nodes.Capacity(), c.links.Cap(), c.subtrees.Capacity(), cap(c.children))
	}
	if got := testing.AllocsPerRun(10, func() { c.GrowRecordArenasFromObservedDensity(10000, 625, 1<<20) }); got != 0 {
		t.Fatalf("warm projection allocs=%g", got)
	}
}
func TestObservedRecordDensityCapsTotalCapacityIncludingSkew(t *testing.T) {
	c := projectionFixture(t)
	c.links.Grow(5000)
	before := observedArenaCapacity(c)
	ceiling := before + 1024
	if !c.GrowRecordArenasFromObservedDensity(10000, 625, ceiling) {
		t.Fatal("bounded growth refused")
	}
	if after := observedArenaCapacity(c); after <= before || after > ceiling {
		t.Fatalf("total capacity %d -> %d exceeds %d", before, after, ceiling)
	}
	if c.links.Cap() != 5000 {
		t.Fatal("projection changed retained larger capacity")
	}
	before = observedArenaCapacity(c)
	if c.GrowRecordArenasFromObservedDensity(10000, 625, before) || observedArenaCapacity(c) != before {
		t.Fatal("spent total ceiling changed capacity")
	}
	if c.GrowRecordArenasFromObservedDensity(10000, 625, before+1) || observedArenaCapacity(c) != before {
		t.Fatal("one byte admitted partial growth")
	}
}
func TestObservedRecordDensityRejectsInvalidProgress(t *testing.T) {
	c := projectionFixture(t)
	before := c.FootprintBytes()
	inputs := []struct {
		source   int
		progress uint32
	}{{-1, 1}, {0, 1}, {1, 1}, {10000, 0}, {10000, 624}, {10000, 10000}, {10000, 10001}}
	if strconv.IntSize == 64 {
		source := uint64(1) << 32
		inputs = append(inputs, struct {
			source   int
			progress uint32
		}{int(source), 1 << 28})
	}
	for _, in := range inputs {
		if c.GrowRecordArenasFromObservedDensity(in.source, in.progress, 0) {
			t.Fatalf("invalid input %+v", in)
		}
	}
	if (*Core)(nil).GrowRecordArenasFromObservedDensity(10000, 625, 0) {
		t.Fatal("nil core projected")
	}
	if c.FootprintBytes() != before {
		t.Fatal("invalid progress allocated")
	}
}
func TestObservedRecordDensityHonorsLimitsOnLargestSource(t *testing.T) {
	c := projectionFixture(t)
	c.limits.MaxNodes, c.limits.MaxLinks, c.limits.MaxSubtrees, c.limits.MaxChildren = 120, 110, 100, 90
	source := int(1<<31 - 1)
	if strconv.IntSize == 64 {
		value := uint64(1<<32 - 1)
		source = int(value)
	}
	if !c.GrowRecordArenasFromObservedDensity(source, uint32(source/16), 0) {
		t.Fatal("bounded projection refused")
	}
	if c.nodes.Capacity() > 120 || cap(c.nodeLineages) > 120 || c.links.Cap() > 110 || c.subtrees.Capacity() > 100 || cap(c.children) > 90 {
		t.Fatal("record limits exceeded")
	}
}

func observedNodeRecords(c *Core) []nodeRecord {
	records := make([]nodeRecord, c.nodes.Len())
	for i := range records {
		records[i] = *c.nodes.At(i)
	}
	return records
}
func observedSubtreeRecords(c *Core) []subtreeRecord {
	records := make([]subtreeRecord, c.subtrees.Len())
	for i := range records {
		records[i] = *c.subtrees.At(i)
	}
	return records
}
func TestObservedDensityChargesBlockMetadataBeforeGrowth(t *testing.T) {
	c := projectionFixture(t)
	c.limits.MaxNodes = 200000
	for c.nodes.Len() < 5000 {
		if _, ok := c.nodes.Append(nodeRecord{state: 1}, 200000); !ok {
			t.Fatal("fixture nodes")
		}
	}
	c.nodes.Grow(nodeRecordBlockSize)
	c.nodeLineages = growArena(c.nodeLineages, 85000)
	c.links.Grow(1530)
	c.subtrees.Grow(1360)
	c.children = growArena(c.children, 1275)
	before := observedArenaCapacity(c)
	if c.GrowRecordArenasFromObservedDensity(10000, 625, before+coreNodeRecordBytes) || observedArenaCapacity(c) != before {
		t.Fatal("slot-only budget admitted uncharged block metadata")
	}
}
