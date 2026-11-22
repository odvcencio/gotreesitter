//go:build !grammar_subset || grammar_subset_cue

package grammarruntime

import "testing"

// Reproducers from the pinned C scanner (eonpatapon/tree-sitter-cue be0f609c,
// src/scanner.c). The expected results follow the C code, not the old port.
// Masks list the externals in order: multi_str, multi_bytes, raw_str, raw_bytes,
// multi_raw_str, multi_raw_bytes.
func TestCueScannerCParityReproducers(t *testing.T) {
	type tc struct {
		name, src, mask string
		found           bool
		end             uint32
	}
	for _, c := range []tc{
		// :26-40 the other quote kind also marks the end and is not content: a
		// lone ' inside a """ string yields no token before the closing quotes
		// (no content has been counted), and the end is where the ' began.
		{"multi-other-quote-not-content", "'\"\"\"", "100000", false, 0},
		// :26-40 with content before it, the token ends at the other quote mark.
		{"multi-other-quote-ends-content", "ab'\"\"\"", "100000", true, 2},
		// :56-62 a NUL byte inside the content is content; only real end of input fails.
		{"multi-nul-is-content", "a\x00b\"\"\"", "100000", true, 3},
		{"multi-eof-fails", "ab", "100000", false, 0},
		// :81-98 raw multiline: either quote kind starts the closing test.
		{"rawmulti-other-quote", "a'\"\"\"#", "000010", false, 0},
		{"rawmulti-nul-is-content", "a\x00b\"\"\"#", "000010", true, 3},
		// :139-150 raw: either quote kind followed by '#' ends the content.
		{"raw-other-quote", "a'#", "001000", true, 1},
		{"raw-nul-is-content", "a\x00b\"#", "001000", true, 3},
		// :168-174 raw: a NUL byte right at the start is content, not the end.
		{"raw-nul-first", "\x00\"#", "001000", true, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := CueExternalScanner{}
			valid := make([]bool, len(c.mask))
			for i := range valid {
				valid[i] = c.mask[i] == '1'
			}
			lexer := newPortParityLexer([]byte(c.src), 0, 0, 0)
			portInstallObserver(lexer)
			ok := sc.Scan(nil, lexer, valid)
			r := portObserve(lexer, ok)
			if r.ok != c.found || (c.found && r.end != c.end) {
				t.Errorf("got found=%v end=%d; want found=%v end=%d", r.ok, r.end, c.found, c.end)
			}
		})
	}
}
