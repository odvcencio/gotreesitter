package parse_test

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestMarkdownInlineAttributeHTMLTagStaysWhole(t *testing.T) {
	lang := grammars.MarkdownInlineLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte(`<link rel="stylesheet" href="x">`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	defer tree.Release()

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("expected error-free parse, got %s", root.SExpr(lang))
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := root.Child(0).Type(lang), "html_tag"; got != want {
		t.Fatalf("root child type = %q, want %q; tree=%s", got, want, root.SExpr(lang))
	}
}

func TestMarkdownInlineSingleTildeInLinkDestinationStaysLiteral(t *testing.T) {
	lang := grammars.MarkdownInlineLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte(`[Context](https://example.com/~user/file.pdf)`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	defer tree.Release()

	root := tree.RootNode()
	sexpr := root.SExpr(lang)
	if root.HasError() {
		t.Fatalf("expected error-free parse, got %s", sexpr)
	}
	if !strings.Contains(sexpr, "inline_link") {
		t.Fatalf("expected inline_link, got %s", sexpr)
	}
	if strings.Contains(sexpr, "_strikethrough_open") {
		t.Fatalf("single tilde in link destination became strikethrough delimiter: %s", sexpr)
	}
}
