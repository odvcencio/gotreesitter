package pythonruntime

import "testing"

func TestPythonScannerSerializationRecomputesInterpolatedStringState(t *testing.T) {
	scanner := PythonExternalScanner{}
	buf := make([]byte, 256)

	formatted := &State{
		Indents:                  []uint16{0},
		Delimiters:               []Delimiter{DelimiterDoubleQuote | DelimiterFormat},
		InsideInterpolatedString: false,
	}
	n := scanner.Serialize(formatted, buf)
	var restoredFormatted State
	scanner.Deserialize(&restoredFormatted, buf[:n])
	if !restoredFormatted.InsideInterpolatedString {
		t.Fatal("f-string checkpoint restored InsideInterpolatedString=false, want true")
	}

	// Reuse the same buffer after a formatted checkpoint. A false flag must
	// overwrite the old byte, or later reuse gates see a stale f-string state.
	plain := &State{
		Indents:                  []uint16{0},
		Delimiters:               []Delimiter{DelimiterDoubleQuote},
		InsideInterpolatedString: true,
	}
	n = scanner.Serialize(plain, buf)
	if n == 0 || buf[0] != 0 {
		t.Fatalf("plain checkpoint flag=%d, want 0 after formatted serialization", buf[0])
	}
	var restoredPlain State
	scanner.Deserialize(&restoredPlain, buf[:n])
	if restoredPlain.InsideInterpolatedString {
		t.Fatal("plain string checkpoint restored InsideInterpolatedString=true, want false")
	}
}

func TestPythonScannerSerializationFailsClosed(t *testing.T) {
	scanner := PythonExternalScanner{}

	state := &State{
		Indents:    []uint16{0, 4, 8, 12},
		Delimiters: []Delimiter{DelimiterSingleQuote | DelimiterFormat},
	}
	full := make([]byte, 256)
	n := scanner.Serialize(state, full)
	if n == 0 {
		t.Fatal("representable scanner state did not serialize")
	}
	if got, want := n, 4+1+3*2; got != want {
		t.Fatalf("serialized checkpoint length=%d, want %d", got, want)
	}

	short := make([]byte, n-1)
	for i := range short {
		short[i] = 0xA5
	}
	if got := scanner.Serialize(state, short); got != 0 {
		t.Fatalf("undersized checkpoint serialized %d bytes, want 0", got)
	}
	for i, b := range short {
		if b != 0xA5 {
			t.Fatalf("undersized checkpoint wrote byte %d before rejecting", i)
		}
	}

	var partial State
	partial.Delimiters = []Delimiter{DelimiterDoubleQuote}
	partial.Indents = []uint16{0, 20}
	scanner.Deserialize(&partial, full[:n-1])
	if len(partial.Delimiters) != 0 || len(partial.Indents) != 1 || partial.Indents[0] != 0 || partial.InsideInterpolatedString {
		t.Fatalf("partial checkpoint restored scanner state: %+v", partial)
	}

	malformedFlag := append([]byte(nil), full[:n]...)
	malformedFlag[0] = 0
	var inconsistent State
	scanner.Deserialize(&inconsistent, malformedFlag)
	if len(inconsistent.Delimiters) != 0 || len(inconsistent.Indents) != 1 || inconsistent.Indents[0] != 0 || inconsistent.InsideInterpolatedString {
		t.Fatalf("inconsistent checkpoint restored scanner state: %+v", inconsistent)
	}

	malformedTrailing := append(append([]byte(nil), full[:n]...), 0x00)
	var trailing State
	scanner.Deserialize(&trailing, malformedTrailing)
	if len(trailing.Delimiters) != 0 || len(trailing.Indents) != 1 || trailing.Indents[0] != 0 || trailing.InsideInterpolatedString {
		t.Fatalf("trailing checkpoint bytes restored scanner state: %+v", trailing)
	}

	// Checkpoint bytes are ephemeral and version-local. Reject the former
	// delimiter-only shape instead of treating it as a complete state.
	legacyShape := []byte{1, 1, byte(DelimiterSingleQuote | DelimiterFormat)}
	var legacy State
	scanner.Deserialize(&legacy, legacyShape)
	if len(legacy.Delimiters) != 0 || len(legacy.Indents) != 1 || legacy.Indents[0] != 0 || legacy.InsideInterpolatedString {
		t.Fatalf("legacy checkpoint shape restored scanner state: %+v", legacy)
	}

	tooManyDelimiters := &State{Delimiters: make([]Delimiter, maxPythonScannerDelimiterCount+1)}
	if got := scanner.Serialize(tooManyDelimiters, make([]byte, 4096)); got != 0 {
		t.Fatalf("unrepresentable delimiter stack serialized %d bytes, want 0", got)
	}

	// A deeply nested indentation stack needs more than the runtime checkpoint
	// buffer. The serializer must reject the complete state rather than publish
	// a prefix that could restore a shallower stack.
	deep := &State{Indents: make([]uint16, 3000)}
	if got := scanner.Serialize(deep, make([]byte, 4096)); got != 0 {
		t.Fatalf("deep indentation stack serialized %d bytes, want fail-closed 0", got)
	}
}
