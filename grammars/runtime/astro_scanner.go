//go:build !grammar_subset || grammar_subset_astro

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Astro grammar (ext[i] positions).
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
)

// Symbol IDs corresponding to the grammar's node types.
const (
	astroSymStartTagName         gotreesitter.Symbol = 21
	astroSymScriptStartTagName   gotreesitter.Symbol = 22
	astroSymStyleStartTagName    gotreesitter.Symbol = 23
	astroSymEndTagName           gotreesitter.Symbol = 24
	astroSymErroneousEndTagName  gotreesitter.Symbol = 25
	astroSymSelfClosingTagDelim  gotreesitter.Symbol = 6
	astroSymImplicitEndTag       gotreesitter.Symbol = 26
	astroSymRawText              gotreesitter.Symbol = 27
	astroSymComment              gotreesitter.Symbol = 28
	astroSymInterpolationStart   gotreesitter.Symbol = 29
	astroSymInterpolationEnd     gotreesitter.Symbol = 30
	astroSymFrontmatterJSBlock   gotreesitter.Symbol = 31
	astroSymAttributeJSExpr      gotreesitter.Symbol = 32
	astroSymAttributeBacktickStr gotreesitter.Symbol = 33
	astroSymPermissibleText      gotreesitter.Symbol = 34
	astroSymFragmentTagDelim     gotreesitter.Symbol = 35
)

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
type AstroExternalScanner struct{}

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

func (AstroExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*astroState)
	lx := &goLexerAdapter{lexer}

	// --- FRONTMATTER_JS_BLOCK ---
	// Only valid when the tag stack is empty (we are at the top of the file,
	// before any HTML has been opened).
	if astroValid(validSymbols, astroTokFrontmatterJSBlock) && len(s.tags) == 0 {
		astroScanJSExprFrontmatter(lx)
		lexer.SetResultSymbol(astroSymFrontmatterJSBlock)
		return true
	}

	// --- RAW_TEXT (script/style bodies) ---
	if astroValid(validSymbols, astroTokRawText) &&
		!astroValid(validSymbols, astroTokStartTagName) &&
		!astroValid(validSymbols, astroTokEndTagName) {
		return htmlScanRawText(lx, s.tags, astroSymRawText, lexer)
	}

	// --- ATTRIBUTE_JS_EXPR ---
	if astroValid(validSymbols, astroTokAttributeJSExpr) {
		astroScanJSExprCurly(lx)
		lexer.SetResultSymbol(astroSymAttributeJSExpr)
		return true
	}

	// --- PERMISSIBLE_TEXT (when lookahead is whitespace) ---
	if astroValid(validSymbols, astroTokPermissibleText) {
		if unicode.IsSpace(lexer.Lookahead()) {
			// Can't be anything else.
			return astroScanPermissibleText(lx, lexer)
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
			return htmlScanComment(lx, astroSymComment, lexer)
		}

		if astroValid(validSymbols, astroTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, astroSymImplicitEndTag, lexer)
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
			return htmlScanImplicitEndTag(lx, &s.tags, astroSymImplicitEndTag, lexer)
		}

	case '/':
		if astroValid(validSymbols, astroTokSelfClosingTagDelim) {
			return htmlScanSelfClosingDelim(lx, &s.tags, astroSymSelfClosingTagDelim, lexer)
		}

	case '{':
		if astroValid(validSymbols, astroTokInterpolationStart) {
			lexer.Advance(false)
			s.tags = append(s.tags, htmlTag{tagType: htmlTagInterpolation})
			lexer.SetResultSymbol(astroSymInterpolationStart)
			return true
		}

	case '}':
		// Close any void tags before exiting the interpolation node.
		if astroValid(validSymbols, astroTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, astroSymImplicitEndTag, lexer)
		}

		if astroValid(validSymbols, astroTokInterpolationEnd) &&
			len(s.tags) > 0 &&
			s.tags[len(s.tags)-1].tagType == htmlTagInterpolation {
			lexer.Advance(false)
			s.tags = s.tags[:len(s.tags)-1]
			lexer.SetResultSymbol(astroSymInterpolationEnd)
			return true
		}

	case '`':
		if astroValid(validSymbols, astroTokAttributeBacktickStr) {
			astroScanBacktickString(lx)
			lx.markEnd()
			lexer.SetResultSymbol(astroSymAttributeBacktickStr)
			return true
		}

	default:
		if (astroValid(validSymbols, astroTokStartTagName) || astroValid(validSymbols, astroTokEndTagName)) &&
			!astroValid(validSymbols, astroTokRawText) {
			if astroValid(validSymbols, astroTokStartTagName) {
				return astroScanStartTagName(s, lx, lexer)
			}
			return astroScanEndTagName(s, lx, lexer)
		}
	}

	if !definitelyNotPermissibleText && astroValid(validSymbols, astroTokPermissibleText) {
		return astroScanPermissibleText(lx, lexer)
	}

	return false
}

// ---------------------------------------------------------------------------
// Astro-specific start/end tag scanning (with fragment support)
// ---------------------------------------------------------------------------

func astroScanStartTagName(s *astroState, lx htmlLexer, lexer *gotreesitter.ExternalLexer) bool {
	tagName := htmlScanTagName(lx)
	if len(tagName) == 0 {
		// Fragment tags don't contain spaces.
		if lx.lookahead() == '>' {
			lx.advance(false)
			s.tags = append(s.tags, htmlTag{tagType: htmlTagCustom, customName: astroFragmentName})
			lexer.SetResultSymbol(astroSymFragmentTagDelim)
			return true
		}
		return false
	}

	tag := htmlTagForName(tagName)
	s.tags = append(s.tags, tag)
	switch tag.tagType {
	case htmlTagScript:
		lexer.SetResultSymbol(astroSymScriptStartTagName)
	case htmlTagStyle:
		lexer.SetResultSymbol(astroSymStyleStartTagName)
	default:
		lexer.SetResultSymbol(astroSymStartTagName)
	}
	return true
}

func astroScanEndTagName(s *astroState, lx htmlLexer, lexer *gotreesitter.ExternalLexer) bool {
	tagName := htmlScanTagName(lx)
	if len(tagName) == 0 {
		if lx.lookahead() == '>' {
			lx.advance(false)
			if len(s.tags) > 0 && astroIsFragment(&s.tags[len(s.tags)-1]) {
				s.tags = s.tags[:len(s.tags)-1]
				lexer.SetResultSymbol(astroSymFragmentTagDelim)
				return true
			}
			lexer.SetResultSymbol(astroSymErroneousEndTagName)
			return true
		}
		return false
	}

	tag := htmlTagForName(tagName)
	if len(s.tags) > 0 && htmlTagEq(&s.tags[len(s.tags)-1], &tag) {
		s.tags = s.tags[:len(s.tags)-1]
		lexer.SetResultSymbol(astroSymEndTagName)
	} else {
		lexer.SetResultSymbol(astroSymErroneousEndTagName)
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

func astroScanPermissibleText(lx htmlLexer, lexer *gotreesitter.ExternalLexer) bool {
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
		lexer.SetResultSymbol(astroSymPermissibleText)
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
