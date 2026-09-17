//go:build !grammar_subset || grammar_subset_erlang

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the erlang grammar.
const (
	erlangTokTQString      = 0 // "_tq_string" — triple-quoted string
	erlangTokTQSigilString = 1 // "_tq_sigil_string" — triple-quoted sigil string (~s""")
	erlangTokErrorSentinel = 2 // "error_sentinel"
)

// Concrete symbol IDs from the generated erlang grammar ExternalSymbols.
const (
	erlangSymTQString      gotreesitter.Symbol = 138
	erlangSymTQSigilString gotreesitter.Symbol = 139
	erlangSymErrorSentinel gotreesitter.Symbol = 140
)

// ErlangExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-erlang.
//
// This is a Go port of the C external scanner from tree-sitter-erlang
// (WhatsApp/tree-sitter-erlang). The scanner handles Erlang's triple-quoted
// strings (EEP-0064): strings delimited by 3+ quote characters, where the
// closing delimiter must appear at the start of a line (after optional
// whitespace) and match the same number of quotes. Sigil strings optionally
// have a ~[sSbB]? prefix.
type ErlangExternalScanner struct{}

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

func (ErlangExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
					lexer.SetResultSymbol(erlangSymTQSigilString)
				} else {
					lexer.SetResultSymbol(erlangSymTQString)
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
