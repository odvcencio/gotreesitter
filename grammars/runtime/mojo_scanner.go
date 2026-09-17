//go:build !grammar_subset || grammar_subset_mojo

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Mojo grammar
// (whistlebee/tree-sitter-mojo, a tree-sitter-python derivative). The order
// matches tree-sitter-python's externals minus the trailing `except`, so the
// token indexes line up one-to-one with python for the first eleven tokens.
const (
	mojoTokNewline = iota
	mojoTokIndent
	mojoTokDedent
	mojoTokStringStart
	mojoTokStringContent
	mojoTokEscapeInterpolation
	mojoTokStringEnd
	mojoTokComment
	mojoTokCloseBracket
	mojoTokCloseParen
	mojoTokCloseBrace
	// mojoTokExcept does not exist in the mojo grammar (python's 12th
	// external). validSymbols has only 11 entries, so isValid(mojoTokExcept)
	// is always false and the shared except-handling branch is dead code.
	mojoTokExcept
)

// Concrete symbol IDs from the generated mojo grammar ExternalSymbols.
const (
	mojoSymNewline             gotreesitter.Symbol = 108
	mojoSymIndent              gotreesitter.Symbol = 109
	mojoSymDedent              gotreesitter.Symbol = 110
	mojoSymStringStart         gotreesitter.Symbol = 111
	mojoSymStringContent       gotreesitter.Symbol = 112
	mojoSymEscapeInterpolation gotreesitter.Symbol = 113
	mojoSymStringEnd           gotreesitter.Symbol = 114
)

// MojoExternalScanner is the tree-sitter-python external scanner retargeted
// at the mojo grammar's symbol IDs. Mojo's scanner.c is byte-for-byte
// python's scanner minus the `except` external, so the state shape and the
// scan logic are shared with PythonExternalScanner.
type MojoExternalScanner struct{}

func (MojoExternalScanner) Create() any {
	return &pythonScannerState{Indents: []uint16{0}}
}

func (MojoExternalScanner) Destroy(payload any) {}

func (MojoExternalScanner) Serialize(payload any, buf []byte) int {
	return serializePythonScannerState(payload.(*pythonScannerState), buf)
}

func (MojoExternalScanner) Deserialize(payload any, buf []byte) {
	deserializePythonScannerState(payload.(*pythonScannerState), buf)
}

// SupportsIncrementalReuse remains disabled with Python's scanner: Mojo shares
// the same serialized indentation state and checkpoint restoration semantics.
// Re-enable only after Mojo's DEDENT behavior is independently certified.
func (MojoExternalScanner) SupportsIncrementalReuse() bool { return false }

func (MojoExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

// ASCII digits share every character branch, including string and comment scans.
func (MojoExternalScanner) ExternalScannerASCIIEquivalenceClass(b byte) uint8 {
	if b >= '0' && b <= '9' {
		return 1
	}
	return 0
}

func (MojoExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*pythonScannerState)
	if len(s.Indents) == 0 {
		s.Indents = append(s.Indents, 0)
	}
	s.SyncInsideInterpolatedString()

	isValid := func(idx int) bool {
		return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
	}

	errorRecoveryMode := isValid(mojoTokStringContent) && isValid(mojoTokIndent)
	withinBrackets := isValid(mojoTokCloseBrace) || isValid(mojoTokCloseParen) || isValid(mojoTokCloseBracket)

	advancedOnce := false
	if isValid(mojoTokEscapeInterpolation) && len(s.Delimiters) > 0 &&
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
				lexer.SetResultSymbol(mojoSymEscapeInterpolation)
				return true
			}
			return false
		}
	}

	if isValid(mojoTokStringContent) && len(s.Delimiters) > 0 && !errorRecoveryMode {
		delimiter := s.Delimiters[len(s.Delimiters)-1]
		EndChar := delimiter.EndChar()
		hasContent := advancedOnce

		for lexer.Lookahead() != 0 {
			if (advancedOnce || lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && delimiter.IsFormat() {
				lexer.MarkEnd()
				lexer.SetResultSymbol(mojoSymStringContent)
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
						lexer.SetResultSymbol(mojoSymStringContent)
						return hasContent
					}
				} else {
					lexer.MarkEnd()
					lexer.SetResultSymbol(mojoSymStringContent)
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
								lexer.SetResultSymbol(mojoSymStringContent)
							} else {
								lexer.Advance(false)
								lexer.MarkEnd()
								s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
								lexer.SetResultSymbol(mojoSymStringEnd)
								s.InsideInterpolatedString = false
							}
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(mojoSymStringContent)
						return true
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(mojoSymStringContent)
					return true
				}

				if hasContent {
					lexer.SetResultSymbol(mojoSymStringContent)
				} else {
					lexer.Advance(false)
					s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
					lexer.SetResultSymbol(mojoSymStringEnd)
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
			indentLength++
			lexer.Advance(true)
		case '\r', '\f':
			indentLength = 0
			lexer.Advance(true)
		case '\t':
			indentLength += 8
			lexer.Advance(true)
		case '#':
			if isValid(mojoTokIndent) || isValid(mojoTokDedent) || isValid(mojoTokNewline) || isValid(mojoTokExcept) {
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

		if isValid(mojoTokIndent) && indentLength > currentIndent {
			s.Indents = append(s.Indents, indentLength)
			lexer.SetResultSymbol(mojoSymIndent)
			return true
		}

		nextTokIsStringStart := lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' || lexer.Lookahead() == '`'
		if (isValid(mojoTokDedent) ||
			(!isValid(mojoTokNewline) && !(isValid(mojoTokStringStart) && nextTokIsStringStart) && !withinBrackets)) &&
			indentLength < currentIndent &&
			!s.InsideInterpolatedString &&
			firstCommentIndentLength < int32(currentIndent) {
			s.Indents = s.Indents[:len(s.Indents)-1]
			lexer.SetResultSymbol(mojoSymDedent)
			return true
		}

		if isValid(mojoTokNewline) && !errorRecoveryMode {
			lexer.SetResultSymbol(mojoSymNewline)
			return true
		}
	}

	if firstCommentIndentLength == -1 && isValid(mojoTokStringStart) {
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

		if delimiter.EndChar() != 0 {
			s.Delimiters = append(s.Delimiters, delimiter)
			lexer.SetResultSymbol(mojoSymStringStart)
			s.InsideInterpolatedString = delimiter.IsFormat()
			return true
		}
		if hasFlags {
			return false
		}
	}

	return false
}
