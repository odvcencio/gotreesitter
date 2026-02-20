package grammars

import (
	"fmt"

	"github.com/odvcencio/mane/gotreesitter"
)

// LuaTokenSource is a lexer bridge for tree-sitter-lua.
type LuaTokenSource struct {
	src  []byte
	lang *gotreesitter.Language
	cur  sourceCursor

	done    bool
	pending []gotreesitter.Token

	eofSymbol        gotreesitter.Symbol
	identifierSymbol gotreesitter.Symbol
	numberSymbol     gotreesitter.Symbol
	escapeSymbol     gotreesitter.Symbol
	doubleQuoteSym   gotreesitter.Symbol
	singleQuoteSym   gotreesitter.Symbol
	dqContentSym     gotreesitter.Symbol
	sqContentSym     gotreesitter.Symbol
	stringContentSym gotreesitter.Symbol
	shebangSymbol    gotreesitter.Symbol
	commentDashSym   gotreesitter.Symbol
	commentContent   gotreesitter.Symbol
	breakSymbol      gotreesitter.Symbol
	varargSymbol     gotreesitter.Symbol

	keywordSymbols map[string]gotreesitter.Symbol
	literalSymbols map[string]gotreesitter.Symbol
	maxLiteralLen  int
}

// NewLuaTokenSource creates a token source for Lua source text.
func NewLuaTokenSource(src []byte, lang *gotreesitter.Language) (*LuaTokenSource, error) {
	if lang == nil {
		return nil, fmt.Errorf("lua lexer: language is nil")
	}

	ts := &LuaTokenSource{
		src:            src,
		lang:           lang,
		cur:            newSourceCursor(src),
		keywordSymbols: make(map[string]gotreesitter.Symbol),
		literalSymbols: make(map[string]gotreesitter.Symbol),
	}

	tl := newTokenLookup(lang, "lua")
	ts.identifierSymbol = tl.require("identifier")
	ts.numberSymbol = tl.require("number")
	ts.escapeSymbol = tl.optional("escape_sequence")
	ts.doubleQuoteSym = tl.optional("\"")
	ts.singleQuoteSym = tl.optional("'")
	ts.dqContentSym = tl.optional("_doublequote_string_content_token1", "string_content")
	ts.sqContentSym = tl.optional("_singlequote_string_content_token1", "string_content")
	ts.stringContentSym = tl.optional("string_content")
	ts.shebangSymbol = tl.optional("hash_bang_line")
	ts.commentDashSym = tl.optional("--")
	ts.commentContent = tl.optional("comment_content")
	ts.breakSymbol = tl.optional("break_statement")
	ts.varargSymbol = tl.optional("vararg_expression")

	if ts.eofSymbol, _ = lang.SymbolByName("end"); ts.eofSymbol == 0 {
		ts.eofSymbol = 0
	}

	ts.buildSymbolTables()

	if err := tl.err(); err != nil {
		return nil, err
	}
	return ts, nil
}

// NewLuaTokenSourceOrEOF returns a Lua token source, or EOF-only fallback if
// symbol setup fails.
func NewLuaTokenSourceOrEOF(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ts, err := NewLuaTokenSource(src, lang)
	if err != nil {
		return tokenSourceInitError{sourceLen: uint32(len(src))}
	}
	return ts
}

func (ts *LuaTokenSource) Next() gotreesitter.Token {
	if len(ts.pending) > 0 {
		tok := ts.pending[0]
		ts.pending = ts.pending[1:]
		return tok
	}
	if ts.done {
		return ts.eofToken()
	}

	for {
		if ts.cur.offset == 0 {
			if tok, ok := ts.shebangToken(); ok {
				return tok
			}
		}

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
		if tok, ok := ts.longStringToken(); ok {
			return tok
		}

		if ts.varargSymbol != 0 && ts.matchLiteralAtCurrent("...") {
			start := ts.cur.offset
			startPt := ts.cur.point()
			ts.advanceBytes(3)
			return makeToken(ts.varargSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
		}

		b := ts.cur.peekByte()
		if isLuaIdentStart(b) {
			return ts.identifierOrKeywordToken()
		}
		if isASCIIDigit(b) {
			return ts.numberToken()
		}
		if tok, ok := ts.literalToken(); ok {
			return tok
		}

		ts.cur.advanceRune()
	}
}

func (ts *LuaTokenSource) SkipToByte(offset uint32) gotreesitter.Token {
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

func (ts *LuaTokenSource) buildSymbolTables() {
	limit := int(ts.lang.TokenCount)
	if limit > len(ts.lang.SymbolNames) {
		limit = len(ts.lang.SymbolNames)
	}
	literalEscapes := make(map[string]int)

	for i := 0; i < limit; i++ {
		name := ts.lang.SymbolNames[i]
		if name == "" {
			continue
		}
		if i == 0 {
			// token id 0 is EOF sentinel, even if display name collides with "end".
			continue
		}
		sym := gotreesitter.Symbol(i)

		switch name {
		case "identifier", "number", "hash_bang_line", "escape_sequence", "comment_content", "string_content", "break_statement", "vararg_expression":
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

func (ts *LuaTokenSource) shebangToken() (gotreesitter.Token, bool) {
	if ts.shebangSymbol == 0 || len(ts.src) < 2 || ts.src[0] != '#' || ts.src[1] != '!' {
		return gotreesitter.Token{}, false
	}
	start := ts.cur.offset
	startPt := ts.cur.point()
	for !ts.cur.eof() && ts.cur.peekByte() != '\n' {
		ts.cur.advanceRune()
	}
	return makeToken(ts.shebangSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *LuaTokenSource) commentToken() (gotreesitter.Token, bool) {
	if !ts.matchLiteralAtCurrent("--") {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(2)
	commentDash := makeToken(firstNonZeroSymbol(ts.commentDashSym, ts.literalSymbols["--"]), ts.src, start, ts.cur.offset, startPt, ts.cur.point())

	contentStart := ts.cur.offset
	contentPt := ts.cur.point()
	for !ts.cur.eof() && ts.cur.peekByte() != '\n' {
		ts.cur.advanceRune()
	}
	if ts.commentContent != 0 && contentStart < ts.cur.offset {
		ts.pending = append(ts.pending, makeToken(ts.commentContent, ts.src, contentStart, ts.cur.offset, contentPt, ts.cur.point()))
	}
	if commentDash.Symbol == 0 {
		if len(ts.pending) > 0 {
			tok := ts.pending[0]
			ts.pending = ts.pending[1:]
			return tok, true
		}
		return gotreesitter.Token{Symbol: 0}, true
	}
	return commentDash, true
}

func (ts *LuaTokenSource) stringToken() (gotreesitter.Token, bool) {
	ch := ts.cur.peekByte()
	if ch != '"' && ch != '\'' {
		return gotreesitter.Token{}, false
	}

	openSym := ts.doubleQuoteSym
	contentSym := ts.dqContentSym
	if ch == '\'' {
		openSym = ts.singleQuoteSym
		contentSym = ts.sqContentSym
	}
	if contentSym == 0 {
		contentSym = ts.stringContentSym
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	openTok := makeToken(openSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
	ts.scanDelimitedBody(ch, contentSym, ts.escapeSymbol, openSym)
	return openTok, true
}

func (ts *LuaTokenSource) longStringToken() (gotreesitter.Token, bool) {
	if !ts.matchLiteralAtCurrent("[[") {
		return gotreesitter.Token{}, false
	}

	open := ts.literalSymbols["[["]
	close := ts.literalSymbols["]]"]
	if open == 0 {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(2)
	openTok := makeToken(open, ts.src, start, ts.cur.offset, startPt, ts.cur.point())

	contentStart := ts.cur.offset
	contentPt := ts.cur.point()
	for !ts.cur.eof() {
		if ts.matchLiteralAtCurrent("]]") {
			if ts.stringContentSym != 0 && contentStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(ts.stringContentSym, ts.src, contentStart, ts.cur.offset, contentPt, ts.cur.point()))
			}
			closeStart := ts.cur.offset
			closePt := ts.cur.point()
			ts.advanceBytes(2)
			if close != 0 {
				ts.pending = append(ts.pending, makeToken(close, ts.src, closeStart, ts.cur.offset, closePt, ts.cur.point()))
			}
			return openTok, true
		}
		ts.cur.advanceRune()
	}

	if ts.stringContentSym != 0 && contentStart < ts.cur.offset {
		ts.pending = append(ts.pending, makeToken(ts.stringContentSym, ts.src, contentStart, ts.cur.offset, contentPt, ts.cur.point()))
	}
	return openTok, true
}

func (ts *LuaTokenSource) scanDelimitedBody(close byte, contentSym, escapeSym, closeSym gotreesitter.Symbol) {
	segStart := ts.cur.offset
	segPt := ts.cur.point()

	for !ts.cur.eof() {
		ch := ts.cur.peekByte()
		if ch == close {
			if contentSym != 0 && segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(contentSym, ts.src, segStart, ts.cur.offset, segPt, ts.cur.point()))
			}
			closeStart := ts.cur.offset
			closePt := ts.cur.point()
			ts.cur.advanceByte()
			if closeSym != 0 {
				ts.pending = append(ts.pending, makeToken(closeSym, ts.src, closeStart, ts.cur.offset, closePt, ts.cur.point()))
			}
			return
		}
		if ch == '\\' {
			if contentSym != 0 && segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(contentSym, ts.src, segStart, ts.cur.offset, segPt, ts.cur.point()))
			}
			escStart := ts.cur.offset
			escPt := ts.cur.point()
			ts.cur.advanceByte()
			if !ts.cur.eof() {
				ts.cur.advanceRune()
			}
			if escapeSym != 0 {
				ts.pending = append(ts.pending, makeToken(escapeSym, ts.src, escStart, ts.cur.offset, escPt, ts.cur.point()))
			}
			segStart = ts.cur.offset
			segPt = ts.cur.point()
			continue
		}
		ts.cur.advanceRune()
	}

	if contentSym != 0 && segStart < ts.cur.offset {
		ts.pending = append(ts.pending, makeToken(contentSym, ts.src, segStart, ts.cur.offset, segPt, ts.cur.point()))
	}
}

func (ts *LuaTokenSource) identifierOrKeywordToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() && isLuaIdentPart(ts.cur.peekByte()) {
		ts.cur.advanceByte()
	}

	text := string(ts.src[start:ts.cur.offset])
	sym := ts.identifierSymbol
	if text == "break" && ts.breakSymbol != 0 {
		sym = ts.breakSymbol
	} else if kw, ok := ts.keywordSymbols[text]; ok {
		sym = kw
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *LuaTokenSource) numberToken() gotreesitter.Token {
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
		if ts.cur.offset+1 >= len(ts.src) || ts.src[ts.cur.offset+1] != '.' {
			ts.cur.advanceByte()
			for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
				ts.cur.advanceByte()
			}
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

	return makeToken(ts.numberSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *LuaTokenSource) literalToken() (gotreesitter.Token, bool) {
	sym, n := ts.matchLiteral()
	if sym == 0 {
		return gotreesitter.Token{}, false
	}
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(n)
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *LuaTokenSource) matchLiteral() (gotreesitter.Symbol, int) {
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

func (ts *LuaTokenSource) matchLiteralAtCurrent(lexeme string) bool {
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

func (ts *LuaTokenSource) advanceBytes(n int) {
	for i := 0; i < n && !ts.cur.eof(); i++ {
		ts.cur.advanceByte()
	}
}

func (ts *LuaTokenSource) eofToken() gotreesitter.Token {
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

func isLuaIdentStart(b byte) bool {
	return isASCIIAlpha(b) || b == '_'
}

func isLuaIdentPart(b byte) bool {
	return isLuaIdentStart(b) || isASCIIDigit(b)
}
