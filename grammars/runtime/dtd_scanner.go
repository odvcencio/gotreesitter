//go:build !grammar_subset || grammar_subset_dtd

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the dtd grammar.
const (
	dtdTokPITarget  = 0
	dtdTokPIContent = 1
	dtdTokComment   = 2
)

const (
	dtdSymPITarget  gotreesitter.Symbol = 58
	dtdSymPIContent gotreesitter.Symbol = 59
	dtdSymComment   gotreesitter.Symbol = 60
)

// DtdExternalScanner handles processing instructions and <!-- --> comments for DTD.
type DtdExternalScanner struct{}

func (DtdExternalScanner) Create() any                           { return nil }
func (DtdExternalScanner) Destroy(payload any)                   {}
func (DtdExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (DtdExternalScanner) Deserialize(payload any, buf []byte)   {}
func (DtdExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (DtdExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (DtdExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// Error recovery
	if dtdValid(validSymbols, dtdTokPITarget) &&
		dtdValid(validSymbols, dtdTokPIContent) &&
		dtdValid(validSymbols, dtdTokComment) {
		return false
	}

	if dtdValid(validSymbols, dtdTokPITarget) {
		return dtdScanPITarget(lexer)
	}

	if dtdValid(validSymbols, dtdTokPIContent) {
		return dtdScanPIContent(lexer)
	}

	if dtdValid(validSymbols, dtdTokComment) {
		return dtdScanComment(lexer)
	}

	return false
}

func dtdIsValidNameStartChar(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_' || ch == ':'
}

func dtdIsValidNameChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) ||
		ch == '_' || ch == ':' || ch == '.' || ch == '-' || ch == 0xB7
}

func dtdScanPITarget(lexer *gotreesitter.ExternalLexer) bool {
	if !dtdIsValidNameStartChar(lexer.Lookahead()) {
		return false
	}
	foundXFirst := (lexer.Lookahead() == 'x' || lexer.Lookahead() == 'X')
	if foundXFirst {
		lexer.MarkEnd()
	}
	lexer.Advance(false)

	for dtdIsValidNameChar(lexer.Lookahead()) {
		if foundXFirst && (lexer.Lookahead() == 'm' || lexer.Lookahead() == 'M') {
			lexer.Advance(false)
			if lexer.Lookahead() == 'l' || lexer.Lookahead() == 'L' {
				lexer.Advance(false)
				if dtdIsValidNameChar(lexer.Lookahead()) {
					// Not "xml" exactly, continue
					foundXFirst = false
					lexer.Advance(false)
					continue
				}
				// This is exactly "xml" — not a valid PI target
				return false
			}
		}
		foundXFirst = false
		lexer.Advance(false)
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(dtdSymPITarget)
	return true
}

func dtdScanPIContent(lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' && lexer.Lookahead() != '?' {
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '?' {
		return false
	}
	lexer.MarkEnd()
	lexer.Advance(false)
	if lexer.Lookahead() == '>' {
		lexer.Advance(false)
		for lexer.Lookahead() == ' ' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '\n' {
			lexer.Advance(false)
		} else if lexer.Lookahead() == 0 {
			return false
		} else {
			return false
		}
		lexer.SetResultSymbol(dtdSymPIContent)
		return true
	}
	return false
}

func dtdScanComment(lexer *gotreesitter.ExternalLexer) bool {
	// Expect <!-- (< and ! already consumed by grammar)
	if lexer.Lookahead() != '<' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '!' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)

	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '-' {
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
				break
			}
		} else {
			lexer.Advance(false)
		}
	}
	if lexer.Lookahead() == '>' {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(dtdSymComment)
		return true
	}
	return false
}

func dtdValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
