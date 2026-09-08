package gotreesitter

import "testing"

func TestRealTokenAttachmentGapHonorsLexerSkipProvenanceWithIncludedRange(t *testing.T) {
	source := []byte("a\n * b")
	stack := newGLRStack(1)
	stack.byteOffset = 1
	tok := Token{
		Symbol:                  1,
		StartByte:               5,
		EndByte:                 6,
		StartPoint:              Point{Row: 1, Column: 3},
		EndPoint:                Point{Row: 1, Column: 4},
		lexFlags:                tokenFlagSkippedPrefix,
		lexerSkippedPrefixStart: 1,
	}
	parser := &Parser{included: []Range{{StartByte: 1, EndByte: uint32(len(source))}}}
	if !parser.guardRealTokenAttachmentGap(source, &stack, tok, "included-range") {
		t.Fatal("guardRealTokenAttachmentGap = false, want true for lexer skip provenance")
	}
	if stack.dead {
		t.Fatal("stack.dead = true, want false for lexer skip provenance")
	}

	badStack := newGLRStack(1)
	badStack.byteOffset = 1
	tok.lexerSkippedPrefixStart = 0
	if (&Parser{}).guardRealTokenAttachmentGap(source, &badStack, tok, "included-range") {
		t.Fatal("guardRealTokenAttachmentGap = true, want false for mismatched skip provenance")
	}
	if !badStack.dead {
		t.Fatal("badStack.dead = false, want true for mismatched skip provenance")
	}
}

func TestNormalizeRootSourceStartRequiresAcceptedSkipProvenance(t *testing.T) {
	source := []byte{1, '0'}
	newTree := func(provenance bool) *Node {
		arena := newNodeArena(arenaClassFull)
		child := newLeafNodeInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
		child.setLexerSkippedPrefixAtSourceStart(provenance)
		root := newParentNodeInArena(arena, 2, true, []*Node{child}, nil, 0)
		root.startByte = 1
		root.startPoint = Point{Column: 1}
		return root
	}

	accepted := newTree(true)
	(&Parser{}).normalizeRootSourceStart(accepted, source)
	if accepted.startByte != 1 || accepted.startPoint != (Point{Column: 1}) {
		t.Fatalf("accepted root span moved across leading skipped byte: %d at %#v", accepted.startByte, accepted.startPoint)
	}

	unproven := newTree(false)
	(&Parser{}).normalizeRootSourceStart(unproven, source)
	if unproven.startByte != 0 || unproven.startPoint != (Point{}) {
		t.Fatalf("unproven root span = %d at %#v, want pullback to 0", unproven.startByte, unproven.startPoint)
	}
}

func TestLeadingSkipProvenanceSurvivesCompactAndPendingPayloads(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	noTreeLeaf := newNoTreeLeafNodeInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	noTreeLeaf.setLexerSkippedPrefixAtSourceStart(true)
	if !stackEntryHasLeadingLexerSkippedPrefixAtSourceStart(newStackEntryNoTreeNode(1, noTreeLeaf), arena) {
		t.Fatal("no-tree leaf lost leading skip provenance")
	}

	compactLeaf := newCompactFullLeafInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	compactLeaf.setLexerSkippedPrefixAtSourceStart(true)
	inner := newPendingParentInArena(arena, 2, true, 0,
		[]stackEntry{newStackEntryCompactFullLeaf(1, compactLeaf)},
		1, 2, Point{Column: 1}, Point{Column: 2}, false)
	outer := newPendingParentInArena(arena, 3, true, 0,
		[]stackEntry{newStackEntryPendingParent(1, inner)},
		1, 2, Point{Column: 1}, Point{Column: 2}, false)
	root := newParentNodeInArenaWithFinalChildRefs(arena, 4, true, outer.childRange, 0, false)
	if !root.hasLeadingLexerSkippedPrefixAtSourceStart() {
		t.Fatal("compact leaf provenance did not survive pending and final-child payloads")
	}

	compactLeaf.setLexerSkippedPrefixAtSourceStart(false)
	if root.hasLeadingLexerSkippedPrefixAtSourceStart() {
		t.Fatal("unproven compact leaf retained leading skip provenance")
	}
}
