//go:build !grammar_subset || grammar_subset_cmake

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the cmake grammar.
const (
	cmakeTokBracketArgOpen    = 0
	cmakeTokBracketArgContent = 1
	cmakeTokBracketArgClose   = 2
	cmakeTokBracketComOpen    = 3
	cmakeTokBracketComContent = 4
	cmakeTokBracketComClose   = 5
	cmakeTokLineComment       = 6
)

const (
	cmakeSymBracketArgOpen    gotreesitter.Symbol = 36
	cmakeSymBracketArgContent gotreesitter.Symbol = 37
	cmakeSymBracketArgClose   gotreesitter.Symbol = 38
	cmakeSymBracketComOpen    gotreesitter.Symbol = 39
	cmakeSymBracketComContent gotreesitter.Symbol = 40
	cmakeSymBracketComClose   gotreesitter.Symbol = 41
	cmakeSymLineComment       gotreesitter.Symbol = 42
)

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

// CmakeExternalScanner handles CMake bracket arguments, bracket comments, and line comments.
type CmakeExternalScanner struct{}

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

func (CmakeExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*cmakeState)

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Bracket argument open: [=*[
	if cmakeValid(validSymbols, cmakeTokBracketArgOpen) {
		if level, ok := cmakeTryOpenBracket(lexer); ok {
			s.level = level
			s.token = uint8(cmakeTokBracketArgOpen)
			lexer.SetResultSymbol(cmakeSymBracketArgOpen)
			return true
		}
	}

	// Bracket argument content
	if cmakeValid(validSymbols, cmakeTokBracketArgContent) && s.token == uint8(cmakeTokBracketArgOpen) {
		cmakeParseBracketedContent(lexer, s.level)
		s.token = uint8(cmakeTokBracketArgContent)
		lexer.SetResultSymbol(cmakeSymBracketArgContent)
		return true
	}

	// Bracket argument close: ]=*]
	if cmakeValid(validSymbols, cmakeTokBracketArgClose) && s.token == uint8(cmakeTokBracketArgContent) {
		if cmakeTryCloseBracket(lexer, s.level) {
			s.level = 0
			lexer.SetResultSymbol(cmakeSymBracketArgClose)
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
			s.level = level
			s.token = uint8(cmakeTokBracketComOpen)
			lexer.SetResultSymbol(cmakeSymBracketComOpen)
			return true
		}

		// Line comment: consume rest of line
		for lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(cmakeSymLineComment)
		return true
	}

	// Bracket comment content
	if cmakeValid(validSymbols, cmakeTokBracketComContent) && s.token == uint8(cmakeTokBracketComOpen) {
		cmakeParseBracketedContent(lexer, s.level)
		s.token = uint8(cmakeTokBracketComContent)
		lexer.SetResultSymbol(cmakeSymBracketComContent)
		return true
	}

	// Bracket comment close
	if cmakeValid(validSymbols, cmakeTokBracketComClose) && s.token == uint8(cmakeTokBracketComContent) {
		if cmakeTryCloseBracket(lexer, s.level) {
			s.level = 0
			lexer.SetResultSymbol(cmakeSymBracketComClose)
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
