//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// This file is the A3 certification-workstream (spec.campaign.v7, finding
// tied-election-family-compact-retirement) materiality-gate receipt for
// Kotlin.
//
// The finding's platform-modifier-recovery witness ("internal actual fun
// f(): String = \"x\"") matches the C oracle once the compact route accepts
// after a converged-path split drop
// (TestAdmissionCandidateKotlinPlatformModifierSplitAcceptsAdjudicatedException,
// admission_switch_converged_path_test.go).
//
// Combining CompactConvergedReductionSplitDropsCertified with
// CompactPrimaryAcceptanceDerivationCertified used to regress a distinct,
// pre-existing witness: TestKotlinTopLevelObjectParsesAsDeclaration (issue
// #93, query_kotlin_object_declaration_test.go). With both flags on, the
// compact route used to accept "object Singleton { fun work() = Unit }" as
// infix_expression(object_literal, Singleton, lambda_literal) instead of
// object_declaration, even though the C oracle sides with production's
// object_declaration
// (cgo_harness/kotlin_a3_certification_object_declaration_regression_test.go).
//
// selectCompactAcceptanceDerivation's materiality gate
// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous) closed
// this exact shape: the two derivations at this witness's accepted head
// materialize to different trees, so the election is material and the
// compact route declines and falls back to production instead of publishing
// the wrong one. That gate is the prerequisite grammars/runtime_profiles.go
// cites for landing both certificates. Kotlin's A3 certification is now
// shipped there; TestKotlinCompactCertificationObjectDeclarationDeclinesUnderMaterialityGate
// below pins that the shipped, certified language still declines this exact
// witness -- certification did not bypass the gate. An unrelated witness
// (annotated_declaration, a derivation-coverage gap rather than an election
// error) remains open; see kotlinA3KnownDivergences and the sweep's
// per-witness log
// (cgo_harness/kotlin_a3_certification_sweep_test.go).

// TestKotlinCompactCertificationObjectDeclarationDeclinesUnderMaterialityGate
// pins the counterexample that used to block Kotlin's A3 certification, now
// against the shipped, certified language: with both
// CompactConvergedReductionSplitDropsCertified and
// CompactPrimaryAcceptanceDerivationCertified on, the accepted head still
// carries two derivations that materialize to different trees (a material
// election), so selectCompactAcceptanceDerivation's materiality gate
// declines the compact route instead of publishing the wrong one, and the
// route falls back to production's (C-oracle-matching, see the cgo_harness
// companion receipt) object_declaration parse. Certification did not bypass
// this gate -- if it ever does, that is a soundness hole, not a fixed
// witness.
//
// The route counters alone only prove "some soft decline happened" -- the
// generic "did not accept EOF" fallback reason is shared by every soft
// decline, not specific to this gate. GTS_ADMISSION_CENSUS=1
// (ResetAdmissionCensusEnabledForTest clears the cached env read so setting
// it here reliably takes effect) surfaces the classified mechanism tag, so
// this asserts mechanism=material-acceptance-election specifically: proof
// this decline came from the materiality gate, not some other soft-decline
// path that also contains "did not accept EOF".
func TestKotlinCompactCertificationObjectDeclarationDeclinesUnderMaterialityGate(t *testing.T) {
	source := []byte("package demo\n\nobject Singleton {\n    fun work() = Unit\n}\n")
	lang := grammars.KotlinLanguage()
	if !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("kotlin did not receive the A3 converged-split-drop certification")
	}
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("kotlin did not receive the A3 primary-acceptance-derivation certification")
	}

	production := gts.NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()
	productionSExpr := productionTree.RootNode().SExpr(lang)
	if !strings.Contains(productionSExpr, "object_declaration") {
		t.Fatalf("production tree lost object_declaration (issue #93 regressed independently of this file): %s", productionSExpr)
	}

	t.Setenv("GTS_ADMISSION_CENSUS", "1")
	gts.ResetAdmissionCensusEnabledForTest()
	t.Cleanup(gts.ResetAdmissionCensusEnabledForTest)

	gts.ResetAdmissionCandidateCountersForTest()
	candidate := gts.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer candidateTree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 0 || fallback != 1 {
		t.Fatalf(
			"certified candidate route counters = %d/%d, want 0/1 "+
				"(the materiality gate must decline this material election even though "+
				"both A3 certificates are shipped -- an accept here would be a soundness "+
				"hole, not a fixed witness); reason=%s",
			routed, fallback, gts.AdmissionCandidateLastFallbackReason(),
		)
	}
	reason := gts.AdmissionCandidateLastFallbackReason()
	if !strings.Contains(reason, "mechanism=material-acceptance-election") {
		t.Fatalf(
			"certified candidate route fallback reason does not classify as the materiality gate "+
				"(want mechanism=material-acceptance-election): %s", reason,
		)
	}
	// Sanity check only, not a correctness proof: a decline falls back to
	// production, so the served tree is production's own parse of the same
	// source, computed identically above. This can only fail if production
	// parsing itself is non-deterministic across two fresh parsers, a much
	// larger and separately-tested property (TestW5DeterminismAcrossFreshParsers).
	candidateSExpr := candidateTree.RootNode().SExpr(lang)
	t.Logf(
		"confirmed: the materiality gate declines this material election (two derivations, two "+
			"different trees, reason=%s) instead of publishing the positional primary; production's "+
			"object_declaration tree is served: %s",
		reason, candidateSExpr,
	)
	if candidateSExpr != productionSExpr {
		t.Fatalf("fallback tree = %s, want production's object_declaration tree %s (production parse non-determinism?)", candidateSExpr, productionSExpr)
	}
}
