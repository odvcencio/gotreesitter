//go:build !grammar_subset || grammar_subset_haskell

package grammars

// Haskell external scanner -- ported from tree-sitter-haskell/src/scanner.c.
//
// This scanner handles Haskell's layout-sensitive indentation, virtual
// semicolons, phantom tokens, nested comments, CPP preprocessor directives,
// pragmas, Haddock documentation comments, quasiquote bodies, and operator
// classification (varsym/consym, prefix/infix/tight/qualified).

import (
	"encoding/binary"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// ---------------------------------------------------------------------------
// Symbols -- mirrors externals in grammar/externals.js
// ---------------------------------------------------------------------------

const (
	hsFAIL           = 0
	hsSEMICOLON      = 1
	hsSTART          = 2
	hsSTART_DO       = 3
	hsSTART_CASE     = 4
	hsSTART_IF       = 5
	hsSTART_LET      = 6
	hsSTART_QUOTE    = 7
	hsSTART_EXPLICIT = 8
	hsEND            = 9
	hsEND_EXPLICIT   = 10
	hsSTART_BRACE    = 11
	hsEND_BRACE      = 12
	hsSTART_TEXP     = 13
	hsEND_TEXP       = 14
	hsWHERE          = 15
	hsIN             = 16
	hsARROW          = 17
	hsBAR            = 18
	hsDERIVING       = 19
	hsCOMMENT        = 20
	hsHADDOCK        = 21
	hsCPP            = 22
	hsPRAGMA         = 23
	hsQQ_START       = 24
	hsQQ_BODY        = 25
	hsSPLICE         = 26
	hsQUAL_DOT       = 27
	hsTIGHT_DOT      = 28
	hsPREFIX_DOT     = 29
	hsDOTDOT         = 30
	hsTIGHT_AT       = 31
	hsPREFIX_AT      = 32
	hsTIGHT_BANG     = 33
	hsPREFIX_BANG    = 34
	hsTIGHT_TILDE    = 35
	hsPREFIX_TILDE   = 36
	hsPREFIX_PERCENT = 37
	hsQUALIFIED_OP   = 38
	hsLEFT_SECTION   = 39
	hsNO_SECTION_OP  = 40
	hsMINUS          = 41
	hsCONTEXT        = 42
	hsINFIX          = 43
	hsDATA_INFIX     = 44
	hsTYPE_INSTANCE  = 45
	hsVARSYM         = 46
	hsCONSYM         = 47
	hsUPDATE         = 48
)

// Concrete symbol IDs from the generated Haskell grammar.
var hsSymMap = [49]gotreesitter.Symbol{
	108, // FAIL (error_sentinel)
	109, // SEMICOLON
	110, // START
	111, // START_DO
	112, // START_CASE
	113, // START_IF
	114, // START_LET
	115, // START_QUOTE
	116, // START_EXPLICIT ({)
	117, // END
	118, // END_EXPLICIT (})
	119, // START_BRACE
	120, // END_BRACE
	121, // START_TEXP
	122, // END_TEXP
	123, // WHERE
	124, // IN
	125, // ARROW
	126, // BAR
	127, // DERIVING
	128, // COMMENT
	129, // HADDOCK
	130, // CPP
	131, // PRAGMA
	132, // QQ_START
	133, // QQ_BODY
	134, // SPLICE
	135, // QUAL_DOT
	136, // TIGHT_DOT
	137, // PREFIX_DOT
	138, // DOTDOT
	139, // TIGHT_AT
	140, // PREFIX_AT
	141, // TIGHT_BANG
	142, // PREFIX_BANG
	143, // TIGHT_TILDE
	144, // PREFIX_TILDE
	145, // PREFIX_PERCENT
	146, // QUALIFIED_OP
	147, // LEFT_SECTION_OP
	148, // NO_SECTION_OP
	149, // MINUS
	150, // CONTEXT
	151, // INFIX
	152, // DATA_INFIX
	153, // TYPE_INSTANCE (assoc_tyinst)
	154, // VARSYM
	155, // CONSYM
	107, // UPDATE (_token1)
}

// ---------------------------------------------------------------------------
// Context sorts
// ---------------------------------------------------------------------------

const (
	hsDeclLayout   = 0
	hsDoLayout     = 1
	hsCaseLayout   = 2
	hsLetLayout    = 3
	hsQuoteLayout  = 4
	hsMultiWayIf   = 5
	hsBraces       = 6
	hsTExp         = 7
	hsModuleHeader = 8
	hsNoContext    = 9
)

// ---------------------------------------------------------------------------
// Lexed token enum
// ---------------------------------------------------------------------------

const (
	hsLNothing      = 0
	hsLEof          = 1
	hsLWhere        = 2
	hsLIn           = 3
	hsLThen         = 4
	hsLElse         = 5
	hsLDeriving     = 6
	hsLModule       = 7
	hsLUpper        = 8
	hsLTick         = 9
	hsLSymop        = 10
	hsLSymopSpecial = 11
	hsLDotDot       = 12
	hsLDotId        = 13
	hsLDotSymop     = 14
	hsLDotOpen      = 15
	hsLDollar       = 16
	hsLBang         = 17
	hsLTilde        = 18
	hsLAt           = 19
	hsLPercent      = 20
	hsLHash         = 21
	hsLBar          = 22
	hsLArrow        = 23
	hsLCArrow       = 24
	hsLTexpCloser   = 25
	hsLQuoteClose   = 26
	hsLPragma       = 27
	hsLBlockComment = 28
	hsLLineComment  = 29
	hsLBraceClose   = 30
	hsLBraceOpen    = 31
	hsLBracketOpen  = 32
	hsLUnboxedClose = 33
	hsLSemi         = 34
	hsLCppElse      = 35
	hsLCpp          = 36
)

// ---------------------------------------------------------------------------
// Newline state
// ---------------------------------------------------------------------------

const (
	hsNInactive = 0
	hsNInit     = 1
	hsNProcess  = 2
	hsNResume   = 3
)

// ---------------------------------------------------------------------------
// Space enum
// ---------------------------------------------------------------------------

const (
	hsNoSpace  = 0
	hsIndented = 1
	hsBOL      = 2
)

// ---------------------------------------------------------------------------
// CppDirective enum
// ---------------------------------------------------------------------------

const (
	hsCppNothing = 0
	hsCppStart   = 1
	hsCppElse    = 2
	hsCppEnd     = 3
	hsCppOther   = 4
)

// ---------------------------------------------------------------------------
// CtrResult enum
// ---------------------------------------------------------------------------

const (
	hsCtrUndecided   = 0
	hsCtrImpossible  = 1
	hsCtrArrowFound  = 2
	hsCtrInfixFound  = 3
	hsCtrEqualsFound = 4
	hsCtrBarFound    = 5
)

// ---------------------------------------------------------------------------
// QualifiedName enum
// ---------------------------------------------------------------------------

const (
	hsNoQualifiedName = 0
	hsQualifiedTarget = 1
	hsQualifiedConid  = 2
)

// ---------------------------------------------------------------------------
// Data structures
// ---------------------------------------------------------------------------

type hsContext struct {
	sort   uint8
	indent uint32
}

type hsNewline struct {
	state    uint8
	end      int // Lexed token
	indent   uint32
	eof      bool
	noSemi   bool
	skipSemi bool
	unsafe   bool
}

type hsLookahead struct {
	contents []rune
	offset   uint32
}

type hsState struct {
	contexts  []hsContext
	newline   hsNewline
	lookahead hsLookahead
}

// hsEnv bundles transient state for one scanner run.
type hsEnv struct {
	lexer   *gotreesitter.ExternalLexer
	symbols []bool
	symop   uint32
	state   *hsState
}

func hsEnvNew(lexer *gotreesitter.ExternalLexer, symbols []bool, state *hsState) *hsEnv {
	return &hsEnv{
		lexer:   lexer,
		symbols: symbols,
		symop:   0,
		state:   state,
	}
}

func (env *hsEnv) resetNewline() {
	env.state.newline = hsNewline{}
}

func (env *hsEnv) newlineActive() bool {
	return env.state.newline.state == hsNInit || env.state.newline.state == hsNProcess
}

func (env *hsEnv) newlineInit() bool {
	return env.state.newline.state == hsNInit
}

// ---------------------------------------------------------------------------
// Unicode character classification -- Haskell spec compatible
// ---------------------------------------------------------------------------

// hsIsIdentifierChar: Unicode letters, digits, underscore, single quote.
// Haskell allows a superset of Unicode ID chars in identifiers.
func hsIsIdentifierChar(c rune) bool {
	return unicode.IsLetter(c) || unicode.IsDigit(c) || unicode.Is(unicode.Nd, c) ||
		c == '_' || c == '\''
}

// hsIsVaridStartChar: lowercase letters and underscore-like characters.
func hsIsVaridStartChar(c rune) bool {
	return c == '_' || (unicode.IsLetter(c) && unicode.IsLower(c))
}

// hsIsConidStartChar: uppercase letters (and titlecase).
func hsIsConidStartChar(c rune) bool {
	return unicode.IsLetter(c) && (unicode.IsUpper(c) || unicode.IsTitle(c))
}

// hsIsSymopChar: symbolic operator characters per Haskell spec.
func hsIsSymopChar(c rune) bool {
	if c < 128 {
		switch c {
		case '!', '#', '$', '%', '&', '*', '+', '.', '/', '<', '=', '>', '?', '@', '\\', '^', '|', '-', '~', ':':
			return true
		default:
			return false
		}
	}
	// Unicode symbol/punctuation classes
	return unicode.IsSymbol(c) || unicode.Is(unicode.Pc, c) || unicode.Is(unicode.Pd, c) ||
		unicode.Is(unicode.Po, c) || unicode.Is(unicode.Pi, c) || unicode.Is(unicode.Pf, c)
}

// hsIsSpaceChar: Unicode space characters excluding newlines (horizontal space).
func hsIsSpaceChar(c rune) bool {
	if c == ' ' || c == '\t' {
		return true
	}
	if c == '\n' || c == '\r' || c == '\f' {
		return false
	}
	return unicode.Is(unicode.Zs, c)
}

func hsIsNewline(c rune) bool {
	return c == '\n' || c == '\r' || c == '\f'
}

func hsIsSpaceOrTab(c rune) bool {
	return c == ' ' || c == '\t'
}

// hsVaridStartChar: _ or lowercase letter.
func hsVaridStartChar(c rune) bool {
	return c == '_' || hsIsVaridStartChar(c)
}

func hsIsIdChar(c rune) bool {
	return c == '_' || c == '\'' || hsIsIdentifierChar(c)
}

func hsIsInnerIdChar(c rune) bool {
	return hsIsIdChar(c) || c == '#'
}

func hsQuoterChar(c rune) bool {
	return hsIsIdChar(c) || c == '.'
}

func hsReservedSymbolic(c rune) bool {
	switch c {
	case '(', ')', ',', ';', '[', ']', '`', '{', '}', '"', '\'', '_':
		return true
	}
	return false
}

func hsSymopCharNotReserved(c rune) bool {
	return hsIsSymopChar(c) && !hsReservedSymbolic(c)
}

func hsTokenEnd(c rune) bool {
	return !hsIsInnerIdChar(c)
}

// ---------------------------------------------------------------------------
// Lexer interaction
// ---------------------------------------------------------------------------

func (env *hsEnv) isEof() bool {
	return env.lexer.Lookahead() == 0
}

func (env *hsEnv) notEof() bool {
	return !env.isEof()
}

func (env *hsEnv) peek() rune {
	return env.lexer.Lookahead()
}

func (env *hsEnv) column() uint32 {
	if env.isEof() {
		return 0
	}
	return env.lexer.Column()
}

func (env *hsEnv) advance() {
	if env.notEof() {
		env.state.lookahead.contents = append(env.state.lookahead.contents, env.peek())
		env.lexer.Advance(false)
	}
}

func (env *hsEnv) skip() {
	env.lexer.Advance(true)
}

func (env *hsEnv) markEnd() {
	env.lexer.MarkEnd()
}

func (env *hsEnv) valid(s int) bool {
	return s >= 0 && s < len(env.symbols) && env.symbols[s]
}

func (env *hsEnv) setResultSymbol(result int) bool {
	if result != hsFAIL {
		env.lexer.SetResultSymbol(hsSymMap[result])
		return true
	}
	return false
}

func (env *hsEnv) afterError() bool {
	return env.valid(hsFAIL)
}

// ---------------------------------------------------------------------------
// Lookahead buffer
// ---------------------------------------------------------------------------

func (env *hsEnv) advanceOverAbs(abs uint32) {
	for i := uint32(len(env.state.lookahead.contents)); i <= abs; i++ {
		env.advance()
	}
}

func (env *hsEnv) advanceOver(rel uint32) {
	env.advanceOverAbs(env.state.lookahead.offset + rel)
}

func (env *hsEnv) skipOver(rel uint32) {
	la := &env.state.lookahead
	if la.offset > uint32(len(la.contents)) {
		env.advanceOverAbs(la.offset - 1)
	}
	abs := la.offset + rel
	for i := uint32(len(env.state.lookahead.contents)); i <= abs; i++ {
		env.skip()
	}
}

func (env *hsEnv) advanceBefore(rel uint32) {
	abs := env.state.lookahead.offset + rel
	if abs > 0 {
		env.advanceOverAbs(abs - 1)
	}
}

func (env *hsEnv) unsafePeekAbs(abs uint32) rune {
	if abs < uint32(len(env.state.lookahead.contents)) {
		return env.state.lookahead.contents[abs]
	}
	return 0
}

func (env *hsEnv) unsafePeek(rel uint32) rune {
	return env.unsafePeekAbs(env.state.lookahead.offset + rel)
}

func (env *hsEnv) peekAt(rel uint32) rune {
	abs := env.state.lookahead.offset + rel
	if abs < uint32(len(env.state.lookahead.contents)) {
		return env.unsafePeek(rel)
	}
	env.advanceBefore(rel)
	return env.peek()
}

func (env *hsEnv) peek0() rune { return env.peekAt(0) }
func (env *hsEnv) peek1() rune { return env.peekAt(1) }
func (env *hsEnv) peek2() rune { return env.peekAt(2) }

func (env *hsEnv) charAt(n uint32, c rune) bool { return env.peekAt(n) == c }
func (env *hsEnv) char0(c rune) bool            { return env.charAt(0, c) }
func (env *hsEnv) char1(c rune) bool            { return env.charAt(1, c) }
func (env *hsEnv) char2(c rune) bool            { return env.charAt(2, c) }

func (env *hsEnv) resetLookaheadAbs(abs uint32) {
	env.state.lookahead.offset = abs
	env.symop = 0
}

func (env *hsEnv) resetLookaheadTo(rel uint32) {
	env.resetLookaheadAbs(env.state.lookahead.offset + rel)
}

func (env *hsEnv) resetLookahead() {
	env.resetLookaheadAbs(uint32(len(env.state.lookahead.contents)))
}

func (env *hsEnv) noLookahead() bool {
	return len(env.state.lookahead.contents) == 0
}

func (env *hsEnv) startColumn() uint32 {
	return env.column() - uint32(len(env.state.lookahead.contents))
}

func (env *hsEnv) advanceWhile(i uint32, pred func(rune) bool) uint32 {
	for pred(env.peekAt(i)) {
		i++
	}
	return i
}

func (env *hsEnv) advanceUntilChar(i uint32, c rune) uint32 {
	for env.notEof() && !env.charAt(i, c) {
		i++
	}
	return i
}

// ---------------------------------------------------------------------------
// Context manipulation
// ---------------------------------------------------------------------------

func (env *hsEnv) hasContexts() bool {
	return len(env.state.contexts) != 0
}

func (env *hsEnv) pushContext(sort uint8, indent uint32) {
	env.state.contexts = append(env.state.contexts, hsContext{sort: sort, indent: indent})
}

func (env *hsEnv) pop() {
	if env.hasContexts() {
		env.state.contexts = env.state.contexts[:len(env.state.contexts)-1]
	}
}

func (env *hsEnv) currentContext() uint8 {
	if env.hasContexts() {
		return env.state.contexts[len(env.state.contexts)-1].sort
	}
	return hsNoContext
}

func (env *hsEnv) isLayoutContext() bool {
	return env.currentContext() < hsBraces
}

func (env *hsEnv) isSemicolonContext() bool {
	return env.currentContext() < hsMultiWayIf
}

func (env *hsEnv) currentIndent() uint32 {
	for i := len(env.state.contexts) - 1; i >= 0; i-- {
		if env.state.contexts[i].sort < hsBraces {
			return env.state.contexts[i].indent
		}
	}
	return 0
}

func (env *hsEnv) indentLess(indent uint32) bool {
	return env.isLayoutContext() && indent < env.currentIndent()
}

func (env *hsEnv) indentLesseq(indent uint32) bool {
	return env.isLayoutContext() && indent <= env.currentIndent()
}

func (env *hsEnv) topLayout() bool {
	return len(env.state.contexts) == 1
}

func (env *hsEnv) inModuleHeader() bool {
	return env.currentContext() == hsModuleHeader
}

func (env *hsEnv) contextEndSym(s uint8) int {
	switch s {
	case hsTExp:
		return hsEND_TEXP
	case hsBraces:
		return hsEND_BRACE
	default:
		if s < hsBraces {
			return hsEND
		}
		return hsFAIL
	}
}

func (env *hsEnv) uninitialized() bool {
	return !env.hasContexts()
}

// ---------------------------------------------------------------------------
// String / lookahead matching
// ---------------------------------------------------------------------------

func (env *hsEnv) seqFrom(s string, start uint32) bool {
	l := uint32(len(s))
	for i := uint32(0); i < l; i++ {
		if env.peekAt(start+i) != rune(s[i]) {
			return false
		}
	}
	env.peekAt(start + l)
	return true
}

func (env *hsEnv) seq(s string) bool {
	return env.seqFrom(s, 0)
}

func (env *hsEnv) tokenFrom(s string, start uint32) bool {
	return env.seqFrom(s, start) && hsTokenEnd(env.peekAt(start+uint32(len(s))))
}

func (env *hsEnv) token(s string) bool {
	return env.seq(s) && hsTokenEnd(env.peekAt(uint32(len(s))))
}

func (env *hsEnv) anyTokenFrom(tokens []string, start uint32) bool {
	for _, t := range tokens {
		if env.tokenFrom(t, start) {
			return true
		}
	}
	return false
}

func (env *hsEnv) matchSymop(target string) bool {
	return env.symopLookahead() == uint32(len(target)) && env.seq(target)
}

func (env *hsEnv) takeLine() {
	for env.notEof() && !hsIsNewline(env.peek()) {
		env.advance()
	}
}

func (env *hsEnv) takeLineEscapedNewline() {
	for {
		for env.notEof() && !hsIsNewline(env.peek()) && env.peek() != '\\' {
			env.advance()
		}
		if env.peek() == '\\' {
			env.advance()
			if hsIsSpaceOrTab(env.peek()) {
				for hsIsSpaceOrTab(env.peek()) {
					env.advance()
				}
				if hsIsNewline(env.peek()) {
					env.advance()
				}
			} else {
				env.advance()
			}
		} else {
			return
		}
	}
}

func (env *hsEnv) skipSpace() bool {
	if !hsIsSpaceChar(env.peek()) {
		return false
	}
	env.skip()
	for hsIsSpaceChar(env.peek()) {
		env.skip()
	}
	return true
}

func (env *hsEnv) skipNewlines() bool {
	if !hsIsNewline(env.peek()) {
		return false
	}
	env.skip()
	for hsIsNewline(env.peek()) {
		env.skip()
	}
	return true
}

// skipWhitespace alternates skipping space and newlines and returns which was last.
func (env *hsEnv) skipWhitespace() int {
	space := hsNoSpace
	for {
		if env.skipSpace() {
			space = hsIndented
		} else if env.skipNewlines() {
			space = hsBOL
		} else {
			return space
		}
	}
}

func (env *hsEnv) takeSpaceFrom(start uint32) uint32 {
	return env.advanceWhile(start, hsIsSpaceChar)
}

// ---------------------------------------------------------------------------
// Symop lookahead
// ---------------------------------------------------------------------------

func (env *hsEnv) symopLookahead() uint32 {
	if env.symop == 0 {
		env.symop = env.advanceWhile(0, hsSymopCharNotReserved)
	}
	return env.symop
}

func (env *hsEnv) isSymop() bool {
	return env.symopLookahead() > 0
}

// ---------------------------------------------------------------------------
// Conid
// ---------------------------------------------------------------------------

func (env *hsEnv) conid() uint32 {
	if !hsIsConidStartChar(env.peek0()) {
		return 0
	}
	return env.advanceWhile(1, hsIsInnerIdChar)
}

func (env *hsEnv) qualifiedName(name func() bool) int {
	qualified := false
	for {
		end := env.conid()
		if end == 0 {
			break
		}
		if !env.charAt(end, '.') {
			if qualified {
				return hsQualifiedConid
			}
			break
		}
		qualified = true
		env.resetLookaheadTo(end + 1)
		if name() {
			return hsQualifiedTarget
		}
	}
	return hsNoQualifiedName
}

// ---------------------------------------------------------------------------
// String/char literal helpers
// ---------------------------------------------------------------------------

func (env *hsEnv) oddBackslashesBefore(index int) bool {
	odd := false
	for index >= 0 && env.peekAt(uint32(index)) == '\\' {
		odd = !odd
		index--
	}
	return odd
}

func (env *hsEnv) takeStringLiteral() uint32 {
	end := uint32(1)
	for {
		end = env.advanceUntilChar(end, '"') + 1
		if env.isEof() || !env.oddBackslashesBefore(int(end)-2) {
			return end
		}
	}
}

func (env *hsEnv) takeCharLiteral() uint32 {
	if env.char1('\\') {
		return env.advanceUntilChar(2, '\'') + 2
	}
	if env.charAt(2, '\'') {
		return 3
	}
	return 1
}

// ---------------------------------------------------------------------------
// CPP directives
// ---------------------------------------------------------------------------

var hsCppTokensStart = []string{"if", "ifdef", "ifndef"}
var hsCppTokensElse = []string{"else", "elif", "elifdef", "elifndef"}
var hsCppTokensOther = []string{"define", "undef", "include", "pragma", "error", "warning", "line"}

func (env *hsEnv) cppDirective() int {
	if !env.char0('#') {
		return hsCppNothing
	}
	start := env.takeSpaceFrom(1)
	if env.anyTokenFrom(hsCppTokensStart, start) {
		return hsCppStart
	} else if env.anyTokenFrom(hsCppTokensElse, start) {
		return hsCppElse
	} else if env.tokenFrom("endif", start) {
		return hsCppEnd
	} else if env.anyTokenFrom(hsCppTokensOther, start) || hsIsNewline(env.peekAt(start)) ||
		(env.char1('!') && env.uninitialized()) {
		return hsCppOther
	}
	return hsCppNothing
}

// ---------------------------------------------------------------------------
// Layout start
// ---------------------------------------------------------------------------

func (env *hsEnv) startBrace() int {
	if env.valid(hsSTART_BRACE) {
		env.pushContext(hsBraces, 0)
		return hsSTART_BRACE
	}
	return hsFAIL
}

func (env *hsEnv) endBrace() int {
	if env.valid(hsEND_BRACE) && env.currentContext() == hsBraces {
		env.pop()
		return hsEND_BRACE
	}
	return hsFAIL
}

func (env *hsEnv) validLayoutStartSym() int {
	for i := hsSTART; i < hsEND; i++ {
		if env.valid(i) {
			return i
		}
	}
	return hsFAIL
}

func hsLayoutSort(s int) uint8 {
	switch s {
	case hsSTART_DO:
		return hsDoLayout
	case hsSTART_CASE:
		return hsCaseLayout
	case hsSTART_IF:
		return hsMultiWayIf
	case hsSTART_LET:
		return hsLetLayout
	case hsSTART_QUOTE:
		return hsQuoteLayout
	default:
		return hsDeclLayout
	}
}

type hsStartLayout struct {
	sym  int
	sort uint8
}

func (env *hsEnv) validLayoutStart(next int) hsStartLayout {
	start := hsStartLayout{sym: env.validLayoutStartSym(), sort: hsNoContext}
	if env.uninitialized() || start.sym == hsFAIL {
		return start
	}
	sort := hsLayoutSort(start.sym)
	switch next {
	case hsLBar:
		// ok
	case hsLBraceOpen:
		if env.newlineActive() {
			return start
		}
		sort = hsBraces
		start.sym = hsSTART_EXPLICIT
	default:
		if sort == hsMultiWayIf {
			return start
		}
	}
	start.sort = sort
	return start
}

func (env *hsEnv) indentCanStartLayout(sort uint8, indent uint32) bool {
	if env.currentContext() == hsBraces {
		return true
	}
	cur := env.currentIndent()
	return indent > cur || (indent == cur && sort == hsDoLayout)
}

func (env *hsEnv) startLayout(start hsStartLayout, indent uint32) int {
	if env.inModuleHeader() {
		env.pop()
	} else if start.sort == hsBraces {
		env.markEnd()
	} else if !env.indentCanStartLayout(start.sort, indent) {
		return hsFAIL
	}
	env.pushContext(start.sort, indent)
	return start.sym
}

func (env *hsEnv) startLayoutInterior(next int) int {
	start := env.validLayoutStart(next)
	if start.sort == hsNoContext {
		return hsFAIL
	}
	return env.startLayout(start, env.startColumn())
}

func (env *hsEnv) startLayoutNewline() int {
	start := env.validLayoutStart(env.state.newline.end)
	if start.sort == hsNoContext {
		return hsFAIL
	}
	result := env.startLayout(start, env.state.newline.indent)
	if result != hsFAIL {
		env.state.newline.noSemi = true
	}
	return result
}

func (env *hsEnv) texpContext() int {
	if env.valid(hsSTART_TEXP) {
		env.pushContext(hsTExp, 0)
		return hsSTART_TEXP
	} else if env.valid(hsEND_TEXP) && env.currentContext() == hsTExp {
		env.pop()
		return hsEND_TEXP
	}
	return hsFAIL
}

// ---------------------------------------------------------------------------
// Layout end
// ---------------------------------------------------------------------------

func (env *hsEnv) endLayoutUnchecked() int {
	env.pop()
	return hsEND
}

func (env *hsEnv) endLayout() int {
	if env.valid(hsEND) {
		return env.endLayoutUnchecked()
	}
	return hsFAIL
}

func (env *hsEnv) endLayoutBrace() int {
	if env.valid(hsEND_EXPLICIT) && env.currentContext() == hsBraces {
		env.advanceOver(0)
		env.markEnd()
		env.pop()
		return hsEND_EXPLICIT
	}
	return hsFAIL
}

func (env *hsEnv) endLayoutIndent() int {
	if env.valid(hsEND) && env.indentLess(env.state.newline.indent) {
		if env.topLayout() {
			env.state.contexts[len(env.state.contexts)-1].indent = env.state.newline.indent
			return hsUPDATE
		}
		env.state.newline.skipSemi = false
		return env.endLayoutUnchecked()
	}
	return hsFAIL
}

func (env *hsEnv) endLayoutInfix() int {
	if !env.valid(hsVARSYM) && !env.valid(hsCONSYM) {
		return env.endLayout()
	}
	return hsFAIL
}

func (env *hsEnv) endLayoutWhere() int {
	if env.valid(hsEND) && !env.valid(hsWHERE) && env.isLayoutContext() {
		return env.endLayout()
	}
	return hsFAIL
}

func (env *hsEnv) endLayoutIn() int {
	if env.valid(hsEND) && (!env.valid(hsIN) || env.currentContext() == hsLetLayout) {
		return env.endLayout()
	}
	return hsFAIL
}

func (env *hsEnv) endLayoutDeriving() int {
	if env.valid(hsEND) && !env.valid(hsDERIVING) && !env.topLayout() && env.currentContext() == hsDeclLayout {
		return env.endLayout()
	}
	return hsFAIL
}

func (env *hsEnv) layoutsInTexp() bool {
	if env.isLayoutContext() && len(env.state.contexts) > 1 {
		for i := len(env.state.contexts) - 2; i >= 0; i-- {
			s := env.state.contexts[i].sort
			if s == hsTExp || s == hsBraces {
				return true
			}
			if s > hsBraces {
				break
			}
		}
	}
	return false
}

func (env *hsEnv) tokenEndLayoutTexp() int {
	if env.valid(hsEND) && env.layoutsInTexp() {
		return env.endLayout()
	}
	return hsFAIL
}

func (env *hsEnv) forceEndContext() int {
	for i := len(env.state.contexts) - 1; i >= 0; i-- {
		ctx := env.state.contexts[i].sort
		s := env.contextEndSym(ctx)
		env.pop()
		if s != hsFAIL && env.valid(s) {
			return s
		}
	}
	return hsFAIL
}

// ---------------------------------------------------------------------------
// Operator processing
// ---------------------------------------------------------------------------

func (env *hsEnv) openingToken(i uint32) bool {
	c := env.peekAt(i)
	switch c {
	case 0x27e6, 0x2987, '(', '[', '"':
		return true
	case '{':
		return env.peekAt(i+1) != '-'
	default:
		return hsIsIdChar(c)
	}
}

func hsValidSymopTwoChars(first, second rune) bool {
	switch first {
	case '=':
		return second != '>'
	case '<':
		return second != '-'
	case ':':
		return second != ':'
	}
	return true
}

func (env *hsEnv) lexPrefix(t int) int {
	if env.openingToken(1) {
		return t
	}
	return hsLSymop
}

func hsLexSplice(c rune) int {
	if hsVaridStartChar(c) || c == '(' {
		return hsLDollar
	}
	return hsLSymop
}

func (env *hsEnv) lexSymop() int {
	length := env.symopLookahead()
	if length == 0 {
		return hsLNothing
	}
	c1 := env.unsafePeek(0)
	if length == 1 {
		switch c1 {
		case '?':
			if hsVaridStartChar(env.peek1()) {
				return hsLNothing
			}
			return hsLSymop
		case '#':
			if env.char1(')') {
				return hsLUnboxedClose
			}
			return hsLHash
		case '|':
			if env.char1(']') {
				return hsLQuoteClose
			}
			return hsLBar
		case '!':
			return env.lexPrefix(hsLBang)
		case '~':
			return env.lexPrefix(hsLTilde)
		case '@':
			return env.lexPrefix(hsLAt)
		case '%':
			return env.lexPrefix(hsLPercent)
		case '$':
			return hsLexSplice(env.peek1())
		case '.':
			if hsIsIdChar(env.peek1()) {
				return hsLDotId
			} else if env.openingToken(1) {
				return hsLDotOpen
			}
			return hsLSymop
		case 0x2192, 0x22b8: // -> ⊸
			return hsLArrow
		case 0x21d2: // =>
			return hsLCArrow
		case '=', 0x27e7, 0x2988:
			return hsLTexpCloser
		case '*', '-':
			return hsLSymopSpecial
		case '\\', 0x2190, 0x2200, 0x2237, 0x2605, 0x27e6, 0x2919, 0x291a, 0x291b, 0x291c, 0x2987:
			return hsLNothing
		}
	} else if length == 2 {
		if env.seq("->") {
			return hsLArrow
		}
		if env.seq("=>") {
			return hsLCArrow
		}
		c2 := env.unsafePeek(1)
		switch c1 {
		case '$':
			if c2 == '$' {
				return hsLexSplice(env.peek2())
			}
		case '|':
			if c2 == '|' && env.char2(']') {
				return hsLQuoteClose
			}
		case '.':
			if c2 == '.' {
				return hsLDotDot
			}
			return hsLDotSymop
		case '#':
			if c2 == '#' || c2 == '|' {
				return hsLSymopSpecial
			}
		default:
			if !hsValidSymopTwoChars(c1, c2) {
				return hsLNothing
			}
		}
	} else {
		switch c1 {
		case '-':
			if env.seq("->.") {
				return hsLArrow
			}
		case '.':
			return hsLDotSymop
		}
	}
	return hsLSymop
}

// ---------------------------------------------------------------------------
// Left section operators
// ---------------------------------------------------------------------------

func (env *hsEnv) leftSectionOp(start uint32) int {
	if env.valid(hsLEFT_SECTION) {
		env.advanceBefore(start)
		space := env.skipWhitespace()
		if env.charAt(start, ')') {
			return hsLEFT_SECTION
		}
		if space != hsNoSpace {
			if env.valid(hsNO_SECTION_OP) {
				return hsNO_SECTION_OP
			}
		}
	}
	return hsFAIL
}

func (env *hsEnv) leftSectionTicked() int {
	if env.valid(hsLEFT_SECTION) {
		endTick := env.advanceUntilChar(1, '`')
		if env.charAt(endTick, '`') {
			return env.leftSectionOp(endTick + 1)
		}
	}
	return hsFAIL
}

func (env *hsEnv) finishSymop(s int) int {
	if env.valid(s) || env.valid(hsLEFT_SECTION) {
		afterSymop := env.symopLookahead()
		r := env.leftSectionOp(afterSymop)
		if r != hsFAIL {
			return r
		}
		env.markEnd()
		return s
	}
	return hsFAIL
}

func (env *hsEnv) tightOp(whitespace bool, s int) int {
	if !whitespace {
		if env.valid(s) {
			return s
		}
	}
	return hsFAIL
}

func (env *hsEnv) prefixOrVarsym(whitespace bool, s int) int {
	if whitespace {
		if env.valid(s) {
			return s
		}
	}
	return env.finishSymop(hsVARSYM)
}

func (env *hsEnv) tightOrVarsym(whitespace bool, s int) int {
	r := env.tightOp(whitespace, s)
	if r != hsFAIL {
		return r
	}
	return env.finishSymop(hsVARSYM)
}

func (env *hsEnv) infixOrVarsym(whitespace bool, prefix, tight int) int {
	var s int
	if whitespace {
		s = prefix
	} else {
		s = tight
	}
	if env.valid(s) {
		return s
	}
	return env.finishSymop(hsVARSYM)
}

func (env *hsEnv) qualifiedOp() int {
	r := env.qualifiedName(func() bool { return env.isSymop() })
	if r == hsQualifiedTarget {
		lso := env.leftSectionOp(env.symopLookahead())
		if lso != hsFAIL {
			return lso
		}
		return hsQUALIFIED_OP
	}
	return hsFAIL
}

// ---------------------------------------------------------------------------
// Token lookahead
// ---------------------------------------------------------------------------

func (env *hsEnv) isQqStart() bool {
	end := env.advanceWhile(1, hsQuoterChar)
	return env.charAt(end, '|')
}

func (env *hsEnv) tryEndToken(target string, match int) int {
	if env.token(target) {
		return match
	}
	return hsLNothing
}

func (env *hsEnv) onlyMinus() bool {
	i := uint32(2)
	for env.peekAt(i) == '-' {
		i++
	}
	return !hsSymopCharNotReserved(env.peekAt(i))
}

func (env *hsEnv) lineCommentHerald() bool {
	return env.seq("--") && env.onlyMinus()
}

func (env *hsEnv) lexCpp() int {
	switch env.cppDirective() {
	case hsCppElse:
		return hsLCppElse
	case hsCppNothing:
		return hsLNothing
	default:
		return hsLCpp
	}
}

func (env *hsEnv) lexExtras(bol bool) int {
	switch env.peek0() {
	case '{':
		if env.char1('-') {
			if env.char2('#') {
				return hsLPragma
			}
			return hsLBlockComment
		}
	case '#':
		if bol {
			return env.lexCpp()
		}
	case '-':
		if env.lineCommentHerald() {
			return hsLLineComment
		}
	}
	return hsLNothing
}

func (env *hsEnv) lex(bol bool) int {
	r := env.lexExtras(bol)
	if r != hsLNothing {
		return r
	}
	if hsSymopCharNotReserved(env.peek0()) {
		r = env.lexSymop()
		if r != hsLNothing {
			return r
		}
	} else {
		switch env.peek0() {
		case 'w':
			return env.tryEndToken("where", hsLWhere)
		case 'i':
			return env.tryEndToken("in", hsLIn)
		case 't':
			return env.tryEndToken("then", hsLThen)
		case 'e':
			return env.tryEndToken("else", hsLElse)
		case 'd':
			return env.tryEndToken("deriving", hsLDeriving)
		case 'm':
			if (env.uninitialized() || env.inModuleHeader()) && env.token("module") {
				return hsLModule
			}
		case '{':
			return hsLBraceOpen
		case '}':
			return hsLBraceClose
		case ';':
			return hsLSemi
		case '`':
			return hsLTick
		case '[':
			if env.valid(hsQQ_START) && env.isQqStart() {
				return hsLBracketOpen
			}
		case ']', ')', ',':
			return hsLTexpCloser
		default:
			if hsIsConidStartChar(env.peek0()) {
				return hsLUpper
			}
		}
	}
	return hsLNothing
}

// ---------------------------------------------------------------------------
// CPP
// ---------------------------------------------------------------------------

func (env *hsEnv) cppElse(emit bool) int {
	nesting := uint32(1)
	for {
		env.takeLineEscapedNewline()
		if emit {
			env.markEnd()
		}
		env.advance()
		env.resetLookahead()
		switch env.cppDirective() {
		case hsCppStart:
			nesting++
		case hsCppEnd:
			nesting--
		}
		if env.isEof() || nesting == 0 {
			break
		}
	}
	if emit {
		return hsCPP
	}
	return hsFAIL
}

func (env *hsEnv) cppLine() int {
	env.takeLineEscapedNewline()
	env.markEnd()
	return hsCPP
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

func (env *hsEnv) commentType() int {
	i := uint32(2)
	for env.peekAt(i) == '-' {
		i++
	}
	for env.notEof() {
		c := env.peekAt(i)
		i++
		if c == '|' || c == '^' {
			return hsHADDOCK
		}
		if !hsIsSpaceChar(c) {
			break
		}
	}
	return hsCOMMENT
}

func (env *hsEnv) inlineComment() int {
	sym := env.commentType()
	for {
		env.takeLine()
		env.markEnd()
		env.advance()
		env.resetLookahead()
		if !env.lineCommentHerald() {
			break
		}
	}
	return sym
}

func (env *hsEnv) consumeBlockComment(col uint32) uint32 {
	level := uint32(0)
	for {
		if env.isEof() {
			return col
		}
		col++
		c := env.peek()
		switch c {
		case '{':
			env.advance()
			if env.peek() == '-' {
				env.advance()
				col++
				level++
			}
		case '-':
			env.advance()
			if env.peek() == '}' {
				env.advance()
				col++
				if level == 0 {
					return col
				}
				level--
			}
		case '\n', '\r', '\f':
			env.advance()
			col = 0
		case '\t':
			env.advance()
			col += 7
		default:
			env.advance()
		}
	}
}

func (env *hsEnv) blockComment() int {
	sym := env.commentType()
	env.consumeBlockComment(uint32(len(env.state.lookahead.contents)))
	env.markEnd()
	return sym
}

// ---------------------------------------------------------------------------
// Pragma
// ---------------------------------------------------------------------------

func (env *hsEnv) consumePragma() bool {
	if env.seq("{-#") {
		for !env.seq("#-}") && env.notEof() {
			env.resetLookahead()
			env.advanceOver(0)
		}
		return true
	}
	return false
}

func (env *hsEnv) pragma() int {
	if env.consumePragma() {
		env.markEnd()
		if env.state.newline.state != hsNInactive {
			env.state.newline.state = hsNResume
		}
		return hsPRAGMA
	}
	return hsFAIL
}

// ---------------------------------------------------------------------------
// Quasiquote
// ---------------------------------------------------------------------------

func (env *hsEnv) qqBody() int {
	for {
		if env.isEof() {
			return hsQQ_BODY
		} else if env.peek() == 0x27e7 {
			env.markEnd()
			return hsQQ_BODY
		} else if env.peek() == '|' {
			env.markEnd()
			env.advance()
			if env.peek() == ']' {
				return hsQQ_BODY
			}
		} else {
			env.advance()
		}
	}
}

// ---------------------------------------------------------------------------
// Semicolons
// ---------------------------------------------------------------------------

func (env *hsEnv) explicitSemicolon() int {
	if env.valid(hsSEMICOLON) && !env.state.newline.skipSemi {
		env.state.newline.skipSemi = true
		return hsUPDATE
	}
	return hsFAIL
}

func (env *hsEnv) resolveSemicolon(next int) int {
	if env.state.newline.skipSemi {
		switch next {
		case hsLLineComment, hsLBlockComment, hsLPragma, hsLSemi:
			// keep skip_semi
		default:
			env.state.newline.skipSemi = false
			return hsUPDATE
		}
	}
	return hsFAIL
}

func (env *hsEnv) semicolon() int {
	if env.isSemicolonContext() &&
		!(env.state.newline.noSemi || env.state.newline.skipSemi) &&
		env.indentLesseq(env.state.newline.indent) {
		env.state.newline.noSemi = true
		return hsSEMICOLON
	}
	return hsFAIL
}

// ---------------------------------------------------------------------------
// High-level Lexed dispatch
// ---------------------------------------------------------------------------

func (env *hsEnv) processTokenSafe(next int) int {
	switch next {
	case hsLWhere:
		return env.endLayoutWhere()
	case hsLIn:
		return env.endLayoutIn()
	case hsLThen, hsLElse:
		return env.endLayout()
	case hsLDeriving:
		return env.endLayoutDeriving()
	case hsLBar:
		if !env.valid(hsBAR) {
			return env.endLayout()
		}
	case hsLPragma:
		return env.pragma()
	case hsLBlockComment:
		return env.blockComment()
	case hsLLineComment:
		return env.inlineComment()
	case hsLCppElse:
		return env.cppElse(true)
	case hsLCpp:
		return env.cppLine()
	case hsLSymop, hsLTick, hsLHash:
		return env.endLayoutInfix()
	case hsLUnboxedClose:
		r := env.tokenEndLayoutTexp()
		if r != hsFAIL {
			return r
		}
		return env.endLayoutInfix()
	case hsLArrow:
		if !env.valid(hsARROW) {
			return env.tokenEndLayoutTexp()
		}
	case hsLTexpCloser:
		return env.tokenEndLayoutTexp()
	case hsLQuoteClose:
		return env.endLayout()
	}
	return hsFAIL
}

func (env *hsEnv) processTokenSymop(whitespace bool, next int) int {
	switch next {
	case hsLDotDot:
		if env.valid(hsDOTDOT) {
			return hsDOTDOT
		}
		return env.tightOp(whitespace, hsQUAL_DOT)
	case hsLDotId:
		var s int
		if whitespace {
			s = hsPREFIX_DOT
		} else {
			s = hsTIGHT_DOT
		}
		if env.valid(s) {
			return s
		}
		return env.tightOp(whitespace, hsQUAL_DOT)
	case hsLDotSymop:
		return env.tightOrVarsym(whitespace, hsQUAL_DOT)
	case hsLDotOpen:
		return env.prefixOrVarsym(whitespace, hsPREFIX_DOT)
	case hsLBang:
		return env.infixOrVarsym(whitespace, hsPREFIX_BANG, hsTIGHT_BANG)
	case hsLTilde:
		return env.infixOrVarsym(whitespace, hsPREFIX_TILDE, hsTIGHT_TILDE)
	case hsLAt:
		return env.infixOrVarsym(whitespace, hsPREFIX_AT, hsTIGHT_AT)
	case hsLPercent:
		return env.prefixOrVarsym(whitespace, hsPREFIX_PERCENT)
	case hsLSymop:
		if env.char0(':') {
			return env.finishSymop(hsCONSYM)
		}
		return env.finishSymop(hsVARSYM)
	case hsLSymopSpecial:
		r := env.leftSectionOp(env.symopLookahead())
		if r != hsFAIL {
			return r
		}
		if env.valid(hsMINUS) && env.matchSymop("-") {
			return hsMINUS
		}
	case hsLUnboxedClose, hsLHash:
		return env.leftSectionOp(env.symopLookahead())
	case hsLTick:
		return env.leftSectionTicked()
	case hsLUpper:
		if env.valid(hsQUALIFIED_OP) || env.valid(hsLEFT_SECTION) {
			r := env.qualifiedOp()
			if r != hsFAIL {
				return r
			}
		}
	}
	return hsFAIL
}

func (env *hsEnv) processTokenSplice(next int) int {
	if next == hsLDollar {
		if env.valid(hsSPLICE) {
			return hsSPLICE
		}
	}
	return hsFAIL
}

func (env *hsEnv) processTokenInterior(next int) int {
	switch next {
	case hsLBraceClose:
		r := env.endLayoutBrace()
		if r != hsFAIL {
			return r
		}
		return env.tokenEndLayoutTexp()
	case hsLModule:
		return hsFAIL
	case hsLSemi:
		return env.explicitSemicolon()
	case hsLBracketOpen:
		return hsQQ_START
	}
	r := env.processTokenSafe(next)
	if r != hsFAIL {
		return r
	}
	return env.startLayoutInterior(next)
}

func (env *hsEnv) processTokenInit(indent uint32, next int) int {
	switch next {
	case hsLModule:
		env.pushContext(hsModuleHeader, 0)
		return hsUPDATE
	case hsLBraceOpen:
		env.advanceOver(0)
		env.markEnd()
		env.pushContext(hsBraces, indent)
		return hsSTART_EXPLICIT
	default:
		env.pushContext(hsDeclLayout, indent)
		return hsSTART
	}
}

// ---------------------------------------------------------------------------
// Newline actions
// ---------------------------------------------------------------------------

func (env *hsEnv) newlineExtras(space int) int {
	bol := space == hsBOL || (space == hsNoSpace && env.newlineInit())
	next := env.lexExtras(bol)
	return env.processTokenSafe(next)
}

func (env *hsEnv) newlineProcess() int {
	indent := env.state.newline.indent
	end := env.state.newline.end

	r := env.endLayoutIndent()
	if r != hsFAIL {
		return r
	}
	r = env.processTokenSafe(end)
	if r != hsFAIL {
		return r
	}
	space := env.skipWhitespace()
	env.markEnd()
	if env.state.newline.unsafe {
		r = env.newlineExtras(space)
		if r != hsFAIL {
			return r
		}
	}
	if !env.state.newline.eof {
		r = env.startLayoutNewline()
		if r != hsFAIL {
			return r
		}
	}
	r = env.semicolon()
	if r != hsFAIL {
		return r
	}
	env.resetNewline()
	if env.uninitialized() {
		r = env.processTokenInit(indent, end)
		if r != hsFAIL {
			return r
		}
	} else {
		r = env.processTokenSymop(true, end)
		if r != hsFAIL {
			return r
		}
		r = env.processTokenSplice(end)
		if r != hsFAIL {
			return r
		}
	}
	return hsUPDATE
}

func (env *hsEnv) newlinePost() int {
	res := env.newlineProcess()
	if env.newlineInit() {
		env.state.newline.state = hsNProcess
	}
	return res
}

func (env *hsEnv) newlineLookahead(nl *hsNewline) {
	for {
		c := env.peek0()
		switch {
		case hsIsNewline(c):
			env.skipOver(0)
			nl.indent = 0
		case c == '\t':
			env.skipOver(0)
			nl.indent += 8
		default:
			if hsIsSpaceChar(c) {
				env.skipOver(0)
				nl.indent++
			} else {
				nl.end = env.lex(nl.indent == 0)
				nl.unsafe = nl.unsafe || !env.noLookahead()
				switch nl.end {
				case hsLEof:
					nl.indent = 0
					nl.eof = true
					return
				case hsLThen, hsLElse, hsLSemi:
					nl.noSemi = true
					return
				case hsLBlockComment:
					nl.indent = env.consumeBlockComment(nl.indent + 2)
				case hsLLineComment:
					nl.indent = 0
					env.takeLine()
				case hsLCppElse:
					env.cppElse(false)
					env.takeLineEscapedNewline()
				case hsLCpp:
					env.takeLineEscapedNewline()
				default:
					return
				}
			}
		}
		env.resetLookahead()
	}
}

func (env *hsEnv) newlineStart() int {
	env.state.newline.state = hsNInit
	env.newlineLookahead(&env.state.newline)
	if env.state.newline.unsafe {
		return hsUPDATE
	}
	return env.newlinePost()
}

func (env *hsEnv) newlineResume() int {
	indent := env.state.newline.indent
	env.skipSpace()
	env.resetNewline()
	env.state.newline.indent = indent
	return env.newlineStart()
}

// ---------------------------------------------------------------------------
// Constraint lookahead
// ---------------------------------------------------------------------------

type hsCtrState struct {
	reset        uint32
	brackets     uint32
	context      bool
	infix        bool
	dataInfix    bool
	typeInstance bool
}

func (state *hsCtrState) bracketOpen() int {
	state.brackets++
	state.reset = 1
	return hsCtrUndecided
}

func (state *hsCtrState) bracketClose() int {
	if state.brackets == 0 {
		return hsCtrImpossible
	}
	state.brackets--
	state.reset = 1
	return hsCtrUndecided
}

func (env *hsEnv) ctrStopOnToken(target string) int {
	if env.token(target) {
		return hsCtrImpossible
	}
	return hsCtrUndecided
}

func (env *hsEnv) ctrTop(next int) int {
	switch next {
	case hsLCArrow:
		return hsCtrArrowFound
	case hsLSymop, hsLSymopSpecial, hsLTilde, hsLTick:
		return hsCtrInfixFound
	case hsLBar:
		return hsCtrBarFound
	case hsLArrow, hsLWhere, hsLDotDot, hsLSemi:
		// impossible
	case hsLTexpCloser:
		if env.peek0() == '=' {
			return hsCtrEqualsFound
		}
	default:
		c := env.peek0()
		switch c {
		case '=':
			return hsCtrEqualsFound
		case 0x2200: // forall
			// impossible
		case ':':
			if env.char1(':') {
				break
			}
			return hsCtrUndecided
		case 'f':
			r := env.ctrStopOnToken("forall")
			if r != hsCtrUndecided {
				return r
			}
			return env.ctrStopOnToken("family")
		case 'i':
			return env.ctrStopOnToken("instance")
		default:
			return hsCtrUndecided
		}
	}
	return hsCtrImpossible
}

func (env *hsEnv) ctrLookaheadStep(state *hsCtrState, next int) int {
	state.reset = 1
	switch next {
	case hsLBraceClose:
		return state.bracketClose()
	case hsLUnboxedClose:
		r := state.bracketClose()
		if r != hsCtrUndecided {
			return r
		}
		state.reset = 2
		return hsCtrUndecided
	case hsLBraceOpen:
		return state.bracketOpen()
	case hsLSymopSpecial, hsLSymop:
		state.reset = env.symopLookahead()
	case hsLUpper:
		state.reset = env.conid()
		return hsCtrUndecided
	case hsLDotId:
		return hsCtrUndecided
	case hsLPragma:
		if env.consumePragma() {
			state.reset = 3
		}
		return hsCtrUndecided
	case hsLTexpCloser, hsLNothing:
		c := env.peek0()
		switch c {
		case ')', ']':
			return state.bracketClose()
		case '(', '[':
			return state.bracketOpen()
		case '"':
			state.reset = env.takeStringLiteral()
			return hsCtrUndecided
		case '\'':
			state.reset = env.takeCharLiteral()
			return hsCtrUndecided
		default:
			if hsVaridStartChar(c) {
				state.reset = env.advanceWhile(1, hsIsIdChar)
			}
		}
	}
	if state.brackets != 0 {
		return hsCtrUndecided
	}
	return env.ctrTop(next)
}

func (env *hsEnv) constraintLookahead() int {
	state := hsCtrState{}
	done := false
	for !done && env.notEof() {
		nl := hsNewline{indent: 99999}
		env.newlineLookahead(&nl)
		if nl.indent <= env.currentIndent() && env.currentContext() != hsBraces {
			break
		}
		result := env.ctrLookaheadStep(&state, nl.end)
		switch result {
		case hsCtrArrowFound:
			state.context = true
			done = true
		case hsCtrInfixFound:
			if env.char0(':') || env.char0('`') {
				state.dataInfix = true
			}
			state.infix = true
			done = !env.valid(hsCONTEXT)
		case hsCtrEqualsFound:
			done = !env.valid(hsTYPE_INSTANCE)
			state.typeInstance = true
		case hsCtrBarFound:
			done = true
			state.typeInstance = false
		case hsCtrImpossible:
			done = true
		case hsCtrUndecided:
			// continue
		}
		env.resetLookaheadTo(state.reset)
		state.reset = 0
	}
	if state.context {
		if env.valid(hsCONTEXT) {
			return hsCONTEXT
		}
	}
	if state.infix {
		if env.valid(hsINFIX) {
			return hsINFIX
		}
	}
	if state.dataInfix {
		if env.valid(hsDATA_INFIX) {
			return hsDATA_INFIX
		}
	}
	if state.typeInstance {
		if env.valid(hsTYPE_INSTANCE) {
			return hsTYPE_INSTANCE
		}
	}
	return hsFAIL
}

func (env *hsEnv) processTokenConstraint() int {
	if env.valid(hsCONTEXT) || env.valid(hsINFIX) || env.valid(hsDATA_INFIX) || env.valid(hsTYPE_INSTANCE) {
		return env.constraintLookahead()
	}
	return hsFAIL
}

// ---------------------------------------------------------------------------
// Interior
// ---------------------------------------------------------------------------

func (env *hsEnv) interior(whitespace bool) int {
	next := env.lex(false)

	r := env.resolveSemicolon(next)
	if r != hsFAIL {
		return r
	}
	r = env.processTokenInterior(next)
	if r != hsFAIL {
		return r
	}
	r = env.processTokenSymop(whitespace, next)
	if r != hsFAIL {
		return r
	}
	r = env.processTokenConstraint()
	if r != hsFAIL {
		return r
	}
	r = env.processTokenSplice(next)
	if r != hsFAIL {
		return r
	}
	return hsFAIL
}

// ---------------------------------------------------------------------------
// Pre-whitespace commands
// ---------------------------------------------------------------------------

func (env *hsEnv) preWsCommands() int {
	r := env.texpContext()
	if r != hsFAIL {
		return r
	}
	r = env.startBrace()
	if r != hsFAIL {
		return r
	}
	r = env.endBrace()
	if r != hsFAIL {
		return r
	}
	if env.valid(hsQQ_BODY) {
		return env.qqBody()
	}
	if env.newlineActive() {
		r = env.newlinePost()
		if r != hsFAIL {
			return r
		}
	} else if env.state.newline.state == hsNResume {
		r = env.newlineResume()
		if r != hsFAIL {
			return r
		}
	}
	return hsFAIL
}

func (env *hsEnv) scanMain() int {
	env.markEnd()
	r := env.preWsCommands()
	if r != hsFAIL {
		return r
	}
	whitespace := env.skipSpace()
	if hsIsNewline(env.peek()) {
		return env.newlineStart()
	} else if env.notEof() {
		return env.interior(whitespace)
	}
	return hsFAIL
}

func (env *hsEnv) processResult(result int) bool {
	if result == hsFAIL && env.isEof() && env.noLookahead() {
		env.markEnd()
		if env.valid(hsEND) {
			result = env.endLayoutUnchecked()
		} else if env.valid(hsSEMICOLON) {
			result = hsSEMICOLON
		} else {
			result = env.forceEndContext()
		}
	}
	return env.setResultSymbol(result)
}

func (env *hsEnv) scan() bool {
	if env.afterError() {
		return false
	}
	result := env.scanMain()
	return env.processResult(result)
}

// ---------------------------------------------------------------------------
// Serialization: compact binary format
// ---------------------------------------------------------------------------

// Serialization layout:
//   [1 byte] newline.state
//   [1 byte] newline.end (as uint8)
//   [4 bytes] newline.indent (uint32 LE)
//   [1 byte] flags (bit0=eof, bit1=noSemi, bit2=skipSemi, bit3=unsafe)
//   [2 bytes] number of contexts (uint16 LE)
//   per context:
//     [1 byte] sort
//     [4 bytes] indent (uint32 LE)

const hsSerializeHeaderSize = 1 + 1 + 4 + 1 + 2 // 9 bytes
const hsSerializeContextSize = 1 + 4            // 5 bytes

func hsSerialize(state *hsState, buf []byte) int {
	needed := hsSerializeHeaderSize + len(state.contexts)*hsSerializeContextSize
	if needed > len(buf) {
		return 0
	}
	i := 0
	buf[i] = state.newline.state
	i++
	buf[i] = uint8(state.newline.end)
	i++
	binary.LittleEndian.PutUint32(buf[i:], state.newline.indent)
	i += 4
	flags := byte(0)
	if state.newline.eof {
		flags |= 1
	}
	if state.newline.noSemi {
		flags |= 2
	}
	if state.newline.skipSemi {
		flags |= 4
	}
	if state.newline.unsafe {
		flags |= 8
	}
	buf[i] = flags
	i++
	binary.LittleEndian.PutUint16(buf[i:], uint16(len(state.contexts)))
	i += 2
	for _, ctx := range state.contexts {
		buf[i] = ctx.sort
		i++
		binary.LittleEndian.PutUint32(buf[i:], ctx.indent)
		i += 4
	}
	return i
}

func hsDeserialize(state *hsState, buf []byte) {
	state.lookahead.contents = state.lookahead.contents[:0]
	state.lookahead.offset = 0
	if len(buf) == 0 {
		state.contexts = state.contexts[:0]
		state.newline = hsNewline{state: hsNResume}
		return
	}
	if len(buf) < hsSerializeHeaderSize {
		state.contexts = state.contexts[:0]
		state.newline = hsNewline{}
		return
	}
	i := 0
	state.newline.state = buf[i]
	i++
	state.newline.end = int(buf[i])
	i++
	state.newline.indent = binary.LittleEndian.Uint32(buf[i:])
	i += 4
	flags := buf[i]
	i++
	state.newline.eof = flags&1 != 0
	state.newline.noSemi = flags&2 != 0
	state.newline.skipSemi = flags&4 != 0
	state.newline.unsafe = flags&8 != 0
	nCtx := int(binary.LittleEndian.Uint16(buf[i:]))
	i += 2
	if cap(state.contexts) < nCtx {
		state.contexts = make([]hsContext, nCtx)
	} else {
		state.contexts = state.contexts[:nCtx]
	}
	for j := 0; j < nCtx && i+hsSerializeContextSize <= len(buf); j++ {
		state.contexts[j].sort = buf[i]
		i++
		state.contexts[j].indent = binary.LittleEndian.Uint32(buf[i:])
		i += 4
	}
}

// ---------------------------------------------------------------------------
// ExternalScanner interface
// ---------------------------------------------------------------------------

// HaskellExternalScanner implements the gotreesitter.ExternalScanner interface.
type HaskellExternalScanner struct{}

func (HaskellExternalScanner) Create() any {
	return &hsState{
		contexts:  make([]hsContext, 0, 8),
		lookahead: hsLookahead{contents: make([]rune, 0, 32)},
	}
}

func (HaskellExternalScanner) Destroy(_ any) {}

func (HaskellExternalScanner) Serialize(payload any, buf []byte) int {
	state := payload.(*hsState)
	return hsSerialize(state, buf)
}

func (HaskellExternalScanner) Deserialize(payload any, buf []byte) {
	state := payload.(*hsState)
	hsDeserialize(state, buf)
}

func (HaskellExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*hsState)
	// Reset transient lookahead state for each scan run.
	state.lookahead.contents = state.lookahead.contents[:0]
	state.lookahead.offset = 0
	env := hsEnvNew(lexer, validSymbols, state)
	return env.scan()
}
