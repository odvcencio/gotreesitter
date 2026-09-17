//go:build !grammar_subset || grammar_subset_angular || grammar_subset_astro || grammar_subset_blade || grammar_subset_html || grammar_subset_svelte || grammar_subset_vue

package grammarruntime

import "strings"

// htmlTagType represents the type of an HTML element.
type htmlTagType uint8

const (
	// Void elements (self-closing) — must come before htmlEndOfVoidTags.
	htmlTagArea htmlTagType = iota
	htmlTagBase
	htmlTagBasefont
	htmlTagBgsound
	htmlTagBr
	htmlTagCol
	htmlTagEmbed
	htmlTagFrame
	htmlTagHr
	htmlTagImage
	htmlTagImg
	htmlTagInput
	htmlTagKeygen
	htmlTagLink
	htmlTagMenuitem
	htmlTagMeta
	htmlTagParam
	htmlTagSource
	htmlTagTrack
	htmlTagWbr
	htmlEndOfVoidTags // sentinel

	// Standard elements.
	htmlTagA
	htmlTagAbbr
	htmlTagAddress
	htmlTagArticle
	htmlTagAside
	htmlTagB
	htmlTagBlockquote
	htmlTagBody
	htmlTagButton
	htmlTagCaption
	htmlTagCode
	htmlTagColgroup
	htmlTagDd
	htmlTagDetails
	htmlTagDiv
	htmlTagDl
	htmlTagDt
	htmlTagEm
	htmlTagFieldset
	htmlTagFigcaption
	htmlTagFigure
	htmlTagFooter
	htmlTagForm
	htmlTagH1
	htmlTagH2
	htmlTagH3
	htmlTagH4
	htmlTagH5
	htmlTagH6
	htmlTagHead
	htmlTagHeader
	htmlTagHgroup
	htmlTagHtml
	htmlTagI
	htmlTagIframe
	htmlTagLi
	htmlTagMain
	htmlTagNav
	htmlTagNoscript
	htmlTagOl
	htmlTagOptgroup
	htmlTagOption
	htmlTagP
	htmlTagPre
	htmlTagRb
	htmlTagRp
	htmlTagRt
	htmlTagRuby
	htmlTagS
	htmlTagScript
	htmlTagSection
	htmlTagSelect
	htmlTagSmall
	htmlTagSpan
	htmlTagStrong
	htmlTagStyle
	htmlTagSummary
	htmlTagTable
	htmlTagTbody
	htmlTagTd
	htmlTagTemplate
	htmlTagTfoot
	htmlTagTh
	htmlTagThead
	htmlTagTitle
	htmlTagTr
	htmlTagUl
	htmlTagCustom

	// Extra virtual tag types for specific grammars.
	htmlTagInterpolation // used by angular for {{ }} tracking
)

// htmlTag represents an element on the tag stack.
type htmlTag struct {
	tagType    htmlTagType
	customName string // non-empty only for htmlTagCustom
}

func htmlTagIsVoid(t *htmlTag) bool {
	return t.tagType < htmlEndOfVoidTags
}

func htmlTagEq(a, b *htmlTag) bool {
	if a.tagType != b.tagType {
		return false
	}
	if a.tagType == htmlTagCustom {
		return a.customName == b.customName
	}
	return true
}

// htmlTagCanContain implements simplified HTML content model rules.
func htmlTagCanContain(parent, child *htmlTag) bool {
	switch parent.tagType {
	case htmlTagLi:
		return child.tagType != htmlTagLi
	case htmlTagDt, htmlTagDd:
		return child.tagType != htmlTagDt && child.tagType != htmlTagDd
	case htmlTagP:
		return !htmlIsBlockElement(child.tagType)
	case htmlTagColgroup:
		return child.tagType == htmlTagCol
	case htmlTagRb, htmlTagRt, htmlTagRp:
		return child.tagType != htmlTagRb && child.tagType != htmlTagRt && child.tagType != htmlTagRp
	case htmlTagOptgroup:
		return child.tagType != htmlTagOptgroup
	case htmlTagTr:
		return child.tagType != htmlTagTr
	case htmlTagTd, htmlTagTh:
		return child.tagType != htmlTagTd && child.tagType != htmlTagTh && child.tagType != htmlTagTr
	default:
		return true
	}
}

func htmlIsBlockElement(t htmlTagType) bool {
	switch t {
	case htmlTagAddress, htmlTagArticle, htmlTagAside, htmlTagBlockquote,
		htmlTagDetails, htmlTagDiv, htmlTagDl, htmlTagFieldset, htmlTagFigcaption,
		htmlTagFigure, htmlTagFooter, htmlTagForm, htmlTagH1, htmlTagH2,
		htmlTagH3, htmlTagH4, htmlTagH5, htmlTagH6, htmlTagHeader,
		htmlTagHgroup, htmlTagHr, htmlTagMain, htmlTagNav,
		htmlTagNoscript, htmlTagOl, htmlTagP, htmlTagPre, htmlTagScript,
		htmlTagSection, htmlTagTable, htmlTagTemplate, htmlTagUl:
		return true
	}
	return false
}

// htmlTagNameMap maps lowercase tag name to tag type.
var htmlTagNameMap = map[string]htmlTagType{
	"AREA": htmlTagArea, "BASE": htmlTagBase, "BASEFONT": htmlTagBasefont,
	"BGSOUND": htmlTagBgsound, "BR": htmlTagBr, "COL": htmlTagCol,
	"EMBED": htmlTagEmbed, "FRAME": htmlTagFrame, "HR": htmlTagHr,
	"IMAGE": htmlTagImage, "IMG": htmlTagImg, "INPUT": htmlTagInput,
	"KEYGEN": htmlTagKeygen, "LINK": htmlTagLink, "MENUITEM": htmlTagMenuitem,
	"META": htmlTagMeta, "PARAM": htmlTagParam, "SOURCE": htmlTagSource,
	"TRACK": htmlTagTrack, "WBR": htmlTagWbr,
	"A": htmlTagA, "ABBR": htmlTagAbbr, "ADDRESS": htmlTagAddress,
	"ARTICLE": htmlTagArticle, "ASIDE": htmlTagAside, "B": htmlTagB,
	"BLOCKQUOTE": htmlTagBlockquote, "BODY": htmlTagBody, "BUTTON": htmlTagButton,
	"CAPTION": htmlTagCaption, "CODE": htmlTagCode, "COLGROUP": htmlTagColgroup,
	"DD": htmlTagDd, "DETAILS": htmlTagDetails, "DIV": htmlTagDiv,
	"DL": htmlTagDl, "DT": htmlTagDt, "EM": htmlTagEm,
	"FIELDSET": htmlTagFieldset, "FIGCAPTION": htmlTagFigcaption,
	"FIGURE": htmlTagFigure, "FOOTER": htmlTagFooter, "FORM": htmlTagForm,
	"H1": htmlTagH1, "H2": htmlTagH2, "H3": htmlTagH3,
	"H4": htmlTagH4, "H5": htmlTagH5, "H6": htmlTagH6,
	"HEAD": htmlTagHead, "HEADER": htmlTagHeader, "HGROUP": htmlTagHgroup,
	"HTML": htmlTagHtml, "I": htmlTagI, "IFRAME": htmlTagIframe,
	"LI": htmlTagLi, "MAIN": htmlTagMain, "NAV": htmlTagNav,
	"NOSCRIPT": htmlTagNoscript, "OL": htmlTagOl, "OPTGROUP": htmlTagOptgroup,
	"OPTION": htmlTagOption, "P": htmlTagP, "PRE": htmlTagPre,
	"RB": htmlTagRb, "RP": htmlTagRp, "RT": htmlTagRt, "RUBY": htmlTagRuby,
	"S": htmlTagS, "SCRIPT": htmlTagScript, "SECTION": htmlTagSection,
	"SELECT": htmlTagSelect, "SMALL": htmlTagSmall, "SPAN": htmlTagSpan,
	"STRONG": htmlTagStrong, "STYLE": htmlTagStyle, "SUMMARY": htmlTagSummary,
	"TABLE": htmlTagTable, "TBODY": htmlTagTbody, "TD": htmlTagTd,
	"TEMPLATE": htmlTagTemplate, "TFOOT": htmlTagTfoot, "TH": htmlTagTh,
	"THEAD": htmlTagThead, "TITLE": htmlTagTitle, "TR": htmlTagTr,
	"UL": htmlTagUl,
}

func htmlTagForName(name string) htmlTag {
	upper := strings.ToUpper(name)
	if t, ok := htmlTagNameMap[upper]; ok {
		return htmlTag{tagType: t}
	}
	return htmlTag{tagType: htmlTagCustom, customName: upper}
}

// htmlScanTagName reads a tag name from the lexer, returning it uppercased.
func htmlScanTagName(lexer htmlLexer) string {
	var stack [64]byte
	buf := stack[:0]
	for {
		ch := lexer.lookahead()
		if !htmlIsTagNameChar(ch) {
			break
		}
		if ch >= 'a' && ch <= 'z' {
			ch = ch - 'a' + 'A'
		}
		buf = append(buf, byte(ch))
		lexer.advance(false)
	}
	return string(buf)
}

func htmlIsTagNameChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '-' || ch == ':' || ch == '.'
}

// htmlLexer is a thin interface to adapt gotreesitter.ExternalLexer for shared HTML scanning.
type htmlLexer interface {
	lookahead() rune
	advance(skip bool)
	markEnd()
	eof() bool
}
