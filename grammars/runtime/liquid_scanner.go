//go:build !grammar_subset || grammar_subset_liquid

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the liquid grammar.
const (
	liquidTokInlineCommentContent    = 0
	liquidTokPairedCommentContent    = 1
	liquidTokPairedCommentContentLiq = 2
	liquidTokRawContent              = 3
	liquidTokFrontMatter             = 4
	liquidTokErrorSentinel           = 5
)

const (
	liquidSymInlineCommentContent    gotreesitter.Symbol = 96
	liquidSymPairedCommentContent    gotreesitter.Symbol = 97
	liquidSymPairedCommentContentLiq gotreesitter.Symbol = 98
	liquidSymRawContent              gotreesitter.Symbol = 99
	liquidSymFrontMatter             gotreesitter.Symbol = 100
	liquidSymErrorSentinel           gotreesitter.Symbol = 101
)

// LiquidExternalScanner handles comment content, raw blocks, and front matter for Liquid templates.
type LiquidExternalScanner struct{}

func (LiquidExternalScanner) Create() any                           { return nil }
func (LiquidExternalScanner) Destroy(payload any)                   {}
func (LiquidExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (LiquidExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (LiquidExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (LiquidExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (LiquidExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (LiquidExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// Error recovery
	if liquidValid(validSymbols, liquidTokErrorSentinel) {
		return false
	}

	// Front matter: ---\n...\n---\n
	if liquidValid(validSymbols, liquidTokFrontMatter) {
		return liquidScanFrontMatter(lexer)
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Inline comment: # ... (until %} or newline)
	if liquidValid(validSymbols, liquidTokInlineCommentContent) {
		if lexer.Lookahead() == '#' {
			lexer.SetResultSymbol(liquidSymInlineCommentContent)
			lexer.Advance(false)
			for lexer.Lookahead() != 0 {
				lexer.MarkEnd()
				if lexer.Lookahead() == '\n' {
					lexer.Advance(false)
					lexer.MarkEnd()
					return true
				}
				if lexer.Lookahead() == '%' {
					lexer.Advance(false)
					if lexer.Lookahead() == '}' {
						lexer.Advance(false)
						return true
					}
				}
				lexer.Advance(false) // consume other chars
			}
		}
	}

	// Paired comment or raw content
	if liquidValid(validSymbols, liquidTokPairedCommentContent) ||
		liquidValid(validSymbols, liquidTokPairedCommentContentLiq) ||
		liquidValid(validSymbols, liquidTokRawContent) {
		return liquidScanPairedContent(lexer, validSymbols)
	}

	return false
}

func liquidScanFrontMatter(lexer *gotreesitter.ExternalLexer) bool {
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	// Skip trailing spaces/tabs
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' {
		return false
	}

	for {
		// Advance over newline
		if lexer.Lookahead() == '\r' {
			lexer.Advance(false)
			if lexer.Lookahead() == '\n' {
				lexer.Advance(false)
			}
		} else {
			lexer.Advance(false)
		}
		// Check for dashes
		dashCount := 0
		for lexer.Lookahead() == '-' {
			dashCount++
			lexer.Advance(false)
		}
		if dashCount == 3 {
			for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || lexer.Lookahead() == 0 {
				if lexer.Lookahead() == '\r' {
					lexer.Advance(false)
					if lexer.Lookahead() == '\n' {
						lexer.Advance(false)
					}
				} else if lexer.Lookahead() != 0 {
					lexer.Advance(false)
				}
				lexer.MarkEnd()
				lexer.SetResultSymbol(liquidSymFrontMatter)
				return true
			}
		}
		// Consume rest of line
		for lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == 0 {
			return false
		}
	}
}

func liquidScanStr(lexer *gotreesitter.ExternalLexer, s string) bool {
	for _, ch := range s {
		if lexer.Lookahead() != ch {
			return false
		}
		lexer.Advance(false)
	}
	return true
}

func liquidScanPairedContent(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	for lexer.Lookahead() != 0 {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		lexer.MarkEnd()

		if !liquidValid(validSymbols, liquidTokPairedCommentContentLiq) {
			if lexer.Lookahead() != '{' {
				lexer.Advance(false)
				continue
			}
			lexer.Advance(false)
			if lexer.Lookahead() != '%' {
				continue
			}
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
			}
			for unicode.IsSpace(lexer.Lookahead()) {
				lexer.Advance(true)
			}
		}

		// Try "end"
		if lexer.Lookahead() == 'e' {
			if !liquidScanStr(lexer, "end") {
				lexer.Advance(false)
				continue
			}
		}

		isRaw := liquidScanStr(lexer, "raw")
		isComment := liquidScanStr(lexer, "comment")

		if isComment && liquidValid(validSymbols, liquidTokPairedCommentContent) {
			lexer.SetResultSymbol(liquidSymPairedCommentContent)
		} else if isComment && liquidValid(validSymbols, liquidTokPairedCommentContentLiq) {
			lexer.SetResultSymbol(liquidSymPairedCommentContentLiq)
			return true
		} else if isRaw && liquidValid(validSymbols, liquidTokRawContent) {
			lexer.SetResultSymbol(liquidSymRawContent)
		} else {
			lexer.Advance(false)
			continue
		}

		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '-' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() != '%' {
			continue
		}
		lexer.Advance(false)
		if lexer.Lookahead() == '}' {
			lexer.Advance(false)
			return true
		}
	}
	return false
}

func liquidValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
