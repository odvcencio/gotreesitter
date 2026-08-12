package query_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Regression coverage for issue #93: the bundled Kotlin grammar misparses a
// top-level `object Foo { ... }` declaration as an infix_expression
// (object_literal + simple_identifier + lambda_literal) instead of an
// object_declaration, hiding the singleton from declaration walkers. The
// grammar DOES define object_declaration — the parser was resolving the
// object-at-declaration-position ambiguity toward the expression reading.
//
// The fix must NOT flip the *anonymous* object form (`object : Iface { }`),
// which is genuinely an expression and must stay object_literal.

func kotlinParseTree(t *testing.T, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	lang := grammars.KotlinLanguage()
	tree, err := gotreesitter.NewParser(lang).Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return tree, lang
}

func kotlinHasNodeType(n *gotreesitter.Node, lang *gotreesitter.Language, typ string) bool {
	if n == nil {
		return false
	}
	if n.Type(lang) == typ {
		return true
	}
	for i := 0; i < n.NamedChildCount(); i++ {
		if kotlinHasNodeType(n.NamedChild(i), lang, typ) {
			return true
		}
	}
	return false
}

func kotlinFirstNodeOfType(n *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if n == nil {
		return nil
	}
	if n.Type(lang) == typ {
		return n
	}
	for i := 0; i < n.ChildCount(); i++ {
		if found := kotlinFirstNodeOfType(n.Child(i), lang, typ); found != nil {
			return found
		}
	}
	return nil
}

func TestKotlinTopLevelObjectParsesAsDeclaration(t *testing.T) {
	src := "package demo\n\nobject Singleton {\n    fun work() = Unit\n}\n"
	tree, lang := kotlinParseTree(t, src)
	defer tree.Release()
	root := tree.RootNode()

	if kotlinHasNodeType(root, lang, "infix_expression") {
		t.Errorf("#93: named top-level object misparsed as infix_expression:\n%s", root.SExpr(lang))
	}
	if !kotlinHasNodeType(root, lang, "object_declaration") {
		t.Errorf("#93: named top-level object did not produce object_declaration:\n%s", root.SExpr(lang))
	}
}

func TestKotlinObjectWithSupertypeParsesAsDeclaration(t *testing.T) {
	src := "package demo\n\nobject Singleton : Runnable {\n    override fun run() = Unit\n}\n"
	tree, lang := kotlinParseTree(t, src)
	defer tree.Release()
	if !kotlinHasNodeType(tree.RootNode(), lang, "object_declaration") {
		t.Errorf("#93: object-with-supertype did not produce object_declaration:\n%s", tree.RootNode().SExpr(lang))
	}
}

func TestKotlinAnonymousObjectStaysExpression(t *testing.T) {
	src := "package demo\n\nval listener = object : Runnable {\n    override fun run() = Unit\n}\n"
	tree, lang := kotlinParseTree(t, src)
	defer tree.Release()
	root := tree.RootNode()

	// The anonymous form is an expression value, not a declaration. It must NOT
	// be promoted to object_declaration by the #93 fix.
	if kotlinHasNodeType(root, lang, "object_declaration") {
		t.Errorf("anonymous object must NOT become object_declaration:\n%s", root.SExpr(lang))
	}
	if !kotlinHasNodeType(root, lang, "object_literal") {
		t.Errorf("anonymous object should parse as an object_literal expression:\n%s", root.SExpr(lang))
	}
}

func TestKotlinInterpolatedCallExpressionWrapsCallSuffix(t *testing.T) {
	src := "package demo\n\nfun f() {\n  val time = if (true) \"${Instant.now()} \" else \"\"\n}\n"
	tree, lang := kotlinParseTree(t, src)
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("Kotlin interpolated call parse has errors:\n%s", root.SExpr(lang))
	}

	interpolated := kotlinFirstNodeOfType(root, lang, "interpolated_expression")
	if interpolated == nil {
		t.Fatalf("missing interpolated_expression:\n%s", root.SExpr(lang))
	}
	if got, want := interpolated.ChildCount(), 1; got != want {
		t.Fatalf("interpolated_expression child count = %d, want %d:\n%s", got, want, interpolated.SExpr(lang))
	}
	call := interpolated.Child(0)
	if call == nil || call.Type(lang) != "call_expression" {
		t.Fatalf("interpolated_expression child = %v, want call_expression:\n%s", call, interpolated.SExpr(lang))
	}
}
