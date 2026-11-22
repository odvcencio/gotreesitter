//go:build !grammar_subset || grammar_subset_cpp

package grammarruntime

import "testing"

func TestCppScannerCheckpointRoundTrip(t *testing.T) {
	scanner := CppExternalScanner{}
	for _, delimiter := range []string{"", "a", "delimiter", "0123456789abcdef", "é世😀"} {
		t.Run(delimiter, func(t *testing.T) {
			state := &cppState{rawStringState: rawStringState{delimiter: []rune(delimiter)}}
			wire := make([]byte, len(delimiter)+1)
			if n := scanner.Serialize(state, wire); n != len(wire) {
				t.Fatalf("serialized=%d, want complete %d-byte checkpoint", n, len(wire))
			}
			restored := &cppState{rawStringState: rawStringState{delimiter: []rune("old")}}
			scanner.Deserialize(restored, wire)
			if string(restored.rawStringState.delimiter) != delimiter {
				t.Fatalf("restored=%q want %q", string(restored.rawStringState.delimiter), delimiter)
			}
			if scanner.Serialize(state, wire[:len(wire)-1]) != 0 {
				t.Fatal("short buffer produced an incomplete checkpoint")
			}
		})
	}
}
