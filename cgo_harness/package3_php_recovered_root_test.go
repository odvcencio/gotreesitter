//go:build cgo && treesitter_c_parity && !gts_no_parsercorephase0

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestPackage3PHPRecoveredRootLockedC(t *testing.T) {
	const source = "<?php namespace ; ?>"
	const sourceSHA = "669f317dd185ce38c907f47d7ef88de5c81d13f2f1db6de7649b672646095ec4"
	// The issue first pinned 3f2465c2. The current repository lock uses
	// 3fda2fb9, so this test verifies the active C oracle and Go blob.
	const grammarCommit = "3fda2fb9577166c6399834917f9844f30370beea"
	const grammarArtifactSHA = "1daea60ac1ee31227b8e1ed3cbd76b841435fe693e95af65cc61dad447d27891"
	const wantDigest = "ccda45c83ece81066dc557d0742945c929b17508e67b8884a64583e4958d09fd"
	const wantSExpr = "(program (php_tag) (ERROR) (empty_statement) (text_interpolation (php_end_tag)))"
	if len(source) != 20 || fmt.Sprintf("%x", sha256.Sum256([]byte(source))) != sourceSHA {
		t.Fatal("the PHP witness source changed")
	}

	cLanguage, err := ParityCLanguage("php")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("php")
	if err != nil {
		t.Fatal(err)
	}
	if identity.RuntimeCommit != COracleRuntimeCommit || identity.GrammarCommit != grammarCommit ||
		identity.GrammarArtifactSHA256 != grammarArtifactSHA {
		t.Fatalf("locked C identity=%+v", identity)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse([]byte(source), nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked C returned no root")
	}
	t.Cleanup(cTree.Close)
	cRoot := cTree.RootNode()
	if got := formatCNodeSExpr(cRoot); got != wantSExpr {
		t.Fatalf("locked C tree=%q, want %q", got, wantSExpr)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil || cDigest != wantDigest {
		t.Fatalf("locked C digest=%s error=%v, want %s", cDigest, err, wantDigest)
	}
	if cRoot.StartByte() != 0 || cRoot.EndByte() != 20 || !cRoot.HasError() || cRoot.IsError() ||
		cRoot.IsMissing() || cRoot.ChildCount() != 4 {
		t.Fatalf("locked C root range=%d..%d flags=%t/%t/%t children=%d",
			cRoot.StartByte(), cRoot.EndByte(), cRoot.HasError(), cRoot.IsError(), cRoot.IsMissing(), cRoot.ChildCount())
	}

	baseLanguage := grammars.PhpLanguage()
	if baseLanguage == nil {
		t.Fatal("the Go PHP grammar is unavailable")
	}
	productionParser := gotreesitter.NewParser(baseLanguage)
	productionParser.SetAdmissionCandidateRoute(false)
	productionTree, err := productionParser.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(productionTree.Release)
	if diff := FirstDivergenceDumpV1(productionTree.RootNode(), baseLanguage, cRoot); diff != nil {
		t.Fatalf("production PHP differs from locked C: %+v", *diff)
	}

	// Arm this package's capabilities on a private language copy. The
	// production profile stays unchanged until broader PHP certification.
	language := *baseLanguage
	language.CompactStrategy2ErrorRegionCertified = true
	language.CompactMissingTokenInsertionCertified = true
	language.CompactS5EOFMissingInsertionCertified = true
	language.CompactFaithfulS5RecoveryCertified = true
	language.CompactStackSummaryRecoveryCertified = true
	language.CompactPrimaryAcceptanceDerivationCertified = true
	language.CompactAcceptanceStructuralElectionCertified = true
	language.CompactRecoveryTrailingLineageRetirementCertified = true
	language.CompactRecoveryErrorModeKeywordCaptureCertified = true
	gotreesitter.ResetAdmissionCandidateCounters()
	parser := gotreesitter.NewParser(&language)
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	reason := gotreesitter.AdmissionCandidateLastFallbackReason()
	if routed != 0 || fallback != 1 ||
		!strings.Contains(reason, "no-lookahead reduction requires one runnable head") {
		t.Fatalf("PHP compact route=%d fallback=%d reason=%q", routed, fallback, reason)
	}
	root := tree.RootNode()
	if got := root.SExpr(&language); got != wantSExpr {
		t.Fatalf("compact PHP tree=%q, want %q", got, wantSExpr)
	}
	if root.StartByte() != 0 || root.EndByte() != 20 || !root.HasError() || root.IsError() ||
		root.IsMissing() || root.ChildCount() != 4 {
		t.Fatalf("compact root range=%d..%d flags=%t/%t/%t children=%d",
			root.StartByte(), root.EndByte(), root.HasError(), root.IsError(), root.IsMissing(), root.ChildCount())
	}
	for index, want := range []string{"php_tag", "ERROR", "empty_statement", "text_interpolation"} {
		if got := root.Child(index).Type(&language); got != want {
			t.Fatalf("root child %d=%q, want %q", index, got, want)
		}
	}
	if diff := FirstDivergenceDumpV1(root, &language, cRoot); diff != nil {
		t.Fatalf("compact PHP differs from locked C: %+v", *diff)
	}
	inspection, err := benchfixtures.InspectGoTree(root, &language)
	if err != nil || inspection.SHA256 != wantDigest {
		t.Fatalf("compact PHP digest=%s error=%v, want %s", inspection.SHA256, err, wantDigest)
	}
	t.Logf("PHP package-three source=%s C=%s grammar=%s artifact=%s digest=%s routed=%d fallback=%d reason=%q",
		sourceSHA, identity.RuntimeCommit, identity.GrammarCommit, identity.GrammarArtifactSHA256,
		inspection.SHA256, routed, fallback, reason)
}
