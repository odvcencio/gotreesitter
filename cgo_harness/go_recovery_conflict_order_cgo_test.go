//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoRecoveryConflictOrderParity(t *testing.T) {
	for _, tc := range []struct{ name, prefix string }{
		{"whole_source", ""},
		{"included_source", "// excluded\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.prefix + "ne.\npackage main\n")
			language := grammars.GoLanguage()
			parser := gts.NewParser(language)
			parser.SetAdmissionCandidateRoute(false)
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(loadCanonicalGoCLanguage(t)); err != nil {
				t.Fatal(err)
			}
			if tc.prefix != "" {
				start := len(tc.prefix)
				sr, sc := includedRangesPointAt(source, start)
				er, ec := includedRangesPointAt(source, len(source))
				parser.SetIncludedRanges([]gts.Range{{StartByte: uint32(start), EndByte: uint32(len(source)), StartPoint: gts.Point{Row: uint32(sr), Column: uint32(sc)}, EndPoint: gts.Point{Row: uint32(er), Column: uint32(ec)}}})
				if err := cParser.SetIncludedRanges([]sitter.Range{{StartByte: uint(start), EndByte: uint(len(source)), StartPoint: sitter.Point{Row: sr, Column: sc}, EndPoint: sitter.Point{Row: er, Column: ec}}}); err != nil {
					t.Fatal(err)
				}
			}
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("Go returned no tree")
			}
			defer tree.Release()
			cTree := cParser.Parse(source, nil)
			if cTree == nil {
				t.Fatal("C returned no tree")
			}
			defer cTree.Close()
			if !tree.ParseRuntime().CRecoveryEnteredErrorState {
				t.Fatal("fixture did not enter recovery")
			}
			if !tree.RootNode().HasError() {
				t.Fatal("malformed source returned a clean tree")
			}
			assertG18LockedCExact(t, tc.name, tree, language, cTree)
		})
	}
}
