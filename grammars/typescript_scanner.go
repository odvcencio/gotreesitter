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
	tsTokenCount         = 10
)

// Concrete symbol IDs from the checked-in TypeScript grammar ExternalSymbols.
// These are DEFAULTS only: the fallback entries in tsDefaultSymTable, used
// as-is if Scan is ever invoked on a scanner value that was never bound to a
// Language (should not happen in production; see ExternalScannerForLanguage).
// ExternalScannerForLanguage binds the real per-Language symbols
// POSITIONALLY (by external index, via bindExternalScannerSymbolNames), not
// by these absolute IDs, so a grammargen-regenerated typescript blob -- which
// emits the same 10 externals in the same order but under different absolute
// Symbol numbering once the automaton grows or shrinks -- still lexes
// correctly instead of silently mistyping every external scan result. See
// bindExternalScannerSymbolNames for why positional (not absolute-ID or
// by-name) binding is required; the same pattern is used by the
// javascript/kotlin/python/swift/dart/rust/hcl scanners.
const (
	tsSymAutoSemicolon   gotreesitter.Symbol = 160
	tsSymTemplateChars   gotreesitter.Symbol = 161
	tsSymTernaryQmark    gotreesitter.Symbol = 162
	tsSymHtmlComment     gotreesitter.Symbol = 163
	tsSymLogicalOr       gotreesitter.Symbol = 72
	tsSymEscapeSequence  gotreesitter.Symbol = 103
	tsSymRegexPattern    gotreesitter.Symbol = 108
	tsSymJsxText         gotreesitter.Symbol = 164
	tsSymFuncSigAutoSemi gotreesitter.Symbol = 165
	tsSymErrorRecovery   gotreesitter.Symbol = 166
)

// tsDefaultSymTable mirrors the tsTok* index order above.
var tsDefaultSymTable = [tsTokenCount]gotreesitter.Symbol{
	tsTokAutoSemicolon:   tsSymAutoSemicolon,
	tsTokTemplateChars:   tsSymTemplateChars,
	tsTokTernaryQmark:    tsSymTernaryQmark,
	tsTokHtmlComment:     tsSymHtmlComment,
	tsTokLogicalOr:       tsSymLogicalOr,
	tsTokEscapeSequence:  tsSymEscapeSequence,
	tsTokRegexPattern:    tsSymRegexPattern,
	tsTokJsxText:         tsSymJsxText,
	tsTokFuncSigAutoSemi: tsSymFuncSigAutoSemi,
	tsTokErrorRecovery:   tsSymErrorRecovery,
}

// tsExternalSymbolNames lists the typescript grammar's external tokens in
// declaration order (matching the tsTok* indexes above and the language's
// ExternalSymbols order). Used by ExternalScannerForLanguage to bind this
// scanner's token slots to a Language's external symbols positionally, the
// same pattern used by the javascript/kotlin/python/swift/dart/rust/hcl
// scanners. See bindExternalScannerSymbolNames for why positional (not
// absolute-ID or by-name) binding is required.
var tsExternalSymbolNames = []string{
	"_automatic_semicolon",
	"_template_chars",
	"_ternary_qmark",
	"html_comment",
	"||",
	"escape_sequence",
	"regex_pattern",
	"jsx_text",
	"_function_signature_automatic_semicolon",
	"__error_recovery",
}

// TypeScriptExternalScanner handles automatic semicolons, template strings,
// JSX text, ternary question marks, and HTML comments for TypeScript.
type TypeScriptExternalScanner struct {
	symbols         [tsTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token slots to lang's
// external symbols positionally (external index i -> scanner token i), so
// Scan resolves result symbols through the bound table instead of the
// hardcoded absolute IDs above. Required whenever the attached Language did
// not come from the exact blob those absolute IDs were pinned against (e.g.
// a grammargen regeneration that shifts the automaton's overall symbol
// numbering) -- see bindExternalScannerSymbolNames.
func (TypeScriptExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := TypeScriptExternalScanner{symbols: tsDefaultSymTable}
	s.externalToToken = bindExternalScannerSymbolNames(lang, tsExternalSymbolNames, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (TypeScriptExternalScanner) Create() any                           { return nil }
func (TypeScriptExternalScanner) Destroy(payload any)                   {}
func (TypeScriptExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (TypeScriptExternalScanner) Deserialize(payload any, buf []byte)   {}
func (TypeScriptExternalScanner) SupportsIncrementalReuse() bool        { return true }

// All ASCII digits follow identical branches, including failed speculative scans.
// Grammar bindings change symbols and masks, not character classification.
func (TypeScriptExternalScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= '0' && b <= '9' {
		return 1
	}
	return 0
}

// symbolTable returns the per-Language-bound result-symbol table, falling
// back to the pinned defaults when Scan is invoked on an unbound scanner
// value (s.symbols is still its zero value).
func (s TypeScriptExternalScanner) symbolTable() *[tsTokenCount]gotreesitter.Symbol {
	if s.symbols == ([tsTokenCount]gotreesitter.Symbol{}) {
		return &tsDefaultSymTable
	}
	return &s.symbols
}

// remapValidSymbols translates a Language-external-indexed validSymbols
// slice into scanner-token-indexed space via s.externalToToken. When the
// Language's external count and order agree with tsExternalSymbolNames (the
// common case), externalToToken is the identity permutation and this is a
// copy; it only diverges when a future grammar version adds, removes, or
// reorders an external token relative to tsExternalSymbolNames.
func (s TypeScriptExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[tsTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [tsTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < tsTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (s TypeScriptExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var semanticValid [tsTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	if tsValid(validSymbols, tsTokTemplateChars) {
		if tsValid(validSymbols, tsTokAutoSemicolon) {
			return false
		}
		return tsScanTemplateChars(lexer, symbols)
	}

	preferAutoSemicolon := tsPreferAutoSemicolonOverJsxText(lexer, validSymbols)

	if tsValid(validSymbols, tsTokJsxText) && !preferAutoSemicolon {
		if tsScanJsxText(lexer, symbols) {
			return true
		}
	}

	if tsValid(validSymbols, tsTokAutoSemicolon) || tsValid(validSymbols, tsTokFuncSigAutoSemi) {
		scannedComment := false
		ret := tsScanAutoSemicolon(lexer, validSymbols, symbols, &scannedComment)
		if !ret && !scannedComment && tsValid(validSymbols, tsTokTernaryQmark) && lexer.Lookahead() == '?' {
			return tsScanTernaryQmark(lexer, symbols)
		}
		if !ret && !scannedComment && preferAutoSemicolon && tsValid(validSymbols, tsTokJsxText) {
			return tsScanJsxText(lexer, symbols)
		}
		return ret
	}

	if tsValid(validSymbols, tsTokJsxText) && preferAutoSemicolon {
		return tsScanJsxText(lexer, symbols)
	}

	if tsValid(validSymbols, tsTokTernaryQmark) {
		return tsScanTernaryQmark(lexer, symbols)
	}

	if tsValid(validSymbols, tsTokHtmlComment) &&
		!tsValid(validSymbols, tsTokLogicalOr) &&
		!tsValid(validSymbols, tsTokEscapeSequence) &&
		!tsValid(validSymbols, tsTokRegexPattern) {
		return tsScanClosingComment(lexer, symbols)
	}

	return false
}

func tsScanTemplateChars(lexer *gotreesitter.ExternalLexer, symbols *[tsTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[tsTokTemplateChars])
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

func tsScanAutoSemicolon(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[tsTokenCount]gotreesitter.Symbol, scannedComment *bool) bool {
	lexer.SetResultSymbol(symbols[tsTokAutoSemicolon])
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

func tsScanTernaryQmark(lexer *gotreesitter.ExternalLexer, symbols *[tsTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[tsTokTernaryQmark])

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

func tsScanClosingComment(lexer *gotreesitter.ExternalLexer, symbols *[tsTokenCount]gotreesitter.Symbol) bool {
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

	lexer.SetResultSymbol(symbols[tsTokHtmlComment])
	lexer.MarkEnd()
	return true
}

func tsScanJsxText(lexer *gotreesitter.ExternalLexer, symbols *[tsTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[tsTokJsxText])
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
