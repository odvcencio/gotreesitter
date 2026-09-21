//go:build !grammar_subset || grammar_subset_fortran

package grammarruntime

import (
	"encoding/binary"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Fortran grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see ftnDefaultSymTable below.
const (
	ftnTokLineContinuation  = 0  // "&"
	ftnTokIntegerLiteral    = 1  // _integer_literal
	ftnTokFloatLiteral      = 2  // _float_literal
	ftnTokBozLiteral        = 3  // _boz_literal
	ftnTokStringLiteral     = 4  // _string_literal
	ftnTokStringLiteralKind = 5  // _string_literal_kind, displays as "identifier"
	ftnTokEndOfStatement    = 6  // _external_end_of_statement
	ftnTokPreprocUnaryOp    = 7  // _preproc_unary_operator
	ftnTokHollerithConstant = 8  // hollerith_constant
	ftnTokDoLabel           = 9  // _do_label, displays as "statement_label_reference"
	ftnTokDoLabelVirtual    = 10 // do_label_virtual
	ftnTokDoLabelContinue   = 11 // _do_label_continue, displays as "statement_label"
	ftnTokenCount           = 12
)

// ftnDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped fortran.bin assigns to each external, in ftnTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var ftnDefaultSymTable = [ftnTokenCount]gotreesitter.Symbol{
	34,  // "&"
	276, // _integer_literal
	277, // _float_literal
	278, // _boz_literal
	279, // _string_literal
	280, // _string_literal_kind, displays as "identifier"
	281, // _external_end_of_statement
	282, // _preproc_unary_operator
	283, // hollerith_constant
	284, // _do_label, displays as "statement_label_reference"
	285, // do_label_virtual
	286, // _do_label_continue, displays as "statement_label"
}

// ftnExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (ftnTok* order).
var ftnExternalScannerSpec = ExternalScannerSpec{
	Language:       "fortran",
	UpstreamRepo:   "https://github.com/stadelmanma/tree-sitter-fortran",
	UpstreamCommit: "2880b7aab4fb7cc618de1ef3d4c6d93b2396c031",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "3164fd2f33a1e2c0e0cde33a7a974db8b7acc1e25dec29b870c2d9997b8f018d"},
		{Path: "src/scanner.c", SHA256: "ee896acebecc430cbd4b4ac427cc31726ae38a45d1195e13bd547cc5549be268"},
	},
	Externals: []string{
		"&",
		"_integer_literal",
		"_float_literal",
		"_boz_literal",
		"_string_literal",
		"_string_literal_kind",
		"_external_end_of_statement",
		"_preproc_unary_operator",
		"hollerith_constant",
		"_do_label",
		"do_label_virtual",
		"_do_label_continue",
	},
}

func init() {
	RegisterExternalScannerSpec(ftnExternalScannerSpec)
}

// ftnMaxLabelStack is the maximum nesting depth for labeled DO loops.
const ftnMaxLabelStack = 100

// ftnNumberType identifies the kind of number parsed.
type ftnNumberType int

const (
	ftnNumberNone ftnNumberType = iota
	ftnNumberInteger
	ftnNumberFloat
)

// ftnNumberResult holds the result of scanning a number.
type ftnNumberResult struct {
	typ        ftnNumberType
	value      int32
	digitCount int
}

// ftnState holds the scanner state for Fortran.
type ftnState struct {
	inLineContinuation  bool
	depth               int32
	labels              [ftnMaxLabelStack]int32
	counts              [ftnMaxLabelStack]int32
	pendingLabelVirtual int32
	isPendingEosVirtual bool
}

// FortranExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-fortran.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type FortranExternalScanner struct {
	symbols         [ftnTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers fortran's external symbols.
func (FortranExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := FortranExternalScanner{symbols: ftnDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, ftnExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s FortranExternalScanner) symbolTable() *[ftnTokenCount]gotreesitter.Symbol {
	if s.symbols == ([ftnTokenCount]gotreesitter.Symbol{}) {
		return &ftnDefaultSymTable
	}
	return &s.symbols
}

func (FortranExternalScanner) Create() any {
	return &ftnState{}
}

func (FortranExternalScanner) Destroy(payload any) {}

func (FortranExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*ftnState)
	if s.depth > ftnMaxLabelStack {
		return 0
	}

	// Layout: 1 byte inLineContinuation
	//         4 bytes depth
	//         depth*4 bytes labels
	//         depth*4 bytes counts
	//         4 bytes pendingLabelVirtual
	//         1 byte isPendingEosVirtual
	needed := 1 + 4 + int(s.depth)*4 + int(s.depth)*4 + 4 + 1
	if needed > len(buf) {
		return 0
	}

	size := 0

	if s.inLineContinuation {
		buf[size] = 1
	} else {
		buf[size] = 0
	}
	size++

	binary.LittleEndian.PutUint32(buf[size:], uint32(s.depth))
	size += 4

	for i := int32(0); i < s.depth; i++ {
		binary.LittleEndian.PutUint32(buf[size:], uint32(s.labels[i]))
		size += 4
	}

	for i := int32(0); i < s.depth; i++ {
		binary.LittleEndian.PutUint32(buf[size:], uint32(s.counts[i]))
		size += 4
	}

	binary.LittleEndian.PutUint32(buf[size:], uint32(s.pendingLabelVirtual))
	size += 4

	if s.isPendingEosVirtual {
		buf[size] = 1
	} else {
		buf[size] = 0
	}
	size++

	return size
}

func (FortranExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*ftnState)
	s.inLineContinuation = false
	s.depth = 0
	s.pendingLabelVirtual = 0
	s.isPendingEosVirtual = false

	if len(buf) == 0 {
		return
	}

	size := 0

	s.inLineContinuation = buf[size] != 0
	size++

	s.depth = int32(binary.LittleEndian.Uint32(buf[size:]))
	size += 4

	if s.depth > ftnMaxLabelStack {
		s.depth = 0
		s.pendingLabelVirtual = 0
		s.isPendingEosVirtual = false
		return
	}

	for i := int32(0); i < s.depth; i++ {
		s.labels[i] = int32(binary.LittleEndian.Uint32(buf[size:]))
		size += 4
	}

	for i := int32(0); i < s.depth; i++ {
		s.counts[i] = int32(binary.LittleEndian.Uint32(buf[size:]))
		size += 4
	}

	s.pendingLabelVirtual = int32(binary.LittleEndian.Uint32(buf[size:]))
	size += 4

	s.isPendingEosVirtual = buf[size] != 0
}

func (sc FortranExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*ftnState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [ftnTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < ftnTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}

	return ftnScan(s, lexer, validSymbols, sc.symbolTable())
}

// ---------------------------------------------------------------------------
// Helper utilities
// ---------------------------------------------------------------------------

func ftnIsValid(validSymbols []bool, idx int) bool {
	return idx < len(validSymbols) && validSymbols[idx]
}

func ftnAdvance(lexer *gotreesitter.ExternalLexer) {
	lexer.Advance(false)
}

func ftnSkip(lexer *gotreesitter.ExternalLexer) {
	lexer.Advance(true)
}

func ftnIsIdentifierChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_'
}

func ftnIsBozSentinel(ch rune) bool {
	switch ch {
	case 'B', 'b', 'O', 'o', 'Z', 'z':
		return true
	}
	return false
}

func ftnIsExpSentinel(ch rune) bool {
	switch ch {
	case 'D', 'd', 'E', 'e', 'Q', 'q':
		return true
	}
	return false
}

func ftnIsBlank(ch rune) bool {
	// iswblank: horizontal whitespace (space and tab), but not newlines
	return ch == ' ' || ch == '\t'
}

func ftnIsHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// ---------------------------------------------------------------------------
// Skip literal continuation sequence: handles '&' ... '&' across lines
// inside a literal token.
// ---------------------------------------------------------------------------

func ftnSkipLiteralContinuationSequence(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '&' {
		return true
	}

	ftnAdvance(lexer)
	for unicode.IsSpace(lexer.Lookahead()) {
		ftnAdvance(lexer)
	}
	// second '&' technically required to continue the literal
	if lexer.Lookahead() == '&' {
		ftnAdvance(lexer)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Scan integer digits, optionally computing value and digit count.
// ---------------------------------------------------------------------------

func ftnScanInt(lexer *gotreesitter.ExternalLexer, value *int32, count *int) bool {
	if !unicode.IsDigit(lexer.Lookahead()) {
		return false
	}

	if value != nil && count != nil {
		*value = 0
		*count = 0
	}

	// outer loop for handling line continuations
	for {
		// consume digits
		for unicode.IsDigit(lexer.Lookahead()) {
			if value != nil && count != nil && *count < 7 {
				*value = *value*10 + int32(lexer.Lookahead()-'0')
				*count++
			}
			ftnAdvance(lexer)
		}
		lexer.MarkEnd()

		if lexer.Lookahead() == '&' {
			if ftnSkipLiteralContinuationSequence(lexer) {
				continue
			}
		}

		break
	}

	return true
}

// ---------------------------------------------------------------------------
// Scan number: integer or float (1XXX, 1.0XXX, 0.1XXX, .1X, etc.)
// ---------------------------------------------------------------------------

func ftnScanNumber(lexer *gotreesitter.ExternalLexer) ftnNumberResult {
	result := ftnNumberResult{typ: ftnNumberInteger}

	// collect initial digits and compute value
	digits := ftnScanInt(lexer, &result.value, &result.digitCount)

	if lexer.Lookahead() == '.' {
		ftnAdvance(lexer)
		// exclude decimal if followed by any letter other than d/D and e/E
		// if no leading digits are present and a non-digit follows
		// the decimal it's a nonmatch.
		if digits && !unicode.IsLetter(lexer.Lookahead()) && !unicode.IsDigit(lexer.Lookahead()) {
			lexer.MarkEnd() // add decimal to token
		}
		result.typ = ftnNumberFloat
	}

	// if next char isn't number return since we handle exp
	// notation and precision identifiers separately.
	moreDigits := ftnScanInt(lexer, nil, nil)
	digits = moreDigits || digits

	if digits {
		// process exp notation
		if ftnIsExpSentinel(lexer.Lookahead()) {
			ftnAdvance(lexer)
			if lexer.Lookahead() == '+' || lexer.Lookahead() == '-' {
				ftnAdvance(lexer)
				lexer.MarkEnd()
			}
			if !ftnScanInt(lexer, nil, nil) {
				result.typ = ftnNumberInteger
				return result // valid number token with junk after it
			}
			result.typ = ftnNumberFloat
		}
	}

	if !digits {
		result.typ = ftnNumberNone
	}
	return result
}

// ---------------------------------------------------------------------------
// Scan BOZ literal (binary/octal/hex)
// ---------------------------------------------------------------------------

func ftnScanBoz(lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	bozPrefix := false
	var quote rune

	if ftnIsBozSentinel(lexer.Lookahead()) {
		ftnAdvance(lexer)
		bozPrefix = true
	}
	if lexer.Lookahead() == '\'' || lexer.Lookahead() == '"' {
		quote = lexer.Lookahead()
		ftnAdvance(lexer)
		if !ftnIsHexDigit(lexer.Lookahead()) {
			return false
		}
		for ftnIsHexDigit(lexer.Lookahead()) {
			ftnAdvance(lexer) // store all hex digits
		}
		if lexer.Lookahead() != quote {
			return false
		}
		ftnAdvance(lexer) // store enclosing quote
		if !bozPrefix && !ftnIsBozSentinel(lexer.Lookahead()) {
			return false // no boz suffix or prefix provided
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[ftnTokBozLiteral])
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Scan Hollerith constant (nH<text>)
// ---------------------------------------------------------------------------

func ftnScanHollerithConstant(lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	// Read integer prefix 'n'
	var length uint32
	for unicode.IsDigit(lexer.Lookahead()) {
		newLength := length*10 + uint32(lexer.Lookahead()-'0')
		// overflow check
		if newLength < length {
			return false
		}
		length = newLength
		ftnAdvance(lexer)

		if !ftnSkipLiteralContinuationSequence(lexer) {
			return false
		}
	}

	// 0 is invalid 'n' in Hollerith constants
	if length == 0 {
		return false
	}

	// Expect 'H' or 'h'
	if lexer.Lookahead() != 'H' && lexer.Lookahead() != 'h' {
		return false
	}
	ftnAdvance(lexer)

	// Read exactly 'n' characters
	for i := uint32(0); i < length; i++ {
		if lexer.Lookahead() == 0 {
			return false
		}
		if !ftnSkipLiteralContinuationSequence(lexer) {
			return false
		}
		ftnAdvance(lexer)
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[ftnTokHollerithConstant])
	return true
}

// ---------------------------------------------------------------------------
// Scan end-of-statement
// ---------------------------------------------------------------------------

func ftnScanEndOfStatement(s *ftnState, lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	// EOF always ends the statement
	if lexer.Lookahead() == 0 {
		ftnSkip(lexer)
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[ftnTokEndOfStatement])
		return true
	}

	// If we're in a line continuation, then don't end the statement
	if s.inLineContinuation {
		return false
	}

	// Consume end of line characters: '\n', '\r\n', '\r'
	// Handle comments here too, but don't consume them
	if lexer.Lookahead() == '\r' {
		ftnSkip(lexer)
		if lexer.Lookahead() == '\n' {
			ftnSkip(lexer)
		}
	} else {
		if lexer.Lookahead() == '\n' {
			ftnSkip(lexer)
		} else if lexer.Lookahead() != '!' {
			// Not a newline and not a comment, so not an end-of-statement
			return false
		}
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[ftnTokEndOfStatement])
	return true
}

// ---------------------------------------------------------------------------
// Line continuation start/end
// ---------------------------------------------------------------------------

func ftnScanStartLineContinuation(s *ftnState, lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	s.inLineContinuation = (lexer.Lookahead() == '&')
	if !s.inLineContinuation {
		return false
	}
	// Consume the '&'
	ftnAdvance(lexer)
	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[ftnTokLineContinuation])
	return true
}

func ftnScanEndLineContinuation(s *ftnState, lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	if !s.inLineContinuation {
		return false
	}
	// Everything except comments ends a line continuation
	if lexer.Lookahead() == '!' {
		return false
	}

	s.inLineContinuation = false

	// Consume any leading line continuation markers
	if lexer.Lookahead() == '&' {
		ftnAdvance(lexer)
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[ftnTokLineContinuation])
	return true
}

// ---------------------------------------------------------------------------
// String literal kind (identifier prefix for typed strings, e.g. c_"hello")
// ---------------------------------------------------------------------------

func ftnScanStringLiteralKind(lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	if !unicode.IsLetter(lexer.Lookahead()) {
		return false
	}

	var currentChar rune

	for ftnIsIdentifierChar(lexer.Lookahead()) && lexer.Lookahead() != 0 {
		currentChar = lexer.Lookahead()
		// Don't capture the trailing underscore as part of the kind identifier
		if lexer.Lookahead() == '_' {
			lexer.MarkEnd()
		}
		ftnAdvance(lexer)
	}

	if currentChar != '_' || (lexer.Lookahead() != '"' && lexer.Lookahead() != '\'') {
		return false
	}

	lexer.SetResultSymbol(syms[ftnTokStringLiteralKind])
	return true
}

// ---------------------------------------------------------------------------
// String literal
// ---------------------------------------------------------------------------

func ftnScanStringLiteral(lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	openingQuote := lexer.Lookahead()

	if openingQuote != '"' && openingQuote != '\'' {
		return false
	}

	ftnAdvance(lexer)

	for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
		// Handle line continuations inside string literals
		if lexer.Lookahead() == '&' {
			ftnAdvance(lexer)
			// Consume blanks up to the end of the line or non-blank
			for ftnIsBlank(lexer.Lookahead()) {
				ftnAdvance(lexer)
			}
			// If we hit the end of the line, consume all whitespace including new lines
			if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
				for unicode.IsSpace(lexer.Lookahead()) {
					ftnAdvance(lexer)
				}
			}
			continue
		}

		// If we hit the same kind of quote that opened this literal,
		// check to see if there's two in a row (escaped quote)
		if lexer.Lookahead() == openingQuote {
			ftnAdvance(lexer)
			// It was just one quote, so we've successfully reached the end of the literal.
			// Also check that an escaped quote isn't split in half by a line continuation.
			lexer.MarkEnd()
			ftnSkipLiteralContinuationSequence(lexer)
			if lexer.Lookahead() != openingQuote {
				lexer.SetResultSymbol(syms[ftnTokStringLiteral])
				return true
			}
		}
		ftnAdvance(lexer)
	}

	// We hit the end of the line without an '&', so this is an unclosed string literal (an error)
	return false
}

// ---------------------------------------------------------------------------
// Preprocessor unary operator
// ---------------------------------------------------------------------------

func ftnScanPreprocUnaryOperator(lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	ch := lexer.Lookahead()
	if ch == '!' || ch == '~' || ch == '-' || ch == '+' {
		ftnAdvance(lexer)
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[ftnTokPreprocUnaryOp])
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Labeled DO loop tracking
// ---------------------------------------------------------------------------

func ftnTrackLabeledDo(s *ftnState, label int32) {
	// check if label already exists at top of stack
	if s.depth > 0 {
		i := s.depth - 1
		if s.labels[i] == label {
			s.counts[i]++
			return
		}
	}

	// not at top of stack, assume new label, add it to stack
	if s.depth < ftnMaxLabelStack {
		s.labels[s.depth] = label
		s.counts[s.depth] = 1
		s.depth++
	}
}

// ftnScanDoLabelEos checks whether an end-of-statement token for virtual do labels
// is pending, emits END_OF_STATEMENT and updates internal state accordingly.
func ftnScanDoLabelEos(s *ftnState, lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	if s.isPendingEosVirtual {
		s.isPendingEosVirtual = false
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[ftnTokEndOfStatement])
		return true
	}
	return false
}

// ftnScanDoLabelPending checks whether do labels are pending, emits
// DO_LABEL_VIRTUAL or DO_LABEL_CONTINUE and updates internal state accordingly.
func ftnScanDoLabelPending(s *ftnState, lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	if s.pendingLabelVirtual > 0 {
		lexer.MarkEnd()
		if s.pendingLabelVirtual > 1 {
			s.pendingLabelVirtual--
			// schedule an eos for the next token to finish the virtual statement
			s.isPendingEosVirtual = true
			lexer.SetResultSymbol(syms[ftnTokDoLabelVirtual])
		} else {
			// emit last termination symbol which is do_label_continue
			s.pendingLabelVirtual = 0
			lexer.SetResultSymbol(syms[ftnTokDoLabelContinue])
		}
		return true
	}
	return false
}

// ftnScanDoLabel is invoked after the parser has found a "do" and the scan has
// consumed a proper integer value as a label.
func ftnScanDoLabel(s *ftnState, lexer *gotreesitter.ExternalLexer, label int32, syms *[ftnTokenCount]gotreesitter.Symbol) {
	ftnTrackLabeledDo(s, label)
	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[ftnTokDoLabel])
}

func ftnScanDoLabelContinue(s *ftnState, lexer *gotreesitter.ExternalLexer, label int32, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	// determine whether this label belongs to the last labeled do,
	// if it does, remove it from stack and determine how many loops it closes
	var loopsToClose int32
	if s.depth > 0 {
		i := s.depth - 1
		if s.labels[i] == label {
			loopsToClose = s.counts[i]
			// remove from stack
			s.depth--
		}
	}

	// counts[i] is always > 0 if the label is on the stack,
	// hence loopsToClose == 0 means depth=0 or label is not at top of stack
	if loopsToClose == 0 {
		return false
	}

	s.pendingLabelVirtual = loopsToClose
	s.isPendingEosVirtual = false
	ftnScanDoLabelPending(s, lexer, syms)
	return true
}

// ---------------------------------------------------------------------------
// Scan label, number, or BOZ
// ---------------------------------------------------------------------------

func ftnScanLabelNumberBoz(s *ftnState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	result := ftnScanNumber(lexer)

	// check for a do-label: at most 5 digits and DO_LABEL or DO_LABEL_CONTINUE valid
	if result.typ == ftnNumberInteger && result.digitCount < 6 {
		if ftnIsValid(validSymbols, ftnTokDoLabel) {
			ftnScanDoLabel(s, lexer, result.value, syms)
			return true
		}
		if ftnIsValid(validSymbols, ftnTokDoLabelContinue) &&
			ftnScanDoLabelContinue(s, lexer, result.value, syms) {
			return true
		}
	}

	// not a label
	if result.typ == ftnNumberInteger {
		lexer.SetResultSymbol(syms[ftnTokIntegerLiteral])
		return true
	} else if result.typ == ftnNumberFloat {
		lexer.SetResultSymbol(syms[ftnTokFloatLiteral])
		return true
	}

	if ftnScanBoz(lexer, syms) {
		return true
	}

	return false
}

// ---------------------------------------------------------------------------
// Main scan entry point
// ---------------------------------------------------------------------------

func ftnScan(s *ftnState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	// handle pending virtual labels and eos first
	if ftnIsValid(validSymbols, ftnTokEndOfStatement) {
		if ftnScanDoLabelEos(s, lexer, syms) {
			return true
		}
	}

	if ftnIsValid(validSymbols, ftnTokDoLabelContinue) || ftnIsValid(validSymbols, ftnTokDoLabelVirtual) {
		if ftnScanDoLabelPending(s, lexer, syms) {
			return true
		}
	}

	// Consume any leading whitespace except newlines
	for ftnIsBlank(lexer.Lookahead()) {
		ftnSkip(lexer)
	}

	// Close the current statement if we can
	if ftnIsValid(validSymbols, ftnTokEndOfStatement) {
		if ftnScanEndOfStatement(s, lexer, syms) {
			return true
		}
	}

	// We're now either in a line continuation or between statements,
	// so we should eat all whitespace including newlines until we come
	// to something more interesting.
	for unicode.IsSpace(lexer.Lookahead()) {
		ftnSkip(lexer)
	}

	if ftnScanEndLineContinuation(s, lexer, syms) {
		return true
	}

	if ftnIsValid(validSymbols, ftnTokStringLiteral) {
		if ftnScanStringLiteral(lexer, syms) {
			return true
		}
	}

	if ftnIsValid(validSymbols, ftnTokHollerithConstant) {
		if ftnScanHollerithConstant(lexer, syms) {
			return true
		}
	}

	if ftnIsValid(validSymbols, ftnTokIntegerLiteral) ||
		ftnIsValid(validSymbols, ftnTokFloatLiteral) ||
		ftnIsValid(validSymbols, ftnTokBozLiteral) ||
		ftnIsValid(validSymbols, ftnTokDoLabel) ||
		ftnIsValid(validSymbols, ftnTokDoLabelContinue) {
		if ftnScanLabelNumberBoz(s, lexer, validSymbols, syms) {
			return true
		}
	}

	if ftnIsValid(validSymbols, ftnTokPreprocUnaryOp) {
		if ftnScanPreprocUnaryOperator(lexer, syms) {
			return true
		}
	}

	if ftnScanStartLineContinuation(s, lexer, syms) {
		return true
	}

	if ftnIsValid(validSymbols, ftnTokStringLiteralKind) {
		// This may need a lot of lookahead, so should (probably) always
		// be the last token to look for
		if ftnScanStringLiteralKind(lexer, syms) {
			return true
		}
	}

	return false
}
