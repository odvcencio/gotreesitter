//go:build !grammar_subset || grammar_subset_bitbake

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the BitBake grammar.
const (
	bbTokConcat        = 0
	bbTokNewline       = 1
	bbTokIndent        = 2
	bbTokDedent        = 3
	bbTokStringStart   = 4
	bbTokStringContent = 5
	bbTokEscapeInterp  = 6
	bbTokStringEnd     = 7
	bbTokComment       = 8
	bbTokCloseParen    = 9
	bbTokCloseBracket  = 10
	bbTokCloseBrace    = 11
	bbTokShellContent  = 12
)

const (
	bbSymConcat        gotreesitter.Symbol = 140
	bbSymNewline       gotreesitter.Symbol = 141
	bbSymIndent        gotreesitter.Symbol = 142
	bbSymDedent        gotreesitter.Symbol = 143
	bbSymStringStart   gotreesitter.Symbol = 144
	bbSymStringContent gotreesitter.Symbol = 145
	bbSymEscapeInterp  gotreesitter.Symbol = 146
	bbSymStringEnd     gotreesitter.Symbol = 147
	bbSymShellContent  gotreesitter.Symbol = 148
)

// BitbakeExternalScanner handles Python-like indent/dedent, strings, concat, and shell content.
type BitbakeExternalScanner struct{}

// Reuse pythonScannerState — same internal structure.

func (BitbakeExternalScanner) Create() any {
	return &pythonScannerState{Indents: []uint16{0}}
}
func (BitbakeExternalScanner) Destroy(payload any) {}

func (BitbakeExternalScanner) Serialize(payload any, buf []byte) int {
	// Bitbake serialize format: 1 byte f-string flag, 1 byte delim count,
	// N bytes Delimiters, remaining bytes = Indents[1:] (1 byte each).
	s := payload.(*pythonScannerState)
	if len(buf) == 0 {
		return 0
	}
	size := 0
	if s.InsideInterpolatedString {
		buf[size] = 1
	} else {
		buf[size] = 0
	}
	size++

	delimCount := len(s.Delimiters)
	if delimCount > 255 {
		delimCount = 255
	}
	if size >= len(buf) {
		return size
	}
	buf[size] = byte(delimCount)
	size++
	for i := 0; i < delimCount && size < len(buf); i++ {
		buf[size] = byte(s.Delimiters[i])
		size++
	}
	for i := 1; i < len(s.Indents) && size < len(buf); i++ {
		buf[size] = byte(s.Indents[i])
		size++
	}
	return size
}

func (BitbakeExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*pythonScannerState)
	s.Delimiters = s.Delimiters[:0]
	s.Indents = s.Indents[:0]
	s.Indents = append(s.Indents, 0)
	s.InsideInterpolatedString = false

	if len(buf) == 0 {
		return
	}
	size := 0
	s.InsideInterpolatedString = buf[size] != 0
	size++
	if size >= len(buf) {
		return
	}
	delimCount := int(buf[size])
	size++
	for i := 0; i < delimCount && size < len(buf); i++ {
		s.Delimiters = append(s.Delimiters, pyDelimiter(buf[size]))
		size++
	}
	for ; size < len(buf); size++ {
		s.Indents = append(s.Indents, uint16(buf[size]))
	}
}

func (BitbakeExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*pythonScannerState)
	if len(s.Indents) == 0 {
		s.Indents = append(s.Indents, 0)
	}

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	errorRecoveryMode := isValid(bbTokStringContent) && isValid(bbTokIndent)
	withinBrackets := isValid(bbTokCloseBrace) || isValid(bbTokCloseParen) || isValid(bbTokCloseBracket)

	// ---- CONCAT ----
	if isValid(bbTokConcat) && !errorRecoveryMode {
		ch := lexer.Lookahead()
		if ch != 0 && !unicode.IsSpace(ch) && ch != '(' && ch != ':' && ch != '[' && ch != '=' {
			lexer.SetResultSymbol(bbSymConcat)
			return true
		}
	}

	// ---- Escape interpolation ----
	advancedOnce := false
	if isValid(bbTokEscapeInterp) && len(s.Delimiters) > 0 &&
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
				lexer.SetResultSymbol(bbSymEscapeInterp)
				return true
			}
			return false
		}
	}

	// ---- String content ----
	if isValid(bbTokStringContent) && len(s.Delimiters) > 0 && !errorRecoveryMode {
		delimiter := s.Delimiters[len(s.Delimiters)-1]
		EndChar := delimiter.EndChar()
		hasContent := advancedOnce

		for lexer.Lookahead() != 0 {
			if (advancedOnce || lexer.Lookahead() == '{' || lexer.Lookahead() == '}') && delimiter.IsFormat() {
				lexer.MarkEnd()
				lexer.SetResultSymbol(bbSymStringContent)
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
						lexer.SetResultSymbol(bbSymStringContent)
						return hasContent
					}
				} else {
					lexer.MarkEnd()
					lexer.SetResultSymbol(bbSymStringContent)
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
								lexer.SetResultSymbol(bbSymStringContent)
							} else {
								lexer.Advance(false)
								lexer.MarkEnd()
								s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
								lexer.SetResultSymbol(bbSymStringEnd)
								s.InsideInterpolatedString = false
							}
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(bbSymStringContent)
						return true
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(bbSymStringContent)
					return true
				}
				if hasContent {
					lexer.SetResultSymbol(bbSymStringContent)
				} else {
					lexer.Advance(false)
					s.Delimiters = s.Delimiters[:len(s.Delimiters)-1]
					lexer.SetResultSymbol(bbSymStringEnd)
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

	// ---- Indent/dedent scanning ----
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
		case '\\':
			if isValid(bbTokStringContent) {
				lexer.Advance(true)
				if lexer.Lookahead() == '\r' {
					lexer.Advance(true)
				}
				if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
					lexer.Advance(true)
				} else {
					return false
				}
				continue
			}
			goto bbAfterIndentLoop
		case 0:
			indentLength = 0
			foundEndOfLine = true
			goto bbAfterIndentLoop
		default:
			goto bbAfterIndentLoop
		}
	}

bbAfterIndentLoop:
	if foundEndOfLine {
		currentIndent := s.Indents[len(s.Indents)-1]

		if isValid(bbTokIndent) && indentLength > currentIndent {
			s.Indents = append(s.Indents, indentLength)
			lexer.SetResultSymbol(bbSymIndent)
			return true
		}

		nextTokIsStringStart := lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' || lexer.Lookahead() == '`'

		if (isValid(bbTokDedent) ||
			(!isValid(bbTokNewline) && !(isValid(bbTokStringStart) && nextTokIsStringStart) && !withinBrackets)) &&
			indentLength < currentIndent &&
			!s.InsideInterpolatedString &&
			firstCommentIndentLength < int32(currentIndent) {
			s.Indents = s.Indents[:len(s.Indents)-1]
			lexer.SetResultSymbol(bbSymDedent)
			return true
		}

		if isValid(bbTokNewline) && !errorRecoveryMode {
			lexer.SetResultSymbol(bbSymNewline)
			return true
		}
	}

	// ---- String start ----
	if firstCommentIndentLength == -1 && isValid(bbTokStringStart) {
		var delimiter pyDelimiter
		hasFlags := false

		for lexer.Lookahead() != 0 {
			switch lexer.Lookahead() {
			case 'f', 'F':
				delimiter |= pyDelimFormat
			case 'r', 'R':
				delimiter |= pyDelimRaw
			case 'b', 'B':
				delimiter |= pyDelimBytes
			case 'u', 'U':
				// accepted prefix, no flag
			default:
				goto bbAfterFlags
			}
			hasFlags = true
			lexer.Advance(false)
		}

	bbAfterFlags:
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
			lexer.SetResultSymbol(bbSymStringStart)
			s.InsideInterpolatedString = delimiter.IsFormat()
			return true
		}
		if hasFlags {
			return false
		}
	}

	// ---- Shell content ----
	if isValid(bbTokShellContent) && !errorRecoveryMode {
		// Skip whitespace until newline
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
			if lexer.Lookahead() == '\n' {
				lexer.Advance(true)
				break
			}
		}

		advOnce := false
		var braceDepth uint8
		var startQuote rune

		for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
			switch lexer.Lookahead() {
			case '\'', '"':
				if startQuote == 0 {
					startQuote = lexer.Lookahead()
				} else if lexer.Lookahead() == startQuote {
					startQuote = 0
				}
				lexer.Advance(false)
				advOnce = true
			case '$':
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '{' {
					lexer.Advance(false)
					braceDepth++
					if lexer.Lookahead() == '@' {
						lexer.Advance(false)
						lexer.SetResultSymbol(bbSymShellContent)
						return advOnce
					}
				}
				advOnce = true
			case '{':
				lexer.Advance(false)
				if startQuote == 0 {
					braceDepth++
				}
			case '}':
				lexer.Advance(false)
				if startQuote == 0 {
					braceDepth--
				}
			case '\r', '\t', '\f', '\v', ' ':
				lexer.Advance(false)
			default:
				lexer.Advance(false)
				advOnce = true
			}
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(bbSymShellContent)
		return advOnce && braceDepth == 0
	}

	return false
}
