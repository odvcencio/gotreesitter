//go:build !grammar_subset || grammar_subset_typescript

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the typescript grammar.
const (
	tsTokAutoSemicolon   = 0
	tsTokTemplateChars   = 1
	tsTokTernaryQmark    = 2
	tsTokHtmlComment     = 3
	tsTokLogicalOr       = 4
	tsTokEscapeSequence  = 5
	tsTokRegexPattern    = 6
	tsTokJsxText         = 7
	tsTokFuncSigAutoSemi = 8
	tsTokErrorRecovery   = 9
)

const (
	tsSymAutoSemicolon   gotreesitter.Symbol = 159
	tsSymTemplateChars   gotreesitter.Symbol = 160
	tsSymTernaryQmark    gotreesitter.Symbol = 161
	tsSymHtmlComment     gotreesitter.Symbol = 162
	tsSymJsxText         gotreesitter.Symbol = 163
	tsSymFuncSigAutoSemi gotreesitter.Symbol = 164
)

// TypeScriptExternalScanner handles automatic semicolons, template strings,
// JSX text, ternary question marks, and HTML comments for TypeScript.
type TypeScriptExternalScanner struct{}

func (TypeScriptExternalScanner) Create() any                           { return nil }
func (TypeScriptExternalScanner) Destroy(payload any)                   {}
func (TypeScriptExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (TypeScriptExternalScanner) Deserialize(payload any, buf []byte)   {}
func (TypeScriptExternalScanner) SupportsIncrementalReuse() bool        { return true }

func (TypeScriptExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if tsValid(validSymbols, tsTokTemplateChars) {
		if tsValid(validSymbols, tsTokAutoSemicolon) {
			return false
		}
		return tsScanTemplateChars(lexer)
	}

	preferAutoSemicolon := tsPreferAutoSemicolonOverJsxText(lexer, validSymbols)

	if tsValid(validSymbols, tsTokJsxText) && !preferAutoSemicolon {
		if tsScanJsxText(lexer) {
			return true
		}
	}

	if tsValid(validSymbols, tsTokAutoSemicolon) || tsValid(validSymbols, tsTokFuncSigAutoSemi) {
		scannedComment := false
		ret := tsScanAutoSemicolon(lexer, validSymbols, &scannedComment)
		if !ret && !scannedComment && tsValid(validSymbols, tsTokTernaryQmark) && lexer.Lookahead() == '?' {
			return tsScanTernaryQmark(lexer)
		}
		if !ret && !scannedComment && preferAutoSemicolon && tsValid(validSymbols, tsTokJsxText) {
			return tsScanJsxText(lexer)
		}
		return ret
	}

	if tsValid(validSymbols, tsTokJsxText) && preferAutoSemicolon {
		return tsScanJsxText(lexer)
	}

	if tsValid(validSymbols, tsTokTernaryQmark) {
		return tsScanTernaryQmark(lexer)
	}

	if tsValid(validSymbols, tsTokHtmlComment) &&
		!tsValid(validSymbols, tsTokLogicalOr) &&
		!tsValid(validSymbols, tsTokEscapeSequence) &&
		!tsValid(validSymbols, tsTokRegexPattern) {
		return tsScanClosingComment(lexer)
	}

	return false
}

func tsScanTemplateChars(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(tsSymTemplateChars)
	hasContent := false
	for {
		lexer.MarkEnd()
		switch lexer.Lookahead() {
		case '`':
			return hasContent
		case 0:
			return false
		case '$':
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				return hasContent
			}
			// The '$' was consumed and is not the start of a substitution, so it
			// counts as fragment content. C's scan_template_chars sets
			// has_content = true via the for-loop post-statement on every
			// iteration after the first, so the surviving '$' must mark content.
			hasContent = true
		case '\\':
			return hasContent
		default:
			lexer.Advance(false)
			hasContent = true
		}
	}
}

func tsScanAutoSemicolon(lexer *gotreesitter.ExternalLexer, validSymbols []bool, scannedComment *bool) bool {
	lexer.SetResultSymbol(tsSymAutoSemicolon)
	lexer.MarkEnd()

	for {
		ch := lexer.Lookahead()
		if ch == 0 {
			return true
		}
		if ch == '}' {
			lexer.Advance(true)
			for unicode.IsSpace(lexer.Lookahead()) {
				lexer.Advance(true)
			}
			switch lexer.Lookahead() {
			case ':':
				return tsValid(validSymbols, tsTokLogicalOr)
			default:
				if tsValid(validSymbols, tsTokJsxText) {
					return false
				}
			}
			switch lexer.Lookahead() {
			case '>':
				return false
			case '/':
				lexer.Advance(true)
				return lexer.Lookahead() != '>'
			case '<':
				lexer.Advance(true)
				return lexer.Lookahead() != '/'
			default:
				return true
			}
		}
		if !unicode.IsSpace(ch) {
			return false
		}
		if ch == '\n' {
			break
		}
		lexer.Advance(true)
	}

	lexer.Advance(true)

	if !tsScanWSAndComments(lexer, scannedComment) {
		return false
	}

	switch lexer.Lookahead() {
	case '`', ',', '.', ';', '*', '%', '>', '<', '=', '?', '^', '|', '&', '/', ':':
		return false
	case '{':
		if tsValid(validSymbols, tsTokFuncSigAutoSemi) {
			return false
		}
	case '(', '[':
		if tsValid(validSymbols, tsTokLogicalOr) {
			return false
		}
	case '+':
		lexer.Advance(true)
		return lexer.Lookahead() == '+'
	case '-':
		lexer.Advance(true)
		return lexer.Lookahead() == '-'
	case '!':
		lexer.Advance(true)
		return lexer.Lookahead() != '='
	case 'i':
		lexer.Advance(true)
		if lexer.Lookahead() != 'n' {
			return true
		}
		lexer.Advance(true)
		if !unicode.IsLetter(lexer.Lookahead()) {
			return false
		}
		stanceof := "stanceof"
		for i := 0; i < len(stanceof); i++ {
			if lexer.Lookahead() != rune(stanceof[i]) {
				return true
			}
			lexer.Advance(true)
		}
		if !unicode.IsLetter(lexer.Lookahead()) {
			return false
		}
	}

	return true
}

func tsScanWSAndComments(lexer *gotreesitter.ExternalLexer, scannedComment *bool) bool {
	for {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '/' {
			lexer.Advance(true)
			if lexer.Lookahead() == '/' {
				lexer.Advance(true)
				for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
					lexer.Advance(true)
				}
				*scannedComment = true
			} else if lexer.Lookahead() == '*' {
				lexer.Advance(true)
				for lexer.Lookahead() != 0 {
					if lexer.Lookahead() == '*' {
						lexer.Advance(true)
						if lexer.Lookahead() == '/' {
							lexer.Advance(true)
							break
						}
					} else {
						lexer.Advance(true)
					}
				}
			} else {
				return false
			}
		} else {
			return true
		}
	}
}

func tsScanTernaryQmark(lexer *gotreesitter.ExternalLexer) bool {
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if lexer.Lookahead() != '?' {
		return false
	}
	lexer.Advance(false)

	// Optional chaining
	if lexer.Lookahead() == '?' || lexer.Lookahead() == '.' {
		return false
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(tsSymTernaryQmark)

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(false)
	}

	if lexer.Lookahead() == ':' || lexer.Lookahead() == ')' || lexer.Lookahead() == ',' {
		return false
	}

	if lexer.Lookahead() == '.' {
		lexer.Advance(false)
		return unicode.IsDigit(lexer.Lookahead())
	}
	return true
}

func tsScanClosingComment(lexer *gotreesitter.ExternalLexer) bool {
	for unicode.IsSpace(lexer.Lookahead()) || lexer.Lookahead() == 0x2028 || lexer.Lookahead() == 0x2029 {
		lexer.Advance(true)
	}

	commentStart := "<!--"
	commentEnd := "-->"

	if lexer.Lookahead() == '<' {
		for i := 0; i < len(commentStart); i++ {
			if lexer.Lookahead() != rune(commentStart[i]) {
				return false
			}
			lexer.Advance(false)
		}
	} else if lexer.Lookahead() == '-' {
		for i := 0; i < len(commentEnd); i++ {
			if lexer.Lookahead() != rune(commentEnd[i]) {
				return false
			}
			lexer.Advance(false)
		}
	} else {
		return false
	}

	for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' &&
		lexer.Lookahead() != 0x2028 && lexer.Lookahead() != 0x2029 {
		lexer.Advance(false)
	}

	lexer.SetResultSymbol(tsSymHtmlComment)
	lexer.MarkEnd()
	return true
}

func tsScanJsxText(lexer *gotreesitter.ExternalLexer) bool {
	sawText := false
	atNewline := false
	onlyWhitespace := true

	for lexer.Lookahead() != 0 && lexer.Lookahead() != '<' && lexer.Lookahead() != '>' &&
		lexer.Lookahead() != '{' && lexer.Lookahead() != '}' && lexer.Lookahead() != '&' {
		if lexer.Lookahead() == '/' && onlyWhitespace {
			lexer.Advance(false)
			if lexer.Lookahead() == '>' {
				return false
			}
			sawText = true
			onlyWhitespace = false
			continue
		}
		if onlyWhitespace && (lexer.Lookahead() == '_' || unicode.IsLetter(lexer.Lookahead())) {
			for {
				lexer.Advance(false)
				ch := lexer.Lookahead()
				if ch == '_' || ch == '-' || ch == ':' || ch == '.' ||
					unicode.IsLetter(ch) || unicode.IsDigit(ch) {
					continue
				}
				break
			}
			for unicode.IsSpace(lexer.Lookahead()) {
				lexer.Advance(false)
			}
			if lexer.Lookahead() == '=' {
				return false
			}
			sawText = true
			onlyWhitespace = false
			continue
		}
		isWS := unicode.IsSpace(lexer.Lookahead())
		if lexer.Lookahead() == '\n' {
			atNewline = true
		} else {
			atNewline = atNewline && isWS
			if !atNewline {
				sawText = true
			}
		}
		if !isWS {
			onlyWhitespace = false
		}
		lexer.Advance(false)
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(tsSymJsxText)
	return sawText
}

func tsValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

func tsPreferAutoSemicolonOverJsxText(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !tsValid(validSymbols, tsTokAutoSemicolon) || !tsValid(validSymbols, tsTokJsxText) {
		return false
	}
	switch lexer.Lookahead() {
	case 0, '\n', '\r', 0x2028, 0x2029:
		return true
	default:
		return false
	}
}
