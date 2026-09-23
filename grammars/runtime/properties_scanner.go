//go:build !grammar_subset || grammar_subset_properties

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the properties grammar. This is the
// external index (the position of the token in the grammar's
// `externals: [...]` list), which is exactly what tree-sitter's
// `valid_symbols` array and C's result_symbol enum are indexed by. The
// external index is stable across a blob regen as long as the externals
// list itself does not reorder; concrete numeric gotreesitter.Symbol IDs
// are NOT stable (they shift whenever the grammar's total symbol count
// changes), so this scanner never hardcodes them -- see
// propertiesDefaultSymTable below.
const (
	propertiesTokEof     = 0
	propertiesTokenCount = 1
)

// propertiesDefaultSymTable records the concrete gotreesitter.Symbol IDs
// the currently shipped properties.bin assigns to each external, in
// propertiesTok* order. It exists only as a pre-bind fallback (and as an
// independent value to compare a real bind against in tests);
// ExternalScannerForLanguage below overwrites it with values read from
// the actual loaded Language at bind time, which is what the scanner
// must do to survive a future blob regen that renumbers absolute symbol
// IDs without touching the externals list order.
var propertiesDefaultSymTable = [propertiesTokenCount]gotreesitter.Symbol{
	16, // _eof
}

// propertiesExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (propertiesTok* order).
var propertiesExternalScannerSpec = ExternalScannerSpec{
	Language:       "properties",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-properties",
	UpstreamCommit: "6310671b24d4e04b803577b1c675d765cbd5773b",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "9ea8512b5767162cb7daf1e2afdbbaf9a5240ae6593598134e30e2b9ba948b62"},
		{Path: "src/scanner.c", SHA256: "28695124310dee60ae3cfcd18445e3e4d7aba6e307fd152c375ea2b929115068"},
	},
	Externals: []string{
		"_eof",
	},
}

func init() {
	RegisterExternalScannerSpec(propertiesExternalScannerSpec)
}

type propertiesScannerState struct {
	emittedEOF bool
}

// PropertiesExternalScanner handles EOF detection for Java .properties files.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type PropertiesExternalScanner struct {
	symbols         [propertiesTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers properties' external symbols.
func (PropertiesExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := PropertiesExternalScanner{symbols: propertiesDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, propertiesExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s PropertiesExternalScanner) symbolTable() *[propertiesTokenCount]gotreesitter.Symbol {
	if s.symbols == ([propertiesTokenCount]gotreesitter.Symbol{}) {
		return &propertiesDefaultSymTable
	}
	return &s.symbols
}

func (PropertiesExternalScanner) Create() any {
	return &propertiesScannerState{}
}

func (PropertiesExternalScanner) Destroy(payload any) {}

func (PropertiesExternalScanner) Serialize(payload any, buf []byte) int {
	st := payload.(*propertiesScannerState)
	if len(buf) == 0 {
		return 0
	}
	if st.emittedEOF {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	return 1
}

func (PropertiesExternalScanner) Deserialize(payload any, buf []byte) {
	st := payload.(*propertiesScannerState)
	if len(buf) == 0 {
		st.emittedEOF = false
		return
	}
	st.emittedEOF = buf[0] != 0
}

func (sc PropertiesExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [propertiesTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < propertiesTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	st := payload.(*propertiesScannerState)
	if !propertiesValid(validSymbols, propertiesTokEof) {
		return false
	}
	if lexer.Lookahead() == 0 {
		if st.emittedEOF {
			return false
		}
		st.emittedEOF = true
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[propertiesTokEof])
		return true
	}
	st.emittedEOF = false
	return false
}

func propertiesValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
