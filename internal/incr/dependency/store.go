// Package dependency stores coordinate-bound reuse receipts by arena node slot.
package dependency

import (
	"sync/atomic"
	"unsafe"
)

const pageSlots = 128

const (
	phaseMask    = 7
	phaseLive    = 1
	phaseUnseen  = 2
	phaseSeen    = 3
	phaseUnknown = 4
	phaseWrite   = 5
)

// Record binds an examined lookahead extent to the node's source geometry.
type Record struct {
	Start, End, Lookahead, Column uint32
}

// A page never moves while its arena is live. Readers can use an old directory
// during growth because all directories refer to the same existing pages.
type entry struct {
	state atomic.Uint64
	span  atomic.Uint64
	proof atomic.Uint64
}

type page [pageSlots]entry
type directory struct{ pages []atomic.Pointer[page] }

// Store has lock-free reads. Its owner must serialize mutations and range
// visits, including the accompanying interval index and memory-budget charge.
// Node coordinates are owned by the caller; invalidation reads only receipts.
type Store struct {
	directory atomic.Pointer[directory]
	count     int
	bytes     int64
}

func (s *Store) Len() int     { return s.count }
func (s *Store) Bytes() int64 { return s.bytes }

func (s *Store) entry(slot int) *entry {
	if slot < 0 {
		return nil
	}
	d := s.directory.Load()
	if d == nil || slot/pageSlots >= len(d.pages) {
		return nil
	}
	p := d.pages[slot/pageSlots].Load()
	if p == nil {
		return nil
	}
	return &p[slot%pageSlots]
}

func (s *Store) Get(slot int) (Record, bool) {
	e := s.entry(slot)
	if e == nil {
		return Record{}, false
	}
	before := e.state.Load()
	if before&phaseMask != phaseLive {
		return Record{}, false
	}
	span, proof := e.span.Load(), e.proof.Load()
	if e.state.Load() != before {
		return Record{}, false
	}
	return Record{uint32(span >> 32), uint32(span), uint32(proof), uint32(proof >> 32)}, true
}

// Growth reports the additional retained capacity required by a node slot.
func (s *Store) Growth(slot int) int64 {
	if slot < 0 || s.entry(slot) != nil {
		return 0
	}
	d := s.directory.Load()
	pages, oldPages := slot/pageSlots+1, 0
	if d != nil {
		oldPages = len(d.pages)
	}
	cost := int64(unsafe.Sizeof(page{}))
	if pages > oldPages {
		pages = max(pages, 2*oldPages)
		cost += int64(pages-oldPages) * int64(unsafe.Sizeof((*page)(nil)))
		if d == nil {
			cost += int64(unsafe.Sizeof(directory{}))
		}
	}
	return cost
}

func (s *Store) ensure(slot int, available int64) (*entry, int64) {
	if slot < 0 {
		return nil, 0
	}
	e := s.entry(slot)
	var cost int64
	if e == nil {
		d := s.directory.Load()
		pages := slot/pageSlots + 1
		oldPages := 0
		if d != nil {
			oldPages = len(d.pages)
		}
		cost = s.Growth(slot)
		if available >= 0 && cost > available {
			return nil, 0
		}
		if pages > oldPages {
			pages = max(pages, 2*oldPages)
			next := &directory{pages: make([]atomic.Pointer[page], pages)}
			if d != nil {
				for i := range d.pages {
					next.pages[i].Store(d.pages[i].Load())
				}
			}
			s.directory.Store(next)
			d = next
		}
		d.pages[slot/pageSlots].Store(new(page))
		s.bytes += cost
		e = s.entry(slot)
	}
	return e, cost
}

// Set charges actual backing capacity before allocation. A negative available
// budget means unlimited. Zero-lookahead receipts are explicitly present.
func (s *Store) Set(slot int, r Record, available int64) (int64, bool) {
	e, cost := s.ensure(slot, available)
	if e == nil {
		return 0, false
	}
	state := e.state.Load()
	if state&phaseMask != phaseLive {
		s.count++
	}
	// Advance the sequence even across invalidation and reauthentication, so a
	// concurrent read cannot combine fields from different published receipts.
	next := (state &^ phaseMask) + phaseMask + 1
	e.state.Store(next | phaseWrite)
	e.span.Store(uint64(r.Start)<<32 | uint64(r.End))
	e.proof.Store(uint64(r.Column)<<32 | uint64(r.Lookahead))
	e.state.Store(next | phaseLive)
	return cost, true
}

func (s *Store) Clear(slot int) {
	e := s.entry(slot)
	if e != nil {
		state := e.state.Load()
		if state&phaseMask == phaseLive {
			s.count--
		}
		e.state.Store((state &^ phaseMask) + phaseMask + 1)
	}
}

// Stage keeps producer proof state in the same dense slot as its eventual
// receipt. Drafts cannot be read by Get. Copied receipts retain their maximum
// extent only if every producer authenticates the final geometry.
func (s *Store) Stage(slot int, r Record, available int64) (int64, bool) {
	e, cost := s.ensure(slot, available)
	if e == nil {
		return 0, false
	}
	state := e.state.Load()
	if staged(state) {
		return cost, true
	}
	if prior, ok := s.Get(slot); ok && prior.Start == r.Start && prior.End == r.End && prior.Column == r.Column {
		r.Lookahead = max(r.Lookahead, prior.Lookahead)
	}
	if state&phaseMask == phaseLive {
		s.count--
	}
	next := (state &^ phaseMask) + phaseMask + 1
	e.state.Store(next | phaseWrite)
	e.span.Store(uint64(r.Start)<<32 | uint64(r.End))
	e.proof.Store(uint64(r.Column)<<32 | uint64(r.Lookahead))
	e.state.Store(next | phaseUnseen)
	return cost, true
}

func staged(state uint64) bool {
	phase := state & phaseMask
	return phase >= phaseUnseen && phase <= phaseUnknown
}

func (s *Store) IsStaged(slot int) bool {
	e := s.entry(slot)
	return e != nil && staged(e.state.Load())
}

// Observe combines all raw projections of a public node. One unknown producer
// defeats every known producer, regardless of observation order.
func (s *Store) Observe(slot int, frontier uint32, valid bool) {
	e := s.entry(slot)
	if e == nil {
		return
	}
	state := e.state.Load()
	if !staged(state) || state&phaseMask == phaseUnknown {
		return
	}
	end := uint32(e.span.Load())
	if !valid || frontier < end {
		e.state.Store(state&^phaseMask | phaseUnknown)
		return
	}
	proof := e.proof.Load()
	e.proof.Store(proof&0xffffffff00000000 | uint64(max(uint32(proof), frontier-end)))
	e.state.Store(state&^phaseMask | phaseSeen)
}

// Finish publishes a fully authenticated draft or discards an unknown/unseen
// one. It allocates nothing, including for zero lookahead.
func (s *Store) Finish(slot int) {
	e := s.entry(slot)
	if e == nil {
		return
	}
	state := e.state.Load()
	if !staged(state) {
		return
	}
	next := (state &^ phaseMask) + phaseMask + 1
	if state&phaseMask == phaseSeen {
		s.count++
		next |= phaseLive
	}
	e.state.Store(next)
}

func (s *Store) RangeStaged(visit func(int)) {
	d := s.directory.Load()
	if d == nil {
		return
	}
	for i := range d.pages {
		p := d.pages[i].Load()
		if p == nil {
			continue
		}
		for j := range p {
			if staged(p[j].state.Load()) {
				visit(i*pageSlots + j)
			}
		}
	}
}

func (s *Store) Range(visit func(int, Record)) {
	d := s.directory.Load()
	if d == nil {
		return
	}
	for i := range d.pages {
		p := d.pages[i].Load()
		if p == nil {
			continue
		}
		for j := range p {
			slot := i*pageSlots + j
			if r, ok := s.Get(slot); ok {
				visit(slot, r)
			}
		}
	}
}

// Reset runs only after the last arena reference is released. Bounded warm
// storage is pointer-free and contains no receipts for the next parse.
func (s *Store) Reset(retainBytes int64) {
	s.count = 0
	if s.bytes > retainBytes {
		s.directory.Store(nil)
		s.bytes = 0
		return
	}
	if d := s.directory.Load(); d != nil {
		for i := range d.pages {
			p := d.pages[i].Load()
			if p != nil {
				clear(p[:])
			}
		}
	}
}
