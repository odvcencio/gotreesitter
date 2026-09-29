package grammars_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestHTTPEmptySectionSelectionMatchesC(t *testing.T) {
	entry := grammars.DetectLanguageByName("http")
	if entry == nil || entry.Language == nil {
		t.Fatal("http grammar is not registered")
	}
	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte("#t\n\n"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse has errors: %s", root.SExpr(lang))
	}
	const want = "(document (section (comment)))"
	if got := root.SExpr(lang); got != want {
		t.Fatalf("tree differs from C oracle:\n got: %s\nwant: %s", got, want)
	}
}
