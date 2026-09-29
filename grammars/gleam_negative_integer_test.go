package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestGleamNegativeIntegerMatchesCShape(t *testing.T) {
	lang := grammars.GleamLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte("{-1}"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned no root")
	}
	if got, want := tree.RootNode().SExpr(lang), "(source_file (block (integer)))"; got != want {
		t.Fatalf("negative integer tree = %s, want C-compatible %s", got, want)
	}
}
