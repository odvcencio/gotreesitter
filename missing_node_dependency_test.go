package gotreesitter

import (
	"testing"
	"unsafe"
)

func newMissingDependencyTree(t *testing.T) (*Tree, *Node, missingNodeDependency) {
	t.Helper()
	arena := acquireNodeArena(arenaClassIncremental)
	dependency := missingNodeDependency{
		stackByte: 0, stackPoint: Point{},
		paddingBytes: 3, paddingExtent: Point{Column: 3},
		lookaheadBytes: 3,
	}
	node := newLeafNodeInArena(arena, 1, true, 3, 3, Point{Column: 3}, Point{Column: 3})
	node.setMissing(true)
	node.setHasError(true)
	if !arena.setMissingNodeDependency(node, dependency) {
		arena.Release()
		t.Fatal("set missing dependency")
	}
	return &Tree{root: node, arena: arena, source: []byte("abcXYZ")}, node, dependency
}

func TestMissingNodeDependencyLayoutAndOwnership(t *testing.T) {
	if got := unsafe.Sizeof(Node{}); got != 104 {
		t.Fatalf("Node size=%d, want 104", got)
	}
	if got := unsafe.Sizeof(missingNodeDependencyEntry{}); got != 40 {
		t.Fatalf("missing dependency entry size=%d, want 40", got)
	}
	tree, node, want := newMissingDependencyTree(t)
	defer tree.Release()
	if got, ok := missingNodeDependencyForNode(node); !ok || got != want {
		t.Fatalf("dependency=%+v exact=%t, want %+v", got, ok, want)
	}
	breakdown := tree.arena.collectArenaBreakdown()
	if breakdown.MissingNodeDependencyCount != 1 || breakdown.MissingNodeDependencyBytesAllocated < int64(unsafe.Sizeof(missingNodeDependencyEntry{})) {
		t.Fatalf("missing dependency telemetry count=%d bytes=%d", breakdown.MissingNodeDependencyCount, breakdown.MissingNodeDependencyBytesAllocated)
	}
	foreign := acquireNodeArena(arenaClassIncremental)
	defer foreign.Release()
	if foreign.setMissingNodeDependency(node, want) {
		t.Fatal("foreign arena accepted a missing dependency")
	}
}

func TestRecoveryMissingNodeDependencyMatchesLexerReset(t *testing.T) {
	parser := NewParser(nil)
	parser.included = []Range{{
		StartByte: 1, EndByte: 3,
		StartPoint: Point{Row: 1}, EndPoint: Point{Row: 1, Column: 2},
	}}
	dependency, exact := parser.recoveryMissingNodeDependency(0, Point{}, Token{
		StartByte: 1, EndByte: 2,
		StartPoint: Point{Row: 1}, EndPoint: Point{Row: 1, Column: 1},
		lexerLookaheadEndByte: 3,
	})
	want := missingNodeDependency{
		stackByte: 0, stackPoint: Point{},
		paddingBytes: 1, paddingExtent: Point{Row: 1},
		lookaheadBytes: 3,
	}
	if !exact || dependency != want {
		t.Fatalf("dependency=%+v exact=%t, want %+v", dependency, exact, want)
	}
}

func TestRecoveryMissingNodeDependencyUsesIncludedRangeStartPoint(t *testing.T) {
	parser := NewParser(nil)
	parser.included = []Range{{
		StartByte:  1,
		EndByte:    3,
		StartPoint: Point{Row: 7, Column: 9},
		EndPoint:   Point{Row: 7, Column: 11},
	}}
	dependency, exact := parser.recoveryMissingNodeDependency(1, Point{Column: 1}, Token{
		StartByte:             1,
		EndByte:               2,
		StartPoint:            Point{Row: 7, Column: 9},
		EndPoint:              Point{Row: 7, Column: 10},
		lexerLookaheadEndByte: 3,
	})
	if !exact {
		t.Fatal("recovery missing dependency was not exact")
	}
	if got, ok := dependency.positionedPoint(); !ok || got != (Point{Row: 7, Column: 9}) {
		t.Fatalf("positioned point=%+v exact=%t, want included start point", got, ok)
	}
}

func TestRecoveryMissingTokenUsesEmptyStackPosition(t *testing.T) {
	parser := NewParser(nil)
	parser.included = []Range{{
		StartByte: 1, EndByte: 3,
		StartPoint: Point{Row: 1}, EndPoint: Point{Row: 1, Column: 2},
	}}
	stack := &glrStack{}
	token, exact := parser.recoveryMissingToken([]byte("\n=i"), stack, 1, Token{
		StartByte: 1, EndByte: 2,
		StartPoint: Point{Row: 1}, EndPoint: Point{Row: 1, Column: 1},
		lexerLookaheadEndByte: 3,
	})
	if !exact || !token.Missing || token.StartByte != 1 || token.EndByte != 1 ||
		token.StartPoint != (Point{Row: 1}) || token.EndPoint != (Point{Row: 1}) ||
		!token.missingDependencyExact {
		t.Fatalf("missing token=%+v exact=%t", token, exact)
	}
	dependency, dependencyExact := missingNodeDependencyFromToken(token)
	if !dependencyExact || dependency.lookaheadBytes != 3 {
		t.Fatalf("token dependency=%+v exact=%t", dependency, dependencyExact)
	}
}

func TestMissingNodeDependencySurvivesTreeCopy(t *testing.T) {
	tree, _, want := newMissingDependencyTree(t)
	copyTree := tree.Copy()
	defer copyTree.Release()
	tree.Release()
	if got, ok := missingNodeDependencyForNode(copyTree.root); !ok || got != want {
		t.Fatalf("copied dependency=%+v exact=%t, want %+v", got, ok, want)
	}
}

func TestMissingNodeDependencyArenaAccountingIsExact(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	defer arena.Release()
	node := newLeafNodeInArena(arena, 1, true, 3, 3, Point{Column: 3}, Point{Column: 3})
	node.setMissing(true)
	dependency := missingNodeDependency{
		paddingBytes: 3, paddingExtent: Point{Column: 3}, lookaheadBytes: 3,
	}
	before := arena.allocatedBytes
	beforeCapacity := cap(arena.missingNodeDependencies)
	if !arena.setMissingNodeDependency(node, dependency) {
		t.Fatal("set missing dependency")
	}
	wantDelta := missingNodeDependencyEntryBytesForCap(cap(arena.missingNodeDependencies) - beforeCapacity)
	if got := arena.allocatedBytes - before; got != wantDelta {
		t.Fatalf("allocated byte delta=%d, want %d", got, wantDelta)
	}
	wantAllocated := arena.allocatedBytes
	arena.recomputeAllocatedBytes()
	if arena.allocatedBytes != wantAllocated {
		t.Fatalf("recomputed allocated bytes=%d, want %d", arena.allocatedBytes, wantAllocated)
	}
}

func TestMissingNodeDependencyLookupKeepsTwoEntriesDistinct(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	defer arena.Release()
	dependencies := []missingNodeDependency{
		{paddingBytes: 2, paddingExtent: Point{Column: 2}, lookaheadBytes: 3},
		{stackByte: 5, stackPoint: Point{Column: 5}, paddingBytes: 1, paddingExtent: Point{Column: 1}, lookaheadBytes: 4},
	}
	for index, dependency := range dependencies {
		position, _ := dependency.positionedByte()
		point, _ := dependency.positionedPoint()
		node := newLeafNodeInArena(arena, Symbol(index+1), true, position, position, point, point)
		node.setMissing(true)
		if !arena.setMissingNodeDependency(node, dependency) {
			t.Fatalf("set dependency %d", index)
		}
		if got, ok := missingNodeDependencyForNode(node); !ok || got != dependency {
			t.Fatalf("dependency %d=%+v exact=%t, want %+v", index, got, ok, dependency)
		}
	}
}

func TestMissingNodeDependencyInvalidatesPaddingAndLookahead(t *testing.T) {
	for offset := uint32(0); offset < 6; offset++ {
		t.Run(string(rune('0'+offset)), func(t *testing.T) {
			tree, node, _ := newMissingDependencyTree(t)
			defer tree.Release()
			tree.Edit(InputEdit{
				StartByte: offset, OldEndByte: offset + 1, NewEndByte: offset + 1,
				StartPoint: Point{Column: offset}, OldEndPoint: Point{Column: offset + 1}, NewEndPoint: Point{Column: offset + 1},
			})
			if !node.dirty() {
				t.Fatalf("edit at dependency byte %d did not dirty the missing leaf", offset)
			}
		})
	}
}

func TestMissingNodeDependencyTracksPaddingInsertion(t *testing.T) {
	tree, node, want := newMissingDependencyTree(t)
	defer tree.Release()
	tree.Edit(InputEdit{
		StartByte: 1, OldEndByte: 1, NewEndByte: 2,
		StartPoint: Point{Column: 1}, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 2},
	})
	if !node.dirty() {
		t.Fatal("padding insertion did not dirty the missing leaf")
	}
	want.paddingBytes++
	want.paddingExtent.Column++
	if node.startByte != 4 || node.endByte != 4 || node.startPoint != (Point{Column: 4}) || node.endPoint != (Point{Column: 4}) {
		t.Fatalf("positioned missing node=%d..%d %+v..%+v, want zero-width byte 4", node.startByte, node.endByte, node.startPoint, node.endPoint)
	}
	if got, ok := missingNodeDependencyForNode(node); !ok || got != want {
		t.Fatalf("edited dependency=%+v exact=%t, want %+v", got, ok, want)
	}
}

func TestMissingNodeDependencyInvalidatesInsertionAtStack(t *testing.T) {
	tree, node, want := newMissingDependencyTree(t)
	defer tree.Release()
	tree.Edit(InputEdit{
		StartByte: 0, OldEndByte: 0, NewEndByte: 1,
		StartPoint: Point{}, OldEndPoint: Point{}, NewEndPoint: Point{Column: 1},
	})
	want.paddingBytes++
	want.paddingExtent.Column++
	if !node.dirty() || node.startByte != 4 || node.endByte != 4 {
		t.Fatalf("stack insertion produced node=%d..%d dirty=%t, want dirty zero-width byte 4", node.startByte, node.endByte, node.dirty())
	}
	if got, ok := missingNodeDependencyForNode(node); !ok || got != want {
		t.Fatalf("edited dependency=%+v exact=%t, want %+v", got, ok, want)
	}
}

func TestMissingNodeDependencyTracksEditAcrossStackBoundary(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	dependency := missingNodeDependency{
		stackByte: 3, stackPoint: Point{Column: 3},
		paddingBytes: 2, paddingExtent: Point{Column: 2}, lookaheadBytes: 2,
	}
	node := newLeafNodeInArena(arena, 1, true, 5, 5, Point{Column: 5}, Point{Column: 5})
	node.setMissing(true)
	node.setHasError(true)
	if !arena.setMissingNodeDependency(node, dependency) {
		arena.Release()
		t.Fatal("set missing dependency")
	}
	tree := &Tree{root: node, arena: arena, source: []byte("abcdeXY")}
	defer tree.Release()
	tree.Edit(InputEdit{
		StartByte: 2, OldEndByte: 4, NewEndByte: 2,
		StartPoint: Point{Column: 2}, OldEndPoint: Point{Column: 4}, NewEndPoint: Point{Column: 2},
	})
	dependency.stackByte = 2
	dependency.stackPoint = Point{Column: 2}
	dependency.paddingBytes = 1
	dependency.paddingExtent = Point{Column: 1}
	if !node.dirty() || node.startByte != 3 || node.endByte != 3 {
		t.Fatalf("cross-stack edit produced node=%d..%d dirty=%t, want dirty zero-width byte 3", node.startByte, node.endByte, node.dirty())
	}
	if got, ok := missingNodeDependencyForNode(node); !ok || got != dependency {
		t.Fatalf("edited dependency=%+v exact=%t, want %+v", got, ok, dependency)
	}
}

func TestMissingNodeDependencyShiftsAfterEarlierEdit(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	dependency := missingNodeDependency{
		stackByte: 3, stackPoint: Point{Column: 3},
		paddingBytes: 2, paddingExtent: Point{Column: 2}, lookaheadBytes: 2,
	}
	node := newLeafNodeInArena(arena, 1, true, 5, 5, Point{Column: 5}, Point{Column: 5})
	node.setMissing(true)
	node.setHasError(true)
	if !arena.setMissingNodeDependency(node, dependency) {
		t.Fatal("set missing dependency")
	}
	tree := &Tree{root: node, arena: arena, source: []byte("abcdeXY")}
	defer tree.Release()
	tree.Edit(InputEdit{
		StartByte: 0, OldEndByte: 0, NewEndByte: 1,
		StartPoint: Point{}, OldEndPoint: Point{}, NewEndPoint: Point{Column: 1},
	})
	dependency.stackByte++
	dependency.stackPoint.Column++
	if node.startByte != 6 || node.endByte != 6 || node.dirty() {
		t.Fatalf("shifted node=%d..%d dirty=%t, want clean zero-width byte 6", node.startByte, node.endByte, node.dirty())
	}
	if got, ok := missingNodeDependencyForNode(node); !ok || got != dependency {
		t.Fatalf("shifted dependency=%+v exact=%t, want %+v", got, ok, dependency)
	}
}

func TestMissingNodeDependencyResetClearsPointers(t *testing.T) {
	tree, _, _ := newMissingDependencyTree(t)
	arena := tree.arena
	tree.Release()
	if len(arena.missingNodeDependencies) != 0 {
		t.Fatalf("arena reset retained %d missing dependencies", len(arena.missingNodeDependencies))
	}
}

func TestMissingNodeDependencyStaleReceiptFailsClosed(t *testing.T) {
	tree, node, _ := newMissingDependencyTree(t)
	defer tree.Release()
	node.startByte++
	node.endByte++
	tree.Edit(InputEdit{
		StartByte: 100, OldEndByte: 101, NewEndByte: 101,
		StartPoint: Point{Column: 100}, OldEndPoint: Point{Column: 101}, NewEndPoint: Point{Column: 101},
	})
	if !node.dirty() {
		t.Fatal("stale missing dependency allowed span-only edit skip")
	}
	node.setDirty(false)
	copyTree := tree.Copy()
	defer copyTree.Release()
	if !copyTree.root.dirty() {
		t.Fatal("stale missing dependency produced a clean copied node")
	}
}

func TestInputEditNoopUsesBytesOnlyAtMissingDependencyEnd(t *testing.T) {
	tree, node, want := newMissingDependencyTree(t)
	defer tree.Release()
	tree.Edit(InputEdit{
		StartByte: 6, OldEndByte: 6, NewEndByte: 6,
		StartPoint: Point{}, OldEndPoint: Point{Row: 1, Column: 7}, NewEndPoint: Point{Row: 2, Column: 3},
	})
	if node.dirty() {
		t.Fatal("byte-only no-op at dependency end dirtied the missing leaf")
	}
	if got, ok := missingNodeDependencyForNode(node); !ok || got != want {
		t.Fatalf("no-op changed dependency=%+v exact=%t, want %+v", got, ok, want)
	}
}

func TestTreeEditUpdatesIncludedRangesLikeC(t *testing.T) {
	tree := &Tree{includedRanges: []Range{
		{StartByte: 2, EndByte: 4, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 4}},
		{StartByte: 8, EndByte: 10, StartPoint: Point{Row: 1, Column: 2}, EndPoint: Point{Row: 1, Column: 4}},
	}}
	tree.Edit(InputEdit{
		StartByte: 3, OldEndByte: 5, NewEndByte: 6,
		StartPoint: Point{Column: 3}, OldEndPoint: Point{Column: 5}, NewEndPoint: Point{Column: 6},
	})
	want := []Range{
		{StartByte: 2, EndByte: 3, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 3}},
		{StartByte: 9, EndByte: 11, StartPoint: Point{Row: 1, Column: 2}, EndPoint: Point{Row: 1, Column: 4}},
	}
	if len(tree.includedRanges) != len(want) {
		t.Fatalf("included range count=%d, want %d", len(tree.includedRanges), len(want))
	}
	for i := range want {
		if tree.includedRanges[i] != want[i] {
			t.Fatalf("included range %d=%+v, want %+v", i, tree.includedRanges[i], want[i])
		}
	}
}

func TestTreeEditIncludedRangesShiftsPureInsertion(t *testing.T) {
	tree := &Tree{includedRanges: []Range{
		{StartByte: 0, EndByte: 1, StartPoint: Point{}, EndPoint: Point{Column: 1}},
		{StartByte: 3, EndByte: 5, StartPoint: Point{Column: 3}, EndPoint: Point{Column: 5}},
	}}
	tree.Edit(InputEdit{
		StartByte: 1, OldEndByte: 1, NewEndByte: 2,
		StartPoint: Point{Column: 1}, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 2},
	})
	want := []Range{
		{StartByte: 0, EndByte: 2, StartPoint: Point{}, EndPoint: Point{Column: 2}},
		{StartByte: 4, EndByte: 6, StartPoint: Point{Column: 4}, EndPoint: Point{Column: 6}},
	}
	for i := range want {
		if tree.includedRanges[i] != want[i] {
			t.Fatalf("included range %d=%+v, want %+v", i, tree.includedRanges[i], want[i])
		}
	}
}

func newMissingDependencyFinalChildTree(t *testing.T) (*Tree, *Node, *Node) {
	t.Helper()
	arena := acquireNodeArena(arenaClassIncremental)
	arena.finalChildRefs = true
	dependency := missingNodeDependency{
		stackByte: 0, stackPoint: Point{},
		paddingBytes: 3, paddingExtent: Point{Column: 3},
		lookaheadBytes: 3,
	}
	missing := newLeafNodeInArena(arena, 1, true, 3, 3, Point{Column: 3}, Point{Column: 3})
	missing.setMissing(true)
	missing.setHasError(true)
	if !arena.setMissingNodeDependency(missing, dependency) {
		arena.Release()
		t.Fatal("set missing dependency")
	}
	parent := newPendingParentInArena(arena, 2, true, 4,
		[]stackEntry{newStackEntryNode(missing.parseState, missing)},
		0, 8, Point{}, Point{Column: 8}, true)
	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil || !nodeHasFinalChildRefs(root) {
		arena.Release()
		t.Fatal("materialize final child refs")
	}
	return &Tree{root: root, arena: arena, source: []byte("abcXYZ123")}, root, missing
}

func TestTreeEditFinalChildSkipConsultsMissingDependency(t *testing.T) {
	tree, root, missing := newMissingDependencyFinalChildTree(t)
	defer tree.Release()
	tree.Edit(InputEdit{
		StartByte: 4, OldEndByte: 4, NewEndByte: 5,
		StartPoint: Point{Column: 4}, OldEndPoint: Point{Column: 4}, NewEndPoint: Point{Column: 5},
	})
	if !missing.dirty() {
		t.Fatal("final-child skip ignored a missing descendant dependency")
	}
	if len(root.children) != 0 || root.ownerArena.finalChildRefsMaterializedParents != 0 {
		t.Fatalf("edit materialized final child refs: children=%d parents=%d", len(root.children), root.ownerArena.finalChildRefsMaterializedParents)
	}
}

func TestEditPendingParentSkipConsultsMissingDependency(t *testing.T) {
	tree, missing, _ := newMissingDependencyTree(t)
	defer tree.Release()
	parent := newPendingParentInArena(tree.arena, 2, true, 4,
		[]stackEntry{newStackEntryNode(missing.parseState, missing)},
		0, 3, Point{}, Point{Column: 3}, true)
	editStackEntryWithDelta(tree.arena, newStackEntryPendingParent(parent.parseState, parent), InputEdit{
		StartByte: 4, OldEndByte: 4, NewEndByte: 5,
		StartPoint: Point{Column: 4}, OldEndPoint: Point{Column: 4}, NewEndPoint: Point{Column: 5},
	}, 1, 0, true, nil, nil)
	if !missing.dirty() {
		t.Fatal("pending-parent skip ignored a missing descendant dependency")
	}
	if got := parent.childEntry(tree.arena, 0); stackEntryNode(got) != missing {
		t.Fatal("pending edit replaced the child node")
	}
}
