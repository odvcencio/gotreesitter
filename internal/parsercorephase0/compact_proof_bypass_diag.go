//go:build gts_diag

package parsercorephase0

import "os"

const compactProofBypassEnv = "GTS_DIAG_BYPASS_CONVERGED_SPLIT_NO_ACTION_PROOFS"

// CompactConvergedSplitProofBypassEnabled reports whether a diagnostic build
// should continue past the compact converged-split no-action proof gates.
// It has no effect unless the binary is built with gts_diag.
func CompactConvergedSplitProofBypassEnabled() bool {
	return compactProofBypassFromEnv()
}

//go:noinline
func compactProofBypassFromEnv() bool {
	return os.Getenv(compactProofBypassEnv) == "1"
}
