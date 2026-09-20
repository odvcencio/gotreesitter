//go:build !grammar_subset || grammar_subset_angular

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Angular grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see angularDefaultSymTable below.
const (
	angularTokStartTagName        = 0
	angularTokScriptStartTagName  = 1
	angularTokStyleStartTagName   = 2
	angularTokEndTagName          = 3
	angularTokErroneousEndTagName = 4
	angularTokSelfClosingTagDelim = 5
	angularTokImplicitEndTag      = 6
	angularTokRawText             = 7
	angularTokComment             = 8
	angularTokInterpolationStart  = 9
	angularTokInterpolationEnd    = 10
	angularTokControlFlowStart    = 11
	angularTokEmptyQuotedString   = 12
	angularTokenCount             = 13
)

// angularDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped angular.bin assigns to each external, in angularTok*
// order. It exists only as a pre-bind fallback (and as an independent value
// to compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var angularDefaultSymTable = [angularTokenCount]gotreesitter.Symbol{
	107, // _start_tag_name (display: tag_name)
	108, // _script_start_tag_name (display: tag_name)
	109, // _style_start_tag_name (display: tag_name)
	110, // _end_tag_name (display: tag_name)
	111, // erroneous_end_tag_name
	6,   // /> (self-closing tag delimiter, a literal-string external)
	112, // _implicit_end_tag
	113, // raw_text
	114, // comment
	115, // _interpolation_start
	116, // _interpolation_end
	117, // _control_flow_start
	// _empty_quoted_string does not exist in the currently shipped blob
	// (upstream f0d0685701b7): it is only declared here for the
	// tree-sitter-angular@38a8014 port below. This placeholder binds to
	// nothing until the pending blob bump adds the external; ordinary
	// binding still succeeds against the current blob because
	// bindExternalScannerSymbolNames binds min(externals, spec tokens) and
	// leaves surplus scanner tokens at their default (see
	// external_scanner_binding.go). The real symbol ID is recorded here
	// once the bump lands.
	0,
}

// angularExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its token
// list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (angularTok* order).
//
// UpstreamCommit 38a8014ed545 carries one src/scanner.c change since the
// prior pinned commit (f0d0685701b7): dlvandenberg/tree-sitter-angular@6a31043
// ("fix: empty quoted attribute values on multiline no longer brakes
// parsing") adds an EMPTY_QUOTED_STRING external token so `[binding]=""`
// lexes as a single token instead of two adjacent double-quote tokens with
// nothing between them. The token is appended at the end of the externals
// list (index 12); every existing external keeps its index. See Scan's `"`
// case below for the ported logic.
var angularExternalScannerSpec = ExternalScannerSpec{
	Language:       "angular",
	UpstreamRepo:   "https://github.com/dlvandenberg/tree-sitter-angular",
	UpstreamCommit: "38a8014ed5452cd6b7cf1399c00177a1f5374256",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "ab9c49037f8b64590f142806d2aeb20bdd4caa5fc88890c628ab7ec4fa518ce3"},
		{Path: "src/scanner.c", SHA256: "c0d9ac5cb9f572bf3f562ac32f88440a83f0733d3828cc357391aa0d37242ff5"},
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
		"_interpolation_start",
		"_interpolation_end",
		"_control_flow_start",
		"_empty_quoted_string",
	},
}

func init() {
	RegisterExternalScannerSpec(angularExternalScannerSpec)
}

type angularState struct {
	tags []htmlTag
}

// AngularExternalScanner handles HTML tag tracking plus Angular-specific
// interpolation for Angular templates. It reuses the shared HTML scanning
// infrastructure (html_tags.go, blade_scanner.go) that is also used by the
// astro, blade, html, svelte, and vue scanners.
type AngularExternalScanner struct {
	symbols         [angularTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers angular's external symbols.
func (AngularExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := AngularExternalScanner{symbols: angularDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, angularExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (AngularExternalScanner) Create() any         { return &angularState{} }
func (AngularExternalScanner) Destroy(payload any) {}

func (AngularExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*angularState)
	return htmlSerializeTags(s.tags, buf)
}

func (AngularExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*angularState)
	s.tags = htmlDeserializeTagsInto(s.tags, buf)
}

func (s AngularExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	st := payload.(*angularState)
	lx := &goLexerAdapter{lexer}

	if len(s.externalToToken) > 0 {
		var semanticValid [angularTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < angularTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	symbols := s.symbolTable()

	if angularValid(validSymbols, angularTokRawText) && !angularValid(validSymbols, angularTokStartTagName) &&
		!angularValid(validSymbols, angularTokEndTagName) {
		return htmlScanRawText(lx, st.tags, symbols[angularTokRawText], lexer)
	}

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	switch lexer.Lookahead() {
	case '<':
		lexer.MarkEnd()
		lexer.Advance(false)

		if lexer.Lookahead() == '!' {
			lexer.Advance(false)
			return htmlScanComment(lx, symbols[angularTokComment], lexer)
		}

		if angularValid(validSymbols, angularTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &st.tags, symbols[angularTokImplicitEndTag], lexer)
		}

	case 0:
		if angularValid(validSymbols, angularTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &st.tags, symbols[angularTokImplicitEndTag], lexer)
		}

	case '/':
		if angularValid(validSymbols, angularTokSelfClosingTagDelim) {
			return htmlScanSelfClosingDelim(lx, &st.tags, symbols[angularTokSelfClosingTagDelim], lexer)
		}

	case '{':
		if angularValid(validSymbols, angularTokInterpolationStart) {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				lexer.Advance(false)
				lexer.MarkEnd()
				st.tags = append(st.tags, htmlTag{tagType: htmlTagInterpolation})
				lexer.SetResultSymbol(symbols[angularTokInterpolationStart])
				return true
			}
		}

	case '}':
		if angularValid(validSymbols, angularTokInterpolationEnd) {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '}' && len(st.tags) > 0 &&
				st.tags[len(st.tags)-1].tagType == htmlTagInterpolation {
				lexer.Advance(false)
				lexer.MarkEnd()
				st.tags = st.tags[:len(st.tags)-1]
				lexer.SetResultSymbol(symbols[angularTokInterpolationEnd])
				return true
			}
		}

	case '@':
		if angularValid(validSymbols, angularTokControlFlowStart) {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(symbols[angularTokControlFlowStart])
			return true
		}

	case '"':
		// tree-sitter-angular@6a31043: lex a bare `""` as one
		// EMPTY_QUOTED_STRING token when the grammar asks for it (used by
		// `_binding_assignment` to alias `=""` without an expression
		// between the quotes), instead of two independent double-quote
		// tokens with nothing between them.
		if angularValid(validSymbols, angularTokEmptyQuotedString) {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(symbols[angularTokEmptyQuotedString])
				return true
			}
		}

	default:
		if (angularValid(validSymbols, angularTokStartTagName) || angularValid(validSymbols, angularTokEndTagName)) &&
			!angularValid(validSymbols, angularTokRawText) {
			if angularValid(validSymbols, angularTokStartTagName) {
				return htmlScanStartTagName(lx, &st.tags, symbols[angularTokStartTagName], symbols[angularTokScriptStartTagName], symbols[angularTokStyleStartTagName], 0, lexer)
			}
			return htmlScanEndTagName(lx, &st.tags, symbols[angularTokEndTagName], symbols[angularTokErroneousEndTagName], lexer)
		}
	}

	return false
}

func (s AngularExternalScanner) symbolTable() *[angularTokenCount]gotreesitter.Symbol {
	if s.symbols == ([angularTokenCount]gotreesitter.Symbol{}) {
		return &angularDefaultSymTable
	}
	return &s.symbols
}

func angularValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
