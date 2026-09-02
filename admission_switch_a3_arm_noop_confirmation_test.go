//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// This file is the A3 certification-workstream (spec.campaign.v7, finding
// tied-election-family-compact-retirement) arm no-op confirmation receipt.
//
// Method (the finding's own method note): compare the compact route's raw
// runner tree against its normal tailed tree. The compat tail runs inside
// the compact route wrapper, so the probe must suppress it directly
// (SetNoResultCompatibilityBenchmarkOnlyForTest) rather than through the
// public ParseNoResultCompatibilityBenchmarkOnly wrapper, which also forces
// production and can never serve as a compact-route probe. Equality between
// the raw and tailed trees would prove the language's compat arm is a no-op
// on compact-origin trees for that witness -- the retirement precondition
// follow-up arm-deletion PRs need.
//
// RESULT, corrected from the finding: Python's tuple-assignment witnesses now
// confirm as no-op after C-ordered clean-tie selection moves the choice into
// the producer. Perl and Ada still perform load-bearing reshaping on their
// tied-election witnesses. Apex has a separate material-election decline.
//
// This is consistent with the arm taxonomy (spec.campaign.v7): apex's arm
// is a fixed derivation relabel that A3's primary-acceptance-derivation
// certification already subsumes at the scheduler level, so there is
// nothing left for the arm to do. Perl's arm is a scheduler-action arm that
// performs source-text-scanning list regrouping
// (ambiguous_function_call_expression). Python's former
// pattern_list-vs-expression_list rewrite is now an observed no-op on the
// certified tuple witnesses because the producer makes the C choice.
// Ada's arms (materialization-owned) relabel one aggregate production into
// another via a similar structural scan. None of these three
// transformations are subsumed by the admission-time election flags this
// workstream certifies.
//
// Follow-up arm-deletion PRs for perl, python, and ada must not treat this
// finding's retirement precondition as met on this evidence.
//
// UPDATE: ada's array_others_choice witness now confirms. A declared-conflict
// election policy (grammars/runtime_profiles.go "ada",
// ConflictPolicyDeclaredReduceReduceHighestSymbol in conflict_policy.go)
// resolves the component_choice_list/discrete_choice tie natively before the
// parser forks, so dispatch.ada.association-choice-materialization
// (previously materialization-owned) retired outright and
// dispatch.ada.aggregate-kind-election no longer reshapes this witness --
// see TestA3ArmNoOpConfirmationAdaOthersChoiceConfirms.
// locked_positional_array_aggregate's ambiguity
// (record_component_association_list vs positional_array_aggregate) is a
// separate shift/reduce fork with no dynamic-precedence differentiator, a
// table-generation gap this policy mechanism does not reach; ada's arm
// stays load-bearing for that witness.
//
// UPDATE: apex's witness no longer confirms. selectCompactAcceptanceDerivation's
// materiality gate (parsercore_phase0_driver.go,
// compactAcceptanceElectionIsVacuous) found the class_literal_alias witness's
// two tied derivations are not byte-identical (one assigns the "object"
// field, the other does not), so the compact route now declines it instead
// of publishing an unproven pick. With the route no longer originating a
// tree for this witness, the raw-vs-tailed comparison this file's method
// depends on no longer applies -- see
// TestA3ArmNoOpConfirmationApexClassLiteralDeclinesAsMaterialElection. Apex's
// arm-deletion follow-on needs a different witness or a direct review before
// it can still cite this receipt.
//
// This does NOT affect A3's own certification landing. The certified
// languages' full-corpus sweeps (cgo_harness a3_certification_sweep
// pattern) compare the real shipped Parse() output, which always applies
// the compat tail regardless of route, and found zero unadjudicated
// divergence there beyond the enumerated, tracked pre-existing
// production-route defects each sweep file carries (perlA3KnownDivergences,
// pythonA3KnownDivergences, adaA3KnownDivergences; apex carries none). Those
// defects reproduce with the compact route disabled, so the compact route
// contributes nothing to them; they are unrelated to this file's own arm
// no-op question. The no-op question is a separate, later concern: only
// whether the arm itself can be deleted.

// TestA3ArmNoOpConfirmationApexClassLiteralDeclinesAsMaterialElection
// supersedes this file's original apex "confirms no-op" receipt for the
// class_literal_alias witness. selectCompactAcceptanceDerivation's
// materiality gate (parsercore_phase0_driver.go,
// compactAcceptanceElectionIsVacuous) found this witness's two tied
// derivations are not byte-identical -- one assigns the "object" field to
// its first child, the other does not -- so the compact route now declines
// this witness (fail-closed) instead of publishing the positional primary,
// and falls back to production on both the raw and tailed probes.
//
// The route no longer originates a tree here at all, so the raw-vs-tailed
// "arm no-op" comparison this file's method depends on no longer applies to
// this witness: with neither side routed, any raw/tailed difference would
// only be re-measuring production's own noResultCompatibilityBenchmarkOnly
// behavior, not the compact route's. The apex arm's retirement evidence
// (this file's original RESULT note) needs a different witness or a direct
// review before it can still be cited; this test only pins the corrected
// admission outcome, not an arm no-op finding.
func TestA3ArmNoOpConfirmationApexClassLiteralDeclinesAsMaterialElection(t *testing.T) {
	source := []byte("public class C {\n  void m() {\n    Object t = RecordPage.class;\n  }\n}\n")
	lang := grammars.ApexLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("apex did not receive its A3 certification")
	}

	for _, tt := range []struct {
		name                    string
		noResultCompatOnlyProbe bool
	}{
		{"tailed", false},
		{"raw", true},
	} {
		gts.ResetAdmissionCandidateCountersForTest()
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(true)
		var restore func()
		if tt.noResultCompatOnlyProbe {
			restore = gts.SetNoResultCompatibilityBenchmarkOnlyForTest(p, true)
		}
		tree, err := p.Parse(source)
		if restore != nil {
			restore()
		}
		if err != nil {
			t.Fatalf("apex/class_literal_alias %s: parse: %v", tt.name, err)
		}
		tree.Release()
		routed, fallback := gts.AdmissionCandidateCounters()
		if routed != 0 || fallback != 1 {
			t.Fatalf(
				"apex/class_literal_alias %s: route counters routed=%d fallback=%d, want 0/1 "+
					"(the materiality gate should decline this material election); reason=%s",
				tt.name, routed, fallback, gts.AdmissionCandidateLastFallbackReason(),
			)
		}
	}
}

func TestA3ArmNoOpConfirmationPerlDoesNotConfirm(t *testing.T) {
	lang := grammars.PerlLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified || !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("perl did not receive its A3 certification")
	}
	for _, tt := range []struct{ name, source string }{
		{"push_two_args", "push @found, $_;\n"},
		{"push_three_args", "push @found, $a, $b;\n"},
	} {
		assertA3ArmNoOp(t, "perl/"+tt.name, lang, []byte(tt.source), false)
	}
}

func TestA3ArmNoOpConfirmationPythonConfirms(t *testing.T) {
	lang := grammars.PythonLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified || !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("python did not receive its A3 certification")
	}
	for _, tt := range []struct{ name, source string }{
		{"assignment_bare_tuple", "x, y, z = 1, 2, 3\nxyz = x, y, z\n"},
		{"assignment_bare_pair", "a = 1\nb = 2\npair = a, b\n"},
	} {
		assertA3ArmNoOp(t, "python/"+tt.name, lang, []byte(tt.source), true)
	}
}

// TestA3ArmNoOpConfirmationAdaDoesNotConfirm covers Ada's still-load-bearing
// witness. locked_positional_array_aggregate's ambiguity
// (record_component_association_list vs positional_array_aggregate, a
// shift/reduce fork with no dynamic-precedence differentiator) is a
// table-generation gap, not a runtime election policy; the arm still
// reshapes the compact route's tree for it.
func TestA3ArmNoOpConfirmationAdaDoesNotConfirm(t *testing.T) {
	lang := grammars.AdaLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified || !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("ada did not receive its A3 certification")
	}
	for _, tt := range []struct{ name, source string }{
		{"locked_positional_array_aggregate", "package P is\n   type A is array (1 .. 3) of Boolean;\n   V : constant A := (1, 2, 3);\nend;\n"},
	} {
		assertA3ArmNoOp(t, "ada/"+tt.name, lang, []byte(tt.source), false)
	}
}

// TestA3ArmNoOpConfirmationAdaOthersChoiceConfirms is the corrected receipt
// for Ada's other tied-election witness. array_others_choice's ambiguity
// (component_choice_list vs discrete_choice, a declared reduce-reduce
// conflict with no dynamic precedence) is now resolved natively by a
// declared-conflict election policy before the parser forks
// (grammars/runtime_profiles.go "ada"), so the compact route's raw and
// tailed trees are identical: the arm is confirmed inert on this witness.
func TestA3ArmNoOpConfirmationAdaOthersChoiceConfirms(t *testing.T) {
	lang := grammars.AdaLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified || !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("ada did not receive its A3 certification")
	}
	source := "procedure P is\nbegin\n   A := (others => 0);\nend;\n"
	assertA3ArmNoOp(t, "ada/array_others_choice", lang, []byte(source), true)
}

// TestA3ArmNoOpConfirmationTrivialWitnessesAreVacuouslyNoOp documents the
// baseline: on a source that never triggers a language's compat arm at
// all, compact-raw trivially equals compact-tailed for every certified
// language (there is nothing for the arm to rewrite). This is a different,
// weaker property than the tied-election witnesses above and must not be
// read as confirming the retirement precondition.
func TestA3ArmNoOpConfirmationTrivialWitnessesAreVacuouslyNoOp(t *testing.T) {
	tests := []struct {
		name string
		load func() *gts.Language
	}{
		{"perl", grammars.PerlLanguage},
		{"python", grammars.PythonLanguage},
		{"apex", grammars.ApexLanguage},
		{"ada", grammars.AdaLanguage},
	}
	for _, tt := range tests {
		lang := tt.load()
		source := []byte(grammars.ParseSmokeSample(tt.name))
		assertA3ArmNoOp(t, tt.name+"/smoke_sample", lang, source, true)
	}
}

// assertA3ArmNoOp compares the compact route's raw (pre-compat-tail) tree
// against its normal tailed tree for source, and asserts the observed
// no-op status matches wantNoOp exactly (a Fatal either way -- a status
// flip in either direction changes the campaign's retirement evidence and
// must be investigated, not silently accepted).
//
// Tailed (normal) must route through the compact route: a tailed fallback
// means the witness is a material election under
// selectCompactAcceptanceDerivation's materiality gate
// (compactAcceptanceElectionIsVacuous) and needs its own dedicated test (see
// TestA3ArmNoOpConfirmationApexClassLiteralDeclinesAsMaterialElection), not
// this shared no-op probe.
//
// Raw (SetNoResultCompatibilityBenchmarkOnlyForTest) suppresses the compat
// arm entirely, including during the gate's own comparison (Node.RootNode
// applies the compat finalizer, so the gate's tailed-mode comparison already
// runs post-arm; suppressing the arm removes whatever convergence it
// provided). If raw then declines while tailed still routes, the arm was
// exactly what let the election resolve at all -- unambiguous evidence it is
// not a no-op, so this counts as gotNoOp=false without attempting the SExpr
// comparison (there is no raw compact-route tree to compare).
func assertA3ArmNoOp(t *testing.T, name string, lang *gts.Language, source []byte, wantNoOp bool) {
	t.Helper()

	gts.ResetAdmissionCandidateCountersForTest()
	tailed := gts.NewParser(lang)
	tailed.SetAdmissionCandidateRoute(true)
	tailedTree, err := tailed.Parse(source)
	if err != nil {
		t.Fatalf("%s: tailed parse: %v", name, err)
	}
	defer tailedTree.Release()
	if routed, fallback := gts.AdmissionCandidateCounters(); routed != 1 || fallback != 0 {
		t.Fatalf("%s: tailed parse did not route through the compact route (routed=%d fallback=%d reason=%q)",
			name, routed, fallback, gts.AdmissionCandidateLastFallbackReason())
	}

	gts.ResetAdmissionCandidateCountersForTest()
	raw := gts.NewParser(lang)
	raw.SetAdmissionCandidateRoute(true)
	restore := gts.SetNoResultCompatibilityBenchmarkOnlyForTest(raw, true)
	rawTree, err := raw.Parse(source)
	restore()
	if err != nil {
		t.Fatalf("%s: raw parse: %v", name, err)
	}
	defer rawTree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()
	// Exactly one of routed/fallback must be 1: (routed == 1) == (fallback
	// == 1) is true both when neither fired (0, 0; no parse reached a
	// terminal outcome) and when both did (1, 1; a double count), and false
	// only in the two valid, mutually exclusive outcomes (1, 0) and (0, 1).
	if (routed == 1) == (fallback == 1) {
		t.Fatalf("%s: ambiguous raw route counters routed=%d fallback=%d, want exactly one to be 1", name, routed, fallback)
	}

	tailedSExpr := tailedTree.RootNode().SExpr(lang)
	var gotNoOp bool
	var rawSExpr string
	if routed == 1 {
		rawSExpr = rawTree.RootNode().SExpr(lang)
		gotNoOp = tailedSExpr == rawSExpr
	} else {
		// Raw declined (the arm's absence made the election unresolvable):
		// definitively not a no-op, and there is no raw compact-route tree
		// to compare against tailed's.
		rawSExpr = "<declined: " + gts.AdmissionCandidateLastFallbackReason() + ">"
		gotNoOp = false
	}
	if gotNoOp != wantNoOp {
		t.Fatalf(
			"%s: arm no-op status changed (compact-raw==compact-tailed is now %t, want %t) -- "+
				"this flips the campaign's retirement evidence, re-verify before updating either "+
				"the arm-deletion readiness or this receipt.\nraw   =%s\ntailed=%s",
			name, gotNoOp, wantNoOp, rawSExpr, tailedSExpr,
		)
	}
}
