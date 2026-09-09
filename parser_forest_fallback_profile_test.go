package gotreesitter

import (
	"testing"
	"time"
)

func TestProfileForestRecoveryFallbackIncludesBothAttempts(t *testing.T) {
	before := IncrementalParseProfile{
		ReparseNanos: 40, TokensConsumed: 31, NewNodesAllocated: 23,
		ReusedSubtrees: 2, ReusedBytes: 100, OldTreeReuseRoute: true,
	}
	fallback := &Tree{parseRuntime: ParseRuntime{
		StopReason: ParseStopAccepted, TokensConsumed: 11, NodesAllocated: 7,
	}}
	got := profileForestRecoveryFallback(before, fallback, 13*time.Nanosecond)
	if got.ReparseNanos != 53 || got.TokensConsumed != 42 || got.NewNodesAllocated != 30 {
		t.Fatalf("both attempts must contribute time and work: %+v", got)
	}
	if !got.ReuseUnsupported || got.ReuseUnsupportedReason != forestRecoveryFallbackReuseReason || got.OldTreeReuseRoute || got.ReusedBytes != 0 || got.ReusedSubtrees != 0 {
		t.Fatalf("fallback must not claim incremental reuse: %+v", got)
	}
	if got.StopReason != ParseStopAccepted {
		t.Fatalf("stop=%s", got.StopReason)
	}
	unchanged := profileForestRecoveryFallback(before, nil, 13*time.Nanosecond)
	if unchanged != before {
		t.Fatal("nil fallback changed the existing profile")
	}
}
