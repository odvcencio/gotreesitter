// Package slicearena provides bounded, reusable storage for temporary vectors.
package slicearena

// Arena allocates disjoint slices from chunks, retaining at most Limit elements.
// Allocations beyond that bound use individual backing arrays. Reset clears all
// references in the retained chunks before making them available again.
type Arena[T any] struct {
	Limit, Chunk int
	slabs        [][]T
	capacity     int
	cursor       int
	offset       int
}

func (a *Arena[T]) Alloc(n int) []T {
	if n <= 0 {
		return nil
	}
	if a == nil {
		return make([]T, n)
	}
	for a.cursor < len(a.slabs) {
		slab := a.slabs[a.cursor]
		if len(slab)-a.offset >= n {
			start := a.offset
			a.offset += n
			return slab[start:a.offset:a.offset]
		}
		a.cursor++
		a.offset = 0
	}
	remaining := a.Limit - a.capacity
	if n > remaining {
		return make([]T, n)
	}
	capacity := min(max(n, a.Chunk), remaining)
	slab := make([]T, capacity)
	a.slabs = append(a.slabs, slab)
	a.capacity += capacity
	a.offset = n
	return slab[:n:n]
}

func (a *Arena[T]) Reset() {
	if a == nil {
		return
	}
	for _, slab := range a.slabs {
		clear(slab)
	}
	a.cursor, a.offset = 0, 0
}
