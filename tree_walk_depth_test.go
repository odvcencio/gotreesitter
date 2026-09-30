package gotreesitter

import (
	"strings"
	"testing"
)

func boundedWalkLanguage() *Language {
	return &Language{SymbolNames: []string{"EOF", "wrapper", "leaf", "other"}, SymbolMetadata: []SymbolMetadata{{}, {Name: "wrapper", Visible: true, Named: true}, {Name: "leaf", Visible: true, Named: true}, {Name: "other", Visible: true, Named: true}}}
}

func boundedNamedChain(arena *nodeArena, depth int, symbol Symbol) (*Node, *Node) {
	leaf := newLeafNodeInArena(arena, symbol, true, 0, 1, Point{}, Point{Column: 1})
	root := leaf
	for i := 0; i < depth; i++ {
		root = newParentNodeInArenaNoLinksWithFieldSources(arena, 1, true, []*Node{root}, nil, nil, 0, true)
	}
	return root, leaf
}

func TestInputDepthTreeWalks(t *testing.T) {
	const depth = 20000
	lang := boundedWalkLanguage()
	arena := newNodeArena(arenaClassFull)
	defer arena.Release()
	root, leaf := boundedNamedChain(arena, depth, 2)
	want := strings.Repeat("(wrapper ", depth) + "(leaf)" + strings.Repeat(")", depth)
	if got := root.SExpr(lang); got != want {
		t.Fatalf("deep S-expression differs: %d bytes, want %d", len(got), len(want))
	}
	if got := root.descendantForByteRangeContained(0, 1, true); got != leaf {
		t.Fatal("contained-range walk lost the deepest named node")
	}
	edit := InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}}
	var hint *Node
	editNodeSingleByteReplacement(root, edit, &hint)
	if hint != leaf {
		t.Fatal("edit lost its leaf hint")
	}
	for n, count := root, 0; n != nil; count++ {
		if !n.HasChanges() || n.EndByte() != 1 {
			t.Fatalf("replacement lost node %d", count)
		}
		if len(n.children) == 0 {
			break
		}
		n = n.children[0]
	}
	otherArena := newNodeArena(arenaClassFull)
	defer otherArena.Release()
	other, _ := boundedNamedChain(otherArena, depth, 3)
	var ranges []Range
	diffNodes(root, other, &ranges)
	if len(ranges) != 1 || ranges[0].StartByte != 0 || ranges[0].EndByte != 1 {
		t.Fatalf("deep changed ranges = %+v", ranges)
	}
	edit = InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 2, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 2}}
	editNodeWithDelta(other, edit, 1, 0, true, nil, nil)
	for n, count := other, 0; n != nil; count++ {
		if !n.HasChanges() || n.EndByte() != 2 || n.EndPoint().Column != 2 {
			t.Fatalf("length-changing edit lost node %d", count)
		}
		if len(n.children) == 0 {
			break
		}
		n = n.children[0]
	}
}

func TestInputDepthAliasAndErrorDependency(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	defer arena.Release()
	a, _ := boundedNamedChain(arena, 20000, 2)
	b, _ := boundedNamedChain(arena, 20000, 3)
	lang := boundedWalkLanguage()
	lang.SymbolNames[3] = "leaf"
	if got := compareNodeAliasPreference(NewParser(lang), arena, a, b); got != 0 {
		t.Fatalf("deep equivalent aliases = %d", got)
	}
	markColumnDependentSubtreeChanged(a)
	for n := a; n != nil; {
		if !n.HasChanges() {
			t.Fatal("deep column invalidation lost a descendant")
		}
		n.setHasError(true)
		if len(n.children) == 0 {
			break
		}
		n = n.children[0]
	}
	if !nodeEndsBeforeEditDependency(a, 2) || nodeEndsBeforeEditDependency(a, 0) {
		t.Fatal("deep error dependency changed its boundary")
	}
	if !reconcileStaleHasErrorFlags(a, 0) || !a.hasError() {
		t.Fatal("error reconciliation cleared an unverified deep claim")
	}
}

func TestInputDepthPendingHiddenWalks(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	defer arena.Release()
	leaf := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	entry := newStackEntryNode(0, leaf)
	const depth = 20000
	for i := 0; i < depth; i++ {
		parent := newPendingParentInArena(arena, 1, false, 0, []stackEntry{entry}, 0, 1, Point{}, Point{Column: 1}, false)
		entry = newStackEntryPendingParent(0, parent)
	}
	meta := boundedWalkLanguage().SymbolMetadata
	meta[1].Visible = false
	if stackEntryTreeHasFieldIDs(entry, arena) || pendingPlainHiddenVisibleDescendantCount(entry, arena, meta, nil) != 1 {
		t.Fatal("hidden pending field/count walk changed its result")
	}
	count, payload, hasError, ok := pendingNoFieldChildCount(entry, arena, true, meta, nil)
	if count != 1 || !payload || hasError || !ok {
		t.Fatalf("pending count = %d %t %t %t", count, payload, hasError, ok)
	}
	first, firstOK := pendingNoFieldFirstChild(entry, arena, true, meta, nil)
	last, lastOK := pendingNoFieldLastChild(entry, arena, true, meta, nil)
	if !firstOK || !lastOK || stackEntryNode(first) != leaf || stackEntryNode(last) != leaf {
		t.Fatal("pending endpoints lost their leaf")
	}
	dst := make([]pendingChildEntry, 1)
	next, parents, refs := fillPendingNoFieldChildren(dst, 0, entry, arena, true, meta, nil)
	if next != 1 || parents != depth || refs != depth || stackEntryNode(dst[0].stackEntry()) != leaf {
		t.Fatalf("pending fill = %d %d %d", next, parents, refs)
	}
}

func TestInputDepthErrorRankPreservesForeignNodes(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	defer arena.Release()
	root, _ := boundedNamedChain(arena, 20000, errorSymbol)
	current := newNodeArena(arenaClassIncremental)
	defer current.Release()
	if got := cachedStackEntryErrorRank(newStackEntryNode(0, root), current); got != 2 {
		t.Fatalf("foreign error rank = %d", got)
	}
	for n := root; n != nil; {
		if n.errorRankCache != 0 {
			t.Fatal("foreign error-rank walk wrote an inline cache")
		}
		if len(n.children) == 0 {
			break
		}
		n = n.children[0]
	}
	if got := cachedStackEntryErrorRank(newStackEntryNode(0, root), arena); got != 2 {
		t.Fatalf("owned error rank = %d", got)
	}
	if root.errorRankCache != 3 {
		t.Fatal("owned root rank was not cached")
	}
}

func TestInputDepthPendingParentMaterialization(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	defer arena.Release()
	leaf := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	entry := newStackEntryNode(0, leaf)
	const depth = 20000
	for i := 0; i < depth; i++ {
		parent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{entry}, 0, 1, Point{}, Point{Column: 1}, false)
		parent.setChildFieldEntry(arena, 0, 7, fieldSourceDirect)
		parent.setHasFieldEntries(true)
		entry = newStackEntryPendingParent(0, parent)
	}
	node, updated := materializeStackEntryPendingParentEntryWithParser(nil, arena, entry, materializeForFinalTree)
	if node == nil || stackEntryNode(updated) != node {
		t.Fatal("materializer did not update the root payload")
	}
	for i := 0; i < depth; i++ {
		if len(node.children) != 1 || len(node.fieldIDs()) != 1 || node.fieldIDs()[0] != 7 || node.fieldSources()[0] != fieldSourceDirect || node.EndByte() != 1 {
			t.Fatalf("materialization lost node %d", i)
		}
		node = node.children[0]
	}
	if node != leaf {
		t.Fatal("materialization changed the leaf identity")
	}
}
