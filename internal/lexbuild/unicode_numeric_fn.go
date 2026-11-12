package lexbuild

import "sort"

// isRustNumeric is char::is_numeric of the Rust standard library.
func isRustNumeric(c uint32) bool {
	i := sort.Search(len(rustNumericRanges), func(i int) bool { return rustNumericRanges[i][1] >= c })
	return i < len(rustNumericRanges) && rustNumericRanges[i][0] <= c
}
