package pythonruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes must match the generated Python grammar ExternalSymbols order.
const (
	pyTokNewline = iota
	pyTokIndent
	pyTokDedent
	pyTokStringStart
	pyTokStringContent
	pyTokEscapeInterpolation
	pyTokStringEnd
	pyTokComment
	pyTokCloseBracket
	pyTokCloseParen
	pyTokCloseBrace
	pyTokExcept
	pyTokenCount
)

// Concrete symbol IDs from the checked-in Python grammar ExternalSymbols.
const (
	pySymNewline             gotreesitter.Symbol = 101
	pySymIndent              gotreesitter.Symbol = 102
	pySymDedent              gotreesitter.Symbol = 103
	pySymStringStart         gotreesitter.Symbol = 104
	pySymStringContent       gotreesitter.Symbol = 105
	pySymEscapeInterpolation gotreesitter.Symbol = 106
	pySymStringEnd           gotreesitter.Symbol = 107
	pySymComment             gotreesitter.Symbol = 99
	pySymCloseBracket        gotreesitter.Symbol = 47
	pySymCloseParen          gotreesitter.Symbol = 8
	pySymCloseBrace          gotreesitter.Symbol = 53
	pySymExcept              gotreesitter.Symbol = 33
)

var pyDefaultSymTable = [pyTokenCount]gotreesitter.Symbol{
	pySymNewline,
	pySymIndent,
	pySymDedent,
	pySymStringStart,
	pySymStringContent,
	pySymEscapeInterpolation,
	pySymStringEnd,
	pySymComment,
	pySymCloseBracket,
	pySymCloseParen,
	pySymCloseBrace,
	pySymExcept,
}

type PythonExternalScanner struct {
	symbols         [pyTokenCount]gotreesitter.Symbol
	externalToToken []int
}

func (PythonExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	return Bind(lang, bindExternalSymbols)
}

// Bind uses the caller's positional binder and preserves its diagnostics.
func Bind(lang *gotreesitter.Language, bind func(*gotreesitter.Language, func(int, gotreesitter.Symbol)) []int) PythonExternalScanner {
	s := PythonExternalScanner{symbols: pyDefaultSymTable}
	s.externalToToken = bind(lang, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func bindExternalSymbols(lang *gotreesitter.Language, setSymbol func(int, gotreesitter.Symbol)) []int {
	if lang == nil {
		return nil
	}
	externalToToken := make([]int, len(lang.ExternalSymbols))
	for i := range externalToToken {
		externalToToken[i] = -1
	}
	bindCount := len(externalToToken)
	if pyTokenCount < bindCount {
		bindCount = pyTokenCount
	}
	for i := 0; i < bindCount; i++ {
		setSymbol(i, lang.ExternalSymbols[i])
		externalToToken[i] = i
	}
	return externalToToken
}

func (PythonExternalScanner) Create() any {
	return &State{
		Indents: []uint16{0},
	}
}

func (PythonExternalScanner) Destroy(payload any) {}

func (PythonExternalScanner) Serialize(payload any, buf []byte) int {
	return SerializeState(payload.(*State), buf)
}

func (PythonExternalScanner) Deserialize(payload any, buf []byte) {
	DeserializeState(payload.(*State), buf)
}

// SupportsIncrementalReuse enables checkpoint-authenticated subtree reuse.
// Python serializes its complete indentation and string-delimiter state, so a
// reused boundary is accepted only after the live scanner matches its start
// checkpoint and restores its end checkpoint.
func (PythonExternalScanner) SupportsIncrementalReuse() bool { return true }

func (PythonExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

// ASCII digits share every character branch, including string and comment scans.
func (PythonExternalScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= '0' && b <= '9' {
		return 1
	}
	return 0
}

// RequiresIncrementalPrefixFrontierProof prevents a changed indentation
// prefix from transferring a reduction that belongs to the old source.
func (PythonExternalScanner) RequiresIncrementalPrefixFrontierProof() bool { return true }

func (PythonExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (p PythonExternalScanner) symbolTable() *[pyTokenCount]gotreesitter.Symbol {
	if p.symbols == ([pyTokenCount]gotreesitter.Symbol{}) {
		return &pyDefaultSymTable
	}
	return &p.symbols
}

func (p PythonExternalScanner) remapValidSymbols(validSymbols []bool, semanticValid *[pyTokenCount]bool) []bool {
	if len(p.externalToToken) == 0 {
		return validSymbols
	}
	*semanticValid = [pyTokenCount]bool{}
	for externalIdx, valid := range validSymbols {
		if !valid || externalIdx >= len(p.externalToToken) {
			continue
		}
		tokenIdx := p.externalToToken[externalIdx]
		if tokenIdx >= 0 && tokenIdx < pyTokenCount {
			semanticValid[tokenIdx] = true
		}
	}
	return semanticValid[:]
}

func (p PythonExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*State)
	if len(s.Indents) == 0 {
		s.Indents = append(s.Indents, 0)
	}
	s.SyncInsideInterpolatedString()
	symbols := p.symbolTable()
	if len(p.externalToToken) > 0 {
		var semanticValid [pyTokenCount]bool
		validSymbols = p.remapValidSymbols(validSymbols, &semanticValid)
	}

	isValid := func(idx int) bool {
		return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
	}

	errorRecoveryMode := isValid(pyTokStringContent) && isValid(pyTokIndent)
	withinBrackets := isValid(pyTokCloseBrace) || isValid(pyTokCloseParen) || isValid(pyTokCloseBracket)

	advancedOnce := false
	if isValid(pyTokEscapeInterpolation) && len(s.Delimiters) > 0 &&
		(lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && !errorRecoveryMode {
		delimiter := s.Delimiters[len(s.Delimiters)-1]
		if delimiter.IsFormat() {
			lexer.MarkEnd()
			isLeftBrace := lexer.Lookahead() == '{'
			lexer.Advance(false)
			advancedOnce = true
			if (lexer.Lookahead() == '{' && isLeftBrace) || (lexer.Lookahead() == '}' && !isLeftBrace) {
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(symbols[pyTokEscapeInterpolation])
				return true
			}
			return false
		}
	}

	if isValid(pyTokStringContent) && len(s.Delimiters) > 0 && !errorRecoveryMode {
		delimiter := s.Delimiters[len(s.Delimiters)-1]
		EndChar := delimiter.EndChar()
		hasContent := advancedOnce

		for lexer.Lookahead() != 0 {
			if (advancedOnce || lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && delimiter.IsFormat() {
				lexer.MarkEnd()
				lexer.SetResultSymbol(symbols[pyTokStringContent])
				return hasContent
			}

			if lexer.Lookahead() == '\\' {
				if delimiter.IsRaw() {
					lexer.Advance(false)
					if lexer.Lookahead() == EndChar || lexer.Lookahead() == '\\' {
						lexer.Advance(false)
					}
					if lexer.Lookahead() == '\r' {
						lexer.Advance(false)
						if lexer.Lookahead() == '\n' {
							lexer.Advance(false)
						}
					} else if lexer.Lookahead() == '\n' {
						lexer.Advance(false)
					}
					continue
				}

				if delimiter.IsBytes() {
					lexer.MarkEnd()
					lexer.Advance(false)
					if lexer.Lookahead() == 'N' || lexer.Lookahead() == 'u' || lexer.Lookahead() == 'U' {
						lexer.Advance(false)
					} else {
						lexer.SetResultSymbol(symbols[pyTokStringContent])
						return hasContent
					}
				} else {
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[pyTokStringContent])
					return hasContent
				}
			} else if lexer.Lookahead() == EndChar {
				if delimiter.IsTriple() {
					lexer.MarkEnd()
					lexer.Advance(false)
					if lexer.Lookahead() == EndChar {
						lexer.Advance(false)
						if lexer.Lookahead() == EndChar {
							if hasContent {
								lexer.SetResultSymbol(symbols[pyTokStringContent])
							} else {
								lexer.Advance(false)
								lexer.MarkEnd()
								s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
								lexer.SetResultSymbol(symbols[pyTokStringEnd])
								s.InsideInterpolatedString = false
							}
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(symbols[pyTokStringContent])
						return true
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(symbols[pyTokStringContent])
					return true
				}

				if hasContent {
					lexer.SetResultSymbol(symbols[pyTokStringContent])
				} else {
					lexer.Advance(false)
					s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
					lexer.SetResultSymbol(symbols[pyTokStringEnd])
					s.InsideInterpolatedString = false
				}
				lexer.MarkEnd()
				return true
			} else if lexer.Lookahead() == '\n' && hasContent && !delimiter.IsTriple() {
				return false
			}

			lexer.Advance(false)
			hasContent = true
		}
	}

	lexer.MarkEnd()

	foundEndOfLine := false
	var indentLength uint16
	firstCommentIndentLength := int32(-1)

	for {
		switch lexer.Lookahead() {
		case '\n':
			foundEndOfLine = true
			indentLength = 0
			lexer.Advance(true)
		case ' ':
			indentLength += uint16(lexer.AdvanceSpaces(true))
		case '\r', '\f':
			indentLength = 0
			lexer.Advance(true)
		case '\t':
			indentLength += 8
			lexer.Advance(true)
		case '#':
			if isValid(pyTokIndent) || isValid(pyTokDedent) || isValid(pyTokNewline) || isValid(pyTokExcept) {
				if !foundEndOfLine {
					return false
				}
				if firstCommentIndentLength == -1 {
					firstCommentIndentLength = int32(indentLength)
				}
				lexer.AdvanceUntilNewline(true)
				lexer.Advance(true)
				indentLength = 0
				continue
			}
			goto afterIndentLoop
		case '\\':
			lexer.Advance(true)
			if lexer.Lookahead() == '\r' {
				lexer.Advance(true)
			}
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
				lexer.Advance(true)
			} else {
				return false
			}
		case 0:
			indentLength = 0
			foundEndOfLine = true
			goto afterIndentLoop
		default:
			goto afterIndentLoop
		}
	}

afterIndentLoop:
	if foundEndOfLine {
		currentIndent := s.Indents[len(s.Indents)-1]

		if isValid(pyTokIndent) && indentLength > currentIndent {
			s.Indents = append(s.Indents, indentLength)
			lexer.SetResultSymbol(symbols[pyTokIndent])
			return true
		}

		nextTokIsStringStart := lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' || lexer.Lookahead() == '`'
		if (isValid(pyTokDedent) ||
			(!isValid(pyTokNewline) && !(isValid(pyTokStringStart) && nextTokIsStringStart) && !withinBrackets)) &&
			indentLength < currentIndent &&
			!s.InsideInterpolatedString &&
			firstCommentIndentLength < int32(currentIndent) {
			s.Indents = s.Indents[:len(s.Indents)-1]
			lexer.SetResultSymbol(symbols[pyTokDedent])
			return true
		}

		if isValid(pyTokNewline) && !errorRecoveryMode {
			lexer.SetResultSymbol(symbols[pyTokNewline])
			return true
		}
	}

	if firstCommentIndentLength == -1 && isValid(pyTokStringStart) {
		var delimiter Delimiter
		hasFlags := false

		for lexer.Lookahead() != 0 {
			switch lexer.Lookahead() {
			case 'f', 'F', 't', 'T':
				delimiter |= DelimiterFormat
			case 'r', 'R':
				delimiter |= DelimiterRaw
			case 'b', 'B':
				delimiter |= DelimiterBytes
			case 'u', 'U':
				// accepted prefix, no scanner flag
			default:
				goto afterFlags
			}
			hasFlags = true
			lexer.Advance(false)
		}

	afterFlags:
		switch lexer.Lookahead() {
		case '`':
			delimiter |= DelimiterBackQuote
			lexer.Advance(false)
			lexer.MarkEnd()
		case '\'':
			delimiter |= DelimiterSingleQuote
			lexer.Advance(false)
			lexer.MarkEnd()
			if lexer.Lookahead() == '\'' {
				lexer.Advance(false)
				if lexer.Lookahead() == '\'' {
					lexer.Advance(false)
					lexer.MarkEnd()
					delimiter |= DelimiterTriple
				}
			}
		case '"':
			delimiter |= DelimiterDoubleQuote
			lexer.Advance(false)
			lexer.MarkEnd()
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.Advance(false)
					lexer.MarkEnd()
					delimiter |= DelimiterTriple
				}
			}
		}

		if delimiter.EndChar() != 0 {
			s.Delimiters = append(s.Delimiters, delimiter)
			lexer.SetResultSymbol(symbols[pyTokStringStart])
			s.InsideInterpolatedString = delimiter.IsFormat()
			return true
		}
		if hasFlags {
			return false
		}
	}

	return false
}
