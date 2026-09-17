//go:build !grammar_subset || grammar_subset_nickel

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the nickel grammar.
const (
	nickelTokMultstrStart       = 0
	nickelTokMultstrEnd         = 1
	nickelTokStrStart           = 2
	nickelTokStrEnd             = 3
	nickelTokInterpStart        = 4
	nickelTokInterpEnd          = 5
	nickelTokQuotedEnumTagStart = 6
	nickelTokComment            = 7
)

const (
	nickelSymMultstrStart       gotreesitter.Symbol = 74
	nickelSymMultstrEnd         gotreesitter.Symbol = 75
	nickelSymStrStart           gotreesitter.Symbol = 76
	nickelSymStrEnd             gotreesitter.Symbol = 77
	nickelSymInterpStart        gotreesitter.Symbol = 78
	nickelSymInterpEnd          gotreesitter.Symbol = 79
	nickelSymQuotedEnumTagStart gotreesitter.Symbol = 80
	nickelSymComment            gotreesitter.Symbol = 81
)

// nickelState tracks the percent count stack for nested strings.
type nickelState struct {
	expectedPercentCount []uint8
}

// NickelExternalScanner handles Nickel's multi-line strings, interpolation, and comments.
type NickelExternalScanner struct{}

func (NickelExternalScanner) Create() any         { return &nickelState{} }
func (NickelExternalScanner) Destroy(payload any) {}

func (NickelExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*nickelState)
	if len(s.expectedPercentCount)+1 > len(buf) {
		return 0
	}
	l := len(s.expectedPercentCount)
	if l > 255 {
		l = 255
	}
	n := 0
	buf[n] = byte(l)
	n++
	for i := 0; i < l; i++ {
		buf[n] = s.expectedPercentCount[i]
		n++
	}
	return n
}

func (NickelExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*nickelState)
	s.expectedPercentCount = s.expectedPercentCount[:0]
	if len(buf) > 0 {
		vecLen := int(buf[0])
		for i := 1; i <= vecLen && i < len(buf); i++ {
			s.expectedPercentCount = append(s.expectedPercentCount, buf[i])
		}
	}
}

func (NickelExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*nickelState)

	// Error recovery guard
	if nickelValid(validSymbols, nickelTokMultstrStart) && nickelValid(validSymbols, nickelTokMultstrEnd) &&
		nickelValid(validSymbols, nickelTokStrStart) && nickelValid(validSymbols, nickelTokStrEnd) &&
		nickelValid(validSymbols, nickelTokInterpStart) && nickelValid(validSymbols, nickelTokInterpEnd) &&
		nickelValid(validSymbols, nickelTokComment) && nickelValid(validSymbols, nickelTokQuotedEnumTagStart) {
		return false
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	switch lexer.Lookahead() {
	case '"':
		if nickelValid(validSymbols, nickelTokMultstrEnd) {
			lexer.Advance(false)
			if lexer.Lookahead() == '%' {
				return nickelScanMultstrEnd(s, lexer)
			}
		} else if nickelValid(validSymbols, nickelTokStrStart) {
			return nickelScanStrStart(s, lexer)
		} else if nickelValid(validSymbols, nickelTokStrEnd) {
			return nickelScanStrEnd(s, lexer)
		}
	case '%':
		if nickelValid(validSymbols, nickelTokInterpStart) {
			return nickelScanInterpStart(s, lexer)
		}
	case '}':
		if nickelValid(validSymbols, nickelTokInterpEnd) {
			lexer.Advance(false)
			lexer.SetResultSymbol(nickelSymInterpEnd)
			return true
		}
	case '\'':
		if nickelValid(validSymbols, nickelTokQuotedEnumTagStart) {
			lexer.Advance(false)
			if lexer.Lookahead() == '"' {
				return nickelScanQuotedEnumTagStart(s, lexer)
			}
		}
	case '#':
		if nickelValid(validSymbols, nickelTokComment) {
			return nickelScanComment(s, lexer)
		}
	default:
		if nickelValid(validSymbols, nickelTokMultstrStart) {
			return nickelScanMultstrStart(s, lexer)
		}
	}

	return false
}

func nickelScanMultstrStart(s *nickelState, lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nickelSymMultstrStart)
	mScanned := false

	if lexer.Lookahead() == 'm' {
		lexer.Advance(false)
		mScanned = true
	}

	if mScanned && lexer.Lookahead() == '%' {
		lexer.Advance(false)
	} else if !nickelScanUntilSstrStartEnd(lexer, mScanned) {
		return false
	}

	// Count % signs (starting at 1 since we already consumed one)
	count := uint8(1)
	for lexer.Lookahead() == '%' {
		count++
		lexer.Advance(false)
	}

	quote := false
	if lexer.Lookahead() == '"' {
		quote = true
		lexer.Advance(false)
	}

	s.expectedPercentCount = append(s.expectedPercentCount, count)
	return quote
}

func nickelScanMultstrEnd(s *nickelState, lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nickelSymMultstrEnd)
	if len(s.expectedPercentCount) == 0 {
		return false
	}
	count := s.expectedPercentCount[len(s.expectedPercentCount)-1]

	for lexer.Lookahead() == '%' && count > 0 {
		count--
		lexer.Advance(false)
	}

	s.expectedPercentCount = s.expectedPercentCount[:len(s.expectedPercentCount)-1]
	return count == 0 && lexer.Lookahead() != '{'
}

func nickelScanStrStart(s *nickelState, lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nickelSymStrStart)
	s.expectedPercentCount = append(s.expectedPercentCount, 1)
	lexer.Advance(false)
	return true
}

func nickelScanStrEnd(s *nickelState, lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nickelSymStrEnd)
	lexer.Advance(false)
	if len(s.expectedPercentCount) > 0 {
		s.expectedPercentCount = s.expectedPercentCount[:len(s.expectedPercentCount)-1]
	}
	return true
}

func nickelScanInterpStart(s *nickelState, lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nickelSymInterpStart)
	if len(s.expectedPercentCount) == 0 {
		return false
	}
	count := s.expectedPercentCount[len(s.expectedPercentCount)-1]
	if count == 0 {
		return false // no interpolation allowed
	}

	for lexer.Lookahead() == '%' {
		count--
		lexer.Advance(false)
	}

	brace := false
	if lexer.Lookahead() == '{' {
		brace = true
		lexer.Advance(false)
	}

	return brace && count == 0
}

func nickelScanQuotedEnumTagStart(s *nickelState, lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nickelSymQuotedEnumTagStart)
	// 0 = no interpolation allowed
	s.expectedPercentCount = append(s.expectedPercentCount, 0)
	lexer.Advance(false)
	return true
}

func nickelScanComment(s *nickelState, lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nickelSymComment)
	// Only allow comments outside strings
	if len(s.expectedPercentCount) > 0 {
		return false
	}
	lexer.Advance(false)
	for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
		lexer.Advance(false)
	}
	return true
}

// nickelScanUntilSstrStartEnd recognizes a symbolic string prefix like "tag-s%".
func nickelScanUntilSstrStartEnd(lexer *gotreesitter.ExternalLexer, mScanned bool) bool {
	const (
		stStart   = 0
		stMiddle  = 1
		stDash    = 2
		stS       = 3
		stPercent = 4
	)
	state := stStart
	if mScanned {
		state = stMiddle
	}
	for lexer.Lookahead() != 0 {
		ch := lexer.Lookahead()
		switch state {
		case stStart:
			if nickelIsSymtagStart(ch) {
				lexer.Advance(false)
				state = stMiddle
			} else {
				return false
			}
		case stMiddle:
			if !nickelIsSymtagMiddle(ch) {
				return false
			}
			if ch == '-' {
				state = stDash
			}
			lexer.Advance(false)
		case stDash:
			if ch == 's' {
				state = stS
				lexer.Advance(false)
			} else {
				state = stMiddle
			}
		case stS:
			if ch == '%' {
				state = stPercent
				lexer.Advance(false)
			} else {
				state = stMiddle
			}
		case stPercent:
			return true
		}
	}
	return false
}

func nickelIsSymtagStart(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func nickelIsSymtagMiddle(ch rune) bool {
	return nickelIsSymtagStart(ch) || (ch >= '0' && ch <= '9') || ch == '-' || ch == '\'' || ch == '_'
}

func nickelValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
