//go:build gts_compact_node_views

package parsercorephase0

// Qualification switch: ordinary parsing uses the complete public adapter,
// including indexed queries and the exclusive mutation boundary.
const NativeNodeViewsEnabled = true

func NativeNodeViewsForContract(identity [32]byte) bool {
	return NativeSelectedSnapshotContract(identity)
}
