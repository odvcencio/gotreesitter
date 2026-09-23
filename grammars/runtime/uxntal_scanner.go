//go:build !grammar_subset || grammar_subset_uxntal

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the uxntal grammar.
const (
	uxntalTokComment = 0
	uxntalTokenCount = 1
)

// uxntalDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped uxntal.bin assigns to each external, in uxntalTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var uxntalDefaultSymTable = [uxntalTokenCount]gotreesitter.Symbol{
	285, // comment
}

// uxntalExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (uxntalTok* order).
var uxntalExternalScannerSpec = ExternalScannerSpec{
	Language:       "uxntal",
	UpstreamRepo:   "https://github.com/amaanq/tree-sitter-uxntal",
	UpstreamCommit: "ad9b638b914095320de85d59c49ab271603af048",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "4e8060bb9e4fcc156ca881bc43ac8471548fb621d0b47e93f13e36b08b30165c"},
		{Path: "src/scanner.c", SHA256: "e875acefdcbbffa1cc0dc5d371a88cc58cabb96e467ecb8c9de448f8e867fe72"},
	},
	Externals: []string{
		"comment",
	},
}

func init() {
	RegisterExternalScannerSpec(uxntalExternalScannerSpec)
}

// UxntalExternalScanner handles nestable ( ) Forth-style comments for Uxntal.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type UxntalExternalScanner struct {
	symbols         [uxntalTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers uxntal's external symbols.
func (UxntalExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := UxntalExternalScanner{symbols: uxntalDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, uxntalExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s UxntalExternalScanner) symbolTable() *[uxntalTokenCount]gotreesitter.Symbol {
	if s.symbols == ([uxntalTokenCount]gotreesitter.Symbol{}) {
		return &uxntalDefaultSymTable
	}
	return &s.symbols
}

func (UxntalExternalScanner) Create() any                           { return nil }
func (UxntalExternalScanner) Destroy(payload any)                   {}
func (UxntalExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (UxntalExternalScanner) Deserialize(payload any, buf []byte)   {}

// Comment depth is local to one Scan call. No state survives a token or a
// failed unterminated-comment scan.
func (UxntalExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (UxntalExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (UxntalExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc UxntalExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, _ []bool) bool {
	syms := sc.symbolTable()

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '(' {
		return false
	}
	lexer.Advance(false)

	depth := 1
	for depth > 0 {
		ch := lexer.Lookahead()
		if ch == 0 {
			return false
		}
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
		}
		lexer.Advance(false)
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[uxntalTokComment])
	return true
}
