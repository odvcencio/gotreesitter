//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// These receipt-session reductions reach recover_eof with children, or with
// leading trivia outside a childless root. Acceptance must preserve C's root.
func TestParityRecoverEOFRootPublication(t *testing.T) {
	cases := []struct {
		grammar string
		name    string
		source  string
		load    func() *gotreesitter.Language
	}{
		{"corn", "trivia", "\n", grammars.CornLanguage},
		{"corn", "child", "}", grammars.CornLanguage},
		{"dot", "trivia", "\n", grammars.DotLanguage},
		{"dot", "child", "}", grammars.DotLanguage},
	}
	for _, tc := range cases {
		t.Run(tc.grammar+"/"+tc.name, func(t *testing.T) {
			language := tc.load()
			cLanguage, err := COracleLanguage(tc.grammar)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			source := []byte(tc.source)
			cTree := cParser.Parse(source, nil)
			if cTree == nil {
				t.Fatal("C parse returned no tree")
			}
			defer cTree.Close()
			if cTree.RootNode().Kind() != "ERROR" || !cTree.RootNode().HasError() {
				t.Fatalf("C recovery root changed: %s", cTree.RootNode().ToSexp())
			}
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			for _, compact := range []bool{false, true} {
				parser := gotreesitter.NewParser(language)
				parser.SetAdmissionCandidateRoute(compact)
				tree, err := parser.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				defer tree.Release()
				inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
				if err != nil {
					t.Fatal(err)
				}
				if inspection.SHA256 != cDigest {
					t.Fatalf("compact=%v: Go/C difference: %+v", compact, FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode()))
				}
				if tree.RootNode().EndByte() < uint32(len(source)) && tree.ParseStopReason() == gotreesitter.ParseStopAccepted {
					t.Fatalf("compact=%v: short recovery root has no explanatory stop reason", compact)
				}
				old, err := parser.Parse(nil)
				if err != nil {
					t.Fatal(err)
				}
				endPoint := gotreesitter.Point{Column: uint32(len(source))}
				if tc.source == "\n" {
					endPoint = gotreesitter.Point{Row: 1}
				}
				old.Edit(gotreesitter.InputEdit{NewEndByte: uint32(len(source)), NewEndPoint: endPoint})
				incremental, err := parser.ParseIncremental(source, old)
				old.Release()
				if err != nil {
					t.Fatal(err)
				}
				defer incremental.Release()
				incInspection, err := benchfixtures.InspectGoTree(incremental.RootNode(), language)
				if err != nil {
					t.Fatal(err)
				}
				if incInspection.SHA256 != inspection.SHA256 {
					t.Fatalf("compact=%v: incremental differs from fresh", compact)
				}
				allocations := testing.AllocsPerRun(10, func() {
					next, err := parser.ParseIncremental(source, incremental)
					if err != nil {
						panic(err)
					}
					next.Release()
				})
				if allocations != 0 {
					t.Fatalf("compact=%v: no-edit reparse allocated %.0f times", compact, allocations)
				}
			}
		})
	}
}
