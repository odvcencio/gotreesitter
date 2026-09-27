//go:build gts_diag && gts_diag_converged_split_no_action_bypass

package parsercorephase0

// CompactConvergedSplitProofBypassEnabled is true only in the explicitly
// tagged diagnostic build. As a constant, it leaves no runtime switch symbol.
const CompactConvergedSplitProofBypassEnabled = true
