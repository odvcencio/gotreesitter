//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// kotlinA3ObjectDeclarationCOracleRoot loads the locked Kotlin C oracle and
// parses source, returning its root node and a close func the caller must
// defer.
func kotlinA3ObjectDeclarationCOracleRoot(t *testing.T, source []byte) (*sitter.Node, func()) {
	t.Helper()
	cLang, err := COracleLanguage("kotlin")
	if err != nil {
		t.Fatalf("load Kotlin C oracle: %v", err)
	}
	cParser := sitter.NewParser()
	if err := cParser.SetLanguage(cLang); err != nil {
		cParser.Close()
		t.Fatalf("set Kotlin C oracle language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		cParser.Close()
		t.Fatal("C oracle parse returned no root")
	}
	return cTree.RootNode(), func() {
		cTree.Close()
		cParser.Close()
	}
}

// TestKotlinA3CertificationObjectDeclarationDeclinesUnderShippedProfileCOracle
// is the C-oracle receipt for the #93 witness ("object Singleton { fun
// work() = Unit }", query_kotlin_object_declaration_test.go) against the
// actual shipped Kotlin profile (CompactPrimaryAcceptanceDerivationCertified
// only). It adjudicates the served tree directly against the locked C
// oracle: it must stay C-exact, not merely self-consistent with a second
// production parse (the companion no-cgo receipt,
// admission_switch_kotlin_certification_test.go, checks that weaker
// property). See cgo_harness/kotlin_a3_certification_sweep_test.go for the
// same witness (object_declaration_multiline) inside the full-corpus sweep.
func TestKotlinA3CertificationObjectDeclarationDeclinesUnderShippedProfileCOracle(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	source := []byte("package demo\n\nobject Singleton {\n    fun work() = Unit\n}\n")
	goLang := grammars.KotlinLanguage()
	if !goLang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("kotlin did not receive the A3 primary-acceptance-derivation certification")
	}
	if goLang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("kotlin unexpectedly carries the withheld converged-split-drop certification")
	}

	cRoot, closeC := kotlinA3ObjectDeclarationCOracleRoot(t, source)
	defer closeC()

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

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	candidate := gotreesitter.NewParser(goLang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("shipped compact parse: %v", err)
	}
	defer candidateTree.Release()

	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
		t.Fatalf(
			"shipped candidate route counters before=(%d,%d) after=(%d,%d), want fallback+1 only "+
				"(an accept here would be a soundness hole, not a fixed witness); reason=%s",
			routedBefore, fallbackBefore, routedAfter, fallbackAfter, gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}

	var candidateMismatches []string
	compareNodes(candidateTree.RootNode(), goLang, cRoot, "root", &candidateMismatches)
	if len(candidateMismatches) != 0 {
		t.Fatalf(
			"shipped compact route (declined, serving production's fallback tree) diverges from "+
				"the C oracle at %d point(s); production is proven C-exact above, so this can only "+
				"mean the fallback did not actually serve production's tree:\n%s",
			len(candidateMismatches), strings.Join(candidateMismatches, "\n"),
		)
	}
	t.Logf(
		"confirmed: the shipped compact route declines this witness (reason=%s) and falls back to "+
			"production, which stays C-exact on the #93 witness",
		gotreesitter.AdmissionCandidateLastFallbackReason(),
	)
}

// TestKotlinA3CertificationObjectDeclarationCExactCOracleWhenSplitDropsForced
// forces CompactConvergedReductionSplitDropsCertified on locally (it is not
// shipped) and adjudicates the served tree against the locked C oracle.
// Before tree-sitter-kotlin 1852ea17 (#280), the compact route accepted
// "object Singleton { fun work() = Unit }" as an infix_expression under
// both certificates, and selectCompactAcceptanceDerivation's materiality
// gate (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous)
// had to decline that material election. The upstream grammar removed the
// infix derivation. The witness has one derivation now, the compact route
// accepts it, and the accepted tree must stay C-exact. The gate keeps its
// own receipt in admission_switch_acceptance_frontier_test.go.
func TestKotlinA3CertificationObjectDeclarationCExactCOracleWhenSplitDropsForced(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	source := []byte("package demo\n\nobject Singleton {\n    fun work() = Unit\n}\n")
	goLang := grammars.KotlinLanguage()
	if !goLang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("kotlin did not receive the A3 primary-acceptance-derivation certification")
	}
	if goLang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("kotlin unexpectedly carries the withheld converged-split-drop certification")
	}

	cRoot, closeC := kotlinA3ObjectDeclarationCOracleRoot(t, source)
	defer closeC()

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

	goLang.CompactConvergedReductionSplitDropsCertified = true
	defer func() { goLang.CompactConvergedReductionSplitDropsCertified = false }()

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	candidate := gotreesitter.NewParser(goLang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("forced-split-drops compact parse: %v", err)
	}
	defer candidateTree.Release()

	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
		t.Fatalf(
			"forced-split-drops candidate route counters before=(%d,%d) after=(%d,%d), want "+
				"routed+1 only (tree-sitter-kotlin #280 removed the infix derivation, so this witness "+
				"has no tied election left to decline); reason=%s",
			routedBefore, fallbackBefore, routedAfter, fallbackAfter, gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}

	var candidateMismatches []string
	compareNodes(candidateTree.RootNode(), goLang, cRoot, "root", &candidateMismatches)
	if len(candidateMismatches) != 0 {
		t.Fatalf(
			"forced-split-drops compact route accepted a tree that diverges from the C oracle at "+
				"%d point(s):\n%s",
			len(candidateMismatches), strings.Join(candidateMismatches, "\n"),
		)
	}
	t.Log("confirmed: with split-drops forced on, the compact route accepts the #93 witness and the accepted tree is C-exact")
}
