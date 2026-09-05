//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestTypeScriptLexicalErrorLeafLockedC(t *testing.T) {
	language := grammars.TypescriptLanguage()
	cLanguage, err := ParityCLanguage("typescript")
	if err != nil {
		t.Fatal(err)
	}
	for _, unexpected := range []byte{1, 2, 8} {
		for _, compact := range []bool{false, true} {
			t.Run(fmt.Sprintf("byte_%d/compact_%t", unexpected, compact), func(t *testing.T) {
				source := []byte("const n = \x01;\n")
				source[10] = unexpected
				parser := gts.NewParser(language)
				parser.SetAdmissionCandidateRoute(compact)
				tree, err := parser.Parse(source)
				if err != nil || tree == nil {
					t.Fatalf("Go parse: %v", err)
				}
				defer tree.Release()
				oracleParser := sitter.NewParser()
				defer oracleParser.Close()
				if err := oracleParser.SetLanguage(cLanguage); err != nil {
					t.Fatal(err)
				}
				oracle := oracleParser.Parse(source, nil)
				if oracle == nil {
					t.Fatal("C parse returned no tree")
				}
				defer oracle.Close()
				if !tree.RootNode().HasError() || !oracle.RootNode().HasError() {
					t.Fatal("the invalid-character fixture must retain its error")
				}
				if diff := FirstDivergenceDumpV1(tree.RootNode(), language, oracle.RootNode()); diff != nil {
					t.Fatalf("tree differs from C: %+v", diff)
				}
				actual, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
				if err != nil {
					t.Fatal(err)
				}
				want, err := COracleDeepDigest(oracle)
				if err != nil {
					t.Fatal(err)
				}
				if actual.SHA256 != want {
					t.Fatalf("deep tree digest: Go=%s C=%s", actual.SHA256, want)
				}
				t.Logf("exact C tree: %s", want)
			})
		}
	}
}
