//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCompactRouteLexerSkippedByteGapDeclines pins four witnesses of the
// campaign v7 class-e closure (spore.2026-08-02.alder-e.js-false-clean and
// spore.2026-08-02.hornbeam-e.byte-continuity): a lexer-skipped byte run the
// compact scheduler used to silently shift across, tolerated only because the
// B1 leaf-tiling auditor's own bytesAreSingleByteDecorationTrivia exemption
// (parsercore_phase0_driver.go) excused the resulting gap. Retiring that
// predicate makes the B1 leaf-tiling auditor decline every such gap on its
// own terms (accepted-leaf-tiling-gap), the same gate that already covers
// html_min_a and the 18-witness manifest
// (admission_route_equality_leaf_tiling_test.go).
//
// The four cases do NOT all pin the same claim. Verified per case, against
// the C oracle where noted, before writing this comment:
//
//   - js_function_stray_hash, js_statement_stray_hash: genuine false-clean
//     closures. Production and the C oracle both report an error for these
//     two inputs; the compact route used to publish HasError()==false; it
//     now correctly declines.
//   - html_amp_stray: NOT a closure this PR delivers. By the time this PR
//     landed, an independent, separately-merged change (PR #630, native
//     erroneous-end-tag recovery) already makes compact accept this exact
//     input directly, with HasError()==true, C-exact -- with or without
//     this PR's predicate retirement. This case pins that route-level
//     behavior stays stable (a regression guard), not a false-clean this
//     fix closes. The html slice of the original defect class (42
//     instances in the record's own sweep) is real and is closed by this
//     predicate retirement in general; this specific witness just no
//     longer needs that closure to already be correct.
//   - bash_stray_backslash_space: NOT a false-clean at all, and NOT a
//     closure. The C oracle parses `e \ cho hi` CLEAN
//     (HasError()==false): a backslash-space is a scanner-skipped escape
//     in bash, and the two skipped bytes are legitimately uncovered by any
//     leaf in C's OWN tree -- the leaf-tiling invariant this gate enforces
//     is not a C invariant for scanner-skip escape classes. Before this
//     fix, compact accepted this input directly and matched the C oracle
//     byte-exactly. After this fix, the now-narrower
//     bytesAreSingleByteDecorationTrivia-free auditor cannot tell this
//     shape apart from a genuine drop, so it declines -- a coverage loss
//     traded for safety, not a defect closure: the auditor's own decline
//     contract is "stop rather than guess," and it is honoring that
//     contract correctly here even though the input was actually fine.
//     The production tree served after the decline (a flat
//     ERROR[0:10], HasError()==true) diverges from the C oracle; that
//     divergence is a pre-existing, unrelated production defect in how
//     the GLR engine handles a bare backslash-space escape (real shell:
//     `echo \ leading`, `tar -c \ -f x.tar dir`, `VAR=1 \ cmd`;
//     backslash-newline line continuation is a different, unaffected
//     class), not something this PR introduces or fixes. It is exposed
//     here only because compact no longer masks it by accepting the
//     input directly. Tracked separately as a production bash repair
//     lane; out of scope for this PR.
func TestCompactRouteLexerSkippedByteGapDeclines(t *testing.T) {
	// html and javascript are not otherwise loaded by the always-on
	// (untagged) suite; purge the process-wide embedded cache afterward so
	// this test does not inflate heap for later whole-process tests (matches
	// admission_scorecard_test.go's convention).
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	cases := []struct {
		name   string
		lang   string
		source string
	}{
		// Genuine false-clean closures: production and the C oracle both
		// report an error; compact used to publish HasError()==false and
		// now correctly declines. See the doc comment above.
		{"js_function_stray_hash", "javascript", "function A(){A000000} # 0"},
		{"js_statement_stray_hash", "javascript", "a; # ; b"},
		// Route-behavior regression guard, not a closure this PR delivers:
		// PR #630's independent native erroneous-end-tag recovery already
		// makes compact accept this input correctly. See the doc comment
		// above.
		{"html_amp_stray", "html", "<html> & <body>Hello</body></html>"},
		// NOT a false-clean: the C oracle reports this input clean. Pins
		// the auditor's fail-closed decline contract, not a defect
		// closure; the production-served tree's own divergence from C is
		// a separate, pre-existing production defect. See the doc comment
		// above.
		{"bash_stray_backslash_space", "bash", "e \\ cho hi"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(c.lang)
			if entry == nil {
				t.Fatalf("language %q is not registered", c.lang)
			}
			lang := entry.Language()
			source := []byte(c.source)

			production := gts.NewParser(lang)
			production.SetAdmissionCandidateRoute(false)
			productionTree, err := production.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()
			// Production's own verdict is HasError()==true for all four
			// cases, including bash_stray_backslash_space -- that witness's
			// production tree diverges from the C oracle (see the doc
			// comment above), but production still reports it as an error
			// on its own terms, which is the verdict the route-safety check
			// below holds compact's served tree to.
			if !productionTree.RootNode().HasError() {
				t.Fatalf("production HasError=false, want true for %q", c.source)
			}

			gts.ResetAdmissionCandidateCountersForTest()
			candidate := gts.NewParser(lang)
			candidate.SetAdmissionCandidateRoute(true)
			candidateTree, err := candidate.Parse(source)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			defer candidateTree.Release()

			// The caller-visible tree -- production-served on a decline
			// (post-fallback within the same Parse call), or compact's own
			// accepted tree when a route admits it directly -- must match
			// production's own HasError verdict either way. This is a
			// route-safety invariant (the caller never sees a route-level
			// disagreement), independent of whether production's own
			// verdict happens to agree with the C oracle: bash's case
			// above satisfies this invariant while still diverging from C,
			// which is exactly why it is pinned as a route-behavior case,
			// not a false-clean-closure case.
			if got := candidateTree.RootNode().HasError(); got != productionTree.RootNode().HasError() {
				t.Fatalf("served HasError=%t, want %t (production's own verdict)",
					got, productionTree.RootNode().HasError())
			}

			routed, fallback := gts.AdmissionCandidateCounters()
			switch {
			case fallback == 1 && routed == 0:
				// Declined through the B1 leaf-tiling gate this fix
				// restores: pin the exact mechanism as direct evidence.
				reason := gts.AdmissionCandidateLastFallbackReason()
				if !strings.Contains(reason, "accepted-leaf-tiling-gap") {
					t.Fatalf("fallback reason=%q, want it to cite accepted-leaf-tiling-gap", reason)
				}
			case routed == 1 && fallback == 0:
				// Compact accepted its own tree (verified above to match
				// production's HasError verdict) through a route unrelated
				// to this gate. Not a regression; the HasError check above
				// is what matters.
			default:
				t.Fatalf("route counters routed=%d fallback=%d, want exactly one of (routed=1,fallback=0) or (routed=0,fallback=1)", routed, fallback)
			}
		})
	}
}
