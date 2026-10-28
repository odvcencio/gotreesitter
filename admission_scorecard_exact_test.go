//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

func TestAdmissionCandidateScorecardExactDenominator(t *testing.T) {
	if os.Getenv("GTS_COMPACT_G4_SCORECARD") != "1" {
		t.Skip("set GTS_COMPACT_G4_SCORECARD=1 to run the exact G4 scorecard")
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	counts := make(map[string]int)
	entries := grammars.AllLanguages()
	for _, entry := range entries {
		row := runAdmissionScorecardLanguage(entry)
		counts[row.status]++
	}
	if len(entries) != 206 || counts[scorecardPass] != 202 || counts[scorecardFallback] != 0 ||
		counts[scorecardSkip] != 4 || counts[scorecardDiverge] != 0 || counts[scorecardError] != 0 {
		t.Fatalf(
			"G4 scorecard PASS=%d FALLBACK=%d SKIP=%d DIVERGE=%d ERROR=%d total=%d; want 202/0/4/0/0/206",
			counts[scorecardPass],
			counts[scorecardFallback],
			counts[scorecardSkip],
			counts[scorecardDiverge],
			counts[scorecardError],
			len(entries),
		)
	}
}
