package gotreesitter

import "testing"

func TestApplyActionTerminalExtraShiftPreservesCurrentState(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	tok := Token{
		Symbol:     2,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}

	parser.applyAction(nil, &s, ParseAction{
		Type:  ParseActionShift,
		State: 0,
		Extra: true,
	}, tok, &anyReduced, &nodeCount, nil, nil, nil, nil, false, nil)

	if got, want := s.top().state, lang.InitialState; got != want {
		t.Fatalf("top state = %d, want %d", got, want)
	}
	if got, want := stackEntryNode(s.top()).parseState, lang.InitialState; got != want {
		t.Fatalf("extra leaf parseState = %d, want %d", got, want)
	}
	if got, want := stackEntryNode(s.top()).preGotoState, lang.InitialState; got != want {
		t.Fatalf("extra leaf preGotoState = %d, want %d", got, want)
	}
}

func TestApplyActionNonterminalExtraShiftUsesActionState(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     2,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}

	parser.applyAction(nil, &s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
		Extra: true,
	}, tok, &anyReduced, &nodeCount, nil, nil, nil, nil, false, nil)

	if got := s.top().state; got != targetState {
		t.Fatalf("top state = %d, want %d", got, targetState)
	}
	if got := stackEntryNode(s.top()).parseState; got != targetState {
		t.Fatalf("extra leaf parseState = %d, want %d", got, targetState)
	}
	if got, want := stackEntryNode(s.top()).preGotoState, lang.InitialState; got != want {
		t.Fatalf("extra leaf preGotoState = %d, want %d", got, want)
	}
}

func TestApplyExtraShiftMissingMarksChildErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		language string
		explicit bool
	}{
		{name: "explicit missing", explicit: true},
		{name: "C zero-width missing", language: "c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := buildArithmeticLanguage()
			lang.Name = tc.language
			parser := NewParser(lang)
			arena := newNodeArena(arenaClassFull)
			stack := newGLRStack(lang.InitialState)
			scratch := &parserScratch{}
			trackChildErrors := false
			tok := Token{
				Symbol:    1,
				StartByte: 4,
				EndByte:   4,
				Missing:   tc.explicit,
			}

			parser.applyExtraShiftAction(&stack, lang.InitialState, ParseAction{
				Type:  ParseActionShift,
				Extra: true,
			}, tok, arena, scratch, &trackChildErrors)

			if !trackChildErrors {
				t.Fatal("missing extra shift did not mark child-error tracking")
			}
			leaf := stackEntryNode(stack.top())
			if leaf == nil || !leaf.isMissing() || !leaf.hasError() {
				t.Fatalf("missing extra leaf = %#v", leaf)
			}
		})
	}
}

// TestApplyExtraShiftMissingRecordsDependencyWithoutSearching pins that an
// extra missing leaf, like any shifted missing leaf, is recorded without a
// search of the arena's list (issue #1556).
func TestApplyExtraShiftMissingRecordsDependencyWithoutSearching(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	planted := plantMissingNodeDependencyForNextNode(t, arena)
	stack := newGLRStack(lang.InitialState)
	trackChildErrors := false
	tok := Token{
		Symbol: 1, StartByte: 3, EndByte: 3,
		StartPoint: Point{Column: 3}, EndPoint: Point{Column: 3}, Missing: true,
		missingStackRef:       parser.recordMissingStackAnchor(0, Point{}),
		lexerLookaheadEndByte: 5, lexFlags: tokenFlagMissingDependencyExact,
	}

	parser.applyExtraShiftAction(&stack, lang.InitialState, ParseAction{
		Type:  ParseActionShift,
		Extra: true,
	}, tok, arena, &parserScratch{}, &trackChildErrors)
	if !parser.retainExternalFallbackMissingFlags {
		t.Fatal("missing extra shift did not retain recovery flags")
	}

	want, exact := parser.missingNodeDependencyFromToken(tok)
	if !exact {
		t.Fatal("the token's dependency is not exact")
	}
	checkPlantedMissingNodeDependencyKept(t, arena, planted, stackEntryNode(stack.top()), want)
}

func TestApplyActionNonExtraShiftUsesActionState(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     1,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}

	parser.applyAction(nil, &s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
	}, tok, &anyReduced, &nodeCount, nil, nil, nil, nil, false, nil)

	if got := s.top().state; got != targetState {
		t.Fatalf("top state = %d, want %d", got, targetState)
	}
	if got := stackEntryNode(s.top()).parseState; got != targetState {
		t.Fatalf("leaf parseState = %d, want %d", got, targetState)
	}
}

func TestApplyActionNoTreeCompactShiftUsesNoTreeNode(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	parser.noTreeBenchmarkOnly = true
	parser.compactNoTreeShiftLeaves = true

	arena := newNodeArena(arenaClassFull)
	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     1,
		StartByte:  2,
		EndByte:    3,
		StartPoint: Point{Row: 0, Column: 2},
		EndPoint:   Point{Row: 0, Column: 3},
	}

	parser.applyAction(nil, &s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
	}, tok, &anyReduced, &nodeCount, arena, nil, nil, nil, false, nil)

	if got := stackEntryNode(s.top()); got != nil {
		t.Fatalf("stackEntryNode = %p, want nil", got)
	}
	leaf := stackEntryNoTreeNode(s.top())
	if leaf == nil {
		t.Fatal("stackEntryNoTreeNode = nil, want compact leaf")
	}
	if got := leaf.parseState; got != targetState {
		t.Fatalf("compact leaf parseState = %d, want %d", got, targetState)
	}
	if got, want := leaf.preGotoState, lang.InitialState; got != want {
		t.Fatalf("compact leaf preGotoState = %d, want %d", got, want)
	}
	if got := arena.leafNodesConstructed; got != 0 {
		t.Fatalf("leafNodesConstructed = %d, want 0", got)
	}
	if got := arena.noTreeLeafNodesConstructed; got != 1 {
		t.Fatalf("noTreeLeafNodesConstructed = %d, want 1", got)
	}
}

func TestApplyActionCompactFullShiftUsesCompactLeaf(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	parser.noResultCompatibilityBenchmarkOnly = true
	parser.compactFullShiftLeaves = true

	arena := newNodeArena(arenaClassFull)
	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     1,
		StartByte:  2,
		EndByte:    3,
		StartPoint: Point{Row: 0, Column: 2},
		EndPoint:   Point{Row: 0, Column: 3},
	}

	parser.applyAction(nil, &s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
	}, tok, &anyReduced, &nodeCount, arena, nil, nil, nil, false, nil)

	if got := stackEntryNode(s.top()); got != nil {
		t.Fatalf("stackEntryNode = %p, want nil", got)
	}
	leaf := stackEntryCompactFullLeaf(s.top())
	if leaf == nil {
		t.Fatal("stackEntryCompactFullLeaf = nil, want compact full leaf")
	}
	if got := leaf.parseState; got != targetState {
		t.Fatalf("compact leaf parseState = %d, want %d", got, targetState)
	}
	if got := arena.leafNodesConstructed; got != 0 {
		t.Fatalf("leafNodesConstructed = %d, want 0", got)
	}
	if got := arena.compactFullLeafCreated; got != 1 {
		t.Fatalf("compactFullLeafCreated = %d, want 1", got)
	}
}

func TestApplyActionNoTreeMissingShiftStaysNode(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	parser.noTreeBenchmarkOnly = true
	parser.compactNoTreeShiftLeaves = true

	arena := newNodeArena(arenaClassFull)
	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     1,
		StartByte:  2,
		EndByte:    2,
		StartPoint: Point{Row: 0, Column: 2},
		EndPoint:   Point{Row: 0, Column: 2},
		Missing:    true,
	}

	parser.applyAction(nil, &s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
	}, tok, &anyReduced, &nodeCount, arena, nil, nil, nil, false, nil)

	leaf := stackEntryNode(s.top())
	if leaf == nil {
		t.Fatal("stackEntryNode = nil, want full missing leaf")
	}
	if compact := stackEntryNoTreeNode(s.top()); compact != nil {
		t.Fatalf("stackEntryNoTreeNode = %p, want nil", compact)
	}
	if !leaf.isMissing() || !leaf.hasError() {
		t.Fatalf("missing leaf flags: missing=%v hasError=%v, want both true", leaf.isMissing(), leaf.hasError())
	}
	if got := arena.leafNodesConstructed; got != 1 {
		t.Fatalf("leafNodesConstructed = %d, want 1", got)
	}
	if got := arena.noTreeLeafNodesConstructed; got != 0 {
		t.Fatalf("noTreeLeafNodesConstructed = %d, want 0", got)
	}
}
