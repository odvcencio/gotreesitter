//go:build !grammar_subset || grammar_subset_pkl

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Pkl grammar.
const (
	pklTokSlStringChars        = 0
	pklTokSl1StringChars       = 1
	pklTokSl2StringChars       = 2
	pklTokSl3StringChars       = 3
	pklTokSl4StringChars       = 4
	pklTokSl5StringChars       = 5
	pklTokSl6StringChars       = 6
	pklTokMlStringChars        = 7
	pklTokMl1StringChars       = 8
	pklTokMl2StringChars       = 9
	pklTokMl3StringChars       = 10
	pklTokMl4StringChars       = 11
	pklTokMl5StringChars       = 12
	pklTokMl6StringChars       = 13
	pklTokOpenSubscriptBracket = 14
	pklTokOpenArgumentParen    = 15
	pklTokBinaryMinus          = 16
)

const (
	pklSymSlStringChars        gotreesitter.Symbol = 127
	pklSymSl1StringChars       gotreesitter.Symbol = 128
	pklSymSl2StringChars       gotreesitter.Symbol = 129
	pklSymSl3StringChars       gotreesitter.Symbol = 130
	pklSymSl4StringChars       gotreesitter.Symbol = 131
	pklSymSl5StringChars       gotreesitter.Symbol = 132
	pklSymSl6StringChars       gotreesitter.Symbol = 133
	pklSymMlStringChars        gotreesitter.Symbol = 134
	pklSymMl1StringChars       gotreesitter.Symbol = 135
	pklSymMl2StringChars       gotreesitter.Symbol = 136
	pklSymMl3StringChars       gotreesitter.Symbol = 137
	pklSymMl4StringChars       gotreesitter.Symbol = 138
	pklSymMl5StringChars       gotreesitter.Symbol = 139
	pklSymMl6StringChars       gotreesitter.Symbol = 140
	pklSymOpenSubscriptBracket gotreesitter.Symbol = 141
	pklSymOpenArgumentParen    gotreesitter.Symbol = 142
	pklSymBinaryMinus          gotreesitter.Symbol = 143
)

// Pound-indexed symbol IDs for single-line and multi-line string chars.
var pklSlxSyms = [7]gotreesitter.Symbol{
	pklSymSlStringChars,
	pklSymSl1StringChars, pklSymSl2StringChars, pklSymSl3StringChars,
	pklSymSl4StringChars, pklSymSl5StringChars, pklSymSl6StringChars,
}
var pklMlxSyms = [7]gotreesitter.Symbol{
	pklSymMlStringChars,
	pklSymMl1StringChars, pklSymMl2StringChars, pklSymMl3StringChars,
	pklSymMl4StringChars, pklSymMl5StringChars, pklSymMl6StringChars,
}

// PklExternalScanner handles string content and contextual operators for Pkl.
type PklExternalScanner struct{}

func (PklExternalScanner) Create() any                           { return nil }
func (PklExternalScanner) Destroy(payload any)                   {}
func (PklExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (PklExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (PklExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (PklExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (PklExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (PklExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// Error recovery: if all string tokens valid, bail out.
	if pklValid(validSymbols, pklTokSlStringChars) && pklValid(validSymbols, pklTokSl1StringChars) &&
		pklValid(validSymbols, pklTokSl2StringChars) && pklValid(validSymbols, pklTokSl3StringChars) &&
		pklValid(validSymbols, pklTokSl4StringChars) && pklValid(validSymbols, pklTokSl5StringChars) &&
		pklValid(validSymbols, pklTokSl6StringChars) && pklValid(validSymbols, pklTokMlStringChars) &&
		pklValid(validSymbols, pklTokMl1StringChars) && pklValid(validSymbols, pklTokMl2StringChars) &&
		pklValid(validSymbols, pklTokMl3StringChars) && pklValid(validSymbols, pklTokMl4StringChars) &&
		pklValid(validSymbols, pklTokMl5StringChars) && pklValid(validSymbols, pklTokMl6StringChars) &&
		pklValid(validSymbols, pklTokOpenSubscriptBracket) && pklValid(validSymbols, pklTokOpenArgumentParen) &&
		pklValid(validSymbols, pklTokBinaryMinus) {
		return false
	}

	// Single-line string without pounds
	if pklValid(validSymbols, pklTokSlStringChars) {
		return pklParseSlStringChars(lexer)
	}
	// Multi-line string without pounds
	if pklValid(validSymbols, pklTokMlStringChars) {
		return pklParseMlStringChars(lexer)
	}
	// Single-line strings with N pounds
	for i := 1; i <= 6; i++ {
		if pklValid(validSymbols, pklTokSlStringChars+i) {
			return pklParseSlxStringChars(lexer, i)
		}
	}
	// Multi-line strings with N pounds
	for i := 1; i <= 6; i++ {
		if pklValid(validSymbols, pklTokMlStringChars+i) {
			return pklParseMlxStringChars(lexer, i)
		}
	}
	// Contextual operators: [, (, -
	if pklValid(validSymbols, pklTokOpenSubscriptBracket) || pklValid(validSymbols, pklTokOpenArgumentParen) ||
		pklValid(validSymbols, pklTokBinaryMinus) {
		return pklParseContextualOp(lexer, validSymbols)
	}

	return false
}

// pklParseSlStringChars: simple single-line string content (no pound variant).
func pklParseSlStringChars(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(pklSymSlStringChars)
	hasContent := false
	for {
		switch lexer.Lookahead() {
		case '"', '\\', '\n', '\r', 0:
			return hasContent
		default:
			hasContent = true
			lexer.Advance(false)
		}
	}
}

// pklParseSlxStringChars: single-line string content with N pound signs.
func pklParseSlxStringChars(lexer *gotreesitter.ExternalLexer, numPounds int) bool {
	lexer.SetResultSymbol(pklSlxSyms[numPounds])
	hasContent := false
	for {
		switch lexer.Lookahead() {
		case '"', '\\':
			lexer.MarkEnd()
			lexer.Advance(false)
			matched := true
			for i := 0; i < numPounds; i++ {
				if lexer.Lookahead() != '#' {
					matched = false
					hasContent = true
					break
				}
				lexer.Advance(false)
			}
			if matched {
				return hasContent
			}
		case '\n', '\r', 0:
			lexer.MarkEnd()
			return hasContent
		default:
			hasContent = true
			lexer.Advance(false)
		}
	}
}

// pklParseMlStringChars: multi-line string content (no pound variant).
func pklParseMlStringChars(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(pklSymMlStringChars)
	hasContent := false
	for {
		switch lexer.Lookahead() {
		case '"':
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					return hasContent
				}
			}
			hasContent = true
		case '\\', 0:
			lexer.MarkEnd()
			return hasContent
		default:
			hasContent = true
			lexer.Advance(false)
		}
	}
}

// pklParseMlxStringChars: multi-line string content with N pound signs.
func pklParseMlxStringChars(lexer *gotreesitter.ExternalLexer, numPounds int) bool {
	lexer.SetResultSymbol(pklMlxSyms[numPounds])
	hasContent := false
	for {
		switch lexer.Lookahead() {
		case '"':
			lexer.MarkEnd()
			quoteCount := 0
			for lexer.Lookahead() == '"' {
				quoteCount++
				lexer.Advance(false)
			}
			if quoteCount < 3 {
				hasContent = true
				continue
			}
			matched := true
			for i := 0; i < numPounds; i++ {
				if lexer.Lookahead() != '#' {
					matched = false
					hasContent = true
					break
				}
				lexer.Advance(false)
			}
			if matched {
				return hasContent
			}
		case '\\':
			lexer.MarkEnd()
			lexer.Advance(false)
			matched := true
			for i := 0; i < numPounds; i++ {
				if lexer.Lookahead() != '#' {
					matched = false
					hasContent = true
					break
				}
				lexer.Advance(false)
			}
			if matched {
				return hasContent
			}
		case 0:
			lexer.MarkEnd()
			return hasContent
		default:
			hasContent = true
			lexer.Advance(false)
		}
	}
}

// pklParseContextualOp: handles [, (, - that can't have preceding newline or semicolon.
func pklParseContextualOp(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if lexer.Lookahead() == 0 {
		return false
	}
	for {
		switch lexer.Lookahead() {
		case ' ', '\t', '\r', '\f':
			lexer.Advance(true)
		case '[':
			if pklValid(validSymbols, pklTokOpenSubscriptBracket) {
				lexer.Advance(false)
				lexer.SetResultSymbol(pklSymOpenSubscriptBracket)
				return true
			}
			return false
		case '(':
			if pklValid(validSymbols, pklTokOpenArgumentParen) {
				lexer.Advance(false)
				lexer.SetResultSymbol(pklSymOpenArgumentParen)
				return true
			}
			return false
		case '-':
			if pklValid(validSymbols, pklTokBinaryMinus) {
				lexer.Advance(false)
				lexer.MarkEnd()
				// Don't match -> as binary minus
				if lexer.Lookahead() == '>' {
					return false
				}
				lexer.SetResultSymbol(pklSymBinaryMinus)
				return true
			}
			return false
		default:
			return false
		}
	}
}

func pklValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
