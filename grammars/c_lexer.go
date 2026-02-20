package grammars

import (
	"fmt"

	"github.com/odvcencio/mane/gotreesitter"
)

// CTokenSource is a lightweight lexer bridge for tree-sitter-c.
type CTokenSource struct {
	src  []byte
	lang *gotreesitter.Language
	cur  sourceCursor

	done    bool
	pending []gotreesitter.Token

	eofSymbol           gotreesitter.Symbol
	identifierSymbol    gotreesitter.Symbol
	numberSymbol        gotreesitter.Symbol
	commentSymbol       gotreesitter.Symbol
	quoteSymbol         gotreesitter.Symbol
	apostropheSymbol    gotreesitter.Symbol
	stringContentSymbol gotreesitter.Symbol
	escapeSymbol        gotreesitter.Symbol
	characterSymbol     gotreesitter.Symbol
	primitiveTypeSymbol gotreesitter.Symbol

	keywordSymbols map[string]gotreesitter.Symbol
	literalSymbols map[string]gotreesitter.Symbol
	maxLiteralLen  int

	stringOpeners []prefixedToken
	charOpeners   []prefixedToken
}

type prefixedToken struct {
	lexeme string
	sym    gotreesitter.Symbol
}

// NewCTokenSource creates a token source for C source text.
func NewCTokenSource(src []byte, lang *gotreesitter.Language) (*CTokenSource, error) {
	if lang == nil {
		return nil, fmt.Errorf("c lexer: language is nil")
	}

	ts := &CTokenSource{
		src:            src,
		lang:           lang,
		cur:            newSourceCursor(src),
		keywordSymbols: make(map[string]gotreesitter.Symbol),
		literalSymbols: make(map[string]gotreesitter.Symbol),
	}

	tl := newTokenLookup(lang, "c")
	ts.identifierSymbol = tl.require("identifier")
	ts.numberSymbol = tl.require("number_literal")
	ts.commentSymbol = tl.optional("comment")
	ts.quoteSymbol = tl.optional("\"")
	ts.apostropheSymbol = tl.optional("'")
	ts.stringContentSymbol = tl.optional("string_content")
	ts.escapeSymbol = tl.optional("escape_sequence")
	ts.characterSymbol = tl.optional("character")
	ts.primitiveTypeSymbol = tl.optional("primitive_type")

	if ts.eofSymbol, _ = lang.SymbolByName("end"); ts.eofSymbol == 0 {
		ts.eofSymbol = 0
	}

	ts.buildSymbolTables()
	ts.fixAmbiguousTokenChoices()
	ts.initDelimiters()

	if err := tl.err(); err != nil {
		return nil, err
	}
	return ts, nil
}

// NewCTokenSourceOrEOF returns a C token source, or EOF-only fallback if
// symbol setup fails.
func NewCTokenSourceOrEOF(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		return tokenSourceInitError{sourceLen: uint32(len(src))}
	}
	return ts
}

func (ts *CTokenSource) Next() gotreesitter.Token {
	if len(ts.pending) > 0 {
		tok := ts.pending[0]
		ts.pending = ts.pending[1:]
		return tok
	}
	if ts.done {
		return ts.eofToken()
	}

	for {
		ts.cur.skipWhitespace()
		if ts.cur.eof() {
			ts.done = true
			return ts.eofToken()
		}

		if tok, ok := ts.commentToken(); ok {
			if tok.Symbol == 0 {
				continue
			}
			return tok
		}
		if tok, ok := ts.stringToken(); ok {
			return tok
		}
		if tok, ok := ts.charToken(); ok {
			return tok
		}

		b := ts.cur.peekByte()
		if isCIdentStart(b) {
			return ts.identifierOrKeywordToken()
		}
		if isASCIIDigit(b) {
			return ts.numberToken()
		}
		if tok, ok := ts.literalToken(); ok {
			return tok
		}

		// Unknown byte: consume one rune and continue.
		ts.cur.advanceRune()
	}
}

func (ts *CTokenSource) SkipToByte(offset uint32) gotreesitter.Token {
	target := int(offset)
	if target < 0 {
		target = 0
	}
	if target > len(ts.src) {
		target = len(ts.src)
	}

	ts.pending = nil
	ts.done = false

	if target < ts.cur.offset {
		ts.cur = newSourceCursor(ts.src)
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

func (ts *CTokenSource) buildSymbolTables() {
	limit := int(ts.lang.TokenCount)
	if limit > len(ts.lang.SymbolNames) {
		limit = len(ts.lang.SymbolNames)
	}
	literalEscapes := make(map[string]int)

	for i := 0; i < limit; i++ {
		name := ts.lang.SymbolNames[i]
		if name == "" || name == "end" {
			continue
		}
		sym := gotreesitter.Symbol(i)

		switch name {
		case "identifier", "number_literal", "comment", "string_content", "escape_sequence", "character", "primitive_type", "preproc_arg", "preproc_directive", "preproc_include_token2", "system_lib_string":
			continue
		}
		if isSyntheticTokenName(name) {
			continue
		}

		if isTokenNameWord(name) {
			if _, exists := ts.keywordSymbols[name]; !exists {
				ts.keywordSymbols[name] = sym
			}
			continue
		}

		lexeme := normalizeTokenLexeme(name)
		if lexeme == "" {
			continue
		}
		escapes := tokenNameEscapeCount(name)
		if prev, exists := literalEscapes[lexeme]; exists && prev <= escapes {
			continue
		}
		ts.literalSymbols[lexeme] = sym
		literalEscapes[lexeme] = escapes
		if len(lexeme) > ts.maxLiteralLen {
			ts.maxLiteralLen = len(lexeme)
		}
	}
}

func (ts *CTokenSource) fixAmbiguousTokenChoices() {
	// tree-sitter-c has two distinct token IDs that display as "(".
	// The declaration/parser path expects the higher-ID variant.
	if syms := ts.lang.TokenSymbolsByName("("); len(syms) > 0 {
		ts.literalSymbols["("] = syms[len(syms)-1]
	}
}

func (ts *CTokenSource) initDelimiters() {
	ts.stringOpeners = ts.collectOpeners([]string{"u8\"", "L\"", "U\"", "u\"", "\""}, ts.quoteSymbol)
	ts.charOpeners = ts.collectOpeners([]string{"u8'", "L'", "U'", "u'", "'"}, ts.apostropheSymbol)
}

func (ts *CTokenSource) collectOpeners(lexemes []string, fallback gotreesitter.Symbol) []prefixedToken {
	out := make([]prefixedToken, 0, len(lexemes))
	for _, lex := range lexemes {
		sym := ts.literalSymbols[lex]
		if sym == 0 && len(lex) == 1 {
			sym = fallback
		}
		if sym == 0 {
			continue
		}
		out = append(out, prefixedToken{lexeme: lex, sym: sym})
	}
	return out
}

func (ts *CTokenSource) commentToken() (gotreesitter.Token, bool) {
	if ts.cur.offset+1 >= len(ts.src) || ts.src[ts.cur.offset] != '/' {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	next := ts.src[ts.cur.offset+1]
	if next != '/' && next != '*' {
		return gotreesitter.Token{}, false
	}

	ts.cur.advanceByte()
	ts.cur.advanceByte()
	if next == '/' {
		for !ts.cur.eof() && ts.cur.peekByte() != '\n' {
			ts.cur.advanceRune()
		}
	} else {
		for !ts.cur.eof() {
			if ts.cur.peekByte() == '*' && ts.cur.offset+1 < len(ts.src) && ts.src[ts.cur.offset+1] == '/' {
				ts.cur.advanceByte()
				ts.cur.advanceByte()
				break
			}
			ts.cur.advanceRune()
		}
	}

	if ts.commentSymbol == 0 {
		return gotreesitter.Token{Symbol: 0}, true
	}
	return makeToken(ts.commentSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *CTokenSource) stringToken() (gotreesitter.Token, bool) {
	for _, opener := range ts.stringOpeners {
		if !ts.matchAt(opener.lexeme) {
			continue
		}
		start := ts.cur.offset
		startPt := ts.cur.point()
		ts.advanceBytes(len(opener.lexeme))
		openTok := makeToken(opener.sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
		closeSym := ts.quoteSymbol
		if closeSym == 0 {
			closeSym = opener.sym
		}
		ts.scanDelimitedBody('"', ts.stringContentSymbol, ts.escapeSymbol, closeSym)
		return openTok, true
	}
	return gotreesitter.Token{}, false
}

func (ts *CTokenSource) charToken() (gotreesitter.Token, bool) {
	for _, opener := range ts.charOpeners {
		if !ts.matchAt(opener.lexeme) {
			continue
		}
		start := ts.cur.offset
		startPt := ts.cur.point()
		ts.advanceBytes(len(opener.lexeme))
		openTok := makeToken(opener.sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
		closeSym := ts.apostropheSymbol
		if closeSym == 0 {
			closeSym = opener.sym
		}
		ts.scanDelimitedBody('\'', ts.characterSymbol, ts.escapeSymbol, closeSym)
		return openTok, true
	}
	return gotreesitter.Token{}, false
}

func (ts *CTokenSource) scanDelimitedBody(close byte, contentSym, escapeSym, closeSym gotreesitter.Symbol) {
	segStart := ts.cur.offset
	segStartPt := ts.cur.point()

	for !ts.cur.eof() {
		ch := ts.cur.peekByte()
		if ch == close {
			if contentSym != 0 && segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(contentSym, ts.src, segStart, ts.cur.offset, segStartPt, ts.cur.point()))
			}
			closeStart := ts.cur.offset
			closeStartPt := ts.cur.point()
			ts.cur.advanceByte()
			if closeSym != 0 {
				ts.pending = append(ts.pending, makeToken(closeSym, ts.src, closeStart, ts.cur.offset, closeStartPt, ts.cur.point()))
			}
			return
		}

		if ch == '\\' {
			if contentSym != 0 && segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(contentSym, ts.src, segStart, ts.cur.offset, segStartPt, ts.cur.point()))
			}
			escStart := ts.cur.offset
			escStartPt := ts.cur.point()
			ts.cur.advanceByte()
			if !ts.cur.eof() {
				switch ts.cur.peekByte() {
				case 'x':
					ts.cur.advanceByte()
					for i := 0; i < 2 && !ts.cur.eof() && isASCIIHex(ts.cur.peekByte()); i++ {
						ts.cur.advanceByte()
					}
				case 'u':
					ts.cur.advanceByte()
					for i := 0; i < 4 && !ts.cur.eof() && isASCIIHex(ts.cur.peekByte()); i++ {
						ts.cur.advanceByte()
					}
				case 'U':
					ts.cur.advanceByte()
					for i := 0; i < 8 && !ts.cur.eof() && isASCIIHex(ts.cur.peekByte()); i++ {
						ts.cur.advanceByte()
					}
				default:
					ts.cur.advanceRune()
				}
			}
			if escapeSym != 0 {
				ts.pending = append(ts.pending, makeToken(escapeSym, ts.src, escStart, ts.cur.offset, escStartPt, ts.cur.point()))
			} else if contentSym != 0 {
				ts.pending = append(ts.pending, makeToken(contentSym, ts.src, escStart, ts.cur.offset, escStartPt, ts.cur.point()))
			}
			segStart = ts.cur.offset
			segStartPt = ts.cur.point()
			continue
		}

		ts.cur.advanceRune()
	}

	if contentSym != 0 && segStart < ts.cur.offset {
		ts.pending = append(ts.pending, makeToken(contentSym, ts.src, segStart, ts.cur.offset, segStartPt, ts.cur.point()))
	}
}

func (ts *CTokenSource) identifierOrKeywordToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() && isCIdentPart(ts.cur.peekByte()) {
		ts.cur.advanceByte()
	}

	text := string(ts.src[start:ts.cur.offset])
	sym := ts.identifierSymbol
	if kw, ok := ts.keywordSymbols[text]; ok {
		sym = kw
	} else if ts.primitiveTypeSymbol != 0 && isCPrimitiveType(text) {
		sym = ts.primitiveTypeSymbol
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *CTokenSource) numberToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()

	if ts.cur.peekByte() == '0' && ts.cur.offset+1 < len(ts.src) && (ts.src[ts.cur.offset+1] == 'x' || ts.src[ts.cur.offset+1] == 'X') {
		ts.cur.advanceByte()
		ts.cur.advanceByte()
		for !ts.cur.eof() && (isASCIIHex(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	} else {
		for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	}

	if !ts.cur.eof() && ts.cur.peekByte() == '.' {
		ts.cur.advanceByte()
		for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	}

	if !ts.cur.eof() && (ts.cur.peekByte() == 'e' || ts.cur.peekByte() == 'E' || ts.cur.peekByte() == 'p' || ts.cur.peekByte() == 'P') {
		ts.cur.advanceByte()
		if !ts.cur.eof() && (ts.cur.peekByte() == '+' || ts.cur.peekByte() == '-') {
			ts.cur.advanceByte()
		}
		for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	}

	for !ts.cur.eof() {
		b := ts.cur.peekByte()
		if isASCIIAlpha(b) || isASCIIDigit(b) || b == '_' {
			ts.cur.advanceByte()
			continue
		}
		break
	}

	return makeToken(ts.numberSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *CTokenSource) literalToken() (gotreesitter.Token, bool) {
	sym, n := ts.matchLiteral()
	if sym == 0 {
		return gotreesitter.Token{}, false
	}
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(n)
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *CTokenSource) matchLiteral() (gotreesitter.Symbol, int) {
	remaining := len(ts.src) - ts.cur.offset
	maxN := ts.maxLiteralLen
	if maxN > remaining {
		maxN = remaining
	}

	for n := maxN; n >= 1; n-- {
		lex := string(ts.src[ts.cur.offset : ts.cur.offset+n])
		sym, ok := ts.literalSymbols[lex]
		if !ok {
			continue
		}
		if lexemeNeedsBoundary(lex) && !hasWordBoundaryAfter(ts.src, ts.cur.offset+n) {
			continue
		}
		return sym, n
	}
	return 0, 0
}

func (ts *CTokenSource) matchAt(lexeme string) bool {
	if ts.cur.offset+len(lexeme) > len(ts.src) {
		return false
	}
	for i := 0; i < len(lexeme); i++ {
		if ts.src[ts.cur.offset+i] != lexeme[i] {
			return false
		}
	}
	if lexemeNeedsBoundary(lexeme) && !hasWordBoundaryAfter(ts.src, ts.cur.offset+len(lexeme)) {
		return false
	}
	return true
}

func (ts *CTokenSource) advanceBytes(n int) {
	for i := 0; i < n && !ts.cur.eof(); i++ {
		ts.cur.advanceByte()
	}
}

func (ts *CTokenSource) eofToken() gotreesitter.Token {
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

func isCIdentStart(b byte) bool {
	return isASCIIAlpha(b) || b == '_'
}

func isCIdentPart(b byte) bool {
	return isCIdentStart(b) || isASCIIDigit(b)
}

func isCPrimitiveType(text string) bool {
	switch text {
	case "char", "int", "float", "double", "void", "_Bool", "_Complex", "bool", "__int128", "size_t", "ssize_t", "ptrdiff_t", "intptr_t", "uintptr_t", "wchar_t", "char16_t", "char32_t":
		return true
	default:
		return false
	}
}
