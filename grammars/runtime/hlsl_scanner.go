//go:build !grammar_subset || grammar_subset_hlsl

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the hlsl grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see hlslDefaultSymTable below.
const (
	hlslTokRawStringDelimiter = 0
	hlslTokRawStringContent   = 1
	hlslTokenCount            = 2
)

// hlslDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped hlsl.bin assigns to each external, in hlslTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var hlslDefaultSymTable = [hlslTokenCount]gotreesitter.Symbol{
	244, // raw_string_delimiter
	245, // raw_string_content
}

// hlslExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (hlslTok* order).
var hlslExternalScannerSpec = ExternalScannerSpec{
	Language:       "hlsl",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-hlsl",
	UpstreamCommit: "bab9111922d53d43668fabb61869bec51bbcb915",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "9b4a61d23e241ab6eb5c8b6572fee07cea59e2acceafd962a47c70984d0c1713"},
		{Path: "src/scanner.c", SHA256: "1c67f7a48120c161238d70abb8596c655b8f00c0fb7d4e3320a0edd0c4387df6"},
	},
	Externals: []string{
		"raw_string_delimiter",
		"raw_string_content",
	},
}

func init() {
	RegisterExternalScannerSpec(hlslExternalScannerSpec)
}

// HlslExternalScanner handles C++ R"delim(...)delim" raw string literals for
// HLSL. It reuses the shared rawstring_scanner.go helper (also used by cpp,
// arduino, and cuda), passing this instance's bound symbols in as concrete
// gotreesitter.Symbol arguments; the shared helper itself never hardcodes an
// absolute Symbol value.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type HlslExternalScanner struct {
	symbols         [hlslTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers hlsl's external symbols.
func (HlslExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := HlslExternalScanner{symbols: hlslDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, hlslExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s HlslExternalScanner) symbolTable() *[hlslTokenCount]gotreesitter.Symbol {
	if s.symbols == ([hlslTokenCount]gotreesitter.Symbol{}) {
		return &hlslDefaultSymTable
	}
	return &s.symbols
}

func (HlslExternalScanner) Create() any         { return rawStringCreate() }
func (HlslExternalScanner) Destroy(payload any) {}
func (HlslExternalScanner) Serialize(payload any, buf []byte) int {
	return rawStringSerialize(payload, buf)
}
func (HlslExternalScanner) Deserialize(payload any, buf []byte) { rawStringDeserialize(payload, buf) }

func (s HlslExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [hlslTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < hlslTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()
	return rawStringScan(payload, lexer, validSymbols,
		hlslTokRawStringDelimiter, hlslTokRawStringContent,
		syms[hlslTokRawStringDelimiter], syms[hlslTokRawStringContent])
}
