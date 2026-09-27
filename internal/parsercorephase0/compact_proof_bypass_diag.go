//go:build gts_diag && gts_diag_converged_split_no_action_bypass

package parsercorephase0

// CompactConvergedSplitProofBypassEnabled reports whether a diagnostic build
// explicitly opted into the converged-split no-action proof bypass.
func CompactConvergedSplitProofBypassEnabled() bool { return true }
