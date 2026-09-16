//go:build !grammar_subset || grammar_subset_markdown

package grammarruntime

import (
	"strings"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Markdown grammar.
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
)

const (
	mdSymLineEnding              gotreesitter.Symbol = 43
	mdSymSoftLineEnding          gotreesitter.Symbol = 44
	mdSymBlockClose              gotreesitter.Symbol = 45
	mdSymBlockContinuation       gotreesitter.Symbol = 46
	mdSymBlockQuoteStart         gotreesitter.Symbol = 47
	mdSymIndentedChunkStart      gotreesitter.Symbol = 48
	mdSymAtxH1Marker             gotreesitter.Symbol = 49
	mdSymSetextH1Underline       gotreesitter.Symbol = 55
	mdSymSetextH2Underline       gotreesitter.Symbol = 56
	mdSymThematicBreak           gotreesitter.Symbol = 57
	mdSymListMarkerMinus         gotreesitter.Symbol = 58
	mdSymListMarkerPlus          gotreesitter.Symbol = 59
	mdSymListMarkerStar          gotreesitter.Symbol = 60
	mdSymListMarkerParenthesis   gotreesitter.Symbol = 61
	mdSymListMarkerDot           gotreesitter.Symbol = 62
	mdSymListMarkerMinusDI       gotreesitter.Symbol = 63
	mdSymListMarkerPlusDI        gotreesitter.Symbol = 64
	mdSymListMarkerStarDI        gotreesitter.Symbol = 65
	mdSymListMarkerParenthesisDI gotreesitter.Symbol = 66
	mdSymListMarkerDotDI         gotreesitter.Symbol = 67
	mdSymFencedCodeStartBT       gotreesitter.Symbol = 68
	mdSymFencedCodeStartTilde    gotreesitter.Symbol = 69
	mdSymBlankLineStart          gotreesitter.Symbol = 70
	mdSymFencedCodeEndBT         gotreesitter.Symbol = 71
	mdSymFencedCodeEndTilde      gotreesitter.Symbol = 72
	mdSymHTMLBlock1Start         gotreesitter.Symbol = 73
	mdSymHTMLBlock1End           gotreesitter.Symbol = 74
	mdSymHTMLBlock2Start         gotreesitter.Symbol = 75
	mdSymHTMLBlock3Start         gotreesitter.Symbol = 76
	mdSymHTMLBlock4Start         gotreesitter.Symbol = 77
	mdSymHTMLBlock5Start         gotreesitter.Symbol = 78
	mdSymHTMLBlock6Start         gotreesitter.Symbol = 79
	mdSymHTMLBlock7Start         gotreesitter.Symbol = 80
	mdSymCloseBlock              gotreesitter.Symbol = 81
	mdSymError                   gotreesitter.Symbol = 83
	mdSymTriggerError            gotreesitter.Symbol = 84
	mdSymTokenEOF                gotreesitter.Symbol = 85
	mdSymMinusMetadata           gotreesitter.Symbol = 86
	mdSymPlusMetadata            gotreesitter.Symbol = 87
	mdSymPipeTableStart          gotreesitter.Symbol = 88
	mdSymPipeTableLineEnding     gotreesitter.Symbol = 89
)

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
type MarkdownExternalScanner struct{}

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

func (MarkdownExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*mdState)
	s.simulate = false
	return mdScan(s, lexer, validSymbols)
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

func mdScan(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	if isValid(mdTokTriggerError) {
		lexer.SetResultSymbol(mdSymError)
		return true
	}

	if isValid(mdTokCloseBlock) {
		s.state |= mdStateCloseBlock
		lexer.SetResultSymbol(mdSymCloseBlock)
		return true
	}

	if lexer.Lookahead() == 0 {
		if isValid(mdTokEOF) {
			lexer.SetResultSymbol(mdSymTokenEOF)
			return true
		}
		if len(s.openBlocks) > 0 {
			lexer.SetResultSymbol(mdSymBlockClose)
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
				lexer.SetResultSymbol(mdSymIndentedChunkStart)
				mdPushBlock(s, mdIndentedCodeBlock)
				s.indentation -= 4
				return true
			}
		}

		switch lexer.Lookahead() {
		case '\r', '\n':
			if isValid(mdTokBlankLineStart) {
				lexer.SetResultSymbol(mdSymBlankLineStart)
				return true
			}
		case '`':
			return mdParseFencedCodeBlock(s, '`', lexer, validSymbols)
		case '~':
			return mdParseFencedCodeBlock(s, '~', lexer, validSymbols)
		case '*':
			return mdParseStar(s, lexer, validSymbols)
		case '_':
			return mdParseThematicBreakUnderscore(s, lexer, validSymbols)
		case '>':
			return mdParseBlockQuote(s, lexer, validSymbols)
		case '#':
			return mdParseAtxHeading(s, lexer, validSymbols)
		case '=':
			return mdParseSetextUnderline(s, lexer, validSymbols)
		case '+':
			return mdParsePlus(s, lexer, validSymbols)
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return mdParseOrderedListMarker(s, lexer, validSymbols)
		case '-':
			return mdParseMinus(s, lexer, validSymbols)
		case '<':
			return mdParseHTMLBlock(s, lexer, validSymbols)
		}

		if lexer.Lookahead() != '\r' && lexer.Lookahead() != '\n' && isValid(mdTokPipeTableStart) {
			return mdParsePipeTable(s, lexer, validSymbols)
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
			lexer.SetResultSymbol(mdSymBlockContinuation)
			return true
		}
		if (s.state & mdStateWasSoftLineBreak) == 0 {
			lexer.SetResultSymbol(mdSymBlockClose)
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
			if lexer.Lookahead() != 0 && !mdScan(s, lexer, mdParagraphInterruptSymbols) {
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
						lexer.SetResultSymbol(mdSymPipeTableLineEnding)
						return true
					}
				} else {
					lexer.SetResultSymbol(mdSymSoftLineEnding)
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
			lexer.SetResultSymbol(mdSymLineEnding)
			return true
		}
	}
	return false
}

func mdParseFencedCodeBlock(s *mdState, delimiter rune, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	isValid := func(idx int) bool { return idx < len(validSymbols) && validSymbols[idx] }

	level := uint8(0)
	for lexer.Lookahead() == delimiter {
		mdAdvance(s, lexer)
		level++
	}
	mdMarkEnd(s, lexer)

	endTok := mdTokFencedCodeBlockEndBacktick
	startTok := mdTokFencedCodeBlockStartBacktick
	endSym := mdSymFencedCodeEndBT
	startSym := mdSymFencedCodeStartBT
	if delimiter == '~' {
		endTok = mdTokFencedCodeBlockEndTilde
		startTok = mdTokFencedCodeBlockStartTilde
		endSym = mdSymFencedCodeEndTilde
		startSym = mdSymFencedCodeStartTilde
	}

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

func mdParseStar(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
		lexer.SetResultSymbol(mdSymThematicBreak)
		mdMarkEnd(s, lexer)
		s.indentation = 0
		return true
	}
	tok := mdTokListMarkerStar
	sym := mdSymListMarkerStar
	if dontInterrupt {
		tok = mdTokListMarkerStarDontInterrupt
		sym = mdSymListMarkerStarDI
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

func mdParseThematicBreakUnderscore(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
		lexer.SetResultSymbol(mdSymThematicBreak)
		mdMarkEnd(s, lexer)
		s.indentation = 0
		return true
	}
	return false
}

func mdParseBlockQuote(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if validSymbols[mdTokBlockQuoteStart] {
		mdAdvance(s, lexer)
		s.indentation = 0
		if lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			s.indentation += mdAdvance(s, lexer) - 1
		}
		lexer.SetResultSymbol(mdSymBlockQuoteStart)
		mdPushBlock(s, mdBlockQuote)
		return true
	}
	return false
}

func mdParseAtxHeading(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if validSymbols[mdTokAtxH1Marker] && s.indentation <= 3 {
		mdMarkEnd(s, lexer)
		level := uint16(0)
		for lexer.Lookahead() == '#' && level <= 6 {
			mdAdvance(s, lexer)
			level++
		}
		if level <= 6 && (lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' ||
			lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r') {
			lexer.SetResultSymbol(gotreesitter.Symbol(uint16(mdSymAtxH1Marker) + level - 1))
			s.indentation = 0
			mdMarkEnd(s, lexer)
			return true
		}
	}
	return false
}

func mdParseSetextUnderline(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if validSymbols[mdTokSetextH1Underline] && s.matched == uint8(len(s.openBlocks)) {
		mdMarkEnd(s, lexer)
		for lexer.Lookahead() == '=' {
			mdAdvance(s, lexer)
		}
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			mdAdvance(s, lexer)
		}
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
			lexer.SetResultSymbol(mdSymSetextH1Underline)
			mdMarkEnd(s, lexer)
			return true
		}
	}
	return false
}

func mdParsePlus(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
						lexer.SetResultSymbol(mdSymPlusMetadata)
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
			sym := mdSymListMarkerPlus
			if dontInterrupt {
				tok = mdTokListMarkerPlusDontInterrupt
				sym = mdSymListMarkerPlusDI
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

func mdParseOrderedListMarker(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
					sym = mdSymListMarkerDot
					if dontInterrupt {
						tok = mdTokListMarkerDotDontInterrupt
						sym = mdSymListMarkerDotDI
					}
				} else {
					tok = mdTokListMarkerParenthesis
					sym = mdSymListMarkerParenthesis
					if dontInterrupt {
						tok = mdTokListMarkerParenthesisDontInterrupt
						sym = mdSymListMarkerParenthesisDI
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

func mdParseMinus(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
			lexer.SetResultSymbol(mdSymSetextH2Underline)
			mdMarkEnd(s, lexer)
			s.indentation = 0
			success = true
		} else if isValid(mdTokThematicBreak) && thematicBreak {
			lexer.SetResultSymbol(mdSymThematicBreak)
			mdMarkEnd(s, lexer)
			s.indentation = 0
			success = true
		} else {
			tok := mdTokListMarkerMinus
			sym := mdSymListMarkerMinus
			if dontInterrupt {
				tok = mdTokListMarkerMinusDontInterrupt
				sym = mdSymListMarkerMinusDI
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
						lexer.SetResultSymbol(mdSymMinusMetadata)
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

func mdParseHTMLBlock(s *mdState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
		lexer.SetResultSymbol(mdSymHTMLBlock3Start)
		mdPushBlock(s, mdAnonymous)
		return true
	}
	if lexer.Lookahead() == '!' {
		mdAdvance(s, lexer)
		if lexer.Lookahead() == '-' {
			mdAdvance(s, lexer)
			if lexer.Lookahead() == '-' && isValid(mdTokHTMLBlock2Start) {
				mdAdvance(s, lexer)
				lexer.SetResultSymbol(mdSymHTMLBlock2Start)
				mdPushBlock(s, mdAnonymous)
				return true
			}
		} else if lexer.Lookahead() >= 'A' && lexer.Lookahead() <= 'Z' && isValid(mdTokHTMLBlock4Start) {
			mdAdvance(s, lexer)
			lexer.SetResultSymbol(mdSymHTMLBlock4Start)
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
									lexer.SetResultSymbol(mdSymHTMLBlock5Start)
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
							lexer.SetResultSymbol(mdSymHTMLBlock1End)
							return true
						}
					} else if isValid(mdTokHTMLBlock1Start) {
						lexer.SetResultSymbol(mdSymHTMLBlock1Start)
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
					lexer.SetResultSymbol(mdSymHTMLBlock6Start)
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
		lexer.SetResultSymbol(mdSymHTMLBlock7Start)
		mdPushBlock(s, mdAnonymous)
		return true
	}
	return false
}

func mdParsePipeTable(s *mdState, lexer *gotreesitter.ExternalLexer, _ []bool) bool {
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
	lexer.SetResultSymbol(mdSymPipeTableStart)
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
