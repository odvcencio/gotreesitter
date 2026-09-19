//go:build gts_recovery_telemetry

package gotreesitter

import (
	"errors"
	"testing"
)

func seedRecoveryRuntimeDetailedForTest(parser *Parser) {
	tree := &Tree{}
	cold := parser.ensureParserColdState()
	cold.recoveryRuntime = recoveryRuntimeTelemetry{
		stats: RecoveryRuntimeStats{
			Enabled:              true,
			Completed:            true,
			RecoveryEntryCount:   7,
			RetrySelectedAttempt: "stale",
		},
		pendingRetryReason: "stale",
	}
	cold.recoveryRuntimeDetailed = &recoveryRuntimeDetailedState{
		attempts: RecoveryRuntimeAttempts{{
			Ordinal:           3,
			Rung:              "stale",
			Cause:             "stale",
			CandidateSelected: true,
		}},
		byTree: map[*Tree]int{tree: 0},
	}
}

func requireRecoveryRuntimeDetailedCleared(t *testing.T, parser *Parser) {
	t.Helper()
	if got := parser.DebugRecoveryRuntimeStats(); got != (RecoveryRuntimeStats{}) {
		t.Fatalf("selected-tree receipt = %+v, want zero", got)
	}
	if got := parser.DebugRecoveryRuntimeAttempts(); len(got) != 0 {
		t.Fatalf("attempt receipt = %+v, want empty", got)
	}
}

func requireRecoveryRuntimeDetailedStorageCleared(t *testing.T, parser *Parser) {
	t.Helper()
	if parser == nil || parser.forestDeclineMemo == nil {
		return
	}
	cold := parser.forestDeclineMemo
	if cold.recoveryRuntime.stats != (RecoveryRuntimeStats{}) ||
		cold.recoveryRuntime.pendingRetryReason != "" ||
		cold.recoveryRuntime.retryAttempts != nil {
		t.Fatalf("regular recovery state retained after pool release: %+v", cold.recoveryRuntime)
	}
	if cold.recoveryRuntimeDetailed != nil {
		t.Fatalf("detailed recovery state retained after pool release: %+v", cold.recoveryRuntimeDetailed)
	}
}

func seedRecoveryRuntimeDetailedAttemptForTest(parser *Parser, tree *Tree) {
	parser.beginRecoveryRuntimeTelemetry()
	parser.beginRecoveryRuntimeTelemetryDetailed()
	runtime := *tree.rawParseRuntime()
	parser.finishRecoveryRuntimeTelemetry(tree, nil)
	parser.finishRecoveryRuntimeTelemetryDetailed(tree, &runtime)
	parser.recordRecoveryRuntimeRetryTree(tree, "initial")
	parser.recordRecoveryRuntimeRetryTreeDetailed(tree, "initial", "initial_parse")
}

func requireOneSelectedRecoveryRuntimeAttempt(t *testing.T, parser *Parser, wantRung string) RecoveryRuntimeAttemptStats {
	t.Helper()
	attempts := parser.DebugRecoveryRuntimeAttempts()
	if len(attempts) != 1 {
		t.Fatalf("attempt count = %d, want 1: %+v", len(attempts), attempts)
	}
	if !attempts[0].CandidateSelected || attempts[0].Rung != wantRung {
		t.Fatalf("selected attempt = %+v, want selected rung %q", attempts[0], wantRung)
	}
	return attempts[0]
}

func detailedAcceptedErrorTreeForTest(language *Language, sourceLen int) *Tree {
	return &Tree{
		language: language,
		root:     &Node{endByte: uint32(sourceLen), flags: nodeFlagHasError},
		parseRuntime: &ParseRuntime{
			StopReason:       ParseStopAccepted,
			SourceLen:        uint32(sourceLen),
			ExpectedEOFByte:  uint32(sourceLen),
			RootEndByte:      uint32(sourceLen),
			LastTokenEndByte: uint32(sourceLen),
			LastTokenWasEOF:  true,
			MaxStacksSeen:    8,
		},
	}
}

func detailedCSharpNamespaceRecoveryTreeForTest() (*Tree, []byte) {
	source := []byte("namespace N {}")
	language := &Language{
		Name:        "c_sharp",
		SymbolNames: []string{"EOF", "compilation_unit", "namespace_declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "compilation_unit", Visible: true, Named: true},
			{Name: "namespace_declaration", Visible: true, Named: true},
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	namespace := newLeafNodeInArena(arena, 2, true, 0, uint32(len(source)), Point{}, Point{})
	namespace.setHasError(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{namespace}, nil, 0)
	return &Tree{
		language: language,
		root:     root,
		arena:    arena,
		parseRuntime: &ParseRuntime{
			StopReason:       ParseStopAccepted,
			SourceLen:        uint32(len(source)),
			ExpectedEOFByte:  uint32(len(source)),
			RootEndByte:      uint32(len(source)),
			LastTokenEndByte: uint32(len(source)),
			LastTokenWasEOF:  true,
			MaxStacksSeen:    64,
		},
	}, source
}

func TestRecoveryRuntimeDetailedResetsEarlyFailures(t *testing.T) {
	EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { EnableRecoveryRuntimeTelemetry(false) })

	factoryError := errors.New("factory failed")
	tests := []struct {
		name   string
		parser *Parser
		parse  func(*Parser) error
		want   error
	}{
		{
			name:   "language",
			parser: &Parser{},
			parse: func(parser *Parser) error {
				_, err := parser.Parse(nil)
				return err
			},
			want: ErrNoLanguage,
		},
		{
			name:   "token source",
			parser: NewParser(buildArithmeticLanguage()),
			parse: func(parser *Parser) error {
				_, err := parser.ParseWithTokenSource(nil, nil)
				return err
			},
			want: ErrNoTokenSource,
		},
		{
			name:   "factory",
			parser: NewParser(buildArithmeticLanguage()),
			parse: func(parser *Parser) error {
				_, err := parser.ParseWithTokenSourceFactory(nil, func([]byte) (TokenSource, error) {
					return nil, factoryError
				})
				return err
			},
			want: factoryError,
		},
		{
			name:   "UTF-16",
			parser: NewParser(buildArithmeticLanguage()),
			parse: func(parser *Parser) error {
				_, err := parser.ParseUTF16Bytes([]byte{0x31}, UTF16LittleEndian)
				return err
			},
			want: ErrInvalidUTF16ByteLength,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seedRecoveryRuntimeDetailedForTest(test.parser)
			if err := test.parse(test.parser); !errors.Is(err, test.want) {
				t.Fatalf("parse error = %v, want %v", err, test.want)
			}
			requireRecoveryRuntimeDetailedCleared(t, test.parser)
		})
	}

	forestLanguage := buildArithmeticLanguage()
	forestLanguage.NativeUnaryWrapperFlattening = []UnaryWrapperFlatteningRule{{}}
	forestParser := NewParser(forestLanguage)
	seedRecoveryRuntimeDetailedForTest(forestParser)
	if tree, ok := forestParser.ParseForestExperimental(nil); ok || tree != nil {
		t.Fatalf("forest parse = tree:%v ok:%v, want nil and false", tree, ok)
	}
	requireRecoveryRuntimeDetailedCleared(t, forestParser)
}

func TestRecoveryRuntimeDetailedKeepsAttemptsSeparateFromSelectedTree(t *testing.T) {
	EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { EnableRecoveryRuntimeTelemetry(false) })

	parser := &Parser{}
	initialRoot := &Node{endByte: 4, symbol: errorSymbol}
	initialRoot.setHasError(true)
	initial := &Tree{root: initialRoot}
	losing := &Tree{root: &Node{endByte: 4}}

	parser.beginRecoveryRuntimeTelemetry()
	parser.beginRecoveryRuntimeTelemetryDetailed()
	parser.recordRecoveryEntry()
	initialRuntime := &ParseRuntime{StopReason: ParseStopAccepted, SourceLen: 4, ExpectedEOFByte: 4, ParseWallNanos: 10}
	parser.finishRecoveryRuntimeTelemetry(initial, nil)
	parser.finishRecoveryRuntimeTelemetryDetailed(initial, initialRuntime)
	parser.recordRecoveryRuntimeRetryTree(initial, "initial")
	parser.recordRecoveryRuntimeRetryTreeDetailed(initial, "initial", "initial_parse")
	parser.recordRecoveryRuntimeSelectedTree(initial)
	parser.recordRecoveryRuntimeSelectedTreeDetailed(initial)

	parser.fullParseRetryPassesTaken = 1
	parser.recordRecoveryRuntimeRetry("boundary_retry")
	parser.beginRecoveryRuntimeTelemetry()
	parser.beginRecoveryRuntimeTelemetryDetailed()
	parser.recordRecoveryEntry()
	parser.recordRecoveryEntry()
	losingRuntime := &ParseRuntime{StopReason: ParseStopAccepted, SourceLen: 4, ExpectedEOFByte: 4, ParseWallNanos: 20}
	parser.finishRecoveryRuntimeTelemetry(losing, nil)
	parser.finishRecoveryRuntimeTelemetryDetailed(losing, losingRuntime)
	parser.recordRecoveryRuntimeRetryTree(losing, "final_merge")
	parser.recordRecoveryRuntimeRetryTreeDetailed(losing, "final_merge", "boundary_retry")
	parser.finishRecoveryRuntimeRetryTelemetry(initial, 4)

	stats := parser.DebugRecoveryRuntimeStats()
	attempts := parser.DebugRecoveryRuntimeAttempts()
	if stats.RecoveryEntryCount != 1 || stats.RetrySelectedAttempt != "initial" {
		t.Fatalf("selected-tree receipt = %+v, want initial attempt", stats)
	}
	if len(attempts) != 2 || attempts[0].RecoveryEntryCount != 1 || attempts[1].RecoveryEntryCount != 2 {
		t.Fatalf("attempt receipt = %+v, want separate 1-entry and 2-entry attempts", attempts)
	}
	if !attempts[0].CandidateSelected || attempts[1].CandidateSelected {
		t.Fatalf("attempt selection = %+v, want only initial selected", attempts)
	}

	parser.fullParseRetryPassesTaken = 2
	parser.recordRecoveryRuntimeRetry("pooled_pointer_retry")
	parser.beginRecoveryRuntimeTelemetry()
	parser.beginRecoveryRuntimeTelemetryDetailed()
	parser.recordRecoveryEntry()
	parser.recordRecoveryEntry()
	parser.recordRecoveryEntry()
	parser.finishRecoveryRuntimeTelemetry(losing, nil)
	parser.finishRecoveryRuntimeTelemetryDetailed(losing, &ParseRuntime{StopReason: ParseStopAccepted, SourceLen: 4, ExpectedEOFByte: 4, ParseWallNanos: 30})
	parser.recordRecoveryRuntimeRetryTree(losing, "secondary_node")
	parser.recordRecoveryRuntimeRetryTreeDetailed(losing, "secondary_node", "pooled_pointer_retry")
	parser.finishRecoveryRuntimeRetryTelemetry(initial, 4)

	stats = parser.DebugRecoveryRuntimeStats()
	attempts = parser.DebugRecoveryRuntimeAttempts()
	if len(attempts) != 3 || attempts[2].RecoveryEntryCount != 3 {
		t.Fatalf("pooled tree pointer collapsed attempt history: %+v", attempts)
	}
	if stats.RecoveryEntryCount != 1 || stats.RetrySelectedAttempt != "initial" {
		t.Fatalf("pooled losing tree replaced selected receipt: %+v", stats)
	}

	attempts[0].Rung = "mutated"
	if got := parser.DebugRecoveryRuntimeAttempts()[0].Rung; got == "mutated" {
		t.Fatal("attempt API returned mutable parser storage")
	}
}

func TestRecoveryRuntimeDetailedResetsNoEditAndParserPools(t *testing.T) {
	EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { EnableRecoveryRuntimeTelemetry(false) })

	parser := NewParser(buildArithmeticLanguage())
	source := []byte("1+2")
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	if unchanged, err := parser.ParseIncremental(source, tree); err != nil || unchanged != tree {
		t.Fatalf("no-edit parse = tree:%p err:%v, want tree:%p", unchanged, err, tree)
	}
	requireRecoveryRuntimeDetailedCleared(t, parser)

	seedRecoveryRuntimeDetailedForTest(parser)
	(&ParserPool{}).applyDefaults(parser)
	requireRecoveryRuntimeDetailedCleared(t, parser)

	seedRecoveryRuntimeDetailedForTest(parser)
	resetSnippetParser(parser)
	requireRecoveryRuntimeDetailedCleared(t, parser)
	tree.Release()
}

func TestRecoveryRuntimeDetailedPoolReleaseClearsDisabledState(t *testing.T) {
	EnableRecoveryRuntimeTelemetry(true)
	parser := NewParser(buildArithmeticLanguage())
	seedRecoveryRuntimeDetailedForTest(parser)
	EnableRecoveryRuntimeTelemetry(false)

	(&ParserPool{}).applyDefaults(parser)
	requireRecoveryRuntimeDetailedStorageCleared(t, parser)

	seedRecoveryRuntimeDetailedForTest(parser)
	resetSnippetParser(parser)
	requireRecoveryRuntimeDetailedStorageCleared(t, parser)
}

func TestRecoveryRuntimeDetailedCertifiedSkipSelectsReturnedTree(t *testing.T) {
	EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { EnableRecoveryRuntimeTelemetry(false) })

	const sourceLen = 128
	language := &Language{
		Name: "certified_skip",
		FullParseAcceptedErrorRetryProfile: FullParseAcceptedErrorRetryProfile{
			SkipFreshCompleteAcceptedErrorRetry: true,
			SkipCompleteMinSourceBytes:          sourceLen,
		},
	}
	tree := detailedAcceptedErrorTreeForTest(language, sourceLen)
	parser := &Parser{}
	seedRecoveryRuntimeDetailedAttemptForTest(parser, tree)
	got := parser.retryFullParseForOrigin(make([]byte, sourceLen), 8, tree, fullParseRetryOriginFresh,
		func(int, int, int) *Tree {
			t.Fatal("certified skip ran a retry")
			return nil
		})
	if got != tree {
		t.Fatalf("returned tree = %p, want initial tree %p", got, tree)
	}
	requireOneSelectedRecoveryRuntimeAttempt(t, parser, "initial")
}

func TestRecoveryRuntimeDetailedCSharpSkipSelectsReturnedTree(t *testing.T) {
	EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { EnableRecoveryRuntimeTelemetry(false) })

	tree, source := detailedCSharpNamespaceRecoveryTreeForTest()
	parser := &Parser{}
	seedRecoveryRuntimeDetailedAttemptForTest(parser, tree)
	got := parser.retryFullParseForOrigin(source, 8, tree, fullParseRetryOriginFresh,
		func(int, int, int) *Tree {
			t.Fatal("C# namespace recovery skip ran a retry")
			return nil
		})
	if got != tree {
		t.Fatalf("returned tree = %p, want initial tree %p", got, tree)
	}
	requireOneSelectedRecoveryRuntimeAttempt(t, parser, "initial")
	tree.Release()
}

func TestRecoveryRuntimeDetailedIncrementalNoRetrySelectsReturnedTree(t *testing.T) {
	EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { EnableRecoveryRuntimeTelemetry(false) })

	const sourceLen = 128
	tree := detailedAcceptedErrorTreeForTest(&Language{Name: "go"}, sourceLen)
	tree.ensureParseRuntime().IncrementalOldTreeReuseRoute = true
	parser := &Parser{language: tree.language}
	seedRecoveryRuntimeDetailedAttemptForTest(parser, tree)
	got := parser.retryIncrementalAcceptedErrorWithBaseMergeCap(make([]byte, sourceLen), tree, nil, nil)
	if got != tree {
		t.Fatalf("returned tree = %p, want initial tree %p", got, tree)
	}
	requireOneSelectedRecoveryRuntimeAttempt(t, parser, "initial")
}

func TestRecoveryRuntimeDetailedIncrementalTieKeepsFirstPassSelected(t *testing.T) {
	EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { EnableRecoveryRuntimeTelemetry(false) })

	const sourceLen = 128
	language := &Language{Name: "incremental_tie"}
	initial := &Tree{
		language: language,
		root:     &Node{endByte: sourceLen},
		parseRuntime: &ParseRuntime{
			StopReason:       ParseStopNoStacksAlive,
			SourceLen:        sourceLen,
			ExpectedEOFByte:  sourceLen,
			RootEndByte:      sourceLen,
			LastTokenEndByte: sourceLen,
			LastTokenWasEOF:  true,
			NodesAllocated:   2,
		},
	}
	parser := &Parser{language: language}
	seedRecoveryRuntimeDetailedAttemptForTest(parser, initial)

	got := parser.retryFullParseForOrigin(make([]byte, sourceLen), 8, initial, fullParseRetryOriginIncremental,
		func(int, int, int) *Tree {
			candidate := &Tree{
				language: language,
				root:     &Node{endByte: sourceLen},
				parseRuntime: &ParseRuntime{
					StopReason:       ParseStopNoStacksAlive,
					SourceLen:        sourceLen,
					ExpectedEOFByte:  sourceLen,
					RootEndByte:      sourceLen,
					LastTokenEndByte: sourceLen,
					LastTokenWasEOF:  true,
					NodesAllocated:   1,
				},
			}
			parser.finishRecoveryRuntimeTelemetry(candidate, nil)
			parser.finishRecoveryRuntimeTelemetryDetailed(candidate, candidate.rawParseRuntime())
			return candidate
		})
	if got != initial {
		t.Fatalf("quality-tied retry = %p, want first pass %p", got, initial)
	}
	attempts := parser.DebugRecoveryRuntimeAttempts()
	if len(attempts) < 2 || !attempts[0].CandidateSelected {
		t.Fatalf("attempt selection = %+v, want first pass selected", attempts)
	}
	for i := 1; i < len(attempts); i++ {
		if attempts[i].CandidateSelected {
			t.Fatalf("quality-tied retry attempt %d selected: %+v", i, attempts[i])
		}
	}
}

func TestRecoveryRuntimeDetailedReplacementFactsRemainSeparate(t *testing.T) {
	EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { EnableRecoveryRuntimeTelemetry(false) })

	const sourceLen = 128
	initial := detailedAcceptedErrorTreeForTest(&Language{Name: "replacement"}, sourceLen)
	candidate := detailedAcceptedErrorTreeForTest(&Language{Name: "replacement"}, sourceLen)
	parser := &Parser{}
	parser.beginRecoveryRuntimeTelemetry()
	parser.beginRecoveryRuntimeTelemetryDetailed()
	parser.recordRecoveryEntry()
	parser.finishRecoveryRuntimeTelemetry(initial, nil)
	parser.finishRecoveryRuntimeTelemetryDetailed(initial, initial.rawParseRuntime())
	parser.recordRecoveryRuntimeRetryTree(initial, "initial")
	parser.recordRecoveryRuntimeRetryTreeDetailed(initial, "initial", "initial_parse")

	parser.fullParseRetryPassesTaken = 1
	parser.beginRecoveryRuntimeTelemetry()
	parser.beginRecoveryRuntimeTelemetryDetailed()
	parser.recordRecoveryEntry()
	parser.recordRecoveryEntry()
	parser.recordRecoveryEntry()
	parser.finishRecoveryRuntimeTelemetry(candidate, nil)
	parser.finishRecoveryRuntimeTelemetryDetailed(candidate, candidate.rawParseRuntime())
	parser.recordRecoveryRuntimeRetryTree(candidate, "retry")
	parser.recordRecoveryRuntimeRetryTreeDetailed(candidate, "retry", "replacement_retry")
	parser.recordRecoveryRuntimeSelectedTree(candidate)
	parser.recordRecoveryRuntimeSelectedTreeDetailed(candidate)
	parser.recordRecoveryRuntimeCandidateReplacedDetailed(candidate)
	parser.finishRecoveryRuntimeRetryTelemetry(candidate, sourceLen)

	attempts := parser.DebugRecoveryRuntimeAttempts()
	if len(attempts) != 2 {
		t.Fatalf("attempt count = %d, want 2: %+v", len(attempts), attempts)
	}
	if attempts[0].RecoveryEntryCount != 1 || attempts[0].CandidateSelected {
		t.Fatalf("initial attempt facts = %+v, want one entry and not selected", attempts[0])
	}
	if attempts[1].RecoveryEntryCount != 3 || !attempts[1].CandidateSelected || !attempts[1].CandidateReplacedIncumbent {
		t.Fatalf("replacement attempt facts = %+v, want three entries, selected, and replaced", attempts[1])
	}
	if stats := parser.DebugRecoveryRuntimeStats(); stats.RecoveryEntryCount != 3 || stats.RetrySelectedAttempt != "retry" {
		t.Fatalf("selected-tree facts = %+v, want retry facts only", stats)
	}
}

func TestRecoveryRuntimeDetailedDisabledDoesNotWrite(t *testing.T) {
	EnableRecoveryRuntimeTelemetry(false)
	parser := NewParser(buildArithmeticLanguage())
	seedRecoveryRuntimeDetailedForTest(parser)
	beforeStats := parser.forestDeclineMemo.recoveryRuntime.stats
	beforeAttempts := parser.forestDeclineMemo.recoveryRuntimeDetailed.attempts
	if _, err := parser.ParseWithTokenSource(nil, nil); !errors.Is(err, ErrNoTokenSource) {
		t.Fatalf("parse error = %v, want ErrNoTokenSource", err)
	}
	if got := parser.forestDeclineMemo.recoveryRuntime.stats; got != beforeStats {
		t.Fatalf("disabled reset changed selected receipt: got %+v want %+v", got, beforeStats)
	}
	if got := parser.forestDeclineMemo.recoveryRuntimeDetailed.attempts; len(got) != len(beforeAttempts) {
		t.Fatalf("disabled reset changed attempt receipt: got %+v want %+v", got, beforeAttempts)
	}
}
