//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestAdmissionCandidateCertifiedAcceptanceFrontiers(t *testing.T) {
	// meson is not in this list: its smoke sample has a genuine, score-tied
	// grammar ambiguity (variableunit vs var_unit) that
	// selectCompactAcceptanceDerivation's materiality gate
	// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous) can
	// no longer prove correct without a C oracle, so the route now declines
	// and falls back to production instead of publishing an unproven pick.
	// See admission_scorecard_test.go's admissionScorecardRequiredCompactPasses
	// comment for the full account.
	for _, name := range []string{"http", "robot"} {
		t.Run(name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(name)
			if entry == nil {
				t.Fatal("language entry is missing")
			}
			lang := entry.Language()
			source := []byte(grammars.ParseSmokeSample(name))

			production := gts.NewParser(lang)
			production.SetAdmissionCandidateRoute(false)
			productionTree, err := production.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()
			productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), lang)
			if err != nil {
				t.Fatalf("inspect production tree: %v", err)
			}

			gts.ResetAdmissionCandidateCountersForTest()
			candidate := gts.NewParser(lang)
			candidate.SetAdmissionCandidateRoute(true)
			candidateTree, err := candidate.Parse(source)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			defer candidateTree.Release()

			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != 1 || fallback != 0 {
				t.Fatalf(
					"route counters routed=%d fallback=%d reason=%q",
					routed,
					fallback,
					gts.AdmissionCandidateLastFallbackReason(),
				)
			}
			candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), lang)
			if err != nil {
				t.Fatalf("inspect candidate tree: %v", err)
			}
			if candidateInspection.SHA256 != productionInspection.SHA256 {
				t.Fatalf(
					"tree digest candidate=%s production=%s",
					candidateInspection.SHA256,
					productionInspection.SHA256,
				)
			}
		})
	}
}
