//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"bytes"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestCompactExternalScannerCheckpointTransferOwnsBytesWithoutWarmAllocations(t *testing.T) {
	compact, err := core.New(&parserCoreRootTables{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	startBytes, endBytes := []byte{1, 2, 3}, []byte{4, 5, 6, 7}
	start, err := compact.InternCheckpoint(startBytes)
	if err != nil {
		t.Fatal(err)
	}
	end, err := compact.InternCheckpoint(endBytes)
	if err != nil {
		t.Fatal(err)
	}
	arena := newNodeArena(arenaClassIncremental)
	defer arena.Release()
	node := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	view := core.MaterializationSubtreeView{
		Terminal: true, ExternalScannerCheckpointExact: true,
		ExternalScannerCheckpointStart: start, ExternalScannerCheckpointEnd: end,
	}
	var scratch core.CheckpointCopyScratch
	if !materializeCompactExternalScannerCheckpoint(compact, arena, node, view, &scratch) {
		t.Fatal("checkpoint transfer failed")
	}
	// Reusing the copy buffers for another leaf must not change the first leaf.
	other := newLeafNodeInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	view.ExternalScannerCheckpointStart, view.ExternalScannerCheckpointEnd = end, start
	if !materializeCompactExternalScannerCheckpoint(compact, arena, other, view, &scratch) {
		t.Fatal("second checkpoint transfer failed")
	}
	view.ExternalScannerCheckpointStart, view.ExternalScannerCheckpointEnd = start, start
	if got := testing.AllocsPerRun(100, func() {
		if !materializeCompactExternalScannerCheckpoint(compact, arena, other, view, &scratch) {
			t.Fatal("warm transfer failed")
		}
	}); got != 0 {
		t.Fatalf("warm checkpoint transfer allocates: %g", got)
	}
	scratch.Reset()
	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := compact.InternCheckpoint([]byte{9, 9, 9, 9}); err != nil {
		t.Fatal(err)
	}
	checkpoint, ok := externalScannerCheckpointForNode(node)
	if !ok || !bytes.Equal(checkpoint.start, startBytes) || !bytes.Equal(checkpoint.end, endBytes) {
		t.Fatal("result checkpoint changed after scratch reuse and core reset")
	}
}

func TestCompactExternalScannerCheckpointCopyMemoryBudget(t *testing.T) {
	compact, err := core.New(&parserCoreRootTables{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := compact.InternCheckpoint(make([]byte, 2048))
	if err != nil {
		t.Fatal(err)
	}
	var scratch diagnosticParserCoreMaterializationScratch
	if _, _, ok := scratch.checkpoints.CopyPair(compact, id, id); !ok {
		t.Fatal("checkpoint copy failed")
	}
	parser := NewParser(&Language{})
	var scheduler diagnosticParserCoreGenericScheduler
	baseline := diagnosticParserCoreSchedulerFootprintBytes(&scheduler)
	scheduler.options.stopControlMemoryBudgetBytes = int64(baseline + 1024)
	m := compactMaterializer{parser: parser, budgetScheduler: &scheduler, materializationScratch: &scratch}
	if err := m.poll(); err == nil {
		t.Fatal("materialization budget omitted checkpoint copy storage")
	}
	m.materializationScratch = nil
	if err := m.poll(); err != nil {
		t.Fatalf("empty materializer exceeds the budget: %v", err)
	}
	m.materializationScratch = &scratch
	m.eager = true
	scheduler.options.eagerMaterializer = &m
	if err := scheduler.pollStopControl(); err == nil {
		t.Fatal("eager materialization budget omitted checkpoint copy storage")
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
	var scratch core.CheckpointCopyScratch
	beforeRecords := arena.externalScannerCheckpointRecords
	beforeLeaves := arena.externalScannerCheckpointLeafNodes
	if materializeCompactExternalScannerCheckpoint(compact, arena, node, view, &scratch) {
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
	if materializeCompactExternalScannerCheckpoint(compact, arena, foreignNode, view, &scratch) {
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
