package parserresult_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestNormalizeCVariadicParameterEllipsis(t *testing.T) {
	const src = "void *f(void *, size_t, size_t, int, ...);\n"
	tree, lang := parseByLanguageName(t, "c", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected C parse error: %s", root.SExpr(lang))
	}

	variadic := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "variadic_parameter"
	})
	if variadic == nil {
		t.Fatalf("missing variadic_parameter: %s", root.SExpr(lang))
	}
	if got, want := variadic.ChildCount(), 1; got != want {
		t.Fatalf("variadic_parameter child count = %d, want %d: %s", got, want, variadic.SExpr(lang))
	}
	if got, want := variadic.Child(0).Type(lang), "..."; got != want {
		t.Fatalf("variadic_parameter child type = %q, want %q", got, want)
	}
}

func TestNormalizeCUnresolvedTypedefLikeCallShape(t *testing.T) {
	const src = "int f(void) { return (mstime_t)(server.unixtime - server.master->lastinteraction) * 1000; }\n"
	tree, lang := parseByLanguageName(t, "c", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected C parse error: %s", root.SExpr(lang))
	}

	wantText := "(mstime_t)(server.unixtime - server.master->lastinteraction)"
	call := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == wantText
	})
	if call == nil {
		t.Fatalf("missing unresolved typedef-like call_expression: %s", root.SExpr(lang))
	}
	if bad := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "cast_expression" && n.Text([]byte(src)) == wantText
	}); bad != nil {
		t.Fatalf("unexpected cast_expression for unresolved typedef-like call: %s", bad.SExpr(lang))
	}
}
