// Package lexpadding proves when a shared external lookahead is padding for
// another parse version's internal lexer.
package lexpadding

// SharedExternalSkipped requires an actual DFA skip over the entire external
// token, a stateless scanner, and whitespace containing a newline. Horizontal
// whitespace alone may be a meaningful external concatenation token. A version may
// then wait for the next shared lookahead without attaching an error node.
func SharedExternalSkipped(source []byte, start, end, skippedStart, skippedEnd uint32, external, skipped, stateless bool) bool {
	if !external || !skipped || !stateless || start >= end || int(end) > len(source) || skippedStart != start || skippedEnd < end {
		return false
	}
	lineBreak := false
	for _, b := range source[start:end] {
		switch b {
		case '\n':
			lineBreak = true
		case ' ', '\t', '\r', '\f':
		default:
			return false
		}
	}
	return lineBreak
}

// Whitespace checks the gap to a continuation token. Lexer.Next may skip
// invalid bytes while searching, so its result alone cannot prove this gap.
func Whitespace(source []byte, start, end uint32) bool {
	if start > end || uint64(end) > uint64(len(source)) {
		return false
	}
	for _, b := range source[start:end] {
		switch b {
		case ' ', '\t', '\n', '\r', '\f':
		default:
			return false
		}
	}
	return true
}
