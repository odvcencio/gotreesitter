//go:build !grammar_subset || grammar_subset_markdown_inline

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the markdown_inline grammar. This is the
// external index (the position of the token in the grammar's
// `externals: [...]` list), which is exactly what tree-sitter's
// `valid_symbols` array and C's result_symbol enum are indexed by. The
// external index is stable across a blob regen as long as the externals
// list itself does not reorder; concrete numeric gotreesitter.Symbol IDs
// are NOT stable (they shift whenever the grammar's total symbol count
// changes), so this scanner never hardcodes them -- see
// mdiDefaultSymTable below.
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
	mdiTokenCount                 = 15
)

// mdiDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped markdown_inline.bin assigns to each external, in
// mdiTok* order. It exists only as a pre-bind fallback (and as an
// independent value to compare a real bind against in tests);
// ExternalScannerForLanguage below overwrites it with values read from the
// actual loaded Language at bind time, which is what the scanner must do
// to survive a future blob regen that renumbers absolute symbol IDs
// without touching the externals list order.
var mdiDefaultSymTable = [mdiTokenCount]gotreesitter.Symbol{
	52, // _error, displays as "_error"
	53, // _trigger_error, displays as "_trigger_error"
	54, // _code_span_start, displays as "code_span_delimiter"
	55, // _code_span_close, displays as "code_span_delimiter"
	56, // _emphasis_open_star, displays as "_emphasis_open_star"
	57, // _emphasis_open_underscore, displays as "_emphasis_open_underscore"
	58, // _emphasis_close_star, displays as "emphasis_delimiter"
	59, // _emphasis_close_underscore, displays as "emphasis_delimiter"
	60, // _last_token_whitespace, displays as "_last_token_whitespace"
	61, // _last_token_punctuation, displays as "_last_token_punctuation"
	62, // _strikethrough_open, displays as "_strikethrough_open"
	63, // _strikethrough_close, displays as "emphasis_delimiter"
	64, // _latex_span_start, displays as "latex_span_delimiter"
	65, // _latex_span_close, displays as "latex_span_delimiter"
	66, // _unclosed_span, displays as "_unclosed_span"
}

// mdiExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (mdiTok* order).
var mdiExternalScannerSpec = ExternalScannerSpec{
	Language:       "markdown_inline",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-markdown",
	UpstreamCommit: "f969cd3ae3f9fbd4e43205431d0ae286014c05b5",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "tree-sitter-markdown-inline/src/grammar.json", SHA256: "4845724f73df627ea2da0383f3574978eb5d3c4d9c114b405a628c3aa558d31a"},
		{Path: "tree-sitter-markdown-inline/src/scanner.c", SHA256: "6a9647420b7260955e8bf1b8338979726347211177bb5cf3d2a9c18109655ac3"},
	},
	Externals: []string{
		"_error",
		"_trigger_error",
		"_code_span_start",
		"_code_span_close",
		"_emphasis_open_star",
		"_emphasis_open_underscore",
		"_emphasis_close_star",
		"_emphasis_close_underscore",
		"_last_token_whitespace",
		"_last_token_punctuation",
		"_strikethrough_open",
		"_strikethrough_close",
		"_latex_span_start",
		"_latex_span_close",
		"_unclosed_span",
	},
}

func init() {
	RegisterExternalScannerSpec(mdiExternalScannerSpec)
}

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
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type MarkdownInlineExternalScanner struct {
	symbols         [mdiTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers markdown_inline's external symbols.
func (MarkdownInlineExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := MarkdownInlineExternalScanner{symbols: mdiDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, mdiExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s MarkdownInlineExternalScanner) symbolTable() *[mdiTokenCount]gotreesitter.Symbol {
	if s.symbols == ([mdiTokenCount]gotreesitter.Symbol{}) {
		return &mdiDefaultSymTable
	}
	return &s.symbols
}

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

func (sc MarkdownInlineExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*mdiState)

	syms := sc.symbolTable()

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	// A normal tree-sitter rule decided that the current branch is invalid and
	// now "requests" an error to stop the branch.
	if isValid(mdiTokTriggerError) {
		lexer.SetResultSymbol(syms[mdiTokError])
		return true
	}

	// Decide which tokens to consider based on the first non-whitespace character.
	switch lexer.Lookahead() {
	case '`':
		return mdiParseLeafDelimiter(s, lexer, &s.codeSpanDelimiterLength,
			validSymbols, '`', mdiTokCodeSpanStart, mdiTokCodeSpanClose,
			syms[mdiTokCodeSpanStart], syms[mdiTokCodeSpanClose], isValid, syms)
	case '$':
		return mdiParseLeafDelimiter(s, lexer, &s.latexSpanDelimiterLength,
			validSymbols, '$', mdiTokLatexSpanStart, mdiTokLatexSpanClose,
			syms[mdiTokLatexSpanStart], syms[mdiTokLatexSpanClose], isValid, syms)
	case '*':
		return mdiParseStar(s, lexer, validSymbols, isValid, syms)
	case '_':
		return mdiParseUnderscore(s, lexer, validSymbols, isValid, syms)
	case '~':
		return mdiParseTilde(s, lexer, validSymbols, isValid, syms)
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
	syms *[mdiTokenCount]gotreesitter.Symbol,
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
			lexer.SetResultSymbol(syms[mdiTokUnclosedSpan])
			return true
		}
	}
	return false
}

// mdiParseStar handles star-based emphasis delimiters.
func mdiParseStar(s *mdiState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, isValid func(int) bool, syms *[mdiTokenCount]gotreesitter.Symbol) bool {
	lexer.Advance(false)

	// If numEmphasisDelimitersLeft is not zero then we already decided that
	// this should be part of an emphasis delimiter run, so interpret it as such.
	if s.numEmphasisDelimitersLeft > 0 {
		if (s.state&mdiStateEmphasisDelimiterIsOpen) != 0 && isValid(mdiTokEmphasisOpenStar) {
			s.state &^= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(syms[mdiTokEmphasisOpenStar])
			s.numEmphasisDelimitersLeft--
			return true
		}
		if isValid(mdiTokEmphasisCloseStar) {
			lexer.SetResultSymbol(syms[mdiTokEmphasisCloseStar])
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
			lexer.SetResultSymbol(syms[mdiTokEmphasisCloseStar])
			return true
		}
		if !nextSymbolWhitespace &&
			(!nextSymbolPunctuation || isValid(mdiTokLastTokenPunctuation) || isValid(mdiTokLastTokenWhitespace)) {
			s.state |= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(syms[mdiTokEmphasisOpenStar])
			return true
		}
	}
	return false
}

// mdiParseTilde handles tilde-based strikethrough delimiters.
func mdiParseTilde(s *mdiState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, isValid func(int) bool, syms *[mdiTokenCount]gotreesitter.Symbol) bool {
	lexer.Advance(false)

	// If numEmphasisDelimitersLeft is not zero then we already decided that
	// this should be part of an emphasis delimiter run, so interpret it as such.
	if s.numEmphasisDelimitersLeft > 0 {
		if (s.state&mdiStateEmphasisDelimiterIsOpen) != 0 && isValid(mdiTokStrikethroughOpen) {
			s.state &^= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(syms[mdiTokStrikethroughOpen])
			s.numEmphasisDelimitersLeft--
			return true
		}
		if isValid(mdiTokStrikethroughClose) {
			lexer.SetResultSymbol(syms[mdiTokStrikethroughClose])
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
			lexer.SetResultSymbol(syms[mdiTokStrikethroughClose])
			return true
		}
		if !nextSymbolWhitespace &&
			(!nextSymbolPunctuation || isValid(mdiTokLastTokenPunctuation) || isValid(mdiTokLastTokenWhitespace)) {
			s.state |= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(syms[mdiTokStrikethroughOpen])
			return true
		}
	}
	return false
}

// mdiParseUnderscore handles underscore-based emphasis delimiters.
func mdiParseUnderscore(s *mdiState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, isValid func(int) bool, syms *[mdiTokenCount]gotreesitter.Symbol) bool {
	lexer.Advance(false)

	// If numEmphasisDelimitersLeft is not zero then we already decided that
	// this should be part of an emphasis delimiter run, so interpret it as such.
	if s.numEmphasisDelimitersLeft > 0 {
		if (s.state&mdiStateEmphasisDelimiterIsOpen) != 0 && isValid(mdiTokEmphasisOpenUnderscore) {
			s.state &^= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(syms[mdiTokEmphasisOpenUnderscore])
			s.numEmphasisDelimitersLeft--
			return true
		}
		if isValid(mdiTokEmphasisCloseUnderscore) {
			lexer.SetResultSymbol(syms[mdiTokEmphasisCloseUnderscore])
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
			lexer.SetResultSymbol(syms[mdiTokEmphasisCloseUnderscore])
			return true
		}
		if !nextSymbolWhitespace &&
			(!nextSymbolPunctuation || isValid(mdiTokLastTokenPunctuation) || isValid(mdiTokLastTokenWhitespace)) {
			s.state |= mdiStateEmphasisDelimiterIsOpen
			lexer.SetResultSymbol(syms[mdiTokEmphasisOpenUnderscore])
			return true
		}
	}
	return false
}
