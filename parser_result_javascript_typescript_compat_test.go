package gotreesitter

import "testing"

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

func TestNormalizeJavaScriptEmptyStatementRestoresSemicolonChild(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "empty_statement", ";"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "empty_statement", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	stmt := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)

	normalizeJavaScriptCompatibility(root, []byte(";"), lang)

	if got, want := resultChildCount(stmt), 1; got != want {
		t.Fatalf("empty_statement child count = %d, want %d", got, want)
	}
	child := resultChildAt(stmt, 0)
	if child == nil {
		t.Fatal("empty_statement child is nil")
	}
	if got, want := child.Type(lang), ";"; got != want {
		t.Fatalf("empty_statement child type = %q, want %q", got, want)
	}
}

func TestNormalizeTypeScriptSyntaxPassRestoresEmptyStatementSemicolonChild(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "program", "empty_statement", ";", "call_expression", "unary_expression", "binary_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "empty_statement", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "unary_expression", Visible: true, Named: true},
			{Name: "binary_expression", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	stmt := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)

	normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedence(root, []byte(";"), lang)

	if got, want := resultChildCount(stmt), 1; got != want {
		t.Fatalf("empty_statement child count = %d, want %d", got, want)
	}
	child := resultChildAt(stmt, 0)
	if child == nil {
		t.Fatal("empty_statement child is nil")
	}
	if got, want := child.Type(lang), ";"; got != want {
		t.Fatalf("empty_statement child type = %q, want %q", got, want)
	}
}

func TestTreeRootNodeAppliesDeferredTypeScriptCompatibility(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "program", "empty_statement", ";", "call_expression", "unary_expression", "binary_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "empty_statement", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "unary_expression", Visible: true, Named: true},
			{Name: "binary_expression", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	stmt := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)
	tree := newTreeWithArenas(root, []byte(";"), lang, arena, nil)
	tree.deferResultCompatibility()

	if got := resultChildCount(stmt); got != 0 {
		t.Fatalf("empty_statement child count before RootNode = %d, want 0", got)
	}
	if tree.RootNode() != root {
		t.Fatal("RootNode returned a different root")
	}
	if got, want := resultChildCount(stmt), 1; got != want {
		t.Fatalf("empty_statement child count after RootNode = %d, want %d", got, want)
	}
	child := resultChildAt(stmt, 0)
	if child == nil {
		t.Fatal("empty_statement child is nil")
	}
	if got, want := child.Type(lang), ";"; got != want {
		t.Fatalf("empty_statement child type = %q, want %q", got, want)
	}
}

func TestTreeRootNodeRecordsDeferredTypeScriptCompatibilityTiming(t *testing.T) {
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)

	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "program", "empty_statement", ";", "call_expression", "unary_expression", "binary_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "empty_statement", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "unary_expression", Visible: true, Named: true},
			{Name: "binary_expression", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	stmt := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)
	tree := newTreeWithArenas(root, []byte(";"), lang, arena, nil)
	tree.deferResultCompatibility()

	_ = tree.RootNode()
	if tree.resultCompatibilityPending {
		t.Fatal("resultCompatibilityPending = true after deferred compatibility ran")
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
}

func TestReturnedTreeNormalizationUsesRawRootForDeferredTypeScriptCompatibility(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "program", "empty_statement", ";", "call_expression", "unary_expression", "binary_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "empty_statement", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "unary_expression", Visible: true, Named: true},
			{Name: "binary_expression", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	stmt := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)
	tree := newTreeWithArenas(root, []byte(";"), lang, arena, nil)
	tree.deferResultCompatibility()

	normalizeReturnedTree(rawRootOrNil(tree), []byte(";"), lang)
	if got := resultChildCount(stmt); got != 0 {
		t.Fatalf("empty_statement child count after returned-tree normalization = %d, want deferred", got)
	}
	_ = tree.RootNode()
	if got, want := resultChildCount(stmt), 1; got != want {
		t.Fatalf("empty_statement child count after RootNode = %d, want %d", got, want)
	}
}

func TestParseRuntimeRootStatsUsesRawRootForDeferredTypeScriptCompatibility(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "program", "empty_statement", ";", "call_expression", "unary_expression", "binary_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "empty_statement", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "unary_expression", Visible: true, Named: true},
			{Name: "binary_expression", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	stmt := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)
	tree := newTreeWithArenas(root, []byte(";"), lang, arena, nil)
	tree.deferResultCompatibility()

	var rt ParseRuntime
	recordParseRuntimeRootStats(&rt, tree, 1, false, lang)
	if got, want := rt.RootEndByte, uint32(1); got != want {
		t.Fatalf("RootEndByte = %d, want %d", got, want)
	}
	if rt.Truncated {
		t.Fatal("Truncated = true, want false")
	}
	if got := resultChildCount(stmt); got != 0 {
		t.Fatalf("empty_statement child count after ParseRuntime stats = %d, want deferred", got)
	}
	_ = tree.RootNode()
	if got, want := resultChildCount(stmt), 1; got != want {
		t.Fatalf("empty_statement child count after RootNode = %d, want %d", got, want)
	}
}

func TestNormalizeTypeScriptSyntaxPassRestoresExistentialTypeStarChild(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"EOF", "program", "existential_type", "*", "call_expression", "unary_expression", "binary_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "existential_type", Visible: true, Named: true},
			{Name: "*", Visible: true, Named: false},
			{Name: "call_expression", Visible: true, Named: true},
			{Name: "unary_expression", Visible: true, Named: true},
			{Name: "binary_expression", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	existentialType := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArena(arena, 1, true, []*Node{existentialType}, nil, 0)

	normalizeTypeScriptTreeCompatibility(root, []byte("*"), lang)

	if got, want := resultChildCount(existentialType), 1; got != want {
		t.Fatalf("existential_type child count = %d, want %d", got, want)
	}
	child := resultChildAt(existentialType, 0)
	if child == nil {
		t.Fatal("existential_type child is nil")
	}
	if got, want := child.Type(lang), "*"; got != want {
		t.Fatalf("existential_type child type = %q, want %q", got, want)
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

func TestNormalizeJavaScriptStatementKeywordRestoresWhileLeaf(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "while_statement", "while", "parenthesized_expression", "statement_block", "}"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "while_statement", Visible: true, Named: true},
			{Name: "while", Visible: true, Named: false},
			{Name: "parenthesized_expression", Visible: true, Named: true},
			{Name: "statement_block", Visible: true, Named: true},
			{Name: "}", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	source := []byte("while (x) {}")
	strayClose := newLeafNodeInArena(arena, 6, false, 11, 12, Point{Column: 11}, Point{Column: 12})
	condition := newLeafNodeInArena(arena, 4, true, 6, 9, Point{Column: 6}, Point{Column: 9})
	body := newLeafNodeInArena(arena, 5, true, 10, 12, Point{Column: 10}, Point{Column: 12})
	stmt := newParentNodeInArena(arena, 2, true, []*Node{strayClose, condition, body}, nil, 0)
	stmt.startByte = 0
	stmt.startPoint = Point{}
	stmt.endByte = 12
	stmt.endPoint = Point{Column: 12}
	root := newParentNodeInArena(arena, 1, true, []*Node{stmt}, nil, 0)

	normalizeJavaScriptCompatibility(root, source, lang)

	first := resultChildAt(stmt, 0)
	if first == nil {
		t.Fatal("while_statement first child is nil")
	}
	if got, want := first.Type(lang), "while"; got != want {
		t.Fatalf("while_statement first child type = %q, want %q", got, want)
	}
	if got, want := resultChildCount(stmt), 3; got != want {
		t.Fatalf("while_statement child count = %d, want %d", got, want)
	}
}

func TestNormalizeJavaScriptStatementKeywordRestoresFinalRefWhileLeaf(t *testing.T) {
	lang := &Language{
		Name:        "javascript",
		SymbolNames: []string{"EOF", "program", "while_statement", "while", "parenthesized_expression", "statement_block", "}"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "program", Visible: true, Named: true},
			{Name: "while_statement", Visible: true, Named: true},
			{Name: "while", Visible: true, Named: false},
			{Name: "parenthesized_expression", Visible: true, Named: true},
			{Name: "statement_block", Visible: true, Named: true},
			{Name: "}", Visible: true, Named: false},
		},
	}

	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	source := []byte("while (x) {}")
	strayClose := newCompactFullLeafInArena(arena, 6, false, 11, 12, Point{Column: 11}, Point{Column: 12})
	condition := newCompactFullLeafInArena(arena, 4, true, 6, 9, Point{Column: 6}, Point{Column: 9})
	body := newCompactFullLeafInArena(arena, 5, true, 10, 12, Point{Column: 10}, Point{Column: 12})
	stmt := newPendingParentInArena(arena, 2, true, 0, []stackEntry{
		newStackEntryCompactFullLeaf(strayClose.parseState, strayClose),
		newStackEntryCompactFullLeaf(condition.parseState, condition),
		newStackEntryCompactFullLeaf(body.parseState, body),
	}, 0, 12, Point{}, Point{Column: 12}, false)
	rootParent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{
		newStackEntryPendingParent(stmt.parseState, stmt),
	}, 0, 12, Point{}, Point{Column: 12}, false)
	rootEntry := newStackEntryPendingParent(rootParent.parseState, rootParent)
	root := materializeStackEntryPendingParent(arena, &rootEntry, pendingParentMaterializeForFinalTree)

	normalizeJavaScriptCompatibility(root, source, lang)

	child := root.Child(0)
	if child == nil {
		t.Fatal("program child is nil")
	}
	first := resultChildAt(child, 0)
	if first == nil {
		t.Fatal("while_statement first child is nil")
	}
	if got, want := first.Type(lang), "while"; got != want {
		t.Fatalf("while_statement first child type = %q, want %q", got, want)
	}
	if got, want := resultChildCount(child), 3; got != want {
		t.Fatalf("while_statement child count = %d, want %d", got, want)
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
