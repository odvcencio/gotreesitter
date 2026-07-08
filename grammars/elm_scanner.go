//go:build !grammar_subset || grammar_subset_elm

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Elm grammar.
const (
	elmTokVirtualEndDecl   = 0
	elmTokVirtualOpenSect  = 1
	elmTokVirtualEndSect   = 2
	elmTokOperatorIdent    = 3 // minus without trailing whitespace
	elmTokGlslContent      = 4
	elmTokBlockCommentBody = 5
	elmTokStringMultiline  = 6
)

const (
	elmSymVirtualEndDecl   gotreesitter.Symbol = 78
	elmSymVirtualOpenSect  gotreesitter.Symbol = 79
	elmSymVirtualEndSect   gotreesitter.Symbol = 80
	elmSymOperatorIdent    gotreesitter.Symbol = 81
	elmSymGlslContent      gotreesitter.Symbol = 82
	elmSymBlockCommentBody gotreesitter.Symbol = 83
	elmSymStringMultiline  gotreesitter.Symbol = 84
)

type elmState struct {
	indentLength uint32
	indents      []uint8
	runback      []uint8 // 0 = END_DECL, 1 = END_SECTION
}

// ElmExternalScanner handles indentation-based layout for Elm.
type ElmExternalScanner struct{}

func (ElmExternalScanner) Create() any {
	return &elmState{indents: []uint8{0}}
}
func (ElmExternalScanner) Destroy(payload any) {}

func (ElmExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*elmState)
	size := 0

	runbackLen := len(s.runback)
	if runbackLen > 255 {
		runbackLen = 255
	}
	if 3+len(s.indents)+runbackLen >= len(buf) {
		return 0
	}

	buf[size] = byte(runbackLen)
	size++
	for i := 0; i < runbackLen; i++ {
		buf[size] = s.runback[i]
		size++
	}

	// indent_length as 4 bytes little-endian
	buf[size] = 4
	size++
	buf[size] = byte(s.indentLength)
	buf[size+1] = byte(s.indentLength >> 8)
	buf[size+2] = byte(s.indentLength >> 16)
	buf[size+3] = byte(s.indentLength >> 24)
	size += 4

	for i := 1; i < len(s.indents) && size < len(buf); i++ {
		buf[size] = s.indents[i]
		size++
	}

	return size
}

func (ElmExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*elmState)
	s.runback = s.runback[:0]
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	s.indentLength = 0

	if len(buf) == 0 {
		return
	}

	size := 0
	runbackLen := int(buf[size])
	size++
	for i := 0; i < runbackLen && size < len(buf); i++ {
		s.runback = append(s.runback, buf[size])
		size++
	}

	if size >= len(buf) {
		return
	}
	indentLenLen := int(buf[size])
	size++
	if indentLenLen > 0 && size+indentLenLen <= len(buf) {
		s.indentLength = uint32(buf[size]) |
			uint32(buf[size+1])<<8 |
			uint32(buf[size+2])<<16 |
			uint32(buf[size+3])<<24
		size += indentLenLen
	}

	for ; size < len(buf); size++ {
		s.indents = append(s.indents, buf[size])
	}
}

func (ElmExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*elmState)

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	// Error recovery: all tokens valid at once
	if isValid(elmTokVirtualEndDecl) && isValid(elmTokVirtualOpenSect) &&
		isValid(elmTokVirtualEndSect) && isValid(elmTokOperatorIdent) &&
		isValid(elmTokGlslContent) && isValid(elmTokBlockCommentBody) &&
		isValid(elmTokStringMultiline) {
		return false
	}

	// Handle deferred runback tokens
	if len(s.runback) > 0 && s.runback[len(s.runback)-1] == 0 && isValid(elmTokVirtualEndDecl) {
		s.runback = s.runback[:len(s.runback)-1]
		lexer.SetResultSymbol(elmSymVirtualEndDecl)
		return true
	}
	if len(s.runback) > 0 && s.runback[len(s.runback)-1] == 1 && isValid(elmTokVirtualEndSect) {
		s.runback = s.runback[:len(s.runback)-1]
		lexer.SetResultSymbol(elmSymVirtualEndSect)
		return true
	}
	s.runback = s.runback[:0]

	// Multiline string content (triple-quoted)
	if isValid(elmTokStringMultiline) {
		lexer.SetResultSymbol(elmSymStringMultiline)
		hasContent := false
		for lexer.Lookahead() != 0 {
			switch lexer.Lookahead() {
			case '"':
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.Advance(false)
					if lexer.Lookahead() == '"' {
						return hasContent
					}
					hasContent = true
				} else {
					hasContent = true
				}
			case '\\':
				lexer.MarkEnd()
				return hasContent
			default:
				hasContent = true
				lexer.Advance(false)
			}
		}
		lexer.MarkEnd()
		return hasContent
	}

	// Whitespace/newline/comment scanning
	hasNewline := false
	foundIn := false
	canCallMarkEnd := true
	lexer.MarkEnd()

	for {
		ch := lexer.Lookahead()
		if ch == ' ' || ch == '\r' {
			lexer.Advance(true)
		} else if ch == '\n' {
			lexer.Advance(true)
			hasNewline = true
			for lexer.Lookahead() == ' ' {
				lexer.Advance(true)
			}
			s.indentLength = lexer.Column()
		} else if !isValid(elmTokBlockCommentBody) && ch == '-' {
			lexer.Advance(false)
			la := lexer.Lookahead()

			// Minus without trailing whitespace (negation)
			if isValid(elmTokOperatorIdent) &&
				((la >= 'a' && la <= 'z') || (la >= 'A' && la <= 'Z') || la == '(' || la > 127) {
				if canCallMarkEnd {
					lexer.SetResultSymbol(elmSymOperatorIdent)
					lexer.MarkEnd()
					return true
				}
				return false
			}

			// Line comment: --
			if la == '-' && hasNewline {
				canCallMarkEnd = false
				lexer.Advance(false)
				for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
					lexer.Advance(false)
				}
			} else if isValid(elmTokBlockCommentBody) && la == '}' {
				lexer.SetResultSymbol(elmSymBlockCommentBody)
				return true
			} else {
				return false
			}
		} else if lexer.Lookahead() == 0 { // EOF
			if isValid(elmTokVirtualEndSect) {
				lexer.SetResultSymbol(elmSymVirtualEndSect)
				return true
			}
			if isValid(elmTokVirtualEndDecl) {
				lexer.SetResultSymbol(elmSymVirtualEndDecl)
				return true
			}
			break
		} else {
			break
		}
	}

	// Check for `in` keyword (ends let section)
	if isValid(elmTokVirtualEndSect) && lexer.Lookahead() == 'i' {
		lexer.Advance(true)
		if lexer.Lookahead() == 'n' {
			lexer.Advance(true)
			if elmIsSpace(lexer) || lexer.Lookahead() == 0 {
				if hasNewline {
					foundIn = true
				} else {
					lexer.SetResultSymbol(elmSymVirtualEndSect)
					if len(s.indents) > 0 {
						s.indents = s.indents[:len(s.indents)-1]
					}
					return true
				}
			}
		}
	}

	// Check for section-ending tokens: ), comma, }
	if isValid(elmTokVirtualEndSect) &&
		(lexer.Lookahead() == ')' || lexer.Lookahead() == ',' || lexer.Lookahead() == '}') {
		lexer.SetResultSymbol(elmSymVirtualEndSect)
		if len(s.indents) > 0 {
			s.indents = s.indents[:len(s.indents)-1]
		}
		return true
	}

	// Virtual open section
	if isValid(elmTokVirtualOpenSect) && lexer.Lookahead() != 0 {
		if len(s.indents) >= 256 {
			return false
		}
		s.indents = append(s.indents, uint8(lexer.Column()))
		lexer.SetResultSymbol(elmSymVirtualOpenSect)
		return true
	}

	// Block comment content
	if isValid(elmTokBlockCommentBody) {
		if !canCallMarkEnd {
			return false
		}
		lexer.MarkEnd()
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() != '{' && lexer.Lookahead() != '-' {
				lexer.Advance(false)
			} else if lexer.Lookahead() == '-' {
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == '}' {
					break
				}
			} else {
				// '{' — might be nested block comment
				if elmScanBlockComment(lexer) {
					lexer.MarkEnd()
				}
			}
		}
		lexer.SetResultSymbol(elmSymBlockCommentBody)
		return true
	}

	// Newline indent handling
	if hasNewline {
		s.runback = s.runback[:0]

		// Skip past block comments that could distort indent measurement
		if lexer.Lookahead() == '{' && !isValid(elmTokBlockCommentBody) &&
			len(s.indents) > 0 && s.indentLength < uint32(s.indents[len(s.indents)-1]) {
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				canCallMarkEnd = false
				lexer.Advance(false)
				elmSkipBlockComment(lexer)
				elmSkipWhitespaceAndRemeasure(lexer, s)
				// Check for additional block comments
				for lexer.Lookahead() == '{' {
					lexer.Advance(false)
					if lexer.Lookahead() == '-' {
						lexer.Advance(false)
						elmSkipBlockComment(lexer)
						elmSkipWhitespaceAndRemeasure(lexer, s)
					} else {
						break
					}
				}
			}
		}

		for len(s.indents) > 0 && s.indentLength <= uint32(s.indents[len(s.indents)-1]) {
			if s.indentLength == uint32(s.indents[len(s.indents)-1]) {
				if foundIn {
					s.indents = s.indents[:len(s.indents)-1]
					s.runback = append(s.runback, 1)
					foundIn = false
					break
				}
				// Don't insert END_DECL before line or block comment
				if lexer.Lookahead() == '-' {
					lexer.Advance(true)
					if lexer.Lookahead() == '-' {
						break
					}
				}
				if lexer.Lookahead() == '{' {
					lexer.Advance(true)
					if lexer.Lookahead() == '-' {
						break
					}
				}
				s.runback = append(s.runback, 0)
				break
			}
			if s.indentLength < uint32(s.indents[len(s.indents)-1]) {
				s.indents = s.indents[:len(s.indents)-1]
				s.runback = append(s.runback, 1)
				if foundIn && (len(s.indents) == 0 ||
					s.indentLength > uint32(s.indents[len(s.indents)-1])) {
					foundIn = false
				}
			}
		}

		if foundIn {
			if len(s.indents) > 0 {
				s.indents = s.indents[:len(s.indents)-1]
			}
			s.runback = append(s.runback, 1)
		}

		// Reverse runback so we pop from the end
		for i, j := 0, len(s.runback)-1; i < j; i, j = i+1, j-1 {
			s.runback[i], s.runback[j] = s.runback[j], s.runback[i]
		}

		if len(s.runback) > 0 && s.runback[len(s.runback)-1] == 0 && isValid(elmTokVirtualEndDecl) {
			s.runback = s.runback[:len(s.runback)-1]
			lexer.SetResultSymbol(elmSymVirtualEndDecl)
			return true
		}
		if len(s.runback) > 0 && s.runback[len(s.runback)-1] == 1 && isValid(elmTokVirtualEndSect) {
			s.runback = s.runback[:len(s.runback)-1]
			lexer.SetResultSymbol(elmSymVirtualEndSect)
			return true
		}
		if lexer.Lookahead() == 0 && isValid(elmTokVirtualEndSect) {
			lexer.SetResultSymbol(elmSymVirtualEndSect)
			return true
		}
	}

	// GLSL content: scan until |]
	if isValid(elmTokGlslContent) {
		if !canCallMarkEnd {
			return false
		}
		lexer.SetResultSymbol(elmSymGlslContent)
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == '|' {
				lexer.MarkEnd()
				lexer.Advance(false)
				if lexer.Lookahead() == ']' {
					lexer.Advance(false)
					return true
				}
			} else {
				lexer.Advance(false)
			}
		}
		lexer.MarkEnd()
		return true
	}

	return false
}

func elmIsSpace(lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	return ch == ' ' || ch == '\r' || ch == '\n'
}

func elmScanBlockComment(lexer *gotreesitter.ExternalLexer) bool {
	lexer.MarkEnd()
	if lexer.Lookahead() != '{' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	for lexer.Lookahead() != 0 {
		switch lexer.Lookahead() {
		case '{':
			elmScanBlockComment(lexer)
		case '-':
			lexer.Advance(false)
			if lexer.Lookahead() == '}' {
				lexer.Advance(false)
				return true
			}
		default:
			lexer.Advance(false)
		}
	}
	return true
}

func elmSkipBlockComment(lexer *gotreesitter.ExternalLexer) {
	nesting := 1
	for nesting > 0 && lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '{' {
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
				nesting++
			}
		} else if lexer.Lookahead() == '-' {
			lexer.Advance(false)
			if lexer.Lookahead() == '}' {
				lexer.Advance(false)
				nesting--
			}
		} else {
			lexer.Advance(false)
		}
	}
}

func elmSkipWhitespaceAndRemeasure(lexer *gotreesitter.ExternalLexer, s *elmState) {
	for unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' {
			lexer.Advance(false)
			for lexer.Lookahead() == ' ' {
				lexer.Advance(false)
			}
			s.indentLength = lexer.Column()
		} else {
			lexer.Advance(false)
		}
	}
}
