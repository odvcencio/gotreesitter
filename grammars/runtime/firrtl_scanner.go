//go:build !grammar_subset || grammar_subset_firrtl

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the firrtl grammar.
const (
	firrtlTokNewline = 0
	firrtlTokIndent  = 1
	firrtlTokDedent  = 2
)

const (
	firrtlSymNewline gotreesitter.Symbol = 126
	firrtlSymIndent  gotreesitter.Symbol = 127
	firrtlSymDedent  gotreesitter.Symbol = 128
)

// firrtlState tracks indent stack for FIRRTL parsing.
type firrtlState struct {
	indents []uint16
}

// FirrtlExternalScanner handles newline/indent/dedent for FIRRTL.
type FirrtlExternalScanner struct{}

func (FirrtlExternalScanner) Create() any {
	return &firrtlState{indents: []uint16{0}}
}

func (FirrtlExternalScanner) Destroy(payload any) {}

func (FirrtlExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*firrtlState)
	n := 0
	// Skip index 0 (always 0), serialize rest as bytes
	for i := 1; i < len(s.indents) && n < len(buf); i++ {
		buf[n] = byte(s.indents[i])
		n++
	}
	return n
}

func (FirrtlExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*firrtlState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	for i := 0; i < len(buf); i++ {
		s.indents = append(s.indents, uint16(buf[i]))
	}
}

func (FirrtlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*firrtlState)

	lexer.MarkEnd()

	foundEol := false
	indentLen := uint32(0)

	for {
		ch := lexer.Lookahead()
		switch {
		case ch == '\n':
			foundEol = true
			indentLen = 0
			lexer.Advance(true)
		case ch == ' ':
			indentLen++
			lexer.Advance(true)
		case ch == '\r' || ch == '\f':
			indentLen = 0
			lexer.Advance(true)
		case ch == '\t':
			indentLen += 8
			lexer.Advance(true)
		case ch == '#':
			// Skip comment
			for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
				lexer.Advance(true)
			}
			lexer.Advance(true)
			indentLen = 0
		case ch == '\\':
			lexer.Advance(true)
			if lexer.Lookahead() == '\r' {
				lexer.Advance(true)
			}
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
				lexer.Advance(true)
			} else {
				return false
			}
		case ch == 0:
			indentLen = 0
			foundEol = true
			goto done
		default:
			goto done
		}
	}

done:
	if foundEol {
		if len(s.indents) > 0 {
			currentIndent := s.indents[len(s.indents)-1]

			if firrtlValid(validSymbols, firrtlTokIndent) && indentLen > uint32(currentIndent) {
				s.indents = append(s.indents, uint16(indentLen))
				lexer.SetResultSymbol(firrtlSymIndent)
				return true
			}

			if (firrtlValid(validSymbols, firrtlTokDedent) || !firrtlValid(validSymbols, firrtlTokNewline)) &&
				indentLen < uint32(currentIndent) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.SetResultSymbol(firrtlSymDedent)
				return true
			}
		}

		if firrtlValid(validSymbols, firrtlTokNewline) {
			lexer.SetResultSymbol(firrtlSymNewline)
			return true
		}
	}

	return false
}

func firrtlValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
