//go:build !grammar_subset || grammar_subset_purescript

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the PureScript grammar.
// Must match the order of external symbols in the generated PureScript grammar.
const (
	psTokSemicolon = 0  // _layout_semicolon
	psTokStart     = 1  // _layout_start
	psTokEnd       = 2  // _layout_end
	psTokDot       = 3  // _dot
	psTokWhere     = 4  // where
	psTokTyconsym  = 5  // type_operator
	psTokComment   = 6  // comment
	psTokComma     = 7  // comma
	psTokAtsign    = 8  // @
	psTokEquals    = 9  // =
	psTokBar       = 10 // |
	psTokIn        = 11 // in
	psTokIndent    = 12 // _token1 (dummy indent symbol)
	psTokEmpty     = 13 // empty_file
	psTokFail      = 14 // internal failure sentinel (not a real token)
)

// Concrete symbol IDs from the generated PureScript grammar ExternalSymbols.
const (
	psSymSemicolon gotreesitter.Symbol = 72
	psSymStart     gotreesitter.Symbol = 73
	psSymEnd       gotreesitter.Symbol = 74
	psSymDot       gotreesitter.Symbol = 75
	psSymWhere     gotreesitter.Symbol = 76
	psSymTyconsym  gotreesitter.Symbol = 77
	psSymComment   gotreesitter.Symbol = 78
	psSymComma     gotreesitter.Symbol = 79
	psSymAtsign    gotreesitter.Symbol = 33
	psSymEquals    gotreesitter.Symbol = 32
	psSymBar       gotreesitter.Symbol = 28
	psSymIn        gotreesitter.Symbol = 51
	psSymIndent    gotreesitter.Symbol = 71
	psSymEmpty     gotreesitter.Symbol = 80
)

// Map from external token index to concrete symbol ID.
var psSymMap = [15]gotreesitter.Symbol{
	psSymSemicolon,
	psSymStart,
	psSymEnd,
	psSymDot,
	psSymWhere,
	psSymTyconsym,
	psSymComment,
	psSymComma,
	psSymAtsign,
	psSymEquals,
	psSymBar,
	psSymIn,
	psSymIndent,
	psSymEmpty,
	0, // FAIL has no real symbol
}

// psScannerState holds the indent stack for the PureScript external scanner.
// The stack tracks layout indentation levels. Each entry is a column number (uint16).
type psScannerState struct {
	indents []uint16
}

func (s *psScannerState) push(indent uint16) {
	s.indents = append(s.indents, indent)
}

func (s *psScannerState) pop() {
	if len(s.indents) > 0 {
		s.indents = s.indents[:len(s.indents)-1]
	}
}

func (s *psScannerState) back() uint16 {
	if len(s.indents) == 0 {
		return 0
	}
	return s.indents[len(s.indents)-1]
}

func (s *psScannerState) indentExists() bool {
	return len(s.indents) > 0
}

func (s *psScannerState) keepLayout(indent uint16) bool {
	return s.indentExists() && indent >= s.back()
}

func (s *psScannerState) sameIndent(indent uint32) bool {
	return s.indentExists() && uint16(indent) == s.back()
}

func (s *psScannerState) smallerIndent(indent uint32) bool {
	return s.indentExists() && uint16(indent) < s.back()
}

func (s *psScannerState) indentLessEq(indent uint32) bool {
	return s.indentExists() && uint16(indent) <= s.back()
}

func (s *psScannerState) uninitialized() bool {
	return !s.indentExists()
}

// psResult is a scanner result analogous to the C scanner's Result struct.
// finished=true means the scanner has made a decision.
// sym=psTokFail means failure; any other sym means success with that token.
type psResult struct {
	sym      int
	finished bool
}

var psResCont = psResult{sym: psTokFail, finished: false}
var psResFail = psResult{sym: psTokFail, finished: true}

func psResFinish(sym int) psResult {
	return psResult{sym: sym, finished: true}
}

// psState bundles the scanner state, lexer, and valid symbols for convenient passing.
type psState struct {
	lexer   *gotreesitter.ExternalLexer
	symbols []bool
	indents *psScannerState
}

func psValid(symbols []bool, idx int) bool {
	return idx >= 0 && idx < len(symbols) && symbols[idx]
}

func (st *psState) sym(idx int) bool {
	return psValid(st.symbols, idx)
}

func (st *psState) peek() rune {
	return st.lexer.Lookahead()
}

func (st *psState) advance() {
	st.lexer.Advance(false)
}

func (st *psState) skip() {
	st.lexer.Advance(true)
}

func (st *psState) markEnd() {
	st.lexer.MarkEnd()
}

func (st *psState) column() uint32 {
	return st.lexer.Column()
}

func (st *psState) isEOF() bool {
	return st.peek() == 0
}

// allSyms returns true when all tokens up to and including EMPTY are valid
// (indicates error recovery mode).
func (st *psState) allSyms() bool {
	for i := 0; i <= psTokEmpty; i++ {
		if !st.sym(i) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Character classification
// ---------------------------------------------------------------------------

func psVaridStartChar(c rune) bool {
	return c == '_' || unicode.IsLower(c)
}

func psIsNewline(c rune) bool {
	switch c {
	case '\n', '\r', '\f':
		return true
	}
	return false
}

func psTokenEnd(c rune) bool {
	switch c {
	case ' ', '\f', '\n', '\r', '\t', '\v', 0, '(', ')', '[', ']':
		return true
	}
	return false
}

// psUnicodeSymbol tests whether a character is a unicode symbol in categories
// used by PureScript/Haskell for symbolic operators:
// ConnectorPunctuation, DashPunctuation, OtherPunctuation,
// MathSymbol, CurrencySymbol, ModifierSymbol, OtherSymbol.
func psUnicodeSymbol(c rune) bool {
	if c < 128 {
		return false // ASCII symbols handled by psSymbolic
	}
	return unicode.Is(unicode.Pc, c) || // ConnectorPunctuation
		unicode.Is(unicode.Pd, c) || // DashPunctuation
		unicode.Is(unicode.Po, c) || // OtherPunctuation
		unicode.Is(unicode.Sm, c) || // MathSymbol
		unicode.Is(unicode.Sc, c) || // CurrencySymbol
		unicode.Is(unicode.Sk, c) || // ModifierSymbol
		unicode.Is(unicode.So, c) // OtherSymbol
}

func psSymbolic(c rune) bool {
	switch c {
	case '!', '#', '$', '%', '&', '*', '+', '.', '/', '<', '>',
		'?', '^', ':', '=', '-', '~', '@', '\\', '|':
		return true
	}
	return psUnicodeSymbol(c)
}

// Symbolic operator classification
const (
	psSOp       = iota // regular operator
	psStar             // *
	psTilde            // ~
	psSEquals          // =
	psSAtsign          // @
	psSImplicit        // ?varid
	psSBar             // |
	psSComment         // --...
	psSInvalid         // not a valid operator
)

// ---------------------------------------------------------------------------
// String and token matching helpers
// ---------------------------------------------------------------------------

// psSeq checks if the characters starting at the current position match the string s,
// advancing past them. Returns false (and leaves the lexer in an indeterminate position)
// if they don't match.
func psSeq(s string, st *psState) bool {
	for _, c := range s {
		if st.peek() != c {
			return false
		}
		st.advance()
	}
	return true
}

// psToken checks if the current position starts with the string s followed by a token-ending
// character (whitespace, brackets, or EOF).
func psToken(s string, st *psState) bool {
	return psSeq(s, st) && psTokenEnd(st.peek())
}

// psReadSymop reads a sequence of symbolic characters and classifies it.
func psReadSymop(st *psState) int {
	var chars []rune
	for psSymbolic(st.peek()) {
		chars = append(chars, st.peek())
		st.advance()
	}
	return psClassifySymop(chars, st)
}

func psClassifySymop(chars []rune, st *psState) int {
	if len(chars) == 0 {
		return psSInvalid
	}
	c := chars[0]
	if len(chars) == 1 {
		if c == '#' && psVaridStartChar(st.peek()) {
			return psSInvalid
		}
		if c == '?' && psVaridStartChar(st.peek()) {
			return psSImplicit
		}
		if c == '|' {
			return psSBar
		}
		switch c {
		case '*':
			return psStar
		case '~':
			return psTilde
		case '=':
			return psSEquals
		case '@':
			return psSAtsign
		case '.', '\\':
			return psSInvalid
		default:
			return psSOp
		}
	}
	// Multi-character
	isComment := chars[0] == '-' && chars[1] == '-'
	if isComment {
		return psSComment
	}
	if len(chars) == 2 {
		if !psValidSymopTwoChars(chars[0], chars[1]) {
			return psSInvalid
		}
	}
	return psSOp
}

func psValidSymopTwoChars(first, second rune) bool {
	switch first {
	case '-':
		return second != '-' && second != '>'
	case '=':
		return second != '>'
	case '<':
		return second != '-'
	case ':':
		return second != ':'
	}
	return true
}

// psExpressionOp returns true for symbolic types that can close a layout on a newline.
func psExpressionOp(symType int) bool {
	return symType == psSOp || symType == psStar
}

// ---------------------------------------------------------------------------
// Whitespace and indentation
// ---------------------------------------------------------------------------

func psSkipSpace(st *psState) {
	for {
		switch st.peek() {
		case ' ', '\t':
			st.skip()
		default:
			return
		}
	}
}

// psCountIndent skips whitespace/newlines and returns the indent of the first
// non-whitespace character on the next non-empty line.
func psCountIndent(st *psState) uint32 {
	var indent uint32
	for {
		switch st.peek() {
		case '\n', '\r', '\f':
			st.skip()
			indent = 0
		case ' ':
			st.skip()
			indent++
		case '\t':
			st.skip()
			indent += 8
		default:
			return indent
		}
	}
}

// ---------------------------------------------------------------------------
// Layout helpers
// ---------------------------------------------------------------------------

func psLayoutEnd(st *psState) psResult {
	if st.sym(psTokEnd) {
		st.indents.pop()
		return psResFinish(psTokEnd)
	}
	return psResCont
}

func psEndOrSemicolon(st *psState) psResult {
	res := psLayoutEnd(st)
	if res.finished {
		return res
	}
	if st.sym(psTokSemicolon) {
		return psResFinish(psTokSemicolon)
	}
	return psResCont
}

// ---------------------------------------------------------------------------
// Main parsing logic
// ---------------------------------------------------------------------------

func psEof(st *psState) psResult {
	if st.isEOF() {
		if st.sym(psTokEmpty) {
			return psResFinish(psTokEmpty)
		}
		res := psEndOrSemicolon(st)
		if res.finished {
			return res
		}
		return psResFail
	}
	return psResCont
}

func psInitialize(col uint32, st *psState) psResult {
	if st.indents.uninitialized() {
		st.markEnd()
		if psToken("module", st) {
			return psResFail
		}
		st.indents.push(uint16(col))
		return psResFinish(psTokIndent)
	}
	return psResCont
}

func psInitializeInit(st *psState) psResult {
	if st.indents.uninitialized() {
		col := st.column()
		if col == 0 {
			return psInitialize(col, st)
		}
	}
	return psResCont
}

func psDot(st *psState) psResult {
	if st.sym(psTokDot) && st.peek() == '.' {
		st.advance()
		st.markEnd()
		if st.sym(psTokDot) {
			return psResFinish(psTokDot)
		}
	}
	return psResCont
}

func psDedent(indent uint32, st *psState) psResult {
	if st.indents.smallerIndent(indent) {
		return psLayoutEnd(st)
	}
	return psResCont
}

func psNewlineWhere(indent uint32, st *psState) psResult {
	// is_newline_where: keep_layout && (SEMICOLON || END) && !WHERE && peek == 'w'
	if st.indents.keepLayout(uint16(indent)) &&
		(st.sym(psTokSemicolon) || st.sym(psTokEnd)) &&
		!st.sym(psTokWhere) &&
		st.peek() == 'w' {
		st.markEnd()
		if psToken("where", st) {
			return psEndOrSemicolon(st)
		}
		return psResFail
	}
	return psResCont
}

func psNewlineSemicolon(indent uint32, st *psState) psResult {
	if st.sym(psTokSemicolon) && st.indents.sameIndent(indent) {
		return psResFinish(psTokSemicolon)
	}
	return psResCont
}

func psNewlineInfix(indent uint32, symType int, st *psState) psResult {
	if st.indents.indentLessEq(indent) && (psExpressionOp(symType) || st.peek() == '`') {
		return psLayoutEnd(st)
	}
	return psResCont
}

func psWhere(st *psState) psResult {
	if psToken("where", st) {
		if st.sym(psTokWhere) {
			st.markEnd()
			return psResFinish(psTokWhere)
		}
		return psLayoutEnd(st)
	}
	return psResCont
}

func psIn(st *psState) psResult {
	if st.sym(psTokIn) && psToken("in", st) {
		st.markEnd()
		st.indents.pop()
		return psResFinish(psTokIn)
	}
	return psResCont
}

func psElse(st *psState) psResult {
	if psToken("else instance", st) {
		return psResCont
	}
	if psToken("else", st) {
		return psEndOrSemicolon(st)
	}
	return psResCont
}

func psInlineComment(st *psState) psResult {
	for {
		c := st.peek()
		if psIsNewline(c) || c == 0 {
			break
		}
		st.advance()
	}
	st.markEnd()
	return psResFinish(psTokComment)
}

func psMinus(st *psState) psResult {
	if !psSeq("--", st) {
		return psResCont
	}
	for st.peek() == '-' {
		st.advance()
	}
	if psSymbolic(st.peek()) {
		return psResFail
	}
	return psInlineComment(st)
}

func psMultilineComment(st *psState) psResult {
	for {
		switch st.peek() {
		case '-':
			st.advance()
			if st.peek() == '}' {
				st.advance()
				st.markEnd()
				return psResFinish(psTokComment)
			}
		case 0:
			res := psEof(st)
			if res.finished {
				return res
			}
			return psResFail
		default:
			st.advance()
		}
	}
}

func psBrace(st *psState) psResult {
	if st.peek() != '{' {
		return psResFail
	}
	st.advance()
	if st.peek() != '-' {
		return psResFail
	}
	st.advance()
	return psMultilineComment(st)
}

func psComment(st *psState) psResult {
	switch st.peek() {
	case '-':
		res := psMinus(st)
		if res.finished {
			return res
		}
		return psResFail
	case '{':
		res := psBrace(st)
		if res.finished {
			return res
		}
		return psResFail
	}
	return psResCont
}

func psSymopMarked(symType int, st *psState) psResult {
	switch symType {
	case psSInvalid, psStar, psTilde, psSImplicit:
		return psResFail
	case psSAtsign:
		return psResFinish(psTokAtsign)
	case psSEquals:
		return psResFinish(psTokEquals)
	case psSComment:
		return psInlineComment(st)
	default:
		return psResCont
	}
}

func psSymop(symType int, st *psState) psResult {
	if symType == psSBar {
		if st.sym(psTokBar) {
			st.markEnd()
			return psResFinish(psTokBar)
		}
		res := psLayoutEnd(st)
		if res.finished {
			return res
		}
		return psResFail
	}
	st.markEnd()
	res := psSymopMarked(symType, st)
	if res.finished {
		return res
	}
	return psResFail
}

func psCloseLayoutInList(st *psState) psResult {
	switch st.peek() {
	case ']':
		if st.sym(psTokEnd) {
			st.indents.pop()
			return psResFinish(psTokEnd)
		}
	case ',':
		st.advance()
		if st.sym(psTokComma) {
			st.markEnd()
			return psResFinish(psTokComma)
		}
		res := psLayoutEnd(st)
		if res.finished {
			return res
		}
		return psResFail
	}
	return psResCont
}

func psInlineTokens(st *psState) psResult {
	c := st.peek()
	isSymbolic := false

	switch c {
	case 'w':
		res := psWhere(st)
		if res.finished {
			return res
		}
		return psResFail
	case 'i':
		res := psIn(st)
		if res.finished {
			return res
		}
		return psResFail
	case 'e':
		res := psElse(st)
		if res.finished {
			return res
		}
		return psResFail
	case ')':
		res := psLayoutEnd(st)
		if res.finished {
			return res
		}
		return psResFail
	case '!', '#', '$', '%', '&', '*', '+', '.', '/', '<', '>',
		'?', '^', ':', '=', '-', '~', '@', '\\':
		isSymbolic = true
		// Fall through to comment check for '{' case below
		res := psComment(st)
		if res.finished {
			return res
		}
	case '{':
		res := psComment(st)
		if res.finished {
			return res
		}
	case '|':
		isSymbolic = true
	}

	if isSymbolic || psUnicodeSymbol(c) {
		s := psReadSymop(st)
		return psSymop(s, st)
	}
	return psCloseLayoutInList(st)
}

func psLayoutStart(col uint32, st *psState) psResult {
	if st.sym(psTokStart) {
		switch st.peek() {
		case '-':
			res := psMinus(st)
			if res.finished {
				return res
			}
		}
		st.indents.push(uint16(col))
		return psResFinish(psTokStart)
	}
	return psResCont
}

func psPostEndSemicolon(col uint32, st *psState) psResult {
	if st.sym(psTokSemicolon) && st.indents.indentLessEq(col) {
		return psResFinish(psTokSemicolon)
	}
	return psResCont
}

func psRepeatEnd(col uint32, st *psState) psResult {
	if st.sym(psTokEnd) && st.indents.smallerIndent(col) {
		return psLayoutEnd(st)
	}
	return psResCont
}

func psNewlineIndent(indent uint32, st *psState) psResult {
	res := psDedent(indent, st)
	if res.finished {
		return res
	}
	res = psCloseLayoutInList(st)
	if res.finished {
		return res
	}
	return psNewlineSemicolon(indent, st)
}

func psNewlineToken(indent uint32, st *psState) psResult {
	c := st.peek()
	isSymbolic := false
	switch c {
	case '!', '#', '$', '%', '&', '*', '+', '.', '/', '<', '>',
		'?', '^', ':', '=', '-', '~', '@', '\\', '|', '`':
		isSymbolic = true
	}
	if isSymbolic || psUnicodeSymbol(c) {
		s := psReadSymop(st)
		res := psNewlineInfix(indent, s, st)
		if res.finished {
			return res
		}
		return psResFail
	}
	res := psNewlineWhere(indent, st)
	if res.finished {
		return res
	}
	if st.peek() == 'i' {
		return psIn(st)
	}
	return psResCont
}

func psNewline(indent uint32, st *psState) psResult {
	res := psEof(st)
	if res.finished {
		return res
	}
	res = psInitialize(indent, st)
	if res.finished {
		return res
	}
	res = psComment(st)
	if res.finished {
		return res
	}
	res = psNewlineToken(indent, st)
	if res.finished {
		return res
	}
	return psNewlineIndent(indent, st)
}

func psImmediate(col uint32, st *psState) psResult {
	res := psLayoutStart(col, st)
	if res.finished {
		return res
	}
	res = psPostEndSemicolon(col, st)
	if res.finished {
		return res
	}
	res = psRepeatEnd(col, st)
	if res.finished {
		return res
	}
	return psInlineTokens(st)
}

func psInit(st *psState) psResult {
	res := psEof(st)
	if res.finished {
		return res
	}
	if st.allSyms() {
		return psResFail
	}
	res = psInitializeInit(st)
	if res.finished {
		return res
	}
	res = psDot(st)
	if res.finished {
		return res
	}
	return psResCont
}

func psScanMain(st *psState) psResult {
	psSkipSpace(st)
	res := psEof(st)
	if res.finished {
		return res
	}
	st.markEnd()
	if psIsNewline(st.peek()) {
		st.skip()
		indent := psCountIndent(st)
		return psNewline(indent, st)
	}
	col := st.column()
	return psImmediate(col, st)
}

func psScanAll(st *psState) psResult {
	res := psInit(st)
	if res.finished {
		return res
	}
	return psScanMain(st)
}

// PurescriptExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-purescript.
//
// This is a Go port of the C external scanner from tree-sitter-purescript
// (https://github.com/purerl/tree-sitter-purescript). The scanner handles
// layout-sensitive indentation for PureScript's offside rule, tracking an
// indent stack and emitting layout tokens (semicolons, starts, ends), as
// well as qualified module dots, comments, and various keyword tokens.
type PurescriptExternalScanner struct{}

func (PurescriptExternalScanner) Create() any {
	return &psScannerState{}
}

func (PurescriptExternalScanner) Destroy(payload any) {}

func (PurescriptExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*psScannerState)
	// Each indent is a uint16, so 2 bytes per entry.
	needed := len(s.indents) * 2
	if needed > len(buf) {
		return 0
	}
	for i, v := range s.indents {
		buf[i*2] = byte(v)
		buf[i*2+1] = byte(v >> 8)
	}
	return needed
}

func (PurescriptExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*psScannerState)
	s.indents = s.indents[:0]
	els := len(buf) / 2
	if els == 0 {
		return
	}
	if cap(s.indents) < els {
		s.indents = make([]uint16, 0, els)
	}
	for i := 0; i < els; i++ {
		v := uint16(buf[i*2]) | uint16(buf[i*2+1])<<8
		s.indents = append(s.indents, v)
	}
}

func (PurescriptExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*psScannerState)

	st := &psState{
		lexer:   lexer,
		symbols: validSymbols,
		indents: s,
	}

	result := psScanAll(st)
	if result.finished && result.sym != psTokFail {
		if result.sym >= 0 && result.sym < len(psSymMap) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(psSymMap[result.sym])
		}
		return true
	}
	return false
}
