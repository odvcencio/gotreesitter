//go:build !grammar_subset || grammar_subset_hack

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the hack grammar.
const (
	hackTokHeredocStart        = 0
	hackTokHeredocStartNewline = 1
	hackTokHeredocBody         = 2
	hackTokHeredocEndNewline   = 3
	hackTokHeredocEnd          = 4
	hackTokEmbeddedOpenBrace   = 5
)

const (
	hackSymHeredocStart        gotreesitter.Symbol = 182
	hackSymHeredocStartNewline gotreesitter.Symbol = 183
	hackSymHeredocBody         gotreesitter.Symbol = 184
	hackSymHeredocEndNewline   gotreesitter.Symbol = 185
	hackSymHeredocEnd          gotreesitter.Symbol = 186
	hackSymEmbeddedOpenBrace   gotreesitter.Symbol = 187
)

// hackState tracks the heredoc delimiter and parsing phase.
type hackState struct {
	delimiter []byte
	isNowdoc  bool
	didStart  bool
	didEnd    bool
}

// HackExternalScanner handles heredoc/nowdoc strings for Hack.
type HackExternalScanner struct{}

func (HackExternalScanner) Create() any {
	return &hackState{}
}

func (HackExternalScanner) Destroy(payload any) {}

func (HackExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*hackState)
	if len(s.delimiter)+3 >= len(buf) {
		return 0
	}
	n := 0
	if s.isNowdoc {
		buf[n] = 1
	} else {
		buf[n] = 0
	}
	n++
	if s.didStart {
		buf[n] = 1
	} else {
		buf[n] = 0
	}
	n++
	if s.didEnd {
		buf[n] = 1
	} else {
		buf[n] = 0
	}
	n++
	copy(buf[n:], s.delimiter)
	n += len(s.delimiter)
	return n
}

func (HackExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*hackState)
	if len(buf) == 0 {
		s.isNowdoc = false
		s.didStart = false
		s.didEnd = false
		s.delimiter = s.delimiter[:0]
	} else {
		s.isNowdoc = buf[0] != 0
		s.didStart = buf[1] != 0
		s.didEnd = buf[2] != 0
		s.delimiter = append(s.delimiter[:0], buf[3:]...)
	}
}

func (HackExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*hackState)

	if (hackValid(validSymbols, hackTokHeredocBody) || hackValid(validSymbols, hackTokHeredocEnd) ||
		hackValid(validSymbols, hackTokEmbeddedOpenBrace)) && len(s.delimiter) > 0 {
		return hackScanBody(s, lexer, validSymbols)
	}

	if hackValid(validSymbols, hackTokHeredocStart) {
		return hackScanStart(s, lexer)
	}

	return false
}

func hackScanStart(s *hackState, lexer *gotreesitter.ExternalLexer) bool {
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	s.isNowdoc = lexer.Lookahead() == '\''
	s.delimiter = s.delimiter[:0]

	var quote rune
	if s.isNowdoc || lexer.Lookahead() == '"' {
		quote = lexer.Lookahead()
		lexer.Advance(false)
	}

	if hackIsIdentStart(lexer.Lookahead()) {
		s.delimiter = append(s.delimiter, byte(lexer.Lookahead()))
		lexer.Advance(false)
		for hackIsIdentPart(lexer.Lookahead()) {
			s.delimiter = append(s.delimiter, byte(lexer.Lookahead()))
			lexer.Advance(false)
		}
	}

	if lexer.Lookahead() == quote {
		lexer.Advance(false)
	} else if quote != 0 {
		return false
	}

	if lexer.Lookahead() != '\n' || len(s.delimiter) == 0 {
		return false
	}

	lexer.SetResultSymbol(hackSymHeredocStart)
	lexer.MarkEnd()
	lexer.Advance(false) // consume \n

	// Check if the delimiter immediately follows
	if hackMatchDelimiter(s, lexer) {
		if lexer.Lookahead() == ';' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '\n' {
			s.didEnd = true
		}
	}

	return true
}

func hackScanBody(s *hackState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	didAdvance := false

	for {
		if lexer.Lookahead() == 0 {
			return false
		}

		// Handle escape sequences in heredocs
		if lexer.Lookahead() == '\\' {
			lexer.Advance(false)
			lexer.Advance(false)
			didAdvance = true
			continue
		}

		// Handle embedded variables/expressions in heredocs (not nowdocs)
		if (lexer.Lookahead() == '{' || lexer.Lookahead() == '$') && !s.isNowdoc {
			lexer.MarkEnd()

			if lexer.Lookahead() == '{' {
				lexer.Advance(false)
				if lexer.Lookahead() == '$' && !didAdvance {
					lexer.MarkEnd()
					lexer.Advance(false)
					if hackIsIdentStart(lexer.Lookahead()) {
						lexer.SetResultSymbol(hackSymEmbeddedOpenBrace)
						return true
					}
				}
			}

			if lexer.Lookahead() == '$' {
				lexer.Advance(false)
				if hackIsIdentStart(lexer.Lookahead()) {
					lexer.SetResultSymbol(hackSymHeredocBody)
					return didAdvance
				}
			}

			didAdvance = true
			continue
		}

		if s.didEnd || lexer.Lookahead() == '\n' {
			if didAdvance {
				lexer.MarkEnd()
				lexer.Advance(false)
			} else if lexer.Lookahead() == '\n' {
				if s.didEnd {
					lexer.Advance(true)
				} else {
					lexer.Advance(false)
					lexer.MarkEnd()
				}
			}

			if hackMatchDelimiter(s, lexer) {
				if !didAdvance && s.didEnd {
					lexer.MarkEnd()
				}

				if lexer.Lookahead() == ';' {
					lexer.Advance(false)
				}
				if lexer.Lookahead() == '\n' {
					if didAdvance {
						lexer.SetResultSymbol(hackSymHeredocBody)
						s.didStart = true
						s.didEnd = true
					} else if s.didEnd {
						lexer.SetResultSymbol(hackSymHeredocEnd)
						s.delimiter = s.delimiter[:0]
						s.isNowdoc = false
						s.didStart = false
						s.didEnd = false
					} else {
						lexer.SetResultSymbol(hackSymHeredocEndNewline)
						s.didStart = true
						s.didEnd = true
					}
					return true
				}
			} else if !s.didStart && !didAdvance {
				s.didStart = true
				lexer.SetResultSymbol(hackSymHeredocStartNewline)
				return true
			}

			didAdvance = true
			continue
		}

		lexer.Advance(false)
		didAdvance = true
	}
}

func hackMatchDelimiter(s *hackState, lexer *gotreesitter.ExternalLexer) bool {
	for i := 0; i < len(s.delimiter); i++ {
		if lexer.Lookahead() == rune(s.delimiter[i]) {
			lexer.Advance(false)
		} else {
			return false
		}
	}
	return true
}

func hackIsIdentStart(ch rune) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= 128 && ch <= 255)
}

func hackIsIdentPart(ch rune) bool {
	return hackIsIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func hackValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
