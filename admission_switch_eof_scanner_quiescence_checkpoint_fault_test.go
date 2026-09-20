//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// TestAdmissionCandidateEOFScannerQuiescenceCheckpointChangeDeclinesFailClosed
// reaches the "proof.proved" arm of validateCompactEOFRecoveryAdmission's
// scanner checkpoint comparison (parsercore_phase0_driver.go), which no other
// test covers: every existing scanner-checkpoint test forges an unproved
// receipt instead (parsercore_phase0_eof_scanner_proof_seal_internal_test.go).
//
// The Scala witness reaches a completed quiescence proof
// (proveCompactEOFScannerQuiescence, parsercore_phase0_eof_scanner_quiescence.go),
// then CompactEOFRecoveryAdmissionCheckpointFaultForTest rewrites the proved
// receipt's checkpoint identity right after it is produced, before the first
// validate call, and reseals it so only the live checkpoint comparison can
// reject the tamper. The route must decline and fall back to production, and
// the served tree must still equal production's, because a decline is a
// fallback, never a wrong answer.
func TestAdmissionCandidateEOFScannerQuiescenceCheckpointChangeDeclinesFailClosed(t *testing.T) {
	lang := grammars.ScalaLanguage()
	if lang.ExternalScanner == nil {
		t.Fatal("scala lost its external scanner, so this witness no longer exercises the proof")
	}
	source := []byte(scalaEOFScannerQuiescenceWitnesses[0].source)

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

	restore := gts.CompactEOFRecoveryAdmissionCheckpointFaultForTest()
	defer restore()

	gts.ResetAdmissionCandidateCountersForTest()
	candidate := gts.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer candidateTree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 0 || fallback != 1 {
		t.Fatalf("route counters = %d/%d, want 0/1 (the checkpoint tamper must decline)", routed, fallback)
	}
	reason := gts.AdmissionCandidateLastFallbackReason()
	if !strings.Contains(reason, "scanner checkpoint changed") {
		t.Fatalf("fallback reason = %q, want it to name the scanner checkpoint change", reason)
	}

	candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), lang)
	if err != nil {
		t.Fatalf("inspect candidate tree: %v", err)
	}
	if candidateInspection.SHA256 != productionInspection.SHA256 {
		t.Fatalf(
			"declined route served %s, want production's %s",
			candidateInspection.SHA256, productionInspection.SHA256,
		)
	}
}
