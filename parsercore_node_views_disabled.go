//go:build !gts_compact_node_views && !gts_no_parsercorephase0

package gotreesitter

import core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"

const compactNodeViewsEnabled = false

func compactNodeViewsForContract(identity [32]byte) bool {
	return core.NativeSelectedSnapshotDefaultContract(identity)
}
