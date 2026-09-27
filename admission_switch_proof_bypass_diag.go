//go:build gts_diag && !gts_no_parsercorephase0

package gotreesitter

import core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"

func compactConvergedSplitNoActionProofBypassEnabled() bool {
	return core.CompactConvergedSplitProofBypassEnabled()
}
