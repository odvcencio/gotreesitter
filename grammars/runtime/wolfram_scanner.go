//go:build !grammar_subset || grammar_subset_wolfram

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the wolfram grammar.
const (
	wolframTokComment = 0
)

const (
	wolframSymComment gotreesitter.Symbol = 68
)

// WolframExternalScanner handles nestable (* *) comments for Wolfram/Mathematica.
type WolframExternalScanner struct{}

func (WolframExternalScanner) Create() any                           { return nil }
func (WolframExternalScanner) Destroy(payload any)                   {}
func (WolframExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (WolframExternalScanner) Deserialize(payload any, buf []byte)   {}

func (WolframExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
	lexer.SetResultSymbol(wolframSymComment)
	return true
}

func wolframValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
