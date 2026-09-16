//go:build !grammar_subset || grammar_subset_powershell

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the powershell grammar.
const (
	powershellTokStatementTerminator = 0
)

const (
	powershellSymStatementTerminator gotreesitter.Symbol = 232
)

// PowershellExternalScanner handles statement termination detection for PowerShell.
// A statement terminator is a zero-width token that fires when the next
// significant character is EOF, }, ;, ), or newline.
type PowershellExternalScanner struct{}

func (PowershellExternalScanner) Create() any                           { return nil }
func (PowershellExternalScanner) Destroy(payload any)                   {}
func (PowershellExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (PowershellExternalScanner) Deserialize(payload any, buf []byte)   {}

// SupportsIncrementalReuse certifies changed-edit subtree reuse. The scanner
// has no payload and recognizes only a statement terminator from the current
// lookahead and parser-provided valid-symbol set.
func (PowershellExternalScanner) SupportsIncrementalReuse() bool { return true }

// ExternalScannerIsStateless discharges the scanner-quiescence proof: Create
// returns nil, serialization is empty, and Scan reads no parse history or
// mutable package state. Skipped whitespace is local lexer input, not scanner
// state, so every reuse boundary begins from the same state as a fresh parse.
func (PowershellExternalScanner) ExternalScannerIsStateless() bool { return true }

// PreservesStateOnScanFailure is true because there is no persisted payload to
// mutate. This also lets scanner retries avoid a pointless snapshot attempt.
func (PowershellExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (PowershellExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !powershellValid(validSymbols, powershellTokStatementTerminator) {
		return false
	}
	lexer.SetResultSymbol(powershellSymStatementTerminator)
	lexer.MarkEnd()

	for {
		ch := lexer.Lookahead()
		if ch == 0 || ch == '}' || ch == ';' || ch == ')' || ch == '\n' {
			return true
		}
		if !unicode.IsSpace(ch) {
			return false
		}
		lexer.Advance(true)
	}
}

func powershellValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
