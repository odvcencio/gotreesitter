package gotreesitter

import (
	"unicode/utf8"
	"unsafe"
)

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
}

func bytesToStringNoCopy(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// Lexer tokenizes source text using a table-driven DFA.
type Lexer struct {
	states []LexState
	source []byte
	pos    int
	row    uint32
	col    uint32
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
func (l *Lexer) Next(startState uint16) Token {
	for {
		// EOF check.
		if l.pos >= len(l.source) {
			return Token{
				StartByte:  uint32(l.pos),
				EndByte:    uint32(l.pos),
				StartPoint: Point{Row: l.row, Column: l.col},
				EndPoint:   Point{Row: l.row, Column: l.col},
			}
		}

		tokenStartPos := l.pos
		tokenStartRow := l.row
		tokenStartCol := l.col

		tok, ok := l.scan(startState, tokenStartPos, tokenStartRow, tokenStartCol)
		if ok {
			if tok.Symbol == 0 {
				// Skip token (whitespace). Verify the lexer actually
				// advanced past the skipped content to prevent an
				// infinite loop on zero-width skip matches.
				if l.pos <= tokenStartPos {
					l.skipOneRune()
				}
				continue
			}
			return tok
		}

		// No accepting state was found. Skip one rune as error recovery.
		l.skipOneRune()
	}
}

// scan runs the DFA from the given start state and position. It returns
// a token and true if an accepting state was reached, or false if not.
// On a skip (whitespace) match, it returns a zero-Symbol token and true.
func (l *Lexer) scan(startState uint16, startPos int, startRow, startCol uint32) (Token, bool) {
	curState := int(startState)
	scanPos := startPos
	scanRow := startRow
	scanCol := startCol

	// Track the last accepting state.
	acceptPos := -1
	acceptRow := startRow
	acceptCol := startCol
	acceptSymbol := Symbol(0)
	acceptSkip := false

	// Check if the start state itself is accepting.
	st := &l.states[curState]
	if st.AcceptToken > 0 || st.Skip {
		acceptPos = scanPos
		acceptRow = scanRow
		acceptCol = scanCol
		acceptSymbol = st.AcceptToken
		acceptSkip = st.Skip
	}

	// Walk the DFA.
	for scanPos < len(l.source) {
		r, size := utf8.DecodeRune(l.source[scanPos:])

		nextState := -1
		st = &l.states[curState]
		for i := range st.Transitions {
			tr := &st.Transitions[i]
			if r >= tr.Lo && r <= tr.Hi {
				nextState = tr.NextState
				break
			}
		}
		if nextState < 0 && st.Default >= 0 {
			nextState = st.Default
		}
		if nextState < 0 {
			break
		}

		// Advance scan position and track row/column.
		scanPos += size
		if r == '\n' {
			scanRow++
			scanCol = 0
		} else {
			scanCol++
		}

		curState = nextState
		ns := &l.states[curState]
		if ns.AcceptToken > 0 || ns.Skip {
			acceptPos = scanPos
			acceptRow = scanRow
			acceptCol = scanCol
			acceptSymbol = ns.AcceptToken
			acceptSkip = ns.Skip
		}
	}

	if acceptPos < 0 {
		return Token{}, false
	}

	// Rewind (or advance) to the accept position.
	l.pos = acceptPos
	l.row = acceptRow
	l.col = acceptCol

	if acceptSkip {
		// Return a zero-Symbol token to signal "skip".
		return Token{
			StartByte:  uint32(startPos),
			EndByte:    uint32(acceptPos),
			StartPoint: Point{Row: startRow, Column: startCol},
			EndPoint:   Point{Row: acceptRow, Column: acceptCol},
		}, true
	}

	return Token{
		Symbol:     acceptSymbol,
		Text:       bytesToStringNoCopy(l.source[startPos:acceptPos]),
		StartByte:  uint32(startPos),
		EndByte:    uint32(acceptPos),
		StartPoint: Point{Row: startRow, Column: startCol},
		EndPoint:   Point{Row: acceptRow, Column: acceptCol},
	}, true
}

// skipOneRune advances the lexer position by one rune, updating row/column.
func (l *Lexer) skipOneRune() {
	if l.pos >= len(l.source) {
		return
	}
	r, size := utf8.DecodeRune(l.source[l.pos:])
	l.pos += size
	if r == '\n' {
		l.row++
		l.col = 0
	} else {
		l.col++
	}
}
