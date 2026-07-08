//go:build !grammar_subset || grammar_subset_just

package grammars

import (
	"sync"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the just grammar.
const (
	justTokIndent        = 0
	justTokDedent        = 1
	justTokNewline       = 2
	justTokText          = 3
	justTokErrorRecovery = 4
)

// justSyms caches resolved external symbol IDs for the just grammar.
var justSyms struct {
	once          sync.Once
	indent        gotreesitter.Symbol
	dedent        gotreesitter.Symbol
	newline       gotreesitter.Symbol
	text          gotreesitter.Symbol
	errorRecovery gotreesitter.Symbol
}

func resolveJustSyms() {
	justSyms.once.Do(func() {
		lang := JustLanguage()
		justSyms.indent = lang.ExternalSymbols[justTokIndent]
		justSyms.dedent = lang.ExternalSymbols[justTokDedent]
		justSyms.newline = lang.ExternalSymbols[justTokNewline]
		justSyms.text = lang.ExternalSymbols[justTokText]
		justSyms.errorRecovery = lang.ExternalSymbols[justTokErrorRecovery]
	})
}

// justState tracks indent level and brace advancement for just files.
type justState struct {
	prevIndent        uint32
	advanceBraceCount uint16
	hasSeenEof        bool
}

// JustExternalScanner handles indent/dedent/newline/text for justfiles.
type JustExternalScanner struct{}

func (JustExternalScanner) Create() any         { return &justState{} }
func (JustExternalScanner) Destroy(payload any) {}

func (JustExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*justState)
	if len(buf) < 7 {
		return 0
	}
	buf[0] = byte(s.prevIndent)
	buf[1] = byte(s.prevIndent >> 8)
	buf[2] = byte(s.prevIndent >> 16)
	buf[3] = byte(s.prevIndent >> 24)
	buf[4] = byte(s.advanceBraceCount)
	buf[5] = byte(s.advanceBraceCount >> 8)
	if s.hasSeenEof {
		buf[6] = 1
	} else {
		buf[6] = 0
	}
	return 7
}

func (JustExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*justState)
	s.prevIndent = 0
	s.advanceBraceCount = 0
	s.hasSeenEof = false
	if len(buf) >= 7 {
		s.prevIndent = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
		s.advanceBraceCount = uint16(buf[4]) | uint16(buf[5])<<8
		s.hasSeenEof = buf[6] != 0
	}
}

func (JustExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	resolveJustSyms()
	s := payload.(*justState)

	if lexer.Lookahead() == 0 {
		return justHandleEof(lexer, s, validSymbols)
	}

	// NEWLINE
	if justValid(validSymbols, justTokNewline) {
		escape := false
		if lexer.Lookahead() == '\\' {
			escape = true
			lexer.Advance(true)
		}

		eolFound := false
		for unicode.IsSpace(lexer.Lookahead()) {
			if lexer.Lookahead() == '\n' {
				lexer.Advance(true)
				eolFound = true
				break
			}
			lexer.Advance(true)
		}

		if eolFound && !escape {
			lexer.MarkEnd()
			lexer.SetResultSymbol(justSyms.newline)
			return true
		}
	}

	// INDENT / DEDENT
	if justValid(validSymbols, justTokIndent) || justValid(validSymbols, justTokDedent) {
		for lexer.Lookahead() != 0 && justIsSpace(lexer.Lookahead()) {
			switch lexer.Lookahead() {
			case '\n':
				if justValid(validSymbols, justTokIndent) {
					return false
				}
				lexer.Advance(true)
			case '\t', ' ':
				lexer.Advance(true)
			default:
				return false
			}
		}

		if lexer.Lookahead() == 0 {
			return justHandleEof(lexer, s, validSymbols)
		}

		indent := lexer.Column()

		if indent > s.prevIndent && justValid(validSymbols, justTokIndent) && s.prevIndent == 0 {
			lexer.MarkEnd()
			lexer.SetResultSymbol(justSyms.indent)
			s.prevIndent = indent
			return true
		}
		if indent < s.prevIndent && justValid(validSymbols, justTokDedent) && indent == 0 {
			lexer.MarkEnd()
			lexer.SetResultSymbol(justSyms.dedent)
			s.prevIndent = indent
			return true
		}
	}

	// TEXT
	if justValid(validSymbols, justTokText) {
		// Don't start text at column == prevIndent for certain chars
		if lexer.Column() == s.prevIndent &&
			(lexer.Lookahead() == '\n' || lexer.Lookahead() == '@' || lexer.Lookahead() == '-') {
			return false
		}

		advancedOnce := false

		// Advance past braces tracked from previous interpolation
		for lexer.Lookahead() == '{' && s.advanceBraceCount > 0 && lexer.Lookahead() != 0 {
			s.advanceBraceCount--
			lexer.Advance(false)
			advancedOnce = true
		}

		for {
			if lexer.Lookahead() == 0 {
				return justHandleEof(lexer, s, validSymbols)
			}

			// Consume until newline or '{'
			for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' && lexer.Lookahead() != '{' {
				if lexer.Lookahead() == '#' && !advancedOnce {
					lexer.Advance(false)
					if lexer.Lookahead() == '!' {
						return false
					}
				}
				lexer.Advance(false)
				advancedOnce = true
			}

			if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
				lexer.MarkEnd()
				lexer.SetResultSymbol(justSyms.text)
				if advancedOnce {
					return true
				}
				if lexer.Lookahead() == 0 {
					return justHandleEof(lexer, s, validSymbols)
				}
				lexer.Advance(false)
			} else if lexer.Lookahead() == '{' {
				lexer.MarkEnd()
				lexer.Advance(false)

				if lexer.Lookahead() == 0 || lexer.Lookahead() == '\n' {
					lexer.MarkEnd()
					lexer.SetResultSymbol(justSyms.text)
					return advancedOnce
				}

				if lexer.Lookahead() == '{' {
					lexer.Advance(false)

					for lexer.Lookahead() == '{' {
						s.advanceBraceCount++
						lexer.Advance(false)
					}

					// Scan till balanced }}
					for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
						lexer.Advance(false)
						if lexer.Lookahead() == '}' {
							lexer.Advance(false)
							if lexer.Lookahead() == '}' {
								lexer.SetResultSymbol(justSyms.text)
								return advancedOnce
							}
						}
					}

					if !advancedOnce {
						return false
					}
				}
			}
		}
	}

	return false
}

func justHandleEof(lexer *gotreesitter.ExternalLexer, s *justState, validSymbols []bool) bool {
	lexer.MarkEnd()

	if justValid(validSymbols, justTokDedent) {
		lexer.SetResultSymbol(justSyms.dedent)
		return true
	}

	if justValid(validSymbols, justTokNewline) {
		if s.hasSeenEof {
			return false
		}
		lexer.SetResultSymbol(justSyms.newline)
		s.hasSeenEof = true
		return true
	}
	return false
}

func justIsSpace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
}

func justValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
