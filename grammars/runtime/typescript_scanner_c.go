//go:build !grammar_subset || grammar_subset_typescript

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// The C-faithful mode of the typescript scanner: the pinned scanner's code
// (tree-sitter-typescript common/scanner.h) with its character classes (iswspace,
// iswdigit and iswalpha in the C locale: ASCII only, no U+2028 as white space
// except where C compares it). Only the core turns the mode on.

func tsWSAndCommentsC(lexer *gotreesitter.ExternalLexer, scannedComment *bool) bool {
	for { // scanner.h:44-77
		for cIsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() != '/' {
			return true
		}
		lexer.Advance(true)
		switch lexer.Lookahead() {
		case '/':
			lexer.Advance(true)
			for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
				lexer.Advance(true)
			}
			*scannedComment = true
		case '*':
			lexer.Advance(true)
		block:
			for lexer.Lookahead() != 0 {
				if lexer.Lookahead() == '*' {
					lexer.Advance(true)
					if lexer.Lookahead() == '/' {
						lexer.Advance(true)
						break block
					}
				} else {
					lexer.Advance(true)
				}
			}
		default:
			return false
		}
	}
}

// tsAutoSemicolonC is the semicolon scan of the C-faithful mode. It is the default
// scan (tsScanAutoSemicolon) with the two things the pinned scanner does differently
// at "}" and in character classes (common/scanner.h:90-101, :105): at "}" it skips
// white space and returns the || flag before ":" and true otherwise, with none of the
// JSX rules; white space is iswspace of the C locale. The branches after a newline
// are those of the scanner the oracle compiles, which is the pinned
// tree-sitter-typescript scanner.h (commit 75b3874e) plus the patch
// grammars/patches/tree-sitter-typescript-import-type.patch that the oracle builder
// applies before compiling: after a newline "<" returns the function-signature
// flag instead of refusing, and "in" is read only when the || flag is valid.
func tsAutoSemicolonC(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[tsTokenCount]gotreesitter.Symbol, scannedComment *bool) bool {
	lexer.SetResultSymbol(symbols[tsTokAutoSemicolon])
	lexer.MarkEnd()
	for {
		ch := lexer.Lookahead()
		if ch == 0 {
			return true
		}
		if ch == '}' {
			for { // do { skip } while (iswspace)
				lexer.Advance(true)
				if !cIsSpace(lexer.Lookahead()) {
					break
				}
			}
			if lexer.Lookahead() == ':' {
				return tsValid(validSymbols, tsTokLogicalOr)
			}
			return true
		}
		if !cIsSpace(ch) {
			return false
		}
		if ch == '\n' {
			break
		}
		lexer.Advance(true)
	}
	lexer.Advance(true)
	if !tsWSAndCommentsC(lexer, scannedComment) {
		return false
	}
	return tsAfterNewlineC(lexer, validSymbols)
}

// tsAfterNewlineC is the part of the default scan that follows the newline and the
// white space and comments after it, with iswalpha of the C locale.
func tsAfterNewlineC(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	switch lexer.Lookahead() {
	case '`', ',', '.', ';', '*', '%', '>', '=', '?', '^', '|', '&', '/', ':':
		return false
	case '<':
		return tsValid(validSymbols, tsTokFuncSigAutoSemi)
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
		if !tsValid(validSymbols, tsTokLogicalOr) {
			return true
		}
		lexer.Advance(true)
		if lexer.Lookahead() != 'n' {
			return true
		}
		lexer.Advance(true)
		if !cIsAlpha(lexer.Lookahead()) {
			return false
		}
		stanceof := "stanceof"
		for i := 0; i < len(stanceof); i++ {
			if lexer.Lookahead() != rune(stanceof[i]) {
				return true
			}
			lexer.Advance(true)
		}
		if !cIsAlpha(lexer.Lookahead()) {
			return false
		}
	}

	return true
}

// tsTernaryQmarkC is scan_ternary_qmark (scanner.h:187-228).
func tsTernaryQmarkC(lexer *gotreesitter.ExternalLexer, symbols *[tsTokenCount]gotreesitter.Symbol) bool {
	for cIsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}
	if lexer.Lookahead() != '?' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() == '?' || lexer.Lookahead() == '.' {
		return false
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(symbols[tsTokTernaryQmark])
	for cIsSpace(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	if lexer.Lookahead() == ':' || lexer.Lookahead() == ')' || lexer.Lookahead() == ',' {
		return false
	}
	if lexer.Lookahead() == '.' {
		lexer.Advance(false)
		return cIsDigit(lexer.Lookahead())
	}
	return true
}

// tsClosingCommentC is scan_closing_comment (scanner.h:230-269).
func tsClosingCommentC(lexer *gotreesitter.ExternalLexer, symbols *[tsTokenCount]gotreesitter.Symbol) bool {
	for cIsSpace(lexer.Lookahead()) || lexer.Lookahead() == 0x2028 || lexer.Lookahead() == 0x2029 {
		lexer.Advance(true)
	}
	const commentStart, commentEnd = "<!--", "-->"
	switch lexer.Lookahead() {
	case '<':
		for i := 0; i < 4; i++ {
			if lexer.Lookahead() != rune(commentStart[i]) {
				return false
			}
			lexer.Advance(false)
		}
	case '-':
		for i := 0; i < 3; i++ {
			if lexer.Lookahead() != rune(commentEnd[i]) {
				return false
			}
			lexer.Advance(false)
		}
	default:
		return false
	}
	for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' && lexer.Lookahead() != 0x2028 && lexer.Lookahead() != 0x2029 {
		lexer.Advance(false)
	}
	lexer.SetResultSymbol(symbols[tsTokHtmlComment])
	lexer.MarkEnd()
	return true
}

// tsJsxTextC is scan_jsx_text (scanner.h:271-310).
func tsJsxTextC(lexer *gotreesitter.ExternalLexer, symbols *[tsTokenCount]gotreesitter.Symbol) bool {
	sawText, atNewline := false, false
	for lexer.Lookahead() != 0 && lexer.Lookahead() != '<' && lexer.Lookahead() != '>' && lexer.Lookahead() != '{' &&
		lexer.Lookahead() != '}' && lexer.Lookahead() != '&' {
		isWspace := cIsSpace(lexer.Lookahead())
		if lexer.Lookahead() == '\n' {
			atNewline = true
		} else {
			atNewline = atNewline && isWspace
			if !atNewline {
				sawText = true
			}
		}
		lexer.Advance(false)
	}
	lexer.SetResultSymbol(symbols[tsTokJsxText])
	return sawText
}

// tsScanC is external_scanner_scan (scanner.h:312-352).
func (s TypeScriptExternalScanner) scanC(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[tsTokenCount]gotreesitter.Symbol) bool {
	if tsValid(validSymbols, tsTokTemplateChars) {
		if tsValid(validSymbols, tsTokAutoSemicolon) {
			return false
		}
		return tsScanTemplateChars(lexer, symbols)
	}
	if tsValid(validSymbols, tsTokJsxText) && tsJsxTextC(lexer, symbols) {
		return true
	}
	if tsValid(validSymbols, tsTokAutoSemicolon) || tsValid(validSymbols, tsTokFuncSigAutoSemi) {
		scannedComment := false
		ret := tsAutoSemicolonC(lexer, validSymbols, symbols, &scannedComment)
		if !ret && !scannedComment && tsValid(validSymbols, tsTokTernaryQmark) && lexer.Lookahead() == '?' {
			return tsTernaryQmarkC(lexer, symbols)
		}
		return ret
	}
	if tsValid(validSymbols, tsTokTernaryQmark) {
		return tsTernaryQmarkC(lexer, symbols)
	}
	if tsValid(validSymbols, tsTokHtmlComment) && !tsValid(validSymbols, tsTokLogicalOr) &&
		!tsValid(validSymbols, tsTokEscapeSequence) && !tsValid(validSymbols, tsTokRegexPattern) {
		return tsClosingCommentC(lexer, symbols)
	}
	return false
}

func init() {
	scanmode.RegisterScanner(TypeScriptExternalScanner{}, (*scanmode.Flag)(nil), true)
}
