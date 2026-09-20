//go:build cgo && treesitter_c_parity && gts_derivation_set_census && gts_eof_history_census && gts_eof_recovery_shadow && gts_eof_recovery_admission_contract

package cgoharness

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestEOFRecoveryAdmissionUsesLockedCEventsAndPublishedTree(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	if !gotreesitter.EOFRecoveryAdmissionCensusBuilt() {
		t.Fatal("EOF recovery admission receipt support is absent")
	}
	previousForest := os.Getenv("GOT_GLR_FOREST") != "0"
	gotreesitter.SetGLRForestEnabled(false)
	t.Cleanup(func() { gotreesitter.SetGLRForestEnabled(previousForest) })

	want := map[string]struct {
		frontier uint32
		cost     uint32
	}{
		"http":  {frontier: 1, cost: 640},
		"robot": {frontier: 4, cost: 1027},
	}
	for _, languageName := range []string{"http", "robot"} {
		languageName := languageName
		t.Run(languageName, func(t *testing.T) {
			source := []byte(grammars.ParseSmokeSample(languageName))
			cLanguage, err := COracleLanguage(languageName)
			if err != nil {
				t.Fatalf("load %s locked C grammar: %v", languageName, err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatalf("set %s locked C grammar: %v", languageName, err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatalf("%s locked C parser returned a nil tree", languageName)
			}
			defer cTree.Close()

			cEvents, err := cReconstructVersionSet(cLanguage, source)
			if err != nil {
				t.Fatalf("reconstruct %s locked-C accepts: %v", languageName, err)
			}
			if len(cEvents.Accepts) != 2 || cEvents.Accepts[0].RecoverEOF ||
				len(cEvents.Accepts[0].Folds) != 0 || !cEvents.Accepts[1].RecoverEOF ||
				len(cEvents.Accepts[1].Folds) != 1 {
				t.Fatalf("%s locked-C accept topology=%+v", languageName, cEvents.Accepts)
			}
			cHistory := runEOFAcceptHistoryCOracle(t, languageName, source)
			if len(cHistory.Versions) != 2 {
				t.Fatalf("%s locked-C roots=%d, want 2", languageName, len(cHistory.Versions))
			}
			var rootsByEvent [2]eofHistoryCVersion
			var seen [2]bool
			for _, version := range cHistory.Versions {
				if version.AcceptIndex < 0 || version.AcceptIndex >= len(rootsByEvent) {
					t.Fatalf("%s locked-C accept index=%d", languageName, version.AcceptIndex)
				}
				rootsByEvent[version.AcceptIndex] = version
				seen[version.AcceptIndex] = true
			}
			if !seen[0] || !seen[1] || rootsByEvent[0].Shape != cHistory.Published ||
				rootsByEvent[0].ErrorCost >= rootsByEvent[1].ErrorCost {
				t.Fatalf("%s locked-C ordered cost election=%+v published=%s", languageName, rootsByEvent, cHistory.Published)
			}

			entry := grammars.DetectLanguageByName(languageName)
			if entry == nil || entry.Language == nil || entry.Language() == nil {
				t.Fatalf("load %s Go grammar", languageName)
			}
			goLanguage := entry.Language()
			if goLanguage.CompactEOFAcceptNoActionSiblingsCertified {
				t.Error("loaded profile still sets the legacy EOF sibling bypass")
			}
			candidateLanguage := *goLanguage
			candidateLanguage.CompactEOFAcceptNoActionSiblingsCertified = false
			gotreesitter.EOFRecoveryAdmissionCensusReset()
			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			goParser := gotreesitter.NewParser(&candidateLanguage)
			goParser.SetAdmissionCandidateRoute(true)
			goTree, err := goParser.Parse(source)
			if err != nil {
				t.Fatalf("parse %s admission route: %v", languageName, err)
			}
			defer goTree.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if routedAfter-routedBefore != 1 || fallbackAfter-fallbackBefore != 0 {
				t.Fatalf(
					"%s admission counter delta routed=%d fallback=%d reason=%q",
					languageName,
					routedAfter-routedBefore,
					fallbackAfter-fallbackBefore,
					gotreesitter.AdmissionCandidateLastFallbackReason(),
				)
			}
			assertLockedCTreeExact(t, languageName+" EOF admission", goTree, &candidateLanguage, cTree)

			receipts := gotreesitter.EOFRecoveryAdmissionCensusSnapshot()
			if len(receipts) != 1 {
				t.Fatalf("%s admission receipts=%d, want 1", languageName, len(receipts))
			}
			receipt := receipts[0]
			expected := want[languageName]
			if receipt.State != "consumed" || receipt.ConstructionRoute != "public_tree" ||
				receipt.ConsumptionCount != 1 || receipt.CoreGeneration == 0 ||
				receipt.SourceLength != uint32(len(source)) || len(receipt.Events) != 2 ||
				receipt.Events[0].Ordinal != 0 || receipt.Events[0].Kind != "normal" ||
				receipt.Events[1].Ordinal != 1 || receipt.Events[1].Kind != "recover_eof" ||
				receipt.Events[0].Cost != rootsByEvent[0].ErrorCost ||
				receipt.Events[1].Cost != rootsByEvent[1].ErrorCost ||
				receipt.Events[0].Cost != 0 || receipt.Events[1].Cost != expected.cost ||
				receipt.NormalFrontier != 1 || receipt.RecoveryFrontier != expected.frontier ||
				receipt.SelectedEvent != 0 || !receipt.MetadataOnly ||
				receipt.Work.BytesInspected != uint64(len(source)) ||
				receipt.Work.MaxDepth > 256 || receipt.Work.PayloadRecordsVisited > 65536 ||
				receipt.Work.MaxChildGroup > 64 || receipt.Work.MaxSourceChunk > 4096 ||
				receipt.Work.Overflow || receipt.Work.PublicationAttempts != 0 {
				t.Fatalf("%s admission receipt does not match locked C: %+v", languageName, receipt)
			}
			t.Logf("%s option-A receipt: %+v", languageName, receipt)
		})
	}
}
