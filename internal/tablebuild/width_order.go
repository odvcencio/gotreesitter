package tablebuild

import "math/bits"

// This file is a copy of grammargen/width_order.go, which stays in place for
// the grammargen path that uses it. The copy takes the key function of
// sort_unstable_by_key instead of a width table, and keeps the ancestor pivot
// beside a flag instead of the value -1. It ports the unstable sort of the Rust
// standard library (core::slice::sort::unstable, ipnsort) for a slice of
// usize, the element type of minimize_parse_table.rs: fn
// reorder_states_by_descending_size, where slice::sort_unstable_by_key calls
// it. The order of equal keys is defined only by this algorithm, so the
// port reproduces it step by step.
//
// The copy was checked against the text of library/core/src/slice/sort at
// Rust 1.90.0 (the source of grammargen's port) and at Rust 1.99.0. The
// v0.27.1 release binary of the tree-sitter CLI names rustc 1.99.0
// (b940084d7 2026-09-28) in its version string. Between the two releases the
// files differ only in attribute names and in how unsafe slices are formed;
// every comparison and move on this path is the same. For an element of 8
// bytes that is Copy, the small sort is small_sort_network (threshold 32),
// and the partition is partition_lomuto_branchless_cyclic.
//
// Adapted from Rust's library/core/src/slice/sort under the MIT license.
// See licenses/Rust-MIT.txt.

// sortUnstableByKey is slice::sort_unstable_by_key on a slice of usize:
// core::slice::sort::unstable::sort with is_less(a, b) = key(a) < key(b).
func sortUnstableByKey(v []int, key func(int) int64) {
	less := func(a, b int) bool { return key(a) < key(b) }
	if len(v) < 2 {
		return
	}
	// MAX_LEN_ALWAYS_INSERTION_SORT
	if len(v) <= 20 {
		widthInsert(v, 1, less)
		return
	}
	ipnsort(v, less)
}

// ipnsort is core::slice::sort::unstable::ipnsort.
func ipnsort(v []int, less func(int, int) bool) {
	// shared::find_existing_run
	reverse := less(v[1], v[0])
	run := 2
	for run < len(v) && less(v[run], v[run-1]) == reverse {
		run++
	}
	if run == len(v) {
		if reverse {
			for i, j := 0, len(v)-1; i < j; i, j = i+1, j-1 {
				v[i], v[j] = v[j], v[i]
			}
		}
		return
	}
	// limit = 2 * (len | 1).ilog2()
	widthQuick(v, 0, false, 2*(bits.Len(uint(len(v)|1))-1), less)
}

// widthQuick is unstable::quicksort::quicksort. parent holds the ancestor
// pivot when hasParent is set.
func widthQuick(v []int, parent int, hasParent bool, limit int, less func(int, int) bool) {
	for len(v) > 32 {
		if limit == 0 {
			widthHeap(v, less)
			return
		}
		limit--
		pivot := widthPivot(v, 0, len(v)/8*4, len(v)/8*7, len(v)/8, less)
		if hasParent && !less(parent, v[pivot]) {
			cut := widthPartition(v, pivot, func(a, b int) bool { return !less(b, a) })
			hasParent = false
			v = v[cut+1:]
			continue
		}
		cut := widthPartition(v, pivot, less)
		widthQuick(v[:cut], parent, hasParent, limit, less)
		parent, hasParent = v[cut], true
		v = v[cut+1:]
	}
	widthSmall(v, less)
}

// widthPivot is shared::pivot::choose_pivot with median3_rec and median3:
// the position of the pseudo-median of the regions at a, b and c, each n long.
func widthPivot(v []int, a, b, c, n int, less func(int, int) bool) int {
	if n >= 8 {
		stride := n / 8
		a = widthPivot(v, a, a+4*stride, a+7*stride, stride, less)
		b = widthPivot(v, b, b+4*stride, b+7*stride, stride, less)
		c = widthPivot(v, c, c+4*stride, c+7*stride, stride, less)
	}
	x, y := less(v[a], v[b]), less(v[a], v[c])
	if x != y {
		return a
	}
	if less(v[b], v[c]) != x {
		return c
	}
	return b
}

// widthPartition is unstable::quicksort::partition with
// partition_lomuto_branchless_cyclic.
func widthPartition(v []int, pivot int, less func(int, int) bool) int {
	v[0], v[pivot] = v[pivot], v[0]
	p := v[0]
	body := v[1:]
	saved := body[0]
	gap, left := 0, 0
	for right := 1; right <= len(body); right++ {
		value := saved
		if right < len(body) {
			value = body[right]
		}
		body[gap] = body[left]
		body[left] = value
		if less(value, p) {
			left++
		}
		gap = right
	}
	v[0], v[left] = v[left], v[0]
	return left
}

// widthHeap is unstable::heapsort::heapsort with sift_down.
func widthHeap(v []int, less func(int, int) bool) {
	for i := len(v) + len(v)/2 - 1; i >= 0; i-- {
		node, end := 0, i
		if i >= len(v) {
			node = i - len(v)
			end = len(v)
		} else {
			v[0], v[i] = v[i], v[0]
		}
		for child := 2*node + 1; child < end; child = 2*node + 1 {
			if child+1 < end && less(v[child], v[child+1]) {
				child++
			}
			if !less(v[node], v[child]) {
				break
			}
			v[node], v[child] = v[child], v[node]
			node = child
		}
	}
}

// widthInsert is shared::smallsort::insertion_sort_shift_left.
func widthInsert(v []int, start int, less func(int, int) bool) {
	for i := start; i < len(v); i++ {
		for j := i; j > 0 && less(v[j], v[j-1]); j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// widthSmall is shared::smallsort::small_sort_network. Its
// bidirectional_merge writes the stable merge of the two sorted halves from
// both ends; for a strict weak order that is the forward stable merge below.
func widthSmall(v []int, less func(int, int) bool) {
	region := func(s []int) {
		var network [][2]int
		start := 1
		if len(s) >= 13 {
			network = widthNetwork13
			start = 13
		} else if len(s) >= 9 {
			network = widthNetwork9
			start = 9
		}
		for _, pair := range network {
			a, b := pair[0], pair[1]
			if less(s[b], s[a]) {
				s[a], s[b] = s[b], s[a]
			}
		}
		widthInsert(s, start, less)
	}
	if len(v) < 18 {
		region(v)
		return
	}
	mid := len(v) / 2
	region(v[:mid])
	region(v[mid:])
	var scratch [32]int
	left, right := 0, mid
	for i := range v {
		if right == len(v) || left < mid && !less(v[right], v[left]) {
			scratch[i] = v[left]
			left++
		} else {
			scratch[i] = v[right]
			right++
		}
	}
	copy(v, scratch[:len(v)])
}

// widthNetwork9 is the swap_if_less sequence of smallsort::sort9_optimal.
var widthNetwork9 = [][2]int{{0, 3}, {1, 7}, {2, 5}, {4, 8}, {0, 7}, {2, 4}, {3, 8}, {5, 6}, {0, 2}, {1, 3}, {4, 5}, {7, 8}, {1, 4}, {3, 6}, {5, 7}, {0, 1}, {2, 4}, {3, 5}, {6, 8}, {2, 3}, {4, 5}, {6, 7}, {1, 2}, {3, 4}, {5, 6}}

// widthNetwork13 is the swap_if_less sequence of smallsort::sort13_optimal.
var widthNetwork13 = [][2]int{{0, 12}, {1, 10}, {2, 9}, {3, 7}, {5, 11}, {6, 8}, {1, 6}, {2, 3}, {4, 11}, {7, 9}, {8, 10}, {0, 4}, {1, 2}, {3, 6}, {7, 8}, {9, 10}, {11, 12}, {4, 6}, {5, 9}, {8, 11}, {10, 12}, {0, 5}, {3, 8}, {4, 7}, {6, 11}, {9, 10}, {0, 1}, {2, 5}, {6, 9}, {7, 8}, {10, 11}, {1, 3}, {2, 4}, {5, 6}, {9, 10}, {1, 2}, {3, 4}, {5, 7}, {6, 8}, {2, 3}, {4, 5}, {6, 7}, {8, 9}, {3, 4}, {5, 6}}
