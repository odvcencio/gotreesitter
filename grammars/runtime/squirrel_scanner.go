//go:build !grammar_subset || grammar_subset_squirrel

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the squirrel grammar.
const (
	squirrelTokVerbatimString = 0
	squirrelTokenCount        = 1
)

// squirrelDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped squirrel.bin assigns to each external, in squirrelTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var squirrelDefaultSymTable = [squirrelTokenCount]gotreesitter.Symbol{
	95, // verbatim_string
}

// squirrelExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (squirrelTok* order).
var squirrelExternalScannerSpec = ExternalScannerSpec{
	Language:       "squirrel",
	UpstreamRepo:   "https://github.com/amaanq/tree-sitter-squirrel",
	UpstreamCommit: "072c969749e66f000dba35a33c387650e203e96e",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "fc715c5f9f2c3448e8899fe5935970e9d72f0d2874be83c121cdfc90bfcc6a17"},
		{Path: "src/scanner.c", SHA256: "264a47e601127b06ea0a6eec202fe7c43c80ceef033b07c881a4e836e9028908"},
	},
	Externals: []string{
		"verbatim_string",
	},
}

func init() {
	RegisterExternalScannerSpec(squirrelExternalScannerSpec)
}

// SquirrelExternalScanner handles @"..." verbatim string literals for Squirrel.
// Inside a verbatim string, "" is an escaped double-quote.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type SquirrelExternalScanner struct {
	symbols         [squirrelTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers squirrel's external symbols.
func (SquirrelExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := SquirrelExternalScanner{symbols: squirrelDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, squirrelExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s SquirrelExternalScanner) symbolTable() *[squirrelTokenCount]gotreesitter.Symbol {
	if s.symbols == ([squirrelTokenCount]gotreesitter.Symbol{}) {
		return &squirrelDefaultSymTable
	}
	return &s.symbols
}

func (SquirrelExternalScanner) Create() any                           { return nil }
func (SquirrelExternalScanner) Destroy(payload any)                   {}
func (SquirrelExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (SquirrelExternalScanner) Deserialize(payload any, buf []byte)   {}

// Verbatim-string scanning is self-contained in one token and retains no
// payload, including when the closing quote is absent.
func (SquirrelExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (SquirrelExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (SquirrelExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc SquirrelExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [squirrelTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < squirrelTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if !squirrelValid(validSymbols, squirrelTokVerbatimString) {
		return false
	}
	// Expect @"
	if lexer.Lookahead() != '@' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)

	// Scan until unescaped closing "
	for {
		ch := lexer.Lookahead()
		if ch == 0 {
			return false
		}
		if ch == '"' {
			lexer.Advance(false)
			// "" is an escaped quote, continue scanning
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				continue
			}
			// Single " ends the string
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[squirrelTokVerbatimString])
			return true
		}
		lexer.Advance(false)
	}
}

func squirrelValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
