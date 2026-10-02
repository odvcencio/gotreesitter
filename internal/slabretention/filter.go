package slabretention

// Filter keeps the live entries satisfying keep, in their original order.
// Removed entries are cleared so their backing allocations can be collected.
// Slots beyond the slice's length keep their separate lifecycle bookkeeping.
func Filter[T any](items []T, keep func(T) bool) []T {
	count := 0
	for _, item := range items {
		if keep(item) {
			items[count] = item
			count++
		}
	}
	clear(items[count:])
	return items[:count]
}
