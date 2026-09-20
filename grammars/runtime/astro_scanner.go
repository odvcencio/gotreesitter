//go:build !grammar_subset || grammar_subset_astro

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Astro grammar (ext[i] positions). This is
// the external index (the position of the token in the grammar's
// `externals: [...]` list), which is exactly what tree-sitter's
// `valid_symbols` array and C's result_symbol enum are indexed by. The
// external index is stable across a blob regen as long as the externals
// list itself does not reorder; concrete numeric gotreesitter.Symbol IDs
// are NOT stable (they shift whenever the grammar's total symbol count
// changes), so this scanner never hardcodes them -- see
// astroDefaultSymTable below.
const (
	astroTokStartTagName         = 0  // START_TAG_NAME
	astroTokScriptStartTagName   = 1  // SCRIPT_START_TAG_NAME
	astroTokStyleStartTagName    = 2  // STYLE_START_TAG_NAME
	astroTokEndTagName           = 3  // END_TAG_NAME
	astroTokErroneousEndTagName  = 4  // ERRONEOUS_END_TAG_NAME
	astroTokSelfClosingTagDelim  = 5  // />
	astroTokImplicitEndTag       = 6  // IMPLICIT_END_TAG
	astroTokRawText              = 7  // RAW_TEXT
	astroTokComment              = 8  // COMMENT
	astroTokInterpolationStart   = 9  // {
	astroTokInterpolationEnd     = 10 // }
	astroTokFrontmatterJSBlock   = 11 // FRONTMATTER_JS_BLOCK
	astroTokAttributeJSExpr      = 12 // ATTRIBUTE_JS_EXPR
	astroTokAttributeBacktickStr = 13 // ATTRIBUTE_BACKTICK_STRING
	astroTokPermissibleText      = 14 // PERMISSIBLE_TEXT
	astroTokFragmentTagDelim     = 15 // > (fragment)
	astroTokenCount              = 16
)

// astroDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped astro.bin assigns to each external, in astroTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order. The three tag_name start variants and the end tag_name each
// alias to a shared "tag_name" node type, and the self-closing-tag
// delimiter shares a Symbol ID with every other occurrence of "/>"
// elsewhere in the grammar, exactly like blade's "/>" external.
var astroDefaultSymTable = [astroTokenCount]gotreesitter.Symbol{
	21, // _start_tag_name (display: tag_name)
	22, // _script_start_tag_name (display: tag_name)
	23, // _style_start_tag_name (display: tag_name)
	24, // _end_tag_name (display: tag_name)
	25, // erroneous_end_tag_name
	6,  // "/>" (display: "/>")
	26, // _implicit_end_tag
	27, // raw_text
	28, // comment
	29, // _html_interpolation_start
	30, // _html_interpolation_end
	31, // frontmatter_js_block
	32, // attribute_js_expr
	33, // attribute_backtick_string
	34, // permissible_text
	35, // _fragment_tag_delim
}

// astroExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (astroTok* order).
var astroExternalScannerSpec = ExternalScannerSpec{
	Language:       "astro",
	UpstreamRepo:   "https://github.com/virchau13/tree-sitter-astro",
	UpstreamCommit: "213f6e6973d9b456c6e50e86f19f66877e7ef0ee",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "ff7427dcba7238584b9043d57a198760e135720f2e00403e96d3934fcefb37e3"},
		{Path: "src/scanner.c", SHA256: "9d7233e8ecdf695c849eb7c48c724ec8b26f06bc1543f499cc0ae95e28df6817"},
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
		"_html_interpolation_start",
		"_html_interpolation_end",
		"frontmatter_js_block",
		"attribute_js_expr",
		"attribute_backtick_string",
		"permissible_text",
		"_fragment_tag_delim",
	},
}

func init() {
	RegisterExternalScannerSpec(astroExternalScannerSpec)
}

// astroFragmentName is the sentinel custom-tag name used to represent
// Astro's fragment tag (<> </>) on the tag stack.  We use htmlTagCustom
// with this name because the shared htmlTagType enum doesn't have a
// dedicated FRAGMENT constant.
const astroFragmentName = "__ASTRO_FRAGMENT__"

type astroState struct {
	tags []htmlTag
}

// AstroExternalScanner handles Astro-specific external scanning:
// HTML tag tracking, fragment tags, frontmatter, JS expressions,
// backtick strings, interpolation, and permissible text.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type AstroExternalScanner struct {
	symbols         [astroTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers astro's external symbols.
func (AstroExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := AstroExternalScanner{symbols: astroDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, astroExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s AstroExternalScanner) symbolTable() *[astroTokenCount]gotreesitter.Symbol {
	if s.symbols == ([astroTokenCount]gotreesitter.Symbol{}) {
		return &astroDefaultSymTable
	}
	return &s.symbols
}

func (AstroExternalScanner) Create() any { return &astroState{} }
func (AstroExternalScanner) Destroy(any) {}

func (AstroExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*astroState)
	return htmlSerializeTags(s.tags, buf)
}

func (AstroExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*astroState)
	s.tags = htmlDeserializeTagsInto(s.tags, buf)
}

func (s AstroExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*astroState)
	lx := &goLexerAdapter{lexer}

	if len(s.externalToToken) > 0 {
		var semanticValid [astroTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < astroTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	// --- FRONTMATTER_JS_BLOCK ---
	// Only valid when the tag stack is empty (we are at the top of the file,
	// before any HTML has been opened).
	if astroValid(validSymbols, astroTokFrontmatterJSBlock) && len(state.tags) == 0 {
		astroScanJSExprFrontmatter(lx)
		lexer.SetResultSymbol(syms[astroTokFrontmatterJSBlock])
		return true
	}

	// --- RAW_TEXT (script/style bodies) ---
	if astroValid(validSymbols, astroTokRawText) &&
		!astroValid(validSymbols, astroTokStartTagName) &&
		!astroValid(validSymbols, astroTokEndTagName) {
		return htmlScanRawText(lx, state.tags, syms[astroTokRawText], lexer)
	}

	// --- ATTRIBUTE_JS_EXPR ---
	if astroValid(validSymbols, astroTokAttributeJSExpr) {
		astroScanJSExprCurly(lx)
		lexer.SetResultSymbol(syms[astroTokAttributeJSExpr])
		return true
	}

	// --- PERMISSIBLE_TEXT (when lookahead is whitespace) ---
	if astroValid(validSymbols, astroTokPermissibleText) {
		if unicode.IsSpace(lexer.Lookahead()) {
			// Can't be anything else.
			return astroScanPermissibleText(lx, lexer, syms[astroTokPermissibleText])
		}
	} else {
		// Skip whitespace when permissible_text is not expected.
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
	}

	definitelyNotPermissibleText := false

	switch lexer.Lookahead() {
	case '<':
		lexer.MarkEnd()
		lexer.Advance(false)

		if lexer.Lookahead() == '!' {
			lexer.Advance(false)
			return htmlScanComment(lx, syms[astroTokComment], lexer)
		}

		if astroValid(validSymbols, astroTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &state.tags, syms[astroTokImplicitEndTag], lexer)
		}

		if astroValid(validSymbols, astroTokPermissibleText) {
			ch := lexer.Lookahead()
			invalid := astroIsASCIIAlpha(ch) ||
				ch == '/' ||
				ch == '?' ||
				ch == '>'
			if invalid {
				definitelyNotPermissibleText = true
			}
		}

	case 0:
		definitelyNotPermissibleText = true
		if astroValid(validSymbols, astroTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &state.tags, syms[astroTokImplicitEndTag], lexer)
		}

	case '/':
		if astroValid(validSymbols, astroTokSelfClosingTagDelim) {
			return htmlScanSelfClosingDelim(lx, &state.tags, syms[astroTokSelfClosingTagDelim], lexer)
		}

	case '{':
		if astroValid(validSymbols, astroTokInterpolationStart) {
			lexer.Advance(false)
			state.tags = append(state.tags, htmlTag{tagType: htmlTagInterpolation})
			lexer.SetResultSymbol(syms[astroTokInterpolationStart])
			return true
		}

	case '}':
		// Close any void tags before exiting the interpolation node.
		if astroValid(validSymbols, astroTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &state.tags, syms[astroTokImplicitEndTag], lexer)
		}

		if astroValid(validSymbols, astroTokInterpolationEnd) &&
			len(state.tags) > 0 &&
			state.tags[len(state.tags)-1].tagType == htmlTagInterpolation {
			lexer.Advance(false)
			state.tags = state.tags[:len(state.tags)-1]
			lexer.SetResultSymbol(syms[astroTokInterpolationEnd])
			return true
		}

	case '`':
		if astroValid(validSymbols, astroTokAttributeBacktickStr) {
			astroScanBacktickString(lx)
			lx.markEnd()
			lexer.SetResultSymbol(syms[astroTokAttributeBacktickStr])
			return true
		}

	default:
		if (astroValid(validSymbols, astroTokStartTagName) || astroValid(validSymbols, astroTokEndTagName)) &&
			!astroValid(validSymbols, astroTokRawText) {
			if astroValid(validSymbols, astroTokStartTagName) {
				return astroScanStartTagName(state, lx, lexer, syms)
			}
			return astroScanEndTagName(state, lx, lexer, syms)
		}
	}

	if !definitelyNotPermissibleText && astroValid(validSymbols, astroTokPermissibleText) {
		return astroScanPermissibleText(lx, lexer, syms[astroTokPermissibleText])
	}

	return false
}

// ---------------------------------------------------------------------------
// Astro-specific start/end tag scanning (with fragment support)
// ---------------------------------------------------------------------------

func astroScanStartTagName(s *astroState, lx htmlLexer, lexer *gotreesitter.ExternalLexer, syms *[astroTokenCount]gotreesitter.Symbol) bool {
	tagName := htmlScanTagName(lx)
	if len(tagName) == 0 {
		// Fragment tags don't contain spaces.
		if lx.lookahead() == '>' {
			lx.advance(false)
			s.tags = append(s.tags, htmlTag{tagType: htmlTagCustom, customName: astroFragmentName})
			lexer.SetResultSymbol(syms[astroTokFragmentTagDelim])
			return true
		}
		return false
	}

	tag := htmlTagForName(tagName)
	s.tags = append(s.tags, tag)
	switch tag.tagType {
	case htmlTagScript:
		lexer.SetResultSymbol(syms[astroTokScriptStartTagName])
	case htmlTagStyle:
		lexer.SetResultSymbol(syms[astroTokStyleStartTagName])
	default:
		lexer.SetResultSymbol(syms[astroTokStartTagName])
	}
	return true
}

func astroScanEndTagName(s *astroState, lx htmlLexer, lexer *gotreesitter.ExternalLexer, syms *[astroTokenCount]gotreesitter.Symbol) bool {
	tagName := htmlScanTagName(lx)
	if len(tagName) == 0 {
		if lx.lookahead() == '>' {
			lx.advance(false)
			if len(s.tags) > 0 && astroIsFragment(&s.tags[len(s.tags)-1]) {
				s.tags = s.tags[:len(s.tags)-1]
				lexer.SetResultSymbol(syms[astroTokFragmentTagDelim])
				return true
			}
			lexer.SetResultSymbol(syms[astroTokErroneousEndTagName])
			return true
		}
		return false
	}

	tag := htmlTagForName(tagName)
	if len(s.tags) > 0 && htmlTagEq(&s.tags[len(s.tags)-1], &tag) {
		s.tags = s.tags[:len(s.tags)-1]
		lexer.SetResultSymbol(syms[astroTokEndTagName])
	} else {
		lexer.SetResultSymbol(syms[astroTokErroneousEndTagName])
	}
	return true
}

// ---------------------------------------------------------------------------
// JS expression scanners
// ---------------------------------------------------------------------------

// astroJSCommentState tracks whether the JS scanner is inside a comment.
type astroJSCommentState int

const (
	astroNotInComment astroJSCommentState = iota
	astroSingleLine
	astroMultiLine
)

// astroScanJSExprFrontmatter scans a JS block until the closing "\n---"
// delimiter is found.  The delimiter itself is NOT consumed.
func astroScanJSExprFrontmatter(lx htmlLexer) {
	lx.markEnd()
	// We start with delimiterIndex = 1 because tree-sitter has already
	// parsed "---\n" and hands us the lexer right after that newline.
	// Index 1 means we have already "seen" the leading '\n'.
	delimiterIndex := 1
	inComment := astroNotInComment

	const endDelim = "\n---"

	for lx.lookahead() != 0 {
		if inComment == astroNotInComment {
			// Pre-emptively mark_end when we are at index 0 so the
			// returned token ends just before the delimiter.
			if delimiterIndex == 0 {
				lx.markEnd()
			}

			ch := lx.lookahead()
			if ch == rune(endDelim[delimiterIndex]) {
				delimiterIndex++
				if delimiterIndex == len(endDelim) {
					break
				}
			} else {
				lx.markEnd()
				if ch == '\n' {
					delimiterIndex = 1
				} else {
					delimiterIndex = 0
				}
			}

			if ch == '"' || ch == '\'' || ch == '`' {
				astroScanJSString(lx)
				continue
			}
			if ch == '/' {
				lx.advance(false)
				next := lx.lookahead()
				if next == '/' {
					inComment = astroSingleLine
				} else if next == '*' {
					inComment = astroMultiLine
				}
				continue
			}
		} else if inComment == astroSingleLine {
			if lx.lookahead() == '\n' {
				inComment = astroNotInComment
				delimiterIndex = 1
				lx.markEnd()
			}
		} else if inComment == astroMultiLine {
			if lx.lookahead() == '*' {
				lx.advance(false)
				if lx.lookahead() == '/' {
					inComment = astroNotInComment
					delimiterIndex = 0
				} else {
					continue
				}
			}
		}
		lx.advance(false)
	}
}

// astroScanJSExprCurly scans a JS expression until an unbalanced closing '}'
// is found.  Braces inside strings and comments are properly balanced.
func astroScanJSExprCurly(lx htmlLexer) {
	lx.markEnd()
	curlyCount := 0
	inComment := astroNotInComment

	for lx.lookahead() != 0 {
		if inComment == astroNotInComment {
			lx.markEnd()
			ch := lx.lookahead()

			if ch == '{' {
				curlyCount++
			} else if ch == '}' {
				if curlyCount == 0 {
					lx.markEnd()
					break
				}
				curlyCount--
			}

			if ch == '"' || ch == '\'' || ch == '`' {
				astroScanJSString(lx)
				continue
			}
			if ch == '/' {
				lx.advance(false)
				next := lx.lookahead()
				if next == '/' {
					inComment = astroSingleLine
				} else if next == '*' {
					inComment = astroMultiLine
				}
				continue
			}
		} else if inComment == astroSingleLine {
			if lx.lookahead() == '\n' {
				inComment = astroNotInComment
			}
		} else if inComment == astroMultiLine {
			if lx.lookahead() == '*' {
				lx.advance(false)
				if lx.lookahead() == '/' {
					inComment = astroNotInComment
				} else {
					continue
				}
			}
		}
		lx.advance(false)
	}
}

// ---------------------------------------------------------------------------
// JS string helpers
// ---------------------------------------------------------------------------

// astroScanBacktickString scans a template-literal string (backtick),
// including ${} interpolations (which may nest JS expressions).
func astroScanBacktickString(lx htmlLexer) {
	// Advance past the opening backtick.
	lx.advance(false)
	for lx.lookahead() != 0 {
		ch := lx.lookahead()
		if ch == '$' {
			lx.advance(false)
			if lx.lookahead() == '{' {
				lx.advance(false)
				// Recursively scan the interpolation body until '}'.
				astroScanJSExprCurly(lx)
				// The curly scanner stops before '}', but we still
				// need to advance past it.
				// Actually, looking at the C code, scan_js_expr_with_delimiter
				// for EndCurly breaks when it finds an unbalanced '}', but does
				// NOT advance past it.  The comment in scan_js_backtick_string
				// says "Advance past the final curly" — but the advance happens
				// in the outer loop's lx.advance at the bottom.  However, the
				// C code has an explicit "continue" after the call, meaning it
				// goes back to the top of the while loop.  Let's follow that:
				// we do NOT advance here; the next iteration will see '}' and
				// fall through to the bottom advance.
			} else {
				// Reprocess this character.
				continue
			}
		} else if ch == '`' {
			// End of string.
			lx.advance(false)
			break
		}
		lx.advance(false)
	}
}

// astroScanJSString scans a string literal starting at the current
// lookahead character.  Handles backtick, single-quote, and double-quote.
func astroScanJSString(lx htmlLexer) {
	ch := lx.lookahead()
	if ch == '`' {
		astroScanBacktickString(lx)
		return
	}
	endChar := ch
	lx.advance(false)
	for lx.lookahead() != 0 {
		if lx.lookahead() == '\\' {
			lx.advance(false) // skip escape
		} else if lx.lookahead() == endChar {
			lx.advance(false)
			return
		}
		lx.advance(false)
	}
}

// ---------------------------------------------------------------------------
// Permissible text scanner
// ---------------------------------------------------------------------------

func astroScanPermissibleText(lx htmlLexer, lexer *gotreesitter.ExternalLexer, permissibleTextSym gotreesitter.Symbol) bool {
	thereIsText := false

	for lx.lookahead() != 0 {
		ch := lx.lookahead()

		if ch == '{' || ch == '}' {
			break
		}

		if ch == '\'' || ch == '"' || ch == '`' {
			astroScanJSString(lx)
			thereIsText = true
			lx.markEnd()
			continue
		}

		if ch == '/' {
			lx.advance(false)
			thereIsText = true
			if lx.lookahead() == '/' {
				// Single-line comment — consume until EOL.
				for lx.lookahead() != '\r' && lx.lookahead() != '\n' && lx.lookahead() != 0 {
					lx.advance(false)
				}
			}
			if lx.lookahead() == '*' {
				// Multi-line comment.
				for lx.lookahead() != 0 {
					lx.advance(false)
					if lx.lookahead() == '*' {
						lx.advance(false)
						if lx.lookahead() == '/' {
							lx.advance(false)
							break
						}
					}
				}
			}
			thereIsText = true
			lx.markEnd()
			continue
		}

		if ch == '<' {
			lx.advance(false)
			next := lx.lookahead()
			if astroIsASCIIAlpha(next) {
				break
			}
			if next == '/' {
				break
			}
			if next == '?' {
				break
			}
			if next == '>' {
				break
			}
			// None of the tag-like conditions matched — there's text here.
			thereIsText = true
			lx.markEnd()
			continue
		}

		lx.advance(false)
		thereIsText = true
		lx.markEnd()
	}

	if thereIsText {
		lexer.SetResultSymbol(permissibleTextSym)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Fragment helpers
// ---------------------------------------------------------------------------

func astroIsFragment(tag *htmlTag) bool {
	return tag.tagType == htmlTagCustom && tag.customName == astroFragmentName
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func astroValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

func astroIsASCIIAlpha(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}
