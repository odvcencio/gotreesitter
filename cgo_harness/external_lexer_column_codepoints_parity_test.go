//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCobolExternalLexerColumnCountsCodePoints pins ExternalLexer.Column()
// to C tree-sitter's code-point column contract using COBOL's fixed-format
// columns. Line 4 places a 3-byte rune (U+2713) in the columns 1-6 sequence
// area, followed by 5 ASCII characters, a blank column-7 indicator, and a
// DISPLAY statement in the code area.
//
// A byte-counting Column() (the pre-fix behavior) reaches byte-column 6
// after only 4 real characters (the 3-byte rune plus 3 ASCII characters),
// because the rune's byte width outruns its single code point. It then
// misreads the 5th real character, '*', as the column-7 indicator and
// swallows the rest of the line -- including the DISPLAY statement -- as a
// full-line comment. A code-point-counting Column() (matching C) reaches
// column 6 only after all 6 real characters, correctly treats the blank
// 7th character as the indicator, and parses DISPLAY 1. as a statement.
func TestCobolExternalLexerColumnCountsCodePoints(t *testing.T) {
	goLang := grammars.CobolLanguage()
	cLang, err := ParityCLanguage("cobol")
	if err != nil {
		t.Skipf("C parser unavailable: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("C SetLanguage: %v", err)
	}
	goParser := gotreesitter.NewParser(goLang)

	src := []byte(
		"       IDENTIFICATION DIVISION.\n" +
			"       PROGRAM-ID. A.\n" +
			"       PROCEDURE DIVISION.\n" +
			"✓123*6 DISPLAY 1.\n",
	)

	goTree, err := goParser.Parse(src)
	if err != nil {
		t.Fatalf("go parse: %v", err)
	}
	goRoot := goTree.RootNode()
	if goRoot == nil {
		t.Fatal("go nil root")
	}
	if goRoot.HasError() {
		t.Fatalf("go root has error:\n%s", goRoot.SExpr(goLang))
	}

	cTree := cParser.Parse(src, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C nil tree")
	}
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot.HasError() {
		t.Fatalf("C root has error:\n%s", dumpCTree(cRoot, 0))
	}

	var errs []string
	compareNodes(goRoot, goLang, cRoot, "root", &errs)
	if len(errs) > 0 {
		t.Fatalf("go-vs-C divergences:\n%s\n\ngo:\n%s\n\ngo spans:\n%s\n\nc:\n%s",
			joinTopErrors(errs), goRoot.SExpr(goLang), dumpGoTree(goRoot, goLang, 0), dumpCTree(cRoot, 0))
	}
}
