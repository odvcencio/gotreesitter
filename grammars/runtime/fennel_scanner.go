//go:build !grammar_subset || grammar_subset_fennel

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the fennel grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see fennelDefaultSymTable below.
const (
	fennelTokHashfn             = 0
	fennelTokQuote              = 1
	fennelTokQuasiQuote         = 2
	fennelTokUnquote            = 3
	fennelTokReaderMacroCount   = 4 // sentinel, not produced
	fennelTokColonStringStartMk = 5 // not produced by scanner
	fennelTokColonStringEndMk   = 6 // not produced by scanner
	fennelTokShebang            = 7
	fennelTokCount              = 8 // error sentinel
	fennelTokenCount            = 9
)

// fennelDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped fennel.bin assigns to each external, in fennelTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var fennelDefaultSymTable = [fennelTokenCount]gotreesitter.Symbol{
	72, // _hashfn_reader_macro_char, displays as "#"
	73, // _quote_reader_macro_char, displays as "'"
	74, // _quasi_quote_reader_macro_char, displays as "`"
	75, // _unquote_reader_macro_char, displays as ","
	76, // __reader_macro_count (sentinel, never emitted)
	77, // __colon_string_start_mark (never emitted by this scanner)
	78, // __colon_string_end_mark (never emitted by this scanner)
	79, // shebang
	80, // __token_count (error-recovery sentinel, never emitted)
}

// fennelExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (fennelTok* order).
var fennelExternalScannerSpec = ExternalScannerSpec{
	Language:       "fennel",
	UpstreamRepo:   "https://github.com/alexmozaidze/tree-sitter-fennel",
	UpstreamCommit: "3f0f6b24d599e92460b969aabc4f4c5a914d15a0",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "7e6502ffb5cd787b01e8ac1d285b54d1aaaeb2dc1a5ee14797fd2549a1e9735f"},
		{Path: "src/scanner.c", SHA256: "f6f45d0b3d86ee0b1cae8446d8dd2fe6c3ca397d6bf483dd7c0950af78689c39"},
	},
	Externals: []string{
		"_hashfn_reader_macro_char",
		"_quote_reader_macro_char",
		"_quasi_quote_reader_macro_char",
		"_unquote_reader_macro_char",
		"__reader_macro_count",
		"__colon_string_start_mark",
		"__colon_string_end_mark",
		"shebang",
		"__token_count",
	},
}

func init() {
	RegisterExternalScannerSpec(fennelExternalScannerSpec)
}

// Reader macro characters indexed by token index.
var fennelReaderMacroChars = [4]rune{'#', '\'', '`', ','}

// FennelExternalScanner handles reader macros and shebang for Fennel.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type FennelExternalScanner struct {
	symbols         [fennelTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers fennel's external symbols.
func (FennelExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := FennelExternalScanner{symbols: fennelDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, fennelExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s FennelExternalScanner) symbolTable() *[fennelTokenCount]gotreesitter.Symbol {
	if s.symbols == ([fennelTokenCount]gotreesitter.Symbol{}) {
		return &fennelDefaultSymTable
	}
	return &s.symbols
}

func (FennelExternalScanner) Create() any                           { return nil }
func (FennelExternalScanner) Destroy(payload any)                   {}
func (FennelExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (FennelExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (FennelExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (FennelExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (FennelExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc FennelExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [fennelTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < fennelTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	// Error recovery guard: if the error sentinel is valid, bail out.
	if fennelValid(validSymbols, fennelTokCount) {
		return false
	}

	skippedWhitespace := unicode.IsSpace(lexer.Lookahead())
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Try shebang: #!...
	skippedHashfn := false
	if fennelValid(validSymbols, fennelTokShebang) {
		lexer.MarkEnd()
		if lexer.Lookahead() == '#' {
			skippedHashfn = true
			lexer.Advance(false)
			if lexer.Lookahead() == '!' {
				skippedHashfn = false
				lexer.Advance(false)
				for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
					lexer.Advance(false)
				}
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[fennelTokShebang])
				return true
			}
		}
	}

	// Try reader macros: #, ', `, ,
	if fennelValid(validSymbols, fennelTokHashfn) && (skippedWhitespace || !fennelValid(validSymbols, fennelTokColonStringStartMk)) {
		if skippedHashfn {
			// Already consumed '#', check position validity
			ch := lexer.Lookahead()
			if !unicode.IsSpace(ch) && !fennelIsCloseBracket(ch) && ch != 0 {
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[fennelTokHashfn])
				return true
			}
			return false
		}
		for i := 0; i < 4; i++ {
			if lexer.Lookahead() == fennelReaderMacroChars[i] {
				lexer.Advance(false)
				ch := lexer.Lookahead()
				if !unicode.IsSpace(ch) && !fennelIsCloseBracket(ch) && ch != 0 {
					lexer.MarkEnd()
					lexer.SetResultSymbol(syms[i])
					return true
				}
				return false
			}
		}
	}

	return false
}

func fennelIsCloseBracket(ch rune) bool {
	return ch == ')' || ch == '}' || ch == ']'
}

func fennelValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
