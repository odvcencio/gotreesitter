//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCIncrementalReductionChainLockedCParity(t *testing.T) {
	source, err := os.ReadFile("../testdata/incremental_gate/c_repeated_functions.c")
	if err != nil {
		t.Fatal(err)
	}
	cLanguage, err := ParityCLanguage("c")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	for _, replacement := range []string{"", "z"} {
		name := "replace"
		if replacement == "" {
			name = "delete"
		}
		t.Run(name, func(t *testing.T) {
			const start = 23995
			edited := append([]byte(nil), source[:start]...)
			edited = append(edited, replacement...)
			edited = append(edited, source[start+1:]...)
			edit := gotreesitter.InputEdit{
				StartByte: uint32(start), OldEndByte: uint32(start + 1), NewEndByte: uint32(start + len(replacement)),
				StartPoint: pointAtOffset(source, start), OldEndPoint: pointAtOffset(source, start+1),
				NewEndPoint: pointAtOffset(edited, start+len(replacement)),
			}
			old, lang, err := parseWithGo(parityCase{name: "c"}, source, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseGoTree(old)
			old.Edit(edit)
			incremental, _, err := parseWithGo(parityCase{name: "c"}, edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseGoTree(incremental)
			cOld := cParser.Parse(source, nil)
			if cOld == nil {
				t.Fatal("C baseline parse returned no tree")
			}
			defer cOld.Close()
			cEdit := realCorpusCInputEdit(edit)
			cOld.Edit(&cEdit)
			for _, previous := range []*sitter.Tree{nil, cOld} {
				cTree := cParser.Parse(edited, previous)
				if cTree == nil {
					t.Fatal("C parse returned no tree")
				}
				defer cTree.Close()
				if cTree.RootNode().HasError() || cTree.RootNode().EndByte() != uint(len(edited)) {
					t.Fatal("C fixture contains errors or omits source bytes")
				}
				if difference := FirstDivergenceDumpV1(incremental.RootNode(), lang, cTree.RootNode()); difference != nil {
					t.Fatalf("incremental tree differs from C: %+v", difference)
				}
			}
		})
	}
}
