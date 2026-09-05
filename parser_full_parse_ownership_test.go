package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestGoFullParseRepeatedParserPreservesLiveTree(t *testing.T) {
	gotreesitter.DrainArenaPools()
	defer gotreesitter.DrainArenaPools()
	lang := grammars.GoLanguage()
	p := gotreesitter.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	source := makeGoBenchmarkSource(500)
	parse := func() *gotreesitter.Tree {
		t.Helper()
		tree, err := p.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
			tree.Release()
			t.Fatal("full parse returned an incomplete tree")
		}
		return tree
	}
	live := parse()
	defer live.Release()
	want := live.RootNode().SExpr(lang)
	for i := 0; i < 4; i++ {
		tree := parse()
		if got := tree.RootNode().SExpr(lang); got != want {
			tree.Release()
			t.Fatalf("parse %d changed the tree", i)
		}
		tree.Release()
		if got := live.RootNode().SExpr(lang); got != want {
			t.Fatalf("parse %d changed the retained tree", i)
		}
	}
}
