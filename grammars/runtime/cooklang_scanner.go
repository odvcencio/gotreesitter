//go:build !grammar_subset || grammar_subset_cooklang

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the cooklang grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see cooklangDefaultSymTable below.
const (
	cooklangTokNewline = 0
	cooklangTokenCount = 1
)

// cooklangDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped cooklang.bin assigns to each external, in
// cooklangTok* order. It exists only as a pre-bind fallback (and as an
// independent value to compare a real bind against in tests);
// ExternalScannerForLanguage below overwrites it with values read from
// the actual loaded Language at bind time, which is what the scanner
// must do to survive a future blob regen that renumbers absolute symbol
// IDs without touching the externals list order.
var cooklangDefaultSymTable = [cooklangTokenCount]gotreesitter.Symbol{
	25, // _newline
}

// cooklangExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (cooklangTok* order).
var cooklangExternalScannerSpec = ExternalScannerSpec{
	Language:       "cooklang",
	UpstreamRepo:   "https://github.com/addcninblue/tree-sitter-cooklang",
	UpstreamCommit: "4ebe237c1cf64cf3826fc249e9ec0988fe07e58e",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "0a74637ef3823d13b9a9ec11ac6807a89c407abd749f69e0b1e1629a1d6ec4b6"},
		{Path: "src/scanner.c", SHA256: "ddde462ee78fa7e4a67d4c94772cda140d94ed7bca565b623215d62550d455f9"},
	},
	Externals: []string{
		"_newline",
	},
}

func init() {
	RegisterExternalScannerSpec(cooklangExternalScannerSpec)
}

// CooklangExternalScanner handles newline detection for Cooklang recipe files.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type CooklangExternalScanner struct {
	symbols         [cooklangTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers cooklang's external symbols.
func (CooklangExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CooklangExternalScanner{symbols: cooklangDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, cooklangExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s CooklangExternalScanner) symbolTable() *[cooklangTokenCount]gotreesitter.Symbol {
	if s.symbols == ([cooklangTokenCount]gotreesitter.Symbol{}) {
		return &cooklangDefaultSymTable
	}
	return &s.symbols
}

func (CooklangExternalScanner) Create() any                           { return nil }
func (CooklangExternalScanner) Destroy(payload any)                   {}
func (CooklangExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (CooklangExternalScanner) Deserialize(payload any, buf []byte)   {}

func (s CooklangExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [cooklangTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < cooklangTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	if !cooklangValid(validSymbols, cooklangTokNewline) {
		return false
	}
	ch := lexer.Lookahead()
	if ch == '\n' {
		lexer.SetResultSymbol(syms[cooklangTokNewline])
		return true
	}
	return false
}

func cooklangValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
