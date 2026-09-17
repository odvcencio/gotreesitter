package grammarruntime

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestSwiftExternalScannerCheckpointRoundTrip(t *testing.T) {
	scanner := SwiftExternalScanner{}
	checkpointed, ok := any(scanner).(interface {
		UsesExternalScannerCheckpoints() bool
	})
	if !ok || !checkpointed.UsesExternalScannerCheckpoints() {
		t.Fatal("Swift scanner does not advertise complete checkpoints")
	}

	original := &swtScannerState{
		ongoingRawStrHashCount: 17,
		carriedPreviousRune:    '\u03bb',
		carriedPreviousValid:   true,
	}
	buf := make([]byte, 9)
	if got := scanner.Serialize(original, buf); got != len(buf) {
		t.Fatalf("serialized checkpoint bytes = %d, want %d", got, len(buf))
	}

	restored := &swtScannerState{
		ongoingRawStrHashCount: 99,
		carriedPreviousRune:    'x',
		carriedPreviousValid:   true,
	}
	scanner.Deserialize(restored, buf)
	if *restored != *original {
		t.Fatalf("restored scanner state = %+v, want %+v", *restored, *original)
	}

	scanner.Deserialize(restored, nil)
	if *restored != (swtScannerState{}) {
		t.Fatalf("empty checkpoint did not clear scanner state: %+v", *restored)
	}
}

func TestSwiftExternalScannerFailedScanMutatesCarryCheckpoint(t *testing.T) {
	scanner := SwiftExternalScanner{}
	state := &swtScannerState{
		carriedPreviousRune:  'x',
		carriedPreviousValid: true,
	}
	start := make([]byte, 9)
	if got := scanner.Serialize(state, start); got != len(start) {
		t.Fatalf("start checkpoint bytes = %d, want %d", got, len(start))
	}

	if scanner.Scan(state, &gotreesitter.ExternalLexer{}, make([]bool, swtTokenCount)) {
		t.Fatal("empty-source scanner produced a token")
	}
	end := make([]byte, 9)
	if got := scanner.Serialize(state, end); got != len(end) {
		t.Fatalf("end checkpoint bytes = %d, want %d", got, len(end))
	}
	if bytes.Equal(start, end) {
		t.Fatalf("failed scan kept one checkpoint: start=%v end=%v", start, end)
	}
	if state.carriedPreviousValid {
		t.Fatalf("failed scan kept stale carry state: %+v", state)
	}
}
