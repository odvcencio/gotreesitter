//go:build !grammar_subset || grammar_subset_markdown

package grammarruntime

import (
	"strings"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Markdown grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see mdDefaultSymTable below.
const (
	mdTokLineEnding                         = 0
	mdTokSoftLineEnding                     = 1
	mdTokBlockClose                         = 2
	mdTokBlockContinuation                  = 3
	mdTokBlockQuoteStart                    = 4
	mdTokIndentedChunkStart                 = 5
	mdTokAtxH1Marker                        = 6
	mdTokAtxH2Marker                        = 7
	mdTokAtxH3Marker                        = 8
	mdTokAtxH4Marker                        = 9
	mdTokAtxH5Marker                        = 10
	mdTokAtxH6Marker                        = 11
	mdTokSetextH1Underline                  = 12
	mdTokSetextH2Underline                  = 13
	mdTokThematicBreak                      = 14
	mdTokListMarkerMinus                    = 15
	mdTokListMarkerPlus                     = 16
	mdTokListMarkerStar                     = 17
	mdTokListMarkerParenthesis              = 18
	mdTokListMarkerDot                      = 19
	mdTokListMarkerMinusDontInterrupt       = 20
	mdTokListMarkerPlusDontInterrupt        = 21
	mdTokListMarkerStarDontInterrupt        = 22
	mdTokListMarkerParenthesisDontInterrupt = 23
	mdTokListMarkerDotDontInterrupt         = 24
	mdTokFencedCodeBlockStartBacktick       = 25
	mdTokFencedCodeBlockStartTilde          = 26
	mdTokBlankLineStart                     = 27
	mdTokFencedCodeBlockEndBacktick         = 28
	mdTokFencedCodeBlockEndTilde            = 29
	mdTokHTMLBlock1Start                    = 30
	mdTokHTMLBlock1End                      = 31
	mdTokHTMLBlock2Start                    = 32
	mdTokHTMLBlock3Start                    = 33
	mdTokHTMLBlock4Start                    = 34
	mdTokHTMLBlock5Start                    = 35
	mdTokHTMLBlock6Start                    = 36
	mdTokHTMLBlock7Start                    = 37
	mdTokCloseBlock                         = 38
	mdTokNoIndentedChunk                    = 39
	mdTokError                              = 40
	mdTokTriggerError                       = 41
	mdTokEOF                                = 42
	mdTokMinusMetadata                      = 43
	mdTokPlusMetadata                       = 44
	mdTokPipeTableStart                     = 45
	mdTokPipeTableLineEnding                = 46
	mdTokenCount                            = 47
)

// mdDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped markdown.bin assigns to each external, in mdTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order. All 47 externals sit in one contiguous run (43-89).
var mdDefaultSymTable = [mdTokenCount]gotreesitter.Symbol{
	43, // _line_ending
	44, // _soft_line_ending
	45, // _block_close
	46, // block_continuation
	47, // _block_quote_start (display: block_quote_marker)
	48, // _indented_chunk_start
	49, // atx_h1_marker
	50, // atx_h2_marker
	51, // atx_h3_marker
	52, // atx_h4_marker
	53, // atx_h5_marker
	54, // atx_h6_marker
	55, // setext_h1_underline
	56, // setext_h2_underline
	57, // _thematic_break
	58, // _list_marker_minus
	59, // _list_marker_plus
	60, // _list_marker_star
	61, // _list_marker_parenthesis
	62, // _list_marker_dot
	63, // _list_marker_minus_dont_interrupt
	64, // _list_marker_plus_dont_interrupt
	65, // _list_marker_star_dont_interrupt
	66, // _list_marker_parenthesis_dont_interrupt
	67, // _list_marker_dot_dont_interrupt
	68, // _fenced_code_block_start_backtick (display: fenced_code_block_delimiter)
	69, // _fenced_code_block_start_tilde (display: fenced_code_block_delimiter)
	70, // _blank_line_start
	71, // _fenced_code_block_end_backtick (display: fenced_code_block_delimiter)
	72, // _fenced_code_block_end_tilde (display: fenced_code_block_delimiter)
	73, // _html_block_1_start
	74, // _html_block_1_end
	75, // _html_block_2_start
	76, // _html_block_3_start
	77, // _html_block_4_start
	78, // _html_block_5_start
	79, // _html_block_6_start
	80, // _html_block_7_start
	81, // _close_block
	82, // _no_indented_chunk
	83, // _error
	84, // _trigger_error
	85, // _eof
	86, // minus_metadata
	87, // plus_metadata
	88, // _pipe_table_start
	89, // _pipe_table_line_ending
}

// mdExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (mdTok* order).
var mdExternalScannerSpec = ExternalScannerSpec{
	Language:       "markdown",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-markdown",
	UpstreamCommit: "a0a00f817d02412bd92c54d316f164d827b57b5c",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "tree-sitter-markdown/src/grammar.json", SHA256: "a96d7cd418f0af885bdee8cbdef098879c1542e8895fe4f977e248b630011ce2"},
		{Path: "tree-sitter-markdown/src/scanner.c", SHA256: "02834bcbaf0cf51178e74450d487cc0231b9a52541cd374527ee35cfaf17fd4a"},
	},
	Externals: []string{
		"_line_ending",
		"_soft_line_ending",
		"_block_close",
		"block_continuation",
		"_block_quote_start",
		"_indented_chunk_start",
		"atx_h1_marker",
		"atx_h2_marker",
		"atx_h3_marker",
		"atx_h4_marker",
		"atx_h5_marker",
		"atx_h6_marker",
		"setext_h1_underline",
		"setext_h2_underline",
		"_thematic_break",
		"_list_marker_minus",
		"_list_marker_plus",
		"_list_marker_star",
		"_list_marker_parenthesis",
		"_list_marker_dot",
		"_list_marker_minus_dont_interrupt",
		"_list_marker_plus_dont_interrupt",
		"_list_marker_star_dont_interrupt",
		"_list_marker_parenthesis_dont_interrupt",
		"_list_marker_dot_dont_interrupt",
		"_fenced_code_block_start_backtick",
		"_fenced_code_block_start_tilde",
		"_blank_line_start",
		"_fenced_code_block_end_backtick",
		"_fenced_code_block_end_tilde",
		"_html_block_1_start",
		"_html_block_1_end",
		"_html_block_2_start",
		"_html_block_3_start",
		"_html_block_4_start",
		"_html_block_5_start",
		"_html_block_6_start",
		"_html_block_7_start",
		"_close_block",
		"_no_indented_chunk",
		"_error",
		"_trigger_error",
		"_eof",
		"minus_metadata",
		"plus_metadata",
		"_pipe_table_start",
		"_pipe_table_line_ending",
	},
}

func init() {
	RegisterExternalScannerSpec(mdExternalScannerSpec)
}

// Block types
type mdBlock uint8

const (
	mdBlockQuote mdBlock = iota
	mdIndentedCodeBlock
	mdListItem
	mdListItem1Indent
	mdListItem2Indent
	mdListItem3Indent
	mdListItem4Indent
	mdListItem5Indent
	mdListItem6Indent
	mdListItem7Indent
	mdListItem8Indent
	mdListItem9Indent
	mdListItem10Indent
	mdListItem11Indent
	mdListItem12Indent
	mdListItem13Indent
	mdListItem14Indent
	mdListItemMaxIndent
	mdFencedCodeBlock
	mdAnonymous
)

// State bitflags
const (
	mdStateMatching         = 1 << 0
	mdStateWasSoftLineBreak = 1 << 1
	mdStateCloseBlock       = 1 << 4
)

var mdHTMLTagNamesRule1 = []string{"pre", "script", "style"}

var mdHTMLTagNamesRule7 = []string{
	"address", "article", "aside", "base", "basefont", "blockquote",
	"body", "caption", "center", "col", "colgroup", "dd",
	"details", "dialog", "dir", "div", "dl", "dt",
	"fieldset", "figcaption", "figure", "footer", "form", "frame",
	"frameset", "h1", "h2", "h3", "h4", "h5",
	"h6", "head", "header", "hr", "html", "iframe",
	"legend", "li", "link", "main", "menu", "menuitem",
	"nav", "noframes", "ol", "optgroup", "option", "p",
	"param", "section", "source", "summary", "table", "tbody",
	"td", "tfoot", "th", "thead", "title", "tr",
	"track", "ul",
}

// Tokens that can interrupt a paragraph.
var mdParagraphInterruptSymbols = []bool{
	false, false, false, false, true, false, // LINE_ENDING..INDENTED_CHUNK_START
	true, true, true, true, true, true, // ATX_H1..ATX_H6
	true, true, true, // SETEXT_H1, SETEXT_H2, THEMATIC
	true, true, true, true, true, // LIST_MARKER (interrupting)
	false, false, false, false, false, // LIST_MARKER (dont_interrupt)
	true, true, true, false, false, // FENCED_CODE start/BLANK/end
	true, false, true, true, true, true, true, false, // HTML blocks
	false, false, false, false, false, false, false, // CLOSE_BLOCK..PLUS_META
	true, false, // PIPE_TABLE_START, PIPE_TABLE_LINE_ENDING
}

type mdState struct {
	openBlocks                     []mdBlock
	state                          uint8
	matched                        uint8
	indentation                    uint8
	column                         uint8
	fencedCodeBlockDelimiterLength uint8
	simulate                       bool
}

func mdListItemIndentation(b mdBlock) uint8 {
	return uint8(b-mdListItem) + 2
}

// MarkdownExternalScanner handles block-level markdown parsing.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type MarkdownExternalScanner struct {
	symbols         [mdTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers markdown's external symbols.
func (MarkdownExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := MarkdownExternalScanner{symbols: mdDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, mdExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s MarkdownExternalScanner) symbolTable() *[mdTokenCount]gotreesitter.Symbol {
	if s.symbols == ([mdTokenCount]gotreesitter.Symbol{}) {
		return &mdDefaultSymTable
	}
	return &s.symbols
}

func (MarkdownExternalScanner) SupportsIncrementalReuse() bool { return true }

func (MarkdownExternalScanner) SupportsIncrementalReuseFromErrorTree() bool { return false }

func (MarkdownExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

func (MarkdownExternalScanner) Create() any {
	return &mdState{}
}
func (MarkdownExternalScanner) Destroy(payload any) {}

func (MarkdownExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*mdState)
	size := 0
	if 5+len(s.openBlocks) > len(buf) {
		return 0
	}
	buf[size] = s.state
	size++
	buf[size] = s.matched
	size++
	buf[size] = s.indentation
	size++
	buf[size] = s.column
	size++
	buf[size] = s.fencedCodeBlockDelimiterLength
	size++
	for _, b := range s.openBlocks {
		buf[size] = byte(b)
		size++
	}
	return size
}

func (MarkdownExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*mdState)
	s.openBlocks = s.openBlocks[:0]
	s.state = 0
	s.matched = 0
	s.indentation = 0
	s.column = 0
	s.fencedCodeBlockDelimiterLength = 0
	if len(buf) == 0 {
		return
	}
	size := 0
	s.state = buf[size]
	size++
	s.matched = buf[size]
	size++
	s.indentation = buf[size]
	size++
	s.column = buf[size]
	size++
	s.fencedCodeBlockDelimiterLength = buf[size]
	size++
	for ; size < len(buf); size++ {
		s.openBlocks = append(s.openBlocks, mdBlock(buf[size]))
	}
}

func (s MarkdownExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*mdState)
	state.simulate = false
	if len(s.externalToToken) > 0 {
		var semanticValid [mdTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < mdTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	return mdScan(state, lexer, validSymbols, s.symbolTable())
}

// mdAdvance advances the lexer and tracks columns (tab stops of 4).
func mdAdvance(s *mdState, lexer *gotreesitter.ExternalLexer) uint8 {
	size := uint8(1)
	if lexer.Lookahead() == '\t' {
		size = 4 - s.column
		s.column = 0
	} else {
		s.column = (s.column + 1) % 4
	}
	lexer.Advance(false)
	return size
}

func mdMarkEnd(s *mdState, lexer *gotreesitter.ExternalLexer) {
	if !s.simulate {
		lexer.MarkEnd()
	}
}

func mdPushBlock(s *mdState, b mdBlock) {
	if !s.simulate {
		s.openBlocks = append(s.openBlocks, b)
	}
}

func mdPopBlock(s *mdState) mdBlock {
	if len(s.openBlocks) == 0 {
		return mdAnonymous
	}
	b := s.openBlocks[len(s.openBlocks)-1]
	s.openBlocks = s.openBlocks[:len(s.openBlocks)-1]
	return b
}

func mdIsListItem(b mdBlock) bool {
	return b >= mdListItem && b <= mdListItemMaxIndent
}

func mdMatch(s *mdState, lexer *gotreesitter.ExternalLexer, block mdBlock) bool {
	switch {
	case block == mdIndentedCodeBlock:
		for s.indentation < 4 {
			if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				s.indentation += mdAdvance(s, lexer)
			} else {
				break
			}
		}
		if s.indentation >= 4 && lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' {
			s.indentation -= 4
			return true
		}
	case mdIsListItem(block):
		target := mdListItemIndentation(block)
		for s.indentation < target {
			if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				s.indentation += mdAdvance(s, lexer)
			} else {
				break
			}
		}
		if s.indentation >= target {
			s.indentation -= target
			return true
		}
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
			s.indentation = 0
			return true
		}
	case block == mdBlockQuote:
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			s.indentation += mdAdvance(s, lexer)
		}
		if lexer.Lookahead() == '>' {
			mdAdvance(s, lexer)
			s.indentation = 0
			if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				s.indentation += mdAdvance(s, lexer) - 1
			}
			return true
		}
	case block == mdFencedCodeBlock || block == mdAnonymous:
		return true
	}
	return false
}

func mdScan(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	if isValid(mdTokTriggerError) {
		lexer.SetResultSymbol(syms[mdTokError])
		return true
	}

	if isValid(mdTokCloseBlock) {
		s.state |= mdStateCloseBlock
		lexer.SetResultSymbol(syms[mdTokCloseBlock])
		return true
	}

	if lexer.Lookahead() == 0 {
		if isValid(mdTokEOF) {
			lexer.SetResultSymbol(syms[mdTokEOF])
			return true
		}
		if len(s.openBlocks) > 0 {
			lexer.SetResultSymbol(syms[mdTokBlockClose])
			if !s.simulate {
				mdPopBlock(s)
			}
			return true
		}
		return false
	}

	if (s.state & mdStateMatching) == 0 {
		// Parse preceding whitespace
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			s.indentation += mdAdvance(s, lexer)
		}

		if isValid(mdTokIndentedChunkStart) && !isValid(mdTokNoIndentedChunk) {
			if s.indentation >= 4 && lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' {
				lexer.SetResultSymbol(syms[mdTokIndentedChunkStart])
				mdPushBlock(s, mdIndentedCodeBlock)
				s.indentation -= 4
				return true
			}
		}

		switch lexer.Lookahead() {
		case '\r', '\n':
			if isValid(mdTokBlankLineStart) {
				lexer.SetResultSymbol(syms[mdTokBlankLineStart])
				return true
			}
		case '`':
			return mdParseFencedCodeBlock(s, '`', lexer, validSymbols, syms)
		case '~':
			return mdParseFencedCodeBlock(s, '~', lexer, validSymbols, syms)
		case '*':
			return mdParseStar(s, lexer, validSymbols, syms)
		case '_':
			return mdParseThematicBreakUnderscore(s, lexer, validSymbols, syms)
		case '>':
			return mdParseBlockQuote(s, lexer, validSymbols, syms)
		case '#':
			return mdParseAtxHeading(s, lexer, validSymbols, syms)
		case '=':
			return mdParseSetextUnderline(s, lexer, validSymbols, syms)
		case '+':
			return mdParsePlus(s, lexer, validSymbols, syms)
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return mdParseOrderedListMarker(s, lexer, validSymbols, syms)
		case '-':
			return mdParseMinus(s, lexer, validSymbols, syms)
		case '<':
			return mdParseHTMLBlock(s, lexer, validSymbols, syms)
		}

		if lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' && isValid(mdTokPipeTableStart) {
			return mdParsePipeTable(s, lexer, validSymbols, syms)
		}
	} else {
		// Matching state
		partialSuccess := false
		for s.matched < uint8(len(s.openBlocks)) {
			if s.matched == uint8(len(s.openBlocks))-1 && (s.state&mdStateCloseBlock) != 0 {
				if !partialSuccess {
					s.state &^= mdStateCloseBlock
				}
				break
			}
			if mdMatch(s, lexer, s.openBlocks[s.matched]) {
				partialSuccess = true
				s.matched++
			} else {
				if (s.state & mdStateWasSoftLineBreak) != 0 {
					s.state &^= mdStateMatching
				}
				break
			}
		}
		if partialSuccess {
			if s.matched == uint8(len(s.openBlocks)) {
				s.state &^= mdStateMatching
			}
			lexer.SetResultSymbol(syms[mdTokBlockContinuation])
			return true
		}
		if (s.state & mdStateWasSoftLineBreak) == 0 {
			lexer.SetResultSymbol(syms[mdTokBlockClose])
			mdPopBlock(s)
			if s.matched == uint8(len(s.openBlocks)) {
				s.state &^= mdStateMatching
			}
			return true
		}
	}

	// Line break handling
	if (isValid(mdTokLineEnding) || isValid(mdTokSoftLineEnding) || isValid(mdTokPipeTableLineEnding)) &&
		(lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r') {
		if lexer.Lookahead() == '\r' {
			mdAdvance(s, lexer)
			if lexer.Lookahead() == '\n' {
				mdAdvance(s, lexer)
			}
		} else {
			mdAdvance(s, lexer)
		}
		s.indentation = 0
		s.column = 0

		if (s.state&mdStateCloseBlock) == 0 &&
			(isValid(mdTokSoftLineEnding) || isValid(mdTokPipeTableLineEnding)) {
			lexer.MarkEnd()
			for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				s.indentation += mdAdvance(s, lexer)
			}
			s.simulate = true
			matchedTemp := s.matched
			s.matched = 0
			oneWillBeMatched := false
			for s.matched < uint8(len(s.openBlocks)) {
				if mdMatch(s, lexer, s.openBlocks[s.matched]) {
					s.matched++
					oneWillBeMatched = true
				} else {
					break
				}
			}
			allWillBeMatched := s.matched == uint8(len(s.openBlocks))
			if lexer.Lookahead() != 0 && !mdScan(s, lexer, mdParagraphInterruptSymbols, syms) {
				s.matched = matchedTemp
				s.matched = 0
				s.indentation = 0
				s.column = 0
				if oneWillBeMatched {
					s.state |= mdStateMatching
				} else {
					s.state &^= mdStateMatching
				}
				if isValid(mdTokPipeTableLineEnding) {
					if allWillBeMatched {
						lexer.SetResultSymbol(syms[mdTokPipeTableLineEnding])
						return true
					}
				} else {
					lexer.SetResultSymbol(syms[mdTokSoftLineEnding])
					s.state |= mdStateWasSoftLineBreak
					return true
				}
			} else {
				s.matched = matchedTemp
			}
			s.indentation = 0
			s.column = 0
		}

		if isValid(mdTokLineEnding) {
			s.matched = 0
			if len(s.openBlocks) > 0 {
				s.state |= mdStateMatching
			} else {
				s.state &^= mdStateMatching
			}
			s.state &^= mdStateWasSoftLineBreak
			lexer.SetResultSymbol(syms[mdTokLineEnding])
			return true
		}
	}
	return false
}

func mdParseFencedCodeBlock(s *mdState, delimiter rune, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	isValid := func(idx int) bool { return idx < len(validSymbols) && validSymbols[idx] }

	level := uint8(0)
	for lexer.Lookahead() == delimiter {
		mdAdvance(s, lexer)
		level++
	}
	mdMarkEnd(s, lexer)

	endTok := mdTokFencedCodeBlockEndBacktick
	startTok := mdTokFencedCodeBlockStartBacktick
	if delimiter == '~' {
		endTok = mdTokFencedCodeBlockEndTilde
		startTok = mdTokFencedCodeBlockStartTilde
	}
	endSym := syms[endTok]
	startSym := syms[startTok]

	if isValid(endTok) && s.indentation < 4 && level >= s.fencedCodeBlockDelimiterLength {
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			mdAdvance(s, lexer)
		}
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
			s.fencedCodeBlockDelimiterLength = 0
			lexer.SetResultSymbol(endSym)
			return true
		}
	}

	if isValid(startTok) && level >= 3 {
		infoHasBacktick := false
		if delimiter == '`' {
			for lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' && lexer.Lookahead() != 0 {
				if lexer.Lookahead() == '`' {
					infoHasBacktick = true
					break
				}
				mdAdvance(s, lexer)
			}
		}
		if !infoHasBacktick {
			lexer.SetResultSymbol(startSym)
			mdPushBlock(s, mdFencedCodeBlock)
			s.fencedCodeBlockDelimiterLength = level
			s.indentation = 0
			return true
		}
	}
	return false
}

func mdParseStar(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	isValid := func(idx int) bool { return idx < len(validSymbols) && validSymbols[idx] }

	mdAdvance(s, lexer)
	mdMarkEnd(s, lexer)
	starCount := uint16(1)
	extraIndent := uint8(0)
	for {
		if lexer.Lookahead() == '*' {
			if starCount == 1 && extraIndent >= 1 && isValid(mdTokListMarkerStar) {
				mdMarkEnd(s, lexer)
			}
			starCount++
			mdAdvance(s, lexer)
		} else if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			if starCount == 1 {
				extraIndent += mdAdvance(s, lexer)
			} else {
				mdAdvance(s, lexer)
			}
		} else {
			break
		}
	}
	lineEnd := lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r'
	dontInterrupt := false
	if starCount == 1 && lineEnd {
		extraIndent = 1
		dontInterrupt = s.matched == uint8(len(s.openBlocks))
	}
	thematicBreak := starCount >= 3 && lineEnd
	listMarkerStar := starCount >= 1 && extraIndent >= 1

	if isValid(mdTokThematicBreak) && thematicBreak && s.indentation < 4 {
		lexer.SetResultSymbol(syms[mdTokThematicBreak])
		mdMarkEnd(s, lexer)
		s.indentation = 0
		return true
	}
	tok := mdTokListMarkerStar
	sym := syms[mdTokListMarkerStar]
	if dontInterrupt {
		tok = mdTokListMarkerStarDontInterrupt
		sym = syms[mdTokListMarkerStarDontInterrupt]
	}
	if isValid(tok) && listMarkerStar {
		if starCount == 1 {
			mdMarkEnd(s, lexer)
		}
		extraIndent--
		if extraIndent <= 3 {
			extraIndent += s.indentation
			s.indentation = 0
		} else {
			tmp := s.indentation
			s.indentation = extraIndent
			extraIndent = tmp
		}
		mdPushBlock(s, mdBlock(uint8(mdListItem)+extraIndent))
		lexer.SetResultSymbol(sym)
		return true
	}
	return false
}

func mdParseThematicBreakUnderscore(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	isValid := func(idx int) bool { return idx < len(validSymbols) && validSymbols[idx] }

	mdAdvance(s, lexer)
	mdMarkEnd(s, lexer)
	count := uint16(1)
	for {
		if lexer.Lookahead() == '_' {
			count++
			mdAdvance(s, lexer)
		} else if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			mdAdvance(s, lexer)
		} else {
			break
		}
	}
	lineEnd := lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r'
	if count >= 3 && lineEnd && isValid(mdTokThematicBreak) {
		lexer.SetResultSymbol(syms[mdTokThematicBreak])
		mdMarkEnd(s, lexer)
		s.indentation = 0
		return true
	}
	return false
}

func mdParseBlockQuote(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	if validSymbols[mdTokBlockQuoteStart] {
		mdAdvance(s, lexer)
		s.indentation = 0
		if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			s.indentation += mdAdvance(s, lexer) - 1
		}
		lexer.SetResultSymbol(syms[mdTokBlockQuoteStart])
		mdPushBlock(s, mdBlockQuote)
		return true
	}
	return false
}

func mdParseAtxHeading(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	if validSymbols[mdTokAtxH1Marker] && s.indentation <= 3 {
		mdMarkEnd(s, lexer)
		level := uint16(0)
		for lexer.Lookahead() == '#' && level <= 6 {
			mdAdvance(s, lexer)
			level++
		}
		if level <= 6 && (lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' ||
			lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r') {
			lexer.SetResultSymbol(syms[mdTokAtxH1Marker+int(level)-1])
			s.indentation = 0
			mdMarkEnd(s, lexer)
			return true
		}
	}
	return false
}

func mdParseSetextUnderline(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	if validSymbols[mdTokSetextH1Underline] && s.matched == uint8(len(s.openBlocks)) {
		mdMarkEnd(s, lexer)
		for lexer.Lookahead() == '=' {
			mdAdvance(s, lexer)
		}
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			mdAdvance(s, lexer)
		}
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
			lexer.SetResultSymbol(syms[mdTokSetextH1Underline])
			mdMarkEnd(s, lexer)
			return true
		}
	}
	return false
}

func mdParsePlus(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	isValid := func(idx int) bool { return idx < len(validSymbols) && validSymbols[idx] }

	if s.indentation <= 3 && (isValid(mdTokListMarkerPlus) || isValid(mdTokListMarkerPlusDontInterrupt) || isValid(mdTokPlusMetadata)) {
		mdAdvance(s, lexer)
		if isValid(mdTokPlusMetadata) && lexer.Lookahead() == '+' {
			mdAdvance(s, lexer)
			if lexer.Lookahead() != '+' {
				return false
			}
			mdAdvance(s, lexer)
			for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				mdAdvance(s, lexer)
			}
			if lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' {
				return false
			}
			for {
				if lexer.Lookahead() == '\r' {
					mdAdvance(s, lexer)
					if lexer.Lookahead() == '\n' {
						mdAdvance(s, lexer)
					}
				} else {
					mdAdvance(s, lexer)
				}
				plusCount := 0
				for lexer.Lookahead() == '+' {
					plusCount++
					mdAdvance(s, lexer)
				}
				if plusCount == 3 {
					for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
						mdAdvance(s, lexer)
					}
					if lexer.Lookahead() == '\r' || lexer.Lookahead() == '\n' {
						if lexer.Lookahead() == '\r' {
							mdAdvance(s, lexer)
							if lexer.Lookahead() == '\n' {
								mdAdvance(s, lexer)
							}
						} else {
							mdAdvance(s, lexer)
						}
						mdMarkEnd(s, lexer)
						lexer.SetResultSymbol(syms[mdTokPlusMetadata])
						return true
					}
				}
				for lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' && lexer.Lookahead() != 0 {
					mdAdvance(s, lexer)
				}
				if lexer.Lookahead() == 0 {
					break
				}
			}
		} else {
			extraIndent := uint8(0)
			for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				extraIndent += mdAdvance(s, lexer)
			}
			dontInterrupt := false
			if lexer.Lookahead() == '\r' || lexer.Lookahead() == '\n' {
				extraIndent = 1
				dontInterrupt = true
			}
			dontInterrupt = dontInterrupt && s.matched == uint8(len(s.openBlocks))
			tok := mdTokListMarkerPlus
			sym := syms[mdTokListMarkerPlus]
			if dontInterrupt {
				tok = mdTokListMarkerPlusDontInterrupt
				sym = syms[mdTokListMarkerPlusDontInterrupt]
			}
			if extraIndent >= 1 && isValid(tok) {
				lexer.SetResultSymbol(sym)
				extraIndent--
				if extraIndent <= 3 {
					extraIndent += s.indentation
					s.indentation = 0
				} else {
					tmp := s.indentation
					s.indentation = extraIndent
					extraIndent = tmp
				}
				mdPushBlock(s, mdBlock(uint8(mdListItem)+extraIndent))
				return true
			}
		}
	}
	return false
}

func mdParseOrderedListMarker(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	isValid := func(idx int) bool { return idx < len(validSymbols) && validSymbols[idx] }

	if s.indentation <= 3 && (isValid(mdTokListMarkerParenthesis) || isValid(mdTokListMarkerDot) ||
		isValid(mdTokListMarkerParenthesisDontInterrupt) || isValid(mdTokListMarkerDotDontInterrupt)) {
		digits := uint16(1)
		dontInterrupt := !mdIsDigit(lexer.Lookahead())
		mdAdvance(s, lexer)
		for mdIsDigit(lexer.Lookahead()) {
			dontInterrupt = true
			digits++
			mdAdvance(s, lexer)
		}
		if digits >= 1 && digits <= 9 {
			isDot := false
			isParen := false
			if lexer.Lookahead() == '.' {
				mdAdvance(s, lexer)
				isDot = true
			} else if lexer.Lookahead() == ')' {
				mdAdvance(s, lexer)
				isParen = true
			}
			if isDot || isParen {
				extraIndent := uint8(0)
				for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
					extraIndent += mdAdvance(s, lexer)
				}
				lineEnd := lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r'
				if lineEnd {
					extraIndent = 1
					dontInterrupt = true
				}
				dontInterrupt = dontInterrupt && s.matched == uint8(len(s.openBlocks))

				var tok int
				var sym gotreesitter.Symbol
				if isDot {
					tok = mdTokListMarkerDot
					sym = syms[mdTokListMarkerDot]
					if dontInterrupt {
						tok = mdTokListMarkerDotDontInterrupt
						sym = syms[mdTokListMarkerDotDontInterrupt]
					}
				} else {
					tok = mdTokListMarkerParenthesis
					sym = syms[mdTokListMarkerParenthesis]
					if dontInterrupt {
						tok = mdTokListMarkerParenthesisDontInterrupt
						sym = syms[mdTokListMarkerParenthesisDontInterrupt]
					}
				}

				if extraIndent >= 1 && isValid(tok) {
					lexer.SetResultSymbol(sym)
					extraIndent--
					if extraIndent <= 3 {
						extraIndent += s.indentation
						s.indentation = 0
					} else {
						tmp := s.indentation
						s.indentation = extraIndent
						extraIndent = tmp
					}
					mdPushBlock(s, mdBlock(uint8(mdListItem)+extraIndent+uint8(digits)))
					return true
				}
			}
		}
	}
	return false
}

func mdParseMinus(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	isValid := func(idx int) bool { return idx < len(validSymbols) && validSymbols[idx] }

	if s.indentation <= 3 && (isValid(mdTokListMarkerMinus) || isValid(mdTokListMarkerMinusDontInterrupt) ||
		isValid(mdTokSetextH2Underline) || isValid(mdTokThematicBreak) || isValid(mdTokMinusMetadata)) {
		mdMarkEnd(s, lexer)
		wsAfterMinus := false
		minusAfterWS := false
		minusCount := uint16(0)
		extraIndent := uint8(0)

		for {
			if lexer.Lookahead() == '-' {
				if minusCount == 1 && extraIndent >= 1 {
					mdMarkEnd(s, lexer)
				}
				minusCount++
				mdAdvance(s, lexer)
				minusAfterWS = wsAfterMinus
			} else if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				if minusCount == 1 {
					extraIndent += mdAdvance(s, lexer)
				} else {
					mdAdvance(s, lexer)
				}
				wsAfterMinus = true
			} else {
				break
			}
		}
		lineEnd := lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r'
		dontInterrupt := false
		if minusCount == 1 && lineEnd {
			extraIndent = 1
			dontInterrupt = true
		}
		dontInterrupt = dontInterrupt && s.matched == uint8(len(s.openBlocks))

		thematicBreak := minusCount >= 3 && lineEnd
		underline := minusCount >= 1 && !minusAfterWS && lineEnd && s.matched == uint8(len(s.openBlocks))
		listMarkerMinus := minusCount >= 1 && extraIndent >= 1
		success := false

		if isValid(mdTokSetextH2Underline) && underline {
			lexer.SetResultSymbol(syms[mdTokSetextH2Underline])
			mdMarkEnd(s, lexer)
			s.indentation = 0
			success = true
		} else if isValid(mdTokThematicBreak) && thematicBreak {
			lexer.SetResultSymbol(syms[mdTokThematicBreak])
			mdMarkEnd(s, lexer)
			s.indentation = 0
			success = true
		} else {
			tok := mdTokListMarkerMinus
			sym := syms[mdTokListMarkerMinus]
			if dontInterrupt {
				tok = mdTokListMarkerMinusDontInterrupt
				sym = syms[mdTokListMarkerMinusDontInterrupt]
			}
			if isValid(tok) && listMarkerMinus {
				if minusCount == 1 {
					mdMarkEnd(s, lexer)
				}
				extraIndent--
				if extraIndent <= 3 {
					extraIndent += s.indentation
					s.indentation = 0
				} else {
					tmp := s.indentation
					s.indentation = extraIndent
					extraIndent = tmp
				}
				mdPushBlock(s, mdBlock(uint8(mdListItem)+extraIndent))
				lexer.SetResultSymbol(sym)
				return true
			}
		}

		if minusCount == 3 && !minusAfterWS && lineEnd && isValid(mdTokMinusMetadata) {
			for {
				if lexer.Lookahead() == '\r' {
					mdAdvance(s, lexer)
					if lexer.Lookahead() == '\n' {
						mdAdvance(s, lexer)
					}
				} else {
					mdAdvance(s, lexer)
				}
				mc := uint16(0)
				for lexer.Lookahead() == '-' {
					mc++
					mdAdvance(s, lexer)
				}
				if mc == 3 {
					for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
						mdAdvance(s, lexer)
					}
					if lexer.Lookahead() == '\r' || lexer.Lookahead() == '\n' {
						if lexer.Lookahead() == '\r' {
							mdAdvance(s, lexer)
							if lexer.Lookahead() == '\n' {
								mdAdvance(s, lexer)
							}
						} else {
							mdAdvance(s, lexer)
						}
						mdMarkEnd(s, lexer)
						lexer.SetResultSymbol(syms[mdTokMinusMetadata])
						return true
					}
				}
				for lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' && lexer.Lookahead() != 0 {
					mdAdvance(s, lexer)
				}
				if lexer.Lookahead() == 0 {
					break
				}
			}
		}
		if success {
			return true
		}
	}
	return false
}

func mdParseHTMLBlock(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	isValid := func(idx int) bool { return idx < len(validSymbols) && validSymbols[idx] }

	if !(isValid(mdTokHTMLBlock1Start) || isValid(mdTokHTMLBlock1End) ||
		isValid(mdTokHTMLBlock2Start) || isValid(mdTokHTMLBlock3Start) ||
		isValid(mdTokHTMLBlock4Start) || isValid(mdTokHTMLBlock5Start) ||
		isValid(mdTokHTMLBlock6Start) || isValid(mdTokHTMLBlock7Start)) {
		return false
	}
	mdAdvance(s, lexer)

	if lexer.Lookahead() == '?' && isValid(mdTokHTMLBlock3Start) {
		mdAdvance(s, lexer)
		lexer.SetResultSymbol(syms[mdTokHTMLBlock3Start])
		mdPushBlock(s, mdAnonymous)
		return true
	}
	if lexer.Lookahead() == '!' {
		mdAdvance(s, lexer)
		if lexer.Lookahead() == '-' {
			mdAdvance(s, lexer)
			if lexer.Lookahead() == '-' && isValid(mdTokHTMLBlock2Start) {
				mdAdvance(s, lexer)
				lexer.SetResultSymbol(syms[mdTokHTMLBlock2Start])
				mdPushBlock(s, mdAnonymous)
				return true
			}
		} else if lexer.Lookahead() >= 'A' && lexer.Lookahead() <= 'Z' && isValid(mdTokHTMLBlock4Start) {
			mdAdvance(s, lexer)
			lexer.SetResultSymbol(syms[mdTokHTMLBlock4Start])
			mdPushBlock(s, mdAnonymous)
			return true
		} else if lexer.Lookahead() == '[' {
			mdAdvance(s, lexer)
			if lexer.Lookahead() == 'C' {
				mdAdvance(s, lexer)
				if lexer.Lookahead() == 'D' {
					mdAdvance(s, lexer)
					if lexer.Lookahead() == 'A' {
						mdAdvance(s, lexer)
						if lexer.Lookahead() == 'T' {
							mdAdvance(s, lexer)
							if lexer.Lookahead() == 'A' {
								mdAdvance(s, lexer)
								if lexer.Lookahead() == '[' && isValid(mdTokHTMLBlock5Start) {
									mdAdvance(s, lexer)
									lexer.SetResultSymbol(syms[mdTokHTMLBlock5Start])
									mdPushBlock(s, mdAnonymous)
									return true
								}
							}
						}
					}
				}
			}
		}
	}

	startingSlash := lexer.Lookahead() == '/'
	if startingSlash {
		mdAdvance(s, lexer)
	}

	var nameBuf [11]byte
	nameLen := 0
	for unicode.IsLetter(lexer.Lookahead()) {
		if nameLen < 10 {
			nameBuf[nameLen] = byte(unicode.ToLower(lexer.Lookahead()))
			nameLen++
		} else {
			nameLen = 12
		}
		mdAdvance(s, lexer)
	}
	if nameLen == 0 {
		return false
	}

	tagClosed := false
	if nameLen < 11 {
		name := string(nameBuf[:nameLen])
		nextValid := lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' ||
			lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || lexer.Lookahead() == '>'

		if nextValid {
			for _, tag := range mdHTMLTagNamesRule1 {
				if name == tag {
					if startingSlash {
						if isValid(mdTokHTMLBlock1End) {
							lexer.SetResultSymbol(syms[mdTokHTMLBlock1End])
							return true
						}
					} else if isValid(mdTokHTMLBlock1Start) {
						lexer.SetResultSymbol(syms[mdTokHTMLBlock1Start])
						mdPushBlock(s, mdAnonymous)
						return true
					}
				}
			}
		}
		if !nextValid && lexer.Lookahead() == '/' {
			mdAdvance(s, lexer)
			if lexer.Lookahead() == '>' {
				mdAdvance(s, lexer)
				tagClosed = true
			}
		}
		if nextValid || tagClosed {
			for _, tag := range mdHTMLTagNamesRule7 {
				if name == tag && isValid(mdTokHTMLBlock6Start) {
					lexer.SetResultSymbol(syms[mdTokHTMLBlock6Start])
					mdPushBlock(s, mdAnonymous)
					return true
				}
			}
		}
	}

	if !isValid(mdTokHTMLBlock7Start) {
		return false
	}

	if !tagClosed {
		for unicode.IsLetter(lexer.Lookahead()) || unicode.IsDigit(lexer.Lookahead()) || lexer.Lookahead() == '-' {
			mdAdvance(s, lexer)
		}
		if !startingSlash {
			hadWS := false
			for {
				for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
					hadWS = true
					mdAdvance(s, lexer)
				}
				if lexer.Lookahead() == '/' {
					mdAdvance(s, lexer)
					break
				}
				if lexer.Lookahead() == '>' {
					break
				}
				if !hadWS {
					return false
				}
				if !unicode.IsLetter(lexer.Lookahead()) && lexer.Lookahead() != '_' && lexer.Lookahead() != ':' {
					return false
				}
				hadWS = false
				mdAdvance(s, lexer)
				for unicode.IsLetter(lexer.Lookahead()) || unicode.IsDigit(lexer.Lookahead()) ||
					lexer.Lookahead() == '_' || lexer.Lookahead() == '.' ||
					lexer.Lookahead() == ':' || lexer.Lookahead() == '-' {
					mdAdvance(s, lexer)
				}
				for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
					hadWS = true
					mdAdvance(s, lexer)
				}
				if lexer.Lookahead() == '=' {
					mdAdvance(s, lexer)
					hadWS = false
					for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
						mdAdvance(s, lexer)
					}
					if lexer.Lookahead() == '\'' || lexer.Lookahead() == '"' {
						delim := lexer.Lookahead()
						mdAdvance(s, lexer)
						for lexer.Lookahead() != delim && lexer.Lookahead() != '\n' &&
							lexer.Lookahead() != '\r' && lexer.Lookahead() != 0 {
							mdAdvance(s, lexer)
						}
						if lexer.Lookahead() != delim {
							return false
						}
						mdAdvance(s, lexer)
					} else {
						hadOne := false
						for lexer.Lookahead() != ' ' && lexer.Lookahead() != '\t' &&
							lexer.Lookahead() != '"' && lexer.Lookahead() != '\'' &&
							lexer.Lookahead() != '=' && lexer.Lookahead() != '<' &&
							lexer.Lookahead() != '>' && lexer.Lookahead() != '`' &&
							lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' &&
							lexer.Lookahead() != 0 {
							mdAdvance(s, lexer)
							hadOne = true
						}
						if !hadOne {
							return false
						}
					}
				}
			}
		} else {
			for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
				mdAdvance(s, lexer)
			}
		}
		if lexer.Lookahead() != '>' {
			return false
		}
		mdAdvance(s, lexer)
	}
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		mdAdvance(s, lexer)
	}
	if lexer.Lookahead() == '\r' || lexer.Lookahead() == '\n' {
		lexer.SetResultSymbol(syms[mdTokHTMLBlock7Start])
		mdPushBlock(s, mdAnonymous)
		return true
	}
	return false
}

func mdParsePipeTable(s *mdState, lexer *gotreesitter.ExternalLexer, _ []bool, syms *[mdTokenCount]gotreesitter.Symbol) bool {
	mdMarkEnd(s, lexer)
	cellCount := uint16(0)
	startingPipe := false
	endingPipe := false
	if lexer.Lookahead() == '|' {
		startingPipe = true
		mdAdvance(s, lexer)
	}
	for lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '|' {
			cellCount++
			endingPipe = true
			mdAdvance(s, lexer)
		} else {
			if lexer.Lookahead() != ' ' && lexer.Lookahead() != '\t' {
				endingPipe = false
			}
			if lexer.Lookahead() == '\\' {
				mdAdvance(s, lexer)
				if mdIsPunctuation(lexer.Lookahead()) {
					mdAdvance(s, lexer)
				}
			} else {
				mdAdvance(s, lexer)
			}
		}
	}
	if cellCount == 0 && !(startingPipe && endingPipe) {
		return false
	}
	if !endingPipe {
		cellCount++
	}

	// Check delimiter row
	if lexer.Lookahead() == '\n' {
		mdAdvance(s, lexer)
	} else if lexer.Lookahead() == '\r' {
		mdAdvance(s, lexer)
		if lexer.Lookahead() == '\n' {
			mdAdvance(s, lexer)
		}
	} else {
		return false
	}
	s.indentation = 0
	s.column = 0
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
		s.indentation += mdAdvance(s, lexer)
	}
	s.simulate = true
	matchedTemp := uint8(0)
	for matchedTemp < uint8(len(s.openBlocks)) {
		if mdMatch(s, lexer, s.openBlocks[matchedTemp]) {
			matchedTemp++
		} else {
			return false
		}
	}

	delimCellCount := uint16(0)
	if lexer.Lookahead() == '|' {
		mdAdvance(s, lexer)
	}
	for {
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			mdAdvance(s, lexer)
		}
		if lexer.Lookahead() == '|' {
			delimCellCount++
			mdAdvance(s, lexer)
			continue
		}
		if lexer.Lookahead() == ':' {
			mdAdvance(s, lexer)
			if lexer.Lookahead() != '-' {
				return false
			}
		}
		hadOneMinus := false
		for lexer.Lookahead() == '-' {
			hadOneMinus = true
			mdAdvance(s, lexer)
		}
		if hadOneMinus {
			delimCellCount++
		}
		if lexer.Lookahead() == ':' {
			if !hadOneMinus {
				return false
			}
			mdAdvance(s, lexer)
		}
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			mdAdvance(s, lexer)
		}
		if lexer.Lookahead() == '|' {
			if !hadOneMinus {
				delimCellCount++
			}
			mdAdvance(s, lexer)
			continue
		}
		if lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' {
			return false
		}
		break
	}
	if cellCount != delimCellCount {
		return false
	}
	lexer.SetResultSymbol(syms[mdTokPipeTableStart])
	return true
}

func mdIsDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func mdIsPunctuation(ch rune) bool {
	return (ch >= '!' && ch <= '/') || (ch >= ':' && ch <= '@') ||
		(ch >= '[' && ch <= '`') || (ch >= '{' && ch <= '~')
}

// Ensure string comparison helper is available.
var _ = strings.ToLower
