//go:build !grammar_subset || grammar_subset_gn

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the gn grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner
// never hardcodes them -- see gnDefaultSymTable below.
const (
	gnTokStringContent = 0
	gnTokenCount       = 1
)

// gnDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped gn.bin assigns to each external, in gnTok* order. It
// exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var gnDefaultSymTable = [gnTokenCount]gotreesitter.Symbol{
	37, // _string_content
}

// gnExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (gnTok* order).
var gnExternalScannerSpec = ExternalScannerSpec{
	Language:       "gn",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-gn",
	UpstreamCommit: "bc06955bc1e3c9ff8e9b2b2a55b38b94da923c05",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "f43731f3721101c497f7c50c723b698a1ad3e5fb84f66d78472bcb085029feb1"},
		{Path: "src/scanner.c", SHA256: "5fa2247c32d9ec315144031abbb6841bb3ed2e888c5500826cb840892bedf3f7"},
	},
	Externals: []string{
		"_string_content",
	},
}

func init() {
	RegisterExternalScannerSpec(gnExternalScannerSpec)
}

// GnExternalScanner handles string content for GN (Generate Ninja) build files.
// Scans content inside "..." strings, stopping at closing quote, escape
// sequences, and ${...} interpolations.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type GnExternalScanner struct {
	symbols         [gnTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers gn's external symbols.
func (GnExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := GnExternalScanner{symbols: gnDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, gnExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s GnExternalScanner) symbolTable() *[gnTokenCount]gotreesitter.Symbol {
	if s.symbols == ([gnTokenCount]gotreesitter.Symbol{}) {
		return &gnDefaultSymTable
	}
	return &s.symbols
}

func (GnExternalScanner) Create() any                           { return nil }
func (GnExternalScanner) Destroy(payload any)                   {}
func (GnExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (GnExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (GnExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (GnExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (GnExternalScanner) PreservesStateOnScanFailure() bool { return true }

// Scan is a line-faithful port of the pinned upstream src/scanner.c
// (tree-sitter-grammars/tree-sitter-gn @ bc06955b).
func (sc GnExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [gnTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < gnTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if !gnValid(validSymbols, gnTokStringContent) {
		return false
	}
	didAdvance := false
	for {
		switch lexer.Lookahead() {
		case 0:
			// Lookahead()==0 covers both lexer->eof() and a literal NUL
			// byte; the C scanner returns false for either.
			return false
		case '\\':
			// The token ends before the backslash only when it begins a
			// recognized escape (\" \$ \\); otherwise both the backslash
			// and the escaped character become string content.
			lexer.MarkEnd()
			lexer.Advance(false)
			la := lexer.Lookahead()
			if la == '"' || la == '$' || la == '\\' {
				lexer.SetResultSymbol(syms[gnTokStringContent])
				return didAdvance
			}
			didAdvance = true
			lexer.Advance(false)
		case '$':
			// The token ends before '$' when it starts an interpolation:
			// ${...}, $identifier ($ followed by a letter or '_').
			// Otherwise '$' and the following character are content.
			lexer.MarkEnd()
			lexer.Advance(false)
			la := lexer.Lookahead()
			if la == '{' || gnIsAlpha(la) || la == '_' {
				lexer.SetResultSymbol(syms[gnTokStringContent])
				return didAdvance
			}
			didAdvance = true
			lexer.Advance(false)
		case '"':
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[gnTokStringContent])
			return didAdvance
		default:
			didAdvance = true
			lexer.Advance(false)
		}
	}
}

// gnIsAlpha mirrors C isalpha() in the C locale (ASCII letters only).
func gnIsAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func gnValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
