package gotreesitter

import (
	"strings"
	"testing"
)

func TestReclaimRawShapeStorageClearsArenaPayloadRefs(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	defer arena.Release()

	node := arena.allocNodeFast()
	noTree := arena.allocNoTreeNode()
	compact := arena.allocCompactFullLeaf()
	pending := arena.allocPendingParent()
	checkpoint := arena.allocCompactCheckpointLeaf()

	ref, shape := arena.allocRawShape()
	if ref == 0 || shape == nil {
		t.Fatal("allocRawShape returned no shape")
	}
	shape.childRange = arena.allocRawShapeChildren(1)
	shapeLimit := maxRetainedRawShapeCapacityForClass(arena.class)
	for i := 1; i < shapeLimit+defaultRawShapeSlabCap(arena.class); i++ {
		if _, allocated := arena.allocRawShape(); allocated == nil {
			t.Fatalf("allocRawShape failed at %d", i)
		}
	}
	childLimit := maxRetainedRawShapeChildCapacityForClass(arena.class)
	arena.allocRawShapeChildren(childLimit + 1)
	for _, raw := range []*rawShapeRef{
		&node.rawShape,
		&noTree.rawShape,
		&compact.rawShape,
		&pending.rawShape,
		&checkpoint.rawShape,
	} {
		*raw = ref
	}

	rawBytes := arena.rawShapeBytesAllocated() + arena.rawShapeChildBytesAllocated()
	if rawBytes == 0 {
		t.Fatal("raw-shape allocation = 0 before reclamation")
	}
	allocatedBefore := arena.allocatedBytes
	arena.reclaimRawShapeStorage()

	retainedRawBytes := arena.rawShapeBytesAllocated() + arena.rawShapeChildBytesAllocated()
	maxRetainedRawBytes := rawShapeBytesForCap(shapeLimit) + rawShapeChildBytesForCap(childLimit)
	if retainedRawBytes > maxRetainedRawBytes {
		t.Fatalf("retained raw-shape bytes = %d, bounded maximum %d", retainedRawBytes, maxRetainedRawBytes)
	}
	if retainedRawBytes >= rawBytes {
		t.Fatalf("retained raw-shape bytes = %d, want less than parse allocation %d", retainedRawBytes, rawBytes)
	}
	if arena.rawShapeSlabCursor != 0 || arena.rawShapeChildSlabCursor != 0 {
		t.Fatalf("raw-shape cursors = (%d,%d), want zeroes", arena.rawShapeSlabCursor, arena.rawShapeChildSlabCursor)
	}
	for i, slab := range arena.rawShapeSlabs {
		if slab.used != 0 {
			t.Fatalf("raw-shape slab %d used = %d, want 0", i, slab.used)
		}
	}
	for i, slab := range arena.rawShapeChildSlabs {
		if slab.used != 0 {
			t.Fatalf("raw-shape child slab %d used = %d, want 0", i, slab.used)
		}
	}
	if got, want := arena.allocatedBytes, allocatedBefore-(rawBytes-retainedRawBytes); got != want {
		t.Fatalf("allocated bytes after reclamation = %d, want %d", got, want)
	}
	for name, raw := range map[string]rawShapeRef{
		"node":               node.rawShape,
		"no-tree":            noTree.rawShape,
		"compact-full-leaf":  compact.rawShape,
		"pending-parent":     pending.rawShape,
		"checkpoint-compact": checkpoint.rawShape,
	} {
		if raw != 0 {
			t.Fatalf("%s raw-shape ref = %d, want 0", name, raw)
		}
	}
}

// TestParseReclaimsRawShapeStorageAfterAccounting exercises the reclaim/
// accounting invariant on a genuinely forking parse (buildPrefixForkLanguage,
// proven to fork by TestPrefixForkLanguageActuallyForks in
// raw_shape_elision_test.go). A single-stack input like "1+2+3+4" no longer
// captures any raw shapes at all under the single-stack capture elision (see
// gssScratch.mayElideRawShape / spore.2026-07-12.hazel.rawshape-elision-rca),
// so it can no longer exercise "nonzero captured, then reclaimed" here; see
// TestParseReclaimsRawShapeStorageAccountsForZeroOnSingleStackParse below for
// that (now-zero) single-stack case.
func TestParseReclaimsRawShapeStorageAfterAccounting(t *testing.T) {
	EnableArenaBreakdown(true)
	defer EnableArenaBreakdown(false)

	lang := buildPrefixForkLanguage()
	tree := mustParse(t, NewParser(lang), []byte("x x y"))
	defer tree.Release()

	rt := tree.ParseRuntime()
	if rt.MaxStacksSeen <= 1 {
		t.Fatalf("MaxStacksSeen = %d, want > 1 (this test requires a genuinely forking parse)", rt.MaxStacksSeen)
	}
	breakdown := assertParseRuntimeArenaBreakdown(t, tree, rt)
	parseRawBytes := breakdown.RawShapeBytesAllocated + breakdown.RawShapeChildBytesAllocated + breakdown.RawShapeHashCacheBytesAllocated
	if parseRawBytes == 0 {
		t.Fatal("parse-time raw-shape bytes = 0, want captured allocation once the parse has forked")
	}
	assertTreeRawShapeStorageReclaimed(t, tree)
	retainedRawBytes := tree.arena.rawShapeBytesAllocated() + tree.arena.rawShapeChildBytesAllocated() + tree.arena.rawShapeHashCacheBytesAllocated()
	if got, want := tree.ParseRuntime().ArenaBytesAllocated-tree.arena.allocatedBytes, parseRawBytes-retainedRawBytes; got != want {
		t.Fatalf("retained arena reduction = %d, want reclaimed raw-shape allocation %d", got, want)
	}
	assertReachableRawShapeRefsCleared(t, tree.RootNode())
}

// TestParseReclaimsRawShapeStorageAccountsForZeroOnSingleStackParse is the
// single-stack counterpart the RCA asked for: a pure single-stack parse
// (buildArithmeticLanguage has no GLR conflicts) now elides every
// captureRawShape call, so the arena-breakdown accounting invariant must hold
// with parse-time raw-shape bytes at zero, not just at some captured-then-
// reclaimed nonzero value.
func TestParseReclaimsRawShapeStorageAccountsForZeroOnSingleStackParse(t *testing.T) {
	DrainArenaPools()
	EnableArenaBreakdown(true)
	defer EnableArenaBreakdown(false)

	lang := buildArithmeticLanguage()
	// Pin to production: this test asserts a production-engine internal (arena
	// raw-shape reclaim breakdown) the compact candidate route does not report.
	parser := NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree := mustParse(t, parser, []byte("1+2+3+4"))
	defer tree.Release()

	rt := tree.ParseRuntime()
	if rt.MaxStacksSeen > 1 {
		t.Fatalf("MaxStacksSeen = %d, want <= 1 (arithmetic grammar has no GLR conflicts)", rt.MaxStacksSeen)
	}
	breakdown := assertParseRuntimeArenaBreakdown(t, tree, rt)
	parseRawBytes := breakdown.RawShapeBytesAllocated + breakdown.RawShapeChildBytesAllocated + breakdown.RawShapeHashCacheBytesAllocated
	if parseRawBytes != 0 {
		t.Fatalf("parse-time raw-shape bytes = %d, want 0 on a pure single-stack parse (elision should skip every capture)", parseRawBytes)
	}
	assertTreeRawShapeStorageReclaimed(t, tree)
	retainedRawBytes := tree.arena.rawShapeBytesAllocated() + tree.arena.rawShapeChildBytesAllocated() + tree.arena.rawShapeHashCacheBytesAllocated()
	if got, want := tree.ParseRuntime().ArenaBytesAllocated-tree.arena.allocatedBytes, parseRawBytes-retainedRawBytes; got != want {
		t.Fatalf("retained arena reduction = %d, want reclaimed raw-shape allocation %d", got, want)
	}
	assertReachableRawShapeRefsCleared(t, tree.RootNode())
}

func TestIncrementalParseAfterRawShapeReclamationMatchesFresh(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	oldTree := mustParse(t, parser, []byte("1+2+3"))
	defer oldTree.Release()
	assertTreeRawShapeStorageReclaimed(t, oldTree)
	root := oldTree.RootNode()
	if child := root.Child(0); child == nil || child.Parent() != root {
		t.Fatal("parent access failed after raw-shape reclamation")
	}
	cursor := NewTreeCursorFromTree(oldTree)
	if !cursor.GotoFirstChild() || cursor.CurrentNode() == nil {
		t.Fatal("cursor access failed after raw-shape reclamation")
	}
	query, err := NewQuery(`(expression) @expr`, lang)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}
	if matches := query.Execute(oldTree); len(matches) == 0 {
		t.Fatal("query access failed after raw-shape reclamation")
	}

	oldTree.Edit(InputEdit{
		StartByte:   4,
		OldEndByte:  5,
		NewEndByte:  5,
		StartPoint:  Point{Column: 4},
		OldEndPoint: Point{Column: 5},
		NewEndPoint: Point{Column: 5},
	})
	source := []byte("1+2+4")
	incremental := mustParseIncremental(t, parser, source, oldTree)
	defer incremental.Release()
	fresh := mustParse(t, NewParser(lang), source)
	defer fresh.Release()

	if got, want := incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental tree = %s, fresh = %s", got, want)
	}
	assertTreeRawShapeStorageReclaimed(t, incremental)
	assertReachableRawShapeRefsCleared(t, incremental.RootNode())
	assertTreeRawShapeStorageReclaimed(t, fresh)
}

func TestParseForestExperimentalReclaimsRawShapeStorage(t *testing.T) {
	lang := loadBlobForDecode(t, "json")
	tree, ok := NewParser(lang).ParseForestExperimental([]byte(`[1, 2, 3]`))
	if !ok || tree == nil {
		t.Fatal("ParseForestExperimental failed")
	}
	defer tree.Release()

	assertTreeRawShapeStorageReclaimed(t, tree)
	assertReachableRawShapeRefsCleared(t, tree.RootNode())
}

func TestTryForestFastPathReclaimsRawShapeStorage(t *testing.T) {
	lang := loadBlobForDecode(t, "json")
	lang.WantsForest = true
	previous := glrForestEnabled
	glrForestEnabled = true
	defer func() { glrForestEnabled = previous }()

	tree := NewParser(lang).tryForestFastPath([]byte(`[1, 2, 3]`))
	if tree == nil {
		t.Fatal("tryForestFastPath declined")
	}
	defer tree.Release()
	if !tree.forestFastPath {
		t.Fatal("returned tree is not marked as forest fast path")
	}
	assertTreeRawShapeStorageReclaimed(t, tree)
	assertReachableRawShapeRefsCleared(t, tree.RootNode())
}

func BenchmarkReturnedTreeRawShapeRetention(b *testing.B) {
	EnableArenaBreakdown(true)
	defer EnableArenaBreakdown(false)
	lang := buildArithmeticLanguage()
	source := []byte("1" + strings.Repeat("+1", 127))
	parser := NewParser(lang)
	var parseRawBytes int64
	var retainedRawBytes int64

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		if breakdown, ok := tree.ArenaBreakdown(); ok {
			parseRawBytes += breakdown.RawShapeBytesAllocated + breakdown.RawShapeChildBytesAllocated + breakdown.RawShapeHashCacheBytesAllocated
		}
		if tree.arena != nil {
			retainedRawBytes += tree.arena.rawShapeBytesAllocated() + tree.arena.rawShapeChildBytesAllocated() + tree.arena.rawShapeHashCacheBytesAllocated()
		}
		tree.Release()
	}
	b.StopTimer()
	b.ReportMetric(float64(parseRawBytes)/float64(b.N), "parse-raw-shape-B/op")
	b.ReportMetric(float64(retainedRawBytes)/float64(b.N), "retained-raw-shape-B/op")
}

func BenchmarkReclaimRawShapeStorageScan(b *testing.B) {
	const nodeCount = 64 * 1024
	b.ReportAllocs()
	b.ReportMetric(nodeCount, "nodes/op")
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		arena := acquireNodeArena(arenaClassFull)
		arena.ensureNodeCapacity(nodeCount)
		for j := 0; j < nodeCount; j++ {
			node := arena.allocNodeFast()
			node.ownerArena = arena
			node.rawShape = 1
		}
		if ref, shape := arena.allocRawShape(); ref == 0 || shape == nil {
			b.Fatal("allocRawShape returned no shape")
		}
		arena.allocRawShapeChildren(nodeCount)
		b.StartTimer()
		arena.reclaimRawShapeStorage()
		b.StopTimer()
		arena.Release()
	}
}

func assertTreeRawShapeStorageReclaimed(t *testing.T, tree *Tree) {
	t.Helper()
	if tree == nil || tree.arena == nil {
		t.Fatal("tree has no owned arena")
	}
	retained := tree.arena.rawShapeBytesAllocated() + tree.arena.rawShapeChildBytesAllocated()
	maximum := rawShapeBytesForCap(maxRetainedRawShapeCapacityForClass(tree.arena.class)) +
		rawShapeChildBytesForCap(maxRetainedRawShapeChildCapacityForClass(tree.arena.class))
	if retained > maximum {
		t.Fatalf("returned arena retains %d raw-shape bytes, bounded maximum %d", retained, maximum)
	}
	for i, slab := range tree.arena.rawShapeSlabs {
		if slab.used != 0 {
			t.Fatalf("returned raw-shape slab %d used = %d, want 0", i, slab.used)
		}
	}
	for i, slab := range tree.arena.rawShapeChildSlabs {
		if slab.used != 0 {
			t.Fatalf("returned raw-shape child slab %d used = %d, want 0", i, slab.used)
		}
	}
}

func assertReachableRawShapeRefsCleared(t *testing.T, root *Node) {
	t.Helper()
	Walk(root, func(node *Node, _ int) WalkAction {
		if node.rawShape != 0 {
			t.Errorf("reachable node %d retains raw-shape ref %d", node.symbol, node.rawShape)
			return WalkStop
		}
		return WalkContinue
	})
}
