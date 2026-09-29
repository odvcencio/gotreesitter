package grammars_test

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestHTMLIncrementalAttributeQuoteReplaceMatchesFresh(t *testing.T) {
	for _, tc := range []struct {
		name, before, after, want string
	}{
		{"missing_name", `<meta=">`, `<meta=x>`, "(document (element (start_tag (tag_name) (ERROR (ERROR)))))"},
		{"opening_quote", `<meta t="">`, `<meta t=x">`, "(document (element (start_tag (tag_name) (attribute (attribute_name) (attribute_value)) (ERROR))))"},
	} {
		for _, candidate := range []bool{false, true} {
			for _, profiled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/candidate=%t/profiled=%t", tc.name, candidate, profiled), func(t *testing.T) {
					testHTMLAttributeQuoteReplacement(t, tc.before, tc.after, tc.want, candidate, profiled)
				})
			}
		}
	}
}

func testHTMLAttributeQuoteReplacement(t *testing.T, beforeText, afterText, want string, candidate, profiled bool) {
	t.Helper()
	entry := grammars.DetectLanguageByName("html")
	if entry == nil || entry.Language() == nil {
		t.Fatal("html language unavailable")
	}
	lang := entry.Language()
	before, after := []byte(beforeText), []byte(afterText)
	// The candidate route exposes the fresh/incremental recovery mismatch;
	// pin it so this regression does not depend on the process default.
	beforeParser := gotreesitter.NewParser(lang)
	beforeParser.SetAdmissionCandidateRoute(candidate)
	old, err := beforeParser.Parse(before)
	if err != nil {
		t.Fatalf("Parse before: %v", err)
	}
	defer old.Release()
	old.Edit(spliceTestEdit(before, after))
	incParser := gotreesitter.NewParser(lang)
	incParser.SetAdmissionCandidateRoute(candidate)
	var inc *gotreesitter.Tree
	if profiled {
		inc, _, err = incParser.ParseIncrementalProfiled(after, old)
	} else {
		inc, err = incParser.ParseIncremental(after, old)
	}
	if err != nil {
		t.Fatalf("ParseIncremental: %v", err)
	}
	defer inc.Release()
	freshParser := gotreesitter.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(candidate)
	fresh, err := freshParser.Parse(after)
	if err != nil {
		t.Fatalf("Parse after: %v", err)
	}
	defer fresh.Release()
	if !fresh.RootNode().HasError() {
		t.Fatalf("fresh root has no error: %s", fresh.RootNode().SExpr(lang))
	}
	if got := fresh.RootNode().SExpr(lang); candidate && got != want {
		t.Fatalf("fresh tree = %s, want %s", got, want)
	}
	if diff := spliceTreeDiff(inc.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
		t.Fatalf("incremental differs from fresh at %s\nincremental: %s\nfresh: %s", diff, inc.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
	}
}
