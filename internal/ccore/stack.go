package ccore

// The parse stack: a graph-structured stack in an index arena with a free list,
// C's reference counts and caps (C/stack.c).

type stackLink struct { // C StackLink
	node    int32
	sub     Handle
	pending bool
}

type stackNode struct { // C StackNode
	links     [maxLinkCount]stackLink
	position  Length
	refCount  uint32
	errorCost uint32
	nodeCount uint32
	dynPrec   int32
	state     uint16
	linkCount uint16
}

const (
	headActive uint8 = iota
	headPaused
	headHalted
)

type stackHead struct { // C StackHead (summary is recovery-only: not kept)
	node                 int32
	nodeCountAtLastError uint32
	lastExternal         Handle
	lookaheadWhenPaused  Handle
	status               uint8
	summary              []summaryEntry // recovery (ccore_stack_summary.go)
}

type stackSlice struct { // C StackSlice
	subtrees []Handle
	version  int
}

type stackIter struct { // C StackIterator
	node         int32
	subtrees     []Handle
	subtreeCount uint32
	pending      bool
}

func (p *Parser) nodeNew(prev int32, sub Handle, pending bool, state uint16) int32 { // C/stack.c stack_node_new
	var id int32
	if n := len(p.free); n > 0 {
		id = p.free[n-1]
		p.free = p.free[:n-1]
	} else {
		p.nodes = append(p.nodes, stackNode{})
		id = int32(len(p.nodes) - 1)
	}
	node := &p.nodes[id]
	*node = stackNode{refCount: 1, state: state}
	if prev >= 0 {
		pn := &p.nodes[prev]
		node.linkCount = 1
		node.links[0] = stackLink{node: prev, sub: sub, pending: pending}
		node.position, node.errorCost, node.dynPrec, node.nodeCount = pn.position, pn.errorCost, pn.dynPrec, pn.nodeCount
		if sub != 0 {
			node.errorCost += p.errorCost(sub)
			node.position = lenAdd(node.position, p.totalSize(sub))
			node.nodeCount += p.subtreeNodeCount(sub)
			node.dynPrec += p.dynPrec(sub)
		}
	}
	return id
}

func (p *Parser) subtreeNodeCount(sub Handle) uint32 { // C/stack.c stack__subtree_node_count
	c := p.visibleDescendants(sub)
	if p.visible(sub) {
		c++
	}
	if p.symbol(sub) == symErrorRepeat {
		c++
	}
	return c
}

func (p *Parser) nodeRetain(id int32) {
	if id >= 0 {
		p.nodes[id].refCount++
	}
}

func (p *Parser) nodeRelease(id int32) { // C/stack.c stack_node_release
	for id >= 0 {
		node := &p.nodes[id]
		node.refCount--
		if node.refCount > 0 {
			return
		}
		first := int32(-1)
		if node.linkCount > 0 {
			for i := int(node.linkCount) - 1; i > 0; i-- {
				p.nodeRelease(node.links[i].node)
			}
			first = node.links[0].node
		}
		p.free = append(p.free, id)
		id = first
	}
}

func (p *Parser) subtreeEquivalent(left, right Handle) bool { // C/stack.c stack__subtree_is_equivalent
	if left == right {
		return true
	}
	if left == 0 || right == 0 {
		return false
	}
	if p.symbol(left) != p.symbol(right) {
		return false
	}
	if p.errorCost(left) > 0 && p.errorCost(right) > 0 {
		return true
	}
	return p.padding(left).bytes == p.padding(right).bytes && p.size(left).bytes == p.size(right).bytes &&
		p.childCount(left) == p.childCount(right) && p.extra(left) == p.extra(right) &&
		p.extStateEq(left, right)
}

func (p *Parser) addLink(self int32, link stackLink) { // C/stack.c stack_node_add_link
	if link.node == self {
		return
	}
	for i := 0; i < int(p.nodes[self].linkCount); i++ {
		existing := &p.nodes[self].links[i]
		if p.subtreeEquivalent(existing.sub, link.sub) {
			if existing.node == link.node {
				if p.dynPrec(link.sub) > p.dynPrec(existing.sub) {
					existing.sub = link.sub
					p.nodes[self].dynPrec = p.nodes[link.node].dynPrec + p.dynPrec(link.sub)
				}
				return
			}
			en, ln := &p.nodes[existing.node], &p.nodes[link.node]
			if en.state == ln.state && en.position.bytes == ln.position.bytes && en.errorCost == ln.errorCost {
				target := existing.node
				for j := 0; j < int(p.nodes[link.node].linkCount); j++ {
					p.addLink(target, p.nodes[link.node].links[j])
				}
				dp := p.nodes[link.node].dynPrec
				if link.sub != 0 {
					dp += p.dynPrec(link.sub)
				}
				if dp > p.nodes[self].dynPrec {
					p.nodes[self].dynPrec = dp
				}
				return
			}
		}
	}
	node := &p.nodes[self]
	if node.linkCount == maxLinkCount {
		return
	}
	p.nodeRetain(link.node)
	nodeCount := p.nodes[link.node].nodeCount
	dp := p.nodes[link.node].dynPrec
	node = &p.nodes[self]
	node.links[node.linkCount] = link
	node.linkCount++
	if link.sub != 0 {
		nodeCount += p.subtreeNodeCount(link.sub)
		dp += p.dynPrec(link.sub)
	}
	if nodeCount > node.nodeCount {
		node.nodeCount = nodeCount
	}
	if dp > node.dynPrec {
		node.dynPrec = dp
	}
}

func (p *Parser) headDelete(h *stackHead) { // C/stack.c stack_head_delete
	if h.node >= 0 {
		p.nodeRelease(h.node)
	}
}

func (p *Parser) addVersion(original int, node int32) int { // C/stack.c ts_stack__add_version
	o := &p.heads[original]
	p.heads = append(p.heads, stackHead{node: node, nodeCountAtLastError: o.nodeCountAtLastError, lastExternal: o.lastExternal, status: headActive})
	p.nodeRetain(node)
	return len(p.heads) - 1
}

func (p *Parser) addSlice(original int, node int32, subtrees []Handle) { // C/stack.c ts_stack__add_slice
	for i := len(p.slices) - 1; i >= 0; i-- {
		v := p.slices[i].version
		if p.heads[v].node == node {
			p.slices = append(p.slices, stackSlice{})
			copy(p.slices[i+2:], p.slices[i+1:])
			p.slices[i+1] = stackSlice{subtrees: subtrees, version: v}
			return
		}
	}
	v := p.addVersion(original, node)
	p.slices = append(p.slices, stackSlice{subtrees: subtrees, version: v})
}

func (p *Parser) getBuf() []Handle {
	if n := len(p.bufs); n > 0 {
		b := p.bufs[n-1]
		p.bufs = p.bufs[:n-1]
		return b[:0]
	}
	return make([]Handle, 0, 16)
}

func (p *Parser) putBuf(b []Handle) {
	if cap(b) != 0 {
		p.bufs = append(p.bufs, b[:0])
	}
}

const (
	popCount = iota
	popAll
)

// C/stack.c stack__iter with pop_count_callback or pop_all_callback.
func (p *Parser) iter(version int, mode int, goal uint32) []stackSlice {
	for i := range p.slices {
		p.slices[i] = stackSlice{}
	}
	p.slices = p.slices[:0]
	p.iters = p.iters[:0]
	p.iters = append(p.iters, stackIter{node: p.heads[version].node, subtrees: p.getBuf(), pending: true})
	for len(p.iters) > 0 {
		size := len(p.iters)
		for i := 0; i < size; i++ {
			it := &p.iters[i]
			node := it.node
			var pop, stop bool
			if mode == popCount {
				pop = it.subtreeCount == goal
				stop = pop
			} else {
				pop = p.nodes[node].linkCount == 0
			}
			stop = stop || p.nodes[node].linkCount == 0
			if pop {
				subs := it.subtrees
				if !stop {
					subs = append(p.getBuf(), subs...)
				}
				for a, b := 0, len(subs)-1; a < b; a, b = a+1, b-1 {
					subs[a], subs[b] = subs[b], subs[a]
				}
				p.addSlice(version, node, subs)
			}
			if stop {
				if !pop {
					p.putBuf(it.subtrees)
				}
				p.iters = append(p.iters[:i], p.iters[i+1:]...)
				i--
				size--
				continue
			}
			lc := int(p.nodes[node].linkCount)
			for j := 1; j <= lc; j++ {
				var next *stackIter
				var link stackLink
				if j == lc {
					link = p.nodes[node].links[0]
					next = &p.iters[i]
				} else {
					if len(p.iters) >= maxIteratorCount {
						continue
					}
					link = p.nodes[node].links[j]
					cur := p.iters[i]
					cur.subtrees = append(p.getBuf(), cur.subtrees...)
					p.iters = append(p.iters, cur)
					next = &p.iters[len(p.iters)-1]
				}
				next.node = link.node
				if link.sub != 0 {
					next.subtrees = append(next.subtrees, link.sub)
					if !p.extra(link.sub) {
						next.subtreeCount++
						if !link.pending {
							next.pending = false
						}
					}
				} else {
					next.subtreeCount++
					next.pending = false
				}
			}
		}
	}
	return p.slices
}

func (p *Parser) push(version int, sub Handle, pending bool, state uint16) { // C/stack.c ts_stack_push
	h := &p.heads[version]
	n := p.nodeNew(h.node, sub, pending, state)
	h = &p.heads[version]
	if sub == 0 {
		h.nodeCountAtLastError = p.nodes[n].nodeCount
	}
	h.node = n
}

func (p *Parser) removeVersion(v int) { // C/stack.c ts_stack_remove_version
	p.headDelete(&p.heads[v])
	p.heads = append(p.heads[:v], p.heads[v+1:]...)
}

func (p *Parser) renumber(v1, v2 int) { // C/stack.c ts_stack_renumber_version
	if v1 == v2 {
		return
	}
	if p.heads[v2].summary != nil && p.heads[v1].summary == nil {
		p.heads[v1].summary = p.heads[v2].summary
	}
	p.headDelete(&p.heads[v2])
	p.heads[v2] = p.heads[v1]
	p.heads = append(p.heads[:v1], p.heads[v1+1:]...)
}

func (p *Parser) canMerge(v1, v2 int) bool { // C/stack.c ts_stack_can_merge
	h1, h2 := &p.heads[v1], &p.heads[v2]
	n1, n2 := &p.nodes[h1.node], &p.nodes[h2.node]
	return h1.status == headActive && h2.status == headActive && n1.state == n2.state &&
		n1.position.bytes == n2.position.bytes && n1.errorCost == n2.errorCost &&
		p.extStateEq(h1.lastExternal, h2.lastExternal)
}

func (p *Parser) merge(v1, v2 int) bool { // C/stack.c ts_stack_merge
	if !p.canMerge(v1, v2) {
		return false
	}
	n1, n2 := p.heads[v1].node, p.heads[v2].node
	for i := 0; i < int(p.nodes[n2].linkCount); i++ {
		p.addLink(n1, p.nodes[n2].links[i])
	}
	if p.nodes[n1].state == 0 {
		p.heads[v1].nodeCountAtLastError = p.nodes[n1].nodeCount
	}
	p.removeVersion(v2)
	return true
}

func (p *Parser) haltedCount() int {
	c := 0
	for i := range p.heads {
		if p.heads[i].status == headHalted {
			c++
		}
	}
	return c
}

func (p *Parser) errorCostOf(v int) uint32 { // C/stack.c ts_stack_error_cost
	h := &p.heads[v]
	n := &p.nodes[h.node]
	r := n.errorCost
	if h.status == headPaused || (n.state == 0 && n.links[0].sub == 0) {
		r += costPerRecovery
	}
	return r
}

func (p *Parser) nodeCountSinceError(v int) uint32 { // C/stack.c ts_stack_node_count_since_error
	h := &p.heads[v]
	n := &p.nodes[h.node]
	if n.nodeCount < h.nodeCountAtLastError {
		h.nodeCountAtLastError = n.nodeCount
	}
	return n.nodeCount - h.nodeCountAtLastError
}
