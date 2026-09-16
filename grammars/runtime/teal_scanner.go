//go:build !grammar_subset || grammar_subset_teal

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the teal grammar.
const (
	tealTokComment         = 0
	tealTokLongStringStart = 1
	tealTokLongStringChar  = 2
	tealTokLongStringEnd   = 3
	tealTokShortStrStart   = 4
	tealTokShortStrChar    = 5
	tealTokShortStrEnd     = 6
)

const (
	tealSymComment         gotreesitter.Symbol = 76
	tealSymLongStringStart gotreesitter.Symbol = 77
	tealSymLongStringChar  gotreesitter.Symbol = 78
	tealSymLongStringEnd   gotreesitter.Symbol = 79
	tealSymShortStrStart   gotreesitter.Symbol = 80
	tealSymShortStrChar    gotreesitter.Symbol = 81
	tealSymShortStrEnd     gotreesitter.Symbol = 82
)

// tealState tracks Lua-style string parsing state.
type tealState struct {
	openingEqs   uint32
	inStr        bool
	openingQuote rune
}

// TealExternalScanner handles Teal/Lua string and comment scanning.
type TealExternalScanner struct{}

func (TealExternalScanner) Create() any         { return &tealState{} }
func (TealExternalScanner) Destroy(payload any) {}

func (TealExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*tealState)
	if len(buf) < 6 {
		return 0
	}
	buf[0] = byte(s.openingEqs)
	buf[1] = byte(s.openingEqs >> 8)
	buf[2] = byte(s.openingEqs >> 16)
	buf[3] = byte(s.openingEqs >> 24)
	if s.inStr {
		buf[4] = 1
	} else {
		buf[4] = 0
	}
	buf[5] = byte(s.openingQuote)
	return 6
}

func (TealExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*tealState)
	*s = tealState{}
	if len(buf) >= 6 {
		s.openingEqs = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
		s.inStr = buf[4] != 0
		s.openingQuote = rune(buf[5])
	}
}

func (TealExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*tealState)

	if lexer.Lookahead() == 0 {
		return false
	}

	// Inside a string, handle end/char
	if s.inStr {
		if s.openingQuote > 0 {
			// Short string mode
			if tealValid(validSymbols, tealTokShortStrEnd) && lexer.Lookahead() == s.openingQuote {
				lexer.Advance(false)
				lexer.SetResultSymbol(tealSymShortStrEnd)
				*s = tealState{}
				return true
			}
			if tealValid(validSymbols, tealTokShortStrChar) {
				ch := lexer.Lookahead()
				if ch != s.openingQuote && ch != '\n' && ch != '\r' && ch != '\\' && ch != '%' {
					lexer.Advance(false)
					lexer.SetResultSymbol(tealSymShortStrChar)
					return true
				}
			}
			return false
		}

		// Long string mode
		if lexer.Lookahead() == ']' {
			lexer.Advance(false)
			eqs := tealConsumeEqs(lexer)
			if s.openingEqs == eqs && lexer.Lookahead() == ']' {
				lexer.Advance(false)
				lexer.SetResultSymbol(tealSymLongStringEnd)
				*s = tealState{}
				return true
			}
		}
		// Long string char (not %)
		if lexer.Lookahead() == '%' {
			return false
		}
		lexer.Advance(false)
		lexer.SetResultSymbol(tealSymLongStringChar)
		return true
	}

	// Skip whitespace
	for tealIsASCIIWhitespace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Short string start
	if tealValid(validSymbols, tealTokShortStrStart) {
		if lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' {
			s.openingQuote = lexer.Lookahead()
			s.inStr = true
			lexer.Advance(false)
			lexer.SetResultSymbol(tealSymShortStrStart)
			return true
		}
	}

	// Long string start: [=*[
	if tealValid(validSymbols, tealTokLongStringStart) {
		if lexer.Lookahead() == '[' {
			lexer.Advance(false)
			*s = tealState{}
			eqs := tealConsumeEqs(lexer)
			if lexer.Lookahead() == '[' {
				lexer.Advance(false)
				s.inStr = true
				s.openingEqs = eqs
				lexer.SetResultSymbol(tealSymLongStringStart)
				return true
			}
			return false
		}
	}

	// Comment: -- followed by optional [=*[ for long comment
	if tealValid(validSymbols, tealTokComment) {
		return tealScanComment(lexer)
	}

	return false
}

func tealScanComment(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)

	lexer.SetResultSymbol(tealSymComment)

	// Check for long comment --[=*[
	if lexer.Lookahead() != '[' {
		tealConsumeRestOfLine(lexer)
		return true
	}
	lexer.Advance(false)
	eqs := tealConsumeEqs(lexer)

	if lexer.Lookahead() != '[' {
		tealConsumeRestOfLine(lexer)
		return true
	}

	// Long comment: consume until ]=*]
	for lexer.Lookahead() != 0 {
		for lexer.Lookahead() != 0 && lexer.Lookahead() != ']' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() != ']' {
			return true
		}
		lexer.Advance(false)
		testEqs := tealConsumeEqs(lexer)
		if lexer.Lookahead() == ']' {
			lexer.Advance(false)
			if testEqs == eqs {
				return true
			}
		} else if lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}
	}

	return true
}

func tealConsumeEqs(lexer *gotreesitter.ExternalLexer) uint32 {
	var count uint32
	for lexer.Lookahead() == '=' {
		lexer.Advance(false)
		count++
	}
	return count
}

func tealConsumeRestOfLine(lexer *gotreesitter.ExternalLexer) {
	for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' {
		lexer.Advance(false)
	}
}

func tealIsASCIIWhitespace(ch rune) bool {
	return ch == '\n' || ch == '\r' || ch == ' ' || ch == '\t'
}

func tealValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
