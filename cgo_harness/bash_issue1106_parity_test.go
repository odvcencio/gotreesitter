//go:build cgo && treesitter_c_parity

package cgoharness_test

import (
	"encoding/json"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	harness "github.com/odvcencio/gotreesitter/cgo_harness"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestIssue1106BashEmptyAssignmentsCParity(t *testing.T) {
	data, err := os.ReadFile("../grammars/testdata/bash_empty_assignments.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name    string
		Source  string
		Invalid bool
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	cLang, err := harness.COracleLanguage("bash")
	if err != nil {
		t.Fatal(err)
	}
	lang := grammars.BashLanguage()
	for _, test := range cases {
		if test.Invalid {
			continue
		}
		t.Run(test.Name, func(t *testing.T) {
			source := []byte(test.Source)
			parser := sitter.NewParser()
			defer parser.Close()
			if err := parser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := parser.Parse(source, nil)
			if cTree == nil {
				t.Fatal("C parser returned no tree")
			}
			defer cTree.Close()
			if cTree.RootNode() == nil || cTree.RootNode().HasError() {
				t.Fatal("C parser did not return a complete tree")
			}
			cDigest, err := harness.COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			for _, route := range []string{"strict", "production", "raw"} {
				t.Run(route, func(t *testing.T) {
					goParser := gotreesitter.NewParser(lang)
					var tree *gotreesitter.Tree
					var err error
					switch route {
					case "strict":
						tree, err = goParser.ParseStrict(source)
					case "production":
						tree, err = goParser.Parse(source)
					case "raw":
						goParser.SetAdmissionCandidateRoute(false)
						tree, err = goParser.ParseNoResultCompatibilityBenchmarkOnly(source)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					root := tree.RootNode()
					if root == nil || root.HasErrorOrMissing() {
						t.Fatal("Go parser did not return a complete tree")
					}
					if diff := harness.FirstDivergenceDumpV1(root, lang, cTree.RootNode()); diff != nil {
						t.Fatalf("tree divergence: %+v", diff)
					}
					inspection, err := benchfixtures.InspectGoTree(root, lang)
					if err != nil {
						t.Fatal(err)
					}
					if inspection.SHA256 != cDigest {
						t.Fatalf("deep digest Go=%s C=%s", inspection.SHA256, cDigest)
					}
				})
			}
		})
	}
}
