package benchfixtures

import (
	"math/rand"
	"strconv"
)

// FreshSizesForSeed permutes the fresh-parse sizes with an explicit test shuffle
// seed. Go's -shuffle permutes top-level benchmarks, so a benchmark that groups
// its sizes into sub-benchmarks needs to shuffle those cases itself as well.
func FreshSizesForSeed(seed string) []int {
	n, _ := strconv.ParseInt(seed, 10, 64)
	sizes := []int{137 << 10, 1 << 20}
	rand.New(rand.NewSource(n)).Shuffle(len(sizes), func(i, j int) {
		sizes[i], sizes[j] = sizes[j], sizes[i]
	})
	return sizes
}
