//go:build !grammar_subset || grammar_subset_authzed || grammar_subset_c || grammar_subset_cpp || grammar_subset_go || grammar_subset_html || grammar_subset_java || grammar_subset_json || grammar_subset_lua || grammar_subset_toml

package grammarruntime

import (
	"fmt"
	"strings"
	"unicode/utf8"
	"unsafe"

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

// skipSpacesAndTabs skips horizontal whitespace only (spaces, tabs, \r, \f).
// Use this instead of skipWhitespace when newlines are significant tokens.
func (c *sourceCursor) skipSpacesAndTabs() {
	for !c.eof() {
		switch c.peekByte() {
		case ' ', '\t', '\r', '\f':
			c.advanceByte()
		default:
			return
		}
	}
}

func (c *sourceCursor) skipEscapedNewline() bool {
	if c.eof() || c.peekByte() != '\\' {
		return false
	}
	next := c.offset + 1
	if next >= len(c.src) {
		return false
	}
	switch c.src[next] {
	case '\n':
		c.advanceByte()
		c.advanceByte()
		return true
	case '\r':
		if next+1 >= len(c.src) || c.src[next+1] != '\n' {
			return false
		}
		c.advanceByte()
		c.advanceByte()
		c.advanceByte()
		return true
	default:
		return false
	}
}

func (c *sourceCursor) skipSpacesTabsAndEscapedNewlines() {
	for {
		c.skipSpacesAndTabs()
		if !c.skipEscapedNewline() {
			return
		}
	}
}

func (c *sourceCursor) matchLiteralAtCurrent(lexeme string) bool {
	if c.offset+len(lexeme) > len(c.src) {
		return false
	}
	for i := 0; i < len(lexeme); i++ {
		if c.src[c.offset+i] != lexeme[i] {
			return false
		}
	}
	return true
}

func (c *sourceCursor) advanceBytes(n int) {
	for i := 0; i < n && !c.eof(); i++ {
		c.advanceByte()
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
	idx := strings.LastIndex(name, "_token")
	if idx < 0 {
		return false
	}
	suffix := name[idx+len("_token"):]
	if len(suffix) == 0 {
		return false
	}
	for i := 0; i < len(suffix); i++ {
		if suffix[i] < '0' || suffix[i] > '9' {
			return false
		}
	}
	return true
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
		Text:       bytesToStringNoCopy(src[startOffset:endOffset]),
		StartByte:  uint32(startOffset),
		EndByte:    uint32(endOffset),
		StartPoint: startPoint,
		EndPoint:   endPoint,
	}
}

func bytesToStringNoCopy(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

type tokenLookup struct {
	lang      *gotreesitter.Language
	lexerName string
	firstErr  error
}

func newTokenLookup(lang *gotreesitter.Language, lexerName string) *tokenLookup {
	return &tokenLookup{lang: lang, lexerName: lexerName}
}

type tokenSourceInitError struct {
	sourceLen uint32
}

func (e tokenSourceInitError) Next() gotreesitter.Token {
	return gotreesitter.Token{
		StartByte: e.sourceLen,
		EndByte:   e.sourceLen,
	}
}

func (e tokenSourceInitError) SkipToByte(offset uint32) gotreesitter.Token {
	if offset > e.sourceLen {
		offset = e.sourceLen
	}
	return gotreesitter.Token{
		StartByte: offset,
		EndByte:   offset,
	}
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
