//go:build !grammar_subset

package grammarruntime

import (
	"bytes"
	"strings"
	"testing"
)

// Pinned C scanner src/scanner.c:1331 distinguishes NUL from EOF.
func TestMarkdownScannerCParityReproducers(t *testing.T) {
	checkScannerGroup1(t, MarkdownExternalScanner{}, scannerGroup1Case{
		"ordinary-lookahead", "\x00", 0, "01001111111100111111111111110010111111010000010", "", false, 0, 0, 0, 2,
	})
}

// mdParityRun deserializes state into a new scanner, scans src from byte 0 with the listed
// external tokens valid, and returns what the scan observed and the saved state after it.
func mdParityRun(state []byte, src string, valid ...int) (portScanResult, []byte) {
	sc := MarkdownExternalScanner{}
	p := sc.Create()
	sc.Deserialize(p, state)
	mask := make([]bool, mdTokenCount)
	for _, i := range valid {
		mask[i] = true
	}
	lexer := newPortParityLexer([]byte(src), 0, 0, 0)
	portInstallObserver(lexer)
	ok := sc.Scan(p, lexer, mask)
	r := portObserve(lexer, ok)
	buf := make([]byte, 2048)
	n := sc.Serialize(p, buf)
	return r, buf[:n]
}

// src/scanner.c:239-253 and :257-289: an open block is a C enum Block, saved as four bytes,
// least significant byte first. A list marker "- " opens LIST_ITEM (2).
func TestMarkdownScannerCParityBlocksAreFourBytes(t *testing.T) {
	_, got := mdParityRun(nil, "- x", mdTokListMarkerMinus)
	want := []byte{0, 0, 0, 2, 0, 2, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("saved state % x; want % x", got, want)
	}

	// ANONYMOUS (19) is one block: at the end of input BLOCK_CLOSE pops that block.
	_, got = mdParityRun([]byte{0, 0, 0, 0, 0, 19, 0, 0, 0}, "", mdTokBlockClose)
	want = []byte{0, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("saved state after the close % x; want % x", got, want)
	}

	// :274-281 the blocks are the whole groups of four bytes: a rest of two bytes is ignored.
	_, got = mdParityRun([]byte{0, 0, 0, 0, 0, 19, 0, 0, 0, 9, 9}, "x")
	want = []byte{0, 0, 0, 0, 0, 19, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("saved state with a rest % x; want % x", got, want)
	}
}

// :1009-1101 iswalpha and iswalnum are those of the C locale, which only know ASCII: a tag or
// an attribute name that starts with, or goes on with, a non-ASCII letter or digit is not one.
func TestMarkdownScannerCParityHTMLNamesAreASCII(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"tag-name-letter", "<\u00e9>\n"},
		{"tag-name-continued-by-digit", "<a\u0663>\n"},
		{"attribute-name-letter", "<a \u00e9=1>\n"},
		{"attribute-name-continued-by-letter", "<a b\u00e9=1>\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, _ := mdParityRun(nil, c.src, mdTokHTMLBlock7Start)
			if r.ok {
				t.Errorf("found a token ending at %d; want none", r.end)
			}
		})
	}
	// With ASCII names the same text is an HTML block of kind 7.
	if r, _ := mdParityRun(nil, "<a b=1>\n", mdTokHTMLBlock7Start); !r.ok {
		t.Error("an ASCII tag was not found")
	}
}

// :467 a count of stars is a size_t, not a 16-bit number: 65,537 stars and a line end are a
// thematic break (the old count wrapped round to one).
func TestMarkdownScannerCParityStarCountDoesNotWrap(t *testing.T) {
	src := strings.Repeat("*", 65537) + "\n"
	r, _ := mdParityRun(nil, src, mdTokThematicBreak)
	if !r.ok || r.end != 65537 {
		t.Errorf("got found=%v end=%d; want a thematic break ending at 65537", r.ok, r.end)
	}
}

// :497 and others: s->matched == s->open_blocks.size compares a byte with the whole count.
// With 256 open blocks and no block matched yet the two differ, so a lone "*" is not a
// list marker that may not interrupt a paragraph (the old code cut the count to a byte).
func TestMarkdownScannerCParityMatchedIsComparedWithFullBlockCount(t *testing.T) {
	state := make([]byte, 5, 5+4*256)
	for i := 0; i < 256; i++ {
		state = append(state, 19, 0, 0, 0)
	}
	r, _ := mdParityRun(state, "*\n", mdTokListMarkerStarDontInterrupt)
	if r.ok {
		t.Errorf("found a token ending at %d; want none", r.end)
	}
}
