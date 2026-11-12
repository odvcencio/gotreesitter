//go:build !grammar_subset || grammar_subset_bash

package grammarruntime

import (
	"bytes"
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
// (:859-880) marks the end, advances over ")" and sees white space at byte 1: it returns false
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

// TestBashNeverProducesOpeningParenOrEsac: C declares OPENING_PAREN and ESAC (:11-41) and never
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

// TestBashAdvanceWordAppendsLikeC: advance_word pushes onto the delimiter (:164-190) and
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
// TREE_SITTER_SERIALIZATION_BUFFER_SIZE, 1024 bytes (:101). Main refused only above the buffer.
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

// TestBashDeserializeKeepsExtraHeredocsLikeC: deserialize (:121-157) never shortens the heredoc
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
