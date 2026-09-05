package gotreesitter

import (
	"bytes"
	"unicode/utf8"
	"unsafe"
)

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// Point is a row/column position in source text.
type Point struct {
	Row    uint32
	Column uint32
}

// Token is a lexed token with position info.
type Token struct {
	Symbol     Symbol
	Text       string
	StartByte  uint32
	EndByte    uint32
	StartPoint Point
	EndPoint   Point
	Missing    bool
	// lexerLookaheadEndByte is the farthest byte inspected while selecting a
	// lexed token. Synthetic missing tokens retain that token's frontier.
	lexerLookaheadEndByte uint32
	// missingStack identifies the stack position before padding for a
	// synthetic recovery token. It is never set on lexed input.
	missingStackByte       uint32
	missingStackPoint      Point
	missingDependencyExact bool
	// NoLookahead marks a synthetic EOF used to force EOF-table reductions
	// without consuming input, matching tree-sitter's lex_state = -1.
	NoLookahead bool
	// ExternalScannerToken marks tokens produced by an external scanner.
	// ExternalScannerStartByte is the byte offset where that scanner call
	// began, before scanner-side skip advances moved StartByte forward.
	ExternalScannerToken     bool
	ExternalScannerStartByte uint32
	// lexerSkippedPrefix records that the DFA consumed one or more skip
	// transitions before producing this token.
	lexerSkippedPrefix      bool
	lexerSkippedPrefixStart uint32
	// lexerErrorModeLexed proves that the active DFA source produced this
	// recovery token while parser state zero selected the error lex mode.
	lexerErrorModeLexed bool
	// lexerErrorRunLexed identifies bytes that no internal error-mode token
	// could consume. It does not certify the external scanner's state.
	lexerErrorRunLexed bool
	// lexerInternalDFALexed proves that Lexer.scan accepted this token from
	// the internal DFA. External, generated, missing, and EOF tokens omit it.
	lexerInternalDFALexed bool
	// isKeyword mirrors C's Subtree.is_keyword bit (subtree.h:131): the
	// keyword-capture word token was re-lexed as a keyword and the parser
	// adopted that keyword symbol (parser.c:645-668). False for every other
	// token, including a word token the keyword re-lex did not adopt.
	isKeyword bool
}

func bytesToStringNoCopy(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// Lexer tokenizes source text using a table-driven DFA.
type Lexer struct {
	states           []LexState
	asciiTable       [][128]int32 // ASCII fast-path transition table (nil = not available)
	source           []byte
	pos              int
	row              uint32
	col              uint32
	includedRanges   []Range
	includedRangeIdx int
	immediateTokens  []bool // symbol IDs that are token.immediate(); rejected after whitespace skip
	zeroWidthTokens  []bool // symbol IDs whose terminal pattern can intentionally match empty input

	// Set by scan on failure: where the token attempt began after the DFA
	// consumed skip (whitespace) transitions. errorRunToken uses this so an
	// unlexable run starts after legitimately skipped whitespace, like C.
	failTokenStartPos      int
	failTokenStartRow      uint32
	failTokenStartCol      uint32
	failTokenStartRangeIdx int

	// The grammar's most permissive lex state (LexModes[0], C's ERROR_STATE
	// mode). NextWithErrorRuns only emits an error-run token when even this
	// state cannot lex at a position — mirroring C, which retries failed
	// lexes in error mode before skipping characters into an error subtree.
	errorRunLexState    uint32
	hasErrorRunLexState bool

	// errorModeRetry enables the full C ts_parser__lex failure behavior for
	// NextWithErrorRuns (faithful C error-recovery port, parser_recover_c.go):
	// when the requested lex state fails, re-lex from the call's start
	// position in errorRunLexState and return THAT token — C switches
	// lex_mode to the ERROR_STATE mode and continues, surfacing real (often
	// invisible) tokens like scheme's block_comment_token1 which the recovery
	// then absorbs as hidden error-region leaves. Without the flag, a failed
	// scan falls through to the error-run check / silent skip exactly as
	// before.
	errorModeRetry bool
}

// NewLexer creates a new Lexer that will tokenize source using the given
// DFA state table.
func NewLexer(states []LexState, source []byte) *Lexer {
	return &Lexer{
		states: states,
		source: source,
	}
}

// Next lexes the next token starting from the given lex state index.
// It automatically skips tokens from states where Skip=true (whitespace).
// Returns a zero-Symbol token with StartByte==EndByte at EOF.
func (l *Lexer) Next(startState uint32) Token {
	return l.next(startState, false)
}

// NextWithErrorRuns behaves like Next, except that bytes for which no
// accepting DFA state exists are not silently dropped: the whole unlexable
// run is consumed and returned as an errorSymbol token. This mirrors C
// ts_parser__lex, which surfaces skipped characters as an error subtree —
// the run starts after any whitespace the DFA legitimately skipped and ends
// at the first position where a token can be lexed (or EOF).
func (l *Lexer) NextWithErrorRuns(startState uint32) Token {
	return l.next(startState, true)
}

func (l *Lexer) next(startState uint32, emitErrorRuns bool) Token {
	return l.nextWithFrontier(startState, emitErrorRuns, 0)
}

// nextWithFrontier carries the largest lexer frontier observed by every
// attempt in one C-style lex call. C updates lookahead_end_byte after each
// internal, external, and error-mode attempt, including attempts that fail.
func (l *Lexer) nextWithFrontier(startState uint32, emitErrorRuns bool, lookaheadEndByte uint32) Token {
	l.normalizeIncludedPosition()
	l.skipLeadingBOM()
	// C ts_parser__lex resets to the lex call's start position (before any
	// whitespace the failed attempt skipped) when it switches to error-mode
	// lexing; capture it for the errorModeRetry branch below.
	callStartPos, callStartRow, callStartCol := l.pos, l.row, l.col
	callStartRangeIdx := l.includedRangeIdx
	skippedPrefix := false
	for {
		// EOF check.
		if l.atLogicalEOF() {
			lookaheadEndByte = maxUint32(lookaheadEndByte, l.lookaheadEndByteAt(l.pos, false))
			return Token{
				StartByte:             uint32(l.pos),
				EndByte:               uint32(l.pos),
				StartPoint:            Point{Row: l.row, Column: l.col},
				EndPoint:              Point{Row: l.row, Column: l.col},
				lexerLookaheadEndByte: lookaheadEndByte,
			}
		}

		tokenStartPos := l.pos
		tokenStartRow := l.row
		tokenStartCol := l.col

		tok, ok := l.scan(startState, tokenStartPos, tokenStartRow, tokenStartCol)
		lookaheadEndByte = maxUint32(lookaheadEndByte, tok.lexerLookaheadEndByte)
		if ok {
			if tok.Symbol == 0 {
				// Skip token (whitespace). Verify the lexer actually
				// advanced past the skipped content to prevent an
				// infinite loop on zero-width skip matches.
				if l.pos <= tokenStartPos {
					skippedPrefix = false
					l.skipOneRune()
				} else {
					skippedPrefix = true
				}
				continue
			}
			if skippedPrefix {
				tok.lexerSkippedPrefix = true
				tok.lexerSkippedPrefixStart = uint32(callStartPos)
			}
			tok.lexerLookaheadEndByte = lookaheadEndByte
			return tok
		}
		skippedPrefix = false

		if emitErrorRuns && l.hasErrorRunLexState && l.errorModeRetry && startState != l.errorRunLexState {
			// Faithful C error-recovery port: ts_parser__lex retries a failed
			// lex in the ERROR_STATE mode (LexModes[0]) from the call's start
			// position and returns its token; characters are skipped into an
			// errorSymbol run only when even error mode cannot lex. The
			// recursive call has startState == errorRunLexState, so it takes
			// the error-run branch below on failure instead of recursing.
			l.pos, l.row, l.col = callStartPos, callStartRow, callStartCol
			l.includedRangeIdx = callStartRangeIdx
			return l.nextWithFrontier(l.errorRunLexState, true, lookaheadEndByte)
		}
		if emitErrorRuns && l.hasErrorRunLexState && !l.canLexAt(l.errorRunLexState, tokenStartPos, tokenStartRow, tokenStartCol, &lookaheadEndByte) {
			return l.errorRunToken(&lookaheadEndByte)
		}
		// No accepting state was found. Skip one rune as error recovery.
		l.skipOneRune()
	}
}

func (l *Lexer) skipLeadingBOM() {
	if l == nil || l.pos != 0 || !bytes.HasPrefix(l.source, utf8BOM) {
		return
	}
	l.pos = len(utf8BOM)
	l.col = uint32(len(utf8BOM))
}

// canLexAt reports whether the DFA can lex a token (or whitespace skip)
// starting at the given position, without moving the lexer.
func (l *Lexer) canLexAt(lexState uint32, pos int, row, col uint32, frontier *uint32) bool {
	savedPos, savedRow, savedCol := l.pos, l.row, l.col
	savedRangeIdx := l.includedRangeIdx
	tok, ok := l.scan(lexState, pos, row, col)
	if frontier != nil {
		*frontier = maxUint32(*frontier, tok.lexerLookaheadEndByte)
	}
	l.pos, l.row, l.col = savedPos, savedRow, savedCol
	l.includedRangeIdx = savedRangeIdx
	return ok
}

// errorRunToken consumes the unlexable run beginning at the last failed
// scan's token start (i.e. after any whitespace the DFA skipped) and returns
// it as an errorSymbol token. The run ends at the first position where the
// error-mode lex state can lex again, matching C's character-by-character
// error skipping.
func (l *Lexer) errorRunToken(frontier *uint32) Token {
	// Position the lexer at the real error start: scan records where the
	// token attempt began after consuming skip (whitespace) transitions.
	if l.failTokenStartPos > l.pos && l.failTokenStartPos <= len(l.source) {
		l.pos = l.failTokenStartPos
		l.row = l.failTokenStartRow
		l.col = l.failTokenStartCol
		l.includedRangeIdx = l.failTokenStartRangeIdx
	}
	l.normalizeIncludedPosition()
	if l.atLogicalEOF() {
		// Only whitespace remained: this is end-of-input, not an error run.
		if frontier != nil {
			*frontier = maxUint32(*frontier, l.lookaheadEndByteAt(l.pos, false))
		}
		return Token{
			StartByte:             uint32(l.pos),
			EndByte:               uint32(l.pos),
			StartPoint:            Point{Row: l.row, Column: l.col},
			EndPoint:              Point{Row: l.row, Column: l.col},
			lexerLookaheadEndByte: frontierValue(frontier),
		}
	}
	startPos, startRow, startCol := l.pos, l.row, l.col

	l.skipOneRune()
	for !l.atLogicalEOF() {
		if l.canLexAt(l.errorRunLexState, l.pos, l.row, l.col, frontier) {
			break
		}
		l.skipOneRune()
	}
	if frontier != nil {
		*frontier = maxUint32(*frontier, l.lookaheadEndByteAt(l.pos, false))
	}
	return Token{
		Symbol:                errorSymbol,
		lexerErrorRunLexed:    true,
		Text:                  l.tokenText(startPos, l.pos),
		StartByte:             uint32(startPos),
		EndByte:               uint32(l.pos),
		StartPoint:            Point{Row: startRow, Column: startCol},
		EndPoint:              Point{Row: l.row, Column: l.col},
		lexerLookaheadEndByte: frontierValue(frontier),
	}
}

// scan runs the DFA from the given start state and position. It returns
// a token and true if an accepting state was reached, or false if not.
// On a skip (whitespace) match, it returns a zero-Symbol token and true.
func (l *Lexer) scan(startState uint32, startPos int, startRow, startCol uint32) (Token, bool) {
	if len(l.includedRanges) != 0 {
		return l.scanIncluded(startState, startPos, startRow, startCol)
	}
	return l.scanContiguous(startState, startPos, startRow, startCol)
}

func (l *Lexer) scanContiguous(startState uint32, startPos int, startRow, startCol uint32) (Token, bool) {
	// work-count-assembly: raw main-lexer invocation seam
	workCountRecordRawMainLexerInvocation()
	curState := int32(startState)
	if curState < 0 || int(curState) >= len(l.states) {
		return Token{}, false
	}

	scanPos := startPos
	scanRow := startRow
	scanCol := startCol
	tokenStartPos := startPos
	tokenStartRow := startRow
	tokenStartCol := startCol
	skippedPrefix := false

	// Track the last accepting state.
	acceptPos := -1
	acceptRow := uint32(0)
	acceptCol := uint32(0)
	acceptStartPos := 0
	acceptStartRow := uint32(0)
	acceptStartCol := uint32(0)
	acceptSymbol := Symbol(0)
	acceptSkip := false
	acceptPriorityBest := int16(32767) // max int16; any real priority beats this

	eofHops := 0
	// Walk the DFA in the same style as tree-sitter START_LEXER/ADVANCE/SKIP.
	for {
		if curState < 0 || int(curState) >= len(l.states) {
			break
		}
		st := &l.states[int(curState)]

		if st.AcceptToken > 0 || st.Skip {
			// Reject immediate tokens that matched after whitespace was
			// consumed. Immediate tokens must match at the original position.
			isImmediate := st.AcceptToken > 0 && int(st.AcceptToken) < len(l.immediateTokens) && l.immediateTokens[st.AcceptToken]
			skippedWhitespace := tokenStartPos > startPos
			zeroWidthVisible := st.AcceptToken > 0 && scanPos == tokenStartPos && !l.allowsZeroWidthToken(st.AcceptToken)
			if !(isImmediate && skippedWhitespace) && !zeroWidthVisible {
				newPrio := st.AcceptPriority
				if acceptPos < 0 || newPrio < acceptPriorityBest || (newPrio == acceptPriorityBest && scanPos > acceptPos) {
					acceptPos = scanPos
					acceptRow = scanRow
					acceptCol = scanCol
					acceptStartPos = tokenStartPos
					acceptStartRow = tokenStartRow
					acceptStartCol = tokenStartCol
					acceptSymbol = st.AcceptToken
					acceptSkip = st.Skip
					acceptPriorityBest = newPrio
				}
			}
		}

		if scanPos >= len(l.source) {
			if st.EOF >= 0 && eofHops <= len(l.states) {
				curState = int32(st.EOF)
				eofHops++
				continue
			}
			break
		}
		eofHops = 0

		b := l.source[scanPos]
		var r rune
		var size int
		if b < 0x80 {
			r = rune(b)
			size = 1
		} else {
			r, size = utf8.DecodeRune(l.source[scanPos:])
		}
		nextState := int32(-1)
		skipTransition := false
		if b < 0x80 && l.asciiTable != nil && int(curState) < len(l.asciiTable) {
			// ASCII fast-path: O(1) lookup instead of linear scan.
			v := l.asciiTable[curState][b]
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
		// Default transitions are treated as non-skipping.
		skipTransition = skipTransition && nextState >= 0
		if nextState < 0 && st.Default >= 0 {
			nextState = int32(st.Default)
			skipTransition = false
		}
		if nextState < 0 {
			break
		}

		scanPos += size
		if r == '\n' {
			scanRow++
			scanCol = 0
		} else {
			scanCol += uint32(size)
		}

		if skipTransition {
			// tree-sitter SKIP(state) consumes and resets token start.
			skippedPrefix = true
			tokenStartPos = scanPos
			tokenStartRow = scanRow
			tokenStartCol = scanCol
			acceptPos = -1
			acceptSymbol = 0
			acceptSkip = false
		}

		curState = nextState
	}

	if acceptPos < 0 && eofHops > 0 {
		// The DFA walk reached true EOF mid-scan and exhausted the per-state
		// EOF-transition chain (tree-sitter's universal "if (eof) ADVANCE(...)"
		// escape hatch, e.g. C case87 -> case99 in a compiled grammar's ts_lex)
		// without any state along the way registering a real accept. In C
		// tree-sitter, the chain's terminal state always calls
		// ACCEPT_TOKEN(ts_builtin_sym_end) before END_STATE(), so a partially
		// matched multi-character token (like AWK's "\\\n" line-continuation
		// extras, which SKIPs the backslash before discovering there's no
		// following newline) is silently absorbed as trivia at true EOF
		// instead of failing the lex. Mirror that: accept an empty/skip token
		// at the position reached (after any SKIP-consumed prefix). This only
		// fires when nothing else was accepted along the path, so it can't
		// override a real token match (e.g. an identifier ending at EOF
		// accepts before its state's EOF check would ever run).
		acceptPos = scanPos
		acceptRow = scanRow
		acceptCol = scanCol
		acceptStartPos = tokenStartPos
		acceptStartRow = tokenStartRow
		acceptStartCol = tokenStartCol
		acceptSymbol = 0
		acceptSkip = true
	}

	lookaheadEndByte := l.lookaheadEndByteAt(scanPos, true)
	if lookaheadEndByte < uint32(maxInt(acceptPos, 0)) {
		lookaheadEndByte = uint32(maxInt(acceptPos, 0))
	}

	if acceptPos < 0 {
		l.failTokenStartPos = tokenStartPos
		l.failTokenStartRow = tokenStartRow
		l.failTokenStartCol = tokenStartCol
		l.failTokenStartRangeIdx = 0
		return Token{lexerLookaheadEndByte: lookaheadEndByte}, false
	}

	// Rewind (or advance) to the accept position.
	l.pos = acceptPos
	l.row = acceptRow
	l.col = acceptCol

	if acceptSkip {
		// Return a zero-Symbol token to signal "skip".
		return Token{
			StartByte:               uint32(acceptStartPos),
			EndByte:                 uint32(acceptPos),
			StartPoint:              Point{Row: acceptStartRow, Column: acceptStartCol},
			EndPoint:                Point{Row: acceptRow, Column: acceptCol},
			lexerSkippedPrefix:      skippedPrefix,
			lexerSkippedPrefixStart: uint32(startPos),
			lexerLookaheadEndByte:   lookaheadEndByte,
		}, true
	}

	return Token{
		Symbol:                  acceptSymbol,
		Text:                    bytesToStringNoCopy(l.source[acceptStartPos:acceptPos]),
		StartByte:               uint32(acceptStartPos),
		EndByte:                 uint32(acceptPos),
		StartPoint:              Point{Row: acceptStartRow, Column: acceptStartCol},
		EndPoint:                Point{Row: acceptRow, Column: acceptCol},
		lexerSkippedPrefix:      skippedPrefix,
		lexerSkippedPrefixStart: uint32(startPos),
		lexerInternalDFALexed:   true,
		lexerLookaheadEndByte:   lookaheadEndByte,
	}, true
}

func tokenLookaheadEndByte(token Token) uint32 {
	if token.lexerLookaheadEndByte > token.EndByte {
		return token.lexerLookaheadEndByte
	}
	return token.EndByte
}

func maxUint32(left, right uint32) uint32 {
	if left >= right {
		return left
	}
	return right
}

func frontierValue(frontier *uint32) uint32 {
	if frontier == nil {
		return 0
	}
	return *frontier
}

// lookaheadEndByteAt mirrors ts_lexer_finish. The C lexer records one byte
// beyond the current cursor, plus four bytes when the current lookahead is an
// invalid UTF-8 sequence. Included-range EOF must not inspect the source gap.
func (l *Lexer) lookaheadEndByteAt(pos int, inspectInvalid bool) uint32 {
	if pos < 0 {
		pos = 0
	}
	frontier := uint64(pos) + 1
	if inspectInvalid && pos < len(l.source) {
		r, size := utf8.DecodeRune(l.source[pos:])
		if r == utf8.RuneError && size == 1 && l.source[pos] >= utf8.RuneSelf {
			frontier += 4
		}
	}
	if frontier > uint64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(frontier)
}

// skipOneRune advances the lexer position by one rune, updating row/column.
func (l *Lexer) skipOneRune() {
	if len(l.includedRanges) != 0 {
		l.skipOneIncludedRune()
		return
	}
	if l.pos >= len(l.source) {
		return
	}
	r, size := utf8.DecodeRune(l.source[l.pos:])
	l.pos += size
	if r == '\n' {
		l.row++
		l.col = 0
	} else {
		l.col += uint32(size)
	}
}

func (l *Lexer) allowsZeroWidthToken(sym Symbol) bool {
	if l == nil || len(l.zeroWidthTokens) == 0 {
		return true
	}
	return int(sym) < len(l.zeroWidthTokens) && l.zeroWidthTokens[sym]
}
