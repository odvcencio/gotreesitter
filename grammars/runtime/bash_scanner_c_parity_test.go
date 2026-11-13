//go:build !grammar_subset || grammar_subset_bash

package grammarruntime

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Each test runs the C-faithful mode (cFaithfulPayload) and, where main's scan differs,
// pins main's behaviour with the mode off. C references are src/scanner.c of the pinned
// tree-sitter-bash commit.

func bashValid(toks ...int) []bool {
	v := make([]bool, bshTokenCount)
	for _, t := range toks {
		v[t] = true
	}
	return v
}

// TestBashExtglobReadsNoByteBeyondC: ")\n" with TEST_OPERATOR, EXTGLOB_PATTERN and BRACE_START valid (the reproducer of the 1,908 differences). C
// (:879-884) marks the end, advances over ")" and sees white space at byte 1: it returns false
// having examined bytes 0-1 (lookahead end 2). Main also advanced over the newline.
func TestBashExtglobReadsNoByteBeyondC(t *testing.T) {
	sc := BashExternalScanner{}
	valid := bashValid(bshTokTestOperator, bshTokExtglobPattern, bshTokBraceStart)
	got := portScan(sc, cFaithfulPayload(sc), ")\n", valid)
	if got.ok || got.frontier != 2 {
		t.Errorf("mode on: ok=%v frontier=%d, want false and 2", got.ok, got.frontier)
	}
	off := portScan(sc, sc.Create(), ")\n", valid)
	if off.ok || off.frontier != 3 {
		t.Errorf("mode off (main): ok=%v frontier=%d, want false and 3", off.ok, off.frontier)
	}
}

// TestBashNulIsNotTheEndLikeC: EMPTY_VALUE (:431) tests lexer->eof, so a NUL byte in the
// middle of the input is not an empty value. Main took the NUL for the end.
func TestBashNulIsNotTheEndLikeC(t *testing.T) {
	sc := BashExternalScanner{}
	valid := bashValid(bshTokEmptyValue)
	if got := portScanAt(sc, cFaithfulPayload(sc), "/\x00", 1, valid); got.ok {
		t.Errorf("mode on: empty value at a NUL byte, got %+v", got)
	}
	if got := portScanAt(sc, sc.Create(), "/\x00", 1, valid); !got.ok {
		t.Errorf("mode off (main): expected the NUL read as the end, got %+v", got)
	}
	// the real end of the input is still an empty value
	portWantScan(t, "mode on, end", portScanAt(sc, cFaithfulPayload(sc), "/", 1, valid), true, gotSym(bshTokEmptyValue), 1)
}

func gotSym(tok int) gotreesitter.Symbol { return bshDefaultSymTable[tok] }

// TestBashWhiteSpaceIsTheCLocale: iswspace in the C locale is ASCII only (:431 and all
// others); U+2003 is not white space. Main used unicode.IsSpace.
func TestBashWhiteSpaceIsTheCLocale(t *testing.T) {
	sc := BashExternalScanner{}
	valid := bashValid(bshTokEmptyValue)
	if got := portScan(sc, cFaithfulPayload(sc), " ", valid); got.ok {
		t.Errorf("mode on: U+2003 taken for white space: %+v", got)
	}
	if got := portScan(sc, sc.Create(), " ", valid); !got.ok {
		t.Errorf("mode off (main): expected white space, got %+v", got)
	}
}

// TestBashNeverProducesOpeningParenOrEsac: C declares OPENING_PAREN and ESAC (:9-39) and never
// returns them. Main scans them.
func TestBashNeverProducesOpeningParenOrEsac(t *testing.T) {
	sc := BashExternalScanner{}
	for _, c := range []struct {
		src string
		tok int
	}{{"(", bshTokOpeningParen}, {"esac", bshTokEsac}} {
		valid := bashValid(c.tok)
		if got := portScan(sc, cFaithfulPayload(sc), c.src, valid); got.ok {
			t.Errorf("mode on: %q returned a token: %+v", c.src, got)
		}
		if got := portScan(sc, sc.Create(), c.src, valid); !got.ok {
			t.Errorf("mode off (main): %q returned no token", c.src)
		}
	}
}

// TestBashDelimiterIsSignedLikeC: the delimiter is a char array (:226-244) compared as an int
// with the lookahead; the byte 0xE9 is -23 and never equals U+00E9. Main compared unsigned.
func TestBashDelimiterIsSignedLikeC(t *testing.T) {
	sc := BashExternalScanner{}
	valid := bashValid(bshTokHeredocEnd)
	for _, on := range []bool{true, false} {
		st := sc.Create().(*bshState)
		st.SetCFaithful(on)
		st.heredocs = []bshHeredoc{{delimiter: []byte{0xE9, 0}}}
		got := portScan(sc, st, "é", valid)
		if on && got.ok {
			t.Errorf("mode on: matched a delimiter byte >= 0x80: %+v", got)
		}
		if !on && !got.ok {
			t.Errorf("mode off (main): expected a match")
		}
	}
}

// TestBashAdvanceWordAppendsLikeC: advance_word pushes onto the delimiter (:164-193) and
// scan_heredoc_start does not clear it first (:210-224). Main replaced it.
func TestBashAdvanceWordAppendsLikeC(t *testing.T) {
	sc := BashExternalScanner{}
	valid := bashValid(bshTokHeredocStart)
	for _, on := range []bool{true, false} {
		st := sc.Create().(*bshState)
		st.SetCFaithful(on)
		st.heredocs = []bshHeredoc{{delimiter: []byte("ab\x00")}}
		portScan(sc, st, "cd", valid)
		want := "cd\x00"
		if on {
			want = "ab\x00cd\x00"
		}
		if string(st.heredocs[0].delimiter) != want {
			t.Errorf("mode %v: delimiter %q, want %q", on, st.heredocs[0].delimiter, want)
		}
	}
}

// TestBashSerializeStopsAtC1024: serialize refuses a state that reaches
// TREE_SITTER_SERIALIZATION_BUFFER_SIZE, 1024 bytes (:103). Main refused only above the buffer.
func TestBashSerializeStopsAtC1024(t *testing.T) {
	sc := BashExternalScanner{}
	for _, on := range []bool{true, false} {
		st := sc.Create().(*bshState)
		st.SetCFaithful(on)
		st.heredocs = []bshHeredoc{{delimiter: make([]byte, 1013)}} // 4 + 3 + 4 + 1013 = 1024
		n := sc.Serialize(st, make([]byte, 4096))
		if on && n != 0 || !on && n != 1024 {
			t.Errorf("mode %v: Serialize = %d", on, n)
		}
		st.heredocs[0].delimiter = make([]byte, 1012)
		if n := sc.Serialize(st, make([]byte, 4096)); n != 1023 {
			t.Errorf("mode %v: Serialize of 1023 bytes = %d", on, n)
		}
	}
}

// TestBashDeserializeKeepsExtraHeredocsLikeC: deserialize (:121-155) never shortens the heredoc
// array, so a state with fewer heredocs leaves the older ones in the scanner. Main truncated.
func TestBashDeserializeKeepsExtraHeredocsLikeC(t *testing.T) {
	sc := BashExternalScanner{}
	two := &bshState{heredocs: []bshHeredoc{{delimiter: []byte("a\x00")}, {delimiter: []byte("b\x00")}}}
	buf := make([]byte, 256)
	n2 := sc.Serialize(two, buf)
	one := &bshState{heredocs: []bshHeredoc{{delimiter: []byte("c\x00")}}}
	n1 := sc.Serialize(one, make([]byte, 256))
	state1 := make([]byte, 256)
	sc.Serialize(one, state1)
	for _, on := range []bool{true, false} {
		st := sc.Create().(*bshState)
		st.SetCFaithful(on)
		sc.Deserialize(st, buf[:n2])
		sc.Deserialize(st, state1[:n1])
		want := 1
		if on {
			want = 2
		}
		if len(st.heredocs) != want {
			t.Errorf("mode %v: %d heredocs after the shorter state, want %d", on, len(st.heredocs), want)
		}
		if on && !bytes.Equal(st.heredocs[0].delimiter, []byte("c\x00")) {
			t.Errorf("first heredoc not replaced: %q", st.heredocs[0].delimiter)
		}
	}
}

// Truncated checkpoints can end in the heredoc flags or length, including in
// a later heredoc. The C-mode scanner must not slice past the supplied bytes.
func TestBashDeserializeCTruncatedCheckpoints(t *testing.T) {
	sc := BashExternalScanner{}
	cases := []struct {
		name  string
		count int
		state []byte
	}{
		{"single", 1, []byte{0, 0, 0, 1, 1, 1, 1, 3, 0, 0, 0, 'e', 'n', 'd'}},
		{"two", 2, []byte{7, 1, 0, 2, 1, 0, 1, 3, 0, 0, 0, 'e', 'n', 'd', 0, 1, 0, 2, 0, 0, 0, 'x', 'y'}},
	}
	for _, tc := range cases {
		for cut := 0; cut < len(tc.state); cut++ {
			t.Run(fmt.Sprintf("%s/prefix_%d", tc.name, cut), func(t *testing.T) {
				st := sc.Create().(*bshState)
				st.SetCFaithful(true)
				sc.Deserialize(st, tc.state[:cut:cut])
				if cut >= 4 && len(st.heredocs) != tc.count {
					t.Fatalf("got %d heredocs, want %d", len(st.heredocs), tc.count)
				}
				if cut >= 14 && !bytes.Equal(st.heredocs[0].delimiter, []byte("end")) {
					t.Fatalf("first delimiter = %q, want end", st.heredocs[0].delimiter)
				}
				if tc.count == 2 && cut >= 21 && !bytes.Equal(st.heredocs[1].delimiter, tc.state[21:cut]) {
					t.Fatalf("second delimiter = %q, want %q", st.heredocs[1].delimiter, tc.state[21:cut])
				}
			})
		}
	}
}

// Every complete checkpoint must retain its exact wire representation, from
// the shortest states through the largest state C's serializer accepts.
func TestBashDeserializeCValidCheckpointsRoundTrip(t *testing.T) {
	maximum := []byte{0, 0, 0, 1, 1, 1, 1, 0xf4, 0x03, 0, 0} // delimiter length 1012
	maximum = append(maximum, bytes.Repeat([]byte{'z'}, 1012)...)
	cases := []struct {
		name  string
		state []byte
	}{
		{"empty", []byte{0, 0, 0, 0}},
		{"empty delimiter", []byte{7, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0}},
		{"single heredoc", []byte{0, 0, 0, 1, 1, 1, 1, 3, 0, 0, 0, 'e', 'n', 'd'}},
		{"two heredocs", []byte{7, 1, 0, 2, 1, 0, 1, 3, 0, 0, 0, 'e', 'n', 'd', 0, 1, 0, 2, 0, 0, 0, 'x', 'y'}},
		{"maximum", maximum},
	}
	sc := BashExternalScanner{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := sc.Create().(*bshState)
			st.SetCFaithful(true)
			sc.Deserialize(st, tc.state)
			got := make([]byte, 1024)
			n := sc.Serialize(st, got)
			if !bytes.Equal(got[:n], tc.state) {
				t.Fatalf("round-trip mismatch: got %x, want %x", got[:n], tc.state)
			}
		})
	}
}

func TestBashDeserializeCOversizedDelimiterAndHeredocCount(t *testing.T) {
	sc := BashExternalScanner{}
	st := sc.Create().(*bshState)
	st.SetCFaithful(true)
	st.heredocs = []bshHeredoc{
		{delimiter: []byte("old first")},
		{isRaw: true, delimiter: []byte("old second")},
		{delimiter: []byte("kept")},
	}
	// The first length claims 2^32-1 bytes, but only "ab" remains. The
	// second heredoc is absent; the third predates this shorter checkpoint.
	checkpoint := []byte{0, 0, 0, 2, 1, 0, 1, 0xff, 0xff, 0xff, 0xff, 'a', 'b'}
	sc.Deserialize(st, checkpoint)
	if len(st.heredocs) != 3 || string(st.heredocs[0].delimiter) != "ab" ||
		len(st.heredocs[1].delimiter) != 0 || st.heredocs[1].isRaw ||
		string(st.heredocs[2].delimiter) != "kept" {
		t.Fatalf("unexpected heredocs after oversized length: %+v", st.heredocs)
	}

	// The one-byte count is bounded even if no heredoc header follows it.
	st = sc.Create().(*bshState)
	st.SetCFaithful(true)
	sc.Deserialize(st, []byte{0, 0, 0, 255})
	if len(st.heredocs) != 255 {
		t.Fatalf("got %d heredocs, want 255", len(st.heredocs))
	}
}
