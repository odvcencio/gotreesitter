package parserresult_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestRubyIfThenStartsAtConditionEnd(t *testing.T) {
	const src = "if formats = details[:formats]\n  unless valid\n    details = details.dup\n  end\nend\n"
	tree, lang := parseByLanguageName(t, "ruby", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected ruby parse error: %s", root.SExpr(lang))
	}

	ifNode := findFirstNodeByType(root, lang, "if")
	if ifNode == nil {
		t.Fatalf("missing ruby if node: %s", root.SExpr(lang))
	}
	thenNode := findDirectChildByType(ifNode, lang, "then")
	if thenNode == nil {
		t.Fatalf("missing ruby then node: %s", ifNode.SExpr(lang))
	}
	cond := findDirectFieldChild(ifNode, lang, "condition")
	if cond == nil {
		t.Fatalf("missing ruby if condition: %s", ifNode.SExpr(lang))
	}
	if got, want := thenNode.StartByte(), cond.EndByte(); got != want {
		t.Fatalf("ruby then.StartByte = %d, want %d in %s", got, want, ifNode.SExpr(lang))
	}
}

func TestRubyWhenThenStartsAtPatternEnd(t *testing.T) {
	const src = "case record\nwhen String, Symbol\n  model = false\n  object_name = record\nend\n"
	tree, lang := parseByLanguageName(t, "ruby", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected ruby parse error: %s", root.SExpr(lang))
	}

	whenNode := findFirstNodeByType(root, lang, "when")
	if whenNode == nil {
		t.Fatalf("missing ruby when node: %s", root.SExpr(lang))
	}
	thenNode := findDirectChildByType(whenNode, lang, "then")
	if thenNode == nil {
		t.Fatalf("missing ruby when body: %s", whenNode.SExpr(lang))
	}
	pattern := findLastDirectChildByType(whenNode, lang, "pattern")
	if pattern == nil {
		t.Fatalf("missing ruby when pattern: %s", whenNode.SExpr(lang))
	}
	if got, want := thenNode.StartByte(), pattern.EndByte(); got != want {
		t.Fatalf("ruby when then.StartByte = %d, want %d in %s", got, want, whenNode.SExpr(lang))
	}
}

func TestRubyUnlessThenStartsAtConditionEnd(t *testing.T) {
	const src = "unless valid\n  values = values.dup\nend\n"
	tree, lang := parseByLanguageName(t, "ruby", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected ruby parse error: %s", root.SExpr(lang))
	}

	unlessNode := findFirstNodeByType(root, lang, "unless")
	if unlessNode == nil {
		t.Fatalf("missing ruby unless node: %s", root.SExpr(lang))
	}
	thenNode := findDirectChildByType(unlessNode, lang, "then")
	if thenNode == nil {
		t.Fatalf("missing ruby unless body: %s", unlessNode.SExpr(lang))
	}
	cond := findDirectFieldChild(unlessNode, lang, "condition")
	if cond == nil {
		t.Fatalf("missing ruby unless condition: %s", unlessNode.SExpr(lang))
	}
	if got, want := thenNode.StartByte(), cond.EndByte(); got != want {
		t.Fatalf("ruby unless then.StartByte = %d, want %d in %s", got, want, unlessNode.SExpr(lang))
	}
}

func TestRubyElsifThenStartsAtConditionEnd(t *testing.T) {
	const src = "if valid\n  values = values.dup\nelsif backup\n  values = defaults\nend\n"
	tree, lang := parseByLanguageName(t, "ruby", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected ruby parse error: %s", root.SExpr(lang))
	}

	elsifNode := findFirstNodeByType(root, lang, "elsif")
	if elsifNode == nil {
		t.Fatalf("missing ruby elsif node: %s", root.SExpr(lang))
	}
	thenNode := findDirectChildByType(elsifNode, lang, "then")
	if thenNode == nil {
		t.Fatalf("missing ruby elsif body: %s", elsifNode.SExpr(lang))
	}
	cond := findDirectFieldChild(elsifNode, lang, "condition")
	if cond == nil {
		t.Fatalf("missing ruby elsif condition: %s", elsifNode.SExpr(lang))
	}
	if got, want := thenNode.StartByte(), cond.EndByte(); got != want {
		t.Fatalf("ruby elsif then.StartByte = %d, want %d in %s", got, want, elsifNode.SExpr(lang))
	}
}

func findFirstNodeByType(root *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if root.Type(lang) == typ {
		return root
	}
	for i := 0; i < int(root.ChildCount()); i++ {
		if child := findFirstNodeByType(root.Child(i), lang, typ); child != nil {
			return child
		}
	}
	return nil
}

func findDirectChildByType(parent *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if parent == nil {
		return nil
	}
	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(i)
		if child != nil && child.Type(lang) == typ {
			return child
		}
	}
	return nil
}

func findLastDirectChildByType(parent *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if parent == nil {
		return nil
	}
	for i := int(parent.ChildCount()) - 1; i >= 0; i-- {
		child := parent.Child(i)
		if child != nil && child.Type(lang) == typ {
			return child
		}
	}
	return nil
}

func findDirectFieldChild(parent *gotreesitter.Node, lang *gotreesitter.Language, field string) *gotreesitter.Node {
	if parent == nil {
		return nil
	}
	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(i)
		if child != nil && parent.FieldNameForChild(i, lang) == field {
			return child
		}
	}
	return nil
}
