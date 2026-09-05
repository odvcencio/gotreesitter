package gotreesitter

import "unicode/utf8"

// ExternalLexer is the scanner-facing lexer API used by external scanners.
// It mirrors the essential tree-sitter scanner API: lookahead, advance,
// mark_end, and result_symbol.
type ExternalLexer struct {
	source []byte

	startPos int
	pos      int
	endPos   int

	startPoint Point
	point      Point
	endPoint   Point
	endMarked  bool

	// advancedContent is set when Advance(false) is called at least once.
	// It is only used to preserve an explicit MarkEnd position when later
	// Advance(true) calls move startPos past that mark.
	advancedContent bool

	resultSymbol Symbol
	hasResult    bool

	// lookaheadEndByte records the largest lexer frontier observed while this
	// scanner attempt ran. The token source carries it across scanner retries.
	lookaheadEndByte uint32
	// A scan owns this observer. Value-copy rollback must not erase reads.
	readFrontier *externalReadFrontier
}

type externalReadFrontier struct {
	lookahead uint32
	examined  uint32
}

func (l *ExternalLexer) clearSource() {
	observer := l.readFrontier
	if observer != nil {
		*observer = externalReadFrontier{}
	}
	*l = ExternalLexer{readFrontier: observer}
}

func (l *ExternalLexer) reset(source []byte, pos int, row, col uint32) {
	pt := Point{Row: row, Column: col}
	l.source = source
	l.startPos = pos
	l.pos = pos
	l.endPos = pos
	l.startPoint = pt
	l.point = pt
	l.endPoint = pt
	l.endMarked = false
	l.advancedContent = false
	l.resultSymbol = 0
	l.hasResult = false
	l.lookaheadEndByte = 0
}

func newExternalLexer(source []byte, pos int, row, col uint32) *ExternalLexer {
	l := &ExternalLexer{}
	l.reset(source, pos, row, col)
	return l
}

// Lookahead returns the current rune or 0 at EOF.
func (l *ExternalLexer) Lookahead() rune {
	l.recordReadFrontier()
	if l.pos >= len(l.source) {
		return 0
	}
	b := l.source[l.pos]
	if b < utf8.RuneSelf {
		return rune(b)
	}
	r, _ := utf8.DecodeRune(l.source[l.pos:])
	return r
}

// Previous returns the rune immediately before the current lexer position.
// It returns 0 only at the start of the source (pos <= 0) or when pos is out
// of range. For invalid UTF-8 immediately before pos, it returns
// utf8.RuneError (U+FFFD), matching utf8.DecodeLastRune -- never 0.
//
// Warning: a scanner that reads Previous() looks behind the current scan
// position, which is a byte, not a grammar token. A caller cannot assume the
// preceding byte belongs to the token that logically precedes this position:
// extras (comments, whitespace) the core lexer matches between two scanner
// invocations are invisible to the scanner, so the byte immediately before
// pos can be the tail of a comment rather than the true previous token. A
// scanner that relies on Previous() to decide something as history-sensitive
// as this must track token-boundary state itself across calls (see
// grammars/swift_scanner.go) rather than trust a single raw byte read.
// This lookbehind also breaks external-scanner quiescence obligation 2 (see
// external_scanner_quiescence.go): Scan must depend only on bytes at or after
// the current position plus the valid-symbol set, not on bytes before it. A
// scanner that calls Previous() must not claim StatelessExternalScanner.
func (l *ExternalLexer) Previous() rune {
	if l == nil || l.pos <= 0 || l.pos > len(l.source) {
		return 0
	}
	r, _ := utf8.DecodeLastRune(l.source[:l.pos])
	return r
}

// Advance consumes one rune. When skip is true, consumed bytes are excluded
// from the token span (scanner whitespace skipping behavior).
func (l *ExternalLexer) Advance(skip bool) {
	if l.pos >= len(l.source) {
		l.recordReadFrontier()
		return
	}

	b := l.source[l.pos]
	r := rune(b)
	size := 1
	if b >= utf8.RuneSelf {
		r, size = utf8.DecodeRune(l.source[l.pos:])
	}
	l.pos += size
	l.recordReadFrontier()
	if r == '\n' {
		l.point.Row++
		l.point.Column = 0
	} else {
		l.point.Column += uint32(size)
	}

	if skip {
		l.startPos = l.pos
		l.startPoint = l.point
		// Note: endPos/endPoint are NOT updated here.  In C tree-sitter,
		// ts_lexer_advance(skip=true) only moves token_start_position, not
		// token_end_position.  MarkEnd() is the sole way to advance endPos.
		// This matters for scanners (e.g. YAML) that mark the end before
		// skipping whitespace and then return a zero-width token: the parser
		// must re-position at the mark, not past the skipped bytes.
	} else {
		l.advancedContent = true
	}
}

// AdvanceSpaces consumes consecutive ASCII spaces and returns the number of
// bytes consumed. It is equivalent to repeated Advance(skip) while Lookahead is
// a space, but avoids per-byte rune decoding in external scanners that skip
// indentation runs.
func (l *ExternalLexer) AdvanceSpaces(skip bool) int {
	if l == nil {
		return 0
	}
	l.recordReadFrontier()
	if l.pos >= len(l.source) || l.source[l.pos] != ' ' {
		return 0
	}
	start := l.pos
	for l.pos < len(l.source) && l.source[l.pos] == ' ' {
		l.pos++
	}
	l.recordReadFrontier()
	n := l.pos - start
	l.point.Column += uint32(n)
	if skip {
		l.startPos = l.pos
		l.startPoint = l.point
	} else {
		l.advancedContent = true
	}
	return n
}

// AdvanceUntilNewline consumes bytes up to, but not including, '\n' or EOF and
// returns the number of bytes consumed. For non-newline bytes, Advance updates
// Column by the UTF-8 width, which is equal to the byte count for the whole
// consumed span.
func (l *ExternalLexer) AdvanceUntilNewline(skip bool) int {
	if l == nil {
		return 0
	}
	l.recordReadFrontier()
	if l.pos >= len(l.source) || l.source[l.pos] == '\n' {
		return 0
	}
	start := l.pos
	for l.pos < len(l.source) && l.source[l.pos] != '\n' {
		l.pos++
	}
	l.recordReadFrontier()
	n := l.pos - start
	l.point.Column += uint32(n)
	if skip {
		l.startPos = l.pos
		l.startPoint = l.point
	} else {
		l.advancedContent = true
	}
	return n
}

// MarkEnd marks the current scanner position as the token end.
func (l *ExternalLexer) MarkEnd() {
	l.endPos = l.pos
	l.endPoint = l.point
	l.endMarked = true
}

// SetResultSymbol sets the token symbol to emit when Scan returns true.
func (l *ExternalLexer) SetResultSymbol(sym Symbol) {
	l.resultSymbol = sym
	l.hasResult = true
}

// lookaheadEndByteAtCursor mirrors ts_lexer_finish for an external scanner.
// Tree-sitter records one byte beyond the current cursor, plus four bytes when
// the current lookahead is an invalid UTF-8 sequence.
func (l *ExternalLexer) lookaheadEndByteAtCursor() uint32 {
	if l == nil {
		return 0
	}
	pos := l.pos
	if pos < 0 {
		pos = 0
	}
	frontier := uint64(pos) + 1
	if pos < len(l.source) {
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

func (l *ExternalLexer) recordReadFrontier() {
	if l.readFrontier != nil {
		frontier := l.lookaheadEndByteAtCursor()
		l.readFrontier.lookahead = maxUint32(l.readFrontier.lookahead, frontier)
		l.readFrontier.examined = maxUint32(l.readFrontier.examined, tokenInvariantExaminedEnd(l.source, frontier))
	}
}

// Column returns the current column (0-based) at the scanner cursor.
func (l *ExternalLexer) Column() uint32 {
	return l.point.Column
}

// GetColumn returns the current column (0-based) at the scanner cursor.
//
// Deprecated: use Column.
func (l *ExternalLexer) GetColumn() uint32 {
	return l.Column()
}

// HasPreviousBytes reports whether the bytes immediately before the scanner
// cursor match text. External scanners use this to guard context-sensitive
// content tokens when merged parser states expose them too broadly.
func (l *ExternalLexer) HasPreviousBytes(text string) bool {
	if text == "" {
		return true
	}
	start := l.pos - len(text)
	if start < 0 {
		return false
	}
	for i := 0; i < len(text); i++ {
		if l.source[start+i] != text[i] {
			return false
		}
	}
	return true
}

func (l *ExternalLexer) token() (Token, bool) {
	if !l.hasResult {
		return Token{}, false
	}
	endPos := l.endPos
	endPoint := l.endPoint
	if !l.endMarked {
		// C tree-sitter calls mark_end(current_position) during lexer_finish
		// when a successful external scan never marked an end explicitly.
		// That means skip-only scans default to the cursor after skipped
		// whitespace, not back at the scan start.
		endPos = l.pos
		endPoint = l.point
	}
	// When endPos < startPos the scanner marked a position before skip
	// advanced startPos past it.  This yields a zero-width token at the
	// mark position — the parser will re-position the lexer there so the
	// skipped bytes are re-encountered on the next scan, matching C
	// tree-sitter semantics.
	if endPos < l.startPos {
		lookaheadEndByte := maxUint32(l.lookaheadEndByte, l.lookaheadEndByteAtCursor())
		if lookaheadEndByte < uint32(endPos) {
			lookaheadEndByte = uint32(endPos)
		}
		return Token{
			Symbol:                l.resultSymbol,
			StartByte:             uint32(endPos),
			EndByte:               uint32(endPos),
			StartPoint:            endPoint,
			EndPoint:              endPoint,
			lexerLookaheadEndByte: lookaheadEndByte,
		}, true
	}

	lookaheadEndByte := maxUint32(l.lookaheadEndByte, l.lookaheadEndByteAtCursor())
	if lookaheadEndByte < uint32(endPos) {
		lookaheadEndByte = uint32(endPos)
	}
	return Token{
		Symbol:                l.resultSymbol,
		Text:                  bytesToStringNoCopy(l.source[l.startPos:endPos]),
		StartByte:             uint32(l.startPos),
		EndByte:               uint32(endPos),
		StartPoint:            l.startPoint,
		EndPoint:              endPoint,
		lexerLookaheadEndByte: lookaheadEndByte,
	}, true
}
