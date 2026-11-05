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
		missingStackRef:       parser.recordMissingStackAnchor(0, Point{}),
		lexerLookaheadEndByte: 5, lexFlags: tokenFlagMissingDependencyExact,
	})
	second := applyMissingLeafInternTestShift(t, parser, arena, act, Token{
		Symbol: 1, StartByte: 3, EndByte: 3,
		StartPoint: Point{Column: 3}, EndPoint: Point{Column: 3}, Missing: true,
		missingStackRef:       parser.recordMissingStackAnchor(1, Point{Column: 1}),
		lexerLookaheadEndByte: 6, lexFlags: tokenFlagMissingDependencyExact,
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
		missingStackRef:       parser.recordMissingStackAnchor(4, Point{Column: 4}),
		lexerLookaheadEndByte: 5, lexFlags: tokenFlagMissingDependencyExact,
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

// TestMissingLeafShiftRecordsDependencyWithoutSearching pins issue #1556: a
// shifted missing leaf is new, so applyShiftAction appends its entry without
// searching the arena's list. The planted entry names the node the arena
// allocates next. A search would rewrite it in place and leave one entry.
func TestMissingLeafShiftRecordsDependencyWithoutSearching(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	arena := newNodeArena(arenaClassFull)
	planted := plantMissingNodeDependencyForNextNode(t, arena)
	tok := Token{
		Symbol: 1, StartByte: 3, EndByte: 3,
		StartPoint: Point{Column: 3}, EndPoint: Point{Column: 3}, Missing: true,
		missingStackRef:       parser.recordMissingStackAnchor(0, Point{}),
		lexerLookaheadEndByte: 5, lexFlags: tokenFlagMissingDependencyExact,
	}
	leaf := applyMissingLeafInternTestShift(t, parser, arena, ParseAction{Type: ParseActionShift, State: 1}, tok)
	if !parser.retainExternalFallbackMissingFlags {
		t.Fatal("missing shift did not retain recovery flags")
	}
	want, exact := parser.missingNodeDependencyFromToken(tok)
	if !exact {
		t.Fatal("the token's dependency is not exact")
	}
	checkPlantedMissingNodeDependencyKept(t, arena, planted, leaf, want)
}

// TestMissingLeafShiftsRecordOneEntryEach pins that a run of missing-leaf
// shifts into one arena leaves one entry per leaf, in shift order, each equal
// to the dependency its token describes.
func TestMissingLeafShiftsRecordOneEntryEach(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	arena := newNodeArena(arenaClassFull)
	const leaves = 300
	nodes := make([]*Node, 0, leaves)
	wants := make([]missingNodeDependency, 0, leaves)
	for index := 0; index < leaves; index++ {
		stack := uint32(index)
		at := stack + 2
		tok := Token{
			Symbol: 1, StartByte: at, EndByte: at,
			StartPoint: Point{Column: at}, EndPoint: Point{Column: at}, Missing: true,
			missingStackRef:       parser.recordMissingStackAnchor(stack, Point{Column: stack}),
			lexerLookaheadEndByte: at + uint32(index%4), lexFlags: tokenFlagMissingDependencyExact,
		}
		want, exact := parser.missingNodeDependencyFromToken(tok)
		if !exact {
			t.Fatalf("token %d: dependency is not exact", index)
		}
		nodes = append(nodes, applyMissingLeafInternTestShift(t, parser, arena, ParseAction{Type: ParseActionShift, State: 1}, tok))
		wants = append(wants, want)
	}
	if got := len(arena.missingNodeDependencies); got != leaves {
		t.Fatalf("list has %d entries, want %d", got, leaves)
	}
	for index, entry := range arena.missingNodeDependencies {
		if entry.node != nodes[index] || entry.dependency != wants[index] {
			t.Fatalf("entry %d = %+v, want the dependency of shifted leaf %d, %+v", index, entry.dependency, index, wants[index])
		}
		if got, ok := missingNodeDependencyForNode(nodes[index]); !ok || got != wants[index] {
			t.Fatalf("leaf %d reads back as %+v exact=%t, want %+v", index, got, ok, wants[index])
		}
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
