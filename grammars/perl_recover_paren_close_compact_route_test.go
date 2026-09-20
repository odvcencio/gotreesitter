package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// perlParenCloseCompactRouteDecline is the current compact-route census
// classification for TestPerlRecoverParenCloseMatchesCleanCOracleShape's own
// witness bytes ("foo(1, 2;\n"). It stays a fallback today: see
// relexZeroWidthExternalTokenForState's design-decision comment
// (parsercore_phase0_driver.go) for why the port stops short of admitting it.
const perlParenCloseCompactRouteDecline = "compact route declined at recovery [mechanism=recovery-entered]: " +
	"did not accept EOF: generic scheduler has no table action for the elected token"

// TestPerlRecoverParenCloseCompactRouteStillFallsBackToProduction pins the
// compact route's current outcome on the same perl `_NONASSOC` witness bytes
// TestPerlRecoverParenCloseMatchesCleanCOracleShape proves production parses
// cleanly: a fallback to production, not yet an accept.
//
// relexZeroWidthExternalTokenForState (parsercore_phase0_driver.go) ports
// production's non-identity-based rescue proof to the compact route and is
// directly unit-tested (parsercore_phase0_relex_zero_width_external_witness_test.go,
// gts_parsercorephase0 build) to admit this exact witness grammar's marker.
// It is not wired into the live dispatch path: its only route to acting on a
// successful probe (ragged ownership activation,
// activateVersionLexerOwnershipAtRagged) has its own no-action-head-drop
// proof that cannot yet tolerate the byte-ragged frontier a rescued header
// produces once the marker fork runs one owned token ahead of an unrescued
// sibling. Wiring the call in was measured to still decline this exact
// witness (from inside owned dispatch instead of the ordinary no-action
// path) while additionally moving an unrelated real-corpus perl fallback
// (testdata/admission_direct/external_payload/perl.pl) from a pre-existing
// "live-link cap exceeded" decline to the same owned no-action-drop dead
// end -- a regression TestAdmissionCandidateExactExternalPayloadCorpus
// pins. This test exists to notice the day either gap closes: it fails the
// moment the compact route starts accepting this witness (a strict
// improvement this test's own comment invites a maintainer to update, not
// silently paper over), and it would also fail if the decline mechanism
// regresses to something less specific than today's classified reason.
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
