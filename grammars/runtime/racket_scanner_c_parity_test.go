//go:build !grammar_subset || grammar_subset_racket

package grammarruntime

import "testing"

// TestRacketHereStringEndMatchesC pins the end test of a here string in the
// pinned C scanner (tree-sitter-racket src/scanner.c:44-52 and :117). C keeps
// each character as a 4-byte int and compares the two lines with strcmp, which
// stops at the first zero byte, so for ASCII text only the first character of
// the terminator and of the line are compared. The port compared whole lines.
func TestRacketHereStringEndMatchesC(t *testing.T) {
	scanner := RacketExternalScanner{}
	valid := make([]bool, racketTokenCount)
	valid[racketTokHereStringBody] = true
	sym := racketDefaultSymTable[racketTokHereStringBody]
	cases := []struct {
		name string
		src  string
		ok   bool
		end  uint32
	}{
		{"whole line equal", "END\nbody\nEND", true, 12},
		{"line starts with the terminator's first character", "END\nbody\nEXIT", true, 13},
		{"first character differs", "END\nbody\nX", false, 0},
		{"empty terminator, empty line", "\nbody\n\n", true, 6},
		{"line with a zero low byte equals the empty line", "\nbody\nĀ", true, 8},
	}
	for _, c := range cases {
		got := portScan(scanner, cFaithfulPayload(scanner), c.src, valid)
		portWantScan(t, c.name, got, c.ok, sym, c.end)
	}
}

// TestRacketHereStringRawBytesLikeC covers two cases. C reads an
// invalid UTF-8 byte as the decode error -1 (four 0xff bytes in its buffer, no
// zero byte), so "\xffA\n\xffB" does not match: the lines differ at the second
// byte of the first character. A NUL byte is a character, not the end of the
// input (lexer->eof is false): "A\x00B\nA" then "A" matches at the third line
// as C reads it, so the terminator "A\x00B" is compared with the line "A".
func TestRacketHereStringRawBytesLikeC(t *testing.T) {
	scanner := RacketExternalScanner{}
	valid := make([]bool, racketTokenCount)
	valid[racketTokHereStringBody] = true
	sym := racketDefaultSymTable[racketTokHereStringBody]
	portWantScan(t, "invalid bytes", portScan(scanner, cFaithfulPayload(scanner), "\xffA\n\xffB", valid), false, sym, 0)
	portWantScan(t, "NUL is a character", portScan(scanner, cFaithfulPayload(scanner), "A\x00B\nA", valid), true, sym, 5)
	// A literal U+FFFD is not a decode error: its bytes hold zero, so the lines match.
	portWantScan(t, "literal U+FFFD", portScan(scanner, cFaithfulPayload(scanner), "\xef\xbf\xbdA\n\xef\xbf\xbdB", valid), true, sym, 9)
}
