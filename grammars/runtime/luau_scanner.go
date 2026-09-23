//go:build !grammar_subset || grammar_subset_luau

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the luau grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see luauDefaultSymTable below.
const (
	luauTokBlockCommentStart   = 0
	luauTokBlockCommentContent = 1
	luauTokBlockCommentEnd     = 2
	luauTokStringStart         = 3
	luauTokStringContent       = 4
	luauTokStringEnd           = 5
	luauTokenCount             = 6
)

// luauDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped luau.bin assigns to each external, in luauTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var luauDefaultSymTable = [luauTokenCount]gotreesitter.Symbol{
	91, // _block_comment_start, displays as "[["
	92, // _block_comment_content, displays as "comment_content"
	93, // _block_comment_end, displays as "]]"
	94, // _block_string_start, displays as "[["
	95, // _block_string_content, displays as "string_content"
	96, // _block_string_end, displays as "]]"
}

// luauExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (luauTok* order).
var luauExternalScannerSpec = ExternalScannerSpec{
	Language:       "luau",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-luau",
	UpstreamCommit: "a8914d6c1fc5131f8e1c13f769fa704c9f5eb02f",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "f91f98419c0682cf28ce92fea3762cdd0ef30164fc850dd6375b64a7fa44edb8"},
		{Path: "src/scanner.c", SHA256: "a157bb5210454add058a08ce53eabcccad41aab6dac7006def2554e5cfebd376"},
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
	RegisterExternalScannerSpec(luauExternalScannerSpec)
}

// luauState stores the ending character and the bracket level count.
type luauState struct {
	endingChar rune
	levelCount uint8
}

// LuauExternalScanner handles Lua-style block comments --[=[ ... ]=]
// and block/quoted strings for Luau.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type LuauExternalScanner struct {
	symbols         [luauTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers luau's external symbols.
func (LuauExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := LuauExternalScanner{symbols: luauDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, luauExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s LuauExternalScanner) symbolTable() *[luauTokenCount]gotreesitter.Symbol {
	if s.symbols == ([luauTokenCount]gotreesitter.Symbol{}) {
		return &luauDefaultSymTable
	}
	return &s.symbols
}

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

func (sc LuauExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*luauState)

	syms := sc.symbolTable()

	// String end
	if luauValid(validSymbols, luauTokStringEnd) && luauScanStringEnd(s, lexer) {
		luauResetState(s)
		lexer.SetResultSymbol(syms[luauTokStringEnd])
		return true
	}

	// String content
	if luauValid(validSymbols, luauTokStringContent) && luauScanStringContent(s, lexer) {
		lexer.SetResultSymbol(syms[luauTokStringContent])
		return true
	}

	// Block comment end
	if luauValid(validSymbols, luauTokBlockCommentEnd) && s.endingChar == 0 && luauScanBlockEnd(s, lexer) {
		luauResetState(s)
		lexer.SetResultSymbol(syms[luauTokBlockCommentEnd])
		return true
	}

	// Block comment content
	if luauValid(validSymbols, luauTokBlockCommentContent) && luauScanCommentContent(s, lexer, syms) {
		return true
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// String start
	if luauValid(validSymbols, luauTokStringStart) && luauScanStringStart(s, lexer) {
		lexer.SetResultSymbol(syms[luauTokStringStart])
		return true
	}

	// Block comment start: --[=[
	if luauValid(validSymbols, luauTokBlockCommentStart) && luauScanCommentStart(s, lexer, syms) {
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

func luauScanCommentStart(s *luauState, lexer *gotreesitter.ExternalLexer, syms *[luauTokenCount]gotreesitter.Symbol) bool {
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
		lexer.SetResultSymbol(syms[luauTokBlockCommentStart])
		return true
	}
	return false
}

func luauScanCommentContent(s *luauState, lexer *gotreesitter.ExternalLexer, syms *[luauTokenCount]gotreesitter.Symbol) bool {
	if s.endingChar == 0 {
		if luauScanBlockContent(s, lexer) {
			lexer.SetResultSymbol(syms[luauTokBlockCommentContent])
			return true
		}
		return false
	}
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == s.endingChar {
			luauResetState(s)
			lexer.SetResultSymbol(syms[luauTokBlockCommentContent])
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
