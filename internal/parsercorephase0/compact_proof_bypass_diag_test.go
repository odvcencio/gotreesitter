//go:build gts_diag

package parsercorephase0

import "testing"

func TestCompactConvergedSplitProofBypassRequiresExplicitSwitch(t *testing.T) {
	for _, value := range []string{"", "0", "true"} {
		t.Run("disabled_"+value, func(t *testing.T) {
			t.Setenv("GTS_DIAG_BYPASS_CONVERGED_SPLIT_NO_ACTION_PROOFS", value)
			if CompactConvergedSplitProofBypassEnabled() {
				t.Fatalf("switch value %q enabled the bypass", value)
			}
		})
	}
	t.Setenv("GTS_DIAG_BYPASS_CONVERGED_SPLIT_NO_ACTION_PROOFS", "1")
	if !CompactConvergedSplitProofBypassEnabled() {
		t.Fatal("diagnostic switch value 1 did not enable the bypass")
	}
}
