package grammars

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"
)

// HTMLTokenSource is a lightweight lexer bridge for tree-sitter-html.
// It targets standard tag/text/comment flows and is intended for parser
// coverage and editor use-cases.
type HTMLTokenSource struct {
	src  []byte
	lang *gotreesitter.Language
	cur  sourceCursor

	done bool

	inTag         bool
	inEndTag      bool
	expectTagName bool

	eofSymbol gotreesitter.Symbol

	ltSym      gotreesitter.Symbol
	ltSlashSym gotreesitter.Symbol
	gtSym      gotreesitter.Symbol
	slashGtSym gotreesitter.Symbol
	eqSym      gotreesitter.Symbol

	openTagNameSym gotreesitter.Symbol
	endTagNameSym  gotreesitter.Symbol
	attrNameSym  gotreesitter.Symbol
	attrValueSym gotreesitter.Symbol
	textSym      gotreesitter.Symbol
	commentSym   gotreesitter.Symbol
}

// NewHTMLTokenSource creates a token source for HTML source text.
func NewHTMLTokenSource(src []byte, lang *gotreesitter.Language) (*HTMLTokenSource, error) {
	if lang == nil {
		return nil, fmt.Errorf("html lexer: language is nil")
	}

	lookup := newTokenLookup(lang, "html")

	ts := &HTMLTokenSource{
		src:  src,
		lang: lang,
		cur:  newSourceCursor(src),
	}

	ts.eofSymbol, _ = lang.SymbolByName("end")
	ts.ltSym = lookup.require("<")
	ts.ltSlashSym = lookup.require("</")
	ts.gtSym = lookup.require(">")
	ts.slashGtSym = lookup.optional("/>")
	ts.eqSym = lookup.optional("=")
	tagNameSyms := lang.TokenSymbolsByName("tag_name")
	if len(tagNameSyms) == 0 {
		lookup.require("tag_name")
	} else {
		ts.openTagNameSym = tagNameSyms[0]
		ts.endTagNameSym = tagNameSyms[len(tagNameSyms)-1]
	}
	ts.attrNameSym = lookup.optional("attribute_name")
	ts.attrValueSym = lookup.optional("attribute_value")
	ts.textSym = lookup.optional("text")
	ts.commentSym = lookup.optional("comment")

	if err := lookup.err(); err != nil {
		return nil, err
	}
	if ts.textSym == 0 {
		return nil, fmt.Errorf("html lexer: token symbol %q not found", "text")
	}
	if ts.attrNameSym == 0 {
		ts.attrNameSym = ts.openTagNameSym
	}
	if ts.attrValueSym == 0 {
		ts.attrValueSym = ts.textSym
	}
	if ts.endTagNameSym == 0 {
		ts.endTagNameSym = ts.openTagNameSym
	}

	return ts, nil
}

// NewHTMLTokenSourceOrEOF returns an HTML token source, or EOF-only fallback if
// setup fails.
func NewHTMLTokenSourceOrEOF(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ts, err := NewHTMLTokenSource(src, lang)
	if err != nil {
		return tokenSourceInitError{sourceLen: uint32(len(src))}
	}
	return ts
}

func (ts *HTMLTokenSource) Next() gotreesitter.Token {
	if ts.done {
		return ts.eofToken()
	}
	if ts.cur.eof() {
		ts.done = true
		return ts.eofToken()
	}

	for {
		if ts.cur.eof() {
			ts.done = true
			return ts.eofToken()
		}

		if !ts.inTag {
			if tok, ok := ts.commentToken(); ok {
				return tok
			}
			if ts.matchLiteralAtCurrent("</") {
				ts.inTag = true
				ts.inEndTag = true
				ts.expectTagName = true
				return ts.literalToken(ts.ltSlashSym, 2)
			}
			if ts.matchLiteralAtCurrent("<") {
				ts.inTag = true
				ts.inEndTag = false
				ts.expectTagName = true
				return ts.literalToken(ts.ltSym, 1)
			}
			return ts.textToken()
		}

		// In tag mode.
		if ts.cur.peekByte() == ' ' || ts.cur.peekByte() == '\t' || ts.cur.peekByte() == '\n' || ts.cur.peekByte() == '\r' || ts.cur.peekByte() == '\f' {
			ts.cur.advanceByte()
			continue
		}

		if ts.matchLiteralAtCurrent("/>") && ts.slashGtSym != 0 {
			ts.inTag = false
			ts.inEndTag = false
			ts.expectTagName = false
			return ts.literalToken(ts.slashGtSym, 2)
		}
		if ts.matchLiteralAtCurrent(">") {
			ts.inTag = false
			ts.inEndTag = false
			ts.expectTagName = false
			return ts.literalToken(ts.gtSym, 1)
		}
		if ts.matchLiteralAtCurrent("=") && ts.eqSym != 0 {
			return ts.literalToken(ts.eqSym, 1)
		}
		if ts.cur.peekByte() == '"' || ts.cur.peekByte() == '\'' {
			return ts.quotedAttributeValue()
		}
		if isHTMLNameStart(ts.cur.peekByte()) {
			return ts.nameToken()
		}

		// Skip unexpected bytes inside tags.
		ts.cur.advanceRune()
	}
}

func (ts *HTMLTokenSource) SkipToByte(offset uint32) gotreesitter.Token {
	target := int(offset)
	if target < 0 {
		target = 0
	}
	if target > len(ts.src) {
		target = len(ts.src)
	}

	ts.done = false
	if target < ts.cur.offset {
		ts.cur = newSourceCursor(ts.src)
		ts.inTag = false
		ts.inEndTag = false
		ts.expectTagName = false
	}
	for ts.cur.offset < target {
		ts.cur.advanceRune()
	}
	if ts.cur.eof() {
		ts.done = true
		return ts.eofToken()
	}
	return ts.Next()
}

func (ts *HTMLTokenSource) commentToken() (gotreesitter.Token, bool) {
	if ts.commentSym == 0 || !ts.matchLiteralAtCurrent("<!--") {
		return gotreesitter.Token{}, false
	}
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(4)
	for !ts.cur.eof() {
		if ts.matchLiteralAtCurrent("-->") {
			ts.advanceBytes(3)
			break
		}
		ts.cur.advanceRune()
	}
	return makeToken(ts.commentSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *HTMLTokenSource) textToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	for !ts.cur.eof() && ts.cur.peekByte() != '<' {
		ts.cur.advanceRune()
	}
	return makeToken(ts.textSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *HTMLTokenSource) quotedAttributeValue() gotreesitter.Token {
	quote := ts.cur.peekByte()
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() {
		ch := ts.cur.peekByte()
		if ch == quote {
			ts.cur.advanceByte()
			break
		}
		ts.cur.advanceRune()
	}
	return makeToken(ts.attrValueSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *HTMLTokenSource) nameToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() && isHTMLNamePart(ts.cur.peekByte()) {
		ts.cur.advanceByte()
	}

	sym := ts.attrNameSym
	if ts.expectTagName {
		if ts.inEndTag {
			sym = ts.endTagNameSym
		} else {
			sym = ts.openTagNameSym
		}
		ts.expectTagName = false
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *HTMLTokenSource) literalToken(sym gotreesitter.Symbol, n int) gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(n)
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *HTMLTokenSource) matchLiteralAtCurrent(lexeme string) bool {
	if ts.cur.offset+len(lexeme) > len(ts.src) {
		return false
	}
	for i := 0; i < len(lexeme); i++ {
		if ts.src[ts.cur.offset+i] != lexeme[i] {
			return false
		}
	}
	return true
}

func (ts *HTMLTokenSource) advanceBytes(n int) {
	for i := 0; i < n && !ts.cur.eof(); i++ {
		ts.cur.advanceByte()
	}
}

func (ts *HTMLTokenSource) eofToken() gotreesitter.Token {
	n := uint32(len(ts.src))
	pt := ts.cur.point()
	return gotreesitter.Token{
		Symbol:     ts.eofSymbol,
		StartByte:  n,
		EndByte:    n,
		StartPoint: pt,
		EndPoint:   pt,
	}
}

func isHTMLNameStart(b byte) bool {
	return isASCIIAlpha(b) || b == '_' || b == ':'
}

func isHTMLNamePart(b byte) bool {
	return isHTMLNameStart(b) || isASCIIDigit(b) || b == '-' || b == '.'
}
