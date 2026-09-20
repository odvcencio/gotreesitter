//go:build !grammar_subset || grammar_subset_erlang

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the erlang grammar. These are external indices
// (the position of each token in upstream's grammar.json "externals" list),
// which is exactly what tree-sitter's valid_symbols array and C's
// result_symbol enum are indexed by. External indices are stable across a
// blob regen as long as the externals list itself does not reorder; concrete
// numeric gotreesitter.Symbol IDs are NOT stable (they shift whenever the
// grammar's total symbol count changes), so this scanner never hardcodes
// them -- see the symbols field on ErlangExternalScanner below.
const (
	erlangTokTQString      = 0 // "_tq_string" — triple-quoted string
	erlangTokTQSigilString = 1 // "_tq_sigil_string" — triple-quoted sigil string (~s""")
	erlangTokErrorSentinel = 2 // "error_sentinel"
	erlangTokenCount       = 3
)

// erlangDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped erlang.bin assigns to each external index, in erlangTok*
// order. It exists only as a pre-bind fallback; ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that shifts the grammar's absolute symbol numbering without
// touching the externals list order.
var erlangDefaultSymTable = [erlangTokenCount]gotreesitter.Symbol{
	145, // _tq_string
	146, // _tq_sigil_string
	147, // error_sentinel
}

// erlangExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its token
// list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (erlangTok* order).
var erlangExternalScannerSpec = ExternalScannerSpec{
	Language:       "erlang",
	UpstreamRepo:   "https://github.com/WhatsApp/tree-sitter-erlang",
	UpstreamCommit: "6ba4c762eb3065495e3db85697ffeecdf364ce35",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "a225ad78f04d9f50b51c3907e0daece60c63e7b5992453cca2bb36d8fa323f16"},
		{Path: "src/scanner.c", SHA256: "bcb05457c981783245db637ec98a0b47f18e0acfe939efdd69a9da3e26f1b8a6"},
	},
	Externals: []string{
		"_tq_string",
		"_tq_sigil_string",
		"error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(erlangExternalScannerSpec)
}

// ErlangExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-erlang.
//
// This is a Go port of the C external scanner from tree-sitter-erlang
// (WhatsApp/tree-sitter-erlang). The scanner handles Erlang's triple-quoted
// strings (EEP-0064): strings delimited by 3+ quote characters, where the
// closing delimiter must appear at the start of a line (after optional
// whitespace) and match the same number of quotes. Sigil strings optionally
// have a ~[sSbB]? prefix.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type ErlangExternalScanner struct {
	symbols         [erlangTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token indices to lang's
// concrete external symbol IDs, positionally, via erlangExternalScannerSpec.
func (ErlangExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := ErlangExternalScanner{symbols: erlangDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, erlangExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (ErlangExternalScanner) Create() any                           { return nil }
func (ErlangExternalScanner) Destroy(payload any)                   {}
func (ErlangExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (ErlangExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (ErlangExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (ErlangExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (ErlangExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (s ErlangExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [erlangTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < erlangTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	if !erlangValid(validSymbols, erlangTokTQString) && !erlangValid(validSymbols, erlangTokTQSigilString) {
		return false
	}

	// Skip leading whitespace.
	for isErlangWhitespace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Check for optional sigil prefix: ~[sSbB]?
	isSigilString := false
	if erlangValid(validSymbols, erlangTokTQSigilString) && lexer.Lookahead() == '~' {
		isSigilString = true
		lexer.Advance(false)
		switch lexer.Lookahead() {
		case 's', 'S', 'b', 'B':
			lexer.Advance(false)
		case '"':
			// No modifier — proceed directly to quotes.
		default:
			return false
		}
	}

	// Count opening quotes: need at least 3.
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)

	delimiterCount := uint16(3)
	for lexer.Lookahead() == '"' {
		delimiterCount++
		lexer.Advance(false)
	}

	// Skip whitespace to end of opening line, then expect newline.
	for lexer.Lookahead() != '\n' && isErlangWhitespace(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '\n' {
		return false
	}
	lexer.Advance(false)

	// Scan body: look for a line that starts with optional whitespace
	// followed by exactly delimiterCount quotes.
	for {
		ch := lexer.Lookahead()
		if ch == '\n' {
			lexer.Advance(false)
			// At start of new line — skip whitespace and check for closing delimiter.
			for lexer.Lookahead() != '\n' && isErlangWhitespace(lexer.Lookahead()) {
				lexer.Advance(false)
			}
			remaining := delimiterCount
			for remaining > 0 {
				if lexer.Lookahead() != '"' {
					break
				}
				lexer.Advance(false)
				remaining--
			}
			if remaining == 0 {
				lexer.MarkEnd()
				if isSigilString {
					lexer.SetResultSymbol(syms[erlangTokTQSigilString])
				} else {
					lexer.SetResultSymbol(syms[erlangTokTQString])
				}
				return true
			}
		} else if ch == 0 { // EOF
			return false
		} else {
			lexer.Advance(false)
		}
	}
}

func (s ErlangExternalScanner) symbolTable() *[erlangTokenCount]gotreesitter.Symbol {
	if s.symbols == ([erlangTokenCount]gotreesitter.Symbol{}) {
		return &erlangDefaultSymTable
	}
	return &s.symbols
}

// isErlangWhitespace matches the C scanner's whitespace range: 0x01-0x20 and 0x80-0xA0,
// excluding newline (which is handled separately).
func isErlangWhitespace(ch rune) bool {
	if ch == '\n' {
		return false
	}
	return (ch >= 0x01 && ch <= 0x20) || (ch >= 0x80 && ch <= 0xA0)
}

func erlangValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
