//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Tranche B8 gate: for each stop-control class (memory budget, timeout,
// cancellation), an admission-eligible input that trips the class produces a
// stop receipt from the candidate route (routed through the scheduler's
// pollStopControl, parsercore_phase0_driver.go) that matches what production
// itself reports for the identical input and configuration: the same
// ParseStopReason, ParseStoppedEarly() true, and a tree with no out-of-range
// partial state. Every witness here stays modest in size deliberately, not
// because a size boundary still gates eligibility (tranche B9 removed it):
// these tests isolate the stop-control mechanism itself, so keeping the
// witnesses small keeps them fast without changing what they prove.

// stopControlWitnessGoSource returns a small, valid Go source with `lines`
// statements.
func stopControlWitnessGoSource(lines int) []byte {
	var buf bytes.Buffer
	buf.WriteString("package p\n\nfunc f() {\n")
	for i := 0; i < lines; i++ {
		buf.WriteString("\tvar x = 1\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes()
}

// requireStopControlWitnessUnderWall keeps these deliberately modest witnesses
// well clear of ParseRuntimeMemoryMinSourceBytesForTest, the source-length
// floor where production's own runtime-heap watchdog additionally arms
// (parser_memory_budget_runtime.go). That floor no longer gates compact-route
// eligibility (tranche B9); staying under it here is only about keeping these
// witnesses small and fast, not about avoiding an eligibility decline.
func requireStopControlWitnessUnderWall(t *testing.T, source []byte) {
	t.Helper()
	if wall := gts.ParseRuntimeMemoryMinSourceBytesForTest(); len(source) >= wall {
		t.Fatalf("witness source is %d bytes, want < %d (keep these stop-control witnesses modest and fast)", len(source), wall)
	}
}

// requireSaneStoppedTree checks the partial-state half of the gate: whatever
// the stopped tree contains, it must not claim byte coverage past the source
// it was built from.
func requireSaneStoppedTree(t *testing.T, tree *gts.Tree, source []byte, label string) {
	t.Helper()
	if tree == nil {
		t.Fatalf("%s: nil tree", label)
	}
	if !tree.ParseStoppedEarly() {
		t.Fatalf("%s: ParseStoppedEarly() = false, want true", label)
	}
	if root := tree.RootNode(); root != nil && root.EndByte() > uint32(len(source)) {
		t.Fatalf("%s: partial tree root end %d exceeds source length %d", label, root.EndByte(), len(source))
	}
}

// TestAdmissionSwitchCompactTimeoutStopReceiptMatchesProduction proves the
// timeout class: the candidate route is eligible (tranche B8 removed the
// outright timeout decline), the scheduler's poll detects the deadline
// already expired, cleanly aborts and falls back, and production's own
// receipt for the identical input carries the same ParseStopReason.
func TestAdmissionSwitchCompactTimeoutStopReceiptMatchesProduction(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)

	source := stopControlWitnessGoSource(50)
	requireStopControlWitnessUnderWall(t, source)

	// Production reference: pin production, expire the deadline before Parse
	// runs (the same technique TestParseWithSnippetParserInheritsExpiredParentDeadline
	// uses internally): BeginParseOperationBudgetForTest opens the same outer
	// budget scope Parse opens, computing the deadline now, then the sleep
	// pushes it into the past before Parse's own nested scope reads it.
	prod := gts.NewParser(grammars.GoLanguage())
	prod.SetAdmissionCandidateRoute(false)
	prod.SetTimeoutMicros(50)
	endProdBudget := prod.BeginParseOperationBudgetForTest()
	time.Sleep(2 * time.Millisecond)
	prodTree, err := prod.Parse(source)
	endProdBudget()
	if err != nil {
		t.Fatalf("production reference parse: %v", err)
	}
	defer prodTree.Release()
	if got, want := prodTree.ParseStopReason(), gts.ParseStopTimeout; got != want {
		t.Fatalf("production reference ParseStopReason() = %q, want %q", got, want)
	}

	// Candidate route: identical construction and expiry technique, default
	// routing (compact eligible; nothing else declines this input).
	gts.ResetAdmissionCandidateCountersForTest()
	candidate := gts.NewParser(grammars.GoLanguage())
	candidate.SetTimeoutMicros(50)
	endCandidateBudget := candidate.BeginParseOperationBudgetForTest()
	time.Sleep(2 * time.Millisecond)
	tree, err := candidate.Parse(source)
	endCandidateBudget()
	if err != nil {
		t.Fatalf("candidate-route parse: %v", err)
	}
	defer tree.Release()

	if got, want := tree.ParseStopReason(), gts.ParseStopTimeout; got != want {
		t.Fatalf("candidate-route ParseStopReason() = %q, want %q (production reference agrees on %q)", got, want, prodTree.ParseStopReason())
	}
	requireSaneStoppedTree(t, tree, source, "timeout")

	routed, fallback := gts.AdmissionCandidateCounters()
	if fallback != 1 || routed != 0 {
		t.Fatalf("expected exactly one stop-control fallback and zero routed: routed=%d fallback=%d", routed, fallback)
	}
	if reason := gts.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "stop-control tripped: timeout") {
		t.Fatalf("fallback reason = %q, want it to name the scheduler's timeout trip", reason)
	}
}

// TestAdmissionSwitchCompactCancellationStopReceiptMatchesProduction is the
// cancellation counterpart: the flag is already tripped before Parse is
// called, so the scheduler's very first poll (before its first election)
// catches it deterministically -- no elapsed-time technique needed.
func TestAdmissionSwitchCompactCancellationStopReceiptMatchesProduction(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)

	source := stopControlWitnessGoSource(50)
	requireStopControlWitnessUnderWall(t, source)

	prod := gts.NewParser(grammars.GoLanguage())
	prod.SetAdmissionCandidateRoute(false)
	var prodFlag uint32 = 1
	prod.SetCancellationFlag(&prodFlag)
	prodTree, err := prod.Parse(source)
	if err != nil {
		t.Fatalf("production reference parse: %v", err)
	}
	defer prodTree.Release()
	if got, want := prodTree.ParseStopReason(), gts.ParseStopCancelled; got != want {
		t.Fatalf("production reference ParseStopReason() = %q, want %q", got, want)
	}

	gts.ResetAdmissionCandidateCountersForTest()
	candidate := gts.NewParser(grammars.GoLanguage())
	var flag uint32 = 1
	candidate.SetCancellationFlag(&flag)
	tree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate-route parse: %v", err)
	}
	defer tree.Release()

	if got, want := tree.ParseStopReason(), gts.ParseStopCancelled; got != want {
		t.Fatalf("candidate-route ParseStopReason() = %q, want %q (production reference agrees on %q)", got, want, prodTree.ParseStopReason())
	}
	requireSaneStoppedTree(t, tree, source, "cancellation")

	routed, fallback := gts.AdmissionCandidateCounters()
	if fallback != 1 || routed != 0 {
		t.Fatalf("expected exactly one stop-control fallback and zero routed: routed=%d fallback=%d", routed, fallback)
	}
	if reason := gts.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "stop-control tripped: cancelled") {
		t.Fatalf("fallback reason = %q, want it to name the scheduler's cancellation trip", reason)
	}
}

// TestAdmissionSwitchCompactMemoryBudgetStopReceiptMatchesProduction proves
// the memory-budget class: this modest, deliberately small witness (see the
// package doc comment above), combined with a tiny GOT_PARSE_MEMORY_BUDGET_MB,
// trips the scheduler's deterministic FootprintBytes-vs-budget poll -- the
// same configured budget production's own arena/scratch accounting honors --
// and both engines report ParseStopMemoryBudget for the identical input.
func TestAdmissionSwitchCompactMemoryBudgetStopReceiptMatchesProduction(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "1")
	gts.ResetParseEnvConfigCacheForTests()
	defer gts.ResetParseEnvConfigCacheForTests()

	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)

	source := stopControlWitnessGoSource(5000)
	requireStopControlWitnessUnderWall(t, source)

	// Drain the shared arena pool before each parse: a warm, large-capacity
	// retained arena needs fewer growth events to reach this witness's node
	// count, and the deterministic arena budget check only runs at
	// materialization-boundary call sites (not every allocation), so a warm
	// pool can let a single large batch jump straight past the budget between
	// polls. Draining forces every parse below to grow its arena from cold,
	// matching the poll cadence this witness was sized against.
	gts.DrainArenaPools()
	prod := gts.NewParser(grammars.GoLanguage())
	prod.SetAdmissionCandidateRoute(false)
	prodTree, err := prod.Parse(source)
	if err != nil {
		t.Fatalf("production reference parse: %v", err)
	}
	defer prodTree.Release()
	if got, want := prodTree.ParseStopReason(), gts.ParseStopMemoryBudget; got != want {
		t.Fatalf("production reference ParseStopReason() = %q, want %q (runtime=%s)", got, want, prodTree.ParseRuntime().Summary())
	}

	gts.ResetAdmissionCandidateCountersForTest()
	gts.DrainArenaPools()
	candidate := gts.NewParser(grammars.GoLanguage())
	tree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate-route parse: %v", err)
	}
	defer tree.Release()

	if got, want := tree.ParseStopReason(), gts.ParseStopMemoryBudget; got != want {
		t.Fatalf("candidate-route ParseStopReason() = %q, want %q (production reference agrees on %q)", got, want, prodTree.ParseStopReason())
	}
	requireSaneStoppedTree(t, tree, source, "memory-budget")

	routed, fallback := gts.AdmissionCandidateCounters()
	if fallback != 1 || routed != 0 {
		t.Fatalf("expected exactly one stop-control fallback and zero routed: routed=%d fallback=%d", routed, fallback)
	}
	if reason := gts.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "stop-control tripped: memory_budget") {
		t.Fatalf("fallback reason = %q, want it to name the scheduler's memory-budget trip", reason)
	}
}

// TestAdmissionSwitchCompactMemoryBudgetPollIsDeterministic is the direct
// determinism gate (tranche B8 work order item 4): the memory-budget half of
// pollStopControl compares Core.FootprintBytes(), already-tracked slice and
// map length/capacity reads (tranche B9 honest-accounting gate), against a
// fixed byte budget -- pure integer arithmetic with no wall-clock or
// GC-timing input. Same input and same budget must trip the
// scheduler's poll at the same internal juncture on every attempt, unlike
// production's own hard ceiling (runtime.MemStats-based, explicitly
// documented as non-deterministic in parser_memory_budget_runtime.go). This
// drives the candidate engine seam directly (tryCompactFullParseRoute, not
// the public Parse route), draining and re-arming the arena pool between
// attempts, and requires the identical decline detail both times.
func TestAdmissionSwitchCompactMemoryBudgetPollIsDeterministic(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "1")
	gts.ResetParseEnvConfigCacheForTests()
	defer gts.ResetParseEnvConfigCacheForTests()

	source := stopControlWitnessGoSource(5000)
	requireStopControlWitnessUnderWall(t, source)

	var reasons [3]string
	for i := range reasons {
		gts.DrainArenaPools()
		parser := gts.NewParser(grammars.GoLanguage())
		_, ok, reason := gts.TryCompactFullParseRouteForTest(parser, source)
		if ok {
			t.Fatalf("attempt %d: candidate engine accepted instead of stopping on the memory budget", i)
		}
		if !strings.Contains(reason, "stop-control tripped: memory_budget") {
			t.Fatalf("attempt %d: decline reason = %q, want the scheduler's memory-budget trip", i, reason)
		}
		reasons[i] = reason
	}
	for i := 1; i < len(reasons); i++ {
		if reasons[i] != reasons[0] {
			t.Fatalf("non-deterministic stop point: attempt 0 = %q, attempt %d = %q", reasons[0], i, reasons[i])
		}
	}
}
