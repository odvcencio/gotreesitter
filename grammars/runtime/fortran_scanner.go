//go:build !grammar_subset || grammar_subset_fortran

package grammarruntime

import (
	"encoding/binary"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
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
	// Checked against the pinned C scanner (cgo_harness scandiff, 200,000 calls) on the
	// safe-state runs: the pinned C scanner itself fails on forged saved states; no mode.
	scanmode.RegisterScanner(FortranExternalScanner{}, (*ftnState)(nil), false, func(v any) (any, []byte, bool) {
		s := v.(FortranExternalScanner)
		s.externalToToken = append([]int(nil), s.externalToToken...)
		return s, scannerKey(s.symbols[:], s.externalToToken), true
	}, func(left, right any) bool {
		a, b := left.(FortranExternalScanner), right.(FortranExternalScanner)
		return scannerConfigEqual(a.symbols[:], b.symbols[:], a.externalToToken, b.externalToToken)
	})
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

// Deserialize is src/scanner.c:700-739. The C code never looks at the length except for the
// empty state: it reads the whole layout from the buffer, so a state that is too short is read
// as if it continued with zero bytes (the scanner's saved-state buffer is zero filled), and
// bytes past the layout are ignored. The old data in labels and counts stays where the state
// does not reach it, as in C.
func (FortranExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*ftnState)

	// :705-711
	if len(buf) == 0 {
		s.inLineContinuation = false
		s.depth = 0
		s.pendingLabelVirtual = 0
		s.isPendingEosVirtual = false
		return
	}

	// u32 reads four little-endian bytes at off, zero past the end of the saved state.
	u32 := func(off int) uint32 {
		var w [4]byte
		for i := range w {
			if off+i < len(buf) {
				w[i] = buf[off+i]
			}
		}
		return binary.LittleEndian.Uint32(w[:])
	}

	size := 0

	s.inLineContinuation = buf[size] != 0
	size++

	s.depth = int32(u32(size))
	size += 4

	// :721-726. A depth below 0 passes this test in C and then crashes in memcpy (the size
	// is a huge unsigned number); no result is defined for it, so it is treated like a
	// depth above the limit.
	if s.depth > ftnMaxLabelStack || s.depth < 0 {
		s.depth = 0
		s.pendingLabelVirtual = 0
		s.isPendingEosVirtual = false
		return
	}

	for i := int32(0); i < s.depth; i++ {
		s.labels[i] = int32(u32(size))
		size += 4
	}

	for i := int32(0); i < s.depth; i++ {
		s.counts[i] = int32(u32(size))
		size += 4
	}

	s.pendingLabelVirtual = int32(u32(size))
	size += 4

	if size < len(buf) {
		s.isPendingEosVirtual = buf[size] != 0
	} else {
		s.isPendingEosVirtual = false
	}
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

// ftnLA is lexer->lookahead as C sees it: -1 for a byte that is not valid UTF-8.
func ftnLA(lexer *gotreesitter.ExternalLexer) int32 { return scanmode.CLookahead(lexer) }

// ftnChar is the conversion of lexer->lookahead to a plain char, which C does where a
// function takes a char (src/scanner.c:66, :68, :82 and the char variables on :364, :383,
// :449): the low eight bits, as a signed number.
func ftnChar(la int32) int8 { return int8(la) }

// ftnIsIdentifierChar is is_identifier_char (src/scanner.c:66): the argument is a char, and
// iswalnum of a negative char is false in the C locale.
func ftnIsIdentifierChar(la int32) bool {
	c := ftnChar(la)
	return c >= 0 && (cIsAlnum(rune(c)) || c == '_')
}

// ftnIsBozSentinel is is_boz_sentinel (:68-80); its argument is a char.
func ftnIsBozSentinel(la int32) bool {
	switch ftnChar(la) {
	case 'B', 'b', 'O', 'o', 'Z', 'z':
		return true
	}
	return false
}

// ftnIsExpSentinel is is_exp_sentinel (:82-94); its argument is a char.
func ftnIsExpSentinel(la int32) bool {
	switch ftnChar(la) {
	case 'D', 'd', 'E', 'e', 'Q', 'q':
		return true
	}
	return false
}

// ftnIsBlank is iswblank in the C locale: space and tab.
func ftnIsBlank(la int32) bool {
	return la == ' ' || la == '\t'
}

// ftnIsHexDigit is iswxdigit in the C locale.
func ftnIsHexDigit(la int32) bool {
	return (la >= '0' && la <= '9') || (la >= 'a' && la <= 'f') || (la >= 'A' && la <= 'F')
}

// ---------------------------------------------------------------------------
// Skip literal continuation sequence: handles '&' ... '&' across lines
// inside a literal token. (src/scanner.c:97-112)
// ---------------------------------------------------------------------------

func ftnSkipLiteralContinuationSequence(lexer *gotreesitter.ExternalLexer) bool {
	if ftnLA(lexer) != '&' {
		return true
	}

	ftnAdvance(lexer)
	for cIsSpace(ftnLA(lexer)) {
		ftnAdvance(lexer)
	}
	// second '&' technically required to continue the literal
	if ftnLA(lexer) == '&' {
		ftnAdvance(lexer)
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Scan integer digits, optionally computing value and digit count.
// (src/scanner.c:115-150; iswdigit is ASCII only in the C locale)
// ---------------------------------------------------------------------------

func ftnScanInt(lexer *gotreesitter.ExternalLexer, value *int32, count *int) bool {
	if !cIsDigit(ftnLA(lexer)) {
		return false
	}

	if value != nil && count != nil {
		*value = 0
		*count = 0
	}

	// outer loop for handling line continuations
	for {
		// consume digits
		for cIsDigit(ftnLA(lexer)) {
			if value != nil && count != nil && *count < 7 {
				*value = *value*10 + int32(ftnLA(lexer)-'0')
				*count++
			}
			ftnAdvance(lexer)
		}
		lexer.MarkEnd()

		if ftnLA(lexer) == '&' {
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
// (src/scanner.c:153-200)
// ---------------------------------------------------------------------------

func ftnScanNumber(lexer *gotreesitter.ExternalLexer) ftnNumberResult {
	result := ftnNumberResult{typ: ftnNumberInteger}

	// collect initial digits and compute value
	digits := ftnScanInt(lexer, &result.value, &result.digitCount)

	if ftnLA(lexer) == '.' {
		ftnAdvance(lexer)
		// :168 iswalnum is ASCII only in the C locale
		if digits && !cIsAlnum(ftnLA(lexer)) {
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
		if ftnIsExpSentinel(ftnLA(lexer)) {
			ftnAdvance(lexer)
			if ftnLA(lexer) == '+' || ftnLA(lexer) == '-' {
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
// Scan BOZ literal (binary/octal/hex) (src/scanner.c:202-230)
// ---------------------------------------------------------------------------

func ftnScanBoz(lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	bozPrefix := false
	var quote int32

	if ftnIsBozSentinel(ftnLA(lexer)) {
		ftnAdvance(lexer)
		bozPrefix = true
	}
	if ftnLA(lexer) == '\'' || ftnLA(lexer) == '"' {
		quote = ftnLA(lexer)
		ftnAdvance(lexer)
		if !ftnIsHexDigit(ftnLA(lexer)) {
			return false
		}
		for ftnIsHexDigit(ftnLA(lexer)) {
			ftnAdvance(lexer) // store all hex digits
		}
		if ftnLA(lexer) != quote {
			return false
		}
		ftnAdvance(lexer) // store enclosing quote
		if !bozPrefix && !ftnIsBozSentinel(ftnLA(lexer)) {
			return false // no boz suffix or prefix provided
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[ftnTokBozLiteral])
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Scan Hollerith constant (nH<text>) (src/scanner.c:233-276)
// ---------------------------------------------------------------------------

func ftnScanHollerithConstant(lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	// Read integer prefix 'n'
	var length uint32
	for cIsDigit(ftnLA(lexer)) {
		newLength := length*10 + uint32(ftnLA(lexer)-'0')
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
	if ftnLA(lexer) != 'H' && ftnLA(lexer) != 'h' {
		return false
	}
	ftnAdvance(lexer)

	// Read exactly 'n' characters
	for i := uint32(0); i < length; i++ {
		// :265 a NUL byte or the end of input
		if ftnLA(lexer) == 0 || scanmode.AtEOF(lexer) {
			return false
		}
		if !ftnSkipLiteralContinuationSequence(lexer) {
			return false
		}
		ftnAdvance(lexer)
	}
	lexer.SetResultSymbol(syms[ftnTokHollerithConstant])
	lexer.MarkEnd()
	return true
}

// ---------------------------------------------------------------------------
// Scan end-of-statement (src/scanner.c:278-321). C never calls mark_end here
// or in the other tokens that only move over a few characters, so the token
// ends where the cursor is.
// ---------------------------------------------------------------------------

func ftnScanEndOfStatement(s *ftnState, lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	// EOF always ends the statement (:290; a NUL byte is not the end)
	if scanmode.AtEOF(lexer) {
		ftnSkip(lexer)
		lexer.SetResultSymbol(syms[ftnTokEndOfStatement])
		return true
	}

	// If we're in a line continuation, then don't end the statement
	if s.inLineContinuation {
		return false
	}

	// Consume end of line characters: '\n', '\r\n', '\r'
	// Handle comments here too, but don't consume them
	if ftnLA(lexer) == '\r' {
		ftnSkip(lexer)
		if ftnLA(lexer) == '\n' {
			ftnSkip(lexer)
		}
	} else {
		if ftnLA(lexer) == '\n' {
			ftnSkip(lexer)
		} else if ftnLA(lexer) != '!' {
			// Not a newline and not a comment, so not an end-of-statement
			return false
		}
	}

	lexer.SetResultSymbol(syms[ftnTokEndOfStatement])
	return true
}

// ---------------------------------------------------------------------------
// Line continuation start/end (src/scanner.c:323-352)
// ---------------------------------------------------------------------------

func ftnScanStartLineContinuation(s *ftnState, lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	s.inLineContinuation = (ftnLA(lexer) == '&')
	if !s.inLineContinuation {
		return false
	}
	// Consume the '&'
	ftnAdvance(lexer)
	lexer.SetResultSymbol(syms[ftnTokLineContinuation])
	return true
}

func ftnScanEndLineContinuation(s *ftnState, lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	if !s.inLineContinuation {
		return false
	}
	// Everything except comments ends a line continuation
	if ftnLA(lexer) == '!' {
		return false
	}

	s.inLineContinuation = false

	// Consume any leading line continuation markers
	if ftnLA(lexer) == '&' {
		ftnAdvance(lexer)
	}
	lexer.SetResultSymbol(syms[ftnTokLineContinuation])
	return true
}

// ---------------------------------------------------------------------------
// String literal kind (identifier prefix for typed strings, e.g. c_"hello")
// (src/scanner.c:354-380)
// ---------------------------------------------------------------------------

func ftnScanStringLiteralKind(lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	if !cIsAlpha(ftnLA(lexer)) {
		return false
	}

	lexer.SetResultSymbol(syms[ftnTokStringLiteralKind])

	// current_char is a char: the low byte of the lookahead
	var currentChar int8

	for ftnIsIdentifierChar(ftnLA(lexer)) && !scanmode.AtEOF(lexer) {
		currentChar = ftnChar(ftnLA(lexer))
		// Don't capture the trailing underscore as part of the kind identifier
		if ftnLA(lexer) == '_' {
			lexer.MarkEnd()
		}
		ftnAdvance(lexer)
	}

	if currentChar != '_' || (ftnLA(lexer) != '"' && ftnLA(lexer) != '\'') {
		return false
	}

	return true
}

// ---------------------------------------------------------------------------
// String literal (src/scanner.c:382-445)
// ---------------------------------------------------------------------------

func ftnScanStringLiteral(lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	// :383 opening_quote is a char: only the low byte of the lookahead is tested
	openingQuote := ftnChar(ftnLA(lexer))

	if openingQuote != '"' && openingQuote != '\'' {
		return false
	}

	ftnAdvance(lexer)
	lexer.SetResultSymbol(syms[ftnTokStringLiteral])

	// :392 a NUL byte is content; only the end of input stops the loop with '\n'
	for ftnLA(lexer) != '\n' && !scanmode.AtEOF(lexer) {
		// Handle line continuations inside string literals
		if ftnLA(lexer) == '&' {
			ftnAdvance(lexer)
			// Consume blanks up to the end of the line or non-blank
			for ftnIsBlank(ftnLA(lexer)) {
				ftnAdvance(lexer)
			}
			// If we hit the end of the line, consume all whitespace including new lines
			if ftnLA(lexer) == '\n' || ftnLA(lexer) == '\r' {
				for cIsSpace(ftnLA(lexer)) {
					ftnAdvance(lexer)
				}
			}
			continue
		}

		// If we hit the same kind of quote that opened this literal,
		// check to see if there's two in a row (escaped quote)
		if ftnLA(lexer) == int32(openingQuote) {
			ftnAdvance(lexer)
			// It was just one quote, so we've successfully reached the end of the literal.
			// Also check that an escaped quote isn't split in half by a line continuation.
			lexer.MarkEnd()
			ftnSkipLiteralContinuationSequence(lexer)
			if ftnLA(lexer) != int32(openingQuote) {
				return true
			}
		}
		ftnAdvance(lexer)
	}

	// We hit the end of the line without an '&', so this is an unclosed string literal (an error)
	return false
}

// ---------------------------------------------------------------------------
// Preprocessor unary operator (src/scanner.c:448-456); next_char is a char
// ---------------------------------------------------------------------------

func ftnScanPreprocUnaryOperator(lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	ch := ftnChar(ftnLA(lexer))
	if ch == '!' || ch == '~' || ch == '-' || ch == '+' {
		ftnAdvance(lexer)
		lexer.SetResultSymbol(syms[ftnTokPreprocUnaryOp])
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Labeled DO loop tracking (src/scanner.c:459-475)
// ---------------------------------------------------------------------------

func ftnTrackLabeledDo(s *ftnState, label int32) {
	// :461 C reads labels[depth-1] without testing depth. With an empty stack that
	// is labels[-1], which is the depth field itself (0), so a label of 0 matches,
	// and counts[-1]++ then increments labels[ftnMaxLabelStack-1], the item in front of
	// counts. Nothing is pushed in that case.
	if s.depth == 0 {
		if label == 0 {
			s.labels[ftnMaxLabelStack-1]++
			return
		}
	} else if s.depth > 0 && s.depth <= ftnMaxLabelStack {
		i := s.depth - 1
		if s.labels[i] == label {
			s.counts[i]++
			return
		}
	}

	// not at top of stack, assume new label, add it to stack
	if s.depth >= 0 && s.depth < ftnMaxLabelStack {
		s.labels[s.depth] = label
		s.counts[s.depth] = 1
		s.depth++
	}
}

// ftnScanDoLabelEos checks whether an end-of-statement token for virtual do labels
// is pending, emits END_OF_STATEMENT and updates internal state accordingly
// (src/scanner.c:479-487; no mark_end).
func ftnScanDoLabelEos(s *ftnState, lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	if s.isPendingEosVirtual {
		s.isPendingEosVirtual = false
		lexer.SetResultSymbol(syms[ftnTokEndOfStatement])
		return true
	}
	return false
}

// ftnScanDoLabelPending checks whether do labels are pending, emits
// DO_LABEL_VIRTUAL or DO_LABEL_CONTINUE and updates internal state accordingly
// (src/scanner.c:491-507; no mark_end).
func ftnScanDoLabelPending(s *ftnState, lexer *gotreesitter.ExternalLexer, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	if s.pendingLabelVirtual > 0 {
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
// consumed a proper integer value as a label (src/scanner.c:511-514). The token
// ends at the last mark_end of the number scan.
func ftnScanDoLabel(s *ftnState, lexer *gotreesitter.ExternalLexer, label int32, syms *[ftnTokenCount]gotreesitter.Symbol) {
	ftnTrackLabeledDo(s, label)
	lexer.SetResultSymbol(syms[ftnTokDoLabel])
}

func ftnScanDoLabelContinue(s *ftnState, lexer *gotreesitter.ExternalLexer, label int32, syms *[ftnTokenCount]gotreesitter.Symbol) bool {
	// determine whether this label belongs to the last labeled do,
	// if it does, remove it from stack and determine how many loops it closes
	var loopsToClose int32
	if s.depth > 0 && s.depth <= ftnMaxLabelStack {
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
// Scan label, number, or BOZ (src/scanner.c:543-574)
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
// Main scan entry point (src/scanner.c:576-654)
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
	for ftnIsBlank(ftnLA(lexer)) {
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
	for cIsSpace(ftnLA(lexer)) {
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
