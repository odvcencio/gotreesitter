//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
	"testing"
)

func TestJavaScriptRecoverySiblingSelectionLockedC(t *testing.T) {
	requireJavaScriptRecoverySelectionLockedC(t, []byte("const f = (a) => a + 1#&\nclass A { m() { return 1 } }\n\n"))
}

func TestJavaScriptClassBodyRecoverySelectionLockedC(t *testing.T) {
	requireJavaScriptRecoverySelectionLockedC(t, []byte("const f = (a) => a + 1;\nclass  A { m() { return 1 }?)}\n"))
}

func TestJavaScriptClosingParenRecoveryDeclinesLossyClosure(t *testing.T) {
	source := []byte("const f = (a) => a + 1)&\nclass A { m() { return 1 } }\n\n")
	parser := gts.NewParser(grammars.JavascriptLanguage())
	parser.SetAdmissionCandidateRoute(true)
	routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
	tree, err := parser.Parse(source)
	if err != nil || tree == nil {
		t.Fatalf("Go parse: %v", err)
	}
	defer tree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()
	if routed-routedBefore != 0 || fallback-fallbackBefore != 1 {
		t.Fatalf("compact recovery route=%d/%d, want 0/1", routed-routedBefore, fallback-fallbackBefore)
	}
}

func requireJavaScriptRecoverySelectionLockedC(t *testing.T, source []byte) {
	t.Helper()
	language := grammars.JavascriptLanguage()
	parser := gts.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
	tree, err := parser.Parse(source)
	if err != nil || tree == nil {
		t.Fatalf("Go parse: %v", err)
	}
	defer tree.Release()
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
		t.Fatal("C returned no tree")
	}
	defer oracle.Close()
	routed, fallback := gts.AdmissionCandidateCounters()
	routed -= routedBefore
	fallback -= fallbackBefore
	if routed != 1 || fallback != 0 {
		t.Fatalf("compact recovery route=%d/%d reason=%q", routed, fallback, gts.AdmissionCandidateLastFallbackReason())
	}
	if diff := FirstDivergenceDumpV1(tree.RootNode(), language, oracle.RootNode()); diff != nil {
		t.Fatalf("recovery selection mismatch: %+v\nGo: %s\nC: %s", diff, tree.RootNode().SExpr(language), oracle.RootNode().ToSexp())
	}
	if diff := firstLockedCTreeFlagDivergence(tree.RootNode(), language, oracle.RootNode(), "/"); diff != nil {
		t.Fatalf("recovery flags: %v", diff)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := COracleDeepDigest(oracle)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != digest {
		t.Fatalf("recovery digest Go=%s C=%s", inspection.SHA256, digest)
	}
	t.Logf("exact recovery digest: %s", digest)
}
