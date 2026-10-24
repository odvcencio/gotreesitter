package parsercorephase0

import (
	"reflect"
	"strconv"
	"testing"
)

func projectionFixture(t *testing.T) *Core {
	t.Helper()
	c := reserveTestCore(t, reserveTestLimits())
	c.nodes = nodeBlocksFromSliceForTest(make([]nodeRecord, 100))
	c.nodeLineages = make([]nodeLineageHot, 100)
	c.links = makeLinkRecordStorage(90, 90)
	c.subtrees = subtreeBlocksFromSliceForTest(make([]subtreeRecord, 80))
	c.children = make([]SubtreeID, 75)
	for i := range c.nodes.Len() {
		(*c.nodes.At(int(i))).byteOffset = uint32(i)
		c.nodeLineages[i].owner = uint32(i + 1)
	}
	for i := range c.links.Len() {
		{
			linkStorageMutation := c.links.At(int(i))
			linkStorageMutation.prev = NodeID(i + 1)
			c.links.Set(int(i), linkStorageMutation)
		}
	}
	for i := range c.subtrees.Len() {
		(*c.subtrees.At(int(i))).symbol = Symbol(i + 1)
	}
	for i := range c.children {
		c.children[i] = SubtreeID(i + 1)
	}
	return c
}

func TestProjectRecordArenasPreservesPublishedRecordsAndWork(t *testing.T) {
	c := projectionFixture(t)
	nodes := c.nodes.snapshotForTest()
	lineages := append([]nodeLineageHot(nil), c.nodeLineages...)
	links := c.links.Values()
	subtrees := c.subtrees.snapshotForTest()
	children := append([]SubtreeID(nil), c.children...)
	storage, work := c.StorageBytes(), c.Work()
	before := c.FootprintBytes()
	if !c.ProjectRecordArenas(10000, 5000, 1<<20) {
		t.Fatal("projection did not grow")
	}
	if !reflect.DeepEqual(c.nodes.snapshotForTest(), nodes) || !reflect.DeepEqual(c.nodeLineages, lineages) || !reflect.DeepEqual(c.links.Values(), links) || !reflect.DeepEqual(c.subtrees.snapshotForTest(), subtrees) || !reflect.DeepEqual(c.children, children) {
		t.Fatal("projection changed published records")
	}
	if c.StorageBytes() != storage || c.Work() != work {
		t.Fatal("projection changed storage or work")
	}
	if after := c.FootprintBytes(); after <= before || after-before > 1<<20 {
		t.Fatalf("capacity delta outside ceiling: %d -> %d", before, after)
	}
	if got := testing.AllocsPerRun(10, func() { c.ProjectRecordArenas(10000, 5000, 1<<20) }); got != 0 {
		t.Fatalf("warm projection allocated %g times", got)
	}
}

func TestProjectRecordArenasRejectsWholeGrowthWithoutChangingCapacity(t *testing.T) {
	c := projectionFixture(t)
	before := c.FootprintBytes()
	pointers := [5]any{c.nodes.At(int(0)), &c.nodeLineages[0], &c.links.records[0], c.subtrees.At(int(0)), &c.children[0]}
	if c.ProjectRecordArenas(10000, 5000, 1) {
		t.Fatal("projection exceeded caller ceiling")
	}
	afterPointers := [5]any{c.nodes.At(int(0)), &c.nodeLineages[0], &c.links.records[0], c.subtrees.At(int(0)), &c.children[0]}
	if c.FootprintBytes() != before || pointers != afterPointers {
		t.Fatal("rejected projection allocated partial capacity")
	}
}

func TestProjectRecordArenasRequiresLateBoundedSourceProgress(t *testing.T) {
	c := projectionFixture(t)
	inputs := []struct {
		source   int
		progress uint32
	}{{-1, 1}, {0, 1}, {10000, 0}, {10000, 4999}, {10000, 10000}, {10000, 10001}}
	if strconv.IntSize == 64 {
		source := uint64(1) << 32
		inputs = append(inputs, struct {
			source   int
			progress uint32
		}{int(source), 1 << 31})
	}
	before := c.FootprintBytes()
	for _, in := range inputs {
		if c.ProjectRecordArenas(in.source, in.progress, 0) {
			t.Fatalf("projected invalid progress %+v", in)
		}
	}
	var nilCore *Core
	if nilCore.ProjectRecordArenas(10000, 5000, 0) {
		t.Fatal("nil core projected")
	}
	if c.FootprintBytes() != before {
		t.Fatal("invalid progress allocated")
	}
}

func TestProjectRecordArenasHonorsRecordLimitsAtLargestSource(t *testing.T) {
	c := projectionFixture(t)
	c.limits.MaxNodes, c.limits.MaxLinks, c.limits.MaxSubtrees, c.limits.MaxChildren = 120, 110, 100, 90
	// MaxInt32 is representable on every supported architecture. On 64-bit,
	// also cover the entire uint32 source range without a large allocation.
	source, progress := int(1<<31-1), uint32(1<<30)
	if strconv.IntSize == 64 {
		value := uint64(1<<32 - 1)
		source, progress = int(value), 1<<31
	}
	if !c.ProjectRecordArenas(source, progress, 0) {
		t.Fatal("bounded large-source projection did not grow")
	}
	if c.nodes.Capacity() > 120 || cap(c.nodeLineages) > 120 || c.links.Cap() > 110 || c.subtrees.Capacity() > 100 || cap(c.children) > 90 {
		t.Fatal("projection exceeded record limits")
	}
}

func TestRecordArenaProjectionBudgetHonorsEachArmedLimit(t *testing.T) {
	for _, tc := range []struct {
		used       uint64
		soft, hard int64
		want       uint64
		ok         bool
	}{
		{100, 0, 0, 0, true}, {100, -1, -1, 0, true},
		{100, 500, 0, 100, true}, {100, 0, 500, 100, true},
		{100, 500, 900, 100, true}, {100, 900, 500, 100, true},
		{100, 100, 500, 0, false}, {100, 500, 99, 0, false},
		{100, 103, 500, 0, false}, {^uint64(0), 500, 500, 0, false},
	} {
		got, ok := RecordArenaProjectionBudget(tc.used, tc.soft, tc.hard)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("budget(%d,%d,%d)=(%d,%t), want (%d,%t)", tc.used, tc.soft, tc.hard, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRecordArenaProjectionStartCannotMissOddLengthMidpoint(t *testing.T) {
	for _, source := range []int{2, 3, 9, 10, 1025, 10000, 1048591, 1<<31 - 1} {
		start := RecordArenaProjectionStart(source)
		if uint64(start)*2 < uint64(source) || uint64(start) >= uint64(source) {
			t.Fatalf("source %d armed invalid midpoint %d", source, start)
		}
		c := projectionFixture(t)
		if !c.ProjectRecordArenas(source, start, 1<<20) {
			t.Fatalf("source %d rejected its scheduled projection at %d", source, start)
		}
	}
	for _, source := range []int{-1, 0, 1} {
		if RecordArenaProjectionStart(source) != 0 {
			t.Fatalf("armed invalid source %d", source)
		}
	}
	if strconv.IntSize == 64 {
		source := uint64(1) << 32
		if RecordArenaProjectionStart(int(source)) != 0 {
			t.Fatal("armed unrepresentable source")
		}
	}
}
