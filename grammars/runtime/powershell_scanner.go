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
	powershellTokenCount             = 1
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
}

var powershellExternalScannerSpec = ExternalScannerSpec{
	Language:       "powershell",
	UpstreamRepo:   "https://github.com/airbus-cert/tree-sitter-powershell",
	UpstreamCommit: "e7bd348c49fdfd5c853a146a670965ba516a6239",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "8d3b7b851a037064f08f350c0eed45324db644641424c71396a976e746549211"},
		{Path: "src/scanner.c", SHA256: "cf366b70e258a0ac1eb264ac0f4165c8f0c1b943d1a6e9db9d291c6cf178221f"},
	},
	Externals: []string{
		"_statement_terminator",
	},
}

func init() {
	RegisterExternalScannerSpec(powershellExternalScannerSpec)
}

// PowershellExternalScanner handles statement termination detection for PowerShell.
// A statement terminator is a zero-width token that fires when the next
// significant character is EOF, }, ;, ), or newline.
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

// Scan ports airbus-cert/tree-sitter-powershell src/scanner.c
// scan_statement_terminator 1:1. Commit c6d1897 (the only src/scanner.c
// change between da65ba3acc93 and e7bd348c49fd) switched the parser.h
// include from angle brackets to quotes for build portability; it changed no
// scanner behavior, so this port needed no update.
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

func (s PowershellExternalScanner) symbolTable() *[powershellTokenCount]gotreesitter.Symbol {
	if s.symbols == ([powershellTokenCount]gotreesitter.Symbol{}) {
		return &powershellDefaultSymTable
	}
	return &s.symbols
}

func powershellValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
