//go:build !grammar_subset || grammar_subset_xml

package grammarruntime

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Reproducers from the pinned C scanner, xml/src/scanner.c with common/scanner.h
// (tree-sitter-grammars/tree-sitter-xml 5000ae8f). The expected results follow the C code, not
// the old port. Masks list the valid external tokens in xmlTok* order.
func TestXMLScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src, mask string
		found           bool
		symbol          gotreesitter.Symbol
		start, end      uint32
	}
	for _, c := range []tc{
		// scanner.c:79-81 in_char_data tests lexer->eof: a NUL byte is character data.
		{"chardata-nul", "a\x00b<", "00010000000", true, 69, 0, 3},
		{"chardata-starts-with-nul", "\x00a<", "00010000000", true, 69, 0, 2},
		// scanner.c:120 scan_cdata tests lexer->eof.
		{"cdata-nul", "a\x00]]>", "00001000000", true, 70, 0, 2},
		// common/scanner.h:135 the comment loop tests lexer->eof.
		{"comment-nul", "<!--\x00-->", "00100000000", true, 68, 0, 8},
		// common/scanner.h:109 the processing-instruction loop tests lexer->eof.
		{"pi-content-nul", "a\x00b?>\n", "01000000000", true, 67, 0, 3},
		// common/scanner.h:43-45 iswalpha is ASCII-only in the C locale.
		{"pi-target-nonascii-start", "éa", "10000000000", false, 0, 0, 0},
		// scanner.c:20 and :24 with common/scanner.h:37-45: a name stops at a non-ASCII letter.
		{"tag-name-nonascii-start", "é", "00000001000", false, 0, 0, 0},
		{"tag-name-nonascii-ends-name", "abé", "00000001000", true, 71, 0, 2},
		{"tag-name-nonascii-digit-ends-name", "a١", "00000001000", true, 71, 0, 1},
		// common/scanner.h:38 0xB7 still continues a name.
		{"tag-name-middle-dot", "a·b>", "00000001000", true, 71, 0, 4},
		// scanner.c:38 start-tag-name does not call mark_end, so the end of the earlier
		// scan_char_data (scanner.c:89) stays: the token ends at byte 1, not at the cursor.
		{"start-tag-keeps-earlier-mark-end", "a]]>b", "00010001000", true, 71, 0, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := XMLExternalScanner{}
			valid := make([]bool, len(c.mask))
			for i := range valid {
				valid[i] = c.mask[i] == '1'
			}
			r := portScanAt(sc, sc.Create(), c.src, 0, valid)
			if r.ok != c.found || (c.found && (r.sym != c.symbol || r.start != c.start || r.end != c.end)) {
				t.Errorf("got found=%v symbol=%d start=%d end=%d; want found=%v symbol=%d start=%d end=%d", r.ok, r.sym, r.start, r.end, c.found, c.symbol, c.start, c.end)
			}
		})
	}
}

// scanner.c:220: serialize stops when size + 2 + name_length reaches 1024, not when the name no
// longer fits the buffer.
func TestXMLScannerCParitySerializeLimit(t *testing.T) {
	sc := XMLExternalScanner{}
	p := sc.Create().(*xmlScannerState)
	p.tags = []string{strings.Repeat("a", 255), strings.Repeat("b", 255), strings.Repeat("c", 255), strings.Repeat("d", 246)}
	buf := make([]byte, 1024)
	n := sc.Serialize(p, buf)
	// 8 header bytes and three tags of 256 bytes. The fourth tag would end at size 1023 + 1.
	if n != 8+3*256 {
		t.Errorf("serialized %d bytes; want %d", n, 8+3*256)
	}
	if buf[0] != 3 || buf[4] != 4 {
		t.Errorf("serialized count %d, tag count %d; want 3 and 4", buf[0], buf[4])
	}
}

// scanner.c:243-271: only a zero length returns early; a shorter state reads zero bytes past its
// end and pads with empty tags up to tag_count.
func TestXMLScannerCParityShortState(t *testing.T) {
	sc := XMLExternalScanner{}
	p := sc.Create().(*xmlScannerState)
	sc.Deserialize(p, []byte{1, 0, 0, 0, 2})
	if len(p.tags) != 2 || p.tags[0] != "" || p.tags[1] != "" {
		t.Errorf("tags after a 5-byte state = %q; want two empty tags", p.tags)
	}
	// A name that runs past the end reads zero bytes.
	sc.Deserialize(p, []byte{1, 0, 0, 0, 1, 0, 0, 0, 3})
	if len(p.tags) != 1 || p.tags[0] != "\x00\x00\x00" {
		t.Errorf("tags = %q; want one tag of three zero bytes", p.tags)
	}
}
