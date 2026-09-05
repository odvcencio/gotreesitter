//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import "testing"

func TestCompactReuseDependencyNodeEditKeepsBorrowedPendingRefsLazy(t *testing.T) {
	borrowedArena := newNodeArena(arenaClassIncremental)
	defer borrowedArena.Release()
	unaffected := newLeafNodeInArena(borrowedArena, 1, true, 0, 1, Point{}, Point{Column: 1})
	borrowed := newLeafNodeInArena(borrowedArena, 2, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	if !setCompactReuseDependency(unaffected, 0) || !setCompactReuseDependency(borrowed, 6) {
		t.Fatal("could not authenticate borrowed receipts")
	}

	arena := newNodeArena(arenaClassFull)
	defer arena.Release()
	arena.finalChildRefs = true
	inner := newPendingParentInArena(arena, 3, true, 0, []stackEntry{
		newStackEntryNode(11, unaffected),
		newStackEntryNode(12, borrowed),
	}, 0, 4, Point{}, Point{Column: 4}, false)
	tail := newCompactFullLeafInArena(arena, 4, true, 10, 12, Point{Column: 10}, Point{Column: 12})
	outer := newPendingParentInArena(arena, 5, true, 0, []stackEntry{
		newStackEntryPendingParent(13, inner),
		newStackEntryCompactFullLeaf(14, tail),
	}, 0, 12, Point{}, Point{Column: 12}, false)
	entry := newStackEntryPendingParent(15, outer)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root materialization failed")
	}
	child, ok := nodeChildEntryAtNoMaterialize(root, 0)
	if !ok || child.kind != stackEntryKindPendingParent {
		t.Fatal("fixture lost its pending parent behind the final child reference")
	}
	pendingBefore := arena.pendingParentMaterialized

	// Node.Edit has no borrowed-arena list. The dependency reaches byte 10,
	// although both the borrowed node and its pending parent end at byte 4.
	root.Edit(InputEdit{
		StartByte: 8, OldEndByte: 9, NewEndByte: 9,
		StartPoint: Point{Column: 8}, OldEndPoint: Point{Column: 9}, NewEndPoint: Point{Column: 9},
	})
	if _, ok := compactReuseDependencyForNode(borrowed); ok {
		t.Error("Node.Edit retained the borrowed node's intersecting dependency")
	}
	if extent, ok := compactReuseDependencyForNode(unaffected); !ok || extent != 0 {
		t.Error("Node.Edit removed the unaffected zero-extent receipt")
	}
	if borrowed.StartByte() != 2 || borrowed.EndByte() != 4 {
		t.Error("the suffix edit changed the borrowed node's range")
	}
	if arena.finalChildRefsMaterializedParents != 0 || arena.finalChildRefsMaterializedChildren != 0 ||
		arena.finalChildRefsSingleChildMaterializedChildren != 0 || arena.compactFullLeafMaterialized != 0 ||
		arena.pendingParentMaterialized != pendingBefore || arena.pendingParentMaterializedForEdit != 0 {
		t.Fatalf("Node.Edit materialized lazy payloads: parents=%d children=%d single=%d leaves=%d pending_delta=%d edit_pending=%d",
			arena.finalChildRefsMaterializedParents, arena.finalChildRefsMaterializedChildren,
			arena.finalChildRefsSingleChildMaterializedChildren, arena.compactFullLeafMaterialized,
			arena.pendingParentMaterialized-pendingBefore, arena.pendingParentMaterializedForEdit)
	}
}
