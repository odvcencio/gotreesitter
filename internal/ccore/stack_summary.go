package ccore

// Stack operations that only error recovery uses: the summary record, the pop
// of a preceding ERROR, the version copy and stack__iter with an arbitrary
// callback (C/stack.c).

// summaryEntry is C's StackSummaryEntry.
type summaryEntry struct {
	position Length
	depth    uint32
	state    uint16
}

// iterCB is C/stack.c stack__iter with a callback. includeSubtrees is
// C's goal_subtree_count >= 0.
func (p *Parser) iterCB(version int, cb func(it *stackIter) (pop, stop bool), includeSubtrees bool) []stackSlice {
	for i := range p.slices {
		p.slices[i] = stackSlice{}
	}
	p.slices = p.slices[:0]
	p.iters = p.iters[:0]
	first := stackIter{node: p.heads[version].node, pending: true}
	if includeSubtrees {
		first.subtrees = p.getBuf()
	}
	p.iters = append(p.iters, first)
	for len(p.iters) > 0 {
		size := len(p.iters)
		for i := 0; i < size; i++ {
			node := p.iters[i].node
			pop, stop := cb(&p.iters[i])
			stop = stop || p.nodes[node].linkCount == 0
			if pop {
				subs := p.iters[i].subtrees
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
					p.putBuf(p.iters[i].subtrees)
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
					if includeSubtrees {
						cur.subtrees = append(p.getBuf(), cur.subtrees...)
					}
					p.iters = append(p.iters, cur)
					next = &p.iters[len(p.iters)-1]
				}
				next.node = link.node
				if link.sub != 0 {
					if includeSubtrees {
						next.subtrees = append(next.subtrees, link.sub)
					}
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

// recordSummary is C/stack.c:624 ts_stack_record_summary with
// summarize_stack_callback (:606-622).
func (p *Parser) recordSummary(version int, maxDepth uint32) {
	var entries []summaryEntry
	p.iterCB(version, func(it *stackIter) (bool, bool) {
		n := &p.nodes[it.node]
		depth := it.subtreeCount
		if depth > maxDepth {
			return false, true
		}
		for i := len(entries) - 1; i >= 0; i-- {
			if entries[i].depth < depth {
				break
			}
			if entries[i].depth == depth && entries[i].state == n.state {
				return false, false
			}
		}
		entries = append(entries, summaryEntry{position: n.position, depth: depth, state: n.state})
		return false, false
	}, false)
	p.heads[version].summary = entries
}

// popError is C/stack.c ts_stack_pop_error with pop_error_callback.
func (p *Parser) popError(version int) []Handle {
	node := p.heads[version].node
	for i := 0; i < int(p.nodes[node].linkCount); i++ {
		sub := p.nodes[node].links[i].sub
		if sub != 0 && p.symbol(sub) == symError {
			found := false
			pop := p.iterCB(version, func(it *stackIter) (bool, bool) {
				if len(it.subtrees) > 0 {
					if !found && p.symbol(it.subtrees[0]) == symError {
						found = true
						return true, true
					}
					return false, true
				}
				return false, false
			}, true)
			if len(pop) > 0 {
				subs := pop[0].subtrees
				p.renumber(pop[0].version, version)
				return subs
			}
			break
		}
	}
	return nil
}

// copyVersion is C/stack.c ts_stack_copy_version.
func (p *Parser) copyVersion(v int) int {
	h := p.heads[v]
	h.summary = nil
	p.heads = append(p.heads, h)
	p.nodeRetain(h.node)
	return len(p.heads) - 1
}
