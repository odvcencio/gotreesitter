//go:build !grammar_subset || grammar_subset_odin

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the odin grammar.
const (
	odinTokNewline      = 0
	odinTokBackslash    = 1
	odinTokNlComma      = 2
	odinTokFloat        = 3
	odinTokBlockComment = 4
	odinTokBracket      = 5
	odinTokQuote        = 6
)

const (
	odinSymNewline      gotreesitter.Symbol = 123
	odinSymBackslash    gotreesitter.Symbol = 124
	odinSymNlComma      gotreesitter.Symbol = 125
	odinSymFloat        gotreesitter.Symbol = 126
	odinSymBlockComment gotreesitter.Symbol = 127
)

// OdinExternalScanner handles newlines, floats, block comments, and other
// context-sensitive tokens for Odin.
type OdinExternalScanner struct{}

func (OdinExternalScanner) Create() any                           { return nil }
func (OdinExternalScanner) Destroy(payload any)                   {}
func (OdinExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (OdinExternalScanner) Deserialize(payload any, buf []byte)   {}
func (OdinExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (OdinExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (OdinExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
					lexer.SetResultSymbol(odinSymFloat)
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
					lexer.SetResultSymbol(odinSymFloat)
					return true
				}
			case ch == 'i' || ch == 'j' || ch == 'k':
				if !foundNumAfterDecimal {
					goto newline
				}
				if (foundDecimal || foundExponent) && (foundNumAfterDecimal || foundNumBeforeDecimal) {
					lexer.Advance(false)
					lexer.SetResultSymbol(odinSymFloat)
					lexer.MarkEnd()
					return true
				}
				goto newline
			case ch == 'e' || ch == 'E':
				if foundExponent && (foundNumAfterDecimal || foundNumBeforeDecimal) {
					lexer.SetResultSymbol(odinSymFloat)
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
						lexer.SetResultSymbol(odinSymFloat)
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
			lexer.SetResultSymbol(odinSymNlComma)
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
			lexer.SetResultSymbol(odinSymNewline)
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
			lexer.SetResultSymbol(odinSymBackslash)
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
		lexer.SetResultSymbol(odinSymBlockComment)
		return true
	}

	return false
}

func odinIsDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func odinValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
