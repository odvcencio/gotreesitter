//go:build !grammar_subset || grammar_subset_css

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the CSS grammar.
const (
	cssTokDescendantOp  = 0 // "_descendant_operator"
	cssTokColon         = 1 // ":" — pseudo-class selector colon
	cssTokErrorRecovery = 2 // "__error_recovery"
)

// Concrete symbol IDs from the generated CSS grammar ExternalSymbols.
const (
	cssSymDescendantOp  gotreesitter.Symbol = 72
	cssSymColon         gotreesitter.Symbol = 73
	cssSymErrorRecovery gotreesitter.Symbol = 74
)

// CssExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-css.
//
// This is a Go port of the C external scanner from tree-sitter-css. The
// scanner handles three tokens:
//   - _descendant_operator: whitespace between two selectors (descendant combinator)
//   - pseudo_class_selector_colon: a ":" that starts a pseudo-class (vs property-value separator)
//   - __error_recovery: sentinel that causes immediate bail-out
//
// The key challenge is contextual disambiguation: whitespace might be a
// descendant combinator or just formatting, and ":" might start a pseudo-class
// or separate a property from its value.
type CssExternalScanner struct{}

func (CssExternalScanner) Create() any                           { return nil }
func (CssExternalScanner) Destroy(payload any)                   {}
func (CssExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (CssExternalScanner) Deserialize(payload any, buf []byte)   {}
func (CssExternalScanner) SupportsIncrementalReuse() bool        { return true }

// Scan reads only forward source bytes and the valid-symbol set. It stores no state.
func (CssExternalScanner) ExternalScannerIsStateless() bool { return true }

func (CssExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
			lexer.SetResultSymbol(cssSymDescendantOp)
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
					lexer.SetResultSymbol(cssSymDescendantOp)
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
					lexer.SetResultSymbol(cssSymColon)
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
				lexer.SetResultSymbol(cssSymColon)
				return true
			}
			return false
		}
	}

	return false
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
