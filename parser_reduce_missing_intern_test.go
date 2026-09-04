package gotreesitter

import "testing"

func TestMissingLeafShiftDoesNotInternDependencyProvenance(t *testing.T) {
	oldObserve, oldSubstitute := internLeavesObserveEnabled, internLeavesSubstituteEnabled
	t.Cleanup(func() {
		internLeavesObserveEnabled = oldObserve
		internLeavesSubstituteEnabled = oldSubstitute
	})
	internLeavesObserveEnabled = false
	internLeavesSubstituteEnabled = true

	parser := NewParser(buildArithmeticLanguage())
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	act := ParseAction{Type: ParseActionShift, State: 1}
	first := applyMissingLeafInternTestShift(t, parser, arena, act, Token{
		Symbol: 1, StartByte: 3, EndByte: 3,
		StartPoint: Point{Column: 3}, EndPoint: Point{Column: 3}, Missing: true,
		missingStackByte: 0, missingStackPoint: Point{},
		lexerLookaheadEndByte: 5, missingDependencyExact: true,
	})
	second := applyMissingLeafInternTestShift(t, parser, arena, act, Token{
		Symbol: 1, StartByte: 3, EndByte: 3,
		StartPoint: Point{Column: 3}, EndPoint: Point{Column: 3}, Missing: true,
		missingStackByte: 1, missingStackPoint: Point{Column: 1},
		lexerLookaheadEndByte: 6, missingDependencyExact: true,
	})
	if first == second {
		t.Fatal("missing leaves with different dependency provenance were canonicalized")
	}
	firstDependency, firstOK := missingNodeDependencyForNode(first)
	secondDependency, secondOK := missingNodeDependencyForNode(second)
	if !firstOK || !secondOK {
		t.Fatalf("missing dependency receipts: first=%+v exact=%t second=%+v exact=%t", firstDependency, firstOK, secondDependency, secondOK)
	}
	if firstDependency == secondDependency {
		t.Fatalf("missing dependency provenance collapsed: first=%+v second=%+v", firstDependency, secondDependency)
	}
	if arena.internLeavesFull != nil && arena.internLeavesFull.stats().Stores != 0 {
		t.Fatalf("missing leaf entered canonical table: %+v", arena.internLeavesFull.stats())
	}
}

func TestMissingLeafShiftSetterFailureDoesNotPublishCanonicalEntry(t *testing.T) {
	oldObserve, oldSubstitute := internLeavesObserveEnabled, internLeavesSubstituteEnabled
	t.Cleanup(func() {
		internLeavesObserveEnabled = oldObserve
		internLeavesSubstituteEnabled = oldSubstitute
	})
	internLeavesObserveEnabled = false
	internLeavesSubstituteEnabled = true

	parser := NewParser(buildArithmeticLanguage())
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	invalid := applyMissingLeafInternTestShift(t, parser, arena, ParseAction{Type: ParseActionShift, State: 1}, Token{
		Symbol: 1, StartByte: 3, EndByte: 3,
		StartPoint: Point{Column: 3}, EndPoint: Point{Column: 3}, Missing: true,
		missingStackByte: 4, missingStackPoint: Point{Column: 4},
		lexerLookaheadEndByte: 5, missingDependencyExact: true,
	})
	if !invalid.dirty() {
		t.Fatal("failed missing dependency write did not mark the leaf dirty")
	}
	if _, ok := missingNodeDependencyEntryForNode(invalid); ok {
		t.Fatal("failed missing dependency write published a sidecar receipt")
	}
	if arena.internLeavesFull != nil && arena.internLeavesFull.stats().Stores != 0 {
		t.Fatalf("failed missing leaf entered canonical table: %+v", arena.internLeavesFull.stats())
	}
}

func applyMissingLeafInternTestShift(t *testing.T, parser *Parser, arena *nodeArena, act ParseAction, tok Token) *Node {
	t.Helper()
	stack := newGLRStack(0)
	nodeCount := 0
	trackChildErrors := false
	parser.applyShiftAction(&stack, act, tok, &nodeCount, arena, nil, nil, &trackChildErrors)
	node := stackEntryNode(stack.top())
	if node == nil {
		t.Fatal("missing shift did not push a node")
	}
	return node
}
