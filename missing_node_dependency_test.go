package gotreesitter

import (
	"fmt"
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
		!token.missingDependencyExact() {
		t.Fatalf("missing token=%+v exact=%t", token, exact)
	}
	dependency, dependencyExact := parser.missingNodeDependencyFromToken(token)
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

// freshMissingLeaf returns a zero-width missing leaf that arena has just
// allocated, with a dependency that fits it. Leaves differ in position and
// lookahead by index, so equal entries cannot pass for each other.
func freshMissingLeaf(arena *nodeArena, index int) (*Node, missingNodeDependency) {
	stack := uint32(index) * 4
	at := stack + 2
	dependency := missingNodeDependency{
		stackByte: stack, stackPoint: Point{Column: stack},
		paddingBytes: 2, paddingExtent: Point{Column: 2},
		lookaheadBytes: uint32(index%5) + 2,
	}
	node := newLeafNodeInArena(arena, Symbol(index%7+1), true, at, at, Point{Column: at}, Point{Column: at})
	node.setMissing(true)
	node.setHasError(true)
	return node, dependency
}

// plantedMissingNodeDependency is the dependency that
// plantMissingNodeDependencyForNextNode records.
var plantedMissingNodeDependency = missingNodeDependency{
	stackByte: 1, stackPoint: Point{Column: 1},
	paddingBytes: 1, paddingExtent: Point{Column: 1}, lookaheadBytes: 1,
}

// plantMissingNodeDependencyForNextNode adds an entry for the node that arena
// allocates next, and returns that node. It stands in for a list that holds a
// match. A call that searches the list finds the entry when the next leaf
// arrives and rewrites it in place; a call that does not search leaves it alone.
func plantMissingNodeDependencyForNextNode(t *testing.T, arena *nodeArena) *Node {
	t.Helper()
	if arena.used >= len(arena.nodes) {
		t.Fatal("the arena's primary slab is full, so its next node cannot be named")
	}
	next := &arena.nodes[arena.used]
	arena.missingNodeDependencies = append(arena.missingNodeDependencies,
		missingNodeDependencyEntry{node: next, dependency: plantedMissingNodeDependency})
	return next
}

// checkPlantedMissingNodeDependencyKept checks the list after one leaf took the
// planted node: the planted entry is untouched and the leaf's own entry follows.
func checkPlantedMissingNodeDependencyKept(t *testing.T, arena *nodeArena, planted, leaf *Node, want missingNodeDependency) {
	t.Helper()
	if leaf != planted {
		t.Fatal("the leaf is not the node that the test planted an entry for")
	}
	if got := len(arena.missingNodeDependencies); got != 2 {
		t.Fatalf("list has %d entries, want the planted entry and the leaf's own: recording a new leaf must not search the list", got)
	}
	if got := arena.missingNodeDependencies[0]; got.node != planted || got.dependency != plantedMissingNodeDependency {
		t.Fatalf("planted entry is now %+v, want it untouched", got.dependency)
	}
	if got := arena.missingNodeDependencies[1]; got.node != leaf || got.dependency != want {
		t.Fatalf("leaf entry is %+v, want %+v", got.dependency, want)
	}
}

// TestSetNewMissingNodeDependencyMatchesSetMissingNodeDependency pins that the
// no-search setter writes what the searching setter writes, one leaf at a time:
// the same entries in the same order, the same capacity growth, and the same
// byte accounting.
func TestSetNewMissingNodeDependencyMatchesSetMissingNodeDependency(t *testing.T) {
	// Enough appends to grow the list through several capacities.
	const leaves = 700
	searching := newNodeArena(arenaClassIncremental)
	appending := newNodeArena(arenaClassIncremental)
	for index := 0; index < leaves; index++ {
		a, dependency := freshMissingLeaf(searching, index)
		b, _ := freshMissingLeaf(appending, index)
		beforeCap, beforeBytes := cap(appending.missingNodeDependencies), appending.allocatedBytes
		if !searching.setMissingNodeDependency(a, dependency) || !appending.setNewMissingNodeDependency(b, dependency) {
			t.Fatalf("leaf %d: a setter rejected a dependency that fits", index)
		}
		if got, want := len(appending.missingNodeDependencies), len(searching.missingNodeDependencies); got != want {
			t.Fatalf("leaf %d: list has %d entries, want %d", index, got, want)
		}
		if got, want := cap(appending.missingNodeDependencies), cap(searching.missingNodeDependencies); got != want {
			t.Fatalf("leaf %d: list capacity %d, want %d", index, got, want)
		}
		if got, want := appending.allocatedBytes, searching.allocatedBytes; got != want {
			t.Fatalf("leaf %d: allocated bytes %d, want %d", index, got, want)
		}
		wantDelta := missingNodeDependencyEntryBytesForCap(cap(appending.missingNodeDependencies) - beforeCap)
		if got := appending.allocatedBytes - beforeBytes; got != wantDelta {
			t.Fatalf("leaf %d: allocated byte delta %d, want %d from the list capacity", index, got, wantDelta)
		}
	}
	for index, want := range searching.missingNodeDependencies {
		got := appending.missingNodeDependencies[index]
		if got.dependency != want.dependency || got.node.symbol != want.node.symbol || got.node.startByte != want.node.startByte {
			t.Fatalf("entry %d = %+v at byte %d, want %+v at byte %d", index, got.dependency, got.node.startByte, want.dependency, want.node.startByte)
		}
		if read, ok := missingNodeDependencyForNode(got.node); !ok || read != want.dependency {
			t.Fatalf("entry %d reads back as %+v exact=%t, want %+v", index, read, ok, want.dependency)
		}
	}
	wantBytes := appending.allocatedBytes
	appending.recomputeAllocatedBytes()
	if appending.allocatedBytes != wantBytes {
		t.Fatalf("recomputed allocated bytes %d, want %d", appending.allocatedBytes, wantBytes)
	}
}

// TestSetNewMissingNodeDependencyAppliesTheSameChecks pins that every input the
// searching setter refuses, the no-search setter refuses too, and that a
// refusal leaves the list alone.
func TestSetNewMissingNodeDependencyAppliesTheSameChecks(t *testing.T) {
	type input struct {
		arena      *nodeArena
		node       *Node
		dependency missingNodeDependency
	}
	build := func(edit func(in *input)) input {
		arena := newNodeArena(arenaClassIncremental)
		node, dependency := freshMissingLeaf(arena, 1)
		in := input{arena: arena, node: node, dependency: dependency}
		if edit != nil {
			edit(&in)
		}
		return in
	}
	cases := []struct {
		name string
		edit func(in *input)
		want bool
	}{
		{name: "fits", want: true},
		{name: "nil arena", edit: func(in *input) { in.arena = nil }},
		{name: "nil node", edit: func(in *input) { in.node = nil }},
		{name: "node of another arena", edit: func(in *input) {
			other := newNodeArena(arenaClassIncremental)
			in.node, _ = freshMissingLeaf(other, 1)
		}},
		{name: "node is not missing", edit: func(in *input) { in.node.setMissing(false) }},
		{name: "node has width", edit: func(in *input) { in.node.endByte++ }},
		{name: "node points differ", edit: func(in *input) { in.node.endPoint.Column++ }},
		{name: "padding stops short of the node", edit: func(in *input) { in.dependency.paddingBytes-- }},
		{name: "padding extent stops short of the node", edit: func(in *input) { in.dependency.paddingExtent.Column-- }},
		{name: "positioned byte overflows", edit: func(in *input) { in.dependency.stackByte = ^uint32(0) }},
		{name: "lookahead end overflows", edit: func(in *input) { in.dependency.lookaheadBytes = ^uint32(0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, setter := range []struct {
				name string
				set  func(a *nodeArena, n *Node, d missingNodeDependency) bool
			}{
				{"searching", (*nodeArena).setMissingNodeDependency},
				{"no-search", (*nodeArena).setNewMissingNodeDependency},
			} {
				in := build(tc.edit)
				if got := setter.set(in.arena, in.node, in.dependency); got != tc.want {
					t.Fatalf("%s setter returned %t, want %t", setter.name, got, tc.want)
				}
				if in.arena == nil {
					continue
				}
				wantEntries := 0
				if tc.want {
					wantEntries = 1
				}
				if got := len(in.arena.missingNodeDependencies); got != wantEntries {
					t.Fatalf("%s setter left %d entries, want %d", setter.name, got, wantEntries)
				}
			}
		})
	}
}

// TestSetNewMissingNodeDependencyDoesNotSearchTheList pins the split between
// the two setters. The searching setter updates a node's entry in place, which
// the edit and clone paths rely on. The no-search setter never looks at the
// list, so for a node that already has an entry it leaves two. That outcome is
// what shows the call skipped the search; callers must not rely on it.
func TestSetNewMissingNodeDependencyDoesNotSearchTheList(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	node, first := freshMissingLeaf(arena, 1)
	second := first
	second.lookaheadBytes += 3
	if !arena.setNewMissingNodeDependency(node, first) {
		t.Fatal("first record")
	}

	if !arena.setMissingNodeDependency(node, second) {
		t.Fatal("searching setter rejected a dependency that fits")
	}
	if len(arena.missingNodeDependencies) != 1 {
		t.Fatalf("searching setter left %d entries, want the existing entry updated in place", len(arena.missingNodeDependencies))
	}
	if got, ok := missingNodeDependencyForNode(node); !ok || got != second {
		t.Fatalf("after the in-place update the dependency is %+v exact=%t, want %+v", got, ok, second)
	}

	if !arena.setNewMissingNodeDependency(node, first) {
		t.Fatal("no-search setter rejected a dependency that fits")
	}
	if len(arena.missingNodeDependencies) != 2 {
		t.Fatalf("no-search setter left %d entries, want 2: it must not search the list", len(arena.missingNodeDependencies))
	}
	if got := arena.missingNodeDependencies[0].dependency; got != second {
		t.Fatalf("no-search setter changed the existing entry to %+v, want %+v", got, second)
	}
}

func BenchmarkSetMissingNodeDependencyFreshLeaves(b *testing.B) {
	for _, leaves := range []int{1 << 10, 1 << 12, 1 << 14} {
		for _, setter := range []struct {
			name string
			set  func(a *nodeArena, n *Node, d missingNodeDependency) bool
		}{
			{"searching", (*nodeArena).setMissingNodeDependency},
			{"no-search", (*nodeArena).setNewMissingNodeDependency},
		} {
			b.Run(fmt.Sprintf("%s/leaves=%d", setter.name, leaves), func(b *testing.B) {
				arena := newNodeArena(arenaClassFull)
				nodes := make([]*Node, leaves)
				dependencies := make([]missingNodeDependency, leaves)
				for index := range nodes {
					nodes[index], dependencies[index] = freshMissingLeaf(arena, index)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					arena.missingNodeDependencies = arena.missingNodeDependencies[:0]
					for index, node := range nodes {
						if !setter.set(arena, node, dependencies[index]) {
							b.Fatal("set missing dependency")
						}
					}
				}
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*leaves), "ns/leaf")
			})
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

// Recovery in Haskell and Elsa can leave many nested error-bearing parents
// before a later edit. Each dependency check must descend each node once.
func TestTreeEditSkipsDeepMissingDependencyBeforeReplacement(t *testing.T) {
	tree, missing, want := newMissingDependencyTree(t)
	defer tree.Release()
	chain := missing
	for range 64 {
		chain = newParentNodeInArena(tree.arena, 2, true, []*Node{chain}, nil, 0)
	}
	if stackEntryEndsBeforeEditDependency(tree.arena, newStackEntryNode(chain.parseState, chain), 5) {
		t.Fatal("missing lookahead was skipped before its dependency end")
	}
	later := newLeafNodeInArena(tree.arena, 3, true, 8, 9, Point{Column: 8}, Point{Column: 9})
	tree.root = newParentNodeInArena(tree.arena, 4, true, []*Node{chain, later}, nil, 0)
	tree.source = []byte("abcXYZ--z")
	tree.Edit(InputEdit{
		StartByte: 8, OldEndByte: 9, NewEndByte: 9,
		StartPoint: Point{Column: 8}, OldEndPoint: Point{Column: 9}, NewEndPoint: Point{Column: 9},
	})
	if missing.dirty() || chain.dirty() || !later.dirty() || !tree.root.dirty() {
		t.Fatalf("replacement dirtiness: missing=%t chain=%t later=%t root=%t", missing.dirty(), chain.dirty(), later.dirty(), tree.root.dirty())
	}
	if got, ok := missingNodeDependencyForNode(missing); !ok || got != want {
		t.Fatalf("unaffected missing dependency=%+v exact=%t, want %+v", got, ok, want)
	}
}

func BenchmarkMissingNodeDependencyErrorChain(b *testing.B) {
	for _, depth := range []int{8, 16} {
		b.Run(map[int]string{8: "depth8", 16: "depth16"}[depth], func(b *testing.B) {
			arena := acquireNodeArena(arenaClassIncremental)
			defer arena.Release()
			node := newLeafNodeInArena(arena, 1, true, 3, 3, Point{Column: 3}, Point{Column: 3})
			node.setMissing(true)
			node.setHasError(true)
			if !arena.setMissingNodeDependency(node, missingNodeDependency{paddingBytes: 3, paddingExtent: Point{Column: 3}, lookaheadBytes: 3}) {
				b.Fatal("set missing dependency")
			}
			for range depth {
				node = newParentNodeInArena(arena, 2, true, []*Node{node}, nil, 0)
			}
			entry := newStackEntryNode(node.parseState, node)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if !stackEntryEndsBeforeEditDependency(arena, entry, 8) {
					b.Fatal("chain should precede the edit")
				}
			}
		})
	}
}

func TestNestedErrorDependencyBoundary(t *testing.T) {
	tree, missing, _ := newMissingDependencyTree(t)
	defer tree.Release()
	root := missing
	for i := 0; i < 20; i++ {
		root = newParentNodeInArena(tree.arena, 2, true, []*Node{root}, nil, 0)
		root.setHasError(true)
	}
	for _, start := range []uint32{3, 4, 5, 6, 7} {
		entry := newStackEntryNode(0, root)
		if got := stackEntryEndsBeforeEditDependency(tree.arena, entry, start); got != (start > 6) {
			t.Fatalf("nested missing dependency at edit %d: before=%t", start, got)
		}
	}
	// The replacement lies after every visible span but within the missing
	// token's lexer dependency. Tree.Edit must still mark the entire path.
	tree.root = root
	tree.Edit(InputEdit{StartByte: 4, OldEndByte: 5, NewEndByte: 5, StartPoint: Point{Column: 4}, OldEndPoint: Point{Column: 5}, NewEndPoint: Point{Column: 5}})
	if !missing.dirty() || !root.dirty() {
		t.Fatal("nested dependency was dropped")
	}
}

func BenchmarkNestedErrorDependency(b *testing.B) {
	arena := acquireNodeArena(arenaClassIncremental)
	defer arena.Release()
	root := newLeafNodeInArena(arena, 1, true, 3, 3, Point{Column: 3}, Point{Column: 3})
	root.setMissing(true)
	root.setHasError(true)
	if !arena.setMissingNodeDependency(root, missingNodeDependency{paddingBytes: 3, paddingExtent: Point{Column: 3}, lookaheadBytes: 3}) {
		b.Fatal("missing dependency")
	}
	for i := 0; i < 16; i++ {
		root = newParentNodeInArena(arena, 2, true, []*Node{root}, nil, 0)
		root.setHasError(true)
	}
	entry := newStackEntryNode(0, root)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !stackEntryEndsBeforeEditDependency(arena, entry, 7) {
			b.Fatal("dependency beyond edit")
		}

	}
}

func TestStackEntryMissingReceiptStillChecksErrorChildren(t *testing.T) {
	tree, missing, _ := newMissingDependencyTree(t)
	defer tree.Release()
	missing.children = []*Node{{endByte: 8}}
	if !nodeEndsBeforeEditDependency(missing, 7) {
		t.Fatal("missing receipt must end before the edit")
	}
	if stackEntryEndsBeforeEditDependency(tree.arena, stackEntry{node: unsafe.Pointer(missing)}, 7) {
		t.Fatal("missing error node must still check its descendants")
	}
}

func TestEditDependencyMissingReceiptStillChecksErrorChildren(t *testing.T) {
	tree, missing, _ := newMissingDependencyTree(t)
	defer tree.Release()
	missing.children = []*Node{{endByte: 8}}
	if !nodeEndsBeforeEditDependency(missing, 7) {
		t.Fatal("missing receipt must end before the edit")
	}
	if stackEntryEndsBeforeEditDependency(tree.arena, stackEntry{node: unsafe.Pointer(missing)}, 7) {
		t.Fatal("missing error node must still check its descendants")
	}
}

func TestStackEntryEditDependencyDeepErrorAncestors(t *testing.T) {
	for _, lazy := range []bool{false, true} {
		name := "materialized"
		if lazy {
			name = "final-child-refs"
		}
		t.Run(name, func(t *testing.T) {
			tree, leaf, _ := newMissingDependencyTree(t)
			defer tree.Release()
			arena := tree.arena
			arena.finalChildRefs = lazy
			root := leaf
			for depth := 0; depth < 48; depth++ {
				if lazy {
					parent := newPendingParentInArena(arena, 2, true, 4,
						[]stackEntry{newStackEntryNode(root.parseState, root)},
						3, 3, Point{Column: 3}, Point{Column: 3}, true)
					entry := newStackEntryPendingParent(parent.parseState, parent)
					root = materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
				} else {
					root = newParentNodeInArena(arena, 2, true, []*Node{root}, nil, 0)
				}
				if root == nil || !root.hasError() {
					t.Fatalf("depth %d lost the missing descendant's error", depth)
				}
			}
			tree.root = root
			entry := newStackEntryNode(root.parseState, root)
			for _, boundary := range []struct {
				start uint32
				want  bool
			}{{3, false}, {6, false}, {7, true}} {
				if got := stackEntryEndsBeforeEditDependency(arena, entry, boundary.start); got != boundary.want {
					t.Fatalf("edit start %d: ends before dependency=%t, want %t", boundary.start, got, boundary.want)
				}
			}
			if lazy && (len(root.children) != 0 || arena.finalChildRefsMaterializedParents != 0) {
				t.Fatal("dependency check materialized the final child references")
			}
		})
	}
}

func TestFreshMissingDependencyArenaResetAndUpdate(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	defer arena.Release()
	first, dep := freshMissingLeaf(arena, 3)
	if !arena.setNewMissingNodeDependency(first, dep) {
		t.Fatal("first fresh dependency rejected")
	}
	updated := dep
	updated.lookaheadBytes++
	if !arena.setMissingNodeDependency(first, updated) || len(arena.missingNodeDependencies) != 1 {
		t.Fatal("existing dependency was not updated in place")
	}
	if got, ok := missingNodeDependencyForNode(first); !ok || got != updated {
		t.Fatalf("updated dependency = %+v, exact=%t", got, ok)
	}
	arena.reset()
	if len(arena.missingNodeDependencies) != 0 {
		t.Fatal("reset retained old entries")
	}
	next, nextDep := freshMissingLeaf(arena, 4)
	if next != first {
		t.Fatal("test did not exercise arena slot reuse")
	}
	if !arena.setNewMissingNodeDependency(next, nextDep) {
		t.Fatal("fresh dependency after reset rejected")
	}
	if got, ok := missingNodeDependencyForNode(next); !ok || got != nextDep {
		t.Fatalf("new dependency = %+v, exact=%t", got, ok)
	}
}
