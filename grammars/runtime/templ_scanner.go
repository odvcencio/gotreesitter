//go:build !grammar_subset || grammar_subset_templ

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the templ grammar.
const (
	templTokCssPropertyValue = 0
	templTokScriptBlockText  = 1
	templTokSwitchElemText   = 2
	templTokElemText         = 3
)

const (
	templSymCssPropertyValue gotreesitter.Symbol = 127
	templSymScriptBlockText  gotreesitter.Symbol = 128
	templSymSwitchElemText   gotreesitter.Symbol = 129
	templSymElemText         gotreesitter.Symbol = 130
)

var templStatementKeywords = []string{
	"//", "/*",
	"if ", "else ", "for ", "switch ",
}
var templSwitchKeywords = []string{
	"case ", "default:",
}

// templState tracks whether we've seen an @ symbol for component expressions.
type templState struct {
	sawAtSymbol bool
}

// TemplExternalScanner handles CSS property values, script blocks, and element text for templ.
type TemplExternalScanner struct{}

func (TemplExternalScanner) Create() any         { return &templState{} }
func (TemplExternalScanner) Destroy(payload any) {}

func (TemplExternalScanner) Serialize(payload any, buf []byte) int {
	return 0 // minimal state
}

func (TemplExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*templState)
	s.sawAtSymbol = false
}

func (TemplExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*templState)

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if templValid(validSymbols, templTokCssPropertyValue) {
		return templScanCssPropertyValue(lexer)
	}

	if templValid(validSymbols, templTokScriptBlockText) {
		return templScanScriptBlockText(lexer)
	}

	if templValid(validSymbols, templTokSwitchElemText) {
		return templScanElementText(s, lexer, templSymSwitchElemText, true)
	}

	if templValid(validSymbols, templTokElemText) {
		return templScanElementText(s, lexer, templSymElemText, false)
	}

	return false
}

func templScanCssPropertyValue(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() == '{' {
		return false
	}
	lexer.SetResultSymbol(templSymCssPropertyValue)
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == ';' {
			return true
		}
		lexer.Advance(false)
	}
	return false
}

func templScanScriptBlockText(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(templSymScriptBlockText)
	lexer.MarkEnd()

	if lexer.Lookahead() == 0 {
		return false
	}

	hasMarked := false
	braceCount := 1

	for lexer.Lookahead() != 0 {
		switch lexer.Lookahead() {
		case '{':
			braceCount++
		case '}':
			braceCount--
			if braceCount == 0 {
				return hasMarked
			}
		}
		lexer.Advance(false)
		lexer.MarkEnd()
		hasMarked = true
	}

	return hasMarked
}

func templScanElementText(s *templState, lexer *gotreesitter.ExternalLexer, sym gotreesitter.Symbol, inSwitch bool) bool {
	lexer.SetResultSymbol(sym)
	lexer.MarkEnd()

	if lexer.Lookahead() == 0 {
		return false
	}

	// Buffer for keyword lookahead
	var buf []rune
	count := 0

	// Check statement keywords
	keywords := templStatementKeywords
	if inSwitch {
		keywords = append(keywords, templSwitchKeywords...)
	}

	for _, kw := range keywords {
		if templMatchesKeyword(lexer, &buf, kw) {
			return false
		}
	}

	// Check for @ symbol (component expression)
	if templMatchesKeyword(lexer, &buf, "@") {
		s.sawAtSymbol = true
		return false
	}

	// Check buffer for terminators
	for _, ch := range buf {
		if templIsElemTextTerminator(ch) {
			return false
		}
		if s.sawAtSymbol && templIsImportExprTerminator(ch) {
			return false
		}
	}

	count += len(buf)

	// Continue scanning
	for lexer.Lookahead() != 0 {
		if templIsElemTextTerminator(lexer.Lookahead()) {
			break
		}
		if s.sawAtSymbol && templIsImportExprTerminator(lexer.Lookahead()) {
			break
		}
		lexer.Advance(false)
		lexer.MarkEnd()
		count++
	}

	if count > 0 {
		lexer.MarkEnd()
		s.sawAtSymbol = false
		return true
	}
	return false
}

func templMatchesKeyword(lexer *gotreesitter.ExternalLexer, buf *[]rune, kw string) bool {
	runes := []rune(kw)
	for i, r := range runes {
		var ch rune
		if i < len(*buf) {
			ch = (*buf)[i]
		} else {
			if lexer.Lookahead() == 0 {
				return false
			}
			ch = lexer.Lookahead()
			*buf = append(*buf, ch)
			lexer.Advance(false)
		}
		if ch != r {
			return false
		}
	}
	return true
}

func templIsElemTextTerminator(ch rune) bool {
	return ch == '<' || ch == '{' || ch == '}' || ch == '\n'
}

func templIsImportExprTerminator(ch rune) bool {
	return ch == '.' || ch == '(' || ch == ')' || ch == '['
}

func templValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
