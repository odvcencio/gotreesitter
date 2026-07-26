package gotreesitter

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestNormalizeJavaScriptTopLevelObjectLiteralsRewritesObjectLiteral(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "statement_block", "{", "}", "labeled_statement", "statement_identifier", ":", "expression_statement", "arrow_function", "object", "pair", "property_identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "statement_block", Visible: true, Named: true},
			{Name: "{", Visible: true, Named: false},
			{Name: "}", Visible: true, Named: false},
			{Name: "labeled_statement", Visible: true, Named: true},
			{Name: "statement_identifier", Visible: true, Named: true},
			{Name: ":", Visible: true, Named: false},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "arrow_function", Visible: true, Named: true},
			{Name: "object", Visible: true, Named: true},
			{Name: "pair", Visible: true, Named: true},
			{Name: "property_identifier", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	open := newLeafNodeInArena(arena, 3, false, 0, 1, Point{}, Point{Column: 1})
	key := newLeafNodeInArena(arena, 6, true, 2, 8, Point{Column: 2}, Point{Column: 8})
	colon := newLeafNodeInArena(arena, 7, false, 8, 9, Point{Column: 8}, Point{Column: 9})
	value := newLeafNodeInArena(arena, 9, true, 10, 16, Point{Column: 10}, Point{Column: 16})
	valueStmt := newParentNodeInArena(arena, 8, true, []*Node{value}, nil, 0)
	label := newParentNodeInArena(arena, 5, true, []*Node{key, colon, valueStmt}, nil, 0)
	close := newLeafNodeInArena(arena, 4, false, 17, 18, Point{Column: 17}, Point{Column: 18})
	block := newParentNodeInArena(arena, 2, true, []*Node{open, label, close}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{block}, nil, 0)

	normalizeJavaScriptTopLevelObjectLiterals(root, lang)

	if got, want := root.children[0].Type(lang), "expression_statement"; got != want {
		t.Fatalf("root.children[0].Type = %q, want %q", got, want)
	}
	object := root.children[0].children[0]
	if got, want := object.Type(lang), "object"; got != want {
		t.Fatalf("object.Type = %q, want %q", got, want)
	}
	pair := object.children[1]
	if got, want := pair.Type(lang), "pair"; got != want {
		t.Fatalf("pair.Type = %q, want %q", got, want)
	}
	if got, want := pair.children[0].Type(lang), "property_identifier"; got != want {
		t.Fatalf("pair.children[0].Type = %q, want %q", got, want)
	}
	if got, want := pair.children[2].Type(lang), "arrow_function"; got != want {
		t.Fatalf("pair.children[2].Type = %q, want %q", got, want)
	}
}

func TestNormalizeJavaScriptTopLevelExpressionStatementBoundsSnapToChildren(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "expression_statement", "identifier", ";"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	expr := newLeafNodeInArena(arena, 3, true, 10, 20, Point{Row: 1, Column: 2}, Point{Row: 1, Column: 12})
	semi := newLeafNodeInArena(arena, 4, false, 20, 21, Point{Row: 1, Column: 12}, Point{Row: 1, Column: 13})
	stmt := newParentNodeInArena(arena, 2, true, []*Node{expr, semi}, nil, 0)
	stmt.startByte = 0
	stmt.startPoint = Point{}
	stmt.endByte = 22
	stmt.endPoint = Point{Row: 2}
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)

	normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)

	if got, want := stmt.StartByte(), uint32(10); got != want {
		t.Fatalf("stmt.StartByte = %d, want %d", got, want)
	}
	if got, want := stmt.EndByte(), uint32(21); got != want {
		t.Fatalf("stmt.EndByte = %d, want %d", got, want)
	}
	if got, want := stmt.StartPoint(), (Point{Row: 1, Column: 2}); got != want {
		t.Fatalf("stmt.StartPoint = %#v, want %#v", got, want)
	}
	if got, want := stmt.EndPoint(), (Point{Row: 1, Column: 13}); got != want {
		t.Fatalf("stmt.EndPoint = %#v, want %#v", got, want)
	}
}

// newDeferredCompatDynamicImportFixture builds a minimal "program" root with
// one childless dynamic-import leaf (named "import", symbol 2) over source
// "import". normalizeJavaScriptTypeScriptDynamicImportLeafWithSymbolChanged
// (still live — not part of the wave11 compat-sunset's six confirmed-dead
// classes) retypes it into a single-child node the same way the removed
// empty_statement leaf-retype used to, so it's the fixture below's stand-in
// observable marker for "did deferred JS/TS compat actually run".
func newDeferredCompatDynamicImportFixture() (*Language, *Node, *Node) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "program", "import", "import"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "import", Visible: true, Named: true},
			{Name: "import", Visible: true, Named: false},
		},
	}
	arena := newNodeArena(arenaClassFull)
	stmt := newLeafNodeInArena(arena, 2, true, 0, 6, Point{}, Point{Column: 6})
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)
	return lang, root, stmt
}

func TestTreeRootNodeAppliesDeferredTypeScriptCompatibility(t *testing.T) {
	lang, root, stmt := newDeferredCompatDynamicImportFixture()
	tree := newTreeWithArenas(root, []byte("import"), lang, root.ownerArena, nil)
	tree.deferResultCompatibility()

	if got := resultChildCount(stmt); got != 0 {
		t.Fatalf("import child count before RootNode = %d, want 0", got)
	}
	if tree.RootNode() != root {
		t.Fatal("RootNode returned a different root")
	}
	if got, want := resultChildCount(stmt), 1; got != want {
		t.Fatalf("import child count after RootNode = %d, want %d", got, want)
	}
	child := resultChildAt(stmt, 0)
	if child == nil {
		t.Fatal("import child is nil")
	}
	if got, want := child.Type(lang), "import"; got != want {
		t.Fatalf("import child type = %q, want %q", got, want)
	}
}

func TestTreeUTF16DescendantAppliesDeferredTypeScriptCompatibility(t *testing.T) {
	lang, root, stmt := newDeferredCompatDynamicImportFixture()
	source, sourceMap := encodeUTF16ToUTF8WithMap([]uint16{'i', 'm', 'p', 'o', 'r', 't'})
	tree := newTreeWithArenas(root, source, lang, root.ownerArena, nil)
	tree.utf16Map = sourceMap
	tree.deferResultCompatibility()

	if got := resultChildCount(stmt); got != 0 {
		t.Fatalf("import child count before UTF16 descendant = %d, want 0", got)
	}
	if got := tree.NamedDescendantForUTF16Range(0, 6); got != stmt {
		t.Fatalf("NamedDescendantForUTF16Range returned %p, want import %p", got, stmt)
	}
	if got, want := resultChildCount(stmt), 1; got != want {
		t.Fatalf("import child count after UTF16 descendant = %d, want %d", got, want)
	}
	child := resultChildAt(stmt, 0)
	if child == nil {
		t.Fatal("import child is nil")
	}
	if got, want := child.Type(lang), "import"; got != want {
		t.Fatalf("import child type = %q, want %q", got, want)
	}
}

func TestTreeRootNodeRecordsDeferredTypeScriptCompatibilityTiming(t *testing.T) {
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	lang, root, _ := newDeferredCompatDynamicImportFixture()
	tree := newTreeWithArenas(root, []byte("import"), lang, root.ownerArena, nil)
	tree.deferResultCompatibility()

	_ = tree.RootNode()
	if tree.resultErrorSummary != resultErrorSummaryClean {
		t.Fatalf("resultErrorSummary = %d, want clean", tree.resultErrorSummary)
	}
	if !tree.resultCompatibilityApplied {
		t.Fatal("resultCompatibilityApplied = false after deferred compatibility ran")
	}
	rt := tree.ParseRuntime()
	if rt.NormalizationPassesRun == 0 {
		t.Fatal("NormalizationPassesRun = 0, want deferred compatibility pass attribution")
	}
	if rt.NormalizationPasses == nil || len(*rt.NormalizationPasses) == 0 {
		t.Fatal("NormalizationPasses is empty, want deferred compatibility named pass attribution")
	}
	before := rt.NormalizationPassesRun
	_ = tree.RootNode()
	if got := tree.ParseRuntime().NormalizationPassesRun; got != before {
		t.Fatalf("NormalizationPassesRun after second RootNode = %d, want %d", got, before)
	}

	clone := tree.Copy()
	defer clone.Release()
	if clone.resultErrorSummary != tree.resultErrorSummary || clone.resultCompatibilityApplied != tree.resultCompatibilityApplied {
		t.Fatal("Tree.Copy did not preserve finalized compatibility state")
	}
}

func TestDeferredResultCompatibilitySynchronizesConcurrentTreeReaders(t *testing.T) {
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	lang, root, _ := newDeferredCompatDynamicImportFixture()
	tree := newTreeWithArenas(root, []byte("import"), lang, root.ownerArena, nil)
	tree.deferResultCompatibility()
	defer tree.Release()

	start := make(chan struct{})
	var wg sync.WaitGroup
	readers := []func(){
		func() { _ = tree.RootNode() },
		func() { _ = tree.ParseRuntime() },
		func() { _ = tree.ParseStopReason() },
		func() {
			clone := tree.Copy()
			if clone != nil {
				clone.Release()
			}
		},
		func() { _ = tree.WriteDOT(&bytes.Buffer{}, lang) },
		func() { _ = tree.RootNodeWithOffset(1, Point{Column: 1}) },
		func() { _ = tree.NodeAtByte(0) },
	}
	for _, read := range readers {
		read := read
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			read()
		}()
	}
	close(start)
	wg.Wait()

	if !tree.resultCompatibilityApplied {
		t.Fatal("deferred compatibility was not applied")
	}
	rt := tree.ParseRuntime()
	if rt.NormalizationPassesRun == 0 {
		t.Fatal("deferred compatibility did not record a normalization pass")
	}
}

func TestParserKeepsTypeScriptCompatibilityLazyUntilFirstPublicTreeRead(t *testing.T) {
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	t.Setenv("GOT_TS_LAZY_COMPAT", "1")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	blob, err := os.ReadFile("grammars/grammar_blobs/typescript.bin")
	if err != nil {
		t.Fatalf("read TypeScript grammar blob: %v", err)
	}
	lang, err := LoadLanguage(blob)
	if err != nil {
		t.Fatalf("load TypeScript grammar: %v", err)
	}
	tree, err := NewParser(lang).Parse([]byte("import"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	if !tree.hasDeferredResultCompatibility() {
		t.Fatal("real Parser result has no deferred TypeScript compatibility finalizer")
	}
	if tree.resultCompatibilityApplied {
		t.Fatal("TypeScript compatibility was applied before the Parser returned")
	}
	if got := tree.rawParseRuntime().NormalizationPassesRun; got != 0 {
		t.Fatalf("normalization passes before public read = %d, want 0", got)
	}

	first := tree.ParseRuntime()
	if first.NormalizationPassesRun == 0 {
		t.Fatal("first public runtime read did not run deferred TypeScript compatibility")
	}
	if !tree.resultCompatibilityApplied {
		t.Fatal("compatibility remains unapplied after first public read")
	}
	_ = tree.RootNode()
	if got := tree.ParseRuntime().NormalizationPassesRun; got != first.NormalizationPassesRun {
		t.Fatalf("deferred compatibility ran more than once: first=%d after=%d", first.NormalizationPassesRun, got)
	}
}

// TestParseRuntimeNormalizationPassesSurvivesCompactPublicationForIni proves
// the phase0 runtime-clobber fix in materializeDiagnosticParserCoreAcceptedSelection
// (parsercore_phase0_driver.go). The compact/phase0 admission route's internal
// sanity check used to call the PUBLIC tree.ParseRuntime() accessor as a
// sanity check on the just-materialized tree. That accessor fires the tree's
// one-shot deferred-compatibility finalizer (ensureResultCompatibility,
// tree.go), which writes named normalization counters into the runtime record.
// A few lines later, setParseRuntime full-struct-replaced that same record to
// install the final accepted stop reason. This permanently erased the metrics
// because the finalizer runs at most once. All generic passes are now retired.
// This test enables the dispatcher census so INI records the intentional
// dispatch.ini metric across the same publication boundary. INI always defers
// compatibility, so it needs no compatibility feature flag.
//
// Two preconditions are verified directly:
//
//  1. GTS_DISPATCHER_CENSUS must be enabled because no generic pass remains
//     to supply a named metric. The census records dispatch.ini.
//  2. DEFAULT admission settings are sufficient to route this parse through
//     the compact engine: no SetAdmissionCandidateRoute override is used
//     below. Confirmed with AdmissionCandidateCounters that a fresh default
//     Parser already reports routed=1, fallback=0 for this ~23-byte ini
//     source, because the compact candidate route has been the process
//     default since Phase-3 admission and this input is far under the 64 KiB
//     production-memory-budget size-gate floor that would otherwise decline it.
//
// Reverting the fix (materializeDiagnosticParserCoreAcceptedSelection calling
// tree.ParseRuntime() again instead of tree.rawParseRuntime()/rawParseStopReason())
// reproduces the bug directly: NormalizationPasses goes back to nil below.
func TestParseRuntimeNormalizationPassesSurvivesCompactPublicationForIni(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	resetAdmissionCandidateCounters()

	blob, err := os.ReadFile("grammars/grammar_blobs/ini.bin")
	if err != nil {
		t.Skipf("ini grammar blob unavailable: %v", err)
	}
	lang, err := LoadLanguage(blob)
	if err != nil {
		t.Fatalf("load ini grammar: %v", err)
	}

	parser := NewParser(lang)
	tree, err := parser.Parse([]byte("[section]\nkey = value\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	if routed, _ := AdmissionCandidateCounters(); routed == 0 {
		// Mirrors TestAdmissionCandidateMemoryBudgetContractPreserved
		// (admission_switch_test.go): in the emergency opt-out build
		// (-tags gts_no_parsercorephase0) the compact engine is compiled out,
		// so the routed counter cannot move and this test cannot exercise the
		// route it claims to. Skip rather than fail in that configuration.
		if reason := AdmissionCandidateLastFallbackReason(); strings.Contains(reason, "compiled out") {
			t.Skip("compact candidate route compiled out (-tags gts_no_parsercorephase0); cannot exercise the phase0 route")
		}
		t.Fatalf("default admission settings did not route this small, clean ini source through the compact engine; routed=%d", routed)
	}
	if !tree.compactMaterialized {
		t.Fatal("expected the compact/phase0 route to have materialized this tree")
	}

	rt := tree.ParseRuntime()
	if rt.StopReason != ParseStopAccepted {
		t.Fatalf("StopReason = %s, want %s (%s)", rt.StopReason, ParseStopAccepted, rt.Summary())
	}
	if !parseRuntimeHasNormalizationPass(rt, "dispatch.ini") {
		t.Fatal("NormalizationPasses has no dispatch.ini metric after a real Parse() through the compact route: the phase0 runtime-clobber regressed")
	}
}

func TestDeferredResultCompatibilityStateScrubbedAcrossTreePoolLifecycle(t *testing.T) {
	newDeferredTree := func() *Tree {
		lang, root, _ := newDeferredCompatDynamicImportFixture()
		tree := newTreeWithArenas(root, []byte("import"), lang, root.ownerArena, nil)
		tree.deferResultCompatibility()
		return tree
	}
	newPlainTree := func() *Tree {
		arena := acquireNodeArena(arenaClassFull)
		lang := &Language{Name: "pool-reuse", SymbolNames: []string{"EOF", "root"}}
		root := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
		return newTreeWithArenas(root, []byte("x"), lang, arena, nil)
	}

	for _, finalizeFirst := range []bool{false, true} {
		stale := newDeferredTree()
		if finalizeFirst {
			_ = stale.RootNode()
		}
		// Force the exact pooled-value reset seam on this pointer. This avoids
		// assuming sync.Pool will hand a released pointer back to this goroutine.
		plain := newPlainTree()
		root, source, lang, arena := plain.root, plain.source, plain.language, plain.arena
		plain.arena = nil
		plain.Release()
		if stale.arena != nil {
			stale.arena.Release()
		}
		resetTreeForReuse(stale, root, source, lang, arena, nil)
		reused := stale
		if reused.hasDeferredResultCompatibility() {
			t.Fatalf("pool acquisition inherited a deferred finalizer (finalized=%t)", finalizeFirst)
		}
		if reused.released {
			t.Fatalf("released state survived pool reset (finalized=%t)", finalizeFirst)
		}
		reused.Release()
	}

	tree := newDeferredTree()
	_ = tree.RootNode()
	before := tree.ParseRuntime().NormalizationPassesRun
	_ = tree.RootNode()
	if got := tree.ParseRuntime().NormalizationPassesRun; got != before {
		t.Fatalf("compatibility ran more than once: before=%d after=%d", before, got)
	}
	tree.Release()
}

func TestRawRootAccessKeepsDeferredTypeScriptCompatibility(t *testing.T) {
	lang, root, stmt := newDeferredCompatDynamicImportFixture()
	tree := newTreeWithArenas(root, []byte("import"), lang, root.ownerArena, nil)
	tree.deferResultCompatibility()

	if got := rawRootOrNil(tree); got != root {
		t.Fatalf("raw root = %p, want %p", got, root)
	}
	if got := resultChildCount(stmt); got != 0 {
		t.Fatalf("import child count after raw root access = %d, want deferred", got)
	}
	_ = tree.RootNode()
	if got, want := resultChildCount(stmt), 1; got != want {
		t.Fatalf("import child count after RootNode = %d, want %d", got, want)
	}
}

func TestParseRuntimeRootStatsUsesRawRootForDeferredTypeScriptCompatibility(t *testing.T) {
	lang, root, stmt := newDeferredCompatDynamicImportFixture()
	tree := newTreeWithArenas(root, []byte("import"), lang, root.ownerArena, nil)
	tree.deferResultCompatibility()

	var rt ParseRuntime
	recordParseRuntimeRootStats(&rt, tree, tree.Source(), 1, nil, false, lang)
	if got, want := rt.RootEndByte, uint32(6); got != want {
		t.Fatalf("RootEndByte = %d, want %d", got, want)
	}
	if rt.Truncated {
		t.Fatal("Truncated = true, want false")
	}
	if got := resultChildCount(stmt); got != 0 {
		t.Fatalf("import child count after ParseRuntime stats = %d, want deferred", got)
	}
	_ = tree.RootNode()
	if got, want := resultChildCount(stmt), 1; got != want {
		t.Fatalf("import child count after RootNode = %d, want %d", got, want)
	}
}

func TestNormalizeTypeScriptCompatibilityCandidatesApplyIndexedDirectRewrites(t *testing.T) {
	lang := &Language{
		Name: "typescript",
		SymbolNames: []string{
			"EOF", "program", "call_expression", "type_arguments", "arguments", "predefined_type",
			"binary_expression", ">", "parenthesized_expression", "<", "identifier", "member_expression",
			"sequence_expression", "enum_body", "enum_assignment", "import",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "type_arguments", Visible: true, Named: true},
			{Name: "arguments", Visible: true, Named: true},
			{Name: "predefined_type", Visible: true, Named: true},
			{Name: "binary_expression", Visible: true, Named: true},
			{Name: ">", Visible: true, Named: false},
			{Name: "parenthesized_expression", Visible: true, Named: true},
			{Name: "<", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "member_expression", Visible: true, Named: true},
			{Name: "sequence_expression", Visible: true, Named: true},
			{Name: "enum_body", Visible: true, Named: true},
			{Name: "enum_assignment", Visible: true, Named: true},
			{Name: "import", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	source := []byte("type enum E")
	keyword := newLeafNodeInArena(arena, 7, false, 0, 4, Point{}, Point{Column: 4})
	identifier := newParentNodeInArena(arena, 10, true, []*Node{keyword}, nil, 0)
	identifier.startByte = 0
	identifier.endByte = 4
	identifier.endPoint = Point{Column: 4}
	assignment := newLeafNodeInArena(arena, 14, true, 5, 6, Point{Column: 5}, Point{Column: 6})
	enumBody := newParentNodeInArena(arena, 13, true, []*Node{assignment}, []FieldID{1}, 0)
	enumBody.setFieldSources([]uint8{fieldSourceDirect})
	root := newParentNodeInArena(arena, 1, true, []*Node{identifier, enumBody}, nil, 0)

	stats, _ := normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats(root, source, lang, nil)
	if !stats.typeScriptCompatibility.built {
		t.Fatal("typeScriptCompatibility candidate index was not built")
	}
	if got := stats.typeScriptCompatibility.len(); got != 2 {
		t.Fatalf("candidate event count = %d, want 2", got)
	}

	normalizeTypeScriptCompatibilityCandidates(stats.typeScriptCompatibility, root, source, lang)

	if got := identifier.ChildCount(); got != 0 {
		t.Fatalf("identifier child count after candidate compatibility = %d, want 0", got)
	}
	if got := enumBody.fieldIDs()[0]; got != 0 {
		t.Fatalf("enum_body fieldIDs[0] = %d, want cleared", got)
	}
	if got := enumBody.fieldSources()[0]; got != fieldSourceNone {
		t.Fatalf("enum_body fieldSources[0] = %d, want none", got)
	}
}

func TestTypeScriptBinaryOperatorCompatibilityGate(t *testing.T) {
	ctx := typeScriptNormalizationContext{
		binaryExpressionSym: 1,
		greaterThanSym:      2,
		pipeSym:             3,
		ampersandSym:        4,
		hasPipeSym:          true,
		hasAmpersandSym:     true,
	}
	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 6, true, 0, 1, Point{}, Point{Column: 1})
	op := newLeafNodeInArena(arena, 5, false, 1, 2, Point{Column: 1}, Point{Column: 2})
	right := newLeafNodeInArena(arena, 6, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	binary := newParentNodeInArena(arena, ctx.binaryExpressionSym, true, []*Node{left, op, right}, nil, 0)

	if typeScriptBinaryOperatorCouldBeGenericCall(binary, &ctx) {
		t.Fatal("plus operator should not be a generic-call candidate")
	}
	if typeScriptBinaryOperatorCouldBeAsTypeChain(binary, &ctx) {
		t.Fatal("plus operator should not be an as-type-chain candidate")
	}

	op.symbol = ctx.greaterThanSym
	if !typeScriptBinaryOperatorCouldBeGenericCall(binary, &ctx) {
		t.Fatal("greater-than operator should be a generic-call candidate")
	}
	if typeScriptBinaryOperatorCouldBeAsTypeChain(binary, &ctx) {
		t.Fatal("greater-than operator should not be an as-type-chain candidate")
	}

	op.symbol = ctx.pipeSym
	if typeScriptBinaryOperatorCouldBeGenericCall(binary, &ctx) {
		t.Fatal("pipe operator should not be a generic-call candidate")
	}
	if !typeScriptBinaryOperatorCouldBeAsTypeChain(binary, &ctx) {
		t.Fatal("pipe operator should be an as-type-chain candidate")
	}
}

func TestTypeScriptGenericCallCandidateCheckDoesNotRetargetParents(t *testing.T) {
	ctx := typeScriptNormalizationContext{
		binaryExpressionSym:  1,
		greaterThanSym:       2,
		pipeSym:              3,
		lessThanSym:          4,
		parenthesizedExprSym: 5,
		hasPipeSym:           true,
	}
	arena := newNodeArena(arenaClassFull)
	callee := newLeafNodeInArena(arena, 6, true, 0, 8, Point{}, Point{Column: 8})
	lt := newLeafNodeInArena(arena, ctx.lessThanSym, false, 8, 9, Point{Column: 8}, Point{Column: 9})
	stringType := newLeafNodeInArena(arena, 7, true, 9, 15, Point{Column: 9}, Point{Column: 15})
	open := newParentNodeInArena(arena, ctx.binaryExpressionSym, true, []*Node{callee, lt, stringType}, nil, 0)
	pipe := newLeafNodeInArena(arena, ctx.pipeSym, false, 16, 17, Point{Column: 16}, Point{Column: 17})
	nullType := newLeafNodeInArena(arena, 8, true, 18, 22, Point{Column: 18}, Point{Column: 22})
	union := newParentNodeInArena(arena, ctx.binaryExpressionSym, true, []*Node{open, pipe, nullType}, nil, 0)
	gt := newLeafNodeInArena(arena, ctx.greaterThanSym, false, 22, 23, Point{Column: 22}, Point{Column: 23})
	args := newParentNodeInArena(arena, ctx.parenthesizedExprSym, true, nil, nil, 0)
	callShape := newParentNodeInArena(arena, ctx.binaryExpressionSym, true, []*Node{union, gt, args}, nil, 0)

	if !typeScriptBinaryExpressionHasGenericCallClose(callShape, &ctx) {
		t.Fatal("union generic call shape was not detected")
	}
	if nullType.parent != union {
		t.Fatal("candidate check retargeted union child parent link")
	}
	if pipe.parent != union {
		t.Fatal("candidate check retargeted union operator parent link")
	}
	if stringType.parent != open {
		t.Fatal("candidate check retargeted open generic child parent link")
	}
}

func TestTypeScriptCallInstantiatedCompatibilityGate(t *testing.T) {
	ctx := typeScriptNormalizationContext{
		callSym:              1,
		instantiationExprSym: 2,
		argsSym:              3,
	}
	arena := newNodeArena(arenaClassFull)
	callee := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
	args := newLeafNodeInArena(arena, ctx.argsSym, true, 1, 3, Point{Column: 1}, Point{Column: 3})
	call := newParentNodeInArena(arena, ctx.callSym, true, []*Node{callee, args}, nil, 0)

	if typeScriptCallCouldBeInstantiated(call, &ctx) {
		t.Fatal("plain callee should not be an instantiated-call candidate")
	}

	callee.symbol = ctx.instantiationExprSym
	if !typeScriptCallCouldBeInstantiated(call, &ctx) {
		t.Fatal("instantiation_expression callee should be an instantiated-call candidate")
	}
}

func TestNormalizeJavaScriptTopLevelDeclarationBoundsSnapToChildren(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "comment", "lexical_declaration", "const", "variable_declarator", ";"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "lexical_declaration", Visible: true, Named: true},
			{Name: "const", Visible: true, Named: false},
			{Name: "variable_declarator", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 2, true, 0, 6, Point{}, Point{Column: 6})
	constTok := newLeafNodeInArena(arena, 4, false, 7, 12, Point{Row: 1}, Point{Row: 1, Column: 5})
	decl := newLeafNodeInArena(arena, 5, true, 13, 22, Point{Row: 1, Column: 6}, Point{Row: 1, Column: 15})
	semi := newLeafNodeInArena(arena, 6, false, 22, 23, Point{Row: 1, Column: 15}, Point{Row: 1, Column: 16})
	lex := newParentNodeInArena(arena, 3, true, []*Node{constTok, decl, semi}, nil, 0)
	lex.startByte = 0
	lex.startPoint = Point{}
	lex.endByte = 23
	lex.endPoint = Point{Row: 1, Column: 16}
	root := newParentNodeInArena(arena, 1, true, []*Node{comment, lex}, nil, 0)

	normalizeJavaScriptTopLevelDeclarationBounds(root, lang)

	if got, want := lex.StartByte(), uint32(7); got != want {
		t.Fatalf("lex.StartByte = %d, want %d", got, want)
	}
	if got, want := lex.EndByte(), uint32(23); got != want {
		t.Fatalf("lex.EndByte = %d, want %d", got, want)
	}
	if got, want := lex.StartPoint(), (Point{Row: 1}); got != want {
		t.Fatalf("lex.StartPoint = %#v, want %#v", got, want)
	}
	if got, want := lex.EndPoint(), (Point{Row: 1, Column: 16}); got != want {
		t.Fatalf("lex.EndPoint = %#v, want %#v", got, want)
	}
}

func TestNormalizeTypeScriptRecoveredNamespaceRootRewrapsNamespaceBody(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "ERROR", "program", "comment", "namespace", "identifier", "{", "enum_declaration", "statement_block", "internal_module", "expression_statement"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "ERROR", Visible: true, Named: true},
			{Name: "program", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "namespace", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "{", Visible: true, Named: false},
			{Name: "enum_declaration", Visible: true, Named: true},
			{Name: "statement_block", Visible: true, Named: true},
			{Name: "internal_module", Visible: true, Named: true},
			{Name: "expression_statement", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	source := []byte("// c\nnamespace ts {\n  enum X\n\n")
	comment := newLeafNodeInArena(arena, 3, true, 0, 4, Point{}, Point{Column: 4})
	namespaceTok := newLeafNodeInArena(arena, 4, false, 5, 14, Point{Row: 1}, Point{Row: 1, Column: 9})
	name := newLeafNodeInArena(arena, 5, true, 15, 17, Point{Row: 1, Column: 10}, Point{Row: 1, Column: 12})
	openBrace := newLeafNodeInArena(arena, 6, false, 18, 19, Point{Row: 1, Column: 13}, Point{Row: 1, Column: 14})
	enumDecl := newLeafNodeInArena(arena, 7, true, 22, 28, Point{Row: 2, Column: 2}, Point{Row: 2, Column: 8})
	wsErr := newLeafNodeInArena(arena, 1, true, 28, 30, Point{Row: 2, Column: 8}, Point{Row: 4})
	wsErr.setHasError(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{comment, namespaceTok, name, openBrace, enumDecl, wsErr}, nil, 0)
	root.setHasError(true)

	normalizeTypeScriptRecoveredNamespaceRoot(root, source, lang)

	if got, want := root.Type(lang), "program"; got != want {
		t.Fatalf("root.Type = %q, want %q", got, want)
	}
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root.ChildCount = %d, want %d", got, want)
	}
	expr := root.Child(1)
	if got, want := expr.Type(lang), "expression_statement"; got != want {
		t.Fatalf("expr.Type = %q, want %q", got, want)
	}
	mod := expr.Child(0)
	if got, want := mod.Type(lang), "internal_module"; got != want {
		t.Fatalf("module.Type = %q, want %q", got, want)
	}
	block := mod.Child(1)
	if got, want := block.Type(lang), "statement_block"; got != want {
		t.Fatalf("block.Type = %q, want %q", got, want)
	}
	if got, want := block.ChildCount(), 1; got != want {
		t.Fatalf("block.ChildCount = %d, want %d", got, want)
	}
	if got, want := block.Child(0).Type(lang), "enum_declaration"; got != want {
		t.Fatalf("block.Child(0).Type = %q, want %q", got, want)
	}
	if root.HasError() {
		t.Fatal("root.HasError = true, want false")
	}
}
