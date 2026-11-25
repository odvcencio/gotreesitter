package tablebuild

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// sortCases hold the order that slice::sort_unstable_by_key gives the ids
// 0..n under the key of reorder_states_by_descending_size (ids 0 and 1 first,
// then by descending width), for widths drawn from splitmix64 with the given
// seed, modulo range. The digest is the first 16 bytes of the SHA-256 of the
// order written as comma-separated ids. A Rust program compiled with rustc
// 1.90.0 wrote them; the sort code of Rust 1.99.0, which built the
// generator's v0.27.1 release binary, takes the same steps (width_order.go).
var sortCases = []struct {
	seed   uint64
	n      int
	rng    uint64
	digest string
}{
	{1, 21, 3, "e8fb00483090ad5c4b1d1ca804945786"},
	{2, 33, 5, "0a73e843a308e511f7088239e98520f5"},
	{3, 40, 2, "5c924d815f8c7a3afde6f8ddee86ff69"},
	{4, 64, 8, "8fe176f76abf3d08cb9a10a238409c4e"},
	{5, 65, 1, "06b5d4146b6b0ea6bda19b5fe4b23033"},
	{6, 100, 10, "eea8b90d0af99cf6f3218586f0d03181"},
	{7, 257, 6, "b9b95914cf7fba59d0aeb935be3bc173"},
	{8, 500, 40, "d4ab05342fc71fcdb711aadaf9b0525b"},
	{9, 600, 3, "c4d7a6e878fc22e1085445a40c346fb7"},
	{10, 1000, 25, "6ea8f429b20db8ad8e1b88db7b4ccc31"},
	{11, 2000, 60, "1dcef069b7e1a3aeb9d8b4a93b35c931"},
	{12, 7899, 90, "3b0c2b45f90055a8ef2b77cc0b5aa478"},
	{13, 20, 3, "5a0b06d939a55128220005db3990daf0"},
	{14, 5, 2, "03898aa9587574ff01e80324dc94eb22"},
	{15, 33, 1, "614b2e84b297317ae91eb96168255bc9"},
	{16, 4000, 2, "ef646d0fae7cc714e95c663a78546b0c"},
}

func splitmix64(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// TestSortUnstableByKeyMatchesRust compares the tie order of the port with
// the order that the Rust standard library gives.
func TestSortUnstableByKeyMatchesRust(t *testing.T) {
	for _, c := range sortCases {
		s := c.seed
		width := make([]int64, c.n)
		for i := range width {
			width[i] = int64(splitmix64(&s) % c.rng)
		}
		ids := make([]int, c.n)
		for i := range ids {
			ids[i] = i
		}
		sortUnstableByKey(ids, func(i int) int64 {
			if i <= 1 {
				return int64(i) - 1_000_000
			}
			return -width[i]
		})
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = strconv.Itoa(id)
		}
		text := strings.Join(parts, ",")
		sum := sha256.Sum256([]byte(text))
		if got := hex.EncodeToString(sum[:16]); got != c.digest {
			head := text
			if len(head) > 200 {
				head = head[:200] + "..."
			}
			t.Errorf("seed %d, n %d, range %d: order digest %s, Rust's %s; order %s", c.seed, c.n, c.rng, got, c.digest, head)
		}
		// Whatever the tie order, the result is sorted and a permutation.
		if !sort.SliceIsSorted(ids, func(a, b int) bool {
			ka, kb := -width[ids[a]], -width[ids[b]]
			if ids[a] <= 1 || ids[b] <= 1 {
				return ids[a] < ids[b]
			}
			return ka < kb
		}) {
			t.Errorf("seed %d: not sorted", c.seed)
		}
	}
}
