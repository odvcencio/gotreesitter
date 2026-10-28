//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

var admissionTokenSourceShadowLanguages = map[string]struct{}{
	"authzed": {},
	"c":       {},
	"cpp":     {},
	"java":    {},
}

func TestAdmissionCandidateTokenSourceEntriesExposeDFASmoke(t *testing.T) {
	wanted := make(map[string]struct{}, len(admissionTokenSourceShadowLanguages))
	for name := range admissionTokenSourceShadowLanguages {
		wanted[name] = struct{}{}
	}
	for _, entry := range grammars.AllLanguages() {
		if _, ok := wanted[entry.Name]; !ok {
			continue
		}
		entry := entry
		t.Run(entry.Name, func(t *testing.T) {
			lang := entry.Language()
			support := grammars.EvaluateParseSupport(entry, lang)
			if support.Backend != grammars.ParseBackendTokenSource {
				t.Fatalf("preferred backend = %q, want token_source", support.Backend)
			}
			if !support.HasDFALexer {
				t.Fatal("embedded language has no DFA lexer")
			}
			if support.RequiresExternalScanner && !support.HasExternalScanner {
				t.Fatal("embedded language requires an external scanner, but none is attached")
			}

			// This copy tests latent DFA capability. It does not change the
			// preferred production backend or the scorecard SKIP count.
			dfaEntry := entry
			dfaEntry.TokenSourceFactory = nil
			row := runAdmissionScorecardLanguage(dfaEntry)
			if row.status != scorecardPass {
				t.Fatalf("DFA compact smoke = %s: %s", row.status, row.detail)
			}
		})
		delete(wanted, entry.Name)
	}
	for name := range wanted {
		t.Errorf("missing registry entry %q", name)
	}
}

func TestAdmissionCandidateDFASmokeMatchesPreferredTokenSource(t *testing.T) {
	wanted := make(map[string]struct{}, len(admissionTokenSourceShadowLanguages))
	for name := range admissionTokenSourceShadowLanguages {
		wanted[name] = struct{}{}
	}
	for _, entry := range grammars.AllLanguages() {
		if _, ok := wanted[entry.Name]; !ok {
			continue
		}
		entry := entry
		t.Run(entry.Name, func(t *testing.T) {
			lang := entry.Language()
			source := []byte(grammars.ParseSmokeSample(entry.Name))

			tokenTree, err := gts.NewParser(lang).ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
			if err != nil {
				t.Fatalf("token-source parse: %v", err)
			}
			defer tokenTree.Release()
			tokenInspection, err := benchfixtures.InspectGoTree(tokenTree.RootNode(), lang)
			if err != nil {
				t.Fatalf("token-source digest: %v", err)
			}

			candidate := gts.NewParser(lang)
			candidate.SetAdmissionCandidateRoute(true)
			gts.ResetAdmissionCandidateCountersForTest()
			candidateTree, err := candidate.Parse(source)
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			defer candidateTree.Release()
			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != 1 || fallback != 0 {
				t.Fatalf("compact route counters = routed %d, fallback %d", routed, fallback)
			}
			candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), lang)
			if err != nil {
				t.Fatalf("compact digest: %v", err)
			}
			if candidateInspection.SHA256 != tokenInspection.SHA256 {
				first := firstAdmissionTreeDivergence(candidateTree.RootNode(), tokenTree.RootNode(), lang, "root")
				t.Fatalf("compact tree differs from preferred token-source tree: %s", first)
			}
		})
		delete(wanted, entry.Name)
	}
	for name := range wanted {
		t.Errorf("missing registry entry %q", name)
	}
}
