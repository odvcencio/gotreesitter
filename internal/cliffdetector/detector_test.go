package cliffdetector

import "testing"

func TestFailuresAtBoundary(t *testing.T) {
	c := Frontier{MaxLive: 4, MultiShare: 0.2, Measured: true}
	if got := Failures(Frontier{MaxLive: 10, MultiShare: 0.35, Measured: true}, c); len(got) != 0 {
		t.Fatalf("boundary failed: %v", got)
	}
	if got := Failures(Frontier{MaxLive: 11, MultiShare: 0.36, Measured: true}, c); len(got) != 2 {
		t.Fatalf("both excesses should fail: %v", got)
	}
	if got := Failures(Frontier{}, c); len(got) != 0 {
		t.Fatalf("declined route failed: %v", got)
	}
}
