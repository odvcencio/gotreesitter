//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// nightlyRouteMatch states what TestNightlyFuzzRouteWitnessesLockedC requires
// of one Go route against locked C.
type nightlyRouteMatch int

const (
	// nightlyRouteExact requires the locked C tree: the same nodes, fields,
	// spans, points and flags.
	nightlyRouteExact nightlyRouteMatch = iota
	// nightlyRouteHasError requires the same HasError result as locked C. The
	// test logs any tree difference as an open one.
	nightlyRouteHasError
	// nightlyRouteOpen requires nothing. The test logs the HasError results and
	// the first tree difference.
	nightlyRouteOpen
)

// TestNightlyFuzzRouteWitnessesLockedC adjudicates the FuzzAdmissionRouteEquality
// and FuzzJavaScriptAdmissionRouteEquality witnesses from the fuzz-nightly runs
// of 2026-10-08 to 2026-10-13 against the locked C oracle.
//
// Go "," (seed 0d6dbe8c80002ec6) finishes two compact recovery trees at the
// same error cost. C keeps the earlier finished tree as the incumbent and takes
// the later one on a tie, so it publishes (source_file (ERROR)). Both routes
// must match C exactly. The second Go witness already matched C on both routes.
//
// On the two Haskell witnesses, the compact route matches C and production
// does not: production's one-survivor merge drops the pattern-signature reading
// that C keeps as a second link. Changing that needs a per-grammar merge
// admission grant, so this test pins only the compact route for them and logs
// the production difference.
//
// On the two JavaScript witnesses, locked C reports an error and production
// reports none (#1519). When the lookahead cannot continue the stack but
// can start a top-level construct, production's recovery pops the stuck
// fragments and re-attaches them as top-level extras without an ERROR node
// (tryReplayTopLevelRecovery, in its fully degenerate split). The compact route
// reports the error on both inputs. On ";! ;A0" it matches C exactly. On
// "<A><A>AAAAA</A></A>0" C inserts a missing ";" and starts a second statement,
// while compact wraps the number in an ERROR node, so that case pins only the
// compact route's HasError result. This test logs the production difference.
// When production reports the error, require its HasError result here.
func TestNightlyFuzzRouteWitnessesLockedC(t *testing.T) {
	for _, tc := range []struct {
		name, language, source string
		compact, production    nightlyRouteMatch
	}{
		{"go_lone_comma", "go", ",", nightlyRouteExact, nightlyRouteExact},
		{"go_unterminated_string", "go", "package main\n\nfunc main() {\"00000000000\n,", nightlyRouteExact, nightlyRouteExact},
		{"haskell_signature_pattern", "haskell", "a::()0=0", nightlyRouteExact, nightlyRouteOpen},
		{"haskell_infix_pattern", "haskell", "m!0000000000000()00000000A00000000000000! 000000000000=0000000000\"00000\"", nightlyRouteExact, nightlyRouteOpen},
		{"javascript_jsx_before_number", "javascript", "<A><A>AAAAA</A></A>0", nightlyRouteHasError, nightlyRouteOpen},
		{"javascript_stray_unary_operator", "javascript", ";! ;A0", nightlyRouteExact, nightlyRouteOpen},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(tc.language)
			if entry == nil {
				t.Fatalf("%s is not registered", tc.language)
			}
			lang := entry.Language()
			oracle, err := COracleLanguage(tc.language)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(oracle); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse([]byte(tc.source), nil)
			defer cTree.Close()
			t.Logf("locked C: %s has_error=%t", cTree.RootNode().ToSexp(), cTree.RootNode().HasError())
			for _, route := range []struct {
				name    string
				compact bool
				match   nightlyRouteMatch
			}{{"compact", true, tc.compact}, {"production", false, tc.production}} {
				t.Run(route.name, func(t *testing.T) {
					parser := gotreesitter.NewParser(lang)
					parser.SetAdmissionCandidateRoute(route.compact)
					tree, err := parser.Parse([]byte(tc.source))
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					t.Logf("Go: %s has_error=%t", tree.RootNode().SExpr(lang), tree.RootNode().HasError())
					switch route.match {
					case nightlyRouteExact:
						assertLockedCTreeExactWithErrors(t, tc.language+" "+route.name, tree, lang, cTree)
					case nightlyRouteHasError:
						if got, want := tree.RootNode().HasError(), cTree.RootNode().HasError(); got != want {
							t.Fatalf("%s %s HasError = %t, locked C = %t", tc.language, route.name, got, want)
						}
						logOpenLockedCDifference(t, route.name, tree, lang, cTree)
					default:
						logOpenLockedCDifference(t, route.name, tree, lang, cTree)
					}
				})
			}
		})
	}
}

// logOpenLockedCDifference logs how a route's tree differs from locked C, for a
// witness that the test does not require to match.
func logOpenLockedCDifference(t *testing.T, route string, tree *gotreesitter.Tree, lang *gotreesitter.Language, cTree *sitter.Tree) {
	t.Helper()
	if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil {
		t.Logf("open %s difference from locked C (has_error Go=%t C=%t): %+v",
			route, tree.RootNode().HasError(), cTree.RootNode().HasError(), diff)
	}
}
