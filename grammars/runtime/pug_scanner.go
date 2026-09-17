//go:build !grammar_subset || grammar_subset_pug

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the pug grammar.
const (
	pugTokNewline = 0
	pugTokIndent  = 1
	pugTokDedent  = 2
)

const (
	pugSymNewline gotreesitter.Symbol = 78
	pugSymIndent  gotreesitter.Symbol = 79
	pugSymDedent  gotreesitter.Symbol = 80
)

// pugState tracks indent stack for Pug parsing.
type pugState struct {
	indents []uint16
}

// PugExternalScanner handles newline/indent/dedent for Pug templates.
type PugExternalScanner struct{}

func (PugExternalScanner) Create() any {
	return &pugState{indents: []uint16{0}}
}

func (PugExternalScanner) Destroy(payload any) {}

func (PugExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*pugState)
	n := 0
	for i := 1; i < len(s.indents) && n < len(buf); i++ {
		buf[n] = byte(s.indents[i])
		n++
	}
	return n
}

func (PugExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*pugState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	for i := 0; i < len(buf); i++ {
		s.indents = append(s.indents, uint16(buf[i]))
	}
}

func (PugExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*pugState)
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
			indentLen += 2
			lexer.Advance(true)
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

			if pugValid(validSymbols, pugTokIndent) && indentLen > uint32(currentIndent) {
				s.indents = append(s.indents, uint16(indentLen))
				lexer.MarkEnd()
				lexer.SetResultSymbol(pugSymIndent)
				return true
			}

			if (pugValid(validSymbols, pugTokDedent) || !pugValid(validSymbols, pugTokNewline)) &&
				indentLen < uint32(currentIndent) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.MarkEnd()
				lexer.SetResultSymbol(pugSymDedent)
				return true
			}
		}

		if pugValid(validSymbols, pugTokNewline) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(pugSymNewline)
			return true
		}
	}

	return false
}

func pugValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
