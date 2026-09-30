// Package lex contains source-independent lexer rules shared by the engines.
package lex

// KeepRecoveryExternalToken matches the locked C lexer's progress rule.
// Padding counts toward progress even when the token's visible span is empty.
// Without byte progress, recovery keeps an external token only if the scanner
// changed state; otherwise a permissive error-mode scan could loop forever.
func KeepRecoveryExternalToken(scanStart, tokenEnd uint32, stateChanged bool) bool {
	return tokenEnd > scanStart || stateChanged
}

// IsLineEndingToken identifies hidden internal layout from its consumed bytes.
func IsLineEndingToken(source []byte, start, end uint32) bool {
	if start >= end || uint64(end) > uint64(len(source)) {
		return false
	}
	if end-start == 1 {
		return source[start] == '\n'
	}
	return end-start == 2 && source[start] == '\r' && source[start+1] == '\n'
}
