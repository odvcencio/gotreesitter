//go:build !grammar_subset || grammar_subset_tablegen

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the tablegen grammar.
const (
	tablegenTokMultilineComment = 0
)

const (
	tablegenSymMultilineComment gotreesitter.Symbol = 98
)

// TablegenExternalScanner handles nestable /* */ comments for TableGen.
type TablegenExternalScanner struct{}

func (TablegenExternalScanner) Create() any                           { return nil }
func (TablegenExternalScanner) Destroy(payload any)                   {}
func (TablegenExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (TablegenExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (TablegenExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (TablegenExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (TablegenExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (TablegenExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !tablegenValid(validSymbols, tablegenTokMultilineComment) {
		return false
	}

	// Skip leading whitespace before the comment delimiter. The C scanner
	// (scanner.c) does exactly this with advance(lexer, /*skip=*/true); without
	// it the scanner bails the moment the lexer is positioned on the space that
	// precedes "/*" (e.g. inside "{ /*x*/ }"), and the DFA then mis-lexes the
	// comment body. Passing skip=true moves only the token start, matching C.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if lexer.Lookahead() != '/' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)

	depth := 1
	afterStar := false
	for {
		ch := lexer.Lookahead()
		if ch == 0 {
			// Unterminated comment: C returns false (no token) on EOF.
			return false
		}
		if ch == '*' {
			lexer.Advance(false)
			afterStar = true
			continue
		}
		if ch == '/' {
			lexer.Advance(false)
			if afterStar {
				afterStar = false
				depth--
				if depth == 0 {
					lexer.MarkEnd()
					lexer.SetResultSymbol(tablegenSymMultilineComment)
					return true
				}
			} else if lexer.Lookahead() == '*' {
				depth++
				lexer.Advance(false)
			}
			continue
		}
		lexer.Advance(false)
		afterStar = false
	}
}

func tablegenValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
