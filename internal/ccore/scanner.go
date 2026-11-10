package ccore

// External scanner support: a function-by-function port of the external-scanner
// parts of tree-sitter 0.27.1, cited as C/<file>:<line>.
//
//   C/parser.c:357  ts_parser__external_scanner_create      -> ExtScanner (lazy, in the adapter)
//   C/parser.c:374  ts_parser__external_scanner_destroy     -> extFinish, ExtScanner.Close
//   C/parser.c:390  ts_parser__external_scanner_serialize   -> ExtScanner.Scan (second step)
//   C/parser.c:413  ts_parser__external_scanner_deserialize -> ExtScanner.Scan (first step)
//   C/parser.c:443  ts_parser__external_scanner_scan        -> ExtScanner.Scan
//   C/parser.c:505  ts_parser__lex, external branch         -> lexExternal
//   C/stack.c:482   ts_stack_last_external_token            -> heads[v].lastExternal
//   C/stack.c:486   ts_stack_set_last_external_token         -> extShift
//   C/stack.c:647   ts_stack_has_advanced_since_error        -> hasAdvancedSinceError
//   C/subtree.c:60  ts_external_scanner_state_eq             -> extStateEq
//   C/subtree.c:801 ts_subtree_last_external_token           -> lastExternalToken
//   C/subtree.c:1074 ts_subtree_external_scanner_state       -> extState
//   C/subtree.c:349 ts_subtree_summarize_children (flags)    -> extSummarize
//
// Go constructs that differ from C, semantics kept:
//   - Scanner state bytes of a leaf live in a side arena (ext.bytes). The
//     heap leaf record keeps offset and length in two fields a leaf does not
//     otherwise use (visibleDescendantCount, namedChildCount), as C's
//     SubtreeHeapData unions the children pointer with the scanner state.
//   - The core owns C's call rules: the valid-symbol row, the empty-token rule
//     and the state comparison. One ExtScanner.Scan call performs C's
//     deserialize, scan and serialize in that order.
//   - The scan runs without the read-frontier observer: the lookahead end
//     byte is the lexer cursor plus one (plus four after an invalid UTF-8
//     lookahead), as C's ts_lexer_finish computes it.

import "bytes"

// ExtScan is the result of one external scan.
type ExtScan struct {
	Token        Token
	StateLen     int    // bytes of the serialized scanner state written to buf
	LookaheadEnd uint32 // C ts_lexer_finish lookahead end, set even when no token was found
	DidGetColumn bool   // the scanner read the column (C Lexer.did_get_column)
}

// ExtScanner runs a language's external scanner.
//
// Scan performs, in C's order: (1) deserialize prev into the scanner (C
// ts_parser__external_scanner_deserialize), (2) scan from src[pos] with the
// valid-symbol row (C ts_parser__external_scanner_scan), (3) when a token was
// found, serialize the scanner state into buf and set out.StateLen (C
// ts_parser__external_scanner_serialize). out.LookaheadEnd and
// out.DidGetColumn are set in every case. The scanner payload is created on
// the first Scan after construction or Close (C external_scanner_create).
// Close destroys the payload if one exists (C external_scanner_destroy).
type ExtScanner interface {
	Scan(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ExtScan) bool
	Close()
}

// heapExternalStateChange is C's has_external_scanner_state_change (heap flag).
const heapExternalStateChange uint16 = 1 << 10

const extSerializationBufferSize = 4096 // C: TREE_SITTER_SERIALIZATION_BUFFER_SIZE is 1024; Go scanners may write more

// scannerState is the scanner side of Parser.
type scannerState struct {
	bytes     []byte // side arena: serialized scanner state of every external leaf
	buf       []byte
	cacheLast Handle   // C TokenCache.last_external_token
	scans     int      // statistics (not in C)
	rows      [][]bool // valid-symbol rows derived from the action table (Tables.ExternalLexStates empty)
	rowsDone  bool
	active    bool // some lex mode has an external lex state; false: every hook returns at once
	inited    bool
	res       ExtScan // scratch for one scan
}

// extResult is the result of one successful external scan.
type extResult struct {
	tok     Token
	off, n  uint32
	changed bool
}

func (p *Parser) extReset(fresh bool) {
	if !p.ext.inited {
		p.ext.inited = true
		for _, m := range p.t.LexModes {
			if m.ExternalLexState != 0 {
				p.ext.active = true
				break
			}
		}
	}
	if fresh { // an incremental parse keeps the old tree's scanner states (same lifetime as the record arena)
		p.ext.bytes = p.ext.bytes[:0]
	}
	p.ext.cacheLast = 0
	p.ext.scans = 0
}

// extRows is C/language.h ts_language_enabled_external_tokens' table: the
// valid external tokens of each external lex state. Tables.ExternalLexStates
// is used when the blob has it; some blobs lack it, and
// then the rows are derived the way tree-sitter's generator derives them: an
// external token is valid in a state when the state has an action for it, and
// every token is valid in the error state's external lex state.
func (p *Parser) extRows() [][]bool {
	if len(p.t.ExternalLexStates) > 0 && !p.forceRows {
		return p.t.ExternalLexStates
	}
	e := &p.ext
	if e.rowsDone {
		return e.rows
	}
	e.rowsDone = true
	maxID := 0
	for _, m := range p.t.LexModes {
		if int(m.ExternalLexState) > maxID {
			maxID = int(m.ExternalLexState)
		}
	}
	e.rows = make([][]bool, maxID+1)
	for id := range e.rows {
		e.rows[id] = make([]bool, len(p.t.ExternalSymbols))
	}
	for s, m := range p.t.LexModes {
		if m.ExternalLexState == 0 {
			continue
		}
		for i, sym := range p.t.ExternalSymbols {
			if s == 0 || p.hasActions(uint16(s), sym) {
				e.rows[m.ExternalLexState][i] = true
			}
		}
	}
	return e.rows
}

// extFinish is ts_parser__external_scanner_destroy.
func (p *Parser) extFinish() {
	if p.scanner != nil {
		p.scanner.Close()
	}
}

// extState is ts_subtree_external_scanner_state: only a heap leaf with
// has_external_tokens has state; everything else has the empty state.
func (p *Parser) extState(h Handle) []byte {
	if h == 0 || h&1 != 0 {
		return nil
	}
	r := p.rec(h)
	if r.flags&heapHasExternalTokens == 0 || r.childCount != 0 {
		return nil
	}
	return p.ext.bytes[r.visibleDescendantCount : r.visibleDescendantCount+r.namedChildCount]
}

// extStateEq is ts_subtree_external_scanner_state_eq.
func (p *Parser) extStateEq(a, b Handle) bool {
	if a == b || !p.ext.active {
		return true
	}
	return bytes.Equal(p.extState(a), p.extState(b))
}

// lastExternalToken is C/subtree.c:801 ts_subtree_last_external_token.
func (p *Parser) lastExternalToken(tree Handle) Handle {
	if !p.hasExternalTokens(tree) {
		return 0
	}
	for p.rec(tree).childCount > 0 {
		kids := p.children(tree)
		next := Handle(0)
		for i := len(kids) - 1; i >= 0; i-- {
			if p.hasExternalTokens(kids[i]) {
				next = kids[i]
				break
			}
		}
		if next == 0 {
			break
		}
		tree = next
	}
	return tree
}

// extShift is the tail of C/parser.c:914 ts_parser__shift.
func (p *Parser) extShift(version int, pushed Handle) {
	if p.ext.active && p.hasExternalTokens(pushed) {
		p.heads[version].lastExternal = p.lastExternalToken(pushed)
	}
}

// extSummarize is the external part of C/subtree.c:349
// ts_subtree_summarize_children for one child. It runs before the child's
// size is added, as in C: the row test reads the size of the earlier children.
func (p *Parser) extSummarize(self *rec, child Handle, first bool) {
	if (first || self.size.row == 0) && p.dependsOnColumn(child) {
		self.flags |= heapDependsOnColumn
	}
	if p.hasExternalStateChange(child) {
		self.flags |= heapExternalStateChange
	}
	if p.hasExternalTokens(child) {
		self.flags |= heapHasExternalTokens
	}
}

// extAttach is the tail of C/parser.c:685 for a found external token:
// ts_external_scanner_state_init and has_external_scanner_state_change.
func (p *Parser) extAttach(leaf Handle, e *extResult) {
	r := p.rec(leaf)
	r.visibleDescendantCount, r.namedChildCount = e.off, e.n
	if e.changed {
		r.flags |= heapExternalStateChange
	}
}

// hasAdvancedSinceError is C/stack.c:647 ts_stack_has_advanced_since_error.
func (p *Parser) hasAdvancedSinceError(version int) bool {
	h := &p.heads[version]
	node := h.node
	if p.nodes[node].errorCost == 0 {
		return true
	}
	for node >= 0 {
		n := &p.nodes[node]
		if n.linkCount > 0 {
			sub := n.links[0].sub
			if sub != 0 {
				if p.totalSize(sub).bytes > 0 {
					return true
				} else if n.nodeCount > h.nodeCountAtLastError && p.errorCost(sub) == 0 {
					node = n.links[0].node
					continue
				}
			}
		}
		break
	}
	return false
}

// lexExternal is the external branch of C/parser.c:505 ts_parser__lex (one
// pass of the for(;;) loop). cur is the lexer's current position at the top of
// the pass. It returns true when a token was found (out is filled). A false
// return with p.stop set ends the parse.
func (p *Parser) lexExternal(version int, parseState uint16, mode LexMode, cur Length, errorMode bool, out *extResult, lookaheadEnd *uint32, calledGetColumn *bool) bool {
	e := &p.ext
	rows := p.extRows()
	if p.scanner == nil || int(mode.ExternalLexState) >= len(rows) {
		p.stop = "needs scanner: external lex state without scanner or valid-symbol row"
		return false
	}
	if p.logf != nil {
		p.log("lex_external state:%d, row:%d, column:%d", mode.ExternalLexState, cur.row, cur.col)
	}
	if e.buf == nil {
		e.buf = make([]byte, extSerializationBufferSize)
	}
	last := p.heads[version].lastExternal
	prev := p.extState(last)
	e.scans++
	res := &e.res
	*res = ExtScan{}
	found := p.scanner.Scan(p.src, int(cur.bytes), cur.row, cur.col, prev, rows[mode.ExternalLexState], e.buf, res)
	*calledGetColumn = *calledGetColumn || res.DidGetColumn
	if res.LookaheadEnd > *lookaheadEnd { // ts_lexer_finish
		*lookaheadEnd = res.LookaheadEnd
	}
	if !found {
		return false
	}
	tok := res.Token
	n := res.StateLen
	state := e.buf[:n]
	changed := !bytes.Equal(prev, state)
	// Ignore empty external tokens (C/parser.c:557-583).
	if tok.EndByte <= cur.bytes && !changed {
		next := p.nextState(parseState, tok.Symbol)
		tokenIsExtra := next == parseState
		if errorMode || !p.hasAdvancedSinceError(version) || tokenIsExtra {
			if p.logf != nil {
				p.log("ignore_empty_external_token symbol:%s", p.symName(tok.Symbol))
			}
			return false
		}
	}
	out.tok, out.changed = tok, changed
	out.off, out.n = uint32(len(e.bytes)), uint32(n)
	e.bytes = append(e.bytes, state...)
	return true
}
