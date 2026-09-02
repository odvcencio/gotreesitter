//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// D2-1 slice: span-unlocked per-header relex. relexTokenForState used to
// reject any per-header relex whose
// span (StartByte and EndByte) did not exactly match the shared election's
// span. This slice unlocks EndByte: a same-start relex may now return a
// wider or narrower token than the shared election. The scheduler now
// activates owned per-header lexer requests for that shape.
//
// bashRaggedEndWitnessSource ("A(1%") is a real, minimal witness found by
// targeted fuzzing of FuzzAdmissionRouteEquality's own bash lane (not
// hand-crafted): at byte 2, the shared election is a 1-byte token (symbol
// 160, span 2..3); the no-action header's own state (7338) relexes the
// same start byte to a 2-byte token (symbol 1, span 2..4). Reproduced
// deterministically via RelexTokenForStateForTest below. This witness's
// own no-action head is drop-eligible on every recurrence during a full
// scheduler run (a sibling header keeps advancing the same pass), so it
// does not reach the scheduler-level ownership activation itself; see
// swiftRaggedEndWitnessSource below for that.
var bashRaggedEndWitnessSource = []byte("A(1%")

// swiftRaggedEndWitnessSource is the scheduler-level witness
// for D2-1 different-span ownership activation (F1). At byte
// 4, swift's shared election is a 2-byte external-scanner token (symbol
// 217, span 4..6, the "<#" raw-string-literal opener); a sibling header's
// own state (47) relexes the same start byte to a narrower 1-byte token
// (symbol 35, span 4..5, a plain "<" operator). Unlike
// bashRaggedEndWitnessSource, every live header here is a no-action head
// and at least one is ragged, so this shape is not drop-eligible: the
// scheduler activates owned lexer requests before a sibling can reduce.
var swiftRaggedEndWitnessSource = []byte("a<(A<#/x/#)")

// TestD2SpanUnlockedRelexTokenForStateFindsRaggedEndOnRealBashWitness pins
// the probe's own contract (Phase 1 item 2) against the real witness above.
// Enabled (the zero-value default), the probe returns the wider token: it
// starts at the same byte as the shared election but ends two bytes later,
// with a different symbol -- exactly the "may differ in Symbol and/or
// EndByte/EndPoint" contract. Disabled
// (DisablePerHeaderSpanUnlockedRelex), the probe restores the pre-D2-1
// span-locked reject: an EndByte mismatch declines the same way a scan
// failure does, regardless of Symbol.
func TestD2SpanUnlockedRelexTokenForStateFindsRaggedEndOnRealBashWitness(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("bash")
	if entry == nil {
		t.Fatal("bash is absent from the language registry")
	}
	lang := entry.Language()

	shared := gts.Token{
		Symbol:                   160,
		Text:                     "1",
		StartByte:                2,
		EndByte:                  3,
		StartPoint:               gts.Point{Column: 2},
		EndPoint:                 gts.Point{Column: 3},
		ExternalScannerToken:     true,
		ExternalScannerStartByte: 2,
	}
	const raggedState = gts.StateID(7338)

	relexed, ok := gts.RelexTokenForStateForTest(
		lang, bashRaggedEndWitnessSource, 1, gts.DiagnosticParserCorePrefixOptions{}, raggedState, shared,
	)
	if !ok {
		t.Fatalf("span-unlocked probe declined the real witness; want a wider token")
	}
	if relexed.StartByte != shared.StartByte {
		t.Fatalf("relexed StartByte = %d, want %d (same start as the shared election)", relexed.StartByte, shared.StartByte)
	}
	if relexed.EndByte != 4 || relexed.Symbol != 1 {
		t.Fatalf("relexed token = %+v, want a wider token (symbol=1, end=4)", relexed)
	}

	relexedDisabled, okDisabled := gts.RelexTokenForStateForTest(
		lang, bashRaggedEndWitnessSource, 1,
		gts.DiagnosticParserCorePrefixOptions{DisablePerHeaderSpanUnlockedRelex: true},
		raggedState, shared,
	)
	if okDisabled {
		t.Fatalf("disabled probe accepted a ragged-end relex = %+v, want span-locked decline", relexedDisabled)
	}
	if relexedDisabled != shared {
		t.Fatalf("disabled probe token = %+v, want the unchanged shared token %+v", relexedDisabled, shared)
	}
}

// TestD2SpanUnlockedRaggedRelexDeclineDetailCarriesWiderTokenSpan pins the
// legacy detail format for telemetry migrations.
func TestD2SpanUnlockedRaggedRelexDeclineDetailCarriesWiderTokenSpan(t *testing.T) {
	shared := gts.Token{Symbol: 160, StartByte: 2, EndByte: 3}
	relexed := gts.Token{Symbol: 1, StartByte: 2, EndByte: 4}

	detail := gts.DiagnosticParserCoreRaggedRelexDeclineDetailFormatForTest(relexed, shared)
	wantPrefix := gts.DiagnosticParserCoreRaggedRelexDeclineDetailForTest()
	if !strings.HasPrefix(detail, wantPrefix) {
		t.Fatalf("decline detail = %q, want prefix %q", detail, wantPrefix)
	}
	// F14: assert the full detail string, not an ambiguous substring set --
	// "span=2..3" alone cannot tell the relexed token's span from the shared
	// token's span apart if their formats ever collide.
	want := wantPrefix + ": relexed symbol=1 span=2..4 shared span=2..3"
	if detail != want {
		t.Fatalf("decline detail = %q, want %q", detail, want)
	}
}

// TestD2SpanUnlockedSchedulerActivatesOwnedLexerRequests proves that the
// Swift witness preempts the shared pass and publishes both lexer views.
func TestD2SpanUnlockedSchedulerActivatesOwnedLexerRequests(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("swift")
	if entry == nil {
		t.Fatal("swift is absent from the language registry")
	}
	lang := entry.Language()

	receipt, err := gts.RunStateDependentRelexSchedulerForTest(lang, swiftRaggedEndWitnessSource)
	if err != nil {
		t.Fatalf("compact scheduler: %v", err)
	}
	if receipt.Acceptance == nil {
		t.Fatalf("compact scheduler did not accept the ownership witness: stop=%+v", receipt.Stop)
	}
	if receipt.PerVersionLexRequests != 4 || receipt.PerVersionLexRestores != 4 ||
		receipt.PerVersionLexPublications != 14 || receipt.PerVersionLexAcceptedRaggedSpans != 3 ||
		receipt.PerVersionLexViabilityDrops != 1 {
		t.Fatalf("owned lexer counters=%d/%d/%d/%d/%d, want 4/4/14/3/1",
			receipt.PerVersionLexRequests, receipt.PerVersionLexRestores,
			receipt.PerVersionLexPublications, receipt.PerVersionLexAcceptedRaggedSpans,
			receipt.PerVersionLexViabilityDrops)
	}
	want := map[gts.StateID]struct {
		symbol  gts.Symbol
		endByte uint32
	}{
		441:  {symbol: 35, endByte: 5},
		1554: {symbol: 35, endByte: 5},
	}
	if len(receipt.VersionLexerRequests) < len(want) {
		t.Fatalf("owned lexer requests=%d, want at least %d", len(receipt.VersionLexerRequests), len(want))
	}
	for _, request := range receipt.VersionLexerRequests {
		expect, ok := want[request.State]
		if !ok {
			continue
		}
		if request.Token.Symbol != expect.symbol || request.Token.StartByte != 4 || request.Token.EndByte != expect.endByte {
			t.Fatalf("state %d token=%+v, want symbol=%d span=4..%d", request.State, request.Token, expect.symbol, expect.endByte)
		}
	}
}

// TestD2SpanUnlockedSameSpanRelexOnRealJavaScriptDivisionRegexWitness is
// the D2-1 Phase 2 item 1b anchor's CI-covered counterpart (F3): a real,
// always-embedded grammar's own state-dependent lexing produces a genuine
// same-span, different-symbol relex, so this test never needs the
// git-ignored, CI-absent cgo_harness/corpus_real directory the sibling
// test below is gated behind.
//
// javascript's own division-vs-regex-literal ambiguity is the real shape:
// in "var y = a/2/g;", the "/" right after the identifier "a" is division
// under one lex state (symbol 87) and the open of a regex literal under
// another (symbol 111) -- both exactly one byte here (span 59..60), a
// genuine per-state disagreement on the very same span the shared
// election's own state produced. relexTokenForState finds the
// division-state's own token first (mirroring what a real shared election
// would report), then the regex-state's own relex at that identical span;
// diagnosticParserCoreSameSpanRelex accepts the pair and returns the
// promoted (symbol 111) token. This pins the same-span accept contract
// that the shift path depends on; no CI test drives the shift itself.
func TestD2SpanUnlockedSameSpanRelexOnRealJavaScriptDivisionRegexWitness(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("javascript")
	if entry == nil {
		t.Fatal("javascript is absent from the language registry")
	}
	lang := entry.Language()
	source := []byte("let x = {}; function f(){ return `a${1}b${2}c`; } var y = a/2/g;")
	const (
		divisionState    = gts.StateID(1)
		regexOpenState   = gts.StateID(1770)
		slashStartByte   = 59
		slashEndByte     = 60
		divisionSymbol   = gts.Symbol(87)
		regexOpenSymbol  = gts.Symbol(111)
		placeholderProbe = gts.Symbol(9999) // matches no real symbol; avoids relexTokenForState's true-no-op short circuit
	)
	probeTok := gts.Token{
		Symbol: placeholderProbe, StartByte: slashStartByte, EndByte: slashEndByte,
		StartPoint: gts.Point{Column: slashStartByte}, EndPoint: gts.Point{Column: slashEndByte},
	}

	shared, ok := gts.RelexTokenForStateForTest(lang, source, 1, gts.DiagnosticParserCorePrefixOptions{}, divisionState, probeTok)
	if !ok || shared.Symbol != divisionSymbol {
		t.Fatalf("division-state relex = %+v/%t, want symbol=%d/true (the shared election's own division token)", shared, ok, divisionSymbol)
	}

	relexed, ok := gts.RelexTokenForStateForTest(lang, source, 1, gts.DiagnosticParserCorePrefixOptions{}, regexOpenState, shared)
	if !ok {
		t.Fatalf("regex-open-state relex declined the shared division token %+v; want a genuine same-span relex", shared)
	}
	if relexed.StartByte != shared.StartByte || relexed.EndByte != shared.EndByte {
		t.Fatalf("relexed span = %d..%d, want the shared election's own span %d..%d", relexed.StartByte, relexed.EndByte, shared.StartByte, shared.EndByte)
	}
	if relexed.Symbol != regexOpenSymbol {
		t.Fatalf("relexed symbol = %d, want %d (the regex-literal-open token)", relexed.Symbol, regexOpenSymbol)
	}

	verified, ok := gts.DiagnosticParserCoreSameSpanRelexForTest(shared, relexed)
	if !ok {
		t.Fatalf("diagnosticParserCoreSameSpanRelex declined a genuine same-span relex: shared=%+v relexed=%+v", shared, relexed)
	}
	if verified.Symbol != regexOpenSymbol {
		t.Fatalf("verified symbol = %d, want %d", verified.Symbol, regexOpenSymbol)
	}
}

// TestD2SpanUnlockedSameSpanRelexStillShiftsRealBashInstallWitness is
// the D2-1 Phase 2 item 1b anchor: a same-span, different-symbol relex must
// keep today's behavior exactly (it still shifts via the cell token, same
// as before this slice). The designated scala W2 anchor
// (TestDiagnosticParserCoreStateDependentRelexKeepsExactSpanBranch,
// parsercore_phase0_state_relex_test.go) is broken pre-existing on this
// branch, independent of D2-1 (confirmed against origin/main 8fbb9538 by
// git stash: both its raw scheduler call and its certified admission-
// scorecard call already fail there, for an unrelated G18/D6 alternative-
// set convergence reason -- see the D2-1 report). This test substitutes a
// real, unmodified grammar fixture instead: tree-sitter-bash's own
// examples/install.sh (when the directory is present locally and
// GTS_ADMISSION_REAL_CORPUS=1 enables the test, at
// cgo_harness/corpus_real/bash/medium__install.sh --
// see the git-ignored note below; it is never committed), confirmed by direct
// code-path instrumentation during evidence gathering to exercise a
// successful same-span relex mid-parse.
//
// NoActionDrops is the discriminating signal, verified directly: forcing
// diagnosticParserCoreSameSpanRelex to always decline changes this exact
// fixture's NoActionDrops from 16 to 17 (one more header falls back to a
// genuine no-action drop instead of shifting via the relexed symbol), while
// every other Stop field (boundary, detail, byte offset) stays identical
// either way -- so NoActionDrops is the only field this regression would
// move, and pinning it here catches a future regression the terminal Stop
// alone would silently hide.
//
// Gated behind GTS_ADMISSION_REAL_CORPUS=1 like every other
// cgo_harness/corpus_real consumer in this package
// (admission_real_corpus_matrix_test.go): that directory is git-ignored and
// no CI job provisions it (the CI gate coverage record (Hyphae space m31labs/gotreesitter)), so this test must
// skip cleanly, not fail, when it is absent. F3's CI-covered anchor below
// pins the same-span accept contract the shift path depends on over an
// embedded-grammar fixture; no CI test drives the shift itself.
func TestD2SpanUnlockedSameSpanRelexStillShiftsRealBashInstallWitness(t *testing.T) {
	if os.Getenv("GTS_ADMISSION_REAL_CORPUS") != "1" {
		t.Skip("set GTS_ADMISSION_REAL_CORPUS=1 to run this cgo_harness/corpus_real-backed witness")
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("bash")
	if entry == nil {
		t.Fatal("bash is absent from the language registry")
	}
	lang := entry.Language()
	source, err := os.ReadFile(filepath.Join("cgo_harness", "corpus_real", "bash", "medium__install.sh"))
	if err != nil {
		t.Fatal(err)
	}

	receipt, err := gts.RunStateDependentRelexSchedulerForTest(lang, source)
	if err != nil {
		t.Fatalf("compact scheduler: %v", err)
	}
	if got, want := receipt.Stop.Work.NoActionDrops, uint64(16); got != want {
		t.Fatalf("NoActionDrops = %d, want %d (a same-span relex regressed for tree-sitter-bash's examples/install.sh)", got, want)
	}
}

// TestD2SpanUnlockedDisableOptionMatchesLegacyBehavior is the D2-1 Phase 2
// item 1c anchor (F4): DisablePerHeaderSpanUnlockedRelex restores the
// pre-D2-1 span-locked probe exactly, using swiftRaggedEndWitnessSource
// (the same witness TestD2SpanUnlockedSchedulerActivatesOwnedLexerRequests
// pins) so the two routes are provably different, not just superficially
// identical: enabled activates owned per-header requests;
// disabled produces exactly diagnosticParserCoreNoTableActionDetail, the
// origin/main behavior for this witness (relexTokenForState never returns
// a different-span token at all when disabled, so the no-action head takes
// the ordinary genuinely-empty-row path instead). Mutation-verified:
// deleting the option check at relexTokenForState's own
// DisablePerHeaderSpanUnlockedRelex guard makes disabled match enabled
// instead, and this test's disabled-detail assertion below fails.
func TestD2SpanUnlockedDisableOptionMatchesLegacyBehavior(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("swift")
	if entry == nil {
		t.Fatal("swift is absent from the language registry")
	}
	lang := entry.Language()

	enabled, err := gts.RunStateDependentRelexSchedulerForTest(lang, swiftRaggedEndWitnessSource)
	if err != nil {
		t.Fatalf("enabled: compact scheduler: %v", err)
	}
	disabled, err := gts.RunStateDependentRelexSchedulerWithSpanUnlockedRelexDisabledForTest(lang, swiftRaggedEndWitnessSource)
	if err != nil {
		t.Fatalf("disabled: compact scheduler: %v", err)
	}

	if enabled.Acceptance == nil || disabled.Acceptance != nil {
		t.Fatalf("enabled/disabled acceptance=%t/%t, want true/false", enabled.Acceptance != nil, disabled.Acceptance != nil)
	}
	if enabled.Stop.Detail != "" || disabled.Stop.Boundary != gts.DiagnosticParserCoreRecovery {
		t.Fatalf("enabled stop=%+v disabled boundary=%q, want accepted/recovery", enabled.Stop, disabled.Stop.Boundary)
	}
	wantDisabledDetail := gts.DiagnosticParserCoreNoTableActionDetailForTest()
	if disabled.Stop.Detail != wantDisabledDetail {
		t.Fatalf("disabled detail = %q, want %q (origin/main's own legacy detail)", disabled.Stop.Detail, wantDisabledDetail)
	}
}
