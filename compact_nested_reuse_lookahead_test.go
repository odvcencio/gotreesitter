//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"testing"
)

// A changed token after unchanged padding can invalidate an earlier reduction.
func TestCompactNestedReuseChangedReductionLookahead(t *testing.T) {
	g := grammargen.NewGrammar("nested_lookahead")
	g.Define("program", grammargen.Repeat1(grammargen.Sym("statement")))
	g.Define("statement", grammargen.Seq(grammargen.Sym("_expression"), grammargen.Str(";")))
	g.Define("_expression", grammargen.Choice(grammargen.Sym("binary_expression"), grammargen.Sym("identifier")))
	g.Define("binary_expression", grammargen.Choice(
		grammargen.PrecLeft(1, grammargen.Seq(grammargen.Sym("_expression"), grammargen.Str("+"), grammargen.Sym("_expression"))),
		grammargen.PrecLeft(2, grammargen.Seq(grammargen.Sym("_expression"), grammargen.Str("*"), grammargen.Sym("_expression"))),
	))
	g.Define("identifier", grammargen.Pat("[a-z]+"))
	g.SetExtras(grammargen.Pat("[ ]+"))
	lang, err := grammargen.GenerateLanguage(g)
	if err != nil {
		t.Fatal(err)
	}
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	source := []byte("a+b ;")
	beforeRouted, beforeFallback := gts.AdmissionCandidateCounters()
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	afterRouted, afterFallback := gts.AdmissionCandidateCounters()
	if afterRouted-beforeRouted != 1 || afterFallback != beforeFallback {
		t.Fatal("old source did not use the compact route")
	}
	if old.RootNode().HasError() {
		t.Fatal("old source did not produce a clean expression")
	}
	edited := []byte("a+b *c;")
	old.Edit(gts.InputEdit{StartByte: 4, OldEndByte: 5, NewEndByte: 7, StartPoint: gts.Point{Column: 4}, OldEndPoint: gts.Point{Column: 5}, NewEndPoint: gts.Point{Column: 7}})
	next, err := p.ParseIncremental(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	const want = "(program (statement (binary_expression (identifier) (binary_expression (identifier) (identifier)))))"
	if fresh.RootNode().HasError() || fresh.RootNode().SExpr(lang) != want {
		t.Fatalf("fresh parse did not preserve multiplication precedence: %s", fresh.RootNode().SExpr(lang))
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
		t.Fatalf("incremental=%s fresh=%s; incremental tree=%s", a.SHA256, b.SHA256, next.RootNode().SExpr(lang))
	}
}
