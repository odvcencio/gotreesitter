//go:build !grammar_subset || grammar_subset_uxntal

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

const (
	uxntalSymComment gotreesitter.Symbol = 285
)

// UxntalExternalScanner handles nestable ( ) Forth-style comments for Uxntal.
type UxntalExternalScanner struct{}

func (UxntalExternalScanner) Create() any                           { return nil }
func (UxntalExternalScanner) Destroy(payload any)                   {}
func (UxntalExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (UxntalExternalScanner) Deserialize(payload any, buf []byte)   {}

// Comment depth is local to one Scan call. No state survives a token or a
// failed unterminated-comment scan.
func (UxntalExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (UxntalExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (UxntalExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (UxntalExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, _ []bool) bool {
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '(' {
		return false
	}
	lexer.Advance(false)

	depth := 1
	for depth > 0 {
		ch := lexer.Lookahead()
		if ch == 0 {
			return false
		}
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
		}
		lexer.Advance(false)
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(uxntalSymComment)
	return true
}
