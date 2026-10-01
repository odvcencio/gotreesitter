package slabretention

// Closest returns an approximately sized reusable allocation, excluding entries
// over four times the demand. A negative result leaves large entries cached.
func Closest[T any](items []T, target int, capacity func(T) int) int {
	if target <= 0 {
		return len(items) - 1
	}
	best, distance := -1, int(^uint(0)>>1)
	for i, item := range items {
		size := capacity(item)
		if size/4 > target {
			continue
		}
		delta := size - target
		if delta < 0 {
			delta = -delta
		}
		if delta <= distance {
			best, distance = i, delta
		}
	}
	return best
}
