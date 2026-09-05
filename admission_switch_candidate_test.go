//go:build gts_parsercorephase0

package gotreesitter

import (
	"strconv"
	"testing"
)

// requireCandidateRouteBudgetMB arms GOT_PARSE_MEMORY_BUDGET_MB at mb for the
// duration of the calling test (t.Setenv restores the ambient value
// automatically), so a prior test's environment change or the process
// environment can never silently leave a routing test under-budgeted. mb
// must be comfortably above what the scheduler's stop-control poll needs to
// see this fixture through to acceptance (Core.FootprintBytes() against the
// configured budget, tranche B9 honest-accounting gate) -- grammargen_lr
// specifically measured routing at 48 MB and declining at 32 MB on the
// development host, so callers here use several times that for real
// headroom against host and grammar drift.
func requireCandidateRouteBudgetMB(t testing.TB, mb int) {
	t.Helper()
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", strconv.Itoa(mb))
	ResetParseEnvConfigCacheForTests()
	t.Cleanup(ResetParseEnvConfigCacheForTests)
}

// These tests run under the gts_parsercorephase0 tag, where the compact
// candidate route is compiled in. They prove the switch actually routes a fresh
// full parse through the compact route, that the routed tree is byte-exact with
// production on the four canonical fixtures, and that incremental reuse is
// available only when a routed tree carries the replay/scanner proof.

func newAdmissionCandidateGoParser(t testing.TB) *Parser {
	t.Helper()
	lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
	if err != nil {
		t.Fatalf("authenticate certified Go language: %v", err)
	}
	return NewParser(lang)
}

// TestAdmissionSwitchParseRoutesCandidateWhenOn proves Parse serves the compact
// candidate route for every canonical fixture when the switch is on,
// including grammargen_lr (235,626 bytes): tranche B9 removed the
// source-length eligibility decline that used to keep it on production, so
// every canonical fixture now routes on size alone. Large-input safety comes
// from the tranche B8 scheduler stop-control poll instead.
func TestAdmissionSwitchParseRoutesCandidateWhenOn(t *testing.T) {
	requireCandidateRouteBudgetMB(t, 256)
	resetAdmissionCandidateCounters()
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(true)
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
		routed0, fb0 := AdmissionCandidateCounters()
		tree, err := p.Parse(fixture.Source)
		if err != nil {
			t.Fatalf("%s: parse: %v", row.id, err)
		}
		routed1, fb1 := AdmissionCandidateCounters()
		if routed1 != routed0+1 || fb1 != fb0 {
			t.Fatalf("%s (%d bytes): expected one candidate route, got routed %d->%d fallback %d->%d (last=%q)",
				row.id, len(fixture.Source), routed0, routed1, fb0, fb1, AdmissionCandidateLastFallbackReason())
		}
		if !tree.compactMaterialized {
			t.Fatalf("%s: routed tree is not compact-materialized", row.id)
		}
		if !tree.incrementalReuseDisabled {
			requireCompactIncrementalStateProof(t, tree)
		}
		tree.Release()
	}
}

// TestAdmissionSwitchGrammargenLRRoutesCompactByDefault proves grammargen_lr
// (235,626 bytes) routes through the compact candidate on the shipped API: no
// per-Parser SetAdmissionCandidateRoute pin, no diagnostic build flag, just
// the process-wide default the campaign shipped ON. Before tranche B9 this
// fixture was permanently ineligible (it sits above the retired 64 KiB size
// floor); today source length no longer decides eligibility, so the shipped
// default routes it exactly like any other fixture.
func TestAdmissionSwitchGrammargenLRRoutesCompactByDefault(t *testing.T) {
	requireCandidateRouteBudgetMB(t, 256)
	previousDefault := AdmissionCandidateRouteDefault()
	t.Cleanup(func() { SetAdmissionCandidateRouteDefault(previousDefault) })
	SetAdmissionCandidateRouteDefault(true)
	resetAdmissionCandidateCounters()

	p := newAdmissionCandidateGoParser(t) // no SetAdmissionCandidateRoute call: follows the process default
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "grammargen_lr")
	if len(fixture.Source) < 64*1024 {
		t.Fatalf("grammargen_lr fixture is %d bytes, want it to exceed the retired 64 KiB floor", len(fixture.Source))
	}
	routed0, fb0 := AdmissionCandidateCounters()
	tree, err := p.Parse(fixture.Source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	routed1, fb1 := AdmissionCandidateCounters()
	if routed1 != routed0+1 || fb1 != fb0 {
		t.Fatalf("grammargen_lr (%d bytes) did not route on the shipped default: routed %d->%d fallback %d->%d (last=%q)",
			len(fixture.Source), routed0, routed1, fb0, fb1, AdmissionCandidateLastFallbackReason())
	}
	if !tree.compactMaterialized {
		t.Fatal("grammargen_lr routed tree is not compact-materialized")
	}
	digest := requireDiagnosticParserCoreCanonicalTreeDigest(t, tree, p.language)
	if digest != fixture.DeepTreeSHA256 {
		t.Fatalf("grammargen_lr shipped-route digest=%s want=%s", digest, fixture.DeepTreeSHA256)
	}
}

// TestAdmissionSwitchParseProductionWhenOff proves a forced-off Parser stays on
// production and never touches the counters, independent of the process default
// (this test forces off so it is robust to GTS_ADMISSION_CANDIDATE).
func TestAdmissionSwitchParseProductionWhenOff(t *testing.T) {
	resetAdmissionCandidateCounters()
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(false) // forced off wins over any default/env
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	routed0, fb0 := AdmissionCandidateCounters()
	tree, err := p.Parse(fixture.Source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	routed1, fb1 := AdmissionCandidateCounters()
	if routed1 != routed0 || fb1 != fb0 {
		t.Fatalf("switch off must not move counters: routed %d->%d fallback %d->%d", routed0, routed1, fb0, fb1)
	}
	if tree.compactMaterialized {
		t.Fatal("switch off produced a compact-materialized tree")
	}
}

// TestAdmissionSwitchCandidateDigestMatchesProduction is the byte-exact fidelity
// proof: the compact candidate ENGINE and production produce the same frozen
// deep-tree digest on all four canonical fixtures.
//
// It drives the candidate tree through the engine seam (tryCompactFullParseRoute)
// rather than the public Parse route, independent of routing: the engine
// byte-exactness this proves holds regardless of which admission path a
// caller takes to reach it. TestAdmissionSwitchParseRoutesCandidateWhenOn
// separately proves the public Parse route reaches this same engine for every
// canonical fixture, grammargen_lr included (tranche B9).
func TestAdmissionSwitchCandidateDigestMatchesProduction(t *testing.T) {
	lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
	if err != nil {
		t.Fatalf("authenticate certified Go language: %v", err)
	}
	candidate := NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	production := NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
		candidateTree, ok, reason := candidate.tryCompactFullParseRoute(fixture.Source)
		if !ok || candidateTree == nil {
			t.Fatalf("%s: candidate engine declined: %s", row.id, reason)
		}
		productionTree, err := production.Parse(fixture.Source)
		if err != nil {
			t.Fatalf("%s: production parse: %v", row.id, err)
		}
		candidateDigest := requireDiagnosticParserCoreCanonicalTreeDigest(t, candidateTree, lang)
		productionDigest := requireDiagnosticParserCoreCanonicalTreeDigest(t, productionTree, lang)
		if candidateDigest != row.deepTreeSHA256 {
			t.Fatalf("%s: candidate digest=%s want=%s", row.id, candidateDigest, row.deepTreeSHA256)
		}
		if productionDigest != candidateDigest {
			t.Fatalf("%s: production digest=%s != candidate digest=%s", row.id, productionDigest, candidateDigest)
		}
		if !candidateTree.compactMaterialized {
			t.Fatalf("%s: candidate tree is not compact-materialized", row.id)
		}
		candidateTree.Release()
		productionTree.Release()
	}
}

// TestAdmissionSwitchEligibilityFailsClosedForReuse proves the centralized
// routing policy the three fresh-full-parse entry points share: only a fresh
// DFA parse is eligible; every reuse-consuming path and every non-DFA lexer is
// barred, even when the switch is forced on.
func TestAdmissionSwitchEligibilityFailsClosedForReuse(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(true)
	if !p.admissionCandidateFullParseEligible(nil, true) {
		t.Fatal("a fresh DFA parse must be eligible when the switch is on")
	}
	reuse := &Tree{}
	if p.admissionCandidateFullParseEligible(reuse, true) {
		t.Fatal("a reuse-consuming path (oldTree != nil) must never be eligible")
	}
	if p.admissionCandidateFullParseEligible(nil, false) {
		t.Fatal("a caller-supplied token source must never be eligible")
	}
	p.SetAdmissionCandidateRoute(false)
	if p.admissionCandidateFullParseEligible(nil, true) {
		t.Fatal("a forced-off Parser must not be eligible")
	}
}

// Incremental attempts must not change counters reserved for full parsing.
func TestAdmissionSwitchParseIncrementalDoesNotCountFullRoute(t *testing.T) {
	resetAdmissionCandidateCounters()
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(true)
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	oldTree, err := p.Parse(fixture.Source)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer oldTree.Release()
	routedAfterFresh, _ := AdmissionCandidateCounters()

	// Append a single space at EOF: a clean, minimal edit.
	edited := append(append([]byte(nil), fixture.Source...), ' ')
	eofPoint := admissionTestPointAtByte(fixture.Source, len(fixture.Source))
	edit := InputEdit{
		StartByte:   uint32(len(fixture.Source)),
		OldEndByte:  uint32(len(fixture.Source)),
		NewEndByte:  uint32(len(fixture.Source) + 1),
		StartPoint:  eofPoint,
		OldEndPoint: eofPoint,
		NewEndPoint: Point{Row: eofPoint.Row, Column: eofPoint.Column + 1},
	}
	oldTree.Edit(edit)
	newTree, err := p.ParseIncremental(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	if newTree != nil && newTree != oldTree {
		defer newTree.Release()
	}
	routedAfterIncremental, _ := AdmissionCandidateCounters()
	if routedAfterIncremental != routedAfterFresh {
		t.Fatalf("ParseIncremental changed full-route counts: %d -> %d", routedAfterFresh, routedAfterIncremental)
	}
	if newTree != nil && newTree.compactMaterialized && !newTree.rawParseRuntime().CompactIncrementalReuseRoute {
		t.Fatal("ParseIncremental published a compact tree without incremental execution")
	}
}

// TestAdmissionSwitchRecoveryDoesNotLeakCompactTree drives malformed input
// through the real compact route with the switch forced on. The compact route
// declines the malformed parse and production recovery runs, so the returned
// tree must be a production (non-compact) tree, and the pinned recovery
// sub-parser must never publish a compact fragment.
func TestAdmissionSwitchRecoveryDoesNotLeakCompactTree(t *testing.T) {
	lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
	if err != nil {
		t.Fatalf("authenticate certified Go language: %v", err)
	}
	p := NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	malformed := []byte("package main\n\nfunc () {\n\tvar x = \n}\n")
	tree, err := p.Parse(malformed)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	if tree.compactMaterialized {
		t.Fatal("malformed/recovery parse leaked a compact-materialized tree")
	}
	if root := tree.RootNode(); root == nil || !root.HasError() {
		t.Fatal("malformed input should recover into an error-bearing production tree")
	}
}

// admissionTestPointAtByte returns the row/column point for a byte offset.
func admissionTestPointAtByte(source []byte, offset int) Point {
	var row, col uint32
	if offset > len(source) {
		offset = len(source)
	}
	for i := 0; i < offset; i++ {
		if source[i] == '\n' {
			row++
			col = 0
			continue
		}
		col++
	}
	return Point{Row: row, Column: col}
}
