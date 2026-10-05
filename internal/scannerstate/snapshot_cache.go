package scannerstate

// SnapshotCache holds a bounded set of generation-local, owned snapshots.
// The caller clears the cache when either generation or owner changes.
type SnapshotCache[T any] struct {
	keys   [8]uint32
	values [8]T
	used   uint8
	next   uint8
}

func (c *SnapshotCache[T]) Lookup(key uint32) (T, bool) {
	for i := uint8(0); i < c.used; i++ {
		if c.keys[i] == key {
			return c.values[i], true
		}
	}
	var zero T
	return zero, false
}
func (c *SnapshotCache[T]) Store(key uint32, value T) {
	for i := uint8(0); i < c.used; i++ {
		if c.keys[i] == key {
			c.values[i] = value
			return
		}
	}
	c.keys[c.next] = key
	c.values[c.next] = value
	c.next = (c.next + 1) % 8
	if c.used < 8 {
		c.used++
	}
}
