//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Tranche B8 gate: for each stop-control class (memory budget, timeout,
// cancellation), an admission-eligible input that trips the class produces a
// stop receipt from the candidate route (routed through the scheduler's
// pollStopControl, parsercore_phase0_stop_control.go) that matches what production
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

// TestAdmissionSwitchCompactTimeoutSharesDeadlineWithProduction proves that
// Parse applies SetTimeoutMicros once per call. The compact route must use the
// deadline that Parse opens. When the compact route trips the timeout, the
// production fallback must find the deadline already expired. Production must
// then stop before its first iteration. It must not start a second timeout
// window.
func TestAdmissionSwitchCompactTimeoutSharesDeadlineWithProduction(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)

	// The witness is large so that the compact route cannot finish inside
	// the timeout, also on a fast host. The timeout is long enough that a
	// second production window would run many iterations after its setup.
	source := stopControlWitnessGoSource(20000)

	gts.ResetAdmissionCandidateCountersForTest()
	parser := gts.NewParser(grammars.GoLanguage())
	parser.SetTimeoutMicros(20_000)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	if got, want := tree.ParseStopReason(), gts.ParseStopTimeout; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q", got, want)
	}
	requireSaneStoppedTree(t, tree, source, "shared timeout")
	if reason := gts.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "stop-control tripped: timeout") {
		t.Fatalf("fallback reason = %q, want the compact route to trip the timeout first", reason)
	}
	if got := tree.ParseRuntime().Iterations; got != 0 {
		t.Fatalf("production fallback ran %d iterations after the compact route used the whole timeout; want 0", got)
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

// TestAdmissionSwitchCompactMemoryBudgetTripsUnderThrottledPoll is lever 2's
// regression gate: pollStopControl now recomputes the scheduler's memory
// footprint only every footprintPollStride-th call instead of on every
// dispatch (parsercore_phase0_stop_control.go). This proves the throttle
// does not let a pathological input evade the budget: an adversarial witness
// large enough to drive well past footprintPollStride dispatches, combined
// with a tiny configured budget, must still decline with the scheduler's
// memory-budget trip, deterministically, on every attempt.
func TestAdmissionSwitchCompactMemoryBudgetTripsUnderThrottledPoll(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "1")
	gts.ResetParseEnvConfigCacheForTests()
	defer gts.ResetParseEnvConfigCacheForTests()

	// Ten times TestAdmissionSwitchCompactMemoryBudgetPollIsDeterministic's
	// witness: comfortably more statements (and so more dispatches) than
	// footprintPollStride, so a parse that somehow dodged every throttled
	// poll would have many more chances to do so than a small witness gives.
	if got, want := gts.FootprintPollStrideForTest(), 64; got != want {
		t.Fatalf("footprintPollStride = %d, want %d (update this witness's size if the stride changes)", got, want)
	}
	source := stopControlWitnessGoSource(50000)

	var reasons [3]string
	for i := range reasons {
		gts.DrainArenaPools()
		parser := gts.NewParser(grammars.GoLanguage())
		tree, ok, reason := gts.TryCompactFullParseRouteForTest(parser, source)
		if ok {
			t.Fatalf("attempt %d: candidate engine accepted a %d-line adversarial witness under a 1 MB budget instead of stopping", i, 50000)
		}
		if tree != nil {
			t.Fatalf("attempt %d: decline returned a non-nil tree", i)
		}
		if !strings.Contains(reason, "stop-control tripped: memory_budget") {
			t.Fatalf("attempt %d: decline reason = %q, want the scheduler's memory-budget trip (throttling must not silently disable the budget)", i, reason)
		}
		reasons[i] = reason
	}
	for i := 1; i < len(reasons); i++ {
		if reasons[i] != reasons[0] {
			t.Fatalf("non-deterministic stop point under the throttled poll: attempt 0 = %q, attempt %d = %q", reasons[0], i, reasons[i])
		}
	}
}

// footprintOvershootEpsilon bounds how far the PEAK exact scheduler
// footprint observed during a parse (stopControlExactFootprintObserverForTest,
// which fires on every exact recompute regardless of throttling) may exceed
// the configured budget. buildbox measured 1.3-2.2% overshoot on the
// witnesses below with the growth-triggered poll (footprintTriggerProxy,
// parsercore_phase0_stop_control.go); 10% keeps real margin above that
// measurement instead of pinning the exact figure, which would make this
// test brittle to unrelated scheduler-footprint changes.
const footprintOvershootEpsilon = 0.10

// assertPeakFootprintBounded drives source through the compact route with a
// tiny configured budget, recording the peak EXACT footprint
// stopControlMemoryBudgetReasonWithAdditionalBytes ever computed (not just
// the value at the point that finally trips), and asserts it clears budget
// only by footprintOvershootEpsilon at most. This is a materially stronger
// claim than "the parse eventually declines" (TestAdmissionSwitchCompact
// MemoryBudgetTripsUnderThrottledPoll): it bounds how far over budget the
// scheduler's real memory footprint is ever allowed to run, given the
// growth-triggered poll now forces an exact recompute whenever the cheap
// capacity-based proxy crosses footprintTriggerFractionDenominator's worth
// of budget, not just every footprintPollStride-th dispatch.
func assertPeakFootprintBounded(t *testing.T, label string, lang *gts.Language, source []byte, budgetBytes int64) (peak uint64, ok bool, reason string) {
	t.Helper()
	var samples int
	gts.SetStopControlExactFootprintObserverForTest(func(exact uint64) {
		samples++
		if exact > peak {
			peak = exact
		}
	})
	defer gts.SetStopControlExactFootprintObserverForTest(nil)

	gts.DrainArenaPools()
	parser := gts.NewParser(lang)
	parser.SetMemoryBudgetBytes(budgetBytes)
	var tree *gts.Tree
	tree, ok, reason = gts.TryCompactFullParseRouteForTest(parser, source)
	if tree != nil {
		tree.Release()
	}
	if samples == 0 {
		t.Fatalf("%s: the exact-footprint observer never fired; the compact route did not run far enough to prove anything about its peak footprint", label)
	}
	limit := uint64(float64(budgetBytes) * (1 + footprintOvershootEpsilon))
	t.Logf("%s: peak=%d budget=%d limit=%d(+%.0f%%) samples=%d ok=%v reason=%q",
		label, peak, budgetBytes, limit, footprintOvershootEpsilon*100, samples, ok, reason)
	if peak > limit {
		t.Fatalf("%s: peak exact footprint %d exceeds budget %d * (1+%.2f) = %d",
			label, peak, budgetBytes, footprintOvershootEpsilon, limit)
	}
	return peak, ok, reason
}

// TestAdmissionSwitchCompactMemoryBudgetPeakFootprintBoundedOnAdversarialWitness
// is the explicit, enforced overshoot bound the throttled poll (lever 2)
// must hold, not just "the budget eventually trips"
// (TestAdmissionSwitchCompactMemoryBudgetTripsUnderThrottledPoll): on the
// same adversarial many-statement witness, the PEAK exact footprint ever
// observed must clear the configured budget by no more than
// footprintOvershootEpsilon.
func TestAdmissionSwitchCompactMemoryBudgetPeakFootprintBoundedOnAdversarialWitness(t *testing.T) {
	const budgetBytes = 1 << 20 // 1 MiB, exact -- no MB-env-var rounding.
	source := stopControlWitnessGoSource(50000)
	_, ok, reason := assertPeakFootprintBounded(t, "adversarial-witness", grammars.GoLanguage(), source, budgetBytes)
	if ok {
		t.Fatal("adversarial-witness: candidate engine accepted a 50,000-line witness under a 1 MiB budget instead of stopping")
	}
	if !strings.Contains(reason, "stop-control tripped: memory_budget") {
		t.Fatalf("adversarial-witness: decline reason = %q, want the scheduler's memory-budget trip", reason)
	}
}

// buildWideGLRAmbiguousLanguage builds the classic ambiguous expression
// grammar (E -> E + E | 1), the textbook construction for a GLR fork
// explosion: for N terms, the number of distinct parse trees grows with the
// Nth Catalan number, so the scheduler must keep many simultaneous GSS
// stack forks (headers) alive while it explores them.
func buildWideGLRAmbiguousLanguage(t *testing.T) *gts.Language {
	t.Helper()
	g := grammargen.NewGrammar("wide_glr_footprint_probe")
	g.Define("program", grammargen.Sym("expr"))
	g.Define("expr", grammargen.Choice(
		grammargen.Seq(grammargen.Sym("expr"), grammargen.Str("+"), grammargen.Sym("expr")),
		grammargen.Str("1"),
	))
	grammargen.AddConflict(g, "expr")
	lang, err := grammargen.GenerateLanguage(g)
	if err != nil {
		t.Fatalf("generate ambiguous grammar: %v", err)
	}
	return lang
}

// TestAdmissionSwitchCompactMemoryBudgetPeakFootprintBoundedOnWideGLR is the
// pathological-wide-GLR half of the overshoot bound (c): a genuinely
// ambiguous input (see buildWideGLRAmbiguousLanguage), under the same tiny
// budget, must also never let the PEAK exact footprint clear budget by more
// than footprintOvershootEpsilon.
//
// FINDING: this input does not actually exercise the memory-budget poll at
// all. The compact scheduler enforces an independent, always-on, O(1)
// structural cap on live links per shared GSS boundary
// (core.Limits.MaxLinksPerBoundary, internal/parsercorephase0/core.go,
// configured at 8 for the admission-candidate route in
// admission_switch_candidate.go) that fires within the first handful of
// ambiguous terms, long before ambiguity could compound into a large
// footprint -- see the logged decline reason. This is a real, valuable,
// negative finding, not a workaround: it demonstrates defense in depth. A
// wide-GLR input that somehow cleared that structural cap would still be
// bounded by footprintGrowthCrossedTriggerFraction (see the adversarial
// witness above for a case that does reach and exercise that poll), but the
// link cap means this specific failure mode -- unbounded GLR fork
// width driving unbounded footprint growth -- is foreclosed before the
// memory-budget poll would ever need to catch it.
func TestAdmissionSwitchCompactMemoryBudgetPeakFootprintBoundedOnWideGLR(t *testing.T) {
	const budgetBytes = 1 << 20 // 1 MiB, exact.
	lang := buildWideGLRAmbiguousLanguage(t)

	terms := make([]string, 40)
	for i := range terms {
		terms[i] = "1"
	}
	source := []byte(strings.Join(terms, "+"))

	peak, ok, reason := assertPeakFootprintBounded(t, "wide-glr", lang, source, budgetBytes)
	if ok {
		t.Fatal("wide-glr: candidate engine accepted a 40-term maximally-ambiguous expression instead of declining")
	}
	if !strings.Contains(reason, "live-link cap exceeded") {
		t.Fatalf("wide-glr: decline reason = %q, want the independent live-link structural cap (see the test's FINDING doc comment); "+
			"if this fires, MaxLinksPerBoundary's configuration changed and this witness may now actually exercise the memory-budget poll -- "+
			"re-verify the peak-footprint bound still holds either way", reason)
	}
	// The independent link cap declines this input almost immediately, so
	// its peak footprint should be a small fraction of budget, not merely
	// under the (1+epsilon) ceiling assertPeakFootprintBounded already
	// checked. A peak anywhere near budget here would mean the link cap
	// stopped being the first line of defense it is today.
	if quarterBudget := uint64(budgetBytes) / 4; peak > quarterBudget {
		t.Fatalf("wide-glr: peak footprint %d is not comfortably below budget (>%d, a quarter of the %d budget); "+
			"the independent link cap may no longer be declining this input early", peak, quarterBudget, uint64(budgetBytes))
	}
}
