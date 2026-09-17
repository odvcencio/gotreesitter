//go:build !grammar_subset || grammar_subset_blade

package grammarruntime

import (
	"sync"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Blade grammar.
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
)

// bladeSyms caches resolved external symbol IDs for the blade grammar.
var bladeSyms struct {
	once                sync.Once
	startTagName        gotreesitter.Symbol
	scriptStartTagName  gotreesitter.Symbol
	styleStartTagName   gotreesitter.Symbol
	endTagName          gotreesitter.Symbol
	erroneousEndTagName gotreesitter.Symbol
	selfClosingTagDelim gotreesitter.Symbol
	implicitEndTag      gotreesitter.Symbol
	rawText             gotreesitter.Symbol
	comment             gotreesitter.Symbol
}

func resolveBladeSyms() {
	bladeSyms.once.Do(func() {
		lang := loadEmbeddedLanguage("blade.bin")
		bladeSyms.startTagName = lang.ExternalSymbols[bladeTokStartTagName]
		bladeSyms.scriptStartTagName = lang.ExternalSymbols[bladeTokScriptStartTagName]
		bladeSyms.styleStartTagName = lang.ExternalSymbols[bladeTokStyleStartTagName]
		bladeSyms.endTagName = lang.ExternalSymbols[bladeTokEndTagName]
		bladeSyms.erroneousEndTagName = lang.ExternalSymbols[bladeTokErroneousEndTagName]
		bladeSyms.selfClosingTagDelim = lang.ExternalSymbols[bladeTokSelfClosingTagDelim]
		bladeSyms.implicitEndTag = lang.ExternalSymbols[bladeTokImplicitEndTag]
		bladeSyms.rawText = lang.ExternalSymbols[bladeTokRawText]
		bladeSyms.comment = lang.ExternalSymbols[bladeTokComment]
	})
}

type bladeState struct {
	tags []htmlTag
}

// BladeExternalScanner handles HTML tag tracking for Blade templates.
type BladeExternalScanner struct{}

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

func (BladeExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*bladeState)
	lx := &goLexerAdapter{lexer}
	resolveBladeSyms()

	// Raw text in script/style tags
	if bladeValid(validSymbols, bladeTokRawText) && !bladeValid(validSymbols, bladeTokStartTagName) &&
		!bladeValid(validSymbols, bladeTokEndTagName) {
		return htmlScanRawText(lx, s.tags, bladeSyms.rawText, lexer)
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
			return htmlScanComment(lx, bladeSyms.comment, lexer)
		}

		if bladeValid(validSymbols, bladeTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, bladeSyms.implicitEndTag, lexer)
		}

	case 0:
		if bladeValid(validSymbols, bladeTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, bladeSyms.implicitEndTag, lexer)
		}

	case '/':
		if bladeValid(validSymbols, bladeTokSelfClosingTagDelim) {
			return htmlScanSelfClosingDelim(lx, &s.tags, bladeSyms.selfClosingTagDelim, lexer)
		}

	default:
		if (bladeValid(validSymbols, bladeTokStartTagName) || bladeValid(validSymbols, bladeTokEndTagName)) &&
			!bladeValid(validSymbols, bladeTokRawText) {
			if bladeValid(validSymbols, bladeTokStartTagName) {
				return htmlScanStartTagName(lx, &s.tags, bladeSyms.startTagName, bladeSyms.scriptStartTagName, bladeSyms.styleStartTagName, 0, lexer)
			}
			return htmlScanEndTagName(lx, &s.tags, bladeSyms.endTagName, bladeSyms.erroneousEndTagName, lexer)
		}
	}

	return false
}

func bladeValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
