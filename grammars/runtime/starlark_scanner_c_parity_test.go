//go:build !grammar_subset || grammar_subset_starlark

package grammarruntime

import (
	"bytes"
	"testing"
)

// TestStarlarkStateBytesMatchPinnedC pins the starlark scanner state layout at
// the pinned commit (src/scanner.c:363-413): flag byte, delimiter count,
// delimiters, then ONE byte per indent (the low byte), unlike Python's two.
func TestStarlarkStateBytesMatchPinnedC(t *testing.T) {
	scanner := StarlarkExternalScanner{}
	state := &pythonScannerState{
		Indents:                  []uint16{0, 4, 8},
		Delimiters:               []pyDelimiter{pyDelimDoubleQuote | pyDelimFormat},
		InsideInterpolatedString: true,
	}
	state.SetCFaithful(true) // the layout of C is the C-faithful mode
	buf := make([]byte, 1024)
	n := scanner.Serialize(state, buf)
	want := []byte{1, 1, byte(pyDelimDoubleQuote | pyDelimFormat), 4, 8}
	if !bytes.Equal(buf[:n], want) {
		t.Fatalf("bytes = %v, want %v", buf[:n], want)
	}
	var back pythonScannerState
	back.SetCFaithful(true)
	scanner.Deserialize(&back, buf[:n])
	if !back.InsideInterpolatedString || len(back.Delimiters) != 1 || len(back.Indents) != 3 || back.Indents[2] != 8 {
		t.Fatalf("round trip = %+v", back)
	}
}

// TestStarlarkNulIsNotTheEndLikeC: with
// indents [0,4], input "\x00x" and only DEDENT valid, C returns false (a NUL is
// not the end of the input); the legacy mode dedents.
func TestStarlarkNulIsNotTheEndLikeC(t *testing.T) {
	scanner := StarlarkExternalScanner{}
	valid := make([]bool, slTokenCount)
	valid[slTokDedent] = true
	for _, cf := range []bool{true, false} {
		st := scanner.Create().(*pythonScannerState)
		st.SetCFaithful(cf)
		st.Indents = []uint16{0, 4}
		got := portScan(scanner, st, "\x00x", valid)
		if cf && got.ok {
			t.Errorf("C-faithful: dedent at a NUL byte, got %+v", got)
		}
		if !cf && !got.ok {
			t.Errorf("legacy: expected the NUL read as the end, got %+v", got)
		}
	}
}
