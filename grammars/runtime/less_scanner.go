//go:build !grammar_subset || grammar_subset_less

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the less grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see lessDefaultSymTable below.
const (
	lessTokDescendantOp = 0
	lessTokenCount      = 1
)

// lessDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped less.bin assigns to each external, in lessTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var lessDefaultSymTable = [lessTokenCount]gotreesitter.Symbol{
	68, // _descendant_operator
}

// lessExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (lessTok* order).
var lessExternalScannerSpec = ExternalScannerSpec{
	Language:       "less",
	UpstreamRepo:   "https://github.com/rhino1998/tree-sitter-less",
	UpstreamCommit: "2bd739e106a3485bca210cf7b6d25ba09fd10dff",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "76d4f7bd1a2b3d010b7617786f40c82c2f4988fa9124d51aa9354d89e90ca249"},
		{Path: "src/scanner.c", SHA256: "5e4cced265efa886eea71baea0726331f877af380d786c2baf40a9f0012d83a9"},
	},
	Externals: []string{
		"_descendant_operator",
	},
}

func init() {
	RegisterExternalScannerSpec(lessExternalScannerSpec)
}

// LessExternalScanner handles the CSS descendant combinator for Less.
// Nearly identical to the SCSS descendant operator scanner.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type LessExternalScanner struct {
	symbols         [lessTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers less's external symbols.
func (LessExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := LessExternalScanner{symbols: lessDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, lessExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s LessExternalScanner) symbolTable() *[lessTokenCount]gotreesitter.Symbol {
	if s.symbols == ([lessTokenCount]gotreesitter.Symbol{}) {
		return &lessDefaultSymTable
	}
	return &s.symbols
}

func (LessExternalScanner) Create() any                           { return nil }
func (LessExternalScanner) Destroy(payload any)                   {}
func (LessExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (LessExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (LessExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (LessExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (LessExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc LessExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	syms := sc.symbolTable()

	if !lessValid(validSymbols, lessTokDescendantOp) {
		return false
	}
	ch := lexer.Lookahead()
	if !isLessSpace(ch) {
		return false
	}
	lexer.Advance(true)
	for isLessSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[lessTokDescendantOp])

	next := lexer.Lookahead()
	if next == '#' || next == '.' || next == '[' || next == '-' ||
		next == '&' || next == '*' || unicode.IsLetter(next) || unicode.IsDigit(next) {
		return true
	}
	if next == ':' {
		lexer.Advance(false)
		if isLessSpace(lexer.Lookahead()) {
			return false
		}
		for {
			c := lexer.Lookahead()
			if c == ';' || c == '}' || c == 0 {
				return false
			}
			if c == '{' {
				return true
			}
			lexer.Advance(false)
		}
	}
	return false
}

func isLessSpace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
}

func lessValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
