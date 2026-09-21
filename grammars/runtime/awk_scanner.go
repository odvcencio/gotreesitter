//go:build !grammar_subset || grammar_subset_awk

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the awk grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see awkDefaultSymTable below.
const (
	awkTokConcatenatingSpace = 0
	awkTokIfElseSeparator    = 1
	awkTokNoSpace            = 2
	awkTokFuncCall           = 3
	awkTokenCount            = 4
)

// awkDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped awk.bin assigns to each external, in awkTok* order. It
// exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var awkDefaultSymTable = [awkTokenCount]gotreesitter.Symbol{
	138, // concatenating_space
	139, // _if_else_separator
	140, // _no_space
	141, // _func_call
}

// awkExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (awkTok* order).
var awkExternalScannerSpec = ExternalScannerSpec{
	Language:       "awk",
	UpstreamRepo:   "https://github.com/Beaglefoot/tree-sitter-awk",
	UpstreamCommit: "34bbdc7cce8e803096f47b625979e34c1be38127",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "a48511d40570cc25c829451107828502e6d0f48a91f331ec5ece8965c2aded71"},
		{Path: "src/scanner.c", SHA256: "bbeb8f95a6c6863fdd3533d03e086b18af1591f1d4399799dff83bef84dbb33a"},
	},
	Externals: []string{
		"concatenating_space",
		"_if_else_separator",
		"_no_space",
		"_func_call",
	},
}

func init() {
	RegisterExternalScannerSpec(awkExternalScannerSpec)
}

// AwkExternalScanner handles spacing-sensitive tokens for AWK.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type AwkExternalScanner struct {
	symbols         [awkTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers awk's external symbols.
func (AwkExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := AwkExternalScanner{symbols: awkDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, awkExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s AwkExternalScanner) symbolTable() *[awkTokenCount]gotreesitter.Symbol {
	if s.symbols == ([awkTokenCount]gotreesitter.Symbol{}) {
		return &awkDefaultSymTable
	}
	return &s.symbols
}

func (AwkExternalScanner) Create() any                           { return nil }
func (AwkExternalScanner) Destroy(payload any)                   {}
func (AwkExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (AwkExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols. Failed scans cannot mutate persistent state.
func (AwkExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (AwkExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (AwkExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (s AwkExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [awkTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < awkTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	stmtTermFound := false

	// NO_SPACE: zero-width token when next char is not whitespace
	if awkValid(validSymbols, awkTokNoSpace) {
		if !awkIsWhitespace(lexer.Lookahead()) {
			lexer.SetResultSymbol(syms[awkTokNoSpace])
			return true
		}
	}

	// FUNC_CALL: zero-width token when next char is '(' with no whitespace
	if awkValid(validSymbols, awkTokFuncCall) {
		if !awkIsWhitespace(lexer.Lookahead()) && lexer.Lookahead() == '(' {
			lexer.SetResultSymbol(syms[awkTokFuncCall])
			return true
		}
	}

	// IF_ELSE_SEPARATOR: zero-width, checks if "else" follows after whitespace/newlines
	if awkValid(validSymbols, awkTokIfElseSeparator) {
		awkSkipWS(lexer, false)

		if awkIsStatementTerminator(lexer.Lookahead()) || lexer.Lookahead() == '#' {
			stmtTermFound = true
		}

		if awkIsIfElseSep(lexer) {
			lexer.SetResultSymbol(syms[awkTokIfElseSeparator])
			return true
		}
	}

	// CONCATENATING_SPACE: whitespace that acts as string concatenation
	if awkValid(validSymbols, awkTokConcatenatingSpace) && !stmtTermFound {
		if awkIsConcatenatingSpace(lexer) {
			lexer.SetResultSymbol(syms[awkTokConcatenatingSpace])
			return true
		}
	}

	return false
}

func awkIsWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t'
}

func awkIsStatementTerminator(ch rune) bool {
	return ch == '\n' || ch == ';'
}

func awkIsLineContinuation(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() == '\\' {
		lexer.Advance(true)
		if lexer.Lookahead() == '\r' {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '\n' {
			return true
		}
	}
	return false
}

func awkSkipWS(lexer *gotreesitter.ExternalLexer, skipNewlines bool) {
	for awkIsWhitespace(lexer.Lookahead()) || awkIsLineContinuation(lexer) || lexer.Lookahead() == '\r' || (skipNewlines && lexer.Lookahead() == '\n') {
		lexer.Advance(true)
	}
}

func awkSkipComment(lexer *gotreesitter.ExternalLexer) {
	if lexer.Lookahead() != '#' {
		return
	}
	for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
		lexer.Advance(true)
	}
	lexer.Advance(false)
	awkSkipWS(lexer, true)
	if lexer.Lookahead() == '#' {
		awkSkipComment(lexer)
	}
}

func awkNextCharsEq(lexer *gotreesitter.ExternalLexer, word string) bool {
	for _, ch := range word {
		if lexer.Lookahead() != ch {
			return false
		}
		lexer.Advance(true)
	}
	return true
}

func awkIsIfElseSep(lexer *gotreesitter.ExternalLexer) bool {
	// Skip whitespace, newlines, semicolons
	for awkIsWhitespace(lexer.Lookahead()) || awkIsStatementTerminator(lexer.Lookahead()) || lexer.Lookahead() == '\r' {
		lexer.Advance(true)
	}
	lexer.MarkEnd()

	if lexer.Lookahead() == '#' {
		awkSkipComment(lexer)
		awkSkipWS(lexer, false)
	}

	return awkNextCharsEq(lexer, "else")
}

func awkIsConcatenatingSpace(lexer *gotreesitter.ExternalLexer) bool {
	hadWS := false
	for awkIsWhitespace(lexer.Lookahead()) || awkIsLineContinuation(lexer) || lexer.Lookahead() == '\r' {
		lexer.Advance(false)
		hadWS = true
	}
	_ = hadWS
	lexer.MarkEnd()

	switch lexer.Lookahead() {
	case '^', '*', '/', '%', '+', '-', '<', '>', '=', '!', '~',
		'&', '|', ',', '?', ':', ')', '[', ']', '{', '}', '#', ';', '\n':
		return false
	case 'i':
		lexer.Advance(true)
		ch := lexer.Lookahead()
		if ch == 'n' || ch == 'f' {
			lexer.Advance(true)
			return lexer.Lookahead() != ' '
		}
		return lexer.Lookahead() != 0
	default:
		return lexer.Lookahead() != 0
	}
}

func awkValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
