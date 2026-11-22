//go:build cgo

package tree_sitter

/*
#cgo gts_subtree_counters CFLAGS: -DGTS_SUBTREE_COUNTERS
#include <stdint.h>
int gts_subtree_counters_enabled(void);
void gts_subtree_counters_reset(void);
void gts_subtree_counters_read(uint64_t *out);
*/
import "C"

// SubtreeCreations are the subtree creations of the pinned runtime between
// SubtreeCountersReset and SubtreeCountersRead. Leaves are heap leaves (the
// heap branch of ts_subtree_new_leaf), Nodes are all ts_subtree_new_node calls,
// Clones are ts_subtree_clone calls, and Scratch are the comparison trees of
// ts_parser__select_children, which are also in Nodes.
type SubtreeCreations struct {
	Leaves, Nodes, Clones, Scratch uint64
}

// Total is the creations that a parse made and kept as records: leaves,
// nodes without the comparison trees, and clones.
func (c SubtreeCreations) Total() uint64 { return c.Leaves + c.Nodes - c.Scratch + c.Clones }

// SubtreeCountersEnabled reports whether the runtime was built with the
// gts_subtree_counters tag. Without it the counters stay zero.
func SubtreeCountersEnabled() bool { return C.gts_subtree_counters_enabled() != 0 }

// SubtreeCountersReset zeroes the counters. They are process-wide: use them
// with one parse at a time.
func SubtreeCountersReset() { C.gts_subtree_counters_reset() }

// SubtreeCountersRead returns the counters since the last reset.
func SubtreeCountersRead() SubtreeCreations {
	var v [4]C.uint64_t
	C.gts_subtree_counters_read(&v[0])
	return SubtreeCreations{Leaves: uint64(v[0]), Nodes: uint64(v[1]), Clones: uint64(v[2]), Scratch: uint64(v[3])}
}
