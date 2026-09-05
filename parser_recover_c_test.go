package gotreesitter

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"
)

func TestCVersionStatusAddsPausedSkippedTreeCost(t *testing.T) {
	parser := &Parser{}

	paused := glrStack{cPaused: true}
	stackCost := parser.cStackErrorCost(&paused)
	if stackCost != cErrCostPerRecovery {
		t.Fatalf("paused stack cost = %d, want %d", stackCost, cErrCostPerRecovery)
	}
	status := parser.cVersionStatus(&paused)
	wantStatusCost := stackCost + cErrCostPerSkippedTree
	if status.cost != wantStatusCost {
		t.Fatalf("paused version status cost = %d, want %d", status.cost, wantStatusCost)
	}
	if !status.isInError {
		t.Fatal("paused version status isInError = false, want true")
	}

	openError := glrStack{cRec: &cRecoverState{}}
	stackCost = parser.cStackErrorCost(&openError)
	if stackCost != cErrCostPerRecovery {
		t.Fatalf("open ERROR_STATE stack cost = %d, want %d", stackCost, cErrCostPerRecovery)
	}
	status = parser.cVersionStatus(&openError)
	if status.cost != stackCost {
		t.Fatalf("open ERROR_STATE version status cost = %d, want stack cost %d", status.cost, stackCost)
	}
	if !status.isInError {
		t.Fatal("open ERROR_STATE version status isInError = false, want true")
	}
}

func TestCStackErrorCostFreshParseProofFallsBackAfterError(t *testing.T) {
	childErrors := false
	parser := &Parser{mergeScratch: &glrMergeScratch{childErrors: &childErrors}}
	errorNode := NewLeafNode(errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	errorNode.setHasError(true)
	stack := glrStack{entries: []stackEntry{newStackEntryNode(2, errorNode)}}

	if got := parser.cStackErrorCost(&stack); got != 0 {
		t.Fatalf("fresh full-parse proof cost = %d, want 0", got)
	}
	childErrors = true
	if got := parser.cStackErrorCost(&stack); got == 0 {
		t.Fatal("error cost remained zero after the proof was invalidated")
	}
}

func TestCCondenseFreshTreeGatePreservesOpenRecoveryCosts(t *testing.T) {
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}}
	open := &Node{symbol: errorSymbol}
	for _, tc := range []struct {
		name  string
		stack glrStack
	}{
		{name: "clean"},
		{name: "paused", stack: glrStack{cPaused: true}},
		{name: "open recovery", stack: glrStack{cRec: &cRecoverState{}}},
		{name: "open recovery segments", stack: glrStack{cRec: &cRecoverState{extraRecoveries: 2}}},
		{name: "materialized recovery", stack: glrStack{cRec: &cRecoverState{openErr: open, extraRecoveries: 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parser.cCondenseVersionStatus(&tc.stack, false)
			want := parser.cVersionStatus(&tc.stack)
			if got != want {
				t.Fatalf("fresh-tree status = %+v, want eager %+v", got, want)
			}
		})
	}

	missing := &Node{symbol: 1}
	missing.setMissing(true)
	stack := glrStack{entries: []stackEntry{newStackEntryNode(1, missing)}}
	got := parser.cCondenseVersionStatus(&stack, true)
	want := parser.cVersionStatus(&stack)
	if got.cost != want.cost || got.dynPrec != want.dynPrec || got.isInError != want.isInError {
		t.Fatalf("error-bearing status = %+v, want eager cost/category %+v", got, want)
	}
}

func TestCCondenseVersionStatusPreservesBaselineClamp(t *testing.T) {
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}}
	stack := glrStack{
		entries:       []stackEntry{newStackEntryNode(1, &Node{symbol: 1})},
		cNodeBaseline: 2,
	}
	status := parser.cCondenseVersionStatus(&stack, false)
	if status.nodeCount != 0 {
		t.Fatalf("node count = %d, want 0 after clamp", status.nodeCount)
	}
	if stack.cNodeBaseline != 1 {
		t.Fatalf("baseline = %d, want clamped 1", stack.cNodeBaseline)
	}
}

func TestCCompareCondenseVersionsEvaluatesOnlyReadNodeCount(t *testing.T) {
	parser := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}}
	leaf := &Node{symbol: 1}
	baselineStack := glrStack{
		entries:       []stackEntry{newStackEntryNode(1, leaf)},
		cNodeBaseline: 2,
	}
	parser.cCondenseVersionStatus(&baselineStack, false)
	if baselineStack.cNodeBaseline != 1 {
		t.Fatalf("condense status did not preserve baseline clamp: baseline = %d, want 1", baselineStack.cNodeBaseline)
	}

	stack := glrStack{
		entries:       []stackEntry{newStackEntryNode(1, leaf)},
		cNodeBaseline: 2,
	}

	clean := cErrorStatus{cost: 1}
	inError := cErrorStatus{cost: 2, isInError: true}
	if got := parser.cCompareCondenseVersions(clean, inError, &stack, &stack); got != cCompareVersions(clean, inError) {
		t.Fatalf("category comparison = %v, want %v", got, cCompareVersions(clean, inError))
	}
	if stack.cNodeBaseline != 2 {
		t.Fatalf("category comparison evaluated node count: baseline = %d, want 2", stack.cNodeBaseline)
	}

	equalA := cErrorStatus{cost: 3, dynPrec: 1}
	equalB := cErrorStatus{cost: 3}
	if got := parser.cCompareCondenseVersions(equalA, equalB, &stack, &stack); got != cCompareVersions(equalA, equalB) {
		t.Fatalf("equal-cost comparison = %v, want %v", got, cCompareVersions(equalA, equalB))
	}
	if stack.cNodeBaseline != 2 {
		t.Fatalf("equal-cost comparison evaluated node count: baseline = %d, want 2", stack.cNodeBaseline)
	}

	cheaper := cErrorStatus{cost: 1}
	dearer := cErrorStatus{cost: 2}
	eagerCheaper := cheaper
	stackForWant := stack
	eagerCheaper.nodeCount = parser.cNodeCountSinceError(&stackForWant)
	if got, want := parser.cCompareCondenseVersions(cheaper, dearer, &stack, &stack), cCompareVersions(eagerCheaper, dearer); got != want {
		t.Fatalf("unequal-cost comparison = %v, want %v", got, want)
	}
	if stack.cNodeBaseline != 1 {
		t.Fatalf("unequal-cost comparison did not evaluate cheaper node count: baseline = %d, want 1", stack.cNodeBaseline)
	}
}

func TestCRecoveryCostCompetitionDisabledInNoTreeModes(t *testing.T) {
	parser := &Parser{errorCostCompetition: true}
	if !parser.errorCostCompetitionEnabled() {
		t.Fatal("errorCostCompetitionEnabled = false, want true for ordinary tree mode")
	}

	parser.noTreeBenchmarkOnly = true
	if parser.errorCostCompetitionEnabled() {
		t.Fatal("errorCostCompetitionEnabled = true, want false for no-tree benchmark mode")
	}

	parser.noTreeBenchmarkOnly = false
	parser.noTreeCheckpointBenchmarkOnly = true
	if parser.errorCostCompetitionEnabled() {
		t.Fatal("errorCostCompetitionEnabled = true, want false for no-tree checkpoint mode")
	}
}

func TestCRecoveryCleanCondenseSingleStackSynchronizesMergeScratch(t *testing.T) {
	parser := &Parser{
		errorCostCompetition:                true,
		crecoveryCostCompetitionRelevant:    true,
		crecoveryCostCompetitionWalkEnabled: true,
		mergeScratch:                        &glrMergeScratch{cRecoveryCostWalk: true, cRecoveryConvergence: true},
	}
	scratch := parserScratch{merge: *parser.mergeScratch}
	parser.mergeScratch = &scratch.merge
	trackChildErrors := false

	parser.clearCRecoveryCostIfClean([]glrStack{{}}, &trackChildErrors)
	if scratch.merge.cRecoveryCostWalk || scratch.merge.cRecoveryConvergence {
		t.Fatalf("clean condense left active merge flags walk=%t convergence=%t", scratch.merge.cRecoveryCostWalk, scratch.merge.cRecoveryConvergence)
	}

	result := parser.prepareParseStacksForIteration(
		[]glrStack{newGLRStack(1)},
		&scratch,
		nil,
		arenaClassFull,
		0,
		0,
		false,
		nil,
		nil,
	)
	if result.stopped {
		t.Fatalf("single-stack preparation stopped with %v", result.stopReason)
	}
	if scratch.merge.cRecoveryCostWalk || scratch.merge.cRecoveryConvergence {
		t.Fatalf("single-stack early return leaked merge flags walk=%t convergence=%t", scratch.merge.cRecoveryCostWalk, scratch.merge.cRecoveryConvergence)
	}

	scratch.merge.perKeyCap = 1
	scratch.merge.recoveryCapOneConvergence = true
	scratch.merge.beginEquivEpoch()
	ordinary := newGLRStack(1)
	ordinary.push(7, NewLeafNode(11, true, 0, 5, Point{}, Point{Column: 5}), nil, nil)
	missingNode := NewLeafNode(11, true, 5, 5, Point{Column: 5}, Point{Column: 5})
	missingNode.setMissing(true)
	missingNode.setHasError(true)
	missing := newGLRStack(1)
	missing.push(7, missingNode, nil, nil)
	missing.cRecoverMissingGroup = &cRecGroup{}
	key := mergeKeyForStack(&missing)
	if index, preserve := cRecoveryCostClassForSlice(&scratch.merge, []glrStack{ordinary}, key, &missing); index != -1 || preserve {
		t.Fatalf("later merge retained a stale cost walk: index=%d preserve=%t", index, preserve)
	}
	merged := mergeStacksWithScratch([]glrStack{ordinary, missing}, &scratch.merge)
	if len(merged) == 0 {
		t.Fatal("later single-stack merge discarded every survivor")
	}
}

func TestCRecoveryCostWalkIsOneCycle(t *testing.T) {
	parser := &Parser{errorCostCompetition: true, mergeScratch: &glrMergeScratch{}}
	parser.markCRecoveryCostCompetitionRelevant()
	if !parser.crecoveryCostCompetitionWalkEnabled {
		t.Fatal("recovery start did not enable the cost walk")
	}
	parser.syncCRecoveryMergeScratch(parser.mergeScratch)
	if !parser.mergeScratch.cRecoveryCostWalk || !parser.mergeScratch.cRecoveryConvergence {
		t.Fatal("recovery start did not publish active recovery state")
	}

	trackChildErrors := false
	parser.pendingForkStacks = []glrStack{newGLRStack(1)}
	parser.clearCRecoveryCostIfClean([]glrStack{{}}, &trackChildErrors)
	if parser.crecoveryCostCompetitionRelevant || parser.crecoveryCostCompetitionWalkEnabled {
		t.Fatal("clean recovery exit retained active state with a pending fork")
	}
	if parser.mergeScratch.cRecoveryCostWalk || parser.mergeScratch.cRecoveryConvergence || parser.mergeScratch.cRecoveryFallbackSuppression {
		t.Fatal("clean recovery exit retained scratch state")
	}
}

func TestCRecoveryParseRetryPoolAndSnippetResetsClearState(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	scratch := &glrMergeScratch{
		cRecoveryCostWalk:            true,
		cRecoveryConvergence:         true,
		cRecoveryFallbackSuppression: true,
	}
	parser.mergeScratch = scratch
	parser.crecoveryCostCompetitionRelevant = true
	parser.crecoveryCostCompetitionWalkEnabled = true
	parser.resetCRecoveryCostCompetitionState()
	if parser.crecoveryCostCompetitionRelevant || parser.crecoveryCostCompetitionWalkEnabled || scratch.cRecoveryCostWalk || scratch.cRecoveryConvergence || scratch.cRecoveryFallbackSuppression {
		t.Fatal("retry reset retained recovery state before parse")
	}

	parser.mergeScratch = nil
	parser.errorCostCompetition = false
	parser.recoveryInitialOnly = true
	parser.crecoveryCostCompetitionRelevant = true
	parser.crecoveryCostCompetitionWalkEnabled = true

	tree, err := parser.Parse([]byte("1+2"))
	if err != nil {
		t.Fatalf("parse returned error: %v", err)
	}
	if tree != nil {
		tree.Release()
	}
	if parser.crecoveryCostCompetitionRelevant || parser.crecoveryCostCompetitionWalkEnabled {
		t.Fatalf("parse start retained recovery state relevant=%t walk=%t", parser.crecoveryCostCompetitionRelevant, parser.crecoveryCostCompetitionWalkEnabled)
	}
	parser.errorCostCompetition = true
	parser.recoveryInitialOnly = false

	parser.mergeScratch = scratch
	parser.crecoveryCostCompetitionRelevant = true
	parser.crecoveryCostCompetitionWalkEnabled = true
	parser.resetCRecoveryCostCompetitionState()
	if parser.crecoveryCostCompetitionRelevant || parser.crecoveryCostCompetitionWalkEnabled || scratch.cRecoveryCostWalk || scratch.cRecoveryConvergence || scratch.cRecoveryFallbackSuppression {
		t.Fatal("retry reset retained recovery state")
	}

	parser.crecoveryCostCompetitionRelevant = true
	parser.crecoveryCostCompetitionWalkEnabled = true
	parser.cRecoverSharedTokenErrorModeLexed = true
	parser.cRecoverCustomResyncActive = true
	parser.cRecoverCustomResyncByte = 42
	parser.cRecoverCustomSourceEligible = true
	scratch.cRecoveryCostWalk = true
	scratch.cRecoveryConvergence = true
	scratch.cRecoveryFallbackSuppression = true
	pool := &ParserPool{language: parser.language}
	pool.applyDefaults(parser)
	if parser.crecoveryCostCompetitionRelevant || parser.crecoveryCostCompetitionWalkEnabled ||
		parser.cRecoverSharedTokenErrorModeLexed || parser.cRecoverCustomResyncActive ||
		parser.cRecoverCustomResyncByte != 0 || parser.cRecoverCustomSourceEligible ||
		scratch.cRecoveryCostWalk || scratch.cRecoveryConvergence || scratch.cRecoveryFallbackSuppression {
		t.Fatal("parser pool reset retained recovery state")
	}

	parser.crecoveryCostCompetitionRelevant = true
	parser.crecoveryCostCompetitionWalkEnabled = true
	scratch.cRecoveryCostWalk = true
	scratch.cRecoveryConvergence = true
	scratch.cRecoveryFallbackSuppression = true
	resetSnippetParser(parser)
	if parser.crecoveryCostCompetitionRelevant || parser.crecoveryCostCompetitionWalkEnabled || scratch.cRecoveryCostWalk || scratch.cRecoveryConvergence || scratch.cRecoveryFallbackSuppression {
		t.Fatal("snippet reset retained recovery state")
	}

	scratch.cRecoveryCostWalk = true
	scratch.cRecoveryConvergence = true
	scratch.cRecoveryFallbackSuppression = true
	scratch.reset()
	if scratch.cRecoveryCostWalk || scratch.cRecoveryConvergence || scratch.cRecoveryFallbackSuppression {
		t.Fatal("merge scratch reset retained recovery state")
	}
}

func TestParseOperationBoundaryResetsRetainedRecoveryMemoSize(t *testing.T) {
	parser := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)}
	outer := parser.beginParseOperationBudget()
	if got := len(parser.cNodeMemoCache); got != cNodeMemoCacheInitialSize {
		t.Fatalf("outer operation memo entries = %d, want %d", got, cNodeMemoCacheInitialSize)
	}

	parser.cNodeMemoCache = parser.cNodeMemoCache[:cNodeMemoCacheSize]
	inner := parser.beginParseOperationBudget()
	if got := len(parser.cNodeMemoCache); got != cNodeMemoCacheSize {
		t.Fatalf("nested retry memo entries = %d, want %d", got, cNodeMemoCacheSize)
	}
	parser.growCNodeMemoCacheTo(cNodeMemoRecoveryCacheSize)
	if got := len(parser.cNodeMemoCache); got != cNodeMemoRecoveryCacheSize {
		t.Fatalf("temporary memo entries = %d, want %d", got, cNodeMemoRecoveryCacheSize)
	}
	parser.endParseOperationBudget(inner)
	if got := len(parser.cNodeMemoCache); got != cNodeMemoRecoveryCacheSize {
		t.Fatalf("nested cleanup memo entries = %d, want %d", got, cNodeMemoRecoveryCacheSize)
	}
	parser.endParseOperationBudget(outer)
	if got := len(parser.cNodeMemoCache); got != cNodeMemoCacheSize {
		t.Fatalf("outer cleanup memo entries = %d, want %d", got, cNodeMemoCacheSize)
	}

	next := parser.beginParseOperationBudget()
	defer parser.endParseOperationBudget(next)
	if got := len(parser.cNodeMemoCache); got != cNodeMemoCacheInitialSize {
		t.Fatalf("next operation memo entries = %d, want %d", got, cNodeMemoCacheInitialSize)
	}
}

func TestCAbsorbErrorRunUsesLexerProvenanceForLeafErrorFlag(t *testing.T) {
	for _, test := range []struct {
		name           string
		lexerProduced  bool
		wantChildError bool
	}{
		{name: "lexer-produced", lexerProduced: true},
		{name: "unproven", wantChildError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			parser := cRecoveryElectionTestParser()
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()

			openErr := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
			cSetNodeSpan(openErr, 10, 10, Point{Column: 10}, Point{Column: 10})
			openErr.setHasError(true)
			stack := newGLRStack(1)
			stack.pushEntry(newStackEntryNode(cErrorState, openErr), nil, nil)
			stack.byteOffset = 10
			stack.cRec = &cRecoverState{group: &cRecGroup{}, openErr: openErr}
			nodeCount := 0

			parser.cAbsorbTokenIntoError(
				&stack,
				Token{
					Symbol:              errorSymbol,
					StartByte:           10,
					EndByte:             18,
					StartPoint:          Point{Column: 10},
					EndPoint:            Point{Column: 18},
					lexerErrorModeLexed: test.lexerProduced,
				},
				&nodeCount,
				arena,
				nil,
				nil,
				nil,
			)

			if got, want := openErr.ChildCount(), 1; got != want {
				t.Fatalf("open error ChildCount = %d, want %d", got, want)
			}
			child := openErr.Child(0)
			if !child.IsError() {
				t.Fatalf("absorbed child type = %q, want ERROR", child.Type(parser.language))
			}
			if !child.IsNamed() {
				t.Fatal("absorbed ERROR leaf is not named")
			}
			if got := child.HasError(); got != test.wantChildError {
				t.Fatalf("absorbed ERROR leaf HasError = %t, want %t", got, test.wantChildError)
			}
		})
	}
}

func TestCRecoverResumeDFALookaheadRequiresLexedAlignedErrorToken(t *testing.T) {
	newFixture := func(errorModeAcceptsToken bool) (*Parser, *dfaTokenSource, Token, glrStack) {
		lexStates := []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 0, Skip: true},
				},
			},
			{},
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 2, Skip: true},
					{Lo: 'b', Hi: 'b', NextState: 3},
				},
			},
			{AcceptToken: 1, Default: -1, EOF: -1},
		}
		if errorModeAcceptsToken {
			lexStates[0].Transitions = append(lexStates[0].Transitions, LexTransition{Lo: 'b', Hi: 'b', NextState: 3})
		}
		lang := &Language{
			Name:        "recovery-relex-fixture",
			TokenCount:  2,
			StateCount:  2,
			SymbolCount: 2,
			SymbolMetadata: []SymbolMetadata{
				{Name: "end", Visible: true, Named: true},
				{Name: "hidden"},
			},
			LexStates: lexStates,
			LexModes:  []LexMode{{LexState: 0}, {LexState: 2}},
		}
		parser := &Parser{language: lang, errorCostCompetition: true}
		source := []byte(" b")
		ts := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, source), lang, parser.lookupActionIndex, nil, nil, nil, true)
		ts.SetParserState(1)
		tok := ts.Next()
		stack := newGLRStack(1)
		return parser, ts, tok, stack
	}

	t.Run("verified-error-run", func(t *testing.T) {
		parser, ts, tok, stack := newFixture(false)
		scratch := &parserScratch{}
		got, ok := parser.cRecoverResumeDFALookahead(ts, ts.lexer.source, &stack, tok, scratch)
		if !ok {
			t.Fatal("aligned error-mode re-lex was rejected")
		}
		if got.Symbol != errorSymbol || got.StartByte != tok.StartByte || got.EndByte != tok.EndByte {
			t.Fatalf("replacement = %+v, want ERROR at %d..%d", got, tok.StartByte, tok.EndByte)
		}
		if !got.lexerErrorModeLexed {
			t.Fatal("replacement lacks the lexer error-mode proof")
		}
		if ts.lexer.pos != int(got.EndByte) || ts.lexer.row != got.EndPoint.Row || ts.lexer.col != got.EndPoint.Column {
			t.Fatalf("source position = %d/%d/%d, want %d/%d/%d", ts.lexer.pos, ts.lexer.row, ts.lexer.col, got.EndByte, got.EndPoint.Row, got.EndPoint.Column)
		}
		if !parser.cRecoverSharedTokenErrorModeLexed {
			t.Fatal("verified error-mode token was not recorded")
		}
		if scratch.relexSnapshotInUse {
			t.Fatal("accepted re-lex retained its snapshot")
		}
	})

	t.Run("stack-offset-mismatch", func(t *testing.T) {
		parser, ts, tok, stack := newFixture(false)
		stack.byteOffset = tok.StartByte
		if got, ok := parser.cRecoverResumeDFALookahead(ts, ts.lexer.source, &stack, tok, &parserScratch{}); ok {
			t.Fatalf("misaligned re-lex = %+v, want rejection", got)
		}
		if parser.cRecoverSharedTokenErrorModeLexed {
			t.Fatal("misaligned token was marked as error-mode lexed")
		}
	})

	t.Run("aligned-invisible-error-mode-token", func(t *testing.T) {
		parser, ts, tok, stack := newFixture(true)
		got, ok := parser.cRecoverResumeDFALookahead(ts, ts.lexer.source, &stack, tok, &parserScratch{})
		if !ok {
			t.Fatal("aligned invisible error-mode token was rejected")
		}
		if got.Symbol != errorSymbol || !got.lexerErrorModeLexed {
			t.Fatalf("replacement = %+v, want lexer-proven ERROR", got)
		}
		if !parser.cRecoverSharedTokenErrorModeLexed {
			t.Fatal("lexer-produced token was not marked as error-mode lexed")
		}
	})

	t.Run("visible-error-mode-token", func(t *testing.T) {
		parser, ts, tok, stack := newFixture(true)
		parser.language.SymbolMetadata[tok.Symbol].Visible = true
		savedState := ts.state
		if got, ok := parser.cRecoverResumeDFALookahead(ts, ts.lexer.source, &stack, tok, &parserScratch{}); ok {
			t.Fatalf("visible error-mode token = %+v, want rejection", got)
		}
		if parser.cRecoverSharedTokenErrorModeLexed {
			t.Fatal("visible token was marked as error-mode lexed")
		}
		if ts.state != savedState || ts.lexer.pos != int(tok.EndByte) || ts.lexer.row != tok.EndPoint.Row || ts.lexer.col != tok.EndPoint.Column {
			t.Fatalf("rejected probe did not preserve source: state=%d pos=%d/%d/%d", ts.state, ts.lexer.pos, ts.lexer.row, ts.lexer.col)
		}
	})
}

func TestCRecoverResumeDFARejectsAbsentExternalScannerCheckpoint(t *testing.T) {
	lexStates := []LexState{
		{
			Default: -1,
			EOF:     -1,
			Transitions: []LexTransition{
				{Lo: ' ', Hi: ' ', NextState: 0, Skip: true},
			},
		},
		{},
		{
			Default: -1,
			EOF:     -1,
			Transitions: []LexTransition{
				{Lo: ' ', Hi: ' ', NextState: 2, Skip: true},
				{Lo: 'b', Hi: 'b', NextState: 3},
			},
		},
		{AcceptToken: 1, Default: -1, EOF: -1},
	}
	scanner := checkpointByteExternalScanner{}
	lang := &Language{
		Name:            "recovery-empty-checkpoint-fixture",
		TokenCount:      2,
		StateCount:      2,
		SymbolCount:     2,
		ExternalScanner: scanner,
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "hidden"},
		},
		LexStates: lexStates,
		LexModes:  []LexMode{{LexState: 0}, {LexState: 2}},
	}
	parser := &Parser{language: lang, errorCostCompetition: true}
	source := []byte(" b")
	ts := newDFATokenSourceDirectWithCRecovery(
		NewLexer(lang.LexStates, source),
		lang,
		parser.lookupActionIndex,
		nil,
		nil,
		nil,
		true,
	)
	defer ts.Close()
	liveState := ts.externalPayload.(*byte)
	*liveState = 0xff
	ts.SetParserState(1)
	tok := ts.Next()
	if !tok.lexerSkippedPrefix || len(ts.externalTokenStart) != 0 {
		t.Fatalf("fixture token lacks the absent checkpoint proof: token=%+v checkpoint=%v", tok, ts.externalTokenStart)
	}
	if ts.CanRelexFromTokenStart(tok) || ts.canRelexFromSkippedPrefix(tok) {
		t.Fatal("absent checkpoint admitted direct token reuse")
	}
	stack := newGLRStack(1)
	savedParserState := ts.state
	if got, ok := parser.cRecoverResumeDFALookahead(ts, source, &stack, tok, &parserScratch{}); ok {
		t.Fatalf("absent-checkpoint replay = %+v, want rejection", got)
	}
	if *liveState != 0xff {
		t.Fatalf("rejected replay changed live scanner state to %#x, want 0xff", *liveState)
	}
	if ts.state != savedParserState || ts.lexer.pos != int(tok.EndByte) ||
		ts.lexer.row != tok.EndPoint.Row || ts.lexer.col != tok.EndPoint.Column {
		t.Fatalf("rejected replay changed source state: parser=%d pos=%d/%d/%d", ts.state, ts.lexer.pos, ts.lexer.row, ts.lexer.col)
	}
	if parser.cRecoverSharedTokenErrorModeLexed {
		t.Fatal("absent-checkpoint token was marked as error-mode lexed")
	}
}

func TestCRecoverResumeDFARejectsCheckpointlessExternalScanner(t *testing.T) {
	lexStates := []LexState{
		{
			Default: -1,
			EOF:     -1,
			Transitions: []LexTransition{
				{Lo: ' ', Hi: ' ', NextState: 0, Skip: true},
			},
		},
		{},
		{
			Default: -1,
			EOF:     -1,
			Transitions: []LexTransition{
				{Lo: ' ', Hi: ' ', NextState: 2, Skip: true},
				{Lo: 'b', Hi: 'b', NextState: 3},
			},
		},
		{AcceptToken: 1, Default: -1, EOF: -1},
	}
	lang := &Language{
		Name:            "recovery-checkpointless-scanner-fixture",
		TokenCount:      2,
		StateCount:      2,
		SymbolCount:     2,
		ExternalScanner: skipPrefixExternalScanner{},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "hidden"},
		},
		LexStates: lexStates,
		LexModes:  []LexMode{{LexState: 0}, {LexState: 2}},
	}
	parser := &Parser{language: lang, errorCostCompetition: true}
	source := []byte(" b")
	ts := newDFATokenSourceDirectWithCRecovery(
		NewLexer(lang.LexStates, source),
		lang,
		parser.lookupActionIndex,
		nil,
		nil,
		nil,
		true,
	)
	defer ts.Close()
	ts.SetParserState(1)
	tok := ts.Next()
	if !tok.lexerSkippedPrefix {
		t.Fatalf("fixture token lacks the skipped-prefix proof: token=%+v", tok)
	}
	if !ts.CanRelexFromTokenStart(tok) {
		t.Fatal("generic relex rejected a stateless external scanner")
	}
	if ts.canRelexFromSkippedPrefix(tok) {
		t.Fatal("checkpointless external scanner admitted recovery replay")
	}
	stack := newGLRStack(1)
	savedParserState := ts.state
	if got, ok := parser.cRecoverResumeDFALookahead(ts, source, &stack, tok, &parserScratch{}); ok {
		t.Fatalf("checkpointless scanner replay = %+v, want rejection", got)
	}
	if ts.state != savedParserState || ts.lexer.pos != int(tok.EndByte) ||
		ts.lexer.row != tok.EndPoint.Row || ts.lexer.col != tok.EndPoint.Column {
		t.Fatalf("rejected replay changed source state: parser=%d pos=%d/%d/%d", ts.state, ts.lexer.pos, ts.lexer.row, ts.lexer.col)
	}
	if parser.cRecoverSharedTokenErrorModeLexed {
		t.Fatal("checkpointless scanner token was marked as error-mode lexed")
	}
}

func TestCRecoverDispatchInErrorAdvancesZeroWidthSkippedToken(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	openErr := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
	cSetNodeSpan(openErr, 10, 10, Point{Column: 10}, Point{Column: 10})
	openErr.setHasError(true)
	openErr.parseState = cErrorState
	stack := newGLRStack(1)
	stack.pushEntry(newStackEntryNode(cErrorState, openErr), nil, nil)
	stack.byteOffset = 10
	stack.cRec = &cRecoverState{group: group, openErr: openErr}
	stacks := []glrStack{stack}
	nodeCount := 0

	outcome, forked, reason := parser.cRecoverDispatchInError(
		&stacks,
		0,
		[]byte("012345678901234567890"),
		Token{Symbol: 2, StartByte: 18, EndByte: 18, StartPoint: Point{Column: 18}, EndPoint: Point{Column: 18}},
		&nodeCount,
		arena,
		nil,
		nil,
		nil,
	)

	if reason != ParseStopNone {
		t.Fatalf("cRecoverDispatchInError stop reason = %v, want none", reason)
	}
	if outcome != cRecConsumed || forked {
		t.Fatalf("outcome/forked = %v/%t, want %v/false", outcome, forked, cRecConsumed)
	}
	if !stacks[0].shifted {
		t.Fatal("zero-width skipped token did not mark stack shifted")
	}
	if got, want := stacks[0].byteOffset, uint32(18); got != want {
		t.Fatalf("stack byteOffset = %d, want %d", got, want)
	}
	if got, want := openErr.EndByte(), uint32(18); got != want {
		t.Fatalf("open error end = %d, want %d", got, want)
	}
	if nodeCount == 0 {
		t.Fatal("zero-width skipped token did not update recovery node count")
	}
}

func TestCRecoverDispatchInErrorAbsorbsZeroWidthSkippedTokenAtCurrentOffset(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	openErr := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
	cSetNodeSpan(openErr, 18, 18, Point{Column: 18}, Point{Column: 18})
	openErr.setHasError(true)
	openErr.parseState = cErrorState
	stack := newGLRStack(1)
	stack.pushEntry(newStackEntryNode(cErrorState, openErr), nil, nil)
	stack.byteOffset = 18
	stack.cRec = &cRecoverState{group: group, openErr: openErr}
	stacks := []glrStack{stack}
	nodeCount := 0

	outcome, forked, reason := parser.cRecoverDispatchInError(
		&stacks,
		0,
		[]byte("012345678901234567890"),
		Token{Symbol: 2, StartByte: 18, EndByte: 18, StartPoint: Point{Column: 18}, EndPoint: Point{Column: 18}},
		&nodeCount,
		arena,
		nil,
		nil,
		nil,
	)

	if reason != ParseStopNone {
		t.Fatalf("cRecoverDispatchInError stop reason = %v, want none", reason)
	}
	if outcome != cRecConsumed || forked {
		t.Fatalf("outcome/forked = %v/%t, want %v/false", outcome, forked, cRecConsumed)
	}
	if !stacks[0].shifted {
		t.Fatal("zero-width skipped token did not mark stack shifted")
	}
	if got, want := stacks[0].byteOffset, uint32(18); got != want {
		t.Fatalf("stack byteOffset = %d, want %d", got, want)
	}
	if got, want := openErr.ChildCount(), 1; got != want {
		t.Fatalf("open error ChildCount = %d, want %d", got, want)
	}
	if got, want := openErr.Child(0).StartByte(), uint32(18); got != want {
		t.Fatalf("zero-width child StartByte = %d, want %d", got, want)
	}
	if nodeCount == 0 {
		t.Fatal("zero-width skipped token did not update recovery node count")
	}
}

func TestCRecoverSkipTailBetterVersionUsesErrorStatus(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	absorbing := newGLRStack(1)
	absorbing.pushEntry(stackEntry{state: cErrorState}, nil, nil)
	absorbing.byteOffset = 10
	absorbing.cRec = &cRecoverState{group: group}

	cleanFork := newGLRStack(2)
	errNode := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
	cleanFork.pushEntry(newStackEntryNode(3, errNode), nil, nil)
	cleanFork.byteOffset = absorbing.byteOffset

	stacks := []glrStack{absorbing, cleanFork}
	skipCost := parser.cStackErrorCost(&stacks[0]) + cErrCostPerSkippedTree
	if got := parser.cStackErrorCost(&stacks[1]); got >= skipCost {
		t.Fatalf("clean fork cost = %d, want less than skip candidate cost %d", got, skipCost)
	}

	if parser.cBetterVersionExists(stacks, 0, false, skipCost) {
		t.Fatal("clean fork killed non-error skip candidate; want it to only prefer a non-mergeable clean candidate")
	}
	if !parser.cBetterVersionExists(stacks, 0, true, skipCost) {
		t.Fatal("clean fork did not kill in-error skip candidate")
	}
}

func TestCApplyMergedErrorGroupBaselineUsesMaxHeadNodeCount(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	short := cRecoveryBaselineStack(arena, 2, 1)
	long := cRecoveryBaselineStack(arena, 2, 3)
	versions := []glrStack{short, long}
	for vi := range versions {
		versions[vi].pushEntry(stackEntry{state: cErrorState}, nil, nil)
	}

	baseline := parser.cApplyMergedErrorGroupBaseline(versions)
	if baseline != 3 {
		t.Fatalf("merged baseline = %d, want max cumulative node count 3", baseline)
	}
	for i := range versions {
		if got := versions[i].cNodeBaseline; got != uint32(baseline) {
			t.Fatalf("version %d baseline = %d, want shared %d", i, got, baseline)
		}
	}
	if got := parser.cNodeCountSinceError(&versions[0]); got != 0 {
		t.Fatalf("short version node count since error = %d, want clamp to 0", got)
	}
	if got := versions[0].cNodeBaseline; got != 1 {
		t.Fatalf("short version clamped baseline = %d, want current cumulative count 1", got)
	}
	if got := parser.cNodeCountSinceError(&versions[1]); got != 0 {
		t.Fatalf("long version node count since error = %d, want 0", got)
	}
	if got := versions[1].cNodeBaseline; got != uint32(baseline) {
		t.Fatalf("long version baseline after node count = %d, want shared %d", got, baseline)
	}
}

func TestCRecordSummaryUsesCompactEntryPosition(t *testing.T) {
	parser := &Parser{}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newCompactFullLeafInArena(arena, 1, true, 13, 21, Point{Row: 2, Column: 3}, Point{Row: 2, Column: 11})
	entry := newStackEntryCompactFullLeaf(3, leaf)
	summary := parser.cRecordSummary([]stackEntry{entry, {state: 2}})
	if len(summary) == 0 {
		t.Fatal("summary is empty")
	}
	if got := summary[0].posBytes; got != 21 {
		t.Fatalf("compact summary posBytes = %d, want 21", got)
	}
	if got := summary[0].posRow; got != 2 {
		t.Fatalf("compact summary posRow = %d, want 2", got)
	}
}

func TestCStackEntriesTopFirstReusesScratch(t *testing.T) {
	const depth = 128
	entries := make([]stackEntry, depth)
	for i := range entries {
		entries[i] = stackEntry{state: StateID(i + 1)}
	}
	stack := glrStack{entries: entries}
	var scratch gssScratch

	got := cStackEntriesTopFirst(&stack, &scratch)
	if len(got) != depth {
		t.Fatalf("entry count = %d, want %d", len(got), depth)
	}
	if got[0].state != entries[depth-1].state || got[depth-1].state != entries[0].state {
		t.Fatal("entries are not top-first")
	}

	if allocs := testing.AllocsPerRun(100, func() {
		got = cStackEntriesTopFirst(&stack, &scratch)
	}); allocs != 0 {
		t.Fatalf("allocs per reused walk = %g, want 0", allocs)
	}
	if len(got) != depth {
		t.Fatalf("reused entry count = %d, want %d", len(got), depth)
	}

	ownedFirst := cStackEntriesTopFirst(&stack, nil)
	ownedSecond := cStackEntriesTopFirst(&stack, nil)
	ownedSecond[0].state++
	if ownedFirst[0].state == ownedSecond[0].state {
		t.Fatal("nil-scratch calls reused borrowed storage")
	}
	if ownedFirst[0].state != entries[depth-1].state {
		t.Fatal("later nil-scratch call overwrote the earlier result")
	}
}

func TestCRecordSummaryMatchesPositionTableReference(t *testing.T) {
	parser := &Parser{}
	const maxEntries = 10
	nodes := make([]Node, maxEntries)
	for i := range nodes {
		nodes[i].endByte = uint32(100 + i)
		nodes[i].endPoint = Point{Row: uint32(10 + i), Column: uint32(i)}
	}

	for count := 0; count <= maxEntries; count++ {
		for mask := 0; mask < 1<<count; mask++ {
			entries := make([]stackEntry, count)
			for i := range entries {
				state := StateID(i%7 + 1)
				if mask&(1<<i) != 0 {
					entries[i] = newStackEntryNode(state, &nodes[i])
				} else {
					if i%3 == 0 {
						state = cErrorState
					}
					entries[i] = stackEntry{state: state}
				}
			}

			got := parser.cRecordSummary(entries)
			want := cRecordSummaryPositionTableReference(entries)
			if len(got) != len(want) {
				t.Fatalf("count=%d mask=%#x summary len = %d, want %d", count, mask, len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("count=%d mask=%#x summary[%d] = %+v, want %+v", count, mask, i, got[i], want[i])
				}
			}
		}
	}

	entries := make([]stackEntry, cRecoverMaxSummaryDepth+4)
	boundaryNodes := make([]Node, len(entries))
	for pattern := 0; pattern < 4; pattern++ {
		for i := range entries {
			state := StateID((i+pattern)%7 + 1)
			if (i+pattern)%3 != 0 {
				boundaryNodes[i].endByte = uint32(200 + i)
				boundaryNodes[i].endPoint = Point{Row: uint32(20 + i)}
				entries[i] = newStackEntryNode(state, &boundaryNodes[i])
			} else {
				if i%2 == 0 {
					state = cErrorState
				}
				entries[i] = stackEntry{state: state}
			}
		}
		got := parser.cRecordSummary(entries)
		want := cRecordSummaryPositionTableReference(entries)
		if len(got) != len(want) {
			t.Fatalf("boundary pattern=%d summary len = %d, want %d", pattern, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("boundary pattern=%d summary[%d] = %+v, want %+v", pattern, i, got[i], want[i])
			}
		}
	}
}

func TestCRecordSummaryAvoidsPositionAllocations(t *testing.T) {
	parser := &Parser{}
	entries := make([]stackEntry, 8)
	nodes := make([]Node, len(entries))
	for i := range entries {
		nodes[i].endByte = uint32(i + 1)
		nodes[i].endPoint = Point{Row: uint32(i)}
		entries[i] = newStackEntryNode(StateID(i+1), &nodes[i])
	}

	var summary []cStackSummaryEntry
	if allocs := testing.AllocsPerRun(100, func() {
		summary = parser.cRecordSummary(entries)
	}); allocs != 1 {
		t.Fatalf("allocs per summary = %g, want 1 persistent result allocation", allocs)
	}
	if len(summary) != len(entries) {
		t.Fatalf("summary len = %d, want %d", len(summary), len(entries))
	}
}

func TestCRecordSummaryPreservesMoreThanReserveAtEqualDepth(t *testing.T) {
	parser := &Parser{}
	const entryCount = cRecoverMaxSummaryDepth + 8
	entries := make([]stackEntry, entryCount)
	nodes := make([]Node, entryCount)
	for i := range entries {
		nodes[i].endByte = uint32(100 + i)
		nodes[i].endPoint = Point{Row: uint32(10 + i), Column: uint32(i)}
		nodes[i].setExtra(true)
		entries[i] = newStackEntryNode(StateID(i+1), &nodes[i])
	}

	got := parser.cRecordSummary(entries)
	want := cRecordSummaryPositionTableReference(entries)
	if len(got) != entryCount || len(got) <= cRecoverMaxSummaryDepth+1 {
		t.Fatalf("summary len = %d, want %d entries beyond the reserve hint", len(got), entryCount)
	}
	if len(got) != len(want) {
		t.Fatalf("summary len = %d, reference len = %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("summary[%d] = %+v, want %+v", i, got[i], want[i])
		}
		if got[i].depth != 0 {
			t.Fatalf("summary[%d].depth = %d, want equal-depth entry", i, got[i].depth)
		}
	}
}

func cRecordSummaryPositionTableReference(entries []stackEntry) []cStackSummaryEntry {
	summary := make([]cStackSummaryEntry, 0, 8)
	posBytesAt := make([]uint32, len(entries))
	posRowAt := make([]uint32, len(entries))
	var posBytes uint32
	var posRow uint32
	for i := len(entries) - 1; i >= 0; i-- {
		if stackEntryHasNode(entries[i]) {
			posBytes = stackEntryNodeEndByte(entries[i])
			posRow = stackEntryNodeEndPoint(entries[i]).Row
		}
		posBytesAt[i] = posBytes
		posRowAt[i] = posRow
	}
	depth := 0
	for i := range entries {
		duplicate := false
		for j := len(summary) - 1; j >= 0; j-- {
			if int(summary[j].depth) < depth {
				break
			}
			if int(summary[j].depth) == depth && summary[j].state == entries[i].state {
				duplicate = true
				break
			}
		}
		if !duplicate {
			summary = append(summary, cStackSummaryEntry{
				depth:    uint8(depth),
				state:    entries[i].state,
				posBytes: posBytesAt[i],
				posRow:   posRowAt[i],
			})
		}
		if cEntryCountsTowardDepth(entries[i]) {
			depth++
			if depth > cRecoverMaxSummaryDepth {
				break
			}
		}
	}
	return summary
}

func TestCStackPosRowUsesCompactEntryPoint(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newCompactFullLeafInArena(arena, 1, true, 13, 21, Point{Row: 4, Column: 3}, Point{Row: 4, Column: 11})
	entry := newStackEntryCompactFullLeaf(3, leaf)

	sliceStack := glrStack{entries: []stackEntry{{state: 1}, entry}}
	if got := cStackPosRow(&sliceStack); got != 4 {
		t.Fatalf("slice-backed compact stack row = %d, want 4", got)
	}

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	head := scratch.allocNode(entry, base, 2)
	gssStack := glrStack{gss: gssStack{head: head}}
	if got := cStackPosRow(&gssStack); got != 4 {
		t.Fatalf("GSS-backed compact stack row = %d, want 4", got)
	}
}

// TestCAppendVisibleSpliceRecoveryPreservesExtraErrorCarrier pins C parity
// for ordinary (non-recovery) reduce/splice paths around an extra ERROR
// carrier: cAppendVisibleSplice never dissolves an ERROR node (only invisible
// hidden-symbol subtrees flatten), and an ordinary reduce keeps a popped extra
// ERROR carrier as a direct child — the C oracle's `program` for php's
// `static function a() {}` carries `(ERROR ...)` as a direct child, and an
// earlier (reverted) mechanism that dissolved it here dropped both the
// HasError bit and the error cost. The wave-1 investigation additionally
// found that the production-signature rewrapping this test used to pin
// (cAppendRecoveryVisibleSplice and its cRecoveryVisibleSplice* helpers) is
// itself NOT what C does — C's ts_parser__reduce/recover_to_state never
// reconstruct a synthesized production node over an ERROR carrier's
// children — so that cluster was deleted as production-dead, C-incorrect
// code rather than kept "for coverage".
func TestCAppendVisibleSpliceRecoveryPreservesExtraErrorCarrier(t *testing.T) {
	lang := &Language{
		TokenCount:  1,
		StateCount:  3,
		SymbolCount: 5,
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "brace", Visible: true, Named: true},
			{Name: "match_arm_a", Visible: true, Named: true},
			{Name: "match_arm_b", Visible: true, Named: true},
			{Name: "match_block", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, errorCostCompetition: true}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 1, true, 3, 4, Point{Column: 3}, Point{Column: 4})
	armA := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	armB := newLeafNodeInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	armB.setHasError(true)
	carrier := newParentNodeInArena(arena, errorSymbol, true, []*Node{armA, armB}, nil, 0)
	carrier.setExtra(true)
	carrier.setHasError(true)

	preserved := parser.cAppendVisibleSplice(nil, carrier)
	if len(preserved) != 1 || preserved[0] != carrier {
		t.Fatalf("ordinary visible splice = %#v, want ERROR carrier preserved", preserved)
	}

	entries := []stackEntry{
		newStackEntryNode(1, left),
		newStackEntryNode(1, carrier),
		newStackEntryNode(1, right),
	}
	children, _, _ := parser.buildReduceChildren(entries, 0, len(entries), 2, 4, 0, arena)
	if len(children) != 3 {
		t.Fatalf("reduced child count = %d, want 3", len(children))
	}
	if children[0] != left || children[1] != carrier || children[2] != right {
		t.Fatalf("reduced children = %#v, want left/carrier/right", children)
	}
	if !children[1].hasError() {
		t.Fatal("reduced ERROR carrier hasError was cleared")
	}
}

// TestCRecoverStrategy1ElectionDedupesDuplicateEntryAcrossMembers pins the C
// semantics of the merged-version summary: ts_stack_record_summary dedupes
// (depth, state) pairs across the merged paths at RECORD time, and
// ts_parser__recover reads position/cost from the ONE merged version (m0 in
// this port), not each member's own byteOffset.
//
// Caveat (both guards fire independently here): this fixture's duplicate
// entry's posBytes(5) equals m0's own position(5), so the m0-position-equality
// guard (`entry.posBytes == pos`) ALSO produces no-fork on its own, regardless
// of the seen-key dedup — stack1's own byteOffset(10) is irrelevant because
// position is read from m0, not the member being scanned. This test therefore
// pins the m0-position basis; it does not, by itself, isolate the
// record-time dedup from that guard (both independently veto the fork here).
func TestCRecoverStrategy1ElectionDedupesDuplicateEntryAcrossMembers(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	entry := cStackSummaryEntry{depth: 1, state: 2, posBytes: 5}
	stacks := []glrStack{
		cRecoveryElectionStack(arena, 4, 3, 5, group, 0, []cStackSummaryEntry{entry}),
		cRecoveryElectionStack(arena, 2, 3, 10, group, 1, []cStackSummaryEntry{entry}),
	}
	nodeCount := 0
	didRecover, forked, reason := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, nil, nil)
	if reason != ParseStopNone {
		t.Fatalf("cRecoverStrategy1Election stop reason = %v, want none", reason)
	}
	if didRecover || forked {
		t.Fatalf("strategy 1 election = didRecover %v forked %v, want false/false (entry position equals the merged version position; duplicate entries dedupe at first encounter)", didRecover, forked)
	}
	if len(stacks) != 2 {
		t.Fatalf("stack count = %d, want no fork appended", len(stacks))
	}
}

// TestCRecoverStrategy1ElectionSkipsCostForWouldMergeNoActionEntry pins the
// guard order for a no-action entry whose state/position already exists. The
// would-merge guard skips that entry before cost, allowing the later candidate
// to recover. This is not the cost-before-action case; that has a dedicated
// non-wouldMerge fixture below.
func TestCRecoverStrategy1ElectionSkipsCostForWouldMergeNoActionEntry(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	stacks := []glrStack{
		cRecoveryElectionStack(arena, 2, 3, 2500, group, 0, []cStackSummaryEntry{
			{depth: 0, state: 4, posBytes: 0},
			{depth: 1, state: 2, posBytes: 2490},
		}),
		cRecoveryElectionPlainStack(arena, 4, 4, 2500),
	}
	nodeCount := 0
	didRecover, forked, reason := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, nil, nil)
	if reason != ParseStopNone {
		t.Fatalf("cRecoverStrategy1Election stop reason = %v, want none", reason)
	}
	if !didRecover || !forked {
		t.Fatalf("strategy 1 election = didRecover %v forked %v, want true/true", didRecover, forked)
	}
	if len(stacks) != 3 {
		t.Fatalf("stack count = %d, want recovered fork appended", len(stacks))
	}
	if got := stacks[2].top().state; got != StateID(2) {
		t.Fatalf("recovered fork top state = %d, want 2", got)
	}
}

// TestCRecoverStrategy1ElectionUsesMergedVersionCostBasis pins C's cost basis
// for the strategy-1 scan: parser.c ts_parser__recover computes position and
// current_error_cost ONCE for the single merged error-state version (this
// engine's first group member). A candidate entry whose hypothetical recovery
// is clearly worse than an existing version aborts the whole scan (C `break`),
// even when a later member's engine-local byte offset would have produced a
// cheaper-looking cost.
func TestCRecoverStrategy1ElectionUsesMergedVersionCostBasis(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	entry := cStackSummaryEntry{depth: 1, state: 2, posBytes: 0}
	stacks := []glrStack{
		cRecoveryElectionStack(arena, 4, 3, 2500, group, 0, nil),
		cRecoveryElectionStack(arena, 2, 3, 10, group, 1, []cStackSummaryEntry{entry}),
		cRecoveryElectionPlainStack(arena, 4, 4, 2500),
	}
	nodeCount := 0
	didRecover, forked, reason := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, nil, nil)
	if reason != ParseStopNone {
		t.Fatalf("cRecoverStrategy1Election stop reason = %v, want none", reason)
	}
	if didRecover || forked {
		t.Fatalf("strategy 1 election = didRecover %v forked %v, want false/false (merged-version cost makes the entry clearly worse than the clean sibling, aborting the scan)", didRecover, forked)
	}
	if len(stacks) != 3 {
		t.Fatalf("stack count = %d, want no fork appended", len(stacks))
	}
}

func TestCRecoverElectionDepthIteratorPreservesMergedSummaryOrder(t *testing.T) {
	group := &cRecGroup{}
	stacks := []glrStack{
		{cRec: &cRecoverState{group: group, groupOrder: 2, summary: []cStackSummaryEntry{
			{depth: 0, state: 8},
			{depth: 1, state: 6},
		}}},
		{cRec: &cRecoverState{group: group, groupOrder: 0, summary: []cStackSummaryEntry{
			{depth: 0, state: 2},
			{depth: 1, state: 5},
			{depth: 1, state: 7},
		}}},
		{cRec: &cRecoverState{group: group, groupOrder: 1, summary: []cStackSummaryEntry{
			{depth: 0, state: 3},
			{depth: 1, state: cErrorState},
			{depth: 1, state: 5}, // duplicate: member 0 owns first encounter
			{depth: 1, state: 9},
		}}},
	}

	var scratch cRecoverElectionScratch
	scratch.prepare(len(stacks), 10)
	scratch.members = append(scratch.members, 0, 1, 2)
	scratch.prepareMemberCursors(len(scratch.members))
	cSortRecoverMembersByGroupOrder(stacks, scratch.members)

	type tuple struct {
		depth int
		state StateID
		owner uint32
	}
	var got []tuple
	for depth := 0; depth <= 2; depth++ {
		iter := scratch.beginDepth(depth)
		for {
			mi, entry, status := iter.next(stacks)
			if status == cRecoverElectionIterExhausted {
				// Exhaustion is stable and does not rewind a member cursor.
				if _, _, again := iter.next(stacks); again != cRecoverElectionIterExhausted {
					t.Fatalf("depth %d iterator produced an entry after exhaustion", depth)
				}
				break
			}
			if status == cRecoverElectionIterPoll {
				continue
			}
			got = append(got, tuple{depth: int(entry.depth), state: entry.state, owner: stacks[mi].cRec.groupOrder})
		}
	}
	want := []tuple{
		{depth: 0, state: 2, owner: 0},
		{depth: 0, state: 3, owner: 1},
		{depth: 0, state: 8, owner: 2},
		{depth: 1, state: 5, owner: 0},
		{depth: 1, state: 7, owner: 0},
		{depth: 1, state: 9, owner: 1},
		{depth: 1, state: 6, owner: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("election trace len = %d, want %d: got %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("election trace[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestCRecoverElectionScratchEpochWrapAndReleaseReset(t *testing.T) {
	group := &cRecGroup{}
	stacks := []glrStack{{cRec: &cRecoverState{group: group, summary: []cStackSummaryEntry{{depth: 0, state: 2}}}}}
	var scratch cRecoverElectionScratch
	scratch.prepare(1, 4)
	scratch.members = append(scratch.members, 0)
	scratch.prepareMemberCursors(len(scratch.members))
	scratch.epoch = ^uint32(0)
	for i := range scratch.stateStamp {
		scratch.stateStamp[i] = 1
	}

	iter := scratch.beginDepth(0)
	if scratch.epoch != 1 {
		t.Fatalf("epoch after wrap = %d, want 1", scratch.epoch)
	}
	mi, entry, status := iter.next(stacks)
	if status != cRecoverElectionIterCandidate || mi != 0 || entry.state != 2 {
		t.Fatalf("entry after epoch wrap = member %d entry %+v status %v, want member 0 state 2", mi, entry, status)
	}

	// Ordinary release retains pointer-free capacity and the epoch while
	// dropping active lengths. The next beginDepth makes old stamps stale.
	scratch.resetForRelease()
	if len(scratch.members) != 0 || len(scratch.cursors) != 0 || len(scratch.stateStamp) != 4 || scratch.epoch != 1 {
		t.Fatalf("retained reset = members %d cursors %d stamps %d epoch %d", len(scratch.members), len(scratch.cursors), len(scratch.stateStamp), scratch.epoch)
	}

	// Oversized scratch is not retained by the parser pool.
	scratch.prepare(maxStacksPerMergeKeyCeiling+1, 64*1024+1)
	scratch.prepareMemberCursors(maxStacksPerMergeKeyCeiling + 1)
	scratch.epoch = 7
	scratch.resetForRelease()
	if scratch.members != nil || scratch.cursors != nil || scratch.stateStamp != nil || scratch.epoch != 0 {
		t.Fatalf("oversized reset retained scratch: members=%v cursors=%v stamps=%v epoch=%d",
			scratch.members != nil, scratch.cursors != nil, scratch.stateStamp != nil, scratch.epoch)
	}
}

func TestCRecoverElectionDepthIteratorPollsAcrossDuplicateRuns(t *testing.T) {
	group := &cRecGroup{}
	duplicates := make([]cStackSummaryEntry, 64)
	for i := range duplicates {
		duplicates[i] = cStackSummaryEntry{depth: 0, state: 2}
	}
	stacks := []glrStack{
		{cRec: &cRecoverState{group: group, groupOrder: 0, summary: []cStackSummaryEntry{{depth: 0, state: 2}}}},
		{cRec: &cRecoverState{group: group, groupOrder: 1, summary: duplicates}},
	}
	var scratch cRecoverElectionScratch
	scratch.prepare(len(stacks), 3)
	scratch.members = append(scratch.members, 0, 1)
	scratch.prepareMemberCursors(len(scratch.members))

	iter := scratch.beginDepth(0)
	if _, _, status := iter.next(stacks); status != cRecoverElectionIterCandidate {
		t.Fatalf("first iterator status = %v, want candidate", status)
	}
	if _, _, status := iter.next(stacks); status != cRecoverElectionIterPoll {
		t.Fatalf("duplicate-run iterator status = %v, want poll", status)
	}
	if _, _, status := iter.next(stacks); status != cRecoverElectionIterExhausted {
		t.Fatalf("post-poll iterator status = %v, want exhausted", status)
	}
}

func TestCRecoverElectionDepthIteratorReusesPreparedScratch(t *testing.T) {
	group := &cRecGroup{}
	stacks := []glrStack{{cRec: &cRecoverState{group: group, summary: []cStackSummaryEntry{
		{depth: 0, state: 2},
		{depth: 1, state: 3},
	}}}}
	var scratch cRecoverElectionScratch
	scratch.prepare(1, 4)
	allocs := testing.AllocsPerRun(100, func() {
		scratch.prepare(1, 4)
		scratch.members = append(scratch.members, 0)
		scratch.prepareMemberCursors(len(scratch.members))
		for depth := 0; depth <= 1; depth++ {
			iter := scratch.beginDepth(depth)
			for {
				if _, _, status := iter.next(stacks); status == cRecoverElectionIterExhausted {
					break
				}
			}
		}
	})
	if allocs != 0 {
		t.Fatalf("allocations per prepared election traversal = %g, want 0", allocs)
	}
}

func TestCRecoverStrategy1ElectionReusesParserScratchWithoutAllocating(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	entry := cStackSummaryEntry{depth: 1, state: 2, posBytes: 5}
	stacks := []glrStack{
		cRecoveryElectionStack(arena, 4, 3, 5, group, 0, []cStackSummaryEntry{entry}),
		cRecoveryElectionStack(arena, 2, 3, 10, group, 1, []cStackSummaryEntry{entry}),
	}
	var gssScratch gssScratch
	nodeCount := 0
	if didRecover, forked, reason := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, &gssScratch, nil); didRecover || forked || reason != ParseStopNone {
		t.Fatalf("warm election = didRecover %v forked %v reason %v, want false false none", didRecover, forked, reason)
	}

	allocs := testing.AllocsPerRun(100, func() {
		didRecover, forked, reason := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, &gssScratch, nil)
		if didRecover || forked || reason != ParseStopNone {
			panic("unexpected election result")
		}
	})
	if allocs != 0 {
		t.Fatalf("allocations per warmed strategy-1 election = %g, want 0", allocs)
	}
}

func cRecoveryElectionTestParser() *Parser {
	lang := &Language{
		TokenCount:  2,
		StateCount:  5,
		SymbolCount: 3,
		ParseTable: [][]uint16{
			nil,
			nil,
			{0, 1},
			nil,
			nil,
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "tok", Visible: true, Named: true},
			{Name: "node", Visible: true, Named: true},
		},
	}
	return &Parser{language: lang, denseLimit: len(lang.ParseTable)}
}

func cRecoveryElectionStack(arena *nodeArena, base, top StateID, endByte uint32, group *cRecGroup, groupOrder int, summary []cStackSummaryEntry) glrStack {
	s := cRecoveryElectionPlainStack(arena, base, top, endByte)
	s.cRec = &cRecoverState{
		group:      group,
		groupOrder: uint32(groupOrder),
		summary:    summary,
	}
	s.cNodeBaseline = 1
	return s
}

func cRecoveryElectionPlainStack(arena *nodeArena, base, top StateID, endByte uint32) glrStack {
	s := newGLRStack(base)
	n := newLeafNodeInArena(arena, 1, true, 0, endByte, Point{}, Point{Column: endByte})
	s.pushEntry(newStackEntryNode(top, n), nil, nil)
	return s
}

func cRecoveryBaselineStack(arena *nodeArena, start StateID, visibleNodes int) glrStack {
	s := newGLRStack(start)
	for i := 0; i < visibleNodes; i++ {
		startByte := uint32(i)
		endByte := startByte + 1
		n := newLeafNodeInArena(arena, 1, true, startByte, endByte, Point{Column: startByte}, Point{Column: endByte})
		s.pushEntry(newStackEntryNode(start+StateID(i)+1, n), nil, nil)
	}
	return s
}

func activateCompactPackedGSSVersionOrderForTest(parser *Parser, arena *nodeArena) {
	if parser == nil || parser.language == nil {
		return
	}
	merge := &glrMergeScratch{
		language:                    parser.language,
		packedGSSVersionOrderActive: true,
		arena:                       arena,
		parser:                      parser,
	}
	parser.mergeScratch = merge
}

func TestCSelectReplacementParentEntryUsesExactCOrderWhenCertified(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := &Parser{language: &Language{CompactPackedGSSVersionOrderCertified: true}}
	activateCompactPackedGSSVersionOrderForTest(parser, arena)

	existing := cExactCompareParent(t, parser, arena, 10,
		cExactCompareLeaf(3),
		cExactCompareLeaf(1),
	)
	candidate := cExactCompareParent(t, parser, arena, 10,
		cExactCompareLeaf(2),
		cExactCompareLeaf(9),
	)

	if got := parser.cSelectReplacementParentEntry(arena, existing, candidate); got != cParentEntryUseCandidate {
		t.Fatalf("candidate selection = %d, want use-candidate", got)
	}
	if got := parser.cSelectReplacementParentEntry(arena, candidate, existing); got != cParentEntryRetainExisting {
		t.Fatalf("reverse candidate selection = %d, want retain-existing", got)
	}
}

func TestCSelectReplacementParentEntryRetainsExactEqualIncumbent(t *testing.T) {
	parser := &Parser{language: &Language{CompactPackedGSSVersionOrderCertified: true}}
	activateCompactPackedGSSVersionOrderForTest(parser, nil)
	existing := newStackEntryNode(1, &Node{
		symbol:    7,
		flags:     nodeFlagNamed,
		startByte: 1,
		endByte:   2,
	})
	candidate := newStackEntryNode(1, &Node{
		symbol:    7,
		flags:     nodeFlagNamed,
		startByte: 20,
		endByte:   40,
	})

	if got := parser.cSelectReplacementParentEntry(nil, existing, candidate); got != cParentEntryRetainExisting {
		t.Fatalf("equal candidate selection = %d, want retain-existing", got)
	}
}

func TestCSelectReplacementParentEntryUsesLaterPositiveErrorCandidate(t *testing.T) {
	parser := &Parser{language: &Language{CompactPackedGSSVersionOrderCertified: true}}
	existingNode := &Node{symbol: 7, flags: nodeFlagNamed}
	existingNode.setMissing(true)
	candidateNode := &Node{symbol: 7, flags: nodeFlagNamed}
	candidateNode.setMissing(true)
	existing := newStackEntryNode(1, existingNode)
	candidate := newStackEntryNode(1, candidateNode)

	existingCost := parser.rawStackEntryErrorCost(nil, existing)
	candidateCost := parser.rawStackEntryErrorCost(nil, candidate)
	if existingCost == 0 || existingCost != candidateCost {
		t.Fatalf("test error costs = %d/%d, want equal positive values", existingCost, candidateCost)
	}
	if got := parser.cSelectReplacementParentEntry(nil, existing, candidate); got != cParentEntryUseCandidate {
		t.Fatalf("positive-error candidate selection = %d, want use-candidate", got)
	}
}

func TestCAppendActionReductionVersionsStopsOnIncompleteExactSelection(t *testing.T) {
	parser := newCRecoverySyntheticReduceParser()
	parser.language.CompactPackedGSSVersionOrderCertified = true
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	activateCompactPackedGSSVersionOrderForTest(parser, arena)

	child := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	newShapelessParent := func() *Node {
		return newParentNodeInArena(arena, 4, true, []*Node{child}, nil, 0)
	}
	leftParent := newShapelessParent()
	rightParent := newShapelessParent()
	leftEntry := newStackEntryNode(9, leftParent)
	rightEntry := newStackEntryNode(9, rightParent)

	if got := parser.cSelectReplacementParentEntry(arena, leftEntry, rightEntry); got != cParentEntrySelectionIncomplete {
		t.Fatalf("certified shapeless selection = %d, want incomplete", got)
	}
	parser.mergeScratch.packedGSSVersionOrderActive = false
	if got := parser.cSelectReplacementParentEntry(arena, leftEntry, rightEntry); got != cParentEntryRetainExisting {
		t.Fatalf("inactive certified selection = %d, want retain-existing", got)
	}
	parser.mergeScratch.packedGSSVersionOrderActive = true
	parser.language.CompactPackedGSSVersionOrderCertified = false
	if got := parser.cSelectReplacementParentEntry(arena, leftEntry, rightEntry); got != cParentEntryRetainExisting {
		t.Fatalf("ordinary shapeless selection = %d, want retain-existing", got)
	}
	parser.language.CompactPackedGSSVersionOrderCertified = true

	var scratch gssScratch
	popTo := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	candidates := []glrStack{
		{gss: gssStack{head: scratch.allocNode(leftEntry, popTo, 2)}, byteOffset: 2},
		{gss: gssStack{head: scratch.allocNode(rightEntry, popTo, 2)}, byteOffset: 2},
	}
	if collapsed, reason := parser.cCollapseSamePopReductionCandidates(candidates, arena); reason != ParseStopInvariantViolation || collapsed != nil {
		t.Fatalf("same-pop collapse result = %d stacks, reason=%v; want nil and invariant violation", len(collapsed), reason)
	}

	olderPop := scratch.allocNode(stackEntry{state: 8}, nil, 1)
	olderParent := newParentNodeInArena(arena, 4, true, []*Node{child}, nil, 0)
	olderHead := scratch.allocNode(newStackEntryNode(9, olderParent), olderPop, 2)
	versions := []glrStack{
		{gss: gssStack{head: olderHead}, byteOffset: 2},
		{gss: gssStack{head: scratch.allocNode(stackEntry{state: 3}, popTo, 2)}, byteOffset: 2},
	}
	candidates = []glrStack{
		{gss: gssStack{head: scratch.allocNode(leftEntry, popTo, 2)}, byteOffset: 2},
		{gss: gssStack{head: scratch.allocNode(rightEntry, popTo, 2)}, byteOffset: 2},
	}

	gotVersions, reductionVersion, reason := parser.cAppendActionReductionVersions(versions, candidates, 1, arena)
	if reason != ParseStopInvariantViolation {
		t.Fatalf("action-version append stop reason = %v, want invariant violation", reason)
	}
	if reductionVersion != -1 {
		t.Fatalf("reduction version = %d, want -1", reductionVersion)
	}
	if len(gotVersions) != len(versions) || gotVersions[0].gss.head != olderHead {
		t.Fatal("action-version append changed the incumbent versions after an incomplete comparison")
	}
	if got := olderHead.linkCount(); got != 1 {
		t.Fatalf("older head link count = %d, want one before any later graph merge", got)
	}
}

func TestCDoAllPotentialReductionsCollapsesSamePopTargetSlicesByCParentSelection(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNode(3, right), leftNode, 3)
	altRight := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	altRight.dynamicPrecedence = 9
	rightNode.appendExtraLink(gssMainLink{
		prev:  leftNode,
		entry: newStackEntryNode(3, altRight),
	})
	start := glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}

	nodeCount := 0
	versions, canShift, reason := parser.cDoAllPotentialReductions(nil, start, 0, true, Token{}, &nodeCount, arena, nil, &scratch, nil, nil, nil)
	if reason != ParseStopNone {
		t.Fatalf("cDoAllPotentialReductions stop reason = %v, want none", reason)
	}
	if canShift {
		t.Fatal("canShift = true, want false")
	}
	if len(parser.pendingForkStacks) != 0 {
		t.Fatalf("pending forks = %d, want 0", len(parser.pendingForkStacks))
	}
	if len(versions) != 1 {
		t.Fatalf("version count = %d, want one selected version", len(versions))
	}
	if versions[0].dead {
		t.Fatal("selected reduction version is dead")
	}
	// The forked GSS reducer now applies the selected primary fork directly;
	// C recovery's invariant is that no internal pending fork survives this
	// potential-reduction probe.
	top := versions[0].top()
	if !stackEntryHasNode(top) {
		t.Fatal("selected reduction version top has no node")
	}
	if got, want := stackEntryNode(top).symbol, Symbol(4); got != want {
		t.Fatalf("selected reduction symbol = %d, want %d", got, want)
	}
	parent := stackEntryNode(top)
	if len(parent.children) != 2 || parent.children[1] != altRight {
		t.Fatal("selected parent children did not preserve the C-selected same-pop child array")
	}
	if got := versions[0].gss.head.linkCount(); got != 1 {
		t.Fatalf("reduced top link count = %d, want one selected same-pop alternative", got)
	}
}

func TestCDoAllPotentialReductionsCollapsesSamePopWithTrailingExtra(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	rightLow := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightHigh := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	rightHigh.dynamicPrecedence = 9
	extraLow := newLeafNodeInArena(arena, 5, false, 2, 3, Point{Column: 2}, Point{Column: 3})
	extraLow.setExtra(true)
	extraHigh := newLeafNodeInArena(arena, 6, false, 2, 3, Point{Column: 2}, Point{Column: 3})
	extraHigh.setExtra(true)

	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightLowNode := scratch.allocNode(newStackEntryNode(3, rightLow), leftNode, 3)
	rightHighNode := scratch.allocNode(newStackEntryNode(3, rightHigh), leftNode, 3)
	head := scratch.allocNode(newStackEntryNode(3, extraLow), rightLowNode, 4)
	head.appendExtraLink(gssMainLink{
		prev:  rightHighNode,
		entry: newStackEntryNode(3, extraHigh),
	})
	start := glrStack{gss: gssStack{head: head}, byteOffset: 3}

	nodeCount := 0
	versions, canShift, reason := parser.cDoAllPotentialReductions(nil, start, 0, true, Token{}, &nodeCount, arena, nil, &scratch, nil, nil, nil)
	if reason != ParseStopNone {
		t.Fatalf("cDoAllPotentialReductions stop reason = %v, want none", reason)
	}
	if canShift {
		t.Fatal("canShift = true, want false")
	}
	if len(versions) != 1 {
		t.Fatalf("version count = %d, want one selected version", len(versions))
	}
	if got := versions[0].gss.head.linkCount(); got != 1 {
		t.Fatalf("top link count = %d, want one selected extra replay path", got)
	}
	if stackEntryNode(versions[0].top()) != extraHigh {
		t.Fatal("selected trailing extra did not come from the C-selected same-pop child path")
	}
	parent := stackEntryNode(versions[0].gss.head.prev.entry)
	if parent == nil || parent.symbol != 4 {
		t.Fatalf("replayed extra predecessor = %+v, want reduced parent", parent)
	}
	if len(parent.children) != 2 || parent.children[1] != rightHigh {
		t.Fatal("trailing-extra same-pop collapse did not preserve selected parent children")
	}
}

func TestCAppendActionReductionVersionsCollapsesSamePopBeforeOlderMerge(t *testing.T) {
	old := glrFaithfulCapOneMerge
	glrFaithfulCapOneMerge = true
	t.Cleanup(func() { glrFaithfulCapOneMerge = old })

	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch gssScratch
	olderPop := scratch.allocNode(stackEntry{state: 8}, nil, 1)
	popTo := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	original := glrStack{gss: gssStack{head: scratch.allocNode(stackEntry{state: 3}, popTo, 2)}, byteOffset: 2}

	olderChild := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	lowChild := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	highChild := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	highChild.dynamicPrecedence = 7
	olderParent := newParentNodeInArena(arena, 4, true, []*Node{olderChild}, nil, 0)
	lowParent := newParentNodeInArena(arena, 4, true, []*Node{lowChild}, nil, 0)
	highParent := newParentNodeInArena(arena, 4, true, []*Node{highChild}, nil, 0)
	highParent.dynamicPrecedence = highChild.dynamicPrecedence

	versions := []glrStack{
		{gss: gssStack{head: scratch.allocNode(newStackEntryNode(9, olderParent), olderPop, 2)}, byteOffset: 2},
		original,
	}
	candidates := []glrStack{
		{gss: gssStack{head: scratch.allocNode(newStackEntryNode(9, lowParent), popTo, 2)}, byteOffset: 2},
		{gss: gssStack{head: scratch.allocNode(newStackEntryNode(9, highParent), popTo, 2)}, byteOffset: 2},
	}

	versions, reductionVersion, reason := parser.cAppendActionReductionVersions(versions, candidates, 1, arena)
	if reason != ParseStopNone {
		t.Fatalf("append action reduction versions stop reason = %v, want none", reason)
	}
	if reductionVersion != -1 {
		t.Fatalf("reduction version = %d, want merge into older existing version", reductionVersion)
	}
	if len(versions) != 2 {
		t.Fatalf("version count = %d, want older plus original", len(versions))
	}
	if got := versions[0].gss.head.linkCount(); got != 2 {
		t.Fatalf("older merged head link count = %d, want one selected reduction link added", got)
	}
	_, mergedEntry := versions[0].gss.head.link(1)
	if stackEntryNode(mergedEntry) != highParent {
		t.Fatal("older merge received uncollapsed lower-precedence same-pop parent")
	}
}

func cActionCellTransactionParser() *Parser {
	lang := &Language{
		TokenCount:  2,
		StateCount:  10,
		SymbolCount: 6,
		ParseTable:  make([][]uint16, 10),
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 7}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 8}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "token", Visible: true, Named: true},
			{Name: "leaf", Visible: true, Named: true},
			{Name: "unused", Visible: true, Named: true},
			{Name: "left_parent", Visible: true, Named: true},
			{Name: "right_parent", Visible: true, Named: true},
		},
	}
	lang.ParseTable[1] = []uint16{0, 0, 0, 0, 1, 2}
	return &Parser{language: lang, denseLimit: len(lang.ParseTable)}
}

func cActionCellTransactionStart(arena *nodeArena) glrStack {
	leaf := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	start := newGLRStack(1)
	start.pushEntry(newStackEntryNode(3, leaf), nil, nil)
	return start
}

type cActionCellTokenSource struct {
	tokens []Token
	index  int
}

func (s *cActionCellTokenSource) Next() Token {
	if s.index >= len(s.tokens) {
		return Token{}
	}
	tok := s.tokens[s.index]
	s.index++
	return tok
}

func cActionCellIntegrationLanguage(actions []ParseAction) *Language {
	return &Language{
		Name:                                  "c_action_cell_integration",
		SymbolCount:                           4,
		TokenCount:                            2,
		StateCount:                            5,
		InitialState:                          1,
		CompactPackedGSSVersionOrderCertified: true,
		SymbolNames:                           []string{"end", "x", "low_root", "high_root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false},
			{Name: "x", Visible: true},
			{Name: "low_root", Visible: true, Named: true},
			{Name: "high_root", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 2, ChildCount: 1}}},
			{Actions: actions},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: [][]uint16{
			make([]uint16, 4),
			{0, 1, 2, 4},
			{3, 0, 0, 0},
			{2, 0, 0, 0},
			{4, 0, 0, 0},
		},
	}
}

func TestCompactPackedGSSActionTransactionProcessesReductionVersionSameRound(t *testing.T) {
	tests := []struct {
		name    string
		actions []ParseAction
		want    string
	}{
		{
			name: "reduction before terminal survives",
			actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, DynamicPrecedence: 1},
				{Type: ParseActionAccept},
			},
			want: "high_root",
		},
		{
			name: "terminal before reduction stops scan",
			actions: []ParseAction{
				{Type: ParseActionAccept},
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, DynamicPrecedence: 1},
			},
			want: "low_root",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			language := cActionCellIntegrationLanguage(test.actions)
			parser := NewParser(language)
			parser.hasRootSymbol = false
			tree, err := parser.ParseWithTokenSource([]byte("x"), &cActionCellTokenSource{tokens: []Token{
				{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
				{Symbol: 0, StartByte: 1, EndByte: 1, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 1}},
			}})
			if err != nil {
				t.Fatalf("ParseWithTokenSource() error = %v", err)
			}
			if got := tree.RootNode().Type(language); got != test.want {
				t.Fatalf("root type = %q, want %q; tree=%s", got, test.want, tree.RootNode().SExpr(language))
			}
			if got := tree.ParseStopReason(); got != ParseStopAccepted {
				t.Fatalf("parse stop reason = %q, want accepted", got)
			}
		})
	}
}

func TestCompactPackedGSSActionTransactionRecomputesRetryFragility(t *testing.T) {
	language := cActionCellIntegrationLanguage([]ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
	})
	parser := NewParser(language)
	parser.hasRootSymbol = false
	tree, err := parser.ParseWithTokenSource([]byte("x"), &cActionCellTokenSource{tokens: []Token{
		{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
		{Symbol: 0, StartByte: 1, EndByte: 1, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 1}},
	}})
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	if got := tree.ParseStopReason(); got != ParseStopAccepted {
		t.Fatalf("parse stop reason = %q, want accepted", got)
	}
	root := tree.RootNode()
	if got := root.Type(language); got != "high_root" {
		t.Fatalf("root type = %q, want high_root; tree=%s", got, root.SExpr(language))
	}
	if root.isFragile() {
		t.Fatal("single surviving reduction version stayed fragile after promotion retry")
	}
}

func TestCompactPackedGSSActionTransactionRelexesAfterNoLookaheadReduction(t *testing.T) {
	language := &Language{
		Name:                                  "c_action_cell_no_lookahead",
		SymbolCount:                           3,
		TokenCount:                            2,
		StateCount:                            4,
		InitialState:                          1,
		CompactPackedGSSVersionOrderCertified: true,
		SymbolNames:                           []string{"end", "x", "root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false},
			{Name: "x", Visible: true},
			{Name: "root", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: [][]uint16{
			make([]uint16, 3),
			{1, 0, 2},
			{0, 2, 0},
			{3, 0, 0},
		},
	}
	parser := NewParser(language)
	parser.hasRootSymbol = false
	tokens := &cActionCellTokenSource{tokens: []Token{
		{Symbol: 0, NoLookahead: true},
		{Symbol: 1, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
		{Symbol: 0, StartByte: 1, EndByte: 1, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 1}},
	}}
	tree, err := parser.ParseWithTokenSource([]byte("x"), tokens)
	if err != nil {
		t.Fatalf("ParseWithTokenSource() error = %v", err)
	}
	if got := tree.ParseStopReason(); got != ParseStopAccepted {
		t.Fatalf("parse stop reason = %q, want accepted", got)
	}
	if tokens.index != len(tokens.tokens) {
		t.Fatalf("token reads = %d, want %d; promoted reduction reused null lookahead", tokens.index, len(tokens.tokens))
	}
}

func TestCAppendActionCellReductionVersionsPromotesLastSuccessfulReduction(t *testing.T) {
	parser := cActionCellTransactionParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	start := cActionCellTransactionStart(arena)
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
		{Type: ParseActionReduce, Symbol: 5, ChildCount: 1},
		// This result merges into the first reduction. C keeps the last
		// successful result instead of overwriting it with NONE.
		{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
	}
	nodeCount := 0
	anyReduced := false
	versions, lastReduction, terminalOrdinal, reason := parser.cAppendActionCellReductionVersions(
		nil, []glrStack{start}, 0, actions, Token{}, &nodeCount, arena,
		nil, nil, nil, nil, &anyReduced,
	)
	if reason != ParseStopNone {
		t.Fatalf("action-cell transaction stop reason = %v, want none", reason)
	}
	if terminalOrdinal != -1 || lastReduction != 2 {
		t.Fatalf("action-cell result terminal=%d reduction=%d, want -1/2", terminalOrdinal, lastReduction)
	}
	if !anyReduced {
		t.Fatal("action-cell reductions did not report round progress")
	}
	if got, want := len(versions), 3; got != want {
		t.Fatalf("version count = %d, want %d", got, want)
	}
	for i, want := range []StateID{3, 7, 8} {
		if got := versions[i].top().state; got != want {
			t.Fatalf("pre-promotion version[%d] state = %d, want %d", i, got, want)
		}
	}
	for i, want := range []Symbol{4, 5} {
		if got := stackEntryNode(versions[i+1].top()).symbol; got != want {
			t.Fatalf("reduction version[%d] symbol = %d, want %d", i+1, got, want)
		}
	}
	versions, ok := cRenumberReductionVersion(versions, lastReduction, 0)
	if !ok {
		t.Fatal("last successful reduction did not promote")
	}
	for i, want := range []Symbol{5, 4} {
		if got := stackEntryNode(versions[i].top()).symbol; got != want {
			t.Fatalf("post-promotion version[%d] symbol = %d, want %d", i, got, want)
		}
	}
}

func TestCAppendActionCellReductionVersionsStopsAtFirstTerminal(t *testing.T) {
	parser := cActionCellTransactionParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	start := cActionCellTransactionStart(arena)
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
		{Type: ParseActionShift, Repetition: true},
		{Type: ParseActionReduce, Symbol: 5, ChildCount: 1},
		{Type: ParseActionShift, State: 8},
		{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
	}
	nodeCount := 0
	anyReduced := false
	versions, lastReduction, terminalOrdinal, reason := parser.cAppendActionCellReductionVersions(
		nil, []glrStack{start}, 0, actions, Token{}, &nodeCount, arena,
		nil, nil, nil, nil, &anyReduced,
	)
	if reason != ParseStopNone {
		t.Fatalf("action-cell transaction stop reason = %v, want none", reason)
	}
	if terminalOrdinal != 3 || lastReduction != 2 {
		t.Fatalf("action-cell result terminal=%d reduction=%d, want 3/2", terminalOrdinal, lastReduction)
	}
	if !anyReduced {
		t.Fatal("action-cell reductions did not report round progress")
	}
	if got, want := len(versions), 3; got != want {
		t.Fatalf("version count = %d, want %d", got, want)
	}
	for i, want := range []Symbol{4, 5} {
		if got := stackEntryNode(versions[i+1].top()).symbol; got != want {
			t.Fatalf("reduction version[%d] symbol = %d, want %d", i+1, got, want)
		}
	}
}

func TestCAppendActionCellReductionVersionsTerminalOwnsOriginal(t *testing.T) {
	parser := cActionCellTransactionParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	start := cActionCellTransactionStart(arena)
	actions := []ParseAction{
		{Type: ParseActionShift, State: 8},
		{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
	}
	nodeCount := 0
	anyReduced := false
	versions, lastReduction, terminalOrdinal, reason := parser.cAppendActionCellReductionVersions(
		nil, []glrStack{start}, 0, actions, Token{}, &nodeCount, arena,
		nil, nil, nil, nil, &anyReduced,
	)
	if reason != ParseStopNone {
		t.Fatalf("action-cell transaction stop reason = %v, want none", reason)
	}
	if terminalOrdinal != 0 || lastReduction != -1 {
		t.Fatalf("action-cell result terminal=%d reduction=%d, want 0/-1", terminalOrdinal, lastReduction)
	}
	if anyReduced || len(versions) != 1 {
		t.Fatalf("later reduction ran: anyReduced=%t versions=%d", anyReduced, len(versions))
	}
	if parser.disablePostReduceForkMerge {
		t.Fatal("action-cell transaction did not restore post-reduce merge policy")
	}
}

func TestCAppendActionCellReductionVersionsRepetitionShiftIsNoOp(t *testing.T) {
	parser := cActionCellTransactionParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	start := cActionCellTransactionStart(arena)
	nodeCount := 0
	anyReduced := false
	versions, lastReduction, terminalOrdinal, reason := parser.cAppendActionCellReductionVersions(
		nil, []glrStack{start}, 0,
		[]ParseAction{{Type: ParseActionShift, Repetition: true}},
		Token{}, &nodeCount, arena, nil, nil, nil, nil, &anyReduced,
	)
	if reason != ParseStopNone {
		t.Fatalf("action-cell transaction stop reason = %v, want none", reason)
	}
	if terminalOrdinal != -1 || lastReduction != -1 {
		t.Fatalf("action-cell result terminal=%d reduction=%d, want -1/-1", terminalOrdinal, lastReduction)
	}
	if anyReduced || len(versions) != 1 {
		t.Fatalf("repetition shift changed transaction: anyReduced=%t versions=%d", anyReduced, len(versions))
	}
}

func TestCAppendActionCellReductionVersionsRejectsForeignPendingOwnership(t *testing.T) {
	tests := []struct {
		name  string
		queue func(*Parser) *[]glrStack
	}{
		{name: "post-reduce queue", queue: func(p *Parser) *[]glrStack { return &p.pendingForkStacks }},
		{name: "frontier queue", queue: func(p *Parser) *[]glrStack { return &p.pendingFrontierForkStacks }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := cActionCellTransactionParser()
			queue := test.queue(parser)
			*queue = append(*queue, newGLRStack(9))
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			start := cActionCellTransactionStart(arena)
			nodeCount := 0

			versions, lastReduction, terminalOrdinal, reason := parser.cAppendActionCellReductionVersions(
				nil, []glrStack{start}, 0,
				[]ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 1}},
				Token{}, &nodeCount, arena, nil, nil, nil, nil, nil,
			)
			if reason != ParseStopInvariantViolation {
				t.Fatalf("action-cell transaction stop reason = %v, want invariant violation", reason)
			}
			if lastReduction != -1 || terminalOrdinal != -1 || len(versions) != 1 {
				t.Fatalf("rejected transaction changed result: reduction=%d terminal=%d versions=%d", lastReduction, terminalOrdinal, len(versions))
			}
			if len(*queue) != 1 {
				t.Fatal("rejected transaction discarded a foreign pending fork")
			}
		})
	}
}

func TestCompactPackedGSSActionCellRequiresTransaction(t *testing.T) {
	tests := []struct {
		name    string
		actions []ParseAction
		want    bool
	}{
		{name: "empty"},
		{name: "single ordinary shift", actions: []ParseAction{{Type: ParseActionShift}}},
		{name: "single accept", actions: []ParseAction{{Type: ParseActionAccept}}},
		{name: "single reduction", actions: []ParseAction{{Type: ParseActionReduce}}, want: true},
		{name: "single repetition shift", actions: []ParseAction{{Type: ParseActionShift, Repetition: true}}, want: true},
		{name: "multiple terminals", actions: []ParseAction{{Type: ParseActionShift}, {Type: ParseActionAccept}}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := compactPackedGSSActionCellRequiresTransaction(test.actions); got != test.want {
				t.Fatalf("requires transaction = %t, want %t", got, test.want)
			}
		})
	}
}

func TestCompactPackedGSSDispatchVersionCount(t *testing.T) {
	if got := compactPackedGSSDispatchVersionCount(false, 1, 3); got != 1 {
		t.Fatalf("uncertified dispatch version count = %d, want round snapshot 1", got)
	}
	if got := compactPackedGSSDispatchVersionCount(true, 1, 3); got != 3 {
		t.Fatalf("certified dispatch version count = %d, want current physical count 3", got)
	}
}

func newCRecoverySyntheticReduceParser() *Parser {
	lang := &Language{
		TokenCount:  3,
		StateCount:  10,
		SymbolCount: 7,
		ParseTable: [][]uint16{
			nil,
			{0, 0, 0, 0, 2}, // goto parent(4) -> state 9.
			nil,
			{0, 1, 1},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 9}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "eof", Visible: true, Named: true},
			{Name: "a", Visible: true, Named: true},
			{Name: "b", Visible: true, Named: true},
			{Name: "unused", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
			{Name: "extra_low", Visible: false, Named: false},
			{Name: "extra_high", Visible: false, Named: false},
		},
	}
	return &Parser{language: lang, denseLimit: len(lang.ParseTable)}
}

func TestCRecoveryReductionPreservesTransientParentState(t *testing.T) {
	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch parserScratch
	scratch.reduce.transientParents = &scratch.transientParents
	parser.reduceScratch = &scratch.reduce
	defer func() {
		parser.reduceScratch = nil
	}()

	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	transient := scratch.transientParents.allocParent(
		arena,
		1,
		true,
		[]*Node{leaf},
		0,
		true,
	)
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	start := newGLRStack(1)
	start.pushEntry(newStackEntryNode(2, transient), &scratch.entries, &scratch.gss)
	start.pushEntry(newStackEntryNode(3, right), &scratch.entries, &scratch.gss)

	nodeCount := 0
	candidates, reason := parser.cReductionCandidatesForAction(
		nil,
		start,
		ParseAction{Type: ParseActionReduce, Symbol: 4, ChildCount: 2},
		Token{},
		&nodeCount,
		arena,
		&scratch.entries,
		&scratch.gss,
		nil,
		nil,
	)
	if reason != ParseStopNone {
		t.Fatalf("recovery reduction stop reason = %v, want none", reason)
	}
	if len(candidates) != 1 {
		t.Fatalf("recovery reduction candidates = %d, want 1", len(candidates))
	}
	if transient.parent != nil {
		t.Fatal("recovery reduction replaced transient parent state")
	}
}

func cRecoveryTmpEntriesLinearStack(arena *nodeArena, scratch *gssScratch) glrStack {
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	left := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	leftNode := scratch.allocNode(newStackEntryNode(2, left), base, 2)
	rightNode := scratch.allocNode(newStackEntryNode(3, right), leftNode, 3)
	return glrStack{gss: gssStack{head: rightNode}, byteOffset: 2}
}

func cRecoveryTmpEntriesPackedStack(arena *nodeArena, scratch *gssScratch) glrStack {
	start := cRecoveryTmpEntriesLinearStack(arena, scratch)
	link := start.gss.head
	alt := newLeafNodeInArena(arena, 2, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	link.appendExtraLink(gssMainLink{
		prev:  link.prev,
		entry: newStackEntryNode(3, alt),
	})
	return start
}

func TestCRecoveryReductionCandidatesReuseTmpEntries(t *testing.T) {
	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	var entryScratch glrEntryScratch
	var gss gssScratch
	start := cRecoveryTmpEntriesLinearStack(arena, &gss)
	tmpEntries := make([]stackEntry, 3, 8)
	backing := &tmpEntries[0]
	nodeCount := 0
	candidates, reason := parser.cReductionCandidatesForAction(
		nil,
		start,
		ParseAction{Type: ParseActionReduce, Symbol: 4, ChildCount: 2},
		Token{},
		&nodeCount,
		arena,
		&entryScratch,
		&gss,
		&tmpEntries,
		nil,
	)
	if reason != ParseStopNone {
		t.Fatalf("reduction candidate stop reason = %v, want none", reason)
	}
	if len(candidates) != 1 {
		t.Fatalf("reduction candidates = %d, want one", len(candidates))
	}
	if len(tmpEntries) != 0 || cap(tmpEntries) != 8 {
		t.Fatalf("tmp entries = len %d cap %d, want len 0 cap 8", len(tmpEntries), cap(tmpEntries))
	}
	if &tmpEntries[:cap(tmpEntries)][0] != backing {
		t.Fatal("reduction candidates replaced the temporary entry backing array")
	}
}

func TestCRecoveryReductionCandidatesCommitsGrownTmpEntries(t *testing.T) {
	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	var parserScratch parserScratch
	parserScratch.reduce.transientParents = &parserScratch.transientParents
	parser.reduceScratch = &parserScratch.reduce
	parser.forceRawSpanAll = true
	defer func() { parser.reduceScratch = nil }()
	var entryScratch glrEntryScratch
	var gss gssScratch
	var tmpEntries []stackEntry
	call := func() {
		start := cRecoveryTmpEntriesLinearStack(arena, &gss)
		nodeCount := 0
		candidates, reason := parser.cReductionCandidatesForAction(
			nil,
			start,
			ParseAction{Type: ParseActionReduce, Symbol: 4, ChildCount: 2},
			Token{},
			&nodeCount,
			arena,
			&entryScratch,
			&gss,
			&tmpEntries,
			nil,
		)
		if reason != ParseStopNone || len(candidates) != 1 {
			t.Fatalf("reduction candidates = %d, stop reason = %v; want one candidate and no stop", len(candidates), reason)
		}
	}
	call()
	if cap(tmpEntries) == 0 || len(tmpEntries) != 0 {
		t.Fatalf("grown tmp entries = len %d cap %d, want len 0 and positive capacity", len(tmpEntries), cap(tmpEntries))
	}
	backing := &tmpEntries[:cap(tmpEntries)][0]
	call()
	if len(tmpEntries) != 0 || &tmpEntries[:cap(tmpEntries)][0] != backing {
		t.Fatal("second recovery reduction did not reuse the grown temporary entry backing array")
	}
}

func TestCRecoveryPackedPathPreservesTmpEntries(t *testing.T) {
	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	var entryScratch glrEntryScratch
	var gss gssScratch
	start := cRecoveryTmpEntriesPackedStack(arena, &gss)
	tmpEntries := make([]stackEntry, 3, 8)
	backing := &tmpEntries[0]
	nodeCount := 0
	candidates, reason := parser.cReductionCandidatesForAction(
		nil,
		start,
		ParseAction{Type: ParseActionReduce, Symbol: 4, ChildCount: 2},
		Token{},
		&nodeCount,
		arena,
		&entryScratch,
		&gss,
		&tmpEntries,
		nil,
	)
	if reason != ParseStopNone {
		t.Fatalf("packed reduction stop reason = %v, want none", reason)
	}
	if len(candidates) == 0 {
		t.Fatal("packed reduction returned no candidates")
	}
	if len(tmpEntries) != 0 || cap(tmpEntries) != 8 {
		t.Fatalf("packed tmp entries = len %d cap %d, want len 0 cap 8", len(tmpEntries), cap(tmpEntries))
	}
	if &tmpEntries[:cap(tmpEntries)][0] != backing {
		t.Fatal("packed reduction discarded the master temporary entry backing array")
	}
}

func TestCRecoveryCondenseAndResumeThreadsTmpEntries(t *testing.T) {
	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	var entryScratch glrEntryScratch
	var gss gssScratch
	start := cRecoveryTmpEntriesLinearStack(arena, &gss)
	start.cPaused = true
	stacks := []glrStack{start}
	tmpEntries := make([]stackEntry, 0, 8)
	backing := &tmpEntries[:cap(tmpEntries)][0]
	trackChildErrors := false
	nodeCount := 0
	_, _, _, reason := parser.cCondenseAndResumeDetailed(
		stacks,
		nil,
		nil,
		Token{Symbol: 1, StartByte: 2, EndByte: 3, EndPoint: Point{Column: 3}},
		&nodeCount,
		arena,
		&entryScratch,
		&gss,
		&tmpEntries,
		nil,
		&trackChildErrors,
	)
	if reason != ParseStopNone {
		t.Fatalf("condense stop reason = %v, want none", reason)
	}
	if len(tmpEntries) != 0 || cap(tmpEntries) != 8 {
		t.Fatalf("condense tmp entries = len %d cap %d, want len 0 cap 8", len(tmpEntries), cap(tmpEntries))
	}
	if &tmpEntries[:cap(tmpEntries)][0] != backing {
		t.Fatal("condense did not preserve the threaded temporary entry backing array")
	}
}

func TestCRecoveryWarmPathReducesAllocations(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	// Retain the materialized window so a cold nil-scratch path must allocate.
	measure := func(warm bool) float64 {
		var gss gssScratch
		var tmpEntries []stackEntry
		if warm {
			tmpEntries = make([]stackEntry, 0, 8)
		}
		var retained []stackEntry
		invoke := func() {
			gss.reset()
			arena.reset()
			start := cRecoveryTmpEntriesLinearStack(arena, &gss)
			window, _, ok := reduceWindowFromGSS(&start, 2, tmpEntries)
			if !ok {
				t.Fatal("warm-path reduction window did not materialize")
			}
			retained = window
			if warm {
				tmpEntries = window[:0]
			}
		}
		invoke()
		if len(retained) == 0 {
			t.Fatal("warm-path reduction window was empty")
		}
		return testing.AllocsPerRun(100, invoke)
	}

	cold := measure(false)
	warm := measure(true)
	if warm >= cold {
		t.Fatalf("warm recovery path allocations = %g, cold path = %g, want warm path lower", warm, cold)
	}
}

func TestCRecoveryReleaseClearsPointers(t *testing.T) {
	var scratch parserScratch
	sentinel := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})
	scratch.tmpEntries = make([]stackEntry, 1, 4)
	scratch.tmpEntries[0] = newStackEntryNode(1, sentinel)
	releaseParserScratch(&scratch, false)
	if cap(scratch.tmpEntries) != 4 || len(scratch.tmpEntries) != 0 {
		t.Fatalf("released tmp entries = len %d cap %d, want len 0 cap 4", len(scratch.tmpEntries), cap(scratch.tmpEntries))
	}
	for i, entry := range scratch.tmpEntries[:cap(scratch.tmpEntries)] {
		if entry.node != nil {
			t.Fatalf("released tmp entry %d retains node pointer", i)
		}
	}
}

// TestCDoAllPotentialReductionsKeepsShiftableOriginalWithReductionFork pins
// C's version bookkeeping (parser.c ts_parser__do_all_potential_reductions):
// a version whose state has a non-extra shift action for SOME token is KEPT
// as its own path, and its reduction results become separate versions —
// `if (has_shift_action) can_shift = true; else if (reduction_version ...)
// renumber`. Replacing the shiftable original with its reduction drops the
// merged version's original-shape path and the summary entries C's
// strategy-1 scan elects from it (authzed stray backtick: the (depth 1,
// pre-reduction state) entry closes binary_expression on the newline).
func TestCDoAllPotentialReductionsKeepsShiftableOriginalWithReductionFork(t *testing.T) {
	lang := &Language{
		TokenCount:  2,
		StateCount:  10,
		SymbolCount: 5,
		ParseTable: [][]uint16{
			nil,
			{0, 0, 0, 0, 2}, // goto parent(4) -> state 9.
			nil,
			{0, 1}, // token 1 has both reduce(parent) and shift.
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
				{Type: ParseActionShift, State: 8},
			}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 9}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "tok", Visible: true, Named: true},
			{Name: "leaf", Visible: true, Named: true},
			{Name: "unused", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, denseLimit: len(lang.ParseTable)}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	start := newGLRStack(1)
	start.pushEntry(newStackEntryNode(3, leaf), nil, nil)

	nodeCount := 0
	versions, canShift, reason := parser.cDoAllPotentialReductions(nil, start, 0, true, Token{}, &nodeCount, arena, nil, nil, nil, nil, nil)
	if reason != ParseStopNone {
		t.Fatalf("cDoAllPotentialReductions stop reason = %v, want none", reason)
	}
	if !canShift {
		t.Fatal("canShift = false, want true from the shiftable original state")
	}
	if len(versions) != 2 {
		t.Fatalf("version count = %d, want shiftable original retained alongside its reduction fork", len(versions))
	}
	if got := versions[0].top().state; got != StateID(3) {
		t.Fatalf("original top state = %d, want pre-reduction state 3 kept", got)
	}
	if got := versions[1].top().state; got != StateID(9) {
		t.Fatalf("reduction fork top state = %d, want reduced goto state 9", got)
	}
	if top := versions[1].top(); !stackEntryHasNode(top) || stackEntryNode(top).symbol != 4 {
		t.Fatalf("reduction fork top entry = %+v, want reduced parent symbol", top)
	}
}

func cCallerSeedShiftableParserAndStack(arena *nodeArena) (*Parser, glrStack) {
	lang := &Language{
		TokenCount:  2,
		StateCount:  10,
		SymbolCount: 5,
		ParseTable: [][]uint16{
			nil,
			{0, 0, 0, 0, 2},
			nil,
			{0, 1},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
				{Type: ParseActionShift, State: 8},
			}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 9}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "tok", Visible: true, Named: true},
			{Name: "leaf", Visible: true, Named: true},
			{Name: "unused", Visible: true, Named: true},
			{Name: "parent", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, denseLimit: len(lang.ParseTable)}
	leaf := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	start := newGLRStack(1)
	start.pushEntry(newStackEntryNode(3, leaf), nil, nil)
	return parser, start
}

func cCallerSeedFallbackParserAndStack(arena *nodeArena) (*Parser, glrStack) {
	lang := &Language{
		TokenCount:  4,
		StateCount:  12,
		SymbolCount: 7,
		ParseTable:  make([][]uint16, 12),
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 1}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 5, ChildCount: 1}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 6, ChildCount: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 9}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 10}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 11}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "a", Visible: true, Named: true},
			{Name: "b", Visible: true, Named: true},
			{Name: "c", Visible: true, Named: true},
			{Name: "parent_a", Visible: true, Named: true},
			{Name: "parent_b", Visible: true, Named: true},
			{Name: "parent_c", Visible: true, Named: true},
		},
	}
	lang.ParseTable[1] = []uint16{0, 0, 0, 0, 4, 5, 6}
	lang.ParseTable[3] = []uint16{0, 1, 2, 3}
	parser := &Parser{language: lang, denseLimit: len(lang.ParseTable)}
	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	start := newGLRStack(1)
	start.pushEntry(newStackEntryNode(3, leaf), nil, nil)
	return parser, start
}

func TestCDoAllPotentialReductionsCallerSeedFirstFork(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser, start := cCallerSeedShiftableParserAndStack(arena)
	var callerSeed [2]glrStack
	nodeCount := 0
	versions, canShift, reason := parser.cDoAllPotentialReductions(nil, start, 0, true, Token{}, &nodeCount, arena, nil, nil, nil, nil, callerSeed[:0])
	if reason != ParseStopNone {
		t.Fatalf("cDoAllPotentialReductions stop reason = %v, want none", reason)
	}
	if !canShift || len(versions) != 2 {
		t.Fatalf("caller-seeded versions: canShift=%v len=%d, want true/2", canShift, len(versions))
	}
	if &versions[0] != &callerSeed[0] || &versions[1] != &callerSeed[1] {
		t.Fatal("first reduction fork did not preserve caller-seed ownership")
	}
	if got, want := versions[0].top().state, StateID(3); got != want {
		t.Fatalf("caller-seeded version[0] state = %d, want original state %d", got, want)
	}
	if got, want := versions[1].top().state, StateID(9); got != want {
		t.Fatalf("caller-seeded version[1] state = %d, want reduction state %d", got, want)
	}
}

func TestCDoAllPotentialReductionsCallerSeedFallback(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser, start := cCallerSeedFallbackParserAndStack(arena)
	var callerSeed [2]glrStack
	nodeCount := 0
	versions, canShift, reason := parser.cDoAllPotentialReductions(nil, start, 0, true, Token{}, &nodeCount, arena, nil, nil, nil, nil, callerSeed[:0])
	if reason != ParseStopNone {
		t.Fatalf("cDoAllPotentialReductions stop reason = %v, want none", reason)
	}
	if canShift || len(versions) != 3 {
		t.Fatalf("fallback versions: canShift=%v len=%d, want false/3", canShift, len(versions))
	}
	if &versions[0] == &callerSeed[0] || &versions[1] == &callerSeed[1] {
		t.Fatal("fallback growth retained an alias to the caller seed")
	}
	seedByteOffset := callerSeed[0].byteOffset
	versions[0].byteOffset++
	if callerSeed[0].byteOffset != seedByteOffset {
		t.Fatal("fallback result still aliases caller-seed storage")
	}
}

func TestCAppendReductionVersionCallerSeedFirstAppendAllocatesZero(t *testing.T) {
	parser := &Parser{}
	var callerSeed [2]glrStack
	candidate := glrStack{byteOffset: 1}
	allocs := testing.AllocsPerRun(1000, func() {
		versions := callerSeed[:1]
		versions[0] = glrStack{}
		out, appended := parser.cAppendReductionVersion(versions, candidate, 0)
		if !appended || len(out) != 2 {
			t.Fatalf("caller-seeded append: appended=%v len=%d, want true/2", appended, len(out))
		}
	})
	if allocs != 0 {
		t.Fatalf("caller-seeded first append allocations = %v, want zero", allocs)
	}
}

func TestCRenumberReductionVersionPreservesPhysicalOrderAndClearsTail(t *testing.T) {
	versions := make([]glrStack, 4, 5)
	for i, state := range []StateID{236, 269, 320, 240} {
		versions[i] = newGLRStack(state)
		versions[i].branchOrder = uint64(i + 1)
	}
	backing := versions[:cap(versions)]

	var ok bool
	versions, ok = cRenumberReductionVersion(versions, 3, 0)
	if !ok {
		t.Fatal("renumber failed")
	}
	if got, want := len(versions), 3; got != want {
		t.Fatalf("version count = %d, want %d", got, want)
	}
	for i, want := range []StateID{240, 269, 320} {
		if got := versions[i].top().state; got != want {
			t.Fatalf("version[%d] state = %d, want %d", i, got, want)
		}
	}
	if got, want := versions[0].branchOrder, uint64(4); got != want {
		t.Fatalf("promoted branch order = %d, want %d", got, want)
	}
	if backing[3].gss.head != nil || backing[3].entries != nil || backing[3].branchOrder != 0 {
		t.Fatalf("retired tail = %+v, want cleared", backing[3])
	}
}

func TestCRemoveReductionVersionUsesStablePhysicalCompaction(t *testing.T) {
	versions := make([]glrStack, 4)
	for i, state := range []StateID{236, 269, 320, 240} {
		versions[i] = newGLRStack(state)
	}
	backing := versions

	var ok bool
	versions, ok = cRemoveReductionVersion(versions, 1)
	if !ok {
		t.Fatal("remove failed")
	}
	for i, want := range []StateID{236, 320, 240} {
		if got := versions[i].top().state; got != want {
			t.Fatalf("version[%d] state = %d, want %d", i, got, want)
		}
	}
	if backing[3].gss.head != nil || backing[3].entries != nil || backing[3].branchOrder != 0 {
		t.Fatalf("retired tail = %+v, want cleared", backing[3])
	}

	if out, removed := cRemoveReductionVersion(versions, len(versions)); removed || len(out) != len(versions) {
		t.Fatalf("invalid remove changed versions: removed=%t len=%d", removed, len(out))
	}
	if out, renumbered := cRenumberReductionVersion(versions, 0, 1); renumbered || len(out) != len(versions) {
		t.Fatalf("invalid renumber changed versions: renumbered=%t len=%d", renumbered, len(out))
	}
}

func TestCDoAllPotentialReductionsDistinguishesEOFFromAnyLookahead(t *testing.T) {
	lang := &Language{
		TokenCount:  2,
		StateCount:  4,
		SymbolCount: 3,
		ParseTable: [][]uint16{
			nil,
			{1: 1}, // state 1 can shift a real token, but has no EOF action.
			{0: 2}, // state 2 can accept EOF.
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "tok", Visible: true, Named: true},
			{Name: "node", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, denseLimit: len(lang.ParseTable)}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	nodeCount := 0
	versions, canShift, reason := parser.cDoAllPotentialReductions(nil, newGLRStack(1), 0, true, Token{}, &nodeCount, arena, nil, nil, nil, nil, nil)
	if reason != ParseStopNone {
		t.Fatalf("cDoAllPotentialReductions stop reason = %v, want none", reason)
	}
	if !canShift || len(versions) != 1 {
		t.Fatalf("any-lookahead reductions: canShift=%v versions=%d, want true/1", canShift, len(versions))
	}

	versions, canShift, reason = parser.cDoAllPotentialReductions(nil, newGLRStack(1), 0, false, Token{}, &nodeCount, arena, nil, nil, nil, nil, nil)
	if reason != ParseStopNone {
		t.Fatalf("cDoAllPotentialReductions stop reason = %v, want none", reason)
	}
	if canShift || len(versions) != 0 {
		t.Fatalf("exact EOF reductions on non-EOF state: canShift=%v versions=%d, want false/0", canShift, len(versions))
	}

	versions, canShift, reason = parser.cDoAllPotentialReductions(nil, newGLRStack(2), 0, false, Token{}, &nodeCount, arena, nil, nil, nil, nil, nil)
	if reason != ParseStopNone {
		t.Fatalf("cDoAllPotentialReductions stop reason = %v, want none", reason)
	}
	if !canShift || len(versions) != 1 {
		t.Fatalf("exact EOF accept reductions: canShift=%v versions=%d, want true/1", canShift, len(versions))
	}
}

func TestCResultSelectionPrefersLaterEqualErrorCostTree(t *testing.T) {
	lang := &Language{
		TokenCount:  2,
		StateCount:  2,
		SymbolCount: 3,
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "translation_unit", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, errorCostCompetition: true}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leftMissing := newLeafNodeInArena(arena, 1, true, 0, 0, Point{}, Point{})
	leftMissing.setMissing(true)
	leftMissing.setHasError(true)
	rightMissing := newLeafNodeInArena(arena, 1, true, 0, 0, Point{}, Point{})
	rightMissing.setMissing(true)
	rightMissing.setHasError(true)

	left := glrStack{accepted: true}
	left.pushEntry(newStackEntryNode(1, leftMissing), nil, nil)
	right := glrStack{accepted: true}
	right.pushEntry(newStackEntryNode(1, rightMissing), nil, nil)

	if lc, rc := parser.cStackResultErrorCost(&left), parser.cStackResultErrorCost(&right); lc == 0 || lc != rc {
		t.Fatalf("test setup costs = %d/%d, want equal nonzero", lc, rc)
	}
	if got := stackCompareForResultSelection(parser, arena, &right, &left, false); got <= 0 {
		t.Fatalf("later equal-error candidate compare = %d, want preferred", got)
	}
}

func makeCCondenseMissingGroupTestStack(topState StateID, nodes int, missingTop bool) glrStack {
	if nodes < 1 {
		nodes = 1
	}
	s := newGLRStack(1)
	for i := 0; i < nodes; i++ {
		start := uint32(i)
		end := start + 1
		n := NewLeafNode(1, true, start, end, Point{Column: start}, Point{Column: end})
		if missingTop && i == nodes-1 {
			n.startByte = start
			n.endByte = start
			n.startPoint = Point{Column: start}
			n.endPoint = Point{Column: start}
			n.setMissing(true)
			n.setHasError(true)
		}
		state := StateID(2 + i)
		if i == nodes-1 {
			state = topState
		}
		s.push(state, n, nil, nil)
	}
	if missingTop {
		s.byteOffset = uint32(nodes)
	}
	return s
}

func TestCRecoveryRelevantStackIncludesMissingGroup(t *testing.T) {
	if cRecoveryRelevantStack([]glrStack{{cRecoverMissingGroup: &cRecGroup{}}}) != true {
		t.Fatal("cRecoveryRelevantStack = false for missing recovery group")
	}
}

func TestCCondenseAndResumeComparesMissingGroupStacks(t *testing.T) {
	lang := &Language{
		TokenCount:  2,
		StateCount:  8,
		SymbolCount: 2,
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true, Named: true},
			{Name: "item", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, errorCostCompetition: true}
	clean := makeCCondenseMissingGroupTestStack(7, 4, false)
	missing := makeCCondenseMissingGroupTestStack(7, 4, true)
	missing.cRecoverMissingGroup = &cRecGroup{}
	if clean.top().state != missing.top().state || clean.byteOffset != missing.byteOffset {
		t.Fatalf("test setup headers differ: clean=%d@%d missing=%d@%d", clean.top().state, clean.byteOffset, missing.top().state, missing.byteOffset)
	}
	if cleanCost, missingCost := parser.cStackErrorCost(&clean), parser.cStackErrorCost(&missing); cleanCost >= missingCost {
		t.Fatalf("test setup costs clean=%d missing=%d, want clean lower", cleanCost, missingCost)
	}

	var nodeCount int
	condensed, resumed, _, reason := parser.cCondenseAndResume([]glrStack{missing, clean}, nil, nil, Token{Symbol: 1}, &nodeCount, nil, nil, nil, nil, nil, nil)
	if reason != ParseStopNone {
		t.Fatalf("cCondenseAndResume stop reason = %v, want none", reason)
	}
	if resumed {
		t.Fatal("cCondenseAndResume resumed without paused/error-state versions")
	}
	if len(condensed) != 1 {
		t.Fatalf("condensed len = %d, want 1", len(condensed))
	}
	if condensed[0].cRecoverMissingGroup != nil {
		t.Fatal("condense kept missing-group stack; want lower-cost clean stack")
	}
}

func TestCCondenseVersionRankScratchMatchesLegacyMap(t *testing.T) {
	key := func(state StateID) cCondenseVersionKey {
		return cCondenseVersionKey{state: state}
	}
	cases := []struct {
		name string
		keys []cCondenseVersionKey
	}{
		{
			name: "accepted",
			keys: []cCondenseVersionKey{key(0), key(1), key(2), key(3), key(4), key(5)},
		},
		{
			name: "repeated",
			keys: []cCondenseVersionKey{key(0), key(1), key(2), key(3), key(4), key(5), key(0), key(5), key(1)},
		},
		{
			name: "excess",
			keys: []cCondenseVersionKey{key(0), key(1), key(2), key(3), key(4), key(5), key(6), key(7)},
		},
		{
			name: "repeated-excess",
			keys: []cCondenseVersionKey{key(0), key(1), key(2), key(3), key(4), key(5), key(6), key(6), key(7), key(6), key(1), key(7)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			legacy := make(map[cCondenseVersionKey]int, len(tc.keys))
			scratch := make(map[cCondenseVersionKey]uint8, cRecoverMaxVersionCount)
			for i, key := range tc.keys {
				wantRank, seen := legacy[key]
				if !seen {
					wantRank = len(legacy)
					legacy[key] = wantRank
				}
				gotRank := cCondenseVersionRankFor(scratch, key)
				if (gotRank < cRecoverMaxVersionCount) != (wantRank < cRecoverMaxVersionCount) {
					t.Fatalf("key %d rank decision = %d, want legacy rank %d", i, gotRank, wantRank)
				}
			}
			if got := len(scratch); got > cRecoverMaxVersionCount {
				t.Fatalf("scratch stored %d keys, want at most %d", got, cRecoverMaxVersionCount)
			}
			for i := 0; i < cRecoverMaxVersionCount; i++ {
				if _, ok := scratch[key(StateID(i))]; !ok {
					t.Fatalf("scratch lost accepted key %d", i)
				}
			}
			for i := cRecoverMaxVersionCount; i < 8; i++ {
				if _, ok := scratch[key(StateID(i))]; ok {
					t.Fatalf("scratch retained excess key %d", i)
				}
			}
		})
	}
}

func TestCCondenseVersionRankScratchResetClearsKeys(t *testing.T) {
	parser := &Parser{cCondenseVersionKeyRanks: make(map[cCondenseVersionKey]uint8, cRecoverMaxVersionCount)}
	parser.cCondenseVersionKeyRanks[cCondenseVersionKey{state: 1}] = 0
	resetSnippetParser(parser)
	if got := len(parser.cCondenseVersionKeyRanks); got != 0 {
		t.Fatalf("reset retained %d condense keys, want 0", got)
	}
}

func TestCCondenseVersionRankScratchParseExitClearsKeys(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	parser.SetAdmissionCandidateRoute(false)
	parser.cCondenseVersionKeyRanks = make(map[cCondenseVersionKey]uint8, cRecoverMaxVersionCount)
	parser.cCondenseVersionKeyRanks[cCondenseVersionKey{state: 1}] = 0
	tree, err := parser.Parse([]byte("1 + 2"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tree == nil {
		t.Fatal("parse returned nil tree")
	}
	tree.Release()
	if got := len(parser.cCondenseVersionKeyRanks); got != 0 {
		t.Fatalf("parse exit retained %d condense keys, want 0", got)
	}
}

func TestCCondenseVersionRankScratchLayout(t *testing.T) {
	var parser Parser
	parserSize := unsafe.Sizeof(parser)
	scratchOffset := unsafe.Offsetof(parser.cCondenseVersionKeyRanks)
	scratchSize := unsafe.Sizeof(parser.cCondenseVersionKeyRanks)
	if scratchOffset+scratchSize != parserSize {
		t.Fatalf("condense rank scratch is not at Parser tail: offset=%d size=%d parser_size=%d", scratchOffset, scratchSize, parserSize)
	}
	t.Logf("P23_LAYOUT parser_size=%d scratch_offset=%d scratch_size=%d key_size=%d rank_size=%d", parserSize, scratchOffset, scratchSize, unsafe.Sizeof(cCondenseVersionKey{}), unsafe.Sizeof(uint8(0)))
}

func collidingCNodeMemoNodes(t *testing.T, setCount int) (*Node, *Node, *Node) {
	t.Helper()
	nodes := make([]Node, setCount*2+1)
	seen := make(map[int][]*Node, setCount)
	for i := range nodes {
		n := &nodes[i]
		n.equivVersion = 1
		idx := cNodeMemoCacheIndex(uintptr(unsafe.Pointer(n)), setCount)
		seen[idx] = append(seen[idx], n)
		if len(seen[idx]) == 3 {
			return seen[idx][0], seen[idx][1], seen[idx][2]
		}
	}
	t.Fatal("failed to find three nodes in one recovery memo set")
	return nil, nil, nil
}

func TestCNodeMemoEpochAdvancesAcrossParserParses(t *testing.T) {
	p := NewParser(buildArithmeticLanguage())
	// Pin to production: this test asserts a production-engine internal (the C
	// recovery memo epoch), which the compact candidate route never advances.
	p.SetAdmissionCandidateRoute(false)
	p.errorCostCompetition = true

	first, err := p.Parse([]byte("1"))
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	memoRuntime := first.RecoveryNodeMemoRuntime()
	if memoRuntime.PeakTier != RecoveryNodeMemoTierInitial || memoRuntime.Collisions != 0 {
		t.Fatalf("first parse memo runtime = %+v", memoRuntime)
	}
	first.Release()
	firstEpoch := p.cNodeMemoEpoch
	if firstEpoch == 0 {
		t.Fatal("first parse left recovery memo epoch at zero")
	}

	second, err := p.Parse([]byte("1"))
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	second.Release()
	if got, want := p.cNodeMemoEpoch, firstEpoch+1; got != want {
		t.Fatalf("second parse recovery memo epoch = %d, want %d", got, want)
	}
}

func TestCNodeMemoSlotPreservesVictimPromotionAndEviction(t *testing.T) {
	p := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheInitialSize)}
	p.beginCNodeMemoEpoch()
	a, b, c := collidingCNodeMemoNodes(t, len(p.cNodeMemoCache)>>1)

	store := func(n *Node, cost uint32) {
		slot := p.cNodeMemoSlot(n)
		*slot = cNodeMemoCacheEntry{
			node:    uintptr(unsafe.Pointer(n)),
			ver:     n.equivVersion,
			cost:    cost,
			hasCost: true,
			epoch:   p.cNodeMemoEpoch,
		}
	}
	store(a, 11)
	store(b, 22)

	idx := cNodeMemoCacheIndex(uintptr(unsafe.Pointer(a)), len(p.cNodeMemoCache)>>1)
	if got := p.cNodeMemoCache[idx].node; got != uintptr(unsafe.Pointer(b)) {
		t.Fatalf("primary after collision = %#x, want b %#x", got, uintptr(unsafe.Pointer(b)))
	}
	if got := p.cNodeMemoCache[idx+1].node; got != uintptr(unsafe.Pointer(a)) {
		t.Fatalf("victim after collision = %#x, want a %#x", got, uintptr(unsafe.Pointer(a)))
	}

	promoted := p.cNodeMemoSlot(a)
	if promoted.cost != 11 || !promoted.hasCost {
		t.Fatalf("promoted victim payload = %#v, want cached cost 11", *promoted)
	}
	if got := p.cNodeMemoCache[idx+1].node; got != uintptr(unsafe.Pointer(b)) {
		t.Fatalf("victim after promotion = %#x, want b %#x", got, uintptr(unsafe.Pointer(b)))
	}

	store(c, 33)
	if got := p.cNodeMemoCache[idx].node; got != uintptr(unsafe.Pointer(c)) {
		t.Fatalf("primary after miss = %#x, want c %#x", got, uintptr(unsafe.Pointer(c)))
	}
	if got := p.cNodeMemoCache[idx+1].node; got != uintptr(unsafe.Pointer(a)) {
		t.Fatalf("victim after miss = %#x, want prior primary a %#x", got, uintptr(unsafe.Pointer(a)))
	}
}

func TestCNodeMemoEpochWrapClearsBeforeEpochReuse(t *testing.T) {
	p := &Parser{
		cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheInitialSize),
		cNodeMemoEpoch: ^uint16(0),
	}
	n := &Node{equivVersion: 1}
	idx := cNodeMemoCacheIndex(uintptr(unsafe.Pointer(n)), len(p.cNodeMemoCache)>>1)
	p.cNodeMemoCache[idx] = cNodeMemoCacheEntry{
		node:    uintptr(unsafe.Pointer(n)),
		ver:     n.equivVersion,
		cost:    41,
		hasCost: true,
		epoch:   p.cNodeMemoEpoch,
	}

	p.beginCNodeMemoEpoch()
	if p.cNodeMemoEpoch != 1 {
		t.Fatalf("memo epoch after wrap = %d, want 1", p.cNodeMemoEpoch)
	}
	for i, entry := range p.cNodeMemoCache {
		if entry != (cNodeMemoCacheEntry{}) {
			t.Fatalf("cache slot %d after epoch wrap = %#v, want zero", i, entry)
		}
	}
}

func TestCNodeMemoCacheEntrySize(t *testing.T) {
	want := uintptr(20)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		want = 24
	}
	if got := unsafe.Sizeof(cNodeMemoCacheEntry{}); got != want {
		t.Fatalf("cNodeMemoCacheEntry size = %d, want %d", got, want)
	}
}

func TestCStackSummaryEntrySize(t *testing.T) {
	if got, want := unsafe.Sizeof(cStackSummaryEntry{}), uintptr(16); got != want {
		t.Fatalf("cStackSummaryEntry size = %d, want %d", got, want)
	}
}

func TestRecoveryMemoTelemetryPreservesAMD64HotLayouts(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("amd64 layout ratchet")
	}
	// The scratch lexer shares an eight-byte dependency observer pointer.
	if got, want := unsafe.Sizeof(Parser{}), uintptr(2232); got != want {
		t.Fatalf("Parser size = %d, want %d", got, want)
	}
	// Compact incremental results add 48 bytes of route, reuse, and work telemetry.
	if got, want := unsafe.Sizeof(ParseRuntime{}), uintptr(3088); got != want {
		t.Fatalf("ParseRuntime size = %d, want %d", got, want)
	}
	if got, want := unsafe.Sizeof(Tree{}), uintptr(3288); got != want {
		t.Fatalf("Tree size = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(Parser{}.cNodeMemoPeakTier), unsafe.Offsetof(Parser{}.crecoveryCostCompetitionRelevant)+2; got != want {
		t.Fatalf("Parser memo peak offset = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(Parser{}.fullParseRetryPassesTaken), uintptr(1032); got != want {
		t.Fatalf("Parser full-parse retry offset = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(Tree{}.recoveryNodeMemoPeakTier), unsafe.Offsetof(Tree{}.released)+1; got != want {
		t.Fatalf("Tree memo peak offset = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(Tree{}.recoveryNodeMemoCollisions)+unsafe.Sizeof(Tree{}.recoveryNodeMemoCollisions), unsafe.Sizeof(Tree{}); got != want {
		t.Fatalf("Tree memo telemetry end = %d, want %d", got, want)
	}
}

func TestRecoveryNodeMemoTierMetrics(t *testing.T) {
	if got, want := RecoveryNodeMemoTierTemporary.Entries(), uint32(cNodeMemoRecoveryCacheSize); got != want {
		t.Fatalf("temporary tier entries = %d, want %d", got, want)
	}
	if got, want := RecoveryNodeMemoTierTemporary.Bytes(), uint32(cNodeMemoCacheBytesForEntries(cNodeMemoRecoveryCacheSize)); got != want {
		t.Fatalf("temporary tier bytes = %d, want %d", got, want)
	}
	tree := &Tree{}
	tree.setRecoveryNodeMemoRuntime(RecoveryNodeMemoRuntime{
		PeakTier:   RecoveryNodeMemoTierTemporary,
		Collisions: 123,
	})
	if got := tree.RecoveryNodeMemoRuntime(); got.PeakTier != RecoveryNodeMemoTierTemporary || got.Collisions != 123 {
		t.Fatalf("tree memo runtime = %+v", got)
	}
}

func TestCNodeMemoRecoveryTierFitsMemoryBudget(t *testing.T) {
	if cNodeMemoRecoveryCacheSize&(cNodeMemoRecoveryCacheSize-1) != 0 {
		t.Fatalf("temporary memo entries = %d, want a power of two", cNodeMemoRecoveryCacheSize)
	}
	if cNodeMemoRecoveryCacheSize <= cNodeMemoCacheSize {
		t.Fatalf("temporary memo entries = %d, want more than standard entries %d", cNodeMemoRecoveryCacheSize, cNodeMemoCacheSize)
	}
	const (
		recoveryMemoActiveBudgetBytes   = 3 << 20
		recoveryMemoRetainedBudgetBytes = 384 << 10
		recoveryMemoCombinedBudgetBytes = recoveryMemoActiveBudgetBytes + recoveryMemoRetainedBudgetBytes
	)
	activeBytes := cNodeMemoCacheBytesForEntries(cNodeMemoRecoveryCacheSize)
	if activeBytes > recoveryMemoActiveBudgetBytes {
		t.Fatalf("active temporary memo bytes = %d, exceeds budget %d", activeBytes, recoveryMemoActiveBudgetBytes)
	}
	retainedBytes := cNodeMemoCacheBytesForEntries(cNodeMemoCacheSize)
	if retainedBytes > recoveryMemoRetainedBudgetBytes {
		t.Fatalf("retained standard memo bytes = %d, exceeds budget %d", retainedBytes, recoveryMemoRetainedBudgetBytes)
	}
	if combinedBytes := activeBytes + retainedBytes; combinedBytes > recoveryMemoCombinedBudgetBytes {
		t.Fatalf("combined memo bytes = %d, exceeds budget %d", combinedBytes, recoveryMemoCombinedBudgetBytes)
	}
}

// TestCNodeMemoSlotAdaptiveGrowFiresExactlyAtThrashThreshold is a mechanism-
// level (no real parse, no wall clock) proof of the W2 adaptive growth
// trigger (issue #380/#388): cNodeMemoSlot must grow the cache from
// cNodeMemoCacheInitialSize to cNodeMemoCacheSize on the Nth genuine 2-way-set
// eviction, where N == cNodeMemoThrashGrowThreshold, and nowhere earlier --
// this is what makes the growth decision a deterministic function of THIS
// parse's own observed collision count rather than of wall-clock timing or
// unrelated prior history.
func TestCNodeMemoSlotAdaptiveGrowFiresExactlyAtThrashThreshold(t *testing.T) {
	p := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheInitialSize)}
	p.beginCNodeMemoEpoch()
	a, b, c := collidingCNodeMemoNodes(t, len(p.cNodeMemoCache)>>1)

	// a: fresh slot, no eviction. b: collides with a -> exactly one eviction.
	p.cNodeMemoSlot(a)
	p.cNodeMemoSlot(b)
	if got := p.cNodeMemoThrash; got != 1 {
		t.Fatalf("thrash after one collision = %d, want 1", got)
	}
	if got := len(p.cNodeMemoCache); got != cNodeMemoCacheInitialSize {
		t.Fatalf("cache grew after only 1/%d evictions: len=%d", cNodeMemoThrashGrowThreshold, got)
	}

	// Fast-forward to one eviction short of the threshold: the mechanism
	// under test is "does the Nth eviction trigger growth", not "how many
	// calls does it take to manufacture N collisions" (already covered,
	// cheaply, by the one real collision above).
	p.cNodeMemoThrash = cNodeMemoThrashGrowThreshold - 1

	// c collides with the same set again: this is the threshold-crossing
	// eviction. It must grow the cache AND reset the counter.
	slot := p.cNodeMemoSlot(c)
	if got := len(p.cNodeMemoCache); got != cNodeMemoCacheSize {
		t.Fatalf("cache len at threshold crossing = %d, want cNodeMemoCacheSize (%d)", got, cNodeMemoCacheSize)
	}
	if got := p.cNodeMemoThrash; got != 0 {
		t.Fatalf("thrash counter after grow = %d, want reset to 0", got)
	}
	// The returned slot must belong to the freshly-grown cache (not a
	// dangling pointer into the discarded small array) and must be correctly
	// addressed for c under the new cache's larger set count.
	newSetCount := len(p.cNodeMemoCache) >> 1
	wantIdx := cNodeMemoCacheIndex(uintptr(unsafe.Pointer(c)), newSetCount)
	if got := slot; got != &p.cNodeMemoCache[wantIdx] {
		t.Fatalf("slot after grow = %p, want &cache[%d] (%p)", got, wantIdx, &p.cNodeMemoCache[wantIdx])
	}
	if slot.node != uintptr(unsafe.Pointer(c)) || slot.epoch != p.cNodeMemoEpoch {
		t.Fatalf("slot after grow = %#v, want a fresh entry for c at the current epoch", *slot)
	}
}

// TestCNodeMemoSlotAdaptiveGrowIsNoopOnceAtFullSize confirms growth-crossing
// bookkeeping stays a no-op once the cache is already at cNodeMemoCacheSize
// (e.g. because cHandleError's unconditional trigger already grew it this
// parse): no further reallocation, no infinite-loop risk from repeatedly
// "crossing" the threshold every eviction once len>=cNodeMemoCacheSize.
func TestCNodeMemoSlotAdaptiveGrowIsNoopOnceAtFullSize(t *testing.T) {
	p := &Parser{cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)}
	p.beginCNodeMemoEpoch()
	a, b, c := collidingCNodeMemoNodes(t, len(p.cNodeMemoCache)>>1)
	p.cNodeMemoSlot(a)
	p.cNodeMemoSlot(b)
	p.cNodeMemoThrash = cNodeMemoThrashGrowThreshold
	before := &p.cNodeMemoCache[0]
	p.cNodeMemoSlot(c)
	if got := len(p.cNodeMemoCache); got != cNodeMemoCacheSize {
		t.Fatalf("cache len changed at full size = %d, want unchanged %d", got, cNodeMemoCacheSize)
	}
	if after := &p.cNodeMemoCache[0]; after != before {
		t.Fatal("cache backing array was reallocated at full size")
	}
}

func TestCNodeMemoSlotGrowsTemporaryRecoveryTier(t *testing.T) {
	p := &Parser{
		cNodeMemoCache:             make([]cNodeMemoCacheEntry, cNodeMemoCacheSize),
		crecoveryEnteredErrorState: true,
		cNodeMemoPeakTier:          RecoveryNodeMemoTierStandard,
	}
	p.beginCNodeMemoEpoch()
	a, b, c := collidingCNodeMemoNodes(t, len(p.cNodeMemoCache)>>1)
	p.cNodeMemoSlot(a)
	p.cNodeMemoSlot(b)
	p.cNodeMemoThrash = cNodeMemoRecoveryThrashGrowThreshold - 1
	retained := &p.cNodeMemoCache[0]

	slot := p.cNodeMemoSlot(c)
	if got := len(p.cNodeMemoCache); got != cNodeMemoRecoveryCacheSize {
		t.Fatalf("temporary cache length = %d, want %d", got, cNodeMemoRecoveryCacheSize)
	}
	if got := len(p.forestDeclineMemo.cNodeMemoRetainedCache); got != cNodeMemoCacheSize {
		t.Fatalf("retained cache length = %d, want %d", got, cNodeMemoCacheSize)
	}
	if got := &p.forestDeclineMemo.cNodeMemoRetainedCache[0]; got != retained {
		t.Fatal("temporary growth did not preserve the standard cache")
	}
	if got := p.cNodeMemoPeakTier.Entries(); got != cNodeMemoRecoveryCacheSize {
		t.Fatalf("peak entries = %d, want %d", got, cNodeMemoRecoveryCacheSize)
	}
	if got := p.cNodeMemoCollisionCount(); got != 2 {
		t.Fatalf("collision count = %d, want 2", got)
	}
	setCount := len(p.cNodeMemoCache) >> 1
	wantIdx := cNodeMemoCacheIndex(uintptr(unsafe.Pointer(c)), setCount)
	if slot != &p.cNodeMemoCache[wantIdx] {
		t.Fatalf("slot after temporary growth = %p, want cache[%d]", slot, wantIdx)
	}

	p.finishCNodeMemoParse()
	if got := len(p.cNodeMemoCache); got != cNodeMemoCacheSize {
		t.Fatalf("finished cache length = %d, want %d", got, cNodeMemoCacheSize)
	}
	if got := &p.cNodeMemoCache[0]; got != retained {
		t.Fatal("parse cleanup did not restore the standard cache")
	}
	if p.forestDeclineMemo.cNodeMemoRetainedCache != nil {
		t.Fatal("parse cleanup retained the temporary cache reference")
	}
}

func TestCNodeMemoSlotDoesNotGrowTemporaryTierOutsideRecovery(t *testing.T) {
	p := &Parser{
		cNodeMemoCache:    make([]cNodeMemoCacheEntry, cNodeMemoCacheSize),
		cNodeMemoPeakTier: RecoveryNodeMemoTierStandard,
	}
	p.beginCNodeMemoEpoch()
	a, b, c := collidingCNodeMemoNodes(t, len(p.cNodeMemoCache)>>1)
	p.cNodeMemoSlot(a)
	p.cNodeMemoSlot(b)
	p.cNodeMemoThrash = cNodeMemoRecoveryThrashGrowThreshold
	p.cNodeMemoSlot(c)

	if got := len(p.cNodeMemoCache); got != cNodeMemoCacheSize {
		t.Fatalf("cache length = %d, want %d without recovery", got, cNodeMemoCacheSize)
	}
	if p.forestDeclineMemo != nil && p.forestDeclineMemo.cNodeMemoRetainedCache != nil {
		t.Fatal("non-recovery contention retained a temporary cache")
	}
	if got := p.cNodeMemoPeakTier.Entries(); got != cNodeMemoCacheSize {
		t.Fatalf("peak entries = %d, want %d", got, cNodeMemoCacheSize)
	}
}

func TestCNodeMemoTemporaryTierLivesForParseOperation(t *testing.T) {
	p := &Parser{
		cNodeMemoCache:    make([]cNodeMemoCacheEntry, cNodeMemoCacheSize),
		cNodeMemoPeakTier: RecoveryNodeMemoTierStandard,
	}
	retained := &p.cNodeMemoCache[0]
	outer := p.beginParseOperationBudget()
	inner := p.beginParseOperationBudget()
	p.growCNodeMemoCacheTo(cNodeMemoRecoveryCacheSize)
	if got := len(p.cNodeMemoCache); got != cNodeMemoRecoveryCacheSize {
		t.Fatalf("active cache length = %d, want %d", got, cNodeMemoRecoveryCacheSize)
	}
	p.endParseOperationBudget(inner)
	if got := len(p.cNodeMemoCache); got != cNodeMemoRecoveryCacheSize {
		t.Fatalf("cache length after nested operation = %d, want %d", got, cNodeMemoRecoveryCacheSize)
	}
	if p.cNodeMemoOperationDepth != 1 {
		t.Fatalf("nested operation depth = %d, want 1", p.cNodeMemoOperationDepth)
	}
	p.endParseOperationBudget(outer)

	if p.cNodeMemoOperationDepth != 0 {
		t.Fatalf("operation depth = %d, want 0", p.cNodeMemoOperationDepth)
	}
	if got := len(p.cNodeMemoCache); got != cNodeMemoCacheSize {
		t.Fatalf("finished cache length = %d, want %d", got, cNodeMemoCacheSize)
	}
	if got := &p.cNodeMemoCache[0]; got != retained {
		t.Fatal("operation cleanup did not restore the standard cache")
	}
	peak, peakBytes, collisions := p.DebugCNodeMemoOperationStats()
	if peak != cNodeMemoRecoveryCacheSize {
		t.Fatalf("operation peak = %d, want %d", peak, cNodeMemoRecoveryCacheSize)
	}
	if want := cNodeMemoCacheBytesForEntries(cNodeMemoRecoveryCacheSize); peakBytes != want {
		t.Fatalf("operation peak bytes = %d, want %d", peakBytes, want)
	}
	if collisions != 0 {
		t.Fatalf("operation collisions = %d, want 0", collisions)
	}
}

func TestCNodeMemoTemporaryTierKeepsShortRetainedSlab(t *testing.T) {
	p := &Parser{
		cNodeMemoCache:    make([]cNodeMemoCacheEntry, cNodeMemoCacheInitialSize),
		cNodeMemoPeakTier: RecoveryNodeMemoTierInitial,
	}
	retained := &p.cNodeMemoCache[0]
	operation := p.beginParseOperationBudget()
	p.growCNodeMemoCacheTo(cNodeMemoRecoveryCacheSize)
	p.endParseOperationBudget(operation)

	if got := len(p.cNodeMemoCache); got != cNodeMemoCacheInitialSize {
		t.Fatalf("finished cache length = %d, want %d", got, cNodeMemoCacheInitialSize)
	}
	if got := &p.cNodeMemoCache[0]; got != retained {
		t.Fatal("operation cleanup did not restore the short cache")
	}
}

// electionTraceEntry is the frozen strategy-1 attempt surface. It records
// first ownership before any position, merge, cost, action, or recovery guard
// runs, so a future cursor can be checked without duplicating those guards.
type electionTraceEntry struct {
	EntryDepth   int
	RecoverDepth int
	State        StateID
	OwnerIndex   int
	OwnerOrder   uint32
	PosBytes     uint32
	PosRow       uint32
}

// frozenCRecoveryStrategy1AttemptTrace preserves the pre-linearization strategy-1
// collection, stable member ordering, depth-major walk, and pre-guard global
// dedupe. Keep this test-only reference independent of any future production
// cursor. It deliberately contains no position/merge/cost/action/recovery
// logic.
func frozenCRecoveryStrategy1AttemptTrace(p *Parser, stacks []glrStack, group *cRecGroup) []electionTraceEntry {
	members := make([]int, 0, cRecoverMaxVersionCount)
	for i := range stacks {
		if !stacks[i].dead && stacks[i].cRec != nil && stacks[i].cRec.group == group {
			members = append(members, i)
		}
	}
	if len(members) == 0 {
		return nil
	}

	// Frozen stable insertion sort: equal groupOrder values retain physical
	// stack-slice order.
	for i := 1; i < len(members); i++ {
		cur := members[i]
		curOrder := stacks[cur].cRec.groupOrder
		j := i - 1
		for ; j >= 0; j-- {
			prev := members[j]
			if stacks[prev].cRec.groupOrder <= curOrder {
				break
			}
			members[j+1] = prev
		}
		members[j+1] = cur
	}

	depthBump := 0
	m0 := members[0]
	if p.cStackCumulativeNodeCount(&stacks[m0]) > int(stacks[m0].cNodeBaseline) {
		depthBump = 1
	}
	type seenKey struct {
		depth int
		state StateID
	}
	seen := make(map[seenKey]struct{}, 16)
	var trace []electionTraceEntry
	for depth := 0; depth <= cRecoverMaxSummaryDepth+1; depth++ {
		for _, member := range members {
			rec := stacks[member].cRec
			for _, entry := range rec.summary {
				if int(entry.depth) != depth || entry.state == cErrorState {
					continue
				}
				key := seenKey{depth: int(entry.depth), state: entry.state}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				trace = append(trace, electionTraceEntry{
					EntryDepth:   int(entry.depth),
					RecoverDepth: int(entry.depth) + depthBump,
					State:        entry.state,
					OwnerIndex:   member,
					OwnerOrder:   rec.groupOrder,
					PosBytes:     entry.posBytes,
					PosRow:       entry.posRow,
				})
			}
		}
	}
	return trace
}

// linearCRecoveryStrategy1AttemptTrace exposes only the cursor's attempt
// surface. Position, merge, cost, action, and recovery guards remain outside
// this oracle so ordering and first ownership are tested independently.
func linearCRecoveryStrategy1AttemptTrace(p *Parser, stacks []glrStack, group *cRecGroup) []electionTraceEntry {
	stateCount := 1
	for i := range stacks {
		if stacks[i].cRec == nil {
			continue
		}
		for _, entry := range stacks[i].cRec.summary {
			if entry.state != cErrorState && int(entry.state)+1 > stateCount {
				stateCount = int(entry.state) + 1
			}
		}
	}
	var scratch cRecoverElectionScratch
	scratch.prepare(len(stacks), stateCount)
	for i := range stacks {
		if !stacks[i].dead && stacks[i].cRec != nil && stacks[i].cRec.group == group {
			scratch.members = append(scratch.members, i)
		}
	}
	if len(scratch.members) == 0 {
		return nil
	}
	scratch.prepareMemberCursors(len(scratch.members))
	cSortRecoverMembersByGroupOrder(stacks, scratch.members)

	depthBump := 0
	m0 := scratch.members[0]
	if p.cStackCumulativeNodeCount(&stacks[m0]) > int(stacks[m0].cNodeBaseline) {
		depthBump = 1
	}
	var trace []electionTraceEntry
	for depth := 0; depth <= cRecoverMaxSummaryDepth+1; depth++ {
		iter := scratch.beginDepth(depth)
		for {
			member, entry, status := iter.next(stacks)
			switch status {
			case cRecoverElectionIterExhausted:
				goto nextDepth
			case cRecoverElectionIterPoll:
				continue
			}
			rec := stacks[member].cRec
			trace = append(trace, electionTraceEntry{
				EntryDepth: int(entry.depth), RecoverDepth: int(entry.depth) + depthBump,
				State: entry.state, OwnerIndex: member, OwnerOrder: rec.groupOrder,
				PosBytes: entry.posBytes, PosRow: entry.posRow,
			})
		}
	nextDepth:
	}
	return trace
}

type frozenElectionMemberSpec struct {
	id    int
	order uint32
	dead  bool
}

func TestCRecoverElectionDepthIteratorMatchesFrozenOrdering(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	for memberCount := 1; memberCount <= 8; memberCount++ {
		permutations := [][]int{
			frozenIdentityPermutation(memberCount),
			frozenReversePermutation(memberCount),
			frozenRotatePermutation(memberCount),
		}
		deadMasks := []uint16{0, 1, 0x5555, uint16(1 << (memberCount - 1)), uint16((1 << memberCount) - 1)}
		for permutationIndex, permutation := range permutations {
			for _, deadMask := range deadMasks {
				name := frozenElectionFixtureName(memberCount, permutationIndex, deadMask)
				t.Run(name, func(t *testing.T) {
					group := &cRecGroup{}
					foreign := &cRecGroup{}
					stacks := make([]glrStack, 0, memberCount+1)
					specs := make([]frozenElectionMemberSpec, 0, memberCount)
					for _, id := range permutation {
						spec := frozenElectionMemberSpec{
							id:    id,
							order: uint32(id / 2), // equal-order ties are intentional.
							dead:  deadMask&(1<<id) != 0,
						}
						specs = append(specs, spec)
						stack := cRecoveryElectionStack(arena, 2, 3, uint32(500+id), group, int(spec.order), frozenElectionSummary(id))
						stack.dead = spec.dead
						stacks = append(stacks, stack)
					}
					// A lower-ordered foreign member must never enter this election.
					stacks = append(stacks, cRecoveryElectionStack(arena, 2, 3, 999, foreign, 0, []cStackSummaryEntry{{depth: 0, state: 999}}))

					got := frozenCRecoveryStrategy1AttemptTrace(parser, stacks, group)
					want := frozenElectionExpectedTrace(specs)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("trace mismatch\n got: %#v\nwant: %#v", got, want)
					}
					linear := linearCRecoveryStrategy1AttemptTrace(parser, stacks, group)
					if !reflect.DeepEqual(linear, got) {
						t.Fatalf("linear trace mismatch\n got: %#v\nwant: %#v", linear, got)
					}
				})
			}
		}
	}
}

func TestFrozenCRecoveryStrategy1AttemptTraceDepthBumpUsesSelectedM0(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	newMember := func(group *cRecGroup, order int, state StateID, baseline uint32) glrStack {
		stack := cRecoveryElectionStack(arena, 2, 3, 10, group, order, []cStackSummaryEntry{{depth: 2, state: state}})
		stack.cNodeBaseline = baseline
		return stack
	}

	t.Run("selected_m0_positive", func(t *testing.T) {
		group := &cRecGroup{}
		stacks := []glrStack{newMember(group, 0, 11, 0), newMember(group, 1, 12, 1)}
		trace := linearCRecoveryStrategy1AttemptTrace(parser, stacks, group)
		for i, entry := range trace {
			if entry.RecoverDepth != entry.EntryDepth+1 {
				t.Fatalf("trace[%d] recover depth = %d, want entry depth %d + 1", i, entry.RecoverDepth, entry.EntryDepth)
			}
		}
	})

	t.Run("non_m0_positive_does_not_bump", func(t *testing.T) {
		group := &cRecGroup{}
		stacks := []glrStack{newMember(group, 0, 11, 1), newMember(group, 1, 12, 0)}
		trace := linearCRecoveryStrategy1AttemptTrace(parser, stacks, group)
		for i, entry := range trace {
			if entry.RecoverDepth != entry.EntryDepth {
				t.Fatalf("trace[%d] recover depth = %d, want unbumped %d", i, entry.RecoverDepth, entry.EntryDepth)
			}
		}
	})

	t.Run("sort_and_death_change_m0", func(t *testing.T) {
		group := &cRecGroup{}
		positivePhysicalFirst := newMember(group, 2, 21, 0)
		zeroPhysicalSecond := newMember(group, 1, 22, 1)
		stacks := []glrStack{positivePhysicalFirst, zeroPhysicalSecond}

		trace := linearCRecoveryStrategy1AttemptTrace(parser, stacks, group)
		if len(trace) != 2 || trace[0].OwnerIndex != 1 {
			t.Fatalf("sorted trace = %#v, want physical member 1 selected as m0", trace)
		}
		for _, entry := range trace {
			if entry.RecoverDepth != entry.EntryDepth {
				t.Fatalf("live lower-order m0 unexpectedly bumped trace: %#v", trace)
			}
		}

		stacks[1].dead = true
		trace = frozenCRecoveryStrategy1AttemptTrace(parser, stacks, group)
		if len(trace) != 1 || trace[0].OwnerIndex != 0 || trace[0].RecoverDepth != trace[0].EntryDepth+1 {
			t.Fatalf("trace after lower-order death = %#v, want positive physical member 0 as bumped m0", trace)
		}
	})
}

func TestCRecoverStrategy1ElectionDeadFirstOwnerFallsBackWithGSSScratch(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	entry := cStackSummaryEntry{depth: 1, state: 2, posBytes: 0}
	stacks := []glrStack{
		cRecoveryElectionStack(arena, 4, 3, 5, group, 0, []cStackSummaryEntry{entry}),
		cRecoveryElectionStack(arena, 2, 3, 5, group, 1, []cStackSummaryEntry{entry}),
	}
	stacks[0].dead = true
	stacks[0].branchOrder = 11
	stacks[1].branchOrder = 22
	trace := linearCRecoveryStrategy1AttemptTrace(parser, stacks, group)
	if len(trace) != 1 || trace[0].OwnerIndex != 1 {
		t.Fatalf("attempt trace = %#v, want next live duplicate owner 1", trace)
	}

	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	nodeCount := 0
	didRecover, forked, reason := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, &entryScratch, &gssScratch, nil)
	if reason != ParseStopNone || !didRecover || !forked {
		t.Fatalf("election = recovered %v forked %v reason %v, want true/true/none", didRecover, forked, reason)
	}
	if len(stacks) != 3 || stacks[2].branchOrder != 22 || stacks[2].top().state != StateID(2) {
		t.Fatalf("recovered fork = len %d branch %d top %d, want 3/22/2", len(stacks), stacks[len(stacks)-1].branchOrder, stacks[len(stacks)-1].top().state)
	}
	if gssScratch.usedTotal == 0 || stacks[1].gss.head == nil {
		t.Fatal("election did not exercise the supplied non-nil GSS scratch")
	}
}

func TestCRecoverStrategy1ElectionCostAbortsBeforeNoActionLookup(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	build := func(includeExpensiveNoAction bool) ([]glrStack, *cRecGroup) {
		group := &cRecGroup{}
		summary := make([]cStackSummaryEntry, 0, 2)
		if includeExpensiveNoAction {
			summary = append(summary, cStackSummaryEntry{depth: 0, state: 1, posBytes: 0})
		}
		summary = append(summary, cStackSummaryEntry{depth: 1, state: 2, posBytes: 2490})
		return []glrStack{
			cRecoveryElectionStack(arena, 2, 3, 2500, group, 0, summary),
			cRecoveryElectionPlainStack(arena, 4, 4, 2500),
		}, group
	}

	stacks, group := build(true)
	if parser.lookupActionIndex(1, 1) != 0 {
		t.Fatal("state 1 unexpectedly has a token action; fixture would not prove cost-before-action")
	}
	for i := range stacks {
		if !stacks[i].dead && stacks[i].top().state == 1 && stacks[i].byteOffset == 2500 {
			t.Fatal("state 1 candidate would merge; fixture would not reach its cost check")
		}
	}
	nodeCount := 0
	didRecover, forked, reason := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, nil, nil)
	if reason != ParseStopNone || didRecover || forked || len(stacks) != 2 {
		t.Fatalf("election with expensive no-action entry = recovered %v forked %v reason %v len %d, want false/false/none/2", didRecover, forked, reason, len(stacks))
	}

	// Without the no-action entry, the later actionable candidate succeeds.
	// Therefore the first run stopped at that entry's pre-action cost check.
	control, controlGroup := build(false)
	nodeCount = 0
	didRecover, forked, reason = parser.cRecoverStrategy1Election(&control, controlGroup, nil, Token{Symbol: 1}, &nodeCount, arena, nil, nil, nil)
	if reason != ParseStopNone || !didRecover || !forked || len(control) != 3 {
		t.Fatalf("control election = recovered %v forked %v reason %v len %d, want true/true/none/3", didRecover, forked, reason, len(control))
	}
}

func TestCRecoverStrategy1ElectionFailedRecoveryDoesNotRetryDuplicate(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	entry := cStackSummaryEntry{depth: 1, state: 2, posBytes: 0}
	stacks := []glrStack{
		// This live first owner advertises state 2, but its actual depth-1
		// state is 4, forcing cRecoverToState to fail.
		cRecoveryElectionStack(arena, 4, 3, 5, group, 0, []cStackSummaryEntry{entry}),
		// A duplicate retry here would succeed, but first ownership forbids it.
		cRecoveryElectionStack(arena, 2, 3, 5, group, 1, []cStackSummaryEntry{entry}),
	}
	trace := linearCRecoveryStrategy1AttemptTrace(parser, stacks, group)
	if len(trace) != 1 || trace[0].OwnerIndex != 0 {
		t.Fatalf("attempt trace = %#v, want live first owner 0 only", trace)
	}

	var gssScratch gssScratch
	nodeCount := 0
	didRecover, forked, reason := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, &gssScratch, nil)
	if reason != ParseStopNone || didRecover || forked || len(stacks) != 2 {
		t.Fatalf("election = recovered %v forked %v reason %v len %d, want false/false/none/2", didRecover, forked, reason, len(stacks))
	}
	if gssScratch.usedTotal == 0 || stacks[0].gss.head == nil {
		t.Fatal("failed recovery did not exercise the supplied non-nil GSS scratch")
	}
}

func TestCRecoverStrategy1ElectionPositionFailureDoesNotRetryDuplicate(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	build := func(firstDead bool) ([]glrStack, *cRecGroup) {
		group := &cRecGroup{}
		stacks := []glrStack{
			// The first live owner fails the position guard because the entry
			// position equals the merged-version position.
			cRecoveryElectionStack(arena, 4, 3, 5, group, 0, []cStackSummaryEntry{{depth: 1, state: 2, posBytes: 5}}),
			// The duplicate has a recoverable position and a matching depth-1
			// stack state. Pre-guard ownership must still suppress its retry.
			cRecoveryElectionStack(arena, 2, 3, 5, group, 1, []cStackSummaryEntry{{depth: 1, state: 2, posBytes: 0}}),
		}
		stacks[0].dead = firstDead
		return stacks, group
	}

	stacks, group := build(false)
	trace := linearCRecoveryStrategy1AttemptTrace(parser, stacks, group)
	if len(trace) != 1 || trace[0].OwnerIndex != 0 || trace[0].PosBytes != 5 {
		t.Fatalf("attempt trace = %#v, want position-failing live owner 0 only", trace)
	}
	nodeCount := 0
	didRecover, forked, reason := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, nil, nil, nil)
	if reason != ParseStopNone || didRecover || forked || len(stacks) != 2 {
		t.Fatalf("live-owner election = recovered %v forked %v reason %v len %d, want false/false/none/2", didRecover, forked, reason, len(stacks))
	}

	// Killing the first owner makes the second duplicate the first live owner;
	// it must then recover. This proves the previous no-fork result came from
	// no-fallback ownership, not an independently unrecoverable duplicate.
	control, controlGroup := build(true)
	nodeCount = 0
	didRecover, forked, reason = parser.cRecoverStrategy1Election(&control, controlGroup, nil, Token{Symbol: 1}, &nodeCount, arena, nil, nil, nil)
	if reason != ParseStopNone || !didRecover || !forked || len(control) != 3 || control[2].top().state != StateID(2) {
		t.Fatalf("dead-owner control = recovered %v forked %v reason %v len %d top %d, want true/true/none/3/2", didRecover, forked, reason, len(control), control[len(control)-1].top().state)
	}
}

func cloneRecoveryElectionStacksForSnapshot(stacks []glrStack) []glrStack {
	cloned := make([]glrStack, len(stacks))
	for i := range stacks {
		cloned[i] = stacks[i]
		cloned[i].entries = append([]stackEntry(nil), stacks[i].entries...)
		if stacks[i].cRec != nil {
			rec := *stacks[i].cRec
			rec.summary = append([]cStackSummaryEntry(nil), stacks[i].cRec.summary...)
			cloned[i].cRec = &rec
		}
	}
	return cloned
}

func TestCRecoverStrategy1ElectionPreexistingCancellationDoesNotMutateParseState(t *testing.T) {
	parser := cRecoveryElectionTestParser()
	cancelled := uint32(1)
	parser.SetCancellationFlag(&cancelled)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	group := &cRecGroup{}
	stacks := []glrStack{cRecoveryElectionStack(arena, 2, 3, 5, group, 0, []cStackSummaryEntry{{depth: 1, state: 2, posBytes: 0}})}
	stackBefore := cloneRecoveryElectionStacksForSnapshot(stacks)
	groupBefore := *group
	node := stackEntryNode(stacks[0].top())
	nodeBefore := *node
	arenaUsedBefore := arena.used
	arenaBytesBefore := arena.allocatedBytes
	arenaCursorBefore := arena.nodeSlabCursor
	nodeCount := 73
	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	entryScratchBefore := entryScratch
	gssScratchBefore := gssScratch

	didRecover, forked, reason := parser.cRecoverStrategy1Election(&stacks, group, nil, Token{Symbol: 1}, &nodeCount, arena, &entryScratch, &gssScratch, nil)
	if reason != ParseStopCancelled || didRecover || forked {
		t.Fatalf("cancelled election = recovered %v forked %v reason %v, want false/false/cancelled", didRecover, forked, reason)
	}
	if !reflect.DeepEqual(stacks, stackBefore) || !reflect.DeepEqual(*group, groupBefore) {
		t.Fatalf("cancelled election mutated stacks/group\n got stacks: %#v\nwant stacks: %#v", stacks, stackBefore)
	}
	if !reflect.DeepEqual(*node, nodeBefore) {
		t.Fatal("cancelled election mutated the existing stack node")
	}
	if arena.used != arenaUsedBefore || arena.allocatedBytes != arenaBytesBefore || arena.nodeSlabCursor != arenaCursorBefore {
		t.Fatalf("cancelled election mutated arena: used %d->%d bytes %d->%d cursor %d->%d", arenaUsedBefore, arena.used, arenaBytesBefore, arena.allocatedBytes, arenaCursorBefore, arena.nodeSlabCursor)
	}
	if nodeCount != 73 {
		t.Fatalf("cancelled election mutated node count: got %d, want 73", nodeCount)
	}
	if !reflect.DeepEqual(entryScratch, entryScratchBefore) || !reflect.DeepEqual(gssScratch, gssScratchBefore) {
		t.Fatalf("cancelled election mutated zero scratch\nentry got: %#v\nentry want: %#v\ngss got: %#v\ngss want: %#v", entryScratch, entryScratchBefore, gssScratch, gssScratchBefore)
	}
}

func frozenElectionSummary(id int) []cStackSummaryEntry {
	base := uint32(id * 100)
	return []cStackSummaryEntry{
		{depth: 0, state: 50, posBytes: base + 1, posRow: uint32(id)}, // across-member duplicate
		{depth: 0, state: StateID(100 + id), posBytes: base + 2, posRow: uint32(id)},
		{depth: 0, state: StateID(110 + id), posBytes: base + 3, posRow: uint32(id)}, // same-depth sibling
		{depth: 0, state: cErrorState, posBytes: base + 4, posRow: uint32(id)},
		{depth: 0, state: 50, posBytes: base + 5, posRow: uint32(id)},  // within-member duplicate
		{depth: 16, state: 50, posBytes: base + 6, posRow: uint32(id)}, // same state, different depth
		{depth: 16, state: StateID(200 + id), posBytes: base + 7, posRow: uint32(id)},
		{depth: 17, state: StateID(300 + id), posBytes: base + 8, posRow: uint32(id)},
		{depth: 18, state: StateID(400 + id), posBytes: base + 9, posRow: uint32(id)}, // ignored
	}
}

func frozenElectionExpectedTrace(specs []frozenElectionMemberSpec) []electionTraceEntry {
	ordered := make([]int, 0, len(specs))
	for order := uint32(0); order <= 3; order++ {
		for physicalIndex := range specs {
			if !specs[physicalIndex].dead && specs[physicalIndex].order == order {
				ordered = append(ordered, physicalIndex)
			}
		}
	}
	if len(ordered) == 0 {
		return nil
	}
	var want []electionTraceEntry
	appendEntry := func(physicalIndex, depth int, state StateID, offset uint32) {
		spec := specs[physicalIndex]
		want = append(want, electionTraceEntry{
			EntryDepth: depth, RecoverDepth: depth, State: state,
			OwnerIndex: physicalIndex, OwnerOrder: spec.order,
			PosBytes: uint32(spec.id*100) + offset, PosRow: uint32(spec.id),
		})
	}
	// At depth zero, the first live member owns the shared key. Every live
	// member then owns its two unique same-depth states in stable order.
	appendEntry(ordered[0], 0, 50, 1)
	for _, physicalIndex := range ordered {
		id := specs[physicalIndex].id
		appendEntry(physicalIndex, 0, StateID(100+id), 2)
		appendEntry(physicalIndex, 0, StateID(110+id), 3)
	}
	// Dedupe includes depth, so state 50 is eligible again at depth 16.
	appendEntry(ordered[0], 16, 50, 6)
	for _, physicalIndex := range ordered {
		id := specs[physicalIndex].id
		appendEntry(physicalIndex, 16, StateID(200+id), 7)
	}
	for _, physicalIndex := range ordered {
		id := specs[physicalIndex].id
		appendEntry(physicalIndex, 17, StateID(300+id), 8)
	}
	return want
}

func frozenIdentityPermutation(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func frozenReversePermutation(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = n - 1 - i
	}
	return out
}

func frozenRotatePermutation(n int) []int {
	out := make([]int, n)
	if n == 0 {
		return out
	}
	for i := range out {
		out[i] = (i + n/2) % n
	}
	return out
}

func frozenElectionFixtureName(memberCount, permutation int, deadMask uint16) string {
	return fmt.Sprintf("members=%d/permutation=%d/dead=%x", memberCount, permutation, deadMask)
}

func TestCRecordSummaryDepthsAreMonotonic(t *testing.T) {
	parser := &Parser{}
	var summaries, comparisons, equalDepths, increasedDepths int
	var errorEntries, nodelessEntries, nodeEntries, extraEntries int
	maxDepth := 0
	for fixture := uint32(1); fixture <= 128; fixture++ {
		entries := make([]stackEntry, 48)
		nodes := make([]Node, len(entries))
		state := fixture*747796405 + 2891336453
		for i := range entries {
			state = state*1664525 + 1013904223
			entryState := StateID(state%31 + 1)
			switch state % 4 {
			case 0:
				entries[i] = stackEntry{state: cErrorState}
				errorEntries++
			case 1:
				entries[i] = stackEntry{state: entryState}
				nodelessEntries++
			default:
				nodes[i].symbol = 1
				if state&8 != 0 {
					nodes[i].setExtra(true)
					extraEntries++
				}
				entries[i] = newStackEntryNode(entryState, &nodes[i])
				nodeEntries++
			}
		}
		summary := parser.cRecordSummary(entries)
		if len(summary) > 0 {
			summaries++
			if int(summary[len(summary)-1].depth) > maxDepth {
				maxDepth = int(summary[len(summary)-1].depth)
			}
		}
		for i := 1; i < len(summary); i++ {
			comparisons++
			if int(summary[i].depth) < int(summary[i-1].depth) {
				t.Fatalf("fixture %d summary depth fell at %d: %d -> %d", fixture, i, summary[i-1].depth, summary[i].depth)
			}
			if summary[i].depth == summary[i-1].depth {
				equalDepths++
			} else {
				increasedDepths++
			}
		}
	}
	if summaries != 128 || comparisons == 0 || equalDepths == 0 || increasedDepths == 0 || maxDepth < cRecoverMaxSummaryDepth {
		t.Fatalf("vacuous summary coverage: summaries=%d comparisons=%d equal=%d increased=%d max-depth=%d", summaries, comparisons, equalDepths, increasedDepths, maxDepth)
	}
	if errorEntries == 0 || nodelessEntries == 0 || nodeEntries == 0 || extraEntries == 0 {
		t.Fatalf("vacuous entry coverage: error=%d nodeless=%d nodes=%d extras=%d", errorEntries, nodelessEntries, nodeEntries, extraEntries)
	}
}

func TestCAcceptRootRebuildPreservesCandidateFields(t *testing.T) {
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "root", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "item", Visible: true, Named: true},
		},
	}
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 2, true, 0, 2, Point{}, Point{Column: 2})
	comment.setExtra(true)
	first := newLeafNodeInArena(arena, 3, true, 3, 4, Point{Column: 3}, Point{Column: 4})
	second := newLeafNodeInArena(arena, 3, true, 5, 6, Point{Column: 5}, Point{Column: 6})
	trailing := newLeafNodeInArena(arena, 2, true, 7, 9, Point{Column: 7}, Point{Column: 9})
	trailing.setExtra(true)
	candidate := newParentNodeInArenaWithFieldSources(
		arena,
		1,
		true,
		[]*Node{first, second},
		[]FieldID{7, 8},
		[]uint8{fieldSourceDirect, fieldSourceInherited},
		23,
	)
	stack := glrStack{
		accepted: true,
		entries: []stackEntry{
			{state: 1},
			newStackEntryNode(1, comment),
			newStackEntryNode(2, candidate),
			newStackEntryNode(3, trailing),
		},
	}
	var entries glrEntryScratch
	var gss gssScratch
	if reason := parser.cAcceptRootRebuild(&stack, arena, &entries, &gss); reason != ParseStopNone {
		t.Fatalf("accept rebuild stopped: %s", reason)
	}
	root := stackEntryNode(stack.top())
	if root == nil {
		t.Fatal("accept rebuild returned no root")
	}
	if root.productionID != candidate.productionID {
		t.Fatalf("root production = %d, want %d", root.productionID, candidate.productionID)
	}
	if got, want := root.fieldIDs(), []FieldID{0, 7, 8, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root fields = %v, want %v", got, want)
	}
	if got, want := root.fieldSources(), []uint8{0, fieldSourceDirect, fieldSourceInherited, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root field sources = %v, want %v", got, want)
	}
}

func recoverySummaryTestEntries(count int) []stackEntry {
	entries := make([]stackEntry, count)
	for i := range entries {
		entries[i] = stackEntry{state: StateID(i + 1)}
	}
	return entries
}

func runCHandleErrorForSummaryTest(t *testing.T, gss *gssScratch) *cRecoverState {
	t.Helper()
	parser := cRecoveryElectionTestParser()
	arena := acquireNodeArena(arenaClassFull)
	t.Cleanup(arena.Release)
	stacks := []glrStack{newGLRStack(2)}
	var entries glrEntryScratch
	trackChildErrors := false
	nodeCount := 0
	_, _, reason := parser.cHandleError(
		&stacks,
		0,
		[]byte("x"),
		Token{Symbol: 2, StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}},
		&nodeCount,
		arena,
		&entries,
		gss,
		nil,
		&trackChildErrors,
	)
	if reason != ParseStopNone {
		t.Fatalf("cHandleError stop reason = %s, want %s", reason, ParseStopNone)
	}
	for i := range stacks {
		if stacks[i].cRec != nil {
			return stacks[i].cRec
		}
	}
	t.Fatalf("cHandleError produced no recovery state: %#v", stacks)
	return nil
}

func forestEOFRecoveryTestParser() *Parser {
	parser := cRecoveryElectionTestParser()
	parser.language.ParseTable[2] = []uint16{1, 1}
	return parser
}

func TestCRecoverSummaryArenaSurvivesHandleErrorReturn(t *testing.T) {
	var scratch gssScratch
	first := runCHandleErrorForSummaryTest(t, &scratch)
	if len(first.summary) == 0 {
		t.Fatal("cHandleError recorded an empty summary")
	}
	want := first.summary[0]
	if first.summary[0] != want {
		t.Fatalf("summary changed before cHandleError returned: got=%v want=%v", first.summary[0], want)
	}
	if first.group == nil {
		t.Fatal("cHandleError did not attach a recovery group")
	}
}

func TestCRecoverSummaryArenaDoesNotReuseLiveGroupStorage(t *testing.T) {
	var scratch gssScratch
	first := runCHandleErrorForSummaryTest(t, &scratch)
	second := runCHandleErrorForSummaryTest(t, &scratch)
	if len(first.summary) == 0 || len(second.summary) == 0 {
		t.Fatal("cHandleError recorded an empty summary")
	}
	if &first.summary[0] == &second.summary[0] {
		t.Fatal("summary arena reused live group storage")
	}
	firstState := first.summary[0].state
	second.summary[0].state++
	if first.summary[0].state != firstState {
		t.Fatal("mutating a later summary changed live group storage")
	}
}

func TestCRecoverStateCloneSharesImmutableSummary(t *testing.T) {
	var scratch gssScratch
	original := runCHandleErrorForSummaryTest(t, &scratch)
	clone := original.clone()
	if clone == original {
		t.Fatal("clone returned the original recovery state")
	}
	if len(clone.summary) != len(original.summary) || &clone.summary[0] != &original.summary[0] {
		t.Fatal("recovery-state clone copied immutable summary storage")
	}
}

func TestForestEOFRecoverySummaryArenaLifetime(t *testing.T) {
	parser := forestEOFRecoveryTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	leaf := newLeafNodeInArena(arena, 1, true, 0, 3, Point{}, Point{Column: 3})
	base := &gssForestNode{state: 1, byteOffset: 0}
	node := &gssForestNode{
		state:      2,
		byteOffset: 4,
		links:      []gssLink{{prev: base, subtree: newStackEntryNode(2, leaf)}},
	}
	var index gssForestIndex
	index.init(1)
	index.set(gssForestKey{state: 2, byteOffset: 4}, node)
	competes, reason := parser.forestEOFRecoveryCouldCompete(&index, arena, 4, true)
	if reason != ParseStopNone {
		t.Fatalf("forest EOF recovery stop reason = %s, want %s", reason, ParseStopNone)
	}
	if !competes {
		t.Fatal("forest EOF recovery path did not evaluate a competing recovery")
	}
}

func TestForestEOFRecoverySummaryArenaBudgetStopsBeforeAllocation(t *testing.T) {
	parser := forestEOFRecoveryTestParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	arena.setBudget(1)
	leaf := newLeafNodeInArena(arena, 1, true, 0, 3, Point{}, Point{Column: 3})
	base := &gssForestNode{state: 1, byteOffset: 0}
	node := &gssForestNode{
		state:      2,
		byteOffset: 4,
		links:      []gssLink{{prev: base, subtree: newStackEntryNode(2, leaf)}},
	}
	var index gssForestIndex
	index.init(1)
	index.set(gssForestKey{state: 2, byteOffset: 4}, node)
	competes, reason := parser.forestEOFRecoveryCouldCompete(&index, arena, 4, true)
	if reason != ParseStopMemoryBudget {
		t.Fatalf("forest EOF budget reason = %s, want %s", reason, ParseStopMemoryBudget)
	}
	if competes {
		t.Fatal("forest EOF budget stop reported a recovery competitor")
	}
}

func TestCReductionCandidateDoesNotDoubleCloneRecoveryState(t *testing.T) {
	parser := newCRecoverySyntheticReduceParser()
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	var entryScratch glrEntryScratch
	var scratch gssScratch
	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	start := newGLRStack(1)
	start.pushEntry(newStackEntryNode(2, leaf), &entryScratch, &scratch)
	start.cRec = &cRecoverState{summary: []cStackSummaryEntry{{depth: 0, state: 1}}, group: &cRecGroup{}}
	cloneCalls := 0
	previousObserver := cRecoverStateCloneObserver
	cRecoverStateCloneObserver = func() { cloneCalls++ }
	defer func() { cRecoverStateCloneObserver = previousObserver }()
	nodeCount := 0
	candidates, reason := parser.cReductionCandidatesForAction(
		nil,
		start,
		ParseAction{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
		Token{},
		&nodeCount,
		arena,
		&entryScratch,
		&scratch,
		nil,
		nil,
	)
	if reason != ParseStopNone {
		t.Fatalf("reduction candidate stop reason = %s, want %s", reason, ParseStopNone)
	}
	if len(candidates) == 0 || candidates[0].cRec == nil {
		t.Fatal("reduction candidate lost its recovery state")
	}
	if cloneCalls != 1 {
		t.Fatalf("cReductionCandidatesForAction cloned recovery state %d times, want 1", cloneCalls)
	}
	if &candidates[0].cRec.summary[0] != &start.cRec.summary[0] {
		t.Fatal("reduction candidate did not preserve shared immutable recovery summary")
	}
}
