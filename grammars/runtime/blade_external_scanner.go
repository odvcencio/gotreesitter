//go:build !grammar_subset || grammar_subset_blade

package grammarruntime

import (
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
	UpstreamCommit: "42b3c5a06bc29fbd2c2cbd52b96113365fbed646",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "f8a5d35130ff5de1e264fbf6c3a907f05c5f9474abbc536546b4d5d679038485"},
		{Path: "src/scanner.c", SHA256: "c6e92c8128b23846bdf3330d146739a9fa0e9e75a8fb701803b21d5d7eb6b9ee"},
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
			return htmlScanImplicitEndTag(lx, &st.tags, symbols[bladeTokImplicitEndTag], lexer)
		}

	case 0:
		if bladeValid(validSymbols, bladeTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &st.tags, symbols[bladeTokImplicitEndTag], lexer)
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
			return htmlScanEndTagName(lx, &st.tags, symbols[bladeTokEndTagName], symbols[bladeTokErroneousEndTagName], lexer)
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
