//go:build !grammar_subset || grammar_subset_janet

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the janet grammar.
const (
	janetTokLongBufLit = 0
	janetTokLongStrLit = 1
)

const (
	janetSymLongBufLit gotreesitter.Symbol = 26
	janetSymLongStrLit gotreesitter.Symbol = 27
)

// JanetExternalScanner handles @`...` long buffers and `...` long strings for Janet.
type JanetExternalScanner struct{}

func (JanetExternalScanner) Create() any                           { return nil }
func (JanetExternalScanner) Destroy(payload any)                   {}
func (JanetExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (JanetExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (JanetExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (JanetExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (JanetExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (JanetExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	bufValid := janetValid(validSymbols, janetTokLongBufLit)
	strValid := janetValid(validSymbols, janetTokLongStrLit)
	if !bufValid && !strValid {
		return false
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Determine if it's a long buffer (@`) or long string (`)
	if lexer.Lookahead() == '@' {
		lexer.SetResultSymbol(janetSymLongBufLit)
		lexer.Advance(false)
	} else {
		lexer.SetResultSymbol(janetSymLongStrLit)
	}

	// Must start with backtick
	if lexer.Lookahead() != '`' {
		return false
	}
	lexer.Advance(false)
	nBackticks := uint32(1)
	for lexer.Lookahead() == '`' {
		nBackticks++
		lexer.Advance(false)
	}
	if lexer.Lookahead() == 0 {
		return false
	}
	// Consume the first non-backtick character
	lexer.Advance(false)

	// Now look for nBackticks consecutive backticks
	cbt := uint32(0)
	for {
		if lexer.Lookahead() == 0 {
			return false
		}
		if lexer.Lookahead() == '`' {
			cbt++
			if cbt == nBackticks {
				lexer.Advance(false)
				lexer.MarkEnd()
				return true
			}
		} else {
			cbt = 0
		}
		lexer.Advance(false)
	}
}

func janetValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
