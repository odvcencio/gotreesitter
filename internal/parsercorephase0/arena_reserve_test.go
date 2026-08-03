package parsercorephase0

import "testing"

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

// TestReserveRecordArenasTakesCapacityNotLength proves the reserve is a pure
// capacity operation: every arena gains capacity and none gains a record.
func TestReserveRecordArenasTakesCapacityNotLength(t *testing.T) {
	core := reserveTestCore(t, reserveTestLimits())
	const sourceLen = 235626
	core.ReserveRecordArenas(sourceLen, 1<<30)

	if got := len(core.nodes) + len(core.nodeLineages) + len(core.links) + len(core.subtrees) + len(core.children); got != 0 {
		t.Fatalf("reserve published %d records, want 0", got)
	}
	if got := core.StorageBytes(); got != 0 {
		t.Fatalf("reserve moved StorageBytes to %d, want 0", got)
	}
	want := map[string]int{
		"nodes":        cap(core.nodes),
		"nodeLineages": cap(core.nodeLineages),
		"links":        cap(core.links),
		"subtrees":     cap(core.subtrees),
		"children":     cap(core.children),
	}
	for name, got := range want {
		if got == 0 {
			t.Fatalf("reserve left %s with zero capacity", name)
		}
	}
	if cap(core.nodes) != cap(core.nodeLineages) {
		t.Fatalf("nodes cap=%d nodeLineages cap=%d, want equal (appendNodeAt appends to both)",
			cap(core.nodes), cap(core.nodeLineages))
	}
	// The densest canonical fixture publishes 7333 nodes per 10000 source
	// bytes. The reserve must cover that density without growth.
	if want := sourceLen * 7333 / 10000; cap(core.nodes) < want {
		t.Fatalf("nodes cap=%d, want at least the densest measured fixture's %d", cap(core.nodes), want)
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
	if cap(core.nodes) != cap(core.nodeLineages) || cap(core.links) != cap(core.nodes) {
		t.Fatalf("clamped reserve lost its shape: nodes=%d lineages=%d links=%d",
			cap(core.nodes), cap(core.nodeLineages), cap(core.links))
	}
	if cap(core.subtrees) >= cap(core.nodes) || cap(core.children) >= cap(core.nodes) {
		t.Fatalf("clamped reserve lost its shape: nodes=%d subtrees=%d children=%d",
			cap(core.nodes), cap(core.subtrees), cap(core.children))
	}
}

// TestReserveRecordArenasHonorsLimits proves a reserve never asks for capacity
// the configured Limits would refuse to fill.
func TestReserveRecordArenasHonorsLimits(t *testing.T) {
	limits := reserveTestLimits()
	limits.MaxNodes, limits.MaxLinks, limits.MaxSubtrees, limits.MaxChildren = 100, 200, 300, 400
	core := reserveTestCore(t, limits)
	core.ReserveRecordArenas(1<<20, 1<<30)
	if cap(core.nodes) > 100 || cap(core.nodeLineages) > 100 {
		t.Fatalf("nodes cap=%d lineages cap=%d, want at most MaxNodes=100", cap(core.nodes), cap(core.nodeLineages))
	}
	if cap(core.links) > 200 || cap(core.subtrees) > 300 || cap(core.children) > 400 {
		t.Fatalf("links=%d subtrees=%d children=%d, want at most 200/300/400",
			cap(core.links), cap(core.subtrees), cap(core.children))
	}
}

// TestReserveRecordArenasOnlyGrows proves a reserve never drops retained
// capacity a previous parse on the same core already earned, and never runs at
// all once an arena holds records.
func TestReserveRecordArenasOnlyGrows(t *testing.T) {
	core := reserveTestCore(t, reserveTestLimits())
	core.ReserveRecordArenas(235626, 1<<30)
	big := cap(core.nodes)
	core.ReserveRecordArenas(1024, 1<<30)
	if cap(core.nodes) != big {
		t.Fatalf("a smaller reserve shrank nodes cap from %d to %d", big, cap(core.nodes))
	}

	core.nodes = append(core.nodes, nodeRecord{})
	core.nodeLineages = append(core.nodeLineages, nodeLineageRecord{})
	linksBefore := cap(core.links)
	core.ReserveRecordArenas(1<<20, 1<<30)
	if cap(core.links) != linksBefore {
		t.Fatalf("reserve ran on a non-empty core: links cap %d -> %d", linksBefore, cap(core.links))
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
		"nodes": cap(core.nodes), "nodeLineages": cap(core.nodeLineages),
		"links": cap(core.links), "subtrees": cap(core.subtrees), "children": cap(core.children),
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
	want := cap(core.nodes)
	if err := core.Reset(); err != nil {
		t.Fatal(err)
	}
	if got := cap(core.nodes); got != want {
		t.Fatalf("accepted-path reset changed nodes capacity %d -> %d", want, got)
	}
}
