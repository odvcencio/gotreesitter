//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"
	"time"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPerlRecoverParenCloseMatchesCleanCOracleShapeCOracle is the C-oracle
// twin of grammars.TestPerlRecoverParenCloseMatchesCleanCOracleShape. Both
// pin the fix for a GLR-engine gap the tree-sitter-perl 8917c6e9 bump
// exposed: a starved GLR version that needs a zero-width external token
// (_NONASSOC) before it can accept the shared lookahead used to die with no
// action, leaving the wrong version to recover a MISSING ')' on its own.
//
// grammars/languages.lock now pins tree-sitter-perl@8917c6e9, so
// ParityCLanguage builds the C oracle at the same commit this test's Go side
// targets: no historical-commit workaround is needed here (contrast
// TestPerlCommentAtEOFWithoutTrailingNewlineCOracle, whose oracle commit
// predates its fix).
func TestPerlRecoverParenCloseMatchesCleanCOracleShapeCOracle(t *testing.T) {
	source := []byte("foo(1, 2;\n")
	const wantSExpr = "(source_file (expression_statement (function_call_expression function: (function) arguments: (list_expression (number) (number)))))"

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
			t.Fatalf("C oracle sexpr = %s, want %s", got, wantSExpr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("C oracle at tree-sitter-perl@8917c6e9 did not return within 10s")
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
	if got := goRoot.SExpr(goLanguage); got != "(source_file (expression_statement (function_call_expression (function) (list_expression (number) (number)))))" {
		t.Fatalf("Go sexpr = %s, want the clean function_call_expression shape", got)
	}
}
