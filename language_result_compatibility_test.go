package gotreesitter

import (
	"bytes"
	"os"
	"testing"
	"unicode/utf8"
)

func TestResultCompatibilityCapabilityLanguageBlobRoundTrip(t *testing.T) {
	wantValues := []ResultCompatibilityCapability{1 << 0, 1 << 1, 1 << 2, 1 << 3, 1 << 4, 1 << 5, 1 << 6}
	gotValues := []ResultCompatibilityCapability{
		ResultCompatibilityCSharpNativeNotNull,
		ResultCompatibilityCSharpNativeUnicodeIdentifiers,
		ResultCompatibilityCSharpNativeScopedLambdaStatements,
		ResultCompatibilityCSharpNativeScopedLambdaBlocks,
		ResultCompatibilityCSharpNativeQueryExpressions,
		ResultCompatibilityNativeCollapsedChildren,
		ResultCompatibilityNativeRecoveredStructure,
	}
	for i := range wantValues {
		if gotValues[i] != wantValues[i] {
			t.Fatalf("result compatibility capability %d = %#x, want %#x", i, gotValues[i], wantValues[i])
		}
	}

	want := ResultCompatibilityCSharpNativeNotNull |
		ResultCompatibilityCSharpNativeUnicodeIdentifiers |
		ResultCompatibilityCSharpNativeScopedLambdaStatements |
		ResultCompatibilityCSharpNativeScopedLambdaBlocks |
		ResultCompatibilityCSharpNativeQueryExpressions |
		ResultCompatibilityNativeCollapsedChildren |
		ResultCompatibilityNativeRecoveredStructure
	lang := &Language{
		Name:                      "result_compatibility_round_trip",
		NativeResultCompatibility: want,
	}

	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("EncodeLanguageBlob: %v", err)
	}
	decoded, err := LoadLanguage(blob)
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if got := decoded.NativeResultCompatibility; got != want {
		t.Fatalf("NativeResultCompatibility = %#x, want %#x", got, want)
	}
}

func TestCSharpLegacyCompatibilityCapabilityGatesEveryPass(t *testing.T) {
	tests := []struct {
		name       string
		capability ResultCompatibilityCapability
		unrelated  ResultCompatibilityCapability
	}{
		{"notnull", ResultCompatibilityCSharpNativeNotNull, ResultCompatibilityCSharpNativeUnicodeIdentifiers},
		{"unicode_identifiers", ResultCompatibilityCSharpNativeUnicodeIdentifiers, ResultCompatibilityCSharpNativeScopedLambdaStatements},
		{"scoped_lambda_statements", ResultCompatibilityCSharpNativeScopedLambdaStatements, ResultCompatibilityCSharpNativeScopedLambdaBlocks},
		{"scoped_lambda_blocks", ResultCompatibilityCSharpNativeScopedLambdaBlocks, ResultCompatibilityCSharpNativeQueryExpressions},
		{"query_expressions", ResultCompatibilityCSharpNativeQueryExpressions, ResultCompatibilityCSharpNativeNotNull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacy := csharpMissingNativeResultCompatibility(&Language{Name: "c_sharp"})
			if legacy&tt.capability == 0 {
				t.Fatal("zero-value language suppressed its conservative fallback")
			}

			certified := csharpMissingNativeResultCompatibility(&Language{
				Name:                      "c_sharp",
				NativeResultCompatibility: tt.capability,
			})
			if certified&tt.capability != 0 {
				t.Fatal("native capability did not suppress its fallback")
			}

			partiallyCertified := csharpMissingNativeResultCompatibility(&Language{
				Name:                      "c_sharp",
				NativeResultCompatibility: tt.unrelated,
			})
			if partiallyCertified&tt.capability == 0 {
				t.Fatal("unrelated capability suppressed the fallback")
			}
		})
	}
}

func TestCSharpLegacyNotNullRepairHonorsNativeCapability(t *testing.T) {
	tests := []struct {
		name       string
		capability ResultCompatibilityCapability
		wantType   string
	}{
		{"legacy_repairs", 0, "notnull"},
		{"native_suppresses", ResultCompatibilityCSharpNativeNotNull, "identifier"},
		{"unrelated_still_repairs", ResultCompatibilityCSharpNativeUnicodeIdentifiers, "notnull"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := &Language{
				Name:                      "c_sharp",
				NativeResultCompatibility: tt.capability,
				SymbolNames:               []string{"EOF", "root", "type_parameter_constraint", "identifier", "notnull"},
				SymbolMetadata: []SymbolMetadata{
					{Name: "EOF"},
					{Name: "root", Visible: true, Named: true},
					{Name: "type_parameter_constraint", Visible: true, Named: true},
					{Name: "identifier", Visible: true, Named: true},
					{Name: "notnull", Visible: true},
				},
			}
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			inner := newLeafNodeInArena(arena, 4, false, 0, 7, Point{}, Point{Column: 7})
			identifier := newParentNodeInArena(arena, 3, true, []*Node{inner}, nil, 0)
			constraint := newParentNodeInArena(arena, 2, true, []*Node{identifier}, nil, 0)
			root := newParentNodeInArena(arena, 1, true, []*Node{constraint}, nil, 0)

			normalizeCSharpCompatibility(root, []byte("notnull"), &Parser{skipRecoveryReparse: true}, lang)
			if got := constraint.Child(0).Type(lang); got != tt.wantType {
				t.Fatalf("constraint child type = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestCSharpLegacyUnicodeRepairHonorsNativeCapability(t *testing.T) {
	source := []byte("ග්\u200dරහලෝකය")
	_, firstRuneBytes := utf8.DecodeRune(source)
	tests := []struct {
		name       string
		capability ResultCompatibilityCapability
		wantEnd    uint32
	}{
		{"legacy_repairs", 0, uint32(len(source))},
		{"native_suppresses", ResultCompatibilityCSharpNativeUnicodeIdentifiers, uint32(firstRuneBytes)},
		{"unrelated_still_repairs", ResultCompatibilityCSharpNativeQueryExpressions, uint32(len(source))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := &Language{
				Name:                      "c_sharp",
				NativeResultCompatibility: tt.capability,
				SymbolNames:               []string{"EOF", "root", "identifier"},
				SymbolMetadata: []SymbolMetadata{
					{Name: "EOF"},
					{Name: "root", Visible: true, Named: true},
					{Name: "identifier", Visible: true, Named: true},
				},
			}
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			identifier := newLeafNodeInArena(arena, 2, true, 0, uint32(firstRuneBytes), Point{}, Point{Column: uint32(firstRuneBytes)})
			root := newParentNodeInArena(arena, 1, true, []*Node{identifier}, nil, 0)

			normalizeCSharpCompatibility(root, source, &Parser{skipRecoveryReparse: true}, lang)
			if got := identifier.EndByte(); got != tt.wantEnd {
				t.Fatalf("identifier end = %d, want %d", got, tt.wantEnd)
			}
		})
	}
}

func TestCSharpLegacyScopedLambdaStatementRepairHonorsNativeCapability(t *testing.T) {
	blob := readCSharpLegacyTestBlob(t)
	source := []byte("var first = scoped => null; var second = scoped => null;")
	tests := []struct {
		name       string
		capability ResultCompatibilityCapability
		wantCount  int
	}{
		{"legacy_repairs", 0, 2},
		{"native_suppresses", ResultCompatibilityCSharpNativeScopedLambdaStatements, 1},
		{"unrelated_still_repairs", ResultCompatibilityCSharpNativeQueryExpressions, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := loadCSharpLegacyTestLanguage(t, blob)
			lang.NativeResultCompatibility = tt.capability
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			badStatement := csharpLegacyTestLeaf(t, arena, lang, "local_declaration_statement", source, 0, uint32(len(source)))
			block := csharpLegacyTestParent(t, arena, lang, "block", []*Node{badStatement})

			normalizeCSharpCompatibility(block, source, &Parser{skipRecoveryReparse: true}, lang)
			if got := block.ChildCount(); got != tt.wantCount {
				t.Fatalf("block child count = %d, want %d: %s", got, tt.wantCount, block.SExpr(lang))
			}
		})
	}
}

func TestCSharpLegacyScopedLambdaBlockRepairHonorsNativeCapability(t *testing.T) {
	blob := readCSharpLegacyTestBlob(t)
	source := []byte("{ var first = scoped => null; var second = scoped => null; }")
	statementStart := uint32(2)
	statementEnd := uint32(len(source) - 2)
	tests := []struct {
		name       string
		capability ResultCompatibilityCapability
		wantSplit  bool
	}{
		{
			"legacy_repairs",
			// Skip the statement splitter so this fixture isolates block recovery.
			ResultCompatibilityCSharpNativeScopedLambdaStatements,
			true,
		},
		{
			"native_suppresses",
			ResultCompatibilityCSharpNativeScopedLambdaStatements | ResultCompatibilityCSharpNativeScopedLambdaBlocks,
			false,
		},
		{
			"unrelated_still_repairs",
			ResultCompatibilityCSharpNativeScopedLambdaStatements | ResultCompatibilityCSharpNativeQueryExpressions,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := loadCSharpLegacyTestLanguage(t, blob)
			lang.NativeResultCompatibility = tt.capability
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			badStatement := csharpLegacyTestLeaf(t, arena, lang, "local_declaration_statement", source, statementStart, statementEnd)
			block := csharpLegacyTestParent(t, arena, lang, "block", []*Node{badStatement})
			block.startByte = 0
			block.endByte = uint32(len(source))
			block.startPoint = Point{}
			block.endPoint = advancePointByBytes(Point{}, source)
			parser := NewParser(lang)

			normalizeCSharpCompatibility(block, source, parser, lang)
			gotSplit := block.ChildCount() > 1
			if gotSplit != tt.wantSplit {
				t.Fatalf("block split = %v, want %v (children=%d): %s", gotSplit, tt.wantSplit, block.ChildCount(), block.SExpr(lang))
			}
		})
	}
}

func TestCSharpLegacyQueryExpressionRepairHonorsNativeCapability(t *testing.T) {
	blob := readCSharpLegacyTestBlob(t)
	source := []byte("var result = from a in source where a.Enabled select a;\n")
	// Stay just beyond the general top-level recovery cutoff so this fixture
	// can only become clean through the query-expression fallback under test.
	source = append(source, bytes.Repeat([]byte{' '}, csharpMaxTopLevelChunkRecoverySourceBytes)...)
	tests := []struct {
		name       string
		capability ResultCompatibilityCapability
		wantRepair bool
	}{
		{"legacy_repairs", 0, true},
		{"native_suppresses", ResultCompatibilityCSharpNativeQueryExpressions, false},
		{"unrelated_still_repairs", ResultCompatibilityCSharpNativeUnicodeIdentifiers, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := loadCSharpLegacyTestLanguage(t, blob)
			lang.NativeResultCompatibility = tt.capability
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			root := csharpLegacyTestLeaf(t, arena, lang, "compilation_unit", source, 0, uint32(len(source)))
			root.setHasError(true)
			parser := NewParser(lang)

			normalizeCSharpCompatibility(root, source, parser, lang)
			gotRepair := !root.HasError() && csharpLegacyTreeHasType(root, lang, "query_expression")
			if gotRepair != tt.wantRepair {
				t.Fatalf("query repair = %v, want %v: %s", gotRepair, tt.wantRepair, root.SExpr(lang))
			}
		})
	}
}

func TestCSharpRecoveredStructureCapabilityKeepsSyntheticRepair(t *testing.T) {
	blob := readCSharpLegacyTestBlob(t)
	lang := loadCSharpLegacyTestLanguage(t, blob)
	lang.NativeResultCompatibility = ResultCompatibilityNativeRecoveredStructure
	source := []byte("class C {}")
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	root := csharpLegacyTestLeaf(
		t,
		arena,
		lang,
		"compilation_unit",
		source,
		0,
		uint32(len(source)),
	)
	root.setHasError(true)
	parser := NewParser(lang)

	if nativeRecoveredStructureIsAuthoritative(root, source, parser, lang) {
		t.Fatal("a synthetic full-span root received native producer authority")
	}
	normalizeCSharpCompatibility(root, source, parser, lang)
	if root.HasError() || !csharpLegacyTreeHasType(root, lang, "class_declaration") {
		t.Fatalf("synthetic accepted root did not receive conservative repair: %s", root.SExpr(lang))
	}
}

func TestNativeRecoveredStructureIsolatedErrorReceipt(t *testing.T) {
	tests := []struct {
		name        string
		errorSpans  [][2]uint32
		missing     bool
		rawMismatch bool
		want        bool
	}{
		{name: "isolated_error", errorSpans: [][2]uint32{{0, 1}}, want: true},
		{name: "wide_error", errorSpans: [][2]uint32{{0, 2}}},
		{name: "reversed_error_span", errorSpans: [][2]uint32{{2, 1}}},
		{name: "multiple_errors", errorSpans: [][2]uint32{{0, 1}, {1, 2}}},
		{name: "missing_node", errorSpans: [][2]uint32{{0, 1}}, missing: true},
		{name: "raw_child_mismatch", errorSpans: [][2]uint32{{0, 1}}, rawMismatch: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()

			children := make([]*Node, 0, len(test.errorSpans)+1)
			for _, span := range test.errorSpans {
				node := newLeafNodeInArena(
					arena,
					errorSymbol,
					true,
					span[0],
					span[1],
					Point{Column: span[0]},
					Point{Column: span[1]},
				)
				node.setHasError(true)
				children = append(children, node)
			}
			if test.missing {
				node := newLeafNodeInArena(
					arena,
					1,
					true,
					1,
					1,
					Point{Column: 1},
					Point{Column: 1},
				)
				node.setMissing(true)
				node.setHasError(true)
				children = append(children, node)
			}
			root := newParentNodeInArena(arena, 2, true, children, nil, 0)
			root.setHasError(true)
			rawChildren := children
			if test.rawMismatch {
				rawChildren = []*Node{newLeafNodeInArena(
					arena,
					errorSymbol,
					true,
					0,
					2,
					Point{},
					Point{Column: 2},
				)}
			}
			root.rawShape = captureRawShapeForNodeSlice(arena, root.symbol, 0, rawChildren)

			if got := nativeRecoveredStructureHasIsolatedErrorReceipt(root); got != test.want {
				t.Fatalf("isolated-error receipt = %t, want %t", got, test.want)
			}
		})
	}
}

func TestNativeRecoveredStructureReceiptRejectsDepthExhaustion(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	node := newLeafNodeInArena(
		arena,
		errorSymbol,
		true,
		0,
		1,
		Point{},
		Point{Column: 1},
	)
	node.setHasError(true)
	for depth := 0; depth < maxTreeWalkDepth; depth++ {
		node = newParentNodeInArena(arena, 2, true, []*Node{node}, nil, 0)
		node.setHasError(true)
	}
	node.rawShape = captureRawShapeForNodeSlice(
		arena,
		node.symbol,
		0,
		[]*Node{resultChildAt(node, 0)},
	)

	if nativeRecoveredStructureHasIsolatedErrorReceipt(node) {
		t.Fatal("depth-exhausted tree received native recovered structure authority")
	}
}

func readCSharpLegacyTestBlob(t *testing.T) []byte {
	t.Helper()
	blob, err := os.ReadFile("grammars/grammar_blobs/c_sharp.bin")
	if err != nil {
		t.Fatalf("read C# grammar blob: %v", err)
	}
	return blob
}

func loadCSharpLegacyTestLanguage(t *testing.T, blob []byte) *Language {
	t.Helper()
	lang, err := LoadLanguage(blob)
	if err != nil {
		t.Fatalf("load legacy C# grammar: %v", err)
	}
	if got := lang.NativeResultCompatibility; got != 0 {
		t.Fatalf("raw legacy loader unexpectedly attached native compatibility %#x", got)
	}
	return lang
}

func csharpLegacyTestLeaf(t *testing.T, arena *nodeArena, lang *Language, typ string, source []byte, start, end uint32) *Node {
	t.Helper()
	sym, ok := lang.SymbolByName(typ)
	if !ok {
		t.Fatalf("C# grammar has no %q symbol", typ)
	}
	return newLeafNodeInArena(
		arena,
		sym,
		symbolIsNamed(lang, sym),
		start,
		end,
		advancePointByBytes(Point{}, source[:start]),
		advancePointByBytes(Point{}, source[:end]),
	)
}

func csharpLegacyTestParent(t *testing.T, arena *nodeArena, lang *Language, typ string, children []*Node) *Node {
	t.Helper()
	sym, ok := lang.SymbolByName(typ)
	if !ok {
		t.Fatalf("C# grammar has no %q symbol", typ)
	}
	return newParentNodeInArena(arena, sym, symbolIsNamed(lang, sym), children, nil, 0)
}

func csharpLegacyTreeHasType(root *Node, lang *Language, typ string) bool {
	found := false
	walkResultTree(root, func(node *Node) {
		if node != nil && node.Type(lang) == typ {
			found = true
		}
	})
	return found
}
