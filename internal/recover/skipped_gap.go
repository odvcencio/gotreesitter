// Package recover implements shared recovery decisions.
package recover

// SingleTokenGap recognizes a skipped gap only when error-mode lexing on the
// complete input finds one eligible token ending at the original lookahead.
// Lexing a sliced gap would incorrectly accept prefixes of longer tokens.
// The caller supplies the lexer and keeps scanner and stack state unchanged.
func SingleTokenGap[T any](start, end uint32, lex func() (T, uint32, uint32, bool)) (T, bool) {
	var zero T
	if start >= end {
		return zero, false
	}
	token, tokenStart, tokenEnd, eligible := lex()
	if !eligible || tokenStart < start || tokenStart >= tokenEnd || tokenEnd != end {
		return zero, false
	}
	return token, true
}
