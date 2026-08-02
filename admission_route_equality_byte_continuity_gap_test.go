//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCompactRouteLexerSkippedByteGapDeclines pins the campaign v7 class-e
// closure (spore.2026-08-02.alder-e.js-false-clean and
// spore.2026-08-02.hornbeam-e.byte-continuity): a lexer-skipped byte run the
// compact scheduler used to silently shift across, tolerated only because the
// B1 leaf-tiling auditor's own bytesAreSingleByteDecorationTrivia exemption
// (parsercore_phase0_driver.go) excused the resulting gap. The javascript
// witness `function A(){A000000} # 0` is the record's literal reproduction:
// byte 22 holds a stray "#" flanked by spaces; the DFA lexer's skipOneRune
// path (lexer.go) silently drops it (javascript does not set
// cRecoveryEnabled), and the compact scheduler used to shift the next real
// token ("number" at byte 24) across the gap with no continuity check,
// publishing HasError()==false while production and the C oracle both report
// an error. Retiring bytesAreSingleByteDecorationTrivia closes the class: the
// B1 leaf-tiling auditor now declines every one of these gaps on its own
// terms (accepted-leaf-tiling-gap), the same gate that already covers
// html_min_a and the 18-witness manifest
// (admission_route_equality_leaf_tiling_test.go).
//
// The correctness contract this test pins is servedHasError ==
// productionHasError == true for every case, not "compact must decline":
// an independent, later compact improvement (native html erroneous-end-tag
// recovery) can legitimately promote a witness from "declines, production
// serves" to "compact accepts its own correct tree" without regressing
// anything this tranche cares about. Where compact still declines, the
// reason is also pinned to accepted-leaf-tiling-gap as direct evidence of
// which gate closed it.
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
		{"js_function_stray_hash", "javascript", "function A(){A000000} # 0"},
		{"js_statement_stray_hash", "javascript", "a; # ; b"},
		{"html_amp_stray", "html", "<html> & <body>Hello</body></html>"},
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
			// accepted tree when a native recovery path admits it directly --
			// must match production's own HasError verdict either way. This
			// is the class-e contract: no route may publish HasError()==false
			// for an input production (and the C oracle) call an error.
			if got := candidateTree.RootNode().HasError(); got != productionTree.RootNode().HasError() {
				t.Fatalf("served HasError=%t, want %t (production's own verdict)",
					got, productionTree.RootNode().HasError())
			}
			if !candidateTree.RootNode().HasError() {
				t.Fatal("served tree HasError=false, want true")
			}

			routed, fallback := gts.AdmissionCandidateCounters()
			switch {
			case fallback == 1 && routed == 0:
				// Declined through the B1 leaf-tiling gate this fix restores:
				// pin the exact mechanism as direct evidence.
				reason := gts.AdmissionCandidateLastFallbackReason()
				if !strings.Contains(reason, "accepted-leaf-tiling-gap") {
					t.Fatalf("fallback reason=%q, want it to cite accepted-leaf-tiling-gap", reason)
				}
			case routed == 1 && fallback == 0:
				// Compact accepted its own correct tree (verified above)
				// through a native recovery path unrelated to this gate. Not
				// a regression; the HasError check above is what matters.
			default:
				t.Fatalf("route counters routed=%d fallback=%d, want exactly one of (routed=1,fallback=0) or (routed=0,fallback=1)", routed, fallback)
			}
		})
	}
}
