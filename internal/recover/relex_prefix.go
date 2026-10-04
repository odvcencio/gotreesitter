package recover

import "bytes"

// RelexPrefixPoint recovers the start of an authenticated lexer-call prefix.
// A state's DFA must see those bytes too: bytes skipped by one lex mode may
// make another mode fail before it ever reaches the shared token.
func RelexPrefixPoint(source []byte, prefix, tokenStart, row, column uint32) (uint32, uint32, bool) {
	if prefix > tokenStart || uint64(tokenStart) > uint64(len(source)) {
		return 0, 0, false
	}
	gap := source[prefix:tokenStart]
	newlines := uint32(bytes.Count(gap, []byte{'\n'}))
	if newlines == 0 {
		if column < tokenStart-prefix {
			return 0, 0, false
		}
		return row, column - (tokenStart - prefix), true
	}
	if row < newlines {
		return 0, 0, false
	}
	previousNewline := bytes.LastIndexByte(source[:prefix], '\n')
	return row - newlines, prefix - uint32(previousNewline+1), true
}
