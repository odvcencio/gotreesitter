//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// TestCompactEOFScannerQuiescenceAdmitsOnlyZeroWidthExternalTerminals pins the
// one payload shape the admission path walk accepts under a completed
// quiescence proof. Every other extra or external payload must stay rejected.
func TestCompactEOFScannerQuiescenceAdmitsOnlyZeroWidthExternalTerminals(t *testing.T) {
	base := core.EOFAdmissionSubtreeView{
		Symbol:    140,
		StartByte: 43,
		EndByte:   43,
		External:  true,
		Terminal:  true,
	}
	if !compactEOFScannerQuiescenceExternalAdmitted(base) {
		t.Fatal("the zero-width external terminal was not admitted")
	}

	tests := []struct {
		name   string
		mutate func(view *core.EOFAdmissionSubtreeView)
	}{
		{"positive width", func(v *core.EOFAdmissionSubtreeView) { v.EndByte = v.StartByte + 1 }},
		{"not external", func(v *core.EOFAdmissionSubtreeView) { v.External = false }},
		{"not terminal", func(v *core.EOFAdmissionSubtreeView) { v.Terminal = false }},
		{"extra", func(v *core.EOFAdmissionSubtreeView) { v.Extra = true }},
		{"missing", func(v *core.EOFAdmissionSubtreeView) { v.Missing = true }},
		{"has children", func(v *core.EOFAdmissionSubtreeView) { v.Children = []core.SubtreeID{7} }},
		{"has fields", func(v *core.EOFAdmissionSubtreeView) { v.Fields = []core.FieldMapEntry{{}} }},
		{"has aliases", func(v *core.EOFAdmissionSubtreeView) { v.Aliases = []core.Symbol{3} }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			view := base
			test.mutate(&view)
			if compactEOFScannerQuiescenceExternalAdmitted(view) {
				t.Fatalf("%s was admitted, want rejected", test.name)
			}
		})
	}
}

// TestCompactEOFScannerQuiescenceDeclinesKeepTheirFamilyPrefix keeps every
// decline reason inside one grep-able family. admissionCensusClassify
// (admission_census.go) maps that family onto
// censusMechanismEOFScannerQuiescence, so a reason that drops the prefix
// silently falls back into the scheduler-shape catch-all.
func TestCompactEOFScannerQuiescenceDeclinesKeepTheirFamilyPrefix(t *testing.T) {
	declines := []string{
		compactEOFScannerQuiescenceDeclineContext,
		compactEOFScannerQuiescenceDeclineNoScanner,
		compactEOFScannerQuiescenceDeclineElectionChanged,
		compactEOFScannerQuiescenceDeclineHeaderCheckpoint,
		compactEOFScannerQuiescenceDeclineElectionStates,
		compactEOFScannerQuiescenceDeclineContract,
		compactEOFScannerQuiescenceDeclineSnapshot,
		compactEOFScannerQuiescenceDeclineIdentity,
		compactEOFScannerQuiescenceDeclinePayload,
		compactEOFScannerQuiescenceDeclineLexMode,
		compactEOFScannerQuiescenceDeclineStateToken,
		compactEOFScannerQuiescenceDeclineStatePayload,
		compactEOFScannerQuiescenceDeclineWidth,
		compactEOFScannerQuiescenceDeclineStateOffer,
		compactEOFScannerQuiescenceDeclineWork,
	}
	seen := make(map[string]struct{}, len(declines))
	for _, decline := range declines {
		if !strings.HasPrefix(decline, compactEOFScannerQuiescencePrefix) {
			t.Errorf("decline %q lost the family prefix", decline)
		}
		if decline == compactEOFScannerQuiescencePrefix {
			t.Errorf("decline %q names no failing step", decline)
		}
		if _, repeated := seen[decline]; repeated {
			t.Errorf("decline %q is duplicated", decline)
		}
		seen[decline] = struct{}{}
		if got := admissionCensusClassify(DiagnosticParserCoreAccept, decline); got != censusMechanismEOFScannerQuiescence {
			t.Errorf("decline %q classified as %q, want %q", decline, got, censusMechanismEOFScannerQuiescence)
		}
	}
}

// TestCompactEOFScannerQuiescenceDeclinesWithoutAProbeContext keeps the prover
// fail-closed when it has nothing to measure. A nil scheduler, a nil core, and
// a language with external tokens but no scanner must all decline.
func TestCompactEOFScannerQuiescenceDeclinesWithoutAProbeContext(t *testing.T) {
	var receipt compactEOFRecoveryAdmissionReceipt
	var nilScheduler *diagnosticParserCoreGenericScheduler
	if proof, decline := nilScheduler.proveCompactEOFScannerQuiescence(&receipt, &Language{}, 0); proof != (compactEOFScannerQuiescenceProof{}) ||
		decline != compactEOFScannerQuiescenceDeclineContext {
		t.Fatalf("nil scheduler proof=%+v decline=%q", proof, decline)
	}

	empty := &diagnosticParserCoreGenericScheduler{}
	if proof, decline := empty.proveCompactEOFScannerQuiescence(&receipt, &Language{}, 0); proof != (compactEOFScannerQuiescenceProof{}) ||
		decline != compactEOFScannerQuiescenceDeclineContext {
		t.Fatalf("empty scheduler proof=%+v decline=%q", proof, decline)
	}

	if proof, decline := empty.proveCompactEOFScannerQuiescence(&receipt, nil, 0); proof != (compactEOFScannerQuiescenceProof{}) ||
		decline != compactEOFScannerQuiescenceDeclineContext {
		t.Fatalf("nil language proof=%+v decline=%q", proof, decline)
	}

	// A nil receipt has nowhere to account the probe work, so it declines too.
	if proof, decline := empty.proveCompactEOFScannerQuiescence(nil, &Language{}, 0); proof != (compactEOFScannerQuiescenceProof{}) ||
		decline != compactEOFScannerQuiescenceDeclineContext {
		t.Fatalf("nil receipt proof=%+v decline=%q", proof, decline)
	}
}

// CompactEOFScannerQuiescenceProbeFaultForTest installs one probe fault and
// returns a restore func. Each argument rewrites one measurement the per-state
// probe just took, so a test can reach a decline reason on a real two-head
// frontier. A nil argument leaves that measurement untouched.
func CompactEOFScannerQuiescenceProbeFaultForTest(
	token func(StateID, Token) Token,
	offered func(StateID, uint32) uint32,
	payload func(StateID, bool) bool,
) func() {
	previous := compactEOFScannerQuiescenceProbeFaultHook
	compactEOFScannerQuiescenceProbeFaultHook = &compactEOFScannerQuiescenceProbeFaults{
		token: token, offered: offered, payload: payload,
	}
	return func() { compactEOFScannerQuiescenceProbeFaultHook = previous }
}

// CompactEOFScannerQuiescenceDeclineReasonsForTest exposes the per-state
// decline reasons so an external test can pin the exact text a fault produces.
func CompactEOFScannerQuiescenceDeclineReasonsForTest() (token, offer, payload string) {
	return compactEOFScannerQuiescenceDeclineStateToken,
		compactEOFScannerQuiescenceDeclineStateOffer,
		compactEOFScannerQuiescenceDeclineStatePayload
}

// CompactEOFScannerQuiescenceLastDeclineForTest returns the reason recorded
// while a probe fault was installed, and clears it. It reads the prover
// directly, so a fault test does not depend on the GTS_ADMISSION_CENSUS
// opt-in, whose cached read an earlier test in the same process may already
// have resolved.
func CompactEOFScannerQuiescenceLastDeclineForTest() string {
	reason := compactEOFScannerQuiescenceLastDecline
	compactEOFScannerQuiescenceLastDecline = ""
	return reason
}

// CompactEOFRecoveryAdmissionCheckpointFaultForTest installs a fault hook that
// rewrites a genuinely proved receipt's scanner checkpoint identity right
// after produceCompactEOFRecoveryAdmission runs and before the first validate
// call reads it (parsercore_phase0_driver.go,
// applyCompactEOFRecoveryAdmission calls compactEOFRecoveryAdmissionFault
// exactly there). It fires once, on the first proved receipt it sees, and
// reseals the receipt the same way TestCompactEOFRecoveryScannerProofTamperingDeclines
// reseals a forged proof (parsercore_phase0_eof_scanner_proof_seal_internal_test.go),
// so only the live checkpoint comparison in validateCompactEOFRecoveryAdmission
// can reject the tamper. It exercises the "proof.proved" arm of that
// comparison, which no other test reaches: every other scanner-checkpoint
// test forges an unproved receipt instead. The returned func restores the
// previous hook.
func CompactEOFRecoveryAdmissionCheckpointFaultForTest() func() {
	previous := compactEOFRecoveryAdmissionFaultHook
	fired := false
	compactEOFRecoveryAdmissionFaultHook = func(s *diagnosticParserCoreGenericScheduler, stage string) error {
		if fired || stage != "after_produce" || s == nil || !s.eofRecoveryAdmission.scannerQuiescence.proved {
			return nil
		}
		fired = true
		receipt := &s.eofRecoveryAdmission
		receipt.scannerQuiescence.checkpointBefore++
		var previousSeal [32]byte
		if receipt.transitionCount > 1 {
			previousSeal = receipt.transitionSeals[receipt.transitionCount-2]
		}
		receipt.seal = compactEOFRecoveryAdmissionSeal(receipt, previousSeal)
		receipt.transitionSeals[receipt.transitionCount-1] = receipt.seal
		return nil
	}
	return func() { compactEOFRecoveryAdmissionFaultHook = previous }
}
