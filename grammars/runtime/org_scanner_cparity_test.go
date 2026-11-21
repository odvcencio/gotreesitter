//go:build !grammar_subset || grammar_subset_org

package grammarruntime

import (
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Reproducers from the pinned C scanner, src/scanner.c (emiasims/tree-sitter-org 64cfbc21). The
// expected results follow the C code, not the old port. Masks list the valid external tokens in
// orgTok* order: liststart, listend, listitemend, bullet, hlstars, sectionend, eof.
func TestOrgScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src, mask string
		found           bool
		symbol          gotreesitter.Symbol
		start, end      uint32
	}
	// The scanner skips characters after its mark_end, so the token positions of these cases are
	// not compared.
	const noPos = ^uint32(0)
	for _, c := range []tc{
		// :147 iswspace is ASCII-only in the C locale: a no-break space after '-' is no bullet.
		{"bullet-dash-nbsp", " - x", "1000000", false, 0, 0, 0},
		// :151-152 the same after '+', and :159-161 after a letter and '.'.
		{"bullet-plus-nbsp", " + x", "1000000", false, 0, 0, 0},
		{"bullet-letter-dot-nbsp", " a. x", "1000000", false, 0, 0, 0},
		// :147 an ASCII space still makes the bullet (a list start at indent 1).
		{"bullet-dash-space", " - x", "1000000", true, 117, 0, noPos},
		// :270-275 the stars of a headline need an ASCII space or line break after them.
		{"stars-nbsp", "* x", "0000100", false, 0, 0, 0},
		{"stars-space", "* x", "0000100", true, 121, 0, noPos},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := OrgExternalScanner{}
			valid := make([]bool, len(c.mask))
			for i := range valid {
				valid[i] = c.mask[i] == '1'
			}
			r := portScanAt(sc, sc.Create(), c.src, 0, valid)
			if r.ok != c.found || (c.found && (r.sym != c.symbol || (c.end != noPos && (r.start != c.start || r.end != c.end)))) {
				t.Errorf("got found=%v symbol=%d start=%d end=%d; want found=%v symbol=%d start=%d end=%d", r.ok, r.sym, r.start, r.end, c.found, c.symbol, c.start, c.end)
			}
		})
	}
}

// scanner.c:120-127: each saved byte is a char, which is signed where the C code ran, and the
// indent and bullet loops do not test the length, so a short state reads zero bytes past its end.
func TestOrgScannerCParityDeserialize(t *testing.T) {
	sc := OrgExternalScanner{}
	p := sc.Create().(*orgState)

	sc.Deserialize(p, []byte{1, 0xFC, 0x03})
	if !reflect.DeepEqual(p.indents, []int16{-1, -4}) || !reflect.DeepEqual(p.bullets, []int16{0, 3}) {
		t.Errorf("indents %v bullets %v; want [-1 -4] and [0 3]", p.indents, p.bullets)
	}

	sc.Deserialize(p, []byte{1, 0x01, 0x02, 0xFF})
	if !reflect.DeepEqual(p.sections, []int16{0, -1}) {
		t.Errorf("sections %v; want [0 -1]", p.sections)
	}

	sc.Deserialize(p, []byte{2})
	if !reflect.DeepEqual(p.indents, []int16{-1, 0, 0}) || !reflect.DeepEqual(p.bullets, []int16{0, 0, 0}) {
		t.Errorf("a 1-byte state gave indents %v bullets %v; want [-1 0 0] and [0 0 0]", p.indents, p.bullets)
	}
}

// scanner.c:78-102: serialize writes the whole of each stack, one after the other. It does not cut
// the bullet stack to the length of the indent stack.
func TestOrgScannerCParitySerializeWholeStacks(t *testing.T) {
	sc := OrgExternalScanner{}
	p := sc.Create().(*orgState)
	p.indents = []int16{-1, 4, 6}
	p.bullets = []int16{0, 1}
	p.sections = []int16{0, 2, 3}
	buf := make([]byte, 1024)
	n := sc.Serialize(p, buf)
	want := []byte{2, 4, 6, 1, 2, 3}
	if string(buf[:n]) != string(want) {
		t.Errorf("serialized %v; want %v", buf[:n], want)
	}
}
