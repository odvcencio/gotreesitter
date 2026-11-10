package ccore

// Lexing: C/parser.c ts_parser__lex (lex modes, error retry, error skip,
// keyword promotion, token cache, leaf construction) is core code. The
// automaton walk is lexdfa.go.

import "unicode/utf8"

// Token is one lexed token.
type Token struct {
	Symbol       Symbol
	StartByte    uint32
	EndByte      uint32
	StartRow     uint32
	StartCol     uint32
	EndRow       uint32
	EndCol       uint32
	LookaheadEnd uint32 // the farthest byte inspected while selecting the token
}

// scan runs the main lex automaton (the generated ts_lex of the grammar) and
// records where a failed attempt began and ended.
func (p *Parser) scan(lexState uint32, pos int, row, col uint32, out *Token) bool {
	return scanDFA(p.t.LexStates, p.t.LexAscii, p.src, lexState, pos, row, col, out, &p.fail)
}

// C/parser.c:505 ts_parser__lex: external scan, internal scan, error-mode
// retry, error skip, keyword promotion and leaf construction.
func (p *Parser) lex(version int, parseState uint16) (Handle, bool) {
	p.lexCalls++
	if int(parseState) >= len(p.t.LexModes) {
		p.stop = "needs recovery: no lex mode"
		return 0, false
	}
	mode := p.t.LexModes[parseState]
	if mode.LexState == 0xFFFF {
		if p.logf != nil {
			p.log("no_lookahead_after_non_terminal_extra")
		}
		return 0, true
	}
	start := p.nodes[p.heads[version].node].position
	pos, row, col := int(start.bytes), start.row, start.col
	errorMode := parseState == 0
	tok := &p.lexTok // scratch in the Parser: a pointer passed to an interface method would escape
	*tok = Token{}
	var lookaheadEnd uint32 // C lookahead_end_byte
	var foundExternal, calledGetColumn bool
	var ext extResult
retry:
	if mode.ExternalLexState != 0 {
		if foundExternal = p.lexExternal(version, parseState, mode, start, errorMode, &ext, &lookaheadEnd, &calledGetColumn); foundExternal {
			*tok = ext.tok
		} else if p.stop != "" {
			return 0, false
		}
	}
	if !foundExternal && p.logf != nil {
		p.log("lex_internal state:%d, row:%d, column:%d", mode.LexState, start.row, start.col)
	}
	for !foundExternal {
		lexed := p.scan(mode.Index, pos, row, col, tok)
		if !lexed && errorMode && pos >= len(p.src) {
			// HEURISTIC. In C the error-state lex function accepts the end token at
			// the end of input, so the error skip below never starts there. A blob
			// from the grammar generator can leave end-token acceptance out of its
			// lex states (it keeps it in the parser's own end check), and then the
			// walk fails at the end of input and the error skip would add empty
			// ERROR leaves for ever. The walk's failure is read as the empty end
			// token, which is what the earlier root kernel returned at the end of
			// input.
			lexed = true
			*tok = Token{}
		}
		if lexed && pos >= len(p.src) {
			// C/lexer.c:251-263: the walk ran at the end of input; if it accepted no
			// token, the end token is empty and the lookahead end is one byte past.
			if tok.Symbol == 0 {
				*tok = Token{StartByte: uint32(pos), EndByte: uint32(pos), StartRow: row, StartCol: col, EndRow: row, EndCol: col, LookaheadEnd: uint32(pos) + 1}
			}
			break
		}
		if !lexed {
			// C/parser.c:601 ts_lexer_finish runs after a failed main lex too, so its
			// lookahead end counts toward the token that is lexed next.
			if tok.LookaheadEnd > lookaheadEnd {
				lookaheadEnd = tok.LookaheadEnd
			}
			if !errorMode {
				// C/parser.c:604-609: retry from the start in the error state's lex mode.
				errorMode = true
				mode = p.t.LexModes[0]
				pos, row, col = int(start.bytes), start.row, start.col
				goto retry
			}
			// C/parser.c:611-627: skip characters into an ERROR leaf (lexErrorSkip below).
			result := p.lexErrorSkip(version, parseState, start, mode, lookaheadEnd, &calledGetColumn)
			if p.stop != "" {
				return 0, false
			}
			if p.logf != nil {
				p.log("lexed_lookahead sym:%s, size:%d", escape(p.symName(symError)), p.totalSize(result).bytes)
			}
			return result, true
		}
		if tok.Symbol == 0 {
			if int(tok.EndByte) <= pos {
				p.stop = "needs recovery: lexer made no progress"
				return 0, false
			}
			pos, row, col = int(tok.EndByte), tok.EndRow, tok.EndCol
			continue
		}
		break
	}
	tokStart := Length{bytes: tok.StartByte, row: tok.StartRow, col: tok.StartCol}
	tokEnd := Length{bytes: tok.EndByte, row: tok.EndRow, col: tok.EndCol}
	padding := lenSub(tokStart, start)
	size := lenSub(tokEnd, tokStart)
	if tok.LookaheadEnd > lookaheadEnd {
		lookaheadEnd = tok.LookaheadEnd
	}
	var lookahead uint32
	if lookaheadEnd > tok.EndByte {
		lookahead = lookaheadEnd - tok.EndByte
	}
	sym := tok.Symbol
	isKeyword := false
	if !foundExternal && sym == p.t.KeywordCapture && sym != 0 && len(p.t.KeywordLexStates) != 0 {
		kt := &p.kwTok
		*kt = Token{}
		if scanDFA(p.t.KeywordLexStates, p.t.KeywordLexAscii, p.src, 0, int(tok.StartByte), tok.StartRow, tok.StartCol, kt, &p.kwFail) && kt.Symbol != 0 {
			isKeyword = true
			if kt.EndByte == tok.EndByte && (p.hasActions(parseState, kt.Symbol) || p.isReserved(parseState, kt.Symbol)) {
				sym = kt.Symbol
			}
		}
	}
	result := p.newLeaf(sym, padding, size, lookahead, parseState, isKeyword, foundExternal, calledGetColumn)
	if foundExternal {
		p.extAttach(result, &ext)
	}
	if p.logf != nil {
		p.log("lexed_lookahead sym:%s, size:%d", escape(p.symName(p.symbol(result))), p.totalSize(result).bytes)
	}
	return result, true
}

// C/parser.c:470 ts_parser__can_reuse_first_leaf.
func (p *Parser) canReuseFirstLeaf(state uint16, tree Handle, entry actionEntry) bool {
	leafSym := p.leafSymbol(tree)
	leafState := p.leafParseState(tree)
	cur := p.t.LexModes[state]
	leaf := p.t.LexModes[leafState]
	if cur.LexState == 0xFFFF {
		return false
	}
	if len(entry.actions) > 0 && leaf == cur &&
		(leafSym != p.t.KeywordCapture || (!p.isKeyword(tree) && p.parseState(tree) == state)) {
		return true
	}
	if p.size(tree).bytes == 0 && leafSym != 0 {
		return false
	}
	return cur.ExternalLexState == 0 && entry.reusable
}

// C/parser.c:703 ts_parser__get_cached_token.
func (p *Parser) cachedToken(state uint16, position uint32, lastExternal Handle, entry *actionEntry) Handle {
	if p.cacheTok != 0 && p.cacheAt == position && p.extStateEq(p.ext.cacheLast, lastExternal) {
		*entry = p.tableEntry(state, p.symbol(p.cacheTok))
		if p.canReuseFirstLeaf(state, p.cacheTok, *entry) {
			return p.cacheTok
		}
	}
	return 0
}

// lexErrorSkip is the error branch of C/parser.c:505 ts_parser__lex after
// the lex in the error state's mode failed: skip characters until a token
// can be lexed (or the input ends) and return them as one ERROR leaf.
func (p *Parser) lexErrorSkip(version int, parseState uint16, start Length, mode LexMode, lookaheadEnd uint32, calledGetColumn *bool) Handle {
	if p.logf != nil {
		p.log("skip_unrecognized_character")
	}
	errStart, cur := p.fail[0], p.fail[1]
	errEnd := errStart
	// The failed scans' lookahead ends are already in lookaheadEnd (C/parser.c:601).
	for {
		if cur.bytes == errEnd.bytes {
			if int(cur.bytes) >= len(p.src) {
				break
			}
			r, size := utf8.DecodeRune(p.src[cur.bytes:])
			cur.bytes += uint32(size)
			if r == '\n' {
				cur.row++
				cur.col = 0
			} else {
				cur.col += uint32(size)
			}
		}
		errEnd = cur
		if mode.ExternalLexState != 0 { // loop top of C/parser.c:505: the scanner runs again after each skipped character
			var ext extResult
			if p.lexExternal(version, parseState, mode, cur, true, &ext, &lookaheadEnd, calledGetColumn) {
				break
			}
			if p.stop != "" {
				break
			}
		}
		if p.logf != nil {
			p.log("lex_internal state:%d, row:%d, column:%d", mode.LexState, cur.row, cur.col)
		}
		found, failEnd, la := p.scanFrom(mode, cur)
		if la > lookaheadEnd {
			lookaheadEnd = la
		}
		if found {
			break
		}
		cur = failEnd
	}
	padding := lenSub(errStart, start)
	size := lenSub(errEnd, errStart)
	var lookahead uint32
	if lookaheadEnd > errEnd.bytes {
		lookahead = lookaheadEnd - errEnd.bytes
	}
	return p.newErrorLeaf(padding, size, lookahead, parseState)
}

// scanFrom runs the lexer from a position as the clean path's lex does
// (whitespace skips, end of input); it reports whether a token was found,
// and on failure the position where the failed scan stopped.
func (p *Parser) scanFrom(mode LexMode, at Length) (bool, Length, uint32) {
	pos, row, col := int(at.bytes), at.row, at.col
	tok := &p.skipTok
	*tok = Token{}
	for {
		if pos >= len(p.src) {
			return true, Length{}, uint32(pos) + 1
		}
		if !p.scan(mode.Index, pos, row, col, tok) {
			return false, p.fail[1], tok.LookaheadEnd
		}
		if tok.Symbol == 0 {
			if int(tok.EndByte) <= pos {
				return false, Length{bytes: uint32(pos), row: row, col: col}, tok.LookaheadEnd
			}
			pos, row, col = int(tok.EndByte), tok.EndRow, tok.EndCol
			continue
		}
		return true, Length{}, tok.LookaheadEnd
	}
}
