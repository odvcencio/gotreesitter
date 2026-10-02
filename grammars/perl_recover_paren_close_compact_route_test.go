package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// perlRecoverParenCloseWitnessSexpr is the C-exact shape production always
// serves for the `foo(1, 2;\n` witness, on both routes and both states of
// GOT_COMPACT_ZERO_WIDTH_RESCUE. Compact now accepts directly with the
// rescue disabled because zero-width external markers retain precedence.
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
// Task #81 kept the rescue off after corpus regressions. It remains off.
// Preserving the scanner's zero-width marker now lets the ordinary compact
// route accept this witness without entering that rescue. The default-state
// twin below verifies native acceptance and the same locked-C tree.
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

// TestPerlRecoverParenCloseCompactRouteAcceptsByDefault is the default-state
// twin: the scanner's zero-width marker makes native compact acceptance
// possible with the optional rescue left off. Both states must serve the
// same clean, locked-C tree.
func TestPerlRecoverParenCloseCompactRouteAcceptsByDefault(t *testing.T) {
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

	if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
		t.Fatalf("route counters routed=%d/%d fallback=%d/%d, want native compact acceptance with no fallback",
			routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("perl: expected a clean compact parse (HasError()=false), got:\n%s", sexpr(root, lang))
	}
	if got := sexpr(root, lang); got != perlRecoverParenCloseWitnessSexpr {
		t.Fatalf("perl: default compact S-expression mismatch\n got: %s\nwant: %s", got, perlRecoverParenCloseWitnessSexpr)
	}
}
