//go:build !gts_diag

package parsercorephase0

import "testing"

func TestCompactConvergedSplitProofBypassAbsentFromDefaultBuild(t *testing.T) {
	if CompactConvergedSplitProofBypassEnabled {
		t.Fatal("ordinary build enabled the diagnostic proof bypass")
	}
}
