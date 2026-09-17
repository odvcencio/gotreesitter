//go:build !grammar_subset || grammar_subset_php

package grammarruntime

import (
	"encoding/binary"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the PHP grammar.
const (
	phpTokAutoSemicolon                   = 0
	phpTokEncapsedStringChars             = 1
	phpTokEncapsedStringCharsAfterVar     = 2
	phpTokExecutionStringChars            = 3
	phpTokExecutionStringCharsAfterVar    = 4
	phpTokEncapsedStringCharsHeredoc      = 5
	phpTokEncapsedStringCharsAfterVarHdoc = 6
	phpTokEOF                             = 7
	phpTokHeredocStart                    = 8
	phpTokHeredocEnd                      = 9
	phpTokNowdocString                    = 10
	phpTokSentinelError                   = 11
)

const (
	phpSymAutoSemicolon                   gotreesitter.Symbol = 185
	phpSymEncapsedStringChars             gotreesitter.Symbol = 186
	phpSymEncapsedStringCharsAfterVar     gotreesitter.Symbol = 187
	phpSymExecutionStringChars            gotreesitter.Symbol = 188
	phpSymExecutionStringCharsAfterVar    gotreesitter.Symbol = 189
	phpSymEncapsedStringCharsHeredoc      gotreesitter.Symbol = 190
	phpSymEncapsedStringCharsAfterVarHdoc gotreesitter.Symbol = 191
	phpSymEOF                             gotreesitter.Symbol = 192
	phpSymHeredocStart                    gotreesitter.Symbol = 193
	phpSymHeredocEnd                      gotreesitter.Symbol = 194
	phpSymNowdocString                    gotreesitter.Symbol = 195
)

type phpHeredoc struct {
	endWordIndentAllowed bool
	word                 []rune
}

type phpState struct {
	heredocs []phpHeredoc
}

// PhpExternalScanner handles heredocs, encapsed strings, auto-semicolons, and EOF for PHP.
type PhpExternalScanner struct{}

func (PhpExternalScanner) Create() any {
	return &phpState{}
}
func (PhpExternalScanner) Destroy(payload any) {}

func (PhpExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*phpState)
	size := 0
	if len(buf) == 0 {
		return 0
	}
	buf[size] = byte(len(s.heredocs))
	size++
	for j := range s.heredocs {
		hd := &s.heredocs[j]
		wordBytes := len(hd.word) * 4
		if size+5+wordBytes >= len(buf) {
			return 0
		}
		if hd.endWordIndentAllowed {
			buf[size] = 1
		} else {
			buf[size] = 0
		}
		size++
		binary.LittleEndian.PutUint32(buf[size:], uint32(len(hd.word)))
		size += 4
		for _, ch := range hd.word {
			binary.LittleEndian.PutUint32(buf[size:], uint32(ch))
			size += 4
		}
	}
	return size
}

func (PhpExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*phpState)
	s.heredocs = s.heredocs[:0]

	if len(buf) == 0 {
		return
	}
	size := 0
	hdCount := int(buf[size])
	size++
	for i := 0; i < hdCount && size < len(buf); i++ {
		hd := phpHeredoc{}
		hd.endWordIndentAllowed = buf[size] != 0
		size++
		if size+4 > len(buf) {
			break
		}
		wordLen := int(binary.LittleEndian.Uint32(buf[size:]))
		size += 4
		hd.word = make([]rune, 0, wordLen)
		for j := 0; j < wordLen && size+4 <= len(buf); j++ {
			ch := int32(binary.LittleEndian.Uint32(buf[size:]))
			hd.word = append(hd.word, rune(ch))
			size += 4
		}
		s.heredocs = append(s.heredocs, hd)
	}
}

func (PhpExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*phpState)

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	if isValid(phpTokSentinelError) {
		return false
	}

	lexer.MarkEnd()

	if isValid(phpTokEncapsedStringCharsAfterVar) {
		lexer.SetResultSymbol(phpSymEncapsedStringCharsAfterVar)
		return phpScanEncapsed(s, lexer, true, false, false)
	}

	if isValid(phpTokEncapsedStringChars) {
		lexer.SetResultSymbol(phpSymEncapsedStringChars)
		return phpScanEncapsed(s, lexer, false, false, false)
	}

	if isValid(phpTokExecutionStringCharsAfterVar) {
		lexer.SetResultSymbol(phpSymExecutionStringCharsAfterVar)
		return phpScanEncapsed(s, lexer, true, false, true)
	}

	if isValid(phpTokExecutionStringChars) {
		lexer.SetResultSymbol(phpSymExecutionStringChars)
		return phpScanEncapsed(s, lexer, false, false, true)
	}

	if isValid(phpTokEncapsedStringCharsAfterVarHdoc) {
		lexer.SetResultSymbol(phpSymEncapsedStringCharsAfterVarHdoc)
		return phpScanEncapsed(s, lexer, true, true, false)
	}

	if isValid(phpTokEncapsedStringCharsHeredoc) {
		lexer.SetResultSymbol(phpSymEncapsedStringCharsHeredoc)
		return phpScanEncapsed(s, lexer, false, true, false)
	}

	if isValid(phpTokNowdocString) {
		lexer.SetResultSymbol(phpSymNowdocString)
		return phpScanNowdoc(s, lexer)
	}

	if isValid(phpTokHeredocEnd) {
		lexer.SetResultSymbol(phpSymHeredocEnd)
		if len(s.heredocs) == 0 {
			return false
		}
		hd := s.heredocs[len(s.heredocs)-1]

		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}

		word := phpScanHeredocWord(lexer)
		if !phpRuneSliceEqual(word, hd.word) {
			return false
		}

		lexer.MarkEnd()
		s.heredocs = s.heredocs[:len(s.heredocs)-1]
		return true
	}

	if !phpSkipWhitespace(lexer) {
		return false
	}

	if isValid(phpTokEOF) && lexer.Lookahead() == 0 {
		lexer.SetResultSymbol(phpSymEOF)
		return true
	}

	if isValid(phpTokHeredocStart) {
		lexer.SetResultSymbol(phpSymHeredocStart)
		hd := phpHeredoc{}

		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}

		hd.word = phpScanHeredocWord(lexer)
		if len(hd.word) == 0 {
			return false
		}
		lexer.MarkEnd()

		s.heredocs = append(s.heredocs, hd)
		return true
	}

	if isValid(phpTokAutoSemicolon) {
		lexer.SetResultSymbol(phpSymAutoSemicolon)
		if lexer.Lookahead() != '?' {
			return false
		}
		lexer.Advance(false)
		return lexer.Lookahead() == '>'
	}

	return false
}

func phpIsValidNameChar(lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch >= 0x80
}

func phpIsEscapableSequence(lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	if ch == 'n' || ch == 'r' || ch == 't' || ch == 'v' || ch == 'e' || ch == 'f' ||
		ch == '\\' || ch == '$' || ch == '"' {
		return true
	}
	if ch == 'x' {
		lexer.Advance(false)
		return phpIsHexDigit(lexer.Lookahead())
	}
	if ch == 'u' {
		return true
	}
	return ch >= '0' && ch <= '7'
}

func phpIsHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func phpSkipWhitespace(lexer *gotreesitter.ExternalLexer) bool {
	for {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '/' {
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				lexer.Advance(false)
				for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
					lexer.Advance(false)
				}
			} else {
				return false
			}
		} else {
			return true
		}
	}
}

func phpScanHeredocWord(lexer *gotreesitter.ExternalLexer) []rune {
	var result []rune
	for phpIsValidNameChar(lexer) {
		result = append(result, lexer.Lookahead())
		lexer.Advance(false)
	}
	return result
}

func phpRuneSliceEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func phpScanNowdoc(s *phpState, lexer *gotreesitter.ExternalLexer) bool {
	hasConsumed := false
	if len(s.heredocs) == 0 {
		return false
	}

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(false)
		hasConsumed = true
	}

	tag := s.heredocs[len(s.heredocs)-1].word
	endTagMatched := false
	for i := 0; i < len(tag); i++ {
		if lexer.Lookahead() != tag[i] {
			break
		}
		lexer.Advance(false)
		hasConsumed = true
		if i == len(tag)-1 {
			la := lexer.Lookahead()
			if unicode.IsSpace(la) || la == ';' || la == ',' || la == ')' {
				endTagMatched = true
			}
		}
	}

	if endTagMatched {
		for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' {
			lexer.Advance(false)
			hasConsumed = true
		}
		la := lexer.Lookahead()
		if la == ';' || la == ',' || la == ')' || la == '\n' || la == '\r' {
			return false
		}
	}

	hasContent := hasConsumed
	for {
		lexer.MarkEnd()
		switch lexer.Lookahead() {
		case '\n', '\r':
			return hasContent
		default:
			if lexer.Lookahead() == 0 {
				return false
			}
			lexer.Advance(false)
		}
		hasContent = true
	}
}

func phpScanEncapsed(s *phpState, lexer *gotreesitter.ExternalLexer, isAfterVariable, isHeredoc, isExecution bool) bool {
	hasConsumed := false

	if isHeredoc && len(s.heredocs) > 0 {
		for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' {
			lexer.Advance(false)
			hasConsumed = true
		}

		tag := s.heredocs[len(s.heredocs)-1].word
		endTagMatched := false
		for i := 0; i < len(tag); i++ {
			if lexer.Lookahead() != tag[i] {
				break
			}
			hasConsumed = true
			lexer.Advance(false)
			if i == len(tag)-1 {
				la := lexer.Lookahead()
				if unicode.IsSpace(la) || la == ';' || la == ',' || la == ')' {
					endTagMatched = true
				}
			}
		}

		if endTagMatched {
			for unicode.IsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' {
				lexer.Advance(false)
				hasConsumed = true
			}
			la := lexer.Lookahead()
			if la == ';' || la == ',' || la == ')' || la == '\n' || la == '\r' {
				return false
			}
		}
	}

	hasContent := hasConsumed
	afterVar := isAfterVariable
	for {
		lexer.MarkEnd()

		switch lexer.Lookahead() {
		case '"':
			if !isHeredoc && !isExecution {
				return hasContent
			}
			lexer.Advance(false)
		case '`':
			if isExecution {
				return hasContent
			}
			lexer.Advance(false)
		case '\n', '\r':
			if isHeredoc {
				return hasContent
			}
			lexer.Advance(false)
		case '\\':
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				lexer.Advance(false)
			} else if isExecution && lexer.Lookahead() == '`' {
				return hasContent
			} else if isHeredoc && lexer.Lookahead() == '\\' {
				lexer.Advance(false)
			} else if phpIsEscapableSequence(lexer) {
				return hasContent
			}
		case '$':
			lexer.Advance(false)
			if (phpIsValidNameChar(lexer) && !unicode.IsDigit(lexer.Lookahead())) || lexer.Lookahead() == '{' {
				return hasContent
			}
		case '-':
			if afterVar {
				lexer.Advance(false)
				if lexer.Lookahead() == '>' {
					lexer.Advance(false)
					if phpIsValidNameChar(lexer) {
						return hasContent
					}
				}
			} else {
				// C fallthrough to '[' case: when not afterVar, just advance
				lexer.Advance(false)
			}
		case '[':
			if afterVar {
				return hasContent
			}
			lexer.Advance(false)
		case '{':
			lexer.Advance(false)
			if lexer.Lookahead() == '$' {
				return hasContent
			}
		default:
			if lexer.Lookahead() == 0 {
				return false
			}
			lexer.Advance(false)
		}

		afterVar = false
		hasContent = true
	}
}
