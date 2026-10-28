//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// TestPerlPushFatCommaKeepsDispatcherArm pins why the Perl result-compatibility
// arm is still live. When the first argument of push is followed by a fat comma
// (=>), the engine's own tree leaves the arguments ungrouped, and the arm
// regroups them the way locked C does. The comma form (push @a, $b) never
// reaches the arm. Retire the arm only after these inputs match without it.
// The locked-C comparison is
// cgo_harness/perl_push_fat_comma_live_arm_parity_test.go.
func TestPerlPushFatCommaKeepsDispatcherArm(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")

	language := PerlLanguage()
	for _, source := range []string{
		"push @a => 1;\n",
		"push @a => {x=>1};\n",
		"push @a => $b, $c;\n",
	} {
		source := source
		t.Run(source, func(t *testing.T) {
			raw, err := gotreesitter.NewParser(language).ParseNoResultCompatibilityBenchmarkOnly([]byte(source))
			if err != nil {
				t.Fatalf("raw parse failed: %v", err)
			}
			t.Cleanup(raw.Release)
			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse([]byte(source))
			if err != nil {
				t.Fatalf("production parse failed: %v", err)
			}
			t.Cleanup(production.Release)

			rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}
			productionInspection, err := benchfixtures.InspectGoTree(production.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}
			if rawInspection.SHA256 == productionInspection.SHA256 {
				t.Fatalf("raw and production trees are equal (%s); the Perl arm no longer changes this input, so re-run the locked-C comparison before retiring it", rawInspection.SHA256)
			}
			var rewritten uint64
			if passes := production.ParseRuntime().NormalizationPasses; passes != nil {
				for _, pass := range *passes {
					if pass.Name == "dispatch.perl" {
						rewritten += pass.NodesRewritten
					}
				}
			}
			if rewritten == 0 {
				t.Fatal("the dispatch.perl pass rewrote no nodes; the Perl arm did not fire")
			}
			t.Logf("dispatch.perl nodes_rewritten=%d", rewritten)
		})
	}
}
