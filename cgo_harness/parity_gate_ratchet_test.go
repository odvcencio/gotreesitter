//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

const (
	// Ratchets: these should only move in the "stricter" direction over time.
	minCuratedStructuralLanguages = 206
	minCuratedHighlightLanguages  = 200
	maxKnownDegradedStructural    = 14
	maxKnownDegradedNoErrorClean  = 1
	maxKnownDegradedHighlight     = 49
	maxParitySkips                = 0
)

// TestParityGateCoverageRatchet prevents silent narrowing of correctness gates.
// Update these thresholds only when intentionally tightening/loosening policy.
func TestParityGateCoverageRatchet(t *testing.T) {
	if got := len(curatedStructuralLanguages); got < minCuratedStructuralLanguages {
		t.Fatalf("curatedStructuralLanguages shrank: got=%d min=%d", got, minCuratedStructuralLanguages)
	}
	if got := len(curatedHighlightLanguages); got < minCuratedHighlightLanguages {
		t.Fatalf("curatedHighlightLanguages shrank: got=%d min=%d", got, minCuratedHighlightLanguages)
	}
	if got := len(knownDegradedStructural); got > maxKnownDegradedStructural {
		t.Fatalf("knownDegradedStructural grew: got=%d max=%d", got, maxKnownDegradedStructural)
	}
	if got := len(knownDegradedNoErrorClean); got > maxKnownDegradedNoErrorClean {
		t.Fatalf("knownDegradedNoErrorClean grew: got=%d max=%d", got, maxKnownDegradedNoErrorClean)
	}
	for name := range knownDegradedNoErrorClean {
		if _, ok := knownDegradedStructural[name]; !ok {
			t.Fatalf("knownDegradedNoErrorClean[%q] is not in knownDegradedStructural", name)
		}
	}
	if got := len(knownDegradedHighlight); got > maxKnownDegradedHighlight {
		t.Fatalf("knownDegradedHighlight grew: got=%d max=%d", got, maxKnownDegradedHighlight)
	}
	if got := len(paritySkips); got > maxParitySkips {
		t.Fatalf("paritySkips grew: got=%d max=%d", got, maxParitySkips)
	}
}
