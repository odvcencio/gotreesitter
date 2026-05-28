package gotreesitter

import "unsafe"

const (
	defaultGSSNodeSlabCap   = 4 * 1024
	fullParseGSSNodeSlabCap = 32 * 1024
	maxRetainedGSSNodes     = 256 * 1024
)

type gssNode struct {
	entry stackEntry
	prev  *gssNode
	depth int
	hash  uint64
}

// gssStack is a shared-prefix stack foundation for future GLR work.
// Cloning is O(1): clones share the same head pointer until diverging pushes.
type gssStack struct {
	head *gssNode
}

type gssScratch struct {
	slabs             []gssNodeSlab
	slabCursor        int
	initialCap        int
	skipClear         bool
	usedTotal         int
	allocatedBytes    int64
	singleStackMode   bool
	singleStackAllocs uint64
	multiStackAllocs  uint64
	audit             *runtimeAudit
}

type gssNodeSlab struct {
	data []gssNode
	used int
}

const (
	// 64-bit FNV-1a constants.
	gssHashSeed        uint64 = 1469598103934665603
	gssHashPrime       uint64 = 1099511628211
	gssNilNodeSentinel uint64 = 0xff51afd7ed558ccd
)

func gssEntryHash(prev uint64, entry stackEntry) uint64 {
	h := prev ^ uint64(entry.state)
	h *= gssHashPrime

	if entry.node == nil {
		h ^= gssNilNodeSentinel
		h *= gssHashPrime
		return h
	}

	var symbol Symbol
	var startByte, endByte uint32
	var parseState StateID
	var productionID uint16
	var childCount int
	var flags nodeFlags

	switch entry.kind {
	case stackEntryKindNode:
		n := (*Node)(entry.node)
		symbol = n.symbol
		startByte = n.startByte
		endByte = n.endByte
		parseState = n.parseState
		productionID = n.productionID
		childCount = nodeChildCountNoMaterialize(n)
		flags = n.flags
	case stackEntryKindNoTreeNode:
		n := (*noTreeNode)(entry.node)
		symbol = n.symbol
		startByte = n.startByte
		endByte = n.endByte
		parseState = n.parseState
		productionID = n.productionID
		flags = n.flags
	case stackEntryKindCompactFullLeaf:
		n := (*compactFullLeaf)(entry.node)
		symbol = n.symbol
		startByte = n.startByte
		endByte = n.endByte
		parseState = n.parseState
		productionID = n.productionID
		flags = n.flags
	case stackEntryKindPendingParent:
		n := (*pendingParent)(entry.node)
		symbol = n.symbol
		startByte = n.startByte
		endByte = n.endByte
		parseState = n.parseState
		productionID = n.productionID
		childCount = n.childEntryCount()
		flags = n.flags
	default:
		h ^= gssNilNodeSentinel
		h *= gssHashPrime
		return h
	}

	h ^= uint64(symbol)
	h *= gssHashPrime
	h ^= (uint64(startByte) << 32) | uint64(endByte)
	h *= gssHashPrime
	h ^= uint64(parseState)
	h *= gssHashPrime
	h ^= uint64(productionID)
	h *= gssHashPrime
	h ^= uint64(childCount)
	h *= gssHashPrime
	h ^= gssEntryFlagHash(flags)
	h *= gssHashPrime
	return h
}

func gssEntryFlagHash(flags nodeFlags) uint64 {
	var h uint64
	if flags&nodeFlagExtra != 0 {
		h |= 1
	}
	if flags&nodeFlagNamed != 0 {
		h |= 1 << 1
	}
	if flags&nodeFlagHasError != 0 {
		h |= 1 << 2
	}
	if flags&nodeFlagMissing != 0 {
		h |= 1 << 3
	}
	return h
}

func gssNodeHash(n *gssNode) uint64 {
	if n == nil {
		return gssHashSeed
	}
	if n.hash != 0 {
		return n.hash
	}

	var local [32]*gssNode
	pending := local[:0]
	for cur := n; cur != nil && cur.hash == 0; cur = cur.prev {
		pending = append(pending, cur)
	}
	prevHash := gssHashSeed
	if len(pending) < n.depth {
		prev := pending[len(pending)-1].prev
		if prev != nil {
			prevHash = prev.hash
		}
	}
	for i := len(pending) - 1; i >= 0; i-- {
		h := gssEntryHash(prevHash, pending[i].entry)
		if h == 0 {
			h = 1
		}
		pending[i].hash = h
		prevHash = h
	}
	return n.hash
}

func newGSSStack(initial StateID, scratch *gssScratch) gssStack {
	return buildGSSStack([]stackEntry{{state: initial}}, scratch)
}

func buildGSSStack(entries []stackEntry, scratch *gssScratch) gssStack {
	var s gssStack
	for i := range entries {
		s.pushEntry(entries[i], scratch)
	}
	return s
}

func (s gssStack) clone() gssStack {
	return s
}

func (s gssStack) len() int {
	if s.head == nil {
		return 0
	}
	return s.head.depth
}

func (s gssStack) top() stackEntry {
	if s.head == nil {
		return stackEntry{}
	}
	return s.head.entry
}

func (s gssStack) byteOffset() uint32 {
	for n := s.head; n != nil; n = n.prev {
		if stackEntryHasNode(n.entry) {
			return stackEntryNodeEndByte(n.entry)
		}
	}
	return 0
}

func (s *gssStack) push(state StateID, node *Node, scratch *gssScratch) {
	s.pushEntry(newStackEntryNode(state, node), scratch)
}

func (s *gssStack) pushEntry(entry stackEntry, scratch *gssScratch) {
	var depth int
	if s.head != nil {
		depth = s.head.depth + 1
	} else {
		depth = 1
	}
	n := scratch.allocNode(entry, s.head, depth)
	s.head = n
}

func (s *gssStack) truncate(depth int) bool {
	if depth <= 0 {
		s.head = nil
		return true
	}
	if s.head == nil {
		return depth == 0
	}
	if depth > s.head.depth {
		return false
	}
	keep := s.head
	for keep != nil && keep.depth > depth {
		keep = keep.prev
	}
	if keep == nil || keep.depth != depth {
		return false
	}
	s.head = keep
	return true
}

func (s gssStack) materialize(dst []stackEntry) []stackEntry {
	n := s.len()
	if n == 0 {
		return dst[:0]
	}
	if cap(dst) < n {
		dst = make([]stackEntry, n)
	} else {
		dst = dst[:n]
	}
	i := n - 1
	for node := s.head; node != nil && i >= 0; node = node.prev {
		dst[i] = node.entry
		i--
	}
	if i >= 0 {
		// Invariant violation: depth metadata does not match linked-list length.
		// This indicates internal GSS corruption and is not recoverable here.
		panic("gssStack.materialize: corrupt depth metadata")
	}
	return dst
}

func (s *gssScratch) allocNode(entry stackEntry, prev *gssNode, depth int) *gssNode {
	hash := uint64(0)
	if s == nil || !s.singleStackMode {
		prevHash := gssHashSeed
		if prev != nil {
			prevHash = gssNodeHash(prev)
		}
		hash = gssEntryHash(prevHash, entry)
		if hash == 0 {
			hash = 1
		}
	}

	if s == nil {
		return &gssNode{entry: entry, prev: prev, depth: depth, hash: hash}
	}

	// Fast path: current slab has space. Kept small for inlining.
	if cur := s.slabCursor; cur >= 0 && cur < len(s.slabs) {
		slab := &s.slabs[cur]
		if slab.used < len(slab.data) {
			idx := slab.used
			slab.used++
			s.usedTotal++
			if s.singleStackMode {
				s.singleStackAllocs++
			} else {
				s.multiStackAllocs++
			}
			n := &slab.data[idx]
			n.entry = entry
			n.prev = prev
			n.depth = depth
			n.hash = hash
			return n
		}
	}
	return s.allocNodeSlow(entry, prev, depth, hash)
}

func (s *gssScratch) allocNodeSlow(entry stackEntry, prev *gssNode, depth int, hash uint64) *gssNode {
	if len(s.slabs) == 0 {
		capacity := defaultGSSNodeSlabCap
		if s.initialCap > capacity {
			capacity = s.initialCap
		}
		s.slabs = append(s.slabs, gssNodeSlab{data: make([]gssNode, capacity)})
		s.allocatedBytes += gssNodeBytesForCap(capacity)
		s.slabCursor = 0
	}
	if s.slabCursor < 0 || s.slabCursor >= len(s.slabs) {
		s.slabCursor = 0
	}
	for i := s.slabCursor; ; i++ {
		if i >= len(s.slabs) {
			lastCap := defaultGSSNodeSlabCap
			if len(s.slabs) > 0 {
				lastCap = len(s.slabs[len(s.slabs)-1].data)
			}
			capacity := lastCap * 2
			if capacity < defaultGSSNodeSlabCap {
				capacity = defaultGSSNodeSlabCap
			}
			s.slabs = append(s.slabs, gssNodeSlab{data: make([]gssNode, capacity)})
			s.allocatedBytes += gssNodeBytesForCap(capacity)
		}
		slab := &s.slabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		slab.used++
		s.usedTotal++
		s.slabCursor = i
		if s.singleStackMode {
			s.singleStackAllocs++
		} else {
			s.multiStackAllocs++
		}
		n := &slab.data[idx]
		n.entry = entry
		n.prev = prev
		n.depth = depth
		n.hash = hash
		if s.audit != nil {
			s.audit.recordGSSAlloc(n)
		}
		return n
	}
}

func (s *gssScratch) reset() {
	if len(s.slabs) == 0 {
		s.singleStackMode = false
		s.singleStackAllocs = 0
		s.multiStackAllocs = 0
		s.skipClear = false
		s.allocatedBytes = 0
		s.audit = nil
		return
	}
	total := 0
	for i := range s.slabs {
		total += len(s.slabs[i].data)
	}
	if total > maxRetainedGSSNodes {
		keepFrom := len(s.slabs) - 1
		retained := len(s.slabs[keepFrom].data)
		for keepFrom > 0 {
			next := retained + len(s.slabs[keepFrom-1].data)
			if next > maxRetainedGSSNodes {
				break
			}
			keepFrom--
			retained = next
		}
		if keepFrom > 0 {
			oldLen := len(s.slabs)
			copy(s.slabs, s.slabs[keepFrom:])
			newLen := oldLen - keepFrom
			for i := newLen; i < oldLen; i++ {
				s.slabs[i] = gssNodeSlab{}
			}
			s.slabs = s.slabs[:newLen]
		}
	}
	for i := range s.slabs {
		used := s.slabs[i].used
		if used > len(s.slabs[i].data) {
			used = len(s.slabs[i].data)
		}
		clear(s.slabs[i].data[:used])
		s.slabs[i].used = 0
	}
	s.slabCursor = 0
	s.skipClear = false
	s.usedTotal = 0
	s.singleStackMode = false
	s.singleStackAllocs = 0
	s.multiStackAllocs = 0
	s.audit = nil
	s.recomputeAllocatedBytes()
}

func (s *glrStack) toGSS(scratch *gssScratch) gssStack {
	if s.gss.head != nil {
		return s.gss.clone()
	}
	return buildGSSStack(s.entries, scratch)
}

func gssNodeBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(gssNode{}))
}

func (s *gssScratch) recomputeAllocatedBytes() {
	if s == nil {
		return
	}
	var total int64
	for i := range s.slabs {
		total += gssNodeBytesForCap(len(s.slabs[i].data))
	}
	s.allocatedBytes = total
}
