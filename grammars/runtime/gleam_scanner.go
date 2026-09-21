//go:build !grammar_subset || grammar_subset_gleam

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the gleam grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see gleamDefaultSymTable below.
const (
	gleamTokQuotedContent     = 0 // "quoted_content" — string literal interior
	gleamTokDocCommentContent = 1 // "doc_comment_content" — doc comment line
	gleamTokenCount           = 2
)

// gleamDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped gleam.bin assigns to each external, in gleamTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var gleamDefaultSymTable = [gleamTokenCount]gotreesitter.Symbol{
	97, // quoted_content
	98, // doc_comment_content
}

// gleamExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (gleamTok* order).
var gleamExternalScannerSpec = ExternalScannerSpec{
	Language:       "gleam",
	UpstreamRepo:   "https://github.com/gleam-lang/tree-sitter-gleam",
	UpstreamCommit: "6ea757f7eb8d391dbf24dbb9461990757946dd5e",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "e9ceabaf6e41eefffac9e2ce5a139764cafdd1519eeed92d08906866c14d9d55"},
		{Path: "src/scanner.c", SHA256: "0c63f7194fa6ba5601b688122ad19bb4c1027cbfc9bb932cdfe7a49868202f7d"},
	},
	Externals: []string{
		"quoted_content",
		"doc_comment_content",
	},
}

func init() {
	RegisterExternalScannerSpec(gleamExternalScannerSpec)
}

// GleamExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-gleam.
//
// The gleam grammar uses an external scanner to produce two tokens:
//   - quoted_content: the interior of a string literal, consuming characters
//     until a closing " or escape \ is encountered.
//   - doc_comment_content: a single line of a doc comment, consuming
//     characters until end-of-line or EOF.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type GleamExternalScanner struct {
	symbols         [gleamTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers gleam's external symbols.
func (GleamExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := GleamExternalScanner{symbols: gleamDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, gleamExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s GleamExternalScanner) symbolTable() *[gleamTokenCount]gotreesitter.Symbol {
	if s.symbols == ([gleamTokenCount]gotreesitter.Symbol{}) {
		return &gleamDefaultSymTable
	}
	return &s.symbols
}

func (GleamExternalScanner) Create() any                           { return nil }
func (GleamExternalScanner) Destroy(payload any)                   {}
func (GleamExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (GleamExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (GleamExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (GleamExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (GleamExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc GleamExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [gleamTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < gleamTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if gleamValid(validSymbols, gleamTokQuotedContent) {
		return scanGleamQuotedContent(lexer, syms)
	}
	if gleamValid(validSymbols, gleamTokDocCommentContent) {
		return scanGleamDocCommentContent(lexer, syms)
	}
	return false
}

// scanGleamQuotedContent consumes string content until " or \ is hit.
// Returns true only if at least one character was consumed.
func scanGleamQuotedContent(lexer *gotreesitter.ExternalLexer, syms *[gleamTokenCount]gotreesitter.Symbol) bool {
	hasContent := false
	for {
		ch := lexer.Lookahead()
		if ch == '"' || ch == '\\' {
			break
		}
		if ch == 0 { // EOF
			return false
		}
		hasContent = true
		lexer.Advance(false)
	}
	if !hasContent {
		return false
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[gleamTokQuotedContent])
	return true
}

// scanGleamDocCommentContent consumes a single doc comment line until
// newline (inclusive) or EOF.
func scanGleamDocCommentContent(lexer *gotreesitter.ExternalLexer, syms *[gleamTokenCount]gotreesitter.Symbol) bool {
	for {
		ch := lexer.Lookahead()
		if ch == 0 { // EOF
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[gleamTokDocCommentContent])
			return true
		}
		if ch == '\n' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[gleamTokDocCommentContent])
			return true
		}
		lexer.Advance(false)
	}
}

func gleamValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
