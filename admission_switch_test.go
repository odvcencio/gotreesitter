package gotreesitter_test

import (
	"bytes"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// These tests exercise the Phase-3 admission switch through the public API. They
// are build-agnostic: they assert routing EVENTS (a full parse either serves the
// candidate route or falls back), not which engine served the parse, so they
// hold in both the default and the gts_parsercorephase0 builds. The digest and
// engine-identity proofs live in the tagged internal tests.

func admissionRoutingEvents(t *testing.T) uint64 {
	t.Helper()
	routed, fallback := gts.AdmissionCandidateCounters()
	return routed + fallback
}

// newAdmissionDFAParser returns a parser for the first candidate DFA-backed
// language whose smoke sample parses to a clean, full-span tree in production.
// The DFA Parse path is the one the admission switch can route.
func newAdmissionDFAParser(t *testing.T) (*gts.Parser, []byte) {
	t.Helper()
	for _, name := range []string{"go", "lua", "toml", "ini", "json5", "css"} {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			continue
		}
		lang := entry.Language()
		if grammars.EvaluateParseSupport(*entry, lang).Backend != grammars.ParseBackendDFA {
			continue
		}
		source := []byte(grammars.ParseSmokeSample(name))
		probe := gts.NewParser(lang)
		probe.SetAdmissionCandidateRoute(false)
		tree, err := probe.Parse(source)
		if err != nil || tree == nil || tree.RootNode() == nil {
			continue
		}
		clean := tree.RootNode().EndByte() == uint32(len(source)) && !tree.RootNode().HasError()
		tree.Release()
		if !clean {
			continue
		}
		return gts.NewParser(lang), source
	}
	t.Skip("no clean DFA-backed language available for the routing test")
	return nil, nil
}

func requireCleanFullTree(t *testing.T, tree *gts.Tree, source []byte, label string) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s: nil tree/root", label)
	}
	root := tree.RootNode()
	if root.EndByte() != uint32(len(source)) {
		t.Fatalf("%s: truncated root end=%d source=%d", label, root.EndByte(), len(source))
	}
	if root.HasError() {
		t.Fatalf("%s: tree has error nodes", label)
	}
}

// TestAdmissionSwitchDefaultLeavesParseOnProduction proves the shipped default
// (off) routes no parse through the candidate and moves no counter.
func TestAdmissionSwitchDefaultLeavesParseOnProduction(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(false)
	gts.ResetAdmissionCandidateCountersForTest()

	parser, source := newAdmissionDFAParser(t)
	before := admissionRoutingEvents(t)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	requireCleanFullTree(t, tree, source, "default-off")
	if got := admissionRoutingEvents(t); got != before {
		t.Fatalf("default-off moved a routing counter: %d -> %d", before, got)
	}
}

// TestAdmissionSwitchPerParserOnRoutesFreshParse proves a per-Parser override
// makes a fresh DFA Parse consult the candidate route exactly once.
func TestAdmissionSwitchPerParserOnRoutesFreshParse(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(false)
	gts.ResetAdmissionCandidateCountersForTest()

	parser, source := newAdmissionDFAParser(t)
	parser.SetAdmissionCandidateRoute(true)
	before := admissionRoutingEvents(t)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	requireCleanFullTree(t, tree, source, "per-parser-on")
	if got := admissionRoutingEvents(t); got != before+1 {
		t.Fatalf("per-parser-on expected one routing event: %d -> %d", before, got)
	}
}

// TestAdmissionSwitchPerParserOffOverridesGlobalOn proves precedence: a per-
// Parser off wins over a global-on default and consults no candidate route.
func TestAdmissionSwitchPerParserOffOverridesGlobalOn(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()

	parser, source := newAdmissionDFAParser(t)
	parser.SetAdmissionCandidateRoute(false)
	before := admissionRoutingEvents(t)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	requireCleanFullTree(t, tree, source, "per-parser-off")
	if got := admissionRoutingEvents(t); got != before {
		t.Fatalf("per-parser-off must override global-on and move no counter: %d -> %d", before, got)
	}
}

// TestAdmissionSwitchGlobalOnRoutesFreshParse proves the global default alone
// (no per-Parser override) routes a fresh DFA Parse.
func TestAdmissionSwitchGlobalOnRoutesFreshParse(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()

	parser, source := newAdmissionDFAParser(t)
	before := admissionRoutingEvents(t)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	requireCleanFullTree(t, tree, source, "global-on")
	if got := admissionRoutingEvents(t); got != before+1 {
		t.Fatalf("global-on expected one routing event: %d -> %d", before, got)
	}
}

// Incremental attempts must not change counters reserved for full parsing.
func TestAdmissionSwitchParseIncrementalDoesNotCountFullCandidate(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()

	parser, source := newAdmissionDFAParser(t)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer oldTree.Release()
	before := admissionRoutingEvents(t)

	edited := append(append([]byte(nil), source...), ' ')
	eof := admissionEOFPoint(source)
	edit := gts.InputEdit{
		StartByte:   uint32(len(source)),
		OldEndByte:  uint32(len(source)),
		NewEndByte:  uint32(len(source) + 1),
		StartPoint:  eof,
		OldEndPoint: eof,
		NewEndPoint: gts.Point{Row: eof.Row, Column: eof.Column + 1},
	}
	oldTree.Edit(edit)
	newTree, err := parser.ParseIncremental(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	if newTree != nil && newTree != oldTree {
		defer newTree.Release()
	}
	if got := admissionRoutingEvents(t); got != before {
		t.Fatalf("ParseIncremental changed full-route counts: %d -> %d", before, got)
	}
}

// TestAdmissionSwitchCustomTokenSourceNeverConsultsCandidate proves that a
// caller-supplied token source is never eligible: the seam declines before any
// candidate attempt, so no counter moves.
func TestAdmissionSwitchCustomTokenSourceNeverConsultsCandidate(t *testing.T) {
	// Find any registered token-source-backed language (json is one).
	var entry *grammars.LangEntry
	for _, name := range []string{"json", "go", "typescript", "tsx", "javascript"} {
		e := grammars.DetectLanguageByName(name)
		if e != nil && e.TokenSourceFactory != nil {
			entry = e
			break
		}
	}
	if entry == nil {
		t.Skip("no token-source-backed language registered for the custom source test")
	}
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()

	lang := entry.Language()
	source := []byte(grammars.ParseSmokeSample(entry.Name))
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	before := admissionRoutingEvents(t)
	tree, err := parser.ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
	if err != nil {
		t.Fatalf("token-source parse: %v", err)
	}
	defer tree.Release()
	if got := admissionRoutingEvents(t); got != before {
		t.Fatalf("custom token source consulted the candidate route: %d -> %d", before, got)
	}
}

// TestAdmissionSwitchDeclinesWhenIncludedRangesSet proves the candidate route
// declines when included ranges are configured: the compact runner lexes the
// whole source and cannot honor ranges, so the parse stays on production.
func TestAdmissionSwitchDeclinesWhenIncludedRangesSet(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()

	parser, source := newAdmissionDFAParser(t)
	parser.SetIncludedRanges([]gts.Range{{StartByte: 0, EndByte: uint32(len(source)), EndPoint: admissionEOFPoint(source)}})
	before := admissionRoutingEvents(t)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	if got := admissionRoutingEvents(t); got != before {
		t.Fatalf("included ranges must keep the parse on production: %d -> %d", before, got)
	}
}

// TestAdmissionSwitchConsultsCandidateWhenTimeoutSetButNotExpired proves
// that, since tranche B8 wired the scheduler's own deadline poll, a live but
// unexpired timeout no longer declines eligibility outright: the parse now
// consults the candidate route (one routing event) and still returns a clean
// tree. Like the rest of this file it asserts the routing EVENT, not which
// engine served the parse, so it holds under every build (including
// -tags gts_no_parsercorephase0, where the engine itself is a stub that
// always declines: the event still fires, just as a fallback).
func TestAdmissionSwitchConsultsCandidateWhenTimeoutSetButNotExpired(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()

	parser, source := newAdmissionDFAParser(t)
	parser.SetTimeoutMicros(1_000_000)
	before := admissionRoutingEvents(t)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	requireCleanFullTree(t, tree, source, "timeout-armed-but-not-tripped")
	if got := tree.ParseStopReason(); got == gts.ParseStopTimeout {
		t.Fatalf("a generous timeout must not trip: ParseStopReason() = %q", got)
	}
	if got := admissionRoutingEvents(t); got != before+1 {
		t.Fatalf("a live but unexpired timeout must be eligible (exactly one routing event): %d -> %d", before, got)
	}
}

// TestAdmissionSwitchConsultsCandidateWhenCancellationFlagSetButNotTripped is
// the cancellation-flag counterpart of
// TestAdmissionSwitchConsultsCandidateWhenTimeoutSetButNotExpired.
func TestAdmissionSwitchConsultsCandidateWhenCancellationFlagSetButNotTripped(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()

	parser, source := newAdmissionDFAParser(t)
	var flag uint32
	parser.SetCancellationFlag(&flag)
	before := admissionRoutingEvents(t)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	requireCleanFullTree(t, tree, source, "cancellation-armed-but-not-tripped")
	if got := tree.ParseStopReason(); got == gts.ParseStopCancelled {
		t.Fatalf("an unset cancellation flag must not trip: ParseStopReason() = %q", got)
	}
	if got := admissionRoutingEvents(t); got != before+1 {
		t.Fatalf("a live but untripped cancellation flag must be eligible (exactly one routing event): %d -> %d", before, got)
	}
}

// TestAdmissionSwitchInternalSubParsersPinnedToProduction proves that recovery,
// snippet, and injection sub-parsers are born pinned to the production route:
// even with the global default forced on, they are ineligible to route a
// compact fragment into recovery splicing or an injection subtree.
func TestAdmissionSwitchInternalSubParsersPinnedToProduction(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true) // global ON, the future-flip state

	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		t.Skip("go grammar not registered")
	}
	lang := entry.Language()

	snippet := gts.AcquireSnippetParserForTest(lang)
	defer gts.ReleaseSnippetParserForTest(snippet)
	if !gts.ParserPinnedToProductionForTest(snippet) {
		t.Fatal("recovery/snippet parser is not pinned to production")
	}
	if gts.ParserAdmissionEligibleForTest(snippet) {
		t.Fatal("recovery/snippet parser is eligible to route under global ON")
	}

	child := gts.InjectionChildParserForTest(lang)
	if !gts.ParserPinnedToProductionForTest(child) {
		t.Fatal("injection child parser is not pinned to production")
	}
	if gts.ParserAdmissionEligibleForTest(child) {
		t.Fatal("injection child parser is eligible to route under global ON")
	}
}

// TestAdmissionSwitchPooledParserScrubsRouteState proves ParserPool.applyDefaults
// clears the per-Parser override so a pooled parser follows the default again.
func TestAdmissionSwitchPooledParserScrubsRouteState(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(false)

	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		t.Skip("go grammar not registered")
	}
	pool := gts.NewParserPool(entry.Language())
	// A checked-out parser forced off, then returned, must not leak that override.
	p1 := gts.ParserPoolCheckoutForTest(pool)
	p1.SetAdmissionCandidateRoute(false)
	gts.ParserPoolReleaseForTest(pool, p1)

	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()
	p2 := gts.ParserPoolCheckoutForTest(pool)
	defer gts.ParserPoolReleaseForTest(pool, p2)
	if !gts.ParserAdmissionEligibleForTest(p2) {
		t.Fatal("pooled parser did not scrub its route override; a forced-off override leaked across checkouts")
	}
}

// TestAdmissionSwitchEnvVarContract proves GTS_ADMISSION_CANDIDATE seeds the
// process-wide default: init calls this same parser at package load.
//
// Phase-3 admission made the compact route the default, so an unset or
// unrecognized value resolves ON; only an explicit off value forces the
// production route (the escape hatch).
func TestAdmissionSwitchEnvVarContract(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"1", true}, {"true", true}, {"on", true}, {"yes", true}, {"YES", true},
		{"", true}, {"nonsense", true}, {"On", true}, {"Yes", true},
		{"0", false}, {"false", false}, {"off", false}, {"no", false}, {"NO", false},
		{"Off", false}, {"No", false}, {"False", false}, {"OFF", false},
		{" off ", false}, {"Off\t", false},
	} {
		t.Setenv("GTS_ADMISSION_CANDIDATE", tc.value)
		if got := gts.AdmissionCandidateEnvEnabledForTest(); got != tc.want {
			t.Fatalf("GTS_ADMISSION_CANDIDATE=%q enabled=%v want %v", tc.value, got, tc.want)
		}
	}
}

// TestAdmissionSwitchDeclinesWhenLoggerAttached proves the candidate route
// preserves callback fidelity: with a logger attached and the switch on, the
// parse stays on production (no routing event) so every logger callback fires.
func TestAdmissionSwitchDeclinesWhenLoggerAttached(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()

	parser, source := newAdmissionDFAParser(t)
	parser.SetLogger(func(gts.ParserLogType, string) {})
	before := admissionRoutingEvents(t)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	requireCleanFullTree(t, tree, source, "logger-attached")
	if got := admissionRoutingEvents(t); got != before {
		t.Fatalf("an attached logger must keep the parse on production: %d -> %d", before, got)
	}
}

// TestAdmissionCandidateMemoryBudgetContractPreserved proves the automatic
// large-input memory budget contract survives tranche B9's removal of the
// source-length eligibility decline.
//
// Before tranche B9, the switch declined every input at or above
// parseRuntimeMemoryMinSourceBytes (64 KiB) outright: the compact scheduler
// never attempted them, so it never needed to poll the budget itself. Tranche
// B9 removed that decline; a large input now attempts the candidate route
// like any other. The tranche B8 scheduler stop-control poll is what keeps
// the contract intact: it compares the compact core's own deterministic
// StorageBytes() against the same soft budget production's arena/scratch
// accounting honors, declines (falls back to production) when the budget is
// exceeded, and releases the compact core's storage before returning -- so a
// pathological large input still stops at the configured budget and still
// reports ParseStopMemoryBudget, just via a different mechanism than before.
func TestAdmissionCandidateMemoryBudgetContractPreserved(t *testing.T) {
	minBudgetSourceBytes := gts.ParseRuntimeMemoryMinSourceBytesForTest()

	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)

	lang := grammars.GoLanguage()

	// Positive control: a small clean source routes.
	small := []byte(grammars.ParseSmokeSample("go"))
	if len(small) == 0 || len(small) >= minBudgetSourceBytes {
		t.Skipf("go smoke sample unsuitable for the control (%d bytes)", len(small))
	}
	gts.ResetAdmissionCandidateCountersForTest()
	smallParser := gts.NewParser(lang)
	smallTree, err := smallParser.Parse(small)
	if err != nil {
		t.Fatalf("small parse: %v", err)
	}
	smallTree.Release()
	compiledOut := false
	if routed, _ := gts.AdmissionCandidateCounters(); routed == 0 {
		// In the emergency opt-out build (-tags gts_no_parsercorephase0) the
		// compact engine is compiled out, so every eligible parse fails closed and
		// the routed counter cannot move. The budget contract this test proves
		// still holds trivially (every parse stays on production), so record that
		// and skip the routing-specific assertions below rather than fail.
		if reason := gts.AdmissionCandidateLastFallbackReason(); strings.Contains(reason, "compiled out") {
			compiledOut = true
		} else {
			t.Fatalf("small clean source did not route to the candidate; routed=%d", routed)
		}
	}

	// A clean source well above the former 64 KiB floor now routes too: no
	// eligibility decline stops it by length alone (tranche B9).
	var big bytes.Buffer
	big.WriteString("package p\n\nfunc f() {\n")
	for big.Len() < minBudgetSourceBytes+(1<<15) {
		big.WriteString("\t_ = 1\n")
	}
	big.WriteString("}\n")
	if big.Len() < minBudgetSourceBytes {
		t.Fatalf("large control source too small: %d", big.Len())
	}
	gts.ResetAdmissionCandidateCountersForTest()
	bigParser := gts.NewParser(lang)
	bigTree, err := bigParser.Parse(big.Bytes())
	if err != nil {
		t.Fatalf("large parse: %v", err)
	}
	bigTree.Release()
	if routed, fallback := gts.AdmissionCandidateCounters(); !compiledOut && (routed != 1 || fallback != 0) {
		t.Fatalf("source of %d bytes (>= %d former floor) did not route cleanly to the candidate: routed=%d fallback=%d reason=%q",
			big.Len(), minBudgetSourceBytes, routed, fallback, gts.AdmissionCandidateLastFallbackReason())
	}

	// With a low budget, a pathological large source still stops at
	// ParseStopMemoryBudget: the candidate route attempts it, the scheduler's
	// storage-based stop-control poll trips before completion (an engine
	// decline, not a never-attempted eligibility decline), and production
	// serves the fallback honoring the identical configured budget. The
	// decline must also release the compact core's storage before returning,
	// so production's fallback never runs alongside retained compact storage
	// (tranche B9 storage-release gate).
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "1")
	gts.ResetParseEnvConfigCacheForTests()
	defer gts.ResetParseEnvConfigCacheForTests()
	gts.DrainArenaPools()

	var huge bytes.Buffer
	huge.WriteString("package p\nfunc f() {\n")
	for i := 0; i < 20000; i++ {
		huge.WriteString("var x = 1\n")
	}
	huge.WriteString("}\n")
	gts.ResetAdmissionCandidateCountersForTest()
	hugeParser := gts.NewParser(lang)
	hugeTree, err := hugeParser.Parse(huge.Bytes())
	if err != nil {
		t.Fatalf("huge parse: %v", err)
	}
	defer hugeTree.Release()
	if got := hugeTree.ParseStopReason(); got != gts.ParseStopMemoryBudget {
		t.Fatalf("ParseStopReason() = %q, want %q (production must serve the fallback and honor the budget)",
			got, gts.ParseStopMemoryBudget)
	}
	if routed, fallback := gts.AdmissionCandidateCounters(); routed != 0 || (!compiledOut && fallback != 1) {
		t.Fatalf("budgeted huge source: routed=%d fallback=%d (want routed=0, fallback=1 unless compiled out)", routed, fallback)
	}
	// StorageBytes reads 0 after any Reset regardless of retained capacity
	// (it counts live length, not backing-array size), so it cannot detect a
	// decline that reset length but left a large arena retained -- assert
	// FootprintBytes instead, which reads real capacity. requireCompactFootprintReleased
	// bounds it well under the pre-fix retained level (157-193MB measured for
	// this decline class before the retention-cap gate existed).
	requireCompactFootprintReleased(t, hugeParser, "stop-control memory-budget decline")
}

// compactFootprintReleasedCapBytes bounds a "genuinely released" reading in
// the tests below. It intentionally sits above internal/parsercorephase0's
// own coreRetentionCapBytes (48 MiB): these tests assert the OUTCOME
// (retention stays in the same order of magnitude as production's own
// steady-state ~12-15MB, not the 157-193MB measured before the tranche B9
// retention-cap gate), not the exact internal threshold, so the two can
// evolve independently without coupling this test to that constant's value.
//
// Only one of the three requireCompactFootprintReleased call sites below is
// load-bearing against a deleted release call. Measured on the development
// host with release disabled: the stop-control decline's witness peaks
// around 1 MiB, and the acceptance-gate decline's witness peaks under
// 0.1 MiB -- both already well under this cap whether or not release runs,
// so their own assertions pass either way and prove only the OUTPUT (a
// small footprint), not that release produced it. The
// materialization-decline witness peaks around 77 MiB un-released, so it
// is the one call site here that actually fails when the release call is
// removed. Keep that witness large enough to clear this cap if it is ever
// resized.
const compactFootprintReleasedCapBytes = 64 << 20 // 64 MiB

// requireCompactFootprintReleased asserts p's cached admission-candidate
// runner's compact-core FootprintBytes (real retained capacity, not live
// length) stays well under the pre-fix retained level after a decline,
// proving the tranche B9 storage-release gate for the class of decline
// label names.
func requireCompactFootprintReleased(t *testing.T, p *gts.Parser, label string) {
	t.Helper()
	if got := gts.AdmissionCandidateCompactFootprintBytesForTest(p); got > compactFootprintReleasedCapBytes {
		t.Fatalf("%s: compact footprint retained after decline: %d bytes (want <= %d, released before production's fallback ran)",
			label, got, compactFootprintReleasedCapBytes)
	}
}

// TestAdmissionCandidateStorageReleasedOnAcceptanceGateDecline drives the
// acceptance-gate decline class specifically -- a different release path
// from TestAdmissionCandidateMemoryBudgetContractPreserved's stop-control
// class above. Here the compact scheduler run itself completes without a Go
// error (it never reaches an accepted EOF frontier, so
// RunFreshSchedulerSession commits rather than resets), and the runner's own
// strict sole-exact-EOF acceptance gate declines afterward
// (requireParserCoreFreshFullAcceptance, parsercore_phase0_fresh_full_runner.go).
// Before the tranche B9 reset-completeness gate, this specific path retained
// whatever the scheduler run had allocated until the next parse call on the
// same runner reset it lazily.
//
// The Go generic-instantiation/type-conversion ambiguity
// (TestAdmissionCandidateGoTypeConversionFailsClosed's witness) triggers
// this class naturally: the compact scheduler forks on the conflict but
// cannot rank the two arms by dynamic precedence, so it never reaches an
// accepted EOF frontier at all ("did not accept EOF") rather than hitting a
// hard scheduler error mid-run.
func TestAdmissionCandidateStorageReleasedOnAcceptanceGateDecline(t *testing.T) {
	src := "package p\n\n" +
		"type Foo[T any] struct {\n\tV T\n}\n\n" +
		"func f() {\n" +
		"\ta := Foo[int]{}\n" +
		"\tb := Foo[int](a)\n" +
		"\t_ = a\n" +
		"\t_ = b\n" +
		"}\n"
	lang := grammars.GoLanguage()
	parser := gts.NewParser(lang)
	tree, ok, reason := gts.TryCompactFullParseRouteForTest(parser, []byte(src))
	if ok || tree != nil {
		t.Fatalf("candidate engine accepted instead of declining at the acceptance gate (reason=%q); "+
			"re-verify TestAdmissionCandidateGoTypeConversionFailsClosed's witness still forks this conflict", reason)
	}
	if !strings.Contains(reason, "did not accept EOF") {
		t.Fatalf("decline reason = %q, want the acceptance-gate \"did not accept EOF\" class "+
			"(a different decline no longer exercises this path; this test needs a new acceptance-gate witness)", reason)
	}
	requireCompactFootprintReleased(t, parser, "acceptance-gate decline")
}

// TestAdmissionCandidateStorageReleasedOnMaterializationDecline drives the
// materialization-decline class specifically: the scheduler accepts a clean
// EOF frontier (the full accepted derivation graph is now committed to the
// compact core), and then a stop-control check inside materialization
// itself (parser.resultMaterializationStopReason, parsercore_phase0_driver.go
// -- not the scheduler's own dispatch-loop poll) trips. Before the tranche
// B9 reset-completeness gate, this path released nothing at all: the core
// held the entire accepted parse graph -- the largest possible retention
// for a given witness, since acceptance is a precondition for reaching
// materialization at all -- while production's fallback ran beside it.
//
// The witness is a ~315KB clean Go source (45,000 "_ = 1" statements), well
// past this file's other two decline witnesses: its accepted derivation
// graph's un-released FootprintBytes measures around 77 MiB on the
// development host, clearing compactFootprintReleasedCapBytes (64 MiB). This
// makes the requireCompactFootprintReleased call below load-bearing -- see
// that constant's doc comment -- unlike the stop-control and acceptance-gate
// witnesses, which stay under the cap whether or not release runs.
//
// A timeout in the microsecond band where the scheduler itself has already
// accepted but materialization has not yet finished reliably reproduces
// this: too short and the scheduler's own dispatch-loop poll trips first (a
// different, already-covered path, "scheduler stop-control tripped"); too
// long and materialization finishes before the poll fires at all (a
// successful route, not a decline). The band below is measured against this
// witness on the development host and swept coarsely, resetting the
// counters each attempt so a stale reason from an earlier iteration or an
// earlier test cannot be misread as this iteration's outcome, to absorb
// scheduling jitter; like every other timeout-banded test in this file it
// may need retuning against a materially different host, so an unreproduced
// band skips rather than fails.
func TestAdmissionCandidateStorageReleasedOnMaterializationDecline(t *testing.T) {
	lang := grammars.GoLanguage()
	var src bytes.Buffer
	src.WriteString("package p\n\nfunc f() {\n")
	for i := 0; i < 45000; i++ {
		src.WriteString("\t_ = 1\n")
	}
	src.WriteString("}\n")

	const loBandMicros, hiBandMicros, stepMicros = 250_000, 600_000, 10_000
	var lastReason string
	for us := loBandMicros; us <= hiBandMicros; us += stepMicros {
		gts.DrainArenaPools()
		gts.ResetAdmissionCandidateCountersForTest()
		parser := gts.NewParser(lang)
		parser.SetTimeoutMicros(uint64(us))
		tree, err := parser.Parse(src.Bytes())
		if err != nil {
			t.Fatalf("timeout=%dus: Parse() error = %v", us, err)
		}
		lastReason = gts.AdmissionCandidateLastFallbackReason()
		tree.Release()
		if strings.Contains(lastReason, "accepted-tree materialization stopped: timeout") {
			requireCompactFootprintReleased(t, parser, "materialization decline")
			return
		}
	}
	t.Skipf("no timeout in [%d, %d]us band reproduced a materialization-band timeout decline on this host; last reason=%q "+
		"(the band may need retuning against the current witness or host)",
		loBandMicros, hiBandMicros, lastReason)
}

// TestAdmissionCandidateGoTypeConversionFailsClosed proves the compact candidate
// route FAILS CLOSED on the Go generic-instantiation / type-conversion GLR
// conflict the Phase-3 admission flip surfaced. On the ambiguous construct
// `Foo[int](a)` (a type conversion of a generic type versus a call of an index
// expression) production and the tree-sitter-go C oracle resolve
// type_conversion_expression(generic_type). The compact scheduler forks the same
// conflict but cannot rank the two arms by dynamic precedence, so it declines the
// unauthorized tie fold (see Core.condenseWithOutcomeAtomic) instead of silently keeping
// one arm by insertion order. The parse then falls back to production.
//
// This was a tracked correctness divergence (call_expression(index_expression)
// routed with no fallback). The fix is loud and explicit here: the candidate must
// decline (route counter stays 0, fallback counter moves) and the returned tree
// must equal production byte for byte. If a future change routes this input, the
// route/fallback assertion fails and directs the maintainer to re-verify the
// whole conflict family in TestAdmissionGenericConflictFamilyNoDivergence.
func TestAdmissionCandidateGoTypeConversionFailsClosed(t *testing.T) {
	src := "package p\n\n" +
		"type Foo[T any] struct {\n\tV T\n}\n\n" +
		"func f() {\n" +
		"\ta := Foo[int]{}\n" +
		"\tb := Foo[int](a)\n" +
		"\t_ = a\n" +
		"\t_ = b\n" +
		"}\n"
	lang := grammars.GoLanguage()

	prod := gts.NewParser(lang)
	prod.SetAdmissionCandidateRoute(false)
	prodTree, err := prod.Parse([]byte(src))
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer prodTree.Release()
	prodSExpr := prodTree.RootNode().SExpr(lang)
	if !strings.Contains(prodSExpr, "type_conversion_expression") {
		t.Fatalf("production reference lost type_conversion_expression; the C-oracle witness changed: %s", prodSExpr)
	}

	gts.ResetAdmissionCandidateCountersForTest()
	cand := gts.NewParser(lang)
	cand.SetAdmissionCandidateRoute(true)
	candTree, err := cand.Parse([]byte(src))
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer candTree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()
	candSExpr := candTree.RootNode().SExpr(lang)

	// Fail-closed proof 1: the candidate declined this conflict input.
	if routed != 0 {
		t.Fatalf("conflict input routed (routed=%d fallback=%d); the compact route must fail closed on the "+
			"generic-instantiation / type-conversion conflict. Re-verify TestAdmissionGenericConflictFamilyNoDivergence.\n"+
			"  candidate: %s", routed, fallback, candSExpr)
	}
	// Fail-closed proof 2: the decline moved the fallback counter (the parse ran
	// production, it did not merely skip).
	if fallback == 0 {
		t.Fatalf("candidate declined but the fallback counter did not move (routed=%d fallback=%d); "+
			"the fail-closed path is not exercised", routed, fallback)
	}
	// Fail-closed proof 3: the returned tree is exactly production's tree.
	if candSExpr != prodSExpr {
		t.Fatalf("declined candidate tree diverges from production:\n  production: %s\n  candidate:  %s",
			prodSExpr, candSExpr)
	}
}

// admissionEOFPoint returns the row/column point at the end of source.
func admissionEOFPoint(source []byte) gts.Point {
	var row, col uint32
	for _, b := range source {
		if b == '\n' {
			row++
			col = 0
			continue
		}
		col++
	}
	return gts.Point{Row: row, Column: col}
}
