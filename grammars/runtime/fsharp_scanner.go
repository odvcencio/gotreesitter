//go:build !grammar_subset || grammar_subset_fsharp

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the F# grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner
// never hardcodes them -- see fsDefaultSymTable below.
const (
	fsTokNewline             = 0
	fsTokIndent              = 1
	fsTokDedent              = 2
	fsTokThen                = 3
	fsTokElse                = 4
	fsTokElif                = 5
	fsTokPreprocIf           = 6
	fsTokPreprocElse         = 7
	fsTokPreprocEnd          = 8
	fsTokClass               = 9
	fsTokStruct              = 10
	fsTokInterface           = 11
	fsTokEnd                 = 12
	fsTokAnd                 = 13
	fsTokWith                = 14
	fsTokTripleQuoteContent  = 15
	fsTokBlockCommentContent = 16
	fsTokInsideString        = 17 // _inside_string_marker (never emitted by this scanner)
	fsTokNewlineNoAligned    = 18
	fsTokTupleMarker         = 19 // _tuple_marker (never emitted by this scanner)
	fsTokErrorSentinel       = 20 // _error_sentinel (never emitted by this scanner)
	fsTokenCount             = 21
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
	185, // _newline
	186, // _indent
	187, // _dedent
	62,  // "then"
	61,  // "else"
	63,  // "elif"
	182, // "#if"
	184, // "#else"
	183, // "#endif"
	109, // "class"
	188, // _struct_begin, displays as "struct"
	189, // _interface_begin, displays as "interface"
	82,  // "end"
	12,  // "and"
	43,  // "with"
	190, // _triple_quoted_content
	191, // block_comment_content
	192, // _inside_string_marker (never emitted by this scanner)
	193, // _newline_not_aligned
	194, // _tuple_marker (never emitted by this scanner)
	195, // _error_sentinel (never emitted by this scanner)
}

// fsExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (fsTok* order). Literal-valued
// externals use the upstream literal text ("then", "#if", and so on) since
// that is what the grammar's externals array names them.
var fsExternalScannerSpec = ExternalScannerSpec{
	Language:       "fsharp",
	UpstreamRepo:   "https://github.com/ionide/tree-sitter-fsharp",
	UpstreamCommit: "5141851c278a99958469eb1736c7afc4ec738e47",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "fsharp/src/grammar.json", SHA256: "ac025426164cc8854480386318e80578518883c754f67eb9ce7b1e34b6807b94"},
		{Path: "fsharp/src/scanner.c", SHA256: "4e53e98f5f32715abf2ecee72f4ad9cee23fe040e0930215b196aed1a5956646"},
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
		"_struct_begin",
		"_interface_begin",
		"end",
		"and",
		"with",
		"_triple_quoted_content",
		"block_comment_content",
		"_inside_string_marker",
		"_newline_not_aligned",
		"_tuple_marker",
		"_error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(fsExternalScannerSpec)
}

type fsState struct {
	indents             []uint16
	preprocessorIndents []uint16
}

func fsEmitDedent(s *fsState, lexer *gotreesitter.ExternalLexer, syms *[fsTokenCount]gotreesitter.Symbol) bool {
	if len(s.indents) <= 1 {
		return false
	}
	s.indents = s.indents[:len(s.indents)-1]
	lexer.SetResultSymbol(syms[fsTokDedent])
	return true
}

// FsharpExternalScanner handles indent/dedent, keywords, preprocessor
// directives, and comments for F#.
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

func (FsharpExternalScanner) Create() any {
	return &fsState{indents: []uint16{0}}
}
func (FsharpExternalScanner) Destroy(payload any) {}

func (FsharpExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*fsState)
	size := 0
	if len(buf) == 0 {
		return 0
	}

	ppCount := len(s.preprocessorIndents)
	if ppCount > 255 {
		ppCount = 255
	}
	buf[size] = byte(ppCount)
	size++

	for i := 0; i < ppCount && size < len(buf); i++ {
		buf[size] = byte(s.preprocessorIndents[i])
		size++
	}

	for i := 1; i < len(s.indents) && size < len(buf); i++ {
		buf[size] = byte(s.indents[i])
		size++
	}

	return size
}

func (FsharpExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*fsState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	s.preprocessorIndents = s.preprocessorIndents[:0]

	if len(buf) == 0 {
		return
	}
	size := 0
	ppCount := int(buf[size])
	size++
	for ; size <= ppCount && size < len(buf); size++ {
		s.preprocessorIndents = append(s.preprocessorIndents, uint16(buf[size]))
	}
	for ; size < len(buf); size++ {
		s.indents = append(s.indents, uint16(buf[size]))
	}
}

func (sc FsharpExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*fsState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [fsTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < fsTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	// Error recovery
	if isValid(fsTokErrorSentinel) {
		if len(s.indents) > 1 {
			s.indents = s.indents[:len(s.indents)-1]
			lexer.SetResultSymbol(syms[fsTokDedent])
			return true
		}
		if len(s.preprocessorIndents) > 0 {
			s.preprocessorIndents = s.preprocessorIndents[:len(s.preprocessorIndents)-1]
			lexer.SetResultSymbol(syms[fsTokPreprocEnd])
			return true
		}
		return false
	}

	if isValid(fsTokInsideString) {
		return false
	}

	// Triple-quoted string content
	if isValid(fsTokTripleQuoteContent) {
		lexer.MarkEnd()
		for {
			if lexer.Lookahead() == 0 {
				break
			}
			if lexer.Lookahead() != '"' {
				lexer.Advance(false)
			} else {
				lexer.MarkEnd()
				lexer.Advance(true)
				if lexer.Lookahead() == '"' {
					lexer.Advance(true)
					if lexer.Lookahead() == '"' {
						lexer.Advance(true)
						break
					}
				}
				lexer.MarkEnd()
			}
		}
		lexer.SetResultSymbol(syms[fsTokTripleQuoteContent])
		return true
	}

	lexer.MarkEnd()

	foundEndOfLine := false
	foundEndOfLineSemiColon := false
	foundStartOfInfixOp := false
	foundBracketEnd := false
	foundPreprocessorEnd := false
	foundPreprocIf := false
	foundCommentStart := false
	indentLength := uint16(lexer.Column())

	// Whitespace / preprocessor scanning loop
	for {
		if lexer.Lookahead() == '\n' {
			foundEndOfLine = true
			indentLength = 0
			lexer.Advance(true)
		} else if lexer.Lookahead() == ' ' {
			indentLength++
			lexer.Advance(true)
		} else if lexer.Lookahead() == '\r' || lexer.Lookahead() == '\f' {
			indentLength = 0
			lexer.Advance(true)
		} else if lexer.Lookahead() == '\t' {
			indentLength += 8
			lexer.Advance(true)
		} else if lexer.Lookahead() == 0 { // EOF
			foundEndOfLine = true
			break
		} else if lexer.Lookahead() == '/' {
			lexer.Advance(true)
			if !isValid(fsTokInsideString) && lexer.Lookahead() == '/' {
				if !foundPreprocIf {
					return false
				}
				for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
					lexer.Advance(true)
				}
			} else {
				return false
			}
		} else if lexer.Lookahead() == '#' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'e' {
				lexer.Advance(false)
				if lexer.Lookahead() == 'n' {
					lexer.Advance(false)
					if lexer.Lookahead() == 'd' {
						lexer.Advance(false)
						if lexer.Lookahead() == 'i' {
							lexer.Advance(false)
							if lexer.Lookahead() == 'f' {
								lexer.Advance(false)
								foundPreprocessorEnd = true
								if len(s.indents) > 0 && len(s.preprocessorIndents) > 0 {
									curIndent := s.indents[len(s.indents)-1]
									curPreproc := s.preprocessorIndents[len(s.preprocessorIndents)-1]
									if curPreproc < curIndent {
										s.indents = s.indents[:len(s.indents)-1]
										lexer.SetResultSymbol(syms[fsTokDedent])
										return true
									}
								}
								if isValid(fsTokPreprocEnd) {
									if len(s.preprocessorIndents) > 0 {
										s.preprocessorIndents = s.preprocessorIndents[:len(s.preprocessorIndents)-1]
									}
									lexer.MarkEnd()
									lexer.SetResultSymbol(syms[fsTokPreprocEnd])
									return true
								}
							}
						}
					}
				} else if lexer.Lookahead() == 'l' {
					lexer.Advance(false)
					if lexer.Lookahead() == 's' {
						lexer.Advance(false)
						if lexer.Lookahead() == 'e' {
							lexer.Advance(false)
							if len(s.indents) > 0 && len(s.preprocessorIndents) > 0 {
								curIndent := s.indents[len(s.indents)-1]
								curPreproc := s.preprocessorIndents[len(s.preprocessorIndents)-1]
								if curPreproc < curIndent {
									s.indents = s.indents[:len(s.indents)-1]
									lexer.SetResultSymbol(syms[fsTokDedent])
									return true
								}
							}
							if isValid(fsTokPreprocElse) {
								lexer.MarkEnd()
								lexer.SetResultSymbol(syms[fsTokPreprocElse])
								return true
							}
						}
					}
				}
			} else if lexer.Lookahead() == 'i' {
				lexer.Advance(false)
				if lexer.Lookahead() == 'f' {
					lexer.Advance(false)
					foundPreprocIf = true
					if isValid(fsTokNewline) || isValid(fsTokIndent) {
						for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
							lexer.Advance(true)
						}
					} else {
						if len(s.indents) > 0 {
							if isValid(fsTokPreprocIf) {
								curIndent := s.indents[len(s.indents)-1]
								s.preprocessorIndents = append(s.preprocessorIndents, curIndent)
							} else {
								s.indents = s.indents[:len(s.indents)-1]
								lexer.SetResultSymbol(syms[fsTokDedent])
								return true
							}
						} else {
							lexer.MarkEnd()
							lexer.SetResultSymbol(syms[fsTokPreprocIf])
							return true
						}
					}
				}
			} else {
				if foundEndOfLine && isValid(fsTokNewlineNoAligned) {
					lexer.SetResultSymbol(syms[fsTokNewlineNoAligned])
					return true
				}
				return false
			}
		} else {
			break
		}
	}

	// Keyword: class
	if isValid(fsTokClass) && lexer.Lookahead() == 'c' {
		lexer.MarkEnd()
		indentLength = uint16(lexer.Column())
		lexer.Advance(false)
		if lexer.Lookahead() == 'l' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'a' {
				lexer.Advance(false)
				if lexer.Lookahead() == 's' {
					lexer.Advance(false)
					if lexer.Lookahead() == 's' {
						lexer.Advance(false)
						lexer.MarkEnd()
						lexer.SetResultSymbol(syms[fsTokClass])
						return true
					}
				}
			}
		}
	} else if isValid(fsTokStruct) && lexer.Lookahead() == 's' {
		lexer.MarkEnd()
		indentLength = uint16(lexer.Column())
		lexer.Advance(false)
		if lexer.Lookahead() == 't' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'r' {
				lexer.Advance(false)
				if lexer.Lookahead() == 'u' {
					lexer.Advance(false)
					if lexer.Lookahead() == 'c' {
						lexer.Advance(false)
						if lexer.Lookahead() == 't' {
							lexer.Advance(false)
							lexer.MarkEnd()
							lexer.SetResultSymbol(syms[fsTokStruct])
							return true
						}
					}
				}
			}
		}
	} else if isValid(fsTokInterface) && lexer.Lookahead() == 'i' {
		lexer.MarkEnd()
		indentLength = uint16(lexer.Column())
		lexer.Advance(false)
		if lexer.Lookahead() == 'n' {
			lexer.Advance(false)
			if lexer.Lookahead() == 't' {
				lexer.Advance(false)
				if lexer.Lookahead() == 'e' {
					lexer.Advance(false)
					if lexer.Lookahead() == 'r' {
						lexer.Advance(false)
						if lexer.Lookahead() == 'f' {
							lexer.Advance(false)
							if lexer.Lookahead() == 'a' {
								lexer.Advance(false)
								if lexer.Lookahead() == 'c' {
									lexer.Advance(false)
									if lexer.Lookahead() == 'e' {
										lexer.Advance(false)
										lexer.MarkEnd()
										lexer.SetResultSymbol(syms[fsTokInterface])
										return true
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if foundEndOfLine && isValid(fsTokNewlineNoAligned) &&
		!foundStartOfInfixOp && !foundPreprocessorEnd {
		lexer.SetResultSymbol(syms[fsTokNewlineNoAligned])
		return true
	}

	// Semicolon as newline
	if isValid(fsTokNewline) && lexer.Lookahead() == ';' {
		lexer.Advance(false)
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\n' {
			lexer.Advance(false)
		}
		foundEndOfLine = true
		foundEndOfLineSemiColon = true
		lexer.MarkEnd()
	}

	// Keywords: then, and, with, else/elif/end
	if lexer.Lookahead() == 't' && (isValid(fsTokThen) || isValid(fsTokDedent)) {
		lexer.Advance(false)
		if lexer.Lookahead() == 'h' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'e' {
				lexer.Advance(false)
				if lexer.Lookahead() == 'n' {
					lexer.Advance(false)
					if isValid(fsTokThen) {
						lexer.MarkEnd()
						lexer.SetResultSymbol(syms[fsTokThen])
						return true
					}
					return fsEmitDedent(s, lexer, syms)
				}
			}
		}
	} else if lexer.Lookahead() == 'a' && (isValid(fsTokAnd) || isValid(fsTokDedent)) {
		lexer.Advance(false)
		if lexer.Lookahead() == 'n' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'd' {
				lexer.Advance(false)
				if lexer.Lookahead() == ' ' {
					if isValid(fsTokAnd) {
						lexer.MarkEnd()
						lexer.SetResultSymbol(syms[fsTokAnd])
						return true
					}
					return fsEmitDedent(s, lexer, syms)
				}
			}
		}
	} else if lexer.Lookahead() == 'w' && (isValid(fsTokWith) || isValid(fsTokDedent)) {
		lexer.Advance(false)
		if lexer.Lookahead() == 'i' {
			lexer.Advance(false)
			if lexer.Lookahead() == 't' {
				lexer.Advance(false)
				if lexer.Lookahead() == 'h' {
					lexer.Advance(false)
					if lexer.Lookahead() == ' ' {
						if isValid(fsTokWith) {
							lexer.MarkEnd()
							lexer.SetResultSymbol(syms[fsTokWith])
							return true
						}
						return fsEmitDedent(s, lexer, syms)
					}
				}
			}
		}
	} else if lexer.Lookahead() == 'e' &&
		(isValid(fsTokElse) || isValid(fsTokElif) || isValid(fsTokEnd) || isValid(fsTokDedent)) {
		lexer.Advance(false)
		tokenIndentLevel := int16(lexer.Column())
		if lexer.Lookahead() == 'l' {
			lexer.Advance(false)
			if lexer.Lookahead() == 's' && (isValid(fsTokElse) || isValid(fsTokDedent)) {
				lexer.Advance(false)
				if lexer.Lookahead() == 'e' {
					lexer.Advance(false)
					if isValid(fsTokElse) {
						if len(s.indents) > 0 && tokenIndentLevel < int16(s.indents[len(s.indents)-1]) {
							s.indents = s.indents[:len(s.indents)-1]
							lexer.SetResultSymbol(syms[fsTokDedent])
							return true
						}
						lexer.MarkEnd()
						// Check for "else if" → elif
						for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\n' ||
							lexer.Lookahead() == '\r' || lexer.Lookahead() == '\t' {
							lexer.Advance(false)
						}
						if lexer.Lookahead() == 'i' {
							lexer.Advance(false)
							if lexer.Lookahead() == 'f' {
								lexer.Advance(false)
								if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\n' || lexer.Lookahead() == '\t' {
									lexer.MarkEnd()
									lexer.SetResultSymbol(syms[fsTokElif])
									return true
								}
							}
						}
						lexer.SetResultSymbol(syms[fsTokElse])
						return true
					}
					return fsEmitDedent(s, lexer, syms)
				}
			} else if lexer.Lookahead() == 'i' && (isValid(fsTokElif) || isValid(fsTokDedent)) {
				lexer.Advance(false)
				if lexer.Lookahead() == 'f' {
					lexer.Advance(false)
					if isValid(fsTokElif) {
						if len(s.indents) > 0 && tokenIndentLevel < int16(s.indents[len(s.indents)-1]) {
							s.indents = s.indents[:len(s.indents)-1]
							lexer.SetResultSymbol(syms[fsTokDedent])
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(syms[fsTokElif])
						return true
					}
					return fsEmitDedent(s, lexer, syms)
				}
			}
		} else if lexer.Lookahead() == 'n' && (isValid(fsTokEnd) || isValid(fsTokDedent)) {
			lexer.Advance(false)
			if lexer.Lookahead() == 'd' {
				lexer.Advance(false)
				if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
					if isValid(fsTokEnd) {
						lexer.MarkEnd()
						lexer.SetResultSymbol(syms[fsTokEnd])
						return true
					}
					return fsEmitDedent(s, lexer, syms)
				}
			}
		}
	} else if fsIsBracketEnd(lexer) {
		foundBracketEnd = true
	} else if fsIsInfixOpStart(lexer) {
		foundStartOfInfixOp = true
	} else if lexer.Lookahead() == '|' {
		lexer.Advance(true)
		switch lexer.Lookahead() {
		case ']', '}':
			foundBracketEnd = true
		case '>':
			foundStartOfInfixOp = true
		case ' ':
			if indentLength == 0 {
				indentLength = 1
			}
			if len(s.indents) > 0 {
				curIndent := s.indents[len(s.indents)-1]
				if foundEndOfLine && indentLength == curIndent &&
					indentLength > 0 && !foundStartOfInfixOp && !foundBracketEnd {
					if isValid(fsTokNewline) && !foundPreprocessorEnd {
						lexer.SetResultSymbol(syms[fsTokNewline])
						return true
					}
				}
			}
		default:
			foundStartOfInfixOp = true
		}
	} else if lexer.Lookahead() == '(' {
		lexer.Advance(true)
		if lexer.Lookahead() == '*' {
			foundCommentStart = true
		}
	}

	if isValid(fsTokNewline) && foundEndOfLineSemiColon && !foundCommentStart {
		lexer.SetResultSymbol(syms[fsTokNewline])
		return true
	}

	if isValid(fsTokIndent) && !foundBracketEnd && !foundPreprocessorEnd {
		s.indents = append(s.indents, indentLength)
		lexer.SetResultSymbol(syms[fsTokIndent])
		return true
	}

	if len(s.indents) > 0 {
		curIndent := s.indents[len(s.indents)-1]

		if foundBracketEnd && isValid(fsTokDedent) {
			s.indents = s.indents[:len(s.indents)-1]
			lexer.SetResultSymbol(syms[fsTokDedent])
			return true
		}

		if foundEndOfLine {
			if indentLength == curIndent && indentLength > 0 &&
				!foundStartOfInfixOp && !foundBracketEnd {
				if isValid(fsTokNewline) && !foundPreprocessorEnd && !foundCommentStart {
					lexer.SetResultSymbol(syms[fsTokNewline])
					return true
				}
			}

			canDedentPreproc := true
			if len(s.preprocessorIndents) > 0 {
				curPreproc := s.preprocessorIndents[len(s.preprocessorIndents)-1]
				canDedentPreproc = curPreproc < indentLength
			}

			canDedentInfixOp := true
			if foundStartOfInfixOp {
				canDedentInfixOp = indentLength+2 < curIndent
			}

			if indentLength < curIndent && !foundBracketEnd &&
				canDedentPreproc && canDedentInfixOp &&
				!isValid(fsTokTupleMarker) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.SetResultSymbol(syms[fsTokDedent])
				return true
			}
		}
	}

	// Block comment content
	if isValid(fsTokBlockCommentContent) {
		lexer.MarkEnd()
		for {
			if lexer.Lookahead() == 0 {
				break
			}
			if lexer.Lookahead() != '(' && lexer.Lookahead() != '*' {
				lexer.Advance(false)
			} else if lexer.Lookahead() == '*' {
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == ')' {
					break
				}
			} else if fsScanBlockComment(lexer) {
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '*' {
					break
				}
			}
		}
		lexer.SetResultSymbol(syms[fsTokBlockCommentContent])
		return true
	}

	return false
}

func fsIsBracketEnd(lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	return ch == ')' || ch == ']' || ch == '}'
}

func fsIsInfixOpStart(lexer *gotreesitter.ExternalLexer) bool {
	switch lexer.Lookahead() {
	case '+':
		lexer.Advance(true)
		return lexer.Lookahead() < '0' || lexer.Lookahead() > '9'
	case '-':
		lexer.Advance(true)
		return lexer.Lookahead() < '0' || lexer.Lookahead() > '9'
	case '%', '&', '=', '?', '<', '>', '^':
		return true
	case '/':
		lexer.Advance(true)
		return lexer.Lookahead() != '/'
	case '.':
		lexer.Advance(true)
		return lexer.Lookahead() != '.'
	case '!':
		lexer.Advance(true)
		return lexer.Lookahead() == '='
	case ':':
		lexer.Advance(true)
		return lexer.Lookahead() == '=' || lexer.Lookahead() == ':' ||
			lexer.Lookahead() == '?' || lexer.Lookahead() == ' ' ||
			lexer.Lookahead() == '>'
	case 'o':
		lexer.Advance(true)
		return lexer.Lookahead() == 'r'
	case '@', '$':
		lexer.Advance(true)
		return lexer.Lookahead() != '"'
	default:
		return false
	}
}

func fsScanBlockComment(lexer *gotreesitter.ExternalLexer) bool {
	lexer.MarkEnd()
	if lexer.Lookahead() != '(' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)
	for {
		switch lexer.Lookahead() {
		case '(':
			fsScanBlockComment(lexer)
		case '*':
			lexer.Advance(false)
			if lexer.Lookahead() == ')' {
				lexer.Advance(false)
				return true
			}
		case 0:
			return true
		default:
			lexer.Advance(false)
		}
	}
}
