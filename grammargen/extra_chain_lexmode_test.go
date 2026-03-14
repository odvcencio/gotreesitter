package grammargen

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNonterminalExtraChainLexModesDoNotInheritTerminalExtras(t *testing.T) {
	g := NewGrammar("extra_chain_lexmode")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Choice(Token(Pat(`.`)), Token(Str("//")))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("block_comment"))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	tables, ctx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build LR tables: %v", err)
	}
	addNonterminalExtraChains(tables, ng, ctx)

	slashStarSyms := diagFindAllSymbols(ng, "/*")
	if len(slashStarSyms) != 1 {
		t.Fatalf("expected one /* symbol, got %v", slashStarSyms)
	}
	whitespaceSyms := diagFindAllSymbols(ng, "_whitespace")
	if len(whitespaceSyms) != 1 {
		t.Fatalf("expected one _whitespace symbol, got %v", whitespaceSyms)
	}
	closeCommentSyms := diagFindAllSymbols(ng, "*/")
	if len(closeCommentSyms) != 1 {
		t.Fatalf("expected one */ symbol, got %v", closeCommentSyms)
	}

	acts := tables.ActionTable[0][slashStarSyms[0]]
	if len(acts) != 1 || acts[0].kind != lrShift {
		t.Fatalf("expected synthetic extra-chain shift on /*, got %s", diagFormatActions(ng, acts))
	}
	target := acts[0].state
	if target < tables.ExtraChainStateStart {
		t.Fatalf("expected synthetic state >= %d, got %d", tables.ExtraChainStateStart, target)
	}

	lexModes, stateToMode := computeLexModes(
		tables.StateCount,
		ng.TokenCount(),
		func(state, sym int) bool {
			if bySym, ok := tables.ActionTable[state]; ok {
				if acts, ok := bySym[sym]; ok && len(acts) > 0 {
					return true
				}
			}
			return false
		},
		computeStringPrefixExtensions(ng.Terminals),
		ng.ExtraSymbols,
		tables.ExtraChainStateStart,
		map[int]bool{},
		ng.ExternalSymbols,
		ng.WordSymbolID,
		map[int]bool{},
	)

	initialMode := lexModes[stateToMode[0]]
	if !initialMode.skipWhitespace {
		t.Fatal("initial state should still skip whitespace extras")
	}
	if !initialMode.validSymbols[whitespaceSyms[0]] {
		t.Fatal("initial state should keep terminal extra valid")
	}

	chainMode := lexModes[stateToMode[target]]
	if chainMode.skipWhitespace {
		t.Fatal("synthetic extra-chain state should not skip whitespace")
	}
	if chainMode.validSymbols[whitespaceSyms[0]] {
		t.Fatal("synthetic extra-chain state should not inherit terminal extra symbols")
	}
	if !chainMode.validSymbols[closeCommentSyms[0]] {
		t.Fatal("synthetic extra-chain state should still accept the explicit comment terminator token")
	}
}

func TestNonterminalExtraChainRuntimeProducesReducedExtraNode(t *testing.T) {
	g := NewGrammar("extra_chain_runtime")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("block_comment"))

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tree, err := gotreesitter.NewParser(report.Language).Parse([]byte("/**/foo"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasError() {
		t.Fatalf("root has error: %s", safeSExpr(root, report.Language, 16))
	}
	if root.EndByte() != 7 {
		t.Fatalf("root end byte = %d, want 7", root.EndByte())
	}
	if root.ChildCount() < 2 {
		t.Fatalf("root child count = %d, want at least 2", root.ChildCount())
	}
	if got := root.Child(0).Type(report.Language); got != "block_comment" {
		t.Fatalf("child[0] type = %q, want block_comment; sexpr=%s", got, safeSExpr(root, report.Language, 16))
	}
	if got := root.Child(1).Type(report.Language); got != "item" {
		t.Fatalf("child[1] type = %q, want item; sexpr=%s", got, safeSExpr(root, report.Language, 16))
	}
}

func TestNonterminalExtraChainRuntimeProducesReducedRepeatedExtraNode(t *testing.T) {
	g := NewGrammar("extra_chain_runtime_repeat")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Choice(Token(Pat(`.`)), Token(Str("//")))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("block_comment"))

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tree, err := gotreesitter.NewParser(report.Language).Parse([]byte("/**/foo"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasError() {
		t.Fatalf("root has error: %s", safeSExpr(root, report.Language, 16))
	}
	if got := safeSExpr(root, report.Language, 16); got != "(source_file (block_comment) (item))" {
		t.Fatalf("sexpr = %s, want (source_file (block_comment) (item))", got)
	}
}

func TestNonterminalExtraChainRuntimeProducesReducedRepeatedExtraNodeWithSiblingCommentExtra(t *testing.T) {
	g := NewGrammar("extra_chain_runtime_repeat_comment")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("comment", Seq(
		Token(Str("//")),
		Repeat(Token(Pat(`.`))),
	))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Choice(Token(Pat(`.`)), Token(Str("//")))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("comment"), Sym("block_comment"))

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tree, err := gotreesitter.NewParser(report.Language).Parse([]byte("/**/foo"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasError() {
		t.Fatalf("root has error: %s", safeSExpr(root, report.Language, 16))
	}
	if got := safeSExpr(root, report.Language, 16); got != "(source_file (block_comment) (item))" {
		t.Fatalf("sexpr = %s, want (source_file (block_comment) (item))", got)
	}
}

func TestNonterminalExtraChainRuntimeProducesReducedRepeatedExtraNodeAtEOF(t *testing.T) {
	g := NewGrammar("extra_chain_runtime_repeat_eof")
	g.Define("source_file", Repeat(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Choice(Token(Pat(`.`)), Token(Str("//")))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("block_comment"))

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tree, err := gotreesitter.NewParser(report.Language).Parse([]byte("/**/"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasError() {
		t.Fatalf("root has error: %s", safeSExpr(root, report.Language, 16))
	}
	if got := safeSExpr(root, report.Language, 16); got != "(source_file (block_comment))" {
		t.Fatalf("sexpr = %s, want (source_file (block_comment))", got)
	}
}

func TestNonterminalExtraChainSyntheticStatesCanStartNestedExtras(t *testing.T) {
	g := NewGrammar("extra_chain_nested_state")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Token(Pat(`.`))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("block_comment"))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	tables, ctx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build LR tables: %v", err)
	}
	addNonterminalExtraChains(tables, ng, ctx)

	slashStarSyms := diagFindAllSymbols(ng, "/*")
	if len(slashStarSyms) != 1 {
		t.Fatalf("expected one /* symbol, got %v", slashStarSyms)
	}

	rootActs := tables.ActionTable[0][slashStarSyms[0]]
	if len(rootActs) != 1 || rootActs[0].kind != lrShift {
		t.Fatalf("expected synthetic extra-chain shift on /* from state 0, got %s", diagFormatActions(ng, rootActs))
	}
	outerState := rootActs[0].state
	if outerState < tables.ExtraChainStateStart {
		t.Fatalf("expected synthetic target >= %d, got %d", tables.ExtraChainStateStart, outerState)
	}

	nestedActs := tables.ActionTable[outerState][slashStarSyms[0]]
	if len(nestedActs) == 0 {
		t.Fatalf("expected nested extra shift on /* from synthetic state %d", outerState)
	}
	foundNestedShift := false
	for _, act := range nestedActs {
		if act.kind == lrShift && act.isExtra {
			foundNestedShift = true
			break
		}
	}
	if !foundNestedShift {
		t.Fatalf("expected nested extra shift on /* from synthetic state %d, got %s", outerState, diagFormatActions(ng, nestedActs))
	}
}

func TestNonterminalExtraChainSyntheticStatesPreferStructuralTokensOverExtraInjection(t *testing.T) {
	g := NewGrammar("extra_chain_structural_preference")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("comment", Seq(
		Token(Str("//")),
		Token(Pat(`.`)),
	))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Choice(Token(Pat(`.`)), Token(Str("//")))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("comment"), Sym("block_comment"))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	tables, ctx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build LR tables: %v", err)
	}
	addNonterminalExtraChains(tables, ng, ctx)

	slashStarSyms := diagFindAllSymbols(ng, "/*")
	if len(slashStarSyms) != 1 {
		t.Fatalf("expected one /* symbol, got %v", slashStarSyms)
	}
	slashSlashSyms := diagFindAllSymbols(ng, "//")
	if len(slashSlashSyms) != 1 {
		t.Fatalf("expected one // symbol, got %v", slashSlashSyms)
	}

	rootActs := tables.ActionTable[0][slashStarSyms[0]]
	if len(rootActs) != 1 || rootActs[0].kind != lrShift {
		t.Fatalf("expected synthetic extra-chain shift on /* from state 0, got %s", diagFormatActions(ng, rootActs))
	}
	outerState := rootActs[0].state
	if outerState < tables.ExtraChainStateStart {
		t.Fatalf("expected synthetic target >= %d, got %d", tables.ExtraChainStateStart, outerState)
	}

	actions := tables.ActionTable[outerState][slashSlashSyms[0]]
	if len(actions) == 0 {
		t.Fatalf("expected structural // action in synthetic state %d", outerState)
	}
	for _, act := range actions {
		if act.isExtra {
			t.Fatalf("synthetic state %d should not inject // as an extra when a structural action already exists: %s", outerState, diagFormatActions(ng, actions))
		}
	}
}

func TestNonterminalExtraChainSyntheticReduceStatesDoNotInjectNestedExtraStarts(t *testing.T) {
	g := NewGrammar("extra_chain_reduce_state")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Choice(Token(Pat(`.`)), Token(Str("//")))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("block_comment"))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	tables, ctx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build LR tables: %v", err)
	}
	addNonterminalExtraChains(tables, ng, ctx)

	slashStarSyms := diagFindAllSymbols(ng, "/*")
	if len(slashStarSyms) != 1 {
		t.Fatalf("expected one /* symbol, got %v", slashStarSyms)
	}
	closeCommentSyms := diagFindAllSymbols(ng, "*/")
	if len(closeCommentSyms) != 1 {
		t.Fatalf("expected one */ symbol, got %v", closeCommentSyms)
	}

	rootActs := tables.ActionTable[0][slashStarSyms[0]]
	if len(rootActs) != 1 || rootActs[0].kind != lrShift {
		t.Fatalf("expected synthetic extra-chain shift on /* from state 0, got %s", diagFormatActions(ng, rootActs))
	}
	outerState := rootActs[0].state

	closeActs := tables.ActionTable[outerState][closeCommentSyms[0]]
	if len(closeActs) == 0 {
		t.Fatalf("expected */ shift from synthetic state %d", outerState)
	}
	closeState := -1
	for _, act := range closeActs {
		if act.kind == lrShift {
			closeState = act.state
			break
		}
	}
	if closeState < 0 {
		t.Fatalf("expected shift on */ from synthetic state %d, got %s", outerState, diagFormatActions(ng, closeActs))
	}

	actions := tables.ActionTable[closeState][slashStarSyms[0]]
	if len(actions) == 0 {
		t.Fatalf("expected reduce lookahead on /* from synthetic reduce state %d", closeState)
	}
	for _, act := range actions {
		if act.kind == lrShift {
			t.Fatalf("synthetic reduce state %d should not inject nested extra starts: %s", closeState, diagFormatActions(ng, actions))
		}
	}
}

func TestNonterminalExtraChainRuntimeSupportsNestedExtras(t *testing.T) {
	g := NewGrammar("extra_chain_nested_runtime")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Token(Pat(`.`))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("block_comment"))

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tree, err := gotreesitter.NewParser(report.Language).Parse([]byte("/*a/*b*/c*/foo"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasError() {
		t.Fatalf("root has error: %s", safeSExpr(root, report.Language, 16))
	}
	if root.ChildCount() < 2 {
		t.Fatalf("root child count = %d, want at least 2; sexpr=%s", root.ChildCount(), safeSExpr(root, report.Language, 32))
	}
	outer := root.Child(0)
	if got := outer.Type(report.Language); got != "block_comment" {
		t.Fatalf("child[0] type = %q, want block_comment; sexpr=%s", got, safeSExpr(root, report.Language, 32))
	}
	if got := root.Child(1).Type(report.Language); got != "item" {
		t.Fatalf("child[1] type = %q, want item; sexpr=%s", got, safeSExpr(root, report.Language, 32))
	}
	if got := safeSExpr(root, report.Language, 32); got != "(source_file (block_comment (block_comment)) (item))" {
		t.Fatalf("sexpr = %s, want nested block_comment shape", got)
	}
}

func TestNonterminalExtraChainRuntimeMatchesScalaStyleNestedBlockComments(t *testing.T) {
	g := NewGrammar("extra_chain_scala_style")
	g.Define("source_file", Repeat1(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("comment", Seq(
		Token(Str("//")),
		Repeat(Token(Pat(`[^\n]`))),
	))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Choice(Token(Pat(`[\s\S]`)), Token(Str("//")))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("comment"), Sym("block_comment"))

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	src := []byte(`/**/
/** comment 1
 * /* comment 2
 *  /* / * * /comment 3 */
 // comment 4
 * @param
 *  */
*/
foo`)
	tree, err := gotreesitter.NewParser(report.Language).Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasError() {
		t.Fatalf("root has error: %s", safeSExpr(root, report.Language, 64))
	}
	if got := safeSExpr(root, report.Language, 64); got != "(source_file (block_comment) (block_comment (block_comment (block_comment))) (item))" {
		t.Fatalf("sexpr = %s, want Scala-style nested block_comment shape", got)
	}
}

func TestNonterminalExtraChainRuntimeMatchesScalaStyleNestedBlockCommentsAtEOF(t *testing.T) {
	g := NewGrammar("extra_chain_scala_style_eof")
	g.Define("source_file", Repeat(Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.Define("comment", Seq(
		Token(Str("//")),
		Repeat(Token(Pat(`[^\n]`))),
	))
	g.Define("block_comment", Seq(
		Token(Str("/*")),
		Repeat(Choice(Token(Pat(`[\s\S]`)), Token(Str("//")))),
		Token(Str("*/")),
	))
	g.SetExtras(Pat(`\s`), Sym("comment"), Sym("block_comment"))

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	src := []byte(`/**/
/** comment 1
 * /* comment 2
 *  /* / * * /comment 3 */
 // comment 4
 * @param
 *  */
*/`)
	tree, err := gotreesitter.NewParser(report.Language).Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasError() {
		t.Fatalf("root has error: %s", safeSExpr(root, report.Language, 64))
	}
	if got := safeSExpr(root, report.Language, 64); got != "(source_file (block_comment) (block_comment (block_comment (block_comment))))" {
		t.Fatalf("sexpr = %s, want Scala-style nested block_comment EOF shape", got)
	}
}
