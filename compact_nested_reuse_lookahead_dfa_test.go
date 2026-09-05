//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"testing"
)

func TestCompactNestedReuseDFALongestMatchDependency(t *testing.T) {
	g := grammargen.NewGrammar("nested_longest_match")
	g.Define("program", grammargen.Repeat1(grammargen.Sym("statement")))
	g.Define("statement", grammargen.Seq(grammargen.Sym("group"), grammargen.Repeat(grammargen.Sym("letter")), grammargen.Str(";")))
	g.Define("group", grammargen.Seq(grammargen.Str("("), grammargen.Sym("word")))
	g.Define("word", grammargen.Pat("a|ab+c"))
	g.Define("letter", grammargen.Pat("[bcd]"))
	lang, err := grammargen.GenerateLanguage(g)
	if err != nil {
		t.Fatal(err)
	}
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	source, edited := []byte("(abbbd;"), []byte("(abbbcc;")
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	// The word ends at byte 2, but its failed longer match reads byte 5.
	// The reduction lookahead returns only the first letter at bytes 2..3.
	oldGroup := old.RootNode().Child(0).Child(0)
	if old.RootNode().HasError() || oldGroup.Type(lang) != "group" || oldGroup.EndByte() != 2 {
		old.Release()
		t.Fatal("old fixture did not retain the short word")
	}
	old.Edit(gts.InputEdit{StartByte: 5, OldEndByte: 6, NewEndByte: 7, StartPoint: gts.Point{Column: 5}, OldEndPoint: gts.Point{Column: 6}, NewEndPoint: gts.Point{Column: 7}})
	next, err := p.ParseIncremental(edited, old)
	old.Release()
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	fresh, err := gts.NewParser(lang).Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	if fresh.RootNode().HasError() || fresh.RootNode().Child(0).Child(0).EndByte() != 6 {
		t.Fatal("fresh fixture did not select the longer word")
	}
	a, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	b, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 != b.SHA256 {
		t.Fatalf("longest-match dependency lost: compact=%t incremental=%s fresh=%s", next.ParseRuntime().CompactIncrementalReuseRoute, next.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
	}
}
