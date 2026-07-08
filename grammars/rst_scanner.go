//go:build !grammar_subset || grammar_subset_rst

package grammars

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// ---------------------------------------------------------------------------
// External token indexes — must match the generated RST grammar.
// ---------------------------------------------------------------------------
const (
	rstTokNewline             = 0  // _newline
	rstTokBlankline           = 1  // _blankline
	rstTokIndent              = 2  // _indent
	rstTokNewlineIndent       = 3  // _newline_indent
	rstTokDedent              = 4  // _dedent
	rstTokOverline            = 5  // adornment (overline)
	rstTokUnderline           = 6  // adornment (underline)
	rstTokTransition          = 7  // transition
	rstTokCharBullet          = 8  // bullet (char)
	rstTokNumericBullet       = 9  // bullet (enum)
	rstTokFieldMark           = 10 // : (field marker start)
	rstTokFieldMarkEnd        = 11 // : (field marker end)
	rstTokLiteralIndented     = 12 // :: (literal block mark)
	rstTokLiteralQuoted       = 13 // :: (definition marker)
	rstTokQuotedLiteralBlock  = 14 // literal_block
	rstTokLineBlockMark       = 15 // | (line block marker)
	rstTokAttributionMark     = 16 // -- (option marker)
	rstTokDoctestBlockMark    = 17 // _doctest_block_mark
	rstTokText                = 18 // text
	rstTokEmphasis            = 19 // emphasis
	rstTokStrong              = 20 // strong
	rstTokInterpretedText     = 21 // interpreted_text (first)
	rstTokInterpretedTextPfx  = 22 // interpreted_text (second)
	rstTokRoleNamePrefix      = 23 // role (prefix)
	rstTokRoleNameSuffix      = 24 // role (suffix)
	rstTokLiteral             = 25 // literal
	rstTokSubstitutionRef     = 26 // substitution_reference
	rstTokInlineTarget        = 27 // inline_target
	rstTokFootnoteRef         = 28 // footnote_reference
	rstTokCitationRef         = 29 // citation_reference
	rstTokReference           = 30 // reference
	rstTokStandaloneHyperlink = 31 // standalone_hyperlink
	rstTokExplicitMarkup      = 32 // .. (directive/comment)
	rstTokFootnoteLabel       = 33 // label (first)
	rstTokCitationLabel       = 34 // label (second)
	rstTokTargetName          = 35 // name
	rstTokAnonymousTarget     = 36 // __ (anonymous target)
	rstTokDirectiveName       = 37 // type
	rstTokSubstitutionMark    = 38 // substitution
	rstTokEmptyComment        = 39 // comment
	rstTokInvalidToken        = 40 // _invalid_token
)

// Concrete grammar symbols emitted via SetResultSymbol.
const (
	rstSymNewline             gotreesitter.Symbol = 11
	rstSymBlankline           gotreesitter.Symbol = 12
	rstSymIndent              gotreesitter.Symbol = 13
	rstSymNewlineIndent       gotreesitter.Symbol = 14
	rstSymDedent              gotreesitter.Symbol = 15
	rstSymOverline            gotreesitter.Symbol = 16
	rstSymUnderline           gotreesitter.Symbol = 17
	rstSymTransition          gotreesitter.Symbol = 18
	rstSymCharBullet          gotreesitter.Symbol = 19
	rstSymNumericBullet       gotreesitter.Symbol = 20
	rstSymFieldMark           gotreesitter.Symbol = 21
	rstSymFieldMarkEnd        gotreesitter.Symbol = 22
	rstSymLiteralIndented     gotreesitter.Symbol = 23
	rstSymLiteralQuoted       gotreesitter.Symbol = 24
	rstSymQuotedLiteralBlock  gotreesitter.Symbol = 25
	rstSymLineBlockMark       gotreesitter.Symbol = 26
	rstSymAttributionMark     gotreesitter.Symbol = 27
	rstSymDoctestBlockMark    gotreesitter.Symbol = 28
	rstSymText                gotreesitter.Symbol = 29
	rstSymEmphasis            gotreesitter.Symbol = 30
	rstSymStrong              gotreesitter.Symbol = 31
	rstSymInterpretedText     gotreesitter.Symbol = 32
	rstSymInterpretedTextPfx  gotreesitter.Symbol = 33
	rstSymRoleNamePrefix      gotreesitter.Symbol = 34
	rstSymRoleNameSuffix      gotreesitter.Symbol = 35
	rstSymLiteral             gotreesitter.Symbol = 36
	rstSymSubstitutionRef     gotreesitter.Symbol = 37
	rstSymInlineTarget        gotreesitter.Symbol = 38
	rstSymFootnoteRef         gotreesitter.Symbol = 39
	rstSymCitationRef         gotreesitter.Symbol = 40
	rstSymReference           gotreesitter.Symbol = 41
	rstSymStandaloneHyperlink gotreesitter.Symbol = 42
	rstSymExplicitMarkup      gotreesitter.Symbol = 43
	rstSymFootnoteLabel       gotreesitter.Symbol = 44
	rstSymCitationLabel       gotreesitter.Symbol = 45
	rstSymTargetName          gotreesitter.Symbol = 46
	rstSymAnonymousTarget     gotreesitter.Symbol = 47
	rstSymDirectiveName       gotreesitter.Symbol = 48
	rstSymSubstitutionMark    gotreesitter.Symbol = 49
	rstSymEmptyComment        gotreesitter.Symbol = 50
	rstSymInvalidToken        gotreesitter.Symbol = 51
)

// rstTokToSym maps external token indexes to grammar symbol IDs.
var rstTokToSym = [41]gotreesitter.Symbol{
	rstSymNewline,
	rstSymBlankline,
	rstSymIndent,
	rstSymNewlineIndent,
	rstSymDedent,
	rstSymOverline,
	rstSymUnderline,
	rstSymTransition,
	rstSymCharBullet,
	rstSymNumericBullet,
	rstSymFieldMark,
	rstSymFieldMarkEnd,
	rstSymLiteralIndented,
	rstSymLiteralQuoted,
	rstSymQuotedLiteralBlock,
	rstSymLineBlockMark,
	rstSymAttributionMark,
	rstSymDoctestBlockMark,
	rstSymText,
	rstSymEmphasis,
	rstSymStrong,
	rstSymInterpretedText,
	rstSymInterpretedTextPfx,
	rstSymRoleNamePrefix,
	rstSymRoleNameSuffix,
	rstSymLiteral,
	rstSymSubstitutionRef,
	rstSymInlineTarget,
	rstSymFootnoteRef,
	rstSymCitationRef,
	rstSymReference,
	rstSymStandaloneHyperlink,
	rstSymExplicitMarkup,
	rstSymFootnoteLabel,
	rstSymCitationLabel,
	rstSymTargetName,
	rstSymAnonymousTarget,
	rstSymDirectiveName,
	rstSymSubstitutionMark,
	rstSymEmptyComment,
	rstSymInvalidToken,
}

// ---------------------------------------------------------------------------
// Inline markup type bitmask constants.
// ---------------------------------------------------------------------------
const (
	rstIMEmphasis           uint = 1 << 0
	rstIMStrong             uint = 1 << 1
	rstIMInterpretedText    uint = 1 << 2
	rstIMInterpretedTextPfx uint = 1 << 3
	rstIMLiteral            uint = 1 << 4
	rstIMSubstitutionRef    uint = 1 << 5
	rstIMInlineTarget       uint = 1 << 6
	rstIMFootnoteRef        uint = 1 << 7
	rstIMCitationRef        uint = 1 << 8
	rstIMReference          uint = 1 << 9
)

// ---------------------------------------------------------------------------
// Character constants.
// ---------------------------------------------------------------------------
const (
	rstCharEOF            = 0
	rstCharNewline        = '\n'
	rstCharCarriageReturn = '\r'
	rstCharNBSP           = 160

	rstCharSpace    = ' '
	rstCharFormFeed = '\f'
	rstCharTab      = '\t'
	rstCharVertTab  = '\v'
	rstTabStop      = 8
	rstCharEmDash   = 8212
	rstStackMax     = 99
)

// ---------------------------------------------------------------------------
// Character classification helpers.
// ---------------------------------------------------------------------------

func rstIsNewline(c rune) bool {
	return c == rstCharEOF || c == rstCharNewline || c == rstCharCarriageReturn
}

func rstIsSpace(c rune) bool {
	switch c {
	case rstCharSpace, rstCharFormFeed, rstCharTab, rstCharVertTab, rstCharNBSP:
		return true
	}
	return rstIsNewline(c)
}

func rstIsNumber(c rune) bool       { return c >= '0' && c <= '9' }
func rstIsABCLower(c rune) bool     { return c >= 'a' && c <= 'z' }
func rstIsABCUpper(c rune) bool     { return c >= 'A' && c <= 'Z' }
func rstIsABC(c rune) bool          { return rstIsABCLower(c) || rstIsABCUpper(c) }
func rstIsAlphanumeric(c rune) bool { return rstIsABC(c) || rstIsNumber(c) }

func rstIsAdornmentChar(c rune) bool {
	switch c {
	case '!', '"', '#', '$', '%', '&', '\'', '(', ')', '*', '+', ',', '-', '.',
		'/', ':', ';', '<', '=', '>', '?', '@', '[', '\\', ']', '^', '_', '`',
		'{', '|', '}', '~':
		return true
	}
	return false
}

// rstStartChars, rstEndChars, rstDelimChars, rstDelimRanges are direct
// translations from punctuation_chars.h.
var rstStartChars = [...]rune{
	'"', '\'', '(', '<', '\\', '[', '{',
	0xf3a, 0xf3c, 0x169b, 0x2045, 0x207d, 0x208d, 0x2329,
	0x2768, 0x276a, 0x276c, 0x276e, 0x2770, 0x2772, 0x2774,
	0x27c5, 0x27e6, 0x27e8, 0x27ea, 0x27ec, 0x27ee,
	0x2983, 0x2985, 0x2987, 0x2989, 0x298b, 0x298d, 0x298f,
	0x2991, 0x2993, 0x2995, 0x2997, 0x29d8, 0x29da, 0x29fc,
	0x2e22, 0x2e24, 0x2e26, 0x2e28,
	0x3008, 0x300a, 0x300c, 0x300e, 0x3010, 0x3014, 0x3016, 0x3018, 0x301a, 0x301d, 0x301d,
	0xfd3e, 0xfe17, 0xfe35, 0xfe37, 0xfe39, 0xfe3b, 0xfe3d, 0xfe3f, 0xfe41, 0xfe43, 0xfe47,
	0xfe59, 0xfe5b, 0xfe5d,
	0xff08, 0xff3b, 0xff5b, 0xff5f, 0xff62,
	0xab, 0x2018, 0x201c, 0x2039, 0x2e02, 0x2e04, 0x2e09, 0x2e0c, 0x2e1c, 0x2e20,
	0x201a, 0x201e,
	0xbb, 0x2019, 0x201d, 0x203a, 0x2e03, 0x2e05, 0x2e0a, 0x2e0d, 0x2e1d, 0x2e21,
	0x201b, 0x201f,
}

var rstDelimChars = [...]rune{
	'\\', '-', '/', ':',
	0x58a, 0xa1, 0xb7, 0xbf, 0x37e, 0x387, 0x55f, 0x589, 0x5be, 0x5c0, 0x5c3, 0x5c6,
	0x5f3, 0x5f4, 0x609, 0x60a, 0x60c, 0x60d, 0x61b, 0x61e, 0x61f, 0x66d, 0x6d4, 0x70d,
	0x7f9, 0x83e, 0x964, 0x965, 0x970, 0xdf4, 0xe4f, 0xe5a, 0xe5b, 0xf12, 0xf85, 0xfd4,
	0x104f, 0x10fb, 0x1368, 0x1400, 0x166d, 0x166e, 0x16ed, 0x1735, 0x1736, 0x17d6, 0x17da,
	0x180a, 0x1944, 0x1945, 0x19de, 0x19df, 0x1a1e, 0x1a1f, 0x1aa6, 0x1aad, 0x1b60, 0x1c3f,
	0x1c7e, 0x1c7f, 0x1cd3, 0x2017, 0x2027, 0x2038, 0x203e, 0x2043, 0x2051, 0x2053, 0x205e,
	0x2cfc, 0x2cfe, 0x2cff, 0x2e00, 0x2e01, 0x2e08, 0x2e0b, 0x2e1b, 0x2e1e, 0x2e1f, 0x2e2e,
	0x2e30, 0x2e31, 0x3003, 0x301c, 0x3030, 0x303d, 0x30a0, 0x30fb,
	0xa4fe, 0xa4ff, 0xa60f, 0xa673, 0xa67e, 0xa6f7, 0xa877, 0xa8ce, 0xa8cf, 0xa8fa,
	0xa92e, 0xa92f, 0xa95f, 0xa9cd, 0xa9de, 0xa9df, 0xaa5f, 0xaade, 0xaadf, 0xabeb,
	0xfe16, 0xfe19, 0xfe32, 0xfe45, 0xfe46, 0xfe4c, 0xfe52, 0xfe58, 0xfe61, 0xfe63,
	0xfe68, 0xfe6a, 0xfe6b,
	0xff03, 0xff07, 0xff0a, 0xff0f, 0xff1a, 0xff1b, 0xff1f, 0xff20, 0xff3c, 0xff61, 0xff64, 0xff65,
	0x10100, 0x10101, 0x1039f, 0x103d0, 0x10857, 0x1091f, 0x1093f, 0x10a58, 0x10a7f, 0x10b3f,
	0x110bb, 0x110bc, 0x110c1, 0x12473,
}

var rstDelimRanges = [][2]rune{
	{0x55a, 0x55f}, {0x66a, 0x66d}, {0x700, 0x70d}, {0x7f7, 0x7f9},
	{0x830, 0x83e}, {0xf04, 0xf12}, {0xfd0, 0xfd4}, {0x104a, 0x104f},
	{0x1361, 0x1368}, {0x16eb, 0x16ed}, {0x17d4, 0x17d6}, {0x17d8, 0x17da},
	{0x1800, 0x180a}, {0x1aa0, 0x1aa6}, {0x1aa8, 0x1aad}, {0x1b5a, 0x1b60},
	{0x1c3b, 0x1c3f}, {0x2010, 0x2017}, {0x2020, 0x2027}, {0x2030, 0x2038},
	{0x203b, 0x203e}, {0x2041, 0x2043}, {0x2047, 0x2051}, {0x2055, 0x205e},
	{0x2cf9, 0x2cfc}, {0x2e06, 0x2e08}, {0x2e0e, 0x2e1b}, {0x2e2a, 0x2e2e},
	{0x3001, 0x3003}, {0xa60d, 0xa60f}, {0xa6f2, 0xa6f7}, {0xa874, 0xa877},
	{0xa8f8, 0xa8fa}, {0xa9c1, 0xa9cd}, {0xaa5c, 0xaa5f},
	{0xfe10, 0xfe16}, {0xfe30, 0xfe32}, {0xfe49, 0xfe4c}, {0xfe50, 0xfe52},
	{0xfe54, 0xfe58}, {0xfe5f, 0xfe61},
	{0xff01, 0xff03}, {0xff05, 0xff07}, {0xff0c, 0xff0f},
	{0x10a50, 0x10a58}, {0x10b39, 0x10b3f}, {0x110be, 0x110c1}, {0x12470, 0x12473},
}

var rstEndChars = [...]rune{
	'\\', '\\', '.', ',', ';', '!', '?', '"', '\'', ')', '>', '\\', ']', '}',
	0xf3b, 0xf3d, 0x169c, 0x2046, 0x207e, 0x208e, 0x232a,
	0x2769, 0x276b, 0x276d, 0x276f, 0x2771, 0x2773, 0x2775,
	0x27c6, 0x27e7, 0x27e9, 0x27eb, 0x27ed, 0x27ef,
	0x2984, 0x2986, 0x2988, 0x298a, 0x298c, 0x298e, 0x2990,
	0x2992, 0x2994, 0x2996, 0x2998, 0x29d9, 0x29db, 0x29fd,
	0x2e23, 0x2e25, 0x2e27, 0x2e29,
	0x3009, 0x300b, 0x300d, 0x300f, 0x3011, 0x3015, 0x3017, 0x3019, 0x301b, 0x301e, 0x301f,
	0xfd3f, 0xfe18, 0xfe36, 0xfe38, 0xfe3a, 0xfe3c, 0xfe3e, 0xfe40, 0xfe42, 0xfe44, 0xfe48,
	0xfe5a, 0xfe5c, 0xfe5e,
	0xff09, 0xff3d, 0xff5d, 0xff60, 0xff63,
	0xbb, 0x2019, 0x201d, 0x203a, 0x2e03, 0x2e05, 0x2e0a, 0x2e0d, 0x2e1d, 0x2e21,
	0x201b, 0x201f,
	0xab, 0x2018, 0x201c, 0x2039, 0x2e02, 0x2e04, 0x2e09, 0x2e0c, 0x2e1c, 0x2e20,
	0x201a, 0x201e,
}

func rstIsDelimChar(c rune) bool {
	for _, d := range rstDelimChars {
		if c == d {
			return true
		}
	}
	for _, r := range rstDelimRanges {
		if c >= r[0] && c <= r[1] {
			return true
		}
	}
	return false
}

func rstIsStartChar(c rune) bool {
	for _, s := range rstStartChars {
		if c == s {
			return true
		}
	}
	return rstIsDelimChar(c)
}

func rstIsEndChar(c rune) bool {
	for _, e := range rstEndChars {
		if c == e {
			return true
		}
	}
	return rstIsDelimChar(c)
}

func rstIsInlineMarkupStartChar(c rune) bool {
	switch c {
	case '*', '`', '|', '_', '[':
		return true
	}
	return false
}

func rstIsInlineMarkupEndChar(c rune) bool {
	switch c {
	case '*', '`', '|', ']':
		return true
	}
	return false
}

func rstIsInternalReferenceChar(c rune) bool {
	switch c {
	case '-', '_', '.', ':', '+':
		return true
	}
	return false
}

func rstIsCharBullet(c rune) bool {
	switch c {
	case '*', '+', '-', 8226, 8227, 8259:
		return true
	}
	return false
}

func rstIsNumericBullet(c rune) bool {
	return rstIsNumericBulletSimple(c) ||
		rstIsNumericBulletRomanLower(c) ||
		rstIsNumericBulletRomanUpper(c) ||
		rstIsNumericBulletABCLower(c) ||
		rstIsNumericBulletABCUpper(c)
}

func rstIsNumericBulletSimple(c rune) bool { return rstIsNumber(c) || c == '#' }

func rstIsNumericBulletRomanLower(c rune) bool {
	switch c {
	case 'i', 'v', 'x', 'l', 'c', 'd', 'm':
		return true
	}
	return false
}

func rstIsNumericBulletRomanUpper(c rune) bool {
	switch c {
	case 'I', 'V', 'X', 'L', 'C', 'D', 'M':
		return true
	}
	return false
}

func rstIsNumericBulletABCLower(c rune) bool { return rstIsABCLower(c) }
func rstIsNumericBulletABCUpper(c rune) bool { return rstIsABCUpper(c) }

func rstIsAttributionMark(c rune) bool {
	return c == '-' || c == rstCharEmDash
}

func rstIsKnownSchema(s string) bool {
	switch s {
	case "http", "https", "ftp", "mailto", "telnet", "ssh":
		return true
	}
	return false
}

func rstIsInvalidURIChar(c rune) bool {
	switch c {
	case '^', '}', '{', '\\':
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Scanner state.
// ---------------------------------------------------------------------------

type rstScannerState struct {
	indentStack []int
}

func (s *rstScannerState) push(v int) {
	if len(s.indentStack) >= rstStackMax {
		return
	}
	s.indentStack = append(s.indentStack, v)
}

func (s *rstScannerState) pop() int {
	if len(s.indentStack) == 0 {
		return 0
	}
	v := s.indentStack[len(s.indentStack)-1]
	s.indentStack = s.indentStack[:len(s.indentStack)-1]
	return v
}

func (s *rstScannerState) back() int {
	if len(s.indentStack) == 0 {
		return 0
	}
	return s.indentStack[len(s.indentStack)-1]
}

// ---------------------------------------------------------------------------
// rstCtx bundles scanner state + lexer + valid_symbols to reduce parameter
// passing throughout the port. It also provides advance/skip helpers that
// track the previous rune, mirroring the C scanner's lookahead/previous pair.
// ---------------------------------------------------------------------------

type rstCtx struct {
	state        *rstScannerState
	lexer        *gotreesitter.ExternalLexer
	validSymbols []bool
	lookahead    rune
	previous     rune
	// resultTok is the token index that will be emitted (we convert to Symbol
	// at the very end).
	resultTok int
}

func (r *rstCtx) advance() {
	r.previous = r.lookahead
	r.lexer.Advance(false)
	// Skip \r in \r\n (mirrors the C scanner).
	if r.lexer.Lookahead() == rstCharCarriageReturn {
		r.lexer.Advance(false)
	}
	r.lookahead = r.lexer.Lookahead()
}

func (r *rstCtx) skip() {
	r.previous = r.lookahead
	r.lexer.Advance(true)
	r.lookahead = r.lexer.Lookahead()
}

func (r *rstCtx) valid(tok int) bool {
	return tok >= 0 && tok < len(r.validSymbols) && r.validSymbols[tok]
}

func (r *rstCtx) setResult(tok int) {
	r.resultTok = tok
	r.lexer.SetResultSymbol(rstTokToSym[tok])
}

func (r *rstCtx) markEnd() {
	r.lexer.MarkEnd()
}

// getIndentLevel consumes whitespace and returns the indent width.
func (r *rstCtx) getIndentLevel() int {
	indent := 0
	for {
		c := r.lookahead
		if c == rstCharSpace || c == rstCharVertTab || c == rstCharFormFeed {
			indent++
		} else if c == rstCharTab {
			indent += rstTabStop
		} else {
			break
		}
		r.advance()
	}
	return indent
}

// ---------------------------------------------------------------------------
// RstExternalScanner — implements gotreesitter.ExternalScanner.
// ---------------------------------------------------------------------------

type RstExternalScanner struct{}

func (RstExternalScanner) Create() any {
	return &rstScannerState{}
}

func (RstExternalScanner) Destroy(payload any) {}

func (RstExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*rstScannerState)
	n := len(s.indentStack)
	if n > len(buf) {
		n = len(buf)
	}
	for i := 0; i < n; i++ {
		buf[i] = byte(s.indentStack[i])
	}
	return n
}

func (RstExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*rstScannerState)
	s.indentStack = s.indentStack[:0]
	if len(buf) > 0 {
		// NOTE: The C scanner has a bug in deserialize (copies src→dst reversed),
		// but we faithfully replicate the data format: each byte is one int indent value.
		for _, b := range buf {
			s.indentStack = append(s.indentStack, int(b))
		}
	}
}

func (RstExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*rstScannerState)
	r := &rstCtx{
		state:        s,
		lexer:        lexer,
		validSymbols: validSymbols,
		lookahead:    lexer.Lookahead(),
		previous:     lexer.Lookahead(),
	}

	current := r.lookahead

	// Error recovery mode — if T_INVALID_TOKEN is valid, tree-sitter is in
	// correction mode; fall back to text node.
	if r.valid(rstTokInvalidToken) {
		if !rstIsSpace(current) && r.valid(rstTokText) {
			return r.parseText(true)
		}
		return false
	}

	if rstIsAdornmentChar(current) && (r.valid(rstTokOverline) || r.valid(rstTokTransition)) {
		return r.parseOverline()
	}
	if rstIsAdornmentChar(current) && (r.valid(rstTokUnderline) || r.valid(rstTokTransition)) {
		return r.parseUnderline()
	}
	if rstIsAdornmentChar(current) && r.valid(rstTokQuotedLiteralBlock) {
		return r.parseQuotedLiteralBlock()
	}
	if current == '.' && r.valid(rstTokExplicitMarkup) {
		return r.parseExplicitMarkupStart()
	}
	if rstIsAttributionMark(current) && r.valid(rstTokAttributionMark) {
		return r.parseAttributionMark()
	}
	if current == '[' && (r.valid(rstTokFootnoteLabel) || r.valid(rstTokCitationLabel)) {
		return r.parseLabel()
	}
	if current == '_' && r.valid(rstTokTargetName) {
		return r.parseTargetName()
	}
	if current == '_' && r.valid(rstTokAnonymousTarget) {
		return r.parseAnonymousTargetMark()
	}
	if current == '|' && r.valid(rstTokSubstitutionMark) {
		return r.parseSubstitutionMark()
	}
	if current == '|' && r.valid(rstTokLineBlockMark) {
		return r.parseLineBlockMark()
	}
	if current == '>' && r.valid(rstTokDoctestBlockMark) {
		return r.parseDoctestBlockMark()
	}
	if rstIsAlphanumeric(current) && r.valid(rstTokDirectiveName) {
		return r.parseDirectiveName()
	}
	if rstIsInlineMarkupStartChar(current) &&
		(r.valid(rstTokEmphasis) || r.valid(rstTokStrong) ||
			r.valid(rstTokInterpretedText) || r.valid(rstTokInterpretedTextPfx) ||
			r.valid(rstTokLiteral) || r.valid(rstTokSubstitutionRef) ||
			r.valid(rstTokInlineTarget) || r.valid(rstTokFootnoteRef) ||
			r.valid(rstTokCitationRef) || r.valid(rstTokReference)) {
		return r.parseInlineMarkup()
	}
	if (rstIsNumericBullet(current) || current == '(') && r.valid(rstTokNumericBullet) {
		return r.parseNumericBullet()
	}
	if rstIsCharBullet(current) && r.valid(rstTokCharBullet) {
		return r.parseCharBullet()
	}
	if current == ':' && (r.valid(rstTokLiteralIndented) || r.valid(rstTokLiteralQuoted)) {
		return r.parseLiteralBlockMark()
	}
	if current == ':' && (r.valid(rstTokRoleNamePrefix) || r.valid(rstTokRoleNameSuffix)) {
		return r.parseRole()
	}
	if current == ':' && r.valid(rstTokFieldMark) {
		return r.parseFieldMark()
	}
	if current == ':' && r.valid(rstTokFieldMarkEnd) {
		return r.parseFieldMarkEnd()
	}
	if rstIsABC(current) && r.valid(rstTokStandaloneHyperlink) {
		return r.parseStandaloneHyperlink()
	}
	if !rstIsSpace(current) && !rstIsInternalReferenceChar(current) &&
		!rstIsStartChar(current) && !rstIsEndChar(current) && r.valid(rstTokReference) {
		return r.parseReference()
	}
	if !rstIsSpace(current) && r.valid(rstTokText) {
		return r.parseText(true)
	}
	if rstIsSpace(current) {
		return r.parseIndent()
	}
	return false
}

// ---------------------------------------------------------------------------
// Parsing functions.
// ---------------------------------------------------------------------------

func (r *rstCtx) parseIndent() bool {
	r.markEnd()
	indent := 0
	newlines := 0
	for {
		switch r.lookahead {
		case rstCharSpace, rstCharVertTab, rstCharFormFeed, rstCharNBSP:
			indent++
		case rstCharTab:
			indent += rstTabStop
		case rstCharEOF:
			indent = 0
			newlines++
			goto done
		case rstCharCarriageReturn:
			indent = 0
		case rstCharNewline:
			newlines++
			indent = 0
		default:
			goto done
		}
		r.skip()
	}
done:
	currentIndent := r.state.back()
	if indent > currentIndent && r.valid(rstTokIndent) {
		r.state.push(indent)
		r.setResult(rstTokIndent)
		return true
	}
	if newlines > 0 {
		if indent < currentIndent && r.valid(rstTokDedent) {
			r.state.pop()
			r.setResult(rstTokDedent)
			return true
		}
		if (newlines > 1 || r.lookahead == rstCharEOF) && r.valid(rstTokBlankline) {
			r.setResult(rstTokBlankline)
			return true
		}
		if newlines == 1 && r.valid(rstTokNewlineIndent) && indent > currentIndent {
			r.state.push(indent)
			r.setResult(rstTokNewlineIndent)
			return true
		}
		if r.valid(rstTokNewline) {
			r.setResult(rstTokNewline)
			return true
		}
	}
	return false
}

func (r *rstCtx) parseOverline() bool {
	adornment := r.lookahead
	if !rstIsAdornmentChar(adornment) || (!r.valid(rstTokOverline) && !r.valid(rstTokTransition)) {
		return false
	}

	r.advance()
	r.markEnd()
	length := 1

	for {
		if r.lookahead != adornment {
			ok := r.fallbackAdornment(adornment, length)
			if ok {
				return true
			}
			if rstIsSpace(r.lookahead) {
				break
			}
			return r.parseText(false)
		}
		r.advance()
		length++
	}

	// Mark transition token.
	r.markEnd()

	// Consume trailing whitespace.
	for rstIsSpace(r.lookahead) && !rstIsNewline(r.lookahead) {
		r.advance()
	}
	if !rstIsNewline(r.lookahead) {
		return r.parseText(false)
	}
	r.advance()

	// Check next line — is it empty?
	isEmpty := true
	for !rstIsNewline(r.lookahead) {
		if !rstIsSpace(r.lookahead) {
			isEmpty = false
		}
		r.advance()
	}

	if isEmpty {
		if length >= 4 && r.valid(rstTokTransition) {
			r.setResult(rstTokTransition)
			return true
		}
		return r.parseText(false)
	}

	r.advance()

	// Check underline on the third line.
	ulLength := 0
	for !rstIsNewline(r.lookahead) {
		if r.lookahead != adornment {
			if rstIsSpace(r.lookahead) {
				break
			}
			return r.parseText(false)
		}
		r.advance()
		ulLength++
	}

	// Consume trailing whitespace.
	for rstIsSpace(r.lookahead) && !rstIsNewline(r.lookahead) {
		r.advance()
	}
	if !rstIsNewline(r.lookahead) {
		return r.parseText(false)
	}

	if length >= 1 && length == ulLength {
		r.setResult(rstTokOverline)
		return true
	}
	return r.parseText(false)
}

func (r *rstCtx) parseUnderline() bool {
	adornment := r.lookahead
	if !rstIsAdornmentChar(adornment) || (!r.valid(rstTokUnderline) && !r.valid(rstTokTransition)) {
		return false
	}

	r.advance()
	r.markEnd()
	length := 1

	for !rstIsNewline(r.lookahead) {
		if r.lookahead != adornment {
			ok := r.fallbackAdornment(adornment, length)
			if ok {
				return true
			}
			if rstIsSpace(r.lookahead) {
				break
			}
			return r.parseText(false)
		}
		r.advance()
		length++
	}

	r.markEnd()

	// Consume trailing whitespace.
	for rstIsSpace(r.lookahead) && !rstIsNewline(r.lookahead) {
		r.advance()
	}
	if !rstIsNewline(r.lookahead) {
		return r.parseText(false)
	}

	if length >= 4 && r.valid(rstTokTransition) {
		r.setResult(rstTokTransition)
		return true
	}
	if length >= 1 && r.valid(rstTokUnderline) {
		r.setResult(rstTokUnderline)
		return true
	}
	return r.parseText(false)
}

func (r *rstCtx) fallbackAdornment(adornment rune, adornmentLength int) bool {
	if adornmentLength == 1 {
		if rstIsSpace(r.lookahead) {
			if rstIsCharBullet(adornment) && r.valid(rstTokCharBullet) {
				if r.parseInnerListElement(1, rstTokCharBullet) {
					return true
				}
			} else if adornment == '|' && r.valid(rstTokLineBlockMark) {
				if r.parseInnerListElement(1, rstTokLineBlockMark) {
					return true
				}
			}
		} else {
			if adornment == '*' && r.valid(rstTokEmphasis) {
				return r.parseInnerInlineMarkup(rstIMEmphasis)
			}
			if adornment == ':' && (r.valid(rstTokRoleNamePrefix) || r.valid(rstTokRoleNameSuffix)) {
				if r.parseInnerRole() {
					return true
				}
				return r.parseText(false)
			}
			if adornment == ':' && r.valid(rstTokFieldMark) {
				if r.parseInnerFieldMark() {
					return true
				}
				return r.parseText(false)
			}
			if adornment == '`' && (r.valid(rstTokInterpretedText) || r.valid(rstTokInterpretedTextPfx) || r.valid(rstTokReference)) {
				return r.parseInnerInlineMarkup(rstIMInterpretedText | rstIMInterpretedTextPfx | rstIMReference)
			}
			if adornment == '|' && r.valid(rstTokSubstitutionRef) {
				return r.parseInnerInlineMarkup(rstIMSubstitutionRef)
			}
			if adornment == '_' && r.lookahead == '`' && r.valid(rstTokInlineTarget) {
				return r.parseInnerInlineMarkup(rstIMInlineTarget)
			}
			if adornment == '[' && (r.valid(rstTokFootnoteRef) || r.valid(rstTokCitationRef)) {
				return r.parseInnerInlineMarkup(rstIMFootnoteRef | rstIMCitationRef)
			}
			if adornment == '#' && (r.lookahead == '.' || r.lookahead == ')') {
				r.advance()
				if r.parseInnerListElement(2, rstTokNumericBullet) {
					return true
				}
			}
			if adornment == '(' && rstIsNumericBullet(r.lookahead) && r.valid(rstTokNumericBullet) {
				return r.parseInnerNumericBullet(true)
			}
		}
	} else if adornmentLength >= 2 {
		if rstIsSpace(r.lookahead) {
			if adornmentLength == 3 && adornment == '>' && r.valid(rstTokDoctestBlockMark) {
				r.markEnd()
				r.setResult(rstTokDoctestBlockMark)
				return true
			}
			if adornmentLength == 2 && adornment == '.' {
				return r.parseInnerListElement(2, rstTokExplicitMarkup)
			}
			if adornmentLength == 2 && adornment == '_' && r.valid(rstTokAnonymousTarget) {
				r.markEnd()
				r.setResult(rstTokAnonymousTarget)
				return true
			}
			if adornmentLength == 2 && adornment == ':' &&
				(r.valid(rstTokLiteralIndented) || r.valid(rstTokLiteralQuoted)) {
				return r.parseInnerLiteralBlockMark()
			}
		} else {
			if adornment == '*' && r.valid(rstTokStrong) {
				return r.parseInnerInlineMarkup(rstIMStrong)
			}
			if adornment == '`' && r.valid(rstTokLiteral) {
				return r.parseInnerInlineMarkup(rstIMLiteral)
			}
			if adornment == '|' && r.valid(rstTokSubstitutionRef) {
				return r.parseInnerInlineMarkup(rstIMSubstitutionRef)
			}
			if adornment == '[' && (r.valid(rstTokFootnoteRef) || r.valid(rstTokCitationRef)) {
				return r.parseInnerInlineMarkup(rstIMFootnoteRef | rstIMCitationRef)
			}
		}
	}
	return false
}

func (r *rstCtx) parseCharBullet() bool {
	if !rstIsCharBullet(r.lookahead) || !r.valid(rstTokCharBullet) {
		return false
	}
	r.advance()
	if r.parseInnerListElement(1, rstTokCharBullet) {
		return true
	}
	return r.parseText(true)
}

func (r *rstCtx) parseNumericBullet() bool {
	if !r.valid(rstTokNumericBullet) {
		return false
	}
	parenthesized := false
	if r.lookahead == '(' {
		r.advance()
		parenthesized = true
	}
	if rstIsNumericBullet(r.lookahead) {
		return r.parseInnerNumericBullet(parenthesized)
	}
	return false
}

func (r *rstCtx) parseInnerNumericBullet(parenthesized bool) bool {
	if !rstIsNumericBullet(r.lookahead) || !r.valid(rstTokNumericBullet) {
		return false
	}

	r.advance()
	consumedChars := 1

	if rstIsNumericBulletSimple(r.previous) {
		for rstIsNumericBulletSimple(r.lookahead) && r.lookahead != '#' {
			r.advance()
			consumedChars++
		}
	} else if rstIsNumericBulletABCLower(r.previous) {
		if rstIsNumericBulletRomanLower(r.previous) {
			for rstIsNumericBulletRomanLower(r.lookahead) {
				r.advance()
				consumedChars++
			}
		}
	} else if rstIsNumericBulletABCUpper(r.previous) {
		if rstIsNumericBulletRomanUpper(r.previous) {
			for rstIsNumericBulletRomanUpper(r.lookahead) {
				r.advance()
				consumedChars++
			}
		}
	} else {
		return false
	}

	if (parenthesized && r.lookahead == ')') ||
		(!parenthesized && (r.lookahead == '.' || r.lookahead == ')')) {
		r.advance()
		consumedChars++
		if parenthesized {
			consumedChars++
		}
		if r.parseInnerListElement(consumedChars, rstTokNumericBullet) {
			return true
		}
	} else {
		if rstIsABC(r.lookahead) && r.valid(rstTokStandaloneHyperlink) {
			return r.parseInnerStandaloneHyperlink()
		}
		if rstIsAlphanumeric(r.lookahead) && r.valid(rstTokReference) {
			return r.parseReference()
		}
		if r.valid(rstTokText) {
			r.markEnd()
			r.setResult(rstTokText)
			return true
		}
		return false
	}
	return r.parseText(true)
}

func (r *rstCtx) parseExplicitMarkupStart() bool {
	if r.lookahead != '.' || !r.valid(rstTokExplicitMarkup) {
		return false
	}
	r.advance()
	if r.lookahead != '.' {
		return false
	}
	r.advance()
	return r.parseInnerListElement(2, rstTokExplicitMarkup)
}

func (r *rstCtx) parseInnerListElement(consumedChars int, tokenType int) bool {
	if !r.valid(tokenType) {
		return false
	}

	if rstIsSpace(r.lookahead) {
		r.markEnd()
		r.setResult(tokenType)

		// Set indent level to the first non-whitespace char.
		indent := r.state.back() + consumedChars + r.getIndentLevel()

		if rstIsNewline(r.lookahead) && tokenType == rstTokExplicitMarkup {
			// Check for empty comment.
			isEmpty := true
			r.advance()
			for !rstIsNewline(r.lookahead) {
				if !rstIsSpace(r.lookahead) {
					isEmpty = false
					break
				}
				r.advance()
			}
			if isEmpty && r.valid(rstTokEmptyComment) {
				r.setResult(rstTokEmptyComment)
				return true
			}
		} else if tokenType == rstTokExplicitMarkup {
			// Go to the next line.
			for !rstIsNewline(r.lookahead) {
				r.advance()
			}
			r.advance()
			// The first non-empty line after the marker determines indentation.
			for {
				indent = r.getIndentLevel()
				if !rstIsNewline(r.lookahead) || r.lookahead == rstCharEOF {
					break
				}
				r.advance()
			}
			if indent <= r.state.back() {
				indent = r.state.back() + 1
			}
		} else if tokenType == rstTokNumericBullet {
			// Check if the next line is an underline.
			consumedChars = indent
			for !rstIsNewline(r.lookahead) {
				consumedChars++
				r.advance()
			}
			r.advance()

			adornment := r.lookahead
			adornmentLength := 0
			if rstIsAdornmentChar(adornment) {
				for {
					if rstIsNewline(r.lookahead) {
						break
					}
					if r.lookahead != adornment {
						adornmentLength = -1
						break
					}
					adornmentLength++
					r.advance()
				}
			}
			if adornmentLength > 0 && adornmentLength >= consumedChars {
				return r.parseText(false)
			}
		}

		r.state.push(indent)
		return true
	}
	return false
}

func (r *rstCtx) parseFieldMark() bool {
	if r.lookahead != ':' || !r.valid(rstTokFieldMark) {
		return false
	}
	r.advance()
	r.markEnd()
	if rstIsSpace(r.lookahead) {
		return r.parseText(true)
	}
	if r.parseInnerFieldMark() {
		return true
	}
	return r.parseText(false)
}

func (r *rstCtx) parseInnerFieldMark() bool {
	if !r.valid(rstTokFieldMark) {
		return false
	}
	isEscaped := false
	for !rstIsNewline(r.lookahead) {
		if r.lookahead == '/' {
			r.advance()
			isEscaped = true
		} else {
			isEscaped = false
		}
		if r.lookahead == ':' && !rstIsSpace(r.previous) && !isEscaped {
			r.advance()
			if rstIsSpace(r.lookahead) {
				break
			}
		}
		r.advance()
	}
	if r.previous == ':' && rstIsSpace(r.lookahead) {
		r.setResult(rstTokFieldMark)
		return true
	}
	return false
}

func (r *rstCtx) parseFieldMarkEnd() bool {
	if r.lookahead != ':' || !r.valid(rstTokFieldMarkEnd) {
		return false
	}
	r.advance()
	r.markEnd()

	if rstIsSpace(r.lookahead) {
		// Consume all whitespace.
		r.getIndentLevel()
		// Go to the next line.
		for !rstIsNewline(r.lookahead) {
			r.advance()
		}
		r.advance()

		// The first non-empty line after the field name marker
		// determines the indentation of the field body.
		indent := 0
		for {
			indent = r.getIndentLevel()
			if !rstIsNewline(r.lookahead) || r.lookahead == rstCharEOF {
				break
			}
			r.advance()
		}
		if indent > r.state.back() {
			r.state.push(indent)
		} else {
			r.state.push(r.state.back() + 1)
		}
		r.setResult(rstTokFieldMarkEnd)
		return true
	}
	return r.parseText(false)
}

func (r *rstCtx) parseLabel() bool {
	if r.lookahead != '[' || !(r.valid(rstTokFootnoteLabel) || r.valid(rstTokCitationLabel)) {
		return false
	}
	r.advance()
	labelType := r.parseInnerLabelName()
	if (labelType == int(rstIMCitationRef) && r.valid(rstTokCitationLabel)) ||
		(labelType == int(rstIMFootnoteRef) && r.valid(rstTokFootnoteLabel)) {
		r.advance()
		if rstIsSpace(r.lookahead) {
			r.markEnd()
			if labelType == int(rstIMCitationRef) {
				r.setResult(rstTokCitationLabel)
			} else {
				r.setResult(rstTokFootnoteLabel)
			}
			return true
		}
	}
	return false
}

func (r *rstCtx) parseInnerLabelName() int {
	labelType := -1
	if rstIsNumber(r.lookahead) {
		for rstIsNumber(r.lookahead) {
			r.advance()
		}
		if r.lookahead == ']' {
			labelType = int(rstIMFootnoteRef)
		} else {
			if r.parseInnerAlphanumericLabel() {
				labelType = int(rstIMCitationRef)
			}
		}
	} else if r.lookahead == '*' {
		labelType = int(rstIMFootnoteRef)
		r.advance()
	} else if r.lookahead == '#' {
		r.advance()
		if r.lookahead == ']' {
			labelType = int(rstIMFootnoteRef)
		} else if rstIsAlphanumeric(r.lookahead) {
			if r.parseInnerAlphanumericLabel() {
				labelType = int(rstIMFootnoteRef)
			}
		}
	} else if rstIsAlphanumeric(r.lookahead) {
		if r.parseInnerAlphanumericLabel() {
			labelType = int(rstIMCitationRef)
		}
	} else {
		return -1
	}

	if r.lookahead == ']' {
		return labelType
	}
	return -1
}

func (r *rstCtx) parseInnerAlphanumericLabel() bool {
	if !(rstIsAlphanumeric(r.lookahead) || rstIsInternalReferenceChar(r.lookahead)) {
		return false
	}
	internalSymbol := false
	for rstIsAlphanumeric(r.lookahead) || rstIsInternalReferenceChar(r.lookahead) {
		if rstIsInternalReferenceChar(r.lookahead) {
			if internalSymbol {
				return false
			}
			internalSymbol = true
		} else {
			internalSymbol = false
		}
		r.advance()
	}
	return r.lookahead == ']'
}

func (r *rstCtx) parseTargetName() bool {
	if r.lookahead != '_' || !r.valid(rstTokTargetName) {
		return false
	}
	r.advance()

	if r.lookahead == '_' {
		r.advance()
	} else if r.lookahead == '`' {
		// Find ending "`:".
		for {
			if r.lookahead == '`' {
				r.advance()
				if r.lookahead == ':' {
					break
				}
			}
			if rstIsNewline(r.lookahead) {
				break
			}
			r.advance()
		}
	} else {
		isEscaped := false
		for {
			if r.lookahead == '\\' {
				r.advance()
				isEscaped = true
			} else {
				isEscaped = false
			}
			if rstIsNewline(r.lookahead) {
				break
			}
			if r.lookahead == ':' && !isEscaped {
				break
			}
			r.advance()
		}
	}

	if r.lookahead != ':' {
		return false
	}
	r.advance()
	if rstIsSpace(r.lookahead) {
		r.markEnd()
		r.setResult(rstTokTargetName)
		return true
	}
	return false
}

func (r *rstCtx) parseAnonymousTargetMark() bool {
	if r.lookahead != '_' || !r.valid(rstTokAnonymousTarget) {
		return false
	}
	r.advance()
	if r.lookahead != '_' {
		return false
	}
	r.advance()
	if rstIsSpace(r.lookahead) {
		r.markEnd()
		r.setResult(rstTokAnonymousTarget)
		return true
	}
	return false
}

func (r *rstCtx) parseDirectiveName() bool {
	if !rstIsAlphanumeric(r.lookahead) || !r.valid(rstTokDirectiveName) {
		return false
	}
	r.advance()

	internalSymbol := false
	keepParsing := true
	for rstIsAlphanumeric(r.lookahead) || rstIsInternalReferenceChar(r.lookahead) ||
		(rstIsSpace(r.lookahead) && !rstIsNewline(r.lookahead)) {
		if rstIsSpace(r.lookahead) {
			r.markEnd()
			r.advance()
			r.advance()
			keepParsing = false
			break
		}
		if rstIsInternalReferenceChar(r.lookahead) {
			if internalSymbol {
				break
			}
			internalSymbol = true
			r.markEnd()
		} else {
			internalSymbol = false
		}
		r.advance()
	}

	if r.lookahead != ':' || r.previous != ':' {
		return r.parseText(keepParsing)
	}
	r.advance()
	if rstIsSpace(r.lookahead) {
		r.setResult(rstTokDirectiveName)
		return true
	}
	return false
}

func (r *rstCtx) parseSubstitutionMark() bool {
	if r.lookahead != '|' || !r.valid(rstTokSubstitutionMark) {
		return false
	}
	r.advance()

	if !rstIsSpace(r.lookahead) {
		ok := r.parseInnerInlineMarkup(rstIMSubstitutionRef)
		if ok && r.resultTok == rstTokSubstitutionRef &&
			rstIsSpace(r.lookahead) && !rstIsNewline(r.lookahead) {
			r.setResult(rstTokSubstitutionMark)
			return true
		}
	}
	return false
}

func (r *rstCtx) parseLiteralBlockMark() bool {
	if r.lookahead != ':' || !(r.valid(rstTokLiteralIndented) || r.valid(rstTokLiteralQuoted)) {
		return false
	}
	r.advance()
	if r.lookahead != ':' {
		if r.valid(rstTokRoleNamePrefix) || r.valid(rstTokRoleNameSuffix) {
			return r.parseInnerRole()
		}
		return false
	}
	r.advance()
	return r.parseInnerLiteralBlockMark()
}

func (r *rstCtx) parseInnerLiteralBlockMark() bool {
	if !rstIsSpace(r.lookahead) || !(r.valid(rstTokLiteralIndented) || r.valid(rstTokLiteralQuoted)) {
		return false
	}
	r.markEnd()

	// Consume trailing whitespace.
	for rstIsSpace(r.lookahead) && !rstIsNewline(r.lookahead) {
		r.advance()
	}
	if !rstIsNewline(r.lookahead) {
		return r.parseText(false)
	}
	r.advance()

	// Ensure blank line.
	for !rstIsNewline(r.lookahead) {
		if !rstIsSpace(r.lookahead) {
			return false
		}
		r.advance()
	}
	r.advance()

	// Skip whitespace/newlines, get indent of first non-empty line.
	indent := -1
	for r.lookahead != rstCharEOF {
		localIndent := r.getIndentLevel()
		if !rstIsNewline(r.lookahead) {
			indent = localIndent
			break
		}
		r.advance()
	}

	if indent > r.state.back() {
		r.state.push(r.state.back() + 1)
		r.setResult(rstTokLiteralIndented)
	} else if indent == r.state.back() && rstIsAdornmentChar(r.lookahead) {
		r.setResult(rstTokLiteralQuoted)
	} else {
		return false
	}
	if !r.valid(r.resultTok) {
		return false
	}
	return true
}

func (r *rstCtx) parseQuotedLiteralBlock() bool {
	if !rstIsAdornmentChar(r.lookahead) || !r.valid(rstTokQuotedLiteralBlock) {
		return false
	}
	adornment := r.lookahead
	currentIndent := r.state.back()

	for {
		for !rstIsNewline(r.lookahead) {
			r.advance()
		}
		r.markEnd()
		r.advance()
		indent := r.getIndentLevel()
		if indent != currentIndent || r.lookahead != adornment {
			break
		}
		if r.lookahead != adornment {
			return r.parseText(false)
		}
	}
	r.setResult(rstTokQuotedLiteralBlock)
	return true
}

func (r *rstCtx) parseLineBlockMark() bool {
	if r.lookahead != '|' || !r.valid(rstTokLineBlockMark) {
		return false
	}
	r.advance()
	if rstIsSpace(r.lookahead) {
		return r.parseInnerListElement(1, rstTokLineBlockMark)
	}
	return false
}

func (r *rstCtx) parseAttributionMark() bool {
	if !rstIsAttributionMark(r.lookahead) || !r.valid(rstTokAttributionMark) {
		return false
	}
	consumedChars := 0
	if r.lookahead == '-' {
		for r.lookahead == '-' {
			consumedChars++
			r.advance()
		}
		if consumedChars < 2 || consumedChars > 3 {
			return false
		}
	} else {
		r.advance()
		consumedChars++
	}
	return r.parseInnerListElement(consumedChars, rstTokAttributionMark)
}

func (r *rstCtx) parseDoctestBlockMark() bool {
	if r.lookahead != '>' || !r.valid(rstTokDoctestBlockMark) {
		return false
	}
	consumedChars := 0
	for r.lookahead == '>' {
		consumedChars++
		r.advance()
	}
	if consumedChars == 3 && rstIsSpace(r.lookahead) {
		r.markEnd()
		r.setResult(rstTokDoctestBlockMark)
		return true
	}
	return false
}

func (r *rstCtx) parseInlineMarkup() bool {
	r.advance()
	r.markEnd()

	var imType uint

	if r.previous == '*' && r.lookahead == '*' && r.valid(rstTokStrong) {
		imType = rstIMStrong
	} else if r.previous == '*' && r.valid(rstTokEmphasis) {
		imType = rstIMEmphasis
	} else if r.previous == '`' && r.lookahead == '`' && r.valid(rstTokLiteral) {
		imType = rstIMLiteral
	} else if r.previous == '`' && (r.valid(rstTokInterpretedText) || r.valid(rstTokInterpretedTextPfx) || r.valid(rstTokReference)) {
		imType = rstIMInterpretedText | rstIMInterpretedTextPfx | rstIMReference
	} else if r.previous == '|' && r.valid(rstTokSubstitutionRef) {
		imType = rstIMSubstitutionRef
	} else if r.previous == '_' && r.lookahead == '`' && r.valid(rstTokInlineTarget) {
		imType = rstIMInlineTarget
	} else if r.previous == '[' && (r.valid(rstTokFootnoteRef) || r.valid(rstTokCitationRef)) {
		imType = rstIMFootnoteRef | rstIMCitationRef
	}

	// Skip one char for double-char start tokens.
	if imType&(rstIMStrong|rstIMLiteral|rstIMInlineTarget) != 0 {
		r.advance()
	}

	// Next character can't be whitespace.
	if rstIsSpace(r.lookahead) {
		if imType&rstIMEmphasis != 0 {
			if r.parseInnerListElement(1, rstTokCharBullet) {
				return true
			}
		}
		if r.valid(rstTokText) {
			r.markEnd()
			r.setResult(rstTokText)
			return true
		}
		return false
	}

	return r.parseInnerInlineMarkup(imType)
}

func (r *rstCtx) parseInnerInlineMarkup(imType uint) bool {
	consumedChars := 0
	wordFound := false
	isEscaped := false

	if imType&rstIMFootnoteRef != 0 || imType&rstIMCitationRef != 0 {
		finalType := r.parseInnerLabelName()
		if (finalType == int(rstIMFootnoteRef) && imType&rstIMFootnoteRef != 0) ||
			(finalType == int(rstIMCitationRef) && imType&rstIMCitationRef != 0) {
			r.advance()
			if r.lookahead == '_' {
				r.advance()
				if rstIsSpace(r.lookahead) || rstIsEndChar(r.lookahead) {
					r.markEnd()
					if finalType == int(rstIMCitationRef) {
						r.setResult(rstTokCitationRef)
					} else {
						r.setResult(rstTokFootnoteRef)
					}
					return true
				}
			}
		}
		return r.parseText(false)
	}

	for r.lookahead != rstCharEOF {
		// Skip indentation across lines.
		if rstIsNewline(r.lookahead) {
			if !wordFound {
				wordFound = true
				r.markEnd()
			}
			r.advance()
			indent := r.getIndentLevel()
			if indent != r.state.back() || rstIsNewline(r.lookahead) {
				break
			}
		}

		// Skip escaped chars.
		if r.lookahead == '\\' {
			isEscaped = true
			r.advance()
			if rstIsNewline(r.lookahead) {
				break
			}
		} else {
			isEscaped = false
		}

		// Mark end of word on space.
		if !wordFound && rstIsSpace(r.lookahead) {
			wordFound = true
			r.markEnd()
		}

		// Mark end of word on start char.
		if !wordFound && rstIsStartChar(r.lookahead) {
			wordFound = true
			r.markEnd()
		}

		// Check for terminal character.
		if consumedChars > 0 && !rstIsSpace(r.previous) && rstIsInlineMarkupEndChar(r.lookahead) &&
			(!isEscaped || imType&rstIMLiteral != 0) {
			r.advance()

			isValid := true
			doAdvance := false

			if imType&rstIMStrong != 0 && r.previous == '*' && r.lookahead == '*' {
				r.setResult(rstTokStrong)
				for r.lookahead == '*' {
					r.advance()
					consumedChars++
				}
			} else if imType&rstIMEmphasis != 0 && r.previous == '*' {
				r.setResult(rstTokEmphasis)
			} else if imType&rstIMLiteral != 0 && r.previous == '`' && r.lookahead == '`' {
				r.setResult(rstTokLiteral)
				for r.lookahead == '`' {
					r.advance()
					consumedChars++
				}
			} else if imType&rstIMInlineTarget != 0 && r.previous == '`' {
				r.setResult(rstTokInlineTarget)
			} else if imType&rstIMReference != 0 && r.previous == '`' && r.lookahead == '_' {
				r.setResult(rstTokReference)
				r.advance()
				consumedChars++
				if r.lookahead == '_' {
					doAdvance = true
				}
			} else if (imType&rstIMInterpretedText != 0 || imType&rstIMInterpretedTextPfx != 0) && r.previous == '`' {
				if r.lookahead == ':' && imType&rstIMInterpretedTextPfx != 0 && r.valid(rstTokInterpretedTextPfx) {
					r.markEnd()
					r.advance()
					ok := r.parseRoleName()
					if ok {
						r.setResult(rstTokInterpretedTextPfx)
						return true
					}
					if r.valid(rstTokInterpretedText) {
						r.setResult(rstTokInterpretedText)
						return true
					}
					isValid = false
				} else {
					r.setResult(rstTokInterpretedText)
				}
			} else if imType&rstIMSubstitutionRef != 0 && r.previous == '|' {
				r.setResult(rstTokSubstitutionRef)
				// Substitution references can end with '__'.
				if r.lookahead == '_' {
					r.advance()
					if r.lookahead == '_' {
						doAdvance = true
					}
				}
			} else {
				isValid = false
			}

			if doAdvance {
				r.advance()
				consumedChars++
			}

			// Next char should be whitespace or end char.
			if isValid && (rstIsSpace(r.lookahead) || rstIsEndChar(r.lookahead)) {
				r.markEnd()
				return true
			}
		} else {
			r.advance()
		}
		consumedChars++
	}

	if !wordFound && rstIsNewline(r.lookahead) {
		return r.parseText(true)
	}
	return r.parseText(false)
}

func (r *rstCtx) parseReference() bool {
	if rstIsSpace(r.lookahead) || rstIsInternalReferenceChar(r.lookahead) || !r.valid(rstTokReference) {
		return false
	}
	r.advance()
	return r.parseInnerReference()
}

func (r *rstCtx) parseInnerReference() bool {
	internalSymbol := rstIsInternalReferenceChar(r.previous)
	isWord := false
	for (!rstIsSpace(r.lookahead) && !rstIsEndChar(r.lookahead)) || rstIsInternalReferenceChar(r.lookahead) {
		if rstIsStartChar(r.lookahead) && !isWord {
			isWord = true
			r.markEnd()
		}
		if rstIsInternalReferenceChar(r.lookahead) {
			if internalSymbol {
				break
			}
			internalSymbol = true
		} else {
			internalSymbol = false
		}
		r.advance()
	}

	// Only an anonymous reference can end with two consecutive '_'.
	if r.lookahead == '_' && r.previous == '_' {
		r.advance()
	}
	if r.previous == '_' && (rstIsSpace(r.lookahead) || rstIsEndChar(r.lookahead)) {
		r.markEnd()
		r.setResult(rstTokReference)
		return true
	}
	return r.parseText(!isWord)
}

func (r *rstCtx) parseStandaloneHyperlink() bool {
	if !rstIsABC(r.lookahead) || !r.valid(rstTokStandaloneHyperlink) {
		return false
	}
	r.advance()
	return r.parseInnerStandaloneHyperlink()
}

func (r *rstCtx) parseInnerStandaloneHyperlink() bool {
	const maxSchemaLen = 20
	var schema [maxSchemaLen]byte
	consumed := 0

	if consumed < maxSchemaLen {
		schema[consumed] = byte(r.previous)
		consumed++
	}
	for consumed < maxSchemaLen {
		if !rstIsAlphanumeric(r.lookahead) {
			break
		}
		schema[consumed] = byte(r.lookahead)
		consumed++
		r.advance()
	}

	isWord := false
	if rstIsStartChar(r.lookahead) {
		r.markEnd()
	}

	isValid := false
	if r.lookahead == ':' {
		isValid = rstIsKnownSchema(string(schema[:consumed]))
	} else if r.lookahead == '@' {
		isValid = true
	}

	if !isValid {
		if (!rstIsSpace(r.lookahead) && !rstIsEndChar(r.lookahead)) || rstIsInternalReferenceChar(r.lookahead) {
			return r.parseInnerReference()
		}
		return r.parseText(!isWord)
	}

	r.advance()

	if r.lookahead == '/' {
		r.advance()
	} else if !rstIsAlphanumeric(r.lookahead) {
		return r.parseText(!isWord)
	}

	uriChars := 0
	isEscaped := false
	for {
		r.markEnd()
		if r.lookahead == '\\' {
			r.advance()
			isEscaped = true
		} else {
			isEscaped = false
		}
		if rstIsInvalidURIChar(r.lookahead) {
			break
		}
		if rstIsSpace(r.lookahead) || (rstIsEndChar(r.lookahead) && !isEscaped && r.lookahead != '/') {
			if rstIsEndChar(r.lookahead) {
				r.markEnd()
				r.advance()
				if !rstIsAlphanumeric(r.lookahead) {
					r.setResult(rstTokStandaloneHyperlink)
					return true
				}
			} else {
				break
			}
		}
		r.advance()
		uriChars++
	}

	if uriChars > 0 {
		r.setResult(rstTokStandaloneHyperlink)
		return true
	}
	return r.parseText(!isWord)
}

func (r *rstCtx) parseRole() bool {
	if r.lookahead != ':' || (!r.valid(rstTokRoleNameSuffix) && !r.valid(rstTokRoleNamePrefix)) {
		return false
	}
	r.advance()
	r.markEnd()

	if rstIsSpace(r.lookahead) && r.valid(rstTokFieldMarkEnd) {
		// Consume all whitespace.
		r.getIndentLevel()
		r.markEnd()
		// Go to the next line.
		for !rstIsNewline(r.lookahead) {
			r.advance()
		}
		r.advance()

		indent := 0
		for {
			indent = r.getIndentLevel()
			if !rstIsNewline(r.lookahead) || r.lookahead == rstCharEOF {
				break
			}
			r.advance()
		}
		if indent > r.state.back() {
			r.state.push(indent)
		} else {
			r.state.push(r.state.back() + 1)
		}
		r.setResult(rstTokFieldMarkEnd)
		return true
	}

	if rstIsAlphanumeric(r.lookahead) {
		if r.parseInnerRole() {
			return true
		}
	}
	return r.parseText(false)
}

func (r *rstCtx) parseInnerRole() bool {
	if !rstIsAlphanumeric(r.lookahead) || (!r.valid(rstTokRoleNameSuffix) && !r.valid(rstTokRoleNamePrefix)) {
		return false
	}
	r.markEnd()
	ok := r.parseRoleName()
	if ok {
		if r.lookahead == '`' && r.valid(rstTokRoleNamePrefix) {
			r.markEnd()
			r.setResult(rstTokRoleNamePrefix)
			return true
		}
		if rstIsSpace(r.lookahead) && r.valid(rstTokFieldMark) {
			r.setResult(rstTokFieldMark)
			return true
		}
		if (rstIsSpace(r.lookahead) || rstIsEndChar(r.lookahead)) && r.valid(rstTokRoleNameSuffix) {
			r.markEnd()
			r.setResult(rstTokRoleNameSuffix)
			return true
		}
	}
	if r.valid(rstTokFieldMark) {
		if r.parseInnerFieldMark() {
			return true
		}
	}
	return false
}

func (r *rstCtx) parseRoleName() bool {
	if !rstIsAlphanumeric(r.lookahead) {
		return false
	}
	internalSymbol := true
	for rstIsAlphanumeric(r.lookahead) || rstIsInternalReferenceChar(r.lookahead) {
		if rstIsInternalReferenceChar(r.lookahead) {
			if internalSymbol {
				return false
			}
			internalSymbol = true
		} else {
			internalSymbol = false
		}
		r.advance()
	}
	return r.previous == ':'
}

func (r *rstCtx) parseText(markEnd bool) bool {
	if !r.valid(rstTokText) {
		return false
	}
	if rstIsStartChar(r.lookahead) {
		r.advance()
	} else {
		for !rstIsSpace(r.lookahead) {
			if rstIsStartChar(r.lookahead) {
				break
			}
			r.advance()
		}
	}
	if markEnd {
		r.markEnd()
	}
	r.setResult(rstTokText)
	return true
}
