//go:build gts_parsercorephase0

package gotreesitter

import "testing"

func TestParserCoreFreshFullRunnerCompactRuntimeOptInAndArenaReset(t *testing.T) {
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
	t.Setenv("GOT_PARSE_PHASE_TIMING", "0")

	runner, err := newParserCoreFreshFullRunner(
		parserCoreWarmGoScanner,
		parserCoreFreshFullCanonicalOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")

	disabledTree, err := runner.parse(fixture.Source)
	if err != nil {
		t.Fatalf("timing-disabled parse: %v", err)
	}
	if got, ok := disabledTree.CompactParserCoreRuntime(); ok || got != (CompactParserCoreRuntime{}) {
		disabledTree.Release()
		t.Fatalf("timing-disabled compact runtime = %+v/%t, want zero/false", got, ok)
	}
	disabledArena := disabledTree.arena
	if disabledArena == nil {
		disabledTree.Release()
		t.Fatal("timing-disabled compact tree has no arena")
	}
	disabledTree.Release()
	if disabledArena.compactRuntime != nil {
		t.Fatal("timing-disabled arena retained compact runtime after release")
	}

	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	ResetParseEnvConfigCacheForTests()
	enabledTree, err := runner.parse(fixture.Source)
	if err != nil {
		t.Fatalf("timing-enabled parse: %v", err)
	}
	runtimeReceipt, ok := enabledTree.CompactParserCoreRuntime()
	if !ok || !runtimeReceipt.Authenticated {
		enabledTree.Release()
		t.Fatalf("timing-enabled compact runtime = %+v/%t, want authenticated", runtimeReceipt, ok)
	}
	if runtimeReceipt.SchedulerNanos <= 0 || runtimeReceipt.MaterializationNanos <= 0 ||
		runtimeReceipt.SchedulerFootprintBytes == 0 || runtimeReceipt.RetainedFootprintBytes == 0 {
		enabledTree.Release()
		t.Fatalf("timing-enabled compact runtime lacks timing/footprint: %+v", runtimeReceipt)
	}
	if runtimeReceipt.SelectedNodes == 0 ||
		runtimeReceipt.SelectedParents+runtimeReceipt.SelectedLeaves != runtimeReceipt.SelectedNodes {
		enabledTree.Release()
		t.Fatalf("timing-enabled selected-node census is inconsistent: %+v", runtimeReceipt)
	}
	if runtimeReceipt.CoreWork.Shifts == 0 || runtimeReceipt.SchedulerWork.Dispatches == 0 ||
		runtimeReceipt.SchedulerWork.Accepts != 1 || runtimeReceipt.CoreStats.Nodes == 0 ||
		runtimeReceipt.CoreStats.CurrentExactPaths != 1 {
		enabledTree.Release()
		t.Fatalf("timing-enabled compact work/stats are incomplete: %+v", runtimeReceipt)
	}
	runtimeReceipt.Authenticated = false
	if reread, ok := enabledTree.CompactParserCoreRuntime(); !ok || !reread.Authenticated {
		enabledTree.Release()
		t.Fatalf("mutating accessor copy changed stored receipt: %+v/%t", reread, ok)
	}
	enabledArena := enabledTree.arena
	enabledTree.Release()
	if enabledArena.compactRuntime != nil {
		t.Fatal("released arena retained timing-enabled compact runtime")
	}

	t.Setenv("GOT_PARSE_PHASE_TIMING", "0")
	ResetParseEnvConfigCacheForTests()
	afterResetTree, err := runner.parse(fixture.Source)
	if err != nil {
		t.Fatalf("timing-disabled parse after reset: %v", err)
	}
	defer afterResetTree.Release()
	if got, ok := afterResetTree.CompactParserCoreRuntime(); ok || got != (CompactParserCoreRuntime{}) {
		t.Fatalf("timing-disabled compact runtime after arena reset = %+v/%t, want zero/false", got, ok)
	}
}
