package scannerstate

import "unsafe"

// DictionarySet stores repeated immutable receipts in one small dictionary.
// Rare receipts spill to the ordinary set after the bounded dictionary fills.
type DictionarySet[T comparable] struct {
	values  []T
	indexes Set[uint8]
	spill   Set[T]
}

func (s *DictionarySet[T]) Lookup(index int) (T, bool) {
	if value, ok := s.spill.Lookup(index); ok {
		return value, true
	}
	id, ok := s.indexes.Lookup(index)
	if ok && int(id) < len(s.values) {
		return s.values[id], true
	}
	var zero T
	return zero, false
}
func (s *DictionarySet[T]) Upsert(index int, value T) int64 {
	if index < 0 {
		return 0
	}
	before := s.Bytes()
	if _, ok := s.spill.Lookup(index); ok {
		s.spill.Upsert(index, value)
		return s.Bytes() - before
	}
	for i, v := range s.values {
		if v == value {
			s.indexes.Upsert(index, uint8(i))
			return s.Bytes() - before
		}
	}
	if len(s.values) < 64 {
		s.values = append(s.values, value)
		s.indexes.Upsert(index, uint8(len(s.values)-1))
	} else {
		s.spill.Upsert(index, value)
	}
	return s.Bytes() - before
}
func (s *DictionarySet[T]) EnsureCapacity(n int) int64 { return s.indexes.EnsureCapacity(n) }
func (s *DictionarySet[T]) Reset() {
	clear(s.values)
	s.values = s.values[:0]
	s.indexes.Reset()
	s.spill.Reset()
}
func (s *DictionarySet[T]) Bytes() int64 {
	var value T
	return int64(cap(s.values))*int64(unsafe.Sizeof(value)) + s.indexes.Bytes() + s.spill.Bytes()
}
func (s *DictionarySet[T]) Slots() uint64 { return s.indexes.Slots() + s.spill.Slots() }
