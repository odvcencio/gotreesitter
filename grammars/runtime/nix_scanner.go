//go:build !grammar_subset || grammar_subset_nix

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the nix grammar.
const (
	nixTokStringFragment         = 0
	nixTokIndentedStringFragment = 1
	nixTokPathStart              = 2
	nixTokPathFragment           = 3
	nixTokDollarEscape           = 4
	nixTokIndentedDollarEscape   = 5
)

const (
	nixSymStringFragment         gotreesitter.Symbol = 56
	nixSymIndentedStringFragment gotreesitter.Symbol = 57
	nixSymPathStart              gotreesitter.Symbol = 58 //nolint: unused
	nixSymPathFragment           gotreesitter.Symbol = 59
	nixSymDollarEscape           gotreesitter.Symbol = 60
	nixSymIndentedDollarEscape   gotreesitter.Symbol = 61
)

// NixExternalScanner handles string fragments, path literals, and dollar
// escapes for the Nix expression language.
type NixExternalScanner struct{}

func (NixExternalScanner) Create() any                           { return nil }
func (NixExternalScanner) Destroy(payload any)                   {}
func (NixExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (NixExternalScanner) Deserialize(payload any, buf []byte)   {}

// String and path modes come exclusively from validSymbols; no mode or
// delimiter survives a Scan call. Failed scans therefore preserve empty state.
func (NixExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (NixExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (NixExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (NixExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// Error recovery: all valid
	if nixValid(validSymbols, nixTokStringFragment) &&
		nixValid(validSymbols, nixTokIndentedStringFragment) &&
		nixValid(validSymbols, nixTokPathStart) &&
		nixValid(validSymbols, nixTokPathFragment) &&
		nixValid(validSymbols, nixTokDollarEscape) &&
		nixValid(validSymbols, nixTokIndentedDollarEscape) {
		return false
	}

	if nixValid(validSymbols, nixTokStringFragment) {
		if lexer.Lookahead() == '\\' {
			return nixScanDollarEscape(lexer)
		}
		return nixScanStringFragment(lexer)
	}

	if nixValid(validSymbols, nixTokIndentedStringFragment) {
		if lexer.Lookahead() == '\'' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '\'' {
				return nixScanIndentedDollarEscape(lexer)
			}
		}
		return nixScanIndentedStringFragment(lexer)
	}

	if nixValid(validSymbols, nixTokPathFragment) && nixIsPathChar(lexer.Lookahead()) {
		return nixScanPathFragment(lexer)
	}

	if nixValid(validSymbols, nixTokPathStart) {
		return nixScanPathStart(lexer)
	}

	return false
}

func nixScanDollarEscape(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nixSymDollarEscape)
	lexer.Advance(false)
	lexer.MarkEnd()
	return lexer.Lookahead() == '$'
}

func nixScanIndentedDollarEscape(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nixSymIndentedDollarEscape)
	lexer.Advance(false)
	lexer.MarkEnd()
	if lexer.Lookahead() == '$' {
		return true
	}
	if lexer.Lookahead() == '\\' {
		lexer.Advance(false)
		if lexer.Lookahead() == '$' {
			lexer.MarkEnd()
			return true
		}
	}
	return false
}

func nixScanStringFragment(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nixSymStringFragment)
	hasContent := false
	for {
		lexer.MarkEnd()
		switch lexer.Lookahead() {
		case '"', '\\':
			return hasContent
		case '$':
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				return hasContent
			}
			if lexer.Lookahead() != '"' && lexer.Lookahead() != '\\' {
				lexer.Advance(false)
			}
		case 0:
			return false
		default:
			lexer.Advance(false)
		}
		hasContent = true
	}
}

func nixScanIndentedStringFragment(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nixSymIndentedStringFragment)
	hasContent := false
	for {
		lexer.MarkEnd()
		switch lexer.Lookahead() {
		case '$':
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				return hasContent
			}
			if lexer.Lookahead() != '\'' {
				lexer.Advance(false)
			}
		case '\'':
			lexer.Advance(false)
			if lexer.Lookahead() == '\'' {
				return hasContent
			}
		case 0:
			return false
		default:
			lexer.Advance(false)
		}
		hasContent = true
	}
}

func nixIsPathChar(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') || ch == '-' || ch == '+' ||
		ch == '_' || ch == '.' || ch == '/'
}

func nixScanPathStart(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nixSymPathFragment) // path_start uses the same result sym=58
	// Actually, let me use the correct symbol
	lexer.SetResultSymbol(58) // nixSymPathStart

	// Skip leading whitespace
	for {
		ch := lexer.Lookahead()
		if ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' {
			lexer.Advance(true)
		} else {
			break
		}
	}

	haveSep := false
	haveAfterSep := false
	for {
		lexer.MarkEnd()
		ch := lexer.Lookahead()
		if ch == '/' {
			haveSep = true
		} else if nixIsPathChar(ch) {
			if haveSep {
				haveAfterSep = true
			}
		} else if ch == '$' {
			return haveSep
		} else {
			return haveAfterSep
		}
		lexer.Advance(false)
	}
}

func nixScanPathFragment(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(nixSymPathFragment)
	hasContent := false
	for {
		lexer.MarkEnd()
		if !nixIsPathChar(lexer.Lookahead()) {
			return hasContent
		}
		lexer.Advance(false)
		hasContent = true
	}
}

func nixValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
