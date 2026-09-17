//go:build !grammar_subset || grammar_subset_gdscript

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the GDScript grammar.
const (
	gdsTokNewline = iota
	gdsTokIndent
	gdsTokDedent
	gdsTokStringStart
	gdsTokStringContent
	gdsTokStringEnd
	gdsTokStringNameStart
	gdsTokNodePathStart
	gdsTokCloseParen
	gdsTokCloseBracket
	gdsTokCloseBrace
	gdsTokComma
	gdsTokBodyEnd
)

const (
	gdsSymNewline         gotreesitter.Symbol = 102
	gdsSymIndent          gotreesitter.Symbol = 103
	gdsSymDedent          gotreesitter.Symbol = 104
	gdsSymStringStart     gotreesitter.Symbol = 105
	gdsSymStringContent   gotreesitter.Symbol = 106
	gdsSymStringEnd       gotreesitter.Symbol = 107
	gdsSymStringNameStart gotreesitter.Symbol = 108
	gdsSymNodePathStart   gotreesitter.Symbol = 109
	gdsSymBodyEnd         gotreesitter.Symbol = 110
)

type gdsDelimiter byte

const (
	gdsDelimSingleQuote gdsDelimiter = 1 << 0
	gdsDelimDoubleQuote gdsDelimiter = 1 << 1
	gdsDelimTriple      gdsDelimiter = 1 << 2
	gdsDelimRaw         gdsDelimiter = 1 << 3
	gdsDelimName        gdsDelimiter = 1 << 4
	gdsDelimNodePath    gdsDelimiter = 1 << 5
)

func (d gdsDelimiter) endChar() rune {
	if d&gdsDelimSingleQuote != 0 {
		return '\''
	}
	if d&gdsDelimDoubleQuote != 0 {
		return '"'
	}
	return 0
}
func (d gdsDelimiter) isTriple() bool   { return d&gdsDelimTriple != 0 }
func (d gdsDelimiter) isRaw() bool      { return d&gdsDelimRaw != 0 }
func (d gdsDelimiter) isName() bool     { return d&gdsDelimName != 0 }
func (d gdsDelimiter) isNodePath() bool { return d&gdsDelimNodePath != 0 }

type gdscriptState struct {
	indents    []uint16
	delimiters []gdsDelimiter
}

// GdscriptExternalScanner handles indent/dedent, strings, and body_end for GDScript.
type GdscriptExternalScanner struct{}

func (GdscriptExternalScanner) Create() any {
	return &gdscriptState{indents: []uint16{0}}
}
func (GdscriptExternalScanner) Destroy(payload any) {}

func (GdscriptExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*gdscriptState)
	if len(buf) == 0 {
		return 0
	}
	size := 0
	delimCount := len(s.delimiters)
	if delimCount > 255 {
		delimCount = 255
	}
	buf[size] = byte(delimCount)
	size++
	for i := 0; i < delimCount && size < len(buf); i++ {
		buf[size] = byte(s.delimiters[i])
		size++
	}
	for i := 1; i < len(s.indents) && size < len(buf); i++ {
		buf[size] = byte(s.indents[i])
		size++
	}
	return size
}

func (GdscriptExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*gdscriptState)
	s.delimiters = s.delimiters[:0]
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	if len(buf) == 0 {
		return
	}
	size := 0
	delimCount := int(buf[size])
	size++
	for i := 0; i < delimCount && size < len(buf); i++ {
		s.delimiters = append(s.delimiters, gdsDelimiter(buf[size]))
		size++
	}
	for ; size < len(buf); size++ {
		s.indents = append(s.indents, uint16(buf[size]))
	}
}

func (GdscriptExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*gdscriptState)
	if len(s.indents) == 0 {
		s.indents = append(s.indents, 0)
	}

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	errorRecoveryMode := isValid(gdsTokStringContent) && isValid(gdsTokIndent)

	// ---- String content scanning ----
	if isValid(gdsTokStringContent) && len(s.delimiters) > 0 && !errorRecoveryMode {
		delimiter := s.delimiters[len(s.delimiters)-1]
		endCh := delimiter.endChar()
		hasContent := false
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == '\\' {
				if delimiter.isRaw() {
					lexer.Advance(false)
					if lexer.Lookahead() == endCh || lexer.Lookahead() == '\\' {
						lexer.Advance(false)
					}
					continue
				}
				lexer.MarkEnd()
				lexer.SetResultSymbol(gdsSymStringContent)
				return hasContent
			} else if lexer.Lookahead() == endCh {
				if delimiter.isTriple() {
					lexer.MarkEnd()
					lexer.Advance(false)
					if lexer.Lookahead() == endCh {
						lexer.Advance(false)
						if lexer.Lookahead() == endCh {
							if hasContent {
								lexer.SetResultSymbol(gdsSymStringContent)
							} else {
								lexer.Advance(false)
								lexer.MarkEnd()
								s.delimiters = s.delimiters[:len(s.delimiters)-1]
								lexer.SetResultSymbol(gdsSymStringEnd)
							}
							return true
						}
						lexer.MarkEnd()
						lexer.SetResultSymbol(gdsSymStringContent)
						return true
					}
					lexer.MarkEnd()
					lexer.SetResultSymbol(gdsSymStringContent)
					return true
				}
				if hasContent {
					lexer.SetResultSymbol(gdsSymStringContent)
				} else {
					lexer.Advance(false)
					s.delimiters = s.delimiters[:len(s.delimiters)-1]
					lexer.SetResultSymbol(gdsSymStringEnd)
				}
				lexer.MarkEnd()
				return true
			}
			lexer.Advance(false)
			hasContent = true
		}
	}

	lexer.MarkEnd()

	foundEndOfLine := false
	var indentLength uint32
	var lastNonEmptyIndent uint32

	for {
		ch := lexer.Lookahead()
		switch {
		case ch == '\n':
			foundEndOfLine = true
			indentLength = 0
			lexer.Advance(true)
		case ch == ' ':
			indentLength++
			lexer.Advance(true)
		case ch == '\r' || ch == '\f':
			indentLength = 0
			lexer.Advance(true)
		case ch == '\t':
			indentLength += 8
			lexer.Advance(true)
		case ch == '#':
			if !foundEndOfLine {
				goto afterIndentLoop
			}
			lastNonEmptyIndent = indentLength

			commentIndent := indentLength
			isRegion := false
			if commentIndent == 0 {
				lexer.Advance(true) // skip #
				if lexer.Lookahead() == 'r' {
					isRegion = gdsLookaheadString(lexer, "region")
				} else if lexer.Lookahead() == 'e' {
					isRegion = gdsLookaheadString(lexer, "endregion")
				}
			}

			if !isRegion && len(s.indents) > 1 && commentIndent == 0 {
				funcIndent := s.indents[1]
				if funcIndent > 0 {
					for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
						lexer.Advance(true)
					}
					if lexer.Lookahead() == '\n' {
						lexer.Advance(true)
					}
					var nextIndent uint32
					for gdsSkipWS(lexer, &nextIndent) {
					}
					if nextIndent > 0 {
						indentLength = uint32(funcIndent)
					}
				}
			}

			goto afterIndentLoop

		case ch == '\\':
			lexer.Advance(true)
			if lexer.Lookahead() == '\r' {
				lexer.Advance(true)
			}
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == 0 {
				lexer.Advance(true)
			} else {
				return false
			}
		case ch == 0: // EOF
			if lastNonEmptyIndent > 0 {
				indentLength = lastNonEmptyIndent
			}
			if len(s.indents) > 0 {
				if indentLength != uint32(s.indents[len(s.indents)-1]) {
					indentLength = 0
				}
			} else {
				indentLength = 0
			}
			foundEndOfLine = true
			goto afterIndentLoop
		default:
			if indentLength == 0 && lastNonEmptyIndent > 0 && len(s.indents) > 0 {
				if lastNonEmptyIndent == uint32(s.indents[len(s.indents)-1]) {
					return false
				}
			}
			goto afterIndentLoop
		}
	}

afterIndentLoop:
	if foundEndOfLine {
		if len(s.indents) > 0 {
			current := s.indents[len(s.indents)-1]
			if isValid(gdsTokIndent) && indentLength > uint32(current) {
				s.indents = append(s.indents, uint16(indentLength))
				lexer.SetResultSymbol(gdsSymIndent)
				return true
			}
			if isValid(gdsTokDedent) && indentLength < uint32(current) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.SetResultSymbol(gdsSymDedent)
				return true
			}
		}
		if isValid(gdsTokNewline) && !errorRecoveryMode {
			lexer.SetResultSymbol(gdsSymNewline)
			return true
		}
	}

	// BODY_END — fires when a closing bracket/paren/brace/comma is seen
	// but the grammar doesn't expect the specific bracket token.
	if !isValid(gdsTokComma) &&
		!isValid(gdsTokCloseParen) && !isValid(gdsTokCloseBrace) &&
		!isValid(gdsTokCloseBracket) &&
		(errorRecoveryMode || isValid(gdsTokBodyEnd)) {
		ch := lexer.Lookahead()
		if ch == ',' || ch == ')' || ch == '}' || ch == ']' {
			if isValid(gdsTokDedent) && len(s.indents) > 0 {
				s.indents = s.indents[:len(s.indents)-1]
			}
			lexer.SetResultSymbol(gdsSymBodyEnd)
			return true
		}
	}

	// String / name / node-path start
	if isValid(gdsTokStringStart) || isValid(gdsTokStringNameStart) || isValid(gdsTokNodePathStart) {
		var delimiter gdsDelimiter
		hasFlags := true

		switch lexer.Lookahead() {
		case 'r':
			delimiter |= gdsDelimRaw
		case '&':
			delimiter |= gdsDelimName
		case '^', '@':
			delimiter |= gdsDelimNodePath
		default:
			hasFlags = false
		}

		if hasFlags {
			lexer.Advance(false)
		}

		if lexer.Lookahead() == '\'' || lexer.Lookahead() == '"' {
			gdsHandleQuote(lexer, &delimiter)
		}

		if delimiter.endChar() != 0 {
			s.delimiters = append(s.delimiters, delimiter)
			if delimiter.isNodePath() {
				lexer.SetResultSymbol(gdsSymNodePathStart)
			} else if delimiter.isName() {
				lexer.SetResultSymbol(gdsSymStringNameStart)
			} else {
				lexer.SetResultSymbol(gdsSymStringStart)
			}
			return true
		}

		if hasFlags {
			return false
		}
	}

	return false
}

func gdsHandleQuote(lexer *gotreesitter.ExternalLexer, delimiter *gdsDelimiter) {
	quote := lexer.Lookahead()
	if quote == '\'' {
		*delimiter |= gdsDelimSingleQuote
	} else {
		*delimiter |= gdsDelimDoubleQuote
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	if lexer.Lookahead() == quote {
		lexer.Advance(false)
		if lexer.Lookahead() == quote {
			lexer.Advance(false)
			lexer.MarkEnd()
			*delimiter |= gdsDelimTriple
		}
	}
}

func gdsSkipWS(lexer *gotreesitter.ExternalLexer, indent *uint32) bool {
	switch lexer.Lookahead() {
	case '\n':
		*indent = 0
		lexer.Advance(true)
		return true
	case ' ':
		*indent++
		lexer.Advance(true)
		return true
	case '\r', '\f':
		*indent = 0
		lexer.Advance(true)
		return true
	case '\t':
		*indent += 8
		lexer.Advance(true)
		return true
	}
	return false
}

func gdsLookaheadString(lexer *gotreesitter.ExternalLexer, word string) bool {
	for i := 0; i < len(word); i++ {
		if lexer.Lookahead() != rune(word[i]) {
			return false
		}
		lexer.Advance(true)
	}
	return true
}
