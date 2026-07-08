//go:build !grammar_subset || grammar_subset_earthfile

package grammars

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the earthfile grammar.
const (
	earthfileTokIndent = 0
	earthfileTokDedent = 1
)

const (
	earthfileSymIndent gotreesitter.Symbol = 151
	earthfileSymDedent gotreesitter.Symbol = 152
)

// earthfileState tracks indent level for Earthfile parsing.
type earthfileState struct {
	prevIndent uint32
	hasSeenEof bool
}

// EarthfileExternalScanner handles indent/dedent for Earthfile.
type EarthfileExternalScanner struct{}

func (EarthfileExternalScanner) Create() any         { return &earthfileState{} }
func (EarthfileExternalScanner) Destroy(payload any) {}

func (EarthfileExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*earthfileState)
	if len(buf) < 5 {
		return 0
	}
	buf[0] = byte(s.prevIndent)
	buf[1] = byte(s.prevIndent >> 8)
	buf[2] = byte(s.prevIndent >> 16)
	buf[3] = byte(s.prevIndent >> 24)
	if s.hasSeenEof {
		buf[4] = 1
	} else {
		buf[4] = 0
	}
	return 5
}

func (EarthfileExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*earthfileState)
	s.prevIndent = 0
	s.hasSeenEof = false
	if len(buf) >= 5 {
		s.prevIndent = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
		s.hasSeenEof = buf[4] != 0
	}
}

func (EarthfileExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*earthfileState)

	// EOF acts as dedent
	if lexer.Lookahead() == 0 {
		return earthfileHandleEof(lexer, s, validSymbols)
	}

	if earthfileValid(validSymbols, earthfileTokIndent) || earthfileValid(validSymbols, earthfileTokDedent) {
		// Skip whitespace
		for lexer.Lookahead() != 0 && earthfileIsSpace(lexer.Lookahead()) {
			switch lexer.Lookahead() {
			case '\n', '\r', '\f':
				lexer.Advance(false)
			case '\t', ' ':
				lexer.Advance(true)
			}
		}

		if lexer.Lookahead() == 0 {
			return earthfileHandleEof(lexer, s, validSymbols)
		}

		indent := lexer.Column()
		if indent > s.prevIndent && earthfileValid(validSymbols, earthfileTokIndent) && s.prevIndent == 0 {
			lexer.SetResultSymbol(earthfileSymIndent)
			s.prevIndent = indent
			return true
		}
		if indent < s.prevIndent && earthfileValid(validSymbols, earthfileTokDedent) && indent == 0 {
			lexer.SetResultSymbol(earthfileSymDedent)
			s.prevIndent = indent
			return true
		}
	}

	return false
}

func earthfileHandleEof(lexer *gotreesitter.ExternalLexer, s *earthfileState, validSymbols []bool) bool {
	if s.hasSeenEof {
		return false
	}
	lexer.MarkEnd()
	if earthfileValid(validSymbols, earthfileTokDedent) && s.prevIndent != 0 {
		lexer.SetResultSymbol(earthfileSymDedent)
		s.hasSeenEof = true
		return true
	}
	return false
}

func earthfileIsSpace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
}

func earthfileValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
