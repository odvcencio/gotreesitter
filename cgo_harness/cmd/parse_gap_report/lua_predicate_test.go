//go:build cgo && treesitter_c_parity

package main

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	cgoharness "github.com/odvcencio/gotreesitter/cgo_harness"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestLuaHighlightPredicatesMatchProduction(t *testing.T) {
	t.Chdir("../..")
	source := []byte("struct Upper { 1: string lower }\n")
	language := grammars.ThriftLanguage()
	goTree, err := gotreesitter.NewParser(language).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer goTree.Release()
	cLanguage, err := cgoharness.COracleLanguage("thrift")
	if err != nil {
		t.Fatal(err)
	}
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cTree := parser.Parse(source, nil)
	if cTree == nil {
		t.Fatal("C returned no tree")
	}
	defer cTree.Close()
	r := &runner{goLang: language, cLang: cLanguage}
	for _, tc := range []struct {
		pattern string
		want    int
	}{{"^%u", 1}, {"^U...r$", 1}, {"^L...r$", 0}} {
		t.Run(tc.pattern, func(t *testing.T) {
			query := fmt.Sprintf(`((identifier) @name (#lua-match? @name %q))`, tc.pattern)
			goCaptures, err := collectGoHighlightCaptures(r, goTree, query)
			if err != nil {
				t.Fatal(err)
			}
			cCaptures, err := collectCHighlightCaptures(r, cTree, source, query)
			if err != nil {
				t.Fatal(err)
			}
			if len(goCaptures) != tc.want || len(cCaptures) != tc.want {
				t.Fatalf("captures Go=%v C=%v, want %d each", goCaptures, cCaptures, tc.want)
			}
			if onlyGo, onlyC := diffHighlightCaptures(goCaptures, cCaptures); len(onlyGo)+len(onlyC) != 0 {
				t.Fatal(formatHighlightCaptureMismatch(onlyGo, onlyC))
			}
		})
	}
}
