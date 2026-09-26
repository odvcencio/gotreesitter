//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestIssue454DiffIncrementalLockedC(t *testing.T) {
	var source strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&source, "diff --git a/f%d.txt b/f%d.txt\n--- a/f%d.txt\n+++ b/f%d.txt\n@@ -1,2 +1,2 @@\n-old\n+new\n", i, i, i, i)
	}
	clean := []byte(source.String())
	at := strings.Index(source.String(), "--- a/f76.txt") + 2
	quoted := append(append([]byte{}, clean[:at]...), append([]byte{'"'}, clean[at:]...)...)
	lang := grammars.DiffLanguage()
	cLang, err := ParityCLanguage("diff")
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(compact)
			old, err := parser.Parse(quoted)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			assertIssue454DiffLockedC(t, old, lang, cLang, quoted)
			old.Edit(gts.InputEdit{
				StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at),
				StartPoint: pointAtOffset(quoted, at), OldEndPoint: pointAtOffset(quoted, at+1), NewEndPoint: pointAtOffset(clean, at),
			})
			next, err := parser.ParseIncremental(clean, old)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			assertIssue454DiffLockedC(t, next, lang, cLang, clean)
		})
	}
}

func assertIssue454DiffLockedC(t *testing.T, goTree *gts.Tree, lang *gts.Language, cLang *sitter.Language, source []byte) {
	t.Helper()
	cTree := compactT3ParseC(t, cLang, source)
	defer cTree.Close()
	if diff := FirstDivergenceDumpV1(goTree.RootNode(), lang, cTree.RootNode()); diff != nil {
		t.Fatalf("Go/C shape mismatch: %+v", diff)
	}
	if err := firstLockedCTreeFlagDivergence(goTree.RootNode(), lang, cTree.RootNode(), "/"); err != nil {
		t.Fatal(err)
	}
	inspection, err := benchfixtures.InspectGoTree(goTree.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != digest {
		t.Fatalf("Go/C digest: Go=%s C=%s", inspection.SHA256, digest)
	}
}
