// Package treeview records tree-local occurrences of shared syntax payloads.
package treeview

import "sync"

type id uint32

// Record holds one tree-local occurrence. Payload, Parent, ChildIndex and View
// are initialized before publication and stay immutable until Clear. The
// record's address stays stable as the index grows, allowing allocation-free
// payload and parent reads without a lookup or a lock.
type Record[P, V any] struct {
	Payload    P
	Parent     *Record[P, V]
	View       V
	ChildIndex int32
	firstChild uint32
	childCount uint32
}

const (
	inlineRecords = 8
	blockRecords  = 256
)

// Index stores visited occurrences in stable dense slabs. Views live inside
// their records, avoiding a heap allocation per view. A child is keyed by its
// parent's occurrence and child index, never by payload identity. Callbacks
// run under the index lock and must not call back into this Index.
type Index[P, V any] struct {
	mu       sync.Mutex
	inline   [inlineRecords]Record[P, V]
	blocks   [][]Record[P, V]
	used     uint32
	children []id
}

func (s *Index[P, V]) record(index uint32) *Record[P, V] {
	if index < inlineRecords {
		return &s.inline[index]
	}
	index -= inlineRecords
	return &s.blocks[index/blockRecords][index%blockRecords]
}

func (s *Index[P, V]) appendRecord(payload P, parent *Record[P, V], childIndex int32, create func(*Record[P, V]) V) *Record[P, V] {
	if s.used == ^uint32(0) {
		panic("treeview: occurrence index exhausted")
	}
	if s.used >= inlineRecords && (s.used-inlineRecords)%blockRecords == 0 {
		s.blocks = append(s.blocks, make([]Record[P, V], blockRecords))
	}
	r := s.record(s.used)
	s.used++
	r.Payload, r.Parent, r.ChildIndex = payload, parent, childIndex
	r.View = create(r)
	return r
}

// Root returns the stable root view, initializing an empty index on first use.
func (s *Index[P, V]) Root(payload P, create func(*Record[P, V]) V) *V {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used == 0 {
		s.appendRecord(payload, nil, -1, create)
	}
	return &s.inline[0].View
}

// Child returns the stable view for one child. load supplies the child count
// and payload without consulting shared parent links. The child slot array is
// allocated on the parent's first successful child access; other child views
// and payloads are left untouched.
func (s *Index[P, V]) Child(r *Record[P, V], i int, load func(P, int) (P, int, bool), create func(*Record[P, V]) V) *V {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r == nil || s.used == 0 || i < 0 {
		return nil
	}
	if uint64(i) < uint64(r.childCount) {
		if child := s.children[int(r.firstChild)+i]; child != 0 {
			return &s.record(uint32(child) - 1).View
		}
	}
	payload, count, ok := load(r.Payload, i)
	if !ok || i >= count {
		return nil
	}
	if r.childCount == 0 {
		if uint64(len(s.children))+uint64(count) > uint64(^uint32(0)) || uint64(count) > uint64(^uint32(0)>>1) {
			panic("treeview: child index exhausted")
		}
		r.firstChild = uint32(len(s.children))
		r.childCount = uint32(count)
		s.children = append(s.children, make([]id, count)...)
	}
	child := s.appendRecord(payload, r, int32(i), create)
	s.children[int(r.firstChild)+i] = id(s.used)
	return &child.View
}

// Clear scrubs payload references in every published record before dropping
// slabs. A surviving public view may keep a slab alive, so dropping the slab
// directory alone would retain payload arenas after the tree's final release.
// The owning tree must exclude navigation while it releases its last handle.
func (s *Index[P, V]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.inline[:])
	for _, block := range s.blocks {
		clear(block)
	}
	s.blocks = nil
	s.children = nil
	s.used = 0
}
