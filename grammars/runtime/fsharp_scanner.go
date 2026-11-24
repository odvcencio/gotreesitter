//go:build !grammar_subset || grammar_subset_fsharp

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// This file ports the pinned C scanner of tree-sitter-fsharp:
// common/scanner.h at b576ecf7ced0, which fsharp/src/scanner.c includes. Each
// C function has one Go function of the same name with an "fs" prefix, in the
// same order, so the two read side by side. Line references (scanner.h:N) name
// lines of that file.
//
// Two notes on the lexer:
//   - C's lexer->eof(lexer) is scanmode.AtEOF. The public Lookahead returns 0
//     both at the end and at a NUL byte; C tells them apart, so this port does too.
//   - C's lexer->lookahead is -1 for a byte that is not valid UTF-8, where
//     Lookahead returns U+FFFD. The scanner compares the lookahead only with
//     ASCII characters and ASCII ranges, so both values take the same branch
//     everywhere, and the port reads Lookahead directly.

// External token indexes for the F# grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by (scanner.h:8-43). The external index is
// stable across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner
// never hardcodes them -- see fsDefaultSymTable below.
const (
	fsTokNewline                        = 0
	fsTokIndent                         = 1
	fsTokDedent                         = 2
	fsTokThen                           = 3
	fsTokElse                           = 4
	fsTokElif                           = 5
	fsTokPreprocIf                      = 6
	fsTokPreprocElse                    = 7
	fsTokPreprocEnd                     = 8
	fsTokClass                          = 9
	fsTokBegin                          = 10
	fsTokStruct                         = 11
	fsTokInterface                      = 12
	fsTokEnd                            = 13
	fsTokAnd                            = 14
	fsTokWith                           = 15
	fsTokTripleQuoteContent             = 16
	fsTokFormatTripleQuoteContent       = 17
	fsTokBlockCommentContent            = 18
	fsTokInsideString                   = 19 // _inside_string_marker (never emitted by this scanner)
	fsTokNewlineNoAligned               = 20
	fsTokTupleMarker                    = 21 // _tuple_marker (never emitted by this scanner)
	fsTokQuotedClose                    = 22
	fsTokUntypedQuotedClose             = 23
	fsTokMultiDollarTripleQuoteStart    = 24
	fsTokMultiDollarTripleQuotedContent = 25
	fsTokMultiDollarInterpStart         = 26
	fsTokMultiDollarInterpEnd           = 27
	fsTokMultiDollarTripleQuoteEnd      = 28
	fsTokTyappOpen                      = 29
	fsTokParenIndent                    = 30
	fsTokTypeDeclNewline                = 31
	fsTokIn                             = 32
	fsTokErrorSentinel                  = 33 // _error_sentinel (never emitted by this scanner)
	fsTokenCount                        = 34
)

// fsDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped fsharp.bin assigns to each external, in fsTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var fsDefaultSymTable = [fsTokenCount]gotreesitter.Symbol{
	198, // _newline
	199, // _indent
	200, // _dedent
	67,  // "then"
	66,  // "else"
	68,  // "elif"
	195, // "#if"
	197, // "#else"
	196, // "#endif"
	112, // "class"
	8,   // "begin"
	201, // _struct_begin, displays as "struct"
	202, // _interface_begin, displays as "interface"
	9,   // "end"
	16,  // "and"
	47,  // "with"
	203, // _triple_quoted_content
	204, // _format_triple_quoted_content
	205, // block_comment_content
	206, // _inside_string_marker (never emitted by this scanner)
	207, // _newline_not_aligned
	208, // _tuple_marker (never emitted by this scanner)
	209, // _quoted_close
	210, // _untyped_quoted_close
	211, // _multi_dollar_triple_quote_start
	212, // _multi_dollar_triple_quoted_content
	213, // _multi_dollar_interp_start
	214, // _multi_dollar_interp_end
	215, // _multi_dollar_triple_quote_end
	216, // _tyapp_open
	217, // _paren_indent
	218, // _type_decl_newline
	219, // _in
	220, // _error_sentinel (never emitted by this scanner)
}

// fsExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (fsTok* order). Literal-valued
// externals use the upstream literal text ("then", "#if", and so on) since
// that is what the grammar's externals array names them.
// fsharp/src/scanner.c is a thin shim; common/scanner.h holds the scanner logic.
var fsExternalScannerSpec = ExternalScannerSpec{
	Language:       "fsharp",
	UpstreamRepo:   "https://github.com/ionide/tree-sitter-fsharp",
	UpstreamCommit: "b576ecf7ced09c0cd74a39618694512dcfc6c110",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "fsharp/src/grammar.json", SHA256: "776afdcc2429472086cdfa3e5fb51498af7bac651177ad0deeb118c0e3aada89"},
		{Path: "fsharp/src/scanner.c", SHA256: "4e53e98f5f32715abf2ecee72f4ad9cee23fe040e0930215b196aed1a5956646"},
		{Path: "common/scanner.h", SHA256: "ad0ef2f8495f86c9681426485c3016af45b356b984ed429b9e351ca8ef0f8b5b"},
	},
	Externals: []string{
		"_newline",
		"_indent",
		"_dedent",
		"then",
		"else",
		"elif",
		"#if",
		"#else",
		"#endif",
		"class",
		"begin",
		"_struct_begin",
		"_interface_begin",
		"end",
		"and",
		"with",
		"_triple_quoted_content",
		"_format_triple_quoted_content",
		"block_comment_content",
		"_inside_string_marker",
		"_newline_not_aligned",
		"_tuple_marker",
		"_quoted_close",
		"_untyped_quoted_close",
		"_multi_dollar_triple_quote_start",
		"_multi_dollar_triple_quoted_content",
		"_multi_dollar_interp_start",
		"_multi_dollar_interp_end",
		"_multi_dollar_triple_quote_end",
		"_tyapp_open",
		"_paren_indent",
		"_type_decl_newline",
		"_in",
		"_error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(fsExternalScannerSpec)
}

// fsState is C's Scanner (scanner.h:45-50). indents and parenIndents always
// have the same length: push_indent and pop_indent change both, and
// deserialize fills both.
type fsState struct {
	indents             []uint16
	parenIndents        []bool
	preprocessorIndents []uint16
	multiDollarCount    uint8
}

// FsharpExternalScanner handles indent/dedent, keywords, preprocessor
// directives, quotations, interpolated strings, and comments for F#.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type FsharpExternalScanner struct {
	symbols         [fsTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers fsharp's external symbols.
func (FsharpExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := FsharpExternalScanner{symbols: fsDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, fsExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s FsharpExternalScanner) symbolTable() *[fsTokenCount]gotreesitter.Symbol {
	if s.symbols == ([fsTokenCount]gotreesitter.Symbol{}) {
		return &fsDefaultSymTable
	}
	return &s.symbols
}

// Create is C's create (scanner.h:1184-1191).
func (FsharpExternalScanner) Create() any {
	s := &fsState{}
	fsDeserialize(s, nil)
	return s
}

// Destroy is C's destroy (scanner.h:1193-1198); the garbage collector frees the arrays.
func (FsharpExternalScanner) Destroy(payload any) {}

func (FsharpExternalScanner) Serialize(payload any, buf []byte) int {
	return fsSerialize(payload.(*fsState), buf)
}

func (FsharpExternalScanner) Deserialize(payload any, buf []byte) {
	fsDeserialize(payload.(*fsState), buf)
}

func (sc FsharpExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	var valid [fsTokenCount]bool
	if len(sc.externalToToken) > 0 {
		for externalIdx, ok := range validSymbols {
			if !ok || externalIdx >= len(sc.externalToToken) {
				continue
			}
			if tokenIdx := sc.externalToToken[externalIdx]; tokenIdx >= 0 && tokenIdx < fsTokenCount {
				valid[tokenIdx] = true
			}
		}
	} else {
		copy(valid[:], validSymbols)
	}
	return fsScan(payload.(*fsState), lexer, &valid, sc.symbolTable())
}

func fsAdvance(lexer *gotreesitter.ExternalLexer) { lexer.Advance(false) }

func fsSkip(lexer *gotreesitter.ExternalLexer) { lexer.Advance(true) }

// fsEOF is C's lexer->eof(lexer).
func fsEOF(lexer *gotreesitter.ExternalLexer) bool { return scanmode.AtEOF(lexer) }

func fsResult(lexer *gotreesitter.ExternalLexer, syms *[fsTokenCount]gotreesitter.Symbol, tok int) {
	lexer.SetResultSymbol(syms[tok])
}

// fsIsWordChar is C's is_word_char (scanner.h:56-59).
func fsIsWordChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '\''
}

// fsPushIndent is C's push_indent (scanner.h:61-65).
func fsPushIndent(s *fsState, indentLength uint16, isParenIndent bool) {
	s.indents = append(s.indents, indentLength)
	s.parenIndents = append(s.parenIndents, isParenIndent)
}

// fsPopIndent is C's pop_indent (scanner.h:67-74): it pops nothing from an
// empty stack.
func fsPopIndent(s *fsState) {
	if len(s.indents) > 0 {
		s.indents = s.indents[:len(s.indents)-1]
	}
	if len(s.parenIndents) > 0 {
		s.parenIndents = s.parenIndents[:len(s.parenIndents)-1]
	}
}

// fsPeekIndentLength is C's peek_indent_length (scanner.h:76-78). Every
// caller checks that the stack is not empty first, as in C.
func fsPeekIndentLength(s *fsState) uint16 {
	return s.indents[len(s.indents)-1]
}

// fsPeekIsParenIndent is C's peek_is_paren_indent (scanner.h:80-82).
func fsPeekIsParenIndent(s *fsState) bool {
	return len(s.parenIndents) > 0 && s.parenIndents[len(s.parenIndents)-1]
}

// fsIsTypeApplicationOpen is C's is_type_application_open (scanner.h:88-161).
func fsIsTypeApplicationOpen(lexer *gotreesitter.ExternalLexer) bool {
	angleDepth := 1
	parenDepth := 0

	for !fsEOF(lexer) && angleDepth > 0 {
		c := lexer.Lookahead()

		if c == '\n' || c == '\r' {
			return false
		}

		if fsIsWordChar(c) || c == ' ' || c == '\t' || c == ',' || c == '*' ||
			c == '.' || c == ':' || c == '#' || c == '^' || c == '/' || c == '|' ||
			c == '{' || c == '}' || c == '[' || c == ']' {
			fsAdvance(lexer)
			continue
		}

		if c == '(' {
			parenDepth++
			fsAdvance(lexer)
			continue
		}

		if c == ')' {
			if parenDepth <= 0 {
				return false
			}
			parenDepth--
			fsAdvance(lexer)
			continue
		}

		if c == '<' {
			angleDepth++
			fsAdvance(lexer)
			continue
		}

		if c == '>' {
			angleDepth--
			if angleDepth == 0 {
				return true
			}
			fsAdvance(lexer)
			continue
		}

		if c == '-' {
			fsAdvance(lexer)
			if lexer.Lookahead() == '>' {
				fsAdvance(lexer)
				continue
			}
			return false
		}

		return false
	}

	return false
}

// fsScanNChars is C's scan_n_chars (scanner.h:163-173). On a mismatch it
// returns false with the lexer already advanced past the matched characters.
func fsScanNChars(lexer *gotreesitter.ExternalLexer, ch rune, count uint8) bool {
	lexer.MarkEnd()
	for i := uint8(0); i < count; i++ {
		if lexer.Lookahead() != ch {
			return false
		}
		fsAdvance(lexer)
	}
	lexer.MarkEnd()
	return true
}

// fsScanBlockComment is C's scan_block_comment (scanner.h:175-204).
func fsScanBlockComment(lexer *gotreesitter.ExternalLexer) bool {
	lexer.MarkEnd()
	if lexer.Lookahead() != '(' {
		return false
	}

	fsAdvance(lexer)
	if lexer.Lookahead() != '*' {
		return false
	}

	fsAdvance(lexer)

	for {
		switch lexer.Lookahead() {
		case '(':
			fsScanBlockComment(lexer)
		case '*':
			fsAdvance(lexer)
			if lexer.Lookahead() == ')' {
				fsAdvance(lexer)
				return true
			}
		case 0:
			return true
		default:
			fsAdvance(lexer)
		}
	}
}

// fsIsInfixOpStart is C's is_infix_op_start (scanner.h:206-245). Several
// cases skip one character before they answer; the caller sees the lexer
// after that skip.
func fsIsInfixOpStart(lexer *gotreesitter.ExternalLexer) bool {
	switch lexer.Lookahead() {
	case '+', '-':
		fsSkip(lexer)
		return !(lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9')
	case '*', '%', '&', '=', '?', '<', '>', '^':
		return true
	case '/':
		fsSkip(lexer)
		return lexer.Lookahead() != '/'
	case '.':
		fsSkip(lexer)
		return lexer.Lookahead() != '.'
	case '!':
		fsSkip(lexer)
		return lexer.Lookahead() == '='
	case ':':
		fsSkip(lexer)
		return lexer.Lookahead() == '=' || lexer.Lookahead() == ':' ||
			lexer.Lookahead() == '?' || lexer.Lookahead() == ' ' ||
			lexer.Lookahead() == '>'
	case 'o':
		fsSkip(lexer)
		return lexer.Lookahead() == 'r'
	case '@', '$':
		fsSkip(lexer)
		return lexer.Lookahead() != '"'
	default:
		return false
	}
}

// fsIsBracketEnd is C's is_bracket_end (scanner.h:247-256).
func fsIsBracketEnd(lexer *gotreesitter.ExternalLexer) bool {
	switch lexer.Lookahead() {
	case ')', ']', '}':
		return true
	default:
		return false
	}
}

// fsMatchKeywordRest is C's match_keyword_rest (scanner.h:260-266).
func fsMatchKeywordRest(lexer *gotreesitter.ExternalLexer, rest string) bool {
	for i := 0; i < len(rest); i++ {
		if lexer.Lookahead() != rune(rest[i]) {
			return false
		}
		fsAdvance(lexer)
	}
	return !fsIsWordChar(lexer.Lookahead())
}

// fsBlockOpener is C's BlockOpener (scanner.h:269-273).
type fsBlockOpener struct {
	firstChar rune
	rest      string
	token     int
}

// fsBlockOpeners is C's block_openers (scanner.h:275-280), in the same order.
var fsBlockOpeners = [...]fsBlockOpener{
	{'c', "lass", fsTokClass},
	{'b', "egin", fsTokBegin},
	{'s', "truct", fsTokStruct},
	{'i', "nterface", fsTokInterface},
}

// fsTryDedentForPreproc is C's try_dedent_for_preproc (scanner.h:285-298).
func fsTryDedentForPreproc(s *fsState, lexer *gotreesitter.ExternalLexer, syms *[fsTokenCount]gotreesitter.Symbol) bool {
	if len(s.indents) > 0 && len(s.preprocessorIndents) > 0 {
		currentIndentLength := fsPeekIndentLength(s)
		currentPreprocLength := s.preprocessorIndents[len(s.preprocessorIndents)-1]
		if currentPreprocLength < currentIndentLength {
			fsPopIndent(s)
			fsResult(lexer, syms, fsTokDedent)
			return true
		}
	}
	return false
}

// fsScan is C's scan (scanner.h:300-1088). The C variable indent_length is a
// uint32_t that push_indent truncates to uint16_t and that the comparisons
// with a uint16_t indent widen; token_indent_level is an int16_t. The Go types
// follow C so a column above 65535 or 32767 compares as it does in C.
func fsScan(scanner *fsState, lexer *gotreesitter.ExternalLexer, valid *[fsTokenCount]bool, syms *[fsTokenCount]gotreesitter.Symbol) bool {
	if valid[fsTokErrorSentinel] {
		// At EOF during error recovery, drain the indent stack.
		if fsEOF(lexer) && len(scanner.indents) > 1 {
			fsPopIndent(scanner)
			fsResult(lexer, syms, fsTokDedent)
			return true
		}
	}

	if valid[fsTokInsideString] && !valid[fsTokErrorSentinel] {
		return false
	}

	// Type application '<' (scanner.h:333-350). The token is the '<' alone;
	// the look ahead past it does not move the token end.
	if valid[fsTokTyappOpen] && !valid[fsTokErrorSentinel] &&
		lexer.Lookahead() == '<' {
		lexer.MarkEnd()
		fsAdvance(lexer)
		lexer.MarkEnd()
		if fsIsTypeApplicationOpen(lexer) {
			fsResult(lexer, syms, fsTokTyappOpen)
			return true
		}
		return false
	}

	// Multi-dollar interpolated strings (scanner.h:352-374). A failed
	// scan_n_chars leaves the lexer advanced, and the later checks see that.
	if !valid[fsTokErrorSentinel] && scanner.multiDollarCount > 1 {
		if valid[fsTokMultiDollarInterpStart] && lexer.Lookahead() == '{' {
			if fsScanNChars(lexer, '{', scanner.multiDollarCount) {
				fsResult(lexer, syms, fsTokMultiDollarInterpStart)
				return true
			}
		}

		if valid[fsTokMultiDollarInterpEnd] && lexer.Lookahead() == '}' {
			if fsScanNChars(lexer, '}', scanner.multiDollarCount) {
				fsResult(lexer, syms, fsTokMultiDollarInterpEnd)
				return true
			}
		}

		if valid[fsTokMultiDollarTripleQuoteEnd] && lexer.Lookahead() == '"' {
			if fsScanNChars(lexer, '"', 3) {
				scanner.multiDollarCount = 0
				fsResult(lexer, syms, fsTokMultiDollarTripleQuoteEnd)
				return true
			}
		}
	}

	// Triple-quoted string content (scanner.h:376-461).
	if !valid[fsTokErrorSentinel] &&
		(valid[fsTokTripleQuoteContent] || valid[fsTokFormatTripleQuoteContent] ||
			valid[fsTokMultiDollarTripleQuotedContent]) {
		isFormat := valid[fsTokFormatTripleQuoteContent]
		isMultiDollar := valid[fsTokMultiDollarTripleQuotedContent]
		hasContent := false
		lexer.MarkEnd()
		for {
			if lexer.Lookahead() == 0 {
				break
			}
			if (isFormat || isMultiDollar) && lexer.Lookahead() == '{' {
				braceCount := uint8(1)
				if isMultiDollar {
					braceCount = scanner.multiDollarCount
				}
				lexer.MarkEnd()

				if !isMultiDollar {
					fsAdvance(lexer)
					if lexer.Lookahead() == '{' {
						fsAdvance(lexer)
						lexer.MarkEnd()
						hasContent = true
						continue
					}
					if !hasContent {
						return false
					}
					break
				}

				matchesInterpStart := true
				for i := uint8(0); i < braceCount; i++ {
					if lexer.Lookahead() != '{' {
						matchesInterpStart = false
						break
					}
					fsAdvance(lexer)
				}
				if matchesInterpStart {
					if !hasContent {
						return false
					}
					break
				}
				hasContent = true
				lexer.MarkEnd()
				continue
			}
			if lexer.Lookahead() != '"' {
				fsAdvance(lexer)
				hasContent = true
			} else {
				if isMultiDollar {
					fsAdvance(lexer)
					if lexer.Lookahead() == '"' {
						fsAdvance(lexer)
						if lexer.Lookahead() == '"' {
							break
						}
					}
					hasContent = true
					lexer.MarkEnd()
				} else {
					lexer.MarkEnd()
					fsSkip(lexer)
					if lexer.Lookahead() == '"' {
						fsSkip(lexer)
						if lexer.Lookahead() == '"' {
							fsSkip(lexer)
							break
						}
					}
				}
				hasContent = true
				lexer.MarkEnd()
			}
		}
		if isMultiDollar {
			fsResult(lexer, syms, fsTokMultiDollarTripleQuotedContent)
		} else if isFormat {
			fsResult(lexer, syms, fsTokFormatTripleQuoteContent)
		} else {
			fsResult(lexer, syms, fsTokTripleQuoteContent)
		}
		return true
	}

	// Bare type declaration lookahead (scanner.h:463-507). C has no
	// error-sentinel guard here.
	if valid[fsTokTypeDeclNewline] {
		if fsEOF(lexer) {
			fsResult(lexer, syms, fsTokTypeDeclNewline)
			return true
		}
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
			var nextIndent uint32
			for {
				if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
					nextIndent = 0
					fsSkip(lexer)
				} else if lexer.Lookahead() == ' ' {
					nextIndent++
					fsSkip(lexer)
				} else if lexer.Lookahead() == '\t' {
					nextIndent += 8
					fsSkip(lexer)
				} else {
					break
				}
			}
			if fsEOF(lexer) {
				fsResult(lexer, syms, fsTokTypeDeclNewline)
				return true
			}
			var scopeIndent uint32
			if len(scanner.indents) > 0 {
				scopeIndent = uint32(fsPeekIndentLength(scanner))
			}
			if nextIndent <= scopeIndent {
				fsResult(lexer, syms, fsTokTypeDeclNewline)
				return true
			}
			return false
		}
	}

	lexer.MarkEnd()

	foundEndOfLine := false
	foundEndOfLineSemiColon := false
	foundStartOfInfixOp := false
	foundSameLinePipeInfix := false
	foundBracketEnd := false
	foundPreprocessorEnd := false
	foundPreprocIf := false
	foundCommentStart := false
	indentLength := lexer.Column()

	for {
		if lexer.Lookahead() == '\n' {
			foundEndOfLine = true
			indentLength = 0
			fsSkip(lexer)
		} else if lexer.Lookahead() == ' ' {
			indentLength++
			fsSkip(lexer)
		} else if lexer.Lookahead() == '\r' || lexer.Lookahead() == '\f' {
			indentLength = 0
			fsSkip(lexer)
		} else if lexer.Lookahead() == '\t' {
			indentLength += 8
			fsSkip(lexer)
		} else if fsEOF(lexer) {
			foundEndOfLine = true
			indentLength = 0
			break
		} else if lexer.Lookahead() == '/' {
			fsSkip(lexer)
			if !valid[fsTokInsideString] && lexer.Lookahead() == '/' {
				if !foundPreprocIf {
					return false
				}
				for lexer.Lookahead() != '\n' && !fsEOF(lexer) {
					fsSkip(lexer)
				}
			} else {
				return false
			}
		} else if lexer.Lookahead() == '#' {
			fsAdvance(lexer)
			if lexer.Lookahead() == 'e' {
				fsAdvance(lexer)
				if lexer.Lookahead() == 'n' {
					fsAdvance(lexer)
					if lexer.Lookahead() == 'd' {
						fsAdvance(lexer)
						if lexer.Lookahead() == 'i' {
							fsAdvance(lexer)
							if lexer.Lookahead() == 'f' {
								fsAdvance(lexer)
								foundPreprocessorEnd = true
								if fsTryDedentForPreproc(scanner, lexer, syms) {
									return true
								}
								if valid[fsTokPreprocEnd] {
									if len(scanner.preprocessorIndents) > 0 {
										scanner.preprocessorIndents = scanner.preprocessorIndents[:len(scanner.preprocessorIndents)-1]
									}
									lexer.MarkEnd()
									fsResult(lexer, syms, fsTokPreprocEnd)
									return true
								}
							}
						}
					}
				} else if lexer.Lookahead() == 'l' {
					fsAdvance(lexer)
					if lexer.Lookahead() == 's' {
						fsAdvance(lexer)
						if lexer.Lookahead() == 'e' {
							fsAdvance(lexer)
							if fsTryDedentForPreproc(scanner, lexer, syms) {
								return true
							}
							if valid[fsTokPreprocElse] {
								lexer.MarkEnd()
								fsResult(lexer, syms, fsTokPreprocElse)
								return true
							}
						}
					}
				}
			} else if lexer.Lookahead() == 'i' {
				fsAdvance(lexer)
				if lexer.Lookahead() == 'f' {
					fsAdvance(lexer)
					foundPreprocIf = true
					if valid[fsTokNewline] || valid[fsTokIndent] {
						for lexer.Lookahead() != '\n' && !fsEOF(lexer) {
							fsSkip(lexer)
						}
					} else {
						if len(scanner.indents) > 0 {
							if valid[fsTokPreprocIf] {
								currentIndentLength := fsPeekIndentLength(scanner)
								scanner.preprocessorIndents = append(scanner.preprocessorIndents, currentIndentLength)
							} else {
								fsPopIndent(scanner)
								fsResult(lexer, syms, fsTokDedent)
								return true
							}
						} else {
							lexer.MarkEnd()
							fsResult(lexer, syms, fsTokPreprocIf)
							return true
						}
					}
				}
			} else {
				if foundEndOfLine && valid[fsTokNewlineNoAligned] {
					fsResult(lexer, syms, fsTokNewlineNoAligned)
					return true
				}
				return false
			}
		} else {
			break
		}
	}

	// Quotation closers @> and @@> (scanner.h:634-677).
	failedAtSignMatch := false
	if !valid[fsTokErrorSentinel] && lexer.Lookahead() == '@' {
		lexer.MarkEnd()
		fsAdvance(lexer)
		if lexer.Lookahead() == '@' {
			fsAdvance(lexer)
			if lexer.Lookahead() == '>' {
				if valid[fsTokDedent] && len(scanner.indents) > 1 {
					fsPopIndent(scanner)
					fsResult(lexer, syms, fsTokDedent)
					return true
				}
				fsAdvance(lexer)
				lexer.MarkEnd()
				fsResult(lexer, syms, fsTokUntypedQuotedClose)
				return true
			}
			failedAtSignMatch = true
			foundStartOfInfixOp = true
		} else if lexer.Lookahead() == '>' {
			if valid[fsTokDedent] && len(scanner.indents) > 1 {
				fsPopIndent(scanner)
				fsResult(lexer, syms, fsTokDedent)
				return true
			}
			fsAdvance(lexer)
			lexer.MarkEnd()
			fsResult(lexer, syms, fsTokQuotedClose)
			return true
		} else {
			failedAtSignMatch = true
			foundStartOfInfixOp = true
		}
	}

	// Multi-dollar string start (scanner.h:679-713).
	if !valid[fsTokErrorSentinel] && lexer.Lookahead() == '$' {
		lexer.MarkEnd()

		if valid[fsTokMultiDollarTripleQuoteStart] {
			dollarCount := uint8(0)
			for lexer.Lookahead() == '$' && dollarCount < 255 {
				fsAdvance(lexer)
				dollarCount++
			}
			if dollarCount > 1 && fsScanNChars(lexer, '"', 3) {
				scanner.multiDollarCount = dollarCount
				fsResult(lexer, syms, fsTokMultiDollarTripleQuoteStart)
				return true
			}
			if foundEndOfLine && len(scanner.indents) > 0 {
				currentIndentLength := fsPeekIndentLength(scanner)
				if valid[fsTokDedent] && indentLength < uint32(currentIndentLength) {
					canDedentParenIndent := !fsPeekIsParenIndent(scanner) || indentLength == 0 || fsEOF(lexer)
					if canDedentParenIndent {
						fsPopIndent(scanner)
						fsResult(lexer, syms, fsTokDedent)
						return true
					}
				}
				if valid[fsTokNewline] && indentLength == uint32(currentIndentLength) && indentLength > 0 {
					fsResult(lexer, syms, fsTokNewline)
					return true
				}
			}
			return false
		}
	}

	// Block openers class, begin, struct, interface (scanner.h:715-732).
	failedBlockOpener := false
	for i := range fsBlockOpeners {
		op := &fsBlockOpeners[i]
		if valid[op.token] && lexer.Lookahead() == op.firstChar {
			lexer.MarkEnd()
			indentLength = lexer.Column()
			fsAdvance(lexer)
			if fsMatchKeywordRest(lexer, op.rest) {
				lexer.MarkEnd()
				fsResult(lexer, syms, op.token)
				return true
			}
			failedBlockOpener = true
			break
		}
	}

	if foundEndOfLine && valid[fsTokNewlineNoAligned] &&
		!foundStartOfInfixOp && !foundPreprocessorEnd {
		fsResult(lexer, syms, fsTokNewlineNoAligned)
		return true
	}

	if !failedBlockOpener && !failedAtSignMatch {
		if valid[fsTokNewline] && lexer.Lookahead() == ';' {
			fsAdvance(lexer)
			for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\n' {
				fsAdvance(lexer)
			}
			foundEndOfLine = true
			foundEndOfLineSemiColon = true
			lexer.MarkEnd()
		}

		// Each condition reads the lookahead afresh: fsIsInfixOpStart can skip
		// a character before it answers false, and the '|' and '(' checks
		// then see the next character, as in C.
		if lexer.Lookahead() == 't' &&
			(valid[fsTokThen] || valid[fsTokDedent]) {
			fsAdvance(lexer)
			if lexer.Lookahead() == 'h' {
				fsAdvance(lexer)
				if lexer.Lookahead() == 'e' {
					fsAdvance(lexer)
					if lexer.Lookahead() == 'n' {
						fsAdvance(lexer)
						if !fsIsWordChar(lexer.Lookahead()) {
							// THEN is valid only after the dedents before it.
							if valid[fsTokThen] {
								lexer.MarkEnd()
								fsResult(lexer, syms, fsTokThen)
								return true
							}
							fsPopIndent(scanner)
							fsResult(lexer, syms, fsTokDedent)
							return true
						}
					}
				}
			}
		} else if lexer.Lookahead() == 'a' &&
			(valid[fsTokAnd] || valid[fsTokDedent]) {
			fsAdvance(lexer)
			if lexer.Lookahead() == 'n' {
				fsAdvance(lexer)
				if lexer.Lookahead() == 'd' {
					fsAdvance(lexer)
					if lexer.Lookahead() == ' ' {
						if valid[fsTokAnd] {
							lexer.MarkEnd()
							fsResult(lexer, syms, fsTokAnd)
							return true
						}
						fsPopIndent(scanner)
						fsResult(lexer, syms, fsTokDedent)
						return true
					}
				}
			}
		} else if lexer.Lookahead() == 'w' &&
			(valid[fsTokWith] || valid[fsTokDedent]) {
			fsAdvance(lexer)
			if lexer.Lookahead() == 'i' {
				fsAdvance(lexer)
				if lexer.Lookahead() == 't' {
					fsAdvance(lexer)
					if lexer.Lookahead() == 'h' {
						fsAdvance(lexer)
						if lexer.Lookahead() == ' ' {
							if valid[fsTokWith] {
								lexer.MarkEnd()
								fsResult(lexer, syms, fsTokWith)
								return true
							}
							fsPopIndent(scanner)
							fsResult(lexer, syms, fsTokDedent)
							return true
						}
					}
				}
			}
		} else if lexer.Lookahead() == 'i' && valid[fsTokIn] {
			fsAdvance(lexer)
			if lexer.Lookahead() == 'n' {
				fsAdvance(lexer)
				if !fsIsWordChar(lexer.Lookahead()) {
					// IN closes an _expression_block_for_let and replaces its
					// DEDENT, so it pops the indent of the matching INDENT.
					fsPopIndent(scanner)
					lexer.MarkEnd()
					fsResult(lexer, syms, fsTokIn)
					return true
				}
			}
		} else if lexer.Lookahead() == 'e' &&
			(valid[fsTokElse] || valid[fsTokElif] ||
				valid[fsTokEnd] || valid[fsTokDedent]) {
			fsAdvance(lexer)
			tokenIndentLevel := int16(lexer.Column())
			if lexer.Lookahead() == 'l' {
				fsAdvance(lexer)
				if lexer.Lookahead() == 's' &&
					(valid[fsTokElse] || valid[fsTokDedent]) {
					fsAdvance(lexer)
					if lexer.Lookahead() == 'e' {
						fsAdvance(lexer)
						if !fsIsWordChar(lexer.Lookahead()) {
							if valid[fsTokElse] {
								if len(scanner.indents) > 0 &&
									int32(tokenIndentLevel) < int32(fsPeekIndentLength(scanner)) {
									fsPopIndent(scanner)
									fsResult(lexer, syms, fsTokDedent)
									return true
								}
								lexer.MarkEnd()
								for {
									if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\n' ||
										lexer.Lookahead() == '\r' || lexer.Lookahead() == '\t' {
										fsAdvance(lexer)
									} else {
										break
									}
								}
								if lexer.Lookahead() == 'i' {
									fsAdvance(lexer)
									if lexer.Lookahead() == 'f' {
										fsAdvance(lexer)
										if !fsIsWordChar(lexer.Lookahead()) {
											lexer.MarkEnd()
											fsResult(lexer, syms, fsTokElif)
											return true
										}
									}
								}
								fsResult(lexer, syms, fsTokElse)
								return true
							}
							fsPopIndent(scanner)
							fsResult(lexer, syms, fsTokDedent)
							return true
						}
					}
				} else if lexer.Lookahead() == 'i' &&
					(valid[fsTokElif] || valid[fsTokDedent]) {
					fsAdvance(lexer)
					if lexer.Lookahead() == 'f' {
						fsAdvance(lexer)
						if !fsIsWordChar(lexer.Lookahead()) {
							if valid[fsTokElif] {
								if len(scanner.indents) > 0 &&
									int32(tokenIndentLevel) < int32(fsPeekIndentLength(scanner)) {
									fsPopIndent(scanner)
									fsResult(lexer, syms, fsTokDedent)
									return true
								}
								lexer.MarkEnd()
								fsResult(lexer, syms, fsTokElif)
								return true
							}
							fsPopIndent(scanner)
							fsResult(lexer, syms, fsTokDedent)
							return true
						}
					}
				}
			} else if lexer.Lookahead() == 'n' &&
				(valid[fsTokEnd] || valid[fsTokDedent]) {
				fsAdvance(lexer)
				if lexer.Lookahead() == 'd' {
					fsAdvance(lexer)
					if !fsIsWordChar(lexer.Lookahead()) {
						if valid[fsTokEnd] {
							lexer.MarkEnd()
							fsResult(lexer, syms, fsTokEnd)
							return true
						} else if valid[fsTokDedent] && len(scanner.indents) > 0 {
							fsPopIndent(scanner)
							fsResult(lexer, syms, fsTokDedent)
							return true
						}
					}
				}
			}
		} else if fsIsBracketEnd(lexer) {
			foundBracketEnd = true
		} else if fsIsInfixOpStart(lexer) {
			foundStartOfInfixOp = true
		} else if lexer.Lookahead() == '|' {
			fsSkip(lexer)
			switch lexer.Lookahead() {
			case ']', '}':
				foundBracketEnd = true
			case '>':
				foundStartOfInfixOp = true
			case ' ':
				if !foundEndOfLine {
					foundStartOfInfixOp = true
					foundSameLinePipeInfix = true
					break
				}
				if indentLength == 0 {
					indentLength = 1
				}
				if len(scanner.indents) > 0 {
					currentIndentLength := fsPeekIndentLength(scanner)
					if foundEndOfLine && indentLength == uint32(currentIndentLength) &&
						indentLength > 0 && !foundStartOfInfixOp &&
						!foundBracketEnd {
						if valid[fsTokNewline] && !foundPreprocessorEnd {
							fsResult(lexer, syms, fsTokNewline)
							return true
						}
					}
				}
			default:
				foundStartOfInfixOp = true
			}
		} else if lexer.Lookahead() == '(' {
			fsSkip(lexer)
			if lexer.Lookahead() == '*' {
				foundCommentStart = true
			}
		}
	}

	if valid[fsTokNewline] && foundEndOfLineSemiColon &&
		!foundCommentStart && !foundBracketEnd {
		fsResult(lexer, syms, fsTokNewline)
		return true
	}

	if valid[fsTokIndent] && !valid[fsTokErrorSentinel] &&
		!foundBracketEnd && !foundPreprocessorEnd &&
		!foundSameLinePipeInfix {
		fsPushIndent(scanner, uint16(indentLength), false)
		fsResult(lexer, syms, fsTokIndent)
		return true
	}

	// Like INDENT, but marked as a paren indent, so the DEDENT and NEWLINE
	// rules below are more lenient inside parenthesized expressions.
	if valid[fsTokParenIndent] && !valid[fsTokErrorSentinel] &&
		!foundBracketEnd &&
		!foundPreprocessorEnd && !foundSameLinePipeInfix {
		fsPushIndent(scanner, uint16(indentLength), true)
		fsResult(lexer, syms, fsTokParenIndent)
		return true
	}

	if len(scanner.indents) > 0 {
		isParenIndent := fsPeekIsParenIndent(scanner)
		currentIndentLength := uint32(fsPeekIndentLength(scanner))

		if foundBracketEnd && valid[fsTokDedent] {
			fsPopIndent(scanner)
			fsResult(lexer, syms, fsTokDedent)
			return true
		}

		if foundEndOfLine {
			if indentLength == currentIndentLength && indentLength > 0 &&
				!foundStartOfInfixOp && !foundBracketEnd {
				if valid[fsTokNewline] && !foundPreprocessorEnd &&
					!foundCommentStart {
					fsResult(lexer, syms, fsTokNewline)
					return true
				}
			}

			canDedentPreproc := true
			if len(scanner.preprocessorIndents) > 0 {
				currentPreprocLength := scanner.preprocessorIndents[len(scanner.preprocessorIndents)-1]
				canDedentPreproc = uint32(currentPreprocLength) < indentLength
			}

			canDedentInfixOp := true
			if foundStartOfInfixOp {
				canDedentInfixOp = indentLength+2 < currentIndentLength
			}

			// Inside a paren indent, an under-indented continuation line does
			// not dedent, except at column 0 or at the end of the input.
			canDedentParenIndent := !isParenIndent || indentLength == 0 || fsEOF(lexer)

			if indentLength < currentIndentLength && !foundBracketEnd &&
				canDedentPreproc && canDedentInfixOp &&
				(!valid[fsTokTupleMarker] || valid[fsTokErrorSentinel]) && canDedentParenIndent {
				fsPopIndent(scanner)
				fsResult(lexer, syms, fsTokDedent)
				return true
			}
		}
	}

	if valid[fsTokBlockCommentContent] && !valid[fsTokErrorSentinel] {
		lexer.MarkEnd()
		for {
			if lexer.Lookahead() == 0 {
				break
			}
			if lexer.Lookahead() != '(' && lexer.Lookahead() != '*' {
				fsAdvance(lexer)
			} else if lexer.Lookahead() == '*' {
				lexer.MarkEnd()
				fsAdvance(lexer)
				if lexer.Lookahead() == ')' {
					break
				}
			} else if fsScanBlockComment(lexer) {
				lexer.MarkEnd()
				fsAdvance(lexer)
				if lexer.Lookahead() == '*' {
					break
				}
			}
		}
		fsResult(lexer, syms, fsTokBlockCommentContent)
		return true
	}

	return false
}

// fsSerializationBufferSize is TREE_SITTER_SERIALIZATION_BUFFER_SIZE, the
// bound of C's serialize loops. The largest state the scanner writes is 545
// bytes (3 counts, 255 preprocessor indents, 255 indents, 32 paren bytes).
const fsSerializationBufferSize = 1024

// fsSerialize is C's serialize (scanner.h:1090-1138). The layout:
//
//	multi_dollar_count
//	preprocessor count n (at most 255), then n preprocessor indents, one byte each
//	indent count k (at most 255; the base indent 0 is not saved), then k indents, one byte each
//	(k+7)/8 bytes of paren flags: bit b of byte i is paren_indents[i*8+b+1]
//
// Each indent keeps its low byte, as C's (char) cast does.
func fsSerialize(s *fsState, buf []byte) int {
	limit := fsSerializationBufferSize
	if len(buf) < limit {
		limit = len(buf)
	}
	size := 0
	if size >= len(buf) {
		return size
	}

	buf[size] = s.multiDollarCount
	size++

	preprocessorCount := len(s.preprocessorIndents)
	if preprocessorCount > 255 {
		preprocessorCount = 255
	}

	if size >= len(buf) {
		return size
	}
	buf[size] = byte(preprocessorCount)
	size++

	for iter := 0; iter < preprocessorCount && size < limit; iter++ {
		buf[size] = byte(s.preprocessorIndents[iter])
		size++
	}

	indentCount := 0
	if len(s.indents) > 0 {
		indentCount = len(s.indents) - 1
	}
	if indentCount > 255 {
		indentCount = 255
	}
	if size >= len(buf) {
		return size
	}
	buf[size] = byte(indentCount)
	size++

	for iter := 1; iter <= indentCount && size < limit; iter++ {
		buf[size] = byte(s.indents[iter])
		size++
	}

	parenBytes := (indentCount + 7) / 8
	for byteIndex := 0; byteIndex < parenBytes && size < limit; byteIndex++ {
		var bits uint8
		for bit := 0; bit < 8; bit++ {
			indentIndex := byteIndex*8 + bit + 1
			if indentIndex > indentCount {
				break
			}
			if indentIndex < len(s.parenIndents) && s.parenIndents[indentIndex] {
				bits |= uint8(1) << bit
			}
		}
		buf[size] = bits
		size++
	}

	return size
}

// fsDeserialize is C's deserialize (scanner.h:1140-1182). A short state
// stops at the field it cuts: the counts are bounds, not promises, and paren
// flags past the end read as false.
func fsDeserialize(s *fsState, buf []byte) {
	s.indents = append(s.indents[:0], 0)
	s.parenIndents = append(s.parenIndents[:0], false)

	s.preprocessorIndents = s.preprocessorIndents[:0]
	s.multiDollarCount = 0
	length := len(buf)
	if length > 0 {
		size := 0
		s.multiDollarCount = buf[size]
		size++

		if size >= length {
			return
		}
		preprocessorCount := int(buf[size])
		size++

		preprocEnd := size + preprocessorCount
		if preprocEnd > length {
			preprocEnd = length
		}
		for ; size < preprocEnd; size++ {
			s.preprocessorIndents = append(s.preprocessorIndents, uint16(buf[size]))
		}

		if size >= length {
			return
		}
		indentCount := int(buf[size])
		size++
		indentBytesEnd := size + indentCount
		if indentBytesEnd > length {
			indentBytesEnd = length
		}
		for ; size < indentBytesEnd; size++ {
			s.indents = append(s.indents, uint16(buf[size]))
		}

		actualIndentCount := len(s.indents) - 1
		for indentIndex := 0; indentIndex < actualIndentCount; indentIndex++ {
			byteOffset := indentIndex / 8
			isParenIndent := false
			if size+byteOffset < length {
				bits := buf[size+byteOffset]
				isParenIndent = (bits>>(indentIndex%8))&1 != 0
			}
			s.parenIndents = append(s.parenIndents, isParenIndent)
		}
	}
}
