package gotreesitter

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

type dfaTokenSource struct {
	lexer    *Lexer
	language *Language
	state    StateID

	lookupActionIndex          func(state StateID, sym Symbol) uint16
	lexModeStarts              []lexModeStart
	hasKeywordState            []bool
	externalValidByState       [][]uint16
	externalPayload            any
	externalValid              []bool
	externalSnapshot           []byte
	externalRetrySnap          []byte
	externalTokenStart         []byte
	externalTokenEnd           []byte
	externalCompare            []byte
	externalLexer              ExternalLexer
	externalRetryLexer         ExternalLexer
	lastExternalTokenStartByte uint32
	lastExternalTokenEndByte   uint32
	lastExternalTokenValid     bool
	singleState                [1]StateID
	glrStates                  []StateID // all active GLR stack states
	hasExternalScanner         bool
	hasExternalSymbols         bool
	usesExternalCheckpoints    bool
	isBash                     bool
	isBashGenerated            bool
	isComment                  bool
	isFortran                  bool
	hasZeroWidthTokens         bool
	hasZeroWidthStartAccept    bool

	// maskedScratch is a reusable buffer for runExternalScannerWithRetry,
	// avoiding a per-call heap allocation when masking already-tried symbols.
	maskedScratch []bool

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

	// Cached Bash arithmetic-expansion context. Generated Bash token repair
	// asks this repeatedly while probing operator candidates at nearby byte
	// offsets, so retain the last prefix scan state instead of rescanning from
	// the start of the file each time.
	bashArithmeticCachePos       int
	bashArithmeticCacheDepth     int
	bashArithmeticCacheSkipUntil int
	bashArithmeticCacheResult    bool

	// noPool skips pool return on Close; set for token sources whose lifetime
	// is nested inside an active parse (e.g. recovery reparsing).
	noPool bool
}

const maxConsecutiveZeroWidthTokens = 4
const maxConsecutiveZeroWidthTokensExternal = 128
const maxConsecutiveZeroWidthTokensRepeatableExternal = 4096
const noLookaheadLexState = ^uint32(0)
const externalScannerSerializationBufferSize = 4096

var dfaTokenSourcePool = sync.Pool{
	New: func() any {
		return &dfaTokenSource{
			extZeroPos:             -1,
			zeroWidthPos:           -1,
			bashArithmeticCachePos: -1,
		}
	},
}

func initDFATokenSource(ts *dfaTokenSource, lexer *Lexer, language *Language, lookupActionIndex func(state StateID, sym Symbol) uint16, hasKeywordState []bool, externalValidByState [][]uint16) {
	ts.lexer = lexer
	ts.language = language
	ts.state = 0
	ts.lookupActionIndex = lookupActionIndex
	ts.lexModeStarts = nil
	ts.hasKeywordState = hasKeywordState
	ts.externalValidByState = externalValidByState
	if lexer != nil && language != nil {
		ts.lexer.states = language.LexStates
		ts.lexer.immediateTokens = language.ImmediateTokens
		ts.lexer.zeroWidthTokens = language.ZeroWidthTokens
		ts.lexer.asciiTable = language.LexAsciiTable()
		ts.lexModeStarts = language.LexModeStarts()
	}
	if language != nil {
		ts.hasExternalScanner = language.ExternalScanner != nil
		ts.hasExternalSymbols = len(language.ExternalSymbols) > 0
		ts.usesExternalCheckpoints = languageUsesExternalScannerCheckpoints(language)
		ts.isBash = language.Name == "bash"
		ts.isBashGenerated = ts.isBash && language.GeneratedByGrammargen
		ts.isComment = language.Name == "comment"
		ts.isFortran = language.Name == "fortran"
		ts.hasZeroWidthTokens = languageHasZeroWidthTokens(language)
		ts.hasZeroWidthStartAccept = languageHasZeroWidthStartAccept(language)
	}
	if ts.hasExternalScanner {
		ts.externalPayload = language.ExternalScanner.Create()
	}
}

func acquireDFATokenSource(lexer *Lexer, language *Language, lookupActionIndex func(state StateID, sym Symbol) uint16, hasKeywordState []bool, externalValidByState [][]uint16) *dfaTokenSource {
	ts := dfaTokenSourcePool.Get().(*dfaTokenSource)
	resetPooledDFATokenSource(ts)
	initDFATokenSource(ts, lexer, language, lookupActionIndex, hasKeywordState, externalValidByState)
	return ts
}

func resetPooledDFATokenSource(ts *dfaTokenSource) {
	if ts == nil {
		return
	}
	// Preserve pooled scratch slices across the struct reset below so they can
	// be reused without reallocation on the next parse.
	savedExternalValid := ts.externalValid[:0]
	savedExternalTokenStart := ts.externalTokenStart[:0]
	savedExternalTokenEnd := ts.externalTokenEnd[:0]
	savedExternalSnapshot := ts.externalSnapshot[:0]
	savedExternalRetrySnap := ts.externalRetrySnap[:0]
	savedExternalCompare := ts.externalCompare[:0]
	savedMasked := ts.maskedScratch[:0]
	savedExtZeroTried := ts.extZeroTried[:0]
	*ts = dfaTokenSource{
		extZeroPos:             -1,
		zeroWidthPos:           -1,
		bashArithmeticCachePos: -1,
	}
	ts.externalValid = savedExternalValid
	ts.externalTokenStart = savedExternalTokenStart
	ts.externalTokenEnd = savedExternalTokenEnd
	ts.externalSnapshot = savedExternalSnapshot
	ts.externalRetrySnap = savedExternalRetrySnap
	ts.externalCompare = savedExternalCompare
	ts.maskedScratch = savedMasked
	ts.extZeroTried = savedExtZeroTried
}

func newDFATokenSourceDirect(lexer *Lexer, language *Language, lookupActionIndex func(state StateID, sym Symbol) uint16, hasKeywordState []bool, externalValidByState [][]uint16) *dfaTokenSource {
	ts := &dfaTokenSource{
		extZeroPos:             -1,
		zeroWidthPos:           -1,
		bashArithmeticCachePos: -1,
		noPool:                 true,
	}
	initDFATokenSource(ts, lexer, language, lookupActionIndex, hasKeywordState, externalValidByState)
	return ts
}

func languageHasZeroWidthTokens(lang *Language) bool {
	if lang == nil {
		return false
	}
	for _, ok := range lang.ZeroWidthTokens {
		if ok {
			return true
		}
	}
	return false
}

func languageHasZeroWidthStartAccept(lang *Language) bool {
	if lang == nil || len(lang.ZeroWidthTokens) == 0 {
		return false
	}
	for _, state := range lang.LexStates {
		sym := int(state.AcceptToken)
		if sym >= 0 && sym < len(lang.ZeroWidthTokens) && lang.ZeroWidthTokens[sym] {
			return true
		}
	}
	return false
}

func (d *dfaTokenSource) Reset(source []byte) {
	if d == nil {
		return
	}
	if d.lexer == nil {
		d.lexer = NewLexer(nil, source)
	}
	d.lexer.source = source
	d.lexer.pos = 0
	d.lexer.row = 0
	d.lexer.col = 0
	if d.language != nil {
		d.lexer.states = d.language.LexStates
		d.lexer.immediateTokens = d.language.ImmediateTokens
		d.lexer.zeroWidthTokens = d.language.ZeroWidthTokens
		d.lexer.asciiTable = d.language.LexAsciiTable()
	}
	d.state = 0
	d.glrStates = nil
	if len(d.externalValid) > 0 {
		d.externalValid = d.externalValid[:0]
	}
	if len(d.extZeroTried) > 0 {
		d.extZeroTried = d.extZeroTried[:0]
	}
	d.extZeroPos = -1
	d.extZeroState = 0
	d.zeroWidthPos = -1
	d.zeroWidthCount = 0
	d.bashArithmeticCachePos = -1
	d.bashArithmeticCacheDepth = 0
	d.bashArithmeticCacheSkipUntil = 0
	d.bashArithmeticCacheResult = false
	d.lastExternalTokenStartByte = 0
	d.lastExternalTokenEndByte = 0
	d.lastExternalTokenValid = false
	if d.language == nil || d.language.ExternalScanner == nil {
		return
	}
	if d.externalPayload != nil {
		d.language.ExternalScanner.Destroy(d.externalPayload)
	}
	d.externalPayload = d.language.ExternalScanner.Create()
}

func (d *dfaTokenSource) Close() {
	if d.language != nil && d.language.ExternalScanner != nil && d.externalPayload != nil {
		d.language.ExternalScanner.Destroy(d.externalPayload)
		d.externalPayload = nil
	}
	d.lexer = nil
	d.language = nil
	d.lookupActionIndex = nil
	d.hasKeywordState = nil
	d.externalValidByState = nil
	d.glrStates = nil
	d.extZeroPos = -1
	d.extZeroState = 0
	d.zeroWidthPos = -1
	d.zeroWidthCount = 0
	d.bashArithmeticCachePos = -1
	d.bashArithmeticCacheDepth = 0
	d.bashArithmeticCacheSkipUntil = 0
	d.bashArithmeticCacheResult = false
	d.lastExternalTokenStartByte = 0
	d.lastExternalTokenEndByte = 0
	d.lastExternalTokenValid = false
	if !d.noPool {
		dfaTokenSourcePool.Put(d)
	}
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
		scanStartPos, scanStartRow, scanStartCol := 0, uint32(0), uint32(0)
		if d.hasExternalSymbols || d.hasExternalScanner {
			scanStartPos = d.lexer.pos
			scanStartRow = d.lexer.row
			scanStartCol = d.lexer.col
		}
		var externalStartSnapshot []byte
		if d.usesExternalCheckpoints {
			externalStartSnapshot = d.captureExternalScannerStateInto(&d.externalTokenStart)
		}
		if d.shouldForceEOFLookahead() {
			tok := d.syntheticEOFLookaheadToken()
			d.lastExternalTokenValid = false
			if DebugDFA.Load() {
				fmt.Printf("  SYN tok %d  %d %d state=%d\n", tok.Symbol, tok.StartByte, tok.EndByte, d.state)
			}
			return tok
		}

		tok := Token{}
		tokenFromExternal := false
		if d.hasExternalSymbols {
			if extTok, ok := d.nextExternalToken(); ok {
				tok = extTok
				tokenFromExternal = true
				if d.isBashGenerated {
					if dfaTok, ok := d.bashGeneratedTokenOverZeroWidthConcat(tok, scanStartPos, scanStartRow, scanStartCol); ok {
						tok = dfaTok
						tokenFromExternal = false
						d.lexer.pos = int(tok.EndByte)
						d.lexer.row = tok.EndPoint.Row
						d.lexer.col = tok.EndPoint.Column
					}
				}
			}
		}
		if tok.Symbol == 0 {
			if len(d.glrStates) > 1 {
				if glrTok, ok := d.nextGLRUnionDFAToken(); ok {
					tok = glrTok
				}
			}
			if tok.Symbol == 0 {
				tok = d.nextDFAToken()
			}
		}
		if !tokenFromExternal && d.hasExternalScanner &&
			tok.Symbol != 0 && int(tok.StartByte) > scanStartPos {
			if d.isBashGenerated {
				if nlTok, ok := d.bashSkippedSignificantNewlineToken(tok, scanStartPos, scanStartRow, scanStartCol); ok {
					tok = nlTok
					d.lexer.pos = int(tok.EndByte)
					d.lexer.row = tok.EndPoint.Row
					d.lexer.col = tok.EndPoint.Column
				}
			} else if d.isComment {
				// tree-sitter-comment's DFA text token can skip to a later tag.
				// Only that grammar should retry the external scanner at the
				// DFA token start; broader retries perturb structural scanners.
				dfaEndPos := d.lexer.pos
				dfaEndRow := d.lexer.row
				dfaEndCol := d.lexer.col

				d.lexer.pos = int(tok.StartByte)
				d.lexer.row = tok.StartPoint.Row
				d.lexer.col = tok.StartPoint.Column
				if extTok, ok := d.nextExternalToken(); ok && extTok.StartByte == tok.StartByte {
					tok = extTok
					tokenFromExternal = true
				} else {
					d.lexer.pos = dfaEndPos
					d.lexer.row = dfaEndRow
					d.lexer.col = dfaEndCol
				}
			}
		}
		if d.isFortran && d.shouldSuppressFortranPreprocDefineNewline(tok) {
			continue
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
				case d.language.Name == "yaml" || d.language.Name == "python":
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
		if d.usesExternalCheckpoints && tok.Symbol != 0 && !tok.NoLookahead {
			d.captureExternalScannerStateInto(&d.externalTokenEnd)
			d.lastExternalTokenStartByte = tok.StartByte
			d.lastExternalTokenEndByte = tok.EndByte
			d.lastExternalTokenValid = true
			// Keep start/end snapshots in the token source until the parser
			// either records them on a shifted leaf or advances to the next token.
			if len(externalStartSnapshot) == 0 {
				d.externalTokenStart = d.externalTokenStart[:0]
			}
			if len(d.externalTokenEnd) == 0 {
				d.externalTokenEnd = d.externalTokenEnd[:0]
			}
		} else {
			d.lastExternalTokenValid = false
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
	tok, endPos, endRow, endCol := d.scanPreferredTokenForState(d.state)
	d.lexer.pos = endPos
	d.lexer.row = endRow
	d.lexer.col = endCol
	return tok
}

func (d *dfaTokenSource) shouldForceEOFLookahead() bool {
	if d == nil || d.language == nil {
		return false
	}
	return d.lexStateForState(d.state) == noLookaheadLexState
}

func (d *dfaTokenSource) syntheticEOFLookaheadToken() Token {
	return d.nextTokenForLexState(noLookaheadLexState)
}

func (d *dfaTokenSource) nextTokenForLexState(lexState uint32) Token {
	if d == nil || d.lexer == nil {
		return Token{}
	}
	if lexState == noLookaheadLexState {
		tok := d.eofTokenAtLexerPos()
		tok.NoLookahead = true
		return tok
	}
	return d.lexer.Next(lexState)
}

func (d *dfaTokenSource) lexModeStartRows() []lexModeStart {
	if d == nil {
		return nil
	}
	if len(d.lexModeStarts) == 0 && d.language != nil {
		d.lexModeStarts = d.language.LexModeStarts()
	}
	return d.lexModeStarts
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

	// Check if all GLR states share the same lex mode pair — if so, no union needed.
	lexModes := d.lexModeStartRows()
	if int(d.state) >= len(lexModes) {
		return Token{}, false
	}
	primaryMode := lexModes[d.state]
	allSame := true
	for _, st := range d.glrStates {
		if int(st) >= len(lexModes) {
			allSame = false
			break
		}
		mode := lexModes[st]
		if mode != primaryMode {
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
	bestOriginActions := 0

	type lexModeKey struct {
		lexState                uint32
		afterWhitespaceLexState uint32
	}

	// Deduplicate equivalent lex mode pairs to avoid redundant scans.
	var seenBuf [32]lexModeKey
	seen := seenBuf[:0]
	for _, st := range d.glrStates {
		if int(st) >= len(lexModes) {
			continue
		}
		mode := lexModes[st]
		key := lexModeKey{
			lexState:                mode.lexState,
			afterWhitespaceLexState: mode.afterWhitespaceLexState,
		}
		alreadySeen := false
		for _, existing := range seen {
			if existing == key {
				alreadySeen = true
				break
			}
		}
		if alreadySeen {
			continue
		}
		seen = append(seen, key)

		candTok, candEndPos, candEndRow, candEndCol := d.scanPreferredTokenForState(st)

		score := 0
		for _, liveState := range d.glrStates {
			if d.lookupActionIndex(liveState, candTok.Symbol) != 0 {
				score++
			}
		}
		originActionCount := 0
		if idx := d.lookupActionIndex(st, candTok.Symbol); idx != 0 && int(idx) < len(d.language.ParseActions) {
			originActionCount = len(d.language.ParseActions[idx].Actions)
		}

		if score <= 0 {
			continue
		}

		candVisible := int(candTok.Symbol) < len(d.language.SymbolMetadata) && d.language.SymbolMetadata[candTok.Symbol].Visible
		splitPreference := 0
		if candTok.StartByte == bestTok.StartByte {
			splitPreference = d.compareAngleTokenPreference(candTok, bestTok)
		}
		better := !bestFound ||
			candTok.StartByte < bestTok.StartByte ||
			(candTok.StartByte == bestTok.StartByte && splitPreference > 0) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos > bestEndPos) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos == bestEndPos && candTok.EndByte > bestTok.EndByte) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos == bestEndPos && candTok.EndByte == bestTok.EndByte && d.preferSpecificTokenOnExactMatch(candTok, candEndPos, bestTok, bestEndPos)) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos == bestEndPos && candTok.EndByte == bestTok.EndByte && originActionCount > bestOriginActions) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos == bestEndPos && candTok.EndByte == bestTok.EndByte && score > bestScore) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos == bestEndPos && candTok.EndByte == bestTok.EndByte && score == bestScore && candVisible && !bestVisible)
		if better {
			bestFound = true
			bestScore = score
			bestTok = candTok
			bestEndPos = candEndPos
			bestEndRow = candEndRow
			bestEndCol = candEndCol
			bestVisible = candVisible
			bestOriginActions = originActionCount
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

func (d *dfaTokenSource) lexStateForState(state StateID) uint32 {
	lexModes := d.lexModeStartRows()
	if int(state) >= len(lexModes) {
		return 0
	}
	mode := lexModes[state]
	if after := mode.afterWhitespaceLexState; after != 0 && d.isAfterWhitespacePosition() {
		return after
	}
	return mode.lexState
}

func (d *dfaTokenSource) scanPreferredTokenForState(state StateID) (Token, int, uint32, uint32) {
	if d == nil || d.lexer == nil {
		return Token{}, 0, 0, 0
	}
	lexModes := d.lexModeStartRows()
	if int(state) >= len(lexModes) {
		return Token{}, d.lexer.pos, d.lexer.row, d.lexer.col
	}
	mode := lexModes[state]
	if mode.afterWhitespaceLexState == 0 {
		return d.scanDFATokenForState(state, mode.lexState)
	}
	if !d.isAtWhitespacePosition() && !d.isAfterWhitespacePosition() {
		return d.scanDFATokenForState(state, mode.lexState)
	}

	baseTok, baseEndPos, baseEndRow, baseEndCol := d.scanDFATokenForState(state, mode.lexState)
	afterTok, afterEndPos, afterEndRow, afterEndCol := d.scanDFATokenForState(state, mode.afterWhitespaceLexState)
	if d.shouldPreferBaseLexStateToken(baseTok, afterTok) {
		return baseTok, baseEndPos, baseEndRow, baseEndCol
	}
	return afterTok, afterEndPos, afterEndRow, afterEndCol
}

func (d *dfaTokenSource) scanDFATokenForState(state StateID, lexState uint32) (Token, int, uint32, uint32) {
	if d == nil || d.lexer == nil {
		return Token{}, 0, 0, 0
	}
	savedPos := d.lexer.pos
	savedRow := d.lexer.row
	savedCol := d.lexer.col
	savedState := d.state

	d.state = state
	tok := d.nextTokenForLexState(lexState)
	if d.hasZeroWidthStartAccept {
		if zeroTok, ok := d.preferZeroWidthStartAcceptForState(state, lexState, tok, savedPos, savedRow, savedCol); ok {
			tok = zeroTok
			d.lexer.pos = savedPos
			d.lexer.row = savedRow
			d.lexer.col = savedCol
		}
	}
	tok = d.promoteKeyword(tok)
	tok, endPos, endRow, endCol := d.normalizeDFAToken(tok, d.lexer.pos, d.lexer.row, d.lexer.col)

	d.lexer.pos = savedPos
	d.lexer.row = savedRow
	d.lexer.col = savedCol
	d.state = savedState

	return tok, endPos, endRow, endCol
}

func (d *dfaTokenSource) shouldPreferBaseLexStateToken(baseTok, afterTok Token) bool {
	if baseTok.Symbol == 0 {
		return false
	}
	if afterTok.Symbol == 0 {
		return true
	}
	if d.hasZeroWidthTokens && d.shouldPreferZeroWidthBaseLexStateToken(baseTok, afterTok) {
		return true
	}
	return baseTok.StartByte < afterTok.StartByte
}

func (d *dfaTokenSource) shouldPreferZeroWidthBaseLexStateToken(baseTok, afterTok Token) bool {
	if d == nil || d.language == nil || len(d.language.ZeroWidthTokens) == 0 {
		return false
	}
	if baseTok.StartByte != afterTok.StartByte || baseTok.EndByte != baseTok.StartByte {
		return false
	}
	sym := int(baseTok.Symbol)
	if sym < 0 || sym >= len(d.language.ZeroWidthTokens) || !d.language.ZeroWidthTokens[sym] {
		return false
	}
	return d.hasShiftActionForStateSymbol(d.state, baseTok.Symbol)
}

func (d *dfaTokenSource) preferZeroWidthStartAcceptForState(state StateID, lexState uint32, tok Token, startPos int, startRow, startCol uint32) (Token, bool) {
	if d == nil || d.language == nil || lexState == noLookaheadLexState || int(lexState) >= len(d.language.LexStates) {
		return Token{}, false
	}
	if tok.Symbol != 0 && tok.StartByte != uint32(startPos) {
		return Token{}, false
	}
	startAccept := d.language.LexStates[lexState].AcceptToken
	if startAccept == 0 || startAccept == tok.Symbol || !d.isZeroWidthSymbol(startAccept) {
		return Token{}, false
	}
	if !d.hasShiftActionForStateSymbol(state, startAccept) {
		return Token{}, false
	}
	if tok.Symbol != 0 && d.symbolVisibleOrNamed(tok.Symbol) && !d.sameSymbolName(startAccept, tok.Symbol) {
		return Token{}, false
	}
	pt := Point{Row: startRow, Column: startCol}
	return Token{
		Symbol:     startAccept,
		StartByte:  uint32(startPos),
		EndByte:    uint32(startPos),
		StartPoint: pt,
		EndPoint:   pt,
	}, true
}

func (d *dfaTokenSource) isZeroWidthSymbol(sym Symbol) bool {
	if d == nil || d.language == nil || len(d.language.ZeroWidthTokens) == 0 {
		return false
	}
	idx := int(sym)
	return idx >= 0 && idx < len(d.language.ZeroWidthTokens) && d.language.ZeroWidthTokens[idx]
}

func (d *dfaTokenSource) hasShiftActionForStateSymbol(state StateID, sym Symbol) bool {
	if d == nil || d.language == nil || d.lookupActionIndex == nil || sym == 0 {
		return false
	}
	idx := d.lookupActionIndex(state, sym)
	if idx == 0 || int(idx) >= len(d.language.ParseActions) {
		return false
	}
	for _, act := range d.language.ParseActions[idx].Actions {
		if act.Type == ParseActionShift {
			return true
		}
	}
	return false
}

func (d *dfaTokenSource) symbolVisibleOrNamed(sym Symbol) bool {
	if meta, ok := d.symbolMetadata(sym); ok {
		return meta.Visible || meta.Named
	}
	return false
}

func (d *dfaTokenSource) isAtWhitespacePosition() bool {
	if d == nil || d.lexer == nil || d.lexer.pos < 0 || d.lexer.pos >= len(d.lexer.source) {
		return false
	}
	r, _ := utf8.DecodeRune(d.lexer.source[d.lexer.pos:])
	return unicode.IsSpace(r)
}

func (d *dfaTokenSource) isAfterWhitespacePosition() bool {
	if d == nil || d.lexer == nil || d.lexer.pos <= 0 || d.lexer.pos > len(d.lexer.source) {
		return false
	}
	r, _ := utf8.DecodeLastRune(d.lexer.source[:d.lexer.pos])
	return unicode.IsSpace(r)
}

func (d *dfaTokenSource) normalizeDFAToken(tok Token, endPos int, endRow, endCol uint32) (Token, int, uint32, uint32) {
	if d == nil || d.language == nil || d.lexer == nil {
		return tok, endPos, endRow, endCol
	}
	if d.isBashGenerated {
		if nlTok, nlEndPos, nlEndRow, nlEndCol, ok := d.bashGeneratedDFAOnlyNewlineToken(tok); ok {
			return nlTok, nlEndPos, nlEndRow, nlEndCol
		}
	}
	if splitTok, splitEndPos, splitEndRow, splitEndCol, ok := d.splitCompactCloseAngleToken(tok); ok {
		return splitTok, splitEndPos, splitEndRow, splitEndCol
	}
	if d.isBashGenerated {
		if splitTok, splitEndPos, splitEndRow, splitEndCol, ok := d.splitBashGeneratedDoubleCloseParenToken(tok); ok {
			return splitTok, splitEndPos, splitEndRow, splitEndCol
		}
	}
	if !d.isBash || d.symbolName(tok.Symbol) != "\\n" || tok.EndByte <= tok.StartByte+1 {
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

func (d *dfaTokenSource) bashGeneratedDFAOnlyNewlineToken(tok Token) (Token, int, uint32, uint32, bool) {
	if d == nil || d.language == nil || d.lexer == nil || !d.isBashGenerated ||
		d.symbolName(tok.Symbol) == "\\n" || tok.EndByte <= tok.StartByte {
		return tok, 0, 0, 0, false
	}
	start := int(tok.StartByte)
	end := int(tok.EndByte)
	if start < 0 || end > len(d.lexer.source) {
		return tok, 0, 0, 0, false
	}
	for i := start; i < end; i++ {
		if d.lexer.source[i] != '\n' {
			return tok, 0, 0, 0, false
		}
	}
	sym, ok := d.bestActiveSymbolByName("\\n")
	if !ok || sym == 0 {
		if sym, ok = symbolByName(d.language, "\\n"); !ok || sym == 0 {
			return tok, 0, 0, 0, false
		}
	}
	tok.Symbol = sym
	tok.EndByte = tok.StartByte + 1
	tok.EndPoint = Point{Row: tok.StartPoint.Row + 1, Column: 0}
	tok.Text = "\n"
	return tok, start + 1, tok.StartPoint.Row + 1, 0, true
}

func (d *dfaTokenSource) splitBashGeneratedDoubleCloseParenToken(tok Token) (Token, int, uint32, uint32, bool) {
	if d == nil || d.language == nil || d.lexer == nil || !d.isBashGenerated ||
		d.symbolName(tok.Symbol) != "))" || tok.EndByte != tok.StartByte+2 {
		return tok, 0, 0, 0, false
	}
	start := int(tok.StartByte)
	if start < 0 || start+1 >= len(d.lexer.source) ||
		d.lexer.source[start] != ')' || d.lexer.source[start+1] != ')' ||
		d.bashGeneratedInArithmeticExpansion(start) {
		return tok, 0, 0, 0, false
	}
	sym, ok := d.bestActiveSymbolByName(")")
	if !ok || sym == 0 {
		return tok, 0, 0, 0, false
	}
	tok.Symbol = sym
	tok.EndByte = tok.StartByte + 1
	tok.EndPoint = Point{Row: tok.StartPoint.Row, Column: tok.StartPoint.Column + 1}
	tok.Text = ")"
	return tok, start + 1, tok.EndPoint.Row, tok.EndPoint.Column, true
}

func (d *dfaTokenSource) bashSkippedSignificantNewlineToken(tok Token, scanStartPos int, scanStartRow, scanStartCol uint32) (Token, bool) {
	if d == nil || d.language == nil || d.lexer == nil || !d.isBashGenerated {
		return Token{}, false
	}
	if tok.Symbol == 0 || int(tok.StartByte) <= scanStartPos || scanStartPos < 0 || scanStartPos >= len(d.lexer.source) {
		return Token{}, false
	}
	if d.lexer.source[scanStartPos] != '\n' {
		return Token{}, false
	}
	sym, ok := d.bestActiveSymbolByName("\\n")
	if !ok || sym == 0 {
		return Token{}, false
	}
	return Token{
		Symbol:     sym,
		StartByte:  uint32(scanStartPos),
		EndByte:    uint32(scanStartPos + 1),
		StartPoint: Point{Row: scanStartRow, Column: scanStartCol},
		EndPoint:   Point{Row: scanStartRow + 1, Column: 0},
		Text:       "\n",
	}, true
}

func (d *dfaTokenSource) bashGeneratedTokenOverZeroWidthConcat(tok Token, scanStartPos int, scanStartRow, scanStartCol uint32) (Token, bool) {
	if d == nil || d.language == nil || d.lexer == nil || !d.isBashGenerated {
		return Token{}, false
	}
	if d.symbolName(tok.Symbol) != "_concat" || tok.StartByte != tok.EndByte ||
		int(tok.StartByte) != scanStartPos || scanStartPos < 0 || scanStartPos >= len(d.lexer.source) {
		return Token{}, false
	}
	if d.lexer.source[scanStartPos] == '\n' {
		sym, ok := d.bestActiveSymbolByName("\\n")
		if !ok || sym == 0 {
			return Token{}, false
		}
		return Token{
			Symbol:     sym,
			StartByte:  uint32(scanStartPos),
			EndByte:    uint32(scanStartPos + 1),
			StartPoint: Point{Row: scanStartRow, Column: scanStartCol},
			EndPoint:   Point{Row: scanStartRow + 1, Column: 0},
			Text:       "\n",
		}, true
	}
	if opTok, ok := d.bashGeneratedOperatorTokenAt(scanStartPos, scanStartRow, scanStartCol); ok {
		if DebugDFA.Load() {
			fmt.Printf("  BASH CONCAT->DFA %s %d %d state=%d\n", d.symbolName(opTok.Symbol), opTok.StartByte, opTok.EndByte, d.state)
		}
		return opTok, true
	}
	dfaTok, endPos, endRow, endCol := d.scanPreferredTokenForState(d.state)
	if dfaTok.Symbol == 0 || int(dfaTok.StartByte) != scanStartPos || endPos <= scanStartPos {
		return Token{}, false
	}
	if !d.bashGeneratedShouldPreferDFATokenOverConcat(dfaTok) {
		return Token{}, false
	}
	dfaTok.EndByte = uint32(endPos)
	dfaTok.EndPoint = Point{Row: endRow, Column: endCol}
	return dfaTok, true
}

func (d *dfaTokenSource) bashGeneratedOperatorTokenAt(pos int, row, col uint32) (Token, bool) {
	if d == nil || d.lexer == nil || pos < 0 || pos >= len(d.lexer.source) {
		return Token{}, false
	}
	for _, lit := range bashGeneratedConcatOperatorLookaheads {
		if !bytes.HasPrefix(d.lexer.source[pos:], lit.bytes) {
			continue
		}
		name := lit.name
		if name == "" {
			name = lit.text
		}
		if bashGeneratedOperatorRequiresArithmeticContext(name) && !d.bashGeneratedInArithmeticExpansion(pos) {
			continue
		}
		sym, ok := d.bestActiveSymbolByName(name)
		if !ok || sym == 0 {
			continue
		}
		endCol := col + uint32(len(lit.text))
		return Token{
			Symbol:     sym,
			StartByte:  uint32(pos),
			EndByte:    uint32(pos + len(lit.text)),
			StartPoint: Point{Row: row, Column: col},
			EndPoint:   Point{Row: row, Column: endCol},
			Text:       lit.text,
		}, true
	}
	return Token{}, false
}

func bashGeneratedOperatorRequiresArithmeticContext(name string) bool {
	switch name {
	case "++", "--",
		"+=", "-=", "*=", "/=", "%=", "**=", "<<=", ">>=", "&=", "^=", "|=",
		"^",
		"+", "-", "*", "/", "%", "**", "))",
		"?", ":", ",":
		return true
	default:
		return false
	}
}

func (d *dfaTokenSource) bashGeneratedInArithmeticExpansion(pos int) bool {
	if d == nil || d.lexer == nil || pos <= 0 || pos > len(d.lexer.source) {
		return false
	}
	if d.bashArithmeticCachePos == pos {
		return d.bashArithmeticCacheResult
	}

	start := 0
	depth := 0
	skipUntil := 0
	if d.bashArithmeticCachePos >= 0 && pos >= d.bashArithmeticCachePos {
		start = d.bashArithmeticCachePos
		depth = d.bashArithmeticCacheDepth
		skipUntil = d.bashArithmeticCacheSkipUntil
	}
	i := start
	if skipUntil > i {
		i = skipUntil
	}
	src := d.lexer.source
	for i < pos {
		switch {
		case i+len("$((") <= pos && bytes.HasPrefix(src[i:], []byte("$((")):
			depth++
			i += len("$((")
			skipUntil = 0
		case depth > 0 && i+len("))") <= pos && bytes.HasPrefix(src[i:], []byte("))")):
			depth--
			i += len("))")
			skipUntil = 0
		case src[i] == '\\':
			i += 2
			if i > pos {
				skipUntil = i
			} else {
				skipUntil = 0
			}
		default:
			_, size := utf8.DecodeRune(src[i:pos])
			if size <= 0 {
				size = 1
			}
			i += size
			skipUntil = 0
		}
	}
	result := depth > 0
	d.bashArithmeticCachePos = pos
	d.bashArithmeticCacheDepth = depth
	d.bashArithmeticCacheSkipUntil = skipUntil
	d.bashArithmeticCacheResult = result
	return result
}

type bashGeneratedConcatOperatorLookahead struct {
	text  string
	name  string
	bytes []byte
}

var bashGeneratedConcatOperatorLookaheads = makeBashGeneratedConcatOperatorLookaheads(
	bashGeneratedConcatOperatorLookahead{text: "<<<"},
	bashGeneratedConcatOperatorLookahead{text: "&>>"},
	bashGeneratedConcatOperatorLookahead{text: "<<-"},
	bashGeneratedConcatOperatorLookahead{text: "<&-"},
	bashGeneratedConcatOperatorLookahead{text: ">&-"},
	bashGeneratedConcatOperatorLookahead{text: "**="},
	bashGeneratedConcatOperatorLookahead{text: "<<="},
	bashGeneratedConcatOperatorLookahead{text: ">>="},
	bashGeneratedConcatOperatorLookahead{text: "|&"},
	bashGeneratedConcatOperatorLookahead{text: "&>"},
	bashGeneratedConcatOperatorLookahead{text: "<&"},
	bashGeneratedConcatOperatorLookahead{text: ">&"},
	bashGeneratedConcatOperatorLookahead{text: ">|"},
	bashGeneratedConcatOperatorLookahead{text: "++"},
	bashGeneratedConcatOperatorLookahead{text: "--"},
	bashGeneratedConcatOperatorLookahead{text: "+="},
	bashGeneratedConcatOperatorLookahead{text: "-="},
	bashGeneratedConcatOperatorLookahead{text: "*="},
	bashGeneratedConcatOperatorLookahead{text: "/="},
	bashGeneratedConcatOperatorLookahead{text: "%="},
	bashGeneratedConcatOperatorLookahead{text: "&="},
	bashGeneratedConcatOperatorLookahead{text: "^="},
	bashGeneratedConcatOperatorLookahead{text: "|="},
	bashGeneratedConcatOperatorLookahead{text: "||"},
	bashGeneratedConcatOperatorLookahead{text: "&&"},
	bashGeneratedConcatOperatorLookahead{text: "=="},
	bashGeneratedConcatOperatorLookahead{text: "!="},
	bashGeneratedConcatOperatorLookahead{text: "<="},
	bashGeneratedConcatOperatorLookahead{text: ">="},
	bashGeneratedConcatOperatorLookahead{text: "<<"},
	bashGeneratedConcatOperatorLookahead{text: ">>"},
	bashGeneratedConcatOperatorLookahead{text: "**"},
	bashGeneratedConcatOperatorLookahead{text: "))"},
	bashGeneratedConcatOperatorLookahead{text: ";;"},
	bashGeneratedConcatOperatorLookahead{text: "+"},
	bashGeneratedConcatOperatorLookahead{text: "-"},
	bashGeneratedConcatOperatorLookahead{text: "*"},
	bashGeneratedConcatOperatorLookahead{text: "/"},
	bashGeneratedConcatOperatorLookahead{text: "%"},
	bashGeneratedConcatOperatorLookahead{text: "|"},
	bashGeneratedConcatOperatorLookahead{text: "^"},
	bashGeneratedConcatOperatorLookahead{text: "&"},
	bashGeneratedConcatOperatorLookahead{text: "<"},
	bashGeneratedConcatOperatorLookahead{text: ">"},
	bashGeneratedConcatOperatorLookahead{text: "?", name: "\\?"},
	bashGeneratedConcatOperatorLookahead{text: ":"},
	bashGeneratedConcatOperatorLookahead{text: ","},
	bashGeneratedConcatOperatorLookahead{text: ";"},
)

func makeBashGeneratedConcatOperatorLookaheads(in ...bashGeneratedConcatOperatorLookahead) []bashGeneratedConcatOperatorLookahead {
	for i := range in {
		in[i].bytes = []byte(in[i].text)
	}
	return in
}

func (d *dfaTokenSource) bashGeneratedShouldPreferDFATokenOverConcat(tok Token) bool {
	switch d.symbolName(tok.Symbol) {
	case "++", "--",
		"+=", "-=", "*=", "/=", "%=", "**=", "<<=", ">>=", "&=", "^=", "|=",
		"||", "&&", "|", "|&", "^", "&",
		"==", "!=", "<", ">", "<=", ">=",
		"<<", "<<-", ">>", "<<<", "&>", "&>>", "<&", ">&", "<&-", ">&-", ">|",
		"+", "-", "*", "/", "%", "**",
		"?", ":", ",", "))", ";", ";;":
		return true
	default:
		return false
	}
}

func (d *dfaTokenSource) splitCompactCloseAngleToken(tok Token) (Token, int, uint32, uint32, bool) {
	if d == nil || d.language == nil || d.lookupActionIndex == nil {
		return tok, 0, 0, 0, false
	}
	switch d.language.Name {
	case "dart", "java", "tsx", "typescript":
	default:
		return tok, 0, 0, 0, false
	}
	if d.symbolName(tok.Symbol) != ">>" {
		return tok, 0, 0, 0, false
	}

	gtSym, ok := d.bestActiveSymbolByName(">")
	if !ok {
		return tok, 0, 0, 0, false
	}
	shiftSym, shiftOK := d.bestActiveSymbolByName(">>")
	if !d.shouldSplitCompactCloseAngleToken(tok, gtSym, shiftSym, shiftOK) {
		return tok, 0, 0, 0, false
	}
	if tok.EndByte != tok.StartByte+2 || tok.EndPoint.Row != tok.StartPoint.Row {
		return tok, 0, 0, 0, false
	}

	tok.Symbol = gtSym
	tok.EndByte = tok.StartByte + 1
	tok.EndPoint = Point{Row: tok.StartPoint.Row, Column: tok.StartPoint.Column + 1}
	if len(tok.Text) > 1 {
		tok.Text = tok.Text[:1]
	}
	return tok, int(tok.EndByte), tok.EndPoint.Row, tok.EndPoint.Column, true
}

func (d *dfaTokenSource) shouldSplitCompactCloseAngleToken(tok Token, gtSym, shiftSym Symbol, shiftOK bool) bool {
	if d != nil && d.language != nil && d.language.Name == "java" && !d.hasJavaUnclosedAngleBefore(int(tok.StartByte)) {
		return false
	}
	if !shiftOK {
		return true
	}
	gtSpec := d.activeActionSpecificity(gtSym)
	shiftSpec := d.activeActionSpecificity(shiftSym)
	if gtSpec > shiftSpec {
		return true
	}
	if gtSpec < shiftSpec {
		return false
	}
	next := d.nextNonSpaceByte(int(tok.EndByte))
	switch next {
	case 0, '(', ')', '[', ']', '{', '}', ',', '.', ';', ':', '?':
		return true
	default:
		return isTypeScriptIdentifierStartByte(next) &&
			d.sharesSameReduceOnlyActions(gtSym, shiftSym) &&
			d.hasTypeAssertionStyleOpenerBefore(int(tok.StartByte))
	}
}

func (d *dfaTokenSource) hasJavaUnclosedAngleBefore(pos int) bool {
	if d == nil || d.lexer == nil || pos <= 0 {
		return false
	}
	depth := 0
	for i := pos - 1; i >= 0; i-- {
		switch d.lexer.source[i] {
		case ';', '{', '}', '(', ')':
			return depth > 0
		case '>':
			depth--
		case '<':
			depth++
			if depth > 0 {
				return true
			}
		}
	}
	return depth > 0
}

func (d *dfaTokenSource) nextNonSpaceByte(pos int) byte {
	if d == nil || d.lexer == nil {
		return 0
	}
	for pos < len(d.lexer.source) {
		switch d.lexer.source[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
			continue
		default:
			return d.lexer.source[pos]
		}
	}
	return 0
}

func (d *dfaTokenSource) nextNonSpacePos(pos int) int {
	if d == nil || d.lexer == nil {
		return -1
	}
	for pos < len(d.lexer.source) {
		switch d.lexer.source[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
			continue
		default:
			return pos
		}
	}
	return len(d.lexer.source)
}

func (d *dfaTokenSource) scanBalancedTypeScriptKeywordSuffix(openPos int, open, close byte) (int, bool) {
	if d == nil || d.lexer == nil || openPos < 0 || openPos >= len(d.lexer.source) || d.lexer.source[openPos] != open {
		return -1, false
	}
	depth := 0
	for i := openPos; i < len(d.lexer.source); i++ {
		switch d.lexer.source[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return -1, false
}

func (d *dfaTokenSource) shouldPreferJavaScriptTypeScriptContextualIdentifier(tok, kwTok Token, kwHasAction, idHasAction bool) bool {
	if d == nil || d.language == nil || d.lexer == nil || !idHasAction || !kwHasAction {
		return false
	}
	switch d.language.Name {
	case "javascript", "typescript", "tsx":
	default:
		return false
	}
	if int(kwTok.Symbol) >= len(d.language.SymbolNames) {
		return false
	}
	switch d.language.SymbolNames[kwTok.Symbol] {
	case "get", "set":
	default:
		return false
	}
	nextPos := d.nextNonSpacePos(int(tok.EndByte))
	if nextPos < 0 || nextPos >= len(d.lexer.source) {
		return false
	}
	switch d.lexer.source[nextPos] {
	case '.', '(':
		return true
	case '[':
		afterBracket, ok := d.scanBalancedTypeScriptKeywordSuffix(nextPos, '[', ']')
		if !ok {
			return false
		}
		afterBracket = d.nextNonSpacePos(afterBracket)
		if afterBracket < 0 || afterBracket >= len(d.lexer.source) {
			return true
		}
		switch d.lexer.source[afterBracket] {
		case '.', '[', '}', ',', ';', ':', '?':
			return true
		case '(':
			afterCall, ok := d.scanBalancedTypeScriptKeywordSuffix(afterBracket, '(', ')')
			if !ok {
				return true
			}
			afterCall = d.nextNonSpacePos(afterCall)
			if afterCall < 0 || afterCall >= len(d.lexer.source) {
				return true
			}
			switch d.lexer.source[afterCall] {
			case '{', ';':
				return false
			default:
				return true
			}
		default:
			return true
		}
	default:
		return false
	}
}

func isTypeScriptIdentifierStartByte(ch byte) bool {
	return ch == '_' || ch == '$' ||
		(ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z')
}

func (d *dfaTokenSource) hasTypeAssertionStyleOpenerBefore(pos int) bool {
	if d == nil || d.lexer == nil || pos <= 0 {
		return false
	}
	for i := pos - 1; i >= 0; i-- {
		if isASCIIWhitespace(d.lexer.source[i]) {
			continue
		}
		if d.lexer.source[i] != '<' {
			continue
		}
		prev := d.prevNonSpaceByte(i - 1)
		switch prev {
		case 0, '\n', '=', '(', '[', '{', ':', ',', '?':
			return true
		default:
			continue
		}
	}
	return false
}

func (d *dfaTokenSource) prevNonSpaceByte(pos int) byte {
	if d == nil || d.lexer == nil {
		return 0
	}
	for pos >= 0 {
		if !isASCIIWhitespace(d.lexer.source[pos]) {
			return d.lexer.source[pos]
		}
		pos--
	}
	return 0
}

func isASCIIWhitespace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func (d *dfaTokenSource) sharesSameReduceOnlyActions(a, b Symbol) bool {
	if d == nil || d.language == nil || d.lookupActionIndex == nil || a == 0 || b == 0 {
		return false
	}
	aIdx := d.lookupActionIndex(d.state, a)
	bIdx := d.lookupActionIndex(d.state, b)
	if aIdx == 0 || bIdx == 0 || aIdx != bIdx || int(aIdx) >= len(d.language.ParseActions) {
		return false
	}
	actions := d.language.ParseActions[aIdx].Actions
	if len(actions) == 0 {
		return false
	}
	for _, act := range actions {
		if act.Type != ParseActionReduce {
			return false
		}
	}
	return true
}

func (d *dfaTokenSource) bestActiveSymbolByName(name string) (Symbol, bool) {
	if d == nil || d.language == nil || d.lookupActionIndex == nil {
		return 0, false
	}
	best := Symbol(0)
	bestSpecificity := -1
	bestVisible := false
	found := false
	for i := range d.language.SymbolNames {
		sym := Symbol(i)
		if d.symbolName(sym) != name || !d.hasAnyActionForSymbol(sym) {
			continue
		}
		visible := false
		if meta, ok := d.symbolMetadata(sym); ok {
			visible = meta.Visible
		}
		specificity := d.activeActionSpecificity(sym)
		if !found || specificity > bestSpecificity || (specificity == bestSpecificity && visible && !bestVisible) {
			best = sym
			bestSpecificity = specificity
			bestVisible = visible
			found = true
		}
	}
	return best, found
}

func (d *dfaTokenSource) symbolName(sym Symbol) string {
	if d == nil || d.language == nil {
		return ""
	}
	if meta, ok := d.symbolMetadata(sym); ok && meta.Name != "" {
		return meta.Name
	}
	idx := int(sym)
	if idx < 0 || idx >= len(d.language.SymbolNames) {
		return ""
	}
	return d.language.SymbolNames[idx]
}

func (d *dfaTokenSource) preferSpecificTokenOnExactMatch(candTok Token, candEndPos int, bestTok Token, bestEndPos int) bool {
	if d == nil || d.language == nil {
		return false
	}
	if candTok.StartByte != bestTok.StartByte || candTok.EndByte != bestTok.EndByte || candEndPos != bestEndPos {
		return false
	}
	if d.language.KeywordCaptureToken != 0 {
		candIsCapture := candTok.Symbol == d.language.KeywordCaptureToken
		bestIsCapture := bestTok.Symbol == d.language.KeywordCaptureToken
		if bestIsCapture != candIsCapture {
			return bestIsCapture && !candIsCapture
		}
	}
	if d.sameSymbolName(candTok.Symbol, bestTok.Symbol) {
		candSpecificity := d.activeActionSpecificity(candTok.Symbol)
		bestSpecificity := d.activeActionSpecificity(bestTok.Symbol)
		if candSpecificity != bestSpecificity {
			return candSpecificity > bestSpecificity
		}
	}
	candMeta, candOK := d.symbolMetadata(candTok.Symbol)
	bestMeta, bestOK := d.symbolMetadata(bestTok.Symbol)
	if !candOK || !bestOK {
		return false
	}
	if candMeta.Visible != bestMeta.Visible {
		return candMeta.Visible
	}
	return candMeta.Visible && !candMeta.Named && bestMeta.Visible && bestMeta.Named
}

func (d *dfaTokenSource) compareAngleTokenPreference(candTok, bestTok Token) int {
	if d == nil || d.language == nil {
		return 0
	}
	switch d.language.Name {
	case "dart", "java", "tsx", "typescript":
	default:
		return 0
	}
	if int(candTok.Symbol) >= len(d.language.SymbolNames) || int(bestTok.Symbol) >= len(d.language.SymbolNames) {
		return 0
	}
	candName := d.language.SymbolNames[candTok.Symbol]
	bestName := d.language.SymbolNames[bestTok.Symbol]
	if candName == ">" && bestName == ">>" {
		return 1
	}
	if candName == ">>" && bestName == ">" {
		return -1
	}
	return 0
}

func (d *dfaTokenSource) sameSymbolName(a, b Symbol) bool {
	if d == nil || d.language == nil {
		return false
	}
	if am, ok := d.symbolMetadata(a); ok {
		if bm, ok := d.symbolMetadata(b); ok && am.Name != "" && bm.Name != "" {
			return am.Name == bm.Name
		}
	}
	ai := int(a)
	bi := int(b)
	if ai < 0 || bi < 0 || ai >= len(d.language.SymbolNames) || bi >= len(d.language.SymbolNames) {
		return false
	}
	return d.language.SymbolNames[ai] == d.language.SymbolNames[bi]
}

func (d *dfaTokenSource) activeActionSpecificity(sym Symbol) int {
	if d == nil || d.language == nil || d.lookupActionIndex == nil || sym == 0 {
		return 0
	}
	type actionStats struct {
		maxDyn     int
		totalDyn   int
		maxActions int
		totalActs  int
		supporting int
	}
	stats := actionStats{}
	visit := func(st StateID) {
		idx := d.lookupActionIndex(st, sym)
		if idx == 0 || int(idx) >= len(d.language.ParseActions) {
			return
		}
		acts := d.language.ParseActions[idx].Actions
		if len(acts) == 0 {
			return
		}
		stats.supporting++
		if len(acts) > stats.maxActions {
			stats.maxActions = len(acts)
		}
		stats.totalActs += len(acts)
		for _, act := range acts {
			dyn := int(act.DynamicPrecedence)
			if dyn > stats.maxDyn {
				stats.maxDyn = dyn
			}
			stats.totalDyn += dyn
		}
	}
	visit(d.state)
	for i, st := range d.glrStates {
		if st == d.state || d.priorGLRState(i, st) {
			continue
		}
		visit(st)
	}
	return (((stats.maxDyn*1024)+stats.totalDyn)*1024 + stats.maxActions*64 + stats.totalActs*4 + stats.supporting)
}

func (d *dfaTokenSource) symbolMetadata(sym Symbol) (SymbolMetadata, bool) {
	if d == nil || d.language == nil {
		return SymbolMetadata{}, false
	}
	idx := int(sym)
	if idx < 0 || idx >= len(d.language.SymbolMetadata) {
		return SymbolMetadata{}, false
	}
	return d.language.SymbolMetadata[idx], true
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
		d.singleState[0] = d.state
		states = d.singleState[:]
	}
	if tok, ok := d.nextGLRScoredExternalToken(states); ok {
		return tok, true
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
	} else if len(d.externalValidByState) > 0 {
		for _, st := range states {
			if int(st) >= len(d.externalValidByState) {
				continue
			}
			row := d.externalValidByState[int(st)]
			for _, extIdx := range row {
				i := int(extIdx)
				if i < len(valid) && !valid[i] {
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
	if d.shouldDeferFortranExternalEndOfStatementToDFA(valid, states) {
		return Token{}, false
	}
	if DebugDFA.Load() {
		fmt.Printf("  EXT valid pos=%d state=%d glr=%v els=%s valid=%s\n",
			d.lexer.pos, d.state, states, d.debugExternalLexStateIDs(states), d.debugExternalValidNames(valid))
	}

	if d.language.ExternalScanner == nil {
		tok, ok := d.syntheticExternalToken(valid)
		if !ok {
			return Token{}, false
		}
		d.trackZeroWidthExternalToken(tok)
		return tok, true
	}

	el := &d.externalLexer
	el.reset(d.lexer.source, d.lexer.pos, d.lexer.row, d.lexer.col)
	if !d.runExternalScannerWithRetry(el, valid) {
		if d.isBashGenerated {
			if tok, ok := d.bashGeneratedSyntheticExternalLiteral(valid); ok {
				if DebugDFA.Load() {
					fmt.Printf("  EXT synthetic %s %d %d state=%d\n", d.symbolName(tok.Symbol), tok.StartByte, tok.EndByte, d.state)
				}
				d.trackZeroWidthExternalToken(tok)
				d.lexer.pos = int(tok.EndByte)
				d.lexer.row = tok.EndPoint.Row
				d.lexer.col = tok.EndPoint.Column
				return tok, true
			}
		}
		if DebugDFA.Load() {
			fmt.Printf("  EXT miss pos=%d state=%d valid=%s\n", d.lexer.pos, d.state, d.debugExternalValidNames(valid))
		}
		return Token{}, false
	}
	tok, ok := el.token()
	if !ok {
		return Token{}, false
	}

	if dfaTok, endPos, endRow, endCol, ok := d.preferDFASemicolonOverJSXText(tok, states); ok {
		d.lexer.pos = endPos
		d.lexer.row = endRow
		d.lexer.col = endCol
		return dfaTok, true
	}

	d.trackZeroWidthExternalToken(tok)

	d.lexer.pos = int(tok.EndByte)
	d.lexer.row = tok.EndPoint.Row
	d.lexer.col = tok.EndPoint.Column
	return tok, true
}

func (d *dfaTokenSource) bashGeneratedSyntheticExternalLiteral(valid []bool) (Token, bool) {
	if d == nil || d.language == nil || d.lexer == nil || !d.isBashGenerated {
		return Token{}, false
	}
	literals := []string{"<<-", "<<", "}", "]", "(", "esac"}
	for _, lit := range literals {
		if !bytes.HasPrefix(d.lexer.source[d.lexer.pos:], []byte(lit)) {
			continue
		}
		if lit == "<<" && d.bashGeneratedLongerHeredocOperatorAt(d.lexer.pos) {
			continue
		}
		for i, sym := range d.language.ExternalSymbols {
			if i >= len(valid) || !valid[i] || d.symbolName(sym) != lit {
				continue
			}
			endCol := d.lexer.col + uint32(len(lit))
			return Token{
				Symbol:     sym,
				StartByte:  uint32(d.lexer.pos),
				EndByte:    uint32(d.lexer.pos + len(lit)),
				StartPoint: Point{Row: d.lexer.row, Column: d.lexer.col},
				EndPoint:   Point{Row: d.lexer.row, Column: endCol},
				Text:       lit,
			}, true
		}
	}
	return Token{}, false
}

func (d *dfaTokenSource) bashGeneratedLongerHeredocOperatorAt(pos int) bool {
	if d == nil || d.lexer == nil || pos < 0 || pos+2 >= len(d.lexer.source) {
		return false
	}
	switch d.lexer.source[pos+2] {
	case '<', '-':
		return bytes.HasPrefix(d.lexer.source[pos:], []byte("<<<")) ||
			bytes.HasPrefix(d.lexer.source[pos:], []byte("<<-"))
	default:
		return false
	}
}

func (d *dfaTokenSource) preferDFASemicolonOverJSXText(tok Token, states []StateID) (Token, int, uint32, uint32, bool) {
	if d == nil || d.lexer == nil || d.language == nil || d.lookupActionIndex == nil {
		return Token{}, 0, 0, 0, false
	}
	sym := int(tok.Symbol)
	if sym < 0 || sym >= len(d.language.SymbolNames) || d.language.SymbolNames[sym] != extNameJSXText {
		return Token{}, 0, 0, 0, false
	}
	start := int(tok.StartByte)
	if start < 0 || start >= len(d.lexer.source) || d.lexer.source[start] != ';' {
		return Token{}, 0, 0, 0, false
	}

	for _, st := range states {
		cand, endPos, endRow, endCol := d.scanPreferredTokenForState(st)
		candSym := int(cand.Symbol)
		if int(cand.StartByte) != start || candSym < 0 || candSym >= len(d.language.SymbolNames) {
			continue
		}
		if d.language.SymbolNames[candSym] != ";" {
			continue
		}
		if d.lookupActionIndex(st, cand.Symbol) == 0 {
			continue
		}
		return cand, endPos, endRow, endCol, true
	}

	return Token{}, 0, 0, 0, false
}

func (d *dfaTokenSource) shouldDeferFortranExternalEndOfStatementToDFA(valid []bool, states []StateID) bool {
	if d == nil || d.language == nil || d.lexer == nil || d.language.Name != "fortran" {
		return false
	}
	if d.lexer.pos < 0 || d.lexer.pos >= len(d.lexer.source) {
		return false
	}
	switch d.lexer.source[d.lexer.pos] {
	case '\n', '\r':
	default:
		return false
	}
	if !d.currentLineStartsWithHashDirective() {
		return false
	}
	hasExternalEnd := false
	for i, ok := range valid {
		if !ok || i >= len(d.language.ExternalSymbols) {
			continue
		}
		if d.symbolName(d.language.ExternalSymbols[i]) == "_external_end_of_statement" {
			hasExternalEnd = true
			break
		}
	}
	if !hasExternalEnd {
		return false
	}
	if len(states) == 0 {
		var single [1]StateID
		single[0] = d.state
		states = single[:]
	}
	for _, st := range states {
		tok, endPos, _, _ := d.scanPreferredTokenForState(st)
		if tok.Symbol == 0 || tok.StartByte != uint32(d.lexer.pos) || endPos <= d.lexer.pos {
			continue
		}
		name := d.symbolName(tok.Symbol)
		if strings.Contains(name, "preproc_") || isExplicitLineBreakSymbolName(name) {
			return true
		}
	}
	return false
}

func isExplicitLineBreakSymbolName(name string) bool {
	switch name {
	case "\n", "\r", "\r\n":
		return true
	default:
		return false
	}
}

func (d *dfaTokenSource) currentLineStartsWithHashDirective() bool {
	if d == nil || d.lexer == nil {
		return false
	}
	pos := d.lexer.pos - 1
	for pos >= 0 && d.lexer.source[pos] != '\n' && d.lexer.source[pos] != '\r' {
		pos--
	}
	pos++
	for pos < len(d.lexer.source) {
		switch d.lexer.source[pos] {
		case ' ', '\t':
			pos++
			continue
		case '#':
			return true
		default:
			return false
		}
	}
	return false
}

func (d *dfaTokenSource) shouldSuppressFortranPreprocDefineNewline(tok Token) bool {
	if d == nil || d.language == nil || d.lexer == nil || d.language.Name != "fortran" || tok.Symbol == 0 {
		return false
	}
	name := d.symbolName(tok.Symbol)
	if !strings.Contains(name, "preproc_def_token") {
		return false
	}
	if tok.EndByte <= tok.StartByte || int(tok.StartByte) > len(d.lexer.source) {
		return false
	}
	return !d.lineAtByteStartsWithHashDefine(int(tok.StartByte))
}

func (d *dfaTokenSource) lineAtByteStartsWithHashDefine(pos int) bool {
	if d == nil || d.lexer == nil {
		return false
	}
	if pos > len(d.lexer.source) {
		pos = len(d.lexer.source)
	}
	start := pos - 1
	for start >= 0 && d.lexer.source[start] != '\n' && d.lexer.source[start] != '\r' {
		start--
	}
	start++
	for start < len(d.lexer.source) {
		switch d.lexer.source[start] {
		case ' ', '\t':
			start++
			continue
		case '#':
			start++
			for start < len(d.lexer.source) && (d.lexer.source[start] == ' ' || d.lexer.source[start] == '\t') {
				start++
			}
			return bytes.HasPrefix(d.lexer.source[start:], []byte("define"))
		default:
			return false
		}
	}
	return false
}

func (d *dfaTokenSource) nextGLRScoredExternalToken(states []StateID) (Token, bool) {
	if d == nil || d.language == nil || d.language.ExternalScanner == nil || d.lookupActionIndex == nil {
		return Token{}, false
	}
	if len(states) <= 1 || len(d.language.ExternalLexStates) == 0 {
		return Token{}, false
	}

	primaryELS := -1
	if int(d.state) < len(d.language.LexModes) {
		primaryELS = int(d.language.LexModes[d.state].ExternalLexState)
	}

	var elsOrderBuf [16]int
	elsOrder := elsOrderBuf[:0]
	elsOrder = appendExternalLexStateForState(d.language, elsOrder, d.state)
	for _, st := range states {
		elsOrder = appendExternalLexStateForState(d.language, elsOrder, st)
	}
	if len(elsOrder) <= 1 {
		return Token{}, false
	}

	startPos := d.lexer.pos
	startRow := d.lexer.row
	startCol := d.lexer.col
	snapshot := d.captureExternalScannerStateInto(&d.externalSnapshot)

	bestFound := false
	bestELS := -1
	bestTok := Token{}
	bestEndPos := startPos
	bestEndRow := startRow
	bestEndCol := startCol
	bestSupport := -1
	bestOriginActions := -1
	bestSpecificity := -1
	bestPrimaryHasAction := false

	for _, elsID := range elsOrder {
		row := d.language.ExternalLexStates[elsID]
		d.restoreExternalScannerState(snapshot)

		el := &d.externalLexer
		el.reset(d.lexer.source, startPos, startRow, startCol)
		if !d.runExternalScannerWithRetry(el, row) {
			continue
		}
		tok, ok := el.token()
		if !ok {
			continue
		}

		support := 0
		originActions := 0
		primaryHasAction := d.lookupActionIndex(d.state, tok.Symbol) != 0
		for _, st := range states {
			idx := d.lookupActionIndex(st, tok.Symbol)
			if idx == 0 {
				continue
			}
			support++
			if int(st) < len(d.language.LexModes) && int(d.language.LexModes[st].ExternalLexState) == elsID &&
				int(idx) < len(d.language.ParseActions) {
				if n := len(d.language.ParseActions[idx].Actions); n > originActions {
					originActions = n
				}
			}
		}
		if support == 0 {
			continue
		}

		specificity := tokenSymbolSpecificity(d.language, tok.Symbol)
		better := !bestFound ||
			support > bestSupport ||
			(support == bestSupport && primaryHasAction && !bestPrimaryHasAction) ||
			(support == bestSupport && primaryHasAction == bestPrimaryHasAction && originActions > bestOriginActions) ||
			(support == bestSupport && primaryHasAction == bestPrimaryHasAction && originActions == bestOriginActions &&
				primaryELS == elsID && primaryELS != bestELS) ||
			(support == bestSupport && primaryHasAction == bestPrimaryHasAction && originActions == bestOriginActions &&
				primaryELS == bestELS && specificity > bestSpecificity) ||
			(support == bestSupport && primaryHasAction == bestPrimaryHasAction && originActions == bestOriginActions &&
				primaryELS == bestELS && specificity == bestSpecificity && tok.StartByte < bestTok.StartByte) ||
			(support == bestSupport && primaryHasAction == bestPrimaryHasAction && originActions == bestOriginActions &&
				primaryELS == bestELS && specificity == bestSpecificity && tok.StartByte == bestTok.StartByte && tok.EndByte > bestTok.EndByte) ||
			(support == bestSupport && primaryHasAction == bestPrimaryHasAction && originActions == bestOriginActions &&
				primaryELS == bestELS && specificity == bestSpecificity && tok.StartByte == bestTok.StartByte &&
				tok.EndByte == bestTok.EndByte &&
				(int(tok.EndByte) > bestEndPos || tok.EndPoint.Row > bestEndRow || (tok.EndPoint.Row == bestEndRow && tok.EndPoint.Column > bestEndCol)))
		if !better {
			continue
		}

		bestFound = true
		bestELS = elsID
		bestTok = tok
		bestEndPos = int(tok.EndByte)
		bestEndRow = tok.EndPoint.Row
		bestEndCol = tok.EndPoint.Column
		bestSupport = support
		bestOriginActions = originActions
		bestSpecificity = specificity
		bestPrimaryHasAction = primaryHasAction
	}

	d.restoreExternalScannerState(snapshot)
	if !bestFound {
		return Token{}, false
	}

	el := &d.externalLexer
	el.reset(d.lexer.source, startPos, startRow, startCol)
	if !d.runExternalScannerWithRetry(el, d.language.ExternalLexStates[bestELS]) {
		d.restoreExternalScannerState(snapshot)
		return Token{}, false
	}
	tok, ok := el.token()
	if !ok {
		d.restoreExternalScannerState(snapshot)
		return Token{}, false
	}

	d.trackZeroWidthExternalToken(tok)
	d.lexer.pos = int(tok.EndByte)
	d.lexer.row = tok.EndPoint.Row
	d.lexer.col = tok.EndPoint.Column
	return tok, true
}

func appendExternalLexStateForState(lang *Language, order []int, st StateID) []int {
	if lang == nil || int(st) >= len(lang.LexModes) {
		return order
	}
	elsID := int(lang.LexModes[st].ExternalLexState)
	if elsID < 0 || elsID >= len(lang.ExternalLexStates) {
		return order
	}
	for _, existing := range order {
		if existing == elsID {
			return order
		}
	}
	return append(order, elsID)
}

func tokenSymbolSpecificity(lang *Language, sym Symbol) int {
	if lang == nil || int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
		return 0
	}
	name := lang.SymbolNames[sym]
	switch name {
	case "", "word", "identifier", "_special_character", "string_content":
		return 0
	}
	if name[0] == '_' {
		return 1
	}
	if len(name) == 1 {
		return 3
	}
	return 2
}

func (d *dfaTokenSource) debugExternalLexStateIDs(states []StateID) string {
	if d == nil || d.language == nil || len(d.language.ExternalLexStates) == 0 {
		return ""
	}
	ids := make([]string, 0, len(states))
	seen := map[uint16]struct{}{}
	for _, st := range states {
		if int(st) >= len(d.language.LexModes) {
			continue
		}
		id := d.language.LexModes[st].ExternalLexState
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, fmt.Sprintf("%d", id))
	}
	return strings.Join(ids, ",")
}

func (d *dfaTokenSource) debugExternalValidNames(valid []bool) string {
	if d == nil || d.language == nil {
		return ""
	}
	names := make([]string, 0, len(valid))
	for i, ok := range valid {
		if !ok {
			continue
		}
		name := ""
		if i >= 0 && i < len(d.language.ExternalSymbols) {
			sym := d.language.ExternalSymbols[i]
			if int(sym) >= 0 && int(sym) < len(d.language.SymbolNames) {
				name = d.language.SymbolNames[sym]
			}
		}
		names = append(names, fmt.Sprintf("%d:%s", i, name))
	}
	return strings.Join(names, ",")
}

func (d *dfaTokenSource) runExternalScannerWithRetry(el *ExternalLexer, valid []bool) bool {
	if d == nil || d.language == nil || d.language.ExternalScanner == nil || el == nil {
		return false
	}
	snapshot := d.captureExternalScannerStateInto(&d.externalRetrySnap)
	if RunExternalScanner(d.language, d.externalPayload, el, valid) {
		return true
	}
	if !el.hasResult {
		d.restoreExternalScannerState(snapshot)
		return false
	}
	// Reuse maskedScratch to avoid a per-retry heap allocation.
	if cap(d.maskedScratch) < len(valid) {
		d.maskedScratch = make([]bool, len(valid))
	} else {
		d.maskedScratch = d.maskedScratch[:len(valid)]
	}
	copy(d.maskedScratch, valid)
	masked := d.maskedScratch
	for {
		idx := d.externalSymbolIndex(el.resultSymbol)
		if idx < 0 || idx >= len(masked) || !masked[idx] {
			d.restoreExternalScannerState(snapshot)
			return false
		}
		masked[idx] = false
		anyValid := false
		for _, ok := range masked {
			if ok {
				anyValid = true
				break
			}
		}
		if !anyValid {
			d.restoreExternalScannerState(snapshot)
			return false
		}

		d.restoreExternalScannerState(snapshot)
		retryLexer := &d.externalRetryLexer
		retryLexer.reset(d.lexer.source, d.lexer.pos, d.lexer.row, d.lexer.col)
		if RunExternalScanner(d.language, d.externalPayload, retryLexer, masked) {
			*el = *retryLexer
			return true
		}
		if !retryLexer.hasResult {
			d.restoreExternalScannerState(snapshot)
			return false
		}
		*el = *retryLexer
	}
}

func (d *dfaTokenSource) captureExternalScannerStateInto(dst *[]byte) []byte {
	if d == nil || d.language == nil || d.language.ExternalScanner == nil {
		return nil
	}
	if dst == nil {
		return nil
	}
	if cap(*dst) < externalScannerSerializationBufferSize {
		*dst = make([]byte, externalScannerSerializationBufferSize)
	}
	buf := (*dst)[:externalScannerSerializationBufferSize]
	n := d.language.ExternalScanner.Serialize(d.externalPayload, buf)
	if n <= 0 {
		*dst = (*dst)[:0]
		return nil
	}
	*dst = buf[:n]
	return *dst
}

func (d *dfaTokenSource) restoreExternalScannerState(snapshot []byte) {
	if d == nil || d.language == nil || d.language.ExternalScanner == nil {
		return
	}
	d.language.ExternalScanner.Deserialize(d.externalPayload, snapshot)
}

func (d *dfaTokenSource) lastExternalScannerCheckpoint() (externalScannerCheckpoint, uint32, uint32, bool) {
	if d == nil || !d.lastExternalTokenValid {
		return externalScannerCheckpoint{}, 0, 0, false
	}
	return externalScannerCheckpoint{
		start: d.externalTokenStart,
		end:   d.externalTokenEnd,
	}, d.lastExternalTokenStartByte, d.lastExternalTokenEndByte, true
}

func (d *dfaTokenSource) externalScannerStateMatches(snapshot []byte) bool {
	if d == nil {
		return len(snapshot) == 0
	}
	current := d.captureExternalScannerStateInto(&d.externalCompare)
	return bytes.Equal(current, snapshot)
}

func (d *dfaTokenSource) externalSymbolIndex(sym Symbol) int {
	if d == nil || d.language == nil {
		return -1
	}
	for i, ext := range d.language.ExternalSymbols {
		if ext == sym {
			return i
		}
	}
	return -1
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
	if !d.language.keywordLexCouldMatch(d.lexer.source, start, end) {
		return tok
	}

	kw := Lexer{
		states:     d.language.KeywordLexStates,
		asciiTable: d.language.KeywordLexAsciiTable(),
		source:     d.lexer.source[start:end],
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
	if d.language.Name == "rust" && int(kwTok.Symbol) < len(d.language.SymbolNames) && d.language.SymbolNames[kwTok.Symbol] == "default" {
		if end < len(d.lexer.source) && d.lexer.source[end] == ':' {
			return tok
		}
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
		if !kwHasAction {
			if altSym, ok := d.activeLiteralKeywordSymbol(tok); ok {
				tok.Symbol = altSym
				return tok
			}
		}
		if !kwHasAction && idHasAction {
			return tok // no active stack needs the keyword
		}
		if d.shouldPreferJavaScriptTypeScriptContextualIdentifier(tok, kwTok, kwHasAction, idHasAction) {
			return tok
		}
	}

	tok.Symbol = kwTok.Symbol
	return tok
}

func (d *dfaTokenSource) activeLiteralKeywordSymbol(tok Token) (Symbol, bool) {
	if d == nil || d.language == nil || d.lookupActionIndex == nil || tok.Text == "" {
		return 0, false
	}
	candidates := d.language.TokenSymbolsByName(tok.Text)
	visit := func(state StateID) (Symbol, bool) {
		for _, sym := range candidates {
			if sym == 0 {
				continue
			}
			if d.lookupActionIndex(state, sym) != 0 {
				return sym, true
			}
		}
		if len(candidates) == 0 && d.language.TokenCount == 0 {
			for sym := Symbol(1); uint32(sym) < d.language.SymbolCount && int(sym) < len(d.language.SymbolNames); sym++ {
				if d.language.SymbolNames[sym] != tok.Text {
					continue
				}
				if d.lookupActionIndex(state, sym) != 0 {
					return sym, true
				}
			}
		}
		return 0, false
	}
	if sym, ok := visit(d.state); ok {
		return sym, true
	}
	for i, state := range d.glrStates {
		if state == d.state || d.priorGLRState(i, state) {
			continue
		}
		if sym, ok := visit(state); ok {
			return sym, true
		}
	}
	return 0, false
}

func (d *dfaTokenSource) priorGLRState(limit int, state StateID) bool {
	for i := 0; i < limit && i < len(d.glrStates); i++ {
		if d.glrStates[i] == state {
			return true
		}
	}
	return false
}

// parseIterations returns the iteration limit scaled to input size.
// A correctly-parsed file needs roughly (tokens * grammar_depth) iterations.
// For typical source (~5 bytes/token, ~10 reduce depth), that's sourceLen*2.
// We use sourceLen*20 as a generous upper bound that still prevents runaway
// parsing from OOMing the machine.
