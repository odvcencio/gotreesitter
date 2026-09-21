//go:build !grammar_subset || grammar_subset_matlab

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the MATLAB grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see matDefaultSymTable below.
const (
	matTokComment                = 0
	matTokLineContinuation       = 1
	matTokCommandName            = 2
	matTokCommandArgument        = 3
	matTokSingleQuoteStringStart = 4
	matTokSingleQuoteStringEnd   = 5
	matTokDoubleQuoteStringStart = 6
	matTokDoubleQuoteStringEnd   = 7
	matTokFormattingSequence     = 8
	matTokEscapeSequence         = 9
	matTokStringContent          = 10
	matTokEntryDelimiter         = 11
	matTokMultioutputVarStart    = 12
	matTokIdentifier             = 13
	matTokCatchIdentifier        = 14
	matTokTranspose              = 15
	matTokCTranspose             = 16
	matTokErrorSentinel          = 17
	matTokenCount                = 18
)

// matDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped matlab.bin assigns to each external, in matTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var matDefaultSymTable = [matTokenCount]gotreesitter.Symbol{
	78, // comment
	79, // line_continuation
	80, // command_name
	81, // command_argument
	82, // _single_quote_string_start, displays as "'"
	83, // _single_quote_string_end, displays as "'"
	84, // _double_quote_string_start, displays as "\""
	85, // _double_quote_string_end, displays as "\""
	86, // formatting_sequence
	87, // escape_sequence
	88, // string_content
	89, // _entry_delimiter, displays as ","
	90, // _multioutput_variable_start, displays as "["
	91, // _external_identifier (no alias -- displays as its own rule name)
	92, // _catch_identifier, displays as "identifier" (aliased, unlike index 13 above)
	93, // _transpose, displays as "'"
	94, // _ctranspose, displays as ".'"
	95, // error_sentinel
}

// matExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (matTok* order).
var matExternalScannerSpec = ExternalScannerSpec{
	Language:       "matlab",
	UpstreamRepo:   "https://github.com/acristoffers/tree-sitter-matlab",
	UpstreamCommit: "574dde565caddf8cf44eec7df3cb89eb96053ed7",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "2bd9cb879c3c3f164a283aa6b91903f66dccde6086d6e2f12a3b10d2b1a2e01b"},
		{Path: "src/scanner.c", SHA256: "4beb57040f40601bc606dd04b51cac85cd685b17d8ae00561f648844b003ac59"},
	},
	Externals: []string{
		"comment",
		"line_continuation",
		"command_name",
		"command_argument",
		"_single_quote_string_start",
		"_single_quote_string_end",
		"_double_quote_string_start",
		"_double_quote_string_end",
		"formatting_sequence",
		"escape_sequence",
		"string_content",
		"_entry_delimiter",
		"_multioutput_variable_start",
		"_external_identifier",
		"_catch_identifier",
		"_transpose",
		"_ctranspose",
		"error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(matExternalScannerSpec)
}

// matKeywords are the reserved keywords in MATLAB.
var matKeywords = []string{
	"arguments", "break", "case", "catch", "classdef", "continue", "else", "elseif",
	"end", "enumeration", "events", "for", "function", "global", "if", "methods",
	"otherwise", "parfor", "persistent", "return", "spmd", "switch", "try", "while",
}

type matlabState struct {
	isInsideCommand        bool
	lineContinuation       bool
	isShellEscape          bool
	stringDelimiter        rune
	generateEntryDelimiter bool
}

// MatlabExternalScanner handles the external scanning for the MATLAB grammar.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type MatlabExternalScanner struct {
	symbols         [matTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers matlab's external symbols.
func (MatlabExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := MatlabExternalScanner{symbols: matDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, matExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s MatlabExternalScanner) symbolTable() *[matTokenCount]gotreesitter.Symbol {
	if s.symbols == ([matTokenCount]gotreesitter.Symbol{}) {
		return &matDefaultSymTable
	}
	return &s.symbols
}

func (MatlabExternalScanner) Create() any {
	return &matlabState{}
}

func (MatlabExternalScanner) Destroy(payload any) {}

func (MatlabExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*matlabState)
	if len(buf) < 5 {
		return 0
	}
	boolByte := func(b bool) byte {
		if b {
			return 1
		}
		return 0
	}
	buf[0] = boolByte(s.isInsideCommand)
	buf[1] = boolByte(s.lineContinuation)
	buf[2] = boolByte(s.isShellEscape)
	buf[3] = byte(s.stringDelimiter)
	buf[4] = boolByte(s.generateEntryDelimiter)
	return 5
}

func (MatlabExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*matlabState)
	*s = matlabState{}
	if len(buf) == 5 {
		s.isInsideCommand = buf[0] != 0
		s.lineContinuation = buf[1] != 0
		s.isShellEscape = buf[2] != 0
		s.stringDelimiter = rune(buf[3])
		s.generateEntryDelimiter = buf[4] != 0
	}
}

func (sc MatlabExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*matlabState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [matTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < matTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	if s.generateEntryDelimiter {
		s.generateEntryDelimiter = false
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[matTokEntryDelimiter])
		return true
	}

	if s.stringDelimiter == 0 {
		skipped := matSkipWhitespaces(lexer)

		if (s.lineContinuation || !s.isInsideCommand) && isValid(matTokComment) &&
			(lexer.Lookahead() == '%' || ((skipped&2) == 0 && lexer.Lookahead() == '.')) {
			return matScanComment(s, lexer, isValid(matTokEntryDelimiter), isValid(matTokCTranspose), skipped, syms)
		}

		if !s.isInsideCommand {
			if skipped == 0 && isValid(matTokTranspose) {
				if matScanTranspose(lexer, syms) {
					return true
				}
			}

			if (isValid(matTokSingleQuoteStringStart) && lexer.Lookahead() == '\'') ||
				(isValid(matTokDoubleQuoteStringStart) && lexer.Lookahead() == '"') {
				return matScanStringOpen(s, lexer, syms)
			}

			if !s.lineContinuation {
				if isValid(matTokMultioutputVarStart) && lexer.Lookahead() == '[' {
					return matScanMultioutputVarStart(lexer, syms)
				}

				if isValid(matTokEntryDelimiter) {
					return matScanEntryDelimiter(lexer, skipped, syms)
				}
			}

			if isValid(matTokCommandName) {
				s.isInsideCommand = false
				s.isShellEscape = false
				return matScanCommand(s, lexer, validSymbols, syms)
			}

			if isValid(matTokIdentifier) && (skipped&2) == 0 {
				s.isInsideCommand = false
				s.isShellEscape = false
				return matScanIdentifier(lexer, syms)
			}
		} else {
			if isValid(matTokCommandArgument) {
				return matScanCommandArgument(s, lexer, syms)
			}
		}
	} else {
		if isValid(matTokDoubleQuoteStringEnd) || isValid(matTokSingleQuoteStringEnd) ||
			isValid(matTokFormattingSequence) {
			return matScanStringClose(s, lexer, syms)
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func matIsPunctChar(chr rune) bool {
	if chr >= 0x80 {
		return false
	}
	return (chr >= 33 && chr <= 47) || // !"#$%&'()*+,-./
		(chr >= 58 && chr <= 64) || // :;<=>?@
		(chr >= 91 && chr <= 96) || // [\]^_`
		(chr >= 123 && chr <= 126) // {|}~
}

func matIsEol(chr rune) bool {
	return chr == '\n' || chr == '\r' || chr == ',' || chr == ';'
}

func matIsWspaceMatlab(chr rune) bool {
	return unicode.IsSpace(chr) && chr != '\n' && chr != '\r'
}

func matIsIdentifierChar(chr rune, start bool) bool {
	if chr >= 0x80 {
		return false
	}
	alpha := unicode.IsLetter(chr)
	numeric := !start && unicode.IsDigit(chr)
	special := chr == '_'
	return alpha || numeric || special
}

func matConsumeChar(chr rune, lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != chr {
		return false
	}
	lexer.Advance(false)
	return true
}

func matConsumeLineContinuation(lexer *gotreesitter.ExternalLexer) bool {
	for i := 0; i < 3; i++ {
		if !matConsumeChar('.', lexer) {
			return false
		}
	}
	return true
}

func matConsumeIdentifier(lexer *gotreesitter.ExternalLexer) string {
	var buf []byte
	if matIsIdentifierChar(lexer.Lookahead(), true) {
		buf = append(buf, byte(lexer.Lookahead()))
		lexer.Advance(false)
		for matIsIdentifierChar(lexer.Lookahead(), false) {
			if len(buf) >= 256 {
				return ""
			}
			buf = append(buf, byte(lexer.Lookahead()))
			lexer.Advance(false)
		}
		return string(buf)
	}
	return ""
}

// matSkipWhitespaces skips whitespace (using skip=true).
// Return value bits: 0b001 -> something skipped, 0b010 -> newline skipped,
// 0b100 -> newline was at the end of skipped sequence.
func matSkipWhitespaces(lexer *gotreesitter.ExternalLexer) int {
	skipped := 0
	for lexer.Lookahead() != 0 && unicode.IsSpace(lexer.Lookahead()) {
		skipped &= 0b011
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
			skipped |= 0b111
		} else {
			skipped |= 0b001
		}
		lexer.Advance(true)
	}
	return skipped
}

// matConsumeWhitespaces consumes whitespace (using skip=false / advance).
func matConsumeWhitespaces(lexer *gotreesitter.ExternalLexer) int {
	skipped := 0
	for unicode.IsSpace(lexer.Lookahead()) {
		skipped &= 0b011
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
			skipped |= 0b111
		} else {
			skipped |= 0b001
		}
		lexer.Advance(false)
	}
	return skipped
}

// matConsumeWhitespacesOnce consumes whitespace up to (and including) one newline.
func matConsumeWhitespacesOnce(lexer *gotreesitter.ExternalLexer) {
	for lexer.Lookahead() != 0 && unicode.IsSpace(lexer.Lookahead()) {
		if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' {
			lexer.Advance(false)
			break
		}
		lexer.Advance(false)
	}
}

func matConsumeCommentLine(lexer *gotreesitter.ExternalLexer) {
	for lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' && lexer.Lookahead() != 0 {
		lexer.Advance(false)
	}
}

func matIsKeyword(s string) bool {
	for _, kw := range matKeywords {
		if kw == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Comment scanning
// ---------------------------------------------------------------------------

func matScanComment(
	scanner *matlabState,
	lexer *gotreesitter.ExternalLexer,
	entryDelimiter bool,
	ctranspose bool,
	skipped int,
	syms *[matTokenCount]gotreesitter.Symbol,
) bool {
	lexer.MarkEnd()

	percent := lexer.Lookahead() == '%'
	lineContinuation := matConsumeLineContinuation(lexer)
	block := percent && matConsumeChar('%', lexer) && matConsumeChar('{', lexer)

	// Handle the case where '.' is followed by a digit inside matrices/cells.
	if entryDelimiter && !percent && !lineContinuation {
		if unicode.IsDigit(lexer.Lookahead()) {
			lexer.SetResultSymbol(syms[matTokEntryDelimiter])
			return true
		}
		if lexer.Lookahead() == '\'' {
			lexer.Advance(false)
			lexer.SetResultSymbol(syms[matTokCTranspose])
			lexer.MarkEnd()
			return skipped == 0
		}
		return false
	}

	// Line continuation inside a matrix/cell row.
	if entryDelimiter && lineContinuation {
		matConsumeCommentLine(lexer)
		matConsumeWhitespaces(lexer)

		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[matTokLineContinuation])

		la := lexer.Lookahead()
		isAlpha := unicode.IsLetter(la)
		isDigit := unicode.IsDigit(la)
		isMeta := la == '?' || la == '@'
		isQuote := la == '\'' || la == '"'
		isContainer := la == '{' || la == '[' || la == '('

		if la == '~' {
			lexer.Advance(false)
			scanner.generateEntryDelimiter = lexer.Lookahead() != '='
		} else if la == '+' || la == '-' {
			lexer.Advance(false)
			scanner.generateEntryDelimiter = lexer.Lookahead() != ' '
		} else if la == '.' {
			lexer.Advance(false)
			scanner.generateEntryDelimiter = unicode.IsDigit(lexer.Lookahead())
		} else if isAlpha || isDigit || isQuote || isContainer || isMeta {
			scanner.generateEntryDelimiter = true
		}
		return true
	}

	if block {
		if skipped&2 != 0 {
			return false
		}

		// If it has things on the same line, it's not a block, just a comment.
		for lexer.Lookahead() != 0 && matIsWspaceMatlab(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		if !matConsumeChar('\n', lexer) && !matConsumeChar('\r', lexer) {
			matConsumeCommentLine(lexer)
			lexer.SetResultSymbol(syms[matTokComment])
			lexer.MarkEnd()
			return true
		}

		// Otherwise, find the matching closing block.
		level := 1
		for lexer.Lookahead() != 0 {
			matConsumeWhitespaces(lexer)
			if matConsumeChar('%', lexer) {
				if matConsumeChar('{', lexer) && (matConsumeWhitespaces(lexer)&2) != 0 {
					level++
				} else if matConsumeChar('}', lexer) {
					lexer.MarkEnd()
					if (matConsumeWhitespaces(lexer) & 2) != 0 {
						level--
					}
				}
				if level == 0 {
					break
				}
				continue
			}
			matConsumeCommentLine(lexer)
			lexer.MarkEnd()
		}

		lexer.SetResultSymbol(syms[matTokComment])
		return true
	}

	if percent || lineContinuation {
		matConsumeCommentLine(lexer)
		lexer.MarkEnd()

		if !lineContinuation {
			lexer.SetResultSymbol(syms[matTokComment])
			lexer.Advance(false)
		} else {
			lexer.SetResultSymbol(syms[matTokLineContinuation])
			matConsumeWhitespacesOnce(lexer)
			lexer.MarkEnd()
			return true
		}

		// Merges consecutive comments into one token, unless separated by a newline.
		for lexer.Lookahead() != 0 && (lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t') {
			lexer.Advance(false)
		}

		if lexer.Lookahead() == '%' {
			return matScanComment(scanner, lexer, false, false, 0, syms)
		}

		return true
	}

	if ctranspose && lexer.Lookahead() == '\'' {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[matTokCTranspose])
		return true
	}

	return false
}

// ---------------------------------------------------------------------------
// Command scanning
// ---------------------------------------------------------------------------

func matScanCommand(scanner *matlabState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[matTokenCount]gotreesitter.Symbol) bool {
	isValid := func(idx int) bool {
		return idx < len(validSymbols) && validSymbols[idx]
	}

	// Special case: shell escape
	if lexer.Lookahead() == '!' {
		lexer.Advance(false)
		for matIsWspaceMatlab(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		for lexer.Lookahead() != ' ' && lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}
		lexer.SetResultSymbol(syms[matTokCommandName])
		lexer.MarkEnd()
		for matIsWspaceMatlab(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		scanner.isInsideCommand = lexer.Lookahead() != '\n'
		scanner.isShellEscape = scanner.isInsideCommand
		return true
	}

	if !matIsIdentifierChar(lexer.Lookahead(), true) {
		return false
	}

	buffer := matConsumeIdentifier(lexer)
	lexer.MarkEnd()

	allowedCommands := []string{"methods", "arguments", "enumeration", "events"}

	if buffer != "" {
		if lexer.Lookahead() == '.' {
			// Since it is not followed by a space, it cannot be a command.
			if buffer == "get" || buffer == "set" {
				return false
			}
			// Check for line continuation
			if matConsumeLineContinuation(lexer) {
				// If it is a keyword, yield to the internal scanner
				for _, kw := range matKeywords {
					if kw == buffer {
						if buffer == "enumeration" {
							goto checkEnumeration
						}
						return false
					}
				}
			}
			lexer.SetResultSymbol(syms[matTokIdentifier])
			return true
		}
		// The following keywords are allowed as commands if they get 1 argument
		for _, cmd := range allowedCommands {
			if cmd == buffer {
				goto checkCommandForArgument
			}
		}
		if matIsKeyword(buffer) {
			return false
		}
	}
	goto skipCommandCheck

checkEnumeration:
	{
		ws := matConsumeWhitespaces(lexer)
		if ws&2 != 0 {
			// enumeration can be a function
			if lexer.Lookahead() == '(' {
				lexer.SetResultSymbol(syms[matTokIdentifier])
				return true
			}
		}
		return false
	}

checkCommandForArgument:
	// If this is a keyword-command, check if it has an argument.
	lexer.SetResultSymbol(syms[matTokCommandName])
	for lexer.Lookahead() != 0 && matIsWspaceMatlab(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	if matIsIdentifierChar(lexer.Lookahead(), true) {
		scanner.isInsideCommand = true
		return true
	}
	return false

skipCommandCheck:
	// First case: found an end-of-line already, so this is a command for sure.
	if matIsEol(lexer.Lookahead()) {
		if isValid(matTokCatchIdentifier) {
			lexer.SetResultSymbol(syms[matTokCatchIdentifier])
		} else {
			lexer.SetResultSymbol(syms[matTokCommandName])
		}
		return true
	}

	// If it's not followed by a space, it may be something else.
	if lexer.Lookahead() != ' ' {
		lexer.SetResultSymbol(syms[matTokIdentifier])
		return true
	}

	// If followed by a line continuation, look after it
	ws := matConsumeWhitespaces(lexer)
	if ws&2 != 0 {
		// `catch e `
		if isValid(matTokCatchIdentifier) {
			lexer.SetResultSymbol(syms[matTokCatchIdentifier])
			return true
		}
		// Command followed by spaces then newline
		scanner.isInsideCommand = false
		lexer.SetResultSymbol(syms[matTokCommandName])
		return true
	}
	if matConsumeLineContinuation(lexer) {
		lexer.SetResultSymbol(syms[matTokIdentifier])
		return true
	}

	// Mark it already as this is the right place.
	lexer.SetResultSymbol(syms[matTokCommandName])
	for lexer.Lookahead() != 0 && matIsWspaceMatlab(lexer.Lookahead()) {
		lexer.Advance(false)
	}

	// Check for end-of-line again.
	if matIsEol(lexer.Lookahead()) || lexer.Lookahead() == '%' {
		if isValid(matTokCatchIdentifier) && (ws&4) == 0 {
			lexer.SetResultSymbol(syms[matTokCatchIdentifier])
			return true
		}
		scanner.isInsideCommand = true
		return true
	}

	// The first char of the first argument cannot be /=()/
	if lexer.Lookahead() == '=' || lexer.Lookahead() == '(' || lexer.Lookahead() == ')' {
		lexer.SetResultSymbol(syms[matTokIdentifier])
		return true
	}

	// If it is a single quote, it is a command.
	if lexer.Lookahead() == '\'' {
		scanner.isInsideCommand = true
		return true
	}

	// If it is an identifier char, then it's a command.
	if matIsIdentifierChar(lexer.Lookahead(), false) {
		scanner.isInsideCommand = true
		return true
	}

	// If it is a char >= 0xC0, assume it's a valid UTF-8 char and a command.
	if lexer.Lookahead() >= 0xC0 {
		scanner.isInsideCommand = true
		return true
	}

	// Let's now consider punctuation marks.
	if matIsPunctChar(lexer.Lookahead()) {
		first := lexer.Lookahead()
		lexer.Advance(false)
		second := lexer.Lookahead()

		// If it's the end-of-line, then it's a command.
		if matIsEol(second) {
			scanner.isInsideCommand = true
			return true
		}

		if matIsWspaceMatlab(second) {
			operators := []rune{'!', '&', '*', '+', '-', '/', '<', '>', '@', '\\', '^', '|'}
			isInvalid := false
			for _, op := range operators {
				if first == op {
					isInvalid = true
					break
				}
			}
			if isInvalid {
				lexer.Advance(false)
				for matIsWspaceMatlab(lexer.Lookahead()) {
					lexer.Advance(false)
				}
				scanner.isInsideCommand = matIsEol(lexer.Lookahead())
				if scanner.isInsideCommand {
					lexer.SetResultSymbol(syms[matTokCommandName])
				} else {
					lexer.SetResultSymbol(syms[matTokIdentifier])
				}
				return true
			}

			// If it's not an operator, then this is a command.
			scanner.isInsideCommand = true
			return true
		}

		// Now check for two-character operators.
		lexer.Advance(false)

		if lexer.Lookahead() != ' ' {
			scanner.isInsideCommand = true
			return true
		}

		twoCharOps := [][2]rune{
			{'&', '&'}, {'|', '|'}, {'=', '='}, {'~', '='},
			{'<', '='}, {'>', '='}, {'.', '+'}, {'.', '-'},
			{'.', '*'}, {'.', '/'}, {'.', '\\'}, {'.', '^'},
		}

		for _, op := range twoCharOps {
			if first == op[0] && second == op[1] {
				lexer.SetResultSymbol(syms[matTokIdentifier])
				return true
			}
		}

		scanner.isInsideCommand = true
		return true
	}

	return false
}

// ---------------------------------------------------------------------------
// Command argument scanning
// ---------------------------------------------------------------------------

func matScanCommandArgument(scanner *matlabState, lexer *gotreesitter.ExternalLexer, syms *[matTokenCount]gotreesitter.Symbol) bool {
	// Shell escape: break arguments on spaces.
	if scanner.isShellEscape {
		if lexer.Lookahead() == 0 {
			return false
		}

		for lexer.Lookahead() != ' ' && lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
			lexer.Advance(false)
		}
		lexer.SetResultSymbol(syms[matTokCommandArgument])
		lexer.MarkEnd()
		for matIsWspaceMatlab(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '\n' {
			scanner.isInsideCommand = false
			scanner.isShellEscape = false
		}
		return true
	}

	// Avoids infinite loop when the argument is right before the eof.
	if lexer.Lookahead() == 0 {
		return false
	}

	quote := false
	parens := int32(0)
	consumed := false

	for lexer.Lookahead() != 0 {
		// No matter what, found new line
		cond1 := lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r'
		// No quotes, no parens, found end-of-line or space
		cond2 := !quote && parens == 0 &&
			(matIsEol(lexer.Lookahead()) || matIsWspaceMatlab(lexer.Lookahead()))
		// Inside parens, no quotes, found ;
		cond3 := !quote && parens != 0 && lexer.Lookahead() == ';'

		if cond1 || cond2 || cond3 {
			lexer.SetResultSymbol(syms[matTokCommandArgument])
			lexer.MarkEnd()

			for matIsWspaceMatlab(lexer.Lookahead()) {
				lexer.Advance(false)
			}

			if matIsEol(lexer.Lookahead()) || cond1 {
				scanner.lineContinuation = false
				scanner.isInsideCommand = false
			}

			return true
		}

		// Line comment, finish.
		if (!quote || (quote && parens != 0)) && lexer.Lookahead() == '%' {
			scanner.isInsideCommand = false
			if consumed {
				lexer.SetResultSymbol(syms[matTokCommandArgument])
				lexer.MarkEnd()
				return true
			}
			return matScanComment(scanner, lexer, false, false, 0, syms)
		}

		// Line continuation
		if (!quote || (quote && parens != 0)) && lexer.Lookahead() == '.' {
			lexer.SetResultSymbol(syms[matTokCommandArgument])
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '.' {
				lexer.Advance(false)
				if lexer.Lookahead() == '.' {
					if consumed {
						scanner.lineContinuation = true
					} else {
						matConsumeCommentLine(lexer)
						lexer.SetResultSymbol(syms[matTokLineContinuation])
						lexer.MarkEnd()
					}
					return true
				}
				consumed = true
				continue
			}
			consumed = true
			continue
		}

		if (lexer.Lookahead() == '(' || lexer.Lookahead() == '[' || lexer.Lookahead() == '{') &&
			(!quote || (quote && parens != 0)) {
			parens++
		}

		if (lexer.Lookahead() == ')' || lexer.Lookahead() == ']' || lexer.Lookahead() == '}') &&
			(!quote || (quote && parens != 0)) {
			parens--
		}

		if lexer.Lookahead() == '\'' {
			quote = !quote
		}

		lexer.Advance(false)
		consumed = true
	}

	// Mark as argument so the scanner doesn't get called again in an infinite loop.
	if lexer.Lookahead() == 0 {
		lexer.SetResultSymbol(syms[matTokCommandArgument])
		lexer.MarkEnd()
		return true
	}

	return false
}

// ---------------------------------------------------------------------------
// String scanning
// ---------------------------------------------------------------------------

func matScanStringOpen(scanner *matlabState, lexer *gotreesitter.ExternalLexer, syms *[matTokenCount]gotreesitter.Symbol) bool {
	switch lexer.Lookahead() {
	case '"':
		scanner.stringDelimiter = '"'
		lexer.Advance(false)
		lexer.SetResultSymbol(syms[matTokDoubleQuoteStringStart])
		lexer.MarkEnd()
		return true
	case '\'':
		scanner.stringDelimiter = '\''
		lexer.Advance(false)
		lexer.SetResultSymbol(syms[matTokSingleQuoteStringStart])
		lexer.MarkEnd()
		// A single quote string has to be ended in the same line.
		for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' {
			if lexer.Lookahead() == '\'' {
				return true
			}
			lexer.Advance(false)
		}
		return false
	default:
		return false
	}
}

func matScanStringClose(scanner *matlabState, lexer *gotreesitter.ExternalLexer, syms *[matTokenCount]gotreesitter.Symbol) bool {
	if lexer.Lookahead() == scanner.stringDelimiter {
		lexer.Advance(false)
		if lexer.Lookahead() == scanner.stringDelimiter {
			lexer.Advance(false)
			lexer.SetResultSymbol(syms[matTokStringContent])
			goto content
		}
		if scanner.stringDelimiter == '"' {
			lexer.SetResultSymbol(syms[matTokDoubleQuoteStringEnd])
		} else {
			lexer.SetResultSymbol(syms[matTokSingleQuoteStringEnd])
		}
		lexer.MarkEnd()
		scanner.stringDelimiter = 0
		return true
	}

	// Unterminated string.
	if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || lexer.Lookahead() == 0 {
		scanner.stringDelimiter = 0
		return false
	}

	if lexer.Lookahead() == '%' {
		lexer.Advance(false)

		if lexer.Lookahead() == '%' {
			lexer.Advance(false)
			lexer.SetResultSymbol(syms[matTokFormattingSequence])
			lexer.MarkEnd()
			return true
		}

		validTokens := "1234567890.-+ #btcdeEfgGosuxX"
		endTokens := "cdeEfgGosuxX"
		for lexer.Lookahead() != 0 && lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' {
			isValidCh := false
			for _, ch := range validTokens {
				if lexer.Lookahead() == ch {
					isValidCh = true
					break
				}
			}

			if !isValidCh {
				lexer.SetResultSymbol(syms[matTokStringContent])
				goto content
			}

			for _, ch := range endTokens {
				if lexer.Lookahead() == ch {
					lexer.Advance(false)
					lexer.SetResultSymbol(syms[matTokFormattingSequence])
					lexer.MarkEnd()
					return true
				}
			}

			lexer.Advance(false)
		}

		scanner.stringDelimiter = 0
		return false
	}

	if lexer.Lookahead() == '\\' {
		lexer.Advance(false)

		if lexer.Lookahead() == 'x' {
			lexer.Advance(false)
			for lexer.Lookahead() != 0 {
				hexaChars := "1234567890abcdefABCDEF"
				isValidHex := false
				for _, ch := range hexaChars {
					if lexer.Lookahead() == ch {
						isValidHex = true
						break
					}
				}

				if !isValidHex {
					lexer.SetResultSymbol(syms[matTokEscapeSequence])
					lexer.MarkEnd()
					return true
				}

				lexer.Advance(false)
			}
		}

		if lexer.Lookahead() >= '0' && lexer.Lookahead() <= '7' {
			for lexer.Lookahead() >= '0' && lexer.Lookahead() <= '7' && lexer.Lookahead() != 0 {
				lexer.Advance(false)
			}

			lexer.SetResultSymbol(syms[matTokEscapeSequence])
			lexer.MarkEnd()
			return true
		}

		escapes := "abfnrtv\\"
		isValidEsc := false
		for _, ch := range escapes {
			if lexer.Lookahead() == ch {
				isValidEsc = true
				break
			}
		}

		if isValidEsc {
			lexer.Advance(false)
			lexer.SetResultSymbol(syms[matTokEscapeSequence])
			lexer.MarkEnd()
			return true
		}
	}

content:
	for lexer.Lookahead() != '\n' && lexer.Lookahead() != '\r' && lexer.Lookahead() != 0 {
		// In MATLAB '' and "" are valid escapes inside their own kind.
		if lexer.Lookahead() == scanner.stringDelimiter {
			lexer.SetResultSymbol(syms[matTokStringContent])
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() != scanner.stringDelimiter {
				return true
			}
			lexer.Advance(false)
			continue
		}

		// The scanner will be called again for % or \ sequences.
		if lexer.Lookahead() == '%' || lexer.Lookahead() == '\\' {
			lexer.SetResultSymbol(syms[matTokStringContent])
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == scanner.stringDelimiter || matIsWspaceMatlab(lexer.Lookahead()) {
				goto content
			}
			return true
		}

		lexer.Advance(false)
	}

	// Unterminated string: mark end of content here.
	if lexer.Lookahead() == '\n' || lexer.Lookahead() == '\r' || lexer.Lookahead() == 0 {
		lexer.SetResultSymbol(syms[matTokStringContent])
		lexer.MarkEnd()
		return true
	}

	scanner.stringDelimiter = 0
	return false
}

// ---------------------------------------------------------------------------
// Multi-output variable start
// ---------------------------------------------------------------------------

func matScanMultioutputVarStart(lexer *gotreesitter.ExternalLexer, syms *[matTokenCount]gotreesitter.Symbol) bool {
	lexer.Advance(false)
	lexer.SetResultSymbol(syms[matTokMultioutputVarStart])
	lexer.MarkEnd()

	var sbCount uint32

	for lexer.Lookahead() != 0 {
		if matConsumeLineContinuation(lexer) {
			matConsumeCommentLine(lexer)
			lexer.Advance(false)
		}

		if lexer.Lookahead() == '[' {
			sbCount++
			lexer.Advance(false)
		}

		if lexer.Lookahead() != ']' {
			lexer.Advance(false)
		} else if sbCount > 0 {
			sbCount--
			lexer.Advance(false)
		} else {
			break
		}
	}

	if lexer.Lookahead() != ']' {
		return false
	}

	lexer.Advance(false)

	for lexer.Lookahead() != 0 {
		if matConsumeLineContinuation(lexer) {
			matConsumeCommentLine(lexer)
			lexer.Advance(false)
		} else if matIsWspaceMatlab(lexer.Lookahead()) {
			lexer.Advance(false)
		} else {
			break
		}
	}

	if lexer.Lookahead() == '=' {
		lexer.Advance(false)
		if lexer.Lookahead() != '=' {
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Entry delimiter scanning
// ---------------------------------------------------------------------------

func matScanEntryDelimiter(lexer *gotreesitter.ExternalLexer, skipped int, syms *[matTokenCount]gotreesitter.Symbol) bool {
	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[matTokEntryDelimiter])

	if skipped&2 != 0 {
		return false
	}

	if lexer.Lookahead() == ',' {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[matTokEntryDelimiter])
		return true
	}

	if lexer.Lookahead() == '.' {
		lexer.Advance(false)
		lexer.Advance(false)
		return unicode.IsDigit(lexer.Lookahead())
	}

	if lexer.Lookahead() == '{' || lexer.Lookahead() == '(' || lexer.Lookahead() == '\'' {
		return skipped != 0
	}

	if lexer.Lookahead() == '[' {
		return true
	}

	// These chars mean we cannot end the cell here.
	noEnd := []rune{']', '}', '&', '|', '=', '<', '>', '*', '/', '\\', '^', ';', ':'}
	for _, ch := range noEnd {
		if lexer.Lookahead() == ch {
			return false
		}
	}

	if lexer.Lookahead() == '~' {
		lexer.Advance(false)
		return lexer.Lookahead() != '='
	}

	maybeEnd := []rune{'+', '-'}
	for _, ch := range maybeEnd {
		if lexer.Lookahead() == ch {
			lexer.Advance(false)
			if lexer.Lookahead() == ' ' {
				return false
			}
			return skipped != 0
		}
	}

	if skipped != 0 {
		return true
	}

	if matIsIdentifierChar(lexer.Lookahead(), true) {
		return matScanIdentifier(lexer, syms)
	}

	return false
}

// ---------------------------------------------------------------------------
// Identifier scanning
// ---------------------------------------------------------------------------

func matScanIdentifier(lexer *gotreesitter.ExternalLexer, syms *[matTokenCount]gotreesitter.Symbol) bool {
	buffer := matConsumeIdentifier(lexer)
	if buffer != "" {
		if lexer.Lookahead() == '.' {
			if buffer == "get" || buffer == "set" {
				return false
			}
			lexer.SetResultSymbol(syms[matTokIdentifier])
			lexer.MarkEnd()
			return true
		}
		if matIsKeyword(buffer) {
			return false
		}
		lexer.SetResultSymbol(syms[matTokIdentifier])
		lexer.MarkEnd()
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Transpose scanning
// ---------------------------------------------------------------------------

func matScanTranspose(lexer *gotreesitter.ExternalLexer, syms *[matTokenCount]gotreesitter.Symbol) bool {
	if lexer.Lookahead() == '\'' {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[matTokTranspose])
		return true
	}
	// Faithfully ported from C: the original checks lookahead == '.' && consume_char('\'')
	// which will always be false when lookahead is '.', since consume_char checks for '\''.
	// The CTRANSPOSE (.' operator) is handled elsewhere (in scan_comment).
	if lexer.Lookahead() == '.' && matConsumeChar('\'', lexer) {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[matTokCTranspose])
		return true
	}
	return false
}
