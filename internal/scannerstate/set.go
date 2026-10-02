// Package scannerstate stores scanner checkpoints without copying live arrays
// when the number of recorded nodes grows.
package scannerstate

import (
	"sort"
	"unsafe"
)

const maxChunk = 32 * 1024

type chunk[T any] struct {
	indexes []uint32
	refs    []T
}

// Set associates a node index with a checkpoint. Chunks grow geometrically up
// to maxChunk, then linearly. Indexes and references have separate backing
// arrays so aligning a reference cannot add padding to every index.
type Set[T any] struct {
	chunks []chunk[T]
	cursor int
	count  int
	bytes  int64
}

func (s *Set[T]) grow() {
	n := 128
	if len(s.chunks) != 0 {
		n = min(cap(s.chunks[len(s.chunks)-1].indexes)*2, maxChunk)
	}
	s.addChunk(n)
}

func (s *Set[T]) addChunk(n int) {
	before := cap(s.chunks)
	s.chunks = append(s.chunks, chunk[T]{
		indexes: make([]uint32, 0, n),
		refs:    make([]T, 0, n),
	})
	var ref T
	s.bytes += int64(n)*(4+int64(unsafe.Sizeof(ref))) + int64(cap(s.chunks)-before)*int64(unsafe.Sizeof(chunk[T]{}))
}

func (s *Set[T]) tail() *chunk[T] {
	if len(s.chunks) == 0 {
		s.grow()
	}
	if len(s.chunks[s.cursor].indexes) == cap(s.chunks[s.cursor].indexes) {
		s.cursor++
		if s.cursor == len(s.chunks) {
			s.grow()
		}
	}
	return &s.chunks[s.cursor]
}

func (s *Set[T]) find(key uint32) (int, int) {
	last := &s.chunks[s.cursor]
	c := s.cursor
	if key < last.indexes[0] {
		c = sort.Search(s.cursor, func(i int) bool {
			indexes := s.chunks[i].indexes
			return indexes[len(indexes)-1] >= key
		})
	}
	indexes := s.chunks[c].indexes
	i := sort.Search(len(indexes), func(i int) bool { return indexes[i] >= key })
	return c, i
}

func (s *Set[T]) Lookup(idx int) (T, bool) {
	var zero T
	if s == nil || s.count == 0 || idx < 0 {
		return zero, false
	}
	c, i := s.find(uint32(idx))
	chunk := &s.chunks[c]
	if i == len(chunk.indexes) || chunk.indexes[i] != uint32(idx) {
		return zero, false
	}
	return chunk.refs[i], true
}

// Upsert returns the growth in retained backing bytes, including the directory.
func (s *Set[T]) Upsert(idx int, ref T) int64 {
	if s == nil || idx < 0 {
		return 0
	}
	before := s.Bytes()
	key := uint32(idx)
	if s.count == 0 || s.chunks[s.cursor].indexes[len(s.chunks[s.cursor].indexes)-1] < key {
		tail := s.tail()
		tail.indexes = append(tail.indexes, key)
		tail.refs = append(tail.refs, ref)
		s.count++
		return s.Bytes() - before
	}
	c, pos := s.find(key)
	if s.chunks[c].indexes[pos] == key {
		s.chunks[c].refs[pos] = ref
		return 0
	}
	s.tail()
	// Make one slot in the selected chunk by carrying its final entry into
	// each following chunk. All but the active tail are full.
	for i := s.cursor; i > c; i-- {
		current, previous := &s.chunks[i], &s.chunks[i-1]
		last := len(previous.indexes) - 1
		current.indexes = append(current.indexes, 0)
		copy(current.indexes[1:], current.indexes)
		current.indexes[0] = previous.indexes[last]
		var zero T
		current.refs = append(current.refs, zero)
		copy(current.refs[1:], current.refs)
		current.refs[0] = previous.refs[last]
		previous.indexes = previous.indexes[:last]
		previous.refs = previous.refs[:last]
	}
	selected := &s.chunks[c]
	selected.indexes = append(selected.indexes, 0)
	copy(selected.indexes[pos+1:], selected.indexes[pos:])
	selected.indexes[pos] = key
	var zero T
	selected.refs = append(selected.refs, zero)
	copy(selected.refs[pos+1:], selected.refs[pos:])
	selected.refs[pos] = ref
	s.count++
	return s.Bytes() - before
}

func (s *Set[T]) EnsureCapacity(n int) int64 {
	if s == nil || n <= 0 {
		return 0
	}
	before := s.Bytes()
	for capacity := int(s.Slots()); capacity < n; {
		chunkSize := min(n-capacity, maxChunk)
		s.addChunk(chunkSize)
		capacity += chunkSize
	}
	return s.Bytes() - before
}

func (s *Set[T]) Reset() {
	if s == nil {
		return
	}
	for i := range s.chunks {
		clear(s.chunks[i].refs)
		s.chunks[i].refs = s.chunks[i].refs[:0]
		s.chunks[i].indexes = s.chunks[i].indexes[:0]
	}
	s.count, s.cursor = 0, 0
}

func (s *Set[T]) Bytes() int64 {
	if s == nil {
		return 0
	}
	return s.bytes
}

func (s *Set[T]) Slots() uint64 {
	if s == nil {
		return 0
	}
	var count uint64
	for i := range s.chunks {
		count += uint64(cap(s.chunks[i].refs))
	}
	return count
}
