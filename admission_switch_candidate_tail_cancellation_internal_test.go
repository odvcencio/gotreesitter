//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"
)

// TestFinalizeCompactReturnedTreeForParsePreservesCompletedTreeUnderPresetCancellation
// is the deterministic counterpart to the end-to-end race,
// TestCompatTailElisionSurvivesConcurrentCancellation
// (admission_compat_tail_elision_test.go), which reproduces the reintroduced
// regression only on some fraction of runs because it races a background
// goroutine's flag flip against a live Parse call. This test removes the
// race entirely: it materializes a real compact-route tree directly (the
// same runner.parse call tryCompactFullParseRoute itself makes), THEN sets
// the cancellation flag, THEN calls the returned-tree tail function
// directly. With the flag already set before the call, the mechanism either
// catches the reintroduced bug or it does not on every single run -- no
// timing window, no flake.
//
// See admission_switch_candidate.go's finalizeCompactReturnedTreeForParse doc
// comment ("Corrected finding") for the historical bug this guards: an
// earlier version checked the terminal stop reason unconditionally, ahead of
// the "!tree.resultCompatibilityApplied" guard, so a cancellation observed
// strictly after materialization already completed a good tree discarded
// that tree instead of returning it.
//
// Both tail functions are exercised on their own real compact-materialized
// tree: finalizeCompactReturnedTreeForParse (the elision-eligible path that
// carried the historical bug) and normalizeReturnedTreeForParse (the
// production tail every non-eligible language still runs, and the oracle
// finalizeCompactReturnedTreeForParse's own doc comment says its control
// flow reproduces exactly).
func TestFinalizeCompactReturnedTreeForParsePreservesCompletedTreeUnderPresetCancellation(t *testing.T) {
	source := []byte("1 + 2 + 3")

	t.Run("finalizeCompactReturnedTreeForParse", func(t *testing.T) {
		p := NewParser(buildArithmeticLanguage())
		p.SetAdmissionCandidateRoute(true)
		tree := mustMaterializeCompactTreeForTailTest(t, p, source)
		defer tree.Release()
		requireCompactTailPreconditions(t, tree, "finalizeCompactReturnedTreeForParse")

		var flag uint32 = 1
		p.SetCancellationFlag(&flag)

		p.finalizeCompactReturnedTreeForParse(tree, source)

		if tree.ParseStoppedEarly() {
			t.Fatalf("finalizeCompactReturnedTreeForParse discarded an already-complete compact tree under a pre-set cancellation flag: ParseStopReason()=%q", tree.ParseStopReason())
		}
	})

	t.Run("normalizeReturnedTreeForParse", func(t *testing.T) {
		p := NewParser(buildArithmeticLanguage())
		p.SetAdmissionCandidateRoute(true)
		tree := mustMaterializeCompactTreeForTailTest(t, p, source)
		defer tree.Release()
		requireCompactTailPreconditions(t, tree, "normalizeReturnedTreeForParse")

		var flag uint32 = 1
		p.SetCancellationFlag(&flag)

		p.normalizeReturnedTreeForParse(tree, source)

		if tree.ParseStoppedEarly() {
			t.Fatalf("normalizeReturnedTreeForParse discarded an already-complete compact tree under a pre-set cancellation flag: ParseStopReason()=%q", tree.ParseStopReason())
		}
	})
}

// mustMaterializeCompactTreeForTailTest drives a real compact-route parse
// through the same runner.parse call tryCompactFullParseRoute itself makes,
// stopping short of that function's own tail call so the caller can invoke
// the tail directly and control exactly when the cancellation flag is set.
func mustMaterializeCompactTreeForTailTest(t *testing.T, p *Parser, source []byte) *Tree {
	t.Helper()
	runner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatalf("acquire admission candidate runner: %v", err)
	}
	endOp := p.beginParseOperationBudget()
	defer endOp()
	endParse := p.enterParseBudget()
	defer endParse()
	tree, err := runner.parse(source)
	if err != nil {
		t.Fatalf("compact runner declined a real materialization: %v", err)
	}
	if tree == nil {
		t.Fatal("compact runner returned a nil tree")
	}
	return tree
}

// requireCompactTailPreconditions asserts the two facts that make this test
// meaningful: the tree is a genuine compact-route materialization
// (resultCompatibilityApplied already true, matching every real
// compact-route tree per finalizeCompactReturnedTreeForParse's doc comment)
// and it has not already stopped early (so shouldNormalizeReturnedTree lets
// the tail function's body run at all).
func requireCompactTailPreconditions(t *testing.T, tree *Tree, label string) {
	t.Helper()
	if !tree.compactMaterialized {
		t.Fatalf("%s: precondition failed: tree is not compact-materialized", label)
	}
	if !tree.resultCompatibilityApplied {
		t.Fatalf("%s: precondition failed: tree.resultCompatibilityApplied is false before the tail call; this test is meaningless unless materialization already applied result compatibility", label)
	}
	if tree.rawParseStoppedEarly() {
		t.Fatalf("%s: precondition failed: tree already reports stopped-early before the tail call", label)
	}
}
