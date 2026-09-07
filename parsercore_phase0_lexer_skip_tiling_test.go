//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestCompactLexerSkippedPrefixTilingRequiresAlignedChildProof(t *testing.T) {
	source := []byte("a^b")
	left := &Node{startByte: 0, endByte: 1}
	right := &Node{startByte: 2, endByte: 3}
	entries := []stackEntry{newStackEntryNode(0, left), newStackEntryNode(0, right)}
	childIDs := []core.SubtreeID{1, 2}
	nodesByID := []*Node{nil, left, right, right}
	coverage := &diagnosticParserCoreAcceptedLeafCoverageScratch{
		leadingLexerSkippedPrefixStarts: []uint32{0, 0, 2, 2},
	}

	if gapStart, gapEnd, gapped := diagnosticParserCoreReduceChildrenTilingGapWithLexerProvenance(
		0, 3, entries, childIDs, source, coverage, nodesByID, true,
	); gapped {
		t.Fatalf("exact aligned prefix produced gap=%d..%d", gapStart, gapEnd)
	}

	tests := []struct {
		name      string
		ids       []core.SubtreeID
		proofs    []uint32
		certified bool
	}{
		{name: "uncertified", ids: childIDs, proofs: []uint32{0, 0, 2}, certified: false},
		{name: "wrong_prefix_start", ids: childIDs, proofs: []uint32{0, 0, 1}, certified: true},
		{name: "proof_on_unrelated_subtree", ids: childIDs, proofs: []uint32{0, 0, 0, 2}, certified: true},
		{name: "misaligned_child_id", ids: []core.SubtreeID{1, 3}, proofs: []uint32{0, 0, 2, 0}, certified: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coverage.leadingLexerSkippedPrefixStarts = test.proofs
			gapStart, gapEnd, gapped := diagnosticParserCoreReduceChildrenTilingGapWithLexerProvenance(
				0, 3, entries, test.ids, source, coverage, nodesByID, test.certified,
			)
			if !gapped || gapStart != 1 || gapEnd != 2 {
				t.Fatalf("gap=%d..%d gapped=%t, want 1..2 true", gapStart, gapEnd, gapped)
			}
		})
	}
}

func TestCompactLexerSkippedPrefixLengthFailsClosedWhenUnbounded(t *testing.T) {
	bounded := Token{StartByte: 65535, lexFlags: tokenFlagSkippedPrefix, lexerSkippedPrefixStart: 0}
	if got := diagnosticParserCoreLexerSkippedPrefixLength(bounded, true); got != 65535 {
		t.Fatalf("bounded prefix length=%d, want 65535", got)
	}
	unbounded := Token{StartByte: 65536, lexFlags: tokenFlagSkippedPrefix, lexerSkippedPrefixStart: 0}
	if got := diagnosticParserCoreLexerSkippedPrefixLength(unbounded, true); got != 0 {
		t.Fatalf("unbounded prefix length=%d, want fail-closed zero", got)
	}
	if got := diagnosticParserCoreLexerSkippedPrefixLength(bounded, false); got != 0 {
		t.Fatalf("uncertified prefix length=%d, want zero", got)
	}
}
