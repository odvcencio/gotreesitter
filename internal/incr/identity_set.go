package incr

import "unsafe"

// IdentitySet is scratch-only pointer membership. Clear removes every strong
// reference. It never exposes its slots to callers.
type IdentitySet[T any] struct {
	slots []*T
	count int
}

const identitySetMaxSlots = 1 << 26

func (s *IdentitySet[T]) Len() int      { return s.count }
func (s *IdentitySet[T]) Capacity() int { return len(s.slots) }
func (s *IdentitySet[T]) Bytes() int64  { return int64(cap(s.slots)) * int64(unsafe.Sizeof(uintptr(0))) }
func (s *IdentitySet[T]) Clear()        { clear(s.slots); s.count = 0 }
func (s *IdentitySet[T]) Drop()         { s.slots = nil; s.count = 0 }

// Go currently uses non-moving heap addresses. Hashes are ephemeral; the set
// retains typed pointers and never reconstructs a pointer from an integer.
func identityHash[T any](p *T) uint64 {
	x := uint64(uintptr(unsafe.Pointer(p)))
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return x
}
func (s *IdentitySet[T]) Contains(p *T) bool {
	if p == nil || len(s.slots) == 0 {
		return false
	}
	mask := uint64(len(s.slots) - 1)
	i := identityHash(p) & mask
	for {
		item := s.slots[i]
		if item == nil {
			return false
		}
		if item == p {
			return true
		}
		i = (i + 1) & mask
	}
}

// GrowthBytes is the full new allocation, including temporary overlap with
// the old backing array. A negative result means the representable cap ended.
func (s *IdentitySet[T]) GrowthBytes() int64 {
	if len(s.slots) > 0 && s.count+1 <= len(s.slots)/2 {
		return 0
	}
	size := 128
	if len(s.slots) > 0 {
		if len(s.slots) >= identitySetMaxSlots {
			return -1
		}
		size = len(s.slots) * 2
	}
	return int64(size) * int64(unsafe.Sizeof(uintptr(0)))
}
func (s *IdentitySet[T]) Grow() bool {
	cost := s.GrowthBytes()
	if cost <= 0 {
		return cost == 0
	}
	size := int(cost / int64(unsafe.Sizeof(uintptr(0))))
	next := make([]*T, size)
	mask := uint64(size - 1)
	for _, p := range s.slots {
		if p != nil {
			i := identityHash(p) & mask
			for next[i] != nil {
				i = (i + 1) & mask
			}
			next[i] = p
		}
	}
	s.slots = next
	return true
}

// AddKnownAbsent requires that Grow has made enough room and that p is nonnil.
func (s *IdentitySet[T]) AddKnownAbsent(p *T) {
	mask := uint64(len(s.slots) - 1)
	i := identityHash(p) & mask
	for s.slots[i] != nil {
		if s.slots[i] == p {
			return
		}
		i = (i + 1) & mask
	}
	s.slots[i] = p
	s.count++
}
