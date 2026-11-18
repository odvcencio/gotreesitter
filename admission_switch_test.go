package gotreesitter_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
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
		scannerParser := gts.NewParser(scannerLang)
		scannerParser.SetCompactCertificationTelemetry(true)
		incremental("ParseIncremental with a stateful scanner ("+name+")", scannerParser, sched.Incremental, scannerSource,
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

// The engine exit census separately measures all 206 locked samples. This
// quick CI check exercises a core serve, an early refusal, and Go's default.
func TestAdmissionCcoreDefaultPerformanceSanity(t *testing.T) {
	if testing.Short() {
		t.Skip("performance sanity gate skipped under -short")
	}
	const (
		maxRatio        = 1.15
		maxDeclineRatio = 1.10
		rounds          = 25
		batch           = 8
	)
	procs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(procs)
	compactDefault := gts.AdmissionCandidateRouteDefault()
	gts.SetAdmissionCandidateRouteDefault(false)
	defer gts.SetAdmissionCandidateRouteDefault(compactDefault)

	for _, tc := range []struct {
		name, language string
		observer       bool
	}{
		{"json", "json", false},
		{"json_observer", "json", true},
		{"python", "python", false},
		{"go", "go", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := ccoreRouteLang(t, tc.language)
			source, err := os.ReadFile(filepath.Join("internal", "benchfixtures", "testdata", "real", tc.language))
			if err != nil {
				t.Fatal(err)
			}
			blob, _ := lang.GrammarBlobSHA256()
			_, hasCertificate := certified.Blobs[blob]
			if tc.language == "json" && !hasCertificate {
				t.Fatal("JSON must supply a certified core route for the sanity check")
			}

			legacy := gts.NewParser(lang)
			legacy.SetAdmissionCandidateRoute(false)
			candidate := gts.NewParser(lang)
			legacy.SetCompactCertificationTelemetry(tc.observer)
			candidate.SetCompactCertificationTelemetry(tc.observer)
			decline := ""
			if tc.observer {
				decline = "observer"
			}
			checkPublicPerformanceRoute(t, legacy, source, false, "")
			checkPublicPerformanceRoute(t, candidate, source, hasCertificate && !tc.observer, decline)

			timed := func(p *gts.Parser) time.Duration {
				var elapsed time.Duration
				for i := 0; i < batch; i++ {
					start := time.Now()
					tree, err := p.Parse(source)
					elapsed += time.Since(start)
					if err != nil || tree == nil {
						t.Fatalf("timed parse: tree=%v err=%v", tree != nil, err)
					}
					tree.Release()
				}
				return elapsed
			}
			timed(legacy)
			timed(candidate)
			ratios := make([]float64, 0, rounds)
			for i := 0; i < rounds; i++ {
				var base, current time.Duration
				if i%2 == 0 {
					base, current = timed(legacy), timed(candidate)
				} else {
					current, base = timed(candidate), timed(legacy)
				}
				ratios = append(ratios, float64(current)/float64(base))
			}
			ratio := medianFloat64(ratios)
			t.Logf("public default/forced legacy median = %.3f over %d paired samples", ratio, rounds)
			limit := maxRatio
			if decline != "" {
				limit = maxDeclineRatio
			}
			if ratio > limit {
				t.Fatalf("public default/forced legacy median %.3f exceeds %.2f", ratio, limit)
			}
		})
	}
}

func checkPublicPerformanceRoute(t *testing.T, parser *gts.Parser, source []byte, core bool, decline string) {
	t.Helper()
	before, freshBefore, incrementalBefore := bind.RouteCounters()
	tree, err := parser.Parse(source)
	if err != nil || tree == nil {
		t.Fatalf("route witness: tree=%v err=%v", tree != nil, err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if tree.ParseStopReason() != gts.ParseStopAccepted || root == nil || root.HasError() || root.EndByte() != uint32(len(source)) {
		t.Fatalf("route witness must accept all %d bytes", len(source))
	}
	after, freshAfter, incrementalAfter := bind.RouteCounters()
	wantFresh := int64(0)
	if core {
		wantFresh = 1
	}
	if gts.CcoreTestFromCore(tree) != core || freshAfter-freshBefore != wantFresh || incrementalAfter != incrementalBefore {
		t.Fatalf("route witness: core=%v fresh=%d incremental=%d, want core=%v fresh=%d",
			gts.CcoreTestFromCore(tree), freshAfter-freshBefore, incrementalAfter-incrementalBefore, core, wantFresh)
	}
	for reason, count := range after {
		if reason != decline && count != before[reason] {
			t.Fatalf("unexpected core refusal %q", reason)
		}
	}
	if decline != "" && after[decline] != before[decline]+1 {
		t.Fatalf("route witness did not record one %q refusal", decline)
	}
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
	defer gts.CcoreTestViewsOn(false)()
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
	lang := ccoreCopyLanguage(ccoreRouteLang(t, "json"))
	gts.CcoreTestSetBlob(lang, [32]byte{1})
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
	run := func(arm string, mib int, views bool) map[string]string {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCcoreRouteBudgetChild$", "-test.count=1", "-test.v")
		var env []string
		for _, kv := range os.Environ() { // a control variable inherited from the caller must not reach the child
			if !strings.HasPrefix(kv, "CCORE_BUDGET_CHILD=") && !strings.HasPrefix(kv, "CCORE_BUDGET_MIB=") && !strings.HasPrefix(kv, "CCORE_BUDGET_VIEWS=") {
				env = append(env, kv)
			}
		}
		cmd.Env = append(env, "CCORE_BUDGET_CHILD="+arm, fmt.Sprintf("CCORE_BUDGET_MIB=%d", mib), fmt.Sprintf("CCORE_BUDGET_VIEWS=%t", views), "GTS_TEST_CCORE_ROUTE=0", "GOMAXPROCS=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %d MiB: %v\n%s", arm, mib, err, out)
		}
		res := map[string]string{}
		for _, l := range strings.Split(string(out), "\n") {
			for _, key := range []string{"CCBUDGET ", "CCTREES ", "CCCORE ", "CCSERVED "} {
				if strings.HasPrefix(l, key) {
					res[key] = strings.TrimPrefix(l, key)
				}
			}
		}
		if res["CCBUDGET "] == "" || res["CCTREES "] == "" || res["CCCORE "] == "" || res["CCSERVED "] == "" {
			t.Fatalf("%s %d MiB: incomplete result lines\n%s", arm, mib, out)
		}
		return res
	}
	compared, viewsServed := 0, 0
	for _, mib := range []int{3, 4, 8} {
		off := run("off", mib, false)
		for _, arm := range []string{"uncertified", "certified"} {
			// The uncertified arm hides the certificate; the certified arm here builds
			// converted trees, the regime these budgets were chosen for.
			got := run(arm, mib, false)
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
				// The generated list has a certificate, so the route is asked; it
				// declines each of the 6 requests because this blob has none.
				want = "served=0 stop_memory_budget=0 other=6"
			}
			if got["CCCORE "] != want {
				t.Errorf("%d MiB, arm %s: core counters %q, want %q", mib, arm, got["CCCORE "], want)
			}
			compared++
		}
		if !strings.Contains(off["CCBUDGET "], "memory_budget") {
			t.Errorf("%d MiB: the present engine never stopped for memory (%s); the case proves nothing", mib, off["CCBUDGET "])
		}
		// The certified arm with lazy views, which is what a certificate ships. A view
		// tree needs less memory than a converted one, so under the same budget the core
		// may serve a request that the present engine stops on. The rule per request:
		// one the core declines is answered by the present engine with one of the answers
		// the present engine gives in the route-off run (a stop for memory or the accepted
		// tree); one the core serves is the accepted tree. When the core serves nothing,
		// the whole run equals the route-off run, as above.
		views := run("certified", mib, true)
		var served, stopped, other int
		fmt.Sscanf(views["CCCORE "], "served=%d stop_memory_budget=%d other=%d", &served, &stopped, &other)
		if served+stopped != 6 || other != 0 {
			t.Errorf("%d MiB, views: core counters %q; want every one of the 6 requests served or declined for memory", mib, views["CCCORE "])
		}
		offReasons, offTrees := strings.Split(off["CCBUDGET "], ","), strings.Split(off["CCTREES "], ",")
		accepted := ""
		for i, r := range offReasons {
			if r == string(gts.ParseStopAccepted) {
				accepted = offTrees[i]
			}
		}
		allowed := map[string]bool{}
		for i := range offReasons {
			allowed[offReasons[i]+" "+offTrees[i]] = true
		}
		reasons, trees, mask := strings.Split(views["CCBUDGET "], ","), strings.Split(views["CCTREES "], ","), views["CCSERVED "]
		if len(reasons) != 6 || len(trees) != 6 || len(mask) != 6 {
			t.Fatalf("%d MiB, views: incomplete result: %v %v %q", mib, reasons, trees, mask)
		}
		if served == 0 && (views["CCBUDGET "] != off["CCBUDGET "] || views["CCTREES "] != off["CCTREES "]) {
			t.Errorf("%d MiB, views: the core served nothing, yet the run differs from the route-off run:\n got %s %s\nwant %s %s", mib, views["CCBUDGET "], views["CCTREES "], off["CCBUDGET "], off["CCTREES "])
		}
		for i := range reasons {
			if mask[i] == '1' {
				viewsServed++
				if reasons[i] != string(gts.ParseStopAccepted) || accepted == "" || trees[i] != accepted {
					t.Errorf("%d MiB, views, request %d: the core served %s %s; want the accepted tree %s", mib, i, reasons[i], trees[i], accepted)
				}
			} else if !allowed[reasons[i]+" "+trees[i]] {
				t.Errorf("%d MiB, views, request %d: after the core declined the answer is %s %s, which the present engine never gives in the route-off run (%s %s)", mib, i, reasons[i], trees[i], off["CCBUDGET "], off["CCTREES "])
			}
		}
		compared++
	}
	if compared == 0 {
		t.Fatal("nothing was compared")
	}
	t.Logf("with views the core served %d of 18 requests under these budgets", viewsServed)
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
	defer gts.CcoreTestViewsOn(os.Getenv("CCORE_BUDGET_VIEWS") == "true")()
	lang := ccoreRouteLang(t, "json")
	if arm == "uncertified" {
		// json has a certificate in the generated list. This arm is the candidate
		// switch with nothing certified, so it parses with a copy of the language
		// whose blob identity is not in that list.
		lang = ccoreCopyLanguage(lang)
		gts.CcoreTestSetBlob(lang, [32]byte{1})
	}
	src := ccoreJSONSource(3000)
	var out, trees []string
	servedMask := ""
	gts.CcoreTestReset()
	for i := 0; i < 6; i++ {
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(arm != "off")
		p.SetMemoryBudgetBytes(int64(mib) << 20)
		tr, err := p.Parse(src)
		if err != nil || tr == nil {
			out = append(out, "err")
			trees = append(trees, "err")
			servedMask += "0"
			continue
		}
		if gts.CcoreTestFromCore(tr) {
			servedMask += "1"
		} else {
			servedMask += "0"
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
	fmt.Printf("CCBUDGET %s\nCCTREES %s\nCCCORE served=%d stop_memory_budget=%d other=%d\nCCSERVED %s\n", strings.Join(out, ","), strings.Join(trees, ","), served, decl["stop_memory_budget"], other, servedMask)
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

// ccoreSkipUnderRace skips a test in a build with the race detector. It reads
// the race build tag: the build information of a test binary does not carry
// the -race setting.
func ccoreSkipUnderRace(t *testing.T, why string) {
	t.Helper()
	if raceEnabled {
		t.Skip(why)
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
	ccoreDroppedTreeCopyIsCollectable(t)
}

func TestCcoreViewsDroppedTreeCopyIsCollectable(t *testing.T) {
	defer gts.CcoreTestViewsOn(true)()
	ccoreDroppedTreeCopyIsCollectable(t)
}

func ccoreDroppedTreeCopyIsCollectable(t *testing.T) {
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

func TestCcoreJSONEndOfInputFlagCannotBeToldApart(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	sha, ok := lang.GrammarBlobSHA256()
	if !ok || fmt.Sprintf("%x", sha) != "ad656555b6909bc8efbf40eceb2ac98121f7abdf707c829c9dc03ebe6d83bfbf" {
		t.Fatalf("unexpected JSON blob: %x, present=%t", sha, ok)
	}
	flagged := ccoreCopyLanguage(lang)
	flagged.LexStates = append([]gts.LexState(nil), lang.LexStates...)
	seen := make(map[int]bool)
	for _, state := range lang.LexStates {
		if state.EOF >= 0 && !seen[state.EOF] {
			seen[state.EOF] = true
			target := &flagged.LexStates[state.EOF]
			if target.AcceptToken == 0 && !target.Skip && !target.AcceptEOF {
				target.AcceptEOF = true
			}
		}
	}
	if !flagged.LexStates[21].AcceptEOF || lang.LexStates[21].AcceptEOF {
		t.Fatal("JSON end-of-input state changed")
	}
	defer ccoreCertifyAll()()
	inputs := []string{"", " ", "\n\n", "1", "-", "1.", "1e", "true", "tru", "nul", "\"", "\"abc", "\"a\\", "[", "[1", "[1,", "[1,]", "{", "{\"a\"", "{\"a\":", "{\"a\":1", "{\"a\":1,", "/", "/*", "// c", "[1] ", "[1]x", "\x00", "[\"\xff\"]", "[[[[[[[[1]]]]]]]]", string(ccoreJSONSource(30))}
	for _, input := range inputs {
		parse := func(l *gts.Language) string {
			gts.CcoreTestReset()
			tree, err := ccoreRouteParser(l).Parse([]byte(input))
			if err != nil || tree == nil {
				t.Fatalf("%q: %v", input, err)
			}
			defer tree.Release()
			if _, served := gts.CcoreTestCounters(); served != 1 {
				t.Fatalf("%q: core did not serve the parse", input)
			}
			return ccoreWalkDigest(tree, l)
		}
		if got, want := parse(lang), parse(flagged); got != want {
			t.Fatalf("%q: end-of-input flag changed the tree: %s != %s", input, got, want)
		}
	}
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
	defer gts.CcoreTestViewsOn(false)()
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
			// Exclude the old store, which was allocated before this request.
			oldPages := int64(gts.CcoreTestStoreBytes(old))
			stopAcc := gts.CcoreTestAccountProbe(func(b, a, _, _ int64) { runtime.ReadMemStats(&at1); charged = b + a - oldPages })
			defer stopAcc()
			gts.CcoreTestDrainPool(lang)
			gts.DrainArenaPools()
			nt, err := p.ParseIncremental(next, old)
			if err != nil || nt == nil {
				t.Fatal(err)
			}
			defer nt.Release()
			if _, served := ccoreIncrCount(t); served != 1 {
				t.Fatalf("the reparse was not served incrementally (%d)", served)
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
	scanmode.RegisterScanner(gradPanicScanner{}, (*gradPanicPayload)(nil), true, func(v any) (any, []byte, bool) { return v, nil, true })
}

func TestCcoreDefaultServesJSON(t *testing.T) {
	if os.Getenv("GTS_TEST_CCORE_ROUTE") == "1" {
		t.Skip("the route-on run certifies every language")
	}
	lang := ccoreRouteLang(t, "json")
	p := gts.NewParser(lang)
	for i, src := range [][]byte{[]byte(`[123,456]`), []byte(`[123,]`)} {
		tree, err := p.Parse(src)
		if err != nil || tree == nil || !gts.CcoreTestFromCore(tree) {
			t.Fatalf("default parse %q was not served: %v", src, err)
		}
		if tree.RootNode().HasError() != (i == 1) {
			t.Fatalf("unexpected error state for %q", src)
		}
		tree.Release()
	}
	src := []byte(`[123,456]`)
	tree, err := p.Parse(src)
	if err != nil || tree == nil {
		t.Fatal(err)
	}
	defer tree.Release()
	_, _, counts := gts.CcoreTestViewCounters()
	before := counts["shortcut_reparses"]
	nextSrc, edit := ccoreReplace(src, 2, '4')
	tree.Edit(edit)
	next, err := p.ParseIncremental(nextSrc, tree)
	if err != nil || next == nil || !gts.CcoreTestFromCore(next) {
		t.Fatalf("reparse: %v", err)
	}
	defer next.Release()
	_, incremental := gts.CcoreTestIncr()
	_, _, counts = gts.CcoreTestViewCounters()
	if incremental == 0 || counts["shortcut_reparses"] != before+1 {
		t.Fatal("reparse did not use the incremental shortcut")
	}
	p.SetAdmissionCandidateRoute(false)
	legacy, err := p.Parse(src)
	if err != nil || legacy == nil || gts.CcoreTestFromCore(legacy) {
		t.Fatalf("opt-out parse: %v", err)
	}
	legacy.Release()
	uncertified, err := gts.NewParser(ccoreUncertifiedJSONLanguage(t)).Parse(src)
	if err != nil || uncertified == nil || gts.CcoreTestFromCore(uncertified) {
		t.Fatalf("parse of an uncertified blob: %v", err)
	}
	uncertified.Release()
}

func ccoreUncertifiedJSONLanguage(t *testing.T) *gts.Language {
	t.Helper()
	lang := ccoreCopyLanguage(ccoreRouteLang(t, "json"))
	gts.CcoreTestSetBlob(lang, [32]byte{})
	return lang
}

// A reparse without edits returns the old tree for a grammar without a
// certificate, on the first call (which records the answer on the language) and
// on every later one (which reads it), and a parser that opted out of the core
// does not hand back a tree the core built.
//
// The body runs in a child process: there no test certification was ever
// installed, as in production, so the short first test of the reparse answers.
func TestCcoreUnchangedReparseWithACertificateInTheBuild(t *testing.T) {
	if os.Getenv("GTS_TEST_CCORE_ROUTE") == "1" {
		t.Skip("the route-on run certifies every language")
	}
	if os.Getenv("GTS_TEST_CCORE_UNCHANGED_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCcoreUnchangedReparseWithACertificateInTheBuild$", "-test.count=1")
		cmd.Env = append(os.Environ(), "GTS_TEST_CCORE_UNCHANGED_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child: %v\n%s", err, out)
		}
		return
	}
	if gts.CcoreTestEverTest() {
		t.Fatal("setup: a test certification was installed in the child process")
	}
	uncertified := gts.NewParser(ccoreUncertifiedJSONLanguage(t))
	src := []byte(`[123,456]`)
	tree, err := uncertified.Parse(src)
	if err != nil || tree == nil || gts.CcoreTestFromCore(tree) {
		t.Fatalf("parse of an uncertified blob: %v", err)
	}
	defer tree.Release()
	for i := 0; i < 3; i++ {
		again, err := uncertified.ParseIncremental(src, tree)
		if err != nil || again != tree {
			t.Fatalf("reparse %d without edits of an uncertified grammar did not return the old tree: %v", i, err)
		}
	}

	js := gts.NewParser(ccoreRouteLang(t, "json"))
	jsrc := []byte(`[123,456]`)
	core, err := js.Parse(jsrc)
	if err != nil || core == nil || !gts.CcoreTestFromCore(core) {
		t.Fatalf("json parse was not served by the core: %v", err)
	}
	defer core.Release()
	same, err := js.ParseIncremental(jsrc, core)
	if err != nil || same != core {
		t.Fatalf("a reparse without edits by the core's own parser did not return the old tree: %v", err)
	}
	js.SetAdmissionCandidateRoute(false)
	other, err := js.ParseIncremental(jsrc, core)
	if err != nil || other == nil {
		t.Fatalf("reparse after opting out: %v", err)
	}
	if other == core || gts.CcoreTestFromCore(other) {
		t.Fatal("a parser that opted out of the core handed back a tree the core built")
	}
	other.Release()
}

func TestCcoreDefaultIgnoresCandidateDefaultOff(t *testing.T) {
	if os.Getenv("GTS_TEST_CCORE_ENV_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCcoreDefaultIgnoresCandidateDefaultOff$", "-test.count=1")
		cmd.Env = append(os.Environ(), "GTS_ADMISSION_CANDIDATE=0", "GTS_TEST_CCORE_ENV_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child: %v\n%s", err, out)
		}
		return
	}
	if gts.AdmissionCandidateRouteDefault() {
		t.Fatal("environment did not disable the candidate default")
	}
	for _, setDefault := range []bool{false, true} {
		if setDefault {
			gts.SetAdmissionCandidateRouteDefault(false)
		}
		tree, err := gts.NewParser(ccoreRouteLang(t, "json")).Parse([]byte(`{"x":1}`))
		if err != nil || tree == nil || !gts.CcoreTestFromCore(tree) {
			t.Fatalf("default-off parse: %v", err)
		}
		tree.Release()
	}
}

func TestCcorePanicInConversionFallsBack(t *testing.T) {
	defer gts.CcoreTestViewsOn(false)()
	json := ccoreRouteLang(t, "json")
	src := ccoreJSONSource(3000)
	defer ccoreCertifyAll()()
	// Establish an empty pool so the assertion covers only this request.
	gts.CcoreTestDrainPool(json)
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

func TestCcoreReparseDeclineAttemptsOnce(t *testing.T) {
	defer gts.CcoreTestViewsOn(false)()
	defer ccoreCertifyAll()()
	lang := ccoreRouteLang(t, "json")
	for _, profiled := range []bool{false, true} {
		t.Run(fmt.Sprintf("profiled=%t", profiled), func(t *testing.T) {
			p := ccoreRouteParser(lang)
			src := ccoreJSONSource(400)
			old, err := p.Parse(src)
			if err != nil || old == nil {
				t.Fatalf("old parse: %v", err)
			}
			defer old.Release()
			end := uint32(len(src))
			old.Edit(gts.InputEdit{StartByte: end, OldEndByte: end, NewEndByte: end + 1})
			changed := append(append([]byte(nil), src...), ' ')
			attempts := 0
			defer gts.CcoreTestParserLease(func(_ uintptr, before bool) {
				if before {
					attempts++
				}
			})()
			restorePanic := gts.CcoreTestPanicInConversion(1024)
			defer restorePanic()
			gts.CcoreTestReset()
			var tree *gts.Tree
			if profiled {
				tree, _, err = p.ParseIncrementalProfiled(changed, old)
			} else {
				tree, err = p.ParseIncremental(changed, old)
			}
			if err != nil || tree == nil {
				t.Fatalf("reparse: %v", err)
			}
			defer tree.Release()
			decl, _ := gts.CcoreTestCounters()
			if attempts != 1 || decl["panic"] != 1 {
				t.Fatalf("attempts=%d declines=%v, want one attempt and one panic decline", attempts, decl)
			}
			restorePanic()
			again, err := p.Parse(changed)
			if err != nil || again == nil || !gts.CcoreTestFromCore(again) {
				t.Fatalf("later parse did not use core: %v", err)
			}
			again.Release()
		})
	}
}

func ccoreIncrCount(t *testing.T) (map[string]int64, int64) {
	t.Helper()
	return gts.CcoreTestIncr()
}

func TestCcoreIncrProfile(t *testing.T) {
	defer ccoreCertifyIncr()()
	lang := ccoreRouteLang(t, "json")
	for _, useOptions := range []bool{false, true} {
		for _, bad := range []bool{false, true} {
			t.Run(fmt.Sprintf("options=%t/error=%t", useOptions, bad), func(t *testing.T) {
				p := ccoreRouteParser(lang)
				src := ccoreJSONSource(300)
				if bad {
					src, _ = ccoreInsert(src, 20, '}')
				}
				old, err := p.Parse(src)
				if err != nil || old == nil || old.RootNode().HasError() != bad {
					t.Fatalf("old tree: %v", err)
				}
				defer old.Release()
				next, edit := ccoreInsert(src, 25, ' ')
				old.Edit(edit)
				var tree *gts.Tree
				var profile gts.IncrementalParseProfile
				if useOptions {
					result, err := p.ParseWith(next, gts.WithOldTree(old), gts.WithProfiling())
					if err != nil || !result.ProfileAvailable {
						t.Fatalf("ParseWith: %v", err)
					}
					tree, profile = result.Tree, result.Profile
				} else {
					tree, profile, err = p.ParseIncrementalProfiled(next, old)
					if err != nil {
						t.Fatal(err)
					}
				}
				if tree == nil {
					t.Fatal("nil tree")
				}
				defer tree.Release()
				nodes, bytes, incremental := gts.CcoreTestReuse(tree)
				if bad {
					if incremental || !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "ccore_fresh" || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
						t.Fatalf("fresh core profile: %+v", profile)
					}
				} else if !incremental || nodes == 0 || bytes == 0 || profile.ReusedSubtrees != uint64(nodes) || profile.ReusedBytes != bytes || profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute {
					t.Fatalf("incremental core reuse=(%d, %d, %t) profile=%+v", nodes, bytes, incremental, profile)
				}
			})
		}
	}
}

func TestCcoreIncrSessionEqualsFresh(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	defer ccoreCertifyIncr()()
	src := ccoreJSONSource(300)
	p, fp := ccoreRouteParser(lang), ccoreRouteParser(lang)
	gts.CcoreTestReset()
	tree, err := p.Parse(src)
	if err != nil || tree == nil {
		t.Fatal(err)
	}
	steps, reused := 0, uint64(0)
	for i := 0; i < 60; i++ {
		var next []byte
		var e gts.InputEdit
		at := 5 + (i*131)%(len(src)-10)
		for at < len(src)-3 && !(src[at] >= '0' && src[at] <= '8') { // a digit: every edit below keeps the text valid json
			at++
		}
		switch i % 3 {
		case 0: // same-length replacement
			next, e = ccoreReplace(src, at, src[at]+1)
		case 1: // a typed character
			next, e = ccoreInsert(src, at, '1')
		default: // a deleted byte (the digit before a digit, or a typed one)
			if src[at+1] >= '0' && src[at+1] <= '9' {
				next, e = ccoreDeleteByte(src, at)
			} else {
				next, e = ccoreInsert(src, at, '2')
			}
		}
		tree.Edit(e)
		nt, err := p.ParseIncremental(next, tree)
		if err != nil || nt == nil {
			t.Fatal(err)
		}
		fresh, err := fp.Parse(next)
		if err != nil {
			t.Fatal(err)
		}
		if ccoreWalkDigest(nt, lang) != ccoreWalkDigest(fresh, lang) {
			t.Fatalf("step %d: the incremental tree differs from a fresh parse", i)
		}
		if has, edits, broken := gts.CcoreTestLink(nt); !has || edits != 0 || broken {
			t.Fatalf("step %d: new tree link has=%v edits=%d broken=%v", i, has, edits, broken)
		}
		if _, b, incr := gts.CcoreTestReuse(nt); incr {
			reused += b
		}
		fresh.Release()
		tree.Release()
		tree, src = nt, next
		steps++
	}
	tree.Release()
	fb, served := ccoreIncrCount(t)
	if served != int64(steps) || len(fb) != 0 || reused == 0 {
		t.Fatalf("steps %d: served incrementally %d, fallbacks %v, reused bytes %d", steps, served, fb, reused)
	}
}

// Each case in which a reparse is a fresh core parse, with its counter.
func TestCcoreIncrFallbackCases(t *testing.T) {
	defer gts.CcoreTestViewsOn(false)()
	lang := ccoreCopyLanguage(ccoreRouteLang(t, "json"))
	gts.CcoreTestSetBlob(lang, [32]byte{1})
	base := ccoreJSONSource(40)
	type tc struct {
		name  string
		modes uint8
		want  string // fallback counter that must be 1 ("" : served incrementally)
		setup func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree)
	}
	cases := []tc{
		{"clean", ccoreModesIncr, "", func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree) {
			out, e := ccoreInsert(src, 20, ' ')
			tree.Edit(e)
			return out, tree
		}},
		{"incr_not_certified", ccoreAllModes, "incr_not_certified", func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree) {
			out, e := ccoreInsert(src, 20, ' ')
			tree.Edit(e)
			return out, tree
		}},
		{"edit_over_budget", ccoreModesIncr, "edits_not_carried", func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree) {
			// A budget one byte above the pages the tree holds: the first page the edit
			// claims is refused, the store of the core tree is poisoned, the edit does not
			// reach it, and the reparse is a fresh parse by the core in a new store (counted
			// as edits_not_carried, no route decline) that equals the present engine's.
			pages := gts.CcoreTestEditBudget(tree, 1)
			gts.CcoreTestEditBudget(tree, int64(pages)+1)
			out, e := ccoreInsert(src, 20, ' ')
			tree.Edit(e)
			if _, _, broken := gts.CcoreTestLink(tree); !broken {
				t.Fatal("an edit whose pages the budget refuses did not mark the link as broken")
			}
			return out, tree
		}},
		{"old_tree_foreign", ccoreModesIncr, "old_tree_foreign", func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree) {
			gts.CcoreTestStripCore(tree) // a tree whose core tree is gone: as one from the present engine
			out, e := ccoreInsert(src, 20, ' ')
			tree.Edit(e)
			return out, tree
		}},
		{"edits_not_carried", ccoreModesIncr, "edits_not_carried", func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree) {
			out, e := ccoreInsert(src, 20, ' ')
			tree.Edit(e)
			gts.CcoreTestBreakLink(tree) // the edit did not reach the core tree
			return out, tree
		}},
		{"edit_mismatch", ccoreModesIncr, "edit_mismatch", func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree) {
			out, e := ccoreInsert(src, 20, ' ')
			out[300] = ' ' // a change the edit does not describe
			tree.Edit(e)
			return out, tree
		}},
		{"edit_mismatch_in_the_gap", ccoreModesIncr, "edit_mismatch", func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree) {
			// Two recorded replacements that keep the length, and a third change between them
			// that no edit describes. Comparing only the text before the first edit and after
			// the last one accepts this; the unchanged number between them would be reused.
			var at []int
			for i := 30; i < len(src)-8 && len(at) < 3; i++ {
				if src[i] >= '0' && src[i] <= '8' && (len(at) == 0 || i > at[len(at)-1]+40) {
					at = append(at, i)
				}
			}
			if len(at) < 3 {
				t.Fatal("setup: no three digits")
			}
			out := append([]byte(nil), src...)
			out[at[1]]++ // the change nobody recorded
			out[at[0]]++
			out[at[2]]++
			for _, i := range []int{at[0], at[2]} {
				tree.Edit(gts.InputEdit{StartByte: uint32(i), OldEndByte: uint32(i + 1), NewEndByte: uint32(i + 1),
					StartPoint: ccorePoint(src, i), OldEndPoint: ccorePoint(src, i+1), NewEndPoint: ccorePoint(out, i+1)})
			}
			return out, tree
		}},
		{"old_has_error", ccoreModesIncr, "old_has_error", func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree) {
			// make a tree with an error first, then edit that one
			bad, e0 := ccoreInsert(src, 20, '}')
			tree.Edit(e0)
			bt, err := p.ParseIncremental(bad, tree)
			if err != nil || !bt.RootNode().HasError() {
				t.Fatalf("setup: %v", err)
			}
			tree.Release()
			gts.CcoreTestReset()
			out, e := ccoreInsert(bad, 25, ' ')
			bt.Edit(e)
			return out, bt
		}},
		{"result_has_error", ccoreModesIncr, "result_has_error", func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree) {
			out, e := ccoreInsert(src, 20, '}')
			tree.Edit(e)
			return out, tree
		}},
		{"old_tree_released", ccoreModesIncr, "old_tree_released", func(t *testing.T, p *gts.Parser, tree *gts.Tree, src []byte) ([]byte, *gts.Tree) {
			out, e := ccoreInsert(src, 20, ' ')
			tree.Edit(e)
			gts.CcoreTestCoreTree(tree).Release() // the core tree's store entry is already released
			return out, tree
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer gts.CcoreTestCertify(c.modes, "ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState")()
			p, fp := ccoreRouteParser(lang), ccoreRouteParser(lang)
			tree, err := p.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			gts.CcoreTestReset()
			next, tree := c.setup(t, p, tree, base)
			nt, err := p.ParseIncremental(next, tree)
			if err != nil || nt == nil {
				t.Fatal(err)
			}
			fresh, _ := fp.Parse(next)
			if ccoreWalkDigest(nt, lang) != ccoreWalkDigest(fresh, lang) {
				t.Fatal("the result differs from a fresh parse")
			}
			fb, served := ccoreIncrCount(t)
			decl, _ := gts.CcoreTestCounters()
			if len(decl) != 0 {
				t.Fatalf("declines %v", decl)
			}
			if c.want == "" {
				if served != 1 || len(fb) != 0 {
					t.Fatalf("served %d fallbacks %v; want one incremental parse", served, fb)
				}
			} else if served != 0 || fb[c.want] != 1 || len(fb) != 1 {
				t.Fatalf("served %d fallbacks %v; want only %s", served, fb, c.want)
			}
			nt.Release()
			if tree != nt {
				tree.Release()
			}
			fresh.Release()
		})
	}
}

// ccoreEditsDescribe accepts the edit lists that turn the old text into the new one and refuses the rest.
func TestCcoreEditsDescribe(t *testing.T) {
	old := []byte("0123456789")
	ed := func(s, oe, ne uint32) gts.InputEdit {
		return gts.InputEdit{StartByte: s, OldEndByte: oe, NewEndByte: ne}
	}
	cases := []struct {
		name  string
		nw    string
		edits []gts.InputEdit
		ok    bool
	}{
		{"replace", "01234X6789", []gts.InputEdit{ed(5, 6, 6)}, true},
		{"insert", "01234XX56789", []gts.InputEdit{ed(5, 5, 7)}, true},
		{"two", "0Y2345X789", []gts.InputEdit{ed(1, 2, 2), ed(6, 7, 7)}, true},
		{"insert then delete after", "01X2345678", []gts.InputEdit{ed(2, 2, 3), ed(10, 11, 10)}, true},
		{"two with the gap unchanged", "0Y2345X789", []gts.InputEdit{ed(1, 2, 2), ed(6, 7, 7)}, true},
		{"two, a change in the gap between them", "0Y2Z45X789", []gts.InputEdit{ed(1, 2, 2), ed(6, 7, 7)}, false},
		{"two, a change in the gap, same lengths, wide", "0Y23Z5X789", []gts.InputEdit{ed(1, 2, 2), ed(6, 7, 7)}, false},
		{"second edit over the first (overlap)", "0AB3456789", []gts.InputEdit{ed(1, 3, 3), ed(1, 3, 3)}, false},
		{"second edit overlaps the first", "0AB3X56789", []gts.InputEdit{ed(1, 3, 3), ed(2, 5, 5)}, false},
		{"second edit overlaps the first, change outside both", "0AB3456780", []gts.InputEdit{ed(1, 3, 3), ed(2, 5, 5)}, false},
		{"insert inside the text the first edit wrote (overlap)", "0A1B3456789", []gts.InputEdit{ed(1, 3, 3), ed(2, 2, 3)}, false},
		{"touching edits: the second starts where the first ends", "0ABX456789", []gts.InputEdit{ed(1, 3, 3), ed(3, 4, 4)}, true},
		{"edits one byte apart", "0AB3X56789", []gts.InputEdit{ed(1, 3, 3), ed(4, 5, 5)}, true},
		{"touching edits: the second ends where the first starts", "0Y2B456789", []gts.InputEdit{ed(2, 4, 4), ed(1, 2, 2)}, true},
		{"edits one byte apart, the byte between changed", "0AB9X56789", []gts.InputEdit{ed(1, 3, 3), ed(4, 5, 5)}, false},
		{"delete then insert at the same place", "0123X56789", []gts.InputEdit{ed(4, 5, 4), ed(4, 4, 5)}, true},
		{"undescribed change", "01234X6789", []gts.InputEdit{ed(1, 2, 2)}, false},
		{"wrong length", "01234X678", []gts.InputEdit{ed(5, 6, 6)}, false},
		{"no edit, other text", "0123456780", nil, false},
		{"end before start", "0123456789", []gts.InputEdit{ed(5, 4, 4)}, false},
		{"past the end", "0123456789X", []gts.InputEdit{ed(11, 11, 12)}, false},
	}
	for _, c := range cases {
		if got := gts.CcoreTestEditsDescribe(old, []byte(c.nw), c.edits); got != c.ok {
			t.Errorf("%s: got %v, want %v", c.name, got, c.ok)
		}
	}
	for _, n := range []int{64, 65} {
		edits := make([]gts.InputEdit, n)
		if got := gts.CcoreTestEditsDescribe(old, old, edits); got != (n == 64) {
			t.Errorf("%d edits: got %v", n, got)
		}
	}
	// Two recorded replacements that keep the length; the number between them also became a
	// string, and no edit says so.
	oldJSON, newJSON := []byte("[1,0,123,0,3]"), []byte(`[4,0,"x",0,6]`)
	if gts.CcoreTestEditsDescribe(oldJSON, newJSON, []gts.InputEdit{ed(1, 2, 2), ed(11, 12, 12)}) {
		t.Error("a change between two recorded edits was described by them")
	}
	if !gts.CcoreTestEditsDescribe(oldJSON, newJSON, []gts.InputEdit{ed(1, 2, 2), ed(5, 8, 5+3), ed(11, 12, 12)}) {
		t.Error("three edits that cover every change were not accepted")
	}
}

// Copy, edit, reparse, then release the original, the copy and the new tree in
// every order: each tree has its own core tree, the others stay readable while
// one is released, and every core tree is released with its public tree.
func TestCcoreIncrLifecycleOrders(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	defer ccoreCertifyIncr()()
	src := ccoreJSONSource(120)
	at := 41
	for !(src[at] >= '0' && src[at] <= '8') {
		at++
	}
	next, e := ccoreReplace(src, at, src[at]+1)
	fp := ccoreRouteParser(lang)
	f, err := fp.Parse(next)
	if err != nil {
		t.Fatal(err)
	}
	freshDigest := ccoreWalkDigest(f, lang)
	f.Release()
	perms := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, perm := range perms {
		p := ccoreRouteParser(lang)
		a, err := p.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		b := a.Copy()
		a.Edit(e)
		a2, err := p.ParseIncremental(next, a)
		if err != nil || a2 == nil {
			t.Fatal(err)
		}
		trees := [3]*gts.Tree{a, b, a2}
		cts := [3]*ccore.Tree{gts.CcoreTestCoreTree(a), gts.CcoreTestCoreTree(b), gts.CcoreTestCoreTree(a2)}
		if cts[0] == nil || cts[1] == nil || cts[2] == nil || cts[0] == cts[1] || cts[0] == cts[2] || cts[1] == cts[2] {
			t.Fatalf("%v: the trees do not each have a core tree of their own", perm)
		}
		if ccoreWalkDigest(a2, lang) != freshDigest {
			t.Fatalf("%v: the reparse differs from a fresh parse", perm)
		}
		origDigest := ccoreWalkDigest(b, lang)
		for step, i := range perm {
			trees[i].Release()
			if !cts[i].Released() {
				t.Fatalf("%v: releasing tree %d left its core tree alive", perm, i)
			}
			for _, j := range perm[step+1:] {
				if cts[j].Released() || gts.CcoreTestCoreWalk(cts[j]) == 0 {
					t.Fatalf("%v: releasing tree %d damaged tree %d", perm, i, j)
				}
			}
			if !cts[1].Released() && ccoreWalkDigest(b, lang) != origDigest {
				t.Fatalf("%v: the copy changed", perm)
			}
		}
	}
	// The new tree goes on after the old tree and the copy are released.
	p := ccoreRouteParser(lang)
	gts.CcoreTestReset()
	a, _ := p.Parse(src)
	a.Edit(e)
	a2, _ := p.ParseIncremental(next, a)
	a.Release()
	at2 := 60
	for !(next[at2] >= '0' && next[at2] <= '8') {
		at2++
	}
	next2, e2 := ccoreReplace(next, at2, next[at2]+1)
	a2.Edit(e2)
	a3, err := p.ParseIncremental(next2, a2)
	if err != nil {
		t.Fatal(err)
	}
	if _, served := gts.CcoreTestIncr(); served != 2 {
		t.Fatalf("served %d incremental parses, want 2", served)
	}
	f, _ = fp.Parse(next2)
	if ccoreWalkDigest(a3, lang) != ccoreWalkDigest(f, lang) {
		t.Fatal("second reparse differs from fresh")
	}
	f.Release()
	a2.Release()
	a3.Release()
}

// A public tree that is dropped without Release does not keep its core tree
// reachable, and a pooled core parser does not keep the tree of its last parse.
func TestCcoreIncrDroppedTreeIsCollectable(t *testing.T) {
	ccoreIncrDroppedTreeIsCollectable(t)
}

func TestCcoreViewsIncrDroppedTreeIsCollectable(t *testing.T) {
	defer gts.CcoreTestViewsOn(true)()
	ccoreIncrDroppedTreeIsCollectable(t)
}

func ccoreIncrDroppedTreeIsCollectable(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	defer ccoreCertifyIncr()()
	src := ccoreJSONSource(120)
	var freed atomic.Int32
	{ // a released tree first: its arena goes to the pool, and the next tree takes it from there
		pre, err := ccoreRouteParser(lang).Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		pre.Release()
	}
	func() {
		p := ccoreRouteParser(lang)
		tree, err := p.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		runtime.SetFinalizer(gts.CcoreTestCoreTree(tree), func(*ccore.Tree) { freed.Add(1) })
		at := 41
		for !(src[at] >= '0' && src[at] <= '8') {
			at++
		}
		next, e := ccoreReplace(src, at, src[at]+1)
		tree.Edit(e)
		nt, err := p.ParseIncremental(next, tree) // neither tree is released
		if err != nil {
			t.Fatal(err)
		}
		runtime.SetFinalizer(gts.CcoreTestCoreTree(nt), func(*ccore.Tree) { freed.Add(1) })
	}()
	// Nothing else is parsed or released from here on: the dropped trees must be collectable
	// by themselves, without being retained by the arena pool.
	for i := 0; i < 300 && freed.Load() < 2; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	if freed.Load() != 2 {
		t.Fatalf("%d of 2 core trees were collected after the public trees were dropped", freed.Load())
	}
}

// The store of a long edit chain does not grow: after 1,000 edits through the
// public API (each old tree released) the pages are at most 1.5 times the first tree's.
func TestCcoreIncrStoreBoundAfter1000Edits(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	defer ccoreCertifyIncr()()
	src := ccoreJSONSource(300)
	p := ccoreRouteParser(lang)
	tree, err := p.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	first := gts.CcoreTestStoreBytes(tree)
	maxSeen := first
	for i := 0; i < 1000; i++ {
		at := 5 + (i*977)%(len(src)-10)
		for !(src[at] >= '0' && src[at] <= '8') {
			at++
		}
		next, e := ccoreReplace(src, at, src[at]+1)
		tree.Edit(e)
		nt, err := p.ParseIncremental(next, tree)
		if err != nil {
			t.Fatal(err)
		}
		tree.Release()
		tree, src = nt, next
		if b := gts.CcoreTestStoreBytes(tree); b > maxSeen {
			maxSeen = b
		}
	}
	defer tree.Release()
	if _, served := gts.CcoreTestIncr(); served < 1000 {
		t.Fatalf("served %d of 1000", served)
	}
	if float64(maxSeen) > 1.5*float64(first) {
		t.Fatalf("store pages grew from %d to %d bytes (more than 1.5 times) over 1,000 edits", first, maxSeen)
	}
	t.Logf("store pages: first tree %d bytes, largest %d, last %d", first, maxSeen, gts.CcoreTestStoreBytes(tree))
}

// Readers of a copy on other goroutines while the original is edited and reparsed
// (run under -race).
func TestCcoreIncrConcurrentReaders(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	defer ccoreCertifyIncr()()
	src := ccoreJSONSource(200)
	p := ccoreRouteParser(lang)
	tree, err := p.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	reader := func(cp *gts.Tree) {
		defer wg.Done()
		ct := gts.CcoreTestCoreTree(cp)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if gts.CcoreTestCoreWalk(ct) == 0 {
				t.Error("empty walk")
				return
			}
		}
	}
	for i := 0; i < 3; i++ {
		cp := tree.Copy()
		wg.Add(1)
		go func() { reader(cp); cp.Release() }()
	}
	for i := 0; i < 40; i++ {
		at := 41 + (i*13)%100
		for !(src[at] >= '0' && src[at] <= '8') {
			at++
		}
		next, e := ccoreReplace(src, at, src[at]+1)
		tree.Edit(e)
		nt, err := p.ParseIncremental(next, tree)
		if err != nil {
			t.Fatal(err)
		}
		tree.Release()
		tree, src = nt, next
	}
	close(stop)
	wg.Wait()
	tree.Release()
}

// ccoreEdit reports whether the core tree
// took the edit and moves the core root; ccoreRoot reads it; ccoreLink is the one
// link (nil for a tree without a core tree).
func TestCcoreIncrSeams(t *testing.T) {
	lang := ccoreRouteLang(t, "json")
	defer ccoreCertifyIncr()()
	src := ccoreJSONSource(50)
	p := ccoreRouteParser(lang)
	tree, err := p.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	r0, ok := gts.CcoreTestSeamRoot(tree)
	if !ok || r0 == 0 || gts.CcoreTestSeamLink(tree) != gts.CcoreTestCoreTree(tree) {
		t.Fatalf("root %d ok %v", r0, ok)
	}
	at := 41
	for !(src[at] >= '0' && src[at] <= '8') {
		at++
	}
	_, e := ccoreReplace(src, at, src[at]+1)
	if !gts.CcoreTestSeamEdit(tree, e) {
		t.Fatal("the core tree did not take the edit")
	}
	if r1, _ := gts.CcoreTestSeamRoot(tree); r1 == r0 {
		t.Fatal("the core root did not move after the edit")
	}
	bad := e
	bad.StartByte, bad.OldEndByte = 9, 5 // start after end: refused, and the tree is marked as not carried
	if gts.CcoreTestSeamEdit(tree, bad) {
		t.Fatal("an edit with start after end was taken")
	}
	if _, _, broken := gts.CcoreTestLink(tree); !broken {
		t.Fatal("the refused edit did not mark the link")
	}
	gts.CcoreTestStripCore(tree)
	if gts.CcoreTestSeamLink(tree) != nil || gts.CcoreTestSeamEdit(tree, e) {
		t.Fatal("a tree without a core tree took an edit")
	}
	if _, ok := gts.CcoreTestSeamRoot(tree); ok {
		t.Fatal("a tree without a core tree has a core root")
	}
}

func TestCcoreReclamationIsCountedAgainstTheRequestBudget(t *testing.T) {
	ccoreSkipUnderRace(t, "the race detector changes the allocation sizes this test compares")
	defer ccoreCertifyIncr()()
	lang := ccoreRouteLang(t, "json")
	src := ccoreWideJSON(100000)
	lo, hi := len(src)/5, 4*len(src)/5
	changed := append([]byte(nil), src...)
	for i := lo; i < hi; i++ {
		if changed[i] == '1' {
			changed[i] = '7'
		}
	}
	final, e2 := ccoreReplace(changed, hi+1, changed[hi+1]) // a second, small edit for the reparse that drains
	// scenario: A -> B (the large replacement), release A, then reparse B -> C under a budget.
	var a *gts.Tree
	var m0, m1 runtime.MemStats // heap counters at the start of the request and after its drain
	var convs []bool            // for each limit check, whether the public tree was being built
	scenario := func(budget int64, releaseAt int) (totals []int64, declines map[string]int64, c *gts.Tree, fresh *gts.Tree) {
		p := ccoreRouteParser(lang)
		p.SetMemoryBudgetBytes(1 << 40)
		gts.CcoreTestDrainPool(lang)
		gts.DrainArenaPools()
		gts.CcoreTestReset()
		var err error
		a, err = p.Parse(src)
		if err != nil || a == nil || !gts.CcoreTestFromCore(a) {
			t.Fatalf("setup: %v", err)
		}
		a.Edit(gts.InputEdit{StartByte: uint32(lo), OldEndByte: uint32(hi), NewEndByte: uint32(hi),
			StartPoint: ccorePoint(src, lo), OldEndPoint: ccorePoint(src, hi), NewEndPoint: ccorePoint(changed, hi)})
		b, err := p.ParseIncremental(changed, a)
		if err != nil || b == nil || !gts.CcoreTestFromCore(b) {
			t.Fatalf("setup: the large replacement was not served by the core: %v", err)
		}
		b.Edit(e2)
		p.SetMemoryBudgetBytes(budget)
		stop := gts.CcoreTestPollProbe(func(total int64, sample bool, converting bool) {
			totals = append(totals, total)
			convs = append(convs, converting)
			if len(totals) == 2 && releaseAt == 1 {
				runtime.ReadMemStats(&m1)
			}
			if len(totals) == 1 && releaseAt == 1 {
				runtime.ReadMemStats(&m0)
			}
			if len(totals) == releaseAt {
				// The request holds the store's lease: the release of A cannot be applied now. At
				// call 1 (the start of the request) it is queued ahead of the request's first drain,
				// which frees every record that only A holds. At call 2 (after that drain) it is
				// queued behind the parse and freed by the drain at the exit of the request.
				a.Release()
			}
		})
		gts.CcoreTestReset()
		c, err = p.ParseIncremental(final, b)
		stop()
		if err != nil || c == nil {
			t.Fatalf("reparse: %v", err)
		}
		declines, _ = gts.CcoreTestCounters()
		b.Release()
		pp := gts.NewParser(lang)
		pp.SetAdmissionCandidateRoute(false)
		fresh, _ = pp.Parse(final)
		return
	}
	// The first check is the start of the request (Begin); the second is the one after the drain.
	convs = nil
	totals, _, c0, f0 := scenario(1<<40, 1)
	if !gts.CcoreTestFromCore(c0) {
		t.Fatal("setup: the reparse under a large budget was not served by the core")
	}
	c0.Release()
	f0.Release()
	if len(totals) < 3 {
		t.Fatalf("only %d limit checks ran", len(totals))
	}
	first, afterDrain := totals[0], totals[1]
	if afterDrain-first < 200<<10 {
		t.Fatalf("the drain added %d bytes to the account (%d to %d); the reclamation is not in it", afterDrain-first, first, afterDrain)
	}
	realDrain := int64(m1.TotalAlloc - m0.TotalAlloc)
	t.Logf("drain: counted %d bytes, allocated %d bytes (%.1f%%)", afterDrain-first, realDrain, 100*float64(afterDrain-first)/float64(realDrain))
	if float64(afterDrain-first) < 0.90*float64(realDrain) || float64(afterDrain-first) > 1.05*float64(realDrain)+16384 {
		t.Fatalf("the drain was counted as %d bytes for %d bytes really allocated; want 0.90 x allocated <= counted <= 1.05 x allocated + 16384", afterDrain-first, realDrain)
	}
	budget := first + 64<<10 // above the start of the request, below what the drain's growth adds
	totals2, decl, c1, f1 := scenario(budget, 1)
	defer c1.Release()
	defer f1.Release()
	if decl["stop_memory_budget"] != 1 {
		t.Fatalf("budget %d (the request starts at %d, the drain brings it to %d): declines %v; want one stop_memory_budget", budget, first, afterDrain, decl)
	}
	// The request ends at the check after the drain: nothing else was reserved for it.
	if len(totals2) != 2 || totals2[1] <= budget {
		t.Fatalf("limit checks %v with a budget of %d; want the request to end at the check after the drain", totals2, budget)
	}
	if gts.CcoreTestFromCore(c1) {
		t.Fatal("a request over its budget was served by the core")
	}
	if ccoreSExpr(c1, lang) != ccoreSExpr(f1, lang) {
		t.Fatal("the result of the declined reparse differs from a fresh parse by the present engine")
	}
	t.Logf("the request starts at %d bytes; the drain takes it to %d; with a budget of %d it declines there", first, afterDrain, budget)

	// A release that comes in during the parse is freed by the drain at the exit of the request.
	// That drain is counted too, and the check after it runs before the tree is served.
	convs = nil
	totals3, _, c2, f2 := scenario(1<<40, 2)
	if !gts.CcoreTestFromCore(c2) {
		t.Fatal("setup: the reparse with a release during the parse was not served by the core")
	}
	c2.Release()
	f2.Release()
	last := -1
	for i, conv := range convs {
		if !conv {
			last = i
		}
	}
	if last < 2 {
		t.Fatalf("limit checks %v (conversion %v): no check ran at the exit of the parse", totals3, convs)
	}
	exitDrain := totals3[last] - totals3[last-1]
	if exitDrain < 200<<10 {
		t.Fatalf("the drain at the exit added %d bytes to the account (%d to %d); the cleanup is not in it", exitDrain, totals3[last-1], totals3[last])
	}
	budget2 := totals3[last-1] + 64<<10 // above everything the parse reserved, below what the exit drain adds
	totals4, decl2, c3, f3 := scenario(budget2, 2)
	defer c3.Release()
	defer f3.Release()
	if decl2["stop_memory_budget"] != 1 || gts.CcoreTestFromCore(c3) {
		t.Fatalf("budget %d (the parse ends at %d, the exit drain brings it to %d): declines %v, core tree %t; want one stop_memory_budget and the present engine's tree", budget2, totals3[last-1], totals3[last], decl2, gts.CcoreTestFromCore(c3))
	}
	if ccoreSExpr(c3, lang) != ccoreSExpr(f3, lang) {
		t.Fatal("the result of the reparse declined at the exit differs from a fresh parse by the present engine")
	}
	t.Logf("a release during the parse: the exit drain adds %d bytes; with a budget of %d the request declines after it (%d limit checks)", exitDrain, budget2, len(totals4))
}

// ---- Lazy public trees against the eager conversion ----
//
// The oracle parses with the views switch on and compares every node with the
// eager conversion of a copy of the same core tree, matched by path (the list
// of child indexes from the root). Parts: 1 scalars, 2 structure and fields,
// 4 identity, 5 access orders (the views are first made through position
// lookups and parent chains, through a cursor, or in reverse order).

type ccoreOraclePath struct {
	parent *ccoreOraclePath
	index  int
}

func (p *ccoreOraclePath) String() string {
	var parts []string
	for ; p != nil; p = p.parent {
		parts = append(parts, fmt.Sprint(p.index))
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return "/" + strings.Join(parts, "/")
}

type ccoreOracle struct {
	t      *testing.T
	lang   *gts.Language
	src    []byte
	name   string
	nodes  int
	checks int
	diffs  int
	shown  int
}

func (o *ccoreOracle) fail(p *ccoreOraclePath, format string, args ...any) {
	o.diffs++
	if o.shown < 8 {
		o.shown++
		o.t.Logf("ORACLEDIFF %s path=%s %s", o.name, p, fmt.Sprintf(format, args...))
	}
}

func (o *ccoreOracle) eq(p *ccoreOraclePath, what string, got, want any) {
	o.checks++
	if got != want {
		o.fail(p, "%s: view %v, eager %v", what, got, want)
	}
}

// triple names a node for comparison across trees.
func ccoreOracleTriple(n *gts.Node, lang *gts.Language) string {
	if n == nil {
		return "nil"
	}
	return fmt.Sprintf("%s@%d-%d", n.Type(lang), n.StartByte(), n.EndByte())
}

func (o *ccoreOracle) scalars(p *ccoreOraclePath, v, e *gts.Node) {
	o.eq(p, "Symbol", v.Symbol(), e.Symbol())
	o.eq(p, "Type", v.Type(o.lang), e.Type(o.lang))
	o.eq(p, "IsNamed", v.IsNamed(), e.IsNamed())
	o.eq(p, "IsExtra", v.IsExtra(), e.IsExtra())
	o.eq(p, "IsMissing", v.IsMissing(), e.IsMissing())
	o.eq(p, "IsError", v.IsError(), e.IsError())
	o.eq(p, "HasError", v.HasError(), e.HasError())
	o.eq(p, "HasErrorOrMissing", v.HasErrorOrMissing(), e.HasErrorOrMissing())
	o.eq(p, "HasChanges", v.HasChanges(), e.HasChanges())
	o.eq(p, "ParseState", v.ParseState(), e.ParseState())
	o.eq(p, "PreGotoState", v.PreGotoState(), e.PreGotoState())
	o.eq(p, "StartByte", v.StartByte(), e.StartByte())
	o.eq(p, "EndByte", v.EndByte(), e.EndByte())
	o.eq(p, "StartPoint", v.StartPoint(), e.StartPoint())
	o.eq(p, "EndPoint", v.EndPoint(), e.EndPoint())
	o.eq(p, "Range", v.Range(), e.Range())
	o.eq(p, "ChildCount", v.ChildCount(), e.ChildCount())
	o.eq(p, "NamedChildCount", v.NamedChildCount(), e.NamedChildCount())
	if v.EndByte() <= uint32(len(o.src)) {
		o.eq(p, "Text", v.Text(o.src), e.Text(o.src))
	}
}

// compare walks both trees with an explicit stack (the depth of a tree does not limit it).
func (o *ccoreOracle) compare(vroot, eroot *gts.Node) {
	type pair struct {
		v, e *gts.Node
		p    *ccoreOraclePath
	}
	o.eq(nil, "SExpr", vroot.SExpr(o.lang), eroot.SExpr(o.lang))
	stack := []pair{{vroot, eroot, nil}}
	nf := len(o.lang.FieldNames)
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		v, e, p := cur.v, cur.e, cur.p
		o.nodes++
		o.scalars(p, v, e)
		vk, ek := v.Children(), e.Children()
		o.eq(p, "len(Children)", len(vk), len(ek))
		if len(vk) != len(ek) {
			continue
		}
		wide := len(vk) > 512
		for i := range vk {
			if wide && i >= 32 && i+32 < len(vk) && i%211 != 0 {
				// A node with more than 512 children: the first 32, the last 32 and every 211th get the per-child checks.
				stack = append(stack, pair{vk[i], ek[i], &ccoreOraclePath{p, i}})
				continue
			}
			o.checks++
			if v.Child(i) != vk[i] {
				o.fail(p, "Child(%d) is not the same pointer twice", i)
			}
			if vk[i].Parent() != v {
				o.fail(p, "Parent of child %d is another node", i)
			}
			var wantNext, wantPrev *gts.Node
			if i+1 < len(vk) {
				wantNext = vk[i+1]
			}
			if i > 0 {
				wantPrev = vk[i-1]
			}
			if vk[i].NextSibling() != wantNext {
				o.fail(p, "NextSibling of child %d", i)
			}
			if vk[i].PrevSibling() != wantPrev {
				o.fail(p, "PrevSibling of child %d", i)
			}
			o.eq(p, fmt.Sprintf("FieldNameForChild(%d)", i), v.FieldNameForChild(i, o.lang), e.FieldNameForChild(i, o.lang))
			stack = append(stack, pair{vk[i], ek[i], &ccoreOraclePath{p, i}})
		}
		named := v.NamedChildCount()
		for j := 0; j < named; j++ {
			if wide && j >= 32 && j+32 < named {
				continue
			}
			vn, en := v.NamedChild(j), e.NamedChild(j)
			o.eq(p, fmt.Sprintf("NamedChild(%d)", j), ccoreOracleTriple(vn, o.lang), ccoreOracleTriple(en, o.lang))
			if vn != nil {
				found := false
				for _, k := range vk {
					if k == vn {
						found = true
						break
					}
				}
				o.checks++
				if !found && !wide {
					o.fail(p, "NamedChild(%d) is not one of the children", j)
				}
			}
		}
		if nf > 1 {
			// every field the children carry, and three others that rotate with the node
			seen := map[string]bool{}
			names := []string{}
			for i := range ek {
				if f := e.FieldNameForChild(i, o.lang); f != "" && !seen[f] {
					seen[f] = true
					names = append(names, f)
				}
			}
			for k := 0; k < 3; k++ {
				names = append(names, o.lang.FieldNames[1+(o.nodes+k)%(nf-1)])
			}
			for _, f := range names {
				if f == "" {
					continue
				}
				o.eq(p, "ChildByFieldName("+f+")", ccoreOracleTriple(v.ChildByFieldName(f, o.lang), o.lang), ccoreOracleTriple(e.ChildByFieldName(f, o.lang), o.lang))
			}
		}
	}
}

// ccoreOracleSamples returns the committed sample of each language that has one.
func ccoreOracleSamples(t *testing.T) map[string][]byte {
	raw, err := os.ReadFile(filepath.Join("internal", "benchfixtures", "real_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Entries []struct {
			Language      string `json:"language"`
			Role          string `json:"role"`
			CommittedPath string `json:"committed_path"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, e := range m.Entries {
		if e.Role != "sample" || e.CommittedPath == "" || out[e.Language] != nil {
			continue
		}
		src, err := os.ReadFile(filepath.Join("internal", "benchfixtures", e.CommittedPath))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Language] = src
	}
	return out
}

// cursors walks a view tree and the eager tree with two tree cursors in step and
// compares the node, its field id and its field name at every step (up to a cap).
func (o *ccoreOracle) cursors(vt, et *gts.Tree) {
	vc, ec := gts.NewTreeCursorFromTree(vt), gts.NewTreeCursorFromTree(et)
	for steps := 0; steps < 300000; steps++ {
		vn, en := vc.CurrentNode(), ec.CurrentNode()
		o.eq(nil, "cursor Symbol", vn.Symbol(), en.Symbol())
		o.eq(nil, "cursor StartByte", vn.StartByte(), en.StartByte())
		o.eq(nil, "cursor EndByte", vn.EndByte(), en.EndByte())
		o.eq(nil, "cursor CurrentFieldID", vc.CurrentFieldID(), ec.CurrentFieldID())
		o.eq(nil, "cursor CurrentFieldName", vc.CurrentFieldName(), ec.CurrentFieldName())
		if a, b := vc.GotoFirstChild(), ec.GotoFirstChild(); a != b {
			o.fail(nil, "GotoFirstChild: view %v, eager %v", a, b)
			return
		} else if a {
			continue
		}
		for {
			a, b := vc.GotoNextSibling(), ec.GotoNextSibling()
			if a != b {
				o.fail(nil, "GotoNextSibling: view %v, eager %v", a, b)
				return
			}
			if a {
				break
			}
			a, b = vc.GotoParent(), ec.GotoParent()
			if a != b {
				o.fail(nil, "GotoParent: view %v, eager %v", a, b)
				return
			}
			if !a {
				return
			}
		}
	}
}

// ccoreOraclePrime makes views of a tree in an order other than the descent of compare.
func ccoreOraclePrime(root *gts.Node, tree *gts.Tree, src []byte, lang *gts.Language, mode int) {
	switch mode {
	case 1: // leaf first: position lookups, then the parent chain to the root
		step := max(1, len(src)/97)
		for at := 0; at < len(src); at += step {
			for n := root.DescendantForByteRange(uint32(at), uint32(at+1)); n != nil; n = n.Parent() {
			}
		}
	case 2: // a cursor over the whole tree
		c := gts.NewTreeCursorFromTree(tree)
		for {
			c.CurrentNode()
			if c.GotoFirstChild() {
				continue
			}
			for !c.GotoNextSibling() {
				if !c.GotoParent() {
					return
				}
			}
		}
	case 4: // NamedChild first, through the one-past-the-end index, before any other descent
		var visit func(n *gts.Node, depth int)
		visit = func(n *gts.Node, depth int) {
			if depth > 2000 {
				return
			}
			count := n.NamedChildCount()
			for i := 0; i <= count; i++ {
				if c := n.NamedChild(i); c != nil && i < count {
					visit(c, depth+1)
				}
			}
		}
		visit(root, 0)
	case 5: // field lookup first
		var visit func(n *gts.Node, depth int)
		visit = func(n *gts.Node, depth int) {
			if depth > 2000 {
				return
			}
			for f := 1; f < len(lang.FieldNames); f++ {
				n.ChildByFieldName(lang.FieldNames[f], lang)
			}
			for i := 0; i < n.ChildCount() && i < 64; i++ {
				visit(n.Child(i), depth+1)
			}
		}
		visit(root, 0)
	case 3: // reverse order
		var visit func(n *gts.Node, depth int)
		visit = func(n *gts.Node, depth int) {
			if depth > 2000 {
				return
			}
			for i := n.ChildCount() - 1; i >= 0; i-- {
				visit(n.Child(i), depth+1)
			}
		}
		visit(root, 0)
	}
}

// ccoreSameConversions reports the conversion counters that changed between two readings.
func ccoreSameConversions(before, after map[string]int64) []string {
	var changed []string
	for k, v := range after {
		if k != "root_only_edits" && v != before[k] {
			changed = append(changed, fmt.Sprintf("%s %d->%d", k, before[k], v))
		}
	}
	return changed
}

// TestCcoreViewsOracle is oracle parts 1, 2, 4 and 5 over the committed sample of every language
// the route serves, in six access orders. CCORE_VIEWS_LANGS=a,b limits the languages; a named
// language that is missing or not served fails. A language the route declines is reported as
// not covered; the test fails if nothing at all was compared. Every read-only comparison must
// leave the tree backed by views and make no whole-tree conversion.
func TestCcoreViewsOracle(t *testing.T) {
	defer ccoreCertifyAll()()
	samples := ccoreOracleSamples(t)
	names := make([]string, 0, len(samples))
	for n := range samples {
		names = append(names, n)
	}
	sort.Strings(names)
	explicit := os.Getenv("CCORE_VIEWS_LANGS") != ""
	if explicit {
		names = strings.Split(os.Getenv("CCORE_VIEWS_LANGS"), ",")
	}
	var nodes, checks, diffs, served int
	var notCovered []string
	for _, name := range names {
		src := samples[name]
		entry := grammars.DetectLanguageByName(name)
		if src == nil || entry == nil || entry.Language() == nil {
			if explicit {
				t.Fatalf("%s: no sample or no grammar", name)
			}
			notCovered = append(notCovered, name+" (no sample)")
			continue
		}
		lang := entry.Language()
		langNodes := 0
		for mode := 0; mode <= 5; mode++ {
			restore := gts.CcoreTestViewsOn(true)
			tree, err := ccoreRouteParser(lang).Parse(src)
			restore()
			if err != nil || tree == nil {
				t.Fatalf("%s: parse: %v", name, err)
			}
			if _, ok := gts.CcoreTestViewHeaders(tree); !ok {
				tree.Release() // the route declined this language
				break
			}
			eager := gts.CcoreTestEager(tree)
			if eager == nil {
				tree.Release()
				t.Errorf("%s: no eager conversion", name)
				break
			}
			_, _, before := gts.CcoreTestViewCounters()
			o := &ccoreOracle{t: t, lang: lang, src: src, name: fmt.Sprintf("%s/order%d", name, mode)}
			if root := tree.RootNode(); root != nil {
				if mode == 2 {
					o.cursors(tree, eager)
				}
				ccoreOraclePrime(root, tree, src, lang, mode)
				o.compare(root, eager.RootNode())
			}
			_, _, after := gts.CcoreTestViewCounters()
			if ch := ccoreSameConversions(before, after); len(ch) != 0 {
				t.Errorf("%s order %d: read-only checks converted the tree: %v", name, mode, ch)
			}
			if _, ok := gts.CcoreTestViewHeaders(tree); !ok {
				t.Errorf("%s order %d: the tree is no longer backed by views after read-only checks", name, mode)
			}
			nodes += o.nodes
			checks += o.checks
			diffs += o.diffs
			langNodes += o.nodes
			if mode == 0 {
				cp := tree.Copy() // identity: a copy has other pointers and equal answers
				if cp.RootNode() == tree.RootNode() || cp.RootNode().SExpr(lang) != tree.RootNode().SExpr(lang) {
					t.Errorf("%s: Copy gives the same root or another tree", name)
				}
				cp.Release()
			}
			eager.Release()
			tree.Release()
		}
		if langNodes == 0 {
			if explicit {
				t.Fatalf("%s: named, but the route did not serve it or nothing was compared", name)
			}
			notCovered = append(notCovered, name)
			continue
		}
		served++
	}
	trees, made, conv := gts.CcoreTestViewCounters()
	fmt.Printf("VIEWSORACLE served=%d nodes=%d checks=%d differences=%d not_covered=%d %v view_trees=%d headers=%d conversions=%v\n", served, nodes, checks, diffs, len(notCovered), notCovered, trees, made, conv)
	if served == 0 || nodes == 0 || checks == 0 {
		t.Fatalf("nothing was checked: served %d, nodes %d, checks %d", served, nodes, checks)
	}
	if diffs != 0 {
		t.Errorf("%d differences between the views and the eager conversion", diffs)
	}
}

// Consumer oracle counters are deltas, so other tests do not affect the report.
type ccoreConsumerStats struct {
	languages, trees, steps, checks, captures, diffs int
	missing                                          []string
	treeBefore, viewBefore                           int64
	convBefore                                       map[string]int64
}

func newCcoreConsumerStats() *ccoreConsumerStats {
	a, b, c := gts.CcoreTestViewCounters()
	return &ccoreConsumerStats{treeBefore: a, viewBefore: b, convBefore: c}
}
func (s *ccoreConsumerStats) report(t *testing.T, arm string) {
	a, b, c := gts.CcoreTestViewCounters()
	for _, k := range []string{"tree_edit_with_views", "node_edit", "adopt", "offset_root", "test", "other"} {
		c[k] -= s.convBefore[k]
	}
	fmt.Printf("%s oracle languages=%d trees=%d steps=%d checks=%d captures=%d differences=%d view_trees=%d views_made=%d conversions=%v not_served=%v\n", arm, s.languages, s.trees, s.steps, s.checks, s.captures, s.diffs, a-s.treeBefore, b-s.viewBefore, c, s.missing)
	if s.languages == 0 || s.trees == 0 || s.checks == 0 {
		t.Errorf("%s oracle compared nothing", arm)
	}
	if s.diffs > 0 {
		t.Errorf("%s oracle found %d differences", arm, s.diffs)
	}
}
func (s *ccoreConsumerStats) compare(t *testing.T, where string, got, want any) {
	s.checks++
	if !reflect.DeepEqual(got, want) {
		s.diffs++
		if s.diffs <= 20 {
			t.Errorf("%s: view=%v eager=%v", where, got, want)
		}
	}
}
func ccoreConsumerNames(samples map[string][]byte) []string {
	names := make([]string, 0, len(samples))
	for n := range samples {
		names = append(names, n)
	}
	sort.Strings(names)
	if v := os.Getenv("CCORE_VIEWS_LANGS"); v != "" {
		names = strings.Split(v, ",")
	}
	return names
}
func ccoreConsumerTree(t *testing.T, lang *gts.Language, src []byte) (*gts.Tree, *gts.Tree) {
	t.Helper()
	restore := gts.CcoreTestViewsOn(true)
	tree, err := ccoreRouteParser(lang).Parse(src)
	restore()
	if err != nil || tree == nil {
		t.Errorf("parse: %v", err)
		return nil, nil
	}
	if _, ok := gts.CcoreTestViewHeaders(tree); !ok {
		tree.Release()
		return nil, nil
	}
	eager := gts.CcoreTestEager(tree)
	if eager == nil {
		t.Error("eager conversion failed")
		tree.Release()
		return nil, nil
	}
	return tree, eager
}

type ccoreCursorSnapshot struct {
	Type, Field, Text    string
	Symbol               gts.Symbol
	Start, End           uint32
	StartPoint, EndPoint gts.Point
	Named                bool
	FieldID              gts.FieldID
	Depth                int
}

func ccoreCursorState(c *gts.TreeCursor, lang *gts.Language) ccoreCursorSnapshot {
	n := c.CurrentNode()
	if n == nil {
		return ccoreCursorSnapshot{}
	}
	return ccoreCursorSnapshot{n.Type(lang), c.CurrentFieldName(), c.CurrentNodeText(), n.Symbol(), n.StartByte(), n.EndByte(), n.StartPoint(), n.EndPoint(), n.IsNamed(), c.CurrentFieldID(), c.Depth()}
}
func ccoreCursorWalk(t *testing.T, s *ccoreConsumerStats, name string, vt, et *gts.Tree, lang *gts.Language, reverse bool, positions []uint32, points []gts.Point) {
	v, e := gts.NewTreeCursorFromTree(vt), gts.NewTreeCursorFromTree(et)
	positionParents := 0
	move := func(label string, vf, ef func() bool) bool {
		a, b := vf(), ef()
		s.compare(t, name+" "+label, a, b)
		s.steps++
		s.compare(t, name+" "+label+" destination", ccoreCursorState(v, lang), ccoreCursorState(e, lang))
		return a && b
	}
	for {
		s.compare(t, name+" cursor", ccoreCursorState(v, lang), ccoreCursorState(e, lang))
		s.compare(t, name+" CurrentNodeType", v.CurrentNodeType(), e.CurrentNodeType())
		s.compare(t, name+" CurrentNodeIsNamed", v.CurrentNodeIsNamed(), e.CurrentNodeIsNamed())
		for _, op := range []struct {
			name string
			fn   func(*gts.TreeCursor) bool
		}{
			{"first named", (*gts.TreeCursor).GotoFirstNamedChild}, {"last named", (*gts.TreeCursor).GotoLastNamedChild},
			{"next named", (*gts.TreeCursor).GotoNextNamedSibling}, {"previous named", (*gts.TreeCursor).GotoPrevNamedSibling},
		} {
			vc, ec := v.Copy(), e.Copy()
			a, b := op.fn(vc), op.fn(ec)
			s.compare(t, name+" "+op.name, a, b)
			s.compare(t, name+" "+op.name+" destination", ccoreCursorState(vc, lang), ccoreCursorState(ec, lang))
		}
		// Test every field present on a child, as well as sentinel and absent IDs.
		ids := []gts.FieldID{0, e.CurrentFieldID()}
		probe := e.Copy()
		if probe.GotoFirstChild() {
			for {
				ids = append(ids, probe.CurrentFieldID())
				if !probe.GotoNextSibling() {
					break
				}
			}
		}
		if positionParents < 8 {
			positionParents++
			for i, pos := range positions {
				vc, ec := v.Copy(), e.Copy()
				a, b := vc.GotoFirstChildForByte(pos), ec.GotoFirstChildForByte(pos)
				s.compare(t, name+" byte index", a, b)
				s.compare(t, name+" byte destination", ccoreCursorState(vc, lang), ccoreCursorState(ec, lang))
				vc, ec = v.Copy(), e.Copy()
				a, b = vc.GotoFirstChildForPoint(points[i]), ec.GotoFirstChildForPoint(points[i])
				s.compare(t, name+" point index", a, b)
				s.compare(t, name+" point destination", ccoreCursorState(vc, lang), ccoreCursorState(ec, lang))
			}
		}
		if len(lang.FieldNames) > 1 {
			ids = append(ids, 1, gts.FieldID(len(lang.FieldNames)-1))
		}
		for _, id := range ids {
			vc, ec := v.Copy(), e.Copy()
			a, b := vc.GotoChildByFieldID(id), ec.GotoChildByFieldID(id)
			s.compare(t, name+" field ID", a, b)
			s.compare(t, name+" field ID destination", ccoreCursorState(vc, lang), ccoreCursorState(ec, lang))
			if int(id) < len(lang.FieldNames) {
				vc, ec = v.Copy(), e.Copy()
				a, b = vc.GotoChildByFieldName(lang.FieldNames[id]), ec.GotoChildByFieldName(lang.FieldNames[id])
				s.compare(t, name+" field name", a, b)
				s.compare(t, name+" field name destination", ccoreCursorState(vc, lang), ccoreCursorState(ec, lang))
			}
		}
		var child, sibling func(*gts.TreeCursor) bool
		if reverse {
			child = (*gts.TreeCursor).GotoLastChild
			sibling = (*gts.TreeCursor).GotoPrevSibling
		} else {
			child = (*gts.TreeCursor).GotoFirstChild
			sibling = (*gts.TreeCursor).GotoNextSibling
		}
		if move("child", func() bool { return child(v) }, func() bool { return child(e) }) {
			continue
		}
		for {
			if move("sibling", func() bool { return sibling(v) }, func() bool { return sibling(e) }) {
				break
			}
			if !move("parent", v.GotoParent, e.GotoParent) {
				goto done
			}
		}
	}
done:
	v.Reset(vt.RootNode())
	e.Reset(et.RootNode())
	s.compare(t, name+" Reset", ccoreCursorState(v, lang), ccoreCursorState(e, lang))
	for i, pos := range positions {
		v.ResetTree(vt)
		e.ResetTree(et)
		a, b := v.GotoFirstChildForByte(pos), e.GotoFirstChildForByte(pos)
		s.compare(t, name+" byte index", a, b)
		s.compare(t, name+" byte destination", ccoreCursorState(v, lang), ccoreCursorState(e, lang))
		v.ResetTree(vt)
		e.ResetTree(et)
		a, b = v.GotoFirstChildForPoint(points[i]), e.GotoFirstChildForPoint(points[i])
		s.compare(t, name+" point index", a, b)
		s.compare(t, name+" point destination", ccoreCursorState(v, lang), ccoreCursorState(e, lang))
	}
}

// Collect boundaries using the eager tree, leaving the view untouched.
func ccoreCursorPositions(root *gts.Node, src []byte) ([]uint32, []gts.Point) {
	const capPositions = 128
	pos := []uint32{0, uint32(len(src))}
	stack := []*gts.Node{root}
	for len(stack) > 0 && len(pos) < capPositions {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		pos = append(pos, n.StartByte(), n.EndByte())
		for i := n.ChildCount() - 1; i >= 0; i-- {
			stack = append(stack, n.Child(i))
		}
	}
	rng := rand.New(rand.NewSource(41))
	for i := 0; i < 32; i++ {
		pos = append(pos, uint32(rng.Intn(len(src)+1)))
	}
	points := make([]gts.Point, len(pos))
	for i, p := range pos {
		for _, b := range src[:min(int(p), len(src))] {
			if b == '\n' {
				points[i].Row++
				points[i].Column = 0
			} else {
				points[i].Column++
			}
		}
	}
	return pos, points
}
func TestCcoreViewsCursorOracle(t *testing.T) {
	defer ccoreCertifyAll()()
	s := newCcoreConsumerStats()
	defer s.report(t, "CURSOR")
	samples := ccoreOracleSamples(t)
	for _, name := range ccoreConsumerNames(samples) {
		entry := grammars.DetectLanguageByName(name)
		src := samples[name]
		if entry == nil || src == nil || entry.Language() == nil {
			s.missing = append(s.missing, name+" (sample or grammar)")
			continue
		}
		lang := entry.Language()
		served := false
		for mode := 0; mode < 2; mode++ {
			view, eager := ccoreConsumerTree(t, lang, src)
			if view == nil {
				break
			}
			served = true
			s.trees++
			pos, points := ccoreCursorPositions(eager.RootNode(), src)
			if mode == 1 {
				stack := []*gts.Node{view.RootNode()}
				for len(stack) > 0 {
					n := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					for _, c := range n.Children() {
						stack = append(stack, c)
					}
					_ = n.Type(lang)
				}
			}
			ccoreCursorWalk(t, s, fmt.Sprintf("%s/mode%d/forward", name, mode), view, eager, lang, false, pos, points)
			ccoreCursorWalk(t, s, fmt.Sprintf("%s/mode%d/reverse", name, mode), view, eager, lang, true, pos, points)
			view.Release()
			eager.Release()
		}
		if served {
			s.languages++
		} else {
			s.missing = append(s.missing, name+" (route declined)")
		}
	}
}

type ccoreQueryCapture struct {
	Name, Type string
	Start, End uint32
}
type ccoreQueryMatch struct {
	Pattern  int
	Captures []ccoreQueryCapture
}

func ccoreQueryRecords(matches []gts.QueryMatch, lang *gts.Language) ([]ccoreQueryMatch, int) {
	var out []ccoreQueryMatch
	count := 0
	for _, m := range matches {
		r := ccoreQueryMatch{Pattern: m.PatternIndex}
		for _, c := range m.Captures {
			r.Captures = append(r.Captures, ccoreQueryCapture{c.Name, c.Node.Type(lang), c.Node.StartByte(), c.Node.EndByte()})
			count++
		}
		out = append(out, r)
	}
	return out, count
}
func TestCcoreViewsQueryOracle(t *testing.T) {
	defer ccoreCertifyAll()()
	s := newCcoreConsumerStats()
	defer s.report(t, "QUERY")
	samples := ccoreOracleSamples(t)
	for _, name := range ccoreConsumerNames(samples) {
		entry := grammars.DetectLanguageByName(name)
		src := samples[name]
		if entry == nil || src == nil || entry.Language() == nil {
			s.missing = append(s.missing, name+" (sample or grammar)")
			continue
		}
		lang := entry.Language()
		view, eager := ccoreConsumerTree(t, lang, src)
		if view == nil {
			s.missing = append(s.missing, name+" (route declined)")
			continue
		}
		for _, field := range lang.FieldNames {
			if field == "" {
				continue
			}
			for _, pattern := range []string{fmt.Sprintf("(_ %s: (_) @c)", field), fmt.Sprintf("(_ !%s) @n", field)} {
				q, err := gts.NewQuery(pattern, lang)
				if err != nil {
					t.Errorf("%s %q: %v", name, pattern, err)
					continue
				}
				var records [4][]ccoreQueryMatch
				for i, tree := range []*gts.Tree{eager, view} {
					records[2*i], _ = ccoreQueryRecords(q.Execute(tree), lang)
					cursor := q.Exec(tree.RootNode(), lang, src)
					var matches []gts.QueryMatch
					for {
						m, ok := cursor.NextMatch()
						if !ok {
							break
						}
						matches = append(matches, m)
					}
					records[2*i+1], _ = ccoreQueryRecords(matches, lang)
				}
				for i := 1; i < len(records); i++ {
					s.compare(t, fmt.Sprintf("%s %s field mode %d", name, pattern, i), records[0], records[i])
				}
			}
		}
		queries := []struct{ name, source string }{{"highlights", entry.HighlightQuery}, {"tags", entry.TagsQuery}}
		used := false
		for _, item := range queries {
			if strings.TrimSpace(item.source) == "" {
				continue
			}
			q, err := gts.NewQuery(item.source, lang)
			if err != nil {
				t.Errorf("%s %s query compile: %v", name, item.name, err)
				continue
			}
			used = true
			for _, mode := range []string{"execute", "stream", "range"} {
				var vm, em []gts.QueryMatch
				if mode == "execute" {
					vm = q.Execute(view)
					em = q.Execute(eager)
				} else {
					vc, ec := q.Exec(view.RootNode(), lang, src), q.Exec(eager.RootNode(), lang, src)
					if mode == "range" {
						vc.SetByteRange(uint32(len(src)/3), uint32(2*len(src)/3))
						ec.SetByteRange(uint32(len(src)/3), uint32(2*len(src)/3))
					}
					for {
						m, ok := vc.NextMatch()
						if !ok {
							break
						}
						vm = append(vm, m)
					}
					for {
						m, ok := ec.NextMatch()
						if !ok {
							break
						}
						em = append(em, m)
					}
				}
				vr, vc := ccoreQueryRecords(vm, lang)
				er, ec := ccoreQueryRecords(em, lang)
				s.captures += max(vc, ec)
				s.compare(t, name+" "+item.name+" "+mode, vr, er)
			}
		}
		if used {
			s.languages++
			s.trees++
		} else {
			s.missing = append(s.missing, name+" (no compilable shipped query)")
		}
		view.Release()
		eager.Release()
	}
	if s.captures == 0 {
		t.Error("query oracle compared no captures")
	}
}

// TestCcoreViewsEditSessions checks that Tree.Edit on a tree
// with only the root view leaves a root equal to the eager conversion of the edited core
// tree, converts nothing, and the reparse equals a fresh parse and passes the oracle.
func TestCcoreViewsEditSessions(t *testing.T) {
	defer ccoreCertifyAll()()
	samples := ccoreOracleSamples(t)
	names := []string{"json", "typescript", "tsx", "python", "go"}
	if only := os.Getenv("CCORE_VIEWS_LANGS"); only != "" {
		names = strings.Split(only, ",")
	}
	served := 0
	for _, name := range names {
		src := samples[name]
		entry := grammars.DetectLanguageByName(name)
		if src == nil || entry == nil || entry.Language() == nil {
			if os.Getenv("CCORE_VIEWS_LANGS") != "" {
				t.Fatalf("%s unavailable", name)
			}
			t.Logf("%s unavailable", name)
			continue
		}
		lang := entry.Language()
		restore := gts.CcoreTestViewsOn(true)
		p := ccoreRouteParser(lang)
		tree, err := p.Parse(src)
		if err != nil || tree == nil {
			restore()
			t.Fatalf("%s: parse: %v", name, err)
		}
		if _, ok := gts.CcoreTestViewHeaders(tree); !ok {
			restore()
			tree.Release()
			if os.Getenv("CCORE_VIEWS_LANGS") != "" {
				t.Fatalf("%s: the route declined it", name)
			}
			t.Logf("%s: the route declined it", name)
			continue
		}
		served++
		_, _, before := gts.CcoreTestViewCounters()
		o := &ccoreOracle{t: t, lang: lang, name: name + "/session"}
		fresh, steps := 0, 0
		for i, step := range benchfixtures.EditingSession(src) {
			steps++
			tree.Edit(step.Edit)
			if made, ok := gts.CcoreTestViewHeaders(tree); !ok || made != 1 {
				t.Fatalf("%s step %d: after Tree.Edit the tree has %d headers (view tree %t), want the root only", name, i+1, made, ok)
			}
			eager := gts.CcoreTestEager(tree)
			if eager == nil {
				t.Fatalf("%s step %d: no eager conversion", name, i+1)
			}
			o.src = step.Source
			o.scalars(nil, tree.RootNode(), eager.RootNode())
			eager.Release()
			next, err := p.ParseIncremental(step.Source, tree)
			if err != nil || next == nil {
				t.Fatalf("%s step %d: reparse: %v", name, i+1, err)
			}
			tree.Release()
			tree = next
			freshTree, err := ccoreRouteParser(lang).Parse(step.Source)
			if err != nil || freshTree == nil {
				t.Fatalf("%s step %d: fresh parse: %v", name, i+1, err)
			}
			cpTree := tree.Copy() // read the whole tree through a copy: reading `tree` would make views below its root
			sexpr := cpTree.RootNode().SExpr(lang)
			cpTree.Release()
			if sexpr != freshTree.RootNode().SExpr(lang) || tree.RootNode().Range() != freshTree.RootNode().Range() {
				fresh++
				t.Errorf("%s step %d: the reparse differs from a fresh parse", name, i+1)
			}
			freshTree.Release()
			if i%12 == 0 { // the full oracle on a copy of the reparsed tree (the copy has its own views)
				cp := tree.Copy()
				eager := gts.CcoreTestEager(tree)
				o.compare(cp.RootNode(), eager.RootNode())
				eager.Release()
				cp.Release()
			}
		}
		_, _, after := gts.CcoreTestViewCounters()
		tree.Release()
		restore()
		if after["tree_edit_with_views"] != before["tree_edit_with_views"] {
			t.Errorf("%s: %d edits converted the tree although no view existed below the root", name, after["tree_edit_with_views"]-before["tree_edit_with_views"])
		}
		if steps == 0 || o.checks == 0 {
			t.Fatalf("%s: no edit-session comparisons", name)
		}
		if got := after["root_only_edits"] - before["root_only_edits"]; got != int64(steps) {
			t.Errorf("%s: %d root-only edits for %d steps", name, got, steps)
		}
		fmt.Printf("VIEWSSESSION lang=%s steps=%d differences=%d reparse_differs=%d root_only_edits=%d\n", name, steps, o.diffs, fresh, after["root_only_edits"]-before["root_only_edits"])
		if o.diffs != 0 {
			t.Errorf("%s: %d differences", name, o.diffs)
		}
	}
	if served == 0 {
		t.Fatal("no edit sessions were compared")
	}
}

// TestCcoreViewsEditKeepsHeldNodes: a node a caller holds stays correct after Tree.Edit. With a
// view below the root, Tree.Edit converts the tree first (reason tree_edit_with_views) and the
// present walk then moves the node, as for an eager tree.
func TestCcoreViewsEditKeepsHeldNodes(t *testing.T) {
	defer ccoreCertifyAll()()
	lang := ccoreRouteLang(t, "json")
	src := ccoreJSONSource(60)
	restore := gts.CcoreTestViewsOn(true)
	defer restore()
	tree, err := ccoreRouteParser(lang).Parse(src)
	if err != nil || tree == nil {
		t.Fatal(err)
	}
	defer tree.Release()
	held := tree.RootNode().Child(0)          // the array
	last := held.Child(held.ChildCount() - 1) // its closing bracket
	startBefore, endBefore := last.StartByte(), last.EndByte()
	_, _, before := gts.CcoreTestViewCounters()
	out, edit := ccoreInsert(src, 3, ' ')
	tree.Edit(edit)
	_, _, after := gts.CcoreTestViewCounters()
	if after["tree_edit_with_views"] != before["tree_edit_with_views"]+1 {
		t.Fatalf("conversions by reason: before %v, after %v", before, after)
	}
	if held != tree.RootNode().Child(0) {
		t.Errorf("the node held before the edit is not the first child after it")
	}
	if last.StartByte() != startBefore+1 || last.EndByte() != endBefore+1 {
		t.Errorf("a held node did not move with the edit: %d-%d, was %d-%d", last.StartByte(), last.EndByte(), startBefore, endBefore)
	}
	next, err := ccoreRouteParser(lang).ParseIncremental(out, tree)
	if err != nil || next == nil {
		t.Fatal(err)
	}
	defer next.Release()
	fresh, _ := ccoreRouteParser(lang).Parse(out)
	defer fresh.Release()
	if next.RootNode().SExpr(lang) != fresh.RootNode().SExpr(lang) {
		t.Error("the reparse differs from a fresh parse")
	}
}

func TestCcoreViewsShortcut(t *testing.T) {
	defer ccoreCertifyIncr()()
	defer gts.CcoreTestViewsOn(true)()
	lang := ccoreRouteLang(t, "json")
	p := ccoreRouteParser(lang)
	src := []byte(`[123,456]`)
	tree, err := p.Parse(src)
	if err != nil || tree == nil {
		t.Fatalf("parse: %v", err)
	}
	defer func() { tree.Release() }()
	shortcutReparses := func() int64 { _, _, c := gts.CcoreTestViewCounters(); return c["shortcut_reparses"] }
	before := shortcutReparses()
	for i := 0; i < 8; i++ {
		old := tree
		held := old.RootNode().Child(0)
		beforeHeld := held.Range()
		beforeDigest := ccoreWalkDigest(old, lang)
		nextSrc, edit := ccoreReplace(src, 2, byte('2'+(i+1)%7))
		old.Edit(edit)
		oldDigest := ccoreWalkDigest(old, lang)
		if oldDigest != beforeDigest {
			t.Fatal("same-length edit changed old visible tree")
		}
		next, err := p.ParseIncremental(nextSrc, old)
		if err != nil || next == nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if shortcutReparses() != before+int64(i)+1 {
			t.Fatalf("step %d did not use shortcut", i)
		}
		fresh, err := ccoreRouteParser(lang).Parse(nextSrc)
		if err != nil || fresh == nil {
			t.Fatalf("fresh: %v", err)
		}
		if ccoreWalkDigest(next, lang) != ccoreWalkDigest(fresh, lang) {
			t.Fatalf("step %d differs from fresh", i)
		}
		if ccoreWalkDigest(old, lang) != oldDigest || held.Range() != beforeHeld {
			t.Fatalf("step %d changed old tree or held node", i)
		}
		seen := map[*gts.Node]bool{}
		var collect func(*gts.Node)
		collect = func(n *gts.Node) {
			seen[n] = true
			for j := 0; j < n.ChildCount(); j++ {
				collect(n.Child(j))
			}
		}
		collect(old.RootNode())
		var check func(*gts.Node)
		check = func(n *gts.Node) {
			if seen[n] {
				t.Fatal("new tree shares a node with old tree")
			}
			for j := 0; j < n.ChildCount(); j++ {
				check(n.Child(j))
			}
		}
		check(next.RootNode())
		fresh.Release()
		old.Release()
		tree, src = next, nextSrc
	}
	restore := gts.CcoreTestViewsOn(true, true)
	defer restore()
	nextSrc, edit := ccoreReplace(src, 2, '9')
	tree.Edit(edit)
	count := shortcutReparses()
	_, normalBefore := gts.CcoreTestIncr()
	next, err := p.ParseIncremental(nextSrc, tree)
	if err != nil || next == nil {
		t.Fatalf("shortcut off: %v", err)
	}
	defer next.Release()
	if shortcutReparses() != count {
		t.Fatal("shortcut ran while disabled")
	}
	if _, normalAfter := gts.CcoreTestIncr(); normalAfter != normalBefore+1 {
		t.Fatal("disabled shortcut did not run the normal incremental route")
	}
	fresh, err := ccoreRouteParser(lang).Parse(nextSrc)
	if err != nil || fresh == nil {
		t.Fatalf("fresh with shortcut off: %v", err)
	}
	defer fresh.Release()
	if ccoreWalkDigest(next, lang) != ccoreWalkDigest(fresh, lang) {
		t.Fatal("normal incremental result differs from fresh")
	}
}

// TestCcoreViewsDeepTree checks that 30,000 nested brackets parse, walk and convert with views on
// (the eager conversion declines a tree deeper than gts.CcoreTestMaxDepth).
func TestCcoreViewsDeepTree(t *testing.T) {
	defer ccoreCertifyAll()()
	lang := ccoreRouteLang(t, "json")
	const depth = 30000
	src := []byte(strings.Repeat("[", depth) + "1" + strings.Repeat("]", depth) + "\n")
	defer gts.CcoreTestViewsOn(true)()
	gts.CcoreTestReset()
	tree, ok := gts.CcoreTestFullParse(ccoreRouteParser(lang), src)
	if !ok || tree == nil {
		decl, _ := gts.CcoreTestCounters()
		t.Fatalf("a %d-deep tree was declined: %v", depth, decl)
	}
	defer tree.Release()
	walk := func() int {
		n, levels := tree.RootNode(), 0
		for n != nil {
			levels++
			var next *gts.Node
			for i := 0; i < n.ChildCount(); i++ {
				if c := n.Child(i); c.ChildCount() > 0 {
					next = c
					break
				}
			}
			n = next
		}
		return levels
	}
	levels := walk()
	if levels < depth {
		t.Fatalf("walked %d levels, want at least %d", levels, depth)
	}
	if !gts.CcoreTestMaterialize(tree) {
		t.Fatal("not a view tree")
	}
	if again := walk(); again != levels {
		t.Errorf("after the conversion the walk has %d levels, was %d", again, levels)
	}
	cp := tree.Copy()
	cp.Release()
}

// TestCcoreViewsRootEditWalksNothing: Tree.Edit on a routed tree with only the root view neither
// walks the tree nor synchronises the flags of its nodes (the control: an eager tree does).
// TestCcoreViewsOtherEntryPoints: Node.Edit on a view, adoption of a view into a hand-built
// tree, and a root with an offset each convert the view tree in place (counted by reason)
// and then answer as the eager tree answers.
func TestCcoreViewsOtherEntryPoints(t *testing.T) {
	defer ccoreCertifyAll()()
	lang := ccoreRouteLang(t, "json")
	src := ccoreJSONSource(40)
	parse := func() (*gts.Tree, *gts.Tree) {
		restore := gts.CcoreTestViewsOn(true)
		tree, err := ccoreRouteParser(lang).Parse(src)
		restore()
		if err != nil || tree == nil {
			t.Fatal(err)
		}
		eager := gts.CcoreTestEager(tree)
		if eager == nil {
			t.Fatal("no eager conversion")
		}
		return tree, eager
	}
	count := func(reason string) int64 { _, _, c := gts.CcoreTestViewCounters(); return c[reason] }
	t.Run("Node.Edit", func(t *testing.T) {
		tree, eager := parse()
		defer tree.Release()
		defer eager.Release()
		before := count("node_edit")
		_, edit := ccoreInsert(src, 3, ' ')
		tree.RootNode().Child(0).Edit(edit)
		eager.RootNode().Child(0).Edit(edit)
		if count("node_edit") != before+1 {
			t.Fatalf("conversions by reason: node_edit %d, want %d", count("node_edit"), before+1)
		}
		o := &ccoreOracle{t: t, lang: lang, src: src, name: "node-edit"}
		o.compare(tree.RootNode().Child(0), eager.RootNode().Child(0))
		if o.diffs != 0 {
			t.Errorf("%d differences after Node.Edit", o.diffs)
		}
	})
	t.Run("adoption", func(t *testing.T) {
		tree, eager := parse()
		defer tree.Release()
		defer eager.Release()
		before := count("adopt")
		arr := tree.RootNode().Child(0)
		kids := []*gts.Node{arr.Child(0), arr.Child(1)}
		parent := gts.NewParentNode(arr.Symbol(), true, kids, nil, 0)
		if count("adopt") != before+1 {
			t.Fatalf("conversions by reason: adopt %d, want %d", count("adopt"), before+1)
		}
		if kids[0].Parent() != parent || kids[1].Parent() != parent {
			t.Errorf("the adopted children do not report the new parent")
		}
	})
	t.Run("offset root", func(t *testing.T) {
		tree, eager := parse()
		defer tree.Release()
		defer eager.Release()
		off := gts.Point{Row: 3, Column: 2}
		before := count("offset_root")
		v, e := tree.RootNodeWithOffset(5, off), eager.RootNodeWithOffset(5, off)
		if count("offset_root") != before+1 {
			t.Fatalf("offset conversion count %d, want %d", count("offset_root"), before+1)
		}
		if _, ok := gts.CcoreTestViewHeaders(tree); !ok {
			t.Fatal("offset converted the original view tree")
		}
		o := &ccoreOracle{t: t, lang: lang, src: src, name: "offset-root"}
		if v.StartByte() != e.StartByte() || v.EndByte() != e.EndByte() || v.StartPoint() != e.StartPoint() || v.SExpr(lang) != e.SExpr(lang) {
			t.Errorf("root with offset: view %d-%d %v, eager %d-%d %v", v.StartByte(), v.EndByte(), v.StartPoint(), e.StartByte(), e.EndByte(), e.StartPoint())
		}
		_ = o
		if tree.RootNode().StartByte() != eager.RootNode().StartByte() {
			t.Errorf("the offset moved the original root")
		}
	})
}

// TestCcoreViewsNamedChildFirstOnHurl: NamedChild is the first descent into a tree whose sample
// has an anonymous visible node that holds named children. The public tree selects the named
// nodes among the visible children in sequence: one past the count is nil, and every node found
// is a child of the node it was asked from, with that node as its parent.
func TestCcoreViewsNamedChildFirstOnHurl(t *testing.T) {
	defer ccoreCertifyAll()()
	lang := ccoreRouteLang(t, "hurl")
	src := ccoreOracleSamples(t)["hurl"]
	if src == nil {
		t.Skip("no hurl sample")
	}
	restore := gts.CcoreTestViewsOn(true)
	tree, err := ccoreRouteParser(lang).Parse(src)
	restore()
	if err != nil || tree == nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if _, ok := gts.CcoreTestViewHeaders(tree); !ok {
		t.Fatal("not a view tree")
	}
	eager := gts.CcoreTestEager(tree)
	defer eager.Release()
	type pair struct{ v, e *gts.Node }
	stack := []pair{{tree.RootNode(), eager.RootNode()}}
	pastEnd, checked := 0, 0
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		count := cur.v.NamedChildCount()
		for i := 0; i <= count; i++ {
			vn, en := cur.v.NamedChild(i), cur.e.NamedChild(i)
			checked++
			if ccoreOracleTriple(vn, lang) != ccoreOracleTriple(en, lang) {
				t.Fatalf("NamedChild(%d) of %s: view %s, eager %s", i, ccoreOracleTriple(cur.v, lang), ccoreOracleTriple(vn, lang), ccoreOracleTriple(en, lang))
			}
			if i == count {
				pastEnd++
			}
			if vn == nil {
				continue
			}
			if vn.Parent() != cur.v {
				t.Fatalf("NamedChild(%d) of %s has another parent", i, ccoreOracleTriple(cur.v, lang))
			}
			stack = append(stack, pair{vn, en})
		}
	}
	// after the named walk, an ordinary descent gives the same pointers with the same parents
	for _, k := range tree.RootNode().Children() {
		if k.Parent() != tree.RootNode() {
			t.Fatal("a child made by the named walk reports another parent")
		}
	}
	t.Logf("hurl: %d NamedChild calls, %d of them one past the count", checked, pastEnd)
}

// A cancellation flag set before a short parse, and a tiny memory budget, stop a routed parse
// with views on as they do with the eager conversion: the last poll runs before the tree is
// published, with the view arena charged.
func TestCcoreViewsShortParseHonoursCancellationAndBudget(t *testing.T) {
	defer ccoreCertifyAll()()
	lang := ccoreRouteLang(t, "json")
	for _, views := range []bool{false, true} {
		restore := gts.CcoreTestViewsOn(views)
		for _, c := range []struct{ name, reason string }{{"cancelled", "stop_cancelled"}, {"budget", "stop_memory_budget"}} {
			p := ccoreRouteParser(lang)
			if c.name == "cancelled" {
				flag := uint32(1)
				p.SetCancellationFlag(&flag)
			} else {
				p.SetMemoryBudgetBytes(1)
			}
			gts.CcoreTestReset()
			tree, _ := p.Parse([]byte("1"))
			decl, served := gts.CcoreTestCounters()
			if served != 0 || decl[c.reason] != 1 {
				t.Errorf("views %t, %s: core served %d, declines %v (want %s once)", views, c.name, served, decl, c.reason)
			}
			if tree != nil {
				tree.Release()
			}
		}
		restore()
	}
}

// TestCcoreViewsConcurrentFirstTouch: readers start together on a view tree that nobody has read,
// and make the same nodes by different routes (Child, NamedChild, field lookup, a position lookup
// then Parent, siblings, a cursor, Copy). Every path gives one pointer in all goroutines, with the
// right parent, and the values equal the eager conversion. Run it under the race detector.
func TestCcoreViewsConcurrentFirstTouch(t *testing.T) {
	defer ccoreCertifyAll()()
	samples := ccoreOracleSamples(t)
	for _, name := range []string{"json", "python"} {
		src := samples[name]
		entry := grammars.DetectLanguageByName(name)
		if src == nil || entry == nil || entry.Language() == nil {
			t.Fatalf("%s: no sample or grammar", name)
		}
		lang := entry.Language()
		restore := gts.CcoreTestViewsOn(true)
		tree, err := ccoreRouteParser(lang).Parse(src)
		restore()
		if err != nil || tree == nil {
			t.Fatal(err)
		}
		if made, ok := gts.CcoreTestViewHeaders(tree); !ok || made != 1 {
			t.Fatalf("%s: setup: %d headers, view tree %t; want an untouched view tree", name, made, ok)
		}
		eager := gts.CcoreTestEager(tree)
		const readers = 8
		seen := make([]map[string]*gts.Node, readers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for g := 0; g < readers; g++ {
			seen[g] = map[string]*gts.Node{}
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				<-start
				var visit func(n *gts.Node, path string, depth int)
				visit = func(n *gts.Node, path string, depth int) {
					seen[g][path] = n
					if depth >= 6 {
						return
					}
					count := n.ChildCount()
					idxs := make([]int, count)
					for i := range idxs {
						idxs[i] = i
					}
					switch g % 4 { // the first touch differs between goroutines
					case 1:
						for i, j := 0, len(idxs)-1; i < j; i, j = i+1, j-1 {
							idxs[i], idxs[j] = idxs[j], idxs[i]
						}
					case 2:
						for f := 1; f < len(lang.FieldNames); f++ {
							n.ChildByFieldName(lang.FieldNames[f], lang)
						}
						for j := 0; j <= n.NamedChildCount(); j++ {
							n.NamedChild(j)
						}
					case 3:
						for j := n.NamedChildCount(); j >= 0; j-- {
							n.NamedChild(j)
						}
					}
					for _, i := range idxs {
						if i >= 24 {
							continue
						}
						c := n.Child(i)
						if c == nil {
							continue
						}
						if g%2 == 0 {
							_ = c.NextSibling()
							_ = c.PrevSibling()
						}
						visit(c, fmt.Sprintf("%s/%d", path, i), depth+1)
					}
				}
				switch g {
				case 6: // a position lookup, then the parent chain
					for at := 0; at < len(src); at += max(1, len(src)/40) {
						for n := tree.RootNode().DescendantForByteRange(uint32(at), uint32(at+1)); n != nil; n = n.Parent() {
						}
					}
				case 7: // a copy has its own headers
					cp := tree.Copy()
					_ = cp.RootNode().SExpr(lang)
					cp.Release()
				}
				c := gts.NewTreeCursorFromTree(tree)
				if g%3 == 0 {
					for steps := 0; steps < 2000; steps++ {
						c.CurrentNode()
						if !c.GotoFirstChild() && !c.GotoNextSibling() {
							if !c.GotoParent() {
								break
							}
						}
					}
				}
				visit(tree.RootNode(), "", 0)
			}(g)
		}
		close(start)
		wg.Wait()
		// one pointer per path in every goroutine, the right parent, the values of the eager tree
		var check func(v, e *gts.Node, path string, depth int)
		check = func(v, e *gts.Node, path string, depth int) {
			for g := 0; g < readers; g++ {
				if got, ok := seen[g][path]; !ok || got != v {
					t.Fatalf("%s %q: goroutine %d missed the path or holds another pointer", name, path, g)
				}
			}
			if ccoreOracleTriple(v, lang) != ccoreOracleTriple(e, lang) {
				t.Fatalf("%s %q: view %s, eager %s", name, path, ccoreOracleTriple(v, lang), ccoreOracleTriple(e, lang))
			}
			if depth >= 6 {
				return
			}
			for i := 0; i < v.ChildCount() && i < 24; i++ {
				vc := v.Child(i)
				if vc == nil || vc.Parent() != v {
					t.Fatalf("%s %q: child %d has the wrong parent", name, path, i)
				}
				check(vc, e.Child(i), fmt.Sprintf("%s/%d", path, i), depth+1)
			}
		}
		check(tree.RootNode(), eager.RootNode(), "", 0)
		eager.Release()
		tree.Release()
	}
}

// Every conversion is counted where it happens, whoever asks for it: the call that
// the edit paths of tree.go make (materializeNodeViews) counts as reason "other".
func TestCcoreViewsConversionIsCountedWhoeverAsks(t *testing.T) {
	defer ccoreCertifyAll()()
	lang := ccoreRouteLang(t, "json")
	restore := gts.CcoreTestViewsOn(true)
	tree, err := ccoreRouteParser(lang).Parse(ccoreJSONSource(20))
	restore()
	if err != nil || tree == nil {
		t.Fatal(err)
	}
	defer tree.Release()
	_ = tree.RootNode().Child(0) // a view below the root
	_, _, before := gts.CcoreTestViewCounters()
	gts.CcoreTestMaterializeViaBacking(tree)
	_, _, after := gts.CcoreTestViewCounters()
	if after["other"] != before["other"]+1 {
		t.Fatalf("conversions by reason: before %v, after %v; want other +1", before, after)
	}
	if _, ok := gts.CcoreTestViewHeaders(tree); ok {
		t.Fatal("the tree is still backed by views after the conversion")
	}
	gts.CcoreTestMaterializeViaBacking(tree) // already converted: nothing more is counted
	if _, _, again := gts.CcoreTestViewCounters(); again["other"] != after["other"] {
		t.Fatalf("a second call counted a conversion: %v", again)
	}
}

// A root-only view edit uses the core and returns before the public edit walk.
// An eager tree is the control for the root's dirty flag.
func TestCcoreViewsRootEditWalksNothing(t *testing.T) {
	defer ccoreCertifyIncr()()
	lang := ccoreRouteLang(t, "json")
	src := ccoreJSONSource(200)
	for _, views := range []bool{true, false} {
		restore := gts.CcoreTestViewsOn(views)
		tree, err := ccoreRouteParser(lang).Parse(src)
		restore()
		if err != nil || tree == nil || !gts.CcoreTestFromCore(tree) {
			t.Fatalf("views %t: parse: %v", views, err)
		}
		_, edit := ccoreInsert(src, 3, ' ')
		_, _, before := gts.CcoreTestViewCounters()
		tree.Edit(edit)
		_, _, after := gts.CcoreTestViewCounters()
		if views {
			if after["root_only_edits"] != before["root_only_edits"]+1 {
				t.Errorf("root-only edit was not taken: before %v after %v", before, after)
			}
			if made, ok := gts.CcoreTestViewHeaders(tree); !ok || made != 1 {
				t.Errorf("views on: %d headers after edit, view tree %t", made, ok)
			}
		} else if after["root_only_edits"] != before["root_only_edits"] {
			t.Errorf("eager control took the root-only view path")
		}
		if !tree.RootNode().HasChanges() {
			t.Errorf("views %t: root does not report the edit", views)
		}
		tree.Release()
	}
}

func TestCcoreViewsRefusedEditForcesFresh(t *testing.T) {
	defer ccoreCertifyIncr()()
	defer gts.CcoreTestViewsOn(true)()
	lang := ccoreRouteLang(t, "json")
	src := ccoreJSONSource(200)
	p := ccoreRouteParser(lang)
	tree, err := p.Parse(src)
	if err != nil || tree == nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	if _, ok := gts.CcoreTestViewHeaders(tree); !ok {
		t.Fatal("parse did not return views")
	}
	pages := gts.CcoreTestEditBudget(tree, 1)
	gts.CcoreTestEditBudget(tree, int64(pages)+1)
	next, edit := ccoreInsert(src, 3, ' ')
	tree.Edit(edit)
	if _, _, broken := gts.CcoreTestLink(tree); !broken {
		t.Fatal("refused edit did not break the core link")
	}
	gts.CcoreTestReset()
	result, err := p.ParseIncremental(next, tree)
	if err != nil || result == nil {
		t.Fatalf("reparse: %v", err)
	}
	defer result.Release()
	fallback, incremental := gts.CcoreTestIncr()
	if incremental != 0 || fallback["edits_not_carried"] != 1 {
		t.Fatalf("incremental %d fallbacks %v", incremental, fallback)
	}
	fresh, err := ccoreRouteParser(lang).Parse(next)
	if err != nil || fresh == nil {
		t.Fatalf("fresh: %v", err)
	}
	defer fresh.Release()
	if result.RootNode().SExpr(lang) != fresh.RootNode().SExpr(lang) {
		t.Fatal("reparse differs from fresh")
	}
}

func TestCcoreViewsPublishedRuntime(t *testing.T) {
	defer ccoreCertifyAll()()
	defer gts.CcoreTestViewsOn(true)()
	lang := ccoreRouteLang(t, "json")
	tree, err := ccoreRouteParser(lang).Parse([]byte(`{"x":1}`))
	if err != nil || tree == nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	if n, ok := gts.CcoreTestViewHeaders(tree); !ok || n != 1 {
		t.Fatalf("view headers: %d, %v", n, ok)
	}
	rt := tree.ParseRuntime()
	if rt.NodesAllocated != 1 || rt.ArenaBytesAllocated <= 0 {
		t.Fatalf("publication runtime: nodes=%d arena=%d", rt.NodesAllocated, rt.ArenaBytesAllocated)
	}
}

type gradPanicScanner struct {
	inner gts.ExternalScanner
	calls *atomic.Int32
}

type gradPanicPayload struct{ inner any }

func (p *gradPanicPayload) SetCFaithful(on bool) {
	if mode, ok := p.inner.(scanmode.CFaithful); ok {
		mode.SetCFaithful(on)
	}
}

func (s gradPanicScanner) Create() any   { return &gradPanicPayload{inner: s.inner.Create()} }
func (s gradPanicScanner) Destroy(p any) { s.inner.Destroy(p.(*gradPanicPayload).inner) }
func (s gradPanicScanner) Serialize(p any, b []byte) int {
	return s.inner.Serialize(p.(*gradPanicPayload).inner, b)
}
func (s gradPanicScanner) Deserialize(p any, b []byte) {
	s.inner.Deserialize(p.(*gradPanicPayload).inner, b)
}
func (s gradPanicScanner) Scan(any, *gts.ExternalLexer, []bool) bool {
	s.calls.Add(1)
	panic("graduation gate: scanner failure")
}

func gradSafetySource(t *testing.T, name string, target int) []byte {
	t.Helper()
	var sample []byte
	for _, e := range gradManifest(t) {
		if e.Language == name && e.Role == "sample" && e.Path != "" {
			b, err := os.ReadFile("internal/benchfixtures/" + e.Path)
			if err != nil {
				t.Fatal(err)
			}
			sample = b
			break
		}
	}
	if sample == nil {
		sample = []byte(grammars.ParseSmokeSample(name))
	}
	var out []byte
	for len(out) < target {
		out = append(out, sample...)
		if len(out) == 0 {
			break
		}
		if out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
	}
	return out
}

type gradManifestEntry struct {
	Language, Role string
	Path           string
}

func gradManifest(t *testing.T) []gradManifestEntry {
	t.Helper()
	raw, err := os.ReadFile("internal/benchfixtures/real_corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Entries []struct {
			Language string `json:"language"`
			Role     string `json:"role"`
			Path     string `json:"committed_path"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	var out []gradManifestEntry
	for _, e := range m.Entries {
		out = append(out, gradManifestEntry{e.Language, e.Role, e.Path})
	}
	return out
}

func gradWSEdit(src []byte, at int) ([]byte, gts.InputEdit) {
	for at < len(src)-1 && src[at] != ' ' && src[at] != '\n' {
		at++
	}
	return ccoreInsert(src, at, ' ')
}

type gradSafetyEdit struct {
	kind     string
	at       int
	old, new byte
}

// gradSafetyEdits reads the C-prepared edits and checks their format and inputs.
func gradSafetyEdits(t *testing.T, small, source []byte) (gradSafetyEdit, string, []gradSafetyEdit, string) {
	t.Helper()
	path := os.Getenv("GRAD_EDITS_FILE")
	if path == "" {
		return gradSafetyEdit{}, "no C-prepared edits", nil, "no C-prepared edits"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("item 8 edit file: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) < 3 || lines[0] != "v1" {
		t.Fatal("item 8 edit file: expected v1 header, small edit and store edits")
	}
	var smallEdit gradSafetyEdit
	var store []gradSafetyEdit
	smallReason, storeReason := "", ""
	current := append([]byte(nil), source...)
	for i, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) == 2 && ((i == 0 && fields[0] == "small-na") || (i == 1 && fields[0] == "store-na" && len(lines) == 3)) {
			if i == 0 {
				smallReason = fields[1]
			} else {
				storeReason = fields[1]
			}
			continue
		}
		if len(fields) != 4 || (i == 0 && fields[0] != "small" && fields[0] != "small-source") || (i > 0 && fields[0] != "same") {
			t.Fatalf("item 8 edit file line %d: invalid record %q", i+2, line)
		}
		at, e1 := strconv.Atoi(fields[1])
		old, e2 := strconv.ParseUint(fields[2], 10, 8)
		newByte, e3 := strconv.ParseUint(fields[3], 10, 8)
		if e1 != nil || e2 != nil || e3 != nil {
			t.Fatalf("item 8 edit file line %d: invalid number", i+2)
		}
		e := gradSafetyEdit{fields[0], at, byte(old), byte(newByte)}
		input := current
		if i == 0 && e.kind == "small" {
			input = small
		}
		if at < 0 || at >= len(input) || input[at] != e.old || (i == 0 && (e.old != '\n' || e.new != ' ')) || (i > 0 && e.old == e.new) {
			t.Fatalf("item 8 edit file line %d: edit does not match source", i+2)
		}
		if i == 0 {
			smallEdit = e
		} else {
			store = append(store, e)
			current[at] = e.new
		}
	}
	if storeReason == "" && len(store) != 300 {
		t.Fatalf("item 8 edit file: got %d store edits, want 300", len(store))
	}
	return smallEdit, smallReason, store, storeReason
}

func TestGraduationSafetyEditFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edits")
	var b strings.Builder
	b.WriteString("v1\nsmall 1 10 32\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&b, "same 0 %d %d\n", '0'+i%10, '0'+(i+1)%10)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAD_EDITS_FILE", path)
	small, smallReason, store, storeReason := gradSafetyEdits(t, []byte("x\n"), []byte("0"))
	if small.at != 1 || smallReason != "" || len(store) != 300 || storeReason != "" {
		t.Fatalf("small=%+v smallReason=%q store=%d storeReason=%q", small, smallReason, len(store), storeReason)
	}
	prepared := strings.Replace(b.String(), "small 1 ", "small-source 1 ", 1)
	if err := os.WriteFile(path, []byte(prepared), 0600); err != nil {
		t.Fatal(err)
	}
	small, smallReason, store, storeReason = gradSafetyEdits(t, nil, []byte("0\n"))
	if small.kind != "small-source" || small.at != 1 || smallReason != "" || len(store) != 300 || storeReason != "" {
		t.Fatalf("small=%+v smallReason=%q store=%d storeReason=%q", small, smallReason, len(store), storeReason)
	}
	if err := os.WriteFile(path, []byte("v1\nsmall-na no-edit\nstore-na no-source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, smallReason, store, storeReason = gradSafetyEdits(t, nil, nil)
	if smallReason != "no-edit" || storeReason != "no-source" || len(store) != 0 {
		t.Fatalf("small=%q store=%q edits=%d", smallReason, storeReason, len(store))
	}
}

func gradSafetyStatus(failed, na int) string {
	if failed > 0 {
		return "fail"
	}
	if na > 0 {
		return "na"
	}
	return "pass"
}

func TestGraduationSafetyStatus(t *testing.T) {
	fmt.Printf("GRADRUN test=%s census=%t views=%t\n", t.Name(), os.Getenv("GRAD_CENSUS") == "1", gts.CcoreTestViewsEnabled())
	for _, tc := range []struct {
		name       string
		failed, na int
		want       string
	}{
		{"all checks pass or do not apply", 0, 0, "pass"},
		{"an applicable check could not run", 0, 1, "na"},
		{"a check fails", 1, 0, "fail"},
		{"failure takes precedence", 1, 1, "fail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := gradSafetyStatus(tc.failed, tc.na); got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGraduationSafety(t *testing.T) {
	name := os.Getenv("GRAD_LANG")
	if name == "" {
		t.Skip("set GRAD_LANG")
	}
	census := os.Getenv("GRAD_CENSUS") == "1"
	if census {
		switch v := os.Getenv("CCORE_VIEWS"); v {
		case "":
		case "0", "1":
			defer gts.CcoreTestViewsOn(v == "1")()
		default:
			t.Fatalf("invalid CCORE_VIEWS=%q: want 0 or 1", v)
		}
		defer ccoreCertifyIncr()()
	} else if _, set := os.LookupEnv("CCORE_VIEWS"); set {
		t.Fatal("CCORE_VIEWS is for --report --census; the gate measures the default views switch")
	}
	views := gts.CcoreTestViewsEnabled()
	fmt.Printf("GRADRUN test=%s census=%t views=%t\n", t.Name(), census, views)
	lang := ccoreRouteLang(t, name)
	route := func(l *gts.Language) *gts.Parser {
		if census {
			return ccoreRouteParser(l)
		}
		return gts.NewParser(l)
	}
	small := gradSafetySource(t, name, 1)
	big := gradSafetySource(t, name, 1<<20)
	var preparedSource []byte
	if path := os.Getenv("GRAD_SOURCE_FILE"); path != "" {
		var err error
		preparedSource, err = os.ReadFile(path)
		if err != nil {
			t.Fatalf("item 8 source file: %v", err)
		}
	}
	smallEdit, smallReason, storeEdits, storeReason := gradSafetyEdits(t, small, preparedSource)
	ownershipInput := small
	if smallEdit.kind == "small-source" {
		ownershipInput = preparedSource
	}
	failed, na, notApplicable, ran := 0, 0, 0, 0
	first := ""
	report := func(check, status, detail string) {
		fmt.Printf("GRADSAFE lang=%s check=%s status=%s %s\n", name, check, status, detail)
		switch status {
		case "ok":
			ran++
		case "na":
			na++
			if first == "" {
				first = check + ": " + detail
			}
		case "not-applicable":
			notApplicable++
		default:
			ran++
			failed++
			if first == "" {
				first = check + ": " + detail
			}
		}
	}
	if !census {
		gts.CcoreTestReset()
		probe, err := route(lang).Parse(small)
		_, served := gts.CcoreTestCounters()
		if probe != nil {
			probe.Release()
		}
		if err != nil || probe == nil || served == 0 {
			for _, check := range []string{"cancel-before", "deadline-1us", "memory-budget", "core-stop", "core-panic", "scanner-panics", "release-orders", "readers", "store-bound"} {
				report(check, "na", "default parser did not serve the sample through the core")
			}
			build := "plain"
			if raceEnabled {
				build = "race"
			}
			fmt.Printf("GRADRESULT item=8 status=na views=%t build=%s source=unavailable bytes=0 checks_run=%d checks_failed=%d checks_na=%d checks_not_applicable=%d first=%q\n", views, build, ran, failed, na, notApplicable, first)
			return
		}
	}
	stopOf := func(p *gts.Parser, src []byte) string {
		tr, err := p.Parse(src)
		if err != nil || tr == nil {
			return "err"
		}
		defer tr.Release()
		return string(tr.ParseRuntime().StopReason)
	}
	// present-path reference: nothing certified.
	presentStop := func(set func(*gts.Parser), src []byte) string {
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(false)
		set(p)
		return stopOf(p, src)
	}

	var flag uint32 = 1
	for _, c := range []struct {
		name, decl string
		set        func(*gts.Parser)
		src        []byte
	}{
		{"cancel-before", "stop_cancelled", func(p *gts.Parser) { p.SetCancellationFlag(&flag) }, small},
		{"deadline-1us", "stop_deadline", func(p *gts.Parser) { p.SetTimeoutMicros(1) }, big},
		{"memory-budget", "stop_memory_budget", func(p *gts.Parser) { p.SetMemoryBudgetBytes(1 << 20) }, big},
	} {
		want := presentStop(c.set, c.src)
		if os.Getenv("GRAD_NEGATIVE") == "8" {
			want += "!" // corrupted expectation
		}
		rp := route(lang)
		c.set(rp)
		gts.CcoreTestReset()
		got := stopOf(rp, c.src)
		decl, served := gts.CcoreTestCounters()
		detail := fmt.Sprintf("route=%q default=%q core_served=%d declines=%v", got, want, served, decl)
		if got == want && decl[c.decl] > 0 && (served == 0 || got == string(gts.ParseStopAccepted)) {
			report(c.name, "ok", detail)
		} else {
			report(c.name, "fail", detail)
		}
	}

	// A stopped or panicking core must fall back to the default tree.
	{
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(false)
		want, err := p.Parse(small)
		if err != nil || want == nil {
			t.Fatalf("item 8 input=%s default parse error=%v", name, err)
		}
		wantDigest := ccoreWalkDigest(want, lang)
		want.Release()
		for _, tc := range []struct {
			name, decline string
			panic         bool
		}{{"core-stop", "core_stop", false}, {"core-panic", "panic", true}} {
			gts.CcoreTestReset()
			restore := gts.CcoreTestStopAfter(1, tc.panic)
			got, err := route(lang).Parse(small)
			restore()
			decl, served := gts.CcoreTestCounters()
			equal := got != nil && ccoreWalkDigest(got, lang) == wantDigest
			if got != nil {
				got.Release()
			}
			detail := fmt.Sprintf("input=%s equal=%t served=%d decline_%s=%d error=%v", name, equal, served, tc.decline, decl[tc.decline], err)
			if err == nil && equal && served == 0 && decl[tc.decline] > 0 {
				report(tc.name, "ok", detail)
			} else {
				report(tc.name, "fail", detail)
			}
		}
	}
	// A scanner that panics must be reached by the core before fallback.
	if lang.ExternalScanner == nil {
		report("scanner-panics", "not-applicable", "the grammar has no external scanner")
	} else {
		// The check wraps the scanner. That changes the language's configuration, so
		// the shipped certificate cannot cover the wrapped language. The check therefore
		// runs under the test certification in every kind of run and selects the route
		// on its parser: what it shows (the core reaches a scanner that panics, declines,
		// and the answer is the present engine's) does not depend on which certificate
		// switched the route on.
		restoreCertificate := func() {}
		if !census {
			restoreCertificate = ccoreCertifyIncr()
		}
		outcome := func(candidate bool) (result string, calls int32, declines map[string]int64) {
			l := ccoreCopyLanguage(lang)
			var n atomic.Int32
			l.ExternalScanner = ccoreWrapped(lang, func(inner gts.ExternalScanner) gts.ExternalScanner {
				return gradPanicScanner{inner: inner, calls: &n}
			})
			p := gts.NewParser(l)
			p.SetAdmissionCandidateRoute(candidate)
			gts.CcoreTestReset()
			func() {
				defer func() {
					if recover() != nil {
						result = "panic"
					}
				}()
				tr, err := p.Parse(small)
				if err != nil {
					result = "error"
					return
				}
				if tr == nil {
					result = "nil"
					return
				}
				tr.Release()
				result = "tree"
			}()
			declines, _ = gts.CcoreTestCounters()
			return result, n.Load(), declines
		}
		routed, routeCalls, declines := outcome(true)
		present, presentCalls, _ := outcome(false)
		restoreCertificate()
		detail := fmt.Sprintf("input=%s route=%s default=%s route_scans=%d default_scans=%d declines=%v", name, routed, present, routeCalls, presentCalls, declines)
		switch {
		case routeCalls == 0 || presentCalls == 0 || declines["panic"] == 0:
			report("scanner-panics", "na", "scanner panic was not observed in both paths and as a core decline: "+detail)
		case routed == present:
			report("scanner-panics", "ok", detail)
		default:
			report("scanner-panics", "fail", detail)
		}
	}
	// Every order of copy, edit, reparse and release.
	var next []byte
	var edit gts.InputEdit
	if smallReason != "" {
		report("release-orders", "na", smallReason)
	} else {
		next, edit = ccoreInsert(ownershipInput, smallEdit.at, smallEdit.new)
		gts.CcoreTestReset()
		ft, _ := route(lang).Parse(next)
		wantNext := ccoreSExpr(ft, lang)
		ft.Release()
		n, reparses, copies, bad := 0, 0, 0, ""
		var rec func(a []string, k int)
		rec = func(a []string, k int) {
			if bad != "" {
				return
			}
			if k < len(a) {
				for i := k; i < len(a); i++ {
					a[k], a[i] = a[i], a[k]
					rec(a, k+1)
					a[k], a[i] = a[i], a[k]
				}
				return
			}
			n++
			p := route(lang)
			tree, err := p.Parse(ownershipInput)
			if err != nil {
				bad = err.Error()
				return
			}
			var cp, re *gts.Tree
			edited, released := false, false
			for _, s := range a {
				switch s {
				case "copy":
					if !released {
						cp = tree.Copy()
					}
				case "edit":
					if !released {
						tree.Edit(edit)
						edited = true
					}
				case "reparse":
					if edited && !released {
						if re, err = p.ParseIncremental(next, tree); err != nil {
							bad = err.Error()
							return
						}
					}
				case "release":
					tree.Release()
					released = true
				}
			}
			if re != nil {
				reparses++
				if ccoreSExpr(re, lang) != wantNext {
					bad = fmt.Sprintf("order %v: reparse differs from fresh", a)
				}
				re.Release()
			}
			if cp != nil {
				copies++
				if cp.RootNode() == nil {
					bad = fmt.Sprintf("order %v: the copy lost its tree", a)
				}
				cp.Release()
			}
			tree.Release()
		}
		rec([]string{"copy", "edit", "reparse", "release"}, 0)
		_, freshServed := gts.CcoreTestCounters()
		_, incrementalServed := gts.CcoreTestIncr()
		detail := fmt.Sprintf("orders=%d reparses=%d copies=%d fresh_served=%d incremental_served=%d %s", n, reparses, copies, freshServed, incrementalServed, bad)
		if bad == "" && n == 24 && reparses > 0 && copies > 0 && freshServed > 0 && incrementalServed > 0 {
			report("release-orders", "ok", detail)
		} else {
			report("release-orders", "fail", detail)
		}
	}

	// A reader walks a tree while two copies of it are edited and reparsed (meaningful under -race).
	{
		if smallReason != "" {
			report("readers", "na", smallReason)
		} else {
			gts.CcoreTestReset()
			p0 := route(lang)
			tree, err := p0.Parse(ownershipInput)
			if err != nil {
				report("readers", "fail", err.Error())
			} else {
				copies := []*gts.Tree{tree.Copy(), tree.Copy()}
				var wg sync.WaitGroup
				var bad, reads atomic.Int32
				stop := make(chan struct{})
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-stop:
							return
						default:
						}
						var walk func(n *gts.Node)
						walk = func(n *gts.Node) {
							_ = n.Type(lang)
							for i := 0; i < n.ChildCount(); i++ {
								walk(n.Child(i))
							}
						}
						walk(tree.RootNode())
						reads.Add(1)
					}
				}()
				var workers sync.WaitGroup
				for _, cp := range copies {
					workers.Add(1)
					go func(cp *gts.Tree) {
						defer workers.Done()
						p := route(lang)
						for i := 0; i < 30; i++ {
							cur, e := next, edit
							if i%2 == 1 {
								cur = small
								e = gts.InputEdit{StartByte: edit.StartByte, OldEndByte: edit.NewEndByte, NewEndByte: edit.OldEndByte, StartPoint: edit.StartPoint, OldEndPoint: edit.NewEndPoint, NewEndPoint: edit.OldEndPoint}
							}
							cp.Edit(e)
							nx, err := p.ParseIncremental(cur, cp)
							if err != nil || nx == nil {
								bad.Add(1)
								return
							}
							if nx != cp {
								cp.Release()
							}
							cp = nx
						}
						cp.Release()
					}(cp)
				}
				workers.Wait()
				close(stop)
				wg.Wait()
				tree.Release()
				_, freshServed := gts.CcoreTestCounters()
				_, incrementalServed := gts.CcoreTestIncr()
				if bad.Load() == 0 && reads.Load() > 0 && freshServed > 0 && incrementalServed > 0 {
					report("readers", "ok", fmt.Sprintf("reader_walks=%d fresh_served=%d incremental_served=%d", reads.Load(), freshServed, incrementalServed))
				} else {
					report("readers", "fail", fmt.Sprintf("failed reparses=%d reader_walks=%d", bad.Load(), reads.Load()))
				}
			}
		}
	}

	// Store bound after many edits: the store belongs to the incremental half of the route.
	storeSourceName, storeSourceBytes := "gradSafetySource", 0
	{
		source := gradSafetySource(t, name, 32<<10)
		switch name {
		case "json", "python", "c", "typescript":
			source = ccoreGateSource(name, 32<<10)
			storeSourceName = "ccoreGateSource"
		}
		if os.Getenv("GRAD_SOURCE_REQUIRED") == "1" {
			storeSourceName = os.Getenv("GRAD_SOURCE_NAME")
			var err error
			source, err = os.ReadFile(os.Getenv("GRAD_SOURCE_FILE"))
			if err != nil {
				source = nil
			}
		}
		storeSourceBytes = len(source)
		if storeReason != "" {
			report("store-bound", "na", storeReason)
		} else {
			p := route(lang)
			tree, err := p.Parse(source)
			if err != nil || tree == nil {
				report("store-bound", "fail", fmt.Sprintf("initial parse error=%v", err))
			} else if tree.RootNode() == nil || tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
				tree.Release()
				report("store-bound", "fail", "initial parse is not clean and complete")
			} else {
				// While a reparse runs, the old tree and the new one both live in the store,
				// so the store can reach about twice the first tree when a reparse rebuilds
				// most of it (a grammar whose scanner state changes after an edit does).
				// That is a fixed cost. What must not happen is growth with the number of
				// edits. The check therefore has two parts: the size after 100 edits is the
				// size the session settles at (later edits add at most 5 percent), and it
				// stays under 2.5 times the first tree.
				firstBytes, maxBytes := gts.CcoreTestStoreBytes(tree), 0
				maxBytes = firstBytes
				settledBytes := 0
				cur, edits, served, rejected, problem := source, 0, 0, 0, ""
				gts.CcoreTestReset()
				for edits < 300 {
					prepared := storeEdits[edits]
					var generator benchfixtures.CleanEditGenerator
					step, proposalErr := generator.Apply(cur, benchfixtures.CleanCandidate{Kind: "same", At: prepared.at})
					if proposalErr != nil || step.Source[prepared.at] != prepared.new {
						t.Fatalf("item 8 edit %d: invalid replacement", edits)
					}
					next, edit := step.Source, step.Edit
					tree.Edit(edit)
					_, before := gts.CcoreTestIncr()
					nt, err := p.ParseIncremental(next, tree)
					_, after := gts.CcoreTestIncr()
					if err != nil || nt == nil {
						problem = fmt.Sprintf("reparse error=%v", err)
						break
					}
					served += int(after - before)
					if tree != nt {
						tree.Release()
					}
					tree, cur = nt, next
					edits++
					if tree.RootNode() == nil || tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(cur)) {
						problem = "incremental parse is not clean and complete"
						break
					}
					if n := gts.CcoreTestStoreBytes(tree); n > maxBytes {
						maxBytes = n
					}
					if edits == 100 {
						settledBytes = maxBytes
					}
				}
				lastBytes := gts.CcoreTestStoreBytes(tree)
				tree.Release()
				fallbacks, _ := gts.CcoreTestIncr()
				detail := fmt.Sprintf("source_bytes=%d edits=%d served=%d first_bytes=%d after_100_edits=%d max_bytes=%d last_bytes=%d fallbacks=%v rejected=%d %s", len(source), edits, served, firstBytes, settledBytes, maxBytes, lastBytes, fallbacks, rejected, problem)
				settled := settledBytes > 0 && float64(maxBytes) <= 1.05*float64(settledBytes)
				if problem == "" && firstBytes > 0 && edits == 300 && served == 300 && len(fallbacks) == 0 && settled && float64(maxBytes) <= 2.5*float64(firstBytes) {
					report("store-bound", "ok", detail)
				} else {
					report("store-bound", "fail", detail)
				}
			}
		}
	}
	status := gradSafetyStatus(failed, na)
	race := "plain"
	if raceEnabled {
		race = "race"
	}
	fmt.Printf("GRADRESULT item=8 status=%s views=%t build=%s source=%q bytes=%d checks_run=%d checks_failed=%d checks_na=%d checks_not_applicable=%d first=%q\n", status, views, race, storeSourceName, storeSourceBytes, ran, failed, na, notApplicable, first)
	if failed > 0 {
		t.Fail()
	}
}
