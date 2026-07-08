//go:build !grammar_subset || grammar_subset_caddy

package grammars

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the caddy grammar.
const (
	caddyTokNewline = 0
	caddyTokIndent  = 1
	caddyTokDedent  = 2
)

// Concrete symbol IDs from the generated caddy grammar ExternalSymbols.
const (
	caddySymNewline gotreesitter.Symbol = 37
	caddySymIndent  gotreesitter.Symbol = 38
	caddySymDedent  gotreesitter.Symbol = 39
)

type caddyScannerState struct {
	indents []uint16
}

// CaddyExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-caddy.
// Handles _newline, _indent, and _dedent tokens for indentation tracking.
type CaddyExternalScanner struct{}

func (CaddyExternalScanner) Create() any {
	return &caddyScannerState{indents: []uint16{0}}
}

func (CaddyExternalScanner) Destroy(payload any) {}

func (CaddyExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*caddyScannerState)
	size := 0
	for i := 1; i < len(s.indents) && size < len(buf); i++ {
		buf[size] = byte(s.indents[i])
		size++
	}
	return size
}

func (CaddyExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*caddyScannerState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	for _, b := range buf {
		s.indents = append(s.indents, uint16(b))
	}
}

func (CaddyExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*caddyScannerState)

	if lexer.Lookahead() == '\n' {
		if caddyValid(validSymbols, caddyTokNewline) {
			lexer.Advance(true)
			lexer.MarkEnd()
			lexer.SetResultSymbol(caddySymNewline)
			return true
		}
		return false
	}

	if lexer.Lookahead() != 0 && lexer.Column() == 0 {
		var indentLen uint16

		lexer.MarkEnd()

		for {
			ch := lexer.Lookahead()
			if ch == ' ' {
				indentLen++
				lexer.Advance(true)
			} else if ch == '\t' {
				indentLen += 8
				lexer.Advance(true)
			} else {
				break
			}
		}

		top := s.indents[len(s.indents)-1]
		if indentLen > top && caddyValid(validSymbols, caddyTokIndent) {
			s.indents = append(s.indents, indentLen)
			lexer.MarkEnd()
			lexer.SetResultSymbol(caddySymIndent)
			return true
		}
		if indentLen < top && caddyValid(validSymbols, caddyTokDedent) {
			s.indents = s.indents[:len(s.indents)-1]
			lexer.MarkEnd()
			lexer.SetResultSymbol(caddySymDedent)
			return true
		}
	}

	return false
}

func caddyValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
