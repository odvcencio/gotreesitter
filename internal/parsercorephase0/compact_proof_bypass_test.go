//go:build !gts_diag

package parsercorephase0

import "testing"

func TestCompactConvergedSplitProofBypassDisabledWithoutDiagnosticTag(t *testing.T) {
	t.Setenv("GTS_DIAG_BYPASS_CONVERGED_SPLIT_NO_ACTION_PROOFS", "1")
	if CompactConvergedSplitProofBypassEnabled() {
		t.Fatal("ordinary build enabled the diagnostic proof bypass")
	}
}
