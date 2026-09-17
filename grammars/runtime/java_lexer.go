//go:build !grammar_subset || grammar_subset_java

package grammarruntime

import (
	"fmt"
	"sync"

	"github.com/odvcencio/gotreesitter"
)

// JavaTokenSource is a lexer bridge for tree-sitter-java.
type JavaTokenSource struct {
	src  []byte
	lang *gotreesitter.Language
	cur  sourceCursor

	done    bool
	pending []gotreesitter.Token

	state     gotreesitter.StateID
	glrStates []gotreesitter.StateID

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
	underscorePatternSymbol      gotreesitter.Symbol

	keywordSymbols map[string]gotreesitter.Symbol
	literalSymbols map[string]gotreesitter.Symbol
	maxLiteralLen  int
}

type javaLexerTables struct {
	keywordSymbols map[string]gotreesitter.Symbol
	literalSymbols map[string]gotreesitter.Symbol
	maxLiteralLen  int
}

var javaLexerTablesCache sync.Map // map[*gotreesitter.Language]*javaLexerTables

// NewJavaTokenSource creates a token source for Java source text.
func NewJavaTokenSource(src []byte, lang *gotreesitter.Language) (*JavaTokenSource, error) {
	if lang == nil {
		return nil, fmt.Errorf("java lexer: language is nil")
	}

	ts := &JavaTokenSource{
		src:  src,
		lang: lang,
		cur:  newSourceCursor(src),
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
	ts.underscorePatternSymbol = tl.optional("underscore_pattern")

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

// Reset reinitializes this token source for a new source buffer.
func (ts *JavaTokenSource) Reset(src []byte) {
	ts.src = src
	ts.cur = newSourceCursor(src)
	ts.done = false
	ts.pending = ts.pending[:0]
	ts.state = 0
	ts.glrStates = ts.glrStates[:0]
}

// SupportsIncrementalReuse reports that JavaTokenSource preserves stable token
// boundaries across edits and supports deterministic SkipToByte behavior.
func (ts *JavaTokenSource) SupportsIncrementalReuse() bool {
	return true
}

// SetParserState lets the parser drive contextual keyword tokenization.
func (ts *JavaTokenSource) SetParserState(state gotreesitter.StateID) {
	ts.state = state
}

// SetGLRStates lets the token source consider all active GLR branches when
// deciding whether a contextual keyword is valid.
func (ts *JavaTokenSource) SetGLRStates(states []gotreesitter.StateID) {
	if len(states) == 0 {
		ts.glrStates = ts.glrStates[:0]
		return
	}
	if cap(ts.glrStates) < len(states) {
		ts.glrStates = make([]gotreesitter.StateID, len(states))
	} else {
		ts.glrStates = ts.glrStates[:len(states)]
	}
	copy(ts.glrStates, states)
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
	if cached, ok := javaLexerTablesCache.Load(ts.lang); ok {
		ts.applyLexerTables(cached.(*javaLexerTables))
		return
	}

	keywordSymbols := make(map[string]gotreesitter.Symbol)
	literalSymbols := make(map[string]gotreesitter.Symbol)
	maxLiteralLen := 0

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
		case "identifier", "decimal_integer_literal", "hex_integer_literal", "octal_integer_literal", "binary_integer_literal", "decimal_floating_point_literal", "hex_floating_point_literal", "line_comment", "block_comment", "string_fragment", "escape_sequence", "character_literal", "boolean_type", "void_type", "null_literal", "underscore_pattern":
			continue
		}
		if isSyntheticTokenName(name) {
			continue
		}

		if isTokenNameWord(name) {
			if _, exists := keywordSymbols[name]; !exists {
				keywordSymbols[name] = sym
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
		literalSymbols[lexeme] = sym
		literalEscapes[lexeme] = escapes
		if len(lexeme) > maxLiteralLen {
			maxLiteralLen = len(lexeme)
		}
	}

	tables := &javaLexerTables{
		keywordSymbols: keywordSymbols,
		literalSymbols: literalSymbols,
		maxLiteralLen:  maxLiteralLen,
	}
	if actual, loaded := javaLexerTablesCache.LoadOrStore(ts.lang, tables); loaded {
		ts.applyLexerTables(actual.(*javaLexerTables))
		return
	}
	ts.applyLexerTables(tables)
}

func (ts *JavaTokenSource) applyLexerTables(tables *javaLexerTables) {
	if tables == nil {
		return
	}
	ts.keywordSymbols = tables.keywordSymbols
	ts.literalSymbols = tables.literalSymbols
	ts.maxLiteralLen = tables.maxLiteralLen
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
	if ts.textBlockQuoteSymbol == 0 || !ts.cur.matchLiteralAtCurrent("\"\"\"") {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceBytes(3)
	openTok := makeToken(ts.textBlockQuoteSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point())

	fragmentSym := ts.multilineStringFragmentToken
	if fragmentSym == 0 {
		fragmentSym = ts.stringFragmentSymbol
	}

	segStart := ts.cur.offset
	segStartPt := ts.cur.point()
	for !ts.cur.eof() {
		if ts.cur.matchLiteralAtCurrent("\"\"\"") {
			if fragmentSym != 0 && segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(fragmentSym, ts.src, segStart, ts.cur.offset, segStartPt, ts.cur.point()))
			}
			closeStart := ts.cur.offset
			closePt := ts.cur.point()
			ts.cur.advanceBytes(3)
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
	if text == "_" && ts.underscorePatternSymbol != 0 && ts.hasActionForSymbol(ts.underscorePatternSymbol) {
		sym = ts.underscorePatternSymbol
	}
	if text == "non" {
		if tok, ok := ts.compoundContextualKeywordToken(start, startPt); ok {
			return tok
		}
	}

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
			if ts.shouldPromoteKeyword(text, kw, ts.identifierSymbol) {
				sym = kw
			}
		}
	}

	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *JavaTokenSource) compoundContextualKeywordToken(start int, startPt gotreesitter.Point) (gotreesitter.Token, bool) {
	const nonSealed = "non-sealed"
	if start+len(nonSealed) > len(ts.src) || string(ts.src[start:start+len(nonSealed)]) != nonSealed {
		return gotreesitter.Token{}, false
	}
	if !hasWordBoundaryAfter(ts.src, start+len(nonSealed)) {
		return gotreesitter.Token{}, false
	}
	kw, ok := ts.keywordSymbols[nonSealed]
	if !ok || !ts.shouldPromoteKeyword(nonSealed, kw, ts.identifierSymbol) {
		return gotreesitter.Token{}, false
	}
	ts.cur.advanceBytes(len(nonSealed) - len("non"))
	return makeToken(kw, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *JavaTokenSource) shouldPromoteKeyword(text string, keywordSym, identifierSym gotreesitter.Symbol) bool {
	if keywordSym == 0 {
		return false
	}
	kwHasAction := ts.hasActionForSymbol(keywordSym)
	idHasAction := ts.hasActionForSymbol(identifierSym)
	if text == "permits" {
		if ts.looksLikePermitsClause() {
			return kwHasAction
		}
		return kwHasAction && !idHasAction
	}
	if !kwHasAction && idHasAction {
		return false
	}
	return true
}

func (ts *JavaTokenSource) looksLikePermitsClause() bool {
	end := ts.cur.offset - len("permits")
	if end < 0 {
		return false
	}
	start := end
	for start > 0 {
		switch ts.src[start-1] {
		case '{', '}', ';':
			goto found
		default:
			start--
		}
	}
found:
	seenSealed := false
	seenDecl := false
	for i := start; i < end; {
		b := ts.src[i]
		if !isJavaIdentStart(b) {
			i++
			continue
		}
		wordStart := i
		i++
		for i < end && isJavaIdentPart(ts.src[i]) {
			i++
		}
		word := string(ts.src[wordStart:i])
		switch word {
		case "sealed":
			seenSealed = true
		case "class", "interface":
			seenDecl = true
		}
	}
	return seenSealed && seenDecl
}

func (ts *JavaTokenSource) hasActionForSymbol(sym gotreesitter.Symbol) bool {
	if sym == 0 {
		return false
	}
	if ts.lookupActionIndex(ts.state, sym) != 0 {
		return true
	}
	for _, state := range ts.glrStates {
		if ts.lookupActionIndex(state, sym) != 0 {
			return true
		}
	}
	return false
}

func (ts *JavaTokenSource) lookupActionIndex(state gotreesitter.StateID, sym gotreesitter.Symbol) uint16 {
	if ts == nil || ts.lang == nil {
		return 0
	}
	denseLimit := len(ts.lang.ParseTable)
	smallBase := 0
	if ts.lang.LargeStateCount > 0 {
		denseLimit = int(ts.lang.LargeStateCount)
		smallBase = int(ts.lang.LargeStateCount)
	}
	if int(state) < denseLimit {
		if int(state) >= len(ts.lang.ParseTable) {
			return 0
		}
		row := ts.lang.ParseTable[state]
		if int(sym) >= len(row) {
			return 0
		}
		return row[sym]
	}

	smallIdx := int(state) - smallBase
	if smallIdx < 0 || smallIdx >= len(ts.lang.SmallParseTableMap) {
		return 0
	}
	offset := ts.lang.SmallParseTableMap[smallIdx]
	table := ts.lang.SmallParseTable
	if int(offset) >= len(table) {
		return 0
	}
	groupCount := table[offset]
	pos := int(offset) + 1
	for i := uint16(0); i < groupCount; i++ {
		if pos+1 >= len(table) {
			return 0
		}
		sectionValue := table[pos]
		symbolCount := table[pos+1]
		pos += 2
		for j := uint16(0); j < symbolCount; j++ {
			if pos >= len(table) {
				return 0
			}
			if table[pos] == uint16(sym) {
				return sectionValue
			}
			pos++
		}
	}
	return 0
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

	digitsEnd := ts.cur.offset
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
	case isOctal && digitsEnd-start > 1:
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
	if n >= 2 && start+n <= len(ts.src) && isJavaCompactCloseAngleLiteral(ts.src[start:start+n]) {
		if tok, ok := ts.compactCloseAngleToken(start, startPt, sym); ok {
			return tok, true
		}
	}
	ts.cur.advanceBytes(n)
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func isJavaCompactCloseAngleLiteral(lit []byte) bool {
	if len(lit) < 2 {
		return false
	}
	for _, b := range lit {
		if b != '>' {
			return false
		}
	}
	return true
}

func (ts *JavaTokenSource) compactCloseAngleToken(start int, startPt gotreesitter.Point, compactSym gotreesitter.Symbol) (gotreesitter.Token, bool) {
	gtSym, ok := ts.literalSymbols[">"]
	if !ok || gtSym == 0 {
		return gotreesitter.Token{}, false
	}
	if !ts.shouldSplitCompactCloseAngleToken(start, gtSym, compactSym) {
		return gotreesitter.Token{}, false
	}
	ts.cur.advanceBytes(1)
	return makeToken(gtSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *JavaTokenSource) shouldSplitCompactCloseAngleToken(start int, gtSym, shiftSym gotreesitter.Symbol) bool {
	if !ts.hasUnclosedAngleBefore(start) {
		return false
	}
	if !ts.hasActionForSymbol(gtSym) {
		return false
	}
	if !ts.hasActionForSymbol(shiftSym) {
		return true
	}
	gtSpec := ts.activeActionSpecificity(gtSym)
	shiftSpec := ts.activeActionSpecificity(shiftSym)
	if gtSpec > shiftSpec {
		return true
	}
	if gtSpec < shiftSpec {
		return false
	}
	next := ts.nextNonSpaceByte(start + 2)
	switch next {
	case 0, '(', ')', '[', ']', '{', '}', ',', '.', ';', ':', '?':
		return true
	default:
		return isJavaIdentStart(next) && ts.hasUnclosedAngleBefore(start)
	}
}

func (ts *JavaTokenSource) activeActionSpecificity(sym gotreesitter.Symbol) int {
	type actionStats struct {
		maxDyn     int
		totalDyn   int
		maxActions int
		totalActs  int
		supporting int
	}
	stats := actionStats{}
	visit := func(state gotreesitter.StateID) {
		idx := ts.lookupActionIndex(state, sym)
		if idx == 0 || ts.lang == nil || int(idx) >= len(ts.lang.ParseActions) {
			return
		}
		actions := ts.lang.ParseActions[idx].Actions
		if len(actions) == 0 {
			return
		}
		stats.supporting++
		if len(actions) > stats.maxActions {
			stats.maxActions = len(actions)
		}
		stats.totalActs += len(actions)
		for _, act := range actions {
			dyn := int(act.DynamicPrecedence)
			if dyn > stats.maxDyn {
				stats.maxDyn = dyn
			}
			stats.totalDyn += dyn
		}
	}
	visit(ts.state)
	for i, state := range ts.glrStates {
		if state == ts.state || ts.priorGLRState(i, state) {
			continue
		}
		visit(state)
	}
	return (((stats.maxDyn*1024)+stats.totalDyn)*1024 + stats.maxActions*64 + stats.totalActs*4 + stats.supporting)
}

func (ts *JavaTokenSource) priorGLRState(limit int, state gotreesitter.StateID) bool {
	for i := 0; i < limit && i < len(ts.glrStates); i++ {
		if ts.glrStates[i] == state {
			return true
		}
	}
	return false
}

func (ts *JavaTokenSource) nextNonSpaceByte(pos int) byte {
	for pos < len(ts.src) {
		switch ts.src[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
			continue
		default:
			return ts.src[pos]
		}
	}
	return 0
}

func (ts *JavaTokenSource) hasUnclosedAngleBefore(pos int) bool {
	depth := 0
	for i := pos - 1; i >= 0; i-- {
		switch ts.src[i] {
		case ';', '{', '}', '(', ')':
			return depth > 0
		case '>':
			depth--
		case '<':
			depth++
			if depth > 0 {
				return true
			}
		}
	}
	return depth > 0
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

func isJavaIdentStart(b byte) bool {
	return isASCIIAlpha(b) || b == '_' || b == '$'
}

func isJavaIdentPart(b byte) bool {
	return isJavaIdentStart(b) || isASCIIDigit(b)
}
