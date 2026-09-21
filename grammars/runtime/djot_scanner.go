//go:build !grammar_subset || grammar_subset_djot

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Djot grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see djotDefaultSymTable below.
// These must match the order from the externals array in grammar.js.
const (
	djotTokIgnored = iota
	djotTokBlockClose
	djotTokEOFOrNewline
	djotTokNewline
	djotTokNewlineInline
	djotTokNonWhitespaceCheck
	djotTokHardLineBreak
	djotTokFrontmatterMarker
	djotTokHeadingBegin
	djotTokHeadingContinuation
	djotTokDivBegin
	djotTokDivEnd
	djotTokCodeBlockBegin
	djotTokCodeBlockEnd
	djotTokListMarkerDash
	djotTokListMarkerStar
	djotTokListMarkerPlus
	djotTokListMarkerTaskBegin
	djotTokListMarkerDefinition
	djotTokListMarkerDecimalPeriod
	djotTokListMarkerLowerAlphaPeriod
	djotTokListMarkerUpperAlphaPeriod
	djotTokListMarkerLowerRomanPeriod
	djotTokListMarkerUpperRomanPeriod
	djotTokListMarkerDecimalParen
	djotTokListMarkerLowerAlphaParen
	djotTokListMarkerUpperAlphaParen
	djotTokListMarkerLowerRomanParen
	djotTokListMarkerUpperRomanParen
	djotTokListMarkerDecimalParens
	djotTokListMarkerLowerAlphaParens
	djotTokListMarkerUpperAlphaParens
	djotTokListMarkerLowerRomanParens
	djotTokListMarkerUpperRomanParens
	djotTokListItemContinuation
	djotTokListItemEnd
	djotTokIndentedContentSpacer
	djotTokCloseParagraph
	djotTokBlockQuoteBegin
	djotTokBlockQuoteContinuation
	djotTokThematicBreakDash
	djotTokThematicBreakStar
	djotTokFootnoteMarkBegin
	djotTokFootnoteContinuation
	djotTokFootnoteEnd
	djotTokLinkRefDefMarkBegin
	djotTokLinkRefDefLabelEnd
	djotTokTableHeaderBegin
	djotTokTableSeparatorBegin
	djotTokTableRowBegin
	djotTokTableRowEndNewline
	djotTokTableCellEnd
	djotTokTableCaptionBegin
	djotTokTableCaptionEnd
	djotTokBlockAttributeBegin
	djotTokCommentEndMarker
	djotTokCommentClose
	djotTokInlineCommentBegin
	djotTokVerbatimBegin
	djotTokVerbatimEnd
	djotTokVerbatimContent
	djotTokEmphasisMarkBegin
	djotTokEmphasisEnd
	djotTokStrongMarkBegin
	djotTokStrongEnd
	djotTokSuperscriptMarkBegin
	djotTokSuperscriptEnd
	djotTokSubscriptMarkBegin
	djotTokSubscriptEnd
	djotTokHighlightedMarkBegin
	djotTokHighlightedEnd
	djotTokInsertMarkBegin
	djotTokInsertEnd
	djotTokDeleteMarkBegin
	djotTokDeleteEnd
	djotTokParensSpanMarkBegin
	djotTokParensSpanEnd
	djotTokCurlyBracketSpanMarkBegin
	djotTokCurlyBracketSpanEnd
	djotTokSquareBracketSpanMarkBegin
	djotTokSquareBracketSpanEnd
	djotTokInFallback
	djotTokError
	djotTokenCount
)

// djotDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped djot.bin assigns to each external, in djotTok* order
// (all 83 externals sit in one contiguous run, 63-145). It exists only as
// a pre-bind fallback (and as an independent value to compare a real bind
// against in tests); ExternalScannerForLanguage below overwrites it with
// values read from the actual loaded Language at bind time, which is what
// the scanner must do to survive a future blob regen that renumbers
// absolute symbol IDs without touching the externals list order.
var djotDefaultSymTable = [djotTokenCount]gotreesitter.Symbol{
	63,  // _ignored
	64,  // _block_close
	65,  // _eof_or_newline
	66,  // _newline
	67,  // _newline_inline
	68,  // _non_whitespace_check
	69,  // hard_line_break
	70,  // frontmatter_marker
	71,  // _heading_begin
	72,  // _heading_continuation
	73,  // _div_begin
	74,  // _div_end
	75,  // _code_block_begin
	76,  // _code_block_end
	77,  // list_marker_dash
	78,  // list_marker_star
	79,  // list_marker_plus
	80,  // _list_marker_task_begin
	81,  // list_marker_definition
	82,  // list_marker_decimal_period
	83,  // list_marker_lower_alpha_period
	84,  // list_marker_upper_alpha_period
	85,  // list_marker_lower_roman_period
	86,  // list_marker_upper_roman_period
	87,  // list_marker_decimal_paren
	88,  // list_marker_lower_alpha_paren
	89,  // list_marker_upper_alpha_paren
	90,  // list_marker_lower_roman_paren
	91,  // list_marker_upper_roman_paren
	92,  // list_marker_decimal_parens
	93,  // list_marker_lower_alpha_parens
	94,  // list_marker_upper_alpha_parens
	95,  // list_marker_lower_roman_parens
	96,  // list_marker_upper_roman_parens
	97,  // _list_item_continuation
	98,  // _list_item_end
	99,  // _indented_content_spacer
	100, // _close_paragraph
	101, // _block_quote_begin
	102, // _block_quote_continuation
	103, // _thematic_break_dash
	104, // _thematic_break_star
	105, // _footnote_mark_begin
	106, // _footnote_continuation
	107, // _footnote_end
	108, // _link_ref_def_mark_begin
	109, // _link_ref_def_label_end
	110, // _table_header_begin
	111, // _table_separator_begin
	112, // _table_row_begin
	113, // _table_row_end_newline
	114, // _table_cell_end
	115, // _table_caption_begin
	116, // _table_caption_end
	117, // _block_attribute_begin
	118, // _comment_end_marker
	119, // _comment_close
	120, // _inline_comment_begin
	121, // _verbatim_begin
	122, // _verbatim_end
	123, // _verbatim_content
	124, // _emphasis_mark_begin
	125, // emphasis_end
	126, // _strong_mark_begin
	127, // strong_end
	128, // _superscript_mark_begin
	129, // superscript_end
	130, // _subscript_mark_begin
	131, // subscript_end
	132, // _highlighted_mark_begin
	133, // highlighted_end
	134, // _insert_mark_begin
	135, // insert_end
	136, // _delete_mark_begin
	137, // delete_end
	138, // _parens_span_mark_begin
	139, // _parens_span_end
	140, // _curly_bracket_span_mark_begin
	141, // _curly_bracket_span_end
	142, // _square_bracket_span_mark_begin
	143, // _square_bracket_span_end
	144, // _in_fallback
	145, // _error
}

// djotExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (djotTok* order).
var djotExternalScannerSpec = ExternalScannerSpec{
	Language:       "djot",
	UpstreamRepo:   "https://github.com/treeman/tree-sitter-djot",
	UpstreamCommit: "74fac1f53c6d52aeac104b6874e5506be6d0cfe6",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "14fa62b9feb06eae88b471b737fbb244b0e80d5e63ae175227e36a6ef4930a0c"},
		{Path: "src/scanner.c", SHA256: "015f12e1ae82089365a916297ac6befaffb72ab61b08ae362ede40ac1b1af974"},
	},
	Externals: []string{
		"_ignored",
		"_block_close",
		"_eof_or_newline",
		"_newline",
		"_newline_inline",
		"_non_whitespace_check",
		"hard_line_break",
		"frontmatter_marker",
		"_heading_begin",
		"_heading_continuation",
		"_div_begin",
		"_div_end",
		"_code_block_begin",
		"_code_block_end",
		"list_marker_dash",
		"list_marker_star",
		"list_marker_plus",
		"_list_marker_task_begin",
		"list_marker_definition",
		"list_marker_decimal_period",
		"list_marker_lower_alpha_period",
		"list_marker_upper_alpha_period",
		"list_marker_lower_roman_period",
		"list_marker_upper_roman_period",
		"list_marker_decimal_paren",
		"list_marker_lower_alpha_paren",
		"list_marker_upper_alpha_paren",
		"list_marker_lower_roman_paren",
		"list_marker_upper_roman_paren",
		"list_marker_decimal_parens",
		"list_marker_lower_alpha_parens",
		"list_marker_upper_alpha_parens",
		"list_marker_lower_roman_parens",
		"list_marker_upper_roman_parens",
		"_list_item_continuation",
		"_list_item_end",
		"_indented_content_spacer",
		"_close_paragraph",
		"_block_quote_begin",
		"_block_quote_continuation",
		"_thematic_break_dash",
		"_thematic_break_star",
		"_footnote_mark_begin",
		"_footnote_continuation",
		"_footnote_end",
		"_link_ref_def_mark_begin",
		"_link_ref_def_label_end",
		"_table_header_begin",
		"_table_separator_begin",
		"_table_row_begin",
		"_table_row_end_newline",
		"_table_cell_end",
		"_table_caption_begin",
		"_table_caption_end",
		"_block_attribute_begin",
		"_comment_end_marker",
		"_comment_close",
		"_inline_comment_begin",
		"_verbatim_begin",
		"_verbatim_end",
		"_verbatim_content",
		"_emphasis_mark_begin",
		"emphasis_end",
		"_strong_mark_begin",
		"strong_end",
		"_superscript_mark_begin",
		"superscript_end",
		"_subscript_mark_begin",
		"subscript_end",
		"_highlighted_mark_begin",
		"highlighted_end",
		"_insert_mark_begin",
		"insert_end",
		"_delete_mark_begin",
		"delete_end",
		"_parens_span_mark_begin",
		"_parens_span_end",
		"_curly_bracket_span_mark_begin",
		"_curly_bracket_span_end",
		"_square_bracket_span_mark_begin",
		"_square_bracket_span_end",
		"_in_fallback",
		"_error",
	},
}

func init() {
	RegisterExternalScannerSpec(djotExternalScannerSpec)
}

// Block types tracked for matching/closing.
type djotBlockType uint8

const (
	djotBlockQuote djotBlockType = iota
	djotCodeBlock
	djotDiv
	djotSection
	djotHeading
	djotFootnote
	djotLinkRefDef
	djotTableRow
	djotTableCaption
	djotListDash
	djotListStar
	djotListPlus
	djotListTask
	djotListDefinition
	djotListDecimalPeriod
	djotListLowerAlphaPeriod
	djotListUpperAlphaPeriod
	djotListLowerRomanPeriod
	djotListUpperRomanPeriod
	djotListDecimalParen
	djotListLowerAlphaParen
	djotListUpperAlphaParen
	djotListLowerRomanParen
	djotListUpperRomanParen
	djotListDecimalParens
	djotListLowerAlphaParens
	djotListUpperAlphaParens
	djotListLowerRomanParens
	djotListUpperRomanParens
)

// Ordered list type classification.
type djotOrderedListType uint8

const (
	djotDecimal djotOrderedListType = iota
	djotLowerAlpha
	djotUpperAlpha
	djotLowerRoman
	djotUpperRoman
)

// Inline element types.
type djotInlineType uint8

const (
	djotInlineVerbatim djotInlineType = iota
	djotInlineEmphasis
	djotInlineStrong
	djotInlineSuperscript
	djotInlineSubscript
	djotInlineHighlighted
	djotInlineInsert
	djotInlineDelete
	djotInlineParensSpan
	djotInlineCurlyBracketSpan
	djotInlineSquareBracketSpan
)

// Span delimiter type.
type djotSpanType uint8

const (
	djotSpanSingle djotSpanType = iota
	djotSpanBracketed
	djotSpanBracketedAndSingle
	djotSpanBracketedAndSingleNoWhitespace
)

type djotBlock struct {
	blockType djotBlockType
	data      uint8
}

type djotInline struct {
	inlineType djotInlineType
	data       uint8
}

type djotScannerState struct {
	openBlocks      []djotBlock
	openInline      []djotInline
	blocksToClose   uint8
	blockQuoteLevel uint8
	indent          uint8
	state           uint8

	// symTable holds the concrete gotreesitter.Symbol each external index
	// maps to in the Language this scanner instance was bound to (see
	// ExternalScannerForLanguage). It is derived, per-attachment data, not
	// persistent parse state, so Serialize/Deserialize below never touch it.
	symTable *[djotTokenCount]gotreesitter.Symbol
}

// State flags.
const (
	djotStateBracketStartsInlineLink uint8 = 1 << 0
	djotStateBracketStartsSpan       uint8 = 1 << 1
	djotStateTableSeparatorNext      uint8 = 1 << 2
)

// DjotExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-djot.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type DjotExternalScanner struct {
	symbols         [djotTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers djot's external symbols.
func (DjotExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := DjotExternalScanner{symbols: djotDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, djotExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s DjotExternalScanner) symbolTable() *[djotTokenCount]gotreesitter.Symbol {
	if s.symbols == ([djotTokenCount]gotreesitter.Symbol{}) {
		return &djotDefaultSymTable
	}
	return &s.symbols
}

func (s DjotExternalScanner) Create() any {
	return &djotScannerState{symTable: s.symbolTable()}
}

func (DjotExternalScanner) Destroy(payload any) {}

func (DjotExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*djotScannerState)
	size := 0

	// 4 scalar fields + 1 byte for open_blocks count + 2 bytes per block + 2 bytes per inline
	needed := 5 + len(s.openBlocks)*2 + len(s.openInline)*2
	if needed > len(buf) {
		return 0
	}

	buf[size] = s.blocksToClose
	size++
	buf[size] = s.blockQuoteLevel
	size++
	buf[size] = s.indent
	size++
	buf[size] = s.state
	size++

	buf[size] = uint8(len(s.openBlocks))
	size++
	for _, b := range s.openBlocks {
		buf[size] = uint8(b.blockType)
		size++
		buf[size] = b.data
		size++
	}

	for _, x := range s.openInline {
		buf[size] = uint8(x.inlineType)
		size++
		buf[size] = x.data
		size++
	}

	return size
}

func (DjotExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*djotScannerState)
	s.openBlocks = s.openBlocks[:0]
	s.openInline = s.openInline[:0]
	s.blocksToClose = 0
	s.blockQuoteLevel = 0
	s.indent = 0
	s.state = 0

	if len(buf) == 0 {
		return
	}

	size := 0
	s.blocksToClose = buf[size]
	size++
	s.blockQuoteLevel = buf[size]
	size++
	s.indent = buf[size]
	size++
	s.state = buf[size]
	size++

	openBlocksCount := int(buf[size])
	size++
	for i := 0; i < openBlocksCount && size+1 < len(buf); i++ {
		bt := djotBlockType(buf[size])
		size++
		data := buf[size]
		size++
		s.openBlocks = append(s.openBlocks, djotBlock{blockType: bt, data: data})
	}
	for size+1 < len(buf) {
		it := djotInlineType(buf[size])
		size++
		data := buf[size]
		size++
		s.openInline = append(s.openInline, djotInline{inlineType: it, data: data})
	}
}

func (sc DjotExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*djotScannerState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [djotTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < djotTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}

	return djotScan(s, lexer, validSymbols)
}

// ---------------------------------------------------------------------------
// Helper: check validSymbols safely.
// ---------------------------------------------------------------------------

func djotValid(vs []bool, tok int) bool {
	return tok >= 0 && tok < len(vs) && vs[tok]
}

// Set the result symbol from a token index.
func djotSetResult(s *djotScannerState, lexer *gotreesitter.ExternalLexer, tok int) {
	lexer.SetResultSymbol(s.symTable[tok])
}

// ---------------------------------------------------------------------------
// Block helpers
// ---------------------------------------------------------------------------

func djotIsList(t djotBlockType) bool {
	switch t {
	case djotListDash, djotListStar, djotListPlus, djotListTask, djotListDefinition,
		djotListDecimalPeriod, djotListLowerAlphaPeriod, djotListUpperAlphaPeriod,
		djotListLowerRomanPeriod, djotListUpperRomanPeriod,
		djotListDecimalParen, djotListLowerAlphaParen, djotListUpperAlphaParen,
		djotListLowerRomanParen, djotListUpperRomanParen,
		djotListDecimalParens, djotListLowerAlphaParens, djotListUpperAlphaParens,
		djotListLowerRomanParens, djotListUpperRomanParens:
		return true
	}
	return false
}

func djotListMarkerToBlock(tok int) djotBlockType {
	switch tok {
	case djotTokListMarkerDash:
		return djotListDash
	case djotTokListMarkerStar:
		return djotListStar
	case djotTokListMarkerPlus:
		return djotListPlus
	case djotTokListMarkerTaskBegin:
		return djotListTask
	case djotTokListMarkerDefinition:
		return djotListDefinition
	case djotTokListMarkerDecimalPeriod:
		return djotListDecimalPeriod
	case djotTokListMarkerLowerAlphaPeriod:
		return djotListLowerAlphaPeriod
	case djotTokListMarkerUpperAlphaPeriod:
		return djotListUpperAlphaPeriod
	case djotTokListMarkerLowerRomanPeriod:
		return djotListLowerRomanPeriod
	case djotTokListMarkerUpperRomanPeriod:
		return djotListUpperRomanPeriod
	case djotTokListMarkerDecimalParen:
		return djotListDecimalParen
	case djotTokListMarkerLowerAlphaParen:
		return djotListLowerAlphaParen
	case djotTokListMarkerUpperAlphaParen:
		return djotListUpperAlphaParen
	case djotTokListMarkerLowerRomanParen:
		return djotListLowerRomanParen
	case djotTokListMarkerUpperRomanParen:
		return djotListUpperRomanParen
	case djotTokListMarkerDecimalParens:
		return djotListDecimalParens
	case djotTokListMarkerLowerAlphaParens:
		return djotListLowerAlphaParens
	case djotTokListMarkerUpperAlphaParens:
		return djotListUpperAlphaParens
	case djotTokListMarkerLowerRomanParens:
		return djotListLowerRomanParens
	case djotTokListMarkerUpperRomanParens:
		return djotListUpperRomanParens
	}
	return djotListDash
}

func djotIsAlphaList(t djotBlockType) bool {
	switch t {
	case djotListLowerAlphaPeriod, djotListLowerAlphaParen, djotListLowerAlphaParens,
		djotListUpperAlphaPeriod, djotListUpperAlphaParen, djotListUpperAlphaParens:
		return true
	}
	return false
}

func djotPushBlock(s *djotScannerState, bt djotBlockType, data uint8) {
	s.openBlocks = append(s.openBlocks, djotBlock{blockType: bt, data: data})
}

func djotPushInline(s *djotScannerState, it djotInlineType, data uint8) {
	s.openInline = append(s.openInline, djotInline{inlineType: it, data: data})
}

func djotRemoveBlock(s *djotScannerState) {
	if len(s.openBlocks) > 0 {
		s.openBlocks = s.openBlocks[:len(s.openBlocks)-1]
		if s.blocksToClose > 0 {
			s.blocksToClose--
		}
	}
}

func djotRemoveInline(s *djotScannerState) {
	if len(s.openInline) > 0 {
		s.openInline = s.openInline[:len(s.openInline)-1]
	}
}

func djotPeekBlock(s *djotScannerState) *djotBlock {
	if len(s.openBlocks) > 0 {
		return &s.openBlocks[len(s.openBlocks)-1]
	}
	return nil
}

func djotPeekInline(s *djotScannerState) *djotInline {
	if len(s.openInline) > 0 {
		return &s.openInline[len(s.openInline)-1]
	}
	return nil
}

func djotDisallowNewline(top *djotBlock) bool {
	if top == nil {
		return false
	}
	switch top.blockType {
	case djotTableRow, djotLinkRefDef:
		return true
	}
	return false
}

// How many blocks from the top of the stack can we find a matching block?
// If directly on top, returns 1. If not found, returns 0.
func djotNumberOfBlocksFromTop(s *djotScannerState, bt djotBlockType, level uint8) int {
	for i := len(s.openBlocks) - 1; i >= 0; i-- {
		b := &s.openBlocks[i]
		if b.blockType == bt && b.data == level {
			return len(s.openBlocks) - i
		}
	}
	return 0
}

func djotFindBlock(s *djotScannerState, bt djotBlockType) *djotBlock {
	for i := len(s.openBlocks) - 1; i >= 0; i-- {
		if s.openBlocks[i].blockType == bt {
			return &s.openBlocks[i]
		}
	}
	return nil
}

func djotFindList(s *djotScannerState) *djotBlock {
	for i := len(s.openBlocks) - 1; i >= 0; i-- {
		if djotIsList(s.openBlocks[i].blockType) {
			return &s.openBlocks[i]
		}
	}
	return nil
}

func djotCountBlocks(s *djotScannerState, bt djotBlockType) uint8 {
	var count uint8
	for i := len(s.openBlocks) - 1; i >= 0; i-- {
		if s.openBlocks[i].blockType == bt {
			count++
		}
	}
	return count
}

func djotCloseBlocks(s *djotScannerState, lexer *gotreesitter.ExternalLexer, count int) {
	if len(s.openBlocks) > 0 {
		djotRemoveBlock(s)
		if count > 1 {
			s.blocksToClose = s.blocksToClose + uint8(count) - 1
		}
	}
	djotSetResult(s, lexer, djotTokBlockClose)
}

// ---------------------------------------------------------------------------
// Lexer helpers
// ---------------------------------------------------------------------------

func djotAdvance(s *djotScannerState, lexer *gotreesitter.ExternalLexer) {
	lexer.Advance(false)
	// Carriage returns should simply be ignored.
	if lexer.Lookahead() == '\r' {
		lexer.Advance(false)
	}
}

func djotConsumeChars(s *djotScannerState, lexer *gotreesitter.ExternalLexer, c rune) uint8 {
	var count uint8
	for lexer.Lookahead() == c {
		djotAdvance(s, lexer)
		count++
	}
	return count
}

func djotConsumeWhitespace(s *djotScannerState, lexer *gotreesitter.ExternalLexer) uint8 {
	var indent uint8
	for {
		ch := lexer.Lookahead()
		if ch == ' ' {
			djotAdvance(s, lexer)
			indent++
		} else if ch == '\r' {
			djotAdvance(s, lexer)
		} else if ch == '\t' {
			djotAdvance(s, lexer)
			indent += 4
		} else {
			break
		}
	}
	return indent
}

// ---------------------------------------------------------------------------
// handle_blocks_to_close
// ---------------------------------------------------------------------------

func djotHandleBlocksToClose(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if len(s.openBlocks) == 0 {
		return false
	}

	// If we reach eof with open blocks, we should close them all.
	if lexer.Lookahead() == 0 || s.blocksToClose > 0 {
		djotSetResult(s, lexer, djotTokBlockClose)
		djotRemoveBlock(s)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Identifier and scanning helpers
// ---------------------------------------------------------------------------

func djotScanIdentifier(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	anyScanned := false
	for lexer.Lookahead() != 0 {
		ch := lexer.Lookahead()
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			anyScanned = true
			djotAdvance(s, lexer)
		} else {
			return anyScanned
		}
	}
	return anyScanned
}

func djotScanUntilUnescaped(s *djotScannerState, lexer *gotreesitter.ExternalLexer, c rune) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == c {
			return true
		} else if lexer.Lookahead() == '\\' {
			djotAdvance(s, lexer)
		}
		djotAdvance(s, lexer)
	}
	return false
}

// ---------------------------------------------------------------------------
// Indented content spacer
// ---------------------------------------------------------------------------

func djotParseIndentedContentSpacer(s *djotScannerState, lexer *gotreesitter.ExternalLexer, isNewline bool) bool {
	if isNewline {
		djotAdvance(s, lexer)
		lexer.MarkEnd()
	}
	djotSetResult(s, lexer, djotTokIndentedContentSpacer)
	return true
}

// ---------------------------------------------------------------------------
// List item continuation
// ---------------------------------------------------------------------------

func djotParseListItemContinuation(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	list := djotFindList(s)
	if list == nil {
		return false
	}
	if s.indent < list.data {
		return false
	}
	lexer.MarkEnd()
	djotSetResult(s, lexer, djotTokListItemContinuation)
	return true
}

// ---------------------------------------------------------------------------
// Close list nested block if needed
// ---------------------------------------------------------------------------

func djotCloseListNestedBlockIfNeeded(s *djotScannerState, lexer *gotreesitter.ExternalLexer, nonNewline bool) bool {
	if len(s.openBlocks) == 0 {
		return false
	}
	if len(s.openInline) > 0 {
		return false
	}

	top := djotPeekBlock(s)
	list := djotFindList(s)

	if nonNewline && list != nil && list != top {
		if s.indent < list.data {
			djotSetResult(s, lexer, djotTokBlockClose)
			djotRemoveBlock(s)
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Close different list if needed
// ---------------------------------------------------------------------------

func djotCloseDifferentListIfNeeded(s *djotScannerState, lexer *gotreesitter.ExternalLexer, list *djotBlock, listMarker int) bool {
	if len(s.openInline) > 0 {
		return false
	}
	if listMarker != djotTokIgnored {
		toOpen := djotListMarkerToBlock(listMarker)
		if list.blockType != toOpen {
			djotSetResult(s, lexer, djotTokBlockClose)
			djotRemoveBlock(s)
			return true
		}
	}
	return false
}

func djotTryCloseDifferentTypedList(s *djotScannerState, lexer *gotreesitter.ExternalLexer, orderedListMarker int) bool {
	if len(s.openBlocks) == 0 {
		return false
	}
	top := djotPeekBlock(s)
	if top.blockType == djotCodeBlock {
		return false
	}
	list := djotFindList(s)
	if list == nil {
		return false
	}

	if djotCloseDifferentListIfNeeded(s, lexer, list, orderedListMarker) {
		return true
	}
	otherListMarker := djotScanUnorderedListMarkerToken(s, lexer)
	return djotCloseDifferentListIfNeeded(s, lexer, list, otherListMarker)
}

// ---------------------------------------------------------------------------
// Div marker scanning
// ---------------------------------------------------------------------------

func djotScanDivMarker(s *djotScannerState, lexer *gotreesitter.ExternalLexer) (colons uint8, fromTop int, ok bool) {
	colons = djotConsumeChars(s, lexer, ':')
	if colons < 3 {
		return colons, 0, false
	}
	fromTop = djotNumberOfBlocksFromTop(s, djotDiv, colons)
	return colons, fromTop, true
}

func djotIsDivMarkerNext(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	_, _, ok := djotScanDivMarker(s, lexer)
	return ok
}

// ---------------------------------------------------------------------------
// Verbatim
// ---------------------------------------------------------------------------

func djotTryImplicitCloseVerbatim(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	top := djotPeekInline(s)
	if top == nil || top.inlineType != djotInlineVerbatim {
		return false
	}
	if top.data > 0 {
		djotRemoveInline(s)
		djotSetResult(s, lexer, djotTokVerbatimEnd)
		return true
	}
	return false
}

func djotParseVerbatimContent(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	top := djotPeekInline(s)
	if top == nil || top.inlineType != djotInlineVerbatim {
		return false
	}

	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '\n' {
			djotAdvance(s, lexer)
			djotConsumeWhitespace(s, lexer)
			if lexer.Lookahead() == 0 || lexer.Lookahead() == '\n' {
				break
			}
			lexer.MarkEnd()
		} else if lexer.Lookahead() == '`' {
			current := djotConsumeChars(s, lexer, '`')
			if current == top.data {
				break
			}
			lexer.MarkEnd()
		} else {
			djotAdvance(s, lexer)
			lexer.MarkEnd()
		}
	}

	djotSetResult(s, lexer, djotTokVerbatimContent)
	return true
}

// ---------------------------------------------------------------------------
// Code block
// ---------------------------------------------------------------------------

func djotTryEndCodeBlock(s *djotScannerState, lexer *gotreesitter.ExternalLexer, ticks uint8) bool {
	top := djotPeekBlock(s)
	if top == nil || top.blockType != djotCodeBlock {
		return false
	}
	if top.data != ticks {
		return false
	}
	djotRemoveBlock(s)
	lexer.MarkEnd()
	djotSetResult(s, lexer, djotTokCodeBlockEnd)
	return true
}

func djotTryCloseCodeBlock(s *djotScannerState, lexer *gotreesitter.ExternalLexer, ticks uint8) bool {
	top := djotPeekBlock(s)
	if top == nil || top.blockType != djotCodeBlock {
		return false
	}
	if top.data != ticks {
		return false
	}
	djotSetResult(s, lexer, djotTokBlockClose)
	return true
}

func djotTryBeginCodeBlock(s *djotScannerState, lexer *gotreesitter.ExternalLexer, ticks uint8) bool {
	top := djotPeekBlock(s)
	if top != nil && top.blockType == djotCodeBlock {
		return false
	}
	djotPushBlock(s, djotCodeBlock, ticks)
	lexer.MarkEnd()
	djotSetResult(s, lexer, djotTokCodeBlockBegin)
	return true
}

func djotParseBacktick(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	if !djotValid(vs, djotTokCodeBlockBegin) && !djotValid(vs, djotTokCodeBlockEnd) &&
		!djotValid(vs, djotTokBlockClose) && !djotValid(vs, djotTokVerbatimBegin) &&
		!djotValid(vs, djotTokVerbatimEnd) {
		return false
	}

	ticks := djotConsumeChars(s, lexer, '`')
	if ticks == 0 {
		return false
	}

	if ticks >= 3 {
		if djotValid(vs, djotTokCodeBlockEnd) && djotTryEndCodeBlock(s, lexer, ticks) {
			return true
		}
		if djotValid(vs, djotTokBlockClose) && djotTryCloseCodeBlock(s, lexer, ticks) {
			return true
		}
		if djotValid(vs, djotTokCodeBlockBegin) && djotTryBeginCodeBlock(s, lexer, ticks) {
			return true
		}
	}

	top := djotPeekInline(s)
	if djotValid(vs, djotTokVerbatimEnd) && top != nil && top.inlineType == djotInlineVerbatim {
		djotRemoveInline(s)
		lexer.MarkEnd()
		djotSetResult(s, lexer, djotTokVerbatimEnd)
		return true
	}
	if djotValid(vs, djotTokVerbatimBegin) {
		lexer.MarkEnd()
		djotSetResult(s, lexer, djotTokVerbatimBegin)
		djotPushInline(s, djotInlineVerbatim, ticks)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Bullet / unordered list marker scanning
// ---------------------------------------------------------------------------

func djotScanBulletListMarker(s *djotScannerState, lexer *gotreesitter.ExternalLexer, marker rune) bool {
	if lexer.Lookahead() != marker {
		return false
	}
	djotAdvance(s, lexer)
	if lexer.Lookahead() != ' ' {
		return false
	}
	djotAdvance(s, lexer)
	return true
}

// ---------------------------------------------------------------------------
// Block quote scanning
// ---------------------------------------------------------------------------

func djotScanBlockQuoteMarker(s *djotScannerState, lexer *gotreesitter.ExternalLexer) (hasMarker bool, endingNewline bool) {
	if lexer.Lookahead() != '>' {
		return false, false
	}
	djotAdvance(s, lexer)
	if lexer.Lookahead() == '\r' {
		djotAdvance(s, lexer)
	}
	if lexer.Lookahead() == ' ' {
		djotAdvance(s, lexer)
		return true, false
	} else if lexer.Lookahead() == '\n' {
		djotAdvance(s, lexer)
		return true, true
	}
	return false, false
}

func djotScanBlockQuoteMarkers(s *djotScannerState, lexer *gotreesitter.ExternalLexer) (markerCount uint8, endingNewline bool) {
	for {
		has, ending := djotScanBlockQuoteMarker(s, lexer)
		if !has {
			break
		}
		markerCount++
		if ending {
			endingNewline = true
			break
		}
	}
	return
}

func djotOutputBlockQuoteContinuation(s *djotScannerState, lexer *gotreesitter.ExternalLexer, markerCount uint8, endingNewline bool) {
	if endingNewline {
		s.blockQuoteLevel = 0
	} else {
		s.blockQuoteLevel = markerCount
	}
	djotSetResult(s, lexer, djotTokBlockQuoteContinuation)
}

func djotParseBlockQuote(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	if !djotValid(vs, djotTokBlockQuoteBegin) &&
		!djotValid(vs, djotTokBlockQuoteContinuation) &&
		!djotValid(vs, djotTokBlockClose) &&
		!djotValid(vs, djotTokCloseParagraph) {
		return false
	}

	hasMarker, endingNewline := djotScanBlockQuoteMarker(s, lexer)

	anyOpenInline := len(s.openInline) > 0

	if hasMarker && endingNewline && !anyOpenInline && djotValid(vs, djotTokCloseParagraph) {
		djotSetResult(s, lexer, djotTokCloseParagraph)
		return true
	}

	var markerCount uint8
	if hasMarker {
		markerCount = s.blockQuoteLevel + 1
	} else {
		markerCount = s.blockQuoteLevel
	}

	matchingBlockPos := djotNumberOfBlocksFromTop(s, djotBlockQuote, markerCount)
	highestBlockQuote := djotFindBlock(s, djotBlockQuote)

	if highestBlockQuote != nil && markerCount < highestBlockQuote.data && !anyOpenInline {
		if djotValid(vs, djotTokCloseParagraph) && hasMarker {
			djotSetResult(s, lexer, djotTokCloseParagraph)
			return true
		}
		if djotValid(vs, djotTokBlockClose) {
			closePos := djotNumberOfBlocksFromTop(s, djotBlockQuote, markerCount+1)
			djotCloseBlocks(s, lexer, closePos)
			return true
		}
	}

	if djotValid(vs, djotTokBlockQuoteContinuation) && hasMarker && matchingBlockPos != 0 {
		lexer.MarkEnd()
		djotOutputBlockQuoteContinuation(s, lexer, markerCount, endingNewline)
		return true
	}

	if djotValid(vs, djotTokBlockQuoteBegin) && hasMarker {
		djotPushBlock(s, djotBlockQuote, markerCount)
		lexer.MarkEnd()
		if endingNewline {
			s.blockQuoteLevel = 0
		} else {
			s.blockQuoteLevel = markerCount
		}
		djotSetResult(s, lexer, djotTokBlockQuoteBegin)
		return true
	}

	return false
}

// ---------------------------------------------------------------------------
// Ordered list type detection
// ---------------------------------------------------------------------------

func djotIsDecimal(c rune) bool    { return c >= '0' && c <= '9' }
func djotIsLowerAlpha(c rune) bool { return c >= 'a' && c <= 'z' }
func djotIsUpperAlpha(c rune) bool { return c >= 'A' && c <= 'Z' }
func djotIsLowerRoman(c rune) bool {
	switch c {
	case 'i', 'v', 'x', 'l', 'c', 'd', 'm':
		return true
	}
	return false
}
func djotIsUpperRoman(c rune) bool {
	switch c {
	case 'I', 'V', 'X', 'L', 'C', 'D', 'M':
		return true
	}
	return false
}

func djotMatchesOrderedList(t djotOrderedListType, c rune) bool {
	switch t {
	case djotDecimal:
		return djotIsDecimal(c)
	case djotLowerAlpha:
		return djotIsLowerAlpha(c)
	case djotUpperAlpha:
		return djotIsUpperAlpha(c)
	case djotLowerRoman:
		return djotIsLowerRoman(c)
	case djotUpperRoman:
		return djotIsUpperRoman(c)
	}
	return false
}

func djotScanOrderedListType(s *djotScannerState, lexer *gotreesitter.ExternalLexer) (djotOrderedListType, bool) {
	canBeDecimal := true
	var scannedDecimal uint8
	canBeLowerRoman := true
	var scannedLowerRoman uint8
	canBeUpperRoman := true
	var scannedUpperRoman uint8
	canBeLowerAlpha := true
	var scannedLowerAlpha uint8
	canBeUpperAlpha := true
	var scannedUpperAlpha uint8

	for lexer.Lookahead() != 0 {
		c := lexer.Lookahead()

		if canBeDecimal {
			if djotMatchesOrderedList(djotDecimal, c) {
				scannedDecimal++
			} else {
				canBeDecimal = false
			}
		}
		if canBeLowerRoman {
			if djotMatchesOrderedList(djotLowerRoman, c) {
				scannedLowerRoman++
			} else {
				canBeLowerRoman = false
			}
		}
		if canBeUpperRoman {
			if djotMatchesOrderedList(djotUpperRoman, c) {
				scannedUpperRoman++
			} else {
				canBeUpperRoman = false
			}
		}
		if canBeLowerAlpha {
			if djotMatchesOrderedList(djotLowerAlpha, c) {
				scannedLowerAlpha++
			} else {
				canBeLowerAlpha = false
			}
		}
		if canBeUpperAlpha {
			if djotMatchesOrderedList(djotUpperAlpha, c) {
				scannedUpperAlpha++
			} else {
				canBeUpperAlpha = false
			}
		}
		if !canBeDecimal && !canBeLowerRoman && !canBeUpperRoman && !canBeLowerAlpha && !canBeUpperAlpha {
			break
		}
		djotAdvance(s, lexer)
	}

	if scannedDecimal > 0 {
		return djotDecimal, true
	}

	top := djotPeekBlock(s)
	insideAlphaList := top != nil && djotIsAlphaList(top.blockType)

	if insideAlphaList {
		if scannedLowerAlpha == 1 {
			return djotLowerAlpha, true
		}
		if scannedUpperAlpha == 1 {
			return djotUpperAlpha, true
		}
	}

	if scannedLowerRoman > 0 {
		return djotLowerRoman, true
	}
	if scannedUpperRoman > 0 {
		return djotUpperRoman, true
	}
	if scannedLowerAlpha == 1 {
		return djotLowerAlpha, true
	}
	if scannedUpperAlpha == 1 {
		return djotUpperAlpha, true
	}
	return 0, false
}

func djotScanOrderedListMarkerTokenType(s *djotScannerState, lexer *gotreesitter.ExternalLexer) int {
	surroundingParens := false
	if lexer.Lookahead() == '(' {
		surroundingParens = true
		djotAdvance(s, lexer)
	}

	listType, ok := djotScanOrderedListType(s, lexer)
	if !ok {
		return djotTokIgnored
	}

	switch lexer.Lookahead() {
	case ')':
		djotAdvance(s, lexer)
		if surroundingParens {
			switch listType {
			case djotDecimal:
				return djotTokListMarkerDecimalParens
			case djotLowerAlpha:
				return djotTokListMarkerLowerAlphaParens
			case djotUpperAlpha:
				return djotTokListMarkerUpperAlphaParens
			case djotLowerRoman:
				return djotTokListMarkerLowerRomanParens
			case djotUpperRoman:
				return djotTokListMarkerUpperRomanParens
			}
		} else {
			switch listType {
			case djotDecimal:
				return djotTokListMarkerDecimalParen
			case djotLowerAlpha:
				return djotTokListMarkerLowerAlphaParen
			case djotUpperAlpha:
				return djotTokListMarkerUpperAlphaParen
			case djotLowerRoman:
				return djotTokListMarkerLowerRomanParen
			case djotUpperRoman:
				return djotTokListMarkerUpperRomanParen
			}
		}
	case '.':
		djotAdvance(s, lexer)
		switch listType {
		case djotDecimal:
			return djotTokListMarkerDecimalPeriod
		case djotLowerAlpha:
			return djotTokListMarkerLowerAlphaPeriod
		case djotUpperAlpha:
			return djotTokListMarkerUpperAlphaPeriod
		case djotLowerRoman:
			return djotTokListMarkerLowerRomanPeriod
		case djotUpperRoman:
			return djotTokListMarkerUpperRomanPeriod
		}
	}
	return djotTokIgnored
}

func djotScanOrderedListMarkerToken(s *djotScannerState, lexer *gotreesitter.ExternalLexer) int {
	res := djotScanOrderedListMarkerTokenType(s, lexer)
	if res == djotTokIgnored {
		return res
	}
	if lexer.Lookahead() == ' ' {
		djotAdvance(s, lexer)
		return res
	}
	return djotTokIgnored
}

// ---------------------------------------------------------------------------
// Task list marker
// ---------------------------------------------------------------------------

func djotScanTaskListMarker(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '[' {
		return false
	}
	djotAdvance(s, lexer)
	ch := lexer.Lookahead()
	if ch != 'x' && ch != 'X' && ch != ' ' {
		return false
	}
	djotAdvance(s, lexer)
	if lexer.Lookahead() != ']' {
		return false
	}
	djotAdvance(s, lexer)
	return lexer.Lookahead() == ' '
}

// ---------------------------------------------------------------------------
// Unordered list marker scanning
// ---------------------------------------------------------------------------

func djotScanUnorderedListMarkerToken(s *djotScannerState, lexer *gotreesitter.ExternalLexer) int {
	if djotScanBulletListMarker(s, lexer, '-') {
		if djotScanTaskListMarker(s, lexer) {
			return djotTokListMarkerTaskBegin
		}
		return djotTokListMarkerDash
	}
	if djotScanBulletListMarker(s, lexer, '*') {
		if djotScanTaskListMarker(s, lexer) {
			return djotTokListMarkerTaskBegin
		}
		return djotTokListMarkerStar
	}
	if djotScanBulletListMarker(s, lexer, '+') {
		if djotScanTaskListMarker(s, lexer) {
			return djotTokListMarkerTaskBegin
		}
		return djotTokListMarkerPlus
	}
	if djotScanBulletListMarker(s, lexer, ':') {
		return djotTokListMarkerDefinition
	}
	return djotTokIgnored
}

func djotScanListMarkerToken(s *djotScannerState, lexer *gotreesitter.ExternalLexer) int {
	unordered := djotScanUnorderedListMarkerToken(s, lexer)
	if unordered != djotTokIgnored {
		return unordered
	}
	return djotScanOrderedListMarkerToken(s, lexer)
}

func djotScanListMarker(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	return djotScanListMarkerToken(s, lexer) != djotTokIgnored
}

// ---------------------------------------------------------------------------
// EOF or blankline
// ---------------------------------------------------------------------------

func djotScanEOFOrBlankline(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() == 0 {
		return true
	}
	if lexer.Lookahead() == '\n' {
		djotAdvance(s, lexer)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Containing block closing marker
// ---------------------------------------------------------------------------

func djotScanContainingBlockClosingMarker(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	return djotIsDivMarkerNext(s, lexer) || djotScanListMarker(s, lexer)
}

// ---------------------------------------------------------------------------
// Ensure list open
// ---------------------------------------------------------------------------

func djotEnsureListOpen(s *djotScannerState, bt djotBlockType, indent uint8) {
	top := djotPeekBlock(s)
	if top != nil && top.blockType == bt && top.data == indent {
		return
	}
	djotPushBlock(s, bt, indent)
}

// ---------------------------------------------------------------------------
// Handle ordered list marker
// ---------------------------------------------------------------------------

func djotHandleOrderedListMarker(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool, marker int) bool {
	if marker != djotTokIgnored && djotValid(vs, marker) {
		djotEnsureListOpen(s, djotListMarkerToBlock(marker), s.indent+1)
		djotSetResult(s, lexer, marker)
		lexer.MarkEnd()
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// consume_line_with_char_or_whitespace
// ---------------------------------------------------------------------------

func djotConsumeLineWithCharOrWhitespace(s *djotScannerState, lexer *gotreesitter.ExternalLexer, c rune) uint8 {
	var seen uint8
	for lexer.Lookahead() != 0 {
		ch := lexer.Lookahead()
		if ch == c {
			seen++
			djotAdvance(s, lexer)
		} else if ch == ' ' {
			djotAdvance(s, lexer)
		} else if ch == '\r' {
			djotAdvance(s, lexer)
		} else if ch == '\n' {
			return seen
		} else {
			return 0
		}
	}
	return seen
}

// ---------------------------------------------------------------------------
// List marker or thematic break
// ---------------------------------------------------------------------------

func djotParseListMarkerOrThematicBreak(
	s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool,
	marker rune, markerType int, listType djotBlockType, thematicBreakType int,
) bool {
	checkFrontmatter := djotValid(vs, djotTokFrontmatterMarker) && marker == '-'

	if !checkFrontmatter && !djotValid(vs, markerType) &&
		!djotValid(vs, thematicBreakType) &&
		!djotValid(vs, djotTokListMarkerTaskBegin) {
		return false
	}

	djotAdvance(s, lexer)

	canBeListMarker := (djotValid(vs, markerType) || djotValid(vs, djotTokListMarkerTaskBegin)) &&
		lexer.Lookahead() == ' '

	var markerCount uint32
	if lexer.Lookahead() == marker {
		markerCount = 2
	} else {
		markerCount = 1
	}

	canBeThematicBreak := djotValid(vs, thematicBreakType) &&
		(markerCount == 2 || lexer.Lookahead() == ' ')

	djotAdvance(s, lexer)
	lexer.MarkEnd()

	if checkFrontmatter {
		markerCount += uint32(djotConsumeChars(s, lexer, marker))
		if markerCount >= 3 {
			djotSetResult(s, lexer, djotTokFrontmatterMarker)
			lexer.MarkEnd()
			return true
		}
	}

	if canBeThematicBreak {
		markerCount += uint32(djotConsumeLineWithCharOrWhitespace(s, lexer, marker))
		if markerCount >= 3 {
			djotSetResult(s, lexer, thematicBreakType)
			lexer.MarkEnd()
			return true
		}
	}

	if canBeListMarker {
		if djotValid(vs, djotTokListMarkerTaskBegin) {
			if djotScanTaskListMarker(s, lexer) {
				djotEnsureListOpen(s, djotListTask, s.indent+1)
				djotSetResult(s, lexer, djotTokListMarkerTaskBegin)
				return true
			}
		}
		if djotValid(vs, markerType) {
			djotEnsureListOpen(s, listType, s.indent+1)
			djotSetResult(s, lexer, markerType)
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Verbatim to end (no newline) - for tables/ref defs
// ---------------------------------------------------------------------------

func djotScanVerbatimToEndNoNewline(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	tickCount := djotConsumeChars(s, lexer, '`')
	if tickCount == 0 {
		return false
	}
	for lexer.Lookahead() != 0 {
		switch lexer.Lookahead() {
		case '\\':
			djotAdvance(s, lexer)
			djotAdvance(s, lexer)
		case '`':
			if djotConsumeChars(s, lexer, '`') == tickCount {
				return true
			}
		case '\n':
			return false
		default:
			djotAdvance(s, lexer)
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Reference definition scanning
// ---------------------------------------------------------------------------

func djotScanRefDef(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 && lexer.Lookahead() != ']' {
		switch lexer.Lookahead() {
		case '\\':
			djotAdvance(s, lexer)
			djotAdvance(s, lexer)
		case '\n':
			return false
		case '`':
			if !djotScanVerbatimToEndNoNewline(s, lexer) {
				return false
			}
		default:
			djotAdvance(s, lexer)
		}
	}
	if lexer.Lookahead() != ']' {
		return false
	}
	djotAdvance(s, lexer)
	if lexer.Lookahead() != ':' {
		return false
	}
	djotAdvance(s, lexer)
	return true
}

func djotParseRefDefBegin(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	if !djotValid(vs, djotTokLinkRefDefMarkBegin) {
		return false
	}
	if !djotScanRefDef(s, lexer) {
		return false
	}
	djotPushBlock(s, djotLinkRefDef, 0)
	djotSetResult(s, lexer, djotTokLinkRefDefMarkBegin)
	return true
}

// ---------------------------------------------------------------------------
// Footnote scanning
// ---------------------------------------------------------------------------

func djotScanFootnoteBegin(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '^' {
		return false
	}
	djotAdvance(s, lexer)
	djotConsumeWhitespace(s, lexer)
	if !djotScanIdentifier(s, lexer) {
		return false
	}
	djotConsumeWhitespace(s, lexer)
	if lexer.Lookahead() != ']' {
		return false
	}
	djotAdvance(s, lexer)
	if lexer.Lookahead() != ':' {
		return false
	}
	djotAdvance(s, lexer)
	return true
}

func djotParseFootnoteBegin(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	if !djotValid(vs, djotTokFootnoteMarkBegin) {
		return false
	}
	if !djotScanFootnoteBegin(s, lexer) {
		return false
	}
	if !djotValid(vs, djotTokInFallback) {
		djotPushBlock(s, djotFootnote, s.indent+2)
	}
	djotSetResult(s, lexer, djotTokFootnoteMarkBegin)
	return true
}

// ---------------------------------------------------------------------------
// Open bracket: footnote or link ref def
// ---------------------------------------------------------------------------

func djotParseOpenBracket(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	if !djotValid(vs, djotTokFootnoteMarkBegin) && !djotValid(vs, djotTokLinkRefDefMarkBegin) {
		return false
	}
	if lexer.Lookahead() != '[' {
		return false
	}
	djotAdvance(s, lexer)
	if lexer.Lookahead() == '^' {
		return djotParseFootnoteBegin(s, lexer, vs)
	}
	return djotParseRefDefBegin(s, lexer, vs)
}

// ---------------------------------------------------------------------------
// Parse dash / star / plus
// ---------------------------------------------------------------------------

func djotParseDash(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	return djotParseListMarkerOrThematicBreak(s, lexer, vs, '-',
		djotTokListMarkerDash, djotListDash, djotTokThematicBreakDash)
}

func djotParseStar(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	return djotParseListMarkerOrThematicBreak(s, lexer, vs, '*',
		djotTokListMarkerStar, djotListStar, djotTokThematicBreakStar)
}

func djotParsePlus(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	if !djotValid(vs, djotTokListMarkerPlus) && !djotValid(vs, djotTokListMarkerTaskBegin) {
		return false
	}
	if !djotScanBulletListMarker(s, lexer, '+') {
		return false
	}
	lexer.MarkEnd()

	if djotValid(vs, djotTokListMarkerTaskBegin) {
		if djotScanTaskListMarker(s, lexer) {
			djotEnsureListOpen(s, djotListTask, s.indent+1)
			djotSetResult(s, lexer, djotTokListMarkerTaskBegin)
			return true
		}
	}
	if djotValid(vs, djotTokListMarkerPlus) {
		djotEnsureListOpen(s, djotListPlus, s.indent+1)
		djotSetResult(s, lexer, djotTokListMarkerPlus)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// List item end
// ---------------------------------------------------------------------------

func djotParseListItemEnd(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	list := djotPeekBlock(s)
	if list == nil || !djotIsList(list.blockType) {
		return false
	}
	if s.indent >= list.data {
		return false
	}
	if len(s.openInline) > 0 {
		return false
	}

	bqMarkerCount, endingNewline := djotScanBlockQuoteMarkers(s, lexer)
	hasBlockQuoteContinuation := false

	if bqMarkerCount > 0 {
		blockQuotes := djotCountBlocks(s, djotBlockQuote)
		if blockQuotes != bqMarkerCount {
			djotSetResult(s, lexer, djotTokListItemEnd)
			s.blocksToClose = 1
			return true
		}

		if endingNewline {
			if djotValid(vs, djotTokBlockQuoteContinuation) {
				hasBlockQuoteContinuation = true
			}
			secondBQMarkerCount, secondNewline := djotScanBlockQuoteMarkers(s, lexer)
			_ = secondNewline
			if blockQuotes != secondBQMarkerCount {
				djotSetResult(s, lexer, djotTokListItemEnd)
				s.blocksToClose = 1
				return true
			}
		}

		if hasBlockQuoteContinuation {
			s.indent = djotConsumeWhitespace(s, lexer)
			if s.indent >= list.data {
				lexer.MarkEnd()
				djotOutputBlockQuoteContinuation(s, lexer, bqMarkerCount, endingNewline)
				return true
			}
		}
	}

	nextMarker := djotScanListMarkerToken(s, lexer)
	if nextMarker != djotTokIgnored {
		differentType := djotListMarkerToBlock(nextMarker) != list.blockType
		differentIndent := list.data != s.indent+1

		if differentType || differentIndent {
			s.blocksToClose = 1
		}
		djotSetResult(s, lexer, djotTokListItemEnd)
		return true
	}

	djotSetResult(s, lexer, djotTokListItemEnd)
	s.blocksToClose = 1
	return true
}

// ---------------------------------------------------------------------------
// Colon: definition list or div
// ---------------------------------------------------------------------------

func djotParseColon(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	canBeDiv := djotValid(vs, djotTokDivBegin) || djotValid(vs, djotTokDivEnd) ||
		djotValid(vs, djotTokBlockClose)
	if !djotValid(vs, djotTokListMarkerDefinition) && !canBeDiv {
		return false
	}

	djotAdvance(s, lexer) // consume first ':'

	if lexer.Lookahead() == ' ' {
		if djotValid(vs, djotTokListMarkerDefinition) {
			djotAdvance(s, lexer) // consume ' '
			djotEnsureListOpen(s, djotListDefinition, s.indent+1)
			djotSetResult(s, lexer, djotTokListMarkerDefinition)
			lexer.MarkEnd()
			return true
		}
		return false
	}

	if !canBeDiv {
		return false
	}

	colons := djotConsumeChars(s, lexer, ':') + 1
	if colons < 3 {
		return false
	}

	fromTop := djotNumberOfBlocksFromTop(s, djotDiv, colons)

	if fromTop == 0 {
		if !djotValid(vs, djotTokDivBegin) {
			return false
		}
		djotPushBlock(s, djotDiv, colons)
		lexer.MarkEnd()
		djotSetResult(s, lexer, djotTokDivBegin)
		return true
	}

	if len(s.openInline) > 0 {
		return false
	}

	if djotValid(vs, djotTokDivEnd) {
		djotRemoveBlock(s)
		lexer.MarkEnd()
		djotSetResult(s, lexer, djotTokDivEnd)
		return true
	}
	if djotValid(vs, djotTokBlockClose) {
		s.blocksToClose = uint8(fromTop) - 1
		djotSetResult(s, lexer, djotTokBlockClose)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Heading
// ---------------------------------------------------------------------------

func djotParseHeading(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	top := djotPeekBlock(s)
	if top != nil && top.blockType == djotCodeBlock {
		return false
	}

	topHeading := top != nil && top.blockType == djotHeading
	hashCount := djotConsumeChars(s, lexer, '#')

	if hashCount > 0 && lexer.Lookahead() == ' ' {
		if !djotValid(vs, djotTokHeadingBegin) && !djotValid(vs, djotTokHeadingContinuation) &&
			!djotValid(vs, djotTokBlockClose) {
			return false
		}
		djotAdvance(s, lexer) // consume ' '

		if djotValid(vs, djotTokHeadingContinuation) && topHeading && top.data == hashCount {
			lexer.MarkEnd()
			djotSetResult(s, lexer, djotTokHeadingContinuation)
			return true
		}

		if djotValid(vs, djotTokBlockClose) && topHeading && top.data != hashCount && len(s.openInline) == 0 {
			djotSetResult(s, lexer, djotTokBlockClose)
			djotRemoveBlock(s)
			return true
		}

		if djotValid(vs, djotTokHeadingBegin) {
			if top == nil || (top.blockType == djotSection && top.data < hashCount) {
				djotPushBlock(s, djotSection, hashCount)
			} else if top != nil && top.blockType == djotSection && top.data >= hashCount {
				djotSetResult(s, lexer, djotTokBlockClose)
				djotRemoveBlock(s)
				return true
			}
			djotPushBlock(s, djotHeading, hashCount)
			lexer.MarkEnd()
			djotSetResult(s, lexer, djotTokHeadingBegin)
			return true
		}
	} else if hashCount == 0 && topHeading {
		if djotValid(vs, djotTokBlockClose) &&
			(djotScanEOFOrBlankline(s, lexer) || djotScanContainingBlockClosingMarker(s, lexer)) {
			djotRemoveBlock(s)
			djotSetResult(s, lexer, djotTokBlockClose)
			return true
		}
		if djotValid(vs, djotTokHeadingContinuation) {
			djotSetResult(s, lexer, djotTokHeadingContinuation)
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Footnote end / continuation
// ---------------------------------------------------------------------------

func djotParseFootnoteEnd(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	top := djotPeekBlock(s)
	if top == nil || top.blockType != djotFootnote {
		return false
	}
	if s.indent >= top.data {
		return false
	}
	if len(s.openInline) > 0 {
		return false
	}
	djotRemoveBlock(s)
	djotSetResult(s, lexer, djotTokFootnoteEnd)
	return true
}

func djotParseFootnoteContinuation(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	footnote := djotPeekBlock(s)
	if footnote == nil || footnote.blockType != djotFootnote {
		return false
	}
	if s.indent < footnote.data {
		return false
	}
	lexer.MarkEnd()
	djotSetResult(s, lexer, djotTokFootnoteContinuation)
	return true
}

// ---------------------------------------------------------------------------
// Table scanning
// ---------------------------------------------------------------------------

func djotScanTableCell(s *djotScannerState, lexer *gotreesitter.ExternalLexer) (ok bool, separator bool) {
	djotConsumeWhitespace(s, lexer)
	separator = true
	firstChar := true

	for lexer.Lookahead() != 0 {
		switch lexer.Lookahead() {
		case '\\':
			separator = false
			djotAdvance(s, lexer)
			djotAdvance(s, lexer)
		case '\n':
			return false, separator
		case '`':
			separator = false
			if !djotScanVerbatimToEndNoNewline(s, lexer) {
				return false, separator
			}
		case '|':
			return true, separator
		case ':':
			djotAdvance(s, lexer)
			djotConsumeWhitespace(s, lexer)
			if lexer.Lookahead() == '|' {
				return true, separator
			} else if !firstChar {
				separator = false
			}
		case '-':
			djotAdvance(s, lexer)
		default:
			separator = false
			djotAdvance(s, lexer)
		}
		firstChar = false
	}
	return false, separator
}

func djotScanSeparatorRow(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	var cellCount uint8
	for {
		ok, currSeparator := djotScanTableCell(s, lexer)
		if !ok {
			break
		}
		if !currSeparator {
			return false
		}
		cellCount++
		if lexer.Lookahead() == '|' {
			djotAdvance(s, lexer)
		}
	}
	if cellCount == 0 {
		return false
	}
	djotConsumeWhitespace(s, lexer)
	return lexer.Lookahead() == '\n'
}

func djotScanTableRow(s *djotScannerState, lexer *gotreesitter.ExternalLexer) (rowType int, ok bool) {
	if s.state&djotStateTableSeparatorNext != 0 {
		s.state &^= djotStateTableSeparatorNext
		return djotTokTableSeparatorBegin, true
	}

	var cellCount uint8
	allSeparators := true
	for {
		cellOk, currSeparator := djotScanTableCell(s, lexer)
		if !cellOk {
			break
		}
		if !currSeparator {
			allSeparators = false
		}
		cellCount++
		if lexer.Lookahead() == '|' {
			djotAdvance(s, lexer)
		}
	}

	if cellCount == 0 {
		return 0, false
	}

	djotConsumeWhitespace(s, lexer)
	if lexer.Lookahead() != '\n' {
		return 0, false
	}
	djotAdvance(s, lexer)

	if allSeparators {
		return djotTokTableSeparatorBegin, true
	}

	// Check if next row is separator.
	_, newline := djotScanBlockQuoteMarkers(s, lexer)
	if !newline && djotScanSeparatorRow(s, lexer) {
		s.state |= djotStateTableSeparatorNext
		return djotTokTableHeaderBegin, true
	}
	return djotTokTableRowBegin, true
}

func djotParseTableBegin(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	if lexer.Lookahead() != '|' {
		return false
	}
	if !djotValid(vs, djotTokTableRowBegin) &&
		!djotValid(vs, djotTokTableSeparatorBegin) &&
		!djotValid(vs, djotTokTableHeaderBegin) {
		return false
	}

	djotAdvance(s, lexer) // consume |
	lexer.MarkEnd()

	rowType, ok := djotScanTableRow(s, lexer)
	if !ok {
		return false
	}

	djotPushBlock(s, djotTableRow, 0)
	djotSetResult(s, lexer, rowType)
	return true
}

func djotParseTableEndNewline(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '\n' {
		return false
	}
	top := djotPeekBlock(s)
	if top == nil || top.blockType != djotTableRow {
		return false
	}
	djotRemoveBlock(s)
	djotAdvance(s, lexer)
	djotSetResult(s, lexer, djotTokTableRowEndNewline)
	lexer.MarkEnd()
	return true
}

func djotParseTableCellEnd(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '|' {
		return false
	}
	if len(s.openInline) > 0 {
		return false
	}
	top := djotPeekBlock(s)
	if top == nil || top.blockType != djotTableRow {
		return false
	}
	if top.data > 0 {
		top.data--
	}
	djotAdvance(s, lexer)
	djotSetResult(s, lexer, djotTokTableCellEnd)
	lexer.MarkEnd()
	return true
}

func djotParseTableCaptionBegin(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '^' {
		return false
	}
	djotAdvance(s, lexer)
	if lexer.Lookahead() != ' ' {
		return false
	}
	djotAdvance(s, lexer)
	djotPushBlock(s, djotTableCaption, s.indent+2)
	lexer.MarkEnd()
	djotSetResult(s, lexer, djotTokTableCaptionBegin)
	return true
}

func djotParseTableCaptionEnd(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	caption := djotPeekBlock(s)
	if caption == nil || caption.blockType != djotTableCaption {
		return false
	}
	if len(s.openInline) > 0 {
		return false
	}
	if s.indent >= caption.data {
		return false
	}
	djotRemoveBlock(s)
	djotSetResult(s, lexer, djotTokTableCaptionEnd)
	return true
}

// ---------------------------------------------------------------------------
// Comment scanning
// ---------------------------------------------------------------------------

func djotScanComment(s *djotScannerState, lexer *gotreesitter.ExternalLexer, indent uint8) (ok bool, mustBeInlineComment bool) {
	if lexer.Lookahead() != '%' {
		return false, false
	}
	djotAdvance(s, lexer)

	for lexer.Lookahead() != 0 {
		switch lexer.Lookahead() {
		case '%':
			djotAdvance(s, lexer)
			return true, mustBeInlineComment
		case '}':
			return true, mustBeInlineComment
		case '\\':
			djotAdvance(s, lexer)
		case '\n':
			djotAdvance(s, lexer)
			if indent != djotConsumeWhitespace(s, lexer) {
				mustBeInlineComment = true
			}
			if lexer.Lookahead() == '\n' {
				return false, mustBeInlineComment
			}
		}
		djotAdvance(s, lexer)
	}
	return false, mustBeInlineComment
}

func djotScanValue(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() == '"' {
		djotAdvance(s, lexer)
		if !djotScanUntilUnescaped(s, lexer, '"') {
			return false
		}
		djotAdvance(s, lexer)
		return true
	}
	return djotScanIdentifier(s, lexer)
}

// ---------------------------------------------------------------------------
// Open curly bracket: block attribute or inline comment
// ---------------------------------------------------------------------------

func djotParseOpenCurlyBracket(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	if !djotValid(vs, djotTokBlockAttributeBegin) && !djotValid(vs, djotTokInlineCommentBegin) {
		return false
	}
	if lexer.Lookahead() != '{' {
		return false
	}
	djotAdvance(s, lexer)
	lexer.MarkEnd()

	indent := s.indent + 1
	canBeInlineComment := lexer.Lookahead() == '%'
	mustBeInlineComment := false

	for lexer.Lookahead() != 0 {
		space := djotConsumeWhitespace(s, lexer)
		if space > 0 {
			canBeInlineComment = false
		}

		switch lexer.Lookahead() {
		case '\\':
			canBeInlineComment = false
			djotAdvance(s, lexer)
			djotAdvance(s, lexer)
		case '}':
			if canBeInlineComment && djotValid(vs, djotTokInlineCommentBegin) {
				djotSetResult(s, lexer, djotTokInlineCommentBegin)
				return true
			} else if !mustBeInlineComment && djotValid(vs, djotTokBlockAttributeBegin) {
				djotSetResult(s, lexer, djotTokBlockAttributeBegin)
				return true
			}
			return false
		case '.':
			canBeInlineComment = false
			djotAdvance(s, lexer)
			if !djotScanIdentifier(s, lexer) {
				return false
			}
		case '#':
			canBeInlineComment = false
			djotAdvance(s, lexer)
			if !djotScanIdentifier(s, lexer) {
				return false
			}
		case '%':
			ok, mustInline := djotScanComment(s, lexer, indent)
			if !ok {
				return false
			}
			if mustInline {
				mustBeInlineComment = true
			}
		case '\n':
			canBeInlineComment = false
			djotAdvance(s, lexer)
			if indent != djotConsumeWhitespace(s, lexer) {
				return false
			}
			if lexer.Lookahead() == '\n' {
				return false
			}
		default:
			canBeInlineComment = false
			if !djotScanIdentifier(s, lexer) {
				return false
			}
			if lexer.Lookahead() != '=' {
				return false
			}
			djotAdvance(s, lexer)
			if !djotScanValue(s, lexer) {
				return false
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Hard line break
// ---------------------------------------------------------------------------

func djotParseHardLineBreak(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '\\' {
		return false
	}
	djotAdvance(s, lexer)
	lexer.MarkEnd()
	if lexer.Lookahead() != '\n' {
		return false
	}
	djotSetResult(s, lexer, djotTokHardLineBreak)
	return true
}

// ---------------------------------------------------------------------------
// Paragraph closing
// ---------------------------------------------------------------------------

func djotEndParagraphInBlockQuote(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	block := djotFindBlock(s, djotBlockQuote)
	if block == nil {
		return false
	}
	markerCount, endingNewline := djotScanBlockQuoteMarkers(s, lexer)
	if markerCount == 0 {
		return false
	}
	if markerCount < block.data || endingNewline {
		return true
	}
	if block != djotPeekBlock(s) && djotScanContainingBlockClosingMarker(s, lexer) {
		return true
	}
	djotConsumeWhitespace(s, lexer)
	return lexer.Lookahead() == '\n'
}

func djotScanBlockMathMarker(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '$' {
		return false
	}
	djotAdvance(s, lexer)
	if lexer.Lookahead() != '$' {
		return false
	}
	djotAdvance(s, lexer)
	if lexer.Lookahead() != '`' {
		return false
	}
	djotAdvance(s, lexer)
	return true
}

func djotCloseParagraph(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	top := djotPeekBlock(s)
	if top != nil && top.blockType == djotBlockQuote && lexer.Lookahead() == '\n' {
		return true
	}
	if djotEndParagraphInBlockQuote(s, lexer) {
		return true
	}
	if djotScanContainingBlockClosingMarker(s, lexer) {
		return true
	}
	if djotScanBlockMathMarker(s, lexer) {
		return true
	}
	return false
}

func djotParseCloseParagraph(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if len(s.openInline) > 0 {
		return false
	}
	if !djotCloseParagraph(s, lexer) {
		return false
	}
	djotSetResult(s, lexer, djotTokCloseParagraph)
	return true
}

// ---------------------------------------------------------------------------
// Newline parsing
// ---------------------------------------------------------------------------

func djotEmitNewlineInline(s *djotScannerState, lexer *gotreesitter.ExternalLexer, newlineColumn uint32) bool {
	if lexer.Lookahead() == 0 {
		return false
	}
	if newlineColumn == 0 {
		return false
	}
	top := djotPeekBlock(s)
	if djotDisallowNewline(top) {
		return false
	}
	if top != nil && top.blockType == djotHeading {
		return false
	}
	nextLineWhitespace := djotConsumeWhitespace(s, lexer)
	if lexer.Lookahead() == '\n' {
		return false
	}
	if top != nil && top.blockType == djotTableCaption && nextLineWhitespace < top.data {
		return false
	}
	if djotCloseParagraph(s, lexer) {
		return false
	}
	djotSetResult(s, lexer, djotTokNewlineInline)
	return true
}

func djotParseNewline(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	if djotValid(vs, djotTokTableRowEndNewline) && djotParseTableEndNewline(s, lexer) {
		return true
	}
	if djotValid(vs, djotTokVerbatimEnd) && djotTryImplicitCloseVerbatim(s, lexer) {
		return true
	}
	if !djotValid(vs, djotTokNewline) && !djotValid(vs, djotTokNewlineInline) &&
		!djotValid(vs, djotTokEOFOrNewline) {
		return false
	}
	top := djotPeekBlock(s)
	if djotDisallowNewline(top) {
		return false
	}
	newlineColumn := lexer.Column()
	if lexer.Lookahead() == '\n' {
		djotAdvance(s, lexer)
	}
	lexer.MarkEnd()

	if djotValid(vs, djotTokNewlineInline) && djotEmitNewlineInline(s, lexer, newlineColumn) {
		djotSetResult(s, lexer, djotTokNewlineInline)
		return true
	}
	if len(s.openInline) > 0 {
		return false
	}
	if djotValid(vs, djotTokNewline) {
		djotSetResult(s, lexer, djotTokNewline)
		return true
	}
	if djotValid(vs, djotTokEOFOrNewline) {
		djotSetResult(s, lexer, djotTokEOFOrNewline)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Comment end / close
// ---------------------------------------------------------------------------

func djotParseCommentEnd(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	if djotValid(vs, djotTokCommentEndMarker) && lexer.Lookahead() == '%' {
		djotAdvance(s, lexer)
		lexer.MarkEnd()
		djotSetResult(s, lexer, djotTokCommentEndMarker)
		return true
	}
	if djotValid(vs, djotTokCommentClose) && lexer.Lookahead() == '}' {
		djotSetResult(s, lexer, djotTokCommentClose)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Link ref def label end
// ---------------------------------------------------------------------------

func djotParseLinkRefDefLabelEnd(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != ']' {
		return false
	}
	top := djotPeekBlock(s)
	if top == nil || top.blockType != djotLinkRefDef {
		return false
	}
	if len(s.openInline) > 0 {
		return false
	}
	djotRemoveBlock(s)
	djotSetResult(s, lexer, djotTokLinkRefDefLabelEnd)
	return true
}

// ---------------------------------------------------------------------------
// Span helpers for inline elements
// ---------------------------------------------------------------------------

func djotInlineSpanType(t djotInlineType) djotSpanType {
	switch t {
	case djotInlineEmphasis, djotInlineStrong:
		return djotSpanBracketedAndSingleNoWhitespace
	case djotInlineSuperscript, djotInlineSubscript:
		return djotSpanBracketedAndSingle
	case djotInlineHighlighted, djotInlineInsert, djotInlineDelete:
		return djotSpanBracketed
	case djotInlineParensSpan, djotInlineCurlyBracketSpan, djotInlineSquareBracketSpan:
		return djotSpanSingle
	}
	return djotSpanSingle
}

func djotInlineBeginToken(t djotInlineType) int {
	switch t {
	case djotInlineVerbatim:
		return djotTokVerbatimBegin
	case djotInlineEmphasis:
		return djotTokEmphasisMarkBegin
	case djotInlineStrong:
		return djotTokStrongMarkBegin
	case djotInlineSuperscript:
		return djotTokSuperscriptMarkBegin
	case djotInlineSubscript:
		return djotTokSubscriptMarkBegin
	case djotInlineHighlighted:
		return djotTokHighlightedMarkBegin
	case djotInlineInsert:
		return djotTokInsertMarkBegin
	case djotInlineDelete:
		return djotTokDeleteMarkBegin
	case djotInlineParensSpan:
		return djotTokParensSpanMarkBegin
	case djotInlineCurlyBracketSpan:
		return djotTokCurlyBracketSpanMarkBegin
	case djotInlineSquareBracketSpan:
		return djotTokSquareBracketSpanMarkBegin
	}
	return djotTokError
}

func djotInlineEndToken(t djotInlineType) int {
	switch t {
	case djotInlineVerbatim:
		return djotTokVerbatimEnd
	case djotInlineEmphasis:
		return djotTokEmphasisEnd
	case djotInlineStrong:
		return djotTokStrongEnd
	case djotInlineSuperscript:
		return djotTokSuperscriptEnd
	case djotInlineSubscript:
		return djotTokSubscriptEnd
	case djotInlineHighlighted:
		return djotTokHighlightedEnd
	case djotInlineInsert:
		return djotTokInsertEnd
	case djotInlineDelete:
		return djotTokDeleteEnd
	case djotInlineParensSpan:
		return djotTokParensSpanEnd
	case djotInlineCurlyBracketSpan:
		return djotTokCurlyBracketSpanEnd
	case djotInlineSquareBracketSpan:
		return djotTokSquareBracketSpanEnd
	}
	return djotTokError
}

func djotInlineMarkerChar(t djotInlineType) rune {
	switch t {
	case djotInlineEmphasis:
		return '_'
	case djotInlineStrong:
		return '*'
	case djotInlineSuperscript:
		return '^'
	case djotInlineSubscript:
		return '~'
	case djotInlineHighlighted:
		return '='
	case djotInlineInsert:
		return '+'
	case djotInlineDelete:
		return '-'
	case djotInlineParensSpan:
		return ')'
	case djotInlineCurlyBracketSpan:
		return '}'
	case djotInlineSquareBracketSpan:
		return ']'
	}
	return '`'
}

func djotFindInline(s *djotScannerState, t djotInlineType) *djotInline {
	for i := len(s.openInline) - 1; i >= 0; i-- {
		if s.openInline[i].inlineType == t {
			return &s.openInline[i]
		}
	}
	return nil
}

func djotScanSingleSpanEnd(s *djotScannerState, lexer *gotreesitter.ExternalLexer, marker rune) bool {
	if lexer.Lookahead() != marker {
		return false
	}
	djotAdvance(s, lexer)
	return true
}

func djotScanBracketedSpanEnd(s *djotScannerState, lexer *gotreesitter.ExternalLexer, marker rune) bool {
	if lexer.Lookahead() != marker {
		return false
	}
	djotAdvance(s, lexer)
	if lexer.Lookahead() != '}' {
		return false
	}
	djotAdvance(s, lexer)
	return true
}

func djotScanSpanEnd(s *djotScannerState, lexer *gotreesitter.ExternalLexer, marker rune, whitespaceSensitive bool) bool {
	if lexer.Lookahead() == marker {
		djotAdvance(s, lexer)
		if lexer.Lookahead() == '}' {
			djotAdvance(s, lexer)
		}
		return true
	}
	if whitespaceSensitive && djotConsumeWhitespace(s, lexer) == 0 {
		return false
	}
	return djotScanBracketedSpanEnd(s, lexer, marker)
}

func djotScanSpanEndMarker(s *djotScannerState, lexer *gotreesitter.ExternalLexer, element djotInlineType) bool {
	marker := djotInlineMarkerChar(element)
	switch djotInlineSpanType(element) {
	case djotSpanSingle:
		return djotScanSingleSpanEnd(s, lexer, marker)
	case djotSpanBracketed:
		return djotScanBracketedSpanEnd(s, lexer, marker)
	case djotSpanBracketedAndSingle:
		return djotScanSpanEnd(s, lexer, marker, false)
	case djotSpanBracketedAndSingleNoWhitespace:
		return djotScanSpanEnd(s, lexer, marker, true)
	}
	return false
}

// scan_until: scans until c, aborting if ending marker for top is found.
func djotScanUntil(s *djotScannerState, lexer *gotreesitter.ExternalLexer, c rune, topType *djotInlineType) bool {
	for lexer.Lookahead() != 0 {
		if topType != nil && djotScanSpanEndMarker(s, lexer, *topType) {
			return false
		}
		if lexer.Lookahead() == c {
			return true
		} else if lexer.Lookahead() == '\\' {
			djotAdvance(s, lexer)
			djotAdvance(s, lexer)
		} else if lexer.Lookahead() == '\n' {
			djotAdvance(s, lexer)
			djotConsumeWhitespace(s, lexer)
			if lexer.Lookahead() == '\n' {
				return false
			}
		} else {
			djotAdvance(s, lexer)
		}
	}
	return false
}

func djotUpdateSquareBracketLookaheadStates(s *djotScannerState, lexer *gotreesitter.ExternalLexer, top *djotInline) {
	s.state &^= djotStateBracketStartsInlineLink
	s.state &^= djotStateBracketStartsSpan

	var topType *djotInlineType
	if top != nil {
		t := top.inlineType
		topType = &t
	}

	if !djotScanUntil(s, lexer, ']', topType) {
		return
	}
	djotAdvance(s, lexer)

	if lexer.Lookahead() == '(' {
		if djotScanUntil(s, lexer, ')', topType) {
			s.state |= djotStateBracketStartsInlineLink
		}
	} else if lexer.Lookahead() == '{' {
		if djotScanUntil(s, lexer, '}', topType) {
			s.state |= djotStateBracketStartsSpan
		}
	}
}

func djotMarkSpanBegin(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool, inlineType djotInlineType, token int) bool {
	top := djotPeekInline(s)

	if djotValid(vs, djotTokInFallback) {
		if inlineType == djotInlineSquareBracketSpan {
			djotUpdateSquareBracketLookaheadStates(s, lexer, top)
		}
		if inlineType == djotInlineParensSpan && (s.state&djotStateBracketStartsInlineLink != 0) {
			return false
		}
		if inlineType == djotInlineCurlyBracketSpan && (s.state&djotStateBracketStartsSpan != 0) {
			return false
		}

		open := djotFindInline(s, inlineType)
		if open != nil {
			open.data++
		}
		djotSetResult(s, lexer, token)
		return true
	}

	if inlineType == djotInlineParensSpan {
		s.state &^= djotStateBracketStartsInlineLink
	} else if inlineType == djotInlineCurlyBracketSpan {
		s.state &^= djotStateBracketStartsSpan
	}

	djotSetResult(s, lexer, token)
	djotPushInline(s, inlineType, 0)
	return true
}

func djotParseSpanEnd(s *djotScannerState, lexer *gotreesitter.ExternalLexer, element djotInlineType, token int) bool {
	top := djotPeekInline(s)
	if top == nil || top.inlineType != element {
		return false
	}
	if top.data > 0 {
		return false
	}
	if !djotScanSpanEndMarker(s, lexer, element) {
		return false
	}
	lexer.MarkEnd()
	djotSetResult(s, lexer, token)
	djotRemoveInline(s)
	return true
}

func djotParseSpan(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool, element djotInlineType) bool {
	beginToken := djotInlineBeginToken(element)
	endToken := djotInlineEndToken(element)
	if djotValid(vs, endToken) && djotParseSpanEnd(s, lexer, element, endToken) {
		return true
	}
	if djotValid(vs, beginToken) && djotMarkSpanBegin(s, lexer, vs, element, beginToken) {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Non-whitespace check
// ---------------------------------------------------------------------------

func djotCheckNonWhitespace(s *djotScannerState, lexer *gotreesitter.ExternalLexer) bool {
	switch lexer.Lookahead() {
	case ' ', '\t', '\r', '\n':
		return false
	default:
		djotSetResult(s, lexer, djotTokNonWhitespaceCheck)
		return true
	}
}

// ---------------------------------------------------------------------------
// Main scan function
// ---------------------------------------------------------------------------

func djotScan(s *djotScannerState, lexer *gotreesitter.ExternalLexer, vs []bool) bool {
	// Mark end right from the start.
	lexer.MarkEnd()

	// Skip carriage returns.
	if lexer.Lookahead() == '\r' {
		djotAdvance(s, lexer)
	}

	// At column 0, consume leading whitespace and track indentation.
	if lexer.Column() == 0 {
		s.indent = djotConsumeWhitespace(s, lexer)
	}
	isNewline := lexer.Lookahead() == '\n'

	if isNewline {
		s.blockQuoteLevel = 0
	}

	// Error recovery.
	if djotValid(vs, djotTokError) {
		djotSetResult(s, lexer, djotTokError)
		return true
	}

	// Handle pending block closes.
	if djotValid(vs, djotTokBlockClose) && djotHandleBlocksToClose(s, lexer) {
		return true
	}
	if s.blocksToClose > 0 {
		return false
	}

	// Close list nested block if needed.
	if djotValid(vs, djotTokBlockClose) && djotCloseListNestedBlockIfNeeded(s, lexer, !isNewline) {
		return true
	}

	// Newline handling.
	if isNewline && djotParseNewline(s, lexer, vs) {
		return true
	}

	// Backtick: code blocks and verbatim.
	if lexer.Lookahead() == '`' && djotParseBacktick(s, lexer, vs) {
		return true
	}
	// Colon: definition list or div.
	if lexer.Lookahead() == ':' && djotParseColon(s, lexer, vs) {
		return true
	}

	// Indented content spacer.
	if djotValid(vs, djotTokIndentedContentSpacer) && djotParseIndentedContentSpacer(s, lexer, isNewline) {
		return true
	}

	// List item continuation.
	if djotValid(vs, djotTokListItemContinuation) && djotParseListItemContinuation(s, lexer) {
		return true
	}
	// Footnote continuation.
	if djotValid(vs, djotTokFootnoteContinuation) && djotParseFootnoteContinuation(s, lexer) {
		return true
	}

	// Verbatim content.
	if djotValid(vs, djotTokVerbatimContent) && djotParseVerbatimContent(s, lexer) {
		return true
	}

	// Close paragraph.
	if djotValid(vs, djotTokCloseParagraph) && djotParseCloseParagraph(s, lexer) {
		return true
	}
	// Footnote end.
	if djotValid(vs, djotTokFootnoteEnd) && djotParseFootnoteEnd(s, lexer) {
		return true
	}
	// Link ref def label end.
	if djotValid(vs, djotTokLinkRefDefLabelEnd) && djotParseLinkRefDefLabelEnd(s, lexer) {
		return true
	}

	// End previous list item before opening new ones.
	if djotValid(vs, djotTokListItemEnd) && djotParseListItemEnd(s, lexer, vs) {
		return true
	}

	// Block quote.
	if djotParseBlockQuote(s, lexer, vs) {
		return true
	}
	// Heading.
	if djotParseHeading(s, lexer, vs) {
		return true
	}
	// Comment end.
	if djotParseCommentEnd(s, lexer, vs) {
		return true
	}

	// Character-specific block-level parsing.
	switch lexer.Lookahead() {
	case '[':
		if djotParseOpenBracket(s, lexer, vs) {
			return true
		}
	case '-':
		if djotParseDash(s, lexer, vs) {
			return true
		}
	case '*':
		if djotParseStar(s, lexer, vs) {
			return true
		}
	case '+':
		if djotParsePlus(s, lexer, vs) {
			return true
		}
	case '|':
		if djotParseTableBegin(s, lexer, vs) {
			return true
		}
	case '{':
		if djotParseOpenCurlyBracket(s, lexer, vs) {
			return true
		}
	}

	// Non-whitespace check.
	if djotValid(vs, djotTokNonWhitespaceCheck) && djotCheckNonWhitespace(s, lexer) {
		return true
	}

	// Inline span scanning.
	if djotParseSpan(s, lexer, vs, djotInlineEmphasis) {
		return true
	}
	if djotParseSpan(s, lexer, vs, djotInlineStrong) {
		return true
	}
	if djotParseSpan(s, lexer, vs, djotInlineSuperscript) {
		return true
	}
	if djotParseSpan(s, lexer, vs, djotInlineSubscript) {
		return true
	}
	if djotParseSpan(s, lexer, vs, djotInlineHighlighted) {
		return true
	}
	if djotParseSpan(s, lexer, vs, djotInlineInsert) {
		return true
	}
	if djotParseSpan(s, lexer, vs, djotInlineDelete) {
		return true
	}
	if djotParseSpan(s, lexer, vs, djotInlineParensSpan) {
		return true
	}
	if djotParseSpan(s, lexer, vs, djotInlineCurlyBracketSpan) {
		return true
	}
	if djotParseSpan(s, lexer, vs, djotInlineSquareBracketSpan) {
		return true
	}

	// Ordered list markers.
	orderedListMarker := djotScanOrderedListMarkerToken(s, lexer)
	if orderedListMarker != djotTokIgnored && djotHandleOrderedListMarker(s, lexer, vs, orderedListMarker) {
		return true
	}

	// Table caption.
	if djotValid(vs, djotTokTableCaptionEnd) && djotParseTableCaptionEnd(s, lexer) {
		return true
	}
	if djotValid(vs, djotTokTableCaptionBegin) && djotParseTableCaptionBegin(s, lexer) {
		return true
	}

	// Table cell end.
	if djotValid(vs, djotTokTableCellEnd) && djotParseTableCellEnd(s, lexer) {
		return true
	}

	// Hard line break.
	if djotValid(vs, djotTokHardLineBreak) && djotParseHardLineBreak(s, lexer) {
		return true
	}

	// Close different typed list.
	if djotValid(vs, djotTokBlockClose) && djotTryCloseDifferentTypedList(s, lexer, orderedListMarker) {
		return true
	}

	// EOF.
	if djotValid(vs, djotTokEOFOrNewline) && lexer.Lookahead() == 0 {
		djotSetResult(s, lexer, djotTokEOFOrNewline)
		return true
	}

	return false
}
