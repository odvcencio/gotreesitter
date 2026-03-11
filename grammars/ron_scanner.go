package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the ron grammar.
const (
	ronTokStringContent = 0
	ronTokRawString     = 1
	ronTokFloat         = 2
	ronTokBlockComment  = 3
)

const (
	ronSymStringContent gotreesitter.Symbol = 23
	ronSymRawString     gotreesitter.Symbol = 24
	ronSymFloat         gotreesitter.Symbol = 25
	ronSymBlockComment  gotreesitter.Symbol = 26
)

// RonExternalScanner handles string content, raw strings, floats,
// and nestable block comments for RON (Rusty Object Notation).
type RonExternalScanner struct{}

func (RonExternalScanner) Create() any                           { return nil }
func (RonExternalScanner) Destroy(payload any)                   {}
func (RonExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (RonExternalScanner) Deserialize(payload any, buf []byte)   {}

func (RonExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// String content (inside a "..." string)
	if ronValid(validSymbols, ronTokStringContent) && !ronValid(validSymbols, ronTokFloat) {
		hasContent := false
		for {
			ch := lexer.Lookahead()
			if ch == '"' || ch == '\\' {
				break
			}
			if ch == 0 {
				return false
			}
			hasContent = true
			lexer.Advance(false)
		}
		lexer.SetResultSymbol(ronSymStringContent)
		return hasContent
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Raw string: r#"..."# or br#"..."#
	if ronValid(validSymbols, ronTokRawString) && (lexer.Lookahead() == 'r' || lexer.Lookahead() == 'b') {
		lexer.SetResultSymbol(ronSymRawString)
		if lexer.Lookahead() == 'b' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() != 'r' {
			return false
		}
		lexer.Advance(false)

		openingHashes := uint32(0)
		for lexer.Lookahead() == '#' {
			lexer.Advance(false)
			openingHashes++
		}
		if lexer.Lookahead() != '"' {
			return false
		}
		lexer.Advance(false)

		for {
			if lexer.Lookahead() == 0 {
				return false
			}
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				hashCount := uint32(0)
				for lexer.Lookahead() == '#' && hashCount < openingHashes {
					lexer.Advance(false)
					hashCount++
				}
				if hashCount == openingHashes {
					lexer.MarkEnd()
					return true
				}
			} else {
				lexer.Advance(false)
			}
		}
	}

	// Float literal
	if ronValid(validSymbols, ronTokFloat) && unicode.IsDigit(lexer.Lookahead()) {
		lexer.SetResultSymbol(ronSymFloat)
		lexer.Advance(false)
		for ronIsNumChar(lexer.Lookahead()) {
			lexer.Advance(false)
		}

		hasFraction := false
		hasExponent := false

		if lexer.Lookahead() == '.' {
			hasFraction = true
			lexer.Advance(false)
			if unicode.IsLetter(lexer.Lookahead()) {
				return false
			}
			if lexer.Lookahead() == '.' {
				return false
			}
			for ronIsNumChar(lexer.Lookahead()) {
				lexer.Advance(false)
			}
		}

		lexer.MarkEnd()

		if lexer.Lookahead() == 'e' || lexer.Lookahead() == 'E' {
			hasExponent = true
			lexer.Advance(false)
			if lexer.Lookahead() == '+' || lexer.Lookahead() == '-' {
				lexer.Advance(false)
			}
			if !ronIsNumChar(lexer.Lookahead()) {
				return true
			}
			lexer.Advance(false)
			for ronIsNumChar(lexer.Lookahead()) {
				lexer.Advance(false)
			}
			lexer.MarkEnd()
		}

		if !hasExponent && !hasFraction {
			return false
		}

		if lexer.Lookahead() != 'u' && lexer.Lookahead() != 'i' && lexer.Lookahead() != 'f' {
			return true
		}
		lexer.Advance(false)
		if !unicode.IsDigit(lexer.Lookahead()) {
			return true
		}
		for unicode.IsDigit(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		return true
	}

	// Nestable block comment: /* ... */
	if lexer.Lookahead() == '/' {
		lexer.Advance(false)
		if lexer.Lookahead() != '*' {
			return false
		}
		lexer.Advance(false)

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
			case ch == '/':
				if afterStar {
					lexer.Advance(false)
					afterStar = false
					depth--
				} else {
					lexer.Advance(false)
					afterStar = false
					if lexer.Lookahead() == '*' {
						depth++
						lexer.Advance(false)
					}
				}
			default:
				lexer.Advance(false)
				afterStar = false
			}
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(ronSymBlockComment)
		return true
	}

	return false
}

func ronIsNumChar(ch rune) bool {
	return ch == '_' || unicode.IsDigit(ch)
}

func ronValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
