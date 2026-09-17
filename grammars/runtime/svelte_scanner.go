//go:build !grammar_subset || grammar_subset_svelte

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Svelte grammar.
const (
	svelteTokStartTagName             = 0  // tag_name (start)
	svelteTokScriptStartTagName       = 1  // tag_name (script)
	svelteTokStyleStartTagName        = 2  // tag_name (style)
	svelteTokEndTagName               = 3  // tag_name (end)
	svelteTokErroneousEndTagName      = 4  // erroneous_end_tag_name
	svelteTokSelfClosingTagDelim      = 5  // />
	svelteTokImplicitEndTag           = 6  // _implicit_end_tag
	svelteTokRawText                  = 7  // raw_text
	svelteTokComment                  = 8  // comment
	svelteTokSvelteRawText            = 9  // svelte_raw_text (first variant)
	svelteTokSvelteRawTextEach        = 10 // svelte_raw_text (each variant)
	svelteTokSvelteRawTextSnippetArgs = 11 // svelte_raw_text (snippet arguments)
	svelteTokAt                       = 12 // @
	svelteTokHash                     = 13 // #
	svelteTokSlash                    = 14 // /
	svelteTokColon                    = 15 // :
)

// Symbol IDs matching the grammar's node-type table.
const (
	svelteSymStartTagName             gotreesitter.Symbol = 41
	svelteSymScriptStartTagName       gotreesitter.Symbol = 42
	svelteSymStyleStartTagName        gotreesitter.Symbol = 43
	svelteSymEndTagName               gotreesitter.Symbol = 44
	svelteSymErroneousEndTagName      gotreesitter.Symbol = 45
	svelteSymSelfClosingTagDelim      gotreesitter.Symbol = 6
	svelteSymImplicitEndTag           gotreesitter.Symbol = 46
	svelteSymRawText                  gotreesitter.Symbol = 47
	svelteSymComment                  gotreesitter.Symbol = 48
	svelteSymSvelteRawText            gotreesitter.Symbol = 49
	svelteSymSvelteRawTextEach        gotreesitter.Symbol = 50
	svelteSymSvelteRawTextSnippetArgs gotreesitter.Symbol = 51
	svelteSymAt                       gotreesitter.Symbol = 36
	svelteSymHash                     gotreesitter.Symbol = 17
	svelteSymSlash                    gotreesitter.Symbol = 24
	svelteSymColon                    gotreesitter.Symbol = 21
)

type svelteState struct {
	tags []htmlTag
}

// SvelteExternalScanner handles HTML tag tracking plus Svelte-specific
// raw text scanning (for expression blocks like {#each}, {@html}, etc.)
// and special sigil characters (@, #, /, :).
type SvelteExternalScanner struct{}

func (SvelteExternalScanner) Create() any { return &svelteState{} }
func (SvelteExternalScanner) Destroy(any) {}

func (SvelteExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*svelteState)
	return htmlSerializeTags(s.tags, buf)
}

func (SvelteExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*svelteState)
	s.tags = htmlDeserializeTagsInto(s.tags, buf)
}

// The shared tag encoding is exact and fail-closed, but Svelte's additional
// raw-text and expression-block behavior has not completed the scanner-wide
// checkpoint certification matrix. Changed edits remain conservative.
func (SvelteExternalScanner) SupportsIncrementalReuse() bool { return false }

func (SvelteExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*svelteState)
	lx := &goLexerAdapter{lexer}

	// Raw text in script/style bodies.
	if svelteValid(validSymbols, svelteTokRawText) &&
		!svelteValid(validSymbols, svelteTokStartTagName) &&
		!svelteValid(validSymbols, svelteTokEndTagName) {
		return htmlScanRawText(lx, s.tags, svelteSymRawText, lexer)
	}

	// Svelte raw text for snippet arguments (inside parentheses of #snippet).
	if svelteValid(validSymbols, svelteTokSvelteRawTextSnippetArgs) {
		return svelteScanRawTextSnippet(lx, lexer)
	}

	// Svelte raw text for expression blocks.
	if svelteValid(validSymbols, svelteTokSvelteRawText) ||
		svelteValid(validSymbols, svelteTokSvelteRawTextEach) {
		return svelteScanRawText(lx, lexer, validSymbols)
	}

	// Skip whitespace.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	switch lexer.Lookahead() {
	case '<':
		lexer.MarkEnd()
		lexer.Advance(false)

		if lexer.Lookahead() == '!' {
			lexer.Advance(false)
			return htmlScanComment(lx, svelteSymComment, lexer)
		}

		if svelteValid(validSymbols, svelteTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, svelteSymImplicitEndTag, lexer)
		}

	case '{', 0:
		// Svelte triggers implicit end tags on '{' (curly brace starts new
		// Svelte blocks) and on EOF, same as '<'.
		if svelteValid(validSymbols, svelteTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, svelteSymImplicitEndTag, lexer)
		}

	case '/':
		if svelteValid(validSymbols, svelteTokSelfClosingTagDelim) {
			return htmlScanSelfClosingDelim(lx, &s.tags, svelteSymSelfClosingTagDelim, lexer)
		}

	default:
		if (svelteValid(validSymbols, svelteTokStartTagName) || svelteValid(validSymbols, svelteTokEndTagName)) &&
			!svelteValid(validSymbols, svelteTokRawText) {
			if svelteValid(validSymbols, svelteTokStartTagName) {
				return htmlScanStartTagName(lx, &s.tags,
					svelteSymStartTagName,
					svelteSymScriptStartTagName,
					svelteSymStyleStartTagName,
					0, // no template symbol for Svelte
					lexer)
			}
			return htmlScanEndTagName(lx, &s.tags, svelteSymEndTagName, svelteSymErroneousEndTagName, lexer)
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Svelte raw text scanning
// ---------------------------------------------------------------------------

// svelteScanRawText scans the content of a Svelte expression block (e.g., the
// expression inside {#if ...}, {#each ... as ...}, {@html ...}, etc.).
// It balances braces, respects JS strings and comments, and stops at the
// closing unbalanced '}'. For the EACH variant, it also stops at "as" followed
// by whitespace.
func svelteScanRawText(lx htmlLexer, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// Skip leading whitespace.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	ch := lexer.Lookahead()

	// If one of the special sigils is both the current character and a valid
	// symbol, we bail out so that the sigil can be recognized as its own token.
	if (ch == '@' && svelteValid(validSymbols, svelteTokAt)) ||
		(ch == '#' && svelteValid(validSymbols, svelteTokHash)) ||
		(ch == ':' && svelteValid(validSymbols, svelteTokColon)) {
		return false
	}

	// Any sigil character at all disqualifies this as raw text.
	if ch == '@' || ch == '#' || ch == ':' {
		return false
	}

	advancedOnce := false

	// Handle leading '/' when SLASH is valid. We need to check if it starts
	// a JS block comment; if so, consume it. If it's a line-starting '//'
	// that's not a line comment, bail out.
	if ch == '/' && svelteValid(validSymbols, svelteTokSlash) {
		lx.advance(false)
		if lexer.Lookahead() == '*' {
			return svelteJSBlockComment(lx)
		}
		if lexer.Lookahead() != '/' { // not a JS comment
			return false
		}
		advancedOnce = true
	}

	isEach := svelteValid(validSymbols, svelteTokSvelteRawTextEach)
	if isEach {
		lexer.SetResultSymbol(svelteSymSvelteRawTextEach)
	} else {
		lexer.SetResultSymbol(svelteSymSvelteRawText)
	}

	braceLevel := 0

	for !lx.eof() {
		switch lexer.Lookahead() {
		case '/':
			lx.advance(false)
			advancedOnce = true
			if lexer.Lookahead() == '*' {
				svelteJSBlockComment(lx)
			} else if lexer.Lookahead() == '/' {
				svelteJSLineComment(lx)
			}

		case '\\':
			// Escape mode: advance past the escaped character.
			lx.advance(false)
			advancedOnce = true

		case '\'', '"':
			svelteJSQuotedString(lx, lexer.Lookahead())
			advancedOnce = true

		case '`':
			svelteJSTemplateString(lx)
			advancedOnce = true

		case '}':
			if braceLevel == 0 {
				lx.markEnd()
				return advancedOnce
			}
			lx.advance(false)
			braceLevel--
			advancedOnce = true

		case '{':
			lx.advance(false)
			braceLevel++
			advancedOnce = true

		case 'a':
			if isEach {
				lx.markEnd()
				lx.advance(false)
				advancedOnce = true
				if lexer.Lookahead() == 's' {
					lx.advance(false)
					if unicode.IsSpace(lexer.Lookahead()) {
						return advancedOnce
					}
				}
			} else {
				lx.advance(false)
				advancedOnce = true
			}

		default:
			lx.advance(false)
			advancedOnce = true
		}
	}

	return false
}

// svelteScanRawTextSnippet scans inside the parentheses of a #snippet
// definition, consuming everything until the next balanced closing ')'.
func svelteScanRawTextSnippet(lx htmlLexer, lexer *gotreesitter.ExternalLexer) bool {
	// Skip leading whitespace.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	lexer.SetResultSymbol(svelteSymSvelteRawTextSnippetArgs)
	parenLevel := 0
	advancedOnce := false

	for !lx.eof() {
		switch lexer.Lookahead() {
		case '/':
			lx.advance(false)
			if lexer.Lookahead() == '*' {
				svelteJSBlockComment(lx)
			} else if lexer.Lookahead() == '/' {
				svelteJSLineComment(lx)
			}

		case '\\':
			lx.advance(false)

		case '\'', '"':
			svelteJSQuotedString(lx, lexer.Lookahead())

		case '`':
			svelteJSTemplateString(lx)

		case ')':
			if parenLevel == 0 {
				lx.markEnd()
				return advancedOnce
			}
			lx.advance(false)
			parenLevel--

		case '(':
			lx.advance(false)
			parenLevel++

		default:
			lx.advance(false)
		}
		advancedOnce = true
	}

	return false
}

// ---------------------------------------------------------------------------
// JavaScript sub-scanners (used for balancing inside Svelte expressions)
// ---------------------------------------------------------------------------

// svelteJSBlockComment advances past a block comment. The lexer should be
// positioned right after the '/' with '*' as lookahead.
func svelteJSBlockComment(lx htmlLexer) bool {
	if lx.lookahead() != '*' {
		return false
	}
	lx.advance(false)
	for lx.lookahead() != 0 {
		if lx.lookahead() == '*' {
			lx.advance(false)
			if lx.lookahead() == '/' {
				lx.advance(false)
				return true
			}
		} else {
			lx.advance(false)
		}
	}
	return false
}

// svelteJSLineComment advances past a line comment. The lexer should be
// positioned right after the first '/' with the second '/' as lookahead.
func svelteJSLineComment(lx htmlLexer) bool {
	if lx.lookahead() != '/' {
		return false
	}
	lx.advance(false)
	for lx.lookahead() != 0 {
		if lx.lookahead() == '\n' || lx.lookahead() == '\r' {
			lx.advance(false)
			return true
		}
		lx.advance(false)
	}
	return false
}

// svelteJSBalancedBrace scans past a balanced pair of curly braces. The lexer
// should be positioned with '{' as lookahead.
func svelteJSBalancedBrace(lx htmlLexer) bool {
	if lx.lookahead() != '{' {
		return false
	}
	braceLevel := 0
	lx.advance(false)
	for lx.lookahead() != 0 {
		switch lx.lookahead() {
		case '`':
			svelteJSTemplateString(lx)
		case '\\':
			lx.advance(false)
			lx.advance(false)
		case '\'', '"':
			svelteJSQuotedString(lx, lx.lookahead())
		case '{':
			braceLevel++
			lx.advance(false)
		case '}':
			lx.advance(false)
			if braceLevel == 0 {
				return true
			}
			braceLevel--
		default:
			lx.advance(false)
		}
	}
	return false
}

// svelteJSQuotedString scans past a single- or double-quoted string.
func svelteJSQuotedString(lx htmlLexer, delimiter rune) bool {
	if lx.lookahead() != delimiter {
		return false
	}
	lx.advance(false)
	for lx.lookahead() != 0 {
		if lx.lookahead() == '\\' {
			lx.advance(false) // escape
			lx.advance(false)
		} else if lx.lookahead() == delimiter {
			lx.advance(false)
			return true
		} else {
			lx.advance(false)
		}
	}
	return false
}

// svelteJSTemplateString scans past a backtick-delimited template string,
// including ${} interpolations.
func svelteJSTemplateString(lx htmlLexer) bool {
	if lx.lookahead() != '`' {
		return false
	}
	lx.advance(false)
	for lx.lookahead() != 0 {
		switch lx.lookahead() {
		case '$':
			lx.advance(false)
			if lx.lookahead() == '{' {
				svelteJSBalancedBrace(lx)
			}
		case '\\':
			lx.advance(false)
			lx.advance(false)
		case '`':
			lx.advance(false)
			return true
		default:
			lx.advance(false)
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func svelteValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
