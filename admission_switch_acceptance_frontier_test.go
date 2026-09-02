//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestAdmissionCandidateCertifiedAcceptanceFrontiers(t *testing.T) {
	for _, name := range []string{"http", "meson", "robot"} {
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

func TestMesonStructuralAcceptanceElectionFailsClosedWithoutArtifactCapability(t *testing.T) {
	language := *grammars.MesonLanguage()
	language.CompactAcceptanceStructuralElectionCertified = false
	source := []byte(grammars.ParseSmokeSample("meson"))
	t.Setenv("GTS_ADMISSION_CENSUS", "1")
	gts.ResetAdmissionCensusEnabledForTest()
	t.Cleanup(gts.ResetAdmissionCensusEnabledForTest)

	gts.ResetAdmissionCandidateCountersForTest()
	parser := gts.NewParser(&language)
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	if routed, fallback := gts.AdmissionCandidateCounters(); routed != 0 || fallback != 1 {
		t.Fatalf("route counters=%d/%d, want 0/1", routed, fallback)
	}
	if reason := gts.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "material-acceptance-election") {
		t.Fatalf("fallback reason=%q, want material acceptance election", reason)
	}
}

func TestMesonStructuralAcceptanceElectionDoesNotDependOnPrimaryCapability(t *testing.T) {
	language := *grammars.MesonLanguage()
	language.CompactPrimaryAcceptanceDerivationCertified = false
	source := []byte(grammars.ParseSmokeSample("meson"))

	gts.ResetAdmissionCandidateCountersForTest()
	parser := gts.NewParser(&language)
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	if routed, fallback := gts.AdmissionCandidateCounters(); routed != 1 || fallback != 0 {
		t.Fatalf("route counters=%d/%d, want 1/0", routed, fallback)
	}
	if got := tree.RootNode().SExpr(&language); got != "(source_file (normal_command (identifier) (variableunit (string))))" {
		t.Fatalf("tree=%s, want the C-ordered variableunit branch", got)
	}
}
