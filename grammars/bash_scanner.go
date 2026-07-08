//go:build !grammar_subset || grammar_subset_bash

package grammars

import (
	"encoding/binary"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Bash grammar.
// Must match the order of external symbols in the generated grammar.
const (
	bshTokHeredocStart             = 0
	bshTokSimpleHeredocBody        = 1
	bshTokHeredocBodyBeginning     = 2
	bshTokHeredocContent           = 3
	bshTokHeredocEnd               = 4
	bshTokFileDescriptor           = 5
	bshTokEmptyValue               = 6
	bshTokConcat                   = 7
	bshTokVariableName             = 8
	bshTokTestOperator             = 9
	bshTokRegex                    = 10
	bshTokRegexNoSlash             = 11
	bshTokRegexNoSpace             = 12
	bshTokExpansionWord            = 13
	bshTokExtglobPattern           = 14
	bshTokBareDollar               = 15
	bshTokBraceStart               = 16
	bshTokImmediateDoubleHash      = 17
	bshTokExternalExpansionSymHash = 18
	bshTokExternalExpansionSymBang = 19
	bshTokExternalExpansionSymEq   = 20
	bshTokClosingBrace             = 21
	bshTokClosingBracket           = 22
	bshTokHeredocArrow             = 23
	bshTokHeredocArrowDash         = 24
	bshTokNewline                  = 25
	bshTokOpeningParen             = 26
	bshTokEsac                     = 27
	bshTokErrorRecovery            = 28
)

// Concrete symbol IDs from the generated Bash grammar ExternalSymbols.
const (
	bshSymHeredocStart             gotreesitter.Symbol = 152
	bshSymSimpleHeredocBody        gotreesitter.Symbol = 153
	bshSymHeredocBodyBeginning     gotreesitter.Symbol = 154
	bshSymHeredocContent           gotreesitter.Symbol = 155
	bshSymHeredocEnd               gotreesitter.Symbol = 156
	bshSymFileDescriptor           gotreesitter.Symbol = 157
	bshSymEmptyValue               gotreesitter.Symbol = 158
	bshSymConcat                   gotreesitter.Symbol = 159
	bshSymVariableName             gotreesitter.Symbol = 160
	bshSymTestOperator             gotreesitter.Symbol = 161
	bshSymRegex                    gotreesitter.Symbol = 162
	bshSymRegexNoSlash             gotreesitter.Symbol = 163
	bshSymRegexNoSpace             gotreesitter.Symbol = 164
	bshSymExpansionWord            gotreesitter.Symbol = 165
	bshSymExtglobPattern           gotreesitter.Symbol = 166
	bshSymBareDollar               gotreesitter.Symbol = 167
	bshSymBraceStart               gotreesitter.Symbol = 168
	bshSymImmediateDoubleHash      gotreesitter.Symbol = 169
	bshSymExternalExpansionSymHash gotreesitter.Symbol = 170
	bshSymExternalExpansionSymBang gotreesitter.Symbol = 171
	bshSymExternalExpansionSymEq   gotreesitter.Symbol = 172
	bshSymClosingBrace             gotreesitter.Symbol = 111
	bshSymClosingBracket           gotreesitter.Symbol = 67
	bshSymHeredocArrow             gotreesitter.Symbol = 36
	bshSymHeredocArrowDash         gotreesitter.Symbol = 85
	bshSymNewline                  gotreesitter.Symbol = 86
	bshSymOpeningParen             gotreesitter.Symbol = 44
	bshSymEsac                     gotreesitter.Symbol = 57
	bshSymErrorRecovery            gotreesitter.Symbol = 173
)

// bshHeredoc tracks a single pending heredoc.
type bshHeredoc struct {
	isRaw              bool
	started            bool
	allowsIndent       bool
	delimiter          []byte
	currentLeadingWord []byte
}

func (h *bshHeredoc) reset() {
	h.isRaw = false
	h.started = false
	h.allowsIndent = false
	h.delimiter = h.delimiter[:0]
}

// bshState holds scanner state across parse calls.
type bshState struct {
	lastGlobParenDepth  uint8
	extWasInDoubleQuote bool
	extSawOutsideQuote  bool
	heredocs            []bshHeredoc
}

// BashExternalScanner implements gotreesitter.ExternalScanner for the Bash grammar.
type BashExternalScanner struct{}

func (BashExternalScanner) Create() any {
	return &bshState{}
}

func (BashExternalScanner) Destroy(payload any) {}

func (BashExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*bshState)
	size := 0
	if len(buf) < 4 {
		return 0
	}

	buf[size] = s.lastGlobParenDepth
	size++
	if s.extWasInDoubleQuote {
		buf[size] = 1
	} else {
		buf[size] = 0
	}
	size++
	if s.extSawOutsideQuote {
		buf[size] = 1
	} else {
		buf[size] = 0
	}
	size++
	buf[size] = byte(len(s.heredocs))
	size++

	for i := range s.heredocs {
		hd := &s.heredocs[i]
		// 3 bools + uint32 length + delimiter bytes
		needed := 3 + 4 + len(hd.delimiter)
		if size+needed >= len(buf) {
			return 0
		}

		if hd.isRaw {
			buf[size] = 1
		} else {
			buf[size] = 0
		}
		size++
		if hd.started {
			buf[size] = 1
		} else {
			buf[size] = 0
		}
		size++
		if hd.allowsIndent {
			buf[size] = 1
		} else {
			buf[size] = 0
		}
		size++

		binary.LittleEndian.PutUint32(buf[size:], uint32(len(hd.delimiter)))
		size += 4
		if len(hd.delimiter) > 0 {
			copy(buf[size:], hd.delimiter)
			size += len(hd.delimiter)
		}
	}
	return size
}

func (BashExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*bshState)
	// Reset existing heredocs.
	for i := range s.heredocs {
		s.heredocs[i].reset()
	}

	if len(buf) == 0 {
		return
	}

	size := 0
	s.lastGlobParenDepth = buf[size]
	size++
	s.extWasInDoubleQuote = buf[size] != 0
	size++
	s.extSawOutsideQuote = buf[size] != 0
	size++
	heredocCount := int(buf[size])
	size++

	// Resize the heredocs slice.
	for len(s.heredocs) < heredocCount {
		s.heredocs = append(s.heredocs, bshHeredoc{})
	}
	s.heredocs = s.heredocs[:heredocCount]

	for i := 0; i < heredocCount; i++ {
		hd := &s.heredocs[i]
		hd.isRaw = buf[size] != 0
		size++
		hd.started = buf[size] != 0
		size++
		hd.allowsIndent = buf[size] != 0
		size++

		delimLen := int(binary.LittleEndian.Uint32(buf[size:]))
		size += 4
		hd.delimiter = make([]byte, delimLen)
		if delimLen > 0 {
			copy(hd.delimiter, buf[size:size+delimLen])
			size += delimLen
		}
	}
}

func (BashExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*bshState)
	return bshScan(s, lexer, validSymbols)
}

// ---- helpers ----

func bshIsValid(vs []bool, idx int) bool {
	return idx < len(vs) && vs[idx]
}

func bshInErrorRecovery(vs []bool) bool {
	return bshIsValid(vs, bshTokErrorRecovery)
}

func bshAdvance(lexer *gotreesitter.ExternalLexer) {
	lexer.Advance(false)
}

func bshSkip(lexer *gotreesitter.ExternalLexer) {
	lexer.Advance(true)
}

func bshSkipHorizontalSpace(lexer *gotreesitter.ExternalLexer) {
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		bshSkip(lexer)
	}
}

// bshAdvanceWord consumes a POSIX "word" from the lexer, appending the
// unquoted content to unquoted. Returns true if the word is non-empty.
func bshAdvanceWord(lexer *gotreesitter.ExternalLexer) (unquoted []byte, ok bool) {
	empty := true

	var quote rune
	if lexer.Lookahead() == '\'' || lexer.Lookahead() == '"' {
		quote = lexer.Lookahead()
		bshAdvance(lexer)
	}

	for lexer.Lookahead() != 0 {
		if quote != 0 {
			if lexer.Lookahead() == quote || lexer.Lookahead() == '\r' || lexer.Lookahead() == '\n' {
				break
			}
		} else {
			if bshIsSpace(lexer.Lookahead()) {
				break
			}
		}
		if lexer.Lookahead() == '\\' {
			bshAdvance(lexer)
			if lexer.Lookahead() == 0 {
				return nil, false
			}
		}
		empty = false
		unquoted = append(unquoted, byte(lexer.Lookahead()))
		bshAdvance(lexer)
	}
	unquoted = append(unquoted, 0) // NUL terminator, mirroring the C code

	if quote != 0 && lexer.Lookahead() == quote {
		bshAdvance(lexer)
	}

	return unquoted, !empty
}

func bshIsSpace(r rune) bool {
	return unicode.IsSpace(r)
}

func bshIsAlpha(r rune) bool {
	return unicode.IsLetter(r)
}

func bshIsDigit(r rune) bool {
	return unicode.IsDigit(r)
}

func bshIsAlnum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// bshBytesEqual compares two NUL-terminated byte slices.
func bshBytesEqual(a, b []byte) bool {
	// Both are NUL-terminated from the C convention. Compare up to NUL.
	la := len(a)
	lb := len(b)
	// Find effective length (up to first NUL).
	for i := 0; i < la; i++ {
		if a[i] == 0 {
			la = i
			break
		}
	}
	for i := 0; i < lb; i++ {
		if b[i] == 0 {
			lb = i
			break
		}
	}
	if la != lb {
		return false
	}
	for i := 0; i < la; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// bshDelimiterSize returns the effective size of the delimiter (up to first NUL).
func bshDelimiterSize(d []byte) int {
	for i := 0; i < len(d); i++ {
		if d[i] == 0 {
			return i
		}
	}
	return len(d)
}

func bshIsReservedWordBoundary(r rune) bool {
	return r == 0 || bshIsSpace(r) || r == ';' || r == '&' || r == '|' || r == ')'
}

func bshScanOpeningParen(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !bshIsValid(validSymbols, bshTokConcat) {
		bshSkipHorizontalSpace(lexer)
	}
	if lexer.Lookahead() != '(' {
		return false
	}

	bshAdvance(lexer)
	lexer.MarkEnd()
	lexer.SetResultSymbol(bshSymOpeningParen)
	return true
}

func bshScanEsac(lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !bshIsValid(validSymbols, bshTokConcat) {
		bshSkipHorizontalSpace(lexer)
	}
	for _, want := range []rune{'e', 's', 'a', 'c'} {
		if lexer.Lookahead() != want {
			return false
		}
		bshAdvance(lexer)
	}
	if !bshIsReservedWordBoundary(lexer.Lookahead()) {
		return false
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(bshSymEsac)
	return true
}

// ---- heredoc scanning ----

func bshScanBareDollar(lexer *gotreesitter.ExternalLexer) bool {
	for bshIsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
		bshSkip(lexer)
	}

	if lexer.Lookahead() == '$' {
		bshAdvance(lexer)
		lexer.MarkEnd()
		lexer.SetResultSymbol(bshSymBareDollar)
		return bshIsSpace(lexer.Lookahead()) || lexer.Lookahead() == 0 || lexer.Lookahead() == '"'
	}

	return false
}

func bshScanHeredocStart(heredoc *bshHeredoc, lexer *gotreesitter.ExternalLexer) bool {
	for bshIsSpace(lexer.Lookahead()) {
		bshSkip(lexer)
	}

	heredoc.isRaw = lexer.Lookahead() == '\'' || lexer.Lookahead() == '"' || lexer.Lookahead() == '\\'

	unquoted, found := bshAdvanceWord(lexer)
	if !found {
		heredoc.delimiter = heredoc.delimiter[:0]
		return false
	}
	heredoc.delimiter = unquoted
	lexer.MarkEnd()
	lexer.SetResultSymbol(bshSymHeredocStart)
	return true
}

func bshScanHeredocEndIdentifier(heredoc *bshHeredoc, lexer *gotreesitter.ExternalLexer) bool {
	heredoc.currentLeadingWord = heredoc.currentLeadingWord[:0]
	delimSize := bshDelimiterSize(heredoc.delimiter)
	size := 0
	if delimSize > 0 {
		for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' &&
			size < delimSize &&
			rune(heredoc.delimiter[size]) == lexer.Lookahead() &&
			len(heredoc.currentLeadingWord) < delimSize {
			heredoc.currentLeadingWord = append(heredoc.currentLeadingWord, byte(lexer.Lookahead()))
			bshAdvance(lexer)
			size++
		}
	}
	heredoc.currentLeadingWord = append(heredoc.currentLeadingWord, 0)

	if delimSize == 0 {
		return false
	}
	return bshBytesEqual(heredoc.currentLeadingWord, heredoc.delimiter)
}

func bshScanHeredocContent(
	s *bshState,
	lexer *gotreesitter.ExternalLexer,
	middleTok int,
	endTok int,
	middleSym gotreesitter.Symbol,
	endSym gotreesitter.Symbol,
) bool {
	didAdvance := false
	heredoc := &s.heredocs[len(s.heredocs)-1]

	for {
		switch lexer.Lookahead() {
		case 0:
			// EOF
			if lexer.Lookahead() == 0 && didAdvance {
				heredoc.reset()
				lexer.SetResultSymbol(endSym)
				return true
			}
			return false

		case '\\':
			didAdvance = true
			bshAdvance(lexer)
			bshAdvance(lexer)

		case '$':
			if heredoc.isRaw {
				didAdvance = true
				bshAdvance(lexer)
				continue
			}
			if didAdvance {
				lexer.MarkEnd()
				lexer.SetResultSymbol(middleSym)
				heredoc.started = true
				bshAdvance(lexer)
				if bshIsAlpha(lexer.Lookahead()) || lexer.Lookahead() == '{' || lexer.Lookahead() == '(' {
					return true
				}
				continue
			}
			if middleTok == bshTokHeredocBodyBeginning && lexer.Column() == 0 {
				lexer.SetResultSymbol(middleSym)
				heredoc.started = true
				return true
			}
			return false

		case '\n':
			if !didAdvance {
				bshSkip(lexer)
			} else {
				bshAdvance(lexer)
			}
			didAdvance = true
			if heredoc.allowsIndent {
				for bshIsSpace(lexer.Lookahead()) {
					bshAdvance(lexer)
				}
			}
			if heredoc.started {
				lexer.SetResultSymbol(middleSym)
			} else {
				lexer.SetResultSymbol(endSym)
			}
			lexer.MarkEnd()
			if bshScanHeredocEndIdentifier(heredoc, lexer) {
				if endTok == bshTokHeredocEnd {
					// This corresponds to lexer->result_symbol == HEREDOC_END check
					// in the C code; when endSym is set, pop the heredoc.
					if !heredoc.started {
						s.heredocs = s.heredocs[:len(s.heredocs)-1]
					} else {
						// middleSym was set (heredoc was started), and we matched end.
						// The C code checks if result_symbol == HEREDOC_END, which
						// happens only when !heredoc.started. Otherwise middleSym stays.
					}
				}
				return true
			}

		default:
			if lexer.Column() == 0 {
				for bshIsSpace(lexer.Lookahead()) {
					if didAdvance {
						bshAdvance(lexer)
					} else {
						bshSkip(lexer)
					}
				}
				if endTok != bshTokSimpleHeredocBody {
					lexer.SetResultSymbol(middleSym)
					if bshScanHeredocEndIdentifier(heredoc, lexer) {
						return true
					}
				}
				if endTok == bshTokSimpleHeredocBody {
					lexer.SetResultSymbol(endSym)
					lexer.MarkEnd()
					if bshScanHeredocEndIdentifier(heredoc, lexer) {
						return true
					}
				}
			}
			didAdvance = true
			bshAdvance(lexer)
		}
	}
}

// bshScanRegex handles REGEX, REGEX_NO_SLASH, REGEX_NO_SPACE scanning.
func bshScanRegex(s *bshState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if bshIsValid(validSymbols, bshTokRegex) || bshIsValid(validSymbols, bshTokRegexNoSpace) {
		for bshIsSpace(lexer.Lookahead()) {
			bshSkip(lexer)
		}
	}

	if (lexer.Lookahead() != '"' && lexer.Lookahead() != '\'') ||
		((lexer.Lookahead() == '$' || lexer.Lookahead() == '\'') && bshIsValid(validSymbols, bshTokRegexNoSlash)) ||
		(lexer.Lookahead() == '\'' && bshIsValid(validSymbols, bshTokRegexNoSpace)) {

		if lexer.Lookahead() == '$' && bshIsValid(validSymbols, bshTokRegexNoSlash) {
			lexer.MarkEnd()
			bshAdvance(lexer)
			if lexer.Lookahead() == '(' {
				return false
			}
		}

		lexer.MarkEnd()

		type regexState struct {
			done                     bool
			advancedOnce             bool
			foundNonAlnumDollarUDash bool
			lastWasEscape            bool
			inSingleQuote            bool
			parenDepth               uint32
			bracketDepth             uint32
			braceDepth               uint32
		}
		st := regexState{}

		for !st.done {
			if st.inSingleQuote {
				if lexer.Lookahead() == '\'' {
					st.inSingleQuote = false
					bshAdvance(lexer)
					lexer.MarkEnd()
				}
			}
			switch lexer.Lookahead() {
			case '\\':
				st.lastWasEscape = true
			case 0:
				return false
			case '(':
				st.parenDepth++
				st.lastWasEscape = false
			case '[':
				st.bracketDepth++
				st.lastWasEscape = false
			case '{':
				if !st.lastWasEscape {
					st.braceDepth++
				}
				st.lastWasEscape = false
			case ')':
				if st.parenDepth == 0 {
					st.done = true
				}
				st.parenDepth--
				st.lastWasEscape = false
			case ']':
				if st.bracketDepth == 0 {
					st.done = true
				}
				st.bracketDepth--
				st.lastWasEscape = false
			case '}':
				if st.braceDepth == 0 {
					st.done = true
				}
				st.braceDepth--
				st.lastWasEscape = false
			case '\'':
				st.inSingleQuote = !st.inSingleQuote
				bshAdvance(lexer)
				st.advancedOnce = true
				st.lastWasEscape = false
				continue
			default:
				st.lastWasEscape = false
			}

			if !st.done {
				if bshIsValid(validSymbols, bshTokRegex) {
					wasSpace := !st.inSingleQuote && bshIsSpace(lexer.Lookahead())
					bshAdvance(lexer)
					st.advancedOnce = true
					if !wasSpace || st.parenDepth > 0 {
						lexer.MarkEnd()
					}
				} else if bshIsValid(validSymbols, bshTokRegexNoSlash) {
					if lexer.Lookahead() == '/' {
						lexer.MarkEnd()
						lexer.SetResultSymbol(bshSymRegexNoSlash)
						return st.advancedOnce
					}
					if lexer.Lookahead() == '\\' {
						bshAdvance(lexer)
						st.advancedOnce = true
						if lexer.Lookahead() != 0 && lexer.Lookahead() != '[' && lexer.Lookahead() != '/' {
							bshAdvance(lexer)
							lexer.MarkEnd()
						}
					} else {
						wasSpace := !st.inSingleQuote && bshIsSpace(lexer.Lookahead())
						bshAdvance(lexer)
						st.advancedOnce = true
						if !wasSpace {
							lexer.MarkEnd()
						}
					}
				} else if bshIsValid(validSymbols, bshTokRegexNoSpace) {
					if lexer.Lookahead() == '\\' {
						st.foundNonAlnumDollarUDash = true
						bshAdvance(lexer)
						if lexer.Lookahead() != 0 {
							bshAdvance(lexer)
						}
					} else if lexer.Lookahead() == '$' {
						lexer.MarkEnd()
						bshAdvance(lexer)
						if lexer.Lookahead() == '(' {
							return false
						}
						if bshIsSpace(lexer.Lookahead()) {
							lexer.SetResultSymbol(bshSymRegexNoSpace)
							lexer.MarkEnd()
							return true
						}
					} else {
						wasSpace := !st.inSingleQuote && bshIsSpace(lexer.Lookahead())
						if wasSpace && st.parenDepth == 0 {
							lexer.MarkEnd()
							lexer.SetResultSymbol(bshSymRegexNoSpace)
							return st.foundNonAlnumDollarUDash
						}
						if !bshIsAlnum(lexer.Lookahead()) && lexer.Lookahead() != '$' && lexer.Lookahead() != '-' && lexer.Lookahead() != '_' {
							st.foundNonAlnumDollarUDash = true
						}
						bshAdvance(lexer)
					}
				}
			}
		}

		if bshIsValid(validSymbols, bshTokRegexNoSlash) {
			lexer.SetResultSymbol(bshSymRegexNoSlash)
		} else if bshIsValid(validSymbols, bshTokRegexNoSpace) {
			lexer.SetResultSymbol(bshSymRegexNoSpace)
		} else {
			lexer.SetResultSymbol(bshSymRegex)
		}
		if bshIsValid(validSymbols, bshTokRegex) && !st.advancedOnce {
			return false
		}
		return true
	}
	return false
}

// bshScanExtglobPattern handles the EXTGLOB_PATTERN token.
func bshScanExtglobPattern(s *bshState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !bshIsValid(validSymbols, bshTokExtglobPattern) || bshInErrorRecovery(validSymbols) {
		return false
	}

	for bshIsSpace(lexer.Lookahead()) {
		bshSkip(lexer)
	}

	la := lexer.Lookahead()
	if la == '?' || la == '*' || la == '+' || la == '@' ||
		la == '!' || la == '-' || la == ')' || la == '\\' ||
		la == '.' || la == '[' || bshIsAlpha(la) {

		if lexer.Lookahead() == '\\' {
			bshAdvance(lexer)
			if (bshIsSpace(lexer.Lookahead()) || lexer.Lookahead() == '"') &&
				lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' {
				bshAdvance(lexer)
			} else {
				return false
			}
		}

		if lexer.Lookahead() == ')' && s.lastGlobParenDepth == 0 {
			lexer.MarkEnd()
			bshAdvance(lexer)
			if bshIsSpace(lexer.Lookahead()) {
				return false
			}
		}

		lexer.MarkEnd()
		wasNonAlpha := !bshIsAlpha(lexer.Lookahead())
		if lexer.Lookahead() != '[' {
			// No esac
			if lexer.Lookahead() == 'e' {
				lexer.MarkEnd()
				bshAdvance(lexer)
				if lexer.Lookahead() == 's' {
					bshAdvance(lexer)
					if lexer.Lookahead() == 'a' {
						bshAdvance(lexer)
						if lexer.Lookahead() == 'c' {
							bshAdvance(lexer)
							if bshIsSpace(lexer.Lookahead()) {
								return false
							}
						}
					}
				}
			} else {
				bshAdvance(lexer)
			}
		}

		// -\w is just a word, find something else special
		if lexer.Lookahead() == '-' {
			lexer.MarkEnd()
			bshAdvance(lexer)
			for bshIsAlnum(lexer.Lookahead()) {
				bshAdvance(lexer)
			}
			if lexer.Lookahead() == ')' || lexer.Lookahead() == '\\' || lexer.Lookahead() == '.' {
				return false
			}
			lexer.MarkEnd()
		}

		// case item -) or *)
		if lexer.Lookahead() == ')' && s.lastGlobParenDepth == 0 {
			lexer.MarkEnd()
			bshAdvance(lexer)
			if bshIsSpace(lexer.Lookahead()) {
				lexer.SetResultSymbol(bshSymExtglobPattern)
				return wasNonAlpha
			}
		}

		if bshIsSpace(lexer.Lookahead()) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(bshSymExtglobPattern)
			s.lastGlobParenDepth = 0
			return true
		}

		if lexer.Lookahead() == '$' {
			lexer.MarkEnd()
			bshAdvance(lexer)
			if lexer.Lookahead() == '{' || lexer.Lookahead() == '(' {
				lexer.SetResultSymbol(bshSymExtglobPattern)
				return true
			}
		}

		if lexer.Lookahead() == '|' {
			lexer.MarkEnd()
			bshAdvance(lexer)
			lexer.SetResultSymbol(bshSymExtglobPattern)
			return true
		}

		if !bshIsAlnum(lexer.Lookahead()) && lexer.Lookahead() != '(' && lexer.Lookahead() != '"' &&
			lexer.Lookahead() != '[' && lexer.Lookahead() != '?' && lexer.Lookahead() != '/' &&
			lexer.Lookahead() != '\\' && lexer.Lookahead() != '_' && lexer.Lookahead() != '*' {
			return false
		}

		type extglobState struct {
			done         bool
			sawNonAlpha  bool
			parenDepth   uint32
			bracketDepth uint32
			braceDepth   uint32
		}
		est := extglobState{
			sawNonAlpha: wasNonAlpha,
			parenDepth:  uint32(s.lastGlobParenDepth),
		}

		for !est.done {
			switch lexer.Lookahead() {
			case 0:
				return false
			case '(':
				est.parenDepth++
			case '[':
				est.bracketDepth++
			case '{':
				est.braceDepth++
			case ')':
				if est.parenDepth == 0 {
					est.done = true
				}
				est.parenDepth--
			case ']':
				if est.bracketDepth == 0 {
					est.done = true
				}
				est.bracketDepth--
			case '}':
				if est.braceDepth == 0 {
					est.done = true
				}
				est.braceDepth--
			}

			if lexer.Lookahead() == '|' {
				lexer.MarkEnd()
				bshAdvance(lexer)
				if est.parenDepth == 0 && est.bracketDepth == 0 && est.braceDepth == 0 {
					lexer.SetResultSymbol(bshSymExtglobPattern)
					return true
				}
			}

			if !est.done {
				wasSpace := bshIsSpace(lexer.Lookahead())
				if lexer.Lookahead() == '$' {
					lexer.MarkEnd()
					if !bshIsAlpha(lexer.Lookahead()) && lexer.Lookahead() != '.' && lexer.Lookahead() != '\\' {
						est.sawNonAlpha = true
					}
					bshAdvance(lexer)
					if lexer.Lookahead() == '(' || lexer.Lookahead() == '{' {
						lexer.SetResultSymbol(bshSymExtglobPattern)
						s.lastGlobParenDepth = uint8(est.parenDepth)
						return est.sawNonAlpha
					}
				}
				if wasSpace {
					lexer.MarkEnd()
					lexer.SetResultSymbol(bshSymExtglobPattern)
					s.lastGlobParenDepth = 0
					return est.sawNonAlpha
				}
				if lexer.Lookahead() == '"' {
					lexer.MarkEnd()
					lexer.SetResultSymbol(bshSymExtglobPattern)
					s.lastGlobParenDepth = 0
					return est.sawNonAlpha
				}
				if lexer.Lookahead() == '\\' {
					if !bshIsAlpha(lexer.Lookahead()) && lexer.Lookahead() != '.' && lexer.Lookahead() != '\\' {
						est.sawNonAlpha = true
					}
					bshAdvance(lexer)
					if bshIsSpace(lexer.Lookahead()) || lexer.Lookahead() == '"' {
						bshAdvance(lexer)
					}
				} else {
					if !bshIsAlpha(lexer.Lookahead()) && lexer.Lookahead() != '.' && lexer.Lookahead() != '\\' {
						est.sawNonAlpha = true
					}
					bshAdvance(lexer)
				}
				if !wasSpace {
					lexer.MarkEnd()
				}
			}
		}

		lexer.SetResultSymbol(bshSymExtglobPattern)
		s.lastGlobParenDepth = 0
		return est.sawNonAlpha
	}

	s.lastGlobParenDepth = 0
	return false
}

// bshScanExpansionWord handles the EXPANSION_WORD token.
func bshScanExpansionWord(s *bshState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !bshIsValid(validSymbols, bshTokExpansionWord) {
		return false
	}

	advancedOnce := false
	advanceOnceSpace := false
	for {
		if lexer.Lookahead() == '"' {
			return false
		}
		if lexer.Lookahead() == '$' {
			lexer.MarkEnd()
			bshAdvance(lexer)
			if lexer.Lookahead() == '{' || lexer.Lookahead() == '(' || lexer.Lookahead() == '\'' ||
				bshIsAlnum(lexer.Lookahead()) {
				lexer.SetResultSymbol(bshSymExpansionWord)
				return advancedOnce
			}
			advancedOnce = true
		}

		if lexer.Lookahead() == '}' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(bshSymExpansionWord)
			return advancedOnce || advanceOnceSpace
		}

		if lexer.Lookahead() == '(' && !(advancedOnce || advanceOnceSpace) {
			lexer.MarkEnd()
			bshAdvance(lexer)
			for lexer.Lookahead() != ')' && lexer.Lookahead() != 0 {
				if lexer.Lookahead() == '$' {
					lexer.MarkEnd()
					bshAdvance(lexer)
					if lexer.Lookahead() == '{' || lexer.Lookahead() == '(' || lexer.Lookahead() == '\'' ||
						bshIsAlnum(lexer.Lookahead()) {
						lexer.SetResultSymbol(bshSymExpansionWord)
						return advancedOnce
					}
					advancedOnce = true
				} else {
					advancedOnce = advancedOnce || !bshIsSpace(lexer.Lookahead())
					advanceOnceSpace = advanceOnceSpace || bshIsSpace(lexer.Lookahead())
					bshAdvance(lexer)
				}
			}
			lexer.MarkEnd()
			if lexer.Lookahead() == ')' {
				advancedOnce = true
				bshAdvance(lexer)
				lexer.MarkEnd()
				if lexer.Lookahead() == '}' {
					return false
				}
			} else {
				return false
			}
		}

		if lexer.Lookahead() == '\'' {
			return false
		}

		if lexer.Lookahead() == 0 {
			return false
		}
		advancedOnce = advancedOnce || !bshIsSpace(lexer.Lookahead())
		advanceOnceSpace = advanceOnceSpace || bshIsSpace(lexer.Lookahead())
		bshAdvance(lexer)
	}
}

// bshScanBraceStart handles the BRACE_START token ({N..M}).
func bshScanBraceStart(s *bshState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !bshIsValid(validSymbols, bshTokBraceStart) || bshInErrorRecovery(validSymbols) {
		return false
	}

	for bshIsSpace(lexer.Lookahead()) {
		bshSkip(lexer)
	}

	if lexer.Lookahead() != '{' {
		return false
	}

	bshAdvance(lexer)
	lexer.MarkEnd()

	for lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
		bshAdvance(lexer)
	}

	if lexer.Lookahead() != '.' {
		return false
	}
	bshAdvance(lexer)

	if lexer.Lookahead() != '.' {
		return false
	}
	bshAdvance(lexer)

	for lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
		bshAdvance(lexer)
	}

	if lexer.Lookahead() != '}' {
		return false
	}

	lexer.SetResultSymbol(bshSymBraceStart)
	return true
}

// ---- main scan ----

func bshScan(s *bshState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// OPENING_PAREN / ESAC
	if bshIsValid(validSymbols, bshTokOpeningParen) && !bshInErrorRecovery(validSymbols) {
		if bshScanOpeningParen(lexer, validSymbols) {
			return true
		}
	}
	if bshIsValid(validSymbols, bshTokEsac) && !bshInErrorRecovery(validSymbols) {
		if bshScanEsac(lexer, validSymbols) {
			return true
		}
	}

	// CONCAT
	if bshIsValid(validSymbols, bshTokConcat) && !bshInErrorRecovery(validSymbols) {
		la := lexer.Lookahead()
		if !(la == 0 || bshIsSpace(la) || la == '>' || la == '<' || la == ')' || la == '(' ||
			la == ';' || la == '&' || la == '|' ||
			(la == '}' && bshIsValid(validSymbols, bshTokClosingBrace)) ||
			(la == ']' && bshIsValid(validSymbols, bshTokClosingBracket))) {

			lexer.SetResultSymbol(bshSymConcat)

			if lexer.Lookahead() == '`' {
				lexer.MarkEnd()
				bshAdvance(lexer)
				for lexer.Lookahead() != '`' && lexer.Lookahead() != 0 {
					bshAdvance(lexer)
				}
				if lexer.Lookahead() == 0 {
					return false
				}
				if lexer.Lookahead() == '`' {
					bshAdvance(lexer)
				}
				return bshIsSpace(lexer.Lookahead()) || lexer.Lookahead() == 0
			}

			if lexer.Lookahead() == '\\' {
				lexer.MarkEnd()
				bshAdvance(lexer)
				if lexer.Lookahead() == '"' || lexer.Lookahead() == '\'' || lexer.Lookahead() == '\\' {
					return true
				}
				if lexer.Lookahead() == 0 {
					return false
				}
			} else {
				return true
			}
		}
		if bshIsSpace(lexer.Lookahead()) && bshIsValid(validSymbols, bshTokClosingBrace) && !bshIsValid(validSymbols, bshTokExpansionWord) {
			lexer.SetResultSymbol(bshSymConcat)
			return true
		}
	}

	// IMMEDIATE_DOUBLE_HASH
	if bshIsValid(validSymbols, bshTokImmediateDoubleHash) && !bshInErrorRecovery(validSymbols) {
		if lexer.Lookahead() == '#' {
			lexer.MarkEnd()
			bshAdvance(lexer)
			if lexer.Lookahead() == '#' {
				bshAdvance(lexer)
				if lexer.Lookahead() != '}' {
					lexer.SetResultSymbol(bshSymImmediateDoubleHash)
					lexer.MarkEnd()
					return true
				}
			}
		}
	}

	// EXTERNAL_EXPANSION_SYM_HASH / BANG / EQUAL
	if bshIsValid(validSymbols, bshTokExternalExpansionSymHash) && !bshInErrorRecovery(validSymbols) {
		if lexer.Lookahead() == '#' || lexer.Lookahead() == '=' || lexer.Lookahead() == '!' {
			var sym gotreesitter.Symbol
			switch lexer.Lookahead() {
			case '#':
				sym = bshSymExternalExpansionSymHash
			case '!':
				sym = bshSymExternalExpansionSymBang
			default:
				sym = bshSymExternalExpansionSymEq
			}
			lexer.SetResultSymbol(sym)
			bshAdvance(lexer)
			lexer.MarkEnd()
			for lexer.Lookahead() == '#' || lexer.Lookahead() == '=' || lexer.Lookahead() == '!' {
				bshAdvance(lexer)
			}
			for bshIsSpace(lexer.Lookahead()) {
				bshSkip(lexer)
			}
			return lexer.Lookahead() == '}'
		}
	}

	// EMPTY_VALUE
	if bshIsValid(validSymbols, bshTokEmptyValue) {
		la := lexer.Lookahead()
		if bshIsSpace(la) || la == 0 || la == ';' || la == '&' {
			lexer.SetResultSymbol(bshSymEmptyValue)
			return true
		}
	}

	// HEREDOC_BODY_BEGINNING / SIMPLE_HEREDOC_BODY
	if (bshIsValid(validSymbols, bshTokHeredocBodyBeginning) || bshIsValid(validSymbols, bshTokSimpleHeredocBody)) &&
		len(s.heredocs) > 0 && !s.heredocs[len(s.heredocs)-1].started && !bshInErrorRecovery(validSymbols) {
		return bshScanHeredocContent(s, lexer,
			bshTokHeredocBodyBeginning, bshTokSimpleHeredocBody,
			bshSymHeredocBodyBeginning, bshSymSimpleHeredocBody)
	}

	// HEREDOC_END
	if bshIsValid(validSymbols, bshTokHeredocEnd) && len(s.heredocs) > 0 {
		heredoc := &s.heredocs[len(s.heredocs)-1]
		if bshScanHeredocEndIdentifier(heredoc, lexer) {
			heredoc.currentLeadingWord = nil
			heredoc.delimiter = nil
			s.heredocs = s.heredocs[:len(s.heredocs)-1]
			lexer.SetResultSymbol(bshSymHeredocEnd)
			return true
		}
	}

	// HEREDOC_CONTENT
	if bshIsValid(validSymbols, bshTokHeredocContent) && len(s.heredocs) > 0 &&
		s.heredocs[len(s.heredocs)-1].started && !bshInErrorRecovery(validSymbols) {
		return bshScanHeredocContent(s, lexer,
			bshTokHeredocContent, bshTokHeredocEnd,
			bshSymHeredocContent, bshSymHeredocEnd)
	}

	// HEREDOC_START
	if bshIsValid(validSymbols, bshTokHeredocStart) && !bshInErrorRecovery(validSymbols) && len(s.heredocs) > 0 {
		return bshScanHeredocStart(&s.heredocs[len(s.heredocs)-1], lexer)
	}

	// TEST_OPERATOR
	if bshIsValid(validSymbols, bshTokTestOperator) && !bshIsValid(validSymbols, bshTokExpansionWord) {
		for bshIsSpace(lexer.Lookahead()) && lexer.Lookahead() != '\n' {
			bshSkip(lexer)
		}

		if lexer.Lookahead() == '\\' {
			if bshIsValid(validSymbols, bshTokExtglobPattern) {
				return bshScanExtglobPattern(s, lexer, validSymbols)
			}
			if bshIsValid(validSymbols, bshTokRegexNoSpace) {
				return bshScanRegex(s, lexer, validSymbols)
			}
			bshSkip(lexer)

			if lexer.Lookahead() == 0 {
				return false
			}

			if lexer.Lookahead() == '\r' {
				bshSkip(lexer)
				if lexer.Lookahead() == '\n' {
					bshSkip(lexer)
				}
			} else if lexer.Lookahead() == '\n' {
				bshSkip(lexer)
			} else {
				return false
			}

			for bshIsSpace(lexer.Lookahead()) {
				bshSkip(lexer)
			}
		}

		if lexer.Lookahead() == '\n' && !bshIsValid(validSymbols, bshTokNewline) {
			bshSkip(lexer)
			for bshIsSpace(lexer.Lookahead()) {
				bshSkip(lexer)
			}
		}

		if lexer.Lookahead() == '-' {
			bshAdvance(lexer)

			advancedOnce := false
			for bshIsAlpha(lexer.Lookahead()) {
				advancedOnce = true
				bshAdvance(lexer)
			}

			if bshIsSpace(lexer.Lookahead()) && advancedOnce {
				lexer.MarkEnd()
				bshAdvance(lexer)
				if lexer.Lookahead() == '}' && bshIsValid(validSymbols, bshTokClosingBrace) {
					if bshIsValid(validSymbols, bshTokExpansionWord) {
						lexer.MarkEnd()
						lexer.SetResultSymbol(bshSymExpansionWord)
						return true
					}
					return false
				}
				lexer.SetResultSymbol(bshSymTestOperator)
				return true
			}
			if bshIsSpace(lexer.Lookahead()) && bshIsValid(validSymbols, bshTokExtglobPattern) {
				lexer.SetResultSymbol(bshSymExtglobPattern)
				return true
			}
		}

		if bshIsValid(validSymbols, bshTokBareDollar) && !bshInErrorRecovery(validSymbols) && bshScanBareDollar(lexer) {
			return true
		}
	}

	// VARIABLE_NAME / FILE_DESCRIPTOR / HEREDOC_ARROW
	if (bshIsValid(validSymbols, bshTokVariableName) || bshIsValid(validSymbols, bshTokFileDescriptor) ||
		bshIsValid(validSymbols, bshTokHeredocArrow)) &&
		!bshIsValid(validSymbols, bshTokRegexNoSlash) && !bshInErrorRecovery(validSymbols) {

		for {
			if (lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' || lexer.Lookahead() == '\r' ||
				(lexer.Lookahead() == '\n' && !bshIsValid(validSymbols, bshTokNewline))) &&
				!bshIsValid(validSymbols, bshTokExpansionWord) {
				bshSkip(lexer)
			} else if lexer.Lookahead() == '\\' {
				bshSkip(lexer)

				if lexer.Lookahead() == 0 {
					lexer.MarkEnd()
					lexer.SetResultSymbol(bshSymVariableName)
					return true
				}

				if lexer.Lookahead() == '\r' {
					bshSkip(lexer)
				}
				if lexer.Lookahead() == '\n' {
					bshSkip(lexer)
				} else {
					if lexer.Lookahead() == '\\' && bshIsValid(validSymbols, bshTokExpansionWord) {
						return bshScanExpansionWord(s, lexer, validSymbols)
					}
					return false
				}
			} else {
				break
			}
		}

		// no '*', '@', '?', '-', '$', '0', '_'
		if !bshIsValid(validSymbols, bshTokExpansionWord) {
			la := lexer.Lookahead()
			if la == '*' || la == '@' || la == '?' || la == '-' || la == '0' || la == '_' {
				lexer.MarkEnd()
				bshAdvance(lexer)
				la2 := lexer.Lookahead()
				if la2 == '=' || la2 == '[' || la2 == ':' || la2 == '-' || la2 == '%' || la2 == '#' || la2 == '/' {
					return false
				}
				if bshIsValid(validSymbols, bshTokExtglobPattern) && bshIsSpace(lexer.Lookahead()) {
					lexer.MarkEnd()
					lexer.SetResultSymbol(bshSymExtglobPattern)
					return true
				}
			}
		}

		// HEREDOC_ARROW
		if bshIsValid(validSymbols, bshTokHeredocArrow) && lexer.Lookahead() == '<' {
			bshAdvance(lexer)
			if lexer.Lookahead() == '<' {
				bshAdvance(lexer)
				if lexer.Lookahead() == '-' {
					bshAdvance(lexer)
					hd := bshHeredoc{allowsIndent: true}
					s.heredocs = append(s.heredocs, hd)
					lexer.SetResultSymbol(bshSymHeredocArrowDash)
				} else if lexer.Lookahead() == '<' || lexer.Lookahead() == '=' {
					return false
				} else {
					hd := bshHeredoc{}
					s.heredocs = append(s.heredocs, hd)
					lexer.SetResultSymbol(bshSymHeredocArrow)
				}
				return true
			}
			return false
		}

		isNumber := true
		if bshIsDigit(lexer.Lookahead()) {
			bshAdvance(lexer)
		} else if bshIsAlpha(lexer.Lookahead()) || lexer.Lookahead() == '_' {
			isNumber = false
			bshAdvance(lexer)
		} else {
			if lexer.Lookahead() == '{' {
				return bshScanBraceStart(s, lexer, validSymbols)
			}
			if bshIsValid(validSymbols, bshTokExpansionWord) {
				return bshScanExpansionWord(s, lexer, validSymbols)
			}
			if bshIsValid(validSymbols, bshTokExtglobPattern) {
				return bshScanExtglobPattern(s, lexer, validSymbols)
			}
			return false
		}

		for {
			if bshIsDigit(lexer.Lookahead()) {
				bshAdvance(lexer)
			} else if bshIsAlpha(lexer.Lookahead()) || lexer.Lookahead() == '_' {
				isNumber = false
				bshAdvance(lexer)
			} else {
				break
			}
		}

		if isNumber && bshIsValid(validSymbols, bshTokFileDescriptor) &&
			(lexer.Lookahead() == '>' || lexer.Lookahead() == '<') {
			lexer.SetResultSymbol(bshSymFileDescriptor)
			return true
		}

		if bshIsValid(validSymbols, bshTokVariableName) {
			if lexer.Lookahead() == '+' {
				lexer.MarkEnd()
				bshAdvance(lexer)
				if lexer.Lookahead() == '=' || lexer.Lookahead() == ':' || bshIsValid(validSymbols, bshTokClosingBrace) {
					lexer.SetResultSymbol(bshSymVariableName)
					return true
				}
				return false
			}
			if lexer.Lookahead() == '/' {
				return false
			}
			if lexer.Lookahead() == '=' || lexer.Lookahead() == '[' ||
				(lexer.Lookahead() == ':' && !bshIsValid(validSymbols, bshTokClosingBrace) &&
					!bshIsValid(validSymbols, bshTokOpeningParen)) ||
				lexer.Lookahead() == '%' ||
				(lexer.Lookahead() == '#' && !isNumber) || lexer.Lookahead() == '@' ||
				(lexer.Lookahead() == '-' && bshIsValid(validSymbols, bshTokClosingBrace)) {
				lexer.MarkEnd()
				lexer.SetResultSymbol(bshSymVariableName)
				return true
			}

			if lexer.Lookahead() == '?' {
				lexer.MarkEnd()
				bshAdvance(lexer)
				lexer.SetResultSymbol(bshSymVariableName)
				return bshIsAlpha(lexer.Lookahead())
			}
		}

		return false
	}

	// BARE_DOLLAR (standalone)
	if bshIsValid(validSymbols, bshTokBareDollar) && !bshInErrorRecovery(validSymbols) && bshScanBareDollar(lexer) {
		return true
	}

	// REGEX / REGEX_NO_SLASH / REGEX_NO_SPACE
	if (bshIsValid(validSymbols, bshTokRegex) || bshIsValid(validSymbols, bshTokRegexNoSlash) ||
		bshIsValid(validSymbols, bshTokRegexNoSpace)) && !bshInErrorRecovery(validSymbols) {
		if bshScanRegex(s, lexer, validSymbols) {
			return true
		}
	}

	// EXTGLOB_PATTERN
	if bshIsValid(validSymbols, bshTokExtglobPattern) && !bshInErrorRecovery(validSymbols) {
		if bshScanExtglobPattern(s, lexer, validSymbols) {
			return true
		}
	}

	// EXPANSION_WORD
	if bshIsValid(validSymbols, bshTokExpansionWord) {
		if bshScanExpansionWord(s, lexer, validSymbols) {
			return true
		}
	}

	// BRACE_START
	if bshIsValid(validSymbols, bshTokBraceStart) && !bshInErrorRecovery(validSymbols) {
		if bshScanBraceStart(s, lexer, validSymbols) {
			return true
		}
	}

	return false
}
