//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestJavaScriptSharedRecoveryAliasWithoutArtifactGrantLockedC(t *testing.T) {
	language := *grammars.JavascriptLanguage()
	language.CompactRecoveryTerminalAliasRules = nil
	if language.CompactOwnedEOFRecoveryCertified {
		t.Fatal("fixture unexpectedly uses owned EOF recovery")
	}
	source := []byte(stage6JavaScriptRecoverySource)
	cLanguage, err := ParityCLanguage("javascript")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	oracle := cParser.Parse(source, nil)
	if oracle == nil {
		t.Fatal("C parser returned no tree")
	}
	defer oracle.Close()
	parser := gts.NewParser(&language)
	parser.SetAdmissionCandidateRoute(true)
	routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
	tree, err := parser.Parse(source)
	if err != nil || tree == nil {
		t.Fatalf("shared recovery parse: %v", err)
	}
	defer tree.Release()
	routedAfter, fallbackAfter := gts.AdmissionCandidateCounters()
	if routedAfter-routedBefore != 1 || fallbackAfter-fallbackBefore != 0 {
		t.Fatalf("shared recovery route=%d/%d reason=%q", routedAfter-routedBefore, fallbackAfter-fallbackBefore, gts.AdmissionCandidateLastFallbackReason())
	}
	if diff := FirstDivergenceDumpV1(tree.RootNode(), &language, oracle.RootNode()); diff != nil {
		t.Fatalf("shared recovery shape: %+v", diff)
	}
	if diff := firstLockedCTreeFlagDivergence(tree.RootNode(), &language, oracle.RootNode(), "/"); diff != nil {
		t.Fatalf("shared recovery flags: %v", diff)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), &language)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := COracleDeepDigest(oracle)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != digest {
		t.Fatalf("shared recovery digest Go=%s C=%s", inspection.SHA256, digest)
	}
	t.Logf("shared recovery without alias grant matches locked C: %s", digest)
}
