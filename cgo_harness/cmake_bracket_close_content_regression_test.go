//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCmakeBracketArgumentContentEndingInCloseBracketCOracle pins the fix
// ported from upstream tree-sitter-cmake commit 3725810 ("fix: handle
// bracketed strings with `]` at the end of its content"), part of the
// 58993af75218 grammar bump, against the real C tree-sitter oracle.
//
// "[=[a]]=]" is a level-1 bracket argument whose content "a]" ends in a
// literal "]" immediately before the real close "]=]". Before the fix, the
// Go scanner's failed-close-match path consumed one extra byte
// unconditionally instead of re-examining the current lookahead, swallowing
// the close's opening "]" as ordinary content and never finding a matching
// close.
func TestCmakeBracketArgumentContentEndingInCloseBracketCOracle(t *testing.T) {
	source := []byte("message([=[a]]=])")

	cLanguage, err := ParityCLanguage("cmake")
	if err != nil {
		t.Skipf("C cmake oracle unavailable: %v", err)
	}
	cTree := compactT3ParseC(t, cLanguage, source)
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot == nil {
		t.Fatal("cmake C oracle returned no root")
	}
	if cRoot.HasError() {
		t.Fatalf("cmake C oracle root has error:\n%s", dumpCTree(cRoot, 0))
	}

	goLanguage := grammars.CmakeLanguage()
	goParser := gotreesitter.NewParser(goLanguage)
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("cmake Go parse: %v", err)
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()
	if goRoot == nil {
		t.Fatal("cmake Go parse returned no root")
	}
	if goRoot.HasError() {
		t.Fatalf("cmake Go root has error:\n%s", dumpGoTree(goRoot, goLanguage, 0))
	}

	var errs []string
	compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
	if len(errs) != 0 {
		t.Fatalf(
			"cmake Go/C bracket-argument tree diverged:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
			joinTopErrors(errs),
			goRoot.SExpr(goLanguage),
			dumpGoTree(goRoot, goLanguage, 0),
			cRoot.ToSexp(),
			dumpCTree(cRoot, 0),
		)
	}

	wantSExpr := "(source_file (normal_command (identifier) (argument_list (argument (bracket_argument (bracket_argument_open) (bracket_argument_content) (bracket_argument_close))))))"
	if got := goRoot.SExpr(goLanguage); got != wantSExpr {
		t.Fatalf("cmake Go S-expression = %s, want %s", got, wantSExpr)
	}
	if got := fmt.Sprintf("%d", goRoot.EndByte()); got != fmt.Sprintf("%d", len(source)) {
		t.Fatalf("cmake Go root EndByte=%s, want %d (source: %q)", got, len(source), source)
	}
}
