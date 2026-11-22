//go:build !grammar_subset || grammar_subset_ocaml

package grammarruntime

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Reproducers from the pinned C scanner, common/scanner.h (tree-sitter/tree-sitter-ocaml
// 3b2e14e0). The expected results follow the C code, not the old port. Masks list the valid
// external tokens in ocamlTok* order: comment, left quoted string delimiter, right quoted string
// delimiter, string delimiter, line number directive, null character, error sentinel.
func TestOcamlScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src, mask string
		found           bool
		symbol          gotreesitter.Symbol
		end             uint32
	}
	for _, c := range []tc{
		// :583-588 a NUL byte inside the input is the null character; only the end of input is not.
		{"null-character", "\x00x", "0000010", true, 169, 1},
		{"null-character-at-eof", "", "0000010", false, 0, 0},
		// :458-465 a NUL byte in a comment does not end it; the end of input does.
		{"comment-nul", "(*\x00*)", "1000000", true, 165, 5},
		{"comment-nul-then-eof", "(*\x00", "1000000", true, 165, 3},
		// :83-86 a NUL byte in a string inside a comment does not end the string.
		{"comment-string-nul", "(* \"a\x00\" *)", "1000000", true, 165, 10},
		// :344-347 a NUL byte after a quote in a comment is skipped, not the end of the character.
		{"comment-character-nul", "(* '\x00' *)", "1000000", true, 165, 9},
		// :276-279 a NUL byte in a quoted string inside a comment does not end it.
		{"comment-quoted-string-nul", "(* {|\x00|} *)", "1000000", true, 165, 11},
		// :559-562 the file name of a line directive ends at the end of input, not at a NUL byte.
		{"line-directive-nul", "# 1 \"a\x00b\"\n", "0000100", true, 168, 9},
		// :529 iswspace is ASCII-only in the C locale: a no-break space is not skipped.
		{"nbsp-not-skipped", " (* x *)", "1000000", false, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := OcamlExternalScanner{}
			valid := make([]bool, len(c.mask))
			for i := range valid {
				valid[i] = c.mask[i] == '1'
			}
			r := portScanAt(sc, sc.Create(), c.src, 0, valid)
			if r.ok != c.found || (c.found && (r.sym != c.symbol || (c.end != 0 && r.end != c.end))) {
				t.Errorf("got found=%v symbol=%d end=%d; want found=%v symbol=%d end=%d", r.ok, r.sym, r.end, c.found, c.symbol, c.end)
			}
		})
	}
}

// :232-248 scan_left_quoted_string_delimiter pushes onto the identifier the saved state loaded;
// it does not clear it first.
func TestOcamlScannerCParityLeftDelimiterKeepsIdentifier(t *testing.T) {
	sc := OcamlExternalScanner{}
	p := sc.Create().(*ocamlScannerState)
	p.idPush('a')
	r := portScanAt(sc, p, "b|x", 0, []bool{false, true})
	if !r.ok {
		t.Fatal("no token")
	}
	if p.idLen != 2 || p.id[0] != 'a' || p.id[1] != 'b' {
		t.Errorf("identifier = %v (length %d); want [a b]", p.id[:p.idLen], p.idLen)
	}
}

// :56-60 quoted_string_id_copy copies quoted_string_id_length BYTES of the int32 array, so the
// saved state is one flag byte and then that many bytes.
func TestOcamlScannerCParitySerialize(t *testing.T) {
	sc := OcamlExternalScanner{}
	p := sc.Create().(*ocamlScannerState)
	p.inString = true
	p.idPush(0x61)
	p.idPush(0x62)
	buf := make([]byte, 1024)
	n := sc.Serialize(p, buf)
	want := []byte{1, 0x61, 0x00}
	if string(buf[:n]) != string(want) {
		t.Errorf("serialized %v; want %v", buf[:n], want)
	}
	// :488-492 an identifier of 1024 items or more saves no identifier at all.
	q := sc.Create().(*ocamlScannerState)
	for i := 0; i < 1024; i++ {
		q.idPush('a')
	}
	if n := sc.Serialize(q, buf); n != 1 {
		t.Errorf("a 1024-item identifier serialized to %d bytes; want 1", n)
	}
}

// :47-54 quoted_string_id_assign copies length-1 bytes into the int32 array and takes length-1 as
// the number of items. Bytes the state does not cover keep their old value.
func TestOcamlScannerCParityDeserialize(t *testing.T) {
	sc := OcamlExternalScanner{}
	p := sc.Create().(*ocamlScannerState)
	sc.Deserialize(p, []byte{1, 0x61, 0x62, 0x63})
	if !p.inString || p.idLen != 3 || p.id[0] != 0x636261 || p.id[1] != 0 || p.id[2] != 0 {
		t.Errorf("state = in_string %v, length %d, items %v; want true, 3, [0x636261 0 0]", p.inString, p.idLen, p.id[:p.idLen])
	}
	// A shorter state over the same payload leaves the old upper bytes in place.
	sc.Deserialize(p, []byte{0, 0x7a})
	if p.inString || p.idLen != 1 || p.id[0] != 0x63627a {
		t.Errorf("state = in_string %v, length %d, item %#x; want false, 1, 0x63627a", p.inString, p.idLen, p.id[0])
	}
	sc.Deserialize(p, nil)
	if p.inString || p.idLen != 0 {
		t.Errorf("an empty state gave in_string %v, length %d", p.inString, p.idLen)
	}
}
