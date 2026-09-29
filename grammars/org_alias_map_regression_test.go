package grammars_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestOrgAliasedWrapperChildren(t *testing.T) {
	lang := grammars.OrgLanguage()
	for _, input := range []string{"#+begin_t\n#+end_s", ":s:\n:END:"} {
		parser := gotreesitter.NewParser(lang)
		tree, err := parser.Parse([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		var visit func(*gotreesitter.Node)
		visit = func(node *gotreesitter.Node) {
			if node.Type(lang) == "expr" {
				found = true
				if node.ChildCount() != 1 {
					t.Errorf("%q: expr children = %d, want 1: %s", input, node.ChildCount(), tree.RootNode().SExpr(lang))
				}
			}
			for i := 0; i < int(node.ChildCount()); i++ {
				visit(node.Child(i))
			}
		}
		visit(tree.RootNode())
		if !found {
			t.Errorf("%q: missing expr wrapper", input)
		}
		tree.Release()
	}
}
