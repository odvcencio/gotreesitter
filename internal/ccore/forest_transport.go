package ccore

// ForestElection holds the scanner binding for one fresh deterministic forest parse.
// The scanner supplies one authoritative token per shift and never enters error mode.
type ForestElection struct {
	// The compact records may cross into the ordinary incremental parser only if
	// the elected leaves, action choices and version scheduling matched its semantics.
	diverged, scalarLeaf            bool
	dominatedPauses, discardedHeads int
	numericPosition                 uint32
	Resolve                         func(state uint16, n int) int // -1 retains the action set; -2 abstains on an unmappable policy
	Next                            func(states []uint16) (t Token, external, keyword, noLookahead bool)
	CanShift                        func(byteOffset uint32) bool
	States                          []uint16
	Token                           Token
	Stop                            string
	Calls                           uint32
	WorkLimit, ReduceLimit          int
	work, reductions                int
	external, keyword               bool
	MultiElections                  uint32
	MaxHeads                        int
}

// ParseFreshForest binds one scanner for the lifetime of a fresh compact forest request.
func (p *Parser) ParseFreshForest(src []byte, e *ForestElection) (*Tree, string) {
	p.forestElection = e
	defer func() { p.forestElection = nil }()
	return p.Parse(src)
}

func (e *ForestElection) OrdinaryCompatible() bool {
	return e.Stop == "" && e.scalarLeaf && !e.diverged && e.dominatedPauses == e.discardedHeads
}

func (p *Parser) canDiscardNumericPause(version int, state uint16, lookahead Handle) bool {
	if version != 1 || len(p.heads) != 2 || p.heads[0].status != headForestShifted || state == 0 || lookahead == 0 || p.reuseActive {
		return false
	}
	if p.isKeyword(lookahead) && p.symbol(lookahead) != p.t.KeywordCapture && !p.isReserved(state, p.symbol(lookahead)) && len(p.tableEntry(state, p.t.KeywordCapture).actions) != 0 {
		return false
	}
	// Reconstruct the exact head state the native no-action case feeds condense.
	// With two heads and takeLeft the losing head is deleted before any recovery or merge.
	a, b := p.heads[0], p.heads[1]
	p.heads[0].status = headActive
	p.heads[1].status = headPaused
	p.heads[1].lookaheadWhenPaused = lookahead
	p.heads[1].nodeCountAtLastError = p.nodes[b.node].nodeCount
	dominated := compareVersions(p.versionStatus(0), p.versionStatus(1)) == takeLeft
	p.heads[0], p.heads[1] = a, b
	return dominated
}

func (p *Parser) noteForestTick(i int) {
	e := p.forestElection
	if len(p.heads) > 2 {
		e.diverged = true
	}
	h := p.heads[i]
	pos := p.nodes[h.node].position.bytes
	if pos > e.numericPosition || (i > 0 && pos == e.numericPosition) {
		e.numericPosition = pos
		return
	}
	// The ordinary scheduler immediately requests the next token on a singleton
	// zero-width shift. Condensing one healthy active head has no observable effect.
	if h.status == headForestShifted && (len(p.heads) != 1 || pos != e.numericPosition || p.nodes[h.node].state == 0 || i != 0) {
		e.diverged = true
	}
}

func (p *Parser) forestLex(version int, state uint16) (Handle, bool) {
	e := p.forestElection
	t, external, keyword := e.Token, e.external, e.keyword
	start := p.nodes[p.heads[version].node].position
	if t.StartByte < start.bytes || t.EndByte < t.StartByte || int(t.EndByte) > len(p.src) {
		p.stop = "forest_invalid_token"
		e.Stop = p.stop
		return 0, false
	}
	first := Length{bytes: t.StartByte, row: t.StartRow, col: t.StartCol}
	end := Length{bytes: t.EndByte, row: t.EndRow, col: t.EndCol}
	var lookahead uint32
	if t.LookaheadEnd > t.EndByte {
		lookahead = t.LookaheadEnd - t.EndByte
	}
	// Native marks a capture leaf when the keyword DFA recognizes even a prefix.
	// Promotion of the public token symbol is a separate decision.
	if !external && !keyword && t.Symbol != 0 && t.Symbol == p.t.KeywordCapture && len(p.t.KeywordLexStates) != 0 {
		kt := &p.kwTok
		*kt = Token{}
		keyword = scanDFA(p.t.KeywordLexStates, p.t.KeywordLexAscii, p.src, 0, int(t.StartByte), t.StartRow, t.StartCol, kt, &p.kwFail) && kt.Symbol != 0
	}
	pad, size := lenSub(first, start), lenSub(end, first)
	if !e.diverged {
		// Reuse the canonical scalar leaf itself when its lexical contract matches the
		// strict forest election. Only the scalar lexer runs here; the forest owns all actions.
		priorStop := p.stop
		n, ok := p.nativeLex(version, state)
		if !ok && p.led.Active() && !p.checkProgressNow() {
			return 0, false
		}
		p.stop = priorStop
		if ok && n != 0 && p.symbol(n) == t.Symbol && p.parseState(n) == state &&
			p.padding(n) == pad && p.size(n) == size && p.lookaheadBytes(n) == lookahead &&
			p.isKeyword(n) == keyword && p.hasExternalTokens(n) == external && !p.hasExternalStateChange(n) &&
			!p.dependsOnColumn(n) && !p.missing(n) && p.extra(n) == (t.Symbol == 0) && p.errorCost(n) == 0 && p.extStateEq(n, 0) {
			e.scalarLeaf = true
			return n, true
		}
		e.diverged = true
	}
	return p.newLeaf(t.Symbol, pad, size, lookahead, state, keyword, external, false), true
}

func (p *Parser) forestVisit(state uint16, actions []Action) bool {
	e := p.forestElection
	e.work++
	var reduces int
	for _, a := range actions {
		if a.Type == ActionShift && a.Repetition {
			continue
		}
		if a.Type == ActionReduce {
			reduces++
		}
	}
	e.reductions += reduces
	switch {
	case e.work > e.WorkLimit:
		e.Stop = "worklist-cap"
	case e.reductions > e.ReduceLimit:
		e.Stop = "reduce-visit-cap"
	case p.led.Active() && !p.checkProgress(1):
		e.Stop = stopProgress
	}
	if e.Stop != "" {
		p.stop = e.Stop
		return false
	}
	for _, s := range e.States {
		if s == state {
			return true
		}
	}
	if len(e.States) == cap(e.States) {
		e.States = grow(&p.led, e.States, 1)
	}
	e.States = append(e.States, state)
	return true
}

// forestEpochs elects a token once for the current heads. A shifted head waits
// until every unshifted sibling has finished its reductions on this token.
func (p *Parser) forestEpochs() string {
	e := p.forestElection
	for len(p.heads) > 0 {
		epochOffset := p.nodes[p.heads[0].node].position.bytes
		if len(p.heads) > 2 {
			e.diverged = true
		}
		for _, h := range p.heads {
			if p.nodes[h.node].position.bytes != epochOffset {
				e.diverged = true
				break
			}
		}
		e.States = e.States[:0]
		for i := range p.heads {
			state := p.nodes[p.heads[i].node].state
			if len(e.States) == cap(e.States) {
				e.States = grow(&p.led, e.States, 1)
			}
			e.States = append(e.States, state)
		}
		if len(e.States) > 1 {
			e.MultiElections++
		}
		if len(e.States) > p.maxVersions {
			p.maxVersions = len(e.States)
		}
		if len(e.States) > e.MaxHeads {
			e.MaxHeads = len(e.States)
		}
		var noLookahead bool
		e.Token, e.external, e.keyword, noLookahead = e.Next(e.States)
		e.Calls++
		if e.Stop != "" {
			return e.Stop
		}
		if e.Token.EndByte == epochOffset && len(p.heads) > 1 {
			e.diverged = true
		}
		e.work, e.reductions = 0, 0
		if noLookahead {
			e.Stop = "forest_no_lookahead"
			return e.Stop
		}
		for i := 0; i < len(p.heads); i++ {
			if len(p.heads) > p.maxVersions {
				p.maxVersions = len(p.heads)
			}
			if p.heads[i].status == headActive {
				if !p.advance(i, false, 0) {
					return p.stop
				}
				if !e.diverged {
					p.noteForestTick(i)
				}
			}
			if e.Stop != "" {
				return e.Stop
			}
		}
		if p.finished != 0 && (len(p.heads) != 1 || e.Token.Symbol != 0) {
			e.diverged = true
		}
		for i := len(p.heads) - 1; i >= 0; i-- {
			switch p.heads[i].status {
			case headForestShifted:
				p.heads[i].status = headActive
			case headHalted:
				if p.finished == 0 {
					e.discardedHeads++
				}
				p.removeVersion(i)
			default:
				e.Stop = "forest_unprocessed_version"
				return e.Stop
			}
		}
		if p.finished != 0 {
			for len(p.heads) > 0 {
				p.removeVersion(len(p.heads) - 1)
			}
			return ""
		}
		p.condense()
		if e.Stop != "" {
			return e.Stop
		}
	}
	e.Stop = "dead_end"
	return e.Stop
}
