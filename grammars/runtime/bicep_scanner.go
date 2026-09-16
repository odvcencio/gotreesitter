//go:build !grammar_subset || grammar_subset_bicep

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the bicep grammar.
const (
	bicepTokExternalAsterisk       = 0
	bicepTokMultilineStringContent = 1
)

const (
	bicepSymExternalAsterisk       gotreesitter.Symbol = 77
	bicepSymMultilineStringContent gotreesitter.Symbol = 78
)

// bicepState stores the number of quotes to skip before the next content token.
type bicepState struct {
	quoteBeforeEndCount uint8
}

// BicepExternalScanner handles wildcard resource type matching and
// triple-single-quote multiline strings for Bicep.
type BicepExternalScanner struct{}

func (BicepExternalScanner) Create() any         { return &bicepState{} }
func (BicepExternalScanner) Destroy(payload any) {}
func (BicepExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*bicepState)
	buf[0] = s.quoteBeforeEndCount
	return 1
}
func (BicepExternalScanner) Deserialize(payload any, buf []byte) {
	if len(buf) >= 1 {
		s := payload.(*bicepState)
		s.quoteBeforeEndCount = buf[0]
	}
}

func (BicepExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*bicepState)

	if bicepValid(validSymbols, bicepTokExternalAsterisk) {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '*' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(bicepSymExternalAsterisk)
			if lexer.Lookahead() == ':' {
				return true
			}
		}
	}

	if bicepValid(validSymbols, bicepTokMultilineStringContent) {
		advancedOnce := false
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == '\'' {
				if s.quoteBeforeEndCount > 0 {
					for s.quoteBeforeEndCount > 0 {
						lexer.Advance(false)
						s.quoteBeforeEndCount--
					}
					lexer.SetResultSymbol(bicepSymMultilineStringContent)
					return true
				}

				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '\'' {
					lexer.Advance(false)
					if lexer.Lookahead() == '\'' {
						lexer.Advance(false)
						// Count extra quotes beyond the closing '''
						for lexer.Lookahead() == '\'' {
							s.quoteBeforeEndCount++
							lexer.Advance(false)
						}
						lexer.SetResultSymbol(bicepSymMultilineStringContent)
						return advancedOnce
					}
				}
			}
			lexer.Advance(false)
			advancedOnce = true
		}
	}

	return false
}

func bicepValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
