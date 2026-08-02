//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestKotlinA3CertificationObjectDeclarationMaterialityGateCOracle is the
// C-oracle receipt for Kotlin's A3 certification-workstream materiality gate
// (spec.campaign.v7, finding tied-election-family-compact-retirement).
//
// Combining CompactConvergedReductionSplitDropsCertified with
// CompactPrimaryAcceptanceDerivationCertified used to regress this witness
// (issue #93, query_kotlin_object_declaration_test.go): with both forced on,
// the compact route used to accept "object Singleton { fun work() = Unit }"
// as an infix_expression instead of an object_declaration, diverging from
// the locked C oracle, which sides with production's object_declaration.
// selectCompactAcceptanceDerivation's materiality gate
// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous) closed
// that: the witness's two tied derivations do not materialize to the same
// public tree, so the shipped, certified compact route declines this
// material election and falls back to production instead. This test
// adjudicates the served tree directly against the locked C oracle: it must
// stay C-exact under the shipped certification, not merely self-consistent
// with a second production parse (the companion no-cgo receipt,
// admission_switch_kotlin_certification_withheld_test.go, checks that
// weaker property plus the classified decline mechanism). See
// cgo_harness/kotlin_a3_certification_sweep_test.go for the same witness
// (object_declaration_multiline) inside the full-corpus sweep.
func TestKotlinA3CertificationObjectDeclarationMaterialityGateCOracle(t *testing.T) {
	source := []byte("package demo\n\nobject Singleton {\n    fun work() = Unit\n}\n")
	goLang := grammars.KotlinLanguage()
	if !goLang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("kotlin did not receive the A3 converged-split-drop certification")
	}
	if !goLang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("kotlin did not receive the A3 primary-acceptance-derivation certification")
	}

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

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	candidate := gotreesitter.NewParser(goLang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("certified compact parse: %v", err)
	}
	defer candidateTree.Release()

	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
		t.Fatalf(
			"certified candidate route counters before=(%d,%d) after=(%d,%d), want fallback+1 only "+
				"(the materiality gate must decline this material election even though both A3 "+
				"certificates are shipped -- an accept here would be a soundness hole, not a fixed "+
				"witness); reason=%s",
			routedBefore, fallbackBefore, routedAfter, fallbackAfter, gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}

	var candidateMismatches []string
	compareNodes(candidateTree.RootNode(), goLang, cRoot, "root", &candidateMismatches)
	if len(candidateMismatches) != 0 {
		t.Fatalf(
			"certified compact route (declined, serving production's fallback tree) diverges from "+
				"the C oracle at %d point(s); production is proven C-exact above, so this can only "+
				"mean the fallback did not actually serve production's tree:\n%s",
			len(candidateMismatches), strings.Join(candidateMismatches, "\n"),
		)
	}
	t.Logf(
		"confirmed: the certified compact route declines this material election (materiality gate) " +
			"and falls back to production, which stays C-exact on the #93 witness",
	)
}
