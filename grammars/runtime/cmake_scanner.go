//go:build !grammar_subset || grammar_subset_cmake

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the cmake grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see cmakeDefaultSymTable below.
const (
	cmakeTokBracketArgOpen    = 0
	cmakeTokBracketArgContent = 1
	cmakeTokBracketArgClose   = 2
	cmakeTokBracketComOpen    = 3
	cmakeTokBracketComContent = 4
	cmakeTokBracketComClose   = 5
	cmakeTokLineComment       = 6
	cmakeTokenCount           = 7
)

// cmakeDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped cmake.bin assigns to each external, in cmakeTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var cmakeDefaultSymTable = [cmakeTokenCount]gotreesitter.Symbol{
	36, // bracket_argument_open
	37, // bracket_argument_content
	38, // bracket_argument_close
	39, // bracket_comment_open
	40, // bracket_comment_content
	41, // bracket_comment_close
	42, // line_comment
}

// cmakeExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (cmakeTok* order).
var cmakeExternalScannerSpec = ExternalScannerSpec{
	Language:       "cmake",
	UpstreamRepo:   "https://github.com/uyha/tree-sitter-cmake",
	UpstreamCommit: "58993af75218bc99a1f5a04c832a5937e7c422cb",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "c449fc074b4bbc42dd681edf83259974acbf4b4e74461e69d2f8149a36eff723"},
		{Path: "src/scanner.c", SHA256: "10e6fb30e7df09585f115ba385951e22b8179bc7f830478f57d42d82c7c79ce3"},
	},
	Externals: []string{
		"bracket_argument_open",
		"bracket_argument_content",
		"bracket_argument_close",
		"bracket_comment_open",
		"bracket_comment_content",
		"bracket_comment_close",
		"line_comment",
	},
}

func init() {
	RegisterExternalScannerSpec(cmakeExternalScannerSpec)
}

// cmakeState tracks the bracket level and last token type.
type cmakeState struct {
	level uint32
	token uint8
}

// CmakeExternalScanner handles CMake bracket arguments, bracket comments, and
// line comments.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type CmakeExternalScanner struct {
	symbols         [cmakeTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers cmake's external symbols.
func (CmakeExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CmakeExternalScanner{symbols: cmakeDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, cmakeExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s CmakeExternalScanner) symbolTable() *[cmakeTokenCount]gotreesitter.Symbol {
	if s.symbols == ([cmakeTokenCount]gotreesitter.Symbol{}) {
		return &cmakeDefaultSymTable
	}
	return &s.symbols
}

func (CmakeExternalScanner) Create() any                    { return &cmakeState{} }
func (CmakeExternalScanner) Destroy(payload any)            {}
func (CmakeExternalScanner) SupportsIncrementalReuse() bool { return true }
func (CmakeExternalScanner) UsesExternalScannerCheckpoints() bool {
	return true
}
func (CmakeExternalScanner) AllowsIncrementalReuseWithoutCheckpoint() bool {
	return true
}
func (CmakeExternalScanner) PreservesStateOnScanFailure() bool {
	return true
}

func (CmakeExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*cmakeState)
	if len(buf) < 5 {
		return 0
	}
	buf[0] = byte(s.level)
	buf[1] = byte(s.level >> 8)
	buf[2] = byte(s.level >> 16)
	buf[3] = byte(s.level >> 24)
	buf[4] = s.token
	return 5
}

func (CmakeExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*cmakeState)
	s.level = 0
	s.token = 0
	if len(buf) >= 5 {
		s.level = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
		s.token = buf[4]
	}
}

func (s CmakeExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*cmakeState)

	if len(s.externalToToken) > 0 {
		var semanticValid [cmakeTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < cmakeTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Bracket argument open: [=*[
	if cmakeValid(validSymbols, cmakeTokBracketArgOpen) {
		if level, ok := cmakeTryOpenBracket(lexer); ok {
			state.level = level
			state.token = uint8(cmakeTokBracketArgOpen)
			lexer.SetResultSymbol(syms[cmakeTokBracketArgOpen])
			return true
		}
	}

	// Bracket argument content
	if cmakeValid(validSymbols, cmakeTokBracketArgContent) && state.token == uint8(cmakeTokBracketArgOpen) {
		cmakeParseBracketedContent(lexer, state.level)
		state.token = uint8(cmakeTokBracketArgContent)
		lexer.SetResultSymbol(syms[cmakeTokBracketArgContent])
		return true
	}

	// Bracket argument close: ]=*]
	if cmakeValid(validSymbols, cmakeTokBracketArgClose) && state.token == uint8(cmakeTokBracketArgContent) {
		if cmakeTryCloseBracket(lexer, state.level) {
			state.level = 0
			lexer.SetResultSymbol(syms[cmakeTokBracketArgClose])
			return true
		}
	}

	// # starts a bracket comment or line comment
	if lexer.Lookahead() == '#' {
		if !cmakeValid(validSymbols, cmakeTokBracketComOpen) && !cmakeValid(validSymbols, cmakeTokLineComment) {
			return false
		}

		lexer.Advance(false)

		// Try bracket comment open: #[=*[
		if level, ok := cmakeTryOpenBracket(lexer); ok {
			state.level = level
			state.token = uint8(cmakeTokBracketComOpen)
			lexer.SetResultSymbol(syms[cmakeTokBracketComOpen])
			return true
		}

		// Line comment: consume rest of line
		for lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[cmakeTokLineComment])
		return true
	}

	// Bracket comment content
	if cmakeValid(validSymbols, cmakeTokBracketComContent) && state.token == uint8(cmakeTokBracketComOpen) {
		cmakeParseBracketedContent(lexer, state.level)
		state.token = uint8(cmakeTokBracketComContent)
		lexer.SetResultSymbol(syms[cmakeTokBracketComContent])
		return true
	}

	// Bracket comment close
	if cmakeValid(validSymbols, cmakeTokBracketComClose) && state.token == uint8(cmakeTokBracketComContent) {
		if cmakeTryCloseBracket(lexer, state.level) {
			state.level = 0
			lexer.SetResultSymbol(syms[cmakeTokBracketComClose])
			return true
		}
	}

	return false
}

// cmakeTryOpenBracket tries to match [=*[ and returns the level (number of =).
func cmakeTryOpenBracket(lexer *gotreesitter.ExternalLexer) (uint32, bool) {
	if lexer.Lookahead() != '[' {
		return 0, false
	}
	lexer.Advance(false)

	var level uint32
	for lexer.Lookahead() == '=' {
		level++
		lexer.Advance(false)
	}

	if lexer.Lookahead() != '[' {
		return 0, false
	}
	lexer.Advance(false)
	lexer.MarkEnd()

	return level, true
}

// cmakeParseBracketedContent consumes content until ]=*] with matching level.
//
// A failed close attempt must re-examine the current lookahead rather than
// blindly consuming one more byte: content that itself ends in "]" (for
// example "[;\]]=]") places two "]" bytes back to back, and the second one
// starts the real close. Falling through to an unconditional advance here
// would swallow that second "]" as ordinary content and never find the
// close. See upstream tree-sitter-cmake commit 3725810 ("fix: handle
// bracketed strings with `]` at the end of its content").
func cmakeParseBracketedContent(lexer *gotreesitter.ExternalLexer, level uint32) {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == ']' {
			lexer.MarkEnd()

			var eqCount uint32
			lexer.Advance(false)
			for lexer.Lookahead() == '=' {
				eqCount++
				lexer.Advance(false)
			}

			if eqCount == level && lexer.Lookahead() == ']' {
				break
			}

			continue
		}

		lexer.Advance(false)
		lexer.MarkEnd()
	}
}

// cmakeTryCloseBracket tries to match ]=*] with the specified level.
func cmakeTryCloseBracket(lexer *gotreesitter.ExternalLexer, level uint32) bool {
	if lexer.Lookahead() != ']' {
		return false
	}

	var eqCount uint32
	lexer.Advance(false)
	for lexer.Lookahead() == '=' {
		eqCount++
		lexer.Advance(false)
	}

	if eqCount != level || lexer.Lookahead() != ']' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	return true
}

func cmakeValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
