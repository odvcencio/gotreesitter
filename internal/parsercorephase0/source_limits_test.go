package parsercorephase0

import (
	"math"
	"testing"
)

func TestSourceRecordLimitsPreserveLiveBounds(t *testing.T) {
	base := reserveTestLimits()
	for _, n := range []int{-1, 0, 1, 32 << 10, 137 << 10, (1 << 20) - 1} {
		if got := SourceRecordLimits(base, n); got != base {
			t.Fatalf("source %d changed first-tier limits: %+v", n, got)
		}
	}
	got := SourceRecordLimits(base, 1<<20)
	if got.MaxNodes != base.MaxNodes*2 || got.MaxLinks != base.MaxLinks*2 ||
		got.MaxSubtrees != base.MaxSubtrees*2 || got.MaxChildren != base.MaxChildren*2 || got.MaxMetadata != base.MaxMetadata*2 {
		t.Fatalf("record capacity did not scale: %+v", got)
	}
	if got.MaxLinksPerBoundary != base.MaxLinksPerBoundary || got.MaxPopPaths != base.MaxPopPaths || got.MaxDerivations != base.MaxDerivations {
		t.Fatalf("live work bounds changed: %+v", got)
	}
	large := SourceRecordLimits(base, math.MaxInt)
	if large.MaxNodes != math.MaxUint32 || large.MaxLinks != math.MaxUint32 || large.MaxChildren != math.MaxUint32 {
		t.Fatalf("large-source capacities wrapped: %+v", large)
	}
}
