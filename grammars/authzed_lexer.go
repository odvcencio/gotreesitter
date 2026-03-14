package grammars

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"
)

// AuthzedTokenSource is a custom lexer for the authzed (SpiceDB/Zanzibar)
// permission language. The grammar is derived from tree-sitter-go and has
// keyword-like named tokens (definition_literal, relation_literal, etc.)
// that the generic lexer cannot handle, plus significant newlines.
type AuthzedTokenSource struct {
	src  []byte
	lang *gotreesitter.Language
	cur  sourceCursor

	done    bool
	pending []gotreesitter.Token

	// Token symbols
	eofSym        gotreesitter.Symbol
	identifierSym gotreesitter.Symbol
	newlineSym    gotreesitter.Symbol
	semicolonSym  gotreesitter.Symbol
	nullSym       gotreesitter.Symbol // \0

	// Keywords (identifier-like tokens with special symbol IDs)
	importSym     gotreesitter.Symbol
	asSym         gotreesitter.Symbol
	inSym         gotreesitter.Symbol
	allSym        gotreesitter.Symbol
	anySym        gotreesitter.Symbol
	intTypeSym    gotreesitter.Symbol
	uintTypeSym   gotreesitter.Symbol
	boolTypeSym   gotreesitter.Symbol
	strTypeSym    gotreesitter.Symbol
	doubleTypeSym gotreesitter.Symbol
	bytesSym      gotreesitter.Symbol
	durationSym   gotreesitter.Symbol
	timestampSym  gotreesitter.Symbol
	nilSym        gotreesitter.Symbol
	trueSym       gotreesitter.Symbol
	falseSym      gotreesitter.Symbol

	// Special named keyword tokens
	definitionLitSym gotreesitter.Symbol
	caveatLitSym     gotreesitter.Symbol
	relationLitSym   gotreesitter.Symbol
	permissionLitSym gotreesitter.Symbol

	// Literals
	intLitSym       gotreesitter.Symbol
	floatLitSym     gotreesitter.Symbol
	imaginaryLitSym gotreesitter.Symbol
	rawStringLitSym gotreesitter.Symbol

	// String split tokens
	openQuoteSym  gotreesitter.Symbol
	closeQuoteSym gotreesitter.Symbol
	strContentSym gotreesitter.Symbol
	escapeSeqSym  gotreesitter.Symbol

	// Punctuation
	dotSym    gotreesitter.Symbol
	lparenSym gotreesitter.Symbol
	rparenSym gotreesitter.Symbol
	commaSym  gotreesitter.Symbol
	lbrackSym gotreesitter.Symbol
	rbrackSym gotreesitter.Symbol
	lbraceSym gotreesitter.Symbol
	rbraceSym gotreesitter.Symbol
	equalSym  gotreesitter.Symbol
	colonSym  gotreesitter.Symbol

	// Operators
	starSym     gotreesitter.Symbol
	slashSym    gotreesitter.Symbol
	percentSym  gotreesitter.Symbol
	shlSym      gotreesitter.Symbol
	shrSym      gotreesitter.Symbol
	ampSym      gotreesitter.Symbol
	ampCaretSym gotreesitter.Symbol
	plusSym     gotreesitter.Symbol
	minusSym    gotreesitter.Symbol
	pipeSym     gotreesitter.Symbol
	caretSym    gotreesitter.Symbol
	eqeqSym     gotreesitter.Symbol
	neqSym      gotreesitter.Symbol
	ltSym       gotreesitter.Symbol
	leSym       gotreesitter.Symbol
	gtSym       gotreesitter.Symbol
	geSym       gotreesitter.Symbol
	landSym     gotreesitter.Symbol
	lorSym      gotreesitter.Symbol

	// Special
	stabbySym       gotreesitter.Symbol // ->
	hashLitSym      gotreesitter.Symbol // #
	wildcardTypeSym gotreesitter.Symbol

	// Comment
	commentSym    gotreesitter.Symbol
	whitespaceSym gotreesitter.Symbol

	// Keyword map for identifier resolution
	keywordMap map[string]gotreesitter.Symbol
}

// NewAuthzedTokenSource creates a token source for authzed source text.
func NewAuthzedTokenSource(src []byte, lang *gotreesitter.Language) (*AuthzedTokenSource, error) {
	if lang == nil {
		return nil, fmt.Errorf("authzed lexer: language is nil")
	}

	ts := &AuthzedTokenSource{
		src:  src,
		lang: lang,
		cur:  newSourceCursor(src),
	}

	tl := newTokenLookup(lang, "authzed")

	// EOF
	if eof, ok := lang.SymbolByName("end"); ok {
		ts.eofSym = eof
	}

	// Core tokens
	ts.identifierSym = tl.require("identifier")
	ts.newlineSym = tl.require("\n")
	ts.semicolonSym = tl.require(";")
	ts.nullSym = tl.optional("\\0")

	// Keywords
	ts.importSym = tl.require("import")
	ts.asSym = tl.require("as")
	ts.inSym = tl.require("in")
	ts.allSym = tl.require("all")
	ts.anySym = tl.require("any")
	ts.intTypeSym = tl.require("int")
	ts.uintTypeSym = tl.require("uint")
	ts.boolTypeSym = tl.require("bool")
	ts.strTypeSym = tl.require("string")
	ts.doubleTypeSym = tl.require("double")
	ts.bytesSym = tl.require("bytes")
	ts.durationSym = tl.require("duration")
	ts.timestampSym = tl.require("timestamp")
	ts.nilSym = tl.require("nil")
	ts.trueSym = tl.require("true")
	ts.falseSym = tl.require("false")

	// Special named keyword tokens
	ts.definitionLitSym = tl.require("definition_literal")
	ts.caveatLitSym = tl.require("caveat_literal")
	ts.relationLitSym = tl.require("relation_literal")
	ts.permissionLitSym = tl.require("permission_literal")

	// Numeric literals
	ts.intLitSym = tl.require("int_literal")
	ts.floatLitSym = tl.require("float_literal")
	ts.imaginaryLitSym = tl.require("imaginary_literal")
	ts.rawStringLitSym = tl.require("raw_string_literal")

	// String split tokens - note the grammar has two " symbols
	quoteSyms := lang.TokenSymbolsByName("\"")
	if len(quoteSyms) >= 2 {
		ts.openQuoteSym = quoteSyms[0]
		ts.closeQuoteSym = quoteSyms[1]
	} else if len(quoteSyms) == 1 {
		ts.openQuoteSym = quoteSyms[0]
		ts.closeQuoteSym = quoteSyms[0]
	} else {
		return nil, fmt.Errorf("authzed lexer: quote symbol not found")
	}
	ts.strContentSym = tl.require("_interpreted_string_literal_basic_content")
	ts.escapeSeqSym = tl.require("escape_sequence")

	// Punctuation
	ts.dotSym = tl.require(".")
	ts.lparenSym = tl.require("(")
	ts.rparenSym = tl.require(")")
	ts.commaSym = tl.require(",")
	ts.lbrackSym = tl.require("[")
	ts.rbrackSym = tl.require("]")
	ts.lbraceSym = tl.require("{")
	ts.rbraceSym = tl.require("}")
	ts.equalSym = tl.require("=")
	ts.colonSym = tl.require(":")

	// Operators
	ts.starSym = tl.require("*")
	ts.slashSym = tl.require("/")
	ts.percentSym = tl.require("%")
	ts.shlSym = tl.require("<<")
	ts.shrSym = tl.require(">>")
	ts.ampSym = tl.require("&")
	ts.ampCaretSym = tl.require("&^")
	ts.plusSym = tl.require("+")
	ts.minusSym = tl.require("-")
	ts.pipeSym = tl.require("|")
	ts.caretSym = tl.require("^")
	ts.eqeqSym = tl.require("==")
	ts.neqSym = tl.require("!=")
	ts.ltSym = tl.require("<")
	ts.leSym = tl.require("<=")
	ts.gtSym = tl.require(">")
	ts.geSym = tl.require(">=")
	ts.landSym = tl.require("&&")
	ts.lorSym = tl.require("||")

	// Special
	ts.stabbySym = tl.require("stabby")
	ts.hashLitSym = tl.require("hash_literal")
	ts.wildcardTypeSym = tl.require("wildcard_type")

	// Comment and whitespace
	ts.commentSym = tl.require("comment")
	ts.whitespaceSym = tl.optional("_whitespace")

	if err := tl.err(); err != nil {
		return nil, err
	}

	// Build keyword map: identifier text -> symbol
	ts.keywordMap = map[string]gotreesitter.Symbol{
		"definition": ts.definitionLitSym,
		"caveat":     ts.caveatLitSym,
		"relation":   ts.relationLitSym,
		"permission": ts.permissionLitSym,
		"import":     ts.importSym,
		"as":         ts.asSym,
		"in":         ts.inSym,
		"all":        ts.allSym,
		"any":        ts.anySym,
		"int":        ts.intTypeSym,
		"uint":       ts.uintTypeSym,
		"bool":       ts.boolTypeSym,
		"string":     ts.strTypeSym,
		"double":     ts.doubleTypeSym,
		"bytes":      ts.bytesSym,
		"duration":   ts.durationSym,
		"timestamp":  ts.timestampSym,
		"nil":        ts.nilSym,
		"true":       ts.trueSym,
		"false":      ts.falseSym,
	}

	return ts, nil
}

// NewAuthzedTokenSourceOrEOF returns an authzed token source, or EOF-only
// fallback if symbol setup fails.
func NewAuthzedTokenSourceOrEOF(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ts, err := NewAuthzedTokenSource(src, lang)
	if err != nil {
		return tokenSourceInitError{sourceLen: uint32(len(src))}
	}
	return ts
}

// Reset reinitializes this token source for a new source buffer.
func (ts *AuthzedTokenSource) Reset(src []byte) {
	ts.src = src
	ts.cur = newSourceCursor(src)
	ts.done = false
	ts.pending = ts.pending[:0]
}

// SupportsIncrementalReuse reports that AuthzedTokenSource preserves stable
// token boundaries across edits and supports deterministic SkipToByte behavior.
func (ts *AuthzedTokenSource) SupportsIncrementalReuse() bool {
	return true
}

// Next returns the next token from the source.
func (ts *AuthzedTokenSource) Next() gotreesitter.Token {
	if len(ts.pending) > 0 {
		tok := ts.pending[0]
		ts.pending = ts.pending[1:]
		return tok
	}
	if ts.done {
		return ts.eofToken()
	}

	for {
		// Skip only spaces and tabs (NOT newlines — they are significant tokens)
		ts.cur.skipSpacesAndTabs()

		if ts.cur.eof() {
			ts.done = true
			return ts.eofToken()
		}

		b := ts.cur.peekByte()

		// Newline is a significant token
		if b == '\n' {
			return ts.newlineToken()
		}

		// Carriage return: skip (handle \r\n)
		if b == '\r' {
			ts.cur.advanceByte()
			continue
		}

		// Null byte
		if b == 0 {
			start := ts.cur.offset
			startPt := ts.cur.point()
			ts.cur.advanceByte()
			return makeToken(ts.nullSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
		}

		// Comments: // style
		if b == '/' && ts.cur.offset+1 < len(ts.src) && ts.src[ts.cur.offset+1] == '/' {
			return ts.lineComment()
		}

		// String literals
		if b == '"' {
			return ts.interpretedString()
		}

		// Raw string literals (backtick)
		if b == '`' {
			return ts.rawString()
		}

		// Numbers
		if isASCIIDigit(b) {
			return ts.numberLiteral()
		}

		// Identifiers and keywords
		if isAuthzedIdentStart(b) {
			return ts.identifierOrKeyword()
		}

		// Multi-character operators (must check before single-char)
		if tok, ok := ts.multiCharOp(); ok {
			return tok
		}

		// Single-character punctuation and operators
		if tok, ok := ts.singleCharToken(b); ok {
			return tok
		}

		// Unknown byte: skip
		ts.cur.advanceRune()
	}
}

// SkipToByte advances to the given byte offset and returns the next token.
func (ts *AuthzedTokenSource) SkipToByte(offset uint32) gotreesitter.Token {
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

func (ts *AuthzedTokenSource) newlineToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte() // consume '\n'
	return makeToken(ts.newlineSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *AuthzedTokenSource) lineComment() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	// Consume // and the rest of the line (but NOT the newline)
	ts.cur.advanceByte() // /
	ts.cur.advanceByte() // /
	for !ts.cur.eof() && ts.cur.peekByte() != '\n' {
		ts.cur.advanceRune()
	}
	return makeToken(ts.commentSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *AuthzedTokenSource) interpretedString() gotreesitter.Token {
	// Opening quote
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte() // consume "
	openTok := makeToken(ts.openQuoteSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())

	// Scan body: content segments and escape sequences
	segStart := ts.cur.offset
	segPt := ts.cur.point()

	for !ts.cur.eof() {
		ch := ts.cur.peekByte()

		if ch == '"' {
			// Flush content segment if any
			if segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(ts.strContentSym, ts.src, segStart, ts.cur.offset, segPt, ts.cur.point()))
			}
			// Closing quote
			closeStart := ts.cur.offset
			closePt := ts.cur.point()
			ts.cur.advanceByte()
			ts.pending = append(ts.pending, makeToken(ts.closeQuoteSym, ts.src, closeStart, ts.cur.offset, closePt, ts.cur.point()))
			return openTok
		}

		if ch == '\\' {
			// Flush content segment if any
			if segStart < ts.cur.offset {
				ts.pending = append(ts.pending, makeToken(ts.strContentSym, ts.src, segStart, ts.cur.offset, segPt, ts.cur.point()))
			}
			// Escape sequence
			escStart := ts.cur.offset
			escPt := ts.cur.point()
			ts.cur.advanceByte() // backslash
			if !ts.cur.eof() {
				next := ts.cur.peekByte()
				switch next {
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
					// Simple escape: \n, \t, \\, \", \0, etc.
					ts.cur.advanceRune()
				}
			}
			ts.pending = append(ts.pending, makeToken(ts.escapeSeqSym, ts.src, escStart, ts.cur.offset, escPt, ts.cur.point()))
			segStart = ts.cur.offset
			segPt = ts.cur.point()
			continue
		}

		if ch == '\n' {
			// Unterminated string at newline
			break
		}

		ts.cur.advanceRune()
	}

	// Unterminated string: flush remaining content
	if segStart < ts.cur.offset {
		ts.pending = append(ts.pending, makeToken(ts.strContentSym, ts.src, segStart, ts.cur.offset, segPt, ts.cur.point()))
	}
	return openTok
}

func (ts *AuthzedTokenSource) rawString() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte() // consume opening `

	// Scan until closing backtick
	for !ts.cur.eof() {
		if ts.cur.peekByte() == '`' {
			ts.cur.advanceByte()
			return makeToken(ts.rawStringLitSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
		}
		ts.cur.advanceRune()
	}

	// Unterminated raw string
	return makeToken(ts.rawStringLitSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *AuthzedTokenSource) numberLiteral() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	isFloat := false

	// Hex prefix
	if ts.cur.peekByte() == '0' && ts.cur.offset+1 < len(ts.src) &&
		(ts.src[ts.cur.offset+1] == 'x' || ts.src[ts.cur.offset+1] == 'X') {
		ts.cur.advanceByte() // 0
		ts.cur.advanceByte() // x/X
		for !ts.cur.eof() && (isASCIIHex(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	} else if ts.cur.peekByte() == '0' && ts.cur.offset+1 < len(ts.src) &&
		(ts.src[ts.cur.offset+1] == 'o' || ts.src[ts.cur.offset+1] == 'O') {
		// Octal
		ts.cur.advanceByte()
		ts.cur.advanceByte()
		for !ts.cur.eof() && (ts.cur.peekByte() >= '0' && ts.cur.peekByte() <= '7' || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	} else if ts.cur.peekByte() == '0' && ts.cur.offset+1 < len(ts.src) &&
		(ts.src[ts.cur.offset+1] == 'b' || ts.src[ts.cur.offset+1] == 'B') {
		// Binary
		ts.cur.advanceByte()
		ts.cur.advanceByte()
		for !ts.cur.eof() && (ts.cur.peekByte() == '0' || ts.cur.peekByte() == '1' || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	} else {
		// Decimal digits
		for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	}

	// Fractional part
	if !ts.cur.eof() && ts.cur.peekByte() == '.' {
		// Only treat as float if not followed by another dot (e.g., range operator)
		if ts.cur.offset+1 >= len(ts.src) || ts.src[ts.cur.offset+1] != '.' {
			isFloat = true
			ts.cur.advanceByte()
			for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
				ts.cur.advanceByte()
			}
		}
	}

	// Exponent
	if !ts.cur.eof() && (ts.cur.peekByte() == 'e' || ts.cur.peekByte() == 'E') {
		isFloat = true
		ts.cur.advanceByte()
		if !ts.cur.eof() && (ts.cur.peekByte() == '+' || ts.cur.peekByte() == '-') {
			ts.cur.advanceByte()
		}
		for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	}

	// Imaginary suffix
	if !ts.cur.eof() && ts.cur.peekByte() == 'i' {
		ts.cur.advanceByte()
		return makeToken(ts.imaginaryLitSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
	}

	if isFloat {
		return makeToken(ts.floatLitSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
	}
	return makeToken(ts.intLitSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *AuthzedTokenSource) identifierOrKeyword() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() && isAuthzedIdentPart(ts.cur.peekByte()) {
		ts.cur.advanceByte()
	}

	text := string(ts.src[start:ts.cur.offset])

	// Check keyword map
	if sym, ok := ts.keywordMap[text]; ok {
		return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
	}

	return makeToken(ts.identifierSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *AuthzedTokenSource) multiCharOp() (gotreesitter.Token, bool) {
	if ts.cur.offset+1 >= len(ts.src) {
		return gotreesitter.Token{}, false
	}

	b0 := ts.src[ts.cur.offset]
	b1 := ts.src[ts.cur.offset+1]
	var b2 byte
	if ts.cur.offset+2 < len(ts.src) {
		b2 = ts.src[ts.cur.offset+2]
	}

	start := ts.cur.offset
	startPt := ts.cur.point()

	// Three-character operators
	if b0 == '&' && b1 == '^' && b2 != 0 {
		// &^ (and-not)
		ts.cur.advanceByte()
		ts.cur.advanceByte()
		return makeToken(ts.ampCaretSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
	}

	// Two-character operators
	var sym gotreesitter.Symbol
	switch {
	case b0 == '-' && b1 == '>':
		sym = ts.stabbySym
	case b0 == '<' && b1 == '<':
		sym = ts.shlSym
	case b0 == '>' && b1 == '>':
		sym = ts.shrSym
	case b0 == '=' && b1 == '=':
		sym = ts.eqeqSym
	case b0 == '!' && b1 == '=':
		sym = ts.neqSym
	case b0 == '<' && b1 == '=':
		sym = ts.leSym
	case b0 == '>' && b1 == '=':
		sym = ts.geSym
	case b0 == '&' && b1 == '&':
		sym = ts.landSym
	case b0 == '|' && b1 == '|':
		sym = ts.lorSym
	default:
		return gotreesitter.Token{}, false
	}

	ts.cur.advanceByte()
	ts.cur.advanceByte()
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *AuthzedTokenSource) singleCharToken(b byte) (gotreesitter.Token, bool) {
	var sym gotreesitter.Symbol
	switch b {
	case '{':
		sym = ts.lbraceSym
	case '}':
		sym = ts.rbraceSym
	case '(':
		sym = ts.lparenSym
	case ')':
		sym = ts.rparenSym
	case '[':
		sym = ts.lbrackSym
	case ']':
		sym = ts.rbrackSym
	case '.':
		sym = ts.dotSym
	case ',':
		sym = ts.commaSym
	case ':':
		sym = ts.colonSym
	case ';':
		sym = ts.semicolonSym
	case '=':
		sym = ts.equalSym
	case '+':
		sym = ts.plusSym
	case '-':
		sym = ts.minusSym
	case '*':
		sym = ts.starSym
	case '/':
		sym = ts.slashSym
	case '%':
		sym = ts.percentSym
	case '&':
		sym = ts.ampSym
	case '|':
		sym = ts.pipeSym
	case '^':
		sym = ts.caretSym
	case '<':
		sym = ts.ltSym
	case '>':
		sym = ts.gtSym
	case '#':
		sym = ts.hashLitSym
	default:
		return gotreesitter.Token{}, false
	}

	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *AuthzedTokenSource) eofToken() gotreesitter.Token {
	n := uint32(len(ts.src))
	pt := ts.cur.point()
	return gotreesitter.Token{
		Symbol:     ts.eofSym,
		StartByte:  n,
		EndByte:    n,
		StartPoint: pt,
		EndPoint:   pt,
	}
}

func isAuthzedIdentStart(b byte) bool {
	return isASCIIAlpha(b) || b == '_'
}

func isAuthzedIdentPart(b byte) bool {
	return isASCIIAlpha(b) || isASCIIDigit(b) || b == '_'
}
