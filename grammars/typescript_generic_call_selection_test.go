package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestTypeScriptGenericCallSelectionMatchesC(t *testing.T) {
	lang := grammars.DetectLanguageByName("typescript").Language()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte("5<u,t>('',(x)=>'')"))
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned a nil tree or root")
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse returned an error tree: %s", root.SExpr(lang))
	}
	const want = "(program (expression_statement (call_expression (number) (type_arguments (type_identifier) (type_identifier)) (arguments (string) (arrow_function (formal_parameters (required_parameter (identifier))) (string))))))"
	if got := root.SExpr(lang); got != want {
		t.Fatalf("tree differs from C oracle\n got: %s\nwant: %s", got, want)
	}
}
