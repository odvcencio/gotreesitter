package ccore

// The lex automaton walk. Ports the generated ts_lex and ts_lex_keywords
// functions of a grammar's parser.c (START_LEXER, ADVANCE, SKIP, ACCEPT_TOKEN,
// END_STATE) over the lex tables in Tables, together with the character
// reading of C/lexer.c: ts_lexer__get_lookahead, ts_lexer__advance,
// ts_lexer__eof and the start of ts_lexer_start (the byte order mark). The
// reading equals C's in these points: at the end of input the lookahead is 0
// and advance does nothing; an ill-formed UTF-8 sequence is lookahead -1
// (TS_DECODE_ERROR), not U+FFFD; a SKIP keeps an earlier accept (ts_lexer_finish
// moves the token start back to the token end); U+FEFF at byte 0 is padding.
//
// The blob keeps less than C's source of ts_lex, so three rules in this file
// are heuristics, each marked HEURISTIC where it stands:
//
//	ccNulNext          (lexdfa.go) a step on character 0 at the end of input
//	ccInvalidNext      (lexdfa.go) the step on lookahead -1
//	end accept         (lexdfa.go, in scanDFA) an end transition read as the end accept
//
// A fourth, in lexer.go (Parser.lex), reads a failed error-mode scan at the
// end of input as the empty end token. Each comment gives the count of lex
// states, over the 241 pinned parser.c files (118,580 states), where the rule
// differs from C.

import "unicode/utf8"

// Values of the ASCII fast-path table: the next state in bits 0-30, the SKIP
// flag in bit 31, and LexAsciiNoMatch when there is no transition.
const (
	LexAsciiNoMatch = int32(0x7fff_ffff)
	LexAsciiSkipBit = int32(-1 << 31)
)

// BuildLexAscii derives the ASCII fast-path table of states: for each state
// and each character below 128, the first matching transition (with the SKIP
// flag in bit 31), or LexAsciiNoMatch. It is what the root package's
// buildLexAsciiTable gives for the same states.
func BuildLexAscii(states []LexDFAState) [][128]int32 {
	tbl := make([][128]int32, len(states))
	for si := range states {
		row := &tbl[si]
		for c := range row {
			row[c] = LexAsciiNoMatch
		}
		remaining := 128
		for ti := range states[si].Trans {
			if remaining == 0 {
				break
			}
			tr := &states[si].Trans[ti]
			if tr.Hi < 0 || tr.Lo >= 128 {
				continue
			}
			lo, hi := tr.Lo, tr.Hi
			if lo < 0 {
				lo = 0
			}
			if hi > 127 {
				hi = 127
			}
			v := tr.Next
			if tr.Skip {
				v |= LexAsciiSkipBit
			}
			for c := int(lo); c <= int(hi); c++ {
				if row[c] == LexAsciiNoMatch {
					row[c] = v
					remaining--
				}
			}
		}
	}
	return tbl
}

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
		start := NewLength(uint32(startPos), startRow, startCol)
		fail[0], fail[1] = start, start
		return false
	}

	scanPos, scanRow, scanCol := startPos, startRow, startCol
	// C/lexer.c:438-441 ts_lexer_start: U+FEFF at byte 0 is skipped as padding
	// (advance with skip). The column counts its three bytes (lexer.c:207-217).
	if startPos == 0 && len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
		scanPos, scanCol = 3, startCol+3
	}
	tokenStartPos, tokenStartRow, tokenStartCol := scanPos, scanRow, scanCol

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
				// so the end transition itself is read as that accept. The rule is
				// needed where a state accepts a token and also has "if (eof)" (18
				// pinned states: caddy, csv, ledger, nginx, nushell, yaml); C's end
				// accept comes last there too, so the result is C's. It differs from
				// C where the importer invents an end transition (341 states; known
				// difference).
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
			// C takes no other step at the end of input (the tests carry "!eof"),
			// except a bare character 0 in an ADVANCE_MAP (see ccNulNext).
			next := ccNulNext(st)
			if next < 0 {
				next = st.Default // tables from the grammar generator; none in C tables
			}
			if next < 0 {
				break
			}
			eofHops++
			curState = next
			continue
		} else {
			eofHops = 0
			b = src[scanPos]
			if b < 0x80 {
				r = rune(b)
				size = 1
			} else {
				r, size = utf8.DecodeRune(src[scanPos:])
				if r == utf8.RuneError && size == 1 {
					// C/lexer.c:129-142: an ill-formed sequence is lookahead -1 (TS_DECODE_ERROR), one byte (140-142).
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
			// C SKIP(state) consumes and resets the token start. It does not reset
			// the accepted token: ts_lexer_finish moves the token start back to the
			// token end (C/lexer.c:455-457), so the token is empty. An accept of the
			// generator's own skip states (acceptSkip) is dropped as before.
			tokenStartPos = scanPos
			tokenStartRow = scanRow
			tokenStartCol = scanCol
			if acceptSkip {
				acceptPos = -1
				acceptSymbol = 0
				acceptSkip = false
			} else if acceptPos >= 0 {
				acceptStartPos, acceptStartRow, acceptStartCol = acceptPos, acceptRow, acceptCol
			}
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

	// C/lexer.c:459-470 ts_lexer_finish: the lookahead end is one byte past the
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
// ill-formed UTF-8 sequence). HEURISTIC. Over the 118,580 lex states of the
// 241 pinned parser.c files it differs from C in 21 states on the range lists
// the core walks (org 19, gitattributes 1, http 1) and in 20 on the lists
// before adjacent ranges of one target are merged; the rule of the first fix
// differed in 3,015.
// -1 is below every range, so C takes a step on it only from a test without a
// lower bound. The generator writes a negated set as "lookahead != 0 && ...",
// which the importer keeps as ranges that start at code point 1 and cover all
// but a few code points; the target whose ranges start at 1 and cover all but
// 256 code points is taken, and a lone [0,0] range ("lookahead == 0") is
// ignored. The blob does not keep the form of the test, so it cannot tell
// every C condition apart. A range that starts at 0 comes from several forms:
// "(!eof && lookahead <= x)" and a bare "if (!eof)" hold for -1; a set test,
// "(!eof && set_contains(...))", does not, because -1 is in no set. So the
// rule rejects a target with a range that starts at 0. The scan finds no
// state with a [0,0x10FFFF] range. The states the rule misses: in org a
// negated set function, "!..._character_set_N(lookahead)", whose ranges cover
// less than the rule asks; in gitattributes a guarded range from 0; in http a
// [0,0] range that the blob merges with [1,8], so that it is no longer alone.
// Exact repair: keep C's step for -1 at import.
// When several targets qualify (their ranges overlap), C takes the first
// condition that holds, in source order. A condition that holds for -1 starts
// at code point 1 in the blob, so the target whose first range starting at 1
// comes first wins, with that range's SKIP flag.
// The rule is exact for the ranges the blob holds. A target can qualify only
// if one of its ranges starts at code point 1, so the first pass collects the
// targets that have such a range, in order, and the second pass sums their
// coverage: two linear passes while at most ccInvalidSlots targets have one.
// A state with more of them takes ccInvalidNextAll, which gives the same
// answer with one pass per target. In the pinned states no state has more
// than one such target, so none takes it.
func ccInvalidNext(st *LexDFAState) (int32, bool) {
	var cand [ccInvalidSlots]int32
	var skip [ccInvalidSlots]bool
	n := 0
	for i := range st.Trans {
		t := &st.Trans[i]
		if t.Lo != 1 {
			continue
		}
		seen := false
		for j := 0; j < n; j++ {
			if cand[j] == t.Next {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		if n == ccInvalidSlots {
			return ccInvalidNextAll(st)
		}
		cand[n], skip[n] = t.Next, t.Skip
		n++
	}
	if n == 0 {
		return -1, false
	}
	var covered [ccInvalidSlots]int64
	var fromZero [ccInvalidSlots]bool
	for i := range st.Trans {
		t := &st.Trans[i]
		if t.Lo == 0 && t.Hi == 0 {
			continue
		}
		for j := 0; j < n; j++ {
			if cand[j] != t.Next {
				continue
			}
			covered[j] += int64(t.Hi) - int64(t.Lo) + 1
			if t.Lo == 0 {
				fromZero[j] = true
			}
			break
		}
	}
	// cand is in the order of each target's first range that starts at 1.
	for j := 0; j < n; j++ {
		if covered[j] >= ccInvalidCover && !fromZero[j] {
			return cand[j], skip[j]
		}
	}
	return -1, false
}

// ccInvalidNextAll is ccInvalidNext without the limit on the number of
// targets: for each target that has a range starting at code point 1, in the
// order of those ranges, it sums the target's ranges in one pass and returns
// the first target that qualifies. Its cost can reach the square of the number
// of transitions: one pass per such target, and for each range that starts at
// code point 1 a scan of the earlier transitions to see whether its target was
// already summed.
func ccInvalidNextAll(st *LexDFAState) (int32, bool) {
	for i := range st.Trans {
		t := &st.Trans[i]
		if t.Lo != 1 {
			continue
		}
		again := false
		for j := 0; j < i; j++ {
			if st.Trans[j].Lo == 1 && st.Trans[j].Next == t.Next {
				again = true
				break
			}
		}
		if again {
			continue
		}
		var covered int64
		fromZero := false
		for k := range st.Trans {
			u := &st.Trans[k]
			if u.Next != t.Next || u.Lo == 0 && u.Hi == 0 {
				continue
			}
			covered += int64(u.Hi) - int64(u.Lo) + 1
			if u.Lo == 0 {
				fromZero = true
			}
		}
		if covered >= ccInvalidCover && !fromZero {
			return t.Next, t.Skip
		}
	}
	return -1, false
}

const (
	// ccInvalidSlots is the number of targets with a range starting at code
	// point 1 that ccInvalidNext handles in two passes.
	ccInvalidSlots = 8
	// ccInvalidCover is the coverage a target needs: every code point but 256.
	ccInvalidCover = 0x110000 - 256
	// ccNulMapRanges is the number of leading simple ranges from which the
	// generator writes an ADVANCE_MAP.
	ccNulMapRanges = 8
)

// ccNulNext is the step on character 0 at the end of input, or -1. HEURISTIC:
// the blob does not say which step C takes there, so the rule rebuilds what
// the generator wrote. Over the 118,580 pinned lex states it gives C's step in
// every state, on the range lists the core walks and on the lists before
// adjacent ranges of one target are merged. The rule of the second fix
// differed in 2 states (http, bitbake), that of the first in 129, and "never"
// in 61.
// At the end of input C's lookahead is 0 and eof is true. A generated test
// "(!eof && lookahead == 0)" or "!eof && ..." is false there, and so is a
// range test; only a bare character 0 in an ADVANCE_MAP (no "!eof"), or the
// bare "lookahead == 0" of old generators, holds. The importer drops the
// "!eof" operand (cmd/ts2go/lex_dfa.go:1126-1156), so the blob keeps a range
// that starts at 0 for each form and for ADVANCE_MAP alike. C writes the
// "!eof" test in states with a catch-all and in states without one, so a
// catch-all does not tell the forms apart. The map does.
// The generator sorts a state's transitions with the simple ones first: not a
// SKIP, and every range of the character set at most two code points wide and
// not above 0xFFFF. When the leading simple transitions hold at least 8
// ranges it writes them as an ADVANCE_MAP and the rest as "if" tests
// (tree-sitter, crates/generate/src/render.rs, the block that sets
// leading_simple_transition_range_count). The importer copies the map's pairs
// in source order. So the rule counts the leading ranges of that kind. The
// range that ends the run can belong to a transition whose first ranges looked
// simple: a transition has one target, so when the ending range is not a SKIP
// and has the target of the run's last ranges, those leave the run. With at
// least 8 ranges left, the range that starts at 0 is a map entry, and C takes
// its step at the end of input.
// What a looser rule did: http's first lex state begins with the guarded
// "(!eof && lookahead == 0)", four single characters and three single
// characters of a transition that goes on with wider ranges. Counting those
// three gave 8, the step was taken, and it accepted an empty token at the end
// of input again and again.
// Exact repair: keep C's end-of-input step at import.
func ccNulNext(st *LexDFAState) int32 {
	run := 0
	for run < len(st.Trans) {
		t := &st.Trans[run]
		if t.Skip || t.Hi-t.Lo > 1 || t.Hi > 0xFFFF {
			break
		}
		run++
	}
	if run < len(st.Trans) && !st.Trans[run].Skip {
		next := st.Trans[run].Next
		for run > 0 && st.Trans[run-1].Next == next {
			run--
		}
	}
	if run < ccNulMapRanges {
		return -1
	}
	for i := 0; i < run; i++ {
		if t := &st.Trans[i]; t.Lo == 0 {
			return t.Next
		}
	}
	return -1
}
