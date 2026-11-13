//go:build !grammar_subset || grammar_subset_bash

package grammarruntime

import (
	"encoding/binary"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// The C-faithful scan of bash: a line-by-line port of the pinned C scanner
// (tree-sitter-bash src/scanner.c), used only when the core turns the scanner
// mode on (internal/scanmode). With the mode off the port keeps the scan of main
// (bshScan in bash_scanner.go).
//
// Differences from main that the mode removes (each pinned by a test in
// bash_scanner_c_parity_test.go):
//   - C has no OPENING_PAREN or ESAC scan: both tokens are declared but never
//     produced (scanner.c:9-39; the enum), so the mode never returns them.
//   - C tests lexer->eof for the end of the input in some places and
//     lexer->lookahead == 0 in others; a NUL byte is not the end of the input.
//   - Character classes are those of the C locale (iswspace, iswalpha, iswalnum,
//     iswdigit: ASCII only), and a byte that is not valid UTF-8 is -1.
//   - The heredoc delimiter is a signed char array; it is compared with the
//     lookahead as a signed value (scanner.c:233).
//   - advance_word appends to the delimiter instead of replacing it (:164-193).
//   - The saved state is limited by TREE_SITTER_SERIALIZATION_BUFFER_SIZE (1024),
//     and a state with fewer heredocs does not shorten the scanner's heredoc
//     array (deserialize, :121-155).

// bshCSerializationBufferSize is TREE_SITTER_SERIALIZATION_BUFFER_SIZE of the C
// runtime (parser.h:14).
const bshCSerializationBufferSize = 1024

// bshC carries one C-faithful scan.
type bshC struct {
	lx   *gotreesitter.ExternalLexer
	vs   []bool
	syms *[bshTokenCount]gotreesitter.Symbol
	s    *bshState
}

func (c *bshC) la() int32   { return scanmode.CLookahead(c.lx) }
func (c *bshC) eof() bool   { return scanmode.AtEOF(c.lx) }
func (c *bshC) adv()        { c.lx.Advance(false) }
func (c *bshC) skip()       { c.lx.Advance(true) }
func (c *bshC) mark()       { c.lx.MarkEnd() }
func (c *bshC) res(tok int) { c.lx.SetResultSymbol(c.syms[tok]) }
func (c *bshC) valid(tok int) bool {
	return tok < len(c.vs) && c.vs[tok]
}
func (c *bshC) errRec() bool { return c.valid(bshTokErrorRecovery) }

func bshCSpace(ch int32) bool { return cIsSpace(rune(ch)) }
func bshCAlpha(ch int32) bool { return cIsAlpha(rune(ch)) }
func bshCAlnum(ch int32) bool { return cIsAlnum(rune(ch)) }
func bshCDigit(ch int32) bool { return cIsDigit(rune(ch)) }

// bshSerializeC is C's serialize (scanner.c:93-119).
func bshSerializeC(s *bshState, buf []byte) int {
	if len(buf) < 4 {
		return 0
	}
	size := 0
	buf[size] = s.lastGlobParenDepth
	size++
	buf[size] = bshCBool(s.extWasInDoubleQuote)
	size++
	buf[size] = bshCBool(s.extSawOutsideQuote)
	size++
	buf[size] = byte(len(s.heredocs))
	size++
	for i := range s.heredocs {
		h := &s.heredocs[i]
		needed := 3 + 4 + len(h.delimiter)
		// C refuses a state that would reach the serialization buffer size (:103); the
		// limit is C's constant, and a buffer shorter than that is respected as well.
		if size+needed >= bshCSerializationBufferSize || size+needed > len(buf) {
			return 0
		}
		buf[size] = bshCBool(h.isRaw)
		size++
		buf[size] = bshCBool(h.started)
		size++
		buf[size] = bshCBool(h.allowsIndent)
		size++
		binary.LittleEndian.PutUint32(buf[size:], uint32(len(h.delimiter)))
		size += 4
		copy(buf[size:], h.delimiter)
		size += len(h.delimiter)
	}
	return size
}

func bshCBool(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// bshDeserializeC is C's deserialize (scanner.c:121-155). A state with fewer
// heredocs than the scanner holds leaves the extra ones in place (C never shortens
// the array here), and current_leading_word is not touched.
func bshDeserializeC(s *bshState, buf []byte) {
	if len(buf) == 0 {
		for i := range s.heredocs { // reset (:87-91)
			s.heredocs[i].reset()
		}
		return
	}
	at := func(i int) byte {
		if i < len(buf) {
			return buf[i]
		}
		return 0
	}
	size := 0
	s.lastGlobParenDepth = at(size)
	size++
	s.extWasInDoubleQuote = at(size) != 0
	size++
	s.extSawOutsideQuote = at(size) != 0
	size++
	count := int(at(size))
	size++
	for i := 0; i < count; i++ {
		if i >= len(s.heredocs) {
			s.heredocs = append(s.heredocs, bshHeredoc{})
		}
		h := &s.heredocs[i]
		h.isRaw = at(size) != 0
		size++
		h.started = at(size) != 0
		size++
		h.allowsIndent = at(size) != 0
		size++
		var n uint32
		if size+4 <= len(buf) {
			n = binary.LittleEndian.Uint32(buf[size:])
		}
		size += 4
		if size > len(buf) { // a damaged state can end inside the header
			size = len(buf)
		}
		avail := len(buf) - size
		if int64(n) > int64(avail) { // a damaged state: C would read past its buffer
			n = uint32(avail)
		}
		h.delimiter = append(h.delimiter[:0], buf[size:size+int(n)]...)
		size += int(n)
	}
}

// advanceWord is C's advance_word (:164-193): it appends the unquoted word and a
// NUL to *word and reports whether the word was not empty.
func (c *bshC) advanceWord(word *[]byte) bool {
	empty := true
	var quote int32
	if c.la() == '\'' || c.la() == '"' {
		quote = c.la()
		c.adv()
	}
	for c.la() != 0 {
		if quote != 0 {
			if c.la() == quote || c.la() == '\r' || c.la() == '\n' {
				break
			}
		} else if bshCSpace(c.la()) {
			break
		}
		if c.la() == '\\' {
			c.adv()
			if c.la() == 0 {
				return false
			}
		}
		empty = false
		*word = append(*word, byte(c.la()))
		c.adv()
	}
	*word = append(*word, 0)
	if quote != 0 && c.la() == quote {
		c.adv()
	}
	return !empty
}

// scanBareDollar is C's scan_bare_dollar (:195-208).
func (c *bshC) scanBareDollar() bool {
	for bshCSpace(c.la()) && c.la() != '\n' && !c.eof() {
		c.skip()
	}
	if c.la() == '$' {
		c.adv()
		c.res(bshTokBareDollar)
		c.mark()
		return bshCSpace(c.la()) || c.eof() || c.la() == '"'
	}
	return false
}

// scanHeredocStart is C's scan_heredoc_start (:210-224).
func (c *bshC) scanHeredocStart(h *bshHeredoc) bool {
	for bshCSpace(c.la()) {
		c.skip()
	}
	c.res(bshTokHeredocStart)
	h.isRaw = c.la() == '\'' || c.la() == '"' || c.la() == '\\'
	if !c.advanceWord(&h.delimiter) {
		h.delimiter = h.delimiter[:0] // reset_string
		return false
	}
	return true
}

// scanHeredocEndIdentifier is C's scan_heredoc_end_identifier (:226-244). The
// delimiter is an array of signed char compared with the lookahead as an int.
func (c *bshC) scanHeredocEndIdentifier(h *bshHeredoc) bool {
	h.currentLeadingWord = h.currentLeadingWord[:0]
	size := 0
	if len(h.delimiter) > 0 {
		for c.la() != 0 && c.la() != '\n' &&
			size < len(h.delimiter) && int32(int8(h.delimiter[size])) == c.la() &&
			len(h.currentLeadingWord) < len(h.delimiter) {
			h.currentLeadingWord = append(h.currentLeadingWord, byte(c.la()))
			c.adv()
			size++
		}
	}
	h.currentLeadingWord = append(h.currentLeadingWord, 0)
	if len(h.delimiter) == 0 {
		return false
	}
	return bshBytesEqual(h.currentLeadingWord, h.delimiter) // strcmp
}

// scanHeredocContent is C's scan_heredoc_content (:246-347).
func (c *bshC) scanHeredocContent(middle, end int) bool {
	didAdvance := false
	h := &c.s.heredocs[len(c.s.heredocs)-1]
	for {
		switch c.la() {
		case 0:
			if c.eof() && didAdvance {
				h.reset()
				c.res(end)
				return true
			}
			return false
		case '\\':
			didAdvance = true
			c.adv()
			c.adv()
		case '$':
			if h.isRaw {
				didAdvance = true
				c.adv()
				break
			}
			if didAdvance {
				c.mark()
				c.res(middle)
				h.started = true
				c.adv()
				if bshCAlpha(c.la()) || c.la() == '{' || c.la() == '(' {
					return true
				}
				break
			}
			if middle == bshTokHeredocBodyBeginning && c.lx.GetColumn() == 0 {
				c.res(middle)
				h.started = true
				return true
			}
			return false
		case '\n':
			if !didAdvance {
				c.skip()
			} else {
				c.adv()
			}
			didAdvance = true
			if h.allowsIndent {
				for bshCSpace(c.la()) {
					c.adv()
				}
			}
			sym := end
			if h.started {
				sym = middle
			}
			c.res(sym)
			c.mark()
			if c.scanHeredocEndIdentifier(h) {
				if sym == bshTokHeredocEnd {
					c.s.heredocs = c.s.heredocs[:len(c.s.heredocs)-1] // array_pop
				}
				return true
			}
		default:
			if c.lx.GetColumn() == 0 {
				for bshCSpace(c.la()) {
					if didAdvance {
						c.adv()
					} else {
						c.skip()
					}
				}
				if end != bshTokSimpleHeredocBody {
					c.res(middle)
					if c.scanHeredocEndIdentifier(h) {
						return true
					}
				}
				if end == bshTokSimpleHeredocBody {
					c.res(end)
					c.mark()
					if c.scanHeredocEndIdentifier(h) {
						return true
					}
				}
			}
			didAdvance = true
			c.adv()
		}
	}
}

// bshScanC is C's scan (:349-1185). The goto labels regex, extglob_pattern,
// expansion_word and brace_start become the sections of tail; each returns whether
// it decided the scan, the way a C label falls through to the next one.
func bshScanC(s *bshState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[bshTokenCount]gotreesitter.Symbol) bool {
	c := &bshC{lx: lexer, vs: validSymbols, syms: syms, s: s}
	sections := [...]func() (bool, bool){c.regex, c.extglobPattern, c.expansionWord, c.braceStart}
	tail := func(from int) bool {
		for i := from; i < len(sections); i++ {
			if r, done := sections[i](); done {
				return r
			}
		}
		return false
	}
	const (
		toRegex = iota
		toExtglob
		toExpansionWord
		toBraceStart
	)

	if c.valid(bshTokConcat) && !c.errRec() { // :350
		la := c.la()
		if !(la == 0 || bshCSpace(la) || la == '>' || la == '<' || la == ')' || la == '(' ||
			la == ';' || la == '&' || la == '|' ||
			(la == '}' && c.valid(bshTokClosingBrace)) ||
			(la == ']' && c.valid(bshTokClosingBracket))) {
			c.res(bshTokConcat)
			if c.la() == '`' {
				c.mark()
				c.adv()
				for c.la() != '`' && !c.eof() {
					c.adv()
				}
				if c.eof() {
					return false
				}
				if c.la() == '`' {
					c.adv()
				}
				return bshCSpace(c.la()) || c.eof()
			}
			if c.la() == '\\' {
				c.mark()
				c.adv()
				if c.la() == '"' || c.la() == '\'' || c.la() == '\\' {
					return true
				}
				if c.eof() {
					return false
				}
			} else {
				return true
			}
		}
		if bshCSpace(c.la()) && c.valid(bshTokClosingBrace) && !c.valid(bshTokExpansionWord) {
			c.res(bshTokConcat)
			return true
		}
	}

	if c.valid(bshTokImmediateDoubleHash) && !c.errRec() { // :395
		if c.la() == '#' {
			c.mark()
			c.adv()
			if c.la() == '#' {
				c.adv()
				if c.la() != '}' {
					c.res(bshTokImmediateDoubleHash)
					c.mark()
					return true
				}
			}
		}
	}

	if c.valid(bshTokExternalExpansionSymHash) && !c.errRec() { // :411
		if c.la() == '#' || c.la() == '=' || c.la() == '!' {
			switch c.la() {
			case '#':
				c.res(bshTokExternalExpansionSymHash)
			case '!':
				c.res(bshTokExternalExpansionSymBang)
			default:
				c.res(bshTokExternalExpansionSymEq)
			}
			c.adv()
			c.mark()
			for c.la() == '#' || c.la() == '=' || c.la() == '!' {
				c.adv()
			}
			for bshCSpace(c.la()) {
				c.skip()
			}
			return c.la() == '}'
		}
	}

	if c.valid(bshTokEmptyValue) { // :431
		if bshCSpace(c.la()) || c.eof() || c.la() == ';' || c.la() == '&' {
			c.res(bshTokEmptyValue)
			return true
		}
	}

	if (c.valid(bshTokHeredocBodyBeginning) || c.valid(bshTokSimpleHeredocBody)) && len(s.heredocs) > 0 &&
		!s.heredocs[len(s.heredocs)-1].started && !c.errRec() { // :438
		return c.scanHeredocContent(bshTokHeredocBodyBeginning, bshTokSimpleHeredocBody)
	}

	if c.valid(bshTokHeredocEnd) && len(s.heredocs) > 0 { // :443
		h := &s.heredocs[len(s.heredocs)-1]
		if c.scanHeredocEndIdentifier(h) {
			s.heredocs = s.heredocs[:len(s.heredocs)-1]
			c.res(bshTokHeredocEnd)
			return true
		}
	}

	if c.valid(bshTokHeredocContent) && len(s.heredocs) > 0 && s.heredocs[len(s.heredocs)-1].started && !c.errRec() { // :454
		return c.scanHeredocContent(bshTokHeredocContent, bshTokHeredocEnd)
	}

	if c.valid(bshTokHeredocStart) && !c.errRec() && len(s.heredocs) > 0 { // :459
		return c.scanHeredocStart(&s.heredocs[len(s.heredocs)-1])
	}

	if c.valid(bshTokTestOperator) && !c.valid(bshTokExpansionWord) { // :463
		for bshCSpace(c.la()) && c.la() != '\n' {
			c.skip()
		}
		if c.la() == '\\' {
			if c.valid(bshTokExtglobPattern) {
				return tail(toExtglob)
			}
			if c.valid(bshTokRegexNoSpace) {
				return tail(toRegex)
			}
			c.skip()
			if c.eof() {
				return false
			}
			if c.la() == '\r' {
				c.skip()
				if c.la() == '\n' {
					c.skip()
				}
			} else if c.la() == '\n' {
				c.skip()
			} else {
				return false
			}
			for bshCSpace(c.la()) {
				c.skip()
			}
		}
		if c.la() == '\n' && !c.valid(bshTokNewline) {
			c.skip()
			for bshCSpace(c.la()) {
				c.skip()
			}
		}
		if c.la() == '-' {
			c.adv()
			advancedOnce := false
			for bshCAlpha(c.la()) {
				advancedOnce = true
				c.adv()
			}
			if bshCSpace(c.la()) && advancedOnce {
				c.mark()
				c.adv()
				if c.la() == '}' && c.valid(bshTokClosingBrace) {
					if c.valid(bshTokExpansionWord) {
						c.mark()
						c.res(bshTokExpansionWord)
						return true
					}
					return false
				}
				c.res(bshTokTestOperator)
				return true
			}
			if bshCSpace(c.la()) && c.valid(bshTokExtglobPattern) {
				c.res(bshTokExtglobPattern)
				return true
			}
		}
		if c.valid(bshTokBareDollar) && !c.errRec() && c.scanBareDollar() {
			return true
		}
	}

	if (c.valid(bshTokVariableName) || c.valid(bshTokFileDescriptor) || c.valid(bshTokHeredocArrow)) &&
		!c.valid(bshTokRegexNoSlash) && !c.errRec() { // :539
		for {
			if (c.la() == ' ' || c.la() == '\t' || c.la() == '\r' || (c.la() == '\n' && !c.valid(bshTokNewline))) &&
				!c.valid(bshTokExpansionWord) {
				c.skip()
			} else if c.la() == '\\' {
				c.skip()
				if c.eof() {
					c.mark()
					c.res(bshTokVariableName)
					return true
				}
				if c.la() == '\r' {
					c.skip()
				}
				if c.la() == '\n' {
					c.skip()
				} else {
					if c.la() == '\\' && c.valid(bshTokExpansionWord) {
						return tail(toExpansionWord)
					}
					return false
				}
			} else {
				break
			}
		}

		// no '*', '@', '?', '-', '$', '0', '_' (:571)
		if !c.valid(bshTokExpansionWord) &&
			(c.la() == '*' || c.la() == '@' || c.la() == '?' || c.la() == '-' || c.la() == '0' || c.la() == '_') {
			c.mark()
			c.adv()
			if c.la() == '=' || c.la() == '[' || c.la() == ':' || c.la() == '-' || c.la() == '%' || c.la() == '#' || c.la() == '/' {
				return false
			}
			if c.valid(bshTokExtglobPattern) && bshCSpace(c.la()) {
				c.mark()
				c.res(bshTokExtglobPattern)
				return true
			}
		}

		if c.valid(bshTokHeredocArrow) && c.la() == '<' { // :589
			c.adv()
			if c.la() == '<' {
				c.adv()
				if c.la() == '-' {
					c.adv()
					s.heredocs = append(s.heredocs, bshHeredoc{allowsIndent: true})
					c.res(bshTokHeredocArrowDash)
				} else if c.la() == '<' || c.la() == '=' {
					return false
				} else {
					s.heredocs = append(s.heredocs, bshHeredoc{})
					c.res(bshTokHeredocArrow)
				}
				return true
			}
			return false
		}

		isNumber := true // :611
		if bshCDigit(c.la()) {
			c.adv()
		} else if bshCAlpha(c.la()) || c.la() == '_' {
			isNumber = false
			c.adv()
		} else {
			if c.la() == '{' {
				return tail(toBraceStart)
			}
			if c.valid(bshTokExpansionWord) {
				return tail(toExpansionWord)
			}
			if c.valid(bshTokExtglobPattern) {
				return tail(toExtglob)
			}
			return false
		}
		for {
			if bshCDigit(c.la()) {
				c.adv()
			} else if bshCAlpha(c.la()) || c.la() == '_' {
				isNumber = false
				c.adv()
			} else {
				break
			}
		}
		if isNumber && c.valid(bshTokFileDescriptor) && (c.la() == '>' || c.la() == '<') {
			c.res(bshTokFileDescriptor)
			return true
		}
		if c.valid(bshTokVariableName) { // :646
			if c.la() == '+' {
				c.mark()
				c.adv()
				if c.la() == '=' || c.la() == ':' || c.valid(bshTokClosingBrace) {
					c.res(bshTokVariableName)
					return true
				}
				return false
			}
			if c.la() == '/' {
				return false
			}
			if c.la() == '=' || c.la() == '[' ||
				(c.la() == ':' && !c.valid(bshTokClosingBrace) && !c.valid(bshTokOpeningParen)) ||
				c.la() == '%' ||
				(c.la() == '#' && !isNumber) || c.la() == '@' ||
				(c.la() == '-' && c.valid(bshTokClosingBrace)) {
				c.mark()
				c.res(bshTokVariableName)
				return true
			}
			if c.la() == '?' {
				c.mark()
				c.adv()
				c.res(bshTokVariableName)
				return bshCAlpha(c.la())
			}
		}
		return false
	}

	if c.valid(bshTokBareDollar) && !c.errRec() && c.scanBareDollar() { // :682
		return true
	}
	return tail(toRegex)
}

// regex is the section of the label regex (:686-857).
func (c *bshC) regex() (bool, bool) {
	if !((c.valid(bshTokRegex) || c.valid(bshTokRegexNoSlash) || c.valid(bshTokRegexNoSpace)) && !c.errRec()) {
		return false, false
	}
	if c.valid(bshTokRegex) || c.valid(bshTokRegexNoSpace) {
		for bshCSpace(c.la()) {
			c.skip()
		}
	}
	if !((c.la() != '"' && c.la() != '\'') ||
		((c.la() == '$' || c.la() == '\'') && c.valid(bshTokRegexNoSlash)) ||
		(c.la() == '\'' && c.valid(bshTokRegexNoSpace))) {
		return false, false
	}
	var done, advancedOnce, foundNonAlnumDollarUnderDash, lastWasEscape, inSingleQuote bool
	var parenDepth, bracketDepth, braceDepth uint32

	if c.la() == '$' && c.valid(bshTokRegexNoSlash) {
		c.mark()
		c.adv()
		if c.la() == '(' {
			return false, true
		}
	}
	c.mark()

	for !done {
		if inSingleQuote {
			if c.la() == '\'' {
				inSingleQuote = false
				c.adv()
				c.mark()
			}
		}
		switch c.la() {
		case '\\':
			lastWasEscape = true
		case 0:
			return false, true
		case '(':
			parenDepth++
			lastWasEscape = false
		case '[':
			bracketDepth++
			lastWasEscape = false
		case '{':
			if !lastWasEscape {
				braceDepth++
			}
			lastWasEscape = false
		case ')':
			if parenDepth == 0 {
				done = true
			}
			parenDepth--
			lastWasEscape = false
		case ']':
			if bracketDepth == 0 {
				done = true
			}
			bracketDepth--
			lastWasEscape = false
		case '}':
			if braceDepth == 0 {
				done = true
			}
			braceDepth--
			lastWasEscape = false
		case '\'':
			// Enter or exit a single-quoted string.
			inSingleQuote = !inSingleQuote
			c.adv()
			advancedOnce = true
			lastWasEscape = false
			continue
		default:
			lastWasEscape = false
		}

		if !done {
			if c.valid(bshTokRegex) {
				wasSpace := !inSingleQuote && bshCSpace(c.la())
				c.adv()
				advancedOnce = true
				if !wasSpace || parenDepth > 0 {
					c.mark()
				}
			} else if c.valid(bshTokRegexNoSlash) {
				if c.la() == '/' {
					c.mark()
					c.res(bshTokRegexNoSlash)
					return advancedOnce, true
				}
				if c.la() == '\\' {
					c.adv()
					advancedOnce = true
					if !c.eof() && c.la() != '[' && c.la() != '/' {
						c.adv()
						c.mark()
					}
				} else {
					wasSpace := !inSingleQuote && bshCSpace(c.la())
					c.adv()
					advancedOnce = true
					if !wasSpace {
						c.mark()
					}
				}
			} else if c.valid(bshTokRegexNoSpace) {
				if c.la() == '\\' {
					foundNonAlnumDollarUnderDash = true
					c.adv()
					if !c.eof() {
						c.adv()
					}
				} else if c.la() == '$' {
					c.mark()
					c.adv()
					// do not parse a command substitution
					if c.la() == '(' {
						return false, true
					}
					// end $ always means regex, e.g. 99999999$
					if bshCSpace(c.la()) {
						c.res(bshTokRegexNoSpace)
						c.mark()
						return true, true
					}
				} else {
					wasSpace := !inSingleQuote && bshCSpace(c.la())
					if wasSpace && parenDepth == 0 {
						c.mark()
						c.res(bshTokRegexNoSpace)
						return foundNonAlnumDollarUnderDash, true
					}
					if !bshCAlnum(c.la()) && c.la() != '$' && c.la() != '-' && c.la() != '_' {
						foundNonAlnumDollarUnderDash = true
					}
					c.adv()
				}
			}
		}
	}

	switch {
	case c.valid(bshTokRegexNoSlash):
		c.res(bshTokRegexNoSlash)
	case c.valid(bshTokRegexNoSpace):
		c.res(bshTokRegexNoSpace)
	default:
		c.res(bshTokRegex)
	}
	if c.valid(bshTokRegex) && !advancedOnce {
		return false, true
	}
	return true, true
}

// extglobPattern is the section of the label extglob_pattern (:859-1069).
func (c *bshC) extglobPattern() (bool, bool) {
	if !(c.valid(bshTokExtglobPattern) && !c.errRec()) {
		return false, false
	}
	s := c.s
	// first skip ws, then check for ? * + @ !
	for bshCSpace(c.la()) {
		c.skip()
	}
	if c.la() == '?' || c.la() == '*' || c.la() == '+' || c.la() == '@' ||
		c.la() == '!' || c.la() == '-' || c.la() == ')' || c.la() == '\\' ||
		c.la() == '.' || c.la() == '[' || bshCAlpha(c.la()) {
		if c.la() == '\\' {
			c.adv()
			if (bshCSpace(c.la()) || c.la() == '"') && c.la() != '\r' && c.la() != '\n' {
				c.adv()
			} else {
				return false, true
			}
		}
		if c.la() == ')' && s.lastGlobParenDepth == 0 {
			c.mark()
			c.adv()
			if bshCSpace(c.la()) {
				return false, true
			}
		}
		c.mark()
		wasNonAlpha := !bshCAlpha(c.la())
		if c.la() != '[' {
			// no esac
			if c.la() == 'e' {
				c.mark()
				c.adv()
				if c.la() == 's' {
					c.adv()
					if c.la() == 'a' {
						c.adv()
						if c.la() == 'c' {
							c.adv()
							if bshCSpace(c.la()) {
								return false, true
							}
						}
					}
				}
			} else {
				c.adv()
			}
		}
		// -\w is just a word, find something else special
		if c.la() == '-' {
			c.mark()
			c.adv()
			for bshCAlnum(c.la()) {
				c.adv()
			}
			if c.la() == ')' || c.la() == '\\' || c.la() == '.' {
				return false, true
			}
			c.mark()
		}
		// case item -) or *)
		if c.la() == ')' && s.lastGlobParenDepth == 0 {
			c.mark()
			c.adv()
			if bshCSpace(c.la()) {
				c.res(bshTokExtglobPattern)
				return wasNonAlpha, true
			}
		}
		if bshCSpace(c.la()) {
			c.mark()
			c.res(bshTokExtglobPattern)
			s.lastGlobParenDepth = 0
			return true, true
		}
		if c.la() == '$' {
			c.mark()
			c.adv()
			if c.la() == '{' || c.la() == '(' {
				c.res(bshTokExtglobPattern)
				return true, true
			}
		}
		if c.la() == '|' {
			c.mark()
			c.adv()
			c.res(bshTokExtglobPattern)
			return true, true
		}
		if !bshCAlnum(c.la()) && c.la() != '(' && c.la() != '"' &&
			c.la() != '[' && c.la() != '?' && c.la() != '/' &&
			c.la() != '\\' && c.la() != '_' && c.la() != '*' {
			return false, true
		}

		var done bool
		sawNonAlphaDot := wasNonAlpha
		parenDepth := uint32(s.lastGlobParenDepth)
		var bracketDepth, braceDepth uint32
		for !done {
			switch c.la() {
			case 0:
				return false, true
			case '(':
				parenDepth++
			case '[':
				bracketDepth++
			case '{':
				braceDepth++
			case ')':
				if parenDepth == 0 {
					done = true
				}
				parenDepth--
			case ']':
				if bracketDepth == 0 {
					done = true
				}
				bracketDepth--
			case '}':
				if braceDepth == 0 {
					done = true
				}
				braceDepth--
			}
			if c.la() == '|' {
				c.mark()
				c.adv()
				if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
					c.res(bshTokExtglobPattern)
					return true, true
				}
			}
			if !done {
				wasSpace := bshCSpace(c.la())
				if c.la() == '$' {
					c.mark()
					if !bshCAlpha(c.la()) && c.la() != '.' && c.la() != '\\' {
						sawNonAlphaDot = true
					}
					c.adv()
					if c.la() == '(' || c.la() == '{' {
						c.res(bshTokExtglobPattern)
						s.lastGlobParenDepth = uint8(parenDepth)
						return sawNonAlphaDot, true
					}
				}
				if wasSpace {
					c.mark()
					c.res(bshTokExtglobPattern)
					s.lastGlobParenDepth = 0
					return sawNonAlphaDot, true
				}
				if c.la() == '"' {
					c.mark()
					c.res(bshTokExtglobPattern)
					s.lastGlobParenDepth = 0
					return sawNonAlphaDot, true
				}
				if c.la() == '\\' {
					if !bshCAlpha(c.la()) && c.la() != '.' && c.la() != '\\' {
						sawNonAlphaDot = true
					}
					c.adv()
					if bshCSpace(c.la()) || c.la() == '"' {
						c.adv()
					}
				} else {
					if !bshCAlpha(c.la()) && c.la() != '.' && c.la() != '\\' {
						sawNonAlphaDot = true
					}
					c.adv()
				}
				if !wasSpace {
					c.mark()
				}
			}
		}
		c.res(bshTokExtglobPattern)
		s.lastGlobParenDepth = 0
		return sawNonAlphaDot, true
	}
	s.lastGlobParenDepth = 0
	return false, true
}

// expansionWord is the section of the label expansion_word (:1071-1143).
func (c *bshC) expansionWord() (bool, bool) {
	if !c.valid(bshTokExpansionWord) {
		return false, false
	}
	advancedOnce := false
	advanceOnceSpace := false
	for {
		if c.la() == '"' {
			return false, true
		}
		if c.la() == '$' {
			c.mark()
			c.adv()
			if c.la() == '{' || c.la() == '(' || c.la() == '\'' || bshCAlnum(c.la()) {
				c.res(bshTokExpansionWord)
				return advancedOnce, true
			}
			advancedOnce = true
		}
		if c.la() == '}' {
			c.mark()
			c.res(bshTokExpansionWord)
			return advancedOnce || advanceOnceSpace, true
		}
		if c.la() == '(' && !(advancedOnce || advanceOnceSpace) {
			c.mark()
			c.adv()
			for c.la() != ')' && !c.eof() {
				// if we find a $( or ${ assume this is valid and is a garbage
				// concatenation of some weird word + an expansion
				if c.la() == '$' {
					c.mark()
					c.adv()
					if c.la() == '{' || c.la() == '(' || c.la() == '\'' || bshCAlnum(c.la()) {
						c.res(bshTokExpansionWord)
						return advancedOnce, true
					}
					advancedOnce = true
				} else {
					advancedOnce = advancedOnce || !bshCSpace(c.la())
					advanceOnceSpace = advanceOnceSpace || bshCSpace(c.la())
					c.adv()
				}
			}
			c.mark()
			if c.la() == ')' {
				advancedOnce = true
				c.adv()
				c.mark()
				if c.la() == '}' {
					return false, true
				}
			} else {
				return false, true
			}
		}
		if c.la() == '\'' {
			return false, true
		}
		if c.eof() {
			return false, true
		}
		advancedOnce = advancedOnce || !bshCSpace(c.la())
		advanceOnceSpace = advanceOnceSpace || bshCSpace(c.la())
		c.adv()
	}
}

// braceStart is the section of the label brace_start (:1145-1184). It is the last
// section: it always decides.
func (c *bshC) braceStart() (bool, bool) {
	if !(c.valid(bshTokBraceStart) && !c.errRec()) {
		return false, true
	}
	for bshCSpace(c.la()) {
		c.skip()
	}
	if c.la() != '{' {
		return false, true
	}
	c.adv()
	c.mark()
	for bshCDigit(c.la()) {
		c.adv()
	}
	if c.la() != '.' {
		return false, true
	}
	c.adv()
	if c.la() != '.' {
		return false, true
	}
	c.adv()
	for bshCDigit(c.la()) {
		c.adv()
	}
	if c.la() != '}' {
		return false, true
	}
	c.res(bshTokBraceStart)
	return true, true
}
