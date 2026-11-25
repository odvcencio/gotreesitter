//go:build !gts_no_parsercorephase0

package admission_test

import (
	"os"
	"reflect"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func cloneAdmissionTestLanguage(language *gts.Language) *gts.Language {
	source := reflect.ValueOf(language).Elem()
	clone := reflect.New(source.Type()).Elem()
	for i := 0; i < source.NumField(); i++ {
		if source.Type().Field(i).IsExported() {
			clone.Field(i).Set(source.Field(i))
		}
	}
	return clone.Addr().Interface().(*gts.Language)
}

func TestAdmissionCandidateRoutesEOFSiblingsWithoutProfileGrant(t *testing.T) {
	previousForest := os.Getenv("GOT_GLR_FOREST") != "0"
	gts.SetGLRForestEnabled(false)
	t.Cleanup(func() { gts.SetGLRForestEnabled(previousForest) })

	for _, name := range []string{"http", "robot"} {
		name := name
		t.Run(name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(name)
			if entry == nil || entry.Language == nil {
				t.Fatalf("load %s grammar", name)
			}
			loaded := entry.Language()
			if loaded == nil {
				t.Fatalf("load %s grammar", name)
			}
			if loaded.ExternalScanner != nil || loaded.ExternalTokenCount != 0 {
				t.Fatalf(
					"%s scanner state is not quiescent: scanner=%T tokens=%d",
					name,
					loaded.ExternalScanner,
					loaded.ExternalTokenCount,
				)
			}
			if loaded.CompactEOFAcceptNoActionSiblingsCertified {
				t.Errorf("loaded profile still sets the legacy EOF sibling bypass")
			}

			productionLanguage := cloneAdmissionTestLanguage(loaded)
			productionLanguage.CompactEOFAcceptNoActionSiblingsCertified = false
			production := gts.NewParser(productionLanguage)
			production.SetAdmissionCandidateRoute(false)
			source := []byte(grammars.ParseSmokeSample(name))
			productionTree, err := production.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()
			productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), productionLanguage)
			if err != nil {
				t.Fatalf("inspect production tree: %v", err)
			}

			legacyLanguage := cloneAdmissionTestLanguage(loaded)
			legacyLanguage.CompactEOFAcceptNoActionSiblingsCertified = true
			gts.ResetAdmissionCandidateCounters()
			legacy := gts.NewParser(legacyLanguage)
			legacy.SetAdmissionCandidateRoute(true)
			legacyTree, err := legacy.Parse(source)
			if err != nil {
				t.Fatalf("explicit legacy-bypass parse: %v", err)
			}
			defer legacyTree.Release()
			legacyRouted, legacyFallback := gts.AdmissionCandidateCounters()
			if legacyRouted != 1 || legacyFallback != 0 {
				t.Fatalf(
					"legacy route counters routed=%d fallback=%d reason=%q",
					legacyRouted,
					legacyFallback,
					gts.AdmissionCandidateLastFallbackReason(),
				)
			}
			legacyInspection, err := benchfixtures.InspectGoTree(legacyTree.RootNode(), legacyLanguage)
			if err != nil {
				t.Fatalf("inspect legacy tree: %v", err)
			}
			if !reflect.DeepEqual(legacyInspection, productionInspection) {
				t.Fatalf(
					"legacy tree inspection differs: candidate=%+v production=%+v",
					legacyInspection,
					productionInspection,
				)
			}

			candidateLanguage := cloneAdmissionTestLanguage(loaded)
			candidateLanguage.CompactEOFAcceptNoActionSiblingsCertified = false
			gts.ResetAdmissionCandidateCounters()
			candidate := gts.NewParser(candidateLanguage)
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
			candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), candidateLanguage)
			if err != nil {
				t.Fatalf("inspect candidate tree: %v", err)
			}
			if !reflect.DeepEqual(candidateInspection, productionInspection) {
				t.Fatalf(
					"tree inspection differs: candidate=%+v production=%+v",
					candidateInspection,
					productionInspection,
				)
			}
		})
	}
}
