//go:build !grammar_subset || grammar_subset_gn

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the gn grammar.
const (
	gnTokStringContent = 0
)

const (
	gnSymStringContent gotreesitter.Symbol = 37
)

// GnExternalScanner handles string content for GN (Generate Ninja) build files.
// Scans content inside "..." strings, stopping at closing quote, escape
// sequences, and ${...} interpolations.
type GnExternalScanner struct{}

func (GnExternalScanner) Create() any                           { return nil }
func (GnExternalScanner) Destroy(payload any)                   {}
func (GnExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (GnExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (GnExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (GnExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (GnExternalScanner) PreservesStateOnScanFailure() bool { return true }

// Scan is a line-faithful port of the pinned upstream src/scanner.c
// (tree-sitter-grammars/tree-sitter-gn @ bc06955b).
func (GnExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !gnValid(validSymbols, gnTokStringContent) {
		return false
	}
	didAdvance := false
	for {
		switch lexer.Lookahead() {
		case 0:
			// Lookahead()==0 covers both lexer->eof() and a literal NUL
			// byte; the C scanner returns false for either.
			return false
		case '\\':
			// The token ends before the backslash only when it begins a
			// recognized escape (\" \$ \\); otherwise both the backslash
			// and the escaped character become string content.
			lexer.MarkEnd()
			lexer.Advance(false)
			la := lexer.Lookahead()
			if la == '"' || la == '$' || la == '\\' {
				lexer.SetResultSymbol(gnSymStringContent)
				return didAdvance
			}
			didAdvance = true
			lexer.Advance(false)
		case '$':
			// The token ends before '$' when it starts an interpolation:
			// ${...}, $identifier ($ followed by a letter or '_').
			// Otherwise '$' and the following character are content.
			lexer.MarkEnd()
			lexer.Advance(false)
			la := lexer.Lookahead()
			if la == '{' || gnIsAlpha(la) || la == '_' {
				lexer.SetResultSymbol(gnSymStringContent)
				return didAdvance
			}
			didAdvance = true
			lexer.Advance(false)
		case '"':
			lexer.MarkEnd()
			lexer.SetResultSymbol(gnSymStringContent)
			return didAdvance
		default:
			didAdvance = true
			lexer.Advance(false)
		}
	}
}

// gnIsAlpha mirrors C isalpha() in the C locale (ASCII letters only).
func gnIsAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func gnValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
