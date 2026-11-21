//go:build !grammar_subset || grammar_subset_sql

package grammarruntime

import "testing"

// Reproducers from the pinned C scanner (m-novikov/tree-sitter-sql 587f30d1,
// src/scanner.cc). The expected results follow the C code, not the old port.
func TestSQLScannerCParityReproducers(t *testing.T) {
	const (
		tag     = 1 << sqlTokDollarTagStart
		content = 1 << sqlTokContent
		end     = 1 << sqlTokDollarTagEnd
	)
	type tc struct {
		name, src string
		valid     int
		state     *sqlScannerState
		pos       int
		cFaithful bool
		found     bool
		sym       int // external token index, when found
		endByte   uint32
		wantState string // Serialize after a found scan; "" skips the check
	}
	for _, c := range []tc{
		// :71 iswspace holds the vertical tab: it is skipped before the tag.
		{name: "vtab-is-space", src: "\v$$", valid: tag, state: &sqlScannerState{}, found: true, sym: sqlTokDollarTagStart, endByte: 3, wantState: "\x01$$"},
		// :71 a lone vertical tab is skipped and the scan then fails at the end of input.
		{name: "vtab-then-eof", src: "\v", valid: tag, state: &sqlScannerState{}, found: false},
		// :30-32 the saved state is the started byte and the tag, with no terminator.
		{name: "state-layout-start", src: "$ab$", valid: tag, state: &sqlScannerState{}, found: true, sym: sqlTokDollarTagStart, endByte: 4, wantState: "\x01$ab$"},
		// :40 any non-zero first byte means started; the tag is empty, and the end tag
		// matches with nothing read (:98-105). :120 started ends; the saved state is one byte.
		{name: "damaged-started-empty-tag-ends", src: "\n", pos: 1, valid: end, state: &sqlScannerState{started: true}, found: true, sym: sqlTokDollarTagEnd, endByte: 1, wantState: "\x00"},
		// :119-120 the end tag ends started and leaves the tag in the state.
		{name: "end-tag-keeps-tag", src: "$$", valid: end, state: &sqlScannerState{started: true, tag: "$$"}, found: true, sym: sqlTokDollarTagEnd, endByte: 2, wantState: "\x00$$"},
		// :102 the end tag compares chars: U+0124 keeps its low byte, '$'.
		{name: "end-tag-low-byte", src: "ĤĤ", valid: end, state: &sqlScannerState{started: true, tag: "$$"}, found: true, sym: sqlTokDollarTagEnd, endByte: 4},
		// :53 the tag is compared as signed chars: the byte 0xFF is -1 and does not equal U+00FF.
		{name: "content-signed-tag-byte", src: "ÿ", valid: content, state: &sqlScannerState{started: true, tag: "\xff"}, found: false},
		// :53, :65 an empty tag reads as NUL and never matches: the scan advances to the end.
		{name: "content-empty-tag-runs-on", src: "ab", valid: content, state: &sqlScannerState{started: true}, pos: 0, found: false},
		// :81 C reads only ASCII letters in a tag name; the default engine also takes
		// digits and underscores.
		{name: "tag-digit-c", src: "$a1$", valid: tag, state: &sqlScannerState{}, cFaithful: true, found: false},
		{name: "tag-underscore-c", src: "$_a$", valid: tag, state: &sqlScannerState{}, cFaithful: true, found: false},
		{name: "tag-digit-default", src: "$a1$", valid: tag, state: &sqlScannerState{}, found: true, sym: sqlTokDollarTagStart, endByte: 4},
		{name: "tag-letters-c", src: "$aB$", valid: tag, state: &sqlScannerState{}, cFaithful: true, found: true, sym: sqlTokDollarTagStart, endByte: 4, wantState: "\x01$aB$"},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := SqlExternalScanner{}
			state := *c.state
			state.cFaithful = c.cFaithful
			valid := make([]bool, sqlTokDollarTagEnd+1)
			for i := range valid {
				valid[i] = c.valid&(1<<i) != 0
			}
			r := portScanAt(sc, &state, c.src, c.pos, valid)
			if r.ok != c.found {
				t.Fatalf("found = %v, want %v", r.ok, c.found)
			}
			if !c.found {
				return
			}
			if want := SqlLanguage().ExternalSymbols[c.sym]; r.sym != want {
				t.Errorf("symbol = %d, want %d", r.sym, want)
			}
			if r.end != c.endByte {
				t.Errorf("token end = %d, want %d", r.end, c.endByte)
			}
			if c.wantState != "" {
				buf := make([]byte, 64)
				n := sc.Serialize(&state, buf)
				if got := string(buf[:n]); got != c.wantState {
					t.Errorf("state = %q, want %q", got, c.wantState)
				}
			}
		})
	}
}

// :40-41 Deserialize reads the started byte and then the whole rest as the tag.
func TestSQLScannerCParityDeserialize(t *testing.T) {
	sc := SqlExternalScanner{}
	for _, c := range []struct {
		state   string
		started bool
		tag     string
	}{
		{"", false, ""},
		{"\x00", false, ""},
		{"\xcb", true, ""},
		{"\x01$a$", true, "$a$"},
		{"\x00$a$", false, "$a$"},
		{"\xff\xff\xff\xff", true, "\xff\xff\xff"},
		{"\x01a\x00b", true, "a\x00b"},
	} {
		s := &sqlScannerState{started: true, tag: "stale"}
		sc.Deserialize(s, []byte(c.state))
		if s.started != c.started || s.tag != c.tag {
			t.Errorf("Deserialize(%q) = started %v tag %q, want started %v tag %q", c.state, s.started, s.tag, c.started, c.tag)
		}
	}
}
