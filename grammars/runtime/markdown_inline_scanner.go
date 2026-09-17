//go:build !grammar_subset || grammar_subset_markdown_inline

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the markdown_inline grammar.
const (
	mdiTokError                   = 0
	mdiTokTriggerError            = 1
	mdiTokCodeSpanStart           = 2
	mdiTokCodeSpanClose           = 3
	mdiTokEmphasisOpenStar        = 4
	mdiTokEmphasisOpenUnderscore  = 5
	mdiTokEmphasisCloseStar       = 6
	mdiTokEmphasisCloseUnderscore = 7
	mdiTokLastTokenWhitespace     = 8
	mdiTokLastTokenPunctuation    = 9
	mdiTokStrikethroughOpen       = 10
	mdiTokStrikethroughClose      = 11
	mdiTokLatexSpanStart          = 12
	mdiTokLatexSpanClose          = 13
	mdiTokUnclosedSpan            = 14
)

// Symbol constants for the markdown_inline grammar.
const (
	mdiSymError                   gotreesitter.Symbol = 52
	mdiSymTriggerError            gotreesitter.Symbol = 53
	mdiSymCodeSpanStart           gotreesitter.Symbol = 54
	mdiSymCodeSpanClose           gotreesitter.Symbol = 55
	mdiSymEmphasisOpenStar        gotreesitter.Symbol = 56
	mdiSymEmphasisOpenUnderscore  gotreesitter.Symbol = 57
	mdiSymEmphasisCloseStar       gotreesitter.Symbol = 58
	mdiSymEmphasisCloseUnderscore gotreesitter.Symbol = 59
	mdiSymLastTokenWhitespace     gotreesitter.Symbol = 60
	mdiSymLastTokenPunctuation    gotreesitter.Symbol = 61
	mdiSymStrikethroughOpen       gotreesitter.Symbol = 62
	mdiSymStrikethroughClose      gotreesitter.Symbol = 63
	mdiSymLatexSpanStart          gotreesitter.Symbol = 64
	mdiSymLatexSpanClose          gotreesitter.Symbol = 65
	mdiSymUnclosedSpan            gotreesitter.Symbol = 66
)

// State bitflags used with mdiState.state
const (
	mdiStateEmphasisDelimiterIsOpen uint8 = 1 << 2
)

// mdiState holds the scanner state for markdown_inline.
type mdiState struct {
	state                     uint8
	codeSpanDelimiterLength   uint8
	latexSpanDelimiterLength  uint8
	numEmphasisDelimitersLeft uint8
}

// MarkdownInlineExternalScanner handles external scanning for the markdown_inline grammar.
type MarkdownInlineExternalScanner struct{}

func (MarkdownInlineExternalScanner) Create() any {
	return &mdiState{}
}

func (MarkdownInlineExternalScanner) Destroy(payload any) {}

func (MarkdownInlineExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*mdiState)
	if len(buf) < 4 {
		return 0
	}
	buf[0] = s.state
	buf[1] = s.codeSpanDelimiterLength
	buf[2] = s.latexSpanDelimiterLength
	buf[3] = s.numEmphasisDelimitersLeft
	return 4
}

func (MarkdownInlineExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*mdiState)
	s.state = 0
	s.codeSpanDelimiterLength = 0
	s.latexSpanDelimiterLength = 0
	s.numEmphasisDelimitersLeft = 0
	if len(buf) > 0 {
		s.state = buf[0]
		s.codeSpanDelimiterLength = buf[1]
		s.latexSpanDelimiterLength = buf[2]
		s.numEmphasisDelimitersLeft = buf[3]
	}
}

func (MarkdownInlineExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*mdiState)

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	// A normal tree-sitter rule decided that the current branch is invalid and
	// now "requests" an error to stop the branch.
	if isValid(mdiTokTriggerError) {
		lexer.SetResultSymbol(mdiSymError)
		return true
	}

	// Decide which tokens to consider based on the first non-whitespace character.
	switch lexer.Lookahead() {
	case '`':
		return mdiParseLeafDelimiter(s, lexer, &s.codeSpanDelimiterLength,
			validSymbols, '`', mdiTokCodeSpanStart, mdiTokCodeSpanClose,
			mdiSymCodeSpanStart, mdiSymCodeSpanClose, isValid)
	case '$':
		return mdiParseLeafDelimiter(s, lexer, &s.latexSpanDelimiterLength,
			validSymbols, '$', mdiTokLatexSpanStart, mdiTokLatexSpanClose,
			mdiSymLatexSpanStart, mdiSymLatexSpanClose, isValid)
	case '*':
		return mdiParseStar(s, lexer, validSymbols, isValid)
	case '_':
		return mdiParseUnderscore(s, lexer, validSymbols, isValid)
	case '~':
		return mdiParseTilde(s, lexer, validSymbols, isValid)
	}
	return false
}

// mdiIsPunctuation determines if a character is punctuation as defined by the
// markdown spec.
func mdiIsPunctuation(chr rune) bool {
	return (chr >= '!' && chr <= '/') || (chr >= ':' && chr <= '@') ||
		(chr >= '[' && chr <= '`') || (chr >= '{' && chr <= '~')
}

// mdiParseLeafDelimiter handles parsing of code span (backtick) and latex span
// (dollar) delimiters.
func mdiParseLeafDelimiter(
	s *mdiState,
	lexer *gotreesitter.ExternalLexer,
	delimiterLength *uint8,
	validSymbols []bool,
	delimiter rune,
	openTok int,
	closeTok int,
	openSym gotreesitter.Symbol,
	closeSym gotreesitter.Symbol,
	isValid func(int) bool,
) bool {
	var level uint8
	for lexer.Lookahead() == delimiter {
		lexer.Advance(false)
		level++
	}
	lexer.MarkEnd()

	if level == *delimiterLength && isValid(closeTok) {
		*delimiterLength = 0
		lexer.SetResultSymbol(closeSym)
		return true
	}

	if isValid(openTok) {
		// Parse ahead to check if there is a closing delimiter.
		var closeLevel uint8
		for lexer.Lookahead() != 0 {
			if lexer.Lookahead() == delimiter {
				closeLevel++
			} else {
				if closeLevel == level {
					// Found a matching delimiter.
					break
				}
				closeLevel = 0
			}
			lexer.Advance(false)
		}
		if closeLevel == level {
			*delimiterLength = level
			lexer.SetResultSymbol(openSym)
			return true
		}
		if isValid(mdiTokUnclosedSpan) {
			lexer.SetResultSymbol(mdiSymUnclosedSpan)
			return true
		}
	}
	return false
}

// mdiParseStar handles star-based emphasis delimiters.
func mdiParseStar(s *mdiState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, isValid func(int) bool) bool {
	lexer.Advance(false)

	// If numEmphasisDelimitersLeft is not zero then we already decided that
	// this should be part of an emphasis delimiter run, so interpret it as such.
	if s.numEmphasisDelimitersLeft > 0 {
		if (s.state&mdiStateEmphasisDelimiterIsOpen) != 0 && isValid(mdiTokEmphasisOpenStar) {
			s.state &^= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(mdiSymEmphasisOpenStar)
			s.numEmphasisDelimitersLeft--
			return true
		}
		if isValid(mdiTokEmphasisCloseStar) {
			lexer.SetResultSymbol(mdiSymEmphasisCloseStar)
			s.numEmphasisDelimitersLeft--
			return true
		}
	}

	lexer.MarkEnd()

	// Count the number of stars.
	starCount := uint8(1)
	for lexer.Lookahead() == '*' {
		starCount++
		lexer.Advance(false)
	}

	lineEnd := lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || lexer.Lookahead() == 0

	if isValid(mdiTokEmphasisOpenStar) || isValid(mdiTokEmphasisCloseStar) {
		s.numEmphasisDelimitersLeft = starCount - 1

		nextSymbolWhitespace := lineEnd || lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t'
		nextSymbolPunctuation := mdiIsPunctuation(lexer.Lookahead())

		// Closing delimiters take precedence.
		if isValid(mdiTokEmphasisCloseStar) &&
			!isValid(mdiTokLastTokenWhitespace) &&
			(!isValid(mdiTokLastTokenPunctuation) || nextSymbolPunctuation || nextSymbolWhitespace) {
			s.state &^= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(mdiSymEmphasisCloseStar)
			return true
		}
		if !nextSymbolWhitespace &&
			(!nextSymbolPunctuation || isValid(mdiTokLastTokenPunctuation) || isValid(mdiTokLastTokenWhitespace)) {
			s.state |= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(mdiSymEmphasisOpenStar)
			return true
		}
	}
	return false
}

// mdiParseTilde handles tilde-based strikethrough delimiters.
func mdiParseTilde(s *mdiState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, isValid func(int) bool) bool {
	lexer.Advance(false)

	// If numEmphasisDelimitersLeft is not zero then we already decided that
	// this should be part of an emphasis delimiter run, so interpret it as such.
	if s.numEmphasisDelimitersLeft > 0 {
		if (s.state&mdiStateEmphasisDelimiterIsOpen) != 0 && isValid(mdiTokStrikethroughOpen) {
			s.state &^= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(mdiSymStrikethroughOpen)
			s.numEmphasisDelimitersLeft--
			return true
		}
		if isValid(mdiTokStrikethroughClose) {
			lexer.SetResultSymbol(mdiSymStrikethroughClose)
			s.numEmphasisDelimitersLeft--
			return true
		}
	}

	lexer.MarkEnd()

	// Count the number of tildes.
	tildeCount := uint8(1)
	for lexer.Lookahead() == '~' {
		tildeCount++
		lexer.Advance(false)
	}
	if tildeCount < 2 {
		return false
	}

	lineEnd := lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || lexer.Lookahead() == 0

	if isValid(mdiTokStrikethroughOpen) || isValid(mdiTokStrikethroughClose) {
		s.numEmphasisDelimitersLeft = tildeCount - 1

		nextSymbolWhitespace := lineEnd || lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t'
		nextSymbolPunctuation := mdiIsPunctuation(lexer.Lookahead())

		// Closing delimiters take precedence.
		if isValid(mdiTokStrikethroughClose) &&
			!isValid(mdiTokLastTokenWhitespace) &&
			(!isValid(mdiTokLastTokenPunctuation) || nextSymbolPunctuation || nextSymbolWhitespace) {
			s.state &^= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(mdiSymStrikethroughClose)
			return true
		}
		if !nextSymbolWhitespace &&
			(!nextSymbolPunctuation || isValid(mdiTokLastTokenPunctuation) || isValid(mdiTokLastTokenWhitespace)) {
			s.state |= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(mdiSymStrikethroughOpen)
			return true
		}
	}
	return false
}

// mdiParseUnderscore handles underscore-based emphasis delimiters.
func mdiParseUnderscore(s *mdiState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, isValid func(int) bool) bool {
	lexer.Advance(false)

	// If numEmphasisDelimitersLeft is not zero then we already decided that
	// this should be part of an emphasis delimiter run, so interpret it as such.
	if s.numEmphasisDelimitersLeft > 0 {
		if (s.state&mdiStateEmphasisDelimiterIsOpen) != 0 && isValid(mdiTokEmphasisOpenUnderscore) {
			s.state &^= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(mdiSymEmphasisOpenUnderscore)
			s.numEmphasisDelimitersLeft--
			return true
		}
		if isValid(mdiTokEmphasisCloseUnderscore) {
			lexer.SetResultSymbol(mdiSymEmphasisCloseUnderscore)
			s.numEmphasisDelimitersLeft--
			return true
		}
	}

	lexer.MarkEnd()

	// Count the number of underscores.
	underscoreCount := uint8(1)
	for lexer.Lookahead() == '_' {
		underscoreCount++
		lexer.Advance(false)
	}

	lineEnd := lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || lexer.Lookahead() == 0

	if isValid(mdiTokEmphasisOpenUnderscore) || isValid(mdiTokEmphasisCloseUnderscore) {
		s.numEmphasisDelimitersLeft = underscoreCount - 1

		nextSymbolWhitespace := lineEnd || lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t'
		nextSymbolPunctuation := mdiIsPunctuation(lexer.Lookahead())

		// Closing delimiters take precedence.
		if isValid(mdiTokEmphasisCloseUnderscore) &&
			!isValid(mdiTokLastTokenWhitespace) &&
			(!isValid(mdiTokLastTokenPunctuation) || nextSymbolPunctuation || nextSymbolWhitespace) {
			s.state &^= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(mdiSymEmphasisCloseUnderscore)
			return true
		}
		if !nextSymbolWhitespace &&
			(!nextSymbolPunctuation || isValid(mdiTokLastTokenPunctuation) || isValid(mdiTokLastTokenWhitespace)) {
			s.state |= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(mdiSymEmphasisOpenUnderscore)
			return true
		}
	}
	return false
}
