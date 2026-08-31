package gotreesitter

import (
	"bytes"
	"sync"
	"testing"
	"unsafe"
)

func testGLRStackPtr(s glrStack) *glrStack { return &s }

func TestStackEntrySizeBudget(t *testing.T) {
	if got := unsafe.Sizeof(stackEntry{}); got != 16 {
		t.Fatalf("stackEntry size = %d, want 16", got)
	}
}

func TestCRecoverStateSizeBudget(t *testing.T) {
	if got := unsafe.Sizeof(cRecoverState{}); got != 48 {
		t.Fatalf("cRecoverState size = %d, want 48", got)
	}
}

func TestGLRStackSizeBudget(t *testing.T) {
	if got := unsafe.Sizeof(glrStack{}); got != 104 {
		t.Fatalf("glrStack size = %d, want 104", got)
	}
}

func TestNoTreeNodeSizeBudget(t *testing.T) {
	// Ratchet: e70dd873 ("Add raw shape tracking, GLR recovery flags, and
	// refactor parser APIs") intentionally added rawShape (rawShapeRef,
	// 4 bytes) and dynamicPrecedence (int32, 4 bytes) to noTreeNode to
	// support precedence tracking and shape propagation used by the GLR
	// merge/equivalence logic (061b67f9). This budget was never bumped to
	// match; 32 is the correct current size, not a loosened check.
	if got := unsafe.Sizeof(noTreeNode{}); got != 32 {
		t.Fatalf("noTreeNode size = %d, want 32", got)
	}
}

func TestCompactFullLeafSizeBudget(t *testing.T) {
	// Ratchet: compactFullLeaf embeds noTreeNode, so it grew by the same
	// 8 bytes as TestNoTreeNodeSizeBudget (see comment there); 68 is the
	// correct current size.
	if got := unsafe.Sizeof(compactFullLeaf{}); got != 68 {
		t.Fatalf("compactFullLeaf size = %d, want 68", got)
	}
}

func TestPendingParentSizeBudget(t *testing.T) {
	if got := unsafe.Sizeof(pendingParent{}); got != 56 {
		t.Fatalf("pendingParent size = %d, want 56", got)
	}
}

func TestPendingChildEntrySizeBudget(t *testing.T) {
	if got := unsafe.Sizeof(pendingChildEntry{}); got != 16 {
		t.Fatalf("pendingChildEntry size = %d, want 16", got)
	}
	if stackEntryKindPendingParent > uint32(pendingChildEntryKindMask) {
		t.Fatalf("pendingChildEntry kind mask = %d does not cover pending-parent kind %d", pendingChildEntryKindMask, stackEntryKindPendingParent)
	}
}

func TestGLRStackClonePreservesRecoverability(t *testing.T) {
	original := glrStack{
		entries:             []stackEntry{{state: 1}, {state: 2}},
		cacheEntries:        true,
		byteOffset:          12,
		score:               7,
		recoverabilityKnown: true,
		mayRecover:          false,
		branchOrder:         3,
	}

	entryClone := original.clone()
	if !entryClone.recoverabilityKnown {
		t.Fatal("entry clone lost recoverabilityKnown")
	}
	if entryClone.mayRecover {
		t.Fatal("entry clone changed mayRecover")
	}

	var scratch gssScratch
	original.ensureGSS(&scratch)
	gssClone := original.clone()
	if !gssClone.recoverabilityKnown {
		t.Fatal("GSS clone lost recoverabilityKnown")
	}
	if gssClone.mayRecover {
		t.Fatal("GSS clone changed mayRecover")
	}

	scratchClone := original.cloneWithScratch(&scratch)
	if !scratchClone.recoverabilityKnown {
		t.Fatal("scratch clone lost recoverabilityKnown")
	}
	if scratchClone.mayRecover {
		t.Fatal("scratch clone changed mayRecover")
	}

	original.mayRecover = true
	recoveringClone := original.cloneWithScratch(&scratch)
	if !recoveringClone.recoverabilityKnown || !recoveringClone.mayRecover {
		t.Fatalf("recovering clone flags = known:%v may:%v, want true/true", recoveringClone.recoverabilityKnown, recoveringClone.mayRecover)
	}
}

func TestGLRStackCloneWithScratchPreservesRecoverabilityFromEntries(t *testing.T) {
	original := glrStack{
		entries:             []stackEntry{{state: 1}, {state: 2}},
		recoverabilityKnown: true,
		mayRecover:          false,
	}

	var scratch gssScratch
	gssClone := original.cloneWithScratch(&scratch)
	if !gssClone.recoverabilityKnown {
		t.Fatal("cloneWithScratch lost recoverabilityKnown")
	}
	if gssClone.mayRecover {
		t.Fatal("cloneWithScratch changed mayRecover")
	}

	original.mayRecover = true
	recoveringClone := original.cloneWithScratch(&scratch)
	if !recoveringClone.recoverabilityKnown || !recoveringClone.mayRecover {
		t.Fatalf("recovering clone flags = known:%v may:%v, want true/true", recoveringClone.recoverabilityKnown, recoveringClone.mayRecover)
	}
}

func TestPendingChildEntryRoundTripsKindAndState(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	node := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	node.parseState = 11
	noTree := newNoTreeLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	noTree.parseState = 12
	leaf := newCompactFullLeafInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	leaf.parseState = 13
	parent := newPendingParentInArena(arena, 4, true, 5, nil, 3, 4, Point{Column: 3}, Point{Column: 4}, false)
	parent.parseState = 14

	for _, tc := range []struct {
		name  string
		entry stackEntry
	}{
		{name: "node", entry: newStackEntryNode(node.parseState, node)},
		{name: "no-tree", entry: newStackEntryNoTreeNode(noTree.parseState, noTree)},
		{name: "leaf", entry: newStackEntryCompactFullLeaf(leaf.parseState, leaf)},
		{name: "parent", entry: newStackEntryPendingParent(parent.parseState, parent)},
	} {
		got := newPendingChildEntry(tc.entry).stackEntry()
		if got.node != tc.entry.node {
			t.Fatalf("%s node = %p, want %p", tc.name, got.node, tc.entry.node)
		}
		if got.kind != tc.entry.kind {
			t.Fatalf("%s kind = %d, want %d", tc.name, got.kind, tc.entry.kind)
		}
		if got.state != tc.entry.state {
			t.Fatalf("%s state = %d, want %d", tc.name, got.state, tc.entry.state)
		}
	}
}

func TestPendingParentMaterializationCachesMaterializedChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	leaf := newCompactFullLeafInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	leaf.parseState = 13
	parent := newPendingParentInArena(arena, 4, true, 5, []stackEntry{newStackEntryCompactFullLeaf(leaf.parseState, leaf)}, 2, 3, Point{Column: 2}, Point{Column: 3}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	if node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree); node == nil {
		t.Fatal("first materialized node = nil")
	}
	if got := arena.compactFullLeafMaterialized; got != 1 {
		t.Fatalf("compact leaf materialized after first pass = %d, want 1", got)
	}

	entry = newStackEntryPendingParent(parent.parseState, parent)
	if node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree); node == nil {
		t.Fatal("second materialized node = nil")
	}
	if got := arena.compactFullLeafMaterialized; got != 1 {
		t.Fatalf("compact leaf materialized after second pass = %d, want 1", got)
	}
	if child := parent.childEntry(arena, 0); stackEntryNode(child) == nil {
		t.Fatal("pending child ref was not updated to materialized node")
	}
}

func TestPendingParentFinalChildRefsKeepCompactLeafLazy(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leaf := newCompactFullLeafInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	leaf.parseState = 13
	parent := newPendingParentInArena(arena, 4, true, 5, []stackEntry{newStackEntryCompactFullLeaf(leaf.parseState, leaf)}, 2, 3, Point{Column: 2}, Point{Column: 3}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if node == nil {
		t.Fatal("materialized node = nil")
	}
	if got := arena.compactFullLeafMaterialized; got != 0 {
		t.Fatalf("compact leaf materialized during final parent build = %d, want 0", got)
	}
	if got := len(node.children); got != 0 {
		t.Fatalf("len(node.children) = %d, want lazy sidecar", got)
	}
	if got := node.ChildCount(); got != 1 {
		t.Fatalf("ChildCount = %d, want 1 from sidecar", got)
	}
	if got := arena.finalChildRefsCreated; got != 1 {
		t.Fatalf("finalChildRefsCreated = %d, want 1", got)
	}

	child := node.Child(0)
	if child == nil {
		t.Fatal("Child(0) = nil")
	}
	if got := child.Symbol(); got != 3 {
		t.Fatalf("child symbol = %d, want 3", got)
	}
	if got := arena.compactFullLeafMaterializedForParentAPI; got != 1 {
		t.Fatalf("compact leaf materialized for parent API = %d, want 1", got)
	}
	if got := len(node.children); got != 0 {
		t.Fatalf("len(node.children) after Child(0) = %d, want lazy sidecar", got)
	}
	if got := arena.finalChildRefsMaterializedChildren; got != 0 {
		t.Fatalf("final child ref range materialized children = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children materialized = %d, want 1", got)
	}
}

func TestPendingParentFinalChildRefsSurviveParentLinkWiring(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leaf := newCompactFullLeafInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	leaf.parseState = 13
	inner := newPendingParentInArena(arena, 4, true, 5, []stackEntry{newStackEntryCompactFullLeaf(leaf.parseState, leaf)}, 2, 3, Point{Column: 2}, Point{Column: 3}, false)
	inner.parseState = 14
	outer := newPendingParentInArena(arena, 5, true, 6, []stackEntry{newStackEntryPendingParent(inner.parseState, inner)}, 2, 3, Point{Column: 2}, Point{Column: 3}, false)
	outer.parseState = 15

	entry := newStackEntryPendingParent(outer.parseState, outer)
	outerNode := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if outerNode == nil {
		t.Fatal("outer node = nil")
	}
	rootChildren := arena.allocNodeSlice(1)
	rootChildren[0] = outerNode
	root := newParentNodeInArenaNoLinksWithFieldSources(arena, 1, true, rootChildren, nil, nil, 0, false)
	arena.deferParentLinks(root)
	outerNode = root.Child(0)
	if outerNode == nil {
		t.Fatal("root.Child(0) = nil")
	}
	innerNode := outerNode.Child(0)
	if innerNode == nil {
		t.Fatal("outer.Child(0) = nil")
	}
	if got := innerNode.ChildCount(); got != 1 {
		t.Fatalf("inner ChildCount before link wiring = %d, want 1", got)
	}
	if parent := innerNode.Parent(); parent != outerNode {
		t.Fatalf("inner Parent = %p, want outer %p", parent, outerNode)
	}
	leafNode := innerNode.Child(0)
	if leafNode == nil {
		t.Fatal("inner.Child(0) = nil")
	}
	if parent := leafNode.Parent(); parent != innerNode {
		t.Fatalf("leaf Parent = %p, want inner %p", parent, innerNode)
	}
}

func TestQueryMatcherUsesLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leaf := newCompactFullLeafInArena(arena, 1, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	leaf.parseState = 13
	parent := newPendingParentInArena(arena, 2, true, 5, []stackEntry{newStackEntryCompactFullLeaf(leaf.parseState, leaf)}, 2, 3, Point{Column: 2}, Point{Column: 3}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if node == nil {
		t.Fatal("materialized parent = nil")
	}
	lang := &Language{
		SymbolNames: []string{"", "leaf", "parent"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Named: true, Visible: true},
			{Named: true, Visible: true},
		},
	}
	query := &Query{}
	steps := []QueryStep{
		{symbol: 2, isNamed: true, depth: 0, quantifier: queryQuantifierOne},
		{symbol: 1, isNamed: true, depth: 1, quantifier: queryQuantifierOne},
	}
	var captures []QueryCapture
	if !query.matchStepsWithPredicates(steps, 0, node, lang, nil, nil, &captures) {
		t.Fatal("query did not match compact final child")
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children materialized during deferred Node.Edit = %d, want 0", got)
	}
}

func TestQueryCursorDefersFinalChildRefsUntilCurrentNodeExhausted(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leaf := newCompactFullLeafInArena(arena, 1, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	leaf.parseState = 13
	parent := newPendingParentInArena(arena, 2, true, 5, []stackEntry{newStackEntryCompactFullLeaf(leaf.parseState, leaf)}, 2, 3, Point{Column: 2}, Point{Column: 3}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if node == nil {
		t.Fatal("materialized parent = nil")
	}
	lang := &Language{
		SymbolNames: []string{"", "leaf", "parent"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Named: true, Visible: true},
			{Named: true, Visible: true},
		},
	}
	query, err := NewQuery(`(parent) @p`, lang)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}
	cursor := query.Exec(node, lang, nil)
	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("NextMatch returned !ok")
	}
	if len(match.Captures) != 1 || match.Captures[0].Node != node {
		t.Fatalf("captures = %#v, want parent node capture", match.Captures)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children materialized = %d, want 0", got)
	}
}

func TestQueryCursorByteRangeSkipsOutOfRangeLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	left := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	left.parseState = 11
	right := newCompactFullLeafInArena(arena, 1, true, 10, 11, Point{Column: 10}, Point{Column: 11})
	right.parseState = 12
	parent := newPendingParentInArena(arena, 2, true, 5, []stackEntry{
		newStackEntryCompactFullLeaf(left.parseState, left),
		newStackEntryCompactFullLeaf(right.parseState, right),
	}, 0, 11, Point{}, Point{Column: 11}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if node == nil {
		t.Fatal("materialized parent = nil")
	}
	lang := &Language{
		SymbolNames: []string{"", "leaf", "parent"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Named: true, Visible: true},
			{Named: true, Visible: true},
		},
	}
	query, err := NewQuery(`(leaf) @l`, lang)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}
	cursor := query.Exec(node, lang, []byte("a         b"))
	cursor.SetByteRange(0, 2)
	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("NextMatch returned !ok")
	}
	if len(match.Captures) != 1 || match.Captures[0].Node.StartByte() != 0 {
		t.Fatalf("captures = %#v, want left leaf only", match.Captures)
	}
	if _, ok := cursor.NextMatch(); ok {
		t.Fatal("unexpected second match")
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children materialized = %d, want 1", got)
	}
}

func TestQueryMatcherSkipsNonCandidateLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	other := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	other.parseState = 11
	wanted := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	wanted.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(other.parseState, other),
		newStackEntryCompactFullLeaf(wanted.parseState, wanted),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if node == nil {
		t.Fatal("materialized parent = nil")
	}
	lang := &Language{
		SymbolNames: []string{"", "other", "wanted", "parent"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Named: true, Visible: true},
			{Named: true, Visible: true},
			{Named: true, Visible: true},
		},
	}
	query, err := NewQuery(`(parent (wanted) @w)`, lang)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}
	cursor := query.Exec(node, lang, []byte("ow"))
	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("NextMatch returned !ok")
	}
	if len(match.Captures) != 1 || match.Captures[0].Node.Symbol() != 2 {
		t.Fatalf("captures = %#v, want wanted child capture", match.Captures)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children materialized = %d, want 1", got)
	}
}

func TestAlternativeFieldMatchesUsesLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	other := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	other.parseState = 13
	leaf := newCompactFullLeafInArena(arena, 1, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	leaf.parseState = 14
	parent := newPendingParentInArena(arena, 2, true, 5, []stackEntry{
		newStackEntryCompactFullLeaf(other.parseState, other),
		newStackEntryCompactFullLeaf(leaf.parseState, leaf),
	}, 0, 3, Point{}, Point{Column: 3}, false)
	parent.parseState = 15

	entry := newStackEntryPendingParent(parent.parseState, parent)
	node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if node == nil {
		t.Fatal("materialized parent = nil")
	}
	node.setFieldIDs([]FieldID{0, 1})
	child := nodeChildAtForReason(node, 1, materializeForQuery)
	if child == nil {
		t.Fatal("nodeChildAtForReason = nil")
	}
	lang := &Language{FieldNames: []string{"", "name"}}
	query := &Query{}
	if !query.alternativeFieldMatches(&alternativeSymbol{field: 1}, child, nil, -1, lang) {
		t.Fatal("alternativeFieldMatches = false, want true")
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children materialized = %d, want 1", got)
	}
}

func TestLazyFinalChildRefsParentAndSiblingAvoidRangeMaterialization(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftLeaf.parseState = 11
	innerLeaf := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	innerLeaf.parseState = 12
	inner := newPendingParentInArena(arena, 3, true, 6, []stackEntry{newStackEntryCompactFullLeaf(innerLeaf.parseState, innerLeaf)}, 1, 2, Point{Column: 1}, Point{Column: 2}, false)
	inner.parseState = 13
	outer := newPendingParentInArena(arena, 4, true, 7, []stackEntry{
		newStackEntryCompactFullLeaf(leftLeaf.parseState, leftLeaf),
		newStackEntryPendingParent(inner.parseState, inner),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	outer.parseState = 14

	entry := newStackEntryPendingParent(outer.parseState, outer)
	outerNode := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if outerNode == nil {
		t.Fatal("outer node = nil")
	}
	leftNode := outerNode.Child(0)
	if leftNode == nil {
		t.Fatal("outer.Child(0) = nil")
	}
	innerNode := leftNode.NextSibling()
	if innerNode == nil {
		t.Fatal("left.NextSibling() = nil")
	}
	if innerNode.Symbol() != 3 {
		t.Fatalf("left.NextSibling symbol = %d, want 3", innerNode.Symbol())
	}
	if parent := innerNode.Parent(); parent != outerNode {
		t.Fatalf("inner Parent = %p, want outer %p", parent, outerNode)
	}
	if prev := innerNode.PrevSibling(); prev != leftNode {
		t.Fatalf("inner PrevSibling = %p, want left %p", prev, leftNode)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 2 {
		t.Fatalf("final child ref single children materialized = %d, want 2", got)
	}
}

func TestNamedChildAPIsSkipAnonymousLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	anon := newCompactFullLeafInArena(arena, 1, false, 0, 1, Point{}, Point{Column: 1})
	anon.parseState = 11
	named := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	named.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(anon.parseState, anon),
		newStackEntryCompactFullLeaf(named.parseState, named),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	if got := root.NamedChildCount(); got != 1 {
		t.Fatalf("NamedChildCount = %d, want 1", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children after NamedChildCount = %d, want 0", got)
	}
	child := root.NamedChild(0)
	if child == nil {
		t.Fatal("NamedChild(0) = nil")
	}
	if got := child.Symbol(); got != 2 {
		t.Fatalf("NamedChild(0).Symbol = %d, want 2", got)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children after NamedChild = %d, want 1", got)
	}
}

func TestSExprSkipsAnonymousLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	anon := newCompactFullLeafInArena(arena, 1, false, 0, 1, Point{}, Point{Column: 1})
	anon.parseState = 11
	named := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	named.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(anon.parseState, anon),
		newStackEntryCompactFullLeaf(named.parseState, named),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	lang := &Language{
		SymbolNames: []string{"", "anon", "name", "parent"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Named: false, Visible: true},
			{Named: true, Visible: true},
			{Named: true, Visible: true},
		},
	}
	if got, want := root.SExpr(lang), "(parent (name))"; got != want {
		t.Fatalf("SExpr = %q, want %q", got, want)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children after SExpr = %d, want 1", got)
	}
}

func TestTreeCursorNamedNavigationSkipsAnonymousLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	anonLeft := newCompactFullLeafInArena(arena, 1, false, 0, 1, Point{}, Point{Column: 1})
	anonLeft.parseState = 11
	namedLeft := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	namedLeft.parseState = 12
	anonMiddle := newCompactFullLeafInArena(arena, 1, false, 2, 3, Point{Column: 2}, Point{Column: 3})
	anonMiddle.parseState = 13
	namedRight := newCompactFullLeafInArena(arena, 2, true, 3, 4, Point{Column: 3}, Point{Column: 4})
	namedRight.parseState = 14
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(anonLeft.parseState, anonLeft),
		newStackEntryCompactFullLeaf(namedLeft.parseState, namedLeft),
		newStackEntryCompactFullLeaf(anonMiddle.parseState, anonMiddle),
		newStackEntryCompactFullLeaf(namedRight.parseState, namedRight),
	}, 0, 4, Point{}, Point{Column: 4}, false)
	parent.parseState = 15

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	cursor := NewTreeCursor(root, nil)
	if !cursor.GotoFirstNamedChild() {
		t.Fatal("GotoFirstNamedChild = false")
	}
	if got := cursor.CurrentNode().StartByte(); got != 1 {
		t.Fatalf("first named StartByte = %d, want 1", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children after first named = %d, want 1", got)
	}
	if !cursor.GotoNextNamedSibling() {
		t.Fatal("GotoNextNamedSibling = false")
	}
	if got := cursor.CurrentNode().StartByte(); got != 3 {
		t.Fatalf("next named StartByte = %d, want 3", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 2 {
		t.Fatalf("final child ref single children after next named = %d, want 2", got)
	}
	if !cursor.GotoPrevNamedSibling() {
		t.Fatal("GotoPrevNamedSibling = false")
	}
	if got := cursor.CurrentNode().StartByte(); got != 1 {
		t.Fatalf("previous named StartByte = %d, want 1", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 2 {
		t.Fatalf("final child ref single children after prev named = %d, want 2", got)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
}

func TestTreeCursorRangeSearchUsesLazyFinalChildHeaders(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	children := make([]stackEntry, 4)
	for i := range children {
		leaf := newCompactFullLeafInArena(arena, 1, true, uint32(i), uint32(i+1), Point{Column: uint32(i)}, Point{Column: uint32(i + 1)})
		leaf.parseState = StateID(10 + i)
		children[i] = newStackEntryCompactFullLeaf(leaf.parseState, leaf)
	}
	parent := newPendingParentInArena(arena, 2, true, 4, children, 0, 4, Point{}, Point{Column: 4}, false)
	parent.parseState = 15

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	cursor := NewTreeCursor(root, nil)
	if got := cursor.GotoFirstChildForByte(2); got != 2 {
		t.Fatalf("GotoFirstChildForByte = %d, want 2", got)
	}
	if got := cursor.CurrentNode().StartByte(); got != 2 {
		t.Fatalf("byte cursor StartByte = %d, want 2", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children after byte search = %d, want 1", got)
	}
	cursor.Reset(root)
	if got := cursor.GotoFirstChildForPoint(Point{Column: 2}); got != 2 {
		t.Fatalf("GotoFirstChildForPoint = %d, want 2", got)
	}
	if got := cursor.CurrentNode().StartByte(); got != 2 {
		t.Fatalf("point cursor StartByte = %d, want 2", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children after point search = %d, want 1", got)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
}

func TestDescendantRangeUsesLazyFinalChildHeaders(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	children := make([]stackEntry, 3)
	for i := range children {
		leaf := newCompactFullLeafInArena(arena, 1, true, uint32(i), uint32(i+1), Point{Column: uint32(i)}, Point{Column: uint32(i + 1)})
		leaf.parseState = StateID(10 + i)
		children[i] = newStackEntryCompactFullLeaf(leaf.parseState, leaf)
	}
	parent := newPendingParentInArena(arena, 2, true, 4, children, 0, 3, Point{}, Point{Column: 3}, false)
	parent.parseState = 15

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	if got := root.DescendantForByteRange(1, 2); got == nil || got.StartByte() != 1 {
		t.Fatalf("DescendantForByteRange = %#v, want middle child", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children after byte descendant = %d, want 1", got)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
}

func TestDescendantPointRangeUsesLazyFinalChildHeaders(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	children := make([]stackEntry, 3)
	for i := range children {
		leaf := newCompactFullLeafInArena(arena, 1, true, uint32(i), uint32(i+1), Point{Column: uint32(i)}, Point{Column: uint32(i + 1)})
		leaf.parseState = StateID(10 + i)
		children[i] = newStackEntryCompactFullLeaf(leaf.parseState, leaf)
	}
	parent := newPendingParentInArena(arena, 2, true, 4, children, 0, 3, Point{}, Point{Column: 3}, false)
	parent.parseState = 15

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	if got := root.DescendantForPointRange(Point{Column: 1}, Point{Column: 2}); got == nil || got.StartByte() != 1 {
		t.Fatalf("DescendantForPointRange = %#v, want middle child", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children after point descendant = %d, want 1", got)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
}

func TestReuseCursorWalksLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	left := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	left.parseState = 11
	right := newCompactFullLeafInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	right.parseState = 12
	parent := newPendingParentInArena(arena, 2, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(left.parseState, left),
		newStackEntryCompactFullLeaf(right.parseState, right),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	oldTree := &Tree{root: root, source: []byte("ab"), arena: arena}

	var scratch reuseScratch
	reuse := (&reuseCursor{}).reset(oldTree, []byte("abc"), &scratch)
	if reuse == nil {
		t.Fatal("reuse cursor reset returned nil")
	}
	candidates := reuse.candidates(1)
	if len(candidates) != 1 || candidates[0].StartByte() != 1 {
		t.Fatalf("candidates = %#v, want right child at byte 1", candidates)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children = %d, want only matching candidate materialized", got)
	}
}

func TestReuseCursorTopLevelUsesLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	left := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	left.parseState = 11
	mid := newCompactFullLeafInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	mid.parseState = 12
	right := newCompactFullLeafInArena(arena, 1, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	right.parseState = 13
	parent := newPendingParentInArena(arena, 2, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(left.parseState, left),
		newStackEntryCompactFullLeaf(mid.parseState, mid),
		newStackEntryCompactFullLeaf(right.parseState, right),
	}, 0, 3, Point{}, Point{Column: 3}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	oldTree := &Tree{
		root:   root,
		source: []byte("abc"),
		arena:  arena,
		edits: []InputEdit{{
			StartByte: 0,
		}},
	}

	var scratch reuseScratch
	reuse := (&reuseCursor{}).reset(oldTree, []byte("abc"), &scratch)
	if reuse == nil {
		t.Fatal("reuse cursor reset returned nil")
	}
	candidates := reuse.candidates(2)
	if len(candidates) != 1 || candidates[0].StartByte() != 2 {
		t.Fatalf("top-level candidates = %#v, want right child at byte 2", candidates)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children = %d, want only matching top-level candidate materialized", got)
	}
}

func TestReuseCursorTopLevelRejectsChangedFinalRefWithoutMaterialization(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	left := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	left.parseState = 11
	mid := newCompactFullLeafInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	mid.parseState = 12
	right := newCompactFullLeafInArena(arena, 1, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	right.parseState = 13
	parent := newPendingParentInArena(arena, 2, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(left.parseState, left),
		newStackEntryCompactFullLeaf(mid.parseState, mid),
		newStackEntryCompactFullLeaf(right.parseState, right),
	}, 0, 3, Point{}, Point{Column: 3}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	oldTree := &Tree{
		root:   root,
		source: []byte("abc"),
		arena:  arena,
		edits: []InputEdit{{
			StartByte: 0,
		}},
	}

	var scratch reuseScratch
	reuse := (&reuseCursor{}).reset(oldTree, []byte("axc"), &scratch)
	if reuse == nil {
		t.Fatal("reuse cursor reset returned nil")
	}
	candidates := reuse.candidates(1)
	if len(candidates) != 0 {
		t.Fatalf("top-level candidates = %#v, want none for changed bytes", candidates)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children = %d, want changed candidate rejected without materialization", got)
	}
}

func TestTreeEditKeepsUnaffectedFinalChildRefsLazy(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftLeaf.parseState = 11
	rightLeaf := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightLeaf.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(leftLeaf.parseState, leftLeaf),
		newStackEntryCompactFullLeaf(rightLeaf.parseState, rightLeaf),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	tree := &Tree{root: root, arena: arena}
	tree.Edit(InputEdit{
		StartByte:   1,
		OldEndByte:  1,
		NewEndByte:  2,
		StartPoint:  Point{Column: 1},
		OldEndPoint: Point{Column: 1},
		NewEndPoint: Point{Column: 2},
	})
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children materialized during edit = %d, want 0", got)
	}
	if root.EndByte() != 3 {
		t.Fatalf("root EndByte = %d, want 3", root.EndByte())
	}
	if left := root.Child(0); left == nil || left.EndByte() != 1 {
		t.Fatalf("left child after edit = %#v, want unchanged end 1", left)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children after left access = %d, want 1", got)
	}
	if right := root.Child(1); right == nil || right.StartByte() != 2 || right.EndByte() != 3 {
		t.Fatalf("right child after edit = %#v, want shifted range [2,3)", right)
	}
}

func TestResultMutableChildViewFilterFinalRefsCopyOnWrite(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	left := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	left.parseState = 11
	right := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	right.parseState = 12
	dropped := newCompactFullLeafInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	dropped.parseState = 13
	parent := newPendingParentInArena(arena, 4, true, 5, []stackEntry{
		newStackEntryCompactFullLeaf(left.parseState, left),
		newStackEntryCompactFullLeaf(right.parseState, right),
		newStackEntryCompactFullLeaf(dropped.parseState, dropped),
	}, 0, 3, Point{}, Point{Column: 3}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	root.setFieldMetadata(
		[]FieldID{1, 2, 3},
		[]uint8{fieldSourceDirect, fieldSourceInherited, fieldSourceDirect},
	)
	oldRange, ok := arena.finalChildRange(root)
	if !ok {
		t.Fatal("root did not start with final-child refs")
	}
	oldRefs := oldRange.refs(arena)

	view := resultMutableChildrenForMutation(root)
	if !view.FilterFinalRefs(func(_ int, entry stackEntry) bool {
		return stackEntryNodeSymbol(entry) != right.symbol
	}) {
		t.Fatal("FilterFinalRefs returned false")
	}
	newRange, ok := arena.finalChildRange(root)
	if !ok {
		t.Fatal("root lost final-child refs")
	}
	if newRange == oldRange {
		t.Fatal("FilterFinalRefs reused original child-ref range")
	}
	if got := root.ChildCount(); got != 2 {
		t.Fatalf("root child count = %d, want 2", got)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children during filter = %d, want 0", got)
	}
	if got, want := root.fieldIDs(), []FieldID{1, 3}; !equalFieldIDSlices(got, want) {
		t.Fatalf("root field IDs = %v, want %v", got, want)
	}
	if got, want := root.fieldSources(), []uint8{fieldSourceDirect, fieldSourceDirect}; !bytes.Equal(got, want) {
		t.Fatalf("root field sources = %v, want %v", got, want)
	}

	other := newCompactFullLeafInArena(arena, 9, true, 9, 10, Point{Column: 9}, Point{Column: 10})
	other.parseState = 99
	oldRefs[0] = newPendingChildEntry(newStackEntryCompactFullLeaf(other.parseState, other))
	first := root.Child(0)
	if first == nil || first.Symbol() != left.symbol || first.StartByte() != 0 {
		t.Fatalf("first child after mutating old range = %#v, want original left child", first)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children after first access = %d, want 1", got)
	}
}

func TestReplaceChildRangeWithSingleNodeKeepsFinalRefsLazy(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	left := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	left.parseState = 11
	mid := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	mid.parseState = 12
	right := newCompactFullLeafInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	right.parseState = 13
	parent := newPendingParentInArena(arena, 4, true, 5, []stackEntry{
		newStackEntryCompactFullLeaf(left.parseState, left),
		newStackEntryCompactFullLeaf(mid.parseState, mid),
		newStackEntryCompactFullLeaf(right.parseState, right),
	}, 0, 3, Point{}, Point{Column: 3}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	replacement := newLeafNodeInArena(arena, 9, true, 1, 3, Point{Column: 1}, Point{Column: 3})
	replaceChildRangeWithSingleNode(root, 3, 2, replacement)
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents after invalid replace = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children after invalid replace = %d, want 0", got)
	}

	replaceChildRangeWithSingleNode(root, 1, 3, replacement)

	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children during replace = %d, want 0", got)
	}
	if !nodeHasFinalChildRefs(root) {
		t.Fatal("replace dropped final-child refs")
	}
	if got := root.ChildCount(); got != 2 {
		t.Fatalf("root child count = %d, want 2", got)
	}
	if got := root.Child(0); got == nil || got.Symbol() != left.symbol {
		t.Fatalf("root child 0 = %#v, want left", got)
	}
	if got := root.Child(1); got != replacement {
		t.Fatalf("root child 1 = %p, want replacement %p", got, replacement)
	}
}

func TestFilterHiddenExtraTriviaRootFiltersFinalRefsWithoutDrain(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	left := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	left.parseState = 11
	trivia := newCompactFullLeafInArena(arena, 2, false, 1, 2, Point{Column: 1}, Point{Column: 2})
	trivia.setExtra(true)
	trivia.parseState = 12
	right := newCompactFullLeafInArena(arena, 1, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	right.parseState = 13
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(left.parseState, left),
		newStackEntryCompactFullLeaf(trivia.parseState, trivia),
		newStackEntryCompactFullLeaf(right.parseState, right),
	}, 0, 3, Point{}, Point{Column: 3}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	root.setFieldMetadata(
		[]FieldID{1, 0, 2},
		[]uint8{fieldSourceDirect, fieldSourceNone, fieldSourceInherited},
	)
	filterHiddenExtraTriviaRoot(root, []byte("a b"), nil)
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children during filter = %d, want 0", got)
	}
	if !nodeHasFinalChildRefs(root) {
		t.Fatal("filter dropped final-child refs")
	}
	if got := root.ChildCount(); got != 2 {
		t.Fatalf("root child count after filter = %d, want 2", got)
	}
	if child := root.Child(0); child == nil || child.Symbol() != left.symbol ||
		child.StartByte() != 0 || child.EndByte() != 1 {
		t.Fatalf("root child 0 = %#v, want left leaf", child)
	}
	if child := root.Child(1); child == nil || child.Symbol() != right.symbol ||
		child.StartByte() != 2 || child.EndByte() != 3 {
		t.Fatalf("root child 1 = %#v, want right leaf", child)
	}
	if got, want := root.fieldIDs(), []FieldID{1, 2}; !equalFieldIDSlices(got, want) {
		t.Fatalf("root field IDs = %v, want %v", got, want)
	}
	if got, want := root.fieldSources(), []uint8{fieldSourceDirect, fieldSourceInherited}; !bytes.Equal(got, want) {
		t.Fatalf("root field sources = %v, want %v", got, want)
	}
	if got, want := root.EndByte(), uint32(3); got != want {
		t.Fatalf("root end byte = %d, want %d", got, want)
	}
}

func TestNodeEditNoopKeepsLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leaf.parseState = 11
	parent := newPendingParentInArena(arena, 2, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(leaf.parseState, leaf),
	}, 0, 1, Point{}, Point{Column: 1}, false)
	parent.parseState = 12

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	root.Edit(InputEdit{
		StartByte:   1,
		OldEndByte:  1,
		NewEndByte:  1,
		StartPoint:  Point{Column: 1},
		OldEndPoint: Point{Column: 1},
		NewEndPoint: Point{Column: 1},
	})
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children after noop Node.Edit = %d, want 0", got)
	}
}

func TestNodeEditKeepsFinalChildRefsRangeLazy(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftLeaf.parseState = 11
	rightLeaf := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightLeaf.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(leftLeaf.parseState, leftLeaf),
		newStackEntryCompactFullLeaf(rightLeaf.parseState, rightLeaf),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	root.Edit(InputEdit{
		StartByte:   1,
		OldEndByte:  1,
		NewEndByte:  2,
		StartPoint:  Point{Column: 1},
		OldEndPoint: Point{Column: 1},
		NewEndPoint: Point{Column: 2},
	})
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children materialized during Node.Edit = %d, want 0", got)
	}
	if got := root.EndByte(); got != 3 {
		t.Fatalf("root EndByte = %d, want 3", got)
	}
	if right := root.Child(1); right == nil || right.StartByte() != 2 || right.EndByte() != 3 {
		t.Fatalf("right child after Node.Edit = %#v, want shifted range [2,3)", right)
	}
}

func TestNodeEditMarksCompactFinalRefDirtyWithoutMaterialization(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftLeaf.parseState = 11
	rightLeaf := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightLeaf.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(leftLeaf.parseState, leftLeaf),
		newStackEntryCompactFullLeaf(rightLeaf.parseState, rightLeaf),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	root.Edit(InputEdit{
		StartByte:   0,
		OldEndByte:  1,
		NewEndByte:  1,
		StartPoint:  Point{},
		OldEndPoint: Point{Column: 1},
		NewEndPoint: Point{Column: 1},
	})
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children materialized during Node.Edit = %d, want 0", got)
	}
	if !root.HasChanges() {
		t.Fatal("root HasChanges = false, want true")
	}
	left := root.Child(0)
	if left == nil {
		t.Fatal("left child after edit = nil")
	}
	if !left.HasChanges() {
		t.Fatal("left child HasChanges = false, want compact dirty flag to materialize")
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("final child ref single children after left access = %d, want 1", got)
	}
}

func TestNodeEditDeferredRootKeepsFinalChildRefsRangeLazy(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftLeaf.parseState = 11
	rightLeaf := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightLeaf.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(leftLeaf.parseState, leftLeaf),
		newStackEntryCompactFullLeaf(rightLeaf.parseState, rightLeaf),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	arena.deferParentLinks(root)

	root.Edit(InputEdit{
		StartByte:   1,
		OldEndByte:  2,
		NewEndByte:  3,
		StartPoint:  Point{Column: 1},
		OldEndPoint: Point{Column: 2},
		NewEndPoint: Point{Column: 3},
	})
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children materialized during deferred Node.Edit = %d, want 0", got)
	}
	if got := root.EndByte(); got != 3 {
		t.Fatalf("root EndByte = %d, want 3", got)
	}
}

func TestNodeEditFromDeferredSubnodeWiresOnlyTargetPath(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	children := arena.allocNodeSliceNoClear(2)
	children[0] = left
	children[1] = right
	root := newParentNodeInArenaNoLinksWithFieldSources(arena, 3, true, children, nil, nil, 0, false)
	arena.deferParentLinks(root)

	if left.parent != nil || right.parent != nil {
		t.Fatal("expected children to start unwired")
	}
	right.Edit(InputEdit{
		StartByte:   1,
		OldEndByte:  2,
		NewEndByte:  3,
		StartPoint:  Point{Column: 1},
		OldEndPoint: Point{Column: 2},
		NewEndPoint: Point{Column: 3},
	})
	if got := right.parent; got != root {
		t.Fatalf("right parent = %p, want root %p", got, root)
	}
	if left.parent != nil {
		t.Fatal("left parent should remain unwired")
	}
	if got := root.EndByte(); got != 3 {
		t.Fatalf("root EndByte = %d, want 3", got)
	}
}

func TestSiblingFallbackScanKeepsUnreturnedFinalChildRefsLazy(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftLeaf.parseState = 11
	middleLeaf := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	middleLeaf.parseState = 12
	rightLeaf := newCompactFullLeafInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	rightLeaf.parseState = 13
	parent := newPendingParentInArena(arena, 4, true, 5, []stackEntry{
		newStackEntryCompactFullLeaf(leftLeaf.parseState, leftLeaf),
		newStackEntryCompactFullLeaf(middleLeaf.parseState, middleLeaf),
		newStackEntryCompactFullLeaf(rightLeaf.parseState, rightLeaf),
	}, 0, 3, Point{}, Point{Column: 3}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	middle := root.Child(1)
	if middle == nil {
		t.Fatal("middle child nil")
	}
	middle.childIndex = -1

	next := middle.NextSibling()
	if next == nil || next.Symbol() != 3 {
		t.Fatalf("next sibling = %#v, want symbol 3", next)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 2 {
		t.Fatalf("single children after NextSibling = %d, want middle+right only", got)
	}
	prev := middle.PrevSibling()
	if prev == nil || prev.Symbol() != 1 {
		t.Fatalf("prev sibling = %#v, want symbol 1", prev)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents after PrevSibling = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 3 {
		t.Fatalf("single children after PrevSibling = %d, want all three", got)
	}
}

func TestCloneTreeNodesIntoArenaKeepsFinalChildRefsRangeLazy(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftLeaf.parseState = 11
	rightLeaf := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightLeaf.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(leftLeaf.parseState, leftLeaf),
		newStackEntryCompactFullLeaf(rightLeaf.parseState, rightLeaf),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}

	cloneArena := newNodeArena(arenaClassFull)
	clone := cloneTreeNodesIntoArena(root, cloneArena)
	if clone == nil {
		t.Fatal("clone = nil")
	}
	if clone == root {
		t.Fatal("clone should be a distinct node")
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("source final child ref single children materialized during clone = %d, want 0", got)
	}
	if !nodeHasFinalChildRefs(clone) {
		t.Fatal("clone did not preserve lazy final-child refs")
	}
	if got := clone.ChildCount(); got != 2 {
		t.Fatalf("clone child count = %d, want 2", got)
	}
	if clone.ownerArena != cloneArena {
		t.Fatal("clone owner arena mismatch")
	}
	child := clone.Child(0)
	if child == nil {
		t.Fatal("clone first child = nil")
	}
	if child.ownerArena != cloneArena {
		t.Fatal("clone child owner arena mismatch")
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("source final child ref single children after clone child access = %d, want 0", got)
	}
	if got := cloneArena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("clone final child ref single children after child access = %d, want 1", got)
	}
}

func TestCloneTreeNodesWithOffsetKeepsFinalChildRefsRangeLazy(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftLeaf.parseState = 11
	rightLeaf := newCompactFullLeafInArena(arena, 2, true, 2, 6, Point{Row: 1}, Point{Row: 1, Column: 4})
	rightLeaf.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(leftLeaf.parseState, leftLeaf),
		newStackEntryCompactFullLeaf(rightLeaf.parseState, rightLeaf),
	}, 0, 6, Point{}, Point{Row: 1, Column: 4}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}

	clone := cloneTreeNodesWithOffset(root, 10, Point{Row: 3, Column: 7})
	if clone == nil {
		t.Fatal("clone = nil")
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("source final child ref single children materialized during offset clone = %d, want 0", got)
	}
	if !nodeHasFinalChildRefs(clone) {
		t.Fatal("offset clone did not preserve lazy final-child refs")
	}
	first := clone.Child(0)
	if first == nil {
		t.Fatal("offset first child nil")
	}
	if first.ownerArena == arena {
		t.Fatal("offset first child reused source arena")
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("source final child ref single children after offset clone child access = %d, want 0", got)
	}
	if got, want := first.StartByte(), uint32(10); got != want {
		t.Fatalf("first child start byte = %d, want %d", got, want)
	}
	if got, want := first.StartPoint(), (Point{Row: 3, Column: 7}); got != want {
		t.Fatalf("first child start point = %+v, want %+v", got, want)
	}
	second := clone.Child(1)
	if second == nil {
		t.Fatal("offset second child nil")
	}
	if got, want := second.StartByte(), uint32(12); got != want {
		t.Fatalf("second child start byte = %d, want %d", got, want)
	}
	if got, want := second.StartPoint(), (Point{Row: 4}); got != want {
		t.Fatalf("second child start point = %+v, want %+v", got, want)
	}
}

func TestCloneNodeInArenaDropsSourceFinalChildSidecarID(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftLeaf.parseState = 11
	rightLeaf := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightLeaf.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(leftLeaf.parseState, leftLeaf),
		newStackEntryCompactFullLeaf(rightLeaf.parseState, rightLeaf),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	clone := cloneNodeInArena(arena, root)
	if clone == nil {
		t.Fatal("clone = nil")
	}
	if nodeHasFinalChildRefs(clone) {
		t.Fatal("clone retained source final-child sidecar id")
	}
	if got := clone.ChildCount(); got != 2 {
		t.Fatalf("clone ChildCount = %d, want 2", got)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 2 {
		t.Fatalf("single children materialized = %d, want 2", got)
	}
	clone.children = nil
	if got := clone.ChildCount(); got != 0 {
		t.Fatalf("clone ChildCount after children rewrite = %d, want 0", got)
	}
}

func TestMutationClonePatchesFinalChildRefsWithoutSourceMaterialization(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftLeaf.parseState = 11
	rightLeaf := newCompactFullLeafInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightLeaf.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(leftLeaf.parseState, leftLeaf),
		newStackEntryCompactFullLeaf(rightLeaf.parseState, rightLeaf),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	parent.parseState = 13

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}

	cloneArena := newNodeArena(arenaClassFull)
	cloneArena.finalChildRefs = true
	replacement := newLeafNodeInArena(cloneArena, 9, true, 1, 3, Point{Column: 1}, Point{Column: 3})
	replacement.parseState = 19
	clone := cloneNodeInArenaReplacingChildForMutation(cloneArena, root, 1, replacement)
	if clone == nil {
		t.Fatal("replacement clone = nil")
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("source single children materialized during replacement clone = %d, want 0", got)
	}
	if !nodeHasFinalChildRefs(clone) {
		t.Fatal("replacement clone did not preserve lazy final-child refs")
	}
	if got := clone.ChildCount(); got != 2 {
		t.Fatalf("replacement clone ChildCount = %d, want 2", got)
	}
	if got := clone.Child(1); got != replacement {
		t.Fatalf("replacement child = %p, want %p", got, replacement)
	}
	if parent, index, ok := nodeParentLink(replacement); !ok || parent != clone || index != 1 {
		t.Fatalf("replacement parent link = (%p, %d, %v), want (%p, 1, true)", parent, index, ok, clone)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("source single children materialized after replacement access = %d, want 0", got)
	}

	appended := cloneNodeInArenaAppendingChildForMutation(cloneArena, root, replacement)
	if appended == nil {
		t.Fatal("append clone = nil")
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("source single children materialized during append clone = %d, want 0", got)
	}
	if !nodeHasFinalChildRefs(appended) {
		t.Fatal("append clone did not preserve lazy final-child refs")
	}
	if got := appended.ChildCount(); got != 3 {
		t.Fatalf("append clone ChildCount = %d, want 3", got)
	}
	if got := appended.Child(2); got != replacement {
		t.Fatalf("appended child = %p, want %p", got, replacement)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("source single children materialized after append access = %d, want 0", got)
	}
}

func TestResultMutableChildViewSurroundFinalRefsKeepsChildrenLazy(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	left := newCompactFullLeafInArena(arena, 1, true, 5, 6, Point{Column: 5}, Point{Column: 6})
	left.parseState = 11
	right := newCompactFullLeafInArena(arena, 2, true, 6, 7, Point{Column: 6}, Point{Column: 7})
	right.parseState = 12
	parent := newPendingParentInArena(arena, 3, true, 13, []stackEntry{
		newStackEntryCompactFullLeaf(left.parseState, left),
		newStackEntryCompactFullLeaf(right.parseState, right),
	}, 5, 7, Point{Column: 5}, Point{Column: 7}, false)
	parent.parseState = 14

	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	leading := newLeafNodeInArena(arena, 9, true, 0, 1, Point{}, Point{Column: 1})
	trailing := newLeafNodeInArena(arena, 10, true, 8, 9, Point{Column: 8}, Point{Column: 9})
	if !resultMutableChildrenForMutation(root).SurroundFinalRefs([]*Node{leading}, []*Node{trailing}) {
		t.Fatal("SurroundFinalRefs returned false")
	}
	if got := root.ChildCount(); got != 4 {
		t.Fatalf("root ChildCount = %d, want 4", got)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("single final child refs materialized during surround = %d, want 0", got)
	}
	if got := root.Child(0); got != leading {
		t.Fatalf("leading child = %p, want %p", got, leading)
	}
	if got := root.Child(3); got != trailing {
		t.Fatalf("trailing child = %p, want %p", got, trailing)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("single final child refs after edge access = %d, want 0", got)
	}
	if got := root.Child(1); got == nil || got.Symbol() != left.symbol {
		t.Fatalf("first compact child = %#v, want left symbol", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 1 {
		t.Fatalf("single final child refs after compact access = %d, want 1", got)
	}
}

func TestPendingParentMaterializationPreservesPackedFieldEntries(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	parent := newPendingParentShellInArena(arena, 4, true, 5, 2, 0, 2, Point{}, Point{Column: 2}, false)
	parent.setChildEntry(arena, 0, newStackEntryNode(6, left))
	parent.setChildEntry(arena, 1, newStackEntryNode(7, right))
	parent.setChildFieldEntry(arena, 1, 9, fieldSourceDirect)
	if got := arena.pendingChildEntriesAllocated; got != 2 {
		t.Fatalf("pendingChildEntriesAllocated = %d, want one packed slot per child", got)
	}

	entry := newStackEntryPendingParent(8, parent)
	node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if node == nil {
		t.Fatal("materialized node = nil")
	}
	if len(node.fieldIDs()) != 2 || node.fieldIDs()[0] != 0 || node.fieldIDs()[1] != 9 {
		t.Fatalf("fieldIDs = %#v, want [0 9]", node.fieldIDs())
	}
	if len(node.fieldSources()) != 2 || node.fieldSources()[0] != fieldSourceNone || node.fieldSources()[1] != fieldSourceDirect {
		t.Fatalf("fieldSources = %#v, want [none direct]", node.fieldSources())
	}
	if node.flags&(pendingParentFlagFieldEntries|pendingParentFlagDirectFieldEntry) != 0 {
		t.Fatalf("internal pending field flags leaked to materialized node flags: %08b", node.flags)
	}
}

func TestPendingParentMaterializationUsesPackedDirectFieldEntriesWithoutParser(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	extra := newLeafNodeInArena(arena, 3, false, 0, 1, Point{}, Point{Column: 1})
	extra.setExtra(true)
	left := newLeafNodeInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	right := newLeafNodeInArena(arena, 2, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	parent := newPendingParentShellInArena(arena, 4, true, 5, 3, 0, 3, Point{}, Point{Column: 3}, false)
	parent.setChildEntry(arena, 0, newStackEntryNode(6, extra))
	parent.setChildEntry(arena, 1, newStackEntryNode(7, left))
	parent.setChildEntry(arena, 2, newStackEntryNode(8, right))
	parent.setHasDirectFieldEntries(true)
	parent.setChildFieldEntry(arena, 1, 7, fieldSourceDirect)
	parent.setChildFieldEntry(arena, 2, 9, fieldSourceDirect)

	entry := newStackEntryPendingParent(9, parent)
	node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if node == nil {
		t.Fatal("materialized node = nil")
	}
	if got := arena.pendingChildEntriesAllocated; got != 3 {
		t.Fatalf("pendingChildEntriesAllocated = %d, want direct child slots only", got)
	}
	if len(node.fieldIDs()) != 3 || node.fieldIDs()[0] != 0 || node.fieldIDs()[1] != 7 || node.fieldIDs()[2] != 9 {
		t.Fatalf("fieldIDs = %#v, want [0 7 9]", node.fieldIDs())
	}
	if len(node.fieldSources()) != 3 || node.fieldSources()[0] != fieldSourceNone || node.fieldSources()[1] != fieldSourceDirect || node.fieldSources()[2] != fieldSourceDirect {
		t.Fatalf("fieldSources = %#v, want [none direct direct]", node.fieldSources())
	}
	if node.flags&(pendingParentFlagFieldEntries|pendingParentFlagDirectFieldEntry) != 0 {
		t.Fatalf("internal pending field flags leaked to materialized node flags: %08b", node.flags)
	}
}

func TestPendingChildEntryPackedFieldMetadataSurvivesPayloadReplacement(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	first := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	replacement := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	parent := newPendingParentShellInArena(arena, 4, true, 5, 1, 0, 1, Point{}, Point{Column: 1}, false)
	parent.setChildEntry(arena, 0, newStackEntryNode(6, first))
	parent.setChildFieldEntry(arena, 0, FieldID(^uint16(0)), fieldSourceInherited)

	parent.setChildEntry(arena, 0, newStackEntryNode(7, replacement))
	entry := parent.childEntry(arena, 0)
	if stackEntryNode(entry) != replacement {
		t.Fatalf("replacement payload = %p, want %p", stackEntryNode(entry), replacement)
	}
	if fid, source := parent.childFieldEntry(arena, 0); fid != FieldID(^uint16(0)) || source != fieldSourceInherited {
		t.Fatalf("packed field metadata = (%d,%d), want (%d,%d)", fid, source, FieldID(^uint16(0)), fieldSourceInherited)
	}
}

func TestPendingPackedDirectFieldsRemainTransparentToHiddenCompaction(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	direct := newPendingParentInArena(arena, 4, true, 5, []stackEntry{newStackEntryNode(6, leaf)}, 0, 1, Point{}, Point{Column: 1}, false)
	direct.setHasDirectFieldEntries(true)
	direct.setChildFieldEntry(arena, 0, 7, fieldSourceDirect)
	if direct.hasFieldEntries() || !direct.hasDirectFieldEntries() {
		t.Fatalf("packed direct-field classification = dense:%t direct:%t", direct.hasFieldEntries(), direct.hasDirectFieldEntries())
	}
	if stackEntryTreeHasFieldIDs(newStackEntryPendingParent(7, direct), arena) {
		t.Fatal("packed direct fields became an opaque hidden-compaction barrier")
	}

	dense := newPendingParentInArena(arena, 4, true, 5, []stackEntry{newStackEntryNode(6, leaf)}, 0, 1, Point{}, Point{Column: 1}, false)
	dense.setHasFieldEntries(true)
	dense.setChildFieldEntry(arena, 0, 7, fieldSourceDirect)
	if !stackEntryTreeHasFieldIDs(newStackEntryPendingParent(7, dense), arena) {
		t.Fatal("dense packed fields lost their existing hidden-compaction barrier")
	}
}

func TestPendingParentMergeEqualityVerifiesPackedFieldsAndDescendants(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	makeEntry := func(field FieldID, source uint8, leafSymbol Symbol) stackEntry {
		leaf := newLeafNodeInArena(arena, leafSymbol, true, 0, 1, Point{}, Point{Column: 1})
		leaf.parseState = 3
		inner := newPendingParentInArena(arena, 10, true, 2, []stackEntry{newStackEntryNode(3, leaf)}, 0, 1, Point{}, Point{Column: 1}, false)
		inner.parseState = 4
		inner.setHasDirectFieldEntries(true)
		inner.setChildFieldEntry(arena, 0, field, source)
		outer := newPendingParentInArena(arena, 20, true, 3, []stackEntry{newStackEntryPendingParent(4, inner)}, 0, 1, Point{}, Point{Column: 1}, false)
		outer.parseState = 5
		return newStackEntryPendingParent(5, outer)
	}
	makeStack := func(entry stackEntry) glrStack {
		return glrStack{
			entries:    []stackEntry{{state: 1}, entry},
			byteOffset: 1,
		}
	}

	base := makeEntry(7, fieldSourceDirect, 1)
	same := makeEntry(7, fieldSourceDirect, 1)
	fieldCollision := makeEntry(8, fieldSourceDirect, 1)
	fieldSourceCollision := makeEntry(7, fieldSourceInherited, 1)
	descendantCollision := makeEntry(7, fieldSourceDirect, 2)
	for name, candidate := range map[string]stackEntry{
		"field":        fieldCollision,
		"field-source": fieldSourceCollision,
		"descendant":   descendantCollision,
	} {
		if got, want := gssEntryHash(gssHashSeed, base), gssEntryHash(gssHashSeed, candidate); got != want {
			t.Fatalf("%s setup hashes differ: got %x want coarse collision %x", name, got, want)
		}
	}

	scratch := glrMergeScratch{arena: arena}
	scratch.beginEquivEpoch()
	if !stackEquivalentForLanguageWithScratch(&scratch, nil, testGLRStackPtr(makeStack(base)), testGLRStackPtr(makeStack(same))) {
		t.Fatal("structurally identical pending parents were not equivalent")
	}
	materializedSame := same
	if node := materializeStackEntryPendingParent(arena, &materializedSame, pendingParentMaterializeForFinalTree); node == nil {
		t.Fatal("materializing equivalent pending parent returned nil")
	}
	if !stackEquivalentForLanguageWithScratch(&scratch, nil, testGLRStackPtr(makeStack(base)), testGLRStackPtr(makeStack(materializedSame))) {
		t.Fatal("pending and materialized representations of the same graph were not equivalent")
	}
	if stackEquivalentForLanguageWithScratch(&scratch, nil, testGLRStackPtr(makeStack(base)), testGLRStackPtr(makeStack(fieldCollision))) {
		t.Fatal("packed field mismatch survived exact pending-parent equality")
	}
	if stackEquivalentForLanguageWithScratch(&scratch, nil, testGLRStackPtr(makeStack(base)), testGLRStackPtr(makeStack(fieldSourceCollision))) {
		t.Fatal("packed field-source mismatch survived exact pending-parent equality")
	}
	if stackEquivalentForLanguageWithScratch(&scratch, nil, testGLRStackPtr(makeStack(base)), testGLRStackPtr(makeStack(descendantCollision))) {
		t.Fatal("recursive descendant mismatch survived exact pending-parent equality")
	}

	stacks := []glrStack{makeStack(base), makeStack(descendantCollision)}
	if got := len(mergeStacksWithScratch(stacks, &scratch)); got != 2 {
		t.Fatalf("merge result = %d stacks, want both coarse-hash collision alternatives", got)
	}

	var gssScratch gssScratch
	sharedPrev := gssScratch.allocNode(stackEntry{state: 1}, nil, 1)
	leftHead := gssScratch.allocNode(base, sharedPrev, 2)
	rightHead := gssScratch.allocNode(descendantCollision, sharedPrev, 2)
	leftStack := glrStack{gss: gssStack{head: leftHead}, byteOffset: 1}
	rightStack := glrStack{gss: gssStack{head: rightHead}, byteOffset: 1}
	if !gssMainMergeWithScratch(&scratch, &leftStack, &rightStack) {
		t.Fatal("lossless GSS merge rejected distinct pending alternatives")
	}
	if got := leftHead.linkCount(); got != 2 {
		t.Fatalf("GSS pending collision links = %d, want both alternatives", got)
	}

	noArena := glrMergeScratch{}
	noArena.beginEquivEpoch()
	if stackEquivalentForLanguageWithScratch(&noArena, nil, testGLRStackPtr(makeStack(base)), testGLRStackPtr(makeStack(same))) {
		t.Fatal("distinct pending parents without an arena must fail closed")
	}
}

func TestNodeMergeEqualityDistinguishesDeferredFieldSources(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	direct := newParentNodeInArenaWithFieldSources(
		arena, 2, false, []*Node{leaf}, []FieldID{7}, []uint8{fieldSourceDirect}, 0,
	)
	defaultDirect := newParentNodeInArena(arena, 2, false, []*Node{leaf}, []FieldID{7}, 0)
	deferred := newParentNodeInArenaWithFieldSources(
		arena, 2, false, []*Node{leaf}, []FieldID{7}, []uint8{fieldSourceDeferredDirect}, 0,
	)
	makeStack := func(node *Node) glrStack {
		return glrStack{
			entries:    []stackEntry{{state: 1}, newStackEntryNode(2, node)},
			byteOffset: 1,
		}
	}

	scratch := glrMergeScratch{arena: arena}
	scratch.beginEquivEpoch()
	if !stackEquivalentForLanguageWithScratch(&scratch, nil, testGLRStackPtr(makeStack(direct)), testGLRStackPtr(makeStack(defaultDirect))) {
		t.Fatal("implicit and explicit direct field sources were not equivalent")
	}
	if stackEquivalentForLanguageWithScratch(&scratch, nil, testGLRStackPtr(makeStack(direct)), testGLRStackPtr(makeStack(deferred))) {
		t.Fatal("deferred and resolved field sources merged before the visible boundary")
	}
}

func TestNoTreeNodeStackEntryKeepsBytesAndDropsPoints(t *testing.T) {
	leaf := newNoTreeLeafNodeInArena(nil, 7, true, 11, 19, Point{Row: 3, Column: 5}, Point{Row: 3, Column: 13})
	entry := newStackEntryNoTreeNode(2, leaf)

	if got := stackEntryNodeStartByte(entry); got != 11 {
		t.Fatalf("start byte = %d, want 11", got)
	}
	if got := stackEntryNodeEndByte(entry); got != 19 {
		t.Fatalf("end byte = %d, want 19", got)
	}
	if got := stackEntryNodeStartPoint(entry); got != (Point{}) {
		t.Fatalf("start point = %#v, want zero point", got)
	}
	if got := stackEntryNodeEndPoint(entry); got != (Point{}) {
		t.Fatalf("end point = %#v, want zero point", got)
	}
}

func TestCompactFullLeafStackEntryKeepsPointsAndMaterializes(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	leaf := newCompactFullLeafInArena(arena, 9, true, 13, 21, Point{Row: 2, Column: 3}, Point{Row: 2, Column: 11})
	entry := newStackEntryCompactFullLeaf(4, leaf)

	if got := stackEntryNode(entry); got != nil {
		t.Fatalf("stackEntryNode = %p, want nil before materialization", got)
	}
	if got := stackEntryNodeStartPoint(entry); got != (Point{Row: 2, Column: 3}) {
		t.Fatalf("start point = %#v", got)
	}
	if got := stackEntryNodeEndPoint(entry); got != (Point{Row: 2, Column: 11}) {
		t.Fatalf("end point = %#v", got)
	}

	node := materializeStackEntryCompactFullLeaf(arena, &entry, compactFullLeafMaterializeForParentReduce)
	if node == nil {
		t.Fatal("materialized node = nil")
	}
	if got := stackEntryNode(entry); got != node {
		t.Fatal("entry was not retargeted to materialized node")
	}
	if got := node.startPoint; got != (Point{Row: 2, Column: 3}) {
		t.Fatalf("node start point = %#v", got)
	}
	if got := arena.compactFullLeafCreated; got != 1 {
		t.Fatalf("compactFullLeafCreated = %d, want 1", got)
	}
	if got := arena.compactFullLeafMaterialized; got != 1 {
		t.Fatalf("compactFullLeafMaterialized = %d, want 1", got)
	}
	if got := arena.compactFullLeafMaterializedForParentReduce; got != 1 {
		t.Fatalf("compactFullLeafMaterializedForParentReduce = %d, want 1", got)
	}
}

func TestMaterializationReasonCountersClassifyNonParentReasons(t *testing.T) {
	arena := newNodeArena(arenaClassFull)

	leaf := newCompactFullLeafInArena(arena, 9, true, 13, 21, Point{Row: 2, Column: 3}, Point{Row: 2, Column: 11})
	leafEntry := newStackEntryCompactFullLeaf(4, leaf)
	_ = materializeStackEntryCompactFullLeaf(arena, &leafEntry, compactFullLeafMaterializeForNormalization)
	if got := arena.compactFullLeafMaterializedForNormalization; got != 1 {
		t.Fatalf("compactFullLeafMaterializedForNormalization = %d, want 1", got)
	}
	if got := arena.compactFullLeafMaterializedForParentReduce; got != 0 {
		t.Fatalf("compactFullLeafMaterializedForParentReduce = %d, want 0", got)
	}

	fallbackLeaf := newCompactFullLeafInArena(arena, 9, true, 22, 23, Point{Row: 3, Column: 0}, Point{Row: 3, Column: 1})
	fallbackEntry := newStackEntryCompactFullLeaf(4, fallbackLeaf)
	_ = materializeStackEntryPendingParent(arena, &fallbackEntry, pendingParentMaterializeForQuery)
	if got := arena.compactFullLeafMaterializedForQuery; got != 1 {
		t.Fatalf("compactFullLeafMaterializedForQuery = %d, want 1", got)
	}
	if got := arena.compactFullLeafMaterializedForParentReduce; got != 0 {
		t.Fatalf("compactFullLeafMaterializedForParentReduce after fallback = %d, want 0", got)
	}

	parent := newPendingParentInArena(arena, 10, true, 7, nil, 0, 0, Point{}, Point{}, false)
	parentEntry := newStackEntryPendingParent(5, parent)
	_ = materializeStackEntryPendingParent(arena, &parentEntry, pendingParentMaterializeForQuery)
	if got := arena.pendingParentMaterializedForQuery; got != 1 {
		t.Fatalf("pendingParentMaterializedForQuery = %d, want 1", got)
	}
	if got := arena.pendingParentMaterializedForParentReduce; got != 0 {
		t.Fatalf("pendingParentMaterializedForParentReduce = %d, want 0", got)
	}
}

func TestPendingParentMaterializationPropagatesReasonToPayloads(t *testing.T) {
	arena := newNodeArena(arenaClassFull)

	leaf := newCompactFullLeafInArena(arena, 9, true, 13, 21, Point{Row: 2, Column: 3}, Point{Row: 2, Column: 11})
	leafEntry := newStackEntryCompactFullLeaf(4, leaf)
	childParent := newPendingParentInArena(arena, 11, true, 8, []stackEntry{leafEntry}, 13, 21, Point{Row: 2, Column: 3}, Point{Row: 2, Column: 11}, false)
	childEntry := newStackEntryPendingParent(5, childParent)
	rootParent := newPendingParentInArena(arena, 10, true, 7, []stackEntry{childEntry}, 13, 21, Point{Row: 2, Column: 3}, Point{Row: 2, Column: 11}, false)
	rootEntry := newStackEntryPendingParent(6, rootParent)

	if node := materializeStackEntryPendingParent(arena, &rootEntry, pendingParentMaterializeForFinalTree); node == nil {
		t.Fatal("materialized root = nil")
	}
	if got := arena.pendingParentMaterializedForFinalTree; got != 2 {
		t.Fatalf("pendingParentMaterializedForFinalTree = %d, want 2", got)
	}
	if got := arena.pendingParentMaterializedForParentReduce; got != 0 {
		t.Fatalf("pendingParentMaterializedForParentReduce = %d, want 0", got)
	}
	if got := arena.compactFullLeafMaterializedForFinalTree; got != 1 {
		t.Fatalf("compactFullLeafMaterializedForFinalTree = %d, want 1", got)
	}
	if got := arena.compactFullLeafMaterializedForParentReduce; got != 0 {
		t.Fatalf("compactFullLeafMaterializedForParentReduce = %d, want 0", got)
	}
}

func TestPendingParentRejectCountersClassifyReasons(t *testing.T) {
	arena := newNodeArena(arenaClassFull)

	arena.recordPendingParentRejected(pendingParentRejectAlias)
	arena.recordPendingParentRejected(pendingParentRejectFields)
	arena.recordPendingParentRejected(pendingParentRejectChild)

	if got := arena.pendingParentRejectedAlias; got != 1 {
		t.Fatalf("pendingParentRejectedAlias = %d, want 1", got)
	}
	if got := arena.pendingParentRejectedFields; got != 1 {
		t.Fatalf("pendingParentRejectedFields = %d, want 1", got)
	}
	if got := arena.pendingParentRejectedChild; got != 1 {
		t.Fatalf("pendingParentRejectedChild = %d, want 1", got)
	}
	if got := arena.pendingParentRejectedRawSpan; got != 0 {
		t.Fatalf("pendingParentRejectedRawSpan = %d, want 0", got)
	}
	if got := arena.pendingParentLastRejectReason; got != pendingParentRejectChild {
		t.Fatalf("pendingParentLastRejectReason = %d, want child", got)
	}
}

func TestPendingParentFieldRejectCountersClassifyShapes(t *testing.T) {
	cases := []struct {
		name  string
		shape pendingParentFieldRejectShape
		got   func(PendingParentFieldRejectStats) uint64
	}{
		{"parent-hidden", pendingParentFieldRejectParentHidden, func(s PendingParentFieldRejectStats) uint64 { return s.ParentHidden }},
		{"no-ids", pendingParentFieldRejectNoIDs, func(s PendingParentFieldRejectStats) uint64 { return s.NoIDs }},
		{"inherited", pendingParentFieldRejectInherited, func(s PendingParentFieldRejectStats) uint64 { return s.Inherited }},
		{"hidden-child", pendingParentFieldRejectHiddenChild, func(s PendingParentFieldRejectStats) uint64 { return s.HiddenChild }},
		{"hidden-child-plain", pendingParentFieldRejectHiddenChildPlain, func(s PendingParentFieldRejectStats) uint64 { return s.HiddenChildPlain }},
		{"hidden-child-plain-empty", pendingParentFieldRejectHiddenChildPlainEmpty, func(s PendingParentFieldRejectStats) uint64 { return s.HiddenChildPlainEmpty }},
		{"hidden-child-plain-one", pendingParentFieldRejectHiddenChildPlainOne, func(s PendingParentFieldRejectStats) uint64 { return s.HiddenChildPlainOne }},
		{"hidden-child-plain-many", pendingParentFieldRejectHiddenChildPlainMany, func(s PendingParentFieldRejectStats) uint64 { return s.HiddenChildPlainMany }},
		{"hidden-child-with-fields", pendingParentFieldRejectHiddenChildWithFields, func(s PendingParentFieldRejectStats) uint64 { return s.HiddenChildWithFields }},
		{"child", pendingParentFieldRejectChild, func(s PendingParentFieldRejectStats) uint64 { return s.Child }},
		{"all-visible-direct", pendingParentFieldRejectAllVisibleDirect, func(s PendingParentFieldRejectStats) uint64 { return s.AllVisibleDirect }},
		{"unknown", pendingParentFieldRejectUnknown, func(s PendingParentFieldRejectStats) uint64 { return s.Unknown }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arena := newNodeArena(arenaClassFull)
			arena.recordPendingParentFieldRejected(tc.shape)
			if got := arena.pendingParentLastFieldRejectShape; got != tc.shape {
				t.Fatalf("pendingParentLastFieldRejectShape = %d, want %d", got, tc.shape)
			}
			stats := PendingParentFieldRejectStats{}
			stats.increment(tc.shape)
			if got := tc.got(stats); got != 1 {
				t.Fatalf("field reject stats count = %d, want 1", got)
			}
		})
	}
}

func TestParentRejectPayloadMaterializedCountersClassifyPayloadKind(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.breakdownEnabled = true
	leaf := newCompactFullLeafInArena(arena, 9, true, 13, 21, Point{Row: 2, Column: 3}, Point{Row: 2, Column: 11})
	leafEntry := newStackEntryCompactFullLeaf(4, leaf)
	arena.recordParentRejectPayloadMaterialized(leafEntry, pendingParentRejectFields)
	if got := arena.compactFullLeafMaterializedForParentReject.Fields; got != 1 {
		t.Fatalf("compact leaf parent reject fields = %d, want 1", got)
	}

	parent := newPendingParentInArena(arena, 10, true, 7, nil, 0, 0, Point{}, Point{}, false)
	parentEntry := newStackEntryPendingParent(5, parent)
	arena.recordParentRejectPayloadMaterialized(parentEntry, pendingParentRejectAlias)
	if got := arena.pendingParentMaterializedForParentReject.Alias; got != 1 {
		t.Fatalf("pending parent reject alias = %d, want 1", got)
	}

	arena.pendingParentActiveFieldRejectShape = pendingParentFieldRejectAllVisibleDirect
	arena.recordParentRejectPayloadMaterialized(parentEntry, pendingParentRejectFields)
	if got := arena.pendingParentMaterializedForFieldReject.AllVisibleDirect; got != 1 {
		t.Fatalf("pending parent field reject all-visible-direct = %d, want 1", got)
	}
}

func TestPendingParentFieldRejectPayloadStatsSplitVisibleShapes(t *testing.T) {
	var stats PendingParentFieldRejectPayloadStats
	stats.increment(pendingParentFieldRejectPayloadVisibleFinalLike)
	if got := stats.Visible; got != 1 {
		t.Fatalf("visible aggregate = %d, want 1", got)
	}
	if got := stats.VisibleFinalLike; got != 1 {
		t.Fatalf("visible final-like = %d, want 1", got)
	}
	stats.increment(pendingParentFieldRejectPayloadVisibleNestedPayload)
	if got := stats.Visible; got != 2 {
		t.Fatalf("visible aggregate after nested = %d, want 2", got)
	}
	if got := stats.VisibleNestedPayload; got != 1 {
		t.Fatalf("visible nested payload = %d, want 1", got)
	}
	stats.increment(pendingParentFieldRejectPayloadVisibleCompactLeaf)
	if got := stats.VisibleCompactLeaf; got != 1 {
		t.Fatalf("visible compact leaf = %d, want 1", got)
	}
	stats.increment(pendingParentFieldRejectPayloadVisibleFieldedDescendant)
	if got := stats.VisibleFieldedDesc; got != 1 {
		t.Fatalf("visible fielded descendant = %d, want 1", got)
	}
}

func TestPendingParentFieldRejectPayloadShapeSplitsVisiblePendingParents(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{},
		{Name: "leaf", Visible: true, Named: true},
		{},
		{},
		{},
		{},
		{},
		{},
		{},
		{},
		{Name: "parent", Visible: true, Named: true},
	}}}
	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	finalLike := newPendingParentInArena(arena, 10, true, 7, []stackEntry{newStackEntryNode(2, leaf)}, 0, 1, Point{}, Point{Column: 1}, false)
	if got := parser.pendingParentFieldRejectPayloadShape(newStackEntryPendingParent(3, finalLike), arena); got != pendingParentFieldRejectPayloadVisibleFinalLike {
		t.Fatalf("final-like visible payload shape = %d", got)
	}
	nestedLeaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	nested := newPendingParentInArena(arena, 10, true, 8, []stackEntry{newStackEntryCompactFullLeaf(4, nestedLeaf)}, 0, 1, Point{}, Point{Column: 1}, false)
	if got := parser.pendingParentFieldRejectPayloadShape(newStackEntryPendingParent(5, nested), arena); got != pendingParentFieldRejectPayloadVisibleCompactLeaf {
		t.Fatalf("compact-leaf visible payload shape = %d", got)
	}
	inner := newPendingParentInArena(arena, 10, true, 9, []stackEntry{newStackEntryNode(6, leaf)}, 0, 1, Point{}, Point{Column: 1}, false)
	nestedParent := newPendingParentInArena(arena, 10, true, 10, []stackEntry{newStackEntryPendingParent(7, inner)}, 0, 1, Point{}, Point{Column: 1}, false)
	if got := parser.pendingParentFieldRejectPayloadShape(newStackEntryPendingParent(8, nestedParent), arena); got != pendingParentFieldRejectPayloadVisibleNestedPayload {
		t.Fatalf("nested-parent visible payload shape = %d", got)
	}
	fieldedChild := newParentNodeInArenaNoLinksWithFieldSources(arena, 10, true, []*Node{leaf}, []FieldID{3}, nil, 11, false)
	fieldedParent := newPendingParentInArena(arena, 10, true, 12, []stackEntry{newStackEntryNode(9, fieldedChild)}, 0, 1, Point{}, Point{Column: 1}, false)
	if got := parser.pendingParentFieldRejectPayloadShape(newStackEntryPendingParent(10, fieldedParent), arena); got != pendingParentFieldRejectPayloadVisibleFieldedDescendant {
		t.Fatalf("fielded-desc visible payload shape = %d", got)
	}
}

func TestMaterializePendingPayloadEntriesPropagatesFieldRejectShape(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.breakdownEnabled = true
	parent := newPendingParentInArena(arena, 10, true, 7, nil, 0, 0, Point{}, Point{}, false)
	entries := []stackEntry{newStackEntryPendingParent(5, parent)}

	arena.pendingParentLastRejectReason = pendingParentRejectFields
	arena.pendingParentLastFieldRejectShape = pendingParentFieldRejectAllVisibleDirect
	arena.pendingParentActiveRejectReason = pendingParentRejectAlias
	arena.pendingParentActiveFieldRejectShape = pendingParentFieldRejectHiddenChildPlain
	arena.pendingParentActiveFieldPayloadShape = pendingParentFieldRejectPayloadHiddenMany

	materializePendingPayloadEntries(nil, entries, 0, len(entries), arena)

	if got := stackEntryPendingParent(entries[0]); got != nil {
		t.Fatalf("entry still pending parent = %p, want materialized node", got)
	}
	if got := arena.pendingParentMaterializedForParentReject.Fields; got != 1 {
		t.Fatalf("pending parent materialized parent reject fields = %d, want 1", got)
	}
	if got := arena.pendingParentMaterializedForFieldReject.AllVisibleDirect; got != 1 {
		t.Fatalf("pending parent materialized field reject all-visible-direct = %d, want 1", got)
	}
	if got := arena.pendingParentActiveRejectReason; got != pendingParentRejectAlias {
		t.Fatalf("active reject reason restored = %d, want alias", got)
	}
	if got := arena.pendingParentActiveFieldRejectShape; got != pendingParentFieldRejectHiddenChildPlain {
		t.Fatalf("active field reject shape restored = %d, want hidden-child", got)
	}
	if got := arena.pendingParentActiveFieldPayloadShape; got != pendingParentFieldRejectPayloadHiddenMany {
		t.Fatalf("active field payload shape restored = %d, want hidden-many", got)
	}
}

func TestMaterializeStackEntryPayloadTracksActiveParentReject(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.breakdownEnabled = true
	leaf := newCompactFullLeafInArena(arena, 9, true, 13, 21, Point{Row: 2, Column: 3}, Point{Row: 2, Column: 11})
	entry := newStackEntryCompactFullLeaf(4, leaf)

	arena.pendingParentActiveRejectReason = pendingParentRejectFields
	node := materializeStackEntryPayload(arena, &entry, compactFullLeafMaterializeForParentReduce, pendingParentMaterializeForParentReduce)
	if node == nil {
		t.Fatal("materialized node = nil")
	}
	if got := arena.compactFullLeafMaterializedForParentReject.Fields; got != 1 {
		t.Fatalf("compact leaf active parent reject fields = %d, want 1", got)
	}

	parent := newPendingParentInArena(arena, 10, true, 7, nil, 0, 0, Point{}, Point{}, false)
	parentEntry := newStackEntryPendingParent(5, parent)
	arena.pendingParentActiveFieldRejectShape = pendingParentFieldRejectHiddenChildWithFields
	node = materializeStackEntryPayload(arena, &parentEntry, compactFullLeafMaterializeForParentReduce, pendingParentMaterializeForParentReduce)
	if node == nil {
		t.Fatal("materialized pending parent = nil")
	}
	if got := arena.pendingParentMaterializedForParentReject.Fields; got != 1 {
		t.Fatalf("pending parent active parent reject fields = %d, want 1", got)
	}
	if got := arena.pendingParentMaterializedForFieldReject.HiddenChildWithFields; got != 1 {
		t.Fatalf("pending parent active field reject hidden-child-with-fields = %d, want 1", got)
	}
}

func TestMaterializePendingParentRecursionClassifiesNestedPendingPayloadShape(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.breakdownEnabled = true
	lang := &Language{SymbolMetadata: []SymbolMetadata{
		{},
		{Name: "leaf", Visible: true, Named: true},
		{},
		{},
		{},
		{},
		{},
		{},
		{},
		{},
		{Name: "outer", Visible: true, Named: true},
		{},
		{},
		{},
		{},
		{},
		{},
		{},
		{},
		{},
		{Name: "hidden", Visible: false, Named: false},
	}}
	parser := &Parser{language: lang}
	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	hidden := newPendingParentInArena(arena, 20, false, 3, []stackEntry{newStackEntryNode(4, leaf)}, 0, 1, Point{}, Point{Column: 1}, false)
	outer := newPendingParentInArena(arena, 10, true, 5, []stackEntry{newStackEntryPendingParent(6, hidden)}, 0, 1, Point{}, Point{Column: 1}, false)
	entries := []stackEntry{newStackEntryPendingParent(7, outer)}

	arena.pendingParentLastRejectReason = pendingParentRejectFields
	arena.pendingParentLastFieldRejectShape = pendingParentFieldRejectAllVisibleDirect
	materializePendingPayloadEntries(parser, entries, 0, len(entries), arena)

	if got := arena.pendingParentMaterializedForFieldRejectPayload.Visible; got != 1 {
		t.Fatalf("visible payload count = %d, want 1", got)
	}
	if got := arena.pendingParentMaterializedForFieldRejectPayload.HiddenOne; got != 1 {
		t.Fatalf("hidden-one payload count = %d, want 1", got)
	}
}

func TestMaterializePendingParentRecursionClassifiesNestedCompactLeafPayloadShape(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.breakdownEnabled = true
	lang := &Language{SymbolMetadata: []SymbolMetadata{
		{},
		{},
		{Name: "hidden_leaf", Visible: false, Named: false},
	}}
	parser := &Parser{language: lang}
	leaf := newCompactFullLeafInArena(arena, 2, false, 0, 1, Point{}, Point{Column: 1})
	entries := []stackEntry{newStackEntryCompactFullLeaf(3, leaf)}

	arena.pendingParentLastRejectReason = pendingParentRejectFields
	arena.pendingParentLastFieldRejectShape = pendingParentFieldRejectHiddenChildPlainEmpty
	materializePendingPayloadEntries(parser, entries, 0, len(entries), arena)

	if got := arena.compactFullLeafMaterializedForParentReject.Fields; got != 1 {
		t.Fatalf("compact leaf reject fields = %d, want 1", got)
	}
	if got := arena.compactFullLeafMaterializedForFieldRejectPayload.HiddenEmpty; got != 1 {
		t.Fatalf("compact leaf hidden-empty payload count = %d, want 1", got)
	}
}

func TestStackResultErrorRankIncludesCompactRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	errorLeaf := newCompactFullLeafInArena(arena, errorSymbol, false, 0, 1, Point{}, Point{Column: 1})
	leafStack := &glrStack{entries: []stackEntry{newStackEntryCompactFullLeaf(2, errorLeaf)}}
	if got := stackResultErrorRank(leafStack, arena); got != 2 {
		t.Fatalf("compact leaf error rank = %d, want 2", got)
	}

	child := newLeafNodeInArena(arena, errorSymbol, false, 0, 1, Point{}, Point{Column: 1})
	parent := newPendingParentInArena(arena, 10, true, 7, []stackEntry{newStackEntryNode(3, child)}, 0, 1, Point{}, Point{Column: 1}, false)
	parentStack := &glrStack{entries: []stackEntry{newStackEntryPendingParent(4, parent)}}
	if got := stackResultErrorRank(parentStack, arena); got != 2 {
		t.Fatalf("pending parent child error rank = %d, want 2", got)
	}
}

func TestCachedStackEntryErrorRankUsesInlineNodeCache(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	entry := newStackEntryNode(2, leaf)

	if got := cachedStackEntryErrorRank(entry, arena); got != 0 {
		t.Fatalf("clean leaf rank = %d, want 0", got)
	}
	if got := leaf.errorRankCache; got != 1 {
		t.Fatalf("clean leaf cache = %d, want encoded rank 1", got)
	}
	if got := cachedStackEntryErrorRank(entry, arena); got != 0 {
		t.Fatalf("cached clean leaf rank = %d, want 0", got)
	}

	leaf.setHasError(true)
	nodeBumpEquivVersion(leaf)
	if leaf.errorRankCache != 0 {
		t.Fatalf("cache after mutation = %d, want unknown", leaf.errorRankCache)
	}
	if got := cachedStackEntryErrorRank(entry, arena); got != 1 {
		t.Fatalf("has-error leaf rank = %d, want 1", got)
	}
	if got := leaf.errorRankCache; got != 2 {
		t.Fatalf("has-error leaf cache = %d, want encoded rank 2", got)
	}

	leaf.symbol = errorSymbol
	nodeBumpEquivVersion(leaf)
	if got := cachedStackEntryErrorRank(entry, arena); got != 2 {
		t.Fatalf("ERROR leaf rank = %d, want 2", got)
	}
	if got := leaf.errorRankCache; got != 3 {
		t.Fatalf("ERROR leaf cache = %d, want encoded rank 3", got)
	}

	cloned := cloneNodeInArena(arena, leaf)
	if got := cloned.errorRankCache; got != 0 {
		t.Fatalf("cloned leaf cache = %d, want unknown", got)
	}
	treeClone := cloneTreeNodesIntoArena(leaf, newNodeArena(arenaClassFull))
	if got := treeClone.errorRankCache; got != 0 {
		t.Fatalf("tree-cloned leaf cache = %d, want unknown", got)
	}
	aliased := aliasedNodeInArena(arena, nil, leaf, 2)
	if got := aliased.errorRankCache; got != 0 {
		t.Fatalf("aliased leaf cache = %d, want unknown", got)
	}
}

func TestCachedStackEntryErrorRankCachesNestedNode(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	errorLeaf := newLeafNodeInArena(arena, errorSymbol, false, 0, 1, Point{}, Point{Column: 1})
	parent := newParentNodeInArena(arena, 10, true, cloneNodeSliceInArena(arena, []*Node{errorLeaf}), nil, 0)

	if got := cachedStackEntryErrorRank(newStackEntryNode(3, parent), arena); got != 2 {
		t.Fatalf("parent rank = %d, want 2", got)
	}
	if got := parent.errorRankCache; got != 3 {
		t.Fatalf("parent cache = %d, want encoded rank 3", got)
	}
	if got := errorLeaf.errorRankCache; got != 3 {
		t.Fatalf("child cache = %d, want encoded rank 3", got)
	}
}

func TestCachedStackEntryErrorRankDoesNotCacheForeignNode(t *testing.T) {
	currentArena := newNodeArena(arenaClassIncremental)
	oldTreeArena := newNodeArena(arenaClassFull)
	errorLeaf := newLeafNodeInArena(oldTreeArena, errorSymbol, false, 0, 1, Point{}, Point{Column: 1})
	parent := newParentNodeInArena(oldTreeArena, 10, true, cloneNodeSliceInArena(oldTreeArena, []*Node{errorLeaf}), nil, 0)
	errorLeaf.errorRankCache = 1 // stale parse-time values from the old tree
	parent.errorRankCache = 1

	if got := cachedStackEntryErrorRank(newStackEntryNode(2, parent), currentArena); got != 2 {
		t.Fatalf("foreign nested rank = %d, want 2", got)
	}
	if got := parent.errorRankCache; got != 1 {
		t.Fatalf("foreign parent cache changed to %d, want untouched value 1", got)
	}
	if got := errorLeaf.errorRankCache; got != 1 {
		t.Fatalf("foreign child cache changed to %d, want untouched value 1", got)
	}
}

func TestCachedStackEntryErrorRankForeignNodeConcurrentReads(t *testing.T) {
	oldTreeArena := newNodeArena(arenaClassFull)
	errorLeaf := newLeafNodeInArena(oldTreeArena, errorSymbol, false, 0, 1, Point{}, Point{Column: 1})
	parent := newParentNodeInArena(oldTreeArena, 10, true, cloneNodeSliceInArena(oldTreeArena, []*Node{errorLeaf}), nil, 0)
	parent.errorRankCache = 1
	entry := newStackEntryNode(2, parent)

	const workers = 8
	errCh := make(chan int, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			arena := newNodeArena(arenaClassIncremental)
			for j := 0; j < 100; j++ {
				if got := cachedStackEntryErrorRank(entry, arena); got != 2 {
					errCh <- got
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for got := range errCh {
		t.Fatalf("foreign concurrent rank = %d, want 2", got)
	}
	if got := parent.errorRankCache; got != 1 {
		t.Fatalf("foreign parent cache changed to %d, want untouched value 1", got)
	}
}

func TestCompareAcceptedStackAliasPreferenceIncludesCompactRefs(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "identifier", "alias_identifier", "root"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
		},
		AliasSequences: [][]Symbol{{2}},
	}
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)

	normalLeaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	aliasLeaf := newCompactFullLeafInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	normalStack := glrStack{entries: []stackEntry{newStackEntryNode(1, normalLeaf)}}
	aliasStack := glrStack{entries: []stackEntry{newStackEntryCompactFullLeaf(1, aliasLeaf)}}
	if got := compareAcceptedStackAliasPreference(parser, arena, &aliasStack, &normalStack); got != 1 {
		t.Fatalf("compact alias preference = %d, want 1", got)
	}
	if got := compareAcceptedStackAliasPreference(parser, arena, &normalStack, &aliasStack); got != -1 {
		t.Fatalf("reverse compact alias preference = %d, want -1", got)
	}

	normalParent := newParentNodeInArena(arena, 3, true, []*Node{normalLeaf}, nil, 0)
	aliasParent := newPendingParentInArena(arena, 3, true, 0, []stackEntry{newStackEntryCompactFullLeaf(1, aliasLeaf)}, 0, 1, Point{}, Point{Column: 1}, false)
	parentNormalStack := glrStack{entries: []stackEntry{newStackEntryNode(2, normalParent)}}
	parentAliasStack := glrStack{entries: []stackEntry{newStackEntryPendingParent(2, aliasParent)}}
	if got := compareAcceptedStackAliasPreference(parser, arena, &parentAliasStack, &parentNormalStack); got != 1 {
		t.Fatalf("pending alias preference = %d, want 1", got)
	}
}

func TestCompareAcceptedStackAliasPreferenceIncludesGSSCompactRefs(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "identifier", "alias_identifier", "root"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
		},
		AliasSequences: [][]Symbol{{2}},
	}
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	var scratch gssScratch

	normalLeaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	aliasLeaf := newCompactFullLeafInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	normalStack := glrStack{entries: []stackEntry{{state: 0}, newStackEntryNode(1, normalLeaf)}}
	aliasGSSStack := glrStack{
		gss: buildGSSStack([]stackEntry{{state: 0}, newStackEntryCompactFullLeaf(1, aliasLeaf)}, &scratch),
	}
	if got := compareAcceptedStackAliasPreference(parser, arena, &aliasGSSStack, &normalStack); got != 1 {
		t.Fatalf("gss compact alias preference = %d, want 1", got)
	}
	if got := compareAcceptedStackAliasPreference(parser, arena, &normalStack, &aliasGSSStack); got != -1 {
		t.Fatalf("reverse gss compact alias preference = %d, want -1", got)
	}

	normalParent := newParentNodeInArena(arena, 3, true, []*Node{normalLeaf}, nil, 0)
	aliasParent := newPendingParentInArena(arena, 3, true, 0, []stackEntry{newStackEntryCompactFullLeaf(1, aliasLeaf)}, 0, 1, Point{}, Point{Column: 1}, false)
	parentNormalGSS := glrStack{
		gss: buildGSSStack([]stackEntry{{state: 0}, newStackEntryNode(2, normalParent)}, &scratch),
	}
	parentAliasGSS := glrStack{
		gss: buildGSSStack([]stackEntry{{state: 0}, newStackEntryPendingParent(2, aliasParent)}, &scratch),
	}
	if got := compareAcceptedStackAliasPreference(parser, arena, &parentAliasGSS, &parentNormalGSS); got != 1 {
		t.Fatalf("gss pending alias preference = %d, want 1", got)
	}
	if got := arena.compactFullLeafMaterialized; got != 0 {
		t.Fatalf("compact leaf materialized during alias preference = %d, want 0", got)
	}
	if got := arena.pendingParentMaterialized; got != 0 {
		t.Fatalf("pending parent materialized during alias preference = %d, want 0", got)
	}
}

func TestCompareAcceptedStackAliasPreferencePreservesGSSResultOrder(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "identifier", "alias_identifier", "property", "alias_property"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
		},
		AliasSequences: [][]Symbol{{2}, {4}},
	}
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	var scratch gssScratch

	bottomAlias := newCompactFullLeafInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	bottomNormal := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	topNormal := newLeafNodeInArena(arena, 3, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	topAlias := newCompactFullLeafInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	preferBottomStack := glrStack{
		gss: buildGSSStack([]stackEntry{
			{state: 0},
			newStackEntryCompactFullLeaf(1, bottomAlias),
			newStackEntryNode(2, topNormal),
		}, &scratch),
	}
	preferTopStack := glrStack{
		gss: buildGSSStack([]stackEntry{
			{state: 0},
			newStackEntryNode(1, bottomNormal),
			newStackEntryCompactFullLeaf(2, topAlias),
		}, &scratch),
	}
	if got := compareAcceptedStackAliasPreference(parser, arena, &preferBottomStack, &preferTopStack); got != 1 {
		t.Fatalf("gss alias preference order = %d, want 1", got)
	}
}

func TestCompareAcceptedStackAliasPreferenceKeepsWideNodeFallback(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "identifier", "alias_identifier"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
		},
		AliasSequences: [][]Symbol{{2}},
	}
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	var scratch gssScratch

	aEntries := []stackEntry{{state: 0}}
	bEntries := []stackEntry{{state: 0}}
	for i := 0; i < 9; i++ {
		symA, symB := Symbol(1), Symbol(1)
		if i == 8 {
			symA = 2
		}
		aNode := newLeafNodeInArena(arena, symA, true, uint32(i), uint32(i+1), Point{Column: uint32(i)}, Point{Column: uint32(i + 1)})
		bNode := newLeafNodeInArena(arena, symB, true, uint32(i), uint32(i+1), Point{Column: uint32(i)}, Point{Column: uint32(i + 1)})
		aEntries = append(aEntries, newStackEntryNode(StateID(i+1), aNode))
		bEntries = append(bEntries, newStackEntryNode(StateID(i+1), bNode))
	}
	aStack := glrStack{gss: buildGSSStack(aEntries, &scratch)}
	bStack := glrStack{gss: buildGSSStack(bEntries, &scratch)}
	if got := compareAcceptedStackAliasPreference(parser, arena, &aStack, &bStack); got != 1 {
		t.Fatalf("wide node alias preference = %d, want 1", got)
	}
}

func TestAliasSequenceFrontierEquivalenceChecksAllSmallSemanticChildren(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "root", "inner", "leaf_a", "leaf_b", "tail", "alias"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "root", Visible: true, Named: true},
			{Name: "inner", Visible: true, Named: true},
			{Name: "leaf_a", Visible: true, Named: true},
			{Name: "leaf_b", Visible: true, Named: true},
			{Name: "tail", Visible: true, Named: true},
			{Name: "alias", Visible: true, Named: true},
		},
		AliasSequences: [][]Symbol{{6}},
	}
	arena := newNodeArena(arenaClassFull)

	leftLeaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	rightLeaf := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
	leftInner := newParentNodeInArena(arena, 2, true, []*Node{leftLeaf}, nil, 0)
	rightInner := newParentNodeInArena(arena, 2, true, []*Node{rightLeaf}, nil, 0)
	leftTail := newLeafNodeInArena(arena, 5, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightTail := newLeafNodeInArena(arena, 5, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftRoot := newParentNodeInArena(arena, 1, true, []*Node{leftInner, leftTail}, nil, 0)
	rightRoot := newParentNodeInArena(arena, 1, true, []*Node{rightInner, rightTail}, nil, 0)

	leftStack := glrStack{entries: []stackEntry{{state: 0}, newStackEntryNode(7, leftRoot)}, byteOffset: 2}
	rightStack := glrStack{entries: []stackEntry{{state: 0}, newStackEntryNode(7, rightRoot)}, byteOffset: 2}
	var scratch glrMergeScratch
	scratch.language = lang
	scratch.beginEquivEpoch()

	if stackEquivalentForMergeState(&scratch, lang, 7, &leftStack, &rightStack) {
		t.Fatal("stackEquivalentForMergeState = true, want false for distinct earlier semantic child")
	}
}

func TestObservePreMaterializationFieldRejectForkCountsSameKey(t *testing.T) {
	parser := &Parser{
		language:           &Language{},
		pendingFullParents: true,
		reduceHasFields:    []bool{true},
	}
	arena := newNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	stack := &glrStack{
		entries: []stackEntry{
			{state: 7},
			newStackEntryNode(8, leaf),
		},
		byteOffset: 1,
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1, ProductionID: 0},
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 0},
	}

	candidates, sameKey, overflow := parser.observePreMaterializationFieldRejectFork(stack, actions, nil, 1)
	if candidates != 2 {
		t.Fatalf("candidates = %d, want 2", candidates)
	}
	if sameKey != 2 {
		t.Fatalf("sameKey = %d, want 2", sameKey)
	}
	if overflow != 1 {
		t.Fatalf("overflow = %d, want 1", overflow)
	}
}

func TestCompactCheckpointLeafStackEntryUsesNoTreePrefix(t *testing.T) {
	leaf := newCompactCheckpointLeafInArena(nil, 9, true, 13, 21, externalScannerCheckpointRef{})
	entry := newStackEntryCompactCheckpointLeaf(4, leaf)

	if got := stackEntryNoTreeNode(entry); got == nil {
		t.Fatal("compact checkpoint leaf did not expose no-tree prefix")
	}
	if got := stackEntryNodeSymbol(entry); got != 9 {
		t.Fatalf("symbol = %d, want 9", got)
	}
	if got := stackEntryNodeStartByte(entry); got != 13 {
		t.Fatalf("start byte = %d, want 13", got)
	}
	if got := stackEntryNodeEndByte(entry); got != 21 {
		t.Fatalf("end byte = %d, want 21", got)
	}
	if got := stackEntryNodeIsNamed(entry); !got {
		t.Fatal("named = false, want true")
	}
}

func TestNoTreeNodeConstructorsResetReusedSlots(t *testing.T) {
	arena := newNodeArena(arenaClassFull)

	stale := newNoTreeLeafNodeInArena(arena, 7, true, 11, 19, Point{}, Point{})
	stale.parseState = 99
	stale.preGotoState = 88
	stale.productionID = 77
	stale.rawShape = rawShapeRef(55)
	stale.dynamicPrecedence = 66
	stale.setExtra(true)
	stale.setMissing(true)
	stale.setHasError(true)

	arena.reset()

	leaf := newNoTreeLeafNodeInArena(arena, 8, false, 23, 29, Point{}, Point{})
	if leaf.parseState != 0 || leaf.preGotoState != 0 || leaf.productionID != 0 || leaf.rawShape != 0 || leaf.dynamicPrecedence != 0 {
		t.Fatalf("leaf reused state = (%d,%d,%d,%d,%d), want zeroes", leaf.parseState, leaf.preGotoState, leaf.productionID, leaf.rawShape, leaf.dynamicPrecedence)
	}
	if leaf.isNamed() || leaf.isExtra() || leaf.isMissing() || leaf.hasError() {
		t.Fatalf("leaf reused flags = %08b, want zero", leaf.flags)
	}

	leaf.setExtra(true)
	leaf.setMissing(true)
	leaf.setHasError(true)
	arena.reset()

	reduced := newNoTreeReduceNodeInArena(arena, ParseAction{Symbol: 9, ProductionID: 13, DynamicPrecedence: 5}, true, nil, 0, 0, Token{StartByte: 31}, false)
	if reduced.parseState != 0 || reduced.preGotoState != 0 || reduced.productionID != 13 || reduced.rawShape != 0 || reduced.dynamicPrecedence != 5 {
		t.Fatalf("reduce reused state = (%d,%d,%d,%d,%d), want production and dynamic precedence only", reduced.parseState, reduced.preGotoState, reduced.productionID, reduced.rawShape, reduced.dynamicPrecedence)
	}
	if !reduced.isNamed() || reduced.isExtra() || reduced.isMissing() || reduced.hasError() {
		t.Fatalf("reduce reused flags = %08b, want named only", reduced.flags)
	}
}

func TestRetargetStackEntryPayloadHandlesNoTreeNode(t *testing.T) {
	node := &noTreeNode{parseState: 3}
	entry := newStackEntryNoTreeNode(2, node)

	got, ok := retargetStackEntryPayload(entry, 7)
	if !ok {
		t.Fatal("retarget compact no-tree payload failed")
	}
	if got.state != 7 {
		t.Fatalf("entry state = %d, want 7", got.state)
	}
	if node.parseState != 7 {
		t.Fatalf("compact parseState = %d, want 7", node.parseState)
	}
	if stackEntryNoTreeNode(got) != node {
		t.Fatal("retarget changed compact payload pointer")
	}
}

func TestMergeStacksRemovesDead(t *testing.T) {
	s1 := newGLRStack(StateID(1))
	s2 := newGLRStack(StateID(2))
	s2.dead = true
	s3 := newGLRStack(StateID(3))

	result := mergeStacks([]glrStack{s1, s2, s3})
	if len(result) != 2 {
		t.Fatalf("expected 2 alive stacks, got %d", len(result))
	}
	if result[0].top().state != 1 || result[1].top().state != 3 {
		t.Errorf("unexpected states: %d, %d", result[0].top().state, result[1].top().state)
	}
}

func TestNodeEquivCacheDepthKeyDoesNotAlias(t *testing.T) {
	var scratch glrMergeScratch
	scratch.beginEquivEpoch()

	a := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})
	b := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})

	storeNodeEquivCache(&scratch, a, b, glrNodeEquivCacheMaxDepth, true)
	if hit, ok := lookupNodeEquivCache(&scratch, a, b, glrNodeEquivCacheMaxDepth); !ok || !hit {
		t.Fatalf("lookup at max cache depth = (%v, %v), want (true, true)", hit, ok)
	}

	tooDeep := glrNodeEquivCacheMaxDepth + 1
	if hit, ok := lookupNodeEquivCache(&scratch, a, b, tooDeep); ok || hit {
		t.Fatalf("lookup above max cache depth = (%v, %v), want (false, false)", hit, ok)
	}

	storeNodeEquivCache(&scratch, a, b, tooDeep, false)
	if hit, ok := lookupNodeEquivCache(&scratch, a, b, glrNodeEquivCacheMaxDepth); !ok || !hit {
		t.Fatalf("out-of-range store changed max-depth entry: (%v, %v), want (true, true)", hit, ok)
	}
}

func TestNodeEquivCacheZeroVersionInvalidatesAfterBump(t *testing.T) {
	var scratch glrMergeScratch
	scratch.beginEquivEpoch()

	a := &Node{symbol: 1}
	b := &Node{symbol: 1}
	storeNodeEquivCache(&scratch, a, b, 0, true)
	if hit, ok := lookupNodeEquivCache(&scratch, a, b, 0); !ok || !hit {
		t.Fatalf("lookup with zero versions = (%v, %v), want (true, true)", hit, ok)
	}

	nodeBumpEquivVersion(a)
	if hit, ok := lookupNodeEquivCache(&scratch, a, b, 0); ok || hit {
		t.Fatalf("lookup after version bump = (%v, %v), want (false, false)", hit, ok)
	}
}

func TestGSSStackEquivCacheSymmetricAndEpochScoped(t *testing.T) {
	var scratch glrMergeScratch
	scratch.beginEquivEpoch()
	var gss gssScratch
	a := buildGSSStack([]stackEntry{{state: 1}, {state: 2}}, &gss)
	b := buildGSSStack([]stackEntry{{state: 1}, {state: 2}}, &gss)

	storeGSSStackEquivCache(&scratch, a.head, b.head, true)
	if hit, ok := lookupGSSStackEquivCache(&scratch, b.head, a.head); !ok || !hit {
		t.Fatalf("reversed lookup = (%v, %v), want (true, true)", hit, ok)
	}

	scratch.beginEquivEpoch()
	if hit, ok := lookupGSSStackEquivCache(&scratch, a.head, b.head); ok || hit {
		t.Fatalf("lookup after epoch bump = (%v, %v), want (false, false)", hit, ok)
	}
}

func TestGSSStacksEqualStoresStackEquivCache(t *testing.T) {
	var merge glrMergeScratch
	merge.beginEquivEpoch()
	var gss gssScratch
	arena := newNodeArena(arenaClassFull)

	aEntries := []stackEntry{{state: 1}}
	bEntries := []stackEntry{{state: 1}}
	for i := 0; i < 12; i++ {
		aNode := newLeafNodeInArena(arena, Symbol(10+i), true, uint32(i), uint32(i+1), Point{Column: uint32(i)}, Point{Column: uint32(i + 1)})
		bNode := newLeafNodeInArena(arena, Symbol(10+i), true, uint32(i), uint32(i+1), Point{Column: uint32(i)}, Point{Column: uint32(i + 1)})
		aEntries = append(aEntries, newStackEntryNode(StateID(i+2), aNode))
		bEntries = append(bEntries, newStackEntryNode(StateID(i+2), bNode))
	}
	a := buildGSSStack(aEntries, &gss)
	b := buildGSSStack(bEntries, &gss)
	if !gssStacksEqualForLanguageWithScratch(&merge, nil, a, b) {
		t.Fatal("gssStacksEqualForLanguageWithScratch = false, want true")
	}
	if hit, ok := lookupGSSStackEquivCache(&merge, a.head, b.head); !ok || !hit {
		t.Fatalf("cached stack equivalence = (%v, %v), want (true, true)", hit, ok)
	}
}

func TestPerlFrontierMergeHashPreservesGenericEquivalentStacks(t *testing.T) {
	var merge glrMergeScratch
	merge.beginEquivEpoch()
	merge.frontierMergeHash = true
	lang := &Language{Name: "perl"}

	a := perlFrontierHashTestStack(perlFrontierHashTestNode(30))
	b := perlFrontierHashTestStack(perlFrontierHashTestNode(30))

	hashA := stackHashForMerge(&merge, lang, &a)
	hashB := stackHashForMerge(&merge, lang, &b)
	if hashA != hashB {
		t.Fatalf("Perl frontier merge hashes differ for equivalent stacks: %x != %x", hashA, hashB)
	}
	if !stackEquivalentForLanguageWithScratch(&merge, lang, &a, &b) {
		t.Fatal("Perl frontier hash test stacks are not generically equivalent")
	}
}

func TestPerlFrontierMergeHashRejectsDeepDivergentStacks(t *testing.T) {
	var merge glrMergeScratch
	merge.beginEquivEpoch()
	merge.frontierMergeHash = true
	lang := &Language{Name: "perl"}

	a := perlFrontierHashTestStack(perlFrontierHashTestNode(30))
	b := perlFrontierHashTestStack(perlFrontierHashTestNode(31))

	if stackEquivalentForLanguageWithScratch(&merge, lang, &a, &b) {
		t.Fatal("Perl frontier hash test stacks are unexpectedly equivalent")
	}
	hashA := stackHashForMerge(&merge, lang, &a)
	hashB := stackHashForMerge(&merge, lang, &b)
	if hashA == hashB {
		t.Fatalf("Perl frontier merge hashes matched for divergent stacks: %x", hashA)
	}
	if _, ok := lookupStackFrontierHashCache(&merge, a.gss.head); !ok {
		t.Fatal("Perl frontier hash for stack A was not cached")
	}
	if _, ok := lookupStackFrontierHashCache(&merge, b.gss.head); !ok {
		t.Fatal("Perl frontier hash for stack B was not cached")
	}
}

func perlFrontierHashTestStack(root *Node) glrStack {
	var gss gssScratch
	stack := glrStack{
		gss:        buildGSSStack([]stackEntry{{state: 1}, newStackEntryNode(27, root)}, &gss),
		byteOffset: root.endByte,
	}
	return stack
}

func perlFrontierHashTestNode(grandchild Symbol) *Node {
	leaf := &Node{
		symbol:       grandchild,
		startByte:    2,
		endByte:      3,
		flags:        nodeFlagNamed,
		parseState:   6,
		productionID: 7,
	}
	child := &Node{
		children:     []*Node{leaf},
		symbol:       20,
		startByte:    1,
		endByte:      4,
		flags:        nodeFlagNamed,
		parseState:   4,
		productionID: 5,
	}
	return &Node{
		children:     []*Node{child},
		symbol:       10,
		startByte:    0,
		endByte:      5,
		flags:        nodeFlagNamed,
		parseState:   2,
		productionID: 3,
	}
}

func TestPythonShallowEquivalentMatchesFrontierDepthZero(t *testing.T) {
	cases := []struct {
		name string
		a    *Node
		b    *Node
	}{
		{
			name: "same immediate child",
			a: &Node{
				symbol:        10,
				startByte:     0,
				endByte:       5,
				flags:         nodeFlagNamed,
				parseState:    1,
				preGotoState:  2,
				productionID:  3,
				fieldMetadata: &nodeFieldMetadata{ids: []FieldID{4}},
				children: []*Node{{
					symbol:        20,
					startByte:     0,
					endByte:       5,
					flags:         nodeFlagNamed,
					fieldMetadata: &nodeFieldMetadata{ids: []FieldID{6}},
				}},
			},
			b: &Node{
				symbol:        10,
				startByte:     0,
				endByte:       5,
				flags:         nodeFlagNamed,
				parseState:    1,
				preGotoState:  2,
				productionID:  3,
				fieldMetadata: &nodeFieldMetadata{ids: []FieldID{4}},
				children: []*Node{{
					symbol:        20,
					startByte:     0,
					endByte:       5,
					flags:         nodeFlagNamed,
					fieldMetadata: &nodeFieldMetadata{ids: []FieldID{6}},
				}},
			},
		},
		{
			name: "parent field mismatch",
			a:    &Node{symbol: 10, startByte: 0, endByte: 5, fieldMetadata: &nodeFieldMetadata{ids: []FieldID{4}}},
			b:    &Node{symbol: 10, startByte: 0, endByte: 5, fieldMetadata: &nodeFieldMetadata{ids: []FieldID{5}}},
		},
		{
			name: "child symbol mismatch",
			a:    &Node{symbol: 10, startByte: 0, endByte: 5, children: []*Node{{symbol: 20, startByte: 0, endByte: 5}}},
			b:    &Node{symbol: 10, startByte: 0, endByte: 5, children: []*Node{{symbol: 21, startByte: 0, endByte: 5}}},
		},
		{
			name: "depth zero ignores grandchild content",
			a: &Node{symbol: 10, startByte: 0, endByte: 5, children: []*Node{{
				symbol: 20, startByte: 0, endByte: 5, children: []*Node{{symbol: 30}},
			}}},
			b: &Node{symbol: 10, startByte: 0, endByte: 5, children: []*Node{{
				symbol: 20, startByte: 0, endByte: 5, children: []*Node{{symbol: 31}},
			}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var scratch glrMergeScratch
			scratch.beginEquivEpoch()
			got := stackEntryNodesEquivalentPythonShallow(tc.a, tc.b)
			want := stackEntryNodesEquivalentFrontierWithScratch(&scratch, tc.a, tc.b, 0)
			if got != want {
				t.Fatalf("python shallow equivalence = %v, want %v", got, want)
			}
		})
	}
}

func TestMergeStacksSameTopState(t *testing.T) {
	s1 := newGLRStack(StateID(5))
	s1.score = 10
	s2 := newGLRStack(StateID(5))
	s2.score = 20

	result := mergeStacks([]glrStack{s1, s2})
	if len(result) != 1 {
		t.Fatalf("expected 1 merged stack, got %d", len(result))
	}
	if result[0].score != 20 {
		t.Errorf("expected higher-scoring stack (score 20), got %d", result[0].score)
	}
}

func TestMergeStacksSameStateDifferentByteOffset(t *testing.T) {
	s1 := newGLRStack(StateID(5))
	s1.push(5, NewLeafNode(1, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	s2 := newGLRStack(StateID(5))
	s2.push(5, NewLeafNode(1, true, 0, 7, Point{}, Point{Column: 7}), nil, nil)

	result := mergeStacks([]glrStack{s1, s2})
	if len(result) != 2 {
		t.Fatalf("expected 2 stacks (distinct byte offsets), got %d", len(result))
	}
}

func TestMergeStacksSameStateDifferentEntries(t *testing.T) {
	s1 := newGLRStack(StateID(5))
	s1.push(5, NewLeafNode(1, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	s2 := newGLRStack(StateID(5))
	s2.push(5, NewLeafNode(2, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	result := mergeStacks([]glrStack{s1, s2})
	if len(result) != 2 {
		t.Fatalf("expected 2 stacks (distinct parse paths), got %d", len(result))
	}
}

func TestMergeStacksSmallPathKeepsBestDuplicateAndDistinctKeys(t *testing.T) {
	s1 := newGLRStack(StateID(5))
	s1.score = 10
	s1.push(5, NewLeafNode(1, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	s2 := newGLRStack(StateID(5))
	s2.score = 20
	s2.push(5, NewLeafNode(1, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	s3 := newGLRStack(StateID(5))
	s3.score = 15
	s3.push(5, NewLeafNode(1, true, 0, 7, Point{}, Point{Column: 7}), nil, nil)

	s4 := newGLRStack(StateID(6))
	s4.score = 5

	result := mergeStacks([]glrStack{s1, s2, s3, s4})
	if len(result) != 3 {
		t.Fatalf("expected 3 stacks after small-path merge, got %d", len(result))
	}

	foundBestDuplicate := false
	foundOffsetSeven := false
	foundStateSix := false
	for i := range result {
		top := result[i].top()
		switch {
		case top.state == 5 && result[i].byteOffset == 3:
			if result[i].score != 20 {
				t.Fatalf("best duplicate score = %d, want 20", result[i].score)
			}
			foundBestDuplicate = true
		case top.state == 5 && result[i].byteOffset == 7:
			foundOffsetSeven = true
		case top.state == 6:
			foundStateSix = true
		}
	}
	if !foundBestDuplicate || !foundOffsetSeven || !foundStateSix {
		t.Fatalf("missing expected survivors: duplicate=%v offset7=%v state6=%v", foundBestDuplicate, foundOffsetSeven, foundStateSix)
	}
}

func TestMergeStacksSmallPathKeepsDistinctDeepStructures(t *testing.T) {
	makeTop := func(grandchild Symbol) *Node {
		left := NewLeafNode(grandchild, true, 0, 2, Point{}, Point{Column: 2})
		right := NewLeafNode(13, true, 2, 5, Point{Column: 2}, Point{Column: 5})
		mid := NewParentNode(11, true, []*Node{left, right}, nil, 0)
		return NewParentNode(10, true, []*Node{mid}, nil, 0)
	}

	s1 := newGLRStack(StateID(5))
	s1.push(5, makeTop(12), nil, nil)

	s2 := newGLRStack(StateID(5))
	s2.push(5, makeTop(14), nil, nil)

	result := mergeStacks([]glrStack{s1, s2})
	if len(result) != 2 {
		t.Fatalf("expected 2 stacks for distinct deep structures, got %d", len(result))
	}
}

func TestMergeStacksSmallPathCapOnePrunesStrongRankMismatch(t *testing.T) {
	s1 := newGLRStack(StateID(5))
	s1.score = 10
	s1.push(5, NewLeafNode(1, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	s2 := newGLRStack(StateID(5))
	s2.score = 20
	s2.push(5, NewLeafNode(2, true, 0, 3, Point{}, Point{Column: 3}), nil, nil)

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch([]glrStack{s1, s2}, &scratch)
	if len(result) != 1 {
		t.Fatalf("expected one capped survivor, got %d", len(result))
	}
	if result[0].score != 20 {
		t.Fatalf("capped survivor score = %d, want 20", result[0].score)
	}
	if stackEntryNode(result[0].top()).Symbol() != 2 {
		t.Fatalf("capped survivor symbol = %d, want 2", stackEntryNode(result[0].top()).Symbol())
	}
}

func TestMergeStacksCapOnePreservesDifferentCRecoveryCosts(t *testing.T) {
	ordinaryNode := NewLeafNode(11, true, 0, 5, Point{}, Point{Column: 5})
	missingNode := NewLeafNode(11, true, 5, 5, Point{Column: 5}, Point{Column: 5})
	missingNode.setMissing(true)
	missingNode.setHasError(true)

	ordinary := newGLRStack(StateID(1))
	ordinary.push(7, ordinaryNode, nil, nil)
	missing := newGLRStack(StateID(1))
	missing.push(7, missingNode, nil, nil)
	missing.cRecoverMissingGroup = &cRecGroup{}

	if oc, mc := cStackErrorCostForMerge(nil, &ordinary), cStackErrorCostForMerge(nil, &missing); oc == mc {
		t.Fatalf("test setup costs equal: ordinary=%d missing=%d", oc, mc)
	}

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.cRecoveryCost = true
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch([]glrStack{ordinary, missing}, &scratch)
	if len(result) != 2 {
		t.Fatalf("C recovery cost-distinct same-header survivor count = %d, want 2", len(result))
	}
}

func TestMergeStacksCRecoveryCostBlocksGSSMerge(t *testing.T) {
	var gssScratch gssScratch
	node := NewLeafNode(11, true, 0, 5, Point{}, Point{Column: 5})
	entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
	clean := glrStack{
		gss:        buildGSSStack(entries, &gssScratch),
		byteOffset: 5,
	}
	paused := glrStack{
		gss:        buildGSSStack(entries, &gssScratch),
		byteOffset: 5,
		cPaused:    true,
	}
	if cleanCost, pausedCost := cStackErrorCostForMerge(nil, &clean), cStackErrorCostForMerge(nil, &paused); cleanCost == pausedCost {
		t.Fatalf("test setup costs equal: clean=%d paused=%d", cleanCost, pausedCost)
	}

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.cRecoveryCost = true
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch([]glrStack{clean, paused}, &scratch)
	if len(result) != 2 {
		t.Fatalf("GSS cost-distinct survivor count = %d, want 2", len(result))
	}
	for i := range result {
		if got := result[i].gss.head.linkCount(); got != 1 {
			t.Fatalf("result[%d] GSS link count = %d, want 1 (no incompatible GSS merge)", i, got)
		}
	}
}

func TestCRecoveryMergeCostComparisonReusesParserScratch(t *testing.T) {
	var gssScratch gssScratch
	node := NewLeafNode(11, true, 0, 5, Point{}, Point{Column: 5})
	entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
	clean := glrStack{
		gss:        buildGSSStack(entries, &gssScratch),
		byteOffset: 5,
	}
	paused := clean
	paused.cPaused = true

	var mergeScratch glrMergeScratch
	mergeScratch.beginEquivEpoch()
	mergeScratch.ensureMergeHotCaches()
	parser := &Parser{
		language:                         &Language{},
		errorCostCompetition:             true,
		crecoveryCostCompetitionRelevant: true,
		mergeScratch:                     &mergeScratch,
	}
	if !cRecoveryMergeCostsDifferForParser(parser, &clean, &paused) {
		t.Fatal("cost comparison treated clean and paused stacks as equal")
	}

	allocs := testing.AllocsPerRun(100, func() {
		if !cRecoveryMergeCostsDifferForParser(parser, &clean, &paused) {
			panic("cost comparison changed after warmup")
		}
	})
	if allocs != 0 {
		t.Fatalf("warm recovery cost comparison allocations = %g, want 0", allocs)
	}
}

func TestCNodeErrorCostScratchTracksEquivVersion(t *testing.T) {
	lang := &Language{SymbolMetadata: make([]SymbolMetadata, 12)}
	lang.SymbolMetadata[11].Visible = true

	skipped := NewLeafNode(11, true, 0, 1, Point{}, Point{Column: 1})
	errNode := NewParentNode(errorSymbol, false, []*Node{skipped}, nil, 0)

	var scratch glrMergeScratch
	first := cNodeErrorCostLangWithScratch(&scratch, lang, errNode)
	skipped.setExtra(true)
	nodeBumpEquivVersion(errNode)
	second := cNodeErrorCostLangWithScratch(&scratch, lang, errNode)
	want := cNodeErrorCostLang(lang, errNode)

	if second != want {
		t.Fatalf("scratch cost after version bump = %d, want %d", second, want)
	}
	if second >= first {
		t.Fatalf("scratch cost did not reflect visible child removal: before=%d after=%d", first, second)
	}
}

func TestCNodeErrorCostScratchSharesActiveParserMemo(t *testing.T) {
	lang := &Language{SymbolMetadata: make([]SymbolMetadata, 12)}
	lang.SymbolMetadata[11].Visible = true

	skipped := NewLeafNode(11, true, 0, 1, Point{}, Point{Column: 1})
	errNode := NewParentNode(errorSymbol, false, []*Node{skipped}, nil, 0)
	parser := &Parser{
		language:       lang,
		cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheInitialSize),
	}
	parser.beginCNodeMemoEpoch()
	scratch := glrMergeScratch{language: lang, cErrorCostParser: parser}

	first := cNodeErrorCostLangWithScratch(&scratch, lang, errNode)
	if len(scratch.cErrorCost) != 0 {
		t.Fatalf("merge fallback memo entries = %d, want 0", len(scratch.cErrorCost))
	}
	if got := parser.cNodeErrorCost(errNode); got != first {
		t.Fatalf("parser memo cost = %d, want %d", got, first)
	}

	skipped.setExtra(true)
	nodeBumpEquivVersion(errNode)
	second := cNodeErrorCostLangWithScratch(&scratch, lang, errNode)
	if want := cNodeErrorCostLang(lang, errNode); second != want {
		t.Fatalf("shared cost after version bump = %d, want %d", second, want)
	}
	if second >= first {
		t.Fatalf("shared cost did not reflect visible child removal: before=%d after=%d", first, second)
	}
}

func TestActivateCNodeMemoMergeSharingDropsFallbackMap(t *testing.T) {
	lang := &Language{SymbolMetadata: make([]SymbolMetadata, 2)}
	node := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})
	scratch := glrMergeScratch{
		language: lang,
		cErrorCost: map[*Node]glrCErrorCostEntry{
			node: {cost: 9},
		},
	}
	parser := &Parser{language: lang, mergeScratch: &scratch}
	parser.activateCNodeMemoMergeSharing()

	if scratch.cErrorCostParser != parser {
		t.Fatal("merge scratch did not activate the parser memo")
	}
	if scratch.cErrorCost != nil {
		t.Fatal("merge scratch retained the fallback map after activation")
	}
}

func TestTryGSSMainMergeResultClearsMaterializingCache(t *testing.T) {
	var gssScratch gssScratch
	node := NewLeafNode(11, true, 0, 5, Point{}, Point{Column: 5})
	entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
	left := glrStack{
		gss:        buildGSSStack(entries, &gssScratch),
		byteOffset: 5,
	}
	right := glrStack{
		gss:        buildGSSStack(entries, &gssScratch),
		byteOffset: 5,
	}

	var scratch glrMergeScratch
	scratch.beginEquivEpoch()
	scratch.ensureMergeHotCaches()
	if gssStacksHaveDistinctMaterializingShapesWithScratch(&scratch, &left, &right) {
		t.Fatalf("test setup produced distinct materializing shapes")
	}
	if len(scratch.shapePrefixCache) == 0 {
		t.Fatalf("test setup did not populate materializing shape prefix cache")
	}
	epochBefore := scratch.shapePrefixEpoch
	if _, hit := lookupShapePrefixCache(&scratch, left.gss.head); !hit {
		t.Fatalf("test setup did not cache the left head's shape prefix")
	}

	result := []glrStack{left}
	merged, attempted := tryGSSMainMergeResult(&scratch, result, 0, &right)
	if !attempted || !merged {
		t.Fatalf("GSS merge attempted=%v merged=%v, want true/true", attempted, merged)
	}
	if scratch.shapePrefixEpoch == epochBefore {
		t.Fatalf("shape prefix epoch not bumped after successful merge (still %d): stale spine prefixes would survive link rewrites", epochBefore)
	}
	if _, hit := lookupShapePrefixCache(&scratch, left.gss.head); hit {
		t.Fatalf("stale shape prefix still readable after successful merge")
	}
}

func TestCertifiedMaterializingShapeHashIncludesRawDescendants(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	parser := &Parser{}
	makeMethodType := func(leafSymbol Symbol) *Node {
		leaf := NewLeafNode(leafSymbol, true, 1, 4, Point{Column: 1}, Point{Column: 4})
		inner := NewParentNode(356, true, []*Node{leaf}, nil, 0)
		inner.parseState = 3814
		inner.rawShape = parser.captureRawShape(
			nil,
			arena,
			inner.symbol,
			inner.productionID,
			[]stackEntry{newStackEntryNode(inner.parseState, leaf)},
			0,
			1,
		)
		outer := NewParentNode(469, true, []*Node{inner}, nil, 0)
		outer.parseState = 4827
		outer.rawShape = parser.captureRawShape(
			nil,
			arena,
			outer.symbol,
			outer.productionID,
			[]stackEntry{newStackEntryNode(outer.parseState, inner)},
			0,
			1,
		)
		return outer
	}

	leftEntry := newStackEntryNode(4827, makeMethodType(1))
	rightEntry := newStackEntryNode(4827, makeMethodType(587))
	if leftHash, rightHash := gssEntryHash(gssHashSeed, leftEntry), gssEntryHash(gssHashSeed, rightEntry); leftHash != rightHash {
		t.Fatalf("test setup shallow hashes differ: left=%d right=%d", leftHash, rightHash)
	}

	var gssScratch gssScratch
	left := glrStack{gss: buildGSSStack([]stackEntry{{state: 1}, leftEntry}, &gssScratch), byteOffset: 4}
	right := glrStack{gss: buildGSSStack([]stackEntry{{state: 1}, rightEntry}, &gssScratch), byteOffset: 4}
	scratch := glrMergeScratch{
		arena: arena,
		language: &Language{
			ExactStackNodeEquivalenceCertified: true,
		},
	}
	scratch.beginEquivEpoch()
	scratch.ensureMergeHotCaches()
	if !gssStacksHaveDistinctMaterializingShapesWithScratch(&scratch, &left, &right) {
		t.Fatal("certified materializing shape hash merged alias-distinct descendants")
	}
}

func TestTryGSSMainMergeResultRejectsDistinctScoreBeforeMutation(t *testing.T) {
	var gssScratch gssScratch
	node := NewLeafNode(11, true, 0, 5, Point{}, Point{Column: 5})
	entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
	left := glrStack{
		gss:        buildGSSStack(entries, &gssScratch),
		byteOffset: 5,
		score:      1,
	}
	right := glrStack{
		gss:        buildGSSStack(entries, &gssScratch),
		byteOffset: 5,
		score:      2,
		cPaused:    true,
	}
	if !stacksHeaderEquivalent(&left, &right) {
		t.Fatal("test setup did not produce same-header stacks")
	}
	if leftCost, rightCost := cStackErrorCostForMerge(nil, &left), cStackErrorCostForMerge(nil, &right); leftCost == rightCost {
		t.Fatalf("test setup recovery costs equal: left=%d right=%d", leftCost, rightCost)
	}
	leftHead := left.gss.head
	leftLinks := leftHead.linkCount()

	scratch := glrMergeScratch{cRecoveryCost: true}
	scratch.beginEquivEpoch()
	result := []glrStack{left}
	merged, attempted := tryGSSMainMergeResult(&scratch, result, 0, &right)
	if merged || attempted {
		t.Fatalf("distinct-score GSS merge = merged:%v attempted:%v, want false/false", merged, attempted)
	}
	if result[0].gss.head != leftHead || leftHead.linkCount() != leftLinks {
		t.Fatal("distinct-score GSS prefilter mutated the retained stack")
	}
}

func TestTryGSSMainMergeForParserBumpsShapePrefixEpoch(t *testing.T) {
	// Cross-token invalidation guard for the DISPATCH-time GSS main merge
	// (reached through tryGSSMainMergeForParser from parser_reduce /
	// parser_recover_c). Like the collapse-phase tryGSSMainMergeResult, a
	// successful merge rewrites link 0 of surviving nodes (setGSSMainLink), so
	// cached root->head shape prefixes in the active merge scratch must be
	// invalidated or the materializing-shape gate compares stale hashes on the
	// next token.
	var gssScratch gssScratch
	node := NewLeafNode(11, true, 0, 5, Point{}, Point{Column: 5})
	entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
	left := glrStack{
		gss:        buildGSSStack(entries, &gssScratch),
		byteOffset: 5,
	}
	right := glrStack{
		gss:        buildGSSStack(entries, &gssScratch),
		byteOffset: 5,
	}

	var scratch glrMergeScratch
	scratch.beginEquivEpoch()
	scratch.ensureMergeHotCaches()
	if gssStacksHaveDistinctMaterializingShapesWithScratch(&scratch, &left, &right) {
		t.Fatalf("test setup produced distinct materializing shapes")
	}
	epochBefore := scratch.shapePrefixEpoch
	if _, hit := lookupShapePrefixCache(&scratch, left.gss.head); !hit {
		t.Fatalf("test setup did not cache the left head's shape prefix")
	}

	// tryGSSMainMergeForParser only carries *Parser; the fix reaches the active
	// scratch through p.mergeScratch, exactly as parseInternal wires it.
	p := &Parser{mergeScratch: &scratch}
	if !tryGSSMainMergeForParser(p, &left, &right) {
		t.Fatalf("dispatch-time GSS main merge did not merge")
	}
	if scratch.shapePrefixEpoch == epochBefore {
		t.Fatalf("shape prefix epoch not bumped after dispatch-time merge (still %d): stale spine prefixes would survive link rewrites", epochBefore)
	}
	if _, hit := lookupShapePrefixCache(&scratch, left.gss.head); hit {
		t.Fatalf("stale shape prefix still readable after dispatch-time merge")
	}
}

func TestRetargetStackEntryPayloadParseStateFeedsShapePrefixCache(t *testing.T) {
	// Locks the invariant behind the retargetStackEntryPayload reduce-site epoch
	// bumps: parseState feeds gssEntryHash, which the root->head shape prefix
	// rolls in, so the pointer-keyed prefix cache keeps serving the pre-retarget
	// hash until the epoch is bumped. Without the reduce-site bump the
	// materializing-shape gate would read the stale prefix on the next token.
	var gssScratch gssScratch
	node := NewLeafNode(11, true, 0, 5, Point{}, Point{Column: 5})
	node.parseState = 7
	entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
	stack := glrStack{
		gss:        buildGSSStack(entries, &gssScratch),
		byteOffset: 5,
	}

	var scratch glrMergeScratch
	scratch.beginEquivEpoch()
	scratch.ensureMergeHotCaches()

	hashBefore, ok := stackMaterializingShapeHashWithScratch(&scratch, &stack)
	if !ok {
		t.Fatalf("could not compute initial materializing shape hash")
	}
	head := stack.gss.head
	if _, hit := lookupShapePrefixCache(&scratch, head); !hit {
		t.Fatalf("test setup did not cache the head's shape prefix")
	}

	// Retarget the head payload in place, exactly as the reduce sites do.
	got, ok := retargetStackEntryPayload(head.entry, 99)
	if !ok {
		t.Fatalf("retarget of head payload failed")
	}
	head.entry = got
	if node.parseState != 99 {
		t.Fatalf("retarget did not rewrite the shared node's parseState: got %d", node.parseState)
	}

	// Same epoch: the cache still serves the pre-retarget hash.
	if staleHash, ok := stackMaterializingShapeHashWithScratch(&scratch, &stack); !ok || staleHash != hashBefore {
		t.Fatalf("expected stale cached hash %d before invalidation, got %d (ok=%v)", hashBefore, staleHash, ok)
	}

	// The reduce-site fix bumps the epoch; the gate must then observe the fresh
	// parseState.
	scratch.bumpShapePrefixEpoch()
	freshHash, ok := stackMaterializingShapeHashWithScratch(&scratch, &stack)
	if !ok {
		t.Fatalf("could not recompute materializing shape hash")
	}
	if freshHash == hashBefore {
		t.Fatalf("shape hash unchanged after parseState retarget + epoch bump (%d): stale prefix survived", hashBefore)
	}
}

func TestMergeStacksSmallCapOneScansLaterCRecoveryCostClass(t *testing.T) {
	makeStack := func(node *Node, score int) glrStack {
		s := newGLRStack(StateID(1))
		s.push(7, node, nil, nil)
		s.score = score
		return s
	}
	makeMissing := func(score int) glrStack {
		n := NewLeafNode(11, true, 5, 5, Point{Column: 5}, Point{Column: 5})
		n.setMissing(true)
		n.setHasError(true)
		s := makeStack(n, score)
		s.cRecoverMissingGroup = &cRecGroup{}
		return s
	}
	ordinary := makeStack(NewLeafNode(11, true, 0, 5, Point{}, Point{Column: 5}), 0)
	missingLow := makeMissing(1)
	missingHigh := makeMissing(2)

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.cRecoveryCost = true
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch([]glrStack{ordinary, missingLow, missingHigh}, &scratch)
	if len(result) != 2 {
		t.Fatalf("small path C cost-class survivor count = %d, want 2", len(result))
	}
	missingCost := cStackErrorCostForMerge(nil, &missingLow)
	missingCount := 0
	for i := range result {
		if cStackErrorCostForMerge(nil, &result[i]) != missingCost {
			continue
		}
		missingCount++
		if result[i].score != missingHigh.score {
			t.Fatalf("small path same-cost survivor score = %d, want %d", result[i].score, missingHigh.score)
		}
	}
	if missingCount != 1 {
		t.Fatalf("small path missing cost-class survivor count = %d, want 1", missingCount)
	}
}

func TestMergeStacksSlotCapOnePreservesCRecoveryCostClasses(t *testing.T) {
	makeStack := func(state StateID, node *Node) glrStack {
		s := newGLRStack(StateID(1))
		s.push(state, node, nil, nil)
		return s
	}
	makeLeaf := func(sym Symbol, start, end uint32) *Node {
		return NewLeafNode(sym, true, start, end, Point{Column: start}, Point{Column: end})
	}

	ordinary := makeStack(7, makeLeaf(11, 0, 5))

	errChild := makeLeaf(12, 0, 5)
	errNode := NewParentNode(errorSymbol, true, []*Node{errChild}, nil, 0)
	errNode.setHasError(true)
	errNode.startByte = 0
	errNode.endByte = 5
	errNode.startPoint = Point{}
	errNode.endPoint = Point{Column: 5}
	errorStack := makeStack(7, errNode)

	missingNode := makeLeaf(11, 5, 5)
	missingNode.setMissing(true)
	missingNode.setHasError(true)
	missing := makeStack(7, missingNode)
	missing.cRecoverMissingGroup = &cRecGroup{}

	duplicateMissingNode := makeLeaf(11, 5, 5)
	duplicateMissingNode.setMissing(true)
	duplicateMissingNode.setHasError(true)
	duplicateMissing := makeStack(7, duplicateMissingNode)
	duplicateMissing.cRecoverMissingGroup = &cRecGroup{}

	costs := map[uint32]bool{}
	for _, s := range []glrStack{ordinary, errorStack, missing} {
		costs[cStackErrorCostForMerge(nil, &s)] = true
	}
	if len(costs) != 3 {
		t.Fatalf("test setup distinct C costs = %d, want 3", len(costs))
	}
	if cStackErrorCostForMerge(nil, &missing) != cStackErrorCostForMerge(nil, &duplicateMissing) {
		t.Fatal("duplicate missing setup does not share C error cost")
	}

	fillerA := makeStack(8, makeLeaf(21, 10, 15))
	fillerB := makeStack(9, makeLeaf(22, 20, 25))

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.cRecoveryCost = true
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch([]glrStack{
		ordinary,
		errorStack,
		missing,
		duplicateMissing,
		fillerA,
		fillerB,
	}, &scratch)

	sameHeaderCosts := map[uint32]int{}
	for i := range result {
		if result[i].top().state == 7 && result[i].byteOffset == 5 {
			sameHeaderCosts[cStackErrorCostForMerge(nil, &result[i])]++
		}
	}
	if len(sameHeaderCosts) != 3 {
		t.Fatalf("same-header C cost classes = %d (%v), want 3", len(sameHeaderCosts), sameHeaderCosts)
	}
	for cost, count := range sameHeaderCosts {
		if count != 1 {
			t.Fatalf("C cost class %d survivor count = %d, want 1", cost, count)
		}
	}
	if len(result) != 5 {
		t.Fatalf("result count = %d, want 5 (three cost classes plus two fillers)", len(result))
	}

	key := glrMergeKey{state: 7, byteOffset: 5}
	foundSlot := false
	for i := range scratch.slots {
		if scratch.slots[i].key != key {
			continue
		}
		foundSlot = true
		if got := mergeSlotTrackedCount(&scratch.slots[i]); got != 3 {
			t.Fatalf("tracked same-header slot count = %d, want 3", got)
		}
	}
	if !foundSlot {
		t.Fatal("did not find same-header merge slot")
	}
}

func TestMergeStacksSlotCapOneTracksOverflowCRecoveryCostClasses(t *testing.T) {
	makeStack := func(node *Node) glrStack {
		s := newGLRStack(StateID(1))
		s.push(7, node, nil, nil)
		return s
	}
	makeErrorNode := func(start uint32) *Node {
		child := NewLeafNode(12, true, start, 6, Point{Column: start}, Point{Column: 6})
		n := NewParentNode(errorSymbol, true, []*Node{child}, nil, 0)
		n.setHasError(true)
		n.startByte = start
		n.endByte = 6
		n.startPoint = Point{Column: start}
		n.endPoint = Point{Column: 6}
		return n
	}

	stacks := make([]glrStack, 0, maxStacksPerMergeKey+1)
	ordinary := makeStack(NewLeafNode(11, true, 0, 6, Point{}, Point{Column: 6}))
	stacks = append(stacks, ordinary)
	for i := 0; i < maxStacksPerMergeKey; i++ {
		stacks = append(stacks, makeStack(makeErrorNode(uint32(i))))
	}

	costs := map[uint32]bool{}
	for i := range stacks {
		costs[cStackErrorCostForMerge(nil, &stacks[i])] = true
	}
	if len(costs) != len(stacks) {
		t.Fatalf("test setup distinct C costs = %d, want %d", len(costs), len(stacks))
	}

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.cRecoveryCost = true
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch(stacks, &scratch)
	if len(result) != len(stacks) {
		t.Fatalf("overflow C cost-class survivor count = %d, want %d", len(result), len(stacks))
	}

	key := glrMergeKey{state: 7, byteOffset: 6}
	foundSlot := false
	for i := range scratch.slots {
		if scratch.slots[i].key != key {
			continue
		}
		foundSlot = true
		if got := mergeSlotTrackedCount(&scratch.slots[i]); got != len(stacks) {
			t.Fatalf("tracked overflow slot count = %d, want %d", got, len(stacks))
		}
		if len(scratch.slots[i].extraIndices) == 0 {
			t.Fatal("expected overflow indices to track survivors beyond fixed slot array")
		}
	}
	if !foundSlot {
		t.Fatal("did not find overflow merge slot")
	}
}

func TestMergeStacksLargeCapPreservesDifferentCRecoveryCosts(t *testing.T) {
	var gssScratch gssScratch
	build := func(state StateID, sym Symbol, start uint32, paused bool) glrStack {
		node := NewLeafNode(sym, true, start, start+5, Point{Column: start}, Point{Column: start + 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(state, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &gssScratch),
			byteOffset: stackByteOffset(entries),
			cPaused:    paused,
		}
	}
	clean := build(7, 11, 0, false)
	paused := build(7, 12, 0, true)
	if cleanCost, pausedCost := cStackErrorCostForMerge(nil, &clean), cStackErrorCostForMerge(nil, &paused); cleanCost == pausedCost {
		t.Fatalf("test setup costs equal: clean=%d paused=%d", cleanCost, pausedCost)
	}
	stacks := []glrStack{
		clean,
		paused,
		build(8, 13, 6, false),
		build(9, 14, 12, false),
		build(10, 15, 18, false),
	}

	var scratch glrMergeScratch
	scratch.perKeyCap = maxStacksPerMergeKey + 1
	scratch.cRecoveryCost = true
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch(stacks, &scratch)
	if len(result) != len(stacks) {
		t.Fatalf("large-cap survivor count = %d, want %d", len(result), len(stacks))
	}
	sameKeyCount := 0
	for i := range result {
		if result[i].top().state != 7 || result[i].byteOffset != 5 {
			continue
		}
		sameKeyCount++
		if got := result[i].gss.head.linkCount(); got != 1 {
			t.Fatalf("same-key result[%d] link count = %d, want 1", i, got)
		}
	}
	if sameKeyCount != 2 {
		t.Fatalf("large-cap same-key survivor count = %d, want 2", sameKeyCount)
	}
}

func TestMergeStacksSmallFaithfulGSSUnionPreservesExtraLinks(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = false
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	tests := []struct {
		name    string
		scratch glrMergeScratch
	}{
		{
			name: "certified full parse",
			scratch: glrMergeScratch{
				faithfulCapOne: true,
			},
		},
		{
			name: "recovery cost became relevant",
			scratch: glrMergeScratch{
				recoveryCapOneConvergence: true,
				cRecoveryCost:             true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gssScratch gssScratch
			buildStack := func(sym Symbol) glrStack {
				node := NewLeafNode(sym, true, 0, 5, Point{}, Point{Column: 5})
				entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
				return glrStack{
					gss:        buildGSSStack(entries, &gssScratch),
					byteOffset: stackByteOffset(entries),
				}
			}

			scratch := test.scratch
			scratch.perKeyCap = 1
			scratch.beginEquivEpoch()

			result := mergeStacksWithScratch([]glrStack{buildStack(11), buildStack(12)}, &scratch)
			if len(result) != 1 {
				t.Fatalf("merged stack count = %d, want 1", len(result))
			}
			if got := result[0].gss.head.linkCount(); got != 2 {
				t.Fatalf("merged link count = %d, want 2", got)
			}
			_, extra := result[0].gss.head.link(1)
			if got := stackEntryNodeSymbol(extra); got != 12 {
				t.Fatalf("extra link symbol = %d, want 12", got)
			}
		})
	}
}

func TestFaithfulCapOneMergeEnabledByParsePolicy(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = false
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	if faithfulCapOneMergeEnabled(&glrMergeScratch{}) {
		t.Fatal("inactive parse enabled faithful cap-one merge")
	}
	scratch := &glrMergeScratch{
		faithfulCapOne: true,
	}
	if !faithfulCapOneMergeEnabled(scratch) {
		t.Fatal("active parse did not enable faithful cap-one merge")
	}
	recoveryScratch := &glrMergeScratch{
		recoveryCapOneConvergence: true,
	}
	if faithfulCapOneMergeEnabled(recoveryScratch) {
		t.Fatal("recovery policy activated before error cost became relevant")
	}
	recoveryScratch.cRecoveryCost = true
	if !faithfulCapOneMergeEnabled(recoveryScratch) {
		t.Fatal("recovery policy did not activate after error cost became relevant")
	}
}

func TestMergeStacksPreservesDistinctGSSMaterializingShapesWhenBudgetAllows(t *testing.T) {
	var gssScratch gssScratch
	buildStack := func(sym Symbol) glrStack {
		node := NewLeafNode(sym, true, 0, 5, Point{}, Point{Column: 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &gssScratch),
			byteOffset: stackByteOffset(entries),
		}
	}

	var scratch glrMergeScratch
	scratch.perKeyCap = maxStacksPerMergeKey
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch([]glrStack{buildStack(11), buildStack(12)}, &scratch)
	if len(result) != 2 {
		t.Fatalf("merged stack count = %d, want 2", len(result))
	}
	for i := range result {
		if got := result[i].gss.head.linkCount(); got != 1 {
			t.Fatalf("result[%d] link count = %d, want 1", i, got)
		}
	}
}

func TestMergeStacksPreservesDistinctGSSShapesWhenBudgetAllowsAcrossPaths(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = false
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	buildStack := func(gssScratch *gssScratch, state StateID, sym Symbol, start uint32) glrStack {
		node := NewLeafNode(sym, true, start, start+5, Point{Column: start}, Point{Column: start + 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(state, node)}
		return glrStack{
			gss:        buildGSSStack(entries, gssScratch),
			byteOffset: stackByteOffset(entries),
		}
	}
	check := func(t *testing.T, result []glrStack, wantTotal int) {
		t.Helper()
		if len(result) != wantTotal {
			t.Fatalf("merged stack count = %d, want %d", len(result), wantTotal)
		}
		sameKeyCount := 0
		for i := range result {
			if result[i].top().state == 7 && result[i].byteOffset == 5 {
				sameKeyCount++
				if got := result[i].gss.head.linkCount(); got != 1 {
					t.Fatalf("same-key survivor link count = %d, want 1", got)
				}
			}
		}
		if sameKeyCount != 2 {
			t.Fatalf("same-key survivor count = %d, want 2", sameKeyCount)
		}
	}

	t.Run("small", func(t *testing.T) {
		var gssScratch gssScratch
		var scratch glrMergeScratch
		scratch.perKeyCap = maxStacksPerMergeKey
		scratch.beginEquivEpoch()
		result := mergeStacksWithScratch([]glrStack{
			buildStack(&gssScratch, 7, 11, 0),
			buildStack(&gssScratch, 7, 12, 0),
		}, &scratch)
		check(t, result, 2)
	})

	t.Run("default-cap", func(t *testing.T) {
		var gssScratch gssScratch
		var scratch glrMergeScratch
		scratch.perKeyCap = maxStacksPerMergeKey
		scratch.beginEquivEpoch()
		result := mergeStacksWithScratch([]glrStack{
			buildStack(&gssScratch, 7, 21, 0),
			buildStack(&gssScratch, 7, 22, 0),
			buildStack(&gssScratch, 8, 23, 10),
			buildStack(&gssScratch, 9, 24, 20),
			buildStack(&gssScratch, 10, 25, 30),
		}, &scratch)
		check(t, result, 5)
	})

	t.Run("defer-exact", func(t *testing.T) {
		var gssScratch gssScratch
		var scratch glrMergeScratch
		scratch.perKeyCap = maxStacksPerMergeKey
		scratch.deferExactDedupe = true
		scratch.beginEquivEpoch()
		result := mergeStacksWithScratch([]glrStack{
			buildStack(&gssScratch, 7, 31, 0),
			buildStack(&gssScratch, 7, 32, 0),
			buildStack(&gssScratch, 8, 33, 10),
			buildStack(&gssScratch, 9, 34, 20),
			buildStack(&gssScratch, 10, 35, 30),
		}, &scratch)
		check(t, result, 5)
	})

	t.Run("large-cap", func(t *testing.T) {
		var gssScratch gssScratch
		var scratch glrMergeScratch
		scratch.perKeyCap = maxStacksPerMergeKey + 1
		scratch.beginEquivEpoch()
		result := mergeStacksWithScratch([]glrStack{
			buildStack(&gssScratch, 7, 41, 0),
			buildStack(&gssScratch, 7, 42, 0),
			buildStack(&gssScratch, 8, 43, 10),
			buildStack(&gssScratch, 9, 44, 20),
			buildStack(&gssScratch, 10, 45, 30),
		}, &scratch)
		check(t, result, 5)
	})
}

func TestMergeStacksFaithfulGSSMergeSeparatesGoScoreAndShifted(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	buildStack := func(gssScratch *gssScratch, sym Symbol) glrStack {
		node := NewLeafNode(sym, true, 0, 5, Point{}, Point{Column: 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
		return glrStack{
			gss:        buildGSSStack(entries, gssScratch),
			byteOffset: stackByteOffset(entries),
		}
	}

	t.Run("score", func(t *testing.T) {
		var gssScratch gssScratch
		low := buildStack(&gssScratch, 11)
		high := buildStack(&gssScratch, 12)
		low.score = 1
		high.score = 2

		if gssMainCanMerge(&low, &high) {
			t.Fatal("gssMainCanMerge = true for different Go scores")
		}

		var scratch glrMergeScratch
		scratch.perKeyCap = 1
		scratch.beginEquivEpoch()

		result := mergeStacksWithScratch([]glrStack{low, high}, &scratch)
		if len(result) != 1 {
			t.Fatalf("merged stack count = %d, want 1", len(result))
		}
		if got := result[0].gss.head.linkCount(); got != 1 {
			t.Fatalf("survivor link count = %d, want 1; Go score must block GSS merge", got)
		}
	})

	t.Run("shifted", func(t *testing.T) {
		var gssScratch gssScratch
		shifted := buildStack(&gssScratch, 21)
		unshifted := buildStack(&gssScratch, 22)
		shifted.shifted = true

		if gssMainCanMerge(&shifted, &unshifted) {
			t.Fatal("gssMainCanMerge = true for different shifted flags")
		}

		var scratch glrMergeScratch
		scratch.perKeyCap = 1
		scratch.beginEquivEpoch()

		result := mergeStacksWithScratch([]glrStack{shifted, unshifted}, &scratch)
		if len(result) != 1 {
			t.Fatalf("merged stack count = %d, want 1", len(result))
		}
		if got := result[0].gss.head.linkCount(); got != 1 {
			t.Fatalf("survivor link count = %d, want 1; shifted flag must block GSS merge", got)
		}
	})
}

func TestGSSMainCanMergeRejectsErrorBearingStacks(t *testing.T) {
	var gssScratch gssScratch
	buildStack := func(sym Symbol, markError bool) glrStack {
		node := NewLeafNode(sym, true, 0, 5, Point{}, Point{Column: 5})
		if markError {
			node.setHasError(true)
		}
		entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &gssScratch),
			byteOffset: stackByteOffset(entries),
		}
	}

	clean := buildStack(11, false)
	withError := buildStack(11, true)
	if gssMainCanMerge(&clean, &withError) {
		t.Fatal("gssMainCanMerge = true for error-bearing stack")
	}
}

func TestGSSCleanZeroAllLinksHandlesLongCleanChain(t *testing.T) {
	var gssScratch gssScratch
	var mergeScratch glrMergeScratch
	mergeScratch.beginEquivEpoch()

	var head *gssNode
	const chainLen = 20000
	for i := 0; i < chainLen; i++ {
		head = gssScratch.allocNode(stackEntry{state: StateID(i%17 + 1)}, head, uint32(i+1))
	}

	if !gssNodeCleanZeroErrorAllLinksWithScratch(&mergeScratch, head) {
		t.Fatal("clean-zero scan rejected a long clean GSS chain")
	}
	if !gssNodeCleanZeroErrorAllLinksWithScratch(&mergeScratch, head) {
		t.Fatal("cached clean-zero scan rejected a long clean GSS chain")
	}
}

func TestGSSCleanZeroAllLinksRejectsErrorBearingPackedPredecessor(t *testing.T) {
	var gssScratch gssScratch
	var mergeScratch glrMergeScratch
	mergeScratch.beginEquivEpoch()

	base := gssScratch.allocNode(stackEntry{state: 1}, nil, 1)
	extraPrev := gssScratch.allocNode(stackEntry{state: 9}, nil, 1)
	head := gssScratch.allocNode(newStackEntryNode(2, NewLeafNode(11, true, 0, 1, Point{}, Point{Column: 1})), base, 2)
	errorEntry := newStackEntryNode(2, NewLeafNode(99, true, 0, 1, Point{}, Point{Column: 1}))
	stackEntryNode(errorEntry).setHasError(true)
	head.appendExtraLink(gssMainLink{prev: extraPrev, entry: errorEntry})

	if gssNodeCleanZeroErrorAllLinksWithScratch(&mergeScratch, head) {
		t.Fatal("clean-zero scan accepted an error-bearing packed predecessor link")
	}
}

func TestGSSCleanZeroAllLinksStoresFailureForEveryActiveAncestor(t *testing.T) {
	var nodes gssScratch
	var scratch glrMergeScratch
	scratch.beginEquivEpoch()

	cleanSibling := nodes.allocNode(stackEntry{state: 8}, nil, 1)
	errorPrev := nodes.allocNode(stackEntry{state: 9}, nil, 1)
	errorEntry := newStackEntryNode(10, NewLeafNode(errorSymbol, true, 0, 1, Point{}, Point{Column: 1}))
	stackEntryNode(errorEntry).setHasError(true)
	bad := nodes.allocNode(stackEntry{state: 3}, nil, 1)
	bad.appendExtraLink(gssMainLink{prev: errorPrev, entry: errorEntry})
	middle := nodes.allocNode(stackEntry{state: 2}, bad, 2)
	head := nodes.allocNode(stackEntry{state: 1}, cleanSibling, 3)
	head.appendExtraLink(gssMainLink{prev: middle, entry: stackEntry{state: 2}})

	if gssNodeCleanZeroErrorAllLinksWithScratch(&scratch, head) {
		t.Fatal("clean-zero scan accepted an error-bearing ancestor path")
	}
	gen := gssPrefixAggGen.Load()
	for _, node := range []*gssNode{head, middle, bad} {
		if clean, ok := lookupCleanZeroNodeState(node, gen); !ok || clean {
			t.Fatalf("ancestor state = %v, %v; want false, true", clean, ok)
		}
	}
	if clean, ok := lookupCleanZeroNodeState(cleanSibling, gen); !ok || !clean {
		t.Fatalf("clean sibling state = %v, %v; want true, true", clean, ok)
	}
	if gssNodeCleanZeroErrorAllLinksWithScratch(&scratch, middle) {
		t.Fatal("cached ancestor failure returned clean")
	}
	if len(scratch.cleanZeroFrames) != 0 {
		t.Fatalf("clean-zero frames after cached failure = %d, want 0", len(scratch.cleanZeroFrames))
	}
}

func TestGSSCleanZeroAllLinksTerminatesOnCycle(t *testing.T) {
	a := &gssNode{entry: stackEntry{state: 1}}
	b := &gssNode{entry: stackEntry{state: 2}, prev: a}
	a.prev = b
	var scratch glrMergeScratch

	if !gssNodeCleanZeroErrorAllLinksWithScratch(&scratch, a) {
		t.Fatal("clean-zero scan rejected a clean cycle")
	}
	gen := gssPrefixAggGen.Load()
	for _, node := range []*gssNode{a, b} {
		if clean, ok := lookupCleanZeroNodeState(node, gen); !ok || !clean {
			t.Fatalf("cycle node state = %v, %v; want true, true", clean, ok)
		}
	}
}

func TestGSSCleanZeroAllLinksFreshParseProofFallsBackAfterError(t *testing.T) {
	var gssScratch gssScratch
	childErrors := false
	mergeScratch := glrMergeScratch{childErrors: &childErrors}
	mergeScratch.beginEquivEpoch()

	errorEntry := newStackEntryNode(2, NewLeafNode(errorSymbol, true, 0, 1, Point{}, Point{Column: 1}))
	stackEntryNode(errorEntry).setHasError(true)
	head := gssScratch.allocNode(errorEntry, nil, 1)

	// parseInternal only exposes the false proof before any error-bearing
	// payload exists. While it is valid, the hot path must not walk the graph.
	if !gssNodeCleanZeroErrorAllLinksWithScratch(&mergeScratch, head) {
		t.Fatal("fresh full-parse clean proof did not bypass the GSS walk")
	}
	childErrors = true
	if gssNodeCleanZeroErrorAllLinksWithScratch(&mergeScratch, head) {
		t.Fatal("clean-zero proof did not fall back after an error was recorded")
	}
}

func TestGSSCleanZeroNodeStateSurvivesScratchEpochForStableNode(t *testing.T) {
	var nodes gssScratch
	payload := NewLeafNode(11, true, 0, 1, Point{}, Point{Column: 1})
	head := nodes.allocNode(newStackEntryNode(2, payload), nil, 1)
	var scratch glrMergeScratch
	scratch.beginEquivEpoch()

	if !gssNodeCleanZeroErrorAllLinksWithScratch(&scratch, head) {
		t.Fatal("clean-zero scan rejected the initial clean node")
	}
	if clean, ok := lookupCleanZeroNodeState(head, gssPrefixAggGen.Load()); !ok || !clean {
		t.Fatalf("clean-zero node state = %v, %v; want true, true", clean, ok)
	}

	scratch.cleanZeroCache = nil
	clear(scratch.cleanZeroFront)
	scratch.beginCleanZeroEpoch()
	if !gssNodeCleanZeroErrorAllLinksWithScratch(&scratch, head) {
		t.Fatal("stable clean-zero node state did not survive a scratch epoch")
	}
}

func TestGSSCleanZeroNodeStateRejectsPublishedPayloadMutation(t *testing.T) {
	var nodes gssScratch
	payload := NewLeafNode(11, true, 0, 1, Point{}, Point{Column: 1})
	head := nodes.allocNode(newStackEntryNode(2, payload), nil, 1)
	var scratch glrMergeScratch
	scratch.beginEquivEpoch()

	if !gssNodeCleanZeroErrorAllLinksWithScratch(&scratch, head) {
		t.Fatal("clean-zero scan rejected the initial clean node")
	}
	payload.setHasError(true)
	nodeBumpEquivVersion(payload)
	scratch.beginCleanZeroEpoch()
	if gssNodeCleanZeroErrorAllLinksWithScratch(&scratch, head) {
		t.Fatal("clean-zero state survived a recovery-relevant payload mutation")
	}
}

func BenchmarkGSSCleanZeroAllLinksNegativeAncestorCache(b *testing.B) {
	var nodes gssScratch
	errorEntry := newStackEntryNode(10, NewLeafNode(errorSymbol, true, 0, 1, Point{}, Point{Column: 1}))
	stackEntryNode(errorEntry).setHasError(true)
	bad := nodes.allocNode(stackEntry{state: 3}, nil, 1)
	bad.appendExtraLink(gssMainLink{entry: errorEntry})

	head := bad
	const chainLen = 1000
	for i := 1; i < chainLen; i++ {
		head = nodes.allocNode(stackEntry{state: StateID(i%17 + 1)}, head, uint32(i+1))
	}

	var scratch glrMergeScratch
	scratch.ensureMergeHotCaches()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scratch.beginCleanZeroEpoch()
		for node := head; node != nil; node = node.prev {
			if gssNodeCleanZeroErrorAllLinksWithScratch(&scratch, node) {
				b.Fatal("negative ancestor returned clean")
			}
		}
	}
}

func TestStackCleanZeroErrorCostFreshParseProofCoversContiguousStacks(t *testing.T) {
	childErrors := false
	mergeScratch := glrMergeScratch{childErrors: &childErrors}
	errorNode := NewLeafNode(errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	errorNode.setHasError(true)
	stack := glrStack{
		entries: []stackEntry{newStackEntryNode(2, errorNode)},
		cPaused: true,
	}

	got, ok := cStackCleanZeroErrorCostForMerge(&mergeScratch, &stack)
	if want := cStackOpenRecoveryCost(&stack); !ok || got != want {
		t.Fatalf("fresh full-parse cost proof = (%d, %v), want (%d, true)", got, ok, want)
	}
	childErrors = true
	if _, ok := cStackCleanZeroErrorCostForMerge(&mergeScratch, &stack); ok {
		t.Fatal("clean-zero cost proof did not fall back after an error was recorded")
	}
	if got := cStackErrorCostForMergeCached(&mergeScratch, nil, &stack); got <= cStackOpenRecoveryCost(&stack) {
		t.Fatalf("fallback cost = %d, want child error cost above open recovery cost", got)
	}
}

func TestGSSNodesCanMergeAllowsCleanPackedPredecessorLinks(t *testing.T) {
	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	extraPrev := scratch.allocNode(stackEntry{state: 9}, nil, 1)
	extraEntry := newStackEntryNode(2, NewLeafNode(99, true, 0, 1, Point{}, Point{Column: 1}))
	withExtra := scratch.allocNode(newStackEntryNode(2, NewLeafNode(11, true, 0, 1, Point{}, Point{Column: 1})), base, 2)
	withExtra.appendExtraLink(gssMainLink{prev: extraPrev, entry: extraEntry})
	candidate := scratch.allocNode(newStackEntryNode(2, NewLeafNode(11, true, 0, 1, Point{}, Point{Column: 1})), base, 2)

	if !gssNodesCanMerge(withExtra, candidate) {
		t.Fatal("gssNodesCanMerge = false for clean packed predecessor")
	}

	errorEntry := newStackEntryNode(2, NewLeafNode(99, true, 0, 1, Point{}, Point{Column: 1}))
	stackEntryNode(errorEntry).setHasError(true)
	withExtra.setExtraLink(0, gssMainLink{prev: extraPrev, entry: errorEntry})
	if _, ok := lookupCleanZeroNodeState(withExtra, gssPrefixAggGen.Load()); ok {
		t.Fatal("extra-link rewrite retained the clean-zero node result")
	}
	if gssNodesCanMerge(withExtra, candidate) {
		t.Fatal("gssNodesCanMerge = true for error-bearing packed predecessor")
	}
}

func TestGSSNodesCanMergeRejectsZeroWidthAncestorDescendantPackedMerge(t *testing.T) {
	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	entry := func() stackEntry {
		return newStackEntryNode(2, NewLeafNode(11, true, 0, 0, Point{}, Point{}))
	}
	ancestor := scratch.allocNode(entry(), base, 2)
	descendant := scratch.allocNode(entry(), ancestor, 3)

	if gssNodesCanMerge(ancestor, descendant) {
		t.Fatal("gssNodesCanMerge = true for zero-width ancestor/descendant pair")
	}

	incumbent := glrStack{gss: gssStack{head: ancestor}, byteOffset: 0}
	candidate := glrStack{gss: gssStack{head: descendant}, byteOffset: 0}
	if gssMainMerge(&incumbent, &candidate) {
		t.Fatal("gssMainMerge reported success for zero-width ancestor/descendant pair")
	}

	forks := reduceWindowsFromGSS(&incumbent, 1, maxMainLinkCount)
	if len(forks) != 1 {
		t.Fatalf("reduce windows after rejected cycle merge = %d, want 1", len(forks))
	}
	if got := incumbent.gss.head.linkCount(); got != 1 {
		t.Fatalf("ancestor link count = %d, want 1", got)
	}
}

// TestResetGSSCanReachVisitedMapForPoolClearsExactMap proves the map clearing
// contract without depending on sync.Pool to return a specific item.
func TestResetGSSCanReachVisitedMapForPoolClearsExactMap(t *testing.T) {
	visited := make(map[*gssNode]bool, 100)
	for i := 0; i < 100; i++ {
		visited[&gssNode{depth: uint32(i + 1)}] = true
	}

	if !resetGSSCanReachVisitedMapForPool(visited) {
		t.Fatal("ordinary visited map was rejected from the pool")
	}
	if len(visited) != 0 {
		t.Fatalf("released visited map retained %d node pointer(s)", len(visited))
	}
}

func TestMergeStacksGeneralFaithfulGSSUnionPreservesMoreThanFourStacks(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	var gssScratch gssScratch
	buildStack := func(sym Symbol) glrStack {
		node := NewLeafNode(sym, true, 0, 5, Point{}, Point{Column: 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &gssScratch),
			byteOffset: stackByteOffset(entries),
		}
	}

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch([]glrStack{
		buildStack(11),
		buildStack(12),
		buildStack(13),
		buildStack(14),
		buildStack(15),
	}, &scratch)
	if len(result) != 1 {
		t.Fatalf("merged stack count = %d, want 1", len(result))
	}
	if got := result[0].gss.head.linkCount(); got != 5 {
		t.Fatalf("merged link count = %d, want 5", got)
	}
}

func TestMergeStacksDeferExactFaithfulGSSUnionPreservesMoreThanFourStacks(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	var gssScratch gssScratch
	buildStack := func(sym Symbol) glrStack {
		node := NewLeafNode(sym, true, 0, 5, Point{}, Point{Column: 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &gssScratch),
			byteOffset: stackByteOffset(entries),
		}
	}

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.deferExactDedupe = true
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch([]glrStack{
		buildStack(21),
		buildStack(22),
		buildStack(23),
		buildStack(24),
		buildStack(25),
	}, &scratch)
	if len(result) != 1 {
		t.Fatalf("merged stack count = %d, want 1", len(result))
	}
	if got := result[0].gss.head.linkCount(); got != 5 {
		t.Fatalf("merged link count = %d, want 5", got)
	}
}

func TestGSSMainMergePreservesCandidateBeyondUnsafeCLinkCap(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	var gssScratch gssScratch
	buildStack := func(sym Symbol) glrStack {
		node := NewLeafNode(sym, true, 0, 5, Point{}, Point{Column: 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &gssScratch),
			byteOffset: stackByteOffset(entries),
		}
	}
	incumbent := buildStack(20)
	for i := 1; i < maxMainLinkCount; i++ {
		extraNode := NewLeafNode(Symbol(20+i), true, 0, 5, Point{}, Point{Column: 5})
		incumbent.gss.head.appendExtraLink(gssMainLink{
			prev:  incumbent.gss.head.prev,
			entry: newStackEntryNode(7, extraNode),
		})
	}
	if got := incumbent.gss.head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("setup link count = %d, want %d", got, maxMainLinkCount)
	}

	scratch := glrMergeScratch{language: &Language{}}
	scratch.perKeyCap = 1
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch([]glrStack{incumbent, buildStack(99)}, &scratch)
	if len(result) != 2 {
		t.Fatalf("stack count after capped merge = %d, want 2", len(result))
	}
	if got := result[0].gss.head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("link count after capped merge = %d, want %d", got, maxMainLinkCount)
	}
}

func TestGSSMainMergeCLinkPolicyRetiresCandidateAtEightLinkCap(t *testing.T) {
	var gssScratch gssScratch
	buildStack := func(sym Symbol) glrStack {
		node := NewLeafNode(sym, true, 0, 5, Point{}, Point{Column: 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &gssScratch),
			byteOffset: stackByteOffset(entries),
		}
	}

	incumbent := buildStack(20)
	for i := 1; i < maxCMainLinkCount; i++ {
		extraNode := NewLeafNode(Symbol(20+i), true, 0, 5, Point{}, Point{Column: 5})
		incumbent.gss.head.appendExtraLinkWithLimit(gssMainLink{
			prev:  incumbent.gss.head.prev,
			entry: newStackEntryNode(StateID(7+i), extraNode),
		}, maxCMainLinkCount)
	}
	before := snapshotGSSMainLinks(incumbent.gss.head)
	candidate := buildStack(99)
	scratch := glrMergeScratch{language: &Language{CompactPackedGSSVersionOrderCertified: true}}
	if !gssMainCanMergeWithScratch(&scratch, &incumbent, &candidate) {
		t.Fatal("certified C link policy rejected the merge eligibility gate")
	}
	if !gssMainMergeWithScratch(&scratch, &incumbent, &candidate) {
		t.Fatal("certified C link policy retained a candidate that C retires")
	}
	assertGSSMainLinksEqual(t, incumbent.gss.head, before)
}

func TestMergeStacksGeneralFaithfulGSSPreservesCandidateBeyondUnsafeCLinkCap(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	var gssScratch gssScratch
	buildStack := func(state StateID, sym Symbol, start uint32) glrStack {
		node := NewLeafNode(sym, true, start, start+5, Point{Column: start}, Point{Column: start + 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(state, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &gssScratch),
			byteOffset: stackByteOffset(entries),
		}
	}
	incumbent := buildStack(7, 30, 0)
	for i := 1; i < maxMainLinkCount; i++ {
		extraNode := NewLeafNode(Symbol(30+i), true, 0, 5, Point{}, Point{Column: 5})
		incumbent.gss.head.appendExtraLink(gssMainLink{
			prev:  incumbent.gss.head.prev,
			entry: newStackEntryNode(StateID(7+i), extraNode),
		})
	}
	candidate := buildStack(7, 99, 0)
	fillers := []glrStack{
		buildStack(8, 41, 10),
		buildStack(9, 42, 20),
		buildStack(10, 43, 30),
	}

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch(append([]glrStack{incumbent, candidate}, fillers...), &scratch)
	sameKeyCount := 0
	foundCandidate := false
	for i := range result {
		if result[i].top().state == 7 && result[i].byteOffset == 5 {
			sameKeyCount++
			if stackEntryNodeSymbol(result[i].top()) == 99 {
				foundCandidate = true
			}
		}
	}
	if sameKeyCount != 2 {
		t.Fatalf("same-key survivor count = %d, want 2", sameKeyCount)
	}
	if !foundCandidate {
		t.Fatal("candidate stack was not preserved after unsafe capped GSS merge")
	}
	if got := result[0].gss.head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("link count after capped merge = %d, want %d", got, maxMainLinkCount)
	}
}

func TestMergeStacksDeferExactFaithfulGSSPreservesCandidateBeyondUnsafeCLinkCap(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	var gssScratch gssScratch
	buildStack := func(state StateID, sym Symbol, start uint32) glrStack {
		node := NewLeafNode(sym, true, start, start+5, Point{Column: start}, Point{Column: start + 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(state, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &gssScratch),
			byteOffset: stackByteOffset(entries),
		}
	}
	incumbent := buildStack(7, 50, 0)
	for i := 1; i < maxMainLinkCount; i++ {
		extraNode := NewLeafNode(Symbol(50+i), true, 0, 5, Point{}, Point{Column: 5})
		incumbent.gss.head.appendExtraLink(gssMainLink{
			prev:  incumbent.gss.head.prev,
			entry: newStackEntryNode(StateID(7+i), extraNode),
		})
	}
	candidate := buildStack(7, 109, 0)
	fillers := []glrStack{
		buildStack(8, 61, 10),
		buildStack(9, 62, 20),
		buildStack(10, 63, 30),
	}

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.deferExactDedupe = true
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch(append([]glrStack{incumbent, candidate}, fillers...), &scratch)
	sameKeyCount := 0
	foundCandidate := false
	for i := range result {
		if result[i].top().state == 7 && result[i].byteOffset == 5 {
			sameKeyCount++
			if stackEntryNodeSymbol(result[i].top()) == 109 {
				foundCandidate = true
			}
		}
	}
	if sameKeyCount != 2 {
		t.Fatalf("same-key survivor count = %d, want 2", sameKeyCount)
	}
	if !foundCandidate {
		t.Fatal("candidate stack was not preserved after unsafe capped GSS merge")
	}
	if got := result[0].gss.head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("link count after capped merge = %d, want %d", got, maxMainLinkCount)
	}
}

func TestMergeStacksSmallFaithfulGSSSameInlinePreservesCandidateBeyondUnsafeCLinkCap(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	var scratchGSS gssScratch
	prev := scratchGSS.allocNode(stackEntry{state: 1}, nil, 1)
	sharedTop := NewLeafNode(30, true, 0, 5, Point{}, Point{Column: 5})
	topEntry := newStackEntryNode(7, sharedTop)
	incumbentHead := scratchGSS.allocNode(topEntry, prev, 2)
	candidateHead := scratchGSS.allocNode(topEntry, prev, 2)
	for i := 1; i < maxMainLinkCount; i++ {
		extraNode := NewLeafNode(Symbol(30+i), true, 0, 5, Point{}, Point{Column: 5})
		incumbentHead.appendExtraLink(gssMainLink{
			prev:  prev,
			entry: newStackEntryNode(StateID(7+i), extraNode),
		})
	}
	candidateExtra := NewLeafNode(99, true, 0, 5, Point{}, Point{Column: 5})
	candidateHead.appendExtraLink(gssMainLink{
		prev:  prev,
		entry: newStackEntryNode(99, candidateExtra),
	})

	incumbent := glrStack{gss: gssStack{head: incumbentHead}, byteOffset: 5}
	candidate := glrStack{gss: gssStack{head: candidateHead}, byteOffset: 5}

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.beginEquivEpoch()

	result := mergeStacksWithScratch([]glrStack{incumbent, candidate}, &scratch)
	if len(result) != 2 {
		t.Fatalf("stack count after same-inline capped merge = %d, want 2", len(result))
	}
	if got := result[0].gss.head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("incumbent link count = %d, want %d", got, maxMainLinkCount)
	}
}

func TestGSSMainAddLinkReplacesEquivalentSubtreeWithHigherDynamicPrecedence(t *testing.T) {
	var scratch gssScratch
	prev := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	lowNode := NewLeafNode(10, true, 0, 1, Point{}, Point{Column: 1})
	highNode := NewLeafNode(10, true, 0, 1, Point{}, Point{Column: 1})
	highNode.dynamicPrecedence = 5
	aHead := scratch.allocNode(newStackEntryNode(2, lowNode), prev, 2)
	bHead := scratch.allocNode(newStackEntryNode(3, highNode), prev, 2)
	a := glrStack{gss: gssStack{head: aHead}, byteOffset: 1}
	b := glrStack{gss: gssStack{head: bHead}, byteOffset: 1}

	if !gssMainMerge(&a, &b) {
		t.Fatal("gssMainMerge returned false")
	}
	if got := a.gss.head.linkCount(); got != 1 {
		t.Fatalf("link count = %d, want 1", got)
	}
	if got := stackEntryNode(a.gss.head.entry); got != highNode {
		t.Fatal("equivalent subtree link was not replaced by higher dynamic-precedence tree")
	}
}

func TestStackComparePtrPrefersEarlierBranchOrderOnExactTie(t *testing.T) {
	a := newGLRStack(StateID(5))
	b := newGLRStack(StateID(5))
	a.branchOrder = 1
	b.branchOrder = 2

	if got := stackComparePtr(&a, &b); got <= 0 {
		t.Fatalf("stackComparePtr(a,b) = %d, want > 0", got)
	}
	if got := stackComparePtr(&b, &a); got >= 0 {
		t.Fatalf("stackComparePtr(b,a) = %d, want < 0", got)
	}
}

func TestGLRStackClone(t *testing.T) {
	s := newGLRStack(StateID(1))
	s.push(2, nil, nil, nil)
	s.score = 5

	clone := s.clone()
	clone.push(3, nil, nil, nil)
	clone.score = 10

	if s.depth() != 2 {
		t.Errorf("original entries modified: len=%d, want 2", s.depth())
	}
	if s.score != 5 {
		t.Errorf("original score modified: %d, want 5", s.score)
	}
	if clone.depth() != 3 {
		t.Errorf("clone entries wrong: len=%d, want 3", clone.depth())
	}
}

// buildAmbiguousLanguage creates a grammar where an input can be parsed
// two ways, triggering GLR fork. The grammar:
//
//	S -> A | B
//	A -> x     (production 0, DynamicPrecedence = 0)
//	B -> x     (production 1, DynamicPrecedence = 5)
//
// Both A and B match the same input "x", but B has higher precedence.
// The parser should fork, try both, and pick B.
//
// Symbols: 0=EOF, 1=x (terminal), 2=A (nonterminal), 3=B (nonterminal), 4=S (nonterminal)
//
// States:
//
//	0: x -> shift 1, S -> goto 3, A -> goto 2, B -> goto 2
//	1: any -> reduce A->x AND reduce B->x (multi-action = GLR fork!)
//	2: EOF -> accept
//	3: EOF -> accept (same as state 2 for S)
func buildAmbiguousLanguage() *Language {
	return &Language{
		Name:               "ambiguous",
		SymbolCount:        5,
		TokenCount:         2,
		ExternalTokenCount: 0,
		StateCount:         4,
		LargeStateCount:    0,
		FieldCount:         0,
		ProductionIDCount:  2,

		SymbolNames: []string{"EOF", "x", "A", "B", "S"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "x", Visible: true, Named: true},
			{Name: "A", Visible: true, Named: true},
			{Name: "B", Visible: true, Named: true},
			{Name: "S", Visible: true, Named: true},
		},
		FieldNames: []string{""},

		ParseActions: []ParseActionEntry{
			// 0: error / no action
			{Actions: nil},
			// 1: shift to state 1
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			// 2: TWO actions — GLR fork!
			//    reduce A -> x (1 child, symbol 2, prec 0)
			//    reduce B -> x (1 child, symbol 3, prec 5)
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 2, ChildCount: 1, ProductionID: 0, DynamicPrecedence: 0},
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 1, DynamicPrecedence: 5},
			}},
			// 3: goto state 2 (for A)
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			// 4: goto state 2 (for B)
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			// 5: accept
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},

		ParseTable: [][]uint16{
			// State 0: x->shift(1), A->goto(3), B->goto(4), S->... (unused)
			{0, 1, 3, 4, 0},
			// State 1: any -> action 2 (multi-action: reduce A or reduce B)
			{2, 2, 0, 0, 0},
			// State 2: EOF -> accept
			{5, 0, 0, 0, 0},
			// State 3: (unused, but needed for state count)
			{0, 0, 0, 0, 0},
		},

		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
		},

		LexStates: []LexState{
			// State 0: start
			{
				AcceptToken: 0,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: 'x', Hi: 'x', NextState: 1},
					{Lo: ' ', Hi: ' ', NextState: 2},
				},
			},
			// State 1: accept x (symbol 1)
			{
				AcceptToken: 1,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
			},
			// State 2: whitespace (skip)
			{
				AcceptToken: 0,
				Skip:        true,
				Default:     -1,
				EOF:         -1,
			},
		},
	}
}

func TestGLRForkPicksHigherPrecedence(t *testing.T) {
	lang := buildAmbiguousLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("x"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// The root should be B (symbol 3, prec 5) not A (symbol 2, prec 0)
	// because B has higher dynamic precedence.
	if root.Symbol() != 3 {
		t.Errorf("GLR should pick B (symbol 3, prec 5) but got symbol %d (%s)",
			root.Symbol(), root.Type(lang))
	}
}

func buildForkLanguage(precedences []int16, childCounts []uint8) *Language {
	if len(precedences) == 0 {
		panic("buildForkLanguage requires at least one reduce action")
	}
	if len(precedences) != len(childCounts) {
		panic("buildForkLanguage precedence and childCount lengths must match")
	}

	symbolCount := 2 + len(precedences)
	symbolNames := make([]string, symbolCount)
	symbolMeta := make([]SymbolMetadata, symbolCount)
	symbolNames[0] = "EOF"
	symbolMeta[0] = SymbolMetadata{Name: "EOF"}
	symbolNames[1] = "x"
	symbolMeta[1] = SymbolMetadata{Name: "x", Visible: true, Named: true}
	for i := range precedences {
		name := string(rune('A' + i))
		symbolNames[2+i] = name
		symbolMeta[2+i] = SymbolMetadata{Name: name, Visible: true, Named: true}
	}

	multi := make([]ParseAction, 0, len(precedences))
	for i, prec := range precedences {
		multi = append(multi, ParseAction{
			Type:              ParseActionReduce,
			Symbol:            Symbol(2 + i),
			ChildCount:        childCounts[i],
			ProductionID:      uint16(i),
			DynamicPrecedence: prec,
		})
	}

	// Parse action table:
	//   0: error/no-action
	//   1: shift x -> state 1
	//   2: multi-reduce fork entry
	//   3..(3+n-1): goto actions for non-terminals -> state 2
	//   acceptIdx: accept action
	parseActions := make([]ParseActionEntry, 0, 4+len(precedences))
	parseActions = append(parseActions, ParseActionEntry{Actions: nil})                                               // 0
	parseActions = append(parseActions, ParseActionEntry{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}}) // 1
	parseActions = append(parseActions, ParseActionEntry{Actions: multi})                                             // 2
	for range precedences {
		parseActions = append(parseActions, ParseActionEntry{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}})
	}
	acceptIdx := len(parseActions)
	parseActions = append(parseActions, ParseActionEntry{Actions: []ParseAction{{Type: ParseActionAccept}}})

	rowWidth := symbolCount
	state0 := make([]uint16, rowWidth)
	state0[1] = 1 // x -> shift
	for i := range precedences {
		state0[2+i] = uint16(3 + i) // goto for each non-terminal
	}
	state1 := make([]uint16, rowWidth)
	state1[0] = 2 // EOF -> multi reduce
	state1[1] = 2 // x   -> multi reduce
	state2 := make([]uint16, rowWidth)
	state2[0] = uint16(acceptIdx) // EOF -> accept
	state3 := make([]uint16, rowWidth)

	return &Language{
		Name:               "fork_language",
		SymbolCount:        uint32(symbolCount),
		TokenCount:         2,
		ExternalTokenCount: 0,
		StateCount:         4,
		LargeStateCount:    0,
		FieldCount:         0,
		ProductionIDCount:  uint32(len(precedences)),
		SymbolNames:        symbolNames,
		SymbolMetadata:     symbolMeta,
		FieldNames:         []string{""},
		ParseActions:       parseActions,
		ParseTable:         [][]uint16{state0, state1, state2, state3},
		LexModes:           []LexMode{{LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}},
		LexStates: []LexState{
			{
				AcceptToken: 0,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: 'x', Hi: 'x', NextState: 1},
					{Lo: ' ', Hi: ' ', NextState: 2},
				},
			},
			{
				AcceptToken: 1,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
			},
			{
				AcceptToken: 0,
				Skip:        true,
				Default:     -1,
				EOF:         -1,
			},
		},
	}
}

func TestGLRForkEqualPrecedenceTieKeepsFirstAction(t *testing.T) {
	lang := buildForkLanguage([]int16{0, 0}, []uint8{1, 1})
	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("x"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}
	if root.Symbol() != 2 {
		t.Fatalf("equal-precedence tie should keep first reduce symbol 2, got %d (%s)", root.Symbol(), root.Type(lang))
	}
}

func TestGLRForkHandlesThreeAlternatives(t *testing.T) {
	lang := buildForkLanguage([]int16{0, 5, 3}, []uint8{1, 1, 1})
	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("x"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}
	if root.Symbol() != 3 {
		t.Fatalf("three-way fork should pick highest precedence symbol 3, got %d (%s)", root.Symbol(), root.Type(lang))
	}
}

func TestGLRForkPrunesErroringAlternative(t *testing.T) {
	lang := buildForkLanguage([]int16{10, 1}, []uint8{2, 1})
	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("x"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}
	// First branch requires two non-extra children and should die.
	if root.Symbol() != 3 {
		t.Fatalf("error-pruned fork should keep surviving symbol 3, got %d (%s)", root.Symbol(), root.Type(lang))
	}
}

// TestMergeKeyGroupsEquivalentStacks proves stack equivalence implies equal
// merge keys. With coarse merge keys,
// this ensures equivalent stacks are always deduped in the same bucket.
func TestMergeKeyGroupsEquivalentStacks(t *testing.T) {
	scratch := &gssScratch{}

	// Helper to build a GSS-backed stack with known entries.
	buildStack := func(entries []stackEntry) glrStack {
		var s glrStack
		s.gss = buildGSSStack(entries, scratch)
		s.byteOffset = stackByteOffset(entries)
		return s
	}

	// Case 1: identical entries → equivalent, same hash.
	node1a := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, flags: nodeFlagNamed}
	node1b := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, flags: nodeFlagNamed}
	a := buildStack([]stackEntry{{state: 1}, newStackEntryNode(2, node1a)})
	b := buildStack([]stackEntry{{state: 1}, newStackEntryNode(2, node1b)})

	if !stackEquivalentForLanguageWithScratch(nil, nil, &a, &b) {
		t.Fatal("case 1: expected equivalent stacks")
	}
	ka := mergeKeyForStack(&a)
	kb := mergeKeyForStack(&b)
	if ka != kb {
		t.Fatalf("case 1: equivalent stacks have different merge keys: %+v vs %+v", ka, kb)
	}

	// Case 2: different symbol → not equivalent.
	node2a := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1}
	node2b := &Node{symbol: 11, startByte: 0, endByte: 5, parseState: 1}
	c := buildStack([]stackEntry{{state: 1}, newStackEntryNode(2, node2a)})
	d := buildStack([]stackEntry{{state: 1}, newStackEntryNode(2, node2b)})
	if stackEquivalentForLanguageWithScratch(nil, nil, &c, &d) {
		t.Fatal("case 2: expected non-equivalent stacks")
	}
	kc := mergeKeyForStack(&c)
	kd := mergeKeyForStack(&d)
	if kc == kd {
		t.Log("case 2: non-equivalent stacks share coarse merge key (expected collision)")
	}

	// Case 3: isMissing differs → not equivalent (hash includes isMissing).
	node3a := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1}
	node3b := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, flags: nodeFlagMissing}
	e := buildStack([]stackEntry{{state: 1}, newStackEntryNode(2, node3a)})
	f := buildStack([]stackEntry{{state: 1}, newStackEntryNode(2, node3b)})
	if stackEquivalentForLanguageWithScratch(nil, nil, &e, &f) {
		t.Fatal("case 3: isMissing differs, stacks should not be equivalent")
	}

	// Case 4: nil nodes on both sides → equivalent, same hash.
	g := buildStack([]stackEntry{{state: 1}, {state: 2}})
	h := buildStack([]stackEntry{{state: 1}, {state: 2}})
	if !stackEquivalentForLanguageWithScratch(nil, nil, &g, &h) {
		t.Fatal("case 4: expected equivalent nil-node stacks")
	}
	kg := mergeKeyForStack(&g)
	kh := mergeKeyForStack(&h)
	if kg != kh {
		t.Fatalf("case 4: equivalent nil-node stacks have different merge keys: %+v vs %+v", kg, kh)
	}

	// Case 5: with children — same children → equivalent.
	child1 := &Node{symbol: 20, startByte: 0, endByte: 3}
	child2 := &Node{symbol: 20, startByte: 0, endByte: 3}
	node5a := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, children: []*Node{child1}}
	node5b := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, children: []*Node{child2}}
	i := buildStack([]stackEntry{{state: 1}, newStackEntryNode(2, node5a)})
	j := buildStack([]stackEntry{{state: 1}, newStackEntryNode(2, node5b)})
	if !stackEquivalentForLanguageWithScratch(nil, nil, &i, &j) {
		t.Fatal("case 5: expected equivalent stacks with same children")
	}
	ki := mergeKeyForStack(&i)
	kj := mergeKeyForStack(&j)
	if ki != kj {
		t.Fatalf("case 5: equivalent stacks with children have different merge keys: %+v vs %+v", ki, kj)
	}

	// Case 6: children differ in symbol → not equivalent (children check).
	// Coarse key may collide; stackEquivalent must reject the mismatch.
	child3 := &Node{symbol: 20, startByte: 0, endByte: 3}
	child4 := &Node{symbol: 21, startByte: 0, endByte: 3}
	node6a := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, children: []*Node{child3}}
	node6b := &Node{symbol: 10, startByte: 0, endByte: 5, parseState: 1, children: []*Node{child4}}
	k := buildStack([]stackEntry{{state: 1}, newStackEntryNode(2, node6a)})
	l := buildStack([]stackEntry{{state: 1}, newStackEntryNode(2, node6b)})
	if stackEquivalentForLanguageWithScratch(nil, nil, &k, &l) {
		t.Fatal("case 6: expected non-equivalent stacks with different children")
	}
	// These may share the same coarse merge key; that's fine because
	// stackEquivalent still rejects them.
}

func TestStackEquivalentForAliasLanguageRejectsDeepAliasMismatch(t *testing.T) {
	lang := &Language{
		Name:        "go",
		SymbolCount: 16,
		SymbolNames: make([]string, 16),
		AliasSequences: [][]Symbol{
			{0, 12},
		},
	}
	buildDeepNode := func(leafSym Symbol) *Node {
		leaf := &Node{symbol: leafSym, startByte: 0, endByte: 5, flags: nodeFlagNamed}
		n := leaf
		for sym := Symbol(11); sym >= 4; sym-- {
			n = &Node{
				symbol:    sym,
				startByte: 0,
				endByte:   5,
				flags:     nodeFlagNamed,
				children:  []*Node{n},
			}
			if sym == 4 {
				break
			}
		}
		return &Node{
			symbol:    3,
			startByte: 0,
			endByte:   5,
			flags:     nodeFlagNamed,
			children:  []*Node{n},
		}
	}

	a := glrStack{entries: []stackEntry{{state: 1}, newStackEntryNode(2, buildDeepNode(10))}, byteOffset: 5}
	b := glrStack{entries: []stackEntry{{state: 1}, newStackEntryNode(2, buildDeepNode(12))}, byteOffset: 5}

	if stackEquivalentForLanguageWithScratch(nil, lang, &a, &b) {
		t.Fatal("expected deep alias mismatch to remain distinct for alias language")
	}
}

func TestStackEquivalentForTypeScriptChecksNonFrontierChildren(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolCount: 16,
		SymbolNames: make([]string, 16),
		AliasSequences: [][]Symbol{
			{0, 12},
		},
	}
	buildNode := func(earlyLeaf Symbol) *Node {
		early := &Node{
			symbol:    2,
			startByte: 0,
			endByte:   5,
			flags:     nodeFlagNamed,
			children: []*Node{{
				symbol:    earlyLeaf,
				startByte: 0,
				endByte:   5,
				flags:     nodeFlagNamed,
			}},
		}
		frontier := &Node{
			symbol:    6,
			startByte: 5,
			endByte:   10,
			flags:     nodeFlagNamed,
			children: []*Node{{
				symbol:    7,
				startByte: 5,
				endByte:   10,
				flags:     nodeFlagNamed,
			}},
		}
		return &Node{
			symbol:    1,
			startByte: 0,
			endByte:   10,
			flags:     nodeFlagNamed,
			children:  []*Node{early, frontier},
		}
	}

	aNode := buildNode(3)
	bNode := buildNode(4)
	// Precondition reconciled with 061b67f9, which deliberately strengthened
	// the small-subtree frontier heuristic
	// (TestAliasSequenceFrontierEquivalenceChecksAllSmallSemanticChildren now
	// requires it to CATCH structurally-identical early-child mismatches).
	// The frontier fast path may therefore either miss or catch this
	// mismatch; the load-bearing assertion below is only that full
	// TypeScript stack equivalence rejects the merge.
	_ = stackEntryNodesEquivalentFrontierWithScratch(nil, aNode, bNode, stackEquivalentFrontierDepthLimit)

	a := glrStack{entries: []stackEntry{{state: 1}, newStackEntryNode(2, aNode)}, byteOffset: 10}
	b := glrStack{entries: []stackEntry{{state: 1}, newStackEntryNode(2, bNode)}, byteOffset: 10}
	if stackEquivalentForLanguageWithScratch(nil, lang, &a, &b) {
		t.Fatal("expected TypeScript stack equivalence to compare non-frontier children")
	}
}

func TestMergeStacksAllDeadReturnsEmpty(t *testing.T) {
	s1 := newGLRStack(StateID(1))
	s2 := newGLRStack(StateID(2))
	s1.dead = true
	s2.dead = true
	merged := mergeStacks([]glrStack{s1, s2})
	if len(merged) != 0 {
		t.Fatalf("expected all-dead merge to return empty, got %d", len(merged))
	}
}
