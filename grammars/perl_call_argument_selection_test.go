package grammars_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPerlCallArgumentSelectionMatchesC(t *testing.T) {
	entry := grammars.DetectLanguageByName("perl")
	if entry == nil || entry.Language == nil {
		t.Fatal("perl grammar is not registered")
	}
	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte("{l$s, }"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse has errors: %s", root.SExpr(lang))
	}
	const want = "(source_file (block_statement (expression_statement (ambiguous_function_call_expression (function) (list_expression (scalar (varname)))))))"
	if got := root.SExpr(lang); got != want {
		t.Fatalf("tree differs from C oracle:\n got: %s\nwant: %s", got, want)
	}
}
