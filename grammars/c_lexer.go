//go:build !grammar_subset || grammar_subset_c || grammar_subset_cpp

package grammars

import (
	"fmt"
	"sync"

	"github.com/odvcencio/gotreesitter"
)

// CTokenSource is a lightweight lexer bridge for tree-sitter-c.
type CTokenSource struct {
	src  []byte
	lang *gotreesitter.Language
	cur  sourceCursor

	done    bool
	pending []gotreesitter.Token

	eofSymbol              gotreesitter.Symbol
	identifierSymbol       gotreesitter.Symbol
	numberSymbol           gotreesitter.Symbol
	commentSymbol          gotreesitter.Symbol
	quoteSymbol            gotreesitter.Symbol
	apostropheSymbol       gotreesitter.Symbol
	stringContentSymbol    gotreesitter.Symbol
	systemLibStringSymbol  gotreesitter.Symbol
	escapeSymbol           gotreesitter.Symbol
	characterSymbol        gotreesitter.Symbol
	primitiveTypeSymbol    gotreesitter.Symbol
	preprocParamLParen     gotreesitter.Symbol
	preprocEndSymbol       gotreesitter.Symbol // preproc_include_token2: line terminator for preprocessor directives
	preprocArgSymbol       gotreesitter.Symbol
	preprocDirectiveSymbol gotreesitter.Symbol
	newlineSymbol          gotreesitter.Symbol
	endifSymbol            gotreesitter.Symbol
	rBraceSymbol           gotreesitter.Symbol

	// Preprocessor state tracking
	preprocState            int
	parserState             gotreesitter.StateID
	glrStates               []gotreesitter.StateID
	lastSyntheticOffset     int
	preprocDefineNameEnd    int
	preprocOpaqueArgPending bool
	preprocOpaqueArgActive  bool

	keywordSymbols    map[string]gotreesitter.Symbol
	literalSymbols    map[string]gotreesitter.Symbol
	literalAlternates map[string][]gotreesitter.Symbol
	literalByFirst    [256][]literalMatchCandidate

	stringOpeners []prefixedToken
	charOpeners   []prefixedToken
}

type cLexerTables struct {
	keywordSymbols    map[string]gotreesitter.Symbol
	literalSymbols    map[string]gotreesitter.Symbol
	literalAlternates map[string][]gotreesitter.Symbol
	literalByFirst    [256][]literalMatchCandidate
	stringOpeners     []prefixedToken
	charOpeners       []prefixedToken
}

type literalCandidate struct {
	sym     gotreesitter.Symbol
	escapes int
}

type literalMatchCandidate struct {
	lexeme     string
	sym        gotreesitter.Symbol
	alternates []gotreesitter.Symbol
}

var cLexerTablesCache sync.Map // map[*gotreesitter.Language]*cLexerTables

var cTokenSourcePool = sync.Pool{
	New: func() any {
		return &CTokenSource{
			lastSyntheticOffset:  -1,
			preprocDefineNameEnd: -1,
		}
	},
}

const (
	cPreprocNormal = iota
	cPreprocAfterDefine
	cPreprocAfterDefineName
	cPreprocAfterDefineParams
	cPreprocAfterName
	cPreprocAfterInclude
	cPreprocConditionalExpr
)

type prefixedToken struct {
	lexeme string
	sym    gotreesitter.Symbol
}

// NewCTokenSource creates a token source for C source text.
func NewCTokenSource(src []byte, lang *gotreesitter.Language) (*CTokenSource, error) {
	if lang == nil {
		return nil, fmt.Errorf("c lexer: language is nil")
	}

	ts := cTokenSourcePool.Get().(*CTokenSource)
	savedPending := ts.pending[:0]
	savedGLRStates := ts.glrStates[:0]
	*ts = CTokenSource{
		src:                  src,
		lang:                 lang,
		cur:                  newSourceCursor(src),
		lastSyntheticOffset:  -1,
		preprocDefineNameEnd: -1,
	}
	ts.pending = savedPending
	ts.glrStates = savedGLRStates

	tl := newTokenLookup(lang, "c")
	ts.identifierSymbol = tl.require("identifier")
	ts.numberSymbol = tl.require("number_literal")
	ts.commentSymbol = tl.optional("comment")
	ts.quoteSymbol = tl.optional("\"")
	ts.apostropheSymbol = tl.optional("'")
	ts.stringContentSymbol = tl.optional("string_content")
	ts.systemLibStringSymbol = tl.optional("system_lib_string")
	ts.escapeSymbol = tl.optional("escape_sequence")
	ts.characterSymbol = tl.optional("character")
	ts.primitiveTypeSymbol = tl.optional("primitive_type")
	ts.preprocEndSymbol = tl.optional("preproc_include_token2")
	ts.newlineSymbol = tl.optional("\n")
	ts.preprocArgSymbol = tl.optional("preproc_arg")
	ts.preprocDirectiveSymbol = tl.optional("preproc_directive")
	ts.newlineSymbol = tl.optional("\n")
	ts.endifSymbol = tl.optional("#endif")
	ts.rBraceSymbol = tl.optional("}")
	if syms := lang.TokenSymbolsByName("("); len(syms) > 0 {
		ts.preprocParamLParen = syms[0]
	}

	if ts.eofSymbol, _ = lang.SymbolByName("end"); ts.eofSymbol == 0 {
		ts.eofSymbol = 0
	}

	ts.buildSymbolTables()

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

// Reset reinitializes this token source for a new source buffer.
func (ts *CTokenSource) Reset(src []byte) {
	ts.src = src
	ts.cur = newSourceCursor(src)
	ts.done = false
	ts.pending = ts.pending[:0]
	ts.preprocState = cPreprocNormal
	ts.parserState = 0
	ts.glrStates = ts.glrStates[:0]
	ts.lastSyntheticOffset = -1
	ts.preprocDefineNameEnd = -1
	ts.preprocOpaqueArgPending = false
	ts.preprocOpaqueArgActive = false
}

// Close clears parser-owned state and returns this token source to the pool.
func (ts *CTokenSource) Close() {
	if ts == nil {
		return
	}
	ts.src = nil
	ts.lang = nil
	ts.cur = sourceCursor{}
	ts.done = false
	ts.pending = ts.pending[:0]
	ts.eofSymbol = 0
	ts.identifierSymbol = 0
	ts.numberSymbol = 0
	ts.commentSymbol = 0
	ts.quoteSymbol = 0
	ts.apostropheSymbol = 0
	ts.stringContentSymbol = 0
	ts.systemLibStringSymbol = 0
	ts.escapeSymbol = 0
	ts.characterSymbol = 0
	ts.primitiveTypeSymbol = 0
	ts.preprocParamLParen = 0
	ts.preprocEndSymbol = 0
	ts.preprocArgSymbol = 0
	ts.preprocDirectiveSymbol = 0
	ts.newlineSymbol = 0
	ts.endifSymbol = 0
	ts.rBraceSymbol = 0
	ts.preprocState = cPreprocNormal
	ts.parserState = 0
	ts.glrStates = ts.glrStates[:0]
	ts.lastSyntheticOffset = -1
	ts.preprocDefineNameEnd = -1
	ts.preprocOpaqueArgPending = false
	ts.preprocOpaqueArgActive = false
	ts.keywordSymbols = nil
	ts.literalSymbols = nil
	ts.literalAlternates = nil
	ts.literalByFirst = [256][]literalMatchCandidate{}
	ts.stringOpeners = nil
	ts.charOpeners = nil
	cTokenSourcePool.Put(ts)
}

// SupportsIncrementalReuse reports that CTokenSource preserves stable token
// boundaries across edits and supports deterministic SkipToByte behavior.
func (ts *CTokenSource) SupportsIncrementalReuse() bool {
	return true
}

func (ts *CTokenSource) SetParserState(state gotreesitter.StateID) {
	ts.parserState = state
}

func (ts *CTokenSource) SetGLRStates(states []gotreesitter.StateID) {
	if len(states) == 0 {
		ts.glrStates = ts.glrStates[:0]
		return
	}
	ts.glrStates = append(ts.glrStates[:0], states...)
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
		ts.cur.skipSpacesAndTabs() // NOT skipWhitespace — preserve \n
		if ts.cur.eof() {
			ts.done = true
			return ts.eofToken()
		}

		b := ts.cur.peekByte()

		if tok, ok := ts.syntheticConditionalEndToken(); ok {
			return tok
		}

		// Newline handling: in preprocessor context, emit as directive terminator
		if b == '\n' {
			ts.preprocOpaqueArgPending = false
			ts.preprocOpaqueArgActive = false
			if ts.preprocState == cPreprocConditionalExpr && ts.newlineSymbol != 0 {
				ts.preprocState = cPreprocNormal
				return ts.lineEndToken(ts.newlineSymbol)
			}
			if ts.preprocState != cPreprocNormal && ts.preprocEndSymbol != 0 {
				ts.preprocState = cPreprocNormal
				return ts.lineEndToken(ts.preprocEndSymbol)
			}
			ts.preprocState = cPreprocNormal
			ts.cur.advanceByte()
			continue
		}
		if b == '#' {
			if tok, ok := ts.directiveToken(); ok {
				return tok
			}
		}
		if ts.preprocState == cPreprocAfterInclude {
			if tok, ok := ts.systemLibStringToken(); ok {
				return tok
			}
			if ts.preprocArgSymbol != 0 && b != '"' {
				if tok, ok := ts.preprocArgToken(); ok {
					return tok
				}
			}
		}
		if ts.preprocState == cPreprocAfterDefineName {
			if ts.cur.offset == ts.preprocDefineNameEnd && ts.cur.peekByte() == '(' {
				ts.preprocState = cPreprocAfterDefineParams
			} else {
				ts.preprocState = cPreprocAfterName
			}
			ts.preprocDefineNameEnd = -1
		}
		if ts.preprocState == cPreprocAfterName && ts.preprocArgSymbol != 0 {
			if tok, ok := ts.preprocArgToken(); ok {
				return tok
			}
		}
		if ts.preprocOpaqueArgActive {
			if tok, ok := ts.opaquePreprocArgToken(); ok {
				return tok
			}
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

		if isCIdentStart(b) {
			tok := ts.identifierOrKeywordToken()
			// After #define, the next identifier is the macro name.
			if ts.preprocState == cPreprocAfterDefine {
				ts.preprocState = cPreprocAfterDefineName
				ts.preprocDefineNameEnd = ts.cur.offset
			}
			return tok
		}
		if ts.shouldLexSignedNumber() {
			return ts.numberToken()
		}
		if isASCIIDigit(b) {
			return ts.numberToken()
		}
		if ts.preprocState == cPreprocAfterDefineParams && ts.cur.peekByte() == '(' && ts.preprocParamLParen != 0 {
			start := ts.cur.offset
			startPt := ts.cur.point()
			ts.cur.advanceByte()
			return makeToken(ts.preprocParamLParen, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
		}
		if tok, ok := ts.literalToken(); ok {
			ts.updatePreprocStateForLiteral(tok.Text)
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
	ts.preprocState = cPreprocNormal
	ts.parserState = 0
	ts.glrStates = ts.glrStates[:0]
	ts.lastSyntheticOffset = -1
	ts.preprocDefineNameEnd = -1
	ts.preprocOpaqueArgPending = false
	ts.preprocOpaqueArgActive = false

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
	if cached, ok := cLexerTablesCache.Load(ts.lang); ok {
		ts.applyLexerTables(cached.(*cLexerTables))
		return
	}

	keywordSymbols := make(map[string]gotreesitter.Symbol)
	literalSymbols := make(map[string]gotreesitter.Symbol)
	literalAlternates := make(map[string][]gotreesitter.Symbol)
	var literalByFirst [256][]literalMatchCandidate

	limit := int(ts.lang.TokenCount)
	if limit > len(ts.lang.SymbolNames) {
		limit = len(ts.lang.SymbolNames)
	}
	literalCandidates := make(map[string][]literalCandidate)

	for i := 0; i < limit; i++ {
		name := ts.lang.SymbolNames[i]
		if name == "" || name == "end" {
			continue
		}
		sym := gotreesitter.Symbol(i)

		switch name {
		case "identifier", "number_literal", "comment", "string_content", "escape_sequence", "character", "primitive_type", "preproc_directive", "preproc_include_token2", "system_lib_string":
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
		candidates := literalCandidates[lexeme]
		duplicate := false
		for _, candidate := range candidates {
			if candidate.sym == sym {
				duplicate = true
				break
			}
		}
		if !duplicate {
			literalCandidates[lexeme] = append(candidates, literalCandidate{
				sym:     sym,
				escapes: escapes,
			})
		}
	}

	for lexeme, candidates := range literalCandidates {
		if len(candidates) == 0 {
			continue
		}
		orderLiteralCandidates(lexeme, candidates)
		ordered := make([]gotreesitter.Symbol, 0, len(candidates))
		for _, candidate := range candidates {
			ordered = append(ordered, candidate.sym)
		}
		literalSymbols[lexeme] = ordered[0]
		matchCandidate := literalMatchCandidate{
			lexeme: lexeme,
			sym:    ordered[0],
		}
		if len(ordered) > 1 {
			literalAlternates[lexeme] = ordered
			matchCandidate.alternates = ordered
		}
		literalByFirst[lexeme[0]] = append(literalByFirst[lexeme[0]], matchCandidate)
	}
	for i := range literalByFirst {
		sortLiteralMatchCandidates(literalByFirst[i])
	}

	base := &CTokenSource{
		literalSymbols:    literalSymbols,
		literalAlternates: literalAlternates,
		quoteSymbol:       ts.quoteSymbol,
		apostropheSymbol:  ts.apostropheSymbol,
	}
	stringOpeners := base.collectOpeners([]string{"u8\"", "L\"", "U\"", "u\"", "\""}, ts.quoteSymbol)
	charOpeners := base.collectOpeners([]string{"u8'", "L'", "U'", "u'", "'"}, ts.apostropheSymbol)

	tables := &cLexerTables{
		keywordSymbols:    keywordSymbols,
		literalSymbols:    literalSymbols,
		literalAlternates: literalAlternates,
		literalByFirst:    literalByFirst,
		stringOpeners:     stringOpeners,
		charOpeners:       charOpeners,
	}
	if actual, loaded := cLexerTablesCache.LoadOrStore(ts.lang, tables); loaded {
		ts.applyLexerTables(actual.(*cLexerTables))
		return
	}
	ts.applyLexerTables(tables)
}

func (ts *CTokenSource) applyLexerTables(tables *cLexerTables) {
	if tables == nil {
		return
	}
	ts.keywordSymbols = tables.keywordSymbols
	ts.literalSymbols = tables.literalSymbols
	ts.literalAlternates = tables.literalAlternates
	ts.literalByFirst = tables.literalByFirst
	ts.stringOpeners = tables.stringOpeners
	ts.charOpeners = tables.charOpeners
}

func orderLiteralCandidates(lexeme string, candidates []literalCandidate) {
	for i := 0; i < len(candidates); i++ {
		best := i
		for j := i + 1; j < len(candidates); j++ {
			if preferLiteralCandidate(lexeme, candidates[j], candidates[best]) {
				best = j
			}
		}
		if best != i {
			candidates[i], candidates[best] = candidates[best], candidates[i]
		}
	}
}

func preferLiteralCandidate(lexeme string, a, b literalCandidate) bool {
	if a.escapes != b.escapes {
		return a.escapes < b.escapes
	}
	switch lexeme {
	case "(", ">":
		return a.sym > b.sym
	}
	return a.sym < b.sym
}

func sortLiteralMatchCandidates(candidates []literalMatchCandidate) {
	for i := 0; i < len(candidates); i++ {
		best := i
		for j := i + 1; j < len(candidates); j++ {
			if len(candidates[j].lexeme) > len(candidates[best].lexeme) {
				best = j
				continue
			}
			if len(candidates[j].lexeme) == len(candidates[best].lexeme) && candidates[j].lexeme < candidates[best].lexeme {
				best = j
			}
		}
		if best != i {
			candidates[i], candidates[best] = candidates[best], candidates[i]
		}
	}
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
		for !ts.cur.eof() {
			if ts.cur.peekByte() == '\\' {
				if ts.cur.offset+1 < len(ts.src) && ts.src[ts.cur.offset+1] == '\n' {
					ts.cur.advanceByte()
					ts.cur.advanceByte()
					continue
				}
				if ts.cur.offset+1 < len(ts.src) && ts.src[ts.cur.offset+1] == '\r' {
					ts.cur.advanceByte()
					ts.cur.advanceByte()
					if !ts.cur.eof() && ts.cur.peekByte() == '\n' {
						ts.cur.advanceByte()
					}
					continue
				}
			}
			if ts.cur.peekByte() == '\n' {
				break
			}
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
		ts.cur.advanceBytes(len(opener.lexeme))
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
		ts.cur.advanceBytes(len(opener.lexeme))
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

	text := bytesToStringNoCopy(ts.src[start:ts.cur.offset])
	sym := ts.identifierSymbol
	if kw, ok := ts.keywordSymbols[text]; ok {
		sym = kw
	} else if ts.primitiveTypeSymbol != 0 && isCPrimitiveType(text) {
		sym = ts.primitiveTypeSymbol
	}
	if ts.preprocState == cPreprocConditionalExpr {
		if isPreprocOpaqueBuiltin(text) {
			ts.preprocOpaqueArgPending = true
		} else {
			ts.preprocOpaqueArgPending = false
		}
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *CTokenSource) numberToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()

	if !ts.cur.eof() && (ts.cur.peekByte() == '+' || ts.cur.peekByte() == '-') {
		ts.cur.advanceByte()
	}
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

func (ts *CTokenSource) shouldLexSignedNumber() bool {
	if ts.numberSymbol == 0 || !ts.hasAction(ts.numberSymbol) || ts.cur.eof() {
		return false
	}
	b := ts.cur.peekByte()
	if b != '+' && b != '-' {
		return false
	}
	next := ts.cur.offset + 1
	if next >= len(ts.src) {
		return false
	}
	if isASCIIDigit(ts.src[next]) {
		return true
	}
	return ts.src[next] == '.' && next+1 < len(ts.src) && isASCIIDigit(ts.src[next+1])
}

func (ts *CTokenSource) literalToken() (gotreesitter.Token, bool) {
	sym, n := ts.matchLiteral()
	if sym == 0 {
		return gotreesitter.Token{}, false
	}
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceBytes(n)
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *CTokenSource) directiveToken() (gotreesitter.Token, bool) {
	if ts.cur.eof() || ts.cur.peekByte() != '#' {
		return gotreesitter.Token{}, false
	}

	end, canonical, specificSym, ok := ts.scanDirective(ts.cur.offset)
	if !ok {
		return gotreesitter.Token{}, false
	}
	if ts.shouldUseGenericDirective(specificSym) {
		return ts.emitGenericDirectiveLine(end), true
	}
	if specificSym == 0 {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceBytes(end - start)
	ts.preprocState = cPreprocNormal
	ts.lastSyntheticOffset = -1
	ts.preprocOpaqueArgPending = false
	ts.preprocOpaqueArgActive = false

	tok := makeToken(specificSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
	ts.updatePreprocStateForLiteral(canonical)
	if specificSym == ts.preprocDirectiveSymbol && canonical != "#if" && canonical != "#elif" {
		ts.preprocState = cPreprocAfterName
	}
	return tok, true
}

func (ts *CTokenSource) scanDirective(offset int) (int, string, gotreesitter.Symbol, bool) {
	if offset < 0 || offset >= len(ts.src) || ts.src[offset] != '#' {
		return 0, "", 0, false
	}

	i := offset + 1
	for i < len(ts.src) {
		switch ts.src[i] {
		case ' ', '\t':
			i++
			continue
		}
		break
	}
	wordStart := i
	for i < len(ts.src) && (isASCIIAlpha(ts.src[i]) || isASCIIDigit(ts.src[i]) || ts.src[i] == '_') {
		i++
	}
	if wordStart == i {
		return 0, "", 0, false
	}

	canonical := "#" + bytesToStringNoCopy(ts.src[wordStart:i])
	sym := ts.literalSymbols[canonical]
	if sym == 0 {
		sym = ts.preprocDirectiveSymbol
	}
	return i, canonical, sym, true
}

func (ts *CTokenSource) shouldUseGenericDirective(specificSym gotreesitter.Symbol) bool {
	if ts.preprocDirectiveSymbol == 0 {
		return false
	}
	if specificSym == 0 {
		return true
	}
	if ts.hasAction(specificSym) {
		return false
	}
	return ts.hasAction(ts.preprocDirectiveSymbol)
}

func (ts *CTokenSource) emitGenericDirectiveLine(directiveEnd int) gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	probe := ts.cur
	probe.advanceBytes(directiveEnd - start)
	dirTok := makeToken(ts.preprocDirectiveSymbol, ts.src, start, directiveEnd, startPt, probe.point())

	argProbe := probe
	argProbe.skipSpacesAndTabs()
	if ts.preprocArgSymbol != 0 && !argProbe.eof() && argProbe.peekByte() != '\n' {
		argStart := argProbe.offset
		argStartPt := argProbe.point()
		for !argProbe.eof() {
			b := argProbe.peekByte()
			if b == '\n' {
				break
			}
			if b == '\\' && argProbe.offset+1 < len(ts.src) && ts.src[argProbe.offset+1] == '\n' {
				argProbe.advanceByte()
				argProbe.advanceByte()
				continue
			}
			argProbe.advanceRune()
		}
		if argProbe.offset > argStart {
			ts.pending = append(ts.pending, makeToken(ts.preprocArgSymbol, ts.src, argStart, argProbe.offset, argStartPt, argProbe.point()))
		}
	}
	if ts.preprocEndSymbol != 0 && !argProbe.eof() && argProbe.peekByte() == '\n' {
		nlStart := argProbe.offset
		nlStartPt := argProbe.point()
		argProbe.advanceByte()
		ts.pending = append(ts.pending, makeToken(ts.preprocEndSymbol, ts.src, nlStart, argProbe.offset, nlStartPt, argProbe.point()))
	}

	ts.cur = argProbe
	ts.preprocState = cPreprocNormal
	ts.lastSyntheticOffset = -1
	ts.preprocOpaqueArgPending = false
	ts.preprocOpaqueArgActive = false
	return dirTok
}

func (ts *CTokenSource) syntheticConditionalEndToken() (gotreesitter.Token, bool) {
	if ts.endifSymbol == 0 || ts.rBraceSymbol == 0 {
		return gotreesitter.Token{}, false
	}
	if ts.cur.eof() || ts.cur.peekByte() != '\n' {
		return gotreesitter.Token{}, false
	}
	if ts.cur.offset == ts.lastSyntheticOffset {
		return gotreesitter.Token{}, false
	}
	if !ts.hasAction(ts.endifSymbol) || ts.hasAction(ts.rBraceSymbol) {
		return gotreesitter.Token{}, false
	}
	if !ts.nextLineStartsWithBraceThenDirective("#endif") {
		return gotreesitter.Token{}, false
	}

	pt := ts.cur.point()
	ts.lastSyntheticOffset = ts.cur.offset
	tok := makeToken(ts.endifSymbol, ts.src, ts.cur.offset, ts.cur.offset, pt, pt)
	tok.Missing = true
	return tok, true
}

func (ts *CTokenSource) nextLineStartsWithBraceThenDirective(want string) bool {
	i := ts.cur.offset + 1
	i = skipCSpacesAndTabs(ts.src, i)
	if i >= len(ts.src) || ts.src[i] != '}' {
		return false
	}
	i++
	i = skipCSpacesAndTabs(ts.src, i)
	if i >= len(ts.src) || ts.src[i] != '\n' {
		return false
	}
	i++
	i = skipCSpacesAndTabs(ts.src, i)
	_, canonical, _, ok := ts.scanDirective(i)
	return ok && canonical == want
}

func (ts *CTokenSource) matchLiteral() (gotreesitter.Symbol, int) {
	if ts.cur.eof() {
		return 0, 0
	}
	candidates := ts.literalByFirst[ts.cur.peekByte()]
	var fallbackSym gotreesitter.Symbol
	var fallbackN int
	for _, candidate := range candidates {
		lexeme := candidate.lexeme
		if !ts.matchAt(lexeme) {
			continue
		}
		if fallbackSym == 0 {
			fallbackSym = candidate.sym
			fallbackN = len(lexeme)
		}
		if len(candidate.alternates) == 0 {
			if ts.hasAction(candidate.sym) {
				return candidate.sym, len(lexeme)
			}
			continue
		}
		for _, sym := range candidate.alternates {
			if ts.hasAction(sym) {
				return sym, len(lexeme)
			}
		}
	}
	return fallbackSym, fallbackN
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

func (ts *CTokenSource) lineEndToken(sym gotreesitter.Symbol) gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte() // consume '\n'
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

// preprocArgToken scans the rest of the line (until \n) as a preproc_arg token.
func (ts *CTokenSource) preprocArgToken() (gotreesitter.Token, bool) {
	ts.cur.skipSpacesAndTabs()
	for !ts.cur.eof() && ts.cur.peekByte() == '\\' {
		next := ts.cur.offset + 1
		if next >= len(ts.src) {
			break
		}
		if ts.src[next] == '\r' {
			if next+1 >= len(ts.src) || ts.src[next+1] != '\n' {
				break
			}
			ts.cur.advanceByte()
			ts.cur.advanceByte()
			ts.cur.advanceByte()
			ts.cur.skipSpacesAndTabs()
			continue
		}
		if ts.src[next] != '\n' {
			break
		}
		ts.cur.advanceByte()
		ts.cur.advanceByte()
		ts.cur.skipSpacesAndTabs()
	}
	if ts.cur.eof() || ts.cur.peekByte() == '\n' {
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()

	// Scan until newline or EOF, handling backslash-newline continuations
	for !ts.cur.eof() {
		b := ts.cur.peekByte()
		if b == '\n' {
			break
		}
		if b == '/' && ts.cur.offset+1 < len(ts.src) {
			next := ts.src[ts.cur.offset+1]
			if next == '/' {
				break
			}
			if next == '*' {
				commentStart := ts.cur
				ts.consumeBlockComment()
				afterComment := ts.cur
				afterComment.skipSpacesAndTabs()
				if afterComment.eof() || afterComment.peekByte() == '\n' {
					ts.cur = commentStart
					break
				}
				continue
			}
		}
		if b == '\\' && ts.cur.offset+1 < len(ts.src) && ts.src[ts.cur.offset+1] == '\n' {
			ts.cur.advanceByte() // backslash
			ts.cur.advanceByte() // newline
			continue
		}
		ts.cur.advanceRune()
	}

	if ts.cur.offset <= start {
		return gotreesitter.Token{}, false
	}

	// Leave preprocState > 0 so the following \n is emitted as a token
	return makeToken(ts.preprocArgSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *CTokenSource) consumeBlockComment() {
	if ts.cur.offset+1 >= len(ts.src) || ts.src[ts.cur.offset] != '/' || ts.src[ts.cur.offset+1] != '*' {
		return
	}
	ts.cur.advanceByte()
	ts.cur.advanceByte()
	for !ts.cur.eof() {
		if ts.cur.peekByte() == '*' && ts.cur.offset+1 < len(ts.src) && ts.src[ts.cur.offset+1] == '/' {
			ts.cur.advanceByte()
			ts.cur.advanceByte()
			return
		}
		ts.cur.advanceRune()
	}
}

func (ts *CTokenSource) systemLibStringToken() (gotreesitter.Token, bool) {
	if ts.systemLibStringSymbol == 0 || ts.cur.eof() || ts.cur.peekByte() != '<' {
		return gotreesitter.Token{}, false
	}
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() {
		b := ts.cur.peekByte()
		if b == '\n' {
			return gotreesitter.Token{}, false
		}
		ts.cur.advanceRune()
		if b == '>' {
			return makeToken(ts.systemLibStringSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
		}
	}
	return gotreesitter.Token{}, false
}

// updatePreprocStateForLiteral tracks directive-specific newline/argument
// behavior for the token-source bridge. Flat directives use preproc_arg +
// preproc_include_token2, while #if/#elif condition lines tokenize their
// expression normally and terminate with the literal newline token.
func (ts *CTokenSource) updatePreprocStateForLiteral(text string) {
	if ts.preprocState == cPreprocConditionalExpr && ts.preprocOpaqueArgPending {
		if text == "(" {
			ts.preprocOpaqueArgPending = false
			ts.preprocOpaqueArgActive = true
		} else if text != "" {
			ts.preprocOpaqueArgPending = false
		}
	}
	if ts.preprocState == cPreprocAfterDefineParams {
		if text == ")" {
			ts.preprocState = cPreprocAfterName
		}
		return
	}
	switch text {
	case "#define":
		ts.preprocState = cPreprocAfterDefine
	case "#include":
		ts.preprocState = cPreprocAfterInclude
	case "#if", "#elif":
		ts.preprocState = cPreprocConditionalExpr
	case "#pragma", "#undef", "#error", "#warning", "#line", "#embed":
		ts.preprocState = cPreprocAfterName
	}
}

// opaquePreprocArgToken collapses feature-test arguments like __has_include(...)
// and __has_embed(...) into one identifier token so legacy C/C++ grammars can
// accept modern preprocessor argument forms.
func (ts *CTokenSource) opaquePreprocArgToken() (gotreesitter.Token, bool) {
	if !ts.preprocOpaqueArgActive || ts.identifierSymbol == 0 {
		ts.preprocOpaqueArgActive = false
		return gotreesitter.Token{}, false
	}

	ts.cur.skipSpacesAndTabs()
	if ts.cur.eof() || ts.cur.peekByte() == '\n' || ts.cur.peekByte() == ')' {
		ts.preprocOpaqueArgActive = false
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	depth := 0
	for !ts.cur.eof() {
		b := ts.cur.peekByte()
		if b == '\n' {
			break
		}
		if b == '/' && ts.cur.offset+1 < len(ts.src) {
			next := ts.src[ts.cur.offset+1]
			if next == '/' {
				break
			}
			if next == '*' {
				ts.consumeBlockComment()
				continue
			}
		}
		if b == '"' || b == '\'' {
			ts.scanOpaqueQuoted(b)
			continue
		}
		if b == '\\' && ts.cur.offset+1 < len(ts.src) {
			next := ts.src[ts.cur.offset+1]
			if next == '\n' {
				ts.cur.advanceByte()
				ts.cur.advanceByte()
				continue
			}
			if next == '\r' {
				ts.cur.advanceByte()
				ts.cur.advanceByte()
				if !ts.cur.eof() && ts.cur.peekByte() == '\n' {
					ts.cur.advanceByte()
				}
				continue
			}
		}
		if b == '(' {
			depth++
			ts.cur.advanceByte()
			continue
		}
		if b == ')' {
			if depth == 0 {
				ts.preprocOpaqueArgActive = false
				if ts.cur.offset <= start {
					return gotreesitter.Token{}, false
				}
				return makeToken(ts.identifierSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
			}
			depth--
			ts.cur.advanceByte()
			continue
		}
		ts.cur.advanceRune()
	}

	ts.preprocOpaqueArgActive = false
	if ts.cur.offset <= start {
		return gotreesitter.Token{}, false
	}
	return makeToken(ts.identifierSymbol, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *CTokenSource) scanOpaqueQuoted(delim byte) {
	ts.cur.advanceByte()
	for !ts.cur.eof() {
		b := ts.cur.peekByte()
		if b == '\n' {
			return
		}
		if b == '\\' {
			ts.cur.advanceByte()
			if !ts.cur.eof() {
				ts.cur.advanceRune()
			}
			continue
		}
		ts.cur.advanceRune()
		if b == delim {
			return
		}
	}
}

func isPreprocOpaqueBuiltin(text string) bool {
	switch text {
	case "__has_include", "__has_include_next", "__has_embed",
		"__has_cpp_attribute", "__has_c_attribute":
		return true
	default:
		return false
	}
}

func (ts *CTokenSource) hasAction(sym gotreesitter.Symbol) bool {
	if ts == nil || ts.lang == nil || sym == 0 {
		return false
	}
	if len(ts.glrStates) > 0 {
		for _, state := range ts.glrStates {
			if ts.lookupActionIndex(state, sym) != 0 {
				return true
			}
		}
		return false
	}
	return ts.lookupActionIndex(ts.parserState, sym) != 0
}

func (ts *CTokenSource) lookupActionIndex(state gotreesitter.StateID, sym gotreesitter.Symbol) uint16 {
	if ts.lang == nil {
		return 0
	}
	denseLimit := len(ts.lang.ParseTable)
	if ts.lang.LargeStateCount > 0 {
		denseLimit = int(ts.lang.LargeStateCount)
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

	smallBase := int(ts.lang.LargeStateCount)
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
			break
		}
		sectionValue := table[pos]
		symbolCount := table[pos+1]
		pos += 2
		for j := uint16(0); j < symbolCount; j++ {
			if pos >= len(table) {
				break
			}
			if table[pos] == uint16(sym) {
				return sectionValue
			}
			pos++
		}
	}
	return 0
}

func skipCSpacesAndTabs(src []byte, offset int) int {
	for offset < len(src) {
		switch src[offset] {
		case ' ', '\t', '\r', '\f':
			offset++
			continue
		}
		break
	}
	return offset
}

func isCIdentStart(b byte) bool {
	return isASCIIAlpha(b) || b == '_'
}

func isCIdentPart(b byte) bool {
	return isCIdentStart(b) || isASCIIDigit(b)
}

func isCPrimitiveType(text string) bool {
	switch text {
	case "char", "int", "float", "double", "void", "_Bool", "_Complex", "bool", "__int128",
		"size_t", "ssize_t", "ptrdiff_t", "intptr_t", "uintptr_t",
		"int8_t", "int16_t", "int32_t", "int64_t",
		"uint8_t", "uint16_t", "uint32_t", "uint64_t",
		"wchar_t", "char16_t", "char32_t":
		return true
	default:
		return false
	}
}
