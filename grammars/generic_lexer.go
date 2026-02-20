package grammars

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// GenericTokenSource is a best-effort scanner that maps source text to
// tree-sitter token symbols using token-name heuristics.
//
// It is intended as a rollout bridge for grammars without DFA tables.
type GenericTokenSource struct {
	src  []byte
	lang *gotreesitter.Language
	cur  sourceCursor

	done    bool
	pending []gotreesitter.Token

	eofSymbol gotreesitter.Symbol

	identifierSym gotreesitter.Symbol
	primitiveType gotreesitter.Symbol
	intSym        gotreesitter.Symbol
	floatSym      gotreesitter.Symbol
	numberSym     gotreesitter.Symbol
	charSym       gotreesitter.Symbol
	stringSym     gotreesitter.Symbol
	stringContent gotreesitter.Symbol
	escapeSym     gotreesitter.Symbol

	doubleQuoteSym gotreesitter.Symbol
	singleQuoteSym gotreesitter.Symbol
	backtickSym    gotreesitter.Symbol
	tripleQuoteSym gotreesitter.Symbol

	lineCommentSym  gotreesitter.Symbol
	blockCommentSym gotreesitter.Symbol
	commentSym      gotreesitter.Symbol
	shebangSym      gotreesitter.Symbol

	keywordSymbols map[string]gotreesitter.Symbol
	literalSymbols map[string]gotreesitter.Symbol
	maxLiteralLen  int
}

// NewGenericTokenSource creates a best-effort generic token source.
func NewGenericTokenSource(src []byte, lang *gotreesitter.Language) (*GenericTokenSource, error) {
	if lang == nil {
		return nil, fmt.Errorf("generic lexer: language is nil")
	}

	ts := &GenericTokenSource{
		src:            src,
		lang:           lang,
		cur:            newSourceCursor(src),
		keywordSymbols: make(map[string]gotreesitter.Symbol),
		literalSymbols: make(map[string]gotreesitter.Symbol),
	}

	ts.buildSymbolTables()

	if ts.identifierSym == 0 {
		return nil, fmt.Errorf("generic lexer: no identifier-like token symbol found")
	}
	if ts.numberSym == 0 {
		return nil, fmt.Errorf("generic lexer: no number-like token symbol found")
	}

	return ts, nil
}

// NewGenericTokenSourceOrEOF returns a generic token source, or EOF-only
// fallback if setup fails.
func NewGenericTokenSourceOrEOF(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ts, err := NewGenericTokenSource(src, lang)
	if err != nil {
		return tokenSourceInitError{sourceLen: uint32(len(src))}
	}
	return ts
}

func (ts *GenericTokenSource) Next() gotreesitter.Token {
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
		if tok, ok := ts.charToken(); ok {
			return tok
		}

		b := ts.cur.peekByte()
		if isGenericIdentStart(b) {
			if tok, ok := ts.identifierOrKeywordToken(); ok {
				return tok
			}
			continue
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

func (ts *GenericTokenSource) SkipToByte(offset uint32) gotreesitter.Token {
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

func (ts *GenericTokenSource) buildSymbolTables() {
	if ts.eofSymbol, _ = ts.lang.SymbolByName("end"); ts.eofSymbol == 0 {
		ts.eofSymbol = 0
	}

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
			// token id 0 is runtime EOF sentinel.
			continue
		}

		sym := gotreesitter.Symbol(i)
		lname := strings.ToLower(name)

		ts.captureNamedSpecial(name, lname, sym)

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
		if lexeme == "()" || lexeme == "[]" || lexeme == "{}" {
			// Prefer tokenizing bracket pairs as separate delimiters; several
			// grammars define pair aliases, but parser states commonly expect the
			// individual open/close tokens.
			continue
		}

		escapes := tokenNameEscapeCount(name)
		// Prefer lower escape-count representations first. For equal escape
		// counts, prefer later token IDs because several grammars encode
		// context-sensitive punctuation as duplicate visible lexemes where the
		// parser expects the later symbol.
		if prev, exists := literalEscapes[lexeme]; exists && prev < escapes {
			continue
		}
		ts.literalSymbols[lexeme] = sym
		literalEscapes[lexeme] = escapes
		if len(lexeme) > ts.maxLiteralLen {
			ts.maxLiteralLen = len(lexeme)
		}
	}

	ts.numberSym = firstNonZeroSymbol(ts.numberSym, ts.intSym, ts.floatSym)
}

func (ts *GenericTokenSource) captureNamedSpecial(name, lname string, sym gotreesitter.Symbol) {
	switch {
	case ts.identifierSym == 0 && (name == "identifier" || name == "ident" || strings.HasSuffix(name, "_identifier") || strings.HasSuffix(name, "_ident") || name == "bare_key"):
		ts.identifierSym = sym
	case ts.primitiveType == 0 && name == "primitive_type":
		ts.primitiveType = sym
	case ts.intSym == 0 && (strings.Contains(lname, "integer") || strings.HasPrefix(lname, "int_") || strings.HasSuffix(lname, "_int") || name == "number"):
		ts.intSym = sym
	case ts.floatSym == 0 && (strings.Contains(lname, "float") || strings.Contains(lname, "real_")):
		ts.floatSym = sym
	case ts.numberSym == 0 && (name == "number" || strings.Contains(lname, "number_literal") || strings.Contains(lname, "numeric")):
		ts.numberSym = sym
	case ts.charSym == 0 && (strings.Contains(lname, "char_literal") || name == "character"):
		ts.charSym = sym
	case ts.stringSym == 0 && (strings.Contains(lname, "string_literal") || name == "string"):
		ts.stringSym = sym
	case ts.stringContent == 0 && (name == "string_content" || strings.Contains(lname, "string_fragment") || strings.Contains(lname, "string_content")):
		ts.stringContent = sym
	case ts.escapeSym == 0 && strings.Contains(lname, "escape_sequence"):
		ts.escapeSym = sym
	case ts.doubleQuoteSym == 0 && name == "\"":
		ts.doubleQuoteSym = sym
	case ts.singleQuoteSym == 0 && name == "'":
		ts.singleQuoteSym = sym
	case ts.backtickSym == 0 && name == "`":
		ts.backtickSym = sym
	case ts.tripleQuoteSym == 0 && name == "\"\"\"":
		ts.tripleQuoteSym = sym
	case ts.shebangSym == 0 && (name == "hash_bang_line" || name == "shebang"):
		ts.shebangSym = sym
	case ts.lineCommentSym == 0 && (name == "line_comment" || strings.HasPrefix(name, "line_comment_token") || name == "doc_comment" || name == "inner_doc_comment_marker" || name == "outer_doc_comment_marker"):
		ts.lineCommentSym = sym
	case ts.blockCommentSym == 0 && (name == "block_comment" || name == "multiline_comment"):
		ts.blockCommentSym = sym
	case ts.commentSym == 0 && strings.Contains(lname, "comment"):
		ts.commentSym = sym
	}
}

func (ts *GenericTokenSource) shebangToken() (gotreesitter.Token, bool) {
	if ts.shebangSym == 0 || len(ts.src) < 2 || ts.src[0] != '#' || ts.src[1] != '!' {
		return gotreesitter.Token{}, false
	}
	start := ts.cur.offset
	startPt := ts.cur.point()
	for !ts.cur.eof() && ts.cur.peekByte() != '\n' {
		ts.cur.advanceRune()
	}
	return makeToken(ts.shebangSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *GenericTokenSource) commentToken() (gotreesitter.Token, bool) {
	if ts.matchLiteralAtCurrent("//") {
		sym := firstNonZeroSymbol(ts.lineCommentSym, ts.commentSym, ts.literalSymbols["//"])
		return ts.lineCommentWithPrefix(2, sym), true
	}
	if ts.matchLiteralAtCurrent("/*") {
		sym := firstNonZeroSymbol(ts.blockCommentSym, ts.commentSym)
		return ts.blockCommentWithPrefix(sym), true
	}
	if ts.matchLiteralAtCurrent("--") {
		sym := firstNonZeroSymbol(ts.lineCommentSym, ts.commentSym, ts.literalSymbols["--"])
		if sym != ts.literalSymbols["--"] {
			return ts.lineCommentWithPrefix(2, sym), true
		}
	}
	if ts.cur.peekByte() == '#' && ts.commentSym != 0 {
		// Avoid consuming preprocessor directives as comments for C-like languages.
		if ts.cur.offset == 0 || ts.src[ts.cur.offset-1] == '\n' {
			if ts.literalSymbols["#"] != 0 || ts.literalSymbols["#include"] != 0 || ts.literalSymbols["#define"] != 0 {
				return gotreesitter.Token{}, false
			}
		}
		return ts.lineCommentWithPrefix(1, ts.commentSym), true
	}
	return gotreesitter.Token{}, false
}

func (ts *GenericTokenSource) lineCommentWithPrefix(prefixLen int, sym gotreesitter.Symbol) gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(prefixLen)
	for !ts.cur.eof() && ts.cur.peekByte() != '\n' {
		ts.cur.advanceRune()
	}
	if sym == 0 {
		return gotreesitter.Token{Symbol: 0}
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *GenericTokenSource) blockCommentWithPrefix(sym gotreesitter.Symbol) gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(2)
	for !ts.cur.eof() {
		if ts.matchLiteralAtCurrent("*/") {
			ts.advanceBytes(2)
			break
		}
		ts.cur.advanceRune()
	}
	if sym == 0 {
		return gotreesitter.Token{Symbol: 0}
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *GenericTokenSource) stringToken() (gotreesitter.Token, bool) {
	if ts.tripleQuoteSym != 0 && ts.matchLiteralAtCurrent("\"\"\"") {
		return ts.scanQuotedString("\"\"\"", ts.tripleQuoteSym, ts.stringContent, ts.escapeSym)
	}
	if ts.doubleQuoteSym != 0 && ts.cur.peekByte() == '"' {
		if ts.stringSym != 0 && ts.stringContent == 0 {
			return ts.scanWholeString('"', ts.stringSym)
		}
		content := firstNonZeroSymbol(ts.stringContent, ts.stringSym)
		return ts.scanSplitString("\"", ts.doubleQuoteSym, content, ts.escapeSym)
	}
	if ts.backtickSym != 0 && ts.cur.peekByte() == '`' {
		if ts.stringSym != 0 && ts.stringContent == 0 {
			return ts.scanWholeString('`', ts.stringSym)
		}
		content := firstNonZeroSymbol(ts.stringContent, ts.stringSym)
		return ts.scanSplitString("`", ts.backtickSym, content, 0)
	}
	return gotreesitter.Token{}, false
}

func (ts *GenericTokenSource) charToken() (gotreesitter.Token, bool) {
	if ts.cur.peekByte() != '\'' {
		return gotreesitter.Token{}, false
	}
	if ts.charSym != 0 {
		return ts.scanWholeString('\'', ts.charSym)
	}
	if ts.singleQuoteSym == 0 {
		return gotreesitter.Token{}, false
	}
	content := firstNonZeroSymbol(ts.stringContent, ts.stringSym)
	return ts.scanSplitString("'", ts.singleQuoteSym, content, ts.escapeSym)
}

func (ts *GenericTokenSource) scanWholeString(close byte, sym gotreesitter.Symbol) (gotreesitter.Token, bool) {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() {
		ch := ts.cur.peekByte()
		if ch == '\\' {
			ts.cur.advanceByte()
			if !ts.cur.eof() {
				ts.cur.advanceRune()
			}
			continue
		}
		if ch == close {
			ts.cur.advanceByte()
			break
		}
		ts.cur.advanceRune()
	}
	if sym == 0 {
		return gotreesitter.Token{}, false
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *GenericTokenSource) scanQuotedString(quote string, quoteSym, contentSym, escapeSym gotreesitter.Symbol) (gotreesitter.Token, bool) {
	if quoteSym == 0 {
		return gotreesitter.Token{}, false
	}
	return ts.scanSplitString(quote, quoteSym, contentSym, escapeSym)
}

func (ts *GenericTokenSource) scanSplitString(quote string, quoteSym, contentSym, escapeSym gotreesitter.Symbol) (gotreesitter.Token, bool) {
	if !ts.matchLiteralAtCurrent(quote) {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(len(quote))
	openTok := makeToken(quoteSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())

	segStart := ts.cur.offset
	segPt := ts.cur.point()
	for !ts.cur.eof() {
		if ts.matchLiteralAtCurrent(quote) {
			if contentSym != 0 && segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(contentSym, ts.src, segStart, ts.cur.offset, segPt, ts.cur.point()))
			}
			closeStart := ts.cur.offset
			closePt := ts.cur.point()
			ts.advanceBytes(len(quote))
			ts.pending = append(ts.pending, makeToken(quoteSym, ts.src, closeStart, ts.cur.offset, closePt, ts.cur.point()))
			return openTok, true
		}
		if ts.cur.peekByte() == '\\' {
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
	return openTok, true
}

func (ts *GenericTokenSource) identifierOrKeywordToken() (gotreesitter.Token, bool) {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() && isGenericIdentPart(ts.cur.peekByte()) {
		ts.cur.advanceByte()
	}

	text := string(ts.src[start:ts.cur.offset])
	if sym, ok := ts.keywordSymbols[text]; ok {
		return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
	}
	if ts.primitiveType != 0 && isCPrimitiveType(text) {
		return makeToken(ts.primitiveType, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
	}
	if ts.identifierSym == 0 {
		return gotreesitter.Token{}, false
	}
	return makeToken(ts.identifierSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *GenericTokenSource) numberToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()

	isFloat := false
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
			isFloat = true
			ts.cur.advanceByte()
			for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
				ts.cur.advanceByte()
			}
		}
	}

	if !ts.cur.eof() && (ts.cur.peekByte() == 'e' || ts.cur.peekByte() == 'E' || ts.cur.peekByte() == 'p' || ts.cur.peekByte() == 'P') {
		isFloat = true
		ts.cur.advanceByte()
		if !ts.cur.eof() && (ts.cur.peekByte() == '+' || ts.cur.peekByte() == '-') {
			ts.cur.advanceByte()
		}
		for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	}

	sym := ts.numberSym
	if isFloat {
		sym = firstNonZeroSymbol(ts.floatSym, ts.numberSym, ts.intSym)
	} else {
		sym = firstNonZeroSymbol(ts.intSym, ts.numberSym, ts.floatSym)
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *GenericTokenSource) literalToken() (gotreesitter.Token, bool) {
	sym, n := ts.matchLiteral()
	if sym == 0 {
		return gotreesitter.Token{}, false
	}
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(n)
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *GenericTokenSource) matchLiteral() (gotreesitter.Symbol, int) {
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

func (ts *GenericTokenSource) matchLiteralAtCurrent(lexeme string) bool {
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

func (ts *GenericTokenSource) advanceBytes(n int) {
	for i := 0; i < n && !ts.cur.eof(); i++ {
		ts.cur.advanceByte()
	}
}

func (ts *GenericTokenSource) eofToken() gotreesitter.Token {
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

func isGenericIdentStart(b byte) bool {
	return isASCIIAlpha(b) || b == '_' || b == '$'
}

func isGenericIdentPart(b byte) bool {
	return isGenericIdentStart(b) || isASCIIDigit(b) || b == '-'
}
