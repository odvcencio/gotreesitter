package grammars

import (
	"fmt"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
)

// JSONTokenSource bridges raw JSON source to tree-sitter token symbols.
// It emits punctuation/literal tokens directly and splits strings into:
// open quote, string_content / escape_sequence chunks, close quote.
type JSONTokenSource struct {
	src  []byte
	lang *gotreesitter.Language

	offset int
	row    uint32
	col    uint32
	done   bool

	pending []gotreesitter.Token

	eofSymbol           gotreesitter.Symbol
	lbraceSymbol        gotreesitter.Symbol
	rbraceSymbol        gotreesitter.Symbol
	lbrackSymbol        gotreesitter.Symbol
	rbrackSymbol        gotreesitter.Symbol
	colonSymbol         gotreesitter.Symbol
	commaSymbol         gotreesitter.Symbol
	quoteSymbol         gotreesitter.Symbol
	stringContentSymbol gotreesitter.Symbol
	escapeSymbol        gotreesitter.Symbol
	numberSymbol        gotreesitter.Symbol
	trueSymbol          gotreesitter.Symbol
	falseSymbol         gotreesitter.Symbol
	nullSymbol          gotreesitter.Symbol
	commentSymbol       gotreesitter.Symbol
}

// NewJSONTokenSource creates a token source for JSON.
func NewJSONTokenSource(src []byte, lang *gotreesitter.Language) (*JSONTokenSource, error) {
	ts := &JSONTokenSource{
		src:  src,
		lang: lang,
	}

	if lang == nil {
		return nil, fmt.Errorf("json lexer: language is nil")
	}

	var firstErr error
	tokenSym := func(name string) gotreesitter.Symbol {
		syms := lang.TokenSymbolsByName(name)
		if len(syms) == 0 {
			if firstErr == nil {
				firstErr = fmt.Errorf("json lexer: token symbol %q not found", name)
			}
			return 0
		}
		return syms[0]
	}

	ts.eofSymbol = 0
	if eof, ok := lang.SymbolByName("end"); ok {
		ts.eofSymbol = eof
	}

	ts.lbraceSymbol = tokenSym("{")
	ts.rbraceSymbol = tokenSym("}")
	ts.lbrackSymbol = tokenSym("[")
	ts.rbrackSymbol = tokenSym("]")
	ts.colonSymbol = tokenSym(":")
	ts.commaSymbol = tokenSym(",")
	ts.quoteSymbol = tokenSym("\"")
	ts.stringContentSymbol = tokenSym("string_content")
	ts.escapeSymbol = tokenSym("escape_sequence")
	ts.numberSymbol = tokenSym("number")
	ts.trueSymbol = tokenSym("true")
	ts.falseSymbol = tokenSym("false")
	ts.nullSymbol = tokenSym("null")

	// Comments are optional in some JSON dialects.
	if syms := lang.TokenSymbolsByName("comment"); len(syms) > 0 {
		ts.commentSymbol = syms[0]
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return ts, nil
}

// NewJSONTokenSourceOrEOF returns a token source for callers that cannot
// surface constructor errors through their API.
func NewJSONTokenSourceOrEOF(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ts, err := NewJSONTokenSource(src, lang)
	if err != nil {
		return tokenSourceInitError{sourceLen: uint32(len(src))}
	}
	return ts
}

func (ts *JSONTokenSource) Next() gotreesitter.Token {
	if len(ts.pending) > 0 {
		tok := ts.pending[0]
		ts.pending = ts.pending[1:]
		return tok
	}

	if ts.done {
		return ts.eofToken()
	}

	for {
		ts.skipWhitespace()
		if ts.offset >= len(ts.src) {
			ts.done = true
			return ts.eofToken()
		}

		ch := ts.src[ts.offset]
		switch ch {
		case '{':
			return ts.singleByteToken(ts.lbraceSymbol)
		case '}':
			return ts.singleByteToken(ts.rbraceSymbol)
		case '[':
			return ts.singleByteToken(ts.lbrackSymbol)
		case ']':
			return ts.singleByteToken(ts.rbrackSymbol)
		case ':':
			return ts.singleByteToken(ts.colonSymbol)
		case ',':
			return ts.singleByteToken(ts.commaSymbol)
		case '"':
			return ts.stringTokens()
		case '/':
			if tok, ok := ts.commentToken(); ok {
				return tok
			}
			ts.advanceOneRune()
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			if tok, ok := ts.numberToken(); ok {
				return tok
			}
			ts.advanceOneRune()
		case 't':
			if tok, ok := ts.literalToken("true", ts.trueSymbol); ok {
				return tok
			}
			ts.advanceOneRune()
		case 'f':
			if tok, ok := ts.literalToken("false", ts.falseSymbol); ok {
				return tok
			}
			ts.advanceOneRune()
		case 'n':
			if tok, ok := ts.literalToken("null", ts.nullSymbol); ok {
				return tok
			}
			ts.advanceOneRune()
		default:
			// Unknown byte: consume one rune and continue.
			ts.advanceOneRune()
		}
	}
}

func (ts *JSONTokenSource) SkipToByte(offset uint32) gotreesitter.Token {
	target := int(offset)
	if target < 0 {
		target = 0
	}
	if target > len(ts.src) {
		target = len(ts.src)
	}

	// Clear pending split-string tokens because caller requested a jump.
	ts.pending = nil
	ts.done = false

	if target < ts.offset {
		ts.offset = 0
		ts.row = 0
		ts.col = 0
	}
	for ts.offset < target {
		ts.advanceOneRune()
	}
	if ts.offset >= len(ts.src) {
		ts.done = true
		return ts.eofToken()
	}
	return ts.Next()
}

func (ts *JSONTokenSource) singleByteToken(sym gotreesitter.Symbol) gotreesitter.Token {
	startOffset := ts.offset
	startPoint := gotreesitter.Point{Row: ts.row, Column: ts.col}
	ts.advanceByte()
	endPoint := gotreesitter.Point{Row: ts.row, Column: ts.col}
	return gotreesitter.Token{
		Symbol:     sym,
		Text:       string(ts.src[startOffset:ts.offset]),
		StartByte:  uint32(startOffset),
		EndByte:    uint32(ts.offset),
		StartPoint: startPoint,
		EndPoint:   endPoint,
	}
}

func (ts *JSONTokenSource) stringTokens() gotreesitter.Token {
	// Opening quote.
	openStartOffset := ts.offset
	openStart := gotreesitter.Point{Row: ts.row, Column: ts.col}
	ts.advanceByte()
	openEnd := gotreesitter.Point{Row: ts.row, Column: ts.col}
	openTok := gotreesitter.Token{
		Symbol:     ts.quoteSymbol,
		Text:       string(ts.src[openStartOffset:ts.offset]),
		StartByte:  uint32(openStartOffset),
		EndByte:    uint32(ts.offset),
		StartPoint: openStart,
		EndPoint:   openEnd,
	}

	segStartOffset := ts.offset
	segStartPoint := gotreesitter.Point{Row: ts.row, Column: ts.col}

	for ts.offset < len(ts.src) {
		ch := ts.src[ts.offset]
		switch ch {
		case '"':
			if segStartOffset < ts.offset {
				ts.pending = append(ts.pending, gotreesitter.Token{
					Symbol:     ts.stringContentSymbol,
					Text:       string(ts.src[segStartOffset:ts.offset]),
					StartByte:  uint32(segStartOffset),
					EndByte:    uint32(ts.offset),
					StartPoint: segStartPoint,
					EndPoint:   gotreesitter.Point{Row: ts.row, Column: ts.col},
				})
			}
			closeStartOffset := ts.offset
			closeStart := gotreesitter.Point{Row: ts.row, Column: ts.col}
			ts.advanceByte()
			closeEnd := gotreesitter.Point{Row: ts.row, Column: ts.col}
			ts.pending = append(ts.pending, gotreesitter.Token{
				Symbol:     ts.quoteSymbol,
				Text:       string(ts.src[closeStartOffset:ts.offset]),
				StartByte:  uint32(closeStartOffset),
				EndByte:    uint32(ts.offset),
				StartPoint: closeStart,
				EndPoint:   closeEnd,
			})
			return openTok

		case '\\':
			if segStartOffset < ts.offset {
				ts.pending = append(ts.pending, gotreesitter.Token{
					Symbol:     ts.stringContentSymbol,
					Text:       string(ts.src[segStartOffset:ts.offset]),
					StartByte:  uint32(segStartOffset),
					EndByte:    uint32(ts.offset),
					StartPoint: segStartPoint,
					EndPoint:   gotreesitter.Point{Row: ts.row, Column: ts.col},
				})
			}
			escStartOffset := ts.offset
			escStart := gotreesitter.Point{Row: ts.row, Column: ts.col}
			ts.advanceByte() // '\'
			if ts.offset < len(ts.src) {
				if ts.src[ts.offset] == 'u' {
					ts.advanceByte()
					for i := 0; i < 4 && ts.offset < len(ts.src); i++ {
						if !isHexByte(ts.src[ts.offset]) {
							break
						}
						ts.advanceByte()
					}
				} else {
					ts.advanceOneRune()
				}
			}
			ts.pending = append(ts.pending, gotreesitter.Token{
				Symbol:     ts.escapeSymbol,
				Text:       string(ts.src[escStartOffset:ts.offset]),
				StartByte:  uint32(escStartOffset),
				EndByte:    uint32(ts.offset),
				StartPoint: escStart,
				EndPoint:   gotreesitter.Point{Row: ts.row, Column: ts.col},
			})
			segStartOffset = ts.offset
			segStartPoint = gotreesitter.Point{Row: ts.row, Column: ts.col}

		default:
			ts.advanceOneRune()
		}
	}

	// Unterminated string: emit remaining content and let parser recover.
	if segStartOffset < ts.offset {
		ts.pending = append(ts.pending, gotreesitter.Token{
			Symbol:     ts.stringContentSymbol,
			Text:       string(ts.src[segStartOffset:ts.offset]),
			StartByte:  uint32(segStartOffset),
			EndByte:    uint32(ts.offset),
			StartPoint: segStartPoint,
			EndPoint:   gotreesitter.Point{Row: ts.row, Column: ts.col},
		})
	}
	return openTok
}

func (ts *JSONTokenSource) numberToken() (gotreesitter.Token, bool) {
	origOffset, origRow, origCol := ts.offset, ts.row, ts.col
	startOffset := ts.offset
	startPoint := gotreesitter.Point{Row: ts.row, Column: ts.col}

	if ts.src[ts.offset] == '-' {
		ts.advanceByte()
		if ts.offset >= len(ts.src) {
			ts.offset, ts.row, ts.col = origOffset, origRow, origCol
			return gotreesitter.Token{}, false
		}
	}

	if ts.offset >= len(ts.src) || !isDigit(ts.src[ts.offset]) {
		ts.offset, ts.row, ts.col = origOffset, origRow, origCol
		return gotreesitter.Token{}, false
	}

	if ts.src[ts.offset] == '0' {
		ts.advanceByte()
	} else {
		for ts.offset < len(ts.src) && isDigit(ts.src[ts.offset]) {
			ts.advanceByte()
		}
	}

	if ts.offset < len(ts.src) && ts.src[ts.offset] == '.' {
		ts.advanceByte()
		digitStart := ts.offset
		for ts.offset < len(ts.src) && isDigit(ts.src[ts.offset]) {
			ts.advanceByte()
		}
		if digitStart == ts.offset {
			ts.offset, ts.row, ts.col = origOffset, origRow, origCol
			return gotreesitter.Token{}, false
		}
	}

	if ts.offset < len(ts.src) && (ts.src[ts.offset] == 'e' || ts.src[ts.offset] == 'E') {
		ts.advanceByte()
		if ts.offset < len(ts.src) && (ts.src[ts.offset] == '+' || ts.src[ts.offset] == '-') {
			ts.advanceByte()
		}
		digitStart := ts.offset
		for ts.offset < len(ts.src) && isDigit(ts.src[ts.offset]) {
			ts.advanceByte()
		}
		if digitStart == ts.offset {
			ts.offset, ts.row, ts.col = origOffset, origRow, origCol
			return gotreesitter.Token{}, false
		}
	}

	endPoint := gotreesitter.Point{Row: ts.row, Column: ts.col}
	return gotreesitter.Token{
		Symbol:     ts.numberSymbol,
		Text:       string(ts.src[startOffset:ts.offset]),
		StartByte:  uint32(startOffset),
		EndByte:    uint32(ts.offset),
		StartPoint: startPoint,
		EndPoint:   endPoint,
	}, true
}

func (ts *JSONTokenSource) literalToken(lit string, sym gotreesitter.Symbol) (gotreesitter.Token, bool) {
	startOffset := ts.offset
	startPoint := gotreesitter.Point{Row: ts.row, Column: ts.col}
	if !ts.matchLiteral(lit) {
		return gotreesitter.Token{}, false
	}
	endPoint := gotreesitter.Point{Row: ts.row, Column: ts.col}
	return gotreesitter.Token{
		Symbol:     sym,
		Text:       lit,
		StartByte:  uint32(startOffset),
		EndByte:    uint32(ts.offset),
		StartPoint: startPoint,
		EndPoint:   endPoint,
	}, true
}

func (ts *JSONTokenSource) matchLiteral(lit string) bool {
	if ts.offset+len(lit) > len(ts.src) {
		return false
	}
	for i := 0; i < len(lit); i++ {
		if ts.src[ts.offset+i] != lit[i] {
			return false
		}
	}
	// Ensure we are not consuming an identifier-like prefix.
	next := ts.offset + len(lit)
	if next < len(ts.src) && isIdentLike(ts.src[next]) {
		return false
	}
	for i := 0; i < len(lit); i++ {
		ts.advanceByte()
	}
	return true
}

func (ts *JSONTokenSource) commentToken() (gotreesitter.Token, bool) {
	if ts.commentSymbol == 0 || ts.offset+1 >= len(ts.src) || ts.src[ts.offset] != '/' {
		return gotreesitter.Token{}, false
	}
	startOffset := ts.offset
	startPoint := gotreesitter.Point{Row: ts.row, Column: ts.col}
	next := ts.src[ts.offset+1]

	switch next {
	case '/':
		ts.advanceByte()
		ts.advanceByte()
		for ts.offset < len(ts.src) && ts.src[ts.offset] != '\n' {
			ts.advanceOneRune()
		}
	case '*':
		ts.advanceByte()
		ts.advanceByte()
		for ts.offset < len(ts.src) {
			if ts.src[ts.offset] == '*' && ts.offset+1 < len(ts.src) && ts.src[ts.offset+1] == '/' {
				ts.advanceByte()
				ts.advanceByte()
				break
			}
			ts.advanceOneRune()
		}
	default:
		return gotreesitter.Token{}, false
	}

	return gotreesitter.Token{
		Symbol:     ts.commentSymbol,
		Text:       string(ts.src[startOffset:ts.offset]),
		StartByte:  uint32(startOffset),
		EndByte:    uint32(ts.offset),
		StartPoint: startPoint,
		EndPoint:   gotreesitter.Point{Row: ts.row, Column: ts.col},
	}, true
}

func (ts *JSONTokenSource) skipWhitespace() {
	for ts.offset < len(ts.src) {
		switch ts.src[ts.offset] {
		case ' ', '\t', '\n', '\r':
			ts.advanceByte()
		default:
			return
		}
	}
}

func (ts *JSONTokenSource) eofToken() gotreesitter.Token {
	n := uint32(len(ts.src))
	pt := gotreesitter.Point{Row: ts.row, Column: ts.col}
	return gotreesitter.Token{
		Symbol:     ts.eofSymbol,
		StartByte:  n,
		EndByte:    n,
		StartPoint: pt,
		EndPoint:   pt,
	}
}

func (ts *JSONTokenSource) advanceByte() {
	if ts.offset >= len(ts.src) {
		return
	}
	b := ts.src[ts.offset]
	ts.offset++
	if b == '\n' {
		ts.row++
		ts.col = 0
		return
	}
	ts.col++
}

func (ts *JSONTokenSource) advanceOneRune() {
	if ts.offset >= len(ts.src) {
		return
	}
	r, size := utf8.DecodeRune(ts.src[ts.offset:])
	ts.offset += size
	if r == '\n' {
		ts.row++
		ts.col = 0
	} else {
		ts.col++
	}
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isHexByte(b byte) bool {
	return (b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'f') ||
		(b >= 'A' && b <= 'F')
}

func isIdentLike(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}
