package grammargen

import "math/bits"

// parserStateWidthOrder reproduces the 64-bit integer specialization of Rust
// 1.90.0 ipnsort used for Tree-sitter v0.25.10's state-id vector. Recovery
// and start are explicit members of the permutation, ordered before all rows.
// Adapted from Rust's library/core/src/slice/sort at
// 1159e78c4747b02ef996e55082b704c09b970588 under the MIT license.
// See licenses/Rust-MIT.txt.
func parserStateWidthOrder(width []int) []int {
	ids := make([]int, len(width))
	for i := range ids {
		ids[i] = i
	}
	less := func(a, b int) bool {
		if a < 2 || b < 2 {
			return a < b
		}
		return width[a] > width[b]
	}
	if len(ids) <= 20 {
		widthInsert(ids, 1, less)
		return ids
	}
	reverse := less(ids[1], ids[0])
	run := 2
	for run < len(ids) && less(ids[run], ids[run-1]) == reverse {
		run++
	}
	if run == len(ids) {
		if reverse {
			for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
		return ids
	}
	widthQuick(ids, -1, 2*(bits.Len(uint(len(ids)))-1), less)
	return ids
}

func widthQuick(v []int, parent, limit int, less func(int, int) bool) {
	for len(v) > 32 {
		if limit == 0 {
			widthHeap(v, less)
			return
		}
		limit--
		pivot := widthPivot(v, 0, len(v)/8*4, len(v)/8*7, len(v)/8, less)
		if parent >= 0 && !less(parent, v[pivot]) {
			cut := widthPartition(v, pivot, func(a, b int) bool { return !less(b, a) })
			parent = -1
			v = v[cut+1:]
			continue
		}
		cut := widthPartition(v, pivot, less)
		widthQuick(v[:cut], parent, limit, less)
		parent = v[cut]
		v = v[cut+1:]
	}
	widthSmall(v, less)
}

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

func widthInsert(v []int, start int, less func(int, int) bool) {
	for i := start; i < len(v); i++ {
		for j := i; j > 0 && less(v[j], v[j-1]); j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

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

var widthNetwork9 = [][2]int{{0, 3}, {1, 7}, {2, 5}, {4, 8}, {0, 7}, {2, 4}, {3, 8}, {5, 6}, {0, 2}, {1, 3}, {4, 5}, {7, 8}, {1, 4}, {3, 6}, {5, 7}, {0, 1}, {2, 4}, {3, 5}, {6, 8}, {2, 3}, {4, 5}, {6, 7}, {1, 2}, {3, 4}, {5, 6}}

var widthNetwork13 = [][2]int{{0, 12}, {1, 10}, {2, 9}, {3, 7}, {5, 11}, {6, 8}, {1, 6}, {2, 3}, {4, 11}, {7, 9}, {8, 10}, {0, 4}, {1, 2}, {3, 6}, {7, 8}, {9, 10}, {11, 12}, {4, 6}, {5, 9}, {8, 11}, {10, 12}, {0, 5}, {3, 8}, {4, 7}, {6, 11}, {9, 10}, {0, 1}, {2, 5}, {6, 9}, {7, 8}, {10, 11}, {1, 3}, {2, 4}, {5, 6}, {9, 10}, {1, 2}, {3, 4}, {5, 7}, {6, 8}, {2, 3}, {4, 5}, {6, 7}, {8, 9}, {3, 4}, {5, 6}}
