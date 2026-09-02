//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestStage6TypeScriptCompactCertification proves one grammar through the
// compact route with fresh and incremental trees against the locked C oracle.
func TestStage6TypeScriptCompactCertification(t *testing.T) {
	source := []byte(grammars.ParseSmokeSample("typescript"))
	if len(source) == 0 {
		t.Fatal("TypeScript smoke source is empty")
	}
	language := grammars.TypescriptLanguage()
	cLanguage, err := ParityCLanguage("typescript")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	defer cParser.Close()
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C parser returned a nil TypeScript tree")
	}
	defer cTree.Close()

	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
	fresh, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	afterRouted, afterFallback := gotreesitter.AdmissionCandidateCounters()
	if afterRouted-beforeRouted != 1 || afterFallback-beforeFallback != 0 {
		t.Fatalf("fresh compact route counters=%d/%d, want 1/0; reason=%q", afterRouted-beforeRouted, afterFallback-beforeFallback, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	assertLockedCTreeExact(t, "TypeScript compact fresh", fresh, language, cTree)

	offset := bytes.IndexByte(source, '1')
	if offset < 0 {
		t.Fatal("TypeScript smoke source has no numeric edit witness")
	}
	edited := append([]byte(nil), source...)
	edited[offset] = '2'
	edit := gotreesitter.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointAtOffset(source, offset),
		OldEndPoint: pointAtOffset(source, offset+1),
		NewEndPoint: pointAtOffset(edited, offset+1),
	}
	oldTree := fresh
	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer incremental.Release()
	if profile.ReuseUnsupported {
		t.Fatalf("TypeScript incremental certification fell back: %+v", profile)
	}
	cEdited := cParser.Parse(edited, nil)
	if cEdited == nil || cEdited.RootNode() == nil {
		t.Fatal("C parser returned a nil edited TypeScript tree")
	}
	defer cEdited.Close()
	assertLockedCTreeExact(t, "TypeScript compact incremental", incremental, language, cEdited)
}
