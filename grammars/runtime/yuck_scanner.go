//go:build !grammar_subset || grammar_subset_yuck

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the yuck grammar.
const (
	yuckTokUnescapedSingleQuote = 0
	yuckTokUnescapedDoubleQuote = 1
	yuckTokUnescapedBacktick    = 2
)

// Concrete symbol IDs from the generated yuck grammar ExternalSymbols.
const (
	yuckSymUnescapedSingleQuote gotreesitter.Symbol = 44
	yuckSymUnescapedDoubleQuote gotreesitter.Symbol = 45
	yuckSymUnescapedBacktick    gotreesitter.Symbol = 46
)

// YuckExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-yuck.
// Handles unescaped string fragments for single-quote, double-quote, and backtick strings.
type YuckExternalScanner struct{}

func (YuckExternalScanner) Create() any                           { return nil }
func (YuckExternalScanner) Destroy(payload any)                   {}
func (YuckExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (YuckExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (YuckExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (YuckExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (YuckExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (YuckExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if yuckValid(validSymbols, yuckTokUnescapedDoubleQuote) {
		if yuckScanStringFragment(lexer, '"') {
			lexer.SetResultSymbol(yuckSymUnescapedDoubleQuote)
			return true
		}
		return false
	}
	if yuckValid(validSymbols, yuckTokUnescapedSingleQuote) {
		if yuckScanStringFragment(lexer, '\'') {
			lexer.SetResultSymbol(yuckSymUnescapedSingleQuote)
			return true
		}
		return false
	}
	if yuckValid(validSymbols, yuckTokUnescapedBacktick) {
		if yuckScanStringFragment(lexer, '`') {
			lexer.SetResultSymbol(yuckSymUnescapedBacktick)
			return true
		}
		return false
	}
	return false
}

func yuckScanStringFragment(lexer *gotreesitter.ExternalLexer, quote rune) bool {
	hasContent := false
	for {
		lexer.MarkEnd()
		ch := lexer.Lookahead()
		if ch == quote {
			return hasContent
		}
		if ch == 0 {
			return false
		}
		if ch == '$' {
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				return hasContent
			}
		} else if ch == '\\' {
			return hasContent
		} else {
			lexer.Advance(false)
		}
		hasContent = true
	}
}

func yuckValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
