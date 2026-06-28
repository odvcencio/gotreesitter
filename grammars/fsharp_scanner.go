//go:build !grammar_subset || grammar_subset_fsharp

package grammars

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the F# grammar.
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
	fsTokInsideString        = 17
	fsTokNewlineNoAligned    = 18
	fsTokTupleMarker         = 19
	fsTokErrorSentinel       = 20
)

const (
	fsSymNewline             gotreesitter.Symbol = 185
	fsSymIndent              gotreesitter.Symbol = 186
	fsSymDedent              gotreesitter.Symbol = 187
	fsSymThen                gotreesitter.Symbol = 62
	fsSymElse                gotreesitter.Symbol = 61
	fsSymElif                gotreesitter.Symbol = 63
	fsSymPreprocIf           gotreesitter.Symbol = 182
	fsSymPreprocElse         gotreesitter.Symbol = 184
	fsSymPreprocEnd          gotreesitter.Symbol = 183
	fsSymClass               gotreesitter.Symbol = 109
	fsSymStruct              gotreesitter.Symbol = 188
	fsSymInterface           gotreesitter.Symbol = 189
	fsSymEnd                 gotreesitter.Symbol = 82
	fsSymAnd                 gotreesitter.Symbol = 12
	fsSymWith                gotreesitter.Symbol = 43
	fsSymTripleQuoteContent  gotreesitter.Symbol = 190
	fsSymBlockCommentContent gotreesitter.Symbol = 191
	fsSymNewlineNoAligned    gotreesitter.Symbol = 193
)

type fsState struct {
	indents             []uint16
	preprocessorIndents []uint16
}

func fsEmitDedent(s *fsState, lexer *gotreesitter.ExternalLexer) bool {
	if len(s.indents) <= 1 {
		return false
	}
	s.indents = s.indents[:len(s.indents)-1]
	lexer.SetResultSymbol(fsSymDedent)
	return true
}

// FsharpExternalScanner handles indent/dedent, keywords, preprocessor directives, and comments for F#.
type FsharpExternalScanner struct{}

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

func (FsharpExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*fsState)

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	// Error recovery
	if isValid(fsTokErrorSentinel) {
		if len(s.indents) > 1 {
			s.indents = s.indents[:len(s.indents)-1]
			lexer.SetResultSymbol(fsSymDedent)
			return true
		}
		if len(s.preprocessorIndents) > 0 {
			s.preprocessorIndents = s.preprocessorIndents[:len(s.preprocessorIndents)-1]
			lexer.SetResultSymbol(fsSymPreprocEnd)
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
		lexer.SetResultSymbol(fsSymTripleQuoteContent)
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
	indentLength := uint16(lexer.GetColumn())

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
										lexer.SetResultSymbol(fsSymDedent)
										return true
									}
								}
								if isValid(fsTokPreprocEnd) {
									if len(s.preprocessorIndents) > 0 {
										s.preprocessorIndents = s.preprocessorIndents[:len(s.preprocessorIndents)-1]
									}
									lexer.MarkEnd()
									lexer.SetResultSymbol(fsSymPreprocEnd)
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
									lexer.SetResultSymbol(fsSymDedent)
									return true
								}
							}
							if isValid(fsTokPreprocElse) {
								lexer.MarkEnd()
								lexer.SetResultSymbol(fsSymPreprocElse)
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
								lexer.SetResultSymbol(fsSymDedent)
								return true
							}
						} else {
							lexer.MarkEnd()
							lexer.SetResultSymbol(fsSymPreprocIf)
							return true
						}
					}
				}
			} else {
				if foundEndOfLine && isValid(fsTokNewlineNoAligned) {
					lexer.SetResultSymbol(fsSymNewlineNoAligned)
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
		indentLength = uint16(lexer.GetColumn())
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
						lexer.SetResultSymbol(fsSymClass)
						return true
					}
				}
			}
		}
	} else if isValid(fsTokStruct) && lexer.Lookahead() == 's' {
		lexer.MarkEnd()
		indentLength = uint16(lexer.GetColumn())
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
							lexer.SetResultSymbol(fsSymStruct)
							return true
						}
					}
				}
			}
		}
	} else if isValid(fsTokInterface) && lexer.Lookahead() == 'i' {
		lexer.MarkEnd()
		indentLength = uint16(lexer.GetColumn())
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
										lexer.SetResultSymbol(fsSymInterface)
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
		lexer.SetResultSymbol(fsSymNewlineNoAligned)
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
						lexer.SetResultSymbol(fsSymThen)
						return true
					}
					return fsEmitDedent(s, lexer)
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
						lexer.SetResultSymbol(fsSymAnd)
						return true
					}
					return fsEmitDedent(s, lexer)
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
							lexer.SetResultSymbol(fsSymWith)
							return true
						}
						return fsEmitDedent(s, lexer)
					}
				}
			}
		}
	} else if lexer.Lookahead() == 'e' &&
		(isValid(fsTokElse) || isValid(fsTokElif) || isValid(fsTokEnd) || isValid(fsTokDedent)) {
		lexer.Advance(false)
		tokenIndentLevel := int16(lexer.GetColumn())
		if lexer.Lookahead() == 'l' {
			lexer.Advance(false)
			if lexer.Lookahead() == 's' && (isValid(fsTokElse) || isValid(fsTokDedent)) {
				lexer.Advance(false)
				if lexer.Lookahead() == 'e' {
					lexer.Advance(false)
					if isValid(fsTokElse) {
						if len(s.indents) > 0 && tokenIndentLevel < int16(s.indents[len(s.indents)-1]) {
							s.indents = s.indents[:len(s.indents)-1]
							lexer.SetResultSymbol(fsSymDedent)
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
									lexer.SetResultSymbol(fsSymElif)
									return true
								}
							}
						}
						lexer.SetResultSymbol(fsSymElse)
						return true
					}
					return fsEmitDedent(s, lexer)
				}
			} else if lexer.Lookahead() == 'i' && (isValid(fsTokElif) || isValid(fsTokDedent)) {
				lexer.Advance(false)
				if lexer.Lookahead() == 'f' {
					lexer.Advance(false)
					if isValid(fsTokElif) {
						if len(s.indents) > 0 && tokenIndentLevel < int16(s.indents[len(s.indents)-1]) {
							s.indents = s.indents[:len(s.indents)-1]
							lexer.SetResultSymbol(fsSymDedent)
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(fsSymElif)
						return true
					}
					return fsEmitDedent(s, lexer)
				}
			}
		} else if lexer.Lookahead() == 'n' && (isValid(fsTokEnd) || isValid(fsTokDedent)) {
			lexer.Advance(false)
			if lexer.Lookahead() == 'd' {
				lexer.Advance(false)
				if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
					if isValid(fsTokEnd) {
						lexer.MarkEnd()
						lexer.SetResultSymbol(fsSymEnd)
						return true
					}
					return fsEmitDedent(s, lexer)
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
						lexer.SetResultSymbol(fsSymNewline)
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
		lexer.SetResultSymbol(fsSymNewline)
		return true
	}

	if isValid(fsTokIndent) && !foundBracketEnd && !foundPreprocessorEnd {
		s.indents = append(s.indents, indentLength)
		lexer.SetResultSymbol(fsSymIndent)
		return true
	}

	if len(s.indents) > 0 {
		curIndent := s.indents[len(s.indents)-1]

		if foundBracketEnd && isValid(fsTokDedent) {
			s.indents = s.indents[:len(s.indents)-1]
			lexer.SetResultSymbol(fsSymDedent)
			return true
		}

		if foundEndOfLine {
			if indentLength == curIndent && indentLength > 0 &&
				!foundStartOfInfixOp && !foundBracketEnd {
				if isValid(fsTokNewline) && !foundPreprocessorEnd && !foundCommentStart {
					lexer.SetResultSymbol(fsSymNewline)
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
				lexer.SetResultSymbol(fsSymDedent)
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
		lexer.SetResultSymbol(fsSymBlockCommentContent)
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
