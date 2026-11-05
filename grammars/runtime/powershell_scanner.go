//go:build !grammar_subset || grammar_subset_powershell

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the powershell grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see the symbols field on PowershellExternalScanner below.
const (
	powershellTokStatementTerminator = 0
	powershellTokBareDashes          = 1
	powershellTokenCount             = 2
)

// powershellDefaultSymTable records the concrete gotreesitter.Symbol ID the
// currently shipped powershell.bin assigns to the statement-terminator
// external. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with the value read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var powershellDefaultSymTable = [powershellTokenCount]gotreesitter.Symbol{
	232, // _statement_terminator
	233, // _bare_dashes
}

var powershellExternalScannerSpec = ExternalScannerSpec{
	Language:       "powershell",
	UpstreamRepo:   "https://github.com/airbus-cert/tree-sitter-powershell",
	UpstreamCommit: "b03e6b0e39692037a98e37a26f0a969b2c46fbc1",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "a7504b323a1730437b32d7f312f9ce16ede8b8d7c4d608fa3a98bdec309492f2"},
		{Path: "src/scanner.c", SHA256: "dab03ff73a44495c72e6b51f43fd2f5128a1a29344a29974f12fba250328cf99"},
	},
	Externals: []string{
		"_statement_terminator",
		"_bare_dashes",
	},
}

func init() {
	RegisterExternalScannerSpec(powershellExternalScannerSpec)
}

// PowershellExternalScanner handles statement termination detection and bare
// dash arguments for PowerShell. A statement terminator is a zero-width token
// that fires when the next significant character is EOF, }, ;, ), or newline.
// A bare dash argument is a `-` or `--` followed by whitespace or the end of
// the command.
type PowershellExternalScanner struct {
	symbols         [powershellTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's one token slot to the
// loaded Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers powershell's external symbol.
func (PowershellExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := PowershellExternalScanner{symbols: powershellDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, powershellExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (PowershellExternalScanner) Create() any                           { return nil }
func (PowershellExternalScanner) Destroy(payload any)                   {}
func (PowershellExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (PowershellExternalScanner) Deserialize(payload any, buf []byte)   {}

// SupportsIncrementalReuse certifies changed-edit subtree reuse. The scanner
// has no payload and recognizes only a statement terminator or bare dashes from the current
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

// Scan ports airbus-cert/tree-sitter-powershell src/scanner.c
// tree_sitter_powershell_external_scanner_scan 1:1. The bare dashes scan runs
// first because the statement terminator consumes nothing and would otherwise
// win over a `--`.
func (s PowershellExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [powershellTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < powershellTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	if s.scanBareDashes(lexer, validSymbols) {
		return true
	}
	return s.scanStatementTerminator(lexer, validSymbols)
}

func (s PowershellExternalScanner) scanStatementTerminator(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !powershellValid(validSymbols, powershellTokStatementTerminator) {
		return false
	}
	lexer.SetResultSymbol(s.symbolTable()[powershellTokStatementTerminator])
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

func (s PowershellExternalScanner) scanBareDashes(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !powershellValid(validSymbols, powershellTokBareDashes) {
		return false
	}
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(true)
	}
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() == '-' {
		lexer.Advance(false)
	}
	lexer.MarkEnd()

	switch lexer.Lookahead() {
	case 0, ' ', '\t', '\r', '\n', ';', '|', '&', ')', '}':
		lexer.SetResultSymbol(s.symbolTable()[powershellTokBareDashes])
		return true
	}
	return false
}

func (s PowershellExternalScanner) symbolTable() *[powershellTokenCount]gotreesitter.Symbol {
	if s.symbols == ([powershellTokenCount]gotreesitter.Symbol{}) {
		return &powershellDefaultSymTable
	}
	return &s.symbols
}

func powershellValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
