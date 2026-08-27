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

// D2-1 slice: span-unlocked per-header relex with fail-closed ragged-end
// decline. relexTokenForState used to reject any per-header relex whose
// span (StartByte and EndByte) did not exactly match the shared election's
// span. This slice unlocks EndByte: a same-start relex may now return a
// wider or narrower token than the shared election, and dispatchPassActive
// declines that shape fail-closed instead of shifting it (see
// diagnosticParserCoreRaggedRelexDeclineDetail's doc comment).
//
// bashRaggedEndWitnessSource ("A(1%") is a real, minimal witness found by
// targeted fuzzing of FuzzAdmissionRouteEquality's own bash lane (not
// hand-crafted): at byte 2, the shared election is a 1-byte token (symbol
// 160, span 2..3); the no-action header's own state (7338) relexes the
// same start byte to a 2-byte token (symbol 1, span 2..4). Reproduced
// deterministically via RelexTokenForStateForTest below. This witness's
// own no-action head is drop-eligible on every recurrence during a full
// scheduler run (a sibling header keeps advancing the same pass), so it
// does not reach the scheduler-level ragged-end decline itself; see
// swiftRaggedEndWitnessSource below for that.
var bashRaggedEndWitnessSource = []byte("A(1%")

// swiftRaggedEndWitnessSource ("a<A<#-") is the scheduler-level, real
// witness for the D2-1 ragged-end (different-span) decline (F1): at byte
// 3, swift's shared election is a 2-byte external-scanner token (symbol
// 217, span 3..5, the "<#" raw-string-literal opener); a sibling header's
// own state (47) relexes the same start byte to a narrower 1-byte token
// (symbol 35, span 3..4, a plain "<" operator). Unlike
// bashRaggedEndWitnessSource, every live header here is a no-action head
// and at least one is ragged, so this shape is not drop-eligible: the
// terminal Stop for this witness is the ragged-end decline itself (see
// TestD2SpanUnlockedSchedulerDeclinesRaggedEndRelex).
var swiftRaggedEndWitnessSource = []byte("a<A<#-")

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

// TestD2SpanUnlockedRaggedRelexDeclineDetailCarriesWiderTokenSpan pins
// the decline detail's exact format (Phase 1 item 4: receipts must record
// the wider token's own symbol and span) against the same real witness's
// exact values.
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

// TestD2SpanUnlockedSchedulerDeclinesRaggedEndRelex is the scheduler-level
// fail-closed proof (Phase 2 item 1a, F1) for swiftRaggedEndWitnessSource:
// the terminal Stop for the whole parse is the D2-1 ragged-end decline
// itself (boundary recovery, detail prefixed by
// diagnosticParserCoreRaggedRelexDeclineDetail's own text), not the
// ordinary genuinely-empty-row boundary. Earlier evidence for this slice
// (bashRaggedEndWitnessSource) claimed that branch was unreachable unless
// every live header in a pass were simultaneously ragged; that claim was
// wrong. The true condition is only that every live header is a no-action
// head and at least one of them is ragged -- a shape swiftRaggedEndWitnessSource
// exercises directly, and one this test's own mutation-verify step
// (recorded in the D2-1 report) confirms kills a reverted decline.
//
// The primary assertion is the terminal Stop's boundary and detail (F2):
// asserting there directly proves dispatchPassActive's own fail-closed
// branch ran. The secondary NoActionDrops loop below is kept only as a
// weaker check and cannot, on its own, detect a span leak: a
// DiagnosticParserCoreGenericNoActionDrop's Token is the shared election's
// own token by construction (a cell can never carry a different EndByte --
// dispatchPassActive declines that shape before a cell is built), so a
// leaked shift would remove the drop record entirely rather than change
// its span.
//
// D2-2a note: swiftRaggedEndWitnessSource is also the D2-2a "second witness"
// (spore.2026-08-27.rowan.d2-2a-unanimous-relex-adoption): with unanimous
// relex adoption on (the D2-2a zero-value default), every guard for this
// exact pass holds, so the scheduler adopts the relexed token instead of
// reaching this decline at all, and the parse instead surfaces a separate,
// already-recorded converged-path alternative-set coverage gap (B4b) a
// later pass hits. That gap is a distinct, filed defect, not this test's
// concern (see the D2Adoption B4b test), so this test forces
// DisableUnanimousRelexAdoption to keep pinning D2-1's own ragged-end
// decline shape in isolation.
func TestD2SpanUnlockedSchedulerDeclinesRaggedEndRelex(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("swift")
	if entry == nil {
		t.Fatal("swift is absent from the language registry")
	}
	lang := entry.Language()

	receipt, err := gts.RunStateDependentRelexSchedulerWithUnanimousRelexAdoptionDisabledForTest(lang, swiftRaggedEndWitnessSource)
	if err != nil {
		t.Fatalf("compact scheduler: %v", err)
	}
	if receipt.Acceptance != nil {
		t.Fatal("compact scheduler unexpectedly accepted the ragged-end witness")
	}
	if receipt.Stop.Boundary != gts.DiagnosticParserCoreRecovery {
		t.Fatalf("terminal Stop.Boundary = %q, want %q", receipt.Stop.Boundary, gts.DiagnosticParserCoreRecovery)
	}
	wantPrefix := gts.DiagnosticParserCoreRaggedRelexDeclineDetailForTest()
	if !strings.HasPrefix(receipt.Stop.Detail, wantPrefix) {
		t.Fatalf("terminal Stop.Detail = %q, want prefix %q", receipt.Stop.Detail, wantPrefix)
	}

	// Secondary, weaker check (see doc comment above): no no-action drop
	// ever carries the narrower relexed token in place of the shared
	// election's own token.
	for _, drop := range receipt.NoActionDrops {
		if drop.Token.StartByte != 3 {
			continue
		}
		if drop.Token.EndByte != 5 || drop.Token.Symbol != 217 {
			t.Fatalf("no-action drop at byte 3 = %+v, want the shared election's own token (symbol=217, end=5), never the relexed one", drop.Token)
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
// no CI job provisions it (docs/ci-gate-coverage.md), so this test must
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
// (the same witness TestD2SpanUnlockedSchedulerDeclinesRaggedEndRelex
// pins) so the two routes are provably different, not just superficially
// identical: enabled produces the D2-1 ragged-end decline's own detail;
// disabled produces exactly diagnosticParserCoreNoTableActionDetail, the
// origin/main behavior for this witness (relexTokenForState never returns
// a different-span token at all when disabled, so the no-action head takes
// the ordinary genuinely-empty-row path instead). Mutation-verified:
// deleting the option check at relexTokenForState's own
// DisablePerHeaderSpanUnlockedRelex guard makes disabled match enabled
// instead, and this test's disabled-detail assertion below fails.
//
// D2-2a note: "enabled" here forces DisableUnanimousRelexAdoption to isolate
// the D2-1 span-unlocked-vs-span-locked comparison this test owns from
// D2-2a's own, separately-tested, now-default-on adoption (see the doc
// comment on TestD2SpanUnlockedSchedulerDeclinesRaggedEndRelex above for why
// this same witness's D2-2a-enabled outcome differs).
func TestD2SpanUnlockedDisableOptionMatchesLegacyBehavior(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("swift")
	if entry == nil {
		t.Fatal("swift is absent from the language registry")
	}
	lang := entry.Language()

	enabled, err := gts.RunStateDependentRelexSchedulerWithUnanimousRelexAdoptionDisabledForTest(lang, swiftRaggedEndWitnessSource)
	if err != nil {
		t.Fatalf("enabled: compact scheduler: %v", err)
	}
	disabled, err := gts.RunStateDependentRelexSchedulerWithSpanUnlockedRelexDisabledForTest(lang, swiftRaggedEndWitnessSource)
	if err != nil {
		t.Fatalf("disabled: compact scheduler: %v", err)
	}

	if enabled.Acceptance != nil || disabled.Acceptance != nil {
		t.Fatal("compact scheduler unexpectedly accepted the ragged-end witness")
	}
	if enabled.Stop.Boundary != gts.DiagnosticParserCoreRecovery || disabled.Stop.Boundary != gts.DiagnosticParserCoreRecovery {
		t.Fatalf("enabled/disabled boundaries = %q/%q, want both %q",
			enabled.Stop.Boundary, disabled.Stop.Boundary, gts.DiagnosticParserCoreRecovery)
	}
	wantEnabledPrefix := gts.DiagnosticParserCoreRaggedRelexDeclineDetailForTest()
	if !strings.HasPrefix(enabled.Stop.Detail, wantEnabledPrefix) {
		t.Fatalf("enabled detail = %q, want prefix %q (the D2-1 ragged-end decline)", enabled.Stop.Detail, wantEnabledPrefix)
	}
	wantDisabledDetail := gts.DiagnosticParserCoreNoTableActionDetailForTest()
	if disabled.Stop.Detail != wantDisabledDetail {
		t.Fatalf("disabled detail = %q, want %q (origin/main's own legacy detail)", disabled.Stop.Detail, wantDisabledDetail)
	}
}
