package grammars

import (
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

// TestPerlCommentAtEOFWithoutTrailingNewline pins the fix ported from
// upstream tree-sitter-perl commit d5ae131 ("fix: infinite loop when
// comment at EOF has no trailing newline").
//
// The autoquote lookahead's comment-skip loop used
// `for lexer.Column() != 0 { lexer.Advance(false); ... }` to skip to the
// next line. When the source ends with a comment and no trailing newline,
// the column never reaches 0 and Advance at EOF is a no-op, so the old
// condition spun forever. The parse itself runs on a goroutine with a short
// deadline, so a reintroduced infinite loop fails this test instead of
// hanging the whole suite; only the call that could hang (Parse) runs off
// the test goroutine, since testing.T failure methods are not safe to call
// from another goroutine.
func TestPerlCommentAtEOFWithoutTrailingNewline(t *testing.T) {
	const src = "x #"
	const wantSexpr = "(source_file (expression_statement (bareword)) (comment))"

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
		t.Fatal("perl: parsing a comment at EOF with no trailing newline did not return within 5s (infinite loop regression)")
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
		t.Fatal("perl: unexpected error node")
	}
	if got := sexpr(root, lang); got != wantSexpr {
		t.Fatalf("perl: S-expression mismatch\n got: %s\nwant: %s", got, wantSexpr)
	}
}
