// Package treeview records tree-local occurrences of shared syntax payloads.
package treeview

import "sync"

// ID identifies an occurrence within one Index. Zero means no occurrence.
type ID uint32

type record[P, V any] struct {
	payload    P
	view       V
	parent     ID
	childIndex int32
	firstChild uint32
	childCount uint32
}

// Index caches relations and views only for visited occurrences. A child is
// keyed by its parent's occurrence and child index, never by payload identity.
// Callbacks run under the index lock and must not call back into this Index.
type Index[P, V any] struct {
	mu       sync.RWMutex
	records  []record[P, V]
	children []ID
}

// Root returns the stable root view, initializing an empty index on first use.
func (s *Index[P, V]) Root(payload P, create func(ID) V) V {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) == 0 {
		s.records = append(s.records, record[P, V]{payload: payload, childIndex: -1, view: create(1)})
	}
	return s.records[0].view
}

// Payload returns an occurrence's shared payload. Invalid IDs return zero.
func (s *Index[P, V]) Payload(id ID) (p P) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id != 0 && uint64(id) <= uint64(len(s.records)) {
		return s.records[id-1].payload
	}
	return p
}

// Parent returns the stable parent view and this occurrence's child index.
func (s *Index[P, V]) Parent(id ID) (v V, childIndex int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id == 0 || uint64(id) > uint64(len(s.records)) {
		return v, -1
	}
	r := &s.records[id-1]
	if r.parent != 0 {
		v = s.records[r.parent-1].view
	}
	return v, int(r.childIndex)
}

// Child returns the stable view for one child. load supplies the child count
// and payload without consulting shared parent links. The child slot array is
// allocated on the parent's first successful child access; other child views
// and payloads are left untouched.
func (s *Index[P, V]) Child(id ID, i int, load func(P, int) (P, int, bool), create func(ID) V) (v V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == 0 || uint64(id) > uint64(len(s.records)) || i < 0 {
		return v
	}
	r := &s.records[id-1]
	if uint64(i) < uint64(r.childCount) {
		if child := s.children[int(r.firstChild)+i]; child != 0 {
			return s.records[child-1].view
		}
	}
	payload, count, ok := load(r.payload, i)
	if !ok || i >= count {
		return v
	}
	if r.childCount == 0 {
		if uint64(len(s.children))+uint64(count) > uint64(^uint32(0)) || uint64(count) > uint64(^uint32(0)>>1) {
			panic("treeview: child index exhausted")
		}
		r.firstChild = uint32(len(s.children))
		r.childCount = uint32(count)
		s.children = append(s.children, make([]ID, count)...)
	}
	// Reserve zero for nil. No real Go slice can hold 2^32 records.
	next := ID(len(s.records) + 1)
	if next == 0 {
		panic("treeview: occurrence index exhausted")
	}
	v = create(next)
	s.children[int(r.firstChild)+i] = next
	s.records = append(s.records, record[P, V]{payload: payload, view: v, parent: id, childIndex: int32(i)})
	return v
}

// Clear drops payloads and cached views at the owning tree's final release.
func (s *Index[P, V]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
	s.children = nil
}
