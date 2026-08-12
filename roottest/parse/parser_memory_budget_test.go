package parse_test

import (
	"bytes"
	"fmt"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestParserMemoryBudgetStopsParse(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "1")
	gotreesitter.ResetParseEnvConfigCacheForTests()
	defer gotreesitter.ResetParseEnvConfigCacheForTests()

	parser := gotreesitter.NewParser(grammars.GoLanguage())
	var source bytes.Buffer
	source.WriteString("package p\nfunc f() {\n")
	for i := 0; i < 20000; i++ {
		source.WriteString("var x = 1\n")
	}
	source.WriteString("}\n")
	tree, err := parser.Parse(source.Bytes())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	defer tree.Release()

	if got, want := tree.ParseStopReason(), gotreesitter.ParseStopMemoryBudget; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q (runtime=%s)", got, want, tree.ParseRuntime().Summary())
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
	rt := tree.ParseRuntime()
	if rt.MemoryBudgetBytes <= 0 {
		t.Fatalf("MemoryBudgetBytes = %d, want > 0", rt.MemoryBudgetBytes)
	}
	switch rt.MemoryBudgetStopSource {
	case "arena":
		if growth := rt.ArenaBytesAllocated - rt.ArenaBaselineBytes; growth < rt.MemoryBudgetBytes {
			t.Fatalf("arena growth = %d, want >= budget %d (runtime=%s)", growth, rt.MemoryBudgetBytes, rt.Summary())
		}
	case "scratch":
		if growth := rt.ScratchBytesAllocated - rt.ScratchBaselineBytes; growth < rt.MemoryBudgetBytes {
			t.Fatalf("scratch growth = %d, want >= budget %d (runtime=%s)", growth, rt.MemoryBudgetBytes, rt.Summary())
		}
	case "runtime_heap":
		if rt.RuntimeHeapGrowthBytes < uint64(rt.MemoryBudgetBytes) {
			t.Fatalf("runtime heap growth = %d, want >= budget %d (runtime=%s)", rt.RuntimeHeapGrowthBytes, rt.MemoryBudgetBytes, rt.Summary())
		}
	case "runtime_sys":
		if rt.RuntimeSysGrowthBytes < uint64(rt.MemoryBudgetBytes) {
			t.Fatalf("runtime sys growth = %d, want >= budget %d (runtime=%s)", rt.RuntimeSysGrowthBytes, rt.MemoryBudgetBytes, rt.Summary())
		}
	default:
		t.Fatalf("MemoryBudgetStopSource = %q, want exact attribution (runtime=%s)", rt.MemoryBudgetStopSource, rt.Summary())
	}
}

// TestGoParseGiantTableLiteralStopsWithinMemoryBudget is the RCA's bare-Parse
// witness (2026-07-12 budget-containment RCA), adapted to a SYNTHETIC
// generated input rather than depending on any repo source file's content
// (the RCA's own witness was a 70KiB prefix of query_test.go, which is not
// used here). It constructs a large slice-of-struct-literal table -- a
// "giant table literal", generated at test time -- sized to require real
// arena/scratch allocation well past a small memory budget, and asserts bare
// NewParser(...).Parse stops with ParseStopMemoryBudget while process heap
// growth stays bounded rather than ballooning many multiples of the budget
// past it (the RCA measured 16-26x overshoot before this fix).
//
// This validates the general containment path end-to-end: the hardened
// runtime poll (parser_memory_budget_runtime.go), including its
// volume-triggered forced check, and the decoupled hard ceiling, all wired
// through bare Parse with the DFA lexer (the routing the RCA identified as
// the asymmetric path -- the gateway token source for .go files keeps
// survivor sets bounded and rarely needs this containment at all).
//
// It does NOT specifically exercise mergeStacksWithScratchLargeCap's
// coarse-stride in-merge poll: Go's steady-state full-parse merge per-key
// cap (3) never widens past maxStacksPerMergeKey (6) for well-formed input,
// so the large-cap merge path is not reached by this witness. That path is
// covered directly and deterministically by
// TestMergeStacksWithScratchLargeCapPollsMemoryBudgetMidGrind in the
// internal package (parser_memory_budget_merge_test.go), which constructs
// the O(survivors^2) shape directly instead of relying on a natural-language
// trigger.
//
// A malformed/truncated variant of this input (mirroring the RCA's
// query_test.go-prefix witness more literally -- balanced braces up to a
// dangling partial identifier at EOF) was investigated while building this
// fix but is intentionally NOT used here: truncating mid-token reliably drove
// the parser into pathological C-recovery, and the resulting tree tripped a
// SEPARATE, pre-existing bug in Go-specific post-parse compatibility
// normalization (walkGoCompatSubtree, parser_result_go_compat.go) that has no
// memory-budget awareness at all -- it only polls for ParseStopTimeout /
// ParseStopCancelled -- and runs unconditionally while finalizing the result
// tree, even for a GLR loop that already stopped early on budget. That is a
// distinct, out-of-scope escape vector (it OOMed a throwaway repro at
// several GiB regardless of this change) and is being raised separately
// rather than folded into this fix; a truncated input here would crash
// unpredictably for a reason unrelated to what this change addresses.
func TestGoParseGiantTableLiteralStopsWithinMemoryBudget(t *testing.T) {
	// budgetMB must be strictly above the tight-poll-mask threshold (64MiB,
	// parseRuntimeMemoryTightBudget) so this witness exercises the loose (255)
	// poll mask this fix's volume-triggered bypass targets, not the
	// already-responsive tight (15) mask a small budget would select.
	const budgetMB = 96
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", strconv.Itoa(budgetMB))
	gotreesitter.ResetParseEnvConfigCacheForTests()
	defer gotreesitter.ResetParseEnvConfigCacheForTests()

	var src bytes.Buffer
	src.WriteString("package p\n\ntype kv struct {\n\tName    string\n\tVisible bool\n\tNamed   bool\n}\n\nvar t = []kv{\n")
	// entries is deliberately large (well beyond what's needed to cross the
	// budget in an isolated run) so the stop is robust to ambient allocator/GC
	// state from whatever else ran earlier in the same test binary, rather
	// than depending on a razor-thin margin over the budget.
	const entries = 300000
	for i := 0; i < entries; i++ {
		fmt.Fprintf(&src, "\t{Name: %q, Visible: true, Named: true}, // %d\n", fmt.Sprintf("identifier%d", i), i)
	}
	src.WriteString("}\n")

	// Disable GC during the measured window so heap growth reflects true
	// allocation volume rather than being masked by a GC cycle landing
	// mid-parse; restore the default afterward.
	debug.SetGCPercent(-1)
	defer debug.SetGCPercent(100)
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	parser := gotreesitter.NewParser(grammars.GoLanguage())
	// Pin to production: this witness's ~12.7MB source sat above the compact
	// admission switch's former 64 KiB size floor, so before tranche B9 "bare
	// Parse" already meant production-only here, matching the doc comment
	// above ("bare Parse with the DFA lexer"). B9 removed that floor, so an
	// unpinned Parse here would also attempt the compact route first: its own
	// bounded, independent decline still contains its own heap growth, but
	// this test measures RAW PROCESS heap growth with GC disabled, which sums
	// across both engines' retained arena/backing-array capacity rather than
	// isolating either one -- a different, dual-engine measurement this
	// witness was never designed to make. The explicit pin restores the
	// single-engine, production-only scope the RCA this test guards against
	// actually targets.
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(src.Bytes())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	defer tree.Release()

	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)

	if got, want := tree.ParseStopReason(), gotreesitter.ParseStopMemoryBudget; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q (runtime=%s)", got, want, tree.ParseRuntime().Summary())
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}

	budgetBytes := int64(budgetMB) * 1024 * 1024
	const maxOvershootFactor = 6
	if heapGrowth > budgetBytes*maxOvershootFactor {
		t.Fatalf(
			"process heap growth = %d bytes (%.2fx budget), want < %dx budget %d bytes -- containment escape (runtime=%s)",
			heapGrowth, float64(heapGrowth)/float64(budgetBytes), maxOvershootFactor, budgetBytes, tree.ParseRuntime().Summary(),
		)
	}
	t.Logf("heap growth = %d bytes (%.2fx budget %d bytes), stop=%s", heapGrowth, float64(heapGrowth)/float64(budgetBytes), budgetBytes, tree.ParseRuntime().Summary())
}

// TestGoParseGiantTableLiteralShippedRouteStaysWithinAchievedBound is the
// unpinned counterpart of TestGoParseGiantTableLiteralStopsWithinMemoryBudget
// above: the same witness, the same GC-disabled cumulative-allocation
// measurement, with the candidate route left on its shipped default instead
// of pinned to production. This is the end-to-end coverage the production
// pin above deliberately removes (that test isolates production's own
// contract; this one measures what a caller who never pins actually gets).
//
// The compact route attempts this witness first (tranche B9 removed the
// former 64 KiB size floor that used to keep it production-only). Its own
// scheduler stop-control poll compares a real, capacity-based retained-
// footprint gauge (Core.FootprintBytes, internal/parsercorephase0) against
// the configured budget and declines close to it (measured 103.6 MB
// footprint against a 96 MB budget on this witness), releasing its retained
// capacity immediately on decline. Production then serves the fallback
// exactly as the pinned test above already proves.
//
// The bound this asserts is measured, not the production-only 6x contract:
// this GC-disabled methodology counts cumulative allocation, not retained
// footprint, so it also counts every ephemeral value the compact scheduler's
// hot dispatch/election path allocates and discards per token -- a live GC
// reclaims that continuously in normal operation, but nothing a
// retained-footprint poll tracks can bound it, because it is never part of
// any tracked structure. Measured on this witness: 9.14-9.56x across six
// runs (a real, verified improvement over the pre-honest-accounting
// length-only gauge's 11.51x on the same witness), not inside the
// production-only 6x contract. maxOvershootFactor sits just above the high
// end of that measured range and well below the pre-fix figure, so this
// test still catches a regression back to length-only accounting while
// tolerating ordinary run-to-run variance. Closing the remaining gap to 6x
// is an open trade-off against routing coverage (see
// stopControlFootprintChurnRatio's doc comment, parsercore_phase0_driver.go)
// that this test intentionally does not force by tightening this number
// further.
func TestGoParseGiantTableLiteralShippedRouteStaysWithinAchievedBound(t *testing.T) {
	const budgetMB = 96
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", strconv.Itoa(budgetMB))
	gotreesitter.ResetParseEnvConfigCacheForTests()
	defer gotreesitter.ResetParseEnvConfigCacheForTests()

	var src bytes.Buffer
	src.WriteString("package p\n\ntype kv struct {\n\tName    string\n\tVisible bool\n\tNamed   bool\n}\n\nvar t = []kv{\n")
	const entries = 300000
	for i := 0; i < entries; i++ {
		fmt.Fprintf(&src, "\t{Name: %q, Visible: true, Named: true}, // %d\n", fmt.Sprintf("identifier%d", i), i)
	}
	src.WriteString("}\n")

	debug.SetGCPercent(-1)
	defer debug.SetGCPercent(100)
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	parser := gotreesitter.NewParser(grammars.GoLanguage())
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	tree, err := parser.Parse(src.Bytes())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	defer tree.Release()

	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()

	if got, want := tree.ParseStopReason(), gotreesitter.ParseStopMemoryBudget; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q (runtime=%s)", got, want, tree.ParseRuntime().Summary())
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
	// The candidate route must have attempted and declined this witness (not
	// silently stayed on production by some other eligibility gate): routed
	// stays 0 (production served the returned tree) and fallback moves by
	// exactly 1 (one candidate-route decline).
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
		t.Fatalf("candidate route routing: routed %d->%d fallback %d->%d, want routed unchanged and fallback +1",
			routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}

	budgetBytes := int64(budgetMB) * 1024 * 1024
	const maxOvershootFactor = 10
	if heapGrowth > budgetBytes*maxOvershootFactor {
		t.Fatalf(
			"process heap growth = %d bytes (%.2fx budget), want < %dx budget %d bytes -- regression past the tranche B9 achieved bound (runtime=%s)",
			heapGrowth, float64(heapGrowth)/float64(budgetBytes), maxOvershootFactor, budgetBytes, tree.ParseRuntime().Summary(),
		)
	}
	t.Logf("shipped-route heap growth = %d bytes (%.2fx budget %d bytes)", heapGrowth, float64(heapGrowth)/float64(budgetBytes), budgetBytes)
	t.Logf("heap growth = %d bytes (%.2fx budget %d bytes), stop=%s", heapGrowth, float64(heapGrowth)/float64(budgetBytes), budgetBytes, tree.ParseRuntime().Summary())
}
