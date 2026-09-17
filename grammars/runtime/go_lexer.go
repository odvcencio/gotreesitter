//go:build !grammar_subset || grammar_subset_go

package grammarruntime

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

	// symbolTable caches go/token -> tree-sitter symbol mapping in a dense
	// lookup table to avoid hash-map overhead in the hot lexing path.
	symbolTable [goTokenTableSize]gotreesitter.Symbol

	keywordNewSymbol   gotreesitter.Symbol
	keywordMakeSymbol  gotreesitter.Symbol
	keywordNilSymbol   gotreesitter.Symbol
	keywordTrueSymbol  gotreesitter.Symbol
	keywordFalseSymbol gotreesitter.Symbol
	keywordIotaSymbol  gotreesitter.Symbol

	// Common symbols used in fast paths.
	eofSymbol                         gotreesitter.Symbol
	commentSymbol                     gotreesitter.Symbol
	runeLiteralSymbol                 gotreesitter.Symbol
	intLiteralSymbol                  gotreesitter.Symbol
	floatLiteralSymbol                gotreesitter.Symbol
	imaginaryLiteralSymbol            gotreesitter.Symbol
	identifierSymbol                  gotreesitter.Symbol
	blankIdentifierSymbol             gotreesitter.Symbol
	autoSemicolonSymbol               gotreesitter.Symbol
	interpretedStringOpenQuoteSymbol  gotreesitter.Symbol
	interpretedStringCloseQuoteSymbol gotreesitter.Symbol
	interpretedStringContentSymbol    gotreesitter.Symbol
	rawStringQuoteSymbol              gotreesitter.Symbol
	rawStringContentSymbol            gotreesitter.Symbol
	escapeSequenceSymbol              gotreesitter.Symbol

	// Incremental position tracking for offsetToPoint.
	// Instead of scanning from byte 0 every call (O(n²) over a file),
	// we track the last converted offset and scan forward from there.
	lastOffset int
	lastRow    uint32
	lastCol    uint32
}

type goLexerTables struct {
	symbolTable [goTokenTableSize]gotreesitter.Symbol

	eofSymbol                         gotreesitter.Symbol
	commentSymbol                     gotreesitter.Symbol
	runeLiteralSymbol                 gotreesitter.Symbol
	intLiteralSymbol                  gotreesitter.Symbol
	floatLiteralSymbol                gotreesitter.Symbol
	imaginaryLiteralSymbol            gotreesitter.Symbol
	identifierSymbol                  gotreesitter.Symbol
	blankIdentifierSymbol             gotreesitter.Symbol
	autoSemicolonSymbol               gotreesitter.Symbol
	interpretedStringOpenQuoteSymbol  gotreesitter.Symbol
	interpretedStringCloseQuoteSymbol gotreesitter.Symbol
	interpretedStringContentSymbol    gotreesitter.Symbol
	rawStringQuoteSymbol              gotreesitter.Symbol
	rawStringContentSymbol            gotreesitter.Symbol
	escapeSequenceSymbol              gotreesitter.Symbol
	keywordNewSymbol                  gotreesitter.Symbol
	keywordMakeSymbol                 gotreesitter.Symbol
	keywordNilSymbol                  gotreesitter.Symbol
	keywordTrueSymbol                 gotreesitter.Symbol
	keywordFalseSymbol                gotreesitter.Symbol
	keywordIotaSymbol                 gotreesitter.Symbol
}

var goLexerTablesCache sync.Map // map[*gotreesitter.Language]*goLexerTables

const goTokenTableSize = 256

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

// NewGoTokenSourceOrEOF returns a token source for callers that cannot surface
// constructor errors through their own API.
func NewGoTokenSourceOrEOF(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ts, err := NewGoTokenSource(src, lang)
	if err != nil {
		return tokenSourceInitError{sourceLen: uint32(len(src))}
	}
	return ts
}

// RebuildTokenSource constructs a fresh Go token source for the given source.
func (ts *GoTokenSource) RebuildTokenSource(src []byte, lang *gotreesitter.Language) (gotreesitter.TokenSource, error) {
	if lang == nil {
		lang = ts.lang
	}
	return NewGoTokenSource(src, lang)
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

// SupportsIncrementalReuse reports that GoTokenSource preserves stable token
// boundaries across edits and supports deterministic SkipToByte* behavior.
func (ts *GoTokenSource) SupportsIncrementalReuse() bool {
	return true
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

		case tok == token.SEMICOLON:
			sym := ts.lookupSymbol(tok)
			if sym == 0 {
				continue
			}
			if lit == "\n" {
				// Auto-inserted semicolon: consume the newline byte when present
				// and stay zero-width at EOF insertion.
				//
				// The Go grammar exposes an invisible source_file_token1 alias for
				// some top-level separator positions, but using that alias for every
				// inserted newline semicolon breaks statement-list parsing inside
				// function bodies (for example real-corpus casgstatus-style blocks).
				// Emit the regular semicolon token here; the parser accepts it in
				// both top-level and statement contexts.
				if offset < 0 || offset > len(ts.src) {
					continue
				}
				endOffset := offset
				endPoint := startPoint
				if offset < len(ts.src) {
					endOffset = offset + 1
					endPoint = ts.offsetToPoint(endOffset)
				}
				return gotreesitter.Token{
					Symbol:     sym,
					Text:       lit,
					StartByte:  uint32(offset),
					EndByte:    uint32(endOffset),
					StartPoint: startPoint,
					EndPoint:   endPoint,
				}
			}
			endOffset := offset + 1 // explicit ';'
			if endOffset > len(ts.src) {
				continue
			}
			return gotreesitter.Token{
				Symbol:     sym,
				Text:       ";",
				StartByte:  uint32(offset),
				EndByte:    uint32(endOffset),
				StartPoint: startPoint,
				EndPoint:   ts.offsetToPoint(endOffset),
			}

		default:
			// Map go/token to tree-sitter symbol.
			sym := ts.lookupSymbol(tok)
			if sym == 0 {
				// Unknown token — skip.
				continue
			}
			text := lit
			if text == "" {
				text = tok.String()
			}
			endOffset := offset + len(text)
			if endOffset > len(ts.src) {
				continue
			}
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

func (ts *GoTokenSource) lookupSymbol(tok token.Token) gotreesitter.Symbol {
	i := int(tok)
	if i < 0 || i >= goTokenTableSize {
		return 0
	}
	return ts.symbolTable[i]
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

// SkipToByteWithPoint jumps to offset using the provided Point, avoiding the
// O(n) offset-to-point scan that SkipToByte performs.
func (ts *GoTokenSource) SkipToByteWithPoint(offset uint32, pt gotreesitter.Point) gotreesitter.Token {
	target := int(offset)
	if target > len(ts.src) {
		target = len(ts.src)
		offset = uint32(target)
	}
	if target > ts.scanBase && len(ts.pending) == 0 {
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
	if sym := ts.keywordSymbol(lit); sym != 0 {
		return gotreesitter.Token{
			Symbol:     sym,
			Text:       lit,
			StartByte:  uint32(offset),
			EndByte:    uint32(endOffset),
			StartPoint: startPoint,
			EndPoint:   endPoint,
		}
	}

	// The blank identifier "_" is emitted as a regular identifier token.
	// The grammar tables do not accept blank_identifier (symbol 8) in all
	// syntactic positions where "_" is valid Go (assignments, range clauses,
	// parameters), causing parse failures that shatter the AST. Since the
	// distinction is purely semantic, treating "_" as identifier is safe.

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

func (ts *GoTokenSource) keywordSymbol(lit string) gotreesitter.Symbol {
	switch lit {
	case "new":
		return ts.keywordNewSymbol
	case "make":
		return ts.keywordMakeSymbol
	case "nil":
		return ts.keywordNilSymbol
	case "true":
		return ts.keywordTrueSymbol
	case "false":
		return ts.keywordFalseSymbol
	case "iota":
		return ts.keywordIotaSymbol
	default:
		return 0
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

	terminated := len(lit) >= 2 && lit[len(lit)-1] == '"'
	contentEnd := len(lit)
	if terminated {
		contentEnd--
	}
	if contentEnd > 1 {
		ts.splitInterpretedStringContent(offset+1, lit[1:contentEnd])
	}

	if terminated {
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
	}

	return openTok
}

func (ts *GoTokenSource) splitInterpretedStringContent(contentOffset int, content string) {
	if len(content) == 0 {
		return
	}
	segmentStart := 0
	flushContent := func(end int) {
		if end <= segmentStart {
			return
		}
		startByte := contentOffset + segmentStart
		endByte := contentOffset + end
		ts.pending = append(ts.pending, gotreesitter.Token{
			Symbol:     ts.interpretedStringContentSymbol,
			Text:       content[segmentStart:end],
			StartByte:  uint32(startByte),
			EndByte:    uint32(endByte),
			StartPoint: ts.offsetToPoint(startByte),
			EndPoint:   ts.offsetToPoint(endByte),
		})
	}

	for i := 0; i < len(content); {
		if content[i] != '\\' {
			i++
			continue
		}
		flushContent(i)
		escLen := goStringEscapeLen(content[i:])
		startByte := contentOffset + i
		endByte := startByte + escLen
		ts.pending = append(ts.pending, gotreesitter.Token{
			Symbol:     ts.escapeSequenceSymbol,
			Text:       content[i : i+escLen],
			StartByte:  uint32(startByte),
			EndByte:    uint32(endByte),
			StartPoint: ts.offsetToPoint(startByte),
			EndPoint:   ts.offsetToPoint(endByte),
		})
		i += escLen
		segmentStart = i
	}
	flushContent(len(content))
}

func goStringEscapeLen(s string) int {
	if len(s) < 2 || s[0] != '\\' {
		return 1
	}
	switch s[1] {
	case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\', '\'', '"':
		return 2
	case 'x':
		if len(s) >= 4 {
			return 4
		}
	case 'u':
		if len(s) >= 6 {
			return 6
		}
	case 'U':
		if len(s) >= 10 {
			return 10
		}
	default:
		if s[1] >= '0' && s[1] <= '7' {
			if len(s) >= 4 {
				return 4
			}
		}
	}
	return 2
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
	terminated := len(lit) >= 2 && lit[len(lit)-1] == '`'
	contentEnd := offset + len(lit)
	contentEndIdx := len(lit)
	if terminated {
		contentEnd--
		contentEndIdx--
	}
	if contentEnd > contentStart {
		content := lit[1:contentEndIdx]
		ts.pending = append(ts.pending, gotreesitter.Token{
			Symbol:     ts.rawStringContentSymbol,
			Text:       content,
			StartByte:  uint32(contentStart),
			EndByte:    uint32(contentEnd),
			StartPoint: ts.offsetToPoint(contentStart),
			EndPoint:   ts.offsetToPoint(contentEnd),
		})
	}

	if terminated {
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
	}

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
		b := ts.src[i]
		if b < utf8.RuneSelf {
			if b == '\n' {
				row++
				col = 0
			} else {
				col++
			}
			i++
			continue
		}
		r, size := utf8.DecodeRune(ts.src[i:])
		if r == '\n' {
			row++
			col = 0
		} else {
			col += uint32(size)
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
	// aliasedTerminalSym resolves a named token that a grammargen-compiled
	// blob may represent as a tree-sitter `alias()` display name rather than
	// a real shiftable terminal. In that shape, TokenSymbolsByName(name)
	// comes back empty because the grammar table's actual shift/lex symbol
	// is a distinct, hidden, auto-generated terminal (e.g.
	// "_raw_string_literal_token1") — the parser relabels that hidden
	// terminal's leaf to the alias's symbol at reduce time via
	// Language.AliasSequences, purely for tree display. GoTokenSource must
	// feed the parser the hidden terminal id (what the DFA lexer would also
	// shift), not the alias id, or the parse table has no action for it.
	// Falls back to the ts2go-style direct lookup first for backward
	// compatibility with older blobs where the name IS the real terminal.
	aliasedTerminalSym := func(name string) gotreesitter.Symbol {
		if syms := ts.lang.TokenSymbolsByName(name); len(syms) > 0 {
			return syms[0]
		}
		aliasSym, ok := ts.lang.SymbolByName(name)
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("go lexer: token symbol %q not found", name)
			}
			return 0
		}
		for _, ps := range ts.lang.ProductionSignatures {
			if int(ps.ProductionID) >= len(ts.lang.AliasSequences) {
				continue
			}
			aliasSeq := ts.lang.AliasSequences[ps.ProductionID]
			for idx, rhsSym := range ps.RHS {
				if idx >= len(aliasSeq) || aliasSeq[idx] != aliasSym {
					continue
				}
				if uint32(rhsSym) < ts.lang.TokenCount {
					return rhsSym
				}
			}
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("go lexer: token symbol %q not found", name)
		}
		return 0
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
	// `blank_identifier` is a terminal in ts2go's Go grammar but a non-terminal
	// in grammargen's, so a hard lookup fails on grammargen blobs. The lexer
	// never actually emits this symbol (`_` goes out as a plain identifier),
	// so a soft lookup keeps backward compatibility with ts2go blobs without
	// breaking grammargen blobs.
	if syms := ts.lang.TokenSymbolsByName("blank_identifier"); len(syms) > 0 {
		ts.blankIdentifierSymbol = syms[0]
	}
	if autoSemiSyms := ts.lang.TokenSymbolsByName("source_file_token1"); len(autoSemiSyms) > 0 {
		ts.autoSemicolonSymbol = autoSemiSyms[0]
	} else if semicolonSyms := ts.lang.TokenSymbolsByName(";"); len(semicolonSyms) > 0 {
		// ts2go collapses the `\n | ; | \x00` auto-semi alternatives into one
		// anonymous composite `source_file_token1`. grammargen preserves the
		// three as distinct terminals — emitting plain `;` is accepted at the
		// same positions. This fallback keeps GoTokenSource usable on either
		// backend for downstream callers who opt back in via the public API.
		ts.autoSemicolonSymbol = semicolonSyms[0]
	}

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
	ts.rawStringContentSymbol = aliasedTerminalSym("raw_string_literal_content")
	ts.interpretedStringOpenQuoteSymbol = tokenSymAt("\"", 0)
	ts.interpretedStringCloseQuoteSymbol = tokenSymAt("\"", 1)
	ts.interpretedStringContentSymbol = aliasedTerminalSym("interpreted_string_literal_content")
	ts.escapeSequenceSymbol = tokenSym("escape_sequence")

	symbolMap := map[token.Token]gotreesitter.Symbol{
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
	for tok, sym := range symbolMap {
		i := int(tok)
		if i < 0 || i >= goTokenTableSize {
			continue
		}
		ts.symbolTable[i] = sym
	}

	// Keywords that go/scanner returns as IDENT but tree-sitter has special symbols for.
	ts.keywordNewSymbol = newSym
	ts.keywordMakeSymbol = makeSym
	ts.keywordNilSymbol = tokenSym("nil")
	ts.keywordTrueSymbol = tokenSym("true")
	ts.keywordFalseSymbol = tokenSym("false")
	ts.keywordIotaSymbol = tokenSym("iota")

	if firstErr != nil {
		return firstErr
	}

	tables := &goLexerTables{
		symbolTable:                       ts.symbolTable,
		eofSymbol:                         ts.eofSymbol,
		commentSymbol:                     ts.commentSymbol,
		runeLiteralSymbol:                 ts.runeLiteralSymbol,
		intLiteralSymbol:                  ts.intLiteralSymbol,
		floatLiteralSymbol:                ts.floatLiteralSymbol,
		imaginaryLiteralSymbol:            ts.imaginaryLiteralSymbol,
		identifierSymbol:                  ts.identifierSymbol,
		blankIdentifierSymbol:             ts.blankIdentifierSymbol,
		autoSemicolonSymbol:               ts.autoSemicolonSymbol,
		interpretedStringOpenQuoteSymbol:  ts.interpretedStringOpenQuoteSymbol,
		interpretedStringCloseQuoteSymbol: ts.interpretedStringCloseQuoteSymbol,
		interpretedStringContentSymbol:    ts.interpretedStringContentSymbol,
		rawStringQuoteSymbol:              ts.rawStringQuoteSymbol,
		rawStringContentSymbol:            ts.rawStringContentSymbol,
		escapeSequenceSymbol:              ts.escapeSequenceSymbol,
		keywordNewSymbol:                  ts.keywordNewSymbol,
		keywordMakeSymbol:                 ts.keywordMakeSymbol,
		keywordNilSymbol:                  ts.keywordNilSymbol,
		keywordTrueSymbol:                 ts.keywordTrueSymbol,
		keywordFalseSymbol:                ts.keywordFalseSymbol,
		keywordIotaSymbol:                 ts.keywordIotaSymbol,
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
	ts.symbolTable = tables.symbolTable
	ts.eofSymbol = tables.eofSymbol
	ts.commentSymbol = tables.commentSymbol
	ts.runeLiteralSymbol = tables.runeLiteralSymbol
	ts.intLiteralSymbol = tables.intLiteralSymbol
	ts.floatLiteralSymbol = tables.floatLiteralSymbol
	ts.imaginaryLiteralSymbol = tables.imaginaryLiteralSymbol
	ts.identifierSymbol = tables.identifierSymbol
	ts.blankIdentifierSymbol = tables.blankIdentifierSymbol
	ts.autoSemicolonSymbol = tables.autoSemicolonSymbol
	ts.interpretedStringOpenQuoteSymbol = tables.interpretedStringOpenQuoteSymbol
	ts.interpretedStringCloseQuoteSymbol = tables.interpretedStringCloseQuoteSymbol
	ts.interpretedStringContentSymbol = tables.interpretedStringContentSymbol
	ts.rawStringQuoteSymbol = tables.rawStringQuoteSymbol
	ts.rawStringContentSymbol = tables.rawStringContentSymbol
	ts.escapeSequenceSymbol = tables.escapeSequenceSymbol
	ts.keywordNewSymbol = tables.keywordNewSymbol
	ts.keywordMakeSymbol = tables.keywordMakeSymbol
	ts.keywordNilSymbol = tables.keywordNilSymbol
	ts.keywordTrueSymbol = tables.keywordTrueSymbol
	ts.keywordFalseSymbol = tables.keywordFalseSymbol
	ts.keywordIotaSymbol = tables.keywordIotaSymbol
}
