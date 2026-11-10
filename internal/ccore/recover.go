package ccore

// Error recovery (C/parser.c ts_parser__handle_error,
// ts_parser__do_all_potential_reductions, ts_parser__recover,
// ts_parser__recover_to_state, ts_parser__better_version_exists, and
// C/subtree.c ts_subtree_new_error, ts_subtree_new_error_node,
// ts_subtree_new_missing_leaf). The error branch of ts_parser__lex is in lexer.go.

const maxSummaryDepth = 16

type reduceAction struct { // C ReduceAction
	symbol       Symbol
	count        uint32
	dynPrec      int32
	productionID uint16
}

// C/subtree.c:245 ts_subtree_new_error.
func (p *Parser) newErrorLeaf(padding, size Length, lookahead uint32, state uint16) Handle {
	h := p.newLeaf(symError, padding, size, lookahead, state, false, false, false)
	p.rec(h).flags |= heapFragileLeft | heapFragileRight
	return h
}

// C/subtree.c ts_subtree_new_error_node.
func (p *Parser) newErrorNode(children []Handle, extra bool) Handle {
	h := p.newNode(symError, children, 0)
	if extra {
		p.rec(h).flags |= heapExtra
	}
	return h
}

// C/subtree.c ts_subtree_new_missing_leaf.
func (p *Parser) newMissingLeaf(sym Symbol, state uint16, padding Length, lookahead uint32) Handle {
	h := p.newLeaf(sym, padding, Length{}, lookahead, state, false, false, false)
	if h&1 != 0 {
		return h | Handle(inlMissing)
	}
	p.rec(h).flags |= heapMissing
	return h
}

// C/parser.c:304 ts_parser__better_version_exists.
func (p *Parser) betterVersionExists(version int, inError bool, cost uint32) bool {
	if p.finished != 0 && p.errorCost(p.finished) <= cost {
		return true
	}
	position := p.nodes[p.heads[version].node].position
	status := errorStatus{cost: cost, inError: inError, dynPrec: p.nodes[p.heads[version].node].dynPrec, nodeCount: p.nodeCountSinceError(version)}
	for i, n := 0, len(p.heads); i < n; i++ {
		if i == version || p.heads[i].status != headActive || p.nodes[p.heads[i].node].position.bytes < position.bytes {
			continue
		}
		si := p.versionStatus(i)
		switch compareVersions(status, si) {
		case takeRight:
			return true
		case preferRight:
			if p.canMerge(i, version) {
				return true
			}
		}
	}
	return false
}

// C/parser.c:1107 ts_parser__process_candidate_recovery_actions with
// C/reduce_action.h ts_reduce_action_set_add.
func (p *Parser) candidateRecoveryActions(actions []Action, set *[]reduceAction) bool {
	hasShift := false
	for _, a := range actions {
		switch a.Type {
		case ActionShift, ActionRecover:
			if !a.Extra && !a.Repetition {
				hasShift = true
			}
		case ActionReduce:
			if a.ChildCount > 0 {
				dup := false
				for _, r := range *set {
					if r.symbol == a.Symbol && r.count == uint32(a.ChildCount) {
						dup = true
						break
					}
				}
				if !dup {
					*set = append(*set, reduceAction{symbol: a.Symbol, count: uint32(a.ChildCount), dynPrec: int32(a.DynamicPrecedence), productionID: a.ProductionID})
				}
			}
		}
	}
	return hasShift
}

// C/parser.c:1136 ts_parser__do_all_potential_reductions.
func (p *Parser) doAllPotentialReductions(starting int, lookaheadSym Symbol) bool {
	initial := len(p.heads)
	canShift := false
	version := starting
	for i := 0; ; i++ {
		versionCount := len(p.heads)
		if version >= versionCount {
			break
		}
		merged := false
		for j := initial; j < version; j++ {
			if p.merge(j, version) {
				merged = true
				break
			}
		}
		if merged {
			continue
		}
		state := p.nodes[p.heads[version].node].state
		hasShift := false
		set := p.reduceSet[:0]
		if lookaheadSym != 0 {
			e := p.tableEntry(state, lookaheadSym)
			hasShift = p.candidateRecoveryActions(e.actions, &set)
		} else {
			p.forEachLookahead(state, func(sym Symbol, value uint16) {
				if sym == 0 || uint32(sym) >= p.t.TokenCount || int(value) >= len(p.t.Actions) {
					return
				}
				if p.candidateRecoveryActions(p.t.Actions[value].Actions, &set) {
					hasShift = true
				}
			})
			for j := 1; j < len(set); j++ {
				key := set[j]
				k := j - 1
				for k >= 0 && set[k].symbol < key.symbol {
					set[k+1] = set[k]
					k--
				}
				set[k+1] = key
			}
		}
		p.reduceSet = set
		reductionVersion := -1
		for _, a := range set {
			reductionVersion = p.reduce(version, a.symbol, a.count, a.dynPrec, a.productionID, true, false)
		}
		if hasShift {
			canShift = true
		} else if reductionVersion >= 0 && i < maxVersionCount {
			p.renumber(reductionVersion, version)
			continue
		} else if lookaheadSym != 0 {
			p.removeVersion(version)
			continue
		}
		if version == starting {
			version = versionCount
		} else {
			version++
		}
	}
	return canShift
}

// C/parser.c:1480 ts_parser__handle_error.
func (p *Parser) handleError(version int, lookahead Handle) {
	previousCount := len(p.heads)
	p.doAllPotentialReductions(version, 0)
	versionCount := len(p.heads)
	// C reads the version's position for ts_lexer_reset; without included
	// ranges the missing token's padding is zero.
	didInsertMissing := false
	leafSym := p.leafSymbol(lookahead)
	for v := version; v < versionCount; {
		if !didInsertMissing {
			state := p.nodes[p.heads[v].node].state
			for missing := Symbol(1); uint32(missing) < p.t.TokenCount; missing++ {
				after := p.nextState(state, missing)
				if after == 0 || after == state {
					continue
				}
				e := p.tableEntry(after, leafSym)
				if len(e.actions) == 0 || e.actions[0].Type != ActionReduce {
					continue
				}
				// ts_lexer_reset + ts_lexer_mark_end at the version's position:
				// without included ranges the padding is zero.
				lookaheadBytes := p.totalSize(lookahead).bytes + p.lookaheadBytes(lookahead)
				withMissing := p.copyVersion(v)
				missingTree := p.newMissingLeaf(missing, state, Length{}, lookaheadBytes)
				p.push(withMissing, missingTree, false, after)
				if p.doAllPotentialReductions(withMissing, leafSym) {
					if p.logf != nil {
						p.log("recover_with_missing symbol:%s, state:%d", p.symName(missing), p.nodes[p.heads[withMissing].node].state)
					}
					didInsertMissing = true
					break
				}
			}
		}
		p.push(v, 0, false, 0)
		if v == version {
			v = previousCount
		} else {
			v++
		}
	}
	for i := previousCount; i < versionCount; i++ {
		p.merge(version, previousCount)
	}
	p.recordSummary(version, maxSummaryDepth)
	if p.childCount(lookahead) > 0 {
		lookahead = p.breakdownLookahead(lookahead, 0)
	}
	p.recover(version, lookahead)
}

// C/parser.c:1225 ts_parser__recover_to_state.
func (p *Parser) recoverToState(version int, depth uint32, goal uint16) bool {
	// The pop's slices live in the stack's slice array, which
	// ts_stack_pop_error reuses; C only overwrites entries it has already
	// read, so working on a copy is equivalent.
	pop := append(p.recSlices[:0], p.iter(version, popCount, depth)...)
	defer func() { p.recSlices = pop[:0] }()
	previous := -1
	for i := 0; i < len(pop); i++ {
		slice := pop[i]
		if slice.version == previous {
			p.putBuf(slice.subtrees)
			continue
		}
		if p.nodes[p.heads[slice.version].node].state != goal {
			p.heads[slice.version].status = headHalted
			p.putBuf(slice.subtrees)
			continue
		}
		subtrees := slice.subtrees
		if errorTrees := p.popError(slice.version); len(errorTrees) > 0 {
			errorTree := errorTrees[0]
			if p.childCount(errorTree) > 0 {
				nested := append([]Handle(nil), p.children(errorTree)...)
				nestedError := p.newNode(symErrorRepeat, nested, 0)
				subtrees = append(subtrees, 0)
				copy(subtrees[1:], subtrees)
				subtrees[0] = nestedError
			}
			p.putBuf(errorTrees)
		}
		var rest []Handle
		rest, p.trailing = p.removeTrailingExtras(subtrees, p.trailing)
		if len(rest) > 0 {
			errNode := p.newErrorNode(rest, true)
			p.push(slice.version, errNode, false, goal)
		}
		for _, t := range p.trailing {
			p.push(slice.version, t, false, goal)
		}
		p.putBuf(subtrees)
		previous = slice.version
	}
	return previous >= 0
}

// C/parser.c:1291 ts_parser__recover.
func (p *Parser) recover(version int, lookahead Handle) {
	didRecover := false
	previousCount := len(p.heads)
	position := p.nodes[p.heads[version].node].position
	summary := p.heads[version].summary
	nodeCountSinceError := p.nodeCountSinceError(version)
	currentCost := p.errorCostOf(version)
	if summary != nil && p.symbol(lookahead) != symError {
		for _, entry := range summary {
			if entry.state == 0 || entry.position.bytes == position.bytes {
				continue
			}
			depth := entry.depth
			if nodeCountSinceError > 0 {
				depth++
			}
			wouldMerge := false
			for j := 0; j < previousCount; j++ {
				n := &p.nodes[p.heads[j].node]
				if n.state == entry.state && n.position.bytes == position.bytes {
					wouldMerge = true
					break
				}
			}
			if wouldMerge {
				continue
			}
			newCost := currentCost + entry.depth*costPerSkippedTree +
				(position.bytes-entry.position.bytes)*1 + (position.row-entry.position.row)*30
			if p.betterVersionExists(version, false, newCost) {
				break
			}
			if p.hasActions(entry.state, p.symbol(lookahead)) {
				if p.recoverToState(version, depth, entry.state) {
					didRecover = true
					if p.logf != nil {
						p.log("recover_to_previous state:%d, depth:%d", entry.state, depth)
					}
					break
				}
			}
		}
	}
	for i := previousCount; i < len(p.heads); i++ {
		if p.heads[i].status != headActive {
			if p.logf != nil {
				p.log("removed paused version:%d", i)
			}
			p.removeVersion(i)
			i--
		}
	}
	if p.symbol(lookahead) == 0 {
		if p.logf != nil {
			p.log("recover_eof")
		}
		parent := p.newErrorNode(nil, false)
		p.push(version, parent, false, 1)
		p.accept(version, lookahead)
		return
	}
	if didRecover && len(p.heads) > maxVersionCount {
		p.heads[version].status = headHalted
		return
	}
	if didRecover && p.hasExternalStateChange(lookahead) { // C/parser.c:1392
		p.heads[version].status = headHalted
		return
	}
	total := p.totalSize(lookahead)
	newCost := currentCost + costPerSkippedTree + total.bytes*1 + total.row*30
	if p.betterVersionExists(version, false, newCost) {
		p.heads[version].status = headHalted
		return
	}
	if e := p.tableEntry(1, p.symbol(lookahead)); len(e.actions) > 0 {
		last := e.actions[len(e.actions)-1]
		if last.Type == ActionShift && last.Extra {
			lookahead = p.setExtra(lookahead, true)
		}
	}
	if p.logf != nil {
		p.log("skip_token symbol:%s", p.symName(p.symbol(lookahead)))
	}
	errorRepeat := p.newNode(symErrorRepeat, []Handle{lookahead}, 0)
	if nodeCountSinceError > 0 {
		pop := p.iter(version, popCount, 1)
		first := pop[0]
		if len(pop) > 1 {
			for i := 1; i < len(pop); i++ {
				p.putBuf(pop[i].subtrees)
			}
			for len(p.heads) > first.version+1 {
				p.removeVersion(first.version + 1)
			}
		}
		p.renumber(first.version, version)
		children := append(first.subtrees, errorRepeat)
		errorRepeat = p.newNode(symErrorRepeat, children, 0)
		p.putBuf(children)
	}
	p.push(version, errorRepeat, false, 0)
	p.extShift(version, lookahead) // C/parser.c:1463
}
