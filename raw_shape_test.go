package gotreesitter

import (
	"testing"
	"unsafe"
)

const (
	rawShapeSemanticRootSym Symbol = 1
	rawShapeSemanticWrapSym Symbol = 2
	rawShapeSemanticASym    Symbol = 3
	rawShapeSemanticBSym    Symbol = 4
	rawShapeSemanticCWrap   Symbol = 5
	rawShapeSemanticCSplice Symbol = 6
	rawShapeSemanticAlias   Symbol = 7
	rawShapeSemanticWrapPID        = 1
)

func TestRawShapeHeaderLayoutStaysCompact(t *testing.T) {
	if got, want := unsafe.Sizeof(rawShape{}), uintptr(16); got != want {
		t.Fatalf("rawShape header size = %d bytes, want %d", got, want)
	}
	if got, want := unsafe.Sizeof(rawShapeChild{}), uintptr(16); got != want {
		t.Fatalf("rawShape child size = %d bytes, want %d", got, want)
	}
}

func TestRawShapeChildPacksShapeRefAndRestoresCurrentState(t *testing.T) {
	node := &Node{parseState: 41, rawShape: 73}
	child := newRawShapeChild(newStackEntryNode(17, node))
	if got := child.shapeRef(); got != 73 {
		t.Fatalf("packed shape ref = %d, want 73", got)
	}
	entry := child.entry()
	if got := stackEntryNode(entry); got != node {
		t.Fatalf("restored node = %p, want %p", got, node)
	}
	if got := entry.state; got != 41 {
		t.Fatalf("restored state = %d, want current node state 41", got)
	}
}

func TestRawShapeHashCacheRecomputesAfterEviction(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := testRawShapeParser()

	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	ref := parser.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, leaf)}, 0, 1)
	if ref == 0 {
		t.Fatal("captureRawShape returned no reference")
	}
	want, ok := arena.rawShapeHash(ref)
	if !ok {
		t.Fatal("rawShapeHash failed before eviction")
	}

	// Find a colliding reference outside the arena and replace the cached hash.
	const fakeRefBase = rawShapeRef(2 << rawShapeRefIndexBits)
	evicted := false
	for i := 0; i < 1<<rawShapeRefIndexBits; i++ {
		fakeRef := fakeRefBase + rawShapeRef(i)
		if rawShapeHashCacheIndex(fakeRef) == rawShapeHashCacheIndex(ref) {
			arena.storeRawShapeHash(fakeRef, want^1)
			evicted = true
			break
		}
	}
	if !evicted {
		t.Fatal("no cache collision found")
	}
	got, ok := arena.rawShapeHash(ref)
	if !ok {
		t.Fatal("rawShapeHash failed after eviction")
	}
	if got != want {
		t.Fatalf("recomputed raw-shape hash = %#x, want %#x", got, want)
	}
}

func TestRawErrorCostUsesCapturedZeroShapeReference(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := testRawShapeParser()

	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	parent := newParentNodeInArena(arena, 1, true, []*Node{leaf}, nil, 0)
	parent.rawShape = parser.captureRawShape(
		nil,
		arena,
		1,
		0,
		[]stackEntry{newStackEntryNode(0, parent)},
		0,
		1,
	)
	if parent.rawShape == 0 {
		t.Fatal("parent raw shape = 0")
	}
	shape, ok := arena.rawShapeForRef(parent.rawShape)
	if !ok {
		t.Fatal("parent raw shape is unavailable")
	}
	children := arena.rawShapeChildren(shape)
	if len(children) != 1 || children[0].shapeRef() != 0 {
		t.Fatalf("captured child shape refs = %+v, want one zero ref", children)
	}

	if got := parser.rawStackEntryErrorCost(arena, newStackEntryNode(1, parent)); got != 0 {
		t.Fatalf("raw error cost = %d, want 0", got)
	}
	if parent.rawShape == 0 {
		t.Fatal("raw error-cost walk cleared the live parent shape")
	}
}

func testRawShapeParser() *Parser {
	lang := &Language{
		Name:        "raw-shape-test",
		SymbolNames: []string{"EOF", "root", "_hidden", "leaf_a", "leaf_b"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: true, Named: true},
			{Name: "root", Visible: true, Named: true},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "leaf_a", Visible: true, Named: true},
			{Name: "leaf_b", Visible: true, Named: true},
		},
	}
	return &Parser{language: lang, hasRootSymbol: true, rootSymbol: 1}
}

func TestRawShapeCompareDistinguishesHiddenFlattenedPublicShape(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := testRawShapeParser()

	leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	hidden := newParentNodeInArena(arena, 2, false, []*Node{leaf}, nil, 0)
	flatParent := newParentNodeInArena(arena, 1, true, []*Node{leaf}, nil, 0)
	hiddenParent := newParentNodeInArena(arena, 1, true, []*Node{leaf}, nil, 0)

	flatParent.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, leaf)}, 0, 1)
	hiddenParent.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, hidden)}, 0, 1)

	flatEntry := newStackEntryNode(1, flatParent)
	hiddenEntry := newStackEntryNode(1, hiddenParent)
	if stackEntryNodeChildCount(flatEntry) != stackEntryNodeChildCount(hiddenEntry) {
		t.Fatal("synthetic parents do not have matching public child counts")
	}
	if cmp := parser.compareRawStackEntries(arena, hiddenEntry, flatEntry); cmp >= 0 {
		t.Fatalf("raw compare = %d, want hidden raw child symbol lower than flat leaf", cmp)
	}
}

func TestAcceptedStackSelectionUsesRawShapeBeforeBranchOrder(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := testRawShapeParser()

	leafA := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	leafB := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
	rootA := newParentNodeInArena(arena, 1, true, []*Node{leafA}, nil, 0)
	rootB := newParentNodeInArena(arena, 1, true, []*Node{leafA}, nil, 0)
	rootA.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, leafA)}, 0, 1)
	rootB.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, leafB)}, 0, 1)

	laterBetter := glrStack{
		accepted:    true,
		entries:     []stackEntry{newStackEntryNode(1, rootA)},
		branchOrder: 2,
	}
	earlierWorse := glrStack{
		accepted:    true,
		entries:     []stackEntry{newStackEntryNode(1, rootB)},
		branchOrder: 1,
	}
	if cmp := stackCompareForResultSelection(parser, arena, &laterBetter, &earlierWorse, false); cmp <= 0 {
		t.Fatalf("stack compare = %d, want later branch with smaller raw shape before branchOrder", cmp)
	}
}

func TestAcceptedStackRawShapeDoesNotOverrideDistinctMaterializedRoot(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		replacementSym Symbol = 2
		qmarkSym       Symbol = 3
		varSym         Symbol = 4
		argsSym        Symbol = 5
		macroCallSym   Symbol = 6
		callSym        Symbol = 7
	)
	lang := &Language{
		Name:        "raw-shape-visible-root-test",
		SymbolNames: []string{"EOF", "root", "_replacement", "?", "var", "args", "macro_call_expr", "call"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "_replacement", Visible: false, Named: false},
			{Name: "?", Visible: false, Named: false},
			{Name: "var", Visible: true, Named: true},
			{Name: "args", Visible: true, Named: true},
			{Name: "macro_call_expr", Visible: true, Named: true},
			{Name: "call", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: 1}

	qmark := newLeafNodeInArena(arena, qmarkSym, false, 0, 1, Point{}, Point{Column: 1})
	varName := newLeafNodeInArena(arena, varSym, true, 1, 7, Point{Column: 1}, Point{Column: 7})
	macroArgs := newLeafNodeInArena(arena, argsSym, true, 7, 22, Point{Column: 7}, Point{Column: 22})
	macro := newParentNodeInArena(arena, macroCallSym, true, []*Node{qmark, varName, macroArgs}, nil, 0)
	macro.rawShape = parser.captureRawShape(nil, arena, replacementSym, 0, []stackEntry{
		newStackEntryNode(0, qmark),
		newStackEntryNode(0, varName),
		newStackEntryNode(0, macroArgs),
	}, 0, 3)

	macroName := newLeafNodeInArena(arena, macroCallSym, true, 0, 7, Point{}, Point{Column: 7})
	callArgs := newLeafNodeInArena(arena, argsSym, true, 7, 22, Point{Column: 7}, Point{Column: 22})
	call := newParentNodeInArena(arena, callSym, true, []*Node{macroName, callArgs}, nil, 0)
	call.rawShape = parser.captureRawShape(nil, arena, replacementSym, 0, []stackEntry{
		newStackEntryNode(0, macroName),
		newStackEntryNode(0, callArgs),
	}, 0, 2)

	macroEntry := newStackEntryNode(1, macro)
	callEntry := newStackEntryNode(1, call)
	if cmp := parser.compareRawStackEntries(arena, callEntry, macroEntry); cmp >= 0 {
		t.Fatalf("raw compare(call, macro) = %d, want call to be raw-shape preferred in the old ordering", cmp)
	}

	earlierMacro := glrStack{
		accepted:    true,
		byteOffset:  22,
		branchOrder: 4,
		entries:     []stackEntry{macroEntry},
	}
	laterCall := glrStack{
		accepted:    true,
		byteOffset:  22,
		branchOrder: 7,
		entries:     []stackEntry{callEntry},
	}
	if cmp := compareAcceptedStackRawShapePreference(parser, arena, &laterCall, &earlierMacro); cmp <= 0 {
		t.Fatalf("accepted raw-shape compare(call, macro) = %d, want later call raw-shape preferred in the full-root ordering", cmp)
	}
	if cmp := stackCompareForResultSelection(parser, arena, &laterCall, &earlierMacro, false); cmp <= 0 {
		t.Fatalf("result compare(call, macro) = %d, want normal result selection to retain raw-shape ordering", cmp)
	}

	acceptedNode := &gssForestNode{
		state:      99,
		byteOffset: 22,
		links: []gssLink{
			{subtree: macroEntry, score: 0, errorCost: 0},
			{subtree: callEntry, score: 0, errorCost: 0},
		},
	}
	callLink := &gssLink{subtree: callEntry, score: 0, errorCost: 0}
	macroLink := &gssLink{subtree: macroEntry, score: 0, errorCost: 0}
	if cmp := forestResultLinkCompare(parser, arena, acceptedNode, callLink, 7, macroLink, 4); cmp <= 0 {
		t.Fatalf("forest local link compare(call, macro) = %d, want raw-shape to remain active locally", cmp)
	}
	if cmp := forestResultLinkCompareWithRawShape(parser, arena, acceptedNode, callLink, 7, macroLink, 4, false); cmp >= 0 {
		t.Fatalf("forest accepted-root compare(call, macro) = %d, want later raw-preferred root to lose", cmp)
	}
	if best := acceptedNode.bestAcceptedRootResultLink(parser, arena); best == nil || stackEntryNode(best.subtree) != macro {
		t.Fatalf("accepted root best link = %v, want earlier macro root", best)
	}
}

func TestForestChildAlternativeResolutionUsesLocalSameSpanBest(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym        Symbol = 1
		replacementSym Symbol = 2
		partASym       Symbol = 3
		partBSym       Symbol = 4
		partCSym       Symbol = 5
		macroCallSym   Symbol = 6
		callSym        Symbol = 7
	)
	lang := &Language{
		Name:        "forest-child-alternative-test",
		SymbolNames: []string{"EOF", "root", "_replacement", "part_a", "part_b", "part_c", "macro_call_expr", "call"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "_replacement", Visible: false, Named: false},
			{Name: "part_a", Visible: true, Named: true},
			{Name: "part_b", Visible: true, Named: true},
			{Name: "part_c", Visible: true, Named: true},
			{Name: "macro_call_expr", Visible: true, Named: true},
			{Name: "call", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	macroA := newLeafNodeInArena(arena, partASym, true, 0, 2, Point{}, Point{Column: 2})
	macroB := newLeafNodeInArena(arena, partBSym, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	macro := newParentNodeInArena(arena, macroCallSym, true, []*Node{macroA, macroB}, nil, 0)
	macro.rawShape = parser.captureRawShape(nil, arena, replacementSym, 0, []stackEntry{
		newStackEntryNode(0, macroA),
		newStackEntryNode(0, macroB),
	}, 0, 2)

	callA := newLeafNodeInArena(arena, partASym, true, 0, 1, Point{}, Point{Column: 1})
	callB := newLeafNodeInArena(arena, partBSym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	callC := newLeafNodeInArena(arena, partCSym, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	call := newParentNodeInArena(arena, callSym, true, []*Node{callA, callB, callC}, nil, 0)
	call.rawShape = parser.captureRawShape(nil, arena, replacementSym, 0, []stackEntry{
		newStackEntryNode(0, callA),
		newStackEntryNode(0, callB),
		newStackEntryNode(0, callC),
	}, 0, 3)

	root := newParentNodeInArena(arena, rootSym, true, []*Node{call}, nil, 0)
	root.rawShape = parser.captureRawShape(nil, arena, rootSym, 0, []stackEntry{newStackEntryNode(0, call)}, 0, 1)

	local := &gssForestNode{
		state:      42,
		byteOffset: 4,
		links: []gssLink{
			{subtree: newStackEntryNode(42, call), score: 0, errorCost: 0},
			{subtree: newStackEntryNode(42, macro), score: 0, errorCost: 0},
		},
	}
	if best := local.bestResultLink(parser, arena); best == nil || stackEntryNode(best.subtree) != macro {
		t.Fatalf("local best link = %v, want macro raw-shape alternative", best)
	}

	alternatives := newForestAlternativeIndex(4)
	forestRecordAlternative(alternatives, newStackEntryNode(42, call), local)
	forestRecordParentChildAlternatives(alternatives, root, root.children, []stackEntry{newStackEntryNode(42, call)})
	resolveForestChildAlternatives(parser, arena, root, alternatives, nil, 0)
	if got := root.children[0]; got != macro {
		t.Fatalf("resolved child = %v, want macro alternative", got)
	}
	shape, ok := arena.rawShapeForRef(root.rawShape)
	if !ok {
		t.Fatal("root raw shape missing")
	}
	children := arena.rawShapeChildren(shape)
	if len(children) != 1 || stackEntryNode(children[0].entry()) != macro {
		t.Fatalf("root raw-shape child = %v, want macro alternative", children)
	}
}

func TestForestChildAlternativeResolutionRejectsIncompatibleSiblingPath(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym        Symbol = 1
		replacementSym Symbol = 2
		partASym       Symbol = 3
		partBSym       Symbol = 4
		partCSym       Symbol = 5
		macroCallSym   Symbol = 6
		callSym        Symbol = 7
		siblingSym     Symbol = 8
	)
	lang := &Language{
		Name:        "forest-child-alternative-path-test",
		SymbolNames: []string{"EOF", "root", "_replacement", "part_a", "part_b", "part_c", "macro_call_expr", "call", "sibling"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "_replacement", Visible: false, Named: false},
			{Name: "part_a", Visible: true, Named: true},
			{Name: "part_b", Visible: true, Named: true},
			{Name: "part_c", Visible: true, Named: true},
			{Name: "macro_call_expr", Visible: true, Named: true},
			{Name: "call", Visible: true, Named: true},
			{Name: "sibling", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	macroA := newLeafNodeInArena(arena, partASym, true, 0, 1, Point{}, Point{Column: 1})
	macroB := newLeafNodeInArena(arena, partBSym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	macro := newParentNodeInArena(arena, macroCallSym, true, []*Node{macroA, macroB}, nil, 0)
	macro.rawShape = parser.captureRawShape(nil, arena, replacementSym, 0, []stackEntry{
		newStackEntryNode(0, macroA),
		newStackEntryNode(0, macroB),
	}, 0, 2)

	callA := newLeafNodeInArena(arena, partASym, true, 0, 1, Point{}, Point{Column: 1})
	callC := newLeafNodeInArena(arena, partCSym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	call := newParentNodeInArena(arena, callSym, true, []*Node{callA, callC}, nil, 0)
	call.rawShape = parser.captureRawShape(nil, arena, replacementSym, 0, []stackEntry{
		newStackEntryNode(0, callA),
		newStackEntryNode(0, callC),
	}, 0, 2)

	sibling := newLeafNodeInArena(arena, siblingSym, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	root := newParentNodeInArena(arena, rootSym, true, []*Node{call, sibling}, nil, 0)
	root.rawShape = parser.captureRawShape(nil, arena, rootSym, 0, []stackEntry{
		newStackEntryNode(0, call),
		newStackEntryNode(0, sibling),
	}, 0, 2)

	compatiblePrev := &gssForestNode{state: 10}
	incompatiblePrev := &gssForestNode{state: 20}
	local := &gssForestNode{
		state:      42,
		byteOffset: 2,
		links: []gssLink{
			{prev: compatiblePrev, subtree: newStackEntryNode(42, call), score: 0, errorCost: 0},
			{prev: incompatiblePrev, subtree: newStackEntryNode(42, macro), score: 0, errorCost: 0},
		},
	}
	if best := local.bestResultLink(parser, arena); best == nil || stackEntryNode(best.subtree) != macro {
		t.Fatalf("global best link = %v, want incompatible macro alternative to be globally preferred", best)
	}

	alternatives := newForestAlternativeIndex(4)
	forestRecordAlternative(alternatives, newStackEntryNode(42, call), local)
	forestRecordParentChildAlternatives(alternatives, root, root.children, []stackEntry{
		newStackEntryNode(42, call),
		newStackEntryNode(43, sibling),
	})
	resolveForestChildAlternatives(parser, arena, root, alternatives, nil, 0)
	if got := root.children[0]; got != call {
		t.Fatalf("resolved child = %v, want original call because global best alternative is path-incompatible", got)
	}
	shape, ok := arena.rawShapeForRef(root.rawShape)
	if !ok {
		t.Fatal("root raw shape missing")
	}
	children := arena.rawShapeChildren(shape)
	if len(children) != 2 || stackEntryNode(children[0].entry()) != root.children[0] || stackEntryNode(children[1].entry()) != root.children[1] {
		t.Fatalf("root raw-shape children inconsistent with tree children: raw=%v tree=%v", children, root.children)
	}
}

func TestForestChildAlternativeResolutionPreservesVisibleNamedUnaryWrapper(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym   Symbol = 1
		guardSym  Symbol = 2
		callSym   Symbol = 3
		targetSym Symbol = 4
	)
	lang := &Language{
		Name:        "forest-visible-unary-wrapper-test",
		SymbolNames: []string{"EOF", "root", "guard_clause", "call", "target"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "guard_clause", Visible: true, Named: true},
			{Name: "call", Visible: true, Named: true},
			{Name: "target", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	targetA := newLeafNodeInArena(arena, targetSym, true, 8, 10, Point{Column: 8}, Point{Column: 10})
	targetB := newLeafNodeInArena(arena, targetSym, true, 10, 12, Point{Column: 10}, Point{Column: 12})
	call := newParentNodeInArena(arena, callSym, true, []*Node{targetA, targetB}, nil, 0)
	guard := newParentNodeInArena(arena, guardSym, true, []*Node{call}, nil, 0)
	root := newParentNodeInArena(arena, rootSym, true, []*Node{guard}, nil, 0)
	root.rawShape = parser.captureRawShape(nil, arena, rootSym, 0, []stackEntry{newStackEntryNode(0, guard)}, 0, 1)

	prev := &gssForestNode{state: 7}
	local := &gssForestNode{
		state:      42,
		byteOffset: 12,
		links: []gssLink{
			{prev: prev, subtree: newStackEntryNode(42, guard), score: 0, errorCost: 0},
			{prev: prev, subtree: newStackEntryNode(42, call), score: 10, errorCost: 0},
		},
	}
	alternatives := newForestAlternativeIndex(4)
	forestRecordAlternative(alternatives, newStackEntryNode(42, guard), local)
	forestRecordParentChildAlternatives(
		alternatives,
		root,
		root.children,
		[]stackEntry{newStackEntryNode(42, guard)},
	)
	resolveForestChildAlternatives(parser, arena, root, alternatives, nil, 0)
	if got := root.children[0]; got != guard {
		t.Fatalf("resolved child = %v, want visible guard_clause wrapper preserved", got)
	}
}

func TestStackEntryDirectChildPreferencePreservesVisibleNamedUnaryWrapper(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		wrapperSym Symbol = 1
		directSym  Symbol = 2
		partSym    Symbol = 3
	)
	lang := &Language{
		SymbolNames: []string{"EOF", "type", "template_instance", "part"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "type", Visible: true, Named: true},
			{Name: "template_instance", Visible: true, Named: true},
			{Name: "part", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang}
	partA := newLeafNodeInArena(arena, partSym, true, 0, 1, Point{}, Point{Column: 1})
	partB := newLeafNodeInArena(arena, partSym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	direct := newParentNodeInArena(arena, directSym, true, []*Node{partA, partB}, nil, 0)
	wrapper := newParentNodeInArena(arena, wrapperSym, true, []*Node{direct}, nil, 0)
	wrapperEntry := newStackEntryNode(1, wrapper)
	directEntry := newStackEntryNode(1, direct)

	if cmp := compareStackEntryDirectChildPreference(parser, arena, wrapperEntry, directEntry, 0); cmp <= 0 {
		t.Fatalf("wrapper/direct preference = %d, want wrapper winner", cmp)
	}
	if cmp := compareStackEntryDirectChildPreference(parser, arena, directEntry, wrapperEntry, 0); cmp >= 0 {
		t.Fatalf("direct/wrapper preference = %d, want direct loser", cmp)
	}
}

func TestForestChildAlternativeResolutionPreservesVisibleContainerOverHiddenFlatRepeat(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym   Symbol = 1
		repeatSym Symbol = 2
		groupSym  Symbol = 3
		partASym  Symbol = 4
		partBSym  Symbol = 5
	)
	lang := &Language{
		Name:        "forest-visible-container-flat-repeat-test",
		SymbolNames: []string{"EOF", "root", "_root_repeat1", "group", "part_a", "part_b"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "_root_repeat1", Visible: false, Named: false},
			{Name: "group", Visible: true, Named: true},
			{Name: "part_a", Visible: true, Named: true},
			{Name: "part_b", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	groupA := newLeafNodeInArena(arena, partASym, true, 0, 2, Point{}, Point{Column: 2})
	groupB := newLeafNodeInArena(arena, partBSym, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	group := newParentNodeInArena(arena, groupSym, true, []*Node{groupA, groupB}, nil, 0)
	group.rawShape = parser.captureRawShape(nil, arena, groupSym, 0, []stackEntry{
		newStackEntryNode(0, groupA),
		newStackEntryNode(0, groupB),
	}, 0, 2)

	repeatA := newLeafNodeInArena(arena, partASym, true, 0, 2, Point{}, Point{Column: 2})
	repeatB := newLeafNodeInArena(arena, partBSym, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	flatRepeat := newParentNodeInArena(arena, repeatSym, false, []*Node{repeatA, repeatB}, nil, 0)
	flatRepeat.rawShape = parser.captureRawShape(nil, arena, repeatSym, 0, []stackEntry{
		newStackEntryNode(0, repeatA),
		newStackEntryNode(0, repeatB),
	}, 0, 2)

	root := newParentNodeInArena(arena, rootSym, true, []*Node{group}, nil, 0)
	root.rawShape = parser.captureRawShape(nil, arena, rootSym, 0, []stackEntry{newStackEntryNode(42, group)}, 0, 1)

	prev := &gssForestNode{state: 7}
	local := &gssForestNode{
		state:      42,
		byteOffset: 4,
		links: []gssLink{
			{prev: prev, subtree: newStackEntryNode(42, group), score: 0, errorCost: 0},
			{prev: prev, subtree: newStackEntryNode(42, flatRepeat), score: 10, errorCost: 0},
		},
	}
	if best := local.bestResultLinkForPrev(parser, arena, prev); best == nil || stackEntryNode(best.subtree) != flatRepeat {
		t.Fatalf("setup best link = %v, want higher-score flat repeat", best)
	}

	alternatives := newForestAlternativeIndex(4)
	forestRecordAlternative(alternatives, newStackEntryNode(42, group), local)
	forestRecordParentChildAlternatives(alternatives, root, root.children, []stackEntry{newStackEntryNode(42, group)})
	resolveForestChildAlternatives(parser, arena, root, alternatives, nil, 0)
	if got := root.children[0]; got != group {
		t.Fatalf("resolved child = %v, want visible group container preserved", got)
	}
	shape, ok := arena.rawShapeForRef(root.rawShape)
	if !ok {
		t.Fatal("root raw shape missing")
	}
	children := arena.rawShapeChildren(shape)
	if len(children) != 1 || stackEntryNode(children[0].entry()) != group {
		t.Fatalf("root raw-shape child = %v, want visible group container", children)
	}
}

func TestForestRootPreservesVisibleContainerAlternativeOverHiddenRepeatSlice(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym      Symbol = 1
		repeatSym    Symbol = 2
		containerSym Symbol = 3
		headerSym    Symbol = 4
		bodyASym     Symbol = 5
		bodyBSym     Symbol = 6
		endSym       Symbol = 7
	)
	lang := &Language{
		Name:        "forest-root-visible-container-test",
		SymbolNames: []string{"EOF", "root", "_root_repeat1", "container", "header", "body_a", "body_b", "_end"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "_root_repeat1", Visible: false, Named: false},
			{Name: "container", Visible: true, Named: true},
			{Name: "header", Visible: true, Named: true},
			{Name: "body_a", Visible: true, Named: true},
			{Name: "body_b", Visible: true, Named: true},
			{Name: "_end", Visible: false, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	header := newLeafNodeInArena(arena, headerSym, true, 0, 1, Point{}, Point{Column: 1})
	repeatA := newLeafNodeInArena(arena, bodyASym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	repeatB := newLeafNodeInArena(arena, bodyBSym, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	hiddenRepeat := newParentNodeInArena(arena, repeatSym, false, []*Node{repeatA, repeatB}, nil, 0)
	hiddenEnd := newLeafNodeInArena(arena, endSym, true, 3, 3, Point{Column: 3}, Point{Column: 3})
	root := newParentNodeInArena(arena, rootSym, true, []*Node{header, hiddenRepeat, hiddenEnd}, nil, 0)

	containerHeader := newLeafNodeInArena(arena, headerSym, true, 0, 1, Point{}, Point{Column: 1})
	containerA := newLeafNodeInArena(arena, bodyASym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	containerB := newLeafNodeInArena(arena, bodyBSym, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	container := newParentNodeInArena(arena, containerSym, true, []*Node{containerHeader, containerA, containerB}, nil, 0)

	alternatives := newForestAlternativeIndex(4)
	alternatives.setNode(container, &gssForestNode{state: 10})
	if !forestPreserveRootVisibleContainerAlternatives(parser, arena, root, alternatives) {
		t.Fatal("forestPreserveRootVisibleContainerAlternatives = false, want true")
	}
	if got, want := resultChildCount(root), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got := resultChildAt(root, 0); got != container {
		t.Fatalf("root child = %v, want visible container alternative", got)
	}
}

func TestForestRootPreservesRepeatedVisibleContainerAlternative(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym      Symbol = 1
		containerSym Symbol = 2
		headSym      Symbol = 3
		bodySym      Symbol = 4
		nextSym      Symbol = 5
	)
	lang := &Language{
		Name:        "forest-root-repeated-visible-container-test",
		SymbolNames: []string{"EOF", "root", "container", "head", "body", "next"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "container", Visible: true, Named: true},
			{Name: "head", Visible: true, Named: true},
			{Name: "body", Visible: true, Named: true},
			{Name: "next", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	head := newLeafNodeInArena(arena, headSym, true, 0, 1, Point{}, Point{Column: 1})
	body := newLeafNodeInArena(arena, bodySym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	next := newLeafNodeInArena(arena, nextSym, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	first := newParentNodeInArena(arena, containerSym, true, []*Node{head}, nil, 0)
	second := newParentNodeInArena(arena, containerSym, true, []*Node{body}, nil, 0)
	third := newParentNodeInArena(arena, containerSym, true, []*Node{next}, nil, 0)
	root := newParentNodeInArena(arena, rootSym, true, []*Node{first, second, third}, nil, 0)

	candidateHead := newLeafNodeInArena(arena, headSym, true, 0, 1, Point{}, Point{Column: 1})
	candidateBody := newLeafNodeInArena(arena, bodySym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	candidate := newParentNodeInArena(
		arena,
		containerSym,
		true,
		[]*Node{candidateHead, candidateBody},
		nil,
		0,
	)

	alternatives := newForestAlternativeIndex(4)
	alternatives.setNode(candidate, &gssForestNode{state: 10})
	if !forestPreserveRootVisibleContainerAlternatives(parser, arena, root, alternatives) {
		t.Fatal("forestPreserveRootVisibleContainerAlternatives = false, want true")
	}
	if got, want := resultChildCount(root), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got := resultChildAt(root, 0); got != candidate {
		t.Fatalf("root child = %v, want recorded container alternative", got)
	}
	if got := resultChildAt(root, 1); got != third {
		t.Fatalf("root final child = %v, want untouched sibling", got)
	}
}

func TestForestRootRejectsRepeatedVisibleContainerAlternativeWithDifferentFields(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym      Symbol = 1
		containerSym Symbol = 2
		headSym      Symbol = 3
		bodySym      Symbol = 4
	)
	lang := &Language{
		Name:        "forest-root-repeated-visible-container-fields-test",
		SymbolNames: []string{"EOF", "root", "container", "head", "body"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "container", Visible: true, Named: true},
			{Name: "head", Visible: true, Named: true},
			{Name: "body", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	head := newLeafNodeInArena(arena, headSym, true, 0, 1, Point{}, Point{Column: 1})
	body := newLeafNodeInArena(arena, bodySym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	first := newParentNodeInArena(arena, containerSym, true, []*Node{head}, []FieldID{1}, 0)
	second := newParentNodeInArena(arena, containerSym, true, []*Node{body}, []FieldID{2}, 0)
	root := newParentNodeInArena(arena, rootSym, true, []*Node{first, second}, nil, 0)

	candidateHead := newLeafNodeInArena(arena, headSym, true, 0, 1, Point{}, Point{Column: 1})
	candidateBody := newLeafNodeInArena(arena, bodySym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	candidate := newParentNodeInArena(
		arena,
		containerSym,
		true,
		[]*Node{candidateHead, candidateBody},
		[]FieldID{1, 3},
		0,
	)

	alternatives := newForestAlternativeIndex(4)
	alternatives.setNode(candidate, &gssForestNode{state: 10})
	if forestPreserveRootVisibleContainerAlternatives(parser, arena, root, alternatives) {
		t.Fatal("forestPreserveRootVisibleContainerAlternatives = true, want false")
	}
	if got, want := resultChildCount(root), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got := resultChildAt(root, 0); got != first {
		t.Fatalf("root child = %v, want original repeated container", got)
	}
}

func TestAcceptedStackSelectionUsesDynamicPrecedenceBeforeRawShape(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := testRawShapeParser()

	rawPreferredLeaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	rawWorseLeaf := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
	rawPreferredRoot := newParentNodeInArena(arena, 1, true, []*Node{rawPreferredLeaf}, nil, 0)
	dynamicPreferredRoot := newParentNodeInArena(arena, 1, true, []*Node{rawWorseLeaf}, nil, 0)
	rawPreferredRoot.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, rawPreferredLeaf)}, 0, 1)
	dynamicPreferredRoot.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, rawWorseLeaf)}, 0, 1)
	dynamicPreferredRoot.dynamicPrecedence = 7

	dynamicPreferred := glrStack{
		accepted:    true,
		entries:     []stackEntry{newStackEntryNode(1, dynamicPreferredRoot)},
		branchOrder: 2,
	}
	rawPreferred := glrStack{
		accepted:    true,
		entries:     []stackEntry{newStackEntryNode(1, rawPreferredRoot)},
		branchOrder: 1,
	}
	if cmp := stackCompareForResultSelection(parser, arena, &dynamicPreferred, &rawPreferred, false); cmp <= 0 {
		t.Fatalf("stack compare = %d, want higher subtree dynamic precedence before raw shape", cmp)
	}
}

func TestAcceptedStackSelectionPrefersGeneratedRepeatShorterBoundary(t *testing.T) {
	parser, arena, shorter, wider := testGeneratedRepeatBoundaryStacks(t, true)

	if cmp := stackCompareForResultSelection(parser, arena, &shorter, &wider, false); cmp <= 0 {
		t.Fatalf("stack compare(shorter, wider) = %d, want shorter generated repeat boundary", cmp)
	}
	if cmp := stackCompareForResultSelection(parser, arena, &wider, &shorter, false); cmp >= 0 {
		t.Fatalf("stack compare(wider, shorter) = %d, want wider generated repeat boundary loser", cmp)
	}
}

func TestAcceptedStackSelectionDoesNotUseNameOnlyRepeatBoundary(t *testing.T) {
	parser, arena, shorter, wider := testGeneratedRepeatBoundaryStacks(t, false)

	if cmp := stackCompareForResultSelection(parser, arena, &shorter, &wider, false); cmp >= 0 {
		t.Fatalf("stack compare(shorter, wider) = %d, want name-only repeat to keep raw child order", cmp)
	}
	if cmp := stackCompareForResultSelection(parser, arena, &wider, &shorter, false); cmp <= 0 {
		t.Fatalf("stack compare(wider, shorter) = %d, want name-only repeat to keep raw child order", cmp)
	}
}

func testGeneratedRepeatBoundaryStacks(t *testing.T, generated bool) (*Parser, *nodeArena, glrStack, glrStack) {
	t.Helper()

	arena := acquireNodeArena(arenaClassFull)
	t.Cleanup(arena.Release)

	lang := &Language{
		Name:        "generated-repeat-boundary-test",
		SymbolNames: []string{"EOF", "root", "items_repeat1", "item"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "items_repeat1", Visible: false, Named: false, GeneratedRepeatAux: generated},
			{Name: "item", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: 1}

	first := newLeafNodeInArena(arena, 3, true, 94, 200, Point{Row: 4}, Point{Row: 5})
	second := newLeafNodeInArena(arena, 3, true, 300, 431, Point{Row: 8}, Point{Row: 11})
	tail := newLeafNodeInArena(arena, 3, true, 900, 1005, Point{Row: 20}, Point{Row: 24})

	shortRepeat := newParentNodeInArena(arena, 2, false, []*Node{first, second}, nil, 0)
	shortRepeat.rawShape = parser.captureRawShape(nil, arena, 2, 0, []stackEntry{
		newStackEntryNode(0, first),
		newStackEntryNode(0, second),
	}, 0, 2)

	wideRepeat := newParentNodeInArena(arena, 2, false, []*Node{shortRepeat, tail}, nil, 0)
	wideRepeat.rawShape = parser.captureRawShape(nil, arena, 2, 0, []stackEntry{
		newStackEntryNode(0, shortRepeat),
		newStackEntryNode(0, tail),
	}, 0, 2)

	shortRoot := newParentNodeInArena(arena, 1, true, []*Node{shortRepeat}, nil, 0)
	shortRoot.startByte = 0
	shortRoot.endByte = 1005
	shortRoot.startPoint = Point{}
	shortRoot.endPoint = Point{Row: 24}
	shortRoot.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, shortRepeat)}, 0, 1)

	wideRoot := newParentNodeInArena(arena, 1, true, []*Node{wideRepeat}, nil, 0)
	wideRoot.startByte = 0
	wideRoot.endByte = 1005
	wideRoot.startPoint = Point{}
	wideRoot.endPoint = Point{Row: 24}
	wideRoot.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, wideRepeat)}, 0, 1)

	shorter := glrStack{
		accepted:    true,
		byteOffset:  1005,
		branchOrder: 1,
		entries:     []stackEntry{newStackEntryNode(1, shortRoot)},
	}
	wider := glrStack{
		accepted:    true,
		byteOffset:  1005,
		branchOrder: 0,
		entries:     []stackEntry{newStackEntryNode(1, wideRoot)},
	}
	return parser, arena, shorter, wider
}

func TestAcceptedStackSelectionPrefersSharedPrefixSpliceBeforeRawShape(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	lang := &Language{
		Name:        "shared-prefix-raw-shape-test",
		SymbolNames: []string{"EOF", "root", "_wrap", "a", "b", "c_wrapped", "c_spliced"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "_wrap", Visible: false, Named: false},
			{Name: "a", Visible: true, Named: true},
			{Name: "b", Visible: true, Named: true},
			{Name: "c_wrapped", Visible: true, Named: true},
			{Name: "c_spliced", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: 1}

	wrappedA := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	wrappedB := newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	wrappedC := newLeafNodeInArena(arena, 5, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	wrap := newParentNodeInArena(arena, 2, false, []*Node{wrappedA, wrappedB, wrappedC}, nil, 0)
	wrappedRoot := newParentNodeInArena(arena, 1, true, []*Node{wrap}, nil, 0)

	splicedA := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	splicedB := newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	splicedC := newLeafNodeInArena(arena, 6, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	splicedRoot := newParentNodeInArena(arena, 1, true, []*Node{splicedA, splicedB, splicedC}, nil, 0)

	wrap.rawShape = parser.captureRawShape(nil, arena, 2, 0, []stackEntry{
		newStackEntryNode(0, wrappedA),
		newStackEntryNode(0, wrappedB),
		newStackEntryNode(0, wrappedC),
	}, 0, 3)
	wrappedRoot.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, wrap)}, 0, 1)
	splicedRoot.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{
		newStackEntryNode(0, splicedA),
		newStackEntryNode(0, splicedB),
		newStackEntryNode(0, splicedC),
	}, 0, 3)

	wrapped := glrStack{
		accepted:    true,
		byteOffset:  4,
		branchOrder: 0,
		entries:     []stackEntry{newStackEntryNode(1, wrappedRoot)},
	}
	spliced := glrStack{
		accepted:    true,
		byteOffset:  4,
		branchOrder: 1,
		entries:     []stackEntry{newStackEntryNode(1, splicedRoot)},
	}

	if cmp := compareAcceptedStackTreeOrderPreference(parser, arena, &wrapped, &spliced); cmp >= 0 {
		t.Fatalf("tree-order compare(wrapped, spliced) = %d, want wrapped loser", cmp)
	}
	if cmp := compareAcceptedStackTreeOrderPreference(parser, arena, &spliced, &wrapped); cmp <= 0 {
		t.Fatalf("tree-order compare(spliced, wrapped) = %d, want spliced winner", cmp)
	}
	if cmp := compareAcceptedStackRawShapePreference(parser, arena, &wrapped, &spliced); cmp <= 0 {
		t.Fatalf("raw-shape compare(wrapped, spliced) = %d, want wrapped raw shape winner before result-selection override", cmp)
	}
	if cmp := stackCompareForResultSelection(parser, arena, &spliced, &wrapped, false); cmp <= 0 {
		t.Fatalf("result compare(spliced, wrapped) = %d, want shared-prefix spliced winner", cmp)
	}
	if cmp := stackCompareForResultSelection(parser, arena, &wrapped, &spliced, false); cmp >= 0 {
		t.Fatalf("result compare(wrapped, spliced) = %d, want wrapped loser", cmp)
	}
}

func TestAcceptedStackSelectionDoesNotSpliceVisibleNamedStructuralContainer(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym    Symbol = 1
		repeatSym  Symbol = 2
		headerSym  Symbol = 3
		entrySym   Symbol = 4
		sectionSym Symbol = 5
		itemSym    Symbol = 6
	)
	lang := &Language{
		Name:        "visible-structural-container-splice-test",
		SymbolNames: []string{"EOF", "root", "root_repeat1", "header", "entry", "section", "item"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "root_repeat1", Visible: false, Named: false, GeneratedRepeatAux: true},
			{Name: "header", Visible: true, Named: true},
			{Name: "entry", Visible: true, Named: true},
			{Name: "section", Visible: true, Named: true},
			{Name: "item", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	wrappedHeader := newLeafNodeInArena(arena, headerSym, true, 0, 1, Point{}, Point{Column: 1})
	wrappedEntry := newLeafNodeInArena(arena, entrySym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	wrappedItem := newLeafNodeInArena(arena, itemSym, true, 2, 10, Point{Column: 2}, Point{Column: 10})
	section := newParentNodeInArena(arena, sectionSym, true, []*Node{wrappedItem}, nil, 0)
	repeat := newParentNodeInArena(arena, repeatSym, false, []*Node{wrappedHeader, wrappedEntry, section}, nil, 0)
	wrappedRoot := newParentNodeInArena(arena, rootSym, true, []*Node{repeat}, nil, 0)

	splicedHeader := newLeafNodeInArena(arena, headerSym, true, 0, 1, Point{}, Point{Column: 1})
	splicedEntry := newLeafNodeInArena(arena, entrySym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	splicedItem := newLeafNodeInArena(arena, itemSym, true, 2, 10, Point{Column: 2}, Point{Column: 10})
	splicedRoot := newParentNodeInArena(arena, rootSym, true, []*Node{splicedHeader, splicedEntry, splicedItem}, nil, 0)

	wrapped := glrStack{
		accepted:    true,
		byteOffset:  10,
		branchOrder: 0,
		entries:     []stackEntry{newStackEntryNode(1, wrappedRoot)},
	}
	spliced := glrStack{
		accepted:    true,
		byteOffset:  10,
		branchOrder: 1,
		entries:     []stackEntry{newStackEntryNode(1, splicedRoot)},
	}

	if cmp := compareAcceptedStackTreeOrderPreference(parser, arena, &wrapped, &spliced); cmp != 0 {
		t.Fatalf("tree-order compare(wrapped, spliced) = %d, want neutral when splice would remove section", cmp)
	}
	if cmp := compareAcceptedStackTreeOrderPreference(parser, arena, &spliced, &wrapped); cmp != 0 {
		t.Fatalf("tree-order compare(spliced, wrapped) = %d, want neutral when splice would remove section", cmp)
	}
	if cmp := stackCompareForResultSelection(parser, arena, &wrapped, &spliced, false); cmp <= 0 {
		t.Fatalf("result compare(wrapped, spliced) = %d, want earlier visible section container preserved", cmp)
	}
	if cmp := stackCompareForResultSelection(parser, arena, &spliced, &wrapped, false); cmp >= 0 {
		t.Fatalf("result compare(spliced, wrapped) = %d, want spliced root not to outrank visible section container", cmp)
	}
}

func TestForestAcceptedNodeSelectionUsesResultSemanticsAcrossAccepts(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym    Symbol = 1
		repeatSym  Symbol = 2
		headerSym  Symbol = 3
		entrySym   Symbol = 4
		sectionSym Symbol = 5
		itemSym    Symbol = 6
	)
	lang := &Language{
		Name:        "forest-accepted-final-container-test",
		SymbolNames: []string{"EOF", "root", "root_repeat1", "header", "entry", "section", "item"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "root_repeat1", Visible: false, Named: false, GeneratedRepeatAux: true},
			{Name: "header", Visible: true, Named: true},
			{Name: "entry", Visible: true, Named: true},
			{Name: "section", Visible: true, Named: true},
			{Name: "item", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	wrappedHeader := newLeafNodeInArena(arena, headerSym, true, 0, 1, Point{}, Point{Column: 1})
	wrappedEntry := newLeafNodeInArena(arena, entrySym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	wrappedItem := newLeafNodeInArena(arena, itemSym, true, 2, 10, Point{Column: 2}, Point{Column: 10})
	section := newParentNodeInArena(arena, sectionSym, true, []*Node{wrappedItem}, nil, 0)
	repeat := newParentNodeInArena(arena, repeatSym, false, []*Node{wrappedHeader, wrappedEntry, section}, nil, 0)
	wrappedRoot := newParentNodeInArena(arena, rootSym, true, []*Node{repeat}, nil, 0)

	splicedHeader := newLeafNodeInArena(arena, headerSym, true, 0, 1, Point{}, Point{Column: 1})
	splicedEntry := newLeafNodeInArena(arena, entrySym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	splicedItem := newLeafNodeInArena(arena, itemSym, true, 2, 10, Point{Column: 2}, Point{Column: 10})
	splicedRoot := newParentNodeInArena(arena, rootSym, true, []*Node{splicedHeader, splicedEntry, splicedItem}, nil, 0)

	wrappedAccept := &gssForestNode{
		state:      99,
		byteOffset: 10,
		links: []gssLink{
			{subtree: newStackEntryNode(99, wrappedRoot), score: 0, errorCost: 0},
		},
	}
	splicedAccept := &gssForestNode{
		state:      99,
		byteOffset: 10,
		links: []gssLink{
			{subtree: newStackEntryNode(99, splicedRoot), score: 0, errorCost: 0},
		},
	}

	if cmp := forestAcceptedNodeCompare(parser, arena, wrappedAccept, 0, splicedAccept, 1); cmp <= 0 {
		t.Fatalf("forest accepted compare(wrapped, spliced) = %d, want earlier visible section container preserved", cmp)
	}
	if cmp := forestAcceptedNodeCompare(parser, arena, splicedAccept, 1, wrappedAccept, 0); cmp >= 0 {
		t.Fatalf("forest accepted compare(spliced, wrapped) = %d, want later spliced accept to lose", cmp)
	}
}

func TestAcceptedStackTreeOrderPreservesVisibleNamedUnaryWrapper(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym  Symbol = 1
		guardSym Symbol = 2
		callSym  Symbol = 3
	)
	lang := &Language{
		Name:        "visible-unary-wrapper-selection-test",
		SymbolNames: []string{"EOF", "root", "guard_clause", "call"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "guard_clause", Visible: true, Named: true},
			{Name: "call", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	wrappedCall := newLeafNodeInArena(arena, callSym, true, 0, 13, Point{}, Point{Column: 13})
	guard := newParentNodeInArena(arena, guardSym, true, []*Node{wrappedCall}, nil, 0)
	wrappedRoot := newParentNodeInArena(arena, rootSym, true, []*Node{guard}, nil, 0)

	directCall := newLeafNodeInArena(arena, callSym, true, 0, 13, Point{}, Point{Column: 13})
	directRoot := newParentNodeInArena(arena, rootSym, true, []*Node{directCall}, nil, 0)

	wrapped := glrStack{
		accepted:    true,
		byteOffset:  13,
		branchOrder: 0,
		entries:     []stackEntry{newStackEntryNode(1, wrappedRoot)},
	}
	directStack := glrStack{
		accepted:    true,
		byteOffset:  13,
		branchOrder: 1,
		entries:     []stackEntry{newStackEntryNode(1, directRoot)},
	}

	if cmp := compareAcceptedStackTreeOrderPreference(parser, arena, &directStack, &wrapped); cmp != 0 {
		t.Fatalf("tree-order compare(direct, wrapped) = %d, want neutral for visible semantic wrapper", cmp)
	}
	if cmp := compareAcceptedStackTreeOrderPreference(parser, arena, &wrapped, &directStack); cmp != 0 {
		t.Fatalf("tree-order compare(wrapped, direct) = %d, want neutral for visible semantic wrapper", cmp)
	}
	if cmp := stackCompareForResultSelection(parser, arena, &wrapped, &directStack, false); cmp <= 0 {
		t.Fatalf("result compare(wrapped, direct) = %d, want earlier visible wrapper preserved", cmp)
	}
	if cmp := stackCompareForResultSelection(parser, arena, &directStack, &wrapped, false); cmp >= 0 {
		t.Fatalf("result compare(direct, wrapped) = %d, want direct child not to strip visible wrapper", cmp)
	}
}

func TestAcceptedStackTreeOrderPrefersDirectChildOverTransparentUnaryWrapperChain(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const (
		rootSym   Symbol = 1
		wrap1Sym  Symbol = 2
		wrap2Sym  Symbol = 3
		prefixSym Symbol = 4
		directSym Symbol = 5
	)
	lang := &Language{
		Name:        "transparent-direct-child-wrapper-selection-test",
		SymbolNames: []string{"EOF", "root", "_wrapper1", "_wrapper2", "prefix", "direct"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "_wrapper1", Visible: false, Named: false},
			{Name: "_wrapper2", Visible: false, Named: false},
			{Name: "prefix", Visible: true, Named: true},
			{Name: "direct", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rootSym}

	wrappedPrefix := newLeafNodeInArena(arena, prefixSym, true, 0, 1, Point{}, Point{Column: 1})
	wrappedDirect := newLeafNodeInArena(arena, directSym, true, 1, 4, Point{Column: 1}, Point{Column: 4})
	wrap2 := newParentNodeInArena(arena, wrap2Sym, true, []*Node{wrappedDirect}, nil, 0)
	wrap1 := newParentNodeInArena(arena, wrap1Sym, true, []*Node{wrap2}, nil, 0)
	wrappedRoot := newParentNodeInArena(arena, rootSym, true, []*Node{wrappedPrefix, wrap1}, nil, 0)

	directPrefix := newLeafNodeInArena(arena, prefixSym, true, 0, 1, Point{}, Point{Column: 1})
	direct := newLeafNodeInArena(arena, directSym, true, 1, 4, Point{Column: 1}, Point{Column: 4})
	directRoot := newParentNodeInArena(arena, rootSym, true, []*Node{directPrefix, direct}, nil, 0)

	wrapped := glrStack{
		accepted:    true,
		byteOffset:  4,
		branchOrder: 0,
		entries:     []stackEntry{newStackEntryNode(1, wrappedRoot)},
	}
	directStack := glrStack{
		accepted:    true,
		byteOffset:  4,
		branchOrder: 1,
		entries:     []stackEntry{newStackEntryNode(1, directRoot)},
	}

	if cmp := compareAcceptedStackTreeOrderPreference(parser, arena, &directStack, &wrapped); cmp <= 0 {
		t.Fatalf("tree-order compare(direct, wrapped) = %d, want direct child winner", cmp)
	}
	if cmp := stackCompareForResultSelection(parser, arena, &directStack, &wrapped, false); cmp <= 0 {
		t.Fatalf("result compare(direct, wrapped) = %d, want direct child winner before branch order", cmp)
	}
}

func TestAcceptedStackTreeOrderDoesNotStripSemanticInvisibleWrappers(t *testing.T) {
	tests := []struct {
		name      string
		configure func(lang *Language, arena *nodeArena, wrap *Node)
	}{
		{
			name: "direct-field-id",
			configure: func(lang *Language, arena *nodeArena, wrap *Node) {
				fieldIDs := cloneFieldIDSliceInArena(arena, []FieldID{1, 0, 0})
				wrap.setFieldMetadata(fieldIDs, defaultFieldSourcesInArena(arena, fieldIDs))
			},
		},
		{
			name: "effective-field-map",
			configure: func(lang *Language, arena *nodeArena, wrap *Node) {
				lang.FieldMapSlices = [][2]uint16{{0, 0}, {0, 1}}
				lang.FieldMapEntries = []FieldMapEntry{{FieldID: 1, ChildIndex: 0}}
			},
		},
		{
			name: "alias-sequence",
			configure: func(lang *Language, arena *nodeArena, wrap *Node) {
				lang.AliasSequences = [][]Symbol{
					nil,
					{0, 0, rawShapeSemanticAlias},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()

			parser, wrapped, spliced := testSemanticInvisibleWrapperRawShapeStacks(t, arena, tt.configure)

			if cmp := compareAcceptedStackTreeOrderPreference(parser, arena, &wrapped, &spliced); cmp != 0 {
				t.Fatalf("tree-order compare(wrapped, spliced) = %d, want neutral for semantic wrapper", cmp)
			}
			if cmp := compareAcceptedStackTreeOrderPreference(parser, arena, &spliced, &wrapped); cmp != 0 {
				t.Fatalf("tree-order compare(spliced, wrapped) = %d, want neutral for semantic wrapper", cmp)
			}
			if cmp := compareAcceptedStackRawShapePreference(parser, arena, &wrapped, &spliced); cmp <= 0 {
				t.Fatalf("raw-shape compare(wrapped, spliced) = %d, want semantic wrapper protected", cmp)
			}
			if cmp := stackCompareForResultSelection(parser, arena, &wrapped, &spliced, false); cmp <= 0 {
				t.Fatalf("result compare(wrapped, spliced) = %d, want raw shape to protect semantic wrapper", cmp)
			}
		})
	}
}

func testSemanticInvisibleWrapperRawShapeStacks(t *testing.T, arena *nodeArena, configure func(*Language, *nodeArena, *Node)) (*Parser, glrStack, glrStack) {
	t.Helper()

	lang := &Language{
		Name:        "semantic-wrapper-raw-shape-test",
		SymbolNames: []string{"EOF", "root", "_wrap", "a", "b", "c_wrapped", "c_spliced", "alias"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "root", Visible: true, Named: true},
			{Name: "_wrap", Visible: false, Named: false},
			{Name: "a", Visible: true, Named: true},
			{Name: "b", Visible: true, Named: true},
			{Name: "c_wrapped", Visible: true, Named: true},
			{Name: "c_spliced", Visible: true, Named: true},
			{Name: "alias", Visible: true, Named: true},
		},
		FieldNames: []string{"", "semantic"},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: rawShapeSemanticRootSym}

	wrappedA := newLeafNodeInArena(arena, rawShapeSemanticASym, true, 0, 1, Point{}, Point{Column: 1})
	wrappedB := newLeafNodeInArena(arena, rawShapeSemanticBSym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	wrappedC := newLeafNodeInArena(arena, rawShapeSemanticCWrap, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	wrap := newParentNodeInArena(arena, rawShapeSemanticWrapSym, false, []*Node{wrappedA, wrappedB, wrappedC}, nil, rawShapeSemanticWrapPID)
	wrappedRoot := newParentNodeInArena(arena, rawShapeSemanticRootSym, true, []*Node{wrap}, nil, 0)

	if configure != nil {
		configure(lang, arena, wrap)
	}

	splicedA := newLeafNodeInArena(arena, rawShapeSemanticASym, true, 0, 1, Point{}, Point{Column: 1})
	splicedB := newLeafNodeInArena(arena, rawShapeSemanticBSym, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	splicedC := newLeafNodeInArena(arena, rawShapeSemanticCSplice, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	splicedRoot := newParentNodeInArena(arena, rawShapeSemanticRootSym, true, []*Node{splicedA, splicedB, splicedC}, nil, 0)

	wrap.rawShape = parser.captureRawShape(nil, arena, rawShapeSemanticWrapSym, rawShapeSemanticWrapPID, []stackEntry{
		newStackEntryNode(0, wrappedA),
		newStackEntryNode(0, wrappedB),
		newStackEntryNode(0, wrappedC),
	}, 0, 3)
	wrappedRoot.rawShape = parser.captureRawShape(nil, arena, rawShapeSemanticRootSym, 0, []stackEntry{newStackEntryNode(0, wrap)}, 0, 1)
	splicedRoot.rawShape = parser.captureRawShape(nil, arena, rawShapeSemanticRootSym, 0, []stackEntry{
		newStackEntryNode(0, splicedA),
		newStackEntryNode(0, splicedB),
		newStackEntryNode(0, splicedC),
	}, 0, 3)

	wrapped := glrStack{
		accepted:    true,
		byteOffset:  4,
		branchOrder: 1,
		entries:     []stackEntry{newStackEntryNode(1, wrappedRoot)},
	}
	spliced := glrStack{
		accepted:    true,
		byteOffset:  4,
		branchOrder: 0,
		entries:     []stackEntry{newStackEntryNode(1, splicedRoot)},
	}
	return parser, wrapped, spliced
}

func TestDynamicPrecedencePropagatesThroughCompactMaterialization(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := testRawShapeParser()

	leaf := newCompactFullLeafInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	leaf.rawShape = parser.captureRawShape(nil, arena, 3, 0, []stackEntry{newStackEntryNode(0, newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1}))}, 0, 1)
	leaf.dynamicPrecedence = 11
	leafEntry := newStackEntryCompactFullLeaf(1, leaf)
	leafNode := materializeStackEntryCompactFullLeaf(arena, &leafEntry, compactFullLeafMaterializeForFinalTree)
	if leafNode == nil {
		t.Fatal("materialized compact leaf = nil")
	}
	if got := leafNode.dynamicPrecedence; got != 11 {
		t.Fatalf("compact leaf dynamic precedence = %d, want 11", got)
	}
	if got := leafNode.rawShape; got != leaf.rawShape {
		t.Fatalf("compact leaf raw shape = %d, want %d", got, leaf.rawShape)
	}

	parent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{leafEntry}, 0, 1, Point{}, Point{Column: 1}, false)
	parent.rawShape = parser.captureRawShape(nil, arena, 1, 0, []stackEntry{leafEntry}, 0, 1)
	parent.dynamicPrecedence = 13
	parentEntry := newStackEntryPendingParent(1, parent)
	parentNode := materializeStackEntryPendingParent(arena, &parentEntry, pendingParentMaterializeForFinalTree)
	if parentNode == nil {
		t.Fatal("materialized pending parent = nil")
	}
	if got := parentNode.dynamicPrecedence; got != 13 {
		t.Fatalf("pending parent dynamic precedence = %d, want 13", got)
	}
	if got := parentNode.rawShape; got != parent.rawShape {
		t.Fatalf("pending parent raw shape = %d, want %d", got, parent.rawShape)
	}
}

func TestDynamicPrecedencePropagatesThroughPendingFinalChildRefs(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	arena.finalChildRefs = true

	child := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	parent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{newStackEntryNode(1, child)}, 0, 1, Point{}, Point{Column: 1}, false)
	parent.rawShape = testRawShapeParser().captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(1, child)}, 0, 1)
	parent.dynamicPrecedence = 17
	parentEntry := newStackEntryPendingParent(1, parent)
	parentNode := materializeStackEntryPendingParent(arena, &parentEntry, pendingParentMaterializeForFinalTree)
	if parentNode == nil {
		t.Fatal("materialized pending final-child-ref parent = nil")
	}
	if got := parentNode.dynamicPrecedence; got != 17 {
		t.Fatalf("pending final-child-ref parent dynamic precedence = %d, want 17", got)
	}
	if got := parentNode.rawShape; got != parent.rawShape {
		t.Fatalf("pending final-child-ref parent raw shape = %d, want %d", got, parent.rawShape)
	}
	if got := len(parentNode.children); got != 0 {
		t.Fatalf("pending final-child-ref parent materialized children = %d, want 0", got)
	}
}

func TestPendingFinalChildRefsPreserveCompactAnonymousLeafChild(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	arena.finalChildRefs = true

	leaf := newCompactFullLeafInArena(arena, 3, false, 0, 1, Point{}, Point{Column: 1})
	leaf.parseState = 7
	parent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{newStackEntryCompactFullLeaf(7, leaf)}, 0, 1, Point{}, Point{Column: 1}, false)
	parentEntry := newStackEntryPendingParent(9, parent)

	parentNode := materializeStackEntryPendingParent(arena, &parentEntry, pendingParentMaterializeForFinalTree)
	if parentNode == nil {
		t.Fatal("materialized pending final-child-ref parent = nil")
	}
	if got, want := parentNode.ChildCount(), 1; got != want {
		t.Fatalf("pending parent child count = %d, want %d", got, want)
	}
	if got := len(parentNode.children); got != 0 {
		t.Fatalf("pending final-child-ref parent eagerly materialized children = %d, want 0", got)
	}
	child := parentNode.Child(0)
	if child == nil {
		t.Fatal("pending parent child[0] = nil")
	}
	if got, want := child.Symbol(), Symbol(3); got != want {
		t.Fatalf("child symbol = %d, want %d", got, want)
	}
	if child.IsNamed() {
		t.Fatal("anonymous compact leaf materialized as named")
	}
	if got, want := child.StartByte(), uint32(0); got != want {
		t.Fatalf("child start byte = %d, want %d", got, want)
	}
	if got, want := child.EndByte(), uint32(1); got != want {
		t.Fatalf("child end byte = %d, want %d", got, want)
	}
}

func TestPendingFinalChildRefsSpanVisibleExtraCompactLeaf(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	arena.finalChildRefs = true

	leaf := newCompactFullLeafInArena(arena, 3, true, 0, 15, Point{}, Point{Row: 1, Column: 6})
	leaf.setExtra(true)
	leaf.parseState = 7
	parent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{newStackEntryCompactFullLeaf(7, leaf)}, 0, 0, Point{}, Point{}, false)
	parentEntry := newStackEntryPendingParent(9, parent)

	parentNode := materializeStackEntryPendingParent(arena, &parentEntry, pendingParentMaterializeForFinalTree)
	if parentNode == nil {
		t.Fatal("materialized pending final-child-ref parent = nil")
	}
	if got, want := parentNode.ChildCount(), 1; got != want {
		t.Fatalf("pending parent child count = %d, want %d", got, want)
	}
	if got, want := parentNode.StartByte(), uint32(0); got != want {
		t.Fatalf("parent start byte = %d, want %d", got, want)
	}
	if got, want := parentNode.EndByte(), uint32(15); got != want {
		t.Fatalf("parent end byte = %d, want %d", got, want)
	}
	child := parentNode.Child(0)
	if child == nil {
		t.Fatal("pending parent child[0] = nil")
	}
	if !child.IsExtra() {
		t.Fatal("visible extra compact leaf did not materialize as extra")
	}
	if got, want := child.EndByte(), uint32(15); got != want {
		t.Fatalf("child end byte = %d, want %d", got, want)
	}
}

func TestDynamicPrecedencePropagatesThroughReduceAndSyntheticRoot(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	left := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	left.dynamicPrecedence = 2
	right := newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	right.dynamicPrecedence = 3
	entries := []stackEntry{newStackEntryNode(0, left), newStackEntryNode(0, right)}
	parent := newParentNodeInArena(arena, 1, true, []*Node{left, right}, nil, 0)
	setReduceNodeDynamicPrecedence(parent, entries, 0, len(entries), ParseAction{Symbol: 1, DynamicPrecedence: 5})
	if got := parent.dynamicPrecedence; got != 10 {
		t.Fatalf("reduce parent dynamic precedence = %d, want child sum plus action precedence 10", got)
	}

	root := newParentNodeInArena(arena, 1, true, []*Node{left, right}, nil, 0)
	if got := root.dynamicPrecedence; got != 5 {
		t.Fatalf("synthetic parent dynamic precedence = %d, want child sum 5", got)
	}
}

func TestRawShapePropagatesThroughNoTreeAndCollapsedUnary(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	child := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	entries := []stackEntry{newStackEntryNode(1, child)}
	reduced := newNoTreeReduceNodeInArena(arena, ParseAction{Symbol: 1, ProductionID: 7}, true, entries, 0, 1, Token{}, false)
	shape, ok := rawShapeForStackEntry(arena, newStackEntryNoTreeNode(1, reduced))
	if !ok {
		t.Fatal("no-tree reduce raw shape missing")
	}
	if got := shape.symbol; got != 1 {
		t.Fatalf("no-tree reduce raw symbol = %d, want 1", got)
	}

	entry := entries[0]
	raw := captureCollapsedUnaryRawShape(nil, arena, ParseAction{Symbol: 2, ProductionID: 9}, entries, 1)
	setStackEntryRawShapeRef(&entry, raw)
	shape, ok = rawShapeForStackEntry(arena, entry)
	if !ok {
		t.Fatal("collapsed unary raw shape missing")
	}
	if got := shape.symbol; got != 2 {
		t.Fatalf("collapsed unary raw symbol = %d, want 2", got)
	}
}

func TestRawShapeRebuiltAcceptRootUsesSplicedChildren(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	left := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	ref := captureRawShapeForNodeSlice(arena, 1, 0, []*Node{left, right})
	shape, ok := arena.rawShapeForRef(ref)
	if !ok {
		t.Fatal("rebuilt root raw shape missing")
	}
	if got := shape.childCount(); got != 2 {
		t.Fatalf("rebuilt root raw child count = %d, want 2", got)
	}
}

func TestRawShapeHashCacheUsesSlabIdentity(t *testing.T) {
	arena := acquireNodeArena(arenaClassIncremental)
	defer arena.Release()
	// Equal offsets from separate slabs must remain cached together.
	var refs [16]rawShapeRef
	for i := range refs {
		refs[i] = rawShapeRef((i + 1) << rawShapeRefIndexBits)
		arena.storeRawShapeHash(refs[i], uint64(i+1))
	}
	for i, ref := range refs {
		got, ok := arena.rawShapeHash(ref)
		if !ok || got != uint64(i+1) {
			t.Fatalf("slab %d cached hash = %d, %v; want %d, true", i, got, ok, i+1)
		}
	}
}
