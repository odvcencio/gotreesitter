package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestHTMLIncrementalChainMatchesFresh(t *testing.T) {
	previousDefault := gotreesitter.AdmissionCandidateRouteDefault()
	t.Cleanup(func() { gotreesitter.SetAdmissionCandidateRouteDefault(previousDefault) })
	gotreesitter.SetAdmissionCandidateRouteDefault(true)

	entry := grammars.DetectLanguageByName("html")
	if entry == nil || entry.Language() == nil {
		t.Fatal("html language unavailable")
	}
	lang := entry.Language()
	before := []byte("<html g=\"\"><m></d><m><l r></m>\n")
	parser := gotreesitter.NewParser(lang)
	old, err := parser.Parse(before)
	if err != nil {
		t.Fatalf("parse before: %v", err)
	}
	defer old.Release()

	first := append([]byte("x"), before...)
	old.Edit(spliceTestEdit(before, first))
	inc, err := parser.ParseIncremental(first, old)
	if err != nil {
		t.Fatalf("parse first edit incrementally: %v", err)
	}
	defer inc.Release()

	second := append(append(append([]byte(nil), first[:10]...), 'x'), first[10:]...)
	inc.Edit(spliceTestEdit(first, second))
	last, err := parser.ParseIncremental(second, inc)
	if err != nil {
		t.Fatalf("parse second edit incrementally: %v", err)
	}
	defer last.Release()

	fresh, err := gotreesitter.NewParser(lang).Parse(second)
	if err != nil {
		t.Fatalf("parse final text fresh: %v", err)
	}
	defer fresh.Release()
	if diff := spliceTreeDiff(last.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
		t.Fatalf("incremental tree differs from fresh tree after two edits at %s\nincremental: %s\nfresh:       %s", diff, last.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
	}
}
