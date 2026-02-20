package grammars

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"
)

// TomlTokenSource is a lightweight lexer bridge for tree-sitter-toml.
// It focuses on practical coverage for common editor workflows and
// incremental parsing.
type TomlTokenSource struct {
	src  []byte
	lang *gotreesitter.Language
	cur  sourceCursor

	done bool

	eofSymbol gotreesitter.Symbol

	docStartSym gotreesitter.Symbol
	commentSym  gotreesitter.Symbol
	bareKeySym  gotreesitter.Symbol
	booleanSym  gotreesitter.Symbol
	intSym      gotreesitter.Symbol
	floatSym    gotreesitter.Symbol
	lineEndSym  gotreesitter.Symbol

	eqSym      gotreesitter.Symbol
	dotSym     gotreesitter.Symbol
	commaSym   gotreesitter.Symbol
	lbrackSym  gotreesitter.Symbol
	rbrackSym  gotreesitter.Symbol
	lbrack2Sym gotreesitter.Symbol
	rbrack2Sym gotreesitter.Symbol
	lbraceSym  gotreesitter.Symbol
	rbraceSym  gotreesitter.Symbol

	basicStringSym   gotreesitter.Symbol
	literalStringSym gotreesitter.Symbol

	emittedEOFLineEnd bool
	emittedDocStart   bool
}

// NewTomlTokenSource creates a token source for TOML source text.
func NewTomlTokenSource(src []byte, lang *gotreesitter.Language) (*TomlTokenSource, error) {
	if lang == nil {
		return nil, fmt.Errorf("toml lexer: language is nil")
	}

	lookup := newTokenLookup(lang, "toml")

	ts := &TomlTokenSource{
		src:  src,
		lang: lang,
		cur:  newSourceCursor(src),
	}

	ts.eofSymbol, _ = lang.SymbolByName("end")
	ts.docStartSym = lookup.optional("document_token1")
	ts.commentSym = lookup.optional("comment")
	ts.bareKeySym = lookup.require("bare_key")
	ts.booleanSym = lookup.optional("boolean")
	ts.intSym = lookup.optional("integer_token1", "integer_token2", "integer_token3", "integer_token4")
	ts.floatSym = lookup.optional("float_token1", "float_token2")
	ts.lineEndSym = lookup.optional("_line_ending_or_eof")

	ts.eqSym = lookup.optional("=")
	ts.dotSym = lookup.optional(".")
	ts.commaSym = lookup.optional(",")
	ts.lbrackSym = lookup.optional("[")
	ts.rbrackSym = lookup.optional("]")
	ts.lbrack2Sym = lookup.optional("[[")
	ts.rbrack2Sym = lookup.optional("]]")
	ts.lbraceSym = lookup.optional("{")
	ts.rbraceSym = lookup.optional("}")

	ts.basicStringSym = lookup.optional("_basic_string_token1")
	ts.literalStringSym = lookup.optional("_literal_string_token1")

	if err := lookup.err(); err != nil {
		return nil, err
	}
	if ts.intSym == 0 && ts.floatSym == 0 {
		return nil, fmt.Errorf("toml lexer: missing number token symbols")
	}

	return ts, nil
}

// NewTomlTokenSourceOrEOF returns a TOML token source, or EOF-only fallback if
// setup fails.
func NewTomlTokenSourceOrEOF(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ts, err := NewTomlTokenSource(src, lang)
	if err != nil {
		return tokenSourceInitError{sourceLen: uint32(len(src))}
	}
	return ts
}

func (ts *TomlTokenSource) Next() gotreesitter.Token {
	if ts.done {
		return ts.eofToken()
	}

	for {
		if !ts.emittedDocStart && ts.docStartSym != 0 {
			ts.emittedDocStart = true
			pt := ts.cur.point()
			return gotreesitter.Token{
				Symbol:     ts.docStartSym,
				StartByte:  uint32(ts.cur.offset),
				EndByte:    uint32(ts.cur.offset),
				StartPoint: pt,
				EndPoint:   pt,
			}
		}

		if ts.cur.eof() {
			if ts.lineEndSym != 0 && !ts.emittedEOFLineEnd {
				ts.emittedEOFLineEnd = true
				pt := ts.cur.point()
				n := uint32(len(ts.src))
				return gotreesitter.Token{
					Symbol:     ts.lineEndSym,
					StartByte:  n,
					EndByte:    n,
					StartPoint: pt,
					EndPoint:   pt,
				}
			}
			ts.done = true
			return ts.eofToken()
		}

		ch := ts.cur.peekByte()
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\f' {
			ts.cur.advanceByte()
			continue
		}

		if ch == '\n' {
			start := ts.cur.offset
			startPt := ts.cur.point()
			ts.cur.advanceByte()
			if ts.lineEndSym != 0 {
				if ts.cur.eof() {
					ts.emittedEOFLineEnd = true
				}
				return makeToken(ts.lineEndSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
			}
			continue
		}

		if ch == '#' {
			start := ts.cur.offset
			startPt := ts.cur.point()
			for !ts.cur.eof() && ts.cur.peekByte() != '\n' {
				ts.cur.advanceRune()
			}
			if ts.commentSym != 0 {
				return makeToken(ts.commentSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
			}
			continue
		}

		if tok, ok := ts.punctToken(); ok {
			return tok
		}

		if ch == '"' {
			if tok, ok := ts.scanQuotedString('"', ts.basicStringSym); ok {
				return tok
			}
		}
		if ch == '\'' {
			if tok, ok := ts.scanQuotedString('\'', ts.literalStringSym); ok {
				return tok
			}
		}

		if isASCIIDigit(ch) || ch == '+' || ch == '-' {
			return ts.numberToken()
		}

		if isTomlBareKeyStart(ch) {
			return ts.bareKeyOrBooleanToken()
		}

		// Unknown byte: consume and continue.
		ts.cur.advanceRune()
	}
}

func (ts *TomlTokenSource) SkipToByte(offset uint32) gotreesitter.Token {
	target := int(offset)
	if target < 0 {
		target = 0
	}
	if target > len(ts.src) {
		target = len(ts.src)
	}

	ts.done = false
	ts.emittedEOFLineEnd = false
	ts.emittedDocStart = ts.docStartSym == 0 || target > 0

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

func (ts *TomlTokenSource) punctToken() (gotreesitter.Token, bool) {
	if ts.matchLiteralAtCurrent("[[") && ts.lbrack2Sym != 0 {
		return ts.makeLiteralToken(ts.lbrack2Sym, 2), true
	}
	if ts.matchLiteralAtCurrent("]]") && ts.rbrack2Sym != 0 {
		return ts.makeLiteralToken(ts.rbrack2Sym, 2), true
	}

	ch := ts.cur.peekByte()
	switch ch {
	case '=':
		if ts.eqSym != 0 {
			return ts.makeLiteralToken(ts.eqSym, 1), true
		}
	case '.':
		if ts.dotSym != 0 {
			return ts.makeLiteralToken(ts.dotSym, 1), true
		}
	case ',':
		if ts.commaSym != 0 {
			return ts.makeLiteralToken(ts.commaSym, 1), true
		}
	case '[':
		if ts.lbrackSym != 0 {
			return ts.makeLiteralToken(ts.lbrackSym, 1), true
		}
	case ']':
		if ts.rbrackSym != 0 {
			return ts.makeLiteralToken(ts.rbrackSym, 1), true
		}
	case '{':
		if ts.lbraceSym != 0 {
			return ts.makeLiteralToken(ts.lbraceSym, 1), true
		}
	case '}':
		if ts.rbraceSym != 0 {
			return ts.makeLiteralToken(ts.rbraceSym, 1), true
		}
	}
	return gotreesitter.Token{}, false
}

func (ts *TomlTokenSource) makeLiteralToken(sym gotreesitter.Symbol, n int) gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	for i := 0; i < n && !ts.cur.eof(); i++ {
		ts.cur.advanceByte()
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *TomlTokenSource) scanQuotedString(quote byte, sym gotreesitter.Symbol) (gotreesitter.Token, bool) {
	if sym == 0 || ts.cur.peekByte() != quote {
		return gotreesitter.Token{}, false
	}
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() {
		ch := ts.cur.peekByte()
		if quote == '"' && ch == '\\' {
			ts.cur.advanceByte()
			if !ts.cur.eof() {
				ts.cur.advanceRune()
			}
			continue
		}
		if ch == quote {
			ts.cur.advanceByte()
			break
		}
		ts.cur.advanceRune()
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point()), true
}

func (ts *TomlTokenSource) bareKeyOrBooleanToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()
	ts.cur.advanceByte()
	for !ts.cur.eof() && isTomlBareKeyPart(ts.cur.peekByte()) {
		ts.cur.advanceByte()
	}

	text := string(ts.src[start:ts.cur.offset])
	if ts.booleanSym != 0 && (text == "true" || text == "false") {
		return makeToken(ts.booleanSym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
	}
	return makeToken(ts.bareKeySym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *TomlTokenSource) numberToken() gotreesitter.Token {
	start := ts.cur.offset
	startPt := ts.cur.point()

	if !ts.cur.eof() && (ts.cur.peekByte() == '+' || ts.cur.peekByte() == '-') {
		ts.cur.advanceByte()
	}

	isFloat := false

	if ts.matchLiteralAtCurrent("0x") || ts.matchLiteralAtCurrent("0X") ||
		ts.matchLiteralAtCurrent("0o") || ts.matchLiteralAtCurrent("0O") ||
		ts.matchLiteralAtCurrent("0b") || ts.matchLiteralAtCurrent("0B") {
		ts.cur.advanceByte()
		ts.cur.advanceByte()
		for !ts.cur.eof() && (isASCIIHex(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}
	} else {
		for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
			ts.cur.advanceByte()
		}

		if !ts.cur.eof() && ts.cur.peekByte() == '.' {
			isFloat = true
			ts.cur.advanceByte()
			for !ts.cur.eof() && (isASCIIDigit(ts.cur.peekByte()) || ts.cur.peekByte() == '_') {
				ts.cur.advanceByte()
			}
		}

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
	}

	sym := ts.intSym
	if isFloat {
		sym = firstNonZeroSymbol(ts.floatSym, ts.intSym)
	}
	return makeToken(sym, ts.src, start, ts.cur.offset, startPt, ts.cur.point())
}

func (ts *TomlTokenSource) matchLiteralAtCurrent(lexeme string) bool {
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

func (ts *TomlTokenSource) eofToken() gotreesitter.Token {
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

func isTomlBareKeyStart(b byte) bool {
	return isASCIIAlpha(b) || isASCIIDigit(b) || b == '_' || b == '-'
}

func isTomlBareKeyPart(b byte) bool {
	return isTomlBareKeyStart(b)
}
