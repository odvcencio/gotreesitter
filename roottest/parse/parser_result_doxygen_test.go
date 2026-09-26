package parse_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestDoxygenWholeBlockCommentErrorMatchesCShape(t *testing.T) {
	lang := grammars.DoxygenLanguage()
	if lang == nil {
		t.Fatal("DoxygenLanguage returned nil")
	}
	source := []byte("/** Adds all words in \\a s to document \\a doc with weight \\a wfd */")

	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "ERROR" {
		t.Fatalf("root type = %q, want ERROR; tree=%s", got, root.SExpr(lang))
	}
	if got := root.ChildCount(); got != 0 {
		t.Fatalf("root child count = %d, want 0; tree=%s", got, root.SExpr(lang))
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root EndByte = %d, want %d", got, want)
	}
}
