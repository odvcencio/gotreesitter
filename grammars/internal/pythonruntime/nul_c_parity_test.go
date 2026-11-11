package pythonruntime

import (
	"testing"
	_ "unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

//go:linkname newNulLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newNulLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer

// TestPythonNulIsNotTheEndLikeC: with indents
// [0,4], input "\x00x" and only DEDENT valid, C does not take the NUL byte for
// the end of the input (lexer->eof is false, scanner.c:258) and returns false;
// the legacy mode takes the NUL for the end and dedents.
func TestPythonNulIsNotTheEndLikeC(t *testing.T) {
	scanner := PythonExternalScanner{}
	valid := make([]bool, pyTokenCount)
	valid[pyTokDedent] = true
	for _, cf := range []bool{true, false} {
		st := scanner.Create().(*State)
		st.SetCFaithful(cf)
		st.Indents = []uint16{0, 4}
		got := scanner.Scan(st, newNulLexer([]byte("\x00x"), 0, 0, 0), valid)
		if cf && got {
			t.Errorf("C-faithful: dedent at a NUL byte")
		}
		if !cf && !got {
			t.Errorf("legacy: expected the NUL read as the end (dedent)")
		}
	}
}

// TestPythonCModeScanCases pins three scan behaviours of the pinned C scanner in the
// C-faithful mode: a backslash before a NUL byte is not a line continuation
// (scanner.c:258: the end-of-input test is lexer->eof); a comment stops at a NUL
// byte, which is then skipped (scanner.c:243-246), so the cursor ends after it; and a
// dedent at the end of input does not depend on a format delimiter that is still on
// the stack when the stored interpolation flag is false (scanner.c:283).
func TestPythonCModeScanCases(t *testing.T) {
	scanner := PythonExternalScanner{}
	dedent := make([]bool, pyTokenCount)
	dedent[pyTokDedent] = true
	run := func(state []byte, src string) (bool, uint32) {
		st := scanner.Create().(*State)
		st.SetCFaithful(true)
		scanner.Deserialize(st, state)
		l := newNulLexer([]byte(src), 0, 0, 0)
		ok := scanner.Scan(st, l, dedent)
		return ok, l.Column()
	}
	indent4 := []byte{0, 0, 4, 0}
	if ok, _ := run(indent4, "\\\x00"); ok {
		t.Error("backslash before NUL: C declines")
	}
	if ok, col := run(indent4, "\n#\x00x"); !ok || col != 2 {
		t.Errorf("comment then NUL: ok=%v column=%d, want a dedent with the cursor at column 2", ok, col)
	}
	if ok, _ := run([]byte{0, 1, 0x12, 4, 0}, ""); !ok {
		t.Error("dedent at the end of input with a format delimiter and a false stored flag: C dedents")
	}
}

// TestPythonCModeSerializeSignalsAStateThatDoesNotFit: with an odd number of delimiters
// the indent loop of C ends at 1,025 bytes (scanner.c:383-389). The port returns that
// size and writes only inside the 1,024-byte buffer; the core stops a parse whose
// state is longer than the buffer with a named reason.
func TestPythonCModeSerializeSignalsAStateThatDoesNotFit(t *testing.T) {
	scanner := PythonExternalScanner{}
	st := scanner.Create().(*State)
	st.SetCFaithful(true)
	st.Delimiters = []Delimiter{DelimiterDoubleQuote}
	st.Indents = make([]uint16, 512)
	for i := range st.Indents {
		st.Indents[i] = uint16(i)
	}
	buf := make([]byte, 1024)
	if n := scanner.Serialize(st, buf); n != 1025 {
		t.Fatalf("size %d, want 1025 (C's size; above the 1,024-byte buffer)", n)
	}
	// An even header ends at 1,024 and fits.
	st.Delimiters = nil
	if n := scanner.Serialize(st, buf); n != 1024 {
		t.Fatalf("even header: size %d, want 1024", n)
	}
	// A state that fits is unchanged, and the legacy writer is not used.
	st.Indents = []uint16{0, 4}
	if n := scanner.Serialize(st, buf); n != 4 {
		t.Fatalf("small state: size %d, want 4", n)
	}
}
