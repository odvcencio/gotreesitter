//go:build !grammar_subset || grammar_subset_pkl

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Pkl grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see pklDefaultSymTable below.
const (
	pklTokSlStringChars        = 0
	pklTokSl1StringChars       = 1
	pklTokSl2StringChars       = 2
	pklTokSl3StringChars       = 3
	pklTokSl4StringChars       = 4
	pklTokSl5StringChars       = 5
	pklTokSl6StringChars       = 6
	pklTokMlStringChars        = 7
	pklTokMl1StringChars       = 8
	pklTokMl2StringChars       = 9
	pklTokMl3StringChars       = 10
	pklTokMl4StringChars       = 11
	pklTokMl5StringChars       = 12
	pklTokMl6StringChars       = 13
	pklTokOpenSubscriptBracket = 14
	pklTokOpenArgumentParen    = 15
	pklTokBinaryMinus          = 16
	pklTokenCount              = 17
)

// pklDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped pkl.bin assigns to each external, in pklTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var pklDefaultSymTable = [pklTokenCount]gotreesitter.Symbol{
	127, // _sl_string_chars
	128, // _sl1_string_chars
	129, // _sl2_string_chars
	130, // _sl3_string_chars
	131, // _sl4_string_chars
	132, // _sl5_string_chars
	133, // _sl6_string_chars
	134, // _ml_string_chars
	135, // _ml1_string_chars
	136, // _ml2_string_chars
	137, // _ml3_string_chars
	138, // _ml4_string_chars
	139, // _ml5_string_chars
	140, // _ml6_string_chars
	141, // _open_subscript_bracket, displays as "["
	142, // _open_argument_paren, displays as "("
	143, // _binary_minus, displays as "-"
}

// pklExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (pklTok* order).
var pklExternalScannerSpec = ExternalScannerSpec{
	Language:       "pkl",
	UpstreamRepo:   "https://github.com/apple/tree-sitter-pkl",
	UpstreamCommit: "a02fc36f6001a22e7fdf35eaabbadb7b39c74ba5",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "c9bf13e6a14f1ac87a23fbd065a28a43c137e426d985b49550e3c0608095b78d"},
		{Path: "src/scanner.c", SHA256: "6cac8c9af1515e2576b9ab2df82fa96b6b0f696ecd56aaf40eedec618c7c0886"},
	},
	Externals: []string{
		"_sl_string_chars",
		"_sl1_string_chars",
		"_sl2_string_chars",
		"_sl3_string_chars",
		"_sl4_string_chars",
		"_sl5_string_chars",
		"_sl6_string_chars",
		"_ml_string_chars",
		"_ml1_string_chars",
		"_ml2_string_chars",
		"_ml3_string_chars",
		"_ml4_string_chars",
		"_ml5_string_chars",
		"_ml6_string_chars",
		"_open_subscript_bracket",
		"_open_argument_paren",
		"_binary_minus",
	},
}

func init() {
	RegisterExternalScannerSpec(pklExternalScannerSpec)
}

// PklExternalScanner handles string content and contextual operators for Pkl.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type PklExternalScanner struct {
	symbols         [pklTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers pkl's external symbols.
func (PklExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := PklExternalScanner{symbols: pklDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, pklExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s PklExternalScanner) symbolTable() *[pklTokenCount]gotreesitter.Symbol {
	if s.symbols == ([pklTokenCount]gotreesitter.Symbol{}) {
		return &pklDefaultSymTable
	}
	return &s.symbols
}

func (PklExternalScanner) Create() any                           { return nil }
func (PklExternalScanner) Destroy(payload any)                   {}
func (PklExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (PklExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (PklExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (PklExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (PklExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc PklExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [pklTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < pklTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	// Error recovery: if all string tokens valid, bail out.
	if pklValid(validSymbols, pklTokSlStringChars) && pklValid(validSymbols, pklTokSl1StringChars) &&
		pklValid(validSymbols, pklTokSl2StringChars) && pklValid(validSymbols, pklTokSl3StringChars) &&
		pklValid(validSymbols, pklTokSl4StringChars) && pklValid(validSymbols, pklTokSl5StringChars) &&
		pklValid(validSymbols, pklTokSl6StringChars) && pklValid(validSymbols, pklTokMlStringChars) &&
		pklValid(validSymbols, pklTokMl1StringChars) && pklValid(validSymbols, pklTokMl2StringChars) &&
		pklValid(validSymbols, pklTokMl3StringChars) && pklValid(validSymbols, pklTokMl4StringChars) &&
		pklValid(validSymbols, pklTokMl5StringChars) && pklValid(validSymbols, pklTokMl6StringChars) &&
		pklValid(validSymbols, pklTokOpenSubscriptBracket) && pklValid(validSymbols, pklTokOpenArgumentParen) &&
		pklValid(validSymbols, pklTokBinaryMinus) {
		return false
	}

	// Single-line string without pounds
	if pklValid(validSymbols, pklTokSlStringChars) {
		return pklParseSlStringChars(lexer, syms)
	}
	// Multi-line string without pounds
	if pklValid(validSymbols, pklTokMlStringChars) {
		return pklParseMlStringChars(lexer, syms)
	}
	// Single-line strings with N pounds
	for i := 1; i <= 6; i++ {
		if pklValid(validSymbols, pklTokSlStringChars+i) {
			return pklParseSlxStringChars(lexer, i, syms)
		}
	}
	// Multi-line strings with N pounds
	for i := 1; i <= 6; i++ {
		if pklValid(validSymbols, pklTokMlStringChars+i) {
			return pklParseMlxStringChars(lexer, i, syms)
		}
	}
	// Contextual operators: [, (, -
	if pklValid(validSymbols, pklTokOpenSubscriptBracket) || pklValid(validSymbols, pklTokOpenArgumentParen) ||
		pklValid(validSymbols, pklTokBinaryMinus) {
		return pklParseContextualOp(lexer, validSymbols, syms)
	}

	return false
}

// pklParseSlStringChars: simple single-line string content (no pound variant).
func pklParseSlStringChars(lexer *gotreesitter.ExternalLexer, syms *[pklTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[pklTokSlStringChars])
	hasContent := false
	for {
		switch lexer.Lookahead() {
		case '"', '\\', '\n', '\r', 0:
			return hasContent
		default:
			hasContent = true
			lexer.Advance(false)
		}
	}
}

// pklParseSlxStringChars: single-line string content with N pound signs.
func pklParseSlxStringChars(lexer *gotreesitter.ExternalLexer, numPounds int, syms *[pklTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[pklTokSlStringChars+numPounds])
	hasContent := false
	for {
		switch lexer.Lookahead() {
		case '"', '\\':
			lexer.MarkEnd()
			lexer.Advance(false)
			matched := true
			for i := 0; i < numPounds; i++ {
				if lexer.Lookahead() != '#' {
					matched = false
					hasContent = true
					break
				}
				lexer.Advance(false)
			}
			if matched {
				return hasContent
			}
		case '\n', '\r', 0:
			lexer.MarkEnd()
			return hasContent
		default:
			hasContent = true
			lexer.Advance(false)
		}
	}
}

// pklParseMlStringChars: multi-line string content (no pound variant).
func pklParseMlStringChars(lexer *gotreesitter.ExternalLexer, syms *[pklTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[pklTokMlStringChars])
	hasContent := false
	for {
		switch lexer.Lookahead() {
		case '"':
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					return hasContent
				}
			}
			hasContent = true
		case '\\', 0:
			lexer.MarkEnd()
			return hasContent
		default:
			hasContent = true
			lexer.Advance(false)
		}
	}
}

// pklParseMlxStringChars: multi-line string content with N pound signs.
func pklParseMlxStringChars(lexer *gotreesitter.ExternalLexer, numPounds int, syms *[pklTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(syms[pklTokMlStringChars+numPounds])
	hasContent := false
	for {
		switch lexer.Lookahead() {
		case '"':
			lexer.MarkEnd()
			quoteCount := 0
			for lexer.Lookahead() == '"' {
				quoteCount++
				lexer.Advance(false)
			}
			if quoteCount < 3 {
				hasContent = true
				continue
			}
			matched := true
			for i := 0; i < numPounds; i++ {
				if lexer.Lookahead() != '#' {
					matched = false
					hasContent = true
					break
				}
				lexer.Advance(false)
			}
			if matched {
				return hasContent
			}
		case '\\':
			lexer.MarkEnd()
			lexer.Advance(false)
			matched := true
			for i := 0; i < numPounds; i++ {
				if lexer.Lookahead() != '#' {
					matched = false
					hasContent = true
					break
				}
				lexer.Advance(false)
			}
			if matched {
				return hasContent
			}
		case 0:
			lexer.MarkEnd()
			return hasContent
		default:
			hasContent = true
			lexer.Advance(false)
		}
	}
}

// pklParseContextualOp: handles [, (, - that can't have preceding newline or semicolon.
func pklParseContextualOp(lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[pklTokenCount]gotreesitter.Symbol) bool {
	if lexer.Lookahead() == 0 {
		return false
	}
	for {
		switch lexer.Lookahead() {
		case ' ', '\t', '\r', '\f':
			lexer.Advance(true)
		case '[':
			if pklValid(validSymbols, pklTokOpenSubscriptBracket) {
				lexer.Advance(false)
				lexer.SetResultSymbol(syms[pklTokOpenSubscriptBracket])
				return true
			}
			return false
		case '(':
			if pklValid(validSymbols, pklTokOpenArgumentParen) {
				lexer.Advance(false)
				lexer.SetResultSymbol(syms[pklTokOpenArgumentParen])
				return true
			}
			return false
		case '-':
			if pklValid(validSymbols, pklTokBinaryMinus) {
				lexer.Advance(false)
				lexer.MarkEnd()
				// Don't match -> as binary minus
				if lexer.Lookahead() == '>' {
					return false
				}
				lexer.SetResultSymbol(syms[pklTokBinaryMinus])
				return true
			}
			return false
		default:
			return false
		}
	}
}

func pklValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
