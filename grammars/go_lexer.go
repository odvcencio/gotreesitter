package grammars

import (
	"go/scanner"
	"go/token"
	"unicode/utf8"

	"github.com/odvcencio/mane/gotreesitter"
)

// GoTokenSource bridges Go's standard library scanner to tree-sitter's
// token format. It implements gotreesitter.TokenSource.
//
// The tree-sitter Go grammar expects tokens at a finer granularity than
// go/scanner provides. In particular:
//   - String literals are split into open-quote, content, close-quote
//   - Raw string literals similarly split with backtick delimiters
//   - Comments are emitted as tokens (symbol 94)
//   - Newline-based automatic semicolons are mapped to ";"
type GoTokenSource struct {
	src     []byte
	scanner scanner.Scanner
	fset    *token.FileSet
	lang    *gotreesitter.Language

	// Pending tokens from splitting strings/raw strings.
	pending []gotreesitter.Token
	done    bool

	// symbolMap caches the go/token -> tree-sitter symbol mapping.
	symbolMap map[token.Token]gotreesitter.Symbol

	// keywordMap maps keyword strings to their tree-sitter symbol IDs.
	keywordMap map[string]gotreesitter.Symbol
}

// NewGoTokenSource creates a token source that lexes Go source code and
// produces tree-sitter tokens compatible with the Go grammar.
func NewGoTokenSource(src []byte, lang *gotreesitter.Language) *GoTokenSource {
	ts := &GoTokenSource{
		src:  src,
		lang: lang,
		fset: token.NewFileSet(),
	}
	ts.buildMaps()
	file := ts.fset.AddFile("", ts.fset.Base(), len(src))
	ts.scanner.Init(file, src, func(_ token.Position, _ string) {
		// Ignore errors — produce error tokens instead.
	}, scanner.ScanComments)
	return ts
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

		offset := ts.fset.Position(pos).Offset
		startPoint := ts.offsetToPoint(offset)

		switch {
		case tok == token.COMMENT:
			// Comments are token symbol 94 in the Go grammar.
			endOffset := offset + len(lit)
			endPoint := ts.offsetToPoint(endOffset)
			return gotreesitter.Token{
				Symbol:     94, // sym_comment
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
			// Rune literal (symbol 89).
			endOffset := offset + len(lit)
			return gotreesitter.Token{
				Symbol:     89, // sym_rune_literal
				Text:       lit,
				StartByte:  uint32(offset),
				EndByte:    uint32(endOffset),
				StartPoint: startPoint,
				EndPoint:   ts.offsetToPoint(endOffset),
			}

		case tok == token.INT:
			endOffset := offset + len(lit)
			return gotreesitter.Token{
				Symbol:     86, // sym_int_literal
				Text:       lit,
				StartByte:  uint32(offset),
				EndByte:    uint32(endOffset),
				StartPoint: startPoint,
				EndPoint:   ts.offsetToPoint(endOffset),
			}

		case tok == token.FLOAT:
			endOffset := offset + len(lit)
			return gotreesitter.Token{
				Symbol:     87, // sym_float_literal
				Text:       lit,
				StartByte:  uint32(offset),
				EndByte:    uint32(endOffset),
				StartPoint: startPoint,
				EndPoint:   ts.offsetToPoint(endOffset),
			}

		case tok == token.IMAG:
			endOffset := offset + len(lit)
			return gotreesitter.Token{
				Symbol:     88, // sym_imaginary_literal
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
			Symbol:     8, // sym_blank_identifier
			Text:       lit,
			StartByte:  uint32(offset),
			EndByte:    uint32(endOffset),
			StartPoint: startPoint,
			EndPoint:   endPoint,
		}
	}

	// Regular identifier.
	return gotreesitter.Token{
		Symbol:     1, // sym_identifier
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
		Symbol:     82, // anon_sym_DQUOTE (open)
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
			Symbol:     83, // aux_sym_interpreted_string_literal_token1
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
		Symbol:     84, // anon_sym_DQUOTE2 (close)
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
		Symbol:     80, // anon_sym_BQUOTE
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
			Symbol:     81, // aux_sym_raw_string_literal_token1
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
		Symbol:     80, // anon_sym_BQUOTE (same symbol for close)
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
		Symbol:     0, // ts_builtin_sym_end
		StartByte:  n,
		EndByte:    n,
		StartPoint: pt,
		EndPoint:   pt,
	}
}

// offsetToPoint converts a byte offset to a row/column Point.
func (ts *GoTokenSource) offsetToPoint(offset int) gotreesitter.Point {
	row := uint32(0)
	col := uint32(0)
	for i := 0; i < offset && i < len(ts.src); {
		r, size := utf8.DecodeRune(ts.src[i:])
		if r == '\n' {
			row++
			col = 0
		} else {
			col++
		}
		i += size
	}
	return gotreesitter.Point{Row: row, Column: col}
}

// buildMaps creates the go/token to tree-sitter symbol mapping tables.
func (ts *GoTokenSource) buildMaps() {
	ts.symbolMap = map[token.Token]gotreesitter.Symbol{
		token.SEMICOLON: 3,  // anon_sym_SEMI
		token.PERIOD:    7,  // anon_sym_DOT
		token.LPAREN:    9,  // anon_sym_LPAREN
		token.RPAREN:    10, // anon_sym_RPAREN
		token.COMMA:     12, // anon_sym_COMMA
		token.ASSIGN:    13, // anon_sym_EQ
		token.LBRACK:    16, // anon_sym_LBRACK
		token.RBRACK:    17, // anon_sym_RBRACK
		token.ELLIPSIS:  18, // anon_sym_DOT_DOT_DOT
		token.MUL:       20, // anon_sym_STAR
		token.TILDE:     22, // anon_sym_TILDE
		token.LBRACE:    23, // anon_sym_LBRACE
		token.RBRACE:    24, // anon_sym_RBRACE
		token.OR:        26, // anon_sym_PIPE
		token.ARROW:     29, // anon_sym_LT_DASH
		token.DEFINE:    30, // anon_sym_COLON_EQ
		token.INC:       31, // anon_sym_PLUS_PLUS
		token.DEC:       32, // anon_sym_DASH_DASH
		token.MUL_ASSIGN: 33, // anon_sym_STAR_EQ
		token.QUO_ASSIGN: 34, // anon_sym_SLASH_EQ
		token.REM_ASSIGN: 35, // anon_sym_PERCENT_EQ
		token.SHL_ASSIGN: 36, // anon_sym_LT_LT_EQ
		token.SHR_ASSIGN: 37, // anon_sym_GT_GT_EQ
		token.AND_ASSIGN: 38, // anon_sym_AMP_EQ
		token.AND_NOT_ASSIGN: 39, // anon_sym_AMP_CARET_EQ
		token.ADD_ASSIGN: 40, // anon_sym_PLUS_EQ
		token.SUB_ASSIGN: 41, // anon_sym_DASH_EQ
		token.OR_ASSIGN:  42, // anon_sym_PIPE_EQ
		token.XOR_ASSIGN: 43, // anon_sym_CARET_EQ
		token.COLON:     44,  // anon_sym_COLON
		token.ADD:       62,  // anon_sym_PLUS
		token.SUB:       63,  // anon_sym_DASH
		token.NOT:       64,  // anon_sym_BANG
		token.XOR:       65,  // anon_sym_CARET
		token.AND:       66,  // anon_sym_AMP
		token.QUO:       67,  // anon_sym_SLASH
		token.REM:       68,  // anon_sym_PERCENT
		token.SHL:       69,  // anon_sym_LT_LT
		token.SHR:       70,  // anon_sym_GT_GT
		token.AND_NOT:   71,  // anon_sym_AMP_CARET
		token.EQL:       72,  // anon_sym_EQ_EQ
		token.NEQ:       73,  // anon_sym_BANG_EQ
		token.LSS:       74,  // anon_sym_LT
		token.LEQ:       75,  // anon_sym_LT_EQ
		token.GTR:       76,  // anon_sym_GT
		token.GEQ:       77,  // anon_sym_GT_EQ
		token.LAND:      78,  // anon_sym_AMP_AMP
		token.LOR:       79,  // anon_sym_PIPE_PIPE

		// Keywords mapped from go/token
		token.PACKAGE:     5,  // anon_sym_package
		token.IMPORT:      6,  // anon_sym_import
		token.CONST:       11, // anon_sym_const
		token.VAR:         14, // anon_sym_var
		token.FUNC:        15, // anon_sym_func
		token.TYPE:        19, // anon_sym_type
		token.STRUCT:      21, // anon_sym_struct
		token.INTERFACE:   25, // anon_sym_interface
		token.MAP:         27, // anon_sym_map
		token.CHAN:        28, // anon_sym_chan
		token.FALLTHROUGH: 45, // anon_sym_fallthrough
		token.BREAK:       46, // anon_sym_break
		token.CONTINUE:    47, // anon_sym_continue
		token.GOTO:        48, // anon_sym_goto
		token.RETURN:      49, // anon_sym_return
		token.GO:          50, // anon_sym_go
		token.DEFER:       51, // anon_sym_defer
		token.IF:          52, // anon_sym_if
		token.ELSE:        53, // anon_sym_else
		token.FOR:         54, // anon_sym_for
		token.RANGE:       55, // anon_sym_range
		token.SWITCH:      56, // anon_sym_switch
		token.CASE:        57, // anon_sym_case
		token.DEFAULT:     58, // anon_sym_default
		token.SELECT:      59, // anon_sym_select
	}

	// Keywords that go/scanner returns as IDENT but tree-sitter has special symbols for.
	ts.keywordMap = map[string]gotreesitter.Symbol{
		"new":   60, // anon_sym_new
		"make":  61, // anon_sym_make
		"nil":   90, // sym_nil
		"true":  91, // sym_true
		"false": 92, // sym_false
		"iota":  93, // sym_iota
	}
}
