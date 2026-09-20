//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// ocamlCommentScannerCOracleRoot loads the locked OCaml C oracle and parses
// source, returning its root node and a close func the caller must defer.
func ocamlCommentScannerCOracleRoot(t *testing.T, source []byte) (*sitter.Node, func()) {
	t.Helper()
	cLang, err := COracleLanguage("ocaml")
	if err != nil {
		t.Fatalf("load OCaml C oracle: %v", err)
	}
	cParser := sitter.NewParser()
	if err := cParser.SetLanguage(cLang); err != nil {
		cParser.Close()
		t.Fatalf("set OCaml C oracle language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		cParser.Close()
		t.Fatal("C oracle parse returned no root")
	}
	return cTree.RootNode(), func() {
		cTree.Close()
		cParser.Close()
	}
}

// TestOCamlCommentCharLiteralInnerCloseIsCExact is the C-oracle twin of
// TestOCamlCommentCharLiteralInnerCloseMatchesUpstream
// (grammars/ocaml_comment_scanner_regression_test.go). It reads the expected
// tree shape from the locked tree-sitter-ocaml C oracle instead of assuming
// it, so a future upstream scanner change is caught here rather than only
// pinned as a fixed host expectation.
func TestOCamlCommentCharLiteralInnerCloseIsCExact(t *testing.T) {
	source := []byte("(* c '*)' d *)\n")

	cRoot, closeC := ocamlCommentScannerCOracleRoot(t, source)
	defer closeC()
	if !cRoot.HasError() {
		t.Fatalf("C oracle unexpectedly parsed this witness cleanly; the divergence this test pins may no longer exist upstream")
	}

	goLang := grammars.OcamlLanguage()
	tree, err := gotreesitter.NewParser(goLang).Parse(source)
	if err != nil {
		t.Fatalf("gotreesitter parse: %v", err)
	}
	defer tree.Release()

	var mismatches []string
	compareNodes(tree.RootNode(), goLang, cRoot, "root", &mismatches)
	if len(mismatches) != 0 {
		t.Fatalf("gotreesitter diverges from the C oracle at %d point(s):\n%s", len(mismatches), strings.Join(mismatches, "\n"))
	}
}

// TestOCamlCommentUnterminatedAtEOFIsCExact is the C-oracle twin of
// TestOCamlCommentUnterminatedAtEOFMatchesUpstream
// (grammars/ocaml_comment_scanner_regression_test.go).
func TestOCamlCommentUnterminatedAtEOFIsCExact(t *testing.T) {
	source := []byte("(* unterminated")

	cRoot, closeC := ocamlCommentScannerCOracleRoot(t, source)
	defer closeC()
	if cRoot.HasError() {
		t.Fatalf("C oracle unexpectedly reported has_error=1 for this witness; the divergence this test pins may no longer exist upstream")
	}

	goLang := grammars.OcamlLanguage()
	tree, err := gotreesitter.NewParser(goLang).Parse(source)
	if err != nil {
		t.Fatalf("gotreesitter parse: %v", err)
	}
	defer tree.Release()

	var mismatches []string
	compareNodes(tree.RootNode(), goLang, cRoot, "root", &mismatches)
	if len(mismatches) != 0 {
		t.Fatalf("gotreesitter diverges from the C oracle at %d point(s):\n%s", len(mismatches), strings.Join(mismatches, "\n"))
	}
}
