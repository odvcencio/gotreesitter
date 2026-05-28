package gotreesitter

import (
	"bytes"
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

func TestTypeScriptRepetitionShiftConflictChoiceAllowsHotProgramRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "function", "identifier", "const", "return", "if", "export", "program_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 7, ChildCount: 2},
		{Type: ParseActionShift, State: 3693, Repetition: true},
	}

	for _, sym := range []Symbol{1, 2, 3, 4, 5, 6} {
		chosen, ok := typescriptRepetitionShiftConflictChoice(lang, Token{Symbol: sym}, 9, actions)
		if !ok {
			t.Fatalf("typescriptRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 3693 || !chosen.Repetition {
			t.Fatalf("typescriptRepetitionShiftConflictChoice(%q) picked %+v, want repetition shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestTypeScriptRepetitionShiftConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "function", "program_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 3693, Repetition: true},
	}

	if _, ok := typescriptRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 10, actions); ok {
		t.Fatal("typescriptRepetitionShiftConflictChoice = true, want false")
	}
}

func TestTSXRepetitionReduceConflictChoiceAllowsHotRepeats(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", ";", "_jsx_start_opening_element_repeat1", "object_type_repeat1", "const", "let", "program_repeat1"}}
	for _, tc := range []struct {
		name      string
		state     StateID
		lookahead Symbol
		reduceSym Symbol
	}{
		{name: "jsx opening element", state: 3468, lookahead: 1, reduceSym: 3},
		{name: "object type semicolon", state: 3885, lookahead: 2, reduceSym: 4},
		{name: "program const", state: 9, lookahead: 5, reduceSym: 7},
		{name: "program let", state: 9, lookahead: 6, reduceSym: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actions := []ParseAction{
				{Type: ParseActionReduce, Symbol: tc.reduceSym, ChildCount: 2},
				{Type: ParseActionShift, State: 3552, Repetition: true},
			}
			chosen, ok := tsxRepetitionReduceConflictChoice(lang, Token{Symbol: tc.lookahead}, tc.state, actions)
			if !ok {
				t.Fatal("tsxRepetitionReduceConflictChoice = false, want true")
			}
			if chosen.Type != ParseActionReduce || chosen.Symbol != tc.reduceSym {
				t.Fatalf("tsxRepetitionReduceConflictChoice picked %+v, want reduce", chosen)
			}
		})
	}
}

func TestTSXRepetitionReduceConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "_jsx_start_opening_element_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 3552, Repetition: true},
	}
	if _, ok := tsxRepetitionReduceConflictChoice(lang, Token{Symbol: 1}, 3469, actions); ok {
		t.Fatal("tsxRepetitionReduceConflictChoice = true, want false")
	}
}

func TestTSXRepetitionReduceConflictChoiceRejectsWrongReduce(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "_jsx_start_opening_element_repeat1", "wrong_repeat"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 3552, Repetition: true},
	}
	if _, ok := tsxRepetitionReduceConflictChoice(lang, Token{Symbol: 1}, 3468, actions); ok {
		t.Fatal("tsxRepetitionReduceConflictChoice = true, want false")
	}
}

func TestRustRepetitionShiftConflictChoiceAllowsTopLevelItemStarts(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "pub", "#", "impl", "fn", "mod", "use", "source_file_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 7, ChildCount: 2},
		{Type: ParseActionShift, State: 2039, Repetition: true},
	}

	for _, sym := range []Symbol{1, 2, 3, 4, 5, 6} {
		chosen, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: sym}, 7, actions)
		if !ok {
			t.Fatalf("rustRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 2039 || !chosen.Repetition {
			t.Fatalf("rustRepetitionShiftConflictChoice(%q) picked %+v, want repetition shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestRustRepetitionShiftConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "pub", "source_file_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 2039, Repetition: true},
	}

	if _, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 8, actions); ok {
		t.Fatal("rustRepetitionShiftConflictChoice = true, want false")
	}
}

func TestRustRepetitionShiftConflictChoiceAllowsTokenTreeRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", ",", "(", "primitive_type", "::", ".", ";", "delim_token_tree_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 8, ChildCount: 2},
		{Type: ParseActionShift, State: 246, Repetition: true},
	}

	for _, sym := range []Symbol{1, 2, 3, 4, 5, 6, 7} {
		chosen, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: sym}, 83, actions)
		if !ok {
			t.Fatalf("rustRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 246 || !chosen.Repetition {
			t.Fatalf("rustRepetitionShiftConflictChoice(%q) picked %+v, want repetition shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestJavaRepetitionShiftConflictChoiceAllowsStringLiteralContinuation(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "escape_sequence", "string_fragment", "_string_literal_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionShift, State: 983, Repetition: true},
	}

	for _, sym := range []Symbol{1, 2} {
		chosen, ok := javaRepetitionShiftConflictChoice(lang, nil, Token{Symbol: sym}, 983, actions)
		if !ok {
			t.Fatalf("javaRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 983 || !chosen.Repetition {
			t.Fatalf("javaRepetitionShiftConflictChoice(%q) picked %+v, want string repeat shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestJavaRepetitionShiftConflictChoiceRejectsOtherStringLiteralLookahead(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "_string_literal_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1},
		{Type: ParseActionShift, State: 983, Repetition: true},
	}

	if _, ok := javaRepetitionShiftConflictChoice(lang, nil, Token{Symbol: 1}, 983, actions); ok {
		t.Fatal("javaRepetitionShiftConflictChoice = true, want false")
	}
}

func TestJavaRepetitionShiftConflictChoiceAllowsArrayInitializerSeparator(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ",", "array_initializer_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 145, Repetition: true},
	}
	source := []byte(`class T { int[] values = { 1, /* keep going */ 2 }; }`)
	comma := uint32(bytes.IndexByte(source, ',') + 1)

	chosen, ok := javaRepetitionShiftConflictChoice(lang, source, Token{Symbol: 1, EndByte: comma}, 1104, actions)
	if !ok {
		t.Fatal("javaRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 145 || !chosen.Repetition {
		t.Fatalf("javaRepetitionShiftConflictChoice picked %+v, want array initializer comma shift", chosen)
	}
}

func TestJavaRepetitionShiftConflictChoiceRejectsArrayInitializerTrailingComma(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ",", "array_initializer_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 145, Repetition: true},
	}
	source := []byte("class T { int[] values = { 1, // trailing\n}; }")
	comma := uint32(bytes.IndexByte(source, ',') + 1)

	if _, ok := javaRepetitionShiftConflictChoice(lang, source, Token{Symbol: 1, EndByte: comma}, 1104, actions); ok {
		t.Fatal("javaRepetitionShiftConflictChoice = true, want false for trailing comma")
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
	if shouldRunInitialFullParseMergeRetry(nil) {
		t.Fatal("shouldRunInitialFullParseMergeRetry(nil) = true, want false")
	}
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason: ParseStopNodeLimit,
		},
	}
	if shouldRunInitialFullParseMergeRetry(tree) {
		t.Fatal("shouldRunInitialFullParseMergeRetry(node_limit) = true, want false")
	}
	tree.parseRuntime.StopReason = ParseStopNoStacksAlive
	if !shouldRunInitialFullParseMergeRetry(tree) {
		t.Fatal("shouldRunInitialFullParseMergeRetry(no_stacks_alive) = false, want true")
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

	if !preferRetryTree(candidate, incumbent) {
		t.Fatal("preferRetryTree = false, want true for accepted full-length retry")
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

	if !preferRetryTree(candidate, incumbent) {
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
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "bash"}, maxGLRStacks); got != 256 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(bash) = %d, want 256", got)
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
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "javascript"}, maxGLRStacks); got != 6 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(javascript) = %d, want 6", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "typescript"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(typescript) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "tsx"}, maxGLRStacks); got != 6 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(tsx) = %d, want 6", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "python"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(python) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "rust"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(rust) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "go"}, maxGLRStacks); got != 32 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(go) = %d, want 32", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "markdown"}, maxGLRStacks); got != 4 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(markdown) = %d, want 4", got)
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
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "typescript"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(typescript, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "tsx"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(tsx, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "rust"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(rust, explicit override) = %d, want 16", got)
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
	if got := effectiveParseMergePerKeyCap(&Language{Name: "typescript"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(typescript, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "typescript"}, maxStacksPerMergeKey, false, 128*1024); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(typescript, large default, full) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "tsx"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(tsx, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "tsx"}, maxStacksPerMergeKey, false, 128*1024); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(tsx, large default, full) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(java, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, maxStacksPerMergeKey, false, javaTightMergeCapSourceLen); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(java, large default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "c"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(c, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "json"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(json, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "kotlin"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(kotlin, default, full) = %d, want 1", got)
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
	if got := effectiveParseMergePerKeyCap(&Language{Name: "json"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(json, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "kotlin"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(kotlin, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "javascript"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(javascript, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "starlark"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(starlark, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
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
	if got := effectiveParseMergePerKeyCap(&Language{Name: "tsx"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(tsx, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
}

func TestEffectiveParseMergePerKeyCapJavaExplicitOverride(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "4")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(java, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "c"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(c, explicit, full) = %d, want 4", got)
	}
}

func TestJavaAnnotationInterfaceSourceUsesWideMergeCap(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if !javaFullParseNeedsAnnotationDeclarationMergeWidth(&Language{Name: "java"}, []byte("@interface Demo {}"), nil) {
		t.Fatal("javaFullParseNeedsAnnotationDeclarationMergeWidth = false, want true")
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
