//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"bytes"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestCompactExternalScannerCheckpointTransferRetainsExactStateWithoutRepeatedCopies(t *testing.T) {
	// This witness only interns checkpoints; it needs no action-table cells.
	compact, err := core.New(&parserCoreRootTables{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	state := []byte{3, 7, 11}
	id, err := compact.InternCheckpoint(state)
	if err != nil {
		t.Fatal(err)
	}
	arena := newNodeArena(arenaClassIncremental)
	defer arena.Release()
	node := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	view := core.MaterializationSubtreeView{Terminal: true, ExternalScannerCheckpointExact: true, ExternalScannerCheckpointStart: id, ExternalScannerCheckpointEnd: id}
	if !materializeCompactExternalScannerCheckpoint(compact, arena, node, view) {
		t.Fatal("exact transfer failed")
	}
	if allocs := testing.AllocsPerRun(5, func() {
		if !materializeCompactExternalScannerCheckpoint(compact, arena, node, view) {
			t.Fatal("repeated transfer failed")
		}
	}); allocs != 0 {
		t.Fatalf("unchanged scanner state allocated %g copies", allocs)
	}
	changed := []byte{5, 13, 17}
	end, err := compact.InternCheckpoint(changed)
	if err != nil {
		t.Fatal(err)
	}
	view.ExternalScannerCheckpointEnd = end
	other := newLeafNodeInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	if !materializeCompactExternalScannerCheckpoint(compact, arena, other, view) {
		t.Fatal("changed scanner state failed")
	}
	old, ok := externalScannerCheckpointRefForNode(node)
	if !ok || !bytes.Equal(arena.externalScannerSnapshotBytes(old.start), state) || !bytes.Equal(arena.externalScannerSnapshotBytes(old.end), state) {
		t.Fatal("later transfer changed the preceding node's scanner state")
	}
	current, ok := externalScannerCheckpointRefForNode(other)
	if !ok || !bytes.Equal(arena.externalScannerSnapshotBytes(current.start), state) || !bytes.Equal(arena.externalScannerSnapshotBytes(current.end), changed) {
		t.Fatal("changed boundary did not retain both exact states")
	}
}

func TestCompactExternalScannerCheckpointTransferFailsClosed(t *testing.T) {
	compact := &core.Core{}
	arena := newNodeArena(arenaClassIncremental)
	defer arena.Release()
	node := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	view := core.MaterializationSubtreeView{
		External:                       true,
		Terminal:                       true,
		ExternalScannerCheckpointExact: true,
		ExternalScannerCheckpointStart: core.CheckpointID(1),
		ExternalScannerCheckpointEnd:   core.CheckpointID(2),
	}
	beforeRecords := arena.externalScannerCheckpointRecords
	beforeLeaves := arena.externalScannerCheckpointLeafNodes
	if materializeCompactExternalScannerCheckpoint(compact, arena, node, view) {
		t.Fatal("checkpoint transfer accepted missing core snapshots")
	}
	if arena.externalScannerCheckpointRecords != beforeRecords || arena.externalScannerCheckpointLeafNodes != beforeLeaves {
		t.Fatalf("failed transfer changed checkpoint counters: records=%d/%d leaves=%d/%d", arena.externalScannerCheckpointRecords, beforeRecords, arena.externalScannerCheckpointLeafNodes, beforeLeaves)
	}
	if _, ok := externalScannerCheckpointRefForNode(node); ok {
		t.Fatal("failed transfer left a usable node checkpoint")
	}

	foreignArena := newNodeArena(arenaClassIncremental)
	defer foreignArena.Release()
	foreignNode := newLeafNodeInArena(foreignArena, 1, true, 0, 1, Point{}, Point{Column: 1})
	beforeSlots := arena.externalScannerCheckpointSlotsAllocated()
	beforeBytes := arena.externalScannerCheckpointBytesAllocated()
	beforePayload := arena.externalScannerSnapshotPayloadBytes
	if materializeCompactExternalScannerCheckpoint(compact, arena, foreignNode, view) {
		t.Fatal("checkpoint transfer accepted a node owned by another arena")
	}
	if arena.externalScannerCheckpointRecords != beforeRecords || arena.externalScannerCheckpointLeafNodes != beforeLeaves {
		t.Fatalf("foreign-node transfer changed counters: records=%d/%d leaves=%d/%d", arena.externalScannerCheckpointRecords, beforeRecords, arena.externalScannerCheckpointLeafNodes, beforeLeaves)
	}
	if got := arena.externalScannerCheckpointSlotsAllocated(); got != beforeSlots {
		t.Fatalf("foreign-node transfer allocated checkpoint slots: %d/%d", got, beforeSlots)
	}
	if got := arena.externalScannerCheckpointBytesAllocated(); got != beforeBytes {
		t.Fatalf("foreign-node transfer allocated checkpoint bytes: %d/%d", got, beforeBytes)
	}
	if got := arena.externalScannerSnapshotPayloadBytes; got != beforePayload {
		t.Fatalf("foreign-node transfer allocated snapshot payload: %d/%d", got, beforePayload)
	}
	if _, ok := externalScannerCheckpointRefForNode(foreignNode); ok {
		t.Fatal("foreign-node transfer left a usable checkpoint")
	}
}

func TestCompactMaterializerScannerCacheAlternatingPairsAndReset(t *testing.T) {
	compact, err := core.New(&parserCoreRootTables{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := compact.InternCheckpoint([]byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	second, err := compact.InternCheckpoint([]byte{4, 5, 6})
	if err != nil {
		t.Fatal(err)
	}
	arena := newNodeArena(arenaClassIncremental)
	defer arena.Release()
	m := compactMaterializer{compact: compact, arena: arena}
	nodes := make([]*Node, 2)
	for i := range nodes {
		nodes[i] = newLeafNodeInArena(arena, 1, true, uint32(i), uint32(i+1), Point{Column: uint32(i)}, Point{Column: uint32(i + 1)})
	}
	views := []core.MaterializationSubtreeView{
		{Terminal: true, ExternalScannerCheckpointExact: true, ExternalScannerCheckpointStart: first, ExternalScannerCheckpointEnd: second},
		{Terminal: true, ExternalScannerCheckpointExact: true, ExternalScannerCheckpointStart: second, ExternalScannerCheckpointEnd: first},
	}
	for i := range nodes {
		if !m.materializeScannerCheckpoint(arena, nodes[i], views[i]) {
			t.Fatal("pair transfer failed")
		}
	}
	if allocations := testing.AllocsPerRun(5, func() {
		for i := range nodes {
			if !m.materializeScannerCheckpoint(arena, nodes[i], views[i]) {
				t.Fatal("cached pair transfer failed")
			}
		}
	}); allocations != 0 {
		t.Fatalf("cached alternating pairs allocated %g", allocations)
	}
	for i, n := range nodes {
		ref, ok := externalScannerCheckpointRefForNode(n)
		if !ok {
			t.Fatal("pair absent")
		}
		wantStart, wantEnd := []byte{1, 2, 3}, []byte{4, 5, 6}
		if i == 1 {
			wantStart, wantEnd = wantEnd, wantStart
		}
		if !bytes.Equal(arena.externalScannerSnapshotBytes(ref.start), wantStart) || !bytes.Equal(arena.externalScannerSnapshotBytes(ref.end), wantEnd) {
			t.Fatal("dictionary pair lost exact bytes")
		}
	}
	m.clearState()
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	replacement, err := compact.InternCheckpoint([]byte{9, 8, 7})
	if err != nil {
		t.Fatal(err)
	}
	if replacement != first {
		t.Fatal("fixture did not reuse the core checkpoint identity")
	}
	another := newNodeArena(arenaClassIncremental)
	defer another.Release()
	m.compact, m.arena = compact, another
	n := newLeafNodeInArena(another, 1, true, 0, 1, Point{}, Point{Column: 1})
	view := views[0]
	view.ExternalScannerCheckpointStart, view.ExternalScannerCheckpointEnd = replacement, replacement
	if !m.materializeScannerCheckpoint(another, n, view) {
		t.Fatal("reset pair transfer failed")
	}
	ref, ok := externalScannerCheckpointRefForNode(n)
	if !ok || !bytes.Equal(another.externalScannerSnapshotBytes(ref.start), []byte{9, 8, 7}) || !bytes.Equal(another.externalScannerSnapshotBytes(ref.end), []byte{9, 8, 7}) {
		t.Fatal("cache crossed the core or arena lifetime")
	}
	if ref, ok := externalScannerCheckpointRefForNode(nodes[0]); !ok || !bytes.Equal(arena.externalScannerSnapshotBytes(ref.start), []byte{1, 2, 3}) {
		t.Fatal("reset changed the preceding arena")
	}
}

func TestCompactCheckpointDictionaryPreservesPreviouslyShapedParent(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	defer arena.Release()
	first := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	second := newLeafNodeInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	cp := arena.recordExternalScannerExactCompactCheckpoint([]byte{1, 2, 3}, []byte{4, 5, 6})
	if !arena.setExternalScannerCheckpoint(first, cp) {
		t.Fatal("initial shaped receipt failed")
	}
	oldBytes := arena.externalScannerCheckpointBytesAllocated()
	if !arena.setCompactExternalScannerCheckpoint(second, cp) {
		t.Fatal("compact receipt failed")
	}
	for _, node := range []*Node{first, second} {
		got, ok := externalScannerCheckpointRefForNode(node)
		if !ok || got != cp {
			t.Fatal("conversion lost a shaped receipt")
		}
	}
	if got := arena.externalScannerCheckpointBytesAllocated(); got >= oldBytes {
		t.Fatalf("conversion grew storage: %d >= %d", got, oldBytes)
	}
}
