//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestRRawStringContentEndingInClosingBracketCOracle pins the scanner split
// ported from upstream tree-sitter-r commit 3ee0e0a ("Split raw strings into
// open/content/close (#199)"), part of the 58a22794466c grammar bump,
// against the real C tree-sitter oracle.
//
// `r"(a))"` has content "a)": a literal ")" sits right before the real close
// ")\"". A scanner that advanced past a failed close attempt unconditionally,
// instead of re-examining the lookahead, would swallow the close's opening
// ")" as content and never find a matching close.
func TestRRawStringContentEndingInClosingBracketCOracle(t *testing.T) {
	source := []byte(`x <- r"(a))"`)

	cLanguage, err := ParityCLanguage("r")
	if err != nil {
		t.Skipf("C r oracle unavailable: %v", err)
	}
	cTree := compactT3ParseC(t, cLanguage, source)
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot == nil {
		t.Fatal("r C oracle returned no root")
	}
	if cRoot.HasError() {
		t.Fatalf("r C oracle root has error:\n%s", dumpCTree(cRoot, 0))
	}

	goLanguage := grammars.RLanguage()
	goParser := gotreesitter.NewParser(goLanguage)
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("r Go parse: %v", err)
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()
	if goRoot == nil {
		t.Fatal("r Go parse returned no root")
	}
	if goRoot.HasError() {
		t.Fatalf("r Go root has error:\n%s", dumpGoTree(goRoot, goLanguage, 0))
	}

	var errs []string
	compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
	if len(errs) != 0 {
		t.Fatalf(
			"r Go/C raw-string tree diverged:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
			joinTopErrors(errs),
			goRoot.SExpr(goLanguage),
			dumpGoTree(goRoot, goLanguage, 0),
			cRoot.ToSexp(),
			dumpCTree(cRoot, 0),
		)
	}

	wantSExpr := "(program (binary_operator (identifier) (string (string_open) (string_content) (string_close))))"
	if got := goRoot.SExpr(goLanguage); got != wantSExpr {
		t.Fatalf("r Go S-expression = %s, want %s", got, wantSExpr)
	}
	if got := fmt.Sprintf("%d", goRoot.EndByte()); got != fmt.Sprintf("%d", len(source)) {
		t.Fatalf("r Go root EndByte=%s, want %d (source: %q)", got, len(source), source)
	}
}

// TestRElseKeywordNotConsumedFromLongerIdentifierCOracle pins the fix ported
// from upstream tree-sitter-r commit 40899e0 ("Don't consume `else` if it is
// part of a larger `identifier`" (#201)), part of the 58a22794466c grammar
// bump, against the real C tree-sitter oracle.
//
// Before the fix, the external scanner greedily matched the four bytes
// "else" as the ELSE token even when they were really the prefix of a longer
// identifier like "else_idx", splitting "else_idx <- 1" into a stray ELSE
// token and an unparsable "_idx <- 1" tail.
func TestRElseKeywordNotConsumedFromLongerIdentifierCOracle(t *testing.T) {
	source := []byte("{\nif (TRUE) 1\nelse_idx <- 1\n}")

	cLanguage, err := ParityCLanguage("r")
	if err != nil {
		t.Skipf("C r oracle unavailable: %v", err)
	}
	cTree := compactT3ParseC(t, cLanguage, source)
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot == nil {
		t.Fatal("r C oracle returned no root")
	}
	if cRoot.HasError() {
		t.Fatalf("r C oracle root has error:\n%s", dumpCTree(cRoot, 0))
	}

	goLanguage := grammars.RLanguage()
	goParser := gotreesitter.NewParser(goLanguage)
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("r Go parse: %v", err)
	}
	defer goTree.Release()
	goRoot := goTree.RootNode()
	if goRoot == nil {
		t.Fatal("r Go parse returned no root")
	}
	if goRoot.HasError() {
		t.Fatalf("r Go root has error:\n%s", dumpGoTree(goRoot, goLanguage, 0))
	}

	var errs []string
	compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
	if len(errs) != 0 {
		t.Fatalf(
			"r Go/C else-identifier tree diverged:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
			joinTopErrors(errs),
			goRoot.SExpr(goLanguage),
			dumpGoTree(goRoot, goLanguage, 0),
			cRoot.ToSexp(),
			dumpCTree(cRoot, 0),
		)
	}

	wantSExpr := "(program (braced_expression (if_statement (true) (float)) (binary_operator (identifier) (float))))"
	if got := goRoot.SExpr(goLanguage); got != wantSExpr {
		t.Fatalf("r Go S-expression = %s, want %s", got, wantSExpr)
	}
}
