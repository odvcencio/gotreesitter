package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Scala grammar.
const (
	scaTokAutoSemicolon          = 0
	scaTokIndent                 = 1
	scaTokOutdent                = 2
	scaTokSimpleStringStart      = 3
	scaTokSimpleStringMiddle     = 4
	scaTokSimpleMultiStringStart = 5
	scaTokInterpStringMiddle     = 6
	scaTokInterpMultiStringMiddle = 7
	scaTokRawStringStart         = 8
	scaTokRawStringMiddle        = 9
	scaTokRawMultiStringMiddle   = 10
	scaTokSingleLineStringEnd    = 11
	scaTokMultilineStringEnd     = 12
	scaTokElse                   = 13
	scaTokCatch                  = 14
	scaTokFinally                = 15
	scaTokExtends                = 16
	scaTokDerives                = 17
	scaTokWith                   = 18
	scaTokErrorSentinel          = 19
)

const (
	scaSymAutoSemicolon          gotreesitter.Symbol = 104
	scaSymIndent                 gotreesitter.Symbol = 105
	scaSymOutdent                gotreesitter.Symbol = 106
	scaSymSimpleStringStart      gotreesitter.Symbol = 107
	scaSymSimpleStringMiddle     gotreesitter.Symbol = 108
	scaSymSimpleMultiStringStart gotreesitter.Symbol = 109
	scaSymInterpStringMiddle     gotreesitter.Symbol = 110
	scaSymInterpMultiStringMiddle gotreesitter.Symbol = 111
	scaSymRawStringStart         gotreesitter.Symbol = 112
	scaSymRawStringMiddle        gotreesitter.Symbol = 113
	scaSymRawMultiStringMiddle   gotreesitter.Symbol = 114
	scaSymSingleLineStringEnd    gotreesitter.Symbol = 115
	scaSymMultilineStringEnd     gotreesitter.Symbol = 116
)

type scalaState struct {
	indents             []int16
	lastIndentationSize int16
	lastNewlineCount    int16
	lastColumn          int16
}

// ScalaExternalScanner handles auto-semicolons, indent/outdent, and string scanning for Scala.
type ScalaExternalScanner struct{}

func (ScalaExternalScanner) Create() any {
	return &scalaState{
		lastIndentationSize: -1,
		lastColumn:          -1,
	}
}
func (ScalaExternalScanner) Destroy(payload any) {}

func (ScalaExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*scalaState)
	needed := (len(s.indents) + 3) * 2
	if needed > len(buf) {
		return 0
	}
	size := 0
	putI16 := func(v int16) {
		buf[size] = byte(v)
		buf[size+1] = byte(v >> 8)
		size += 2
	}
	putI16(s.lastIndentationSize)
	putI16(s.lastNewlineCount)
	putI16(s.lastColumn)
	for _, v := range s.indents {
		putI16(v)
	}
	return size
}

func (ScalaExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*scalaState)
	s.indents = s.indents[:0]
	s.lastIndentationSize = -1
	s.lastColumn = -1
	s.lastNewlineCount = 0

	if len(buf) == 0 {
		return
	}
	size := 0
	getI16 := func() int16 {
		v := int16(buf[size]) | int16(buf[size+1])<<8
		size += 2
		return v
	}
	s.lastIndentationSize = getI16()
	s.lastNewlineCount = getI16()
	s.lastColumn = getI16()
	for size+1 < len(buf) {
		s.indents = append(s.indents, getI16())
	}
}

func (ScalaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*scalaState)

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	scaBack := func() int16 {
		if len(s.indents) > 0 {
			return s.indents[len(s.indents)-1]
		}
		return -1
	}

	prev := scaBack()
	var newlineCount int16
	var indentSize int16

	// Skip whitespace, count newlines
	for unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' {
			newlineCount++
			indentSize = 0
		} else {
			indentSize++
		}
		lexer.Advance(true)
	}

	// Double outdent check
	if isValid(scaTokOutdent) &&
		(lexer.Lookahead() == 0 ||
			(prev != -1 && (lexer.Lookahead() == ')' || lexer.Lookahead() == ']' || lexer.Lookahead() == '}')) ||
			(s.lastIndentationSize != -1 && prev != -1 && s.lastIndentationSize < prev)) {
		if len(s.indents) > 0 {
			s.indents = s.indents[:len(s.indents)-1]
		}
		lexer.SetResultSymbol(scaSymOutdent)
		return true
	}
	s.lastIndentationSize = -1

	// Indent
	if isValid(scaTokIndent) && newlineCount > 0 &&
		(len(s.indents) == 0 || indentSize > scaBack()) {
		if scaDetectCommentStart(lexer) {
			return false
		}
		s.indents = append(s.indents, indentSize)
		lexer.SetResultSymbol(scaSymIndent)
		return true
	}

	// Single outdent
	if isValid(scaTokOutdent) &&
		(lexer.Lookahead() == 0 ||
			(newlineCount > 0 && prev != -1 && indentSize < prev)) {
		if len(s.indents) > 0 {
			s.indents = s.indents[:len(s.indents)-1]
		}
		lexer.SetResultSymbol(scaSymOutdent)
		s.lastIndentationSize = indentSize
		s.lastNewlineCount = newlineCount
		if lexer.Lookahead() == 0 {
			s.lastColumn = -1
		} else {
			s.lastColumn = int16(lexer.GetColumn())
		}
		return true
	}

	// Recover newline_count from outdent reset
	isEOF := lexer.Lookahead() == 0
	if (s.lastNewlineCount > 0 && isEOF && s.lastColumn == -1) ||
		(!isEOF && int16(lexer.GetColumn()) == s.lastColumn) {
		newlineCount += s.lastNewlineCount
	}
	s.lastNewlineCount = 0

	// Auto-semicolon
	if isValid(scaTokAutoSemicolon) && newlineCount > 0 {
		lexer.MarkEnd()
		lexer.SetResultSymbol(scaSymAutoSemicolon)

		if lexer.Lookahead() == '.' {
			return false
		}

		// Comments
		if lexer.Lookahead() == '/' {
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				return false
			}
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				for lexer.Lookahead() != 0 {
					if lexer.Lookahead() == '*' {
						lexer.Advance(false)
						if lexer.Lookahead() == '/' {
							lexer.Advance(false)
							break
						}
					} else {
						lexer.Advance(false)
					}
				}
				for unicode.IsSpace(lexer.Lookahead()) {
					if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
						return false
					}
					lexer.Advance(true)
				}
				return true
			}
		}

		if isValid(scaTokElse) {
			return !scaScanWord(lexer, "else")
		}
		if isValid(scaTokCatch) && scaScanWord(lexer, "catch") {
			return false
		}
		if isValid(scaTokFinally) && scaScanWord(lexer, "finally") {
			return false
		}
		if isValid(scaTokExtends) && scaScanWord(lexer, "extends") {
			return false
		}
		if isValid(scaTokWith) && scaScanWord(lexer, "with") {
			return false
		}
		if isValid(scaTokDerives) && scaScanWord(lexer, "derives") {
			return false
		}

		return true
	}

	// Additional whitespace skip for string scanning
	for unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' {
			newlineCount++
		}
		lexer.Advance(true)
	}

	// Simple string start
	if isValid(scaTokSimpleStringStart) && lexer.Lookahead() == '"' {
		lexer.Advance(false)
		lexer.MarkEnd()
		if lexer.Lookahead() == '"' {
			lexer.Advance(false)
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				lexer.SetResultSymbol(scaSymSimpleMultiStringStart)
				lexer.MarkEnd()
				return true
			}
		}
		lexer.SetResultSymbol(scaSymSimpleStringStart)
		return true
	}

	// Raw string start: raw"
	if isValid(scaTokRawStringStart) && lexer.Lookahead() == 'r' {
		lexer.Advance(false)
		if lexer.Lookahead() == 'a' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'w' {
				lexer.Advance(false)
				if lexer.Lookahead() == '"' {
					lexer.MarkEnd()
					lexer.SetResultSymbol(scaSymRawStringStart)
					return true
				}
			}
		}
	}

	// String content scanning
	if isValid(scaTokSimpleStringMiddle) {
		return scaScanStringContent(lexer, false, scaStringSimple)
	}
	if isValid(scaTokInterpStringMiddle) {
		return scaScanStringContent(lexer, false, scaStringInterp)
	}
	if isValid(scaTokRawStringMiddle) {
		return scaScanStringContent(lexer, false, scaStringRaw)
	}
	if isValid(scaTokRawMultiStringMiddle) {
		return scaScanStringContent(lexer, true, scaStringRaw)
	}
	if isValid(scaTokInterpMultiStringMiddle) {
		return scaScanStringContent(lexer, true, scaStringInterp)
	}
	if isValid(scaTokMultilineStringEnd) {
		return scaScanStringContent(lexer, true, scaStringSimple)
	}

	return false
}

type scaStringMode int

const (
	scaStringSimple scaStringMode = iota
	scaStringInterp
	scaStringRaw
)

func scaScanStringContent(lexer *gotreesitter.ExternalLexer, isMultiline bool, mode scaStringMode) bool {
	closingQuotes := uint32(0)
	for {
		if lexer.Lookahead() == '"' {
			lexer.Advance(false)
			closingQuotes++
			if !isMultiline {
				lexer.SetResultSymbol(scaSymSingleLineStringEnd)
				lexer.MarkEnd()
				return true
			}
			if closingQuotes >= 3 && lexer.Lookahead() != '"' {
				lexer.SetResultSymbol(scaSymMultilineStringEnd)
				lexer.MarkEnd()
				return true
			}
		} else if lexer.Lookahead() == '$' && mode != scaStringSimple {
			switch mode {
			case scaStringInterp:
				if isMultiline {
					lexer.SetResultSymbol(scaSymInterpMultiStringMiddle)
				} else {
					lexer.SetResultSymbol(scaSymInterpStringMiddle)
				}
			case scaStringRaw:
				if isMultiline {
					lexer.SetResultSymbol(scaSymRawMultiStringMiddle)
				} else {
					lexer.SetResultSymbol(scaSymRawStringMiddle)
				}
			}
			lexer.MarkEnd()
			return true
		} else {
			closingQuotes = 0
			if lexer.Lookahead() == '\\' {
				if isMultiline || mode == scaStringRaw {
					lexer.Advance(false)
					if !isMultiline && mode == scaStringRaw &&
						(lexer.Lookahead() == '"' || lexer.Lookahead() == '\\') {
						lexer.Advance(false)
					}
				} else {
					if mode == scaStringSimple {
						lexer.SetResultSymbol(scaSymSimpleStringMiddle)
					} else {
						lexer.SetResultSymbol(scaSymInterpStringMiddle)
					}
					lexer.MarkEnd()
					return true
				}
			} else if lexer.Lookahead() == '\n' && !isMultiline {
				return false
			} else if lexer.Lookahead() == 0 {
				return false
			} else {
				lexer.Advance(false)
			}
		}
	}
}

func scaDetectCommentStart(lexer *gotreesitter.ExternalLexer) bool {
	lexer.MarkEnd()
	if lexer.Lookahead() == '/' {
		lexer.Advance(false)
		if lexer.Lookahead() == '/' || lexer.Lookahead() == '*' {
			return true
		}
	}
	return false
}

func scaScanWord(lexer *gotreesitter.ExternalLexer, word string) bool {
	for i := 0; i < len(word); i++ {
		if lexer.Lookahead() != rune(word[i]) {
			return false
		}
		lexer.Advance(false)
	}
	return !unicode.IsLetter(lexer.Lookahead()) && !unicode.IsDigit(lexer.Lookahead())
}
