// Package compactheader stores slice headers with 32-bit lengths and capacities.
package compactheader

import "unsafe"

// Slice retains normal slice aliasing, capacity, and nil/empty semantics. On
// 64-bit systems it occupies 16 bytes instead of 24. A slice too large for a
// uint32 capacity uses a boxed ordinary header, preserving the full Go range.
type Slice[T any] struct {
	data     unsafe.Pointer
	length   uint32
	capacity uint32
}

func From[T any](value []T) Slice[T] {
	if uint64(cap(value)) > uint64(^uint32(0)) {
		return wide(value)
	}
	return Slice[T]{unsafe.Pointer(unsafe.SliceData(value)), uint32(len(value)), uint32(cap(value))}
}

func wide[T any](value []T) Slice[T] {
	return Slice[T]{unsafe.Pointer(&value), ^uint32(0), 0}
}

func (s Slice[T]) Get() []T {
	if s.length > s.capacity {
		return *(*[]T)(s.data)
	}
	return unsafe.Slice((*T)(s.data), int(s.capacity))[:int(s.length)]
}
