//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoUnterminatedStringCommaMatchesLockedC(t *testing.T) {
	lang := grammars.GoLanguage()
	cLang, err := COracleLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"00000000000", "hello", "é"} {
		for _, gap := range []string{"\n", "\n ", "\r\n"} {
			source := []byte("package main\n\nfunc main() {\"" + body + gap + ",")
			cTree := cParser.Parse(source, nil)
			if cTree == nil {
				t.Fatal("locked C returned no tree")
			}
			for _, candidate := range []bool{false, true} {
				t.Run(fmt.Sprintf("body=%q/gap=%q/candidate=%t", body, gap, candidate), func(t *testing.T) {
					parser := gotreesitter.NewParser(lang)
					parser.SetAdmissionCandidateRoute(candidate)
					tree, err := parser.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil {
						t.Fatalf("source=%q divergence=%+v", source, diff)
					}
				})
			}
			cTree.Close()
		}
	}
}
