//go:build gts_diag && gts_diag_converged_split_no_action_bypass

package parsercorephase0

import "testing"

func TestCompactConvergedSplitProofBypassEnabledByOptInTag(t *testing.T) {
	if !CompactConvergedSplitProofBypassEnabled {
		t.Fatal("diagnostic bypass build tag did not enable the bypass")
	}
}
