//go:build !grammar_subset || grammar_subset_svelte

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Svelte grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner
// never hardcodes them -- see svelteDefaultSymTable below.
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
	svelteTokenCount                  = 16
)

// svelteDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped svelte.bin assigns to each external, in svelteTok*
// order. It exists only as a pre-bind fallback (and as an independent value
// to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order. The four tag_name variants (start/script/style/end) each
// alias to the same visible "tag_name" node type; the four sigil externals
// (/>, @, #, /, :) each share a Symbol ID with every other occurrence of
// that literal elsewhere in the grammar, exactly like blade's "/>" external.
var svelteDefaultSymTable = [svelteTokenCount]gotreesitter.Symbol{
	41, // _start_tag_name (display: tag_name)
	42, // _script_start_tag_name (display: tag_name)
	43, // _style_start_tag_name (display: tag_name)
	44, // _end_tag_name (display: tag_name)
	45, // erroneous_end_tag_name
	6,  // "/>" (display: "/>")
	46, // _implicit_end_tag
	47, // raw_text
	48, // comment
	49, // svelte_raw_text
	50, // svelte_raw_text_each (display: svelte_raw_text)
	51, // svelte_raw_text_snippet_arguments (display: svelte_raw_text)
	36, // "@" (display: "@")
	17, // "#" (display: "#")
	24, // "/" (display: "/")
	21, // ":" (display: ":")
}

// svelteExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (svelteTok* order).
var svelteExternalScannerSpec = ExternalScannerSpec{
	Language:       "svelte",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-svelte",
	UpstreamCommit: "ae5199db47757f785e43a14b332118a5474de1a2",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "3d968be73671924e39680bd68790b76937b5002688d5cec5e243d70780f510a5"},
		{Path: "src/scanner.c", SHA256: "878988200a5e2c77cc822cc0a2c69f4fc593ba76a734a6a36b9370d7501b09ce"},
	},
	Externals: []string{
		"_start_tag_name",
		"_script_start_tag_name",
		"_style_start_tag_name",
		"_end_tag_name",
		"erroneous_end_tag_name",
		"/>",
		"_implicit_end_tag",
		"raw_text",
		"comment",
		"svelte_raw_text",
		"svelte_raw_text_each",
		"svelte_raw_text_snippet_arguments",
		"@",
		"#",
		"/",
		":",
	},
}

func init() {
	RegisterExternalScannerSpec(svelteExternalScannerSpec)
}

type svelteState struct {
	tags []htmlTag
}

// SvelteExternalScanner handles HTML tag tracking plus Svelte-specific
// raw text scanning (for expression blocks like {#each}, {@html}, etc.)
// and special sigil characters (@, #, /, :).
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type SvelteExternalScanner struct {
	symbols         [svelteTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers svelte's external symbols.
func (SvelteExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := SvelteExternalScanner{symbols: svelteDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, svelteExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s SvelteExternalScanner) symbolTable() *[svelteTokenCount]gotreesitter.Symbol {
	if s.symbols == ([svelteTokenCount]gotreesitter.Symbol{}) {
		return &svelteDefaultSymTable
	}
	return &s.symbols
}

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

func (s SvelteExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*svelteState)
	lx := &goLexerAdapter{lexer}

	if len(s.externalToToken) > 0 {
		var semanticValid [svelteTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < svelteTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	// Raw text in script/style bodies.
	if svelteValid(validSymbols, svelteTokRawText) &&
		!svelteValid(validSymbols, svelteTokStartTagName) &&
		!svelteValid(validSymbols, svelteTokEndTagName) {
		return htmlScanRawText(lx, state.tags, syms[svelteTokRawText], lexer)
	}

	// Svelte raw text for snippet arguments (inside parentheses of #snippet).
	if svelteValid(validSymbols, svelteTokSvelteRawTextSnippetArgs) {
		return svelteScanRawTextSnippet(lx, lexer, syms[svelteTokSvelteRawTextSnippetArgs])
	}

	// Svelte raw text for expression blocks.
	if svelteValid(validSymbols, svelteTokSvelteRawText) ||
		svelteValid(validSymbols, svelteTokSvelteRawTextEach) {
		return svelteScanRawText(lx, lexer, validSymbols, syms[svelteTokSvelteRawText], syms[svelteTokSvelteRawTextEach])
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
			return htmlScanComment(lx, syms[svelteTokComment], lexer)
		}

		if svelteValid(validSymbols, svelteTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &state.tags, syms[svelteTokImplicitEndTag], lexer)
		}

	case '{', 0:
		// Svelte triggers implicit end tags on '{' (curly brace starts new
		// Svelte blocks) and on EOF, same as '<'.
		if svelteValid(validSymbols, svelteTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &state.tags, syms[svelteTokImplicitEndTag], lexer)
		}

	case '/':
		if svelteValid(validSymbols, svelteTokSelfClosingTagDelim) {
			return htmlScanSelfClosingDelim(lx, &state.tags, syms[svelteTokSelfClosingTagDelim], lexer)
		}

	default:
		if (svelteValid(validSymbols, svelteTokStartTagName) || svelteValid(validSymbols, svelteTokEndTagName)) &&
			!svelteValid(validSymbols, svelteTokRawText) {
			if svelteValid(validSymbols, svelteTokStartTagName) {
				return htmlScanStartTagName(lx, &state.tags,
					syms[svelteTokStartTagName],
					syms[svelteTokScriptStartTagName],
					syms[svelteTokStyleStartTagName],
					0, // no template symbol for Svelte
					lexer)
			}
			return htmlScanEndTagName(lx, &state.tags, syms[svelteTokEndTagName], syms[svelteTokErroneousEndTagName], lexer)
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
func svelteScanRawText(lx htmlLexer, lexer *gotreesitter.ExternalLexer, validSymbols []bool, rawTextSym, rawTextEachSym gotreesitter.Symbol) bool {
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
		lexer.SetResultSymbol(rawTextEachSym)
	} else {
		lexer.SetResultSymbol(rawTextSym)
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
func svelteScanRawTextSnippet(lx htmlLexer, lexer *gotreesitter.ExternalLexer, rawTextSnippetArgsSym gotreesitter.Symbol) bool {
	// Skip leading whitespace.
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	lexer.SetResultSymbol(rawTextSnippetArgsSym)
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
