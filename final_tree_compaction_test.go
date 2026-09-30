package gotreesitter

import (
	"bytes"
	"sync"
	"testing"
)

func TestFinalTreeCompactionWorthwhileForMeasuredLargeTree(t *testing.T) {
	stats := finalTreeMaterializationStats{
		nodes:               3_321_766,
		parentNodes:         872_327,
		leafNodes:           2_449_439,
		nodeFieldMetadata:   566_534,
		fieldedParentNodes:  566_534,
		childPointers:       3_321_765,
		fieldIDElements:     1_756_684,
		fieldSourceElements: 1_756_684,
	}
	estimated, ok := estimateCompactedFinalTreeBytes(stats)
	if !ok {
		t.Fatal("estimateCompactedFinalTreeBytes rejected measured counts")
	}
	const retained = int64(834_842_568)
	if !finalTreeCompactionWorthwhile(retained, estimated, 2<<30) {
		t.Fatalf("measured large tree should compact: retained=%d estimated=%d", retained, estimated)
	}
	if finalTreeCompactionWorthwhile(retained, estimated, 1<<30) {
		t.Fatal("compaction should be rejected without coexistence headroom")
	}
}

func TestFinalTreeCompactionWorthwhileRejectsMarginalSavings(t *testing.T) {
	tests := []struct {
		name      string
		retained  int64
		compacted int64
		budget    int64
	}{
		{name: "below absolute threshold", retained: 700 << 20, compacted: 500 << 20, budget: 2 << 30},
		{name: "below fractional threshold", retained: 1 << 30, compacted: 750 << 20, budget: 3 << 30},
		{name: "no savings", retained: 700 << 20, compacted: 700 << 20, budget: 2 << 30},
		{name: "no budget", retained: 700 << 20, compacted: 300 << 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if finalTreeCompactionWorthwhile(tt.retained, tt.compacted, tt.budget) {
				t.Fatal("marginal or unsafe compaction was accepted")
			}
		})
	}
}

func TestMaybeCompactReturnedFullTreePreservesTreeBehavior(t *testing.T) {
	lang := queryTestLanguage()
	source := []byte("ab")
	arena := acquireNodeArena(arenaClassFull)

	left := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	left.parseState = 7
	right := newLeafNodeInArena(arena, Symbol(2), true, 1, 2, Point{Column: 1}, Point{Column: 2})
	right.parseState = 9
	children := arena.allocNodeSlice(2)
	children[0], children[1] = left, right
	fields := arena.allocFieldIDSlice(2)
	fields[0], fields[1] = FieldID(1), FieldID(2)
	root := newParentNode(arena, Symbol(7), true, children, fields, 3)
	root.parseState = 11

	tree := newTreeWithArenas(root, source, lang, arena, nil)
	tree.setParseRuntime(ParseRuntime{
		StopReason:      ParseStopAccepted,
		ExpectedEOFByte: uint32(len(source)),
		RootEndByte:     uint32(len(source)),
	})
	fastStats := collectMaterializedFinalTreeStorageStats(root)
	fullStats := collectFinalTreeMaterializationStats(root, lang)
	if fastStats.nodes != fullStats.nodes ||
		fastStats.parentNodes != fullStats.parentNodes ||
		fastStats.leafNodes != fullStats.leafNodes ||
		fastStats.nodeFieldMetadata != fullStats.nodeFieldMetadata ||
		fastStats.childPointers != fullStats.childPointers ||
		fastStats.fieldIDElements != fullStats.fieldIDElements ||
		fastStats.fieldSourceElements != fullStats.fieldSourceElements {
		t.Fatalf("storage stats differ: fast=%+v full=%+v", fastStats, fullStats)
	}
	beforeSExpr := tree.RootNode().SExpr(lang)
	oldRoot := tree.root
	oldArena := tree.arena

	// Model a large post-parse arena without allocating hundreds of MiB in a
	// unit test. The compaction decision uses retained backing, while the clone
	// itself still exercises real arena storage and ownership.
	oldArena.allocatedBytes = 800 << 20
	parser := &Parser{language: lang}
	got := parser.maybeCompactReturnedFullTreeWithBudget(tree, source, 2<<30)
	if got != tree {
		t.Fatal("compaction replaced the Tree wrapper")
	}
	if tree.arena == oldArena {
		t.Fatal("tree retained the oversized arena")
	}
	if tree.root == oldRoot {
		t.Fatal("tree retained the original root node")
	}
	if refs := oldArena.refs.Load(); refs != 0 {
		t.Fatalf("old arena refs = %d, want 0", refs)
	}
	defer tree.Release()

	newRoot := tree.RootNode()
	if got := newRoot.SExpr(lang); got != beforeSExpr {
		t.Fatalf("S-expression changed: got %q, want %q", got, beforeSExpr)
	}
	if got := newRoot.FieldNameForChild(0, lang); got != "name" {
		t.Fatalf("first child field = %q, want name", got)
	}
	if got := newRoot.FieldNameForChild(1, lang); got != "body" {
		t.Fatalf("second child field = %q, want body", got)
	}
	if child := newRoot.Child(0); child == nil || child.Parent() != newRoot || child.parseState != 7 {
		t.Fatal("first child parent link or parser state changed")
	}

	cursor := NewTreeCursorFromTree(tree)
	if !cursor.GotoFirstChild() || cursor.CurrentNode() != newRoot.Child(0) {
		t.Fatal("cursor could not enter the compacted tree")
	}
	query, err := NewQuery(`(identifier) @ident`, lang)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}
	if matches := query.Execute(tree); len(matches) != 1 {
		t.Fatalf("query matches = %d, want 1", len(matches))
	}
}

func TestFinalTreeCompactionEligibilityIsNarrow(t *testing.T) {
	lang := testLanguage()
	source := []byte("x")
	arena := acquireNodeArena(arenaClassFull)
	root := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	tree := newTreeWithArenas(root, source, lang, arena, nil)
	tree.setParseRuntime(ParseRuntime{StopReason: ParseStopAccepted, RootEndByte: 1, ExpectedEOFByte: 1})
	parser := &Parser{language: lang}
	if !eligibleForFinalTreeCompaction(parser, tree, source) {
		t.Fatal("fresh accepted full parse should be eligible")
	}
	tree.deferResultCompatibility()
	if eligibleForFinalTreeCompaction(parser, tree, source) {
		t.Fatal("deferred compatibility tree should be excluded")
	}
	tree.resultCompatibilityFinalizer.Store(nil)
	tree.externalScannerCheckpointsDeferred = true
	if eligibleForFinalTreeCompaction(parser, tree, source) {
		t.Fatal("deferred external checkpoints should be excluded")
	}
	tree.externalScannerCheckpointsDeferred = false
	tree.arena.finalChildRefs = true
	if eligibleForFinalTreeCompaction(parser, tree, source) {
		t.Fatal("lazy final-child refs should be excluded")
	}
	tree.arena.finalChildRefs = false
	tree.Release()
}

func TestIncrementalParseAfterFinalTreeCompactionMatchesFresh(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	// Pin to production: this test asserts a production-engine internal (final
	// tree compaction), which the compact candidate route does not perform.
	parser.SetAdmissionCandidateRoute(false)
	source := []byte("1+2+3")
	oldTree := mustParse(t, parser, source)
	oldArena := oldTree.arena
	oldArena.allocatedBytes = 800 << 20
	parser.maybeCompactReturnedFullTreeWithBudget(oldTree, source, 2<<30)
	if oldTree.arena == oldArena {
		t.Fatal("test tree was not compacted")
	}
	defer oldTree.Release()

	oldTree.Edit(InputEdit{
		StartByte:   4,
		OldEndByte:  5,
		NewEndByte:  5,
		StartPoint:  Point{Column: 4},
		OldEndPoint: Point{Column: 5},
		NewEndPoint: Point{Column: 5},
	})
	updated := []byte("1+2+4")
	incremental := mustParseIncremental(t, parser, updated, oldTree)
	defer incremental.Release()
	fresh := mustParse(t, NewParser(lang), updated)
	defer fresh.Release()

	if got, want := incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental tree = %s, fresh = %s", got, want)
	}
}

func TestConcurrentReadsAfterFinalTreeCompaction(t *testing.T) {
	lang := buildArithmeticLanguage()
	source := []byte("1+2+3+4")
	parser := NewParser(lang)
	// Pin to production: this test asserts a production-engine internal (final
	// tree compaction), which the compact candidate route does not perform.
	parser.SetAdmissionCandidateRoute(false)
	tree := mustParse(t, parser, source)
	oldArena := tree.arena
	oldArena.allocatedBytes = 800 << 20
	parser.maybeCompactReturnedFullTreeWithBudget(tree, source, 2<<30)
	if tree.arena == oldArena {
		t.Fatal("test tree was not compacted")
	}
	defer tree.Release()

	const readers = 8
	errors := make(chan string, readers)
	var wg sync.WaitGroup
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				root := tree.RootNode()
				first := root.Child(0)
				if first == nil || first.Parent() != root {
					errors <- "parent link changed"
					return
				}
				if next := first.NextSibling(); next != nil && next.PrevSibling() != first {
					errors <- "sibling links changed"
					return
				}
				cursor := NewTreeCursorFromTree(tree)
				if !cursor.GotoFirstChild() || cursor.CurrentNode() == nil {
					errors <- "cursor traversal failed"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for message := range errors {
		t.Fatal(message)
	}
}

func TestCloneTreePreservesRecordedExternalScannerCheckpoint(t *testing.T) {
	sourceArena := acquireNodeArena(arenaClassIncremental)
	defer sourceArena.Release()
	destinationArena := acquireNodeArena(arenaClassIncremental)
	defer destinationArena.Release()

	leaf := newLeafNodeInArena(sourceArena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	startState := []byte{1, 2, 3}
	endState := []byte{4, 5, 6}
	if !sourceArena.recordExternalScannerLeafCheckpoint(leaf, startState, endState) {
		t.Fatal("recordExternalScannerLeafCheckpoint failed")
	}
	clone := cloneTreeNodesIntoArena(leaf, destinationArena)
	cp, ok := externalScannerCheckpointRefForNode(clone)
	if !ok {
		t.Fatal("cloned checkpoint not found")
	}
	if got := destinationArena.externalScannerSnapshotBytes(cp.start); !bytes.Equal(got, startState) {
		t.Fatalf("cloned start state = %v, want %v", got, startState)
	}
	if got := destinationArena.externalScannerSnapshotBytes(cp.end); !bytes.Equal(got, endState) {
		t.Fatalf("cloned end state = %v, want %v", got, endState)
	}
}
