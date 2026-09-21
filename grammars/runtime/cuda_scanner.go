//go:build !grammar_subset || grammar_subset_cuda

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the cuda grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see cudaDefaultSymTable below.
const (
	cudaTokRawStringDelimiter = 0
	cudaTokRawStringContent   = 1
	cudaTokenCount            = 2
)

// cudaDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped cuda.bin assigns to each external, in cudaTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var cudaDefaultSymTable = [cudaTokenCount]gotreesitter.Symbol{
	230, // raw_string_delimiter
	231, // raw_string_content
}

// cudaExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (cudaTok* order).
var cudaExternalScannerSpec = ExternalScannerSpec{
	Language:       "cuda",
	UpstreamRepo:   "https://github.com/theHamsta/tree-sitter-cuda",
	UpstreamCommit: "48b066f334f4cf2174e05a50218ce2ed98b6fd01",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "2033e9d09a6e5cd9353682644fdf509a57c39e03ec6124c8b0e11267106d92b5"},
		{Path: "src/scanner.c", SHA256: "31da79aef61980aaefa8d5bfd8a38e3a5930051d06ea2959a30b0e34e2c921ea"},
	},
	Externals: []string{
		"raw_string_delimiter",
		"raw_string_content",
	},
}

func init() {
	RegisterExternalScannerSpec(cudaExternalScannerSpec)
}

// CudaExternalScanner handles C++ R"delim(...)delim" raw string literals for
// CUDA. It reuses the shared rawstring_scanner.go helper (also used by cpp,
// arduino, and hlsl), passing this instance's bound symbols in as concrete
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
type CudaExternalScanner struct {
	symbols         [cudaTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers cuda's external symbols.
func (CudaExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CudaExternalScanner{symbols: cudaDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, cudaExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s CudaExternalScanner) symbolTable() *[cudaTokenCount]gotreesitter.Symbol {
	if s.symbols == ([cudaTokenCount]gotreesitter.Symbol{}) {
		return &cudaDefaultSymTable
	}
	return &s.symbols
}

func (CudaExternalScanner) Create() any         { return rawStringCreate() }
func (CudaExternalScanner) Destroy(payload any) {}
func (CudaExternalScanner) Serialize(payload any, buf []byte) int {
	return rawStringSerialize(payload, buf)
}
func (CudaExternalScanner) Deserialize(payload any, buf []byte) { rawStringDeserialize(payload, buf) }

func (s CudaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [cudaTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < cudaTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()
	return rawStringScan(payload, lexer, validSymbols,
		cudaTokRawStringDelimiter, cudaTokRawStringContent,
		syms[cudaTokRawStringDelimiter], syms[cudaTokRawStringContent])
}
