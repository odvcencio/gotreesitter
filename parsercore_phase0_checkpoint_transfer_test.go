//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"bytes"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestCompactExternalScannerCheckpointTransferRetainsExactStateWithoutRepeatedCopies(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
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
