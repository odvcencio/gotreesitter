package ccore

// The lex automaton walk. Ports the generated ts_lex and ts_lex_keywords
// functions of a grammar's parser.c (START_LEXER, ADVANCE, SKIP, ACCEPT_TOKEN,
// END_STATE) over the lex tables in Tables, together with the character
// reading of C/lexer.c: ts_lexer__get_lookahead, ts_lexer__advance and
// ts_lexer__eof. Two changes against the earlier root kernel make the reading
// equal C's: at the end of input the lookahead is 0 and the walk goes on (end
// transitions first, then transitions on character 0), and an ill-formed UTF-8
// sequence is lookahead -1 (TS_DECODE_ERROR), not U+FFFD.
//
// Two rules below are heuristics, because the blob keeps less than C's source
// of ts_lex: ccInvalidNext (the "lookahead != x" tests) and ccBodyState (the
// "!eof" operand). Each says what it assumes.

import "unicode/utf8"

// Values of the ASCII fast-path table: the next state in bits 0-30, the SKIP
// flag in bit 31, and LexAsciiNoMatch when there is no transition.
const (
	LexAsciiNoMatch = int32(0x7fff_ffff)
	LexAsciiSkipBit = int32(-1 << 31)
)

// LexTrans maps a character range (inclusive) to a next state.
type LexTrans struct {
	Lo, Hi rune
	Next   int32
	Skip   bool // C SKIP(state): consume the character and reset the token start
}

// LexDFAState is one state of the lex automaton (one case of ts_lex).
type LexDFAState struct {
	AcceptToken    Symbol // 0 unless the state accepts a token other than the end token
	AcceptPriority int16  // lower wins
	Skip           bool   // the state accepts skipped characters
	AcceptEOF      bool   // the state accepts the end token at the end of input
	Default        int32  // next state when no range matches; -1 if none
	EOF            int32  // next state at the end of input; -1 if none
	Trans          []LexTrans
}

// scanDFA runs the automaton in states from startState at src[startPos]
// (startRow and startCol are that position's point). It reports whether a
// token was accepted. On failure, fail holds where the attempt began (after
// skipped characters) and where it ended, and out.LookaheadEnd is the farthest
// byte inspected.
func scanDFA(states []LexDFAState, ascii [][128]int32, src []byte, startState uint32, startPos int, startRow, startCol uint32, out *Token, fail *[2]Length) bool {
	curState := int32(startState)
	if curState < 0 || int(curState) >= len(states) {
		*out = Token{}
		return false
	}

	scanPos, scanRow, scanCol := startPos, startRow, startCol
	tokenStartPos, tokenStartRow, tokenStartCol := startPos, startRow, startCol

	// The last accepting state.
	acceptPos := -1
	var acceptRow, acceptCol uint32
	var acceptStartPos int
	var acceptStartRow, acceptStartCol uint32
	var acceptSymbol Symbol
	acceptSkip := false
	acceptPriorityBest := int16(32767)

	eofHops := 0
	for {
		if curState < 0 || int(curState) >= len(states) {
			break
		}
		st := &states[int(curState)]

		if st.AcceptToken > 0 || st.Skip || (st.AcceptEOF && scanPos >= len(src)) {
			newPrio := st.AcceptPriority
			if acceptPos < 0 || newPrio < acceptPriorityBest || (newPrio == acceptPriorityBest && scanPos >= acceptPos) {
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

		var b byte
		var r rune
		var size int
		if scanPos >= len(src) {
			// C/lexer.c:110-114, 251-256, 263: at the end of input the lookahead
			// is 0, eof() is true, and advance() does nothing.
			if eofHops > len(states) {
				break
			}
			if st.EOF >= 0 {
				curState = st.EOF
				eofHops++
				// HEURISTIC. In C the state after an end transition is
				// ACCEPT_TOKEN(ts_builtin_sym_end), and the last accept wins, so an
				// earlier zero-width accept of the start state (a token that can
				// match nothing) does not survive. The blob keeps the end accept
				// only as AcceptEOF, which blobs from older generator runs omit;
				// so the end transition itself is read as that accept.
				if int(curState) < len(states) && curState >= 0 {
					if tg := &states[curState]; tg.AcceptToken == 0 && !tg.Skip {
						acceptPos = scanPos
						acceptRow = scanRow
						acceptCol = scanCol
						acceptStartPos = tokenStartPos
						acceptStartRow = tokenStartRow
						acceptStartCol = tokenStartCol
						acceptSymbol = 0
						acceptSkip = false
						acceptPriorityBest = tg.AcceptPriority
					}
				}
				continue
			}
			if ccBodyState(st) {
				break
			}
		} else {
			eofHops = 0
			b = src[scanPos]
			if b < 0x80 {
				r = rune(b)
				size = 1
			} else {
				r, size = utf8.DecodeRune(src[scanPos:])
				if r == utf8.RuneError && size == 1 {
					// C/lexer.c:131-133, 152-154: an ill-formed sequence is lookahead -1 (TS_DECODE_ERROR), one byte.
					r = -1
				}
			}
		}
		nextState := int32(-1)
		skipTransition := false
		if b < 0x80 && ascii != nil && int(curState) < len(ascii) {
			v := ascii[curState][b]
			if v != LexAsciiNoMatch {
				nextState = v & ^LexAsciiSkipBit
				skipTransition = v&LexAsciiSkipBit != 0
			}
		} else {
			for i := range st.Trans {
				tr := &st.Trans[i]
				if r >= tr.Lo && r <= tr.Hi {
					nextState = tr.Next
					skipTransition = tr.Skip
					break
				}
			}
		}
		if nextState < 0 && r < 0 {
			nextState, skipTransition = ccInvalidNext(st)
		}
		// Default transitions do not skip.
		skipTransition = skipTransition && nextState >= 0
		if nextState < 0 && st.Default >= 0 {
			nextState = st.Default
			skipTransition = false
		}
		if nextState < 0 {
			break
		}

		if size == 0 {
			eofHops++ // a transition on character 0 at the end of input
		}
		scanPos += size
		if r == '\n' {
			scanRow++
			scanCol = 0
		} else {
			scanCol += uint32(size)
		}

		// Staying in one state keeps acceptance, priority and token start, so a
		// run of plain ASCII characters can be passed in one loop; the last
		// acceptance is recorded at the top of the next iteration. Newline,
		// non-ASCII, SKIP, default and changed-state transitions stay scalar.
		if size > 0 && b < 128 && r != '\n' && nextState == curState && !skipTransition && ascii != nil && int(curState) < len(ascii) {
			row := &ascii[curState]
			end := scanPos
			for end < len(src) {
				c := src[end]
				if c >= 128 || c == '\n' || row[c] != curState {
					break
				}
				end++
			}
			scanCol += uint32(end - scanPos)
			scanPos = end
		}

		if skipTransition {
			// C SKIP(state) consumes and resets the token start.
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
		// Older blobs omit end-token acceptance. Keep their end-chain
		// fallback when no state records an acceptance.
		acceptPos = scanPos
		acceptRow = scanRow
		acceptCol = scanCol
		acceptStartPos = tokenStartPos
		acceptStartRow = tokenStartRow
		acceptStartCol = tokenStartCol
		acceptSymbol = 0
		acceptSkip = true
	}

	// C/lexer.c:253 ts_lexer_finish: the lookahead end is one byte past the
	// cursor, plus four when the lookahead there is an invalid sequence.
	frontier := uint64(scanPos) + 1
	if scanPos < len(src) && src[scanPos] >= utf8.RuneSelf {
		if r, size := utf8.DecodeRune(src[scanPos:]); r == utf8.RuneError && size == 1 {
			frontier += 4
		}
	}
	lookaheadEnd := uint32(^uint32(0))
	if frontier <= uint64(^uint32(0)) {
		lookaheadEnd = uint32(frontier)
	}
	if acceptPos > 0 && lookaheadEnd < uint32(acceptPos) {
		lookaheadEnd = uint32(acceptPos)
	}

	if acceptPos < 0 {
		fail[0] = NewLength(uint32(tokenStartPos), tokenStartRow, tokenStartCol)
		fail[1] = NewLength(uint32(scanPos), scanRow, scanCol)
		*out = Token{LookaheadEnd: lookaheadEnd}
		return false
	}

	// A skipped run is returned with symbol 0.
	*out = Token{
		Symbol:       acceptSymbol,
		StartByte:    uint32(acceptStartPos),
		EndByte:      uint32(acceptPos),
		StartRow:     acceptStartRow,
		StartCol:     acceptStartCol,
		EndRow:       acceptRow,
		EndCol:       acceptCol,
		LookaheadEnd: lookaheadEnd,
	}
	if acceptSkip {
		out.Symbol = 0
	}
	return true
}

// ccInvalidNext is the transition that C takes on lookahead -1 (an
// ill-formed UTF-8 sequence). HEURISTIC. -1 is below every range, so only a C
// test that excludes a few code points ("lookahead != x") holds. The blob keeps
// such a test as ranges that together cover all but a few code points, and it
// does not keep the form of the test; the first transition whose target is
// reached by ranges covering that much is taken.
func ccInvalidNext(st *LexDFAState) (int32, bool) {
	for i := range st.Trans {
		next := st.Trans[i].Next
		var covered int64
		minLo := rune(0x10FFFF)
		for j := range st.Trans {
			if st.Trans[j].Next == next {
				covered += int64(st.Trans[j].Hi) - int64(st.Trans[j].Lo) + 1
				if st.Trans[j].Lo < minLo {
					minLo = st.Trans[j].Lo
				}
			}
		}
		// A test with a lower bound ("lookahead > x") fails on -1; a
		// negation starts at code point 0 (or 1 for "lookahead != 0").
		if covered >= 0x110000-256 && minLo <= 1 {
			return next, st.Trans[i].Skip
		}
	}
	return -1, false
}

// ccBodyState reports a state with a catch-all transition ("lookahead != 0"
// and the like). HEURISTIC. C writes the character-0 test of such a state as
// "!eof && lookahead == 0", so the end of input does not take it; the blob
// keeps only "lookahead == 0" (the generator drops the "!eof" operand), so it
// cannot say which form C had.
func ccBodyState(st *LexDFAState) bool {
	var covered int64
	for i := range st.Trans {
		t := &st.Trans[i]
		lo := t.Lo
		if lo == 0 {
			lo = 1
		}
		if t.Hi >= lo {
			covered += int64(t.Hi) - int64(lo) + 1
		}
	}
	return covered >= 0x110000-256
}
