package grammars

import (
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

// TestPerlRecoverParenCloseMatchesCleanCOracleShape pins the fix for a
// GLR-engine gap the tree-sitter-perl 8917c6e9 bump exposed (adds the
// zero-width external _RECOVER_PAREN_CLOSE, external index 38).
//
// C lexes once per parse version. At byte 4 in "foo(1, 2;\n" (the '('),
// state 976 forks into a version that wants the DFA `number` token and a
// version that first needs the zero-width external `_NONASSOC` before
// `number` has any action. This engine used to lex one token per frontier
// iteration for every live version, so the shared lexer's choice of
// `number` starved the `_NONASSOC` version with no action and killed it,
// leaving only the `number` version to recover the missing ')' on its own:
// an ambiguous_function_call_expression with a MISSING ')' and
// HasError()=true. The C oracle at 8917c6e9 parses the same bytes cleanly.
//
// relexTokenForStackLexState's zero-width-external rescue
// (parser_recover_c.go) fixes the underlying engine gap: a starved version
// can now shift a zero-width external token from its own lex mode before
// retrying the shared lookahead. See that function's doc for the full
// mechanism.
func TestPerlRecoverParenCloseMatchesCleanCOracleShape(t *testing.T) {
	const src = "foo(1, 2;\n"
	const wantSexpr = "(source_file (expression_statement (function_call_expression (function) (list_expression (number) (number)))))"

	var entry LangEntry
	found := false
	for _, e := range AllLanguages() {
		if e.Name == "perl" {
			entry, found = e, true
			break
		}
	}
	if !found {
		t.Fatal("perl language not registered")
	}
	UnloadEmbeddedLanguage(entry.Name + ".bin")
	t.Cleanup(func() { UnloadEmbeddedLanguage(entry.Name + ".bin") })
	lang := entry.Language()
	report := EvaluateParseSupport(entry, lang)
	parser := gotreesitter.NewParser(lang)
	b := []byte(src)

	type result struct {
		tree *gotreesitter.Tree
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var tree *gotreesitter.Tree
		var err error
		if report.Backend == ParseBackendTokenSource {
			tree, err = parser.ParseWithTokenSource(b, entry.TokenSourceFactory(b, lang))
		} else {
			tree, err = parser.Parse(b)
		}
		done <- result{tree: tree, err: err}
	}()

	var res result
	select {
	case res = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("perl: parsing \"foo(1, 2;\\n\" did not return within 5s")
	}
	if res.err != nil {
		t.Fatalf("perl parse failed: %v", res.err)
	}
	if res.tree == nil || res.tree.RootNode() == nil {
		t.Fatal("perl parse returned nil root")
	}
	defer res.tree.Release()

	root := res.tree.RootNode()
	if root.HasError() {
		t.Fatalf("perl: expected a clean parse (HasError()=false), got:\n%s", sexpr(root, lang))
	}
	if got := sexpr(root, lang); got != wantSexpr {
		t.Fatalf("perl: S-expression mismatch\n got: %s\nwant: %s", got, wantSexpr)
	}
}
