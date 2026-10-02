//go:build linux && cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"encoding/json"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Check the unchanged design thresholds against the locked C frontier. Keep
// failed rows visible; serving an edit does not authorize graduation.
func TestCompactEditsFrontier(t *testing.T) {
	name := ceilingLanguage(t)
	language := grammars.DetectLanguageByName(name).Language()
	cl, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range ceilingInputs(t) {
		if input.mode != "fresh" {
			continue
		}
		t.Run(input.size, func(t *testing.T) {
			oracle := measureCliffC(t, cl, input.source[0])
			for _, candidate := range []bool{false, true} {
				route := measureCliffGo(t, language, input.source[0], candidate)
				route.MatchesC = route.TreeSHA256 == oracle.TreeSHA256
				route.Failures = benchfixtures.CliffFailures(route.Frontier, oracle.Frontier)
				receipt, err := json.Marshal(map[string]any{
					"language": name, "size": input.size, "source_bytes": len(input.source[0]), "C": oracle, "Go": route,
				})
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("FRONTIER %s", receipt)
				if !route.MatchesC || route.HasError != oracle.HasError || route.RootEnd != oracle.RootEnd {
					t.Errorf("%s fresh parse differs from C", route.Route)
				}
				for _, failure := range route.Failures {
					t.Errorf("%s: %s", route.Route, failure)
				}
			}
		})
	}
}
