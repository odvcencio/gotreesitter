//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestB4bAlternativeSetV2KotlinWitnessCOracleAdjudication independently
// verifies, against the locked C oracle, which side of the Kotlin witness
// (admission_switch_converged_path_test.go's
// TestAdmissionCandidateKotlinPlatformModifierSplitDeclinesWithSplitDropsWithheld)
// is the divergent one:
// production's error-recovery tree, or the clean function_declaration tree
// the v1/pre-branch-discrimination containment predicate would have
// produced had it (wrongly) admitted the drop.
//
// This is the campaign's C-oracle adjudication amendment (2026-08-02) to
// the stage 2a class-1 differential harness's stop rule: a v2-admit-where-
// scalar-declines election whose compact tree diverges from production is
// not automatically a v2 theorem falsifier -- it must first be adjudicated
// against the C oracle. Compact==C means production is the divergent side
// (an adjudicated-exception candidate, not a falsifier). Compact!=C is a
// true falsifier.
//
// The stage 2a census (parsercore_phase0_alternative_set_v2_differential_test.go)
// found v2's own branch-discriminated proof correctly DECLINES this drop
// (TestB4bV2OptInDeciderDifferentialKotlinWitness), so there is no class-1
// admission to adjudicate here on the live theorem. This test instead
// resolves the underlying three-way disagreement directly, forcing the
// compact route through via the certified-artifact-style bypass (ignoring
// the correct decline) to see which tree a naive admit would have produced,
// and checks it against C -- exactly the "compact-if-forced" differential
// the brief asks for, now graded against the correct oracle.
func TestB4bAlternativeSetV2KotlinWitnessCOracleAdjudication(t *testing.T) {
	source := []byte("internal actual fun f(): String = \"x\"\n")
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
	t.Logf("C oracle: kind=%s hasError=%t childCount=%d", cRoot.Kind(), cRoot.HasError(), cRoot.ChildCount())

	production := gotreesitter.NewParser(goLang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()
	productionRoot := productionTree.RootNode()
	t.Logf("production: hasError=%t sExpr=%s", productionRoot.HasError(), productionRoot.SExpr(goLang))

	var productionMismatches []string
	compareNodes(productionRoot, goLang, cRoot, "root", &productionMismatches)

	// Force the compact route through regardless of proof (mirrors the
	// established erlangConvergedSplitDecertify / certified-escape pattern:
	// admission_switch_erlang_converged_split_probe_test.go), to materialize
	// the "if forced" tree the theorem's decline is protecting against.
	forcedLang := *goLang
	forcedLang.CompactConvergedReductionSplitDropsCertified = true
	candidate := gotreesitter.NewParser(&forcedLang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("forced compact parse: %v", err)
	}
	defer candidateTree.Release()
	candidateRoot := candidateTree.RootNode()
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	t.Logf("forced compact route: routed=%d fallback=%d hasError=%t sExpr=%s", routed, fallback, candidateRoot.HasError(), candidateRoot.SExpr(goLang))

	var candidateMismatches []string
	compareNodes(candidateRoot, goLang, cRoot, "root", &candidateMismatches)

	productionMatchesC := len(productionMismatches) == 0
	candidateMatchesC := len(candidateMismatches) == 0

	t.Logf("production matches C oracle: %t (%d mismatch line(s))", productionMatchesC, len(productionMismatches))
	t.Logf("forced-compact matches C oracle: %t (%d mismatch line(s))", candidateMatchesC, len(candidateMismatches))
	if len(productionMismatches) > 0 {
		t.Logf("production/C divergence detail:\n%s", strings.Join(productionMismatches, "\n"))
	}
	if len(candidateMismatches) > 0 {
		t.Logf("forced-compact/C divergence detail:\n%s", strings.Join(candidateMismatches, "\n"))
	}

	switch {
	case candidateMatchesC && !productionMatchesC:
		t.Logf("ADJUDICATED EXCEPTION: the forced (v1-class, unbranched) compact route matches the C oracle; production is the divergent side on this witness. v2's own decline (confirmed separately) is still the correct routing decision for the LIVE theorem: it does not claim to admit joint-resolution proof here, it only declines. This finding does not change v2's soundness verdict; it is recorded for the campaign's adjudicated-exceptions manifest.")
	case productionMatchesC && !candidateMatchesC:
		t.Logf("Production matches the C oracle; the forced compact route diverges. The existing decline (both scalar today and v2) is unambiguously correct and load-bearing for this witness.")
	case productionMatchesC && candidateMatchesC:
		t.Logf("Both production and the forced compact route match the C oracle (no divergence either way on this witness).")
	default:
		t.Logf("Neither production nor the forced compact route matches the C oracle; both diverge from C in their own way.")
	}
}
