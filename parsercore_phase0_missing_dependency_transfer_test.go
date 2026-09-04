//go:build gts_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestCompactMaterializationTransfersMissingDependency(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	defer arena.Release()
	node := newLeafNodeInArena(arena, 1, true, 3, 3, Point{Column: 3}, Point{Column: 3})
	node.setMissing(true)
	node.setHasError(true)
	view := core.MaterializationSubtreeView{
		Missing: true, MissingDependencyExact: true,
		MissingDependency: core.MissingLeafDependency{
			StackByte: 0, PaddingBytes: 3, PaddingColumn: 3, LookaheadBytes: 3,
		},
	}
	if !materializeCompactMissingNodeDependency(arena, node, view) {
		t.Fatal("exact compact missing dependency did not transfer")
	}
	want := missingNodeDependency{
		stackByte: 0, stackPoint: Point{},
		paddingBytes: 3, paddingExtent: Point{Column: 3}, lookaheadBytes: 3,
	}
	if got, ok := missingNodeDependencyForNode(node); !ok || got != want {
		t.Fatalf("transferred dependency=%+v exact=%t, want %+v", got, ok, want)
	}
}

func TestCompactMaterializationRejectsMissingDependencyWithoutReceipt(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	defer arena.Release()
	node := newLeafNodeInArena(arena, 1, true, 3, 3, Point{Column: 3}, Point{Column: 3})
	node.setMissing(true)
	if materializeCompactMissingNodeDependency(arena, node, core.MaterializationSubtreeView{Missing: true}) {
		t.Fatal("compact materialization accepted missing dependency without an exact receipt")
	}
}

func TestCompactMaterializationRejectsUnauthenticatedZeroPaddingPoint(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	defer arena.Release()
	point := Point{Row: 1, Column: 2}
	node := newLeafNodeInArena(arena, 1, true, 3, 3, point, point)
	node.setMissing(true)
	view := core.MaterializationSubtreeView{
		Missing: true, MissingDependencyExact: true,
		MissingDependency: core.MissingLeafDependency{StackByte: 3},
	}
	if materializeCompactMissingNodeDependency(arena, node, view) {
		t.Fatal("compact materialization rewrote an unauthenticated stack point")
	}
}

func TestCompactMaterializationAcceptsAuthenticatedZeroPaddingPoint(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	defer arena.Release()
	point := Point{Row: 1, Column: 2}
	node := newLeafNodeInArena(arena, 1, true, 3, 3, point, point)
	node.setMissing(true)
	view := core.MaterializationSubtreeView{
		Missing: true, MissingDependencyExact: true,
		MissingDependency: core.MissingLeafDependency{
			StackByte: 3, StackRow: point.Row, StackColumn: point.Column,
		},
	}
	if !materializeCompactMissingNodeDependency(arena, node, view) {
		t.Fatal("compact materialization rejected an authenticated stack point")
	}
}
