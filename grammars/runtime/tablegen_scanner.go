//go:build !grammar_subset || grammar_subset_tablegen

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the tablegen grammar.
const (
	tablegenTokMultilineComment = 0
	tablegenTokenCount          = 1
)

// tablegenDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped tablegen.bin assigns to each external, in tablegenTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var tablegenDefaultSymTable = [tablegenTokenCount]gotreesitter.Symbol{
	98, // multiline_comment
}

// tablegenExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (tablegenTok* order).
var tablegenExternalScannerSpec = ExternalScannerSpec{
	Language:       "tablegen",
	UpstreamRepo:   "https://github.com/amaanq/tree-sitter-tablegen",
	UpstreamCommit: "b1170880c61355aaf38fc06f4af7d3c55abdabc4",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "553c99aab3d545246592bc66dba47375b3cc7d3a72cff181ca381fe4dbee253e"},
		{Path: "src/scanner.c", SHA256: "d798961834df791e6df45932ab16aaa79327c8db4745b275c46f74bce2245fca"},
	},
	Externals: []string{
		"multiline_comment",
	},
}

func init() {
	RegisterExternalScannerSpec(tablegenExternalScannerSpec)
}

// TablegenExternalScanner handles nestable /* */ comments for TableGen.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type TablegenExternalScanner struct {
	symbols         [tablegenTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers tablegen's external symbols.
func (TablegenExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := TablegenExternalScanner{symbols: tablegenDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, tablegenExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s TablegenExternalScanner) symbolTable() *[tablegenTokenCount]gotreesitter.Symbol {
	if s.symbols == ([tablegenTokenCount]gotreesitter.Symbol{}) {
		return &tablegenDefaultSymTable
	}
	return &s.symbols
}

func (TablegenExternalScanner) Create() any                           { return nil }
func (TablegenExternalScanner) Destroy(payload any)                   {}
func (TablegenExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (TablegenExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (TablegenExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (TablegenExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (TablegenExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc TablegenExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [tablegenTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < tablegenTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if !tablegenValid(validSymbols, tablegenTokMultilineComment) {
		return false
	}

	// Skip leading whitespace before the comment delimiter. The C scanner
	// (scanner.c) does exactly this with advance(lexer, /*skip=*/true); without
	// it the scanner bails the moment the lexer is positioned on the space that
	// precedes "/*" (e.g. inside "{ /*x*/ }"), and the DFA then mis-lexes the
	// comment body. Passing skip=true moves only the token start, matching C.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if lexer.Lookahead() != '/' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)

	depth := 1
	afterStar := false
	for {
		ch := lexer.Lookahead()
		if ch == 0 {
			// Unterminated comment: C returns false (no token) on EOF.
			return false
		}
		if ch == '*' {
			lexer.Advance(false)
			afterStar = true
			continue
		}
		if ch == '/' {
			lexer.Advance(false)
			if afterStar {
				afterStar = false
				depth--
				if depth == 0 {
					lexer.MarkEnd()
					lexer.SetResultSymbol(syms[tablegenTokMultilineComment])
					return true
				}
			} else if lexer.Lookahead() == '*' {
				depth++
				lexer.Advance(false)
			}
			continue
		}
		lexer.Advance(false)
		afterStar = false
	}
}

func tablegenValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
