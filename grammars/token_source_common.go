package grammars

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
)

// sourceCursor tracks byte offset and row/column while scanning source bytes.
type sourceCursor struct {
	src    []byte
	offset int
	row    uint32
	col    uint32
}

func newSourceCursor(src []byte) sourceCursor {
	return sourceCursor{src: src}
}

func (c *sourceCursor) eof() bool {
	return c.offset >= len(c.src)
}

func (c *sourceCursor) point() gotreesitter.Point {
	return gotreesitter.Point{Row: c.row, Column: c.col}
}

func (c *sourceCursor) peekByte() byte {
	if c.eof() {
		return 0
	}
	return c.src[c.offset]
}

func (c *sourceCursor) advanceByte() {
	if c.eof() {
		return
	}
	b := c.src[c.offset]
	c.offset++
	if b == '\n' {
		c.row++
		c.col = 0
		return
	}
	c.col++
}

func (c *sourceCursor) advanceRune() {
	if c.eof() {
		return
	}
	r, size := utf8.DecodeRune(c.src[c.offset:])
	c.offset += size
	if r == '\n' {
		c.row++
		c.col = 0
		return
	}
	c.col++
}

func (c *sourceCursor) skipWhitespace() {
	for !c.eof() {
		switch c.peekByte() {
		case ' ', '\t', '\n', '\r', '\f':
			c.advanceByte()
		default:
			return
		}
	}
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isASCIIHex(b byte) bool {
	return isASCIIDigit(b) || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func isASCIIWordStart(b byte) bool {
	return isASCIIAlpha(b) || b == '_'
}

func isASCIIWordPart(b byte) bool {
	return isASCIIWordStart(b) || isASCIIDigit(b) || b == '$'
}

func isTokenNameWord(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		b := name[i]
		if i == 0 && !isASCIIWordStart(b) {
			return false
		}
		if isASCIIWordPart(b) || b == '-' {
			continue
		}
		return false
	}
	return true
}

func isSyntheticTokenName(name string) bool {
	return strings.Contains(name, "_token")
}

func normalizeTokenLexeme(name string) string {
	if name == "" {
		return ""
	}
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		if name[i] == '\\' && i+1 < len(name) {
			i++
		}
		out = append(out, name[i])
	}
	return string(out)
}

func tokenNameEscapeCount(name string) int {
	count := 0
	for i := 0; i < len(name); i++ {
		if name[i] == '\\' {
			count++
		}
	}
	return count
}

func lexemeNeedsBoundary(lexeme string) bool {
	if lexeme == "" {
		return false
	}
	last := lexeme[len(lexeme)-1]
	return isASCIIWordPart(last)
}

func hasWordBoundaryAfter(src []byte, endOffset int) bool {
	if endOffset >= len(src) {
		return true
	}
	return !isASCIIWordPart(src[endOffset])
}

func makeToken(sym gotreesitter.Symbol, src []byte, startOffset, endOffset int, startPoint, endPoint gotreesitter.Point) gotreesitter.Token {
	return gotreesitter.Token{
		Symbol:     sym,
		Text:       string(src[startOffset:endOffset]),
		StartByte:  uint32(startOffset),
		EndByte:    uint32(endOffset),
		StartPoint: startPoint,
		EndPoint:   endPoint,
	}
}

type tokenLookup struct {
	lang      *gotreesitter.Language
	lexerName string
	firstErr  error
}

func newTokenLookup(lang *gotreesitter.Language, lexerName string) *tokenLookup {
	return &tokenLookup{lang: lang, lexerName: lexerName}
}

func (tl *tokenLookup) require(name string) gotreesitter.Symbol {
	syms := tl.lang.TokenSymbolsByName(name)
	if len(syms) == 0 {
		if tl.firstErr == nil {
			tl.firstErr = fmt.Errorf("%s lexer: token symbol %q not found", tl.lexerName, name)
		}
		return 0
	}
	return syms[0]
}

func (tl *tokenLookup) optional(names ...string) gotreesitter.Symbol {
	for _, name := range names {
		syms := tl.lang.TokenSymbolsByName(name)
		if len(syms) > 0 {
			return syms[0]
		}
	}
	return 0
}

func (tl *tokenLookup) err() error {
	return tl.firstErr
}
