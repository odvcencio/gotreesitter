//go:build !grammar_subset || grammar_subset_dhall

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the dhall grammar.
const (
	dhallTokBlockCommentContent = 0
	dhallTokBlockCommentEnd     = 1
)

const (
	dhallSymBlockCommentContent gotreesitter.Symbol = 127
	dhallSymBlockCommentEnd     gotreesitter.Symbol = 128
)

// DhallExternalScanner handles nestable {- -} block comments for Dhall.
type DhallExternalScanner struct{}

func (DhallExternalScanner) Create() any                           { return nil }
func (DhallExternalScanner) Destroy(payload any)                   {}
func (DhallExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (DhallExternalScanner) Deserialize(payload any, buf []byte)   {}
func (DhallExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (DhallExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (DhallExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !dhallValid(validSymbols, dhallTokBlockCommentContent) {
		return false
	}
	depth := 1
	for depth > 0 {
		ch := lexer.Lookahead()
		switch {
		case ch == 0:
			return false
		case ch == '{':
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
				depth++
			}
		case ch == '-':
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '}' {
				depth--
			}
		default:
			lexer.Advance(false)
		}
	}
	lexer.SetResultSymbol(dhallSymBlockCommentContent)
	return true
}

func dhallValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
