//go:build !grammar_subset || grammar_subset_fish

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the fish grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner
// never hardcodes them -- see fishDefaultSymTable below. fishTokBracketConcat
// disagrees with the upstream rule name at the same index (_brace_concat,
// per fishExternalScannerSpec.Externals below); this is a pre-existing
// naming quirk in this hand port, not fixed here since positional binding
// does not depend on the name matching.
const (
	fishTokConcat        = 0
	fishTokBracketConcat = 1
	fishTokConcatList    = 2
	fishTokBeginBrace    = 3
	fishTokenCount       = 4
)

// fishDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped fish.bin assigns to each external, in fishTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var fishDefaultSymTable = [fishTokenCount]gotreesitter.Symbol{
	53, // _concat
	54, // _brace_concat
	55, // _concat_list
	56, // _begin_brace, displays as "{"
}

// fishExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (fishTok* order).
var fishExternalScannerSpec = ExternalScannerSpec{
	Language:       "fish",
	UpstreamRepo:   "https://github.com/ram02z/tree-sitter-fish",
	UpstreamCommit: "fa2143f5d66a9eb6c007ba9173525ea7aaafe788",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "982322cb843ef4781052d7e882eb3de513207bc133297982573785ee3b08b0f4"},
		{Path: "src/scanner.c", SHA256: "ecf7bd668bf07280d468df5dad5fc66ef63952d41fb7db588976205f328ee647"},
	},
	Externals: []string{
		"_concat",
		"_brace_concat",
		"_concat_list",
		"_begin_brace",
	},
}

func init() {
	RegisterExternalScannerSpec(fishExternalScannerSpec)
}

// FishExternalScanner handles concatenation detection and begin-brace
// disambiguation for the Fish shell.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type FishExternalScanner struct {
	symbols         [fishTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers fish's external symbols.
func (FishExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := FishExternalScanner{symbols: fishDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, fishExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s FishExternalScanner) symbolTable() *[fishTokenCount]gotreesitter.Symbol {
	if s.symbols == ([fishTokenCount]gotreesitter.Symbol{}) {
		return &fishDefaultSymTable
	}
	return &s.symbols
}

func (FishExternalScanner) Create() any                           { return nil }
func (FishExternalScanner) Destroy(payload any)                   {}
func (FishExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (FishExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (FishExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (FishExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (FishExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc FishExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [fishTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < fishTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	// BEGIN_BRACE: { followed by whitespace or ;
	if fishValid(validSymbols, fishTokBeginBrace) {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '{' {
			lexer.Advance(false)
			if lexer.Lookahead() == ';' || unicode.IsSpace(lexer.Lookahead()) {
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[fishTokBeginBrace])
				return true
			}
		}
	}

	// CONCAT_LIST: next char is [
	if fishValid(validSymbols, fishTokConcatList) {
		if lexer.Lookahead() == '[' {
			lexer.SetResultSymbol(syms[fishTokConcatList])
			return true
		}
	}

	// CONCAT: not followed by certain chars
	if fishValid(validSymbols, fishTokConcat) {
		ch := lexer.Lookahead()
		if ch != 0 && ch != '>' && ch != '<' && ch != ')' &&
			ch != ';' && ch != '&' && ch != '|' && !unicode.IsSpace(ch) {
			lexer.SetResultSymbol(syms[fishTokConcat])
			return true
		}
	}

	// BRACKET_CONCAT
	if fishValid(validSymbols, fishTokBracketConcat) {
		ch := lexer.Lookahead()
		if ch != 0 && ch != ')' && ch != '(' && ch != '}' &&
			ch != ',' && !unicode.IsSpace(ch) {
			lexer.SetResultSymbol(syms[fishTokBracketConcat])
			return true
		}
	}

	return false
}

func fishValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
