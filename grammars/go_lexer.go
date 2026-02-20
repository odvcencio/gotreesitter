package grammars

import (
	"fmt"
	"go/scanner"
	"go/token"
	"sync"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
)

// GoTokenSource bridges Go's standard library scanner to tree-sitter's
// token format. It implements gotreesitter.TokenSource.
//
// The tree-sitter Go grammar expects tokens at a finer granularity than
// go/scanner provides. In particular:
//   - String literals are split into open-quote, content, close-quote
//   - Raw string literals similarly split with backtick delimiters
//   - Comments are emitted as explicit tokens
//   - Newline-based automatic semicolons are mapped to ";"
type GoTokenSource struct {
	src      []byte
	scanner  scanner.Scanner
	fset     *token.FileSet
	file     *token.File
	baseFile *token.File
	lang     *gotreesitter.Language
	scanBase int

	// Pending tokens from splitting strings/raw strings.
	pending []gotreesitter.Token
	done    bool

	// symbolMap caches the go/token -> tree-sitter symbol mapping.
	symbolMap map[token.Token]gotreesitter.Symbol

	// keywordMap maps keyword strings to their tree-sitter symbol IDs.
	keywordMap map[string]gotreesitter.Symbol

	// Common symbols used in fast paths.
	eofSymbol                         gotreesitter.Symbol
	commentSymbol                     gotreesitter.Symbol
	runeLiteralSymbol                 gotreesitter.Symbol
	intLiteralSymbol                  gotreesitter.Symbol
	floatLiteralSymbol                gotreesitter.Symbol
	imaginaryLiteralSymbol            gotreesitter.Symbol
	identifierSymbol                  gotreesitter.Symbol
	blankIdentifierSymbol             gotreesitter.Symbol
	interpretedStringOpenQuoteSymbol  gotreesitter.Symbol
	interpretedStringCloseQuoteSymbol gotreesitter.Symbol
	interpretedStringContentSymbol    gotreesitter.Symbol
	rawStringQuoteSymbol              gotreesitter.Symbol
	rawStringContentSymbol            gotreesitter.Symbol

	// Incremental position tracking for offsetToPoint.
	// Instead of scanning from byte 0 every call (O(n²) over a file),
	// we track the last converted offset and scan forward from there.
	lastOffset int
	lastRow    uint32
	lastCol    uint32
}

type goLexerTables struct {
	symbolMap  map[token.Token]gotreesitter.Symbol
	keywordMap map[string]gotreesitter.Symbol

	eofSymbol                         gotreesitter.Symbol
	commentSymbol                     gotreesitter.Symbol
	runeLiteralSymbol                 gotreesitter.Symbol
	intLiteralSymbol                  gotreesitter.Symbol
	floatLiteralSymbol                gotreesitter.Symbol
	imaginaryLiteralSymbol            gotreesitter.Symbol
	identifierSymbol                  gotreesitter.Symbol
	blankIdentifierSymbol             gotreesitter.Symbol
	interpretedStringOpenQuoteSymbol  gotreesitter.Symbol
	interpretedStringCloseQuoteSymbol gotreesitter.Symbol
	interpretedStringContentSymbol    gotreesitter.Symbol
	rawStringQuoteSymbol              gotreesitter.Symbol
	rawStringContentSymbol            gotreesitter.Symbol
}

var goLexerTablesCache sync.Map // map[*gotreesitter.Language]*goLexerTables

// NewGoTokenSource creates a token source that lexes Go source code and
// produces tree-sitter tokens compatible with the Go grammar.
func NewGoTokenSource(src []byte, lang *gotreesitter.Language) (*GoTokenSource, error) {
	ts := &GoTokenSource{
		lang: lang,
		fset: token.NewFileSet(),
	}
	if err := ts.buildMaps(); err != nil {
		return nil, err
	}
	ts.Reset(src)
	return ts, nil
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

// NewGoTokenSourceOrEOF returns a token source for callers that cannot surface
// constructor errors through their own API.
func NewGoTokenSourceOrEOF(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ts, err := NewGoTokenSource(src, lang)
	if err != nil {
		return tokenSourceInitError{sourceLen: uint32(len(src))}
	}
	return ts
}

// Reset reinitializes this token source for a new source buffer.
func (ts *GoTokenSource) Reset(src []byte) {
	ts.src = src
	ts.pending = ts.pending[:0]
	ts.done = false
	ts.lastOffset = 0
	ts.lastRow = 0
	ts.lastCol = 0
	ts.initScanner(0)
}

func (ts *GoTokenSource) initScanner(base int) {
	if base < 0 {
		base = 0
	}
	if base > len(ts.src) {
		base = len(ts.src)
	}
	ts.scanBase = base
	if ts.fset == nil {
		ts.fset = token.NewFileSet()
	}
	size := len(ts.src) - base
	if base == 0 && ts.baseFile != nil && ts.baseFile.Size() == size {
		ts.file = ts.baseFile
	} else {
		ts.file = ts.fset.AddFile("", ts.fset.Base(), size)
		seedScannerLines(ts.file, size)
		if base == 0 {
			ts.baseFile = ts.file
		}
	}
	ts.scanner.Init(ts.file, ts.src[base:], func(_ token.Position, _ string) {
		// Ignore scanner diagnostics — parser performs error recovery.
	}, scanner.ScanComments)
}

// Next returns the next token. Returns a zero-Symbol token at EOF.
func (ts *GoTokenSource) Next() gotreesitter.Token {
	// Return pending tokens first (from split strings).
	if len(ts.pending) > 0 {
		tok := ts.pending[0]
		ts.pending = ts.pending[1:]
		return tok
	}

	if ts.done {
		return ts.eofToken()
	}

	for {
		pos, tok, lit := ts.scanner.Scan()
		if tok == token.EOF {
			ts.done = true
			return ts.eofToken()
		}

		offset := ts.scanBase + ts.tokenOffset(pos)
		startPoint := ts.offsetToPoint(offset)

		switch {
		case tok == token.COMMENT:
			endOffset := offset + len(lit)
			endPoint := ts.offsetToPoint(endOffset)
			return gotreesitter.Token{
				Symbol:     ts.commentSymbol,
				Text:       lit,
				StartByte:  uint32(offset),
				EndByte:    uint32(endOffset),
				StartPoint: startPoint,
				EndPoint:   endPoint,
			}

		case tok == token.STRING:
			// Split string literal into parts.
			return ts.splitString(offset, lit)

		case tok == token.CHAR:
			endOffset := offset + len(lit)
			return gotreesitter.Token{
				Symbol:     ts.runeLiteralSymbol,
				Text:       lit,
				StartByte:  uint32(offset),
				EndByte:    uint32(endOffset),
				StartPoint: startPoint,
				EndPoint:   ts.offsetToPoint(endOffset),
			}

		case tok == token.INT:
			endOffset := offset + len(lit)
			return gotreesitter.Token{
				Symbol:     ts.intLiteralSymbol,
				Text:       lit,
				StartByte:  uint32(offset),
				EndByte:    uint32(endOffset),
				StartPoint: startPoint,
				EndPoint:   ts.offsetToPoint(endOffset),
			}

		case tok == token.FLOAT:
			endOffset := offset + len(lit)
			return gotreesitter.Token{
				Symbol:     ts.floatLiteralSymbol,
				Text:       lit,
				StartByte:  uint32(offset),
				EndByte:    uint32(endOffset),
				StartPoint: startPoint,
				EndPoint:   ts.offsetToPoint(endOffset),
			}

		case tok == token.IMAG:
			endOffset := offset + len(lit)
			return gotreesitter.Token{
				Symbol:     ts.imaginaryLiteralSymbol,
				Text:       lit,
				StartByte:  uint32(offset),
				EndByte:    uint32(endOffset),
				StartPoint: startPoint,
				EndPoint:   ts.offsetToPoint(endOffset),
			}

		case tok == token.IDENT:
			return ts.identToken(offset, lit)

		default:
			// Map go/token to tree-sitter symbol.
			sym, ok := ts.symbolMap[tok]
			if !ok {
				// Unknown token — skip.
				continue
			}
			text := lit
			if text == "" {
				text = tok.String()
			}
			endOffset := offset + len(text)
			return gotreesitter.Token{
				Symbol:     sym,
				Text:       text,
				StartByte:  uint32(offset),
				EndByte:    uint32(endOffset),
				StartPoint: startPoint,
				EndPoint:   ts.offsetToPoint(endOffset),
			}
		}
	}
}

func (ts *GoTokenSource) tokenOffset(pos token.Pos) int {
	if ts.file != nil {
		return ts.file.Offset(pos)
	}
	if ts.fset != nil {
		return ts.fset.Position(pos).Offset
	}
	return 0
}

func seedScannerLines(file *token.File, size int) {
	if file == nil || size <= 1 {
		return
	}
	// We only read byte offsets from token.Pos; forcing a fixed, high watermark
	// line table prevents scanner AddLine growth churn on large inputs.
	_ = file.SetLines([]int{0, size - 1})
}

// SkipToByte advances until it reaches the first token at or after offset.
func (ts *GoTokenSource) SkipToByte(offset uint32) gotreesitter.Token {
	// Large forward jumps during incremental reuse are common. Re-seeding the
	// scanner near the target byte avoids token-by-token traversal of skipped
	// regions.
	const reseekThreshold = 4 * 1024
	target := int(offset)
	if target > ts.scanBase && len(ts.pending) == 0 && target-ts.scanBase >= reseekThreshold {
		pt := ts.offsetToPoint(target)
		ts.lastOffset = target
		ts.lastRow = pt.Row
		ts.lastCol = pt.Column
		ts.done = false
		ts.initScanner(target)
	}

	for {
		tok := ts.Next()
		if tok.Symbol == 0 || tok.StartByte >= offset {
			return tok
		}
	}
}

// identToken handles identifiers, keywords, and special names.
func (ts *GoTokenSource) identToken(offset int, lit string) gotreesitter.Token {
	endOffset := offset + len(lit)
	startPoint := ts.offsetToPoint(offset)
	endPoint := ts.offsetToPoint(endOffset)

	// Check for special identifiers that tree-sitter treats as keywords.
	if sym, ok := ts.keywordMap[lit]; ok {
		return gotreesitter.Token{
			Symbol:     sym,
			Text:       lit,
			StartByte:  uint32(offset),
			EndByte:    uint32(endOffset),
			StartPoint: startPoint,
			EndPoint:   endPoint,
		}
	}

	// Check for blank identifier.
	if lit == "_" {
		return gotreesitter.Token{
			Symbol:     ts.blankIdentifierSymbol,
			Text:       lit,
			StartByte:  uint32(offset),
			EndByte:    uint32(endOffset),
			StartPoint: startPoint,
			EndPoint:   endPoint,
		}
	}

	// Regular identifier.
	return gotreesitter.Token{
		Symbol:     ts.identifierSymbol,
		Text:       lit,
		StartByte:  uint32(offset),
		EndByte:    uint32(endOffset),
		StartPoint: startPoint,
		EndPoint:   endPoint,
	}
}

// splitString handles splitting a string literal into its tree-sitter
// component tokens. go/scanner gives us the whole literal, but tree-sitter
// expects: open_quote, content, close_quote (with possible escape sequences).
func (ts *GoTokenSource) splitString(offset int, lit string) gotreesitter.Token {
	if len(lit) == 0 {
		return ts.eofToken()
	}

	if lit[0] == '`' {
		// Raw string literal: `, content, `
		return ts.splitRawString(offset, lit)
	}

	// Interpreted string literal: ", content, "
	// Open quote
	openEnd := offset + 1
	openTok := gotreesitter.Token{
		Symbol:     ts.interpretedStringOpenQuoteSymbol,
		Text:       "\"",
		StartByte:  uint32(offset),
		EndByte:    uint32(openEnd),
		StartPoint: ts.offsetToPoint(offset),
		EndPoint:   ts.offsetToPoint(openEnd),
	}

	// Content (between quotes, may be empty)
	contentStart := offset + 1
	contentEnd := offset + len(lit) - 1
	if contentEnd > contentStart {
		content := lit[1 : len(lit)-1]
		ts.pending = append(ts.pending, gotreesitter.Token{
			Symbol:     ts.interpretedStringContentSymbol,
			Text:       content,
			StartByte:  uint32(contentStart),
			EndByte:    uint32(contentEnd),
			StartPoint: ts.offsetToPoint(contentStart),
			EndPoint:   ts.offsetToPoint(contentEnd),
		})
	}

	// Close quote
	closeStart := offset + len(lit) - 1
	closeEnd := offset + len(lit)
	ts.pending = append(ts.pending, gotreesitter.Token{
		Symbol:     ts.interpretedStringCloseQuoteSymbol,
		Text:       "\"",
		StartByte:  uint32(closeStart),
		EndByte:    uint32(closeEnd),
		StartPoint: ts.offsetToPoint(closeStart),
		EndPoint:   ts.offsetToPoint(closeEnd),
	})

	return openTok
}

// splitRawString handles raw string literals (`content`).
func (ts *GoTokenSource) splitRawString(offset int, lit string) gotreesitter.Token {
	// Open backtick
	openEnd := offset + 1
	openTok := gotreesitter.Token{
		Symbol:     ts.rawStringQuoteSymbol,
		Text:       "`",
		StartByte:  uint32(offset),
		EndByte:    uint32(openEnd),
		StartPoint: ts.offsetToPoint(offset),
		EndPoint:   ts.offsetToPoint(openEnd),
	}

	// Content
	contentStart := offset + 1
	contentEnd := offset + len(lit) - 1
	if contentEnd > contentStart {
		content := lit[1 : len(lit)-1]
		ts.pending = append(ts.pending, gotreesitter.Token{
			Symbol:     ts.rawStringContentSymbol,
			Text:       content,
			StartByte:  uint32(contentStart),
			EndByte:    uint32(contentEnd),
			StartPoint: ts.offsetToPoint(contentStart),
			EndPoint:   ts.offsetToPoint(contentEnd),
		})
	}

	// Close backtick
	closeStart := offset + len(lit) - 1
	closeEnd := offset + len(lit)
	ts.pending = append(ts.pending, gotreesitter.Token{
		Symbol:     ts.rawStringQuoteSymbol,
		Text:       "`",
		StartByte:  uint32(closeStart),
		EndByte:    uint32(closeEnd),
		StartPoint: ts.offsetToPoint(closeStart),
		EndPoint:   ts.offsetToPoint(closeEnd),
	})

	return openTok
}

// eofToken returns the EOF token.
func (ts *GoTokenSource) eofToken() gotreesitter.Token {
	n := uint32(len(ts.src))
	pt := ts.offsetToPoint(int(n))
	return gotreesitter.Token{
		Symbol:     ts.eofSymbol,
		StartByte:  n,
		EndByte:    n,
		StartPoint: pt,
		EndPoint:   pt,
	}
}

// offsetToPoint converts a byte offset to a row/column Point.
// Uses incremental tracking — scans forward from the last queried offset
// instead of from byte 0, turning amortized cost from O(n²) to O(n).
func (ts *GoTokenSource) offsetToPoint(offset int) gotreesitter.Point {
	if offset < ts.lastOffset {
		// Backward seek — reset to start (rare in sequential scanning).
		ts.lastOffset = 0
		ts.lastRow = 0
		ts.lastCol = 0
	}

	i := ts.lastOffset
	row := ts.lastRow
	col := ts.lastCol
	for i < offset && i < len(ts.src) {
		r, size := utf8.DecodeRune(ts.src[i:])
		if r == '\n' {
			row++
			col = 0
		} else {
			col++
		}
		i += size
	}
	ts.lastOffset = i
	ts.lastRow = row
	ts.lastCol = col
	return gotreesitter.Point{Row: row, Column: col}
}

// buildMaps creates the go/token to tree-sitter symbol mapping tables.
func (ts *GoTokenSource) buildMaps() error {
	if ts.lang == nil {
		return fmt.Errorf("go lexer: language is nil")
	}
	if cached, ok := goLexerTablesCache.Load(ts.lang); ok {
		ts.applyLexerTables(cached.(*goLexerTables))
		return nil
	}

	var firstErr error
	tokenSym := func(name string) gotreesitter.Symbol {
		syms := ts.lang.TokenSymbolsByName(name)
		if len(syms) == 0 {
			if firstErr == nil {
				firstErr = fmt.Errorf("go lexer: token symbol %q not found", name)
			}
			return 0
		}
		return syms[0]
	}
	tokenSymAt := func(name string, idx int) gotreesitter.Symbol {
		syms := ts.lang.TokenSymbolsByName(name)
		if idx < 0 || idx >= len(syms) {
			if firstErr == nil {
				firstErr = fmt.Errorf("go lexer: token symbol %q missing index %d", name, idx)
			}
			return 0
		}
		return syms[idx]
	}

	ts.eofSymbol = 0
	if eof, ok := ts.lang.SymbolByName("end"); ok {
		ts.eofSymbol = eof
	}

	identifierSyms := ts.lang.TokenSymbolsByName("identifier")
	if len(identifierSyms) == 0 {
		return fmt.Errorf("go lexer: identifier token symbol not found")
	}
	ts.identifierSymbol = identifierSyms[0]
	ts.blankIdentifierSymbol = tokenSym("blank_identifier")

	// Go's grammar aliases "new" and "make" to additional identifier token IDs.
	// If aliases are absent, fall back to the base identifier symbol.
	newSym := ts.identifierSymbol
	makeSym := ts.identifierSymbol
	if len(identifierSyms) >= 2 {
		newSym = identifierSyms[1]
		makeSym = identifierSyms[1]
	}
	if len(identifierSyms) >= 3 {
		makeSym = identifierSyms[2]
	}

	ts.commentSymbol = tokenSym("comment")
	ts.runeLiteralSymbol = tokenSym("rune_literal")
	ts.intLiteralSymbol = tokenSym("int_literal")
	ts.floatLiteralSymbol = tokenSym("float_literal")
	ts.imaginaryLiteralSymbol = tokenSym("imaginary_literal")

	ts.rawStringQuoteSymbol = tokenSym("`")
	ts.rawStringContentSymbol = tokenSym("raw_string_literal_content")
	ts.interpretedStringOpenQuoteSymbol = tokenSymAt("\"", 0)
	ts.interpretedStringCloseQuoteSymbol = tokenSymAt("\"", 1)
	ts.interpretedStringContentSymbol = tokenSym("interpreted_string_literal_content")

	ts.symbolMap = map[token.Token]gotreesitter.Symbol{
		token.SEMICOLON:      tokenSym(";"),
		token.PERIOD:         tokenSym("."),
		token.LPAREN:         tokenSym("("),
		token.RPAREN:         tokenSym(")"),
		token.COMMA:          tokenSym(","),
		token.ASSIGN:         tokenSym("="),
		token.LBRACK:         tokenSym("["),
		token.RBRACK:         tokenSym("]"),
		token.ELLIPSIS:       tokenSym("..."),
		token.MUL:            tokenSym("*"),
		token.TILDE:          tokenSym("~"),
		token.LBRACE:         tokenSym("{"),
		token.RBRACE:         tokenSym("}"),
		token.OR:             tokenSym("|"),
		token.ARROW:          tokenSym("<-"),
		token.DEFINE:         tokenSym(":="),
		token.INC:            tokenSym("++"),
		token.DEC:            tokenSym("--"),
		token.MUL_ASSIGN:     tokenSym("*="),
		token.QUO_ASSIGN:     tokenSym("/="),
		token.REM_ASSIGN:     tokenSym("%="),
		token.SHL_ASSIGN:     tokenSym("<<="),
		token.SHR_ASSIGN:     tokenSym(">>="),
		token.AND_ASSIGN:     tokenSym("&="),
		token.AND_NOT_ASSIGN: tokenSym("&^="),
		token.ADD_ASSIGN:     tokenSym("+="),
		token.SUB_ASSIGN:     tokenSym("-="),
		token.OR_ASSIGN:      tokenSym("|="),
		token.XOR_ASSIGN:     tokenSym("^="),
		token.COLON:          tokenSym(":"),
		token.ADD:            tokenSym("+"),
		token.SUB:            tokenSym("-"),
		token.NOT:            tokenSym("!"),
		token.XOR:            tokenSym("^"),
		token.AND:            tokenSym("&"),
		token.QUO:            tokenSym("/"),
		token.REM:            tokenSym("%"),
		token.SHL:            tokenSym("<<"),
		token.SHR:            tokenSym(">>"),
		token.AND_NOT:        tokenSym("&^"),
		token.EQL:            tokenSym("=="),
		token.NEQ:            tokenSym("!="),
		token.LSS:            tokenSym("<"),
		token.LEQ:            tokenSym("<="),
		token.GTR:            tokenSym(">"),
		token.GEQ:            tokenSym(">="),
		token.LAND:           tokenSym("&&"),
		token.LOR:            tokenSym("||"),

		// Keywords mapped from go/token
		token.PACKAGE:     tokenSym("package"),
		token.IMPORT:      tokenSym("import"),
		token.CONST:       tokenSym("const"),
		token.VAR:         tokenSym("var"),
		token.FUNC:        tokenSym("func"),
		token.TYPE:        tokenSym("type"),
		token.STRUCT:      tokenSym("struct"),
		token.INTERFACE:   tokenSym("interface"),
		token.MAP:         tokenSym("map"),
		token.CHAN:        tokenSym("chan"),
		token.FALLTHROUGH: tokenSym("fallthrough"),
		token.BREAK:       tokenSym("break"),
		token.CONTINUE:    tokenSym("continue"),
		token.GOTO:        tokenSym("goto"),
		token.RETURN:      tokenSym("return"),
		token.GO:          tokenSym("go"),
		token.DEFER:       tokenSym("defer"),
		token.IF:          tokenSym("if"),
		token.ELSE:        tokenSym("else"),
		token.FOR:         tokenSym("for"),
		token.RANGE:       tokenSym("range"),
		token.SWITCH:      tokenSym("switch"),
		token.CASE:        tokenSym("case"),
		token.DEFAULT:     tokenSym("default"),
		token.SELECT:      tokenSym("select"),
	}

	// Keywords that go/scanner returns as IDENT but tree-sitter has special symbols for.
	ts.keywordMap = map[string]gotreesitter.Symbol{
		"new":   newSym,
		"make":  makeSym,
		"nil":   tokenSym("nil"),
		"true":  tokenSym("true"),
		"false": tokenSym("false"),
		"iota":  tokenSym("iota"),
	}

	if firstErr != nil {
		return firstErr
	}

	tables := &goLexerTables{
		symbolMap:                         ts.symbolMap,
		keywordMap:                        ts.keywordMap,
		eofSymbol:                         ts.eofSymbol,
		commentSymbol:                     ts.commentSymbol,
		runeLiteralSymbol:                 ts.runeLiteralSymbol,
		intLiteralSymbol:                  ts.intLiteralSymbol,
		floatLiteralSymbol:                ts.floatLiteralSymbol,
		imaginaryLiteralSymbol:            ts.imaginaryLiteralSymbol,
		identifierSymbol:                  ts.identifierSymbol,
		blankIdentifierSymbol:             ts.blankIdentifierSymbol,
		interpretedStringOpenQuoteSymbol:  ts.interpretedStringOpenQuoteSymbol,
		interpretedStringCloseQuoteSymbol: ts.interpretedStringCloseQuoteSymbol,
		interpretedStringContentSymbol:    ts.interpretedStringContentSymbol,
		rawStringQuoteSymbol:              ts.rawStringQuoteSymbol,
		rawStringContentSymbol:            ts.rawStringContentSymbol,
	}

	if actual, loaded := goLexerTablesCache.LoadOrStore(ts.lang, tables); loaded {
		ts.applyLexerTables(actual.(*goLexerTables))
	}
	return nil
}

func (ts *GoTokenSource) applyLexerTables(tables *goLexerTables) {
	if tables == nil {
		return
	}
	ts.symbolMap = tables.symbolMap
	ts.keywordMap = tables.keywordMap
	ts.eofSymbol = tables.eofSymbol
	ts.commentSymbol = tables.commentSymbol
	ts.runeLiteralSymbol = tables.runeLiteralSymbol
	ts.intLiteralSymbol = tables.intLiteralSymbol
	ts.floatLiteralSymbol = tables.floatLiteralSymbol
	ts.imaginaryLiteralSymbol = tables.imaginaryLiteralSymbol
	ts.identifierSymbol = tables.identifierSymbol
	ts.blankIdentifierSymbol = tables.blankIdentifierSymbol
	ts.interpretedStringOpenQuoteSymbol = tables.interpretedStringOpenQuoteSymbol
	ts.interpretedStringCloseQuoteSymbol = tables.interpretedStringCloseQuoteSymbol
	ts.interpretedStringContentSymbol = tables.interpretedStringContentSymbol
	ts.rawStringQuoteSymbol = tables.rawStringQuoteSymbol
	ts.rawStringContentSymbol = tables.rawStringContentSymbol
}
