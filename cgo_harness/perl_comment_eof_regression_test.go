//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"
	"time"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPerlCommentAtEOFWithoutTrailingNewlineCOracle is the C-oracle twin of
// grammars.TestPerlCommentAtEOFWithoutTrailingNewline. Both pin the fix
// ported from upstream tree-sitter-perl commit d5ae131 ("fix: infinite loop
// when comment at EOF has no trailing newline").
//
// grammars/languages.lock now pins tree-sitter-perl@8917c6e9, well past the
// fix commit, so the from-lock C oracle (ParityCLanguage("perl")) answers
// this input directly instead of hanging: the parse runs on a goroutine with
// a bounded deadline only as a safety net against a future regression, not
// because a timeout is expected. Before the lock carried the fix, this test
// instead expected the from-lock oracle to reproduce the pre-fix infinite
// loop and skipped; see git history for that version if useful context. The
// expected shape below was originally read from a standalone CLI
// reproduction against tree-sitter-perl@d5ae131 directly (`tree-sitter
// generate && tree-sitter parse -r`) and now matches the from-lock oracle's
// own answer:
//
//	(source_file [0, 0] - [0, 3]
//	  (expression_statement [0, 0] - [0, 1]
//	    (bareword [0, 0] - [0, 1]))
//	  (comment [0, 2] - [0, 3]))
func TestPerlCommentAtEOFWithoutTrailingNewlineCOracle(t *testing.T) {
	source := []byte("x #")
	const wantSExpr = "(source_file (expression_statement (bareword)) (comment))"

	cLanguage, err := ParityCLanguage("perl")
	if err != nil {
		t.Skipf("C perl oracle unavailable: %v", err)
	}

	type cResult struct {
		tree *sitter.Tree
	}
	done := make(chan cResult, 1)
	go func() {
		parser := sitter.NewParser()
		defer parser.Close()
		if err := parser.SetLanguage(cLanguage); err != nil {
			return
		}
		tree := parser.Parse(source, nil)
		done <- cResult{tree: tree}
	}()

	select {
	case res := <-done:
		if res.tree == nil || res.tree.RootNode() == nil {
			t.Fatal("C oracle parse returned no root")
		}
		defer res.tree.Close()
		cRoot := res.tree.RootNode()
		if cRoot.HasError() {
			t.Fatalf("C oracle root has error:\n%s", dumpCTree(cRoot, 0))
		}
		if got := cRoot.ToSexp(); got != wantSExpr {
			t.Fatalf("C oracle sexpr = %s, want %s (lock bumped past the fix: update this test to assert the oracle's own answer)", got, wantSExpr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("C oracle at tree-sitter-perl@8917c6e9 (well past the d5ae131 fix commit) did not return within 10s: possible regression of the infinite-loop fix in the pinned upstream scanner")
	}

	goLanguage := grammars.PerlLanguage()
	goParser := gotreesitter.NewParser(goLanguage)
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("Go parse: %v", err)
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()
	if goRoot.HasError() {
		t.Fatalf("Go root has error:\n%s", dumpGoTree(goRoot, goLanguage, 0))
	}
	if got := goRoot.SExpr(goLanguage); got != wantSExpr {
		t.Fatalf("Go sexpr = %s, want %s", got, wantSExpr)
	}
}
