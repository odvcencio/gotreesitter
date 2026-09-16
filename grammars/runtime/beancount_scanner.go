//go:build !grammar_subset || grammar_subset_beancount

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the beancount grammar.
const (
	beancountTokStars      = 0
	beancountTokSectionEnd = 1
	beancountTokEof        = 2
)

const (
	beancountSymStars      gotreesitter.Symbol = 60
	beancountSymSectionEnd gotreesitter.Symbol = 61
	beancountSymEof        gotreesitter.Symbol = 62
)

const beancountTabWidth = 8

// beancountState tracks section nesting for org-mode style headers in Beancount.
type beancountState struct {
	orgSectionStack []int16
	eofReturned     bool
}

// BeancountExternalScanner handles org-mode style section headers and EOF for Beancount.
type BeancountExternalScanner struct{}

func (BeancountExternalScanner) Create() any {
	return &beancountState{orgSectionStack: []int16{0}}
}
func (BeancountExternalScanner) Destroy(payload any) {}
func (BeancountExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*beancountState)
	i := 0
	if s.eofReturned {
		buf[i] = 1
	} else {
		buf[i] = 0
	}
	i++
	// We skip indent_length_stack (unused in section parsing)
	buf[i] = 0
	i++
	// Write org section stack (skip base element 0)
	count := len(s.orgSectionStack) - 1
	if count > 255 {
		count = 255
	}
	buf[i] = byte(count)
	i++
	for j := 1; j < len(s.orgSectionStack) && i < len(buf); j++ {
		buf[i] = byte(s.orgSectionStack[j])
		i++
	}
	return i
}
func (BeancountExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*beancountState)
	s.orgSectionStack = s.orgSectionStack[:0]
	s.orgSectionStack = append(s.orgSectionStack, 0)
	s.eofReturned = false
	if len(buf) == 0 {
		return
	}
	i := 0
	s.eofReturned = buf[i] != 0
	i++
	if i >= len(buf) {
		return
	}
	// Skip indent count
	indentCount := int(buf[i])
	i++
	i += indentCount // skip indent data
	if i >= len(buf) {
		return
	}
	sectionCount := int(buf[i])
	i++
	for j := 0; j < sectionCount && i < len(buf); j++ {
		s.orgSectionStack = append(s.orgSectionStack, int16(buf[i]))
		i++
	}
}

func (BeancountExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*beancountState)

	// Don't produce tokens during error recovery
	if beancountValid(validSymbols, beancountTokStars) &&
		beancountValid(validSymbols, beancountTokSectionEnd) &&
		beancountValid(validSymbols, beancountTokEof) {
		return false
	}

	lexer.MarkEnd()

	// Count leading whitespace
	indentLength := int16(0)
	for {
		ch := lexer.Lookahead()
		if ch == ' ' {
			indentLength++
			lexer.Advance(true)
		} else if ch == '\t' {
			indentLength += beancountTabWidth
			lexer.Advance(true)
		} else {
			break
		}
	}

	// Handle EOF
	if lexer.Lookahead() == 0 {
		if beancountValid(validSymbols, beancountTokSectionEnd) {
			lexer.SetResultSymbol(beancountSymSectionEnd)
			return true
		}
		if beancountValid(validSymbols, beancountTokEof) && !s.eofReturned {
			s.eofReturned = true
			lexer.SetResultSymbol(beancountSymEof)
			return true
		}
		return false
	}

	// Check for section headers (at column 0)
	if indentLength == 0 && isBeancountHeadlineMarker(lexer.Lookahead()) {
		lexer.MarkEnd()
		stars := int16(1)
		lexer.Advance(true)
		for isBeancountHeadlineMarker(lexer.Lookahead()) {
			stars++
			lexer.Advance(true)
		}
		if !unicode.IsSpace(lexer.Lookahead()) {
			return false
		}

		if beancountValid(validSymbols, beancountTokSectionEnd) && stars > 0 &&
			len(s.orgSectionStack) > 0 &&
			stars <= s.orgSectionStack[len(s.orgSectionStack)-1] {
			s.orgSectionStack = s.orgSectionStack[:len(s.orgSectionStack)-1]
			lexer.SetResultSymbol(beancountSymSectionEnd)
			return true
		}
		if beancountValid(validSymbols, beancountTokStars) {
			s.orgSectionStack = append(s.orgSectionStack, stars)
			lexer.SetResultSymbol(beancountSymStars)
			return true
		}
	}

	return false
}

func isBeancountHeadlineMarker(ch rune) bool {
	return ch == '*' || ch == '#'
}

func beancountValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
