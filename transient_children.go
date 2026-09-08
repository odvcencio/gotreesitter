package gotreesitter

import "unsafe"

type transientChildScratch struct {
	slabs                []childSliceSlab
	slabCursor           int
	allocatedBytes       int64
	slicesAllocated      uint64
	pointersAllocated    uint64
	slicesMaterialized   uint64
	pointersMaterialized uint64
}

func (s *transientChildScratch) alloc(n int) []*Node {
	if n <= 0 {
		return nil
	}
	if s == nil {
		return make([]*Node, n)
	}
	s.slicesAllocated++
	s.pointersAllocated += uint64(n)
	if len(s.slabs) == 0 {
		capacity := max(defaultChildSliceCap(arenaClassFull), n)
		s.slabs = append(s.slabs, childSliceSlab{data: make([]*Node, capacity)})
		s.allocatedBytes += childSliceBytesForCap(capacity)
		s.slabCursor = 0
	}
	if s.slabCursor < 0 || s.slabCursor >= len(s.slabs) {
		s.slabCursor = 0
	}
	for i := s.slabCursor; ; i++ {
		if i >= len(s.slabs) {
			lastCap := len(s.slabs[len(s.slabs)-1].data)
			capacity := boundedNextSlabCap(lastCap, n, int(unsafe.Sizeof((*Node)(nil))))
			s.slabs = append(s.slabs, childSliceSlab{data: make([]*Node, capacity)})
			s.allocatedBytes += childSliceBytesForCap(capacity)
		}
		slab := &s.slabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		s.slabCursor = i
		return slab.data[start:slab.used]
	}
}

func (s *transientChildScratch) owns(children []*Node) bool {
	if s == nil || len(children) == 0 {
		return false
	}
	ptr := uintptr(unsafe.Pointer(&children[0]))
	for i := range s.slabs {
		data := s.slabs[i].data
		if len(data) == 0 {
			continue
		}
		start := uintptr(unsafe.Pointer(&data[0]))
		end := start + uintptr(len(data))*unsafe.Sizeof((*Node)(nil))
		if ptr >= start && ptr < end {
			return true
		}
	}
	return false
}

func (s *transientChildScratch) materializeNodeUntil(root *Node, arena *nodeArena, scratch *[]*Node, p *Parser) ParseStopReason {
	if s == nil || root == nil || arena == nil {
		return ParseStopNone
	}
	var stack []*Node
	if scratch != nil {
		stack = (*scratch)[:0]
	} else {
		var local [64]*Node
		stack = local[:0]
	}
	defer func() {
		if scratch != nil {
			*scratch = stack[:0]
		}
	}()
	stack = append(stack, root)
	visited := 0
	for len(stack) > 0 {
		if visited&63 == 0 {
			if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
				return reason
			}
		}
		visited++
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}
		children := n.children
		if len(children) == 0 {
			continue
		}
		if s.owns(children) {
			out := arena.allocNodeSliceNoClear(len(children))
			copy(out, children)
			n.children = out
			children = out
			s.slicesMaterialized++
			s.pointersMaterialized += uint64(len(children))
		}
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	return ParseStopNone
}

func (s *transientChildScratch) reset() {
	if s == nil {
		return
	}
	for i := range s.slabs {
		slab := &s.slabs[i]
		if slab.used > 0 {
			clear(slab.data[:slab.used])
			slab.used = 0
		}
	}
	s.slabCursor = 0
	s.slicesAllocated = 0
	s.pointersAllocated = 0
	s.slicesMaterialized = 0
	s.pointersMaterialized = 0
}

func (s *transientChildScratch) usedBytes() int64 {
	if s == nil {
		return 0
	}
	var used int64
	for i := range s.slabs {
		used += childSliceBytesForCap(s.slabs[i].used)
	}
	return used
}

func (s *transientChildScratch) recycleForParse() {
	if s == nil {
		return
	}
	slicesAllocated := s.slicesAllocated
	pointersAllocated := s.pointersAllocated
	slicesMaterialized := s.slicesMaterialized
	pointersMaterialized := s.pointersMaterialized
	s.reset()
	s.slicesAllocated = slicesAllocated
	s.pointersAllocated = pointersAllocated
	s.slicesMaterialized = slicesMaterialized
	s.pointersMaterialized = pointersMaterialized
}

// transientChildRetainedCapForSource bounds the transient child slabs a
// parse may inherit from the pool, in child pointers. See
// transientParentRetainedCapForSource.
func transientChildRetainedCapForSource(sourceLen int) int {
	return max(defaultChildSliceCap(arenaClassFull), 4*parseFullArenaInitialNodeCapacity(sourceLen))
}

// trimForSource drops inherited slabs whose total capacity exceeds the bound
// for this source. The slabs are already reset, so no live slice points into
// them.
func (s *transientChildScratch) trimForSource(sourceLen int) {
	if s == nil || len(s.slabs) == 0 {
		return
	}
	totalCap := 0
	for i := range s.slabs {
		totalCap += len(s.slabs[i].data)
	}
	if totalCap <= transientChildRetainedCapForSource(sourceLen) {
		return
	}
	for i := range s.slabs {
		s.slabs[i] = childSliceSlab{}
	}
	s.slabs = nil
	s.slabCursor = 0
	s.allocatedBytes = 0
}

func (s *transientChildScratch) resetForRelease() {
	if s == nil {
		return
	}
	s.reset()
	totalCap := 0
	for i := range s.slabs {
		totalCap += len(s.slabs[i].data)
	}
	if totalCap > maxRetainedFullSliceCap {
		for i := range s.slabs {
			s.slabs[i] = childSliceSlab{}
		}
		s.slabs = nil
		s.allocatedBytes = 0
	}
}
