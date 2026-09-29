package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestHTMLIncrementalAttributeQuoteReplaceMatchesFresh(t *testing.T) {
	entry := grammars.DetectLanguageByName("html")
	if entry == nil || entry.Language() == nil {
		t.Fatal("html language unavailable")
	}
	lang := entry.Language()
	before, after := []byte(`<meta=">`), []byte(`<meta=x>`)
	// The candidate route exposes the fresh/incremental recovery mismatch;
	// pin it so this regression does not depend on the process default.
	beforeParser := gotreesitter.NewParser(lang)
	beforeParser.SetAdmissionCandidateRoute(true)
	old, err := beforeParser.Parse(before)
	if err != nil {
		t.Fatalf("Parse before: %v", err)
	}
	defer old.Release()
	old.Edit(spliceTestEdit(before, after))
	incParser := gotreesitter.NewParser(lang)
	incParser.SetAdmissionCandidateRoute(true)
	inc, err := incParser.ParseIncremental(after, old)
	if err != nil {
		t.Fatalf("ParseIncremental: %v", err)
	}
	defer inc.Release()
	freshParser := gotreesitter.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(true)
	fresh, err := freshParser.Parse(after)
	if err != nil {
		t.Fatalf("Parse after: %v", err)
	}
	defer fresh.Release()
	if !fresh.RootNode().HasError() {
		t.Fatalf("fresh root has no error: %s", fresh.RootNode().SExpr(lang))
	}
	if got, want := fresh.RootNode().SExpr(lang), "(document (element (start_tag (tag_name) (ERROR (ERROR)))))"; got != want {
		t.Fatalf("fresh tree = %s, want %s", got, want)
	}
	if diff := spliceTreeDiff(inc.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
		t.Fatalf("incremental differs from fresh at %s\nincremental: %s\nfresh: %s", diff, inc.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
	}
}
