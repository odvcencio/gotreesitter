//go:build gts_compact_node_views && !gts_no_parsercorephase0

package gotreesitter

import core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"

// Qualification switch: ordinary parsing uses the complete public adapter,
// including indexed queries and the exclusive mutation boundary.
const compactNodeViewsEnabled = true

func compactNodeViewsForContract(identity [32]byte) bool {
	return core.NativeSelectedSnapshotContract(identity)
}
