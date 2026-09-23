//go:build !grammar_subset || grammar_subset_godot_resource

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the godot_resource grammar. This is the
// external index (the position of the token in the grammar's
// `externals: [...]` list), which is exactly what tree-sitter's
// `valid_symbols` array and C's result_symbol enum are indexed by. The
// external index is stable across a blob regen as long as the externals
// list itself does not reorder; concrete numeric gotreesitter.Symbol IDs
// are NOT stable (they shift whenever the grammar's total symbol count
// changes), so this scanner never hardcodes them -- see
// godotResourceDefaultSymTable below.
const (
	godotResourceTokString  = 0
	godotResourceTokenCount = 1
)

// godotResourceDefaultSymTable records the concrete gotreesitter.Symbol
// IDs the currently shipped godot_resource.bin assigns to each external,
// in godotResourceTok* order. It exists only as a pre-bind fallback (and
// as an independent value to compare a real bind against in tests);
// ExternalScannerForLanguage below overwrites it with values read from
// the actual loaded Language at bind time, which is what the scanner
// must do to survive a future blob regen that renumbers absolute symbol
// IDs without touching the externals list order.
var godotResourceDefaultSymTable = [godotResourceTokenCount]gotreesitter.Symbol{
	19, // string
}

// godotResourceExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (godotResourceTok* order).
var godotResourceExternalScannerSpec = ExternalScannerSpec{
	Language:       "godot_resource",
	UpstreamRepo:   "https://github.com/PrestonKnopp/tree-sitter-godot-resource",
	UpstreamCommit: "302c1895f54bf74d53a08572f7b26a6614209adc",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "931c768e4b5b3a5fe3cf320f6a73d807715ea7d2d9fe54307008dfc20ca71753"},
		{Path: "src/scanner.c", SHA256: "138f36cd90bf5bf61042e48368ac85e9fffc9be0767bf1f8c38bc86d87ce8334"},
	},
	Externals: []string{
		"string",
	},
}

func init() {
	RegisterExternalScannerSpec(godotResourceExternalScannerSpec)
}

// GodotResourceExternalScanner handles multiline string literals in
// Godot .tres/.tscn resource files. Strings are "..." with \" escapes.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type GodotResourceExternalScanner struct {
	symbols         [godotResourceTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers godot_resource's external symbols.
func (GodotResourceExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := GodotResourceExternalScanner{symbols: godotResourceDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, godotResourceExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s GodotResourceExternalScanner) symbolTable() *[godotResourceTokenCount]gotreesitter.Symbol {
	if s.symbols == ([godotResourceTokenCount]gotreesitter.Symbol{}) {
		return &godotResourceDefaultSymTable
	}
	return &s.symbols
}

func (GodotResourceExternalScanner) Create() any                           { return nil }
func (GodotResourceExternalScanner) Destroy(payload any)                   {}
func (GodotResourceExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (GodotResourceExternalScanner) Deserialize(payload any, buf []byte)   {}
func (GodotResourceExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (GodotResourceExternalScanner) ExternalScannerIsStateless() bool      { return true }

// Scan is a line-faithful port of the pinned upstream src/scanner.c
// (PrestonKnopp/tree-sitter-godot-resource @ 302c1895).
//
// Note the upstream escape handling: a quote terminates the string only when
// the PREVIOUS character was not a backslash. That means `\\"` does NOT
// terminate (the quote follows a backslash) — upstream does not treat `\\`
// as a completed escape pair. Parity requires reproducing that exactly.
func (sc GodotResourceExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [godotResourceTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < godotResourceTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if !godotResourceValid(validSymbols, godotResourceTokString) {
		return false
	}

	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if lexer.Lookahead() != '"' {
		return false
	}

	lastChar := rune('"')
	lexer.Advance(false)

	for lexer.Lookahead() != 0 {
		if lastChar != '\\' && lexer.Lookahead() == '"' {
			lexer.Advance(false)
			lexer.SetResultSymbol(syms[godotResourceTokString])
			return true
		}
		lastChar = lexer.Lookahead()
		lexer.Advance(false)
	}

	return false
}

func godotResourceValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
