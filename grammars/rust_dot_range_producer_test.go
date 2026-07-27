//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestRustDotRangeProducedBeforeCompatibility(t *testing.T) {
	lang := RustLanguage()
	source := []byte("fn f(s: &str) -> &str { &s[..] }\n")
	rawParser := gotreesitter.NewParser(lang)
	rawParser.SetAdmissionCandidateRoute(false)
	tree, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatalf("parse without result compatibility: %v", err)
	}
	defer tree.Release()

	rangeNode := findRustNodeByType(tree.RootNode(), lang, "range_expression")
	if rangeNode == nil {
		t.Fatal("range_expression = nil")
	}
	if got, want := rangeNode.ChildCount(), 1; got != want {
		t.Fatalf("range_expression child count = %d, want %d; tree: %s", got, want, tree.RootNode().SExpr(lang))
	}
	child := rangeNode.Child(0)
	if child == nil || child.Type(lang) != ".." || child.IsNamed() {
		t.Fatalf("range_expression child = %v, want anonymous ..; tree: %s", child, tree.RootNode().SExpr(lang))
	}
	productionParser := gotreesitter.NewParser(lang)
	productionParser.SetAdmissionCandidateRoute(false)
	productionTree, err := productionParser.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()
	if productionTree.RootNode() == nil {
		t.Fatal("production root = nil")
	}
	productionRange := findRustNodeByType(productionTree.RootNode(), lang, "range_expression")
	if productionRange == nil || productionRange.ChildCount() != 1 || productionRange.Child(0).Type(lang) != ".." {
		t.Fatalf("production range = %v, want one .. child; tree: %s", productionRange, productionTree.RootNode().SExpr(lang))
	}
	if got, want := productionRange.ParseState(), rangeNode.ParseState(); got != want {
		t.Fatalf("production range parse state = %d, want %d", got, want)
	}
	if got, want := productionRange.PreGotoState(), rangeNode.PreGotoState(); got != want {
		t.Fatalf("production range pre-goto state = %d, want %d", got, want)
	}
}

func findRustNodeByType(node *gotreesitter.Node, lang *gotreesitter.Language, nodeType string) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.Type(lang) == nodeType {
		return node
	}
	for index := 0; index < node.ChildCount(); index++ {
		if found := findRustNodeByType(node.Child(index), lang, nodeType); found != nil {
			return found
		}
	}
	return nil
}
