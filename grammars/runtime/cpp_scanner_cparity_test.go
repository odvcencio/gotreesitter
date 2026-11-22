//go:build !grammar_subset || grammar_subset_cpp

package grammarruntime

import (
	"bytes"
	"testing"
)

func cppParityScan(sc CppExternalScanner, payload any, src string, valid []bool) portScanResult {
	lexer := newPortParityLexer([]byte(src), 0, 0, 0)
	portInstallObserver(lexer)
	ok := sc.Scan(payload, lexer, valid)
	return portObserve(lexer, ok)
}

func cppParityState(sc CppExternalScanner, payload any) []byte {
	buf := make([]byte, 1024)
	return buf[:sc.Serialize(payload, buf)]
}

// Reproducers from the pinned C scanner (tree-sitter-cpp c0092228, src/scanner.c).
// The expected results follow the C code, in the mode the core turns on.
func TestCppScannerCParityReproducers(t *testing.T) {
	sc := CppExternalScanner{}
	delim := []bool{true, false}
	content := []bool{false, true}
	type tc struct {
		name, src string
		valid     []bool
		state     []byte // state before the scan (C layout)
		found     bool
		end       uint32
		after     []byte // state after the scan (C layout)
	}
	le := func(cs ...rune) []byte {
		var b []byte
		for _, c := range cs {
			b = append(b, byte(c), byte(c>>8), byte(c>>16), byte(c>>24))
		}
		return b
	}
	for _, c := range []tc{
		// :122-129 four bytes per delimiter character, no format byte.
		{"state-layout", "ab(", delim, nil, true, 2, le('a', 'b')},
		// :44 only lexer->eof ends the delimiter; a NUL byte is a delimiter character.
		{"nul-in-delimiter", "a\x00b(", delim, nil, true, 3, le('a', 0, 'b')},
		// :45 iswspace is ASCII only in the C locale: U+00A0 and U+0085 are delimiter characters.
		{"nbsp-in-delimiter", "a (", delim, nil, true, 3, le('a', 0xa0)},
		{"nel-in-delimiter", "a\u0085(", delim, nil, true, 3, le('a', 0x85)},
		{"vt-ends-delimiter", "a\v(", delim, nil, false, 1, le('a')},
		// :43-55 a failed opening scan leaves the characters read so far in the state.
		{"failed-opening-keeps-partial", "ab c(", delim, nil, false, 2, le('a', 'b')},
		{"failed-at-eof-keeps-partial", "ab", delim, nil, false, 2, le('a', 'b')},
		{"empty-delimiter-fails-empty", "(", delim, nil, false, 0, nil},
		// :33-34 a failed closing scan changes nothing and does not reset the state.
		{"failed-closing-keeps-state", "ax", delim, le('a', 'b'), false, 1, le('a', 'b')},
		// :39 a matched closing delimiter resets the state.
		{"closing-resets", "ab\"", delim, le('a', 'b'), true, 2, nil},
		// :67 a NUL byte inside the content is text; the token ends before ")x".
		{"nul-in-content", "a\x00b)x\"", content, le('x'), true, 3, le('x')},
		// :67-68 the end of input ends the content.
		{"eof-ends-content", "ab", content, le('x'), true, 2, le('x')},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := cFaithfulPayload(sc)
			sc.Deserialize(p, c.state)
			r := cppParityScan(sc, p, c.src, c.valid)
			if r.ok != c.found {
				t.Fatalf("found=%v, want %v", r.ok, c.found)
			}
			if c.found && r.end != c.end {
				t.Errorf("end=%d, want %d", r.end, c.end)
			}
			if !c.found && r.pos != int(c.end) {
				t.Errorf("cursor=%d, want %d", r.pos, c.end)
			}
			if got := cppParityState(sc, p); !bytes.Equal(got, c.after) {
				t.Errorf("state=%x, want %x", got, c.after)
			}
		})
	}
	t.Run("deserialize-length", func(t *testing.T) {
		// :132-140 length / 4 characters are restored.
		p := cFaithfulPayload(sc)
		sc.Deserialize(p, le('a', 0x4e16, 0x1f600))
		if got := string(p.(*cppState).delimiter); got != "a世\U0001f600" {
			t.Errorf("delimiter=%q", got)
		}
		sc.Deserialize(p, nil)
		if len(p.(*cppState).delimiter) != 0 {
			t.Error("empty state not restored as empty")
		}
	})
}
