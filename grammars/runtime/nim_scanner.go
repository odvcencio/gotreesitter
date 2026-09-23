//go:build !grammar_subset || grammar_subset_nim

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// ---------------------------------------------------------------------------
// External token indexes (position in the valid_symbols array). This is
// the external index (the position of the token in the grammar's
// `externals: [...]` list), which is exactly what tree-sitter's
// `valid_symbols` array and C's result_symbol enum are indexed by. The
// external index is stable across a blob regen as long as the externals
// list itself does not reorder; concrete numeric gotreesitter.Symbol IDs
// are NOT stable (they shift whenever the grammar's total symbol count
// changes), so this scanner never hardcodes them -- see nimDefaultSymTable
// below.
// ---------------------------------------------------------------------------
const (
	nimTokBlockCommentContent       = 0
	nimTokBlockDocCommentContent    = 1
	nimTokCommentContent            = 2
	nimTokLongStringQuote           = 3
	nimTokLayoutStart               = 4
	nimTokLayoutEnd                 = 5
	nimTokLayoutTerminator          = 6
	nimTokLayoutEmpty               = 7
	nimTokInhibitLayoutEnd          = 8
	nimTokInhibitKeywordTermination = 9
	nimTokComma                     = 10
	nimTokSynchronize               = 11
	nimTokInvalidLayout             = 12
	nimTokSigilOp                   = 13
	nimTokUnaryOp                   = 14
	nimTokWantExportMarker          = 15
	nimTokOf                        = 16
	nimTokLen                       = 17
)

// nimDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped nim.bin assigns to each external, in nimTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var nimDefaultSymTable = [nimTokLen]gotreesitter.Symbol{
	127, // _block_comment_content, displays as "comment_content"
	128, // _block_documentation_comment_content, displays as "comment_content"
	129, // comment_content
	130, // _long_string_quote
	131, // _layout_start
	132, // _layout_end
	133, // _layout_terminator
	134, // _layout_empty
	135, // _inhibit_layout_end
	136, // _inhibit_keyword_termination
	41,  // "," (literal, shared with the grammar's built-in comma token)
	137, // _synchronize
	138, // _invalid_layout
	87,  // _sigil_operator, displays as "operator"
	88,  // _prefix_operator, displays as "operator"
	139, // _want_export_marker
	140, // _case_of, displays as "of"
}

// nimExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (nimTok* order).
var nimExternalScannerSpec = ExternalScannerSpec{
	Language:       "nim",
	UpstreamRepo:   "https://github.com/alaviss/tree-sitter-nim",
	UpstreamCommit: "9b4ede21a6ca866d29263f6b66c070961bc622b4",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "d099fcf48f7acd36d8a7647bf10123a3aa444aa502df886a0228ccbe3aa11982"},
		{Path: "src/scanner.c", SHA256: "0d3fb9955a3fe49f89037890bc4154f0862a6959338a0d9ef5e79bcb7e7e2892"},
	},
	Externals: []string{
		"_block_comment_content",
		"_block_documentation_comment_content",
		"comment_content",
		"_long_string_quote",
		"_layout_start",
		"_layout_end",
		"_layout_terminator",
		"_layout_empty",
		"_inhibit_layout_end",
		"_inhibit_keyword_termination",
		",",
		"_synchronize",
		"_invalid_layout",
		"_sigil_operator",
		"_prefix_operator",
		"_want_export_marker",
		"_case_of",
	},
}

func init() {
	RegisterExternalScannerSpec(nimExternalScannerSpec)
}

// ---------------------------------------------------------------------------
// Indent value constants
// ---------------------------------------------------------------------------
const nimInvalidIndent uint8 = 0xFF

// ---------------------------------------------------------------------------
// Bitfield helpers for valid tokens
// ---------------------------------------------------------------------------
type nimValidTokens uint32

func nimValidTokensFromArray(validSymbols []bool) nimValidTokens {
	var bits nimValidTokens
	for i := 0; i < nimTokLen && i < len(validSymbols); i++ {
		if validSymbols[i] {
			bits |= 1 << uint(i)
		}
	}
	return bits
}

func (v nimValidTokens) test(tok int) bool {
	return (v & (1 << uint(tok))) != 0
}

func (v nimValidTokens) anyValid(other nimValidTokens) bool {
	return (v & other) != 0
}

func (v nimValidTokens) isError() bool {
	return v == (1<<nimTokLen)-1
}

// Pre-computed bitsets
var (
	nimCommentTokens  = nimValidTokens(1<<nimTokBlockCommentContent | 1<<nimTokBlockDocCommentContent | 1<<nimTokCommentContent)
	nimNoLayoutEndCtx = nimValidTokens(1<<nimTokInhibitLayoutEnd | 1<<nimTokLongStringQuote)
	nimUnaryOps       = nimValidTokens(1<<nimTokUnaryOp | 1<<nimTokSigilOp)
)

// ---------------------------------------------------------------------------
// Scanner state (persisted via Serialize/Deserialize)
// ---------------------------------------------------------------------------
type nimState struct {
	layoutStack []uint8
}

// ---------------------------------------------------------------------------
// Context wraps the lexer + state for a single Scan invocation
// ---------------------------------------------------------------------------
type nimContext struct {
	lexer          *gotreesitter.ExternalLexer
	state          *nimState
	advanceCounter uint32
	validTokens    nimValidTokens
	currentIndent  uint8
	afterNewline   bool
	syms           *[nimTokLen]gotreesitter.Symbol
}

func (ctx *nimContext) lookahead() rune {
	return ctx.lexer.Lookahead()
}

func (ctx *nimContext) eof() bool {
	return ctx.lexer.Lookahead() == 0
}

func (ctx *nimContext) markEnd() {
	ctx.lexer.MarkEnd()
}

func (ctx *nimContext) advance(skip bool) rune {
	if !ctx.eof() {
		ctx.advanceCounter++
		ctx.afterNewline = false
	}
	ctx.lexer.Advance(skip)
	return ctx.lexer.Lookahead()
}

// consume = advance + markEnd
func (ctx *nimContext) consume(skip bool) rune {
	r := ctx.advance(skip)
	ctx.markEnd()
	return r
}

func (ctx *nimContext) finish(tok int) bool {
	ctx.lexer.SetResultSymbol(ctx.syms[tok])
	return true
}

func (ctx *nimContext) indent() uint8 {
	if ctx.afterNewline {
		return ctx.currentIndent
	}
	return nimInvalidIndent
}

// ---------------------------------------------------------------------------
// Character classification helpers
// ---------------------------------------------------------------------------
func nimIsDigit(ch rune) bool      { return ch >= '0' && ch <= '9' }
func nimIsLower(ch rune) bool      { return ch >= 'a' && ch <= 'z' }
func nimIsUpper(ch rune) bool      { return ch >= 'A' && ch <= 'Z' }
func nimIsKeyword(ch rune) bool    { return nimIsLower(ch) || nimIsUpper(ch) || ch == '_' }
func nimIsIdentifier(ch rune) bool { return nimIsKeyword(ch) || nimIsDigit(ch) }

func nimToUpper(ch rune) rune {
	if nimIsLower(ch) {
		return ch & ^rune(1<<5)
	}
	return ch
}

func nimChrCaseEq(a, b rune) bool {
	return nimToUpper(a) == nimToUpper(b)
}

var nimOperatorChars = []rune{
	'$', '^', // OP10
	'*', '%', '\\', '/', // OP9
	'+', '-', '~', '|', // OP8
	'&',                // OP7
	'.',                // OP6
	'=', '<', '>', '!', // OP5
	':', '?', '@', // OP2
}

var nimUnicodeOperatorChars = []rune{
	'\u2219', '\u2218', '\u00D7', '\u2605', '\u2297', '\u2298',
	'\u2299', '\u229B', '\u22A0', '\u22A1', '\u2229', '\u2227',
	'\u2293', '\u00B1', '\u2295', '\u2296', '\u229E', '\u229F',
	'\u222A', '\u2228', '\u2294',
}

func nimIsOperator(ch rune) bool {
	for _, c := range nimOperatorChars {
		if c == ch {
			return true
		}
	}
	if ch > 0xFF {
		for _, c := range nimUnicodeOperatorChars {
			if c == ch {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Whitespace scanning
// ---------------------------------------------------------------------------
func nimScanSpaces(ctx *nimContext, forceUpdate bool) int {
	updateIndent := forceUpdate
	var ind uint8
	spaces := 0
	for {
		switch ctx.lookahead() {
		case ' ':
			if ind != nimInvalidIndent {
				ind++
			}
			spaces++
			ctx.advance(true)
		case '\n', '\r':
			updateIndent = true
			ind = 0
			spaces++
			ctx.advance(true)
		case 0:
			if ctx.eof() {
				updateIndent = true
				ind = 0
			}
			goto loopEnd
		default:
			goto loopEnd
		}
	}
loopEnd:
	if updateIndent {
		ctx.currentIndent = ind
		ctx.afterNewline = true
	}
	return spaces
}

// ---------------------------------------------------------------------------
// Long string quote lexer
// ---------------------------------------------------------------------------
func nimLexLongStringQuote(ctx *nimContext) bool {
	if ctx.lookahead() != '"' || !ctx.validTokens.test(nimTokLongStringQuote) {
		return false
	}

	ctx.consume(false)
	count := uint8(1)
	for ctx.lookahead() == '"' && count < 3 {
		ctx.advance(false)
		count++
	}

	if count < 3 {
		ctx.markEnd()
		return ctx.finish(nimTokLongStringQuote)
	}

	if ctx.lookahead() == '"' {
		return ctx.finish(nimTokLongStringQuote)
	}

	return false
}

// ---------------------------------------------------------------------------
// Comment content lexer
// ---------------------------------------------------------------------------
func nimLexCommentContent(ctx *nimContext) bool {
	if !ctx.validTokens.anyValid(nimCommentTokens) || ctx.validTokens.isError() {
		return false
	}

	// Short (line) comment
	if ctx.validTokens.test(nimTokCommentContent) {
		for !ctx.eof() {
			switch ctx.lookahead() {
			case '\n', '\r':
				goto exitShortCommentLoop
			default:
				ctx.advance(false)
			}
		}
	exitShortCommentLoop:
		ctx.markEnd()
		return ctx.finish(nimTokCommentContent)
	}

	// Block comment content (with nesting)
	nesting := uint32(0)
	for !ctx.eof() {
		if ctx.lookahead() == '#' {
			if ctx.advance(false) == '[' {
				nesting++
			}
		}
		ctx.markEnd()
		if ctx.lookahead() == ']' {
			if ctx.advance(false) == '#' {
				if nesting > 0 {
					nesting--
				} else if ctx.validTokens.test(nimTokBlockDocCommentContent) {
					if ctx.advance(false) == '#' {
						return ctx.finish(nimTokBlockDocCommentContent)
					}
				} else {
					return ctx.finish(nimTokBlockCommentContent)
				}
			}
			continue
		}
		ctx.advance(false)
	}

	return false
}

// ---------------------------------------------------------------------------
// Init lexer (first synchronization)
// ---------------------------------------------------------------------------
func nimLexInit(ctx *nimContext) bool {
	if len(ctx.state.layoutStack) > 0 ||
		ctx.validTokens.isError() ||
		ctx.validTokens.anyValid(nimCommentTokens) {
		return false
	}

	nimScanSpaces(ctx, true)
	if ctx.lookahead() == '#' {
		return false
	}

	currentIndent := ctx.indent()
	if currentIndent == nimInvalidIndent {
		return false
	}
	ctx.state.layoutStack = append(ctx.state.layoutStack, currentIndent)
	return ctx.finish(nimTokSynchronize)
}

// ---------------------------------------------------------------------------
// Continuing keyword scanner (else, elif, except, finally, do)
// ---------------------------------------------------------------------------
func nimSkipUnderscore(ctx *nimContext) {
	if ctx.lookahead() == '_' {
		ctx.advance(false)
	}
}

func nimScanContinuingKeyword(ctx *nimContext, scanDo bool) bool {
	// Helper: advance, skip underscore, check case-insensitive char match
	nextOrFail := func(ch rune) bool {
		ctx.advance(false)
		nimSkipUnderscore(ctx)
		return nimChrCaseEq(ctx.lookahead(), ch)
	}
	finishIfEnd := func() bool {
		ctx.advance(false)
		return !nimIsIdentifier(ctx.lookahead())
	}

	if ctx.lookahead() == 'e' {
		ctx.advance(false)
		nimSkipUnderscore(ctx)
		if nimChrCaseEq(ctx.lookahead(), 'l') {
			ctx.advance(false)
			nimSkipUnderscore(ctx)
			if nimChrCaseEq(ctx.lookahead(), 's') {
				// else
				if !nextOrFail('e') {
					return false
				}
				return finishIfEnd()
			} else if nimChrCaseEq(ctx.lookahead(), 'i') {
				// elif
				if !nextOrFail('f') {
					return false
				}
				return finishIfEnd()
			}
			return false
		}
		if nimChrCaseEq(ctx.lookahead(), 'x') {
			// except
			if !nextOrFail('c') {
				return false
			}
			if !nextOrFail('e') {
				return false
			}
			if !nextOrFail('p') {
				return false
			}
			if !nextOrFail('t') {
				return false
			}
			return finishIfEnd()
		}
	}

	if ctx.lookahead() == 'f' {
		// finally
		if !nextOrFail('i') {
			return false
		}
		if !nextOrFail('n') {
			return false
		}
		if !nextOrFail('a') {
			return false
		}
		if !nextOrFail('l') {
			return false
		}
		if !nextOrFail('l') {
			return false
		}
		if !nextOrFail('y') {
			return false
		}
		return finishIfEnd()
	}

	if scanDo {
		if ctx.lookahead() == 'd' {
			// do
			if !nextOrFail('o') {
				return false
			}
			return finishIfEnd()
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Case-of lexer
// ---------------------------------------------------------------------------
func nimLexCaseOf(ctx *nimContext) bool {
	if ctx.lookahead() != 'o' || !ctx.validTokens.test(nimTokOf) {
		return false
	}

	nimSkipUnderscore(ctx)
	next := ctx.advance(false)
	switch next {
	case 'f', 'F':
		if nimIsIdentifier(ctx.advance(false)) {
			return false
		}
		ctx.markEnd()
		return ctx.finish(nimTokOf)
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Indent-based layout lexer
// ---------------------------------------------------------------------------
func nimLexIndent(ctx *nimContext) bool {
	if ctx.lookahead() == '#' || len(ctx.state.layoutStack) == 0 {
		return false
	}

	currentIndent := ctx.indent()
	if currentIndent == nimInvalidIndent {
		return false
	}

	currentLayout := ctx.state.layoutStack[len(ctx.state.layoutStack)-1]

	// LAYOUT_START
	if ctx.validTokens.test(nimTokLayoutStart) {
		if currentIndent > currentLayout {
			ctx.state.layoutStack = append(ctx.state.layoutStack, currentIndent)
			return ctx.finish(nimTokLayoutStart)
		}
	}

	// LAYOUT_EMPTY
	if ctx.validTokens.test(nimTokLayoutEmpty) {
		if !ctx.validTokens.isError() {
			if currentIndent <= currentLayout {
				return ctx.finish(nimTokLayoutEmpty)
			}
		}
	}

	// LAYOUT_TERMINATOR
	if ctx.validTokens.test(nimTokLayoutTerminator) {
		if currentIndent <= currentLayout {
			lastCount := ctx.advanceCounter
			if currentIndent == currentLayout {
				if ctx.validTokens.test(nimTokInhibitKeywordTermination) &&
					nimScanContinuingKeyword(ctx, true) {
					return false
				}
				if lastCount == ctx.advanceCounter {
					if nimLexCaseOf(ctx) {
						return true
					}
				}
			}
			return ctx.finish(nimTokLayoutTerminator)
		}
	}

	// Implicit layout changes
	if !ctx.validTokens.anyValid(nimNoLayoutEndCtx) ||
		ctx.validTokens.isError() ||
		ctx.eof() {
		// LAYOUT_END
		if currentIndent < currentLayout || ctx.eof() {
			if len(ctx.state.layoutStack) > 1 {
				ctx.state.layoutStack = ctx.state.layoutStack[:len(ctx.state.layoutStack)-1]
				return ctx.finish(nimTokLayoutEnd)
			}
		}
	}

	// TRY_LEX(ctx, lex_case_of)
	lastCount := ctx.advanceCounter
	if nimLexCaseOf(ctx) {
		return true
	}
	if ctx.advanceCounter != lastCount {
		return false
	}

	return false
}

// ---------------------------------------------------------------------------
// Inline layout lexer
// ---------------------------------------------------------------------------
func nimLexInlineLayout(ctx *nimContext) bool {
	if len(ctx.state.layoutStack) == 0 || ctx.afterNewline {
		return false
	}

	matched := false
	switch ctx.lookahead() {
	case ',':
		if ctx.validTokens.test(nimTokComma) {
			return false
		}
		matched = true
	case ')', ']', '}':
		matched = true
	case '.':
		if ctx.advance(false) == '}' {
			matched = true
		} else {
			return false
		}
	default:
		if !ctx.validTokens.test(nimTokInhibitKeywordTermination) &&
			nimScanContinuingKeyword(ctx, false) {
			matched = true
		} else {
			return false
		}
	}

	if !matched {
		return false
	}

	if ctx.validTokens.test(nimTokLayoutTerminator) {
		return ctx.finish(nimTokLayoutTerminator)
	}

	if ctx.validTokens.test(nimTokLayoutEnd) && len(ctx.state.layoutStack) > 1 {
		ctx.state.layoutStack = ctx.state.layoutStack[:len(ctx.state.layoutStack)-1]
		return ctx.finish(nimTokLayoutEnd)
	}

	return false
}

// ---------------------------------------------------------------------------
// Operator scanning
// ---------------------------------------------------------------------------

const (
	nimOSRegular = iota
	nimOSColon
	nimOSColonColon
	nimOSDot
	nimOSEqual
	nimOSMinus
	nimOSStar
)

// nimScanOperator returns the detected token type, or nimTokLen on failure.
func nimScanOperator(ctx *nimContext, immediate bool) int {
	if immediate {
		return nimTokLen
	}

	state := nimOSRegular
	firstChar := ctx.lookahead()
	if !nimIsOperator(firstChar) {
		return nimTokLen
	}

	switch firstChar {
	case '.':
		ctx.advance(false)
		state = nimOSDot
	case '=':
		ctx.advance(false)
		state = nimOSEqual
	case ':':
		ctx.advance(false)
		state = nimOSColon
	case '-':
		ctx.advance(false)
		state = nimOSMinus
	case '*':
		ctx.advance(false)
		state = nimOSStar
	}

	for nimIsOperator(ctx.lookahead()) {
		switch state {
		case nimOSStar:
			switch ctx.lookahead() {
			case ':':
				goto loopEnd
			default:
				state = nimOSRegular
			}
		case nimOSColon:
			switch ctx.lookahead() {
			case ':':
				state = nimOSColonColon
				ctx.advance(false)
			default:
				state = nimOSRegular
			}
		default: // nimOSColonColon, nimOSDot, nimOSEqual, nimOSMinus, nimOSRegular
			state = nimOSRegular
			ctx.advance(false)
		}
	}
loopEnd:

	switch state {
	case nimOSEqual, nimOSColon, nimOSColonColon, nimOSDot:
		return nimTokLen
	case nimOSMinus:
		if nimIsDigit(ctx.lookahead()) {
			return nimTokLen
		}
	case nimOSStar:
		if ctx.validTokens.test(nimTokWantExportMarker) {
			return nimTokLen
		}
	}

	switch ctx.lookahead() {
	case ' ', '\n', '\r':
		return nimTokLen
	default:
		return nimTokUnaryOp
	}
}

func nimLexOperators(ctx *nimContext, immediate bool) bool {
	if !ctx.validTokens.anyValid(nimUnaryOps) {
		return false
	}

	firstChar := ctx.lookahead()
	result := nimScanOperator(ctx, immediate)
	if result == nimTokLen {
		if firstChar == '.' {
			// TRY_LEX(ctx, lex_inline_layout)
			lastCount := ctx.advanceCounter
			if nimLexInlineLayout(ctx) {
				return true
			}
			if ctx.advanceCounter != lastCount {
				return false
			}
		}
		return false
	}

	if firstChar == '@' {
		result = nimTokSigilOp
	} else {
		result = nimTokUnaryOp
	}

	if !ctx.validTokens.test(result) {
		return false
	}

	ctx.markEnd()
	return ctx.finish(result)
}

// ---------------------------------------------------------------------------
// Main lexing entry point
// ---------------------------------------------------------------------------
func nimLexMain(ctx *nimContext) bool {
	// TRY_LEX: lex_init
	{
		last := ctx.advanceCounter
		if nimLexInit(ctx) {
			return true
		}
		if ctx.advanceCounter != last {
			return false
		}
	}

	// TRY_LEX: lex_comment_content
	{
		last := ctx.advanceCounter
		if nimLexCommentContent(ctx) {
			return true
		}
		if ctx.advanceCounter != last {
			return false
		}
	}

	// TRY_LEX: lex_long_string_quote
	{
		last := ctx.advanceCounter
		if nimLexLongStringQuote(ctx) {
			return true
		}
		if ctx.advanceCounter != last {
			return false
		}
	}

	spaces := nimScanSpaces(ctx, false)

	// TRY_LEX: lex_indent
	{
		last := ctx.advanceCounter
		if nimLexIndent(ctx) {
			return true
		}
		if ctx.advanceCounter != last {
			return false
		}
	}

	// TRY_LEX: lex_operators (with immediate flag)
	{
		last := ctx.advanceCounter
		if nimLexOperators(ctx, spaces == 0) {
			return true
		}
		if ctx.advanceCounter != last {
			return false
		}
	}

	// TRY_LEX: lex_inline_layout
	{
		last := ctx.advanceCounter
		if nimLexInlineLayout(ctx) {
			return true
		}
		if ctx.advanceCounter != last {
			return false
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// NimExternalScanner implements gotreesitter.ExternalScanner
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
// ---------------------------------------------------------------------------
type NimExternalScanner struct {
	symbols         [nimTokLen]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers nim's external symbols.
func (NimExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := NimExternalScanner{symbols: nimDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, nimExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s NimExternalScanner) symbolTable() *[nimTokLen]gotreesitter.Symbol {
	if s.symbols == ([nimTokLen]gotreesitter.Symbol{}) {
		return &nimDefaultSymTable
	}
	return &s.symbols
}

func (NimExternalScanner) Create() any {
	return &nimState{}
}

func (NimExternalScanner) Destroy(payload any) {}

func (NimExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*nimState)
	n := len(s.layoutStack)
	if n > len(buf) {
		n = len(buf)
	}
	if n == 0 {
		return 0
	}
	copy(buf[:n], s.layoutStack[:n])
	return n
}

func (NimExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*nimState)
	s.layoutStack = s.layoutStack[:0]
	if len(buf) == 0 {
		return
	}
	s.layoutStack = append(s.layoutStack, buf...)
}

func (sc NimExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {

	s := payload.(*nimState)

	ctx := &nimContext{
		lexer:       lexer,
		state:       s,
		validTokens: nimValidTokensFromArray(validSymbols),
		syms:        sc.symbolTable(),
	}

	ctx.markEnd()
	return nimLexMain(ctx)
}
