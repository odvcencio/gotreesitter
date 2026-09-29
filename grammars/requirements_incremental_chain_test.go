package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestRequirementsIncrementalChainMatchesFresh(t *testing.T) {
	entry := grammars.DetectLanguageByName("requirements")
	if entry == nil || entry.Language() == nil {
		t.Skip("requirements grammar unavailable in this build")
	}
	lang := entry.Language()

	initial := []byte("# ")
	first := []byte("x# ")
	final := []byte("x#x ")

	tree, err := gotreesitter.NewParser(lang).Parse(initial)
	if err != nil {
		t.Fatalf("parse initial input: %v", err)
	}
	tree.Edit(spliceTestEdit(initial, first))
	tree, err = gotreesitter.NewParser(lang).ParseIncremental(first, tree)
	if err != nil {
		t.Fatalf("first incremental parse: %v", err)
	}
	tree.Edit(spliceTestEdit(first, final))
	incremental, err := gotreesitter.NewParser(lang).ParseIncremental(final, tree)
	if err != nil {
		t.Fatalf("second incremental parse: %v", err)
	}
	fresh, err := gotreesitter.NewParser(lang).Parse(final)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	if diff := spliceTreeDiff(incremental.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
		t.Fatalf("incremental tree differs from a fresh parse at %s\nincremental: %s\nfresh:       %s",
			diff, incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
	}
}
