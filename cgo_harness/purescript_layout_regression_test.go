//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestPurescriptLayoutTransitionsLockedC(t *testing.T) {
	cLanguage, err := COracleLanguage("purescript")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"derive instance t::F i\ninstance p::A where p=(x)\ni::g",
		"derive instance u::F t\ninstance a::L where p(L)=(x)\ns::r",
		"derive instance t::F i\ninstance p::A where\n  p=(x)\ni::g",
		"derive instance t::F i\ninstance p::A where p=f x\ni::g",
	} {
		t.Run(source, func(t *testing.T) {
			cTree := cParser.Parse([]byte(source), nil)
			if cTree == nil {
				t.Fatal("locked C returned no tree")
			}
			defer cTree.Close()
			language := grammars.PurescriptLanguage()
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
					parser := gotreesitter.NewParser(language)
					parser.SetAdmissionCandidateRoute(compact)
					tree, err := parser.Parse([]byte(source))
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					assertLockedCTreeExact(t, "PureScript layout", tree, language, cTree)
				})
			}
		})
	}
}
