package gotreesitter

import (
	"bytes"
	"fmt"
	"slices"
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
	// Cached parser recovery gate. Parser-owned token sources pass this from
	// Parser.errorCostCompetition so reset/reuse does not rescan grammar tables.
	cRecoveryEnabled bool

	lookupActionIndex           func(state StateID, sym Symbol) uint16
	lexModeStarts               []lexModeStart
	hasKeywordState             []bool
	externalValidByState        [][]uint16
	externalValidMaskByState    []uint64
	externalPayload             any
	externalValid               []bool
	externalSnapshot            []byte
	externalRetrySnap           []byte
	externalTokenStart          []byte
	externalTokenEnd            []byte
	externalCompare             []byte
	externalLexer               ExternalLexer
	externalRetryLexer          ExternalLexer
	externalLookaheadEndByte    uint32
	lastExternalTokenStartByte  uint32
	lastExternalTokenEndByte    uint32
	lastExternalTokenValid      bool
	lastExternalTokenWasExtra   bool
	externalTokenEndSameAsStart bool
	lastTokenStartByte          uint32
	lastTokenEndByte            uint32
	lastTokenValid              bool
	singleState                 [1]StateID
	glrStates                   []StateID // all active GLR stack states
	hasExternalScanner          bool
	hasExternalSymbols          bool
	usesExternalCheckpoints     bool
	zeroWidthSentinelSymbol     Symbol
	hasZeroWidthSentinelSymbol  bool
	isBash                      bool
	isBashGenerated             bool
	isComment                   bool
	isFortran                   bool
	isScheme                    bool
	isSwift                     bool
	hasZeroWidthTokens          bool
	hasZeroWidthStartAccept     bool

	// maskedScratch is a reusable buffer for runExternalScannerWithRetry,
	// avoiding a per-call heap allocation when masking already-tried symbols.
	maskedScratch []bool

	// sqlKeywordScratch is a reusable upper-case copy buffer for SQL keyword
	// promotion. tree-sitter-sql keywords are case-insensitive, while the
	// generated keyword DFA stores upper-case literals.
	sqlKeywordScratch []byte

	// glrUnionScanScratch retains one preferred DFA result per parser state
	// during a single GLR token election. Token text may alias lexer source, so
	// the inline and overflow storage are cleared before the token source can
	// return to its pool. Most elections fit inline; exceptional wider elections
	// retain their overflow capacity across parser-pool cycles.
	glrUnionScanInline  [8]glrUnionDFAScan
	glrUnionScanScratch []glrUnionDFAScan

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

	// ownedLexer is a private Lexer retained across pool cycles for
	// parser-internal construction sites (acquireDFATokenSourceReusingLexer),
	// so steady-state parses do not allocate a fresh Lexer per parse. It never
	// escapes the token source: Close zeroes its contents (dropping the source
	// reference) but keeps the allocation for the next pooled acquire. Callers
	// that pass their own lexer simply leave it unused.
	ownedLexer *Lexer

	// relexProbeLexer is a private Lexer.
	// It probes the deterministic finite automaton for one parser state.
	// Keep it outside the compact scheduler to limit each scheduler allocation.
	// Close drops its source reference.
	relexProbeLexer *Lexer
}

const maxConsecutiveZeroWidthTokens = 4
const maxConsecutiveZeroWidthTokensExternal = 128
const maxConsecutiveZeroWidthTokensRepeatableExternal = 4096
const noLookaheadLexState = ^uint32(0)
const externalScannerSerializationBufferSize = 4096

type tokenCandidate struct {
	Tok                Token
	Origin             StateID
	RouteMask          uint16
	EndPos             int
	EndRow             uint32
	EndCol             uint32
	ExternalCheckpoint *externalScannerCheckpoint
}

type tokenFrontier struct {
	StartByte  uint32
	StartPoint Point
	Candidates []tokenCandidate
}

type glrUnionDFAScan struct {
	state  StateID
	tok    Token
	endPos int
	endRow uint32
	endCol uint32
}

var dfaTokenSourcePool = sync.Pool{
	New: func() any {
		return &dfaTokenSource{
			extZeroPos:             -1,
			zeroWidthPos:           -1,
			bashArithmeticCachePos: -1,
		}
	},
}

// setLexerErrorRunLexState wires the grammar's most permissive lex mode
// (LexModes[0], the C ERROR_STATE mode) into the lexer so NextWithErrorRuns
// can mirror C's skipped-error lexing for truly unlexable runs.
func setLexerErrorRunLexState(l *Lexer, language *Language) {
	setLexerErrorRunLexStateEnabled(l, language, errorCostCompetitionLanguage(language))
}

func setLexerErrorRunLexStateEnabled(l *Lexer, language *Language, cRecoveryEnabled bool) {
	if l == nil {
		return
	}
	l.errorRunLexState = 0
	l.hasErrorRunLexState = false
	l.errorModeRetry = false
	if language == nil || len(language.LexModes) == 0 {
		return
	}
	// Faithful C error-recovery port (parser_recover_c.go): gated grammars get
	// C's complete ts_parser__lex failure behavior — error-mode retry first
	// (returning real, often invisible tokens that recovery absorbs as hidden
	// error-region leaves), then skipped-run errorSymbol tokens when even
	// LexModes[0] fails.
	if !cRecoveryEnabled {
		return
	}
	ls := language.LexModes[0].LexStateIndex()
	if ls == noLookaheadLexState {
		return
	}
	l.errorRunLexState = ls
	l.hasErrorRunLexState = true
	l.errorModeRetry = true
}

func initDFATokenSourceWithCRecovery(ts *dfaTokenSource, lexer *Lexer, language *Language, lookupActionIndex func(state StateID, sym Symbol) uint16, hasKeywordState []bool, externalValidByState [][]uint16, externalValidMaskByState []uint64, cRecoveryEnabled bool) {
	ts.lexer = lexer
	ts.language = language
	ts.state = 0
	ts.cRecoveryEnabled = cRecoveryEnabled
	ts.lookupActionIndex = lookupActionIndex
	ts.lexModeStarts = nil
	ts.hasKeywordState = hasKeywordState
	ts.externalValidByState = externalValidByState
	ts.externalValidMaskByState = externalValidMaskByState
	if lexer != nil && language != nil {
		ts.lexer.states = language.LexStates
		ts.lexer.immediateTokens = language.ImmediateTokens
		ts.lexer.zeroWidthTokens = language.ZeroWidthTokens
		ts.lexer.asciiTable = language.LexAsciiTable()
		ts.lexModeStarts = language.LexModeStarts()
		setLexerErrorRunLexStateEnabled(ts.lexer, language, cRecoveryEnabled)
	}
	if language != nil {
		zeroWidthInfo := languageZeroWidthInfoFor(language)
		ts.zeroWidthSentinelSymbol = zeroWidthInfo.sentinelSymbol
		ts.hasZeroWidthSentinelSymbol = zeroWidthInfo.hasZeroWidthSentinel
		ts.hasExternalScanner = language.ExternalScanner != nil
		ts.hasExternalSymbols = len(language.ExternalSymbols) > 0
		ts.usesExternalCheckpoints = languageUsesExternalScannerCheckpoints(language)
		ts.isBash = language.Name == "bash"
		ts.isBashGenerated = ts.isBash && language.GeneratedByGrammargen
		ts.isComment = language.Name == "comment"
		ts.isFortran = language.Name == "fortran"
		ts.isScheme = language.Name == "scheme"
		ts.isSwift = language.Name == "swift"
		ts.hasZeroWidthTokens = zeroWidthInfo.hasTokens
		ts.hasZeroWidthStartAccept = zeroWidthInfo.hasStartAccept
	}
	if ts.hasExternalScanner {
		ts.externalPayload = language.ExternalScanner.Create()
	}
}

func acquireDFATokenSource(lexer *Lexer, language *Language, lookupActionIndex func(state StateID, sym Symbol) uint16, hasKeywordState []bool, externalValidByState [][]uint16, externalValidMaskByState []uint64) *dfaTokenSource {
	return acquireDFATokenSourceWithCRecovery(lexer, language, lookupActionIndex, hasKeywordState, externalValidByState, externalValidMaskByState, errorCostCompetitionLanguage(language))
}

func acquireDFATokenSourceWithCRecovery(lexer *Lexer, language *Language, lookupActionIndex func(state StateID, sym Symbol) uint16, hasKeywordState []bool, externalValidByState [][]uint16, externalValidMaskByState []uint64, cRecoveryEnabled bool) *dfaTokenSource {
	ts := dfaTokenSourcePool.Get().(*dfaTokenSource)
	resetPooledDFATokenSource(ts)
	initDFATokenSourceWithCRecovery(ts, lexer, language, lookupActionIndex, hasKeywordState, externalValidByState, externalValidMaskByState, cRecoveryEnabled)
	return ts
}

// acquireDFATokenSourceReusingLexer is acquireDFATokenSourceWithCRecovery for
// parser-internal construction sites that would otherwise allocate a fresh
// Lexer per parse: it reuses the pooled token source's retained ownedLexer
// (creating it on first use) and re-initializes it in place with exactly the
// state NewLexer would have set. Steady-state acquires therefore allocate
// neither the token source nor the lexer. The caller must Close() the
// returned source; Close zeroes the retained lexer and returns both to the
// pool.
func acquireDFATokenSourceReusingLexer(source []byte, language *Language, lookupActionIndex func(state StateID, sym Symbol) uint16, hasKeywordState []bool, externalValidByState [][]uint16, externalValidMaskByState []uint64, cRecoveryEnabled bool) *dfaTokenSource {
	ts := dfaTokenSourcePool.Get().(*dfaTokenSource)
	resetPooledDFATokenSource(ts)
	lexer := ts.ownedLexer
	if lexer == nil {
		lexer = &Lexer{}
		ts.ownedLexer = lexer
	}
	*lexer = Lexer{states: language.LexStates, source: source}
	initDFATokenSourceWithCRecovery(ts, lexer, language, lookupActionIndex, hasKeywordState, externalValidByState, externalValidMaskByState, cRecoveryEnabled)
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
	savedSQLKeywordScratch := ts.sqlKeywordScratch[:0]
	savedExtZeroTried := ts.extZeroTried[:0]
	savedOwnedLexer := ts.ownedLexer
	savedRelexProbeLexer := ts.relexProbeLexer
	var savedGLRUnionScanScratch []glrUnionDFAScan
	if cap(ts.glrUnionScanScratch) > len(ts.glrUnionScanInline) {
		// Close clears every Token before the source enters the pool. Preserve
		// only separately allocated overflow storage; the inline array belongs
		// to the struct that is about to be replaced.
		savedGLRUnionScanScratch = ts.glrUnionScanScratch[:0]
	}
	*ts = dfaTokenSource{
		extZeroPos:             -1,
		zeroWidthPos:           -1,
		bashArithmeticCachePos: -1,
	}
	ts.ownedLexer = savedOwnedLexer
	ts.relexProbeLexer = savedRelexProbeLexer
	ts.externalValid = savedExternalValid
	ts.externalTokenStart = savedExternalTokenStart
	ts.externalTokenEnd = savedExternalTokenEnd
	ts.externalSnapshot = savedExternalSnapshot
	ts.externalRetrySnap = savedExternalRetrySnap
	ts.externalCompare = savedExternalCompare
	ts.maskedScratch = savedMasked
	ts.sqlKeywordScratch = savedSQLKeywordScratch
	ts.extZeroTried = savedExtZeroTried
	if savedGLRUnionScanScratch != nil {
		ts.glrUnionScanScratch = savedGLRUnionScanScratch
	} else {
		ts.glrUnionScanScratch = ts.glrUnionScanInline[:0]
	}
}

func newDFATokenSourceDirect(lexer *Lexer, language *Language, lookupActionIndex func(state StateID, sym Symbol) uint16, hasKeywordState []bool, externalValidByState [][]uint16, externalValidMaskByState []uint64) *dfaTokenSource {
	return newDFATokenSourceDirectWithCRecovery(lexer, language, lookupActionIndex, hasKeywordState, externalValidByState, externalValidMaskByState, errorCostCompetitionLanguage(language))
}

func newDFATokenSourceDirectWithCRecovery(lexer *Lexer, language *Language, lookupActionIndex func(state StateID, sym Symbol) uint16, hasKeywordState []bool, externalValidByState [][]uint16, externalValidMaskByState []uint64, cRecoveryEnabled bool) *dfaTokenSource {
	ts := &dfaTokenSource{
		extZeroPos:             -1,
		zeroWidthPos:           -1,
		bashArithmeticCachePos: -1,
		noPool:                 true,
	}
	initDFATokenSourceWithCRecovery(ts, lexer, language, lookupActionIndex, hasKeywordState, externalValidByState, externalValidMaskByState, cRecoveryEnabled)
	return ts
}

func languageZeroWidthInfoFor(lang *Language) languageZeroWidthInfo {
	if lang == nil {
		return languageZeroWidthInfo{}
	}
	lang.zeroWidthInfoOnce.Do(func() {
		lang.zeroWidthInfo = buildLanguageZeroWidthInfo(lang)
	})
	return lang.zeroWidthInfo
}

func buildLanguageZeroWidthInfo(lang *Language) languageZeroWidthInfo {
	if lang == nil {
		return languageZeroWidthInfo{}
	}
	info := languageZeroWidthInfo{}
	for _, ok := range lang.ZeroWidthTokens {
		if ok {
			info.hasTokens = true
			break
		}
	}
	if len(lang.ZeroWidthTokens) > 0 {
		for _, state := range lang.LexStates {
			sym := int(state.AcceptToken)
			if sym >= 0 && sym < len(lang.ZeroWidthTokens) && lang.ZeroWidthTokens[sym] {
				info.hasStartAccept = true
				break
			}
		}
	}
	if lang.GeneratedByGrammargen {
		limit := int(lang.TokenCount)
		if limit > len(lang.SymbolNames) {
			limit = len(lang.SymbolNames)
		}
		for i := 1; i < limit; i++ {
			if lang.SymbolNames[i] == "\x00" {
				info.sentinelSymbol = Symbol(i)
				info.hasZeroWidthSentinel = true
				break
			}
		}
	}
	info.zeroWidthSentinelKnown = true
	return info
}

func (d *dfaTokenSource) Reset(source []byte) {
	if d == nil {
		return
	}
	d.clearGLRUnionScanCache()
	if d.lexer == nil {
		d.lexer = NewLexer(nil, source)
	}
	d.lexer.source = source
	d.lexer.pos = 0
	d.lexer.row = 0
	d.lexer.col = 0
	d.lexer.includedRangeIdx = 0
	d.lexer.normalizeIncludedPosition()
	if d.language != nil {
		d.lexer.states = d.language.LexStates
		d.lexer.immediateTokens = d.language.ImmediateTokens
		d.lexer.zeroWidthTokens = d.language.ZeroWidthTokens
		d.lexer.asciiTable = d.language.LexAsciiTable()
		setLexerErrorRunLexStateEnabled(d.lexer, d.language, d.cRecoveryEnabled)
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
	d.externalLookaheadEndByte = 0
	d.lastExternalTokenWasExtra = false
	d.externalTokenEndSameAsStart = false
	d.lastTokenStartByte = 0
	d.lastTokenEndByte = 0
	d.lastTokenValid = false
	if d.language == nil || d.language.ExternalScanner == nil {
		return
	}
	if d.externalPayload != nil {
		d.language.ExternalScanner.Destroy(d.externalPayload)
	}
	d.externalPayload = d.language.ExternalScanner.Create()
}

func (d *dfaTokenSource) setIncludedRanges(ranges []Range) bool {
	if d == nil || d.lexer == nil || d.hasExternalScanner ||
		(d.language != nil && (d.language.ExternalScanner != nil || len(d.language.ExternalSymbols) != 0)) {
		return false
	}
	d.lexer.setIncludedRanges(ranges)
	return true
}

func (d *dfaTokenSource) Close() {
	d.clearGLRUnionScanCache()
	if d.language != nil && d.language.ExternalScanner != nil && d.externalPayload != nil {
		d.language.ExternalScanner.Destroy(d.externalPayload)
		d.externalPayload = nil
	}
	if d.ownedLexer != nil {
		// Keep the allocation for the next pooled acquire, but zero the
		// contents so no source bytes or table slices stay pinned while the
		// token source sits in the pool.
		*d.ownedLexer = Lexer{}
	}
	if d.relexProbeLexer != nil {
		*d.relexProbeLexer = Lexer{}
	}
	d.lexer = nil
	d.language = nil
	d.lookupActionIndex = nil
	d.hasKeywordState = nil
	d.externalValidByState = nil
	d.externalValidMaskByState = nil
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
	d.externalLookaheadEndByte = 0
	d.lastExternalTokenWasExtra = false
	d.externalTokenEndSameAsStart = false
	d.lastTokenStartByte = 0
	d.lastTokenEndByte = 0
	d.lastTokenValid = false
	if !d.noPool {
		dfaTokenSourcePool.Put(d)
	}
}

// DebugDFA enables trace logging for DFA token production.
//
// Use `DebugDFA.Store(true/false)` to toggle at runtime.
var DebugDFA atomic.Bool

func (d *dfaTokenSource) Next() Token {
	if d != nil {
		// A token-source read mirrors one C ts_parser__lex call. Preserve the
		// maximum frontier only across attempts within this read.
		d.externalLookaheadEndByte = 0
	}
	if d != nil && d.lexer != nil {
		d.lexer.skipLeadingBOM()
	}
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
		var glrExternalStartSnapshot []byte
		keepGLRExternalStartSnapshot := false
		if d.hasExternalScanner && len(d.glrStates) > 1 {
			glrExternalStartSnapshot = d.captureExternalScannerStateInto(&d.externalCompare)
			keepGLRExternalStartSnapshot = true
		}
		if d.shouldForceEOFLookahead() {
			tok := d.syntheticEOFLookaheadToken()
			tok = d.attachTokenLookaheadFrontier(tok, true)
			d.lastTokenValid = false
			d.lastExternalTokenValid = false
			d.lastExternalTokenWasExtra = false
			d.externalTokenEndSameAsStart = false
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
				extEndPos := d.lexer.pos
				extEndRow := d.lexer.row
				extEndCol := d.lexer.col
				if dfaTok, dfaEndPos, dfaEndRow, dfaEndCol, preferDFA :=
					d.preferGLRUnionDFAOverExternalToken(extTok, extEndPos, extEndRow, extEndCol, scanStartPos, scanStartRow, scanStartCol); preferDFA {
					if keepGLRExternalStartSnapshot {
						d.restoreExternalScannerState(glrExternalStartSnapshot)
					}
					tok = dfaTok
					tokenFromExternal = false
					d.lexer.pos = dfaEndPos
					d.lexer.row = dfaEndRow
					d.lexer.col = dfaEndCol
				} else {
					d.lexer.pos = extEndPos
					d.lexer.row = extEndRow
					d.lexer.col = extEndCol
				}
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
		// loop forever at the same byte. External scanners already have a
		// same-position tried-symbol mask; prefer masking and retrying before
		// falling back to byte skipping so ordinary DFA extras at the same byte
		// are not damaged.
		if tok.Symbol != 0 && tok.EndByte <= tok.StartByte && !d.hasAnyActionForSymbol(tok.Symbol) {
			if tokenFromExternal && d.canRetryAfterUnusableZeroWidthExternal(tok) {
				if DebugDFA.Load() {
					fmt.Printf("  ZERO-WIDTH external retry sym=%d at pos=%d state=%d\n", tok.Symbol, d.lexer.pos, d.state)
				}
				continue
			}
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
		tok = d.attachTokenLookaheadFrontier(tok, !tokenFromExternal && !tok.lexerInternalDFALexed)

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
		if tok.Symbol != 0 && !tok.NoLookahead {
			d.lastTokenStartByte = tok.StartByte
			d.lastTokenEndByte = tok.EndByte
			d.lastTokenValid = true
		} else {
			d.lastTokenValid = false
		}
		if d.usesExternalCheckpoints && tok.Symbol != 0 && !tok.NoLookahead {
			if tokenFromExternal {
				d.captureExternalScannerStateInto(&d.externalTokenEnd)
				d.externalTokenEndSameAsStart = false
			} else if d.externalScannerRetainsStateOnScanFailure() {
				d.captureExternalScannerStateInto(&d.externalTokenEnd)
				d.externalTokenEndSameAsStart = bytes.Equal(d.externalTokenStart, d.externalTokenEnd)
			} else {
				d.externalTokenEnd = d.externalTokenEnd[:0]
				d.externalTokenEndSameAsStart = true
			}
			d.lastExternalTokenStartByte = tok.StartByte
			d.lastExternalTokenEndByte = tok.EndByte
			d.lastExternalTokenValid = true
			d.lastExternalTokenWasExtra = tokenFromExternal &&
				sourcePositionFollowsWhitespace(d.lexer.source, int(tok.EndByte)) &&
				d.tokenIsExtraInAllActiveStates(tok.Symbol)
			// Keep start/end snapshots in the token source until the parser
			// either records them on a shifted leaf or advances to the next token.
			if len(externalStartSnapshot) == 0 {
				d.externalTokenStart = d.externalTokenStart[:0]
			}
			if tokenFromExternal && len(d.externalTokenEnd) == 0 {
				d.externalTokenEnd = d.externalTokenEnd[:0]
			}
		} else {
			d.lastExternalTokenValid = false
			d.lastExternalTokenWasExtra = false
			d.externalTokenEndSameAsStart = false
		}
		// Record provenance only when this source selected the error lex mode.
		// A checkpointless external scanner cannot prove the complete lex path.
		tok.lexerErrorModeLexed = d.cRecoveryEnabled && d.state == cErrorState && !tokenFromExternal &&
			(!d.hasExternalScanner || d.usesExternalCheckpoints)
		return tok
	}
}

// attachTokenLookaheadFrontier preserves the largest external-scanner
// frontier observed during this token-source read. Synthetic replacements do
// not have a scanner token, so derive their frontier from their resulting end.
func (d *dfaTokenSource) attachTokenLookaheadFrontier(tok Token, synthetic bool) Token {
	if d == nil {
		return tok
	}
	frontier := maxUint32(tok.lexerLookaheadEndByte, d.externalLookaheadEndByte)
	if synthetic && d.lexer != nil {
		endPos := uint64(tok.EndByte)
		if endPos > uint64(len(d.lexer.source)) {
			endPos = uint64(len(d.lexer.source))
		}
		frontier = maxUint32(frontier, d.lexer.lookaheadEndByteAt(int(endPos), true))
	}
	if frontier < tok.EndByte {
		frontier = tok.EndByte
	}
	tok.lexerLookaheadEndByte = frontier
	return tok
}

func (d *dfaTokenSource) SetParserState(state StateID) {
	d.state = state
}

// lexesErrorModeAtErrorState reports that this source lexes SetParserState(0)
// tokens with the grammar's ERROR-state mode (LexModes[0]) when the faithful
// C recovery port is active — C-equivalent error-mode lookaheads.
func (d *dfaTokenSource) lexesErrorModeAtErrorState() bool {
	return d != nil && d.cRecoveryEnabled
}

func (d *dfaTokenSource) SetGLRStates(states []StateID) {
	d.glrStates = states
}

func (d *dfaTokenSource) setExternalScannerCheckpointsEnabled(enabled bool) {
	if d == nil {
		return
	}
	d.usesExternalCheckpoints = enabled && languageUsesExternalScannerCheckpoints(d.language)
	if d.usesExternalCheckpoints {
		return
	}
	d.lastExternalTokenValid = false
	d.lastExternalTokenWasExtra = false
	d.externalTokenEndSameAsStart = false
	d.lastTokenStartByte = 0
	d.lastTokenEndByte = 0
	d.lastTokenValid = false
	d.externalTokenStart = d.externalTokenStart[:0]
	d.externalTokenEnd = d.externalTokenEnd[:0]
}

func (d *dfaTokenSource) nextDFAToken() Token {
	if d == nil || d.lexer == nil || d.language == nil {
		return Token{}
	}
	tok, endPos, endRow, endCol := d.scanPreferredTokenForState(d.state)
	d.lexer.pos = endPos
	d.lexer.row = endRow
	d.lexer.col = endCol
	d.lexer.includedRangeIdx = d.lexer.includedRangeIndexForPosition(endPos)
	return tok
}

func (d *dfaTokenSource) preferGLRUnionDFAOverExternalToken(extTok Token, extEndPos int, extEndRow, extEndCol uint32, startPos int, startRow, startCol uint32) (Token, int, uint32, uint32, bool) {
	if d == nil || d.lexer == nil || d.language == nil || d.lookupActionIndex == nil {
		return Token{}, 0, 0, 0, false
	}
	if len(d.glrStates) <= 1 || extTok.Symbol == 0 || extTok.StartByte != uint32(startPos) {
		return Token{}, 0, 0, 0, false
	}
	extSupport := d.countGLRActionSupport(extTok.Symbol)
	if extSupport <= 0 {
		return Token{}, 0, 0, 0, false
	}

	d.lexer.pos = startPos
	d.lexer.row = startRow
	d.lexer.col = startCol
	dfaTok, ok := d.nextGLRUnionDFAToken()
	dfaEndPos := d.lexer.pos
	dfaEndRow := d.lexer.row
	dfaEndCol := d.lexer.col
	if !ok || dfaTok.Symbol == 0 || dfaTok.StartByte != extTok.StartByte {
		if DebugDFA.Load() && ok && dfaTok.Symbol != 0 {
			fmt.Printf("  GLR ext/dfa keep external: dfa start mismatch ext=%s(%d)[%d-%d] dfa=%s(%d)[%d-%d]\n",
				d.symbolName(extTok.Symbol), extTok.Symbol, extTok.StartByte, extTok.EndByte,
				d.symbolName(dfaTok.Symbol), dfaTok.Symbol, dfaTok.StartByte, dfaTok.EndByte)
		}
		return Token{}, 0, 0, 0, false
	}
	dfaSupport := d.countGLRActionSupport(dfaTok.Symbol)
	dfaSpecificity := tokenSymbolSpecificity(d.language, dfaTok.Symbol)
	extSpecificity := tokenSymbolSpecificity(d.language, extTok.Symbol)
	if dfaSupport < extSupport {
		if dfaSpecificity <= extSpecificity || !d.hasGLRActionSupportForBoth(dfaTok.Symbol, extTok.Symbol) {
			if DebugDFA.Load() {
				fmt.Printf("  GLR ext/dfa keep external: ext=%s(%d) support=%d specificity=%d dfa=%s(%d) support=%d specificity=%d\n",
					d.symbolName(extTok.Symbol), extTok.Symbol, extSupport, extSpecificity,
					d.symbolName(dfaTok.Symbol), dfaTok.Symbol, dfaSupport, dfaSpecificity)
			}
			return Token{}, 0, 0, 0, false
		}
	}
	if dfaSupport == extSupport {
		hasSpecificBranch := d.hasGLRActionSupportExclusiveTo(dfaTok.Symbol, extTok.Symbol) ||
			d.hasGLRActionSupportForBoth(dfaTok.Symbol, extTok.Symbol)
		if dfaSpecificity <= extSpecificity || !hasSpecificBranch {
			if DebugDFA.Load() {
				fmt.Printf("  GLR ext/dfa keep external: ext=%s(%d) support=%d specificity=%d dfa=%s(%d) support=%d specificity=%d\n",
					d.symbolName(extTok.Symbol), extTok.Symbol, extSupport, extSpecificity,
					d.symbolName(dfaTok.Symbol), dfaTok.Symbol, dfaSupport, dfaSpecificity)
			}
			return Token{}, 0, 0, 0, false
		}
	}

	if DebugDFA.Load() {
		fmt.Printf("  GLR ext/dfa choose dfa: ext=%s(%d)[%d-%d] end=%d:%d:%d support=%d dfa=%s(%d)[%d-%d] end=%d:%d:%d support=%d\n",
			d.symbolName(extTok.Symbol), extTok.Symbol, extTok.StartByte, extTok.EndByte, extEndPos, extEndRow, extEndCol, extSupport,
			d.symbolName(dfaTok.Symbol), dfaTok.Symbol, dfaTok.StartByte, dfaTok.EndByte, dfaEndPos, dfaEndRow, dfaEndCol, dfaSupport)
	}
	return dfaTok, dfaEndPos, dfaEndRow, dfaEndCol, true
}

func (d *dfaTokenSource) countGLRActionSupport(sym Symbol) int {
	if d == nil || d.lookupActionIndex == nil || sym == 0 {
		return 0
	}
	if len(d.glrStates) == 0 {
		if d.lookupActionIndex(d.state, sym) != 0 {
			return 1
		}
		return 0
	}
	support := 0
	for _, st := range d.glrStates {
		if d.lookupActionIndex(st, sym) != 0 {
			support++
		}
	}
	return support
}

func (d *dfaTokenSource) hasGLRActionSupportExclusiveTo(cand, other Symbol) bool {
	if d == nil || d.lookupActionIndex == nil || cand == 0 {
		return false
	}
	states := d.glrStates
	if len(states) == 0 {
		d.singleState[0] = d.state
		states = d.singleState[:]
	}
	for _, st := range states {
		if d.lookupActionIndex(st, cand) != 0 && d.lookupActionIndex(st, other) == 0 {
			return true
		}
	}
	return false
}

func (d *dfaTokenSource) hasGLRActionSupportForBoth(a, b Symbol) bool {
	if d == nil || d.lookupActionIndex == nil || a == 0 || b == 0 {
		return false
	}
	states := d.glrStates
	if len(states) == 0 {
		d.singleState[0] = d.state
		states = d.singleState[:]
	}
	for _, st := range states {
		if d.lookupActionIndex(st, a) != 0 && d.lookupActionIndex(st, b) != 0 {
			return true
		}
	}
	return false
}

// schemeIsErrorRunBoundary reports whether r terminates an error-recovery run
// in tree-sitter-scheme. The run that C wraps into an ERROR node stops at
// whitespace and the structural delimiters that begin their own datum
// ( "(" ")" string/quote/quasiquote/unquote and comments ). All other bytes —
// including "[" "]" "{" "}" "|" "#" and "\" — are consumed into the run.
func schemeIsErrorRunBoundary(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f', '\v',
		'(', ')', '"', '\'', '`', ',', ';':
		return true
	}
	return unicode.IsSpace(r)
}

// schemeErrorRunToken detects bytes the DFA silently skipped while lexing the
// token tok (a character with no valid token start). When such a skip is
// found, it returns an errorSymbol token spanning the unlexable run starting at
// iterStartPos, matching tree-sitter C's behavior of consuming the run into an
// ERROR node. The run extends from iterStartPos to the next boundary character
// (see schemeIsErrorRunBoundary), which mirrors how C's error recovery absorbs
// any otherwise-lexable trailing token (e.g. "make-accessors" in
// "\#make-accessors") up to the next delimiter.
func (d *dfaTokenSource) schemeErrorRunToken(iterStartPos int, iterStartRow, iterStartCol uint32, tok Token) (Token, bool) {
	if d == nil || d.lexer == nil {
		return Token{}, false
	}
	src := d.lexer.source
	if iterStartPos < 0 || iterStartPos >= len(src) {
		return Token{}, false
	}
	// A silent skip happened iff the lexer consumed bytes at iterStartPos
	// without emitting a token starting there: either the produced token starts
	// later than iterStartPos, or it is EOF/no-token while bytes remain.
	skipped := false
	if tok.Symbol == 0 {
		// EOF or no accepting state at all while input remains.
		skipped = true
	} else if tok.Symbol == errorSymbol {
		// The lexer now surfaces unlexable runs as errorSymbol tokens
		// (NextWithErrorRuns); scheme still re-derives the run end with its
		// own boundary rule, which absorbs lexable tails up to a delimiter.
		skipped = true
	} else if int(tok.StartByte) > iterStartPos {
		skipped = true
	}
	if !skipped {
		return Token{}, false
	}
	// The first byte at iterStartPos must itself be a non-boundary,
	// non-whitespace character that the DFA could not begin a token with.
	// Boundary characters here would have been lexed normally, so a skip over
	// one indicates a different code path we should not touch.
	firstRune, _ := utf8.DecodeRune(src[iterStartPos:])
	if schemeIsErrorRunBoundary(firstRune) {
		return Token{}, false
	}

	pos := iterStartPos
	row := iterStartRow
	col := iterStartCol
	for pos < len(src) {
		r, size := utf8.DecodeRune(src[pos:])
		if schemeIsErrorRunBoundary(r) {
			break
		}
		pos += size
		col += uint32(size)
	}
	if pos <= iterStartPos {
		return Token{}, false
	}
	return Token{
		Symbol:     errorSymbol,
		Text:       bytesToStringNoCopy(src[iterStartPos:pos]),
		StartByte:  uint32(iterStartPos),
		EndByte:    uint32(pos),
		StartPoint: Point{Row: iterStartRow, Column: iterStartCol},
		EndPoint:   Point{Row: row, Column: col},
	}, true
}

func (d *dfaTokenSource) shouldForceEOFLookahead() bool {
	if d == nil || d.language == nil {
		return false
	}
	if d.lexStateForState(d.state) == noLookaheadLexState {
		return true
	}
	for _, st := range d.glrStates {
		if st != d.state && d.lexStateForState(st) == noLookaheadLexState {
			return true
		}
	}
	return false
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
	if !d.cRecoveryEnabled {
		return d.lexer.Next(lexState)
	}
	return d.lexer.NextWithErrorRuns(lexState)
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

func (d *dfaTokenSource) SeekTokenFrontier(pos uint32, pt Point) {
	if d == nil || d.lexer == nil {
		return
	}
	d.lexer.pos = int(pos)
	d.lexer.row = pt.Row
	d.lexer.col = pt.Column
	d.lexer.includedRangeIdx = 0
	d.lexer.normalizeIncludedPosition()
	d.externalLookaheadEndByte = 0
}

// tokensSameLex compares full tokenization state, including the dependency
// frontier, not the lex path that produced the token. isKeyword records the
// promotion path, so it must not take part in a same-tokenization test.
func tokensSameLex(a, b Token) bool {
	a.isKeyword, b.isKeyword = false, false
	return a == b
}

// tokensSameLexForGLRCandidate compares the token surface used by GLR
// candidate routing. The lexer frontier is dependency metadata, not lexical
// identity, so callers must merge it separately when candidates match.
func tokensSameLexForGLRCandidate(a, b Token) bool {
	a.lexerLookaheadEndByte = 0
	b.lexerLookaheadEndByte = 0
	return tokensSameLex(a, b)
}

func mergeTokenLookaheadFrontier(dst *Token, src Token) {
	if dst == nil {
		return
	}
	dst.lexerLookaheadEndByte = maxUint32(dst.lexerLookaheadEndByte, src.lexerLookaheadEndByte)
}

func (d *dfaTokenSource) PeekTokenFrontier(states []StateID, dst []tokenCandidate) (tokenFrontier, bool) {
	dst = dst[:0]
	if d == nil || d.lexer == nil || d.language == nil || d.lookupActionIndex == nil {
		return tokenFrontier{}, false
	}
	activeStates := states
	if len(activeStates) == 0 {
		activeStates = d.glrStates
	}
	if len(activeStates) <= 1 {
		return tokenFrontier{}, false
	}

	lexModes := d.lexModeStartRows()
	primaryState := d.state
	if len(states) > 0 {
		primaryState = states[0]
	}
	if int(primaryState) >= len(lexModes) {
		return tokenFrontier{}, false
	}
	primaryMode := lexModes[primaryState]
	allSame := true
	for _, st := range activeStates {
		if int(st) >= len(lexModes) {
			allSame = false
			break
		}
		if lexModes[st] != primaryMode {
			allSame = false
			break
		}
	}
	if allSame {
		return tokenFrontier{}, false
	}

	startPos := d.lexer.pos
	startRow := d.lexer.row
	startCol := d.lexer.col
	startRangeIdx := d.lexer.includedRangeIdx
	defer func() {
		d.lexer.pos = startPos
		d.lexer.row = startRow
		d.lexer.col = startCol
		d.lexer.includedRangeIdx = startRangeIdx
	}()

	type lexModeKey struct {
		lexState                uint32
		afterWhitespaceLexState uint32
	}

	var seenBuf [32]lexModeKey
	seen := seenBuf[:0]
	for _, st := range activeStates {
		if int(st) >= len(lexModes) {
			continue
		}
		mode := lexModes[st]
		key := lexModeKey(mode)
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
		routeMask := d.tokenFrontierRouteMask(activeStates, candTok, candEndPos, candEndRow, candEndCol)
		if routeMask == 0 {
			continue
		}

		merged := false
		for i := range dst {
			if tokensSameLexForGLRCandidate(dst[i].Tok, candTok) && dst[i].EndPos == candEndPos && dst[i].EndRow == candEndRow && dst[i].EndCol == candEndCol {
				mergeTokenLookaheadFrontier(&dst[i].Tok, candTok)
				dst[i].RouteMask |= routeMask
				merged = true
				break
			}
		}
		if merged {
			continue
		}
		dst = append(dst, tokenCandidate{
			Tok:       candTok,
			Origin:    st,
			RouteMask: routeMask,
			EndPos:    candEndPos,
			EndRow:    candEndRow,
			EndCol:    candEndCol,
		})
	}

	if len(dst) == 0 {
		return tokenFrontier{}, false
	}
	return tokenFrontier{
		StartByte:  uint32(startPos),
		StartPoint: Point{Row: startRow, Column: startCol},
		Candidates: dst,
	}, true
}

func (d *dfaTokenSource) tokenFrontierRouteMask(states []StateID, tok Token, endPos int, endRow, endCol uint32) uint16 {
	var mask uint16
	for i, st := range states {
		if i >= 16 {
			break
		}
		if d.lookupActionIndex(st, tok.Symbol) == 0 {
			continue
		}
		if !d.stateProducesTokenFrontierCandidate(st, tok, endPos, endRow, endCol) {
			continue
		}
		mask |= uint16(1) << uint(i)
	}
	return mask
}

func (d *dfaTokenSource) stateProducesTokenFrontierCandidate(state StateID, tok Token, endPos int, endRow, endCol uint32) bool {
	stateTok, stateEndPos, stateEndRow, stateEndCol := d.scanPreferredTokenForState(state)
	return tokensSameLexForGLRCandidate(stateTok, tok) && stateEndPos == endPos && stateEndRow == endRow && stateEndCol == endCol
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
	startRangeIdx := d.lexer.includedRangeIdx
	d.beginGLRUnionScanCache()

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
		key := lexModeKey(mode)
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

		candTok, candEndPos, candEndRow, candEndCol := d.glrUnionScanForState(st)

		score := 0
		for liveIdx, liveState := range d.glrStates {
			if d.dedupeGLRUnionScoreStates() && d.priorGLRState(liveIdx, liveState) {
				continue
			}
			if d.lookupActionIndex(liveState, candTok.Symbol) == 0 {
				continue
			}
			stateTok, stateEndPos, stateEndRow, stateEndCol := d.glrUnionScanForState(liveState)
			if !tokensSameLexForGLRCandidate(stateTok, candTok) || stateEndPos != candEndPos || stateEndRow != candEndRow || stateEndCol != candEndCol {
				continue
			}
			score++
		}
		originActionCount := 0
		if idx := d.lookupActionIndex(st, candTok.Symbol); idx != 0 && int(idx) < len(d.language.ParseActions) {
			originActionCount = len(d.language.ParseActions[idx].Actions)
		}

		if DebugDFA.Load() {
			fmt.Printf("  GLRUNION cand state=%d tok=%s(%d)[%d-%d] score=%d\n", st, d.symbolName(candTok.Symbol), candTok.Symbol, candTok.StartByte, candTok.EndByte, score)
		}
		if score <= 0 {
			continue
		}

		candVisible := int(candTok.Symbol) < len(d.language.SymbolMetadata) && d.language.SymbolMetadata[candTok.Symbol].Visible

		// A generated zero-width sentinel (e.g. Go's implicit `\0` statement
		// terminator) matched by ONE live GLR stack's own lex mode should not
		// automatically beat a genuine, later-starting token matched by a
		// DIFFERENT live stack's lex mode purely because it "starts earlier"
		// — it always trivially starts earlier by virtue of being
		// zero-width, not because it reflects real content the other
		// candidate misses. These are two independent, mutually exclusive
		// interpretations resolving their own state correctly; picking the
		// sentinel here silently discards a cross-stack-supported real
		// continuation on the very same source line. See
		// realTokenBeatsZeroWidthSentinelAcrossStacks for the narrow
		// same-line / non-boundary scope this override applies within.
		if bestFound {
			if d.realTokenBeatsZeroWidthSentinelAcrossStacks(bestTok, candTok, score) {
				bestFound = true
				bestScore = score
				bestTok = candTok
				bestEndPos = candEndPos
				bestEndRow = candEndRow
				bestEndCol = candEndCol
				bestVisible = candVisible
				bestOriginActions = originActionCount
				continue
			}
			if d.realTokenBeatsZeroWidthSentinelAcrossStacks(candTok, bestTok, bestScore) {
				continue
			}
		}

		splitPreference := 0
		if candTok.StartByte == bestTok.StartByte {
			splitPreference = d.compareAngleTokenPreference(candTok, bestTok)
		}
		specificPreference := d.compareSpecificTokenPreference(candTok, candEndPos, bestTok, bestEndPos)
		better := !bestFound ||
			candTok.StartByte < bestTok.StartByte ||
			(candTok.StartByte == bestTok.StartByte && splitPreference > 0) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos > bestEndPos) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos == bestEndPos && candTok.EndByte > bestTok.EndByte) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos == bestEndPos && candTok.EndByte == bestTok.EndByte && specificPreference > 0) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos == bestEndPos && candTok.EndByte == bestTok.EndByte && specificPreference == 0 && originActionCount > bestOriginActions) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos == bestEndPos && candTok.EndByte == bestTok.EndByte && specificPreference == 0 && score > bestScore) ||
			(candTok.StartByte == bestTok.StartByte && splitPreference == 0 && candEndPos == bestEndPos && candTok.EndByte == bestTok.EndByte && specificPreference == 0 && score == bestScore && candVisible && !bestVisible)
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
		d.lexer.includedRangeIdx = startRangeIdx
		return Token{}, false
	}
	// Preserve the largest dependency frontier among every scanned candidate
	// with the elected token surface. Candidate identity ignores this field,
	// but the returned token must retain the complete invalidation extent.
	for i := range d.glrUnionScanScratch {
		scan := &d.glrUnionScanScratch[i]
		if !tokensSameLexForGLRCandidate(scan.tok, bestTok) ||
			scan.endPos != bestEndPos || scan.endRow != bestEndRow || scan.endCol != bestEndCol {
			continue
		}
		mergeTokenLookaheadFrontier(&bestTok, scan.tok)
	}

	d.lexer.pos = bestEndPos
	d.lexer.row = bestEndRow
	d.lexer.col = bestEndCol
	d.lexer.includedRangeIdx = d.lexer.includedRangeIndexForPosition(bestEndPos)
	return bestTok, true
}

func (d *dfaTokenSource) glrUnionScanForState(state StateID) (Token, int, uint32, uint32) {
	for i := range d.glrUnionScanScratch {
		cached := &d.glrUnionScanScratch[i]
		if cached.state == state {
			return cached.tok, cached.endPos, cached.endRow, cached.endCol
		}
	}
	tok, endPos, endRow, endCol := d.scanPreferredTokenForState(state)
	d.glrUnionScanScratch = append(d.glrUnionScanScratch, glrUnionDFAScan{
		state:  state,
		tok:    tok,
		endPos: endPos,
		endRow: endRow,
		endCol: endCol,
	})
	return tok, endPos, endRow, endCol
}

func (d *dfaTokenSource) beginGLRUnionScanCache() {
	if cap(d.glrUnionScanScratch) > len(d.glrUnionScanInline) {
		d.glrUnionScanScratch = d.glrUnionScanScratch[:0]
		return
	}
	d.glrUnionScanScratch = d.glrUnionScanInline[:0]
}

func (d *dfaTokenSource) clearGLRUnionScanCache() {
	clear(d.glrUnionScanInline[:])
	if cap(d.glrUnionScanScratch) > len(d.glrUnionScanInline) {
		clear(d.glrUnionScanScratch[:cap(d.glrUnionScanScratch)])
		d.glrUnionScanScratch = d.glrUnionScanScratch[:0]
		return
	}
	d.glrUnionScanScratch = d.glrUnionScanInline[:0]
}

// realTokenBeatsZeroWidthSentinelAcrossStacks reports whether real (a
// genuine, non-zero-width token matched by one live GLR stack's lex mode)
// should be preferred over zero (this language's generated zero-width
// sentinel terminal, e.g. Go's implicit `\0` statement terminator, matched
// by a DIFFERENT live stack's own lex mode) when nextGLRUnionDFAToken picks
// the single token shared by all live stacks this round.
//
// The union scan's ordinary tie-break prefers whichever candidate starts
// earliest, which is sound when comparing two real tokens — an earlier
// start reflects a genuine difference in what the source contains at that
// position. It is unsound when one candidate is a zero-width sentinel:
// being zero-width means it always starts at or before any real match
// found by continuing the scan, regardless of whether real content
// actually follows. Two different, mutually exclusive parses (two live GLR
// stacks) can each correctly resolve their OWN lex mode at this exact
// position — one finding the sentinel because ITS state accepts it there,
// the other finding a real, later-starting token because ITS state expects
// one — and blindly preferring the shorter match discards the
// cross-stack-supported real continuation.
//
// Scope is deliberately narrow: real must be separated from zero only by
// horizontal whitespace on the same source line (a real newline is exactly
// what the terminator is supposed to detect, so crossing one leaves the
// sentinel's win untouched), zero must not sit at a genuine
// atGeneratedZeroWidthSentinelBoundary (a closing bracket or EOF — the
// sentinel's own deliberate, pre-existing trigger), and real must carry
// positive cross-stack support (realScore, the same score already computed
// by the caller) so an unsupported stray match can't win.
func (d *dfaTokenSource) realTokenBeatsZeroWidthSentinelAcrossStacks(zero, real Token, realScore int) bool {
	if d == nil || d.lexer == nil || !d.hasZeroWidthSentinelSymbol {
		return false
	}
	if zero.Symbol != d.zeroWidthSentinelSymbol || zero.EndByte != zero.StartByte {
		return false
	}
	if real.Symbol == 0 || real.Symbol == d.zeroWidthSentinelSymbol || realScore <= 0 {
		return false
	}
	if real.StartByte <= zero.StartByte {
		return false
	}
	if d.atGeneratedZeroWidthSentinelBoundary(int(zero.StartByte)) {
		return false
	}
	start := int(zero.StartByte)
	end := int(real.StartByte)
	if start < 0 || end < start || end > len(d.lexer.source) {
		return false
	}
	for _, b := range d.lexer.source[start:end] {
		if b == '\n' || b == '\r' {
			return false
		}
	}
	return true
}

func (d *dfaTokenSource) dedupeGLRUnionScoreStates() bool {
	return d != nil && d.language != nil && d.language.Name == "markdown_inline"
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
	// Selection observes both probes, including a discarded longer match.
	frontier := maxUint32(tokenLookaheadEndByte(baseTok), tokenLookaheadEndByte(afterTok))
	baseTok.lexerLookaheadEndByte, afterTok.lexerLookaheadEndByte = frontier, frontier
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
	savedRangeIdx := d.lexer.includedRangeIdx
	savedState := d.state

	d.state = state
	tok := d.nextTokenForLexState(lexState)
	if realTok, ok := d.preferSameLineTokenOverGeneratedZeroWidthSentinel(state, lexState, tok, savedPos, savedRow, savedCol); ok {
		tok = realTok
	}
	if d.isScheme && !d.lexer.errorModeRetry {
		// With the faithful C recovery port gated on, the lexer's error-mode
		// retry replaces scheme's dedicated run heuristic: failed lexes
		// surface real error-mode tokens (or errorSymbol runs) exactly like
		// C, and re-deriving a wider run here would mask them.
		if errTok, ok := d.schemeErrorRunToken(savedPos, savedRow, savedCol, tok); ok {
			d.lexer.pos = savedPos
			d.lexer.row = savedRow
			d.lexer.col = savedCol
			d.lexer.includedRangeIdx = savedRangeIdx
			d.state = savedState
			if DebugDFA.Load() {
				fmt.Printf("  SCHEME-ERR run %d-%d state=%d\n", errTok.StartByte, errTok.EndByte, state)
			}
			return errTok, int(errTok.EndByte), errTok.EndPoint.Row, errTok.EndPoint.Column
		}
	}
	if zeroTok, ok := d.preferGeneratedZeroWidthSentinelForState(state, tok, savedPos, savedRow, savedCol); ok {
		tok = zeroTok
		d.lexer.pos = savedPos
		d.lexer.row = savedRow
		d.lexer.col = savedCol
		d.lexer.includedRangeIdx = savedRangeIdx
	}
	if tok.Symbol == errorSymbol {
		// Unlexable-run error token from the lexer (mirrors C skipped-error
		// lexing). Return it as-is: keyword promotion and DFA-token
		// normalization only apply to real grammar tokens.
		d.lexer.pos = savedPos
		d.lexer.row = savedRow
		d.lexer.col = savedCol
		d.lexer.includedRangeIdx = savedRangeIdx
		d.state = savedState
		if DebugDFA.Load() {
			fmt.Printf("  LEX-ERR run %d-%d state=%d\n", tok.StartByte, tok.EndByte, state)
		}
		return tok, int(tok.EndByte), tok.EndPoint.Row, tok.EndPoint.Column
	}
	if d.hasZeroWidthStartAccept {
		if zeroTok, ok := d.preferZeroWidthStartAcceptForState(state, lexState, tok, savedPos, savedRow, savedCol); ok {
			tok = zeroTok
			d.lexer.pos = savedPos
			d.lexer.row = savedRow
			d.lexer.col = savedCol
			d.lexer.includedRangeIdx = savedRangeIdx
		}
	}
	var keywordDemoted bool
	tok, keywordDemoted = d.promoteKeyword(tok)
	if !keywordDemoted {
		tok = d.promoteActiveLiteralForCurrentState(tok, savedPos, savedRow, savedCol)
	}
	tok = d.demoteSwiftMemberKeyword(tok)
	tok, endPos, endRow, endCol := d.normalizeDFAToken(tok, d.lexer.pos, d.lexer.row, d.lexer.col)

	d.lexer.pos = savedPos
	d.lexer.row = savedRow
	d.lexer.col = savedCol
	d.lexer.includedRangeIdx = savedRangeIdx
	d.state = savedState

	return tok, endPos, endRow, endCol
}

func (d *dfaTokenSource) scanRawPreferredTokenForState(state StateID) (Token, int, uint32, uint32) {
	if d == nil || d.lexer == nil {
		return Token{}, 0, 0, 0
	}
	lexModes := d.lexModeStartRows()
	if int(state) >= len(lexModes) {
		return Token{}, d.lexer.pos, d.lexer.row, d.lexer.col
	}
	mode := lexModes[state]
	if mode.afterWhitespaceLexState == 0 {
		return d.scanRawDFATokenForLexState(mode.lexState)
	}
	if !d.isAtWhitespacePosition() && !d.isAfterWhitespacePosition() {
		return d.scanRawDFATokenForLexState(mode.lexState)
	}

	baseTok, baseEndPos, baseEndRow, baseEndCol := d.scanRawDFATokenForLexState(mode.lexState)
	afterTok, afterEndPos, afterEndRow, afterEndCol := d.scanRawDFATokenForLexState(mode.afterWhitespaceLexState)
	frontier := maxUint32(tokenLookaheadEndByte(baseTok), tokenLookaheadEndByte(afterTok))
	baseTok.lexerLookaheadEndByte, afterTok.lexerLookaheadEndByte = frontier, frontier
	if d.shouldPreferBaseLexStateToken(baseTok, afterTok) {
		return baseTok, baseEndPos, baseEndRow, baseEndCol
	}
	return afterTok, afterEndPos, afterEndRow, afterEndCol
}

func (d *dfaTokenSource) scanRawDFATokenForLexState(lexState uint32) (Token, int, uint32, uint32) {
	if d == nil || d.lexer == nil {
		return Token{}, 0, 0, 0
	}
	savedPos := d.lexer.pos
	savedRow := d.lexer.row
	savedCol := d.lexer.col
	savedRangeIdx := d.lexer.includedRangeIdx

	tok := d.nextTokenForLexState(lexState)
	endPos := d.lexer.pos
	endRow := d.lexer.row
	endCol := d.lexer.col

	d.lexer.pos = savedPos
	d.lexer.row = savedRow
	d.lexer.col = savedCol
	d.lexer.includedRangeIdx = savedRangeIdx
	return tok, endPos, endRow, endCol
}

func (d *dfaTokenSource) preferSameLineTokenOverGeneratedZeroWidthSentinel(state StateID, lexState uint32, tok Token, startPos int, startRow, startCol uint32) (Token, bool) {
	if d == nil || d.lexer == nil || !d.hasZeroWidthSentinelSymbol || tok.Symbol != d.zeroWidthSentinelSymbol {
		return Token{}, false
	}
	if tok.StartByte != uint32(startPos) || tok.EndByte != tok.StartByte {
		return Token{}, false
	}
	if d.atGeneratedZeroWidthSentinelBoundary(startPos) {
		return Token{}, false
	}
	pos := startPos
	row := startRow
	col := startCol
	for pos < len(d.lexer.source) {
		switch d.lexer.source[pos] {
		case ' ', '\t', '\f':
			pos++
			col++
		case '\n', '\r':
			return Token{}, false
		default:
			goto scanReal
		}
	}
	return Token{}, false

scanReal:
	if pos == startPos {
		return Token{}, false
	}
	d.lexer.pos = pos
	d.lexer.row = row
	d.lexer.col = col
	realTok := d.nextTokenForLexState(lexState)
	if realTok.Symbol == 0 || realTok.StartByte != uint32(pos) || realTok.EndByte <= realTok.StartByte {
		d.lexer.pos = startPos
		d.lexer.row = startRow
		d.lexer.col = startCol
		return Token{}, false
	}
	// The raw DFA scan classifies word-shaped text (identifiers) without
	// keyword promotion — reserved words like "else" come back as a plain
	// "identifier" token here, exactly like every other name. Promote it
	// before checking whether the current state can actually consume it,
	// or a same-line keyword continuation (e.g. `if x {...} else {...}`)
	// always fails this check on its un-promoted "identifier" symbol and
	// falls through to the zero-width sentinel, letting the parser close
	// off the enclosing construct (if_statement) one token too early and
	// treat the keyword text as a bare identifier from then on.
	realTok = d.promoteActiveLiteralForCurrentState(realTok, pos, row, col)
	if !d.hasActionForStateSymbol(state, realTok.Symbol) {
		d.lexer.pos = startPos
		d.lexer.row = startRow
		d.lexer.col = startCol
		return Token{}, false
	}
	return realTok, true
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
	if d.isZeroWidthSymbol(baseTok.Symbol) && baseTok.EndByte == baseTok.StartByte && baseTok.StartByte < afterTok.StartByte {
		return d.shouldPreferGeneratedZeroWidthSentinelAcrossWhitespace(baseTok, afterTok)
	}
	return baseTok.StartByte < afterTok.StartByte
}

func (d *dfaTokenSource) shouldPreferGeneratedZeroWidthSentinelAcrossWhitespace(baseTok, afterTok Token) bool {
	if d == nil || d.lexer == nil || !d.hasZeroWidthSentinelSymbol || baseTok.Symbol != d.zeroWidthSentinelSymbol {
		return false
	}
	if !d.hasActionForStateSymbol(d.state, baseTok.Symbol) {
		return false
	}
	start := int(baseTok.StartByte)
	if d.atGeneratedZeroWidthSentinelBoundary(start) {
		return true
	}
	end := int(afterTok.StartByte)
	if start < 0 || end < start || end > len(d.lexer.source) {
		return false
	}
	for _, b := range d.lexer.source[start:end] {
		if b == '\n' || b == '\r' {
			return true
		}
	}
	return false
}

func (d *dfaTokenSource) shouldPreferZeroWidthBaseLexStateToken(baseTok, afterTok Token) bool {
	if d == nil || d.language == nil {
		return false
	}
	if baseTok.StartByte != afterTok.StartByte || baseTok.EndByte != baseTok.StartByte {
		return false
	}
	if !d.isZeroWidthSymbol(baseTok.Symbol) {
		return false
	}
	return d.hasActionForStateSymbol(d.state, baseTok.Symbol)
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
	if !d.hasActionForStateSymbol(state, startAccept) {
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
	if d == nil || d.language == nil {
		return false
	}
	idx := int(sym)
	if idx >= 0 && idx < len(d.language.ZeroWidthTokens) && d.language.ZeroWidthTokens[idx] {
		return true
	}
	return d.hasZeroWidthSentinelSymbol && sym == d.zeroWidthSentinelSymbol
}

func (d *dfaTokenSource) preferGeneratedZeroWidthSentinelForState(state StateID, tok Token, startPos int, startRow, startCol uint32) (Token, bool) {
	if d == nil || d.language == nil || d.lexer == nil || !d.hasZeroWidthSentinelSymbol || d.zeroWidthSentinelSymbol == 0 {
		return Token{}, false
	}
	if startPos < len(d.lexer.source) && d.lexer.source[startPos] == 0 {
		return Token{}, false
	}
	if !d.atGeneratedZeroWidthSentinelBoundary(startPos) {
		return Token{}, false
	}
	if tok.Symbol == d.zeroWidthSentinelSymbol {
		return Token{}, false
	}
	if !d.hasActionForStateSymbol(state, d.zeroWidthSentinelSymbol) {
		return Token{}, false
	}
	if tok.Symbol != 0 && tok.Symbol != errorSymbol && tok.StartByte == uint32(startPos) && d.hasActionForStateSymbol(state, tok.Symbol) {
		return Token{}, false
	}
	pt := Point{Row: startRow, Column: startCol}
	return Token{
		Symbol:     d.zeroWidthSentinelSymbol,
		StartByte:  uint32(startPos),
		EndByte:    uint32(startPos),
		StartPoint: pt,
		EndPoint:   pt,
	}, true
}

func (d *dfaTokenSource) atGeneratedZeroWidthSentinelBoundary(startPos int) bool {
	if d == nil || d.lexer == nil {
		return false
	}
	source := d.lexer.source
	pos := startPos
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
		case ')', ']', '}':
			return true
		default:
			return false
		}
	}
	return true
}

func (d *dfaTokenSource) hasActionForStateSymbol(state StateID, sym Symbol) bool {
	if d == nil || d.language == nil || d.lookupActionIndex == nil || sym == 0 {
		return false
	}
	idx := d.lookupActionIndex(state, sym)
	if idx == 0 || int(idx) >= len(d.language.ParseActions) {
		return false
	}
	return len(d.language.ParseActions[idx].Actions) > 0
}

// tokenIsExtraInAllActiveStates reports whether every active parser action
// that can consume sym is an extra shift. External scanners can return both
// layout tokens (comments, newlines) and ordinary content tokens; the lexer
// must distinguish them when deciding whether a trailing whitespace byte was
// layout or part of the token itself.
func (d *dfaTokenSource) tokenIsExtraInAllActiveStates(sym Symbol) bool {
	if d == nil || d.language == nil || d.lookupActionIndex == nil || sym == 0 {
		return false
	}
	sawExtra := false
	sawNonExtra := false
	visit := func(state StateID) {
		idx := d.lookupActionIndex(state, sym)
		if idx == 0 || int(idx) >= len(d.language.ParseActions) {
			return
		}
		for _, action := range d.language.ParseActions[idx].Actions {
			if action.Type == ParseActionShift && (action.Extra || action.ExtraChain) {
				sawExtra = true
				continue
			}
			sawNonExtra = true
		}
	}
	visit(d.state)
	for _, state := range d.glrStates {
		if state != d.state {
			visit(state)
		}
	}
	return sawExtra && !sawNonExtra
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
	if ch := d.lexer.source[d.lexer.pos]; ch < utf8.RuneSelf {
		switch ch {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			return true
		default:
			return false
		}
	}
	r, _ := utf8.DecodeRune(d.lexer.source[d.lexer.pos:])
	return unicode.IsSpace(r)
}

func (d *dfaTokenSource) isAfterWhitespacePosition() bool {
	if d == nil || d.lexer == nil || d.lexer.pos <= 0 || d.lexer.pos > len(d.lexer.source) {
		return false
	}
	// A non-extra external token may itself end in whitespace. Python's
	// _string_content is the canonical case: the space in `"\\x12 \\123"`
	// is string data, so the following immediate escape_sequence remains
	// eligible. The byte-only fallback below cannot distinguish that from
	// skipped layout; the external-token checkpoint can.
	if d.lastExternalTokenValid &&
		!d.externalTokenEndSameAsStart &&
		!d.lastExternalTokenWasExtra &&
		d.lastExternalTokenStartByte < d.lastExternalTokenEndByte &&
		d.lastExternalTokenEndByte == uint32(d.lexer.pos) {
		return false
	}
	return sourcePositionFollowsWhitespace(d.lexer.source, d.lexer.pos)
}

func sourcePositionFollowsWhitespace(source []byte, pos int) bool {
	if pos <= 0 || pos > len(source) {
		return false
	}
	if ch := source[pos-1]; ch < utf8.RuneSelf {
		switch ch {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			return true
		default:
			return false
		}
	}
	r, _ := utf8.DecodeLastRune(source[:pos])
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
	if !supportsCompactCloseAngleSplit(d.language.Name) {
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

func supportsCompactCloseAngleSplit(languageName string) bool {
	switch languageName {
	case "dart", "java", "swift", "tsx", "typescript":
		return true
	default:
		return false
	}
}

func (p *Parser) contextualActionIndex(source []byte, state StateID, tok Token) uint16 {
	actionIdx := p.lookupActionIndex(state, tok.Symbol)
	if actionIdx != 0 && p.shouldDeferContextualCloseAngleAction(source, state, tok) {
		return 0
	}
	return actionIdx
}

// shouldDeferContextualCloseAngleAction reports that this stack's lex mode
// reads an adjacent pair as one operator. Another live stack selected the
// single close-angle prefix, so this stack must not consume that prefix.
func (p *Parser) shouldDeferContextualCloseAngleAction(source []byte, state StateID, tok Token) bool {
	return deferContextualCloseAngleAction(p.language, source, state, tok, p.included, &p.relexProbeLexer)
}

// tokenMaybeContextualCloseAngle reports whether tok could possibly be the
// narrow half of a contextual close-angle pair: a single ">" byte that does
// not cross a line. This is exactly deferContextualCloseAngleAction's own
// first guard, extracted so a caller that resolves its own state lazily
// (dispatchCorridor's corridor pre-check, parsercore_c4_vm.go) can rule out
// the common non-">"-token case before paying for that resolve, without a
// second, independently-maintained copy of the shape check.
// deferContextualCloseAngleAction is still the single source of truth for
// the full predicate: it calls this helper for its own first guard rather
// than re-deriving it.
func tokenMaybeContextualCloseAngle(lang *Language, tok Token) bool {
	if lang == nil || int(tok.Symbol) >= len(lang.SymbolNames) {
		return false
	}
	return lang.SymbolNames[tok.Symbol] == ">" &&
		tok.EndByte == tok.StartByte+1 &&
		tok.EndPoint.Row == tok.StartPoint.Row
}

// deferContextualCloseAngleAction reports whether state's own lex mode reads
// the bytes under tok as one wider close-angle operator that carries a real
// parse action in state. When it does, a different, narrower-lexing route
// already elected tok, and state must defer rather than consume tok's
// prefix. The check is symbol-shape based (an adjacent run of `>` bytes),
// not tied to any one language, so both the production stack route and the
// compact admission route can share it. included and probe let the
// production route reuse its own included-range handling and scratch
// lexer; the compact callers (parsercore_c4_vm.go, parsercore_phase0_driver.go)
// always pass included=nil and their own scratch lexer instead, because the
// compact route declines any included-range parse outright before it ever
// reaches here (admission_switch.go:208's own eligibility check) — there is
// no included-range case for a compact caller to reuse.
func deferContextualCloseAngleAction(lang *Language, source []byte, state StateID, tok Token, included []Range, probe *Lexer) bool {
	if probe == nil || !tokenMaybeContextualCloseAngle(lang, tok) {
		return false
	}
	start := int(tok.StartByte)
	if start < 0 || start+1 >= len(source) || source[start] != '>' || source[start+1] != '>' ||
		int(state) >= len(lang.LexModes) {
		return false
	}
	lexState := lang.LexModes[state].LexStateIndex()
	if lexState == noLookaheadLexState || int(lexState) >= len(lang.LexStates) {
		return false
	}

	*probe = Lexer{
		states:          lang.LexStates,
		asciiTable:      lang.LexAsciiTable(),
		source:          source,
		pos:             start,
		row:             tok.StartPoint.Row,
		col:             tok.StartPoint.Column,
		immediateTokens: lang.ImmediateTokens,
		zeroWidthTokens: lang.ZeroWidthTokens,
	}
	if len(included) != 0 && lang.ExternalScanner == nil && len(lang.ExternalSymbols) == 0 {
		probe.setIncludedRanges(included)
	}
	stateToken, ok := probe.scan(uint32(lexState), probe.pos, probe.row, probe.col)
	if !ok || int(stateToken.Symbol) >= len(lang.SymbolNames) {
		return false
	}
	stateTokenName := lang.SymbolNames[stateToken.Symbol]
	// Short-circuit order matches the pre-extraction code: the table lookup
	// below only runs once the cheap shape checks (wide close-angle name,
	// same start byte) already passed, not unconditionally on every call.
	if !isWideCloseAngleTokenName(stateTokenName) || stateToken.StartByte != tok.StartByte {
		return false
	}
	shiftIdx := lookupRepairActionIndex(lang, state, stateToken.Symbol)
	stateHasShiftAction := shiftIdx != 0 && int(shiftIdx) < len(lang.ParseActions) && len(lang.ParseActions[shiftIdx].Actions) > 0
	if !stateHasShiftAction {
		return false
	}
	width := uint32(len(stateTokenName))
	if stateToken.EndByte != tok.StartByte+width {
		return false
	}
	closeIdx := lookupRepairActionIndex(lang, state, tok.Symbol)
	if closeIdx != 0 && closeIdx == shiftIdx && int(closeIdx) < len(lang.ParseActions) {
		actions := lang.ParseActions[closeIdx].Actions
		if len(actions) > 0 {
			reduceOnly := true
			for _, action := range actions {
				if action.Type != ParseActionReduce {
					reduceOnly = false
					break
				}
			}
			if reduceOnly {
				return false
			}
		}
	}
	return true
}

func (d *dfaTokenSource) shouldSplitCompactCloseAngleToken(tok Token, gtSym, shiftSym Symbol, shiftOK bool) bool {
	// With no `>>` symbol active in this state the run cannot shift as one
	// token, so narrowing it to `>` is the only way to make progress. There is
	// no shift-operator reading to protect, and the angle-depth gate below
	// would only stall the parse.
	if !shiftOK {
		return true
	}
	// A real `>>` alternative exists, so the run is ambiguous: nested generic
	// closers or a signed right shift. Only treat it as generic closers when an
	// unclosed '<' actually precedes it.
	if d != nil && d.language != nil && requiresUnclosedAngleForCloseAngleSplit(d.language.Name) &&
		!d.hasUnclosedAngleBefore(int(tok.StartByte)) {
		return false
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

// requiresUnclosedAngleForCloseAngleSplit names the languages where `>>` is
// both a nested generic closer and a real signed right-shift operator. In those
// languages the compact close-angle split may only fire when an unclosed '<'
// actually precedes the run, or a shift expression gets torn into two '>'
// tokens and the parse fails.
//
// Dart and Swift are deliberately absent. Swift runs its own
// splitSwiftWideCloseAngleToken path with an equivalent angle-depth gate, and
// Dart is left on the previous behaviour rather than changed unmeasured here.
func requiresUnclosedAngleForCloseAngleSplit(languageName string) bool {
	switch languageName {
	case "java", "typescript", "tsx":
		return true
	default:
		return false
	}
}

// hasUnclosedAngleBefore reports whether an unclosed '<' precedes pos, scanning
// back until a statement or bracket boundary. It is the "am I really inside a
// generic argument list" heuristic shared by the languages named in
// requiresUnclosedAngleForCloseAngleSplit.
func (d *dfaTokenSource) hasUnclosedAngleBefore(pos int) bool {
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

func (d *dfaTokenSource) compareSpecificTokenPreference(candTok Token, candEndPos int, bestTok Token, bestEndPos int) int {
	if d.preferSpecificTokenOnExactMatch(candTok, candEndPos, bestTok, bestEndPos) {
		return 1
	}
	if d.preferSpecificTokenOnExactMatch(bestTok, bestEndPos, candTok, candEndPos) {
		return -1
	}
	return 0
}

func (d *dfaTokenSource) compareAngleTokenPreference(candTok, bestTok Token) int {
	if d == nil || d.language == nil {
		return 0
	}
	if int(candTok.Symbol) >= len(d.language.SymbolNames) || int(bestTok.Symbol) >= len(d.language.SymbolNames) {
		return 0
	}
	candName := d.language.SymbolNames[candTok.Symbol]
	bestName := d.language.SymbolNames[bestTok.Symbol]
	// One shared token cannot preserve parse versions whose lex modes split a
	// close-angle run at different widths. Prefer one close angle. A parser can
	// compose later angles into nested generic closers, while a wide token
	// consumes those bytes before that lineage can use them.
	if candName == ">" && isWideCloseAngleTokenName(bestName) {
		return 1
	}
	if bestName == ">" && isWideCloseAngleTokenName(candName) {
		return -1
	}
	return 0
}

func isWideCloseAngleTokenName(name string) bool {
	return len(name) > 1 && strings.Trim(name, ">") == ""
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

// clampedSkipTarget validates the parser-internal source invariant defensively
// and compares in uint64 before narrowing to int. On 32-bit builds this avoids
// turning a large uint32 offset into a negative target. The bool reports a
// past-EOF request so SkipToByteWithPoint can derive the real EOF point instead
// of trusting a point associated with an out-of-source offset.
func (d *dfaTokenSource) clampedSkipTarget(offset uint32) (target int, pastEOF, ok bool) {
	if d == nil || d.lexer == nil {
		return 0, false, false
	}
	sourceLen := len(d.lexer.source)
	if uint64(offset) > uint64(sourceLen) {
		return sourceLen, true, true
	}
	return int(offset), false, true
}

func (d *dfaTokenSource) SkipToByte(offset uint32) Token {
	target, _, ok := d.clampedSkipTarget(offset)
	if !ok {
		return Token{}
	}
	// Clamp past-EOF targets (e.g. an out-of-range included range): skipOneRune
	// is a no-op at EOF, so an unclamped target > len(source) would spin the
	// loop below forever.
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
	target, pastEOF, ok := d.clampedSkipTarget(offset)
	if !ok {
		return Token{}
	}
	if pastEOF {
		// The supplied point describes the invalid requested offset, not EOF.
		// Use the bounded scanning variant to preserve exact EOF coordinates.
		return d.SkipToByte(offset)
	}
	if target >= d.lexer.pos {
		d.lexer.pos = target
		d.lexer.row = pt.Row
		d.lexer.col = pt.Column
		d.lexer.includedRangeIdx = 0
		d.lexer.normalizeIncludedPosition()
	}
	return d.Next()
}

func (d *dfaTokenSource) CanRelexFromTokenStart(tok Token) bool {
	if d == nil || d.lexer == nil {
		return false
	}
	target := int(tok.StartByte)
	if target < 0 || target > len(d.lexer.source) {
		return false
	}
	if d.hasExternalScanner {
		if d.usesExternalCheckpoints {
			if !d.lastExternalTokenValid ||
				d.lastExternalTokenStartByte != tok.StartByte ||
				d.lastExternalTokenEndByte != tok.EndByte ||
				len(d.externalTokenStart) == 0 ||
				len(d.captureExternalScannerStateInto(&d.externalCompare)) == 0 {
				return false
			}
		} else if len(d.captureExternalScannerStateInto(&d.externalCompare)) != 0 {
			return false
		}
	}
	return true
}

type dfaRelexSnapshot struct {
	lexerPos               int
	lexerRow               uint32
	lexerCol               uint32
	lexerRangeIdx          int
	externalScannerPresent bool
	// externalLookaheadEndByte is the aggregate for the current C-style
	// lex call. Capture and restore it for rollback, but exclude it from
	// equal because it is not persistent scanner state.
	externalLookaheadEndByte uint32

	// Lexer.scan records the beginning of its most recent failed token
	// attempt. NextWithErrorRuns uses these coordinates to delimit the next
	// error run, so a rejected re-lex must restore them with the cursor.
	failTokenStartPos      int
	failTokenStartRow      uint32
	failTokenStartCol      uint32
	failTokenStartRangeIdx int

	externalPayload []byte

	lastExternalTokenStartByte  uint32
	lastExternalTokenEndByte    uint32
	lastExternalTokenValid      bool
	lastExternalTokenWasExtra   bool
	externalTokenEndSameAsStart bool
	lastTokenStartByte          uint32
	lastTokenEndByte            uint32
	lastTokenValid              bool
	externalTokenStart          []byte
	externalTokenEnd            []byte

	extZeroPos   int
	extZeroState StateID
	extZeroTried []bool

	zeroWidthPos   int
	zeroWidthCount int
}

func (s dfaRelexSnapshot) equal(other dfaRelexSnapshot) bool {
	return s.lexerPos == other.lexerPos && s.lexerRow == other.lexerRow &&
		s.lexerCol == other.lexerCol && s.lexerRangeIdx == other.lexerRangeIdx &&
		s.externalScannerPresent == other.externalScannerPresent &&
		s.failTokenStartPos == other.failTokenStartPos &&
		s.failTokenStartRow == other.failTokenStartRow &&
		s.failTokenStartCol == other.failTokenStartCol &&
		s.failTokenStartRangeIdx == other.failTokenStartRangeIdx &&
		bytes.Equal(s.externalPayload, other.externalPayload) &&
		s.lastExternalTokenStartByte == other.lastExternalTokenStartByte &&
		s.lastExternalTokenEndByte == other.lastExternalTokenEndByte &&
		s.lastExternalTokenValid == other.lastExternalTokenValid &&
		s.lastExternalTokenWasExtra == other.lastExternalTokenWasExtra &&
		s.externalTokenEndSameAsStart == other.externalTokenEndSameAsStart &&
		s.lastTokenStartByte == other.lastTokenStartByte &&
		s.lastTokenEndByte == other.lastTokenEndByte &&
		s.lastTokenValid == other.lastTokenValid &&
		bytes.Equal(s.externalTokenStart, other.externalTokenStart) &&
		bytes.Equal(s.externalTokenEnd, other.externalTokenEnd) &&
		s.extZeroPos == other.extZeroPos && s.extZeroState == other.extZeroState &&
		slices.Equal(s.extZeroTried, other.extZeroTried) &&
		s.zeroWidthPos == other.zeroWidthPos && s.zeroWidthCount == other.zeroWidthCount
}

// dfaRelexSnapshotScratch owns the mutable slice backing for one transient
// snapshot. A caller can reuse it only after the prior snapshot is dead.
// Published parser-version snapshots must still deep-copy these slices.
type dfaRelexSnapshotScratch struct {
	externalPayload    []byte
	externalTokenStart []byte
	externalTokenEnd   []byte
	extZeroTried       []bool
}

func (d *dfaTokenSource) snapshotRelexState() dfaRelexSnapshot {
	snapshot, _ := d.snapshotRelexStateWithExternalBuffer(nil)
	return snapshot
}

// snapshotRelexStateWithScratch captures the same state as
// snapshotRelexState while reusing one caller-owned backing store. The
// returned snapshot aliases scratch and remains valid only until the next
// call with that scratch.
func (d *dfaTokenSource) snapshotRelexStateWithScratch(scratch *dfaRelexSnapshotScratch) dfaRelexSnapshot {
	if scratch == nil {
		return d.snapshotRelexState()
	}
	s := d.snapshotRelexStateIntoScratch(scratch)
	if d.hasExternalScanner && d.language != nil && d.language.ExternalScanner != nil {
		buf := prepareDFARelexExternalPayloadScratch(scratch)
		if n := d.language.ExternalScanner.Serialize(d.externalPayload, buf); n > 0 {
			s.externalPayload = buf[:n]
			scratch.externalPayload = buf[:n]
		} else {
			scratch.externalPayload = buf[:0]
		}
	} else {
		clearDFARelexExternalPayloadScratch(scratch)
	}
	return s
}

// snapshotRelexStateWithScratchFromExternalPayload captures one snapshot
// from scanner bytes that the caller already serialized at this cursor.
// This avoids a second Serialize call during a parser election.
func (d *dfaTokenSource) snapshotRelexStateWithScratchFromExternalPayload(
	scratch *dfaRelexSnapshotScratch,
	externalPayload []byte,
) dfaRelexSnapshot {
	if scratch == nil {
		scratch = &dfaRelexSnapshotScratch{}
	}
	s := d.snapshotRelexStateIntoScratch(scratch)
	if d.hasExternalScanner && d.language != nil && d.language.ExternalScanner != nil {
		buf := prepareDFARelexExternalPayloadScratch(scratch)
		if n := copy(buf, externalPayload); n > 0 {
			s.externalPayload = buf[:n]
			scratch.externalPayload = buf[:n]
		} else {
			scratch.externalPayload = buf[:0]
		}
	} else {
		clearDFARelexExternalPayloadScratch(scratch)
	}
	return s
}

func (d *dfaTokenSource) snapshotRelexStateIntoScratch(scratch *dfaRelexSnapshotScratch) dfaRelexSnapshot {
	s := dfaRelexSnapshot{
		lexerPos:                    d.lexer.pos,
		lexerRow:                    d.lexer.row,
		lexerCol:                    d.lexer.col,
		lexerRangeIdx:               d.lexer.includedRangeIdx,
		externalScannerPresent:      d.hasExternalScanner,
		externalLookaheadEndByte:    d.externalLookaheadEndByte,
		failTokenStartPos:           d.lexer.failTokenStartPos,
		failTokenStartRow:           d.lexer.failTokenStartRow,
		failTokenStartCol:           d.lexer.failTokenStartCol,
		failTokenStartRangeIdx:      d.lexer.failTokenStartRangeIdx,
		lastExternalTokenStartByte:  d.lastExternalTokenStartByte,
		lastExternalTokenEndByte:    d.lastExternalTokenEndByte,
		lastExternalTokenValid:      d.lastExternalTokenValid,
		lastExternalTokenWasExtra:   d.lastExternalTokenWasExtra,
		externalTokenEndSameAsStart: d.externalTokenEndSameAsStart,
		lastTokenStartByte:          d.lastTokenStartByte,
		lastTokenEndByte:            d.lastTokenEndByte,
		lastTokenValid:              d.lastTokenValid,
		extZeroPos:                  d.extZeroPos,
		extZeroState:                d.extZeroState,
		zeroWidthPos:                d.zeroWidthPos,
		zeroWidthCount:              d.zeroWidthCount,
	}
	scratch.externalTokenStart = append(scratch.externalTokenStart[:0], d.externalTokenStart...)
	if len(d.externalTokenStart) != 0 {
		s.externalTokenStart = scratch.externalTokenStart
	}
	scratch.externalTokenEnd = append(scratch.externalTokenEnd[:0], d.externalTokenEnd...)
	if len(d.externalTokenEnd) != 0 {
		s.externalTokenEnd = scratch.externalTokenEnd
	}
	scratch.extZeroTried = append(scratch.extZeroTried[:0], d.extZeroTried...)
	if len(d.extZeroTried) != 0 {
		s.extZeroTried = scratch.extZeroTried
	}
	return s
}

func prepareDFARelexExternalPayloadScratch(scratch *dfaRelexSnapshotScratch) []byte {
	if cap(scratch.externalPayload) != externalScannerSerializationBufferSize {
		if cap(scratch.externalPayload) > 0 {
			clear(scratch.externalPayload[:cap(scratch.externalPayload)])
		}
		scratch.externalPayload = make([]byte, externalScannerSerializationBufferSize)
	} else {
		scratch.externalPayload = scratch.externalPayload[:externalScannerSerializationBufferSize]
		clear(scratch.externalPayload)
	}
	return scratch.externalPayload
}

func clearDFARelexExternalPayloadScratch(scratch *dfaRelexSnapshotScratch) {
	if cap(scratch.externalPayload) != 0 {
		clear(scratch.externalPayload[:cap(scratch.externalPayload)])
		scratch.externalPayload = scratch.externalPayload[:0]
	}
}

func (d *dfaTokenSource) snapshotRelexStateWithExternalBuffer(buf []byte) (dfaRelexSnapshot, []byte) {
	s := dfaRelexSnapshot{
		lexerPos:                    d.lexer.pos,
		lexerRow:                    d.lexer.row,
		lexerCol:                    d.lexer.col,
		lexerRangeIdx:               d.lexer.includedRangeIdx,
		externalScannerPresent:      d.hasExternalScanner,
		externalLookaheadEndByte:    d.externalLookaheadEndByte,
		failTokenStartPos:           d.lexer.failTokenStartPos,
		failTokenStartRow:           d.lexer.failTokenStartRow,
		failTokenStartCol:           d.lexer.failTokenStartCol,
		failTokenStartRangeIdx:      d.lexer.failTokenStartRangeIdx,
		lastExternalTokenStartByte:  d.lastExternalTokenStartByte,
		lastExternalTokenEndByte:    d.lastExternalTokenEndByte,
		lastExternalTokenValid:      d.lastExternalTokenValid,
		lastExternalTokenWasExtra:   d.lastExternalTokenWasExtra,
		externalTokenEndSameAsStart: d.externalTokenEndSameAsStart,
		lastTokenStartByte:          d.lastTokenStartByte,
		lastTokenEndByte:            d.lastTokenEndByte,
		lastTokenValid:              d.lastTokenValid,
		externalTokenStart:          append([]byte(nil), d.externalTokenStart...),
		externalTokenEnd:            append([]byte(nil), d.externalTokenEnd...),
		extZeroPos:                  d.extZeroPos,
		extZeroState:                d.extZeroState,
		extZeroTried:                append([]bool(nil), d.extZeroTried...),
		zeroWidthPos:                d.zeroWidthPos,
		zeroWidthCount:              d.zeroWidthCount,
	}
	if d.hasExternalScanner && d.language != nil && d.language.ExternalScanner != nil {
		if cap(buf) != externalScannerSerializationBufferSize {
			if cap(buf) > 0 {
				clear(buf[:cap(buf)])
			}
			buf = make([]byte, externalScannerSerializationBufferSize)
		} else {
			buf = buf[:externalScannerSerializationBufferSize]
			clear(buf)
		}
		if n := d.language.ExternalScanner.Serialize(d.externalPayload, buf); n > 0 {
			s.externalPayload = buf[:n]
		}
	}
	return s, buf
}

func (s dfaRelexSnapshot) restore(d *dfaTokenSource) {
	d.lexer.pos = s.lexerPos
	d.lexer.row = s.lexerRow
	d.lexer.col = s.lexerCol
	d.lexer.includedRangeIdx = s.lexerRangeIdx
	d.lexer.failTokenStartPos = s.failTokenStartPos
	d.lexer.failTokenStartRow = s.failTokenStartRow
	d.lexer.failTokenStartCol = s.failTokenStartCol
	d.lexer.failTokenStartRangeIdx = s.failTokenStartRangeIdx
	d.externalLookaheadEndByte = s.externalLookaheadEndByte
	if d.hasExternalScanner && d.language != nil && d.language.ExternalScanner != nil {
		d.language.ExternalScanner.Deserialize(d.externalPayload, s.externalPayload)
	}
	d.lastExternalTokenStartByte = s.lastExternalTokenStartByte
	d.lastExternalTokenEndByte = s.lastExternalTokenEndByte
	d.lastExternalTokenValid = s.lastExternalTokenValid
	d.lastExternalTokenWasExtra = s.lastExternalTokenWasExtra
	d.externalTokenEndSameAsStart = s.externalTokenEndSameAsStart
	d.lastTokenStartByte = s.lastTokenStartByte
	d.lastTokenEndByte = s.lastTokenEndByte
	d.lastTokenValid = s.lastTokenValid
	d.externalTokenStart = append(d.externalTokenStart[:0], s.externalTokenStart...)
	d.externalTokenEnd = append(d.externalTokenEnd[:0], s.externalTokenEnd...)
	d.extZeroPos = s.extZeroPos
	d.extZeroState = s.extZeroState
	d.extZeroTried = append(d.extZeroTried[:0], s.extZeroTried...)
	d.zeroWidthPos = s.zeroWidthPos
	d.zeroWidthCount = s.zeroWidthCount
}

func (d *dfaTokenSource) RelexFromTokenStart(tok Token) (Token, bool) {
	if !d.CanRelexFromTokenStart(tok) {
		return Token{}, false
	}
	snapshot := d.snapshotRelexState()
	next, ok := d.relexFromTokenStartInTransaction(tok)
	if !ok {
		snapshot.restore(d)
	}
	return next, ok
}

// relexFromTokenStartInTransaction re-reads tok without starting a transaction.
// The caller must restore its transaction when it rejects the result.
func (d *dfaTokenSource) relexFromTokenStartInTransaction(tok Token) (Token, bool) {
	target := int(tok.StartByte)
	d.beginRelexAt(target, tok.StartPoint)
	if DebugDFA.Load() {
		fmt.Printf("  RELEX from %d state=%d\n", tok.StartByte, d.state)
	}
	next := d.Next()
	if next.StartByte != tok.StartByte || next.StartPoint != tok.StartPoint {
		return Token{}, false
	}
	// A no-lookahead state closes the active parser branch. It does not
	// reinterpret a real token that still starts before the source end.
	if next.NoLookahead || (next.Symbol == 0 && target < len(d.lexer.source)) {
		return Token{}, false
	}
	if tok.ExternalScannerToken && next.ExternalScannerToken &&
		next.StartByte == tok.StartByte && next.EndByte == tok.EndByte {
		next.ExternalScannerStartByte = tok.ExternalScannerStartByte
	}
	return next, true
}

// relexRecoveryTokenFromSkippedPrefixInTransaction re-reads tok from its exact
// lexer-call start. It converts an aligned, invisible error-mode token only
// when state zero cannot act on that token. The caller owns rollback.
func (d *dfaTokenSource) relexRecoveryTokenFromSkippedPrefixInTransaction(tok Token, startByte uint32, startPoint Point) (Token, bool) {
	if !d.canRelexFromSkippedPrefix(tok) || !tok.lexerSkippedPrefix ||
		tok.lexerSkippedPrefixStart != startByte || startByte >= tok.StartByte ||
		d.lexer.pos != int(tok.EndByte) || d.lexer.row != tok.EndPoint.Row || d.lexer.col != tok.EndPoint.Column {
		return Token{}, false
	}
	d.beginRelexAt(int(startByte), startPoint)
	if DebugDFA.Load() {
		fmt.Printf("  RELEX skipped-prefix from %d state=%d\n", startByte, d.state)
	}
	next := d.Next()
	if next.StartByte < startByte || d.lexer.pos != int(next.EndByte) ||
		d.lexer.row != next.EndPoint.Row || d.lexer.col != next.EndPoint.Column {
		return Token{}, false
	}
	next.lexerErrorModeLexed = true
	if next.Symbol != errorSymbol {
		meta, known := d.symbolMetadata(next.Symbol)
		if !known || meta.Visible || next.Symbol != tok.Symbol || next.ExternalScannerToken ||
			d.lookupActionIndex == nil || d.lookupActionIndex(cErrorState, next.Symbol) != 0 ||
			next.StartByte != tok.StartByte || next.EndByte != tok.EndByte ||
			next.StartPoint != tok.StartPoint || next.EndPoint != tok.EndPoint {
			return Token{}, false
		}
		next.Symbol = errorSymbol
	}
	return next, true
}

// canRelexFromSkippedPrefix proves that the active source still owns the
// scanner state for tok. Internal tokens must leave that state unchanged.
// Checkpoint-capable scanners need representable start and live states.
// External scanners must provide representable start and live checkpoints.
func (d *dfaTokenSource) canRelexFromSkippedPrefix(tok Token) bool {
	if d == nil || d.lexer == nil || tok.ExternalScannerToken {
		return false
	}
	target := int(tok.StartByte)
	if target < 0 || target > len(d.lexer.source) {
		return false
	}
	if !d.hasExternalScanner {
		return true
	}
	if !d.lastTokenValid || d.lastTokenStartByte != tok.StartByte || d.lastTokenEndByte != tok.EndByte {
		return false
	}
	if !d.usesExternalCheckpoints {
		return false
	}
	return d.lastExternalTokenValid &&
		d.lastExternalTokenStartByte == tok.StartByte &&
		d.lastExternalTokenEndByte == tok.EndByte &&
		d.externalTokenEndSameAsStart &&
		len(d.externalTokenStart) > 0 &&
		len(d.captureExternalScannerStateInto(&d.externalCompare)) > 0
}

func (d *dfaTokenSource) beginRelexAt(target int, pt Point) {
	d.lexer.pos = target
	d.lexer.row = pt.Row
	d.lexer.col = pt.Column
	d.lexer.includedRangeIdx = 0
	d.lexer.normalizeIncludedPosition()
	if d.hasExternalScanner && d.usesExternalCheckpoints {
		d.restoreExternalScannerState(d.externalTokenStart)
	}
	d.lastExternalTokenValid = false
	d.lastExternalTokenWasExtra = false
	d.lastExternalTokenStartByte = 0
	d.lastExternalTokenEndByte = 0
	d.lastTokenStartByte = 0
	d.lastTokenEndByte = 0
	d.lastTokenValid = false
	d.externalLookaheadEndByte = 0
	if len(d.externalTokenEnd) > 0 {
		d.externalTokenEnd = d.externalTokenEnd[:0]
	}
	d.extZeroPos = -1
	d.extZeroState = 0
	if len(d.extZeroTried) > 0 {
		d.extZeroTried = d.extZeroTried[:0]
	}
	d.zeroWidthPos = -1
	d.zeroWidthCount = 0
}

func (d *dfaTokenSource) nextExternalToken() (Token, bool) {
	if d.language == nil || d.lookupActionIndex == nil {
		return Token{}, false
	}
	if len(d.language.ExternalSymbols) == 0 {
		return Token{}, false
	}

	anyValid := false
	states := d.glrStates
	if len(states) == 0 {
		d.singleState[0] = d.state
		states = d.singleState[:]
	}
	if tok, ok := d.nextGLRScoredExternalToken(states); ok {
		return tok, true
	}

	// Fast path (C-equivalent O(1)): a single active parser state indexes its
	// external-lex-state row directly, exactly as tree-sitter C derives
	// valid_external_tokens from external_lex_state. This avoids zeroing and
	// rebuilding d.externalValid on every token (the per-token cost the CPU
	// profile attributed to nextExternalToken). The row is read-only on this
	// path — the only writer below is the zero-width-retry block, whose guard
	// we exclude here — so referencing the shared table row is safe (the
	// GLR-scored path already passes raw rows straight to the scanner).
	var valid []bool
	// Check the cheap single-state gate first; only then compute the
	// zero-width-retry guard. GLR-heavy languages (multi-state) skip the guard
	// entirely instead of paying it on every external-token lookup.
	if len(states) == 1 && len(d.language.ExternalLexStates) > 0 &&
		!(d.language.Name != "yaml" && d.lexer.pos == d.extZeroPos && d.state == d.extZeroState && len(d.extZeroTried) > 0) {
		st := states[0]
		if int(st) < len(d.language.LexModes) {
			elsID := int(d.language.LexModes[st].ExternalLexState)
			if elsID < len(d.language.ExternalLexStates) {
				row := d.language.ExternalLexStates[elsID]
				for i := 0; i < len(row); i++ {
					if row[i] {
						anyValid = true
						break
					}
				}
				if !anyValid {
					return Token{}, false
				}
				valid = row
			}
		}
	}

	if valid == nil {
		externalSymbolCount := len(d.language.ExternalSymbols)
		if cap(d.externalValid) < externalSymbolCount {
			d.externalValid = make([]bool, externalSymbolCount)
		}
		valid = d.externalValid[:externalSymbolCount]

		// Compute valid external symbols as the union across all active GLR
		// stacks. Different stacks may be in different parser states with
		// different valid external tokens. The scanner needs to see the union
		// so it can produce tokens that any stack might need. Stacks that
		// can't use the resulting token will be pruned by the action phase.
		if len(d.language.ExternalLexStates) == 0 && len(d.externalValidMaskByState) > 0 {
			var mask uint64
			for _, st := range states {
				if int(st) >= len(d.externalValidMaskByState) {
					continue
				}
				mask |= d.externalValidMaskByState[int(st)]
			}
			if mask != 0 {
				anyValid = true
			}
			for i := range valid {
				valid[i] = mask&(uint64(1)<<uint(i)) != 0
			}
		} else if len(d.language.ExternalLexStates) > 0 {
			for i := range valid {
				valid[i] = false
			}
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
			for i := range valid {
				valid[i] = false
			}
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
			for i := range valid {
				valid[i] = false
			}
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
	if d.shouldDeferSwiftOptionalGenericCloseToDFA(valid, states) {
		return Token{}, false
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
		tok = d.attachTokenLookaheadFrontier(tok, true)
		d.trackZeroWidthExternalToken(tok)
		d.lexer.pos = int(tok.EndByte)
		d.lexer.row = tok.EndPoint.Row
		d.lexer.col = tok.EndPoint.Column
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
				tok = d.attachTokenLookaheadFrontier(tok, true)
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
	tok = d.attachTokenLookaheadFrontier(tok, false)
	tok.ExternalScannerToken = true
	tok.ExternalScannerStartByte = uint32(d.lexer.pos)
	if splitTok, endPos, endRow, endCol, ok := d.splitSwiftWideCloseAngleToken(tok, states); ok {
		splitTok = d.attachTokenLookaheadFrontier(splitTok, false)
		d.lexer.pos = endPos
		d.lexer.row = endRow
		d.lexer.col = endCol
		return splitTok, true
	}

	if dfaTok, endPos, endRow, endCol, ok := d.preferDFASemicolonOverJSXText(tok, states); ok {
		dfaTok = d.attachTokenLookaheadFrontier(dfaTok, false)
		d.lexer.pos = endPos
		d.lexer.row = endRow
		d.lexer.col = endCol
		return dfaTok, true
	}

	d.trackZeroWidthExternalToken(tok)

	d.lexer.pos = int(tok.EndByte)
	d.lexer.row = tok.EndPoint.Row
	d.lexer.col = tok.EndPoint.Column
	tok = d.attachTokenLookaheadFrontier(tok, false)
	return tok, true
}

// splitSwiftWideCloseAngleToken narrows a run of external `_custom_operator`
// close-angle characters (">>", ">>>", ">>>>", ...) down to a single `>` when
// the run is really N adjacent generic closers, e.g. the trailing `>>>` in
// `A<B<C<Int>>>`. Swift's external scanner has no notion of generic nesting:
// it greedily lexes any maximal run of `>` bytes as one custom-operator
// token, so only the parser's own angle-bracket bookkeeping can tell that
// run apart from a genuine multi-character user operator (Swift lets you
// declare `infix operator >>> : ...` and use it standalone, e.g. `x >>> y`).
//
// hasSwiftUnclosedAngleBefore is the load-bearing guard: it requires at
// least one unclosed '<' before this run. Without an open generic to close,
// `x >>> y` must be refused here and left as one _custom_operator token.
// This function only ever runs for language "swift" (guard below), so Java
// and JavaScript/TypeScript `>>>`/`>>>=` shift operators never reach it.
//
// Known limitation, tracked separately and not fixed here: a run like
// `>>>?` (close-angle run immediately followed by optional-chaining `?`)
// isn't handled by this split — the all-'>' check below refuses it as soon
// as it sees the `?`, so `A<B<C<Int>>>?` still needs its own follow-up fix.
func (d *dfaTokenSource) splitSwiftWideCloseAngleToken(tok Token, states []StateID) (Token, int, uint32, uint32, bool) {
	if d == nil || d.language == nil || d.language.Name != "swift" || d.lexer == nil ||
		tok.EndPoint.Row != tok.StartPoint.Row || tok.EndByte <= tok.StartByte+1 {
		return tok, 0, 0, 0, false
	}
	if len(tok.Text) == 0 {
		start, end := int(tok.StartByte), int(tok.EndByte)
		if start < 0 || end > len(d.lexer.source) {
			return tok, 0, 0, 0, false
		}
		tok.Text = bytesToStringNoCopy(d.lexer.source[start:end])
	}
	for i := 0; i < len(tok.Text); i++ {
		if tok.Text[i] != '>' {
			return tok, 0, 0, 0, false
		}
	}
	if !d.hasSwiftUnclosedAngleBefore(int(tok.StartByte)) {
		return tok, 0, 0, 0, false
	}
	gtSym, ok := d.bestActiveSymbolByName(">")
	if !ok || gtSym == 0 {
		return tok, 0, 0, 0, false
	}
	// Secondary safety net alongside the angle-depth gate above: if the
	// parser's own action tables treat the run as a more specific match kept
	// whole (as the custom operator) than as a lone '>', defer to that
	// reading. Mirrors the gtSym/shiftSym specificity comparison in
	// shouldSplitCompactCloseAngleToken.
	if d.activeActionSpecificity(gtSym) < d.activeActionSpecificity(tok.Symbol) {
		return tok, 0, 0, 0, false
	}
	for _, state := range states {
		if d.lookupActionIndex(state, gtSym) != 0 {
			tok.Symbol = gtSym
			tok.Text = tok.Text[:1]
			tok.EndByte = tok.StartByte + 1
			tok.EndPoint = Point{Row: tok.StartPoint.Row, Column: tok.StartPoint.Column + 1}
			// tok.ExternalScannerToken stays true (it was set by the caller
			// before this function ran): realTokenAttachmentGapIsParserPadding
			// (parser.go) relies on that flag to treat the gap between the
			// previous real token and this narrowed token as scanner padding
			// rather than a parse error.
			return tok, int(tok.EndByte), tok.EndPoint.Row, tok.EndPoint.Column, true
		}
	}
	return tok, 0, 0, 0, false
}

// hasSwiftUnclosedAngleBefore reports whether an unclosed '<' opens before
// byte pos in the current statement/scope. This is the same "are we inside a
// generic argument list" heuristic hasUnclosedAngleBefore uses.
// The scan stops at a statement/scope boundary (; { } ( )) so an operator in
// one statement is never mistaken for a generic closer left open by an
// unrelated, already-closed earlier statement.
func (d *dfaTokenSource) hasSwiftUnclosedAngleBefore(pos int) bool {
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

// shouldDeferSwiftOptionalGenericCloseToDFA reports that the byte pair at the
// lexer position is `>?` and that at least one active state resolves the DFA's
// plain `>` candidate by reducing a production (closing an open generic
// argument list), not merely shifting it. A shift-only match means some other
// live GLR stack reads `>` as the start of an unrelated construct (for
// example a comparison operator continuing across a line break); deferring
// there would starve every stack of the external `_custom_operator` token
// and turn a harmless trailing `>?` into a full parse failure. Requiring a
// reduce restricts deferral to states where `>` genuinely closes a
// `type_arguments` list, matching the C tree-sitter behavior of splitting the
// generic close from the following `?` only in that context.
//
// Known limitation: this only recognizes the exact `>?` byte pair. A run of
// closing angle brackets before the `?` (for example `A<B<Int>>?`) still
// combines into one external `_custom_operator` token and is out of scope for
// this fix; see the follow-up issue for the `>`-run-then-`?` family.
func (d *dfaTokenSource) shouldDeferSwiftOptionalGenericCloseToDFA(valid []bool, states []StateID) bool {
	if d == nil || d.language == nil || d.lexer == nil || d.lookupActionIndex == nil || d.language.Name != "swift" {
		return false
	}
	pos := d.lexer.pos
	if pos < 0 || pos+1 >= len(d.lexer.source) || d.lexer.source[pos] != '>' || d.lexer.source[pos+1] != '?' {
		return false
	}
	customOperator := false
	for i, isValid := range valid {
		if !isValid || i >= len(d.language.ExternalSymbols) {
			continue
		}
		if d.symbolName(d.language.ExternalSymbols[i]) == "_custom_operator" {
			customOperator = true
			break
		}
	}
	if !customOperator {
		return false
	}
	if len(states) == 0 {
		var single [1]StateID
		single[0] = d.state
		states = single[:]
	}
	for _, state := range states {
		cand, endPos, _, _ := d.scanPreferredTokenForState(state)
		if cand.Symbol == 0 || cand.StartByte != uint32(pos) || endPos <= pos || d.symbolName(cand.Symbol) != ">" {
			continue
		}
		actionIdx := d.lookupActionIndex(state, cand.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(d.language.ParseActions) {
			continue
		}
		if d.swiftCloseAngleActionReduces(actionIdx) {
			return d.hasSwiftUnclosedAngleBefore(pos)
		}
	}
	return false
}

// swiftCloseAngleActionReduces reports that the parse action entry contains a
// reduce action. A reduce here means the `>` candidate closes an open
// production (a generic argument list) rather than merely shifting into a
// state that expects further tokens.
func (d *dfaTokenSource) swiftCloseAngleActionReduces(actionIdx uint16) bool {
	for _, a := range d.language.ParseActions[actionIdx].Actions {
		if a.Type == ParseActionReduce {
			return true
		}
	}
	return false
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

func (d *dfaTokenSource) canRetryAfterUnusableZeroWidthExternal(tok Token) bool {
	if d == nil || d.language == nil || d.lexer == nil || tok.EndByte > tok.StartByte {
		return false
	}
	if d.allowRepeatedZeroWidthExternalSymbol(tok.Symbol) {
		return false
	}
	idx := d.externalSymbolIndex(tok.Symbol)
	if idx < 0 {
		return false
	}
	if d.lexer.pos != int(tok.EndByte) {
		return false
	}
	// Retry a zero-width external symbol at most once per (position, state).
	// If we've already tried this symbol here, retrying again loops forever
	// when the external scanner keeps re-emitting it (observed with
	// markdown_inline). Return false so Next falls through to the byte-skip
	// path, which guarantees forward progress instead of spinning.
	if d.extZeroPos == d.lexer.pos && d.extZeroState == d.state &&
		idx < len(d.extZeroTried) && d.extZeroTried[idx] {
		return false
	}
	d.trackZeroWidthExternalToken(tok)
	return true
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
		tok.ExternalScannerToken = true
		tok.ExternalScannerStartByte = uint32(startPos)

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
			(support == bestSupport && specificity > bestSpecificity) ||
			(support == bestSupport && specificity == bestSpecificity && primaryHasAction && !bestPrimaryHasAction) ||
			(support == bestSupport && specificity == bestSpecificity && primaryHasAction == bestPrimaryHasAction && originActions > bestOriginActions) ||
			(support == bestSupport && specificity == bestSpecificity && primaryHasAction == bestPrimaryHasAction && originActions == bestOriginActions &&
				primaryELS == elsID && primaryELS != bestELS) ||
			(support == bestSupport && specificity == bestSpecificity && primaryHasAction == bestPrimaryHasAction && originActions == bestOriginActions &&
				primaryELS == bestELS && tok.StartByte < bestTok.StartByte) ||
			(support == bestSupport && specificity == bestSpecificity && primaryHasAction == bestPrimaryHasAction && originActions == bestOriginActions &&
				primaryELS == bestELS && tok.StartByte == bestTok.StartByte && tok.EndByte > bestTok.EndByte) ||
			(support == bestSupport && specificity == bestSpecificity && primaryHasAction == bestPrimaryHasAction && originActions == bestOriginActions &&
				primaryELS == bestELS && tok.StartByte == bestTok.StartByte &&
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
	tok.ExternalScannerToken = true
	tok.ExternalScannerStartByte = uint32(startPos)

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
	recordFrontier := func(scannerLexer *ExternalLexer) {
		if scannerLexer == nil {
			return
		}
		scannerLexer.lookaheadEndByte = maxUint32(scannerLexer.lookaheadEndByte, scannerLexer.lookaheadEndByteAtCursor())
		d.externalLookaheadEndByte = maxUint32(d.externalLookaheadEndByte, scannerLexer.lookaheadEndByte)
		scannerLexer.lookaheadEndByte = maxUint32(scannerLexer.lookaheadEndByte, d.externalLookaheadEndByte)
	}
	var snapshot []byte
	retainFailureState := d.externalScannerRetainsStateOnScanFailure()
	// Retention takes precedence if a scanner reports both capabilities.
	// A retained mutation is incompatible with the preservation claim.
	preserveFailureState := !retainFailureState && d.externalScannerPreservesStateOnScanFailure()
	restoreFailedScan := func() {
		if !preserveFailureState && !retainFailureState {
			d.restoreExternalScannerState(snapshot)
		}
	}
	if preserveFailureState {
		foundToken := RunExternalScanner(d.language, d.externalPayload, el, valid)
		recordFrontier(el)
		if foundToken {
			return true
		}
		if !el.hasResult {
			restoreFailedScan()
			return false
		}
		snapshot = d.captureExternalScannerStateInto(&d.externalRetrySnap)
	} else {
		snapshot = d.captureExternalScannerStateInto(&d.externalRetrySnap)
		foundToken := RunExternalScanner(d.language, d.externalPayload, el, valid)
		recordFrontier(el)
		if foundToken {
			return true
		}
		if !el.hasResult {
			restoreFailedScan()
			return false
		}
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
			restoreFailedScan()
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
			restoreFailedScan()
			return false
		}

		d.restoreExternalScannerState(snapshot)
		retryLexer := &d.externalRetryLexer
		retryLexer.reset(d.lexer.source, d.lexer.pos, d.lexer.row, d.lexer.col)
		foundToken := RunExternalScanner(d.language, d.externalPayload, retryLexer, masked)
		recordFrontier(retryLexer)
		if foundToken {
			*el = *retryLexer
			return true
		}
		if !retryLexer.hasResult {
			restoreFailedScan()
			return false
		}
		*el = *retryLexer
	}
}

func (d *dfaTokenSource) externalScannerPreservesStateOnScanFailure() bool {
	if d == nil || d.language == nil || d.language.ExternalScanner == nil {
		return false
	}
	preserving, ok := d.language.ExternalScanner.(FailurePreservingExternalScanner)
	return ok && preserving.PreservesStateOnScanFailure()
}

func (d *dfaTokenSource) externalScannerRetainsStateOnScanFailure() bool {
	if d == nil || d.language == nil || d.language.ExternalScanner == nil {
		return false
	}
	retaining, ok := d.language.ExternalScanner.(FailureStateRetainingExternalScanner)
	return ok && retaining.RetainsStateOnScanFailure()
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
	end := d.externalTokenEnd
	if d.externalTokenEndSameAsStart {
		end = d.externalTokenStart
	}
	// CheckpointedExternalScanner reserves an empty serialization for an
	// unavailable state. Both boundary snapshots must be exact: accepting one
	// missing endpoint would make distinct unrepresentable states compare as
	// the same empty byte slice during incremental authentication.
	if len(d.externalTokenStart) == 0 || len(end) == 0 {
		return externalScannerCheckpoint{}, 0, 0, false
	}
	return externalScannerCheckpoint{
		start: d.externalTokenStart,
		end:   end,
	}, d.lastExternalTokenStartByte, d.lastExternalTokenEndByte, true
}

func (d *dfaTokenSource) externalScannerStateMatches(snapshot []byte) bool {
	if d == nil {
		return len(snapshot) == 0
	}
	current := d.captureExternalScannerStateInto(&d.externalCompare)
	return bytes.Equal(current, snapshot)
}

// externalScannerStateAtLookaheadStartMatches authenticates a reuse boundary
// against the scanner state before the current lookahead was lexed. Parser
// reuse runs after fetching that lookahead, so comparing with the live payload
// would instead compare the candidate's start against the token's end state.
// Scanners such as YAML update position state on nearly every token and make
// that distinction observable.
func (d *dfaTokenSource) externalScannerStateAtLookaheadStartMatches(snapshot []byte, lookaheadStart uint32) bool {
	if d == nil {
		return len(snapshot) == 0
	}
	if d.lastExternalTokenValid && d.lastExternalTokenStartByte == lookaheadStart {
		return bytes.Equal(d.externalTokenStart, snapshot)
	}
	return d.externalScannerStateMatches(snapshot)
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
	case "_virtual_end_section":
		return d.language.Name == "elm"
	case "_dedent":
		// Indentation scanners may legitimately emit several zero-width
		// DEDENTs at one byte while unwinding nested blocks.  Bend uses the
		// same Tree-sitter contract as GDScript; applying the global cap here
		// drops a structural dedent and changes the winning parse branch.
		return d.language.Name == "gdscript" || d.language.Name == "bend"
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

func (d *dfaTokenSource) promoteKeyword(tok Token) (Token, bool) {
	if d.language == nil {
		return tok, false
	}
	if tok.Symbol == 0 {
		return tok, false
	}
	if len(d.language.KeywordLexStates) == 0 {
		return tok, false
	}
	if d.language.KeywordCaptureToken == 0 {
		return tok, false
	}
	if tok.Symbol != d.language.KeywordCaptureToken {
		return tok, false
	}
	if tok.EndByte <= tok.StartByte {
		return tok, false
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
			return tok, false
		}
	}

	start := int(tok.StartByte)
	end := int(tok.EndByte)
	if start < 0 || end < start || end > len(d.lexer.source) {
		return tok, false
	}
	keywordSource := d.lexer.source[start:end]
	if !d.language.keywordLexCouldMatch(d.lexer.source, start, end) {
		upper, ok := d.sqlUppercaseKeywordSource(keywordSource)
		if !ok || !d.language.keywordLexCouldMatch(upper, 0, len(upper)) {
			return tok, false
		}
		keywordSource = upper
	}

	kwTok, ok := d.lexKeywordSource(keywordSource)
	if !ok && d.language.Name == "sql" {
		if upper, upperOK := d.sqlUppercaseKeywordSource(d.lexer.source[start:end]); upperOK && d.language.keywordLexCouldMatch(upper, 0, len(upper)) {
			kwTok, ok = d.lexKeywordSource(upper)
		}
	}
	if !ok {
		return tok, false
	}
	if d.language.Name == "rust" && int(kwTok.Symbol) < len(d.language.SymbolNames) && d.language.SymbolNames[kwTok.Symbol] == "default" {
		if end < len(d.lexer.source) && d.lexer.source[end] == ':' {
			return tok, true
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
						return tok, true // reserved - don't promote
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
				tok.isKeyword = true
				return tok, false
			}
		}
		if !kwHasAction && idHasAction {
			return tok, true // no active stack needs the keyword
		}
		if d.shouldPreferJavaScriptTypeScriptContextualIdentifier(tok, kwTok, kwHasAction, idHasAction) {
			return tok, true
		}
		if d.shouldPreferSwiftMemberIdentifier(tok, kwTok) {
			return tok, true
		}
	}

	tok.Symbol = kwTok.Symbol
	tok.isKeyword = true
	return tok, false
}

func (d *dfaTokenSource) promoteActiveLiteralForCurrentState(tok Token, scanStartPos int, scanStartRow, scanStartCol uint32) Token {
	if d == nil || d.language == nil || d.lexer == nil || d.lookupActionIndex == nil || tok.Symbol == 0 {
		return tok
	}
	meta, ok := d.symbolMetadata(tok.Symbol)
	if ok && !meta.Named {
		return tok
	}
	text := tok.Text
	if text == "" {
		start := int(tok.StartByte)
		end := int(tok.EndByte)
		if start < 0 || end <= start || end > len(d.lexer.source) {
			return tok
		}
		text = bytesToStringNoCopy(d.lexer.source[start:end])
	}
	// Exact shape pre-filter: promotion below requires an ANONYMOUS token
	// symbol whose name equals text (the loop skips Named candidates and
	// name-mismatched candidates). When no anonymous token name shares text's
	// first byte and length, that is impossible, so skip the per-token
	// string-map lookup — the dominant cost for ordinary identifiers.
	if !d.language.anonymousTokenNameShapePossible(text) {
		return tok
	}
	if text == "" || !isIdentifierLikeLiteralText(text) || d.symbolName(tok.Symbol) == text {
		return tok
	}
	for _, sym := range d.language.TokenSymbolsByName(text) {
		if sym == 0 || sym == tok.Symbol || d.symbolName(sym) != text {
			continue
		}
		if symMeta, ok := d.symbolMetadata(sym); ok && symMeta.Named {
			continue
		}
		if !d.activeStateCanPromoteLiteral(tok, sym, scanStartPos, scanStartRow, scanStartCol) {
			continue
		}
		tok.Symbol = sym
		return tok
	}
	return tok
}

func (d *dfaTokenSource) activeStateCanPromoteLiteral(tok Token, sym Symbol, scanStartPos int, scanStartRow, scanStartCol uint32) bool {
	if d == nil || d.lookupActionIndex == nil {
		return false
	}
	if d.stateCanPromoteLiteral(d.state, tok, sym, scanStartPos, scanStartRow, scanStartCol) {
		return true
	}
	for i, state := range d.glrStates {
		if state == d.state || d.priorGLRState(i, state) {
			continue
		}
		if d.stateCanPromoteLiteral(state, tok, sym, scanStartPos, scanStartRow, scanStartCol) {
			return true
		}
	}
	return false
}

func (d *dfaTokenSource) stateCanPromoteLiteral(state StateID, tok Token, sym Symbol, scanStartPos int, scanStartRow, scanStartCol uint32) bool {
	if d == nil || d.lookupActionIndex == nil || sym == 0 {
		return false
	}
	if d.lookupActionIndex(state, sym) == 0 {
		return false
	}
	if d.isImmediateSymbol(sym) && int(tok.StartByte) > scanStartPos {
		return false
	}
	return d.stateLexModeProducesSameSpan(state, tok, scanStartPos, scanStartRow, scanStartCol)
}

func (d *dfaTokenSource) stateLexModeProducesSameSpan(state StateID, tok Token, scanStartPos int, scanStartRow, scanStartCol uint32) bool {
	if d == nil || d.lexer == nil {
		return false
	}
	savedPos := d.lexer.pos
	savedRow := d.lexer.row
	savedCol := d.lexer.col
	savedRangeIdx := d.lexer.includedRangeIdx
	d.lexer.pos = scanStartPos
	d.lexer.row = scanStartRow
	d.lexer.col = scanStartCol
	d.lexer.includedRangeIdx = 0
	d.lexer.normalizeIncludedPosition()
	rawTok, rawEndPos, _, _ := d.scanRawPreferredTokenForState(state)
	d.lexer.pos = savedPos
	d.lexer.row = savedRow
	d.lexer.col = savedCol
	d.lexer.includedRangeIdx = savedRangeIdx
	if rawTok.Symbol == 0 || rawTok.Symbol == errorSymbol {
		return false
	}
	if rawTok.StartByte != tok.StartByte || rawTok.EndByte != tok.EndByte {
		return false
	}
	return rawEndPos == int(tok.EndByte)
}

func (d *dfaTokenSource) isImmediateSymbol(sym Symbol) bool {
	if d == nil || d.language == nil || len(d.language.ImmediateTokens) == 0 {
		return false
	}
	idx := int(sym)
	return idx >= 0 && idx < len(d.language.ImmediateTokens) && d.language.ImmediateTokens[idx]
}

func isIdentifierLikeLiteralText(text string) bool {
	for i, r := range text {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return text != ""
}

func (d *dfaTokenSource) shouldPreferSwiftMemberIdentifier(tok, kwTok Token) bool {
	if d == nil || d.language == nil || d.language.Name != "swift" {
		return false
	}
	if tok.Symbol == kwTok.Symbol {
		return false
	}
	return d.isAfterSwiftMemberDot(int(tok.StartByte))
}

func (d *dfaTokenSource) demoteSwiftMemberKeyword(tok Token) Token {
	if !d.shouldDemoteSwiftMemberKeyword(tok) {
		return tok
	}
	if sym, ok := d.swiftSimpleIdentifierSymbol(); ok {
		tok.Symbol = sym
	}
	return tok
}

func (d *dfaTokenSource) shouldDemoteSwiftMemberKeyword(tok Token) bool {
	if d == nil || d.language == nil || d.language.Name != "swift" || tok.Symbol == 0 {
		return false
	}
	if !d.isAfterSwiftMemberDot(int(tok.StartByte)) {
		return false
	}
	start := int(tok.StartByte)
	end := int(tok.EndByte)
	if start < 0 || end <= start || end > len(d.lexer.source) {
		return false
	}
	text := tok.Text
	if text == "" {
		text = bytesToStringNoCopy(d.lexer.source[start:end])
	}
	if d.symbolName(tok.Symbol) != text {
		return false
	}
	// symbolName(tok.Symbol) == text is trivially satisfied by ANY anonymous
	// literal token (its symbol name in tree-sitter is its own spelling), not
	// just genuine reserved keywords like "self"/"default"/"class". Without
	// this guard, punctuation tokens (e.g. the closing '"' of a string
	// literal) get misclassified as identifiers whenever they happen to
	// immediately follow a raw '.' byte in the source -- including a '.'
	// that is itself just the last character of the preceding string's
	// content (isAfterSwiftMemberDot has no lexical-context awareness).
	// Swift's compiled grammar doesn't route these reserved words through
	// the generic KeywordLexStates/KeywordCaptureToken promotion table (it's
	// empty for swift), so require the spelling itself to be word-shaped
	// (identifier-like) instead: real Swift keywords ("self", "default",
	// "class", ...) are always alphabetic words, never punctuation.
	return swiftTextIsWordShaped(text)
}

// swiftTextIsWordShaped reports whether text looks like an identifier/keyword
// spelling (starts with a letter or underscore, contains only
// letters/digits/underscores) as opposed to punctuation or an operator.
func swiftTextIsWordShaped(text string) bool {
	if text == "" {
		return false
	}
	for i, r := range text {
		if r == '_' || unicode.IsLetter(r) {
			continue
		}
		if i > 0 && unicode.IsDigit(r) {
			continue
		}
		return false
	}
	first := rune(text[0])
	return first == '_' || unicode.IsLetter(first)
}

func (d *dfaTokenSource) swiftSimpleIdentifierSymbol() (Symbol, bool) {
	if d == nil || d.language == nil {
		return 0, false
	}
	if d.language.KeywordCaptureToken != 0 {
		return d.language.KeywordCaptureToken, true
	}
	for i, name := range d.language.SymbolNames {
		if strings.Contains(name, "XID_Start") && strings.Contains(name, "XID_Continue") {
			return Symbol(i), true
		}
	}
	for i := range d.language.SymbolNames {
		sym := Symbol(i)
		meta, ok := d.symbolMetadata(sym)
		if ok && meta.Named && d.symbolName(sym) == "simple_identifier" {
			return sym, true
		}
	}
	return 0, false
}

func (d *dfaTokenSource) isAfterSwiftMemberDot(start int) bool {
	if d == nil || d.lexer == nil {
		return false
	}
	if start <= 0 || start > len(d.lexer.source) {
		return false
	}
	i := start - 1
	for i >= 0 {
		switch d.lexer.source[i] {
		case ' ', '\t', '\r':
			i--
			continue
		}
		return d.lexer.source[i] == '.'
	}
	return false
}

func (d *dfaTokenSource) lexKeywordSource(source []byte) (Token, bool) {
	if d == nil || d.language == nil {
		return Token{}, false
	}
	states := d.language.KeywordLexStates
	if len(states) == 0 {
		return Token{}, false
	}

	curState := int32(0)
	scanPos := 0
	acceptPos := -1
	acceptSymbol := Symbol(0)
	acceptSkip := false
	acceptPriorityBest := int16(32767)
	eofHops := 0
	asciiTable := d.language.KeywordLexAsciiTable()

	for {
		if curState < 0 || int(curState) >= len(states) {
			break
		}
		st := &states[int(curState)]

		if st.AcceptToken > 0 || st.Skip {
			newPrio := st.AcceptPriority
			if acceptPos < 0 || newPrio < acceptPriorityBest || (newPrio == acceptPriorityBest && scanPos > acceptPos) {
				acceptPos = scanPos
				acceptSymbol = st.AcceptToken
				acceptSkip = st.Skip
				acceptPriorityBest = newPrio
			}
		}

		if scanPos >= len(source) {
			if st.EOF >= 0 && eofHops <= len(states) {
				curState = int32(st.EOF)
				eofHops++
				continue
			}
			break
		}
		eofHops = 0

		b := source[scanPos]
		var r rune
		size := 1
		if b < 0x80 {
			r = rune(b)
		} else {
			r, size = utf8.DecodeRune(source[scanPos:])
		}

		nextState := int32(-1)
		skipTransition := false
		if b < 0x80 && asciiTable != nil && int(curState) < len(asciiTable) {
			v := asciiTable[curState][b]
			if v != lexAsciiNoMatch {
				nextState = v & ^lexAsciiSkipBit
				skipTransition = v&lexAsciiSkipBit != 0
			}
		} else {
			for i := range st.Transitions {
				tr := &st.Transitions[i]
				if r >= tr.Lo && r <= tr.Hi {
					nextState = int32(tr.NextState)
					skipTransition = tr.Skip
					break
				}
			}
		}
		skipTransition = skipTransition && nextState >= 0
		if nextState < 0 && st.Default >= 0 {
			nextState = int32(st.Default)
			skipTransition = false
		}
		if nextState < 0 {
			break
		}

		scanPos += size
		if skipTransition {
			// The keyword DFA can carry its own SKIP transitions — e.g. a
			// leading-whitespace loop on the start state — whenever the
			// grammar's word token is itself permissive enough to have
			// absorbed leading extras that a plain identifier-shaped word
			// token never would (tree-sitter's own generated keyword lexer
			// does the same: it SKIPs those characters before matching the
			// literal). Discard the tentative match found so far and keep
			// scanning past the skipped run for the keyword itself.
			acceptPos = -1
			acceptSymbol = 0
			acceptSkip = false
		}
		curState = nextState
	}

	// The keyword must own the rest of the captured source exactly: any
	// leading run consumed by SKIP transitions above is already discarded
	// (never part of accept tracking), and requiring acceptPos to reach
	// len(source) rejects leftover trailing bytes that the keyword didn't
	// consume — the captured span is a keyword only when it is "skippable
	// prefix + exactly one literal", nothing more.
	if acceptSymbol == 0 || acceptSkip || acceptPos != len(source) {
		return Token{}, false
	}
	return Token{
		Symbol:  acceptSymbol,
		EndByte: uint32(acceptPos),
	}, true
}

func (d *dfaTokenSource) sqlUppercaseKeywordSource(source []byte) ([]byte, bool) {
	if d == nil || d.language == nil || d.language.Name != "sql" || len(source) == 0 {
		return nil, false
	}
	if cap(d.sqlKeywordScratch) < len(source) {
		d.sqlKeywordScratch = make([]byte, len(source))
	} else {
		d.sqlKeywordScratch = d.sqlKeywordScratch[:len(source)]
	}
	changed := false
	for i, b := range source {
		switch {
		case b >= 'a' && b <= 'z':
			d.sqlKeywordScratch[i] = b - ('a' - 'A')
			changed = true
		case (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_':
			d.sqlKeywordScratch[i] = b
		default:
			d.sqlKeywordScratch = d.sqlKeywordScratch[:0]
			return nil, false
		}
	}
	if !changed {
		return nil, false
	}
	return d.sqlKeywordScratch, true
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

// externalScannerQuiescent reports whether the external scanner currently holds
// no serialized state. By the tree-sitter external-scanner contract, Serialize
// must persist every bit of scanner state that can change a later scan(); a
// zero-byte serialization therefore means the scanner carries no state forward.
// The block-splice O(1) byte skip (reuseNode, incremental.go) relies on this:
// with an empty serialized state the live scanner behaves for every future
// scan() exactly like a fresh empty scanner, so repositioning the byte cursor
// without re-lexing the reused span cannot change the next real token. A
// language with no external scanner is trivially quiescent.
func (d *dfaTokenSource) externalScannerQuiescent() bool {
	if d == nil || !d.hasExternalScanner {
		return true
	}
	return len(d.captureExternalScannerStateInto(&d.externalCompare)) == 0
}
