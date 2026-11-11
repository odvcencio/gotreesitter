//go:build !grammar_subset || grammar_subset_tsx

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// The C-faithful mode of the tsx scanner: the pinned scanner's code
// (tree-sitter-tsx common/scanner.h) with its character classes (iswspace,
// iswdigit and iswalpha in the C locale: ASCII only, no U+2028 as white space
// except where C compares it). Only the core turns the mode on.

func tsxWSAndCommentsC(lexer *gotreesitter.ExternalLexer, scannedComment *bool) bool {
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

// tsxAutoSemicolonC is the semicolon scan of the C-faithful mode. It is the default
// scan (tsScanAutoSemicolon) with the two things the pinned scanner does differently
// at "}" and in character classes (common/scanner.h:90-101, :105): at "}" it skips
// white space and returns the || flag before ":" and true otherwise, with none of the
// JSX rules; white space is iswspace of the C locale. The branches after a newline
// are those of the scanner the oracle compiles, which is the pinned
// tree-sitter-typescript scanner.h (commit 75b3874e) plus the patch
// grammars/patches/tree-sitter-typescript-import-type.patch that the oracle builder
// applies before compiling: after a newline "<" returns the function-signature
// flag instead of refusing, and "in" is read only when the || flag is valid.
func tsxAutoSemicolonC(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[tsxTokenCount]gotreesitter.Symbol, scannedComment *bool) bool {
	lexer.SetResultSymbol(symbols[tsxTokAutoSemicolon])
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
				return tsxValid(validSymbols, tsxTokLogicalOr)
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
	if !tsxWSAndCommentsC(lexer, scannedComment) {
		return false
	}
	return tsxAfterNewlineC(lexer, validSymbols)
}

// tsxAfterNewlineC is the part of the default scan that follows the newline and the
// white space and comments after it, with iswalpha of the C locale.
func tsxAfterNewlineC(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	switch lexer.Lookahead() {
	case '`', ',', '.', ';', '*', '%', '>', '=', '?', '^', '|', '&', '/', ':':
		return false
	case '<':
		return tsxValid(validSymbols, tsxTokFuncSigAutoSemi)
	case '{':
		if tsxValid(validSymbols, tsxTokFuncSigAutoSemi) {
			return false
		}
	case '(', '[':
		if tsxValid(validSymbols, tsxTokLogicalOr) {
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
		if !tsxValid(validSymbols, tsxTokLogicalOr) {
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

// tsxTernaryQmarkC is scan_ternary_qmark (scanner.h:187-228).
func tsxTernaryQmarkC(lexer *gotreesitter.ExternalLexer, symbols *[tsxTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[tsxTokTernaryQmark])
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

// tsxClosingCommentC is scan_closing_comment (scanner.h:230-269).
func tsxClosingCommentC(lexer *gotreesitter.ExternalLexer, symbols *[tsxTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[tsxTokHtmlComment])
	lexer.MarkEnd()
	return true
}

// tsxJsxTextC is scan_jsx_text (scanner.h:271-310).
func tsxJsxTextC(lexer *gotreesitter.ExternalLexer, symbols *[tsxTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[tsxTokJsxText])
	return sawText
}

// tsScanC is external_scanner_scan (scanner.h:312-352).
func (s TsxExternalScanner) scanC(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[tsxTokenCount]gotreesitter.Symbol) bool {
	if tsxValid(validSymbols, tsxTokTemplateChars) {
		if tsxValid(validSymbols, tsxTokAutoSemicolon) {
			return false
		}
		return tsxScanTemplateChars(lexer, symbols)
	}
	if tsxValid(validSymbols, tsxTokJsxText) && tsxJsxTextC(lexer, symbols) {
		return true
	}
	if tsxValid(validSymbols, tsxTokAutoSemicolon) || tsxValid(validSymbols, tsxTokFuncSigAutoSemi) {
		scannedComment := false
		ret := tsxAutoSemicolonC(lexer, validSymbols, symbols, &scannedComment)
		if !ret && !scannedComment && tsxValid(validSymbols, tsxTokTernaryQmark) && lexer.Lookahead() == '?' {
			return tsxTernaryQmarkC(lexer, symbols)
		}
		return ret
	}
	if tsxValid(validSymbols, tsxTokTernaryQmark) {
		return tsxTernaryQmarkC(lexer, symbols)
	}
	if tsxValid(validSymbols, tsxTokHtmlComment) && !tsxValid(validSymbols, tsxTokLogicalOr) &&
		!tsxValid(validSymbols, tsxTokEscapeSequence) && !tsxValid(validSymbols, tsxTokRegexPattern) {
		return tsxClosingCommentC(lexer, symbols)
	}
	return false
}

func init() {
	scanmode.RegisterScanner(TsxExternalScanner{}, (*scanmode.Flag)(nil), true)
}
