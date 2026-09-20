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
// comment span and has_error flag from the locked tree-sitter-ocaml C oracle
// instead of assuming them, so a future upstream scanner change is caught
// here rather than only pinned as a fixed host expectation.
//
// This test compares the scanner-owned comment token only, not the full
// downstream tree: the C oracle recovers from the malformed trailer
// ("' d *)") with a finer-grained error/expression_item/error split than
// gotreesitter's single catch-all ERROR node. That split is a general GLR
// error-recovery granularity difference (ocaml is not in the curated
// full-C-parity language set), not a property of the external scanner this
// port touches. The scanner-level claim this port makes — where the comment
// ends, and that the parse has an error at all — is what this test pins.
func TestOCamlCommentCharLiteralInnerCloseIsCExact(t *testing.T) {
	source := []byte("(* c '*)' d *)\n")

	cRoot, closeC := ocamlCommentScannerCOracleRoot(t, source)
	defer closeC()
	if !cRoot.HasError() {
		t.Fatalf("C oracle unexpectedly parsed this witness cleanly; the divergence this test pins may no longer exist upstream")
	}
	if cRoot.ChildCount() == 0 {
		t.Fatalf("C oracle root has no children: %v", cRoot)
	}
	cComment := cRoot.Child(0)
	if cComment == nil || cComment.Kind() != "comment" {
		t.Fatalf("C oracle root's first child is not a comment node")
	}
	cStart, cEnd := cComment.StartByte(), cComment.EndByte()

	goLang := grammars.OcamlLanguage()
	tree, err := gotreesitter.NewParser(goLang).Parse(source)
	if err != nil {
		t.Fatalf("gotreesitter parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if !root.HasError() {
		t.Fatalf("gotreesitter parsed this witness cleanly; the C oracle reports has_error=1: %s", root.SExpr(goLang))
	}
	goComment := root.Child(0)
	if goComment == nil || goComment.Type(goLang) != "comment" {
		t.Fatalf("gotreesitter root's first child is not a comment node: %s", root.SExpr(goLang))
	}
	if goComment.StartByte() != uint32(cStart) || goComment.EndByte() != uint32(cEnd) {
		t.Fatalf("comment span go=[%d-%d] c=[%d-%d]", goComment.StartByte(), goComment.EndByte(), cStart, cEnd)
	}
}

// TestOCamlCommentUnterminatedAtEOFIsCExact is the C-oracle twin of
// TestOCamlCommentUnterminatedAtEOFMatchesUpstream
// (grammars/ocaml_comment_scanner_regression_test.go). Unlike the char-literal
// case above, this witness has no downstream error-recovery structure to
// diverge on (the whole input is the one comment token), so a full recursive
// tree comparison against the oracle applies directly.
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
