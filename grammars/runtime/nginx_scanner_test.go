//go:build !grammar_subset || grammar_subset_nginx

package grammarruntime

import (
	"reflect"
	"testing"
)

func TestNginxSerializeDeserializeRoundTripWideIndent(t *testing.T) {
	scanner := NginxExternalScanner{}
	state := scanner.Create().(*nginxScannerState)
	state.indents = []uint16{0, 128, 300, 1024}

	buf := make([]byte, 64)
	n := scanner.Serialize(state, buf)
	if n != 6 {
		t.Fatalf("Serialize size = %d, want 6", n)
	}

	restored := scanner.Create().(*nginxScannerState)
	scanner.Deserialize(restored, buf[:n])
	if !reflect.DeepEqual(restored.indents, state.indents) {
		t.Fatalf("Deserialize round-trip indents = %v, want %v", restored.indents, state.indents)
	}
}

func TestNginxDeserializeLegacySingleByteState(t *testing.T) {
	scanner := NginxExternalScanner{}
	state := scanner.Create().(*nginxScannerState)

	legacy := []byte{2, 10, 255}
	scanner.Deserialize(state, legacy)

	want := []uint16{0, 2, 10, 255}
	if !reflect.DeepEqual(state.indents, want) {
		t.Fatalf("Deserialize legacy indents = %v, want %v", state.indents, want)
	}
}
