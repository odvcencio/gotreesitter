//go:build gts_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// ownedZeroWidthCatchUpWitnessScanner is a minimal external scanner standing
// in for perl's `_NONASSOC` marker in the owned-dispatch synthetic harness
// below: it unconditionally emits a zero-width token for its one external
// symbol when the external lex state row marks it valid, and declines
// (falling back to the internal DFA for `a`/`b`) otherwise. It carries no
// state (ExternalScannerIsStateless), so the synthetic snapshots below never
// need to populate a payload.
type ownedZeroWidthCatchUpWitnessScanner struct{}

func (ownedZeroWidthCatchUpWitnessScanner) Create() any               { return new(int) }
func (ownedZeroWidthCatchUpWitnessScanner) Destroy(any)               {}
func (ownedZeroWidthCatchUpWitnessScanner) Serialize(any, []byte) int { return 0 }
func (ownedZeroWidthCatchUpWitnessScanner) Deserialize(any, []byte)   {}
func (ownedZeroWidthCatchUpWitnessScanner) ExternalScannerIsStateless() bool { return true }

func (ownedZeroWidthCatchUpWitnessScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	lexer.SetResultSymbol(3) // marker
	lexer.MarkEnd()
	return true
}

// ownedZeroWidthCatchUpWitnessLanguage builds the smallest grammar that
// reproduces the perl `_NONASSOC` shape entirely inside owned-dispatch mode:
// two owned headers independently lexing "ab", one of which (state 21) must
// shift a zero-width external marker before it can shift the same `a`/`b`
// content its sibling (state 11) reads directly.
//
// Symbols: 0 end, 1 `a`, 2 `b`, 3 marker (external, zero-width).
// States:  11 sibling-start (wants `a` directly), 12 sibling-after-a (wants
// `b`, and has NO action for it -- this is the witness's stuck point);
// 21 rescued-start (wants the marker first), 22 rescued-after-marker (wants
// `a`), 23 rescued-after-a (wants `b`, and DOES have an action for it,
// unlike the sibling's state 12).
func ownedZeroWidthCatchUpWitnessLanguage() *Language {
	lang := &Language{
		Name:            "owned_zero_width_catch_up_witness",
		SymbolCount:     4,
		TokenCount:      3,
		SymbolNames:     []string{"end", "a", "b", "marker"},
		ExternalSymbols: []Symbol{3},
		ExternalScanner: ownedZeroWidthCatchUpWitnessScanner{},
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: 'a', Hi: 'a', NextState: 1},
				{Lo: 'b', Hi: 'b', NextState: 2},
			}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: make([]LexMode, 24),
	}
	// Every state's ExternalLexState defaults to 0 (row 0: marker invalid,
	// falls to the internal DFA above) except the rescued header's own
	// starting state, which needs the marker before anything else.
	lang.ExternalLexStates = [][]bool{
		{false}, // row 0: marker invalid
		{true},  // row 1: marker valid
	}
	lang.LexModes[21] = LexMode{ExternalLexState: 1}
	return lang
}

// ownedZeroWidthCatchUpWitnessTable is the genericConflictTable
// (parsercore_phase0_generic_conflict_internal_test.go) encoding the witness
// grammar's action table directly: no real grammar compile is needed since
// this scheduler's owned-dispatch path reads only (state, symbol) -> actions
// through this interface.
func ownedZeroWidthCatchUpWitnessTable() *genericConflictTable {
	return &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 11, symbol: 1}: {{Type: core.ActionShift, State: 12}},
		// {state: 12, symbol: 2} is deliberately absent: the sibling has no
		// action for `b` -- the witness's stuck point.
		{state: 21, symbol: 3}: {{Type: core.ActionShift, State: 22}},
		{state: 22, symbol: 1}: {{Type: core.ActionShift, State: 23}},
		{state: 23, symbol: 2}: {{Type: core.ActionShift, State: 24}},
	}}
}

// newOwnedZeroWidthCatchUpStartSnapshot builds the byte-zero starting
// snapshot both witness headers seed from. Unlike
// newDiagnosticParserCoreOwnedLexerSnapshot's own callers
// (parsercore_phase0_per_version_lexer_lifecycle_internal_test.go), this
// harness's language declares a real (though stateless) ExternalScanner, so
// the dfaRelexSnapshot must set externalScannerPresent explicitly --
// newDiagnosticParserCoreVersionLexerSnapshot rejects a snapshot whose
// presence flag disagrees with the language's own scanner contract.
func newOwnedZeroWidthCatchUpStartSnapshot(t *testing.T, compact *core.Core, language *Language) *diagnosticParserCoreVersionLexerSnapshot {
	t.Helper()
	var snapshot *diagnosticParserCoreVersionLexerSnapshot
	err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var snapshotErr error
		snapshot, snapshotErr = newDiagnosticParserCoreVersionLexerSnapshot(
			compact, language, owner,
			dfaRelexSnapshot{lexerPos: 0, externalScannerPresent: true},
			0, 0,
		)
		return snapshotErr
	})
	if err != nil {
		t.Fatalf("construct owned lexer start snapshot: %v", err)
	}
	return snapshot
}

// newOwnedZeroWidthCatchUpWitnessScheduler wires the witness grammar and
// table up to a two-header owned-dispatch scheduler, seeded at state 11
// (sibling) and state 21 (rescued), both at byte 0 of "ab". Each header
// starts with no pending request (lexerRequest: 0), so classifyVersionLexerCell's
// own requestHeaderLexerToken issues its first request through the real
// dfaTokenSource -- this harness drives genuine per-header lexing, not
// pre-scripted tokens.
func newOwnedZeroWidthCatchUpWitnessScheduler(t *testing.T) *diagnosticParserCoreGenericScheduler {
	t.Helper()
	lang := ownedZeroWidthCatchUpWitnessLanguage()
	table := ownedZeroWidthCatchUpWitnessTable()
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatalf("construct compact core: %v", err)
	}
	siblingHead, err := compact.Seed(11, 0)
	if err != nil {
		t.Fatalf("seed sibling head: %v", err)
	}
	rescuedHead, err := compact.Seed(21, 0)
	if err != nil {
		t.Fatalf("seed rescued head: %v", err)
	}
	source := []byte("ab")
	// hasAnyActionForSymbol (parser_dfa_token_source.go) uses this lookup to
	// decide whether a zero-width token is usable at all, independent of the
	// compact core's own grammar table (genericConflictTable above); it must
	// mirror this witness's own action cells or Next() silently skips the
	// marker as dead weight.
	lookupActionIndex := func(state StateID, sym Symbol) uint16 {
		switch {
		case state == 11 && sym == 1, state == 21 && sym == 3,
			state == 22 && sym == 1, state == 23 && sym == 2:
			return 1
		default:
			return 0
		}
	}
	tokenSource := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, lookupActionIndex, nil, nil, nil)
	t.Cleanup(tokenSource.Close)

	startSnapshot := newOwnedZeroWidthCatchUpStartSnapshot(t, compact, lang)
	scannerScratch := make([]byte, 0, 64)

	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: tokenSource,
		headers: []diagnosticParserCoreHeader{
			{head: siblingHead, versionState: &diagnosticParserCoreVersionState{relexSnapshot: startSnapshot}},
			{head: rescuedHead, versionState: &diagnosticParserCoreVersionState{relexSnapshot: startSnapshot}},
		},
		electionIndex:               7,
		versionLexerOwnershipActive: true,
		scannerScratch:              &scannerScratch,
		options: DiagnosticParserCorePrefixOptions{
			ReceiptMode:   DiagnosticParserCoreReceiptSummary,
			MaxDispatches: 64,
		},
		receipt:                     &DiagnosticParserCoreGenericScheduler{},
		ownedZeroWidthCatchUpBudget: maxConsecutiveZeroWidthTokensExternal,
	}
	return scheduler
}

// runOwnedDispatchUntilStuckOrDone drives dispatchPass repeatedly, mirroring
// run()'s own loop (parsercore_phase0_driver.go): between dispatchPass calls
// it replicates run()'s own allClosed check and, when every live header has
// shifted, calls beginNextVersionLexerElection to open the next owned round
// exactly as run() would -- dispatchPass alone has no way to advance a fully
// closed frontier (that is beginNextVersionLexerElection's own job), so a
// harness that only calls dispatchPass would misreport "no runnable head" as
// a decline instead of a normal round boundary.
func runOwnedDispatchUntilStuckOrDone(t *testing.T, scheduler *diagnosticParserCoreGenericScheduler) (*diagnosticParserCoreGenericUnsupported, error) {
	t.Helper()
	for round := 0; round < 32; round++ {
		allClosed := true
		for index := range scheduler.headers {
			header := &scheduler.headers[index]
			if !header.shifted && !header.accepted {
				allClosed = false
				break
			}
		}
		if allClosed {
			// A sole surviving head closing its round is this harness's own
			// success shape: run()'s own single-head handling
			// (rejoinSharedLexerFromOwnedHeader + elect) needs a full shared
			// dispatch setup this synthetic table-only harness does not
			// build, and this test only needs to observe that the drop
			// happened and left the rescued header where it should be.
			if len(scheduler.headers) < 2 {
				return nil, nil
			}
			if err := scheduler.beginNextVersionLexerElection(); err != nil {
				return nil, err
			}
			continue
		}
		stop, err := scheduler.dispatchPass()
		if err != nil || stop != nil {
			return stop, err
		}
	}
	return nil, nil
}

// TestOwnedDispatchZeroWidthCatchUpAdmitsRaggedNoActionDrop is the
// synthetic-harness witness for this task: without ownedZeroWidthCatchUp
// (relexZeroWidthExternalTokenForState's port target), the rescued header's
// zero-width marker shift permanently costs it one owned request relative to
// its sibling, so by the time the sibling gets stuck at state 12 with no
// action for `b`, the rescued header's own last real shift (`a`, byte 0..1)
// starts one byte behind the sibling's stuck token (`b`, byte 1..2) and
// versionLexerNoActionDropEligible's same-start-byte proof declines. With
// ownedZeroWidthCatchUp active, the rescued header re-requests immediately
// after its zero-width marker shift instead of waiting for the next full
// barrier round, so both headers reach real content at the same pace and the
// drop succeeds once the sibling is provably stuck.
func TestOwnedDispatchZeroWidthCatchUpAdmitsRaggedNoActionDrop(t *testing.T) {
	scheduler := newOwnedZeroWidthCatchUpWitnessScheduler(t)
	stop, err := runOwnedDispatchUntilStuckOrDone(t, scheduler)
	if err != nil {
		t.Fatalf("owned dispatch run: %v", err)
	}
	if stop != nil {
		t.Fatalf("owned dispatch declined: %+v", stop)
	}
	// The sibling (no action for `b`) must have been dropped, leaving only
	// the rescued header, now past its own `b` shift (state 24).
	if len(scheduler.headers) != 1 {
		t.Fatalf("headers after drop = %d, want 1 (only the rescued header should survive)", len(scheduler.headers))
	}
	state, byteOffset, err := scheduler.compact.Boundary(scheduler.headers[0].head)
	if err != nil {
		t.Fatalf("boundary of surviving head: %v", err)
	}
	if state != 24 || byteOffset != 2 {
		t.Fatalf("surviving head state=%d byteOffset=%d, want state=24 byteOffset=2 (rescued header past its own `b` shift)", state, byteOffset)
	}
}

// TestOwnedDispatchZeroWidthCatchUpDeclinesWithoutBudget proves the catch-up
// mechanism is what makes the drop possible, not some other latent behavior:
// with its budget exhausted up front, the exact same harness reproduces
// today's decline (generic scheduler has no table action for the elected
// token), matching this task's own investigation of the perl `_NONASSOC`
// witness before this port.
func TestOwnedDispatchZeroWidthCatchUpDeclinesWithoutBudget(t *testing.T) {
	scheduler := newOwnedZeroWidthCatchUpWitnessScheduler(t)
	scheduler.ownedZeroWidthCatchUpBudget = 0
	stop, err := runOwnedDispatchUntilStuckOrDone(t, scheduler)
	if err != nil {
		t.Fatalf("owned dispatch run: %v", err)
	}
	if stop == nil {
		t.Fatal("owned dispatch accepted the ragged frontier with the catch-up budget exhausted, want the pre-fix decline")
	}
	if stop.boundary != DiagnosticParserCoreRecovery || stop.detail != diagnosticParserCoreNoTableActionDetail {
		t.Fatalf("decline = %+v, want boundary=%s detail=%q", stop, DiagnosticParserCoreRecovery, diagnosticParserCoreNoTableActionDetail)
	}
}
