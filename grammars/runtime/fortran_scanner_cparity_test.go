//go:build !grammar_subset || grammar_subset_fortran

package grammarruntime

import (
	"bytes"
	"testing"
)

// Reproducers from the pinned C scanner (stadelmanma/tree-sitter-fortran 2880b7aa,
// src/scanner.c). The expected results follow the C code, not the old port. C reads a
// lookahead of any value, and wherever a function takes a char it keeps only the low
// eight bits (a signed char), so U+0122 acts as '"' there; the classes iswspace, iswalnum
// and the others are those of the C locale (ASCII only).
func TestFortranScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src string
		valid     []int // external token indexes that are valid
		found     bool
		tok       int
		end       uint32
	}
	for _, c := range []tc{
		// :115 an ASCII digit only: U+0663 (Arabic-indic digit three) is not iswdigit in C.
		{"non-ascii-digit", "\u0663", []int{ftnTokIntegerLiteral}, false, 0, 0},
		// :168 iswalnum is ASCII only, so a decimal point before U+00E9 is part of the token.
		{"decimal-before-non-ascii-letter", "1.\u00e9", []int{ftnTokFloatLiteral}, true, ftnTokFloatLiteral, 2},
		// :591, :605 iswblank and iswspace are ASCII only: a no-break space is not skipped.
		{"nbsp-is-not-blank", "\u00a01", []int{ftnTokIntegerLiteral}, false, 0, 0},
		{"nbsp-is-not-space-after-blank", " \u00a01", []int{ftnTokIntegerLiteral}, false, 0, 0},
		// :383 opening_quote is a char: U+0122 has the low byte of '"' and opens a string,
		// and the literal ends at the next real '"'.
		{"string-opens-on-low-byte", "\u0122a\"", []int{ftnTokStringLiteral}, true, ftnTokStringLiteral, 4},
		// :68 is_boz_sentinel takes a char: U+0142 has the low byte of 'B'.
		{"boz-prefix-low-byte", "\u0142'1'", []int{ftnTokBozLiteral}, true, ftnTokBozLiteral, 5},
		// :82 is_exp_sentinel takes a char: U+0145 has the low byte of 'E'; the token
		// ends at the last mark_end, after the sign.
		{"exp-sentinel-low-byte", "1\u0145+", []int{ftnTokIntegerLiteral}, true, ftnTokIntegerLiteral, 4},
		// :449 next_char is a char: U+0121 has the low byte of '!'.
		{"preproc-low-byte", "\u0121", []int{ftnTokPreprocUnaryOp}, true, ftnTokPreprocUnaryOp, 2},
		// :66 is_identifier_char takes a char: U+2041 has the low byte of 'A' and continues
		// an identifier; U+00E9 has a negative char value and ends it.
		{"kind-identifier-low-byte", "a\u2041_\"", []int{ftnTokStringLiteralKind}, true, ftnTokStringLiteralKind, 4},
		{"kind-negative-char-ends", "a\u00e9_\"", []int{ftnTokStringLiteralKind}, false, 0, 0},
		// :392 a NUL byte inside a string is content; only the end of input stops it.
		{"string-nul-is-content", "'a\x00b'", []int{ftnTokStringLiteral}, true, ftnTokStringLiteral, 5},
		// :290 a NUL byte is not the end of input: no end of statement here.
		{"eos-nul-is-not-eof", "\x00", []int{ftnTokEndOfStatement}, false, 0, 0},
		// :511-514 scan_do_label does not call mark_end: the token ends where the number
		// scan marked it, before the 'e' that was read and found to be no exponent.
		{"do-label-ends-at-last-mark", "12ex", []int{ftnTokDoLabel}, true, ftnTokDoLabel, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := FortranExternalScanner{}
			valid := make([]bool, ftnTokenCount)
			for _, i := range c.valid {
				valid[i] = true
			}
			lexer := newPortParityLexer([]byte(c.src), 0, 0, 0)
			portInstallObserver(lexer)
			ok := sc.Scan(sc.Create(), lexer, valid)
			r := portObserve(lexer, ok)
			if r.ok != c.found || (c.found && (r.end != c.end || r.sym != ftnDefaultSymTable[c.tok])) {
				t.Errorf("got found=%v sym=%d end=%d; want found=%v sym=%d end=%d", r.ok, r.sym, r.end, c.found, ftnDefaultSymTable[c.tok], c.end)
			}
		})
	}
}

// :461 track_labeled_do reads labels[depth-1] with no test of depth. With an empty stack
// that is the depth field itself, so the label 0 matches and the scanner counts it in an
// array item that is never saved: nothing is pushed and the saved state stays empty.
func TestFortranScannerCParityDoLabelZeroOnEmptyStack(t *testing.T) {
	sc := FortranExternalScanner{}
	p := sc.Create()
	valid := make([]bool, ftnTokenCount)
	valid[ftnTokDoLabel] = true
	lexer := newPortParityLexer([]byte("0"), 0, 0, 0)
	if !sc.Scan(p, lexer, valid) {
		t.Fatal("no do label found")
	}
	buf := make([]byte, 1024)
	n := sc.Serialize(p, buf)
	want := make([]byte, 10) // in_line_continuation, depth 0, pending 0, eos 0
	if !bytes.Equal(buf[:n], want) {
		t.Errorf("saved state % x; want % x", buf[:n], want)
	}
}

// :700-739 deserialize reads the whole layout whatever the length: a short state is read as
// if it went on with zero bytes (the buffer is zero filled), and it never fails.
func TestFortranScannerCParityShortSavedState(t *testing.T) {
	for _, c := range []struct {
		name  string
		state []byte
		want  []byte
	}{
		// depth bytes 00 03 01 and a missing fourth byte: 0x00010300 is above the limit, so the
		// depth, the pending count and the flag are reset; the first byte stays set (:721-726).
		{"depth-over-limit-short", []byte{2, 0, 3, 1}, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		// depth 1, one label, no count, no pending count, no flag.
		{"depth-one-short", []byte{0, 1, 0, 0, 0, 7, 0, 0, 0}, []byte{0, 1, 0, 0, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"one-byte", []byte{1}, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := FortranExternalScanner{}
			p := sc.Create()
			sc.Deserialize(p, c.state)
			buf := make([]byte, 1024)
			n := sc.Serialize(p, buf)
			if !bytes.Equal(buf[:n], c.want) {
				t.Errorf("saved state % x; want % x", buf[:n], c.want)
			}
		})
	}
}
