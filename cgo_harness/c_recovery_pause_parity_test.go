//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestParityCRecoveryPauseBaseline(t *testing.T) {
	// This reduced case preserves the recovery divergence from issue 1100.
	source := []byte("\t.probe\t\t= gcc_mdm9607_probe,\nstatic int __init gcc_mdm9607_init(void)\n")
	cLanguage, err := COracleLanguage("c")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil {
		t.Fatal("C returned no tree")
	}
	defer cTree.Close()
	want, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"default", "production", "compact"} {
		t.Run(route, func(t *testing.T) {
			language := grammars.CLanguage()
			parser := gts.NewParser(language)
			if route != "default" {
				parser.SetAdmissionCandidateRoute(route == "compact")
			}
			tree, err := parser.Parse(source)
			if tree != nil {
				defer tree.Release()
			}
			if err != nil || tree == nil {
				t.Fatalf("parse: %v", err)
			}
			if tree.ParseStopReason() != gts.ParseStopAccepted || tree.RootNode().EndByte() != uint32(len(source)) {
				t.Fatalf("incomplete recovery: %s", tree.ParseRuntime().Summary())
			}
			got, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}
			if got.SHA256 != want {
				t.Fatalf("tree digest Go=%s C=%s: %+v", got.SHA256, want, FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode()))
			}
		})
	}
}
