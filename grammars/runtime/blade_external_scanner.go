//go:build !grammar_subset || grammar_subset_blade

package grammarruntime

import (
	"strings"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Blade grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see bladeDefaultSymTable below.
const (
	bladeTokStartTagName        = 0
	bladeTokScriptStartTagName  = 1
	bladeTokStyleStartTagName   = 2
	bladeTokEndTagName          = 3
	bladeTokErroneousEndTagName = 4
	bladeTokSelfClosingTagDelim = 5
	bladeTokImplicitEndTag      = 6
	bladeTokRawText             = 7
	bladeTokComment             = 8
	bladeTokenCount             = 9
)

// bladeDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped blade.bin assigns to each external, in bladeTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var bladeDefaultSymTable = [bladeTokenCount]gotreesitter.Symbol{
	172, // _start_tag_name (display: tag_name)
	173, // _script_start_tag_name (display: tag_name)
	174, // _style_start_tag_name (display: tag_name)
	175, // _end_tag_name (display: tag_name)
	176, // erroneous_end_tag_name
	6,   // /> (self-closing tag delimiter, a literal-string external)
	177, // _implicit_end_tag
	178, // raw_text
	12,  // comment
}

// bladeExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (bladeTok* order). Several entries
// display as "tag_name" on the loaded Language because the grammar aliases
// all four tag-name externals to the same visible node type; that is a
// known, benign display-name collapse, not ordering drift.
var bladeExternalScannerSpec = ExternalScannerSpec{
	Language:       "blade",
	UpstreamRepo:   "https://github.com/EmranMR/tree-sitter-blade",
	UpstreamCommit: "b5291d1ba207a8ebb8383b2ecb8a8a6535210a50",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "d03b14c9c99ef3b47c1f527e12b6346ee45b114dcd9d9966f91f3d891546ecb9"},
		{Path: "src/scanner.c", SHA256: "8f7e3a669525be515f31df420ec71ff0bbd9453e73e2f170be6e59a42dd8f953"},
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
	RegisterExternalScannerSpec(bladeExternalScannerSpec)
}

type bladeState struct {
	tags []htmlTag
}

// BladeExternalScanner handles HTML tag tracking for Blade templates. It
// reuses the shared HTML scanning infrastructure (html_tags.go,
// blade_scanner.go) that is also used by the angular, astro, html, svelte,
// and vue scanners.
type BladeExternalScanner struct {
	symbols         [bladeTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers blade's external symbols.
func (BladeExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := BladeExternalScanner{symbols: bladeDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, bladeExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (BladeExternalScanner) Create() any         { return &bladeState{} }
func (BladeExternalScanner) Destroy(payload any) {}

func (BladeExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*bladeState)
	return htmlSerializeTags(s.tags, buf)
}

func (BladeExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*bladeState)
	s.tags = htmlDeserializeTagsInto(s.tags, buf)
}

func (s BladeExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	st := payload.(*bladeState)
	lx := &goLexerAdapter{lexer}

	if len(s.externalToToken) > 0 {
		var semanticValid [bladeTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < bladeTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	symbols := s.symbolTable()

	// Raw text in script/style tags
	if bladeValid(validSymbols, bladeTokRawText) && !bladeValid(validSymbols, bladeTokStartTagName) &&
		!bladeValid(validSymbols, bladeTokEndTagName) {
		return htmlScanRawText(lx, st.tags, symbols[bladeTokRawText], lexer)
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	switch lexer.Lookahead() {
	case '<':
		lexer.MarkEnd()
		lexer.Advance(false)

		if lexer.Lookahead() == '!' {
			lexer.Advance(false)
			return htmlScanComment(lx, symbols[bladeTokComment], lexer)
		}

		if bladeValid(validSymbols, bladeTokImplicitEndTag) {
			return bladeScanImplicitEndTag(lx, &st.tags, symbols[bladeTokImplicitEndTag], lexer)
		}

	case 0:
		if bladeValid(validSymbols, bladeTokImplicitEndTag) {
			return bladeScanImplicitEndTag(lx, &st.tags, symbols[bladeTokImplicitEndTag], lexer)
		}

	case '/':
		if bladeValid(validSymbols, bladeTokSelfClosingTagDelim) {
			return htmlScanSelfClosingDelim(lx, &st.tags, symbols[bladeTokSelfClosingTagDelim], lexer)
		}

	default:
		if (bladeValid(validSymbols, bladeTokStartTagName) || bladeValid(validSymbols, bladeTokEndTagName)) &&
			!bladeValid(validSymbols, bladeTokRawText) {
			if bladeValid(validSymbols, bladeTokStartTagName) {
				return htmlScanStartTagName(lx, &st.tags, symbols[bladeTokStartTagName], symbols[bladeTokScriptStartTagName], symbols[bladeTokStyleStartTagName], 0, lexer)
			}
			return bladeScanEndTagName(lx, &st.tags, symbols[bladeTokEndTagName], symbols[bladeTokErroneousEndTagName], lexer)
		}
	}

	return false
}

func (s BladeExternalScanner) symbolTable() *[bladeTokenCount]gotreesitter.Symbol {
	if s.symbols == ([bladeTokenCount]gotreesitter.Symbol{}) {
		return &bladeDefaultSymTable
	}
	return &s.symbols
}

func bladeValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

// --- Blade-specific tag matching (upstream b5291d1b: scoped slots) ---
//
// tree-sitter-blade@b5291d1b (PR #133, "support scoped slots") added an
// X_SLOT tag type to src/tag.h: a bare closing "</x-slot>" now matches any
// open "<x-slot:name>" tag, in addition to the existing exact-name match. The
// upstream src/scanner.c diff itself only gates name serialization on
// `tag.type == CUSTOM || tag.type == X_SLOT` (serialize/deserialize); the
// matching behavior lives in tag_eq (src/tag.h), which scan_end_tag_name and
// scan_implicit_end_tag call.
//
// blade_scanner.go and html_tags.go are shared with angular, astro, html,
// svelte, and vue, so this rule is implemented here against the existing
// htmlTagCustom bucket instead of touching htmlTagEq, keeping the shared
// helper's behavior unchanged for every other language.

const bladeXSlotTagName = "X-SLOT"

// bladeIsXSlotName reports whether name is the bare "X-SLOT" tag or a
// specifically named "X-SLOT:name" tag. Both map to htmlTagCustom in the
// shared tag model; upstream tag_type_for_name classifies both as X_SLOT.
func bladeIsXSlotName(name string) bool {
	return name == bladeXSlotTagName || strings.HasPrefix(name, bladeXSlotTagName+":")
}

// bladeTagEq mirrors htmlTagEq and adds upstream tag_eq's X_SLOT rule: a
// bare "X-SLOT" closing tag matches any open "X-SLOT" or "X-SLOT:name" tag,
// on top of the ordinary exact-name match every other custom tag already
// gets from htmlTagEq.
func bladeTagEq(a, b *htmlTag) bool {
	if a.tagType != b.tagType {
		return false
	}
	if a.tagType == htmlTagCustom && bladeIsXSlotName(a.customName) && b.customName == bladeXSlotTagName {
		return true
	}
	return htmlTagEq(a, b)
}

// bladeScanEndTagName ports scan_end_tag_name 1:1, using bladeTagEq so a
// bare "</x-slot>" can close a specifically named "<x-slot:name>" tag.
func bladeScanEndTagName(lx htmlLexer, tags *[]htmlTag, endSym, errEndSym gotreesitter.Symbol, lexer *gotreesitter.ExternalLexer) bool {
	tagName := htmlScanTagName(lx)
	if len(tagName) == 0 {
		return false
	}

	tag := htmlTagForName(tagName)
	lx.markEnd()
	if len(*tags) > 0 && bladeTagEq(&(*tags)[len(*tags)-1], &tag) {
		*tags = (*tags)[:len(*tags)-1]
		lexer.SetResultSymbol(endSym)
	} else {
		lexer.SetResultSymbol(errEndSym)
	}
	return true
}

// bladeScanImplicitEndTag ports scan_implicit_end_tag 1:1, using bladeTagEq
// for the same reason as bladeScanEndTagName.
func bladeScanImplicitEndTag(lx htmlLexer, tags *[]htmlTag, implicitEndTagSym gotreesitter.Symbol, lexer *gotreesitter.ExternalLexer) bool {
	var parent *htmlTag
	if len(*tags) > 0 {
		parent = &(*tags)[len(*tags)-1]
	}

	isClosingTag := false
	if lx.lookahead() == '/' {
		isClosingTag = true
		lx.advance(false)
	} else {
		if parent != nil && htmlTagIsVoid(parent) {
			*tags = (*tags)[:len(*tags)-1]
			lexer.SetResultSymbol(implicitEndTagSym)
			return true
		}
	}

	tagName := htmlScanTagName(lx)
	if len(tagName) == 0 && !lx.eof() {
		return false
	}

	nextTag := htmlTagForName(tagName)

	if isClosingTag {
		if len(*tags) > 0 && bladeTagEq(&(*tags)[len(*tags)-1], &nextTag) {
			return false
		}
		for i := len(*tags); i > 0; i-- {
			if (*tags)[i-1].tagType == nextTag.tagType {
				*tags = (*tags)[:len(*tags)-1]
				lexer.SetResultSymbol(implicitEndTagSym)
				return true
			}
		}
	} else if parent != nil &&
		(!htmlTagCanContain(parent, &nextTag) ||
			((parent.tagType == htmlTagHtml || parent.tagType == htmlTagHead || parent.tagType == htmlTagBody) && lx.eof())) {
		*tags = (*tags)[:len(*tags)-1]
		lexer.SetResultSymbol(implicitEndTagSym)
		return true
	}

	return false
}
