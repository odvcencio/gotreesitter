//go:build !grammar_subset || grammar_subset_scss

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the scss grammar (must match grammar.js
// externals). This is the external index (the position of the token in the
// grammar's `externals: [...]` list), which is exactly what tree-sitter's
// `valid_symbols` array and C's result_symbol enum are indexed by. The
// external index is stable across a blob regen as long as the externals
// list itself does not reorder; concrete numeric gotreesitter.Symbol IDs
// are NOT stable (they shift whenever the grammar's total symbol count
// changes), so this scanner never hardcodes them -- see
// scssDefaultSymTable below.
const (
	scssTokDescendantOp  = 0 // "_descendant_operator"
	scssTokColon         = 1 // "_pseudo_class_selector_colon"
	scssTokErrorRecovery = 2 // "__error_recovery"
	scssTokConcat        = 3 // "_concat"
	scssTokenCount       = 4
)

// scssDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped scss.bin assigns to each external, in scssTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order. _pseudo_class_selector_colon displays as the literal ":".
var scssDefaultSymTable = [scssTokenCount]gotreesitter.Symbol{
	85, // _descendant_operator
	86, // _pseudo_class_selector_colon (display: ":")
	87, // __error_recovery
	88, // _concat
}

// scssExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (scssTok* order).
var scssExternalScannerSpec = ExternalScannerSpec{
	Language:       "scss",
	UpstreamRepo:   "https://github.com/tree-sitter-grammars/tree-sitter-scss",
	UpstreamCommit: "2ef6d42e3ad7a8208900f9346f4529806ae0f9f9",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "cac3446a268374e6f05efb81b08828f2e69693c514363eea05c4a4343865e49f"},
		{Path: "src/scanner.c", SHA256: "161546c39369051c9cb6bfa374d62be90aaaa8c673bca68c4a06b2524706e7db"},
	},
	Externals: []string{
		"_descendant_operator",
		"_pseudo_class_selector_colon",
		"__error_recovery",
		"_concat",
	},
}

func init() {
	RegisterExternalScannerSpec(scssExternalScannerSpec)
}

// ScssExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-scss.
//
// Ported from tree-sitter-scss/src/scanner.c (pinned commit bca847c).
// The scanner handles four external tokens:
//   - _descendant_operator: whitespace between two selectors (e.g., "div p")
//   - _pseudo_class_selector_colon: colon in selector context (e.g., ":hover")
//   - __error_recovery: sentinel — always declined
//   - _concat: adjacent selector concatenation (e.g., "#foo.bar")
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type ScssExternalScanner struct {
	symbols         [scssTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers scss's external symbols. This also removes the
// prior per-parse loadEmbeddedLanguage("scss.bin") lookup entirely: the
// scanner now resolves its symbols once, at bind time, instead of caching a
// Language pointer in the scan payload and indexing ExternalSymbols on
// every Scan call.
func (ScssExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := ScssExternalScanner{symbols: scssDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, scssExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s ScssExternalScanner) symbolTable() *[scssTokenCount]gotreesitter.Symbol {
	if s.symbols == ([scssTokenCount]gotreesitter.Symbol{}) {
		return &scssDefaultSymTable
	}
	return &s.symbols
}

func (ScssExternalScanner) Create() any                           { return nil }
func (ScssExternalScanner) Destroy(payload any)                   {}
func (ScssExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (ScssExternalScanner) Deserialize(payload any, buf []byte)   {}
func (ScssExternalScanner) SupportsIncrementalReuse() bool        { return true }

// The payload caches fixed grammar identity, not token history. Scan reads only
// forward source bytes and the valid-symbol set. Serialization remains empty.
func (ScssExternalScanner) ExternalScannerIsStateless() bool { return true }

func (s ScssExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [scssTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < scssTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	// When error recovery is active, all valid_symbols are true including
	// ERROR_RECOVERY. Bail out immediately to avoid producing spurious tokens.
	if scssValid(validSymbols, scssTokErrorRecovery) {
		return false
	}

	// CONCAT: adjacent selector concatenation (e.g., #foo.bar).
	// Must be checked before DESCENDANT_OP because both can start with alnum.
	if scssValid(validSymbols, scssTokConcat) {
		ch := lexer.Lookahead()
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '#' || ch == '-' {
			lexer.SetResultSymbol(syms[scssTokConcat])
			if ch == '#' {
				lexer.MarkEnd()
				lexer.Advance(false)
				return lexer.Lookahead() == '{'
			}
			return true
		}
	}

	// DESCENDANT_OP: whitespace between selectors.
	if isScssSpace(lexer.Lookahead()) && scssValid(validSymbols, scssTokDescendantOp) {
		lexer.SetResultSymbol(syms[scssTokDescendantOp])

		// Skip all whitespace.
		lexer.Advance(true)
		for isScssSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		lexer.MarkEnd()

		ch := lexer.Lookahead()
		// These characters indicate a selector follows.
		if ch == '#' || ch == '.' || ch == '[' || ch == '-' ||
			ch == '*' || ch == '&' || unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			return true
		}

		// If ':' follows, disambiguate: pseudo-class (selector context) vs
		// property-value separator. Scan forward — if we hit '{' before ';' or '}',
		// it's a selector context.
		if ch == ':' {
			lexer.Advance(false)
			if isScssSpace(lexer.Lookahead()) {
				return false
			}
			for {
				ch = lexer.Lookahead()
				if ch == ';' || ch == '}' || ch == 0 {
					return false
				}
				if ch == '{' {
					return true
				}
				lexer.Advance(false)
			}
		}
	}

	// PSEUDO_CLASS_SELECTOR_COLON: disambiguate ':' in selector vs property.
	if scssValid(validSymbols, scssTokColon) {
		// Skip leading whitespace.
		for isScssSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == ':' {
			lexer.Advance(false)
			if lexer.Lookahead() == ':' {
				// '::' is a pseudo-element, not handled here.
				return false
			}
			lexer.MarkEnd()
			// Scan forward: '{' means selector context, ';' or '}' means property.
			for lexer.Lookahead() != ';' && lexer.Lookahead() != '}' && lexer.Lookahead() != 0 {
				lexer.Advance(false)
				if lexer.Lookahead() == '{' {
					lexer.SetResultSymbol(syms[scssTokColon])
					return true
				}
			}
			return false
		}
	}

	return false
}

func isScssSpace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
}

func scssValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
