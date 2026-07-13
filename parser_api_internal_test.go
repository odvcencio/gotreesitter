package gotreesitter

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"
)

type parserTestUnsafeExternalScanner struct{}

func (parserTestUnsafeExternalScanner) Create() any                           { return nil }
func (parserTestUnsafeExternalScanner) Destroy(payload any)                   {}
func (parserTestUnsafeExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (parserTestUnsafeExternalScanner) Deserialize(payload any, buf []byte)   {}
func (parserTestUnsafeExternalScanner) Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	return false
}

type parserTestSafeExternalScanner struct {
	parserTestUnsafeExternalScanner
}

func (parserTestSafeExternalScanner) SupportsIncrementalReuse() bool { return true }

func TestRepetitionShiftConflictChoice(t *testing.T) {
	chosen, ok := repetitionShiftConflictChoice([]ParseAction{
		{Type: ParseActionReduce, Symbol: 191, ChildCount: 2},
		{Type: ParseActionShift, State: 1245, Repetition: true},
	})
	if !ok {
		t.Fatal("repetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 1245 || !chosen.Repetition {
		t.Fatalf("repetitionShiftConflictChoice picked %+v, want repetition shift", chosen)
	}
}

func TestRepetitionShiftConflictChoiceRejectsNonRepetitionShift(t *testing.T) {
	if _, ok := repetitionShiftConflictChoice([]ParseAction{
		{Type: ParseActionReduce, Symbol: 191, ChildCount: 2},
		{Type: ParseActionShift, State: 1245, Repetition: false},
	}); ok {
		t.Fatal("repetitionShiftConflictChoice = true, want false")
	}
}

func TestCSharpRepetitionShiftConflictChoice(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "this", "block_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 1245, Repetition: true},
	}

	chosen, ok := csharpRepetitionShiftConflictChoice(lang, Token{Symbol: 2, Text: "this"}, actions)
	if !ok {
		t.Fatal("csharpRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 1245 || !chosen.Repetition {
		t.Fatalf("csharpRepetitionShiftConflictChoice picked %+v, want repetition shift", chosen)
	}
}

func TestCSharpRepetitionShiftConflictChoiceRejectsScopedContextualIdentifier(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "this", "block_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 1245, Repetition: true},
	}

	if _, ok := csharpRepetitionShiftConflictChoice(lang, Token{Symbol: 1, Text: "scoped"}, actions); ok {
		t.Fatal("csharpRepetitionShiftConflictChoice = true, want false")
	}
}

func TestCSharpRepetitionShiftConflictChoiceAllowsDeclarationLists(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "private", "declaration_list_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 3237, Repetition: true},
	}

	chosen, ok := csharpRepetitionShiftConflictChoice(lang, Token{Symbol: 1, Text: "private"}, actions)
	if !ok {
		t.Fatal("csharpRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 3237 || !chosen.Repetition {
		t.Fatalf("csharpRepetitionShiftConflictChoice picked %+v, want declaration-list shift", chosen)
	}
}

func TestCSharpRepetitionShiftConflictChoiceRejectsOtherRepeats(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "this", "argument_list_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 1245, Repetition: true},
	}

	if _, ok := csharpRepetitionShiftConflictChoice(lang, Token{Symbol: 1, Text: "this"}, actions); ok {
		t.Fatal("csharpRepetitionShiftConflictChoice = true, want false")
	}
}

func TestGeneratedRepeatBoundaryConflictKeepsGLRForkWithoutPolicy(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state StateID
		tok   Symbol
	}{
		{name: "python", state: 72, tok: 1},
		{name: "dart", state: 596, tok: 1},
	} {
		lang := &Language{
			Name:                  tc.name,
			GeneratedByGrammargen: true,
			SymbolNames:           []string{"end", "identifier", "def", "module_repeat1"},
			SymbolMetadata: []SymbolMetadata{
				{Name: "end"},
				{Name: "identifier"},
				{Name: "def"},
				{Name: "module_repeat1", GeneratedRepeatAux: true},
			},
		}
		actions := []ParseAction{
			{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
			{Type: ParseActionShift, State: 616, Repetition: true},
		}
		if !generatedRepeatBoundaryConflict(lang, actions) {
			t.Fatalf("%s generatedRepeatBoundaryConflict = false, want true", tc.name)
		}
		parser := &Parser{language: lang}
		if chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, nil, Token{Symbol: tc.tok}, tc.state, actions, 2, nil); ok {
			t.Fatalf("%s deterministicConflictChoiceForDispatch picked %+v, want GLR fork", tc.name, chosen)
		}
	}
}

func TestGeneratedRepeatBoundaryConflictRequiresGeneratedReduce(t *testing.T) {
	lang := &Language{
		Name:                  "python",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "identifier", "module_repeat1"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "identifier"},
			{Name: "module_repeat1"},
		},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 616, Repetition: true},
	}
	if generatedRepeatBoundaryConflict(lang, actions) {
		t.Fatal("generatedRepeatBoundaryConflict = true for non-generated reduce, want false")
	}
}

func TestGeneratedRepeatBoundaryConflictAllowsMixedReduces(t *testing.T) {
	lang := &Language{
		Name:                  "python",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "identifier", "module_repeat1", "statement"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "identifier"},
			{Name: "module_repeat1", GeneratedRepeatAux: true},
			{Name: "statement"},
		},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionShift, State: 616, Repetition: true},
	}
	if !generatedRepeatBoundaryConflict(lang, actions) {
		t.Fatal("generatedRepeatBoundaryConflict = false for mixed generated/non-generated reduces, want true")
	}
}

func TestDeterministicConflictChoiceUsesCRepetitionFoldOnRecoveryLineages(t *testing.T) {
	// C parser.c:1625 (`if (action.shift.repetition) break;`) takes the single
	// REDUCE and redispatches the same lookahead on both clean and previously
	// recovered idle lineages. Without a stack (nil = no lineage evidence),
	// dispatch makes no deterministic choice and the GLR fork stands.
	lang := &Language{
		Name:        "bash",
		SymbolNames: []string{"end", "namespace", "\\", "name", "use", "new", "program_repeat1"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "namespace"},
			{Name: "\\"},
			{Name: "name"},
			{Name: "use"},
			{Name: "new"},
			{Name: "program_repeat1"},
		},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 6, ChildCount: 2},
		{Type: ParseActionShift, State: 1846, Repetition: true},
	}
	parser := &Parser{language: lang}
	if chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, nil, Token{Symbol: 1}, 2, actions, 2, nil); ok {
		t.Fatalf("deterministicConflictChoiceForDispatch picked %+v with nil stack, want GLR fork", chosen)
	}
	cleanStack := &glrStack{}
	chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, cleanStack, Token{Symbol: 1}, 2, actions, 2, nil)
	if !ok {
		t.Fatal("deterministicConflictChoiceForDispatch = false for clean lineage, want global repetition-skip fold")
	}
	if chosen.Type != ParseActionReduce || chosen.Symbol != 6 {
		t.Fatalf("deterministicConflictChoiceForDispatch picked %+v, want program_repeat1 reduce (C repetition-skip fold)", chosen)
	}
	erroredStack := &glrStack{cEverErrored: true}
	chosen, ok = parser.deterministicConflictChoiceForDispatch(nil, erroredStack, Token{Symbol: 1}, 2, actions, 2, nil)
	if !ok || chosen.Type != ParseActionReduce || chosen.Symbol != 6 {
		t.Fatalf("deterministicConflictChoiceForDispatch picked %+v on recovered lineage, want C repetition reduce", chosen)
	}
	parser.language.Name = "php"
	if chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, erroredStack, Token{Symbol: 1}, 2, actions, 2, nil); ok {
		t.Fatalf("deterministicConflictChoiceForDispatch picked %+v for recovered PHP lineage, want certified exception", chosen)
	}
	parser.language.Name = "bash"
	for i, stack := range []*glrStack{
		{cEverErrored: true, cRec: &cRecoverState{}},
		{cEverErrored: true, cPaused: true},
		{cEverErrored: true, cRecoverMissingGroup: &cRecGroup{}},
	} {
		chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, stack, Token{Symbol: 1}, 2, actions, 2, nil)
		if !ok || chosen.Type != ParseActionReduce || chosen.Symbol != 6 {
			t.Fatalf("active recovery case %d picked %+v, want C repetition reduce", i, chosen)
		}
	}
}

// Dart's repeat-boundary states are now certified ConflictPolicy rows
// (grammars/runtime_profiles.go) instead of the retired
// dartRepetitionShiftConflictChoice helper; these exercise the same shapes
// through the generic data-driven path.
func TestDartConflictPolicyRepetitionShiftCoversHotRepeats(t *testing.T) {
	for _, tc := range []struct {
		name         string
		state        StateID
		reduceSymbol Symbol
	}{
		{name: "enum_body_repeat2", state: 596, reduceSymbol: 1},
		{name: "extension_body_repeat1", state: 602, reduceSymbol: 2},
		{name: "program_repeat4", state: 479, reduceSymbol: 3},
	} {
		lang := &Language{
			Name:             "dart",
			SymbolNames:      []string{"end", "enum_body_repeat2", "extension_body_repeat1", "program_repeat4"},
			ConflictPolicies: []ConflictPolicy{{State: tc.state, Lookahead: 7, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{tc.reduceSymbol}}},
		}
		actions := []ParseAction{
			{Type: ParseActionReduce, Symbol: tc.reduceSymbol, ChildCount: 2},
			{Type: ParseActionShift, State: 9, Repetition: true},
		}

		chosen, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, tc.state, actions)
		if !ok {
			t.Fatalf("conflictPolicyChoice(%s) = false, want true", tc.name)
		}
		if chosen.Type != ParseActionShift || chosen.State != 9 || !chosen.Repetition {
			t.Fatalf("conflictPolicyChoice(%s) picked %+v, want repetition shift", tc.name, chosen)
		}
	}
}

func TestDartConflictPolicyRepetitionShiftRejectsOtherRepeat(t *testing.T) {
	lang := &Language{
		Name:             "dart",
		SymbolNames:      []string{"end", "enum_body_repeat2", "other_repeat1"},
		ConflictPolicies: []ConflictPolicy{{State: 596, Lookahead: 7, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{1}}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 9, Repetition: true},
	}
	if _, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, 596, actions); ok {
		t.Fatal("conflictPolicyChoice = true, want false")
	}
}

func TestHaskellRepeatBoundaryConflictChoiceAllowsHotRepeats(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "_cond_layout_semicolon", ",", "declarations_repeat1", "exports_repeat1"}}
	for _, tc := range []struct {
		name         string
		state        StateID
		reduceSymbol Symbol
		shiftState   StateID
	}{
		{name: "declarations", state: 9609, reduceSymbol: 3, shiftState: 21},
		{name: "exports", state: 10984, reduceSymbol: 4, shiftState: 7942},
	} {
		actions := []ParseAction{
			{Type: ParseActionReduce, Symbol: tc.reduceSymbol, ChildCount: 2},
			{Type: ParseActionShift, State: tc.shiftState, Repetition: true},
		}
		chosen, ok := haskellRepeatBoundaryConflictChoice(lang, tc.state, actions)
		if !ok {
			t.Fatalf("haskellRepeatBoundaryConflictChoice(%s) = false, want true", tc.name)
		}
		if chosen.Type != ParseActionReduce || chosen.Symbol != tc.reduceSymbol || chosen.Repetition {
			t.Fatalf("haskellRepeatBoundaryConflictChoice(%s) picked %+v, want repeat reduce", tc.name, chosen)
		}
	}
}

func TestHaskellRepeatBoundaryConflictChoiceRejectsOtherRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "declarations_repeat1", "other_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 21, Repetition: true},
	}
	if _, ok := haskellRepeatBoundaryConflictChoice(lang, 9609, actions); ok {
		t.Fatal("haskellRepeatBoundaryConflictChoice = true, want false for wrong repeat symbol")
	}
}

func TestHaskellRepeatBoundaryConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "declarations_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 1, ChildCount: 2},
		{Type: ParseActionShift, State: 21, Repetition: true},
	}
	if _, ok := haskellRepeatBoundaryConflictChoice(lang, 9610, actions); ok {
		t.Fatal("haskellRepeatBoundaryConflictChoice = true, want false for wrong state")
	}
}

func TestSwiftBraceTypeExpressionConflictChoiceAllowsHotRR(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "{", "_simple_user_type", "_expression"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1},
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
	}

	chosen, ok := swiftBraceTypeExpressionConflictChoice(lang, Token{Symbol: 1}, 2815, actions)
	if !ok {
		t.Fatal("swiftBraceTypeExpressionConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionReduce || chosen.Symbol != 2 {
		t.Fatalf("swiftBraceTypeExpressionConflictChoice picked %+v, want _simple_user_type reduce", chosen)
	}
}

func TestSwiftBraceTypeExpressionConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "{", "_simple_user_type", "_expression"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1},
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
	}

	if _, ok := swiftBraceTypeExpressionConflictChoice(lang, Token{Symbol: 1}, 2816, actions); ok {
		t.Fatal("swiftBraceTypeExpressionConflictChoice = true, want false")
	}
}

func TestSwiftBraceTypeExpressionConflictChoiceRejectsOtherReduce(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "{", "_simple_user_type", "other"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1},
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
	}

	if _, ok := swiftBraceTypeExpressionConflictChoice(lang, Token{Symbol: 1}, 2815, actions); ok {
		t.Fatal("swiftBraceTypeExpressionConflictChoice = true, want false")
	}
}

func TestSwiftBraceTypeExpressionConflictChoiceAllowsNavigableDotReduce(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ".", "_navigable_type_expression"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1},
		{Type: ParseActionShift, State: 201},
	}

	chosen, ok := swiftBraceTypeExpressionConflictChoice(lang, Token{Symbol: 1}, 72, actions)
	if !ok {
		t.Fatal("swiftBraceTypeExpressionConflictChoice(dot) = false, want true")
	}
	if chosen.Type != ParseActionReduce || chosen.Symbol != 2 {
		t.Fatalf("swiftBraceTypeExpressionConflictChoice(dot) picked %+v, want _navigable_type_expression reduce", chosen)
	}
}

func TestSwiftBraceTypeExpressionConflictChoiceRejectsOtherDotReduce(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ".", "_navigable_type_expression", "other"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionShift, State: 201},
	}

	if _, ok := swiftBraceTypeExpressionConflictChoice(lang, Token{Symbol: 1}, 72, actions); ok {
		t.Fatal("swiftBraceTypeExpressionConflictChoice(dot) = true, want false")
	}
}

func TestShouldRetryNodeLimitParse(t *testing.T) {
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason:     ParseStopNodeLimit,
			NodeLimit:      300_000,
			NodesAllocated: 300_001,
		},
	}

	if !shouldRetryNodeLimitParse(tree, 4096) {
		t.Fatal("shouldRetryNodeLimitParse = false, want true")
	}
}

func TestShouldNotRetryNodeLimitParseForLargeSource(t *testing.T) {
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason:     ParseStopNodeLimit,
			NodeLimit:      300_000,
			NodesAllocated: 300_001,
		},
	}

	if shouldRetryNodeLimitParse(tree, fullParseRetryMaxSourceBytes+1) {
		t.Fatal("shouldRetryNodeLimitParse = true, want false")
	}
}

func TestShouldNotRetryMemoryBudgetParse(t *testing.T) {
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason: ParseStopMemoryBudget,
		},
	}

	if shouldRetryNodeLimitParse(tree, 4096) {
		t.Fatal("shouldRetryNodeLimitParse = true, want false for memory budget stop")
	}
}

func TestFullParseRetryNodeLimitOverride(t *testing.T) {
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason:     ParseStopNodeLimit,
			NodeLimit:      300_000,
			NodesAllocated: 300_001,
		},
	}

	got := fullParseRetryNodeLimitOverride(tree, 4096)
	want := 600_000
	if got != want {
		t.Fatalf("fullParseRetryNodeLimitOverride = %d, want %d", got, want)
	}
}

func TestFullParseRetrySecondaryNodeLimitOverride(t *testing.T) {
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason:     ParseStopNodeLimit,
			NodeLimit:      600_000,
			NodesAllocated: 600_001,
		},
	}

	got := fullParseRetrySecondaryNodeLimitOverride(tree, 4096)
	want := 1_800_000
	if got != want {
		t.Fatalf("fullParseRetrySecondaryNodeLimitOverride = %d, want %d", got, want)
	}
}

func TestShouldRunInitialFullParseMergeRetry(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if shouldRunInitialFullParseMergeRetry(nil, 0, fullParseRetryOriginFresh) {
		t.Fatal("shouldRunInitialFullParseMergeRetry(nil) = true, want false")
	}
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason: ParseStopNodeLimit,
		},
	}
	if shouldRunInitialFullParseMergeRetry(tree, 128, fullParseRetryOriginFresh) {
		t.Fatal("shouldRunInitialFullParseMergeRetry(node_limit) = true, want false")
	}
	tree.parseRuntime.StopReason = ParseStopNoStacksAlive
	if !shouldRunInitialFullParseMergeRetry(tree, 128, fullParseRetryOriginFresh) {
		t.Fatal("shouldRunInitialFullParseMergeRetry(no_stacks_alive) = false, want true")
	}
	tree = &Tree{
		language: &Language{Name: "c_sharp"},
		root: &Node{
			endByte: 128,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 128,
			RootEndByte:     128,
			MaxStacksSeen:   64,
		},
	}
	if !shouldRunInitialFullParseMergeRetry(tree, 128, fullParseRetryOriginFresh) {
		t.Fatal("uncertified same-name C# tree skipped the initial merge retry")
	}
	tree.language.FullParseAcceptedErrorRetryProfile.SkipInitialCompleteAcceptedErrorMergeRetry = true
	if shouldRunInitialFullParseMergeRetry(tree, 128, fullParseRetryOriginFresh) {
		t.Fatal("certified complete accepted-error tree scheduled the initial merge retry")
	}
	if !shouldRunInitialFullParseMergeRetry(tree, 128, fullParseRetryOriginIncremental) {
		t.Fatal("certified incremental fallback skipped the conservative initial merge retry")
	}
	tree.parseRuntime.Truncated = true
	if !shouldRunInitialFullParseMergeRetry(tree, 128, fullParseRetryOriginFresh) {
		t.Fatal("certified truncated accepted-error tree skipped the initial merge retry")
	}
}

func TestCertifiedInitialMergeRetrySkipHonorsExplicitOverrides(t *testing.T) {
	for _, env := range []string{"GOT_GLR_MAX_STACKS", "GOT_GLR_MAX_MERGE_PER_KEY"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("GOT_GLR_MAX_STACKS", "")
			t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
			t.Setenv(env, "8")
			ResetParseEnvConfigCacheForTests()
			defer ResetParseEnvConfigCacheForTests()

			tree := certifiedAcceptedErrorRetryTestTree(false, 128)
			tree.language.Name = "c_sharp"
			tree.language.FullParseAcceptedErrorRetryProfile.SkipInitialCompleteAcceptedErrorMergeRetry = true
			if !shouldRunInitialFullParseMergeRetry(tree, 128, fullParseRetryOriginFresh) {
				t.Fatalf("certified policy ignored explicit %s", env)
			}
		})
	}
}

func TestCertifiedInitialMergeRetrySkipKeepsWideningAndTreeSelection(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	const sourceLen = 128
	source := make([]byte, sourceLen)
	initial := certifiedAcceptedErrorRetryTestTree(false, sourceLen)
	initial.language.Name = "c_sharp"
	initial.language.FullParseAcceptedErrorRetryProfile.SkipInitialCompleteAcceptedErrorMergeRetry = true
	initial.parseRuntime.MaxStacksSeen = 8

	candidate := certifiedAcceptedErrorRetryTestTree(false, sourceLen)
	candidate.language.Name = "c_sharp"
	candidate.root.flags = 0
	candidate.parseRuntime.NodesAllocated = 1

	type retryCall struct {
		stacks int
		merge  int
		nodes  int
	}
	var calls []retryCall
	parser := &Parser{}
	got := parser.retryFullParse(source, 8, initial, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
		calls = append(calls, retryCall{stacks: maxStacks, merge: maxMergePerKeyOverride, nodes: maxNodes})
		return candidate
	})
	if got != candidate {
		t.Fatalf("retryFullParse returned %p, want widened clean tree %p", got, candidate)
	}
	want := []retryCall{{stacks: fullParseRetryMaxGLRStacks}}
	if !slices.Equal(calls, want) {
		t.Fatalf("retry calls = %+v, want widened pass %+v with no initial same-stack merge", calls, want)
	}
}

func duplicateWideRetryTestTree(sourceLen, nodes, maxStacks int, certified bool) *Tree {
	tree := certifiedAcceptedErrorRetryTestTree(false, sourceLen)
	tree.language.Name = "synthetic"
	tree.parseRuntime.NodesAllocated = nodes
	tree.parseRuntime.MaxStacksSeen = maxStacks
	if certified {
		tree.language.FullParseAcceptedErrorRetryProfile = FullParseAcceptedErrorRetryProfile{
			ReuseCleanWideForWideRetry:   true,
			ReuseCleanWideMinSourceBytes: 128 * 1024,
		}
	}
	return tree
}

func TestCertifiedDuplicateWideRetryPreservesPassAccounting(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	t.Setenv("GOT_PARSE_NODE_LIMIT_SCALE", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	const sourceLen = 256 * 1024
	type retryCall struct {
		stacks int
		merge  int
		clean  bool
	}
	run := func(certified bool) (calls []retryCall, passes int, got *Tree) {
		parser := &Parser{}
		initial := duplicateWideRetryTestTree(sourceLen, 100, 18, certified)
		got = parser.retryFullParse(make([]byte, sourceLen), 8, initial, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
			calls = append(calls, retryCall{stacks: maxStacks, merge: maxMergePerKeyOverride, clean: parser.forceCleanRetryPass})
			switch {
			case maxMergePerKeyOverride != 0 && len(calls) == 1:
				return duplicateWideRetryTestTree(sourceLen, 90, 36, certified)
			case parser.forceCleanRetryPass:
				return duplicateWideRetryTestTree(sourceLen, 80, 54, certified)
			case maxMergePerKeyOverride != 0:
				return duplicateWideRetryTestTree(sourceLen, 85, 73, certified)
			default:
				return duplicateWideRetryTestTree(sourceLen, 80, 54, certified)
			}
		})
		return calls, parser.fullParseRetryPassesTaken, got
	}

	genericCalls, genericPasses, genericTree := run(false)
	certifiedCalls, certifiedPasses, certifiedTree := run(true)
	if len(genericCalls) != 4 {
		t.Fatalf("generic retry calls = %+v, want initial-merge, clean-wide, wide, final-merge", genericCalls)
	}
	if len(certifiedCalls) != 3 {
		t.Fatalf("certified retry calls = %+v, want initial-merge, clean-wide, final-merge", certifiedCalls)
	}
	if genericPasses != 4 || certifiedPasses != genericPasses {
		t.Fatalf("retry passes generic=%d certified=%d, want both 4", genericPasses, certifiedPasses)
	}
	if genericTree == nil || certifiedTree == nil || genericTree.parseRuntime.NodesAllocated != certifiedTree.parseRuntime.NodesAllocated {
		t.Fatalf("selected nodes generic=%v certified=%v, want equivalent selected candidate", genericTree, certifiedTree)
	}
}

func TestCertifiedDuplicateWideRetryFallbackScope(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	t.Setenv("GOT_PARSE_NODE_LIMIT_SCALE", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	const sourceLen = 256 * 1024
	eligible := func() (*Parser, *Tree) {
		return &Parser{}, duplicateWideRetryTestTree(sourceLen, 80, 54, true)
	}
	parser, tree := eligible()
	if !certifiedAcceptedErrorRetryReusesCleanWide(parser, tree, sourceLen, fullParseRetryOriginFresh, 0) {
		t.Fatal("eligible complete accepted-error clean-wide tree did not reuse")
	}

	assertFallback := func(label string, parser *Parser, tree *Tree, bytes int, origin fullParseRetryOrigin, maxNodes int) {
		t.Helper()
		if certifiedAcceptedErrorRetryReusesCleanWide(parser, tree, bytes, origin, maxNodes) {
			t.Fatalf("%s unexpectedly reused clean-wide tree", label)
		}
	}

	parser, tree = eligible()
	parser.timeoutMicros = 1_000_000
	if !certifiedAcceptedErrorRetryReusesCleanWide(parser, tree, sourceLen, fullParseRetryOriginFresh, 0) {
		t.Fatal("configured timeout disabled exact-blob duplicate-wide policy")
	}
	parser, tree = eligible()
	cancelled := uint32(0)
	parser.cancellationFlag = &cancelled
	if !certifiedAcceptedErrorRetryReusesCleanWide(parser, tree, sourceLen, fullParseRetryOriginFresh, 0) {
		t.Fatal("configured cancellation pointer disabled exact-blob duplicate-wide policy")
	}
	parser, tree = eligible()
	assertFallback("incremental origin", parser, tree, sourceLen, fullParseRetryOriginIncremental, 0)
	parser, tree = eligible()
	assertFallback("node-limit retry", parser, tree, sourceLen, fullParseRetryOriginFresh, 1)
	parser, tree = eligible()
	assertFallback("below certified floor", parser, tree, 129_488, fullParseRetryOriginFresh, 0)

	for _, env := range []string{"GOT_GLR_MAX_STACKS", "GOT_GLR_MAX_MERGE_PER_KEY", "GOT_PARSE_NODE_LIMIT_SCALE"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, "2")
			parser, tree := eligible()
			assertFallback("explicit "+env, parser, tree, sourceLen, fullParseRetryOriginFresh, 0)
			t.Setenv(env, "")
		})
	}
}

func TestCertifiedDuplicateWideRetryDoesNotBypassTerminalCancellation(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	t.Setenv("GOT_PARSE_NODE_LIMIT_SCALE", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	const sourceLen = 256 * 1024
	cancelled := uint32(1)
	parser := &Parser{cancellationFlag: &cancelled}
	initial := duplicateWideRetryTestTree(sourceLen, 100, 18, true)
	calls := 0
	parser.retryFullParse(make([]byte, sourceLen), 8, initial, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
		calls++
		return duplicateWideRetryTestTree(sourceLen, 90, 36, true)
	})
	if calls != 1 {
		t.Fatalf("retry calls after terminal cancellation = %d, want only the pre-deadline initial merge", calls)
	}
	if parser.fullParseRetryPassesTaken != 1 {
		t.Fatalf("retry passes after terminal cancellation = %d, want 1", parser.fullParseRetryPassesTaken)
	}
}

func TestCSharpAcceptedErrorTreeCanUseNamespaceRecovery(t *testing.T) {
	source := []byte("using X;\nnamespace N { class C { } }\n")
	nsStart := uint32(bytes.Index(source, []byte("namespace")))
	nsEnd := uint32(bytes.LastIndexByte(source, '}') + 1)
	if nsStart == 0 || nsEnd == 0 {
		t.Fatal("test source does not contain expected namespace span")
	}
	lang := &Language{
		Name:        "c_sharp",
		SymbolNames: []string{"EOF", "compilation_unit", "namespace_declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "compilation_unit", Visible: true, Named: true},
			{Name: "namespace_declaration", Visible: true, Named: true},
		},
	}
	arena := newNodeArena(arenaClassFull)
	namespace := newLeafNodeInArena(arena, 2, true, nsStart, nsEnd, Point{}, Point{})
	namespace.setHasError(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{namespace}, nil, 0)
	tree := &Tree{
		language: lang,
		root:     root,
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: uint32(len(source)),
			RootEndByte:     uint32(len(source)),
			MaxStacksSeen:   64,
		},
	}

	if !csharpAcceptedErrorTreeCanUseNamespaceRecovery(tree, source) {
		t.Fatal("csharpAcceptedErrorTreeCanUseNamespaceRecovery = false, want true")
	}
	tree.parseRuntime.Truncated = true
	if csharpAcceptedErrorTreeCanUseNamespaceRecovery(tree, source) {
		t.Fatal("csharpAcceptedErrorTreeCanUseNamespaceRecovery(truncated) = true, want false")
	}
	tree.parseRuntime.Truncated = false
	root.ownerArena = nil
	if csharpAcceptedErrorTreeCanUseNamespaceRecovery(tree, source) {
		t.Fatal("csharpAcceptedErrorTreeCanUseNamespaceRecovery(no arena) = true, want false")
	}
}

func TestCertifiedLargeAcceptedErrorRetryUsesInitialStackCeiling(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	const sourceLen = 96 * 1024
	for _, tt := range []struct {
		name string
		cap  uint16
	}{
		{name: "groovy", cap: 2},
		{name: "d", cap: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tree := certifiedAcceptedErrorRetryTestTree(false, sourceLen)
			tree.language.Name = tt.name
			tree.language.FullParseAcceptedErrorRetryProfile = FullParseAcceptedErrorRetryProfile{
				MinSourceBytes:      64 * 1024,
				InitialStackCeiling: tt.cap,
			}
			if got := fullParseRetryMaxStacksOverrideForOrigin(tree, sourceLen, int(tt.cap), fullParseRetryOriginFresh); got != 0 {
				t.Fatalf("certified fresh stack override = %d, want 0", got)
			}
			if got := fullParseRetryMaxStacksOverrideForOrigin(tree, sourceLen, int(tt.cap), fullParseRetryOriginIncremental); got != fullParseRetryMaxGLRStacks {
				t.Fatalf("incremental stack override = %d, want %d", got, fullParseRetryMaxGLRStacks)
			}
			if got := fullParseRetryMaxStacksOverride(tree, 1024, int(tt.cap)); got != fullParseRetryMaxGLRStacks {
				t.Fatalf("small-source stack override = %d, want %d", got, fullParseRetryMaxGLRStacks)
			}

			tree.language.FullParseAcceptedErrorRetryProfile = FullParseAcceptedErrorRetryProfile{}
			if got := fullParseRetryMaxStacksOverride(tree, sourceLen, int(tt.cap)); got != fullParseRetryMaxGLRStacks {
				t.Fatalf("uncertified same-name stack override = %d, want %d", got, fullParseRetryMaxGLRStacks)
			}
		})
	}
}

func certifiedAcceptedErrorRetryTestTree(profile bool, sourceLen int) *Tree {
	lang := &Language{Name: "java"}
	if profile {
		lang.FullParseAcceptedErrorRetryProfile = FullParseAcceptedErrorRetryProfile{
			MinSourceBytes:      64 * 1024,
			InitialStackCeiling: 14,
		}
	}
	return &Tree{
		language: lang,
		root: &Node{
			endByte: uint32(sourceLen),
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:       ParseStopAccepted,
			SourceLen:        uint32(sourceLen),
			ExpectedEOFByte:  uint32(sourceLen),
			RootEndByte:      uint32(sourceLen),
			LastTokenEndByte: uint32(sourceLen),
			LastTokenWasEOF:  true,
			MaxStacksSeen:    18,
			NodesAllocated:   100,
		},
	}
}

func TestCertifiedAcceptedErrorRetryStackCeilingScope(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	const sourceLen = 64 * 1024
	eligible := certifiedAcceptedErrorRetryTestTree(true, sourceLen)
	if !fullParseRetryUsesInitialStackCeilingForOrigin(eligible, sourceLen, 14, fullParseRetryOriginFresh) {
		t.Fatal("certified fresh accepted-error parse did not keep its initial stack ceiling")
	}
	if got := fullParseRetryMaxStacksOverrideForOrigin(eligible, sourceLen, 14, fullParseRetryOriginFresh); got != 0 {
		t.Fatalf("certified fresh max-stack override = %d, want 0", got)
	}
	if got := fullParseRetryMergePerKeyOverride(eligible, sourceLen, 14); got != javaFullParseRetryMaxMergePerKey {
		t.Fatalf("certified same-stack merge override = %d, want %d", got, javaFullParseRetryMaxMergePerKey)
	}

	assertNotCertified := func(label string, tree *Tree, bytes, stacks int, origin fullParseRetryOrigin) {
		t.Helper()
		if fullParseRetryUsesInitialStackCeilingForOrigin(tree, bytes, stacks, origin) {
			t.Fatalf("%s unexpectedly used the certified stack ceiling", label)
		}
	}

	assertNotCertified("uncertified language", certifiedAcceptedErrorRetryTestTree(false, sourceLen), sourceLen, 14, fullParseRetryOriginFresh)
	assertNotCertified("small source", certifiedAcceptedErrorRetryTestTree(true, sourceLen-1), sourceLen-1, 14, fullParseRetryOriginFresh)
	assertNotCertified("different initial ceiling", certifiedAcceptedErrorRetryTestTree(true, sourceLen), sourceLen, 13, fullParseRetryOriginFresh)
	assertNotCertified("incremental origin", certifiedAcceptedErrorRetryTestTree(true, sourceLen), sourceLen, 14, fullParseRetryOriginIncremental)

	clean := certifiedAcceptedErrorRetryTestTree(true, sourceLen)
	clean.root.flags = 0
	assertNotCertified("clean tree", clean, sourceLen, 14, fullParseRetryOriginFresh)

	otherStop := certifiedAcceptedErrorRetryTestTree(true, sourceLen)
	otherStop.parseRuntime.StopReason = ParseStopNoStacksAlive
	assertNotCertified("other stop reason", otherStop, sourceLen, 14, fullParseRetryOriginFresh)

	truncated := certifiedAcceptedErrorRetryTestTree(true, sourceLen)
	truncated.parseRuntime.Truncated = true
	assertNotCertified("truncated result", truncated, sourceLen, 14, fullParseRetryOriginFresh)

	earlyEOF := certifiedAcceptedErrorRetryTestTree(true, sourceLen)
	earlyEOF.parseRuntime.TokenSourceEOFEarly = true
	assertNotCertified("early token-source EOF", earlyEOF, sourceLen, 14, fullParseRetryOriginFresh)

	shortTree := certifiedAcceptedErrorRetryTestTree(true, sourceLen)
	shortTree.root.endByte--
	shortTree.parseRuntime.RootEndByte--
	shortTree.parseRuntime.LastTokenWasEOF = false
	shortTree.parseRuntime.LastTokenEndByte--
	assertNotCertified("tree not covering EOF", shortTree, sourceLen, 14, fullParseRetryOriginFresh)
}

func TestCertifiedAcceptedErrorRetryStackCeilingHonorsExplicitOverrides(t *testing.T) {
	const sourceLen = 64 * 1024
	tree := certifiedAcceptedErrorRetryTestTree(true, sourceLen)

	for _, env := range []string{"GOT_GLR_MAX_STACKS", "GOT_GLR_MAX_MERGE_PER_KEY"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("GOT_GLR_MAX_STACKS", "")
			t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
			t.Setenv(env, "14")
			if certifiedAcceptedErrorRetryUsesInitialStackCeiling(tree, sourceLen, 14) {
				t.Fatalf("certified policy ignored explicit %s", env)
			}
		})
	}
}

func TestCertifiedAcceptedErrorRetrySchedulingKeepsMergeAndSkipsWidening(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	const sourceLen = 64 * 1024
	source := make([]byte, sourceLen)
	type retryCall struct {
		stacks int
		merge  int
		nodes  int
	}

	t.Run("fresh certified", func(t *testing.T) {
		parser := &Parser{}
		initial := certifiedAcceptedErrorRetryTestTree(true, sourceLen)
		var calls []retryCall
		parser.retryFullParse(source, 14, initial, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
			calls = append(calls, retryCall{maxStacks, maxMergePerKeyOverride, maxNodes})
			candidate := certifiedAcceptedErrorRetryTestTree(true, sourceLen)
			candidate.parseRuntime.NodesAllocated = 99
			return candidate
		})
		want := []retryCall{{stacks: 14, merge: javaFullParseRetryMaxMergePerKey}}
		if !slices.Equal(calls, want) {
			t.Fatalf("retry calls = %+v, want %+v", calls, want)
		}
	})

	for _, tt := range []struct {
		name    string
		profile bool
		origin  fullParseRetryOrigin
	}{
		{name: "uncertified fresh", profile: false, origin: fullParseRetryOriginFresh},
		{name: "certified incremental", profile: true, origin: fullParseRetryOriginIncremental},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parser := &Parser{}
			initial := certifiedAcceptedErrorRetryTestTree(tt.profile, sourceLen)
			var calls []retryCall
			parser.retryFullParseForOrigin(source, 14, initial, tt.origin, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
				calls = append(calls, retryCall{maxStacks, maxMergePerKeyOverride, maxNodes})
				if maxStacks == javaFullParseRetryMaxGLRStacks {
					clean := certifiedAcceptedErrorRetryTestTree(tt.profile, sourceLen)
					clean.root.flags = 0
					clean.parseRuntime.NodesAllocated = 1
					return clean
				}
				candidate := certifiedAcceptedErrorRetryTestTree(tt.profile, sourceLen)
				candidate.parseRuntime.NodesAllocated = 99
				return candidate
			})
			if len(calls) < 2 || calls[0] != (retryCall{stacks: 14, merge: javaFullParseRetryMaxMergePerKey}) || calls[1].stacks != javaFullParseRetryMaxGLRStacks {
				t.Fatalf("retry calls = %+v, want same-stack merge followed by cap-%d widening", calls, javaFullParseRetryMaxGLRStacks)
			}
		})
	}
}

func TestCppAcceptedErrorRetrySkipsCompleteTree(t *testing.T) {
	tree := &Tree{
		language: &Language{
			Name: "cpp",
			FullParseAcceptedErrorRetryProfile: FullParseAcceptedErrorRetryProfile{
				SkipCompleteAcceptedErrorRetry: true,
			},
		},
		root: &Node{
			endByte: 128,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 128,
			RootEndByte:     128,
			MaxStacksSeen:   18,
		},
	}

	if shouldRetryAcceptedErrorParse(tree, 128, 18) {
		t.Fatal("shouldRetryAcceptedErrorParse(cpp complete accepted error) = true, want false")
	}
	if got := fullParseRetryMergePerKeyOverride(tree, 128, 18); got != 0 {
		t.Fatalf("fullParseRetryMergePerKeyOverride(cpp complete accepted error) = %d, want 0", got)
	}
}

func TestCppAcceptedErrorRetryPreservesTruncatedMergeRetry(t *testing.T) {
	tree := &Tree{
		language: &Language{Name: "cpp"},
		root: &Node{
			endByte: 96,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 128,
			RootEndByte:     96,
			Truncated:       true,
			MaxStacksSeen:   18,
		},
	}

	if got := fullParseRetryMergePerKeyOverride(tree, 128, 18); got != fullParseRetryMaxMergePerKey {
		t.Fatalf("fullParseRetryMergePerKeyOverride(cpp truncated accepted error) = %d, want %d", got, fullParseRetryMaxMergePerKey)
	}
}

func TestBashAcceptedErrorRetrySkipsCompleteTree(t *testing.T) {
	tree := &Tree{
		language: &Language{
			Name: "bash",
			FullParseAcceptedErrorRetryProfile: FullParseAcceptedErrorRetryProfile{
				SkipCompleteAcceptedErrorRetry: true,
			},
		},
		root: &Node{
			endByte: 128,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 128,
			RootEndByte:     128,
			MaxStacksSeen:   18,
		},
	}

	if shouldRetryAcceptedErrorParse(tree, 128, 18) {
		t.Fatal("shouldRetryAcceptedErrorParse(bash complete accepted error) = true, want false")
	}
	if got := fullParseRetryMergePerKeyOverride(tree, 128, 18); got != 0 {
		t.Fatalf("fullParseRetryMergePerKeyOverride(bash complete accepted error) = %d, want 0", got)
	}
}

func TestBashAcceptedErrorRetryPreservesTruncatedMergeRetry(t *testing.T) {
	tree := &Tree{
		language: &Language{Name: "bash"},
		root: &Node{
			endByte: 96,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 128,
			RootEndByte:     96,
			Truncated:       true,
			MaxStacksSeen:   18,
		},
	}

	if got := fullParseRetryMergePerKeyOverride(tree, 128, 18); got != fullParseRetryMaxMergePerKey {
		t.Fatalf("fullParseRetryMergePerKeyOverride(bash truncated accepted error) = %d, want %d", got, fullParseRetryMaxMergePerKey)
	}
}

func TestCompleteAcceptedErrorRetrySkipRequiresCertification(t *testing.T) {
	tree := &Tree{
		language: &Language{Name: "rego"},
		root: &Node{
			endByte: 128,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:       ParseStopAccepted,
			SourceLen:        128,
			ExpectedEOFByte:  128,
			RootEndByte:      128,
			LastTokenEndByte: 128,
			LastTokenWasEOF:  true,
			MaxStacksSeen:    9,
		},
	}
	if !shouldRetryAcceptedErrorParse(tree, 128, 8) {
		t.Fatal("uncertified language name suppressed accepted-error retry")
	}

	tree.language.FullParseAcceptedErrorRetryProfile.SkipCompleteAcceptedErrorRetry = true
	if shouldRetryAcceptedErrorParse(tree, 128, 8) {
		t.Fatal("certified complete accepted-error tree scheduled a stack retry")
	}
	if got := fullParseRetryMergePerKeyOverride(tree, 128, 8); got != 0 {
		t.Fatalf("certified complete accepted-error merge override = %d, want 0", got)
	}

	tree.language.FullParseAcceptedErrorRetryProfile.SkipCompleteMaxEntryScratchPeak = 1024
	tree.parseRuntime.EntryScratchPeak = 1025
	if !shouldRetryAcceptedErrorParse(tree, 128, 8) {
		t.Fatal("accepted-error tree above the certified entry-scratch peak skipped retry")
	}
	tree.parseRuntime.EntryScratchPeak = 1024
	if shouldRetryAcceptedErrorParse(tree, 128, 8) {
		t.Fatal("accepted-error tree at the certified entry-scratch peak scheduled retry")
	}
}

func TestCompleteAcceptedErrorRetrySkipHonorsMinimumSourceBytes(t *testing.T) {
	const minSourceBytes = 2 * 1024
	newTree := func(sourceLen int, minSourceBytes uint32) *Tree {
		return &Tree{
			language: &Language{
				Name: "synthetic",
				FullParseAcceptedErrorRetryProfile: FullParseAcceptedErrorRetryProfile{
					SkipCompleteAcceptedErrorRetry: true,
					SkipCompleteMinSourceBytes:     minSourceBytes,
				},
			},
			root: &Node{
				endByte: uint32(sourceLen),
				flags:   nodeFlagHasError,
			},
			parseRuntime: ParseRuntime{
				StopReason:       ParseStopAccepted,
				SourceLen:        uint32(sourceLen),
				ExpectedEOFByte:  uint32(sourceLen),
				RootEndByte:      uint32(sourceLen),
				LastTokenEndByte: uint32(sourceLen),
				LastTokenWasEOF:  true,
				MaxStacksSeen:    9,
			},
		}
	}

	tests := []struct {
		name            string
		sourceLen       int
		minSourceBytes  uint32
		wantRetry       bool
		wantMergePerKey int
	}{
		{name: "legacy zero is unbounded", sourceLen: 128, minSourceBytes: 0, wantMergePerKey: 0},
		{name: "below threshold keeps retry", sourceLen: minSourceBytes - 1, minSourceBytes: minSourceBytes, wantRetry: true, wantMergePerKey: fullParseRetryMaxMergePerKey},
		{name: "at threshold skips retry", sourceLen: minSourceBytes, minSourceBytes: minSourceBytes, wantMergePerKey: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := newTree(tt.sourceLen, tt.minSourceBytes)
			if got := shouldRetryAcceptedErrorParse(tree, tt.sourceLen, 8); got != tt.wantRetry {
				t.Fatalf("shouldRetryAcceptedErrorParse() = %v, want %v", got, tt.wantRetry)
			}
			if got := fullParseRetryMergePerKeyOverride(tree, tt.sourceLen, 8); got != tt.wantMergePerKey {
				t.Fatalf("fullParseRetryMergePerKeyOverride() = %d, want %d", got, tt.wantMergePerKey)
			}
		})
	}
}

func certifiedFreshErrorNoStacksTestTree(profile bool, sourceLen int) *Tree {
	lang := &Language{Name: "synthetic"}
	if profile {
		lang.FullParseAcceptedErrorRetryProfile.FreshErrorNoStacksMaxPasses = 1
	}
	return &Tree{
		language: lang,
		root: &Node{
			endByte: uint32(sourceLen / 2),
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:       ParseStopNoStacksAlive,
			Truncated:        true,
			SourceLen:        uint32(sourceLen),
			ExpectedEOFByte:  uint32(sourceLen),
			RootEndByte:      uint32(sourceLen / 2),
			LastTokenEndByte: uint32(sourceLen/2 + 1),
			MaxStacksSeen:    8,
			NodesAllocated:   100,
		},
	}
}

func TestCertifiedFreshErrorNoStacksRetryPassLimitScope(t *testing.T) {
	const sourceLen = 128
	eligible := certifiedFreshErrorNoStacksTestTree(true, sourceLen)
	if got := certifiedFreshErrorNoStacksRetryPassLimit(eligible, sourceLen, fullParseRetryOriginFresh); got != 1 {
		t.Fatalf("fresh certified retry limit = %d, want 1", got)
	}
	if got := certifiedFreshErrorNoStacksRetryPassLimit(eligible, sourceLen, fullParseRetryOriginIncremental); got != 0 {
		t.Fatalf("incremental certified retry limit = %d, want 0", got)
	}
	if got := certifiedFreshErrorNoStacksRetryPassLimit(certifiedFreshErrorNoStacksTestTree(false, sourceLen), sourceLen, fullParseRetryOriginFresh); got != 0 {
		t.Fatalf("uncertified retry limit = %d, want 0", got)
	}
	clean := certifiedFreshErrorNoStacksTestTree(true, sourceLen)
	clean.root.flags = 0
	if got := certifiedFreshErrorNoStacksRetryPassLimit(clean, sourceLen, fullParseRetryOriginFresh); got != 0 {
		t.Fatalf("clean no-stacks retry limit = %d, want 0", got)
	}
	accepted := certifiedFreshErrorNoStacksTestTree(true, sourceLen)
	accepted.parseRuntime.StopReason = ParseStopAccepted
	if got := certifiedFreshErrorNoStacksRetryPassLimit(accepted, sourceLen, fullParseRetryOriginFresh); got != 0 {
		t.Fatalf("accepted retry limit = %d, want 0", got)
	}
}

func TestCertifiedFreshErrorNoStacksRetryPassLimitScheduling(t *testing.T) {
	const sourceLen = 128
	source := make([]byte, sourceLen)
	parser := &Parser{}
	initial := certifiedFreshErrorNoStacksTestTree(true, sourceLen)
	var calls int
	result := parser.retryFullParse(source, 8, initial, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
		calls++
		return certifiedFreshErrorNoStacksTestTree(true, sourceLen)
	})
	if calls != 1 {
		t.Fatalf("certified retry calls = %d, want 1", calls)
	}
	if result == nil {
		t.Fatal("certified retry returned nil tree")
	}
	calls = 0
	parser.retryFullParse(source, 8, certifiedFreshErrorNoStacksTestTree(true, sourceLen), func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
		calls++
		return certifiedFreshErrorNoStacksTestTree(true, sourceLen)
	})
	if calls != 0 {
		t.Fatalf("second certified retry calls = %d, want shared per-operation limit", calls)
	}

	parser = &Parser{}
	initial = certifiedFreshErrorNoStacksTestTree(false, sourceLen)
	calls = 0
	parser.retryFullParse(source, 8, initial, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
		calls++
		return certifiedFreshErrorNoStacksTestTree(false, sourceLen)
	})
	if calls <= 1 {
		t.Fatalf("uncertified retry calls = %d, want conservative ladder", calls)
	}
}

func certifiedFreshErrorNoStacksRetryTargetTestTree(target uint16, sourceLen int) *Tree {
	tree := certifiedFreshErrorNoStacksTestTree(false, sourceLen)
	tree.language.FullParseAcceptedErrorRetryProfile = FullParseAcceptedErrorRetryProfile{
		SkipCompleteAcceptedErrorRetry:   true,
		FreshErrorNoStacksRetryMaxStacks: target,
	}
	return tree
}

func TestCertifiedFreshErrorNoStacksRetryMaxStacksScope(t *testing.T) {
	const sourceLen = 128
	eligible := certifiedFreshErrorNoStacksRetryTargetTestTree(16, sourceLen)
	if got := certifiedFreshErrorNoStacksRetryMaxStacks(eligible, sourceLen, fullParseRetryOriginFresh); got != 16 {
		t.Fatalf("fresh certified retry target = %d, want 16", got)
	}
	if got := certifiedFreshErrorNoStacksRetryMaxStacks(eligible, sourceLen, fullParseRetryOriginIncremental); got != 0 {
		t.Fatalf("incremental certified retry target = %d, want 0", got)
	}
	if got := certifiedFreshErrorNoStacksRetryMaxStacks(certifiedFreshErrorNoStacksRetryTargetTestTree(0, sourceLen), sourceLen, fullParseRetryOriginFresh); got != 0 {
		t.Fatalf("uncertified retry target = %d, want 0", got)
	}
	clean := certifiedFreshErrorNoStacksRetryTargetTestTree(16, sourceLen)
	clean.root.flags = 0
	if got := certifiedFreshErrorNoStacksRetryMaxStacks(clean, sourceLen, fullParseRetryOriginFresh); got != 0 {
		t.Fatalf("clean no-stacks retry target = %d, want 0", got)
	}
	accepted := certifiedFreshErrorNoStacksRetryTargetTestTree(16, sourceLen)
	accepted.parseRuntime.StopReason = ParseStopAccepted
	if got := certifiedFreshErrorNoStacksRetryMaxStacks(accepted, sourceLen, fullParseRetryOriginFresh); got != 0 {
		t.Fatalf("accepted retry target = %d, want 0", got)
	}
	if got := certifiedFreshErrorNoStacksRetryMaxStacks(eligible, fullParseRetryMaxSourceBytes+1, fullParseRetryOriginFresh); got != 0 {
		t.Fatalf("oversize retry target = %d, want 0", got)
	}
}

func TestCertifiedFreshErrorNoStacksRetryMaxStacksScheduling(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	const sourceLen = 128
	initial := certifiedFreshErrorNoStacksRetryTargetTestTree(16, sourceLen)
	if got := fullParseRetryMaxStacksOverrideForOrigin(initial, sourceLen, 8, fullParseRetryOriginFresh); got != 16 {
		t.Fatalf("fresh certified max-stacks override = %d, want 16", got)
	}
	if got := fullParseRetryMaxStacksOverrideForOrigin(initial, sourceLen, 16, fullParseRetryOriginFresh); got != 0 {
		t.Fatalf("already-at-target max-stacks override = %d, want 0", got)
	}
	if got := fullParseRetryMaxStacksOverrideForOrigin(initial, sourceLen, 8, fullParseRetryOriginIncremental); got != fullParseRetryMaxGLRStacks {
		t.Fatalf("incremental max-stacks override = %d, want generic %d", got, fullParseRetryMaxGLRStacks)
	}

	t.Setenv("GOT_GLR_MAX_STACKS", "12")
	ResetParseEnvConfigCacheForTests()
	if got := fullParseRetryMaxStacksOverrideForOrigin(initial, sourceLen, 8, fullParseRetryOriginFresh); got != 0 {
		t.Fatalf("explicit max-stacks override scheduled certified retry target %d", got)
	}
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	ResetParseEnvConfigCacheForTests()
	if got := fullParseRetryMaxStacksOverrideForOrigin(initial, sourceLen, 8, fullParseRetryOriginFresh); got != 16 {
		t.Fatalf("reset certified max-stacks override = %d, want 16", got)
	}
}

func TestCertifiedFreshErrorNoStacksRetryMaxStacksFlowsThroughLadder(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_STACKS", "")
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	const sourceLen = 128
	source := make([]byte, sourceLen)
	initial := certifiedFreshErrorNoStacksRetryTargetTestTree(16, sourceLen)
	type retryCall struct {
		stacks int
		merge  int
	}
	var calls []retryCall
	parser := &Parser{}
	result := parser.retryFullParse(source, 8, initial, func(maxStacks, maxMergePerKeyOverride, _ int) *Tree {
		calls = append(calls, retryCall{stacks: maxStacks, merge: maxMergePerKeyOverride})
		candidate := certifiedFreshErrorNoStacksRetryTargetTestTree(16, sourceLen)
		if maxStacks == 16 {
			candidate.root.endByte = sourceLen
			candidate.parseRuntime.StopReason = ParseStopAccepted
			candidate.parseRuntime.Truncated = false
			candidate.parseRuntime.RootEndByte = sourceLen
			candidate.parseRuntime.LastTokenEndByte = sourceLen
			candidate.parseRuntime.LastTokenWasEOF = true
		}
		return candidate
	})
	want := []retryCall{{stacks: 8, merge: fullParseRetryMaxMergePerKey}, {stacks: 16}, {stacks: 16}}
	if !slices.Equal(calls, want) {
		t.Fatalf("retry calls = %+v, want %+v", calls, want)
	}
	if result == nil || result.ParseStopReason() != ParseStopAccepted {
		t.Fatalf("retry result = %v, want accepted tree", result)
	}
}

func TestJavaAcceptedErrorRetryUsesWideMergeRetry(t *testing.T) {
	errChild := &Node{
		symbol: errorSymbol,
		flags:  nodeFlagHasError,
	}
	tree := &Tree{
		language: &Language{Name: "java"},
		root: &Node{
			endByte: 128,
			children: []*Node{
				errChild,
			},
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 128,
			RootEndByte:     128,
			MaxStacksSeen:   1,
		},
	}

	if got := fullParseRetryMergePerKeyOverride(tree, 128, 6); got != javaFullParseRetryMaxMergePerKey {
		t.Fatalf("fullParseRetryMergePerKeyOverride(java accepted error) = %d, want %d", got, javaFullParseRetryMaxMergePerKey)
	}
}

func TestNoStacksCleanRootRetryWidenMergeBeforeStackCap(t *testing.T) {
	tree := &Tree{
		language: &Language{Name: "go"},
		root: &Node{
			endByte: 96,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopNoStacksAlive,
			ExpectedEOFByte: 128,
			RootEndByte:     96,
			Truncated:       true,
			MaxStacksSeen:   3,
		},
	}

	if got := fullParseRetryMergePerKeyOverride(tree, 128, 8); got != fullParseRetryMaxMergePerKey {
		t.Fatalf("fullParseRetryMergePerKeyOverride(clean EOF no_stacks) = %d, want %d", got, fullParseRetryMaxMergePerKey)
	}
}

func TestShouldTakeCleanWideRetryRejectsNodeLimitNonTruncatedNoError(t *testing.T) {
	incumbent := &Tree{
		root: &Node{
			endByte: 64,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopNoStacksAlive,
			ExpectedEOFByte: 128,
			RootEndByte:     64,
		},
	}
	candidate := &Tree{
		root: &Node{
			endByte: 128,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopNodeLimit,
			ExpectedEOFByte: 128,
			RootEndByte:     128,
		},
	}

	if shouldTakeCleanWideRetry(incumbent, candidate, 128, 8) {
		t.Fatal("shouldTakeCleanWideRetry(node_limit non-truncated clean candidate) = true, want false")
	}
}

func TestShouldTakeCleanWideRetryAcceptsAcceptedNoError(t *testing.T) {
	incumbent := &Tree{
		root: &Node{
			endByte: 64,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopNoStacksAlive,
			ExpectedEOFByte: 128,
			RootEndByte:     64,
		},
	}
	candidate := &Tree{
		root: &Node{
			endByte: 128,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 128,
			RootEndByte:     128,
		},
	}

	if !shouldTakeCleanWideRetry(incumbent, candidate, 128, 8) {
		t.Fatal("shouldTakeCleanWideRetry(accepted clean candidate) = false, want true")
	}
}

func TestShouldTakeCleanWideRetryNoStacksAliveRequiresExpectedEOFCoverage(t *testing.T) {
	incumbent := &Tree{
		root: &Node{
			endByte: 64,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopNoStacksAlive,
			ExpectedEOFByte: 128,
			RootEndByte:     64,
		},
	}
	candidate := &Tree{
		root: &Node{
			endByte: 127,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopNoStacksAlive,
			ExpectedEOFByte: 128,
			RootEndByte:     127,
		},
	}

	if shouldTakeCleanWideRetry(incumbent, candidate, 128, 8) {
		t.Fatal("shouldTakeCleanWideRetry(no_stacks_alive short clean candidate) = true, want false")
	}
	candidate.root.endByte = 128
	candidate.parseRuntime.RootEndByte = 128
	if !shouldTakeCleanWideRetry(incumbent, candidate, 128, 8) {
		t.Fatal("shouldTakeCleanWideRetry(no_stacks_alive EOF-covering clean candidate) = false, want true")
	}
}

func TestShouldRepeatExternalScannerFullParseHonorsLanguagePolicy(t *testing.T) {
	tree := &Tree{
		root: &Node{
			flags: nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason: ParseStopAccepted,
		},
	}
	scanner := parserTestUnsafeExternalScanner{}

	for _, name := range []string{"dart", "kotlin", "python"} {
		lang := &Language{Name: name, ExternalScanner: scanner}
		if !shouldRepeatExternalScannerFullParse(lang, tree) {
			t.Fatalf("shouldRepeatExternalScannerFullParse(%s default accepted error) = false, want true", name)
		}
		lang.ExternalScannerFullParseRetryPolicy = ExternalScannerFullParseRetrySkipRepeat
		if shouldRepeatExternalScannerFullParse(lang, tree) {
			t.Fatalf("shouldRepeatExternalScannerFullParse(%s certified accepted error) = true, want false", name)
		}
	}
	if !shouldRepeatExternalScannerFullParse(&Language{Name: "ruby", ExternalScanner: scanner}, tree) {
		t.Fatal("shouldRepeatExternalScannerFullParse(ruby accepted error) = false, want true")
	}
}

func TestRetryFullParseStopsSchedulingRetriesAfterTimeout(t *testing.T) {
	parser := &Parser{timeoutMicros: 500}
	source := []byte("1+")
	initial := &Tree{
		root: &Node{
			endByte: 1,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: uint32(len(source)),
			MaxStacksSeen:   2,
			NodesAllocated:  20,
		},
	}
	retry := &Tree{
		root: &Node{
			endByte: 2,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: uint32(len(source)),
			MaxStacksSeen:   2,
			NodesAllocated:  10,
		},
	}
	calls := 0

	got := parser.retryFullParse(source, 2, initial, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
		calls++
		if calls != 1 {
			t.Fatalf("runRetry called %d times, want exactly one retry before timeout cutoff", calls)
		}
		if maxMergePerKeyOverride == 0 {
			t.Fatalf("first retry maxMergePerKeyOverride = 0, want initial merge retry")
		}
		time.Sleep(2 * time.Millisecond)
		return retry
	})

	if got != retry {
		t.Fatalf("retryFullParse returned %p, want retry tree %p", got, retry)
	}
	if calls != 1 {
		t.Fatalf("runRetry calls = %d, want 1", calls)
	}
	lang := &Language{
		ExternalScanner:                     parserTestUnsafeExternalScanner{},
		ExternalScannerFullParseRetryPolicy: ExternalScannerFullParseRetrySkipRepeat,
	}
	if shouldRepeatExternalScannerFullParse(lang, got) {
		t.Fatal("certified first-ladder selected tree scheduled a second external-scanner retry")
	}
}

func TestParseForRecoveryReusesRecoveryParser(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	tree, err := parser.parseForRecovery([]byte("1+2"))
	if err != nil {
		t.Fatalf("first parseForRecovery error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("first parseForRecovery returned nil tree/root")
	}
	tree.Release()

	first := parser.recoveryParser
	if first == nil {
		t.Fatal("recoveryParser = nil after first parseForRecovery")
	}
	if !first.skipRecoveryReparse {
		t.Fatal("recoveryParser.skipRecoveryReparse = false, want true")
	}

	tree, err = parser.parseForRecovery([]byte("3+4"))
	if err != nil {
		t.Fatalf("second parseForRecovery error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("second parseForRecovery returned nil tree/root")
	}
	tree.Release()

	if parser.recoveryParser != first {
		t.Fatal("parseForRecovery did not reuse recoveryParser instance")
	}
}

func TestResetSnippetParserClearsTransientState(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	parser.reparseFactory = func(source []byte) (TokenSource, error) { return nil, nil }
	parser.recoveryParser = NewParser(buildArithmeticLanguage())
	parser.skipRecoveryReparse = true
	parser.fullArenaHint = 123
	parser.compactFullArenaHint = 456
	parser.included = []Range{{StartByte: 1, EndByte: 2}}
	parser.logger = func(kind ParserLogType, message string) {}
	parser.glrTrace = true
	parser.timeoutMicros = 99
	flag := uint32(1)
	parser.cancellationFlag = &flag
	parser.parseBudgetDepth = 1
	parser.parseDeadline = time.Now()
	parser.parseStoppedReason = ParseStopTimeout

	resetSnippetParser(parser)

	if parser.reparseFactory != nil {
		t.Fatal("resetSnippetParser did not clear reparseFactory")
	}
	if parser.recoveryParser != nil {
		t.Fatal("resetSnippetParser did not clear recoveryParser")
	}
	if parser.skipRecoveryReparse {
		t.Fatal("resetSnippetParser did not clear skipRecoveryReparse")
	}
	if parser.fullArenaHint != 0 {
		t.Fatal("resetSnippetParser did not clear fullArenaHint")
	}
	if parser.compactFullArenaHint != 0 {
		t.Fatal("resetSnippetParser did not clear compactFullArenaHint")
	}
	if len(parser.included) != 0 {
		t.Fatal("resetSnippetParser did not clear included ranges")
	}
	if parser.logger != nil {
		t.Fatal("resetSnippetParser did not clear logger")
	}
	if parser.glrTrace {
		t.Fatal("resetSnippetParser did not clear glrTrace")
	}
	if parser.timeoutMicros != 0 {
		t.Fatal("resetSnippetParser did not clear timeoutMicros")
	}
	if parser.cancellationFlag != nil {
		t.Fatal("resetSnippetParser did not clear cancellationFlag")
	}
	if parser.parseBudgetDepth != 0 {
		t.Fatal("resetSnippetParser did not clear parseBudgetDepth")
	}
	if !parser.parseDeadline.IsZero() {
		t.Fatal("resetSnippetParser did not clear parseDeadline")
	}
	if parser.parseStoppedReason != ParseStopNone {
		t.Fatal("resetSnippetParser did not clear parseStoppedReason")
	}
}

func TestParseWithSnippetParserParsesSource(t *testing.T) {
	tree, err := parseWithSnippetParser(buildArithmeticLanguage(), []byte("1+2"))
	if err != nil {
		t.Fatalf("parseWithSnippetParser error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parseWithSnippetParser returned nil tree/root")
	}
	tree.Release()
}

func TestParseWithSnippetParserInheritsExpiredParentDeadline(t *testing.T) {
	parent := NewParser(buildArithmeticLanguage())
	parent.SetTimeoutMicros(100)
	endBudget := parent.beginParseOperationBudget()
	defer endBudget()
	time.Sleep(2 * time.Millisecond)

	tree, err := parseWithSnippetParserInheriting(buildArithmeticLanguage(), []byte("1+2"), parent)
	if err != nil {
		t.Fatalf("parseWithSnippetParserInheriting error: %v", err)
	}
	defer tree.Release()
	if got, want := tree.ParseStopReason(), ParseStopTimeout; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
}

func TestParseWithSnippetParserInheritsParentCancellation(t *testing.T) {
	parent := NewParser(buildArithmeticLanguage())
	var cancelled uint32 = 1
	parent.SetCancellationFlag(&cancelled)
	endBudget := parent.beginParseOperationBudget()
	defer endBudget()

	tree, err := parseWithSnippetParserInheriting(buildArithmeticLanguage(), []byte("1+2"), parent)
	if err != nil {
		t.Fatalf("parseWithSnippetParserInheriting error: %v", err)
	}
	defer tree.Release()
	if got, want := tree.ParseStopReason(), ParseStopCancelled; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
}

func TestParserParseClearsRecoveryParserAcrossTopLevelParses(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	parser.recoveryParser = NewParser(buildArithmeticLanguage())

	if _, err := parser.Parse([]byte("1+2")); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parser.recoveryParser != nil {
		t.Fatal("Parse retained recoveryParser after top-level parse")
	}
}

func TestPreferRetryTreePrefersFurtherAcceptedProgress(t *testing.T) {
	incumbent := &Tree{
		root: &Node{
			endByte:  100,
			flags:    nodeFlagHasError,
			children: []*Node{{}, {}, {}},
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopNoStacksAlive,
			ExpectedEOFByte: 200,
			Truncated:       true,
		},
	}
	candidate := &Tree{
		root: &Node{
			endByte:  200,
			flags:    nodeFlagHasError,
			children: []*Node{{}, {}},
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 200,
		},
	}

	if !preferRetryTree(nil, candidate, incumbent) {
		t.Fatal("preferRetryTree = false, want true for accepted full-length retry")
	}
}

func TestPreferRetryTreeKeepsFurtherErrorOverShortCleanCandidate(t *testing.T) {
	incumbent := &Tree{
		root: &Node{
			endByte: 200,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 200,
		},
	}
	candidate := &Tree{
		root: &Node{
			endByte: 120,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 200,
		},
	}

	if preferRetryTree(nil, candidate, incumbent) {
		t.Fatal("preferRetryTree = true, want false for short clean retry against further error incumbent")
	}
}

func TestPreferRetryTreePrefersFullCleanCandidateOverError(t *testing.T) {
	incumbent := &Tree{
		root: &Node{
			endByte: 200,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 200,
		},
	}
	candidate := &Tree{
		root: &Node{
			endByte: 200,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 200,
		},
	}

	if !preferRetryTree(nil, candidate, incumbent) {
		t.Fatal("preferRetryTree = false, want true for full clean retry against error incumbent")
	}
}

func TestPreferRetryTreePrefersFewerChildrenOnEqualErrorTrees(t *testing.T) {
	incumbent := &Tree{
		root: &Node{
			endByte:  200,
			flags:    nodeFlagHasError,
			children: make([]*Node, 12),
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 200,
			NodesAllocated:  1200,
		},
	}
	candidate := &Tree{
		root: &Node{
			endByte:  200,
			flags:    nodeFlagHasError,
			children: make([]*Node, 4),
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 200,
			NodesAllocated:  800,
		},
	}

	if !preferRetryTree(nil, candidate, incumbent) {
		t.Fatal("preferRetryTree = false, want true for smaller equal-span error tree")
	}
}

func TestGLRStackCullTrigger(t *testing.T) {
	if got := glrStackCullTrigger(8, arenaClassFull, "go"); got != 12 {
		t.Fatalf("glrStackCullTrigger(full, go) = %d, want 12", got)
	}
	if got := glrStackCullTrigger(8, arenaClassFull, "c_sharp"); got != 8 {
		t.Fatalf("glrStackCullTrigger(full, c_sharp) = %d, want 8", got)
	}
	if got := glrStackCullTrigger(8, arenaClassIncremental, "go"); got != 8 {
		t.Fatalf("glrStackCullTrigger(incremental, go) = %d, want 8", got)
	}
	maxInt := int(^uint(0) >> 1)
	if got := glrStackCullTrigger(maxInt, arenaClassFull, "go"); got != maxInt {
		t.Fatalf("glrStackCullTrigger(maxInt) = %d, want %d", got, maxInt)
	}
}

func TestResolveParseMaxStacks(t *testing.T) {
	if got, retry := resolveParseMaxStacks(6, 0, 2); got != 6 || retry {
		t.Fatalf("resolveParseMaxStacks(default) = (%d, %t), want (6, false)", got, retry)
	}
	if got, retry := resolveParseMaxStacks(6, 2, 2); got != 2 || retry {
		t.Fatalf("resolveParseMaxStacks(low override) = (%d, %t), want (2, false)", got, retry)
	}
	if got, retry := resolveParseMaxStacks(6, 32, 2); got != 32 || !retry {
		t.Fatalf("resolveParseMaxStacks(retry widen) = (%d, %t), want (32, true)", got, retry)
	}
	if got, retry := resolveParseMaxStacks(6, 2, 4); got != 4 || retry {
		t.Fatalf("resolveParseMaxStacks(conflict floor) = (%d, %t), want (4, false)", got, retry)
	}
}

func TestEffectiveFullParseInitialMaxStacks(t *testing.T) {
	// bash's historical 256 floor was removed by the 2026-07 cliff campaign:
	// it papered over the shallow-depth cull ordering evicting the accept
	// lineage (see compareStackCullKeys), cost every bash full parse 256
	// survivors x 256 merge-per-key, and did not even prevent the defect
	// (first pass still error-wrapped at caps 2..256). With the cull fix the
	// global default parses the bash corpus clean and byte-shape-identical to
	// the pinned C oracle.
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "bash"}, maxGLRStacks); got != maxGLRStacks {
		t.Fatalf("effectiveFullParseInitialMaxStacks(bash) = %d, want %d (global default)", got, maxGLRStacks)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "css"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(css) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "scss"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(scss) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "hcl"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(hcl) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "objc"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(objc) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "crystal"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(crystal) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "groovy"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(groovy) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "d"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(d) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "javascript"}, maxGLRStacks); got != 6 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(javascript) = %d, want 6", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "java"}, maxGLRStacks); got != 14 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(java) = %d, want 14", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "typescript"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(typescript) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "tsx"}, maxGLRStacks); got != 6 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(tsx) = %d, want 6", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "dart"}, maxGLRStacks); got != 6 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(dart) = %d, want 6", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "python"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(python) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "rust"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(rust) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "comment"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(comment) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "php"}, maxGLRStacks); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(php) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "go"}, maxGLRStacks); got != 32 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(go) = %d, want 32", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "markdown"}, maxGLRStacks); got != 5 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(markdown) = %d, want 5", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "markdown_inline"}, maxGLRStacks); got != 4 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(markdown_inline) = %d, want 4", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "css"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(css, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "javascript"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(javascript, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "java"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(java, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "crystal"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(crystal, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "groovy"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(groovy, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "d"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(d, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "objc"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(objc, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "typescript"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(typescript, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "tsx"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(tsx, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "dart"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(dart, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "rust"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(rust, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "comment"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(comment, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "php"}, 32); got != 32 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(php, explicit override) = %d, want 32", got)
	}
}

func TestParseMaxMergePerKeyValue(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "3")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := parseMaxMergePerKeyValue(); got != 3 {
		t.Fatalf("parseMaxMergePerKeyValue() = %d, want 3", got)
	}
}

func TestLanguageDefersExactDedupe(t *testing.T) {
	for _, name := range []string{"dart", "java", "typescript", "tsx", "rust"} {
		if !languageDefersExactDedupe(&Language{Name: name}, false) {
			t.Fatalf("languageDefersExactDedupe(%s, full tree) = false, want true", name)
		}
		if languageDefersExactDedupe(&Language{Name: name}, true) {
			t.Fatalf("languageDefersExactDedupe(%s, no-tree) = true, want false", name)
		}
	}
	for _, name := range []string{"go", "python", "javascript"} {
		if languageDefersExactDedupe(&Language{Name: name}, false) {
			t.Fatalf("languageDefersExactDedupe(%s, full tree) = true, want false", name)
		}
	}
	if languageDefersExactDedupe(nil, false) {
		t.Fatal("languageDefersExactDedupe(nil, full tree) = true, want false")
	}
}

func TestParsePreMaterializationDiagEnabled(t *testing.T) {
	t.Setenv("GOT_GLR_V2_PRE_MATERIALIZATION_DIAG", "1")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if !parsePreMaterializationDiagEnabled() {
		t.Fatal("parsePreMaterializationDiagEnabled() = false, want true")
	}
}

func TestEffectiveParseMergePerKeyCap(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	t.Setenv("GOT_FAITHFUL_CONDENSE", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := effectiveParseMergePerKeyCap(&Language{Name: "javascript"}, maxStacksPerMergeKey, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(javascript, default, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "go"}, maxStacksPerMergeKey, false); got != 3 {
		t.Fatalf("effectiveParseMergePerKeyCap(go, default, full) = %d, want 3", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "starlark"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(starlark, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "elixir"}, maxStacksPerMergeKey, false); got != 2 {
		t.Fatalf("effectiveParseMergePerKeyCap(elixir, default, full) = %d, want 2", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "typescript"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(typescript, default, full) = %d, want 1", got)
	}
	// The tight TypeScript cap applies uniformly regardless of source size:
	// the former >64KB disengagement retained redundant unreduced-spine
	// survivors whose frontier walks exploded transient stack population on
	// large union-list .d.ts sources (intra-token population explosion).
	if got := effectiveParseMergePerKeyCap(&Language{Name: "typescript"}, maxStacksPerMergeKey, false, 128*1024); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(typescript, large default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "tsx"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(tsx, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "tsx"}, maxStacksPerMergeKey, false, 128*1024); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(tsx, large default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(java, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, maxStacksPerMergeKey, false, javaTightMergeCapSourceLen); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(java, large default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, maxStacksPerMergeKey, false); got != 3 {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, default, full) = %d, want 3", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "c"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(c, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "cpp"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(cpp, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "json"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(json, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "kotlin"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(kotlin, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "scheme"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(scheme, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "php"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(php, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "sql"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(sql, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "r"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(r, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "scala"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(scala, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "powershell"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(powershell, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "graphql"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(graphql, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "haskell"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(haskell, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "make"}, maxStacksPerMergeKey, false); got != 2 {
		t.Fatalf("effectiveParseMergePerKeyCap(make, default, full) = %d, want 2", got)
	}
	// Lua's table-constructor field list (field (sep field)* sep?) needs two
	// same-key survivors at each separator, or the trailing-separator branch is
	// pruned and a clean parse degrades into recovery under the DFA lexer path.
	if got := effectiveParseMergePerKeyCap(&Language{Name: "lua"}, maxStacksPerMergeKey, false); got != 2 {
		t.Fatalf("effectiveParseMergePerKeyCap(lua, default, full) = %d, want 2", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "ruby"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(ruby, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "rust"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(rust, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "svelte"}, maxStacksPerMergeKey, false); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(svelte, default, full) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "xml"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(xml, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "toml"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(toml, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "nix"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(nix, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "ocaml"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(ocaml, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "json"}, 1, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(json, 1, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "kotlin"}, 1, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(kotlin, 1, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "javascript"}, 2, false); got != 2 {
		t.Fatalf("effectiveParseMergePerKeyCap(javascript, 2, full) = %d, want 2", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, 3, false); got != 3 {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, 3, full) = %d, want 3", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "json"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(json, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "kotlin"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(kotlin, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "scheme"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(scheme, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "javascript"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(javascript, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "starlark"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(starlark, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "elixir"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(elixir, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "typescript"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(typescript, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(java, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "c"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(c, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "cpp"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(cpp, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "tsx"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(tsx, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, maxStacksPerMergeKey, true, dartIncrementalReuseMaxSourceBytes); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, small incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, maxStacksPerMergeKey, true, dartIncrementalReuseMaxSourceBytes+1); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, large incremental fallback) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "php"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(php, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "sql"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(sql, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "r"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(r, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "scala"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(scala, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "powershell"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(powershell, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "graphql"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(graphql, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "haskell"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(haskell, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "make"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(make, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "lua"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(lua, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "ruby"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(ruby, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "rust"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(rust, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "svelte"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(svelte, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "xml"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(xml, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "toml"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(toml, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "nix"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(nix, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "ocaml"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(ocaml, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
}

func TestEffectiveParseMergePerKeyCapElixirFaithfulCondense(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	t.Setenv("GOT_FAITHFUL_CONDENSE", "1")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := effectiveParseMergePerKeyCap(&Language{Name: "elixir"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(elixir, faithful default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "elixir"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(elixir, faithful default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
}

func TestEffectiveParseMergePerKeyCapGoFaithfulCondense(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	t.Setenv("GOT_FAITHFUL_CONDENSE", "1")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := effectiveParseMergePerKeyCap(&Language{Name: "go"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(go, faithful default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "go"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(go, faithful default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
}

func TestConfigureParseCapsTypedArrowKeepsTypedArrowWidthOnLargeTypeScript(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	// TypeScript's tight full-parse merge cap now applies uniformly regardless
	// of source size (the former >64KB disengagement caused the intra-token
	// population explosion on large union-list sources). A large typed-arrow
	// source therefore gets the same typed-arrow minimum width (2) that small
	// typed-arrow sources have always used, rather than the former wide
	// default of maxStacksPerMergeKey.
	source := []byte(strings.Repeat("const filler = 1;\n", 8192) + "const f = (str: string) => str;\n")
	parser := &Parser{language: &Language{Name: "typescript"}}
	var scratch parserScratch

	caps := parser.configureParseCaps(source, nil, arenaClassFull, &scratch, 0, 0, 0)
	if caps.mergePerKeyCap != 2 {
		t.Fatalf("configureParseCaps(large TypeScript typed arrow) merge cap = %d, want 2", caps.mergePerKeyCap)
	}

	caps = parser.configureParseCaps(source, nil, arenaClassFull, &scratch, 0, 0, -4)
	if caps.mergePerKeyCap != 4 {
		t.Fatalf("configureParseCaps(large TypeScript typed arrow, exact retry cap) merge cap = %d, want 4", caps.mergePerKeyCap)
	}
}

func TestEffectiveParseMergePerKeyCapJavaExplicitOverride(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "4")
	t.Setenv("GOT_FAITHFUL_CONDENSE", "1")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(java, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "c"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(c, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "cpp"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(cpp, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "rust"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(rust, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "nix"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(nix, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "ocaml"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(ocaml, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "scheme"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(scheme, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "elixir"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(elixir, faithful explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "go"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(go, faithful explicit, full) = %d, want 4", got)
	}
}

func TestEffectiveParseMergePerKeyCapDartExplicitOverride(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "8")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, 8, false); got != 8 {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, explicit, full) = %d, want 8", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, 8, true, dartIncrementalReuseMaxSourceBytes+1); got != 8 {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, explicit, large incremental fallback) = %d, want 8", got)
	}
}

func TestErrorCostCompetitionLanguageRequiresCapabilityByDefault(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")
	if errorCostCompetitionLanguage(&Language{
		CRecoveryCostCompetitionCapable:          true,
		Name:                                     "scheme",
		CRecoveryCostCompetitionEnabledByDefault: true,
	}) {
		t.Fatal("errorCostCompetitionLanguage enabled without table capability")
	}

	t.Setenv("GOT_C_RECOVERY", "scheme")
	if errorCostCompetitionLanguage(&Language{Name: "scheme"}) {
		t.Fatal("GOT_C_RECOVERY=scheme enabled without table capability")
	}
	lang := cRecoveryGateLanguage()
	lang.Name = "scheme"
	lang.CRecoveryCostCompetitionEnabledByDefault = false
	if !errorCostCompetitionLanguage(lang) {
		t.Fatal("GOT_C_RECOVERY=scheme did not force-enable diagnostic gate with runtime capability")
	}

	t.Setenv("GOT_C_RECOVERY", "0")
	if errorCostCompetitionLanguage(&Language{
		CRecoveryCostCompetitionCapable:          true,
		Name:                                     "scheme",
		CRecoveryCostCompetitionEnabledByDefault: true,
	}) {
		t.Fatal("GOT_C_RECOVERY=0 did not disable C recovery")
	}
}

func TestJavaAnnotationInterfaceSourceUsesWideMergeCap(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if !javaFullParseNeedsAnnotationDeclarationMergeWidth(&Language{Name: "java"}, []byte("@interface Demo {}"), nil) {
		t.Fatal("javaFullParseNeedsAnnotationDeclarationMergeWidth = false, want true")
	}
	if javaFullParseNeedsAnnotationDeclarationMergeWidth(&Language{Name: "java"}, []byte(`class Demo { @Generated("x") void f() {} }`), nil) {
		t.Fatal("javaFullParseNeedsAnnotationDeclarationMergeWidth(@Generated) = true, want false")
	}
	if javaFullParseNeedsAnnotationDeclarationMergeWidth(&Language{Name: "java"}, []byte("class Demo {}"), nil) {
		t.Fatal("javaFullParseNeedsAnnotationDeclarationMergeWidth(class) = true, want false")
	}
	if javaFullParseNeedsAnnotationDeclarationMergeWidth(&Language{Name: "java"}, []byte("@interface Demo {}"), &reuseCursor{}) {
		t.Fatal("javaFullParseNeedsAnnotationDeclarationMergeWidth(incremental) = true, want false")
	}
}

func TestNoteRepeatedReduceChainSignatureDetectsCycle(t *testing.T) {
	sig := reduceChainSignature{
		state:        2016,
		depth:        171,
		symbol:       216,
		childCount:   1,
		productionID: 42,
	}
	var prev reduceChainSignature
	count := 0
	cycle := false
	for i := 0; i <= maxRepeatedReduceChainSignature; i++ {
		prev, count, cycle = noteRepeatedReduceChainSignature(prev, count, sig)
	}
	if !cycle {
		t.Fatal("noteRepeatedReduceChainSignature did not report a repeated cycle")
	}
	if prev != sig {
		t.Fatalf("noteRepeatedReduceChainSignature signature = %+v, want %+v", prev, sig)
	}
	if count != maxRepeatedReduceChainSignature+1 {
		t.Fatalf("noteRepeatedReduceChainSignature count = %d, want %d", count, maxRepeatedReduceChainSignature+1)
	}
}

func TestNoteRepeatedReduceChainSignatureResetsOnChange(t *testing.T) {
	first := reduceChainSignature{state: 10, depth: 3, symbol: 7, childCount: 1, productionID: 2}
	second := reduceChainSignature{state: 11, depth: 3, symbol: 7, childCount: 1, productionID: 2}

	prev, count, cycle := noteRepeatedReduceChainSignature(reduceChainSignature{}, 0, first)
	if cycle || count != 1 || prev != first {
		t.Fatalf("first signature = (%+v, %d, %t), want (%+v, 1, false)", prev, count, cycle, first)
	}

	prev, count, cycle = noteRepeatedReduceChainSignature(prev, count, second)
	if cycle {
		t.Fatal("changed signature incorrectly reported a cycle")
	}
	if count != 1 || prev != second {
		t.Fatalf("changed signature = (%+v, %d), want (%+v, 1)", prev, count, second)
	}
}

func TestRecoverReduceChainCycle(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var entryScratch glrEntryScratch
	var gssScratch gssScratch

	t.Run("pushes and extends error node", func(t *testing.T) {
		source := []byte("     abcdef")
		s := newGLRStack(lang.InitialState)
		nodeCount := 0
		trackChildErrors := false

		ok := parser.recoverReduceChainCycle(source, &s, lang.InitialState, Token{
			Symbol:     3,
			StartByte:  5,
			EndByte:    8,
			StartPoint: Point{Row: 0, Column: 5},
			EndPoint:   Point{Row: 0, Column: 8},
		}, &nodeCount, arena, &entryScratch, &gssScratch, &trackChildErrors)
		if !ok {
			t.Fatal("recoverReduceChainCycle returned false, want true")
		}
		if got, want := nodeCount, 1; got != want {
			t.Fatalf("nodeCount after push = %d, want %d", got, want)
		}
		if got, want := s.byteOffset, uint32(8); got != want {
			t.Fatalf("stack byteOffset after push = %d, want %d", got, want)
		}
		top := stackEntryNode(s.top())
		if top == nil || top.symbol != errorSymbol || !top.hasError() {
			t.Fatalf("top node after push = %+v, want ERROR node", top)
		}
		if !trackChildErrors {
			t.Fatal("trackChildErrors = false, want true")
		}
		depthAfterPush := s.depth()

		ok = parser.recoverReduceChainCycle(source, &s, lang.InitialState, Token{
			Symbol:     3,
			StartByte:  8,
			EndByte:    11,
			StartPoint: Point{Row: 0, Column: 8},
			EndPoint:   Point{Row: 0, Column: 11},
		}, &nodeCount, arena, &entryScratch, &gssScratch, &trackChildErrors)
		if !ok {
			t.Fatal("recoverReduceChainCycle extend returned false, want true")
		}
		if got, want := nodeCount, 1; got != want {
			t.Fatalf("nodeCount after extend = %d, want %d", got, want)
		}
		if got, want := s.depth(), depthAfterPush; got != want {
			t.Fatalf("stack depth after extend = %d, want %d", got, want)
		}
		if got, want := s.byteOffset, uint32(11); got != want {
			t.Fatalf("stack byteOffset after extend = %d, want %d", got, want)
		}
		top = stackEntryNode(s.top())
		if top == nil || top.symbol != errorSymbol || top.endByte != 11 {
			t.Fatalf("top node after extend = %+v, want ERROR through byte 11", top)
		}
	})

	t.Run("ignores eof and no-lookahead", func(t *testing.T) {
		source := []byte("    ")
		for _, tc := range []struct {
			name string
			tok  Token
		}{
			{name: "eof", tok: Token{Symbol: 0, StartByte: 5, EndByte: 5}},
			{name: "no-lookahead", tok: Token{Symbol: 3, StartByte: 5, EndByte: 8, NoLookahead: true}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				s := newGLRStack(lang.InitialState)
				s.byteOffset = 4
				nodeCount := 7
				beforeDepth := s.depth()

				ok := parser.recoverReduceChainCycle(source, &s, lang.InitialState, tc.tok, &nodeCount, arena, &entryScratch, &gssScratch, nil)
				if ok {
					t.Fatal("recoverReduceChainCycle returned true, want false")
				}
				if got, want := nodeCount, 7; got != want {
					t.Fatalf("nodeCount = %d, want %d", got, want)
				}
				if got, want := s.depth(), beforeDepth; got != want {
					t.Fatalf("stack depth = %d, want %d", got, want)
				}
				if got, want := s.byteOffset, uint32(4); got != want {
					t.Fatalf("stack byteOffset = %d, want %d", got, want)
				}
			})
		}
	})

	t.Run("pauses eof cycle for c recovery", func(t *testing.T) {
		parser := NewParser(lang)
		parser.errorCostCompetition = true
		s := newGLRStack(lang.InitialState)
		s.byteOffset = 4
		nodeCount := 7
		beforeDepth := s.depth()

		ok := parser.recoverReduceChainCycle([]byte("    "), &s, lang.InitialState, Token{
			Symbol:    0,
			StartByte: 4,
			EndByte:   4,
		}, &nodeCount, arena, &entryScratch, &gssScratch, nil)
		if ok {
			t.Fatal("recoverReduceChainCycle returned true, want false because EOF is not consumed")
		}
		if !s.cPaused {
			t.Fatal("stack was not paused for C EOF recovery")
		}
		if got, want := nodeCount, 7; got != want {
			t.Fatalf("nodeCount = %d, want %d", got, want)
		}
		if got, want := s.depth(), beforeDepth; got != want {
			t.Fatalf("stack depth = %d, want %d", got, want)
		}
		if got, want := s.byteOffset, uint32(4); got != want {
			t.Fatalf("stack byteOffset = %d, want %d", got, want)
		}
	})

	t.Run("rejects skipped comment gap before attaching token", func(t *testing.T) {
		source := []byte("1/*c*/234")
		s := newGLRStack(lang.InitialState)
		s.byteOffset = 1
		nodeCount := 7
		beforeDepth := s.depth()

		ok := parser.recoverReduceChainCycle(source, &s, lang.InitialState, Token{
			Symbol:     3,
			StartByte:  6,
			EndByte:    9,
			StartPoint: Point{Row: 0, Column: 6},
			EndPoint:   Point{Row: 0, Column: 9},
		}, &nodeCount, arena, &entryScratch, &gssScratch, nil)
		if ok {
			t.Fatal("recoverReduceChainCycle returned true, want false for non-padding gap")
		}
		if !s.dead {
			t.Fatal("stack.dead = false, want true")
		}
		if got, want := nodeCount, 7; got != want {
			t.Fatalf("nodeCount = %d, want %d", got, want)
		}
		if got, want := s.depth(), beforeDepth; got != want {
			t.Fatalf("stack depth = %d, want %d", got, want)
		}
	})
}

func TestTryResyncErrorRecoveryRejectsAdvanceCommentGap(t *testing.T) {
	lang := &Language{
		Name:              "c",
		SymbolCount:       4,
		TokenCount:        3,
		StateCount:        2,
		InitialState:      0,
		ProductionIDCount: 1,
		SymbolNames:       []string{"EOF", "TOK", "item", "root"},
		SymbolMetadata:    []SymbolMetadata{{Name: "EOF"}, {Name: "TOK", Visible: true, Named: true}, {Name: "item", Visible: true, Named: true}, {Name: "root", Visible: true, Named: true}},
		ParseActions:      []ParseActionEntry{{}},
		ParseTable:        [][]uint16{{0, 0, 0, 0}, {0, 0, 0, 0}},
		LexModes:          []LexMode{{LexState: 0}, {LexState: 0}},
		LexStates:         []LexState{{Default: -1, EOF: -1}},
	}
	parser := NewParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	source := []byte("x/*c*/y")
	s := newGLRStack(lang.InitialState)
	failed := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	failed.setHasError(true)
	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	parser.pushStackNode(&s, 1, failed, &entryScratch, &gssScratch)
	nodeCount := 0

	status := parser.tryResyncErrorRecovery(source, &s, Token{
		Symbol:     1,
		StartByte:  6,
		EndByte:    7,
		StartPoint: Point{Column: 6},
		EndPoint:   Point{Column: 7},
	}, &nodeCount, arena, &entryScratch, &gssScratch, nil)
	if status != resyncNone {
		t.Fatalf("tryResyncErrorRecovery status = %d, want resyncNone for non-padding gap", status)
	}
	if !s.dead {
		t.Fatal("stack.dead = false, want true")
	}
	if got, want := nodeCount, 0; got != want {
		t.Fatalf("nodeCount = %d, want %d", got, want)
	}
}

func TestShouldNormalizeIncrementalReturnedTree(t *testing.T) {
	root := &Node{symbol: 1}
	oldTree := &Tree{root: root}
	reusedTree := &Tree{root: root}
	newRootTree := &Tree{root: &Node{symbol: 1}}

	if shouldNormalizeIncrementalReturnedTree(nil, oldTree) {
		t.Fatal("shouldNormalizeIncrementalReturnedTree(nil, oldTree) = true, want false")
	}
	if shouldNormalizeIncrementalReturnedTree(reusedTree, oldTree) {
		t.Fatal("shouldNormalizeIncrementalReturnedTree(reusedTree, oldTree) = true, want false")
	}
	if !shouldNormalizeIncrementalReturnedTree(newRootTree, oldTree) {
		t.Fatal("shouldNormalizeIncrementalReturnedTree(newRootTree, oldTree) = false, want true")
	}
	if !shouldNormalizeIncrementalReturnedTree(reusedTree, nil) {
		t.Fatal("shouldNormalizeIncrementalReturnedTree(reusedTree, nil) = false, want true")
	}
}

func TestLanguageSupportsIncrementalReuse(t *testing.T) {
	if languageSupportsIncrementalReuse(nil) {
		t.Fatal("languageSupportsIncrementalReuse(nil) = true, want false")
	}
	if !languageSupportsIncrementalReuse(&Language{}) {
		t.Fatal("languageSupportsIncrementalReuse(no scanner) = false, want true")
	}
	if languageSupportsIncrementalReuse(&Language{ExternalScanner: parserTestUnsafeExternalScanner{}}) {
		t.Fatal("languageSupportsIncrementalReuse(unsafe scanner) = true, want false")
	}
	if !languageSupportsIncrementalReuse(&Language{ExternalScanner: parserTestSafeExternalScanner{}}) {
		t.Fatal("languageSupportsIncrementalReuse(safe scanner) = false, want true")
	}
}

func TestIncrementalReuseUnavailableReason(t *testing.T) {
	if got := incrementalReuseUnavailableReason(nil); got != "token_source_nil" {
		t.Fatalf("incrementalReuseUnavailableReason(nil) = %q, want %q", got, "token_source_nil")
	}
	unsafeTS := &dfaTokenSource{language: &Language{ExternalScanner: parserTestUnsafeExternalScanner{}}}
	if got := incrementalReuseUnavailableReason(unsafeTS); got != "external_scanner_unsupported" {
		t.Fatalf("incrementalReuseUnavailableReason(unsafe external scanner) = %q, want %q", got, "external_scanner_unsupported")
	}
	safeTS := &dfaTokenSource{language: &Language{ExternalScanner: parserTestSafeExternalScanner{}}}
	if got := incrementalReuseUnavailableReason(safeTS); got != "" {
		t.Fatalf("incrementalReuseUnavailableReason(safe external scanner) = %q, want empty", got)
	}

	dartSmallTS := &dfaTokenSource{
		language: &Language{Name: "dart", ExternalScanner: parserTestSafeExternalScanner{}},
		lexer:    &Lexer{source: make([]byte, dartIncrementalReuseMaxSourceBytes)},
	}
	if !tokenSourceSupportsIncrementalReuse(dartSmallTS) {
		t.Fatal("tokenSourceSupportsIncrementalReuse(small dart safe scanner) = false, want true")
	}
	if got := incrementalReuseUnavailableReason(dartSmallTS); got != "" {
		t.Fatalf("incrementalReuseUnavailableReason(small dart safe scanner) = %q, want empty", got)
	}

	dartLargeTS := &dfaTokenSource{
		language: &Language{Name: "dart", ExternalScanner: parserTestSafeExternalScanner{}},
		lexer:    &Lexer{source: make([]byte, dartIncrementalReuseMaxSourceBytes+1)},
	}
	if tokenSourceSupportsIncrementalReuse(dartLargeTS) {
		t.Fatal("tokenSourceSupportsIncrementalReuse(large dart safe scanner) = true, want false")
	}
	if got := incrementalReuseUnavailableReason(dartLargeTS); got != "dart_large_external_scanner_unsupported" {
		t.Fatalf("incrementalReuseUnavailableReason(large dart safe scanner) = %q, want %q", got, "dart_large_external_scanner_unsupported")
	}
}

func TestParseFullArenaNodeCapacityCapsStaleLargeHintBySourceSize(t *testing.T) {
	sourceLen := 32 * 1024
	staleLargeHint := parseNodeLimit(2 * 1024 * 1024)

	got := parseFullArenaNodeCapacity(sourceLen, staleLargeHint)
	limit := parseFullArenaHintLimit(sourceLen)
	if got != limit {
		t.Fatalf("parseFullArenaNodeCapacity(%d, stale large hint) = %d, want source-sized limit %d", sourceLen, got, limit)
	}
	if got >= staleLargeHint {
		t.Fatalf("parseFullArenaNodeCapacity kept stale large hint: got %d, stale hint %d", got, staleLargeHint)
	}
}

func TestParseFullArenaNodeCapacityKeepsUsefulSameSizeHint(t *testing.T) {
	sourceLen := 128 * 1024
	initial := parseFullArenaInitialNodeCapacity(sourceLen)
	limit := parseFullArenaHintLimit(sourceLen)
	if initial >= limit {
		t.Fatalf("test setup invalid: initial=%d limit=%d", initial, limit)
	}
	hint := initial + (limit-initial)/2

	got := parseFullArenaNodeCapacity(sourceLen, hint)
	if got != hint {
		t.Fatalf("parseFullArenaNodeCapacity(%d, useful hint %d) = %d, want hint", sourceLen, hint, got)
	}
}

func TestParseFullArenaInitialNodeCapacityScalesForLargeSources(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	got := parseFullArenaInitialNodeCapacity(sourceLen)
	want := 1_500_000
	if got != want {
		t.Fatalf("parseFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParseFullArenaNodeCapacityCapsLargeTypeScriptFourslashCommentFixture(t *testing.T) {
	source := []byte("/// <reference path=\"fourslash.ts\" />\n////var route = [")
	source = append(source, bytes.Repeat([]byte("[51.1,-0.2],"), 1024*1024/12)...)
	source = append(source, []byte("];\nverify.completions({ marker: \"\", excludes: [\"\"] });\n")...)

	got := parseFullArenaNodeCapacityForSource(source, &Language{Name: "typescript"}, 0)
	if got != typeScriptFourslashLargeCommentArenaNodeCap {
		t.Fatalf("parseFullArenaNodeCapacityForSource(fourslash) = %d, want %d", got, typeScriptFourslashLargeCommentArenaNodeCap)
	}
}

func TestParseFullArenaNodeCapacityKeepsLargeSourceFloorForNonFourslash(t *testing.T) {
	source := bytes.Repeat([]byte("let x = 1;\n"), 128*1024)
	got := parseFullArenaNodeCapacityForSource(source, &Language{Name: "typescript"}, 0)
	want := parseFullArenaNodeCapacity(len(source), 0)
	if got != want {
		t.Fatalf("parseFullArenaNodeCapacityForSource(non-fourslash) = %d, want %d", got, want)
	}
}

func TestParseFullArenaNodeCapacityKeepsLargeSourceFloorForOtherLanguages(t *testing.T) {
	source := []byte("/// <reference path=\"fourslash.ts\" />\n////var route = [")
	source = append(source, bytes.Repeat([]byte("[51.1,-0.2],"), 1024*1024/12)...)
	got := parseFullArenaNodeCapacityForSource(source, &Language{Name: "python"}, 0)
	want := parseFullArenaNodeCapacity(len(source), 0)
	if got != want {
		t.Fatalf("parseFullArenaNodeCapacityForSource(other lang) = %d, want %d", got, want)
	}
}

func TestParseFullArenaInitialNodeCapacityPreallocatesMediumSources(t *testing.T) {
	sourceLen := 192 * 1024
	got := parseFullArenaInitialNodeCapacity(sourceLen)
	want := sourceLen * 2 / 3
	if got != want {
		t.Fatalf("parseFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParsePendingFullArenaInitialNodeCapacityUsesLowerLargeSourceFloor(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	got := parsePendingFullArenaInitialNodeCapacity(sourceLen)
	want := sourceLen / 2
	if got != want {
		t.Fatalf("parsePendingFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParsePendingFullArenaInitialNodeCapacityCapsHugeSourceFloor(t *testing.T) {
	sourceLen := 3 * 1024 * 1024
	got := parsePendingFullArenaInitialNodeCapacity(sourceLen)
	want := 1_050_000
	if got != want {
		t.Fatalf("parsePendingFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParseCompactFullArenaInitialNodeCapacityUsesCompactLargeSourceFloor(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	got := parseCompactFullArenaInitialNodeCapacity(sourceLen)
	want := sourceLen / 4
	if got != want {
		t.Fatalf("parseCompactFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParseCompactFullArenaInitialNodeCapacityCapsHugeSourceFloor(t *testing.T) {
	sourceLen := 4 * 1024 * 1024
	got := parseCompactFullArenaInitialNodeCapacity(sourceLen)
	want := 750_000
	if got != want {
		t.Fatalf("parseCompactFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParseFinalChildRefArenaInitialNodeCapacityUsesSmallerFloor(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	got := parseFinalChildRefArenaInitialNodeCapacity(sourceLen)
	want := 64 * 1024
	if got != want {
		t.Fatalf("parseFinalChildRefArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParsePendingFullArenaNodeCapacityUsesCloseWarmHint(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	initial := parsePendingFullArenaInitialNodeCapacity(sourceLen)
	hint := initial - initial/16
	got := parsePendingFullArenaNodeCapacity(sourceLen, hint)
	if got != hint {
		t.Fatalf("parsePendingFullArenaNodeCapacity(%d, %d) = %d, want hint", sourceLen, hint, got)
	}
}

func TestParseCompactFullArenaNodeCapacityUsesWarmHintBelowInitial(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	initial := parseCompactFullArenaInitialNodeCapacity(sourceLen)
	hint := initial * 3 / 4
	got := parseCompactFullArenaNodeCapacity(sourceLen, hint)
	if got != hint {
		t.Fatalf("parseCompactFullArenaNodeCapacity(%d, %d) = %d, want hint", sourceLen, hint, got)
	}
}

func TestParseCompactFullArenaNodeCapacityRejectsTinyStaleHint(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	initial := parseCompactFullArenaInitialNodeCapacity(sourceLen)
	hint := initial/2 - 1
	got := parseCompactFullArenaNodeCapacity(sourceLen, hint)
	if got != initial {
		t.Fatalf("parseCompactFullArenaNodeCapacity(%d, %d) = %d, want initial %d", sourceLen, hint, got, initial)
	}
}

func TestParsePendingFullArenaHintHeadroomIsTighterForLargeSources(t *testing.T) {
	used := 1_200_000
	got := parsePendingFullArenaHintHeadroom(used)
	want := 32 * 1024
	if got != want {
		t.Fatalf("parsePendingFullArenaHintHeadroom(%d) = %d, want %d", used, got, want)
	}
}

func TestParseCompactFullArenaHintHeadroomIsTighterForLargeSources(t *testing.T) {
	used := 1_200_000
	got := parseCompactFullArenaHintHeadroom(used)
	want := 32 * 1024
	if got != want {
		t.Fatalf("parseCompactFullArenaHintHeadroom(%d) = %d, want %d", used, got, want)
	}
}

func TestParseShouldUsePendingFullParentsDefaultsForLargePythonNoCompat(t *testing.T) {
	source := make([]byte, 256*1024)
	parser := &Parser{
		language:                           &Language{Name: "python"},
		noResultCompatibilityBenchmarkOnly: true,
	}

	if !parseShouldUsePendingFullParents(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUsePendingFullParents = false, want true for large Python no-compat")
	}

	t.Setenv("GOT_GLR_V2_PENDING_PARENTS", "0")
	if parseShouldUsePendingFullParents(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUsePendingFullParents = true, want explicit env disable")
	}
}

func TestParseShouldUsePendingFullParentsKeepsEnvOptInForOtherLargeSources(t *testing.T) {
	source := make([]byte, 256*1024)
	parser := &Parser{
		language: &Language{Name: "java"},
	}

	if parseShouldUsePendingFullParents(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUsePendingFullParents = true, want false without env for Java")
	}

	t.Setenv("GOT_GLR_V2_PENDING_PARENTS", "1")
	if !parseShouldUsePendingFullParents(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUsePendingFullParents = false, want env opt-in")
	}
}

func TestParseShouldUseCompactFullShiftLeavesDefaultsForLargePythonNoCompat(t *testing.T) {
	source := make([]byte, 256*1024)
	parser := &Parser{
		language:                           &Language{Name: "python"},
		noResultCompatibilityBenchmarkOnly: true,
	}

	if !parseShouldUseCompactFullShiftLeaves(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseCompactFullShiftLeaves = false, want true for large Python no-compat")
	}

	t.Setenv("GOT_GLR_V2_COMPACT_FULL_LEAVES", "0")
	if parseShouldUseCompactFullShiftLeaves(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseCompactFullShiftLeaves = true, want explicit env disable")
	}
}

func TestParseShouldUseCompactFullShiftLeavesKeepsEnvOptInForOtherLargeSources(t *testing.T) {
	source := make([]byte, 256*1024)
	parser := &Parser{
		language:                           &Language{Name: "java"},
		noResultCompatibilityBenchmarkOnly: true,
	}

	if parseShouldUseCompactFullShiftLeaves(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseCompactFullShiftLeaves = true, want false without env for Java")
	}

	t.Setenv("GOT_GLR_V2_COMPACT_FULL_LEAVES", "1")
	if !parseShouldUseCompactFullShiftLeaves(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseCompactFullShiftLeaves = false, want env opt-in")
	}

	parser.noResultCompatibilityBenchmarkOnly = false
	if parseShouldUseCompactFullShiftLeaves(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseCompactFullShiftLeaves = true, want no-compat-only gate")
	}
}

func TestParseShouldUseFinalChildRefsDefaultsForLargePythonNoCompat(t *testing.T) {
	source := make([]byte, 256*1024)
	parser := &Parser{
		language:                           &Language{Name: "python"},
		pendingFullParents:                 true,
		noResultCompatibilityBenchmarkOnly: true,
	}
	if !parseShouldUseFinalChildRefs(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseFinalChildRefs = false, want default large Python no-compat pending full parse")
	}

	parser.pendingFullParents = false
	if parseShouldUseFinalChildRefs(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseFinalChildRefs = true, want pending-parent gate")
	}

	parser.pendingFullParents = true
	parser.noResultCompatibilityBenchmarkOnly = false
	if parseShouldUseFinalChildRefs(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseFinalChildRefs = true, want no-compat-only gate")
	}

	parser.noResultCompatibilityBenchmarkOnly = true
	t.Setenv("GOT_GLR_V2_FINAL_CHILD_REFS", "0")
	if parseShouldUseFinalChildRefs(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseFinalChildRefs = true, want explicit env disable")
	}
}

func TestParserShouldDeferResultParentLinksForNoCompatBenchmark(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	root := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	parser := &Parser{
		language:                           &Language{Name: "go"},
		noResultCompatibilityBenchmarkOnly: true,
	}
	if !parser.shouldDeferResultParentLinks(root) {
		t.Fatal("shouldDeferResultParentLinks = false, want true for no-compat benchmark parse")
	}

	parser.noTreeBenchmarkOnly = true
	if parser.shouldDeferResultParentLinks(root) {
		t.Fatal("shouldDeferResultParentLinks = true, want false for no-tree benchmark parse")
	}

	parser.noTreeBenchmarkOnly = false
	parser.noResultCompatibilityBenchmarkOnly = false
	if parser.shouldDeferResultParentLinks(root) {
		t.Fatal("shouldDeferResultParentLinks = true, want false for normal Go parse")
	}

	parser.language = &Language{Name: "java"}
	if !parser.shouldDeferResultParentLinks(root) {
		t.Fatal("shouldDeferResultParentLinks = false, want true for normal Java parse")
	}
}

func TestParseFullEntryScratchCapacityCapsLargePrealloc(t *testing.T) {
	got := parseFullEntryScratchCapacity(2 * 1024 * 1024)
	want := 64 * 1024
	if got != want {
		t.Fatalf("parseFullEntryScratchCapacity large source = %d, want %d", got, want)
	}
}

func TestParseFullArenaHintHeadroomIsBoundedForLargeSources(t *testing.T) {
	used := 1_500_000
	got := parseFullArenaHintHeadroom(used)
	want := 64 * 1024
	if got != want {
		t.Fatalf("parseFullArenaHintHeadroom(%d) = %d, want %d", used, got, want)
	}
}

func TestParseFullArenaHintHeadroomIsTighterForMediumSources(t *testing.T) {
	used := 128 * 1024
	got := parseFullArenaHintHeadroom(used)
	want := used / 16
	if got != want {
		t.Fatalf("parseFullArenaHintHeadroom(%d) = %d, want %d", used, got, want)
	}
}

func TestParseFullExternalScannerCheckpointCapacityUsesNodeHeadroom(t *testing.T) {
	const nodeCapacity = 1_500_000
	const sourceLen = 2 * 1024 * 1024
	got := parseFullExternalScannerCheckpointCapacity(sourceLen, nodeCapacity)
	want := sourceLen * 3 / 8
	if got != want {
		t.Fatalf("parseFullExternalScannerCheckpointCapacity = %d, want %d", got, want)
	}
	if got := parseFullExternalScannerCheckpointCapacity(8*1024*1024, nodeCapacity); got != nodeCapacity {
		t.Fatalf("capped checkpoint capacity = %d, want node capacity %d", got, nodeCapacity)
	}
	if got := parseFullExternalScannerCheckpointCapacity(256*1024-1, nodeCapacity); got != 0 {
		t.Fatalf("small-source checkpoint capacity = %d, want 0", got)
	}
}

func TestParseShouldSkipInvisibleFullLeafCheckpointsIsNarrow(t *testing.T) {
	parser := &Parser{
		language:                           &Language{Name: "python"},
		noResultCompatibilityBenchmarkOnly: true,
	}
	largeSource := make([]byte, 256*1024)
	if !parseShouldSkipInvisibleFullLeafCheckpoints(parser, largeSource, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldSkipInvisibleFullLeafCheckpoints = false, want true for large Python no-compat full parse")
	}
	parser.noResultCompatibilityBenchmarkOnly = false
	if parseShouldSkipInvisibleFullLeafCheckpoints(parser, largeSource, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldSkipInvisibleFullLeafCheckpoints = true for normal parse")
	}
	parser.noResultCompatibilityBenchmarkOnly = true
	if parseShouldSkipInvisibleFullLeafCheckpoints(parser, largeSource[:len(largeSource)-1], nil, nil, arenaClassFull) {
		t.Fatal("parseShouldSkipInvisibleFullLeafCheckpoints = true for sub-threshold source")
	}
	if parseShouldSkipInvisibleFullLeafCheckpoints(parser, largeSource, nil, nil, arenaClassIncremental) {
		t.Fatal("parseShouldSkipInvisibleFullLeafCheckpoints = true for incremental arena")
	}
}

func TestParseShouldCaptureFullMaterializationTimingIsNarrow(t *testing.T) {
	parser := &Parser{language: &Language{Name: "python"}}
	largeSource := make([]byte, 256*1024)
	if !parseShouldCaptureFullMaterializationTiming(parser, largeSource, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldCaptureFullMaterializationTiming = false, want true for large Python full parse")
	}
	if parseShouldCaptureFullMaterializationTiming(parser, largeSource[:len(largeSource)-1], nil, nil, arenaClassFull) {
		t.Fatal("parseShouldCaptureFullMaterializationTiming = true for sub-threshold source")
	}
	if parseShouldCaptureFullMaterializationTiming(parser, largeSource, nil, nil, arenaClassIncremental) {
		t.Fatal("parseShouldCaptureFullMaterializationTiming = true for incremental arena")
	}
	parser.language.Name = "go"
	if parseShouldCaptureFullMaterializationTiming(parser, largeSource, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldCaptureFullMaterializationTiming = true for non-Python language")
	}
}

func TestParseShouldCaptureMaterializationTimingEnv(t *testing.T) {
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	parser := &Parser{language: &Language{Name: "go"}}
	source := []byte("package p\n")
	if !parseShouldCaptureMaterializationTiming(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldCaptureMaterializationTiming = false, want env-enabled full timing")
	}
	if !parseShouldCaptureMaterializationTiming(parser, source, &reuseCursor{}, nil, arenaClassIncremental) {
		t.Fatal("parseShouldCaptureMaterializationTiming = false, want env-enabled incremental timing")
	}
	parser.noTreeBenchmarkOnly = true
	if parseShouldCaptureMaterializationTiming(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldCaptureMaterializationTiming = true for no-tree benchmark mode")
	}
}
