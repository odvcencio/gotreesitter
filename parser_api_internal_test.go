package gotreesitter

import (
	"testing"
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

func TestPreferRetryTreePrefersFurtherAcceptedProgress(t *testing.T) {
	incumbent := &Tree{
		root: &Node{
			endByte:  100,
			hasError: true,
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
			hasError: true,
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
			hasError: true,
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
			hasError: true,
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
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "javascript"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(javascript) = %d, want 2", got)
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
