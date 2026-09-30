package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestTealPercentHasStringContentChild(t *testing.T) {
	// Both short and long content rules alias their repeated body as
	// string_content; the percent literal must remain a child of that node.
	for _, source := range []string{`d{["%"]=""}`, `d[[%]]`} {
		t.Run(source, func(t *testing.T) {
			lang := grammars.TealLanguage()
			tree, err := gotreesitter.NewParser(lang).Parse([]byte(source))
			if err != nil {
				t.Fatalf("parse teal: %v", err)
			}
			defer tree.Release()

			content := findTealNode(tree.RootNode(), lang, "string_content")
			if content == nil {
				t.Fatalf("teal tree has no string_content node: %s", tree.RootNode().SExpr(lang))
			}
			if got := content.ChildCount(); got != 1 {
				t.Fatalf("string_content has %d children, want 1 for percent; tree: %s", got, tree.RootNode().SExpr(lang))
			}
			child := content.Child(0)
			if child == nil || child.Text([]byte(source)) != "%" {
				t.Fatalf("string_content child = %v, want percent token", child)
			}
		})
	}
}

func findTealNode(node *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.Type(lang) == typ {
		return node
	}
	for i := 0; i < node.ChildCount(); i++ {
		if found := findTealNode(node.Child(i), lang, typ); found != nil {
			return found
		}
	}
	return nil
}
