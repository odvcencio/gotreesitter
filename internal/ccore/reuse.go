package ccore

// Node reuse from an old tree: C/reusable_node.h, C/parser.c ts_parser__reuse_node,
// ts_parser__breakdown_top_of_stack, ts_parser__breakdown_lookahead,
// ts_parser__has_included_range_difference (a hook: no included ranges), and
// C/stack.c ts_stack_pop_pending.

type reusableEntry struct { // C reusable_node.h StackEntry
	tree       Handle
	childIndex uint32
	byteOffset uint32
}

func (p *Parser) rnClear() { p.rn, p.rnLastExt = p.rn[:0], 0 } // reusable_node_clear

func (p *Parser) rnTree() Handle {
	if len(p.rn) > 0 {
		return p.rn[len(p.rn)-1].tree
	}
	return 0
}

func (p *Parser) rnByteOffset() uint32 {
	if len(p.rn) > 0 {
		return p.rn[len(p.rn)-1].byteOffset
	}
	return ^uint32(0)
}

func (p *Parser) rnAdvance() { // reusable_node_advance
	last := p.rn[len(p.rn)-1]
	byteOffset := last.byteOffset + p.totalSize(last.tree).bytes
	if p.hasExternalTokens(last.tree) {
		p.rnLastExt = p.lastExternalToken(last.tree)
	}
	var tree Handle
	var next uint32
	for {
		popped := p.rn[len(p.rn)-1]
		p.rn = p.rn[:len(p.rn)-1]
		next = popped.childIndex + 1
		if len(p.rn) == 0 {
			return
		}
		tree = p.rn[len(p.rn)-1].tree
		if p.childCount(tree) > next {
			break
		}
	}
	p.rn = append(p.rn, reusableEntry{tree: p.children(tree)[next], childIndex: next, byteOffset: byteOffset})
}

func (p *Parser) rnDescend() bool { // reusable_node_descend
	last := p.rn[len(p.rn)-1]
	if p.childCount(last.tree) > 0 {
		p.rn = append(p.rn, reusableEntry{tree: p.children(last.tree)[0], childIndex: 0, byteOffset: last.byteOffset})
		return true
	}
	return false
}

func (p *Parser) rnAdvancePastLeaf() { // reusable_node_advance_past_leaf
	for p.rnDescend() {
	}
	p.rnAdvance()
}

func (p *Parser) rnReset(tree Handle) { // reusable_node_reset
	p.rnClear()
	p.rn = append(p.rn, reusableEntry{tree: tree})
	if !p.rnDescend() {
		p.rnClear()
	}
}

// C/parser.c:753 ts_parser__reuse_node. The record of the candidate is fetched
// once per candidate; the answers are the accessors' (symbol, totalSize,
// hasChanges, missing, isFragile).
func (p *Parser) reuseNode(version int, state *uint16, position uint32, lastExternal Handle, entry *actionEntry) Handle {
	for {
		result := p.rnTree()
		if result == 0 {
			break
		}
		var r *rec
		if result&1 == 0 {
			r = p.rec(result)
		}
		var sym Symbol
		var total Length
		if r != nil {
			sym = r.symbol
			total = lenAdd(r.padding, r.size)
		} else {
			sym = p.symbol(result)
			total = p.totalSize(result)
		}
		byteOffset := p.rnByteOffset()
		endByteOffset := byteOffset + total.bytes
		if sym == 0 {
			endByteOffset = ^uint32(0)
		}
		if byteOffset > position {
			if p.logf != nil {
				p.log("before_reusable_node symbol:%s", p.symName(sym))
			}
			break
		}
		if byteOffset < position {
			if p.logf != nil {
				p.log("past_reusable_node symbol:%s", p.symName(sym))
			}
			if endByteOffset <= position || !p.rnDescend() {
				p.rnAdvance()
			}
			continue
		}
		if !p.extStateEq(p.rnLastExt, lastExternal) {
			if p.logf != nil {
				p.log("reusable_node_has_different_external_scanner_state symbol:%s", p.symName(sym))
			}
			p.rnAdvance()
			continue
		}
		reason := ""
		if r != nil {
			switch {
			case r.flags&heapHasChanges != 0:
				reason = "has_changes"
			case sym == symError:
				reason = "is_error"
			case r.flags&heapMissing != 0:
				reason = "is_missing"
			case r.flags&(heapFragileLeft|heapFragileRight) != 0:
				reason = "is_fragile"
			}
		} else {
			switch {
			case p.hasChanges(result):
				reason = "has_changes"
			case sym == symError:
				reason = "is_error"
			case p.missing(result):
				reason = "is_missing"
			case p.isFragile(result):
				reason = "is_fragile"
			}
		}
		if reason == "" && p.hasIncludedRangeDifference(byteOffset, endByteOffset) {
			reason = "contains_different_included_range"
		}
		if reason != "" {
			if p.logf != nil {
				p.log("cant_reuse_node_%s tree:%s", reason, p.symName(sym))
			}
			if !p.rnDescend() {
				p.rnAdvance()
				p.breakdownTopOfStack(version)
				*state = p.nodes[p.heads[version].node].state
			}
			continue
		}
		leafSym := p.leafSymbol(result)
		*entry = p.tableEntry(*state, leafSym)
		if !p.canReuseFirstLeaf(*state, result, *entry) {
			if p.logf != nil {
				p.log("cant_reuse_node symbol:%s, first_leaf_symbol:%s", p.symName(sym), p.symName(leafSym))
			}
			p.rnAdvancePastLeaf()
			break
		}
		if p.logf != nil {
			p.log("reuse_node symbol:%s", p.symName(sym))
		}
		p.reusedNodes++
		p.reusedBytes += uint64(total.bytes)
		if total.bytes > p.largestReuse {
			p.largestReuse = total.bytes
		}
		return result
	}
	return 0
}

// hasIncludedRangeDifference is the hook for C/parser.c:740; there are no
// included ranges in scope, so the difference list is empty.
func (p *Parser) hasIncludedRangeDifference(start, end uint32) bool { return false }

// C/parser.c:224 ts_parser__breakdown_lookahead.
func (p *Parser) breakdownLookahead(lookahead Handle, state uint16) Handle {
	didDescend := false
	tree := p.rnTree()
	for p.childCount(tree) > 0 && p.parseState(tree) != state {
		if p.logf != nil {
			p.log("state_mismatch sym:%s", p.symName(p.symbol(tree)))
		}
		p.rnDescend()
		tree = p.rnTree()
		didDescend = true
	}
	if didDescend {
		return tree
	}
	return lookahead
}

// popPending is C/stack.c ts_stack_pop_pending.
func (p *Parser) popPending(version int) []stackSlice {
	pop := p.iterCB(version, func(it *stackIter) (bool, bool) {
		if it.subtreeCount >= 1 {
			if it.pending {
				return true, true
			}
			return false, true
		}
		return false, false
	}, true)
	if len(pop) > 0 {
		p.renumber(pop[0].version, version)
		pop[0].version = version
	}
	return pop
}

// C/parser.c:176 ts_parser__breakdown_top_of_stack.
func (p *Parser) breakdownTopOfStack(version int) bool {
	didBreakDown := false
	pending := false
	for {
		pop := append(p.recSlices[:0], p.popPending(version)...)
		p.recSlices = pop[:0]
		if len(pop) == 0 {
			break
		}
		didBreakDown = true
		pending = false
		for _, slice := range pop {
			state := p.nodes[p.heads[slice.version].node].state
			parent := slice.subtrees[0]
			for _, child := range p.children(parent) {
				pending = p.childCount(child) > 0
				if p.symbol(child) == symError {
					state = 0
				} else if !p.extra(child) {
					state = p.nextState(state, p.symbol(child))
				}
				p.push(slice.version, child, pending, state)
			}
			for _, tree := range slice.subtrees[1:] {
				p.push(slice.version, tree, false, state)
			}
			if p.logf != nil {
				p.log("breakdown_top_of_stack tree:%s", p.symName(p.symbol(parent)))
			}
			p.putBuf(slice.subtrees)
		}
		if !pending {
			break
		}
	}
	return didBreakDown
}
