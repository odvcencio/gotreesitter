package gotreesitter

import "unicode/utf8"

type includedLexerCursor struct {
	pos      int
	row      uint32
	col      uint32
	rangeIdx int
}

// setIncludedRanges moves the internal DFA cursor to the first selected byte.
func (l *Lexer) setIncludedRanges(ranges []Range) {
	l.includedRanges = ranges
	l.includedRangeIdx = 0
	l.normalizeIncludedPosition()
}

func (l *Lexer) normalizeIncludedPosition() {
	if l == nil || len(l.includedRanges) == 0 {
		return
	}
	cursor := includedLexerCursor{
		pos:      l.pos,
		row:      l.row,
		col:      l.col,
		rangeIdx: l.includedRangeIdx,
	}
	l.normalizeIncludedCursor(&cursor)
	l.pos = cursor.pos
	l.row = cursor.row
	l.col = cursor.col
	l.includedRangeIdx = cursor.rangeIdx
}

func (l *Lexer) normalizeIncludedCursor(cursor *includedLexerCursor) {
	if l == nil || cursor == nil || len(l.includedRanges) == 0 {
		return
	}
	if cursor.rangeIdx < 0 || cursor.rangeIdx > len(l.includedRanges) {
		cursor.rangeIdx = 0
	}
	if cursor.rangeIdx > 0 && cursor.rangeIdx <= len(l.includedRanges) {
		_, previousEnd := l.includedRangeBounds(cursor.rangeIdx - 1)
		if cursor.pos < previousEnd {
			cursor.rangeIdx = 0
		}
	}
	for cursor.rangeIdx < len(l.includedRanges) {
		start, end := l.includedRangeBounds(cursor.rangeIdx)
		if end <= start {
			cursor.rangeIdx++
			continue
		}
		if cursor.pos <= start {
			r := l.includedRanges[cursor.rangeIdx]
			cursor.pos = start
			cursor.row = r.StartPoint.Row
			cursor.col = r.StartPoint.Column
			return
		}
		if cursor.pos < end {
			return
		}
		cursor.rangeIdx++
	}

	endPos, endPoint := l.includedEOFPosition()
	cursor.pos = endPos
	cursor.row = endPoint.Row
	cursor.col = endPoint.Column
}

// includedPointAtPosition returns the logical point for a source byte. It
// applies the configured range points and uses the previous range endpoint for
// a gap between ranges. The boolean is false before the first range.
func (l *Lexer) includedPointAtPosition(pos int) (Point, bool) {
	if l == nil || len(l.includedRanges) == 0 || pos < 0 || pos > len(l.source) {
		return Point{}, false
	}
	var previousEnd Point
	hasPrevious := false
	for i := range l.includedRanges {
		range_ := l.includedRanges[i]
		start, end := l.includedRangeBounds(i)
		if end <= start {
			continue
		}
		if pos < start {
			if hasPrevious {
				return previousEnd, true
			}
			return Point{}, false
		}
		if pos == start {
			return range_.StartPoint, true
		}
		if pos < end {
			return advancePointByBytes(range_.StartPoint, l.source[start:pos]), true
		}
		previousEnd = range_.EndPoint
		hasPrevious = true
	}
	if hasPrevious {
		return previousEnd, true
	}
	return Point{}, false
}

func (l *Lexer) includedRangeBounds(index int) (int, int) {
	if l == nil || index < 0 || index >= len(l.includedRanges) {
		return 0, 0
	}
	r := l.includedRanges[index]
	start := len(l.source)
	if uint64(r.StartByte) < uint64(len(l.source)) {
		start = int(r.StartByte)
	}
	end := len(l.source)
	if uint64(r.EndByte) < uint64(len(l.source)) {
		end = int(r.EndByte)
	}
	return start, end
}

func (l *Lexer) includedEOFPosition() (int, Point) {
	if l == nil || len(l.includedRanges) == 0 {
		if l == nil {
			return 0, Point{}
		}
		return len(l.source), advancePointByBytes(Point{}, l.source)
	}
	for i := len(l.includedRanges) - 1; i >= 0; i-- {
		start, end := l.includedRangeBounds(i)
		if end <= start {
			continue
		}
		last := l.includedRanges[i]
		if uint64(last.EndByte) <= uint64(len(l.source)) {
			return end, last.EndPoint
		}
		return len(l.source), advancePointByBytes(Point{}, l.source)
	}
	return len(l.source), advancePointByBytes(Point{}, l.source)
}

func (l *Lexer) includedRangeIndexForPosition(pos int) int {
	if l == nil || len(l.includedRanges) == 0 {
		return 0
	}
	for i := range l.includedRanges {
		_, end := l.includedRangeBounds(i)
		if pos < end {
			return i
		}
	}
	return len(l.includedRanges)
}

func (l *Lexer) atLogicalEOF() bool {
	if l == nil {
		return true
	}
	if len(l.includedRanges) == 0 {
		return l.pos >= len(l.source)
	}
	return l.includedRangeIdx >= len(l.includedRanges)
}

func (l *Lexer) skipOneIncludedRune() {
	if l == nil {
		return
	}
	cursor := includedLexerCursor{
		pos:      l.pos,
		row:      l.row,
		col:      l.col,
		rangeIdx: l.includedRangeIdx,
	}
	if _, _, ok := l.advanceIncludedCursor(&cursor); !ok {
		return
	}
	l.pos = cursor.pos
	l.row = cursor.row
	l.col = cursor.col
	l.includedRangeIdx = cursor.rangeIdx
}

func (l *Lexer) advanceIncludedCursor(cursor *includedLexerCursor) (rune, int, bool) {
	l.normalizeIncludedCursor(cursor)
	if cursor.rangeIdx >= len(l.includedRanges) {
		return 0, 0, false
	}
	_, end := l.includedRangeBounds(cursor.rangeIdx)
	if cursor.pos >= end {
		return 0, 0, false
	}
	b := l.source[cursor.pos]
	r := rune(b)
	size := 1
	if b >= utf8.RuneSelf {
		r, size = utf8.DecodeRune(l.source[cursor.pos:end])
	}
	cursor.pos += size
	if r == '\n' {
		cursor.row++
		cursor.col = 0
	} else {
		cursor.col += uint32(size)
	}
	if cursor.pos >= end {
		cursor.rangeIdx++
		l.normalizeIncludedCursor(cursor)
	}
	return r, size, true
}

// includedMarkEnd ends a boundary token at the prior range end.
func (l *Lexer) includedMarkEnd(cursor includedLexerCursor) (int, Point) {
	if cursor.rangeIdx > 0 && cursor.rangeIdx < len(l.includedRanges) {
		start, _ := l.includedRangeBounds(cursor.rangeIdx)
		if cursor.pos == start {
			previous := l.includedRanges[cursor.rangeIdx-1]
			_, previousEnd := l.includedRangeBounds(cursor.rangeIdx - 1)
			if uint64(previous.EndByte) <= uint64(len(l.source)) {
				return previousEnd, previous.EndPoint
			}
			return previousEnd, advancePointByBytes(Point{}, l.source[:previousEnd])
		}
	}
	return cursor.pos, Point{Row: cursor.row, Column: cursor.col}
}

// tokenText excludes source gaps when one DFA token crosses selected ranges.
func (l *Lexer) tokenText(start, end int) string {
	if start >= end {
		return ""
	}
	if len(l.includedRanges) == 0 {
		return bytesToStringNoCopy(l.source[start:end])
	}
	segmentCount := 0
	total := 0
	firstStart := 0
	firstEnd := 0
	for i := range l.includedRanges {
		rangeStart, rangeEnd := l.includedRangeBounds(i)
		segmentStart := max(start, rangeStart)
		segmentEnd := min(end, rangeEnd)
		if segmentEnd <= segmentStart {
			continue
		}
		if segmentCount == 0 {
			firstStart = segmentStart
			firstEnd = segmentEnd
		}
		segmentCount++
		total += segmentEnd - segmentStart
	}
	if segmentCount == 0 {
		return ""
	}
	if segmentCount == 1 {
		return bytesToStringNoCopy(l.source[firstStart:firstEnd])
	}
	text := make([]byte, 0, total)
	for i := range l.includedRanges {
		rangeStart, rangeEnd := l.includedRangeBounds(i)
		segmentStart := max(start, rangeStart)
		segmentEnd := min(end, rangeEnd)
		if segmentEnd > segmentStart {
			text = append(text, l.source[segmentStart:segmentEnd]...)
		}
	}
	return bytesToStringNoCopy(text)
}

// scanIncluded keeps the DFA state when the source cursor crosses a range gap.
func (l *Lexer) scanIncluded(startState uint32, startPos int, startRow, startCol uint32) (Token, bool) {
	workCountRecordRawMainLexerInvocation()
	curState := int32(startState)
	if curState < 0 || int(curState) >= len(l.states) {
		return Token{}, false
	}

	scanCursor := includedLexerCursor{
		pos:      startPos,
		row:      startRow,
		col:      startCol,
		rangeIdx: l.includedRangeIdx,
	}
	l.normalizeIncludedCursor(&scanCursor)
	startPos = scanCursor.pos
	startRow = scanCursor.row
	startCol = scanCursor.col
	tokenStart := scanCursor
	skippedPrefix := false

	acceptProgressPos := -1
	acceptPos := -1
	acceptRow := uint32(0)
	acceptCol := uint32(0)
	acceptRangeIdx := 0
	acceptStart := includedLexerCursor{}
	acceptSymbol := Symbol(0)
	acceptSkip := false
	acceptPriorityBest := int16(32767)

	eofHops := 0
	for {
		if curState < 0 || int(curState) >= len(l.states) {
			break
		}
		st := &l.states[int(curState)]

		if st.AcceptToken > 0 || st.Skip {
			isImmediate := st.AcceptToken > 0 && int(st.AcceptToken) < len(l.immediateTokens) && l.immediateTokens[st.AcceptToken]
			skippedWhitespace := tokenStart.pos > startPos
			zeroWidthVisible := st.AcceptToken > 0 && scanCursor.pos == tokenStart.pos && !l.allowsZeroWidthToken(st.AcceptToken)
			if !(isImmediate && skippedWhitespace) && !zeroWidthVisible {
				newPrio := st.AcceptPriority
				if acceptPos < 0 || newPrio < acceptPriorityBest || (newPrio == acceptPriorityBest && scanCursor.pos > acceptProgressPos) {
					acceptProgressPos = scanCursor.pos
					var endPoint Point
					acceptPos, endPoint = l.includedMarkEnd(scanCursor)
					acceptRow = endPoint.Row
					acceptCol = endPoint.Column
					acceptRangeIdx = scanCursor.rangeIdx
					acceptStart = tokenStart
					acceptSymbol = st.AcceptToken
					acceptSkip = st.Skip
					acceptPriorityBest = newPrio
				}
			}
		}

		if scanCursor.rangeIdx >= len(l.includedRanges) {
			if st.EOF >= 0 && eofHops <= len(l.states) {
				curState = int32(st.EOF)
				eofHops++
				continue
			}
			break
		}
		eofHops = 0

		b := l.source[scanCursor.pos]
		var r rune
		if b < utf8.RuneSelf {
			r = rune(b)
		} else {
			_, end := l.includedRangeBounds(scanCursor.rangeIdx)
			r, _ = utf8.DecodeRune(l.source[scanCursor.pos:end])
		}
		nextState := int32(-1)
		skipTransition := false
		if b < utf8.RuneSelf && l.asciiTable != nil && int(curState) < len(l.asciiTable) {
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
		skipTransition = skipTransition && nextState >= 0
		if nextState < 0 && st.Default >= 0 {
			nextState = int32(st.Default)
			skipTransition = false
		}
		if nextState < 0 {
			break
		}

		if _, _, ok := l.advanceIncludedCursor(&scanCursor); !ok {
			break
		}
		if skipTransition {
			skippedPrefix = true
			tokenStart = scanCursor
			acceptProgressPos = -1
			acceptPos = -1
			acceptSymbol = 0
			acceptSkip = false
		}
		curState = nextState
	}

	if acceptPos < 0 && eofHops > 0 {
		acceptProgressPos = scanCursor.pos
		var endPoint Point
		acceptPos, endPoint = l.includedMarkEnd(scanCursor)
		acceptRow = endPoint.Row
		acceptCol = endPoint.Column
		acceptRangeIdx = scanCursor.rangeIdx
		acceptStart = tokenStart
		acceptSymbol = 0
		acceptSkip = true
	}

	lookaheadEndByte := l.lookaheadEndByteAt(scanCursor.pos, scanCursor.rangeIdx < len(l.includedRanges))
	if lookaheadEndByte < uint32(maxInt(acceptPos, 0)) {
		lookaheadEndByte = uint32(maxInt(acceptPos, 0))
	}

	if acceptPos < 0 {
		l.failTokenStartPos = tokenStart.pos
		l.failTokenStartRow = tokenStart.row
		l.failTokenStartCol = tokenStart.col
		l.failTokenStartRangeIdx = tokenStart.rangeIdx
		return Token{lexerLookaheadEndByte: lookaheadEndByte}, false
	}

	l.pos = acceptPos
	l.row = acceptRow
	l.col = acceptCol
	l.includedRangeIdx = acceptRangeIdx

	if acceptSkip {
		return Token{
			StartByte:               uint32(acceptStart.pos),
			EndByte:                 uint32(acceptPos),
			StartPoint:              Point{Row: acceptStart.row, Column: acceptStart.col},
			EndPoint:                Point{Row: acceptRow, Column: acceptCol},
			lexerSkippedPrefix:      skippedPrefix,
			lexerSkippedPrefixStart: uint32(startPos),
			lexerLookaheadEndByte:   lookaheadEndByte,
		}, true
	}

	return Token{
		Symbol:                  acceptSymbol,
		Text:                    l.tokenText(acceptStart.pos, acceptPos),
		StartByte:               uint32(acceptStart.pos),
		EndByte:                 uint32(acceptPos),
		StartPoint:              Point{Row: acceptStart.row, Column: acceptStart.col},
		EndPoint:                Point{Row: acceptRow, Column: acceptCol},
		lexerSkippedPrefix:      skippedPrefix,
		lexerSkippedPrefixStart: uint32(startPos),
		lexerInternalDFALexed:   true,
		lexerLookaheadEndByte:   lookaheadEndByte,
	}, true
}
