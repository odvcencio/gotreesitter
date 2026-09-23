//go:build !grammar_subset || grammar_subset_yuck

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the yuck grammar.
const (
	yuckTokUnescapedSingleQuote = 0
	yuckTokUnescapedDoubleQuote = 1
	yuckTokUnescapedBacktick    = 2
	yuckTokenCount              = 3
)

// yuckDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped yuck.bin assigns to each external, in yuckTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var yuckDefaultSymTable = [yuckTokenCount]gotreesitter.Symbol{
	44, // _unescaped_single_quote_string_fragment
	45, // _unescaped_double_quote_string_fragment
	46, // _unescaped_backtick_string_fragment
}

// yuckExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (yuckTok* order).
var yuckExternalScannerSpec = ExternalScannerSpec{
	Language:       "yuck",
	UpstreamRepo:   "https://github.com/Philipp-M/tree-sitter-yuck",
	UpstreamCommit: "e877f6ade4b77d5ef8787075141053631ba12318",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "2207b6c0e7055de1929806ac7a6e8f978e6447ddd948050476e955eb82d6254a"},
		{Path: "src/scanner.c", SHA256: "bbf32e66698d68e10b925a2e3e9fe446fe766e4694d130c28fbf238a277c4b80"},
	},
	Externals: []string{
		"_unescaped_single_quote_string_fragment",
		"_unescaped_double_quote_string_fragment",
		"_unescaped_backtick_string_fragment",
	},
}

func init() {
	RegisterExternalScannerSpec(yuckExternalScannerSpec)
}

// YuckExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-yuck.
// Handles unescaped string fragments for single-quote, double-quote, and backtick strings.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type YuckExternalScanner struct {
	symbols         [yuckTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers yuck's external symbols.
func (YuckExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := YuckExternalScanner{symbols: yuckDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, yuckExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s YuckExternalScanner) symbolTable() *[yuckTokenCount]gotreesitter.Symbol {
	if s.symbols == ([yuckTokenCount]gotreesitter.Symbol{}) {
		return &yuckDefaultSymTable
	}
	return &s.symbols
}

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

func (sc YuckExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [yuckTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < yuckTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if yuckValid(validSymbols, yuckTokUnescapedDoubleQuote) {
		if yuckScanStringFragment(lexer, '"') {
			lexer.SetResultSymbol(syms[yuckTokUnescapedDoubleQuote])
			return true
		}
		return false
	}
	if yuckValid(validSymbols, yuckTokUnescapedSingleQuote) {
		if yuckScanStringFragment(lexer, '\'') {
			lexer.SetResultSymbol(syms[yuckTokUnescapedSingleQuote])
			return true
		}
		return false
	}
	if yuckValid(validSymbols, yuckTokUnescapedBacktick) {
		if yuckScanStringFragment(lexer, '`') {
			lexer.SetResultSymbol(syms[yuckTokUnescapedBacktick])
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
