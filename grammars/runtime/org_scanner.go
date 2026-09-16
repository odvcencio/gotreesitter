//go:build !grammar_subset || grammar_subset_org

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the org grammar.
const (
	orgTokListStart   = 0
	orgTokListEnd     = 1
	orgTokListItemEnd = 2
	orgTokBullet      = 3
	orgTokHlStars     = 4
	orgTokSectionEnd  = 5
	orgTokEndOfFile   = 6
)

const (
	orgSymListStart   gotreesitter.Symbol = 117
	orgSymListEnd     gotreesitter.Symbol = 118
	orgSymListItemEnd gotreesitter.Symbol = 119
	orgSymBullet      gotreesitter.Symbol = 120
	orgSymHlStars     gotreesitter.Symbol = 121
	orgSymSectionEnd  gotreesitter.Symbol = 122
	orgSymEndOfFile   gotreesitter.Symbol = 123
)

// org bullet types
const (
	orgBulletNone       = 0
	orgBulletDash       = 1
	orgBulletPlus       = 2
	orgBulletStar       = 3
	orgBulletLowerDot   = 4
	orgBulletUpperDot   = 5
	orgBulletLowerParen = 6
	orgBulletUpperParen = 7
	orgBulletNumDot     = 8
	orgBulletNumParen   = 9
)

// orgState tracks indent/bullet/section stacks for org mode.
type orgState struct {
	indents  []int16
	bullets  []int16
	sections []int16
}

// OrgExternalScanner handles list/section/headline detection for org mode.
type OrgExternalScanner struct{}

func (OrgExternalScanner) Create() any {
	return &orgState{
		indents:  []int16{-1},
		bullets:  []int16{orgBulletNone},
		sections: []int16{0},
	}
}

func (OrgExternalScanner) Destroy(payload any) {}

func (OrgExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*orgState)
	n := 0
	indentCount := len(s.indents) - 1
	if indentCount > 255 {
		indentCount = 255
	}
	if n >= len(buf) {
		return 0
	}
	buf[n] = byte(indentCount)
	n++
	for i := 1; i <= indentCount && n < len(buf); i++ {
		buf[n] = byte(s.indents[i])
		n++
	}
	for i := 1; i <= indentCount && n < len(buf); i++ {
		buf[n] = byte(s.bullets[i])
		n++
	}
	for i := 1; i < len(s.sections) && n < len(buf); i++ {
		buf[n] = byte(s.sections[i])
		n++
	}
	return n
}

func (OrgExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*orgState)
	s.sections = s.sections[:0]
	s.sections = append(s.sections, 0)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, -1)
	s.bullets = s.bullets[:0]
	s.bullets = append(s.bullets, orgBulletNone)

	if len(buf) == 0 {
		return
	}

	i := 0
	indentCount := int(buf[i])
	i++
	for ; i <= indentCount && i < len(buf); i++ {
		s.indents = append(s.indents, int16(buf[i]))
	}
	for ; i <= 2*indentCount && i < len(buf); i++ {
		s.bullets = append(s.bullets, int16(buf[i]))
	}
	for ; i < len(buf); i++ {
		s.sections = append(s.sections, int16(buf[i]))
	}
}

func (OrgExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*orgState)

	if orgInErrorRecovery(validSymbols) {
		return false
	}

	indentLength := int16(0)
	lexer.MarkEnd()

	// Scan initial whitespace
	for {
		ch := lexer.Lookahead()
		if ch == ' ' {
			indentLength++
		} else if ch == '\t' {
			indentLength += 8
		} else if ch == 0 {
			if orgValid(validSymbols, orgTokListEnd) {
				lexer.SetResultSymbol(orgSymListEnd)
			} else if orgValid(validSymbols, orgTokSectionEnd) {
				lexer.SetResultSymbol(orgSymSectionEnd)
			} else if orgValid(validSymbols, orgTokEndOfFile) {
				lexer.SetResultSymbol(orgSymEndOfFile)
			} else {
				return false
			}
			return true
		} else {
			break
		}
		lexer.Advance(true)
	}

	// List end/item end
	newlines := int16(0)
	if orgValid(validSymbols, orgTokListEnd) || orgValid(validSymbols, orgTokListItemEnd) {
		for {
			ch := lexer.Lookahead()
			if ch == ' ' {
				indentLength++
			} else if ch == '\t' {
				indentLength += 8
			} else if ch == 0 {
				return orgDedent(s, lexer)
			} else if ch == '\n' {
				newlines++
				if newlines > 1 {
					return orgDedent(s, lexer)
				}
				indentLength = 0
			} else {
				break
			}
			lexer.Advance(true)
		}

		back := s.indents[len(s.indents)-1]
		if indentLength < back {
			return orgDedent(s, lexer)
		} else if indentLength == back {
			bullet := orgGetBullet(lexer)
			if bullet == s.bullets[len(s.bullets)-1] {
				lexer.SetResultSymbol(orgSymListItemEnd)
				return true
			}
			return orgDedent(s, lexer)
		}
	}

	// Headlines (stars at column 0)
	if indentLength == 0 && lexer.Lookahead() == '*' {
		lexer.MarkEnd()
		stars := int16(1)
		lexer.Advance(true)
		for lexer.Lookahead() == '*' {
			stars++
			lexer.Advance(true)
		}

		if orgValid(validSymbols, orgTokSectionEnd) && unicode.IsSpace(lexer.Lookahead()) &&
			stars > 0 && stars <= s.sections[len(s.sections)-1] {
			s.sections = s.sections[:len(s.sections)-1]
			lexer.SetResultSymbol(orgSymSectionEnd)
			return true
		} else if orgValid(validSymbols, orgTokHlStars) && unicode.IsSpace(lexer.Lookahead()) {
			s.sections = append(s.sections, stars)
			lexer.SetResultSymbol(orgSymHlStars)
			return true
		}
		return false
	}

	// List start and bullets
	if (orgValid(validSymbols, orgTokListStart) || orgValid(validSymbols, orgTokBullet)) && newlines == 0 {
		bullet := orgGetBullet(lexer)
		back := s.indents[len(s.indents)-1]
		bulletBack := s.bullets[len(s.bullets)-1]

		if orgValid(validSymbols, orgTokBullet) && bullet == bulletBack && indentLength == back {
			lexer.MarkEnd()
			lexer.SetResultSymbol(orgSymBullet)
			return true
		} else if orgValid(validSymbols, orgTokListStart) && bullet != orgBulletNone && indentLength > back {
			s.indents = append(s.indents, indentLength)
			s.bullets = append(s.bullets, int16(bullet))
			lexer.SetResultSymbol(orgSymListStart)
			return true
		}
	}

	return false
}

func orgDedent(s *orgState, lexer *gotreesitter.ExternalLexer) bool {
	s.indents = s.indents[:len(s.indents)-1]
	s.bullets = s.bullets[:len(s.bullets)-1]
	lexer.SetResultSymbol(orgSymListEnd)
	return true
}

func orgGetBullet(lexer *gotreesitter.ExternalLexer) int16 {
	ch := lexer.Lookahead()
	if ch == '-' {
		lexer.Advance(false)
		if unicode.IsSpace(lexer.Lookahead()) {
			return orgBulletDash
		}
	} else if ch == '+' {
		lexer.Advance(false)
		if unicode.IsSpace(lexer.Lookahead()) {
			return orgBulletPlus
		}
	} else if ch == '*' {
		lexer.Advance(false)
		if unicode.IsSpace(lexer.Lookahead()) {
			return orgBulletStar
		}
	} else if ch >= 'a' && ch <= 'z' {
		lexer.Advance(false)
		if lexer.Lookahead() == '.' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletLowerDot
			}
		} else if lexer.Lookahead() == ')' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletLowerParen
			}
		}
	} else if ch >= 'A' && ch <= 'Z' {
		lexer.Advance(false)
		if lexer.Lookahead() == '.' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletUpperDot
			}
		} else if lexer.Lookahead() == ')' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletUpperParen
			}
		}
	} else if ch >= '0' && ch <= '9' {
		for lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '.' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletNumDot
			}
		} else if lexer.Lookahead() == ')' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletNumParen
			}
		}
	}
	return orgBulletNone
}

func orgInErrorRecovery(vs []bool) bool {
	return orgValid(vs, orgTokListStart) && orgValid(vs, orgTokListEnd) &&
		orgValid(vs, orgTokListItemEnd) && orgValid(vs, orgTokBullet) &&
		orgValid(vs, orgTokHlStars) && orgValid(vs, orgTokSectionEnd) &&
		orgValid(vs, orgTokEndOfFile)
}

func orgValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
