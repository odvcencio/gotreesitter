package grammars

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// perlParenCloseCompactRouteDetailedDecline documents, but does not assert,
// the fine-grained compact-route census classification for
// TestPerlRecoverParenCloseMatchesCleanCOracleShape's own witness bytes
// ("foo(1, 2;\n") when GTS_ADMISSION_CENSUS=1 resolves true in the reading
// process:
//
//	compact route declined at recovery [mechanism=recovery-entered]: did
//	not accept EOF: generic scheduler has no table action for the elected
//	token
//
// admissionCensusEnabled (admission_census.go) reads that env var through a
// package-level sync.Once, so its answer is fixed by whichever goroutine
// asks first in the whole test binary -- a fallback this test's own
// t.Setenv cannot retroactively flip once an earlier test (or the race
// shard's differently-ordered execution) has already resolved it. Both
// modes fold every "never reached an accepted EOF head" scheduler stop
// through requireParserCoreFreshFullAcceptance, so the plain and race
// builds instead report the coarser
// "compact route error: parser-core fresh-full runner did not accept EOF".
// TestPerlRecoverParenCloseCompactRouteStillFallsBackToProduction below
// asserts only what both forms share: the phrase "did not accept EOF".
//
// It stays a fallback today: a compact port of production's zero-width
// external rescue (the `_NONASSOC` marker perl needs at byte 4) can find and
// admit the same marker the production route shifts, but it has nowhere
// safe to act on that admission yet.
//
// Its only route to acting on a successful probe is the existing ragged
// ownership activation machinery (activateVersionLexerOwnershipAtRagged),
// which switches the whole frontier to independently-lexing owned headers.
// That machinery's own no-action-head-drop proof
// (versionLexerNoActionDropEligible, parsercore_phase0_driver.go) requires
// every live head -- the head about to be dropped and every surviving head
// it is compared against -- to sit at the SAME byte position. A rescued
// header legitimately ends up one owned token ahead of an unrescued sibling
// (it took the zero-width marker as an extra owned request the sibling never
// needed), so by the time either head needs a no-action drop the two heads
// no longer share a start byte and versionLexerNoActionDropEligible declines
// to compare them at all. The rescue is parked on that gap, not abandoned:
// see this task's own report for the exact prerequisite.
const perlParenCloseCompactRouteDetailedDecline = "compact route declined at recovery [mechanism=recovery-entered]: " +
	"did not accept EOF: generic scheduler has no table action for the elected token"

// perlParenCloseCompactRouteDeclineSubstring is the phrase both the detailed
// census form and the coarse, uninstrumented form of the decline share. See
// perlParenCloseCompactRouteDetailedDecline's own doc for why this test
// cannot depend on which form a given process reports.
const perlParenCloseCompactRouteDeclineSubstring = "did not accept EOF"

// TestPerlRecoverParenCloseCompactRouteStillFallsBackToProduction pins the
// compact route's current outcome on the same perl `_NONASSOC` witness bytes
// TestPerlRecoverParenCloseMatchesCleanCOracleShape proves production parses
// cleanly: a fallback to production, not yet an accept. It asserts the one
// substring both the detailed (GTS_ADMISSION_CENSUS=1) and coarse decline
// forms share -- see perlParenCloseCompactRouteDetailedDecline's doc comment
// for the exact detailed mechanism string this documents but cannot reliably
// assert, and for why (a process-wide sync.Once env read that an earlier
// test, or a differently-ordered race-shard run, can already have resolved
// before this test's own t.Setenv would take effect).
//
// This test exists to notice the day either gap closes: it fails the moment
// the compact route starts accepting this witness (a strict improvement this
// test's own comment invites a maintainer to update, not silently paper
// over), and it would also fail if neither decline form's shared substring
// appears at all. A real-corpus witness for the same underlying limitation:
// testdata/admission_direct/external_payload/perl.pl's own compact-route
// fallback declines at a pre-existing, unrelated "live-link cap exceeded"
// boundary (TestAdmissionCandidateExactExternalPayloadCorpus) rather than
// this recovery boundary -- any future attempt to wire a compact zero-width
// rescue in must keep that corpus file's fallback mechanism unchanged, not
// move it onto the same no-action-drop dead end this test pins here.
func TestPerlRecoverParenCloseCompactRouteStillFallsBackToProduction(t *testing.T) {
	const src = "foo(1, 2;\n"

	var entry LangEntry
	found := false
	for _, e := range AllLanguages() {
		if e.Name == "perl" {
			entry, found = e, true
			break
		}
	}
	if !found {
		t.Fatal("perl language not registered")
	}
	UnloadEmbeddedLanguage(entry.Name + ".bin")
	t.Cleanup(func() { UnloadEmbeddedLanguage(entry.Name + ".bin") })
	lang := entry.Language()

	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("compact-routed parse returned an error instead of a fallback: %v", err)
	}
	defer tree.Release()
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()

	if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
		t.Fatalf("route counters routed=%d/%d fallback=%d/%d, want an unrouted fallback",
			routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}
	reason := gotreesitter.AdmissionCandidateLastFallbackReason()
	t.Logf("compact route fallback reason: %q", reason)
	if !strings.Contains(reason, perlParenCloseCompactRouteDeclineSubstring) {
		t.Fatalf("compact route fallback reason = %q, want it to contain %q (either decline form)",
			reason, perlParenCloseCompactRouteDeclineSubstring)
	}

	// The fallback must still serve the same clean, C-exact tree production
	// parses directly: falling back to production must never itself corrupt
	// the result.
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("perl: expected a clean fallback parse (HasError()=false), got:\n%s", sexpr(root, lang))
	}
	const wantSexpr = "(source_file (expression_statement (function_call_expression (function) (list_expression (number) (number)))))"
	if got := sexpr(root, lang); got != wantSexpr {
		t.Fatalf("perl: fallback S-expression mismatch\n got: %s\nwant: %s", got, wantSexpr)
	}
}
