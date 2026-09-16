//go:build !grammar_subset || grammar_subset_fennel

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the fennel grammar.
const (
	fennelTokHashfn             = 0
	fennelTokQuote              = 1
	fennelTokQuasiQuote         = 2
	fennelTokUnquote            = 3
	fennelTokReaderMacroCount   = 4 // sentinel, not produced
	fennelTokColonStringStartMk = 5 // not produced by scanner
	fennelTokColonStringEndMk   = 6 // not produced by scanner
	fennelTokShebang            = 7
	fennelTokCount              = 8 // error sentinel
)

const (
	fennelSymHashfn  gotreesitter.Symbol = 72
	fennelSymQuote   gotreesitter.Symbol = 73
	fennelSymQuasiQt gotreesitter.Symbol = 74
	fennelSymUnquote gotreesitter.Symbol = 75
	fennelSymShebang gotreesitter.Symbol = 79
)

// Reader macro characters indexed by token index.
var fennelReaderMacroChars = [4]rune{'#', '\'', '`', ','}
var fennelReaderMacroSyms = [4]gotreesitter.Symbol{
	fennelSymHashfn, fennelSymQuote, fennelSymQuasiQt, fennelSymUnquote,
}

// FennelExternalScanner handles reader macros and shebang for Fennel.
type FennelExternalScanner struct{}

func (FennelExternalScanner) Create() any                           { return nil }
func (FennelExternalScanner) Destroy(payload any)                   {}
func (FennelExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (FennelExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (FennelExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (FennelExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (FennelExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (FennelExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// Error recovery guard: if the error sentinel is valid, bail out.
	if fennelValid(validSymbols, fennelTokCount) {
		return false
	}

	skippedWhitespace := unicode.IsSpace(lexer.Lookahead())
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Try shebang: #!...
	skippedHashfn := false
	if fennelValid(validSymbols, fennelTokShebang) {
		lexer.MarkEnd()
		if lexer.Lookahead() == '#' {
			skippedHashfn = true
			lexer.Advance(false)
			if lexer.Lookahead() == '!' {
				skippedHashfn = false
				lexer.Advance(false)
				for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
					lexer.Advance(false)
				}
				lexer.MarkEnd()
				lexer.SetResultSymbol(fennelSymShebang)
				return true
			}
		}
	}

	// Try reader macros: #, ', `, ,
	if fennelValid(validSymbols, fennelTokHashfn) && (skippedWhitespace || !fennelValid(validSymbols, fennelTokColonStringStartMk)) {
		if skippedHashfn {
			// Already consumed '#', check position validity
			ch := lexer.Lookahead()
			if !unicode.IsSpace(ch) && !fennelIsCloseBracket(ch) && ch != 0 {
				lexer.MarkEnd()
				lexer.SetResultSymbol(fennelSymHashfn)
				return true
			}
			return false
		}
		for i := 0; i < 4; i++ {
			if lexer.Lookahead() == fennelReaderMacroChars[i] {
				lexer.Advance(false)
				ch := lexer.Lookahead()
				if !unicode.IsSpace(ch) && !fennelIsCloseBracket(ch) && ch != 0 {
					lexer.MarkEnd()
					lexer.SetResultSymbol(fennelReaderMacroSyms[i])
					return true
				}
				return false
			}
		}
	}

	return false
}

func fennelIsCloseBracket(ch rune) bool {
	return ch == ')' || ch == '}' || ch == ']'
}

func fennelValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
