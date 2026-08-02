//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCompactRouteRootLeadingGapDeclines pins the eight fuzzer-found
// reproductions of the root-start pull-back class: the accepted
// derivation's own root reduce declared a span that starts after the
// source's first non-trivia byte -- one real byte at document start that no
// node in the derivation ever represented -- fully tiled within itself, so
// diagnosticParserCoreReduceChildrenTilingGap's own-children check (exempt at
// the root symbol, isDerivationRootReduce) never sees a gap. Before the
// accepted-root-leading-gap decline, the shared post-materialization
// normalizeRootSourceStart (parser_result_root_build.go) pulled the public
// root span back over the dropped byte on the assumption of a legitimately
// elided leading extra, publishing HasError()==false while production
// correctly reports an error for the same bytes.
func TestCompactRouteRootLeadingGapDeclines(t *testing.T) {
	// html, erlang, and haskell are not otherwise loaded by the always-on
	// (untagged) suite; purge the process-wide embedded cache afterward so
	// this test does not inflate heap for later whole-process tests (matches
	// admission_scorecard_test.go's convention).
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	cases := []struct {
		name   string
		lang   string
		source string
	}{
		{"html_amp_digit", "html", "&0"},
		{"html_amp_semicolon", "html", "&;"},
		{"html_amp_hash", "html", "&#"},
		{"html_close_angle_digit", "html", ">0"},
		{"html_amp_zeros", "html", "&000"},
		{"erlang_soh_digit", "erlang", "\x010"},
		{"erlang_du_digit", "erlang", "\x100"},
		{"haskell_unterminated_string", "haskell", "\"\n"},
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

			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != 0 || fallback != 1 {
				t.Fatalf("route counters routed=%d fallback=%d, want routed=0 fallback=1 (compact must decline)", routed, fallback)
			}
			reason := gts.AdmissionCandidateLastFallbackReason()
			if c.lang == "html" {
				// B3 stage S3 forces the shared token source's error-run
				// lexing on whenever native html recovery is admitted
				// (parser_api.go's acquireParserDFATokenSourceWithErrorRuns),
				// so these root-leading-gap bytes ('&', '>') now surface as
				// their own errorSymbol token instead of being silently
				// skipped by the plain lexer. s3TryOpenErrorRegion declines
				// them itself (the guard requires at least one real shifted
				// token before the absorbed byte, which none of these
				// zero-content sources have), so the parse never reaches
				// materialization at all and falls back earlier, at the
				// generic no-table-action recovery boundary, rather than at
				// the later accepted-root-leading-gap span check. Both
				// boundaries still decline and fall back to production
				// unchanged (the functional contract this test pins below);
				// only the internal boundary name differs, exactly like B3
				// stage S1's own no-table-action reclassification.
				if !strings.Contains(reason, "did not accept EOF") {
					t.Fatalf("fallback reason=%q, want it to cite the generic no-table-action decline (B3 stage S3 changes which boundary an html leading-byte gap hits first)", reason)
				}
			} else if !strings.Contains(reason, "accepted-root-leading-gap") {
				t.Fatalf("fallback reason=%q, want it to cite accepted-root-leading-gap", reason)
			}

			// The caller-visible tree is production-served (post-fallback
			// within the same Parse call), so it must match production's own
			// verdict exactly.
			if got := candidateTree.RootNode().HasError(); got != productionTree.RootNode().HasError() {
				t.Fatalf("production-served (post-fallback) HasError=%t, want %t (production's own verdict)",
					got, productionTree.RootNode().HasError())
			}
			if got, want := candidateTree.RootNode().StartByte(), productionTree.RootNode().StartByte(); got != want {
				t.Fatalf("production-served (post-fallback) root startByte=%d, want %d", got, want)
			}
			if got, want := candidateTree.RootNode().EndByte(), productionTree.RootNode().EndByte(); got != want {
				t.Fatalf("production-served (post-fallback) root endByte=%d, want %d", got, want)
			}
		})
	}
}

// TestCompactRouteLeadingCommentStillServes is the accepted-root-leading-gap
// check's fail-open control: a source that begins with a comment (or other
// leading extra) and then real code must NOT decline, because a legitimate
// leading extra sits INSIDE the accepted derivation's own root span (the
// root reduce's raw declared start already covers it), unlike the eight
// dropped-byte reproductions above where no node covers the gap at all. This
// pins the trivia-awareness the fix direction called for: the check
// distinguishes "the derivation never represented this byte" from "a
// retained extra owns this byte", using the same raw pre-normalization
// span the decline above reads.
//
// Verified directly: all three cases route through the compact candidate
// with no fallback both before and after this change lands, so the check
// does not regress any of them.
func TestCompactRouteLeadingCommentStillServes(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	cases := []struct {
		name   string
		lang   string
		source string
	}{
		{"go_leading_line_comment", "go", "// leading comment\npackage main\n\nfunc main() {}\n"},
		{"html_leading_block_comment", "html", "<!-- leading comment --><a></a>"},
		{"javascript_leading_line_comment", "javascript", "// leading comment\nconsole.log(1);\n"},
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
				reason := gts.AdmissionCandidateLastFallbackReason()
				t.Fatalf("route counters routed=%d fallback=%d reason=%q, want routed=1 fallback=0 (a leading comment sits inside the root span and must not trip the leading-gap decline)",
					routed, fallback, reason)
			}

			if diff := routeEqualityFirstDivergence(candidateTree.RootNode(), productionTree.RootNode(), lang, "root"); diff != "" {
				t.Fatalf("structural divergence: %s", diff)
			}
		})
	}
}
