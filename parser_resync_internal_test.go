package gotreesitter

import "testing"

func buildStructuralTopLevelResyncTestLanguage() *Language {
	return &Language{
		Name:              "structural_resync_test",
		SymbolCount:       5,
		TokenCount:        4,
		StateCount:        2,
		InitialState:      0,
		ProductionIDCount: 1,
		SymbolNames:       []string{"EOF", "ERROR", "bad", "next", "partial"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "ERROR", Visible: true, Named: true},
			{Name: "bad", Visible: true, Named: true},
			{Name: "next", Visible: true, Named: true},
			{Name: "partial", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: [][]uint16{
			{0, 0, 0, 1, 0},
			{2, 0, 0, 0, 0},
		},
		LexModes:  []LexMode{{LexState: 0}, {LexState: 0}},
		LexStates: []LexState{{Default: -1, EOF: -1}},
	}
}

func structuralTopLevelResyncTestStack(parser *Parser, arena *nodeArena) (glrStack, glrEntryScratch, gssScratch) {
	lang := parser.language
	s := newGLRStack(lang.InitialState)
	failed := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
	failed.setHasError(true)
	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	parser.pushStackNode(&s, 1, failed, &entryScratch, &gssScratch)
	return s, entryScratch, gssScratch
}

func TestRetryStructuralTopLevelResyncAllowsAdvance(t *testing.T) {
	parser := NewParser(buildStructuralTopLevelResyncTestLanguage())
	parser.retryStructuralTopLevelResync = true
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	s, entryScratch, gssScratch := structuralTopLevelResyncTestStack(parser, arena)
	nodeCount := 0

	status := parser.tryOpportunisticTopLevelResyncRecovery([]byte("xy"), &s, Token{
		Symbol:     2,
		StartByte:  1,
		EndByte:    2,
		StartPoint: Point{Column: 1},
		EndPoint:   Point{Column: 2},
	}, &nodeCount, arena, &entryScratch, &gssScratch, nil)

	if status != resyncAdvance {
		t.Fatalf("tryOpportunisticTopLevelResyncRecovery status = %d, want resyncAdvance", status)
	}
	if s.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if got, want := s.top().state, parser.language.InitialState; got != want {
		t.Fatalf("top state = %d, want initial state %d", got, want)
	}
	if got, want := nodeCount, 1; got != want {
		t.Fatalf("nodeCount = %d, want %d", got, want)
	}
	errNode := stackEntryNode(s.top())
	if errNode == nil || errNode.symbol != errorSymbol || !errNode.hasError() {
		t.Fatalf("top node = %#v, want ERROR with hasError", errNode)
	}
	if got, want := nodeChildCountNoMaterialize(errNode), 2; got != want {
		t.Fatalf("ERROR child count = %d, want %d", got, want)
	}
}

func TestOpportunisticTopLevelResyncRejectsAdvanceOutsideRetry(t *testing.T) {
	parser := NewParser(buildStructuralTopLevelResyncTestLanguage())
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	s, entryScratch, gssScratch := structuralTopLevelResyncTestStack(parser, arena)
	nodeCount := 0

	status := parser.tryOpportunisticTopLevelResyncRecovery([]byte("xy"), &s, Token{
		Symbol:     2,
		StartByte:  1,
		EndByte:    2,
		StartPoint: Point{Column: 1},
		EndPoint:   Point{Column: 2},
	}, &nodeCount, arena, &entryScratch, &gssScratch, nil)

	if status != resyncNone {
		t.Fatalf("tryOpportunisticTopLevelResyncRecovery status = %d, want resyncNone", status)
	}
	if s.dead {
		t.Fatal("stack.dead = true, want false")
	}
	if got, want := s.top().state, StateID(1); got != want {
		t.Fatalf("top state = %d, want unchanged state %d", got, want)
	}
	if got, want := nodeCount, 0; got != want {
		t.Fatalf("nodeCount = %d, want %d", got, want)
	}
}

func TestRetryStructuralTopLevelResyncRejectsZeroWidthAdvance(t *testing.T) {
	parser := NewParser(buildStructuralTopLevelResyncTestLanguage())
	parser.retryStructuralTopLevelResync = true
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	s, entryScratch, gssScratch := structuralTopLevelResyncTestStack(parser, arena)
	nodeCount := 0

	status := parser.tryOpportunisticTopLevelResyncRecovery([]byte("x"), &s, Token{
		Symbol:     2,
		StartByte:  1,
		EndByte:    1,
		StartPoint: Point{Column: 1},
		EndPoint:   Point{Column: 1},
	}, &nodeCount, arena, &entryScratch, &gssScratch, nil)

	if status != resyncNone {
		t.Fatalf("tryOpportunisticTopLevelResyncRecovery status = %d, want resyncNone", status)
	}
	if got, want := nodeCount, 0; got != want {
		t.Fatalf("nodeCount = %d, want %d", got, want)
	}
}

func TestRetryFullParseStructuralTopLevelResyncFlagScope(t *testing.T) {
	source := []byte("abcd")
	cleanRetry := &Tree{
		root: &Node{endByte: uint32(len(source))},
		parseRuntime: &ParseRuntime{
			StopReason:       ParseStopAccepted,
			ExpectedEOFByte:  uint32(len(source)),
			LastTokenEndByte: uint32(len(source)),
			LastTokenWasEOF:  true,
			NodesAllocated:   1,
		},
	}

	t.Run("enabled_for_no_stacks_alive", func(t *testing.T) {
		parser := &Parser{}
		initial := &Tree{
			root: &Node{endByte: 1, flags: nodeFlagHasError},
			parseRuntime: &ParseRuntime{
				StopReason:      ParseStopNoStacksAlive,
				ExpectedEOFByte: uint32(len(source)),
				MaxStacksSeen:   8,
				NodesAllocated:  10,
			},
		}
		calls := 0
		got := parser.retryFullParse(source, 8, initial, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
			calls++
			if !parser.retryStructuralTopLevelResync {
				t.Fatal("retryStructuralTopLevelResync = false during no_stacks_alive retry, want true")
			}
			return cleanRetry
		})
		if got != cleanRetry {
			t.Fatalf("retryFullParse returned %p, want clean retry %p", got, cleanRetry)
		}
		if calls != 1 {
			t.Fatalf("runRetry calls = %d, want 1", calls)
		}
		if parser.retryStructuralTopLevelResync {
			t.Fatal("retryStructuralTopLevelResync leaked after retry")
		}
	})

	t.Run("disabled_for_accepted_error_retry", func(t *testing.T) {
		parser := &Parser{}
		initial := &Tree{
			root: &Node{endByte: 1, flags: nodeFlagHasError},
			parseRuntime: &ParseRuntime{
				StopReason:      ParseStopAccepted,
				ExpectedEOFByte: uint32(len(source)),
				MaxStacksSeen:   8,
				NodesAllocated:  10,
			},
		}
		calls := 0
		got := parser.retryFullParse(source, 8, initial, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
			calls++
			if parser.retryStructuralTopLevelResync {
				t.Fatal("retryStructuralTopLevelResync = true during accepted-error retry, want false")
			}
			return cleanRetry
		})
		if got != cleanRetry {
			t.Fatalf("retryFullParse returned %p, want clean retry %p", got, cleanRetry)
		}
		if calls != 1 {
			t.Fatalf("runRetry calls = %d, want 1", calls)
		}
	})
}
