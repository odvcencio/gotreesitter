//go:build !grammar_subset || grammar_subset_vue

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Vue grammar.
const (
	vueTokStartTagName         = 0
	vueTokScriptStartTagName   = 1
	vueTokStyleStartTagName    = 2
	vueTokEndTagName           = 3
	vueTokErroneousEndTagName  = 4
	vueTokSelfClosingTagDelim  = 5
	vueTokImplicitEndTag       = 6
	vueTokRawText              = 7
	vueTokComment              = 8
	vueTokTemplateStartTagName = 9
	vueTokTextFragment         = 10
	vueTokInterpolationText    = 11
	vueTokenCount              = 12
)

// vueDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped vue.bin assigns to each external, in vueTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var vueDefaultSymTable = [vueTokenCount]gotreesitter.Symbol{
	30, // _start_tag_name
	31, // _script_start_tag_name
	32, // _style_start_tag_name
	33, // _end_tag_name
	34, // erroneous_end_tag_name
	6,  // />
	35, // _implicit_end_tag
	36, // raw_text
	37, // comment
	38, // _template_start_tag_name
	39, // _text_fragment
	40, // _interpolation_text
}

// vueExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (vueTok* order).
var vueExternalScannerSpec = ExternalScannerSpec{
	Language:       "vue",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-vue",
	UpstreamCommit: "ce8011a414fdf8091f4e4071752efc376f4afb08",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "a83f1d8208d5c0206fec0078cd62bc051b44d630f403f759a6236260f39dd604"},
		{Path: "src/scanner.c", SHA256: "9c2147fe2de1ede71f3ef5fd3ba54d12dbb6c2f71bce5735358fdbd4f8d19a32"},
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
		"_template_start_tag_name",
		"_text_fragment",
		"_interpolation_text",
	},
}

func init() {
	RegisterExternalScannerSpec(vueExternalScannerSpec)
}

type vueState struct {
	tags []htmlTag
}

// VueExternalScanner handles HTML tag tracking plus Vue-specific text fragments and interpolation.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type VueExternalScanner struct {
	symbols         [vueTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers vue's external symbols.
func (VueExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := VueExternalScanner{symbols: vueDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, vueExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s VueExternalScanner) symbolTable() *[vueTokenCount]gotreesitter.Symbol {
	if s.symbols == ([vueTokenCount]gotreesitter.Symbol{}) {
		return &vueDefaultSymTable
	}
	return &s.symbols
}

func (VueExternalScanner) Create() any         { return &vueState{} }
func (VueExternalScanner) Destroy(payload any) {}

func (VueExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*vueState)
	return htmlSerializeTags(s.tags, buf)
}

func (VueExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*vueState)
	s.tags = htmlDeserializeTagsInto(s.tags, buf)
}

func (sc VueExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [vueTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < vueTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	s := payload.(*vueState)
	lx := &goLexerAdapter{lexer}

	// Text fragment / interpolation text scanning.
	//
	// In the C scanner this is an inline block: when it cannot produce a
	// TEXT_FRAGMENT/INTERPOLATION_TEXT token (e.g. the run is whitespace-only and
	// the next significant character is `<`), control *falls through* to the rest
	// of scan() so that a comment / implicit-end-tag / start/end tag name can be
	// produced instead. The Go port must preserve that fall-through: a leading
	// newline before `<!-- -->` inside a <template> would otherwise dead-end
	// because the external `comment` token never gets a chance to fire.
	isErrorRecovery := vueValid(validSymbols, vueTokStartTagName) && vueValid(validSymbols, vueTokRawText)
	if !isErrorRecovery && lexer.Lookahead() != '<' &&
		(vueValid(validSymbols, vueTokTextFragment) || vueValid(validSymbols, vueTokInterpolationText)) {
		if result, handled := vueScanTextFragment(s, lexer, validSymbols, syms); handled {
			return result
		}
	}

	if vueValid(validSymbols, vueTokRawText) && !vueValid(validSymbols, vueTokStartTagName) &&
		!vueValid(validSymbols, vueTokEndTagName) {
		return htmlScanRawText(lx, s.tags, syms[vueTokRawText], lexer)
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
			return htmlScanComment(lx, syms[vueTokComment], lexer)
		}

		if vueValid(validSymbols, vueTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, syms[vueTokImplicitEndTag], lexer)
		}

	case 0:
		if vueValid(validSymbols, vueTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, syms[vueTokImplicitEndTag], lexer)
		}

	case '/':
		if vueValid(validSymbols, vueTokSelfClosingTagDelim) {
			return htmlScanSelfClosingDelim(lx, &s.tags, syms[vueTokSelfClosingTagDelim], lexer)
		}

	default:
		if (vueValid(validSymbols, vueTokStartTagName) || vueValid(validSymbols, vueTokEndTagName)) &&
			!vueValid(validSymbols, vueTokRawText) {
			if vueValid(validSymbols, vueTokStartTagName) {
				return htmlScanStartTagName(lx, &s.tags, syms[vueTokStartTagName], syms[vueTokScriptStartTagName], syms[vueTokStyleStartTagName], syms[vueTokTemplateStartTagName], lexer)
			}
			return htmlScanEndTagName(lx, &s.tags, syms[vueTokEndTagName], syms[vueTokErroneousEndTagName], lexer)
		}
	}

	return false
}

// vueScanTextFragment mirrors the inline text-fragment block of the C scanner's
// scan(). It returns (result, handled): when handled is false the caller must
// continue with the rest of Scan() (C's fall-through), otherwise result is the
// value to return from Scan().
func vueScanTextFragment(s *vueState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[vueTokenCount]gotreesitter.Symbol) (result bool, handled bool) {
	advancedOnce := false

	if !vueValid(validSymbols, vueTokComment) {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
	}

	for lexer.Lookahead() != 0 {
		switch lexer.Lookahead() {
		case '<':
			lexer.MarkEnd()
			lexer.Advance(false)
			ch := lexer.Lookahead()
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '!' || ch == '?' || ch == '/' {
				goto loopExit
			}
			advancedOnce = true

		case '{':
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				goto loopExit
			}
			advancedOnce = true

		case '}':
			if vueValid(validSymbols, vueTokInterpolationText) {
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '}' {
					lexer.SetResultSymbol(syms[vueTokInterpolationText])
					return advancedOnce, true
				}
			} else {
				lexer.Advance(false)
				advancedOnce = true
			}

		case '\r':
			// Mirror C: handle CRLF; a lone CR behaves like the default case.
			lexer.Advance(false)
			if lexer.Lookahead() != '\n' {
				advancedOnce = true
				lexer.Advance(false)
				break
			}
			fallthrough

		case '\n':
			if vueValid(validSymbols, vueTokTextFragment) {
				lexer.MarkEnd()
				for unicode.IsSpace(lexer.Lookahead()) {
					if advancedOnce {
						lexer.Advance(false)
					} else {
						lexer.Advance(true)
					}
				}
				if lexer.Lookahead() == '<' || lexer.Lookahead() == '>' {
					goto loopExit
				}
			} else {
				lexer.Advance(false)
			}

		default:
			advancedOnce = advancedOnce || lexer.Lookahead() != '\n'
			lexer.Advance(false)
		}
	}

	if lexer.Lookahead() == 0 {
		// C: `if (lexer->eof(lexer)) return false;` — a handled negative result.
		return false, true
	}

loopExit:
	if advancedOnce {
		lexer.SetResultSymbol(syms[vueTokTextFragment])
		return true, true
	}
	// C falls through to the remainder of scan() (comment / tags / etc.).
	return false, false
}

func vueValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
