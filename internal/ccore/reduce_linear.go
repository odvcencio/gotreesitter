package ccore

// SetReduceCheck switches the cross-check of the linear reduction against the
// general walk on or off for this parser. Set it before parsing; it is off in
// normal use. The cross-check compares the pop (endpoint, version number and
// subtrees) before the parent is built.
func (p *Parser) SetReduceCheck(on bool) { p.reduceCheck = on }

// FastReductions returns the number of reductions that took the linear path
// since the parser was made.
func (p *Parser) FastReductions() int { return p.fastReductions }

// reduceLinear is C/parser.c:937 ts_parser__reduce for the case that one version is live and
// the pop walks a chain of stack nodes that each have one link. It gives the
// stack the state that the general path (iter, addSlice, addVersion) gives,
// and returns the same value (the index of the new version). ok is false when
// the chain is not linear; nothing has changed then, and the caller takes the
// general path. The walk is the specialisation of C/stack.c:324 stack__iter with
// the callback of C/stack.c:525 pop_count_callback, as ts_stack_pop_count
// (C/stack.c:534) runs it: every subtree on the way is collected, only
// non-extra subtrees count, and the walk stops right after the requested count.
func (p *Parser) reduceLinear(version int, sym Symbol, count uint32, dynPrec int32, productionID uint16, isFragile, endOfNonTerminalExtra bool) (int, bool) {
	if count == 0 {
		return 0, false
	}
	subs := p.getBuf()
	node := p.heads[version].node
	for n := uint32(0); n < count; {
		nd := &p.nodes[node]
		if nd.linkCount != 1 || nd.links[0].sub == 0 {
			p.putBuf(subs)
			return 0, false
		}
		l := nd.links[0]
		subs = append(subs, l.sub)
		if !p.extra(l.sub) {
			n++
		}
		node = l.node
	}
	for a, b := 0, len(subs)-1; a < b; a, b = a+1, b-1 {
		subs[a], subs[b] = subs[b], subs[a]
	}
	if p.reduceCheck {
		p.checkLinear(version, count, node, subs)
	}
	// With one version and one slice the new version is index 1, and no merge
	// is tried (the only earlier index is the original version). The check of
	// the version cap cannot fire for index 1.
	p.fastReductions++
	sliceVersion := p.addVersion(version, node)
	var children []Handle
	children, p.trailing = p.removeTrailingExtras(subs, p.trailing)
	parent := p.newNode(sym, children, productionID)
	p.putBuf(subs)
	state := p.nodes[node].state
	next := p.nextState(state, sym)
	r := p.rec(parent)
	if endOfNonTerminalExtra && next == state {
		r.flags |= heapExtra
	}
	if isFragile {
		r.flags |= heapFragileLeft | heapFragileRight
		r.parseState = stateNone
	} else {
		r.parseState = state
	}
	r.dynamicPrecedence += dynPrec
	p.push(sliceVersion, parent, false, next)
	for _, t := range p.trailing {
		p.push(sliceVersion, t, false, next)
	}
	return 1, true
}

// checkLinear runs the general walk on the same stack and compares: one slice,
// the same subtrees, the same node. It then undoes the walk's changes.
func (p *Parser) checkLinear(version int, count uint32, node int32, subs []Handle) {
	heads, refs := len(p.heads), p.nodes[node].refCount
	pop := p.iter(version, popCount, count)
	ok := len(pop) == 1 && pop[0].version == heads && p.heads[heads].node == node && len(pop[0].subtrees) == len(subs)
	if ok {
		for i := range subs {
			if subs[i] != pop[0].subtrees[i] {
				ok = false
			}
		}
	}
	if !ok {
		panic("ccore: the linear reduction differs from the general walk")
	}
	p.putBuf(pop[0].subtrees)
	p.heads = p.heads[:heads]
	p.nodes[node].refCount = refs
}
