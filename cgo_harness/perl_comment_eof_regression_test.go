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
// The expected shape below is not read from the C oracle this repo's lock
// pins (grammars/languages.lock still names tree-sitter-perl@ad74e6db, the
// commit immediately BEFORE d5ae131): that pinned commit carries the exact
// same unbounded scanner loop the fix removes, so a from-lock C oracle build
// hangs on this input too. Confirmed directly, three ways, before writing
// this test: `go run` against the unfixed Go port timed out; ParityCLanguage
// ("perl") against the ad74e6db oracle timed out after 10 minutes inside a
// Docker container (--memory 4g --cpus 2); and tree-sitter-perl's own CLI
// (`tree-sitter generate && tree-sitter parse -r`) spun at 100% CPU and had
// to be killed. The expected shape instead comes from running that same CLI
// reproduction recipe against tree-sitter-perl@d5ae131 (the fix commit
// itself), which returns immediately:
//
//	(source_file [0, 0] - [0, 3]
//	  (expression_statement [0, 0] - [0, 1]
//	    (bareword [0, 0] - [0, 1]))
//	  (comment [0, 2] - [0, 3]))
//
// This test still builds and runs the from-lock C oracle (ParityCLanguage
// ("perl")), so a future lock bump that carries the upstream fix keeps this
// test honest against the live pinned oracle. The parse runs on a goroutine
// with a bounded deadline: today, against the pre-fix lock, it is expected
// to time out, and the test reports that plainly instead of hanging the
// suite. A future bump past d5ae131 should make the oracle answer within
// the deadline and this test should then assert its answer directly instead
// of the hardcoded reference shape.
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
		t.Skip("C oracle at the currently pinned tree-sitter-perl@ad74e6db reproduces the pre-fix infinite loop on this input (expected; see commit 71b727e..d5ae131). Skipping instead of hanging the suite. The Go-side fix is pinned by TestPerlCommentAtEOFWithoutTrailingNewline in package grammars, verified against tree-sitter-perl@d5ae131 directly.")
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
