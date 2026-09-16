//go:build !grammar_subset || grammar_subset_angular

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Angular grammar.
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
)

const (
	angularSymStartTagName        gotreesitter.Symbol = 107
	angularSymScriptStartTagName  gotreesitter.Symbol = 108
	angularSymStyleStartTagName   gotreesitter.Symbol = 109
	angularSymEndTagName          gotreesitter.Symbol = 110
	angularSymErroneousEndTagName gotreesitter.Symbol = 111
	angularSymSelfClosingTagDelim gotreesitter.Symbol = 6
	angularSymImplicitEndTag      gotreesitter.Symbol = 112
	angularSymRawText             gotreesitter.Symbol = 113
	angularSymComment             gotreesitter.Symbol = 114
	angularSymInterpolationStart  gotreesitter.Symbol = 115
	angularSymInterpolationEnd    gotreesitter.Symbol = 116
	angularSymControlFlowStart    gotreesitter.Symbol = 117
)

type angularState struct {
	tags []htmlTag
}

// AngularExternalScanner handles HTML tag tracking plus Angular-specific interpolation for Angular templates.
type AngularExternalScanner struct{}

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

func (AngularExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*angularState)
	lx := &goLexerAdapter{lexer}

	if angularValid(validSymbols, angularTokRawText) && !angularValid(validSymbols, angularTokStartTagName) &&
		!angularValid(validSymbols, angularTokEndTagName) {
		return htmlScanRawText(lx, s.tags, angularSymRawText, lexer)
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
			return htmlScanComment(lx, angularSymComment, lexer)
		}

		if angularValid(validSymbols, angularTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, angularSymImplicitEndTag, lexer)
		}

	case 0:
		if angularValid(validSymbols, angularTokImplicitEndTag) {
			return htmlScanImplicitEndTag(lx, &s.tags, angularSymImplicitEndTag, lexer)
		}

	case '/':
		if angularValid(validSymbols, angularTokSelfClosingTagDelim) {
			return htmlScanSelfClosingDelim(lx, &s.tags, angularSymSelfClosingTagDelim, lexer)
		}

	case '{':
		if angularValid(validSymbols, angularTokInterpolationStart) {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				lexer.Advance(false)
				lexer.MarkEnd()
				s.tags = append(s.tags, htmlTag{tagType: htmlTagInterpolation})
				lexer.SetResultSymbol(angularSymInterpolationStart)
				return true
			}
		}

	case '}':
		if angularValid(validSymbols, angularTokInterpolationEnd) {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '}' && len(s.tags) > 0 &&
				s.tags[len(s.tags)-1].tagType == htmlTagInterpolation {
				lexer.Advance(false)
				lexer.MarkEnd()
				s.tags = s.tags[:len(s.tags)-1]
				lexer.SetResultSymbol(angularSymInterpolationEnd)
				return true
			}
		}

	case '@':
		if angularValid(validSymbols, angularTokControlFlowStart) {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(angularSymControlFlowStart)
			return true
		}

	default:
		if (angularValid(validSymbols, angularTokStartTagName) || angularValid(validSymbols, angularTokEndTagName)) &&
			!angularValid(validSymbols, angularTokRawText) {
			if angularValid(validSymbols, angularTokStartTagName) {
				return htmlScanStartTagName(lx, &s.tags, angularSymStartTagName, angularSymScriptStartTagName, angularSymStyleStartTagName, 0, lexer)
			}
			return htmlScanEndTagName(lx, &s.tags, angularSymEndTagName, angularSymErroneousEndTagName, lexer)
		}
	}

	return false
}

func angularValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
