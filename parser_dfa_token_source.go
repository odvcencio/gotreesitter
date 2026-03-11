package gotreesitter

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

type dfaTokenSource struct {
	lexer    *Lexer
	language *Language
	state    StateID

	lookupActionIndex func(state StateID, sym Symbol) uint16
	hasKeywordState   []bool
	externalPayload   any
	externalValid     []bool
	glrStates         []StateID // all active GLR stack states

	// Zero-width external token loop prevention.
	// Tracks which external token indices have been produced as zero-width
	// tokens at the current (position, state) pair, so they can be excluded
	// from validSymbols on subsequent calls. This prevents infinite loops
	// when the parser has no action for a zero-width external token and the
	// state remains unchanged.
	extZeroPos   int
	extZeroState StateID
	extZeroTried []bool

	// Zero-width token guard for all token kinds (DFA + external).
	// Some grammars can emit endless zero-width marker/token sequences at the
	// same byte offset (often alternating symbols/states). Cap consecutive
	// emissions so tokenization always makes forward progress.
	zeroWidthPos   int
	zeroWidthCount int
}

const maxConsecutiveZeroWidthTokens = 4
const maxConsecutiveZeroWidthTokensExternal = 128
const maxConsecutiveZeroWidthTokensRepeatableExternal = 4096
const noLookaheadLexState = ^uint16(0)

var dfaTokenSourcePool = sync.Pool{
	New: func() any {
		return &dfaTokenSource{
			extZeroPos:   -1,
			zeroWidthPos: -1,
		}
	},
}

func acquireDFATokenSource(lexer *Lexer, language *Language, lookupActionIndex func(state StateID, sym Symbol) uint16, hasKeywordState []bool) *dfaTokenSource {
	ts := dfaTokenSourcePool.Get().(*dfaTokenSource)
	ts.lexer = lexer
	ts.language = language
	ts.state = 0
	ts.lookupActionIndex = lookupActionIndex
	ts.hasKeywordState = hasKeywordState
	ts.externalPayload = nil
	ts.glrStates = nil
	if len(ts.externalValid) > 0 {
		ts.externalValid = ts.externalValid[:0]
	}
	ts.extZeroPos = -1
	ts.extZeroState = 0
	if len(ts.extZeroTried) > 0 {
		ts.extZeroTried = ts.extZeroTried[:0]
	}
	ts.zeroWidthPos = -1
	ts.zeroWidthCount = 0
	if language != nil && language.ExternalScanner != nil {
		ts.externalPayload = language.ExternalScanner.Create()
	}
	return ts
}

func (d *dfaTokenSource) Close() {
	if d.language == nil || d.language.ExternalScanner == nil || d.externalPayload == nil {
		// still recycle the token source instance
		d.lexer = nil
		d.language = nil
		d.lookupActionIndex = nil
		d.hasKeywordState = nil
		d.glrStates = nil
		d.extZeroPos = -1
		d.extZeroState = 0
		d.zeroWidthPos = -1
		d.zeroWidthCount = 0
		dfaTokenSourcePool.Put(d)
		return
	}
	d.language.ExternalScanner.Destroy(d.externalPayload)
	d.externalPayload = nil
	d.lexer = nil
	d.language = nil
	d.lookupActionIndex = nil
	d.hasKeywordState = nil
	d.glrStates = nil
	d.extZeroPos = -1
	d.extZeroState = 0
	d.zeroWidthPos = -1
	d.zeroWidthCount = 0
	dfaTokenSourcePool.Put(d)
}

// DebugDFA enables trace logging for DFA token production.
//
// Use `DebugDFA.Store(true/false)` to toggle at runtime.
var DebugDFA atomic.Bool

func (d *dfaTokenSource) Next() Token {
	startPos := 0
	if perfCountersEnabled {
		startPos = d.lexer.pos
	}
	for {
		if d.shouldForceEOFLookahead() {
			tok := d.syntheticEOFLookaheadToken()
			if DebugDFA.Load() {
				fmt.Printf("  SYN tok %d  %d %d state=%d\n", tok.Symbol, tok.StartByte, tok.EndByte, d.state)
			}
			return tok
		}

		tok := Token{}
		tokenFromExternal := false
		if extTok, ok := d.nextExternalToken(); ok {
			tok = extTok
			tokenFromExternal = true
		} else if glrTok, ok := d.nextGLRUnionDFAToken(); ok {
			tok = glrTok
		} else {
			tok = d.nextDFAToken()
		}

		// Some grammars can emit zero-width non-EOF tokens that have no parse
		// action in any live GLR state. If returned as-is, parser recovery can
		// loop forever at the same byte. Skip one rune (or coerce EOF at end)
		// so the token source itself always guarantees forward progress.
		if tok.Symbol != 0 && tok.EndByte <= tok.StartByte && !d.hasAnyActionForSymbol(tok.Symbol) {
			if d.lexer.pos < len(d.lexer.source) {
				if DebugDFA.Load() {
					fmt.Printf("  ZERO-WIDTH skip sym=%d at pos=%d state=%d\n", tok.Symbol, d.lexer.pos, d.state)
				}
				d.extZeroPos = -1
				d.lexer.skipOneRune()
				continue
			}
			tok = d.eofTokenAtLexerPos()
		}

		if tok.Symbol != 0 && tok.EndByte <= tok.StartByte {
			if d.zeroWidthPos == d.lexer.pos {
				d.zeroWidthCount++
			} else {
				d.zeroWidthPos = d.lexer.pos
				d.zeroWidthCount = 1
			}
			limit := maxConsecutiveZeroWidthTokens
			if d.language != nil {
				switch {
				case d.language.Name == "yaml":
					limit = maxConsecutiveZeroWidthTokensExternal
				case d.allowRepeatedZeroWidthExternalSymbol(tok.Symbol):
					limit = maxConsecutiveZeroWidthTokensRepeatableExternal
				}
			}
			if d.zeroWidthCount > limit {
				if d.lexer.pos < len(d.lexer.source) {
					if DebugDFA.Load() {
						fmt.Printf("  ZERO-WIDTH cap skip at pos=%d state=%d sym=%d\n", d.lexer.pos, d.state, tok.Symbol)
					}
					d.extZeroPos = -1
					d.zeroWidthPos = -1
					d.zeroWidthCount = 0
					d.lexer.skipOneRune()
					continue
				}
				tok = d.eofTokenAtLexerPos()
				d.zeroWidthPos = -1
				d.zeroWidthCount = 0
			}
		} else {
			d.zeroWidthPos = -1
			d.zeroWidthCount = 0
		}

		if perfCountersEnabled {
			consumed := d.lexer.pos - startPos
			if consumed < 0 {
				consumed = 0
			}
			perfRecordLexed(consumed, 1)
		}
		if DebugDFA.Load() {
			name := ""
			if int(tok.Symbol) < len(d.language.SymbolNames) {
				name = d.language.SymbolNames[tok.Symbol]
			}
			prefix := "DFA"
			if tokenFromExternal {
				prefix = "EXT"
			}
			fmt.Printf("  %s tok %d %s %d %d %s state=%d\n", prefix, tok.Symbol, name, tok.StartByte, tok.EndByte, tok.Text, d.state)
		}
		return tok
	}
}

func (d *dfaTokenSource) SetParserState(state StateID) {
	d.state = state
}

func (d *dfaTokenSource) SetGLRStates(states []StateID) {
	d.glrStates = states
}

func (d *dfaTokenSource) nextDFAToken() Token {
	if d == nil || d.lexer == nil || d.language == nil {
		return Token{}
	}
	startPos := d.lexer.pos
	startRow := d.lexer.row
	startCol := d.lexer.col
	lexState := d.lexStateForState(d.state)
	tok, endPos, endRow, endCol := d.scanDFAToken(lexState, startPos, startRow, startCol)
	if altTok, altEndPos, altEndRow, altEndCol, ok := d.tryFallbackDFAToken(lexState, tok, startPos, startRow, startCol); ok {
		tok = altTok
		endPos = altEndPos
		endRow = altEndRow
		endCol = altEndCol
	}
	d.lexer.pos = endPos
	d.lexer.row = endRow
	d.lexer.col = endCol
	return tok
}

func (d *dfaTokenSource) shouldForceEOFLookahead() bool {
	if d == nil || d.language == nil {
		return false
	}
	if int(d.state) >= len(d.language.LexModes) {
		return false
	}
	return d.language.LexModes[d.state].LexState == noLookaheadLexState
}

func (d *dfaTokenSource) syntheticEOFLookaheadToken() Token {
	if d == nil || d.lexer == nil {
		return Token{NoLookahead: true}
	}
	pt := Point{Row: d.lexer.row, Column: d.lexer.col}
	return Token{
		StartByte:   uint32(d.lexer.pos),
		EndByte:     uint32(d.lexer.pos),
		StartPoint:  pt,
		EndPoint:    pt,
		NoLookahead: true,
	}
}

// nextGLRUnionDFAToken tries each unique GLR stack state's lex mode and
// picks the DFA token that has valid parse actions in the most stacks.
// This prevents the primary stack's lex mode from producing a token that's
// wrong for other stacks, which would cause them to be killed prematurely.
func (d *dfaTokenSource) nextGLRUnionDFAToken() (Token, bool) {
	if d == nil || d.lexer == nil || d.language == nil || d.lookupActionIndex == nil {
		return Token{}, false
	}
	if len(d.glrStates) <= 1 {
		return Token{}, false
	}

	// Check if all GLR states share the same lex mode — if so, no union needed.
	primaryLexState := d.lexStateForState(d.state)
	allSame := true
	for _, st := range d.glrStates {
		ls := d.lexStateForState(st)
		if ls != primaryLexState {
			allSame = false
			break
		}
	}
	if allSame {
		return Token{}, false
	}

	startPos := d.lexer.pos
	startRow := d.lexer.row
	startCol := d.lexer.col

	bestScore := 0
	bestFound := false
	bestTok := Token{}
	bestEndPos := startPos
	bestEndRow := startRow
	bestEndCol := startCol
	bestVisible := false

	// Deduplicate lex states to avoid redundant scans.
	seen := make(map[uint16]struct{}, len(d.glrStates))
	for _, st := range d.glrStates {
		lexState := d.lexStateForState(st)
		if _, ok := seen[lexState]; ok {
			continue
		}
		seen[lexState] = struct{}{}

		d.lexer.pos = startPos
		d.lexer.row = startRow
		d.lexer.col = startCol

		prevState := d.state
		d.state = st
		candTok := d.lexer.Next(lexState)
		candTok = d.promoteKeyword(candTok)
		candTok, candEndPos, candEndRow, candEndCol := d.normalizeDFAToken(candTok, d.lexer.pos, d.lexer.row, d.lexer.col)
		d.lexer.pos = candEndPos
		d.lexer.row = candEndRow
		d.lexer.col = candEndCol
		d.state = prevState

		score := 0
		for _, liveState := range d.glrStates {
			if d.lookupActionIndex(liveState, candTok.Symbol) != 0 {
				score++
			}
		}

		if score <= 0 {
			continue
		}

		candVisible := int(candTok.Symbol) < len(d.language.SymbolMetadata) && d.language.SymbolMetadata[candTok.Symbol].Visible
		better := !bestFound ||
			candTok.StartByte < bestTok.StartByte ||
			(candTok.StartByte == bestTok.StartByte && score > bestScore) ||
			(candTok.StartByte == bestTok.StartByte && score == bestScore && candTok.EndByte > bestTok.EndByte) ||
			(candTok.StartByte == bestTok.StartByte && score == bestScore && candTok.EndByte == bestTok.EndByte && candEndPos > bestEndPos) ||
			(candTok.StartByte == bestTok.StartByte && score == bestScore && candTok.EndByte == bestTok.EndByte && candEndPos == bestEndPos && candVisible && !bestVisible)
		if better {
			bestFound = true
			bestScore = score
			bestTok = candTok
			bestEndPos = candEndPos
			bestEndRow = candEndRow
			bestEndCol = candEndCol
			bestVisible = candVisible
		}
	}

	if !bestFound {
		d.lexer.pos = startPos
		d.lexer.row = startRow
		d.lexer.col = startCol
		return Token{}, false
	}

	d.lexer.pos = bestEndPos
	d.lexer.row = bestEndRow
	d.lexer.col = bestEndCol
	return bestTok, true
}

func (d *dfaTokenSource) lexStateForState(st StateID) uint16 {
	if d == nil || d.language == nil {
		return 0
	}
	if int(st) < len(d.language.LexModes) && d.language.LexModes[st].LexState == noLookaheadLexState {
		return noLookaheadLexState
	}
	if d.useLayoutFallbackLexState(st) {
		return d.language.LayoutFallbackLexState
	}
	if int(st) < len(d.language.LexModes) {
		return d.language.LexModes[st].LexState
	}
	return 0
}

func (d *dfaTokenSource) scanDFAToken(lexState uint16, startPos int, startRow, startCol uint32) (Token, int, uint32, uint32) {
	if d == nil || d.lexer == nil {
		return Token{}, startPos, startRow, startCol
	}
	d.lexer.pos = startPos
	d.lexer.row = startRow
	d.lexer.col = startCol
	tok := d.lexer.Next(lexState)
	tok = d.promoteKeyword(tok)
	return d.normalizeDFAToken(tok, d.lexer.pos, d.lexer.row, d.lexer.col)
}

func (d *dfaTokenSource) tryFallbackDFAToken(primaryLexState uint16, primaryTok Token, startPos int, startRow, startCol uint32) (Token, int, uint32, uint32, bool) {
	if d == nil || d.language == nil || d.lookupActionIndex == nil {
		return Token{}, 0, 0, 0, false
	}
	if !d.shouldCompareFallbackLexState(d.state) {
		return Token{}, 0, 0, 0, false
	}
	fallbackLexState := d.language.LayoutFallbackLexState
	if fallbackLexState == primaryLexState || fallbackLexState == noLookaheadLexState {
		return Token{}, 0, 0, 0, false
	}
	fallbackTok, fallbackEndPos, fallbackEndRow, fallbackEndCol := d.scanDFAToken(fallbackLexState, startPos, startRow, startCol)
	if d.lookupActionIndex(d.state, fallbackTok.Symbol) == 0 {
		return Token{}, 0, 0, 0, false
	}
	if primaryTok.Symbol == 0 ||
		d.lookupActionIndex(d.state, primaryTok.Symbol) == 0 ||
		fallbackTok.StartByte < primaryTok.StartByte ||
		(fallbackTok.StartByte == primaryTok.StartByte && fallbackTok.EndByte > primaryTok.EndByte) {
		return fallbackTok, fallbackEndPos, fallbackEndRow, fallbackEndCol, true
	}
	return Token{}, 0, 0, 0, false
}

func (d *dfaTokenSource) shouldCompareFallbackLexState(st StateID) bool {
	if d == nil || d.language == nil || !d.language.HasLayoutFallbackLexState {
		return false
	}
	if int(st) < 0 || int(st) >= len(d.language.LexModes) {
		return false
	}
	return d.language.LexModes[st].ExternalLexState > 0
}

func (d *dfaTokenSource) useLayoutFallbackLexState(st StateID) bool {
	if d == nil || d.language == nil || !d.language.HasLayoutFallbackLexState {
		return false
	}
	if len(d.language.ExternalLexStates) == 0 || int(st) >= len(d.language.LexModes) {
		return false
	}
	elsID := int(d.language.LexModes[st].ExternalLexState)
	if elsID <= 0 || elsID >= len(d.language.ExternalLexStates) {
		return false
	}
	row := d.language.ExternalLexStates[elsID]
	for i, ok := range row {
		if !ok || i >= len(d.language.ExternalSymbols) {
			continue
		}
		sym := d.language.ExternalSymbols[i]
		if d.isLayoutEntryExternalSymbol(sym) {
			return true
		}
	}
	return false
}

func (d *dfaTokenSource) isLayoutEntryExternalSymbol(sym Symbol) bool {
	if d == nil || d.language == nil || int(sym) >= len(d.language.SymbolNames) {
		return false
	}
	name := d.language.SymbolNames[sym]
	return name == "{" || strings.HasPrefix(name, "_cmd_layout_start")
}

func (d *dfaTokenSource) normalizeDFAToken(tok Token, endPos int, endRow, endCol uint32) (Token, int, uint32, uint32) {
	if d == nil || d.language == nil || d.lexer == nil {
		return tok, endPos, endRow, endCol
	}
	if d.language.Name != "bash" || tok.Symbol != 86 || tok.EndByte <= tok.StartByte+1 {
		return tok, endPos, endRow, endCol
	}
	start := int(tok.StartByte)
	if start < 0 || start >= len(d.lexer.source) || d.lexer.source[start] != '\n' {
		return tok, endPos, endRow, endCol
	}
	limit := int(tok.EndByte)
	if limit > len(d.lexer.source) {
		limit = len(d.lexer.source)
	}
	for i := start + 1; i < limit; i++ {
		if d.lexer.source[i] != '\n' {
			return tok, endPos, endRow, endCol
		}
	}
	tok.EndByte = tok.StartByte + 1
	tok.EndPoint = Point{Row: tok.StartPoint.Row + 1, Column: 0}
	if len(tok.Text) > 1 {
		tok.Text = tok.Text[:1]
	}
	return tok, start + 1, tok.StartPoint.Row + 1, 0
}

func (d *dfaTokenSource) hasAnyActionForSymbol(sym Symbol) bool {
	if d == nil || d.lookupActionIndex == nil || sym == 0 {
		return false
	}
	if len(d.glrStates) == 0 {
		return d.lookupActionIndex(d.state, sym) != 0
	}
	for _, st := range d.glrStates {
		if d.lookupActionIndex(st, sym) != 0 {
			return true
		}
	}
	return false
}

func (d *dfaTokenSource) eofTokenAtLexerPos() Token {
	if d == nil || d.lexer == nil {
		return Token{}
	}
	pt := Point{Row: d.lexer.row, Column: d.lexer.col}
	return Token{
		StartByte:  uint32(d.lexer.pos),
		EndByte:    uint32(d.lexer.pos),
		StartPoint: pt,
		EndPoint:   pt,
	}
}

func (d *dfaTokenSource) SkipToByte(offset uint32) Token {
	target := int(offset)
	if target < d.lexer.pos {
		// Rewind isn't supported for DFA token sources during parse.
		return d.Next()
	}
	startPos := 0
	if perfCountersEnabled {
		startPos = d.lexer.pos
	}
	for d.lexer.pos < target {
		d.lexer.skipOneRune()
	}
	if perfCountersEnabled {
		consumed := d.lexer.pos - startPos
		if consumed < 0 {
			consumed = 0
		}
		perfRecordLexed(consumed, 0)
	}
	return d.Next()
}

func (d *dfaTokenSource) SkipToByteWithPoint(offset uint32, pt Point) Token {
	target := int(offset)
	if target > len(d.lexer.source) {
		target = len(d.lexer.source)
	}
	if target >= d.lexer.pos {
		d.lexer.pos = target
		d.lexer.row = pt.Row
		d.lexer.col = pt.Column
	}
	return d.Next()
}

func (d *dfaTokenSource) nextExternalToken() (Token, bool) {
	if d.language == nil || d.lookupActionIndex == nil {
		return Token{}, false
	}
	if len(d.language.ExternalSymbols) == 0 {
		return Token{}, false
	}

	if cap(d.externalValid) < len(d.language.ExternalSymbols) {
		d.externalValid = make([]bool, len(d.language.ExternalSymbols))
	}
	valid := d.externalValid[:len(d.language.ExternalSymbols)]
	for i := range valid {
		valid[i] = false
	}

	// Compute valid external symbols as the union across all active GLR
	// stacks. Different stacks may be in different parser states with
	// different valid external tokens. The scanner needs to see the union
	// so it can produce tokens that any stack might need. Stacks that
	// can't use the resulting token will be pruned by the action phase.
	anyValid := false
	states := d.glrStates
	if len(states) == 0 {
		states = []StateID{d.state}
	}

	if len(d.language.ExternalLexStates) > 0 {
		// Use the precise external lex states table (matches C tree-sitter's
		// ts_external_scanner_states). Each parser state maps to an external
		// lex state ID via LexModes, and each external lex state ID maps to
		// a boolean row indicating which external tokens are valid.
		for _, st := range states {
			if int(st) >= len(d.language.LexModes) {
				continue
			}
			elsID := int(d.language.LexModes[st].ExternalLexState)
			if elsID >= len(d.language.ExternalLexStates) {
				continue
			}
			row := d.language.ExternalLexStates[elsID]
			for i := range valid {
				if i < len(row) && row[i] && !valid[i] {
					valid[i] = true
					anyValid = true
				}
			}
		}
	} else {
		// Fallback: probe the parse action table for each external symbol.
		// This is less precise than ExternalLexStates (may include error
		// recovery actions) but works for grammars without the table.
		for _, st := range states {
			for i, sym := range d.language.ExternalSymbols {
				if !valid[i] && d.lookupActionIndex(st, sym) != 0 {
					valid[i] = true
					anyValid = true
				}
			}
		}
	}
	if !anyValid {
		return Token{}, false
	}

	// Zero-width external token loop prevention: exclude external token
	// indices that were already produced as zero-width tokens at this same
	// (position, state) pair. When the parser has no action for a zero-width
	// external token, it error-wraps it without changing state; the same
	// scanner call would then produce the identical token infinitely.
	// C tree-sitter avoids this via its ERROR_STATE lex mode which causes
	// the scanner to bail out via the __error_recovery sentinel. The Go
	// runtime instead tracks tried indices per (position, state).
	if d.language != nil && d.language.Name != "yaml" &&
		d.lexer.pos == d.extZeroPos && d.state == d.extZeroState && len(d.extZeroTried) > 0 {
		for i := range valid {
			if i < len(d.extZeroTried) && d.extZeroTried[i] &&
				!d.allowRepeatedZeroWidthExternalSymbol(d.language.ExternalSymbols[i]) {
				valid[i] = false
			}
		}
		// Recheck if anything is still valid.
		anyValid = false
		for _, v := range valid {
			if v {
				anyValid = true
				break
			}
		}
		if !anyValid {
			return Token{}, false
		}
	}

	if d.language.ExternalScanner == nil {
		tok, ok := d.syntheticExternalToken(valid)
		if !ok {
			return Token{}, false
		}
		d.trackZeroWidthExternalToken(tok)
		return tok, true
	}

	el := newExternalLexer(d.lexer.source, d.lexer.pos, d.lexer.row, d.lexer.col)
	if !RunExternalScanner(d.language, d.externalPayload, el, valid) {
		return Token{}, false
	}
	tok, ok := el.token()
	if !ok {
		return Token{}, false
	}

	d.trackZeroWidthExternalToken(tok)

	d.lexer.pos = int(tok.EndByte)
	d.lexer.row = tok.EndPoint.Row
	d.lexer.col = tok.EndPoint.Column
	return tok, true
}

func (d *dfaTokenSource) trackZeroWidthExternalToken(tok Token) {
	if d == nil || d.language == nil {
		return
	}
	// Track zero-width tokens for loop prevention.
	if tok.EndByte <= tok.StartByte {
		if d.allowRepeatedZeroWidthExternalSymbol(tok.Symbol) {
			d.extZeroPos = -1
			if len(d.extZeroTried) > 0 {
				d.extZeroTried = d.extZeroTried[:0]
			}
			return
		}
		if d.lexer.pos != d.extZeroPos || d.state != d.extZeroState {
			// New position or state — reset the tried set.
			d.extZeroPos = d.lexer.pos
			d.extZeroState = d.state
			if cap(d.extZeroTried) < len(d.language.ExternalSymbols) {
				d.extZeroTried = make([]bool, len(d.language.ExternalSymbols))
			} else {
				d.extZeroTried = d.extZeroTried[:len(d.language.ExternalSymbols)]
				for i := range d.extZeroTried {
					d.extZeroTried[i] = false
				}
			}
		}
		// Mark the token index that produced this symbol.
		for i, sym := range d.language.ExternalSymbols {
			if sym == tok.Symbol {
				if i < len(d.extZeroTried) {
					d.extZeroTried[i] = true
				}
				break
			}
		}
		return
	}
	// Non-zero-width token: clear the zero-width loop state.
	d.extZeroPos = -1
}

func (d *dfaTokenSource) allowRepeatedZeroWidthExternalSymbol(sym Symbol) bool {
	if d == nil || d.language == nil {
		return false
	}
	nameIdx := int(sym)
	if nameIdx < 0 || nameIdx >= len(d.language.SymbolNames) {
		return false
	}
	switch d.language.SymbolNames[nameIdx] {
	case "_implicit_end_tag":
		return true
	default:
		return false
	}
}

const (
	extNameAutomaticSemicolon                  = "_automatic_semicolon"
	extNameFunctionSignatureAutomaticSemicolon = "_function_signature_automatic_semicolon"
	extNameImplicitSemicolon                   = "_implicit_semicolon"
	extNameLineBreak                           = "_line_break"
	extNameNewline                             = "_newline"
	extNameLineEndingOrEOF                     = "_line_ending_or_eof"
	extNameJSXText                             = "jsx_text"
)

func (d *dfaTokenSource) syntheticExternalToken(valid []bool) (Token, bool) {
	// Conservative fallback when no external scanner is registered:
	// synthesize automatic-semicolon style external tokens only when the
	// grammar explicitly allows them in the current state.
	if d.language == nil || d.lexer == nil {
		return Token{}, false
	}

	for i, sym := range d.language.ExternalSymbols {
		if i >= len(valid) || !valid[i] {
			continue
		}
		nameIdx := int(sym)
		if nameIdx < 0 || nameIdx >= len(d.language.SymbolNames) {
			continue
		}
		switch d.language.SymbolNames[nameIdx] {
		case extNameAutomaticSemicolon, extNameFunctionSignatureAutomaticSemicolon, extNameImplicitSemicolon:
			return d.syntheticAutomaticSemicolon(sym)
		case extNameLineBreak, extNameNewline:
			return d.syntheticLineBreak(sym)
		case extNameLineEndingOrEOF:
			return d.syntheticLineEndingOrEOF(sym)
		case extNameJSXText:
			return d.syntheticJSXText(sym)
		}
	}

	return Token{}, false
}

func (d *dfaTokenSource) syntheticAutomaticSemicolon(sym Symbol) (Token, bool) {
	if d.lexer == nil {
		return Token{}, false
	}
	source := d.lexer.source
	startPos := d.lexer.pos
	startPoint := Point{Row: d.lexer.row, Column: d.lexer.col}

	// EOF insertion is always allowed when the grammar requests it.
	if startPos >= len(source) {
		return Token{
			Symbol:     sym,
			StartByte:  uint32(startPos),
			EndByte:    uint32(startPos),
			StartPoint: startPoint,
			EndPoint:   startPoint,
		}, true
	}

	pos := startPos
	endRow := d.lexer.row
	endCol := d.lexer.col
	sawLineBreak := false

	// Consume horizontal space, then allow insertion on line break or EOF.
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
			endCol++
		case '\r':
			pos++
			if pos < len(source) && source[pos] == '\n' {
				pos++
			}
			endRow++
			endCol = 0
			sawLineBreak = true
			goto done
		case '\n':
			pos++
			endRow++
			endCol = 0
			sawLineBreak = true
			goto done
		default:
			return Token{}, false
		}
	}

	// Reached EOF after horizontal space.
	return Token{
		Symbol:     sym,
		StartByte:  uint32(startPos),
		EndByte:    uint32(pos),
		StartPoint: startPoint,
		EndPoint:   Point{Row: endRow, Column: endCol},
	}, true

done:
	if !sawLineBreak {
		return Token{}, false
	}

	// Consume indentation after newline so lexing resumes at next token.
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
			endCol++
		default:
			return Token{
				Symbol:     sym,
				StartByte:  uint32(startPos),
				EndByte:    uint32(pos),
				StartPoint: startPoint,
				EndPoint:   Point{Row: endRow, Column: endCol},
			}, true
		}
	}

	return Token{
		Symbol:     sym,
		StartByte:  uint32(startPos),
		EndByte:    uint32(pos),
		StartPoint: startPoint,
		EndPoint:   Point{Row: endRow, Column: endCol},
	}, true
}

func (d *dfaTokenSource) syntheticLineBreak(sym Symbol) (Token, bool) {
	if d.lexer == nil {
		return Token{}, false
	}
	source := d.lexer.source
	startPos := d.lexer.pos
	startPoint := Point{Row: d.lexer.row, Column: d.lexer.col}

	pos := startPos
	endRow := d.lexer.row
	endCol := d.lexer.col

	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
			endCol++
		case '\r':
			pos++
			if pos < len(source) && source[pos] == '\n' {
				pos++
			}
			endRow++
			endCol = 0
			goto consumeIndent
		case '\n':
			pos++
			endRow++
			endCol = 0
			goto consumeIndent
		default:
			return Token{}, false
		}
	}

	return Token{}, false

consumeIndent:
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
			endCol++
		default:
			return Token{
				Symbol:     sym,
				StartByte:  uint32(startPos),
				EndByte:    uint32(pos),
				StartPoint: startPoint,
				EndPoint:   Point{Row: endRow, Column: endCol},
			}, true
		}
	}

	return Token{
		Symbol:     sym,
		StartByte:  uint32(startPos),
		EndByte:    uint32(pos),
		StartPoint: startPoint,
		EndPoint:   Point{Row: endRow, Column: endCol},
	}, true
}

func (d *dfaTokenSource) syntheticLineEndingOrEOF(sym Symbol) (Token, bool) {
	if d.lexer == nil {
		return Token{}, false
	}
	if tok, ok := d.syntheticLineBreak(sym); ok {
		return tok, true
	}

	source := d.lexer.source
	startPos := d.lexer.pos
	startPoint := Point{Row: d.lexer.row, Column: d.lexer.col}
	if startPos >= len(source) {
		return Token{
			Symbol:     sym,
			StartByte:  uint32(startPos),
			EndByte:    uint32(startPos),
			StartPoint: startPoint,
			EndPoint:   startPoint,
		}, true
	}

	pos := startPos
	endCol := d.lexer.col
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
			endCol++
		default:
			return Token{}, false
		}
	}

	return Token{
		Symbol:     sym,
		StartByte:  uint32(startPos),
		EndByte:    uint32(pos),
		StartPoint: startPoint,
		EndPoint:   Point{Row: d.lexer.row, Column: endCol},
	}, true
}

func (d *dfaTokenSource) syntheticJSXText(sym Symbol) (Token, bool) {
	if d.lexer == nil {
		return Token{}, false
	}
	source := d.lexer.source
	startPos := d.lexer.pos
	if startPos >= len(source) {
		return Token{}, false
	}

	switch source[startPos] {
	case '<', '{', '}':
		return Token{}, false
	}

	pos := startPos
	endRow := d.lexer.row
	endCol := d.lexer.col

	for pos < len(source) {
		switch source[pos] {
		case '<', '{', '}':
			if pos == startPos {
				return Token{}, false
			}
			startPoint := Point{Row: d.lexer.row, Column: d.lexer.col}
			return Token{
				Symbol:     sym,
				StartByte:  uint32(startPos),
				EndByte:    uint32(pos),
				StartPoint: startPoint,
				EndPoint:   Point{Row: endRow, Column: endCol},
			}, true
		case '\r':
			pos++
			if pos < len(source) && source[pos] == '\n' {
				pos++
			}
			endRow++
			endCol = 0
		case '\n':
			pos++
			endRow++
			endCol = 0
		default:
			_, size := utf8.DecodeRune(source[pos:])
			if size <= 0 {
				size = 1
			}
			pos += size
			endCol++
		}
	}

	if pos == startPos {
		return Token{}, false
	}
	startPoint := Point{Row: d.lexer.row, Column: d.lexer.col}
	return Token{
		Symbol:     sym,
		StartByte:  uint32(startPos),
		EndByte:    uint32(pos),
		StartPoint: startPoint,
		EndPoint:   Point{Row: endRow, Column: endCol},
	}, true
}

func (d *dfaTokenSource) promoteKeyword(tok Token) Token {
	if d.language == nil {
		return tok
	}
	if tok.Symbol == 0 {
		return tok
	}
	if len(d.language.KeywordLexStates) == 0 {
		return tok
	}
	if d.language.KeywordCaptureToken == 0 {
		return tok
	}
	if tok.Symbol != d.language.KeywordCaptureToken {
		return tok
	}
	if tok.EndByte <= tok.StartByte {
		return tok
	}
	if len(d.hasKeywordState) > 0 {
		anyHasKeyword := false
		state := int(d.state)
		if state >= 0 && state < len(d.hasKeywordState) && d.hasKeywordState[state] {
			anyHasKeyword = true
		}
		if !anyHasKeyword {
			for _, st := range d.glrStates {
				si := int(st)
				if si >= 0 && si < len(d.hasKeywordState) && d.hasKeywordState[si] {
					anyHasKeyword = true
					break
				}
			}
		}
		if !anyHasKeyword {
			return tok
		}
	}

	start := int(tok.StartByte)
	end := int(tok.EndByte)
	if start < 0 || end < start || end > len(d.lexer.source) {
		return tok
	}

	kw := Lexer{
		states: d.language.KeywordLexStates,
		source: d.lexer.source[start:end],
	}
	kwTok := kw.Next(0)
	if kwTok.Symbol == 0 {
		return tok
	}
	if kwTok.StartByte != 0 {
		return tok
	}
	if kwTok.EndByte != uint32(end-start) {
		return tok
	}

	// ABI 15: Check if keyword is reserved in this parse state.
	if len(d.language.ReservedWords) > 0 && d.language.MaxReservedWordSetSize > 0 {
		if int(d.state) < len(d.language.LexModes) {
			rwSetID := d.language.LexModes[d.state].ReservedWordSetID
			if rwSetID > 0 {
				stride := int(d.language.MaxReservedWordSetSize)
				start := int(rwSetID) * stride
				end := start + stride
				if end > len(d.language.ReservedWords) {
					end = len(d.language.ReservedWords)
				}
				for i := start; i < end; i++ {
					if d.language.ReservedWords[i] == 0 {
						break
					}
					if d.language.ReservedWords[i] == kwTok.Symbol {
						return tok // reserved — don't promote
					}
				}
			}
		}
	}

	// Context-aware promotion: only use the keyword symbol if any active
	// parser state has a valid action for it. This prevents contextual
	// keywords like "get"/"set" from being promoted in positions where
	// they should be treated as identifiers (e.g., obj.get(...)).
	// When multiple GLR stacks exist, check ALL stack states — different
	// forks may need different tokenizations, and demoting a keyword based
	// only on the primary stack's state can kill the correct fork.
	if d.lookupActionIndex != nil {
		kwHasAction := d.lookupActionIndex(d.state, kwTok.Symbol) != 0
		if !kwHasAction && len(d.glrStates) > 0 {
			for _, st := range d.glrStates {
				if d.lookupActionIndex(st, kwTok.Symbol) != 0 {
					kwHasAction = true
					break
				}
			}
		}
		idHasAction := d.lookupActionIndex(d.state, tok.Symbol) != 0
		if !idHasAction && len(d.glrStates) > 0 {
			for _, st := range d.glrStates {
				if d.lookupActionIndex(st, tok.Symbol) != 0 {
					idHasAction = true
					break
				}
			}
		}
		if !kwHasAction && idHasAction {
			return tok // no active stack needs the keyword
		}
	}

	tok.Symbol = kwTok.Symbol
	return tok
}

// parseIterations returns the iteration limit scaled to input size.
// A correctly-parsed file needs roughly (tokens * grammar_depth) iterations.
// For typical source (~5 bytes/token, ~10 reduce depth), that's sourceLen*2.
// We use sourceLen*20 as a generous upper bound that still prevents runaway
// parsing from OOMing the machine.
