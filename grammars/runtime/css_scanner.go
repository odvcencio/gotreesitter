//go:build !grammar_subset || grammar_subset_css

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the CSS grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see cssDefaultSymTable below.
const (
	cssTokDescendantOp  = 0 // _descendant_operator
	cssTokColon         = 1 // _pseudo_class_selector_colon
	cssTokErrorRecovery = 2 // __error_recovery
	cssTokenCount       = 3
)

// cssDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped css.bin assigns to each external, in cssTok* order. It
// exists only as a pre-bind fallback (and as an independent value to compare
// a real bind against in tests); ExternalScannerForLanguage below overwrites
// it with values read from the actual loaded Language at bind time, which is
// what the scanner must do to survive a future blob regen that renumbers
// absolute symbol IDs without touching the externals list order.
var cssDefaultSymTable = [cssTokenCount]gotreesitter.Symbol{
	72, // _descendant_operator
	73, // _pseudo_class_selector_colon
	74, // __error_recovery
}

// cssExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (cssTok* order).
var cssExternalScannerSpec = ExternalScannerSpec{
	Language:       "css",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-css",
	UpstreamCommit: "dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "1a170e0c93009b5b592a611c663d4af4d625ddb97ebff1550c3e7f32fd045292"},
		{Path: "src/scanner.c", SHA256: "18dae8c8c4f515f28a3dc7ffb5bda259b06013a752921dc411a2fad8ecf78988"},
	},
	Externals: []string{
		"_descendant_operator",
		"_pseudo_class_selector_colon",
		"__error_recovery",
	},
}

func init() {
	RegisterExternalScannerSpec(cssExternalScannerSpec)
}

// CssExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-css.
//
// This is a Go port of the C external scanner from tree-sitter-css. The
// scanner handles three tokens:
//   - _descendant_operator: whitespace between two selectors (descendant combinator)
//   - _pseudo_class_selector_colon: a ":" that starts a pseudo-class (vs property-value separator)
//   - __error_recovery: sentinel that causes immediate bail-out
//
// The key challenge is contextual disambiguation: whitespace might be a
// descendant combinator or just formatting, and ":" might start a pseudo-class
// or separate a property from its value.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type CssExternalScanner struct {
	symbols         [cssTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers css's external symbols.
func (CssExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CssExternalScanner{symbols: cssDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, cssExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (CssExternalScanner) Create() any                           { return nil }
func (CssExternalScanner) Destroy(payload any)                   {}
func (CssExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (CssExternalScanner) Deserialize(payload any, buf []byte)   {}
func (CssExternalScanner) SupportsIncrementalReuse() bool        { return true }

// Scan reads only forward source bytes and the valid-symbol set. It stores no state.
func (CssExternalScanner) ExternalScannerIsStateless() bool { return true }

func (s CssExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [cssTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < cssTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	// Error recovery sentinel — always decline.
	if cssValid(validSymbols, cssTokErrorRecovery) {
		return false
	}

	ch := lexer.Lookahead()

	// Descendant operator: whitespace followed by a selector-start character.
	if isCssSpace(ch) && cssValid(validSymbols, cssTokDescendantOp) {
		// Skip all whitespace.
		cssSkip(lexer)
		for isCssSpace(lexer.Lookahead()) {
			cssSkip(lexer)
		}
		lexer.MarkEnd()

		next := lexer.Lookahead()
		// These characters indicate a selector follows.
		if next == '#' || next == '.' || next == '[' || next == '-' || next == '*' ||
			unicode.IsLetter(next) || unicode.IsDigit(next) {
			lexer.SetResultSymbol(syms[cssTokDescendantOp])
			return true
		}

		// Colon after whitespace: could be pseudo-class in selector context.
		// Scan forward to disambiguate.
		if next == ':' {
			lexer.Advance(false)
			if isCssSpace(lexer.Lookahead()) {
				return false
			}
			for {
				c := lexer.Lookahead()
				if c == ';' || c == '}' || c == 0 {
					return false
				}
				if c == '{' {
					lexer.SetResultSymbol(syms[cssTokDescendantOp])
					return true
				}
				lexer.Advance(false)
			}
		}
	}

	// Pseudo-class selector colon: ":" that is NOT "::" (pseudo-element).
	if cssValid(validSymbols, cssTokColon) {
		// Skip leading whitespace.
		for isCssSpace(lexer.Lookahead()) {
			cssSkip(lexer)
		}

		if lexer.Lookahead() == ':' {
			lexer.Advance(false)
			// If the next char is also ':', this is a pseudo-element — decline.
			if lexer.Lookahead() == ':' {
				return false
			}
			lexer.MarkEnd()

			// Scan forward to confirm this is a selector context.
			// If we hit '{' (rule block), it's a pseudo-class.
			// If we hit ';' or '}' (end of declaration), it's not.
			inComment := false
			for {
				c := lexer.Lookahead()
				if c == ';' || c == '}' || c == 0 {
					break
				}
				lexer.Advance(false)
				if c == '{' && !inComment {
					lexer.SetResultSymbol(syms[cssTokColon])
					return true
				}
				if c == '/' && !inComment {
					if lexer.Lookahead() == '*' {
						inComment = true
					}
				} else if c == '*' && inComment {
					if lexer.Lookahead() == '/' {
						inComment = false
					}
				}
			}
			// Reached EOF — treat as valid (matches C behavior).
			if lexer.Lookahead() == 0 {
				lexer.SetResultSymbol(syms[cssTokColon])
				return true
			}
			return false
		}
	}

	return false
}

func (s CssExternalScanner) symbolTable() *[cssTokenCount]gotreesitter.Symbol {
	if s.symbols == ([cssTokenCount]gotreesitter.Symbol{}) {
		return &cssDefaultSymTable
	}
	return &s.symbols
}

func isCssSpace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
}

// cssSkip advances the lexer in skip mode (excluded from token span).
func cssSkip(lexer *gotreesitter.ExternalLexer) {
	lexer.Advance(true)
}

func cssValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
