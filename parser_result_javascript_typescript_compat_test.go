package gotreesitter

import (
	"bytes"
	"os"
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

// newDeferredCompatDynamicImportFixture builds a small TypeScript context.
// Its childless dynamic-import leaf exercises TypeScript's separate candidate
// normalizer. This marker shows that deferred TypeScript compatibility ran.
func newDeferredCompatDynamicImportFixture() (*Language, *Node, *Node) {
	lang := &Language{
		Name: "typescript",
		SymbolNames: []string{
			"EOF", "program", "import", "import", "call_expression",
			"type_arguments", "arguments", "predefined_type",
			"binary_expression", ">", "parenthesized_expression", "<",
			"identifier", "member_expression", "sequence_expression",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "import", Visible: true, Named: false},
			{Name: "import", Visible: true, Named: true},
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
		},
	}
	arena := newNodeArena(arenaClassFull)
	stmt := newLeafNodeInArena(arena, 3, true, 0, 6, Point{}, Point{Column: 6})
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

func TestDeferredTypeScriptCompatibilityInvalidatesCompactReuseProof(t *testing.T) {
	t.Run("unchanged compact tree", func(t *testing.T) {
		lang, _, _ := newDeferredCompatDynamicImportFixture()
		arena := newNodeArena(arenaClassFull)
		identifier := newLeafNodeInArena(arena, 12, true, 0, 3, Point{}, Point{Column: 3})
		root := newParentNodeInArena(arena, 1, true, []*Node{identifier}, nil, 0)
		for _, node := range []*Node{root, identifier} {
			node.setCompactMaterialized(true)
			node.setCompactParseStateProof(true)
			if node.ChildCount() > 0 {
				node.setCompactPreGotoStateProof(true)
			}
		}
		tree := newTreeWithArenas(root, []byte("foo"), lang, arena, nil)
		tree.deferResultCompatibility()

		_ = tree.RootNode()
		if tree.incrementalReuseDisabled {
			t.Fatal("unchanged deferred compatibility disabled compact reuse")
		}
	})

	t.Run("pure compact tree", func(t *testing.T) {
		lang, root, stmt := newDeferredCompatDynamicImportFixture()
		root.setCompactMaterialized(true)
		root.setCompactParseStateProof(true)
		root.setCompactPreGotoStateProof(true)
		stmt.setCompactMaterialized(true)
		stmt.setCompactParseStateProof(true)
		tree := newTreeWithArenas(root, []byte("import"), lang, root.ownerArena, nil)
		tree.deferResultCompatibility()

		if !compactTreeIncrementalReuseProven(root) {
			t.Fatal("fixture lacks compact reuse proof before compatibility")
		}
		_ = tree.RootNode()
		if !tree.incrementalReuseDisabled {
			t.Fatal("deferred compatibility kept stale compact reuse proof")
		}
		if compactTreeIncrementalReuseProven(root) {
			t.Fatal("compatibility rewrite unexpectedly retained compact reuse proof")
		}
	})

	t.Run("mixed incremental lineage", func(t *testing.T) {
		lang, root, stmt := newDeferredCompatDynamicImportFixture()
		stmt.setCompactMaterialized(true)
		stmt.setCompactParseStateProof(true)
		tree := newTreeWithArenas(root, []byte("import"), lang, root.ownerArena, nil)
		tree.deferResultCompatibility()

		if !compactTreeContainsMaterializedNode(root) {
			t.Fatal("fixture lacks inherited compact descendant")
		}
		if tree.incrementalReuseDisabled {
			t.Fatal("fixture disabled incremental reuse before compatibility")
		}
		_ = tree.RootNode()
		if !tree.incrementalReuseDisabled {
			t.Fatal("deferred compatibility kept mixed compact lineage reusable")
		}
	})

	t.Run("in-place child removal", func(t *testing.T) {
		lang, _, _ := newDeferredCompatDynamicImportFixture()
		arena := newNodeArena(arenaClassFull)
		keyword := newLeafNodeInArena(arena, 2, false, 0, 6, Point{}, Point{Column: 6})
		identifier := newParentNodeInArena(arena, 12, true, []*Node{keyword}, nil, 0)
		root := newParentNodeInArena(arena, 1, true, []*Node{identifier}, nil, 0)
		for _, node := range []*Node{root, identifier, keyword} {
			node.setCompactMaterialized(true)
			node.setCompactParseStateProof(true)
			if node.ChildCount() > 0 {
				node.setCompactPreGotoStateProof(true)
			}
		}
		tree := newTreeWithArenas(root, []byte("import"), lang, arena, nil)
		tree.deferResultCompatibility()

		if !compactTreeIncrementalReuseProven(root) {
			t.Fatal("fixture lacks compact reuse proof before compatibility")
		}
		if got := resultChildCount(identifier); got != 1 {
			t.Fatalf("identifier child count before compatibility=%d, want 1", got)
		}
		_ = tree.RootNode()
		if got := resultChildCount(identifier); got != 0 {
			t.Fatalf("identifier child count after compatibility=%d, want 0", got)
		}
		if !compactTreeIncrementalReuseProven(root) {
			t.Fatal("in-place rewrite unexpectedly cleared the stale proof bits")
		}
		if !tree.incrementalReuseDisabled {
			t.Fatal("in-place compatibility rewrite kept compact lineage reusable")
		}
	})
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
