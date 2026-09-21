//go:build !grammar_subset || grammar_subset_janet

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the janet grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see janetDefaultSymTable below.
const (
	janetTokLongBufLit = 0
	janetTokLongStrLit = 1
	janetTokenCount    = 2
)

// janetDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped janet.bin assigns to each external, in janetTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var janetDefaultSymTable = [janetTokenCount]gotreesitter.Symbol{
	26, // long_buf_lit
	27, // long_str_lit
}

// janetExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (janetTok* order).
var janetExternalScannerSpec = ExternalScannerSpec{
	Language:       "janet",
	UpstreamRepo:   "https://github.com/sogaiu/tree-sitter-janet-simple",
	UpstreamCommit: "d183186995204314700be3e9e0a48053ea16b350",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "f5340c22875d38ff04d05ff768123886815125bbbb06e43913b5965e5f0497b6"},
		{Path: "src/scanner.c", SHA256: "ed391752ed7f311f998cc732ce0f021d9925c7fb9b2940278f2fa30bc50be8d4"},
	},
	Externals: []string{
		"long_buf_lit",
		"long_str_lit",
	},
}

func init() {
	RegisterExternalScannerSpec(janetExternalScannerSpec)
}

// JanetExternalScanner handles @`...` long buffers and `...` long strings for Janet.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type JanetExternalScanner struct {
	symbols         [janetTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers janet's external symbols.
func (JanetExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := JanetExternalScanner{symbols: janetDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, janetExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s JanetExternalScanner) symbolTable() *[janetTokenCount]gotreesitter.Symbol {
	if s.symbols == ([janetTokenCount]gotreesitter.Symbol{}) {
		return &janetDefaultSymTable
	}
	return &s.symbols
}

func (JanetExternalScanner) Create() any                           { return nil }
func (JanetExternalScanner) Destroy(payload any)                   {}
func (JanetExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (JanetExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (JanetExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (JanetExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (JanetExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc JanetExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [janetTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < janetTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	bufValid := janetValid(validSymbols, janetTokLongBufLit)
	strValid := janetValid(validSymbols, janetTokLongStrLit)
	if !bufValid && !strValid {
		return false
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Determine if it's a long buffer (@`) or long string (`)
	if lexer.Lookahead() == '@' {
		lexer.SetResultSymbol(syms[janetTokLongBufLit])
		lexer.Advance(false)
	} else {
		lexer.SetResultSymbol(syms[janetTokLongStrLit])
	}

	// Must start with backtick
	if lexer.Lookahead() != '`' {
		return false
	}
	lexer.Advance(false)
	nBackticks := uint32(1)
	for lexer.Lookahead() == '`' {
		nBackticks++
		lexer.Advance(false)
	}
	if lexer.Lookahead() == 0 {
		return false
	}
	// Consume the first non-backtick character
	lexer.Advance(false)

	// Now look for nBackticks consecutive backticks
	cbt := uint32(0)
	for {
		if lexer.Lookahead() == 0 {
			return false
		}
		if lexer.Lookahead() == '`' {
			cbt++
			if cbt == nBackticks {
				lexer.Advance(false)
				lexer.MarkEnd()
				return true
			}
		} else {
			cbt = 0
		}
		lexer.Advance(false)
	}
}

func janetValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
