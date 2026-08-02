//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// This file is the A3 certification-workstream (spec.campaign.v7, finding
// tied-election-family-compact-retirement) certification-gate receipt for
// Kotlin. It is no longer a withholding receipt: kotlin's blob carries
// CompactPrimaryAcceptanceDerivationCertified in grammars/runtime_profiles.go.
// CompactConvergedReductionSplitDropsCertified stays withheld -- review
// found a compact-only divergence class on an annotated extension property
// with a getter followed by a trailing comment (production is C-exact,
// forcing split-drops alone accepts a C-divergent tree); see the
// runtime_profiles.go "kotlin" entry comment and
// cgo_harness/kotlin_a3_certification_sweep_test.go's
// annotated_extension_property_getter_* witnesses for the full ledger.
//
// selectCompactAcceptanceDerivation's materiality gate
// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous) is what
// makes the shipped primary-acceptance-derivation grant safe on its own: the
// object_declaration witness ("object Singleton { fun work() = Unit }",
// issue #93) used to regress to an infix_expression misparse when both
// certificates were forced on together, before the gate existed. The gate
// closes that: the witness's two tied derivations do not materialize to the
// same public tree, so the election is material and the compact route
// declines instead of publishing the wrong one.
// TestKotlinCompactCertificationObjectDeclarationDeclinesUnderShippedProfile
// pins that the shipped language declines this witness today (currently via
// the converged-path-split checkpoint, since split-drops is withheld, so
// the witness never reaches the tied-election point at all).
// TestKotlinCompactCertificationObjectDeclarationDeclinesUnderMaterialityGateWhenSplitDropsForced
// additionally forces split-drops back on locally to reach that tied-
// election point directly and pin the gate's own protection -- load-bearing
// insurance for any future split-drops re-grant attempt, not just today's
// shipped behavior.
//
// TestKotlinCompactCertificationPlatformModifierSplitOnlyIsSafe restores the
// isolation coverage the primary-accept-only decision needs: split-drops
// alone still resolves the finding's platform-modifier witness correctly
// (matches the C oracle) when forced in isolation. The regression that
// blocks its shipped certification is specific to the annotated-extension-
// property witnesses, not a blanket defect in the split-drops mechanism on
// every witness.

// TestKotlinCompactCertificationObjectDeclarationDeclinesUnderShippedProfile
// pins the object_declaration witness's decline under the actual shipped
// language -- no forcing. Certification did not bypass safety here: an
// accept would be a soundness hole, not a fixed witness.
func TestKotlinCompactCertificationObjectDeclarationDeclinesUnderShippedProfile(t *testing.T) {
	source := []byte("package demo\n\nobject Singleton {\n    fun work() = Unit\n}\n")
	lang := grammars.KotlinLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("kotlin did not receive the A3 primary-acceptance-derivation certification")
	}
	if lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("kotlin unexpectedly carries the withheld converged-split-drop certification")
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
			"shipped candidate route counters = %d/%d, want 0/1 (an accept here would be a soundness "+
				"hole, not a fixed witness); reason=%s",
			routed, fallback, gts.AdmissionCandidateLastFallbackReason(),
		)
	}
	candidateSExpr := candidateTree.RootNode().SExpr(lang)
	if candidateSExpr != productionSExpr {
		t.Fatalf("fallback tree = %s, want production's object_declaration tree %s (production parse non-determinism?)", candidateSExpr, productionSExpr)
	}
	t.Logf(
		"confirmed: the shipped language declines this witness (reason=%s) and serves production's "+
			"object_declaration tree: %s",
		gts.AdmissionCandidateLastFallbackReason(), candidateSExpr,
	)
}

// TestKotlinCompactCertificationObjectDeclarationDeclinesUnderMaterialityGateWhenSplitDropsForced
// forces CompactConvergedReductionSplitDropsCertified on locally (it is not
// shipped) to reach the tied-election point this witness needs the
// materiality gate to guard. The route counters alone only prove "some soft
// decline happened" -- the generic "did not accept EOF" fallback reason is
// shared by every soft decline, not specific to this gate.
// GTS_ADMISSION_CENSUS=1 (ResetAdmissionCensusEnabledForTest clears the
// cached env read so setting it here reliably takes effect) surfaces the
// classified mechanism tag, so this asserts
// mechanism=material-acceptance-election specifically: proof this decline
// came from the materiality gate, not the converged-path-split checkpoint
// that intercepts this same witness under the actual shipped profile.
func TestKotlinCompactCertificationObjectDeclarationDeclinesUnderMaterialityGateWhenSplitDropsForced(t *testing.T) {
	source := []byte("package demo\n\nobject Singleton {\n    fun work() = Unit\n}\n")
	lang := grammars.KotlinLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("kotlin did not receive the A3 primary-acceptance-derivation certification")
	}
	if lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("kotlin unexpectedly carries the withheld converged-split-drop certification")
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
	defer func() { lang.CompactConvergedReductionSplitDropsCertified = false }()

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
			"forced-split-drops candidate route counters = %d/%d, want 0/1 "+
				"(the materiality gate must decline this material election even with split-drops "+
				"forced back on -- an accept here would be a soundness hole, not a fixed witness); "+
				"reason=%s",
			routed, fallback, gts.AdmissionCandidateLastFallbackReason(),
		)
	}
	reason := gts.AdmissionCandidateLastFallbackReason()
	if !strings.Contains(reason, "mechanism=material-acceptance-election") {
		t.Fatalf(
			"forced-split-drops candidate route fallback reason does not classify as the materiality "+
				"gate (want mechanism=material-acceptance-election): %s", reason,
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
			"different trees, reason=%s) instead of publishing the positional primary, even with "+
			"split-drops forced back on; production's object_declaration tree is served: %s",
		reason, candidateSExpr,
	)
	if candidateSExpr != productionSExpr {
		t.Fatalf("fallback tree = %s, want production's object_declaration tree %s (production parse non-determinism?)", candidateSExpr, productionSExpr)
	}
}

// TestKotlinCompactCertificationPlatformModifierSplitOnlyIsSafe confirms
// CompactConvergedReductionSplitDropsCertified alone (forced; it is not
// shipped) still resolves the finding's platform-modifier witness correctly
// without introducing the annotated-extension-property regression: the two
// defects are on different witnesses, not the same mechanism failing
// everywhere.
func TestKotlinCompactCertificationPlatformModifierSplitOnlyIsSafe(t *testing.T) {
	source := []byte("internal actual fun f(): String = \"x\"\n")
	lang := grammars.KotlinLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("kotlin did not receive the A3 primary-acceptance-derivation certification")
	}
	if lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("kotlin unexpectedly carries the withheld converged-split-drop certification")
	}

	lang.CompactConvergedReductionSplitDropsCertified = true
	lang.CompactPrimaryAcceptanceDerivationCertified = false
	defer func() {
		lang.CompactConvergedReductionSplitDropsCertified = false
		lang.CompactPrimaryAcceptanceDerivationCertified = true
	}()

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
