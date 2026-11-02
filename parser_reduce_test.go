package gotreesitter

import (
	"fmt"
	"math/rand"
	"testing"
)

func buildTwoWindowFullGSSReduceCase(t *testing.T, scratch *gssScratch, arena *nodeArena) (*Parser, glrStack, ParseAction) {
	t.Helper()

	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNode(3, right), leftNode, 3)
	altRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightNode.appendExtraLink(gssMainLink{
		prev:  leftNode,
		entry: newStackEntryNode(3, altRight),
	})

	parser := &Parser{language: &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}}
	stack := glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2, DynamicPrecedence: 4}
	return parser, stack, act
}

func setSyntheticEOFAction(t *testing.T, parser *Parser, state StateID, actions []ParseAction) {
	t.Helper()
	if parser == nil || parser.language == nil {
		t.Fatal("parser/language must be initialized")
	}
	if parser.language.SymbolCount == 0 {
		parser.language.SymbolCount = 4
	}
	if parser.language.TokenCount == 0 {
		parser.language.TokenCount = 3
	}
	if parser.language.StateCount <= uint32(state) {
		parser.language.StateCount = uint32(state) + 1
	}
	for len(parser.language.ParseTable) <= int(state) {
		parser.language.ParseTable = append(parser.language.ParseTable, make([]uint16, parser.language.SymbolCount))
	}
	for i := range parser.language.ParseTable {
		if len(parser.language.ParseTable[i]) < int(parser.language.SymbolCount) {
			row := make([]uint16, parser.language.SymbolCount)
			copy(row, parser.language.ParseTable[i])
			parser.language.ParseTable[i] = row
		}
	}
	if len(parser.language.ParseActions) == 0 {
		parser.language.ParseActions = append(parser.language.ParseActions, ParseActionEntry{})
	}
	parser.language.ParseActions = append(parser.language.ParseActions, ParseActionEntry{Actions: actions})
	parser.language.ParseTable[state][0] = uint16(len(parser.language.ParseActions) - 1)
	parser.denseLimit = len(parser.language.ParseTable)
}

func setSyntheticPostReducePackingSafeEOF(t *testing.T, parser *Parser, states ...StateID) {
	t.Helper()
	for _, state := range states {
		setSyntheticEOFAction(t, parser, state, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1}})
	}
}

func cExactCompareLeaf(symbol Symbol) stackEntry {
	return newStackEntryNode(1, &Node{symbol: symbol, flags: nodeFlagNamed})
}

func cExactCompareParent(t *testing.T, parser *Parser, arena *nodeArena, symbol Symbol, children ...stackEntry) stackEntry {
	t.Helper()
	node := &Node{symbol: symbol, flags: nodeFlagNamed}
	node.rawShape = parser.captureRawShape(nil, arena, symbol, 0, children, 0, len(children))
	if node.rawShape == 0 {
		t.Fatal("raw parent shape was not captured")
	}
	return newStackEntryNode(1, node)
}

func TestCompareRawStackEntriesCExactUsesCPreorder(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	left := cExactCompareParent(t, parser, arena, 10,
		cExactCompareLeaf(2),
		cExactCompareLeaf(9),
	)
	right := cExactCompareParent(t, parser, arena, 10,
		cExactCompareLeaf(3),
		cExactCompareLeaf(1),
	)

	cmp, complete := compareRawStackEntriesCExact(arena, left, right, 8)
	if !complete || cmp != -1 {
		t.Fatalf("C exact comparison = %d, complete=%t; want -1, true", cmp, complete)
	}
	cmp, complete = compareRawStackEntriesCExact(arena, right, left, 8)
	if !complete || cmp != 1 {
		t.Fatalf("reverse C exact comparison = %d, complete=%t; want 1, true", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactUsesChildCountBeforeChildren(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	left := cExactCompareParent(t, parser, arena, 10, cExactCompareLeaf(9))
	right := cExactCompareParent(t, parser, arena, 10, cExactCompareLeaf(1), cExactCompareLeaf(1))

	cmp, complete := compareRawStackEntriesCExact(arena, left, right, 8)
	if !complete || cmp != -1 {
		t.Fatalf("C exact comparison = %d, complete=%t; want -1, true", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactIgnoresSpansAndHashShortcuts(t *testing.T) {
	leftNode := &Node{symbol: 7, startByte: 1, endByte: 2, flags: nodeFlagNamed}
	rightNode := &Node{symbol: 7, startByte: 20, endByte: 40, flags: nodeFlagNamed}
	cmp, complete := compareRawStackEntriesCExact(nil, newStackEntryNode(1, leftNode), newStackEntryNode(1, rightNode), 1)
	if !complete || cmp != 0 {
		t.Fatalf("span-only comparison = %d, complete=%t; want 0, true", cmp, complete)
	}

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	left := cExactCompareParent(t, parser, arena, 10, cExactCompareLeaf(2))
	right := cExactCompareParent(t, parser, arena, 10, cExactCompareLeaf(3))
	cmp, complete = compareRawStackEntriesCExact(arena, left, right, 4)
	if !complete || cmp != -1 {
		t.Fatalf("distinct raw children comparison = %d, complete=%t; want -1, true", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactUsesCapturedShapeSnapshot(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	sharedChild := &Node{symbol: 9, flags: nodeFlagNamed}
	sharedChild.rawShape = parser.captureRawShape(nil, arena, 9, 0, []stackEntry{cExactCompareLeaf(2)}, 0, 1)
	if sharedChild.rawShape == 0 {
		t.Fatal("first child shape was not captured")
	}
	left := cExactCompareParent(t, parser, arena, 10, newStackEntryNode(1, sharedChild))

	sharedChild.rawShape = parser.captureRawShape(nil, arena, 9, 0, []stackEntry{cExactCompareLeaf(3)}, 0, 1)
	if sharedChild.rawShape == 0 {
		t.Fatal("second child shape was not captured")
	}
	right := cExactCompareParent(t, parser, arena, 10, newStackEntryNode(1, sharedChild))

	cmp, complete := compareRawStackEntriesCExact(arena, left, right, 8)
	if !complete || cmp != -1 {
		t.Fatalf("captured-snapshot comparison = %d, complete=%t; want -1, true", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactSupportsZeroChildReceipts(t *testing.T) {
	left := cExactCompareLeaf(2)
	right := cExactCompareLeaf(3)
	stackEntryNode(left).rawShape = rawShapeZeroChildRef
	stackEntryNode(right).rawShape = rawShapeZeroChildRef
	if cmp, complete := compareRawStackEntriesCExact(nil, left, right, 1); !complete || cmp != -1 {
		t.Fatalf("zero-child receipt comparison = %d, complete=%t; want -1, true", cmp, complete)
	}

	shared := &Node{symbol: 9, flags: nodeFlagNamed, rawShape: rawShapeZeroChildRef}
	sharedEntry := newStackEntryNode(1, shared)
	if cmp, complete := compareRawStackEntriesCExact(nil, sharedEntry, sharedEntry, 1); !complete || cmp != 0 {
		t.Fatalf("shared zero-child receipt = %d, complete=%t; want 0, true", cmp, complete)
	}

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	leftParent := cExactCompareParent(t, parser, arena, 10, sharedEntry)
	rightParent := cExactCompareParent(t, parser, arena, 10, sharedEntry)
	if cmp, complete := compareRawStackEntriesCExact(arena, leftParent, rightParent, 3); !complete || cmp != 0 {
		t.Fatalf("captured zero-child receipt = %d, complete=%t; want 0, true", cmp, complete)
	}
}

func TestCompareRawStackEntriesCapturedZeroUsesLiveChildrenWithoutMutation(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	leftChild := &Node{
		symbol:   9,
		flags:    nodeFlagNamed,
		children: []*Node{{symbol: 2, flags: nodeFlagNamed}},
	}
	rightChild := &Node{
		symbol:   9,
		flags:    nodeFlagNamed,
		children: []*Node{{symbol: 3, flags: nodeFlagNamed}},
	}
	left := cExactCompareParent(t, parser, arena, 10, newStackEntryNode(1, leftChild))
	right := cExactCompareParent(t, parser, arena, 10, newStackEntryNode(1, rightChild))

	if cmp := parser.compareRawStackEntries(arena, left, right); cmp != -1 {
		t.Fatalf("ordinary captured-zero comparison = %d, want -1", cmp)
	}
	if leftChild.rawShape != 0 || rightChild.rawShape != 0 {
		t.Fatalf("ordinary comparison mutated child refs to %d/%d", leftChild.rawShape, rightChild.rawShape)
	}
}

func TestCompareRawStackEntriesCExactHasNoDepthCutoff(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	left := cExactCompareLeaf(2)
	right := cExactCompareLeaf(3)
	for i := 0; i < maxTreeWalkDepth+16; i++ {
		left = cExactCompareParent(t, parser, arena, 10, left)
		right = cExactCompareParent(t, parser, arena, 10, right)
	}

	cmp, complete := compareRawStackEntriesCExact(arena, left, right, 2*(maxTreeWalkDepth+16)+4)
	if !complete || cmp != -1 {
		t.Fatalf("deep C exact comparison = %d, complete=%t; want -1, true", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactFailsClosedWithoutRawTopology(t *testing.T) {
	child := &Node{symbol: 2, flags: nodeFlagNamed}
	shared := &Node{symbol: 10, flags: nodeFlagNamed, children: []*Node{child}}
	sharedLeft := newStackEntryNode(1, shared)
	sharedRight := newStackEntryNode(9, shared)
	if cmp, complete := compareRawStackEntriesCExact(nil, sharedLeft, sharedRight, 1); !complete || cmp != 0 {
		t.Fatalf("shared shapeless comparison = %d, complete=%t; want 0, true", cmp, complete)
	}

	left := newStackEntryNode(1, &Node{symbol: 10, flags: nodeFlagNamed, children: []*Node{child}})
	right := newStackEntryNode(1, &Node{symbol: 10, flags: nodeFlagNamed, children: []*Node{child}})
	if cmp, complete := compareRawStackEntriesCExact(nil, left, right, 8); complete || cmp != 0 {
		t.Fatalf("distinct shapeless comparison = %d, complete=%t; want 0, false", cmp, complete)
	}

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	shaped := cExactCompareParent(t, parser, arena, 10, cExactCompareLeaf(2))
	if cmp, complete := compareRawStackEntriesCExact(arena, shaped, right, 8); complete || cmp != 0 {
		t.Fatalf("one-sided topology comparison = %d, complete=%t; want 0, false", cmp, complete)
	}

	invalid := &Node{symbol: 10, flags: nodeFlagNamed, rawShape: ^rawShapeRef(0)}
	if cmp, complete := compareRawStackEntriesCExact(nil, newStackEntryNode(1, invalid), cExactCompareLeaf(10), 8); complete || cmp != 0 {
		t.Fatalf("invalid-shape comparison = %d, complete=%t; want 0, false", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactFailsClosedAtWorkLimit(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	left := cExactCompareParent(t, parser, arena, 10, cExactCompareLeaf(2), cExactCompareLeaf(3))
	right := cExactCompareParent(t, parser, arena, 10, cExactCompareLeaf(2), cExactCompareLeaf(3))

	cmp, complete := compareRawStackEntriesCExact(arena, left, right, 2)
	if complete || cmp != 0 {
		t.Fatalf("bounded C exact comparison = %d, complete=%t; want 0, false", cmp, complete)
	}
	cmp, complete = compareRawStackEntriesCExact(arena, left, right, 3)
	if !complete || cmp != 0 {
		t.Fatalf("exact-bound C comparison = %d, complete=%t; want 0, true", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactRejectsForwardShapeReference(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	leftChild := cExactCompareParent(t, parser, arena, 9, cExactCompareLeaf(2))
	left := cExactCompareParent(t, parser, arena, 10, leftChild)
	rightChild := cExactCompareParent(t, parser, arena, 9, cExactCompareLeaf(2))
	right := cExactCompareParent(t, parser, arena, 10, rightChild)

	leftNode := stackEntryNode(left)
	leftShape, ok := arena.rawShapeForRef(leftNode.rawShape)
	if !ok {
		t.Fatal("left root shape is unavailable")
	}
	children := arena.rawShapeChildren(leftShape)
	if len(children) != 1 {
		t.Fatalf("left root child count = %d, want 1", len(children))
	}
	children[0].packedEntry.state = StateID(leftNode.rawShape)

	cmp, complete := compareRawStackEntriesCExact(arena, left, right, 8)
	if complete || cmp != 0 {
		t.Fatalf("forward-reference comparison = %d, complete=%t; want 0, false", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactRejectsMalformedSharedSnapshot(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	shared := &Node{symbol: 9, flags: nodeFlagNamed, rawShape: ^rawShapeRef(0)}
	left := cExactCompareParent(t, parser, arena, 10, newStackEntryNode(1, shared))
	right := cExactCompareParent(t, parser, arena, 10, newStackEntryNode(1, shared))

	cmp, complete := compareRawStackEntriesCExact(arena, left, right, 8)
	if complete || cmp != 0 {
		t.Fatalf("shared malformed snapshot comparison = %d, complete=%t; want 0, false", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactRejectsMalformedChildRange(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	left := cExactCompareParent(t, parser, arena, 10, cExactCompareLeaf(2))
	right := cExactCompareParent(t, parser, arena, 10, cExactCompareLeaf(2))
	leftShape, ok := arena.rawShapeForRef(stackEntryNode(left).rawShape)
	if !ok {
		t.Fatal("left root shape is unavailable")
	}
	leftShape.childRange = rawShapeChildRange((uint64(1) << 48) | (uint64(^uint32(0)) << 16) | 1)

	cmp, complete := compareRawStackEntriesCExact(arena, left, right, 8)
	if complete || cmp != 0 {
		t.Fatalf("malformed child-range comparison = %d, complete=%t; want 0, false", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactRejectsCapturedZeroForSharedNonLeaf(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{}
	shared := &Node{
		symbol:   9,
		flags:    nodeFlagNamed,
		children: []*Node{{symbol: 2, flags: nodeFlagNamed}},
	}
	left := cExactCompareParent(t, parser, arena, 10, newStackEntryNode(1, shared))
	right := cExactCompareParent(t, parser, arena, 10, newStackEntryNode(1, shared))

	cmp, complete := compareRawStackEntriesCExact(arena, left, right, 8)
	if complete || cmp != 0 {
		t.Fatalf("captured-zero shared non-leaf comparison = %d, complete=%t; want 0, false", cmp, complete)
	}
}

func TestCompareRawStackEntriesCExactRejectsUnsupportedSharedPayload(t *testing.T) {
	entry := cExactCompareLeaf(7)
	entry.kind = ^uint32(0)
	cmp, complete := compareRawStackEntriesCExact(nil, entry, entry, 1)
	if complete || cmp != 0 {
		t.Fatalf("unsupported shared payload comparison = %d, complete=%t; want 0, false", cmp, complete)
	}

	left := cExactCompareLeaf(7)
	right := cExactCompareLeaf(7)
	left.kind = ^uint32(0)
	right.kind = ^uint32(0)
	cmp, complete = compareRawStackEntriesCExact(nil, left, right, 1)
	if complete || cmp != 0 {
		t.Fatalf("unsupported distinct payload comparison = %d, complete=%t; want 0, false", cmp, complete)
	}
}

func TestFastVisibleReduceFromGSSDeclinesMultiLinkSpan(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})
	right := NewLeafNode(2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNode(3, right), leftNode, 3)
	altRight := NewLeafNode(2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightNode.appendExtraLink(gssMainLink{
		prev:  leftNode,
		entry: newStackEntryNode(3, altRight),
	})

	parser := &Parser{language: &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}}
	stack := &glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}
	var anyReduced bool
	nodeCount := 0
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	if parser.tryFastVisibleReduceActionFromGSS(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, false, false) {
		t.Fatal("tryFastVisibleReduceActionFromGSS = true, want false for multi-link span")
	}
	if anyReduced {
		t.Fatal("anyReduced = true, want false")
	}
	if nodeCount != 0 {
		t.Fatalf("nodeCount = %d, want 0", nodeCount)
	}
	if stack.gss.head != rightNode {
		t.Fatal("stack mutated by declined fast reduce")
	}
}

func TestFastUnaryCollapseFromGSSReplacesLeafReceipt(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	language := &Language{
		CompactPackedGSSVersionOrderCertified: true,
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: false},
			{Name: "child", Visible: true, Named: true},
		},
	}
	merge := &glrMergeScratch{
		language:                    language,
		packedGSSVersionOrderActive: true,
		arena:                       arena,
	}
	parser := &Parser{language: language, mergeScratch: merge}
	merge.parser = parser
	var gss gssScratch
	base := gss.allocNode(stackEntry{state: 1}, nil, 1)
	child := newLeafNodeInArena(arena, 1, true, 0, 2, Point{}, Point{Column: 2})
	head := gss.allocNode(newStackEntryNode(2, child), base, 2)
	stack := &glrStack{gss: gssStack{head: head}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 1, ChildCount: 1}
	var anyReduced bool

	if !parser.tryFastUnaryCollapseFromGSS(stack, act, Token{}, &anyReduced, arena, nil, &gss, nil) {
		t.Fatal("fast unary collapse declined")
	}
	if !anyReduced {
		t.Fatal("fast unary collapse did not mark the reduction")
	}
	collapsed := stackEntryNode(stack.gss.head.entry)
	shape, ok := arena.rawShapeForRef(collapsed.rawShape)
	if !ok || shape.symbol != act.Symbol || shape.childCount() != 1 {
		t.Fatalf("collapsed raw header = (%v, %t), want symbol %d and child count 1", shape, ok, act.Symbol)
	}

	leaf := newLeafNodeInArena(arena, 1, true, 0, 2, Point{}, Point{Column: 2})
	if cStackLinkPayloadsEquivalentAtOffsets(merge, stack.gss.head.entry, newStackEntryNode(2, leaf), 0, true, 0, true) {
		t.Fatal("collapsed unary compared equal to a true leaf")
	}
}

func TestFastUnaryCollapseFromGSSClearsReceiptOutsideCertifiedRoute(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	language := &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: false},
		{Name: "child", Visible: true, Named: true},
	}}
	parser := &Parser{language: language}
	var gss gssScratch
	base := gss.allocNode(stackEntry{state: 1}, nil, 1)
	child := newLeafNodeInArena(arena, 1, true, 0, 2, Point{}, Point{Column: 2})
	ref, shape := arena.allocRawShape()
	if ref == 0 || shape == nil {
		t.Fatal("raw-shape setup failed")
	}
	shape.symbol = 99
	child.rawShape = ref
	head := gss.allocNode(newStackEntryNode(2, child), base, 2)
	stack := &glrStack{gss: gssStack{head: head}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 1, ChildCount: 1}
	var anyReduced bool

	if !parser.tryFastUnaryCollapseFromGSS(stack, act, Token{}, &anyReduced, arena, nil, &gss, nil) {
		t.Fatal("fast unary collapse declined")
	}
	if got := stackEntryRawShapeRef(stack.gss.head.entry); got != 0 {
		t.Fatalf("ordinary collapsed receipt = %d, want unknown", got)
	}
}

func TestFastVisibleTransientReduceFromGSSCapturesRawShapeAndPrecedence(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	language := &Language{CompactPackedGSSVersionOrderCertified: true, SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: false},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}
	merge := &glrMergeScratch{
		language:                    language,
		packedGSSVersionOrderActive: true,
		arena:                       arena,
	}
	parser := &Parser{language: language, mergeScratch: merge}
	merge.parser = parser
	var gss gssScratch
	base := gss.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	left.dynamicPrecedence = 2
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	right.dynamicPrecedence = 3
	leftHead := gss.allocNode(newStackEntryNode(2, left), base, 2)
	rightHead := gss.allocNode(newStackEntryNode(3, right), leftHead, 3)
	stack := &glrStack{gss: gssStack{head: rightHead}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2, DynamicPrecedence: 4}
	var anyReduced bool
	nodeCount := 0

	if !parser.tryFastVisibleReduceActionFromGSSTransientParents(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &gss, nil, true, false) {
		t.Fatal("fast visible reduce declined")
	}
	parent := stackEntryNode(stack.gss.head.entry)
	shape, ok := arena.rawShapeForRef(parent.rawShape)
	if !ok || shape.symbol != act.Symbol || shape.childCount() != 2 {
		t.Fatalf("parent raw header = (%v, %t), want symbol %d and child count 2", shape, ok, act.Symbol)
	}
	if parent.dynamicPrecedence != 9 {
		t.Fatalf("parent dynamic precedence = %d, want 9", parent.dynamicPrecedence)
	}
}

func TestFaithfulForkReduceFromGSSLinkedWindowsCoalescesPostReduceHead(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })
	if perfCountersEnabled {
		ResetPerfCounters()
	}

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	parser, stack, act := buildTwoWindowFullGSSReduceCase(t, &scratch, arena)
	setSyntheticPostReducePackingSafeEOF(t, parser, 1)
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if !anyReduced {
		t.Fatal("anyReduced = false, want true")
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want 1", nodeCount)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0", len(parser.pendingForkStacks))
	}
	if stack.gss.head == nil {
		t.Fatal("post-reduce GSS head is nil")
	}
	if got := stack.gss.head.linkCount(); got != 1 {
		t.Fatalf("post-reduce head linkCount = %d, want 1", got)
	}
	if perfCountersEnabled {
		perf := PerfCountersSnapshot()
		if perf.ReduceForkCalls != 1 || perf.ReduceForkWindows != 2 || perf.ReduceForkMaxWindows != 2 {
			t.Fatalf("reduce fork counters = calls:%d windows:%d max:%d, want 1/2/2", perf.ReduceForkCalls, perf.ReduceForkWindows, perf.ReduceForkMaxWindows)
		}
		if perf.PostReduceMergeAttempts != 0 || perf.PostReduceMergePrimarySuccesses != 0 || perf.PendingForkStackAppends != 0 {
			t.Fatalf("post-reduce merge counters = attempts:%d primary:%d appends:%d, want 0/0/0", perf.PostReduceMergeAttempts, perf.PostReduceMergePrimarySuccesses, perf.PendingForkStackAppends)
		}
	}
}

func TestFaithfulForkReduceImmediateAcceptPacksFinalizerVisibleFork(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })
	if perfCountersEnabled {
		ResetPerfCounters()
	}

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	parser, stack, act := buildTwoWindowFullGSSReduceCase(t, &scratch, arena)
	setSyntheticEOFAction(t, parser, 1, []ParseAction{{Type: ParseActionAccept}})
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if !anyReduced {
		t.Fatal("anyReduced = false, want true")
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want 1", nodeCount)
	}
	if got := stack.gss.head.linkCount(); got != 1 {
		t.Fatalf("primary post-reduce head linkCount = %d, want 1", got)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0", len(parser.pendingForkStacks))
	}
	if perfCountersEnabled {
		perf := PerfCountersSnapshot()
		if perf.ReduceForkCalls != 1 || perf.ReduceForkWindows != 2 || perf.ReduceForkMaxWindows != 2 {
			t.Fatalf("reduce fork counters = calls:%d windows:%d max:%d, want 1/2/2", perf.ReduceForkCalls, perf.ReduceForkWindows, perf.ReduceForkMaxWindows)
		}
		if perf.PostReduceMergeFinalizationRiskSkips != 0 || perf.PostReduceMergeAttempts != 0 || perf.PostReduceMergePrimarySuccesses != 0 || perf.PendingForkStackAppends != 0 {
			t.Fatalf("accept merge counters = skips:%d attempts:%d primary:%d appends:%d, want 0/0/0/0", perf.PostReduceMergeFinalizationRiskSkips, perf.PostReduceMergeAttempts, perf.PostReduceMergePrimarySuccesses, perf.PendingForkStackAppends)
		}
	}
}

func TestFaithfulForkReduceFromPackedGSSHeadEnumeratesReducedParents(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	parser, stack, act := buildTwoWindowFullGSSReduceCase(t, &scratch, arena)
	setSyntheticPostReducePackingSafeEOF(t, parser, 1)
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	forks := reduceWindowsFromGSS(&stack, 1, maxStacksPerMergeKey)
	if len(forks) != 1 {
		t.Fatalf("reduced-parent windows = %d, want 1", len(forks))
	}
	for i, fork := range forks {
		if len(fork.window) != 1 {
			t.Fatalf("fork %d window length = %d, want 1", i, len(fork.window))
		}
		parent := stackEntryNode(fork.window[0])
		if parent == nil || parent.symbol != act.Symbol {
			t.Fatalf("fork %d parent symbol = %v, want %d", i, parent, act.Symbol)
		}
	}
}

func TestCWaveReduceWindowsFromGSSUsesPhysicalIteratorWaveOrder(t *testing.T) {
	entry := func(state StateID, symbol Symbol) stackEntry {
		return newStackEntryNode(state, &Node{symbol: symbol, flags: nodeFlagNamed})
	}
	base := func(state StateID) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, depth: 1}
	}

	baseA0 := base(100)
	baseA1 := base(101)
	baseB0 := base(200)
	baseB1 := base(201)
	leftA := gssNodeWithExtraLinks(gssNode{
		entry: entry(10, 10),
		prev:  baseA0,
		depth: 2,
	}, gssMainLink{prev: baseA1, entry: entry(11, 11)})
	leftB := gssNodeWithExtraLinks(gssNode{
		entry: entry(20, 20),
		prev:  baseB0,
		depth: 2,
	}, gssMainLink{prev: baseB1, entry: entry(21, 21)})
	head := gssNodeWithExtraLinks(gssNode{
		entry: entry(30, 30),
		prev:  leftA,
		depth: 3,
	}, gssMainLink{prev: leftB, entry: entry(31, 31)})

	forks := cWaveReduceWindowsFromGSS(&glrStack{gss: gssStack{head: head}}, 2)
	if len(forks) != 4 {
		t.Fatalf("C-wave windows = %d, want 4", len(forks))
	}
	wantSymbols := [][2]Symbol{{10, 30}, {20, 31}, {11, 30}, {21, 31}}
	wantStates := []StateID{100, 200, 101, 201}
	for i, fork := range forks {
		if len(fork.window) != 2 {
			t.Fatalf("window %d length = %d, want 2", i, len(fork.window))
		}
		gotSymbols := [2]Symbol{
			stackEntryNodeSymbol(fork.window[0]),
			stackEntryNodeSymbol(fork.window[1]),
		}
		if gotSymbols != wantSymbols[i] || fork.topState != wantStates[i] {
			t.Fatalf("window %d = symbols:%v state:%d, want symbols:%v state:%d", i, gotSymbols, fork.topState, wantSymbols[i], wantStates[i])
		}
	}
}

func TestCWaveReduceWindowsFromGSSGroupsRepeatedPopTargetsLikeStackSlices(t *testing.T) {
	entry := func(state StateID, symbol Symbol) stackEntry {
		return newStackEntryNode(state, &Node{symbol: symbol, flags: nodeFlagNamed})
	}
	base := func(state StateID) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, depth: 1}
	}

	sharedBase := base(100)
	baseB0 := base(200)
	baseB1 := base(201)
	leftA := gssNodeWithExtraLinks(gssNode{
		entry: entry(10, 10),
		prev:  sharedBase,
		depth: 2,
	}, gssMainLink{prev: sharedBase, entry: entry(11, 11)})
	leftB := gssNodeWithExtraLinks(gssNode{
		entry: entry(20, 20),
		prev:  baseB0,
		depth: 2,
	}, gssMainLink{prev: baseB1, entry: entry(21, 21)})
	head := gssNodeWithExtraLinks(gssNode{
		entry: entry(30, 30),
		prev:  leftA,
		depth: 3,
	}, gssMainLink{prev: leftB, entry: entry(31, 31)})

	forks := cWaveReduceWindowsFromGSS(&glrStack{gss: gssStack{head: head}}, 2)
	wantSymbols := [][2]Symbol{{10, 30}, {11, 30}, {20, 31}, {21, 31}}
	wantTargets := []*gssNode{sharedBase, sharedBase, baseB0, baseB1}
	if len(forks) != len(wantSymbols) {
		t.Fatalf("C-wave windows = %d, want %d", len(forks), len(wantSymbols))
	}
	for i, fork := range forks {
		gotSymbols := [2]Symbol{
			stackEntryNodeSymbol(fork.window[0]),
			stackEntryNodeSymbol(fork.window[1]),
		}
		if gotSymbols != wantSymbols[i] || fork.popTo != wantTargets[i] {
			t.Fatalf("window %d = symbols:%v target:%p, want symbols:%v target:%p", i, gotSymbols, fork.popTo, wantSymbols[i], wantTargets[i])
		}
	}
}

func TestCWaveReduceWindowsFromGSSStableRemovalPreservesLaterWaveOrder(t *testing.T) {
	entry := func(state StateID, symbol Symbol, extra bool) stackEntry {
		node := &Node{symbol: symbol, flags: nodeFlagNamed}
		node.setExtra(extra)
		return newStackEntryNode(state, node)
	}
	base := func(state StateID) *gssNode {
		return &gssNode{entry: stackEntry{state: state}, depth: 1}
	}
	base0 := base(100)
	baseA0 := base(200)
	baseA1 := base(201)
	baseB0 := base(300)
	baseB1 := base(301)
	branchA := gssNodeWithExtraLinks(gssNode{
		entry: entry(20, 20, false),
		prev:  baseA0,
		depth: 2,
	}, gssMainLink{prev: baseA1, entry: entry(21, 21, false)})
	branchB := gssNodeWithExtraLinks(gssNode{
		entry: entry(30, 30, false),
		prev:  baseB0,
		depth: 2,
	}, gssMainLink{prev: baseB1, entry: entry(31, 31, false)})
	head := gssNodeWithExtraLinks(gssNode{
		entry: entry(10, 10, false),
		prev:  base0,
		depth: 3,
	},
		gssMainLink{prev: branchA, entry: entry(11, 11, true)},
		gssMainLink{prev: branchB, entry: entry(12, 12, true)},
	)

	forks := cWaveReduceWindowsFromGSS(&glrStack{gss: gssStack{head: head}}, 1)
	wantSymbols := [][]Symbol{{10}, {20, 11}, {30, 12}, {21, 11}, {31, 12}}
	wantStates := []StateID{100, 200, 300, 201, 301}
	if len(forks) != len(wantSymbols) {
		t.Fatalf("C-wave windows = %d, want %d", len(forks), len(wantSymbols))
	}
	for i, fork := range forks {
		if len(fork.window) != len(wantSymbols[i]) {
			t.Fatalf("window %d length = %d, want %d", i, len(fork.window), len(wantSymbols[i]))
		}
		for j, want := range wantSymbols[i] {
			if got := stackEntryNodeSymbol(fork.window[j]); got != want {
				t.Fatalf("window %d symbol %d = %d, want %d", i, j, got, want)
			}
		}
		if fork.topState != wantStates[i] {
			t.Fatalf("window %d top state = %d, want %d", i, fork.topState, wantStates[i])
		}
	}
}

func TestCWaveReduceWindowsFromGSSCapsLiveIteratorsAtCMaximum(t *testing.T) {
	entry := func(state StateID, symbol Symbol) stackEntry {
		return newStackEntryNode(state, &Node{symbol: symbol, flags: nodeFlagNamed})
	}
	base := &gssNode{entry: stackEntry{state: 7}, depth: 1}
	lowest := &gssNode{entry: entry(10, 1), prev: base, depth: 2}
	for i := 1; i < maxMainLinkCount; i++ {
		lowest.appendExtraLink(gssMainLink{prev: base, entry: entry(StateID(10+i), Symbol(1+i))})
	}
	middle := &gssNode{entry: entry(30, 20), prev: lowest, depth: 3}
	for i := 1; i < maxMainLinkCount; i++ {
		middle.appendExtraLink(gssMainLink{prev: lowest, entry: entry(StateID(30+i), Symbol(20+i))})
	}
	head := &gssNode{entry: entry(50, 40), prev: middle, depth: 4}
	for i := 1; i < maxMainLinkCount; i++ {
		head.appendExtraLink(gssMainLink{prev: middle, entry: entry(StateID(50+i), Symbol(40+i))})
	}

	forks := cWaveReduceWindowsFromGSS(&glrStack{gss: gssStack{head: head}}, 3)
	if len(forks) != cStackMaxIteratorCount {
		t.Fatalf("C-wave windows = %d, want iterator cap %d", len(forks), cStackMaxIteratorCount)
	}
	mainWindowCount := maxMainLinkCount * maxMainLinkCount
	for i, fork := range forks {
		if len(fork.window) != 3 {
			t.Fatalf("window %d length = %d, want 3", i, len(fork.window))
		}
		wantLowest := Symbol(1)
		if i >= mainWindowCount {
			wantLowest = Symbol(2 + (i-mainWindowCount)%(maxMainLinkCount-1))
		}
		if got := stackEntryNodeSymbol(fork.window[0]); got != wantLowest {
			t.Fatalf("window %d lowest-link symbol = %d, want %d from C cap order", i, got, wantLowest)
		}
		if fork.popTo != base || fork.topState != 7 {
			t.Fatalf("window %d pop target = %p state=%d, want %p state=7", i, fork.popTo, fork.topState, base)
		}
	}
}

// Native pop-all stops each iterator at the base, even when a packed
// alternative reaches it earlier than the primary path.
func TestCWavePopAllPreservesPathsWithDifferentDepths(t *testing.T) {
	entry := func(symbol Symbol) stackEntry {
		return newStackEntryNode(2, &Node{symbol: symbol})
	}
	base := &gssNode{entry: stackEntry{state: 1}, depth: 1}
	left := &gssNode{entry: entry(10), prev: base, depth: 2}
	head := gssNodeWithExtraLinks(gssNode{entry: entry(20), prev: left, depth: 3},
		gssMainLink{entry: entry(30), prev: base})
	slices := cWaveReduceWindowsFromGSS(&glrStack{gss: gssStack{head: head}}, -1)
	if len(slices) != 2 {
		t.Fatalf("pop-all slices = %d, want 2", len(slices))
	}
	want := [][]Symbol{{30}, {10, 20}}
	for i, slice := range slices {
		if slice.popTo != base || len(slice.window) != len(want[i]) {
			t.Fatalf("slice %d has wrong pop target or length", i)
		}
		for j, child := range slice.window {
			if symbol := stackEntryNodeSymbol(child); symbol != want[i][j] {
				t.Fatalf("slice %d child %d = %d, want %d", i, j, symbol, want[i][j])
			}
		}
	}
}

func TestCWaveReduceWindowsFromGSSZeroCountReturnsTheCurrentHead(t *testing.T) {
	head := &gssNode{entry: stackEntry{state: 9}, depth: 1}
	forks := cWaveReduceWindowsFromGSS(&glrStack{gss: gssStack{head: head}}, 0)
	if len(forks) != 1 || len(forks[0].window) != 0 || forks[0].popTo != head || forks[0].topState != 9 {
		t.Fatalf("zero-count C-wave windows = %+v, want one empty window at the current head", forks)
	}
}

func TestCWaveReduceWindowsFromGSSCountsInternalNullSubtreeButNotBase(t *testing.T) {
	base := &gssNode{entry: stackEntry{state: 1}, depth: 1}
	stateOnly := &gssNode{
		entry: stackEntry{state: 2},
		prev:  base,
		depth: 2,
	}
	head := &gssNode{
		entry: newStackEntryNode(3, &Node{symbol: 7, flags: nodeFlagNamed}),
		prev:  stateOnly,
		depth: 3,
	}

	forks := cWaveReduceWindowsFromGSS(&glrStack{gss: gssStack{head: head}}, 2)
	if len(forks) != 1 {
		t.Fatalf("C-wave windows = %d, want 1", len(forks))
	}
	if len(forks[0].window) != 2 || stackEntryHasNode(forks[0].window[0]) || stackEntryNodeSymbol(forks[0].window[1]) != 7 {
		t.Fatalf("C-wave window = %+v, want internal null subtree then symbol 7", forks[0].window)
	}
	if forks[0].popTo != base || forks[0].topState != 1 {
		t.Fatalf("C-wave pop target = %p state=%d, want %p state=1", forks[0].popTo, forks[0].topState, base)
	}
	if got := cWaveReduceWindowsFromGSS(&glrStack{gss: gssStack{head: base}}, 1); len(got) != 0 {
		t.Fatalf("linkless base produced %d windows, want 0", len(got))
	}
}

func TestFaithfulGSSMergeRecursesPredecessorLinksAndReduceSelectsConstructedParent(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	leftLow := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftHigh := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
	leftHigh.dynamicPrecedence = 9
	leftLowNode := scratch.allocNode(newStackEntryNode(2, leftLow), base, 2)
	leftHighNode := scratch.allocNode(newStackEntryNode(2, leftHigh), base, 2)
	rightLow := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightHigh := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	lowHead := scratch.allocNode(newStackEntryNode(3, rightLow), leftLowNode, 3)
	highHead := scratch.allocNode(newStackEntryNode(3, rightHigh), leftHighNode, 3)

	low := glrStack{gss: gssStack{head: lowHead}, byteOffset: 2}
	high := glrStack{gss: gssStack{head: highHead}, byteOffset: 2}
	if !gssMainCanMerge(&low, &high) {
		t.Fatal("clean same-state synthetic stacks should be mergeable")
	}
	if !gssMainMerge(&low, &high) {
		t.Fatal("gssMainMerge returned false")
	}
	if got := low.gss.head.linkCount(); got != 1 {
		t.Fatalf("top link count = %d, want 1 after equivalent-top recursive merge", got)
	}
	if got := low.gss.head.prev.linkCount(); got != 2 {
		t.Fatalf("predecessor link count = %d, want 2 linked child alternatives", got)
	}
	forks := reduceWindowsFromGSS(&low, 2, maxMainLinkCount)
	if len(forks) != 2 {
		t.Fatalf("pre-reduce linked windows = %d, want 2", len(forks))
	}

	parser := &Parser{language: &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left_low", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
			{Name: "left_high", Visible: true, Named: true},
		},
	}}
	setSyntheticPostReducePackingSafeEOF(t, parser, 1)
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &low, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if low.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if !anyReduced {
		t.Fatal("anyReduced = false, want true")
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want exactly one selected parent", nodeCount)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0; selection should not need final expansion", len(parser.pendingForkStacks))
	}
	parent := stackEntryNode(low.top())
	if parent == nil || parent.symbol != act.Symbol {
		t.Fatalf("top parent = %v, want reduced parent symbol %d", parent, act.Symbol)
	}
	if len(parent.children) != 2 || parent.children[0] != leftHigh {
		t.Fatal("constructed-parent selection did not keep the higher dynamic-precedence child path")
	}
	if got := parent.dynamicPrecedence; got != leftHigh.dynamicPrecedence {
		t.Fatalf("parent dynamic precedence = %d, want %d", got, leftHigh.dynamicPrecedence)
	}
}

func TestSelectReduceForkChildrenCoalescesRawOnlySameGroupForksByRecursiveOrder(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	flatRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	nestedRightChild := newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	nestedRight := newParentNodeInArena(arena, 2, true, []*Node{nestedRightChild}, nil, 0)
	nestedRight.startByte = 1
	nestedRight.endByte = 2
	nestedRight.startPoint = Point{Column: 1}
	nestedRight.endPoint = Point{Column: 2}

	flat := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, flatRight),
		},
	}
	nested := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, nestedRight),
		},
	}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
		{Name: "inner", Visible: true, Named: true},
	}}}
	selected := parser.selectReduceForkChildren(arena, ParseAction{Symbol: 3}, []reduceFork{nested, flat})
	if len(selected) != 1 {
		t.Fatalf("selected windows = %d, want one C-selected raw-only alternative", len(selected))
	}
	if stackEntryNode(selected[0].window[1]) != flatRight {
		t.Fatal("raw-only same-group selection did not keep the recursive-order winner")
	}
}

func TestSelectReduceForkChildrenCoalescesHigherDynamicPrecedence(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	leftLow := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftHigh := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leftHigh.dynamicPrecedence = 9
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	low := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, leftLow),
			newStackEntryNode(9, right),
		},
	}
	high := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, leftHigh),
			newStackEntryNode(9, right),
		},
	}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}}
	selected := parser.selectReduceForkChildren(arena, ParseAction{Symbol: 3, ChildCount: 2}, []reduceFork{low, high})
	if len(selected) != 1 {
		t.Fatalf("selected windows = %d, want 1 dynamic-precedence winner", len(selected))
	}
	if stackEntryNode(selected[0].window[0]) != leftHigh {
		t.Fatal("selected low dynamic-precedence fork, want high dynamic-precedence fork")
	}
}

func TestSelectReduceForkChildrenCoalescesLowerErrorCost(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	cleanRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	missingRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	missingRight.setMissing(true)
	missingRight.setHasError(true)
	clean := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, cleanRight),
		},
	}
	missing := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, missingRight),
		},
	}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}}
	selected := parser.selectReduceForkChildren(arena, ParseAction{Symbol: 3, ChildCount: 2}, []reduceFork{missing, clean})
	if len(selected) != 1 {
		t.Fatalf("selected windows = %d, want 1 lower-error-cost winner", len(selected))
	}
	if stackEntryNode(selected[0].window[1]) != cleanRight {
		t.Fatal("selected missing/error fork, want clean lower-error-cost fork")
	}
}

func TestSelectReduceForkChildrenCoalescesNonAdjacentRawOnlySameGroupForks(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	otherPopTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	flatRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	otherRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	nestedRightChild := newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	nestedRight := newParentNodeInArena(arena, 2, true, []*Node{nestedRightChild}, nil, 0)
	nestedRight.startByte = 1
	nestedRight.endByte = 2
	nestedRight.startPoint = Point{Column: 1}
	nestedRight.endPoint = Point{Column: 2}

	nested := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, nestedRight),
		},
	}
	distinct := reduceFork{
		popTo:    otherPopTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, otherRight),
		},
	}
	flat := reduceFork{
		popTo:    popTo,
		topState: 7,
		window: []stackEntry{
			newStackEntryNode(8, left),
			newStackEntryNode(9, flatRight),
		},
	}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
		{Name: "inner", Visible: true, Named: true},
	}}}
	selected := parser.selectReduceForkChildren(arena, ParseAction{Symbol: 3}, []reduceFork{nested, distinct, flat})
	if len(selected) != 2 {
		t.Fatalf("selected windows = %d, want distinct group plus one C-selected same-pop fork", len(selected))
	}
	if stackEntryNode(selected[0].window[1]) != flatRight {
		t.Fatal("selected first group did not keep the recursive-order winner")
	}
	if selected[1].popTo != otherPopTo {
		t.Fatal("selected second group did not preserve distinct group order")
	}
}

func TestReduceForkSelectionScansPastCrossProductRawCapForSameGroupWinner(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	var bestLeft, bestRight *Node
	var firstLeftHub *gssNode
	var head *gssNode
	for rightIdx := 0; rightIdx < maxMainLinkCount; rightIdx++ {
		left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
		leftHub := scratch.allocNode(newStackEntryNode(8, left), popTo, 2)
		for leftIdx := 1; leftIdx < maxMainLinkCount; leftIdx++ {
			altLeft := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
			if rightIdx == 1 && leftIdx == maxMainLinkCount-1 {
				altLeft.dynamicPrecedence = 99
				bestLeft = altLeft
			}
			leftHub.appendExtraLink(gssMainLink{
				prev:  popTo,
				entry: newStackEntryNode(8, altLeft),
			})
		}
		right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
		if rightIdx == 1 {
			bestRight = right
		}
		if rightIdx == 0 {
			firstLeftHub = leftHub
			head = scratch.allocNode(newStackEntryNode(9, right), leftHub, 3)
			continue
		}
		head.appendExtraLink(gssMainLink{
			prev:  leftHub,
			entry: newStackEntryNode(9, right),
		})
	}
	if bestLeft == nil || bestRight == nil {
		t.Fatal("setup did not create late best child path")
	}
	if got := head.linkCount(); got != maxMainLinkCount {
		t.Fatalf("head link count = %d, want production cap %d", got, maxMainLinkCount)
	}
	for i, count := 0, head.linkCount(); i < count; i++ {
		prev, _ := head.link(i)
		if got := prev.linkCount(); got != maxMainLinkCount {
			t.Fatalf("left predecessor %d link count = %d, want production cap %d", i, got, maxMainLinkCount)
		}
	}
	stack := glrStack{gss: gssStack{head: head}, byteOffset: 1}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}

	cappedRaw := reduceWindowsFromGSS(&stack, int(act.ChildCount), maxMainLinkCount)
	if len(cappedRaw) != maxMainLinkCount {
		t.Fatalf("capped raw windows = %d, want %d", len(cappedRaw), maxMainLinkCount)
	}
	for i, fork := range cappedRaw {
		if fork.popTo != popTo {
			t.Fatalf("capped raw fork %d popTo = %p, want shared popTo %p", i, fork.popTo, popTo)
		}
		if stackEntryNode(fork.window[1]) == bestRight || stackEntryNode(fork.window[0]) == bestLeft {
			t.Fatalf("proof setup invalid: capped raw fork %d unexpectedly included late best path", i)
		}
	}
	if firstLeftHub == nil || cappedRaw[0].popTo != popTo || cappedRaw[len(cappedRaw)-1].popTo != popTo {
		t.Fatal("proof setup invalid: capped raw windows are not the first same-pop cross-product group")
	}
	cappedSelected := parser.selectReduceForkChildren(arena, act, cappedRaw)
	if len(cappedSelected) != 1 {
		t.Fatalf("capped selected windows = %d, want one C-selected same-group window", len(cappedSelected))
	}
	for i, fork := range cappedSelected {
		if stackEntryNode(fork.window[0]) == bestLeft {
			t.Fatalf("proof setup invalid: capped raw fork %d unexpectedly selected the late best left child", i)
		}
	}
	if got := firstLeftHub.linkCount(); got != len(cappedRaw) {
		t.Fatalf("proof setup invalid: capped raw windows = %d, want first predecessor product width %d", len(cappedRaw), got)
	}

	selected := parser.selectedReduceWindowsFromGSS(arena, act, &stack, int(act.ChildCount), maxMainLinkCount)
	if len(selected) != 1 {
		t.Fatalf("integrated selected windows = %d, want 1 same-group winner", len(selected))
	}
	if stackEntryNode(selected[0].window[0]) != bestLeft || stackEntryNode(selected[0].window[1]) != bestRight {
		t.Fatal("integrated selection did not scan the cross product past the raw cap to keep the late same-group winner")
	}

	var anyReduced bool
	nodeCount := 0
	parser.applyReduceActionForked(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if !anyReduced {
		t.Fatal("anyReduced = false, want true")
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want 1 selected reduction", nodeCount)
	}
	parent := stackEntryNode(stack.top())
	if parent == nil || parent.symbol != act.Symbol {
		t.Fatalf("top parent = %v, want symbol %d", parent, act.Symbol)
	}
	if len(parent.children) != 2 || parent.children[0] != bestLeft || parent.children[1] != bestRight {
		t.Fatal("forked reduce did not construct the parent from the late cross-product same-group winner")
	}
	if got := parent.dynamicPrecedence; got != bestLeft.dynamicPrecedence {
		t.Fatalf("parent dynamic precedence = %d, want %d", got, bestLeft.dynamicPrecedence)
	}
}

func TestReduceForkSelectionCapsDistinctCrossProductGroups(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	var head *gssNode
	totalWindows := 0
	for rightIdx := 0; rightIdx < 2; rightIdx++ {
		right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
		leftPopTo := scratch.allocNode(stackEntry{state: StateID(20 + rightIdx*maxMainLinkCount)}, nil, 1)
		left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
		leftHub := scratch.allocNode(newStackEntryNode(8, left), leftPopTo, 2)
		for leftIdx := 1; leftIdx < maxMainLinkCount; leftIdx++ {
			distinctPopTo := scratch.allocNode(stackEntry{state: StateID(20 + rightIdx*maxMainLinkCount + leftIdx)}, nil, 1)
			altLeft := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
			leftHub.appendExtraLink(gssMainLink{
				prev:  distinctPopTo,
				entry: newStackEntryNode(8, altLeft),
			})
		}
		totalWindows += leftHub.linkCount()
		if rightIdx == 0 {
			head = scratch.allocNode(newStackEntryNode(9, right), leftHub, 3)
			continue
		}
		head.appendExtraLink(gssMainLink{
			prev:  leftHub,
			entry: newStackEntryNode(9, right),
		})
	}
	if got := head.linkCount(); got >= maxMainLinkCount {
		t.Fatalf("head link count = %d, want below production cap %d", got, maxMainLinkCount)
	}
	for i, count := 0, head.linkCount(); i < count; i++ {
		prev, _ := head.link(i)
		if got := prev.linkCount(); got != maxMainLinkCount {
			t.Fatalf("left predecessor %d link count = %d, want production cap %d", i, got, maxMainLinkCount)
		}
	}
	if totalWindows <= maxMainLinkCount {
		t.Fatalf("setup total windows = %d, want more than distinct group cap %d", totalWindows, maxMainLinkCount)
	}

	stack := glrStack{gss: gssStack{head: head}, byteOffset: 1}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "left", Visible: true, Named: true},
		{Name: "right", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}

	selected := parser.selectedReduceWindowsFromGSS(arena, act, &stack, int(act.ChildCount), maxMainLinkCount)
	if len(selected) != maxMainLinkCount {
		t.Fatalf("selected distinct groups = %d, want cap %d", len(selected), maxMainLinkCount)
	}
	seen := make(map[*gssNode]struct{}, len(selected))
	for i, fork := range selected {
		if _, ok := seen[fork.popTo]; ok {
			t.Fatalf("selected fork %d reused popTo %p, want distinct groups", i, fork.popTo)
		}
		seen[fork.popTo] = struct{}{}
	}
}

func TestSelectedReduceWindowsFromGSSCapsTraversalWork(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	const childCount = 8
	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 7}, nil, 1)
	prev := popTo
	for depth := 0; depth < childCount; depth++ {
		leaf := newLeafNodeInArena(arena, Symbol(1+depth), true, uint32(depth), uint32(depth+1), Point{Column: uint32(depth)}, Point{Column: uint32(depth + 1)})
		layer := scratch.allocNode(newStackEntryNode(StateID(20+depth), leaf), prev, uint32(depth+2))
		for alt := 1; alt < maxMainLinkCount; alt++ {
			altLeaf := newLeafNodeInArena(arena, Symbol(1+depth), true, uint32(depth), uint32(depth+1), Point{Column: uint32(depth)}, Point{Column: uint32(depth + 1)})
			layer.appendExtraLink(gssMainLink{
				prev:  prev,
				entry: newStackEntryNode(StateID(20+depth), altLeaf),
			})
		}
		prev = layer
	}
	stack := glrStack{gss: gssStack{head: prev}, byteOffset: childCount}
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{
		{Name: "eof", Visible: true, Named: true},
		{Name: "n1", Visible: true, Named: true},
		{Name: "n2", Visible: true, Named: true},
		{Name: "n3", Visible: true, Named: true},
		{Name: "n4", Visible: true, Named: true},
		{Name: "n5", Visible: true, Named: true},
		{Name: "n6", Visible: true, Named: true},
		{Name: "n7", Visible: true, Named: true},
		{Name: "n8", Visible: true, Named: true},
		{Name: "parent", Visible: true, Named: true},
	}}}
	act := ParseAction{Type: ParseActionReduce, Symbol: 9, ChildCount: childCount}
	parser.mergeScratch = &glrMergeScratch{}
	budget := selectedReduceGSSWorkBudget(childCount, maxMainLinkCount)

	selected, work, capped := parser.selectedReduceWindowsFromGSSWithBudget(arena, act, &stack, int(act.ChildCount), maxMainLinkCount, budget)
	if !capped {
		t.Fatal("selected reduce traversal did not report hitting the work budget")
	}
	if !parser.mergeScratch.policyPruned {
		t.Fatal("capped reduction traversal certified as unpruned")
	}
	if work != budget {
		t.Fatalf("selected reduce traversal work = %d, want budget %d", work, budget)
	}
	if len(selected) == 0 {
		t.Fatal("selected reduce traversal returned no windows before hitting the work budget")
	}
	if len(selected) > maxMainLinkCount {
		t.Fatalf("selected windows = %d, want at most group cap %d", len(selected), maxMainLinkCount)
	}
}

func TestFaithfulForkReduceTargetMismatchFallsBackToPendingFork(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })
	if perfCountersEnabled {
		ResetPerfCounters()
	}

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNode(3, right), leftNode, 3)

	altBase := scratch.allocNode(stackEntry{state: 9}, nil, 1)
	altLeft := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	altLeftNode := scratch.allocNode(newStackEntryNode(8, altLeft), altBase, 2)
	altRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightNode.appendExtraLink(gssMainLink{
		prev:  altLeftNode,
		entry: newStackEntryNode(7, altRight),
	})

	parser := &Parser{language: &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}}
	setSyntheticPostReducePackingSafeEOF(t, parser, 9)
	stack := glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if len(parser.pendingForkStacks) != 1 {
		t.Fatalf("pending forks = %d, want 1", len(parser.pendingForkStacks))
	}
	if stack.top().state == parser.pendingForkStacks[0].top().state {
		t.Fatalf("primary and pending top states both = %d, want mismatch fallback", stack.top().state)
	}
	if got := stack.gss.head.linkCount(); got != 1 {
		t.Fatalf("primary post-reduce head linkCount = %d, want 1", got)
	}
	if perfCountersEnabled {
		perf := PerfCountersSnapshot()
		if perf.ReduceForkCalls != 1 || perf.ReduceForkWindows != 2 || perf.ReduceForkMaxWindows != 2 {
			t.Fatalf("reduce fork counters = calls:%d windows:%d max:%d, want 1/2/2", perf.ReduceForkCalls, perf.ReduceForkWindows, perf.ReduceForkMaxWindows)
		}
		if perf.PostReduceMergeAttempts != 1 || perf.PostReduceMergePrimarySuccesses != 0 || perf.PendingForkStackAppends != 1 || perf.PendingForkStacksMaxLen != 1 {
			t.Fatalf("target-mismatch counters = attempts:%d primary:%d appends:%d max_pending:%d, want 1/0/1/1", perf.PostReduceMergeAttempts, perf.PostReduceMergePrimarySuccesses, perf.PendingForkStackAppends, perf.PendingForkStacksMaxLen)
		}
	}
}

func TestPostReduceForkMergeFinalizationRiskPredicate(t *testing.T) {
	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	stack := &glrStack{gss: gssStack{head: scratch.allocNode(stackEntry{state: 2}, base, 2)}, byteOffset: 1}
	eof := Token{Symbol: 0}

	t.Run("nil parser", func(t *testing.T) {
		var parser *Parser
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("dead stack", func(t *testing.T) {
		parser := &Parser{language: &Language{}}
		dead := *stack
		dead.dead = true
		if !parser.postReduceForkMergeHasFinalizationRisk(&dead, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("real current-lookahead single childful reduce is risky", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1}})
		parser.language.ParseTable[2][1] = parser.language.ParseTable[2][0]
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, Token{Symbol: 1}) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("non EOF shift lookahead", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionShift, State: 3}})
		parser.language.ParseTable[2][1] = parser.language.ParseTable[2][0]
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, Token{Symbol: 1}) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("missing EOF action", func(t *testing.T) {
		parser := &Parser{language: &Language{
			StateCount:     3,
			SymbolCount:    4,
			TokenCount:     3,
			ParseActions:   []ParseActionEntry{{}},
			ParseTable:     [][]uint16{{0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
			SymbolMetadata: []SymbolMetadata{{Name: "eof"}},
		}}
		parser.denseLimit = len(parser.language.ParseTable)
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("out of range EOF action", func(t *testing.T) {
		parser := &Parser{language: &Language{
			StateCount:     3,
			SymbolCount:    4,
			TokenCount:     3,
			ParseActions:   []ParseActionEntry{{}},
			ParseTable:     [][]uint16{{0, 0, 0, 0}, {0, 0, 0, 0}, {5, 0, 0, 0}},
			SymbolMetadata: []SymbolMetadata{{Name: "eof"}},
		}}
		parser.denseLimit = len(parser.language.ParseTable)
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("single childful EOF reduce", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1}})
		if parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = true, want false")
		}
	})
	t.Run("EOF accept", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionAccept}})
		if parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = true, want false")
		}
	})
	t.Run("no-lookahead is risky", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1}})
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, Token{Symbol: 0, NoLookahead: true}) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("zero child EOF reduce", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 0}})
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("mixed EOF actions", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{
			{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
			{Type: ParseActionAccept},
		})
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
	t.Run("single EOF shift", func(t *testing.T) {
		parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{Name: "eof"}}}}
		setSyntheticEOFAction(t, parser, 2, []ParseAction{{Type: ParseActionShift, State: 3}})
		if !parser.postReduceForkMergeHasFinalizationRisk(stack, eof) {
			t.Fatal("risk = false, want true")
		}
	})
}

func TestFaithfulForkReduceEOFNoActionKeepsPendingFork(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	parser, stack, act := buildTwoWindowFullGSSReduceCase(t, &scratch, arena)
	parser.language.StateCount = 4
	parser.language.SymbolCount = 4
	parser.language.TokenCount = 3
	parser.language.ParseActions = []ParseActionEntry{{}}
	parser.language.ParseTable = [][]uint16{
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
	}
	parser.denseLimit = len(parser.language.ParseTable)
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{Symbol: 0}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if got := stack.gss.head.linkCount(); got != 1 {
		t.Fatalf("primary post-reduce head linkCount = %d, want 1", got)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0 after constructed-parent same-pop selection", len(parser.pendingForkStacks))
	}
}

func TestFaithfulForkReduceEOFZeroChildReduceKeepsPendingFork(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	parser, stack, act := buildTwoWindowFullGSSReduceCase(t, &scratch, arena)
	setSyntheticEOFAction(t, parser, 1, []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 0}})
	var anyReduced bool
	nodeCount := 0

	parser.applyReduceActionFromGSS(nil, &stack, act, Token{Symbol: 0}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false, false)
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if got := stack.gss.head.linkCount(); got != 1 {
		t.Fatalf("primary post-reduce head linkCount = %d, want 1", got)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0 after constructed-parent same-pop selection", len(parser.pendingForkStacks))
	}
}

func TestTryMergePostReduceForkDeclinesAcceptedStacks(t *testing.T) {
	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := glrStack{
		gss:        gssStack{head: scratch.allocNode(stackEntry{state: 2}, base, 2)},
		byteOffset: 3,
		accepted:   true,
	}
	right := glrStack{
		gss:        gssStack{head: scratch.allocNode(stackEntry{state: 2}, base, 2)},
		byteOffset: 3,
		accepted:   true,
	}

	if tryMergePostReduceFork(nil, &left, &right) {
		t.Fatal("tryMergePostReduceFork = true, want false for accepted stacks")
	}
}

func TestTryMergePostReduceForkRejectsDistinctCRecoveryCosts(t *testing.T) {
	var scratch gssScratch
	build := func(sym Symbol, paused bool) glrStack {
		node := NewLeafNode(sym, true, 0, 5, Point{}, Point{Column: 5})
		entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
		return glrStack{
			gss:        buildGSSStack(entries, &scratch),
			byteOffset: stackByteOffset(entries),
			cPaused:    paused,
		}
	}

	clean := build(11, false)
	paused := build(12, true)
	if cleanCost, pausedCost := cStackErrorCostForMerge(nil, &clean), cStackErrorCostForMerge(nil, &paused); cleanCost == pausedCost {
		t.Fatalf("test setup costs equal: clean=%d paused=%d", cleanCost, pausedCost)
	}
	parser := &Parser{errorCostCompetition: true}

	if tryMergePostReduceFork(parser, &clean, &paused) {
		t.Fatal("tryMergePostReduceFork = true, want false for distinct C recovery costs")
	}
	if got := clean.gss.head.linkCount(); got != 1 {
		t.Fatalf("clean link count after rejected merge = %d, want 1", got)
	}

	noTreeParser := &Parser{errorCostCompetition: true, noTreeBenchmarkOnly: true}
	clean = build(11, false)
	paused = build(12, true)
	if !tryMergePostReduceFork(noTreeParser, &clean, &paused) {
		t.Fatal("tryMergePostReduceFork = false, want true when C cost competition is disabled")
	}
	if got := clean.gss.head.linkCount(); got != 2 {
		t.Fatalf("clean link count after no-tree merge = %d, want 2", got)
	}
}

func TestNoTreeReduceFromGSSRejectsMultiLinkSpan(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newNoTreeLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newNoTreeLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNoTreeNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNoTreeNode(3, right), leftNode, 3)
	altRight := newNoTreeLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightNode.appendExtraLink(gssMainLink{
		prev:  leftNode,
		entry: newStackEntryNoTreeNode(3, altRight),
	})

	parser := &Parser{language: &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}}
	stack := &glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}
	var anyReduced bool
	nodeCount := 0

	parser.applyNoTreeReduceActionFromGSS(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, &scratch, nil, nil, false)
	if !stack.dead {
		t.Fatal("stack.dead = false, want true for no-tree multi-link GSS reduce")
	}
	if anyReduced {
		t.Fatal("anyReduced = true, want false")
	}
	if nodeCount != 0 {
		t.Fatalf("nodeCount = %d, want 0", nodeCount)
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0", len(parser.pendingForkStacks))
	}
}

func TestRejectUndrainedPendingForkStacksClearsAndKills(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	parser := &Parser{mergeScratch: &glrMergeScratch{}, pendingForkStacks: []glrStack{newGLRStack(9)}}
	stack := newGLRStack(1)

	if !parser.rejectUndrainedPendingForkStacks(&stack) {
		t.Fatal("rejectUndrainedPendingForkStacks = false, want true")
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0", len(parser.pendingForkStacks))
	}
	if !stack.dead {
		t.Fatal("stack.dead = false, want true")
	}
	if !parser.mergeScratch.policyPruned {
		t.Fatal("discarded pending forks certified as unpruned")
	}
}

func TestBuildReduceChainHintsUsesLanguageMetadata(t *testing.T) {
	t.Setenv("GOT_GLR_REDUCE_CHAIN_HINTS", "1")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	lang := &Language{
		Name:        "python",
		StateCount:  10,
		SymbolCount: 10,
		SymbolNames: []string{
			"", "", "", "", "", "", "", "", "", "",
		},
		ReduceChainHints: []ReduceChainHint{{
			StartState:     StateID(3),
			Lookahead:      Symbol(2),
			TerminalStates: []StateID{StateID(4), StateID(5)},
			TerminalAction: ReduceChainTerminalSingleShift,
			MaxSteps:       7,
		}},
	}

	got := buildReduceChainHints(lang)
	if len(got) != 1 {
		t.Fatalf("hint count = %d, want 1", len(got))
	}
	hint := got[0]
	if hint.startState != StateID(3) || hint.lookahead != Symbol(2) || hint.maxSteps != 7 {
		t.Fatalf("hint = %+v, want state=3 lookahead=2 maxSteps=7", hint)
	}
	if hint.terminalAction != classifiedParseActionSingleShift {
		t.Fatalf("terminal action = %d, want single shift", hint.terminalAction)
	}
	if len(hint.terminalStates) != 2 || hint.terminalStates[0] != StateID(4) || hint.terminalStates[1] != StateID(5) {
		t.Fatalf("terminal states = %v, want [4 5]", hint.terminalStates)
	}

	lang.ReduceChainHints[0].TerminalStates[0] = StateID(9)
	if hint.terminalStates[0] != StateID(4) {
		t.Fatalf("internal hint terminal states alias language metadata: got %v", hint.terminalStates)
	}
}

func TestReduceChainHintForUsesStateIndex(t *testing.T) {
	p := &Parser{
		reduceChainHints: []reduceChainHint{
			{startState: StateID(8), lookahead: Symbol(3), maxSteps: 4},
			{startState: StateID(10), lookahead: Symbol(4), maxSteps: 5},
			{startState: StateID(10), lookahead: Symbol(5), maxSteps: 6},
		},
	}
	p.reduceChainHintByState = buildReduceChainHintIndex(p.reduceChainHints)

	hint, ok := p.reduceChainHintFor(StateID(8), Symbol(3))
	if !ok || hint.maxSteps != 4 {
		t.Fatalf("hint for state=8 lookahead=3 = %+v, %v; want maxSteps=4, true", hint, ok)
	}
	hint, ok = p.reduceChainHintFor(StateID(10), Symbol(5))
	if !ok || hint.maxSteps != 6 {
		t.Fatalf("hint for duplicate state=10 lookahead=5 = %+v, %v; want maxSteps=6, true", hint, ok)
	}
	if _, ok := p.reduceChainHintFor(StateID(9), Symbol(3)); ok {
		t.Fatal("unexpected hint for state without entry")
	}
	if _, ok := p.reduceChainHintFor(StateID(10), Symbol(6)); ok {
		t.Fatal("unexpected hint for duplicate state with unmatched lookahead")
	}
}

func TestCanCollapseNamedLeafWrapperSingleAnonymousToken(t *testing.T) {
	p := &Parser{
		language: &Language{
			SymbolMetadata: []SymbolMetadata{
				{Name: "EOF"},
				{Name: "optional_chain", Visible: true, Named: true},
				{Name: "?.", Visible: true, Named: false},
				{Name: "identifier", Visible: true, Named: true},
				{Name: "_hidden", Visible: false, Named: false},
			},
		},
	}

	if !p.canCollapseNamedLeafWrapper(1, 2) {
		t.Fatal("expected visible named wrapper over visible anonymous token to collapse")
	}
	if p.canCollapseNamedLeafWrapper(1, 3) {
		t.Fatal("did not expect visible named wrapper over visible named child to collapse")
	}
	if p.canCollapseNamedLeafWrapper(1, 4) {
		t.Fatal("did not expect visible named wrapper over invisible child to collapse")
	}
}

// A named rule wrapping a DIFFERENT-named visible anonymous token (e.g.
// optional_chain over "?.") must NOT collapse to a childless leaf: C tree-sitter
// keeps the anonymous token as a visible child (childCount==1). The unary
// self-reduction collapse must therefore decline (return nil) so the normal
// reduce keeps the child.
func TestCollapsibleUnarySelfReductionKeepsDifferentNamedAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "optional_chain", Visible: true, Named: true},
			{Name: "?.", Visible: true, Named: false},
		},
	}
	p := &Parser{language: lang}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil); got != nil {
		t.Fatalf("expected different-named visible anon child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionKeepsDifferentNamedAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "optional_chain", Visible: true, Named: true},
			{Name: "?.", Visible: true, Named: false},
		},
	}
	p := &Parser{language: lang}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("expected different-named visible anon child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleUnarySelfReductionKeepsStarlarkContinueTokenChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "continue_statement", Visible: true, Named: true},
			{Name: "continue", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 2209, 2217, Point{Row: 73, Column: 16}, Point{Row: 73, Column: 24})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil); got != nil {
		t.Fatalf("expected Starlark continue token child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionKeepsStarlarkContinueTokenChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "continue_statement", Visible: true, Named: true},
			{Name: "continue", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 2209, 2217, Point{Row: 73, Column: 16}, Point{Row: 73, Column: 24})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("expected raw Starlark continue token child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleUnarySelfReductionKeepsSingleTokenWrapperDifferentAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "wildcard_pattern", Visible: true, Named: true},
			{Name: "_", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 4, 5, Point{Column: 4}, Point{Column: 5})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil); got != nil {
		t.Fatalf("expected different-named wildcard token child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionKeepsSingleTokenWrapperDifferentAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "wildcard_pattern", Visible: true, Named: true},
			{Name: "_", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 4, 5, Point{Column: 4}, Point{Column: 5})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("expected raw different-named wildcard token child to be kept (no collapse), got node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleUnarySelfReductionCollapsesSameNamedInlinedTokenChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "nil", Visible: true, Named: true},
			{Name: "nil", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 4, 7, Point{Column: 4}, Point{Column: 7})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil)
	if got == nil {
		t.Fatal("expected same-named inlined token child to collapse")
	}
	if got.symbol != 1 {
		t.Fatalf("collapsed symbol = %d, want 1", got.symbol)
	}
	if !got.IsNamed() {
		t.Fatal("collapsed node should be named")
	}
	if got.ChildCount() != 0 {
		t.Fatalf("collapsed ChildCount = %d, want 0", got.ChildCount())
	}
	if got.StartByte() != 4 || got.EndByte() != 7 {
		t.Fatalf("collapsed range = [%d,%d], want [4,7]", got.StartByte(), got.EndByte())
	}
}

func TestCollapsibleRawUnarySelfReductionCollapsesSameNamedInlinedTokenChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "nil", Visible: true, Named: true},
			{Name: "nil", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 4, 7, Point{Column: 4}, Point{Column: 7})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1)
	if got == nil {
		t.Fatal("expected raw same-named inlined token child to collapse")
	}
	if got.symbol != 1 {
		t.Fatalf("collapsed symbol = %d, want 1", got.symbol)
	}
	if !got.IsNamed() {
		t.Fatal("collapsed node should be named")
	}
	if got.ChildCount() != 0 {
		t.Fatalf("collapsed ChildCount = %d, want 0", got.ChildCount())
	}
	if got.StartByte() != 4 || got.EndByte() != 7 {
		t.Fatalf("collapsed range = [%d,%d], want [4,7]", got.StartByte(), got.EndByte())
	}
}

func TestCollapsibleUnarySelfReductionKeepsSharedSingleTokenWrapperAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "self_expression", Visible: true, Named: true},
			{Name: "self", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language:                   lang,
		sharedAnonymousTokenSymbol: []bool{false, false, true},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 0, 4, Point{}, Point{Column: 4})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil); got != nil {
		t.Fatalf("expected shared anonymous token child to be kept, got collapsed node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionKeepsSharedSingleTokenWrapperAnonChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "self_expression", Visible: true, Named: true},
			{Name: "self", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language:                   lang,
		sharedAnonymousTokenSymbol: []bool{false, false, true},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 0, 4, Point{}, Point{Column: 4})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("expected raw shared anonymous token child to be kept, got collapsed node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleUnarySelfReductionKeepsSharedSameSymbolAnonymousChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "shared", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language:                   lang,
		sharedAnonymousTokenSymbol: []bool{false, true},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 1, false, 12, 13, Point{Column: 12}, Point{Column: 13})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleUnarySelfReduction(act, Token{}, arena, entries, 0, 1, []*Node{child}, nil); got != nil {
		t.Fatalf("expected shared same-symbol anonymous child to be kept, got collapsed node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionKeepsSharedSameSymbolAnonymousChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "shared", Visible: true, Named: false},
		},
	}
	p := &Parser{
		language:                   lang,
		sharedAnonymousTokenSymbol: []bool{false, true},
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 1, false, 12, 13, Point{Column: 12}, Point{Column: 13})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("expected raw shared same-symbol anonymous child to be kept, got collapsed node with cc=%d", got.ChildCount())
	}
}

func TestCollapsibleRawUnarySelfReductionRejectsInvisibleChild(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "optional_chain", Visible: true, Named: true},
			{Name: "_hidden", Visible: false, Named: false},
		},
	}
	p := &Parser{
		language: lang,
	}
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	if got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1); got != nil {
		t.Fatalf("raw unary collapse returned %v for invisible child", got)
	}
}

func TestCollapsibleRawUnarySelfReductionCollapsesGeneratedHiddenChoicePassthrough(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "_statement", Visible: false, Named: true},
			{Name: "_simple_statements", Visible: false, Named: true},
			{Name: "import_statement", Visible: true, Named: true},
		},
		HiddenChoicePassthroughSymbols: []bool{false, true, false, false},
	}
	p := &Parser{language: lang}
	arena := newNodeArena(arenaClassFull)
	importStmt := newLeafNodeInArena(arena, 3, true, 1, 7, Point{Column: 1}, Point{Column: 7})
	child := newParentNodeInArena(arena, 2, true, []*Node{importStmt}, nil, 0)
	entries := []stackEntry{newStackEntryNode(0, child)}
	act := ParseAction{Symbol: 1, ChildCount: 1}

	got := p.collapsibleRawUnarySelfReduction(act, Token{}, arena, entries, 0, 1)
	if got != child {
		t.Fatalf("raw unary collapse = %p, want hidden choice child %p", got, child)
	}
}

func TestNonTerminalAliasMapPreservesInvisibleUnaryWrappers(t *testing.T) {
	lang := &Language{
		Name:        "synthetic",
		SymbolNames: []string{"EOF", "literal", "_aliased_wrapper", "alias_target", "_plain_wrapper"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "literal", Visible: true, Named: true},
			{Name: "_aliased_wrapper", Visible: false, Named: false},
			{Name: "alias_target", Visible: true, Named: true},
			{Name: "_plain_wrapper", Visible: false, Named: false},
		},
		HiddenChoicePassthroughSymbols: []bool{false, false, true, false, true},
		NonTerminalAliasMap: [][]Symbol{
			nil,
			nil,
			{2, 3},
		},
	}

	marked := buildAliasPreservedWrapperSymbols(lang)
	if !symbolMarked(marked, 2) {
		t.Fatalf("alias map did not preserve wrapper symbol 2")
	}
	if symbolMarked(marked, 4) {
		t.Fatalf("alias map unexpectedly preserved plain wrapper symbol 4")
	}

	p := NewParser(lang)
	if p.canCollapseInvisibleUnaryWrapperSymbol(2) {
		t.Fatalf("aliased wrapper should not collapse as an invisible unary wrapper")
	}
	if p.canCollapseHiddenChoicePassthroughSymbol(2) {
		t.Fatalf("aliased wrapper should not collapse as hidden choice passthrough")
	}
	if !p.canCollapseInvisibleUnaryWrapperSymbol(4) {
		t.Fatalf("plain invisible wrapper should remain collapsible")
	}
	if !p.canCollapseHiddenChoicePassthroughSymbol(4) {
		t.Fatalf("plain hidden choice passthrough should remain collapsible")
	}
}

func TestReduceProductionHasEffectiveFieldsKeepsConflictedInheritedProjection(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expr", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
		},
		FieldNames: []string{"", "left", "right"},
		FieldMapSlices: [][2]uint16{
			{0, 2},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
			{FieldID: 2, ChildIndex: 0, Inherited: true},
		},
		ParseActions: []ParseActionEntry{
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 1, ChildCount: 1, ProductionID: 0}}},
		},
	}
	p := NewParser(lang)
	arena := newNodeArena(arenaClassFull)

	if !p.reduceProductionHasFields(0) {
		t.Fatal("reduceProductionHasFields = false, want true for conflicted inherited fields")
	}
	if !p.reduceProductionHasEffectiveFields(1, 0, arena) {
		t.Fatal("reduceProductionHasEffectiveFields = false, want true for conflicted inherited fields")
	}
	fieldIDs, _, conflictedInherited := p.buildFieldIDs(1, 0, arena)
	if got := len(fieldIDs); got != 1 {
		t.Fatalf("buildFieldIDs len = %d, want 1", got)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("buildFieldIDs[0] = %d, want 0", got)
	}
	if len(conflictedInherited) != 1 || !conflictedInherited[0] {
		t.Fatalf("conflicted inherited fields = %v, want [true]", conflictedInherited)
	}
}

func TestTryPushPendingNoFieldParentRejectsConflictedInheritedFields(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expr", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
		},
		FieldNames: []string{"", "left", "right"},
		FieldMapSlices: [][2]uint16{
			{0, 2},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
			{FieldID: 2, ChildIndex: 0, Inherited: true},
		},
		ParseActions: []ParseActionEntry{
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 1, ChildCount: 1, ProductionID: 0}}},
		},
	}
	p := NewParser(lang)
	p.pendingFullParents = true
	arena := newNodeArena(arenaClassFull)
	leaf := newCompactFullLeafInArena(arena, 2, true, 1, 3, Point{Column: 1}, Point{Column: 3})
	entry := newStackEntryCompactFullLeaf(4, leaf)
	stack := &glrStack{entries: []stackEntry{entry}}
	act := ParseAction{Symbol: 1, ChildCount: 1, ProductionID: 0}
	anyReduced := false
	nodeCount := 0

	if p.tryPushPendingNoFieldParent(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, nil, []stackEntry{entry}, 0, 1, 1, 0, 0) {
		t.Fatal("tryPushPendingNoFieldParent = true, want false for conflicted inherited fields")
	}
	if anyReduced {
		t.Fatal("anyReduced = true, want false")
	}
	if nodeCount != 0 {
		t.Fatalf("nodeCount = %d, want 0", nodeCount)
	}
	if got := arena.pendingParentRejectedFields; got != 1 {
		t.Fatalf("pendingParentRejectedFields = %d, want 1", got)
	}
	if got := arena.pendingParentCreated; got != 0 {
		t.Fatalf("pendingParentCreated = %d, want 0", got)
	}
	if got := len(stack.entries); got != 1 {
		t.Fatalf("stack entries = %d, want 1", got)
	}
	if got := stackEntryCompactFullLeaf(stack.entries[0]); got == nil {
		t.Fatal("stack entry changed, want original compact leaf")
	}
}

func TestTryPushPendingNoFieldParentCountsOrdinaryHiddenNodeRefs(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expr", Visible: true, Named: true},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
	}
	p := NewParser(lang)
	p.pendingFullParents = true
	arena := newNodeArena(arenaClassFull)
	first := newLeafNodeInArena(arena, 3, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	second := newLeafNodeInArena(arena, 3, true, 3, 4, Point{Column: 3}, Point{Column: 4})
	hidden := newParentNodeInArena(arena, 2, false, []*Node{first, second}, nil, 0)
	entry := newStackEntryNode(4, hidden)
	stack := &glrStack{entries: []stackEntry{entry}}
	act := ParseAction{Symbol: 1, ChildCount: 1, ProductionID: 0}
	anyReduced := false
	nodeCount := 0

	if !p.tryPushPendingNoFieldParent(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, nil, []stackEntry{entry}, 0, 1, 1, 0, 0) {
		t.Fatal("tryPushPendingNoFieldParent = false, want true")
	}
	if got := arena.pendingParentsFlattened; got != 0 {
		t.Fatalf("pendingParentsFlattened = %d, want 0 for ordinary hidden node", got)
	}
	if got := arena.pendingChildRefsFlattened; got != 2 {
		t.Fatalf("pendingChildRefsFlattened = %d, want 2", got)
	}
	parent := stackEntryPendingParent(stack.entries[0])
	if parent == nil {
		t.Fatal("stack entry is not a pending parent")
	}
	if got := parent.childEntryCount(); got != 2 {
		t.Fatalf("pending parent child count = %d, want 2", got)
	}
}

func TestTryPushPendingNoFieldParentKeepsHiddenCompactLeafCompact(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expr", Visible: true, Named: true},
			{Name: "_hidden", Visible: false, Named: false},
		},
	}
	p := NewParser(lang)
	p.pendingFullParents = true
	arena := newNodeArena(arenaClassFull)
	leaf := newCompactFullLeafInArena(arena, 2, false, 5, 8, Point{Column: 5}, Point{Column: 8})
	entry := newStackEntryCompactFullLeaf(4, leaf)
	stack := &glrStack{entries: []stackEntry{entry}}
	act := ParseAction{Symbol: 1, ChildCount: 1, ProductionID: 0}
	anyReduced := false
	nodeCount := 0

	if !p.tryPushPendingNoFieldParent(stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, nil, []stackEntry{entry}, 0, 1, 1, 0, 0) {
		t.Fatal("tryPushPendingNoFieldParent = false, want true for hidden compact leaf")
	}
	if got := arena.compactFullLeafMaterialized; got != 0 {
		t.Fatalf("compactFullLeafMaterialized = %d, want 0", got)
	}
	parent := stackEntryPendingParent(stack.entries[0])
	if parent == nil {
		t.Fatal("stack entry is not a pending parent")
	}
	if got := parent.childEntryCount(); got != 0 {
		t.Fatalf("pending parent child count = %d, want 0", got)
	}
	if parent.startByte != 5 || parent.endByte != 8 {
		t.Fatalf("pending parent span = [%d,%d], want [5,8]", parent.startByte, parent.endByte)
	}
}

func TestCollapsibleRawUnarySelfReductionEntryCollapsesPendingParentSameSymbol(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expr", Visible: true, Named: true},
		},
	}
	p := &Parser{language: lang}
	arena := newNodeArena(arenaClassFull)
	parent := newPendingParentInArena(arena, 1, true, 3, nil, 1, 3, Point{Column: 1}, Point{Column: 3}, false)
	entry := newStackEntryPendingParent(4, parent)
	act := ParseAction{Symbol: 1, ChildCount: 1, ProductionID: 9}

	got, ok := p.collapsibleRawUnarySelfReductionEntry(act, Token{}, arena, []stackEntry{entry}, 0, 1)
	if !ok {
		t.Fatal("expected pending parent raw unary reduction to collapse")
	}
	if stackEntryPendingParent(got) != parent {
		t.Fatal("collapsed entry did not preserve pending parent payload")
	}
	setCollapsedUnaryEntryMetadata(&got, act, false, 2, 5)
	if parent.productionID != 9 || parent.preGotoState != 2 || parent.parseState != 5 || got.state != 5 {
		t.Fatalf("pending parent metadata = prod %d pre %d state %d entry %d", parent.productionID, parent.preGotoState, parent.parseState, got.state)
	}
}

func TestCollapsibleRawUnarySelfReductionEntryCollapsesPendingParentInvisibleWrapper(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "_wrapper", Visible: false, Named: false},
			{Name: "expr", Visible: true, Named: true},
		},
	}
	p := &Parser{language: lang}
	arena := newNodeArena(arenaClassFull)
	parent := newPendingParentInArena(arena, 2, true, 3, nil, 1, 3, Point{Column: 1}, Point{Column: 3}, false)
	entry := newStackEntryPendingParent(4, parent)
	act := ParseAction{Symbol: 1, ChildCount: 1, ProductionID: 9}

	got, ok := p.collapsibleRawUnarySelfReductionEntry(act, Token{}, arena, []stackEntry{entry}, 0, 1)
	if !ok {
		t.Fatal("expected invisible wrapper over pending parent to collapse")
	}
	if stackEntryPendingParent(got) != parent {
		t.Fatal("collapsed wrapper did not preserve pending parent payload")
	}
}

// The tests below compare the scratch-backed C-wave enumeration and the in-place
// fork selection with the code they replaced. The two originals are kept here as
// test-only oracles. Do not edit them to follow the engine.

// cWaveOriginalIterator is the iterator of the allocating enumeration: every
// iterator owns its popped path.
type cWaveOriginalIterator struct {
	node         *gssNode
	revPath      []stackEntry
	subtreeCount int
}

// cWaveReduceWindowsOriginal is cWaveReduceWindowsFromGSS as it read before it
// became a wrapper around cWaveReduceWindowsFromGSSInto. It allocates one
// window per fork and one path per cloned iterator.
func cWaveReduceWindowsOriginal(s *glrStack, childCount int) []reduceFork {
	if s == nil || s.gss.head == nil || childCount < -1 {
		return nil
	}

	iterators := []cWaveOriginalIterator{{node: s.gss.head}}
	var forks []reduceFork
	for len(iterators) > 0 {
		for i, waveSize := 0, len(iterators); i < waveSize; {
			iterator := iterators[i]
			node := iterator.node
			complete := node != nil && (iterator.subtreeCount == childCount || (childCount == -1 && node.prev == nil && node.extraLinkCount == 0))
			if complete {
				window := make([]stackEntry, len(iterator.revPath))
				for j := range iterator.revPath {
					window[j] = iterator.revPath[len(iterator.revPath)-1-j]
				}
				forks = appendCStackSliceOrderReduceFork(forks, reduceFork{
					window:   window,
					topState: node.entry.state,
					popTo:    node,
				})
			}

			linkCount := 0
			if node != nil && !complete {
				linkCount = node.linkCount()
				if node.prev == nil && node.extraLinkCount == 0 {
					linkCount = 0
				}
			}
			if linkCount == 0 {
				copy(iterators[i:], iterators[i+1:])
				iterators[len(iterators)-1] = cWaveOriginalIterator{}
				iterators = iterators[:len(iterators)-1]
				waveSize--
				continue
			}

			for linkIndex := 1; linkIndex < linkCount; linkIndex++ {
				if len(iterators) >= cStackMaxIteratorCount {
					continue
				}
				prev, entry := node.link(linkIndex)
				clonePath := make([]stackEntry, len(iterator.revPath), len(iterator.revPath)+1)
				copy(clonePath, iterator.revPath)
				clonePath = append(clonePath, entry)
				clone := cWaveOriginalIterator{
					node:         prev,
					revPath:      clonePath,
					subtreeCount: iterator.subtreeCount,
				}
				if cStackLinkCountsAsSubtree(entry) {
					clone.subtreeCount++
				}
				iterators = append(iterators, clone)
			}

			prev, entry := node.link(0)
			iterator.node = prev
			iterator.revPath = append(iterator.revPath, entry)
			if cStackLinkCountsAsSubtree(entry) {
				iterator.subtreeCount++
			}
			iterators[i] = iterator
			i++
		}
	}
	return forks
}

// cWaveSelectOriginal is the selection loop of applyReduceActionForked as it
// read before selectCWaveReduceForks. It builds new slices and leaves allForks
// alone; groups holds the runs themselves, the way work-count builds read them.
func cWaveSelectOriginal(allForks []reduceFork, prefer func(candidate, incumbent reduceFork) int) (forks []reduceFork, groups [][]reduceFork, ambiguous bool) {
	for start := 0; start < len(allForks); {
		end := start + 1
		for end < len(allForks) && reduceForksSameChildSelectionGroup(allForks[start], allForks[end]) {
			end++
		}
		group := allForks[start:end]
		if len(group) > 1 {
			ambiguous = true
		}
		selected := 0
		for i := 1; i < len(group); i++ {
			if prefer(group[i], group[selected]) < 0 {
				selected = i
			}
		}
		groups = append(groups, group)
		forks = append(forks, group[selected])
		start = end
	}
	return forks, groups, ambiguous
}

// cWaveGraphGen draws random packed GSS graphs. A node's primary and alternate
// links point at any earlier node, so paths converge on shared pop targets and
// the live iterator set can reach the C limit. A few nodes have no primary
// predecessor, which ends the paths that reach them.
type cWaveGraphGen struct {
	rng    *rand.Rand
	symbol Symbol
}

func (g *cWaveGraphGen) entry() stackEntry {
	switch g.rng.Intn(6) {
	case 0: // a state-only entry: C's null subtree
		return stackEntry{state: StateID(g.rng.Intn(50))}
	case 1: // an extra node: not counted as a popped subtree
		g.symbol++
		return newStackEntryNode(StateID(g.rng.Intn(50)), &Node{symbol: g.symbol, flags: nodeFlagExtra})
	default:
		g.symbol++
		return newStackEntryNode(StateID(g.rng.Intn(50)), &Node{symbol: g.symbol, flags: nodeFlagNamed})
	}
}

func (g *cWaveGraphGen) graph() *gssNode {
	var all []*gssNode
	for i, n := 0, 1+g.rng.Intn(3); i < n; i++ {
		all = append(all, &gssNode{entry: stackEntry{state: StateID(100 + i)}, depth: 1})
	}
	for d, layers := 0, 1+g.rng.Intn(9); d < layers; d++ {
		var layer []*gssNode
		for i, n := 0, 1+g.rng.Intn(4); i < n; i++ {
			node := &gssNode{entry: g.entry(), prev: all[g.rng.Intn(len(all))], depth: uint32(d + 2)}
			if g.rng.Intn(20) == 0 {
				node.prev = nil
			}
			for l, k := 0, g.rng.Intn(5); l < k && int(node.extraLinkCount) < maxCMainLinkCount-1; l++ {
				node.appendExtraLinkWithLimitAndOwner(gssMainLink{prev: all[g.rng.Intn(len(all))], entry: g.entry()}, maxCMainLinkCount, nil)
			}
			layer = append(layer, node)
		}
		all = append(all, layer...)
	}
	return all[len(all)-1]
}

// cWaveForksDiff describes the first difference between two fork lists, or
// returns "" when they are equal. It also checks that each window's capacity
// equals its length, so an append to one window cannot write into the next.
func cWaveForksDiff(want, got []reduceFork) string {
	if len(want) != len(got) {
		return fmt.Sprintf("%d forks, want %d", len(got), len(want))
	}
	for i := range want {
		w, g := want[i], got[i]
		switch {
		case w.topState != g.topState:
			return fmt.Sprintf("fork %d: top state %d, want %d", i, g.topState, w.topState)
		case w.popTo != g.popTo:
			return fmt.Sprintf("fork %d: pop target %p, want %p", i, g.popTo, w.popTo)
		case len(w.window) != len(g.window):
			return fmt.Sprintf("fork %d: window length %d, want %d", i, len(g.window), len(w.window))
		case (w.window == nil) != (g.window == nil):
			return fmt.Sprintf("fork %d: window nil=%t, want nil=%t", i, g.window == nil, w.window == nil)
		case cap(g.window) != len(g.window):
			return fmt.Sprintf("fork %d: window capacity %d, length %d", i, cap(g.window), len(g.window))
		}
		for j := range w.window {
			if w.window[j] != g.window[j] {
				return fmt.Sprintf("fork %d: window entry %d differs", i, j)
			}
		}
	}
	return ""
}

// cWaveForkRank orders forks by their content, so a preference built on it is a
// pure function of the two forks. The modulus makes ties common.
func cWaveForkRank(f reduceFork) uint64 {
	h := uint64(14695981039346656037)
	mix := func(v uint64) {
		h ^= v
		h *= 1099511628211
	}
	mix(uint64(f.topState))
	for _, e := range f.window {
		mix(uint64(e.state))
		mix(uint64(stackEntryNodeSymbol(e)))
	}
	return h % 5
}

type cWavePreferCall struct{ candidate, incumbent uint64 }

// cWavePreferLogging returns a preference over cWaveForkRank that records each
// call. It answers -1, 0 or 1, so the selection's tie rule shows.
func cWavePreferLogging(log *[]cWavePreferCall) func(candidate, incumbent reduceFork) int {
	return func(candidate, incumbent reduceFork) int {
		c, i := cWaveForkRank(candidate), cWaveForkRank(incumbent)
		*log = append(*log, cWavePreferCall{c, i})
		switch {
		case c < i:
			return -1
		case c > i:
			return 1
		}
		return 0
	}
}

// cWaveScribble fills every buffer of w, to its full capacity, with plausible
// leftovers from an earlier use. A call must not depend on its scratch being
// clear.
func cWaveScribble(w *cWaveScratch) {
	junkNode := &gssNode{entry: stackEntry{state: 9999}, depth: 1}
	junkEntry := newStackEntryNode(9999, &Node{symbol: 9999, flags: nodeFlagNamed})
	w.iterators = w.iterators[:cap(w.iterators)]
	for i := range w.iterators {
		w.iterators[i] = cWaveIterator{node: junkNode, link: 7, popped: 99, subtreeCount: 5}
	}
	w.links = w.links[:cap(w.links)]
	for i := range w.links {
		w.links[i] = cWaveLink{entry: junkEntry, prev: 3}
	}
	w.windows = w.windows[:cap(w.windows)]
	for i := range w.windows {
		w.windows[i] = junkEntry
	}
	w.forks = w.forks[:cap(w.forks)]
	for i := range w.forks {
		w.forks[i] = reduceFork{window: []stackEntry{junkEntry}, topState: 9999, popTo: junkNode}
	}
	w.iterators, w.links, w.windows, w.forks = w.iterators[:0], w.links[:0], w.windows[:0], w.forks[:0]
}

// TestCWaveScratchMatchesOriginalOnRandomPackedGraphs compares the scratch-backed
// enumeration, its allocating wrapper, and the in-place fork selection with the
// code they replaced, on 30,000 random packed graphs. One scratch serves every
// trial, and it is filled with leftovers before each call, so state carried over
// from an earlier call would show. The selection must make the same preference
// calls, in the same order, and keep the same forks.
func TestCWaveScratchMatchesOriginalOnRandomPackedGraphs(t *testing.T) {
	const trials = 30000
	// The seed is fixed, so -count=N replays the same 30,000 graphs.
	gen := cWaveGraphGen{rng: rand.New(rand.NewSource(20261101))}
	var scratch cWaveScratch
	cover := map[string]int{}
	for trial := 0; trial < trials; trial++ {
		head := gen.graph()
		childCount := gen.rng.Intn(10) - 1
		s := glrStack{gss: gssStack{head: head}}
		desc := fmt.Sprintf("trial %d, child count %d", trial, childCount)

		want := cWaveReduceWindowsOriginal(&s, childCount)
		cWaveScribble(&scratch)
		scratch.usedIterators = 0 // read below as this trial's peak
		got := cWaveReduceWindowsFromGSSInto(&s, childCount, &scratch)
		if msg := cWaveForksDiff(want, got); msg != "" {
			t.Fatalf("%s: scratch enumeration: %s", desc, msg)
		}
		if msg := cWaveForksDiff(want, cWaveReduceWindowsFromGSS(&s, childCount)); msg != "" {
			t.Fatalf("%s: wrapper enumeration: %s", desc, msg)
		}

		var wantLog, gotLog []cWavePreferCall
		wantKept, wantGroups, wantAmbiguous := cWaveSelectOriginal(want, cWavePreferLogging(&wantLog))
		var gotGroups [][]reduceFork
		gotKept, gotAmbiguous := selectCWaveReduceForks(got, cWavePreferLogging(&gotLog), &gotGroups)
		if msg := cWaveForksDiff(wantKept, gotKept); msg != "" {
			t.Fatalf("%s: selected forks: %s", desc, msg)
		}
		if wantAmbiguous != gotAmbiguous {
			t.Fatalf("%s: ambiguous = %t, want %t", desc, gotAmbiguous, wantAmbiguous)
		}
		if len(wantLog) != len(gotLog) {
			t.Fatalf("%s: %d preference calls, want %d", desc, len(gotLog), len(wantLog))
		}
		for i := range wantLog {
			if wantLog[i] != gotLog[i] {
				t.Fatalf("%s: preference call %d = %+v, want %+v", desc, i, gotLog[i], wantLog[i])
			}
		}
		if workCountInstrumentationEnabled {
			// Work-count receipts read every run after the selection has
			// overwritten the front of the enumeration.
			if len(wantGroups) != len(gotGroups) {
				t.Fatalf("%s: %d groups, want %d", desc, len(gotGroups), len(wantGroups))
			}
			for i := range wantGroups {
				if msg := cWaveForksDiff(wantGroups[i], gotGroups[i]); msg != "" {
					t.Fatalf("%s: group %d: %s", desc, i, msg)
				}
			}
		} else if gotGroups != nil {
			t.Fatalf("%s: production build copied %d groups", desc, len(gotGroups))
		}

		if len(want) > 0 {
			cover["trials with forks"]++
		}
		if childCount == -1 && len(want) > 0 {
			cover["pop-all trials with forks"]++
		}
		if wantAmbiguous {
			cover["trials with a multi-fork run"]++
		}
		for _, call := range wantLog {
			switch {
			case call.candidate < call.incumbent:
				cover["preference calls where the candidate won"]++
			case call.candidate == call.incumbent:
				cover["preference calls that tied"]++
			default:
				cover["preference calls where the incumbent won"]++
			}
		}
		if scratch.usedIterators == cStackMaxIteratorCount {
			cover["trials at the iterator cap"]++
		}
	}
	// The comparison is only worth something if the trials reach these paths.
	t.Logf("coverage over %d trials: %v", trials, cover)
	for _, name := range []string{
		"trials with forks", "pop-all trials with forks", "trials with a multi-fork run",
		"preference calls where the candidate won", "preference calls that tied",
		"preference calls where the incumbent won", "trials at the iterator cap",
	} {
		if cover[name] < trials/200 {
			t.Errorf("only %d of %d trials covered: %s", cover[name], trials, name)
		}
	}
}

// TestCWaveReduceWindowsFromGSSResultsBelongToTheCaller pins the contract that
// the recovery callers rely on: they hold the forks across further reductions,
// so a later call of the allocating form must not change an earlier result.
func TestCWaveReduceWindowsFromGSSResultsBelongToTheCaller(t *testing.T) {
	gen := cWaveGraphGen{rng: rand.New(rand.NewSource(7))}
	for trial := 0; trial < 200; trial++ {
		first := glrStack{gss: gssStack{head: gen.graph()}}
		second := glrStack{gss: gssStack{head: gen.graph()}}
		childCount := 1 + gen.rng.Intn(4)
		firstForks := cWaveReduceWindowsFromGSS(&first, childCount)
		wantFirst := cWaveReduceWindowsOriginal(&first, childCount)

		secondForks := cWaveReduceWindowsFromGSS(&second, childCount)
		for _, fork := range secondForks {
			for j := range fork.window {
				fork.window[j] = stackEntry{state: 4242}
			}
		}
		if msg := cWaveForksDiff(wantFirst, firstForks); msg != "" {
			t.Fatalf("trial %d: first result changed after a later call: %s", trial, msg)
		}
	}
}

// TestCWaveScratchWindowsDoNotOverlap checks that the windows of one call are
// disjoint slices of the scratch: writing to a window, or appending to one,
// leaves every other window as it was.
func TestCWaveScratchWindowsDoNotOverlap(t *testing.T) {
	gen := cWaveGraphGen{rng: rand.New(rand.NewSource(11))}
	var scratch cWaveScratch
	checked := 0
	for trial := 0; trial < 5000 && checked < 200; trial++ {
		s := glrStack{gss: gssStack{head: gen.graph()}}
		forks := cWaveReduceWindowsFromGSSInto(&s, 2+gen.rng.Intn(3), &scratch)
		if len(forks) < 3 {
			continue
		}
		checked++
		snapshot := make([][]stackEntry, len(forks))
		for i, fork := range forks {
			snapshot[i] = append([]stackEntry(nil), fork.window...)
		}
		unchanged := func(except int, action string) {
			t.Helper()
			for k, fork := range forks {
				if k == except {
					continue
				}
				for j, e := range fork.window {
					if e != snapshot[k][j] {
						t.Fatalf("trial %d: %s window %d changed window %d entry %d", trial, action, except, k, j)
					}
				}
			}
		}
		for i, fork := range forks {
			grown := append(fork.window, stackEntry{state: 6666})
			grown[len(grown)-1].state = 6667
			unchanged(i, "appending to")
			for j := range fork.window {
				fork.window[j] = stackEntry{state: StateID(7000 + i)}
			}
			unchanged(i, "writing")
			copy(fork.window, snapshot[i])
		}
	}
	if checked < 200 {
		t.Fatalf("only %d trials produced three or more windows", checked)
	}
}

// TestCWaveScratchResetDropsReferences checks that reset leaves no pointer in a
// buffer, that a gssScratch reset reaches the wave scratch, and that a buffer
// which outgrew the retention limit is released instead of pooled.
func TestCWaveScratchResetDropsReferences(t *testing.T) {
	assertClear := func(t *testing.T, w *cWaveScratch, when string) {
		t.Helper()
		for i, it := range w.iterators[:cap(w.iterators)] {
			if it != (cWaveIterator{}) {
				t.Fatalf("%s: iterator slot %d kept %+v", when, i, it)
			}
		}
		for i, l := range w.links[:cap(w.links)] {
			if l != (cWaveLink{}) {
				t.Fatalf("%s: link slot %d kept %+v", when, i, l)
			}
		}
		for i, e := range w.windows[:cap(w.windows)] {
			if e != (stackEntry{}) {
				t.Fatalf("%s: window slot %d kept %+v", when, i, e)
			}
		}
		for i, f := range w.forks[:cap(w.forks)] {
			if f.window != nil || f.popTo != nil || f.topState != 0 {
				t.Fatalf("%s: fork slot %d kept %+v", when, i, f)
			}
		}
		if len(w.iterators)+len(w.links)+len(w.windows)+len(w.forks) != 0 {
			t.Fatalf("%s: buffers not emptied", when)
		}
	}

	gen := cWaveGraphGen{rng: rand.New(rand.NewSource(13))}
	var scratch cWaveScratch
	for trial := 0; trial < 300; trial++ {
		s := glrStack{gss: gssStack{head: gen.graph()}}
		forks := cWaveReduceWindowsFromGSSInto(&s, gen.rng.Intn(6), &scratch)
		// A caller that has consumed the forks may reorder them in place.
		kept, _ := selectCWaveReduceForks(forks, func(candidate, incumbent reduceFork) int { return -1 }, nil)
		_ = kept
		scratch.reset()
		assertClear(t, &scratch, fmt.Sprintf("trial %d after cWaveScratch.reset", trial))
	}

	var owner gssScratch
	s := glrStack{gss: gssStack{head: gen.graph()}}
	cWaveReduceWindowsFromGSSInto(&s, 2, &owner.cWave)
	owner.reset()
	assertClear(t, &owner.cWave, "after gssScratch.reset")

	// A pop-all of a chain deeper than the retention limit grows every buffer
	// past it. The buffer is released, not kept.
	var long gssScratch
	var chain gssStack
	for i := 0; i < 3*maxRetainedCWaveScratch; i++ {
		chain.pushEntry(newStackEntryNode(StateID(i%40), &Node{symbol: Symbol(i + 1), flags: nodeFlagNamed}), &long)
	}
	deep := glrStack{gss: chain}
	want := cWaveReduceWindowsOriginal(&deep, -1)
	got := cWaveReduceWindowsFromGSSInto(&deep, -1, &long.cWave)
	if msg := cWaveForksDiff(want, got); msg != "" {
		t.Fatalf("deep pop-all: %s", msg)
	}
	if len(got) != 1 || len(got[0].window) <= maxRetainedCWaveScratch {
		t.Fatalf("deep pop-all: %d forks, want one window longer than the retention limit %d", len(got), maxRetainedCWaveScratch)
	}
	long.reset()
	if long.cWave.windows != nil || long.cWave.links != nil {
		t.Fatalf("reset kept an oversized buffer: windows cap %d, links cap %d", cap(long.cWave.windows), cap(long.cWave.links))
	}
	assertClear(t, &long.cWave, "after the deep pop-all")
}

// TestSelectCWaveReduceForksKeepsOneForkPerRun walks the selection rules on
// small inputs: runs are split by pop target, the first fork wins ties, a later
// fork wins only when the preference is negative, and the kept forks are
// written over the front of the input in run order.
func TestSelectCWaveReduceForksKeepsOneForkPerRun(t *testing.T) {
	a, b, c := &gssNode{entry: stackEntry{state: 1}}, &gssNode{entry: stackEntry{state: 2}}, &gssNode{entry: stackEntry{state: 3}}
	fork := func(popTo *gssNode, id StateID) reduceFork {
		return reduceFork{window: []stackEntry{{state: id}}, topState: popTo.entry.state, popTo: popTo}
	}
	// score(id) is the preference rank: the lower id wins.
	prefer := func(candidate, incumbent reduceFork) int {
		return int(candidate.window[0].state) - int(incumbent.window[0].state)
	}
	ids := func(forks []reduceFork) []StateID {
		var out []StateID
		for _, f := range forks {
			out = append(out, f.window[0].state)
		}
		return out
	}
	for _, tc := range []struct {
		name          string
		in            []reduceFork
		want          []StateID
		wantAmbiguous bool
	}{
		{"empty", nil, nil, false},
		{"one fork", []reduceFork{fork(a, 10)}, []StateID{10}, false},
		{"each pop target alone", []reduceFork{fork(a, 10), fork(b, 11), fork(c, 12)}, []StateID{10, 11, 12}, false},
		{"first of a run stays when later forks lose", []reduceFork{fork(a, 10), fork(a, 11), fork(a, 12)}, []StateID{10}, true},
		{"a later fork wins when preferred", []reduceFork{fork(a, 12), fork(a, 11), fork(a, 13)}, []StateID{11}, true},
		{"the last fork can win", []reduceFork{fork(a, 15), fork(a, 14), fork(a, 13)}, []StateID{13}, true},
		{"runs are chosen independently", []reduceFork{fork(a, 12), fork(a, 10), fork(b, 11), fork(c, 14), fork(c, 13)}, []StateID{10, 11, 13}, true},
		{"an equal fork does not replace the first", []reduceFork{fork(a, 10), fork(a, 10)}, []StateID{10}, true},
		{"a repeated pop target after another one starts a new run", []reduceFork{fork(a, 10), fork(b, 11), fork(a, 9)}, []StateID{10, 11, 9}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := append([]reduceFork(nil), tc.in...)
			kept, ambiguous := selectCWaveReduceForks(in, prefer, nil)
			got := ids(kept)
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("kept %v, want %v", got, tc.want)
			}
			if ambiguous != tc.wantAmbiguous {
				t.Fatalf("ambiguous = %t, want %t", ambiguous, tc.wantAmbiguous)
			}
			if len(kept) > 0 && &kept[0] != &in[0] {
				t.Fatal("the kept forks do not start at the front of the input")
			}
		})
	}
}

// TestSelectCWaveReduceForksCopiesGroupsForWorkCountBuilds pins the work-count
// path of the selection: the runs it reports are copies, so the in-place writes
// that follow do not change them. Production builds copy nothing.
func TestSelectCWaveReduceForksCopiesGroupsForWorkCountBuilds(t *testing.T) {
	a, b := &gssNode{entry: stackEntry{state: 1}}, &gssNode{entry: stackEntry{state: 2}}
	fork := func(popTo *gssNode, id StateID) reduceFork {
		return reduceFork{window: []stackEntry{{state: id}}, topState: popTo.entry.state, popTo: popTo}
	}
	in := []reduceFork{fork(a, 13), fork(a, 11), fork(a, 12), fork(b, 21), fork(b, 20)}
	orig := append([]reduceFork(nil), in...)
	var groups [][]reduceFork
	kept, _ := selectCWaveReduceForks(in, func(candidate, incumbent reduceFork) int {
		return int(candidate.window[0].state) - int(incumbent.window[0].state)
	}, &groups)
	if len(kept) != 2 || kept[0].window[0].state != 11 || kept[1].window[0].state != 20 {
		t.Fatalf("kept %+v, want the forks with ids 11 and 20", kept)
	}
	if !workCountInstrumentationEnabled {
		if groups != nil {
			t.Fatalf("a production build copied %d groups", len(groups))
		}
		return
	}
	if len(groups) != 2 || len(groups[0]) != 3 || len(groups[1]) != 2 {
		t.Fatalf("groups = %+v, want runs of 3 and 2", groups)
	}
	for i, start := range []int{0, 3} {
		if msg := cWaveForksDiff(orig[start:start+len(groups[i])], groups[i]); msg != "" {
			t.Fatalf("group %d changed after the selection: %s", i, msg)
		}
	}
}

// cWavePackedReduceFixture builds a packed GSS for a two-child reduce with three
// pop paths. Two paths end at the same pop target and so form one run: the
// alternate right child wins that run when altPrecedence is higher than the
// primary's (zero). The third path ends at a pop target of its own.
func cWavePackedReduceFixture(arena *nodeArena, scratch *gssScratch, altPrecedence int32) (*Parser, glrStack, ParseAction) {
	leaf := func(symbol Symbol, start, end uint32) *Node {
		return newLeafNodeInArena(arena, symbol, true, start, end, Point{Column: start}, Point{Column: end})
	}
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	otherBase := scratch.allocNode(stackEntry{state: 11}, nil, 1)
	left := scratch.allocNode(newStackEntryNode(2, leaf(1, 0, 1)), base, 2)
	otherLeft := scratch.allocNode(newStackEntryNode(2, leaf(1, 0, 1)), otherBase, 2)
	head := scratch.allocNode(newStackEntryNode(3, leaf(2, 1, 2)), left, 3)
	alt := leaf(2, 1, 2)
	alt.dynamicPrecedence = altPrecedence
	head.appendExtraLink(gssMainLink{prev: left, entry: newStackEntryNode(3, alt)})
	head.appendExtraLink(gssMainLink{prev: otherLeft, entry: newStackEntryNode(3, leaf(2, 1, 2))})

	parser := &Parser{language: &Language{
		CompactPackedGSSVersionOrderCertified: true,
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "left", Visible: true, Named: true},
			{Name: "right", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}}
	activateCompactPackedGSSVersionOrderForTest(parser, arena)
	parser.disablePostReduceForkMerge = true
	stack := glrStack{gss: gssStack{head: head}, byteOffset: 2}
	return parser, stack, ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2, DynamicPrecedence: 4}
}

// cWaveDescribeReduceResult writes down what a reduction left behind: for the
// reduced stack and every queued fork, the new top node, its children, its
// precedence and its raw-shape header.
func cWaveDescribeReduceResult(arena *nodeArena, parser *Parser, s *glrStack) string {
	describe := func(label string, g *glrStack) string {
		if g.gss.head == nil {
			return label + ": no head\n"
		}
		parent := stackEntryNode(g.gss.head.entry)
		if parent == nil {
			return fmt.Sprintf("%s: head %p without a node\n", label, g.gss.head)
		}
		shape, _ := arena.rawShapeForRef(parent.rawShape)
		return fmt.Sprintf("%s: head=%p prev=%p symbol=%d state=%d prec=%d children=%v shape=%+v dead=%t\n",
			label, g.gss.head, g.gss.head.prev, parent.symbol, g.gss.head.entry.state, parent.dynamicPrecedence, parent.children, shape, g.dead)
	}
	out := describe("primary", s)
	for i := range parser.pendingForkStacks {
		out += describe(fmt.Sprintf("pending %d", i), &parser.pendingForkStacks[i])
	}
	return out
}

// TestApplyReduceActionForkedScratchReuseKeepsEarlierResults reduces two packed
// stacks one after the other through one gssScratch. The second reduction
// overwrites the wave buffers the first one used. The first one's results must
// still read the same, and match a reduction that had a scratch of its own.
func TestApplyReduceActionForkedScratchReuseKeepsEarlierResults(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	reduce := func(scratch *gssScratch, altPrecedence int32) (*Parser, *glrStack) {
		parser, stack, act := cWavePackedReduceFixture(arena, scratch, altPrecedence)
		var anyReduced bool
		nodeCount := 0
		parser.applyReduceActionForked(nil, &stack, act, Token{}, &anyReduced, &nodeCount, arena, nil, scratch, nil, nil, false, false)
		if stack.dead || !anyReduced {
			t.Fatalf("reduction failed: dead=%t reduced=%t", stack.dead, anyReduced)
		}
		// The run of two paths has more than one fork, so the reduction must
		// flag the choice as policy-pruned. Nothing else in this fixture sets it.
		if !parser.mergeScratch.policyPruned {
			t.Fatal("a pop run with two forks did not set policyPruned")
		}
		// The reduction must have taken its buffers from the parse's scratch
		// and not from a call-local one.
		if scratch.cWave.usedWindows == 0 || scratch.cWave.usedForks == 0 {
			t.Fatalf("the reduction did not use the gssScratch wave buffers: %+v", scratch.cWave)
		}
		return parser, &stack
	}
	// strip makes two descriptions comparable across separate fixtures: node
	// and GSS addresses differ, the shape of the result must not.
	strip := func(parser *Parser, arena *nodeArena, s *glrStack) string {
		var out string
		for _, g := range append([]*glrStack{s}, pendingStackPointers(parser)...) {
			parent := stackEntryNode(g.gss.head.entry)
			var children []string
			for _, c := range parent.children {
				children = append(children, fmt.Sprintf("%d[%d,%d)p%d", c.symbol, c.startByte, c.endByte, c.dynamicPrecedence))
			}
			shape, _ := arena.rawShapeForRef(parent.rawShape)
			out += fmt.Sprintf("symbol=%d state=%d prec=%d children=%v shape=%d/%d\n", parent.symbol, g.gss.head.entry.state, parent.dynamicPrecedence, children, shape.symbol, shape.childCount())
		}
		return out
	}

	for _, altPrecedence := range []int32{0, 3} {
		var shared gssScratch
		firstParser, first := reduce(&shared, altPrecedence)
		before := cWaveDescribeReduceResult(arena, firstParser, first)
		stripped := strip(firstParser, arena, first)
		// The fixture must do what its comment says: the run of two paths yields
		// one winner, and the lone path becomes a queued fork.
		if len(firstParser.pendingForkStacks) != 1 {
			t.Fatalf("altPrecedence %d: %d queued forks, want 1", altPrecedence, len(firstParser.pendingForkStacks))
		}
		primaryRight := stackEntryNode(first.gss.head.entry).children[1]
		if primaryRight.dynamicPrecedence != altPrecedence {
			t.Fatalf("altPrecedence %d: the primary reduced right child %d, want the one with precedence %d", altPrecedence, primaryRight.dynamicPrecedence, altPrecedence)
		}

		// Reuse the scratch for a different reduction: the wave buffers are overwritten.
		reduce(&shared, 7-altPrecedence)

		if after := cWaveDescribeReduceResult(arena, firstParser, first); after != before {
			t.Fatalf("altPrecedence %d: the first result changed when the scratch was reused\nbefore:\n%safter:\n%s", altPrecedence, before, after)
		}
		var own gssScratch
		ownParser, ownStack := reduce(&own, altPrecedence)
		if got := strip(ownParser, arena, ownStack); got != stripped {
			t.Fatalf("altPrecedence %d: a reduction with a reused scratch differs from one with its own\nreused:\n%sown:\n%s", altPrecedence, stripped, got)
		}
	}
}

func pendingStackPointers(parser *Parser) []*glrStack {
	out := make([]*glrStack, len(parser.pendingForkStacks))
	for i := range parser.pendingForkStacks {
		out[i] = &parser.pendingForkStacks[i]
	}
	return out
}

// cWaveBenchStack builds a packed GSS for a five-child pop whose two lowest
// layers each carry an alternate link, so the pop yields several windows. With
// wide set, three layers of C-limit width give the pop 64 windows instead.
func cWaveBenchStack(scratch *gssScratch, wide bool) (glrStack, int) {
	node := func(state StateID, prev *gssNode, symbol Symbol) *gssNode {
		depth := uint32(1)
		if prev != nil {
			depth = prev.depth + 1
		}
		return scratch.allocNode(newStackEntryNode(state, &Node{symbol: symbol, flags: nodeFlagNamed}), prev, depth)
	}
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	if wide {
		lowest := node(10, base, 1)
		for i := 1; i < maxCMainLinkCount; i++ {
			lowest.appendExtraLinkWithLimitAndOwner(gssMainLink{prev: base, entry: newStackEntryNode(StateID(10+i), &Node{symbol: Symbol(1 + i), flags: nodeFlagNamed})}, maxCMainLinkCount, nil)
		}
		middle := node(30, lowest, 20)
		for i := 1; i < maxCMainLinkCount; i++ {
			middle.appendExtraLinkWithLimitAndOwner(gssMainLink{prev: lowest, entry: newStackEntryNode(StateID(30+i), &Node{symbol: Symbol(20 + i), flags: nodeFlagNamed})}, maxCMainLinkCount, nil)
		}
		head := node(50, middle, 40)
		for i := 1; i < maxCMainLinkCount; i++ {
			head.appendExtraLinkWithLimitAndOwner(gssMainLink{prev: middle, entry: newStackEntryNode(StateID(50+i), &Node{symbol: Symbol(40 + i), flags: nodeFlagNamed})}, maxCMainLinkCount, nil)
		}
		return glrStack{gss: gssStack{head: head}}, 3
	}
	a := node(2, base, 2)
	a2 := node(3, base, 3)
	b := node(4, a, 4)
	b.appendExtraLinkWithLimitAndOwner(gssMainLink{prev: a2, entry: newStackEntryNode(4, &Node{symbol: 5, flags: nodeFlagNamed})}, maxCMainLinkCount, nil)
	c := node(5, b, 6)
	d := node(6, c, 7)
	e := node(7, d, 8)
	return glrStack{gss: gssStack{head: e}}, 5
}

// BenchmarkCWaveReduceWindows measures one wave enumeration three ways: the
// allocating original, the allocating wrapper the recovery callers use, and the
// scratch-backed call that applyReduceActionForked makes on a warm scratch.
func BenchmarkCWaveReduceWindows(b *testing.B) {
	for _, shape := range []struct {
		name string
		wide bool
	}{{"narrow", false}, {"wide", true}} {
		var scratch gssScratch
		s, childCount := cWaveBenchStack(&scratch, shape.wide)
		want := len(cWaveReduceWindowsOriginal(&s, childCount))
		b.Run(shape.name+"/original", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if len(cWaveReduceWindowsOriginal(&s, childCount)) != want {
					b.Fatal("wrong fork count")
				}
			}
		})
		b.Run(shape.name+"/wrapper", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if len(cWaveReduceWindowsFromGSS(&s, childCount)) != want {
					b.Fatal("wrong fork count")
				}
			}
		})
		b.Run(shape.name+"/scratch", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if len(cWaveReduceWindowsFromGSSInto(&s, childCount, &scratch.cWave)) != want {
					b.Fatal("wrong fork count")
				}
			}
		})
	}
}

// TestCWaveEnumerationAndSelectionAllocateNothingOnAWarmScratch pins the point
// of the scratch: once the buffers have grown, one enumeration and one fork
// selection make no heap allocation. The selection gets no groups slice, as in
// production builds.
func TestCWaveEnumerationAndSelectionAllocateNothingOnAWarmScratch(t *testing.T) {
	prefer := func(candidate, incumbent reduceFork) int { return 0 }
	for _, shape := range []struct {
		name string
		wide bool
	}{{"narrow", false}, {"wide", true}} {
		var scratch gssScratch
		s, childCount := cWaveBenchStack(&scratch, shape.wide)
		run := func() int {
			all := cWaveReduceWindowsFromGSSInto(&s, childCount, &scratch.cWave)
			kept, _ := selectCWaveReduceForks(all, prefer, nil)
			return len(kept)
		}
		want := run() // grows the buffers
		if want == 0 {
			t.Fatalf("%s: no forks", shape.name)
		}
		if got := testing.AllocsPerRun(50, func() {
			if run() != want {
				panic("fork count changed")
			}
		}); got != 0 {
			t.Errorf("%s: %v allocations per warm enumeration and selection, want 0", shape.name, got)
		}
	}
}
