//go:build !grammar_subset || grammar_subset_odin

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the odin grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see odinDefaultSymTable below.
const (
	odinTokNewline      = 0
	odinTokBackslash    = 1
	odinTokNlComma      = 2
	odinTokFloat        = 3
	odinTokBlockComment = 4
	odinTokBracket      = 5
	odinTokQuote        = 6
	odinTokenCount      = 7
)

// odinDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped odin.bin assigns to each external, in odinTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var odinDefaultSymTable = [odinTokenCount]gotreesitter.Symbol{
	123, // _newline
	124, // _backslash
	125, // _nl_comma, displays as ","
	126, // float
	127, // block_comment
	2,   // "{" (never emitted by SetResultSymbol; checked only via isValid)
	110, // "\"" (never emitted by SetResultSymbol; checked only via isValid)
}

// odinExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (odinTok* order).
var odinExternalScannerSpec = ExternalScannerSpec{
	Language:       "odin",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-odin",
	UpstreamCommit: "d2ca8efb4487e156a60d5bd6db2598b872629403",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "17ee7bc8972a93e2cab56154f0ba077a9fdb6805da3e7af23f16e62c8195dc01"},
		{Path: "src/scanner.c", SHA256: "e342d07d3e35c3865a6bda0587f3a3b46a7b2411fd93cbee01dc40ab7631c93b"},
	},
	Externals: []string{
		"_newline",
		"_backslash",
		"_nl_comma",
		"float",
		"block_comment",
		"{",
		"\"",
	},
}

func init() {
	RegisterExternalScannerSpec(odinExternalScannerSpec)
}

// OdinExternalScanner handles newlines, floats, block comments, and other
// context-sensitive tokens for Odin.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type OdinExternalScanner struct {
	symbols         [odinTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers odin's external symbols.
func (OdinExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := OdinExternalScanner{symbols: odinDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, odinExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s OdinExternalScanner) symbolTable() *[odinTokenCount]gotreesitter.Symbol {
	if s.symbols == ([odinTokenCount]gotreesitter.Symbol{}) {
		return &odinDefaultSymTable
	}
	return &s.symbols
}

func (OdinExternalScanner) Create() any                           { return nil }
func (OdinExternalScanner) Destroy(payload any)                   {}
func (OdinExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (OdinExternalScanner) Deserialize(payload any, buf []byte)   {}
func (OdinExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (OdinExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (sc OdinExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [odinTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < odinTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	// FLOAT parsing
	if odinValid(validSymbols, odinTokFloat) {
		// Skip non-newline whitespace
		for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\n' {
			lexer.Advance(true)
		}
		if !odinValid(validSymbols, odinTokNewline) {
			for unicode.IsSpace(lexer.Lookahead()) {
				lexer.Advance(true)
			}
		}

		foundDecimal := false
		foundExponent := false
		foundNumBeforeDecimal := false
		foundNumAfterDecimal := false
		foundNumAfterExponent := false

		for i := 0; ; i++ {
			ch := lexer.Lookahead()
			switch {
			case ch == '.':
				if (foundDecimal || foundExponent) && (foundNumAfterDecimal || foundNumBeforeDecimal) {
					lexer.SetResultSymbol(syms[odinTokFloat])
					lexer.MarkEnd()
					return true
				}
				lexer.MarkEnd()
				foundDecimal = true
				lexer.Advance(false)
				if lexer.Lookahead() == '.' {
					lexer.Advance(false)
					goto newline
				}
				lexer.MarkEnd()
				if !odinIsDigit(lexer.Lookahead()) && (foundNumAfterDecimal || foundNumBeforeDecimal) {
					lexer.SetResultSymbol(syms[odinTokFloat])
					return true
				}
			case ch == 'i' || ch == 'j' || ch == 'k':
				if !foundNumAfterDecimal {
					goto newline
				}
				if (foundDecimal || foundExponent) && (foundNumAfterDecimal || foundNumBeforeDecimal) {
					lexer.Advance(false)
					lexer.SetResultSymbol(syms[odinTokFloat])
					lexer.MarkEnd()
					return true
				}
				goto newline
			case ch == 'e' || ch == 'E':
				if foundExponent && (foundNumAfterDecimal || foundNumBeforeDecimal) {
					lexer.SetResultSymbol(syms[odinTokFloat])
					lexer.MarkEnd()
					return true
				} else if foundNumBeforeDecimal || foundNumAfterDecimal {
					foundExponent = true
					lexer.Advance(false)
				} else {
					goto newline
				}
			case ch == '+' || ch == '-':
				if i == 0 || (foundExponent && !foundNumAfterExponent) {
					lexer.Advance(false)
				} else {
					goto newline
				}
			default:
				if odinIsDigit(ch) {
					lexer.Advance(false)
					if foundDecimal {
						foundNumAfterDecimal = true
					} else {
						foundNumBeforeDecimal = true
					}
					if foundExponent && !foundNumAfterExponent {
						foundNumAfterExponent = true
					}
				} else {
					if (foundDecimal || foundExponent) && (foundNumAfterDecimal || foundNumBeforeDecimal) {
						lexer.SetResultSymbol(syms[odinTokFloat])
						lexer.MarkEnd()
						return true
					}
					if foundNumBeforeDecimal {
						return false
					}
					goto newline
				}
			}
		}
	}

	// NL_COMMA
	if odinValid(validSymbols, odinTokNlComma) {
		for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\n' {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == ',' {
			lexer.Advance(false)
			lexer.SetResultSymbol(syms[odinTokNlComma])
			lexer.MarkEnd()
			for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\n' {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == '\n' {
				for unicode.IsSpace(lexer.Lookahead()) {
					lexer.Advance(false)
				}
				return lexer.Lookahead() != '}'
			}
		}
	}

newline:
	// NEWLINE
	if odinValid(validSymbols, odinTokNewline) {
		for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\n' {
			lexer.Advance(true)
		}

		if lexer.Lookahead() == '\n' {
			lexer.Advance(false)
			lexer.SetResultSymbol(syms[odinTokNewline])
			lexer.MarkEnd()

			nlCount := uint32(0)
			for unicode.IsSpace(lexer.Lookahead()) {
				if lexer.Lookahead() == '\n' {
					nlCount++
				}
				lexer.Advance(true)
			}

			// Check for "where" or "else" or "{"
			var nextWord [6]rune
			wordLen := 0
			for wordLen < 5 {
				if unicode.IsSpace(lexer.Lookahead()) {
					break
				}
				nextWord[wordLen] = lexer.Lookahead()
				wordLen++
				lexer.Advance(false)
			}

			word := string(nextWord[:wordLen])
			if word == "where" || word == "else" {
				if !unicode.IsSpace(lexer.Lookahead()) {
					return true
				}
				goto backslash
			}
			if word == "{" && nlCount == 0 && odinValid(validSymbols, odinTokBracket) {
				return false
			}
			return true
		}
	}

backslash:
	// BACKSLASH
	if odinValid(validSymbols, odinTokBackslash) && lexer.Lookahead() == '\\' {
		lexer.Advance(false)
		if lexer.Lookahead() == '\n' {
			lexer.Advance(false)
			for unicode.IsSpace(lexer.Lookahead()) {
				lexer.Advance(false)
			}
			lexer.SetResultSymbol(syms[odinTokBackslash])
			return true
		}
	}

	// Skip whitespace for block comment
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// BLOCK_COMMENT: nestable /* */
	if odinValid(validSymbols, odinTokBlockComment) && lexer.Lookahead() == '/' {
		lexer.Advance(false)
		if lexer.Lookahead() != '*' {
			return false
		}
		lexer.Advance(false)
		if lexer.Lookahead() == '"' {
			return false
		}

		afterStar := false
		depth := uint32(1)
		for depth > 0 {
			ch := lexer.Lookahead()
			switch {
			case ch == 0:
				return false
			case ch == '*':
				lexer.Advance(false)
				afterStar = true
			case ch == '/' && afterStar:
				lexer.Advance(false)
				afterStar = false
				depth--
			case ch == '/':
				lexer.Advance(false)
				afterStar = false
				if lexer.Lookahead() == '*' {
					depth++
					lexer.Advance(false)
				}
			default:
				lexer.Advance(false)
				afterStar = false
			}
		}
		lexer.SetResultSymbol(syms[odinTokBlockComment])
		return true
	}

	return false
}

func odinIsDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func odinValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
