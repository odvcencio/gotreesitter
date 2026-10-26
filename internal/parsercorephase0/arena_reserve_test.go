package parsercorephase0

import (
	"fmt"
	"testing"
)

func reserveTestCore(t *testing.T, limits Limits) *Core {
	t.Helper()
	core, err := New(&fakeTable{}, limits)
	if err != nil {
		t.Fatal(err)
	}
	return core
}

func reserveTestLimits() Limits {
	return Limits{
		MaxNodes: 1 << 20, MaxLinks: 1 << 20, MaxSubtrees: 1 << 20,
		MaxChildren: 4 << 20, MaxMetadata: 2 << 20,
		MaxLinksPerBoundary: 8, MaxPopPaths: 1 << 16, MaxDerivations: 1 << 16,
	}
}

func TestRecordArenaReservePlanBoundsSpeculationAndReleasesDecline(t *testing.T) {
	const sourceBytes = 1998
	initial, growAt := RecordArenaReservePlan(sourceBytes)
	if initial != 128 || growAt != 1024 {
		t.Fatalf("reserve plan=(%d,%d), want initial 128 and progress 1024", initial, growAt)
	}
	for _, ceiling := range []uint64{64, 1024, 24 << 20} {
		t.Run(fmt.Sprint(ceiling), func(t *testing.T) {
			core := reserveTestCore(t, reserveTestLimits())
			oldPrefixBytes := core.ReserveRecordArenaBytes(int(growAt), ceiling)
			core.ReserveRecordArenas(initial, ceiling)
			footprint := core.FootprintBytes()
			if footprint > ceiling || footprint > oldPrefixBytes || core.StorageBytes() != 0 || core.Work() != (Work{}) {
				t.Fatalf("speculative reserve changed live state or exceeded budget: footprint=%d old=%d ceiling=%d storage=%d work=%+v",
					footprint, oldPrefixBytes, ceiling, core.StorageBytes(), core.Work())
			}
			if ceiling == 24<<20 && (core.nodes.Capacity() != 96 || footprint*7 >= oldPrefixBytes) {
				t.Fatalf("initial capacity nodes=%d footprint=%d, want 96 nodes and less than one seventh of %d", core.nodes.Capacity(), footprint, oldPrefixBytes)
			}
			if err := core.ResetReleasingRetention(); err != nil {
				t.Fatal(err)
			}
			if core.nodes.Capacity()+cap(core.nodeLineages)+core.links.Cap()+core.subtrees.Capacity()+cap(core.children) != 0 {
				t.Fatal("decline retained speculative record arenas")
			}
		})
	}
}

// TestReserveRecordArenasTakesCapacityNotLength proves the reserve is a pure
// capacity operation: every arena gains capacity and none gains a record.
func TestReserveRecordArenasTakesCapacityNotLength(t *testing.T) {
	core := reserveTestCore(t, reserveTestLimits())
	const sourceLen = 235626
	core.ReserveRecordArenas(sourceLen, 1<<30)

	if got := core.nodes.Len() + len(core.nodeLineages) + core.links.Len() + core.subtrees.Len() + len(core.children); got != 0 {
		t.Fatalf("reserve published %d records, want 0", got)
	}
	if got := core.StorageBytes(); got != 0 {
		t.Fatalf("reserve moved StorageBytes to %d, want 0", got)
	}
	want := map[string]int{
		"nodes":        core.nodes.Capacity(),
		"nodeLineages": cap(core.nodeLineages),
		"links":        core.links.Cap(),
		"subtrees":     core.subtrees.Capacity(),
		"children":     cap(core.children),
	}
	for name, got := range want {
		if got == 0 {
			t.Fatalf("reserve left %s with zero capacity", name)
		}
	}
	if core.nodes.Capacity() != cap(core.nodeLineages) {
		t.Fatalf("nodes cap=%d nodeLineages cap=%d, want equal (appendNodeAt appends to both)",
			core.nodes.Capacity(), cap(core.nodeLineages))
	}
	// The densest canonical fixture publishes 7333 nodes per 10000 source
	// bytes. The reserve must cover that density without growth.
	if want := sourceLen * 7333 / 10000; core.nodes.Capacity() < want {
		t.Fatalf("nodes cap=%d, want at least the densest measured fixture's %d", core.nodes.Capacity(), want)
	}
}

// TestReserveRecordArenaBytesMatchesFootprint proves the estimator a caller
// checks against its budget reports what the reserve actually takes.
func TestReserveRecordArenaBytesMatchesFootprint(t *testing.T) {
	for _, sourceLen := range []int{1, 1024, 20168, 235626, 1 << 20} {
		core := reserveTestCore(t, reserveTestLimits())
		before := core.FootprintBytes()
		want := core.ReserveRecordArenaBytes(sourceLen, 48<<20)
		core.ReserveRecordArenas(sourceLen, 48<<20)
		if got := core.FootprintBytes() - before; got != want {
			t.Fatalf("sourceLen=%d footprint delta=%d, want ReserveRecordArenaBytes=%d", sourceLen, got, want)
		}
	}
}

// TestReserveRecordArenasHonorsMaxBytes proves the byte ceiling binds and that
// the reserve keeps its shape when it does.
func TestReserveRecordArenasHonorsMaxBytes(t *testing.T) {
	const maxBytes = 8 << 20
	core := reserveTestCore(t, reserveTestLimits())
	core.ReserveRecordArenas(1<<20, maxBytes)
	if got := core.FootprintBytes(); got > maxBytes {
		t.Fatalf("reserve took %d bytes, want at most %d", got, maxBytes)
	}
	unclamped := reserveTestCore(t, reserveTestLimits())
	if got := unclamped.ReserveRecordArenaBytes(1<<20, 0); got <= maxBytes {
		t.Fatalf("unclamped reserve=%d, want more than the ceiling %d so this test proves the clamp", got, maxBytes)
	}
	// Shape: every arena keeps the same order relative to nodes.
	if core.nodes.Capacity() != cap(core.nodeLineages) || core.links.Cap() != core.nodes.Capacity() {
		t.Fatalf("clamped reserve lost its shape: nodes=%d lineages=%d links=%d",
			core.nodes.Capacity(), cap(core.nodeLineages), core.links.Cap())
	}
	if core.subtrees.Capacity() >= core.nodes.Capacity() || cap(core.children) >= core.nodes.Capacity() {
		t.Fatalf("clamped reserve lost its shape: nodes=%d subtrees=%d children=%d",
			core.nodes.Capacity(), core.subtrees.Capacity(), cap(core.children))
	}
}

// TestReserveRecordArenasHonorsLimits proves a reserve never asks for capacity
// the configured Limits would refuse to fill.
func TestReserveRecordArenasHonorsLimits(t *testing.T) {
	limits := reserveTestLimits()
	limits.MaxNodes, limits.MaxLinks, limits.MaxSubtrees, limits.MaxChildren = 100, 200, 300, 400
	core := reserveTestCore(t, limits)
	core.ReserveRecordArenas(1<<20, 1<<30)
	if core.nodes.Capacity() > 100 || cap(core.nodeLineages) > 100 {
		t.Fatalf("nodes cap=%d lineages cap=%d, want at most MaxNodes=100", core.nodes.Capacity(), cap(core.nodeLineages))
	}
	if core.links.Cap() > 200 || core.subtrees.Capacity() > 300 || cap(core.children) > 400 {
		t.Fatalf("links=%d subtrees=%d children=%d, want at most 200/300/400",
			core.links.Cap(), core.subtrees.Capacity(), cap(core.children))
	}
}

// TestReserveRecordArenasOnlyGrows proves a reserve never drops retained
// capacity a previous parse on the same core already earned, and never runs at
// all once an arena holds records.
func TestReserveRecordArenasOnlyGrows(t *testing.T) {
	core := reserveTestCore(t, reserveTestLimits())
	core.ReserveRecordArenas(235626, 1<<30)
	big := core.nodes.Capacity()
	core.ReserveRecordArenas(1024, 1<<30)
	if core.nodes.Capacity() != big {
		t.Fatalf("a smaller reserve shrank nodes cap from %d to %d", big, core.nodes.Capacity())
	}

	core.nodes.Append(nodeRecord{}, ^uint32(0))
	core.nodeLineages = append(core.nodeLineages, nodeLineageHot{})
	linksBefore := core.links.Cap()
	core.ReserveRecordArenas(1<<20, 1<<30)
	if core.links.Cap() != linksBefore {
		t.Fatalf("reserve ran on a non-empty core: links cap %d -> %d", linksBefore, core.links.Cap())
	}
}

// TestReserveRecordArenasIgnoresEmptyInput proves the degenerate inputs are
// no-ops rather than panics.
func TestReserveRecordArenasIgnoresEmptyInput(t *testing.T) {
	core := reserveTestCore(t, reserveTestLimits())
	core.ReserveRecordArenas(0, 1<<30)
	core.ReserveRecordArenas(-1, 1<<30)
	if got := core.FootprintBytes(); got != 0 {
		t.Fatalf("reserve on empty input took %d bytes, want 0", got)
	}
	var nilCore *Core
	nilCore.ReserveRecordArenas(1024, 1<<20)
	if got := nilCore.ReserveRecordArenaBytes(1024, 1<<20); got != 0 {
		t.Fatalf("nil core reserve estimate=%d, want 0", got)
	}
}

// TestResetReleasingRetentionDropsTheReserve proves a declined attempt does
// not leave its unfilled reserve billed to the cached core. The reserve is
// taken before the seed, so every decline reason is discovered after it; and
// the reserve is sized below coreRetentionCapBytes by construction, so the
// size-gated release can never see it.
func TestResetReleasingRetentionDropsTheReserve(t *testing.T) {
	core := reserveTestCore(t, reserveTestLimits())
	core.ReserveRecordArenas(235626, 24<<20)
	reserved := core.FootprintBytes()
	if reserved == 0 || reserved > coreRetentionCapBytes {
		t.Fatalf("reserve footprint=%d, want non-zero and below the retention cap %d so this test proves the new rule",
			reserved, uint64(coreRetentionCapBytes))
	}
	if err := core.ResetReleasingRetention(); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]int{
		"nodes": core.nodes.Capacity(), "nodeLineages": cap(core.nodeLineages),
		"links": core.links.Cap(), "subtrees": core.subtrees.Capacity(), "children": cap(core.children),
	} {
		if got != 0 {
			t.Fatalf("decline kept %s capacity %d, want 0", name, got)
		}
	}
	if got := core.FootprintBytes(); got >= reserved {
		t.Fatalf("decline kept footprint %d, want below the reserved %d", got, reserved)
	}
}

// TestResetKeepsTheReserve proves the plain accepted-path Reset still keeps
// arena capacity for reuse. Only the decline path drops it.
func TestResetKeepsTheReserve(t *testing.T) {
	core := reserveTestCore(t, reserveTestLimits())
	core.ReserveRecordArenas(235626, 24<<20)
	want := core.nodes.Capacity()
	if err := core.Reset(); err != nil {
		t.Fatal(err)
	}
	if got := core.nodes.Capacity(); got != want {
		t.Fatalf("accepted-path reset changed nodes capacity %d -> %d", want, got)
	}
}

// TestGrowRecordArenasKeepsRecordsAndReachesFullReserve proves the second
// reserve step keeps every published record, reaches the capacity a one-shot
// reserve takes, and never shrinks an arena.
func TestGrowRecordArenasKeepsRecordsAndReachesFullReserve(t *testing.T) {
	const sourceLen, prefixLen, maxBytes = 235626, 235626 / 16, 48 << 20
	core := reserveTestCore(t, reserveTestLimits())
	core.ReserveRecordArenas(prefixLen, maxBytes)
	if _, err := core.Seed(0, 0); err != nil {
		t.Fatal(err)
	}
	nodes, links, subtrees, children := core.nodes.Len(), core.links.Len(), core.subtrees.Len(), len(core.children)
	if nodes == 0 {
		t.Fatal("seed published no node record")
	}
	firstNode := *core.nodes.At(int(0))
	storage := core.StorageBytes()

	core.GrowRecordArenas(sourceLen, maxBytes)
	if core.nodes.Len() != nodes || core.links.Len() != links || core.subtrees.Len() != subtrees || len(core.children) != children {
		t.Fatal("grow changed an arena length")
	}
	if *core.nodes.At(int(0)) != firstNode || core.StorageBytes() != storage {
		t.Fatal("grow changed a published record")
	}
	full := reserveTestCore(t, reserveTestLimits())
	full.ReserveRecordArenas(sourceLen, maxBytes)
	for name, pair := range map[string][2]int{
		"nodes":        {core.nodes.Capacity(), full.nodes.Capacity()},
		"nodeLineages": {cap(core.nodeLineages), cap(full.nodeLineages)},
		"links":        {core.links.Cap(), full.links.Cap()},
		"subtrees":     {core.subtrees.Capacity(), full.subtrees.Capacity()},
		"children":     {cap(core.children), cap(full.children)},
	} {
		if pair[0] < pair[1] {
			t.Fatalf("%s cap=%d after grow, want at least the one-shot reserve's %d", name, pair[0], pair[1])
		}
	}

	grown := core.nodes.Capacity()
	core.GrowRecordArenas(prefixLen, maxBytes)
	if core.nodes.Capacity() != grown {
		t.Fatalf("grow to a smaller source shrank nodes from %d to %d", grown, core.nodes.Capacity())
	}
}
