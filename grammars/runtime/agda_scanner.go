//go:build !grammar_subset || grammar_subset_agda

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the agda grammar.
const (
	agdaTokNewline = 0
	agdaTokIndent  = 1
	agdaTokDedent  = 2
)

const (
	agdaSymNewline gotreesitter.Symbol = 87
	agdaSymIndent  gotreesitter.Symbol = 88
	agdaSymDedent  gotreesitter.Symbol = 89
)

// agdaState tracks indent stack for Agda parsing.
type agdaState struct {
	indents []uint16
}

// AgdaExternalScanner handles newline/indent/dedent for Agda.
type AgdaExternalScanner struct{}

func (AgdaExternalScanner) Create() any {
	return &agdaState{indents: []uint16{0}}
}

func (AgdaExternalScanner) Destroy(payload any) {}

func (AgdaExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*agdaState)
	n := 0
	for i := 1; i < len(s.indents) && n < len(buf); i++ {
		buf[n] = byte(s.indents[i])
		n++
	}
	return n
}

func (AgdaExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*agdaState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	for i := 0; i < len(buf); i++ {
		s.indents = append(s.indents, uint16(buf[i]))
	}
}

func (AgdaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*agdaState)
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
		case ch == '-':
			// Skip -- line comments
			lexer.Advance(true)
			if lexer.Lookahead() == '-' {
				for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
					lexer.Advance(true)
				}
				indentLen = 0
			} else {
				// Not a comment, put us back
				if foundEol {
					break
				}
				return false
			}
		case ch == '{':
			// Skip {- -} block comments
			lexer.Advance(true)
			if lexer.Lookahead() == '-' {
				lexer.Advance(true)
				depth := 1
				for depth > 0 && lexer.Lookahead() != 0 {
					if lexer.Lookahead() == '{' {
						lexer.Advance(true)
						if lexer.Lookahead() == '-' {
							depth++
							lexer.Advance(true)
						}
					} else if lexer.Lookahead() == '-' {
						lexer.Advance(true)
						if lexer.Lookahead() == '}' {
							depth--
							lexer.Advance(true)
						}
					} else {
						lexer.Advance(true)
					}
				}
				indentLen = 0
			} else {
				if foundEol {
					break
				}
				return false
			}
		case ch == 0:
			indentLen = 0
			foundEol = true
			goto done
		default:
			goto done
		}
		continue
	done:
		break
	}

	if foundEol {
		if len(s.indents) > 0 {
			currentIndent := s.indents[len(s.indents)-1]

			if agdaValid(validSymbols, agdaTokIndent) && indentLen > uint32(currentIndent) {
				s.indents = append(s.indents, uint16(indentLen))
				lexer.SetResultSymbol(agdaSymIndent)
				return true
			}

			if (agdaValid(validSymbols, agdaTokDedent) || !agdaValid(validSymbols, agdaTokNewline)) &&
				indentLen < uint32(currentIndent) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.SetResultSymbol(agdaSymDedent)
				return true
			}
		}

		if agdaValid(validSymbols, agdaTokNewline) {
			lexer.SetResultSymbol(agdaSymNewline)
			return true
		}
	}

	return false
}

func agdaValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
