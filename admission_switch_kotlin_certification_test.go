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
// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous) made
// the shipped primary-acceptance-derivation grant safe on the previous
// blob: the object_declaration witness ("object Singleton { fun work() =
// Unit }", issue #93) had two tied derivations, and forcing both
// certificates together accepted an infix_expression misparse until the
// gate declined that material election. tree-sitter-kotlin 1852ea17
// (#280, "Prefer class and object declarations over infix expressions")
// removed the infix derivation, so the witness has one derivation now.
// TestKotlinCompactCertificationObjectDeclarationDeclinesUnderShippedProfile
// pins that the shipped language still declines this witness today, at
// the converged-path-split checkpoint, since split-drops is withheld.
// TestKotlinCompactCertificationObjectDeclarationAcceptsProductionTreeWhenSplitDropsForced
// forces split-drops back on locally and pins that the compact route now
// accepts the witness with production's object_declaration tree. No
// Kotlin witness reaches the materiality gate any more; the gate keeps
// its own receipt in admission_switch_acceptance_frontier_test.go.
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

// TestKotlinCompactCertificationObjectDeclarationAcceptsProductionTreeWhenSplitDropsForced
// forces CompactConvergedReductionSplitDropsCertified on locally (it is not
// shipped) on the #93 witness. Before tree-sitter-kotlin 1852ea17 (#280),
// this witness had two tied derivations, object_declaration and
// infix_expression, and the materiality gate had to decline it. The
// upstream grammar removed the infix derivation, so no election is left:
// the compact route accepts, and its tree must equal production's
// object_declaration tree. The C-oracle receipt for the same witness is
// cgo_harness/kotlin_a3_certification_object_declaration_regression_test.go.
func TestKotlinCompactCertificationObjectDeclarationAcceptsProductionTreeWhenSplitDropsForced(t *testing.T) {
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
			"forced-split-drops candidate route counters = %d/%d, want 1/0 (tree-sitter-kotlin #280 "+
				"removed the infix derivation, so this witness has no tied election left to decline); "+
				"reason=%s",
			routed, fallback, gts.AdmissionCandidateLastFallbackReason(),
		)
	}
	candidateSExpr := candidateTree.RootNode().SExpr(lang)
	if candidateSExpr != productionSExpr {
		t.Fatalf("forced-split-drops compact tree = %s, want production's object_declaration tree %s", candidateSExpr, productionSExpr)
	}
	t.Logf(
		"confirmed: with split-drops forced on, the compact route accepts the #93 witness and "+
			"serves production's object_declaration tree: %s",
		candidateSExpr,
	)
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
