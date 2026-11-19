package grammargen

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNamedStringKeywordUsesKeywordDFAOnly(t *testing.T) {
	g := NewGrammar("named_string_keyword")
	g.Define("source_file", Choice(Sym("literal"), Sym("punct"), Sym("identifier")))
	g.Define("literal", Str("false"))
	g.Define("punct", Str("@"))
	g.Define("identifier", Pat(`[a-zA-Z_][a-zA-Z0-9_]*`))
	g.SetWord("identifier")
	g.SetExtras(Pat(`\s`))
	ng, err := Normalize(g)
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, wantMain, wantKeyword bool) {
		t.Helper()
		id := -1
		for i, s := range ng.Symbols {
			if s.Name == name {
				id = i
				break
			}
		}
		if id < 0 {
			t.Fatalf("missing %s", name)
		}
		main, keyword := false, false
		for _, x := range ng.Terminals {
			main = main || x.SymbolID == id
		}
		for _, x := range ng.KeywordEntries {
			keyword = keyword || x.SymbolID == id
		}
		if main != wantMain || keyword != wantKeyword {
			t.Errorf("%s main=%v keyword=%v, want %v/%v", name, main, keyword, wantMain, wantKeyword)
		}
	}
	check("literal", false, true)
	check("punct", true, false)
	check("identifier", true, false)
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ src, want string }{{" false ", "(literal)"}, {" @ ", "(punct)"}, {" zebra ", "(identifier)"}} {
		tree, err := gotreesitter.NewParser(lang).Parse([]byte(tc.src))
		if err != nil {
			t.Fatal(err)
		}
		got := tree.RootNode().SExpr(lang)
		if tree.RootNode().HasError() || !strings.Contains(got, tc.want) {
			t.Errorf("%q: got %s, want %s", tc.src, got, tc.want)
		}
		tree.Release()
	}
}
