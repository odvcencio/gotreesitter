//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestP25BIssue454C137KiBParityBaseline(t *testing.T) {
	source := benchfixtures.Issue454CSource()
	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		t.Fatal("C edit marker is absent")
	}
	edited := append(append([]byte(nil), source[:site]...), source[site+1:]...)

	goBase, goLang, err := parseWithGo(parityCase{name: "c", source: string(source)}, source, nil)
	if err != nil {
		t.Fatalf("Go base parse: %v", err)
	}
	defer releaseGoTree(goBase)
	goFresh, _, err := parseWithGo(parityCase{name: "c", source: string(edited)}, edited, nil)
	if err != nil {
		t.Fatalf("Go fresh edited parse: %v", err)
	}
	defer releaseGoTree(goFresh)
	goIncremental, _, err := parseWithGo(parityCase{name: "c", source: string(edited)}, edited, goBase)
	if err != nil {
		t.Fatalf("Go incremental edited parse: %v", err)
	}
	defer releaseGoTree(goIncremental)

	cLang, err := ParityCLanguage("c")
	if err != nil {
		t.Fatalf("load C oracle: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set C oracle language: %v", err)
	}
	cBase := cParser.Parse(source, nil)
	if cBase == nil || cBase.RootNode() == nil {
		t.Fatal("C base parse returned no tree")
	}
	defer cBase.Close()
	cFresh := cParser.Parse(edited, nil)
	if cFresh == nil || cFresh.RootNode() == nil {
		t.Fatal("C fresh edited parse returned no tree")
	}
	defer cFresh.Close()

	goBaseDigest := p25bGoDigest(t, goBase, goLang)
	goFreshDigest := p25bGoDigest(t, goFresh, goLang)
	goIncrementalDigest := p25bGoDigest(t, goIncremental, goLang)
	cBaseDigest, err := COracleDeepDigest(cBase)
	if err != nil {
		t.Fatalf("C base digest: %v", err)
	}
	cFreshDigest, err := COracleDeepDigest(cFresh)
	if err != nil {
		t.Fatalf("C edited digest: %v", err)
	}
	baseDiff := FirstDivergenceDumpV1(goBase.RootNode(), goLang, cBase.RootNode())
	freshDiff := FirstDivergenceDumpV1(goFresh.RootNode(), goLang, cFresh.RootNode())
	incrementalDiff := FirstDivergenceDumpV1(goIncremental.RootNode(), goLang, cFresh.RootNode())
	t.Logf("P25B_C137K bytes=%d site=%d go_base_digest=%s c_base_digest=%s go_fresh_digest=%s c_fresh_digest=%s go_incremental_digest=%s base_diff=%+v fresh_c_diff=%+v incremental_c_diff=%+v incremental_fresh_go_equal=%v go_base_error=%v go_fresh_error=%v go_incremental_error=%v c_base_error=%v c_fresh_error=%v", len(source), site, goBaseDigest, cBaseDigest, goFreshDigest, cFreshDigest, goIncrementalDigest, baseDiff, freshDiff, incrementalDiff, goIncrementalDigest == goFreshDigest, goBase.RootNode().HasError(), goFresh.RootNode().HasError(), goIncremental.RootNode().HasError(), cBase.RootNode().HasError(), cFresh.RootNode().HasError())
	if goIncrementalDigest != goFreshDigest {
		t.Fatalf("incremental Go digest %s differs from fresh Go digest %s", goIncrementalDigest, goFreshDigest)
	}
}

func p25bGoDigest(t *testing.T, tree *gotreesitter.Tree, lang *gotreesitter.Language) string {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		t.Fatalf("inspect Go tree: %v", err)
	}
	return inspection.SHA256
}
