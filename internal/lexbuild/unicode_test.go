package lexbuild

import (
	"testing"
	"unicode"
)

// TestUnicodeAlphabeticVersusGo reports how Go's stand-in for char::is_alphabetic
// (Letter, Letter number, Other_Alphabetic) differs from the embedded table of
// the generator's Rust release. It reports and does not fail.
func TestUnicodeAlphabeticVersusGo(t *testing.T) {
	diff, first := 0, -1
	for c := rune(0); c < 0x110000; c++ {
		if c >= 0xD800 && c <= 0xDFFF {
			continue
		}
		g := unicode.IsLetter(c) || unicode.Is(unicode.Nl, c) || unicode.Is(unicode.Other_Alphabetic, c)
		if g != isRustAlphabetic(uint32(c)) {
			diff++
			if first < 0 {
				first = int(c)
			}
		}
	}
	t.Logf("UNICODE alphabetic: Go %s tables against Rust table of Unicode 17.0.0: %d differing code points (first U+%04X)", unicode.Version, diff, first)
	for _, c := range []uint32{'a', 'Z', '_', 0xAA, 0x3B1, 0x3347A, 0x3347B} {
		_ = isRustAlphabetic(c)
	}
	if !isRustAlphabetic('a') || isRustAlphabetic('_') || !isRustAlphabetic(0xAA) || isRustAlphabetic(0xA9) {
		t.Error("embedded table spot checks")
	}
}
