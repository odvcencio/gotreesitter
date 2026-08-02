//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// This file is the A3 certification-workstream (spec.campaign.v7, finding
// tied-election-family-compact-retirement) withholding receipt for Kotlin.
//
// The finding's platform-modifier-recovery witness ("internal actual fun
// f(): String = \"x\"") matches the C oracle once the compact route accepts
// after a converged-path split drop
// (cgo_harness/b4b_alternative_set_v2_kotlin_adjudication_test.go). Landing
// CompactConvergedReductionSplitDropsCertified alone is safe:
// TestKotlinCompactCertificationPlatformModifierSplitOnlyIsSafe pins that.
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
// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous) closes
// this exact shape: the two derivations at this witness's accepted head
// materialize to different trees, so the election is material and the
// compact route now declines and falls back to production instead of
// publishing the wrong one. TestKotlinCompactCertificationObjectDeclarationRegressionWithheld
// below pins the corrected (declining) behavior. Kotlin's certification
// still does not land in grammars/runtime_profiles.go: an unrelated
// witness (annotated_declaration, a derivation-coverage gap rather than an
// election error) remains open.
//
// This file pins both halves of the finding (the safe grant and the
// now-safely-declining combination) so a future attempt to land Kotlin's
// certification is checked against this exact counterexample.

// TestKotlinCompactCertificationPlatformModifierSplitOnlyIsSafe confirms
// CompactConvergedReductionSplitDropsCertified alone (without
// CompactPrimaryAcceptanceDerivationCertified) resolves the finding's
// platform-modifier witness without introducing the object_declaration
// regression.
func TestKotlinCompactCertificationPlatformModifierSplitOnlyIsSafe(t *testing.T) {
	source := []byte("internal actual fun f(): String = \"x\"\n")
	lang := grammars.KotlinLanguage()
	if lang.CompactConvergedReductionSplitDropsCertified || lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("shared kotlin language unexpectedly carries a withheld A3 certificate")
	}

	lang.CompactConvergedReductionSplitDropsCertified = true
	defer func() { lang.CompactConvergedReductionSplitDropsCertified = false }()

	gts.ResetAdmissionCandidateCountersForTest()
	candidate := gts.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer candidateTree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf(
			"split-drops-only candidate route counters = %d/%d, want 1/0; reason=%s",
			routed, fallback, gts.AdmissionCandidateLastFallbackReason(),
		)
	}
	want := "(source_file (function_declaration (modifiers (visibility_modifier) (platform_modifier)) " +
		"(simple_identifier) (function_value_parameters) (user_type (type_identifier)) " +
		"(function_body (string_literal (string_content)))))"
	if got := candidateTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("split-drops-only candidate tree = %s, want %s", got, want)
	}
}

// TestKotlinCompactCertificationObjectDeclarationRegressionWithheld pins the
// corrected behavior on the counterexample that used to block Kotlin's A3
// certification: with both CompactConvergedReductionSplitDropsCertified and
// CompactPrimaryAcceptanceDerivationCertified forced on, the accepted head
// carries two derivations that materialize to different trees (a material
// election), so selectCompactAcceptanceDerivation's materiality gate now
// declines the compact route instead of publishing the wrong one, and the
// route falls back to production's (C-oracle-matching, see the cgo_harness
// companion receipt) object_declaration parse.
//
// The route counters alone only prove "some soft decline happened" -- the
// generic "did not accept EOF" fallback reason is shared by every soft
// decline, not specific to this gate. GTS_ADMISSION_CENSUS=1
// (ResetAdmissionCensusEnabledForTest clears the cached env read so setting
// it here reliably takes effect) surfaces the classified mechanism tag, so
// this asserts mechanism=material-acceptance-election specifically: proof
// this decline came from the materiality gate, not some other soft-decline
// path that also contains "did not accept EOF".
func TestKotlinCompactCertificationObjectDeclarationRegressionWithheld(t *testing.T) {
	source := []byte("package demo\n\nobject Singleton {\n    fun work() = Unit\n}\n")
	lang := grammars.KotlinLanguage()
	if lang.CompactConvergedReductionSplitDropsCertified || lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("shared kotlin language unexpectedly carries a withheld A3 certificate")
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

	lang.CompactConvergedReductionSplitDropsCertified = true
	lang.CompactPrimaryAcceptanceDerivationCertified = true
	defer func() {
		lang.CompactConvergedReductionSplitDropsCertified = false
		lang.CompactPrimaryAcceptanceDerivationCertified = false
	}()

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
			"forced-both-certificates candidate route counters = %d/%d, want 0/1 "+
				"(the materiality gate should decline this material election and fall back "+
				"to production); reason=%s",
			routed, fallback, gts.AdmissionCandidateLastFallbackReason(),
		)
	}
	reason := gts.AdmissionCandidateLastFallbackReason()
	if !strings.Contains(reason, "mechanism=material-acceptance-election") {
		t.Fatalf(
			"forced-both-certificates fallback reason does not classify as the materiality gate "+
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
