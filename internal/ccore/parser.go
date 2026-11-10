package ccore

// Config names what a Parser needs from the root package.
type Config struct {
	Tables  *Tables
	Scanner ExtScanner // nil when the language has no external scanner
}

// Parser is one parsing context (C TSParser).
type Parser struct {
	Store
	t        *Tables
	scanner  ExtScanner
	src      []byte
	nodes    []stackNode
	free     []int32
	heads    []stackHead
	slices   []stackSlice
	iters    []stackIter
	bufs     [][]Handle
	trailing []Handle
	trail2   []Handle
	scratch  []Handle
	// recovery scratch (recover.go)
	recSlices []stackSlice
	reduceSet []reduceAction
	// incremental parsing (reuse.go, edit.go)
	rn           []reusableEntry
	rnLastExt    Handle // C ReusableNode.last_external_token
	reuseActive  bool
	watermark    uint32 // records below it belong to trees alive before this parse (C: ref_count > 1)
	reusedNodes  int
	reusedBytes  uint64
	largestReuse uint32
	finished     Handle
	cacheTok     Handle
	cacheAt      uint32
	accepts      int
	logf         func(string)
	stop         string
	ext          scannerState // external scanner side (scanner.go)
	forceRows    bool
	fail         [2]Length // where the last failed main scan began and ended
	kwFail       [2]Length // the same for a keyword scan (not read)
	lexTok       Token     // scratch tokens
	kwTok        Token
	skipTok      Token

	// NormNames makes digest records compare node names with backslashes
	// removed (a test switch for grammars whose Go table spells a name with
	// a backslash that C's table does not).
	NormNames bool

	// Counters, always on.
	shifts, reductions, lexCalls, tokens, maxVersions, multiTokens int
}

// NewParser returns a parser for the language described by c, or an error
// that says why the core cannot serve it (validate.go). It checks the tables
// and hooks once; parsing does not repeat the checks.
func NewParser(c Config) (*Parser, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &Parser{t: c.Tables, scanner: c.Scanner}, nil
}

// SetForceDerivedRows derives the external valid-symbol rows from the action
// table even when the tables carry them (a test switch).
func (p *Parser) SetForceDerivedRows(on bool) { p.forceRows = on }

// C/parser.c:914 ts_parser__shift.
func (p *Parser) shift(version int, state uint16, lookahead Handle, extra bool) {
	isLeaf := p.childCount(lookahead) == 0
	sub := lookahead
	if extra != p.extra(lookahead) && isLeaf {
		sub = p.setExtra(lookahead, extra)
	}
	p.push(version, sub, !isLeaf, state)
	p.extShift(version, sub)
}

// C/parser.c:889 ts_parser__select_children.
func (p *Parser) selectChildren(left Handle, children []Handle) bool {
	scratch := p.newNode(p.symbol(left), children, 0)
	return p.selectTree(left, scratch)
}

// C/parser.c:842 ts_parser__select_tree.
func (p *Parser) selectTree(left, right Handle) bool {
	if left == 0 {
		return true
	}
	if right == 0 {
		return false
	}
	le, re := p.errorCost(left), p.errorCost(right)
	if re < le {
		if p.logf != nil {
			p.log("select_smaller_error symbol:%s, over_symbol:%s", p.symName(p.symbol(right)), p.symName(p.symbol(left)))
		}
		return true
	}
	if le < re {
		if p.logf != nil {
			p.log("select_smaller_error symbol:%s, over_symbol:%s", p.symName(p.symbol(left)), p.symName(p.symbol(right)))
		}
		return false
	}
	ld, rd := p.dynPrec(left), p.dynPrec(right)
	if rd > ld {
		if p.logf != nil {
			p.log("select_higher_precedence symbol:%s, prec:%d, over_symbol:%s, other_prec:%d", p.symName(p.symbol(right)), rd, p.symName(p.symbol(left)), ld)
		}
		return true
	}
	if ld > rd {
		if p.logf != nil {
			p.log("select_higher_precedence symbol:%s, prec:%d, over_symbol:%s, other_prec:%d", p.symName(p.symbol(left)), ld, p.symName(p.symbol(right)), rd)
		}
		return false
	}
	if le > 0 {
		return true
	}
	switch p.compare(left, right) {
	case -1:
		if p.logf != nil {
			p.log("select_earlier symbol:%s, over_symbol:%s", p.symName(p.symbol(left)), p.symName(p.symbol(right)))
		}
		return false
	case 1:
		if p.logf != nil {
			p.log("select_earlier symbol:%s, over_symbol:%s", p.symName(p.symbol(right)), p.symName(p.symbol(left)))
		}
		return true
	default:
		if p.logf != nil {
			p.log("select_existing symbol:%s, over_symbol:%s", p.symName(p.symbol(left)), p.symName(p.symbol(right)))
		}
		return false
	}
}

// C/parser.c:937 ts_parser__reduce.
func (p *Parser) reduce(version int, sym Symbol, count uint32, dynPrec int32, productionID uint16, isFragile, endOfNonTerminalExtra bool) int {
	initialCount := len(p.heads)
	pop := p.iter(version, popCount, count)
	removed := 0
	halted := p.haltedCount()
	for i := 0; i < len(pop); i++ {
		slice := pop[i]
		sliceVersion := slice.version - removed
		if sliceVersion > maxVersionCount+maxVersionOverflow+halted {
			p.removeVersion(sliceVersion)
			p.putBuf(slice.subtrees)
			removed++
			for i+1 < len(pop) {
				if p.logf != nil {
					p.log("aborting reduce with too many versions")
				}
				next := pop[i+1]
				if next.version != slice.version {
					break
				}
				p.putBuf(next.subtrees)
				i++
			}
			continue
		}
		var children []Handle
		children, p.trailing = p.removeTrailingExtras(slice.subtrees, p.trailing)
		parent := p.newNode(sym, children, productionID)
		p.putBuf(slice.subtrees)
		for i+1 < len(pop) {
			next := pop[i+1]
			if next.version != slice.version {
				break
			}
			i++
			var nextChildren []Handle
			nextChildren, p.trail2 = p.removeTrailingExtras(next.subtrees, p.trail2)
			if p.selectChildren(parent, nextChildren) {
				p.trailing, p.trail2 = p.trail2, p.trailing
				parent = p.newNode(sym, nextChildren, productionID)
			} else {
				p.trail2 = p.trail2[:0]
			}
			p.putBuf(next.subtrees)
		}
		state := p.nodes[p.heads[sliceVersion].node].state
		next := p.nextState(state, sym)
		r := p.rec(parent)
		if endOfNonTerminalExtra && next == state {
			r.flags |= heapExtra
		}
		if isFragile || len(pop) > 1 || initialCount > 1 {
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
		for j := 0; j < sliceVersion; j++ {
			if j == version {
				continue
			}
			if p.merge(j, sliceVersion) {
				removed++
				break
			}
		}
	}
	if len(p.heads) > initialCount {
		return initialCount
	}
	return -1
}

// C/parser.c:1054 ts_parser__accept.
func (p *Parser) accept(version int, lookahead Handle) {
	p.push(version, lookahead, false, 1)
	pop := p.iter(version, popAll, 0)
	for i := range pop {
		trees := pop[i].subtrees
		var root Handle
		for j := len(trees) - 1; j >= 0; j-- {
			tree := trees[j]
			if !p.extra(tree) {
				kids := p.children(tree)
				spliced := append(p.getBuf(), trees[:j]...)
				spliced = append(spliced, kids...)
				spliced = append(spliced, trees[j+1:]...)
				root = p.newNode(p.symbol(tree), spliced, p.rec(tree).productionID)
				p.putBuf(spliced)
				break
			}
		}
		p.accepts++
		if p.finished != 0 {
			if p.selectTree(p.finished, root) {
				p.finished = root
			}
		} else {
			p.finished = root
		}
		p.putBuf(trees)
	}
	p.removeVersion(pop[0].version)
	p.heads[version].status = headHalted
}

// C/parser.c:1598 ts_parser__advance.
func (p *Parser) advance(version int, allowReuse bool) bool {
	state := p.nodes[p.heads[version].node].state
	position := p.nodes[p.heads[version].node].position.bytes
	var entry actionEntry
	lastExternal := p.heads[version].lastExternal
	var lookahead Handle
	didReuse := true
	if allowReuse && p.reuseActive {
		lookahead = p.reuseNode(version, &state, position, lastExternal, &entry)
	}
	if lookahead == 0 {
		didReuse = false
		lookahead = p.cachedToken(state, position, lastExternal, &entry)
	}
	needsLex := lookahead == 0
	for {
		if needsLex {
			needsLex = false
			var ok bool
			lookahead, ok = p.lex(version, state)
			if !ok {
				return false
			}
			if lookahead != 0 {
				p.cacheTok, p.cacheAt, p.ext.cacheLast = lookahead, position, lastExternal
				entry = p.tableEntry(state, p.symbol(lookahead))
				p.tokens++
				if len(p.heads) > 1 {
					p.multiTokens++
				}
			} else {
				entry = p.tableEntry(state, 0)
			}
		}
		didReduce := false
		lastReduction := -1
		for _, action := range entry.actions {
			switch action.Type {
			case ActionShift:
				if action.Repetition {
					continue
				}
				var next uint16
				if action.Extra {
					next = state
					if p.logf != nil {
						p.log("shift_extra")
					}
				} else {
					next = uint16(action.State)
					if p.logf != nil {
						p.log("shift state:%d", next)
					}
				}
				if p.childCount(lookahead) > 0 {
					lookahead = p.breakdownLookahead(lookahead, state)
					next = p.nextState(state, p.symbol(lookahead))
				}
				p.shifts++
				p.shift(version, next, lookahead, action.Extra)
				if didReuse {
					p.rnAdvance()
				}
				return true
			case ActionReduce:
				if p.logf != nil {
					p.log("reduce sym:%s, child_count:%d", p.symName(action.Symbol), action.ChildCount)
				}
				p.reductions++
				rv := p.reduce(version, action.Symbol, uint32(action.ChildCount), int32(action.DynamicPrecedence), action.ProductionID, len(entry.actions) > 1, lookahead == 0)
				didReduce = true
				if rv >= 0 {
					lastReduction = rv
				}
			case ActionAccept:
				if p.logf != nil {
					p.log("accept")
				}
				p.accept(version, lookahead)
				return true
			case ActionRecover:
				if p.childCount(lookahead) > 0 {
					lookahead = p.breakdownLookahead(lookahead, 0)
				}
				p.recover(version, lookahead)
				if didReuse {
					p.rnAdvance()
				}
				return true
			}
		}
		if lastReduction >= 0 {
			p.renumber(lastReduction, version)
			state = p.nodes[p.heads[version].node].state
			if lookahead == 0 {
				needsLex = true
			} else {
				entry = p.tableEntry(state, p.leafSymbol(lookahead))
			}
			continue
		}
		if didReduce {
			p.heads[version].status = headHalted
			return true
		}
		if p.isKeyword(lookahead) && p.symbol(lookahead) != p.t.KeywordCapture && !p.isReserved(state, p.symbol(lookahead)) {
			entry = p.tableEntry(state, p.t.KeywordCapture)
			if len(entry.actions) > 0 {
				if p.logf != nil {
					p.log("switch from_keyword:%s, to_word_token:%s", p.symName(p.symbol(lookahead)), p.symName(p.t.KeywordCapture))
				}
				lookahead = p.setSymbol(lookahead, p.t.KeywordCapture)
				continue
			}
		}
		if state == 0 {
			p.recover(version, lookahead)
			return true
		}
		// ts_parser__breakdown_top_of_stack: pending links exist only after
		// node reuse, so a parse without an old tree skips the call.
		if p.reuseActive && p.breakdownTopOfStack(version) {
			state = p.nodes[p.heads[version].node].state
			needsLex = true
			continue
		}
		if p.logf != nil {
			p.log("detect_error lookahead:%s", p.symName(p.symbol(lookahead)))
		}
		h := &p.heads[version]
		h.status = headPaused
		h.lookaheadWhenPaused = lookahead
		h.nodeCountAtLastError = p.nodes[h.node].nodeCount
		return true
	}
}

type errorStatus struct { // C ErrorStatus
	cost, nodeCount uint32
	dynPrec         int32
	inError         bool
}

func (p *Parser) versionStatus(v int) errorStatus { // C/parser.c:289
	cost := p.errorCostOf(v)
	paused := p.heads[v].status == headPaused
	if paused {
		cost += costPerSkippedTree
	}
	return errorStatus{cost: cost, nodeCount: p.nodeCountSinceError(v), dynPrec: p.nodes[p.heads[v].node].dynPrec,
		inError: paused || p.nodes[p.heads[v].node].state == 0}
}

const (
	takeLeft = iota
	preferLeft
	cmpNone
	preferRight
	takeRight
)

func compareVersions(a, b errorStatus) int { // C/parser.c:246
	if !a.inError && b.inError {
		if a.cost < b.cost {
			return takeLeft
		}
		return preferLeft
	}
	if a.inError && !b.inError {
		if b.cost < a.cost {
			return takeRight
		}
		return preferRight
	}
	if a.cost < b.cost {
		if (b.cost-a.cost)*(1+a.nodeCount) > maxCostDifference {
			return takeLeft
		}
		return preferLeft
	}
	if b.cost < a.cost {
		if (a.cost-b.cost)*(1+b.nodeCount) > maxCostDifference {
			return takeRight
		}
		return preferRight
	}
	if a.dynPrec > b.dynPrec {
		return preferLeft
	}
	if b.dynPrec > a.dynPrec {
		return preferRight
	}
	return cmpNone
}

// C/parser.c:1814 ts_parser__condense_stack. It returns the minimum error
// cost of the live versions that are not in error.
func (p *Parser) condense() uint32 {
	made := false
	minCost := ^uint32(0)
	for i := 0; i < len(p.heads); i++ {
		if p.heads[i].status == headHalted {
			p.removeVersion(i)
			i--
			continue
		}
		si := p.versionStatus(i)
		if !si.inError && si.cost < minCost {
			minCost = si.cost
		}
		for j := 0; j < i; j++ {
			sj := p.versionStatus(j)
			switch compareVersions(sj, si) {
			case takeLeft:
				made = true
				p.removeVersion(i)
				i--
				j = i
			case preferLeft, cmpNone:
				if p.merge(j, i) {
					made = true
					i--
					j = i
				}
			case preferRight:
				made = true
				if p.merge(j, i) {
					i--
					j = i
				} else {
					p.heads[i], p.heads[j] = p.heads[j], p.heads[i]
				}
			case takeRight:
				made = true
				p.removeVersion(j)
				i--
				j--
			}
		}
	}
	for len(p.heads) > maxVersionCount {
		p.removeVersion(maxVersionCount)
		made = true
	}
	if len(p.heads) > 0 {
		unpaused := false
		for i, n := 0, len(p.heads); i < n; i++ {
			if p.heads[i].status == headPaused {
				if !unpaused && p.accepts < maxVersionCount {
					if p.logf != nil {
						p.log("resume version:%d", i)
					}
					minCost = p.errorCostOf(i)
					h := &p.heads[i]
					lookahead := h.lookaheadWhenPaused
					h.status, h.lookaheadWhenPaused = headActive, 0
					p.handleError(i, lookahead)
					unpaused = true
					continue
				}
				p.removeVersion(i)
				made = true
				i--
				n--
			} else {
				unpaused = true
			}
		}
	}
	if made {
		if p.logf != nil {
			p.log("condense")
		}
	}
	return minCost
}

// parseWith is the C/parser.c:2124 ts_parser_parse loop. It runs the loop with an optional edited old tree (C: old_tree).
// A fresh parse truncates the arenas; an incremental one appends to them.
func (p *Parser) parseWith(src []byte, old Handle) string {
	p.src = src
	if old == 0 {
		p.clear()
	}
	p.watermark = p.recCount()
	p.reusedNodes, p.reusedBytes, p.largestReuse = 0, 0, 0
	p.nodes = p.nodes[:0]
	p.free = p.free[:0]
	p.heads = p.heads[:0]
	p.finished, p.cacheTok, p.cacheAt, p.accepts, p.stop = 0, 0, 0, 0, ""
	p.shifts, p.reductions, p.lexCalls, p.tokens, p.maxVersions, p.multiTokens = 0, 0, 0, 0, 0, 0
	p.extReset(old == 0)
	defer p.extFinish()
	base := p.nodeNew(-1, 0, false, 1)
	p.nodes[base].refCount++ // ts_stack_clear retains the base node
	p.heads = append(p.heads, stackHead{node: base, status: headActive})
	p.reuseActive = old != 0
	if old != 0 {
		p.rnReset(old)
		if p.logf != nil {
			p.log("parse_after_edit")
		}
	} else {
		p.rnClear()
		if p.logf != nil {
			p.log("new_parse")
		}
	}
	var position, lastPosition uint32
	versionCount := 0
	for {
		for version := 0; ; version++ {
			versionCount = len(p.heads)
			if version >= versionCount {
				break
			}
			if versionCount > p.maxVersions {
				p.maxVersions = versionCount
			}
			for p.heads[version].status == headActive {
				n := &p.nodes[p.heads[version].node]
				if p.logf != nil {
					p.log("process version:%d, version_count:%d, state:%d, row:%d, col:%d", version, len(p.heads), n.state, n.position.row, n.position.col)
				}
				if !p.advance(version, versionCount == 1) {
					return p.stop
				}
				position = p.nodes[p.heads[version].node].position.bytes
				if position > lastPosition || (version > 0 && position == lastPosition) {
					lastPosition = position
					break
				}
			}
		}
		minCost := p.condense()
		if p.finished != 0 && p.errorCost(p.finished) < minCost {
			for len(p.heads) > 0 {
				p.removeVersion(len(p.heads) - 1)
			}
			break
		}
		if versionCount == 0 {
			break
		}
	}
	if p.finished == 0 {
		return "no finished tree"
	}
	p.balance()
	if p.logf != nil {
		p.log("done")
	}
	return ""
}
