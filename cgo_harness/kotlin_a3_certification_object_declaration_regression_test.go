//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestKotlinA3CertificationObjectDeclarationRegressionCOracle is the C-oracle
// receipt for withholding Kotlin's A3 certification (spec.campaign.v7,
// finding tied-election-family-compact-retirement).
//
// Forcing both CompactConvergedReductionSplitDropsCertified and
// CompactPrimaryAcceptanceDerivationCertified on the shared Kotlin language
// resolves the finding's platform-modifier-recovery witness
// (b4b_alternative_set_v2_kotlin_adjudication_test.go), but the same pair
// regresses this distinct witness (issue #93,
// query_kotlin_object_declaration_test.go): the compact route accepts
// "object Singleton { fun work() = Unit }" as an infix_expression instead
// of an object_declaration. This test adjudicates directly against the
// locked C oracle: production already matches C here, and the forced
// compact route does not. That is a genuine divergence, not an adjudicated
// exception, so grammars/runtime_profiles.go withholds both certificates
// for Kotlin. See admission_switch_kotlin_certification_withheld_test.go
// for the companion no-cgo receipt (including the safe split-drops-only
// grant).
func TestKotlinA3CertificationObjectDeclarationRegressionCOracle(t *testing.T) {
	source := []byte("package demo\n\nobject Singleton {\n    fun work() = Unit\n}\n")
	goLang := grammars.KotlinLanguage()

	cLang, err := COracleLanguage("kotlin")
	if err != nil {
		t.Fatalf("load Kotlin C oracle: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set Kotlin C oracle language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C oracle parse returned no root")
	}
	defer cTree.Close()
	cRoot := cTree.RootNode()

	production := gotreesitter.NewParser(goLang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()

	var productionMismatches []string
	compareNodes(productionTree.RootNode(), goLang, cRoot, "root", &productionMismatches)
	if len(productionMismatches) != 0 {
		t.Fatalf(
			"production diverges from the C oracle on the #93 witness (unexpected, unrelated "+
				"regression -- not the A3 certification finding this test pins):\n%s",
			strings.Join(productionMismatches, "\n"),
		)
	}

	if goLang.CompactConvergedReductionSplitDropsCertified || goLang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("shared kotlin language unexpectedly carries a withheld A3 certificate")
	}
	goLang.CompactConvergedReductionSplitDropsCertified = true
	goLang.CompactPrimaryAcceptanceDerivationCertified = true
	defer func() {
		goLang.CompactConvergedReductionSplitDropsCertified = false
		goLang.CompactPrimaryAcceptanceDerivationCertified = false
	}()

	candidate := gotreesitter.NewParser(goLang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("forced compact parse: %v", err)
	}
	defer candidateTree.Release()

	var candidateMismatches []string
	compareNodes(candidateTree.RootNode(), goLang, cRoot, "root", &candidateMismatches)
	if len(candidateMismatches) == 0 {
		t.Fatalf(
			"forced compact route (both A3 certificates) now matches the C oracle on the #93 " +
				"witness; the object_declaration regression may be fixed -- re-verify " +
				"admission_switch_kotlin_certification_withheld_test.go and consider landing " +
				"Kotlin's A3 certification",
		)
	}
	t.Logf(
		"confirmed: forced compact route (both A3 certificates) diverges from the C oracle at "+
			"%d point(s); production matches C. Kotlin's A3 certification stays withheld:\n%s",
		len(candidateMismatches), strings.Join(candidateMismatches, "\n"),
	)
}
