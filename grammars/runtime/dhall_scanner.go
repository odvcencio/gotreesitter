//go:build !grammar_subset || grammar_subset_dhall

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the dhall grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner
// never hardcodes them -- see dhallDefaultSymTable below.
const (
	dhallTokBlockCommentContent = 0
	dhallTokBlockCommentEnd     = 1
	dhallTokenCount             = 2
)

// dhallDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped dhall.bin assigns to each external, in dhallTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var dhallDefaultSymTable = [dhallTokenCount]gotreesitter.Symbol{
	127, // block_comment_content
	128, // block_comment_end (never emitted by this scanner)
}

// dhallExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (dhallTok* order).
var dhallExternalScannerSpec = ExternalScannerSpec{
	Language:       "dhall",
	UpstreamRepo:   "https://github.com/jbellerb/tree-sitter-dhall",
	UpstreamCommit: "62013259b26ac210d5de1abf64cf1b047ef88000",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "c3a1ae735c9aad8a503f3b31fc22433a92a00f3f9bef5f0d48d573f1bfec0523"},
		{Path: "src/scanner.c", SHA256: "98b1dad0c02816082b15195a275033db42a4563f670d4367a12ce33cfaa2fc5f"},
	},
	Externals: []string{
		"block_comment_content",
		"block_comment_end",
	},
}

func init() {
	RegisterExternalScannerSpec(dhallExternalScannerSpec)
}

// DhallExternalScanner handles nestable {- -} block comments for Dhall.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type DhallExternalScanner struct {
	symbols         [dhallTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers dhall's external symbols.
func (DhallExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := DhallExternalScanner{symbols: dhallDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, dhallExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s DhallExternalScanner) symbolTable() *[dhallTokenCount]gotreesitter.Symbol {
	if s.symbols == ([dhallTokenCount]gotreesitter.Symbol{}) {
		return &dhallDefaultSymTable
	}
	return &s.symbols
}

func (DhallExternalScanner) Create() any                           { return nil }
func (DhallExternalScanner) Destroy(payload any)                   {}
func (DhallExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (DhallExternalScanner) Deserialize(payload any, buf []byte)   {}
func (DhallExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (DhallExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (sc DhallExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [dhallTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < dhallTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if !dhallValid(validSymbols, dhallTokBlockCommentContent) {
		return false
	}
	depth := 1
	for depth > 0 {
		ch := lexer.Lookahead()
		switch {
		case ch == 0:
			return false
		case ch == '{':
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
				depth++
			}
		case ch == '-':
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '}' {
				depth--
			}
		default:
			lexer.Advance(false)
		}
	}
	lexer.SetResultSymbol(syms[dhallTokBlockCommentContent])
	return true
}

func dhallValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
