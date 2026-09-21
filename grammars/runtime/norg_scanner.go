//go:build !grammar_subset || grammar_subset_norg

package grammarruntime

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

// norgTokenCount is the number of externals in the norg grammar
// (norgNONE=0 .. norgINDENT_SEGMENT=121 above).
const norgTokenCount = 122

// norgDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped norg.bin assigns to each external, in norg token-index
// order (norgNONE, norgSPACE, ... norgINDENT_SEGMENT). It exists only as a
// pre-bind fallback (and as an independent value to compare a real bind
// against in tests); ExternalScannerForLanguage below overwrites it with
// values read from the actual loaded Language at bind time, which is what
// the scanner must do to survive a future blob regen that renumbers
// absolute symbol IDs without touching the externals list order. The
// upstream grammar happens to assign these 122 externals a contiguous
// absolute Symbol range starting at 3 (see the retired norgSymBase
// formula this replaces), but that contiguity is a property of today's
// shipped blob, not a guarantee -- so the values are enumerated
// explicitly here rather than computed.
var norgDefaultSymTable = [norgTokenCount]gotreesitter.Symbol{
	3,   // _
	4,   // space
	5,   // lowercase_word
	6,   // capitalized_word
	7,   // line_break
	8,   // paragraph_break
	9,   // escape_sequence_prefix
	10,  // trailing_modifier
	11,  // detached_modifier_extension_begin
	12,  // mod_extension_delimiter
	13,  // detached_modifier_extension_end
	14,  // _priority
	15,  // _timestamp
	16,  // todo_item_undone
	17,  // todo_item_pending
	18,  // todo_item_done
	19,  // todo_item_on_hold
	20,  // todo_item_cancelled
	21,  // todo_item_urgent
	22,  // todo_item_uncertain
	23,  // _todo_item_recurring
	24,  // heading1_prefix
	25,  // heading2_prefix
	26,  // heading3_prefix
	27,  // heading4_prefix
	28,  // heading5_prefix
	29,  // heading6_prefix
	30,  // quote1_prefix
	31,  // quote2_prefix
	32,  // quote3_prefix
	33,  // quote4_prefix
	34,  // quote5_prefix
	35,  // quote6_prefix
	36,  // unordered_list1_prefix
	37,  // unordered_list2_prefix
	38,  // unordered_list3_prefix
	39,  // unordered_list4_prefix
	40,  // unordered_list5_prefix
	41,  // unordered_list6_prefix
	42,  // ordered_list1_prefix
	43,  // ordered_list2_prefix
	44,  // ordered_list3_prefix
	45,  // ordered_list4_prefix
	46,  // ordered_list5_prefix
	47,  // ordered_list6_prefix
	48,  // single_definition_prefix
	49,  // multi_definition_prefix
	50,  // multi_definition_suffix
	51,  // single_footnote_prefix
	52,  // multi_footnote_prefix
	53,  // multi_footnote_suffix
	54,  // single_table_cell_prefix
	55,  // multi_table_cell_prefix
	56,  // multi_table_cell_suffix
	57,  // strong_paragraph_delimiter
	58,  // weak_paragraph_delimiter
	59,  // horizontal_line
	60,  // link_description_begin
	61,  // link_description_end
	62,  // link_location_begin
	63,  // link_location_end
	64,  // link_file_begin
	65,  // link_file_end
	66,  // link_file_text
	67,  // link_target_url
	68,  // link_target_line_number
	69,  // link_target_wiki
	70,  // link_target_generic
	71,  // link_target_external_file
	72,  // link_target_timestamp
	73,  // link_target_definition
	74,  // link_target_footnote
	75,  // link_target_heading1
	76,  // link_target_heading2
	77,  // link_target_heading3
	78,  // link_target_heading4
	79,  // link_target_heading5
	80,  // link_target_heading6
	81,  // timestamp_data
	82,  // priority_data
	83,  // tag_delimiter
	84,  // macro_tag_prefix
	85,  // macro_tag_end_prefix
	86,  // ranged_tag_prefix
	87,  // ranged_tag_end_prefix
	88,  // ranged_verbatim_tag_prefix
	89,  // ranged_verbatim_tag_end_prefix
	90,  // infirm_tag_prefix
	91,  // weak_carryover_prefix
	92,  // strong_carryover_prefix
	93,  // link_modifier
	94,  // intersecting_modifier
	95,  // attached_mod_extension_begin
	96,  // attached_mod_extension_end
	97,  // bold_open
	98,  // bold_close
	99,  // italic_open
	100, // italic_close
	101, // strikethrough_open
	102, // strikethrough_close
	103, // underline_open
	104, // underline_close
	105, // spoiler_open
	106, // spoiler_close
	107, // superscript_open
	108, // superscript_close
	109, // subscript_open
	110, // subscript_close
	111, // verbatim_open
	112, // verbatim_close
	113, // inline_comment_open
	114, // inline_comment_close
	115, // inline_math_open
	116, // inline_math_close
	117, // inline_macro_open
	118, // inline_macro_close
	119, // free_form_open
	120, // free_form_close
	121, // inline_link_target_open
	122, // inline_link_target_close
	123, // slide_begin
	124, // indent_segment_begin
}

// norgExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (norgNONE, norgSPACE, ... norgINDENT_SEGMENT order).
var norgExternalScannerSpec = ExternalScannerSpec{
	Language:       "norg",
	UpstreamRepo:   "https://github.com/nvim-neorg/tree-sitter-norg",
	UpstreamCommit: "d89d95af13d409f30a6c7676387bde311ec4a2c8",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "36511b1881b682edbf3dcc3e76ed16e545d81232953988aeb281ef2747358958"},
		{Path: "src/scanner.cc", SHA256: "c833239e7ec452b8379717dcd5f12bae3352091b08e5b666088f139edaf1e502"},
	},
	Externals: []string{
		"_",
		"space",
		"lowercase_word",
		"capitalized_word",
		"line_break",
		"paragraph_break",
		"escape_sequence_prefix",
		"trailing_modifier",
		"detached_modifier_extension_begin",
		"mod_extension_delimiter",
		"detached_modifier_extension_end",
		"_priority",
		"_timestamp",
		"todo_item_undone",
		"todo_item_pending",
		"todo_item_done",
		"todo_item_on_hold",
		"todo_item_cancelled",
		"todo_item_urgent",
		"todo_item_uncertain",
		"_todo_item_recurring",
		"heading1_prefix",
		"heading2_prefix",
		"heading3_prefix",
		"heading4_prefix",
		"heading5_prefix",
		"heading6_prefix",
		"quote1_prefix",
		"quote2_prefix",
		"quote3_prefix",
		"quote4_prefix",
		"quote5_prefix",
		"quote6_prefix",
		"unordered_list1_prefix",
		"unordered_list2_prefix",
		"unordered_list3_prefix",
		"unordered_list4_prefix",
		"unordered_list5_prefix",
		"unordered_list6_prefix",
		"ordered_list1_prefix",
		"ordered_list2_prefix",
		"ordered_list3_prefix",
		"ordered_list4_prefix",
		"ordered_list5_prefix",
		"ordered_list6_prefix",
		"single_definition_prefix",
		"multi_definition_prefix",
		"multi_definition_suffix",
		"single_footnote_prefix",
		"multi_footnote_prefix",
		"multi_footnote_suffix",
		"single_table_cell_prefix",
		"multi_table_cell_prefix",
		"multi_table_cell_suffix",
		"strong_paragraph_delimiter",
		"weak_paragraph_delimiter",
		"horizontal_line",
		"link_description_begin",
		"link_description_end",
		"link_location_begin",
		"link_location_end",
		"link_file_begin",
		"link_file_end",
		"link_file_text",
		"link_target_url",
		"link_target_line_number",
		"link_target_wiki",
		"link_target_generic",
		"link_target_external_file",
		"link_target_timestamp",
		"link_target_definition",
		"link_target_footnote",
		"link_target_heading1",
		"link_target_heading2",
		"link_target_heading3",
		"link_target_heading4",
		"link_target_heading5",
		"link_target_heading6",
		"timestamp_data",
		"priority_data",
		"tag_delimiter",
		"macro_tag_prefix",
		"macro_tag_end_prefix",
		"ranged_tag_prefix",
		"ranged_tag_end_prefix",
		"ranged_verbatim_tag_prefix",
		"ranged_verbatim_tag_end_prefix",
		"infirm_tag_prefix",
		"weak_carryover_prefix",
		"strong_carryover_prefix",
		"link_modifier",
		"intersecting_modifier",
		"attached_mod_extension_begin",
		"attached_mod_extension_end",
		"bold_open",
		"bold_close",
		"italic_open",
		"italic_close",
		"strikethrough_open",
		"strikethrough_close",
		"underline_open",
		"underline_close",
		"spoiler_open",
		"spoiler_close",
		"superscript_open",
		"superscript_close",
		"subscript_open",
		"subscript_close",
		"verbatim_open",
		"verbatim_close",
		"inline_comment_open",
		"inline_comment_close",
		"inline_math_open",
		"inline_math_close",
		"inline_macro_open",
		"inline_macro_close",
		"free_form_open",
		"free_form_close",
		"inline_link_target_open",
		"inline_link_target_close",
		"slide_begin",
		"indent_segment_begin",
	},
}

func init() {
	RegisterExternalScannerSpec(norgExternalScannerSpec)
}

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

// norgSym looks up the concrete gotreesitter.Symbol bound to token index
// tok. It used to compute norgSymBase + tok, relying on the shipped blob
// assigning norg's 122 externals a contiguous absolute range; positional
// binding at load time replaces that assumption with an explicit
// per-instance table read from the loaded Language (see
// NorgExternalScanner.ExternalScannerForLanguage).
func norgSym(tok int, syms *[norgTokenCount]gotreesitter.Symbol) gotreesitter.Symbol {
	return syms[tok]
}

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
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type NorgExternalScanner struct {
	symbols         [norgTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers norg's external symbols.
func (NorgExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := NorgExternalScanner{symbols: norgDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, norgExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s NorgExternalScanner) symbolTable() *[norgTokenCount]gotreesitter.Symbol {
	if s.symbols == ([norgTokenCount]gotreesitter.Symbol{}) {
		return &norgDefaultSymTable
	}
	return &s.symbols
}

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

func (sc NorgExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [norgTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < norgTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	s := payload.(*norgState)
	return norgScan(s, lexer, validSymbols, sc.symbolTable())
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

func norgSetResult(s *norgState, lexer *gotreesitter.ExternalLexer, tok int, syms *[norgTokenCount]gotreesitter.Symbol) {
	lexer.MarkEnd()
	lexer.SetResultSymbol(norgSym(tok, syms))
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

func norgScan(s *norgState, lexer *gotreesitter.ExternalLexer, valid []bool, syms *[norgTokenCount]gotreesitter.Symbol) bool {
	// EOF check
	if lexer.Lookahead() == 0 {
		s.resetMods()
		return false
	}

	if s.lastToken == norgTRAILING_MODIFIER {
		norgAdvance(s, lexer)
		return norgParseText(s, lexer, valid, syms)
	}

	if norgIsNewline(lexer.Lookahead()) {
		norgAdvance(s, lexer)
		norgSetResult(s, lexer, norgLINE_BREAK, syms)

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
			norgSetResult(s, lexer, norgPARAGRAPH_BREAK, syms)
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
					norgSetResult(s, lexer, norgRANGED_VERBATIM_TAG_END, syms)
					s.tagContext = norgTagNone
					return true
				}
				norgSetResult(s, lexer, norgWORD, syms)
				return true
			}

			if s.lastToken == norgRANGED_VERBATIM_TAG || s.tagContext == norgTagInVerbatimTag {
				norgSetResult(s, lexer, norgWORD, syms)
				return true
			}

			norgSetResult(s, lexer, norgRANGED_VERBATIM_TAG, syms)
			s.tagContext = norgTagOnVerbatimTag
			return true
		}

		if s.tagContext == norgTagInVerbatimTag {
			return norgParseText(s, lexer, valid, syms)
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
					norgSetResult(s, lexer, norgMACRO_TAG_END, syms)
					s.tagLevel--
					return true
				}
				norgSetResult(s, lexer, norgWORD, syms)
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
						norgSetResult(s, lexer, norgSTRONG_PARAGRAPH_DELIMITER, syms)
						return true
					}
					lexer.MarkEnd()
					norgAdvance(s, lexer)
					norgSetResult(s, lexer, norgWORD, syms)
					return true
				}
				lexer.MarkEnd()
				norgSetResult(s, lexer, norgWORD, syms)
				return true
			}

			if s.lastToken == norgMACRO_TAG {
				norgSetResult(s, lexer, norgWORD, syms)
				return true
			}

			norgSetResult(s, lexer, norgMACRO_TAG, syms)
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
					norgSetResult(s, lexer, norgRANGED_TAG_END, syms)
					s.tagLevel--
					return true
				}
				norgSetResult(s, lexer, norgWORD, syms)
				return true
			}

			if s.lastToken == norgRANGED_TAG {
				norgSetResult(s, lexer, norgWORD, syms)
				return true
			}

			norgSetResult(s, lexer, norgRANGED_TAG, syms)
			s.tagContext = norgTagOnTag
			s.tagLevel++
			return true
		}

		// Strong carryover: #something
		if lexer.Lookahead() == '#' && s.tagContext != norgTagInVerbatimTag {
			norgAdvance(s, lexer)
			if lexer.Lookahead() == 0 || unicode.IsSpace(lexer.Lookahead()) {
				if norgIsNewline(lexer.Lookahead()) {
					norgSetResult(s, lexer, norgINDENT_SEGMENT, syms)
				} else {
					norgSetResult(s, lexer, norgWORD, syms)
				}
				return true
			}
			norgSetResult(s, lexer, norgSTRONG_CARRYOVER, syms)
			return true
		}

		// Weak carryover: +something
		if lexer.Lookahead() == '+' && s.tagContext != norgTagInVerbatimTag {
			norgAdvance(s, lexer)
			if lexer.Lookahead() != '+' {
				norgSetResult(s, lexer, norgWEAK_CARRYOVER, syms)
				return true
			}
		}

		// Infirm tag: .something
		if lexer.Lookahead() == '.' && s.tagContext != norgTagInVerbatimTag {
			norgAdvance(s, lexer)
			if lexer.Lookahead() != '.' {
				norgSetResult(s, lexer, norgINFIRM_TAG, syms)
				return true
			}
		}

		// Detached modifier checks
		if norgCheckDetached(s, lexer, []int{norgHEADING1, norgHEADING2, norgHEADING3, norgHEADING4, norgHEADING5, norgHEADING6}, '*', syms) {
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgQUOTE1, norgQUOTE2, norgQUOTE3, norgQUOTE4, norgQUOTE5, norgQUOTE6}, '>', syms) {
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgUNORDERED_LIST1, norgUNORDERED_LIST2, norgUNORDERED_LIST3, norgUNORDERED_LIST4, norgUNORDERED_LIST5, norgUNORDERED_LIST6}, '-', syms) {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars >= 3 {
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgWEAK_PARAGRAPH_DELIMITER, syms)
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgORDERED_LIST1, norgORDERED_LIST2, norgORDERED_LIST3, norgORDERED_LIST4, norgORDERED_LIST5, norgORDERED_LIST6}, '~', syms) {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars == 1 {
			if lexer.Lookahead() == 0 {
				s.resetMods()
				return false
			}
			norgSetResult(s, lexer, norgTRAILING_MODIFIER, syms)
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgSINGLE_DEFINITION, norgMULTI_DEFINITION, norgNONE}, '$', syms) {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars == 2 {
			norgAdvance(s, lexer)
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgMULTI_DEFINITION_SUFFIX, syms))
			s.lastToken = norgMULTI_DEFINITION_SUFFIX
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgSINGLE_FOOTNOTE, norgMULTI_FOOTNOTE, norgNONE}, '^', syms) {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars == 2 {
			norgAdvance(s, lexer)
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgMULTI_FOOTNOTE_SUFFIX, syms))
			s.lastToken = norgMULTI_FOOTNOTE_SUFFIX
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgSINGLE_TABLE_CELL, norgMULTI_TABLE_CELL, norgNONE}, ':', syms) {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars == 2 {
			norgAdvance(s, lexer)
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgMULTI_TABLE_CELL_SUFFIX, syms))
			s.lastToken = norgMULTI_TABLE_CELL_SUFFIX
			return true
		}

		if norgCheckDetached(s, lexer, []int{norgNONE, norgNONE}, '_', syms) {
			return true
		} else if norgIsNewline(lexer.Lookahead()) && s.parsedChars >= 3 {
			norgSetResult(s, lexer, norgHORIZONTAL_LINE, syms)
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
			norgSetResult(s, lexer, norgTRAILING_MODIFIER, syms)
			return true
		}
		return norgParseText(s, lexer, valid, syms)
	case '\\':
		norgAdvance(s, lexer)
		norgSetResult(s, lexer, norgESCAPE_SEQUENCE, syms)
		return true
	}

	if norgCheckDetachedModExtension(s, lexer, syms) {
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
				norgSetResult(s, lexer, norgWORD, syms)
				return true
			}
			norgAdvance(s, lexer)
			if isIndent {
				norgSetResult(s, lexer, norgINDENT_SEGMENT, syms)
			} else {
				norgSetResult(s, lexer, norgSLIDE, syms)
			}
			return true
		}
	}

	switch lexer.Lookahead() {
	case '<':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(lexer.Lookahead()) {
			norgSetResult(s, lexer, norgINLINE_LINK_TARGET_OPEN, syms)
			s.inLinkLocation = true
			return true
		}
	case '>':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(s.previous) && s.lastToken != norgLINK_LOCATION_BEGIN &&
			s.lastToken != norgLINK_FILE_END {
			norgSetResult(s, lexer, norgINLINE_LINK_TARGET_CLOSE, syms)
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
			norgSetResult(s, lexer, norgATTACHED_MODIFIER_BEGIN, syms)
			return true
		}
		norgSetResult(s, lexer, norgWORD, syms)
		return true
	case ')':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(s.previous) {
			norgSetResult(s, lexer, norgATTACHED_MODIFIER_END, syms)
			return true
		}
	case '[':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(lexer.Lookahead()) {
			norgSetResult(s, lexer, norgLINK_DESCRIPTION_BEGIN, syms)
			return true
		}
	case ']':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(s.previous) {
			norgSetResult(s, lexer, norgLINK_DESCRIPTION_END, syms)
			return true
		}
	case '{':
		norgAdvance(s, lexer)
		if !unicode.IsSpace(lexer.Lookahead()) {
			norgSetResult(s, lexer, norgLINK_LOCATION_BEGIN, syms)
			s.inLinkLocation = true
			return true
		}
	case '}':
		norgAdvance(s, lexer)
		if norgIsNewline(s.previous) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgNONE, syms))
			s.lastToken = norgNONE
			return true
		}
		if !unicode.IsSpace(s.previous) {
			norgSetResult(s, lexer, norgLINK_LOCATION_END, syms)
			s.inLinkLocation = false
			return true
		}
	}

	if s.inLinkLocation {
		if norgCheckLinkLocation(s, lexer, syms) {
			return true
		}
	}

	if norgCheckAttached(s, lexer, syms) {
		return true
	}

	return norgParseText(s, lexer, valid, syms)
}

func norgCheckDetached(s *norgState, lexer *gotreesitter.ExternalLexer, results []int, expected rune, syms *[norgTokenCount]gotreesitter.Symbol) bool {
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

			norgSetResult(s, lexer, result, syms)
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
				norgSetResult(s, lexer, modTok, syms)
				return true
			}
		}
	}

	return false
}

func norgCheckAttached(s *norgState, lexer *gotreesitter.ExternalLexer, syms *[norgTokenCount]gotreesitter.Symbol) bool {
	if lexer.Lookahead() == ':' {
		isWS := s.current == 0 || unicode.IsSpace(s.current)
		norgAdvance(s, lexer)
		if isWS || unicode.IsSpace(lexer.Lookahead()) {
			return false
		}
		norgSetResult(s, lexer, norgLINK_MODIFIER, syms)
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
			norgSetResult(s, lexer, norgFREE_FORM_MODIFIER_OPEN, syms)
			return true
		} else if isAttached {
			modTok := norgAttachedModifiers[lexer.Lookahead()]
			if !canHaveMod() &&
				!(modTok == norgVERBATIM_OPEN && s.isModActive(norgVERBATIM_OPEN)) &&
				!(modTok == norgINLINE_MATH_OPEN && s.isModActive(norgINLINE_MATH_OPEN)) &&
				!(modTok == norgINLINE_MACRO_OPEN && s.isModActive(norgINLINE_MACRO_OPEN)) {
				return false
			}
			norgSetResult(s, lexer, norgFREE_FORM_MODIFIER_CLOSE, syms)
			return true
		} else {
			norgSetResult(s, lexer, norgWORD, syms)
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
			norgSetResult(s, lexer, modTok, syms)
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
		norgSetResult(s, lexer, modTok+1, syms)
		return true
	}

	if (!unicode.IsSpace(s.previous) || s.previous == 0) &&
		(unicode.IsSpace(lexer.Lookahead()) || isPunct(lexer.Lookahead()) || lexer.Lookahead() == 0) {
		s.clearMod(modTok)
		norgSetResult(s, lexer, modTok+1, syms)
		return true
	}

	return false
}

func norgCheckLinkLocation(s *norgState, lexer *gotreesitter.ExternalLexer, syms *[norgTokenCount]gotreesitter.Symbol) bool {
	switch s.lastToken {
	case norgLINK_LOCATION_BEGIN:
		if lexer.Lookahead() == ':' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgLINK_FILE_BEGIN, syms))
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
			norgSetResult(s, lexer, headingTok, syms)
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
			norgSetResult(s, lexer, tok, syms)
			return true
		}

		norgAdvance(s, lexer)
		if !unicode.IsSpace(lexer.Lookahead()) {
			return false
		}
		for unicode.IsSpace(lexer.Lookahead()) {
			norgAdvance(s, lexer)
		}
		norgSetResult(s, lexer, tok, syms)
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
		norgSetResult(s, lexer, norgLINK_FILE_TEXT, syms)
		return true

	case norgLINK_FILE_TEXT:
		if lexer.Lookahead() == ':' {
			lexer.MarkEnd()
			lexer.SetResultSymbol(norgSym(norgLINK_FILE_END, syms))
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

func norgCheckDetachedModExtension(s *norgState, lexer *gotreesitter.ExternalLexer, syms *[norgTokenCount]gotreesitter.Symbol) bool {
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
		norgSetResult(s, lexer, tok, syms)
		return true

	case norgTIMESTAMP, norgPRIORITY, norgTODO_ITEM_RECURRING:
		switch lexer.Lookahead() {
		case ')':
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgDETACHED_MODIFIER_EXTENSION_END, syms)
			return true
		case '|':
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgMODIFIER_EXTENSION_DELIMITER, syms)
			return true
		}
		for lexer.Lookahead() != 0 && lexer.Lookahead() != '|' && lexer.Lookahead() != ')' {
			norgAdvance(s, lexer)
		}
		if s.lastToken == norgTIMESTAMP || s.lastToken == norgTODO_ITEM_RECURRING {
			norgSetResult(s, lexer, norgTIMESTAMP_DATA, syms)
		} else {
			norgSetResult(s, lexer, norgPRIORITY_DATA, syms)
		}
		return true

	case norgTODO_ITEM_UNDONE, norgTODO_ITEM_PENDING, norgTODO_ITEM_DONE,
		norgTODO_ITEM_ON_HOLD, norgTODO_ITEM_CANCELLED, norgTODO_ITEM_URGENT,
		norgTODO_ITEM_UNCERTAIN, norgTIMESTAMP_DATA, norgPRIORITY_DATA:
		switch lexer.Lookahead() {
		case ')':
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgDETACHED_MODIFIER_EXTENSION_END, syms)
			return true
		case '|':
			if _, ok := norgAttachedModifiers[s.current]; !ok {
				norgAdvance(s, lexer)
				norgSetResult(s, lexer, norgMODIFIER_EXTENSION_DELIMITER, syms)
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
			norgSetResult(s, lexer, norgDETACHED_MODIFIER_EXTENSION_BEGIN, syms)
			return true
		case ')':
			norgAdvance(s, lexer)
			norgSetResult(s, lexer, norgDETACHED_MODIFIER_EXTENSION_END, syms)
			return true
		}
	}
	return false
}

func norgParseText(s *norgState, lexer *gotreesitter.ExternalLexer, _ []bool, syms *[norgTokenCount]gotreesitter.Symbol) bool {
	if s.tagContext == norgTagInVerbatimTag {
		for !norgIsNewline(lexer.Lookahead()) {
			norgAdvance(s, lexer)
		}
		norgSetResult(s, lexer, norgWORD, syms)
		return true
	}

	if int(s.tagContext)%2 == 0 && lexer.Lookahead() == '.' {
		norgAdvance(s, lexer)
		norgSetResult(s, lexer, norgTAG_DELIMITER, syms)
		return true
	}

	if norgIsNewline(lexer.Lookahead()) {
		norgSetResult(s, lexer, norgWORD, syms)
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
				norgSetResult(s, lexer, norgINTERSECTING_MODIFIER, syms)
				return true
			}
			norgSetResult(s, lexer, norgWORD, syms)
			return true
		}
		norgSetResult(s, lexer, norgSPACE, syms)
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

	norgSetResult(s, lexer, result, syms)
	return true
}

func isPunct(ch rune) bool {
	return unicode.IsPunct(ch) || unicode.IsSymbol(ch)
}
