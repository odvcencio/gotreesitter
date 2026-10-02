// Package recover implements shared recovery decisions.
package recover

// SingleTokenGap recognizes a skipped gap only when error-mode lexing on the
// complete input finds one eligible token ending at the original lookahead.
// Lexing a sliced gap would incorrectly accept prefixes of longer tokens.
// The caller supplies that token and keeps scanner and stack state unchanged.
// Any leading bytes must have an independent proof that they are padding.
func SingleTokenGap[T any](start, end uint32, token T, tokenStart, tokenEnd uint32, eligible, prefixIsPadding bool) (T, bool) {
	var zero T
	if start >= end {
		return zero, false
	}
	if !eligible || tokenStart < start || tokenStart >= tokenEnd || tokenEnd != end {
		return zero, false
	}
	if tokenStart > start && !prefixIsPadding {
		return zero, false
	}
	return token, true
}
