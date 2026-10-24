package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
	"unsafe"
)

type derivationEnumerationFrame struct {
	links    []linkRecord
	linkIDs  []LinkID
	paths    []Derivation
	payloads [][]SubtreeID
}

// DerivationScratch owns temporary path enumeration storage. Results returned
// by DerivationsWithScratch remain valid until Reset or the next enumeration.
// A caller must not use the scratch concurrently, retain borrowed results past
// that boundary, or mutate Core or scratch from its poll callback.
type DerivationScratch struct {
	frames  []derivationEnumerationFrame
	colors  []uint8
	reverse []LinkID
	inUse   bool
	work    uint64
}

func (s *DerivationScratch) poll(poll func() error) error {
	s.work++
	if s.work&255 == 0 {
		return poll()
	}
	return nil
}

// RetainedBytes accounts for every backing array, including inactive frames.
func (s *DerivationScratch) RetainedBytes() uint64 {
	if s == nil {
		return 0
	}
	total := uint64(0)
	add := func(n int, size uintptr) {
		if uint64(n) > (math.MaxUint64-total)/uint64(size) {
			total = math.MaxUint64
			return
		}
		total += uint64(n) * uint64(size)
	}
	add(cap(s.frames), unsafe.Sizeof(derivationEnumerationFrame{}))
	add(cap(s.colors), 1)
	add(cap(s.reverse), unsafe.Sizeof(LinkID(0)))
	for _, f := range s.frames[:cap(s.frames)] {
		add(cap(f.links), unsafe.Sizeof(linkRecord{}))
		add(cap(f.linkIDs), unsafe.Sizeof(LinkID(0)))
		add(cap(f.paths), unsafe.Sizeof(Derivation{}))
		add(cap(f.payloads), unsafe.Sizeof([]SubtreeID(nil)))
		for _, payloads := range f.payloads {
			add(cap(payloads), unsafe.Sizeof(SubtreeID(0)))
		}
	}
	return total
}

// Reset expires borrowed results and clears logical graph state. Oversized
// storage is dropped, so one large enumeration cannot grow later leases.
func (s *DerivationScratch) Reset() {
	if s == nil || s.inUse {
		return
	}
	if s.RetainedBytes() > 256*1024 {
		*s = DerivationScratch{}
		return
	}
	clear(s.colors)
	s.colors = s.colors[:0]
	s.reverse = s.reverse[:0]
	s.work = 0
	for i := range s.frames {
		f := &s.frames[i]
		clear(f.paths[:cap(f.paths)])
		f.paths = f.paths[:0]
		f.links = f.links[:0]
		f.linkIDs = f.linkIDs[:0]
		for j := range f.payloads {
			clear(f.payloads[j])
			f.payloads[j] = f.payloads[j][:0]
		}
	}
}

func (s *DerivationScratch) frame(depth int) *derivationEnumerationFrame {
	for len(s.frames) <= depth {
		s.frames = append(s.frames, derivationEnumerationFrame{})
	}
	return &s.frames[depth]
}

func (s *DerivationScratch) appendPath(depth int, prefix Derivation, link *linkRecord) error {
	f := s.frame(depth)
	index := len(f.paths)
	for len(f.payloads) <= index {
		f.payloads = append(f.payloads, nil)
	}
	score, err := checkedAddScore(prefix.Score, link.scoreDelta)
	if err != nil {
		return err
	}
	path := Derivation{Score: score, BranchOrder: prefix.BranchOrder, HasBranchOrder: prefix.HasBranchOrder}
	if len(prefix.Payloads) != 0 || link.payload != 0 {
		payloads := append(f.payloads[index][:0], prefix.Payloads...)
		if link.payload != 0 {
			payloads = append(payloads, link.payload)
		}
		f.payloads[index] = payloads
		path.Payloads = payloads
	}
	if link.hasOrder() {
		path.BranchOrder, path.HasBranchOrder = link.order, true
	}
	f.paths = append(f.paths, path)
	return nil
}

func (s *DerivationScratch) walk(c *Core, id NodeID, depth int, poll func() error) ([]Derivation, error) {
	if uint64(id) < uint64(len(s.colors)) && s.colors[id] != 0 {
		return nil, errors.New("parser-core phase zero: graph cycle")
	}
	n, nPaths, err := c.nodeWithPathCount(id)
	if err != nil {
		return nil, err
	}
	f := s.frame(depth)
	f.paths = f.paths[:0]
	if n.linkCount == 0 {
		if nPaths != 1 {
			return nil, errors.New("parser-core phase zero: malformed seed path count")
		}
		f.paths = append(f.paths, Derivation{})
		return f.paths, nil
	}
	s.colors[id] = 1
	defer func() { s.colors[id] = 0 }()
	// Validate the complete adjacency before following predecessors, just as
	// nodeLinks does. Reuse its buffers and preserve its exact error order.
	f.links, f.linkIDs = f.links[:0], f.linkIDs[:0]
	if n.linkCount > 16 {
		// Wide diagnostic frontiers keep the linear-time map validator.
		// A bounded scan is cheaper for the usual narrow adjacency only.
		f.links, err = c.nodeLinks(*n)
		if err != nil {
			return nil, err
		}
	} else {
		for linkID := LinkID(n.firstLink); linkID != 0; {
			for _, seen := range f.linkIDs {
				if seen == linkID {
					return nil, errors.New("parser-core phase zero: adjacency cycle")
				}
			}
			if uint64(linkID) > uint64(c.links.Len()) {
				return nil, errors.New("parser-core phase zero: link adjacency out of range")
			}
			link := c.links.At(int(linkID - 1))
			if err := link.validateShape(); err != nil {
				return nil, err
			}
			f.links = append(f.links, link)
			f.linkIDs = append(f.linkIDs, linkID)
			if uint64(len(f.links)) > uint64(n.linkCount) {
				return nil, errors.New("parser-core phase zero: adjacency exceeds recorded link count")
			}
			linkID = link.next
		}
		if uint32(len(f.links)) != n.linkCount {
			return nil, errors.New("parser-core phase zero: adjacency shorter than recorded link count")
		}
	}
	links := f.links
	if err := s.poll(poll); err != nil {
		return nil, err
	}
	for i := len(links) - 1; i >= 0; i-- {
		link := &links[i]
		prefixes, err := s.walk(c, link.prev, depth+1, poll)
		if err != nil {
			return nil, err
		}
		for _, prefix := range prefixes {
			// Recursion may grow frames; resolve the parent by index again.
			if uint64(len(s.frames[depth].paths)) >= c.limits.MaxDerivations {
				return nil, ErrDerivationEnumerationCap
			}
			if err := s.appendPath(depth, prefix, link); err != nil {
				return nil, err
			}
			if err := s.poll(poll); err != nil {
				return nil, err
			}
		}
	}
	paths := s.frames[depth].paths
	if nPaths != math.MaxUint64 && uint64(len(paths)) != nPaths {
		return nil, fmt.Errorf("parser-core phase zero: path-count mismatch: enumerated %d, recorded %d", len(paths), nPaths)
	}
	return paths, nil
}

// DerivationsWithScratch enumerates the same exact alternatives as Derivations
// into caller-owned storage. It preserves payloads, forward score arithmetic,
// insertion order, branch order, telemetry validation and error precedence.
// The returned slices are borrowed until scratch.Reset or the next call.
func (c *Core) DerivationsWithScratch(head Head, scratch *DerivationScratch, poll func() error) (paths []Derivation, err error) {
	if c == nil || scratch == nil {
		return nil, errors.New("parser-core phase zero: derivation enumeration requires a core and scratch")
	}
	if scratch.inUse {
		return nil, errors.New("parser-core phase zero: derivation scratch is already in use")
	}
	scratch.Reset()
	scratch.inUse = true
	defer func() {
		scratch.inUse = false
		if err != nil {
			scratch.Reset()
		}
	}()
	if poll == nil {
		poll = func() error { return nil }
	}
	if err := poll(); err != nil {
		return nil, err
	}
	_, nPaths, err := c.nodeWithPathCount(head.Node)
	if err != nil {
		return nil, err
	}
	if nPaths == 1 {
		var exact bool
		scratch.reverse, exact, err = c.appendSingleDerivationLinks(head.Node, scratch.reverse[:0])
		if err != nil {
			return nil, err
		}
		if exact {
			f := scratch.frame(0)
			if len(f.payloads) == 0 {
				f.payloads = append(f.payloads, nil)
			}
			path := Derivation{Payloads: f.payloads[0][:0]}
			if path.Payloads == nil {
				path.Payloads = []SubtreeID{}
			}
			for i := len(scratch.reverse) - 1; i >= 0; i-- {
				link := c.links.At(int(scratch.reverse[i] - 1))
				path.Score, err = checkedAddScore(path.Score, link.scoreDelta)
				if err != nil {
					return nil, err
				}
				if link.payload != 0 {
					path.Payloads = append(path.Payloads, link.payload)
				}
				if link.hasOrder() {
					path.BranchOrder, path.HasBranchOrder = link.order, true
				}
			}
			f.payloads[0] = path.Payloads
			f.paths = append(f.paths[:0], path)
			if err := poll(); err != nil {
				return nil, err
			}
			return f.paths, nil
		}
	}
	count := c.nodes.Len() + 1
	if cap(scratch.colors) < count {
		scratch.colors = make([]uint8, count)
	} else {
		scratch.colors = scratch.colors[:count]
	}
	paths, err = scratch.walk(c, head.Node, 0, poll)
	if err == nil {
		err = poll()
	}
	if err != nil {
		return nil, err
	}
	return paths, nil
}
