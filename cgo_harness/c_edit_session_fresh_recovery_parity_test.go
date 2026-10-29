//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// A reused suffix can leave more than 256 list elements to reduce at EOF.
// Each reduction lowers the stack depth and must reach acceptance without
// being mistaken for a reduction cycle.
func TestCIncrementalLongEOFReductionMatchesLockedC(t *testing.T) {
	source := benchfixtures.Issue454CSource()
	site := bytes.Index(source, []byte("x0"))
	if site < 0 || bytes.Contains(source[:site], []byte{'\n'}) {
		t.Fatal("fixture's first-line edit marker is absent")
	}
	edited := append(append([]byte(nil), source[:site]...), source[site+1:]...)
	lang := grammars.CLanguage()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	old.Edit(gotreesitter.InputEdit{
		StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site),
		StartPoint:  gotreesitter.Point{Column: uint32(site)},
		OldEndPoint: gotreesitter.Point{Column: uint32(site + 1)},
		NewEndPoint: gotreesitter.Point{Column: uint32(site)},
	})
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer incremental.Release()
	if incremental.ParseStoppedEarly() {
		t.Fatalf("incremental parse stopped: %s", incremental.ParseRuntime().Summary())
	}
	if profile.ReuseUnsupportedReason != "" || profile.ReusedBytes*8 < uint64(len(edited)) || profile.NewNodesAllocated > 800_000 {
		t.Fatalf("bounded suffix reuse failed: %+v", profile)
	}
	fresh, err := parser.Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	if fresh.ParseStoppedEarly() {
		t.Fatalf("fresh parse stopped: %s", fresh.ParseRuntime().Summary())
	}
	cLang, err := ParityCLanguage("c")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(edited, nil)
	if cTree == nil {
		t.Fatal("locked C returned no tree")
	}
	defer cTree.Close()
	for name, tree := range map[string]*gotreesitter.Tree{"incremental": incremental, "fresh": fresh} {
		if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil {
			t.Fatalf("%s differs from locked C: %+v", name, diff)
		}
	}
}
