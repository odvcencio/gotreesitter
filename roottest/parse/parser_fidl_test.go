package parse_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestParseFIDLVersionedLayoutModifiersMatchCRecoveryShape(t *testing.T) {
	src := []byte("library test;\ntype Color = strict(removed=2) flexible(added=2) enum {\n    RED = 1;\n};\n")
	lang := fidlLanguageForTest(t)
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("fidl parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("fidl parse returned nil tree/root")
	}
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if !root.HasError() {
		t.Fatalf("root.HasError() = false, want C-compatible recovery error: %s", root.SExpr(lang))
	}
	layout := findFirstNodeByType(root, lang, "layout_declaration")
	if layout == nil {
		t.Fatalf("layout_declaration not found in %s", root.SExpr(lang))
	}
	if !layout.HasError() {
		t.Fatal("layout_declaration.HasError() = false, want true")
	}
	if got, want := layout.ChildCount(), 6; got != want {
		t.Fatalf("layout_declaration child count = %d, want %d", got, want)
	}
	if got := layout.Child(2); got == nil || got.Type(lang) != "ERROR" || !got.IsExtra() || got.ChildCount() != 9 {
		t.Fatalf("layout child 2 = %#v, want extra ERROR with 9 children", got)
	}
	if got := layout.Child(3); got == nil || got.Type(lang) != "=" {
		t.Fatalf("layout child 3 type = %v, want =", got)
	}
	if got := layout.Child(4); got == nil || got.Type(lang) != "ERROR" || !got.IsExtra() {
		t.Fatalf("layout child 4 = %#v, want extra ERROR", got)
	}
	inline := layout.Child(5)
	if inline == nil || inline.Type(lang) != "inline_layout" || inline.ChildCount() != 2 {
		t.Fatalf("layout child 5 = %#v, want inline_layout with 2 children", inline)
	}
	if got := inline.Child(0); got == nil || got.Type(lang) != "layout_kind" {
		t.Fatalf("inline first child = %#v, want layout_kind", got)
	}
}

func fidlLanguageForTest(t *testing.T) *gotreesitter.Language {
	t.Helper()
	for _, entry := range grammars.AllLanguages() {
		if entry.Name == "fidl" {
			return entry.Language()
		}
	}
	t.Fatal("fidl language entry not found")
	return nil
}

func findFirstNodeByType(root *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if root.Type(lang) == typ {
		return root
	}
	for i := 0; i < root.ChildCount(); i++ {
		if found := findFirstNodeByType(root.Child(i), lang, typ); found != nil {
			return found
		}
	}
	return nil
}
