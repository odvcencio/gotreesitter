//go:build !grammar_subset || grammar_subset_kconfig

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the kconfig grammar.
const (
	kconfigTokText = 0
)

const (
	kconfigSymText gotreesitter.Symbol = 63
)

// KconfigExternalScanner handles indented help text blocks in Linux Kconfig files.
// Help text continues as long as lines have consistent indentation.
type KconfigExternalScanner struct{}

func (KconfigExternalScanner) Create() any                           { return nil }
func (KconfigExternalScanner) Destroy(payload any)                   {}
func (KconfigExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (KconfigExternalScanner) Deserialize(payload any, buf []byte)   {}
func (KconfigExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (KconfigExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (KconfigExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !kconfigValid(validSymbols, kconfigTokText) {
		return false
	}

	startCol := uint32(0)
	for unicode.IsSpace(lexer.Lookahead()) {
		startCol = kconfigScannerIndentColumn(startCol, lexer.Lookahead())
		lexer.Advance(true)
	}

	for {
		for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}

		if lexer.Lookahead() == 0 {
			lexer.MarkEnd()
			lexer.SetResultSymbol(kconfigSymText)
			return true
		}

		lexer.MarkEnd()
		nextCol := uint32(0)
		for unicode.IsSpace(lexer.Lookahead()) {
			nextCol = kconfigScannerIndentColumn(nextCol, lexer.Lookahead())
			lexer.Advance(false)
		}

		if nextCol < startCol {
			lexer.MarkEnd()
			lexer.SetResultSymbol(kconfigSymText)
			return true
		}
	}
}

func kconfigValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

func kconfigScannerIndentColumn(col uint32, ch rune) uint32 {
	switch ch {
	case ' ':
		return col + 1
	case '\t':
		col += 8
		return col - col%8
	default:
		return col
	}
}
