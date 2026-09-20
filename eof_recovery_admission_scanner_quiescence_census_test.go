//go:build !gts_no_parsercorephase0 && gts_eof_recovery_admission_contract

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestEOFRecoveryAdmissionCensusRecordsScannerQuiescenceMechanism reads the
// consumed admission receipt for the Scala smoke sample and pins the census
// mechanism tag the scanner quiescence route publishes.
//
// The tag separates the two admission routes that now share this receipt: a
// scanner-free language, whose internal lexer needs no proof, and a
// scanner-owning language admitted by proveCompactEOFScannerQuiescence
// (parsercore_phase0_eof_scanner_quiescence.go). An operator reading the
// census can size each route without re-deriving the language set.
func TestEOFRecoveryAdmissionCensusRecordsScannerQuiescenceMechanism(t *testing.T) {
	if !gts.EOFRecoveryAdmissionCensusBuilt() {
		t.Skip("build with -tags gts_eof_recovery_admission_contract")
	}
	lang := grammars.ScalaLanguage()
	if lang.ExternalScanner == nil {
		t.Fatal("scala lost its external scanner, so this witness no longer exercises the proof")
	}
	source := []byte("object Main { def f(x: Int): Int = x + 1 }\n")

	gts.EOFRecoveryAdmissionCensusReset()
	t.Cleanup(gts.EOFRecoveryAdmissionCensusReset)
	gts.ResetAdmissionCandidateCountersForTest()

	candidate := gts.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	tree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer tree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf(
			"candidate route counters = %d/%d, want 1/0; reason=%s",
			routed, fallback, gts.AdmissionCandidateLastFallbackReason(),
		)
	}

	receipts := gts.EOFRecoveryAdmissionCensusSnapshot()
	if len(receipts) != 1 {
		t.Fatalf("census recorded %d receipts, want 1", len(receipts))
	}
	receipt := receipts[0]
	if receipt.Mechanism != gts.EOFRecoveryAdmissionMechanismScannerQuiescent {
		t.Errorf("mechanism = %q, want %q", receipt.Mechanism, gts.EOFRecoveryAdmissionMechanismScannerQuiescent)
	}
	if !receipt.ScannerQuiescenceProved {
		t.Error("receipt does not carry a completed quiescence proof")
	}
	if receipt.ScannerQuiescenceStates != 2 {
		t.Errorf("probed states = %d, want 2 (one accepting head, one no-action head)", receipt.ScannerQuiescenceStates)
	}
	if receipt.ExternalCount != 1 {
		t.Errorf("shared zero-width external count = %d, want 1 (the trailing _automatic_semicolon)", receipt.ExternalCount)
	}
	if receipt.ExternalDigest == ([32]byte{}) {
		t.Error("shared zero-width external digest is empty")
	}
	if receipt.SelectedEvent != 0 {
		t.Errorf("selected event = %d, want 0 (the accepting head)", receipt.SelectedEvent)
	}
	if receipt.Events[0].Cost >= receipt.Events[1].Cost {
		t.Errorf(
			"accepting cost %d is not below the recovery cost %d, so C's error-cost rule did not decide this fork",
			receipt.Events[0].Cost, receipt.Events[1].Cost,
		)
	}
}
