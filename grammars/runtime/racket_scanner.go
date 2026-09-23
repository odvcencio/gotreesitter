//go:build !grammar_subset || grammar_subset_racket

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the racket grammar.
const (
	racketTokHereStringBody = 0
	racketTokenCount        = 1
)

// racketDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped racket.bin assigns to each external, in racketTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var racketDefaultSymTable = [racketTokenCount]gotreesitter.Symbol{
	47, // _here_string_body
}

// racketExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (racketTok* order).
var racketExternalScannerSpec = ExternalScannerSpec{
	Language:       "racket",
	UpstreamRepo:   "https://github.com/6cdh/tree-sitter-racket",
	UpstreamCommit: "56b57807f86aa4ddb14892572b318edd4bc90ebe",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "760d6e8ef481ebd2b36f120ff73dd84fc62744ef233392df1dc303333f285997"},
		{Path: "src/scanner.c", SHA256: "7285b6a4175ff0a58db1c97ed4f3756fcef8fc1f3d24f5c1a5a466ee27ea4a2a"},
	},
	Externals: []string{
		"_here_string_body",
	},
}

func init() {
	RegisterExternalScannerSpec(racketExternalScannerSpec)
}

// RacketExternalScanner handles #<<DELIM here-strings for Racket.
// The scanner reads the terminator from the first line, then consumes
// subsequent lines until it finds one matching the terminator exactly.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type RacketExternalScanner struct {
	symbols         [racketTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers racket's external symbols.
func (RacketExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := RacketExternalScanner{symbols: racketDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, racketExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s RacketExternalScanner) symbolTable() *[racketTokenCount]gotreesitter.Symbol {
	if s.symbols == ([racketTokenCount]gotreesitter.Symbol{}) {
		return &racketDefaultSymTable
	}
	return &s.symbols
}

func (RacketExternalScanner) Create() any                           { return nil }
func (RacketExternalScanner) Destroy(payload any)                   {}
func (RacketExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (RacketExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (RacketExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (RacketExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (RacketExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc RacketExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [racketTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < racketTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if !racketValid(validSymbols, racketTokHereStringBody) {
		return false
	}

	// Read terminator (rest of current line)
	var terminator []rune
	for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
		terminator = append(terminator, lexer.Lookahead())
		lexer.Advance(false)
	}
	if lexer.Lookahead() == 0 {
		return false
	}
	// Skip the newline
	lexer.Advance(true)

	// Read lines until we find one matching terminator
	for {
		var line []rune
		for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
			line = append(line, lexer.Lookahead())
			lexer.Advance(false)
		}
		if racketRunesEqual(terminator, line) {
			lexer.SetResultSymbol(syms[racketTokHereStringBody])
			return true
		}
		if lexer.Lookahead() == 0 {
			return false
		}
		// Skip newline
		lexer.Advance(true)
	}
}

func racketRunesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func racketValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
