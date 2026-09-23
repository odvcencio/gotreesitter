package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestPerlRecoverParenCloseCompactRouteAcceptsCleanly pins the compact
// route's outcome on the perl `_NONASSOC` witness bytes
// TestPerlRecoverParenCloseMatchesCleanCOracleShape proves production parses
// cleanly: the compact route now accepts the same bytes directly, with the
// identical C-exact tree, instead of falling back to production.
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
// This test exists to notice a regression the moment either mechanism
// breaks: it fails if the compact route falls back to production again, or
// if it accepts a tree that disagrees with production's own C-exact shape.
// testdata/admission_direct/external_payload/perl.pl's own compact-route
// admission (TestAdmissionCandidateExactExternalPayloadCorpus) is the
// companion real-corpus witness for the same mechanism.
func TestPerlRecoverParenCloseCompactRouteAcceptsCleanly(t *testing.T) {
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
	const wantSexpr = "(source_file (expression_statement (function_call_expression (function) (list_expression (number) (number)))))"
	if got := sexpr(root, lang); got != wantSexpr {
		t.Fatalf("perl: compact route S-expression mismatch\n got: %s\nwant: %s", got, wantSexpr)
	}
}
