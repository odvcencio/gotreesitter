//go:build !grammar_subset || grammar_subset_tsx

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the tsx grammar.
const (
	tsxTokAutoSemicolon   = 0
	tsxTokTemplateChars   = 1
	tsxTokTernaryQmark    = 2
	tsxTokHtmlComment     = 3
	tsxTokLogicalOr       = 4
	tsxTokEscapeSequence  = 5
	tsxTokRegexPattern    = 6
	tsxTokJsxText         = 7
	tsxTokFuncSigAutoSemi = 8
	tsxTokErrorRecovery   = 9
	tsxTokenCount         = 10
)

// Concrete symbol IDs from the checked-in TSX grammar ExternalSymbols. These
// are DEFAULTS only: the fallback entries in tsxDefaultSymTable, used as-is
// if Scan is ever invoked on a scanner value that was never bound to a
// Language (should not happen in production; see ExternalScannerForLanguage).
// ExternalScannerForLanguage binds the real per-Language symbols
// POSITIONALLY (by external index, via bindExternalScannerSymbolNames), not
// by these absolute IDs, so a grammargen-regenerated tsx blob -- which emits
// the same 10 externals in the same order but under different absolute
// Symbol numbering once the automaton grows or shrinks -- still lexes
// correctly instead of silently mistyping every external scan result. See
// bindExternalScannerSymbolNames for why positional (not absolute-ID or
// by-name) binding is required; the same pattern is used by the
// javascript/typescript/kotlin/python/swift/dart/rust/hcl scanners.
const (
	tsxSymAutoSemicolon   gotreesitter.Symbol = 166
	tsxSymTemplateChars   gotreesitter.Symbol = 167
	tsxSymTernaryQmark    gotreesitter.Symbol = 168
	tsxSymHtmlComment     gotreesitter.Symbol = 169
	tsxSymLogicalOr       gotreesitter.Symbol = 81
	tsxSymEscapeSequence  gotreesitter.Symbol = 109
	tsxSymRegexPattern    gotreesitter.Symbol = 114
	tsxSymJsxText         gotreesitter.Symbol = 170
	tsxSymFuncSigAutoSemi gotreesitter.Symbol = 171
	tsxSymErrorRecovery   gotreesitter.Symbol = 172
)

// tsxDefaultSymTable mirrors the tsxTok* index order above.
var tsxDefaultSymTable = [tsxTokenCount]gotreesitter.Symbol{
	tsxTokAutoSemicolon:   tsxSymAutoSemicolon,
	tsxTokTemplateChars:   tsxSymTemplateChars,
	tsxTokTernaryQmark:    tsxSymTernaryQmark,
	tsxTokHtmlComment:     tsxSymHtmlComment,
	tsxTokLogicalOr:       tsxSymLogicalOr,
	tsxTokEscapeSequence:  tsxSymEscapeSequence,
	tsxTokRegexPattern:    tsxSymRegexPattern,
	tsxTokJsxText:         tsxSymJsxText,
	tsxTokFuncSigAutoSemi: tsxSymFuncSigAutoSemi,
	tsxTokErrorRecovery:   tsxSymErrorRecovery,
}

// tsxExternalSymbolNames lists the tsx grammar's external tokens in
// declaration order (matching the tsxTok* indexes above and the language's
// ExternalSymbols order). Used by ExternalScannerForLanguage to bind this
// scanner's token slots to a Language's external symbols positionally, the
// same pattern used by the javascript/typescript/kotlin/python/swift/dart/
// rust/hcl scanners. See bindExternalScannerSymbolNames for why positional
// (not absolute-ID or by-name) binding is required.
var tsxExternalSymbolNames = []string{
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

// TsxExternalScanner handles automatic semicolons, template strings,
// JSX text, ternary question marks, and HTML comments for TSX.
type TsxExternalScanner struct {
	symbols         [tsxTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token slots to lang's
// external symbols positionally (external index i -> scanner token i), so
// Scan resolves result symbols through the bound table instead of the
// hardcoded absolute IDs above. Required whenever the attached Language did
// not come from the exact blob those absolute IDs were pinned against (e.g.
// a grammargen regeneration that shifts the automaton's overall symbol
// numbering) -- see bindExternalScannerSymbolNames.
func (TsxExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := TsxExternalScanner{symbols: tsxDefaultSymTable}
	s.externalToToken = bindExternalScannerSymbolNames(lang, tsxExternalSymbolNames, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (TsxExternalScanner) Create() any                           { return nil }
func (TsxExternalScanner) Destroy(payload any)                   {}
func (TsxExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (TsxExternalScanner) Deserialize(payload any, buf []byte)   {}
func (TsxExternalScanner) SupportsIncrementalReuse() bool        { return true }

// ExternalScannerIsStateless proves that TSX scanning depends only on
// lookahead and the parser-provided valid-symbol set.
func (TsxExternalScanner) ExternalScannerIsStateless() bool { return true }

// symbolTable returns the per-Language-bound result-symbol table, falling
// back to the pinned defaults when Scan is invoked on an unbound scanner
// value (s.symbols is still its zero value).
func (s TsxExternalScanner) symbolTable() *[tsxTokenCount]gotreesitter.Symbol {
	if s.symbols == ([tsxTokenCount]gotreesitter.Symbol{}) {
		return &tsxDefaultSymTable
	}
	return &s.symbols
}

// remapValidSymbols translates a Language-external-indexed validSymbols
// slice into scanner-token-indexed space via s.externalToToken. When the
// Language's external count and order agree with tsxExternalSymbolNames (the
// common case), externalToToken is the identity permutation and this is a
// copy; it only diverges when a future grammar version adds, removes, or
// reorders an external token relative to tsxExternalSymbolNames.
func (s TsxExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[tsxTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [tsxTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < tsxTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (s TsxExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var semanticValid [tsxTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	if tsxValid(validSymbols, tsxTokTemplateChars) {
		if tsxValid(validSymbols, tsxTokAutoSemicolon) {
			return false
		}
		return tsxScanTemplateChars(lexer, symbols)
	}

	preferAutoSemicolon := tsxPreferAutoSemicolonOverJsxText(lexer, validSymbols)

	if tsxValid(validSymbols, tsxTokJsxText) && !preferAutoSemicolon {
		if tsxScanJsxText(lexer, symbols) {
			return true
		}
	}

	if tsxValid(validSymbols, tsxTokAutoSemicolon) || tsxValid(validSymbols, tsxTokFuncSigAutoSemi) {
		scannedComment := false
		ret := tsxScanAutoSemicolon(lexer, validSymbols, symbols, &scannedComment)
		if !ret && !scannedComment && tsxValid(validSymbols, tsxTokTernaryQmark) && lexer.Lookahead() == '?' {
			return tsxScanTernaryQmark(lexer, symbols)
		}
		if !ret && !scannedComment && preferAutoSemicolon && tsxValid(validSymbols, tsxTokJsxText) {
			return tsxScanJsxText(lexer, symbols)
		}
		return ret
	}

	if tsxValid(validSymbols, tsxTokJsxText) && preferAutoSemicolon {
		return tsxScanJsxText(lexer, symbols)
	}

	if tsxValid(validSymbols, tsxTokTernaryQmark) {
		return tsxScanTernaryQmark(lexer, symbols)
	}

	if tsxValid(validSymbols, tsxTokHtmlComment) &&
		!tsxValid(validSymbols, tsxTokLogicalOr) &&
		!tsxValid(validSymbols, tsxTokEscapeSequence) &&
		!tsxValid(validSymbols, tsxTokRegexPattern) {
		return tsxScanClosingComment(lexer, symbols)
	}

	return false
}

func tsxScanTemplateChars(lexer *gotreesitter.ExternalLexer, symbols *[tsxTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[tsxTokTemplateChars])
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

func tsxScanAutoSemicolon(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[tsxTokenCount]gotreesitter.Symbol, scannedComment *bool) bool {
	lexer.SetResultSymbol(symbols[tsxTokAutoSemicolon])
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
				return tsxValid(validSymbols, tsxTokLogicalOr)
			default:
				if tsxValid(validSymbols, tsxTokJsxText) {
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

	if !tsxScanWSAndComments(lexer, scannedComment) {
		return false
	}

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

func tsxScanWSAndComments(lexer *gotreesitter.ExternalLexer, scannedComment *bool) bool {
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

func tsxScanTernaryQmark(lexer *gotreesitter.ExternalLexer, symbols *[tsxTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[tsxTokTernaryQmark])

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

func tsxScanClosingComment(lexer *gotreesitter.ExternalLexer, symbols *[tsxTokenCount]gotreesitter.Symbol) bool {
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

	lexer.SetResultSymbol(symbols[tsxTokHtmlComment])
	lexer.MarkEnd()
	return true
}

func tsxScanJsxText(lexer *gotreesitter.ExternalLexer, symbols *[tsxTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[tsxTokJsxText])
	return sawText
}

func tsxValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

func tsxPreferAutoSemicolonOverJsxText(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !tsxValid(validSymbols, tsxTokAutoSemicolon) || !tsxValid(validSymbols, tsxTokJsxText) {
		return false
	}
	switch lexer.Lookahead() {
	case 0, '\n', '\r', 0x2028, 0x2029:
		return true
	default:
		return false
	}
}
