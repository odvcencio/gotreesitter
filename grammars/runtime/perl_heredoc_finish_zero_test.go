//go:build !grammar_subset || grammar_subset_perl

package grammarruntime

import (
	"bytes"
	"testing"
)

// TestPerlFinishHeredocZeroesStaleDelimiterBytes pins the fix ported from
// upstream tree-sitter-perl commit 71b727e ("fix: zero heredoc_delim on
// finish to prevent stale bytes in scanner state").
//
// Serialize writes every plMaxTSPStringLen content slot of heredocDelim
// regardless of its active length, so a delimiter's stale rune bytes
// surviving past finishHeredoc would leak into the serialized scanner
// payload. gotreesitter's GLR engine byte-compares that payload for state
// merging (external_scanner_checkpoint_capability.go,
// parser_dfa_token_source.go), so stale bytes could stop two logically
// identical scanner states from comparing equal.
//
// This drives a scanner state through a heredoc whose delimiter is longer
// than plMaxTSPStringLen tracks meaningfully, finishes it, and asserts the
// serialized payload is byte-identical to a scanner that never saw a
// heredoc. Both states pass interpolate=false and indent=false to addHeredoc
// so those two flags start and stay at their zero value on both sides; the
// upstream fix does not touch them, so comparing them is not this test's
// concern.
func TestPerlFinishHeredocZeroesStaleDelimiterBytes(t *testing.T) {
	scanner := PerlExternalScanner{}

	fresh := &plState{}
	freshBuf := make([]byte, 256)
	freshN := scanner.Serialize(fresh, freshBuf)

	used := &plState{}
	var delim plTSPString
	for _, r := range "LONGDELIMITER" {
		delim.push(r)
	}
	used.addHeredoc(&delim, false, false)
	used.finishHeredoc()

	usedBuf := make([]byte, 256)
	usedN := scanner.Serialize(used, usedBuf)

	if freshN != usedN {
		t.Fatalf("serialized length differs: fresh=%d used=%d", freshN, usedN)
	}
	if !bytes.Equal(freshBuf[:freshN], usedBuf[:usedN]) {
		t.Fatalf("serialized payload differs after heredoc finish:\nfresh=%x\nused=%x", freshBuf[:freshN], usedBuf[:usedN])
	}
}
