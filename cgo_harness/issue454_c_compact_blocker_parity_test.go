//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestIssue454COneKiBLockedCParity keeps the smallest issue #454 witness
// visible after the generic recovery fix restores locked-C parity.
func TestIssue454COneKiBLockedCParity(t *testing.T) {
	source := append([]byte(nil), benchfixtures.Issue454CSource()[:1024]...)
	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		t.Fatal("C edit marker is absent")
	}
	edited := append(append([]byte(nil), source[:site]...), source[site+1:]...)

	goTree, goLang, err := parseWithGo(parityCase{name: "c"}, edited, nil)
	if err != nil {
		t.Fatalf("parse edited C witness with Go: %v", err)
	}
	defer releaseGoTree(goTree)

	cLang, err := ParityCLanguage("c")
	if err != nil {
		t.Fatalf("load locked C grammar: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set locked C grammar: %v", err)
	}
	cTree := cParser.Parse(edited, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked C parser returned no tree")
	}
	defer cTree.Close()

	if got, want := goTree.RootNode().HasError(), true; got != want {
		t.Fatalf("Go root HasError() = %v, want %v", got, want)
	}
	if got, want := cTree.RootNode().HasError(), true; got != want {
		t.Fatalf("locked C root HasError() = %v, want %v", got, want)
	}

	diff := FirstDivergenceDumpV1(goTree.RootNode(), goLang, cTree.RootNode())
	if diff != nil {
		t.Fatalf("issue #454 locked-C parity diverged: %+v", *diff)
	}
	t.Log("issue #454 1 KiB witness matches locked C")
}
