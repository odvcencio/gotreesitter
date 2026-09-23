//go:build !grammar_subset || grammar_subset_wolfram

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the wolfram grammar.
const (
	wolframTokComment = 0
	wolframTokenCount = 1
)

// wolframDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped wolfram.bin assigns to each external, in wolframTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var wolframDefaultSymTable = [wolframTokenCount]gotreesitter.Symbol{
	68, // comment
}

// wolframExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (wolframTok* order).
var wolframExternalScannerSpec = ExternalScannerSpec{
	Language:       "wolfram",
	UpstreamRepo:   "https://github.com/bostick/tree-sitter-wolfram",
	UpstreamCommit: "63ebdac6f040d9082d3d8fa88be96ce24549adc5",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "4f016cba1e48033a7254875584ee1277c58bd70d16d754f74b5150d576f7836e"},
		{Path: "src/scanner.cc", SHA256: "2f4c4a75382ca4629ba781691fc639a9bde4fafdb7f8989c23cbb4de3ec7bfa1"},
	},
	Externals: []string{
		"comment",
	},
}

func init() {
	RegisterExternalScannerSpec(wolframExternalScannerSpec)
}

// WolframExternalScanner handles nestable (* *) comments for Wolfram/Mathematica.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type WolframExternalScanner struct {
	symbols         [wolframTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers wolfram's external symbols.
func (WolframExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := WolframExternalScanner{symbols: wolframDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, wolframExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s WolframExternalScanner) symbolTable() *[wolframTokenCount]gotreesitter.Symbol {
	if s.symbols == ([wolframTokenCount]gotreesitter.Symbol{}) {
		return &wolframDefaultSymTable
	}
	return &s.symbols
}

func (WolframExternalScanner) Create() any                           { return nil }
func (WolframExternalScanner) Destroy(payload any)                   {}
func (WolframExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (WolframExternalScanner) Deserialize(payload any, buf []byte)   {}

func (sc WolframExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [wolframTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < wolframTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if !wolframValid(validSymbols, wolframTokComment) {
		return false
	}
	// Skip whitespace (matches upstream)
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}
	// Expect opening (*
	if lexer.Lookahead() != '(' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)

	depth := 1
	afterStar := false
	for depth > 0 {
		ch := lexer.Lookahead()
		if ch == 0 {
			break
		}
		if ch == '*' {
			lexer.Advance(false)
			afterStar = true
			continue
		}
		if ch == ')' && afterStar {
			lexer.Advance(false)
			afterStar = false
			depth--
			continue
		}
		if ch == '(' {
			lexer.Advance(false)
			afterStar = false
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				depth++
			}
			continue
		}
		lexer.Advance(false)
		afterStar = false
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[wolframTokComment])
	return true
}

func wolframValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
