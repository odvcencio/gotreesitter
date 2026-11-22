//go:build !grammar_subset || grammar_subset_cpp

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// The C-faithful mode of the cpp scanner: the pinned C scanner
// (tree-sitter-cpp c0092228, src/scanner.c) step by step. The default engine
// keeps the shared raw-string code, which the arduino, cuda and hlsl scanners
// also use.

// cppSerializeC writes delimiter_length * 4 bytes, each delimiter character as a
// little-endian 4-byte wchar_t, and nothing for an empty delimiter
// (scanner.c:122-129).
func cppSerializeC(s *rawStringState, buf []byte) int {
	n := 0
	for _, r := range s.delimiter {
		if n+4 > len(buf) {
			break
		}
		u := uint32(int32(r))
		buf[n], buf[n+1], buf[n+2], buf[n+3] = byte(u), byte(u>>8), byte(u>>16), byte(u>>24)
		n += 4
	}
	return n
}

// cppDeserializeC sets delimiter_length = length / 4 and copies the bytes
// (scanner.c:132-140). C asserts when length is not a multiple of 4 (:133); this
// port drops the odd bytes. C copies a longer state past its 16-character array;
// this port keeps the first 16 characters.
func cppDeserializeC(s *rawStringState, buf []byte) {
	n := len(buf) / 4
	if n > maxRawStringDelimiterLen {
		n = maxRawStringDelimiterLen
	}
	s.delimiter = s.delimiter[:0]
	for i := 0; i < n; i++ {
		u := uint32(buf[4*i]) | uint32(buf[4*i+1])<<8 | uint32(buf[4*i+2])<<16 | uint32(buf[4*i+3])<<24
		s.delimiter = append(s.delimiter, rune(int32(u)))
	}
}

// cppIsSpaceC is iswspace in the C locale: ASCII white space only (scanner.c:45).
func cppIsSpaceC(c int32) bool {
	return c == ' ' || (c >= '\t' && c <= '\r')
}

// cppScanC is tree_sitter_cpp_external_scanner_scan (scanner.c:101-120).
func cppScanC(s *rawStringState, lexer *gotreesitter.ExternalLexer, valid []bool, symDelim, symContent gotreesitter.Symbol) bool {
	delimValid := rawStringValid(valid, cppTokRawStringDelimiter)
	contentValid := rawStringValid(valid, cppTokRawStringContent)
	if delimValid && contentValid { // :104 error recovery
		return false
	}
	if delimValid {
		lexer.SetResultSymbol(symDelim) // :111 set before the scan, kept on failure
		return cppScanDelimiterC(s, lexer)
	}
	if contentValid {
		lexer.SetResultSymbol(symContent)
		return cppScanContentC(s, lexer)
	}
	return false
}

// cppScanDelimiterC is scan_raw_string_delimiter (scanner.c:27-57). It never calls
// mark_end, so the token ends at the cursor. A failed opening scan leaves the
// characters read so far in the delimiter (:43-55 never reset it), and a failed
// closing scan changes nothing.
func cppScanDelimiterC(s *rawStringState, lexer *gotreesitter.ExternalLexer) bool {
	if len(s.delimiter) > 0 { // :28
		for _, d := range s.delimiter {
			if scanmode.CLookahead(lexer) != int32(d) { // :33
				return false
			}
			lexer.Advance(false)
		}
		s.delimiter = s.delimiter[:0] // :39 reset
		return true
	}
	for {
		la := scanmode.CLookahead(lexer)
		if len(s.delimiter) >= maxRawStringDelimiterLen || scanmode.AtEOF(lexer) || la == '\\' || cppIsSpaceC(la) { // :44-45
			return false
		}
		if la == '(' { // :48
			return len(s.delimiter) > 0 // :51
		}
		s.delimiter = append(s.delimiter, rune(la)) // :53
		lexer.Advance(false)
	}
}

// cppScanContentC is scan_raw_string_content (scanner.c:60-96).
func cppScanContentC(s *rawStringState, lexer *gotreesitter.ExternalLexer) bool {
	idx := -1
	for {
		if scanmode.AtEOF(lexer) { // :67 end of input, not a NUL byte
			lexer.MarkEnd()
			return true
		}
		la := scanmode.CLookahead(lexer)
		if idx >= 0 {
			if idx == len(s.delimiter) {
				if la == '"' {
					return true
				}
				idx = -1
			} else if la == int32(s.delimiter[idx]) {
				idx++
			} else {
				idx = -1
			}
		}
		if idx == -1 && la == ')' {
			lexer.MarkEnd()
			idx = 0
		}
		lexer.Advance(false)
	}
}
