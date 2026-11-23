package ccore

import (
	"sort"
	"unicode/utf8"
)

// includedCursor keeps the read position separate from the token end.
// Jumping over a gap uses the next range's supplied point. Marking a token
// at that start records the preceding range's end instead (C/lexer.c).
type includedCursor struct {
	src    []byte
	ranges []Range
	pos    Length
	index  int
}

func newIncludedCursor(src []byte, ranges []Range, at Length) includedCursor {
	index := sort.Search(len(ranges), func(i int) bool { return ranges[i].EndByte > at.bytes })
	for index < len(ranges) && ranges[index].StartByte == ranges[index].EndByte {
		index++
	}
	c := includedCursor{src: src, ranges: ranges, pos: at, index: index}
	if index == len(ranges) {
		last := ranges[index-1]
		c.pos = NewLength(last.EndByte, last.EndRow, last.EndCol)
	} else if r := ranges[index]; at.bytes <= r.StartByte {
		c.pos = NewLength(r.StartByte, r.StartRow, r.StartCol)
	}
	return c
}

func (c *includedCursor) eof() bool {
	return c.index == len(c.ranges) || uint64(c.pos.bytes) >= uint64(len(c.src))
}

func (c *includedCursor) lookahead() (rune, uint32) {
	if c.eof() {
		return 0, 0
	}
	// C decodes against the input chunk, which may extend past the range.
	r, width := utf8.DecodeRune(c.src[c.pos.bytes:])
	if r == utf8.RuneError && width == 1 {
		return -1, 1
	}
	return r, uint32(width)
}

func (c *includedCursor) advance(r rune, width uint32) {
	c.pos.bytes += width
	if r == '\n' {
		c.pos.row++
		c.pos.col = 0
	} else {
		c.pos.col += width
	}
	if c.pos.bytes < c.ranges[c.index].EndByte {
		return
	}
	for c.index++; c.index < len(c.ranges); c.index++ {
		r := c.ranges[c.index]
		if r.StartByte < r.EndByte {
			c.pos = NewLength(r.StartByte, r.StartRow, r.StartCol)
			return
		}
		// C reaches each range start before checking for an empty range,
		// including the last range when it is empty.
		c.pos = NewLength(r.StartByte, r.StartRow, r.StartCol)
	}
}

func (c *includedCursor) markEnd() Length {
	if !c.eof() && c.index > 0 && c.pos.bytes == c.ranges[c.index].StartByte {
		r := c.ranges[c.index-1]
		return NewLength(r.EndByte, r.EndRow, r.EndCol)
	}
	return c.pos
}

func (c *includedCursor) frontier() uint32 {
	end := uint64(c.pos.bytes) + 1
	if ch, _ := c.lookahead(); ch < 0 {
		end += 4
	}
	return uint32(min(end, uint64(^uint32(0))))
}

// ScannerCursor gives the scanner adapter the same range navigation as
// the table lexer, without exposing the parser's mutable lexer state.
type ScannerCursor struct{ includedCursor }

// At starts a separate traversal from at, moving to the next range if
// necessary. A scanner can use this to restore a saved cursor or read a column.
func (c ScannerCursor) At(at Length) ScannerCursor {
	if at == c.pos {
		return c
	}
	if c.index < len(c.ranges) {
		r := c.ranges[c.index]
		if r.StartByte < at.bytes && at.bytes < r.EndByte {
			c.pos = at
			return c
		}
	}
	return ScannerCursor{newIncludedCursor(c.src, c.ranges, at)}
}

func (c ScannerCursor) Position() Length { return c.pos }

// AtSelectedEnd reports whether a positioned byte has reached the final
// range's end. It can be queried even after the scanner restores its cursor.
func (c ScannerCursor) AtSelectedEnd(position uint32) bool {
	return position >= c.ranges[len(c.ranges)-1].EndByte
}

func (c ScannerCursor) EOF() bool       { return c.eof() }
func (c ScannerCursor) MarkEnd() Length { return c.markEnd() }
func (c ScannerCursor) AtRangeStart() bool {
	return !c.eof() && c.pos.bytes == c.ranges[c.index].StartByte
}

// Advance consumes one code point, then skips to the next selected range
// when the current range ends. The width excludes the skipped gap.
func (c *ScannerCursor) Advance() (rune, uint32) {
	r, width := c.lookahead()
	if width != 0 {
		c.advance(r, width)
	}
	return r, width
}

// scanIncludedDFA walks the same tables as scanDFA with the range cursor.
// Dispatch stays outside scanDFA so ordinary scanning keeps its ASCII run
// loop. The second result distinguishes the end token from a skipped run.
func scanIncludedDFA(states []LexDFAState, ascii [][128]int32, c includedCursor, startState uint32, out *Token, fail *[2]Length) (bool, bool) {
	if c.pos.bytes == 0 {
		if r, width := c.lookahead(); r == '\ufeff' {
			c.advance(r, width)
		}
	}
	start := c.pos
	var end Length
	var symbol Symbol
	var priority int16
	var acceptAt uint32
	var accepted, skip, forceEnd bool
	eofHops := 0
	state := int32(startState)
	for state >= 0 && int(state) < len(states) {
		st := &states[state]
		atEOF := c.eof()
		if forceEnd || st.AcceptToken != 0 || st.Skip || st.AcceptEOF && atEOF {
			if !accepted || forceEnd || st.AcceptPriority < priority || st.AcceptPriority == priority && c.pos.bytes >= acceptAt {
				end, symbol, priority = c.markEnd(), st.AcceptToken, st.AcceptPriority
				accepted, skip, acceptAt = true, st.Skip, c.pos.bytes
			}
		}
		forceEnd = false
		if atEOF {
			if eofHops > len(states) {
				break
			}
			next := st.EOF
			if next >= 0 && int(next) < len(states) {
				target := &states[next]
				// Old tables can omit AcceptEOF on the end state, as in scanDFA.
				forceEnd = target.AcceptToken == 0 && !target.Skip
			} else {
				next = ccNulNext(st)
				if next < 0 {
					next = st.Default
				}
			}
			if next < 0 {
				break
			}
			eofHops++
			state = next
			continue
		}

		r, width := c.lookahead()
		next, skips := includedLexTransition(st, ascii, state, r)
		if next < 0 {
			break
		}
		eofHops = 0
		c.advance(r, width)
		if skips {
			start = c.pos
			if skip {
				accepted = false
			}
		}
		state = next
	}
	frontier := c.frontier()
	if !accepted && eofHops > 0 {
		end, accepted = c.markEnd(), true
	}
	if !accepted {
		end = c.markEnd()
	}
	// C/lexer.c:447-456 clamps the token start even after a failed scan.
	if end.bytes < start.bytes {
		start = end
	}
	if !accepted {
		fail[0], fail[1] = start, c.pos
		*out = Token{LookaheadEnd: frontier}
		return false, false
	}
	if skip {
		symbol = 0
	}
	*out = Token{
		Symbol: symbol, StartByte: start.bytes, EndByte: end.bytes,
		StartRow: start.row, StartCol: start.col, EndRow: end.row, EndCol: end.col,
		LookaheadEnd: max(frontier, end.bytes),
	}
	return true, symbol == 0 && !skip
}

func includedLexTransition(st *LexDFAState, ascii [][128]int32, state int32, ch rune) (int32, bool) {
	if ch >= 0 && ch < 128 && int(state) < len(ascii) {
		entry := ascii[state][ch]
		if entry != LexAsciiNoMatch {
			return entry &^ LexAsciiSkipBit, entry&LexAsciiSkipBit != 0
		}
		return st.Default, false
	}
	for _, tr := range st.Trans {
		if ch >= tr.Lo && ch <= tr.Hi {
			return tr.Next, tr.Skip
		}
	}
	if ch < 0 {
		if next, skip := ccInvalidNext(st); next >= 0 {
			return next, skip
		}
	}
	return st.Default, false
}

// editIncludedRanges applies C get_changed_ranges.c:106 to each boundary.
// The old backing is shared by Tree.Copy and may still be read by a parse.
func editIncludedRanges(snapshot *rangeArray, edit editRange, led *Ledger) *rangeArray {
	ranges := *snapshot
	if led != nil && led.Active() {
		bytes, ok := sliceBytes[Range](len(ranges))
		if !ok {
			panic(ledgerStop{reason: reservationInvalid})
		}
		if reason := led.reserveClassified(sizeOf[rangeArray]()); reason != reservationOK {
			panic(ledgerStop{reason: reason})
		}
		if reason := led.reserveClassified(bytes); reason != reservationOK {
			panic(ledgerStop{reason: reason})
		}
	}
	var edited rangeArray
	for i, r := range ranges {
		start := editIncludedPosition(NewLength(r.StartByte, r.StartRow, r.StartCol), edit)
		end := NewLength(r.EndByte, r.EndRow, r.EndCol)
		if end.bytes != ^uint32(0) {
			end = editIncludedPosition(end, edit)
		}
		next := Range{StartByte: start.bytes, StartRow: start.row, StartCol: start.col, EndByte: end.bytes, EndRow: end.row, EndCol: end.col}
		if edited == nil && next != r {
			var unlimited Ledger
			if led == nil {
				led = &unlimited
			}
			if reason := led.reserveClassified(sizeOf[rangeArray]()); reason != reservationOK {
				panic(ledgerStop{reason: reason})
			}
			edited = mk[Range](led, len(ranges))
			copy(edited[:i], ranges[:i])
		}
		if edited != nil {
			edited[i] = next
		}
	}
	if edited == nil {
		return snapshot
	}
	return &edited
}

func editIncludedPosition(position Length, e editRange) Length {
	if position.bytes >= e.oldEnd.bytes {
		updated := lenAdd(e.newEnd, lenSub(position, e.oldEnd))
		if updated.bytes < e.newEnd.bytes {
			return lengthMax
		}
		return updated
	}
	if position.bytes > e.start.bytes {
		return e.start
	}
	return position
}
