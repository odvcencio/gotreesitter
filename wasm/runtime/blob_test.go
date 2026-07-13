package main

import (
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestRuntimeGoBlobSupportsUTF16Analysis(t *testing.T) {
	blob, err := os.ReadFile("../../grammars/grammar_blobs/go.bin")
	if err != nil {
		t.Fatal(err)
	}
	language, err := grammars.LoadLanguage("go", blob)
	if err != nil {
		t.Fatal(err)
	}
	parser := gotreesitter.NewParser(language)
	tree, err := parser.ParseUTF16(units("package main\n\nfunc main() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if tree.RootNode() == nil || tree.RootNode().HasError() {
		t.Fatalf("unexpected parse tree: %v", tree.RootNode())
	}

	highlighter, err := gotreesitter.NewHighlighter(language, `(identifier) @identifier`)
	if err != nil {
		t.Fatal(err)
	}
	if ranges := highlighter.HighlightTreeUTF16(tree); len(ranges) == 0 {
		t.Fatal("expected at least one identifier highlight")
	}
}
