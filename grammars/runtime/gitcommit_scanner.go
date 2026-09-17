//go:build !grammar_subset || grammar_subset_gitcommit

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the gitcommit grammar.
const (
	gitcommitTokConventionalPrefix = 0
	gitcommitTokTrailerValue       = 1
)

const (
	gitcommitSymConventionalPrefix gotreesitter.Symbol = 456
)

// GitcommitExternalScanner handles conventional commit prefix detection.
type GitcommitExternalScanner struct{}

func (GitcommitExternalScanner) Create() any                           { return nil }
func (GitcommitExternalScanner) Destroy(payload any)                   {}
func (GitcommitExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (GitcommitExternalScanner) Deserialize(payload any, buf []byte)   {}

func (GitcommitExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !gitcommitValid(validSymbols, gitcommitTokConventionalPrefix) {
		return false
	}

	lexer.SetResultSymbol(gitcommitSymConventionalPrefix)

	ch := lexer.Lookahead()
	if unicode.IsControl(ch) || unicode.IsSpace(ch) || ch == ':' || ch == '!' || ch == 0 {
		return false
	}
	lexer.Advance(false)

	// Consume word characters (not control, space, colon, bang, parens)
	for {
		ch = lexer.Lookahead()
		if unicode.IsControl(ch) || unicode.IsSpace(ch) ||
			ch == ':' || ch == '!' || ch == '(' || ch == ')' || ch == 0 {
			break
		}
		lexer.Advance(false)
	}
	lexer.MarkEnd()

	// Optional scope in parentheses
	if lexer.Lookahead() == '(' {
		lexer.Advance(false)
		if lexer.Lookahead() == ')' {
			return false
		}
		for {
			ch = lexer.Lookahead()
			if unicode.IsControl(ch) || ch == '(' || ch == ')' || ch == 0 {
				break
			}
			lexer.Advance(false)
		}
		if lexer.Lookahead() != ')' {
			return false
		}
		lexer.Advance(false)
	}

	// Optional breaking change indicator
	if lexer.Lookahead() == '!' {
		lexer.Advance(false)
	}

	return lexer.Lookahead() == ':' || lexer.Lookahead() == 0xff1a
}

func gitcommitValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
