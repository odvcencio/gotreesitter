package grammars

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"
)

// JavaTokenSource is a lexer bridge for tree-sitter-java.
type JavaTokenSource struct {
	src  []byte
	lang *gotreesitter.Language
	cur  sourceCursor

	done    bool
	pending []gotreesitter.Token

	eofSymbol gotreesitter.Symbol

	identifierSymbol             gotreesitter.Symbol
	decimalIntegerSymbol         gotreesitter.Symbol
	hexIntegerSymbol             gotreesitter.Symbol
	octalIntegerSymbol           gotreesitter.Symbol
	binaryIntegerSymbol          gotreesitter.Symbol
	decimalFloatSymbol           gotreesitter.Symbol
	hexFloatSymbol               gotreesitter.Symbol
	lineCommentSymbol            gotreesitter.Symbol
	blockCommentSymbol           gotreesitter.Symbol
	quoteSymbol                  gotreesitter.Symbol
	textBlockQuoteSymbol         gotreesitter.Symbol
	stringFragmentSymbol         gotreesitter.Symbol
	multilineStringFragmentToken gotreesitter.Symbol
	escapeSymbol                 gotreesitter.Symbol
	charLiteralSymbol            gotreesitter.Symbol
	booleanTypeSymbol            gotreesitter.Symbol
	voidTypeSymbol               gotreesitter.Symbol
	nullLiteralSymbol            gotreesitter.Symbol

	keywordSymbols map[string]gotreesitter.Symbol
	literalSymbols map[string]gotreesitter.Symbol
	maxLiteralLen  int
}

// NewJavaTokenSource creates a token source for Java source text.
func NewJavaTokenSource(src []byte, lang *gotreesitter.Language) (*JavaTokenSource, error) {
	if lang == nil {
		return nil, fmt.Errorf("java lexer: language is nil")
	}

	ts := &JavaTokenSource{
		src:            src,
		lang:           lang,
		cur:            newSourceCursor(src),
		keywordSymbols: make(map[string]gotreesitter.Symbol),
		literalSymbols: make(map[string]gotreesitter.Symbol),
	}

	tl := newTokenLookup(lang, "java")
	ts.identifierSymbol = tl.require("identifier")
	ts.decimalIntegerSymbol = tl.require("decimal_integer_literal")
	ts.hexIntegerSymbol = tl.optional("hex_integer_literal")
	ts.octalIntegerSymbol = tl.optional("octal_integer_literal")
	ts.binaryIntegerSymbol = tl.optional("binary_integer_literal")
	ts.decimalFloatSymbol = tl.optional("decimal_floating_point_literal")
	ts.hexFloatSymbol = tl.optional("hex_floating_point_literal")
	ts.lineCommentSymbol = tl.optional("line_comment")
	ts.blockCommentSymbol = tl.optional("block_comment")
	ts.quoteSymbol = tl.optional("\"")
	ts.textBlockQuoteSymbol = tl.optional("\"\"\"")
	ts.stringFragmentSymbol = tl.optional("string_fragment")
	ts.multilineStringFragmentToken = tl.optional("_multiline_string_fragment_token1", "_multiline_string_fragment_token2")
	ts.escapeSymbol = tl.optional("escape_sequence", "_escape_sequence_token1")
	ts.charLiteralSymbol = tl.optional("character_literal")
	ts.booleanTypeSymbol = tl.optional("boolean_type")
	ts.voidTypeSymbol = tl.optional("void_type")
	ts.nullLiteralSymbol = tl.optional("null_literal")

	if ts.eofSymbol, _ = lang.SymbolByName("end"); ts.eofSymbol == 0 {
		ts.eofSymbol = 0
	}

	ts.buildSymbolTables()

	if err := tl.err(); err != nil {
		return nil, err
	}
	return ts, nil
}

// NewJavaTokenSourceOrEOF returns a Java token source, or EOF-only fallback if
// symbol setup fails.
func NewJavaTokenSourceOrEOF(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		return tokenSourceInitError{sourceLen: uint32(len(src))}
	}
	return ts
}

func (ts *JavaTokenSource) Next() gotreesitter.Token {
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
		if tok, ok := ts.textBlockStringToken(); ok {
			return tok
		}
		if tok, ok := ts.stringToken(); ok {
			return tok
		}
		if tok, ok := ts.charToken(); ok {
			return tok
		}

		b := ts.cur.peekByte()
		if isJavaIdentStart(b) {
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

func (ts *JavaTokenSource) SkipToByte(offset uint32) gotreesitter.Token {
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

func (ts *JavaTokenSource) buildSymbolTables() {
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
		case "identifier", "decimal_integer_literal", "hex_integer_literal", "octal_integer_literal", "binary_integer_literal", "decimal_floating_point_literal", "hex_floating_point_literal", "line_comment", "block_comment", "string_fragment", "escape_sequence", "character_literal", "boolean_type", "void_type", "null_literal":
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

func (ts *JavaTokenSource) commentToken() (gotreesitter.Token, bool) {
	if ts.cur.offset+1 >= len(ts.src) || ts.src[ts.cur.offset] != '/' {
		return gotreesitter.Token{}, false
	}
	next := ts.src[ts.cur.offset+1]
	if next != '/' && next != '*' {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	ts.cur.advanceByte()

	var sym gotreesitter.Symbol
	if next == '/' {
		sym = ts.lineCommentSymbol
		for !ts.cur.eof() && ts.cur.peekByte() != '\n' {
			ts.cur.advanceRune()
		}
	} else {
		sym = ts.blockCommentSymbol
		for !ts.cur.eof() {
			if ts.cur.peekByte() == '*' && ts.cur.offset+1 < len(ts.src) && ts.src[ts.cur.offset+1] == '/' {
				ts.cur.advanceByte()
				ts.cur.advanceByte()
				break
			}
			ts.cur.advanceRune()
		}
	}
	if sym == 0 {
		if ts.blockCommentSymbol != 0 {
			sym = ts.blockCommentSymbol
		} else {
			sym = ts.lineCommentSymbol
		}
	}
	if sym == 0 {
		return gotreesitter.Token{Symbol: 0}, true
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *JavaTokenSource) textBlockStringToken() (gotreesitter.Token, bool) {
	if ts.textBlockQuoteSymbol == 0 || !ts.matchLiteralAtCurrent("\"\"\"") {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(3)
	openTok := makeToken(ts.textBlockQuoteSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point())

	fragmentSym := ts.multilineStringFragmentToken
	if fragmentSym == 0 {
		fragmentSym = ts.stringFragmentSymbol
	}

	segStart := ts.cur.offset
	segStartPt := ts.cur.point()
	for !ts.cur.eof() {
		if ts.matchLiteralAtCurrent("\"\"\"") {
			if fragmentSym != 0 && segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(fragmentSym, ts.src, segStart, ts.cur.offset, segStartPt, ts.cur.point()))
			}
			closeStart := ts.cur.offset
			closePt := ts.cur.point()
			ts.advanceBytes(3)
			ts.pending = append(ts.pending, makeToken(ts.textBlockQuoteSymbol, ts.src, closeStart, ts.cur.offset, closePt, ts.cur.point()))
			return openTok, true
		}
		if ts.cur.peekByte() == '\\' && ts.escapeSymbol != 0 {
			if fragmentSym != 0 && segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(fragmentSym, ts.src, segStart, ts.cur.offset, segStartPt, ts.cur.point()))
			}
			escStart := ts.cur.offset
			escStartPt := ts.cur.point()
			ts.cur.advanceByte()
			if !ts.cur.eof() {
				ts.cur.advanceRune()
			}
			ts.pending = append(ts.pending, makeToken(ts.escapeSymbol, ts.src, escStart, ts.cur.offset, escStartPt, ts.cur.point()))
			segStart = ts.cur.offset
			segStartPt = ts.cur.point()
			continue
		}
		ts.cur.advanceRune()
	}

	if fragmentSym != 0 && segStart < ts.cur.offset {
		ts.pending = append(ts.pending, makeToken(fragmentSym, ts.src, segStart, ts.cur.offset, segStartPt, ts.cur.point()))
	}
	return openTok, true
}

func (ts *JavaTokenSource) stringToken() (gotreesitter.Token, bool) {
	if ts.quoteSymbol == 0 || ts.cur.peekByte() != '"' {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	openTok := makeToken(ts.quoteSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point())

	ts.scanDelimitedBody('"', ts.stringFragmentSymbol, ts.escapeSymbol, ts.quoteSymbol)
	return openTok, true
}

func (ts *JavaTokenSource) charToken() (gotreesitter.Token, bool) {
	if ts.charLiteralSymbol == 0 || ts.cur.peekByte() != '\'' {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() {
		if ts.cur.peekByte() == '\\' {
			ts.cur.advanceByte()
			if !ts.cur.eof() {
				ts.cur.advanceRune()
			}
			continue
		}
		if ts.cur.peekByte() == '\'' {
			ts.cur.advanceByte()
			break
		}
		ts.cur.advanceRune()
	}
	return makeToken(ts.charLiteralSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *JavaTokenSource) scanDelimitedBody(close byte, fragmentSym, escapeSym, closeSym gotreesitter.Symbol) {
	segStart := ts.cur.offset
	segStartPt := ts.cur.point()

	for !ts.cur.eof() {
		ch := ts.cur.peekByte()
		if ch == close {
			if fragmentSym != 0 && segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(fragmentSym, ts.src, segStart, ts.cur.offset, segStartPt, ts.cur.point()))
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
			if fragmentSym != 0 && segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(fragmentSym, ts.src, segStart, ts.cur.offset, segStartPt, ts.cur.point()))
			}
			escStart := ts.cur.offset
			escStartPt := ts.cur.point()
			ts.cur.advanceByte()
			if !ts.cur.eof() {
				if ts.cur.peekByte() == 'u' {
					ts.cur.advanceByte()
					for !ts.cur.eof() && ts.cur.peekByte() == 'u' {
						ts.cur.advanceByte()
					}
					for i := 0; i < 4 && !ts.cur.eof() && isASCIIHex(ts.cur.peekByte()); i++ {
						ts.cur.advanceByte()
					}
				} else {
					ts.cur.advanceRune()
				}
			}
			if escapeSym != 0 {
				ts.pending = append(ts.pending, makeToken(escapeSym, ts.src, escStart, ts.cur.offset, escStartPt, ts.cur.point()))
			}
			segStart = ts.cur.offset
			segStartPt = ts.cur.point()
			continue
		}
		ts.cur.advanceRune()
	}

	if fragmentSym != 0 && segStart < ts.cur.offset {
		ts.pending = append(ts.pending, makeToken(fragmentSym, ts.src, segStart, ts.cur.offset, segStartPt, ts.cur.point()))
	}
}

func (ts *JavaTokenSource) identifierOrKeywordToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() && isJavaIdentPart(ts.cur.peekByte()) {
		ts.cur.advanceByte()
	}

	text := string(ts.src[start:ts.cur.offset])
	sym := ts.identifierSymbol

	switch text {
	case "boolean":
		if ts.booleanTypeSymbol != 0 {
			sym = ts.booleanTypeSymbol
		}
	case "void":
		if ts.voidTypeSymbol != 0 {
			sym = ts.voidTypeSymbol
		}
	case "null":
		if ts.nullLiteralSymbol != 0 {
			sym = ts.nullLiteralSymbol
		}
	default:
		if kw, ok := ts.keywordSymbols[text]; ok {
			sym = kw
		}
	}

	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *JavaTokenSource) numberToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()

	isHex := false
	isBinary := false
	isOctal := false
	isFloat := false
	isHexFloat := false
	sawEightOrNine := false

	if ts.cur.peekByte() == '0' && ts.cur.offset+1 < len(ts.src) {
		next := ts.src[ts.cur.offset+1]
		switch next {
		case 'x', 'X':
			isHex = true
			ts.cur.advanceByte()
			ts.cur.advanceByte()
			for !ts.cur.eof() && (isASCIIHex(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
				ts.cur.advanceByte()
			}
			if !ts.cur.eof() && ts.cur.peekByte() == '.' {
				isFloat = true
				isHexFloat = true
				ts.cur.advanceByte()
				for !ts.cur.eof() && (isASCIIHex(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
					ts.cur.advanceByte()
				}
			}
			if !ts.cur.eof() && (ts.cur.peekByte() == 'p' || ts.cur.peekByte() == 'P') {
				isFloat = true
				isHexFloat = true
				ts.cur.advanceByte()
				if !ts.cur.eof() && (ts.cur.peekByte() == '+' || ts.cur.peekByte() == '-') {
					ts.cur.advanceByte()
				}
				for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
					ts.cur.advanceByte()
				}
			}
		case 'b', 'B':
			isBinary = true
			ts.cur.advanceByte()
			ts.cur.advanceByte()
			for !ts.cur.eof() && (ts.cur.peekByte() == '0' || ts.cur.peekByte() == '1' || ts.cur.peekByte() == '_') {
				ts.cur.advanceByte()
			}
		default:
			isOctal = true
			for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
				if ts.cur.peekByte() == '8' || ts.cur.peekByte() == '9' {
					sawEightOrNine = true
				}
				ts.cur.advanceByte()
			}
		}
	} else {
		for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	}

	if !isHex && !isBinary {
		if !ts.cur.eof() && ts.cur.peekByte() == '.' {
			// Avoid consuming ".." / "..." as a number suffix.
			if ts.cur.offset+1 >= len(ts.src) || ts.src[ts.cur.offset+1] != '.' {
				isFloat = true
				isOctal = false
				ts.cur.advanceByte()
				for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
					ts.cur.advanceByte()
				}
			}
		}
		if !ts.cur.eof() && (ts.cur.peekByte() == 'e' || ts.cur.peekByte() == 'E') {
			isFloat = true
			isOctal = false
			ts.cur.advanceByte()
			if !ts.cur.eof() && (ts.cur.peekByte() == '+' || ts.cur.peekByte() == '-') {
				ts.cur.advanceByte()
			}
			for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
				ts.cur.advanceByte()
			}
		}
	}

	if !ts.cur.eof() {
		suffix := ts.cur.peekByte()
		switch suffix {
		case 'f', 'F', 'd', 'D':
			isFloat = true
			ts.cur.advanceByte()
		case 'l', 'L':
			ts.cur.advanceByte()
		}
	}

	if isOctal && sawEightOrNine {
		isOctal = false
	}

	sym := ts.decimalIntegerSymbol
	switch {
	case isFloat && isHexFloat:
		sym = firstNonZeroSymbol(ts.hexFloatSymbol, ts.decimalFloatSymbol, ts.decimalIntegerSymbol)
	case isFloat:
		sym = firstNonZeroSymbol(ts.decimalFloatSymbol, ts.decimalIntegerSymbol)
	case isHex:
		sym = firstNonZeroSymbol(ts.hexIntegerSymbol, ts.decimalIntegerSymbol)
	case isBinary:
		sym = firstNonZeroSymbol(ts.binaryIntegerSymbol, ts.decimalIntegerSymbol)
	case isOctal && ts.cur.offset-start > 1:
		sym = firstNonZeroSymbol(ts.octalIntegerSymbol, ts.decimalIntegerSymbol)
	}

	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *JavaTokenSource) literalToken() (gotreesitter.Token, bool) {
	sym, n := ts.matchLiteral()
	if sym == 0 {
		return gotreesitter.Token{}, false
	}
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.advanceBytes(n)
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *JavaTokenSource) matchLiteral() (gotreesitter.Symbol, int) {
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

func (ts *JavaTokenSource) matchLiteralAtCurrent(lexeme string) bool {
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

func (ts *JavaTokenSource) advanceBytes(n int) {
	for i := 0; i < n && !ts.cur.eof(); i++ {
		ts.cur.advanceByte()
	}
}

func (ts *JavaTokenSource) eofToken() gotreesitter.Token {
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

func firstNonZeroSymbol(symbols ...gotreesitter.Symbol) gotreesitter.Symbol {
	for _, sym := range symbols {
		if sym != 0 {
			return sym
		}
	}
	return 0
}

func isJavaIdentStart(b byte) bool {
	return isASCIIAlpha(b) || b == '_' || b == '$'
}

func isJavaIdentPart(b byte) bool {
	return isJavaIdentStart(b) || isASCIIDigit(b)
}
