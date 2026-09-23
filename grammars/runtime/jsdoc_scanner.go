//go:build !grammar_subset || grammar_subset_jsdoc

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the jsdoc grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see jsdocDefaultSymTable below.
const (
	jsdocTokType          = 0 // "type" — content between { and }
	jsdocTokCodeBlockLine = 1 // "code_block_line" (never emitted by this scanner)
	jsdocTokenCount       = 2
)

// jsdocDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped jsdoc.bin assigns to each external, in jsdocTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var jsdocDefaultSymTable = [jsdocTokenCount]gotreesitter.Symbol{
	24, // type
	17, // code_block_line (never emitted by this scanner)
}

// jsdocExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (jsdocTok* order).
var jsdocExternalScannerSpec = ExternalScannerSpec{
	Language:       "jsdoc",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-jsdoc",
	UpstreamCommit: "658d18dcdddb75c760363faa4963427a7c6b52db",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "9470c08833c165764d4f60b34ff702bfc7622f908c20ec755507a6edbaa183c2"},
		{Path: "src/scanner.c", SHA256: "4e8e9fdbfbdc306998afee3848227499a4c668534d5e5d5e7a858bff9e99071e"},
	},
	Externals: []string{
		"type",
		"code_block_line",
	},
}

func init() {
	RegisterExternalScannerSpec(jsdocExternalScannerSpec)
}

// JsdocExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-jsdoc.
//
// The jsdoc grammar uses an external scanner to match the "type" token,
// which represents text inside a JSDoc type annotation between { and }.
// The scanner tracks brace nesting depth and consumes all content until
// the unmatched closing } is found.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type JsdocExternalScanner struct {
	symbols         [jsdocTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers jsdoc's external symbols.
func (JsdocExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := JsdocExternalScanner{symbols: jsdocDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, jsdocExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s JsdocExternalScanner) symbolTable() *[jsdocTokenCount]gotreesitter.Symbol {
	if s.symbols == ([jsdocTokenCount]gotreesitter.Symbol{}) {
		return &jsdocDefaultSymTable
	}
	return &s.symbols
}

func (JsdocExternalScanner) Create() any                           { return nil }
func (JsdocExternalScanner) Destroy(payload any)                   {}
func (JsdocExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (JsdocExternalScanner) Deserialize(payload any, buf []byte)   {}

func (sc JsdocExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	syms := sc.symbolTable()

	if jsdocValid(validSymbols, jsdocTokType) {
		return scanJsdocType(lexer, syms)
	}
	return false
}

// scanJsdocType scans a type token inside { ... }. It consumes everything
// until the unmatched closing brace, tracking nested { } pairs. Returns
// false on EOF or newline.
func scanJsdocType(lexer *gotreesitter.ExternalLexer, syms *[jsdocTokenCount]gotreesitter.Symbol) bool {
	stack := 0
	for {
		ch := lexer.Lookahead()
		switch {
		case ch == 0: // EOF
			return false
		case ch == '{':
			stack++
		case ch == '}':
			stack--
			if stack == -1 {
				// Found the unmatched closing brace — emit token up to here.
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[jsdocTokType])
				return true
			}
		case ch == '\n' || ch == '\x00':
			return false
		}
		lexer.Advance(false)
	}
}

func jsdocValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
