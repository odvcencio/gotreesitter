//go:build cgo && treesitter_c_parity && gts_derivation_set_census && gts_eof_history_census && gts_eof_recovery_shadow

package cgoharness

import (
	"os"
	"sort"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestEOFRecoveryShadowDifferential compares one private compact recover_eof
// fold with the ordered locked-C accept set. It changes no serving route.
func TestEOFRecoveryShadowDifferential(t *testing.T) {
	previousForest := os.Getenv("GOT_GLR_FOREST") != "0"
	gotreesitter.SetGLRForestEnabled(false)
	t.Cleanup(func() { gotreesitter.SetGLRForestEnabled(previousForest) })

	for _, language := range []string{"http", "robot"} {
		language := language
		t.Run(language, func(t *testing.T) {
			source := []byte(grammars.ParseSmokeSample(language))
			entry := grammars.DetectLanguageByName(language)
			if entry == nil || entry.Language == nil || entry.Language() == nil {
				t.Fatalf("load %s Go grammar", language)
			}
			parser := gotreesitter.NewParser(entry.Language())
			parser.SetAdmissionCandidateRoute(true)
			gotreesitter.EOFAcceptHistoryCensusReset()
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("parse compact route: %v", err)
			}
			tree.Release()

			frontiers := gotreesitter.EOFAcceptHistoryCensusSnapshot()
			if len(frontiers) != 1 || len(frontiers[0].Heads) != 2 {
				t.Fatalf("compact EOF frontier=%d heads=%d, want 1/2", len(frontiers), eofShadowHeadCount(frontiers))
			}
			var accepting *gotreesitter.EOFAcceptHistoryHead
			var noAction *gotreesitter.EOFAcceptHistoryHead
			for index := range frontiers[0].Heads {
				head := &frontiers[0].Heads[index]
				switch {
				case head.Accepting:
					accepting = head
				case head.NoAction:
					noAction = head
				}
			}
			if accepting == nil || noAction == nil || len(accepting.Candidates) != 1 || len(noAction.Candidates) != 1 {
				t.Fatalf("compact EOF roles or exact derivations are incomplete")
			}
			shadow := noAction.RecoveryShadow
			if shadow == nil || shadow.Error != "" {
				t.Fatalf("private EOF recovery receipt=%+v", shadow)
			}
			if shadow.Kind != "recover_eof" || shadow.AcceptIndex != 1 {
				t.Fatalf("private EOF recovery event=%s[%d], want recover_eof[1]", shadow.Kind, shadow.AcceptIndex)
			}
			if shadow.Steps != 1 || shadow.MaxSteps != 1 || shadow.Payloads == 0 || shadow.Payloads > shadow.MaxPayloads {
				t.Fatalf("private EOF recovery bounds=%+v", *shadow)
			}
			if shadow.SourceFootprintBytes > shadow.MaxCloneBytes || !shadow.MutableStorageDisjoint || !shadow.LiveStateUnchanged {
				t.Fatalf("private EOF recovery isolation=%+v", *shadow)
			}
			if shadow.SubtreesAfter != shadow.SubtreesBefore+1 ||
				shadow.ChildrenAfter != shadow.ChildrenBefore+shadow.Payloads ||
				!shadow.ExistingArenaPreserved || !shadow.RootChildrenExact {
				t.Fatalf("private EOF recovery arena delta=%+v", *shadow)
			}
			assertEOFShadowWork(t, shadow)
			if shadow.StartByte != 0 || shadow.EndByte != uint32(len(source)) {
				t.Fatalf("private EOF recovery span=%d..%d, want 0..%d", shadow.StartByte, shadow.EndByte, len(source))
			}
			if shadow.RootSymbol != 65535 || !shadow.RootNamed || shadow.RootExtra || shadow.RootMissing ||
				!shadow.RootIsError || !shadow.RootHasError || shadow.RootDynamicPrecedence != 0 {
				t.Fatalf("private EOF recovery root metadata=%+v", *shadow)
			}
			if !strings.HasPrefix(shadow.RootShape, "(ERROR[") {
				t.Fatalf("private EOF recovery root=%s", shadow.RootShape)
			}

			noActionPayload, err := eofHistoryRootChildrenShape(noAction.Candidates[0].Shape)
			if err != nil {
				t.Fatalf("decode compact no-action payload: %v", err)
			}
			shadowPayload, err := eofHistoryRootChildrenShape(shadow.RootShape)
			if err != nil {
				t.Fatalf("decode compact recovery payload: %v", err)
			}
			if shadowPayload != noActionPayload {
				t.Fatalf("private recovery changed the pre-recovery payload: before=%s after=%s", noActionPayload, shadowPayload)
			}

			cLanguage, err := COracleLanguage(language)
			if err != nil {
				t.Fatalf("load %s locked C grammar: %v", language, err)
			}
			cEvents, err := cReconstructVersionSet(cLanguage, source)
			if err != nil {
				t.Fatalf("reconstruct locked-C versions: %v", err)
			}
			if len(cEvents.Accepts) != 2 || cEvents.Accepts[0].RecoverEOF || len(cEvents.Accepts[0].Folds) != 0 ||
				!cEvents.Accepts[1].RecoverEOF || len(cEvents.Accepts[1].Folds) != 1 {
				t.Fatalf("locked-C ordered accepts=%+v", cEvents.Accepts)
			}
			cHistory := runEOFAcceptHistoryCOracle(t, language, source)
			if len(cHistory.Versions) != 2 || cHistory.Versions[0].AcceptIndex != 0 || cHistory.Versions[1].AcceptIndex != 1 {
				t.Fatalf("locked-C ordered roots=%+v", cHistory.Versions)
			}

			compactOrdered := []string{accepting.Candidates[0].Shape, shadow.RootShape}
			cOrdered := []string{cHistory.Versions[0].Shape, cHistory.Versions[1].Shape}
			for index := range compactOrdered {
				if compactOrdered[index] != cOrdered[index] {
					t.Fatalf("ordered accept %d differs: compact=%s C=%s", index, compactOrdered[index], cOrdered[index])
				}
			}
			compactMultiset := append([]string(nil), compactOrdered...)
			cMultiset := append([]string(nil), cOrdered...)
			sort.Strings(compactMultiset)
			sort.Strings(cMultiset)
			if !equalStrings(compactMultiset, cMultiset) {
				t.Fatalf("complete root multiset differs: compact=%v C=%v", compactMultiset, cMultiset)
			}
			if shadow.ErrorCost != cHistory.Versions[1].ErrorCost {
				t.Fatalf("recover_eof error cost=%d, want C %d", shadow.ErrorCost, cHistory.Versions[1].ErrorCost)
			}
			t.Logf(
				"G3 BIJECTION language=%s status=PASS ordered=normal[0]/recover_eof[1] error-cost=%d steps=%d/%d payloads=%d/%d footprint=%d/%d shadow-sha256=%x",
				language, shadow.ErrorCost, shadow.Steps, shadow.MaxSteps,
				shadow.Payloads, shadow.MaxPayloads, shadow.SourceFootprintBytes,
				shadow.MaxCloneBytes, shadow.DeepSHA256,
			)
		})
	}
}

func eofShadowHeadCount(frontiers []gotreesitter.EOFAcceptHistoryFrontier) int {
	if len(frontiers) == 0 {
		return 0
	}
	return len(frontiers[0].Heads)
}

func assertEOFShadowWork(t *testing.T, shadow *gotreesitter.EOFRecoveryShadowReceipt) {
	t.Helper()
	before := shadow.WorkBefore
	after := shadow.WorkAfter
	if after.ParentConstructionsProxy != before.ParentConstructionsProxy+1 {
		t.Fatalf("private EOF recovery parent work=%d..%d, want +1", before.ParentConstructionsProxy, after.ParentConstructionsProxy)
	}
	after.ParentConstructionsProxy--
	if after != before || shadow.WorkAfter.Overflow {
		t.Fatalf("private EOF recovery changed unexpected work: before=%+v after=%+v", before, shadow.WorkAfter)
	}
}
