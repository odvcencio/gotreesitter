//go:build !grammar_subset || grammar_subset_squirrel

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the squirrel grammar.
const (
	squirrelTokVerbatimString = 0
)

const (
	squirrelSymVerbatimString gotreesitter.Symbol = 95
)

// SquirrelExternalScanner handles @"..." verbatim string literals for Squirrel.
// Inside a verbatim string, "" is an escaped double-quote.
type SquirrelExternalScanner struct{}

func (SquirrelExternalScanner) Create() any                           { return nil }
func (SquirrelExternalScanner) Destroy(payload any)                   {}
func (SquirrelExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (SquirrelExternalScanner) Deserialize(payload any, buf []byte)   {}

// Verbatim-string scanning is self-contained in one token and retains no
// payload, including when the closing quote is absent.
func (SquirrelExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (SquirrelExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (SquirrelExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (SquirrelExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !squirrelValid(validSymbols, squirrelTokVerbatimString) {
		return false
	}
	// Expect @"
	if lexer.Lookahead() != '@' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)

	// Scan until unescaped closing "
	for {
		ch := lexer.Lookahead()
		if ch == 0 {
			return false
		}
		if ch == '"' {
			lexer.Advance(false)
			// "" is an escaped quote, continue scanning
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				continue
			}
			// Single " ends the string
			lexer.MarkEnd()
			lexer.SetResultSymbol(squirrelSymVerbatimString)
			return true
		}
		lexer.Advance(false)
	}
}

func squirrelValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
