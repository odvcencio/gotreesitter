package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestJuliaReturnRangeRecoveryCompatibility(t *testing.T) {
	lang := grammars.JuliaLanguage()
	if lang == nil {
		t.Fatal("JuliaLanguage returned nil")
	}
	source := []byte("function f()\n    while true\n        return a : b\n    end\n    return (lo + 1) : (hi - 1)\nend\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	bareReturn := findNodeByText(tree.RootNode(), lang, source, "return_statement", "return a : b")
	if bareReturn == nil {
		t.Fatalf("bare return_statement not found:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := bareReturn.ChildCount(), 3; got != want {
		t.Fatalf("bare return child count = %d, want %d; tree:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	if got := bareReturn.Child(1).Type(lang); got != "ERROR" {
		t.Fatalf("bare return child[1] = %q, want ERROR", got)
	}
	if got := bareReturn.Child(2).Type(lang); got != "quote_expression" {
		t.Fatalf("bare return child[2] = %q, want quote_expression", got)
	}

	parenReturn := findNodeByText(tree.RootNode(), lang, source, "return_statement", "return (lo + 1) : (hi - 1")
	if parenReturn == nil {
		t.Fatalf("parenthesized return_statement not found:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := parenReturn.EndByte(), uint32(86); got != want {
		t.Fatalf("parenthesized return end = %d, want %d", got, want)
	}
	parent := parenReturn.Parent()
	if parent == nil || parent.Type(lang) != "block" {
		t.Fatalf("parenthesized return parent = %v, want block", parent)
	}
	if got, want := parent.ChildCount(), 3; got != want {
		t.Fatalf("block child count = %d, want %d; tree:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	trailing := parent.Child(2)
	if got := trailing.Type(lang); got != "ERROR" {
		t.Fatalf("block trailing child = %q, want ERROR", got)
	}
	if got := trailing.Text(source); got != ")" {
		t.Fatalf("block trailing ERROR text = %q, want %q", got, ")")
	}
}

func TestJuliaMacroArgumentJuxtapositionCompatibility(t *testing.T) {
	lang := grammars.JuliaLanguage()
	if lang == nil {
		t.Fatal("JuliaLanguage returned nil")
	}
	source := []byte("@assert 3nstmts == length(di.codelocs)\n@assert workspace.visiting[child] == length(workspace.stack) + 1 \"internal error maintaining workspace\"\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	leading := findNodeByText(tree.RootNode(), lang, source, "macro_argument_list", "3nstmts == length(di.codelocs)")
	if leading == nil {
		t.Fatalf("leading macro_argument_list not found:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := leading.ChildCount(), 1; got != want {
		t.Fatalf("leading macro arg child count = %d, want %d; tree:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	leadingExpr := leading.Child(0)
	if got := leadingExpr.Type(lang); got != "binary_expression" {
		t.Fatalf("leading child type = %q, want binary_expression", got)
	}
	leadingJuxtaposition := findNodeByText(leadingExpr, lang, source, "juxtaposition_expression", "3nstmts")
	if leadingJuxtaposition == nil {
		t.Fatalf("leading juxtaposition not found:\n%s", leadingExpr.SExpr(lang))
	}

	trailing := findNodeByText(tree.RootNode(), lang, source, "macro_argument_list", "workspace.visiting[child] == length(workspace.stack) + 1 \"internal error maintaining workspace\"")
	if trailing == nil {
		t.Fatalf("trailing macro_argument_list not found:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := trailing.ChildCount(), 1; got != want {
		t.Fatalf("trailing macro arg child count = %d, want %d; tree:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	trailingJuxtaposition := findNodeByText(trailing.Child(0), lang, source, "juxtaposition_expression", "1 \"internal error maintaining workspace\"")
	if trailingJuxtaposition == nil {
		t.Fatalf("trailing juxtaposition not found:\n%s", trailing.Child(0).SExpr(lang))
	}
}

func TestJuliaIndexSingleRowMatrixCompatibility(t *testing.T) {
	lang := grammars.JuliaLanguage()
	if lang == nil {
		t.Fatal("JuliaLanguage returned nil")
	}
	source := []byte("edge = di.codelocs[3i-1]\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	index := findNodeByText(tree.RootNode(), lang, source, "index_expression", "di.codelocs[3i-1]")
	if index == nil {
		t.Fatalf("index_expression not found:\n%s", tree.RootNode().SExpr(lang))
	}
	vector := findNodeByText(index, lang, source, "vector_expression", "[3i-1]")
	if vector == nil {
		t.Fatalf("vector_expression not found in index; tree:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := vector.ChildCount(), 3; got != want {
		t.Fatalf("vector child count = %d, want %d; tree:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	if got := vector.Child(1).Type(lang); got != "binary_expression" {
		t.Fatalf("vector child[1] = %q, want binary_expression; tree:\n%s", got, tree.RootNode().SExpr(lang))
	}
}

func TestJuliaBracketForComprehensionCompatibility(t *testing.T) {
	lang := grammars.JuliaLanguage()
	if lang == nil {
		t.Fatal("JuliaLanguage returned nil")
	}
	source := []byte("function f()\n    states = Union{T,Nothing}[\n        State(slottypes[slot])\n        for slot = 1:nslots\n    ]\nend\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	index := findNodeByText(tree.RootNode(), lang, source, "index_expression", "Union{T,Nothing}[\n        State(slottypes[slot])\n        for slot = 1:nslots\n    ]")
	if index == nil {
		t.Fatalf("index_expression not found:\n%s", tree.RootNode().SExpr(lang))
	}
	comprehension := findNodeByText(index, lang, source, "comprehension_expression", "[\n        State(slottypes[slot])\n        for slot = 1:nslots\n    ]")
	if comprehension == nil {
		t.Fatalf("comprehension_expression not found in index; tree:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := comprehension.ChildCount(), 4; got != want {
		t.Fatalf("comprehension child count = %d, want %d; tree:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	if got := comprehension.Child(1).Type(lang); got != "call_expression" {
		t.Fatalf("comprehension child[1] = %q, want call_expression; tree:\n%s", got, tree.RootNode().SExpr(lang))
	}
	if got := comprehension.Child(2).Type(lang); got != "for_clause" {
		t.Fatalf("comprehension child[2] = %q, want for_clause; tree:\n%s", got, tree.RootNode().SExpr(lang))
	}
	if got := comprehension.Child(2).Text(source); got != "for slot = 1:nslots" {
		t.Fatalf("for_clause text = %q, want %q", got, "for slot = 1:nslots")
	}
}

// TestJuliaTrailingCommaAssignmentTupleCompatibility pins the tree shape for
// a lexer-skipped "=" between a trailing tuple comma and its next real
// token. The shape moved (campaign v7 class-e closure,
// spore.2026-08-02.hornbeam-e.byte-continuity): retiring the compact
// scheduler's bytesAreSingleByteDecorationTrivia exemption
// (parsercore_phase0_driver.go) makes compact correctly decline this input
// (the "=" gap is a genuine byte-coverage hole, not tolerable trivia) and
// fall back to production. Production's own gap materialization already
// changed independently, in the already-merged PR #633
// ("materializeSkippedGapAsExtraError", parser.go): a lexer-skipped gap
// mid-separated-list now gets a real, transparent EXTRA ERROR leaf spanning
// the whole gap (here " = ", both flanking spaces included) instead of the
// old silent byte-offset bump the language-specific
// normalizeJuliaTrailingCommaAssignmentTuple (parser_result_julia.go)
// synthesized a narrower ERROR for after the fact. That normalizer's own
// pattern match (comma directly followed by the next real token, with no
// node at all between them) no longer applies once production's own general
// mechanism already inserts the ERROR node first, so it now correctly
// no-ops for this input; the general, per-parse fix it was standing in for
// (see materializeSkippedGapAsExtraError's own doc comment, parser.go --
// normalizeJuliaTrailingCommaAssignmentTuple itself carries no doc comment
// of its own) has arrived. This test was previously masked from ever
// exercising this exact shape by the same falsified
// bytesAreSingleByteDecorationTrivia exemption: compact used to accept the
// input silently (no error), so this Parse call -- which does not force a
// route -- never fell back to the now-current production shape until the
// exemption was retired.
//
// The pinned shape below (Text()==" = ", ChildCount()==0) itself diverges
// from the C oracle, which reports this same ERROR as Text()=="=" with one
// child. That divergence is not this test's own defect and is not new: it
// is the known, already-documented accounting-not-shape gap
// materializeSkippedGapAsExtraError's own doc comment names explicitly (see
// "This is an ACCOUNTING fix, not a shape fix", parser.go) -- the leaf's
// span still covers the whole lexer-skipped gap rather than only the true
// stray token, and the leaf is still childless where C wraps the stray as
// its own child, both named there as open follow-up work. This test pins
// production's actual, current output, not a claim that this output
// matches C.
func TestJuliaTrailingCommaAssignmentTupleCompatibility(t *testing.T) {
	lang := grammars.JuliaLanguage()
	if lang == nil {
		t.Fatal("JuliaLanguage returned nil")
	}
	source := []byte("function f()\n    minarg, maxarg, = T_IFUNC[iidx]\nend\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	if !tree.RootNode().HasError() {
		t.Fatalf("root HasError=false, want true (a lexer-skipped \"=\" is a real error); tree:\n%s", tree.RootNode().SExpr(lang))
	}

	tuple := findNodeByText(tree.RootNode(), lang, source, "open_tuple", "minarg, maxarg, = T_IFUNC[iidx]")
	if tuple == nil {
		t.Fatalf("open_tuple not found:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := tuple.ChildCount(), 6; got != want {
		t.Fatalf("open_tuple child count = %d, want %d; tree:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	if got := tuple.Child(4).Type(lang); got != "ERROR" {
		t.Fatalf("open_tuple child[4] = %q, want ERROR; tree:\n%s", got, tree.RootNode().SExpr(lang))
	}
	if got := tuple.Child(4).Text(source); got != " = " {
		t.Fatalf("open_tuple ERROR text = %q, want %q", got, " = ")
	}
	if !tuple.Child(4).IsExtra() {
		t.Fatalf("open_tuple ERROR IsExtra=false, want true (materializeSkippedGapAsExtraError, parser.go); tree:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := tuple.Child(4).ChildCount(), 0; got != want {
		t.Fatalf("open_tuple ERROR child count = %d, want %d (a flat EXTRA leaf, not a wrapped operator); tree:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
}

func findNodeByText(root *gotreesitter.Node, lang *gotreesitter.Language, source []byte, typ, text string) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if root.Type(lang) == typ && root.Text(source) == text {
		return root
	}
	for i := 0; i < root.ChildCount(); i++ {
		if found := findNodeByText(root.Child(i), lang, source, typ, text); found != nil {
			return found
		}
	}
	return nil
}
