//go:build !grammar_subset || grammar_subset_starlark

package grammars

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for starlark — same layout as Python.
const (
	slTokNewline = iota
	slTokIndent
	slTokDedent
	slTokStringStart
	slTokStringContent
	slTokEscapeInterpolation
	slTokStringEnd
	slTokComment
	slTokCloseBracket
	slTokCloseParen
	slTokCloseBrace
	slTokExcept
)

// Concrete symbol IDs from the starlark grammar ExternalSymbols.
const (
	slSymNewline             gotreesitter.Symbol = 99
	slSymIndent              gotreesitter.Symbol = 100
	slSymDedent              gotreesitter.Symbol = 101
	slSymStringStart         gotreesitter.Symbol = 102
	slSymStringContent       gotreesitter.Symbol = 103
	slSymEscapeInterpolation gotreesitter.Symbol = 104
	slSymStringEnd           gotreesitter.Symbol = 105
)

// StarlarkExternalScanner handles indent/dedent and string literals for Starlark.
// Starlark is essentially Python syntax; this reuses the pythonScannerState type.
type StarlarkExternalScanner struct{}

func (StarlarkExternalScanner) Create() any {
	return &pythonScannerState{indents: []uint16{0}}
}

func (StarlarkExternalScanner) Destroy(payload any) {}

func (StarlarkExternalScanner) Serialize(payload any, buf []byte) int {
	return serializePythonScannerState(payload.(*pythonScannerState), buf)
}

func (StarlarkExternalScanner) Deserialize(payload any, buf []byte) {
	deserializePythonScannerState(payload.(*pythonScannerState), buf)
}

// SupportsIncrementalReuse remains disabled with Python's scanner: Starlark
// shares the same serialized indentation state and checkpoint restoration
// semantics. Re-enable only after Starlark's DEDENT behavior is independently
// certified.
func (StarlarkExternalScanner) SupportsIncrementalReuse() bool { return false }

func (StarlarkExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

// ASCII digits share every character branch, including string and comment scans.
func (StarlarkExternalScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= '0' && b <= '9' {
		return 1
	}
	return 0
}

func (StarlarkExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*pythonScannerState)
	if len(s.indents) == 0 {
		s.indents = append(s.indents, 0)
	}

	isValid := func(idx int) bool {
		return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
	}

	errorRecoveryMode := isValid(slTokStringContent) && isValid(slTokIndent)
	withinBrackets := isValid(slTokCloseBrace) || isValid(slTokCloseParen) || isValid(slTokCloseBracket)

	advancedOnce := false
	if isValid(slTokEscapeInterpolation) && len(s.delimiters) > 0 &&
		(lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && !errorRecoveryMode {
		delimiter := s.delimiters[len(s.delimiters)-1]
		if delimiter.isFormat() {
			lexer.MarkEnd()
			isLeftBrace := lexer.Lookahead() == '{'
			lexer.Advance(false)
			advancedOnce = true
			if (lexer.Lookahead() == '{' && isLeftBrace) || (lexer.Lookahead() == '}' && !isLeftBrace) {
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(slSymEscapeInterpolation)
				return true
			}
			return false
		}
	}

	if isValid(slTokStringContent) && len(s.delimiters) > 0 && !errorRecoveryMode {
		delimiter := s.delimiters[len(s.delimiters)-1]
		endChar := delimiter.endChar()
		hasContent := advancedOnce

		for lexer.Lookahead() != 0 {
			if (advancedOnce || lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && delimiter.isFormat() {
				lexer.MarkEnd()
				lexer.SetResultSymbol(slSymStringContent)
				return hasContent
			}

			if lexer.Lookahead() == '\\' {
				if delimiter.isRaw() {
					lexer.Advance(false)
					if lexer.Lookahead() == endChar || lexer.Lookahead() == '\\' {
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

				if delimiter.isBytes() {
					lexer.MarkEnd()
					lexer.Advance(false)
					if lexer.Lookahead() == 'N' || lexer.Lookahead() == 'u' || lexer.Lookahead() == 'U' {
						lexer.Advance(false)
					} else {
						lexer.SetResultSymbol(slSymStringContent)
						return hasContent
					}
				} else {
					lexer.MarkEnd()
					lexer.SetResultSymbol(slSymStringContent)
					return hasContent
				}
			} else if lexer.Lookahead() == endChar {
				if delimiter.isTriple() {
					lexer.MarkEnd()
					lexer.Advance(false)
					if lexer.Lookahead() == endChar {
						lexer.Advance(false)
						if lexer.Lookahead() == endChar {
							if hasContent {
								lexer.SetResultSymbol(slSymStringContent)
							} else {
								lexer.Advance(false)
								lexer.MarkEnd()
								s.delimiters = s.delimiters[:len(s.delimiters)-1]
								lexer.SetResultSymbol(slSymStringEnd)
								s.insideInterpolatedString = false
							}
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(slSymStringContent)
						return true
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(slSymStringContent)
					return true
				}

				if hasContent {
					lexer.SetResultSymbol(slSymStringContent)
				} else {
					lexer.Advance(false)
					s.delimiters = s.delimiters[:len(s.delimiters)-1]
					lexer.SetResultSymbol(slSymStringEnd)
					s.insideInterpolatedString = false
				}
				lexer.MarkEnd()
				return true
			} else if lexer.Lookahead() == '\n' && hasContent && !delimiter.isTriple() {
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
			indentLength++
			lexer.Advance(true)
		case '\r', '\f':
			indentLength = 0
			lexer.Advance(true)
		case '\t':
			indentLength += 8
			lexer.Advance(true)
		case '#':
			if isValid(slTokIndent) || isValid(slTokDedent) || isValid(slTokNewline) || isValid(slTokExcept) {
				if !foundEndOfLine {
					return false
				}
				if firstCommentIndentLength == -1 {
					firstCommentIndentLength = int32(indentLength)
				}
				for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
					lexer.Advance(true)
				}
				lexer.Advance(true)
				indentLength = 0
				continue
			}
			goto slAfterIndentLoop
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
			goto slAfterIndentLoop
		default:
			goto slAfterIndentLoop
		}
	}

slAfterIndentLoop:
	if foundEndOfLine {
		currentIndent := s.indents[len(s.indents)-1]

		if isValid(slTokIndent) && indentLength > currentIndent {
			s.indents = append(s.indents, indentLength)
			lexer.SetResultSymbol(slSymIndent)
			return true
		}

		nextTokIsStringStart := lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' || lexer.Lookahead() == '`'
		if (isValid(slTokDedent) ||
			(!isValid(slTokNewline) && !(isValid(slTokStringStart) && nextTokIsStringStart) && !withinBrackets)) &&
			indentLength < currentIndent &&
			!s.insideInterpolatedString &&
			firstCommentIndentLength < int32(currentIndent) {
			s.indents = s.indents[:len(s.indents)-1]
			lexer.SetResultSymbol(slSymDedent)
			return true
		}

		if isValid(slTokNewline) && !errorRecoveryMode {
			lexer.SetResultSymbol(slSymNewline)
			return true
		}
	}

	if firstCommentIndentLength == -1 && isValid(slTokStringStart) {
		var delimiter pyDelimiter
		hasFlags := false

		for lexer.Lookahead() != 0 {
			switch lexer.Lookahead() {
			case 'f', 'F', 't', 'T':
				delimiter |= pyDelimFormat
			case 'r', 'R':
				delimiter |= pyDelimRaw
			case 'b', 'B':
				delimiter |= pyDelimBytes
			case 'u', 'U':
				// accepted prefix, no flag
			default:
				goto slAfterFlags
			}
			hasFlags = true
			lexer.Advance(false)
		}

	slAfterFlags:
		switch lexer.Lookahead() {
		case '`':
			delimiter |= pyDelimBackQuote
			lexer.Advance(false)
			lexer.MarkEnd()
		case '\'':
			delimiter |= pyDelimSingleQuote
			lexer.Advance(false)
			lexer.MarkEnd()
			if lexer.Lookahead() == '\'' {
				lexer.Advance(false)
				if lexer.Lookahead() == '\'' {
					lexer.Advance(false)
					lexer.MarkEnd()
					delimiter |= pyDelimTriple
				}
			}
		case '"':
			delimiter |= pyDelimDoubleQuote
			lexer.Advance(false)
			lexer.MarkEnd()
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.Advance(false)
					lexer.MarkEnd()
					delimiter |= pyDelimTriple
				}
			}
		}

		if delimiter.endChar() != 0 {
			s.delimiters = append(s.delimiters, delimiter)
			lexer.SetResultSymbol(slSymStringStart)
			s.insideInterpolatedString = delimiter.isFormat()
			return true
		}
		if hasFlags {
			return false
		}
	}

	return false
}
