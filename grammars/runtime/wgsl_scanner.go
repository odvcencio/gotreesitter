//go:build !grammar_subset || grammar_subset_wgsl

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the wgsl grammar.
const (
	wgslTokBlockComment = 0
)

const (
	wgslSymBlockComment gotreesitter.Symbol = 135
)

// WgslExternalScanner handles nestable /* */ block comments for WGSL.
type WgslExternalScanner struct{}

func (WgslExternalScanner) Create() any                           { return nil }
func (WgslExternalScanner) Destroy(payload any)                   {}
func (WgslExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (WgslExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (WgslExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (WgslExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (WgslExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (WgslExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !wgslValid(validSymbols, wgslTokBlockComment) {
		return false
	}

	// Mirror C scanner: skip leading whitespace before the comment opener so
	// the external token may be recognized at any position the parser offers
	// it (indented comments, comments following a token + space, a second
	// comment after `/* a */ `, etc.). C advances with skip=true so the
	// skipped whitespace is excluded from the token span.
	for wgslIsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if lexer.Lookahead() != '/' {
		return false
	}
	lexer.Advance(false)

	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)

	// Nestable /* */ comment. Byte-faithful port of the C state machine:
	// on '/' followed by '*' the depth increases; on '*' followed by '/' the
	// depth decreases, and reaching depth 0 emits the token. End-of-input
	// before closing returns false (no token) so an unterminated comment is
	// surfaced as an error, exactly as the C scanner does.
	commentDepth := 1
	for {
		ch := lexer.Lookahead()
		switch {
		case ch == '/':
			lexer.Advance(false)
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				commentDepth++
			}
		case ch == '*':
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				lexer.Advance(false)
				commentDepth--
				if commentDepth == 0 {
					lexer.SetResultSymbol(wgslSymBlockComment)
					return true
				}
			}
		case ch == 0:
			// End of input reached before the comment closed.
			return false
		default:
			lexer.Advance(false)
		}
	}
}

func wgslValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

// wgslIsSpace mirrors C's iswspace for the ASCII whitespace the WGSL scanner
// skips before a block comment opener.
func wgslIsSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}
