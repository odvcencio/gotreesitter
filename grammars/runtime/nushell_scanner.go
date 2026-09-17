//go:build !grammar_subset || grammar_subset_nushell

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the nushell grammar.
const (
	nushellTokRawStringBegin   = 0
	nushellTokRawStringContent = 1
	nushellTokRawStringEnd     = 2
)

// Concrete symbol IDs from the generated nushell grammar ExternalSymbols.
const (
	nushellSymRawStringBegin   gotreesitter.Symbol = 269
	nushellSymRawStringContent gotreesitter.Symbol = 270
	nushellSymRawStringEnd     gotreesitter.Symbol = 271
)

// nushellScannerState holds the raw string hash level.
type nushellScannerState struct {
	level uint8
}

// NushellExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-nu.
//
// This is a Go port of the C external scanner from tree-sitter-nu
// (https://github.com/nushell/tree-sitter-nu). The scanner handles:
//   - raw_string_begin: r#'  (with variable # count)
//   - string_content: content between raw string delimiters
//   - raw_string_end: '#  closing delimiter
type NushellExternalScanner struct{}

func (NushellExternalScanner) Create() any {
	return &nushellScannerState{level: 0}
}

func (NushellExternalScanner) Destroy(payload any) {}

func (NushellExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*nushellScannerState)
	if len(buf) >= 1 {
		buf[0] = s.level
		return 1
	}
	return 0
}

func (NushellExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*nushellScannerState)
	s.level = 0
	if len(buf) == 1 {
		s.level = buf[0]
	}
}

func (NushellExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*nushellScannerState)

	// RAW_STRING_BEGIN: r#+'
	if nushellValid(validSymbols, nushellTokRawStringBegin) && s.level == 0 {
		// Skip whitespace
		for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != 0 {
			lexer.Advance(true)
		}

		if lexer.Lookahead() != 'r' {
			return false
		}
		lexer.Advance(false)

		var level uint8
		for lexer.Lookahead() == '#' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
			level++
		}

		if lexer.Lookahead() == '\'' {
			lexer.Advance(false)
			s.level = level
			lexer.MarkEnd()
			lexer.SetResultSymbol(nushellSymRawStringBegin)
			return true
		}
		return false
	}

	// RAW_STRING_CONTENT: everything up to the closing '#+
	if nushellValid(validSymbols, nushellTokRawStringContent) && s.level != 0 {
		for lexer.Lookahead() != 0 {
			lexer.MarkEnd()
			lexer.Advance(false)
			// Count consecutive '#' after current char
			var hashCount uint8
			for lexer.Lookahead() == '#' && lexer.Lookahead() != 0 {
				lexer.Advance(false)
				hashCount++
			}
			if hashCount == s.level {
				lexer.SetResultSymbol(nushellSymRawStringContent)
				return true
			}
		}
		return false
	}

	// RAW_STRING_END: '#+  (closing delimiter)
	if nushellValid(validSymbols, nushellTokRawStringEnd) && s.level != 0 && lexer.Lookahead() == '\'' {
		lexer.Advance(false) // consume '
		remaining := s.level
		for remaining > 0 {
			lexer.Advance(false) // consume #
			remaining--
		}
		s.level = 0
		lexer.MarkEnd()
		lexer.SetResultSymbol(nushellSymRawStringEnd)
		return true
	}

	return false
}

func nushellValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
