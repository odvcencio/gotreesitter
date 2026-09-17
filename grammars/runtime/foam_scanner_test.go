//go:build !grammar_subset || grammar_subset_foam

package grammarruntime

import "testing"

func TestFoamBooleanKeywordDetection(t *testing.T) {
	if !foamIsBooleanKeyword("on") || !foamIsBooleanKeyword("off") || !foamIsBooleanKeyword("true") || !foamIsBooleanKeyword("false") {
		t.Fatal("expected canonical foam boolean keywords to be detected")
	}
	if foamIsBooleanKeyword("only") || foamIsBooleanKeyword("offload") || foamIsBooleanKeyword("trueValue") {
		t.Fatal("unexpected boolean keyword detection for identifier prefixes")
	}
}

func TestFoamIdentifierTerminationBoundary(t *testing.T) {
	if !foamWouldTerminateIdentifier(0, 0) {
		t.Fatal("expected EOF to terminate identifier")
	}
	if !foamWouldTerminateIdentifier(' ', 0) {
		t.Fatal("expected whitespace to terminate identifier")
	}
	if !foamWouldTerminateIdentifier(')', 0) {
		t.Fatal("expected unmatched ')' to terminate identifier")
	}
	if foamWouldTerminateIdentifier('x', 0) {
		t.Fatal("did not expect identifier rune to terminate identifier")
	}
	if foamWouldTerminateIdentifier(' ', 1) {
		t.Fatal("did not expect whitespace to terminate nested identifier")
	}
}
