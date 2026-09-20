//go:build !grammar_subset || grammar_subset_lua

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the lua grammar (enum TokenType in scanner.c).
// This is the external index (the position of the token in the grammar's
// `externals: [...]` list), which is exactly what tree-sitter's
// `valid_symbols` array and C's result_symbol enum are indexed by. The
// external index is stable across a blob regen as long as the externals
// list itself does not reorder; concrete numeric gotreesitter.Symbol IDs
// are NOT stable (they shift whenever the grammar's total symbol count
// changes), so this scanner never hardcodes them -- see luaDefaultSymTable
// below.
const (
	luaTokBlockCommentStart   = 0
	luaTokBlockCommentContent = 1
	luaTokBlockCommentEnd     = 2
	luaTokBlockStringStart    = 3
	luaTokBlockStringContent  = 4
	luaTokBlockStringEnd      = 5
	luaTokenCount             = 6
)

// luaDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped lua.bin assigns to each external, in luaTok* order. It
// exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order. _block_comment_start/_block_string_start display as the literal
// "[[" and _block_comment_end/_block_string_end display as the literal
// "]]", each sharing a Symbol ID with every other occurrence of that
// literal elsewhere in the grammar, exactly like blade's "/>" external.
var luaDefaultSymTable = [luaTokenCount]gotreesitter.Symbol{
	67, // _block_comment_start (display: "[[")
	68, // _block_comment_content (display: comment_content)
	69, // _block_comment_end (display: "]]")
	70, // _block_string_start (display: "[[")
	71, // _block_string_content (display: string_content)
	72, // _block_string_end (display: "]]")
}

// luaExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (luaTok* order).
var luaExternalScannerSpec = ExternalScannerSpec{
	Language:       "lua",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-lua",
	UpstreamCommit: "10fe0054734eec83049514ea2e718b2a56acd0c9",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "016172714e10e7b5a3b25433cfd2a9626c983c6ede228fcc8301537da516e462"},
		{Path: "src/scanner.c", SHA256: "35bbd630b5a7421d46d2e91185eeea09bf78565d44cb676b63ca20d0f1b54bbd"},
	},
	Externals: []string{
		"_block_comment_start",
		"_block_comment_content",
		"_block_comment_end",
		"_block_string_start",
		"_block_string_content",
		"_block_string_end",
	},
}

func init() {
	RegisterExternalScannerSpec(luaExternalScannerSpec)
}

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
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type LuaExternalScanner struct {
	symbols         [luaTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers lua's external symbols.
func (LuaExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := LuaExternalScanner{symbols: luaDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, luaExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s LuaExternalScanner) symbolTable() *[luaTokenCount]gotreesitter.Symbol {
	if s.symbols == ([luaTokenCount]gotreesitter.Symbol{}) {
		return &luaDefaultSymTable
	}
	return &s.symbols
}

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

func luaScanCommentStart(s *luaScannerState, lexer *gotreesitter.ExternalLexer, blockCommentStartSym gotreesitter.Symbol) bool {
	if luaConsumeString("--", lexer) {
		lexer.MarkEnd()
		if luaScanBlockStart(s, lexer) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(blockCommentStartSym)
			return true
		}
	}
	return false
}

func luaScanCommentContent(s *luaScannerState, lexer *gotreesitter.ExternalLexer, blockCommentContentSym gotreesitter.Symbol) bool {
	if s.endingChar == 0 { // block comment
		if luaScanBlockContent(s, lexer) {
			lexer.SetResultSymbol(blockCommentContentSym)
			return true
		}
		return false
	}

	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == rune(s.endingChar) {
			s.reset()
			lexer.SetResultSymbol(blockCommentContentSym)
			return true
		}
		lexer.Advance(false)
	}
	return false
}

func (s LuaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state, ok := payload.(*luaScannerState)
	if !ok {
		return false
	}

	if len(s.externalToToken) > 0 {
		var semanticValid [luaTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < luaTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	if luaValidSym(validSymbols, luaTokBlockStringEnd) && luaScanBlockEnd(state, lexer) {
		state.reset()
		lexer.SetResultSymbol(syms[luaTokBlockStringEnd])
		return true
	}

	if luaValidSym(validSymbols, luaTokBlockStringContent) && luaScanBlockContent(state, lexer) {
		lexer.SetResultSymbol(syms[luaTokBlockStringContent])
		return true
	}

	if luaValidSym(validSymbols, luaTokBlockCommentEnd) && state.endingChar == 0 && luaScanBlockEnd(state, lexer) {
		state.reset()
		lexer.SetResultSymbol(syms[luaTokBlockCommentEnd])
		return true
	}

	if luaValidSym(validSymbols, luaTokBlockCommentContent) && luaScanCommentContent(state, lexer, syms[luaTokBlockCommentContent]) {
		return true
	}

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if luaValidSym(validSymbols, luaTokBlockStringStart) && luaScanBlockStart(state, lexer) {
		lexer.SetResultSymbol(syms[luaTokBlockStringStart])
		return true
	}

	if luaValidSym(validSymbols, luaTokBlockCommentStart) && luaScanCommentStart(state, lexer, syms[luaTokBlockCommentStart]) {
		return true
	}

	return false
}

func luaValidSym(vs []bool, i int) bool { return i < len(vs) && vs[i] }
