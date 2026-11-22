//go:build !grammar_subset || grammar_subset_php

package grammarruntime

import (
	"bytes"
	"testing"
)

// Reproducers from the pinned C scanner (tree-sitter-php 3fda2fb9, common/scanner.h). The
// expected results follow the C code, not the old port.
func phpScanOnce(src string, valid int, heredocs ...string) (bool, uint32) {
	st := &phpState{cFaithful: true}
	for _, h := range heredocs {
		st.heredocs = append(st.heredocs, phpHeredoc{word: []rune(h)})
	}
	vs := make([]bool, phpTokenCount)
	vs[valid] = true
	lexer := newPortParityLexer([]byte(src), 0, 0, 0)
	portInstallObserver(lexer)
	ok := PhpExternalScanner{}.Scan(st, lexer, vs)
	r := portObserve(lexer, ok)
	return r.ok, r.end
}

func TestPhpScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src string
		valid     int
		heredocs  []string
		found     bool
		end       uint32
	}
	for _, c := range []tc{
		// :238 only lexer->eof ends the nowdoc text; a NUL byte is text.
		{"nul-in-nowdoc", "a\x00b\nc", phpTokNowdocString, []string{"A"}, true, 3},
		// :372 the same for encapsed string text.
		{"nul-in-encapsed", "a\x00b\"", phpTokEncapsedStringChars, nil, true, 3},
		// :477 an EOF token needs the end of input, not a NUL byte.
		{"nul-is-not-eof", "\x00", phpTokEOF, nil, false, 0},
		// :456 iswspace is ASCII only in the C locale: U+00A0 is not skipped, and it is a
		// name character (:145), so the word is not "A".
		{"nbsp-before-heredoc-end", " A", phpTokHeredocEnd, []string{"A"}, false, 0},
		// :486 and :179 a byte that is not valid UTF-8 reads as -1 in C: not a name
		// character, so no heredoc word starts there.
		{"invalid-byte-no-word", "\xffA", phpTokHeredocStart, nil, false, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			ok, end := phpScanOnce(c.src, c.valid, c.heredocs...)
			if ok != c.found || (c.found && end != c.end) {
				t.Errorf("got found=%v end=%d; want found=%v end=%d", ok, end, c.found, c.end)
			}
		})
	}
}

// :88-90 C empties the words of its entries but keeps the entries; the next
// serialize writes them (:65-79).
func TestPhpScannerCParityStaleHeredocEntries(t *testing.T) {
	sc := PhpExternalScanner{}
	st := &phpState{cFaithful: true}
	two := []byte{2, 0, 1, 0, 0, 0, 'A', 0, 0, 0, 1, 1, 0, 0, 0, 'B', 0, 0, 0}
	sc.Deserialize(st, two)
	sc.Deserialize(st, nil)
	buf := make([]byte, 4096)
	n := sc.Serialize(st, buf)
	want := []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(buf[:n], want) {
		t.Errorf("serialize after empty state = %v; want %v", buf[:n], want)
	}
	// :99-100 a shorter new state reuses the first entry and leaves the second empty.
	sc.Deserialize(st, []byte{1, 0, 1, 0, 0, 0, 'A', 0, 0, 0})
	n = sc.Serialize(st, buf)
	want = []byte{2, 0, 1, 0, 0, 0, 'A', 0, 0, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(buf[:n], want) {
		t.Errorf("serialize after shorter state = %v; want %v", buf[:n], want)
	}
	// The default mode keeps no stale entries.
	d := &phpState{}
	sc.Deserialize(d, two)
	sc.Deserialize(d, nil)
	if n := sc.Serialize(d, buf); n != 1 {
		t.Errorf("default mode serialize after empty state = %d bytes; want 1", n)
	}
}

// :69 the C check uses the 1,024 bytes of the C runtime, not the larger buffer of the engine.
func TestPhpScannerCParitySerializeLimit(t *testing.T) {
	sc := PhpExternalScanner{}
	st := &phpState{cFaithful: true, heredocs: []phpHeredoc{{word: make([]rune, 300)}}}
	buf := make([]byte, 4096)
	if n := sc.Serialize(st, buf); n != 0 {
		t.Errorf("C-faithful serialize of a 1,200-byte word = %d; want 0", n)
	}
	st.cFaithful = false
	if n := sc.Serialize(st, buf); n == 0 {
		t.Errorf("default serialize of a 1,200-byte word = 0; want the state")
	}
}
