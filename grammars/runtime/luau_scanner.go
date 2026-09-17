//go:build !grammar_subset || grammar_subset_luau

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the luau grammar.
const (
	luauTokBlockCommentStart   = 0
	luauTokBlockCommentContent = 1
	luauTokBlockCommentEnd     = 2
	luauTokStringStart         = 3
	luauTokStringContent       = 4
	luauTokStringEnd           = 5
)

const (
	luauSymBlockCommentStart   gotreesitter.Symbol = 91
	luauSymBlockCommentContent gotreesitter.Symbol = 92
	luauSymBlockCommentEnd     gotreesitter.Symbol = 93
	luauSymStringStart         gotreesitter.Symbol = 94
	luauSymStringContent       gotreesitter.Symbol = 95
	luauSymStringEnd           gotreesitter.Symbol = 96
)

// luauState stores the ending character and the bracket level count.
type luauState struct {
	endingChar rune
	levelCount uint8
}

// LuauExternalScanner handles Lua-style block comments --[=[ ... ]=]
// and block/quoted strings for Luau.
type LuauExternalScanner struct{}

func (LuauExternalScanner) Create() any         { return &luauState{} }
func (LuauExternalScanner) Destroy(payload any) {}
func (LuauExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*luauState)
	buf[0] = byte(s.endingChar)
	buf[1] = s.levelCount
	return 2
}
func (LuauExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*luauState)
	if len(buf) >= 1 {
		s.endingChar = rune(buf[0])
	}
	if len(buf) >= 2 {
		s.levelCount = buf[1]
	}
}

func (LuauExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*luauState)

	// String end
	if luauValid(validSymbols, luauTokStringEnd) && luauScanStringEnd(s, lexer) {
		luauResetState(s)
		lexer.SetResultSymbol(luauSymStringEnd)
		return true
	}

	// String content
	if luauValid(validSymbols, luauTokStringContent) && luauScanStringContent(s, lexer) {
		lexer.SetResultSymbol(luauSymStringContent)
		return true
	}

	// Block comment end
	if luauValid(validSymbols, luauTokBlockCommentEnd) && s.endingChar == 0 && luauScanBlockEnd(s, lexer) {
		luauResetState(s)
		lexer.SetResultSymbol(luauSymBlockCommentEnd)
		return true
	}

	// Block comment content
	if luauValid(validSymbols, luauTokBlockCommentContent) && luauScanCommentContent(s, lexer) {
		return true
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// String start
	if luauValid(validSymbols, luauTokStringStart) && luauScanStringStart(s, lexer) {
		lexer.SetResultSymbol(luauSymStringStart)
		return true
	}

	// Block comment start: --[=[
	if luauValid(validSymbols, luauTokBlockCommentStart) && luauScanCommentStart(s, lexer) {
		return true
	}

	return false
}

func luauResetState(s *luauState) {
	s.endingChar = 0
	s.levelCount = 0
}

func luauScanBlockStart(s *luauState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '[' {
		return false
	}
	lexer.Advance(false)
	level := uint8(0)
	for lexer.Lookahead() == '=' {
		level++
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '[' {
		return false
	}
	lexer.Advance(false)
	s.levelCount = level
	return true
}

func luauScanBlockEnd(s *luauState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != ']' {
		return false
	}
	lexer.Advance(false)
	level := uint8(0)
	for lexer.Lookahead() == '=' {
		level++
		lexer.Advance(false)
	}
	if s.levelCount == level && lexer.Lookahead() == ']' {
		lexer.Advance(false)
		return true
	}
	return false
}

func luauScanBlockContent(s *luauState, lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == ']' {
			lexer.MarkEnd()
			if luauScanBlockEnd(s, lexer) {
				return true
			}
		} else {
			lexer.Advance(false)
		}
	}
	return false
}

func luauScanCommentStart(s *luauState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	if luauScanBlockStart(s, lexer) {
		lexer.MarkEnd()
		lexer.SetResultSymbol(luauSymBlockCommentStart)
		return true
	}
	return false
}

func luauScanCommentContent(s *luauState, lexer *gotreesitter.ExternalLexer) bool {
	if s.endingChar == 0 {
		if luauScanBlockContent(s, lexer) {
			lexer.SetResultSymbol(luauSymBlockCommentContent)
			return true
		}
		return false
	}
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == s.endingChar {
			luauResetState(s)
			lexer.SetResultSymbol(luauSymBlockCommentContent)
			return true
		}
		lexer.Advance(false)
	}
	return false
}

func luauScanStringStart(s *luauState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' {
		s.endingChar = lexer.Lookahead()
		lexer.Advance(false)
		return true
	}
	if luauScanBlockStart(s, lexer) {
		return true
	}
	return false
}

func luauScanStringEnd(s *luauState, lexer *gotreesitter.ExternalLexer) bool {
	if s.endingChar == 0 {
		return luauScanBlockEnd(s, lexer)
	}
	if lexer.Lookahead() == s.endingChar {
		lexer.Advance(false)
		return true
	}
	return false
}

func luauScanStringContent(s *luauState, lexer *gotreesitter.ExternalLexer) bool {
	if s.endingChar == 0 {
		return luauScanBlockContent(s, lexer)
	}
	for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 && lexer.Lookahead() != s.endingChar {
		if lexer.Lookahead() == '\\' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'z' {
				lexer.Advance(false)
				for unicode.IsSpace(lexer.Lookahead()) {
					lexer.Advance(false)
				}
				continue
			}
		}
		if lexer.Lookahead() == 0 {
			return true
		}
		lexer.Advance(false)
	}
	return true
}

func luauValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
