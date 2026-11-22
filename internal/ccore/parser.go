package ccore

import "unsafe"

// Config names what a Parser needs from the root package.
type Config struct {
	Tables  *Tables
	Scanner ExtScanner  // nil when the language has no external scanner
	Nodes   *NodeTables // data for the node readers (node.go); nil when the adapter gives none
}

// Parser is one parsing context (C TSParser).
type Parser struct {
	testLimits *storeLimits // lowered store limits for stores this parser creates (tests only)

	// The balance step (subtree.go): zeroNodes counts the internal nodes of byte
	// size zero that the parse made (a statistic); multi holds, for each new
	// record that the finished tree reaches more than once, the owners beyond the
	// first (found before balancing, for every tree).
	zeroNodes, balanceChecks, balanceRefusals int
	multi                                     map[Handle]uint32
	seen                                      []Handle // scratch of findMulti
	hintRec, hintKid                          int      // pages the last fresh store took (api.go, pageHint)
	compressTmp                               []Handle // scratch of balance (compress's stack)
	reachBits                                 []uint64 // scratch of findMultiBits: one mark bit per record slot
	preBalance                                func()   // tests only: runs before the balance step
	Store
	nodeTables *NodeTables
	t          *Tables
	scanner    ExtScanner
	src        []byte
	scanProv   ScanProvider // bound for the length of one parse (api.go ParseWithScan); nil walks the automaton
	nodes      []stackNode
	free       []int32
	heads      []stackHead
	slices     []stackSlice
	iters      []stackIter
	bufs       [][]Handle
	trailing   []Handle
	trail2     []Handle
	scratch    []Handle
	// recovery scratch (recover.go)
	recSlices []stackSlice
	reduceSet []reduceAction
	// incremental parsing (reuse.go, edit.go)
	rn                []reusableEntry
	rnLastExt         Handle // C ReusableNode.last_external_token
	reuseActive       bool
	reusedNodes       int
	reusedBytes       uint64
	largestReuse      uint32
	finished          Handle
	ops               int // steps of the parse loop in this parse (stop hook)
	stopAt            int
	stopPanics        bool
	cacheTok          Handle
	cacheAt           uint32
	accepts           int
	logf              func(string)
	stop              string
	ext               scannerState // external scanner side (scanner.go)
	forceRows         bool
	fail              [2]Length // where the last failed main scan began and ended
	kwFail            [2]Length // the same for a keyword scan (not read)
	lexTok            Token     // scratch tokens
	kwTok             Token
	skipTok           Token
	scannedBuiltinEnd bool

	// NormNames makes digest records compare node names with backslashes
	// removed (a test switch for grammars whose Go table spells a name with
	// a backslash that C's table does not).
	NormNames bool

	// linear reduction (reduce_linear.go)
	reduceCheck    bool
	fastReductions int
	noLinear       bool // tests: take the general path
	shortcutOn     bool
	shortcutTaken  bool
	shortcutWhy    string
	shortcutCounts [10]uint64
	shortcutMemo   shortcutMemo

	// Counters, always on.
	shifts, reductions, lexCalls, tokens, maxVersions, multiTokens int
	compressions                                                   int    // calls of compress in the last balance
	maxProcessed                                                   int    // largest len(heads) at a "process version" event (C/parser.c:2180-2186)
	selectCandidates                                               int    // selectChildren calls; each one built no record
	opCount                                                        int    // C self->operation_count
	ledBegun                                                       bool   // Parse began the request's account before the drain
	led                                                            Ledger // request account; its pages are the store's
}

// NewParser returns a parser for the language described by c, or an error
// that says why the core cannot serve it (validate.go). It checks the tables
// and hooks once; parsing does not repeat the checks.
func NewParser(c Config) (*Parser, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &Parser{t: c.Tables, scanner: c.Scanner, nodeTables: c.Nodes}, nil
}

// NodeTables returns the node-reader data the parser was configured with (nil if none).
func (p *Parser) NodeTables() *NodeTables { return p.nodeTables }

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
//
// C builds the comparison tree in a reusable scratch array and releases nothing
// (C/parser.c:889-912): it owns no persistent subtree. The candidate here is
// the same: a symbol, production 0 and the popped children, summarized on the
// stack by the code that summarizes a node. It claims no record, takes no
// child-list storage and leaves no handle behind, so a winner is built once by
// the caller (C/parser.c:1001-1011) and a loser costs no record.
func (p *Parser) selectChildren(left Handle, children []Handle) bool {
	sym := p.symbol(left)
	var cand rec
	vis, nam := p.meta(sym)
	if vis {
		cand.flags |= heapVisible
	}
	if nam {
		cand.flags |= heapNamed
	}
	if sym == symError || sym == symErrorRepeat {
		cand.flags |= heapFragileLeft | heapFragileRight
	}
	cand.symbol, cand.childCount = sym, uint32(len(children))
	p.summarizeKids(&cand, children)
	p.selectCandidates++
	return p.selectWith(left, 0, &cand, children)
}

// C/parser.c:842 ts_parser__select_tree.
func (p *Parser) selectTree(left, right Handle) bool {
	return p.selectWith(left, right, nil, nil)
}

// selectWith is selectTree. The right tree is the record right, or, when cand
// is not nil, the summarized candidate cand over the children kids.
func (p *Parser) selectWith(left, right Handle, cand *rec, kids []Handle) bool {
	if left == 0 {
		return true
	}
	var rsym Symbol
	var re uint32
	var rd int32
	if cand != nil {
		rsym, re, rd = cand.symbol, cand.errorCost, cand.dynamicPrecedence
	} else {
		if right == 0 {
			return false
		}
		rsym, re, rd = p.symbol(right), p.errorCost(right), p.dynPrec(right)
	}
	lsym, le, ld := p.symbol(left), p.errorCost(left), p.dynPrec(left)
	if re < le {
		if p.logf != nil {
			p.log("select_smaller_error symbol:%s, over_symbol:%s", p.symName(rsym), p.symName(lsym))
		}
		return true
	}
	if le < re {
		if p.logf != nil {
			p.log("select_smaller_error symbol:%s, over_symbol:%s", p.symName(lsym), p.symName(rsym))
		}
		return false
	}
	if rd > ld {
		if p.logf != nil {
			p.log("select_higher_precedence symbol:%s, prec:%d, over_symbol:%s, other_prec:%d", p.symName(rsym), rd, p.symName(lsym), ld)
		}
		return true
	}
	if ld > rd {
		if p.logf != nil {
			p.log("select_higher_precedence symbol:%s, prec:%d, over_symbol:%s, other_prec:%d", p.symName(lsym), ld, p.symName(rsym), rd)
		}
		return false
	}
	if le > 0 {
		return true
	}
	var c int
	if cand != nil {
		c = p.compareCandidate(left, rsym, kids)
	} else {
		c = p.compare(left, right)
	}
	switch c {
	case -1:
		if p.logf != nil {
			p.log("select_earlier symbol:%s, over_symbol:%s", p.symName(lsym), p.symName(rsym))
		}
		return false
	case 1:
		if p.logf != nil {
			p.log("select_earlier symbol:%s, over_symbol:%s", p.symName(rsym), p.symName(lsym))
		}
		return true
	default:
		if p.logf != nil {
			p.log("select_existing symbol:%s, over_symbol:%s", p.symName(lsym), p.symName(rsym))
		}
		return false
	}
}

// C/parser.c:937 ts_parser__reduce.
func (p *Parser) reduce(version int, sym Symbol, count uint32, dynPrec int32, productionID uint16, isFragile, endOfNonTerminalExtra bool) int {
	initialCount := len(p.heads)
	if initialCount == 1 && !p.noLinear {
		if r, ok := p.reduceLinear(version, sym, count, dynPrec, productionID, isFragile, endOfNonTerminalExtra); ok {
			return r
		}
	}
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
		state := p.nodes[p.heads[sliceVersion].node].state
		next := p.nextState(state, sym)
		var children []Handle
		children, p.trailing = p.removeTrailingExtras(slice.subtrees, p.trailing)
		if sliceVersion > 1 && len(p.trailing) == 0 && (i+1 == len(pop) || pop[i+1].version != slice.version) && p.wouldRejectReduction(version, sliceVersion, sym, next, children) {
			p.putBuf(slice.subtrees)
			p.removeVersion(sliceVersion)
			removed++
			continue
		}
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

// wouldRejectReduction checks the first head that the reduced slice would merge
// into, in reduce's merge order. A full node can still update a compatible
// predecessor, so rule out subtreeEquivalent for each of its resident links.
// The positive, error-free parent has its first child's padding and covers
// exactly its children's bytes. Its symbol or byte extent must distinguish it
// from every resident link. Otherwise keep the ordinary push and merge.
func (p *Parser) wouldRejectReduction(version, sliceVersion int, sym Symbol, next uint16, children []Handle) bool {
	if !p.fresh || p.owner != nil || p.logging || p.reuseActive || p.ext.active || p.scanner != nil ||
		len(children) == 0 || len(children) > 2 || sym == symError || sym == symErrorRepeat {
		return false
	}
	head := p.heads[sliceVersion]
	pred := &p.nodes[head.node]
	if head.status != headActive || head.lastExternal != 0 || pred.errorCost != 0 {
		return false
	}
	if next == 0 {
		return false
	}
	var total uint64
	for _, ch := range children {
		if ch == 0 || p.missing(ch) || p.errorCost(ch) != 0 || p.symbol(ch) == symError || p.symbol(ch) == symErrorRepeat {
			return false
		}
		total += uint64(p.padding(ch).bytes) + uint64(p.size(ch).bytes)
	}
	end := uint64(pred.position.bytes) + total
	if total == 0 || end > uint64(^uint32(0)) {
		return false
	}
	padding := p.padding(children[0]).bytes
	size := uint32(total) - padding
	if size == 0 {
		return false
	}
	for j := 0; j < sliceVersion; j++ {
		if j == version {
			continue
		}
		rh := p.heads[j]
		resident := &p.nodes[rh.node]
		if rh.status != headActive || resident.state != next || resident.position.bytes != uint32(end) || resident.errorCost != 0 {
			continue
		}
		// With external scanning inactive, canMerge considers all checkpoints
		// equal. If either head retained an unexpected checkpoint, let merge
		// handle this first eligible head in its regular order.
		if rh.lastExternal != head.lastExternal || resident.linkCount != maxLinkCount {
			return false
		}
		for i := uint16(0); i < resident.linkCount; i++ {
			prior := resident.links[i].sub
			if prior != 0 && p.symbol(prior) == sym && p.padding(prior).bytes == padding && p.size(prior).bytes == size {
				return false
			}
		}
		return true
	}
	return false
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
				spliced := p.getBuf()
				if p.led.Active() { // reserve the whole buffer once, before the three appends make it
					spliced = grow(&p.led, spliced, len(trees)-1+len(kids))
				}
				{
					values := trees[:j]
					if len(values) > cap(spliced)-len(spliced) {
						spliced = grow(&p.led, spliced, len(values))
					}
					spliced = append(spliced, values...)
				}
				{
					values := kids
					if len(values) > cap(spliced)-len(spliced) {
						spliced = grow(&p.led, spliced, len(values))
					}
					spliced = append(spliced, values...)
				}
				{
					values := trees[j+1:]
					if len(values) > cap(spliced)-len(spliced) {
						spliced = grow(&p.led, spliced, len(values))
					}
					spliced = append(spliced, values...)
				}
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
func (p *Parser) advance(version int, allowReuse bool, lastPosition uint32) bool {
	if p.ops++; p.stopAt > 0 && p.ops >= p.stopAt {
		if p.stopPanics {
			panic("ccore: stop hook")
		}
		p.stop = "stopped by hook"
		return false
	}
	// C calls ts_parser__check_progress at C/parser.c:1650, inside
	// ts_parser__advance after the lookahead is chosen and before the actions
	// run; here it is the first statement. The stop decisions are the same:
	// operation_count rises by one per advance and by nothing in between, the
	// hook receives only byte counts, and a stopped parse returns no tree.
	if p.led.Active() && !p.checkProgress(1) {
		return false
	}
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
		if lookahead&1 == 0 && lookahead != 0 {
			// lifetime.go, step 2: a cached heap token is handed out once; every later
			// hand-out is a copy of the record with its state bytes, so that no record sits in two slots
			// of a tree. C shares the leaf and counts two owners (C/parser.c:711-718).
			lookahead = p.copyHeapLeaf(lookahead)
		}
	}
	needsLex := lookahead == 0
	for {
		if needsLex {
			needsLex = false
			var ok bool
			lookahead, ok = p.lex(version, state)
			if !ok {
				if p.hasDeferredSkip() {
					p.materializeDeferredSkip()
				}
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
			if p.hasDeferredSkip() {
				p.resolveDeferredSkip(lookahead, entry)
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
				p.recover(version, lookahead, lastPosition, didReuse)
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
			p.recover(version, lookahead, lastPosition, didReuse)
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
		if status := p.heads[i].status; status == headHalted || status == headDominated {
			if status == headDominated {
				made = true
			}
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
// ParseWith (api.go) holds the lease, sets up the store and ends the parse.
//
// With a limit active (Ledger().SetPoll) the parse starts an account (Begin) and
// a refused reservation anywhere in the core unwinds by a private panic that this
// function turns into the stop "stopped"; any other panic passes on unchanged. The
// next call rebuilds all state it reads.
func (p *Parser) parseWith(src []byte, old Handle) (stop string) {
	if p.led.Active() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(ledgerStop); !ok {
					panic(r)
				}
				p.finished = 0
				p.stop = stopProgress
				stop = p.stop
			}
		}()
		// Base: what the parse holds before it claims anything. The pages that an old
		// tree already holds are in it (not charged again); the parser's own buffers too.
		if p.ledBegun { // Parse began the account before the drain
			p.ledBegun = false
		} else {
			p.clearStaleHeaders() // no header of an earlier parse is live here
			if !p.led.Begin(p.accountBase()) {
				p.finished = 0
				p.stop = stopProgress
				return p.stop
			}
		}
	}
	p.src = src
	attemptTokens, attemptCalls := 0, 0
	if old != 0 && p.shortcutOn {
		p.stop = ""
		// A taken shortcut returns before the resets below, so the counters it
		// does not set itself start at zero here.
		p.lexCalls, p.tokens, p.compressions = 0, 0, 0
		if p.tryShortcut(src, old) {
			return ""
		}
		attemptTokens, attemptCalls = p.tokens, p.lexCalls
		if p.stop == stopProgress {
			return p.stop
		}
	}
	if p.led.Active() {
		p.opCount = 0
	}
	p.reusedNodes, p.reusedBytes, p.largestReuse = 0, 0, 0
	p.nodes = p.nodes[:0]
	p.free = p.free[:0]
	p.heads = p.heads[:0]
	p.finished, p.cacheTok, p.cacheAt, p.accepts, p.stop = 0, 0, 0, 0, ""
	p.shifts, p.reductions, p.lexCalls, p.tokens, p.maxVersions, p.multiTokens = 0, 0, attemptCalls, attemptTokens, 0, 0
	p.maxProcessed, p.selectCandidates = 0, 0
	p.compressions = 0
	p.extReset(old == 0)
	defer p.extFinish()
	base := p.nodeNew(-1, 0, false, 1)
	p.nodes[base].refCount++ // ts_stack_clear retains the base node
	if len(p.heads) == cap(p.heads) {
		p.heads = grow(&p.led, p.heads, 1)
	}
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
				// The version count at the "process version" event, as C logs it
				// at C/parser.c:2180-2186, counted with logging off too.
				if len(p.heads) > p.maxProcessed {
					p.maxProcessed = len(p.heads)
				}
				if p.logf != nil {
					p.log("process version:%d, version_count:%d, state:%d, row:%d, col:%d", version, len(p.heads), n.state, n.position.row, n.position.col)
				}
				if !p.advance(version, versionCount == 1, lastPosition) {
					return p.stop
				}
				h := &p.heads[version]
				position = p.nodes[h.node].position.bytes
				if h.status == headAwaitingSkip {
					// Preserve the skipped version's logical end while its sibling lexes.
					position += p.totalSize(h.lookaheadWhenPaused).bytes
				}
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
	if !p.balance() {
		p.finished = 0 // a stopped parse returns no tree
		p.stop = stopProgress
		return p.stop
	}
	if p.logf != nil {
		p.log("done")
	}
	return ""
}

// stopProgress is the stop text of a parse that the Progress hook ended.
const stopProgress = "stopped"

// operationsPerCheck is C OP_COUNT_PER_PARSER_CALLBACK_CHECK (C/parser.c:81).
const operationsPerCheck = 100

// checkProgress is C ts_parser__check_progress without the position fields:
// C polls parse_options.progress_callback every 100 operations (called from
// ts_parser__advance and from ts_parser__balance_subtree). Here the poll is the
// ledger's limit check (Ledger().SetPoll); a parse that it stops returns no tree
// and the text "stopped". With no limit the call sites cost one nil check.
func (p *Parser) checkProgress(operations int) bool {
	p.opCount += operations
	if p.opCount < operationsPerCheck {
		return true
	}
	return p.checkProgressNow()
}

// checkProgressNow asks the hook at once, whatever the operation count.
func (p *Parser) checkProgressNow() bool {
	p.opCount = 0
	if p.led.Check(false) {
		return true
	}
	p.stop = stopProgress
	return false
}

// bufferBytes is the memory the parser's own buffers hold, by capacity: the
// stack nodes, heads (with their recovery summaries), slices, iterators and the
// scratch arrays of the parse loop, the recovery code and the reuse walk. It
// does not include the arena (capBytes) or the payload the language's own scanner holds.
func (p *Parser) bufferBytes() int {
	const hs = int(unsafe.Sizeof(Handle(0)))
	n := cap(p.nodes)*int(unsafe.Sizeof(stackNode{})) + cap(p.free)*4
	n += cap(p.heads) * int(unsafe.Sizeof(stackHead{}))
	for _, h := range p.heads[:cap(p.heads)] {
		n += cap(h.summary) * int(unsafe.Sizeof(summaryEntry{}))
	}
	n += cap(p.slices)*int(unsafe.Sizeof(stackSlice{})) + cap(p.recSlices)*int(unsafe.Sizeof(stackSlice{}))
	for _, sl := range p.slices[:cap(p.slices)] {
		n += cap(sl.subtrees) * hs
	}
	for _, sl := range p.recSlices[:cap(p.recSlices)] {
		n += cap(sl.subtrees) * hs
	}
	n += cap(p.iters) * int(unsafe.Sizeof(stackIter{}))
	for _, it := range p.iters[:cap(p.iters)] {
		n += cap(it.subtrees) * hs
	}
	n += cap(p.bufs) * int(unsafe.Sizeof([]Handle(nil)))
	for _, b := range p.bufs[:cap(p.bufs)] {
		n += cap(b) * hs
	}
	n += (cap(p.trailing) + cap(p.trail2) + cap(p.scratch) + cap(p.compressTmp)) * hs
	n += cap(p.seen) * hs
	n += cap(p.reachBits) * int(unsafe.Sizeof(uint64(0))) // the mark bits of findMultiBits
	n += cap(p.reduceSet)*int(unsafe.Sizeof(reduceAction{})) + cap(p.rn)*int(unsafe.Sizeof(reusableEntry{}))
	// the scanner side the core owns: the serialized state of every external leaf,
	// the serialization buffer and the derived valid-symbol rows (not the payload the
	// language's own scanner holds)
	n += cap(p.ext.buf) + cap(p.ext.rows)*int(unsafe.Sizeof([]bool(nil)))
	for _, r := range p.ext.rows {
		n += cap(r)
	}
	return n
}

// BufferBytes is the memory the parser's own buffers hold (Footprint without the
// store's pages). A pooled parser is judged by this number: the store's pages
// belong to the trees that keep them.
func (p *Parser) BufferBytes() int { return p.bufferBytes() }

// Footprint is the complete memory the parser holds during or after a parse:
// the store's pages plus every buffer of the parser, by capacity. (The ledger, not this
// number, counts the arrays that growth abandoned during a request.)
func (p *Parser) Footprint() int { return p.capBytes() + p.bufferBytes() }

// FinishScanner destroys the external scanner's payload and lets it drop its
// reference to the source. A parse does it when it ends; the caller repeats it
// after a panic, which can leave the scanner in the middle of a scan.
func (p *Parser) FinishScanner() { p.extFinish() }

// clearStaleHeaders zeroes the headers the last parse left in the parser's own
// arrays. A stale slice or iterator header keeps a subtree buffer that bufs also
// owns; zeroed, each buffer has one owner and bufferBytes counts it once. It
// must run only between parses, when none of these headers is live. Reset
// calls it, and so does a request with a limit before it takes the base of its
// account: a parser reused without Reset would otherwise start the request with
// its buffers counted two or three times and be stopped for it.
func (p *Parser) clearStaleHeaders() {
	clear(p.heads[:cap(p.heads)])
	clear(p.slices[:cap(p.slices)])
	clear(p.iters[:cap(p.iters)])
	clear(p.recSlices[:cap(p.recSlices)])
	clear(p.bufs[len(p.bufs):cap(p.bufs)])
}

// Reset drops the parser's view of the store and its reference to the last source,
// so a pooled parser holds neither a tree nor a document. The buffers keep their
// capacity; BufferBytes tells how much that is.
func (p *Parser) Reset() {
	p.clearStaleHeaders()
	p.Store = Store{} // a pooled parser must not point at the store of a tree that a public tree still keeps
	p.ext.cacheLast = 0
	p.finished = 0
	p.src = nil
	p.scanProv = nil
	p.cacheTok, p.cacheAt = 0, 0
}
