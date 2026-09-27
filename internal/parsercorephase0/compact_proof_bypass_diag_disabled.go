//go:build gts_diag && !gts_diag_converged_split_no_action_bypass

package parsercorephase0

// CompactConvergedSplitProofBypassEnabled stays off in diagnostic builds
// unless the dedicated bypass tag is also enabled.
const CompactConvergedSplitProofBypassEnabled = false
