//go:build !grammar_subset || grammar_subset_earthfile

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the earthfile grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see earthfileDefaultSymTable below.
const (
	earthfileTokIndent  = 0
	earthfileTokDedent  = 1
	earthfileTokenCount = 2
)

// earthfileDefaultSymTable records the concrete gotreesitter.Symbol IDs
// the currently shipped earthfile.bin assigns to each external, in
// earthfileTok* order. It exists only as a pre-bind fallback (and as an
// independent value to compare a real bind against in tests);
// ExternalScannerForLanguage below overwrites it with values read from
// the actual loaded Language at bind time, which is what the scanner
// must do to survive a future blob regen that renumbers absolute symbol
// IDs without touching the externals list order.
var earthfileDefaultSymTable = [earthfileTokenCount]gotreesitter.Symbol{
	151, // _indent
	152, // _dedent
}

// earthfileExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (earthfileTok* order).
var earthfileExternalScannerSpec = ExternalScannerSpec{
	Language:       "earthfile",
	UpstreamRepo:   "https://github.com/glehmann/tree-sitter-earthfile",
	UpstreamCommit: "5baef88717ad0156fd29a8b12d0d8245bb1096a8",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "67dd8d2a3e668a63d08e20a66c542c8608b9637f61ebd01e638c6584fe153619"},
		{Path: "src/scanner.c", SHA256: "a3e0c97033ce74e62aa89e9aba8cb384bbb0550aaf3a2cb29afaadc0c7d3578f"},
	},
	Externals: []string{
		"_indent",
		"_dedent",
	},
}

func init() {
	RegisterExternalScannerSpec(earthfileExternalScannerSpec)
}

// earthfileState tracks indent level for Earthfile parsing.
type earthfileState struct {
	prevIndent uint32
	hasSeenEof bool
}

// EarthfileExternalScanner handles indent/dedent for Earthfile.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type EarthfileExternalScanner struct {
	symbols         [earthfileTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers earthfile's external symbols.
func (EarthfileExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := EarthfileExternalScanner{symbols: earthfileDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, earthfileExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s EarthfileExternalScanner) symbolTable() *[earthfileTokenCount]gotreesitter.Symbol {
	if s.symbols == ([earthfileTokenCount]gotreesitter.Symbol{}) {
		return &earthfileDefaultSymTable
	}
	return &s.symbols
}

func (EarthfileExternalScanner) Create() any         { return &earthfileState{} }
func (EarthfileExternalScanner) Destroy(payload any) {}

func (EarthfileExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*earthfileState)
	if len(buf) < 5 {
		return 0
	}
	buf[0] = byte(s.prevIndent)
	buf[1] = byte(s.prevIndent >> 8)
	buf[2] = byte(s.prevIndent >> 16)
	buf[3] = byte(s.prevIndent >> 24)
	if s.hasSeenEof {
		buf[4] = 1
	} else {
		buf[4] = 0
	}
	return 5
}

func (EarthfileExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*earthfileState)
	s.prevIndent = 0
	s.hasSeenEof = false
	if len(buf) >= 5 {
		s.prevIndent = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
		s.hasSeenEof = buf[4] != 0
	}
}

func (sc EarthfileExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*earthfileState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [earthfileTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < earthfileTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	// EOF acts as dedent
	if lexer.Lookahead() == 0 {
		return earthfileHandleEof(lexer, s, validSymbols, syms[earthfileTokDedent])
	}

	if earthfileValid(validSymbols, earthfileTokIndent) || earthfileValid(validSymbols, earthfileTokDedent) {
		// Skip whitespace
		for lexer.Lookahead() != 0 && earthfileIsSpace(lexer.Lookahead()) {
			switch lexer.Lookahead() {
			case '\n', '\r', '\f':
				lexer.Advance(false)
			case '\t', ' ':
				lexer.Advance(true)
			}
		}

		if lexer.Lookahead() == 0 {
			return earthfileHandleEof(lexer, s, validSymbols, syms[earthfileTokDedent])
		}

		indent := lexer.Column()
		if indent > s.prevIndent && earthfileValid(validSymbols, earthfileTokIndent) && s.prevIndent == 0 {
			lexer.SetResultSymbol(syms[earthfileTokIndent])
			s.prevIndent = indent
			return true
		}
		if indent < s.prevIndent && earthfileValid(validSymbols, earthfileTokDedent) && indent == 0 {
			lexer.SetResultSymbol(syms[earthfileTokDedent])
			s.prevIndent = indent
			return true
		}
	}

	return false
}

func earthfileHandleEof(lexer *gotreesitter.ExternalLexer, s *earthfileState, validSymbols []bool, dedentSym gotreesitter.Symbol) bool {
	if s.hasSeenEof {
		return false
	}
	lexer.MarkEnd()
	if earthfileValid(validSymbols, earthfileTokDedent) && s.prevIndent != 0 {
		lexer.SetResultSymbol(dedentSym)
		s.hasSeenEof = true
		return true
	}
	return false
}

func earthfileIsSpace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
}

func earthfileValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
