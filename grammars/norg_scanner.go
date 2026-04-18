//go:build !grammar_subset || grammar_subset_norg

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Norg external token types. These match the indices into the
// externals array in the grammar, which map 1:1 to the C++ TokenType enum.
const (
	norgNONE = iota
	norgSPACE
	norgWORD
	norgCAPITALIZED_WORD
	norgLINE_BREAK
	norgPARAGRAPH_BREAK
	norgESCAPE_SEQUENCE
	norgTRAILING_MODIFIER
	norgDETACHED_MODIFIER_EXTENSION_BEGIN
	norgMODIFIER_EXTENSION_DELIMITER
	norgDETACHED_MODIFIER_EXTENSION_END
	norgPRIORITY
	norgTIMESTAMP
	norgTODO_ITEM_UNDONE
	norgTODO_ITEM_PENDING
	norgTODO_ITEM_DONE
	norgTODO_ITEM_ON_HOLD
	norgTODO_ITEM_CANCELLED
	norgTODO_ITEM_URGENT
	norgTODO_ITEM_UNCERTAIN
	norgTODO_ITEM_RECURRING
	norgHEADING1
	norgHEADING2
	norgHEADING3
	norgHEADING4
	norgHEADING5
	norgHEADING6
	norgQUOTE1
	norgQUOTE2
	norgQUOTE3
	norgQUOTE4
	norgQUOTE5
	norgQUOTE6
	norgUNORDERED_LIST1
	norgUNORDERED_LIST2
	norgUNORDERED_LIST3
	norgUNORDERED_LIST4
	norgUNORDERED_LIST5
	norgUNORDERED_LIST6
	norgORDERED_LIST1
	norgORDERED_LIST2
	norgORDERED_LIST3
	norgORDERED_LIST4
	norgORDERED_LIST5
	norgORDERED_LIST6
	norgSINGLE_DEFINITION
	norgMULTI_DEFINITION
	norgMULTI_DEFINITION_SUFFIX
	norgSINGLE_FOOTNOTE
	norgMULTI_FOOTNOTE
	norgMULTI_FOOTNOTE_SUFFIX
	norgSINGLE_TABLE_CELL
	norgMULTI_TABLE_CELL
	norgMULTI_TABLE_CELL_SUFFIX
	norgSTRONG_PARAGRAPH_DELIMITER
	norgWEAK_PARAGRAPH_DELIMITER
	norgHORIZONTAL_LINE
	norgLINK_DESCRIPTION_BEGIN
	norgLINK_DESCRIPTION_END
	norgLINK_LOCATION_BEGIN
	norgLINK_LOCATION_END
	norgLINK_FILE_BEGIN
	norgLINK_FILE_END
	norgLINK_FILE_TEXT
	norgLINK_TARGET_URL
	norgLINK_TARGET_LINE_NUMBER
	norgLINK_TARGET_WIKI
	norgLINK_TARGET_GENERIC
	norgLINK_TARGET_EXTERNAL_FILE
	norgLINK_TARGET_TIMESTAMP
	norgLINK_TARGET_DEFINITION
	norgLINK_TARGET_FOOTNOTE
	norgLINK_TARGET_HEADING1
	norgLINK_TARGET_HEADING2
	norgLINK_TARGET_HEADING3
	norgLINK_TARGET_HEADING4
	norgLINK_TARGET_HEADING5
	norgLINK_TARGET_HEADING6
	norgTIMESTAMP_DATA
	norgPRIORITY_DATA
	norgTAG_DELIMITER
	norgMACRO_TAG
	norgMACRO_TAG_END
	norgRANGED_TAG
	norgRANGED_TAG_END
	norgRANGED_VERBATIM_TAG
	norgRANGED_VERBATIM_TAG_END
	norgINFIRM_TAG
	norgWEAK_CARRYOVER
	norgSTRONG_CARRYOVER
	norgLINK_MODIFIER
	norgINTERSECTING_MODIFIER
	norgATTACHED_MODIFIER_BEGIN
	norgATTACHED_MODIFIER_END
	norgBOLD_OPEN
	norgBOLD_CLOSE
	norgITALIC_OPEN
	norgITALIC_CLOSE
	norgSTRIKETHROUGH_OPEN
	norgSTRIKETHROUGH_CLOSE
	norgUNDERLINE_OPEN
	norgUNDERLINE_CLOSE
	norgSPOILER_OPEN
	norgSPOILER_CLOSE
	norgSUPERSCRIPT_OPEN
	norgSUPERSCRIPT_CLOSE
	norgSUBSCRIPT_OPEN
	norgSUBSCRIPT_CLOSE
	norgVERBATIM_OPEN
	norgVERBATIM_CLOSE
	norgINLINE_COMMENT_OPEN
	norgINLINE_COMMENT_CLOSE
	norgINLINE_MATH_OPEN
	norgINLINE_MATH_CLOSE
	norgINLINE_MACRO_OPEN
	norgINLINE_MACRO_CLOSE
	norgFREE_FORM_MODIFIER_OPEN
	norgFREE_FORM_MODIFIER_CLOSE
	norgINLINE_LINK_TARGET_OPEN
	norgINLINE_LINK_TARGET_CLOSE
	norgSLIDE
	norgINDENT_SEGMENT
)

// norgTagType tracks verbatim/ranged tag context.
type norgTagType int8

const (
	norgTagNone          norgTagType = 1
	norgTagOnTag         norgTagType = 2
	norgTagInTag         norgTagType = 3
	norgTagOnVerbatimTag norgTagType = 4
	norgTagInVerbatimTag norgTagType = 5
)

// norgState is the persistent scanner state.
type norgState struct {
	previous        rune
	current         rune
	tagContext      norgTagType
	tagLevel        int
	inLinkLocation  bool
	lastToken       int
	parsedChars     int
	activeModifiers uint16 // bitset for (BOLD..INLINE_MACRO) open/close pairs
}

const norgSymBase gotreesitter.Symbol = 3

func norgSym(tok int) gotreesitter.Symbol { return norgSymBase + gotreesitter.Symbol(tok) }

func norgModIdx(tok int) int { return (tok - norgBOLD_OPEN) / 2 }

func (s *norgState) isModActive(tok int) bool {
	return s.activeModifiers&(1<<uint(norgModIdx(tok))) != 0
}

func (s *norgState) setMod(tok int) {
	s.activeModifiers |= 1 << uint(norgModIdx(tok))
}

func (s *norgState) clearMod(tok int) {
	s.activeModifiers &^= 1 << uint(norgModIdx(tok))
}

func (s *norgState) resetMods() { s.activeModifiers = 0 }

// NorgExternalScanner handles norg markup disambiguation.
type NorgExternalScanner struct{}

func (NorgExternalScanner) Create() any   { return &norgState{tagContext: norgTagNone} }
func (NorgExternalScanner) Destroy(_ any) {}

func (NorgExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*norgState)
	if len(buf) < 19 {
		return 0
	}
	buf[0] = byte(s.lastToken)
	buf[1] = byte(s.tagLevel)
	buf[2] = byte(s.tagContext)
	if s.inLinkLocation {
		buf[3] = 1
	} else {
		buf[3] = 0
	}
	buf[4] = byte(s.current)
	buf[5] = byte(s.current >> 8)
	buf[6] = byte(s.current >> 16)
	buf[7] = byte(s.current >> 24)
	// Active modifiers bitset
	for i := 0; i < 11; i++ {
		if s.activeModifiers&(1<<uint(i)) != 0 {
			buf[8+i] = 1
		} else {
			buf[8+i] = 0
		}
	}
	return 19
}

func (NorgExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*norgState)
	if len(buf) == 0 {
		s.tagLevel = 0
		s.tagContext = norgTagNone
		s.inLinkLocation = false
		s.lastToken = norgNONE
		s.current = 0
		s.activeModifiers = 0
		return
	}
	s.lastToken = int(buf[0])
	s.tagLevel = int(buf[1])
	s.tagContext = norgTagType(buf[2])
	s.inLinkLocation = buf[3] != 0
	s.current = rune(buf[4]) | rune(buf[5])<<8 | rune(buf[6])<<16 | rune(buf[7])<<24
	s.activeModifiers = 0
	for i := 0; i < 11 && 8+i < len(buf); i++ {
		if buf[8+i] != 0 {
			s.activeModifiers |= 1 << uint(i)
		}
	}
}

func (NorgExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*norgState)
	return norgScan(s, lexer, validSymbols)
}

func norgIsNewline(ch rune) bool { return ch == 0 || ch == '\n' || ch == '\r' }
func norgIsBlank(ch rune) bool   { return ch != 0 && (ch == ' ' || ch == '\t' || ch == '\v') }

func norgAdvance(s *norgState, lexer *gotreesitter.ExternalLexer) {
	s.previous = s.current
	s.current = lexer.Lookahead()
	lexer.Advance(false)
}

func norgSkip(s *norgState, lexer *gotreesitter.ExternalLexer) {
	s.previous = s.current
	s.current = lexer.Lookahead()
	lexer.Advance(true)
}

func norgSetResult(s *norgState, lexer *gotreesitter.ExternalLexer, tok int) {
	lexer.MarkEnd()
	lexer.SetResultSymbol(norgSym(tok))
	s.lastToken = tok
}

func norgToken(lexer *gotreesitter.ExternalLexer, str string) bool {
	for _, ch := range str {
		if lexer.Lookahead() == ch {
			lexer.Advance(false)
		} else {
			return false
		}
	}
	return true
}

// norgDetachedModifiers lists the characters that start detached modifiers.
var norgDetachedModifiers = [12]rune{
	'*', '-', '>', '%', '=', '~', '$', '_', '^', '&', '<', ':',
}

func norgIsDetachedMod(ch rune) bool {
	for _, m := range norgDetachedModifiers {
		if ch == m {
			return true
		}
	}
	return false
}

var norgAttachedModifiers = map[rune]int{
	'*': norgBOLD_OPEN,
	'/': norgITALIC_OPEN,
	'-': norgSTRIKETHROUGH_OPEN,
	'_': norgUNDERLINE_OPEN,
	'!': norgSPOILER_OPEN,
	'`': norgVERBATIM_OPEN,
	'^': norgSUPERSCRIPT_OPEN,
	',': norgSUBSCRIPT_OPEN,
	'%': norgINLINE_COMMENT_OPEN,
	'$': norgINLINE_MATH_OPEN,
	'&': norgINLINE_MACRO_OPEN,
}

func norgScan(s *norgState, lexer *gotreesitter.ExternalLexer, valid []bool) bool {
	// EOF check
	if lexer.Lookahead() == 0 {
		s.resetMods()
		return false
	}

	if s.lastToken == norgTRAILING_MODIFIER {
		norgAdvance(s, lexer)
		return norgParseText(s, lexer, valid)
	}

	if norgIsNewline(lexer.Lookahead()) {
		norgAdvance(s, lexer)
		norgSetResult(s, lexer, norgLINE_BREAK)

		if lexer.Lookahead() == 0 {
			s.resetMods()
			return true
		}

		if s.tagContext != norgTagNone && int(s.tagContext)%2 == 0 {
			s.tagContext++
			return true
		}

		if norgIsNewline(lexer.Lookahead()) {
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgPARAGRAPH_BREAK)
			s.resetMods()
		}
		return true
	}

	// Beginning of line: check detached modifiers
	if lexer.Column() == 0 {
		// Skip leading whitespace
		for norgIsBlank(lexer.Lookahead()) {
			norgSkip(s, lexer)
		}

		// Ranged verbatim tag: @something
		if lexer.Lookahead() == '@' {
			norgAdvance(s, lexer)
			lexer.MarkEnd()

			if norgToken(lexer, "end") && (unicode.IsSpace(lexer.Lookahead()) || lexer.Lookahead() == 0) {
				for norgIsBlank(lexer.Lookahead()) {
					norgAdvance(s, lexer)
				}
				if (unicode.IsSpace(lexer.Lookahead()) || lexer.Lookahead() == 0) &&
					s.tagContext == norgTagInVerbatimTag {
					norgSetResult(s, lexer, norgRANGED_VERBATIM_TAG_END)
					s.tagContext = norgTagNone
					return true
				}
				norgSetResult(s, lexer, norgWORD)
				return true
			}

			if s.lastToken == norgRANGED_VERBATIM_TAG || s.tagContext == norgTagInVerbatimTag {
				norgSetResult(s, lexer, norgWORD)
				return true
			}

			norgSetResult(s, lexer, norgRANGED_VERBATIM_TAG)
			s.tagContext = norgTagOnVerbatimTag
			return true
		}

		if s.tagContext == norgTagInVerbatimTag {
			return norgParseText(s, lexer, valid)
		}

		// Macro tag: =something
		if lexer.Lookahead() == '=' && s.tagContext != norgTagInVerbatimTag {
			norgAdvance(s, lexer)
			lexer.MarkEnd()

			if norgToken(lexer, "end") && (unicode.IsSpace(lexer.Lookahead()) || lexer.Lookahead() == 0) {
				for lexer.Lookahead() != 0 && unicode.IsSpace(lexer.Lookahead()) && !norgIsNewline(lexer.Lookahead()) {
					norgAdvance(s, lexer)
				}
				if (unicode.IsSpace(lexer.Lookahead()) || lexer.Lookahead() == 0) && s.tagLevel > 0 {
					norgSetResult(s, lexer, norgMACRO_TAG_END)
					s.tagLevel--
					return true
				}
				norgSetResult(s, lexer, norgWORD)
				return true
			} else if lexer.Lookahead() == '=' {
				norgAdvance(s, lexer)
				if lexer.Lookahead() == '=' {
					for lexer.Lookahead() == '=' {
						norgAdvance(s, lexer)
					}
					if norgIsNewline(lexer.Lookahead()) {
						lexer.MarkEnd()
						norgAdvance(s, lexer)
						norgSetResult(s, lexer, norgSTRONG_PARAGRAPH_DELIMITER)
						return true
					}
					lexer.MarkEnd()
					norgAdvance(s, lexer)
					norgSetResult(s, lexer, norgWORD)
					return true
				}
				lexer.MarkEnd()
				norgSetResult(s, lexer, norgWORD)
				return true
			}

			if s.lastToken == norgMACRO_TAG {
				norgSetResult(s, lexer, norgWORD)
				return true
			}

			norgSetResult(s, lexer, norgMACRO_TAG)
			s.tagContext = norgTagOnTag
			s.tagLevel++
			return true
		}

		// Ranged tag: |something
		if lexer.Lookahead() == '|' && s.tagContext != norgTagInVerbatimTag {
			norgAdvance(s, lexer)
			lexer.MarkEnd()

			if norgToken(lexer, "end") && (unicode.IsSpace(lexer.Lookahead()) || lexer.Lookahead() == 0) {
				for norgIsBlank(lexer.Lookahead()) {
					norgAdvance(s, lexer)
				}
				if (unicode.IsSpace(lexer.Lookahead()) || lexer.Lookahead() == 0) && s.tagLevel > 0 {
					norgSetResult(s, lexer, norgRANGED_TAG_END)
					s.tagLevel--
					return true
				}
				norgSetResult(s, lexer, norgWORD)
				return true
			}

			if s.lastToken == norgRANGED_TAG {
				norgSetResult(s, lexer, norgWORD)
				return true
			}

			norgSetResult(s, lexer, norgRANGED_TAG)
			s.tagContext = norgTagOnTag
			s.tagLevel++
			return true
		}

		// Strong carryover: #something
		if lexer.Lookahead() == '#' && s.tagContext != norgTagInVerbatimTag {
			norgAdvance(s, lexer)
			if lexer.Lookahead() == 0 || unicode.IsSpace(lexer.Lookahead()) {
				if norgIsNewline(lexer.Lookahead()) {
					norgSetResult(s, lexer, norgINDENT_SEGMENT)
				} else {
					norgSetResult(s, lexer, norgWORD)
				}
				return true
			}
			norgSetResult(s, lexer, norgSTRONG_CARRYOVER)
			return true
		}

		// Weak carryover: +something
		if lexer.Lookahead() == '+' && s.tagContext != norgTagInVerbatimTag {
			norgAdvance(s, lexer)
			if lexer.Lookahead() != '+' {
				norgSetResult(s, lexer, norgWEAK_CARRYOVER)
				return true
			}
		}

		// Infirm tag: .something
		if lexer.Lookahead() == '.' && s.tagContext != norgTagInVerbatimTag {
			norgAdvance(s, lexer)
			if lexer.Lookahead() != '.' {
				norgSetResult(s, lexer, norgINFIRM_TAG)
				return true
			}
		}

		// Detached modifier checks
		if norgCheckDetached(s, lexer, []int{norgHEADING1, norgHEADING2, norgHEADING3, norgHEADING4, norgHEADING5, norgHEADING6}, '*') {
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgQUOTE1, norgQUOTE2, norgQUOTE3, norgQUOTE4, norgQUOTE5, norgQUOTE6}, '>') {
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgUNORDERED_LIST1, norgUNORDERED_LIST2, norgUNORDERED_LIST3, norgUNORDERED_LIST4, norgUNORDERED_LIST5, norgUNORDERED_LIST6}, '-') {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars >= 3 {
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgWEAK_PARAGRAPH_DELIMITER)
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgORDERED_LIST1, norgORDERED_LIST2, norgORDERED_LIST3, norgORDERED_LIST4, norgORDERED_LIST5, norgORDERED_LIST6}, '~') {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars == 1 {
			if lexer.Lookahead() == 0 {
				s.resetMods()
				return false
			}
			norgSetResult(s, lexer, norgTRAILING_MODIFIER)
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgSINGLE_DEFINITION, norgMULTI_DEFINITION, norgNONE}, '$') {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars == 2 {
			norgAdvance(s, lexer)
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgMULTI_DEFINITION_SUFFIX))
			s.lastToken = norgMULTI_DEFINITION_SUFFIX
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgSINGLE_FOOTNOTE, norgMULTI_FOOTNOTE, norgNONE}, '^') {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars == 2 {
			norgAdvance(s, lexer)
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgMULTI_FOOTNOTE_SUFFIX))
			s.lastToken = norgMULTI_FOOTNOTE_SUFFIX
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgSINGLE_TABLE_CELL, norgMULTI_TABLE_CELL, norgNONE}, ':') {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars == 2 {
			norgAdvance(s, lexer)
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgMULTI_TABLE_CELL_SUFFIX))
			s.lastToken = norgMULTI_TABLE_CELL_SUFFIX
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgNONE, norgNONE}, '_') {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars >= 3 {
			norgSetResult(s, lexer, norgHORIZONTAL_LINE)
			return true
		}
	}

	// Non-line-start handling
	switch lexer.Lookahead() {
	case '~':
		norgAdvance(s, lexer)
		lexer.MarkEnd()
		if norgIsNewline(lexer.Lookahead()) {
			norgAdvance(s, lexer)
			if lexer.Lookahead() == 0 {
				s.resetMods()
				return false
			}
			norgSetResult(s, lexer, norgTRAILING_MODIFIER)
			return true
		}
		return norgParseText(s, lexer, valid)
	case '\\':
		norgAdvance(s, lexer)
		norgSetResult(s, lexer, norgESCAPE_SEQUENCE)
		return true
	}

	if norgCheckDetachedModExtension(s, lexer) {
		return true
	}

	if (s.lastToken >= norgHEADING1 && s.lastToken <= norgMULTI_TABLE_CELL_SUFFIX) ||
		s.lastToken == norgDETACHED_MODIFIER_EXTENSION_END {
		if lexer.Lookahead() == ':' {
			norgAdvance(s, lexer)
			isIndent := false
			if lexer.Lookahead() == ':' {
				norgAdvance(s, lexer)
				isIndent = true
			}
			if !norgIsNewline(lexer.Lookahead()) {
				norgSetResult(s, lexer, norgWORD)
				return true
			}
			norgAdvance(s, lexer)
			if isIndent {
				norgSetResult(s, lexer, norgINDENT_SEGMENT)
			} else {
				norgSetResult(s, lexer, norgSLIDE)
			}
			return true
		}
	}

	switch lexer.Lookahead() {
	case '<':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(lexer.Lookahead()) {
			norgSetResult(s, lexer, norgINLINE_LINK_TARGET_OPEN)
			s.inLinkLocation = true
			return true
		}
	case '>':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(s.previous) && s.lastToken != norgLINK_LOCATION_BEGIN &&
			s.lastToken != norgLINK_FILE_END {
			norgSetResult(s, lexer, norgINLINE_LINK_TARGET_CLOSE)
			s.inLinkLocation = false
			return true
		}
	case '(':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(lexer.Lookahead()) && s.lastToken != norgNONE &&
			((s.lastToken >= norgBOLD_OPEN && s.lastToken <= norgINLINE_MACRO_CLOSE &&
				(s.lastToken%2) == (norgBOLD_CLOSE%2)) ||
				s.lastToken == norgLINK_DESCRIPTION_END ||
				s.lastToken == norgLINK_LOCATION_END ||
				s.lastToken == norgINLINE_LINK_TARGET_CLOSE) {
			norgSetResult(s, lexer, norgATTACHED_MODIFIER_BEGIN)
			return true
		}
		norgSetResult(s, lexer, norgWORD)
		return true
	case ')':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(s.previous) {
			norgSetResult(s, lexer, norgATTACHED_MODIFIER_END)
			return true
		}
	case '[':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(lexer.Lookahead()) {
			norgSetResult(s, lexer, norgLINK_DESCRIPTION_BEGIN)
			return true
		}
	case ']':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(s.previous) {
			norgSetResult(s, lexer, norgLINK_DESCRIPTION_END)
			return true
		}
	case '{':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(lexer.Lookahead()) {
			norgSetResult(s, lexer, norgLINK_LOCATION_BEGIN)
			s.inLinkLocation = true
			return true
		}
	case '}':
		norgAdvance(s, lexer)
		if norgIsNewline(s.previous) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgNONE))
			s.lastToken = norgNONE
			return true
		}
		if !unicode.IsSpace(s.previous) {
			norgSetResult(s, lexer, norgLINK_LOCATION_END)
			s.inLinkLocation = false
			return true
		}
	}

	if s.inLinkLocation {
		if norgCheckLinkLocation(s, lexer) {
			return true
		}
	}

	if norgCheckAttached(s, lexer) {
		return true
	}

	return norgParseText(s, lexer, valid)
}

func norgCheckDetached(s *norgState, lexer *gotreesitter.ExternalLexer, results []int, expected rune) bool {
	s.parsedChars = 0
	i := 0

	for {
		if lexer.Lookahead() != expected {
			break
		}
		norgAdvance(s, lexer)

		if norgIsBlank(lexer.Lookahead()) {
			maxIdx := len(results) - 1
			idx := i
			if idx > maxIdx {
				idx = maxIdx
			}
			result := results[idx]

			for norgIsBlank(lexer.Lookahead()) {
				norgAdvance(s, lexer)
			}

			norgSetResult(s, lexer, result)
			s.resetMods()
			return true
		}

		if !norgIsDetachedMod(lexer.Lookahead()) {
			break
		}
		i++
		s.parsedChars++
	}

	// If only one character parsed, might be an attached modifier
	if s.parsedChars == 1 {
		if modTok, ok := norgAttachedModifiers[s.current]; ok {
			if !s.isModActive(modTok) {
				s.setMod(modTok)
				norgSetResult(s, lexer, modTok)
				return true
			}
		}
	}

	return false
}

func norgCheckAttached(s *norgState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() == ':' {
		isWS := s.current == 0 || unicode.IsSpace(s.current)
		norgAdvance(s, lexer)
		if isWS || unicode.IsSpace(lexer.Lookahead()) {
			return false
		}
		norgSetResult(s, lexer, norgLINK_MODIFIER)
		return true
	}

	canHaveMod := func() bool {
		return !s.isModActive(norgVERBATIM_OPEN) &&
			!s.isModActive(norgINLINE_MATH_OPEN) &&
			!s.isModActive(norgINLINE_MACRO_OPEN)
	}

	if lexer.Lookahead() == '|' {
		norgAdvance(s, lexer)

		_, isAttached := norgAttachedModifiers[lexer.Lookahead()]

		if s.lastToken >= norgBOLD_OPEN && s.lastToken <= norgINLINE_MACRO_CLOSE &&
			(s.lastToken%2) == (norgBOLD_OPEN%2) {
			if s.lastToken != norgVERBATIM_OPEN && s.lastToken != norgINLINE_MACRO_OPEN &&
				s.lastToken != norgINLINE_MATH_OPEN && !canHaveMod() {
				return false
			}
			norgSetResult(s, lexer, norgFREE_FORM_MODIFIER_OPEN)
			return true
		} else if isAttached {
			modTok := norgAttachedModifiers[lexer.Lookahead()]
			if !canHaveMod() &&
				!(modTok == norgVERBATIM_OPEN && s.isModActive(norgVERBATIM_OPEN)) &&
				!(modTok == norgINLINE_MATH_OPEN && s.isModActive(norgINLINE_MATH_OPEN)) &&
				!(modTok == norgINLINE_MACRO_OPEN && s.isModActive(norgINLINE_MACRO_OPEN)) {
				return false
			}
			norgSetResult(s, lexer, norgFREE_FORM_MODIFIER_CLOSE)
			return true
		} else {
			norgSetResult(s, lexer, norgWORD)
			return true
		}
	}

	modTok, isAttached := norgAttachedModifiers[lexer.Lookahead()]
	if !isAttached {
		return false
	}

	// Check for opening modifier
	if unicode.IsSpace(s.current) || (isPunct(s.current) && s.lastToken != norgFREE_FORM_MODIFIER_CLOSE) || s.current == 0 {
		norgAdvance(s, lexer)

		// Empty attached modifier
		if lexer.Lookahead() == s.current {
			for lexer.Lookahead() == s.current {
				norgAdvance(s, lexer)
			}
			return false
		}

		if !unicode.IsSpace(lexer.Lookahead()) && !s.isModActive(modTok) && canHaveMod() {
			s.setMod(modTok)
			norgSetResult(s, lexer, modTok)
			return true
		}
	} else {
		norgAdvance(s, lexer)
	}

	if lexer.Lookahead() == s.current {
		for lexer.Lookahead() == s.current {
			norgAdvance(s, lexer)
		}
		return false
	}

	_, isNextAttached := norgAttachedModifiers[lexer.Lookahead()]
	if isNextAttached {
		s.clearMod(modTok)
		norgSetResult(s, lexer, modTok+1)
		return true
	}

	if (!unicode.IsSpace(s.previous) || s.previous == 0) &&
		(unicode.IsSpace(lexer.Lookahead()) || isPunct(lexer.Lookahead()) || lexer.Lookahead() == 0) {
		s.clearMod(modTok)
		norgSetResult(s, lexer, modTok+1)
		return true
	}

	return false
}

func norgCheckLinkLocation(s *norgState, lexer *gotreesitter.ExternalLexer) bool {
	switch s.lastToken {
	case norgLINK_LOCATION_BEGIN:
		if lexer.Lookahead() == ':' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgLINK_FILE_BEGIN))
			s.lastToken = norgLINK_FILE_BEGIN
			norgAdvance(s, lexer)
			return !unicode.IsSpace(lexer.Lookahead())
		}
		fallthrough
	case norgINTERSECTING_MODIFIER, norgLINK_FILE_END:
		tok := norgNONE
		switch lexer.Lookahead() {
		case '?':
			tok = norgLINK_TARGET_WIKI
		case '#':
			tok = norgLINK_TARGET_GENERIC
		case '/':
			if s.lastToken == norgLINK_FILE_END {
				return false
			}
			tok = norgLINK_TARGET_EXTERNAL_FILE
		case '@':
			if s.lastToken == norgLINK_FILE_END {
				return false
			}
			tok = norgLINK_TARGET_TIMESTAMP
		case '$':
			tok = norgLINK_TARGET_DEFINITION
		case '^':
			tok = norgLINK_TARGET_FOOTNOTE
		case '*':
			norgAdvance(s, lexer)
			count := 0
			for lexer.Lookahead() == '*' {
				count++
				norgAdvance(s, lexer)
			}
			headingTok := norgLINK_TARGET_HEADING1 + count
			if count > 5 {
				headingTok = norgLINK_TARGET_HEADING6
			}
			norgSetResult(s, lexer, headingTok)
			if !unicode.IsSpace(lexer.Lookahead()) {
				return false
			}
			for unicode.IsSpace(lexer.Lookahead()) {
				norgAdvance(s, lexer)
			}
			return true
		default:
			if lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
				tok = norgLINK_TARGET_LINE_NUMBER
			} else {
				tok = norgLINK_TARGET_URL
			}
			norgSetResult(s, lexer, tok)
			return true
		}

		norgAdvance(s, lexer)
		if !unicode.IsSpace(lexer.Lookahead()) {
			return false
		}
		for unicode.IsSpace(lexer.Lookahead()) {
			norgAdvance(s, lexer)
		}
		norgSetResult(s, lexer, tok)
		return true

	case norgLINK_FILE_BEGIN:
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == ':' && s.current != '\\' {
				break
			}
			if lexer.Lookahead() == '`' || lexer.Lookahead() == '%' || lexer.Lookahead() == '&' {
				return false
			}
			if lexer.Lookahead() == '$' && s.current != ':' {
				return false
			}
			norgAdvance(s, lexer)
		}
		norgSetResult(s, lexer, norgLINK_FILE_TEXT)
		return true

	case norgLINK_FILE_TEXT:
		if lexer.Lookahead() == ':' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgLINK_FILE_END))
			s.lastToken = norgLINK_FILE_END
			norgAdvance(s, lexer)
			switch lexer.Lookahead() {
			case '}', '#', '%', '$', '^', '*':
				return true
			default:
				return lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9'
			}
		}
		return false

	default:
		return false
	}
}

func norgCheckDetachedModExtension(s *norgState, lexer *gotreesitter.ExternalLexer) bool {
	switch s.lastToken {
	case norgDETACHED_MODIFIER_EXTENSION_BEGIN, norgMODIFIER_EXTENSION_DELIMITER:
		tok := norgNONE
		switch lexer.Lookahead() {
		case '#':
			tok = norgPRIORITY
		case '@':
			tok = norgTIMESTAMP
		case ' ', '\t', '\v':
			tok = norgTODO_ITEM_UNDONE
		case '-':
			tok = norgTODO_ITEM_PENDING
		case 'x':
			tok = norgTODO_ITEM_DONE
		case '=':
			tok = norgTODO_ITEM_ON_HOLD
		case '_':
			tok = norgTODO_ITEM_CANCELLED
		case '!':
			tok = norgTODO_ITEM_URGENT
		case '?':
			tok = norgTODO_ITEM_UNCERTAIN
		case '+':
			tok = norgTODO_ITEM_RECURRING
		default:
			norgAdvance(s, lexer)
			return false
		}
		norgAdvance(s, lexer)
		for unicode.IsSpace(lexer.Lookahead()) {
			norgAdvance(s, lexer)
		}
		norgSetResult(s, lexer, tok)
		return true

	case norgTIMESTAMP, norgPRIORITY, norgTODO_ITEM_RECURRING:
		switch lexer.Lookahead() {
		case ')':
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgDETACHED_MODIFIER_EXTENSION_END)
			return true
		case '|':
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgMODIFIER_EXTENSION_DELIMITER)
			return true
		}
		for lexer.Lookahead() != 0 && lexer.Lookahead() != '|' && lexer.Lookahead() != ')' {
			norgAdvance(s, lexer)
		}
		if s.lastToken == norgTIMESTAMP || s.lastToken == norgTODO_ITEM_RECURRING {
			norgSetResult(s, lexer, norgTIMESTAMP_DATA)
		} else {
			norgSetResult(s, lexer, norgPRIORITY_DATA)
		}
		return true

	case norgTODO_ITEM_UNDONE, norgTODO_ITEM_PENDING, norgTODO_ITEM_DONE,
		norgTODO_ITEM_ON_HOLD, norgTODO_ITEM_CANCELLED, norgTODO_ITEM_URGENT,
		norgTODO_ITEM_UNCERTAIN, norgTIMESTAMP_DATA, norgPRIORITY_DATA:
		switch lexer.Lookahead() {
		case ')':
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgDETACHED_MODIFIER_EXTENSION_END)
			return true
		case '|':
			if _, ok := norgAttachedModifiers[s.current]; !ok {
				norgAdvance(s, lexer)
				norgSetResult(s, lexer, norgMODIFIER_EXTENSION_DELIMITER)
				return true
			}
		}
		return false

	default:
		if s.lastToken < norgHEADING1 || s.lastToken > norgMULTI_TABLE_CELL_SUFFIX {
			return false
		}
		switch lexer.Lookahead() {
		case '(':
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgDETACHED_MODIFIER_EXTENSION_BEGIN)
			return true
		case ')':
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgDETACHED_MODIFIER_EXTENSION_END)
			return true
		}
	}
	return false
}

func norgParseText(s *norgState, lexer *gotreesitter.ExternalLexer, _ []bool) bool {
	if s.tagContext == norgTagInVerbatimTag {
		for !norgIsNewline(lexer.Lookahead()) {
			norgAdvance(s, lexer)
		}
		norgSetResult(s, lexer, norgWORD)
		return true
	}

	if int(s.tagContext)%2 == 0 && lexer.Lookahead() == '.' {
		norgAdvance(s, lexer)
		norgSetResult(s, lexer, norgTAG_DELIMITER)
		return true
	}

	if norgIsNewline(lexer.Lookahead()) {
		norgSetResult(s, lexer, norgWORD)
		return true
	}

	if norgIsBlank(lexer.Lookahead()) {
		for norgIsBlank(lexer.Lookahead()) {
			norgAdvance(s, lexer)
		}
		if lexer.Lookahead() == ':' {
			norgAdvance(s, lexer)
			if norgIsBlank(lexer.Lookahead()) {
				norgAdvance(s, lexer)
				norgSetResult(s, lexer, norgINTERSECTING_MODIFIER)
				return true
			}
			norgSetResult(s, lexer, norgWORD)
			return true
		}
		norgSetResult(s, lexer, norgSPACE)
		return true
	}

	result := norgWORD
	if unicode.IsUpper(lexer.Lookahead()) {
		result = norgCAPITALIZED_WORD
	}

	for {
		brk := false
		switch lexer.Lookahead() {
		case ':', '|', '~', '\\', '<', '>', '[', ']', '{', '}', '(', ')':
			brk = true
		default:
			if _, ok := norgAttachedModifiers[lexer.Lookahead()]; ok {
				brk = true
			}
			if int(s.tagContext)%2 == 0 && lexer.Lookahead() == '.' {
				brk = true
			}
		}
		if brk || lexer.Lookahead() == 0 || unicode.IsSpace(lexer.Lookahead()) || lexer.Lookahead() == '\\' {
			break
		}
		norgAdvance(s, lexer)
	}

	norgSetResult(s, lexer, result)
	return true
}

func isPunct(ch rune) bool {
	return unicode.IsPunct(ch) || unicode.IsSymbol(ch)
}
