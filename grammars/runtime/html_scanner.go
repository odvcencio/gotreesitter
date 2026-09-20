//go:build !grammar_subset || grammar_subset_html

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the HTML grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see htmlDefaultSymTable below.
const (
	htmlTokStartTagName   = 0 // _start_tag_name (display: tag_name)
	htmlTokScriptTagName  = 1 // _script_start_tag_name (display: tag_name)
	htmlTokStyleTagName   = 2 // _style_start_tag_name (display: tag_name)
	htmlTokEndTagName     = 3 // _end_tag_name (display: tag_name)
	htmlTokErrEndName     = 4 // erroneous_end_tag_name
	htmlTokSelfClosingTag = 5 // /> (self-closing tag delimiter, a literal-string external)
	htmlTokImplicitEndTag = 6 // _implicit_end_tag
	htmlTokRawText        = 7 // raw_text
	htmlTokComment        = 8 // comment
	htmlTokenCount        = 9
)

// htmlDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped html.bin assigns to each external, in htmlTok* order. It
// exists only as a pre-bind fallback (and as an independent value to compare
// a real bind against in tests); ExternalScannerForLanguage below overwrites
// it with values read from the actual loaded Language at bind time, which is
// what the scanner must do to survive a future blob regen that renumbers
// absolute symbol IDs without touching the externals list order.
var htmlDefaultSymTable = [htmlTokenCount]gotreesitter.Symbol{
	17, // _start_tag_name (display: tag_name)
	18, // _script_start_tag_name (display: tag_name)
	19, // _style_start_tag_name (display: tag_name)
	20, // _end_tag_name (display: tag_name)
	21, // erroneous_end_tag_name
	6,  // /> (self-closing tag delimiter, a literal-string external)
	22, // _implicit_end_tag
	23, // raw_text
	24, // comment
}

// htmlExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (htmlTok* order). Several entries
// display as "tag_name" on the loaded Language because the grammar aliases
// all four tag-name externals to the same visible node type; that is a
// known, benign display-name collapse, not ordering drift.
var htmlExternalScannerSpec = ExternalScannerSpec{
	Language:       "html",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-html",
	UpstreamCommit: "73a3947324f6efddf9e17c0ea58d454843590cc0",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "242c55ee874835b2d47288859878a0cf5dba50ffaae861a81d19fef6d010e227"},
		{Path: "src/scanner.c", SHA256: "5c0a0567e010277b81fe63be0acd04f6b44c3ef3063ac01429b910c1d22f0dca"},
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
	},
}

func init() {
	RegisterExternalScannerSpec(htmlExternalScannerSpec)
}

// htmlScannerState holds the tag stack for the HTML scanner.
type htmlScannerState struct {
	tags []htmlTag
}

// HTMLExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-html.
//
// This is a Go port of the C external scanner from tree-sitter-html
// (https://github.com/tree-sitter/tree-sitter-html). It reuses the shared
// HTML scanning infrastructure (html_tags.go, blade_scanner.go) that is
// also used by the angular, astro, blade, svelte, and vue scanners.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type HTMLExternalScanner struct {
	symbols         [htmlTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers html's external symbols.
func (HTMLExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := HTMLExternalScanner{symbols: htmlDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, htmlExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

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

func (s HTMLExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	st := payload.(*htmlScannerState)
	lx := &goLexerAdapter{lexer}

	if len(s.externalToToken) > 0 {
		var semanticValid [htmlTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < htmlTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	symbols := s.symbolTable()

	// Raw text mode: inside <script> or <style>, scan until closing tag.
	if htmlV(validSymbols, htmlTokRawText) &&
		!htmlV(validSymbols, htmlTokStartTagName) &&
		!htmlV(validSymbols, htmlTokEndTagName) {
		return htmlScanRawText(lx, st.tags, symbols[htmlTokRawText], lexer)
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
			return htmlScanComment(lx, symbols[htmlTokComment], lexer)
		}

		if htmlV(validSymbols, htmlTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &st.tags, symbols[htmlTokImplicitEndTag], lexer)
		}

	case 0: // EOF
		if htmlV(validSymbols, htmlTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &st.tags, symbols[htmlTokImplicitEndTag], lexer)
		}

	case '/':
		if htmlV(validSymbols, htmlTokSelfClosingTag) {
			return htmlScanSelfClosingDelim(lx, &st.tags, symbols[htmlTokSelfClosingTag], lexer)
		}

	default:
		if (htmlV(validSymbols, htmlTokStartTagName) || htmlV(validSymbols, htmlTokEndTagName)) &&
			!htmlV(validSymbols, htmlTokRawText) {
			if htmlV(validSymbols, htmlTokStartTagName) {
				return htmlScanStartTagName(lx, &st.tags, symbols[htmlTokStartTagName], symbols[htmlTokScriptTagName], symbols[htmlTokStyleTagName], 0, lexer)
			}
			return htmlScanEndTagName(lx, &st.tags, symbols[htmlTokEndTagName], symbols[htmlTokErrEndName], lexer)
		}
	}

	return false
}

func (s HTMLExternalScanner) symbolTable() *[htmlTokenCount]gotreesitter.Symbol {
	if s.symbols == ([htmlTokenCount]gotreesitter.Symbol{}) {
		return &htmlDefaultSymTable
	}
	return &s.symbols
}

func htmlV(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
