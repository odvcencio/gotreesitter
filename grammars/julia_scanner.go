//go:build !grammar_subset || grammar_subset_julia

package grammars

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the Julia grammar.
const (
	juliaTokBlockCommentRest      = 0
	juliaTokImmediateParen        = 1
	juliaTokImmediateBracket      = 2
	juliaTokImmediateBrace        = 3
	juliaTokImmediateStringStart  = 4
	juliaTokImmediateCommandStart = 5
	juliaTokContentCmd1           = 6
	juliaTokContentCmd1Raw        = 7
	juliaTokContentCmd3           = 8
	juliaTokContentCmd3Raw        = 9
	juliaTokContentStr1           = 10
	juliaTokContentStr1Raw        = 11
	juliaTokContentStr3           = 12
	juliaTokContentStr3Raw        = 13
	juliaTokEndCmd                = 14
	juliaTokEndStr                = 15
)

const (
	juliaSymBlockCommentRest      gotreesitter.Symbol = 104
	juliaSymImmediateParen        gotreesitter.Symbol = 105
	juliaSymImmediateBracket      gotreesitter.Symbol = 106
	juliaSymImmediateBrace        gotreesitter.Symbol = 107
	juliaSymImmediateStringStart  gotreesitter.Symbol = 108
	juliaSymImmediateCommandStart gotreesitter.Symbol = 109
	juliaSymContentCmd1           gotreesitter.Symbol = 110
	juliaSymContentCmd1Raw        gotreesitter.Symbol = 111
	juliaSymContentCmd3           gotreesitter.Symbol = 112
	juliaSymContentCmd3Raw        gotreesitter.Symbol = 113
	juliaSymContentStr1           gotreesitter.Symbol = 114
	juliaSymContentStr1Raw        gotreesitter.Symbol = 115
	juliaSymContentStr3           gotreesitter.Symbol = 116
	juliaSymContentStr3Raw        gotreesitter.Symbol = 117
	juliaSymEndCmd                gotreesitter.Symbol = 118
	juliaSymEndStr                gotreesitter.Symbol = 119
)

// JuliaExternalScanner handles block comments, immediate tokens, and string/command content for Julia.
type JuliaExternalScanner struct{}

func (JuliaExternalScanner) Create() any                           { return nil }
func (JuliaExternalScanner) Destroy(payload any)                   {}
func (JuliaExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (JuliaExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (JuliaExternalScanner) SupportsIncrementalReuse() bool   { return true }
func (JuliaExternalScanner) ExternalScannerIsStateless() bool { return true }

// These bytes follow the same branch in every scanner, including failed scans.
func (JuliaExternalScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' {
		return 1
	}
	return 0
}
func (JuliaExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (JuliaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// Immediate tokens: no whitespace consumed, just check current char.
	if juliaValid(validSymbols, juliaTokImmediateParen) && lexer.Lookahead() == '(' {
		lexer.SetResultSymbol(juliaSymImmediateParen)
		return true
	}
	if juliaValid(validSymbols, juliaTokImmediateBracket) && lexer.Lookahead() == '[' {
		lexer.SetResultSymbol(juliaSymImmediateBracket)
		return true
	}
	if juliaValid(validSymbols, juliaTokImmediateBrace) && lexer.Lookahead() == '{' {
		lexer.SetResultSymbol(juliaSymImmediateBrace)
		return true
	}
	if juliaValid(validSymbols, juliaTokImmediateStringStart) && lexer.Lookahead() == '"' {
		lexer.SetResultSymbol(juliaSymImmediateStringStart)
		return true
	}
	if juliaValid(validSymbols, juliaTokImmediateCommandStart) && lexer.Lookahead() == '`' {
		lexer.SetResultSymbol(juliaSymImmediateCommandStart)
		return true
	}

	// Block comment: #= ... =#  (nested)
	if juliaValid(validSymbols, juliaTokBlockCommentRest) {
		if juliaScanBlockComment(lexer) {
			return true
		}
	}

	// String and command content scanning
	if juliaValid(validSymbols, juliaTokContentStr1) {
		return juliaScanContent(lexer, juliaSymContentStr1, '"', 1, true)
	}
	if juliaValid(validSymbols, juliaTokContentStr3) {
		return juliaScanContent(lexer, juliaSymContentStr3, '"', 3, true)
	}
	if juliaValid(validSymbols, juliaTokContentCmd1) {
		return juliaScanContent(lexer, juliaSymContentCmd1, '`', 1, true)
	}
	if juliaValid(validSymbols, juliaTokContentCmd3) {
		return juliaScanContent(lexer, juliaSymContentCmd3, '`', 3, true)
	}
	if juliaValid(validSymbols, juliaTokContentStr1Raw) {
		return juliaScanContent(lexer, juliaSymContentStr1Raw, '"', 1, false)
	}
	if juliaValid(validSymbols, juliaTokContentStr3Raw) {
		return juliaScanContent(lexer, juliaSymContentStr3Raw, '"', 3, false)
	}
	if juliaValid(validSymbols, juliaTokContentCmd1Raw) {
		return juliaScanContent(lexer, juliaSymContentCmd1Raw, '`', 1, false)
	}
	if juliaValid(validSymbols, juliaTokContentCmd3Raw) {
		return juliaScanContent(lexer, juliaSymContentCmd3Raw, '`', 3, false)
	}

	return false
}

func juliaScanContent(lexer *gotreesitter.ExternalLexer, contentSym gotreesitter.Symbol, endChar rune, nDelim uint32, interp bool) bool {
	var endSym gotreesitter.Symbol
	if endChar == '"' {
		endSym = juliaSymEndStr
	} else {
		endSym = juliaSymEndCmd
	}
	hasContent := false

	for lexer.Lookahead() != 0 {
		lexer.MarkEnd()
		if interp && (lexer.Lookahead() == '$' || lexer.Lookahead() == '\\') {
			lexer.SetResultSymbol(contentSym)
			return hasContent
		} else if lexer.Lookahead() == '\\' {
			// Raw string: check escaped delimiters and '\\'
			lexer.Advance(false)
			if lexer.Lookahead() == endChar || lexer.Lookahead() == '\\' {
				lexer.SetResultSymbol(contentSym)
				return hasContent
			}
		} else {
			// Check for end delimiter sequence
			isEndDelimiter := true
			for i := uint32(1); i <= nDelim; i++ {
				if lexer.Lookahead() == endChar {
					lexer.Advance(false)
				} else {
					isEndDelimiter = false
					break
				}
			}
			if isEndDelimiter {
				if hasContent {
					lexer.SetResultSymbol(contentSym)
					return true
				}
				lexer.MarkEnd()
				lexer.SetResultSymbol(endSym)
				return true
			}
		}
		lexer.Advance(false)
		hasContent = true
	}
	return false
}

func juliaScanBlockComment(lexer *gotreesitter.ExternalLexer) bool {
	// The first #= was already consumed by tree-sitter.
	afterEq := false
	nestingDepth := uint32(1)

	for {
		switch lexer.Lookahead() {
		case '=':
			lexer.Advance(false)
			afterEq = true
		case '#':
			lexer.Advance(false)
			if afterEq {
				afterEq = false
				nestingDepth--
				if nestingDepth == 0 {
					lexer.SetResultSymbol(juliaSymBlockCommentRest)
					return true
				}
			} else {
				afterEq = false
				if lexer.Lookahead() == '=' {
					lexer.Advance(false)
					nestingDepth++
				}
			}
		case 0:
			return false
		default:
			lexer.Advance(false)
			afterEq = false
		}
	}
}

func juliaValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
