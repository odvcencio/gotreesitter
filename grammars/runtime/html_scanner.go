//go:build !grammar_subset || grammar_subset_html

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the HTML grammar.
// These must match the order in the grammar's externals array.
const (
	htmlTokStartTagName   = 0 // tag_name (start)
	htmlTokScriptTagName  = 1 // tag_name (script start)
	htmlTokStyleTagName   = 2 // tag_name (style start)
	htmlTokEndTagName     = 3 // tag_name (end)
	htmlTokErrEndName     = 4 // erroneous_end_tag_name
	htmlTokSelfClosingTag = 5 // />
	htmlTokImplicitEndTag = 6 // _implicit_end_tag
	htmlTokRawText        = 7 // raw_text
	htmlTokComment        = 8 // comment
)

// htmlScannerState holds the tag stack for the HTML scanner.
type htmlScannerState struct {
	tags []htmlTag
}

// HTMLExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-html.
//
// This is a Go port of the C external scanner from tree-sitter-html
// (https://github.com/tree-sitter/tree-sitter-html). It reuses the shared
// HTML scanning infrastructure (html_tags.go, blade_scanner.go) that is
// also used by the blade, svelte, vue, angular, and astro scanners.
type HTMLExternalScanner struct{}

func (HTMLExternalScanner) Create() any         { return &htmlScannerState{} }
func (HTMLExternalScanner) Destroy(payload any) {}

func (HTMLExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*htmlScannerState)
	return htmlSerializeTags(s.tags, buf)
}

func (HTMLExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*htmlScannerState)
	s.tags = htmlDeserializeTagsInto(s.tags, buf)
}

// SupportsIncrementalReuse certifies HTML's scanner state for changed edits.
func (HTMLExternalScanner) SupportsIncrementalReuse() bool { return true }

// SupportsIncrementalReuseFromErrorTree remains closed until HTML recovery
// ownership is certified independently of the scanner's exact state proof.
func (HTMLExternalScanner) SupportsIncrementalReuseFromErrorTree() bool { return false }

// UsesExternalScannerCheckpoints selects exact tag-stack checkpoints. States
// that do not fit the runtime buffer serialize to zero and fail closed.
func (HTMLExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

// PreservesStateOnScanFailure holds because tag-stack mutations occur only on
// the successful implicit-end, self-closing, start-tag, and end-tag routes.
func (HTMLExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (HTMLExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*htmlScannerState)
	lx := &goLexerAdapter{lexer}
	lang := loadEmbeddedLanguage("html.bin")

	// Resolve concrete symbol IDs from the grammar's ExternalSymbols table.
	startSym := lang.ExternalSymbols[htmlTokStartTagName]
	scriptSym := lang.ExternalSymbols[htmlTokScriptTagName]
	styleSym := lang.ExternalSymbols[htmlTokStyleTagName]
	endSym := lang.ExternalSymbols[htmlTokEndTagName]
	errEndSym := lang.ExternalSymbols[htmlTokErrEndName]
	selfClosingSym := lang.ExternalSymbols[htmlTokSelfClosingTag]
	implicitEndSym := lang.ExternalSymbols[htmlTokImplicitEndTag]
	rawTextSym := lang.ExternalSymbols[htmlTokRawText]
	commentSym := lang.ExternalSymbols[htmlTokComment]

	// Raw text mode: inside <script> or <style>, scan until closing tag.
	if htmlV(validSymbols, htmlTokRawText) &&
		!htmlV(validSymbols, htmlTokStartTagName) &&
		!htmlV(validSymbols, htmlTokEndTagName) {
		return htmlScanRawText(lx, s.tags, rawTextSym, lexer)
	}

	// Skip whitespace.
	for {
		ch := lx.lookahead()
		if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
			break
		}
		lx.advance(true)
	}

	switch lx.lookahead() {
	case '<':
		lx.markEnd()
		lx.advance(false)

		if lx.lookahead() == '!' {
			lx.advance(false)
			return htmlScanComment(lx, commentSym, lexer)
		}

		if htmlV(validSymbols, htmlTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, implicitEndSym, lexer)
		}

	case 0: // EOF
		if htmlV(validSymbols, htmlTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, implicitEndSym, lexer)
		}

	case '/':
		if htmlV(validSymbols, htmlTokSelfClosingTag) {
			return htmlScanSelfClosingDelim(lx, &s.tags, selfClosingSym, lexer)
		}

	default:
		if (htmlV(validSymbols, htmlTokStartTagName) || htmlV(validSymbols, htmlTokEndTagName)) &&
			!htmlV(validSymbols, htmlTokRawText) {
			if htmlV(validSymbols, htmlTokStartTagName) {
				return htmlScanStartTagName(lx, &s.tags, startSym, scriptSym, styleSym, 0, lexer)
			}
			return htmlScanEndTagName(lx, &s.tags, endSym, errEndSym, lexer)
		}
	}

	return false
}

func htmlV(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
