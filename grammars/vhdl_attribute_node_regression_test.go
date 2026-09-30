package grammars_test

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars/vhdl"
)

func TestVHDLLengthAttributeUsesFunctionNode(t *testing.T) {
	const source = "entity h is port();end;architecture r of h is function r()return t is constant c:g'length;begin\rfor i in-1loop end loop;end;begin\r\nend;"
	lang := vhdl.Language()
	tree, err := ts.NewParser(lang).Parse([]byte(source))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer tree.Release()

	if node := findNodeOfType(tree.RootNode(), lang, "attribute_function"); node == nil {
		t.Fatalf("missing attribute_function; tree=%s", tree.RootNode().SExpr(lang))
	}
	if node := findNodeOfType(tree.RootNode(), lang, "attribute_pure_function"); node != nil {
		t.Fatalf("unexpected attribute_pure_function; tree=%s", tree.RootNode().SExpr(lang))
	}
}

func findNodeOfType(node *ts.Node, lang *ts.Language, typ string) *ts.Node {
	if node == nil {
		return nil
	}
	if node.Type(lang) == typ {
		return node
	}
	for i := 0; i < node.ChildCount(); i++ {
		if found := findNodeOfType(node.Child(i), lang, typ); found != nil {
			return found
		}
	}
	return nil
}
