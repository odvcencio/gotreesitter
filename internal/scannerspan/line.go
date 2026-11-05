// Package scannerspan provides span kernels for scanner-facing lexers.
package scannerspan

import "unicode/utf8"

// LineContent returns the byte and rune counts and last rune of the valid
// UTF-8 prefix before CR, LF, NUL, an invalid UTF-8 byte, or EOF.
func LineContent(source []byte) (bytes, runes int, last rune) {
	for bytes < len(source) {
		b := source[bytes]
		if b == 0 || b == '\r' || b == '\n' {
			break
		}
		if b < utf8.RuneSelf {
			bytes++
			runes++
			last = rune(b)
			continue
		}
		r, size := utf8.DecodeRune(source[bytes:])
		if r == utf8.RuneError && size == 1 {
			break
		}
		bytes += size
		runes++
		last = r
	}
	return
}
