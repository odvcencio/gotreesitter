// Package lexrun contains bounded, state-preserving lexer loops.
package lexrun

// NoMatch and SkipBit are shared with the native ASCII table builder.
const NoMatch = int32(0x7fff_ffff)
const SkipBit = int32(-1 << 31)

// ASCII advances over explicit, non-skipping transitions back to state.
// Newlines, Unicode, failed transitions and every state change stay with the
// caller's scalar scanner. The caller records the final state's acceptance.
func ASCII(source []byte, start int, row *[128]int32, state int32) int {
	if start < 0 || start > len(source) || row == nil || uint32(state) >= uint32(NoMatch) {
		return start
	}
	for start < len(source) {
		b := source[start]
		if b >= 128 || b == '\n' || row[b] != state {
			break
		}
		start++
	}
	return start
}
