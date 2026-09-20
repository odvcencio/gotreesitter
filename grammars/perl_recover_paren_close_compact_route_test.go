package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// perlParenCloseCompactRouteDecline is the exact compact-route census
// classification this test pins for TestPerlRecoverParenCloseMatchesCleanCOracleShape's
// own witness bytes ("foo(1, 2;\n"). It stays a fallback today: a compact
// port of production's zero-width external rescue (the `_NONASSOC` marker
// perl needs at byte 4) can find and admit the same marker the production
// route shifts, but it has nowhere safe to act on that admission yet.
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
const perlParenCloseCompactRouteDecline = "compact route declined at recovery [mechanism=recovery-entered]: " +
	"did not accept EOF: generic scheduler has no table action for the elected token"

// TestPerlRecoverParenCloseCompactRouteStillFallsBackToProduction pins the
// compact route's current outcome on the same perl `_NONASSOC` witness bytes
// TestPerlRecoverParenCloseMatchesCleanCOracleShape proves production parses
// cleanly: a fallback to production, not yet an accept. The exact census
// decline it pins is perlParenCloseCompactRouteDecline's own mechanism string
// -- "recovery [mechanism=recovery-entered]" -- via GTS_ADMISSION_CENSUS=1.
//
// This test exists to notice the day either gap closes: it fails the moment
// the compact route starts accepting this witness (a strict improvement this
// test's own comment invites a maintainer to update, not silently paper
// over), and it would also fail if the decline mechanism regresses to
// something less specific than today's classified reason. A real-corpus
// witness for the same underlying limitation:
// testdata/admission_direct/external_payload/perl.pl's own compact-route
// fallback declines at a pre-existing, unrelated "live-link cap exceeded"
// boundary (TestAdmissionCandidateExactExternalPayloadCorpus) rather than
// this recovery boundary -- any future attempt to wire a compact zero-width
// rescue in must keep that corpus file's fallback mechanism unchanged, not
// move it onto the same no-action-drop dead end this test pins here.
func TestPerlRecoverParenCloseCompactRouteStillFallsBackToProduction(t *testing.T) {
	t.Setenv("GTS_ADMISSION_CENSUS", "1")
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
	if reason := gotreesitter.AdmissionCandidateLastFallbackReason(); reason != perlParenCloseCompactRouteDecline {
		t.Fatalf("compact route fallback reason = %q, want %q", reason, perlParenCloseCompactRouteDecline)
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
