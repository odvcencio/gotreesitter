package gotreesitter

import "testing"

func TestIncrementalReuseBudgetArmsOnlyBelowFullRetryCap(t *testing.T) {
	if incrementalReuseBudgetArmed(0) {
		t.Fatal("empty source armed the reuse budget")
	}
	if !incrementalReuseBudgetArmed(fullParseRetryMaxSourceBytes) {
		t.Fatal("source at the full-retry cap did not arm the reuse budget")
	}
	if incrementalReuseBudgetArmed(fullParseRetryMaxSourceBytes + 1) {
		t.Fatal("source above the full-retry cap armed the reuse budget without a rescue path")
	}
}
