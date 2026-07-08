//go:build !grammar_subset || grammar_subset_perl

package grammars

import (
	"strings"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Perl grammar.
const (
	plTokApostrophe           = 0  // start_delimiter  '
	plTokDoubleQuote          = 1  // end_delimiter    "
	plTokBacktick             = 2  // start_delimiter2  `
	plTokSearchSlash          = 3  // end_delimiter2    /
	plTokNoSearchSlashPlz     = 4  // _no_search_slash_plz
	plTokOpenReadlineBracket  = 5  // <  (heredoc-like)
	plTokOpenFileglobBracket  = 6  // <  (heredoc-like2)
	plTokPerlySemicolon       = 7  // _PERLY_SEMICOLON
	plTokPerlyHeredoc         = 8  // _PERLY_HEREDOC
	plTokCtrlZ                = 9  // eof_marker
	plTokQuotelikeBegin       = 10 // start_delimiter3  '
	plTokQuotelikeMiddleClose = 11 // end_delimiter3    '
	plTokQuotelikeMiddleSkip  = 12 // _quotelike_middle_skip
	plTokQuotelikeEndZW       = 13 // _quotelike_end_zw
	plTokQuotelikeEnd         = 14 // start_delimiter4  '
	plTokQStringContent       = 15 // _q_string_content
	plTokQQStringContent      = 16 // _qq_string_content
	plTokEscapeSequence       = 17 // escape_sequence
	plTokEscapedDelimiter     = 18 // escaped_delimiter
	plTokDollarInRegexp       = 19 // _dollar_in_regexp
	plTokPod                  = 20 // pod
	plTokGobbledContent       = 21 // _gobbled_content
	plTokAttributeValueBegin  = 22 // _attribute_value_begin
	plTokAttributeValue       = 23 // attribute_value
	plTokPrototype            = 24 // prototype
	plTokSignatureStart       = 25 // (
	plTokHeredocDelim         = 26 // _heredoc_delimiter
	plTokCommandHeredocDelim  = 27 // _command_heredoc_delimiter
	plTokHeredocStart         = 28 // _heredoc_start
	plTokHeredocMiddle        = 29 // _heredoc_middle
	plTokHeredocEnd           = 30 // heredoc_end
	plTokFatCommaAutoquoted   = 31 // _fat_comma_autoquoted
	plTokFiletest             = 32 // -x
	plTokBraceAutoquoted      = 33 // varname
	plTokBraceEndZW           = 34 // _brace_end_zw
	plTokDollarIdentZW        = 35 // _dollar_ident_zw
	plTokNoInterpWhitespaceZW = 36 // _no_interp_whitespace_zw
	plTokNonassoc             = 37 // _NONASSOC
	plTokError                = 38 // _ERROR
)

// Symbol constants for the Perl grammar.
const (
	plSymApostrophe           gotreesitter.Symbol = 252
	plSymDoubleQuote          gotreesitter.Symbol = 253
	plSymBacktick             gotreesitter.Symbol = 254
	plSymSearchSlash          gotreesitter.Symbol = 255
	plSymNoSearchSlashPlz     gotreesitter.Symbol = 256
	plSymOpenReadlineBracket  gotreesitter.Symbol = 257
	plSymOpenFileglobBracket  gotreesitter.Symbol = 258
	plSymPerlySemicolon       gotreesitter.Symbol = 259
	plSymPerlyHeredoc         gotreesitter.Symbol = 260
	plSymCtrlZ                gotreesitter.Symbol = 261
	plSymQuotelikeBegin       gotreesitter.Symbol = 262
	plSymQuotelikeMiddleClose gotreesitter.Symbol = 263
	plSymQuotelikeMiddleSkip  gotreesitter.Symbol = 264
	plSymQuotelikeEndZW       gotreesitter.Symbol = 265
	plSymQuotelikeEnd         gotreesitter.Symbol = 266
	plSymQStringContent       gotreesitter.Symbol = 267
	plSymQQStringContent      gotreesitter.Symbol = 268
	plSymEscapeSequence       gotreesitter.Symbol = 269
	plSymEscapedDelimiter     gotreesitter.Symbol = 270
	plSymDollarInRegexp       gotreesitter.Symbol = 271
	plSymPod                  gotreesitter.Symbol = 272
	plSymGobbledContent       gotreesitter.Symbol = 273
	plSymAttributeValueBegin  gotreesitter.Symbol = 274
	plSymAttributeValue       gotreesitter.Symbol = 275
	plSymPrototype            gotreesitter.Symbol = 276
	plSymSignatureStart       gotreesitter.Symbol = 277
	plSymHeredocDelim         gotreesitter.Symbol = 278
	plSymCommandHeredocDelim  gotreesitter.Symbol = 279
	plSymHeredocStart         gotreesitter.Symbol = 280
	plSymHeredocMiddle        gotreesitter.Symbol = 281
	plSymHeredocEnd           gotreesitter.Symbol = 282
	plSymFatCommaAutoquoted   gotreesitter.Symbol = 283
	plSymFiletest             gotreesitter.Symbol = 284
	plSymBraceAutoquoted      gotreesitter.Symbol = 285
	plSymBraceEndZW           gotreesitter.Symbol = 286
	plSymDollarIdentZW        gotreesitter.Symbol = 287
	plSymNoInterpWhitespaceZW gotreesitter.Symbol = 288
	plSymNonassoc             gotreesitter.Symbol = 289
	plSymError                gotreesitter.Symbol = 290
)

// Token index to symbol mapping.
var plSymForTok = [...]gotreesitter.Symbol{
	plSymApostrophe,
	plSymDoubleQuote,
	plSymBacktick,
	plSymSearchSlash,
	plSymNoSearchSlashPlz,
	plSymOpenReadlineBracket,
	plSymOpenFileglobBracket,
	plSymPerlySemicolon,
	plSymPerlyHeredoc,
	plSymCtrlZ,
	plSymQuotelikeBegin,
	plSymQuotelikeMiddleClose,
	plSymQuotelikeMiddleSkip,
	plSymQuotelikeEndZW,
	plSymQuotelikeEnd,
	plSymQStringContent,
	plSymQQStringContent,
	plSymEscapeSequence,
	plSymEscapedDelimiter,
	plSymDollarInRegexp,
	plSymPod,
	plSymGobbledContent,
	plSymAttributeValueBegin,
	plSymAttributeValue,
	plSymPrototype,
	plSymSignatureStart,
	plSymHeredocDelim,
	plSymCommandHeredocDelim,
	plSymHeredocStart,
	plSymHeredocMiddle,
	plSymHeredocEnd,
	plSymFatCommaAutoquoted,
	plSymFiletest,
	plSymBraceAutoquoted,
	plSymBraceEndZW,
	plSymDollarIdentZW,
	plSymNoInterpWhitespaceZW,
	plSymNonassoc,
	plSymError,
}

// plMaxTSPStringLen is the maximum number of runes we track in a heredoc delimiter.
const plMaxTSPStringLen = 8

// plTSPString is a fixed-capacity string for heredoc delimiters.
type plTSPString struct {
	length   int
	contents [plMaxTSPStringLen]rune
}

func (s *plTSPString) push(c rune) {
	if s.length < plMaxTSPStringLen {
		s.contents[s.length] = c
	}
	s.length++
}

func (s *plTSPString) eq(other *plTSPString) bool {
	if s.length != other.length {
		return false
	}
	maxLen := s.length
	if maxLen > plMaxTSPStringLen {
		maxLen = plMaxTSPStringLen
	}
	for i := 0; i < maxLen; i++ {
		if s.contents[i] != other.contents[i] {
			return false
		}
	}
	return true
}

func (s *plTSPString) reset() {
	s.length = 0
}

// plQuote tracks a quotelike delimiter pair and its nesting count.
type plQuote struct {
	open  rune
	close rune
	count int32
}

// plHeredocState tracks the phase of heredoc parsing.
type plHeredocState int

const (
	plHeredocNone plHeredocState = iota
	plHeredocStart
	plHeredocUnknown
	plHeredocContinue
	plHeredocEnd
)

// plState is the persistent state for the Perl external scanner.
type plState struct {
	quotes              []plQuote
	heredocInterpolates bool
	heredocIndents      bool
	heredocState        plHeredocState
	heredocDelim        plTSPString
}

func (st *plState) pushQuote(opener rune) {
	q := plQuote{}
	closer := plCloseForOpen(opener)
	if closer != 0 {
		q.open = opener
		q.close = closer
	} else {
		q.open = 0
		q.close = opener
	}
	q.count = 0
	st.quotes = append(st.quotes, q)
}

// isQuoteOpener checks from the end of the quote stack if c is an opener.
// Returns idx+1 (1-based) or 0 if not found.
func (st *plState) isQuoteOpener(c rune) int {
	for i := len(st.quotes) - 1; i >= 0; i-- {
		if st.quotes[i].open != 0 && c == st.quotes[i].open {
			return i + 1
		}
	}
	return 0
}

func (st *plState) sawOpener(idx int) {
	st.quotes[idx-1].count++
}

// isQuoteCloser checks from the end of the quote stack if c is a closer.
// Returns idx+1 (1-based) or 0 if not found.
func (st *plState) isQuoteCloser(c rune) int {
	for i := len(st.quotes) - 1; i >= 0; i-- {
		if st.quotes[i].close != 0 && c == st.quotes[i].close {
			return i + 1
		}
	}
	return 0
}

func (st *plState) sawCloser(idx int) {
	if st.quotes[idx-1].count > 0 {
		st.quotes[idx-1].count--
	}
}

func (st *plState) isQuoteClosed(idx int) bool {
	return st.quotes[idx-1].count == 0
}

func (st *plState) popQuote(idx int) {
	st.quotes = append(st.quotes[:idx-1], st.quotes[idx:]...)
}

func (st *plState) isPairedDelimiter() bool {
	if len(st.quotes) == 0 {
		return false
	}
	return st.quotes[len(st.quotes)-1].open != 0
}

func (st *plState) addHeredoc(delim *plTSPString, interp bool, indent bool) {
	st.heredocDelim = *delim
	st.heredocInterpolates = interp
	st.heredocIndents = indent
	st.heredocState = plHeredocStart
}

func (st *plState) finishHeredoc() {
	st.heredocDelim.length = 0
	st.heredocState = plHeredocNone
}

// plCloseForOpen returns the matching close bracket or 0 for non-bracketed delimiters.
func plCloseForOpen(c rune) rune {
	switch c {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	case '<':
		return '>'
	default:
		return 0
	}
}

func plIsWhitespace(c rune) bool {
	return unicode.IsSpace(c)
}

func plIsIDFirst(c rune) bool {
	return c == '_' || unicode.IsLetter(c)
}

func plIsIDCont(c rune) bool {
	return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c)
}

func plIsInterpolationEscape(c rune) bool {
	return c < 256 && strings.ContainsRune("$@-[{\\", c)
}

// PerlExternalScanner implements gotreesitter.ExternalScanner for Perl.
type PerlExternalScanner struct{}

func (PerlExternalScanner) Create() any {
	return &plState{}
}

func (PerlExternalScanner) Destroy(payload any) {}

func (PerlExternalScanner) Serialize(payload any, buf []byte) int {
	st := payload.(*plState)
	size := 0

	quoteCount := len(st.quotes)
	if quoteCount > 255 {
		quoteCount = 255
	}
	if size >= len(buf) {
		return 0
	}
	buf[size] = byte(quoteCount)
	size++

	// Each quote: open(4) + close(4) + count(4) = 12 bytes
	for i := 0; i < quoteCount; i++ {
		if size+12 > len(buf) {
			return 0
		}
		q := &st.quotes[i]
		plPutI32(buf[size:], q.open)
		size += 4
		plPutI32(buf[size:], q.close)
		size += 4
		plPutI32(buf[size:], rune(q.count))
		size += 4
	}

	// heredoc state: interp(1) + indent(1) + state(1)
	if size+3 > len(buf) {
		return 0
	}
	buf[size] = plBoolToByte(st.heredocInterpolates)
	size++
	buf[size] = plBoolToByte(st.heredocIndents)
	size++
	buf[size] = byte(st.heredocState)
	size++

	// heredoc delim: length(4) + contents(plMaxTSPStringLen * 4)
	delimSize := 4 + plMaxTSPStringLen*4
	if size+delimSize > len(buf) {
		return 0
	}
	plPutI32(buf[size:], rune(st.heredocDelim.length))
	size += 4
	for i := 0; i < plMaxTSPStringLen; i++ {
		plPutI32(buf[size:], st.heredocDelim.contents[i])
		size += 4
	}

	return size
}

func (PerlExternalScanner) Deserialize(payload any, buf []byte) {
	st := payload.(*plState)
	st.quotes = st.quotes[:0]
	st.heredocInterpolates = false
	st.heredocIndents = false
	st.heredocState = plHeredocNone
	st.heredocDelim.reset()

	if len(buf) == 0 {
		return
	}

	size := 0
	if size >= len(buf) {
		return
	}
	quoteCount := int(buf[size])
	size++

	for i := 0; i < quoteCount; i++ {
		if size+12 > len(buf) {
			return
		}
		var q plQuote
		q.open = plGetI32(buf[size:])
		size += 4
		q.close = plGetI32(buf[size:])
		size += 4
		q.count = int32(plGetI32(buf[size:]))
		size += 4
		st.quotes = append(st.quotes, q)
	}

	if size+3 > len(buf) {
		return
	}
	st.heredocInterpolates = buf[size] != 0
	size++
	st.heredocIndents = buf[size] != 0
	size++
	st.heredocState = plHeredocState(buf[size])
	size++

	delimSize := 4 + plMaxTSPStringLen*4
	if size+delimSize > len(buf) {
		return
	}
	st.heredocDelim.length = int(plGetI32(buf[size:]))
	size += 4
	for i := 0; i < plMaxTSPStringLen; i++ {
		st.heredocDelim.contents[i] = plGetI32(buf[size:])
		size += 4
	}
}

func (PerlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	st := payload.(*plState)

	valid := func(tok int) bool {
		return tok < len(validSymbols) && validSymbols[tok]
	}

	token := func(tok int) bool {
		lexer.SetResultSymbol(plSymForTok[tok])
		return true
	}

	isError := valid(plTokError)
	skippedWhitespace := false
	c := lexer.Lookahead()

	// TOKEN_GOBBLED_CONTENT: consume everything until EOF
	if !isError && valid(plTokGobbledContent) {
		for lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}
		return token(plTokGobbledContent)
	}

	// NONASSOC: force tree-sitter to stay on error branch
	if !isError && valid(plTokNonassoc) {
		return token(plTokNonassoc)
	}

	// Heredoc middle: whitespace-sensitive, must come before whitespace skip
	if valid(plTokHeredocMiddle) && !isError {
		if st.heredocState != plHeredocContinue {
			var line plTSPString
			for lexer.Lookahead() != 0 {
				line.reset()
				isValidStartPos := st.heredocState == plHeredocEnd || lexer.Column() == 0
				sawEscape := false
				if isValidStartPos && st.heredocIndents {
					plSkipWhitespace(lexer)
					c = lexer.Lookahead()
				}
				lexer.MarkEnd()
				// Read the whole line
				for c != '\n' && lexer.Lookahead() != 0 {
					if c == '\r' {
						lexer.Advance(false)
						c = lexer.Lookahead()
						if c == '\n' {
							break
						}
						line.push('\r')
					}
					line.push(c)
					if c == '$' || c == '@' || c == '\\' {
						sawEscape = true
					}
					lexer.Advance(false)
					c = lexer.Lookahead()
				}
				if isValidStartPos && line.eq(&st.heredocDelim) {
					if st.heredocState != plHeredocEnd {
						st.heredocState = plHeredocEnd
						return token(plTokHeredocMiddle)
					}
					lexer.MarkEnd()
					st.finishHeredoc()
					return token(plTokHeredocEnd)
				}
				if sawEscape && st.heredocInterpolates {
					st.heredocState = plHeredocContinue
					return token(plTokHeredocMiddle)
				}
				// Eat the newline and loop
				lexer.Advance(false)
				c = lexer.Lookahead()
			}
		} else {
			// Continue mode: read ahead until newline or interpolation escape
			sawChars := false
			for {
				if plIsInterpolationEscape(c) {
					lexer.MarkEnd()
					break
				}
				if c == '\n' {
					lexer.MarkEnd()
					st.heredocState = plHeredocUnknown
					return token(plTokHeredocMiddle)
				}
				sawChars = true
				lexer.Advance(false)
				c = lexer.Lookahead()
			}
			if sawChars {
				return token(plTokHeredocMiddle)
			}
		}
	}

	// Zero-width no-interp whitespace token
	if plIsWhitespace(c) && valid(plTokNoInterpWhitespaceZW) {
		return token(plTokNoInterpWhitespaceZW)
	}

	// Track if initial c was whitespace before skip_ws_to_eol
	if plIsWhitespace(c) {
		skippedWhitespace = true
	}

	// Skip whitespace to end of line
	plSkipWSToEOL(lexer)

	// Heredoc start: heredocs override everything
	if valid(plTokHeredocStart) {
		if st.heredocState == plHeredocStart && lexer.Column() == 0 {
			st.heredocState = plHeredocUnknown
			return token(plTokHeredocStart)
		}
	}

	c = lexer.Lookahead()

	// Attribute value begin
	if !isError && valid(plTokAttributeValueBegin) && c == '(' {
		return token(plTokAttributeValueBegin)
	}

	// Attribute value
	if !isError && valid(plTokAttributeValue) {
		delimcount := 0
		for lexer.Lookahead() != 0 {
			if c == '\\' {
				lexer.Advance(false)
				// ignore the next char
			} else if c == '(' {
				delimcount++
			} else if c == ')' {
				if delimcount > 0 {
					delimcount--
				} else {
					break
				}
			}
			lexer.Advance(false)
			c = lexer.Lookahead()
		}
		return token(plTokAttributeValue)
	}

	// Additional whitespace skip
	if plIsWhitespace(c) {
		skippedWhitespace = true
		plSkipWhitespace(lexer)
		c = lexer.Lookahead()
	}

	// CTRL-Z (character 26) as EOF marker
	if c == 26 && valid(plTokCtrlZ) {
		return token(plTokCtrlZ)
	}

	// PERLY_SEMICOLON at end of scope
	if valid(plTokPerlySemicolon) {
		if c == '}' || lexer.Lookahead() == 0 {
			if isError || !valid(plTokBraceEndZW) {
				return token(plTokPerlySemicolon)
			}
		}
	}
	if lexer.Lookahead() == 0 {
		return false
	}

	// Readline/fileglob/heredoc bracket handling
	if valid(plTokOpenFileglobBracket) || valid(plTokOpenReadlineBracket) || valid(plTokPerlyHeredoc) {
		if c == '<' {
			lexer.Advance(false)
			c = lexer.Lookahead()
			lexer.MarkEnd()
			// Check for heredoc <<
			if c == '<' {
				return plHandleHeredocToken(st, lexer, token)
			}
			if c == '$' {
				lexer.Advance(false)
				c = lexer.Lookahead()
			}
			// Zoom past ident chars
			for plIsIDCont(c) {
				lexer.Advance(false)
				c = lexer.Lookahead()
			}
			if c == '>' {
				return token(plTokOpenReadlineBracket)
			}
			st.pushQuote('<')
			return token(plTokOpenFileglobBracket)
		}
	}

	// Dollar ident zero-width
	if valid(plTokDollarIdentZW) {
		if !plIsIDCont(c) && !strings.ContainsRune("${", c) {
			if c == ':' {
				lexer.MarkEnd()
				lexer.Advance(false)
				c = lexer.Lookahead()
				if c == ':' {
					return false
				}
			}
			return token(plTokDollarIdentZW)
		}
	}

	// Search slash
	if valid(plTokSearchSlash) && c == '/' && !valid(plTokNoSearchSlashPlz) {
		lexer.Advance(false)
		c = lexer.Lookahead()
		lexer.MarkEnd()
		if c != '/' {
			st.pushQuote('/')
			return token(plTokSearchSlash)
		}
		return false
	}

	// Apostrophe
	if valid(plTokApostrophe) && c == '\'' {
		lexer.Advance(false)
		st.pushQuote('\'')
		return token(plTokApostrophe)
	}
	// Double quote
	if valid(plTokDoubleQuote) && c == '"' {
		lexer.Advance(false)
		st.pushQuote('"')
		return token(plTokDoubleQuote)
	}
	// Backtick
	if valid(plTokBacktick) && c == '`' {
		lexer.Advance(false)
		st.pushQuote('`')
		return token(plTokBacktick)
	}

	// Dollar in regexp
	if valid(plTokDollarInRegexp) && c == '$' {
		lexer.Advance(false)
		c = lexer.Lookahead()
		if st.isQuoteCloser(c) != 0 {
			return token(plTokDollarInRegexp)
		}
		switch c {
		case '(', ')', '|':
			return token(plTokDollarInRegexp)
		}
		return false
	}

	// POD
	if valid(plTokPod) {
		column := lexer.Column()
		if column == 0 && c == '=' {
			cutMarker := "=cut"
			stage := -1
			for lexer.Lookahead() != 0 {
				if c == '\r' {
					// ignore
				} else if stage < 1 && c == '\n' {
					stage = 0
				} else if stage >= 0 && stage < 4 && c == rune(cutMarker[stage]) {
					stage++
				} else if stage == 4 && (c == ' ' || c == '\t') {
					stage = 5
				} else if stage == 4 && c == '\n' {
					stage = 6
				} else {
					stage = -1
				}
				if stage > 4 {
					break
				}
				lexer.Advance(false)
				c = lexer.Lookahead()
			}
			if stage < 6 {
				for lexer.Lookahead() != 0 {
					if c == '\n' {
						break
					}
					lexer.Advance(false)
					c = lexer.Lookahead()
				}
			}
			return token(plTokPod)
		}
	}

	// Past this point, bail on error
	if isError {
		return false
	}

	// Heredoc delimiter
	if valid(plTokHeredocDelim) || valid(plTokCommandHeredocDelim) {
		shouldIndent := false
		shouldInterpolate := true
		var delim plTSPString
		delim.reset()

		if !skippedWhitespace {
			if c == '~' {
				lexer.Advance(false)
				c = lexer.Lookahead()
				shouldIndent = true
			}
			if c == '\\' {
				lexer.Advance(false)
				c = lexer.Lookahead()
				shouldInterpolate = false
			}
			if plIsIDFirst(c) {
				for plIsIDCont(c) {
					delim.push(c)
					lexer.Advance(false)
					c = lexer.Lookahead()
				}
				st.addHeredoc(&delim, shouldInterpolate, shouldIndent)
				return token(plTokHeredocDelim)
			}
		}
		// If we picked up a ~ before, we may have to skip to hit the quote
		if shouldIndent {
			plSkipWhitespace(lexer)
			c = lexer.Lookahead()
		}
		// Quoted heredoc delimiter
		if shouldInterpolate && (c == '\'' || c == '"' || c == '`') {
			delimOpen := c
			shouldInterpolate = c != '\''
			lexer.Advance(false)
			c = lexer.Lookahead()
			for c != delimOpen && lexer.Lookahead() != 0 {
				if c == '\\' {
					toAdd := c
					lexer.Advance(false)
					c = lexer.Lookahead()
					if c == delimOpen {
						toAdd = delimOpen
						lexer.Advance(false)
						c = lexer.Lookahead()
					}
					delim.push(toAdd)
				} else {
					delim.push(c)
					lexer.Advance(false)
					c = lexer.Lookahead()
				}
			}
			if delim.length > 0 {
				// Eat the closing delimiter quote
				lexer.Advance(false)
				st.addHeredoc(&delim, shouldInterpolate, shouldIndent)
				if delimOpen == '`' {
					return token(plTokCommandHeredocDelim)
				}
				return token(plTokHeredocDelim)
			}
		}
	}

	// Quotelike middle skip: for 3-part quotelikes with non-paired delimiters
	if valid(plTokQuotelikeMiddleSkip) {
		if !st.isPairedDelimiter() {
			return token(plTokQuotelikeMiddleSkip)
		}
	}

	// Quotelike begin (generic quote character)
	if valid(plTokQuotelikeBegin) {
		delim := c
		if skippedWhitespace && c == '#' {
			return false
		}
		lexer.MarkEnd()
		lexer.Advance(false)

		// Guard against brace end in autoquote context
		if valid(plTokBraceEndZW) && delim == '}' {
			return token(plTokBraceEndZW)
		}
		lexer.MarkEnd()
		st.pushQuote(delim)
		return token(plTokQuotelikeBegin)
	}

	// Backslash handling (escape sequences and escaped delimiters)
	if c == '\\' && !(valid(plTokQuotelikeEnd) && st.isQuoteCloser('\\') != 0) {
		lexer.Advance(false)
		c = lexer.Lookahead()
		escC := c
		if !plIsWhitespace(c) {
			lexer.Advance(false)
			c = lexer.Lookahead()
		}

		if valid(plTokEscapedDelimiter) {
			if st.isQuoteOpener(escC) != 0 || st.isQuoteCloser(escC) != 0 {
				lexer.MarkEnd()
				return token(plTokEscapedDelimiter)
			}
		}

		if valid(plTokEscapeSequence) {
			lexer.MarkEnd()
			// \\ is always an escape sequence
			if escC == '\\' {
				return token(plTokEscapeSequence)
			}
			// Inside q() string, only \\ is a valid escape; all else is literal
			if valid(plTokQStringContent) {
				return token(plTokQStringContent)
			}

			switch escC {
			case 'x':
				if c == '{' {
					plSkipBraced(lexer)
				} else {
					plSkipHexDigits(lexer, 2)
				}
			case 'N':
				plSkipBraced(lexer)
			case 'o':
				plSkipBraced(lexer)
			case '0':
				plSkipOctDigits(lexer, 3)
			}
			return token(plTokEscapeSequence)
		}
	}

	// String content (q and qq)
	if valid(plTokQStringContent) || valid(plTokQQStringContent) {
		isQQ := valid(plTokQQStringContent)
		matched := false

		for c != 0 {
			if c == '\\' {
				break
			}
			quoteIndex := st.isQuoteOpener(c)
			if quoteIndex != 0 {
				st.sawOpener(quoteIndex)
			} else {
				quoteIndex = st.isQuoteCloser(c)
				if quoteIndex != 0 {
					if st.isQuoteClosed(quoteIndex) {
						break
					}
					st.sawCloser(quoteIndex)
				} else if isQQ && plIsInterpolationEscape(c) {
					break
				}
			}
			matched = true
			lexer.Advance(false)
			c = lexer.Lookahead()
		}

		if matched {
			if isQQ {
				return token(plTokQQStringContent)
			}
			return token(plTokQStringContent)
		}
	}

	// Quotelike middle close
	if valid(plTokQuotelikeMiddleClose) {
		quoteIndex := st.isQuoteCloser(c)
		if quoteIndex != 0 && st.isQuoteClosed(quoteIndex) {
			lexer.Advance(false)
			return token(plTokQuotelikeMiddleClose)
		}
	}

	// Quotelike end
	if valid(plTokQuotelikeEnd) {
		quoteIndex := st.isQuoteCloser(c)
		if quoteIndex != 0 {
			if valid(plTokQuotelikeEndZW) {
				return token(plTokQuotelikeEndZW)
			}
			lexer.Advance(false)
			st.popQuote(quoteIndex)
			return token(plTokQuotelikeEnd)
		}
	}

	// Prototype/signature
	if c == '(' && (valid(plTokPrototype) || valid(plTokSignatureStart)) {
		lexer.Advance(false)
		c = lexer.Lookahead()
		lexer.MarkEnd()

		count := 0
		for lexer.Lookahead() != 0 {
			if c == ')' && count == 0 {
				lexer.Advance(false)
				break
			} else if c == ')' {
				count--
			} else if c == '(' {
				count++
			} else if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
				return token(plTokSignatureStart)
			}
			lexer.Advance(false)
			c = lexer.Lookahead()
		}
		lexer.MarkEnd()
		return token(plTokPrototype)
	}

	// Save c for two-char lookahead
	c1 := c

	// File test operator -x
	if c == '-' && valid(plTokFiletest) {
		lexer.Advance(false)
		c = lexer.Lookahead()
		if strings.ContainsRune("rwxoRWXOezsfdlpSbctugkTBMAC", c) {
			lexer.Advance(false)
			c = lexer.Lookahead()
			if !plIsIDCont(c) {
				return token(plTokFiletest)
			}
		}
		return false
	}

	// Fat comma autoquoted / brace autoquoted
	if plIsIDFirst(c) && (valid(plTokFatCommaAutoquoted) || valid(plTokBraceAutoquoted)) {
		for {
			lexer.Advance(false)
			c = lexer.Lookahead()
			if c == 0 || !plIsIDCont(c) {
				break
			}
		}
		lexer.MarkEnd()

		// Skip whitespace and comments
		for plIsWhitespace(c) || c == '#' {
			for plIsWhitespace(c) {
				lexer.Advance(false)
				c = lexer.Lookahead()
			}
			if c == '#' {
				lexer.Advance(false)
				c = lexer.Lookahead()
				for lexer.Column() != 0 {
					lexer.Advance(false)
					c = lexer.Lookahead()
				}
			}
			if lexer.Lookahead() == 0 {
				return false
			}
		}
		c1 = lexer.Lookahead()
		lexer.Advance(false)
		c = lexer.Lookahead()
		if valid(plTokFatCommaAutoquoted) {
			if c1 == '=' && c == '>' {
				return token(plTokFatCommaAutoquoted)
			}
		}
		if valid(plTokBraceAutoquoted) {
			if c1 == '}' {
				return token(plTokBraceAutoquoted)
			}
		}
	} else {
		// Zero-width lookahead section
		lexer.MarkEnd()
		lexer.Advance(false)
		c2 := lexer.Lookahead()
		if lexer.Lookahead() == 0 {
			return false
		}

		// Check for << (heredoc)
		if c1 == '<' && c2 == '<' {
			return plHandleHeredocToken(st, lexer, token)
		}

		// Brace end zero-width
		if valid(plTokBraceEndZW) {
			if c1 == '}' {
				return token(plTokBraceEndZW)
			}
		}
	}

	return false
}

// plHandleHeredocToken handles the << heredoc detection.
// This is extracted as a helper to avoid goto in Go.
func plHandleHeredocToken(st *plState, lexer *gotreesitter.ExternalLexer, token func(int) bool) bool {
	lexer.Advance(false)
	c := lexer.Lookahead()
	lexer.MarkEnd()
	if c == '\\' || c == '~' || plIsIDFirst(c) {
		return token(plTokPerlyHeredoc)
	}
	plSkipWhitespace(lexer)
	c = lexer.Lookahead()
	if c == '\'' || c == '"' || c == '`' {
		return token(plTokPerlyHeredoc)
	}
	return false
}

// plSkipWhitespace skips all whitespace characters.
func plSkipWhitespace(lexer *gotreesitter.ExternalLexer) {
	for {
		c := lexer.Lookahead()
		if c == 0 {
			return
		}
		if plIsWhitespace(c) {
			lexer.Advance(true)
		} else {
			return
		}
	}
}

// plSkipWSToEOL skips whitespace, stopping after a newline.
func plSkipWSToEOL(lexer *gotreesitter.ExternalLexer) {
	for {
		c := lexer.Lookahead()
		if c == 0 {
			return
		}
		if plIsWhitespace(c) {
			lexer.Advance(true)
			if c == '\n' {
				return
			}
		} else {
			return
		}
	}
}

// plSkipBraced skips a { ... } delimited section.
func plSkipBraced(lexer *gotreesitter.ExternalLexer) {
	c := lexer.Lookahead()
	if c != '{' {
		return
	}
	lexer.Advance(false)
	c = lexer.Lookahead()
	for c != 0 && c != '}' {
		lexer.Advance(false)
		c = lexer.Lookahead()
	}
	lexer.Advance(false)
}

// plSkipChars skips up to maxlen characters from the allow set.
func plSkipChars(lexer *gotreesitter.ExternalLexer, maxlen int, allow string) {
	c := lexer.Lookahead()
	for maxlen != 0 {
		if c == 0 {
			return
		}
		if strings.ContainsRune(allow, c) {
			lexer.Advance(false)
			c = lexer.Lookahead()
			if maxlen > 0 {
				maxlen--
			}
		} else {
			break
		}
	}
}

func plSkipHexDigits(lexer *gotreesitter.ExternalLexer, maxlen int) {
	plSkipChars(lexer, maxlen, "0123456789ABCDEFabcdef")
}

func plSkipOctDigits(lexer *gotreesitter.ExternalLexer, maxlen int) {
	plSkipChars(lexer, maxlen, "01234567")
}

// Helper functions for serialization
func plPutI32(buf []byte, v rune) {
	buf[0] = byte(v)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v >> 16)
	buf[3] = byte(v >> 24)
}

func plGetI32(buf []byte) rune {
	return rune(buf[0]) | rune(buf[1])<<8 | rune(buf[2])<<16 | rune(buf[3])<<24
}

func plBoolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
