//go:build !grammar_subset || grammar_subset_scss

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the scss grammar (must match grammar.js externals).
const (
	scssTokDescendantOp  = 0 // "_descendant_operator"
	scssTokColon         = 1 // "_pseudo_class_selector_colon"
	scssTokErrorRecovery = 2 // "__error_recovery"
	scssTokConcat        = 3 // "_concat"
)

// ScssExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-scss.
//
// Ported from tree-sitter-scss/src/scanner.c (pinned commit bca847c).
// The scanner handles four external tokens:
//   - _descendant_operator: whitespace between two selectors (e.g., "div p")
//   - _pseudo_class_selector_colon: colon in selector context (e.g., ":hover")
//   - __error_recovery: sentinel — always declined
//   - _concat: adjacent selector concatenation (e.g., "#foo.bar")
type ScssExternalScanner struct{}

// scssScanState caches the (singleton) Language pointer for the lifetime of one
// parse's scanner session. Scan() previously called ScssLanguage() on every
// external-token scan purely to index ExternalSymbols; that loader call takes a
// global lock and reads time.Now() for LRU bookkeeping, which showed up as ~4%
// of scss forest parse. The Language is a stable singleton, so resolving it once
// per parse is behavior-identical.
type scssScanState struct{ lang *gotreesitter.Language }

func (ScssExternalScanner) Create() any                           { return &scssScanState{} }
func (ScssExternalScanner) Destroy(payload any)                   {}
func (ScssExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (ScssExternalScanner) Deserialize(payload any, buf []byte)   {}
func (ScssExternalScanner) SupportsIncrementalReuse() bool        { return true }

// The payload caches fixed grammar identity, not token history. Scan reads only
// forward source bytes and the valid-symbol set. Serialization remains empty.
func (ScssExternalScanner) ExternalScannerIsStateless() bool { return true }

func (ScssExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	lang := scssScanLang(payload)

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
			lexer.SetResultSymbol(scssResolve(lang, scssTokConcat))
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
		lexer.SetResultSymbol(scssResolve(lang, scssTokDescendantOp))

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
					lexer.SetResultSymbol(scssResolve(lang, scssTokColon))
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

// scssScanLang returns the scss Language, cached per parse in the scanner
// payload so the hot Scan path does not re-enter the locking, LRU-tracking
// embedded-language loader on every external token. Falls back to a direct
// load if the payload is missing or of an unexpected type.
func scssScanLang(payload any) *gotreesitter.Language {
	st, ok := payload.(*scssScanState)
	if !ok {
		return ScssLanguage()
	}
	if st.lang == nil {
		st.lang = ScssLanguage()
	}
	return st.lang
}

// scssResolve maps external token index to runtime symbol using the language's ExternalSymbols.
func scssResolve(lang *gotreesitter.Language, tokIdx int) gotreesitter.Symbol {
	if lang != nil {
		ext := lang.ExternalSymbols
		if tokIdx < len(ext) {
			return ext[tokIdx]
		}
	}
	// Fallback to hardcoded values (legacy).
	switch tokIdx {
	case scssTokDescendantOp:
		return 85
	case scssTokColon:
		return 86
	case scssTokErrorRecovery:
		return 87
	case scssTokConcat:
		return 88
	}
	return 0
}
