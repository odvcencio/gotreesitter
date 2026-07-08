//go:build !grammar_subset || grammar_subset_nginx

package grammars

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the nginx grammar.
const (
	nginxTokNewline = 0
	nginxTokIndent  = 1
	nginxTokDedent  = 2
)

// Concrete symbol IDs from the generated nginx grammar ExternalSymbols.
const (
	nginxSymNewline gotreesitter.Symbol = 85
	nginxSymIndent  gotreesitter.Symbol = 86
	nginxSymDedent  gotreesitter.Symbol = 87
)

// nginxScannerState holds the indent stack for the nginx external scanner.
type nginxScannerState struct {
	indents []uint16
}

// NginxExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-nginx.
//
// This is a Go port of the C external scanner from tree-sitter-nginx
// (https://github.com/opa-oz/tree-sitter-nginx). The scanner handles:
//   - _newline: newline characters
//   - _indent: increase in indentation level
//   - _dedent: decrease in indentation level
type NginxExternalScanner struct{}

func (NginxExternalScanner) Create() any {
	return &nginxScannerState{indents: []uint16{0}}
}

func (NginxExternalScanner) Destroy(payload any) {}

func (NginxExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*nginxScannerState)
	// Skip the initial 0 sentinel; serialize from index 1 onward.
	size := 0
	for i := 1; i < len(s.indents) && size+1 < len(buf); i++ {
		v := s.indents[i]
		buf[size] = byte(v)
		buf[size+1] = byte(v >> 8)
		size += 2
	}
	return size
}

func (NginxExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*nginxScannerState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0) // sentinel
	// Backward compatibility: older scanner states serialized one byte per indent.
	if len(buf)%2 != 0 {
		for _, b := range buf {
			s.indents = append(s.indents, uint16(b))
		}
		return
	}
	for i := 0; i+1 < len(buf); i += 2 {
		v := uint16(buf[i]) | uint16(buf[i+1])<<8
		s.indents = append(s.indents, v)
	}
}

func (NginxExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*nginxScannerState)

	// If lookahead is newline and NEWLINE is valid, consume it.
	if lexer.Lookahead() == '\n' {
		if nginxValid(validSymbols, nginxTokNewline) {
			lexer.Advance(true) // skip
			lexer.SetResultSymbol(nginxSymNewline)
			return true
		}
		return false
	}

	// At column 0, measure indentation and emit INDENT/DEDENT.
	if lexer.Lookahead() != 0 && lexer.Column() == 0 {
		var indentLen uint16

		// Indent tokens are zero width.
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
		if indentLen > top && nginxValid(validSymbols, nginxTokIndent) {
			s.indents = append(s.indents, indentLen)
			lexer.SetResultSymbol(nginxSymIndent)
			return true
		}
		if indentLen < top && nginxValid(validSymbols, nginxTokDedent) {
			s.indents = s.indents[:len(s.indents)-1]
			lexer.SetResultSymbol(nginxSymDedent)
			return true
		}
	}

	return false
}

func nginxValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
