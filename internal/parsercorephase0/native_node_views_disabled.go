//go:build !gts_compact_node_views

package parsercorephase0

const NativeNodeViewsEnabled = false

func NativeNodeViewsForContract(identity [32]byte) bool {
	return NativeSelectedSnapshotDefaultContract(identity)
}
