//go:build !grammar_subset || grammar_subset_tcl

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the tcl grammar.
const (
	tclTokConcat    = 0
	tclTokImmediate = 1
	tclTokenCount   = 2
)

// tclDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped tcl.bin assigns to each external, in tclTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var tclDefaultSymTable = [tclTokenCount]gotreesitter.Symbol{
	67, // _concat
	68, // _immediate
}

// tclExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (tclTok* order).
var tclExternalScannerSpec = ExternalScannerSpec{
	Language:       "tcl",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-tcl",
	UpstreamCommit: "8f11ac7206a54ed11210491cee1e0657e2962c47",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "b782cf3ad24196740c8c80b18ddd59ebd7b903dab562e17bceb80f1f344e9c4d"},
		{Path: "src/scanner.c", SHA256: "3481bf00ba93ac1b77a0416ec97fdccfbd2008e0e3fd2f6eacfec827e376ff79"},
	},
	Externals: []string{
		"_concat",
		"_immediate",
	},
}

func init() {
	RegisterExternalScannerSpec(tclExternalScannerSpec)
}

// TclExternalScanner handles concatenation detection for Tcl.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type TclExternalScanner struct {
	symbols         [tclTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers tcl's external symbols.
func (TclExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := TclExternalScanner{symbols: tclDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, tclExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s TclExternalScanner) symbolTable() *[tclTokenCount]gotreesitter.Symbol {
	if s.symbols == ([tclTokenCount]gotreesitter.Symbol{}) {
		return &tclDefaultSymTable
	}
	return &s.symbols
}

func (TclExternalScanner) Create() any                           { return nil }
func (TclExternalScanner) Destroy(payload any)                   {}
func (TclExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (TclExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (TclExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (TclExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (TclExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc TclExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [tclTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < tclTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	ch := lexer.Lookahead()

	if tclValid(validSymbols, tclTokImmediate) && !unicode.IsSpace(ch) {
		lexer.SetResultSymbol(syms[tclTokImmediate])
		return false
	}

	if tclValid(validSymbols, tclTokConcat) &&
		!unicode.IsSpace(ch) &&
		ch != ')' && ch != ':' && ch != '}' && ch != ']' {
		lexer.SetResultSymbol(syms[tclTokConcat])
		return true
	}

	return false
}

func tclValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
