//go:build !grammar_subset || grammar_subset_html

package grammarruntime

import (
	"bytes"
	"encoding/binary"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// The C-faithful scan of html: a line-by-line port of the pinned C scanner
// (tree-sitter-html src/scanner.c and src/tag.h), used only when the core turns the
// scanner mode on (internal/scanmode). The tag table of the C scanner is kept here,
// beside html_tags.go, which the angular, astro, blade, svelte and vue ports share
// and which stays as it is: it numbers the tags in another order (so the saved
// state differs), lacks some tags (COMMAND, ISINDEX, NEXTID, AUDIO, ...) and
// has another content model for <p>.

// htmlCTagNames is TAG_TYPES_BY_TAG_NAME of tag.h:140-267 (126 entries). The tag
// type of an entry is its index, plus one from the entry after the last void tag
// (the enum has END_OF_VOID_TAGS between the void tags and A, tag.h:27).
var htmlCTagNames = [...]string{
	"AREA", "BASE", "BASEFONT", "BGSOUND", "BR", "COL", "COMMAND", "EMBED", "FRAME", "HR", "IMAGE", "IMG",
	"INPUT", "ISINDEX", "KEYGEN", "LINK", "MENUITEM", "META", "NEXTID", "PARAM", "SOURCE", "TRACK", "WBR",
	"A", "ABBR", "ADDRESS", "ARTICLE", "ASIDE", "AUDIO", "B", "BDI", "BDO", "BLOCKQUOTE", "BODY", "BUTTON",
	"CANVAS", "CAPTION", "CITE", "CODE", "COLGROUP", "DATA", "DATALIST", "DD", "DEL", "DETAILS", "DFN",
	"DIALOG", "DIV", "DL", "DT", "EM", "FIELDSET", "FIGCAPTION", "FIGURE", "FOOTER", "FORM", "H1", "H2",
	"H3", "H4", "H5", "H6", "HEAD", "HEADER", "HGROUP", "HTML", "I", "IFRAME", "INS", "KBD", "LABEL",
	"LEGEND", "LI", "MAIN", "MAP", "MARK", "MATH", "MENU", "METER", "NAV", "NOSCRIPT", "OBJECT", "OL",
	"OPTGROUP", "OPTION", "OUTPUT", "P", "PICTURE", "PRE", "PROGRESS", "Q", "RB", "RP", "RT", "RTC", "RUBY",
	"S", "SAMP", "SCRIPT", "SECTION", "SELECT", "SLOT", "SMALL", "SPAN", "STRONG", "STYLE", "SUB", "SUMMARY",
	"SUP", "SVG", "TABLE", "TBODY", "TD", "TEMPLATE", "TEXTAREA", "TFOOT", "TH", "THEAD", "TIME", "TITLE",
	"TR", "U", "UL", "VAR", "VIDEO", "CUSTOM",
}

const htmlCEndOfVoidTags = 23 // END_OF_VOID_TAGS (tag.h:27)

// htmlCType returns the TagType of a tag name of the table.
func htmlCType(name string) uint32 {
	for i, n := range htmlCTagNames {
		if n == name {
			if i >= htmlCEndOfVoidTags {
				return uint32(i + 1)
			}
			return uint32(i)
		}
	}
	panic("html: no C tag " + name)
}

var (
	htmlCCustom   = htmlCType("CUSTOM")
	htmlCEnd      = htmlCCustom + 1 // END_ (tag.h:164)
	htmlCScript   = htmlCType("SCRIPT")
	htmlCStyle    = htmlCType("STYLE")
	htmlCHTML     = htmlCType("HTML")
	htmlCHead     = htmlCType("HEAD")
	htmlCBody     = htmlCType("BODY")
	htmlCNotInPar = func() map[uint32]bool { // TAG_TYPES_NOT_ALLOWED_IN_PARAGRAPHS (tag.h:271-276)
		m := map[uint32]bool{}
		for _, n := range []string{"ADDRESS", "ARTICLE", "ASIDE", "BLOCKQUOTE", "DETAILS", "DIV", "DL",
			"FIELDSET", "FIGCAPTION", "FIGURE", "FOOTER", "FORM", "H1", "H2",
			"H3", "H4", "H5", "H6", "HEADER", "HR", "MAIN",
			"NAV", "OL", "P", "PRE", "SECTION"} {
			m[htmlCType(n)] = true
		}
		return m
	}()
)

// htmlCTag is the Tag of tag.h:171-174.
type htmlCTag struct {
	typ  uint32
	name []byte // custom_tag_name, for CUSTOM only
}

// htmlCTypeForName is tag_type_for_name (tag.h:278-290).
func htmlCTypeForName(name []byte) uint32 {
	for i, n := range htmlCTagNames {
		if string(name) == n {
			if i >= htmlCEndOfVoidTags {
				return uint32(i + 1)
			}
			return uint32(i)
		}
	}
	return htmlCCustom
}

// htmlCTagForName is tag_for_name (tag.h:298-307).
func htmlCTagForName(name []byte) htmlCTag {
	t := htmlCTag{typ: htmlCTypeForName(name)}
	if t.typ == htmlCCustom {
		t.name = name
	}
	return t
}

func htmlCTagEq(a, b *htmlCTag) bool { // tag_eq (tag.h:319-333)
	if a.typ != b.typ {
		return false
	}
	if a.typ == htmlCCustom {
		return bytes.Equal(a.name, b.name)
	}
	return true
}

// htmlCCanContain is tag_can_contain (tag.h:335-380).
func htmlCCanContain(self *htmlCTag, other *htmlCTag) bool {
	child := other.typ
	switch self.typ {
	case htmlCType("LI"):
		return child != htmlCType("LI")
	case htmlCType("DT"), htmlCType("DD"):
		return child != htmlCType("DT") && child != htmlCType("DD")
	case htmlCType("P"):
		return !htmlCNotInPar[child]
	case htmlCType("COLGROUP"):
		return child == htmlCType("COL")
	case htmlCType("RB"), htmlCType("RT"), htmlCType("RP"):
		return child != htmlCType("RB") && child != htmlCType("RT") && child != htmlCType("RP")
	case htmlCType("OPTGROUP"):
		return child != htmlCType("OPTGROUP")
	case htmlCType("TR"):
		return child != htmlCType("TR")
	case htmlCType("TD"), htmlCType("TH"):
		return child != htmlCType("TD") && child != htmlCType("TH") && child != htmlCType("TR")
	}
	return true
}

// htmlSerializeC is serialize (scanner.c:36-72). The first two bytes hold the number of
// tags written, the next two the number held; a tag that does not fit ends the list.
func htmlSerializeC(tags []htmlCTag, buf []byte) int {
	const limit = 1024 // TREE_SITTER_SERIALIZATION_BUFFER_SIZE
	if len(buf) < 4 {
		return 0
	}
	tagCount := len(tags)
	if tagCount > 0xFFFF {
		tagCount = 0xFFFF
	}
	serialized := 0
	size := 4
	for ; serialized < tagCount; serialized++ {
		tag := &tags[serialized]
		if tag.typ == htmlCCustom {
			n := len(tag.name)
			if n > 255 {
				n = 255
			}
			if size+2+n >= limit || size+2+n > len(buf) {
				break
			}
			buf[size] = byte(tag.typ)
			size++
			buf[size] = byte(n)
			size++
			copy(buf[size:], tag.name[:n])
			size += n
		} else {
			if size+1 >= limit || size+1 > len(buf) {
				break
			}
			buf[size] = byte(tag.typ)
			size++
		}
	}
	binary.LittleEndian.PutUint16(buf[0:], uint16(serialized))
	binary.LittleEndian.PutUint16(buf[2:], uint16(tagCount))
	return size
}

// htmlDeserializeC is deserialize (scanner.c:74-110). Tags that the buffer had no room for come
// back as END_ tags.
func htmlDeserializeC(buf []byte) []htmlCTag {
	var tags []htmlCTag
	if len(buf) == 0 {
		return tags
	}
	at := func(i int) byte {
		if i < len(buf) {
			return buf[i]
		}
		return 0
	}
	serialized := int(at(0)) | int(at(1))<<8
	tagCount := int(at(2)) | int(at(3))<<8
	size := 4
	if tagCount > 0 {
		iter := 0
		for ; iter < serialized; iter++ {
			tag := htmlCTag{typ: uint32(int32(int8(at(size))))}
			size++
			if tag.typ == htmlCCustom {
				n := int(at(size))
				size++
				for j := 0; j < n; j++ {
					tag.name = append(tag.name, at(size+j))
				}
				size += n
			}
			tags = append(tags, tag)
		}
		for ; iter < tagCount; iter++ {
			tags = append(tags, htmlCTag{typ: htmlCEnd})
		}
	}
	return tags
}

type htmlC struct {
	lx   *gotreesitter.ExternalLexer
	vs   []bool
	syms *[htmlTokenCount]gotreesitter.Symbol
	s    *htmlScannerState
}

func (c *htmlC) la() int32   { return scanmode.CLookahead(c.lx) }
func (c *htmlC) eof() bool   { return scanmode.AtEOF(c.lx) }
func (c *htmlC) adv()        { c.lx.Advance(false) }
func (c *htmlC) skip()       { c.lx.Advance(true) }
func (c *htmlC) mark()       { c.lx.MarkEnd() }
func (c *htmlC) res(tok int) { c.lx.SetResultSymbol(c.syms[tok]) }
func (c *htmlC) valid(tok int) bool {
	return tok < len(c.vs) && c.vs[tok]
}
func (c *htmlC) pop() { c.s.ctags = c.s.ctags[:len(c.s.ctags)-1] }

// htmlCUpper is towupper in the C locale.
func htmlCUpper(ch int32) int32 {
	if ch >= 'a' && ch <= 'z' {
		return ch - 'a' + 'A'
	}
	return ch
}

// scanTagName is scan_tag_name (scanner.c:112-119).
func (c *htmlC) scanTagName() []byte {
	var name []byte
	for cIsAlnum(rune(c.la())) || c.la() == '-' || c.la() == ':' {
		name = append(name, byte(htmlCUpper(c.la())))
		c.adv()
	}
	return name
}

// scanComment is scan_comment (scanner.c:121-150).
func (c *htmlC) scanComment() bool {
	if c.la() != '-' {
		return false
	}
	c.adv()
	if c.la() != '-' {
		return false
	}
	c.adv()
	dashes := 0
	for c.la() != 0 {
		switch c.la() {
		case '-':
			dashes++
		case '>':
			if dashes >= 2 {
				c.res(htmlTokComment)
				c.adv()
				c.mark()
				return true
			}
			dashes = 0
		default:
			dashes = 0
		}
		c.adv()
	}
	return false
}

// scanRawText is scan_raw_text (scanner.c:152-180).
func (c *htmlC) scanRawText() bool {
	if len(c.s.ctags) == 0 {
		return false
	}
	c.mark()
	end := "</STYLE"
	if c.s.ctags[len(c.s.ctags)-1].typ == htmlCScript {
		end = "</SCRIPT"
	}
	idx := 0
	for c.la() != 0 {
		if htmlCUpper(c.la()) == int32(end[idx]) {
			idx++
			if idx == len(end) {
				break
			}
			c.adv()
		} else {
			idx = 0
			c.adv()
			c.mark()
		}
	}
	c.res(htmlTokRawText)
	return true
}

// scanImplicitEndTag is scan_implicit_end_tag (scanner.c:187-245).
func (c *htmlC) scanImplicitEndTag() bool {
	var parent *htmlCTag
	if n := len(c.s.ctags); n > 0 {
		parent = &c.s.ctags[n-1]
	}
	isClosing := false
	if c.la() == '/' {
		isClosing = true
		c.adv()
	} else if parent != nil && parent.typ < htmlCEndOfVoidTags {
		c.pop()
		c.res(htmlTokImplicitEndTag)
		return true
	}
	name := c.scanTagName()
	if len(name) == 0 && !c.eof() {
		return false
	}
	next := htmlCTagForName(name)
	if isClosing {
		// The tag correctly closes the topmost element on the stack
		if n := len(c.s.ctags); n > 0 && htmlCTagEq(&c.s.ctags[n-1], &next) {
			return false
		}
		// Otherwise, dig deeper and queue implicit end tags
		for i := len(c.s.ctags); i > 0; i-- {
			if c.s.ctags[i-1].typ == next.typ {
				c.pop()
				c.res(htmlTokImplicitEndTag)
				return true
			}
		}
	} else if parent != nil &&
		(!htmlCCanContain(parent, &next) ||
			((parent.typ == htmlCHTML || parent.typ == htmlCHead || parent.typ == htmlCBody) && c.eof())) {
		c.pop()
		c.res(htmlTokImplicitEndTag)
		return true
	}
	return false
}

// scanStartTagName is scan_start_tag_name (scanner.c:247-270).
func (c *htmlC) scanStartTagName() bool {
	name := c.scanTagName()
	if len(name) == 0 {
		return false
	}
	tag := htmlCTagForName(name)
	c.s.ctags = append(c.s.ctags, tag)
	switch tag.typ {
	case htmlCScript:
		c.res(htmlTokScriptTagName)
	case htmlCStyle:
		c.res(htmlTokStyleTagName)
	default:
		c.res(htmlTokStartTagName)
	}
	return true
}

// scanEndTagName is scan_end_tag_name (scanner.c:272-291).
func (c *htmlC) scanEndTagName() bool {
	name := c.scanTagName()
	if len(name) == 0 {
		return false
	}
	tag := htmlCTagForName(name)
	if n := len(c.s.ctags); n > 0 && htmlCTagEq(&c.s.ctags[n-1], &tag) {
		c.pop()
		c.res(htmlTokEndTagName)
	} else {
		c.res(htmlTokErrEndName)
	}
	return true
}

// scanSelfClosing is scan_self_closing_tag_delimiter (scanner.c:293-304). With an empty stack C
// returns true with the result symbol it started with (0).
func (c *htmlC) scanSelfClosing() bool {
	c.adv()
	if c.la() == '>' {
		c.adv()
		if len(c.s.ctags) > 0 {
			c.pop()
			c.res(htmlTokSelfClosingTag)
		} else {
			// C's lexer starts every scan with result_symbol 0, which is the external
			// START_TAG_NAME: the scan returns true with that symbol.
			c.res(htmlTokStartTagName)
		}
		return true
	}
	return false
}

// htmlScanC is scan (scanner.c:306-346).
func htmlScanC(s *htmlScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[htmlTokenCount]gotreesitter.Symbol) bool {
	c := &htmlC{lx: lexer, vs: validSymbols, syms: syms, s: s}
	if c.valid(htmlTokRawText) && !c.valid(htmlTokStartTagName) && !c.valid(htmlTokEndTagName) {
		return c.scanRawText()
	}
	for cIsSpace(rune(c.la())) {
		c.skip()
	}
	switch c.la() {
	case '<':
		c.mark()
		c.adv()
		if c.la() == '!' {
			c.adv()
			return c.scanComment()
		}
		if c.valid(htmlTokImplicitEndTag) {
			return c.scanImplicitEndTag()
		}
	case 0:
		if c.valid(htmlTokImplicitEndTag) {
			return c.scanImplicitEndTag()
		}
	case '/':
		if c.valid(htmlTokSelfClosingTag) {
			return c.scanSelfClosing()
		}
	default:
		if (c.valid(htmlTokStartTagName) || c.valid(htmlTokEndTagName)) && !c.valid(htmlTokRawText) {
			if c.valid(htmlTokStartTagName) {
				return c.scanStartTagName()
			}
			return c.scanEndTagName()
		}
	}
	return false
}
