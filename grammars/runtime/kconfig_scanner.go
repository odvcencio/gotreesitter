//go:build !grammar_subset || grammar_subset_kconfig

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the kconfig grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see kconfigDefaultSymTable below.
const (
	kconfigTokText    = 0
	kconfigTokenCount = 1
)

// kconfigDefaultSymTable records the concrete gotreesitter.Symbol IDs
// the currently shipped kconfig.bin assigns to each external, in
// kconfigTok* order. It exists only as a pre-bind fallback (and as an
// independent value to compare a real bind against in tests);
// ExternalScannerForLanguage below overwrites it with values read from
// the actual loaded Language at bind time, which is what the scanner
// must do to survive a future blob regen that renumbers absolute symbol
// IDs without touching the externals list order.
var kconfigDefaultSymTable = [kconfigTokenCount]gotreesitter.Symbol{
	63, // _help_text, displays as "text"
}

// kconfigExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (kconfigTok* order).
var kconfigExternalScannerSpec = ExternalScannerSpec{
	Language:       "kconfig",
	UpstreamRepo:   "https://github.com/amaanq/tree-sitter-kconfig",
	UpstreamCommit: "9ac99fe4c0c27a35dc6f757cef534c646e944881",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "9981922470b80cb79218232c4d0dba0a3fef7743ee039cead1c3aac9c7eec085"},
		{Path: "src/scanner.c", SHA256: "e5846578cbf0e576eafaa9a758fa27186015b06a360e5c2bcbe99599e7b21bba"},
	},
	Externals: []string{
		"_help_text",
	},
}

func init() {
	RegisterExternalScannerSpec(kconfigExternalScannerSpec)
}

// KconfigExternalScanner handles indented help text blocks in Linux Kconfig files.
// Help text continues as long as lines have consistent indentation.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type KconfigExternalScanner struct {
	symbols         [kconfigTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers kconfig's external symbols.
func (KconfigExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := KconfigExternalScanner{symbols: kconfigDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, kconfigExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s KconfigExternalScanner) symbolTable() *[kconfigTokenCount]gotreesitter.Symbol {
	if s.symbols == ([kconfigTokenCount]gotreesitter.Symbol{}) {
		return &kconfigDefaultSymTable
	}
	return &s.symbols
}

func (KconfigExternalScanner) Create() any                           { return nil }
func (KconfigExternalScanner) Destroy(payload any)                   {}
func (KconfigExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (KconfigExternalScanner) Deserialize(payload any, buf []byte)   {}
func (KconfigExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (KconfigExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (sc KconfigExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	syms := sc.symbolTable()

	if !kconfigValid(validSymbols, kconfigTokText) {
		return false
	}

	startCol := uint32(0)
	for unicode.IsSpace(lexer.Lookahead()) {
		startCol = kconfigScannerIndentColumn(startCol, lexer.Lookahead())
		lexer.Advance(true)
	}

	for {
		for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}

		if lexer.Lookahead() == 0 {
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[kconfigTokText])
			return true
		}

		lexer.MarkEnd()
		nextCol := uint32(0)
		for unicode.IsSpace(lexer.Lookahead()) {
			nextCol = kconfigScannerIndentColumn(nextCol, lexer.Lookahead())
			lexer.Advance(false)
		}

		if nextCol < startCol {
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[kconfigTokText])
			return true
		}
	}
}

func kconfigValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

func kconfigScannerIndentColumn(col uint32, ch rune) uint32 {
	switch ch {
	case ' ':
		return col + 1
	case '\t':
		col += 8
		return col - col%8
	default:
		return col
	}
}
