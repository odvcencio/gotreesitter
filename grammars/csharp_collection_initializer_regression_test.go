package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestCSharpGenericObjectCreationInCollectionInitializer(t *testing.T) {
	source := []byte("class C { object x = new D() { { typeof(T?), new F<T>(G.I) }, }; }")
	language := CSharpLanguage()
	parser := gotreesitter.NewParser(language)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root == nil {
		t.Fatal("collection initializer returned no root")
	}
	if root.HasError() {
		t.Fatalf("collection initializer did not parse cleanly: %s", root.SExpr(language))
	}
	creation := findFirstNamedDescendantWhere(root, language, "object_creation_expression", func(node *gotreesitter.Node) bool {
		return node.Text(source) == "new F<T>(G.I)"
	})
	if creation == nil {
		t.Fatalf("generic object creation is absent: %s", root.SExpr(language))
	}
	if binary := findFirstNamedDescendantWhere(root, language, "binary_expression", func(node *gotreesitter.Node) bool {
		return node.Text(source) == "new F<T>(G.I)"
	}); binary != nil {
		t.Fatalf("generic object creation became a comparison: %s", binary.SExpr(language))
	}
}
