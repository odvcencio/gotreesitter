// Package slabretention releases unused slab headers while retaining a warm prefix.
package slabretention

// Prefix clears discarded headers so backing arrays and references can be collected.
func Prefix[T any](slabs []T, count int) []T {
	count = min(max(count, 0), len(slabs))
	clear(slabs[count:])
	return slabs[:count]
}
