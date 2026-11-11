//go:build !grammar_subset || grammar_subset_typescript

package grammarruntime

// Code generated from the pre-change scanner source as a test oracle; DO NOT EDIT.

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"unicode"
)

// External token indexes for the typescript grammar.
const (
	oracleTsTokAutoSemicolon   = 0
	oracleTsTokTemplateChars   = 1
	oracleTsTokTernaryQmark    = 2
	oracleTsTokHtmlComment     = 3
	oracleTsTokLogicalOr       = 4
	oracleTsTokEscapeSequence  = 5
	oracleTsTokRegexPattern    = 6
	oracleTsTokJsxText         = 7
	oracleTsTokFuncSigAutoSemi = 8
	oracleTsTokErrorRecovery   = 9
	oracleTsTokenCount         = 10
)

// Concrete symbol IDs from the checked-in TypeScript grammar ExternalSymbols.
// These are DEFAULTS only: the fallback entries in oracleTsDefaultSymTable, used
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
	oracleTsSymAutoSemicolon   gotreesitter.Symbol = 160
	oracleTsSymTemplateChars   gotreesitter.Symbol = 161
	oracleTsSymTernaryQmark    gotreesitter.Symbol = 162
	oracleTsSymHtmlComment     gotreesitter.Symbol = 163
	oracleTsSymLogicalOr       gotreesitter.Symbol = 72
	oracleTsSymEscapeSequence  gotreesitter.Symbol = 103
	oracleTsSymRegexPattern    gotreesitter.Symbol = 108
	oracleTsSymJsxText         gotreesitter.Symbol = 164
	oracleTsSymFuncSigAutoSemi gotreesitter.Symbol = 165
	oracleTsSymErrorRecovery   gotreesitter.Symbol = 166
)

// oracleTsDefaultSymTable mirrors the tsTok* index order above.
var oracleTsDefaultSymTable = [oracleTsTokenCount]gotreesitter.Symbol{
	oracleTsTokAutoSemicolon:   oracleTsSymAutoSemicolon,
	oracleTsTokTemplateChars:   oracleTsSymTemplateChars,
	oracleTsTokTernaryQmark:    oracleTsSymTernaryQmark,
	oracleTsTokHtmlComment:     oracleTsSymHtmlComment,
	oracleTsTokLogicalOr:       oracleTsSymLogicalOr,
	oracleTsTokEscapeSequence:  oracleTsSymEscapeSequence,
	oracleTsTokRegexPattern:    oracleTsSymRegexPattern,
	oracleTsTokJsxText:         oracleTsSymJsxText,
	oracleTsTokFuncSigAutoSemi: oracleTsSymFuncSigAutoSemi,
	oracleTsTokErrorRecovery:   oracleTsSymErrorRecovery,
}

// oracleTsExternalScannerSpec records the upstream scanner-source contract for this
// port. Externals lists the typescript grammar's external tokens in
// declaration order (matching the tsTok* indexes above and the language's
// ExternalSymbols order). ExternalScannerForLanguage binds this scanner's
// token slots to a Language's external symbols positionally through it, the
// same pattern used by the javascript/kotlin/python/swift/dart/rust/hcl
// scanners. See bindExternalScannerSymbolNames for why positional (not
// absolute-ID or by-name) binding is required. The per-grammar scanner.c is
// a thin shim; common/scanner.h holds the scanner logic.
var oracleTsExternalScannerSpec = ExternalScannerSpec{
	Language:       "typescript",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-typescript",
	UpstreamCommit: "75b3874edb2dc714fb1fd77a32013d0f8699989f",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "typescript/src/grammar.json", SHA256: "99dc4b95424dff47c89acb50d5d3f25b081b07eaff43994b7881dccd4186b214"},
		{Path: "typescript/src/scanner.c", SHA256: "9125013b42cb888379d9be909f1d73dfb75a37626c2cdbf4122718a2b431a6d3"},
		{Path: "common/scanner.h", SHA256: "da66ef2bd14a3f7ea743e25ba068c6c9aae2c3509db200ff80c4a0e6116e564c"},
	},
	Externals: []string{
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
	},
}

// oracleTypeScriptExternalScanner handles automatic semicolons, template strings,
// JSX text, ternary question marks, and HTML comments for TypeScript.
type oracleTypeScriptExternalScanner struct {
	symbols         [oracleTsTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token slots to lang's
// external symbols positionally (external index i -> scanner token i), so
// Scan resolves result symbols through the bound table instead of the
// hardcoded absolute IDs above. Required whenever the attached Language did
// not come from the exact blob those absolute IDs were pinned against (e.g.
// a grammargen regeneration that shifts the automaton's overall symbol
// numbering) -- see bindExternalScannerSymbolNames.
func (oracleTypeScriptExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := oracleTypeScriptExternalScanner{symbols: oracleTsDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, oracleTsExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (oracleTypeScriptExternalScanner) Create() any                           { return nil }
func (oracleTypeScriptExternalScanner) Destroy(payload any)                   {}
func (oracleTypeScriptExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (oracleTypeScriptExternalScanner) Deserialize(payload any, buf []byte)   {}
func (oracleTypeScriptExternalScanner) SupportsIncrementalReuse() bool        { return true }

// Scan uses only forward lookahead and the valid-symbol mask. It carries no
// payload, including across failed scans, so every boundary is quiescent.
func (oracleTypeScriptExternalScanner) SupportsStatelessReadDependencies() bool { return true }

// All ASCII digits follow identical branches, including failed speculative scans.
// Grammar bindings change symbols and masks, not character classification.
func (oracleTypeScriptExternalScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= '0' && b <= '9' {
		return 1
	}
	return 0
}

// symbolTable returns the per-Language-bound result-symbol table, falling
// back to the pinned defaults when Scan is invoked on an unbound scanner
// value (s.symbols is still its zero value).
func (s oracleTypeScriptExternalScanner) symbolTable() *[oracleTsTokenCount]gotreesitter.Symbol {
	if s.symbols == ([oracleTsTokenCount]gotreesitter.Symbol{}) {
		return &oracleTsDefaultSymTable
	}
	return &s.symbols
}

// remapValidSymbols translates a Language-external-indexed validSymbols
// slice into scanner-token-indexed space via s.externalToToken. When the
// Language's external count and order agree with oracleTsExternalScannerSpec.Externals (the
// common case), externalToToken is the identity permutation and this is a
// copy; it only diverges when a future grammar version adds, removes, or
// reorders an external token relative to oracleTsExternalScannerSpec.Externals.
func (s oracleTypeScriptExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[oracleTsTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [oracleTsTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < oracleTsTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (s oracleTypeScriptExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var semanticValid [oracleTsTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	if oracleTsValid(validSymbols, oracleTsTokTemplateChars) {
		if oracleTsValid(validSymbols, oracleTsTokAutoSemicolon) {
			return false
		}
		return oracleTsScanTemplateChars(lexer, symbols)
	}

	preferAutoSemicolon := oracleTsPreferAutoSemicolonOverJsxText(lexer, validSymbols)

	if oracleTsValid(validSymbols, oracleTsTokJsxText) && !preferAutoSemicolon {
		if oracleTsScanJsxText(lexer, symbols) {
			return true
		}
	}

	if oracleTsValid(validSymbols, oracleTsTokAutoSemicolon) || oracleTsValid(validSymbols, oracleTsTokFuncSigAutoSemi) {
		scannedComment := false
		ret := oracleTsScanAutoSemicolon(lexer, validSymbols, symbols, &scannedComment)
		if !ret && !scannedComment && oracleTsValid(validSymbols, oracleTsTokTernaryQmark) && lexer.Lookahead() == '?' {
			return oracleTsScanTernaryQmark(lexer, symbols)
		}
		if !ret && !scannedComment && preferAutoSemicolon && oracleTsValid(validSymbols, oracleTsTokJsxText) {
			return oracleTsScanJsxText(lexer, symbols)
		}
		return ret
	}

	if oracleTsValid(validSymbols, oracleTsTokJsxText) && preferAutoSemicolon {
		return oracleTsScanJsxText(lexer, symbols)
	}

	if oracleTsValid(validSymbols, oracleTsTokTernaryQmark) {
		return oracleTsScanTernaryQmark(lexer, symbols)
	}

	if oracleTsValid(validSymbols, oracleTsTokHtmlComment) &&
		!oracleTsValid(validSymbols, oracleTsTokLogicalOr) &&
		!oracleTsValid(validSymbols, oracleTsTokEscapeSequence) &&
		!oracleTsValid(validSymbols, oracleTsTokRegexPattern) {
		return oracleTsScanClosingComment(lexer, symbols)
	}

	return false
}

func oracleTsScanTemplateChars(lexer *gotreesitter.ExternalLexer, symbols *[oracleTsTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[oracleTsTokTemplateChars])
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

func oracleTsScanAutoSemicolon(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[oracleTsTokenCount]gotreesitter.Symbol, scannedComment *bool) bool {
	lexer.SetResultSymbol(symbols[oracleTsTokAutoSemicolon])
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
				return oracleTsValid(validSymbols, oracleTsTokLogicalOr)
			default:
				if oracleTsValid(validSymbols, oracleTsTokJsxText) {
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

	if !oracleTsScanWSAndComments(lexer, scannedComment) {
		return false
	}

	switch lexer.Lookahead() {
	case '`', ',', '.', ';', '*', '%', '>', '=', '?', '^', '|', '&', '/', ':':
		return false
	case '<':
		return oracleTsValid(validSymbols, oracleTsTokFuncSigAutoSemi)
	case '{':
		if oracleTsValid(validSymbols, oracleTsTokFuncSigAutoSemi) {
			return false
		}
	case '(', '[':
		if oracleTsValid(validSymbols, oracleTsTokLogicalOr) {
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
		if !oracleTsValid(validSymbols, oracleTsTokLogicalOr) {
			return true
		}
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

func oracleTsScanWSAndComments(lexer *gotreesitter.ExternalLexer, scannedComment *bool) bool {
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

func oracleTsScanTernaryQmark(lexer *gotreesitter.ExternalLexer, symbols *[oracleTsTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[oracleTsTokTernaryQmark])

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

func oracleTsScanClosingComment(lexer *gotreesitter.ExternalLexer, symbols *[oracleTsTokenCount]gotreesitter.Symbol) bool {
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

	lexer.SetResultSymbol(symbols[oracleTsTokHtmlComment])
	lexer.MarkEnd()
	return true
}

func oracleTsScanJsxText(lexer *gotreesitter.ExternalLexer, symbols *[oracleTsTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[oracleTsTokJsxText])
	return sawText
}

func oracleTsValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

func oracleTsPreferAutoSemicolonOverJsxText(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !oracleTsValid(validSymbols, oracleTsTokAutoSemicolon) || !oracleTsValid(validSymbols, oracleTsTokJsxText) {
		return false
	}
	switch lexer.Lookahead() {
	case 0, '\n', '\r', 0x2028, 0x2029:
		return true
	default:
		return false
	}
}
