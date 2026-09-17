package grammarruntime

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

var (
	_ gts.IncrementalReuseExternalScanner          = MarkdownExternalScanner{}
	_ gts.ErrorTreeIncrementalReuseExternalScanner = MarkdownExternalScanner{}
	_ gts.CheckpointedExternalScanner              = MarkdownExternalScanner{}
)

func TestMarkdownScannerCheckpointRoundTrip(t *testing.T) {
	scanner := MarkdownExternalScanner{}
	original := &mdState{
		openBlocks: []mdBlock{
			mdBlockQuote,
			mdListItem3Indent,
			mdFencedCodeBlock,
			mdAnonymous,
		},
		state:                          mdStateMatching | mdStateWasSoftLineBreak | mdStateCloseBlock,
		matched:                        2,
		indentation:                    3,
		column:                         1,
		fencedCodeBlockDelimiterLength: 5,
		simulate:                       true,
	}
	buf := make([]byte, 64)
	n := scanner.Serialize(original, buf)
	if n != 5+len(original.openBlocks) {
		t.Fatalf("serialized size = %d, want %d", n, 5+len(original.openBlocks))
	}
	if n == 0 {
		t.Fatal("reachable Markdown scanner state serialized as absent")
	}

	restored := scanner.Create().(*mdState)
	scanner.Deserialize(restored, buf[:n])
	roundTrip := make([]byte, 64)
	roundTripN := scanner.Serialize(restored, roundTrip)
	if roundTripN != n || !bytes.Equal(roundTrip[:roundTripN], buf[:n]) {
		t.Fatalf("checkpoint round trip = %v, want %v", roundTrip[:roundTripN], buf[:n])
	}
	if restored.simulate {
		t.Fatal("transient per-scan simulation flag survived checkpoint restore")
	}
}

func TestMarkdownScannerCheckpointCapacityFailsClosed(t *testing.T) {
	scanner := MarkdownExternalScanner{}
	state := &mdState{openBlocks: make([]mdBlock, 12)}
	exact := make([]byte, 5+len(state.openBlocks))
	if got := scanner.Serialize(state, exact); got != len(exact) {
		t.Fatalf("exact-capacity serialization = %d, want %d", got, len(exact))
	}
	if got := scanner.Serialize(state, exact[:len(exact)-1]); got != 0 {
		t.Fatalf("undersized serialization = %d, want absent checkpoint", got)
	}
}

func TestMarkdownScannerInitialCheckpointIsRepresentable(t *testing.T) {
	scanner := MarkdownExternalScanner{}
	state := scanner.Create()
	buf := make([]byte, 5)
	if got := scanner.Serialize(state, buf); got != 5 {
		t.Fatalf("initial checkpoint size = %d, want 5", got)
	}
	restored := scanner.Create()
	scanner.Deserialize(restored, buf)
	again := make([]byte, 5)
	if got := scanner.Serialize(restored, again); got != 5 || !bytes.Equal(again, buf) {
		t.Fatalf("initial checkpoint round trip = %v/%d, want %v/5", again, got, buf)
	}
}
