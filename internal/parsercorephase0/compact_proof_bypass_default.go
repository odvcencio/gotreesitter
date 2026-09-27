//go:build !gts_diag

package parsercorephase0

// CompactConvergedSplitProofBypassEnabled is hard-disabled in ordinary builds.
func CompactConvergedSplitProofBypassEnabled() bool { return false }
