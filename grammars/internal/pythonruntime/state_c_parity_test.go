package pythonruntime

import (
	"bytes"
	"slices"
	"testing"
)

// The tests below switch the C-faithful mode on (internal/scanmode); with the
// mode off the state keeps the legacy checkpoint layout (state_test.go).
//
// TestStateBytesMatchPinnedC pins the scanner state bytes of
// tree-sitter-python at the pinned commit (src/scanner.c:364-415). The layout
// is: flag byte, delimiter count, delimiters, then one little-endian 16-bit
// word per indent except the root sentinel. C writes no indent count word.
func TestStateBytesMatchPinnedC(t *testing.T) {
	dq := DelimiterDoubleQuote | DelimiterFormat
	cases := []struct {
		name  string
		state State
		want  []byte
	}{
		{"empty state", State{Indents: []uint16{0}}, []byte{0, 0}},
		{"one f-string", State{Indents: []uint16{0}, Delimiters: []Delimiter{dq}, InsideInterpolatedString: true}, []byte{1, 1, byte(dq)}},
		{"indents only", State{Indents: []uint16{0, 4, 300}}, []byte{0, 0, 4, 0, 0x2c, 0x01}},
		{
			"flag is stored, not derived from the delimiters (scanner.c:353 sets it from the newest delimiter only)",
			State{Indents: []uint16{0, 2}, Delimiters: []Delimiter{dq, DelimiterSingleQuote}, InsideInterpolatedString: false},
			[]byte{0, 2, byte(dq), byte(DelimiterSingleQuote), 2, 0},
		},
	}
	for _, c := range cases {
		buf := make([]byte, 1024)
		c.state.SetCFaithful(true)
		n := SerializeState(&c.state, buf)
		if !bytes.Equal(buf[:n], c.want) {
			t.Errorf("%s: bytes = %v, want %v", c.name, buf[:n], c.want)
		}
		var back State
		back.SetCFaithful(true)
		DeserializeState(&back, buf[:n])
		if back.InsideInterpolatedString != c.state.InsideInterpolatedString ||
			!slices.Equal(back.Delimiters, c.state.Delimiters) || !slices.Equal(back.Indents, c.state.Indents) {
			t.Errorf("%s: round trip = %+v, want %+v", c.name, back, c.state)
		}
	}
}

// TestStateIndentWordsStopAtBufferSize pins scanner.c:383: C stops adding
// indent words once the size reaches the 1024-byte serialization buffer.
func TestStateIndentWordsStopAtBufferSize(t *testing.T) {
	s := State{Indents: make([]uint16, 1000)}
	s.SetCFaithful(true)
	buf := make([]byte, 4096)
	if n := SerializeState(&s, buf); n != 1024 {
		t.Fatalf("size = %d, want 1024 (2 header bytes plus 511 indent words)", n)
	}
}

// TestStateDeserializeReadsLikeC pins scanner.c:392-415: the reader takes
// indent words while two bytes remain, so a trailing odd byte is ignored, and
// the flag byte is read as stored.
func TestStateDeserializeReadsLikeC(t *testing.T) {
	var s State
	s.SetCFaithful(true)
	DeserializeState(&s, []byte{1, 1, byte(DelimiterSingleQuote), 4, 0, 9})
	if !s.InsideInterpolatedString || len(s.Delimiters) != 1 || len(s.Indents) != 2 || s.Indents[1] != 4 {
		t.Fatalf("state = %+v, want flag true, one delimiter, indents [0 4]", s)
	}
	// An empty snapshot clears the stacks but keeps the string flag, as C does
	// (python scanner.c:395-402; mojo :420-427; starlark :392-399).
	DeserializeState(&s, nil)
	if !s.InsideInterpolatedString || len(s.Delimiters) != 0 || len(s.Indents) != 1 {
		t.Fatalf("empty buffer: want the stacks cleared and the flag kept, got %+v", s)
	}
	buf := make([]byte, 16)
	if n := SerializeState(&s, buf); n != 2 || buf[0] != 1 {
		t.Fatalf("saved after an empty snapshot: %v, want flag 1 and no delimiters", buf[:n])
	}
}
