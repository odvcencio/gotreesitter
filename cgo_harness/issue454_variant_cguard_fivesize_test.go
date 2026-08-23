//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestIssue454VariantCGuardFiveSizes(t *testing.T) {
	sizes := []int{1024, 4096, 16384, 65536, benchfixtures.Issue454CFixtureBytes}
	for _, size := range sizes {
		size := size
		t.Run(sizeLabel(size), func(t *testing.T) {
			source := append([]byte(nil), benchfixtures.Issue454CSource()[:size]...)
			site := bytes.Index(source, []byte("x0"))
			if site < 0 {
				t.Fatal("C edit marker is absent")
			}
			edited := append(append([]byte(nil), source[:site]...), source[site+1:]...)

			goTree, goLang, err := parseWithGo(parityCase{name: "c"}, edited, nil)
			if err != nil {
				t.Fatalf("parse edited C witness with Go: %v", err)
			}
			t.Cleanup(goTree.Release)

			cLang, err := ParityCLanguage("c")
			if err != nil {
				t.Fatalf("load locked C grammar: %v", err)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatalf("set locked C grammar: %v", err)
			}
			cTree := cParser.Parse(edited, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			t.Cleanup(cTree.Close)

			if diff := FirstDivergenceDumpV1(goTree.RootNode(), goLang, cTree.RootNode()); diff != nil {
				t.Fatalf("size=%d locked-C parity diverged: %+v", size, *diff)
			}
			t.Logf("size=%d locked-C parity passed", size)
		})
	}
}

func sizeLabel(size int) string {
	if size == benchfixtures.Issue454CFixtureBytes {
		return "137KiB"
	}
	return strconv.Itoa(size) + "B"
}
