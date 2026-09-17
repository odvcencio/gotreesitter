//go:build !grammar_subset || grammar_subset_jsonnet

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the jsonnet grammar.
const (
	jsonnetTokStringStart   = 0
	jsonnetTokStringContent = 1
	jsonnetTokStringEnd     = 2
)

const (
	jsonnetSymStringStart   gotreesitter.Symbol = 61
	jsonnetSymStringContent gotreesitter.Symbol = 62
	jsonnetSymStringEnd     gotreesitter.Symbol = 63
)

// jsonnetState tracks whether we're inside a string and what delimiter ends it.
type jsonnetState struct {
	insideString bool
	endingChar   rune // 0 for ||| block strings, '\'' or '"' for quoted
	levelCount   uint8
}

// JsonnetExternalScanner handles Jsonnet string literals including
// single/double quoted strings and ||| block strings.
type JsonnetExternalScanner struct{}

func (JsonnetExternalScanner) Create() any         { return &jsonnetState{} }
func (JsonnetExternalScanner) Destroy(payload any) {}
func (JsonnetExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*jsonnetState)
	if s.insideString {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	buf[1] = byte(s.endingChar)
	buf[2] = s.levelCount
	return 3
}
func (JsonnetExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*jsonnetState)
	if len(buf) == 0 {
		return
	}
	s.insideString = buf[0] != 0
	if len(buf) >= 2 {
		s.endingChar = rune(buf[1])
	}
	if len(buf) >= 3 {
		s.levelCount = buf[2]
	}
}

func (JsonnetExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*jsonnetState)

	if s.insideString {
		if jsonnetValid(validSymbols, jsonnetTokStringEnd) && jsonnetScanStringEnd(s, lexer) {
			s.insideString = false
			s.endingChar = 0
			s.levelCount = 0
			lexer.SetResultSymbol(jsonnetSymStringEnd)
			return true
		}
		if jsonnetValid(validSymbols, jsonnetTokStringContent) && jsonnetScanStringContent(s, lexer) {
			lexer.SetResultSymbol(jsonnetSymStringContent)
			return true
		}
		return false
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if jsonnetValid(validSymbols, jsonnetTokStringStart) && jsonnetScanStringStart(s, lexer) {
		lexer.SetResultSymbol(jsonnetSymStringStart)
		return true
	}

	return false
}

func jsonnetScanBlockStart(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	return true
}

func jsonnetScanBlockEnd(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	return true
}

func jsonnetScanStringStart(s *jsonnetState, lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	if ch == '"' || ch == '\'' {
		s.insideString = true
		s.endingChar = ch
		lexer.Advance(false)
		return true
	}
	if jsonnetScanBlockStart(lexer) {
		s.insideString = true
		s.endingChar = 0
		return true
	}
	return false
}

func jsonnetScanStringEnd(s *jsonnetState, lexer *gotreesitter.ExternalLexer) bool {
	if s.endingChar == 0 {
		return jsonnetScanBlockEnd(lexer)
	}
	if lexer.Lookahead() == s.endingChar {
		lexer.Advance(false)
		return true
	}
	return false
}

func jsonnetScanStringContent(s *jsonnetState, lexer *gotreesitter.ExternalLexer) bool {
	if s.endingChar == 0 {
		return jsonnetScanBlockContent(lexer)
	}

	for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 && lexer.Lookahead() != s.endingChar {
		if lexer.Lookahead() == '\\' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'z' {
				lexer.Advance(false)
				for unicode.IsSpace(lexer.Lookahead()) {
					lexer.Advance(false)
				}
				continue
			}
		}
		if lexer.Lookahead() == 0 {
			return true
		}
		lexer.Advance(false)
	}
	return true
}

func jsonnetScanBlockContent(lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '|' {
			lexer.MarkEnd()
			if jsonnetScanBlockEnd(lexer) {
				return true
			}
		} else {
			lexer.Advance(false)
		}
	}
	return false
}

func jsonnetValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
