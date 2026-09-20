//go:build !grammar_subset || grammar_subset_cpp

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the cpp grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see cppDefaultSymTable below.
const (
	cppTokRawStringDelimiter = 0
	cppTokRawStringContent   = 1
	cppTokenCount            = 2
)

// cppDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped cpp.bin assigns to each external, in cppTok* order. It
// exists only as a pre-bind fallback (and as an independent value to compare
// a real bind against in tests); ExternalScannerForLanguage below overwrites
// it with values read from the actual loaded Language at bind time, which is
// what the scanner must do to survive a future blob regen that renumbers
// absolute symbol IDs without touching the externals list order.
var cppDefaultSymTable = [cppTokenCount]gotreesitter.Symbol{
	223, // raw_string_delimiter
	224, // raw_string_content
}

// cppExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (cppTok* order).
var cppExternalScannerSpec = ExternalScannerSpec{
	Language:       "cpp",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-cpp",
	UpstreamCommit: "c009222808634c1014f82438d4883753516a2c24",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "3aa14b0b6ef87951ef9933d126f6d9774bc7d7dba11e7be1861820e381d36455"},
		{Path: "src/scanner.c", SHA256: "cf60387d290271f4d2fb558d0569b2b9ef879cb72206a9df5cc73b0b1a4b20ff"},
	},
	Externals: []string{
		"raw_string_delimiter",
		"raw_string_content",
	},
}

func init() {
	RegisterExternalScannerSpec(cppExternalScannerSpec)
}

// CppExternalScanner handles C++ R"delim(...)delim" raw string literals. It
// reuses the shared rawstring_scanner.go helper (also used by arduino, cuda,
// and hlsl), passing this instance's bound symbols in as concrete
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
type CppExternalScanner struct {
	symbols         [cppTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers cpp's external symbols.
func (CppExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CppExternalScanner{symbols: cppDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, cppExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s CppExternalScanner) symbolTable() *[cppTokenCount]gotreesitter.Symbol {
	if s.symbols == ([cppTokenCount]gotreesitter.Symbol{}) {
		return &cppDefaultSymTable
	}
	return &s.symbols
}

func (CppExternalScanner) Create() any         { return rawStringCreate() }
func (CppExternalScanner) Destroy(payload any) {}
func (CppExternalScanner) Serialize(payload any, buf []byte) int {
	return rawStringSerialize(payload, buf)
}
func (CppExternalScanner) Deserialize(payload any, buf []byte) { rawStringDeserialize(payload, buf) }

func (s CppExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [cppTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < cppTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()
	return rawStringScan(payload, lexer, validSymbols,
		cppTokRawStringDelimiter, cppTokRawStringContent,
		syms[cppTokRawStringDelimiter], syms[cppTokRawStringContent])
}
