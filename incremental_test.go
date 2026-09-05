package gotreesitter

import "testing"

func TestTreeEditShiftsNodes(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Parse "1+2"
	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}

	// Simulate inserting "0" before "1": "01+2"
	// Edit: at byte 0, old end 0, new end 1 (inserted 1 byte)
	tree.Edit(InputEdit{
		StartByte:   0,
		OldEndByte:  0,
		NewEndByte:  1,
		StartPoint:  Point{0, 0},
		OldEndPoint: Point{0, 0},
		NewEndPoint: Point{0, 1},
	})

	// After edit, the root's end should shift by 1.
	if root.EndByte() != 4 {
		t.Errorf("root EndByte after edit = %d, want 4", root.EndByte())
	}

	// The edit should be recorded.
	if len(tree.Edits()) != 1 {
		t.Fatalf("expected 1 edit recorded, got %d", len(tree.Edits()))
	}
}

func TestParseIncremental(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Parse "1+2"
	tree := mustParse(t, parser, []byte("1+2"))

	// Edit: change to "1+3"
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{0, 2},
		OldEndPoint: Point{0, 3},
		NewEndPoint: Point{0, 3},
	})

	// Incremental re-parse with new source.
	newTree := mustParseIncremental(t, parser, []byte("1+3"), tree)
	root := newTree.RootNode()
	if root == nil {
		t.Fatal("incremental parse returned nil root")
	}

	// Should have the same structure: expression(expression(NUMBER), +, NUMBER)
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	num := root.Child(2)
	if num.Text(newTree.Source()) != "3" {
		t.Errorf("changed NUMBER text = %q, want %q", num.Text(newTree.Source()), "3")
	}
}

func TestHighlightIncremental(t *testing.T) {
	lang := buildArithmeticLanguage()

	// Simple highlight query: capture NUMBER nodes.
	h, err := NewHighlighter(lang, `(NUMBER) @number`)
	if err != nil {
		t.Fatal(err)
	}

	// Initial highlight.
	source1 := []byte("1+2")
	ranges1 := h.Highlight(source1)
	if len(ranges1) < 2 {
		t.Fatalf("expected at least 2 highlight ranges, got %d", len(ranges1))
	}

	// Parse for incremental use.
	parser := NewParser(lang)
	tree := mustParse(t, parser, source1)

	// Edit: "1+2" -> "1+3"
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{0, 2},
		OldEndPoint: Point{0, 3},
		NewEndPoint: Point{0, 3},
	})

	source2 := []byte("1+3")
	ranges2, newTree := h.HighlightIncremental(source2, tree)
	if newTree == nil {
		t.Fatal("HighlightIncremental returned nil tree")
	}

	// Should still have at least 2 number ranges.
	if len(ranges2) < 2 {
		t.Fatalf("expected at least 2 incremental highlight ranges, got %d", len(ranges2))
	}

	// Verify the captures are "number".
	for _, r := range ranges2 {
		if r.Capture != "number" {
			t.Errorf("unexpected capture %q, want %q", r.Capture, "number")
		}
	}
}

func TestParseIncrementalReusesUnchangedLeaf(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	// Exercise ordinary production reuse and its borrowed arena ownership.
	parser.SetAdmissionCandidateRoute(false)

	oldSource := []byte("1+2+3")
	tree := mustParse(t, parser, oldSource)
	root := tree.RootNode()
	if root == nil {
		t.Fatal("initial parse returned nil root")
	}
	oldRight := root.Child(2)
	if oldRight == nil {
		t.Fatal("missing right child in initial tree")
	}

	// Edit the middle number: "1+2+3" -> "1+4+3"
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{0, 2},
		OldEndPoint: Point{0, 3},
		NewEndPoint: Point{0, 3},
	})

	newSource := []byte("1+4+3")
	newTree := mustParseIncremental(t, parser, newSource, tree)
	newRoot := newTree.RootNode()
	if newRoot == nil {
		t.Fatal("incremental parse returned nil root")
	}
	newRight := newRoot.Child(2)
	if newRight == nil {
		t.Fatal("missing right child in incremental tree")
	}

	if newRight != oldRight {
		t.Fatal("expected unchanged right leaf node to be reused")
	}
	if got := newRight.Text(newTree.Source()); got != "3" {
		t.Fatalf("reused leaf text = %q, want %q", got, "3")
	}
	assertTreeHasNoDirtyNodes(t, newRoot)
}

func TestTreeEditTracksEditedLeafHint(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2+3"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("initial parse returned nil root")
	}
	mid := root.DescendantForByteRange(2, 3)
	if mid == nil {
		t.Fatal("missing edited leaf in initial tree")
	}

	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{0, 2},
		OldEndPoint: Point{0, 3},
		NewEndPoint: Point{0, 3},
	})

	if tree.lastEditedLeaf == nil {
		t.Fatal("expected lastEditedLeaf to be tracked")
	}
	if tree.lastEditedLeaf != mid {
		t.Fatal("expected lastEditedLeaf to point at edited leaf")
	}
}

func TestParseIncrementalReusesRootWhenUnchanged(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	source := []byte("1+2")
	tree := mustParse(t, parser, source)
	if tree.RootNode() == nil {
		t.Fatal("initial parse returned nil root")
	}

	// No edits: incremental parse should be able to reuse the whole root subtree.
	newTree := mustParseIncremental(t, parser, source, tree)
	if newTree.RootNode() == nil {
		t.Fatal("incremental parse returned nil root")
	}

	if newTree.RootNode() != tree.RootNode() {
		t.Fatal("expected root node to be reused when there are no edits")
	}
}

func TestParseIncrementalReusesRootAfterUndo(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	// Pin to production: a compact-materialized base tree is hard-barred from
	// incremental reuse (decision 0008), so the reuse assertion needs a
	// production base tree. Reuse-bar lift is the follow-on campaign.
	parser.SetAdmissionCandidateRoute(false)

	source := []byte("1+2+3")
	tree := mustParse(t, parser, source)
	oldRoot := tree.RootNode()
	if oldRoot == nil {
		t.Fatal("initial parse returned nil root")
	}

	// Edit and undo before reparsing: "1+2+3" -> "1+4+3" -> "1+2+3".
	edit := InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{0, 2},
		OldEndPoint: Point{0, 3},
		NewEndPoint: Point{0, 3},
	}
	tree.Edit(edit)
	tree.Edit(edit)

	newTree := mustParseIncremental(t, parser, source, tree)
	if newTree.RootNode() == nil {
		t.Fatal("incremental parse returned nil root")
	}
	if newTree.RootNode() != oldRoot {
		t.Fatal("expected root node to be reused after undo")
	}
	if newTree.RootNode().dirty() {
		t.Fatal("expected reused root to have dirty flag cleared after undo reuse")
	}
}

func TestTreeEditNodesAfterEdit(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2+3"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}

	origEnd := root.EndByte()

	// Delete the "+3" at end: "1+2+3" -> "1+2"
	// Edit: start=3, oldEnd=5, newEnd=3
	tree.Edit(InputEdit{
		StartByte:   3,
		OldEndByte:  5,
		NewEndByte:  3,
		StartPoint:  Point{0, 3},
		OldEndPoint: Point{0, 5},
		NewEndPoint: Point{0, 3},
	})

	// Root should shrink.
	if root.EndByte() != 3 {
		t.Errorf("root EndByte after deletion = %d, want 3 (was %d)", root.EndByte(), origEnd)
	}
}

func TestParseIncrementalReleaseKeepsBorrowedNodesAlive(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	// Pin to production: a compact-materialized base tree is hard-barred from
	// incremental reuse (decision 0008), so the reuse assertion needs a
	// production base tree. Reuse-bar lift is the follow-on campaign.
	parser.SetAdmissionCandidateRoute(false)

	oldSrc := []byte("1+2+3")
	oldTree := mustParse(t, parser, oldSrc)
	oldRoot := oldTree.RootNode()
	if oldRoot == nil {
		t.Fatal("initial parse returned nil root")
	}
	oldRight := oldRoot.Child(2)
	if oldRight == nil {
		t.Fatal("missing right leaf in initial tree")
	}
	oldArena := oldRight.ownerArena
	if oldArena == nil {
		t.Fatal("expected reused leaf to have an owning arena")
	}

	oldTree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{0, 2},
		OldEndPoint: Point{0, 3},
		NewEndPoint: Point{0, 3},
	})

	newSrc := []byte("1+4+3")
	newTree := mustParseIncremental(t, parser, newSrc, oldTree)
	newRight := newTree.RootNode().Child(2)
	if newRight == nil {
		t.Fatal("missing right leaf in incremental tree")
	}
	if newRight != oldRight {
		t.Fatal("expected right leaf to be reused")
	}
	if oldArena.refs.Load() < 2 {
		t.Fatalf("expected borrowed arena to be retained by new tree, refs=%d", oldArena.refs.Load())
	}
	if newTree.arena == nil || newTree.arena == oldArena || newTree.RootNode() == oldRoot {
		t.Fatal("expected the reparse to allocate a new primary arena and root")
	}
	if len(newTree.borrowedArena) != 1 || newTree.borrowedArena[0] != oldArena {
		t.Fatalf("expected the reused leaf arena as the sole borrowed owner, got %v", newTree.borrowedArena)
	}
	fresh := mustParse(t, parser, newSrc)
	defer fresh.Release()
	assertReleaseRootTreeEqual(t, newTree.RootNode(), fresh.RootNode(), lang)

	oldTree.Release()
	oldTree.Release() // idempotent
	if oldArena.refs.Load() < 1 {
		t.Fatalf("borrowed arena refcount dropped too far after old tree release: %d", oldArena.refs.Load())
	}

	// Force arena churn to validate that borrowed nodes are retained correctly.
	for i := 0; i < 2000; i++ {
		tmp := mustParse(t, parser, []byte("7+8"))
		if tmp.RootNode() == nil {
			t.Fatalf("tmp parse %d returned nil root", i)
		}
		tmp.Release()
	}

	if got := newRight.Text(newTree.Source()); got != "3" {
		t.Fatalf("reused right leaf text after old release = %q, want %q", got, "3")
	}

	newTree.Release()
	newTree.Release() // idempotent
	if oldArena.refs.Load() != 0 {
		t.Fatalf("borrowed arena should be fully released after new tree release, refs=%d", oldArena.refs.Load())
	}
}

func TestParseReuseStateRetainsTransitiveBorrowedArenaLifecycle(t *testing.T) {
	inherited := acquireNodeArena(arenaClassFull)
	child := newLeafNodeInArena(inherited, 1, true, 0, 1, Point{}, Point{Column: 1})
	oldest := newTreeWithArenas(child, []byte("x"), nil, inherited, nil)

	direct := acquireNodeArena(arenaClassIncremental)
	parent := newParentNodeInArena(direct, 2, true, []*Node{child}, nil, 0)
	inherited.Retain()
	old := newTreeWithArenas(parent, []byte("x"), nil, direct, []*nodeArena{inherited})

	primary := acquireNodeArena(arenaClassIncremental)
	state := parseReuseState{}
	state.markReused(parent, primary)
	newest := newTreeWithArenas(parent, []byte("x"), nil, primary, state.retainBorrowed(primary))
	if len(newest.borrowedArena) != 2 {
		t.Fatalf("newest tree borrowed arenas=%d, want two reachable owners", len(newest.borrowedArena))
	}

	oldest.Release()
	old.Release()
	if got := child.EndByte(); got != 1 {
		t.Fatalf("transitively borrowed child end byte=%d after prior releases, want 1", got)
	}
	if got := child.Type(&Language{SymbolNames: []string{"EOF", "child"}}); got != "child" {
		t.Fatalf("transitively borrowed child type=%q after prior releases, want child", got)
	}
	if direct.refs.Load() != 1 || inherited.refs.Load() != 1 {
		t.Fatalf("newest tree refs direct=%d inherited=%d, want 1/1", direct.refs.Load(), inherited.refs.Load())
	}

	newest.Release()
	if direct.refs.Load() != 0 || inherited.refs.Load() != 0 || primary.refs.Load() != 0 {
		t.Fatalf("released refs direct=%d inherited=%d primary=%d, want 0/0/0", direct.refs.Load(), inherited.refs.Load(), primary.refs.Load())
	}
}

func assertTreeHasNoDirtyNodes(t *testing.T, root *Node) {
	t.Helper()
	if root == nil {
		return
	}
	stack := []*Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.dirty() {
			t.Fatalf("found dirty node sym=%d at [%d,%d)", n.symbol, n.startByte, n.endByte)
		}
		for i := len(n.children) - 1; i >= 0; i-- {
			if child := n.children[i]; child != nil {
				stack = append(stack, child)
			}
		}
	}
}

func TestTryReuseSubtreeReusesFirstEligibleNonLeafCandidate(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	oldSource := []byte("1+2+3+4")
	newSource := []byte("9+2+3+4")
	oldTree := mustParse(t, parser, oldSource)

	var reuseScratch reuseScratch
	reuse := (&reuseCursor{}).reset(oldTree, newSource, &reuseScratch)
	if reuse == nil {
		t.Fatal("reuse cursor reset returned nil")
	}

	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	stack := newGLRStackWithScratch(lang.InitialState, &entryScratch)

	// Force the non-leaf fallback path by using a non-matching lookahead symbol.
	lookahead := Token{
		Symbol:     2,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{Row: 0, Column: 0},
		EndPoint:   Point{Row: 0, Column: 1},
	}
	candidates := reuse.candidates(lookahead.StartByte)
	var expected *Node
	var expectedState StateID
	var expectedSpan uint32
	for _, n := range candidates {
		if n == nil || n.ChildCount() == 0 || n.Parent() == nil {
			continue
		}
		span := n.EndByte() - n.StartByte()
		if span == 0 || span > 2048 {
			continue
		}
		if _, ok := parser.reuseTargetState(stack.top().state, n, lookahead); !ok {
			continue
		}
		expected = n
		expectedState, _ = parser.reuseTargetState(stack.top().state, n, lookahead)
		expectedSpan = span
		break
	}
	if expected == nil {
		t.Fatal("expected at least one eligible non-leaf reuse candidate")
	}

	ts := &stubTokenSource{
		tokens: []Token{
			{Symbol: 2, StartByte: expected.EndByte(), EndByte: expected.EndByte() + 1},
			{Symbol: 0, StartByte: uint32(len(newSource)), EndByte: uint32(len(newSource))},
		},
	}
	nextTok, reusedBytes, ok := parser.tryReuseSubtree(&stack, lookahead, ts, reuse, &entryScratch, &gssScratch)
	if !ok {
		t.Fatal("expected non-leaf fallback reuse to succeed")
	}
	if stackEntryNode(stack.top()) != expected {
		got := stackEntryNode(stack.top())
		if got == nil {
			t.Fatalf("reused wrong non-leaf candidate: got nil want span=%d", expectedSpan)
		}
		t.Fatalf("reused wrong non-leaf candidate: got span=%d want span=%d", got.EndByte()-got.StartByte(), expectedSpan)
	}
	if stack.top().state != expectedState {
		t.Fatalf("stack top state = %d, want %d", stack.top().state, expectedState)
	}
	if reusedBytes != expectedSpan {
		t.Fatalf("reusedBytes = %d, want %d", reusedBytes, expectedSpan)
	}
	if nextTok.StartByte < expected.EndByte() {
		t.Fatalf("next token did not advance past reused subtree: next=%d reusedEnd=%d", nextTok.StartByte, expected.EndByte())
	}
}

func TestReuseNodePreservesFreshDynamicPrecedenceSelection(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Fresh reductions credit their dynamic precedence directly to the stack.
	// Give the fresh lineage the later branch order so an exact score tie must
	// select the incremental lineage below; without reuseNode restoring the
	// subtree's cumulative credit, the fresh lineage wins on score instead.
	var freshEntries glrEntryScratch
	var freshGSS gssScratch
	reducedSubtree := func() *Node {
		leaf := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})
		parent := NewParentNode(3, true, []*Node{leaf}, nil, 0)
		parent.dynamicPrecedence = 7
		return parent
	}
	freshNode := reducedSubtree()
	fresh := newGLRStackWithScratch(lang.InitialState, &freshEntries)
	fresh.branchOrder = 2
	parser.pushStackNode(&fresh, 1, freshNode, &freshEntries, &freshGSS)
	var reduced bool
	markReduceApplied(&fresh, ParseAction{DynamicPrecedence: 7}, &reduced)
	if !reduced {
		t.Fatal("fresh reduction was not recorded")
	}

	var incrementalEntries glrEntryScratch
	var incrementalGSS gssScratch
	reusedNode := reducedSubtree()
	incremental := newGLRStackWithScratch(lang.InitialState, &incrementalEntries)
	incremental.branchOrder = 1
	_, reusedBytes, ok := reuseNode(
		parser,
		&incremental,
		reusedNode,
		1,
		lang.InitialState,
		Token{},
		nil,
		&reuseCursor{sourceLen: 1},
		&incrementalEntries,
		&incrementalGSS,
		externalScannerCheckpointRef{},
	)
	if !ok || reusedBytes != 1 {
		t.Fatalf("reuseNode = (bytes=%d, ok=%v), want (1, true)", reusedBytes, ok)
	}
	stacks := []glrStack{fresh, incremental}
	parser.promotePrimaryStack(stacks)
	if got := stackEntryNode(stacks[0].top()); got != reusedNode {
		t.Fatalf("fresh/incremental score skew changed the selected lineage: incremental=%d fresh=%d", incremental.score, fresh.score)
	}
}

// nonLeafReuseSpanFixture builds the shared oldTree/newSource/parser fixture
// used by TestTryReuseSubtreeSkipsLargeNonLeafCandidate and
// TestTryReuseSubtreeSkipsFragileNonLeafCandidate: a two-level tree
// (root -> large -> leaf) spanning sourceLen bytes, with a single edit at
// byte 0 so the reuse cursor has something to walk past.
func nonLeafReuseSpanFixture(t *testing.T, sourceLen int, markFragile bool) (*Parser, *reuseCursor, []byte) {
	t.Helper()
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	oldSource := make([]byte, sourceLen)
	newSource := make([]byte, len(oldSource))
	copy(newSource, oldSource)
	newSource[0] = 1

	leaf := NewLeafNode(1, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	leaf.parseState = 1
	large := NewParentNode(3, true, []*Node{leaf}, nil, 0)
	large.parseState = 2
	large.endByte = uint32(len(oldSource))
	large.endPoint = Point{Row: 0, Column: uint32(len(oldSource))}
	if markFragile {
		large.setFragileLeft(true)
		large.setFragileRight(true)
	}
	root := NewParentNode(3, true, []*Node{large}, nil, 0)
	root.parseState = 2
	root.endByte = uint32(len(oldSource))
	root.endPoint = Point{Row: 0, Column: uint32(len(oldSource))}
	oldTree := NewTree(root, oldSource, lang)

	var reuseScratch reuseScratch
	reuse := (&reuseCursor{}).reset(oldTree, newSource, &reuseScratch)
	if reuse == nil {
		t.Fatal("reuse cursor reset returned nil")
	}
	return parser, reuse, newSource
}

// TestTryReuseSubtreeSkipsLargeNonLeafCandidate proves the non-leaf reuse
// lane (reuseNonLeafTargetStateOnStack, incremental.go) still rejects
// candidates whose span exceeds maxNonLeafReuseSpan. maxNonLeafReuseSpan was
// raised from 2048 to 1<<20 once fragility marking (markReduceFragility,
// parser_reduce.go) became the real soundness gate for interior reuse -- see
// TestTryReuseSubtreeSkipsFragileNonLeafCandidate for that gate -- so this
// candidate must be sized just over the new bound to still exercise the span
// cutoff specifically, independent of fragility.
func TestTryReuseSubtreeSkipsLargeNonLeafCandidate(t *testing.T) {
	const overCap = (1 << 20) + 4096
	parser, reuse, newSource := nonLeafReuseSpanFixture(t, overCap, false)

	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	stack := newGLRStackWithScratch(parser.language.InitialState, &entryScratch)

	lookahead := Token{
		Symbol:     2,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{Row: 0, Column: 0},
		EndPoint:   Point{Row: 0, Column: 1},
	}
	ts := &stubTokenSource{tokens: []Token{{Symbol: 0, StartByte: uint32(len(newSource)), EndByte: uint32(len(newSource))}}}
	nextTok, reusedBytes, ok := parser.tryReuseSubtree(&stack, lookahead, ts, reuse, &entryScratch, &gssScratch)
	if ok {
		t.Fatalf("expected large non-leaf candidate to be rejected by span cutoff, reusedBytes=%d nextTok=%+v", reusedBytes, nextTok)
	}
	if stackEntryNode(stack.top()) != nil {
		t.Fatal("stack should remain unchanged when reuse fails")
	}
	if reuse.rejectLargeNonLeaf == 0 {
		t.Fatal("expected rejectLargeNonLeaf to record the span-cutoff rejection")
	}
}

// TestTryReuseSubtreeSkipsFragileNonLeafCandidate proves the non-leaf reuse
// lane rejects a candidate marked fragile (Node.isFragile(), tree.go) even
// though it is well within maxNonLeafReuseSpan -- the actual soundness gate
// issue #380 required before the span cap could be safely raised. See
// markReduceFragility (parser_reduce.go) for how a real parse sets these
// bits, and the maxNonLeafReuseSpan comment (incremental.go) for why span
// alone is no longer the gate.
func TestTryReuseSubtreeSkipsFragileNonLeafCandidate(t *testing.T) {
	const smallSpan = 64
	parser, reuse, newSource := nonLeafReuseSpanFixture(t, smallSpan, true)

	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	stack := newGLRStackWithScratch(parser.language.InitialState, &entryScratch)

	lookahead := Token{
		Symbol:     2,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{Row: 0, Column: 0},
		EndPoint:   Point{Row: 0, Column: 1},
	}
	ts := &stubTokenSource{tokens: []Token{{Symbol: 0, StartByte: uint32(len(newSource)), EndByte: uint32(len(newSource))}}}
	nextTok, reusedBytes, ok := parser.tryReuseSubtree(&stack, lookahead, ts, reuse, &entryScratch, &gssScratch)
	if ok {
		t.Fatalf("expected fragile non-leaf candidate to be rejected, reusedBytes=%d nextTok=%+v", reusedBytes, nextTok)
	}
	if stackEntryNode(stack.top()) != nil {
		t.Fatal("stack should remain unchanged when reuse fails")
	}
	if reuse.rejectFragileNonLeaf == 0 {
		t.Fatal("expected rejectFragileNonLeaf to record the fragility rejection")
	}

	// Control: the same fixture with the fragile bits unset must actually be
	// reused, so the rejection above is proven to come from fragility and not
	// some other property of the fixture.
	parser2, reuse2, newSource2 := nonLeafReuseSpanFixture(t, smallSpan, false)
	stack2 := newGLRStackWithScratch(parser2.language.InitialState, &entryScratch)
	ts2 := &stubTokenSource{tokens: []Token{{Symbol: 0, StartByte: uint32(len(newSource2)), EndByte: uint32(len(newSource2))}}}
	if _, _, ok := parser2.tryReuseSubtree(&stack2, lookahead, ts2, reuse2, &entryScratch, &gssScratch); !ok {
		t.Fatal("expected non-fragile control candidate to be reused")
	}
}

func TestReuseTargetStateAmbiguousShiftMustMatchNodeState(t *testing.T) {
	lang := buildArithmeticLanguage()
	ambiguousActionIdx := uint16(len(lang.ParseActions))
	lang.ParseActions = append(lang.ParseActions, ParseActionEntry{
		Actions: []ParseAction{
			{Type: ParseActionShift, State: 7},
			{Type: ParseActionShift, State: 9},
		},
	})
	lang.ParseTable[0][1] = ambiguousActionIdx
	parser := NewParser(lang)

	lookahead := Token{Symbol: 1}
	leaf := &Node{symbol: 1, parseState: 9}
	nextState, ok := parser.reuseTargetState(0, leaf, lookahead)
	if !ok {
		t.Fatal("expected reuseTargetState to accept matching shift state in ambiguous set")
	}
	if nextState != 9 {
		t.Fatalf("reuseTargetState returned state %d, want 9", nextState)
	}

	leaf.parseState = 8
	if _, ok := parser.reuseTargetState(0, leaf, lookahead); ok {
		t.Fatal("expected reuseTargetState to reject ambiguous shift when node parseState does not match any action")
	}
}

func TestLeafReuseRejectsMismatchedNodeState(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	lookahead := Token{Symbol: 1}

	leaf := &Node{symbol: 1, parseState: 4}
	if _, ok := parser.reuseTargetState(0, leaf, lookahead); ok {
		t.Fatal("expected leaf reuse to reject a unique shift that does not match the stored parse state")
	}

	leaf.parseState = 0
	nextState, ok := parser.reuseTargetState(0, leaf, lookahead)
	if !ok {
		t.Fatal("expected unique-shift fallback for a leaf without stored parse-state metadata")
	}

	leaf.parseState = nextState
	if got, ok := parser.reuseTargetState(0, leaf, lookahead); !ok || got != nextState {
		t.Fatalf("expected matching stored parse state %d, got state=%d ok=%t", nextState, got, ok)
	}
}

func TestConfigureIncrementalParseCapsPreservesConfiguredWidth(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "18")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "16")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	parser := NewParser(buildArithmeticLanguage())
	var scratch parserScratch
	caps := parser.configureParseCaps([]byte("1+2"), &reuseCursor{}, arenaClassIncremental, &scratch, 0, 0, 0)
	if caps.maxStacks != 18 || caps.mergePerKeyCap != 16 {
		t.Fatalf("incremental caps = stacks:%d merge:%d, want stacks:18 merge:16", caps.maxStacks, caps.mergePerKeyCap)
	}
}

func TestReuseNonLeafTargetStateOnStackUsesPreGoto(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("1+2+3"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}

	var target *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil || target != nil {
			return
		}
		if n.ChildCount() > 0 {
			target = n
			return
		}
		for _, c := range n.Children() {
			walk(c)
		}
	}
	walk(root)
	if target == nil {
		t.Fatal("expected non-leaf candidate")
	}
	start := target.StartByte()
	pre := target.PreGotoState()

	stackWithPre := glrStack{
		entries: []stackEntry{
			{state: lang.InitialState},
			newStackEntryNode(pre+1, &Node{endByte: start}),
			newStackEntryNode(pre, &Node{endByte: start}),
		},
	}
	nextState, depth, ok := parser.reuseNonLeafTargetStateOnStack(&stackWithPre, target)
	if !ok {
		t.Fatal("expected non-leaf stack-context match success")
	}
	if depth != stackWithPre.depth() {
		t.Fatalf("reuse depth = %d, want live depth %d", depth, stackWithPre.depth())
	}
	if nextState == 0 {
		t.Fatal("expected non-zero goto state for matched pre-goto state")
	}

	stackMissingPre := glrStack{
		entries: []stackEntry{
			newStackEntryNode(pre+1, &Node{endByte: start}),
			newStackEntryNode(pre+2, &Node{endByte: start}),
		},
	}
	if _, _, ok := parser.reuseNonLeafTargetStateOnStack(&stackMissingPre, target); ok {
		t.Fatal("expected failure when live stack does not own candidate pre-goto state")
	}

	stackWithOlderPre := glrStack{
		entries: []stackEntry{
			{state: lang.InitialState},
			newStackEntryNode(pre, &Node{endByte: start}),
			newStackEntryNode(pre+1, &Node{endByte: start}),
		},
	}
	if _, _, ok := parser.reuseNonLeafTargetStateOnStack(&stackWithOlderPre, target); ok {
		t.Fatal("expected failure when only an older stack entry owns candidate pre-goto state")
	}
}
