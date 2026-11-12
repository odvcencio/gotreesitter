//go:build !grammar_subset || grammar_subset_html

package grammarruntime

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Each test runs the C-faithful mode (cFaithfulPayload) and, where main's scan differs, pins
// main's behaviour with the mode off. C references are src/scanner.c and src/tag.h of the pinned
// tree-sitter-html commit.

func htmlValid(toks ...int) []bool {
	v := make([]bool, htmlTokenCount)
	for _, t := range toks {
		v[t] = true
	}
	return v
}

func htmlSymOf(tok int) gotreesitter.Symbol { return htmlDefaultSymTable[tok] }

// TestHTMLSelfClosingWithNoOpenTagReturnsStartTagNameLikeC: scan_self_closing_tag_delimiter
// (scanner.c:276-287) returns true without setting a result symbol when the stack is empty; the
// C lexer starts a scan with result_symbol 0, the external START_TAG_NAME. Main returned no
// token.
func TestHTMLSelfClosingWithNoOpenTagReturnsStartTagNameLikeC(t *testing.T) {
	sc := HTMLExternalScanner{}
	valid := htmlValid(htmlTokSelfClosingTag)
	portWantScan(t, "mode on", portScanAt(sc, cFaithfulPayload(sc), "T/>", 1, valid), true, htmlSymOf(htmlTokStartTagName), 3)
	if got := portScanAt(sc, sc.Create(), "T/>", 1, valid); got.hasTok {
		t.Errorf("mode off (main): expected no token, got %+v", got)
	}
}

// TestHTMLTagTableAndStateFollowC: the saved state is the C layout (scanner.c:28-60): two counts
// (written, held) then one byte per tag with the tag.h enum (AREA 0 ... WBR 22, END_OF_VOID_TAGS
// 23, A 24, ...). <audio> is a known tag in C (AUDIO) and a custom tag in main's table.
func TestHTMLTagTableAndStateFollowC(t *testing.T) {
	sc := HTMLExternalScanner{}
	valid := htmlValid(htmlTokStartTagName)
	st := cFaithfulPayload(sc)
	portScan(sc, st, "audio", valid)
	buf := make([]byte, 64)
	n := sc.Serialize(st, buf)
	want := []byte{1, 0, 1, 0, byte(htmlCType("AUDIO"))}
	if string(buf[:n]) != string(want) {
		t.Errorf("mode on: state %v, want %v", buf[:n], want)
	}
	if htmlCType("AUDIO") != 29 || htmlCType("WBR") != 22 || htmlCType("A") != 24 || htmlCType("CUSTOM") != 126 {
		t.Errorf("tag.h numbering: AUDIO=%d WBR=%d A=%d CUSTOM=%d", htmlCType("AUDIO"), htmlCType("WBR"), htmlCType("A"), htmlCType("CUSTOM"))
	}
	off := sc.Create()
	portScan(sc, off, "audio", valid)
	n = sc.Serialize(off, buf)
	if string(buf[:n]) == string(want) {
		t.Errorf("mode off (main) wrote the C state: %v", buf[:n])
	}
}

// TestHTMLParagraphContentModelFollowsC: tag_can_contain for P (tag.h:356-362) forbids exactly the
// 26 listed tags. HGROUP, NOSCRIPT, SCRIPT, TABLE, TEMPLATE, UL are block tags in main's table
// and not in C's list; HR and... are in both. A <p> followed by <ul> stays open in C.
func TestHTMLParagraphContentModelFollowsC(t *testing.T) {
	sc := HTMLExternalScanner{}
	for _, on := range []bool{true, false} {
		st := sc.Create().(*htmlScannerState)
		st.SetCFaithful(on)
		portScan(sc, st, "p", htmlValid(htmlTokStartTagName))
		got := portScan(sc, st, "<ul>", htmlValid(htmlTokImplicitEndTag))
		if on && got.ok {
			t.Errorf("mode on: <ul> closed the paragraph")
		}
		if !on && !got.ok {
			t.Errorf("mode off (main): <ul> should close the paragraph")
		}
	}
}

// TestHTMLWhiteSpaceIsTheCLocale: iswspace (scanner.c:294) is ASCII-only in the C locale;
// vertical tab and form feed are white space, U+0085 is not.
func TestHTMLWhiteSpaceIsTheCLocale(t *testing.T) {
	sc := HTMLExternalScanner{}
	valid := htmlValid(htmlTokStartTagName)
	portWantScan(t, "vertical tab", portScan(sc, cFaithfulPayload(sc), "\vdiv", valid), true, htmlSymOf(htmlTokStartTagName), 4)
	if got := portScan(sc, cFaithfulPayload(sc), "\u0085div", valid); got.ok {
		t.Errorf("U+0085 taken for white space: %+v", got)
	}
}

// TestHTMLSerializeLimitIs1024LikeC: a custom tag that would take the state to 1024 bytes ends
// the list (scanner.c:43-45, :51-53), with the written count in the first two bytes.
func TestHTMLSerializeLimitIs1024LikeC(t *testing.T) {
	sc := HTMLExternalScanner{}
	st := sc.Create().(*htmlScannerState)
	st.SetCFaithful(true)
	for i := 0; i < 5; i++ {
		st.ctags = append(st.ctags, htmlCTag{typ: htmlCCustom, name: make([]byte, 255)})
	}
	buf := make([]byte, 4096)
	n := sc.Serialize(st, buf)
	// 4 + 3*(2+255) = 775; the fourth tag would end at 1032 >= 1024
	if n != 775 || buf[0] != 3 || buf[2] != 5 {
		t.Errorf("state length %d written %d held %d, want 775, 3, 5", n, buf[0], buf[2])
	}
	back := sc.Create().(*htmlScannerState)
	back.SetCFaithful(true)
	sc.Deserialize(back, buf[:n])
	if len(back.ctags) != 5 || back.ctags[3].typ != htmlCEnd {
		t.Errorf("deserialized %d tags, tag 3 type %d, want 5 tags and END_ padding", len(back.ctags), back.ctags[3].typ)
	}
}
