//go:build gts_diag && !gts_diag_converged_split_no_action_bypass

package parsercorephase0

import "testing"

func TestCompactConvergedSplitProofBypassDisabledWithoutOptInTag(t *testing.T) {
	if CompactConvergedSplitProofBypassEnabled() {
		t.Fatal("diagnostic build enabled the bypass without the opt-in tag")
	}
}
