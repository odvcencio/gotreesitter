//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestParityIssue454PHPWholeTreeFallback(t *testing.T) {
	source := benchfixtures.Issue454PHPSource()
	site := bytes.Index(source, []byte("$x0"))
	if site < 0 {
		t.Fatal("PHP edit marker is absent")
	}
	site++
	edited := make([]byte, 0, len(source)-1)
	edited = append(edited, source[:site]...)
	edited = append(edited, source[site+1:]...)

	goLang := grammars.PhpLanguage()
	goParser := gotreesitter.NewParser(goLang)
	goParser.SetAdmissionCandidateRoute(false)
	goTree, err := goParser.Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGoTree(goTree)
	if goTree.ParseStoppedEarly() {
		t.Fatalf("Go parse stopped early: %s", goTree.ParseRuntime().Summary())
	}

	cLang, err := ParityCLanguage("php")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(edited, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C reference parser returned nil tree")
	}
	defer cTree.Close()

	var errs []string
	compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &errs)
	if len(errs) > 0 {
		t.Fatalf("PHP issue #454 production tree differs from the pinned C oracle: %s", errs[0])
	}
}
