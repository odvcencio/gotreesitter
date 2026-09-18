package gotreesitter

import "testing"

func TestIncrementalReuseBudgetCalculation(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		oldNodes, sourceLen, want int
	}{
		{"floor", 0, 1, 256 * 1024},
		{"old_tree", 100000, 1, 400000},
		{"source", 0, 137 * 1024, max(256*1024, 4*parseFullArenaInitialNodeCapacity(137*1024))},
		{"saturation", (1<<31-1)/4 + 1, 1, 1<<31 - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := &Tree{parseRuntime: ParseRuntime{NodesAllocated: tc.oldNodes}}
			if got := incrementalReuseNodeBudget(old, tc.sourceLen); got != tc.want {
				t.Fatalf("node budget = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestIncrementalReuseBudgetHostileThreshold(t *testing.T) {
	if incrementalReuseHostile(nil, 64) {
		t.Fatal("missing timing armed the reuse budget")
	}
	for _, reused := range []uint64{0, 7, 8, 9, 64} {
		timing := &incrementalParseTiming{reusedBytes: reused}
		if got, want := incrementalReuseHostile(timing, 64), reused < 8; got != want {
			t.Fatalf("reused=%d: hostile=%v, want %v", reused, got, want)
		}
	}
}

func TestIncrementalReuseBudgetPlainFullRetry(t *testing.T) {
	for _, stop := range []ParseStopReason{ParseStopReuseBudget, ParseStopMemoryBudget, ParseStopAccepted} {
		tree := &Tree{parseRuntime: ParseRuntime{StopReason: stop}}
		for _, size := range []int{0, 1, fullParseRetryMaxSourceBytes, fullParseRetryMaxSourceBytes + 1} {
			want := stop != ParseStopAccepted && size > 0 && size <= fullParseRetryMaxSourceBytes
			if got := shouldRetryIncrementalMemoryBudgetAsPlainFull(tree, size); got != want {
				t.Fatalf("stop=%s size=%d: retry=%v, want %v", stop, size, got, want)
			}
		}
	}
	if got := incrementalPlainFullRetryReason(ParseStopReuseBudget); got != "incremental_parse_reuse_budget_full_retry" {
		t.Fatalf("reuse retry reason = %q", got)
	}
}

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
