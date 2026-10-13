//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestNightlyFuzzRouteWitnessesLockedC adjudicates the FuzzAdmissionRouteEquality
// witnesses from the fuzz-nightly runs of 2026-10-08 to 2026-10-12 against the
// locked C oracle.
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
func TestNightlyFuzzRouteWitnessesLockedC(t *testing.T) {
	for _, tc := range []struct {
		name, language, source string
		productionMatchesC     bool
	}{
		{"go_lone_comma", "go", ",", true},
		{"go_unterminated_string", "go", "package main\n\nfunc main() {\"00000000000\n,", true},
		{"haskell_signature_pattern", "haskell", "a::()0=0", false},
		{"haskell_infix_pattern", "haskell", "m!0000000000000()00000000A00000000000000! 000000000000=0000000000\"00000\"", false},
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
			}{{"compact", true}, {"production", false}} {
				t.Run(route.name, func(t *testing.T) {
					parser := gotreesitter.NewParser(lang)
					parser.SetAdmissionCandidateRoute(route.compact)
					tree, err := parser.Parse([]byte(tc.source))
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					t.Logf("Go: %s has_error=%t", tree.RootNode().SExpr(lang), tree.RootNode().HasError())
					if !route.compact && !tc.productionMatchesC {
						if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil {
							t.Logf("open production difference from locked C: %+v", diff)
						}
						return
					}
					assertLockedCTreeExactWithErrors(t, tc.language+" "+route.name, tree, lang, cTree)
				})
			}
		})
	}
}
