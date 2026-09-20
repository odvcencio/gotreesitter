//go:build !gts_no_parsercorephase0 && gts_eof_recovery_admission_contract

package gotreesitter

import (
	"strings"
	"testing"
)

// TestCompactEOFRecoveryScannerProofTamperingDeclines pins the two independent
// defences over the scanner quiescence proof carried on an admission receipt.
//
// The fixture language owns no external scanner, so its receipt must carry the
// zero proof. Each case forges a proved proof onto it. The first case leaves
// the seal alone, so the seal itself must reject the receipt: the proof fields
// are hashed by compactEOFRecoveryAdmissionSeal (finding F6). The second case
// re-seals the forged receipt, so the seal now agrees and only the semantic
// check in validateCompactEOFRecoveryAdmission can reject it.
func TestCompactEOFRecoveryScannerProofTamperingDeclines(t *testing.T) {
	tests := []struct {
		name    string
		reseal  bool
		mutate  func(*compactEOFRecoveryAdmissionReceipt)
		wantErr string
	}{
		{
			name:    "forged-probed-states",
			mutate:  func(r *compactEOFRecoveryAdmissionReceipt) { r.scannerQuiescence.probedStates = 2 },
			wantErr: "not authentic",
		},
		{
			name:    "forged-probed-states-resealed",
			reseal:  true,
			mutate:  func(r *compactEOFRecoveryAdmissionReceipt) { r.scannerQuiescence.probedStates = 2 },
			wantErr: "scanner proof changed",
		},
		{
			name:    "forged-proof-resealed",
			reseal:  true,
			mutate:  func(r *compactEOFRecoveryAdmissionReceipt) { r.scannerQuiescence.proved = true },
			wantErr: "scanner proof changed",
		},
		{
			name:   "forged-checkpoint-resealed",
			reseal: true,
			mutate: func(r *compactEOFRecoveryAdmissionReceipt) {
				r.scannerQuiescence.checkpointBefore = 7
			},
			wantErr: "scanner proof changed",
		},
		{
			name:   "forged-external-history-resealed",
			reseal: true,
			mutate: func(r *compactEOFRecoveryAdmissionReceipt) {
				r.externalCount = 1
				r.externalDigest[0] = 0xab
			},
			wantErr: "scanner proof changed",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fixture := newG4EOFFixture(t, g4EOFFixtureSpec{metadataActivation: true})
			if _, err := produceCompactEOFRecoveryAdmissionForTest(
				fixture.scheduler, fixture.source, func() error { return nil },
			); err != nil {
				t.Fatalf("produce receipt: %v", err)
			}
			if err := fixture.scheduler.validateCompactEOFRecoveryAdmission(
				fixture.source, compactEOFRecoveryAdmissionProduced,
			); err != nil {
				t.Fatalf("untampered receipt did not validate: %v", err)
			}

			receipt := &fixture.scheduler.eofRecoveryAdmission
			test.mutate(receipt)
			if test.reseal {
				var previous [32]byte
				if receipt.transitionCount > 1 {
					previous = receipt.transitionSeals[receipt.transitionCount-2]
				}
				receipt.seal = compactEOFRecoveryAdmissionSeal(receipt, previous)
				receipt.transitionSeals[receipt.transitionCount-1] = receipt.seal
			}

			err := fixture.scheduler.validateCompactEOFRecoveryAdmission(
				fixture.source, compactEOFRecoveryAdmissionProduced,
			)
			if err == nil {
				t.Fatal("tampered receipt validated")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), test.wantErr)
			}
		})
	}
}
