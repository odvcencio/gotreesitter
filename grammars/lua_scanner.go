//go:build !grammar_subset || grammar_subset_lua

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the lua grammar (enum TokenType in scanner.c).
const (
	luaTokBlockCommentStart   = 0
	luaTokBlockCommentContent = 1
	luaTokBlockCommentEnd     = 2
	luaTokBlockStringStart    = 3
	luaTokBlockStringContent  = 4
	luaTokBlockStringEnd      = 5
)

// External token symbols (ExternalSymbols in the lua blob: indexes 0..5 map
// to symbols 67..72).
const (
	luaSymBlockCommentStart   gotreesitter.Symbol = 67
	luaSymBlockCommentContent gotreesitter.Symbol = 68
	luaSymBlockCommentEnd     gotreesitter.Symbol = 69
	luaSymBlockStringStart    gotreesitter.Symbol = 70
	luaSymBlockStringContent  gotreesitter.Symbol = 71
	luaSymBlockStringEnd      gotreesitter.Symbol = 72
)

// luaExternalLexStates mirrors ts_external_scanner_states in the pinned
// upstream parser.c (tree-sitter-grammars/tree-sitter-lua @ 10fe0054).
var luaExternalLexStates = [][]bool{
	0: {false, false, false, false, false, false},
	1: {true, true, true, true, true, true},
	2: {true, false, false, false, false, false},
	3: {true, false, false, true, false, false},
	4: {true, false, false, false, false, true},
	5: {true, true, false, false, false, false},
	6: {true, false, true, false, false, false},
	7: {true, false, false, false, true, false},
}

// luaScannerState mirrors the Scanner struct in upstream scanner.c.
// ending_char is vestigial upstream (only ever written as 0 by reset_state),
// but it participates in serialization, so it is kept for byte parity.
type luaScannerState struct {
	endingChar byte
	levelCount uint8
}

func (s *luaScannerState) reset() {
	s.endingChar = 0
	s.levelCount = 0
}

// LuaExternalScanner is a line-faithful port of the pinned upstream
// src/scanner.c (tree-sitter-grammars/tree-sitter-lua @ 10fe0054). It scans
// long-bracket block strings/comments: [[ ... ]], [=[ ... ]=], etc.
type LuaExternalScanner struct{}

func (LuaExternalScanner) Create() any         { return &luaScannerState{} }
func (LuaExternalScanner) Destroy(payload any) {}

func (LuaExternalScanner) Serialize(payload any, buf []byte) int {
	s, ok := payload.(*luaScannerState)
	if !ok || len(buf) < 2 {
		return 0
	}
	buf[0] = s.endingChar
	buf[1] = byte(s.levelCount)
	return 2
}

func (LuaExternalScanner) Deserialize(payload any, buf []byte) {
	s, ok := payload.(*luaScannerState)
	if !ok {
		return
	}
	// C: if (length == 0) return;  — state is left untouched, not reset.
	if len(buf) == 0 {
		return
	}
	s.endingChar = buf[0]
	if len(buf) == 1 {
		return
	}
	s.levelCount = buf[1]
}

func luaConsumeChar(c rune, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != c {
		return false
	}
	lexer.Advance(false)
	return true
}

func luaConsumeString(s string, lexer *gotreesitter.ExternalLexer) bool {
	for _, c := range s {
		if !luaConsumeChar(c, lexer) {
			return false
		}
	}
	return true
}

func luaConsumeAndCountChar(c rune, lexer *gotreesitter.ExternalLexer) uint8 {
	var count uint8
	for lexer.Lookahead() == c {
		count++ // uint8 wrap matches C's uint8_t overflow
		lexer.Advance(false)
	}
	return count
}

func luaScanBlockStart(s *luaScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if luaConsumeChar('[', lexer) {
		level := luaConsumeAndCountChar('=', lexer)
		if luaConsumeChar('[', lexer) {
			s.levelCount = level
			return true
		}
	}
	return false
}

func luaScanBlockEnd(s *luaScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if luaConsumeChar(']', lexer) {
		level := luaConsumeAndCountChar('=', lexer)
		if s.levelCount == level && luaConsumeChar(']', lexer) {
			return true
		}
	}
	return false
}

func luaScanBlockContent(s *luaScannerState, lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == ']' {
			lexer.MarkEnd()
			if luaScanBlockEnd(s, lexer) {
				return true
			}
		} else {
			lexer.Advance(false)
		}
	}
	return false
}

func luaScanCommentStart(s *luaScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if luaConsumeString("--", lexer) {
		lexer.MarkEnd()
		if luaScanBlockStart(s, lexer) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(luaSymBlockCommentStart)
			return true
		}
	}
	return false
}

func luaScanCommentContent(s *luaScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if s.endingChar == 0 { // block comment
		if luaScanBlockContent(s, lexer) {
			lexer.SetResultSymbol(luaSymBlockCommentContent)
			return true
		}
		return false
	}

	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == rune(s.endingChar) {
			s.reset()
			lexer.SetResultSymbol(luaSymBlockCommentContent)
			return true
		}
		lexer.Advance(false)
	}
	return false
}

func (LuaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s, ok := payload.(*luaScannerState)
	if !ok {
		return false
	}

	if luaValidSym(validSymbols, luaTokBlockStringEnd) && luaScanBlockEnd(s, lexer) {
		s.reset()
		lexer.SetResultSymbol(luaSymBlockStringEnd)
		return true
	}

	if luaValidSym(validSymbols, luaTokBlockStringContent) && luaScanBlockContent(s, lexer) {
		lexer.SetResultSymbol(luaSymBlockStringContent)
		return true
	}

	if luaValidSym(validSymbols, luaTokBlockCommentEnd) && s.endingChar == 0 && luaScanBlockEnd(s, lexer) {
		s.reset()
		lexer.SetResultSymbol(luaSymBlockCommentEnd)
		return true
	}

	if luaValidSym(validSymbols, luaTokBlockCommentContent) && luaScanCommentContent(s, lexer) {
		return true
	}

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if luaValidSym(validSymbols, luaTokBlockStringStart) && luaScanBlockStart(s, lexer) {
		lexer.SetResultSymbol(luaSymBlockStringStart)
		return true
	}

	if luaValidSym(validSymbols, luaTokBlockCommentStart) && luaScanCommentStart(s, lexer) {
		return true
	}

	return false
}

func luaValidSym(vs []bool, i int) bool { return i < len(vs) && vs[i] }
