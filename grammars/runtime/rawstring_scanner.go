//go:build !grammar_subset || grammar_subset_arduino || grammar_subset_cpp || grammar_subset_cuda || grammar_subset_hlsl

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// rawStringState stores the delimiter for C++ R"delimiter(...)delimiter" raw strings.
// Shared by cuda, arduino, and hlsl scanners.
type rawStringState struct {
	delimiter []rune
}

const maxRawStringDelimiterLen = 16

func rawStringCreate() any { return &rawStringState{} }

func rawStringSerialize(payload any, buf []byte) int {
	s := payload.(*rawStringState)
	n := 0
	for _, r := range s.delimiter {
		if n+4 > len(buf) {
			break
		}
		// Store as UTF-8
		size := runeLen(r)
		encodeRune(buf[n:], r)
		n += size
	}
	return n
}

func rawStringDeserialize(payload any, buf []byte) {
	s := payload.(*rawStringState)
	s.delimiter = s.delimiter[:0]
	i := 0
	for i < len(buf) {
		r, size := decodeRune(buf[i:])
		if size == 0 {
			break
		}
		s.delimiter = append(s.delimiter, r)
		i += size
	}
}

func rawStringScan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool, tokDelim, tokContent int, symDelim, symContent gotreesitter.Symbol) bool {
	delimValid := rawStringValid(validSymbols, tokDelim)
	contentValid := rawStringValid(validSymbols, tokContent)

	// Error recovery
	if delimValid && contentValid {
		return false
	}

	s := payload.(*rawStringState)

	if delimValid {
		if !scanRawStringDelimiter(s, lexer) {
			return false
		}
		lexer.SetResultSymbol(symDelim)
		return true
	}

	if contentValid {
		if !scanRawStringContent(s, lexer) {
			return false
		}
		lexer.SetResultSymbol(symContent)
		return true
	}

	return false
}

func scanRawStringDelimiter(s *rawStringState, lexer *gotreesitter.ExternalLexer) bool {
	if len(s.delimiter) > 0 {
		// Closing delimiter: must match opening
		for _, r := range s.delimiter {
			if lexer.Lookahead() != r {
				return false
			}
			lexer.Advance(false)
		}
		s.delimiter = s.delimiter[:0]
		lexer.MarkEnd()
		return true
	}

	// Opening delimiter: record d-char-sequence up to (
	s.delimiter = s.delimiter[:0]
	for {
		ch := lexer.Lookahead()
		if len(s.delimiter) >= maxRawStringDelimiterLen || ch == 0 || ch == '\\' || unicode.IsSpace(ch) {
			return false
		}
		if ch == '(' {
			if len(s.delimiter) > 0 {
				lexer.MarkEnd()
				return true
			}
			return false
		}
		s.delimiter = append(s.delimiter, ch)
		lexer.Advance(false)
	}
}

func scanRawStringContent(s *rawStringState, lexer *gotreesitter.ExternalLexer) bool {
	delimIdx := -1
	for {
		if lexer.Lookahead() == 0 {
			lexer.MarkEnd()
			return true
		}

		if delimIdx >= 0 {
			if delimIdx == len(s.delimiter) {
				if lexer.Lookahead() == '"' {
					return true
				}
				delimIdx = -1
			} else {
				if lexer.Lookahead() == s.delimiter[delimIdx] {
					delimIdx++
				} else {
					delimIdx = -1
				}
			}
		}

		if delimIdx == -1 && lexer.Lookahead() == ')' {
			lexer.MarkEnd()
			delimIdx = 0
		}

		lexer.Advance(false)
	}
}

func rawStringValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

// Simple UTF-8 helpers to avoid importing "unicode/utf8" (keep binary small).

func runeLen(r rune) int {
	switch {
	case r < 0x80:
		return 1
	case r < 0x800:
		return 2
	case r < 0x10000:
		return 3
	default:
		return 4
	}
}

func encodeRune(buf []byte, r rune) {
	switch {
	case r < 0x80:
		buf[0] = byte(r)
	case r < 0x800:
		buf[0] = byte(0xC0 | (r >> 6))
		buf[1] = byte(0x80 | (r & 0x3F))
	case r < 0x10000:
		buf[0] = byte(0xE0 | (r >> 12))
		buf[1] = byte(0x80 | ((r >> 6) & 0x3F))
		buf[2] = byte(0x80 | (r & 0x3F))
	default:
		buf[0] = byte(0xF0 | (r >> 18))
		buf[1] = byte(0x80 | ((r >> 12) & 0x3F))
		buf[2] = byte(0x80 | ((r >> 6) & 0x3F))
		buf[3] = byte(0x80 | (r & 0x3F))
	}
}

func decodeRune(buf []byte) (rune, int) {
	if len(buf) == 0 {
		return 0, 0
	}
	b := buf[0]
	switch {
	case b < 0x80:
		return rune(b), 1
	case b < 0xE0:
		if len(buf) < 2 {
			return 0, 0
		}
		return rune(b&0x1F)<<6 | rune(buf[1]&0x3F), 2
	case b < 0xF0:
		if len(buf) < 3 {
			return 0, 0
		}
		return rune(b&0x0F)<<12 | rune(buf[1]&0x3F)<<6 | rune(buf[2]&0x3F), 3
	default:
		if len(buf) < 4 {
			return 0, 0
		}
		return rune(b&0x07)<<18 | rune(buf[1]&0x3F)<<12 | rune(buf[2]&0x3F)<<6 | rune(buf[3]&0x3F), 4
	}
}
