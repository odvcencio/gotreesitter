package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// perlRecoverParenCloseWitnessSexpr is the C-exact shape production always
// serves for the `foo(1, 2;\n` witness, on both routes and both states of
// GOT_COMPACT_ZERO_WIDTH_RESCUE: routed compact when the switch admits the
// rescue, and production-served-through-fallback when it does not.
const perlRecoverParenCloseWitnessSexpr = "(source_file (expression_statement (function_call_expression (function) (list_expression (number) (number)))))"

func perlRecoverParenCloseWitnessParser(t *testing.T) (*gotreesitter.Parser, *gotreesitter.Language) {
	t.Helper()
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
	return parser, lang
}

// TestPerlRecoverParenCloseCompactRouteAcceptsCleanly pins the compact
// route's outcome on the perl `_NONASSOC` witness bytes
// TestPerlRecoverParenCloseMatchesCleanCOracleShape proves production parses
// cleanly, with GOT_COMPACT_ZERO_WIDTH_RESCUE admitting the rescue seam: the
// compact route accepts the same bytes directly, with the identical C-exact
// tree, instead of falling back to production.
//
// This witness was a documented, parked fallback until two mechanisms
// landed together: ownedZeroWidthCatchUp (parsercore_phase0_driver.go)
// keeps a rescued header at the same owned byte position as its siblings
// across a zero-width owned shift, so versionLexerNoActionDropEligible's
// same-start-byte proof can compare them again once the rescue fires; and
// relexZeroWidthExternalTokenForState's own call site (dispatchPassActive)
// is wired in, so a starved header actually tries the marker instead of
// leaving the probe reachable only from tests. Together they close the gap
// this test used to pin: a rescued header no longer falls one owned request
// behind an unrescued sibling, so the no-action drop that used to decline
// here now succeeds.
//
// Task #81 found that admitting the rescue by default moves
// testdata/admission_direct/external_payload/perl.pl's live-link-cap
// decline earlier and sends
// /tmp/grammar_parity/perl/test/highlight/map-grep.pm into a decline it did
// not used to reach. The seam now defaults off
// (GOT_COMPACT_ZERO_WIDTH_RESCUE, parser_config.go); this test explicitly
// admits it to keep exercising the mechanism above, and
// TestPerlRecoverParenCloseCompactRouteFallsBackByDefault pins the other
// side: the default state still serves the identical C-exact tree, through
// production's own fallback.
func TestPerlRecoverParenCloseCompactRouteAcceptsCleanly(t *testing.T) {
	const src = "foo(1, 2;\n"

	t.Setenv("GOT_COMPACT_ZERO_WIDTH_RESCUE", "1")
	gotreesitter.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)

	parser, lang := perlRecoverParenCloseWitnessParser(t)
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("compact-routed parse returned an error: %v", err)
	}
	defer tree.Release()
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()

	if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
		t.Fatalf("route counters routed=%d/%d fallback=%d/%d, want the compact route itself to accept with no fallback",
			routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}

	// The compact route's own tree must match production's C-exact shape:
	// admitting the marker must never itself produce a divergent parse.
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("perl: expected a clean compact-route parse (HasError()=false), got:\n%s", sexpr(root, lang))
	}
	if got := sexpr(root, lang); got != perlRecoverParenCloseWitnessSexpr {
		t.Fatalf("perl: compact route S-expression mismatch\n got: %s\nwant: %s", got, perlRecoverParenCloseWitnessSexpr)
	}
}

// TestPerlRecoverParenCloseCompactRouteFallsBackByDefault is
// TestPerlRecoverParenCloseCompactRouteAcceptsCleanly's default-state twin.
// With GOT_COMPACT_ZERO_WIDTH_RESCUE left unset, the compact route declines
// this same witness (its starved fork never gets the rescue's second
// chance), production serves the parse through the ordinary fallback path,
// and that served tree is still the identical C-exact shape the rescue-
// admitted test above pins. This is the regression the default-off gate
// exists to prevent: a categorized fallback with the right tree, not a
// silently wrong one.
func TestPerlRecoverParenCloseCompactRouteFallsBackByDefault(t *testing.T) {
	const src = "foo(1, 2;\n"

	gotreesitter.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)

	parser, lang := perlRecoverParenCloseWitnessParser(t)
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse returned an error: %v", err)
	}
	defer tree.Release()
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()

	if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
		t.Fatalf("route counters routed=%d/%d fallback=%d/%d, want the compact route to decline (fallback), not accept",
			routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}
	const wantReason = "compact route error: parser-core fresh-full runner did not accept EOF"
	if reason := gotreesitter.AdmissionCandidateLastFallbackReason(); reason != wantReason {
		t.Fatalf("fallback reason = %q, want %q", reason, wantReason)
	}

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("perl: expected a clean production-served parse (HasError()=false), got:\n%s", sexpr(root, lang))
	}
	if got := sexpr(root, lang); got != perlRecoverParenCloseWitnessSexpr {
		t.Fatalf("perl: fallback-served S-expression mismatch\n got: %s\nwant: %s", got, perlRecoverParenCloseWitnessSexpr)
	}
}
