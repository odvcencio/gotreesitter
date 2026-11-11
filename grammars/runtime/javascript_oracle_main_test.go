//go:build !grammar_subset || grammar_subset_javascript

package grammarruntime

// This file is a copy of the scanner source as it was before the C-faithful
// mode was added. The differential tests use it as their oracle for the default
// mode. It is not generated; do not edit it by hand.

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"unicode"
)

// External token indexes for the javascript grammar.
const (
	oracleJsTokAutoSemicolon  = 0
	oracleJsTokTemplateChars  = 1
	oracleJsTokTernaryQmark   = 2
	oracleJsTokHtmlComment    = 3
	oracleJsTokLogicalOr      = 4
	oracleJsTokEscapeSequence = 5
	oracleJsTokRegexPattern   = 6
	oracleJsTokJsxText        = 7
	oracleJsTokenCount        = 8
)

// Concrete symbol IDs from the checked-in JavaScript grammar ExternalSymbols.
// These are DEFAULTS only: the fallback entries in oracleJsDefaultSymTable, used
// as-is if Scan is ever invoked on a scanner value that was never bound to a
// Language (should not happen in production; see ExternalScannerForLanguage).
// ExternalScannerForLanguage binds the real per-Language symbols
// POSITIONALLY (by external index, via bindExternalScannerSymbolNames), not
// by these absolute IDs, so a grammargen-regenerated javascript blob — which
// emits the same 8 externals in the same order but under different absolute
// Symbol numbering once the automaton grows or shrinks — still lexes
// correctly instead of silently mistyping every external scan result. See
// bindExternalScannerSymbolNames for why positional (not absolute-ID or
// by-name) binding is required; the same pattern is used by the
// kotlin/python/swift/dart/rust/hcl scanners.
const (
	oracleJsSymAutoSemicolon  gotreesitter.Symbol = 129
	oracleJsSymTemplateChars  gotreesitter.Symbol = 130
	oracleJsSymTernaryQmark   gotreesitter.Symbol = 131
	oracleJsSymHtmlComment    gotreesitter.Symbol = 132
	oracleJsSymLogicalOr      gotreesitter.Symbol = 78
	oracleJsSymEscapeSequence gotreesitter.Symbol = 107
	oracleJsSymRegexPattern   gotreesitter.Symbol = 112
	oracleJsSymJsxText        gotreesitter.Symbol = 133
)

// oracleJsDefaultSymTable mirrors the jsTok* index order above.
var oracleJsDefaultSymTable = [oracleJsTokenCount]gotreesitter.Symbol{
	oracleJsTokAutoSemicolon:  oracleJsSymAutoSemicolon,
	oracleJsTokTemplateChars:  oracleJsSymTemplateChars,
	oracleJsTokTernaryQmark:   oracleJsSymTernaryQmark,
	oracleJsTokHtmlComment:    oracleJsSymHtmlComment,
	oracleJsTokLogicalOr:      oracleJsSymLogicalOr,
	oracleJsTokEscapeSequence: oracleJsSymEscapeSequence,
	oracleJsTokRegexPattern:   oracleJsSymRegexPattern,
	oracleJsTokJsxText:        oracleJsSymJsxText,
}

// oracleJsExternalScannerSpec records the upstream scanner-source contract for this
// port. Externals lists the javascript grammar's external tokens in
// declaration order (matching the jsTok* indexes above and the language's
// ExternalSymbols order). ExternalScannerForLanguage binds this scanner's
// token slots to a Language's external symbols positionally through it, the
// same pattern used by the kotlin/python/swift/dart/rust/hcl scanners. See
// bindExternalScannerSymbolNames for why positional (not absolute-ID or
// by-name) binding is required.
var oracleJsExternalScannerSpec = ExternalScannerSpec{
	Language:       "javascript",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-javascript",
	UpstreamCommit: "58404d8cf191d69f2674a8fd507bd5776f46cb11",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "7d9364b824be9250ba023065784550475d277b9dc9a7d20f9bebe9e471c908c3"},
		{Path: "src/scanner.c", SHA256: "b3d3f64284d97bf80749c026862427782cf7ecc0b7dc094e6698ab311c9a42c7"},
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
	},
}

// oracleJavaScriptExternalScanner handles automatic semicolons, template strings,
// JSX text, ternary question marks, and HTML comments for JavaScript.
type oracleJavaScriptExternalScanner struct {
	symbols         [oracleJsTokenCount]gotreesitter.Symbol
	externalToToken []int
}

type oracleJsWhitespaceResult uint8

const (
	oracleJsWhitespaceReject oracleJsWhitespaceResult = iota
	oracleJsWhitespaceNoNewline
	oracleJsWhitespaceAccept
)

// ExternalScannerForLanguage binds this scanner's token slots to lang's
// external symbols positionally (external index i -> scanner token i), so
// Scan resolves result symbols through the bound table instead of the
// hardcoded absolute IDs above. Required whenever the attached Language did
// not come from the exact blob those absolute IDs were pinned against (e.g.
// a grammargen regeneration that shifts the automaton's overall symbol
// numbering) — see bindExternalScannerSymbolNames.
func (oracleJavaScriptExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := oracleJavaScriptExternalScanner{symbols: oracleJsDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, oracleJsExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (oracleJavaScriptExternalScanner) Create() any                           { return nil }
func (oracleJavaScriptExternalScanner) Destroy(payload any)                   {}
func (oracleJavaScriptExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (oracleJavaScriptExternalScanner) Deserialize(payload any, buf []byte)   {}

// Template, JSX, regex, comment, and ASI decisions are fully determined by
// local lookahead plus validSymbols. The scanner retains no cross-token state.
func (oracleJavaScriptExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (oracleJavaScriptExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (oracleJavaScriptExternalScanner) PreservesStateOnScanFailure() bool { return true }

// symbolTable returns the per-Language-bound result-symbol table, falling
// back to the pinned defaults when Scan is invoked on an unbound scanner
// value (s.symbols is still its zero value).
func (s oracleJavaScriptExternalScanner) symbolTable() *[oracleJsTokenCount]gotreesitter.Symbol {
	if s.symbols == ([oracleJsTokenCount]gotreesitter.Symbol{}) {
		return &oracleJsDefaultSymTable
	}
	return &s.symbols
}

// remapValidSymbols translates a Language-external-indexed validSymbols
// slice into scanner-token-indexed space via s.externalToToken. When the
// Language's external count and order agree with oracleJsExternalScannerSpec.Externals (the
// common case), externalToToken is the identity permutation and this is a
// copy; it only diverges when a future grammar version adds, removes, or
// reorders an external token relative to oracleJsExternalScannerSpec.Externals.
func (s oracleJavaScriptExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[oracleJsTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [oracleJsTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < oracleJsTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (s oracleJavaScriptExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var semanticValid [oracleJsTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	if oracleJsValid(validSymbols, oracleJsTokTemplateChars) {
		if oracleJsValid(validSymbols, oracleJsTokAutoSemicolon) {
			return false
		}
		return oracleJsScanTemplateChars(lexer, symbols)
	}

	preferAutoSemicolon := oracleJsPreferAutoSemicolonOverJsxText(lexer, validSymbols)

	if oracleJsValid(validSymbols, oracleJsTokJsxText) && !preferAutoSemicolon {
		if oracleJsScanJsxText(lexer, symbols) {
			return true
		}
	}

	if oracleJsValid(validSymbols, oracleJsTokAutoSemicolon) {
		scannedComment := false
		ret := oracleJsScanAutoSemicolon(lexer, validSymbols, symbols, &scannedComment)
		if !ret && !scannedComment && oracleJsValid(validSymbols, oracleJsTokTernaryQmark) && lexer.Lookahead() == '?' {
			return oracleJsScanTernaryQmark(lexer, symbols)
		}
		if !ret && !scannedComment && preferAutoSemicolon && oracleJsValid(validSymbols, oracleJsTokJsxText) {
			return oracleJsScanJsxText(lexer, symbols)
		}
		return ret
	}

	if oracleJsValid(validSymbols, oracleJsTokJsxText) && preferAutoSemicolon {
		return oracleJsScanJsxText(lexer, symbols)
	}

	if oracleJsValid(validSymbols, oracleJsTokTernaryQmark) {
		return oracleJsScanTernaryQmark(lexer, symbols)
	}

	if oracleJsValid(validSymbols, oracleJsTokHtmlComment) &&
		!oracleJsValid(validSymbols, oracleJsTokLogicalOr) &&
		!oracleJsValid(validSymbols, oracleJsTokEscapeSequence) &&
		!oracleJsValid(validSymbols, oracleJsTokRegexPattern) {
		return oracleJsScanClosingComment(lexer, symbols)
	}

	return false
}

func oracleJsScanTemplateChars(lexer *gotreesitter.ExternalLexer, symbols *[oracleJsTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[oracleJsTokTemplateChars])
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

func oracleJsScanAutoSemicolon(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[oracleJsTokenCount]gotreesitter.Symbol, scannedComment *bool) bool {
	lexer.SetResultSymbol(symbols[oracleJsTokAutoSemicolon])
	lexer.MarkEnd()

	for {
		ch := lexer.Lookahead()
		if ch == 0 {
			return true
		}
		if ch == '/' {
			result := oracleJsProbeWhitespaceAndComments(lexer, scannedComment, false)
			if result == oracleJsWhitespaceReject {
				return false
			}
			if result == oracleJsWhitespaceAccept &&
				!oracleJsValid(validSymbols, oracleJsTokLogicalOr) &&
				lexer.Lookahead() != ',' && lexer.Lookahead() != '=' {
				return true
			}
			ch = lexer.Lookahead()
		}
		if ch == '}' {
			lexer.Advance(true)
			for unicode.IsSpace(lexer.Lookahead()) {
				lexer.Advance(true)
			}
			switch lexer.Lookahead() {
			case ':':
				return oracleJsValid(validSymbols, oracleJsTokLogicalOr)
			default:
				if oracleJsValid(validSymbols, oracleJsTokJsxText) {
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

	if !oracleJsScanWSAndComments(lexer, scannedComment) {
		return false
	}

	switch lexer.Lookahead() {
	case '`', ',', '.', ';', '*', '%', '>', '<', '=', '?', '^', '|', '&', '/', ':':
		return false
	case '{':
		// JavaScript has no func_sig_auto_semi, so no special handling here.
	case '(', '[':
		if oracleJsValid(validSymbols, oracleJsTokLogicalOr) {
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

// oracleJsProbeWhitespaceAndComments mirrors the pinned JavaScript scanner's
// same-line comment probe. In particular, a valid automatic semicolon is
// emitted at the scanner's original mark before the comment is shifted as an
// extra. Keeping that ordering is what lets the parser reduce `} ASI` first and
// replay the following comment into the surrounding statement list.
func oracleJsProbeWhitespaceAndComments(lexer *gotreesitter.ExternalLexer, scannedComment *bool, consume bool) oracleJsWhitespaceResult {
	sawBlockNewline := false
	for {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}

		if lexer.Lookahead() != '/' {
			return oracleJsWhitespaceAccept
		}
		lexer.Advance(true)
		switch lexer.Lookahead() {
		case '/':
			lexer.Advance(true)
			for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' &&
				lexer.Lookahead() != 0x2028 && lexer.Lookahead() != 0x2029 {
				lexer.Advance(true)
			}
			*scannedComment = true
		case '*':
			lexer.Advance(true)
		scanBlock:
			for lexer.Lookahead() != 0 {
				switch lexer.Lookahead() {
				case '*':
					lexer.Advance(true)
					if lexer.Lookahead() == '/' {
						lexer.Advance(true)
						*scannedComment = true
						if lexer.Lookahead() != '/' && !consume {
							if sawBlockNewline {
								return oracleJsWhitespaceAccept
							}
							return oracleJsWhitespaceNoNewline
						}
						break scanBlock
					}
				case '\n', 0x2028, 0x2029:
					sawBlockNewline = true
					lexer.Advance(true)
				default:
					lexer.Advance(true)
				}
			}
		default:
			return oracleJsWhitespaceReject
		}
	}
}

func oracleJsScanWSAndComments(lexer *gotreesitter.ExternalLexer, scannedComment *bool) bool {
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

func oracleJsScanTernaryQmark(lexer *gotreesitter.ExternalLexer, symbols *[oracleJsTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[oracleJsTokTernaryQmark])

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

func oracleJsScanClosingComment(lexer *gotreesitter.ExternalLexer, symbols *[oracleJsTokenCount]gotreesitter.Symbol) bool {
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

	lexer.SetResultSymbol(symbols[oracleJsTokHtmlComment])
	lexer.MarkEnd()
	return true
}

func oracleJsScanJsxText(lexer *gotreesitter.ExternalLexer, symbols *[oracleJsTokenCount]gotreesitter.Symbol) bool {
	sawText := false
	atNewline := false
	onlyWhitespace := true

	for lexer.Lookahead() != 0 && lexer.Lookahead() != '<' && lexer.Lookahead() != '>' &&
		lexer.Lookahead() != '{' && lexer.Lookahead() != '}' {
		if lexer.Lookahead() == '&' {
			lexer.MarkEnd()
			if oracleJsScanHTMLCharacterReference(lexer) {
				// A full html_character_reference (for example "&amp;")
				// follows. Stop the text token here so that token can lex
				// the entity, same as before this fix.
				lexer.SetResultSymbol(symbols[oracleJsTokJsxText])
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
	lexer.SetResultSymbol(symbols[oracleJsTokJsxText])
	return sawText
}

// oracleJsScanHTMLCharacterReference reports whether the lexer, positioned at
// '&', is looking at a complete html_character_reference as defined in
// grammargen/javascript_grammar.go:
//
//	&(#([xX][0-9a-fA-F]{1,6}|[0-9]{1,5})|[A-Za-z]{1,30});
//
// It always consumes at least the '&'. Callers that get false must treat
// everything the lexer consumed as literal JSX text: none of it can start
// any other token.
func oracleJsScanHTMLCharacterReference(lexer *gotreesitter.ExternalLexer) bool {
	lexer.Advance(false) // '&'
	if lexer.Lookahead() == '#' {
		lexer.Advance(false)
		if lexer.Lookahead() == 'x' || lexer.Lookahead() == 'X' {
			lexer.Advance(false)
			n := 0
			for oracleIsJsHexDigit(lexer.Lookahead()) && n < 6 {
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
		for oracleIsJsASCIIDigit(lexer.Lookahead()) && n < 5 {
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
	for oracleIsJsASCIILetter(lexer.Lookahead()) && n < 30 {
		lexer.Advance(false)
		n++
	}
	if n == 0 || lexer.Lookahead() != ';' {
		return false
	}
	lexer.Advance(false)
	return true
}

func oracleIsJsASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

func oracleIsJsASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func oracleIsJsHexDigit(r rune) bool {
	return oracleIsJsASCIIDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func oracleJsValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

func oracleJsPreferAutoSemicolonOverJsxText(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !oracleJsValid(validSymbols, oracleJsTokAutoSemicolon) || !oracleJsValid(validSymbols, oracleJsTokJsxText) {
		return false
	}
	switch lexer.Lookahead() {
	case 0, '\n', '\r', 0x2028, 0x2029:
		return true
	default:
		return false
	}
}
