package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestFSharpIncrementalInsertAtStartMatchesFresh(t *testing.T) {
	entry := grammars.DetectLanguageByName("fsharp")
	if entry == nil {
		t.Fatal("fsharp language not found")
	}
	lang := entry.Language()
	before := []byte("\ntype i=inherit y()\nlet:e(\"\")]f t t t")
	after := append([]byte("x"), before...)

	old, err := gotreesitter.NewParser(lang).Parse(before)
	if err != nil {
		t.Fatalf("parse before: %v", err)
	}
	old.Edit(spliceTestEdit(before, after))
	inc, err := gotreesitter.NewParser(lang).ParseIncremental(after, old)
	if err != nil {
		t.Fatalf("parse incrementally: %v", err)
	}
	fresh, err := gotreesitter.NewParser(lang).Parse(after)
	if err != nil {
		t.Fatalf("parse fresh: %v", err)
	}
	if diff := spliceTreeDiff(inc.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
		t.Fatalf("incremental tree differs from fresh tree: %s", diff)
	}
}
