//go:build linux && cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func q6GoOriginalSource(t testing.TB) []byte {
	t.Helper()
	for _, fixture := range cliffFixtures {
		if fixture.Name == "go" {
			return loadCliffSource(t, fixture)
		}
	}
	t.Fatal("missing pinned parser.go witness")
	return nil
}

func TestQ6GoFalseErrorLockedC(t *testing.T) {
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
	original := q6GoOriginalSource(t)
	for _, test := range []struct {
		name   string
		source []byte
	}{
		{"case_newline", []byte("package p\nfunc f(){switch x{case\nY:}}\n")},
		{"case_crlf", []byte("package p\nfunc f(){switch x{case\r\nY:}}\n")},
		{"parser.go", original},
	} {
		t.Run(test.name, func(t *testing.T) {
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C returned no tree")
			}
			defer cTree.Close()
			cDigest, err := canonicalCTreeInspection(cTree.RootNode())
			if err != nil {
				t.Fatal(err)
			}
			for _, candidate := range []bool{false, true} {
				parser := gts.NewParser(lang)
				parser.SetAdmissionCandidateRoute(candidate)
				tree, err := parser.Parse(test.source)
				if err != nil {
					t.Fatal(err)
				}
				defer tree.Release()
				goDigest, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil || goDigest.SHA256 != cDigest.SHA256 {
					t.Fatalf("candidate=%t Go=%s C=%s first difference=%+v", candidate, goDigest.SHA256, cDigest.SHA256, diff)
				}
				if tree.ParseRuntime().StopReason != gts.ParseStopAccepted || tree.RootNode().HasError() {
					t.Fatalf("candidate=%t stop=%s error=%t", candidate, tree.ParseRuntime().StopReason, tree.RootNode().HasError())
				}
				t.Logf("candidate=%t tokens=%d nodes=%d stacks=%d C=%s", candidate, tree.ParseRuntime().TokensConsumed, tree.ParseRuntime().NodesAllocated, tree.ParseRuntime().MaxStacksSeen, cDigest.SHA256)
			}
		})
	}
}
