//go:build !grammar_subset || grammar_subset_javascript

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the javascript grammar.
const (
	jsTokAutoSemicolon  = 0
	jsTokTemplateChars  = 1
	jsTokTernaryQmark   = 2
	jsTokHtmlComment    = 3
	jsTokLogicalOr      = 4
	jsTokEscapeSequence = 5
	jsTokRegexPattern   = 6
	jsTokJsxText        = 7
	jsTokenCount        = 8
)

// Concrete symbol IDs from the checked-in JavaScript grammar ExternalSymbols.
// These are DEFAULTS only: the fallback entries in jsDefaultSymTable, used
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
	jsSymAutoSemicolon  gotreesitter.Symbol = 129
	jsSymTemplateChars  gotreesitter.Symbol = 130
	jsSymTernaryQmark   gotreesitter.Symbol = 131
	jsSymHtmlComment    gotreesitter.Symbol = 132
	jsSymLogicalOr      gotreesitter.Symbol = 78
	jsSymEscapeSequence gotreesitter.Symbol = 107
	jsSymRegexPattern   gotreesitter.Symbol = 112
	jsSymJsxText        gotreesitter.Symbol = 133
)

// jsDefaultSymTable mirrors the jsTok* index order above.
var jsDefaultSymTable = [jsTokenCount]gotreesitter.Symbol{
	jsTokAutoSemicolon:  jsSymAutoSemicolon,
	jsTokTemplateChars:  jsSymTemplateChars,
	jsTokTernaryQmark:   jsSymTernaryQmark,
	jsTokHtmlComment:    jsSymHtmlComment,
	jsTokLogicalOr:      jsSymLogicalOr,
	jsTokEscapeSequence: jsSymEscapeSequence,
	jsTokRegexPattern:   jsSymRegexPattern,
	jsTokJsxText:        jsSymJsxText,
}

// jsExternalSymbolNames lists the javascript grammar's external tokens in
// declaration order (matching the jsTok* indexes above and the language's
// ExternalSymbols order). Used by ExternalScannerForLanguage to bind this
// scanner's token slots to a Language's external symbols positionally, the
// same pattern used by the kotlin/python/swift/dart/rust/hcl scanners. See
// bindExternalScannerSymbolNames for why positional (not absolute-ID or
// by-name) binding is required.
var jsExternalSymbolNames = []string{
	"_automatic_semicolon",
	"_template_chars",
	"_ternary_qmark",
	"html_comment",
	"||",
	"escape_sequence",
	"regex_pattern",
	"jsx_text",
}

// JavaScriptExternalScanner handles automatic semicolons, template strings,
// JSX text, ternary question marks, and HTML comments for JavaScript.
type JavaScriptExternalScanner struct {
	symbols         [jsTokenCount]gotreesitter.Symbol
	externalToToken []int
}

type jsWhitespaceResult uint8

const (
	jsWhitespaceReject jsWhitespaceResult = iota
	jsWhitespaceNoNewline
	jsWhitespaceAccept
)

// ExternalScannerForLanguage binds this scanner's token slots to lang's
// external symbols positionally (external index i -> scanner token i), so
// Scan resolves result symbols through the bound table instead of the
// hardcoded absolute IDs above. Required whenever the attached Language did
// not come from the exact blob those absolute IDs were pinned against (e.g.
// a grammargen regeneration that shifts the automaton's overall symbol
// numbering) — see bindExternalScannerSymbolNames.
func (JavaScriptExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := JavaScriptExternalScanner{symbols: jsDefaultSymTable}
	s.externalToToken = bindExternalScannerSymbolNames(lang, jsExternalSymbolNames, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (JavaScriptExternalScanner) Create() any                           { return nil }
func (JavaScriptExternalScanner) Destroy(payload any)                   {}
func (JavaScriptExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (JavaScriptExternalScanner) Deserialize(payload any, buf []byte)   {}

// Template, JSX, regex, comment, and ASI decisions are fully determined by
// local lookahead plus validSymbols. The scanner retains no cross-token state.
func (JavaScriptExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (JavaScriptExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (JavaScriptExternalScanner) PreservesStateOnScanFailure() bool { return true }

// symbolTable returns the per-Language-bound result-symbol table, falling
// back to the pinned defaults when Scan is invoked on an unbound scanner
// value (s.symbols is still its zero value).
func (s JavaScriptExternalScanner) symbolTable() *[jsTokenCount]gotreesitter.Symbol {
	if s.symbols == ([jsTokenCount]gotreesitter.Symbol{}) {
		return &jsDefaultSymTable
	}
	return &s.symbols
}

// remapValidSymbols translates a Language-external-indexed validSymbols
// slice into scanner-token-indexed space via s.externalToToken. When the
// Language's external count and order agree with jsExternalSymbolNames (the
// common case), externalToToken is the identity permutation and this is a
// copy; it only diverges when a future grammar version adds, removes, or
// reorders an external token relative to jsExternalSymbolNames.
func (s JavaScriptExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[jsTokenCount]bool) []bool {
	if len(s.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [jsTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(s.externalToToken) {
			continue
		}
		tokenIdx := s.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < jsTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (s JavaScriptExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var semanticValid [jsTokenCount]bool
	validSymbols = s.remapValidSymbols(validSymbols, &semanticValid)
	symbols := s.symbolTable()

	if jsValid(validSymbols, jsTokTemplateChars) {
		if jsValid(validSymbols, jsTokAutoSemicolon) {
			return false
		}
		return jsScanTemplateChars(lexer, symbols)
	}

	preferAutoSemicolon := jsPreferAutoSemicolonOverJsxText(lexer, validSymbols)

	if jsValid(validSymbols, jsTokJsxText) && !preferAutoSemicolon {
		if jsScanJsxText(lexer, symbols) {
			return true
		}
	}

	if jsValid(validSymbols, jsTokAutoSemicolon) {
		scannedComment := false
		ret := jsScanAutoSemicolon(lexer, validSymbols, symbols, &scannedComment)
		if !ret && !scannedComment && jsValid(validSymbols, jsTokTernaryQmark) && lexer.Lookahead() == '?' {
			return jsScanTernaryQmark(lexer, symbols)
		}
		if !ret && !scannedComment && preferAutoSemicolon && jsValid(validSymbols, jsTokJsxText) {
			return jsScanJsxText(lexer, symbols)
		}
		return ret
	}

	if jsValid(validSymbols, jsTokJsxText) && preferAutoSemicolon {
		return jsScanJsxText(lexer, symbols)
	}

	if jsValid(validSymbols, jsTokTernaryQmark) {
		return jsScanTernaryQmark(lexer, symbols)
	}

	if jsValid(validSymbols, jsTokHtmlComment) &&
		!jsValid(validSymbols, jsTokLogicalOr) &&
		!jsValid(validSymbols, jsTokEscapeSequence) &&
		!jsValid(validSymbols, jsTokRegexPattern) {
		return jsScanClosingComment(lexer, symbols)
	}

	return false
}

func jsScanTemplateChars(lexer *gotreesitter.ExternalLexer, symbols *[jsTokenCount]gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(symbols[jsTokTemplateChars])
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

func jsScanAutoSemicolon(lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[jsTokenCount]gotreesitter.Symbol, scannedComment *bool) bool {
	lexer.SetResultSymbol(symbols[jsTokAutoSemicolon])
	lexer.MarkEnd()

	for {
		ch := lexer.Lookahead()
		if ch == 0 {
			return true
		}
		if ch == '/' {
			result := jsProbeWhitespaceAndComments(lexer, scannedComment, false)
			if result == jsWhitespaceReject {
				return false
			}
			if result == jsWhitespaceAccept &&
				!jsValid(validSymbols, jsTokLogicalOr) &&
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
				return jsValid(validSymbols, jsTokLogicalOr)
			default:
				if jsValid(validSymbols, jsTokJsxText) {
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

	if !jsScanWSAndComments(lexer, scannedComment) {
		return false
	}

	switch lexer.Lookahead() {
	case '`', ',', '.', ';', '*', '%', '>', '<', '=', '?', '^', '|', '&', '/', ':':
		return false
	case '{':
		// JavaScript has no func_sig_auto_semi, so no special handling here.
	case '(', '[':
		if jsValid(validSymbols, jsTokLogicalOr) {
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

// jsProbeWhitespaceAndComments mirrors the pinned JavaScript scanner's
// same-line comment probe. In particular, a valid automatic semicolon is
// emitted at the scanner's original mark before the comment is shifted as an
// extra. Keeping that ordering is what lets the parser reduce `} ASI` first and
// replay the following comment into the surrounding statement list.
func jsProbeWhitespaceAndComments(lexer *gotreesitter.ExternalLexer, scannedComment *bool, consume bool) jsWhitespaceResult {
	sawBlockNewline := false
	for {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}

		if lexer.Lookahead() != '/' {
			return jsWhitespaceAccept
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
								return jsWhitespaceAccept
							}
							return jsWhitespaceNoNewline
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
			return jsWhitespaceReject
		}
	}
}

func jsScanWSAndComments(lexer *gotreesitter.ExternalLexer, scannedComment *bool) bool {
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

func jsScanTernaryQmark(lexer *gotreesitter.ExternalLexer, symbols *[jsTokenCount]gotreesitter.Symbol) bool {
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
	lexer.SetResultSymbol(symbols[jsTokTernaryQmark])

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

func jsScanClosingComment(lexer *gotreesitter.ExternalLexer, symbols *[jsTokenCount]gotreesitter.Symbol) bool {
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

	lexer.SetResultSymbol(symbols[jsTokHtmlComment])
	lexer.MarkEnd()
	return true
}

func jsScanJsxText(lexer *gotreesitter.ExternalLexer, symbols *[jsTokenCount]gotreesitter.Symbol) bool {
	sawText := false
	atNewline := false
	onlyWhitespace := true

	for lexer.Lookahead() != 0 && lexer.Lookahead() != '<' && lexer.Lookahead() != '>' &&
		lexer.Lookahead() != '{' && lexer.Lookahead() != '}' {
		if lexer.Lookahead() == '&' {
			lexer.MarkEnd()
			if jsScanHTMLCharacterReference(lexer) {
				// A full html_character_reference (for example "&amp;")
				// follows. Stop the text token here so that token can lex
				// the entity, same as before this fix.
				lexer.SetResultSymbol(symbols[jsTokJsxText])
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
	lexer.SetResultSymbol(symbols[jsTokJsxText])
	return sawText
}

// jsScanHTMLCharacterReference reports whether the lexer, positioned at
// '&', is looking at a complete html_character_reference as defined in
// grammargen/javascript_grammar.go:
//
//	&(#([xX][0-9a-fA-F]{1,6}|[0-9]{1,5})|[A-Za-z]{1,30});
//
// It always consumes at least the '&'. Callers that get false must treat
// everything the lexer consumed as literal JSX text: none of it can start
// any other token.
func jsScanHTMLCharacterReference(lexer *gotreesitter.ExternalLexer) bool {
	lexer.Advance(false) // '&'
	if lexer.Lookahead() == '#' {
		lexer.Advance(false)
		if lexer.Lookahead() == 'x' || lexer.Lookahead() == 'X' {
			lexer.Advance(false)
			n := 0
			for isJsHexDigit(lexer.Lookahead()) && n < 6 {
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
		for isJsASCIIDigit(lexer.Lookahead()) && n < 5 {
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
	for isJsASCIILetter(lexer.Lookahead()) && n < 30 {
		lexer.Advance(false)
		n++
	}
	if n == 0 || lexer.Lookahead() != ';' {
		return false
	}
	lexer.Advance(false)
	return true
}

func isJsASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

func isJsASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isJsHexDigit(r rune) bool {
	return isJsASCIIDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func jsValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

func jsPreferAutoSemicolonOverJsxText(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !jsValid(validSymbols, jsTokAutoSemicolon) || !jsValid(validSymbols, jsTokJsxText) {
		return false
	}
	switch lexer.Lookahead() {
	case 0, '\n', '\r', 0x2028, 0x2029:
		return true
	default:
		return false
	}
}
