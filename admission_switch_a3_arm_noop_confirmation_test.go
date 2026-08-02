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
// RESULT, corrected from the finding: only Apex's witness confirms as a
// no-op. Perl, Python, and Ada's compat arms still perform real,
// load-bearing tree reshaping on compact-origin trees even after A3
// certification lands, on their own tied-election witnesses. The finding's
// "compact-raw equals compact-tailed on all six witnesses" claim does not
// reproduce for four of those six (perl push-list; python tuple assignment;
// both ada aggregates). It reproduces for the fifth, apex class-literal.
//
// This is consistent with the arm taxonomy (spec.campaign.v7): apex's arm
// is a fixed derivation relabel that A3's primary-acceptance-derivation
// certification already subsumes at the scheduler level, so there is
// nothing left for the arm to do. Perl and Python's arms are
// scheduler-action arms that perform source-text-scanning list regrouping
// (ambiguous_function_call_expression / pattern_list-vs-expression_list) --
// a materially stronger transformation than picking among tied derivations.
// Ada's arms (materialization-owned) relabel one aggregate production into
// another via a similar structural scan. None of these three
// transformations are subsumed by the admission-time election flags this
// workstream certifies.
//
// Follow-up arm-deletion PRs for perl, python, and ada must not treat this
// finding's retirement precondition as met on this evidence; apex's
// arm-deletion follow-on can cite this receipt directly.
//
// This does NOT affect A3's own certification landing. The certified
// languages' full-corpus sweeps (cgo_harness a3_certification_sweep
// pattern) compare the real shipped Parse() output, which always applies
// the compat tail regardless of route, and found zero unadjudicated
// divergence there. The no-op question is a separate, later concern: only
// whether the arm itself can be deleted.

func TestA3ArmNoOpConfirmationApexConfirms(t *testing.T) {
	source := []byte("public class C {\n  void m() {\n    Object t = RecordPage.class;\n  }\n}\n")
	lang := grammars.ApexLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("apex did not receive its A3 certification")
	}
	assertA3ArmNoOp(t, "apex/class_literal_alias", lang, source, true)
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

func TestA3ArmNoOpConfirmationPythonDoesNotConfirm(t *testing.T) {
	lang := grammars.PythonLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified || !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("python did not receive its A3 certification")
	}
	for _, tt := range []struct{ name, source string }{
		{"assignment_bare_tuple", "x, y, z = 1, 2, 3\nxyz = x, y, z\n"},
		{"assignment_bare_pair", "a = 1\nb = 2\npair = a, b\n"},
	} {
		assertA3ArmNoOp(t, "python/"+tt.name, lang, []byte(tt.source), false)
	}
}

func TestA3ArmNoOpConfirmationAdaDoesNotConfirm(t *testing.T) {
	lang := grammars.AdaLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified || !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("ada did not receive its A3 certification")
	}
	for _, tt := range []struct{ name, source string }{
		{"locked_positional_array_aggregate", "package P is\n   type A is array (1 .. 3) of Boolean;\n   V : constant A := (1, 2, 3);\nend;\n"},
		{"array_others_choice", "procedure P is\nbegin\n   A := (others => 0);\nend;\n"},
	} {
		assertA3ArmNoOp(t, "ada/"+tt.name, lang, []byte(tt.source), false)
	}
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
func assertA3ArmNoOp(t *testing.T, name string, lang *gts.Language, source []byte, wantNoOp bool) {
	t.Helper()

	tailed := gts.NewParser(lang)
	tailed.SetAdmissionCandidateRoute(true)
	tailedTree, err := tailed.Parse(source)
	if err != nil {
		t.Fatalf("%s: tailed parse: %v", name, err)
	}
	defer tailedTree.Release()

	raw := gts.NewParser(lang)
	raw.SetAdmissionCandidateRoute(true)
	restore := gts.SetNoResultCompatibilityBenchmarkOnlyForTest(raw, true)
	defer restore()
	rawTree, err := raw.Parse(source)
	if err != nil {
		t.Fatalf("%s: raw parse: %v", name, err)
	}
	defer rawTree.Release()

	tailedSExpr := tailedTree.RootNode().SExpr(lang)
	rawSExpr := rawTree.RootNode().SExpr(lang)
	gotNoOp := tailedSExpr == rawSExpr
	if gotNoOp != wantNoOp {
		t.Fatalf(
			"%s: arm no-op status changed (compact-raw==compact-tailed is now %t, want %t) -- "+
				"this flips the campaign's retirement evidence, re-verify before updating either "+
				"the arm-deletion readiness or this receipt.\nraw   =%s\ntailed=%s",
			name, gotNoOp, wantNoOp, rawSExpr, tailedSExpr,
		)
	}
}
