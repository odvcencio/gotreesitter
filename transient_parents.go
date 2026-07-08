package gotreesitter

import "unsafe"

type transientParentScratch struct {
	slabs             []transientParentSlab
	slabCursor        int
	allocatedBytes    int64
	nodesAllocated    uint64
	nodesMaterialized uint64
	seen              map[*Node]*Node
	frames            []transientParentFrame
}

type transientParentSlab struct {
	data []Node
	used int
}

type transientParentFrame struct {
	node    *Node
	visited bool
}

func (s *transientParentScratch) allocParent(arena *nodeArena, sym Symbol, named bool, children []*Node, productionID uint16, trackChildErrors bool) *Node {
	if s == nil {
		return newParentNodeInArenaNoLinksWithFieldSources(arena, sym, named, children, nil, nil, productionID, trackChildErrors)
	}
	if len(s.slabs) == 0 {
		capacity := max(defaultTransientParentSlabCap(), minArenaNodeCap)
		s.slabs = append(s.slabs, transientParentSlab{data: make([]Node, capacity)})
		s.allocatedBytes += nodeStructBytesForCap(capacity)
		s.slabCursor = 0
	}
	if s.slabCursor < 0 || s.slabCursor >= len(s.slabs) {
		s.slabCursor = 0
	}
	for i := s.slabCursor; ; i++ {
		if i >= len(s.slabs) {
			lastCap := len(s.slabs[len(s.slabs)-1].data)
			capacity := max(lastCap*2, minArenaNodeCap)
			s.slabs = append(s.slabs, transientParentSlab{data: make([]Node, capacity)})
			s.allocatedBytes += nodeStructBytesForCap(capacity)
		}
		slab := &s.slabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		slab.used++
		s.slabCursor = i
		n := &slab.data[idx]
		n.symbol = sym
		n.setNamed(named)
		n.children = children
		n.fieldIDs = nil
		n.fieldSources = nil
		n.productionID = productionID
		n.childIndex = -1
		populateParentNodeNoLinks(n, children, trackChildErrors)
		nodeInitEquivVersion(n)
		s.nodesAllocated++
		return n
	}
}

func defaultTransientParentSlabCap() int {
	size := int(unsafe.Sizeof(Node{}))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := fullParseArenaSlab / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func nodeStructBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(Node{}))
}

func (s *transientParentScratch) owns(node *Node) bool {
	if s == nil || node == nil {
		return false
	}
	ptr := uintptr(unsafe.Pointer(node))
	size := unsafe.Sizeof(Node{})
	for i := range s.slabs {
		data := s.slabs[i].data
		if len(data) == 0 {
			continue
		}
		start := uintptr(unsafe.Pointer(&data[0]))
		end := start + uintptr(len(data))*size
		if ptr < start || ptr >= end {
			continue
		}
		return (ptr-start)%size == 0
	}
	return false
}

func (s *transientParentScratch) materializeEntries(entries []stackEntry, arena *nodeArena, childScratch *transientChildScratch) {
	s.materializeEntriesUntil(entries, arena, childScratch, nil)
}

func (s *transientParentScratch) materializeEntriesUntil(entries []stackEntry, arena *nodeArena, childScratch *transientChildScratch, p *Parser) ParseStopReason {
	if s == nil || len(entries) == 0 || arena == nil {
		return ParseStopNone
	}
	defer s.clearMaterializeScratch()
	roots := make([]*Node, 0, len(entries))
	for i := range entries {
		if i&63 == 0 {
			if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
				return reason
			}
		}
		if node := stackEntryNode(entries[i]); node != nil {
			roots = append(roots, node)
		}
	}
	if reason := s.materializeNodesUntil(roots, arena, childScratch, p); parseStopReasonIsTerminal(reason) {
		return reason
	}
	for i := range entries {
		if i&63 == 0 {
			if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
				return reason
			}
		}
		node := stackEntryNode(entries[i])
		if node == nil {
			continue
		}
		if replacement := s.transientReplacement(node); replacement != nil {
			setStackEntryNode(&entries[i], replacement)
			continue
		}
		if replacement, ok := s.seen[node]; ok {
			setStackEntryNode(&entries[i], replacement)
		}
	}
	return ParseStopNone
}

func (s *transientParentScratch) materializeNodeSliceUntil(nodes []*Node, arena *nodeArena, childScratch *transientChildScratch, p *Parser) ParseStopReason {
	if s == nil || len(nodes) == 0 || arena == nil {
		return ParseStopNone
	}
	defer s.clearMaterializeScratch()
	if reason := s.materializeNodesUntil(nodes, arena, childScratch, p); parseStopReasonIsTerminal(reason) {
		return reason
	}
	for i := range nodes {
		if i&63 == 0 {
			if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
				return reason
			}
		}
		if replacement := s.transientReplacement(nodes[i]); replacement != nil {
			nodes[i] = replacement
			continue
		}
		if replacement, ok := s.seen[nodes[i]]; ok {
			nodes[i] = replacement
		}
	}
	return ParseStopNone
}

func (s *transientParentScratch) materializeNodesUntil(nodes []*Node, arena *nodeArena, childScratch *transientChildScratch, p *Parser) ParseStopReason {
	if s == nil || len(nodes) == 0 || arena == nil {
		return ParseStopNone
	}
	frames := s.frames[:0]
	defer func() {
		clear(frames[:cap(frames)])
		s.frames = frames[:0]
	}()
	for i := range nodes {
		if i&63 == 0 {
			if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
				return reason
			}
		}
		if nodes[i] != nil {
			frames = append(frames, transientParentFrame{node: nodes[i]})
		}
	}
	visited := 0
	for len(frames) > 0 {
		if visited&63 == 0 {
			if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
				return reason
			}
		}
		visited++
		frame := frames[len(frames)-1]
		frames = frames[:len(frames)-1]
		n := frame.node
		if n == nil {
			continue
		}
		if frame.visited {
			if reason := s.materializeVisitedNodeUntil(n, arena, childScratch, p); parseStopReasonIsTerminal(reason) {
				return reason
			}
			continue
		}
		if s.owns(n) {
			if n.parent != nil {
				continue
			}
			n.parent = n
		} else {
			if len(n.children) == 0 {
				continue
			}
			if s.seen == nil {
				s.seen = make(map[*Node]*Node)
			}
			if _, ok := s.seen[n]; ok {
				continue
			}
		}
		frames = append(frames, transientParentFrame{node: n, visited: true})
		for i := len(n.children) - 1; i >= 0; i-- {
			child := n.children[i]
			if child != nil {
				frames = append(frames, transientParentFrame{node: child})
			}
		}
	}
	return ParseStopNone
}

func (s *transientParentScratch) materializeVisitedNodeUntil(n *Node, arena *nodeArena, childScratch *transientChildScratch, p *Parser) ParseStopReason {
	if n == nil {
		return ParseStopNone
	}
	children := n.children
	if len(children) > 0 {
		out := children
		if childScratch != nil && childScratch.owns(children) {
			out = arena.allocNodeSliceNoClear(len(children))
			copy(out, children)
			childScratch.slicesMaterialized++
			childScratch.pointersMaterialized += uint64(len(children))
		}
		for i, child := range out {
			if i&63 == 0 {
				if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
					return reason
				}
			}
			if replacement := s.transientReplacement(child); replacement != nil {
				out[i] = replacement
				continue
			}
			if replacement, ok := s.seen[child]; ok {
				out[i] = replacement
			}
		}
		n.children = out
	}
	if !s.owns(n) {
		s.seen[n] = n
		return ParseStopNone
	}

	clone := arena.allocNodeFast()
	clone.ownerArena = arena
	clone.symbol = n.symbol
	clone.children = n.children
	clone.fieldIDs = n.fieldIDs
	clone.fieldSources = n.fieldSources
	clone.startPoint = n.startPoint
	clone.endPoint = n.endPoint
	clone.startByte = n.startByte
	clone.endByte = n.endByte
	clone.parseState = n.parseState
	clone.preGotoState = n.preGotoState
	clone.productionID = n.productionID
	clone.flags = n.flags
	clone.childIndex = -1
	nodeInitEquivVersion(clone)
	arena.recordParentNodeConstructed(len(clone.children), clone.fieldIDs, clone.fieldSources, len(clone.fieldSources) > 0, true, false)
	s.nodesMaterialized++
	n.parent = clone
	return ParseStopNone
}

func (s *transientParentScratch) transientReplacement(n *Node) *Node {
	if s == nil || n == nil || n.parent == nil || n.parent == n || !s.owns(n) {
		return nil
	}
	return n.parent
}

func (s *transientParentScratch) reset() {
	if s == nil {
		return
	}
	for i := range s.slabs {
		slab := &s.slabs[i]
		used := slab.used
		if used > len(slab.data) {
			used = len(slab.data)
		}
		clear(slab.data[:used])
		slab.used = 0
	}
	s.slabCursor = 0
	s.nodesAllocated = 0
	s.nodesMaterialized = 0
	s.clearMaterializeScratch()
}

func (s *transientParentScratch) resetForRelease() {
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
			s.slabs[i] = transientParentSlab{}
		}
		s.slabs = nil
		s.allocatedBytes = 0
	}
}

func (s *transientParentScratch) clearMaterializeScratch() {
	if s == nil {
		return
	}
	if s.seen != nil {
		for node := range s.seen {
			delete(s.seen, node)
		}
	}
	if cap(s.frames) > 0 {
		clear(s.frames[:cap(s.frames)])
		s.frames = s.frames[:0]
	}
}
