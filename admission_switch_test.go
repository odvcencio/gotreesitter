package gotreesitter_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/sched"
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

// Reuse attempts leave full-parse counters unchanged. Eligible EOF appends
// discard the old EOF reductions and count one fresh full-parse attempt.
func TestAdmissionSwitchParseIncrementalDoesNotCountFullCandidate(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()

	for _, atEOF := range []bool{false, true} {
		parser, source := newAdmissionDFAParser(t)
		oldTree, err := parser.Parse(source)
		if err != nil {
			t.Fatalf("fresh parse: %v", err)
		}
		defer oldTree.Release()
		before := admissionRoutingEvents(t)

		edited, edit := admissionSpaceInsertion(t, source, atEOF)
		oldTree.Edit(edit)
		newTree, err := parser.ParseIncremental(edited, oldTree)
		if err != nil {
			t.Fatalf("incremental parse: %v", err)
		}
		if newTree != nil && newTree != oldTree {
			defer newTree.Release()
		}
		want := before
		if atEOF {
			want++
		}
		if got := admissionRoutingEvents(t); got != want {
			t.Fatalf("ParseIncremental atEOF=%t full-route counts: %d -> %d, want %d", atEOF, before, got, want)
		}
		requireCleanFullTree(t, newTree, edited, "incremental")
	}
}

// Scanner refusal requires a fresh result without a second full-parse event.
func TestAdmissionSwitchScannerFallbackDoesNotCountFullCandidate(t *testing.T) {
	for _, profiled := range []bool{false, true} {
		t.Run(fmt.Sprint(profiled), func(t *testing.T) {
			gts.ResetAdmissionCandidateCountersForTest()
			parser := gts.NewParser(grammars.YamlLanguage())
			parser.SetAdmissionCandidateRoute(true)
			source, edited := []byte("[\n"), []byte("[]\n")
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			routed, fallback := gts.AdmissionCandidateCounters()
			reason := gts.AdmissionCandidateLastFallbackReason()
			old.Edit(gts.InputEdit{
				StartByte: 1, OldEndByte: 1, NewEndByte: 2,
				StartPoint: gts.Point{Column: 1}, OldEndPoint: gts.Point{Column: 1}, NewEndPoint: gts.Point{Column: 2},
			})
			var next *gts.Tree
			if profiled {
				next, _, err = parser.ParseIncrementalProfiled(edited, old)
			} else {
				next, err = parser.ParseIncremental(edited, old)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			if gotRouted, gotFallback := gts.AdmissionCandidateCounters(); gotRouted != routed || gotFallback != fallback || gts.AdmissionCandidateLastFallbackReason() != reason {
				t.Fatalf("scanner fallback changed full admission events: %d/%d -> %d/%d", routed, fallback, gotRouted, gotFallback)
			}
			fresh, err := parser.Parse(edited)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			requireIncrementalDeepTreeMatchesFresh(t, next, fresh, fresh.Language())
			if gotRouted, gotFallback := gts.AdmissionCandidateCounters(); gotRouted+gotFallback != routed+fallback+1 {
				t.Fatal("counter suppression leaked into the following public fresh parse")
			}
		})
	}
}

// TestAdmissionSwitchCustomTokenSourceNeverConsultsCandidate proves that a
// caller-supplied token source is never eligible: the seam declines before any
// candidate attempt, so no counter moves.
func TestAdmissionSwitchCustomTokenSourceNeverConsultsCandidate(t *testing.T) {
	// Find any registered token-source-backed language (java is one).
	var entry *grammars.LangEntry
	for _, name := range []string{"java", "go", "typescript", "tsx", "javascript"} {
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
	parser.SetIncludedRanges([]gts.Range{{StartByte: 0, EndByte: uint32(len(source)), EndPoint: pointForOffset(source, len(source))}})
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
// The compact route is OFF by default, so an unset or unrecognized value
// resolves OFF; only an explicit on value opts back in (the escape hatch).
func TestAdmissionSwitchEnvVarContract(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"1", true}, {"true", true}, {"on", true}, {"yes", true}, {"YES", true},
		{"On", true}, {"Yes", true},
		{"", false}, {"nonsense", false},
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

// admissionSpaceInsertion inserts before the last byte for an interior edit,
// or after it for an EOF append. Both change the source length, so neither
// can be served by the same-width token-invariant probe.
func admissionSpaceInsertion(t *testing.T, source []byte, atEOF bool) ([]byte, gts.InputEdit) {
	t.Helper()
	if len(source) == 0 {
		t.Fatal("space-insertion fixture is empty")
	}
	offset := len(source)
	if !atEOF {
		offset--
	}
	edited := make([]byte, 0, len(source)+1)
	edited = append(edited, source[:offset]...)
	edited = append(edited, ' ')
	edited = append(edited, source[offset:]...)
	point := pointForOffset(source, offset)
	return edited, gts.InputEdit{
		StartByte: uint32(offset), OldEndByte: uint32(offset), NewEndByte: uint32(offset + 1),
		StartPoint: point, OldEndPoint: point, NewEndPoint: gts.Point{Row: point.Row, Column: point.Column + 1},
	}
}

// TestSchedRequestModesMatchCompactStarts compares the capability table with
// the routes that the root package takes today. For each public call it
// builds the complete internal/sched request, runs the call with the
// admission switch on, and checks that the compact engine starts exactly when
// the request needs no open flag.
func TestSchedRequestModesMatchCompactStarts(t *testing.T) {
	restore := gts.AdmissionCandidateRouteDefault()
	defer gts.SetAdmissionCandidateRouteDefault(restore)
	gts.SetAdmissionCandidateRouteDefault(true)
	gts.ResetAdmissionCandidateCountersForTest()

	base, source := newAdmissionDFAParser(t)
	lang := base.Language()
	newParser := func(configure func(*gts.Parser)) *gts.Parser {
		p := gts.NewParser(lang)
		if configure != nil {
			configure(p)
		}
		return p
	}
	check := func(name string, req sched.Request, started bool) {
		t.Helper()
		if started != sched.Supports(req) {
			t.Fatalf("%s: compact started=%v, but request modes=%#x open=%#x", name, started, req.Modes, req.OpenModes())
		}
		t.Logf("%-52s compact started=%-5v modes=%#05x open=%#05x", name, started, req.Modes, req.OpenModes())
	}
	fresh := func(name string, p *gts.Parser, entry sched.Mode, parse func(*gts.Parser) (*gts.Tree, error)) {
		t.Helper()
		req := p.SchedRequestForTest(entry, nil)
		before := admissionRoutingEvents(t)
		tree, err := parse(p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		tree.Release()
		check(name, req, admissionRoutingEvents(t) != before)
	}
	parse := func(p *gts.Parser) (*gts.Tree, error) { return p.Parse(source) }
	var cancel uint32

	fresh("Parse", newParser(nil), 0, parse)
	compactCompiledIn := !strings.Contains(gts.AdmissionCandidateLastFallbackReason(), "compiled out")
	fresh("ParseStrict", newParser(nil), sched.Strict, func(p *gts.Parser) (*gts.Tree, error) { return p.ParseStrict(source) })
	fresh("ParseUTF16", newParser(nil), sched.UTF16, func(p *gts.Parser) (*gts.Tree, error) {
		return p.ParseUTF16(utf16.Encode([]rune(string(source))))
	})
	fresh("ParseWith", newParser(nil), 0, func(p *gts.Parser) (*gts.Tree, error) {
		result, err := p.ParseWith(source)
		return result.Tree, err
	})
	fresh("timeout", newParser(func(p *gts.Parser) { p.SetTimeoutMicros(60_000_000) }), 0, parse)
	fresh("cancellation flag", newParser(func(p *gts.Parser) { p.SetCancellationFlag(&cancel) }), 0, parse)
	fresh("included ranges", newParser(func(p *gts.Parser) {
		p.SetIncludedRanges([]gts.Range{{StartByte: 0, EndByte: uint32(len(source)), EndPoint: pointForOffset(source, len(source))}})
	}), 0, parse)
	fresh("logger", newParser(func(p *gts.Parser) { p.SetLogger(func(gts.ParserLogType, string) {}) }), 0, parse)
	fresh("ambiguity profile", newParser(func(p *gts.Parser) { p.SetAmbiguityProfile(gts.NewAmbiguityProfile()) }), 0, parse)
	fresh("work limits", newParser(func(p *gts.Parser) {
		p.SetParseWorkLimits(gts.ParseWorkLimits{IterationLimit: 1 << 30})
	}), 0, parse)
	fresh("ParseNoTreeBenchmarkOnly", newParser(nil), sched.Measurement, func(p *gts.Parser) (*gts.Tree, error) {
		return p.ParseNoTreeBenchmarkOnly(source)
	})
	fresh("ParseNoResultCompatibilityBenchmarkOnly", newParser(nil), sched.Measurement, func(p *gts.Parser) (*gts.Tree, error) {
		return p.ParseNoResultCompatibilityBenchmarkOnly(source)
	})
	for _, name := range []string{"java", "go", "typescript", "tsx", "javascript"} {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil || entry.TokenSourceFactory == nil {
			continue
		}
		tsLang := entry.Language()
		tsSource := []byte(grammars.ParseSmokeSample(entry.Name))
		fresh("ParseWithTokenSource "+name, gts.NewParser(tsLang), sched.TokenSource, func(p *gts.Parser) (*gts.Tree, error) {
			return p.ParseWithTokenSource(tsSource, entry.TokenSourceFactory(tsSource, tsLang))
		})
		break
	}
	if entry := grammars.DetectLanguageByName("json5"); entry != nil && gts.LanguageWantsForest(entry.Language()) {
		forestLang := entry.Language()
		forestSource := []byte(grammars.ParseSmokeSample("json5"))
		forestParse := func(p *gts.Parser) (*gts.Tree, error) { return p.Parse(forestSource) }
		fresh("forest default", gts.NewParser(forestLang), 0, forestParse)
		forced := gts.NewParser(forestLang)
		forced.SetAdmissionCandidateRoute(true)
		fresh("forest with forced candidate", forced, 0, forestParse)
	}

	if !compactCompiledIn {
		t.Log("compact engine compiled out; incremental checks skipped")
		return
	}
	// Exercise reuse and EOF fresh fallback separately. Both edits change
	// length, so the same-width token-invariant probe does not apply.
	incremental := func(name string, p *gts.Parser, entry sched.Mode, input []byte, parseOld func(*gts.Parser) (*gts.Tree, error),
		reparse func(*gts.Parser, *gts.Tree, []byte) (*gts.Tree, error)) {
		t.Helper()
		for _, atEOF := range []bool{false, true} {
			old, err := parseOld(p)
			if err != nil {
				t.Fatalf("%s: fresh parse: %v", name, err)
			}
			defer old.Release()
			edited, edit := admissionSpaceInsertion(t, input, atEOF)
			old.Edit(edit)
			req := p.SchedRequestForTest(entry, old)
			before := admissionRoutingEvents(t)
			tree, err := reparse(p, old, edited)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if tree != old {
				defer tree.Release()
			}
			rt := tree.ParseRuntime()
			if atEOF {
				// The fallback discards oldTree, so scanner and old-tree reuse
				// flags do not constrain its fresh parse request.
				check(name+" (EOF fresh)", p.SchedRequestForTest(0, nil), admissionRoutingEvents(t) != before)
				if rt.CompactIncrementalReuseRoute {
					t.Fatalf("%s: EOF append reused the old tree", name)
				}
			} else {
				check(name+" (interior reuse)", req, rt.CompactIncrementalReuseRoute || rt.CompactIncrementalFullRecoveryRoute || rt.CompactIncrementalFallbackReason != "")
			}
		}
	}
	reparse := func(p *gts.Parser, old *gts.Tree, edited []byte) (*gts.Tree, error) {
		return p.ParseIncremental(edited, old)
	}
	incremental("ParseIncremental on a compact tree", newParser(nil), sched.Incremental, source, parse, reparse)
	incremental("ParseIncrementalProfiled on a compact tree", newParser(nil), sched.Incremental|sched.Profiling, source, parse,
		func(p *gts.Parser, old *gts.Tree, edited []byte) (*gts.Tree, error) {
			tree, _, err := p.ParseIncrementalProfiled(edited, old)
			return tree, err
		})
	incremental("ParseIncremental on a legacy tree", newParser(nil), sched.Incremental, source,
		func(p *gts.Parser) (*gts.Tree, error) {
			p.SetAdmissionCandidateRoute(false)
			defer p.ClearAdmissionCandidateRoute()
			return p.Parse(source)
		}, reparse)
	for _, name := range []string{"python", "css", "yaml", "markdown", "rust"} {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			continue
		}
		scannerLang := entry.Language()
		if scannerLang == nil || scannerLang.ExternalScanner == nil {
			continue
		}
		if stateless, ok := scannerLang.ExternalScanner.(gts.StatelessExternalScanner); ok && stateless.ExternalScannerIsStateless() {
			continue
		}
		scannerSource := []byte(grammars.ParseSmokeSample(name))
		incremental("ParseIncremental with a stateful scanner ("+name+")", gts.NewParser(scannerLang), sched.Incremental, scannerSource,
			func(p *gts.Parser) (*gts.Tree, error) { return p.Parse(scannerSource) },
			reparse)
		break
	}
}

const ccoreAllModes = 1 | 2 | 4
const ccoreModesIncr = ccoreAllModes | 8

func ccoreCertifyAll() func() {
	return gts.CcoreTestCertify(ccoreAllModes, "ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState")
}

func ccoreRouteLang(t testing.TB, name string) *gts.Language {
	t.Helper()
	e := grammars.DetectLanguageByName(name)
	if e == nil || e.Language() == nil {
		t.Fatalf("language %q unavailable: the test would compare nothing", name)
	}
	return e.Language()
}

func ccoreRouteParser(lang *gts.Language) *gts.Parser {
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	return p
}

func ccoreJSONSource(n int) []byte {
	var b strings.Builder
	b.WriteString("[\n")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",\n")
		}
		fmt.Fprintf(&b, "  {\"k\": %d, \"v\": [1, 2, \"x\"]}", i)
	}
	b.WriteString("\n]\n")
	return []byte(b.String())
}

func ccoreInsert(src []byte, at int, b byte) ([]byte, gts.InputEdit) {
	out := append(append(append([]byte(nil), src[:at]...), b), src[at:]...)
	p0 := ccorePoint(src, at)
	return out, gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1),
		StartPoint: p0, OldEndPoint: p0, NewEndPoint: ccorePoint(out, at+1)}
}

func ccorePoint(src []byte, at int) gts.Point {
	var pt gts.Point
	for _, b := range src[:at] {
		if b == '\n' {
			pt.Row, pt.Column = pt.Row+1, 0
		} else {
			pt.Column++
		}
	}
	return pt
}

func ccoreSExpr(tree *gts.Tree, lang *gts.Language) string { return tree.RootNode().SExpr(lang) }

func ccoreRouteSample(t testing.TB, name string) ([]byte, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("internal", "benchfixtures", "real_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest v1InvariantCorpusManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, row := range manifest.Entries {
		if row.Language == name && row.Role == "sample" && row.CommittedPath != "" {
			src, err := os.ReadFile(filepath.Join("internal", "benchfixtures", row.CommittedPath))
			if err != nil {
				t.Fatal(err)
			}
			return src, "r4"
		}
	}
	return []byte(grammars.ParseSmokeSample(name)), "smoke"
}

func ccoreDeepJSON(depth int) []byte {
	return []byte(strings.Repeat("[", depth) + "1" + strings.Repeat("]", depth) + "\n")
}

func ccoreWideJSON(n int) []byte {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%d", i)
	}
	b.WriteString("]\n")
	return []byte(b.String())
}

func ccorePythonSource(minBytes int) []byte {
	var b strings.Builder
	for i := 0; b.Len() < minBytes; i++ {
		fmt.Fprintf(&b, "def f%d(a):\n    return a + %d\n\n", i, i)
	}
	return []byte(b.String())
}

func TestCcoreRouteChargesTheWholeFootprintDuringConversion(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	for _, c := range []struct {
		name string
		src  []byte
	}{{"deep", ccoreDeepJSON(2000)}, {"wide", ccoreWideJSON(30000)}} {
		t.Run(c.name, func(t *testing.T) {
			defer ccoreCertifyAll()()
			// The ledger counts the capacity a request starts with and every array it makes, so a
			// parser or an arena taken warm from a pool changes the total. The probe run and the
			// budget run both start cold: the pools are drained before each, so the totals are
			// equal and the test does not depend on what the collector did to the pools.
			cold := func() { gts.CcoreTestDrainPool(lang); gts.DrainArenaPools() }
			probe := func() (parse, total int) {
				var base, alloc, held int64
				stop := gts.CcoreTestAccountProbe(func(b, a, _, h int64) { base, alloc, held = b, a, h })
				cold()
				p := ccoreRouteParser(lang)
				p.SetMemoryBudgetBytes(1 << 40) // a limit that is never reached keeps the ledger on
				tr, err := p.Parse(c.src)
				stop()
				total, parse = int(base+alloc), int(base+alloc-held)
				if err != nil || tr == nil || !gts.CcoreTestFromCore(tr) {
					t.Fatalf("no budget: parse %v, linked %t", err, tr != nil && gts.CcoreTestFromCore(tr))
				}
				tr.Release()
				return
			}
			parse, total := probe()
			if total <= parse+1 {
				t.Fatalf("conversion added nothing to the footprint: parse %d, total %d", parse, total)
			}
			budget := int64(total - 1) // the parse passes it; the conversion does not
			pp := gts.NewParser(lang)
			pp.SetAdmissionCandidateRoute(false)
			pp.SetMemoryBudgetBytes(budget)
			present, _ := pp.Parse(c.src)
			var routed *gts.Tree
			var decl map[string]int64
			var served int64
			for attempt := 0; attempt < 5; attempt++ {
				cold()
				rp := ccoreRouteParser(lang)
				rp.SetMemoryBudgetBytes(budget)
				gts.CcoreTestReset()
				var err error
				routed, err = rp.Parse(c.src)
				if err != nil || routed == nil || present == nil {
					t.Fatalf("parse: %v", err)
				}
				decl, served = gts.CcoreTestCounters()
				if served == 0 {
					break
				}
				routed.Release()
				parse, total = probe()
				budget = int64(total - 1)
			}
			if served != 0 || decl["stop_memory_budget"] != 1 {
				t.Errorf("budget %d (parse %d, total %d): core served %d, declines %v", budget, parse, total, served, decl)
			}
			if routed.ParseRuntime().StopReason != present.ParseRuntime().StopReason {
				t.Errorf("routed stop %q, present engine %q", routed.ParseRuntime().StopReason, present.ParseRuntime().StopReason)
			}
			if ccoreSExpr(routed, lang) != ccoreSExpr(present, lang) || routed.RootNode().EndByte() != present.RootNode().EndByte() {
				t.Errorf("the tree the certified arm returned after the decline differs from the present engine's tree")
			}
			t.Logf("%s: parse footprint %d, conversion total %d, present engine stop %q", c.name, parse, total, present.ParseRuntime().StopReason)
		})
	}
}

func TestCcoreRouteServesFreshAndOwnsNothing(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	defer ccoreCertifyAll()()
	src := ccoreJSONSource(200)
	p := ccoreRouteParser(lang)
	gts.CcoreTestReset()
	tree, err := p.Parse(src)
	if err != nil || tree == nil || !gts.CcoreTestFromCore(tree) {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	if n := gts.CcoreTestPooledRecords(lang); n != 0 {
		t.Fatalf("the core's arena holds %d records after the call", n)
	}
	edited, edit := ccoreInsert(src, 40, ' ')
	tree.Edit(edit) // the public tree's own edit
	next, err := p.ParseIncremental(edited, tree)
	if err != nil || next == nil || next == tree || !gts.CcoreTestFromCore(next) {
		t.Fatalf("ParseIncremental: %v", err)
	}
	defer next.Release()
	if decl, served := gts.CcoreTestCounters(); served != 2 || len(decl) != 0 {
		t.Fatalf("served %d fresh parses, declines %v; want 2 (the parse and the reparse)", served, decl)
	}
	fresh, _ := ccoreRouteParser(lang).Parse(edited)
	defer fresh.Release()
	if ccoreSExpr(next, lang) != ccoreSExpr(fresh, lang) {
		t.Fatal("the reparse differs from the fresh parse")
	}
	cp := next.Copy()
	defer cp.Release()
	if cp.RootNode().Child(0).ParseState() != next.RootNode().Child(0).ParseState() {
		t.Fatal("a copy of a core tree lost the meaning of ParseState")
	}
}

func TestCcoreRouteEnforcesEveryMemoryLimit(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	src := ccoreJSONSource(3000)
	defer ccoreCertifyAll()()
	check := func(name string, configure func(*gts.Parser)) {
		t.Helper()
		// 1. the route alone: no heap reading is involved
		rp := ccoreRouteParser(lang)
		configure(rp)
		gts.CcoreTestReset()
		tr, ok := gts.CcoreTestFullParse(rp, src)
		decl, served := gts.CcoreTestCounters()
		if tr != nil || ok || served != 0 || decl["stop_memory_budget"] != 1 {
			t.Errorf("%s: route alone: tree %t, ok %t, core served %d, declines %v; want no tree and one decline for memory", name, tr != nil, ok, served, decl)
		}
		// 2. the certified arm through Parse: the core tries, is declined once for memory, the present engine answers
		cp := ccoreRouteParser(lang)
		configure(cp)
		gts.CcoreTestReset()
		got, err := cp.Parse(src)
		decl, served = gts.CcoreTestCounters()
		if err != nil || got == nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		defer got.Release()
		reason := got.ParseRuntime().StopReason
		if served != 0 || decl["stop_memory_budget"] != 1 || gts.CcoreTestFromCore(got) {
			t.Errorf("%s: certified arm: core served %d, declines %v, tree from core %t; want the core declined once for memory and the present engine's tree", name, served, decl, gts.CcoreTestFromCore(got))
		}
		if reason != gts.ParseStopMemoryBudget && reason != gts.ParseStopAccepted {
			t.Errorf("%s: the present engine's answer after the decline has stop reason %q; want %q or %q", name, reason, gts.ParseStopMemoryBudget, gts.ParseStopAccepted)
		}
	}
	check("SetMemoryBudgetBytes", func(p *gts.Parser) { p.SetMemoryBudgetBytes(1 << 20) })
	t.Run("budget from the environment", func(t *testing.T) {
		t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "1")
		gts.ResetParseEnvConfigCacheForTests()
		t.Cleanup(gts.ResetParseEnvConfigCacheForTests)
		check("GOT_PARSE_MEMORY_BUDGET_MB", func(*gts.Parser) {})
	})
	// GOT_PARSE_MEMORY_HARD_CEILING_MB is the present engine's heap-growth backstop, armed
	// around the whole routed attempt (TestCcoreRouteHardCeilingSeesMemoryOutsideTheCoreAccounting).
	budget, _, hooked, atBudget := gts.CcoreTestLimits(gts.NewParser(lang), len(src))
	if budget <= 0 || !hooked || atBudget != gts.ParseStopMemoryBudget {
		t.Errorf("default budget %d, hook %t, poll at the budget gives %q", budget, hooked, atBudget)
	}
}

func TestCcoreRouteBudgetsMatchThePresentEngineInFreshProcesses(t *testing.T) {
	if os.Getenv("CCORE_BUDGET_PROCESSES") == "" {
		t.Skip("starts nine test processes; set CCORE_BUDGET_PROCESSES=1")
	}
	run := func(arm string, mib int) map[string]string {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCcoreRouteBudgetChild$", "-test.count=1", "-test.v")
		var env []string
		for _, kv := range os.Environ() { // a control variable inherited from the caller must not reach the child
			if !strings.HasPrefix(kv, "CCORE_BUDGET_CHILD=") && !strings.HasPrefix(kv, "CCORE_BUDGET_MIB=") {
				env = append(env, kv)
			}
		}
		cmd.Env = append(env, "CCORE_BUDGET_CHILD="+arm, fmt.Sprintf("CCORE_BUDGET_MIB=%d", mib), "GTS_TEST_CCORE_ROUTE=0", "GOMAXPROCS=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %d MiB: %v\n%s", arm, mib, err, out)
		}
		res := map[string]string{}
		for _, l := range strings.Split(string(out), "\n") {
			for _, key := range []string{"CCBUDGET ", "CCTREES ", "CCCORE "} {
				if strings.HasPrefix(l, key) {
					res[key] = strings.TrimPrefix(l, key)
				}
			}
		}
		if res["CCBUDGET "] == "" || res["CCTREES "] == "" || res["CCCORE "] == "" {
			t.Fatalf("%s %d MiB: incomplete result lines\n%s", arm, mib, out)
		}
		return res
	}
	compared := 0
	for _, mib := range []int{3, 4, 8} {
		off := run("off", mib)
		for _, arm := range []string{"uncertified", "certified"} {
			got := run(arm, mib)
			if got["CCBUDGET "] != off["CCBUDGET "] {
				t.Errorf("%d MiB, arm %s: reasons %s, route off gives %s", mib, arm, got["CCBUDGET "], off["CCBUDGET "])
			}
			if got["CCTREES "] != off["CCTREES "] {
				t.Errorf("%d MiB, arm %s: the returned trees differ from the route-off trees:\n got %s\nwant %s", mib, arm, got["CCTREES "], off["CCTREES "])
			}
			// The certified child must have asked the core and been declined for memory on every
			// parse, and served nothing: without certification all arms run the present engine
			// and the comparisons above would hold without the core.
			want := "served=0 stop_memory_budget=6 other=0"
			if arm == "uncertified" {
				want = "served=0 stop_memory_budget=0 other=0" // nothing certified: the route is not even asked
			}
			if got["CCCORE "] != want {
				t.Errorf("%d MiB, arm %s: core counters %q, want %q", mib, arm, got["CCCORE "], want)
			}
			compared++
		}
		if !strings.Contains(off["CCBUDGET "], "memory_budget") {
			t.Errorf("%d MiB: the present engine never stopped for memory (%s); the case proves nothing", mib, off["CCBUDGET "])
		}
	}
	if compared == 0 {
		t.Fatal("nothing was compared")
	}
	fmt.Printf("CCCOUNT test=TestCcoreRouteBudgetsMatchThePresentEngineInFreshProcesses set=budget checked=%d\n", compared)
}

func TestCcoreRouteBudgetChild(t *testing.T) {
	arm := os.Getenv("CCORE_BUDGET_CHILD")
	if arm == "" {
		t.Skip("run by TestCcoreRouteBudgetsMatchThePresentEngineInFreshProcesses")
	}
	var mib int
	fmt.Sscan(os.Getenv("CCORE_BUDGET_MIB"), &mib)
	if arm == "certified" {
		defer ccoreCertifyAll()()
	}
	lang := ccoreRouteLang(t, "json")
	src := ccoreJSONSource(3000)
	var out, trees []string
	gts.CcoreTestReset()
	for i := 0; i < 6; i++ {
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(arm != "off")
		p.SetMemoryBudgetBytes(int64(mib) << 20)
		tr, err := p.Parse(src)
		if err != nil || tr == nil {
			out = append(out, "err")
			trees = append(trees, "err")
			continue
		}
		out = append(out, string(tr.ParseRuntime().StopReason))
		root := tr.RootNode()
		h := sha256.Sum256([]byte(ccoreSExpr(tr, lang)))
		trees = append(trees, fmt.Sprintf("%d/%d/%t/%x", root.EndByte(), root.ChildCount(), root.HasError(), h[:4]))
		tr.Release()
	}
	decl, served := gts.CcoreTestCounters()
	other := int64(0)
	for k, v := range decl {
		if k != "stop_memory_budget" {
			other += v
		}
	}
	fmt.Printf("CCBUDGET %s\nCCTREES %s\nCCCORE served=%d stop_memory_budget=%d other=%d\n", strings.Join(out, ","), strings.Join(trees, ","), served, decl["stop_memory_budget"], other)
}

func TestCcoreFieldLookupPassesOverAHiddenChildWithoutVisibleDescendants(t *testing.T) {
	got, found := gts.CcoreTestHiddenInvisibleFieldShape()
	if !found || got != 3 {
		t.Fatalf("lookup of field 1 gave node %d (found %t), want the second child, node 3", got, found)
	}
}

func TestCcoreFieldLookupReturnsTheOccurrenceOfAnInlineLeaf(t *testing.T) {
	path, ok := gts.CcoreTestFieldOccurrence()
	if !ok || len(path) != 1 || path[0] != 1 {
		t.Fatalf("field 1 gave path %v (found %t), want [1]: the leaf inside the hidden child, public child 1", path, ok)
	}
}

func ccoreSkipUnderRace(t *testing.T, why string) {
	t.Helper()
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range bi.Settings {
			if kv.Key == "-race" && kv.Value == "true" {
				t.Skip(why)
			}
		}
	}
}

func ccoreObjectJSON(pairs int) []byte {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < pairs; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "\"k%d\":%d", i, i)
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

func ccoreNestedArrays(n int) []byte {
	return []byte(strings.Repeat("[", n) + strings.Repeat("]", n) + "\n")
}

func TestCcoreLedgerEqualsTheRealAllocation(t *testing.T) {
	ccoreSkipUnderRace(t, "the race detector adds allocations of its own to the TotalAlloc bound")
	defer ccoreCertifyAll()()
	lang := ccoreRouteLang(t, "json")
	for _, c := range []struct {
		name string
		src  []byte
	}{
		{"array of 300000", ccoreWideJSON(300000)},
		{"2000 nested arrays", ccoreNestedArrays(2000)},
		{"object of 60000 pairs", ccoreObjectJSON(60000)},
		{"20000 unbalanced brackets", []byte(strings.Repeat("[", 20000))},
	} {
		t.Run(c.name, func(t *testing.T) {
			var at0, at1 runtime.MemStats
			stopLease := gts.CcoreTestParserLease(func(_ uintptr, before bool) {
				if before {
					runtime.ReadMemStats(&at0)
				}
			})
			defer stopLease()
			var total int64
			stopAcc := gts.CcoreTestAccountProbe(func(b, a, _, _ int64) { runtime.ReadMemStats(&at1); total = b + a })
			defer stopAcc()
			gts.CcoreTestDrainPool(lang)
			gts.DrainArenaPools()
			p := ccoreRouteParser(lang)
			p.SetMemoryBudgetBytes(1 << 40)
			gts.CcoreTestReset()
			tr, ok := gts.CcoreTestFullParse(p, c.src)
			if tr != nil {
				tr.Release()
			}
			if total == 0 {
				t.Fatalf("the account hook never ran (ok %t): the request did not reach the end of the conversion", ok)
			}
			real := int64(at1.TotalAlloc - at0.TotalAlloc)
			t.Logf("counted %d, allocated %d (%.1f%%)", total, real, 100*float64(total)/float64(real))
			if float64(total) < 0.90*float64(real) || float64(total) > 1.05*float64(real)+16384 {
				t.Fatalf("counted %d bytes for %d bytes really allocated; want 0.90 x allocated <= counted <= 1.05 x allocated + 16384", total, real)
			}
		})
	}
}

func ccoreCertifyIncr() func() {
	return gts.CcoreTestCertify(ccoreModesIncr, "ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState")
}

func ccoreWalkDigest(tree *gts.Tree, lang *gts.Language) string {
	h := sha256.New()
	var visit func(n *gts.Node, field string)
	visit = func(n *gts.Node, field string) {
		s, e := n.StartPoint(), n.EndPoint()
		fmt.Fprintf(h, "%s|%d|%d|%d.%d|%d.%d|%v|%v|%v|%s;", n.Type(lang), n.StartByte(), n.EndByte(), s.Row, s.Column, e.Row, e.Column, n.IsNamed(), n.IsMissing(), n.HasError(), field)
		for i := 0; i < n.ChildCount(); i++ {
			visit(n.Child(i), n.FieldNameForChild(i, lang))
		}
	}
	visit(tree.RootNode(), "")
	return fmt.Sprintf("%x", h.Sum(nil))
}

func ccoreReplace(src []byte, at int, b byte) ([]byte, gts.InputEdit) {
	out := append([]byte(nil), src...)
	out[at] = b
	p0 := ccorePoint(src, at)
	return out, gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1),
		StartPoint: p0, OldEndPoint: ccorePoint(src, at+1), NewEndPoint: ccorePoint(out, at+1)}
}

func ccoreDeleteByte(src []byte, at int) ([]byte, gts.InputEdit) {
	out := append(append([]byte(nil), src[:at]...), src[at+1:]...)
	p0 := ccorePoint(src, at)
	return out, gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at),
		StartPoint: p0, OldEndPoint: ccorePoint(src, at+1), NewEndPoint: p0}
}

func ccoreGateSource(lang string, n int) []byte {
	switch lang {
	case "json":
		return ccoreJSONSource(n / 12)
	case "python":
		return ccorePythonSource(n)
	}
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		if lang == "c" {
			fmt.Fprintf(&b, "int f%d(int a) { return a + %d; }\n", i, i)
		} else {
			fmt.Fprintf(&b, "function f%d(a: number): number { return a + %d; }\n", i, i)
		}
	}
	return []byte(b.String())
}

func TestCcoreInvariantGateThroughTheRoute(t *testing.T) {
	defer ccoreCertifyIncr()()
	for _, name := range []string{"json", "python", "c", "typescript"} {
		t.Run(name, func(t *testing.T) {
			lang := ccoreRouteLang(t, name)
			src := ccoreGateSource(name, 4000)
			p, fp := ccoreRouteParser(lang), ccoreRouteParser(lang)
			gts.CcoreTestReset()
			tree, err := p.Parse(src)
			if err != nil || tree == nil || !gts.CcoreTestFromCore(tree) {
				t.Fatalf("setup: %v", err)
			}
			steps, errSteps := 0, 0
			for i := 0; i < 72; i++ {
				at := 3 + (i*131)%(len(src)-8)
				for at < len(src)-3 && !(src[at] >= '0' && src[at] <= '8') {
					at++
				}
				var next []byte
				var e gts.InputEdit
				switch {
				case i%12 == 5: // a syntax break
					next, e = ccoreInsert(src, 4, '}')
				case i%12 == 6: // and its repair
					next, e = ccoreDeleteByte(src, 4)
				case i%3 == 0:
					next, e = ccoreReplace(src, at, src[at]+1)
				case i%3 == 1:
					next, e = ccoreInsert(src, at, '1')
				default:
					next, e = ccoreInsert(src, at, '2')
				}
				tree.Edit(e)
				nt, err := p.ParseIncremental(next, tree)
				if err != nil || nt == nil {
					t.Fatalf("step %d: %v", i, err)
				}
				fresh, err := fp.Parse(next)
				if err != nil {
					t.Fatal(err)
				}
				if ccoreWalkDigest(nt, lang) != ccoreWalkDigest(fresh, lang) {
					t.Fatalf("step %d: the incremental tree differs from a fresh parse", i)
				}
				root := nt.RootNode()
				if root.Type(lang) == "ERROR" && !root.HasError() {
					t.Fatalf("step %d: an ERROR root without HasError", i)
				}
				if root.HasError() {
					errSteps++
				}
				if int(root.EndByte()) != len(next) {
					t.Fatalf("step %d: the root ends at %d, the input has %d bytes", i, root.EndByte(), len(next))
				}
				fresh.Release()
				tree.Release()
				tree, src = nt, next
				steps++
			}
			// A reparse without edits returns the same tree and allocates nothing. Each closure
			// checks for a parse error and releases a handle that is not the tree it started with.
			var out *gts.Tree
			var reparseErr error
			routeAllocs := testing.AllocsPerRun(50, func() {
				var o *gts.Tree
				o, err := p.ParseIncremental(src, tree)
				if err != nil && reparseErr == nil {
					reparseErr = err
				}
				if o != nil {
					out = o
					o.Release() // every non-nil handle, also the old tree itself: the unchanged-tree path adds one each call
				}
			})
			if reparseErr != nil {
				t.Fatalf("a reparse without edits: %v", reparseErr)
			}
			if out != tree {
				t.Errorf("a reparse without edits returned another tree")
			}
			if n := gts.TreeExtraHandles(tree); n != 0 {
				t.Errorf("%d handles of the tree are outstanding after the measured reparses", n)
			}
			if routeAllocs != 0 {
				t.Errorf("a reparse without edits allocated %v times, want 0", routeAllocs)
			}
			off := ccoreRouteParser(lang)
			off.SetAdmissionCandidateRoute(false)
			presentTree, perr := off.Parse(src)
			if perr != nil || presentTree == nil {
				t.Fatalf("present engine parse: %v", perr)
			}
			var presentErr error
			presentAllocs := testing.AllocsPerRun(50, func() {
				o, err := off.ParseIncremental(src, presentTree)
				if err != nil && presentErr == nil {
					presentErr = err
				}
				if o != nil {
					o.Release()
				}
			})
			if presentErr != nil {
				t.Fatalf("present engine reparse without edits: %v", presentErr)
			}
			t.Logf("CCNOEDITALLOC lang=%s route_allocs_per_reparse=%v present_engine_allocs_per_reparse=%v steps=%d error_steps=%d", name, routeAllocs, presentAllocs, steps, errSteps)
			if n := gts.TreeExtraHandles(presentTree); n != 0 {
				t.Errorf("%d handles of the present engine's tree are outstanding after the measured reparses", n)
			}
			presentTree.Release()
			tree.Release()
			if errSteps == 0 {
				t.Errorf("no step had an error: the break steps proved nothing")
			}
		})
	}
}

func TestCcoreFieldLookupEqualsTheNodeReaders(t *testing.T) {
	defer ccoreCertifyAll()()
	for _, name := range []string{"json", "python", "c", "typescript"} {
		t.Run(name, func(t *testing.T) {
			lang := ccoreRouteLang(t, name)
			gts.CcoreTestReset()
			// the gate input, and one text with equal leaves next to each other under different fields
			// (equal inline leaves share one record, so a lookup must name the occurrence)
			dup := map[string]string{
				"json":       `[1,1,[1,1],{"a":"a","b":"b"},[[1,1],[1,1]],{"a":{"a":{"a":"a"}}}]`,
				"python":     "a = a\nf(a, a)\nx = a + a\nx[a] = a\nb = a if a else a\nc = [a for a in a]\nd = {a: a}\na, a = a, a\nprint(a, a)\na.a.a(a)\nlambda a: a\nclass a(a): a = a\ndef a(a=a, *a, **a): return a\nfor a in a: a\nwith a as a: a\nimport a as a\nassert a, a\na[a:a]\na += a\nnot a == a\ntry:\n    a\nexcept a as a:\n    a\n",
				"c":          "int g(int a) { a = a; if (a == a) { return a + a; } x[a] = a; f(a, a); return a ? a : a; }\nint a = a;\nstruct a { int a; } a;\nvoid a(a a, a a) { a.a = a->a; a(a)(a); while (a) a++; for (a = a; a < a; a++) a; switch (a) { case a: a; } *a = &a; a = (a)a; a = sizeof(a); }\n",
				"typescript": "function g(a: number) { a = a; if (a == a) { return a + a; } x[a] = a; f(a, a); return a ? a : a; }\nclass a extends a { a: a = a; a(a: a): a { return a.a(a); } }\ninterface a { a: a; a(a: a): a }\nconst {a, a: a} = a; let [a, a] = a;\na => a; (a) => a; a<a>(a); new a(a); a as a; a!.a; a?.a; for (a of a) a; for (a in a) a; type a = a | a;\nimport a, {a as a} from 'a';\n",
			}[name]
			sample, _ := ccoreRouteSample(t, name)
			for _, src := range [][]byte{ccoreGateSource(name, 4000), []byte(dup), sample} {
				tree, err := ccoreRouteParser(lang).Parse(src)
				if err != nil || tree == nil || !gts.CcoreTestFromCore(tree) {
					t.Fatalf("setup: not served by the core: %v", err)
				}
				ccoreCompareFieldLookup(t, tree, lang, len(src) > 1000)
				tree.Release()
			}
		})
	}
}

func TestCcoreFieldLookupEqualsTheNodeReadersOnAnInlineLeafInAHiddenChild(t *testing.T) {
	path, ok := gts.CcoreTestFieldOccurrence()
	if !ok || len(path) != 1 || path[0] != 1 {
		t.Fatalf("conversion answered path %v (found %t); the readers' answer is public child 1", path, ok)
	}
}

func ccoreCompareFieldLookup(t *testing.T, tree *gts.Tree, lang *gts.Language, big bool) {
	t.Helper()
	{
		{
			nodes, lookups, diffs := gts.CcoreTestFieldLookupVsReaders(tree, lang)
			for _, d := range diffs {
				t.Error(d)
			}
			if (big && nodes < 100) || lookups < nodes {
				t.Fatalf("too little compared: %d nodes, %d lookups", nodes, lookups)
			}
			t.Logf("%d nodes, %d lookups compared, %d differences", nodes, lookups, len(diffs))
		}
	}
}

func TestCcoreDroppedTreeCopyIsCollectable(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	defer ccoreCertifyIncr()()
	src := ccoreJSONSource(120)
	keep, err := ccoreRouteParser(lang).Parse(src) // stays alive: the copy is made from it
	if err != nil || keep == nil || !gts.CcoreTestFromCore(keep) {
		t.Fatalf("setup: %v", err)
	}
	defer keep.Release()
	if pre, err := ccoreRouteParser(lang).Parse(src); err == nil { // a released tree puts an arena in the pool
		pre.Release()
	}
	var freed atomic.Int32
	func() {
		cp := keep.Copy()
		ct := gts.CcoreTestCoreTree(cp)
		if ct == nil {
			t.Fatal("setup: the copy has no core tree")
		}
		runtime.SetFinalizer(ct, func(*ccore.Tree) { freed.Add(1) })
	}()
	for i := 0; i < 300 && freed.Load() < 1; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	if freed.Load() != 1 {
		t.Fatal("the core tree of a dropped copy was not collected")
	}
}

type heapScanner struct {
	gts.ExternalScanner
	keep unsafe.Pointer // *func(n int)
}

func ccoreKeepFn(f func(n int)) unsafe.Pointer { return unsafe.Pointer(&f) }
func ccoreCallKeep(p unsafe.Pointer, n int)    { (*(*func(int))(p))(n) }
func ccoreCountFn(f func() int) unsafe.Pointer { return unsafe.Pointer(&f) }
func ccoreCallCount(p unsafe.Pointer) int      { return (*(*func() int)(p))() }

func (h heapScanner) Scan(payload any, lex *gts.ExternalLexer, valid []bool) bool {
	ccoreCallKeep(h.keep, 1<<20)
	return h.ExternalScanner.Scan(payload, lex, valid)
}

// GOT_PARSE_MEMORY_HARD_CEILING_MB is the present engine's heap-growth backstop: a
// scanner that allocates outside the core's accounting stops the routed parse.
func TestCcoreRouteHardCeilingSeesMemoryOutsideTheCoreAccounting(t *testing.T) {
	py := ccoreRouteLang(t, "python")
	src := ccorePythonSource(80 << 10)
	t.Setenv("GOT_PARSE_MEMORY_HARD_CEILING_MB", "48")
	gts.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gts.ResetParseEnvConfigCacheForTests)
	defer ccoreCertifyAll()()
	lang := ccoreCopyLanguage(py)
	// The scanner keeps 1 MiB per call, but at most holdCap MiB in all. The core stops at its
	// first forced sample, after 256 calls. The request then falls back to the present
	// engine, which runs the same scanner: without the cap that parse kept one MiB for each of
	// the 31,509 calls of the input (about 31 GiB). The first run in a process got that memory
	// as fresh, untouched pages; a second run had to reuse and clear it and did not finish.
	const holdCap = 320
	var hold [][]byte
	lang.ExternalScanner = ccoreWrapped(py, func(in gts.ExternalScanner) gts.ExternalScanner {
		return heapScanner{ExternalScanner: in, keep: ccoreKeepFn(func(n int) {
			if len(hold) < holdCap {
				hold = append(hold, make([]byte, n))
			}
		})}
	})
	rp := ccoreRouteParser(lang)
	gts.CcoreTestReset()
	// The ceiling measures heap growth from the live heap when the request starts. Memory
	// that an earlier test or an earlier run of this test left as garbage would sit in that
	// baseline; the collector then frees it in the middle of the parse and the growth stays
	// below the ceiling although the scanner holds 300 MiB. Collect first.
	runtime.GC()
	runtime.GC()
	heldAtFirstSample, samples := -1, 0
	defer gts.CcoreTestPollProbe(func(total int64, sample bool, converting bool) {
		if sample {
			if samples++; samples == 1 {
				heldAtFirstSample = len(hold)
			}
		}
	})()
	tr, err := rp.Parse(src)
	if err != nil || tr == nil {
		t.Fatalf("parse: %v", err)
	}
	decl, served := gts.CcoreTestCounters()
	held := len(hold)
	hold = nil
	if served != 0 || decl["stop_memory_budget"] != 1 {
		t.Fatalf("the scanner held %d MiB outside the accounting; core served %d, declines %v", held, served, decl)
	}
	// The stop must come from the first forced heap sample inside the parse, not from the
	// check after the conversion.
	if heldAtFirstSample < 49 || heldAtFirstSample > holdCap {
		t.Fatalf("first forced sample saw %d MiB held (%d samples), want the sample inside the parse with more than 48 MiB", heldAtFirstSample, samples)
	}
	t.Logf("scanner held %d MiB at the first forced sample; routed result stop %q", heldAtFirstSample, tr.ParseRuntime().StopReason)
}

func ccoreCopyLanguage(l *gts.Language) *gts.Language {
	c := reflect.New(reflect.TypeOf(l).Elem())
	c.Elem().Set(reflect.ValueOf(l).Elem()) // a struct copy, as a caller's `c := *l` makes (reflect keeps go vet quiet)
	return c.Interface().(*gts.Language)
}

func ccoreWrapped(l *gts.Language, wrap func(gts.ExternalScanner) gts.ExternalScanner) gts.ExternalScanner {
	return gts.CcoreTestWrapScanner(l.ExternalScanner, wrap)
}

func ccoreColdRun(t *testing.T, lang *gts.Language, src []byte, budget int64) (base, alloc, charged, held int64, polls []ccorePoll, served bool, decl map[string]int64) {
	t.Helper()
	gts.CcoreTestDrainPool(lang)
	gts.DrainArenaPools()
	stop1 := gts.CcoreTestAccountProbe(func(b, a, c, h int64) { base, alloc, charged, held = b, a, c, h })
	defer stop1()
	stop2 := gts.CcoreTestPollProbe(func(total int64, sample, conv bool) { polls = append(polls, ccorePoll{total, sample, conv}) })
	defer stop2()
	p := ccoreRouteParser(lang)
	p.SetMemoryBudgetBytes(budget)
	gts.CcoreTestReset()
	tr, ok := gts.CcoreTestFullParse(p, src)
	if tr != nil {
		tr.Release()
	}
	var n int64
	decl, n = gts.CcoreTestCounters()
	return base, alloc, charged, held, polls, ok && n == 1, decl
}

type ccorePoll struct {
	total      int64
	sample     bool
	converting bool
}

func TestCcoreRoutePeakHeapIsAtMostThePresentEnginesPlusG(t *testing.T) {
	ccoreSkipUnderRace(t, "the race detector adds allocations of its own to the TotalAlloc bounds")
	defer ccoreCertifyAll()()
	json := ccoreRouteLang(t, "json")
	py := ccoreRouteLang(t, "python")
	type input struct {
		name string
		lang *gts.Language
		src  []byte
	}
	inputs := []input{
		{"array of 300000", json, ccoreWideJSON(300000)},
		{"2000 nested arrays", json, ccoreNestedArrays(2000)},
		{"object of 60000 pairs", json, ccoreObjectJSON(60000)},
		{"20000 unbalanced brackets", json, []byte(strings.Repeat("[", 20000))},
		{"python of 80 KiB", py, ccorePythonSource(80 << 10)},
	}
	old := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(old)
	alloc := func() uint64 { var m runtime.MemStats; runtime.ReadMemStats(&m); return m.TotalAlloc }
	for _, in := range inputs {
		base, a, _, _, _, served, decl := ccoreColdRun(t, in.lang, in.src, 1<<40)
		if !served {
			t.Errorf("%s: not served by the core (%v)", in.name, decl)
			continue
		}
		full := base + a
		for _, frac := range []int64{4, 2} {
			budget := full / frac
			gts.DrainArenaPools()
			runtime.GC()
			pp := gts.NewParser(in.lang)
			pp.SetAdmissionCandidateRoute(false)
			pp.SetMemoryBudgetBytes(budget)
			a0 := alloc()
			present, err := pp.Parse(in.src)
			presentGrown := int64(alloc() - a0)
			if err != nil || present == nil {
				t.Fatalf("%s: present engine: %v", in.name, err)
			}
			reason := present.ParseRuntime().StopReason
			present.Release()
			var routeGrown int64
			gts.CcoreTestDrainPool(in.lang)
			gts.DrainArenaPools()
			runtime.GC()
			rp := ccoreRouteParser(in.lang)
			rp.SetMemoryBudgetBytes(budget)
			gts.CcoreTestReset()
			b0 := alloc()
			tr, ok := gts.CcoreTestFullParse(rp, in.src)
			routeGrown = int64(alloc() - b0)
			if tr != nil {
				tr.Release()
			}
			d, _ := gts.CcoreTestCounters()
			t.Logf("%s, budget 1/%d of %d: present engine %q allocated %d; route allocated %d (declines %v)", in.name, frac, full, reason, presentGrown, routeGrown, d)
			if ok || d["stop_memory_budget"] != 1 {
				t.Errorf("%s, budget %d: the route served %t, declines %v; want a decline for memory", in.name, budget, ok, d)
			}
			if routeGrown > presentGrown+262144 {
				t.Errorf("%s, budget %d: the route allocated %d bytes, the present engine %d (stop %q); want the route at most %d", in.name, budget, routeGrown, presentGrown, reason, presentGrown+262144)
			}
		}
	}
}

func TestCcoreLedgerEqualsTheRealAllocationOnAReparse(t *testing.T) {
	ccoreSkipUnderRace(t, "the race detector adds allocations of its own to the TotalAlloc bound")
	defer ccoreCertifyIncr()()
	lang := ccoreRouteLang(t, "json")
	for _, c := range []struct {
		name string
		src  []byte
	}{
		{"array of 100000", ccoreWideJSON(100000)},
		{"object of 30000 pairs", ccoreObjectJSON(30000)},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := ccoreRouteParser(lang)
			p.SetMemoryBudgetBytes(1 << 40)
			gts.CcoreTestReset()
			old, err := p.Parse(c.src)
			if err != nil || old == nil || !gts.CcoreTestFromCore(old) {
				t.Fatalf("setup: %v", err)
			}
			defer old.Release()
			at := bytes.IndexByte(c.src[len(c.src)/2:], '1') + len(c.src)/2 // a digit in the middle
			next, e := ccoreReplace(c.src, at, c.src[at]+1)
			old.Edit(e)
			var at0, at1 runtime.MemStats
			stopLease := gts.CcoreTestParserLease(func(_ uintptr, before bool) {
				if before {
					runtime.ReadMemStats(&at0)
				}
			})
			defer stopLease()
			var charged int64
			// The fresh request charges its own parser buffers and public arena.
			stopAcc := gts.CcoreTestAccountProbe(func(b, a, _, _ int64) { runtime.ReadMemStats(&at1); charged = b + a })
			defer stopAcc()
			gts.CcoreTestDrainPool(lang)
			gts.DrainArenaPools()
			nt, err := p.ParseIncremental(next, old)
			if err != nil || nt == nil {
				t.Fatal(err)
			}
			defer nt.Release()
			if !gts.CcoreTestFromCore(nt) {
				t.Fatal("the reparse was not served by the core")
			}
			if charged == 0 {
				t.Fatal("the account hook never ran")
			}
			real := int64(at1.TotalAlloc - at0.TotalAlloc)
			t.Logf("reparse: charged %d, allocated %d (%.1f%%)", charged, real, 100*float64(charged)/float64(real))
			if float64(charged) < 0.90*float64(real) || float64(charged) > 1.05*float64(real)+16384 {
				t.Fatalf("charged %d bytes for %d bytes really allocated; want 0.90 x allocated <= charged <= 1.05 x allocated + 16384", charged, real)
			}
		})
	}
}

func init() {
	py := grammars.DetectLanguageByName("python").Language()
	inner := gts.CcoreTestInnermostScanner(py.ExternalScanner)
	payload := inner.Create()
	inner.Destroy(payload)
	scanmode.RegisterScanner(heapScanner{}, payload, true, func(v any) (any, []byte, bool) { return v, nil, true })
}

func TestCcoreDefaultServesNothing(t *testing.T) {
	if os.Getenv("GTS_TEST_CCORE_ROUTE") == "1" {
		t.Skip("the route-on run certifies every language")
	}
	lang := ccoreRouteLang(t, "json")
	gts.CcoreTestReset()
	tree, err := ccoreRouteParser(lang).Parse(ccoreJSONSource(20))
	if err != nil || tree == nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if decl, served := gts.CcoreTestCounters(); served != 0 || gts.CcoreTestFromCore(tree) || len(decl) != 0 {
		t.Fatalf("served %d, declines %v: the default build must not reach the core", served, decl)
	}
	if gts.CcoreTestCertifiedSize() != 0 || gts.CcoreTestAllowlistSize() != 0 {
		t.Fatal("a certificate list is not empty")
	}
}

func TestCcorePanicInConversionFallsBack(t *testing.T) {
	json := ccoreRouteLang(t, "json")
	src := ccoreJSONSource(3000)
	defer ccoreCertifyAll()()
	defer gts.CcoreTestPanicInConversion(2048)()
	gts.CcoreTestReset()
	tr, err := ccoreRouteParser(json).Parse(src)
	if err != nil || tr == nil {
		t.Fatalf("parse: %v", err)
	}
	defer tr.Release()
	decl, served := gts.CcoreTestCounters()
	if served != 0 || decl["panic"] != 1 || gts.CcoreTestFromCore(tr) {
		t.Fatalf("served %d, declines %v", served, decl)
	}
	if n := gts.CcoreTestPooledFootprint(json); n != 0 {
		t.Fatalf("the parser of a request that panicked went back to the pool (%d bytes)", n)
	}
}
