//go:build !grammar_subset || grammar_subset_cairo

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the cairo grammar.
const (
	cairoTokHintStart      = 0
	cairoTokPythonCodeLine = 1
	cairoTokFailure        = 2
)

const (
	cairoSymHintStart      gotreesitter.Symbol = 56
	cairoSymPythonCodeLine gotreesitter.Symbol = 136
	cairoSymFailure        gotreesitter.Symbol = 137
)

// Cairo scanner context
const (
	cairoCtxNone          = 0
	cairoCtxPythonCode    = 1
	cairoCtxPythonString  = 2
	cairoCtxPythonComment = 3
)

// Python string type
const (
	cairoPstNone      = 0
	cairoPst1SqString = 1
	cairoPst3SqString = 2
	cairoPst1DqString = 3
	cairoPst3DqString = 4
)

// cairoState tracks the hint parsing context.
type cairoState struct {
	wsCount uint32
	context uint8
	pst     uint8
}

// CairoExternalScanner handles %{ %} hint blocks with embedded Python in Cairo.
type CairoExternalScanner struct{}

func (CairoExternalScanner) Create() any         { return &cairoState{} }
func (CairoExternalScanner) Destroy(payload any) {}

func (CairoExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*cairoState)
	if len(buf) < 6 {
		return 0
	}
	buf[0] = byte(s.wsCount)
	buf[1] = byte(s.wsCount >> 8)
	buf[2] = byte(s.wsCount >> 16)
	buf[3] = byte(s.wsCount >> 24)
	buf[4] = s.context
	buf[5] = s.pst
	return 6
}

func (CairoExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*cairoState)
	*s = cairoState{}
	if len(buf) >= 6 {
		s.wsCount = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
		s.context = buf[4]
		s.pst = buf[5]
	}
}

func (CairoExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*cairoState)

	if cairoValid(validSymbols, cairoTokFailure) {
		return false
	}

	// HINT_START: detect %{ to enter hint mode
	if cairoValid(validSymbols, cairoTokHintStart) {
		if lexer.Lookahead() == '%' {
			lexer.Advance(true)
			if lexer.Lookahead() == '{' {
				s.context = cairoCtxPythonCode
				return false // let built-in lexer handle the actual token
			}
		}
	}

	// PYTHON_CODE_LINE: scan lines of Python code inside %{ %}
	if cairoValid(validSymbols, cairoTokPythonCodeLine) {
		// Skip leading newline after %{
		if lexer.Lookahead() == '\n' {
			lexer.Advance(true)
		}

		// Check for standalone %} close
		if lexer.Lookahead() == '%' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '}' {
				if s.context == cairoCtxPythonString {
					lexer.SetResultSymbol(cairoSymFailure)
					return true
				}
				s.context = cairoCtxNone
				return false
			}
		}

		// Skip leading whitespace, count it
		var wsCount uint32
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == '\n' {
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(cairoSymPythonCodeLine)
				return true
			}
			if unicode.IsSpace(lexer.Lookahead()) {
				if lexer.Lookahead() == '\t' {
					wsCount += 8
				} else {
					wsCount++
				}
				lexer.Advance(true)
				if s.wsCount > 0 && wsCount == s.wsCount {
					break
				}
			} else {
				if s.wsCount == 0 || wsCount < s.wsCount {
					s.wsCount = wsCount
				}
				break
			}
		}

		// Scan content of line
		contentLen := uint32(0)
		for lexer.Lookahead() != 0 {
			switch lexer.Lookahead() {
			case '\'', '"':
				chr := lexer.Lookahead()
				lexer.Advance(false)
				contentLen++
				if s.context == cairoCtxPythonString {
					iter := 0
					if s.pst == cairoPst3DqString || s.pst == cairoPst3SqString {
						iter = 2
					}
					for iter > 0 {
						if lexer.Lookahead() != chr {
							s.context = cairoCtxPythonCode
							s.pst = cairoPstNone
							return false
						}
						lexer.Advance(false)
						contentLen++
						iter--
					}
					s.context = cairoCtxPythonCode
					s.pst = cairoPstNone
					continue
				}
				if lexer.Lookahead() == chr {
					lexer.Advance(false)
					contentLen++
					if lexer.Lookahead() == chr {
						lexer.Advance(false)
						contentLen++
						s.context = cairoCtxPythonString
						if chr == '"' {
							s.pst = cairoPst3DqString
						} else {
							s.pst = cairoPst3SqString
						}
					} else {
						s.context = cairoCtxPythonCode
						s.pst = cairoPstNone
					}
				} else {
					s.context = cairoCtxPythonString
					if chr == '"' {
						s.pst = cairoPst1DqString
					} else {
						s.pst = cairoPst1SqString
					}
				}
				continue

			case '%':
				if s.context == cairoCtxPythonString {
					lexer.Advance(false)
					contentLen++
					continue
				}
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '}' {
					if s.context == cairoCtxPythonString {
						lexer.SetResultSymbol(cairoSymFailure)
						return true
					}
					s.context = cairoCtxNone
					if contentLen > 0 {
						lexer.SetResultSymbol(cairoSymPythonCodeLine)
						return true
					}
					return false
				}

			case '\n':
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(cairoSymPythonCodeLine)
				return true

			case '#':
				if s.context == cairoCtxPythonString {
					lexer.Advance(false)
					contentLen++
					continue
				}
				s.context = cairoCtxPythonComment
				for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
					lexer.Advance(false)
					contentLen++
				}
				s.context = cairoCtxNone
				continue

			default:
				lexer.Advance(false)
				contentLen++
			}
		}
	}

	return false
}

func cairoValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
