//go:build !grammar_subset || grammar_subset_tsx

package grammarruntime

// This file is a copy of the scanner source as it was before the C-faithful
// mode was added. The differential tests use it as their oracle for the default
// mode. It is not generated; do not edit it by hand.

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"unicode"
)

// External token indexes for the tsx grammar.
const (
	oracleTsxTokAutoSemicolon   = 0
	oracleTsxTokTemplateChars   = 1
	oracleTsxTokTernaryQmark    = 2
	oracleTsxTokHtmlComment     = 3
	oracleTsxTokLogicalOr       = 4
	oracleTsxTokEscapeSequence  = 5
	oracleTsxTokRegexPattern    = 6
	oracleTsxTokJsxText         = 7
	oracleTsxTokFuncSigAutoSemi = 8
	oracleTsxTokErrorRecovery   = 9
	oracleTsxTokenCount         = 10
)

// Concrete symbol IDs from the checked-in TSX grammar ExternalSymbols. These
// are DEFAULTS only: the fallback entries in oracleTsxDefaultSymTable, used as-is
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
	oracleTsxSymAutoSemicolon   gotreesitter.Symbol = 166
	oracleTsxSymTemplateChars   gotreesitter.Symbol = 167
	oracleTsxSymTernaryQmark    gotreesitter.Symbol = 168
	oracleTsxSymHtmlComment     gotreesitter.Symbol = 169
	oracleTsxSymLogicalOr       gotreesitter.Symbol = 81
	oracleTsxSymEscapeSequence  gotreesitter.Symbol = 109
	oracleTsxSymRegexPattern    gotreesitter.Symbol = 114
	oracleTsxSymJsxText         gotreesitter.Symbol = 170
	oracleTsxSymFuncSigAutoSemi gotreesitter.Symbol = 171
	oracleTsxSymErrorRecovery   gotreesitter.Symbol = 172
)

// oracleTsxDefaultSymTable mirrors the tsxTok* index order above.
var oracleTsxDefaultSymTable = [oracleTsxTokenCount]gotreesitter.Symbol{
	oracleTsxTokAutoSemicolon:   oracleTsxSymAutoSemicolon,
	oracleTsxTokTemplateChars:   oracleTsxSymTemplateChars,
	oracleTsxTokTernaryQmark:    oracleTsxSymTernaryQmark,
	oracleTsxTokHtmlComment:     oracleTsxSymHtmlComment,
	oracleTsxTokLogicalOr:       oracleTsxSymLogicalOr,
	oracleTsxTokEscapeSequence:  oracleTsxSymEscapeSequence,
	oracleTsxTokRegexPattern:    oracleTsxSymRegexPattern,
	oracleTsxTokJsxText:         oracleTsxSymJsxText,
	oracleTsxTokFuncSigAutoSemi: oracleTsxSymFuncSigAutoSemi,
	oracleTsxTokErrorRecovery:   oracleTsxSymErrorRecovery,
}

// oracleTsxExternalScannerSpec records the upstream scanner-source contract for this
// port. Externals lists the tsx grammar's external tokens in declaration
// order (matching the tsxTok* indexes above and the language's
// ExternalSymbols order). ExternalScannerForLanguage binds this scanner's
// token slots to a Language's external symbols positionally through it, the
// same pattern used by the javascript/typescript/kotlin/python/swift/dart/
// rust/hcl scanners. See bindExternalScannerSymbolNames for why positional
// (not absolute-ID or by-name) binding is required. The per-grammar
// scanner.c is a thin shim; common/scanner.h holds the scanner logic.
var oracleTsxExternalScannerSpec = ExternalScannerSpec{
	Language:       "tsx",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-typescript",
	UpstreamCommit: "75b3874edb2dc714fb1fd77a32013d0f8699989f",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "tsx/src/grammar.json", SHA256: "f98a7830b67be266c9e9effb673a5fc154ad6aa26f87f7c6d4a9191e09d8b357"},
		{Path: "tsx/src/scanner.c", SHA256: "d563cd30b2f39718c9ae4292795c5ce03a2ad01954ba3a86ef84c2781a736673"},
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

// oracleTsxExternalScanner handles automatic semicolons, template strings,
// JSX text, ternary question marks, and HTML comments for TSX.
type oracleTsxExternalScanner struct {
	symbols         [oracleTsxTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token slots to lang's
// external symbols positionally (external index i -> scanner token i), so
// Scan resolves result symbols through the bound table instead of the
// hardcoded absolute IDs above. Required whenever the attached Language did
// not come from the exact blob those absolute IDs were pinned against (e.g.
// a grammargen regeneration that shifts the automaton's overall symbol
// numbering) -- see bindExternalScannerSymbolNames.
func (oracleTsxExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := oracleTsxExternalScanner{symbols: oracleTsxDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, oracleTsxExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (oracleTsxExternalScanner) Create() any                           { return nil }
func (oracleTsxExternalScanner) Destroy(payload any)                   {}
func (oracleTsxExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (oracleTsxExternalScanner) Deserialize(payload any, buf []byte)   {}
func (oracleTsxExternalScanner) SupportsIncrementalReuse() bool        { return true }

// Scan is stateless and reads source through forward lexer operations. Failed
// probes participate in the same read history as successful tokens.
func (oracleTsxExternalScanner) SupportsLargeStatelessReadDependencies() bool { return true }

// symbolTable returns the per-Language-bound result-symbol table, falling
// back to the pinned defaults when Scan is invoked on an unbound scanner
// value (s.symbols is still its zero value).
func (s oracleTsxExternalScanner) symbolTable() *[oracleTsxTokenCount]gotreesitter.Symbol {
	if s.symbols == ([oracleTsxTokenCount]gotreesitter.Symbol{}) {
		return &oracleTsxDefaultSymTable
	}
	return &s.symbols
}

// remapValidSymbols translates a Language-external-indexed validSymbols
// slice into scanner-token-indexed space via s.externalToToken. When the
// Language's external count and order agree with oracleTsxExternalScannerSpec.Externals (the
// common case), externalToToken is the identity permutation and this is a
// copy; it only diverges when a future grammar version adds, removes, or
// reorders an external token relative to oracleTsxExternalScannerSpec.Externals.
func (s oracleTsxExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[oracleTsxTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [oracleTsxTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < oracleTsxTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (s oracleTsxExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var semanticValid [oracleTsxTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	if oracleTsxValid(validSymbols, oracleTsxTokTemplateChars) {
		if oracleTsxValid(validSymbols, oracleTsxTokAutoSemicolon) {
			return false
		}
		return oracleTsxScanTemplateChars(lexer, symbols)
	}

	preferAutoSemicolon := oracleTsxPreferAutoSemicolonOverJsxText(lexer, validSymbols)

	if oracleTsxValid(validSymbols, oracleTsxTokJsxText) && !preferAutoSemicolon {
		if oracleTsxScanJsxText(lexer, symbols) {
			return true
		}
	}

	if oracleTsxValid(validSymbols, oracleTsxTokAutoSemicolon) || oracleTsxValid(validSymbols, oracleTsxTokFuncSigAutoSemi) {
		scannedComment := false
		ret := oracleTsxScanAutoSemicolon(lexer, validSymbols, symbols, &scannedComment)
		if !ret && !scannedComment && oracleTsxValid(validSymbols, oracleTsxTokTernaryQmark) && lexer.Lookahead() == '?' {
			return oracleTsxScanTernaryQmark(lexer, symbols)
		}
		if !ret && !scannedComment && preferAutoSemicolon && oracleTsxValid(validSymbols, oracleTsxTokJsxText) {
			return oracleTsxScanJsxText(lexer, symbols)
		}
		return ret
	}

	if oracleTsxValid(validSymbols, oracleTsxTokJsxText) && preferAutoSemicolon {
		return oracleTsxScanJsxText(lexer, symbols)
	}

	if oracleTsxValid(validSymbols, oracleTsxTokTernaryQmark) {
		return oracleTsxScanTernaryQmark(lexer, symbols)
	}

	if oracleTsxValid(validSymbols, oracleTsxTokHtmlComment) &&
		!oracleTsxValid(validSymbols, oracleTsxTokLogicalOr) &&
		!oracleTsxValid(validSymbols, oracleTsxTokEscapeSequence) &&
		!oracleTsxValid(validSymbols, oracleTsxTokRegexPattern) {
		return oracleTsxScanClosingComment(lexer, symbols)
	}

	return false
}

func oracleTsxScanTemplateChars(lexer *gotreesitter.ExternalLexer, symbols *[oracleTsxTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[oracleTsxTokTemplateChars])
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

func oracleTsxScanAutoSemicolon(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[oracleTsxTokenCount]gotreesitter.Symbol, scannedComment *bool) bool {
	lexer.SetResultSymbol(symbols[oracleTsxTokAutoSemicolon])
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
				return oracleTsxValid(validSymbols, oracleTsxTokLogicalOr)
			default:
				if oracleTsxValid(validSymbols, oracleTsxTokJsxText) {
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

	if !oracleTsxScanWSAndComments(lexer, scannedComment) {
		return false
	}

	switch lexer.Lookahead() {
	case '`', ',', '.', ';', '*', '%', '>', '=', '?', '^', '|', '&', '/', ':':
		return false
	case '<':
		return oracleTsxValid(validSymbols, oracleTsxTokFuncSigAutoSemi)
	case '{':
		if oracleTsxValid(validSymbols, oracleTsxTokFuncSigAutoSemi) {
			return false
		}
	case '(', '[':
		if oracleTsxValid(validSymbols, oracleTsxTokLogicalOr) {
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
		if !oracleTsxValid(validSymbols, oracleTsxTokLogicalOr) {
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

func oracleTsxScanWSAndComments(lexer *gotreesitter.ExternalLexer, scannedComment *bool) bool {
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

func oracleTsxScanTernaryQmark(lexer *gotreesitter.ExternalLexer, symbols *[oracleTsxTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[oracleTsxTokTernaryQmark])

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

func oracleTsxScanClosingComment(lexer *gotreesitter.ExternalLexer, symbols *[oracleTsxTokenCount]gotreesitter.Symbol) bool {
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

	lexer.SetResultSymbol(symbols[oracleTsxTokHtmlComment])
	lexer.MarkEnd()
	return true
}

func oracleTsxScanJsxText(lexer *gotreesitter.ExternalLexer, symbols *[oracleTsxTokenCount]gotreesitter.Symbol) bool {
	sawText := false
	atNewline := false
	onlyWhitespace := true

	for lexer.Lookahead() != 0 && lexer.Lookahead() != '<' && lexer.Lookahead() != '>' &&
		lexer.Lookahead() != '{' && lexer.Lookahead() != '}' {
		if lexer.Lookahead() == '&' {
			lexer.MarkEnd()
			if oracleTsxScanHTMLCharacterReference(lexer) {
				// A full html_character_reference (for example "&amp;")
				// follows. Stop the text token here so that token can lex
				// the entity, same as before this fix.
				lexer.SetResultSymbol(symbols[oracleTsxTokJsxText])
				return sawText
			}
			// Not a character reference. Real JSX text allows a bare '&'
			// (tsc accepts it); only the external scanner used to stop
			// here unconditionally. Treat the '&', and any letters/digits
			// already examined while checking, as literal text. See
			// gotreesitter issue #1242 and the upstream
			// tree-sitter-javascript#366 report this mirrors.
			sawText = true
			onlyWhitespace = false
			atNewline = false
			continue
		}
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
	lexer.SetResultSymbol(symbols[oracleTsxTokJsxText])
	return sawText
}

// oracleTsxScanHTMLCharacterReference reports whether the lexer, positioned at
// '&', is looking at a complete html_character_reference as defined in
// grammargen/tsx_grammar.go:
//
//	&(#([xX][0-9a-fA-F]{1,6}|[0-9]{1,5})|[A-Za-z]{1,30});
//
// It always consumes at least the '&'. Callers that get false must treat
// everything the lexer consumed as literal JSX text: none of it can start
// any other token.
func oracleTsxScanHTMLCharacterReference(lexer *gotreesitter.ExternalLexer) bool {
	lexer.Advance(false) // '&'
	if lexer.Lookahead() == '#' {
		lexer.Advance(false)
		if lexer.Lookahead() == 'x' || lexer.Lookahead() == 'X' {
			lexer.Advance(false)
			n := 0
			for oracleIsTsxHexDigit(lexer.Lookahead()) && n < 6 {
				lexer.Advance(false)
				n++
			}
			if n == 0 || lexer.Lookahead() != ';' {
				return false
			}
			lexer.Advance(false)
			return true
		}
		n := 0
		for oracleIsTsxASCIIDigit(lexer.Lookahead()) && n < 5 {
			lexer.Advance(false)
			n++
		}
		if n == 0 || lexer.Lookahead() != ';' {
			return false
		}
		lexer.Advance(false)
		return true
	}
	n := 0
	for oracleIsTsxASCIILetter(lexer.Lookahead()) && n < 30 {
		lexer.Advance(false)
		n++
	}
	if n == 0 || lexer.Lookahead() != ';' {
		return false
	}
	lexer.Advance(false)
	return true
}

func oracleIsTsxASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

func oracleIsTsxASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func oracleIsTsxHexDigit(r rune) bool {
	return oracleIsTsxASCIIDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func oracleTsxValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

func oracleTsxPreferAutoSemicolonOverJsxText(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !oracleTsxValid(validSymbols, oracleTsxTokAutoSemicolon) || !oracleTsxValid(validSymbols, oracleTsxTokJsxText) {
		return false
	}
	switch lexer.Lookahead() {
	case 0, '\n', '\r', 0x2028, 0x2029:
		return true
	default:
		return false
	}
}
